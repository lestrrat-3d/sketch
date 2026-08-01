package sketch_test

import (
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
// carries the file as well as the function so the audit FAILS CLOSED: a switch
// added later is exhaustive-by-default and has to be listed here to be excused,
// rather than being excused by matching a bare function name.
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
func TestEntityTypeSwitchesAreExhaustive(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err, `failed to glob the package directory`)
	require.NotEmpty(t, files, `no Go source found in the package directory`)

	fset := token.NewFileSet()
	parsed := make(map[string]*ast.File, len(files))
	names := make([]string, 0, len(files))
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		require.NoErrorf(t, err, `failed to parse %s`, path)
		parsed[filepath.Base(path)] = f
		names = append(names, filepath.Base(path))
	}
	sort.Strings(names)

	// The entity set is derived from the source: a type is an Entity exactly when
	// it declares the sealed interface's entity() method.
	entities := make(map[string]struct{})
	for _, name := range names {
		for _, decl := range parsed[name].Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "entity" || fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if id, ok := star.X.(*ast.Ident); ok {
				entities[id.Name] = struct{}{}
			}
		}
	}
	require.Equal(t, entityTypeNames, sortedKeys(entities),
		`the Entity set moved: add the new type to entityTypeNames, then give it a case in every exhaustive switch this test guards`)

	used := make(map[entitySwitchSite]struct{})
	for _, name := range names {
		for _, decl := range parsed[name].Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			site := entitySwitchSite{File: name, Func: fd.Name.Name}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSwitchStmt)
				if !ok {
					return true
				}
				covered, isEntitySwitch := entitySwitchCases(ts, entities)
				if !isEntitySwitch {
					return true
				}
				missing := missingEntities(entities, covered)
				if len(missing) == 0 {
					return true
				}
				if _, exempt := entitySwitchExempt[site]; exempt {
					used[site] = struct{}{}
					return true
				}
				t.Errorf("%s: entity type switch in %s at line %d does not handle %s; give it a case, or record why it is deliberately partial in entitySwitchExempt",
					name, fd.Name.Name, fset.Position(ts.Pos()).Line, strings.Join(missing, ", "))
				return true
			})
		}
	}

	// A stale exemption is an audit hole in waiting: it would silently excuse a
	// switch that later stops being exhaustive.
	for site := range entitySwitchExempt {
		if _, ok := used[site]; !ok {
			t.Errorf("%s: %s no longer holds a partial entity type switch; drop its entitySwitchExempt entry", site.File, site.Func)
		}
	}
}

// entitySwitchCases reports the entity types a type switch handles, and whether
// it is an entity switch at all. It is one when every pointer case names an
// entity type and at least one case does — so a switch over constraints, over
// options or over the conic adapters is left alone.
func entitySwitchCases(ts *ast.TypeSwitchStmt, entities map[string]struct{}) (map[string]struct{}, bool) {
	covered := make(map[string]struct{})
	for _, stmt := range ts.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		// A clause may name several types (`case *Line, *Arc:`); each counts.
		for _, expr := range cc.List {
			star, ok := expr.(*ast.StarExpr)
			if !ok {
				continue
			}
			id, ok := star.X.(*ast.Ident)
			if !ok {
				continue
			}
			if _, ok := entities[id.Name]; !ok {
				return nil, false
			}
			covered[id.Name] = struct{}{}
		}
	}
	return covered, len(covered) > 0
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
