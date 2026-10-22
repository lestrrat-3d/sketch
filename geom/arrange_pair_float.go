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
