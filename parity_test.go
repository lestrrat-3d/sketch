package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Behavioral coverage for the constraint/dimension parity batch: horizontal /
// vertical between bare points, a generalized midpoint, radius/diameter on arcs,
// and concentric on arcs. Each asserts on solved geometry.

func TestHorizontalPoints(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 4)
	s.Fix(a)
	b := s.CreatePoint(10, -3) // skewed: shares no line with a
	s.AddConstraint(sketch.NewHorizontalPoints(a, b))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 4, b.Y(), 1e-6, "b pulled to a's y")
	require.InDelta(t, 10, b.X(), 1e-6, "x unconstrained, stays put")
}

func TestVerticalPoints(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(5, 0)
	s.Fix(a)
	b := s.CreatePoint(-2, 9)
	s.AddConstraint(sketch.NewVerticalPoints(a, b))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, b.X(), 1e-6, "b pulled to a's x")
	require.InDelta(t, 9, b.Y(), 1e-6, "y unconstrained, stays put")
}

func TestMidpointOf(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 6)
	s.Fix(a)
	s.Fix(b)
	mid := s.CreatePoint(1, 1) // arbitrary start
	s.AddConstraint(sketch.NewMidpointOf(mid, a, b))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, mid.X(), 1e-6, "midpoint x")
	require.InDelta(t, 3, mid.Y(), 1e-6, "midpoint y")
}

func TestRadiusOnArc(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	s.Fix(c)
	start := s.CreatePoint(3, 0)
	end := s.CreatePoint(0, 3)
	a := s.CreateArc(c, start, end)
	s.AddConstraint(sketch.NewRadius(a, 5))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, a.R(), 1e-6, "arc radius driven to 5")
	// The internal radius-consistency constraint keeps start/end equidistant.
	require.InDelta(t, 5, c.DistanceTo(end), 1e-6, "end stays on the circle")
}

func TestDiameterOnArc(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	s.Fix(c)
	start := s.CreatePoint(2, 0)
	end := s.CreatePoint(0, 2)
	a := s.CreateArc(c, start, end)
	s.AddConstraint(sketch.NewDiameter(a, 12))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 6, a.R(), 1e-6, "arc radius is half the diameter")
}

func TestConcentricArcs(t *testing.T) {
	s := newSketch(t)
	c1 := s.CreatePoint(0, 0)
	s.Fix(c1)
	s1 := s.CreatePoint(2, 0)
	e1 := s.CreatePoint(0, 2)
	a1 := s.CreateArc(c1, s1, e1)

	c2 := s.CreatePoint(5, 5) // displaced center
	s2 := s.CreatePoint(8, 5)
	e2 := s.CreatePoint(5, 8)
	a2 := s.CreateArc(c2, s2, e2)

	s.AddConstraint(sketch.NewConcentric(a1, a2))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, c1.DistanceTo(c2), 1e-6, "arc centers coincide")
}

// Concentric must still accept a circle paired with an arc.
func TestConcentricCircleArc(t *testing.T) {
	s := newSketch(t)
	cc := s.CreatePoint(0, 0)
	s.Fix(cc)
	circle := s.CreateCircle(cc, 3)

	ac := s.CreatePoint(5, 1)
	as := s.CreatePoint(7, 1)
	ae := s.CreatePoint(5, 3)
	arc := s.CreateArc(ac, as, ae)

	s.AddConstraint(sketch.NewConcentric(circle, arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, cc.DistanceTo(ac), 1e-6, "circle and arc share a center")
}
