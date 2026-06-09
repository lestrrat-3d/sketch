package sketch

import (
	"errors"
	"math"

	"github.com/lestrrat-3d/sketch/param"
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
}

// New returns an empty sketch.
func New() *Sketch { return &Sketch{} }

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

// Point is a 2D point. Its coordinates are unknowns solved for by the
// constraint solver unless the point is grounded with [Sketch.Fix] or
// [Sketch.Lock].
type Point struct {
	s            *Sketch
	xi, yi       int // indices into Sketch.vars
	id           int // index into Sketch.points
	Name         string
	Construction bool
}

// X returns the point's current x coordinate.
func (p *Point) X() float64 { return p.s.vars[p.xi] }

// Y returns the point's current y coordinate.
func (p *Point) Y() float64 { return p.s.vars[p.yi] }

// ID returns the stable index of the point within its sketch.
func (p *Point) ID() int { return p.id }

func (p *Point) x() float64 { return p.s.vars[p.xi] }
func (p *Point) y() float64 { return p.s.vars[p.yi] }

// AddPoint adds a free point at (x, y) and returns it. The coordinates are
// used as the solver's initial guess.
func (s *Sketch) AddPoint(x, y float64) *Point {
	p := &Point{s: s, xi: s.newVar(x), yi: s.newVar(y), id: len(s.points)}
	s.points = append(s.points, p)
	return p
}

// SetXY moves a point. This sets the solver's starting guess for the point and
// has no effect once constraints pin it down.
func (p *Point) SetXY(x, y float64) {
	p.s.vars[p.xi] = x
	p.s.vars[p.yi] = y
}

// Fix grounds a point at its current location so the solver will not move it.
func (s *Sketch) Fix(p *Point) {
	s.fixed[p.xi] = true
	s.fixed[p.yi] = true
}

// Lock moves a point to (x, y) and grounds it there.
func (s *Sketch) Lock(p *Point, x, y float64) {
	p.SetXY(x, y)
	s.Fix(p)
}

// Unfix releases a previously grounded point so the solver may move it again.
func (s *Sketch) Unfix(p *Point) {
	s.fixed[p.xi] = false
	s.fixed[p.yi] = false
}

// IsFixed reports whether the point is grounded.
func (p *Point) IsFixed() bool { return p.s.fixed[p.xi] && p.s.fixed[p.yi] }

// --- Entities ---------------------------------------------------------------

// Entity is a line, circle or arc.
type Entity interface {
	isConstruction() bool
	entID() int
	entity()
}

// Line is a straight segment between two points.
type Line struct {
	s            *Sketch
	A, B         *Point
	id           int
	Construction bool
}

func (l *Line) entity()              {}
func (l *Line) entID() int           { return l.id }
func (l *Line) isConstruction() bool { return l.Construction }

// Length returns the current distance between the line's endpoints.
func (l *Line) Length() float64 { return math.Hypot(l.B.x()-l.A.x(), l.B.y()-l.A.y()) }

// AddLine adds a line between two existing points.
func (s *Sketch) AddLine(a, b *Point) *Line {
	l := &Line{s: s, A: a, B: b, id: len(s.ents)}
	s.ents = append(s.ents, l)
	return l
}

// Circle is a full circle described by a center point and a radius unknown.
type Circle struct {
	s            *Sketch
	Center       *Point
	ri           int // radius index into Sketch.vars
	id           int
	Construction bool
}

func (c *Circle) entity()              {}
func (c *Circle) entID() int           { return c.id }
func (c *Circle) isConstruction() bool { return c.Construction }

// R returns the circle's current radius.
func (c *Circle) R() float64 { return c.s.vars[c.ri] }

func (c *Circle) r() float64 { return c.s.vars[c.ri] }

// AddCircle adds a circle with the given center point and initial radius.
func (s *Sketch) AddCircle(center *Point, radius float64) *Circle {
	c := &Circle{s: s, Center: center, ri: s.newVar(radius), id: len(s.ents)}
	s.ents = append(s.ents, c)
	return c
}

// Arc is a circular arc, swept counter-clockwise from Start to End about
// Center. Its radius is implied by the geometry; an internal constraint keeps
// the start and end equidistant from the center so the arc stays valid.
type Arc struct {
	s                  *Sketch
	Center, Start, End *Point
	id                 int
	Construction       bool
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
	d := a.EndAngle() - a.StartAngle()
	for d <= 0 {
		d += 2 * math.Pi
	}
	for d > 2*math.Pi {
		d -= 2 * math.Pi
	}
	return d
}

// AddArc adds an arc swept counter-clockwise from start to end about center.
// An internal consistency constraint keeps |center−start| == |center−end|.
func (s *Sketch) AddArc(center, start, end *Point) *Arc {
	a := &Arc{s: s, Center: center, Start: start, End: end, id: len(s.ents)}
	s.ents = append(s.ents, a)
	s.cons = append(s.cons, &arcRadius{a})
	return a
}

// --- Errors -----------------------------------------------------------------

// ErrNotConverged is returned by [Sketch.Solve] when the solver fails to drive
// all constraints to within tolerance within the iteration budget.
var ErrNotConverged = errors.New("sketch: constraint solver did not converge")
