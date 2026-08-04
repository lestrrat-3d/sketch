package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// nanControlCoordinateNURBS builds a degree-3 NURBS whose control points are
// collinear along y=5 — so the curve is an exact straight line from (-2,5) to
// (12,5), crossing a 10x10 square at (0,0)-(10,10) clean through the middle —
// with its middle control point's x coordinate either an ordinary 4 or NaN.
// CreateNURBS rejects a non-finite knot, but CreatePoint has no error return
// and the solver moves control-point coordinates after construction, so a
// construction-time check there could never hold as a precondition; a NaN
// coordinate reaches committed geometry exactly as a NaN knot once did. The
// poisoned point is left UNFIXED (unlike its neighbours): fixing it would
// exclude it from the free-variable set entirely, while leaving it free is
// what carries the NaN into the rank analysis the way it carries into the
// arrangement.
func nanControlCoordinateNURBS(t *testing.T, s *sketch.Sketch, nan bool) *sketch.NURBS {
	t.Helper()
	mid := 4.0
	if nan {
		mid = math.NaN()
	}
	ctrl := []*sketch.Point{
		s.CreatePoint(-2, 5), s.CreatePoint(1, 5), s.CreatePoint(mid, 5),
		s.CreatePoint(7, 5), s.CreatePoint(12, 5),
	}
	for i, p := range ctrl {
		if nan && i == 2 {
			continue
		}
		s.Fix(p)
	}
	knots := sketch.ClampedUniformKnots(len(ctrl), 3)
	c, err := s.CreateNURBS(3, ctrl, nil, knots)
	require.NoError(t, err)
	return c
}

func groundedSquare(t *testing.T, s *sketch.Sketch) *sketch.Rectangle {
	t.Helper()
	r := s.CreateRectangle(0, 0, 10, 10)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 10), sketch.NewDistance(r.A, r.D, 10))
	return r
}

// TestNaNControlCoordinateNURBSCrossingSquareIsInvalidProfile reproduces the
// wrong-region-count defect end to end through the public API: the NaN-
// coordinate NURBS evaluates to NaN at every sample, so every ordered
// comparison against it is false and it contributes no vertex, cut or edge to
// the arrangement — it vanishes, so the profile it would have split is
// reported invalid rather than silently blessed as one clean region of area
// 100. The poisoned control point is left free (see
// nanControlCoordinateNURBS), so the NaN also reaches the rank analysis: the
// point's x/y are free variables no residual constrains, so DOF is 2 rather
// than the 0 the finite control gets. That is incidental to this fixture, not
// the property under test — ProfilesValid/InvalidProfiles/Trustworthy are.
func TestNaNControlCoordinateNURBSCrossingSquareIsInvalidProfile(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nanControlCoordinateNURBS(t, s, true)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Equal(t, 2, rep.DOF)
	require.Equal(t, sketch.Underconstrained, rep.Status)
	require.False(t, rep.ProfilesValid, "the NaN-coordinate crossing must not be silently dropped")
	require.NotEmpty(t, rep.InvalidProfiles, "the square's region is reached by the degenerate NURBS")
	require.False(t, rep.Trustworthy(), "an arrangement degeneracy must not be blessed")
}

// TestFiniteControlNURBSCrossingSquareIsTrustworthy is the healthy control for
// TestNaNControlCoordinateNURBSCrossingSquareIsInvalidProfile: the identical
// crossing curve with an ordinary finite control point cleanly splits the
// square into two ~50 area regions and is fully trustworthy — the converged
// answer the NaN-coordinate case must never silently substitute (one region
// of area 100).
func TestFiniteControlNURBSCrossingSquareIsTrustworthy(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nanControlCoordinateNURBS(t, s, false)

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
