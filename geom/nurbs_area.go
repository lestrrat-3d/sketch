package geom

import "math"

// nurbsBulgeSpan returns the exact signed area between a NURBS FRAGMENT — the
// curve restricted to natural parameters [t0, t1] in walk order (t in [0, 1],
// mapping linearly across the knot domain) — and the straight chord that closes
// it. It is the NURBS analog of splineBulge / chordArcCorrection: the curve
// moment ½∫(x·y′ − y·x′) dt over the fragment plus the chord-closure term
// ½·(ex·ay − ax·ey), where (ax,ay)/(ex,ey) are the fragment's chord endpoints
// (the dense polyline's first/last vertex). That closure term is exactly the
// arc-vs-chord correction (it equals −½·P(t0)×P(t1)), so this reproduces
// signedPolyArea's decomposition with the sampled curve moment replaced by the
// true-curve integral — sampling-independent. The walk direction (and thus the
// sign) is carried by the order of t0, t1.
func nurbsBulgeSpan(c *NURBS, t0, t1, ax, ay, ex, ey float64) float64 {
	return nurbsMoment(c, t0, t1) + chordClosure(ax, ay, ex, ey)
}

// chordClosure is the chord-closure term ½·(ex·ay − ax·ey) shared by the
// freeform bulge decompositions (splineBulge / nurbsBulgeSpan): the signed area
// of the closing edge from the fragment's chord start (ax,ay) to end (ex,ey),
// matching signedPolyArea's implied closing edge. It equals −½·P(t0)×P(t1).
func chordClosure(ax, ay, ex, ey float64) float64 {
	return 0.5 * (ex*ay - ax*ey)
}

// nurbsMoment returns the exact ½∫(x·y′ − y·x′) dt of a NURBS over the natural
// parameter interval t0→t1 (t in [0, 1], signed by direction). The integral is
// split at every interior knot (mapped back into [0, 1]) so no quadrature panel
// straddles a span boundary, where the piecewise-rational curve changes piece.
// Each sub-span is integrated by:
//
//   - non-rational (all weights 1): the moment integrand is a polynomial of
//     degree 2p−1 on a span, so p-point Gauss–Legendre is EXACT;
//   - rational: a 10-point Gauss–Legendre rule with adaptive bisection to a
//     relative tolerance, integrating the true rational curve.
func nurbsMoment(c *NURBS, t0, t1 float64) float64 {
	// A non-rational span of degree p has a polynomial moment integrand of
	// degree 2p−1, integrated EXACTLY by the p-point Gauss rule — but the
	// tabulated rules top out at 10 points (exact through degree 19, so p ≤ 10).
	// For degree > 10, or any rational curve (whose rational integrand no fixed
	// rule integrates exactly), fall back to the adaptive panel so the area is
	// never a silently-inexact blessed value.
	exact := !c.Rational() && c.Degree <= 10
	g := gaussNodes(c.Degree)
	return momentOverBreaks(t0, t1, nurbsParamBreaks(c), func(a, b float64) float64 {
		if exact {
			return nurbsGaussMoment(c, a, b, g)
		}
		return nurbsAdaptiveMoment(c, a, b)
	})
}

// momentOverBreaks integrates a freeform curve's moment over [t0, t1] by
// splitting the interval at every breakpoint STRICTLY inside (lo, hi) so no
// quadrature panel straddles a piecewise boundary, then summing the caller's
// per-panel quadrature and applying the walk-direction sign. The (lo, hi, sign)
// normalization, the strict-interior break filter, the bounds-build order
// {lo}+breaks+{hi}, and the ×sign at the end are load-bearing: every spline /
// NURBS area result is built on this exact sequence, so it must not be
// reassociated. breaks must already be in ascending order.
func momentOverBreaks(t0, t1 float64, breaks []float64, panel func(a, b float64) float64) float64 {
	lo, hi, sign := t0, t1, 1.0
	if lo > hi {
		lo, hi, sign = hi, lo, -1.0
	}
	bounds := []float64{lo}
	for _, b := range breaks {
		if b > lo && b < hi {
			bounds = append(bounds, b)
		}
	}
	bounds = append(bounds, hi)
	var moment float64
	for i := 0; i+1 < len(bounds); i++ {
		moment += panel(bounds[i], bounds[i+1])
	}
	return sign * moment
}

// nurbsParamBreaks returns the interior knots of c expressed as natural
// parameters in (0, 1) — the span boundaries nurbsMoment must split on.
func nurbsParamBreaks(c *NURBS) []float64 {
	lo, hi := c.domain()
	span := hi - lo
	if span <= 0 {
		return nil
	}
	ks := c.InteriorKnots()
	out := make([]float64, len(ks))
	for i, k := range ks {
		out[i] = (k - lo) / span
	}
	return out
}

// nurbsMomentIntegrand evaluates ½(x·y′ − y·x′) at natural parameter t (in
// [0, 1]), using the chain rule dC/dt = (hi−lo)·dC/du so the integral is in the
// natural parameter. The (hi−lo) factor is constant, so it can be — and is —
// applied once outside; here the integrand returns the per-u moment and the
// caller scales by the span. We fold it in directly for clarity.
func nurbsMomentIntegrand(c *NURBS, t float64) float64 {
	lo, hi := c.domain()
	u := lo + (hi-lo)*t
	x, y := c.Eval(u)
	dxu, dyu := c.EvalDeriv(u)
	// dC/dt = (hi−lo)·dC/du.
	dx, dy := (hi-lo)*dxu, (hi-lo)*dyu
	return 0.5 * (x*dy - y*dx)
}

// nurbsGaussMoment integrates the moment over panel [t0, t1] with a fixed
// Gauss–Legendre rule (exact for the non-rational polynomial integrand).
func nurbsGaussMoment(c *NURBS, t0, t1 float64, g gaussRule) float64 {
	return gaussPanel(g, t0, t1, func(t float64) float64 {
		return nurbsMomentIntegrand(c, t)
	})
}

// gaussPanel integrates f over the panel [t0, t1] with the Gauss–Legendre rule
// g (defined on [-1, 1]), mapping each node into the panel. The operation order
// — mid + half·node for the abscissa, weight·f accumulation, then a single ·half
// scale — is load-bearing for the exact, sampling-independent area results;
// reassociating it would let the floating-point sums drift.
func gaussPanel(g gaussRule, t0, t1 float64, f func(float64) float64) float64 {
	half := 0.5 * (t1 - t0)
	mid := 0.5 * (t0 + t1)
	var sum float64
	for k := range g.nodes {
		sum += g.weights[k] * f(mid+half*g.nodes[k])
	}
	return sum * half
}

// nurbsAdaptiveMoment integrates the rational moment over [t0, t1] with a
// 10-point Gauss rule, bisecting recursively until the refined estimate agrees
// with the coarse one to a relative tolerance, integrating the true curve.
func nurbsAdaptiveMoment(c *NURBS, t0, t1 float64) float64 {
	coarse := nurbsGaussMoment(c, t0, t1, gauss10)
	return nurbsAdaptiveStep(c, t0, t1, coarse, 0)
}

func nurbsAdaptiveStep(c *NURBS, t0, t1, coarse float64, depth int) float64 {
	mid := 0.5 * (t0 + t1)
	left := nurbsGaussMoment(c, t0, mid, gauss10)
	right := nurbsGaussMoment(c, mid, t1, gauss10)
	refined := left + right
	if depth >= 12 || math.Abs(refined-coarse) < 1e-12*(1+math.Abs(refined)) {
		return refined
	}
	return nurbsAdaptiveStep(c, t0, mid, left, depth+1) +
		nurbsAdaptiveStep(c, mid, t1, right, depth+1)
}

// gaussRule is a Gauss–Legendre quadrature rule on [-1, 1].
type gaussRule struct {
	nodes, weights []float64
}

// gaussNodes returns a Gauss–Legendre rule with at least p points — exact for the
// degree-(2p−1) polynomial moment integrand of a non-rational NURBS of degree p.
// Tabulated rules cover the common low degrees; higher degrees fall back to the
// 10-point rule (which is exact for p up to 10 — degree-19 integrand).
func gaussNodes(p int) gaussRule {
	switch {
	case p <= 1:
		return gauss1
	case p == 2:
		return gauss2
	case p == 3:
		return gauss3
	case p == 4:
		return gauss4
	case p == 5:
		return gauss5
	default:
		return gauss10
	}
}

// gauss1 is the 1-point (midpoint) rule, exact through degree 1.
var gauss1 = gaussRule{nodes: []float64{0}, weights: []float64{2}}

// gauss2 is exact through degree 3.
var gauss2 = gaussRule{
	nodes:   []float64{-0.5773502691896257, 0.5773502691896257},
	weights: []float64{1, 1},
}

// gauss4 is exact through degree 7.
var gauss4 = gaussRule{
	nodes: []float64{
		-0.8611363115940526, -0.3399810435848563,
		0.3399810435848563, 0.8611363115940526,
	},
	weights: []float64{
		0.3478548451374538, 0.6521451548625461,
		0.6521451548625461, 0.3478548451374538,
	},
}

// gauss5 is exact through degree 9.
var gauss5 = gaussRule{
	nodes: []float64{
		-0.9061798459386640, -0.5384693101056831, 0,
		0.5384693101056831, 0.9061798459386640,
	},
	weights: []float64{
		0.2369268850561891, 0.4786286704993665, 0.5688888888888889,
		0.4786286704993665, 0.2369268850561891,
	},
}

// gauss10 is exact through degree 19 — the rational adaptive panel rule and the
// fallback for high-degree non-rational curves.
var gauss10 = gaussRule{
	nodes: []float64{
		-0.9739065285171717, -0.8650633666889845, -0.6794095682990244,
		-0.4333953941292472, -0.1488743389816312, 0.1488743389816312,
		0.4333953941292472, 0.6794095682990244, 0.8650633666889845,
		0.9739065285171717,
	},
	weights: []float64{
		0.0666713443086881, 0.1494513491505806, 0.2190863625159820,
		0.2692667193099963, 0.2955242247147529, 0.2955242247147529,
		0.2692667193099963, 0.2190863625159820, 0.1494513491505806,
		0.0666713443086881,
	},
}
