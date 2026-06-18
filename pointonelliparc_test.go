package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// quarterEllipticalArc builds a fixed elliptical arc sweeping the first quadrant
// (eccentric angle 0 → π/2) of an rx=6, ry=3 ellipse.
func quarterEllipticalArc(s *sketch.Sketch) *sketch.EllipticalArc {
	c := s.AddPoint(0, 0)
	start := s.AddPoint(6, 0) // eccentric 0
	end := s.AddPoint(0, 3)   // eccentric π/2
	ea := s.AddEllipticalArc(c, start, end, 6, 3, 0)
	s.FixEntity(ea) // lock the whole arc rigid (points + rx/ry/rotation)
	return ea
}

// onEllipse evaluates the implicit ellipse value at p (1 means on the ellipse).
func onEllipse(p *sketch.Point, rx, ry float64) float64 {
	return (p.X()/rx)*(p.X()/rx) + (p.Y()/ry)*(p.Y()/ry)
}

func TestPointOnEllipticalArc(t *testing.T) {
	s := sketch.New()
	ea := quarterEllipticalArc(s)
	p := s.AddPoint(4, 2) // near the arc, inside the sweep
	s.AddConstraint(sketch.NewPointOnEllipticalArc(p, ea))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 1, onEllipse(p, 6, 3), 1e-6, "pulled onto the arc's ellipse")
	require.GreaterOrEqual(t, p.X(), -1e-6, "first quadrant (within the sweep)")
	require.GreaterOrEqual(t, p.Y(), -1e-6)
}

func TestPointOnEllipticalArcConfinedToSweep(t *testing.T) {
	// A point started OUTSIDE the sweep must be pulled into it, not left on the
	// full ellipse off the arc.
	s := sketch.New()
	ea := quarterEllipticalArc(s)
	p := s.AddPoint(4, -2) // below the x-axis: outside the [0, π/2] sweep
	s.AddConstraint(sketch.NewPointOnEllipticalArc(p, ea))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 1, onEllipse(p, 6, 3), 1e-6, "on the ellipse")
	require.GreaterOrEqual(t, p.Y(), -1e-6, "confined to the sweep, not left below the axis")
}

func TestPointOnEllipticalArcDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	ea := quarterEllipticalArc(s)
	p := s.AddPoint(4, 2)
	require.Equal(t, 2, s.DOF(), "the free point has two DOF")

	con := sketch.NewPointOnEllipticalArc(p, ea)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "on a 1-D elliptical arc the point keeps one DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal restores the DOF (slack retired)")
}

func TestPointOnEllipticalArcRoundTrip(t *testing.T) {
	s := sketch.New()
	ea := quarterEllipticalArc(s)
	p := s.AddPoint(4, 2)
	s.AddConstraint(sketch.NewPointOnEllipticalArc(p, ea))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve()
	require.NoError(t, err)
}
