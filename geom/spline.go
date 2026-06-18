package geom

import (
	"errors"
	"fmt"
	"math"
)

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

// NearestParamCubicBSpline returns the parameter t ∈ [0, 1] whose curve point is
// closest to (px, py). It is a robust seed for a foot-point aux variable, not an
// exact projection: a dense polyline broad phase (each segment projected onto,
// not just its samples) locates the best span, then a few golden-section steps
// refine within it. Density scales with the control count so narrow loops are
// not missed. It panics with fewer than 4 control points.
func NearestParamCubicBSpline(ctrl [][2]float64, px, py float64) float64 {
	n := len(ctrl)
	if n < 4 {
		panic(fmt.Sprintf("geom: cubic B-spline needs at least 4 control points, got %d", n))
	}
	segs := 16 * (n - 3)
	if segs < 64 {
		segs = 64
	}
	bestT, bestD2 := 0.0, math.Inf(1)
	px0, py0 := EvalCubicBSpline(ctrl, 0)
	for i := 1; i <= segs; i++ {
		t1 := float64(i) / float64(segs)
		px1, py1 := EvalCubicBSpline(ctrl, t1)
		// Project (px,py) onto the chord [(px0,py0),(px1,py1)], clamped to it,
		// and map the chord parameter back to a curve parameter in this span.
		dx, dy := px1-px0, py1-py0
		seg2 := dx*dx + dy*dy
		u := 0.0
		if seg2 > 0 {
			u = ((px-px0)*dx + (py-py0)*dy) / seg2
			if u < 0 {
				u = 0
			} else if u > 1 {
				u = 1
			}
		}
		t := (float64(i-1) + u) / float64(segs)
		cx, cy := EvalCubicBSpline(ctrl, t)
		if d2 := (px-cx)*(px-cx) + (py-cy)*(py-cy); d2 < bestD2 {
			bestD2, bestT = d2, t
		}
		px0, py0 = px1, py1
	}
	// Golden-section refine within ±1 span of the best parameter.
	span := 1.0 / float64(segs)
	lo, hi := bestT-span, bestT+span
	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	const invphi = 0.6180339887498949
	dist2 := func(t float64) float64 {
		cx, cy := EvalCubicBSpline(ctrl, t)
		return (px-cx)*(px-cx) + (py-cy)*(py-cy)
	}
	c, d := hi-invphi*(hi-lo), lo+invphi*(hi-lo)
	fc, fd := dist2(c), dist2(d)
	for k := 0; k < 24; k++ {
		if fc < fd {
			hi, d, fd = d, c, fc
			c = hi - invphi*(hi-lo)
			fc = dist2(c)
		} else {
			lo, c, fc = c, d, fd
			d = lo + invphi*(hi-lo)
			fd = dist2(d)
		}
	}
	return (lo + hi) / 2
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
