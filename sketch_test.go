package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestRectangleSolves(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2) // deliberately rough guesses
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)

	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)

	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))

	res, err := s.Solve()
	require.NoError(t, err)

	require.Equal(t, 0, res.DOF, "DOF (fully constrained)")
	require.InDelta(t, 20, b.X(), 1e-6, "b.X")
	require.InDelta(t, 0, b.Y(), 1e-6, "b.Y")
	require.InDelta(t, 20, c.X(), 1e-6, "c.X")
	require.InDelta(t, 12, c.Y(), 1e-6, "c.Y")
	require.InDelta(t, 0, d.X(), 1e-6, "d.X")
	require.InDelta(t, 12, d.Y(), 1e-6, "d.Y")
}

func TestParametricUpdate(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2) // deliberately rough guesses
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)

	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)

	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	w := sketch.NewDistance(a, b, 20)
	s.AddConstraint(w)
	s.AddConstraint(sketch.NewDistance(a, d, 12))

	_, err := s.Solve()
	require.NoError(t, err)

	w.Set(35) // change the driving width dimension
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 35, b.X(), 1e-6, "b.X after edit")
	require.InDelta(t, 35, c.X(), 1e-6, "c.X after edit")
	require.InDelta(t, 12, c.Y(), 1e-6, "c.Y after edit (height unchanged)")
}

func TestSharedPointTopology(t *testing.T) {
	// Topology is expressed by sharing a *Point between entities: two lines
	// that share an endpoint move together at that vertex.
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	corner := s.AddPoint(10, 0)
	b := s.AddPoint(10, 10)
	l1 := s.AddLine(a, corner)
	l2 := s.AddLine(corner, b)

	require.Same(t, l1.End, l2.Start, "the shared corner is one point")

	// Moving the corner via a solve moves both lines' shared endpoint.
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewHorizontal(l1), sketch.NewVertical(l2))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 10, l1.End.X(), 1e-6)
	require.InDelta(t, 0, l1.End.Y(), 1e-6)
	require.Same(t, l1.End, l2.Start, "still shared after solving")
}

func TestGeometrySnapshot(t *testing.T) {
	// Geometry() returns a fresh geom snapshot at the CURRENT solved coords,
	// independent of the entity afterwards.
	s := newSketch(t)
	l := s.AddLine(s.AddPoint(1, 2), s.AddPoint(4, 6))
	g := l.Geometry()
	require.InDelta(t, 1, g.Start.X, 1e-9)
	require.InDelta(t, 6, g.End.Y, 1e-9)
	require.InDelta(t, 5, g.Length(), 1e-9)
}

func TestTangentLineCircle(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	b.MoveTo(10, 0)
	s.Fix(b)
	line := s.AddLine(a, b)

	center := s.AddPoint(5, 5)
	s.Fix(center)
	circ := s.AddCircle(center, 2) // bad initial radius
	s.AddConstraint(sketch.NewTangent(line, circ))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, circ.R(), 1e-6, "tangent radius")
}

func TestPerpendicular(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 1)
	c := s.AddPoint(1, 5)
	a.MoveTo(0, 0)
	s.Fix(a)
	l1 := s.AddLine(a, b)
	l2 := s.AddLine(a, c)
	s.AddConstraint(sketch.NewHorizontal(l1), sketch.NewPerpendicular(l1, l2))
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.AddConstraint(sketch.NewDistance(a, c, 5))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, math.Pi/2, math.Abs(l1.AngleTo(l2)), 1e-6, "perpendicular")
	require.InDelta(t, 5, a.DistanceTo(c), 1e-6, "ac length")
}

func TestAngleConstraint(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	c := s.AddPoint(5, 5)
	a.MoveTo(0, 0)
	s.Fix(a)
	l1 := s.AddLine(a, b)
	l2 := s.AddLine(a, c)
	s.AddConstraint(sketch.NewHorizontal(l1))
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.AddConstraint(sketch.NewDistance(a, c, 8))
	s.AddConstraint(sketch.NewAngle(l1, l2, 45)) // degrees (the sketch's default angle unit)

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, math.Pi/4, l1.AngleTo(l2), 1e-6, "angle")
}

func TestArcRadiusConsistency(t *testing.T) {
	s := newSketch(t)
	center := s.AddPoint(0, 0)
	start := s.AddPoint(5, 0)
	end := s.AddPoint(1, 9) // off the radius-5 circle
	center.MoveTo(0, 0)
	s.Fix(center)
	s.Fix(start)
	arc := s.AddArc(center, start, end)

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, end.DistanceTo(center), 1e-6, "arc radius via end")
	require.InDelta(t, 5, arc.R(), 1e-6, "arc R()")
}

func TestConcentricEqualRadius(t *testing.T) {
	s := newSketch(t)
	o1 := s.AddPoint(0, 0)
	o2 := s.AddPoint(3, 2)
	o1.MoveTo(0, 0)
	s.Fix(o1)
	c1 := s.AddCircle(o1, 5)
	c2 := s.AddCircle(o2, 9)
	s.AddConstraint(sketch.NewConcentric(c1, c2), sketch.NewEqualRadius(c1, c2), sketch.NewRadius(c1, 7))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, o2.X(), 1e-6, "c2 center x")
	require.InDelta(t, 0, o2.Y(), 1e-6, "c2 center y")
	require.InDelta(t, 7, c1.R(), 1e-6, "c1 radius")
	require.InDelta(t, 7, c2.R(), 1e-6, "c2 radius")
}

func TestTangentLineArc(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	line := s.AddLine(a, b)

	center := s.AddPoint(5, 5)
	s.Fix(center)
	// The arc spans the bottom of the circle so the downward tangent contact
	// lies within its sweep; seeded at the wrong radius (~4.24).
	start := s.AddPoint(2, 2) // ~225°
	end := s.AddPoint(8, 2)   // ~315°
	arc := s.AddArc(center, start, end)
	s.AddConstraint(sketch.NewTangent(line, arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, arc.R(), 1e-6, "arc radius reaches line")
	require.InDelta(t, start.DistanceTo(center), end.DistanceTo(center), 1e-6, "radius consistency held")
}

func TestTangentCircleArc(t *testing.T) {
	t.Run("external", func(t *testing.T) {
		s := newSketch(t)
		o := s.AddPoint(0, 0)
		s.Fix(o)
		circ := s.AddCircle(o, 3)
		s.AddConstraint(sketch.NewRadius(circ, 3))

		center := s.AddPoint(10, 0)
		s.Fix(center)
		start := s.AddPoint(12, 0)
		end := s.AddPoint(10, 2)
		arc := s.AddArc(center, start, end)
		s.AddConstraint(sketch.NewTangentCircles(circ, arc, false))

		_, err := s.Solve()
		require.NoError(t, err)
		require.InDelta(t, 7, arc.R(), 1e-6, "external: d = r1 + r2")
	})
	t.Run("internal", func(t *testing.T) {
		s := newSketch(t)
		o := s.AddPoint(0, 0)
		s.Fix(o)
		circ := s.AddCircle(o, 10)
		s.AddConstraint(sketch.NewRadius(circ, 10))

		center := s.AddPoint(4, 0)
		s.Fix(center)
		start := s.AddPoint(6, 0)
		end := s.AddPoint(4, 2)
		arc := s.AddArc(center, start, end)
		s.AddConstraint(sketch.NewTangentCircles(circ, arc, true))

		_, err := s.Solve()
		require.NoError(t, err)
		require.InDelta(t, 6, arc.R(), 1e-6, "internal: d = |r1 - r2|")
	})
}

func TestTangentArcArc(t *testing.T) {
	s := newSketch(t)
	c1 := s.AddPoint(0, 0)
	s.Fix(c1)
	s1 := s.AddPoint(3, 0)
	s.Fix(s1) // pins the first arc's radius at 3
	e1 := s.AddPoint(0, 3)
	a1 := s.AddArc(c1, s1, e1)

	c2 := s.AddPoint(10, 0)
	s.Fix(c2)
	s2 := s.AddPoint(12, 0)
	e2 := s.AddPoint(10, 2)
	a2 := s.AddArc(c2, s2, e2)
	s.AddConstraint(sketch.NewTangentCircles(a1, a2, false))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 3, a1.R(), 1e-6, "pinned arc radius")
	require.InDelta(t, 7, a2.R(), 1e-6, "external arc-arc tangency")
}

func TestEqualRadiusCircleArc(t *testing.T) {
	s := newSketch(t)
	o := s.AddPoint(0, 0)
	s.Fix(o)
	circ := s.AddCircle(o, 7)
	s.AddConstraint(sketch.NewRadius(circ, 7))

	center := s.AddPoint(20, 0)
	s.Fix(center)
	start := s.AddPoint(22, 0)
	end := s.AddPoint(20, 2)
	arc := s.AddArc(center, start, end)
	s.AddConstraint(sketch.NewEqualRadius(circ, arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 7, arc.R(), 1e-6, "arc matches circle radius")
}

func TestEqualRadiusArcArc(t *testing.T) {
	s := newSketch(t)
	c1 := s.AddPoint(0, 0)
	s.Fix(c1)
	s1 := s.AddPoint(5, 0)
	s.Fix(s1) // pins the first arc's radius at 5
	e1 := s.AddPoint(0, 5)
	a1 := s.AddArc(c1, s1, e1)

	c2 := s.AddPoint(20, 0)
	s.Fix(c2)
	s2 := s.AddPoint(22, 0)
	e2 := s.AddPoint(20, 2)
	a2 := s.AddArc(c2, s2, e2)
	s.AddConstraint(sketch.NewEqualRadius(a1, a2))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, a2.R(), 1e-6, "arc radii equal")
}

func TestDistancePointLine(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	line := s.AddLine(a, b)

	p := s.AddPoint(3, 2)
	s.AddConstraint(sketch.NewDistancePointLine(p, line, 5))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, p.Y(), 1e-6, "perpendicular distance (stays on starting side)")
}

func TestDistanceLines(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	l1 := s.AddLine(a, b)

	c := s.AddPoint(0, 3)
	d := s.AddPoint(10, 4) // deliberately not parallel
	l2 := s.AddLine(c, d)
	s.AddConstraint(sketch.NewDistanceLines(l1, l2, 6))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 6, c.Y(), 1e-6, "l2 start offset")
	require.InDelta(t, 6, d.Y(), 1e-6, "l2 end offset")
	d1x, d1y := l1.End.X()-l1.Start.X(), l1.End.Y()-l1.Start.Y()
	d2x, d2y := l2.End.X()-l2.Start.X(), l2.End.Y()-l2.Start.Y()
	require.InDelta(t, 0, d1x*d2y-d1y*d2x, 1e-6, "lines forced parallel")
}

func TestJSONRoundTripDistanceDims(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	l1 := s.AddLine(a, b)
	p := s.AddPoint(3, 2)
	c := s.AddPoint(0, 3)
	d := s.AddPoint(10, 4)
	l2 := s.AddLine(c, d)
	s.AddConstraint(sketch.NewDistancePointLine(p, l1, 5), sketch.NewDistanceLines(l1, l2, 6))

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")

	_, err = s2.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, s2.Points()[p.ID()].Y(), 1e-6, "reloaded point-line distance")
	require.InDelta(t, 6, s2.Points()[c.ID()].Y(), 1e-6, "reloaded line-line start offset")
	require.InDelta(t, 6, s2.Points()[d.ID()].Y(), 1e-6, "reloaded line-line end offset")
}

func TestDrivenDimension(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))

	diag := sketch.NewDistance(a, c, 0) // target irrelevant while driven
	diag.SetDriven(true)
	s.AddConstraint(diag)

	res, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "driven dim adds no equation")
	require.Zero(t, res.Redundant, "driven dim is not redundant")
	require.InDelta(t, math.Sqrt(544), diag.Target().Mag(), 1e-6, "measures the 20x12 diagonal")

	// Switching back to driving keeps the measured value as the target; it now
	// duplicates the rectangle's width/height dimensions, so the solve still
	// converges but reports the redundancy.
	diag.SetDriven(false)
	res, err = s.Solve()
	require.NoError(t, err)
	require.NotZero(t, res.Redundant, "driving duplicate is redundant")
}

func TestJSONRoundTripDrivenDimension(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))

	diag := sketch.NewDistance(a, c, 0)
	diag.SetDriven(true)
	s.AddConstraint(diag)
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")

	res, err := s2.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "reloaded DOF")
	cons := s2.Constraints()
	d2, ok := cons[len(cons)-1].(*sketch.Distance)
	require.True(t, ok, "last constraint is the distance dimension")
	require.True(t, d2.Driven(), "driven flag survives round-trip")
	require.InDelta(t, math.Sqrt(544), d2.Target().Mag(), 1e-6, "measured value after reload")
}

func TestSymmetric(t *testing.T) {
	s := newSketch(t)
	// vertical axis along x = 0
	axA := s.AddPoint(0, 0)
	axB := s.AddPoint(0, 10)
	axA.MoveTo(0, 0)
	s.Fix(axA)
	axB.MoveTo(0, 10)
	s.Fix(axB)
	axis := s.AddLine(axA, axB)

	p1 := s.AddPoint(-3, 4)
	p2 := s.AddPoint(5, 1)
	s.Fix(p1)
	s.AddConstraint(sketch.NewSymmetric(p1, p2, axis))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 3, p2.X(), 1e-6, "mirror x")
	require.InDelta(t, 4, p2.Y(), 1e-6, "mirror y")
}

func TestUnderConstrainedDOF(t *testing.T) {
	s := newSketch(t)
	s.AddPoint(0, 0) // single free point, nothing else
	require.Equal(t, 2, s.DOF(), "DOF")
}

func TestRedundantConstraint(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))

	// Add a redundant duplicate width dimension.
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	res, err := s.Solve()
	require.NoError(t, err)
	require.NotZero(t, res.Redundant, "expected at least one redundant equation")
}

func TestRedundantConstraints(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 20, b.X(), 1e-6, "rectangle reached its solved width")
	require.Empty(t, s.RedundantConstraints(), "clean sketch has no redundancy")

	// A duplicate width dimension is consistent but redundant; the
	// later-added duplicate is the one identified.
	dup := sketch.NewDistance(a, b, 20)
	s.AddConstraint(dup)
	_, err = s.Solve()
	require.NoError(t, err)
	red := s.RedundantConstraints()
	require.Len(t, red, 1, "exactly one redundant constraint")
	got, ok := red[0].(*sketch.Distance)
	require.True(t, ok, "redundant constraint is a distance dimension")
	require.Same(t, dup, got, "the duplicate is the one reported")
}

func TestJSONRoundTrip(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))

	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.MarshalIndent(s, "", "  ")
	require.NoError(t, err, "marshal")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	require.Len(t, s2.Points(), len(s.Points()), "points")

	res, err := s2.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "reloaded DOF")
	require.InDelta(t, 20, s2.Points()[b.ID()].X(), 1e-6, "reloaded b.X")
	require.InDelta(t, 12, s2.Points()[c.ID()].Y(), 1e-6, "reloaded c.Y")
	require.InDelta(t, 0, s2.Points()[d.ID()].X(), 1e-6, "reloaded d.X")
}

func TestJSONRoundTripArcTangent(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	line := s.AddLine(a, b)

	center := s.AddPoint(5, 5)
	s.Fix(center)
	// Arc spans the bottom so the downward tangent contact is within the sweep.
	start := s.AddPoint(2, 2)
	end := s.AddPoint(8, 2)
	arc := s.AddArc(center, start, end)

	o := s.AddPoint(20, 0)
	s.Fix(o)
	circ := s.AddCircle(o, 2)

	s.AddConstraint(sketch.NewTangent(line, arc), sketch.NewEqualRadius(circ, arc))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, arc.R(), 1e-6, "arc radius before round-trip")

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	// The internal arc radius-consistency constraint must be recreated by
	// AddArc exactly once, not also deserialized.
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint count")

	_, err = s2.Solve()
	require.NoError(t, err)
	reloaded, ok := s2.Entities()[1].(*sketch.Arc)
	require.True(t, ok, "entity 1 is the arc")
	circ2, ok := s2.Entities()[2].(*sketch.Circle)
	require.True(t, ok, "entity 2 is the circle")
	require.InDelta(t, 5, reloaded.R(), 1e-6, "reloaded arc radius")
	require.InDelta(t, 5, circ2.R(), 1e-6, "reloaded circle equals arc radius")
}

func TestSVGOutput(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))

	_, err := s.Solve()
	require.NoError(t, err)
	o := s.AddPoint(10, 6)
	s.AddCircle(o, 3)

	svg, err := s.SVG()
	require.NoError(t, err)
	for _, want := range []string{"<svg", "<line", "<circle", "</svg>"} {
		require.Containsf(t, svg, want, "SVG missing %q", want)
	}
}

func TestDXFOutput(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.AddLine(a, b)
	o := s.AddPoint(5, 5)
	s.AddCircle(o, 3)
	st := s.AddPoint(8, 5)
	en := s.AddPoint(5, 8)
	s.AddArc(o, st, en)

	dxf, err := s.DXF()
	require.NoError(t, err)
	for _, want := range []string{"SECTION", "ENTITIES", "LINE", "CIRCLE", "ARC", "EOF"} {
		require.Containsf(t, dxf, want, "DXF missing %q", want)
	}
}
