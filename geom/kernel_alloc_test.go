package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// This file pins the allocation contract from .tmp/perf-plan.md item D1: the
// open cubic B-spline and NURBS evaluation kernels build their clamped knot
// vector / basis-function scratch on the stack up to a size limit, and only
// fall back to the heap above it. It also exercises the heap-fallback path
// for correctness, not just for its allocation count.

// waveControl returns n control coordinates laid out as a gentle wave, used
// wherever the test only needs a plausible (non-degenerate) open spline.
func waveControl(n int) [][2]float64 {
	ctrl := make([][2]float64, n)
	for i := range ctrl {
		ctrl[i] = [2]float64{float64(i) * 3, 5 * math.Sin(float64(i)*0.7)}
	}
	return ctrl
}

// collinearControl returns n control coordinates evenly spaced along the line
// y = m*x + b. A cubic B-spline's basis functions form a partition of unity
// at every parameter, so the curve point is an affine combination of the
// control points and — for collinear controls — lands exactly on the line
// regardless of degree, knot spacing, or control count. That invariant is
// this file's independent oracle for the heap-fallback path: it does not
// re-derive the recursion under test, it checks a geometric fact the
// recursion must satisfy.
func collinearControl(n int, m, b float64) [][2]float64 {
	ctrl := make([][2]float64, n)
	for i := range ctrl {
		x := float64(i)
		ctrl[i] = [2]float64{x, m*x + b}
	}
	return ctrl
}

// TestEvalCubicBSplineAllocFree pins D1's stack-buffer path: at n = 60
// control points (n+4 == 64, the documented stack knot-buffer size) a single
// evaluation allocates nothing.
func TestEvalCubicBSplineAllocFree(t *testing.T) {
	ctrl := waveControl(60)
	_, _, err := geom.EvalCubicBSpline(ctrl, 0.37)
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(100, func() {
		_, _, _ = geom.EvalCubicBSpline(ctrl, 0.37)
	})
	require.Equal(t, float64(0), allocs)
}

// TestNURBSEvalAllocFree pins D1's stack-buffer path: at degree 15
// (3*(degree+1) == 48, the documented stack basis-buffer size) a single
// evaluation allocates nothing.
func TestNURBSEvalAllocFree(t *testing.T) {
	const degree = 15
	control := make([]*geom.Point, degree+5)
	for i := range control {
		control[i] = geom.NewPoint(float64(i)*2, 3*math.Sin(float64(i)*0.5))
	}
	knots := geom.ClampedUniformKnots(len(control), degree)
	require.NotNil(t, knots)
	c := geom.NewNURBS(degree, control, knots, nil)

	x, y := c.Eval(0.42)
	require.False(t, math.IsNaN(x) || math.IsNaN(y))

	allocs := testing.AllocsPerRun(100, func() {
		c.Eval(0.42)
	})
	require.Equal(t, float64(0), allocs)
}

// TestSampleCubicBSplineAllocatesOutputOnly pins D1's win for the common
// path: once the knot vector is built once per sampling call instead of once
// per sample, the only allocation left is the returned point slice. This
// targets the kernel entry point directly (as NURBS.Polyline does — it reads
// *Point control coordinates with no separate copy step); geom.Spline's
// Polyline wrapper additionally copies its *Point control points into
// [][2]float64 via controlCoords on every call, an unrelated, pre-existing
// allocation outside D1's scope (the knot vector, not the control points).
func TestSampleCubicBSplineAllocatesOutputOnly(t *testing.T) {
	ctrl := waveControl(8)

	allocs := testing.AllocsPerRun(100, func() {
		_, _ = geom.SampleCubicBSpline(ctrl, 128)
	})
	require.Equal(t, float64(1), allocs)
}

// TestNURBSPolylineAllocatesOutputOnly pins the same win for NURBS.Polyline,
// which inherits Eval's alloc-free basis evaluation.
func TestNURBSPolylineAllocatesOutputOnly(t *testing.T) {
	const degree = 3
	control := make([]*geom.Point, 8)
	weights := make([]float64, 8)
	for i := range control {
		control[i] = geom.NewPoint(float64(i)*10, 10*math.Sin(float64(i)*0.9))
		weights[i] = 1 + 0.5*float64(i%3)
	}
	knots := geom.ClampedUniformKnots(len(control), degree)
	require.NotNil(t, knots)
	c := geom.NewNURBS(degree, control, knots, weights)

	allocs := testing.AllocsPerRun(100, func() {
		c.Polyline(128)
	})
	require.Equal(t, float64(1), allocs)
}

// TestEvalCubicBSplineHeapFallbackCorrect exercises n = 100 control points
// (n+4 = 104, above the 64-float stack knot buffer) so EvalCubicBSpline must
// fall back to a heap-allocated knot vector. The control points are
// collinear, so correctness is checked against the partition-of-unity
// invariant (see collinearControl) rather than by re-deriving the recursion.
func TestEvalCubicBSplineHeapFallbackCorrect(t *testing.T) {
	const n = 100
	const m, b = 2.0, 3.0
	ctrl := collinearControl(n, m, b)

	x0, y0, err := geom.EvalCubicBSpline(ctrl, 0)
	require.NoError(t, err)
	require.InDelta(t, ctrl[0][0], x0, 1e-9)
	require.InDelta(t, ctrl[0][1], y0, 1e-9)

	x1, y1, err := geom.EvalCubicBSpline(ctrl, 1)
	require.NoError(t, err)
	require.InDelta(t, ctrl[n-1][0], x1, 1e-9)
	require.InDelta(t, ctrl[n-1][1], y1, 1e-9)

	for _, tp := range []float64{0.05, 0.13, 0.37, 0.5, 0.77, 0.94} {
		x, y, err := geom.EvalCubicBSpline(ctrl, tp)
		require.NoError(t, err)
		require.InDelta(t, m*x+b, y, 1e-9, "t=%v off the control line", tp)
	}
}

// TestNURBSEvalHeapFallbackCorrect exercises degree 16 (3*(degree+1) = 51 for
// Eval, 5*(degree+1) = 85 for EvalDeriv, both above the 48-float stack basis
// buffer) so both Eval and EvalDeriv must fall back to heap-allocated
// scratch. The control points are collinear and non-rational (weights nil),
// so Eval must land exactly on the line (partition of unity) and EvalDeriv's
// tangent must point along it.
func TestNURBSEvalHeapFallbackCorrect(t *testing.T) {
	const degree = 16
	const m, b = 5.0, -2.0
	coords := collinearControl(degree+10, m, b)
	control := make([]*geom.Point, len(coords))
	for i, xy := range coords {
		control[i] = geom.NewPoint(xy[0], xy[1])
	}
	knots := geom.ClampedUniformKnots(len(control), degree)
	require.NotNil(t, knots)
	c := geom.NewNURBS(degree, control, knots, nil)

	for _, u := range []float64{0, 0.1, 0.33, 0.5, 0.68, 0.9, 1} {
		x, y := c.Eval(u)
		require.InDelta(t, m*x+b, y, 1e-9, "u=%v off the control line", u)

		dx, dy := c.EvalDeriv(u)
		require.InDelta(t, m*dx, dy, 1e-6, "u=%v tangent not along the control line", u)
	}
}
