package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// nanFitPointSpline builds a fit-point spline describing a hump-shaped curve
// crossing a 10x10 square at (0,0)-(10,10), with the fit point at index nanAt
// either an ordinary finite coordinate or NaN (nanAt < 0 leaves every point
// finite). CreateFitSpline has no way to validate a NaN fit point the way
// CreateNURBS validates a knot: CreatePoint has no error return and the
// solver moves point coordinates after construction, so a construction-time
// check could never hold as a precondition. Every fit point is fixed, the
// poisoned one included, mirroring nanControlCoordinateNURBS.
func nanFitPointSpline(t *testing.T, s *sketch.Sketch, nanAt int) *sketch.FitSpline {
	t.Helper()
	pts := [][2]float64{{-2, 1}, {2, 9}, {5, 8}, {8, 9}, {12, 1}}
	fit := make([]*sketch.Point, len(pts))
	for i, p := range pts {
		x := p[0]
		if i == nanAt {
			x = math.NaN()
		}
		fit[i] = s.CreatePoint(x, p[1])
		s.Fix(fit[i])
	}
	fs, err := s.CreateFitSpline(fit...)
	require.NoError(t, err)
	return fs
}

// TestNaNFitPointSplineCrossingSquareIsInvalidProfile reproduces the
// wrong-region-count defect end to end through the public API: before the
// fix, a fit-point spline with a NaN fit point was silently reinterpolated
// through its remaining (finite) points by newFitEvaluator's own coincidence
// filter — so the arrangement never saw a non-finite sample, and the crossing
// curve's profile split was reported as trustworthy despite depending on the
// dropped point. The profile verdict is asserted on its own account through
// Check(), so a future change cannot mask this behind a different failure
// reason.
func TestNaNFitPointSplineCrossingSquareIsInvalidProfile(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nanFitPointSpline(t, s, 2)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.False(t, rep.ProfilesValid, "the NaN fit point must not be silently dropped")
	require.NotEmpty(t, rep.InvalidProfiles, "the square's region is reached by the degenerate fit spline")
	require.ErrorIs(t, rep.Check(), sketch.ErrInvalidProfile, "the profile verdict must fail on its own account")
	require.False(t, rep.Trustworthy(), "an arrangement degeneracy must not be blessed")
}

// TestFiniteFitPointSplineCrossingSquareIsTrustworthy is the healthy control
// for TestNaNFitPointSplineCrossingSquareIsInvalidProfile: the identical
// crossing curve with every fit point finite cleanly splits the square into
// two regions and is fully trustworthy.
func TestFiniteFitPointSplineCrossingSquareIsTrustworthy(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nanFitPointSpline(t, s, -1)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Len(t, rep.Profiles, 2, "the crossing fit spline splits the square into two regions")
	require.True(t, rep.ProfilesValid)
	require.Empty(t, rep.InvalidProfiles)
	total := 0.0
	for _, p := range rep.Profiles {
		total += p.Area
	}
	require.InDelta(t, 100, total, 1e-6, "areas still sum exactly to the whole square")
	require.NoError(t, rep.Check())
	require.True(t, rep.Trustworthy(), "a clean crossing is trustworthy")
}
