package geom_test

import (
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

func TestNewClosedSplineMinThree(t *testing.T) {
	_, err := geom.NewClosedSpline(geom.NewPoint(0, 0), geom.NewPoint(1, 0))
	require.ErrorIs(t, err, geom.ErrTooFewClosedControlPoints)
	_, err = geom.NewClosedSpline(geom.NewPoint(0, 0), geom.NewPoint(2, 0), geom.NewPoint(1, 2))
	require.NoError(t, err, "three control points are enough for a closed cubic")
}
