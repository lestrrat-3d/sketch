package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A first-quadrant elliptical arc (rx=6, ry=3) starting at its right vertex
// S=(6,0) and a circular arc that also has S as an endpoint share the point S.
// The ellipse normal at S is +x; a circle centered on the x-axis at (8,0) has
// outward normal −x at S — so the two curves are externally tangent at the
// shared corner. The shared-endpoint branch enforces tangency *at S*.

func TestTangentConicsSharedEndpoint(t *testing.T) {
	s := newSketch(t)
	ec := s.AddPoint(0, 0)
	shared := s.AddPoint(6, 0) // the shared corner (ellipse right vertex)
	eaEnd := s.AddPoint(0, 3)
	ea := s.AddEllipticalArc(ec, shared, eaEnd, 6, 3, 0)
	s.FixEntity(ea)

	cc := s.AddPoint(8, 0)
	caEnd := s.AddPoint(8, 2) // on the radius-2 circle around (8,0)
	ca := s.AddArc(cc, shared, caEnd)
	s.FixEntity(ca)

	s.AddConstraint(sketch.NewTangentEllipseCircular(ea, ca, false)) // external
	_, err := s.Solve()
	require.NoError(t, err)
	require.True(t, s.Verify().Solvable, "tangent at the shared corner S")
}

func TestTangentConicsSharedEndpointNotTangentRejected(t *testing.T) {
	// Same shared corner, but the circle's center is OFF the x-axis, so the circle
	// normal at S is not parallel to the ellipse normal — the curves meet at S but
	// are not tangent there. With everything rigid the shared-endpoint tangency is
	// infeasible and the oracle must report unsolvable.
	s := newSketch(t)
	ec := s.AddPoint(0, 0)
	shared := s.AddPoint(6, 0)
	eaEnd := s.AddPoint(0, 3)
	ea := s.AddEllipticalArc(ec, shared, eaEnd, 6, 3, 0)
	s.FixEntity(ea)

	cc := s.AddPoint(8, 1)                 // off the x-axis: normal at S not ±x
	caEnd := s.AddPoint(8, 1+math.Sqrt(5)) // on the radius-√5 circle around (8,1)
	ca := s.AddArc(cc, shared, caEnd)
	s.FixEntity(ca)

	s.AddConstraint(sketch.NewTangentEllipseCircular(ea, ca, false))
	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "curves meeting at S but not tangent there are not blessed")
}

func TestTangentConicsSharedEndpointDOFAndRemoval(t *testing.T) {
	// A fixed elliptical arc; a circular arc sharing the corner S has a free center
	// and free far endpoint (3 DOF after its intrinsic radius constraint). The
	// shared-endpoint tangency removes exactly one (it pins the circle's normal at S
	// parallel to the ellipse's).
	s := newSketch(t)
	ec := s.AddPoint(0, 0)
	shared := s.AddPoint(6, 0)
	eaEnd := s.AddPoint(0, 3)
	ea := s.AddEllipticalArc(ec, shared, eaEnd, 6, 3, 0)
	s.FixEntity(ea)

	cc := s.AddPoint(8, 0)
	caEnd := s.AddPoint(8, 2)
	s.AddArc(cc, shared, caEnd) // cc, caEnd free; shared fixed by the elliptical arc
	require.Equal(t, 3, s.DOF(), "free center + far endpoint, minus the arc's radius constraint")

	con := sketch.NewTangentEllipseCircular(ea, s.Entities()[1].(*sketch.Arc), false)
	s.AddConstraint(con)
	require.Equal(t, 2, s.DOF(), "shared-endpoint tangency removes one DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 3, s.DOF(), "removal restores the DOF (the branch slack is retired)")
}

func TestTangentConicsSharedEndpointRoundTrip(t *testing.T) {
	s := newSketch(t)
	ec := s.AddPoint(0, 0)
	shared := s.AddPoint(6, 0)
	eaEnd := s.AddPoint(0, 3)
	ea := s.AddEllipticalArc(ec, shared, eaEnd, 6, 3, 0)
	s.FixEntity(ea)
	cc := s.AddPoint(8, 0)
	caEnd := s.AddPoint(8, 2)
	ca := s.AddArc(cc, shared, caEnd)
	s.FixEntity(ca)
	s.AddConstraint(sketch.NewTangentEllipseCircular(ea, ca, false))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve()
	require.NoError(t, err)
	require.True(t, s2.Verify().Solvable, "shared-endpoint branch reconstructed on load")
}
