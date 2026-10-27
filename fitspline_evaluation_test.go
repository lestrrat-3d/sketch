package sketch_test

import (
	"math"
	"sync"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestFitSplineEvaluationFollowsFitEdits(t *testing.T) {
	t.Parallel()
	s := newSketch(t)
	a, middle, b := s.CreatePoint(0, 0), s.CreatePoint(3, 2), s.CreatePoint(6, 0)
	sp, err := s.CreateFitSpline(a, middle, b)
	require.NoError(t, err)
	check := func() {
		t.Helper()
		fit := make([][2]float64, len(sp.Fit))
		for i, p := range sp.Fit {
			fit[i] = [2]float64{p.X(), p.Y()}
		}
		for i := range 17 {
			parameter := float64(i) / 16
			x, y := sp.Eval(parameter)
			wx, wy, err := geom.EvalFitSpline(fit, parameter)
			if len(fit) < 2 {
				require.ErrorIs(t, err, geom.ErrTooFewFitPoints)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, math.Float64bits(wx), math.Float64bits(x), "parameter %v x", parameter)
			require.Equal(t, math.Float64bits(wy), math.Float64bits(y), "parameter %v y", parameter)
		}
	}
	check()
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewVerticalDistance(a, middle, 5))
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 5, middle.Y(), 1e-6)
	check()
	sp.Fit[1] = s.CreatePoint(3, -4)
	check()
	sp.Fit = append(sp.Fit, s.CreatePoint(8, 3))
	check()
	sp.Fit[0], sp.Fit[1] = sp.Fit[1], sp.Fit[0]
	check()
	sp.Fit = sp.Fit[:1]
	check()
	sp.Fit = nil
	check()
}

func TestFitSplineConcurrentEvaluation(t *testing.T) {
	t.Parallel()
	s := newSketch(t)
	sp, err := s.CreateFitSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 3), s.CreatePoint(5, -1))
	require.NoError(t, err)
	wx, wy, err := geom.EvalFitSpline([][2]float64{{0, 0}, {2, 3}, {5, -1}}, 0.375)
	require.NoError(t, err)
	var group sync.WaitGroup
	failures := make(chan int, 8)
	for worker := range 8 {
		group.Go(func() {
			for range 32 {
				x, y := sp.Eval(0.375)
				if x != wx || y != wy {
					failures <- worker
					return
				}
			}
		})
	}
	group.Wait()
	require.Empty(t, failures)
}
