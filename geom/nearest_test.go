package geom_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestNearestParamPeriodicCubicBSpline(t *testing.T) {
	ctrl := [][2]float64{{0, 0}, {4, 0}, {5, 3}, {2, 5}, {-1, 3}}
	for _, want := range []float64{0.13, 0.37, 0.62, 0.88} {
		x, y := geom.EvalPeriodicCubicBSpline(ctrl, want)
		got := geom.NearestParamPeriodicCubicBSpline(ctrl, x, y)
		require.InDeltaf(t, want, got, 5e-3, "nearest param of an on-curve point at t=%v", want)
	}
	// A point off the curve still returns a valid parameter in [0,1).
	got := geom.NearestParamPeriodicCubicBSpline(ctrl, 2, 2)
	require.GreaterOrEqual(t, got, 0.0)
	require.Less(t, got, 1.0)
}

func TestNearestParamFitSpline(t *testing.T) {
	fit := [][2]float64{{0, 0}, {2, 3}, {6, 3}, {8, 0}}
	for _, want := range []float64{0.1, 0.35, 0.6, 0.9} {
		x, y := geom.EvalFitSpline(fit, want)
		got := geom.NearestParamFitSpline(fit, x, y)
		require.InDeltaf(t, want, got, 5e-3, "nearest param of an on-curve point at t=%v", want)
	}
	// A point past the end seeds near the [0,1] endpoint.
	require.InDelta(t, 1.0, geom.NearestParamFitSpline(fit, 100, -50), 1e-3, "past the end → t≈1")
}
