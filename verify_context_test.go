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
}

func TestVerifyContext(t *testing.T) {
	s := horizontalBarSketch(t)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	t.Run("cancelled context discards the probe but still reports", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rep := s.Verify(ctx, sketch.WithProbe())
		// Verify has no error result: it still returns a populated report, but
		// the bounded probe contributed nothing.
		require.Nil(t, rep.Probe)
		require.Equal(t, 0, rep.DOF)
		require.True(t, rep.Solvable)
	})

	t.Run("live context runs the probe", func(t *testing.T) {
		rep := s.Verify(t.Context(), sketch.WithProbe())
		require.NotNil(t, rep.Probe)
	})
}
