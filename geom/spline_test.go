package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// evalSpline / derivSpline / nearestSpline wrap the cubic B-spline kernels,
// failing the test on the (unreachable, given valid control points) error.
func evalSpline(t *testing.T, ctrl [][2]float64, tp float64) (float64, float64) {
	x, y, err := geom.EvalCubicBSpline(ctrl, tp)
	require.NoError(t, err)
	return x, y
}

func derivSpline(t *testing.T, ctrl [][2]float64, tp float64) (float64, float64) {
	dx, dy, err := geom.EvalCubicBSplineDeriv(ctrl, tp)
	require.NoError(t, err)
	return dx, dy
}

func nearestSpline(t *testing.T, ctrl [][2]float64, px, py float64) float64 {
	tp, err := geom.NearestParamCubicBSpline(ctrl, px, py)
	require.NoError(t, err)
	return tp
}

func TestSplineEndpoints(t *testing.T) {
	sp, err := geom.NewSpline(
		geom.NewPoint(0, 0), geom.NewPoint(2, 5), geom.NewPoint(8, 5),
		geom.NewPoint(10, 0), geom.NewPoint(12, -3),
	)
	require.NoError(t, err)
	x, y := sp.Eval(0)
	require.InDelta(t, 0, x, 1e-12, "start x")
	require.InDelta(t, 0, y, 1e-12, "start y")
	x, y = sp.Eval(1)
	require.InDelta(t, 12, x, 1e-12, "end x (t=1 shortcut)")
	require.InDelta(t, -3, y, 1e-12, "end y")
}

func TestSplineBezierOracle(t *testing.T) {
	// A clamped cubic B-spline with exactly 4 control points IS the cubic
	// Bézier over the same points.
	p0 := [2]float64{0, 0}
	p1 := [2]float64{0, 3}
	p2 := [2]float64{3, 3}
	p3 := [2]float64{3, 0}
	sp, err := geom.NewSpline(
		geom.NewPoint(p0[0], p0[1]), geom.NewPoint(p1[0], p1[1]),
		geom.NewPoint(p2[0], p2[1]), geom.NewPoint(p3[0], p3[1]),
	)
	require.NoError(t, err)
	bezier := func(a, b, c, d, t float64) float64 {
		u := 1 - t
		return u*u*u*a + 3*u*u*t*b + 3*u*t*t*c + t*t*t*d
	}
	for _, tt := range []float64{0.1, 0.25, 0.5, 0.75, 0.9} {
		x, y := sp.Eval(tt)
		require.InDeltaf(t, bezier(p0[0], p1[0], p2[0], p3[0], tt), x, 1e-12, "x at t=%v", tt)
		require.InDeltaf(t, bezier(p0[1], p1[1], p2[1], p3[1], tt), y, 1e-12, "y at t=%v", tt)
	}
}

func TestSplineSymmetry(t *testing.T) {
	// Control polygon symmetric about x=5: the curve midpoint lies on the
	// symmetry axis.
	sp, err := geom.NewSpline(
		geom.NewPoint(0, 0), geom.NewPoint(2, 4), geom.NewPoint(8, 4), geom.NewPoint(10, 0),
	)
	require.NoError(t, err)
	x, _ := sp.Eval(0.5)
	require.InDelta(t, 5, x, 1e-12, "midpoint on symmetry axis")
}

func TestSplineKnots(t *testing.T) {
	require.Equal(t, []float64{0, 0, 0, 0, 1, 1, 1, 1}, geom.ClampedKnots(4), "4 points: Bézier knots")
	require.Equal(t, []float64{0, 0, 0, 0, 0.5, 1, 1, 1, 1}, geom.ClampedKnots(5), "5 points: one interior knot")
	k6 := geom.ClampedKnots(6)
	require.InDelta(t, 1.0/3, k6[4], 1e-12, "6 points: first interior knot")
	require.InDelta(t, 2.0/3, k6[5], 1e-12, "6 points: second interior knot")
}

func TestSplineTooFewControlPoints(t *testing.T) {
	_, err := geom.NewSpline(geom.NewPoint(0, 0), geom.NewPoint(1, 1), geom.NewPoint(2, 2))
	require.ErrorIs(t, err, geom.ErrTooFewControlPoints, "needs 4 control points")
}

func TestNearestParamCubicBSpline(t *testing.T) {
	ctrl := [][2]float64{{0, 0}, {2, 4}, {8, 4}, {10, 0}}
	// A point exactly on the curve recovers its own parameter.
	for _, want := range []float64{0, 0.25, 0.5, 0.75, 1} {
		x, y := evalSpline(t, ctrl, want)
		require.InDelta(t, want, nearestSpline(t, ctrl, x, y), 1e-3,
			"recover the parameter of an on-curve point at t=%v", want)
	}
	// A point off the curve projects to (at least) the nearest sampled point.
	tp := nearestSpline(t, ctrl, 5, 10)
	px, py := evalSpline(t, ctrl, tp)
	best := math.Inf(1)
	for i := 0; i <= 200; i++ {
		qx, qy := evalSpline(t, ctrl, float64(i)/200)
		if d := math.Hypot(5-qx, 10-qy); d < best {
			best = d
		}
	}
	require.LessOrEqual(t, math.Hypot(5-px, 10-py), best+1e-9, "foot is the nearest point")
}

func TestEvalCubicBSplineDeriv(t *testing.T) {
	ctrl := [][2]float64{{0, 0}, {2, 4}, {8, 4}, {10, 0}, {12, -3}}
	// Interior: analytic derivative matches a central finite difference of Eval.
	for _, tp := range []float64{0.15, 0.37, 0.5, 0.62, 0.84} {
		const h = 1e-6
		ax, ay := evalSpline(t, ctrl, tp-h)
		bx, by := evalSpline(t, ctrl, tp+h)
		wantX, wantY := (bx-ax)/(2*h), (by-ay)/(2*h)
		gotX, gotY := derivSpline(t, ctrl, tp)
		require.InDelta(t, wantX, gotX, 1e-3, "dx at t=%v", tp)
		require.InDelta(t, wantY, gotY, 1e-3, "dy at t=%v", tp)
	}
	// Endpoints: one-sided tangent along the first/last control legs.
	d0x, d0y := derivSpline(t, ctrl, 0)
	require.InDelta(t, 0, d0x*(ctrl[1][1]-ctrl[0][1])-d0y*(ctrl[1][0]-ctrl[0][0]), 1e-9,
		"start tangent parallel to first leg")
	require.Greater(t, d0x, 0.0, "start tangent points into the curve")
	n := len(ctrl)
	d1x, d1y := derivSpline(t, ctrl, 1)
	require.InDelta(t, 0, d1x*(ctrl[n-1][1]-ctrl[n-2][1])-d1y*(ctrl[n-1][0]-ctrl[n-2][0]), 1e-9,
		"end tangent parallel to last leg (t=1 shortcut, not (0,0))")
	require.Greater(t, math.Hypot(d1x, d1y), 0.0, "end tangent is nonzero")
}

func TestSplinePolyline(t *testing.T) {
	sp, err := geom.NewSpline(
		geom.NewPoint(0, 0), geom.NewPoint(2, 4), geom.NewPoint(8, 4), geom.NewPoint(10, 0),
	)
	require.NoError(t, err)
	pts := sp.Polyline(16)
	require.Len(t, pts, 17, "segments+1 samples")
	require.InDelta(t, 0, math.Hypot(pts[0][0], pts[0][1]), 1e-12, "starts at first control point")
	require.InDelta(t, 10, pts[16][0], 1e-12, "ends at last control point")
}
