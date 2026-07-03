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

	t.Run("cancellation during the final rank pass is honored", func(t *testing.T) {
		// The rank/DOF pass runs after the solve converges. A context that goes
		// done only once every earlier check has passed exercises the guard that
		// re-checks ctx after that pass — otherwise a deadline expiring there would
		// leak through as a normal (non-context) result.
		//
		// The sketch is authored already satisfying its constraint so lm converges
		// at iteration 0 with a fixed, minimal poll cadence: the Solve entry check,
		// lm's own entry + first-iteration checks, and the post-solve pre-rank
		// check are the four live polls; the fifth — the guard added after the rank
		// pass — is the one that trips.
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10, 0) // already 10 from a, so the distance holds at the seed
		a.MoveTo(0, 0)
		s.Fix(a)
		s.AddConstraint(sketch.NewDistance(a, b, 10))
		fc := &tripContext{tripAfterN: rankPassTripCount}

		res, err := s.Solve(fc)
		require.ErrorIs(t, err, context.Canceled)
		require.True(t, fc.tripped, "context never reached the post-rank check")
		// Converged is set before the post-rank guard, so its being true proves the
		// solve finished and the trip landed at the guard — not on an earlier lm
		// poll (which would leave Converged false).
		require.True(t, res.Converged, "trip did not reach the post-rank guard")
		// The post-rank guard still reports the cancellation with the not-computed
		// sentinels rather than a normal DOF result.
		require.Equal(t, -1, res.DOF)
		require.Equal(t, -1, res.Redundant)
	})
}

// rankPassTripCount is the number of live ctx.Err() polls an already-converged
// single-distance solve makes before the post-rank guard: the Solve entry
// check, lm's entry + first-iteration checks, and the post-solve pre-rank check
// are the first four; the guard added after the rank pass is the fifth. If the
// solver's pre-rank check cadence changes this test fails loudly (the trip lands
// on an earlier poll, leaving Converged false), which is the intended tripwire.
const rankPassTripCount = 5

// tripContext is a context.Context that stays live until Err() has been polled
// tripAfterN times, then reports context.Canceled. It lets a test drive
// cancellation to a specific internal phase of a synchronous Solve without a
// wall-clock race.
type tripContext struct {
	tripAfterN int
	calls      int
	tripped    bool
}

func (c *tripContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *tripContext) Done() <-chan struct{}       { return nil }
func (c *tripContext) Value(any) any               { return nil }

func (c *tripContext) Err() error {
	c.calls++
	if c.calls >= c.tripAfterN {
		c.tripped = true
		return context.Canceled
	}
	return nil
}
