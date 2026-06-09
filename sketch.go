package sketch

import (
	"errors"
	"math"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
)

// Sketch is a collection of geometric primitives and the constraints relating
// them. All scalar unknowns (point coordinates, circle radii) live in a single
// flat parameter vector so the constraint solver can treat the whole sketch as
// one nonlinear system.
//
// A Sketch is not safe for concurrent use.
type Sketch struct {
	vars  []float64 // flat parameter vector (all scalar unknowns)
	fixed []bool    // parallel to vars; true == grounded / not solved for

	points []*Point
	ents   []Entity
	cons   []Constraint

	params *param.Table // optional; drives bound dimensions
	sys    units.System // default length/angle units
}

// New returns an empty sketch using metric default units (millimetres and
// degrees); change them with [Sketch.SetUnits].
func New() *Sketch { return &Sketch{sys: units.Metric()} }

func (s *Sketch) newVar(v float64) int {
	s.vars = append(s.vars, v)
	s.fixed = append(s.fixed, false)
	return len(s.vars) - 1
}

// Points returns the points in creation order. The slice must not be modified.
func (s *Sketch) Points() []*Point { return s.points }

// Entities returns the lines, circles and arcs in creation order. The slice
// must not be modified.
func (s *Sketch) Entities() []Entity { return s.ents }

// Constraints returns the constraints in creation order. The slice must not be
// modified.
func (s *Sketch) Constraints() []Constraint { return s.cons }

// --- Point ------------------------------------------------------------------

// Point is a 2D point. It is constructed detached with [NewPoint] and becomes
// part of a sketch when passed to [Sketch.AddPoint]; until then its coordinates
// are held locally. Once added, its coordinates are unknowns solved for by the
// constraint solver unless the point is grounded with [Sketch.Fix] or
// [Sketch.Lock].
type Point struct {
	s            *Sketch // nil until added to a sketch
	xi, yi       int     // indices into Sketch.vars once added
	x0, y0       float64 // coordinates while detached (and the initial guess on add)
	id           int     // index into Sketch.points
	fixed        bool    // grounding intent / state
	Name         string
	Construction bool
}

// NewPoint constructs a free point at (x, y). The point is not part of any
// sketch until passed to [Sketch.AddPoint].
func NewPoint(x, y float64) *Point { return &Point{x0: x, y0: y} }

// X returns the point's current x coordinate.
func (p *Point) X() float64 { return p.x() }

// Y returns the point's current y coordinate.
func (p *Point) Y() float64 { return p.y() }

// ID returns the stable index of the point within its sketch (valid once added).
func (p *Point) ID() int { return p.id }

func (p *Point) x() float64 {
	if p.s == nil {
		return p.x0
	}
	return p.s.vars[p.xi]
}

func (p *Point) y() float64 {
	if p.s == nil {
		return p.y0
	}
	return p.s.vars[p.yi]
}

// AddPoint commits a point to the sketch, allocating its solver variables, and
// returns it. It is idempotent: adding a point already in this sketch is a
// no-op. A point may belong to only one sketch.
func (s *Sketch) AddPoint(p *Point) *Point {
	if p.s == s {
		return p
	}
	if p.s != nil {
		panic("sketch: point already belongs to another sketch")
	}
	p.s = s
	p.xi = s.newVar(p.x0)
	p.yi = s.newVar(p.y0)
	p.id = len(s.points)
	s.points = append(s.points, p)
	if p.fixed {
		s.fixed[p.xi] = true
		s.fixed[p.yi] = true
	}
	return p
}

func (p *Point) addTo(s *Sketch) { s.AddPoint(p) }

// SetXY moves a point. This sets the solver's starting guess for the point and
// has no effect once constraints pin it down.
func (p *Point) SetXY(x, y float64) {
	p.x0, p.y0 = x, y
	if p.s != nil {
		p.s.vars[p.xi] = x
		p.s.vars[p.yi] = y
	}
}

// Fix grounds a point at its current location so the solver will not move it.
// It may be called before or after the point is added.
func (s *Sketch) Fix(p *Point) {
	p.fixed = true
	if p.s == s {
		s.fixed[p.xi] = true
		s.fixed[p.yi] = true
	}
}

// Lock moves a point to (x, y) and grounds it there.
func (s *Sketch) Lock(p *Point, x, y float64) {
	p.SetXY(x, y)
	s.Fix(p)
}

// Unfix releases a previously grounded point so the solver may move it again.
func (s *Sketch) Unfix(p *Point) {
	p.fixed = false
	if p.s == s {
		s.fixed[p.xi] = false
		s.fixed[p.yi] = false
	}
}

// IsFixed reports whether the point is grounded.
func (p *Point) IsFixed() bool {
	if p.s == nil {
		return p.fixed
	}
	return p.s.fixed[p.xi] && p.s.fixed[p.yi]
}

// --- Entities ---------------------------------------------------------------

// Entity is a line, circle or arc.
type Entity interface {
	isConstruction() bool
	entID() int
	entity()
	committable
}

// committable is implemented by geometry that can be committed to a sketch.
type committable interface {
	addTo(*Sketch)
}

// Line is a straight segment between two points. Construct it with [NewLine]
// and commit it with [Sketch.AddLine].
type Line struct {
	s            *Sketch
	A, B         *Point
	id           int
	Construction bool
}

// NewLine constructs a line between two points (which need not be added to a
// sketch yet).
func NewLine(a, b *Point) *Line { return &Line{A: a, B: b} }

func (l *Line) entity()              {}
func (l *Line) entID() int           { return l.id }
func (l *Line) isConstruction() bool { return l.Construction }

// Length returns the current distance between the line's endpoints.
func (l *Line) Length() float64 { return math.Hypot(l.B.x()-l.A.x(), l.B.y()-l.A.y()) }

// AddLine commits a line to the sketch, first committing its endpoints if
// necessary, and returns it. It is idempotent.
func (s *Sketch) AddLine(l *Line) *Line {
	if l.s == s {
		return l
	}
	if l.s != nil {
		panic("sketch: line already belongs to another sketch")
	}
	s.AddPoint(l.A)
	s.AddPoint(l.B)
	l.s = s
	l.id = len(s.ents)
	s.ents = append(s.ents, l)
	return l
}

func (l *Line) addTo(s *Sketch) { s.AddLine(l) }

// Circle is a full circle described by a center point and a radius. Construct
// it with [NewCircle] and commit it with [Sketch.AddCircle].
type Circle struct {
	s            *Sketch
	Center       *Point
	ri           int     // radius index into Sketch.vars once added
	r0           float64 // radius while detached
	id           int
	Construction bool
}

// NewCircle constructs a circle with the given center point and radius.
func NewCircle(center *Point, radius float64) *Circle {
	return &Circle{Center: center, r0: radius}
}

func (c *Circle) entity()              {}
func (c *Circle) entID() int           { return c.id }
func (c *Circle) isConstruction() bool { return c.Construction }

// R returns the circle's current radius.
func (c *Circle) R() float64 { return c.r() }

func (c *Circle) r() float64 {
	if c.s == nil {
		return c.r0
	}
	return c.s.vars[c.ri]
}

// AddCircle commits a circle to the sketch, first committing its center if
// necessary, and returns it. It is idempotent.
func (s *Sketch) AddCircle(c *Circle) *Circle {
	if c.s == s {
		return c
	}
	if c.s != nil {
		panic("sketch: circle already belongs to another sketch")
	}
	s.AddPoint(c.Center)
	c.s = s
	c.ri = s.newVar(c.r0)
	c.id = len(s.ents)
	s.ents = append(s.ents, c)
	return c
}

func (c *Circle) addTo(s *Sketch) { s.AddCircle(c) }

// Arc is a circular arc, swept counter-clockwise from Start to End about
// Center. Construct it with [NewArc] and commit it with [Sketch.AddArc]. Its
// radius is implied by the geometry; an internal constraint keeps the start and
// end equidistant from the center so the arc stays valid.
type Arc struct {
	s                  *Sketch
	Center, Start, End *Point
	id                 int
	Construction       bool
}

// NewArc constructs an arc swept counter-clockwise from start to end about
// center.
func NewArc(center, start, end *Point) *Arc {
	return &Arc{Center: center, Start: start, End: end}
}

func (a *Arc) entity()              {}
func (a *Arc) entID() int           { return a.id }
func (a *Arc) isConstruction() bool { return a.Construction }

// R returns the arc's current radius (distance from center to start).
func (a *Arc) R() float64 { return math.Hypot(a.Start.x()-a.Center.x(), a.Start.y()-a.Center.y()) }

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

// AddArc commits an arc to the sketch, first committing its points if
// necessary, and adds the internal radius-consistency constraint. It is
// idempotent.
func (s *Sketch) AddArc(a *Arc) *Arc {
	if a.s == s {
		return a
	}
	if a.s != nil {
		panic("sketch: arc already belongs to another sketch")
	}
	s.AddPoint(a.Center)
	s.AddPoint(a.Start)
	s.AddPoint(a.End)
	a.s = s
	a.id = len(s.ents)
	s.ents = append(s.ents, a)
	s.cons = append(s.cons, &arcRadius{a})
	return a
}

func (a *Arc) addTo(s *Sketch) { s.AddArc(a) }

// --- Errors -----------------------------------------------------------------

// ErrNotConverged is returned by [Sketch.Solve] when the solver fails to drive
// all constraints to within tolerance within the iteration budget.
var ErrNotConverged = errors.New("sketch: constraint solver did not converge")
