package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Baseline, chained, and ordinate dimensions are not distinct constraint types in
// this engine — they are authoring patterns over the signed horizontal/vertical
// distance dimensions, all measured from a shared datum (baseline/ordinate) or
// end-to-end (chained). These tests pin that the oracle reports their solvability,
// DOF, redundancy and conflicts correctly, which is what makes a dedicated API
// unnecessary. See docs/verification-roadmap.md.

// TestBaselineDimensions: several features dimensioned from one datum along x.
func TestBaselineDimensions(t *testing.T) {
	s := newSketch(t)
	o := s.AddPoint(0, 0)
	s.Fix(o)
	a := s.AddPoint(3, 1)
	b := s.AddPoint(7, 2)
	s.AddConstraint(
		sketch.NewHorizontalDistance(o, a, 10), // baseline x of A
		sketch.NewHorizontalDistance(o, b, 25), // baseline x of B (same datum O)
	)
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 10, a.X()-o.X(), 1e-9)
	require.InDelta(t, 25, b.X()-o.X(), 1e-9)
	require.Equal(t, 2, s.DOF(), "only the x of each point is pinned; the two y's stay free")
	require.Empty(t, s.RedundantConstraints())
}

// TestOrdinateDimensions: x and y readout of one feature from a common origin.
func TestOrdinateDimensions(t *testing.T) {
	s := newSketch(t)
	o := s.AddPoint(0, 0)
	s.Fix(o)
	p := s.AddPoint(2, 2)
	s.AddConstraint(
		sketch.NewHorizontalDistance(o, p, 10), // ordinate x
		sketch.NewVerticalDistance(o, p, 6),    // ordinate y
	)
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 10, p.X()-o.X(), 1e-9)
	require.InDelta(t, 6, p.Y()-o.Y(), 1e-9)
	require.Equal(t, 0, s.DOF(), "x and y from the datum fully pin the feature")
}

// TestChainedDimensions: dimensions measured end-to-end accumulate.
func TestChainedDimensions(t *testing.T) {
	s := newSketch(t)
	o := s.AddPoint(0, 0)
	s.Fix(o)
	a := s.AddPoint(3, 0)
	b := s.AddPoint(9, 0)
	s.AddConstraint(
		sketch.NewHorizontalDistance(o, a, 10), // O→A
		sketch.NewHorizontalDistance(a, b, 15), // A→B (chained from A)
	)
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 10, a.X()-o.X(), 1e-9)
	require.InDelta(t, 25, b.X()-o.X(), 1e-9, "a chain O→A→B accumulates to O→B = 25")
}

// TestChainPlusBaselineConsistentIsRedundant: a chain O→A→B already determines
// O→B, so adding a consistent baseline O→B = 25 over-constrains (redundant, not
// conflicting). The oracle rejects it pre-commit and flags it post-commit.
func TestChainPlusBaselineConsistentIsRedundant(t *testing.T) {
	s := newSketch(t)
	o := s.AddPoint(0, 0)
	s.Fix(o)
	a := s.AddPoint(3, 0)
	b := s.AddPoint(9, 0)
	s.AddConstraint(
		sketch.NewHorizontalDistance(o, a, 10),
		sketch.NewHorizontalDistance(a, b, 15),
	)
	// O→B is determined by the chain (=25); a redundant baseline must be refused.
	baseline := sketch.NewHorizontalDistance(o, b, 25)
	require.ErrorIs(t, s.CheckConstraint(baseline), sketch.ErrOverconstrained)

	// Commit it anyway: consistent, so still solvable, but reported redundant.
	s.AddConstraint(baseline)
	_, err := s.Solve()
	require.NoError(t, err)
	require.True(t, s.Verify().Solvable)
	require.Contains(t, s.Diagnose().Redundant, sketch.Constraint(baseline),
		"the later baseline duplicates the chain")
	require.Empty(t, s.Diagnose().Conflicting)
}

// TestChainPlusBaselineConflicting: a baseline O→B = 30 contradicts the chain's
// O→B = 25. The oracle refuses it pre-commit and, if committed, reports the
// sketch unsolvable with the baseline blamed against the chain.
func TestChainPlusBaselineConflicting(t *testing.T) {
	s := newSketch(t)
	o := s.AddPoint(0, 0)
	s.Fix(o)
	a := s.AddPoint(3, 0)
	b := s.AddPoint(9, 0)
	oa := sketch.NewHorizontalDistance(o, a, 10)
	ab := sketch.NewHorizontalDistance(a, b, 15)
	s.AddConstraint(oa, ab)

	baseline := sketch.NewHorizontalDistance(o, b, 30) // contradicts O→B = 25
	require.ErrorIs(t, s.CheckConstraint(baseline), sketch.ErrOverconstrained)

	s.AddConstraint(baseline)
	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	rep := s.Verify()
	require.False(t, rep.Solvable)
	require.NotEmpty(t, rep.Conflicts)
	// the later baseline is the conflicting constraint, blamed against the chain
	var found bool
	for _, cs := range rep.Conflicts {
		if cs.Constraint == sketch.Constraint(baseline) {
			found = true
			require.NotEmpty(t, cs.With, "blamed against the earlier chain dimensions")
		}
	}
	require.True(t, found, "the baseline is reported as the conflicting constraint")
}

// TestDrivenOrdinateReadout: a driven (reference) ordinate dimension measures the
// geometry without constraining it — it must not change DOF, add redundancy, or
// conflict with a driving dimension over the same span, and refreshes its target.
func TestDrivenOrdinateReadout(t *testing.T) {
	s := newSketch(t)
	o := s.AddPoint(0, 0)
	s.Fix(o)
	p := s.AddPoint(2, 2)
	s.AddConstraint(
		sketch.NewHorizontalDistance(o, p, 10), // driving
		sketch.NewVerticalDistance(o, p, 6),    // driving
	)
	// a driven ordinate readout over the same x span — would be redundant if it
	// drove, but driven dimensions contribute no rows, so it must not be flagged.
	readout := sketch.NewHorizontalDistance(o, p, 0)
	readout.SetDriven(true)
	s.AddConstraint(readout)

	_, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF(), "the driven readout adds no constraint rows")
	require.Empty(t, s.Diagnose().Redundant, "a driven readout is never redundant")
	require.Empty(t, s.Diagnose().Conflicting)
	require.InDelta(t, 10, readout.Target().Base(), 1e-9, "the readout measured O→P x = 10")
}
