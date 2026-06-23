package geom

import (
	"errors"
	"fmt"
	"math"
)

// ClosedSpline is a closed (periodic) uniform cubic B-spline: a smooth loop, C2
// across the seam, whose control points are ordinary points. Unlike the open
// [Spline] it has no endpoints — it bounds a region on its own, so it is a
// [ClosedCurve], not a [Curve].
type ClosedSpline struct {
	Control []*Point
}

// ErrTooFewClosedControlPoints is returned by [NewClosedSpline] when given fewer
// than the three control points a periodic cubic B-spline requires.
var ErrTooFewClosedControlPoints = errors.New("geom: a closed cubic B-spline requires at least 3 control points")

// NewClosedSpline returns a closed periodic cubic B-spline over the given control
// points. It returns [ErrTooFewClosedControlPoints] with fewer than 3.
func NewClosedSpline(control ...*Point) (*ClosedSpline, error) {
	if len(control) < 3 {
		return nil, fmt.Errorf("%w, got %d", ErrTooFewClosedControlPoints, len(control))
	}
	return &ClosedSpline{Control: control}, nil
}

// closedCurve marks ClosedSpline as a sealed [ClosedCurve] member.
func (sp *ClosedSpline) closedCurve() {}

// Eval returns the curve point at parameter t, reduced modulo 1 into the periodic
// domain (so Eval(0) == Eval(1)).
func (sp *ClosedSpline) Eval(t float64) (float64, float64) {
	// control-point count is guaranteed >=3 by the ClosedSpline constructor.
	x, y, _ := EvalPeriodicCubicBSpline(controlCoords(sp.Control), t)
	return x, y
}

// Polyline samples the closed spline at segments+1 evenly spaced parameters; the
// last point equals the first, closing the ring.
func (sp *ClosedSpline) Polyline(segments int) [][2]float64 {
	pts, _ := SamplePeriodicCubicBSpline(controlCoords(sp.Control), segments)
	return pts
}

// EvalPeriodicCubicBSpline evaluates a closed (periodic) uniform cubic B-spline
// over the control coordinates at parameter t. t is reduced modulo 1, so the
// curve is a smooth closed loop: over n control points each unit-length span i ∈
// [0,n) blends the four cyclic controls P[i..i+3] (indices mod n) with the
// standard uniform cubic basis. It returns [ErrTooFewClosedControlPoints] with
// fewer than 3 control points.
func EvalPeriodicCubicBSpline(ctrl [][2]float64, t float64) (float64, float64, error) {
	n := len(ctrl)
	if err := tooFewPoints(n, 3, ErrTooFewClosedControlPoints); err != nil {
		return 0, 0, err
	}
	t -= math.Floor(t) // reduce to [0,1); handles t=1 -> 0 and negative t
	u := t * float64(n)
	i := int(math.Floor(u))
	if i >= n { // floating-point guard at the seam
		i = n - 1
	}
	v := u - float64(i)
	v2, v3 := v*v, v*v*v
	b0 := (1 - 3*v + 3*v2 - v3) / 6
	b1 := (3*v3 - 6*v2 + 4) / 6
	b2 := (-3*v3 + 3*v2 + 3*v + 1) / 6
	b3 := v3 / 6
	p0, p1 := ctrl[i], ctrl[(i+1)%n]
	p2, p3 := ctrl[(i+2)%n], ctrl[(i+3)%n]
	return b0*p0[0] + b1*p1[0] + b2*p2[0] + b3*p3[0],
		b0*p0[1] + b1*p1[1] + b2*p2[1] + b3*p3[1], nil
}

// EvalPeriodicCubicBSplineDeriv returns the first derivative dS/dt of the closed
// (periodic) uniform cubic B-spline at parameter t (reduced modulo 1). Within
// span i ∈ [0,n) the curve blends the four cyclic controls P[i..i+3] with the
// uniform cubic basis in v = n·t − i; differentiating the basis in v and scaling
// by dv/dt = n gives the analytic tangent. It returns
// [ErrTooFewClosedControlPoints] with fewer than 3 control points.
func EvalPeriodicCubicBSplineDeriv(ctrl [][2]float64, t float64) (float64, float64, error) {
	n := len(ctrl)
	if err := tooFewPoints(n, 3, ErrTooFewClosedControlPoints); err != nil {
		return 0, 0, err
	}
	t -= math.Floor(t)
	u := t * float64(n)
	i := int(math.Floor(u))
	if i >= n {
		i = n - 1
	}
	v := u - float64(i)
	v2 := v * v
	// derivatives of the uniform cubic basis with respect to v
	db0 := (-3 + 6*v - 3*v2) / 6
	db1 := (9*v2 - 12*v) / 6
	db2 := (-9*v2 + 6*v + 3) / 6
	db3 := (3 * v2) / 6
	p0, p1 := ctrl[i], ctrl[(i+1)%n]
	p2, p3 := ctrl[(i+2)%n], ctrl[(i+3)%n]
	nn := float64(n) // dv/dt
	return nn * (db0*p0[0] + db1*p1[0] + db2*p2[0] + db3*p3[0]),
		nn * (db0*p0[1] + db1*p1[1] + db2*p2[1] + db3*p3[1]), nil
}

// SamplePeriodicCubicBSpline samples the closed spline at segments+1 evenly
// spaced parameters (minimum 3 segments). The last point equals the first so the
// returned polyline is a closed ring. It returns [ErrTooFewClosedControlPoints]
// with fewer than 3 control points.
func SamplePeriodicCubicBSpline(ctrl [][2]float64, segments int) ([][2]float64, error) {
	if err := tooFewPoints(len(ctrl), 3, ErrTooFewClosedControlPoints); err != nil {
		return nil, err
	}
	if segments < 3 {
		segments = 3
	}
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		// length already validated up front; the in-loop error is unreachable.
		x, y, _ := EvalPeriodicCubicBSpline(ctrl, float64(i)/float64(segments))
		pts[i] = [2]float64{x, y}
	}
	return pts, nil
}
