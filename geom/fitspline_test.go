package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestRegionsFitSplineWithChord(t *testing.T) {
	a := geom.NewPoint(0, 0)
	m1 := geom.NewPoint(2, 3)
	m2 := geom.NewPoint(4, 3)
	b := geom.NewPoint(6, 0)
	fs, err := geom.NewFitSpline(a, m1, m2, b)
	require.NoError(t, err)
	chord := geom.NewLine(b, a)
	arr := geom.Regions([]geom.Curve{fs, chord}, nil)
	require.Len(t, arr.Regions, 1, "fit spline + chord bound one region")
	require.Greater(t, arr.Regions[0].Area, 0.0)
	require.False(t, arr.Regions[0].SelfIntersecting)
	require.Empty(t, arr.SelfIntersections)
}

func TestRegionsFitSplineSelfCrossing(t *testing.T) {
	// Fit points whose natural-cubic interpolant loops across itself, closed by a chord.
	a := geom.NewPoint(0, 0)
	m1 := geom.NewPoint(4, 1)
	m2 := geom.NewPoint(0, 2)
	m3 := geom.NewPoint(4, 3)
	fs, err := geom.NewFitSpline(a, m1, m2, m3)
	require.NoError(t, err)
	chord := geom.NewLine(m3, a)
	arr := geom.Regions([]geom.Curve{fs, chord}, nil)
	require.NotEmpty(t, arr.SelfIntersections, "the interpolant weaves across itself")
}

func TestFitSplineInterpolatesEval(t *testing.T) {
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1), geom.NewPoint(5, 1), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	var cum []float64
	var total float64
	cum = append(cum, 0)
	for i := 1; i < len(fit); i++ {
		total += math.Hypot(fit[i].X-fit[i-1].X, fit[i].Y-fit[i-1].Y)
		cum = append(cum, total)
	}
	for i, p := range fit {
		x, y := sp.Eval(cum[i] / total)
		require.InDelta(t, p.X, x, 1e-9, "interpolates fit point %d", i)
		require.InDelta(t, p.Y, y, 1e-9)
	}
}

func TestFitSplineTwoPointsIsLine(t *testing.T) {
	sp, err := geom.NewFitSpline(geom.NewPoint(0, 0), geom.NewPoint(6, 3))
	require.NoError(t, err)
	x, y := sp.Eval(0.5)
	require.InDelta(t, 3, x, 1e-9, "two fit points evaluate as a straight line")
	require.InDelta(t, 1.5, y, 1e-9)
}

func TestNewFitSplineMinTwo(t *testing.T) {
	_, err := geom.NewFitSpline(geom.NewPoint(0, 0))
	require.ErrorIs(t, err, geom.ErrTooFewFitPoints)
}
