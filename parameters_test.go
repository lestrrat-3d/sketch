package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/stretchr/testify/require"
)

// parametricRect builds a rectangle whose width and height are driven by
// parameters "w" and "h".
func parametricRect(t *testing.T) (*sketch.Sketch, *sketch.Point, *sketch.Point, *sketch.Point) {
	t.Helper()
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 5, 1)
	c := addPt(s, 4, 6)
	d := addPt(s, 1, 5)
	ab := addLn(s, a, b)
	bc := addLn(s, b, c)
	dc := addLn(s, d, c)
	ad := addLn(s, a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))

	p := param.New()
	require.NoError(t, p.Set("w", "20"))
	require.NoError(t, p.Set("h", "w / 2")) // height depends on width
	wDim := addDist(s, a, b, 0)
	hDim := addDist(s, a, d, 0)
	require.NoError(t, s.Bind(wDim, p, "w"))
	require.NoError(t, s.Bind(hDim, p, "h"))
	return s, b, c, d
}

func TestBoundDimensionsSolve(t *testing.T) {
	s, b, c, d := parametricRect(t)
	res := mustSolve(t, s)
	require.Equal(t, 0, res.DOF, "DOF")
	require.InDelta(t, 20, b.X(), 1e-6, "b.X")
	require.InDelta(t, 10, d.Y(), 1e-6, "d.Y")
	require.InDelta(t, 20, c.X(), 1e-6, "c.X")
	require.InDelta(t, 10, c.Y(), 1e-6, "c.Y")
}

func TestParameterEditPropagates(t *testing.T) {
	s, b, _, d := parametricRect(t)
	mustSolve(t, s)

	// Change the width parameter; height ("w/2") follows automatically.
	require.NoError(t, s.Params().Set("w", "30"))
	mustSolve(t, s)
	require.InDelta(t, 30, b.X(), 1e-6, "b.X after edit")
	require.InDelta(t, 15, d.Y(), 1e-6, "d.Y after edit")
}

func TestManualSetUnbinds(t *testing.T) {
	s, b, _, _ := parametricRect(t)
	mustSolve(t, s)

	// Find the width dimension and override it literally.
	var wDim *sketch.Distance
	for _, c := range s.Constraints() {
		if dd, ok := c.(*sketch.Distance); ok && sketch.DriverExpr(dd) == "w" {
			wDim = dd
		}
	}
	require.NotNil(t, wDim, "could not find bound width dimension")
	wDim.Set(42)
	require.Empty(t, sketch.DriverExpr(wDim), "Set should clear the binding")
	mustSolve(t, s)
	require.InDelta(t, 42, b.X(), 1e-6, "b.X after manual set")

	// Changing the parameter no longer affects the now-literal dimension.
	require.NoError(t, s.Params().Set("w", "99"))
	mustSolve(t, s)
	require.InDelta(t, 42, b.X(), 1e-6, "b.X still literal")
}

func TestBindExpressionInline(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 3, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(addLn(s, a, b)))
	p := param.New()
	require.NoError(t, p.Set("gap", "8"))
	dim := addDist(s, a, b, 0)
	// Expression combining a parameter, a function and a constant.
	require.NoError(t, s.Bind(dim, p, "gap * 2 + sqrt(16)"))
	mustSolve(t, s)
	require.InDelta(t, 20, b.X(), 1e-6, "b.X") // 8*2 + 4
}

func TestBindSyntaxError(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 1, 0)
	dim := sketch.NewDistance(a, b, 1)
	require.Error(t, s.Bind(dim, param.New(), "1 +"), "expected syntax error from Bind")
}

func TestBindNilTable(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 1, 0)
	dim := sketch.NewDistance(a, b, 1)
	require.Error(t, s.Bind(dim, nil, "1"), "expected error binding with a nil table")
}

func TestBindTableMismatch(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 1, 0)
	c := addPt(s, 0, 1)
	p1 := param.New()
	require.NoError(t, p1.Set("x", "1"))
	p2 := param.New()
	require.NoError(t, p2.Set("x", "1"))
	require.NoError(t, s.Bind(sketch.NewDistance(a, b, 0), p1, "x"))
	err := s.Bind(sketch.NewDistance(a, c, 0), p2, "x") // different table
	require.ErrorIs(t, err, sketch.ErrTableMismatch)
}

func TestUndefinedParameterFailsSolve(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 1, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(addLn(s, a, b)))
	dim := addDist(s, a, b, 1)
	require.NoError(t, s.Bind(dim, param.New(), "nope * 2"))
	_, err := s.Solve()
	require.Error(t, err, "expected solve to fail on undefined parameter")
}

// findBoundDistance returns the sketch's distance dimension bound to expr.
func findBoundDistance(t *testing.T, s *sketch.Sketch, expr string) *sketch.Distance {
	t.Helper()
	for _, c := range s.Constraints() {
		if dd, ok := c.(*sketch.Distance); ok && sketch.DriverExpr(dd) == expr {
			return dd
		}
	}
	t.Fatalf("no distance dimension bound to %q", expr)
	return nil
}

func TestUnbind(t *testing.T) {
	s, b, _, d := parametricRect(t)
	mustSolve(t, s)

	wDim := findBoundDistance(t, s, "w")
	s.Unbind(wDim)
	require.Empty(t, sketch.DriverExpr(wDim), "binding cleared")

	// The last applied value (20) stays in place as a literal; parameter
	// edits no longer reach the unbound dimension (height, still bound to
	// "w/2", does follow).
	require.NoError(t, s.Params().Set("w", "70"))
	mustSolve(t, s)
	require.InDelta(t, 20, b.X(), 1e-6, "unbound width keeps its literal value")
	require.InDelta(t, 35, d.Y(), 1e-6, "bound height still follows the parameter")
}

func TestApplyParameters(t *testing.T) {
	s, b, _, _ := parametricRect(t)
	wDim := findBoundDistance(t, s, "w")
	require.InDelta(t, 0, wDim.Target().Mag(), 1e-12, "bound value not applied before ApplyParameters")

	// The public entry point applies bound values without solving: the
	// dimension target updates, the geometry stays put.
	require.NoError(t, s.ApplyParameters())
	require.InDelta(t, 20, wDim.Target().Mag(), 1e-6, "bound value applied")
	require.InDelta(t, 5, b.X(), 1e-12, "geometry untouched without a solve")
}

func TestDeleteParameterInUse(t *testing.T) {
	s, _, _, _ := parametricRect(t)
	mustSolve(t, s)

	// Deleting a parameter a bound dimension references leaves the binding
	// dangling; the next solve must fail cleanly, naming the parameter.
	s.Params().Delete("w")
	_, err := s.Solve()
	require.Error(t, err, "solve must fail once the parameter is gone")
	require.Contains(t, err.Error(), "w", "error names the missing parameter")
}

func TestJSONRoundTripWithParameters(t *testing.T) {
	s, _, _, _ := parametricRect(t)
	mustSolve(t, s)

	data, err := json.MarshalIndent(s, "", "  ")
	require.NoError(t, err)
	require.Contains(t, string(data), `"parameters"`, "serialized sketch should include parameters")
	require.Contains(t, string(data), `"expr"`, "serialized dimension should include its bound expression")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	// The reloaded sketch must still be parametric: edit and re-solve.
	require.NoError(t, s2.Params().Set("w", "50"))
	mustSolve(t, &s2)
	require.InDelta(t, 50, s2.Points()[1].X(), 1e-6, "reloaded b.X")
	require.InDelta(t, 25, s2.Points()[3].Y(), 1e-6, "reloaded d.Y")
}
