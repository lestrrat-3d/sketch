package geom

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
)

// FitSpline is an open spline that INTERPOLATES its fit points: the curve passes
// through every fit point. Unlike the control-point [Spline] (whose polygon only
// approximates), the fit points are the durable handles — the interpolating cubic
// is recomputed from their current coordinates on every evaluation, so the curve
// keeps passing through them as the solver moves them. It carries no extra solver
// unknowns and no internal constraints. The interpolation is a natural cubic spline
// with chord-length parameterization.
type FitSpline struct {
	Fit []*Point
}

// ErrTooFewFitPoints is returned by [NewFitSpline] when given fewer than the two
// fit points an interpolating spline requires.
var ErrTooFewFitPoints = errors.New("geom: a fit-point spline requires at least 2 fit points")

// NewFitSpline returns a fit-point (interpolating) spline through the given fit
// points. It returns [ErrTooFewFitPoints] with fewer than 2.
func NewFitSpline(fit ...*Point) (*FitSpline, error) {
	if len(fit) < 2 {
		return nil, fmt.Errorf("%w, got %d", ErrTooFewFitPoints, len(fit))
	}
	return &FitSpline{Fit: fit}, nil
}

// Endpoints returns the fit spline's endpoints: its first and last fit points,
// which the curve passes through exactly. Returns nil for an empty fit set.
func (sp *FitSpline) Endpoints() (*Point, *Point) {
	if len(sp.Fit) == 0 {
		return nil, nil
	}
	return sp.Fit[0], sp.Fit[len(sp.Fit)-1]
}

// Eval returns the curve point at parameter t ∈ [0, 1] (normalized chord length;
// values outside are clamped).
func (sp *FitSpline) Eval(t float64) (float64, float64) {
	p := newFitEvaluator(controlCoords(sp.Fit)).at(t)
	return p[0], p[1]
}

// Polyline samples the curve at segments+1 evenly spaced (in chord-length) points.
func (sp *FitSpline) Polyline(segments int) [][2]float64 {
	// fit-point count is guaranteed >=2 by the FitSpline constructor.
	pts, _ := SampleFitSpline(controlCoords(sp.Fit), segments)
	return pts
}

// EvalFitSpline evaluates the natural cubic interpolating spline through the given
// fit coordinates at t ∈ [0, 1] (normalized chord length). A one-off evaluation —
// for many samples use [SampleFitSpline], which reuses one evaluator. It returns
// [ErrTooFewFitPoints] with fewer than 2 fit points.
func EvalFitSpline(fit [][2]float64, t float64) (float64, float64, error) {
	if err := tooFewPoints(len(fit), 2, ErrTooFewFitPoints); err != nil {
		return 0, 0, err
	}
	p := newFitEvaluator(fit).at(t)
	return p[0], p[1], nil
}

// EvalFitSplineDeriv returns the first derivative dS/dt of the natural-cubic
// interpolating spline through fit at t ∈ [0, 1] (normalized chord length;
// clamped). A one-off evaluation — for many samples build a fitEvaluator once. It
// returns [ErrTooFewFitPoints] with fewer than 2 fit points.
func EvalFitSplineDeriv(fit [][2]float64, t float64) (float64, float64, error) {
	if err := tooFewPoints(len(fit), 2, ErrTooFewFitPoints); err != nil {
		return 0, 0, err
	}
	d := newFitEvaluator(fit).derivAt(t)
	return d[0], d[1], nil
}

// SampleFitSpline samples the interpolating spline at segments+1 evenly spaced
// parameters (minimum 2 segments), reusing a single natural-cubic evaluator (the
// tridiagonal solve runs once, not per sample). It returns [ErrTooFewFitPoints]
// with fewer than 2 fit points.
func SampleFitSpline(fit [][2]float64, segments int) ([][2]float64, error) {
	if err := tooFewPoints(len(fit), 2, ErrTooFewFitPoints); err != nil {
		return nil, err
	}
	if segments < 2 {
		segments = 2
	}
	e := newFitEvaluator(fit)
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		pts[i] = e.at(float64(i) / float64(segments))
	}
	return pts, nil
}

// FitInterpolant is the built natural-cubic interpolant a [FitSpline] evaluates:
// the curve's DEFINING data rather than points on it. A consumer that has to
// integrate, record or re-express the exact curve — rather than sample it — reads
// it here, or reads the same curve in per-span polynomial form from [FitInterpolant.Spans].
//
// Its parameter p is CUMULATIVE CHORD LENGTH, running from Params[0] == 0 to the
// total Params[len(Params)-1]. The normalized t ∈ [0, 1] that [FitSpline.Eval] and
// the arrangement's fragment bounds are stated in maps to it as p = t·total.
//
// On span i (Params[i] ≤ p ≤ Params[i+1], with h = Params[i+1]−Params[i],
// a = (Params[i+1]−p)/h and b = (p−Params[i])/h) each coordinate v of the curve is
//
//	v(p) = a·v[i] + b·v[i+1] + ((a³−a)·m[i] + (b³−b)·m[i+1])·h²/6
//
// with v the coordinate's Points column and m its SecondDerivs column. That is the
// arrangement [FitInterpolant.Eval] itself computes, so a reconstruction using it
// reproduces [FitSpline.Eval] bit for bit.
//
// The three slices are parallel and Params is strictly increasing from Params[0] == 0.
// A value the constructors return is read WHOLE: [NewFitInterpolant], [FitSpline.Interpolant]
// and the sketch layer's own FitSpline.Interpolant refuse with [ErrNonFiniteFitInterpolant]
// rather than hand back a value the rule below would shorten, so an interpolant they
// returned never describes less than the curve it was built from.
//
// A HAND-BUILT value that violates the preconditions is read over its COHERENT PREFIX:
// the longest leading run of points, bounded by the shortest of the three slices, whose
// Params start at 0, stay finite and strictly increase, whose own coordinates are finite,
// and whose spans have finite coefficients (see [FitSpan]) — a span width that is positive
// and finite can still be wide enough to put h²·m beyond floating point, and a coefficient
// that is not a number describes no curve at all. Everything from the
// first violation onward is not part of the curve. [FitInterpolant.Eval],
// [FitInterpolant.EvalDeriv] and [FitInterpolant.Spans] all read exactly that prefix, so no
// two of them can describe different curves and none of them can return an infinity or a
// NaN; a one-point prefix evaluates as that point with a zero tangent and no spans, and an
// empty prefix evaluates as zero with no spans, exactly as an empty interpolant does.
type FitInterpolant struct {
	// Params holds the cumulative chord-length parameter of each point in Points,
	// with Params[0] == 0.
	Params []float64
	// Points holds the ACTIVE fit positions: consecutive coincident fit points are
	// collapsed, so these are the points the evaluated curve interpolates rather
	// than the raw fit list. An all-coincident fit set leaves one point and no span.
	Points [][2]float64
	// SecondDerivs holds the natural-cubic second derivatives d²/dp² at Points, per
	// coordinate. Both ends are zero — the natural end condition.
	SecondDerivs [][2]float64
}

// ErrNonFiniteFitInterpolant is returned by [NewFitInterpolant] and
// [FitSpline.Interpolant] when the fit coordinates give an interpolant that cannot be
// described: a cumulative chord length that overflows to a non-finite parameter, or a
// span whose published coefficients do. Such an interpolant would be read over a
// shortened prefix (see [FitInterpolant]), describing a different — usually much
// shorter — curve than the fit points do, so it is refused instead of returned.
var ErrNonFiniteFitInterpolant = errors.New("geom: the fit coordinates give a non-finite interpolant")

// Interpolant returns the interpolant [FitSpline.Eval] evaluates, built from the fit
// points' current coordinates. Each call builds a fresh one, so the caller owns the
// returned slices. A spline with no fit points returns an empty interpolant. It returns
// [ErrNonFiniteFitInterpolant] when the fit coordinates give an interpolant that cannot
// be described — one whose cumulative chord length or span coefficients overflow — since
// a shortened description of the curve would be worse than none.
func (sp *FitSpline) Interpolant() (*FitInterpolant, error) {
	if len(sp.Fit) == 0 {
		return &FitInterpolant{}, nil
	}
	return newFitEvaluator(controlCoords(sp.Fit)).checkedInterpolant()
}

// NewFitInterpolant builds the natural-cubic interpolant through the given fit
// coordinates — the same one [EvalFitSpline] and [SampleFitSpline] evaluate. It
// returns [ErrTooFewFitPoints] with fewer than 2 fit points, and
// [ErrNonFiniteFitInterpolant] when the coordinates — finite though they are — give
// an interpolant that cannot be described (see [FitInterpolant]).
func NewFitInterpolant(fit [][2]float64) (*FitInterpolant, error) {
	if err := tooFewPoints(len(fit), 2, ErrTooFewFitPoints); err != nil {
		return nil, err
	}
	return newFitEvaluator(fit).checkedInterpolant()
}

// Eval returns the curve point at normalized parameter t ∈ [0, 1] (clamped). It is
// the same computation on the same values as [FitSpline.Eval], so the two agree bit
// for bit. It rebuilds the internal evaluator per call — sample many points with
// [SampleFitSpline] instead.
func (fi *FitInterpolant) Eval(t float64) (float64, float64) {
	e := fi.evaluator()
	if len(e.x) == 0 {
		return 0, 0
	}
	p := e.at(t)
	return p[0], p[1]
}

// EvalDeriv returns the tangent dS/dt at normalized parameter t ∈ [0, 1] (clamped),
// matching [EvalFitSplineDeriv]. A degenerate interpolant — a single point, or a
// hand-built value whose coherent prefix (see [FitInterpolant]) is one point — has
// zero tangent. Like the other two readers it reads that prefix and nothing beyond it,
// so it never returns an infinity or a NaN.
func (fi *FitInterpolant) EvalDeriv(t float64) (float64, float64) {
	e := fi.evaluator()
	if len(e.x) == 0 {
		return 0, 0
	}
	d := e.derivAt(t)
	return d[0], d[1]
}

// FitSpan is one cubic piece of a [FitInterpolant] in monomial form over the span's
// own NORMALIZED local parameter: with h = PEnd−PStart and u = (p−PStart)/h running
// over [0, 1] as p runs over [PStart, PEnd], the curve is
//
//	x(p) = X[0] + u·(X[1] + u·(X[2] + u·X[3]))
//	y(p) = Y[0] + u·(Y[1] + u·(Y[2] + u·Y[3]))
//
// with p the cumulative chord parameter and TStart/TEnd the same bounds normalized
// to the [0, 1] the rest of the API is stated in. The coefficients are per unit u,
// so the polynomial's derivative is too: divide it by h for d/dp, and multiply that
// by the total chord length (the interpolant's last Params entry) for d/dt.
//
// The polynomial is algebraically the cubic [FitInterpolant.Eval] evaluates, summed
// in a different order — so it agrees to rounding, not bit for bit. The bound that
// holds is a RELATIVE one at every span width: in u each coefficient is on the scale
// of the curve's own displacement across the span, so no term is lost to the span's
// width. (In the absolute parameter p the higher coefficients carry 1/h and 1/h²
// factors, which underflow to zero on a wide span and silently drop whole terms of
// an otherwise ordinary curve.) Use Eval where bit-identity matters.
type FitSpan struct {
	TStart, TEnd float64
	PStart, PEnd float64
	X, Y         [4]float64
}

// Spans returns the interpolant's cubic pieces in monomial form — each in its own
// normalized local parameter, see [FitSpan] for the evaluation formula — one per
// interval between consecutive [FitInterpolant.Points], in order. A degenerate
// (single-point) interpolant has none. A hand-built value is spanned over its
// coherent prefix only (see [FitInterpolant]), the same prefix
// [FitInterpolant.Eval] evaluates. So every coefficient returned here is finite: a
// span whose coefficients are not finite is outside that prefix, and is no more part
// of the curve for Eval than it is here.
func (fi *FitInterpolant) Spans() []FitSpan {
	k := fi.size()
	if k < 2 {
		return nil
	}
	total := fi.Params[k-1]
	spans := make([]FitSpan, 0, k-1)
	for i := 0; i < k-1; i++ {
		h := fi.Params[i+1] - fi.Params[i]
		spans = append(spans, FitSpan{
			TStart: fi.Params[i] / total,
			TEnd:   fi.Params[i+1] / total,
			PStart: fi.Params[i],
			PEnd:   fi.Params[i+1],
			X:      cubicSpanCoeffs(h, fi.Points[i][0], fi.Points[i+1][0], fi.SecondDerivs[i][0], fi.SecondDerivs[i+1][0]),
			Y:      cubicSpanCoeffs(h, fi.Points[i][1], fi.Points[i+1][1], fi.SecondDerivs[i][1], fi.SecondDerivs[i+1][1]),
		})
	}
	return spans
}

// cubicSpanCoeffs converts one natural-cubic span from second-derivative form to
// monomial coefficients c0..c3 in the span's own normalized parameter
// u = (p − pStart)/h, over a span of length h between values v0, v1 whose second
// derivatives are m0, m1:
//
//	c0 = v0
//	c1 = (v1−v0) − h²·(2·m0 + m1)/6
//	c2 = h²·m0/2
//	c3 = h²·(m1−m0)/6
//
// Every h² product is associated as h·(h·term), NEVER as (h·h)·term: on a span of
// width ~1e307 the bare h·h is already +Inf while the answer is perfectly ordinary
// (the terms of a real interpolant are ~1/h² there), and the overflow never comes
// back — it reaches the caller as an infinite coefficient, or as a NaN wherever the
// term is zero.
func cubicSpanCoeffs(h, v0, v1, m0, m1 float64) [4]float64 {
	return [4]float64{
		v0,
		(v1 - v0) - h*(h*((2*m0+m1)/6)),
		h * (h * (m0 / 2)),
		h * (h * ((m1 - m0) / 6)),
	}
}

// size returns the number of points the interpolant is coherent over: the longest
// LEADING run of points, bounded by the shortest of its three parallel slices, whose
// Params start at 0, stay finite and strictly increase, whose coordinates are finite,
// and whose spans have finite coefficients. The constructors refuse to return
// a value this would shorten, so for one of theirs it is the whole length; a hand-built
// one is read over the prefix alone, and it is the ONE definition every exported reader
// uses, so they cannot describe different curves.
func (fi *FitInterpolant) size() int {
	k := min(len(fi.Params), len(fi.Points), len(fi.SecondDerivs))
	if k == 0 || fi.Params[0] != 0 || !finitePair(fi.Points[0]) {
		return 0
	}
	for i := 1; i < k; i++ {
		p := fi.Params[i]
		// A non-finite or non-increasing parameter has no span: h would be zero,
		// negative or NaN, and the coefficients it produces are not on any curve.
		// NaN fails every comparison, so it is rejected explicitly.
		if math.IsNaN(p) || math.IsInf(p, 0) || p <= fi.Params[i-1] {
			return i
		}
		// A positive finite h is not enough: h²·m0/2 and h²·(m1−m0)/6 can still
		// overflow, and Spans would then publish an infinite coefficient — a span
		// no polynomial describes, so it is no more part of the curve than a bad
		// parameter is. A non-finite coordinate lands in c0 the same way.
		if !finitePair(fi.Points[i]) || !fi.spanFinite(i-1) {
			return i
		}
	}
	return k
}

// spanFinite reports whether span i — between Points[i] and Points[i+1] — has finite
// coefficients in both coordinates, i.e. whether [FitInterpolant.Spans] can describe it
// at all. It runs the SAME conversion Spans publishes, so it judges the coefficients a
// caller actually reads rather than an equivalent form of them.
func (fi *FitInterpolant) spanFinite(i int) bool {
	h := fi.Params[i+1] - fi.Params[i]
	x := cubicSpanCoeffs(h, fi.Points[i][0], fi.Points[i+1][0], fi.SecondDerivs[i][0], fi.SecondDerivs[i+1][0])
	y := cubicSpanCoeffs(h, fi.Points[i][1], fi.Points[i+1][1], fi.SecondDerivs[i][1], fi.SecondDerivs[i+1][1])
	for k := range x {
		if !finiteVal(x[k]) || !finiteVal(y[k]) {
			return false
		}
	}
	return true
}

// finitePair reports whether both coordinates of p are ordinary numbers.
func finitePair(p [2]float64) bool {
	return finiteVal(p[0]) && finiteVal(p[1])
}

// finiteVal reports whether v is an ordinary number — neither NaN nor an infinity.
func finiteVal(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// evaluator rebuilds the internal evaluator over the coherent prefix of the
// interpolant's own values, so evaluation reads exactly the data the caller sees.
func (fi *FitInterpolant) evaluator() *fitEvaluator {
	k := fi.size()
	e := &fitEvaluator{
		t:  fi.Params[:k],
		x:  make([]float64, k),
		y:  make([]float64, k),
		mx: make([]float64, k),
		my: make([]float64, k),
	}
	for i := 0; i < k; i++ {
		e.x[i], e.y[i] = fi.Points[i][0], fi.Points[i][1]
		e.mx[i], e.my[i] = fi.SecondDerivs[i][0], fi.SecondDerivs[i][1]
	}
	return e
}

// checkedInterpolant exports the built evaluator's values, refusing an interpolant the
// coherent-prefix rule would shorten. The evaluator accumulates chord length in
// floating point, so fit coordinates that are each finite can still overflow a
// parameter or a span coefficient; truncating the export there would describe a
// single point (or a short leading piece) of a curve the same fit points evaluate
// whole, with no error anywhere. It is the ONE gate every constructor exports through.
func (e *fitEvaluator) checkedInterpolant() (*FitInterpolant, error) {
	fi := e.interpolant()
	if n := fi.size(); n != len(fi.Params) {
		return nil, fmt.Errorf("%w: describable over %d of its %d points", ErrNonFiniteFitInterpolant, n, len(fi.Params))
	}
	return fi, nil
}

// interpolant exports the built evaluator's values as the public [FitInterpolant],
// copying them rather than recomputing so the two can never disagree.
func (e *fitEvaluator) interpolant() *FitInterpolant {
	fi := &FitInterpolant{
		Params:       slices.Clone(e.t),
		Points:       make([][2]float64, len(e.x)),
		SecondDerivs: make([][2]float64, len(e.x)),
	}
	for i := range e.x {
		fi.Points[i] = [2]float64{e.x[i], e.y[i]}
		fi.SecondDerivs[i] = [2]float64{e.mx[i], e.my[i]}
	}
	return fi
}

// fitEvaluator holds a built natural-cubic interpolant: the active (deduplicated)
// fit positions, their cumulative chord parameters, and the per-coordinate second
// derivatives. Build it once and reuse it across samples.
type fitEvaluator struct {
	t      []float64 // cumulative chord parameters, t[0]=0
	x, y   []float64 // active fit positions (consecutive coincident points collapsed)
	mx, my []float64 // natural-cubic second derivatives in x and y
}

// fitChordEps collapses consecutive fit points closer than this (a zero-length
// chord span has no parameterization); the natural-cubic system needs positive
// chord lengths.
const fitChordEps = 1e-12

func newFitEvaluator(fit [][2]float64) *fitEvaluator {
	var px, py []float64
	for _, p := range fit {
		if len(px) == 0 || math.Hypot(p[0]-px[len(px)-1], p[1]-py[len(py)-1]) > fitChordEps {
			px = append(px, p[0])
			py = append(py, p[1])
		}
	}
	e := &fitEvaluator{x: px, y: py}
	k := len(px)
	if k == 0 { // every fit point coincided away (caller guards len(fit) >= 1)
		e.x, e.y = []float64{fit[0][0]}, []float64{fit[0][1]}
		e.t = []float64{0}
		e.mx, e.my = []float64{0}, []float64{0}
		return e
	}
	e.t = make([]float64, k)
	for i := 1; i < k; i++ {
		e.t[i] = e.t[i-1] + math.Hypot(px[i]-px[i-1], py[i]-py[i-1])
	}
	e.mx = naturalSecondDerivs(e.t, px)
	e.my = naturalSecondDerivs(e.t, py)
	return e
}

// at returns the curve point at normalized chord parameter s ∈ [0, 1] (clamped).
func (e *fitEvaluator) at(s float64) [2]float64 {
	k := len(e.x)
	if k == 1 {
		return [2]float64{e.x[0], e.y[0]}
	}
	total := e.t[k-1]
	p := s * total
	if p <= 0 {
		return [2]float64{e.x[0], e.y[0]}
	}
	if p >= total {
		return [2]float64{e.x[k-1], e.y[k-1]}
	}
	i := sort.SearchFloat64s(e.t, p) - 1
	if i < 0 {
		i = 0
	} else if i > k-2 {
		i = k - 2
	}
	return [2]float64{
		evalCubicSpan(e.t, e.x, e.mx, i, p),
		evalCubicSpan(e.t, e.y, e.my, i, p),
	}
}

// evalCubicSpan evaluates one natural-cubic span i at parameter value p ∈
// [t[i], t[i+1]]. At p == t[i] it returns v[i] exactly (the curve interpolates).
func evalCubicSpan(t, v, m []float64, i int, p float64) float64 {
	h := t[i+1] - t[i]
	a := (t[i+1] - p) / h
	b := (p - t[i]) / h
	return a*v[i] + b*v[i+1] + ((a*a*a-a)*m[i]+(b*b*b-b)*m[i+1])*h*h/6
}

// derivAt returns the curve tangent dS/dt at normalized chord parameter s ∈
// [0, 1] (clamped), where t is the normalized chord length. dS/dt = total·dS/dp,
// p = s·total being the un-normalized chord parameter. A degenerate (single
// active point) spline has zero tangent.
func (e *fitEvaluator) derivAt(s float64) [2]float64 {
	k := len(e.x)
	if k == 1 {
		return [2]float64{}
	}
	total := e.t[k-1]
	p := s * total
	if p < 0 {
		p = 0
	} else if p > total {
		p = total
	}
	i := sort.SearchFloat64s(e.t, p) - 1
	if i < 0 {
		i = 0
	} else if i > k-2 {
		i = k - 2
	}
	return [2]float64{
		total * derivCubicSpan(e.t, e.x, e.mx, i, p),
		total * derivCubicSpan(e.t, e.y, e.my, i, p),
	}
}

// interiorBreaks returns the interior knot parameters (normalized chord length
// in (0,1)) where consecutive natural-cubic pieces meet — the span boundaries an
// exact per-span integration must not straddle. A spline with fewer than three
// active points is a single span and has none.
func (e *fitEvaluator) interiorBreaks() []float64 {
	k := len(e.x)
	if k < 3 {
		return nil
	}
	total := e.t[k-1]
	out := make([]float64, 0, k-2)
	for i := 1; i < k-1; i++ {
		out = append(out, e.t[i]/total)
	}
	return out
}

// derivCubicSpan returns d/dp of one natural-cubic span i at parameter value
// p ∈ [t[i], t[i+1]] (the per-coordinate derivative of [evalCubicSpan]).
func derivCubicSpan(t, v, m []float64, i int, p float64) float64 {
	h := t[i+1] - t[i]
	a := (t[i+1] - p) / h
	b := (p - t[i]) / h
	return (v[i+1]-v[i])/h + h/6*(-(3*a*a-1)*m[i]+(3*b*b-1)*m[i+1])
}

// naturalSecondDerivs solves the tridiagonal system for the second derivatives of
// a natural cubic spline (M[0] = M[last] = 0) interpolating values v at parameters
// t, via the Thomas algorithm. Fewer than three points has no interior unknown, so
// every M is zero (a straight-line / single-point span).
func naturalSecondDerivs(t, v []float64) []float64 {
	k := len(v)
	m := make([]float64, k)
	if k < 3 {
		return m
	}
	n := k - 1 // last index
	sz := n - 1
	sub := make([]float64, sz)
	diag := make([]float64, sz)
	sup := make([]float64, sz)
	rhs := make([]float64, sz)
	for idx := 0; idx < sz; idx++ {
		i := idx + 1 // interior index 1..n-1
		h0 := t[i] - t[i-1]
		h1 := t[i+1] - t[i]
		sub[idx] = h0
		diag[idx] = 2 * (h0 + h1)
		sup[idx] = h1
		rhs[idx] = 6 * ((v[i+1]-v[i])/h1 - (v[i]-v[i-1])/h0)
	}
	sol := thomasSolve(sub, diag, sup, rhs)
	for idx := 0; idx < sz; idx++ {
		m[idx+1] = sol[idx]
	}
	return m
}

// thomasSolve solves a tridiagonal system (sub, diag, sup are the three diagonals,
// rhs the right-hand side) by the Thomas algorithm. sub[0] and sup[n-1] are unused.
func thomasSolve(sub, diag, sup, rhs []float64) []float64 {
	n := len(rhs)
	cp := make([]float64, n)
	dp := make([]float64, n)
	cp[0] = sup[0] / diag[0]
	dp[0] = rhs[0] / diag[0]
	for i := 1; i < n; i++ {
		denom := diag[i] - sub[i]*cp[i-1]
		cp[i] = sup[i] / denom
		dp[i] = (rhs[i] - sub[i]*dp[i-1]) / denom
	}
	x := make([]float64, n)
	x[n-1] = dp[n-1]
	for i := n - 2; i >= 0; i-- {
		x[i] = dp[i] - cp[i]*x[i+1]
	}
	return x
}
