package geom

// NURBS is a general non-uniform rational B-spline of arbitrary degree, the
// transient-geometry analog of the sketch NURBS entity: pure coordinate math, no
// document state. It is a clamped (open) curve whose control points are ordinary
// points, with a non-decreasing clamped knot vector and a per-control weight.
//
//	        Σ N_{i,p}(t)·w_i·P_i
//	C(t) = ────────────────────────
//	          Σ N_{i,p}(t)·w_i
//
// The basis functions N_{i,p} are the standard degree-p B-spline basis over the
// knot vector Knots (length len(Control)+Degree+1, clamped so the curve passes
// through the first and last control point). All weights == 1 is a non-rational
// (plain B-spline) curve.
type NURBS struct {
	Degree  int
	Control []*Point
	Knots   []float64
	Weights []float64
}

// NewNURBS returns a NURBS with the given degree, control points, knot vector and
// weights. It is a bare constructor for math/snapshots — the sketch entity's
// AddNURBS validates the inputs; this assumes well-formed data, matching the
// other geom constructors.
func NewNURBS(degree int, control []*Point, knots, weights []float64) *NURBS {
	return &NURBS{Degree: degree, Control: control, Knots: knots, Weights: weights}
}

// Rational reports whether any weight differs from 1 (a true rational curve). A
// nil/empty Weights slice is non-rational (all weights are 1).
func (c *NURBS) Rational() bool {
	for _, w := range c.Weights {
		if w != 1 {
			return true
		}
	}
	return false
}

// weightAt returns the weight of control point i, treating a nil/short Weights
// slice as all-ones.
func (c *NURBS) weightAt(i int) float64 {
	if i < len(c.Weights) {
		return c.Weights[i]
	}
	return 1
}

// domain returns the parametric interval [Knots[p], Knots[n+1]] the clamped
// curve is defined on (the only span where the basis is a partition of unity).
func (c *NURBS) domain() (lo, hi float64) {
	p := c.Degree
	n := len(c.Control) - 1
	return c.Knots[p], c.Knots[n+1]
}

// Domain returns the parametric interval [lo, hi] the clamped curve is defined
// on. A caller working in a normalized t ∈ [0, 1] maps to a knot parameter with
// u = lo + t·(hi−lo) before calling [NURBS.Eval] / [NURBS.EvalDeriv].
func (c *NURBS) Domain() (lo, hi float64) { return c.domain() }

// findSpan returns the knot span index i such that U[i] <= u < U[i+1], with the
// clamped-end conventions (The NURBS Book A2.1). n = len(control)-1.
func findSpan(n, p int, u float64, U []float64) int {
	if u >= U[n+1] {
		return n
	}
	if u <= U[p] {
		return p
	}
	lo, hi := p, n+1
	mid := (lo + hi) / 2
	for u < U[mid] || u >= U[mid+1] {
		if u < U[mid] {
			hi = mid
		} else {
			lo = mid
		}
		mid = (lo + hi) / 2
	}
	return mid
}

// basisFuns returns the p+1 nonzero degree-p basis values at u in span i (The
// NURBS Book A2.2); the slice is indexed 0..p for control points i-p..i.
func basisFuns(i, p int, u float64, U []float64) []float64 {
	N := make([]float64, p+1)
	left := make([]float64, p+1)
	right := make([]float64, p+1)
	N[0] = 1
	for j := 1; j <= p; j++ {
		left[j] = u - U[i+1-j]
		right[j] = U[i+j] - u
		saved := 0.0
		for r := 0; r < j; r++ {
			temp := N[r] / (right[r+1] + left[j-r])
			N[r] = saved + right[r+1]*temp
			saved = left[j-r] * temp
		}
		N[j] = saved
	}
	return N
}

// dersBasisFuns returns the p+1 nonzero degree-p basis values N and their first
// derivatives dN at u in span i. The derivative uses
//
//	N'_{g,p} = p·(N_{g,p-1}/(U[g+p]−U[g]) − N_{g+1,p-1}/(U[g+p+1]−U[g+1]))
//
// with the lower-degree basis low (length p) supplying N_{g,p-1}.
func dersBasisFuns(i, p int, u float64, U []float64) (N, dN []float64) {
	N = basisFuns(i, p, u, U)
	dN = make([]float64, p+1)
	low := basisFuns(i, p-1, u, U) // length p
	for j := 0; j <= p; j++ {
		g := i - p + j
		var t1, t2 float64
		if j >= 1 {
			if den := U[g+p] - U[g]; den > 0 {
				t1 = low[j-1] / den
			}
		}
		if j <= p-1 {
			if den := U[g+p+1] - U[g+1]; den > 0 {
				t2 = low[j] / den
			}
		}
		dN[j] = float64(p) * (t1 - t2)
	}
	return
}

// Eval returns the curve point at knot parameter u (clamped to the domain). The
// rational point is the homogeneous sum (Σ N·w·P, Σ N·w) divided through.
func (c *NURBS) Eval(u float64) (float64, float64) {
	p := c.Degree
	n := len(c.Control) - 1
	lo, hi := c.domain()
	if u < lo {
		u = lo
	}
	if u > hi {
		u = hi
	}
	sp := findSpan(n, p, u, c.Knots)
	N := basisFuns(sp, p, u, c.Knots)
	var X, Y, W float64
	for j := 0; j <= p; j++ {
		idx := sp - p + j
		w := c.weightAt(idx)
		nw := N[j] * w
		X += nw * c.Control[idx].X
		Y += nw * c.Control[idx].Y
		W += nw
	}
	return X / W, Y / W
}

// EvalDeriv returns the analytic first derivative dC/du at knot parameter u
// (clamped to the domain), via the quotient rule on the homogeneous numerator and
// denominator. It is exact, so a tangent or area integrand built on it carries no
// nested finite difference.
func (c *NURBS) EvalDeriv(u float64) (float64, float64) {
	p := c.Degree
	n := len(c.Control) - 1
	lo, hi := c.domain()
	if u < lo {
		u = lo
	}
	if u > hi {
		u = hi
	}
	sp := findSpan(n, p, u, c.Knots)
	N, dN := dersBasisFuns(sp, p, u, c.Knots)
	var X, Y, W, dX, dY, dW float64
	for j := 0; j <= p; j++ {
		idx := sp - p + j
		w := c.weightAt(idx)
		x, y := c.Control[idx].X, c.Control[idx].Y
		X += N[j] * w * x
		Y += N[j] * w * y
		W += N[j] * w
		dX += dN[j] * w * x
		dY += dN[j] * w * y
		dW += dN[j] * w
	}
	return (dX*W - X*dW) / (W * W), (dY*W - Y*dW) / (W * W)
}

// Endpoints returns the curve's endpoints — its first and last control points,
// which a clamped NURBS passes through — so it satisfies the open-curve Curve
// interface. Returns nil for a curve with no control points.
func (c *NURBS) Endpoints() (*Point, *Point) {
	if len(c.Control) == 0 {
		return nil, nil
	}
	return c.Control[0], c.Control[len(c.Control)-1]
}

// Polyline samples the curve at segments+1 evenly spaced knot parameters across
// the domain.
func (c *NURBS) Polyline(segments int) [][2]float64 {
	if segments < 1 {
		segments = 1
	}
	lo, hi := c.domain()
	out := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		u := lo + (hi-lo)*float64(i)/float64(segments)
		x, y := c.Eval(u)
		out[i] = [2]float64{x, y}
	}
	return out
}

// InteriorKnots returns the distinct interior knot values strictly inside the
// domain — the breakpoints where the piecewise-rational curve changes polynomial
// piece. The area moment integration splits on these so no quadrature panel
// straddles a span boundary.
func (c *NURBS) InteriorKnots() []float64 {
	lo, hi := c.domain()
	var out []float64
	for _, k := range c.Knots {
		if k > lo && k < hi {
			if len(out) == 0 || k != out[len(out)-1] {
				out = append(out, k)
			}
		}
	}
	return out
}

// ClampedUniformKnots returns the common clamped knot vector for n control points
// and the given degree: degree+1 copies of 0, then evenly spaced interior knots,
// then degree+1 copies of 1. It has length n+degree+1. With fewer than
// degree+1 control points (an invalid curve) it returns nil.
func ClampedUniformKnots(n, degree int) []float64 {
	if degree < 1 || n < degree+1 {
		return nil
	}
	m := n + degree + 1 // total knots
	U := make([]float64, m)
	interior := n - degree - 1 // count of interior knots
	for i := 0; i < m; i++ {
		switch {
		case i <= degree:
			U[i] = 0
		case i >= n+1:
			U[i] = 1
		default:
			U[i] = float64(i-degree) / float64(interior+1)
		}
	}
	return U
}
