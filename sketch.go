package sketch

import (
	"errors"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
)

// Sketch instantiates generic [geom] geometry as solver-bound geometry and
// relates it with constraints. All scalar unknowns (point coordinates, circle
// radii) live in a single flat parameter vector so the constraint solver can
// treat the whole sketch as one nonlinear system.
//
// Generic geometry is committed with the Add methods, which map each distinct
// generic primitive to one solver-bound instance; the same generic geometry can
// therefore be committed into several independent sketches.
//
// A Sketch is not safe for concurrent use.
type Sketch struct {
	vars  []float64 // flat parameter vector (all scalar unknowns)
	fixed []bool    // parallel to vars; true == grounded / not solved for

	points []*Point
	ents   []Entity
	cons   []Constraint

	// generic geometry -> its solver-bound instance in this sketch
	ptOf  map[*geom.Point]*Point
	lnOf  map[*geom.Line]*Line
	cirOf map[*geom.Circle]*Circle
	arcOf map[*geom.Arc]*Arc
	elOf  map[*geom.Ellipse]*Ellipse
	splOf map[*geom.Spline]*Spline

	params *param.Table // optional; drives bound dimensions
	sys    units.System // default length/angle units
}

// New returns an empty sketch using metric default units (millimetres and
// degrees); change them with [Sketch.SetUnits].
func New() *Sketch {
	s := &Sketch{sys: units.Metric()}
	s.initMaps()
	return s
}

func (s *Sketch) initMaps() {
	s.ptOf = map[*geom.Point]*Point{}
	s.lnOf = map[*geom.Line]*Line{}
	s.cirOf = map[*geom.Circle]*Circle{}
	s.arcOf = map[*geom.Arc]*Arc{}
	s.elOf = map[*geom.Ellipse]*Ellipse{}
	s.splOf = map[*geom.Spline]*Spline{}
}

func (s *Sketch) newVar(v float64) int {
	s.vars = append(s.vars, v)
	s.fixed = append(s.fixed, false)
	return len(s.vars) - 1
}

// Points returns the points in creation order. The slice must not be modified.
func (s *Sketch) Points() []*Point { return s.points }

// Entities returns the lines, circles, arcs and ellipses in creation order.
// The slice must not be modified.
func (s *Sketch) Entities() []Entity { return s.ents }

// Constraints returns the constraints in creation order. The slice must not be
// modified.
func (s *Sketch) Constraints() []Constraint { return s.cons }

// --- Point ------------------------------------------------------------------

// Point is the solver-bound instance of a [geom.Point] within a sketch. Its
// coordinates are unknowns solved for by the constraint solver unless the point
// is grounded with [Sketch.Fix]. Obtain one by committing
// generic geometry with [Sketch.AddPoint] (directly or via a line/circle/arc).
type Point struct {
	g      *geom.Point
	s      *Sketch
	xi, yi int // indices into Sketch.vars
	id     int // index into Sketch.points
}

// X returns the point's current (solved) x coordinate.
func (p *Point) X() float64 { return p.s.vars[p.xi] }

// Y returns the point's current (solved) y coordinate.
func (p *Point) Y() float64 { return p.s.vars[p.yi] }

// ID returns the stable index of the point within its sketch.
func (p *Point) ID() int { return p.id }

// Generic returns the context-agnostic geometry this point was committed from.
func (p *Point) Generic() *geom.Point { return p.g }

func (p *Point) x() float64 { return p.s.vars[p.xi] }
func (p *Point) y() float64 { return p.s.vars[p.yi] }

// AddPoint commits a generic point to the sketch, allocating its solver
// variables initialised from the generic coordinates, and returns its
// solver-bound instance. It is idempotent: a generic point already committed to
// this sketch maps to the same [Point].
func (s *Sketch) AddPoint(g *geom.Point) *Point {
	if p, ok := s.ptOf[g]; ok {
		return p
	}
	p := &Point{g: g, s: s, xi: s.newVar(g.X), yi: s.newVar(g.Y), id: len(s.points)}
	s.points = append(s.points, p)
	s.ptOf[g] = p
	return p
}

// MoveTo moves a point to (x, y). This sets the solver's starting guess for the
// point and has no effect once constraints pin it down. It does not change the
// generic geometry the point came from.
func (p *Point) MoveTo(x, y float64) {
	p.s.vars[p.xi] = x
	p.s.vars[p.yi] = y
}

// Fix grounds a point at its current location so the solver will not move it.
// Grounding is per-sketch: the same generic point may be fixed in one sketch
// and free in another. To ground a point at a specific location, move it first:
// p.MoveTo(x, y) then s.Fix(p).
func (s *Sketch) Fix(p *Point) {
	s.fixed[p.xi] = true
	s.fixed[p.yi] = true
}

// Unfix releases a previously grounded point so the solver may move it again.
func (s *Sketch) Unfix(p *Point) {
	s.fixed[p.xi] = false
	s.fixed[p.yi] = false
}

// IsFixed reports whether the point is grounded.
func (p *Point) IsFixed() bool { return p.s.fixed[p.xi] && p.s.fixed[p.yi] }

// --- Entities ---------------------------------------------------------------

// Entity is a line, circle, arc or ellipse bound to a sketch.
type Entity interface {
	isConstruction() bool
	entID() int
	entity()
}

// Circular is a sketch entity with a center point and a radius: a [*Circle] or
// an [*Arc]. Constraints that relate centers and radii — [NewTangent],
// [NewTangentCircles], [NewEqualRadius] — accept either.
type Circular interface {
	Entity
	R() float64
	centerPt() *Point
}

// Line is the solver-bound instance of a [geom.Line].
type Line struct {
	g          *geom.Line
	s          *Sketch
	Start, End *Point
	id         int
}

func (l *Line) entity()              {}
func (l *Line) entID() int           { return l.id }
func (l *Line) isConstruction() bool { return l.g.Construction }

// Generic returns the context-agnostic geometry this line was committed from.
func (l *Line) Generic() *geom.Line { return l.g }

// Length returns the current distance between the line's endpoints.
func (l *Line) Length() float64 { return math.Hypot(l.End.x()-l.Start.x(), l.End.y()-l.Start.y()) }

// AddLine commits a generic line to the sketch, first committing its endpoints,
// and returns its solver-bound instance. It is idempotent.
func (s *Sketch) AddLine(g *geom.Line) *Line {
	if l, ok := s.lnOf[g]; ok {
		return l
	}
	l := &Line{g: g, s: s, Start: s.AddPoint(g.Start), End: s.AddPoint(g.End), id: len(s.ents)}
	s.ents = append(s.ents, l)
	s.lnOf[g] = l
	return l
}

// Circle is the solver-bound instance of a [geom.Circle].
type Circle struct {
	g      *geom.Circle
	s      *Sketch
	Center *Point
	ri     int // radius index into Sketch.vars
	id     int
}

func (c *Circle) entity()              {}
func (c *Circle) entID() int           { return c.id }
func (c *Circle) isConstruction() bool { return c.g.Construction }

// Generic returns the context-agnostic geometry this circle was committed from.
func (c *Circle) Generic() *geom.Circle { return c.g }

// R returns the circle's current radius.
func (c *Circle) R() float64 { return c.s.vars[c.ri] }

func (c *Circle) r() float64 { return c.s.vars[c.ri] }

func (c *Circle) centerPt() *Point { return c.Center }

// AddCircle commits a generic circle to the sketch, first committing its
// center, and returns its solver-bound instance. It is idempotent.
func (s *Sketch) AddCircle(g *geom.Circle) *Circle {
	if c, ok := s.cirOf[g]; ok {
		return c
	}
	c := &Circle{g: g, s: s, Center: s.AddPoint(g.Center), ri: s.newVar(g.Radius), id: len(s.ents)}
	s.ents = append(s.ents, c)
	s.cirOf[g] = c
	return c
}

// Arc is the solver-bound instance of a [geom.Arc]. Its radius is implied by
// the geometry; an internal constraint keeps the start and end equidistant from
// the center so the arc stays valid.
type Arc struct {
	g                  *geom.Arc
	s                  *Sketch
	Center, Start, End *Point
	id                 int
}

func (a *Arc) entity()              {}
func (a *Arc) entID() int           { return a.id }
func (a *Arc) isConstruction() bool { return a.g.Construction }

// Generic returns the context-agnostic geometry this arc was committed from.
func (a *Arc) Generic() *geom.Arc { return a.g }

// R returns the arc's current radius (distance from center to start).
func (a *Arc) R() float64 { return math.Hypot(a.Start.x()-a.Center.x(), a.Start.y()-a.Center.y()) }

func (a *Arc) centerPt() *Point { return a.Center }

// StartAngle returns the angle (radians) of the start point about the center.
func (a *Arc) StartAngle() float64 {
	return math.Atan2(a.Start.y()-a.Center.y(), a.Start.x()-a.Center.x())
}

// EndAngle returns the angle (radians) of the end point about the center.
func (a *Arc) EndAngle() float64 {
	return math.Atan2(a.End.y()-a.Center.y(), a.End.x()-a.Center.x())
}

// Sweep returns the counter-clockwise sweep angle of the arc in (0, 2π].
func (a *Arc) Sweep() float64 {
	d := math.Mod(a.EndAngle()-a.StartAngle(), 2*math.Pi)
	if d <= 0 {
		d += 2 * math.Pi
	}
	return d
}

// AddArc commits a generic arc to the sketch, first committing its points, and
// adds the internal radius-consistency constraint. It is idempotent.
func (s *Sketch) AddArc(g *geom.Arc) *Arc {
	if a, ok := s.arcOf[g]; ok {
		return a
	}
	a := &Arc{g: g, s: s, Center: s.AddPoint(g.Center), Start: s.AddPoint(g.Start), End: s.AddPoint(g.End), id: len(s.ents)}
	s.ents = append(s.ents, a)
	s.cons = append(s.cons, &arcRadius{a})
	s.arcOf[g] = a
	return a
}

// Ellipse is the solver-bound instance of a [geom.Ellipse]. Its semi-axes and
// rotation are solver unknowns; pin them with [NewSemiMajor], [NewSemiMinor]
// and [NewEllipseRotation] dimensions (the center is a regular point, grounded
// with [Sketch.Fix]).
type Ellipse struct {
	g              *geom.Ellipse
	s              *Sketch
	Center         *Point
	rxi, ryi, roti int // var indices: semi-axes and rotation
	id             int
}

func (e *Ellipse) entity()              {}
func (e *Ellipse) entID() int           { return e.id }
func (e *Ellipse) isConstruction() bool { return e.g.Construction }

// Generic returns the context-agnostic geometry this ellipse was committed
// from.
func (e *Ellipse) Generic() *geom.Ellipse { return e.g }

// Rx returns the current semi-axis along the ellipse's local x axis.
func (e *Ellipse) Rx() float64 { return e.s.vars[e.rxi] }

// Ry returns the current semi-axis along the ellipse's local y axis.
func (e *Ellipse) Ry() float64 { return e.s.vars[e.ryi] }

// Rotation returns the current rotation of the ellipse's local frame, in
// radians counter-clockwise.
func (e *Ellipse) Rotation() float64 { return e.s.vars[e.roti] }

func (e *Ellipse) rx() float64  { return e.s.vars[e.rxi] }
func (e *Ellipse) ry() float64  { return e.s.vars[e.ryi] }
func (e *Ellipse) rot() float64 { return e.s.vars[e.roti] }

// AddEllipse commits a generic ellipse to the sketch, first committing its
// center, and returns its solver-bound instance. It is idempotent.
func (s *Sketch) AddEllipse(g *geom.Ellipse) *Ellipse {
	if e, ok := s.elOf[g]; ok {
		return e
	}
	e := &Ellipse{
		g: g, s: s, Center: s.AddPoint(g.Center),
		rxi: s.newVar(g.Rx), ryi: s.newVar(g.Ry), roti: s.newVar(g.Rotation),
		id: len(s.ents),
	}
	s.ents = append(s.ents, e)
	s.elOf[g] = e
	return e
}

// --- Errors -----------------------------------------------------------------

// ErrNotConverged is returned by [Sketch.Solve] when the solver fails to drive
// all constraints to within tolerance within the iteration budget.
var ErrNotConverged = errors.New("sketch: constraint solver did not converge")
