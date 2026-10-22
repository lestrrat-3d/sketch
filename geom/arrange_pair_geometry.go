package geom

import (
	"math"
	"math/big"
)

// Exact rational geometry for the pair ledger (arrange_pair_ledger.go). Every
// function here takes and returns *big.Rat values that are never mutated, so the
// memoized hulls it receives stay read-only.

var half = big.NewRat(1, 2)

func hullBounds(h *exactFitHull) exactFitBounds {
	var b exactFitBounds
	for _, p := range h.points {
		for axis := range 2 {
			if b.min[axis] == nil || p[axis].Cmp(b.min[axis]) < 0 {
				b.min[axis] = p[axis]
			}
			if b.max[axis] == nil || p[axis].Cmp(b.max[axis]) > 0 {
				b.max[axis] = p[axis]
			}
		}
	}
	return b
}

// boundsDisjoint reports whether two boxes have a positive gap on some axis.
func boundsDisjoint(a, b exactFitBounds) bool {
	if a.min[0] == nil || b.min[0] == nil {
		return false
	}
	for axis := range 2 {
		if a.max[axis].Cmp(b.min[axis]) < 0 || b.max[axis].Cmp(a.min[axis]) < 0 {
			return true
		}
	}
	return false
}

// hullsDisjoint reports whether some axis strictly separates the convex hulls of
// the two point sets. The axes are the coordinate axes and the normal of every
// pair of points in either set, a superset of the edge normals of each hull.
func hullsDisjoint(a, b *exactFitHull) bool {
	if hullAxisApart(a, b, rat(1), rat(0)) || hullAxisApart(a, b, rat(0), rat(1)) {
		return true
	}
	for _, h := range [2]*exactFitHull{a, b} {
		for i := range h.points {
			for j := i + 1; j < len(h.points); j++ {
				dx := sub(h.points[j][0], h.points[i][0])
				dy := sub(h.points[j][1], h.points[i][1])
				if dx.Sign() == 0 && dy.Sign() == 0 {
					continue
				}
				if hullAxisApart(a, b, new(big.Rat).Neg(dy), dx) {
					return true
				}
			}
		}
	}
	return false
}

func hullAxisApart(a, b *exactFitHull, ax, ay *big.Rat) bool {
	project := func(h *exactFitHull) (*big.Rat, *big.Rat) {
		lo := add(mul(ax, h.points[0][0]), mul(ay, h.points[0][1]))
		hi := lo
		for _, p := range h.points[1:] {
			v := add(mul(ax, p[0]), mul(ay, p[1]))
			if v.Cmp(lo) < 0 {
				lo = v
			}
			if v.Cmp(hi) > 0 {
				hi = v
			}
		}
		return lo, hi
	}
	loA, hiA := project(a)
	loB, hiB := project(b)
	return hiA.Cmp(loB) < 0 || hiB.Cmp(loA) < 0
}

// splitHull splits a cubic Bézier at u by de Casteljau.
func splitHull(h exactFitHull, u *big.Rat) (exactFitHull, exactFitHull) {
	var left, right exactFitHull
	for axis := range 2 {
		p := [4]*big.Rat{h.points[0][axis], h.points[1][axis], h.points[2][axis], h.points[3][axis]}
		lerp := func(x, y *big.Rat) *big.Rat { return add(x, mul(u, sub(y, x))) }
		q0, q1, q2 := lerp(p[0], p[1]), lerp(p[1], p[2]), lerp(p[2], p[3])
		r0, r1 := lerp(q0, q1), lerp(q1, q2)
		s := lerp(r0, r1)
		left.points[0][axis], left.points[1][axis], left.points[2][axis], left.points[3][axis] = p[0], q0, r0, s
		right.points[0][axis], right.points[1][axis], right.points[2][axis], right.points[3][axis] = s, r1, q2, p[3]
	}
	return left, right
}

// subHull returns the part of the cubic over [a, b] with 0 <= a < b <= 1.
func subHull(h exactFitHull, a, b *big.Rat) exactFitHull {
	if b.Cmp(rat(1)) != 0 {
		h, _ = splitHull(h, b)
		a = quo(a, b)
	}
	if a.Sign() != 0 {
		_, h = splitHull(h, a)
	}
	return h
}

// bernsteinSquare returns the degree-6 Bernstein coefficients of the square of
// the degree-3 Bernstein polynomial with coefficients c.
func bernsteinSquare(c [4]*big.Rat) [7]*big.Rat {
	var out [7]*big.Rat
	for k := range out {
		sum := rat(0)
		for i := max(0, k-3); i <= min(3, k); i++ {
			w := new(big.Rat).SetFrac64(binomial(3, i)*binomial(3, k-i), binomial(6, k))
			sum = add(sum, mul(w, mul(c[i], c[k-i])))
		}
		out[k] = sum
	}
	return out
}

// circleBernstein returns the degree-6 Bernstein coefficients of
// |P(u) - c|^2 - r^2 along the cubic with control points h.
func circleBernstein(h *exactFitHull, cx, cy, r2 *big.Rat) [7]*big.Rat {
	var xs, ys [4]*big.Rat
	for i, p := range h.points {
		xs[i], ys[i] = sub(p[0], cx), sub(p[1], cy)
	}
	sx, sy := bernsteinSquare(xs), bernsteinSquare(ys)
	var g [7]*big.Rat
	for k := range g {
		g[k] = sub(add(sx[k], sy[k]), r2)
	}
	return g
}

// hullCircleSign returns 1 when every point of the cubic lies strictly outside
// the circle, -1 when every point lies strictly inside, and 0 when the Bernstein
// coefficients do not decide it.
func hullCircleSign(h *exactFitHull, cx, cy, r2 *big.Rat) int {
	g := circleBernstein(h, cx, cy, r2)
	pos, neg := true, true
	for _, v := range g {
		if v.Sign() <= 0 {
			pos = false
		}
		if v.Sign() >= 0 {
			neg = false
		}
	}
	switch {
	case pos:
		return 1
	case neg:
		return -1
	}
	return 0
}

// localCarrierOK proves that the cubic cut meets the carrier of the target (the
// infinite line, or the full circle) at most once, and within merge of the control
// point at index apex. The carrier function is monotone along the cut when every
// difference of its Bernstein coefficients has one strict sign. It then changes by
// at least m per unit of the parameter, so a root lies within |c(apex)|/m of the
// apex, and the curve moves at most S per unit of the parameter.
func localCarrierOK(cut *exactFitHull, apex int, t *ledgerTarget, merge *big.Rat) bool {
	var c []*big.Rat
	var degree int
	switch t.kind {
	case srcLine:
		ax, ay := t.seg.points[0][0], t.seg.points[0][1]
		dx, dy := sub(t.seg.points[1][0], ax), sub(t.seg.points[1][1], ay)
		for _, p := range cut.points {
			c = append(c, sub(mul(dx, sub(p[1], ay)), mul(dy, sub(p[0], ax))))
		}
		degree = 3
	default:
		g := circleBernstein(cut, t.cx, t.cy, t.r2)
		c = g[:]
		degree = 6
		if apex == 3 {
			apex = 6
		}
	}
	var minStep *big.Rat
	sign := 0
	for k := 0; k+1 < len(c); k++ {
		d := sub(c[k+1], c[k])
		s := d.Sign()
		if s == 0 || (sign != 0 && s != sign) {
			return false
		}
		sign = s
		if d.Sign() < 0 {
			d = new(big.Rat).Neg(d)
		}
		if minStep == nil || d.Cmp(minStep) < 0 {
			minStep = d
		}
	}
	m := mul(rat(float64(degree)), minStep)
	var speed *big.Rat
	for k := 0; k < 3; k++ {
		step := add(absRat(sub(cut.points[k+1][0], cut.points[k][0])), absRat(sub(cut.points[k+1][1], cut.points[k][1])))
		if speed == nil || step.Cmp(speed) > 0 {
			speed = step
		}
	}
	reach := mul(mul(rat(3), speed), absRat(c[apex]))
	return reach.Cmp(mul(merge, m)) <= 0
}

func absRat(x *big.Rat) *big.Rat {
	if x.Sign() < 0 {
		return new(big.Rat).Neg(x)
	}
	return x
}

type ratVec [2]*big.Rat

func cross(a, b ratVec) int { return sub(mul(a[0], b[1]), mul(a[1], b[0])).Sign() }
func dot(a, b ratVec) int   { return add(mul(a[0], b[0]), mul(a[1], b[1])).Sign() }

// sector returns the clockwise-most and counter-clockwise-most of the vectors, or
// false when they do not lie in an angle smaller than a half turn.
func sector(vs []ratVec) (ratVec, ratVec, bool) {
	var lo, hi ratVec
	foundLo, foundHi := false, false
	for _, w := range vs {
		allLeft, allRight := true, true
		for _, v := range vs {
			c := cross(w, v)
			if c < 0 {
				allLeft = false
			}
			if c > 0 {
				allRight = false
			}
		}
		if allLeft && !foundLo {
			lo, foundLo = w, true
		}
		if allRight && !foundHi {
			hi, foundHi = w, true
		}
	}
	if !foundLo || !foundHi {
		return lo, hi, false
	}
	if cross(lo, hi) == 0 && dot(lo, hi) <= 0 {
		return lo, hi, false
	}
	return lo, hi, true
}

func inSector(r, s1, s2 ratVec) bool {
	if cross(s1, s2) > 0 {
		return cross(s1, r) >= 0 && cross(r, s2) >= 0
	}
	return cross(s1, r) == 0 && dot(s1, r) > 0
}

// coneVectors returns the vectors from the apex control point to the other three.
func coneVectors(h *exactFitHull, end int) []ratVec {
	apex, step := 0, 1
	if end == 1 {
		apex, step = 3, -1
	}
	var vs []ratVec
	for k := 1; k <= 3; k++ {
		p := h.points[apex+step*k]
		v := ratVec{sub(p[0], h.points[apex][0]), sub(p[1], h.points[apex][1])}
		if v[0].Sign() != 0 || v[1].Sign() != 0 {
			vs = append(vs, v)
		}
	}
	return vs
}

// conesApart proves that two cubics leaving the same point share only that point.
// A cubic lies inside the cone its control vectors span, so two cones that meet
// only at the apex hold curves that meet only there.
func conesApart(a *exactFitHull, endA int, b *exactFitHull, endB int) bool {
	va, vb := coneVectors(a, endA), coneVectors(b, endB)
	if len(va) == 0 || len(vb) == 0 {
		return false
	}
	a1, a2, ok := sector(va)
	if !ok {
		return false
	}
	b1, b2, ok := sector(vb)
	if !ok {
		return false
	}
	return !inSector(b1, a1, a2) && !inSector(b2, a1, a2) && !inSector(a1, b1, b2) && !inSector(a2, b1, b2)
}

// arcWedgeSlack widens an arc's wedge on each side. The arc's end directions come
// from math.Cos and math.Sin, which are accurate to a few units in the last place
// (angle error near 2^-52); the slack is 2^-40, far above that.
var arcWedgeSlack = new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 40))

// ratWedge is the closed region swept by the rays from apex through lo and hi,
// counter-clockwise from lo to hi, with an angle smaller than a half turn.
type ratWedge struct {
	apex   ratVec
	lo, hi ratVec
}

// arcWedge returns the widened wedge of an arc source, or nil when the sweep is a
// half turn or more or not finite.
func arcWedge(src *source, cx, cy *big.Rat) *ratWedge {
	sweep := src.sweep
	if !(math.Abs(sweep) > 0 && math.Abs(sweep) < math.Pi) || !finiteVal(src.phi0) {
		return nil
	}
	a0, a1 := src.phi0, src.phi0+sweep
	if sweep < 0 {
		a0, a1 = a1, a0
	}
	dir := func(angle float64) ratVec { return ratVec{rat(math.Cos(angle)), rat(math.Sin(angle))} }
	lo, hi := dir(a0), dir(a1)
	if lo[0] == nil || lo[1] == nil || hi[0] == nil || hi[1] == nil {
		return nil
	}
	// Rotate each ray outward by adding slack times its perpendicular.
	lo = ratVec{add(lo[0], mul(arcWedgeSlack, lo[1])), sub(lo[1], mul(arcWedgeSlack, lo[0]))}
	hi = ratVec{sub(hi[0], mul(arcWedgeSlack, hi[1])), add(hi[1], mul(arcWedgeSlack, hi[0]))}
	return &ratWedge{apex: ratVec{cx, cy}, lo: lo, hi: hi}
}

// arcBounds returns a box that holds every point of an arc whose wedge exists (a sweep
// of less than a half turn): the box of its two end points and the axis extremes the
// sweep passes, over the sweep widened by twice the wedge slack, then padded outward by
// 2^-36 of the radius plus the rounding of the coordinates it was computed from. The
// full carrier circle's box is a superset too, but it overlaps every source that lies
// inside the circle's box, and each overlap costs an exact wedge test.
func arcBounds(src *source) (exactFitBounds, bool) {
	sweep := src.sweep
	if !(math.Abs(sweep) > 0 && math.Abs(sweep) < math.Pi) || !finiteVal(src.phi0) ||
		!finiteVal(src.cx) || !finiteVal(src.cy) || !(src.r > 0) || !finiteVal(src.r) {
		return exactFitBounds{}, false
	}
	a0, a1 := src.phi0, src.phi0+sweep
	if sweep < 0 {
		a0, a1 = a1, a0
	}
	const widen = 0x1p-39
	a0, a1 = a0-widen, a1+widen
	angles := []float64{a0, a1}
	for k := math.Ceil(a0 / (math.Pi / 2)); k*(math.Pi/2) < a1; k++ {
		angles = append(angles, k*(math.Pi/2))
	}
	lo, hi := [2]float64{math.Inf(1), math.Inf(1)}, [2]float64{math.Inf(-1), math.Inf(-1)}
	for _, a := range angles {
		x, y := src.cx+src.r*math.Cos(a), src.cy+src.r*math.Sin(a)
		lo[0], hi[0] = math.Min(lo[0], x), math.Max(hi[0], x)
		lo[1], hi[1] = math.Min(lo[1], y), math.Max(hi[1], y)
	}
	pad := src.r*0x1p-36 + 8*math.Abs(math.Nextafter(math.Abs(src.cx)+src.r, math.Inf(1))-(math.Abs(src.cx)+src.r)) +
		8*math.Abs(math.Nextafter(math.Abs(src.cy)+src.r, math.Inf(1))-(math.Abs(src.cy)+src.r))
	b := exactFitBounds{
		min: [2]*big.Rat{rat(lo[0] - pad), rat(lo[1] - pad)},
		max: [2]*big.Rat{rat(hi[0] + pad), rat(hi[1] + pad)},
	}
	if b.min[0] == nil || b.min[1] == nil || b.max[0] == nil || b.max[1] == nil {
		return exactFitBounds{}, false
	}
	return b, true
}

// excludes reports whether every control point of the cubic lies strictly outside
// the wedge on the same side of one of its two bounding rays, so the cubic holds no
// point of the wedge. A nil wedge excludes nothing.
func (w *ratWedge) excludes(h *exactFitHull) bool {
	if w == nil {
		return false
	}
	beforeLo, afterHi := true, true
	for _, p := range h.points {
		v := ratVec{sub(p[0], w.apex[0]), sub(p[1], w.apex[1])}
		if cross(w.lo, v) >= 0 {
			beforeLo = false
		}
		if cross(w.hi, v) <= 0 {
			afterHi = false
		}
	}
	return beforeLo || afterHi
}

// boxCircleSign returns 1 when the whole box lies strictly outside the circle, -1
// when it lies strictly inside, and 0 otherwise. It is the cheap test run before
// the Bernstein one.
func boxCircleSign(b exactFitBounds, cx, cy, r2 *big.Rat) int {
	var near, far [2]*big.Rat
	for axis, c := range [2]*big.Rat{cx, cy} {
		lo, hi := sub(b.min[axis], c), sub(b.max[axis], c)
		switch {
		case lo.Sign() > 0:
			near[axis] = lo
		case hi.Sign() < 0:
			near[axis] = new(big.Rat).Neg(hi)
		default:
			near[axis] = rat(0)
		}
		far[axis] = absRat(lo)
		if v := absRat(hi); v.Cmp(far[axis]) > 0 {
			far[axis] = v
		}
	}
	if add(mul(near[0], near[0]), mul(near[1], near[1])).Cmp(r2) > 0 {
		return 1
	}
	if add(mul(far[0], far[0]), mul(far[1], far[1])).Cmp(r2) < 0 {
		return -1
	}
	return 0
}
