package sketch_test

import (
	"context"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// horizontalBarSketch is fully constrained (DOF 0): a grounded origin and a
// second point placed 10 to the right by a distance + horizontal pair. The two
// solutions (±10) make it a legitimate target for the ambiguity probe.
func horizontalBarSketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(7, 3)
	ab := s.CreateLine(a, b)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.AddConstraint(sketch.NewHorizontal(ab))
	return s
}

func TestProbeConfigurationsContext(t *testing.T) {
	s := horizontalBarSketch(t)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	t.Run("already-cancelled context aborts the probe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := s.ProbeConfigurations(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("live context probes normally", func(t *testing.T) {
		pr, err := s.ProbeConfigurations(t.Context())
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(pr.Configurations), 1)
	})

	t.Run("cancellation after the baseline solve reports the context error", func(t *testing.T) {
		// An under-constrained sketch (DOF 1: one free point pinned only by a
		// distance to the grounded origin, authored already satisfying it) makes
		// the probe's post-baseline precondition return ErrUnderconstrained — a
		// non-context verdict. A context that goes done only once every earlier
		// poll has passed exercises the guard added after the baseline rank/DOF
		// pass: it must surface the cancellation, not the ErrUnderconstrained
		// precondition. Without the guard the probe never polls ctx again after
		// the rank pass, so it returns ErrUnderconstrained and the trip never fires.
		u := newSketch(t)
		a := u.CreatePoint(0, 0)
		b := u.CreatePoint(10, 0) // already 10 from a, so lm converges at iteration 0
		a.MoveTo(0, 0)
		u.Fix(a)
		u.AddConstraint(sketch.NewDistance(a, b, 10))
		fc := &tripContext{tripAfterN: probeBaselineRankTripCount}

		_, err := u.ProbeConfigurations(fc)
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, sketch.ErrUnderconstrained)
		require.True(t, fc.tripped, "context never reached the post-baseline-rank guard")
	})
}

// probeBaselineRankTripCount is the number of live ctx.Err() polls the probe of
// an already-converged, DOF-1 sketch makes before the guard that follows the
// baseline rank/DOF pass: the entry check, lm's entry + first-iteration checks,
// and the post-lm check are the first four; the post-rank guard is the fifth. If
// the probe's pre-DOF poll cadence changes this test fails loudly (the guard is
// never reached and ErrUnderconstrained leaks through), which is the intended
// tripwire.
const probeBaselineRankTripCount = 5

func TestVerifyContext(t *testing.T) {
	s := horizontalBarSketch(t)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	t.Run("cancelled context discards the probe but still reports", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rep := s.Verify(ctx, sketch.WithProbe())
		// Verify has no error result: it still returns a populated report, but
		// the requested probe could not run, so it is marked incomplete and must
		// not be blessed as trustworthy (the ambiguity check never happened).
		require.Nil(t, rep.Probe)
		require.True(t, rep.ProbeIncomplete)
		require.False(t, rep.Trustworthy())
		require.Equal(t, 0, rep.DOF)
		require.True(t, rep.Solvable)
	})

	t.Run("cancelled context without WithProbe is not marked incomplete", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rep := s.Verify(ctx)
		// No probe was requested, so cancellation does not make the report
		// incomplete on the probe axis.
		require.False(t, rep.ProbeIncomplete)
	})

	t.Run("live context runs the probe", func(t *testing.T) {
		rep := s.Verify(t.Context(), sketch.WithProbe())
		require.NotNil(t, rep.Probe)
	})
}
