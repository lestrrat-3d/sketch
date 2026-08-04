package sketch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// nilOperandCase is one row of the recorded nil-operand contract: a public
// constraint constructor called with nil operands, and what each of the four
// introspection functions then does. The outcomes are RECORDED measurements,
// not a rule — the four functions behave differently per constraint type, and
// the difference is not derivable from introspect.go (it lives in each type's
// residual method in constraint.go and in its constraintRefs case in
// removal.go). Fields:
//
//   - ctor is the constructor's name, matched against constraint.go itself.
//   - variant is "" for a constructor whose operands are all concrete pointer
//     types, and "typed"/"untyped" for one taking a sealed interface operand
//     (Circular/Elliptical), where a caller can pass either a typed nil (a nil
//     *Circle stored in the interface) or an untyped nil — two different values
//     with two different outcomes.
//   - construct is "ok" or "panic": whether the constructor itself survives nil
//     operands. When it panics, the three outcome columns read "n/a" — nothing
//     was constructed to introspect.
//   - kind is what ConstraintKind returns, or "panic".
//   - refs describes ConstraintRefs's two slices element by element (see
//     describeConstraintRefs), or "panic".
//   - residuals is "len=N" for what ConstraintResiduals returns, or "panic".
type nilOperandCase struct {
	ctor      string
	variant   string
	build     func() sketch.Constraint
	construct string
	kind      string
	refs      string
	residuals string
}

// nilOperandOutcomes records, for every public constraint constructor in
// constraint.go, what the four introspection functions do when the constraint
// was built from nil operands. TestIntrospectionNilOperandOutcomes checks this
// list against the constructors declared in the source, so a constructor added
// later has to be recorded here before the suite passes.
//
// Two behaviours in this table are the reason it exists rather than a prose
// rule. ConstraintResiduals returns an EMPTY slice — no panic — for the twelve
// aux-variable constraints (the point-on and tangent-to families of spline,
// closed spline, fit spline, conic and NURBS, plus the two tangent-conics
// constructors), because their residual returns early while the auxiliary
// solver variables are unallocated, before any operand is read. And a nil
// ENTITY operand comes back from ConstraintRefs as a non-nil interface holding
// a nil pointer whenever the parameter is a concrete pointer type, so `ent ==
// nil` is FALSE for it; only a sealed-interface parameter handed an untyped nil
// produces an element that compares equal to nil.
var nilOperandOutcomes = []nilOperandCase{
	{"NewCoincident", "", func() sketch.Constraint { return sketch.NewCoincident(nil, nil) }, "ok", "coincident", "pts=[nil nil] ents=[]", "panic"},
	{"NewHorizontal", "", func() sketch.Constraint { return sketch.NewHorizontal(nil) }, "ok", "horizontal", "pts=[] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewVertical", "", func() sketch.Constraint { return sketch.NewVertical(nil) }, "ok", "vertical", "pts=[] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewHorizontalPoints", "", func() sketch.Constraint { return sketch.NewHorizontalPoints(nil, nil) }, "ok", "horizontal_points", "pts=[nil nil] ents=[]", "panic"},
	{"NewVerticalPoints", "", func() sketch.Constraint { return sketch.NewVerticalPoints(nil, nil) }, "ok", "vertical_points", "pts=[nil nil] ents=[]", "panic"},
	{"NewParallel", "", func() sketch.Constraint { return sketch.NewParallel(nil, nil) }, "ok", "parallel", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewPerpendicular", "", func() sketch.Constraint { return sketch.NewPerpendicular(nil, nil) }, "ok", "perpendicular", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewPointOnLine", "", func() sketch.Constraint { return sketch.NewPointOnLine(nil, nil) }, "ok", "point_on_line", "pts=[nil] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewCollinear", "", func() sketch.Constraint { return sketch.NewCollinear(nil, nil) }, "ok", "collinear", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewPointOnCircle", "", func() sketch.Constraint { return sketch.NewPointOnCircle(nil, nil) }, "ok", "point_on_circle", "pts=[nil] ents=[typed-nil(*sketch.Circle)]", "panic"},
	{"NewPointOnArc", "", func() sketch.Constraint { return sketch.NewPointOnArc(nil, nil) }, "ok", "point_on_arc", "pts=[nil] ents=[typed-nil(*sketch.Arc)]", "panic"},
	{"NewPointOnEllipticalArc", "", func() sketch.Constraint { return sketch.NewPointOnEllipticalArc(nil, nil) }, "ok", "point_on_elliptical_arc", "pts=[nil] ents=[typed-nil(*sketch.EllipticalArc)]", "panic"},
	{"NewPointOnSpline", "", func() sketch.Constraint { return sketch.NewPointOnSpline(nil, nil) }, "ok", "point_on_spline", "pts=[nil] ents=[typed-nil(*sketch.Spline)]", "len=0"},
	{"NewTangentToSpline", "", func() sketch.Constraint { return sketch.NewTangentToSpline(nil, nil) }, "ok", "tangent_spline", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Spline)]", "len=0"},
	{"NewPointOnClosedSpline", "", func() sketch.Constraint { return sketch.NewPointOnClosedSpline(nil, nil) }, "ok", "point_on_closed_spline", "pts=[nil] ents=[typed-nil(*sketch.ClosedSpline)]", "len=0"},
	{"NewPointOnFitSpline", "", func() sketch.Constraint { return sketch.NewPointOnFitSpline(nil, nil) }, "ok", "point_on_fit_spline", "pts=[nil] ents=[typed-nil(*sketch.FitSpline)]", "len=0"},
	{"NewTangentToClosedSpline", "", func() sketch.Constraint { return sketch.NewTangentToClosedSpline(nil, nil) }, "ok", "tangent_closed_spline", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.ClosedSpline)]", "len=0"},
	{"NewTangentToFitSpline", "", func() sketch.Constraint { return sketch.NewTangentToFitSpline(nil, nil) }, "ok", "tangent_fit_spline", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.FitSpline)]", "len=0"},
	{"NewPointOnConic", "", func() sketch.Constraint { return sketch.NewPointOnConic(nil, nil) }, "ok", "point_on_conic", "pts=[nil] ents=[typed-nil(*sketch.Conic)]", "len=0"},
	{"NewTangentToConic", "", func() sketch.Constraint { return sketch.NewTangentToConic(nil, nil) }, "ok", "tangent_conic", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Conic)]", "len=0"},
	{"NewPointOnNURBS", "", func() sketch.Constraint { return sketch.NewPointOnNURBS(nil, nil) }, "ok", "point_on_nurbs", "pts=[nil] ents=[typed-nil(*sketch.NURBS)]", "len=0"},
	{"NewTangentToNURBS", "", func() sketch.Constraint { return sketch.NewTangentToNURBS(nil, nil) }, "ok", "tangent_nurbs", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.NURBS)]", "len=0"},
	{"NewMidpoint", "", func() sketch.Constraint { return sketch.NewMidpoint(nil, nil) }, "ok", "midpoint", "pts=[nil] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewMidpointOf", "", func() sketch.Constraint { return sketch.NewMidpointOf(nil, nil, nil) }, "ok", "midpoint_of", "pts=[nil nil nil] ents=[]", "panic"},
	{"NewSymmetric", "", func() sketch.Constraint { return sketch.NewSymmetric(nil, nil, nil) }, "ok", "symmetric", "pts=[nil nil] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewSymmetricLines", "", func() sketch.Constraint { return sketch.NewSymmetricLines(nil, nil, nil) }, "ok", "symmetric_lines", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewSymmetricCircles", "", func() sketch.Constraint { return sketch.NewSymmetricCircles(nil, nil, nil) }, "ok", "symmetric_circles", "pts=[] ents=[typed-nil(*sketch.Circle) typed-nil(*sketch.Circle) typed-nil(*sketch.Line)]", "panic"},
	{"NewSymmetricArcs", "", func() sketch.Constraint { return sketch.NewSymmetricArcs(nil, nil, nil) }, "ok", "symmetric_arcs", "pts=[] ents=[typed-nil(*sketch.Arc) typed-nil(*sketch.Arc) typed-nil(*sketch.Line)]", "panic"},
	{"NewEqual", "", func() sketch.Constraint { return sketch.NewEqual(nil, nil) }, "ok", "equal_lines", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewPointOnEllipse", "", func() sketch.Constraint { return sketch.NewPointOnEllipse(nil, nil) }, "ok", "point_on_ellipse", "pts=[nil] ents=[typed-nil(*sketch.Ellipse)]", "panic"},
	{"NewDistance", "", func() sketch.Constraint { return sketch.NewDistance(nil, nil, 0) }, "ok", "distance", "pts=[nil nil] ents=[]", "panic"},
	{"NewHorizontalDistance", "", func() sketch.Constraint { return sketch.NewHorizontalDistance(nil, nil, 0) }, "ok", "hdistance", "pts=[nil nil] ents=[]", "panic"},
	{"NewVerticalDistance", "", func() sketch.Constraint { return sketch.NewVerticalDistance(nil, nil, 0) }, "ok", "vdistance", "pts=[nil nil] ents=[]", "panic"},
	{"NewDistancePointLine", "", func() sketch.Constraint { return sketch.NewDistancePointLine(nil, nil, 0) }, "ok", "distance_point_line", "pts=[nil] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewDistancePointCircle", "", func() sketch.Constraint { return sketch.NewDistancePointCircle(nil, nil, 0) }, "ok", "distance_point_circle", "pts=[nil] ents=[typed-nil(*sketch.Circle)]", "panic"},
	{"NewDistanceLineCircle", "", func() sketch.Constraint { return sketch.NewDistanceLineCircle(nil, nil, 0) }, "ok", "distance_line_circle", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Circle)]", "panic"},
	{"NewDistancePointArc", "", func() sketch.Constraint { return sketch.NewDistancePointArc(nil, nil, 0) }, "ok", "distance_point_arc", "pts=[nil] ents=[typed-nil(*sketch.Arc)]", "panic"},
	{"NewDistanceLineArc", "", func() sketch.Constraint { return sketch.NewDistanceLineArc(nil, nil, 0) }, "ok", "distance_line_arc", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Arc)]", "panic"},
	{"NewDistanceLines", "", func() sketch.Constraint { return sketch.NewDistanceLines(nil, nil, 0) }, "ok", "distance_lines", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewOffset", "", func() sketch.Constraint { return sketch.NewOffset(nil, nil, 0) }, "ok", "offset", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewArcLength", "", func() sketch.Constraint { return sketch.NewArcLength(nil, 1) }, "ok", "arc_length", "pts=[] ents=[typed-nil(*sketch.Arc)]", "panic"},
	{"NewEqualLineArc", "", func() sketch.Constraint { return sketch.NewEqualLineArc(nil, nil) }, "ok", "equal_line_arc", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Arc)]", "panic"},
	{"NewAngle", "", func() sketch.Constraint { return sketch.NewAngle(nil, nil, 0) }, "ok", "angle", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewConcentric", "typed", func() sketch.Constraint { return sketch.NewConcentric((*sketch.Circle)(nil), (*sketch.Circle)(nil)) }, "ok", "concentric", "pts=[] ents=[typed-nil(*sketch.Circle) typed-nil(*sketch.Circle)]", "panic"},
	{"NewConcentric", "untyped", func() sketch.Constraint { return sketch.NewConcentric(nil, nil) }, "ok", "concentric", "pts=[] ents=[untyped-nil untyped-nil]", "panic"},
	{"NewEqualRadius", "typed", func() sketch.Constraint { return sketch.NewEqualRadius((*sketch.Circle)(nil), (*sketch.Circle)(nil)) }, "ok", "equal_radii", "pts=[] ents=[typed-nil(*sketch.Circle) typed-nil(*sketch.Circle)]", "panic"},
	{"NewEqualRadius", "untyped", func() sketch.Constraint { return sketch.NewEqualRadius(nil, nil) }, "ok", "equal_radii", "pts=[] ents=[untyped-nil untyped-nil]", "panic"},
	{"NewTangent", "typed", func() sketch.Constraint { return sketch.NewTangent(nil, (*sketch.Circle)(nil)) }, "ok", "tangent_line_circle", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Circle)]", "panic"},
	{"NewTangent", "untyped", func() sketch.Constraint { return sketch.NewTangent(nil, nil) }, "ok", "tangent_line_circle", "pts=[] ents=[typed-nil(*sketch.Line) untyped-nil]", "panic"},
	{"NewTangentCircles", "typed", func() sketch.Constraint {
		return sketch.NewTangentCircles((*sketch.Circle)(nil), (*sketch.Circle)(nil), false)
	}, "ok", "tangent_circles", "pts=[] ents=[typed-nil(*sketch.Circle) typed-nil(*sketch.Circle)]", "panic"},
	{"NewTangentCircles", "untyped", func() sketch.Constraint { return sketch.NewTangentCircles(nil, nil, false) }, "ok", "tangent_circles", "pts=[] ents=[untyped-nil untyped-nil]", "panic"},
	{"NewTangentEllipse", "typed", func() sketch.Constraint { return sketch.NewTangentEllipse(nil, (*sketch.Ellipse)(nil)) }, "ok", "tangent_ellipse", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Ellipse)]", "panic"},
	{"NewTangentEllipse", "untyped", func() sketch.Constraint { return sketch.NewTangentEllipse(nil, nil) }, "ok", "tangent_ellipse", "pts=[] ents=[typed-nil(*sketch.Line) untyped-nil]", "panic"},
	{"NewTangentEllipseCircular", "typed", func() sketch.Constraint {
		return sketch.NewTangentEllipseCircular((*sketch.Ellipse)(nil), (*sketch.Circle)(nil), false)
	}, "ok", "tangent_ellipse_circle", "pts=[] ents=[typed-nil(*sketch.Ellipse) typed-nil(*sketch.Circle)]", "len=0"},
	{"NewTangentEllipseCircular", "untyped", func() sketch.Constraint {
		return sketch.NewTangentEllipseCircular(nil, nil, false)
	}, "panic", "n/a", "n/a", "n/a"},
	{"NewTangentEllipses", "typed", func() sketch.Constraint {
		return sketch.NewTangentEllipses((*sketch.Ellipse)(nil), (*sketch.Ellipse)(nil), false)
	}, "ok", "tangent_ellipses", "pts=[] ents=[typed-nil(*sketch.Ellipse) typed-nil(*sketch.Ellipse)]", "len=0"},
	{"NewTangentEllipses", "untyped", func() sketch.Constraint {
		return sketch.NewTangentEllipses(nil, nil, false)
	}, "panic", "n/a", "n/a", "n/a"},
	{"NewRadius", "typed", func() sketch.Constraint { return sketch.NewRadius((*sketch.Circle)(nil), 1) }, "ok", "radius", "pts=[] ents=[typed-nil(*sketch.Circle)]", "panic"},
	{"NewRadius", "untyped", func() sketch.Constraint { return sketch.NewRadius(nil, 1) }, "ok", "radius", "pts=[] ents=[untyped-nil]", "panic"},
	{"NewDiameter", "typed", func() sketch.Constraint { return sketch.NewDiameter((*sketch.Circle)(nil), 1) }, "ok", "diameter", "pts=[] ents=[typed-nil(*sketch.Circle)]", "panic"},
	{"NewDiameter", "untyped", func() sketch.Constraint { return sketch.NewDiameter(nil, 1) }, "ok", "diameter", "pts=[] ents=[untyped-nil]", "panic"},
	{"NewSemiMajor", "typed", func() sketch.Constraint { return sketch.NewSemiMajor((*sketch.Ellipse)(nil), 1) }, "ok", "semi_major", "pts=[] ents=[typed-nil(*sketch.Ellipse)]", "panic"},
	{"NewSemiMajor", "untyped", func() sketch.Constraint { return sketch.NewSemiMajor(nil, 1) }, "ok", "semi_major", "pts=[] ents=[untyped-nil]", "panic"},
	{"NewSemiMinor", "typed", func() sketch.Constraint { return sketch.NewSemiMinor((*sketch.Ellipse)(nil), 1) }, "ok", "semi_minor", "pts=[] ents=[typed-nil(*sketch.Ellipse)]", "panic"},
	{"NewSemiMinor", "untyped", func() sketch.Constraint { return sketch.NewSemiMinor(nil, 1) }, "ok", "semi_minor", "pts=[] ents=[untyped-nil]", "panic"},
	{"NewEllipseRotation", "typed", func() sketch.Constraint { return sketch.NewEllipseRotation((*sketch.Ellipse)(nil), 0) }, "ok", "ellipse_rotation", "pts=[] ents=[typed-nil(*sketch.Ellipse)]", "panic"},
	{"NewEllipseRotation", "untyped", func() sketch.Constraint { return sketch.NewEllipseRotation(nil, 0) }, "ok", "ellipse_rotation", "pts=[] ents=[untyped-nil]", "panic"},
}

// TestIntrospectionNilOperandOutcomes records what ConstraintKind,
// ConstraintRefs, ConstraintResiduals and IsInternal do for a constraint built
// from nil operands, one row per public constructor in constraint.go (and one
// row per nil SHAPE for a constructor taking a sealed-interface operand).
//
// It records rather than rules. The four functions do not share one nil-safety
// contract, the differences are per constraint TYPE, and none of them is
// visible in introspect.go — a constructor's residual method decides whether
// ConstraintResiduals panics or returns an empty slice, and its constraintRefs
// case decides what a nil operand comes back as. Nothing in the build, vet or
// lint gates notices when a new constructor lands on a different side of any of
// them, which is how the twelve empty-residual families went unnoticed through
// two rounds of review of introspect.go's own comments.
//
// The constructor list is anchored against constraint.go itself, so a
// constructor added later fails here until its outcomes are recorded, and a
// behaviour change to an existing one fails on its row.
func TestIntrospectionNilOperandOutcomes(t *testing.T) {
	declared := constraintConstructorsFromSource(t)

	want := make(map[string]struct{}, len(declared))
	for name, ctor := range declared {
		if ctor.interfaceOperand {
			want[name+"/typed"] = struct{}{}
			want[name+"/untyped"] = struct{}{}
			continue
		}
		want[name+"/"] = struct{}{}
	}

	got := make(map[string]struct{}, len(nilOperandOutcomes))
	for _, tc := range nilOperandOutcomes {
		key := tc.ctor + "/" + tc.variant
		_, dup := got[key]
		require.Falsef(t, dup, `nilOperandOutcomes lists %s twice`, key)
		got[key] = struct{}{}
	}

	require.Equal(t, sortedKeys(want), sortedKeys(got),
		`nilOperandOutcomes is out of step with constraint.go's New… constructors: record the missing constructor's measured outcomes here (a constructor taking a Circular/Elliptical operand needs both a "typed" and an "untyped" row, since the two nils behave differently)`)

	for _, tc := range nilOperandOutcomes {
		name := tc.ctor
		if tc.variant != "" {
			name += "/" + tc.variant
		}
		t.Run(name, func(t *testing.T) {
			c, construct := recoverConstraint(tc.build)
			require.Equal(t, tc.construct, construct, `the constructor's own behaviour on nil operands changed`)
			if construct != "ok" {
				require.Equal(t, "n/a", tc.kind, `a constructor that panics introspects nothing`)
				require.Equal(t, "n/a", tc.refs, `a constructor that panics introspects nothing`)
				require.Equal(t, "n/a", tc.residuals, `a constructor that panics introspects nothing`)
				return
			}

			require.Equal(t, tc.kind, recoverString(func() string { return sketch.ConstraintKind(c) }),
				`ConstraintKind`)
			require.Equal(t, tc.refs, recoverString(func() string { return describeConstraintRefs(c) }),
				`ConstraintRefs`)
			require.Equal(t, tc.residuals, recoverString(func() string {
				return fmt.Sprintf("len=%d", len(sketch.ConstraintResiduals(c)))
			}), `ConstraintResiduals`)

			// IsInternal is the one uniform column: it is a type assertion
			// against the constraint's method set, so it neither panics nor
			// reports true for anything a public constructor builds. A
			// constructor that ever returns an internal constraint fails here.
			require.Equal(t, "false", recoverString(func() string { return fmt.Sprint(sketch.IsInternal(c)) }),
				`IsInternal`)
		})
	}
}

// describeConstraintRefs renders ConstraintRefs's two slices element by
// element. A point element is "nil" or "live". An entity element distinguishes
// the three values a caller can actually receive: "untyped-nil" (the interface
// itself is nil, so `e == nil` is true), "typed-nil" (a non-nil interface
// holding a nil pointer, where `e == nil` is FALSE and any method call panics),
// and "live".
func describeConstraintRefs(c sketch.Constraint) string {
	pts, ents := sketch.ConstraintRefs(c)

	var b strings.Builder
	b.WriteString("pts=[")
	for i, p := range pts {
		if i > 0 {
			b.WriteString(" ")
		}
		if p == nil {
			b.WriteString("nil")
			continue
		}
		b.WriteString("live")
	}
	b.WriteString("] ents=[")
	for i, e := range ents {
		if i > 0 {
			b.WriteString(" ")
		}
		switch {
		case e == nil:
			b.WriteString("untyped-nil")
		case reflect.ValueOf(e).IsNil():
			fmt.Fprintf(&b, "typed-nil(%T)", e)
		default:
			fmt.Fprintf(&b, "live(%T)", e)
		}
	}
	b.WriteString("]")
	return b.String()
}

// recoverConstraint calls f and reports the constraint it built together with
// "ok", or a zero constraint together with "panic".
func recoverConstraint(f func() sketch.Constraint) (sketch.Constraint, string) {
	var c sketch.Constraint
	status := "ok"
	func() {
		defer func() {
			if r := recover(); r != nil {
				status = "panic"
			}
		}()
		c = f()
	}()
	return c, status
}

// recoverString calls f and reports what it returned, or "panic".
func recoverString(f func() string) string {
	var out string
	func() {
		defer func() {
			if r := recover(); r != nil {
				out = "panic"
			}
		}()
		out = f()
	}()
	return out
}

// constraintConstructor is one public New… constructor as constraint.go
// declares it.
type constraintConstructor struct {
	// interfaceOperand reports whether any parameter is a bare (non-builtin)
	// type name — the sealed Circular/Elliptical operands. Such a parameter
	// accepts two different nils, so the contract has to be recorded for both.
	interfaceOperand bool
	// handle is the exported concrete type the constructor returns (e.g.
	// "Distance"), or "" when it returns the Constraint interface.
	handle string
}

// builtinTypeNames are the predeclared type names a constraint constructor's
// scalar parameters use. A parameter naming anything else without a * is an
// interface-typed operand.
var builtinTypeNames = map[string]struct{}{
	"bool": {}, "byte": {}, "complex64": {}, "complex128": {}, "float32": {},
	"float64": {}, "int": {}, "int8": {}, "int16": {}, "int32": {}, "int64": {},
	"rune": {}, "string": {}, "uint": {}, "uint8": {}, "uint16": {}, "uint32": {},
	"uint64": {}, "uintptr": {},
}

// constraintConstructorsFromSource parses constraint.go and returns its public
// New… constructors keyed by name. Deriving the list from the source rather
// than trusting a hand-kept one is what makes the nil-operand contract
// mechanical: a constructor added later cannot escape the table above by nobody
// remembering to add it.
func constraintConstructorsFromSource(t *testing.T) map[string]constraintConstructor {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "constraint.go", nil, 0)
	require.NoError(t, err, `failed to parse constraint.go`)

	ctors := make(map[string]constraintConstructor)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "New") || !fn.Name.IsExported() {
			continue
		}

		ctor := constraintConstructor{}
		for _, param := range fn.Type.Params.List {
			id, ok := param.Type.(*ast.Ident)
			if !ok {
				continue
			}
			if _, builtin := builtinTypeNames[id.Name]; !builtin {
				ctor.interfaceOperand = true
			}
		}
		if fn.Type.Results != nil && len(fn.Type.Results.List) == 1 {
			if star, ok := fn.Type.Results.List[0].Type.(*ast.StarExpr); ok {
				if id, ok := star.X.(*ast.Ident); ok {
					ctor.handle = id.Name
				}
			}
		}
		ctors[fn.Name.Name] = ctor
	}
	require.NotEmpty(t, ctors, `constraint.go declares no New… constructors — the parse is wrong`)
	return ctors
}
