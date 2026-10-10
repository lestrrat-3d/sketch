package geom

import (
	"math"
	"math/big"
)

// fitCircleScene certifies the four-source outline made by two interpolating
// flanks, their common outer arc, and an inner circle. It is deliberately
// narrow: every other free-form arrangement keeps the sampled fallback.
//
// The proof has three parts. Each flank's squared distance from the common
// centre has a strictly positive derivative on every cubic piece, so it meets
// each radius at most once. The flanks occupy opposite open half-planes of the
// outer arc's radial bisector, so they cannot meet each other. The outer arc
// meets each flank only at its own domain end. Bernstein coefficients give
// whole-piece sign bounds; all coefficient arithmetic below is exact over the
// stored binary64 interpolant data.
func (a *arranger) certifyFitCircleScene() {
	if len(a.sources) != 4 {
		return
	}
	var fits []int
	circle, arc := -1, -1
	for i := range a.sources {
		switch a.sources[i].kind {
		case srcFitSpline:
			fits = append(fits, i)
		case srcCircle:
			circle = i
		case srcArc:
			arc = i
		default:
			return
		}
	}
	if len(fits) != 2 || circle < 0 || arc < 0 {
		return
	}
	c, tip := &a.sources[circle], &a.sources[arc]
	if c.cx != tip.cx || c.cy != tip.cy || !(c.r > 0 && tip.r > c.r) ||
		!(math.Abs(tip.sweep) > 0 && math.Abs(tip.sweep) < math.Pi) {
		return
	}
	ends := [2][2]float64{a.sources[fits[0]].at(1), a.sources[fits[1]].at(1)}
	axisX := (ends[0][0]+ends[1][0])/2 - c.cx
	axisY := (ends[0][1]+ends[1][1])/2 - c.cy
	if !finiteVal(axisX) || !finiteVal(axisY) || math.Hypot(axisX, axisY) <= c.r {
		return
	}
	endArcParam := [2]float64{}
	for k := range fits {
		p := ends[k]
		at0, at1 := tip.at(0), tip.at(1)
		d0 := math.Hypot(p[0]-at0[0], p[1]-at0[1])
		d1 := math.Hypot(p[0]-at1[0], p[1]-at1[1])
		if math.Min(d0, d1) > weldIdentEps*a.scale {
			return
		}
		if d0 < d1 {
			endArcParam[k] = 0
		} else {
			endArcParam[k] = 1
		}
	}
	if endArcParam[0] == endArcParam[1] {
		return
	}

	var roots [2]xEvent
	var side [2]int
	for k, index := range fits {
		src := &a.sources[index]
		if src.fitEval == nil {
			return
		}
		fi := src.fitEval.interpolant()
		pieces, ok := exactFitPieces(fi)
		if !ok {
			return
		}
		side[k] = fitSide(pieces, c.cx, c.cy, axisX, axisY, a.scale)
		if side[k] == 0 {
			return
		}
		t, ok := fitRadiusCrossing(pieces, c.cx, c.cy, c.r, tip.r, a.scale)
		if !ok {
			return
		}
		p := src.at(t)
		if !finitePt(p) || math.Abs(math.Hypot(p[0]-c.cx, p[1]-c.cy)-c.r) > weldIdentEps*a.scale {
			return
		}
		roots[k] = xEvent{x: p[0], y: p[1], ti: t, tj: operandOf(c).circleParam(p[0], p[1]), kind: evCross}
		e := roots[k]
		if index > circle {
			e.ti, e.tj = e.tj, e.ti
		}
		pair := pairKey(index, circle)
		if !a.analyticCrossingsCertified(pair[0], pair[1], []xEvent{e}) {
			return
		}
		if !a.fitArcHasOnlyEndpoint(index, arc, endArcParam[k]) {
			return
		}
	}
	if side[0] == side[1] {
		return
	}

	if a.specialHandled == nil {
		a.specialHandled = make(map[[2]int]struct{}, 5)
	}
	a.specialHandled[pairKey(fits[0], fits[1])] = struct{}{}
	for k, index := range fits {
		e := roots[k]
		pair := pairKey(index, circle)
		if index > circle {
			e.ti, e.tj = e.tj, e.ti
		}
		a.events[pair] = []xEvent{e}
		a.specialHandled[pair] = struct{}{}
		a.applyAnalyticCut(index, roots[k].ti, roots[k].x, roots[k].y)
		a.applyAnalyticCut(circle, roots[k].tj, roots[k].x, roots[k].y)

		end := ends[k]
		e = xEvent{x: end[0], y: end[1], ti: 1, tj: endArcParam[k], kind: evCross}
		pair = pairKey(index, arc)
		if index > arc {
			e.ti, e.tj = e.tj, e.ti
		}
		a.events[pair] = []xEvent{e}
		a.specialHandled[pair] = struct{}{}
	}
	a.exactAllowed = true
}

func (a *arranger) fitArcHasOnlyEndpoint(fit, arc int, arcEnd float64) bool {
	for _, i := range a.sourceSegs[fit] {
		for _, j := range a.sourceSegs[arc] {
			p, ok := segParams(&a.segs[i], &a.segs[j])
			if !ok {
				if _, _, overlap := collinearOverlap(&a.segs[i], &a.segs[j]); overlap {
					return false
				}
				continue
			}
			tf := a.segs[i].pa + p.ti*(a.segs[i].pb-a.segs[i].pa)
			ta := a.segs[j].pa + p.tj*(a.segs[j].pb-a.segs[j].pa)
			if math.Abs(tf-1) > segEps || math.Abs(ta-arcEnd) > segEps {
				return false
			}
		}
	}
	return true
}

type exactFitPiece struct {
	t0, t1 float64
	x, y   [4]*big.Rat
}

func exactFitPieces(fi *FitInterpolant) ([]exactFitPiece, bool) {
	if fi == nil || fi.size() < 2 {
		return nil, false
	}
	spans := fi.Spans()
	pieces := make([]exactFitPiece, 0, len(spans))
	for i, span := range spans {
		// Match the defining interpolant's exact difference of its exported
		// cumulative parameters, rather than rounding the span width first.
		h := sub(rat(fi.Params[i+1]), rat(fi.Params[i]))
		if h == nil || h.Sign() <= 0 {
			return nil, false
		}
		var piece exactFitPiece
		piece.t0, piece.t1 = span.TStart, span.TEnd
		for axis := range 2 {
			v0, v1 := rat(fi.Points[i][axis]), rat(fi.Points[i+1][axis])
			m0, m1 := rat(fi.SecondDerivs[i][axis]), rat(fi.SecondDerivs[i+1][axis])
			if v0 == nil || v1 == nil || m0 == nil || m1 == nil {
				return nil, false
			}
			h2 := mul(h, h)
			c := [4]*big.Rat{
				v0,
				sub(sub(v1, v0), quo(mul(h2, add(mul(rat(2), m0), m1)), rat(6))),
				quo(mul(h2, m0), rat(2)),
				quo(mul(h2, sub(m1, m0)), rat(6)),
			}
			if axis == 0 {
				piece.x = c
			} else {
				piece.y = c
			}
		}
		pieces = append(pieces, piece)
	}
	return pieces, true
}

func fitSide(pieces []exactFitPiece, cx, cy, ax, ay, scale float64) int {
	var side int
	margin := rat(1e-11 * scale * scale)
	if margin == nil || margin.Sign() <= 0 {
		return 0
	}
	for _, piece := range pieces {
		var cross [4]*big.Rat
		for i := range cross {
			x, y := piece.x[i], piece.y[i]
			if i == 0 {
				x, y = sub(x, rat(cx)), sub(y, rat(cy))
			}
			cross[i] = sub(mul(rat(ax), y), mul(rat(ay), x))
		}
		for _, b := range bernstein(cross[:]) {
			s := b.Sign()
			if s == 0 || new(big.Rat).Abs(b).Cmp(margin) <= 0 {
				return 0
			}
			if side != 0 && side != s {
				return 0
			}
			side = s
		}
	}
	return side
}

func fitRadiusCrossing(pieces []exactFitPiece, cx, cy, root, tip, scale float64) (float64, bool) {
	margin := rat(1e-11 * scale * scale)
	if margin == nil || margin.Sign() <= 0 {
		return 0, false
	}
	var crossing *exactFitPiece
	var crossingPoly []*big.Rat
	for i := range pieces {
		piece := &pieces[i]
		f := radialPoly(piece, cx, cy, root)
		df := make([]*big.Rat, len(f)-1)
		for j := 1; j < len(f); j++ {
			df[j-1] = mul(f[j], rat(float64(j)))
		}
		for _, b := range bernstein(df) {
			if b.Cmp(margin) <= 0 {
				return 0, false
			}
		}
		lo, hi := f[0].Sign(), evalRatPoly(f, rat(1)).Sign()
		if lo < 0 && hi > 0 {
			if crossing != nil {
				return 0, false
			}
			crossing, crossingPoly = piece, f
		} else if lo == 0 || hi == 0 || (lo > 0 && hi < 0) {
			return 0, false
		}
		tipF := radialPoly(piece, cx, cy, tip)
		if i+1 < len(pieces) && evalRatPoly(tipF, rat(1)).Cmp(new(big.Rat).Neg(margin)) >= 0 {
			return 0, false
		}
	}
	if crossing == nil || radialPoly(&pieces[0], cx, cy, root)[0].Sign() >= 0 ||
		evalRatPoly(radialPoly(&pieces[len(pieces)-1], cx, cy, root), rat(1)).Sign() <= 0 {
		return 0, false
	}
	tipEnd := evalRatPoly(radialPoly(&pieces[len(pieces)-1], cx, cy, tip), rat(1))
	if new(big.Rat).Abs(tipEnd).Cmp(margin) > 0 {
		return 0, false
	}
	lo, hi := rat(0), rat(1)
	for range 80 {
		mid := quo(add(lo, hi), rat(2))
		if evalRatPoly(crossingPoly, mid).Sign() < 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	u, _ := quo(add(lo, hi), rat(2)).Float64()
	t := crossing.t0 + u*(crossing.t1-crossing.t0)
	return t, finiteVal(t) && t > crossing.t0 && t < crossing.t1
}

func radialPoly(p *exactFitPiece, cx, cy, radius float64) []*big.Rat {
	x, y := make([]*big.Rat, 4), make([]*big.Rat, 4)
	copy(x, p.x[:])
	copy(y, p.y[:])
	x[0], y[0] = sub(x[0], rat(cx)), sub(y[0], rat(cy))
	x2, y2 := polyMul(x, x), polyMul(y, y)
	f := make([]*big.Rat, 7)
	for i := range f {
		f[i] = add(x2[i], y2[i])
	}
	f[0] = sub(f[0], mul(rat(radius), rat(radius)))
	return f
}

func polyMul(a, b []*big.Rat) []*big.Rat {
	out := make([]*big.Rat, len(a)+len(b)-1)
	for i := range out {
		out[i] = rat(0)
	}
	for i, x := range a {
		for j, y := range b {
			out[i+j] = add(out[i+j], mul(x, y))
		}
	}
	return out
}

func evalRatPoly(p []*big.Rat, x *big.Rat) *big.Rat {
	out := rat(0)
	for i := len(p) - 1; i >= 0; i-- {
		out = add(mul(out, x), p[i])
	}
	return out
}

// bernstein converts power coefficients to same-degree Bernstein coefficients.
// Their convex hull contains every value of the polynomial on [0,1].
func bernstein(p []*big.Rat) []*big.Rat {
	n := len(p) - 1
	out := make([]*big.Rat, len(p))
	for k := range out {
		out[k] = rat(0)
		for i := 0; i <= k; i++ {
			factor := new(big.Rat).SetFrac(big.NewInt(binomial(k, i)), big.NewInt(binomial(n, i)))
			out[k] = add(out[k], mul(p[i], factor))
		}
	}
	return out
}

func binomial(n, k int) int64 {
	result := int64(1)
	for i := 1; i <= k; i++ {
		result = result * int64(n-k+i) / int64(i)
	}
	return result
}

func rat(x float64) *big.Rat {
	if !finiteVal(x) {
		return nil
	}
	return new(big.Rat).SetFloat64(x)
}

func add(a, b *big.Rat) *big.Rat { return new(big.Rat).Add(a, b) }
func sub(a, b *big.Rat) *big.Rat { return new(big.Rat).Sub(a, b) }
func mul(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }
func quo(a, b *big.Rat) *big.Rat { return new(big.Rat).Quo(a, b) }
