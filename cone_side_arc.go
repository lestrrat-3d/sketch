package sketch

import (
	"math"
	"math/big"
)

// coneArcDepth and coneArcBudget bound the general arc proof: a wedge is halved
// at most coneArcDepth times, and the whole proof tests at most coneArcBudget
// boxes. An unresolved wedge refuses.
const (
	coneArcDepth  = 12
	coneArcBudget = 2048
	// coneSqrtBits is the binary precision of the rational square-root bounds.
	coneSqrtBits = 100
)

// coneRay is a nonzero exact direction vector.
type coneRay [2]*big.Rat

// cross returns the exact cross product r × o.
// Lean proofs depend on this function; see _proofs/trace.json.
func (r coneRay) cross(o coneRay) *big.Rat {
	return coneSub(coneMul(r[0], o[1]), coneMul(r[1], o[0]))
}

// coneSqrtBounds returns rationals lo <= sqrt(q) <= hi for q > 0. Both bounds
// are exact: they come from the integer square root, never from a float.
// Lean proofs depend on this function; see _proofs/trace.json.
func coneSqrtBounds(q *big.Rat) (*big.Rat, *big.Rat, bool) {
	if q.Sign() <= 0 {
		return nil, nil, false
	}
	num, den := new(big.Int).Set(q.Num()), new(big.Int).Set(q.Denom())
	k := coneSqrtBits - (num.BitLen()-den.BitLen())/2
	if k >= 0 {
		num.Lsh(num, uint(2*k))
	} else {
		den.Lsh(den, uint(-2*k))
	}
	root := new(big.Int).Sqrt(new(big.Int).Quo(num, den)) // floor(sqrt(q*4^k))
	if root.Sign() <= 0 {
		return nil, nil, false
	}
	scale := new(big.Rat).SetFrac(big.NewInt(1), big.NewInt(1))
	if k >= 0 {
		scale.SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), uint(k)))
	} else {
		scale.SetFrac(new(big.Int).Lsh(big.NewInt(1), uint(-k)), big.NewInt(1))
	}
	lo := new(big.Rat).Mul(new(big.Rat).SetInt(root), scale)
	hi := new(big.Rat).Mul(new(big.Rat).SetInt(new(big.Int).Add(root, big.NewInt(1))), scale)
	return lo, hi, true
}

// coneUnit encloses v/|v| coordinate-wise.
// Lean proofs depend on this function; see _proofs/trace.json.
func coneUnit(v coneRay) ([2]coneInterval, bool) {
	lo, hi, ok := coneSqrtBounds(coneAdd(coneMul(v[0], v[0]), coneMul(v[1], v[1])))
	if !ok {
		return [2]coneInterval{}, false
	}
	inv := coneInterval{new(big.Rat).Inv(hi), new(big.Rat).Inv(lo)}
	return [2]coneInterval{inv.scale(v[0]), inv.scale(v[1])}, true
}

// coneMulInterval returns the exact hull of the four corner products of a and b.
// Lean proofs depend on this function; see _proofs/trace.json.
func coneMulInterval(a, b coneInterval) coneInterval {
	p := [4]*big.Rat{coneMul(a.lo, b.lo), coneMul(a.lo, b.hi), coneMul(a.hi, b.lo), coneMul(a.hi, b.hi)}
	lo, hi := p[0], p[0]
	for _, x := range p[1:] {
		if x.Cmp(lo) < 0 {
			lo = x
		}
		if x.Cmp(hi) > 0 {
			hi = x
		}
	}
	return coneInterval{lo, hi}
}

// coneOutward returns float64-exact rational bounds containing [lo, hi].
// Lean proofs depend on this function; see _proofs/trace.json.
func coneOutward(i coneInterval) (coneInterval, bool) {
	lo, _ := i.lo.Float64()
	hi, _ := i.hi.Float64()
	if !coneFinite(lo, hi) {
		return coneInterval{}, false
	}
	if coneRat(lo).Cmp(i.lo) > 0 {
		lo = math.Nextafter(lo, math.Inf(-1))
	}
	if coneRat(hi).Cmp(i.hi) < 0 {
		hi = math.Nextafter(hi, math.Inf(1))
	}
	if !coneFinite(lo, hi) {
		return coneInterval{}, false
	}
	return coneInterval{coneRat(lo), coneRat(hi)}, true
}

// coneArcWedge proves one cone side for the exact circular carrier of an arc:
// the points center + radius*u(t), where u runs counter-clockwise from the
// start ray to the end ray. This is the carrier Arc.CertifiedSpanBounds
// encloses: Center and Start fix the radius, End fixes only the final direction.
type coneArcWedge struct {
	form   coneSideForm
	cx, cy *big.Rat
	radius coneInterval
	budget int
}

// arcWedgeSide is the general arc side proof. It accepts any counter-clockwise
// arc of positive sweep strictly short of a half turn, at any position. The
// sweep is decided exactly by the cross product of the start and end rays, not
// by Arc.Sweep. It halves the angular wedge until each piece's coordinate box
// has one exact sign under boxSide, and refuses otherwise.
// Lean proofs depend on this function; see _proofs/trace.json.
func (f coneSideForm) arcWedgeSide(arc *Arc) (ConeSide, *big.Rat) {
	cx, cy := arc.Center.X(), arc.Center.Y()
	if !coneFinite(cx, cy, arc.Start.X(), arc.Start.Y(), arc.End.X(), arc.End.Y()) {
		return 0, nil
	}
	start := coneRay{coneSub(coneRat(arc.Start.X()), coneRat(cx)), coneSub(coneRat(arc.Start.Y()), coneRat(cy))}
	end := coneRay{coneSub(coneRat(arc.End.X()), coneRat(cx)), coneSub(coneRat(arc.End.Y()), coneRat(cy))}
	if start.cross(end).Sign() <= 0 { // zero sweep, a full turn, or a sweep of a half turn or more
		return 0, nil
	}
	rLo, rHi, ok := coneSqrtBounds(coneAdd(coneMul(start[0], start[0]), coneMul(start[1], start[1])))
	if !ok {
		return 0, nil
	}
	w := &coneArcWedge{form: f, cx: coneRat(cx), cy: coneRat(cy),
		radius: coneInterval{rLo, rHi}, budget: coneArcBudget}
	return w.side(start, end, coneArcDepth)
}

// side proves one cone side for the carrier piece between rays a and b, halving the
// wedge up to depth times while the shared budget lasts. Both halves must prove the
// same side; the smaller margin wins.
// Lean proofs depend on this function; see _proofs/trace.json.
func (w *coneArcWedge) side(a, b coneRay, depth int) (ConeSide, *big.Rat) {
	if w.budget == 0 {
		return 0, nil
	}
	w.budget--
	ua, ok := coneUnit(a)
	if !ok {
		return 0, nil
	}
	ub, ok := coneUnit(b)
	if !ok {
		return 0, nil
	}
	if side, margin := w.box(a, b, ua, ub); side != 0 {
		return side, margin
	}
	if depth == 0 {
		return 0, nil
	}
	m, ok := coneSplit(a, b)
	if !ok {
		return 0, nil
	}
	s0, m0 := w.side(a, m, depth-1)
	if s0 == 0 {
		return 0, nil
	}
	s1, m1 := w.side(m, b, depth-1)
	if s1 != s0 {
		return 0, nil
	}
	if m0.Cmp(m1) <= 0 {
		return s0, m0
	}
	return s0, m1
}

// coneSplit returns a ray strictly inside the wedge from a to b, proved by exact
// cross products. The ray is only a divider: its direction need not be exact.
// Lean proofs depend on this function; see _proofs/trace.json.
func coneSplit(a, b coneRay) (coneRay, bool) {
	var f [4]float64
	f[0], _ = a[0].Float64()
	f[1], _ = a[1].Float64()
	f[2], _ = b[0].Float64()
	f[3], _ = b[1].Float64()
	la, lb := math.Hypot(f[0], f[1]), math.Hypot(f[2], f[3])
	if !coneFinite(la, lb, f[0], f[1], f[2], f[3]) || la == 0 || lb == 0 {
		return coneRay{}, false
	}
	mx, my := f[0]/la+f[2]/lb, f[1]/la+f[3]/lb
	if !coneFinite(mx, my) {
		return coneRay{}, false
	}
	m := coneRay{coneRat(mx), coneRat(my)}
	if a.cross(m).Sign() <= 0 || m.cross(b).Sign() <= 0 {
		return coneRay{}, false
	}
	return m, true
}

// box encloses the carrier piece between rays a and b (a counter-clockwise
// wedge narrower than a half turn) in an axis-aligned rational box and asks
// boxSide for its sign. On the unit circle each coordinate is monotone between
// its axis extremes, so it ranges between the endpoint values unless the wedge
// contains an axis direction, whose coordinate then reaches exactly +-1.
// Lean proofs depend on this function; see _proofs/trace.json.
func (w *coneArcWedge) box(a, b coneRay, ua, ub [2]coneInterval) (ConeSide, *big.Rat) {
	one, zero := coneRat(1), coneRat(0)
	axes := [2][2]coneRay{
		{{one, zero}, {coneRat(-1), zero}}, // +x, -x
		{{zero, one}, {zero, coneRat(-1)}}, // +y, -y
	}
	var span [2]coneInterval
	for i := range span {
		lo, hi := ua[i].lo, ua[i].hi
		if ub[i].lo.Cmp(lo) < 0 {
			lo = ub[i].lo
		}
		if ub[i].hi.Cmp(hi) > 0 {
			hi = ub[i].hi
		}
		if coneWedgeHas(a, b, axes[i][0]) {
			hi = one
		}
		if coneWedgeHas(a, b, axes[i][1]) {
			lo = coneRat(-1)
		}
		span[i] = coneInterval{lo, hi}
	}
	u := coneMulInterval(w.radius, span[0])
	v := coneMulInterval(w.radius, span[1])
	u = coneInterval{coneAdd(u.lo, w.cx), coneAdd(u.hi, w.cx)}
	v = coneInterval{coneAdd(v.lo, w.cy), coneAdd(v.hi, w.cy)}
	u, ok := coneOutward(u)
	if !ok {
		return 0, nil
	}
	v, ok = coneOutward(v)
	if !ok {
		return 0, nil
	}
	return w.form.boxSide(u, v)
}

// coneWedgeHas reports whether direction e lies in the closed wedge from a to b.
// Lean proofs depend on this function; see _proofs/trace.json.
func coneWedgeHas(a, b, e coneRay) bool {
	return a.cross(e).Sign() >= 0 && e.cross(b).Sign() >= 0
}
