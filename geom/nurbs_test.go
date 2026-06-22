package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// quarterCircleNURBS is the classic rational quadratic NURBS that traces an exact
// quarter circle of radius 1 from (1,0) to (0,1): control (1,0),(1,1),(0,1),
// weights 1, 1/√2, 1, knots {0,0,0,1,1,1}.
func quarterCircleNURBS() *geom.NURBS {
	return geom.NewNURBS(2,
		[]*geom.Point{geom.NewPoint(1, 0), geom.NewPoint(1, 1), geom.NewPoint(0, 1)},
		[]float64{0, 0, 0, 1, 1, 1},
		[]float64{1, 1 / math.Sqrt2, 1},
	)
}

func TestNURBSKernelExactCircleArc(t *testing.T) {
	c := quarterCircleNURBS()
	for i := 0; i <= 64; i++ {
		u := float64(i) / 64
		x, y := c.Eval(u)
		require.InDelta(t, 1.0, math.Hypot(x, y), 1e-14, "rational quadratic NURBS lies on the unit circle at u=%v", u)
	}
}

func TestNURBSKernelEndpointsAndPartitionOfUnity(t *testing.T) {
	c := quarterCircleNURBS()
	// Clamped endpoints: Eval(domain) == first/last control point.
	x0, y0 := c.Eval(0)
	require.InDelta(t, 1.0, x0, 1e-15)
	require.InDelta(t, 0.0, y0, 1e-15)
	x1, y1 := c.Eval(1)
	require.InDelta(t, 0.0, x1, 1e-15)
	require.InDelta(t, 1.0, y1, 1e-15)
	require.True(t, c.Rational())

	// A non-rational curve at a control-point-interpolating clamped end has unit
	// weight; partition of unity is implied by the Eval(W)==1 normalization, but we
	// also check a non-rational curve interpolates its ends.
	nr := geom.NewNURBS(3,
		[]*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1), geom.NewPoint(5, 1)},
		geom.ClampedUniformKnots(4, 3), nil)
	require.False(t, nr.Rational())
	ex, ey := nr.Eval(0)
	require.InDelta(t, 0.0, ex, 1e-12)
	require.InDelta(t, 0.0, ey, 1e-12)
	lx, ly := nr.Eval(1)
	require.InDelta(t, 5.0, lx, 1e-12)
	require.InDelta(t, 1.0, ly, 1e-12)
}

func TestNURBSDerivativeMatchesFiniteDifference(t *testing.T) {
	c := quarterCircleNURBS()
	const h = 1e-6
	for _, u := range []float64{0.1, 0.3, 0.5, 0.7, 0.9} {
		dx, dy := c.EvalDeriv(u)
		xp, yp := c.Eval(u + h)
		xm, ym := c.Eval(u - h)
		require.InDelta(t, (xp-xm)/(2*h), dx, 1e-5, "dx at u=%v", u)
		require.InDelta(t, (yp-ym)/(2*h), dy, 1e-5, "dy at u=%v", u)
	}
}

func TestClampedUniformKnots(t *testing.T) {
	k := geom.ClampedUniformKnots(5, 3)
	require.Equal(t, []float64{0, 0, 0, 0, 0.5, 1, 1, 1, 1}, k)
	require.Len(t, k, 5+3+1)
	require.Nil(t, geom.ClampedUniformKnots(2, 3), "too few control points → nil")

	// A single span (n == degree+1) has no interior knots.
	require.Equal(t, []float64{0, 0, 0, 1, 1, 1}, geom.ClampedUniformKnots(3, 2))
}

// fineBulge is the arc-vs-chord area of a NURBS fragment [t0,t1] (t in [0,1])
// computed by a very fine sampled polygon — the independent reference the analytic
// nurbsBulgeSpan must reproduce.
func fineBulge(c *geom.NURBS, t0, t1 float64) float64 {
	const n = 400000
	dom0, dom1 := nurbsDomain(c)
	var moment, px, py float64
	for i := 0; i <= n; i++ {
		t := t0 + (t1-t0)*float64(i)/n
		x, y := c.Eval(dom0 + (dom1-dom0)*t)
		if i > 0 {
			moment += 0.5 * (px*y - x*py)
		}
		px, py = x, y
	}
	ax, ay := c.Eval(dom0 + (dom1-dom0)*t0)
	ex, ey := c.Eval(dom0 + (dom1-dom0)*t1)
	return moment + 0.5*(ex*ay-ax*ey)
}

func nurbsDomain(c *geom.NURBS) (lo, hi float64) {
	p := c.Degree
	n := len(c.Control) - 1
	return c.Knots[p], c.Knots[n+1]
}

func TestNURBSAreaQuarterCircleExact(t *testing.T) {
	c := quarterCircleNURBS()
	// The whole-curve cap area (between the quarter arc and its chord) is
	// π/4 − 1/2; nurbsBulgeSpan is accessible via the arrangement region area, but
	// here we cross-check it against the fine polygon directly through the package.
	got := geom.NURBSBulgeSpanForTest(c, 0, 1)
	require.InDelta(t, math.Pi/4-0.5, got, 1e-10, "rational quarter-circle whole bulge")
	require.InDelta(t, fineBulge(c, 0, 1), got, 1e-9)

	// A split fragment with t0 != 0 (where a wrong arc-vs-subchord correction shows).
	for _, span := range [][2]float64{{0.3, 1.0}, {0.0, 0.6}, {0.25, 0.85}} {
		g := geom.NURBSBulgeSpanForTest(c, span[0], span[1])
		require.InDeltaf(t, fineBulge(c, span[0], span[1]), g, 1e-9,
			"rational split bulge [%v,%v]", span[0], span[1])
	}
}

func TestNURBSRegionAreaSamplingIndependent(t *testing.T) {
	// The rational quarter-circle NURBS plus two lines closing it back to the
	// origin bound one region. Its area is the unit circle's quarter sector,
	// π/4 ≈ 0.7853981634, and must be sampling-independent (the true-curve bulge,
	// not the polyline) across WithSegmentsPerTurn.
	mk := func() []geom.Curve {
		p0 := geom.NewPoint(1, 0)
		p1 := geom.NewPoint(1, 1)
		p2 := geom.NewPoint(0, 1)
		o := geom.NewPoint(0, 0)
		c := geom.NewNURBS(2, []*geom.Point{p0, p1, p2},
			[]float64{0, 0, 0, 1, 1, 1}, []float64{1, 1 / math.Sqrt2, 1})
		return []geom.Curve{c, geom.NewLine(p2, o), geom.NewLine(o, p0)}
	}
	want := math.Pi / 4
	for _, spt := range []int{8, 16, 32, 64, 128, 256} {
		arr := geom.Regions(mk(), nil, geom.WithSegmentsPerTurn(spt))
		require.Len(t, arr.Regions, 1, "spt=%d", spt)
		require.InDeltaf(t, want, arr.Regions[0].Area, 1e-10,
			"rational quarter-circle sector area at spt=%d", spt)
	}
}

func TestNURBSNonRationalRegionAreaSamplingIndependent(t *testing.T) {
	// A non-rational cubic NURBS closed by a chord line bounds one region; the
	// true-curve bulge makes the area exact and independent of sampling density.
	mk := func() []geom.Curve {
		p0 := geom.NewPoint(0, 0)
		p1 := geom.NewPoint(1, 3)
		p2 := geom.NewPoint(3, 3)
		p3 := geom.NewPoint(4, 0)
		c := geom.NewNURBS(3, []*geom.Point{p0, p1, p2, p3}, geom.ClampedUniformKnots(4, 3), nil)
		return []geom.Curve{c, geom.NewLine(p3, p0)}
	}
	ref := geom.Regions(mk(), nil, geom.WithSegmentsPerTurn(1024)).Regions[0].Area
	for _, spt := range []int{8, 16, 32, 64, 128} {
		arr := geom.Regions(mk(), nil, geom.WithSegmentsPerTurn(spt))
		require.Len(t, arr.Regions, 1, "spt=%d", spt)
		require.InDeltaf(t, ref, arr.Regions[0].Area, 1e-9,
			"non-rational NURBS region area at spt=%d", spt)
	}
}

func TestNURBSAreaNonRationalExact(t *testing.T) {
	// A non-rational cubic NURBS: its moment integrand is degree 5 per span, so the
	// p-point Gauss rule is EXACT (limited only by the fine-polygon reference).
	c := geom.NewNURBS(3,
		[]*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1),
			geom.NewPoint(4, 1), geom.NewPoint(5, 0)},
		geom.ClampedUniformKnots(5, 3), nil)
	whole := geom.NURBSBulgeSpanForTest(c, 0, 1)
	require.InDelta(t, fineBulge(c, 0, 1), whole, 1e-8, "non-rational whole bulge")
	for _, span := range [][2]float64{{0.27, 0.83}, {0.0, 0.4}, {0.5, 1.0}} {
		g := geom.NURBSBulgeSpanForTest(c, span[0], span[1])
		require.InDeltaf(t, fineBulge(c, span[0], span[1]), g, 1e-8,
			"non-rational split bulge [%v,%v]", span[0], span[1])
	}
}
