package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// This file is the geom half of the performance-measurement harness in
// .tmp/perf-plan.md, section 1.2. It adds ONLY benchmarks and fixture
// builders — no production code changes. The curve fixtures here are also
// used by arrange_golden_test.go's TestGoldenCurves (same package), and
// circles12Fixture/splineLoopsFixture by its TestGoldenArrangement.

// circlePoints returns n points on a circle of radius r centered at (cx, cy).
func circlePoints(cx, cy, r float64, n int) []*geom.Point {
	pts := make([]*geom.Point, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = geom.NewPoint(cx+r*math.Cos(a), cy+r*math.Sin(a))
	}
	return pts
}

// benchSplineControl returns 8 control points laid out as a gentle wave,
// shared by the open Spline and NURBS fixtures below.
func benchSplineControl() []*geom.Point {
	pts := make([]*geom.Point, 8)
	for i := range pts {
		x := float64(i) * 10
		y := 10 * math.Sin(float64(i)*0.9)
		pts[i] = geom.NewPoint(x, y)
	}
	return pts
}

// benchSplineFixture returns an open cubic B-spline over 8 control points.
func benchSplineFixture(tb testing.TB) *geom.Spline {
	tb.Helper()
	sp, err := geom.NewSpline(benchSplineControl()...)
	require.NoError(tb, err)
	return sp
}

// benchClosedSplineFixture returns a closed (periodic) cubic B-spline over 8
// roughly-circular control points.
func benchClosedSplineFixture(tb testing.TB) *geom.ClosedSpline {
	tb.Helper()
	sp, err := geom.NewClosedSpline(circlePoints(0, 0, 20, 8)...)
	require.NoError(tb, err)
	return sp
}

// benchFitSplineFixture returns a fit (interpolating) spline through 8 fit
// points laid out as a gentle wave.
func benchFitSplineFixture(tb testing.TB) *geom.FitSpline {
	tb.Helper()
	pts := make([]*geom.Point, 8)
	for i := range pts {
		pts[i] = geom.NewPoint(float64(i)*10, 8*math.Sin(float64(i)*1.1))
	}
	sp, err := geom.NewFitSpline(pts...)
	require.NoError(tb, err)
	return sp
}

// benchNURBSFixture returns a degree-3 rational NURBS over 8 control points
// with non-unit weights and a clamped uniform knot vector.
func benchNURBSFixture() *geom.NURBS {
	const degree = 3
	control := benchSplineControl()
	weights := make([]float64, len(control))
	for i := range weights {
		weights[i] = 1 + 0.5*float64(i%3)
	}
	knots := geom.ClampedUniformKnots(len(control), degree)
	return geom.NewNURBS(degree, control, knots, weights)
}

// benchEllipseFixture returns a rotated ellipse, rx=20 ry=10.
func benchEllipseFixture() *geom.Ellipse {
	return geom.NewEllipse(geom.NewPoint(0, 0), 20, 10, 0.4)
}

// benchEllipticalArcFixture returns a rotated elliptical arc (rx=20 ry=10)
// swept through 3/4 of the ellipse.
func benchEllipticalArcFixture() *geom.EllipticalArc {
	center := geom.NewPoint(0, 0)
	rx, ry, rot := 20.0, 10.0, 0.4
	cosr, sinr := math.Cos(rot), math.Sin(rot)
	pointAt := func(ecc float64) *geom.Point {
		lx, ly := rx*math.Cos(ecc), ry*math.Sin(ecc)
		return geom.NewPoint(center.X+cosr*lx-sinr*ly, center.Y+sinr*lx+cosr*ly)
	}
	return geom.NewEllipticalArc(center, pointAt(0), pointAt(3*math.Pi/2), rx, ry, rot)
}

func BenchmarkPolyline(b *testing.B) {
	b.Run("spline", func(b *testing.B) {
		sp := benchSplineFixture(b)
		b.ReportAllocs()
		for b.Loop() {
			sp.Polyline(128)
		}
	})
	b.Run("closedSpline", func(b *testing.B) {
		sp := benchClosedSplineFixture(b)
		b.ReportAllocs()
		for b.Loop() {
			sp.Polyline(128)
		}
	})
	b.Run("fitSpline", func(b *testing.B) {
		sp := benchFitSplineFixture(b)
		b.ReportAllocs()
		for b.Loop() {
			sp.Polyline(128)
		}
	})
	b.Run("nurbs", func(b *testing.B) {
		n := benchNURBSFixture()
		b.ReportAllocs()
		for b.Loop() {
			n.Polyline(128)
		}
	})
	b.Run("ellipse", func(b *testing.B) {
		e := benchEllipseFixture()
		b.ReportAllocs()
		for b.Loop() {
			e.Polyline(256)
		}
	})
	b.Run("ellipticalArc", func(b *testing.B) {
		e := benchEllipticalArcFixture()
		b.ReportAllocs()
		for b.Loop() {
			e.Polyline(256)
		}
	})
}

func BenchmarkRegions(b *testing.B) {
	b.Run("circles12", func(b *testing.B) {
		_, closed := circles12Fixture()
		b.ReportAllocs()
		for b.Loop() {
			geom.Regions(nil, closed)
		}
	})
	b.Run("splineLoops", func(b *testing.B) {
		curves, closed := splineLoopsFixture(b)
		b.ReportAllocs()
		for b.Loop() {
			geom.Regions(curves, closed)
		}
	})
}
