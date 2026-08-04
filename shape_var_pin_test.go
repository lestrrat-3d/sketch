package sketch_test

// A TRIPWIRE for the entity shape-variable set, not a prose linter.
//
// entityShapeVars (sketch.go) is the one definition of which intrinsic scalar
// solver variables each entity type owns beyond its points. Twenty-plus
// documentation and comment sites restate that set — or one of two sets
// derived from it — in prose the compiler does not check, and inspection has
// repeatedly failed to keep them in step (see docs/removal-design.md's
// "exhaustively" claim, false for seven weeks across two entity additions
// before a seven-round review caught it).
//
// A prose detector was prototyped and rejected: pattern-matching restatements
// out of comments and Markdown measured ~8% precision over ~4600 blocks and
// missed real sites in both directions, because the repo talks about circles,
// ellipses and radii constantly for reasons that have nothing to do with this
// set. Pinning exact prose excerpts was tried next and rejected too: this repo
// rewords documentation constantly, so every innocent reword reddens CI, the
// diff says nothing about correctness, and the predictable fix is pasting the
// new wording into the pin — a rubber stamp that pins nothing.
//
// This file does neither. It derives three sets from source with go/ast (the
// way entity_switches_test.go derives the entity set) and compares each
// against one pinned canonical string. On the happy path — the common case —
// it does nothing at all: the site list is read only when a set actually
// changes. Rewording a site's prose around a live anchor — on any path,
// including a site you did not touch — never fails it; only a SET change
// does, and then it prints every registered site's current text, marking any
// whose anchor no longer resolves so the site can be found by hand rather
// than blocking the run.
//
// THREE sets are pinned, not one, because there are three owners:
//
//   - entityShapeVars itself: which scalar variables (with physical kind) each
//     entity type owns beyond its points.
//   - The varKind GROUPING derived from it: for each physical kind (radius,
//     angle, dimensionless), which entities/selectors carry it — the shape
//     conditioning.go and probe.go classify columns and seed perturbations by.
//   - entityStructuralState (revision.go): a SUPERSET of entityShapeVars that
//     additionally covers a NURBS' degree/knots/weights — stored structural
//     data the solver cannot move, but that Sketch.Revision must still
//     fingerprint or a stale Profile reads fresh.
//
// A check pinned only to entityShapeVars leaves the third group unguarded.
import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// The three pins.
// ---------------------------------------------------------------------------

// shapeVarPin is entityShapeVars' own cases: for each entity type, its
// selector:kind pairs in source order, entity types sorted.
const shapeVarPin = "Circle=ri:varRadius|" +
	"Conic=rhoi:varDimensionless|" +
	"Ellipse=rxi:varRadius,ryi:varRadius,roti:varAngle|" +
	"EllipticalArc=rxi:varRadius,ryi:varRadius,roti:varAngle"

// varKindGroupPin is the same data as shapeVarPin, regrouped by physical
// kind rather than by entity type — the question conditioning.go's column
// scaling and probe.go's seeding switch answer.
const varKindGroupPin = "varAngle=Ellipse.roti,EllipticalArc.roti|" +
	"varDimensionless=Conic.rhoi|" +
	"varRadius=Circle.ri,Ellipse.rxi,Ellipse.ryi,EllipticalArc.rxi,EllipticalArc.ryi"

// structuralStatePin is entityStructuralState's cases: for each entity type
// (including the ones with nothing to hash), the accessor names it reads off
// the switch variable, in source order, entity types sorted.
const structuralStatePin = "Arc=|" +
	"Circle=r|" +
	"ClosedSpline=|" +
	"Conic=rho|" +
	"Ellipse=rx,ry,rot|" +
	"EllipticalArc=rx,ry,rot|" +
	"FitSpline=|" +
	"Line=|" +
	"NURBS=degree,knots,weights|" +
	"Spline="

// ---------------------------------------------------------------------------
// The site registry — hand-curated, not derived. If a restatement site is
// missing here, that is a bug in this list: add it rather than treat its
// absence as evidence the class is bounded. The Note strings below are
// themselves restatements of set membership and no anchor can cover them —
// when a pinned set changes, re-read every Note along with the site text it
// introduces, since a Note can go stale exactly as the prose it points at
// does.
// ---------------------------------------------------------------------------

// shapeVarGroup names which of the three pins a site restates.
type shapeVarGroup string

const (
	groupShapeVars   shapeVarGroup = "entityShapeVars"
	groupVarKind     shapeVarGroup = "varKind grouping"
	groupStructState shapeVarGroup = "entityStructuralState"
)

// shapeVarSite is one place in the tree whose prose depends on one of the
// three pinned sets. Anchor is a STRUCTURAL token — a Go identifier, a
// composite-literal key, a Markdown heading, a bolded label this repo uses as
// a heading, or a full `t.Run` subtest name — never a sentence or a prose
// excerpt, so an ordinary reword around it does not disturb it. A `t.Run`
// name is the accepted exception to "never a sentence": inside a subtest
// there is no declaration-level anchor to point at, and changing a test's own
// declared name is a deliberate edit to that test, not an incidental prose
// reword the way rewording a comment is. Before/Lines say how many lines
// above/below the anchor's own line to quote; for a CLAUDE.md row (one
// enormous line) they are ignored in favor of a character window around the
// anchor's offset within the line — see resolveShapeVarSite.
type shapeVarSite struct {
	Group  shapeVarGroup
	File   string
	Anchor string
	Note   string
	Before int
	Lines  int
}

var shapeVarSites = []shapeVarSite{
	// --- entityShapeVars: the per-entity definition itself -----------------
	{groupShapeVars, "sketch.go", "func entityShapeVars",
		"the definition's own doc comment states the set in full", 28, 0},
	{groupShapeVars, "CLAUDE.md", "`entityShapeVars`",
		"the sketch.go row states the set in full", 0, 0},
	{groupShapeVars, "entity_switches_test.go", `{"sketch.go", "entityShapeVars"}`,
		"the exemption reason names which types own none", 0, 0},
	{groupShapeVars, "profile_revision_test.go", "changing an entity's shape value",
		"test comment restates circle/ellipse/conic's shape values", 0, 5},
	{groupShapeVars, "docs/conic-design.md", "`entityShapeVars`",
		"the conic integration site names rho and its kind", 0, 3},
	{groupShapeVars, "docs/nurbs-design.md", "`entityShapeVars`",
		"the NURBS integration site asserts it owns no shape var", 0, 3},
	{groupShapeVars, "docs/nurbs-design.md", "**conditioning.go / probe.go**",
		"a second NURBS-owns-none restatement, for conditioning.go/probe.go", 0, 3},
	{groupShapeVars, "docs/removal-design.md", "`splOf`)",
		"negative statement: which types own none", 0, 7},
	{groupShapeVars, "docs/reference-geometry-design.md", "`Ellipse`/`Spline`",
		"the Model section names which entities own extra vars", 1, 4},
	{groupShapeVars, "docs/reference-geometry-design.md", "**Circle radius**",
		"the Locking section names an arc's and an ellipse's vars", 0, 5},
	{groupShapeVars, "docs/reference-geometry-design.md", "## Serialization",
		"the loader's unsupported-kind rejection names ellipse/spline", 0, 11},
	{groupShapeVars, "docs/reference-geometry-design.md", "## Open questions / follow-ups",
		"the follow-up list restates the ellipse/spline claim again", 0, 5},
	{groupShapeVars, "diagnose.go", "func (s *Sketch) EntityIsFullyConstrained",
		"doc comment's negative half: an arc's radius is derived, so it owns none", 13, 0},
	{groupShapeVars, "removal.go", "for _, v := range entityShapeVars(e)",
		"negative restatement above the loop: a line, an arc and the spline families own none", 4, 0},
	{groupShapeVars, "CLAUDE.md", "Sketch.EntityIsFullyConstrained",
		"the diagnose.go row's negative half: none for a line, an arc or the spline families", 0, 0},
	{groupShapeVars, "revision.go", "for _, e := range s.ents",
		"illustration inside the entity-uid-identity argument: a Line owns none", 0, 14},
	{groupShapeVars, "profile_revision_test.go", "func TestProfileStalenessEntityRecreated",
		"illustration inside the entity-uid-identity test comment: a Line owns none", 14, 0},
	{groupShapeVars, "revision_internal_test.go", "package sketch",
		"the file header comment restates entityShapeVars' per-type selectors", 0, 13},

	// --- the varKind grouping ------------------------------------------------
	{groupVarKind, "sketch.go", "varCoordinate",
		"the varKind constants' own comments name the owning types per kind", 0, 4},
	{groupVarKind, "conditioning.go", "func (s *Sketch) condVarScales",
		"condVarScales' doc comment restates the same grouping", 8, 0},
	{groupVarKind, "probe.go", "case varDimensionless:",
		"the seeding switch's case comments name the kinds by example", 11, 3},
	{groupVarKind, "probe.go", "func configSep",
		"configSep's doc comment enumerates one case per kind", 6, 0},
	{groupVarKind, "docs/conditioning-gate-design.md", "**Column scale `Dcol`**",
		"column-scale prose names the kinds by example", 0, 8},
	{groupVarKind, "docs/conditioning-gate-design.md", "**Variable kinds.**",
		"row/column classification restates radius/rotation", 0, 5},
	{groupVarKind, "docs/ambiguity-probe-design.md", "**Pseudo-random restarts**",
		"seeding section enumerates one case per kind", 0, 8},
	{groupVarKind, "docs/ambiguity-probe-design.md", "## Clustering",
		"clustering section enumerates one case per kind", 0, 8},

	// --- entityStructuralState: the superset ----------------------------
	{groupStructState, "revision.go", "func entityStructuralState",
		"entityStructuralState's own doc comment states the superset in full", 18, 0},
	{groupStructState, "revision.go", "func (s *Sketch) Revision",
		"Sketch.Revision's doc comment restates the per-entity shape state", 33, 0},
	{groupStructState, "CLAUDE.md", "**Shape values**",
		"the revision.go row enumerates the superset", 0, 0},
	{groupStructState, "profile_revision_test.go", "changing NURBS structural data",
		"test comment names NURBS' degree/knots/weights specifically", 0, 5},
	{groupStructState, "revision_internal_test.go", "func TestRevisionResolvesShapeValues",
		"the doc comment restates entityStructuralState's per-type accessors", 9, 0},
	{groupStructState, "shape_var_pin_test.go", "package sketch_test",
		"this file's own header states varKindGroupPin's three keys (radius, angle, dimensionless) and structuralStatePin's NURBS member", 0, 43},
}

// ---------------------------------------------------------------------------
// The test.
// ---------------------------------------------------------------------------

func TestEntityShapeVarSetsArePinned(t *testing.T) {
	shapeVars, kindGroups, structState := deriveShapeVarSets(t)

	shapeVarsOK := shapeVars == shapeVarPin
	kindGroupsOK := kindGroups == varKindGroupPin
	structStateOK := structState == structuralStatePin

	if shapeVarsOK && kindGroupsOK && structStateOK {
		// The happy path: no set changed, so the site list is not read at all.
		// A dead anchor here is a latent gap in the hint a future set change
		// will print, not something this run needs to know about.
		return
	}

	var b strings.Builder
	b.WriteString("\nOne of the three pinned entity shape-variable sets changed.\n\n")
	if !shapeVarsOK {
		fmt.Fprintf(&b, "entityShapeVars (sketch.go):\n  pinned: %s\n  source: %s\n\n", shapeVarPin, shapeVars)
	}
	if !kindGroupsOK {
		fmt.Fprintf(&b, "varKind grouping (derived from entityShapeVars):\n  pinned: %s\n  source: %s\n\n", varKindGroupPin, kindGroups)
	}
	if !structStateOK {
		fmt.Fprintf(&b, "entityStructuralState (revision.go):\n  pinned: %s\n  source: %s\n\n", structuralStatePin, structState)
	}
	b.WriteString(
		"Update shapeVarPin / varKindGroupPin / structuralStatePin in shape_var_pin_test.go LAST, " +
			"not first. Pinning the new value before checking every site below turns this test into a " +
			"rubber stamp: it will pass again, but the sites will still say what they said before the " +
			"set changed. Read each site's current text, fix the ones that are now wrong, THEN update the " +
			"pin(s) above to match the new source.\n\n" +
			"This list is curated, not derived — every place below restates one of the three sets in " +
			"prose the compiler does not check. If a change to entityShapeVars/varKinds/entityStructuralState " +
			"should have touched a site that is NOT listed here, that absence is a bug in shapeVarSites: add it.\n\n")
	for i, site := range shapeVarSites {
		text, ok := resolveShapeVarSite(site)
		fmt.Fprintf(&b, "%2d. [%s] %s — %s\n", i+1, site.Group, site.File, site.Note)
		if !ok {
			fmt.Fprintf(&b, "      !! anchor %q no longer resolves; find this site by hand\n\n", site.Anchor)
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			fmt.Fprintf(&b, "      %s\n", line)
		}
		b.WriteString("\n")
	}
	t.Error(b.String())
}

// ---------------------------------------------------------------------------
// Derivation: read entityShapeVars and entityStructuralState out of the
// package source with go/ast, and regroup entityShapeVars' cases by kind.
// ---------------------------------------------------------------------------

// shapeVarCase is one entityShapeVars case: an entity type and its
// selector:kind pairs, in source order.
type shapeVarCase struct {
	Type string
	Vars []struct{ Sel, Kind string }
}

func deriveShapeVarSets(t *testing.T) (shapeVars, kindGroups, structState string) {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	parsed := make([]*ast.File, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		require.NoErrorf(t, err, "failed to parse %s", path)
		parsed = append(parsed, f)
	}

	cases := deriveShapeVarCases(t, parsed)
	require.NotEmpty(t, cases, "entityShapeVars not found or has no cases; this test's AST reader needs updating")

	shapeVars = formatShapeVarPin(cases)
	kindGroups = formatVarKindGroupPin(cases)
	structState = deriveStructuralStatePin(t, parsed)
	return shapeVars, kindGroups, structState
}

// deriveShapeVarCases walks entityShapeVars' type switch and returns each
// case's entity type plus its {selector, kind} pairs, in source order.
func deriveShapeVarCases(t *testing.T, parsed []*ast.File) []shapeVarCase {
	t.Helper()
	var cases []shapeVarCase
	for _, f := range parsed {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "entityShapeVars" || fd.Recv != nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSwitchStmt)
				if !ok {
					return true
				}
				for _, stmt := range ts.Body.List {
					cc, ok := stmt.(*ast.CaseClause)
					if !ok {
						continue
					}
					for _, texpr := range cc.List {
						sc := shapeVarCase{Type: bareTypeName(texpr)}
						ast.Inspect(cc, func(m ast.Node) bool {
							cl, ok := m.(*ast.CompositeLit)
							if !ok || len(cl.Elts) != 2 {
								return true
							}
							sel, ok := cl.Elts[0].(*ast.SelectorExpr)
							if !ok {
								return true
							}
							kind, ok := cl.Elts[1].(*ast.Ident)
							if !ok {
								return true
							}
							sc.Vars = append(sc.Vars, struct{ Sel, Kind string }{sel.Sel.Name, kind.Name})
							return true
						})
						cases = append(cases, sc)
					}
				}
				return false
			})
		}
	}
	return cases
}

func formatShapeVarPin(cases []shapeVarCase) string {
	entries := make([]string, 0, len(cases))
	for _, c := range cases {
		parts := make([]string, 0, len(c.Vars))
		for _, v := range c.Vars {
			parts = append(parts, v.Sel+":"+v.Kind)
		}
		entries = append(entries, c.Type+"="+strings.Join(parts, ","))
	}
	sort.Strings(entries)
	return strings.Join(entries, "|")
}

// formatVarKindGroupPin regroups the same cases by physical kind: for each
// kind, every Type.selector that carries it, sorted.
func formatVarKindGroupPin(cases []shapeVarCase) string {
	byKind := map[string][]string{}
	for _, c := range cases {
		for _, v := range c.Vars {
			byKind[v.Kind] = append(byKind[v.Kind], c.Type+"."+v.Sel)
		}
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	entries := make([]string, 0, len(kinds))
	for _, k := range kinds {
		members := byKind[k]
		sort.Strings(members)
		entries = append(entries, k+"="+strings.Join(members, ","))
	}
	return strings.Join(entries, "|")
}

// deriveStructuralStatePin walks entityStructuralState's type switch
// (revision.go) and returns, per entity type sorted, the accessor names it
// reads off the switch variable (a method call like t.r() or a field read
// like t.degree), in source order — including the types with nothing to
// hash, so the pin states the negative cases explicitly rather than by
// omission.
func deriveStructuralStatePin(t *testing.T, parsed []*ast.File) string {
	t.Helper()
	type entry struct {
		Type string
		Sels []string
	}
	var entries []entry
	found := false
	for _, f := range parsed {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "entityStructuralState" || fd.Recv != nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSwitchStmt)
				if !ok {
					return true
				}
				found = true
				varName := switchVarName(ts)
				for _, stmt := range ts.Body.List {
					cc, ok := stmt.(*ast.CaseClause)
					if !ok {
						continue
					}
					var sels []string
					ast.Inspect(cc, func(m ast.Node) bool {
						sel, ok := m.(*ast.SelectorExpr)
						if !ok {
							return true
						}
						id, ok := sel.X.(*ast.Ident)
						if !ok || id.Name != varName {
							return true
						}
						sels = append(sels, sel.Sel.Name)
						return true
					})
					for _, texpr := range cc.List {
						entries = append(entries, entry{bareTypeName(texpr), sels})
					}
				}
				return false
			})
		}
	}
	require.True(t, found, "entityStructuralState not found; this test's AST reader needs updating")
	sort.Slice(entries, func(i, j int) bool { return entries[i].Type < entries[j].Type })
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Type+"="+strings.Join(e.Sels, ","))
	}
	return strings.Join(out, "|")
}

// switchVarName returns the bound variable's name in `switch t := e.(type)`.
func switchVarName(ts *ast.TypeSwitchStmt) string {
	as, ok := ts.Assign.(*ast.AssignStmt)
	if !ok || len(as.Lhs) != 1 {
		return ""
	}
	id, ok := as.Lhs[0].(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func bareTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return bareTypeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// ---------------------------------------------------------------------------
// Site resolution.
// ---------------------------------------------------------------------------

// claudeMDWindow is how many characters on each side of the anchor's offset
// to print for a file whose matching line may be enormous (CLAUDE.md rows are
// single lines that can run past 10,000 characters). Truncating from the
// start of such a line — as an earlier version of this check did — hides the
// relevant text entirely; a window centered on the anchor's own offset does
// not.
const claudeMDWindow = 240

// resolveShapeVarSite finds a site's anchor in its file and returns the
// current text around it, read fresh off disk.
func resolveShapeVarSite(site shapeVarSite) (string, bool) {
	data, err := os.ReadFile(site.File)
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(data), "\n")
	for i, l := range lines {
		idx := strings.Index(l, site.Anchor)
		if idx < 0 {
			continue
		}
		if len(l) > 2*claudeMDWindow {
			// A single enormous line (CLAUDE.md's table rows): quote a window
			// of characters centered on the anchor's own offset rather than
			// truncating from the line start, which would print unrelated
			// text from elsewhere in the row and never reach the anchor.
			start := idx - claudeMDWindow
			if start < 0 {
				start = 0
			}
			end := idx + len(site.Anchor) + claudeMDWindow
			if end > len(l) {
				end = len(l)
			}
			prefix, suffix := "", ""
			if start > 0 {
				prefix = "… "
			}
			if end < len(l) {
				suffix = " …"
			}
			return fmt.Sprintf("%s:%d (char %d)\n%s%s%s", site.File, i+1, idx, prefix, l[start:end], suffix), true
		}

		start := i - site.Before
		if start < 0 {
			start = 0
		}
		end := i + site.Lines + 1
		if end > len(lines) {
			end = len(lines)
		}
		out := make([]string, 0, end-start)
		for _, s := range lines[start:end] {
			if len(s) > 300 {
				s = s[:300] + " …"
			}
			out = append(out, s)
		}
		return fmt.Sprintf("%s:%d\n%s", site.File, start+1, strings.Join(out, "\n")), true
	}
	return "", false
}
