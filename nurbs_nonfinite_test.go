package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// nanKnotCrossingNURBS builds a degree-3 NURBS whose control points are
// collinear along y=5 — so the curve is an exact straight line from (-2,5) to
// (12,5), crossing a 10x10 square at (0,0)-(10,10) clean through the middle —
// with its interior knot either an ordinary 0.5 or NaN. CreateNURBS's
// non-decreasing check is knots[i] < knots[i-1], which is false against NaN, so
// the NaN-knot shape is accepted by the public API exactly like a well-formed
// one.
func nanKnotCrossingNURBS(t *testing.T, s *sketch.Sketch, nan bool) *sketch.NURBS {
	t.Helper()
	ctrl := []*sketch.Point{
		s.CreatePoint(-2, 5), s.CreatePoint(1, 5), s.CreatePoint(4, 5),
		s.CreatePoint(7, 5), s.CreatePoint(12, 5),
	}
	for _, p := range ctrl {
		s.Fix(p)
	}
	knots := sketch.ClampedUniformKnots(len(ctrl), 3)
	require.Equal(t, []float64{0, 0, 0, 0, 0.5, 1, 1, 1, 1}, knots, "interior knot at index 4")
	if nan {
		knots[4] = math.NaN()
	}
	c, err := s.CreateNURBS(3, ctrl, nil, knots)
	require.NoError(t, err, "CreateNURBS accepts a NaN interior knot: its validation only rejects knots[i] < knots[i-1]")
	return c
}

func groundedSquare(t *testing.T, s *sketch.Sketch) *sketch.Rectangle {
	t.Helper()
	r := s.CreateRectangle(0, 0, 10, 10)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 10), sketch.NewDistance(r.A, r.D, 10))
	return r
}

// TestNaNKnotNURBSCrossingSquareIsInvalidProfile reproduces the wrong-region-count
// defect end to end through the public API: before the geom-level fix, the
// NaN-knot NURBS evaluated to NaN at every sample, so every ordered comparison
// against it was false and it contributed no vertex, cut or edge to the
// arrangement — it vanished, and Verify reported ONE region of area 100,
// Trustworthy() true, Check() nil, as if the crossing curve were not there at
// all. After the fix, densify samples the whole source before trusting it and
// drops it as degenerate, so the profile it would have split is reported
// invalid instead of silently blessed.
func TestNaNKnotNURBSCrossingSquareIsInvalidProfile(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nanKnotCrossingNURBS(t, s, true)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Equal(t, 0, rep.DOF)
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.False(t, rep.ProfilesValid, "the NaN-knot crossing must not be silently dropped")
	require.NotEmpty(t, rep.InvalidProfiles, "the square's region is reached by the degenerate NURBS")
	require.False(t, rep.Trustworthy(), "an arrangement degeneracy must not be blessed")
}

// TestFiniteControlNURBSCrossingSquareIsTrustworthy is the healthy control for
// TestNaNKnotNURBSCrossingSquareIsInvalidProfile: the identical crossing curve
// with an ordinary finite interior knot cleanly splits the square into two ~50
// area regions and is fully trustworthy — the converged answer the NaN-knot case
// must never silently substitute (one region of area 100).
func TestFiniteControlNURBSCrossingSquareIsTrustworthy(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nanKnotCrossingNURBS(t, s, false)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Equal(t, 0, rep.DOF)
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.Len(t, rep.Profiles, 2, "the crossing line splits the square into two halves")
	require.True(t, rep.ProfilesValid)
	require.Empty(t, rep.InvalidProfiles)
	total := 0.0
	for _, p := range rep.Profiles {
		require.InDelta(t, 50, p.Area, 0.01, "each half is ~10 x 5")
		total += p.Area
	}
	require.InDelta(t, 100, total, 1e-6, "areas still sum exactly to the whole square")
	require.True(t, rep.Trustworthy(), "a clean, fully-constrained crossing is trustworthy")
}
