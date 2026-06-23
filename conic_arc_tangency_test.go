package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A radius-3 circle centered at (7,0) is externally tangent to a fixed rx=4,ry=2
// ellipse at the ellipse's right vertex (4,0) — which is angle π on the circle.
// Building it as an ARC whose sweep includes or excludes that contact exercises
// the sweep confinement: the tangent must touch the swept portion, not just the
// underlying full circle.

func arcStraddlingContact(s *sketch.Sketch) *sketch.Arc {
	c := s.CreatePoint(7, 0)
	start := s.CreatePoint(7+3*math.Cos(3*math.Pi/4), 3*math.Sin(3*math.Pi/4)) // angle 3π/4
	end := s.CreatePoint(7+3*math.Cos(5*math.Pi/4), 3*math.Sin(5*math.Pi/4))   // angle 5π/4 (sweep straddles π)
	arc := s.CreateArc(c, start, end)
	s.FixEntity(arc)
	return arc
}

func arcAwayFromContact(s *sketch.Sketch) *sketch.Arc {
	c := s.CreatePoint(7, 0)
	start := s.CreatePoint(10, 0) // angle 0
	end := s.CreatePoint(7, 3)    // angle π/2 (sweep [0,π/2] excludes the angle-π contact)
	arc := s.CreateArc(c, start, end)
	s.FixEntity(arc)
	return arc
}

func TestTangentEllipseArcInSweep(t *testing.T) {
	s := newSketch(t)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	arc := arcStraddlingContact(s)
	s.AddConstraint(sketch.NewTangentEllipseCircular(e, arc, false))

	_, err := s.Solve()
	require.NoError(t, err)
	require.True(t, s.Verify().Solvable, "the contact (4,0) lies within the arc's sweep")
}

func TestTangentEllipseArcOffSweepRejected(t *testing.T) {
	// Same tangent circle, but the arc spans the far side: the only tangent contact
	// with the ellipse is outside the sweep, so the oracle must report unsolvable.
	s := newSketch(t)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	arc := arcAwayFromContact(s)
	s.AddConstraint(sketch.NewTangentEllipseCircular(e, arc, false))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "a tangent to the full circle off the arc is not blessed")
}

func TestTangentEllipticalArcCircle(t *testing.T) {
	// An elliptical arc (first-quadrant sweep of an rx=6,ry=3 ellipse) tangent to a
	// circle that touches its right vertex (6,0): exercises the ellipticalArcConic
	// operand (Sampson membership + eccentric-sweep confinement). The contact (6,0)
	// is eccentric angle 0, within the [0,π/2] sweep.
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	estart := s.CreatePoint(6, 0) // eccentric 0
	eend := s.CreatePoint(0, 3)   // eccentric π/2
	ea := s.CreateEllipticalArc(ec, estart, eend, 6, 3, 0)
	s.FixEntity(ea)
	c := s.CreateCircle(s.CreatePoint(8, 0), 2) // left point (6,0) meets the arc vertex
	s.FixEntity(c)
	s.AddConstraint(sketch.NewTangentEllipseCircular(ea, c, false))

	_, err := s.Solve()
	require.NoError(t, err)
	require.True(t, s.Verify().Solvable, "elliptical arc externally tangent to the circle at (6,0)")
}

func TestTangentConicsArcDOFAndRemoval(t *testing.T) {
	// A free ellipse tangent to a fixed arc: tangency removes one DOF; the arc's
	// sweep slack adds a var and a row that net to zero.
	s := newSketch(t)
	arc := arcStraddlingContact(s)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0) // free: 5 DOF
	require.Equal(t, 5, s.DOF())

	con := sketch.NewTangentEllipseCircular(e, arc, false)
	s.AddConstraint(con)
	require.Equal(t, 4, s.DOF(), "tangency removes one DOF (the sweep slack nets out)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 5, s.DOF(), "removal restores the DOF (all aux vars retired)")
}

func TestTangentConicsArcRoundTrip(t *testing.T) {
	s := newSketch(t)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	arc := arcStraddlingContact(s)
	s.AddConstraint(sketch.NewTangentEllipseCircular(e, arc, false))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve()
	require.NoError(t, err)
	require.True(t, s2.Verify().Solvable)
}
