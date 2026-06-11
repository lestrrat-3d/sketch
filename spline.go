package sketch

import (
	"fmt"

	"github.com/lestrrat-3d/sketch/geom"
)

// Spline is an open clamped cubic B-spline whose control points are ordinary
// sketch points. The solver reshapes the curve by moving control points — every
// point-based constraint, dimension and goal applies to them directly; the
// spline itself carries no extra unknowns and no internal constraints. The curve
// starts at the first control point and ends at the last (clamped), with end
// tangents along the first and last control-polygon legs.
type Spline struct {
	s            *Sketch
	Control      []*Point
	id           int
	construction bool
}

func (sp *Spline) entity()                {}
func (sp *Spline) entID() int             { return sp.id }
func (sp *Spline) IsConstruction() bool   { return sp.construction }
func (sp *Spline) SetConstruction(v bool) { sp.construction = v }

// Geometry returns a fresh [geom.Spline] snapshot over the spline's current
// control-point coordinates.
func (sp *Spline) Geometry() *geom.Spline {
	ctrl := make([]*geom.Point, len(sp.Control))
	for i, p := range sp.Control {
		ctrl[i] = p.Geometry()
	}
	return geom.NewSpline(ctrl...)
}

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

// AddSpline adds a cubic B-spline over the given control points (at least 4;
// panics otherwise) and returns its handle. Share control points with other
// geometry to relate them.
func (s *Sketch) AddSpline(control ...*Point) *Spline {
	if len(control) < 4 {
		panic(fmt.Sprintf("sketch: AddSpline requires at least 4 control points, got %d", len(control)))
	}
	sp := &Spline{s: s, Control: append([]*Point(nil), control...), id: len(s.ents)}
	s.ents = append(s.ents, sp)
	return sp
}
