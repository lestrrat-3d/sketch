package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestRankMarginHealthy(t *testing.T) {
	// A point pinned by orthogonal horizontal + vertical distances: the rank
	// decision rests on well-separated, O(1) pivots.
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(3, 1)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontalDistance(a, b, 10))
	s.AddConstraint(sketch.NewVerticalDistance(a, b, 5))
	_, err := s.Solve()
	require.NoError(t, err)

	rep := s.Verify()
	require.Equal(t, 0, rep.DOF)
	require.Greater(t, rep.RankMargin, 1e6, "orthogonal constraints decide rank far from the cutoff")
	require.True(t, rep.Trustworthy())
}

func TestRankMarginNearSingularFlagged(t *testing.T) {
	// A free point pinned onto two nearly-parallel lines (≈1e-6 rad apart) that
	// cross at the origin. It is solvable and DOF 0 with no redundancy/conflict, so
	// the structural verdict is FullyConstrained — but the two on-line constraints
	// are nearly linearly dependent. The advisory RankMargin reports the
	// near-threshold rank decision (a hint), and the scale-invariant Conditioning
	// gate now refuses to bless it: Trustworthy is false because the constraint
	// system is numerically near-singular, even though nothing structural is wrong.
	s := newSketch(t)
	o1 := s.AddPoint(0, 0)
	e1 := s.AddPoint(1, 0)
	o2 := s.AddPoint(0, 0)
	e2 := s.AddPoint(1, 1e-6) // ≈1e-6 rad from line 1
	for _, p := range []*sketch.Point{o1, e1, o2, e2} {
		s.Fix(p)
	}
	l1 := s.AddLine(o1, e1)
	l2 := s.AddLine(o2, e2)
	p := s.AddPoint(0, 0) // the intersection
	s.AddConstraint(sketch.NewPointOnLine(p, l1))
	s.AddConstraint(sketch.NewPointOnLine(p, l2))
	_, err := s.Solve()
	require.NoError(t, err)

	rep := s.Verify()
	require.Equal(t, 0, rep.DOF, "the intersection is determined")
	require.Empty(t, rep.Redundant, "not structurally redundant")
	require.Empty(t, rep.Conflicts)
	require.True(t, rep.Solvable)
	require.Equal(t, sketch.FullyConstrained, rep.Status, "structurally fully constrained")
	require.Less(t, rep.RankMargin, 1e4, "the advisory margin flags the near-singular rank decision")
	require.Less(t, rep.Conditioning, 1e-6, "the scale-invariant conditioning measure is near-singular")
	require.False(t, rep.Trustworthy(), "the conditioning gate refuses to bless a near-singular system")
}

func TestRankMarginExactRedundancyNotRediscovered(t *testing.T) {
	// A point constrained onto the same fixed line twice: structurally redundant
	// (DOF 1, one redundant constraint), caught by the existing rank/redundancy
	// machinery. The rank-margin signal is about MARGINAL pivots, not exact
	// redundancy, so it should report well-separated (the accepted pivot is O(1)).
	s := newSketch(t)
	o := s.AddPoint(0, 0)
	e := s.AddPoint(10, 0)
	s.Fix(o)
	s.Fix(e)
	l := s.AddLine(o, e)
	p := s.AddPoint(4, 0)
	s.AddConstraint(sketch.NewPointOnLine(p, l))
	s.AddConstraint(sketch.NewPointOnLine(p, l)) // exact duplicate
	_, err := s.Solve()
	require.NoError(t, err)

	rep := s.Verify()
	require.NotEmpty(t, rep.Redundant, "the duplicate is caught by redundancy analysis")
	require.Greater(t, rep.RankMargin, 1e6, "exact redundancy is an O(1) pivot, not a near-threshold one")
}
