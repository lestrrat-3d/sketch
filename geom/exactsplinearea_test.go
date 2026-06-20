package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// closedShoelace returns the absolute area of the closed polygon through poly
// (the closing edge from last back to first is implied). It is the dense-polyline
// reference the exact spline area must converge to.
func closedShoelace(poly [][2]float64) float64 {
	var s float64
	n := len(poly)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		s += poly[i][0]*poly[j][1] - poly[j][0]*poly[i][1]
	}
	return math.Abs(s / 2)
}

var closedCtrl = [][2]float64{{0, 0}, {4, 0}, {5, 3}, {2, 5}, {-1, 3}}

// A closed cubic B-spline's region area is the exact ½∫(x·y′−y·x′) of its
// piecewise cubic — independent of arrangement sampling density (the old sampled
// bulge moved with it) and equal to the dense-polyline reference.
func TestExactClosedSplineArea(t *testing.T) {
	ctrl := []*geom.Point{
		geom.NewPoint(0, 0), geom.NewPoint(4, 0), geom.NewPoint(5, 3),
		geom.NewPoint(2, 5), geom.NewPoint(-1, 3),
	}
	cs, err := geom.NewClosedSpline(ctrl...)
	require.NoError(t, err)

	coarse := geom.Regions(nil, []geom.ClosedCurve{cs}, geom.WithSegmentsPerTurn(12))
	fine := geom.Regions(nil, []geom.ClosedCurve{cs}, geom.WithSegmentsPerTurn(500))
	require.Len(t, coarse.Regions, 1)
	require.Len(t, fine.Regions, 1)
	// Exact ⇒ sampling-independent: coarse and fine arrangements give one area.
	require.InDelta(t, coarse.Regions[0].Area, fine.Regions[0].Area, 1e-9)

	// SamplePeriodicCubicBSpline repeats the first point as the last; drop it so
	// the shoelace's implied closing edge is not a zero-length duplicate.
	ring := geom.SamplePeriodicCubicBSpline(closedCtrl, 200000)
	ref := closedShoelace(ring[:len(ring)-1])
	require.InDelta(t, ref, coarse.Regions[0].Area, 1e-6)
}

// Enclosed area is invariant under translating every control point.
func TestExactClosedSplineAreaTranslationInvariant(t *testing.T) {
	mk := func(dx, dy float64) *geom.ClosedSpline {
		cs, _ := geom.NewClosedSpline(
			geom.NewPoint(0+dx, 0+dy), geom.NewPoint(4+dx, 0+dy),
			geom.NewPoint(5+dx, 3+dy), geom.NewPoint(2+dx, 5+dy), geom.NewPoint(-1+dx, 3+dy),
		)
		return cs
	}
	a := geom.Regions(nil, []geom.ClosedCurve{mk(0, 0)})
	b := geom.Regions(nil, []geom.ClosedCurve{mk(100, -50)})
	require.Len(t, a.Regions, 1)
	require.Len(t, b.Regions, 1)
	require.InDelta(t, a.Regions[0].Area, b.Regions[0].Area, 1e-9)
}

// An open cubic B-spline closed by a straight chord encloses an exact region.
func TestExactOpenSplineProfileArea(t *testing.T) {
	p0 := geom.NewPoint(0, 0)
	p1 := geom.NewPoint(1, 2)
	p2 := geom.NewPoint(3, 2)
	p3 := geom.NewPoint(4, 0)
	sp, err := geom.NewSpline(p0, p1, p2, p3)
	require.NoError(t, err)
	chord := geom.NewLine(p3, p0)

	coarse := geom.Regions([]geom.Curve{sp, chord}, nil, geom.WithSegmentsPerTurn(8))
	fine := geom.Regions([]geom.Curve{sp, chord}, nil, geom.WithSegmentsPerTurn(500))
	require.Len(t, coarse.Regions, 1)
	require.Len(t, fine.Regions, 1)
	require.InDelta(t, coarse.Regions[0].Area, fine.Regions[0].Area, 1e-9)

	ctrl := [][2]float64{{0, 0}, {1, 2}, {3, 2}, {4, 0}}
	ref := closedShoelace(geom.SampleCubicBSpline(ctrl, 200000))
	require.InDelta(t, ref, coarse.Regions[0].Area, 1e-6)
}

// A fit-point (interpolating) spline closed by a straight chord encloses an
// exact region.
func TestExactFitSplineProfileArea(t *testing.T) {
	f0 := geom.NewPoint(0, 0)
	f1 := geom.NewPoint(1, 1.5)
	f2 := geom.NewPoint(3, 1.2)
	f3 := geom.NewPoint(5, 0)
	fs, err := geom.NewFitSpline(f0, f1, f2, f3)
	require.NoError(t, err)
	chord := geom.NewLine(f3, f0)

	coarse := geom.Regions([]geom.Curve{fs, chord}, nil, geom.WithSegmentsPerTurn(8))
	fine := geom.Regions([]geom.Curve{fs, chord}, nil, geom.WithSegmentsPerTurn(500))
	require.Len(t, coarse.Regions, 1)
	require.Len(t, fine.Regions, 1)
	require.InDelta(t, coarse.Regions[0].Area, fine.Regions[0].Area, 1e-9)

	fit := [][2]float64{{0, 0}, {1, 1.5}, {3, 1.2}, {5, 0}}
	ref := closedShoelace(geom.SampleFitSpline(fit, 200000))
	require.InDelta(t, ref, coarse.Regions[0].Area, 1e-6)
}

// A closed spline nested inside an outer rectangle becomes a hole: the annulus's
// net area is rect − spline and the inner region is the spline area. This pins the
// sign of the spline bulge on a CLOCKWISE-walked fragment (the hole boundary,
// pStart>pEnd → the sign=−1 branch of curveMoment) — a wrong sign would corrupt
// the subtracted hole area.
func TestExactSplineHoleArea(t *testing.T) {
	a := geom.NewPoint(-3, -3)
	b := geom.NewPoint(8, -3)
	c := geom.NewPoint(8, 8)
	d := geom.NewPoint(-3, 8)
	rect := []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
	cs, err := geom.NewClosedSpline(
		geom.NewPoint(0, 0), geom.NewPoint(4, 0), geom.NewPoint(5, 3),
		geom.NewPoint(2, 5), geom.NewPoint(-1, 3),
	)
	require.NoError(t, err)

	arr := geom.Regions(rect, []geom.ClosedCurve{cs})
	require.Len(t, arr.Regions, 2, "annulus + inner spline region")

	ring := geom.SamplePeriodicCubicBSpline(closedCtrl, 200000)
	splineArea := closedShoelace(ring[:len(ring)-1])

	var annulus, inner *geom.Region
	for _, r := range arr.Regions {
		if len(r.Holes) == 1 {
			annulus = r
		} else {
			inner = r
		}
	}
	require.NotNil(t, annulus, "the rectangle carries one spline hole")
	require.NotNil(t, inner, "the spline interior is a separate region")
	require.InDelta(t, 11*11-splineArea, annulus.Area, 1e-6, "annulus = rect − spline")
	require.InDelta(t, splineArea, inner.Area, 1e-6, "inner = spline area")
}

// The new analytic periodic-spline derivative must match a central finite
// difference of the position evaluator (parameters chosen off the span
// boundaries i/n). The fit-spline derivative is exercised by the fit area test.
func TestPeriodicSplineDeriv(t *testing.T) {
	const h = 1e-6
	for _, tp := range []float64{0.07, 0.25, 0.5, 0.73, 0.95} {
		dx, dy := geom.EvalPeriodicCubicBSplineDeriv(closedCtrl, tp)
		x0, y0 := geom.EvalPeriodicCubicBSpline(closedCtrl, tp-h)
		x1, y1 := geom.EvalPeriodicCubicBSpline(closedCtrl, tp+h)
		require.InDelta(t, (x1-x0)/(2*h), dx, 1e-4)
		require.InDelta(t, (y1-y0)/(2*h), dy, 1e-4)
	}
}
