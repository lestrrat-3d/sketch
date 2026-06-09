package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func mustSolve(t *testing.T, s *sketch.Sketch) *sketch.Result {
	t.Helper()
	res, err := s.Solve()
	require.NoErrorf(t, err, "solve failed (residual=%.3e)", res.Residual)
	return res
}

// Test helpers that construct generic geometry and immediately commit it,
// keeping the test bodies terse. They reach the generic point behind a
// solver-bound one via [sketch.Point.Generic].
func addPt(s *sketch.Sketch, x, y float64) *sketch.Point {
	return s.AddPoint(geom.NewPoint(x, y))
}

func addLn(s *sketch.Sketch, a, b *sketch.Point) *sketch.Line {
	return s.AddLine(geom.NewLine(a.Generic(), b.Generic()))
}

func addCir(s *sketch.Sketch, c *sketch.Point, r float64) *sketch.Circle {
	return s.AddCircle(geom.NewCircle(c.Generic(), r))
}

func addArc(s *sketch.Sketch, c, a, b *sketch.Point) *sketch.Arc {
	return s.AddArc(geom.NewArc(c.Generic(), a.Generic(), b.Generic()))
}

func addDist(s *sketch.Sketch, a, b *sketch.Point, d float64) *sketch.Distance {
	c := sketch.NewDistance(a, b, d)
	s.AddConstraint(c)
	return c
}

// lineDir returns the line's direction vector via exported accessors.
func lineDir(l *sketch.Line) (float64, float64) {
	return l.End.X() - l.Start.X(), l.End.Y() - l.Start.Y()
}

// pointDist returns the distance between two points via exported accessors.
func pointDist(a, b *sketch.Point) float64 {
	return math.Hypot(a.X()-b.X(), a.Y()-b.Y())
}

// A fully constrained rectangle: grounded origin, horizontal/vertical sides,
// width and height dimensions.
func newRectangle(t *testing.T) (*sketch.Sketch, *sketch.Distance, *sketch.Point, *sketch.Point, *sketch.Point) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 18, 2) // deliberately rough guesses
	c := addPt(s, 17, 11)
	d := addPt(s, 1, 13)

	ab := addLn(s, a, b)
	bc := addLn(s, b, c)
	dc := addLn(s, d, c)
	ad := addLn(s, a, d)

	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	w := addDist(s, a, b, 20)
	addDist(s, a, d, 12)
	return s, w, b, c, d
}

func TestRectangleSolves(t *testing.T) {
	s, _, b, c, d := newRectangle(t)
	res := mustSolve(t, s)

	require.Equal(t, 0, res.DOF, "DOF (fully constrained)")
	require.InDelta(t, 20, b.X(), 1e-6, "b.X")
	require.InDelta(t, 0, b.Y(), 1e-6, "b.Y")
	require.InDelta(t, 20, c.X(), 1e-6, "c.X")
	require.InDelta(t, 12, c.Y(), 1e-6, "c.Y")
	require.InDelta(t, 0, d.X(), 1e-6, "d.X")
	require.InDelta(t, 12, d.Y(), 1e-6, "d.Y")
}

func TestParametricUpdate(t *testing.T) {
	s, w, b, c, _ := newRectangle(t)
	mustSolve(t, s)

	w.Set(35) // change the driving width dimension
	mustSolve(t, s)
	require.InDelta(t, 35, b.X(), 1e-6, "b.X after edit")
	require.InDelta(t, 35, c.X(), 1e-6, "c.X after edit")
	require.InDelta(t, 12, c.Y(), 1e-6, "c.Y after edit (height unchanged)")
}

func TestGenericGeometryReuse(t *testing.T) {
	// One generic line, inspectable on its own.
	ga := geom.NewPoint(0, 0)
	gb := geom.NewPoint(40, 0)
	gl := geom.NewLine(ga, gb)
	require.InDelta(t, 40, gl.Length(), 1e-6, "generic length")

	// Commit it into two independent sketches with different widths.
	s1 := sketch.New()
	l1 := s1.AddLine(gl)
	l1.Start.MoveTo(0, 0)
	s1.Fix(l1.Start)
	s1.AddConstraint(sketch.NewHorizontal(l1))
	addDist(s1, l1.Start, l1.End, 25)
	mustSolve(t, s1)
	require.InDelta(t, 25, l1.End.X(), 1e-6, "s1 width")

	s2 := sketch.New()
	l2 := s2.AddLine(gl) // same generic geometry, fresh solver state
	l2.Start.MoveTo(0, 0)
	s2.Fix(l2.Start)
	s2.AddConstraint(sketch.NewHorizontal(l2))
	addDist(s2, l2.Start, l2.End, 100)
	mustSolve(t, s2)

	require.InDelta(t, 100, l2.End.X(), 1e-6, "s2 width")
	require.InDelta(t, 25, l1.End.X(), 1e-6, "s1 unaffected") // independent
	require.InDelta(t, 40, gb.X, 1e-6, "generic template unchanged")
}

func TestAddIdempotent(t *testing.T) {
	s := sketch.New()
	g := geom.NewPoint(1, 2)
	s.AddPoint(g)
	s.AddPoint(g) // same generic point -> same bound point
	require.Len(t, s.Points(), 1, "idempotent add")
}

func TestTangentLineCircle(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	b.MoveTo(10, 0)
	s.Fix(b)
	line := addLn(s, a, b)

	center := addPt(s, 5, 5)
	s.Fix(center)
	circ := addCir(s, center, 2) // bad initial radius
	s.AddConstraint(sketch.NewTangent(line, circ))

	mustSolve(t, s)
	require.InDelta(t, 5, circ.R(), 1e-6, "tangent radius")
}

func TestPerpendicular(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 1)
	c := addPt(s, 1, 5)
	a.MoveTo(0, 0)
	s.Fix(a)
	l1 := addLn(s, a, b)
	l2 := addLn(s, a, c)
	s.AddConstraint(sketch.NewHorizontal(l1), sketch.NewPerpendicular(l1, l2))
	addDist(s, a, b, 10)
	addDist(s, a, c, 5)

	mustSolve(t, s)
	d1x, d1y := lineDir(l1)
	d2x, d2y := lineDir(l2)
	require.InDelta(t, 0, d1x*d2x+d1y*d2y, 1e-6, "perp dot")
	require.InDelta(t, 5, pointDist(a, c), 1e-6, "ac length")
}

func TestAngleConstraint(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	c := addPt(s, 5, 5)
	a.MoveTo(0, 0)
	s.Fix(a)
	l1 := addLn(s, a, b)
	l2 := addLn(s, a, c)
	s.AddConstraint(sketch.NewHorizontal(l1))
	addDist(s, a, b, 10)
	addDist(s, a, c, 8)
	s.AddConstraint(sketch.NewAngle(l1, l2, 45)) // degrees (the sketch's default angle unit)

	mustSolve(t, s)
	d1x, d1y := lineDir(l1)
	d2x, d2y := lineDir(l2)
	ang := math.Atan2(d1x*d2y-d1y*d2x, d1x*d2x+d1y*d2y)
	require.InDelta(t, math.Pi/4, ang, 1e-6, "angle")
}

func TestArcRadiusConsistency(t *testing.T) {
	s := sketch.New()
	center := addPt(s, 0, 0)
	start := addPt(s, 5, 0)
	end := addPt(s, 1, 9) // off the radius-5 circle
	center.MoveTo(0, 0)
	s.Fix(center)
	s.Fix(start)
	arc := addArc(s, center, start, end)

	mustSolve(t, s)
	require.InDelta(t, 5, math.Hypot(end.X(), end.Y()), 1e-6, "arc radius via end")
	require.InDelta(t, 5, arc.R(), 1e-6, "arc R()")
}

func TestConcentricEqualRadius(t *testing.T) {
	s := sketch.New()
	o1 := addPt(s, 0, 0)
	o2 := addPt(s, 3, 2)
	o1.MoveTo(0, 0)
	s.Fix(o1)
	c1 := addCir(s, o1, 5)
	c2 := addCir(s, o2, 9)
	s.AddConstraint(sketch.NewConcentric(c1, c2), sketch.NewEqualRadius(c1, c2), sketch.NewRadius(c1, 7))

	mustSolve(t, s)
	require.InDelta(t, 0, o2.X(), 1e-6, "c2 center x")
	require.InDelta(t, 0, o2.Y(), 1e-6, "c2 center y")
	require.InDelta(t, 7, c1.R(), 1e-6, "c1 radius")
	require.InDelta(t, 7, c2.R(), 1e-6, "c2 radius")
}

func TestSymmetric(t *testing.T) {
	s := sketch.New()
	// vertical axis along x = 0
	axA := addPt(s, 0, 0)
	axB := addPt(s, 0, 10)
	axA.MoveTo(0, 0)
	s.Fix(axA)
	axB.MoveTo(0, 10)
	s.Fix(axB)
	axis := addLn(s, axA, axB)

	p1 := addPt(s, -3, 4)
	p2 := addPt(s, 5, 1)
	s.Fix(p1)
	s.AddConstraint(sketch.NewSymmetric(p1, p2, axis))

	mustSolve(t, s)
	require.InDelta(t, 3, p2.X(), 1e-6, "mirror x")
	require.InDelta(t, 4, p2.Y(), 1e-6, "mirror y")
}

func TestUnderConstrainedDOF(t *testing.T) {
	s := sketch.New()
	addPt(s, 0, 0) // single free point, nothing else
	require.Equal(t, 2, s.DOF(), "DOF")
}

func TestRedundantConstraint(t *testing.T) {
	s, _, b, _, _ := newRectangle(t)
	// Add a redundant duplicate width dimension.
	a := s.Points()[0]
	addDist(s, a, b, 20)
	res := mustSolve(t, s)
	require.NotZero(t, res.Redundant, "expected at least one redundant equation")
}

func TestJSONRoundTrip(t *testing.T) {
	s, _, b, c, d := newRectangle(t)
	mustSolve(t, s)

	data, err := json.MarshalIndent(s, "", "  ")
	require.NoError(t, err, "marshal")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	require.Len(t, s2.Points(), len(s.Points()), "points")

	res := mustSolve(t, &s2)
	require.Equal(t, 0, res.DOF, "reloaded DOF")
	require.InDelta(t, 20, s2.Points()[b.ID()].X(), 1e-6, "reloaded b.X")
	require.InDelta(t, 12, s2.Points()[c.ID()].Y(), 1e-6, "reloaded c.Y")
	require.InDelta(t, 0, s2.Points()[d.ID()].X(), 1e-6, "reloaded d.X")
}

func TestSVGOutput(t *testing.T) {
	s, _, _, _, _ := newRectangle(t)
	mustSolve(t, s)
	o := addPt(s, 10, 6)
	addCir(s, o, 3)

	svg, err := s.SVG()
	require.NoError(t, err)
	for _, want := range []string{"<svg", "<line", "<circle", "</svg>"} {
		require.Containsf(t, svg, want, "SVG missing %q", want)
	}
}

func TestDXFOutput(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	addLn(s, a, b)
	o := addPt(s, 5, 5)
	addCir(s, o, 3)
	st := addPt(s, 8, 5)
	en := addPt(s, 5, 8)
	addArc(s, o, st, en)

	dxf, err := s.DXF()
	require.NoError(t, err)
	for _, want := range []string{"SECTION", "ENTITIES", "LINE", "CIRCLE", "ARC", "EOF"} {
		require.Containsf(t, dxf, want, "DXF missing %q", want)
	}
}
