package geom

import (
	"math"
	"math/big"
)

// Spline/circle crossings for the pair ledger (docs/pair-certificate-design.md,
// "Stage 2 — Crossing"). A fit spline that the avoidance proof cannot separate from
// a circle may still cross it transversally at a finite set of points. This file
// finds every such point exactly, brackets each one's spline parameter between two
// rounding boundaries of a binary64, and hands the result to the analytic cut
// machinery as ordinary crossing events.

// bernsteinSplit splits Bernstein coefficients at the middle of their interval
// (de Casteljau at 1/2). The halves are the coefficients of the same polynomial on
// the left and right half of the interval, each reparametrized onto [0, 1].
func bernsteinSplit(c []*big.Rat) ([]*big.Rat, []*big.Rat) {
	n := len(c)
	left := make([]*big.Rat, n)
	right := make([]*big.Rat, n)
	cur := slicesCloneRats(c)
	two := rat(2)
	for k := range n {
		left[k] = cur[0]
		right[n-1-k] = cur[len(cur)-1]
		next := make([]*big.Rat, len(cur)-1)
		for i := range next {
			next[i] = quo(add(cur[i], cur[i+1]), two)
		}
		cur = next
	}
	return left, right
}

func slicesCloneRats(c []*big.Rat) []*big.Rat {
	return append([]*big.Rat(nil), c...)
}

// oneStrictSign reports whether every value is strictly positive or every value is
// strictly negative.
func oneStrictSign(vs []*big.Rat) bool {
	pos, neg := true, true
	for _, v := range vs {
		s := v.Sign()
		pos = pos && s > 0
		neg = neg && s < 0
	}
	return pos || neg
}

// ratBracket is an interval [lo, hi] of a piece's local parameter that holds
// exactly one root of a polynomial that is strictly monotone on it.
type ratBracket struct {
	lo, hi *big.Rat
}

// isolateRoots appends to out one bracket per root of the polynomial with Bernstein
// coefficients c on [lo, hi], and reports whether it could account for every root.
//
// A coefficient list of one strict sign has no root. Otherwise, when the consecutive
// differences of the coefficients (the Bernstein coefficients of the derivative, up
// to a positive factor) are of one strict sign, the polynomial is strictly monotone
// on the interval, so it has at most one root, and exactly one when its two end
// values (the first and last coefficient) differ in sign. An end value of zero is a
// root on the boundary of the interval, which this routine does not place, so it
// reports failure. Any other interval is split in half. Depth is bounded by
// ledgerMaxDepth: roots closer than one part in 2^52 cannot be told apart from the
// stored coordinates.
func isolateRoots(c []*big.Rat, lo, hi *big.Rat, depth int, m *workMeter, out *[]ratBracket) bool {
	if !m.spend() {
		return false
	}
	if oneStrictSign(c) {
		return true
	}
	n := len(c) - 1
	d := make([]*big.Rat, n)
	for k := range d {
		d[k] = sub(c[k+1], c[k])
	}
	if oneStrictSign(d) {
		// The coefficients are monotone and not all of one strict sign, so a zero end
		// value is a root on the boundary. Otherwise the ends differ in sign: equal
		// signs would put every coefficient between them on that sign.
		if c[0].Sign() == 0 || c[n].Sign() == 0 {
			return false
		}
		*out = append(*out, ratBracket{lo: lo, hi: hi})
		return true
	}
	if depth >= ledgerMaxDepth {
		return false
	}
	left, right := bernsteinSplit(c)
	mid := quo(add(lo, hi), rat(2))
	return isolateRoots(left, lo, mid, depth+1, m, out) && isolateRoots(right, mid, hi, depth+1, m, out)
}

// circleCrossings returns the spline parameter of every crossing of the spline with
// the target circle, in increasing order, when it can account for every point the
// two curves share. ok is false when any piece has a root it cannot place as a
// transversal crossing strictly inside the piece (a touch, a root on a piece joint, a
// pair of roots closer than ledgerMaxDepth resolves, or an exhausted work meter). An
// empty result with ok true means the curves share no point.
func (l *pairLedger) circleCrossings(f *ledgerFit, t *ledgerTarget, m *workMeter) ([]float64, bool) {
	var ts []float64
	for k := range f.pieces {
		piece := &f.pieces[k]
		poly := radialPolyRat(piece, t.cx, t.cy, t.r2)
		var brackets []ratBracket
		if !isolateRoots(bernstein(poly), rat(0), rat(1), 0, m, &brackets) {
			return nil, false
		}
		for _, b := range brackets {
			u, ok := roundedBracketRoot(poly, b)
			if !ok {
				return nil, false
			}
			// A root within rounding of a piece joint reports the joint's own
			// parameter. Two roots that round to one parameter cannot be told
			// apart as cut sites, so the pair is refused.
			tt := piece.t0 + u*(piece.t1-piece.t0)
			if !(tt >= piece.t0 && tt <= piece.t1) || (len(ts) > 0 && tt <= ts[len(ts)-1]) {
				return nil, false
			}
			ts = append(ts, tt)
		}
	}
	return ts, true
}

// radialPolyRat is the polynomial |P(u) - c|^2 - r2 over one piece, with the
// circle's centre and squared radius given as exact rationals.
func radialPolyRat(p *exactFitPiece, cx, cy, r2 *big.Rat) []*big.Rat {
	x, y := make([]*big.Rat, 4), make([]*big.Rat, 4)
	copy(x, p.x[:])
	copy(y, p.y[:])
	x[0], y[0] = sub(x[0], cx), sub(y[0], cy)
	x2, y2 := polyMul(x, x), polyMul(y, y)
	f := make([]*big.Rat, 7)
	for i := range f {
		f[i] = add(x2[i], y2[i])
	}
	f[0] = sub(f[0], r2)
	return f
}

// roundedBracketRoot returns the binary64 nearest to the one root of poly inside the
// bracket, when the root is provably inside that float's rounding cell.
//
// The polynomial is strictly monotone on the bracket, so a sign change between two
// points of it shows the root lies between them. A float estimate proposes a
// candidate; two exact evaluations at the cell's boundaries (the midpoints to the
// neighbouring floats) confirm it. When the estimate misses, exact bisection narrows
// the bracket until it sits inside one cell.
func roundedBracketRoot(poly []*big.Rat, b ratBracket) (float64, bool) {
	s0 := evalRatPoly(poly, b.lo).Sign()
	if s0 == 0 {
		return 0, false
	}
	if u, ok := estimatedRoot(poly, b, s0); ok {
		return u, true
	}
	return bisectedRoot(poly, b, s0)
}

// maxRootBisections bounds the exact bisection that places a root inside one rounding
// cell. A root within 2^-k of a piece joint sits in a cell of width 2^-(k+52), so the
// bracket must shrink past it; 1100 halvings reach below the smallest subnormal.
const maxRootBisections = 1100

// bisectedRoot halves the bracket with exact evaluations until it lies inside the
// rounding cell of its own float64, or the halving limit is reached.
func bisectedRoot(poly []*big.Rat, b ratBracket, s0 int) (float64, bool) {
	lo, hi := b.lo, b.hi
	two := rat(2)
	for k := 1; k <= maxRootBisections; k++ {
		mid := quo(add(lo, hi), two)
		sm := evalRatPoly(poly, mid).Sign()
		if sm == 0 {
			return 0, false
		}
		if sm == s0 {
			lo = mid
		} else {
			hi = mid
		}
		if k < 54 || k%4 != 0 {
			continue
		}
		if u, _ := lo.Float64(); u >= 0 && u <= 1 && cellHoldsBracket(u, lo, hi) {
			return u, true
		}
	}
	return 0, false
}

// cellHoldsBracket reports whether [lo, hi] lies strictly inside the set of reals
// that round to u.
func cellHoldsBracket(u float64, lo, hi *big.Rat) bool {
	below, above := math.Nextafter(u, math.Inf(-1)), math.Nextafter(u, math.Inf(1))
	lower := quo(add(rat(below), rat(u)), rat(2))
	upper := quo(add(rat(u), rat(above)), rat(2))
	return lower.Cmp(lo) < 0 && hi.Cmp(upper) < 0
}

// estimatedRoot proposes the rounded root from a float64 bisection of the bracket and
// confirms it with two exact evaluations.
func estimatedRoot(poly []*big.Rat, b ratBracket, s0 int) (float64, bool) {
	coeff := make([]float64, len(poly))
	for i, c := range poly {
		coeff[i], _ = c.Float64()
		if !finiteVal(coeff[i]) {
			return 0, false
		}
	}
	lo, _ := b.lo.Float64()
	hi, _ := b.hi.Float64()
	for range 64 {
		mid := (lo + hi) / 2
		if mid == lo || mid == hi {
			break
		}
		v := coeff[len(coeff)-1]
		for i := len(coeff) - 2; i >= 0; i-- {
			v = v*mid + coeff[i]
		}
		if !finiteVal(v) {
			return 0, false
		}
		if (v < 0) == (s0 < 0) {
			lo = mid
		} else {
			hi = mid
		}
	}
	u := (lo + hi) / 2
	// Bisection resolves u to 2^-64 of the interval at best; a root a hair from a
	// bracket end needs relative accuracy, which Newton steps give.
	for range 3 {
		v, dv := coeff[len(coeff)-1], 0.0
		for i := len(coeff) - 2; i >= 0; i-- {
			dv = dv*u + v
			v = v*u + coeff[i]
		}
		step := v / dv
		if dv == 0 || !finiteVal(step) {
			break
		}
		u -= step
	}
	// The estimate may sit a cell or two off; the exact signs at a cell's two
	// boundaries decide which candidate holds the root.
	for _, cand := range [3]float64{u, math.Nextafter(u, math.Inf(-1)), math.Nextafter(u, math.Inf(1))} {
		if confirmedRoot(poly, b, cand) {
			return cand, true
		}
	}
	return 0, false
}

// confirmedRoot reports whether the one root of the strictly monotone poly in the
// bracket rounds to u: the cell of u lies in the bracket and poly changes sign across it.
func confirmedRoot(poly []*big.Rat, b ratBracket, u float64) bool {
	if !(u >= 0 && u <= 1) {
		return false
	}
	below, above := math.Nextafter(u, math.Inf(-1)), math.Nextafter(u, math.Inf(1))
	lower := quo(add(rat(below), rat(u)), rat(2))
	upper := quo(add(rat(u), rat(above)), rat(2))
	if lower.Cmp(b.lo) < 0 || upper.Cmp(b.hi) > 0 {
		return false
	}
	// A strictly monotone polynomial with opposite signs at the two cell
	// boundaries has its root between them.
	sl, su := evalRatPoly(poly, lower).Sign(), evalRatPoly(poly, upper).Sign()
	return sl != 0 && su != 0 && sl != su
}

// injectCrossings turns the crossing parameters of spline i with circle j into
// analytic crossing events and cuts both sources at them, when the sampled map has
// the same crossing incidence (analyticCrossingsCertified). It reports whether the
// events were injected; on false nothing was changed.
func (a *arranger) injectCrossings(i, j int, ts []float64) bool {
	c := &a.sources[j]
	events := make([]xEvent, 0, len(ts))
	for _, tt := range ts {
		a.budget.step(1)
		p := a.sources[i].at(tt)
		if !finitePt(p) || math.Abs(math.Hypot(p[0]-c.cx, p[1]-c.cy)-c.r) > weldIdentEps*a.scale {
			return false
		}
		e := xEvent{x: p[0], y: p[1], ti: tt, tj: operandOf(c).circleParam(p[0], p[1]), kind: evCross}
		if i > j {
			e.ti, e.tj = e.tj, e.ti
		}
		events = append(events, e)
	}
	pair := pairKey(i, j)
	if !a.analyticCrossingsCertified(pair[0], pair[1], events) {
		return false
	}
	if a.specialHandled == nil {
		a.specialHandled = make(map[[2]int]struct{})
	}
	a.events[pair] = events
	a.specialHandled[pair] = struct{}{}
	for _, e := range events {
		ti, tj := e.ti, e.tj
		if i > j {
			ti, tj = tj, ti
		}
		a.applyAnalyticCut(i, ti, e.x, e.y)
		a.applyAnalyticCut(j, tj, e.x, e.y)
	}
	return true
}
