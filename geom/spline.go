package geom

import "fmt"

// Spline is an open cubic B-spline defined by its control points, using a
// clamped uniform knot vector. Clamping makes the curve start at the first
// control point and end at the last, with end tangents along the first and
// last control-polygon legs. Degree is fixed at 3.
type Spline struct {
	Control []*Point
}

// NewSpline returns a cubic B-spline over the given control points. It panics
// with fewer than 4 control points.
func NewSpline(control ...*Point) *Spline {
	if len(control) < 4 {
		panic(fmt.Sprintf("geom: NewSpline requires at least 4 control points, got %d", len(control)))
	}
	return &Spline{Control: control}
}

// Eval returns the curve point at parameter t ∈ [0, 1] (clamped).
func (sp *Spline) Eval(t float64) (float64, float64) {
	return EvalCubicBSpline(controlCoords(sp.Control), t)
}

// Polyline samples the spline at segments+1 evenly spaced parameters.
func (sp *Spline) Polyline(segments int) [][2]float64 {
	return SampleCubicBSpline(controlCoords(sp.Control), segments)
}

func controlCoords(control []*Point) [][2]float64 {
	pts := make([][2]float64, len(control))
	for i, p := range control {
		pts[i] = [2]float64{p.X, p.Y}
	}
	return pts
}

// EvalCubicBSpline evaluates a clamped uniform cubic B-spline over the given
// control coordinates at t ∈ [0, 1] (values outside are clamped). At t = 1
// the last control point is returned directly: the standard half-open
// Cox–de Boor basis is zero at the trailing multiplicity-4 knot, and the
// shortcut is exact for a clamped curve. It panics with fewer than 4 control
// points.
func EvalCubicBSpline(ctrl [][2]float64, t float64) (float64, float64) {
	n := len(ctrl)
	if n < 4 {
		panic(fmt.Sprintf("geom: cubic B-spline needs at least 4 control points, got %d", n))
	}
	if t <= 0 {
		return ctrl[0][0], ctrl[0][1]
	}
	if t >= 1 {
		return ctrl[n-1][0], ctrl[n-1][1]
	}
	knots := ClampedKnots(n)
	var x, y float64
	for i := 0; i < n; i++ {
		b := bsplineBasis(i, 3, t, knots)
		x += b * ctrl[i][0]
		y += b * ctrl[i][1]
	}
	return x, y
}

// SampleCubicBSpline samples the spline at segments+1 evenly spaced
// parameters (minimum 2 segments).
func SampleCubicBSpline(ctrl [][2]float64, segments int) [][2]float64 {
	if segments < 2 {
		segments = 2
	}
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		x, y := EvalCubicBSpline(ctrl, float64(i)/float64(segments))
		pts[i] = [2]float64{x, y}
	}
	return pts
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
