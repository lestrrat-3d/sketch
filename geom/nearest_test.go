package geom_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestNearestParamPeriodicCubicBSpline(t *testing.T) {
	ctrl := [][2]float64{{0, 0}, {4, 0}, {5, 3}, {2, 5}, {-1, 3}}
	for _, want := range []float64{0.13, 0.37, 0.62, 0.88} {
		x, y, err := geom.EvalPeriodicCubicBSpline(ctrl, want)
		require.NoError(t, err)
		got, err := geom.NearestParamPeriodicCubicBSpline(ctrl, x, y)
		require.NoError(t, err)
		require.InDeltaf(t, want, got, 5e-3, "nearest param of an on-curve point at t=%v", want)
	}
	// A point off the curve still returns a valid parameter in [0,1).
	got, err := geom.NearestParamPeriodicCubicBSpline(ctrl, 2, 2)
	require.NoError(t, err)
	require.GreaterOrEqual(t, got, 0.0)
	require.Less(t, got, 1.0)
}

func TestNearestParamFitSpline(t *testing.T) {
	fit := [][2]float64{{0, 0}, {2, 3}, {6, 3}, {8, 0}}
	for _, want := range []float64{0.1, 0.35, 0.6, 0.9} {
		x, y, err := geom.EvalFitSpline(fit, want)
		require.NoError(t, err)
		got, err := geom.NearestParamFitSpline(fit, x, y)
		require.NoError(t, err)
		require.InDeltaf(t, want, got, 5e-3, "nearest param of an on-curve point at t=%v", want)
	}
	// A point past the end seeds near the [0,1] endpoint.
	past, err := geom.NearestParamFitSpline(fit, 100, -50)
	require.NoError(t, err)
	require.InDelta(t, 1.0, past, 1e-3, "past the end → t≈1")
}

func TestNearestParamConic(t *testing.T) {
	start, apex, end := [2]float64{0, 0}, [2]float64{4, 6}, [2]float64{8, 0}
	c := geom.NewConic(
		&geom.Point{X: start[0], Y: start[1]},
		&geom.Point{X: apex[0], Y: apex[1]},
		&geom.Point{X: end[0], Y: end[1]},
		0.5,
	)
	for _, want := range []float64{0.1, 0.35, 0.6, 0.9} {
		x, y := c.Eval(want)
		got := geom.NearestParamConic(start, apex, end, 0.5, x, y)
		require.InDeltaf(t, want, got, 5e-3, "nearest param of an on-curve point at t=%v", want)
	}
	// A point past the end seeds near the [0,1] endpoint.
	require.InDelta(t, 1.0, geom.NearestParamConic(start, apex, end, 0.5, 100, -50), 1e-3, "past the end → t≈1")
}

func TestNearestParamNURBS(t *testing.T) {
	control := [][2]float64{{0, 0}, {4, 8}, {8, 0}}
	knots := geom.ClampedUniformKnots(3, 2)
	c := geom.NewNURBS(2, []*geom.Point{{X: 0, Y: 0}, {X: 4, Y: 8}, {X: 8, Y: 0}}, knots, nil)
	lo, hi := c.Domain()
	require.Equal(t, 0.0, lo)
	require.Equal(t, 1.0, hi)
	for _, want := range []float64{0.1, 0.35, 0.6, 0.9} {
		x, y := c.Eval(lo + want*(hi-lo))
		got := geom.NearestParamNURBS(control, 2, knots, nil, x, y)
		require.InDeltaf(t, want, got, 5e-3, "normalized nearest param of an on-curve point at t=%v", want)
	}
	// A point past the end seeds near the normalized [0,1] endpoint.
	require.InDelta(t, 1.0, geom.NearestParamNURBS(control, 2, knots, nil, 100, -50), 1e-3, "past the end → t≈1")
}
