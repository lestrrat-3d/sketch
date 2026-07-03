package sketch_test

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// distanceSketch builds a deliberately unsolved two-point sketch: a grounded
// origin and a free point whose distance to it must reach 10 (it starts at
// ~7.07), so at least one solver iteration is required to converge.
func distanceSketch(t *testing.T) (*sketch.Sketch, *sketch.Point) {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(5, 5)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	return s, b
}

func TestSolveContext(t *testing.T) {
	t.Run("live context converges normally", func(t *testing.T) {
		s, _ := distanceSketch(t)
		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.True(t, res.Converged)
	})

	t.Run("already-cancelled context aborts before solving", func(t *testing.T) {
		s, b := distanceSketch(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		bx, by := b.X(), b.Y()
		res, err := s.Solve(ctx)
		require.ErrorIs(t, err, context.Canceled)
		require.False(t, res.Converged)
		// Aborted before any iteration ran, so the geometry never moved.
		require.Equal(t, 0, res.Iterations)
		require.Equal(t, bx, b.X())
		require.Equal(t, by, b.Y())
		// DOF/rank analysis was skipped; the fields are the "not computed"
		// sentinel, never a misleading 0 that reads as fully constrained.
		require.Equal(t, -1, res.DOF)
		require.Equal(t, -1, res.Redundant)
	})

	t.Run("expired deadline reports DeadlineExceeded", func(t *testing.T) {
		s, _ := distanceSketch(t)
		ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
		defer cancel()

		res, err := s.Solve(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.False(t, res.Converged)
	})
}
