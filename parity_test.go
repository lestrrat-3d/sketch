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
	s := sketch.New()
	a := s.AddPoint(0, 4)
	s.Fix(a)
	b := s.AddPoint(10, -3) // skewed: shares no line with a
	s.AddConstraint(sketch.NewHorizontalPoints(a, b))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 4, b.Y(), 1e-6, "b pulled to a's y")
	require.InDelta(t, 10, b.X(), 1e-6, "x unconstrained, stays put")
}

func TestVerticalPoints(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(5, 0)
	s.Fix(a)
	b := s.AddPoint(-2, 9)
	s.AddConstraint(sketch.NewVerticalPoints(a, b))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, b.X(), 1e-6, "b pulled to a's x")
	require.InDelta(t, 9, b.Y(), 1e-6, "y unconstrained, stays put")
}

func TestMidpointOf(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 6)
	s.Fix(a)
	s.Fix(b)
	mid := s.AddPoint(1, 1) // arbitrary start
	s.AddConstraint(sketch.NewMidpointOf(mid, a, b))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, mid.X(), 1e-6, "midpoint x")
	require.InDelta(t, 3, mid.Y(), 1e-6, "midpoint y")
}

func TestRadiusOnArc(t *testing.T) {
	s := sketch.New()
	c := s.AddPoint(0, 0)
	s.Fix(c)
	start := s.AddPoint(3, 0)
	end := s.AddPoint(0, 3)
	a := s.AddArc(c, start, end)
	s.AddConstraint(sketch.NewRadius(a, 5))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, a.R(), 1e-6, "arc radius driven to 5")
	// The internal radius-consistency constraint keeps start/end equidistant.
	require.InDelta(t, 5, c.DistanceTo(end), 1e-6, "end stays on the circle")
}

func TestDiameterOnArc(t *testing.T) {
	s := sketch.New()
	c := s.AddPoint(0, 0)
	s.Fix(c)
	start := s.AddPoint(2, 0)
	end := s.AddPoint(0, 2)
	a := s.AddArc(c, start, end)
	s.AddConstraint(sketch.NewDiameter(a, 12))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 6, a.R(), 1e-6, "arc radius is half the diameter")
}

func TestConcentricArcs(t *testing.T) {
	s := sketch.New()
	c1 := s.AddPoint(0, 0)
	s.Fix(c1)
	s1 := s.AddPoint(2, 0)
	e1 := s.AddPoint(0, 2)
	a1 := s.AddArc(c1, s1, e1)

	c2 := s.AddPoint(5, 5) // displaced center
	s2 := s.AddPoint(8, 5)
	e2 := s.AddPoint(5, 8)
	a2 := s.AddArc(c2, s2, e2)

	s.AddConstraint(sketch.NewConcentric(a1, a2))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, c1.DistanceTo(c2), 1e-6, "arc centers coincide")
}

// Concentric must still accept a circle paired with an arc.
func TestConcentricCircleArc(t *testing.T) {
	s := sketch.New()
	cc := s.AddPoint(0, 0)
	s.Fix(cc)
	circle := s.AddCircle(cc, 3)

	ac := s.AddPoint(5, 1)
	as := s.AddPoint(7, 1)
	ae := s.AddPoint(5, 3)
	arc := s.AddArc(ac, as, ae)

	s.AddConstraint(sketch.NewConcentric(circle, arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, cc.DistanceTo(ac), 1e-6, "circle and arc share a center")
}
