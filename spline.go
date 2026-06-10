package sketch

import "github.com/lestrrat-3d/sketch/geom"

// Spline is the solver-bound instance of a [geom.Spline]: an open clamped
// cubic B-spline whose control points are ordinary sketch points. The solver
// reshapes the curve by moving control points — every point-based constraint,
// dimension and goal applies to them directly; the spline itself carries no
// extra unknowns and no internal constraints. The curve starts at the first
// control point and ends at the last (clamped), with end tangents along the
// first and last control-polygon legs.
type Spline struct {
	g       *geom.Spline
	s       *Sketch
	Control []*Point
	id      int
}

func (sp *Spline) entity()              {}
func (sp *Spline) entID() int           { return sp.id }
func (sp *Spline) isConstruction() bool { return sp.g.Construction }

// Generic returns the context-agnostic geometry this spline was committed
// from.
func (sp *Spline) Generic() *geom.Spline { return sp.g }

// Eval returns the curve point at parameter t ∈ [0, 1] (clamped), using the
// solved control-point coordinates.
func (sp *Spline) Eval(t float64) (float64, float64) {
	return geom.EvalCubicBSpline(sp.controlCoords(), t)
}

// Polyline samples the solved curve at segments+1 evenly spaced parameters.
func (sp *Spline) Polyline(segments int) [][2]float64 {
	return geom.SampleCubicBSpline(sp.controlCoords(), segments)
}

func (sp *Spline) controlCoords() [][2]float64 {
	pts := make([][2]float64, len(sp.Control))
	for i, p := range sp.Control {
		pts[i] = [2]float64{p.x(), p.y()}
	}
	return pts
}

// AddSpline commits a generic spline to the sketch, first committing its
// control points, and returns its solver-bound instance. It is idempotent.
func (s *Sketch) AddSpline(g *geom.Spline) *Spline {
	if sp, ok := s.splOf[g]; ok {
		return sp
	}
	sp := &Spline{g: g, s: s, id: len(s.ents)}
	for _, c := range g.Control {
		sp.Control = append(sp.Control, s.AddPoint(c))
	}
	s.ents = append(s.ents, sp)
	s.splOf[g] = sp
	return sp
}
