package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestConicEvalEndpoints(t *testing.T) {
	c := geom.NewConic(geom.NewPoint(1, 2), geom.NewPoint(4, 6), geom.NewPoint(7, 1), 0.6)
	x0, y0 := c.Eval(0)
	require.InDelta(t, 1, x0, 1e-12)
	require.InDelta(t, 2, y0, 1e-12)
	x1, y1 := c.Eval(1)
	require.InDelta(t, 7, x1, 1e-12)
	require.InDelta(t, 1, y1, 1e-12)
}

func TestConicEndpointTangents(t *testing.T) {
	// The conic is tangent to Start→Apex at t=0 and to Apex→End at t=1: the
	// analytic derivative there is parallel to those chords (cross product zero).
	start := geom.NewPoint(0, 0)
	apex := geom.NewPoint(2, 5)
	end := geom.NewPoint(6, 0)
	for _, rho := range []float64{0.3, 0.5, 0.8} {
		c := geom.NewConic(start, apex, end, rho)
		dx0, dy0 := c.EvalDeriv(0)
		// Start→Apex direction.
		sx, sy := apex.X-start.X, apex.Y-start.Y
		require.InDeltaf(t, 0, dx0*sy-dy0*sx, 1e-9, "rho %v start tangent", rho)
		dx1, dy1 := c.EvalDeriv(1)
		// Apex→End direction.
		ex, ey := end.X-apex.X, end.Y-apex.Y
		require.InDeltaf(t, 0, dx1*ey-dy1*ex, 1e-9, "rho %v end tangent", rho)
	}
}

func TestConicParabolaMidpoint(t *testing.T) {
	// A parabola (rho = 0.5, w = 1) has its midpoint at the control-triangle's
	// 1/4 Start + 1/2 Apex + 1/4 End.
	start := geom.NewPoint(-2, 1)
	apex := geom.NewPoint(3, 7)
	end := geom.NewPoint(5, -1)
	c := geom.NewConic(start, apex, end, 0.5)
	mx, my := c.Eval(0.5)
	wantX := 0.25*start.X + 0.5*apex.X + 0.25*end.X
	wantY := 0.25*start.Y + 0.5*apex.Y + 0.25*end.Y
	require.InDelta(t, wantX, mx, 1e-12)
	require.InDelta(t, wantY, my, 1e-12)
}

func TestConicDerivNumeric(t *testing.T) {
	// The analytic EvalDeriv matches a central finite difference everywhere.
	c := geom.NewConic(geom.NewPoint(0, 0), geom.NewPoint(2, 4), geom.NewPoint(5, 1), 0.7)
	const h = 1e-6
	for _, tt := range []float64{0.1, 0.25, 0.5, 0.75, 0.9} {
		dx, dy := c.EvalDeriv(tt)
		xp, yp := c.Eval(tt + h)
		xm, ym := c.Eval(tt - h)
		require.InDeltaf(t, (xp-xm)/(2*h), dx, 1e-5, "dx at %v", tt)
		require.InDeltaf(t, (yp-ym)/(2*h), dy, 1e-5, "dy at %v", tt)
	}
}

func TestConicExactAreaSamplingIndependent(t *testing.T) {
	// A conic + its chord encloses an exact, sampling-independent area: the
	// region's Area is identical across coarse and fine sampling for every conic
	// family (rho < 0.5 ellipse, = 0.5 parabola, > 0.5 hyperbola).
	start := geom.NewPoint(0, 0)
	apex := geom.NewPoint(2, 5)
	end := geom.NewPoint(6, 0)
	for _, rho := range []float64{0.3, 0.5, 0.8} {
		var areas []float64
		for _, segs := range []int{16, 64, 256} {
			conic := geom.NewConic(start, apex, end, rho)
			chord := geom.NewLine(end, start)
			arr := geom.Regions([]geom.Curve{conic, chord}, nil, geom.WithSegmentsPerTurn(segs))
			require.Lenf(t, arr.Regions, 1, "rho %v segs %d", rho, segs)
			areas = append(areas, arr.Regions[0].Area)
		}
		require.InDeltaf(t, areas[0], areas[1], 1e-9, "rho %v: 16 vs 64", rho)
		require.InDeltaf(t, areas[0], areas[2], 1e-9, "rho %v: 16 vs 256", rho)
		require.Greaterf(t, math.Abs(areas[0]), 1e-6, "rho %v: nonzero area", rho)
	}
}

func TestConicExactAreaMatchesClosedForm(t *testing.T) {
	// The region area equals the chord polygon area (zero here, a degenerate
	// triangle) plus the analytic conic bulge w*(a×b)*I, checked against a fine
	// reference integration of the rational quadratic.
	start := geom.NewPoint(0, 0)
	apex := geom.NewPoint(1, 2)
	end := geom.NewPoint(3, 0.5)
	for _, rho := range []float64{0.17, 0.3, 0.5, 0.7, 0.83} {
		conic := geom.NewConic(start, apex, end, rho)
		chord := geom.NewLine(end, start)
		arr := geom.Regions([]geom.Curve{conic, chord}, nil, geom.WithSegmentsPerTurn(64))
		require.Lenf(t, arr.Regions, 1, "rho %v", rho)
		want := conicBulgeReference(start, apex, end, rho)
		// signedPolyArea of the chord triangle (start, end, start) is 0, so the
		// region area is exactly the absolute bulge.
		require.InDeltaf(t, math.Abs(want), arr.Regions[0].Area, 1e-7, "rho %v area", rho)
	}
}

// conicBulgeReference numerically integrates the signed area between the conic
// and its chord with a fine composite Simpson rule — an independent oracle for
// the closed-form bulge the arrangement uses.
func conicBulgeReference(start, apex, end *geom.Point, rho float64) float64 {
	w := rho / (1 - rho)
	eval := func(t float64) (float64, float64) {
		u := 1 - t
		b0, b1, b2 := u*u, 2*u*t*w, t*t
		den := b0 + b1 + b2
		x := (b0*start.X + b1*apex.X + b2*end.X) / den
		y := (b0*start.Y + b1*apex.Y + b2*end.Y) / den
		return x, y
	}
	const h = 1e-6
	integrand := func(t float64) float64 {
		xp, yp := eval(t + h)
		xm, ym := eval(t - h)
		dx, dy := (xp-xm)/(2*h), (yp-ym)/(2*h)
		x, y := eval(t)
		return x*dy - y*dx
	}
	const n = 20000
	sum := 0.0
	for i := 0; i <= n; i++ {
		tt := float64(i) / float64(n)
		c := integrand(tt)
		switch {
		case i == 0 || i == n:
			sum += c
		case i%2 == 1:
			sum += 4 * c
		default:
			sum += 2 * c
		}
	}
	return 0.5 * sum * (1.0 / float64(n)) / 3
}
