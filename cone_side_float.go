package sketch

import "math/big"

// Float prefilter for the exact cone-side box proofs.
//
// The prefilter never decides a side and never produces a margin. It only
// proves that the exact box test would return no side, so the exact rational
// box is skipped and the piece is bisected as before. Anything it cannot
// settle, including a non-finite value, defers to the exact code, which still
// produces every published side and margin.
//
// Each exact interval endpoint (a rational) is tracked by a float Interval
// that encloses it, built from the outward-rounded helpers in interval.go.

// fiv encloses the two exact endpoints of a coneInterval: lo holds the
// exact lower endpoint and hi the exact upper endpoint.
type fiv struct{ lo, hi Interval }

// ratEnclose returns a float interval holding r. big.Rat.Float64 returns the
// nearest float; when it is exact the interval is that point, otherwise it is
// widened one float each way.
// Lean proofs depend on this function; see _proofs/trace.json.
func ratEnclose(r *big.Rat) Interval {
	f, exact := r.Float64()
	if exact {
		return pt(f)
	}
	return Interval{down(f), up(f)}
}

// fivAdd encloses the exact interval sum of a and b.
// Lean proofs depend on this function; see _proofs/trace.json.
func fivAdd(a, b fiv) fiv { return fiv{iadd(a.lo, b.lo), iadd(a.hi, b.hi)} }

// fivSub encloses the exact interval difference of a and b.
// Lean proofs depend on this function; see _proofs/trace.json.
func fivSub(a, b fiv) fiv { return fiv{isub(a.lo, b.hi), isub(a.hi, b.lo)} }

// ivMin returns the interval holding the minimum of any x in a and y in b.
// Lean proofs depend on this function; see _proofs/trace.json.
func ivMin(a, b Interval) Interval { return Interval{min(a.Lo, b.Lo), min(a.Hi, b.Hi)} }

// ivMax returns the interval holding the maximum of any x in a and y in b.
// Lean proofs depend on this function; see _proofs/trace.json.
func ivMax(a, b Interval) Interval { return Interval{max(a.Lo, b.Lo), max(a.Hi, b.Hi)} }

// fivScale mirrors coneInterval.scale for a factor k of either sign: the
// endpoints are the minimum and maximum of the two products.
// Lean proofs depend on this function; see _proofs/trace.json.
func fivScale(a fiv, k Interval) fiv {
	p, q := imul(a.lo, k), imul(a.hi, k)
	return fiv{ivMin(p, q), ivMax(p, q)}
}

// fivSquare mirrors coneInterval.square. It reports false for a non-finite
// result. An endpoint whose sign the enclosure cannot settle gets [0, max
// square], which holds every case of the exact square.
// Lean proofs depend on this function; see _proofs/trace.json.
func fivSquare(a fiv) (fiv, bool) {
	l, h := isqr(a.lo), isqr(a.hi)
	var r fiv
	switch {
	case a.lo.Lo >= 0:
		r = fiv{l, h}
	case a.lo.Hi < 0 && a.hi.Hi <= 0:
		r = fiv{h, l}
	case a.lo.Hi < 0 && a.hi.Lo > 0:
		r = fiv{pt(0), ivMax(l, h)}
	default:
		m := Interval{0, max(l.Hi, h.Hi)}
		r = fiv{m, m}
	}
	return r, fivFinite(r)
}

// fivFinite reports whether both endpoint enclosures are finite and ordered.
// The Lean model omits it: it only refuses, never skips (A200).
func fivFinite(a fiv) bool { return a.lo.isFinite() && a.hi.isFinite() }

// coneFloatForm holds a float enclosure of each field of a coneSideForm.
type coneFloatForm struct {
	base, u, v, axis [3]Interval
	slope2           Interval
}

// newConeFloatForm encloses every field of f. It returns nil when an
// enclosure is not finite, which disables the prefilter.
// Lean proofs depend on this function; see _proofs/trace.json.
func newConeFloatForm(f coneSideForm) *coneFloatForm {
	g := &coneFloatForm{slope2: ratEnclose(f.slope2)}
	ok := g.slope2.isFinite()
	for i := range 3 {
		g.base[i], g.u[i] = ratEnclose(f.base[i]), ratEnclose(f.u[i])
		g.v[i], g.axis[i] = ratEnclose(f.v[i]), ratEnclose(f.axis[i])
		ok = ok && g.base[i].isFinite() && g.u[i].isFinite() && g.v[i].isFinite() && g.axis[i].isFinite()
	}
	if !ok {
		return nil
	}
	return g
}

// zeroBox reports whether the exact boxSide on a box inside u x v is proved to
// return no side: either the exact half coordinate's lower bound is not
// positive, or the exact residual interval straddles zero. u and v enclose the
// exact endpoints of the box. Any non-finite enclosure gives false.
// Lean proofs depend on this function; see _proofs/trace.json.
func (g *coneFloatForm) zeroBox(u, v fiv) bool {
	var q [3]fiv
	for i := range q {
		q[i] = fivAdd(fivAdd(fiv{g.base[i], g.base[i]}, fivScale(u, g.u[i])), fivScale(v, g.v[i]))
		if !fivFinite(q[i]) {
			return false
		}
	}
	h := fivAdd(fivAdd(fivScale(q[0], g.axis[0]), fivScale(q[1], g.axis[1])), fivScale(q[2], g.axis[2]))
	if !fivFinite(h) {
		return false
	}
	if h.lo.Hi <= 0 {
		return true
	}
	cx := fivSub(fivScale(q[1], g.axis[2]), fivScale(q[2], g.axis[1]))
	cy := fivSub(fivScale(q[2], g.axis[0]), fivScale(q[0], g.axis[2]))
	cz := fivSub(fivScale(q[0], g.axis[1]), fivScale(q[1], g.axis[0]))
	cx2, ok1 := fivSquare(cx)
	cy2, ok2 := fivSquare(cy)
	cz2, ok3 := fivSquare(cz)
	h2, ok4 := fivSquare(h)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return false
	}
	r := fivSub(fivAdd(fivAdd(cx2, cy2), cz2), fivScale(h2, g.slope2))
	// The exact residual's lower endpoint is at most zero and its upper
	// endpoint at least zero, so boxSide finds no sign.
	return fivFinite(r) && r.lo.Hi <= 0 && r.hi.Lo >= 0
}

// floatSkip reports whether the float prefilter proves that boxSide(u, v)
// returns no side. It is false when the prefilter is off.
// Lean proofs depend on this function; see _proofs/trace.json.
func (f coneSideForm) floatSkip(u, v coneInterval) bool {
	return f.fp != nil && f.fp.zeroBox(fiv{ratEnclose(u.lo), ratEnclose(u.hi)},
		fiv{ratEnclose(v.lo), ratEnclose(v.hi)})
}

// coneArcFloat holds the float enclosures the wedge box test needs: the
// radius bracket and the arc center.
type coneArcFloat struct {
	form    *coneFloatForm
	radius  fiv
	centers [2]Interval
}

// newConeArcFloat encloses the wedge radius bracket [rLo, rHi] and the center
// (cx, cy). It returns nil when the form has no prefilter or an enclosure is
// not finite.
// Lean proofs depend on this function; see _proofs/trace.json.
func (f coneSideForm) newConeArcFloat(rLo, rHi *big.Rat, cx, cy float64) *coneArcFloat {
	if f.fp == nil {
		return nil
	}
	a := &coneArcFloat{form: f.fp, radius: fiv{ratEnclose(rLo), ratEnclose(rHi)},
		centers: [2]Interval{pt(cx), pt(cy)}}
	if !fivFinite(a.radius) || a.radius.lo.Lo < 0 || !a.centers[0].isFinite() || !a.centers[1].isFinite() {
		return nil
	}
	return a
}

// axisCrossSign is the sign of the exact cross product a x e for an axis
// direction e in {(+-1, 0), (0, +-1)}: plus or minus the sign of one
// coordinate of a.
// Lean proofs depend on this function; see _proofs/trace.json.
func axisCrossSign(a coneRay, e [2]int) int {
	if e[1] != 0 {
		return a[0].Sign() * e[1]
	}
	return -a[1].Sign() * e[0]
}

// axisInWedge reports whether the axis direction e lies in the closed wedge
// from a to b; it is coneWedgeHas for axis directions without a big.Rat product.
// Lean proofs depend on this function; see _proofs/trace.json.
func axisInWedge(a, b coneRay, e [2]int) bool {
	return axisCrossSign(a, e) >= 0 && axisCrossSign(b, e) <= 0
}

// unitEnclose returns float enclosures of the endpoints of a coneUnit result.
// Lean proofs depend on this function; see _proofs/trace.json.
func unitEnclose(u [2]coneInterval) [2]fiv {
	var r [2]fiv
	for i := range r {
		r[i] = fiv{ratEnclose(u[i].lo), ratEnclose(u[i].hi)}
	}
	return r
}

// boxZero reports whether the exact coneArcWedge.box is proved to return no
// side. It mirrors box: the unit-direction ranges widened to the axis extremes,
// scaled by the radius bracket and shifted by the center, then tested by
// zeroBox. It is false when the prefilter is off, an enclosure is not finite,
// or a sign it needs is not settled.
// Lean proofs depend on this function; see _proofs/trace.json.
func (w *coneArcWedge) boxZero(a, b coneRay, ua, ub [2]coneInterval) bool {
	fl := w.fl
	if fl == nil {
		return false
	}
	fa, fb := unitEnclose(ua), unitEnclose(ub)
	axes := [2][2][2]int{{{1, 0}, {-1, 0}}, {{0, 1}, {0, -1}}}
	var uv [2]fiv
	for i := range uv {
		lo, hi := ivMin(fa[i].lo, fb[i].lo), ivMax(fa[i].hi, fb[i].hi)
		if axisInWedge(a, b, axes[i][0]) {
			hi = pt(1)
		}
		if axisInWedge(a, b, axes[i][1]) {
			lo = pt(-1)
		}
		// coneMulInterval with a nonnegative radius bracket: each end is one
		// corner product chosen by the sign of the span end.
		var rl, rh Interval
		switch {
		case lo.Lo >= 0:
			rl = imul(fl.radius.lo, lo)
		case lo.Hi < 0:
			rl = imul(fl.radius.hi, lo)
		default:
			return false
		}
		switch {
		case hi.Lo >= 0:
			rh = imul(fl.radius.hi, hi)
		case hi.Hi < 0:
			rh = imul(fl.radius.lo, hi)
		default:
			return false
		}
		uv[i] = fivAdd(fiv{rl, rh}, fiv{fl.centers[i], fl.centers[i]})
		if !fivFinite(uv[i]) || !fivFinite(fa[i]) || !fivFinite(fb[i]) {
			return false
		}
	}
	return fl.form.zeroBox(uv[0], uv[1])
}
