package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestDiagnoseClean(t *testing.T) {
	s, _, _, _, _ := newRectangle(t)
	mustSolve(t, s)
	d := s.Diagnose()
	require.Empty(t, d.Redundant, "clean sketch has no redundancy")
	require.Empty(t, d.Conflicting, "clean sketch has no conflicts")
}

func TestDiagnoseRedundant(t *testing.T) {
	s, _, b, _, _ := newRectangle(t)
	a := s.Points()[0]
	dup := addDist(s, a, b, 20) // consistent duplicate of the width dimension
	mustSolve(t, s)

	d := s.Diagnose()
	require.Len(t, d.Redundant, 1, "the duplicate is redundant")
	require.Same(t, dup, d.Redundant[0], "creation order: the later duplicate is reported")
	require.Empty(t, d.Conflicting, "a satisfied duplicate is not a conflict")
}

func TestDiagnoseConflicting(t *testing.T) {
	s, _, b, _, _ := newRectangle(t)
	a := s.Points()[0]
	conflict := addDist(s, a, b, 25) // fights the width-20 dimension

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "contradictory dimensions cannot converge")

	d := s.Diagnose()
	require.Len(t, d.Conflicting, 1, "the contradiction is identified")
	require.Same(t, conflict, d.Conflicting[0], "creation order: the later dimension is blamed")
	require.Empty(t, d.Redundant, "a violated dependent constraint is not merely redundant")
}

func TestCheckConstraint(t *testing.T) {
	t.Run("rejects a duplicate", func(t *testing.T) {
		s, _, b, _, _ := newRectangle(t)
		mustSolve(t, s)
		a := s.Points()[0]
		nCons := len(s.Constraints())

		err := s.CheckConstraint(sketch.NewDistance(a, b, 20))
		require.ErrorIs(t, err, sketch.ErrOverconstrained, "consistent duplicate rejected")
		require.Len(t, s.Constraints(), nCons, "check commits nothing")
	})
	t.Run("rejects a contradiction", func(t *testing.T) {
		s, _, b, _, _ := newRectangle(t)
		mustSolve(t, s)
		a := s.Points()[0]

		err := s.CheckConstraint(sketch.NewDistance(a, b, 25))
		require.ErrorIs(t, err, sketch.ErrOverconstrained, "contradiction rejected")
		// The sketch is untouched and still solves to its dimensions.
		mustSolve(t, s)
		require.InDelta(t, 20, b.X(), 1e-6, "geometry unaffected by the check")
	})
	t.Run("rejects a constraint between grounded points", func(t *testing.T) {
		s := sketch.New()
		a := addPt(s, 0, 0)
		b := addPt(s, 3, 0)
		s.Fix(a)
		s.Fix(b)
		err := s.CheckConstraint(sketch.NewDistance(a, b, 3))
		require.ErrorIs(t, err, sketch.ErrOverconstrained, "no free variable to constrain")
	})
	t.Run("accepts an independent constraint", func(t *testing.T) {
		s := sketch.New()
		a := addPt(s, 0, 0)
		s.Fix(a)
		b := addPt(s, 4, 1)
		require.NoError(t, s.CheckConstraint(sketch.NewDistance(a, b, 5)), "first dimension is independent")
	})
	t.Run("accepts a driven dimension anywhere", func(t *testing.T) {
		s, _, _, c, _ := newRectangle(t)
		mustSolve(t, s)
		a := s.Points()[0]
		diag := sketch.NewDistance(a, c, 0)
		diag.SetDriven(true)
		require.NoError(t, s.CheckConstraint(diag), "driven dimensions never constrain")
	})
}

func TestFreePoints(t *testing.T) {
	s, w, b, c, d := newRectangle(t)
	mustSolve(t, s)
	a := s.Points()[0]

	require.Empty(t, s.FreePoints(), "fully constrained sketch has no free points")
	require.True(t, a.IsFullyConstrained(), "grounded corner")
	require.True(t, b.IsFullyConstrained(), "b pinned by width + horizontal")

	// Dropping the width dimension frees exactly the corners that can slide:
	// b and c can move in x together; d stays pinned by the height dimension
	// and the vertical side.
	s.RemoveConstraint(w)
	free := s.FreePoints()
	require.Equal(t, []*sketch.Point{b, c}, free, "exactly the sliding corners are free")
	require.False(t, b.IsFullyConstrained(), "b can slide")
	require.True(t, d.IsFullyConstrained(), "d still pinned")
	require.Equal(t, 1, s.DOF(), "one remaining freedom, shared by b and c")
}
