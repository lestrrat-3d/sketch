package sketch

import (
	"context"
	"fmt"
	"math"
	"slices"
)

// FoldError is the refusal [Sketch.Enclose] returns when it proves that the
// branch it follows turns back inside the range: the driving value reaches a
// largest value below hi and the branch cannot be carried past it. It wraps
// [ErrNotCertified] and, like every refusal, carries no enclosure.
//
// The claims are about the EXACT solutions of the sketch's constraint
// equations, as for an [Enclosure], and hold for every value of each
// [WithTargetRange] range and every position in each [WithFixedBox] box:
//
//   - the branch followed from lo is certified as a function of the driving
//     value up to Reached;
//   - past Reached the branch continues as one continuous curve of solutions,
//     on which the driving value reaches a largest value q_f inside Fold, and
//     at q_f the Jacobian of the constraint equations with the driver held is
//     singular;
//   - no continuous path of solutions that starts at the branch's solution at
//     Reached, and never takes the driving value below Reached, reaches a
//     driving value above Fold.Hi.
//
// Every driving value in (Fold.Hi, hi] is therefore out of the branch's reach,
// and Fold.Hi < hi. Every value in [Reached, Fold.Lo] lies on the curve, but
// nothing is claimed about the curve being a function of the driving value
// there. Reached comes from this call's split of [lo, hi]; another call over
// [lo, Reached] splits the range differently and may refuse near its end.
type FoldError struct {
	// Reached is the last driving value, in base units, up to which the
	// branch was certified as a function of the driving value.
	Reached float64
	// Fold encloses q_f, the largest driving value the branch reaches, in
	// base units and outward rounded.
	Fold Interval
}

func (e *FoldError) Error() string {
	return fmt.Sprintf("%v: the branch certified up to %v turns back at a driving value in [%v, %v]", ErrNotCertified, e.Reached, e.Fold.Lo, e.Fold.Hi)
}

// Unwrap returns ErrNotCertified.
func (e *FoldError) Unwrap() error { return ErrNotCertified }

// The fold certificate. Where the piece loop cannot advance past qc, the
// branch is followed further with one free variable x_k held as the parameter
// s and the driving value q solved for. Near a fold that system is regular:
// F_x is singular only along the direction the branch turns in, and choosing
// k as the variable that moves fastest with q keeps the column that replaces
// x_k's (∂F/∂q) out of F_x's range.
//
// The s range is split into pieces, each passing the Krawczyk test below and
// tied to its neighbours exactly as q pieces are, so the solutions inside the
// union U of the pieces' uniqueness boxes are one continuous curve C. The curve
// starts and ends at certified points whose driving values lie below qc, and
// the branch's certified box at qc lies inside U. A path of solutions that
// starts at the branch's point at qc and keeps q ≥ qc is then on C while it
// stays in U; C lies in the interior of U except at its two ends, and both ends
// have q < qc, so the path never leaves U. Its driving value is therefore at
// most the largest q on C. Fold.Hi, the highest q any piece box reaches, bounds
// that from above; Fold.Lo, the highest lower end of a tied endpoint box,
// bounds it from below. The largest q is attained inside C, where dq/ds = 0,
// and along C that happens exactly where F_x is singular, because
// [F_x without column k, ∂F/∂q] is not.

// foldTolRel is how wide Fold may be, relative to 1 + |q_f|: a piece whose
// box reaches further above the best lower bound on q_f is split.
const foldTolRel = 1e-12

// foldMaxSteps bounds each float exploration walk.
const foldMaxSteps = 400

// foldCurve is the system with variable k held at s and q solved for. ck is
// k's column, which holds ∂F/∂q instead.
type foldCurve struct {
	r     *encloseRun
	k, ck int
}

// pointEnv is the evaluation point (x, q) in degenerate intervals.
func (f *foldCurve) pointEnv(x []float64, q float64) (*certEnv, bool) {
	box := make([]Interval, len(x))
	for i, v := range x {
		box[i] = pt(v)
	}
	sin, cos, ok := f.r.sys.trig(pt(q))
	return &certEnv{box: box, q: pt(q), sinQ: sin, cosQ: cos}, ok
}

// linearize returns, in float arithmetic at (x, q), the residual, the Jacobian
// with column ck replaced by ∂F/∂q, and the replaced column ∂F/∂x_k.
func (f *foldCurve) linearize(x []float64, q float64) ([]float64, [][]float64, []float64, bool) {
	e, ok := f.pointEnv(x, q)
	if !ok {
		return nil, nil, nil, false
	}
	sys := f.r.sys
	n := len(f.r.free)
	Fi := sys.eval(e)
	J := sys.jacobian(e, f.r.col, n)
	dq := sys.dq(e)
	F := make([]float64, len(Fi))
	G := make([][]float64, n)
	Fs := make([]float64, n)
	for i := range Fi {
		F[i] = Fi[i].mid()
	}
	for i := range G {
		G[i] = make([]float64, n)
		for j := range G[i] {
			G[i][j] = J[i][j].mid()
		}
		Fs[i] = G[i][f.ck]
		G[i][f.ck] = dq[i].mid()
	}
	return F, G, Fs, true
}

// solve runs Newton's method for the solution with x_k = s, starting from
// (x, q). It reports false when the iteration does not settle.
func (f *foldCurve) solve(x []float64, q, s float64) ([]float64, float64, bool) {
	x = slices.Clone(x)
	x[f.k] = s
	free := f.r.free
	for range 60 {
		F, G, _, ok := f.linearize(x, q)
		if !ok {
			return nil, 0, false
		}
		Y, ok := invertMatrix(G)
		if !ok {
			return nil, 0, false
		}
		step, scale := 0.0, 1+math.Abs(q)
		for j := range free {
			d := 0.0
			for i := range F {
				d -= Y[j][i] * F[i]
			}
			if j == f.ck {
				q += d
			} else {
				x[free[j]] += d
				scale = max(scale, 1+math.Abs(x[free[j]]))
			}
			step = max(step, math.Abs(d))
		}
		if math.IsNaN(step) || math.IsInf(step, 0) {
			return nil, 0, false
		}
		if step <= 1e-14*scale {
			return x, q, true
		}
	}
	return nil, 0, false
}

// slope returns dq/ds at a solution: G·(du/ds) = −∂F/∂x_k.
func (f *foldCurve) slope(x []float64, q float64) (float64, bool) {
	_, G, Fs, ok := f.linearize(x, q)
	if !ok {
		return 0, false
	}
	Y, ok := invertMatrix(G)
	if !ok {
		return 0, false
	}
	d := 0.0
	for i := range Fs {
		d -= Y[f.ck][i] * Fs[i]
	}
	return d, !math.IsNaN(d)
}

// foldPoint is a float solution on the curve.
type foldPoint struct {
	s, q float64
	x    []float64
}

// newFoldCurve chooses k at the float solution xc (driving value qc): the free
// variable whose tangent component dx/dq is largest. It returns the direction
// of s in which q grows, or false when the tangent is not finite.
func (r *encloseRun) newFoldCurve(xc []float64, qc float64) (*foldCurve, float64, bool) {
	if len(r.free) == 0 {
		return nil, 0, false
	}
	f := &foldCurve{r: r, k: r.free[0], ck: 0}
	_, G, Fs, ok := f.linearize(xc, qc)
	if !ok {
		return nil, 0, false
	}
	// With ck = 0, G is F_x with column 0 replaced by ∂F/∂q and Fs is F_x's
	// column 0; restore F_x to solve F_x·t = −∂F/∂q.
	n := len(r.free)
	A := make([][]float64, n)
	for i := range A {
		A[i] = slices.Clone(G[i])
		A[i][0] = Fs[i]
	}
	Y, ok := invertMatrix(A)
	if !ok {
		return nil, 0, false
	}
	best, bestAbs, sign := -1, 0.0, 0.0
	for j := range n {
		t := 0.0
		for i := range n {
			t -= Y[j][i] * G[i][0]
		}
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return nil, 0, false
		}
		if math.Abs(t) > bestAbs {
			best, bestAbs, sign = j, math.Abs(t), math.Copysign(1, t)
		}
	}
	if best < 0 {
		return nil, 0, false
	}
	f.k, f.ck = r.free[best], best
	return f, sign, true
}

// explore walks the float curve from p in direction dir, doubling the step
// after each success, until q falls below floor. When toTurn is set it instead
// walks until q stops growing and returns the float point where dq/ds changes
// sign. It reports false when the walk fails or the context ends.
func (f *foldCurve) explore(ctx context.Context, p foldPoint, dir, floor float64, toTurn bool) (foldPoint, bool) {
	h := 1e-9 * (1 + math.Abs(p.s))
	for range foldMaxSteps {
		if ctx.Err() != nil {
			return foldPoint{}, false
		}
		s := p.s + dir*h
		x, q, ok := f.solve(p.x, p.q, s)
		if !ok {
			h /= 2
			if h < 1e-15*(1+math.Abs(p.s)) {
				return foldPoint{}, false
			}
			continue
		}
		next := foldPoint{s: s, q: q, x: x}
		if toTurn {
			d, ok := f.slope(x, q)
			if !ok {
				return foldPoint{}, false
			}
			if dir*d <= 0 {
				return f.bisectTurn(p, next, dir)
			}
		} else if q < floor {
			return next, true
		}
		p = next
		h *= 2
	}
	return foldPoint{}, false
}

// bisectTurn narrows [a, b] (q rising at a, not at b, walking in direction
// dir) to the float point where dq/ds changes sign.
func (f *foldCurve) bisectTurn(a, b foldPoint, dir float64) (foldPoint, bool) {
	for range 200 {
		m := a.s + (b.s-a.s)/2
		if m == a.s || m == b.s {
			break
		}
		x, q, ok := f.solve(a.x, a.q, m)
		if !ok {
			return foldPoint{}, false
		}
		d, ok := f.slope(x, q)
		if !ok {
			return foldPoint{}, false
		}
		p := foldPoint{s: m, q: q, x: x}
		if dir*d > 0 {
			a = p
			continue
		}
		b = p
	}
	if b.q > a.q {
		return b, true
	}
	return a, true
}

// certifyFold tries to prove that the branch, certified as a function of the
// driving value up to qc (float solution xc, certified box pc), turns back
// below hi. It returns the fold refusal, nil when no certificate was found, or
// the context's error.
func (r *encloseRun) certifyFold(ctx context.Context, qc float64, xc []float64, pc []Interval, hi float64) (*FoldError, error) {
	fe, ok := r.foldCertificate(ctx, qc, xc, pc, hi)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return fe, nil
}

func (r *encloseRun) foldCertificate(ctx context.Context, qc float64, xc []float64, pc []Interval, hi float64) (*FoldError, bool) {
	f, sigma, ok := r.newFoldCurve(xc, qc)
	if !ok {
		return nil, false
	}
	// Walk the float curve to the turning point, on past it until q is back
	// below qc, and back from qc until q is below it.
	start := foldPoint{s: xc[f.k], q: qc, x: xc}
	floor := qc - 1e-9*(1+math.Abs(qc))
	turn, ok := f.explore(ctx, start, sigma, 0, true)
	if !ok {
		return nil, false
	}
	far, ok := f.explore(ctx, turn, sigma, floor, false)
	if !ok {
		return nil, false
	}
	near, ok := f.explore(ctx, start, -sigma, floor, false)
	if !ok {
		return nil, false
	}
	first, last := near, far
	if first.s > last.s {
		first, last = last, first
	}

	// The box at the float turning point only steers the piece split; the
	// claimed bounds come from the tied pieces alone. Its upper end carries the
	// spread a ranged target or a fixed box gives the turning value.
	pTurn, ok := f.pointBox(turn)
	if !ok {
		return nil, false
	}
	c := &foldCert{f: f, ref: pTurn[len(pTurn)-1].Hi, fLo: math.Inf(-1), fHi: math.Inf(-1), tol: foldTolRel * (1 + math.Abs(turn.q))}
	pFirst, ok := f.pointBox(first)
	if !ok || !(pFirst[len(pFirst)-1].Hi < qc) {
		return nil, false
	}
	mid, pMid, ok := c.leg(ctx, first, pFirst, turn.s)
	if !ok {
		return nil, false
	}
	end, pEnd, ok := c.leg(ctx, mid, pMid, last.s)
	if !ok || !(pEnd[len(pEnd)-1].Hi < qc) || !c.holds(pc, qc, first.s, end.s) || !(c.fHi < hi) {
		return nil, false
	}
	return &FoldError{Reached: qc, Fold: Interval{c.fLo, c.fHi}}, true
}

// pointBox certifies the solution at p.s alone: every variable's box with the
// driving value's appended.
func (f *foldCurve) pointBox(p foldPoint) ([]Interval, bool) {
	K, _, _ := f.r.sys.foldKrawczyk(p.x, p.q, f.k, f.ck, pt(p.s), f.r.free, f.r.col)
	return K, K != nil
}

// foldCert accumulates the certified s pieces of one fold certificate. fLo and
// fHi bound the largest driving value on the curve from the pieces certified
// so far; ref is the float turning point's box, which only steers the split.
type foldCert struct {
	f             *foldCurve
	ref, fLo, fHi float64
	tol           float64
	pieces        []foldPiece
}

// foldPiece is one certified s piece: its range and its uniqueness box X,
// the driving value's component last.
type foldPiece struct {
	S Interval
	X []Interval
}

// leg certifies pieces from a (a float point and its certified box) to the s
// value end, halving a failed piece and doubling after a success as the
// driving-range loop does. It returns the float point and certified box at
// end.
func (c *foldCert) leg(ctx context.Context, a foldPoint, pa []Interval, end float64) (foldPoint, []Interval, bool) {
	if a.s == end {
		return a, pa, true
	}
	dir := math.Copysign(1, end-a.s)
	span := math.Abs(end - a.s)
	minStep := span * 0x1p-30
	step := span
	for a.s != end {
		if ctx.Err() != nil || len(c.pieces) >= c.f.r.maxPieces {
			return foldPoint{}, nil, false
		}
		b := a.s + dir*step
		if dir*(b-end) >= 0 || math.Abs(end-b) < minStep {
			b = end
		}
		xb, pb, ok := c.tryPiece(a, pa, b)
		if !ok {
			step = math.Abs(b-a.s) / 2
			if step < minStep || a.s+dir*step == a.s {
				return foldPoint{}, nil, false
			}
			continue
		}
		step = 2 * math.Abs(b-a.s)
		a, pa = xb, pb
	}
	return a, pa, true
}

// tryPiece attempts the s piece between a.s and b: solve at its midpoint, run
// the Krawczyk test over it, certify the point at b, tie both endpoint boxes
// into the uniqueness box, and keep the box's driving value within tol of the
// best lower bound on the largest one. On success it records the piece and
// returns the float point and certified box at b.
func (c *foldCert) tryPiece(a foldPoint, pa []Interval, b float64) (foldPoint, []Interval, bool) {
	f := c.f
	S := Interval{min(a.s, b), max(a.s, b)}
	m := S.mid()
	xm, qm, ok := f.solve(a.x, a.q, m)
	if !ok {
		return foldPoint{}, nil, false
	}
	K, X, _ := f.r.sys.foldKrawczyk(xm, qm, f.k, f.ck, S, f.r.free, f.r.col)
	if K == nil || !boxWithin(pa, X) {
		return foldPoint{}, nil, false
	}
	xb, qb, ok := f.solve(xm, qm, b)
	if !ok {
		return foldPoint{}, nil, false
	}
	pb := foldPoint{s: b, q: qb, x: xb}
	boxB, ok := f.pointBox(pb)
	if !ok || !boxWithin(boxB, X) {
		return foldPoint{}, nil, false
	}
	fLo := max(c.fLo, pa[len(pa)-1].Lo, boxB[len(boxB)-1].Lo)
	if K[len(K)-1].Hi > max(fLo, c.ref)+c.tol {
		return foldPoint{}, nil, false
	}
	c.fLo, c.fHi = fLo, max(c.fHi, K[len(K)-1].Hi)
	c.pieces = append(c.pieces, foldPiece{S: S, X: X})
	return pb, boxB, true
}

// holds reports whether the branch's certified box pc at qc lies on the curve:
// its x_k interval inside [s0, s1], and every other component (with qc for the
// driving value) inside the uniqueness box of every piece whose s range it
// meets.
func (c *foldCert) holds(pc []Interval, qc, s0, s1 float64) bool {
	k := c.f.k
	if !pc[k].within(Interval{min(s0, s1), max(s0, s1)}) {
		return false
	}
	met := false
	for _, p := range c.pieces {
		if pc[k].Hi < p.S.Lo || pc[k].Lo > p.S.Hi {
			continue
		}
		met = true
		for vi := range pc {
			if vi != k && !pc[vi].within(p.X[vi]) {
				return false
			}
		}
		if !p.X[len(p.X)-1].Contains(qc) {
			return false
		}
	}
	return met
}

// foldKrawczyk runs the Krawczyk test for the system with variable k held at
// every s in S and the driving value an unknown in column ck, around the float
// solution (xt, qt), which must have xt[k] in S. On success it returns K, every
// variable's enclosure (k's is S) with the driving value's appended, and the
// uniqueness box X ⊇ K in the same layout: for every s in S exactly one
// solution lies in X, it lies in K, and it moves continuously with s.
//
// Unlike krawczyk, the residual over S is split by the mean value theorem,
// F(ũ, s) ∈ F(ũ, s̃) + ∂F/∂x_k(ũ, S)·(S − s̃), and Y multiplies ∂F/∂x_k
// before the factor (S − s̃) does. Near the fold the driving value hardly
// moves with s, and Y·∂F/∂x_k carries that cancellation into the q row; the
// unsplit form would widen q's box by the first-order motion of every other
// row instead.
func (sys *certSystem) foldKrawczyk(xt []float64, qt float64, k, ck int, S Interval, free, col []int) ([]Interval, []Interval, string) {
	if !S.Contains(xt[k]) {
		return nil, nil, "the float solution is outside the parameter range"
	}
	n := len(free)
	nv := len(xt)
	ut := func(j int) float64 {
		if j == ck {
			return qt
		}
		return xt[free[j]]
	}
	point := make([]Interval, nv)
	for i, v := range xt {
		point[i] = pt(v)
	}
	base := slices.Clone(point)
	for _, p := range sys.params {
		base[p.vi] = p.rng
	}

	sinT, cosT, ok := sys.trig(pt(qt))
	if !ok {
		return nil, nil, "the driving value is outside the certified trigonometric range"
	}
	e0 := &certEnv{box: point, q: pt(qt), sinQ: sinT, cosQ: cosT}
	J0 := sys.jacobian(e0, col, n)
	dq0 := sys.dq(e0)
	A := make([][]float64, n)
	for i := range A {
		A[i] = make([]float64, n)
		for j := range A[i] {
			A[i][j] = J0[i][j].mid()
		}
		A[i][ck] = dq0[i].mid()
	}
	Y, ok := invertMatrix(A)
	if !ok {
		return nil, nil, "the Jacobian at the float solution is singular"
	}

	eF := &certEnv{box: base, q: pt(qt), sinQ: sinT, cosQ: cosT}
	F := sys.eval(eF)
	var YFs []Interval
	if S.Lo != S.Hi {
		bS := slices.Clone(base)
		bS[k] = S
		JS := sys.jacobian(&certEnv{box: bS, q: pt(qt), sinQ: sinT, cosQ: cosT}, col, n)
		YFs = make([]Interval, n)
		for i := range YFs {
			acc := pt(0)
			for j := range JS {
				acc = iadd(acc, imul(pt(Y[i][j]), JS[j][ck]))
			}
			YFs[i] = acc
		}
	}
	dS := isub(S, pt(xt[k]))
	c := make([]Interval, n)
	for i := range c {
		acc := pt(0)
		for j := range F {
			acc = iadd(acc, imul(pt(Y[i][j]), F[j]))
		}
		if YFs != nil {
			acc = iadd(acc, imul(YFs[i], dS))
		}
		c[i] = ineg(acc)
		if !c[i].isFinite() {
			return nil, nil, "the residual enclosure is not finite"
		}
	}

	Z := slices.Clone(c)
	X := slices.Clone(base)
	X[k] = S
	var Xq Interval
	D := make([]Interval, n)
	K := make([]Interval, n)
	for iter := 0; iter < krawczykMaxIter; iter++ {
		for j := range n {
			z := Z[j]
			w := z.Hi - z.Lo
			v := ut(j)
			tiny := 1e-15 * (1 + math.Abs(v))
			z = Interval{min(z.Lo-0.1*w-tiny, 0), max(z.Hi+0.1*w+tiny, 0)}
			xi := Interval{down(v + z.Lo), up(v + z.Hi)}
			if j == ck {
				Xq = xi
			} else {
				X[free[j]] = xi
			}
			D[j] = isub(xi, pt(v))
		}
		if sys.driverDistance && !(Xq.Lo > 0) {
			return nil, nil, "the driving distance's box is not positive"
		}
		sinX, cosX, ok := sys.trig(Xq)
		if !ok {
			return nil, nil, "the driving value's box is outside the certified trigonometric range"
		}
		eX := &certEnv{box: X, q: Xq, sinQ: sinX, cosQ: cosX}
		if !sys.side(eX) {
			return nil, nil, "an angle's direction condition does not hold over the box"
		}
		JX := sys.jacobian(eX, col, n)
		dqX := sys.dq(eX)
		for i := range JX {
			JX[i][ck] = dqX[i]
		}
		inside := true
		for i := 0; i < n; i++ {
			acc := c[i]
			for j := 0; j < n; j++ {
				cij := pt(0)
				if i == j {
					cij = pt(1)
				}
				for l := 0; l < n; l++ {
					cij = isub(cij, imul(pt(Y[i][l]), JX[l][j]))
				}
				acc = iadd(acc, imul(cij, D[j]))
			}
			K[i] = iadd(pt(ut(i)), acc)
			if !K[i].isFinite() {
				return nil, nil, "the Krawczyk image is not finite"
			}
			xi := Xq
			if i != ck {
				xi = X[free[i]]
			}
			if !K[i].strictlyInside(xi) {
				inside = false
			}
		}
		if inside {
			out := slices.Clone(base)
			out[k] = S
			for i, vi := range free {
				if i != ck {
					out[vi] = K[i]
				}
			}
			return append(out, K[ck]), append(slices.Clone(X), Xq), ""
		}
		for i := range n {
			Z[i] = isub(K[i], pt(ut(i)))
		}
	}
	return nil, nil, "the contraction test did not close"
}
