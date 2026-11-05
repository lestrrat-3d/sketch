package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestRankMarginHealthy(t *testing.T) {
	// A point pinned by orthogonal horizontal + vertical distances: the rank
	// decision rests on well-separated, O(1) pivots.
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(3, 1)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontalDistance(a, b, 10))
	s.AddConstraint(sketch.NewVerticalDistance(a, b, 5))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context())
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
	o1 := s.CreatePoint(0, 0)
	e1 := s.CreatePoint(1, 0)
	o2 := s.CreatePoint(0, 0)
	e2 := s.CreatePoint(1, 1e-6) // ≈1e-6 rad from line 1
	for _, p := range []*sketch.Point{o1, e1, o2, e2} {
		s.Fix(p)
	}
	l1 := s.CreateLine(o1, e1)
	l2 := s.CreateLine(o2, e2)
	p := s.CreatePoint(0, 0) // the intersection
	s.AddConstraint(sketch.NewPointOnLine(p, l1))
	s.AddConstraint(sketch.NewPointOnLine(p, l2))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context())
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
	o := s.CreatePoint(0, 0)
	e := s.CreatePoint(10, 0)
	s.Fix(o)
	s.Fix(e)
	l := s.CreateLine(o, e)
	p := s.CreatePoint(4, 0)
	s.AddConstraint(sketch.NewPointOnLine(p, l))
	s.AddConstraint(sketch.NewPointOnLine(p, l)) // exact duplicate
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context())
	require.NotEmpty(t, rep.Redundant, "the duplicate is caught by redundancy analysis")
	require.Greater(t, rep.RankMargin, 1e6, "exact redundancy is an O(1) pivot, not a near-threshold one")
}

// The rank/DOF/redundancy/free-point analyses now run on the nondimensional
// Jacobian, so their verdicts are scale- and unit-invariant. These tests pin that:
// the same construction at 1×, 1000×, and 25.4× (inch-equivalent) reports the same
// DOF, redundant/conflicting constraints, and free points.

var rankScales = []float64{1, 1000, 25.4}

func TestRankScaleInvariantMixedUnit(t *testing.T) {
	// A free point determined by a length distance + a dimensionless angle (mixed
	// row units) — exactly where a raw absolute pivot threshold would drift.
	for _, k := range rankScales {
		rep := mixedUnitFixture(k)
		require.Equalf(t, 0, rep.DOF, "k=%v", k)
		require.Emptyf(t, rep.Redundant, "k=%v", k)
		require.Emptyf(t, rep.Conflicts, "k=%v", k)
		require.Emptyf(t, rep.FreePoints, "k=%v", k)
		require.Truef(t, rep.Trustworthy(), "k=%v", k)
	}
}

func TestRankScaleInvariantRedundant(t *testing.T) {
	// A grounded segment with a duplicate length dimension: the later one is
	// redundant at every scale.
	build := func(k float64) *sketch.VerificationReport {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10*k, 0)
		s.Fix(a)
		s.AddConstraint(sketch.NewHorizontalPoints(a, b))
		s.AddConstraint(sketch.NewDistance(a, b, 10*k))
		s.AddConstraint(sketch.NewDistance(a, b, 10*k)) // exact duplicate → redundant
		s.Solve(t.Context())
		return s.Verify(t.Context())
	}
	for _, k := range rankScales {
		rep := build(k)
		require.Lenf(t, rep.Redundant, 1, "one redundant duplicate at k=%v", k)
		require.Emptyf(t, rep.Conflicts, "k=%v", k)
		require.Equalf(t, 0, rep.DOF, "k=%v", k)
	}
}

func TestRankScaleInvariantConflicting(t *testing.T) {
	// Two contradictory length dimensions on the same grounded segment: a conflict
	// at every scale, attributed the same way.
	build := func(k float64) *sketch.VerificationReport {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10*k, 0)
		s.Fix(a)
		s.AddConstraint(sketch.NewHorizontalPoints(a, b))
		s.AddConstraint(sketch.NewDistance(a, b, 10*k))
		s.AddConstraint(sketch.NewDistance(a, b, 7*k)) // contradicts the first
		s.Solve(t.Context())
		return s.Verify(t.Context())
	}
	for _, k := range rankScales {
		rep := build(k)
		require.Lenf(t, rep.Conflicts, 1, "one conflict at k=%v", k)
		require.NotEmptyf(t, rep.Conflicts[0].With, "conflict has an attribution set at k=%v", k)
	}
}

func TestRankScaleInvariantUnderconstrained(t *testing.T) {
	// A point pinned only by a distance to a fixed center keeps one DOF (free on a
	// circle); the same point is reported free at every scale.
	build := func(k float64) (*sketch.VerificationReport, int) {
		s := newSketch(t)
		o := s.CreatePoint(0, 0)
		s.Fix(o)
		p := s.CreatePoint(3*k, 4*k)
		s.AddConstraint(sketch.NewDistance(o, p, 5*k))
		s.Solve(t.Context())
		return s.Verify(t.Context()), p.ID()
	}
	for _, k := range rankScales {
		rep, pid := build(k)
		require.Equalf(t, 1, rep.DOF, "one sliding DOF at k=%v", k)
		require.Lenf(t, rep.FreePoints, 1, "k=%v", k)
		require.Equalf(t, pid, rep.FreePoints[0].ID(), "the same point is free at k=%v", k)
	}
}

func TestRankScaleInvariantAuxHeavy(t *testing.T) {
	// A point-on-arc (a slack-bearing aux constraint) plus a line pinning it,
	// exercising the aux-var column scales in the rank path. Whatever the verdict
	// is (this construction has a benign aux redundancy), it must be IDENTICAL at
	// every scale — that is the scale-invariance property.
	build := func(k float64) *sketch.VerificationReport {
		s := newSketch(t)
		o, a, b := s.CreatePoint(0, 0), s.CreatePoint(5*k, 0), s.CreatePoint(0, 5*k)
		s.Fix(o)
		s.Fix(a)
		s.Fix(b)
		arc := s.CreateArc(o, a, b)
		p := s.CreatePoint(4*k, 3*k)
		diag := s.CreatePoint(5*k, 5*k)
		s.Fix(diag)
		s.AddConstraint(sketch.NewPointOnArc(p, arc))
		s.AddConstraint(sketch.NewPointOnLine(p, s.CreateLine(o, diag)))
		s.Solve(t.Context())
		return s.Verify(t.Context())
	}
	base := build(1)
	require.Equal(t, 0, base.DOF)
	require.False(t, math.IsNaN(base.Conditioning))
	for _, k := range rankScales[1:] {
		rep := build(k)
		require.Equalf(t, base.DOF, rep.DOF, "DOF invariant at k=%v", k)
		require.Lenf(t, rep.Redundant, len(base.Redundant), "redundancy count invariant at k=%v", k)
		require.Lenf(t, rep.Conflicts, len(base.Conflicts), "conflict count invariant at k=%v", k)
		require.InEpsilonf(t, base.Conditioning, rep.Conditioning, 1e-9, "Conditioning invariant at k=%v", k)
	}
}

func TestCheckConstraintScaleInvariant(t *testing.T) {
	// CheckConstraint's augmented-rank over-constraint test is scale-invariant: a
	// non-over-constraining addition is accepted, and a duplicate rejected, at every
	// scale.
	for _, k := range rankScales {
		s := newSketch(t)
		o := s.CreatePoint(0, 0)
		s.Fix(o)
		p := s.CreatePoint(3*k, 4*k)
		first := sketch.NewDistance(o, p, 5*k)
		require.NoErrorf(t, s.CheckConstraint(first), "first distance accepted at k=%v", k)
		s.AddConstraint(first)
		s.Solve(t.Context())
		// A second, independent distance from a different fixed point is fine.
		q := s.CreatePoint(10*k, 0)
		s.Fix(q)
		require.NoErrorf(t, s.CheckConstraint(sketch.NewDistance(q, p, 5*k)), "independent distance accepted at k=%v", k)
		// An exact duplicate of the first is over-constraining.
		require.ErrorIsf(t, s.CheckConstraint(sketch.NewDistance(o, p, 5*k)), sketch.ErrOverconstrained, "duplicate rejected at k=%v", k)
	}
}

func TestCheckConstraintConicTranslationInvariant(t *testing.T) {
	// Regression: CheckConstraint's augmented-rank test must reject a DUPLICATE
	// tangent-conic at any translation. The candidate's witness coordinates are
	// length-kind positions; if they are not centered for the FD pass, the
	// augmented rank is linearized at inconsistent coordinates far from the origin
	// and the duplicate slips through (a false acceptance, an over-constrain the
	// oracle must catch).
	build := func(off float64) error {
		s := newSketch(t)
		ec := s.CreatePoint(off, off)
		e := s.CreateEllipse(ec, 6, 3, 0)
		cc := s.CreatePoint(off+9, off)
		ci := s.CreateCircle(cc, 2)
		s.Fix(ec)
		s.Fix(cc)
		s.AddConstraint(
			sketch.NewSemiMajor(e, 6), sketch.NewSemiMinor(e, 3),
			sketch.NewEllipseRotation(e, 0), sketch.NewRadius(ci, 2),
		)
		s.AddConstraint(sketch.NewTangentEllipseCircular(e, ci, false))
		s.Solve(t.Context())
		return s.CheckConstraint(sketch.NewTangentEllipseCircular(e, ci, false))
	}
	for _, off := range []float64{0, 1000} {
		require.ErrorIsf(t, build(off), sketch.ErrOverconstrained,
			"a duplicate tangent-conic is over-constraining at offset %v", off)
	}
}
