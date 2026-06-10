package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Behavioral coverage for constraints whose only previous exercise was the
// JSON rebuild path. Each test asserts on solved geometry; the catalog lives
// in docs/acceptance-tests.md.

func TestCoincident(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 2, 3)
	s.Fix(a)
	p := addPt(s, 10, -4)
	s.AddConstraint(sketch.NewCoincident(a, p))

	mustSolve(t, s)
	require.InDelta(t, 2, p.X(), 1e-6, "coincident x")
	require.InDelta(t, 3, p.Y(), 1e-6, "coincident y")
	require.InDelta(t, 0, pointDist(a, p), 1e-6, "points occupy one location")
}

func TestParallel(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	s.Fix(a)
	s.Fix(b)
	l1 := addLn(s, a, b)

	c := addPt(s, 0, 5)
	s.Fix(c)
	d := addPt(s, 8, 7) // deliberately skewed
	l2 := addLn(s, c, d)
	s.AddConstraint(sketch.NewParallel(l1, l2))
	addDist(s, c, d, 8)

	mustSolve(t, s)
	d1x, d1y := lineDir(l1)
	d2x, d2y := lineDir(l2)
	require.InDelta(t, 0, d1x*d2y-d1y*d2x, 1e-6, "directions parallel")
	// Parallel constrains direction only: l2 must keep its own offset, not
	// collapse onto l1.
	require.InDelta(t, 5, d.Y(), 1e-6, "l2 keeps its offset from l1")
	require.InDelta(t, 8, pointDist(c, d), 1e-6, "l2 length held by the dimension")
}

func TestCollinear(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	s.Fix(a)
	s.Fix(b)
	l1 := addLn(s, a, b)

	c := addPt(s, 2, 3)
	d := addPt(s, 7, 5)
	l2 := addLn(s, c, d)
	s.AddConstraint(sketch.NewCollinear(l1, l2))
	addDist(s, c, d, 5)

	mustSolve(t, s)
	require.InDelta(t, 0, c.Y(), 1e-6, "c dropped onto l1's infinite line")
	require.InDelta(t, 0, d.Y(), 1e-6, "d dropped onto l1's infinite line")
	require.InDelta(t, 5, pointDist(c, d), 1e-6, "length held by the dimension")
}

func TestPointOnCircle(t *testing.T) {
	s := sketch.New()
	o := addPt(s, 0, 0)
	s.Fix(o)
	circ := addCir(s, o, 5)
	s.AddConstraint(sketch.NewRadius(circ, 5))

	p := addPt(s, 7, 1)
	s.AddConstraint(sketch.NewPointOnCircle(p, circ))

	res := mustSolve(t, s)
	require.InDelta(t, 5, math.Hypot(p.X(), p.Y()), 1e-6, "point lands on the circle")
	require.Equal(t, 1, res.DOF, "point keeps one sliding freedom along the circle")
}

func TestMidpoint(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 9, 1)
	a.MoveTo(0, 0)
	s.Fix(a)
	ab := addLn(s, a, b)
	s.AddConstraint(sketch.NewHorizontal(ab))
	w := addDist(s, a, b, 10)

	m := addPt(s, 3, 3)
	s.AddConstraint(sketch.NewMidpoint(m, ab))

	mustSolve(t, s)
	require.InDelta(t, 5, m.X(), 1e-6, "midpoint x")
	require.InDelta(t, 0, m.Y(), 1e-6, "midpoint y")

	// The midpoint is parametric: stretching the line carries it along.
	w.Set(20)
	mustSolve(t, s)
	require.InDelta(t, 10, m.X(), 1e-6, "midpoint tracks the stretched line")
}

func TestEqualLines(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 8, 0)
	s.Fix(a)
	s.Fix(b)
	l1 := addLn(s, a, b)

	c := addPt(s, 20, 0)
	s.Fix(c)
	d := addPt(s, 25, 3)
	l2 := addLn(s, c, d)
	s.AddConstraint(sketch.NewEqual(l1, l2))

	mustSolve(t, s)
	require.InDelta(t, 8, pointDist(c, d), 1e-6, "lengths equalized")
}

func TestDiameterDimension(t *testing.T) {
	s := sketch.New()
	o := addPt(s, 0, 0)
	s.Fix(o)
	circ := addCir(s, o, 3)
	dia := sketch.NewDiameter(circ, 14)
	s.AddConstraint(dia)

	mustSolve(t, s)
	require.InDelta(t, 7, circ.R(), 1e-6, "radius from diameter")

	dia.Set(20) // diameters are editable like any dimension
	mustSolve(t, s)
	require.InDelta(t, 10, circ.R(), 1e-6, "radius after diameter edit")
}

func TestUnfix(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	s.Fix(a)
	p := addPt(s, 1, 2)
	addDist(s, a, p, 5)

	// While p is also grounded the dimension cannot be satisfied: both points
	// are pinned at distance √5, the constraint demands 5.
	s.Fix(p)
	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "two grounded points cannot satisfy the dimension")

	// Releasing p restores its freedom and the same sketch solves.
	s.Unfix(p)
	require.False(t, p.IsFixed(), "p reports unfixed")
	mustSolve(t, s)
	require.InDelta(t, 5, pointDist(a, p), 1e-6, "distance holds once p is released")
}

func TestHorizontalVerticalDistance(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	s.Fix(a)

	// Aligned 5 with Δx pinned at 4 leaves Δy = 3 (3-4-5 triangle; the sign
	// comes from the starting side).
	b := addPt(s, 4, 1)
	s.AddConstraint(sketch.NewHorizontalDistance(a, b, 4))
	addDist(s, a, b, 5)

	// And the mirror case: Δy pinned at 3 leaves Δx = 4.
	c := addPt(s, 1, 6)
	s.AddConstraint(sketch.NewVerticalDistance(a, c, 3))
	addDist(s, a, c, 5)

	mustSolve(t, s)
	require.InDelta(t, 4, b.X(), 1e-6, "b Δx")
	require.InDelta(t, 3, b.Y(), 1e-6, "b Δy")
	require.InDelta(t, 3, c.Y(), 1e-6, "c Δy")
	require.InDelta(t, 4, c.X(), 1e-6, "c Δx")
}
