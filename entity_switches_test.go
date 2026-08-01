package sketch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// entityTypeNames is every type implementing the sealed Entity interface. It is
// asserted against the source below rather than trusted, so adding an entity
// type also trips this named list and brings the author here.
var entityTypeNames = []string{
	"Arc",
	"Circle",
	"ClosedSpline",
	"Conic",
	"Ellipse",
	"EllipticalArc",
	"FitSpline",
	"Line",
	"NURBS",
	"Spline",
}

// entitySwitchSite identifies one function's entity type switches by the file
// and function that hold them.
type entitySwitchSite struct{ File, Func string }

// entitySwitchExempt lists the entity type switches that are deliberately
// partial, each with the reason it handles fewer than every entity type. The key
// carries the file as well as the function so an entry names ONE site: a switch
// added later is exhaustive-by-default and has to be listed here to be excused,
// rather than being excused by matching a bare function name. One entry excuses
// exactly ONE switch, so a second, differently-partial switch dropped into an
// already-exempted function is reported rather than riding on the first one's
// reason.
var entitySwitchExempt = map[entitySwitchSite]string{
	{"tools.go", "Break"}:              "splitting is defined for a line and an arc only; every other type is the documented false return",
	{"tools.go", "lineCrossings"}:      "only line/circle/arc cutters have a closed-form line intersection in geom; a curve contributes no crossing",
	{"tools.go", "instantiate"}:        "mirror/pattern copies need the transform applied to a shape the point-relinking interface does not carry, so only line/circle/arc are copied",
	{"probe.go", "varKinds"}:           "classifies the variables an entity owns BEYOND its points; line/arc and the spline family own none",
	{"diagnose.go", "entityShapeVars"}: "intrinsic shape variables only; line/arc and the spline family have none, their shape being fixed by their points",
	{"sketch.go", "entitySizeVars"}:    "size variables only; line/arc and the spline family own no scalar variable beyond their points",
	{"removal.go", "RemoveEntity"}:     "retires entity-owned scalar variables; line/arc and the spline family own none, and their points survive removal",
	{"svg.go", "bounds"}:               "extends the box past the defining points; a line lies within its two endpoints, which the point loop above already added",
	{"constraint.go", "conicOf"}:       "adapts the sealed Circular/Elliptical operands only; the spline family and the conic entity are not conic-adaptable operands",
}

// TestEntityTypeSwitchesAreExhaustive audits every entity type switch in the
// package for full coverage, so adding an entity type cannot silently skip a
// switch that must handle it. Only Sketch.localPolyline reports an unhandled
// type at run time; JSON serialization, the three exporters, the profile pass,
// the revision fingerprint and the removal renumbering all fall through their
// switch and drop the entity with no error anywhere, while build, vet, lint and
// the rest of the suite stay green.
//
// It reads the entity contract CLAUDE.md states for the sketch.go row: an entity
// type declares its own entity() marker directly on itself with a pointer
// receiver, and is matched as *T. Embedding is not a supported way to become an
// entity, and this audit is syntactic — it recognizes the contract's forms and
// nothing else.
func TestEntityTypeSwitchesAreExhaustive(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	require.NoError(t, err, `failed to glob the package directory`)
	require.NotEmpty(t, paths, `no Go source found in the package directory`)

	fset := token.NewFileSet()
	parsed := make(map[string]*ast.File, len(paths))
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		require.NoErrorf(t, err, `failed to parse %s`, path)
		parsed[filepath.Base(path)] = f
		names = append(names, filepath.Base(path))
	}
	sort.Strings(names)

	facts := entitySwitchFactsOf(parsed, names)
	require.Equal(t, entityTypeNames, sortedKeys(facts.entities),
		`the Entity set moved: add the new type to entityTypeNames, then give it a case in every exhaustive switch this test guards`)

	for _, problem := range auditEntitySwitches(fset, parsed, names, facts, entitySwitchExempt) {
		t.Error(problem)
	}
}

// entitySwitchFacts is the package-level naming a type switch's cases are
// resolved against.
type entitySwitchFacts struct {
	// entities is every type declaring the sealed interface's entity() method.
	entities map[string]struct{}
	// aliases maps a same-package alias (`type A = B`) to the name it stands for.
	// Without it an aliased case name misses the entity set, which de-classifies
	// the whole switch and lets it through unaudited.
	aliases map[string]string
	// ifaces is every package-level interface that carries Entity, directly or
	// through another such interface — Entity itself, Circular, Elliptical. A case
	// naming one covers several concrete entity types, and which ones is a method
	// set this audit does not compute.
	ifaces map[string]struct{}
}

// resolve follows an alias chain to the name it finally stands for. The loop is
// bounded because a cycle would not compile, but the audit must not hang on
// source it merely parses.
func (f entitySwitchFacts) resolve(name string) string {
	for i := 0; i < 32; i++ {
		target, ok := f.aliases[name]
		if !ok {
			return name
		}
		name = target
	}
	return name
}

// entitySwitchFactsOf derives the entity set, the alias map and the entity-ish
// interfaces from the source, so none of the three is a hand-kept list.
func entitySwitchFactsOf(parsed map[string]*ast.File, names []string) entitySwitchFacts {
	facts := entitySwitchFacts{
		entities: make(map[string]struct{}),
		aliases:  make(map[string]string),
		// Entity is entity-ish by definition, whether or not the source under audit
		// happens to spell out its declaration.
		ifaces: map[string]struct{}{"Entity": {}},
	}
	// embeds records each package-level interface's embedded bare identifiers, the
	// only shape "this interface carries Entity" can be answered from without
	// computing method sets.
	embeds := make(map[string][]string)
	for _, name := range names {
		for _, decl := range parsed[name].Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				// A type is an Entity exactly when it declares the sealed interface's
				// entity() method.
				if fd.Name.Name != "entity" || fd.Recv == nil || len(fd.Recv.List) != 1 {
					continue
				}
				star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				if id, ok := star.X.(*ast.Ident); ok {
					facts.entities[id.Name] = struct{}{}
				}
				continue
			}
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				// An alias is a TypeSpec carrying an `=`; a definition is not.
				if ts.Assign != token.NoPos {
					if id, ok := ts.Type.(*ast.Ident); ok {
						facts.aliases[ts.Name.Name] = id.Name
					}
					continue
				}
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok || it.Methods == nil {
					continue
				}
				for _, field := range it.Methods.List {
					if len(field.Names) != 0 {
						continue // a method, not an embedded interface
					}
					if id, ok := field.Type.(*ast.Ident); ok {
						embeds[ts.Name.Name] = append(embeds[ts.Name.Name], id.Name)
					}
				}
			}
		}
	}

	// An interface is entity-ish by fixed point: Entity is, and so is any
	// interface embedding one that is. That reaches Circular and Elliptical
	// without a method set, which would otherwise need struct-embedding promotion.
	for changed := true; changed; {
		changed = false
		for name, embedded := range embeds {
			if _, ok := facts.ifaces[name]; ok {
				continue
			}
			for _, e := range embedded {
				if _, ok := facts.ifaces[facts.resolve(e)]; !ok {
					continue
				}
				facts.ifaces[name] = struct{}{}
				changed = true
				break
			}
		}
	}
	return facts
}

// auditEntitySwitches reports every problem the audit finds over the given
// files: a partial switch that is not exempt, a switch whose coverage cannot be
// computed, an exemption excusing more than one switch, a stale exemption, and a
// type switch sitting where the (file, function) key cannot name it. It is the
// whole verdict, so a synthetic package can be judged by exactly what the
// package's own source is.
func auditEntitySwitches(fset *token.FileSet, parsed map[string]*ast.File, names []string, facts entitySwitchFacts, exempt map[entitySwitchSite]string) []string {
	var problems []string
	used := make(map[entitySwitchSite]int)
	for _, name := range names {
		for _, decl := range parsed[name].Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				// The (file, function) exemption key can only name a switch inside a
				// function declaration, so one anywhere else — a package-level var
				// initializer's function literal, say — is refused rather than walked
				// past unaudited.
				ast.Inspect(decl, func(n ast.Node) bool {
					ts, ok := n.(*ast.TypeSwitchStmt)
					if !ok {
						return true
					}
					problems = append(problems, fmt.Sprintf("%s: type switch at line %d sits outside a function declaration, where this audit's (file, function) key cannot name it; move it into a named function",
						name, fset.Position(ts.Pos()).Line))
					return true
				})
				continue
			}
			site := entitySwitchSite{File: name, Func: fd.Name.Name}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSwitchStmt)
				if !ok {
					return true
				}
				cov := entitySwitchCases(ts, facts)
				if !cov.isEntity {
					return true
				}
				_, isExempt := exempt[site]
				if cov.unresolved != "" {
					if isExempt {
						used[site]++
						return true
					}
					problems = append(problems, fmt.Sprintf("%s: entity type switch in %s at line %d cannot be audited for coverage: case %s is an interface carrying Entity, and this audit does not expand one into the concrete types it admits; enumerate those types in cases of their own, or record why the switch is deliberately partial in entitySwitchExempt",
						name, fd.Name.Name, fset.Position(ts.Pos()).Line, cov.unresolved))
					return true
				}
				missing := missingEntities(facts.entities, cov.covered)
				if len(missing) == 0 {
					return true
				}
				if isExempt {
					used[site]++
					return true
				}
				problems = append(problems, fmt.Sprintf("%s: entity type switch in %s at line %d does not handle %s; give it a case, or record why it is deliberately partial in entitySwitchExempt",
					name, fd.Name.Name, fset.Position(ts.Pos()).Line, strings.Join(missing, ", ")))
				return true
			})
		}
	}

	// A stale exemption is an audit hole in waiting: it would silently excuse a
	// switch that later stops being exhaustive. A single entry excusing several
	// switches is the same hole reached from the other side — the second switch is
	// covered by a reason written for the first.
	sites := make([]entitySwitchSite, 0, len(exempt))
	for site := range exempt {
		sites = append(sites, site)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].File != sites[j].File {
			return sites[i].File < sites[j].File
		}
		return sites[i].Func < sites[j].Func
	})
	for _, site := range sites {
		switch n := used[site]; {
		case n == 0:
			problems = append(problems, fmt.Sprintf("%s: %s no longer holds a partial entity type switch; drop its entitySwitchExempt entry", site.File, site.Func))
		case n > 1:
			problems = append(problems, fmt.Sprintf("%s: %s holds %d partial entity type switches but one entitySwitchExempt entry, whose reason was written for one of them; make the others exhaustive, or split them into functions that can be exempted on their own terms", site.File, site.Func, n))
		}
	}
	return problems
}

// entitySwitchCoverage is what a type switch's cases say about the entity types
// it handles.
type entitySwitchCoverage struct {
	// covered is the entity types named by a concrete pointer case.
	covered map[string]struct{}
	// isEntity reports whether this is an entity type switch at all.
	isEntity bool
	// unresolved names the entity-ish interface a case is written against, if
	// any. Coverage is then not computable and the switch is refused rather than
	// blessed — expanding the interface would need method sets this audit does not
	// compute, and blessing what it cannot expand is exactly the hole it exists to
	// close (`case Circular:` alone covers Circle and Arc, not all ten types).
	unresolved string
}

// entitySwitchCases reports what a type switch's cases cover. It is an entity
// switch when at least one case names an entity type, or when a case names an
// interface carrying Entity — so a switch over constraints, over options or over
// the conic adapters is left alone. A case naming anything else contributes no
// coverage and is otherwise ignored: a bare nil (a test for the absent operand
// rather than for a type), and a pointer to a non-entity type.
func entitySwitchCases(ts *ast.TypeSwitchStmt, facts entitySwitchFacts) entitySwitchCoverage {
	cov := entitySwitchCoverage{covered: make(map[string]struct{})}
	for _, stmt := range ts.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		// A clause may name several types (`case *Line, *Arc:`); each counts.
		for _, expr := range cc.List {
			if id, ok := expr.(*ast.Ident); ok {
				if id.Name == "nil" {
					continue
				}
				if _, ok := facts.ifaces[facts.resolve(id.Name)]; ok {
					cov.unresolved = facts.resolve(id.Name)
				}
				continue
			}
			star, ok := expr.(*ast.StarExpr)
			if !ok {
				continue
			}
			id, ok := star.X.(*ast.Ident)
			if !ok {
				continue
			}
			name := facts.resolve(id.Name)
			if _, ok := facts.entities[name]; !ok {
				// A non-entity pointer case counts toward no coverage, but it does
				// NOT take the switch out of the audit. Refusing to classify here
				// is what let `case *Line, *Circle, *Point:` over an `any`
				// scrutinee — ordinary Go — go unaudited entirely. A switch that
				// legitimately mixes entity and non-entity cases is reported as
				// partial and takes an entitySwitchExempt entry with a stated
				// reason, like every other deliberately-partial switch.
				continue
			}
			cov.covered[name] = struct{}{}
		}
	}
	cov.isEntity = len(cov.covered) > 0 || cov.unresolved != ""
	return cov
}

// entitySwitchSynthFile is the file name the synthetic packages below are
// audited under.
const entitySwitchSynthFile = "synthetic.go"

// entitySwitchPrelude is the shape the audit reads a package by: three types
// carrying the sealed entity() marker, one type that carries no marker, plus an
// interface embedding Entity. The tests below are run against a synthetic
// package rather than against the tracked source, because the audit reads the
// package's own files and a construct planted there to prove a hole would have
// to be committed.
const entitySwitchPrelude = `package sketch

type Entity interface{ entity() }

type Alpha struct{}

func (x *Alpha) entity() {}

type Beta struct{}

func (x *Beta) entity() {}

type Gamma struct{}

func (x *Gamma) entity() {}

// Other is no entity, so a case naming it says nothing about entity coverage.
type Other struct{}

// Pair carries Entity, so a case naming it admits more than one concrete type.
type Pair interface {
	Entity
	pair()
}
`

// auditSynthetic runs the whole audit over one synthetic package.
func auditSynthetic(t *testing.T, src string, exempt map[entitySwitchSite]string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, entitySwitchSynthFile, src, 0)
	require.NoError(t, err, `failed to parse the synthetic package`)
	parsed := map[string]*ast.File{entitySwitchSynthFile: f}
	names := []string{entitySwitchSynthFile}
	return auditEntitySwitches(fset, parsed, names, entitySwitchFactsOf(parsed, names), exempt)
}

// TestEntitySwitchAuditResolvesAliases covers a switch written against a
// same-package alias of an entity type. Left unresolved, the alias name misses
// the entity set, which de-classifies the whole switch as "not an entity switch"
// and lets a partial one through unaudited.
func TestEntitySwitchAuditResolvesAliases(t *testing.T) {
	t.Run("partial through an alias is still reported", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
type betaAlias = Beta

type betaAliasAlias = betaAlias

func partialThroughAlias(e Entity) {
	switch e.(type) {
	case *Alpha:
	case *betaAliasAlias:
	}
}
`, nil)
		require.Len(t, problems, 1, `an alias case must not de-classify the switch it appears in`)
		require.Contains(t, problems[0], "partialThroughAlias")
		require.Contains(t, problems[0], "does not handle Gamma")
	})

	t.Run("an alias completes coverage", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
type betaAlias = Beta

func exhaustiveThroughAlias(e Entity) {
	switch e.(type) {
	case *Alpha:
	case *betaAlias:
	case *Gamma:
	}
}
`, nil)
		require.Empty(t, problems, `an alias case covers the type it stands for`)
	})
}

// TestEntitySwitchAuditRefusesInterfaceCases covers a switch written against an
// interface that carries Entity. Such a case admits several concrete types and
// which ones is a method set this audit does not compute, so the switch is
// refused rather than blessed: taking `case Pair:` for full coverage is the
// sharpest hole, since Pair admits only some of the entity types.
func TestEntitySwitchAuditRefusesInterfaceCases(t *testing.T) {
	t.Run("an interface case alone is not full coverage", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func interfaceOnly(e Entity) {
	switch e.(type) {
	case Pair:
	}
}
`, nil)
		require.Len(t, problems, 1, `an interface case must not pass as an exhaustive switch`)
		require.Contains(t, problems[0], "interfaceOnly")
		require.Contains(t, problems[0], "case Pair")
	})

	t.Run("coverage through an interface is refused, not false-flagged", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func coveredThroughInterface(e Entity) {
	switch e.(type) {
	case Pair:
	case *Gamma:
	}
}
`, nil)
		require.Len(t, problems, 1)
		require.Contains(t, problems[0], "cannot be audited for coverage")
		require.Contains(t, problems[0], "case Pair")
		require.NotContains(t, problems[0], "does not handle",
			`the switch may well route Alpha and Beta through Pair, so naming them unhandled states something untrue`)
	})

	t.Run("a catch-all Entity case is refused with the true reason", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func catchAll(e Entity) {
	switch e.(type) {
	case *Alpha:
	case Entity:
	}
}
`, nil)
		require.Len(t, problems, 1, `a catch-all must not bless a switch whose concrete coverage is unknown`)
		require.Contains(t, problems[0], "cannot be audited for coverage")
		require.Contains(t, problems[0], "case Entity")
		require.NotContains(t, problems[0], "does not handle",
			`the catch-all does route Beta and Gamma, so naming them unhandled states something untrue`)
	})

	t.Run("a default clause keeps the coverage verdict", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func withDefault(e Entity) {
	switch e.(type) {
	case *Alpha:
	default:
	}
}
`, nil)
		require.Len(t, problems, 1, `a default clause is the nine exempted sites' own shape and must stay a finding`)
		require.Contains(t, problems[0], "does not handle Beta, Gamma")
	})

	t.Run("a bare nil case is ignored", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func withNil(e Entity) {
	switch e.(type) {
	case nil:
	case *Alpha:
	case *Beta:
	case *Gamma:
	}
}
`, nil)
		require.Empty(t, problems, `nil tests for the absent operand, not for a type`)
	})
}

// TestEntitySwitchAuditKeepsNonEntityCases covers a switch that names an entity
// type beside a type that is not one. Refusing to classify it at the non-entity
// case took the WHOLE switch out of the audit, so `case *Alpha, *Beta, *Other:`
// over an `any` scrutinee — ordinary Go, and the one shape of bypass a person
// might write without meaning to — was audited by nothing at all.
func TestEntitySwitchAuditKeepsNonEntityCases(t *testing.T) {
	t.Run("a non-entity case does not take the switch out of the audit", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func partialBesideNonEntity(v any) {
	switch v.(type) {
	case *Alpha:
	case *Beta:
	case *Other:
	}
}
`, nil)
		require.Len(t, problems, 1, `a non-entity case must not carry the whole switch out of the audit`)
		require.Contains(t, problems[0], "partialBesideNonEntity")
		require.Contains(t, problems[0], "does not handle Gamma")
	})

	t.Run("a non-entity case counts toward no coverage", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func exhaustiveBesideNonEntity(v any) {
	switch v.(type) {
	case *Alpha, *Other:
	case *Beta:
	case *Gamma:
	}
}
`, nil)
		require.Empty(t, problems, `every entity type is handled, and the non-entity case is beside the point`)
	})

	t.Run("a switch naming no entity type is left alone", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func noEntityCase(v any) {
	switch v.(type) {
	case *Other:
	}
}
`, nil)
		require.Empty(t, problems, `a switch naming no entity type is not an entity switch`)
	})
}

// TestEntitySwitchAuditExemptionExcusesOneSwitch covers a second,
// differently-partial switch dropped into an already-exempted function. Counting
// only whether an exemption was touched lets that second switch ride on a reason
// written for the first.
func TestEntitySwitchAuditExemptionExcusesOneSwitch(t *testing.T) {
	exempt := map[entitySwitchSite]string{
		{entitySwitchSynthFile, "twoPartialSwitches"}: "the first switch is deliberately partial",
	}

	t.Run("one exemption does not excuse two switches", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func twoPartialSwitches(e Entity) {
	switch e.(type) {
	case *Alpha:
	}
	switch e.(type) {
	case *Beta:
	}
}
`, exempt)
		require.Len(t, problems, 1, `the second switch is covered by a reason written for the first`)
		require.Contains(t, problems[0], "twoPartialSwitches")
		require.Contains(t, problems[0], "holds 2 partial entity type switches")
	})

	t.Run("one exemption excuses one switch", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func twoPartialSwitches(e Entity) {
	switch e.(type) {
	case *Alpha:
	}
}
`, exempt)
		require.Empty(t, problems)
	})

	t.Run("an untouched exemption is stale", func(t *testing.T) {
		problems := auditSynthetic(t, entitySwitchPrelude+`
func twoPartialSwitches(e Entity) {
	switch e.(type) {
	case *Alpha:
	case *Beta:
	case *Gamma:
	}
}
`, exempt)
		require.Len(t, problems, 1)
		require.Contains(t, problems[0], "no longer holds a partial entity type switch")
	})
}

// TestEntitySwitchAuditReachesSwitchesOutsideFunctions covers a type switch in a
// package-level var initializer's function literal. Descending only into
// function declarations never visits it, so it is audited by nothing; and the
// (file, function) exemption key cannot name it, so it is refused where it sits.
func TestEntitySwitchAuditReachesSwitchesOutsideFunctions(t *testing.T) {
	problems := auditSynthetic(t, entitySwitchPrelude+`
var describe = func(e Entity) string {
	switch e.(type) {
	case *Alpha:
		return "alpha"
	}
	return ""
}
`, nil)
	require.Len(t, problems, 1, `a switch outside a function declaration must not go unaudited`)
	require.Contains(t, problems[0], "sits outside a function declaration")
}

// missingEntities lists the entity types a switch leaves unhandled.
func missingEntities(entities, covered map[string]struct{}) []string {
	var out []string
	for name := range entities {
		if _, ok := covered[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// sortedKeys returns a set's members in a deterministic order.
func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
