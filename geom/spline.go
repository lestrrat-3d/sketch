package geom

import (
	"errors"
	"fmt"
)

// tooFewPoints builds the kernel guard error when n is below min, wrapping the
// per-family sentinel ([ErrTooFewControlPoints]/[ErrTooFewClosedControlPoints]/
// [ErrTooFewFitPoints]) with the actual count; it returns nil when n is in
// range. It is the shared guard behind the spline-family math kernels (whose
// precondition the public constructors already enforce by returning an error).
func tooFewPoints(n, min int, sentinel error) error {
	if n < min {
		return fmt.Errorf("%w, got %d", sentinel, n)
	}
	return nil
}

// Spline is an open cubic B-spline defined by its control points, using a
// clamped uniform knot vector. Clamping makes the curve start at the first
// control point and end at the last, with end tangents along the first and
// last control-polygon legs. Degree is fixed at 3.
type Spline struct {
	Control []*Point
}

// ErrTooFewControlPoints is returned by [NewSpline] when given fewer than the
// four control points a cubic B-spline requires.
var ErrTooFewControlPoints = errors.New("geom: a cubic B-spline requires at least 4 control points")

// NewSpline returns a cubic B-spline over the given control points. It returns
// [ErrTooFewControlPoints] with fewer than 4 control points.
func NewSpline(control ...*Point) (*Spline, error) {
	if len(control) < 4 {
		return nil, fmt.Errorf("%w, got %d", ErrTooFewControlPoints, len(control))
	}
	return &Spline{Control: control}, nil
}

// Eval returns the curve point at parameter t ∈ [0, 1] (clamped).
func (sp *Spline) Eval(t float64) (float64, float64) {
	// control-point count is guaranteed >=4 by the Spline constructor.
	x, y, _ := EvalCubicBSpline(controlCoords(sp.Control), t)
	return x, y
}

// Polyline samples the spline at segments+1 evenly spaced parameters.
func (sp *Spline) Polyline(segments int) [][2]float64 {
	pts, _ := SampleCubicBSpline(controlCoords(sp.Control), segments)
	return pts
}

func controlCoords(control []*Point) [][2]float64 {
	pts := make([][2]float64, len(control))
	for i, p := range control {
		pts[i] = [2]float64{p.X, p.Y}
	}
	return pts
}

// maxStackKnots is the largest n+4 knot-vector length evalCubicBSplineKnots's
// callers keep on the stack via a fixed-size array; a curve needing more
// falls back to a heap-allocated slice (see clampedKnotsInto).
const maxStackKnots = 64

// clampedKnotsInto appends the clamped uniform knot vector for n control
// points (the same values [ClampedKnots] produces: four zeros, n−4 evenly
// spaced interior knots, four ones) onto buf, returning the result. Passing a
// buf backed by a fixed-size array keeps the vector on the stack up to the
// array's capacity; append growing past it falls back to the heap.
func clampedKnotsInto(buf []float64, n int) []float64 {
	spans := float64(n - 3)
	for i := 0; i < n+4; i++ {
		switch {
		case i < 4:
			buf = append(buf, 0)
		case i >= n:
			buf = append(buf, 1)
		default:
			buf = append(buf, float64(i-3)/spans)
		}
	}
	return buf
}

// evalCubicBSplineKnots is [EvalCubicBSpline]'s body once its clamped knot
// vector is already built, so a caller evaluating many parameters over the
// same control points (a sampler, a nearest-point search) builds the knot
// vector once rather than once per evaluation. ctrl must hold at least 4
// points and knots must be the clamped vector [ClampedKnots](len(ctrl))
// would produce; callers already guarantee both.
func evalCubicBSplineKnots(ctrl [][2]float64, knots []float64, t float64) (float64, float64) {
	n := len(ctrl)
	if t <= 0 {
		return ctrl[0][0], ctrl[0][1]
	}
	if t >= 1 {
		return ctrl[n-1][0], ctrl[n-1][1]
	}
	var x, y float64
	for i := 0; i < n; i++ {
		b := bsplineBasis(i, 3, t, knots)
		x += b * ctrl[i][0]
		y += b * ctrl[i][1]
	}
	return x, y
}

// EvalCubicBSpline evaluates a clamped uniform cubic B-spline over the given
// control coordinates at t ∈ [0, 1] (values outside are clamped). At t = 1
// the last control point is returned directly: the standard half-open
// Cox–de Boor basis is zero at the trailing multiplicity-4 knot, and the
// shortcut is exact for a clamped curve. It returns [ErrTooFewControlPoints]
// with fewer than 4 control points.
func EvalCubicBSpline(ctrl [][2]float64, t float64) (float64, float64, error) {
	n := len(ctrl)
	if err := tooFewPoints(n, 4, ErrTooFewControlPoints); err != nil {
		return 0, 0, err
	}
	var buf [maxStackKnots]float64
	knots := clampedKnotsInto(buf[:0], n)
	x, y := evalCubicBSplineKnots(ctrl, knots, t)
	return x, y, nil
}

// SampleCubicBSpline samples the spline at segments+1 evenly spaced
// parameters (minimum 2 segments). It returns [ErrTooFewControlPoints] with
// fewer than 4 control points.
func SampleCubicBSpline(ctrl [][2]float64, segments int) ([][2]float64, error) {
	n := len(ctrl)
	if err := tooFewPoints(n, 4, ErrTooFewControlPoints); err != nil {
		return nil, err
	}
	if segments < 2 {
		segments = 2
	}
	var buf [maxStackKnots]float64
	knots := clampedKnotsInto(buf[:0], n)
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		x, y := evalCubicBSplineKnots(ctrl, knots, float64(i)/float64(segments))
		pts[i] = [2]float64{x, y}
	}
	return pts, nil
}

// EvalCubicBSplineDeriv returns the first derivative dS/dt of the clamped uniform
// cubic B-spline at t ∈ [0, 1]. The derivative is a degree-2 B-spline over the
// trimmed knot vector (the clamped knots with the first and last removed) with
// control vectors Qᵢ = 3·(Pᵢ₊₁−Pᵢ)/(Uᵢ₊₄−Uᵢ₊₁). At the clamped ends it returns
// the one-sided endpoint tangent (Q₀ at t≤0, Q_{n−2} at t≥1) — the t≥1 shortcut
// is mandatory because the half-open basis is zero at the trailing knot. It
// returns [ErrTooFewControlPoints] with fewer than 4 control points.
func EvalCubicBSplineDeriv(ctrl [][2]float64, t float64) (float64, float64, error) {
	n := len(ctrl)
	if err := tooFewPoints(n, 4, ErrTooFewControlPoints); err != nil {
		return 0, 0, err
	}
	knots := ClampedKnots(n)
	q := func(i int) (float64, float64) {
		den := knots[i+4] - knots[i+1]
		if den <= 0 {
			return 0, 0
		}
		return 3 * (ctrl[i+1][0] - ctrl[i][0]) / den, 3 * (ctrl[i+1][1] - ctrl[i][1]) / den
	}
	if t <= 0 {
		qx, qy := q(0)
		return qx, qy, nil
	}
	if t >= 1 {
		qx, qy := q(n - 2)
		return qx, qy, nil
	}
	dknots := knots[1 : n+3] // trimmed knot vector for the degree-2 derivative basis
	var dx, dy float64
	for i := 0; i < n-1; i++ {
		b := bsplineBasis(i, 2, t, dknots)
		if b == 0 {
			continue
		}
		qx, qy := q(i)
		dx += b * qx
		dy += b * qy
	}
	return dx, dy, nil
}

// NearestParamCubicBSpline returns the parameter t ∈ [0, 1] whose curve point is
// closest to (px, py). It is a robust seed for a foot-point aux variable, not an
// exact projection: a dense polyline broad phase (each segment projected onto,
// not just its samples) locates the best span, then a few golden-section steps
// refine within it. Density scales with the control count so narrow loops are
// not missed. It returns [ErrTooFewControlPoints] with fewer than 4 control
// points.
func NearestParamCubicBSpline(ctrl [][2]float64, px, py float64) (float64, error) {
	n := len(ctrl)
	if err := tooFewPoints(n, 4, ErrTooFewControlPoints); err != nil {
		return 0, err
	}
	var buf [maxStackKnots]float64
	knots := clampedKnotsInto(buf[:0], n)
	eval := func(t float64) (float64, float64) {
		return evalCubicBSplineKnots(ctrl, knots, t)
	}
	return nearestParamSampled(eval, 16*(n-3), false, px, py), nil
}

// ClampedKnots builds the clamped uniform knot vector used by all splines in
// this package for n control points at degree 3: four zeros, n−4 evenly
// spaced interior knots, four ones. Exposed for exporters (e.g. DXF SPLINE).
func ClampedKnots(n int) []float64 {
	knots := make([]float64, n+4)
	spans := float64(n - 3)
	for i := range knots {
		switch {
		case i < 4:
			knots[i] = 0
		case i >= n:
			knots[i] = 1
		default:
			knots[i] = float64(i-3) / spans
		}
	}
	return knots
}

// bsplineBasis is the Cox–de Boor recursion N_{i,p}(t) with the 0/0 = 0
// convention.
func bsplineBasis(i, p int, t float64, knots []float64) float64 {
	if p == 0 {
		if knots[i] <= t && t < knots[i+1] {
			return 1
		}
		return 0
	}
	var sum float64
	if d := knots[i+p] - knots[i]; d > 0 {
		sum += (t - knots[i]) / d * bsplineBasis(i, p-1, t, knots)
	}
	if d := knots[i+p+1] - knots[i+1]; d > 0 {
		sum += (knots[i+p+1] - t) / d * bsplineBasis(i+1, p-1, t, knots)
	}
	return sum
}
