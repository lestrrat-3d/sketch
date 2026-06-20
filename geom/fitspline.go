package geom

import (
	"errors"
	"fmt"
	"math"
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
	return SampleFitSpline(controlCoords(sp.Fit), segments)
}

// EvalFitSpline evaluates the natural cubic interpolating spline through the given
// fit coordinates at t ∈ [0, 1] (normalized chord length). A one-off evaluation —
// for many samples use [SampleFitSpline], which reuses one evaluator. It panics
// with fewer than 2 fit points.
func EvalFitSpline(fit [][2]float64, t float64) (float64, float64) {
	if len(fit) < 2 {
		panic(fmt.Sprintf("geom: fit-point spline needs at least 2 fit points, got %d", len(fit)))
	}
	p := newFitEvaluator(fit).at(t)
	return p[0], p[1]
}

// EvalFitSplineDeriv returns the first derivative dS/dt of the natural-cubic
// interpolating spline through fit at t ∈ [0, 1] (normalized chord length;
// clamped). A one-off evaluation — for many samples build a fitEvaluator once. It
// panics with fewer than 2 fit points.
func EvalFitSplineDeriv(fit [][2]float64, t float64) (float64, float64) {
	if len(fit) < 2 {
		panic(fmt.Sprintf("geom: fit-point spline needs at least 2 fit points, got %d", len(fit)))
	}
	d := newFitEvaluator(fit).derivAt(t)
	return d[0], d[1]
}

// SampleFitSpline samples the interpolating spline at segments+1 evenly spaced
// parameters (minimum 2 segments), reusing a single natural-cubic evaluator (the
// tridiagonal solve runs once, not per sample). It panics with fewer than 2 fit
// points.
func SampleFitSpline(fit [][2]float64, segments int) [][2]float64 {
	if len(fit) < 2 {
		panic(fmt.Sprintf("geom: fit-point spline needs at least 2 fit points, got %d", len(fit)))
	}
	if segments < 2 {
		segments = 2
	}
	e := newFitEvaluator(fit)
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		pts[i] = e.at(float64(i) / float64(segments))
	}
	return pts
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
