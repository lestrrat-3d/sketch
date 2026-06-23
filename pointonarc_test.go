package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// quarterArc builds a fixed arc sweeping the first quadrant (angle 0 → π/2,
// radius 5) and returns it.
func quarterArc(s *sketch.Sketch) *sketch.Arc {
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(5, 0)
	end := s.CreatePoint(0, 5)
	s.Fix(c)
	s.Fix(start)
	s.Fix(end)
	return s.CreateArc(c, start, end)
}

func TestPointOnArc(t *testing.T) {
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(3, 3) // near the arc, inside the sweep
	s.AddConstraint(sketch.NewPointOnArc(p, arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, math.Hypot(p.X(), p.Y()), 1e-6, "pulled onto the arc's circle")
	ang := math.Atan2(p.Y(), p.X())
	require.GreaterOrEqual(t, ang, -1e-6, "within the sweep (lower bound)")
	require.LessOrEqual(t, ang, math.Pi/2+1e-6, "within the sweep (upper bound)")
}

func TestPointOnArcConfinedToSweep(t *testing.T) {
	// A point started OUTSIDE the sweep must be pulled into it, not left on the
	// full circle off the arc.
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(3, -3) // angle −π/4, outside the [0, π/2] sweep
	s.AddConstraint(sketch.NewPointOnArc(p, arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, math.Hypot(p.X(), p.Y()), 1e-6, "on the circle")
	ang := math.Atan2(p.Y(), p.X())
	require.GreaterOrEqual(t, ang, -1e-6, "confined to the sweep, not left at −π/4")
	require.LessOrEqual(t, ang, math.Pi/2+1e-6)
}

func TestPointOnArcDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(3, 3)
	require.Equal(t, 2, s.DOF(), "the free point has two DOF")

	con := sketch.NewPointOnArc(p, arc)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "on a 1-D arc the point keeps one DOF (slides along it)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal restores the DOF (slack retired)")
}
