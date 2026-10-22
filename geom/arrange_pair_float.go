package geom

import (
	"math"
	"math/big"
)

// The pair ledger separates curves with exact rational arithmetic. Most pairs it
// judges are far apart, and a box test settles those with a few comparisons of
// binary64 numbers, so this file keeps an OUTWARD-rounded float64 box beside each
// exact bound and hull. A float box holds the exact one, so two float boxes that do
// not touch prove the exact boxes do not touch; the pair then takes exactly the
// branch the exact box test would have taken. When the float boxes touch, the exact
// test runs as before, so the prefilter changes the cost of a verdict and never the
// verdict.

// ledgerFloatPrefilter switches the float boxes off for the tests that compare the
// two paths. It is true in every other use.
var ledgerFloatPrefilter = true

// floatBox is an axis-aligned box in float64 that holds some exact box. A box with
// ok false holds nothing known and never separates.
type floatBox struct {
	min, max [2]float64
	ok       bool
}

// apart reports whether the two boxes are strictly separated on some axis.
func (a floatBox) apart(b floatBox) bool {
	if !a.ok || !b.ok {
		return false
	}
	return a.max[0] < b.min[0] || b.max[0] < a.min[0] || a.max[1] < b.min[1] || b.max[1] < a.min[1]
}

// outwardBox rounds an exact box outward to float64.
func outwardBox(b exactFitBounds) floatBox {
	var out floatBox
	for axis := range 2 {
		if b.min[axis] == nil || b.max[axis] == nil {
			return floatBox{}
		}
		lo, ok0 := roundDown(b.min[axis])
		hi, ok1 := roundUp(b.max[axis])
		if !ok0 || !ok1 {
			return floatBox{}
		}
		out.min[axis], out.max[axis] = lo, hi
	}
	out.ok = true
	return out
}

// roundDown returns a float64 no greater than r, and false when r is not a finite float64.
func roundDown(r *big.Rat) (float64, bool) {
	f, exact := r.Float64()
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	if !exact {
		f = math.Nextafter(f, math.Inf(-1))
	}
	return f, true
}

// roundUp returns a float64 no less than r, and false when r is not a finite float64.
func roundUp(r *big.Rat) (float64, bool) {
	f, exact := r.Float64()
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	if !exact {
		f = math.Nextafter(f, math.Inf(1))
	}
	return f, true
}

// floatHull is the control polygon of a cubic in float64 together with an absolute
// error bound: every exact coordinate lies within e of its float. A hull with ok
// false has no float form.
type floatHull struct {
	p  [4][2]float64
	e  float64
	ok bool
}

// ulpOf returns the spacing of float64 values at m, which bounds the spacing at every
// value of smaller magnitude.
func ulpOf(m float64) float64 {
	return math.Nextafter(m, math.Inf(1)) - m
}

// floatHullOf converts the exact control points of h to nearest float64 values.
func floatHullOf(h *exactFitHull) floatHull {
	var f floatHull
	m := 0.0
	for i, p := range h.points {
		for axis := range 2 {
			v, _ := p[axis].Float64()
			if math.IsInf(v, 0) || math.IsNaN(v) {
				return floatHull{}
			}
			f.p[i][axis] = v
			m = math.Max(m, math.Abs(v))
		}
	}
	// Each conversion is off by at most half the spacing at its own value.
	f.e = ulpOf(m)
	f.ok = true
	return f
}

// splitHalf splits the cubic at 1/2 by de Casteljau in float64. Each average of two
// values adds at most half the spacing at the largest coordinate, and a coordinate of
// the children is at most three averages deep, so the error bound grows by at most two.
func (f *floatHull) splitHalf() (floatHull, floatHull) {
	m := 0.0
	for _, p := range f.p {
		m = math.Max(m, math.Max(math.Abs(p[0]), math.Abs(p[1])))
	}
	e := f.e + 2*ulpOf(m)
	left, right := floatHull{e: e, ok: true}, floatHull{e: e, ok: true}
	for axis := range 2 {
		p0, p1, p2, p3 := f.p[0][axis], f.p[1][axis], f.p[2][axis], f.p[3][axis]
		q0, q1, q2 := 0.5*p0+0.5*p1, 0.5*p1+0.5*p2, 0.5*p2+0.5*p3
		r0, r1 := 0.5*q0+0.5*q1, 0.5*q1+0.5*q2
		s := 0.5*r0 + 0.5*r1
		left.p[0][axis], left.p[1][axis], left.p[2][axis], left.p[3][axis] = p0, q0, r0, s
		right.p[0][axis], right.p[1][axis], right.p[2][axis], right.p[3][axis] = s, r1, q2, p3
	}
	return left, right
}

// box returns a float box that holds the exact control polygon.
func (f *floatHull) box() floatBox {
	if !f.ok {
		return floatBox{}
	}
	var b floatBox
	for axis := range 2 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, p := range f.p {
			lo, hi = math.Min(lo, p[axis]), math.Max(hi, p[axis])
		}
		// The subtraction and the addition round too; step one more unit out.
		b.min[axis] = math.Nextafter(lo-f.e, math.Inf(-1))
		b.max[axis] = math.Nextafter(hi+f.e, math.Inf(1))
	}
	b.ok = !math.IsNaN(b.min[0]+b.min[1]+b.max[0]+b.max[1]) && !math.IsInf(b.min[0]+b.min[1]+b.max[0]+b.max[1], 0)
	return b
}

// splitHullHalf is splitHull at 1/2 that also splits the float form.
func splitHullHalf(h exactFitHull) (exactFitHull, exactFitHull) {
	left, right := splitHull(h, half)
	if h.fl.ok {
		left.fl, right.fl = h.fl.splitHalf()
	}
	return left, right
}

// clearOfCircle reports whether the cubic with control polygon f lies wholly outside
// or wholly inside the circle, by a margin far above the rounding of the float
// arithmetic here, so that |P(u) - c|^2 - r^2 keeps one strict sign on the cubic. The
// cubic lies in the box of its control polygon, so a box wholly outside or inside the
// circle holds the cubic outside or inside it.
func (f *floatHull) clearOfCircle(cx, cy, r float64) bool {
	box := f.box()
	if !box.ok {
		return false
	}
	scale := math.Abs(cx) + math.Abs(cy) + r
	for axis := range 2 {
		scale += math.Max(math.Abs(box.min[axis]), math.Abs(box.max[axis]))
	}
	margin := 1e-9 * scale
	// Nearest and farthest points of the box from the centre.
	var near, far float64
	for axis, c := range [2]float64{cx, cy} {
		lo, hi := box.min[axis]-c, box.max[axis]-c
		gap := math.Max(math.Max(lo, -hi), 0)
		reach := math.Max(math.Abs(lo), math.Abs(hi))
		near += gap * gap
		far += reach * reach
	}
	return math.Sqrt(near) > r+margin || math.Sqrt(far) < r-margin
}

// advancesAlongChord reports whether every Bézier control vector of the spline has a
// dot product with the chord that is positive by a margin far above the rounding of the
// float arithmetic here. It answers yes only when the exact test of self would, so the
// exact test runs for every spline it does not settle.
func advancesAlongChord(f *ledgerFit) bool {
	d := [2]float64{f.p1[0] - f.p0[0], f.p1[1] - f.p0[1]}
	dn := math.Abs(d[0]) + math.Abs(d[1])
	ud := ulpOf(math.Max(math.Max(math.Abs(f.p0[0]), math.Abs(f.p0[1])), math.Max(math.Abs(f.p1[0]), math.Abs(f.p1[1]))))
	if !(dn > 0) || math.IsInf(dn, 0) || math.IsNaN(dn) {
		return false
	}
	for k := range f.hulls {
		h := &f.hulls[k].fl
		if !h.ok {
			return false
		}
		for c := range 3 {
			v := [2]float64{h.p[c+1][0] - h.p[c][0], h.p[c+1][1] - h.p[c][1]}
			vn := math.Abs(v[0]) + math.Abs(v[1])
			margin := 1e-9*vn*dn + 64*h.e*dn + 64*ud*vn
			if !(v[0]*d[0]+v[1]*d[1] > margin) {
				return false
			}
		}
	}
	return true
}

// sub returns the float form of the part of the cubic over [a, b], 0 <= a < b <= 1,
// splitting at b and then at a/b like subHull. Each split moves the error bound up by
// sixteen spacings at the largest coordinate: a lerp rounds twice, three levels of lerps
// feed each new point, and the split parameter itself is a rounded quotient.
func (f *floatHull) sub(a, b float64) floatHull {
	if !f.ok {
		return floatHull{}
	}
	out := *f
	if b != 1 {
		out = out.splitAt(b, false)
		a /= b
	}
	if a != 0 {
		out = out.splitAt(a, true)
	}
	return out
}

// splitAt splits at u by de Casteljau and returns the right part when right is true
// and the left part otherwise.
func (f floatHull) splitAt(u float64, right bool) floatHull {
	m := 0.0
	for _, p := range f.p {
		m = math.Max(m, math.Max(math.Abs(p[0]), math.Abs(p[1])))
	}
	out := floatHull{e: f.e + 16*ulpOf(m), ok: true}
	for axis := range 2 {
		p0, p1, p2, p3 := f.p[0][axis], f.p[1][axis], f.p[2][axis], f.p[3][axis]
		lerp := func(x, y float64) float64 { return x + u*(y-x) }
		q0, q1, q2 := lerp(p0, p1), lerp(p1, p2), lerp(p2, p3)
		r0, r1 := lerp(q0, q1), lerp(q1, q2)
		s := lerp(r0, r1)
		if right {
			out.p[0][axis], out.p[1][axis], out.p[2][axis], out.p[3][axis] = s, r1, q2, p3
		} else {
			out.p[0][axis], out.p[1][axis], out.p[2][axis], out.p[3][axis] = p0, q0, r0, s
		}
	}
	for _, p := range out.p {
		if math.IsNaN(p[0]+p[1]) || math.IsInf(p[0]+p[1], 0) {
			return floatHull{}
		}
	}
	return out
}

// localCarrierFloatOK is localCarrierOK in float64. It returns true only when the test
// passes by a wide margin over the rounding of the arithmetic here, and false when it
// fails or is too close to say; the caller then runs the exact test.
//
// The carrier function is the signed distance to a line (degree 3 along the cubic) or
// |P - c|^2 - r^2 (degree 6). Its Bernstein coefficients c_k carry an error bound errC.
// Then the consecutive differences must share one sign, the smallest step is bounded
// below by its float value less 2 errC (which must stay positive, so every difference
// exceeds the rounding of its two terms), and the reach 3 speed |c_apex| is bounded
// above with errC added to |c_apex| and a relative 1e-9 added to the speed.
func localCarrierFloatOK(cut *exactFitHull, apex int, t *ledgerTarget, merge float64) bool {
	h := &cut.fl
	if !h.ok || !(merge > 0) {
		return false
	}
	var c []float64
	var degree int
	var errC float64
	extent := 0.0
	for _, p := range h.p {
		extent = math.Max(extent, math.Max(math.Abs(p[0]), math.Abs(p[1])))
	}
	switch t.kind {
	case srcLine:
		ax, ay := t.fline[0][0], t.fline[0][1]
		dx, dy := t.fline[1][0]-ax, t.fline[1][1]-ay
		for _, p := range h.p {
			c = append(c, dx*(p[1]-ay)-dy*(p[0]-ax))
		}
		degree = 3
		reach := extent + math.Max(math.Abs(ax), math.Abs(ay))
		errC = 1e-12*(math.Abs(dx)+math.Abs(dy))*reach + 16*h.e*(math.Abs(dx)+math.Abs(dy))
	case srcCircle, srcArc:
		var xs, ys [4]float64
		for i, p := range h.p {
			xs[i], ys[i] = p[0]-t.fcx, p[1]-t.fcy
		}
		sx, sy := floatBernsteinSquare(xs), floatBernsteinSquare(ys)
		g := make([]float64, 7)
		for k := range g {
			g[k] = sx[k] + sy[k] - t.fr*t.fr
		}
		c = g
		degree = 6
		if apex == 3 {
			apex = 6
		}
		r := extent + math.Abs(t.fcx) + math.Abs(t.fcy) + t.fr
		errC = 1e-12*r*r + 16*h.e*r
	default:
		return false
	}
	minStep := math.Inf(1)
	sign := 0.0
	for k := 0; k+1 < len(c); k++ {
		d := c[k+1] - c[k]
		if math.IsNaN(d) {
			return false
		}
		if sign != 0 && d*sign < 0 {
			return false
		}
		sign = math.Copysign(1, d)
		minStep = math.Min(minStep, math.Abs(d))
	}
	m := float64(degree) * (minStep - 2*errC)
	speed := 0.0
	for k := range 3 {
		speed = math.Max(speed, math.Abs(h.p[k+1][0]-h.p[k][0])+math.Abs(h.p[k+1][1]-h.p[k][1]))
	}
	speed = speed*(1+1e-9) + 8*h.e
	reachUB := 3 * speed * (math.Abs(c[apex]) + errC)
	return m > 0 && reachUB <= 0.5*merge*m
}

// floatBernsteinSquare is bernsteinSquare in float64.
func floatBernsteinSquare(c [4]float64) [7]float64 {
	var out [7]float64
	for k := range out {
		sum := 0.0
		for i := max(0, k-3); i <= min(3, k); i++ {
			w := float64(binomial(3, i)*binomial(3, k-i)) / float64(binomial(6, k))
			sum += w * c[i] * c[k-i]
		}
		out[k] = sum
	}
	return out
}
