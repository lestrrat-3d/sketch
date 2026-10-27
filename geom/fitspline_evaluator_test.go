package geom_test

import (
	"math"
	"sync"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestFitEvaluatorMatchesOneOffReads(t *testing.T) {
	t.Parallel()
	for sample, fit := range [][][2]float64{
		{{0, 0}, {2, 3}, {5, -1}, {7, 1}},
		{{4, 5}, {4, 5}, {4, 5}},
		{{0, 0}, {0, 0}, {2, 1}, {2, 1}, {5, -2}},
		{{math.Copysign(0, -1), 0}, {2, math.Copysign(0, -1)}},
		{{0, 0}, {math.NaN(), 2}, {3, 4}},
		{{0, 0}, {math.Inf(1), 2}},
		{{-4e307, 1}, {4e307, 0}, {-4e307, -1}},
	} {
		e, err := geom.NewFitEvaluator(fit)
		require.NoError(t, err)
		for _, parameter := range []float64{-1, 0, 0.125, 0.5, 0.999, 1, 2, math.Inf(-1), math.Inf(1), math.NaN()} {
			x, y := e.Eval(parameter)
			wx, wy, err := geom.EvalFitSpline(fit, parameter)
			require.NoError(t, err)
			dx, dy := e.EvalDeriv(parameter)
			wdx, wdy, err := geom.EvalFitSplineDeriv(fit, parameter)
			require.NoError(t, err)
			for coordinate, pair := range [4][2]float64{{x, wx}, {y, wy}, {dx, wdx}, {dy, wdy}} {
				require.Equal(t, math.Float64bits(pair[1]), math.Float64bits(pair[0]),
					"sample %d parameter %v coordinate %d", sample, parameter, coordinate)
			}
		}
	}
}

func TestFitEvaluatorOwnsItsSnapshotAndSharesReads(t *testing.T) {
	t.Parallel()
	fit := [][2]float64{{0, 0}, {2, 3}, {5, -1}, {7, 1}}
	e, err := geom.NewFitEvaluator(fit)
	require.NoError(t, err)
	x, y := e.Eval(0.375)
	dx, dy := e.EvalDeriv(0.375)
	fit[1] = [2]float64{100, -200}
	var group sync.WaitGroup
	failures := make(chan int, 8)
	for worker := range 8 {
		group.Go(func() {
			for range 32 {
				gx, gy := e.Eval(0.375)
				gdx, gdy := e.EvalDeriv(0.375)
				if gx != x || gy != y || gdx != dx || gdy != dy {
					failures <- worker
					return
				}
			}
		})
	}
	group.Wait()
	require.Empty(t, failures)
}

func TestFitEvaluatorEmptyAndTooFewPoints(t *testing.T) {
	t.Parallel()
	for _, fit := range [][][2]float64{nil, {{1, 2}}} {
		e, err := geom.NewFitEvaluator(fit)
		require.ErrorIs(t, err, geom.ErrTooFewFitPoints)
		require.Nil(t, e)
	}
	var zero geom.FitEvaluator
	x, y := zero.Eval(0.5)
	dx, dy := zero.EvalDeriv(0.5)
	require.Zero(t, x)
	require.Zero(t, y)
	require.Zero(t, dx)
	require.Zero(t, dy)
}
