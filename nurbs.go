package sketch

import (
	"fmt"

	"github.com/lestrrat-3d/sketch/geom"
)

// NURBS is an open clamped non-uniform rational B-spline of arbitrary degree
// whose control points are ordinary sketch points. The solver reshapes the curve
// by moving control points — every point-based constraint, dimension and goal
// applies to them directly. The degree, knot vector and per-control weights are
// STORED structural data, not solver unknowns (a free NURBS has DOF 2·(n+1),
// exactly like a [Spline]); promoting weights to dimensionable variables is a
// follow-up. The curve starts at the first control point and ends at the last
// (clamped).
type NURBS struct {
	s            *Sketch
	Control      []*Point
	degree       int
	knots        []float64
	weights      []float64
	id           int
	construction bool
	refState     // reference NURBS are a follow-up; stale derived from control points
}

func (c *NURBS) entity()              {}
func (c *NURBS) entID() int           { return c.id }
func (c *NURBS) IsConstruction() bool { return c.construction }
func (c *NURBS) SetConstruction(v bool) {
	if !c.reference {
		c.construction = v
	}
}

// IsStale reports whether any control point is stale (derived; reference NURBS
// are not yet authorable).
func (c *NURBS) IsStale() bool {
	for _, p := range c.Control {
		if p.IsStale() {
			return true
		}
	}
	return false
}

// Degree returns the curve's polynomial degree.
func (c *NURBS) Degree() int { return c.degree }

// Knots returns a copy of the curve's knot vector.
func (c *NURBS) Knots() []float64 { return append([]float64(nil), c.knots...) }

// Weights returns a copy of the curve's per-control weights.
func (c *NURBS) Weights() []float64 { return append([]float64(nil), c.weights...) }

// Rational reports whether any weight differs from 1 (a true rational curve).
func (c *NURBS) Rational() bool {
	for _, w := range c.weights {
		if w != 1 {
			return true
		}
	}
	return false
}

// Geometry returns a fresh [geom.NURBS] snapshot over the curve's current
// control-point coordinates and stored degree/knots/weights.
func (c *NURBS) Geometry() *geom.NURBS {
	ctrl := make([]*geom.Point, len(c.Control))
	for i, p := range c.Control {
		ctrl[i] = p.Geometry()
	}
	return &geom.NURBS{
		Degree:  c.degree,
		Control: ctrl,
		Knots:   append([]float64(nil), c.knots...),
		Weights: append([]float64(nil), c.weights...),
	}
}

// Eval returns the curve point at knot parameter u (clamped to the domain),
// using the solved control-point coordinates.
func (c *NURBS) Eval(u float64) (float64, float64) { return c.Geometry().Eval(u) }

// Polyline samples the solved curve at segments+1 evenly spaced knot parameters.
func (c *NURBS) Polyline(segments int) [][2]float64 { return c.Geometry().Polyline(segments) }

// ClampedUniformKnots returns the common clamped knot vector for n control points
// and the given degree — degree+1 copies of 0, evenly spaced interior knots, then
// degree+1 copies of 1 (length n+degree+1) — so callers of [Sketch.CreateNURBS]
// rarely hand-write one. It returns nil for an invalid (degree < 1 or n <
// degree+1) request.
func ClampedUniformKnots(n, degree int) []float64 { return geom.ClampedUniformKnots(n, degree) }

// CreateNURBS adds a clamped non-uniform rational B-spline of the given degree over
// the control points, with optional per-control weights and an explicit knot
// vector, and returns its handle. Share control points with other geometry to
// relate them.
//
// It validates: degree >= 1; at least degree+1 control points, none nil;
// len(knots) == len(control)+degree+1, non-decreasing and clamped (the first and
// last degree+1 knots each equal); and, when weights is non-nil, len(weights) ==
// len(control) with every weight > 0 (weights == nil means all 1, a non-rational
// curve). Use [ClampedUniformKnots] for the common knot vector. Any violation
// returns [ErrInvalidShape].
func (s *Sketch) CreateNURBS(degree int, control []*Point, weights, knots []float64) (*NURBS, error) {
	if degree < 1 {
		return nil, fmt.Errorf("%w: CreateNURBS degree must be >= 1, got %d", ErrInvalidShape, degree)
	}
	n := len(control)
	if n < degree+1 {
		return nil, fmt.Errorf("%w: CreateNURBS requires at least degree+1 = %d control points, got %d", ErrInvalidShape, degree+1, n)
	}
	for i, p := range control {
		if p == nil {
			return nil, fmt.Errorf("%w: CreateNURBS control point %d is nil", ErrInvalidShape, i)
		}
	}
	if len(knots) != n+degree+1 {
		return nil, fmt.Errorf("%w: CreateNURBS needs %d knots (control+degree+1), got %d", ErrInvalidShape, n+degree+1, len(knots))
	}
	for i := 1; i < len(knots); i++ {
		if knots[i] < knots[i-1] {
			return nil, fmt.Errorf("%w: CreateNURBS knot vector must be non-decreasing (knot %d < knot %d)", ErrInvalidShape, i, i-1)
		}
	}
	// Clamped: the first and last degree+1 knots are each repeated (equal).
	for i := 1; i <= degree; i++ {
		if knots[i] != knots[0] {
			return nil, fmt.Errorf("%w: CreateNURBS knot vector is not clamped at the start", ErrInvalidShape)
		}
		if knots[len(knots)-1-i] != knots[len(knots)-1] {
			return nil, fmt.Errorf("%w: CreateNURBS knot vector is not clamped at the end", ErrInvalidShape)
		}
	}
	if knots[degree] >= knots[n] {
		return nil, fmt.Errorf("%w: CreateNURBS knot domain is empty", ErrInvalidShape)
	}
	var w []float64
	if weights != nil {
		if len(weights) != n {
			return nil, fmt.Errorf("%w: CreateNURBS needs %d weights (one per control point), got %d", ErrInvalidShape, n, len(weights))
		}
		for i, wi := range weights {
			if !(wi > 0) {
				return nil, fmt.Errorf("%w: CreateNURBS weight %d must be > 0, got %v", ErrInvalidShape, i, wi)
			}
		}
		w = append([]float64(nil), weights...)
	} else {
		w = make([]float64, n)
		for i := range w {
			w[i] = 1
		}
	}
	c := &NURBS{
		s:       s,
		Control: append([]*Point(nil), control...),
		degree:  degree,
		knots:   append([]float64(nil), knots...),
		weights: w,
		id:      len(s.ents),
	}
	s.ents = append(s.ents, c)
	return c, nil
}
