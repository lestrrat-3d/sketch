package geom

import (
	"math"
	"math/big"
)

// certifyFitCircleScene certifies one or more tooth outlines made by pairs of
// interpolating flanks, their outer arcs, and one common inner circle. Other
// free-form arrangements keep the sampled fallback.
//
// The proof has three parts. Each flank's squared distance from the common
// centre has a strictly positive derivative on every cubic piece, so it meets
// each radius at most once. The flanks occupy opposite open half-planes of the
// outer arc's radial bisector, so the two flanks of a tooth cannot meet. Flanks
// of different teeth have disjoint exact Bezier hulls outside the root circle;
// they may meet inside it, so those inner fragments stay inexact. The outer arcs
// meet flanks only at their assigned domain ends and do not meet one another.
// Bernstein coefficients give whole-piece sign bounds; all coefficient
// arithmetic below is exact over the stored binary64 interpolant data.
func (a *arranger) certifyFitCircleScene() {
	if len(a.sources) < 4 || (len(a.sources)-1)%3 != 0 {
		return
	}
	var fits []int
	var arcs []int
	circle := -1
	for i := range a.sources {
		a.budget.step(1)
		switch a.sources[i].kind {
		case srcFitSpline:
			fits = append(fits, i)
		case srcCircle:
			if circle >= 0 {
				return
			}
			circle = i
		case srcArc:
			arcs = append(arcs, i)
		default:
			return
		}
	}
	if len(arcs) == 0 || len(fits) != 2*len(arcs) || circle < 0 {
		return
	}
	c := &a.sources[circle]
	tipRadius := a.sources[arcs[0]].r
	if !(c.r > 0 && tipRadius > c.r) {
		return
	}
	for k, arc := range arcs {
		tip := &a.sources[arc]
		if c.cx != tip.cx || c.cy != tip.cy || math.Abs(tip.r-tipRadius) > weldIdentEps*a.scale ||
			!(math.Abs(tip.sweep) > 0 && math.Abs(tip.sweep) < math.Pi) {
			return
		}
		for _, other := range arcs[:k] {
			events, ambiguous, ok := analyticEvents(tip, &a.sources[other], a.scale)
			if !ok || ambiguous || len(events) != 0 {
				return
			}
		}
	}
	type fitCertificate struct {
		index, arc  int
		pieceKey    string
		hullKey     string
		outerKey    string
		arcEnd      float64
		end         [2]float64
		root        xEvent
		pieces      []exactFitPiece
		hulls       []exactFitHull
		outerHulls  []exactFitHull
		bounds      exactFitBounds
		outerBounds exactFitBounds
	}
	certs := make([]fitCertificate, len(fits))
	arcEnds := make(map[int][2]int, len(arcs))
	for k, index := range fits {
		src := &a.sources[index]
		if src.fitEval == nil {
			return
		}
		fi := src.fitEval.interpolant()
		if fi == nil {
			return
		}
		pieceKey := fitInterpolantKey(fi)
		pieces, ok := memoFitPieces(pieceKey, fi)
		if !ok {
			return
		}
		end := src.at(1)
		matchedArc, matchedEnd := -1, -1
		for _, arc := range arcs {
			tip := &a.sources[arc]
			for endpoint := range 2 {
				p := tip.at(float64(endpoint))
				if math.Hypot(end[0]-p[0], end[1]-p[1]) > weldIdentEps*a.scale {
					continue
				}
				if matchedArc >= 0 {
					return
				}
				matchedArc, matchedEnd = arc, endpoint
			}
		}
		if matchedArc < 0 {
			return
		}
		for _, arc := range arcs {
			if arc == matchedArc {
				continue
			}
			param := operandOf(&a.sources[arc]).circleParam(end[0], end[1])
			if param <= 1+arcParamEps {
				return
			}
		}
		t, ok := memoFitRadiusCrossing(pieceKey, pieces, c.cx, c.cy, c.r, tipRadius, a.scale)
		if !ok {
			return
		}
		p := src.at(t)
		if !finitePt(p) || math.Abs(math.Hypot(p[0]-c.cx, p[1]-c.cy)-c.r) > weldIdentEps*a.scale {
			return
		}
		root := xEvent{x: p[0], y: p[1], ti: t, tj: operandOf(c).circleParam(p[0], p[1]), kind: evCross}
		e := root
		if index > circle {
			e.ti, e.tj = e.tj, e.ti
		}
		pair := pairKey(index, circle)
		if !a.analyticCrossingsCertified(pair[0], pair[1], []xEvent{e}) {
			return
		}
		if !a.fitArcHasOnlyEndpoint(index, matchedArc, float64(matchedEnd)) {
			return
		}
		outerCut := math.Max(0, t-1e-7)
		if !memoFitCutBelowRoot(pieceKey, pieces, outerCut, c.cx, c.cy, c.r, a.scale) {
			return
		}
		hulls := memoFitHulls(pieceKey, pieces)
		outerHulls := memoFitHullsAfter(pieceKey, pieces, outerCut)
		certs[k] = fitCertificate{index: index, arc: matchedArc, arcEnd: float64(matchedEnd),
			pieceKey: pieceKey, hullKey: fitHullsKey(pieceKey), outerKey: fitHullsAfterKey(pieceKey, outerCut),
			end: end, root: root, pieces: pieces, hulls: hulls, outerHulls: outerHulls,
			bounds: exactFitHullBounds(hulls), outerBounds: exactFitHullBounds(outerHulls)}
		arcPair := arcEnds[matchedArc]
		if arcPair[matchedEnd] != 0 {
			return
		}
		arcPair[matchedEnd] = k + 1
		arcEnds[matchedArc] = arcPair
	}
	for _, arc := range arcs {
		pair := arcEnds[arc]
		if pair[0] == 0 || pair[1] == 0 {
			return
		}
		first, second := certs[pair[0]-1], certs[pair[1]-1]
		axisX := (first.end[0]+second.end[0])/2 - c.cx
		axisY := (first.end[1]+second.end[1])/2 - c.cy
		if !finiteVal(axisX) || !finiteVal(axisY) || math.Hypot(axisX, axisY) <= c.r {
			return
		}
		s0 := memoFitSide(first.pieceKey, first.pieces, c.cx, c.cy, axisX, axisY, a.scale)
		s1 := memoFitSide(second.pieceKey, second.pieces, c.cx, c.cy, axisX, axisY, a.scale)
		if s0 == 0 || s1 == 0 || s0 == s1 {
			return
		}
	}
	fullySeparated := make(map[[2]int]struct{})
	margin := rat(1e-11 * a.scale * a.scale)
	for i := range certs {
		for j := i + 1; j < len(certs); j++ {
			pair := pairKey(certs[i].index, certs[j].index)
			if certs[i].arc == certs[j].arc ||
				exactFitBoundsSeparate(certs[i].bounds, certs[j].bounds, margin) ||
				memoFitHullsSeparate(certs[i].hullKey, certs[i].hulls, certs[j].hullKey, certs[j].hulls, a.scale) {
				fullySeparated[pair] = struct{}{}
				continue
			}
			if !exactFitBoundsSeparate(certs[i].outerBounds, certs[j].outerBounds, margin) &&
				!memoFitHullsSeparate(certs[i].outerKey, certs[i].outerHulls, certs[j].outerKey, certs[j].outerHulls, a.scale) {
				return
			}
		}
	}

	if a.specialHandled == nil {
		a.specialHandled = make(map[[2]int]struct{}, len(fits)*(len(fits)+len(arcs)+1))
	}
	for pair := range fullySeparated {
		a.specialHandled[pair] = struct{}{}
	}
	a.fitExactAbove = make(map[int]float64, len(fits))
	for i, index := range fits {
		a.fitExactAbove[index] = certs[i].root.ti
		for _, arc := range arcs {
			a.specialHandled[pairKey(index, arc)] = struct{}{}
		}
		cert := certs[i]
		e := cert.root
		pair := pairKey(index, circle)
		if index > circle {
			e.ti, e.tj = e.tj, e.ti
		}
		a.events[pair] = []xEvent{e}
		a.specialHandled[pair] = struct{}{}
		a.applyAnalyticCut(index, cert.root.ti, cert.root.x, cert.root.y)
		a.applyAnalyticCut(circle, cert.root.tj, cert.root.x, cert.root.y)

		e = xEvent{x: cert.end[0], y: cert.end[1], ti: 1, tj: cert.arcEnd, kind: evCross}
		pair = pairKey(index, cert.arc)
		if index > cert.arc {
			e.ti, e.tj = e.tj, e.ti
		}
		a.events[pair] = []xEvent{e}
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

// exactFitHull is the convex hull witness for one interpolant piece. The
// curve lies in the hull of these exact Bernstein control points.
type exactFitHull struct {
	points [4][2]*big.Rat
}

// exactFitBounds encloses every Bezier hull in a fit. A gap between these
// boxes proves separation of all piece pairs without projecting each hull.
type exactFitBounds struct {
	min, max [2]*big.Rat
}

func exactFitHullBounds(hulls []exactFitHull) exactFitBounds {
	var bounds exactFitBounds
	for _, hull := range hulls {
		for _, point := range hull.points {
			for axis := range 2 {
				if bounds.min[axis] == nil || point[axis].Cmp(bounds.min[axis]) < 0 {
					bounds.min[axis] = point[axis]
				}
				if bounds.max[axis] == nil || point[axis].Cmp(bounds.max[axis]) > 0 {
					bounds.max[axis] = point[axis]
				}
			}
		}
	}
	return bounds
}

func exactFitBoundsSeparate(a, b exactFitBounds, margin *big.Rat) bool {
	if margin == nil || margin.Sign() <= 0 || a.min[0] == nil || b.min[0] == nil {
		return false
	}
	for axis := range 2 {
		if add(a.max[axis], margin).Cmp(b.min[axis]) < 0 ||
			add(b.max[axis], margin).Cmp(a.min[axis]) < 0 {
			return true
		}
	}
	return false
}

func exactFitHulls(pieces []exactFitPiece) []exactFitHull {
	hulls := make([]exactFitHull, len(pieces))
	third := new(big.Rat).SetFrac64(1, 3)
	twoThirds := new(big.Rat).SetFrac64(2, 3)
	for i, p := range pieces {
		h := &hulls[i]
		for axis, coeff := range [2][4]*big.Rat{p.x, p.y} {
			h.points[0][axis] = coeff[0]
			h.points[1][axis] = add(coeff[0], mul(coeff[1], third))
			h.points[2][axis] = add(add(coeff[0], mul(coeff[1], twoThirds)), mul(coeff[2], third))
			h.points[3][axis] = add(add(coeff[0], coeff[1]), add(coeff[2], coeff[3]))
		}
	}
	return hulls
}

// exactFitHullsAfter bounds the portion at or above cut. cut is chosen below
// the certified root crossing, so it includes the whole exterior flank.
func exactFitHullsAfter(pieces []exactFitPiece, cut float64) []exactFitHull {
	all := exactFitHulls(pieces)
	remaining := make([]exactFitHull, 0, len(all))
	for i, piece := range pieces {
		if piece.t1 <= cut {
			continue
		}
		if piece.t0 >= cut {
			remaining = append(remaining, all[i])
			continue
		}
		u := quo(sub(rat(cut), rat(piece.t0)), sub(rat(piece.t1), rat(piece.t0)))
		var right exactFitHull
		for axis := range 2 {
			p := all[i].points
			q0 := add(p[0][axis], mul(u, sub(p[1][axis], p[0][axis])))
			q1 := add(p[1][axis], mul(u, sub(p[2][axis], p[1][axis])))
			q2 := add(p[2][axis], mul(u, sub(p[3][axis], p[2][axis])))
			r0 := add(q0, mul(u, sub(q1, q0)))
			r1 := add(q1, mul(u, sub(q2, q1)))
			right.points[0][axis] = add(r0, mul(u, sub(r1, r0)))
			right.points[1][axis] = r1
			right.points[2][axis] = q2
			right.points[3][axis] = p[3][axis]
		}
		remaining = append(remaining, right)
	}
	return remaining
}

func fitCutBelowRoot(pieces []exactFitPiece, cut, cx, cy, root, scale float64) bool {
	margin := rat(1e-11 * scale * scale)
	for i := range pieces {
		piece := &pieces[i]
		if cut < piece.t0 || cut > piece.t1 {
			continue
		}
		u := quo(sub(rat(cut), rat(piece.t0)), sub(rat(piece.t1), rat(piece.t0)))
		return evalRatPoly(radialPoly(piece, cx, cy, root), u).Cmp(new(big.Rat).Neg(margin)) < 0
	}
	return false
}

func exactFitHullsSeparate(a, b []exactFitHull, scale float64) bool {
	margin := rat(1e-11 * scale * scale)
	if margin == nil || margin.Sign() <= 0 {
		return false
	}
	for _, left := range a {
		for _, right := range b {
			if fitHullAxisSeparate(&left, &right, rat(1), rat(0), margin) ||
				fitHullAxisSeparate(&left, &right, rat(0), rat(1), margin) {
				continue
			}
			separated := false
			for _, hull := range [2]*exactFitHull{&left, &right} {
				for i := range hull.points {
					for j := i + 1; j < len(hull.points); j++ {
						dx := sub(hull.points[j][0], hull.points[i][0])
						dy := sub(hull.points[j][1], hull.points[i][1])
						if dx.Sign() == 0 && dy.Sign() == 0 {
							continue
						}
						if fitHullAxisSeparate(&left, &right, new(big.Rat).Neg(dy), dx, margin) {
							separated = true
							break
						}
					}
					if separated {
						break
					}
				}
				if separated {
					break
				}
			}
			if !separated {
				return false
			}
		}
	}
	return true
}

func fitHullAxisSeparate(a, b *exactFitHull, ax, ay, margin *big.Rat) bool {
	if ax.Sign() == 0 && ay.Sign() == 0 {
		return false
	}
	project := func(h *exactFitHull) (*big.Rat, *big.Rat) {
		first := add(mul(ax, h.points[0][0]), mul(ay, h.points[0][1]))
		lo, hi := first, first
		for _, p := range h.points[1:] {
			value := add(mul(ax, p[0]), mul(ay, p[1]))
			if value.Cmp(lo) < 0 {
				lo = value
			}
			if value.Cmp(hi) > 0 {
				hi = value
			}
		}
		return lo, hi
	}
	loA, hiA := project(a)
	loB, hiB := project(b)
	return add(hiA, margin).Cmp(loB) < 0 || add(hiB, margin).Cmp(loA) < 0
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
	// The two radius polynomials differ only in their constant term.
	tipShift := sub(mul(rat(root), rat(root)), mul(rat(tip), rat(tip)))
	negMargin := new(big.Rat).Neg(margin)
	var crossing *exactFitPiece
	var crossingPoly []*big.Rat
	var firstRootStart, lastRootEnd, lastTipEnd *big.Rat
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
		rootEnd := evalRatPoly(f, rat(1))
		tipEnd := add(rootEnd, tipShift)
		if i == 0 {
			firstRootStart = f[0]
		}
		if i+1 == len(pieces) {
			lastRootEnd, lastTipEnd = rootEnd, tipEnd
		}
		lo, hi := f[0].Sign(), rootEnd.Sign()
		if lo < 0 && hi > 0 {
			if crossing != nil {
				return 0, false
			}
			crossing, crossingPoly = piece, f
		} else if lo == 0 || hi == 0 || (lo > 0 && hi < 0) {
			return 0, false
		}
		if i+1 < len(pieces) && tipEnd.Cmp(negMargin) >= 0 {
			return 0, false
		}
	}
	if crossing == nil || firstRootStart.Sign() >= 0 || lastRootEnd.Sign() <= 0 {
		return 0, false
	}
	if new(big.Rat).Abs(lastTipEnd).Cmp(margin) > 0 {
		return 0, false
	}
	u, ok := fitRoundedRoot(crossingPoly)
	if !ok {
		lo, hi := rat(0), rat(1)
		for range 80 {
			mid := quo(add(lo, hi), rat(2))
			if evalRatPoly(crossingPoly, mid).Sign() < 0 {
				lo = mid
			} else {
				hi = mid
			}
		}
		u, _ = quo(add(lo, hi), rat(2)).Float64()
	}
	t := crossing.t0 + u*(crossing.t1-crossing.t0)
	return t, finiteVal(t) && t > crossing.t0 && t < crossing.t1
}

// fitRoundedRoot finds the same float64 that the fixed 80-step exact bisection
// returns when a fast estimate can be certified away from both rounding ties.
// The derivative was proven positive over this piece before the call.
func fitRoundedRoot(f []*big.Rat) (float64, bool) {
	coeff := make([]float64, len(f))
	for i, c := range f {
		coeff[i], _ = c.Float64()
		if !finiteVal(coeff[i]) {
			return 0, false
		}
	}
	lo, hi := 0.0, 1.0
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
		if v < 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	u := (lo + hi) / 2
	derivative := make([]*big.Rat, len(f)-1)
	for i := 1; i < len(f); i++ {
		derivative[i-1] = mul(f[i], rat(float64(i)))
	}
	for range 2 {
		if !(u > 0 && u < 1) {
			return 0, false
		}
		value, _ := evalRatPoly(f, rat(u)).Float64()
		slope, _ := evalRatPoly(derivative, rat(u)).Float64()
		if !finiteVal(value) || !finiteVal(slope) || slope <= 0 {
			return 0, false
		}
		u -= value / slope
	}
	if !(u > 0 && u < 1) {
		return 0, false
	}
	// After 80 exact bisections the enclosing interval is 2^-80 wide.
	// Prove the true root is farther than that from both rounding boundaries.
	below := math.Nextafter(u, math.Inf(-1))
	above := math.Nextafter(u, math.Inf(1))
	margin := rat(math.Ldexp(1, -80))
	lower := add(quo(add(rat(below), rat(u)), rat(2)), margin)
	upper := sub(quo(add(rat(u), rat(above)), rat(2)), margin)
	if lower.Cmp(upper) >= 0 || evalRatPoly(f, lower).Sign() >= 0 || evalRatPoly(f, upper).Sign() <= 0 {
		return 0, false
	}
	return u, true
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

// polyMul multiplies two exact polynomials. Every coefficient is a float64, or
// a float64 divided by a small integer, so each polynomial has a small common
// denominator. The product is formed over integers and normalized once per
// coefficient, where summing Rat products would normalize once per product. The
// result equals the term-by-term Rat sum exactly: a Rat is stored in lowest
// terms, so equal values are equal Rats.
func polyMul(a, b []*big.Rat) []*big.Rat {
	ai, da := scaleToInts(a)
	bi, db := scaleToInts(b)
	acc := make([]*big.Int, len(a)+len(b)-1)
	for i := range acc {
		acc[i] = new(big.Int)
	}
	var term big.Int
	for i, x := range ai {
		for j, y := range bi {
			acc[i+j].Add(acc[i+j], term.Mul(x, y))
		}
	}
	den := new(big.Int).Mul(da, db)
	out := make([]*big.Rat, len(acc))
	for k, v := range acc {
		out[k] = new(big.Rat).SetFrac(v, den)
	}
	return out
}

// scaleToInts returns the least common multiple of the denominators of vs and
// each value as an integer numerator over it.
func scaleToInts(vs []*big.Rat) ([]*big.Int, *big.Int) {
	den := big.NewInt(1)
	var g big.Int
	for _, v := range vs {
		d := v.Denom()
		g.GCD(nil, nil, den, d)
		den.Mul(den, new(big.Int).Quo(d, &g))
	}
	out := make([]*big.Int, len(vs))
	for i, v := range vs {
		n := new(big.Int).Quo(den, v.Denom())
		out[i] = n.Mul(n, v.Num())
	}
	return out, den
}

func evalRatPoly(p []*big.Rat, x *big.Rat) *big.Rat {
	out := rat(0)
	for i := len(p) - 1; i >= 0; i-- {
		out = add(mul(out, x), p[i])
	}
	return out
}

// bernstein converts power coefficients to same-degree Bernstein coefficients.
// Their convex hull contains every value of the polynomial on [0,1]. Each
// coefficient is the sum of p[i]·C(k,i)/C(n,i); it is formed over the common
// denominator of p and of every C(n,i), then normalized once.
func bernstein(p []*big.Rat) []*big.Rat {
	n := len(p) - 1
	lcm := big.NewInt(1)
	var g big.Int
	for i := 0; i <= n; i++ {
		c := big.NewInt(binomial(n, i))
		g.GCD(nil, nil, lcm, c)
		lcm.Mul(lcm, c.Quo(c, &g))
	}
	pi, dp := scaleToInts(p)
	den := new(big.Int).Mul(dp, lcm)
	out := make([]*big.Rat, len(p))
	var term, weight big.Int
	for k := range out {
		sum := new(big.Int)
		for i := 0; i <= k; i++ {
			weight.Quo(lcm, big.NewInt(binomial(n, i)))
			weight.Mul(&weight, big.NewInt(binomial(k, i)))
			sum.Add(sum, term.Mul(pi[i], &weight))
		}
		out[k] = new(big.Rat).SetFrac(sum, den)
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
