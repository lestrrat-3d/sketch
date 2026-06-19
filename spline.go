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
	refState     // reference splines are a follow-up; stale derived from control points
}

func (sp *Spline) entity()              {}
func (sp *Spline) entID() int           { return sp.id }
func (sp *Spline) IsConstruction() bool { return sp.construction }
func (sp *Spline) SetConstruction(v bool) {
	if !sp.reference {
		sp.construction = v
	}
}

// IsStale reports whether any control point is stale (derived; reference splines
// are not yet authorable).
func (sp *Spline) IsStale() bool {
	for _, p := range sp.Control {
		if p.IsStale() {
			return true
		}
	}
	return false
}

// Geometry returns a fresh [geom.Spline] snapshot over the spline's current
// control-point coordinates.
func (sp *Spline) Geometry() *geom.Spline {
	ctrl := make([]*geom.Point, len(sp.Control))
	for i, p := range sp.Control {
		ctrl[i] = p.Geometry()
	}
	// The control points are already validated (AddSpline requires >= 4), so
	// build the snapshot directly rather than re-validating through NewSpline.
	return &geom.Spline{Control: ctrl}
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

// AddSpline adds a cubic B-spline over the given control points and returns its
// handle. Share control points with other geometry to relate them. It returns
// [ErrInvalidShape] with fewer than 4 control points.
func (s *Sketch) AddSpline(control ...*Point) (*Spline, error) {
	if len(control) < 4 {
		return nil, fmt.Errorf("%w: AddSpline requires at least 4 control points, got %d", ErrInvalidShape, len(control))
	}
	for i, p := range control {
		if p == nil {
			return nil, fmt.Errorf("%w: AddSpline control point %d is nil", ErrInvalidShape, i)
		}
	}
	sp := &Spline{s: s, Control: append([]*Point(nil), control...), id: len(s.ents)}
	s.ents = append(s.ents, sp)
	return sp, nil
}

// ClosedSpline is a closed (periodic) uniform cubic B-spline whose control points
// are ordinary sketch points. It is a smooth closed loop (C2 across the seam) and
// bounds a region on its own, so — unlike the open [Spline] — it has no
// endpoints. The solver reshapes it by moving control points; it carries no extra
// unknowns and no internal constraints.
type ClosedSpline struct {
	s            *Sketch
	Control      []*Point
	id           int
	construction bool
	refState     // reference closed splines are a follow-up; stale derived from control points
}

func (sp *ClosedSpline) entity()              {}
func (sp *ClosedSpline) entID() int           { return sp.id }
func (sp *ClosedSpline) IsConstruction() bool { return sp.construction }
func (sp *ClosedSpline) SetConstruction(v bool) {
	if !sp.reference {
		sp.construction = v
	}
}

// IsStale reports whether any control point is stale (derived; reference closed
// splines are not yet authorable).
func (sp *ClosedSpline) IsStale() bool {
	for _, p := range sp.Control {
		if p.IsStale() {
			return true
		}
	}
	return false
}

// Geometry returns a fresh [geom.ClosedSpline] snapshot over the closed spline's
// current control-point coordinates.
func (sp *ClosedSpline) Geometry() *geom.ClosedSpline {
	ctrl := make([]*geom.Point, len(sp.Control))
	for i, p := range sp.Control {
		ctrl[i] = p.Geometry()
	}
	return &geom.ClosedSpline{Control: ctrl}
}

// Eval returns the curve point at parameter t (reduced modulo 1 into the periodic
// domain), using the solved control-point coordinates.
func (sp *ClosedSpline) Eval(t float64) (float64, float64) {
	return geom.EvalPeriodicCubicBSpline(sp.controlCoords(), t)
}

// Polyline samples the solved closed curve at segments+1 evenly spaced
// parameters; the last point equals the first, closing the ring.
func (sp *ClosedSpline) Polyline(segments int) [][2]float64 {
	return geom.SamplePeriodicCubicBSpline(sp.controlCoords(), segments)
}

func (sp *ClosedSpline) controlCoords() [][2]float64 {
	pts := make([][2]float64, len(sp.Control))
	for i, p := range sp.Control {
		pts[i] = [2]float64{p.x(), p.y()}
	}
	return pts
}

// AddClosedSpline adds a closed (periodic) cubic B-spline over the given control
// points and returns its handle. Share control points with other geometry to
// relate them. It returns [ErrInvalidShape] with fewer than 3 control points or a
// nil control point.
func (s *Sketch) AddClosedSpline(control ...*Point) (*ClosedSpline, error) {
	if len(control) < 3 {
		return nil, fmt.Errorf("%w: AddClosedSpline requires at least 3 control points, got %d", ErrInvalidShape, len(control))
	}
	for i, p := range control {
		if p == nil {
			return nil, fmt.Errorf("%w: AddClosedSpline control point %d is nil", ErrInvalidShape, i)
		}
	}
	sp := &ClosedSpline{s: s, Control: append([]*Point(nil), control...), id: len(s.ents)}
	s.ents = append(s.ents, sp)
	return sp, nil
}

// FitSpline is an open spline that INTERPOLATES its fit points: the curve passes
// through every fit point. The fit points are ordinary sketch points and the
// durable handles — the interpolating natural cubic is recomputed from their
// current coordinates on every evaluation, so the curve keeps passing through them
// as the solver moves them. It carries no extra unknowns and no internal
// constraints.
type FitSpline struct {
	s            *Sketch
	Fit          []*Point
	id           int
	construction bool
	refState     // reference fit splines are a follow-up; stale derived from fit points
}

func (sp *FitSpline) entity()              {}
func (sp *FitSpline) entID() int           { return sp.id }
func (sp *FitSpline) IsConstruction() bool { return sp.construction }
func (sp *FitSpline) SetConstruction(v bool) {
	if !sp.reference {
		sp.construction = v
	}
}

// IsStale reports whether any fit point is stale (derived; reference fit splines
// are not yet authorable).
func (sp *FitSpline) IsStale() bool {
	for _, p := range sp.Fit {
		if p.IsStale() {
			return true
		}
	}
	return false
}

// Geometry returns a fresh [geom.FitSpline] snapshot over the fit spline's current
// fit-point coordinates.
func (sp *FitSpline) Geometry() *geom.FitSpline {
	fit := make([]*geom.Point, len(sp.Fit))
	for i, p := range sp.Fit {
		fit[i] = p.Geometry()
	}
	return &geom.FitSpline{Fit: fit}
}

// Eval returns the interpolated curve point at parameter t ∈ [0, 1] (normalized
// chord length), using the solved fit-point coordinates.
func (sp *FitSpline) Eval(t float64) (float64, float64) {
	return geom.EvalFitSpline(sp.fitCoords(), t)
}

// Polyline samples the solved interpolating curve at segments+1 evenly spaced
// (in chord length) parameters.
func (sp *FitSpline) Polyline(segments int) [][2]float64 {
	return geom.SampleFitSpline(sp.fitCoords(), segments)
}

func (sp *FitSpline) fitCoords() [][2]float64 {
	pts := make([][2]float64, len(sp.Fit))
	for i, p := range sp.Fit {
		pts[i] = [2]float64{p.x(), p.y()}
	}
	return pts
}

// AddFitSpline adds a fit-point (interpolating) cubic spline through the given fit
// points and returns its handle. The curve passes through every fit point; share
// fit points with other geometry to relate them. It returns [ErrInvalidShape] with
// fewer than 2 fit points or a nil fit point.
func (s *Sketch) AddFitSpline(fit ...*Point) (*FitSpline, error) {
	if len(fit) < 2 {
		return nil, fmt.Errorf("%w: AddFitSpline requires at least 2 fit points, got %d", ErrInvalidShape, len(fit))
	}
	for i, p := range fit {
		if p == nil {
			return nil, fmt.Errorf("%w: AddFitSpline fit point %d is nil", ErrInvalidShape, i)
		}
	}
	sp := &FitSpline{s: s, Fit: append([]*Point(nil), fit...), id: len(s.ents)}
	s.ents = append(s.ents, sp)
	return sp, nil
}
