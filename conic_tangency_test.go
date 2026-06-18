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
	s := sketch.New()
	ec := s.AddPoint(0, 0)
	e := s.AddEllipse(ec, 4, 2, 0)
	s.FixEntity(e)
	cc := s.AddPoint(10, 0)
	c := s.AddCircle(cc, 3)
	s.FixEntity(c)                                      // radius fixed
	s.Unfix(cc)                                         // but let the center move
	s.AddConstraint(sketch.NewHorizontalPoints(cc, ec)) // keep it on the x-axis

	s.AddConstraint(sketch.NewTangentEllipseCircle(e, c, false))
	_, err := s.Solve()
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
	s := sketch.New()
	ec := s.AddPoint(0, 0)
	e := s.AddEllipse(ec, 8, 4, 0)
	s.FixEntity(e)
	cc := s.AddPoint(6, 0)
	c := s.AddCircle(cc, 1)
	s.FixEntity(c)
	s.Unfix(cc)
	s.AddConstraint(sketch.NewHorizontalPoints(cc, ec))

	s.AddConstraint(sketch.NewTangentEllipseCircle(e, c, true))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 7, cc.X(), 1e-4, "internal tangent at the ellipse's right vertex")
}

func TestTangentEllipses(t *testing.T) {
	// Two axis-aligned ellipses, the second sliding along x, forced externally
	// tangent: e1 right vertex (4,0) meets e2 left vertex (cx−3,0), so cx → 7.
	s := sketch.New()
	c1 := s.AddPoint(0, 0)
	e1 := s.AddEllipse(c1, 4, 2, 0)
	s.FixEntity(e1)
	c2 := s.AddPoint(10, 0)
	e2 := s.AddEllipse(c2, 3, 1.5, 0)
	s.FixEntity(e2)
	s.Unfix(c2)
	s.AddConstraint(sketch.NewHorizontalPoints(c2, c1))

	s.AddConstraint(sketch.NewTangentEllipses(e1, e2, false))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 7, c2.X(), 1e-4, "external tangent of two ellipses")
}

func TestTangentEllipseCircleSeparateRejected(t *testing.T) {
	// Two rigid conics that are far apart cannot be made tangent (nothing is free
	// to move them together); the oracle must report it unsolvable.
	s := sketch.New()
	e := s.AddEllipse(s.AddPoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	c := s.AddCircle(s.AddPoint(100, 0), 3)
	s.FixEntity(c)
	s.AddConstraint(sketch.NewTangentEllipseCircle(e, c, false))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "separate conics are not tangent")
}

func TestTangentConicsDegenerateRejected(t *testing.T) {
	// A degenerate ellipse has no tangent direction. It must be reported
	// unsolvable — and must not poison the oracle with a NaN residual (the normal's
	// axis-squares are floored). The degeneracy threshold (1e-6) matches that floor,
	// so a sub-floor axis cannot be "solved" against a floored surrogate.
	for _, rx := range []float64{0, 1e-7} { // exact zero, and inside the [1e-9,1e-6) band
		s := sketch.New()
		e := s.AddEllipse(s.AddPoint(0, 0), rx, 2, 0)
		s.FixEntity(e)
		c := s.AddCircle(s.AddPoint(1+1e-6, 0), 1) // placed where the floored surrogate would falsely "touch"
		s.FixEntity(c)
		s.AddConstraint(sketch.NewTangentEllipseCircle(e, c, false))

		_, err := s.Solve()
		require.Error(t, err, "rx=%v is degenerate", rx)
		rep := s.Verify()
		require.False(t, rep.Solvable, "a degenerate ellipse has no tangent (rx=%v)", rx)
		require.False(t, math.IsNaN(rep.Residual), "residual must stay finite, not NaN (rx=%v)", rx)
	}
}

func TestTangentConicsDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	e := s.AddEllipse(s.AddPoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	cc := s.AddPoint(10, 0)
	c := s.AddCircle(cc, 3)
	s.FixEntity(c)
	s.Unfix(cc) // a free circle center: 2 DOF
	require.Equal(t, 2, s.DOF())

	con := sketch.NewTangentEllipseCircle(e, c, false)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "tangency removes one DOF (circle slides around, staying tangent)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestTangentConicsCheckConstraint(t *testing.T) {
	s := sketch.New()
	e := s.AddEllipse(s.AddPoint(0, 0), 4, 2, 0)
	s.FixEntity(e)
	cc := s.AddPoint(10, 0)
	c := s.AddCircle(cc, 3)
	s.FixEntity(c)
	s.Unfix(cc)
	require.NoError(t, s.CheckConstraint(sketch.NewTangentEllipseCircle(e, c, false)))
}

func TestTangentConicsRoundTrip(t *testing.T) {
	s := sketch.New()
	ec := s.AddPoint(0, 0)
	e := s.AddEllipse(ec, 4, 2, 0)
	s.FixEntity(e)
	cc := s.AddPoint(10, 0)
	c := s.AddCircle(cc, 3)
	s.FixEntity(c)
	s.Unfix(cc)
	s.AddConstraint(sketch.NewHorizontalPoints(cc, ec))
	s.AddConstraint(sketch.NewTangentEllipseCircle(e, c, false))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 7, cc.X(), 1e-4)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive reload")
	_, err = s2.Solve()
	require.NoError(t, err)
}

func TestTangentConicsInternalExternalDistinct(t *testing.T) {
	// The internal/external flag is an enforced equation, not just a seed: the same
	// geometry rejects the wrong branch. A circle sitting OUTSIDE the ellipse to the
	// right, with its center fixed where only an external tangent exists, cannot
	// satisfy an INTERNAL tangency request.
	build := func(internal bool) *sketch.Sketch {
		s := sketch.New()
		e := s.AddEllipse(s.AddPoint(0, 0), 4, 2, 0)
		s.FixEntity(e)
		c := s.AddCircle(s.AddPoint(7, 0), 3) // left point exactly at the (4,0) vertex
		s.FixEntity(c)
		s.AddConstraint(sketch.NewTangentEllipseCircle(e, c, internal))
		return s
	}
	// External: the geometry is already a valid external tangency.
	_, err := build(false).Solve()
	require.NoError(t, err, "external tangency holds for this rigid geometry")
	// Internal: same rigid geometry, but the normals are opposed (external), so the
	// internal branch is infeasible.
	si := build(true)
	_, err = si.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, si.Verify().Solvable, "the wrong internal/external branch is rejected")
}
