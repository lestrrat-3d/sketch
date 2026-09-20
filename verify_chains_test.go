package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestVerifyReportsChains pins that one Verify call answers for BOTH of the
// arrangement's publications, so a consumer that wants the open runs as well as
// the regions does not arrange the sketch a second time.
func TestVerifyReportsChains(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(0, 0, 20, 12)
	tail := s.CreateLine(s.CreatePoint(30, 0), s.CreatePoint(40, 0))

	rep := s.Verify(t.Context())
	require.True(t, rep.Analysed())
	require.Len(t, rep.Profiles, 1, "the rectangle")
	require.True(t, rep.ProfilesValid)

	require.Len(t, rep.Chains, 1, "the open tail")
	require.Empty(t, rep.InvalidChains)
	require.Equal(t, []sketch.Entity{tail}, rep.Chains[0].Entities)
	require.InDelta(t, 10, rep.Chains[0].Length, 1e-9)

	// The same answer Sketch.Chains gives, from the same arrangement.
	direct := s.Chains()
	require.Len(t, direct, len(rep.Chains))
	require.Equal(t, direct[0].Edges, rep.Chains[0].Edges)
	require.Equal(t, direct[0].Revision(), rep.Chains[0].Revision())
	require.False(t, rep.Chains[0].IsStale())
}

// TestVerifyChainsDoNotGateTheVerdict pins the deliberate omission: an open run
// is reported, never asserted. A fully constrained sketch whose geometry is one
// open chain passes Check, and a caller that must not sweep a bad chain reads
// Chain.Valid on the chain it is about to use.
func TestVerifyChainsDoNotGateTheVerdict(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20, 0)
	line := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewHorizontalDistance(a, b, 20))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context())
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.Len(t, rep.Chains, 1, "the sketch is one open chain")
	require.Empty(t, rep.Profiles, "and no closed region")
	require.True(t, rep.ProfilesValid, "vacuously: an open sketch has no regions")
	require.NoError(t, rep.Check(), "an open chain is not a defect")
	require.True(t, rep.Trustworthy())
}

// TestVerifyListsAnInvalidChain covers the reporting half. The fixture's walk
// doubles back over itself, which is also a collinear overlap in the
// arrangement, so the verdict fails on the degeneracy through ProfilesValid —
// the chain listing adds no condition of its own.
func TestVerifyListsAnInvalidChain(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.CreateLine(a, b)
	s.CreateLine(b, s.CreatePoint(2, 0))

	rep := s.Verify(t.Context())
	require.Len(t, rep.Chains, 1)
	require.Len(t, rep.InvalidChains, 1, "the self-touching walk is listed")
	require.Same(t, rep.Chains[0], rep.InvalidChains[0], "InvalidChains is a subset of Chains")
	require.True(t, rep.Chains[0].SelfIntersecting)

	require.False(t, rep.ProfilesValid, "the overlap makes the arrangement unresolvable")
	require.ErrorIs(t, rep.Check(), sketch.ErrInvalidProfile,
		"the verdict fails on the arrangement, which is the condition Check owns")
}

// TestVerifySkippedPathLeavesChainsNil pins that Chains carries the same
// unevaluated zero value every other analysis field carries when Verify stops at
// the non-finite screen: nil means the pass never ran, not that the sketch has
// no open geometry.
func TestVerifySkippedPathLeavesChainsNil(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.CreateLine(a, b)
	require.Len(t, s.Chains(), 1, "an open chain before the geometry is poisoned")

	s.CreatePoint(math.NaN(), math.NaN()) // a stray non-finite point
	rep := s.Verify(t.Context())
	require.False(t, rep.Analysed(), "the non-finite screen stops the analysis")
	require.Nil(t, rep.Chains, "never ran, so nothing is claimed")
	require.Nil(t, rep.InvalidChains)
}
