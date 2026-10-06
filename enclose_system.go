package sketch

import (
	"fmt"
	"math"
	"slices"
)

// The certified system restates each supported constraint as an equation with
// the SAME zero set as the solver's residual inside the box being tested, in a
// form whose value and partial derivatives have closed-form interval
// enclosures. Krawczyk's test is invariant under row scaling and needs only
// that the zero sets agree, so a polynomial restatement is as good as the
// residual itself and far easier to bound:
//
//   - coincident, horizontal/vertical, horizontal/vertical points and the
//     signed horizontal/vertical distances are affine already.
//   - point-on-line, collinear, parallel and perpendicular divide a cross or dot
//     product by norm(), which is floored away from zero, so the residual is
//     zero exactly when the product is.
//   - a point-to-point distance |p−q| − d with d > 0 is zero exactly when
//     |p−q|² − d² is.
//   - a signed angle wrapPi(atan2(cross, dot) − θ) is zero exactly when the
//     direction (dot, cross) points along (cos θ, sin θ): cross·cos θ −
//     dot·sin θ = 0 together with cross·sin θ + dot·cos θ > 0. The inequality is
//     checked over the whole box; it also keeps (cross, dot) away from (0, 0),
//     the one point where atan2's value is a convention.
//
// Every other constraint kind refuses the whole sketch with
// ErrUncertifiedConstraint.

// certEnv is one evaluation point of the certified system: an enclosure of
// every sketch variable (a fixed variable is a degenerate interval) and of the
// driving value, with the driver's sine and cosine when it is an angle.
type certEnv struct {
	box        []Interval
	q          Interval
	sinQ, cosQ Interval
}

func (e *certEnv) px(p *Point) Interval { return e.box[p.xi] }
func (e *certEnv) py(p *Point) Interval { return e.box[p.yi] }

// jacWriter accumulates partial derivatives into a dense interval Jacobian
// whose columns are the free variables. A fixed variable has no column and its
// derivative is dropped; a variable named twice by one equation (a point shared
// by both vectors of a cross product) accumulates.
type jacWriter struct {
	J   [][]Interval
	col []int
	row int
}

func (w *jacWriter) add(r, vi int, v Interval) {
	c := w.col[vi]
	if c < 0 {
		return
	}
	w.J[w.row+r][c] = iadd(w.J[w.row+r][c], v)
}

// certEquation is one supported constraint in its certified form.
type certEquation interface {
	rows() int
	eval(e *certEnv, out []Interval) []Interval
	jac(e *certEnv, w *jacWriter)
	// side reports whether the equation's side condition holds over the whole
	// box; equations without one return true.
	side(e *certEnv) bool
}

// --- affine rows -------------------------------------------------------------

type affineTerm struct {
	vi   int
	coef float64 // ±1, so the product is exact
}

// certAffine is Σ coef·var − k, with k the driving value when useQ is set.
type certAffine struct {
	terms []affineTerm
	k     float64
	useQ  bool
}

func (a *certAffine) rows() int { return 1 }

func (a *certAffine) eval(e *certEnv, out []Interval) []Interval {
	v := pt(0)
	for _, t := range a.terms {
		v = iadd(v, imul(pt(t.coef), e.box[t.vi]))
	}
	if a.useQ {
		return append(out, isub(v, e.q))
	}
	return append(out, isub(v, pt(a.k)))
}

func (a *certAffine) jac(_ *certEnv, w *jacWriter) {
	for _, t := range a.terms {
		w.add(0, t.vi, pt(t.coef))
	}
}

func (a *certAffine) side(*certEnv) bool { return true }

// diff is the affine row p − q over one coordinate.
func diff(pi, qi int) []affineTerm { return []affineTerm{{pi, 1}, {qi, -1}} }

// --- cross / dot of two point differences --------------------------------------

// vecOp names the bilinear form a certProduct evaluates.
type vecOp int

const (
	opCross vecOp = iota
	opDot
)

// certProduct is cross(u, v) or dot(u, v) for u = U1 − U0 and v = V1 − V0.
type certProduct struct {
	op             vecOp
	u0, u1, v0, v1 *Point
}

func (c *certProduct) rows() int { return 1 }

func (c *certProduct) vectors(e *certEnv) (Interval, Interval, Interval, Interval) {
	ux := isub(e.px(c.u1), e.px(c.u0))
	uy := isub(e.py(c.u1), e.py(c.u0))
	vx := isub(e.px(c.v1), e.px(c.v0))
	vy := isub(e.py(c.v1), e.py(c.v0))
	return ux, uy, vx, vy
}

func (c *certProduct) eval(e *certEnv, out []Interval) []Interval {
	ux, uy, vx, vy := c.vectors(e)
	if c.op == opCross {
		return append(out, isub(imul(ux, vy), imul(uy, vx)))
	}
	return append(out, iadd(imul(ux, vx), imul(uy, vy)))
}

func (c *certProduct) jac(e *certEnv, w *jacWriter) {
	ux, uy, vx, vy := c.vectors(e)
	// Partials with respect to the components of u and v.
	dux, duy, dvx, dvy := vy, ineg(vx), ineg(uy), ux
	if c.op == opDot {
		dux, duy, dvx, dvy = vx, vy, ux, uy
	}
	writeVecPartials(w, c.u0, c.u1, c.v0, c.v1, dux, duy, dvx, dvy)
}

func (c *certProduct) side(*certEnv) bool { return true }

// writeVecPartials chains partials taken with respect to u = U1 − U0 and
// v = V1 − V0 down to the points' coordinates, for a one-row equation.
func writeVecPartials(w *jacWriter, u0, u1, v0, v1 *Point, dux, duy, dvx, dvy Interval) {
	w.add(0, u1.xi, dux)
	w.add(0, u0.xi, ineg(dux))
	w.add(0, u1.yi, duy)
	w.add(0, u0.yi, ineg(duy))
	w.add(0, v1.xi, dvx)
	w.add(0, v0.xi, ineg(dvx))
	w.add(0, v1.yi, dvy)
	w.add(0, v0.yi, ineg(dvy))
}

// --- point-to-point distance ---------------------------------------------------

// certDistance is |P1 − P2|² − d², with d the driving value when useQ is set.
type certDistance struct {
	p1, p2 *Point
	d      float64
	useQ   bool
}

func (c *certDistance) rows() int { return 1 }

func (c *certDistance) eval(e *certEnv, out []Interval) []Interval {
	dx := isub(e.px(c.p1), e.px(c.p2))
	dy := isub(e.py(c.p1), e.py(c.p2))
	d := pt(c.d)
	if c.useQ {
		d = e.q
	}
	return append(out, isub(iadd(isqr(dx), isqr(dy)), isqr(d)))
}

func (c *certDistance) jac(e *certEnv, w *jacWriter) {
	dx := imul(pt(2), isub(e.px(c.p1), e.px(c.p2)))
	dy := imul(pt(2), isub(e.py(c.p1), e.py(c.p2)))
	w.add(0, c.p1.xi, dx)
	w.add(0, c.p2.xi, ineg(dx))
	w.add(0, c.p1.yi, dy)
	w.add(0, c.p2.yi, ineg(dy))
}

func (c *certDistance) side(*certEnv) bool { return true }

// --- signed angle ---------------------------------------------------------------

// certAngle is cross(d1, d2)·cos θ − dot(d1, d2)·sin θ, with the side
// condition cross·sin θ + dot·cos θ > 0. θ is the driving value when useQ is
// set, else the dimension's own target, whose sine and cosine were enclosed
// once when the system was built.
type certAngle struct {
	l1, l2   *Line
	sin, cos Interval
	useQ     bool
}

func (c *certAngle) rows() int { return 1 }

func (c *certAngle) trig(e *certEnv) (Interval, Interval) {
	if c.useQ {
		return e.sinQ, e.cosQ
	}
	return c.sin, c.cos
}

func (c *certAngle) parts(e *certEnv) (Interval, Interval, Interval, Interval) {
	ux := isub(e.px(c.l1.End), e.px(c.l1.Start))
	uy := isub(e.py(c.l1.End), e.py(c.l1.Start))
	vx := isub(e.px(c.l2.End), e.px(c.l2.Start))
	vy := isub(e.py(c.l2.End), e.py(c.l2.Start))
	return ux, uy, vx, vy
}

func (c *certAngle) eval(e *certEnv, out []Interval) []Interval {
	ux, uy, vx, vy := c.parts(e)
	cross := isub(imul(ux, vy), imul(uy, vx))
	dot := iadd(imul(ux, vx), imul(uy, vy))
	sin, cos := c.trig(e)
	return append(out, isub(imul(cross, cos), imul(dot, sin)))
}

func (c *certAngle) jac(e *certEnv, w *jacWriter) {
	ux, uy, vx, vy := c.parts(e)
	sin, cos := c.trig(e)
	// cos θ·∂cross − sin θ·∂dot, component by component.
	dux := isub(imul(cos, vy), imul(sin, vx))
	duy := isub(imul(cos, ineg(vx)), imul(sin, vy))
	dvx := isub(imul(cos, ineg(uy)), imul(sin, ux))
	dvy := isub(imul(cos, ux), imul(sin, uy))
	writeVecPartials(w, c.l1.Start, c.l1.End, c.l2.Start, c.l2.End, dux, duy, dvx, dvy)
}

func (c *certAngle) side(e *certEnv) bool {
	ux, uy, vx, vy := c.parts(e)
	cross := isub(imul(ux, vy), imul(uy, vx))
	dot := iadd(imul(ux, vx), imul(uy, vy))
	sin, cos := c.trig(e)
	h := iadd(imul(cross, sin), imul(dot, cos))
	return h.Lo > 0
}

// --- driven dimensions -----------------------------------------------------------

// certDriven encloses one driven dimension's measured value over a box. The
// value is in base units (mm or rad); an angle is a mod-2π reading, shifted by
// whole turns to lie near ref.
type certDriven struct {
	d    Dimension
	eval func(e *certEnv) (Interval, bool)
}

// --- the system ------------------------------------------------------------------

// certSystem is a sketch's constraint set in certified form, with the driving
// dimension's value left open.
type certSystem struct {
	eqs         []certEquation
	m           int
	driven      []certDriven
	driverAngle bool
}

// certifiedSystem builds the certified form of every committed constraint, the
// driver's equation reading the open driving value. It refuses the whole sketch
// on the first constraint it has no certified form for.
func (s *Sketch) certifiedSystem(driver Dimension) (*certSystem, error) {
	sys := &certSystem{}
	found := false
	for _, c := range s.cons {
		if d, ok := c.(Dimension); ok && d.Driven() {
			if Constraint(driver) == c {
				return nil, fmt.Errorf("%w: the driver is a driven dimension", ErrNotCertified)
			}
			dv, err := certDrivenOf(d)
			if err != nil {
				return nil, err
			}
			sys.driven = append(sys.driven, dv)
			continue
		}
		isDriver := Constraint(driver) == c
		found = found || isDriver
		eqs, err := certEquationsOf(c, isDriver)
		if err != nil {
			return nil, err
		}
		if isDriver {
			_, sys.driverAngle = c.(*Angle)
		}
		for _, eq := range eqs {
			sys.eqs = append(sys.eqs, eq)
			sys.m += eq.rows()
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: the driver is not a constraint of this sketch", ErrNotCertified)
	}
	return sys, nil
}

func uncertified(c Constraint) error {
	return fmt.Errorf("%w: %s", ErrUncertifiedConstraint, ConstraintKind(c))
}

// certEquationsOf returns the certified form of one driving constraint.
func certEquationsOf(c Constraint, isDriver bool) ([]certEquation, error) {
	switch t := c.(type) {
	case *coincident:
		return []certEquation{
			&certAffine{terms: diff(t.P1.xi, t.P2.xi)},
			&certAffine{terms: diff(t.P1.yi, t.P2.yi)},
		}, nil
	case *horizontal:
		return []certEquation{&certAffine{terms: diff(t.L.Start.yi, t.L.End.yi)}}, nil
	case *vertical:
		return []certEquation{&certAffine{terms: diff(t.L.Start.xi, t.L.End.xi)}}, nil
	case *horizontalPoints:
		return []certEquation{&certAffine{terms: diff(t.P1.yi, t.P2.yi)}}, nil
	case *verticalPoints:
		return []certEquation{&certAffine{terms: diff(t.P1.xi, t.P2.xi)}}, nil
	case *parallel:
		return []certEquation{&certProduct{op: opCross, u0: t.L1.Start, u1: t.L1.End, v0: t.L2.Start, v1: t.L2.End}}, nil
	case *perpendicular:
		return []certEquation{&certProduct{op: opDot, u0: t.L1.Start, u1: t.L1.End, v0: t.L2.Start, v1: t.L2.End}}, nil
	case *pointOnLine:
		return []certEquation{&certProduct{op: opCross, u0: t.L.Start, u1: t.L.End, v0: t.L.Start, v1: t.P}}, nil
	case *collinear:
		l := t.L1
		return []certEquation{
			&certProduct{op: opCross, u0: l.Start, u1: l.End, v0: l.Start, v1: t.L2.Start},
			&certProduct{op: opCross, u0: l.Start, u1: l.End, v0: l.Start, v1: t.L2.End},
		}, nil
	case *Distance:
		d := t.base()
		if !isDriver && !(d > 0) {
			return nil, fmt.Errorf("%w: distance dimension with target %v is not positive", ErrNotCertified, d)
		}
		return []certEquation{&certDistance{p1: t.P1, p2: t.P2, d: d, useQ: isDriver}}, nil
	case *HorizontalDistance:
		return []certEquation{&certAffine{terms: diff(t.P2.xi, t.P1.xi), k: t.base(), useQ: isDriver}}, nil
	case *VerticalDistance:
		return []certEquation{&certAffine{terms: diff(t.P2.yi, t.P1.yi), k: t.base(), useQ: isDriver}}, nil
	case *Angle:
		eq := &certAngle{l1: t.L1, l2: t.L2, useQ: isDriver}
		if !isDriver {
			sin, cos, ok := sinCosPoint(t.base())
			if !ok {
				return nil, fmt.Errorf("%w: angle target %v rad is outside ±%d rad", ErrNotCertified, t.base(), maxTrigArg)
			}
			eq.sin, eq.cos = sin, cos
		}
		return []certEquation{eq}, nil
	}
	return nil, uncertified(c)
}

// certDrivenOf returns the evaluator of one driven dimension's measured value.
func certDrivenOf(d Dimension) (certDriven, error) {
	switch t := d.(type) {
	case *HorizontalDistance:
		return certDriven{d: d, eval: func(e *certEnv) (Interval, bool) {
			return isub(e.px(t.P2), e.px(t.P1)), true
		}}, nil
	case *VerticalDistance:
		return certDriven{d: d, eval: func(e *certEnv) (Interval, bool) {
			return isub(e.py(t.P2), e.py(t.P1)), true
		}}, nil
	case *Distance:
		return certDriven{d: d, eval: func(e *certEnv) (Interval, bool) {
			dx := isub(e.px(t.P1), e.px(t.P2))
			dy := isub(e.py(t.P1), e.py(t.P2))
			return isqrtNonNeg(iadd(isqr(dx), isqr(dy))), true
		}}, nil
	case *Angle:
		return certDriven{d: d, eval: func(e *certEnv) (Interval, bool) {
			eq := certAngle{l1: t.L1, l2: t.L2}
			ux, uy, vx, vy := eq.parts(e)
			cross := isub(imul(ux, vy), imul(uy, vx))
			dot := iadd(imul(ux, vx), imul(uy, vy))
			return atan2Box(cross, dot)
		}}, nil
	}
	return certDriven{}, uncertified(d)
}

// trig encloses the driver's sine and cosine over q when the driver is an
// angle; a driver of any other kind needs neither.
func (sys *certSystem) trig(q Interval) (Interval, Interval, bool) {
	if !sys.driverAngle {
		return Interval{}, Interval{}, true
	}
	return sinCosRange(q)
}

func (sys *certSystem) eval(e *certEnv) []Interval {
	out := make([]Interval, 0, sys.m)
	for _, eq := range sys.eqs {
		out = eq.eval(e, out)
	}
	return out
}

func (sys *certSystem) jacobian(e *certEnv, col []int, n int) [][]Interval {
	J := make([][]Interval, sys.m)
	for i := range J {
		J[i] = make([]Interval, n)
	}
	w := &jacWriter{J: J, col: col}
	for _, eq := range sys.eqs {
		eq.jac(e, w)
		w.row += eq.rows()
	}
	return J
}

func (sys *certSystem) side(e *certEnv) bool {
	for _, eq := range sys.eqs {
		if !eq.side(e) {
			return false
		}
	}
	return true
}

// krawczykMaxIter bounds the inflate-and-test loop. A box that has not closed
// by then is refused.
const krawczykMaxIter = 20

// krawczyk runs the Krawczyk test for the square system F(x, q) = 0 over every
// q in Q, around the float solution xt (solved at qMid ∈ Q). On success it
// returns K, an enclosure of every sketch variable (fixed ones degenerate),
// and X ⊇ K: for every q in Q exactly one solution lies in X, it lies in K, and
// it moves continuously with q. On failure it returns the reason.
//
// The test is the parametric form with F(x̃, Q) in place of the split
// F(x̃, q̃) + F_q·(Q − q̃): for a fixed q the image
// x̃ − Y·F(x̃, q) + (I − Y·F_x(X, q))·(X − x̃) lies inside
// K(X) = x̃ − Y·F(x̃, Q) + (I − Y·F_x(X, Q))·(X − x̃), so K(X) ⊂ int X proves the
// single-q Krawczyk inclusion for every q at once. That inclusion also proves Y
// and every Jacobian in F_x(X, Q) nonsingular, so the implicit function theorem
// gives the continuity. The box is grown by ε-inflation (each pass widens the
// previous image by 10% plus a relative floor, always containing x̃) until the
// image falls strictly inside or the pass budget runs out.
func (sys *certSystem) krawczyk(xt []float64, Q Interval, qMid float64, free, col []int) ([]Interval, []Interval, string) {
	n := len(free)
	point := make([]Interval, len(xt))
	for i, v := range xt {
		point[i] = pt(v)
	}

	// Y ≈ F_x(x̃, q̃)⁻¹, in plain float arithmetic: any matrix is admissible,
	// a good one only makes the test close.
	sinM, cosM, ok := sys.trig(pt(qMid))
	if !ok {
		return nil, nil, "the driving value is outside the certified trigonometric range"
	}
	e0 := &certEnv{box: point, q: pt(qMid), sinQ: sinM, cosQ: cosM}
	J0 := sys.jacobian(e0, col, n)
	A := make([][]float64, n)
	for i := range A {
		A[i] = make([]float64, n)
		for j := range A[i] {
			A[i][j] = J0[i][j].mid()
		}
	}
	Y, ok := invertMatrix(A)
	if !ok {
		return nil, nil, "the Jacobian at the float solution is singular"
	}

	sinQ, cosQ, ok := sys.trig(Q)
	if !ok {
		return nil, nil, "the driving range is outside the certified trigonometric range"
	}
	eQ := &certEnv{box: point, q: Q, sinQ: sinQ, cosQ: cosQ}
	F := sys.eval(eQ)
	// c = −Y·F(x̃, Q): the Newton correction, spread over the driving range.
	c := make([]Interval, n)
	for i := range c {
		acc := pt(0)
		for j := range F {
			acc = iadd(acc, imul(pt(Y[i][j]), F[j]))
		}
		c[i] = ineg(acc)
		if !c[i].isFinite() {
			return nil, nil, "the residual enclosure is not finite"
		}
	}

	Z := slices.Clone(c)
	X := slices.Clone(point)
	D := make([]Interval, n)
	K := make([]Interval, n)
	for iter := 0; iter < krawczykMaxIter; iter++ {
		for j, vi := range free {
			z := Z[j]
			w := z.Hi - z.Lo
			tiny := 1e-15 * (1 + math.Abs(xt[vi]))
			z = Interval{min(z.Lo-0.1*w-tiny, 0), max(z.Hi+0.1*w+tiny, 0)}
			X[vi] = Interval{down(xt[vi] + z.Lo), up(xt[vi] + z.Hi)}
			D[j] = isub(X[vi], pt(xt[vi]))
		}
		eX := &certEnv{box: X, q: Q, sinQ: sinQ, cosQ: cosQ}
		if !sys.side(eX) {
			return nil, nil, "an angle's direction condition does not hold over the box"
		}
		JX := sys.jacobian(eX, col, n)
		inside := true
		for i := 0; i < n; i++ {
			acc := c[i]
			for j := 0; j < n; j++ {
				// C_ij = δ_ij − Σ_k Y_ik·JX_kj
				cij := pt(0)
				if i == j {
					cij = pt(1)
				}
				for k := 0; k < n; k++ {
					cij = isub(cij, imul(pt(Y[i][k]), JX[k][j]))
				}
				acc = iadd(acc, imul(cij, D[j]))
			}
			K[i] = iadd(pt(xt[free[i]]), acc)
			if !K[i].isFinite() {
				return nil, nil, "the Krawczyk image is not finite"
			}
			if !K[i].strictlyInside(X[free[i]]) {
				inside = false
			}
		}
		if inside {
			out := slices.Clone(point)
			for i, vi := range free {
				out[vi] = K[i]
			}
			return out, slices.Clone(X), ""
		}
		for i, vi := range free {
			Z[i] = isub(K[i], pt(xt[vi]))
		}
	}
	return nil, nil, "the contraction test did not close"
}

// invertMatrix inverts A by Gauss–Jordan elimination with partial pivoting,
// reporting false on a zero pivot or a non-finite result. It is the float
// approximate inverse the Krawczyk test needs, never a certified quantity.
func invertMatrix(A [][]float64) ([][]float64, bool) {
	n := len(A)
	M := make([][]float64, n)
	for i := range M {
		M[i] = make([]float64, 2*n)
		copy(M[i], A[i])
		M[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		p := col
		for r := col + 1; r < n; r++ {
			if math.Abs(M[r][col]) > math.Abs(M[p][col]) {
				p = r
			}
		}
		if !(math.Abs(M[p][col]) > 0) {
			return nil, false
		}
		M[col], M[p] = M[p], M[col]
		inv := 1 / M[col][col]
		for j := range M[col] {
			M[col][j] *= inv
		}
		for r := 0; r < n; r++ {
			if r == col || M[r][col] == 0 {
				continue
			}
			f := M[r][col]
			for j := range M[r] {
				M[r][j] -= f * M[col][j]
			}
		}
	}
	Y := make([][]float64, n)
	for i := range Y {
		Y[i] = M[i][n:]
		for _, v := range Y[i] {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, false
			}
		}
	}
	return Y, true
}
