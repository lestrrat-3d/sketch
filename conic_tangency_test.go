package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestTangentEllipseCircleExternal(t *testing.T) {
	// Fixed ellipse (rx=4, ry=2) at the origin; a radius-3 circle whose center
	// slides along the x-axis is forced externally tangent. The contact is the
	// ellipse's right vertex (4,0): the circle's left point (cx−3,0) meets it, so
	// cx → 7.
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	e := s.CreateEllipse(ec, 4, 2, 0)
	s.FixEntity(e)
	cc := s.CreatePoint(10, 0)
	c := s.CreateCircle(cc, 3)
	s.FixEntity(c)                                      // radius fixed
	s.Unfix(cc)                                         // but let the center move
	s.AddConstraint(sketch.NewHorizontalPoints(cc, ec)) // keep it on the x-axis

	s.AddConstraint(sketch.NewTangentEllipseCircular(e, c, false))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, cc.X(), 1e-4, "external tangent at the ellipse's right vertex")
	require.InDelta(t, 0, cc.Y(), 1e-6)
}

func TestTangentEllipseCircleInternal(t *testing.T) {
	// A radius-1 circle inside a fixed ellipse (rx=8, ry=4), internally tangent on
	// the right at the ellipse vertex (8,0): the circle's right point (cx+1,0)
	// meets it, so cx → 7 (outward normals aligned). The radius (1) is below the
	// ellipse's minimum curvature radius (ry²/rx = 2), so the vertex contact is the
	// genuine single-point internal tangency.
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	e := s.CreateEllipse(ec, 8, 4, 0)
	s.FixEntity(e)
	cc := s.CreatePoint(6, 0)
	c := s.CreateCircle(cc, 1)
	s.FixEntity(c)
	s.Unfix(cc)
	s.AddConstraint(sketch.NewHorizontalPoints(cc, ec))

	s.AddConstraint(sketch.NewTangentEllipseCircular(e, c, true))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, cc.X(), 1e-4, "internal tangent at the ellipse's right vertex")
}

func TestTangentEllipses(t *testing.T) {
	// Two axis-aligned ellipses, the second sliding along x, forced externally
	// tangent: e1 right vertex (4,0) meets e2 left vertex (cx−3,0), so cx → 7.
	s := newSketch(t)
	c1 := s.CreatePoint(0, 0)
	e1 := s.CreateEllipse(c1, 4, 2, 0)
	s.FixEntity(e1)
	c2 := s.CreatePoint(10, 0)
	e2 := s.CreateEllipse(c2, 3, 1.5, 0)
	s.FixEntity(e2)
	s.Unfix(c2)
	s.AddConstraint(sketch.NewHorizontalPoints(c2, c1))

	s.AddConstraint(sketch.NewTangentEllipses(e1, e2, false))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, c2.X(), 1e-4, "external tangent of two ellipses")
}

func TestTangentEllipseCircleSeparateRejected(t *testing.T) {
	// Two rigid conics that are far apart cannot be made tangent (nothing is free
	// to move them together); the oracle must report it unsolvable.
	s := newSketch(t)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	c := s.CreateCircle(s.CreatePoint(100, 0), 3)
	s.FixEntity(c)
	s.AddConstraint(sketch.NewTangentEllipseCircular(e, c, false))

	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify(t.Context()).Solvable, "separate conics are not tangent")
}

func TestTangentConicsDegenerateRejected(t *testing.T) {
	// A degenerate ellipse has no tangent direction. It must be reported
	// unsolvable — and must not poison the oracle with a NaN residual (the normal's
	// axis-squares are floored). The degeneracy threshold (1e-6) matches that floor,
	// so a sub-floor axis cannot be "solved" against a floored surrogate.
	for _, rx := range []float64{0, 1e-7} { // exact zero, and inside the [1e-9,1e-6) band
		s := newSketch(t)
		e := s.CreateEllipse(s.CreatePoint(0, 0), rx, 2, 0)
		s.FixEntity(e)
		c := s.CreateCircle(s.CreatePoint(1+1e-6, 0), 1) // placed where the floored surrogate would falsely "touch"
		s.FixEntity(c)
		s.AddConstraint(sketch.NewTangentEllipseCircular(e, c, false))

		_, err := s.Solve(t.Context())
		require.Error(t, err, "rx=%v is degenerate", rx)
		rep := s.Verify(t.Context())
		require.False(t, rep.Solvable, "a degenerate ellipse has no tangent (rx=%v)", rx)
		require.False(t, math.IsNaN(rep.Residual), "residual must stay finite, not NaN (rx=%v)", rx)
	}
}

func TestTangentConicsDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	cc := s.CreatePoint(10, 0)
	c := s.CreateCircle(cc, 3)
	s.FixEntity(c)
	s.Unfix(cc) // a free circle center: 2 DOF
	require.Equal(t, 2, s.DOF())

	con := sketch.NewTangentEllipseCircular(e, c, false)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "tangency removes one DOF (circle slides around, staying tangent)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestTangentConicsCheckConstraint(t *testing.T) {
	s := newSketch(t)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	cc := s.CreatePoint(10, 0)
	c := s.CreateCircle(cc, 3)
	s.FixEntity(c)
	s.Unfix(cc)
	require.NoError(t, s.CheckConstraint(sketch.NewTangentEllipseCircular(e, c, false)))
}

func TestTangentConicsRoundTrip(t *testing.T) {
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	e := s.CreateEllipse(ec, 4, 2, 0)
	s.FixEntity(e)
	cc := s.CreatePoint(10, 0)
	c := s.CreateCircle(cc, 3)
	s.FixEntity(c)
	s.Unfix(cc)
	s.AddConstraint(sketch.NewHorizontalPoints(cc, ec))
	s.AddConstraint(sketch.NewTangentEllipseCircular(e, c, false))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, cc.X(), 1e-4)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive reload")
	_, err = s2.Solve(t.Context())
	require.NoError(t, err)
}

func TestTangentConicsInternalExternalDistinct(t *testing.T) {
	// The internal/external flag is an enforced equation, not just a seed: the same
	// geometry rejects the wrong branch. A circle sitting OUTSIDE the ellipse to the
	// right, with its center fixed where only an external tangent exists, cannot
	// satisfy an INTERNAL tangency request.
	build := func(internal bool) *sketch.Sketch {
		s := newSketch(t)
		e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
		s.FixEntity(e)
		c := s.CreateCircle(s.CreatePoint(7, 0), 3) // left point exactly at the (4,0) vertex
		s.FixEntity(c)
		s.AddConstraint(sketch.NewTangentEllipseCircular(e, c, internal))
		return s
	}
	// External: the geometry is already a valid external tangency.
	_, err := build(false).Solve(t.Context())
	require.NoError(t, err, "external tangency holds for this rigid geometry")
	// Internal: same rigid geometry, but the normals are opposed (external), so the
	// internal branch is infeasible.
	si := build(true)
	_, err = si.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, si.Verify(t.Context()).Solvable, "the wrong internal/external branch is rejected")
}

// A first-quadrant elliptical arc (rx=6, ry=3) starting at its right vertex
// S=(6,0) and a circular arc that also has S as an endpoint share the point S.
// The ellipse normal at S is +x; a circle centered on the x-axis at (8,0) has
// outward normal −x at S — so the two curves are externally tangent at the
// shared corner. The shared-endpoint branch enforces tangency *at S*.

func TestTangentConicsSharedEndpoint(t *testing.T) {
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	shared := s.CreatePoint(6, 0) // the shared corner (ellipse right vertex)
	eaEnd := s.CreatePoint(0, 3)
	ea := s.CreateEllipticalArc(ec, shared, eaEnd, 6, 3, 0)
	s.FixEntity(ea)

	cc := s.CreatePoint(8, 0)
	caEnd := s.CreatePoint(8, 2) // on the radius-2 circle around (8,0)
	ca := s.CreateArc(cc, shared, caEnd)
	s.FixEntity(ca)

	s.AddConstraint(sketch.NewTangentEllipseCircular(ea, ca, false)) // external
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, s.Verify(t.Context()).Solvable, "tangent at the shared corner S")
}

func TestTangentConicsSharedEndpointNotTangentRejected(t *testing.T) {
	// Same shared corner, but the circle's center is OFF the x-axis, so the circle
	// normal at S is not parallel to the ellipse normal — the curves meet at S but
	// are not tangent there. With everything rigid the shared-endpoint tangency is
	// infeasible and the oracle must report unsolvable.
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	shared := s.CreatePoint(6, 0)
	eaEnd := s.CreatePoint(0, 3)
	ea := s.CreateEllipticalArc(ec, shared, eaEnd, 6, 3, 0)
	s.FixEntity(ea)

	cc := s.CreatePoint(8, 1)                 // off the x-axis: normal at S not ±x
	caEnd := s.CreatePoint(8, 1+math.Sqrt(5)) // on the radius-√5 circle around (8,1)
	ca := s.CreateArc(cc, shared, caEnd)
	s.FixEntity(ca)

	s.AddConstraint(sketch.NewTangentEllipseCircular(ea, ca, false))
	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify(t.Context()).Solvable, "curves meeting at S but not tangent there are not blessed")
}

func TestTangentConicsSharedEndpointDOFAndRemoval(t *testing.T) {
	// A fixed elliptical arc; a circular arc sharing the corner S has a free center
	// and free far endpoint (3 DOF after its intrinsic radius constraint). The
	// shared-endpoint tangency removes exactly one (it pins the circle's normal at S
	// parallel to the ellipse's).
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	shared := s.CreatePoint(6, 0)
	eaEnd := s.CreatePoint(0, 3)
	ea := s.CreateEllipticalArc(ec, shared, eaEnd, 6, 3, 0)
	s.FixEntity(ea)

	cc := s.CreatePoint(8, 0)
	caEnd := s.CreatePoint(8, 2)
	s.CreateArc(cc, shared, caEnd) // cc, caEnd free; shared fixed by the elliptical arc
	require.Equal(t, 3, s.DOF(), "free center + far endpoint, minus the arc's radius constraint")

	con := sketch.NewTangentEllipseCircular(ea, s.Entities()[1].(*sketch.Arc), false)
	s.AddConstraint(con)
	require.Equal(t, 2, s.DOF(), "shared-endpoint tangency removes one DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 3, s.DOF(), "removal restores the DOF (the branch slack is retired)")
}

func TestTangentConicsSharedEndpointRoundTrip(t *testing.T) {
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	shared := s.CreatePoint(6, 0)
	eaEnd := s.CreatePoint(0, 3)
	ea := s.CreateEllipticalArc(ec, shared, eaEnd, 6, 3, 0)
	s.FixEntity(ea)
	cc := s.CreatePoint(8, 0)
	caEnd := s.CreatePoint(8, 2)
	ca := s.CreateArc(cc, shared, caEnd)
	s.FixEntity(ca)
	s.AddConstraint(sketch.NewTangentEllipseCircular(ea, ca, false))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, s2.Verify(t.Context()).Solvable, "shared-endpoint branch reconstructed on load")
}

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

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, s.Verify(t.Context()).Solvable, "the contact (4,0) lies within the arc's sweep")
}

func TestTangentEllipseArcOffSweepRejected(t *testing.T) {
	// Same tangent circle, but the arc spans the far side: the only tangent contact
	// with the ellipse is outside the sweep, so the oracle must report unsolvable.
	s := newSketch(t)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	arc := arcAwayFromContact(s)
	s.AddConstraint(sketch.NewTangentEllipseCircular(e, arc, false))

	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify(t.Context()).Solvable, "a tangent to the full circle off the arc is not blessed")
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

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, s.Verify(t.Context()).Solvable, "elliptical arc externally tangent to the circle at (6,0)")
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
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, s2.Verify(t.Context()).Solvable)
}
