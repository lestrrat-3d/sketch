package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestRegionsClosedSplineBoundsRegion(t *testing.T) {
	// A square-ish closed spline bounds one region with positive sampled area
	// (smaller than the 4x4 control hull since the loop is inset).
	pts := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(4, 0), geom.NewPoint(4, 4), geom.NewPoint(0, 4)}
	sp, err := geom.NewClosedSpline(pts...)
	require.NoError(t, err)
	arr := geom.Regions(nil, []geom.ClosedCurve{sp})
	require.Len(t, arr.Regions, 1, "a closed spline bounds one region")
	require.Greater(t, arr.Regions[0].Area, 0.0)
	require.Less(t, arr.Regions[0].Area, 16.0)
	require.False(t, arr.Regions[0].SelfIntersecting)
	require.Empty(t, arr.SelfIntersections)
}

func TestRegionsClosedSplineFigureEight(t *testing.T) {
	// A control polygon that makes the periodic loop cross itself (a figure-8).
	pts := []*geom.Point{
		geom.NewPoint(0, 0), geom.NewPoint(4, 3), geom.NewPoint(0, 3), geom.NewPoint(4, 0),
	}
	sp, err := geom.NewClosedSpline(pts...)
	require.NoError(t, err)
	arr := geom.Regions(nil, []geom.ClosedCurve{sp})
	require.NotEmpty(t, arr.SelfIntersections, "the periodic loop crosses itself")
}

func TestEvalPeriodicCubicBSplineClosure(t *testing.T) {
	pts := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(4, 0), geom.NewPoint(4, 4), geom.NewPoint(0, 4)}
	sp, err := geom.NewClosedSpline(pts...)
	require.NoError(t, err)
	x0, y0 := sp.Eval(0)
	x1, y1 := sp.Eval(1)
	require.InDelta(t, x0, x1, 1e-12, "Eval(0) == Eval(1): periodic closure")
	require.InDelta(t, y0, y1, 1e-12)
	// reducing t modulo 1: Eval(2.5) == Eval(0.5)
	xa, ya := sp.Eval(0.5)
	xb, yb := sp.Eval(2.5)
	require.InDelta(t, xa, xb, 1e-12)
	require.InDelta(t, ya, yb, 1e-12)
	ring := sp.Polyline(48)
	require.Equal(t, ring[0], ring[len(ring)-1], "the sampled ring closes")
}

func TestEvalPeriodicCubicBSplineNonFiniteParam(t *testing.T) {
	// A parameter the modulo-1 reduction cannot place must answer NaN rather than
	// index the control list out of range: reducing a NaN leaves a NaN, and
	// reducing an infinity produces one. Every closed-spline entry point.
	ctrl := [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}}
	pts := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(4, 0), geom.NewPoint(4, 4), geom.NewPoint(0, 4)}
	sp, err := geom.NewClosedSpline(pts...)
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		t    float64
	}{
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(1)},
		{"-Inf", math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				x, y := sp.Eval(tc.t)
				require.True(t, math.IsNaN(x), "ClosedSpline.Eval x")
				require.True(t, math.IsNaN(y), "ClosedSpline.Eval y")
			})
			require.NotPanics(t, func() {
				x, y, err := geom.EvalPeriodicCubicBSpline(ctrl, tc.t)
				require.NoError(t, err, "a placeable-parameter failure is not the too-few-points error")
				require.True(t, math.IsNaN(x), "EvalPeriodicCubicBSpline x")
				require.True(t, math.IsNaN(y), "EvalPeriodicCubicBSpline y")
			})
			require.NotPanics(t, func() {
				dx, dy, err := geom.EvalPeriodicCubicBSplineDeriv(ctrl, tc.t)
				require.NoError(t, err)
				require.True(t, math.IsNaN(dx), "EvalPeriodicCubicBSplineDeriv dx")
				require.True(t, math.IsNaN(dy), "EvalPeriodicCubicBSplineDeriv dy")
			})
			// The control-count guard still runs before the reduction, so a short
			// control list reports its own error rather than evaluating.
			_, _, err := geom.EvalPeriodicCubicBSpline([][2]float64{{0, 0}, {1, 0}}, tc.t)
			require.ErrorIs(t, err, geom.ErrTooFewClosedControlPoints)
			_, _, err = geom.EvalPeriodicCubicBSplineDeriv([][2]float64{{0, 0}, {1, 0}}, tc.t)
			require.ErrorIs(t, err, geom.ErrTooFewClosedControlPoints)
		})
	}
}

func TestEvalPeriodicCubicBSplineFiniteParamValues(t *testing.T) {
	// Concrete curve and tangent values across the parameter range — inside the
	// first span, at each span boundary, at the seam and beyond it, and at a
	// negative parameter — so a change to the modulo-1 reduction or to the span
	// index moves a number here rather than passing quietly.
	square := [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}}
	for _, tc := range []struct {
		name         string
		t            float64
		x, y, dx, dy float64
	}{
		{"seam", 0, 10.0 / 3, 2.0 / 3, 8, 8},
		{"inside first span", 0.125, 23.0 / 6, 2, 0, 12},
		{"span boundary", 0.25, 10.0 / 3, 10.0 / 3, -8, 8},
		{"half turn", 0.5, 2.0 / 3, 10.0 / 3, -8, -8},
		{"three quarters", 0.75, 2.0 / 3, 2.0 / 3, 8, -8},
		{"one full turn", 1, 10.0 / 3, 2.0 / 3, 8, 8},
		{"beyond the seam", 1.25, 10.0 / 3, 10.0 / 3, -8, 8},
		{"negative", -0.25, 2.0 / 3, 2.0 / 3, 8, -8},
		{"two and a half turns", 2.5, 2.0 / 3, 10.0 / 3, -8, -8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, y, err := geom.EvalPeriodicCubicBSpline(square, tc.t)
			require.NoError(t, err)
			require.InDelta(t, tc.x, x, 1e-12)
			require.InDelta(t, tc.y, y, 1e-12)
			dx, dy, err := geom.EvalPeriodicCubicBSplineDeriv(square, tc.t)
			require.NoError(t, err)
			require.InDelta(t, tc.dx, dx, 1e-12)
			require.InDelta(t, tc.dy, dy, 1e-12)
		})
	}

	// An odd control count with no symmetry, so a wrong span index cannot land on
	// an equal value by luck.
	five := [][2]float64{{0, 0}, {3, -1}, {5, 2}, {2, 5}, {-1, 3}}
	for _, tc := range []struct {
		name         string
		t            float64
		x, y, dx, dy float64
	}{
		{"first span", 0.1, 3.875, 0.5833333333333334, 7.5, 12.5},
		{"third span", 0.42, 1.7006666666666665, 4.192333333333334, -14.900000000000002, 0.09999999999999787},
		{"last span", 0.9, 1.5208333333333335, -0.375, 13.125, -3.75},
	} {
		t.Run("five/"+tc.name, func(t *testing.T) {
			x, y, err := geom.EvalPeriodicCubicBSpline(five, tc.t)
			require.NoError(t, err)
			require.InDelta(t, tc.x, x, 1e-12)
			require.InDelta(t, tc.y, y, 1e-12)
			dx, dy, err := geom.EvalPeriodicCubicBSplineDeriv(five, tc.t)
			require.NoError(t, err)
			require.InDelta(t, tc.dx, dx, 1e-12)
			require.InDelta(t, tc.dy, dy, 1e-12)
		})
	}

	// Where the whole-turn shift is exact in binary the two agree bit for bit.
	x0, y0, err := geom.EvalPeriodicCubicBSpline(five, 0.25)
	require.NoError(t, err)
	x1, y1, err := geom.EvalPeriodicCubicBSpline(five, 2.25)
	require.NoError(t, err)
	require.Equal(t, x0, x1)
	require.Equal(t, y0, y1)
}

func TestNewClosedSplineMinThree(t *testing.T) {
	_, err := geom.NewClosedSpline(geom.NewPoint(0, 0), geom.NewPoint(1, 0))
	require.ErrorIs(t, err, geom.ErrTooFewClosedControlPoints)
	_, err = geom.NewClosedSpline(geom.NewPoint(0, 0), geom.NewPoint(2, 0), geom.NewPoint(1, 2))
	require.NoError(t, err, "three control points are enough for a closed cubic")
}
