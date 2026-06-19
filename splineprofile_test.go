package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A spline now participates in the planar arrangement, so a spline plus a chord
// that closes back to its endpoints bounds a profile the oracle can report.

func TestSplineBoundsProfile(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	c1 := s.AddPoint(1, 2)
	c2 := s.AddPoint(3, 2)
	b := s.AddPoint(4, 0)
	sp, err := s.AddSpline(a, c1, c2, b) // open spline A → B (a hump above y=0)
	require.NoError(t, err)
	s.AddLine(b, a) // chord B → A closes the loop (shared endpoints a, b)

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "the spline + chord bound exactly one region")
	p := profiles[0]
	require.True(t, p.Valid, "a simple spline-bounded region is valid")
	require.Greater(t, p.Area, 0.0, "the bounded region has positive area")
	require.Contains(t, p.Entities, sketch.Entity(sp), "the spline is part of the boundary")
}

func TestSplineProfileSampledAreaApprox(t *testing.T) {
	// Closing a half-disc-like spline cap with its chord: the sampled area is
	// finite and close to the true hump area (sampling, not exact). Controls
	// (0,0),(0,h),(w,h),(w,0) make a flat-topped cap; assert area is in a sane band.
	s := sketch.New()
	a := s.AddPoint(0, 0)
	c1 := s.AddPoint(0, 3)
	c2 := s.AddPoint(6, 3)
	b := s.AddPoint(6, 0)
	_, err := s.AddSpline(a, c1, c2, b)
	require.NoError(t, err)
	s.AddLine(b, a)

	profiles := s.Profiles()
	require.Len(t, profiles, 1)
	// The cap sits below its control hull; area is positive and well under the
	// bounding box (6×3 = 18), confirming the sampled bulge is signed correctly.
	require.Greater(t, profiles[0].Area, 1.0)
	require.Less(t, profiles[0].Area, 18.0)
}

func TestSelfIntersectingSplineLoopInvalid(t *testing.T) {
	// A single cubic Bézier (4 control points, clamped knots) whose control polygon
	// loops, so the curve crosses itself; closing it with a chord makes a
	// self-intersecting boundary the oracle must NOT bless.
	s := sketch.New()
	p0 := s.AddPoint(0, 0)
	p1 := s.AddPoint(-4.0/3.0, -5.0/12.0)
	p2 := s.AddPoint(-4.0/3.0, -3.0/2.0)
	p3 := s.AddPoint(0, 3.0/4.0)
	_, err := s.AddSpline(p0, p1, p2, p3)
	require.NoError(t, err)
	s.AddLine(p3, p0) // close the loop

	rep := s.Verify()
	require.False(t, rep.ProfilesValid, "a self-intersecting spline loop is not valid")
	require.NotEmpty(t, rep.InvalidProfiles)
	var sawSelfX bool
	for _, p := range rep.InvalidProfiles {
		if p.SelfIntersecting {
			sawSelfX = true
		}
	}
	require.True(t, sawSelfX, "the invalid profile is flagged self-intersecting")
}

func TestOpenSplineBoundsNoProfile(t *testing.T) {
	// An open spline with no closing edge bounds no region.
	s := sketch.New()
	a := s.AddPoint(0, 0)
	c1 := s.AddPoint(1, 2)
	c2 := s.AddPoint(3, 2)
	b := s.AddPoint(4, 0)
	_, err := s.AddSpline(a, c1, c2, b)
	require.NoError(t, err)

	require.Empty(t, s.Profiles(), "a lone open spline closes nothing")
}

func TestConstructionSplineExcludedFromProfiles(t *testing.T) {
	// A construction spline is excluded from profiles, so the chord alone (one open
	// line) bounds no region.
	s := sketch.New()
	a := s.AddPoint(0, 0)
	c1 := s.AddPoint(1, 2)
	c2 := s.AddPoint(3, 2)
	b := s.AddPoint(4, 0)
	sp, err := s.AddSpline(a, c1, c2, b)
	require.NoError(t, err)
	sp.SetConstruction(true)
	s.AddLine(b, a)

	require.Empty(t, s.Profiles(), "a construction spline does not close a profile")
}

func TestSplineProfileCoordinateMergeMatchesLines(t *testing.T) {
	// The planar arrangement closes regions by COORDINATE merge (unlike the
	// identity-based geom.Loops), so a chord whose endpoints are distinct *Points
	// at the spline's endpoint coordinates still closes the profile — exactly as it
	// would for lines. A spline must behave consistently with the other curves here.
	s := sketch.New()
	a := s.AddPoint(0, 0)
	c1 := s.AddPoint(1, 2)
	c2 := s.AddPoint(3, 2)
	b := s.AddPoint(4, 0)
	_, err := s.AddSpline(a, c1, c2, b)
	require.NoError(t, err)
	a2 := s.AddPoint(0, 0) // same coords as a, distinct identity
	b2 := s.AddPoint(4, 0) // same coords as b, distinct identity
	s.AddLine(b2, a2)

	require.Len(t, s.Profiles(), 1, "coordinate-coincident chord closes the spline, like lines")
}

func TestAddSplineRejectsNilControl(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	c := s.AddPoint(2, 1)
	_, err := s.AddSpline(a, b, nil, c)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "a nil control point is rejected, not a panic later")
}

func TestEndpointClosedSplineNotSelfIntersecting(t *testing.T) {
	// A spline whose first and last control points are the SAME point forms a
	// closed loop (a teardrop). Its first/last sampled segments meet at the shared
	// endpoint — the natural closure seam, NOT a self-crossing.
	s := sketch.New()
	a := s.AddPoint(0, 0)
	c1 := s.AddPoint(1, 2)
	c2 := s.AddPoint(-1, 2)
	sp, err := s.AddSpline(a, c1, c2, a) // first == last == a: a closed teardrop
	require.NoError(t, err)

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "the closed spline bounds one region")
	require.True(t, profiles[0].Valid, "a simple closed spline is not self-intersecting")
	require.Contains(t, profiles[0].Entities, sketch.Entity(sp))
}
