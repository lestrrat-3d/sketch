package sketch

import (
	"errors"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/space"
	"github.com/lestrrat-3d/sketch/units"
)

// Sketch holds solver-bound geometry and the constraints that relate it. All
// scalar unknowns (point coordinates, circle radii, ellipse axes/rotation) live
// in a single flat parameter vector so the constraint solver can treat the whole
// sketch as one nonlinear system.
//
// Geometry is authored directly against the sketch: [Sketch.AddPoint] takes
// coordinates and returns a durable [Point] handle; the curve builders
// ([Sketch.AddLine], [Sketch.AddCircle], [Sketch.AddArc], …) take those points.
// Topology is expressed by sharing a [Point] between entities. The [geom]
// package is the transient math/snapshot layer: [Entity] values expose their
// current geometry as a fresh geom value via their Geometry method, and the
// modification tools use geom for intermediate math, but geom values are never
// committed as sketch geometry.
//
// A Sketch is not safe for concurrent use.
type Sketch struct {
	vars  []float64 // flat parameter vector (all scalar unknowns)
	fixed []bool    // parallel to vars; true == grounded / not solved for

	points []*Point
	ents   []Entity
	cons   []Constraint

	params   *param.Table        // optional; drives bound dimensions
	sys      units.System        // default length/angle units
	pl       *Plane              // placement; nil reads as the world XY datum
	refSeals map[Entity][]*Point // reference entity -> its construction-time defining points (topology seal)
}

// New returns an empty sketch placed on the world XY datum plane, using metric
// default units (millimetres and degrees); change the units with
// [Sketch.SetUnits].
func New() *Sketch { return newSketch(WorldXY()) }

// NewOn returns an empty sketch placed on plane, for engine-only (world-less)
// use. plane must be a live, owner-less plane (a world-frame datum from
// [WorldXY]/[PlaneFromFrame]/[PlaneFromPoints]): it returns [ErrWorldOwnedPlane]
// for a world-owned plane (use [World.Sketch] for those) and [ErrPlaneRemoved]
// for a removed plane. A nil plane is normalized to the world XY datum (so
// NewOn(nil) equals New()).
func NewOn(plane *Plane) (*Sketch, error) {
	if plane == nil {
		plane = WorldXY()
	}
	if plane.removed {
		return nil, ErrPlaneRemoved
	}
	if plane.owner != nil {
		return nil, ErrWorldOwnedPlane
	}
	return newSketch(plane), nil
}

// newSketch is the shared constructor for [New]/[NewOn]/[World.Sketch].
func newSketch(plane *Plane) *Sketch {
	return &Sketch{sys: units.Metric(), pl: plane}
}

// Plane returns the construction plane the sketch is drawn on. A sketch created
// without an explicit placement reads as the world XY datum.
func (s *Sketch) Plane() *Plane { return s.plane() }

// plane returns the sketch's placement, defaulting a nil placement to the world
// XY datum. The nil default is a zero-value/unmarshal safety net so world
// read-out never dereferences a nil plane; it is not a license for a v2 document
// to omit placement (the loader rejects that).
func (s *Sketch) plane() *Plane {
	if s.pl == nil {
		return WorldXY()
	}
	return s.pl
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

// worldPolylineSegments is the per-curve sampling density of [Sketch.WorldPolyline].
const worldPolylineSegments = 32

// WorldPolyline samples entity e in world space: its plane-local polyline (the
// same curve-sampling math the exporters use) lifted through the sketch plane's
// frame. It is the additive 3D read path for placing 2D geometry in 3D; it does
// not change what the 2D exporters emit. e must be a live entity of this sketch
// ([ErrForeignEntity] otherwise); it errors for a degenerate or removed plane
// (well-formed planes never error) and for an unsupported entity type.
func (s *Sketch) WorldPolyline(e Entity) ([]space.Vec3, error) {
	local, err := s.localPolyline(e)
	if err != nil {
		return nil, err
	}
	f, err := s.plane().Frame()
	if err != nil {
		return nil, err
	}
	out := make([]space.Vec3, len(local))
	for i, p := range local {
		out[i] = f.ToWorldUV(p[0], p[1])
	}
	return out, nil
}

// localPolyline samples entity e (which must belong to this sketch) into
// plane-local 2D points via the centralized geom samplers (geom/sample.go).
func (s *Sketch) localPolyline(e Entity) ([][2]float64, error) {
	if !s.ownsEntity(e) {
		return nil, ErrForeignEntity
	}
	switch t := e.(type) {
	case *Line:
		return t.Geometry().Polyline(), nil
	case *Circle:
		return t.Geometry().Polyline(worldPolylineSegments), nil
	case *Arc:
		return t.Geometry().Polyline(worldPolylineSegments), nil
	case *Ellipse:
		return t.Geometry().Polyline(worldPolylineSegments), nil
	case *Spline:
		return t.Polyline(worldPolylineSegments), nil
	}
	return nil, fmt.Errorf("sketch: entity type %T cannot be sampled", e)
}

// ownsEntity reports whether e is a live entity of this sketch (id in range and
// the slot still holds it), mirroring how removed handles are treated as dead.
func (s *Sketch) ownsEntity(e Entity) bool {
	if isNilEntity(e) { // also catches a typed-nil interface, whose entID() would panic
		return false
	}
	id := e.entID()
	return id >= 0 && id < len(s.ents) && s.ents[id] == e
}

// --- Point ------------------------------------------------------------------

// Point is a solver-bound point. Its coordinates are unknowns solved for by the
// constraint solver unless the point is grounded with [Sketch.Fix]. Create one
// with [Sketch.AddPoint] and share it between entities to express topology.
type Point struct {
	s            *Sketch
	xi, yi       int // indices into Sketch.vars
	id           int // index into Sketch.points
	name         string
	construction bool
	refState     // reference-geometry provenance (stale = coordinate freshness)
}

// IsStale reports whether this reference point's coordinates may be out of date
// with its 3D source (always false for non-reference points).
func (p *Point) IsStale() bool { return p.stale }

// X returns the point's current (solved) x coordinate.
func (p *Point) X() float64 { return p.s.vars[p.xi] }

// Y returns the point's current (solved) y coordinate.
func (p *Point) Y() float64 { return p.s.vars[p.yi] }

// ID returns the stable index of the point within its sketch.
func (p *Point) ID() int { return p.id }

// Name returns the point's optional label.
func (p *Point) Name() string { return p.name }

// SetName sets the point's optional label.
func (p *Point) SetName(name string) { p.name = name }

// IsConstruction reports whether the point is construction geometry.
func (p *Point) IsConstruction() bool { return p.construction }

// SetConstruction marks the point as construction geometry or not. It is a
// no-op on reference geometry (the two categories are mutually exclusive).
func (p *Point) SetConstruction(v bool) {
	if !p.reference {
		p.construction = v
	}
}

// Geometry returns a fresh [geom.Point] snapshot at the point's current
// coordinates.
func (p *Point) Geometry() *geom.Point { return geom.NewPoint(p.x(), p.y()) }

// World returns the point's world-space coordinates: its plane-local (x, y)
// lifted through the sketch plane's frame, in base units (millimetres). For a
// degenerate or removed plane it returns the zero vector; use [Point.WorldErr]
// to detect that case (well-formed planes never error).
func (p *Point) World() space.Vec3 {
	f, err := p.s.plane().Frame()
	if err != nil {
		return space.Vec3{}
	}
	return f.ToWorldUV(p.x(), p.y())
}

// WorldErr reports any error computing the sketch plane's frame — only possible
// for a degenerate or removed plane. It is nil for a well-formed plane.
func (p *Point) WorldErr() error {
	_, err := p.s.plane().Frame()
	return err
}

// DistanceTo returns the Euclidean distance from this point to other, in base
// units, at the current solved coordinates.
func (p *Point) DistanceTo(other *Point) float64 { return p.Geometry().DistanceTo(other.Geometry()) }

// DistanceToLine returns the perpendicular distance from this point to the
// infinite line through l, in base units, at the current solved coordinates.
func (p *Point) DistanceToLine(l *Line) float64 { return p.Geometry().DistanceToLine(l.Geometry()) }

func (p *Point) x() float64 { return p.s.vars[p.xi] }
func (p *Point) y() float64 { return p.s.vars[p.yi] }

// AddPoint adds a point at (x, y), allocating its solver variables, and returns
// its handle. Share the returned point between entities to make them meet.
func (s *Sketch) AddPoint(x, y float64) *Point {
	p := &Point{s: s, xi: s.newVar(x), yi: s.newVar(y), id: len(s.points)}
	s.points = append(s.points, p)
	return p
}

// MoveTo moves a point to (x, y). This sets the solver's starting guess for the
// point and has no effect once constraints pin it down. It is a no-op on
// reference geometry, whose coordinates are externally locked — re-feed those
// with [Sketch.RefreshReference].
func (p *Point) MoveTo(x, y float64) {
	if p.reference {
		return
	}
	p.s.vars[p.xi] = x
	p.s.vars[p.yi] = y
}

// Fix grounds a point at its current location so the solver will not move it.
// To ground a point at a specific location, move it first: p.MoveTo(x, y) then
// s.Fix(p).
func (s *Sketch) Fix(p *Point) {
	s.fixed[p.xi] = true
	s.fixed[p.yi] = true
}

// Unfix releases a previously grounded point so the solver may move it again. It
// is a no-op on reference geometry, whose lock cannot be lifted through the
// grounding API.
func (s *Sketch) Unfix(p *Point) {
	if p.reference {
		return
	}
	s.fixed[p.xi] = false
	s.fixed[p.yi] = false
}

// IsFixed reports whether the point is grounded.
func (p *Point) IsFixed() bool { return p.s.fixed[p.xi] && p.s.fixed[p.yi] }

// entityPoints returns an entity's defining points (endpoints, center, control
// points). entitySizeVars returns the extra solver variables an entity owns
// beyond its points — a circle's radius, an ellipse's semi-axes and rotation. An
// arc's radius is derived from its points, so it owns no size variable.
func (s *Sketch) entityPoints(e Entity) []*Point {
	switch t := e.(type) {
	case *Line:
		return []*Point{t.Start, t.End}
	case *Circle:
		return []*Point{t.Center}
	case *Arc:
		return []*Point{t.Center, t.Start, t.End}
	case *Ellipse:
		return []*Point{t.Center}
	case *Spline:
		return t.Control
	}
	return nil
}

func (s *Sketch) entitySizeVars(e Entity) []int {
	switch t := e.(type) {
	case *Circle:
		return []int{t.ri}
	case *Ellipse:
		return []int{t.rxi, t.ryi, t.roti}
	}
	return nil
}

// FixEntity grounds all of an entity's variables — its defining points and any
// size variables (a circle's radius, an ellipse's semi-axes and rotation) — so
// the solver holds the whole entity rigid at its current shape and location. It
// is the entity-level counterpart of [Sketch.Fix].
func (s *Sketch) FixEntity(e Entity) {
	for _, p := range s.entityPoints(e) {
		s.fixed[p.xi] = true
		s.fixed[p.yi] = true
	}
	for _, i := range s.entitySizeVars(e) {
		s.fixed[i] = true
	}
}

// UnfixEntity releases an entity's variables previously grounded with
// [Sketch.FixEntity]. It is a no-op on reference geometry; it also leaves any
// reference-locked point the entity happens to share untouched, since a
// reference lock cannot be lifted through the grounding API.
func (s *Sketch) UnfixEntity(e Entity) {
	if e.IsReference() {
		return
	}
	for _, p := range s.entityPoints(e) {
		if p.reference {
			continue // a shared, externally-locked reference point keeps its lock
		}
		s.fixed[p.xi] = false
		s.fixed[p.yi] = false
	}
	for _, i := range s.entitySizeVars(e) {
		s.fixed[i] = false
	}
}

// EntityFixed reports whether all of an entity's variables are grounded.
func (s *Sketch) EntityFixed(e Entity) bool {
	pts := s.entityPoints(e)
	sz := s.entitySizeVars(e)
	if len(pts) == 0 && len(sz) == 0 {
		return false
	}
	for _, p := range pts {
		if !s.fixed[p.xi] || !s.fixed[p.yi] {
			return false
		}
	}
	for _, i := range sz {
		if !s.fixed[i] {
			return false
		}
	}
	return true
}

// --- Entities ---------------------------------------------------------------

// Entity is a line, circle, arc, ellipse or spline in a sketch. Construction
// status is a settable per-entity property; reference status (externally-locked
// 3D-snapshot geometry with a source id and staleness) is set at creation by the
// AddReference… constructors and is read-only.
type Entity interface {
	entity()
	entID() int
	IsConstruction() bool
	SetConstruction(v bool)
	IsReference() bool
	Source() string
	IsStale() bool
}

// Circular is a sketch entity with a center point and a radius: a [*Circle] or
// an [*Arc]. Constraints that relate centers and radii — [NewTangent],
// [NewTangentCircles], [NewEqualRadius] — accept either.
type Circular interface {
	Entity
	R() float64
	centerPt() *Point
}

// Line is a straight segment between two sketch points.
type Line struct {
	s            *Sketch
	Start, End   *Point
	id           int
	construction bool
	refState     // stale derived from the endpoints
}

func (l *Line) entity()              {}
func (l *Line) entID() int           { return l.id }
func (l *Line) IsConstruction() bool { return l.construction }
func (l *Line) SetConstruction(v bool) {
	if !l.reference {
		l.construction = v
	}
}

// IsStale reports whether either endpoint is stale (a line owns no coordinate of
// its own, so its staleness is derived).
func (l *Line) IsStale() bool { return l.Start.IsStale() || l.End.IsStale() }

// Geometry returns a fresh [geom.Line] snapshot at the line's current
// coordinates.
func (l *Line) Geometry() *geom.Line { return geom.NewLine(l.Start.Geometry(), l.End.Geometry()) }

// Length returns the current distance between the line's endpoints.
func (l *Line) Length() float64 { return math.Hypot(l.End.x()-l.Start.x(), l.End.y()-l.Start.y()) }

// AngleTo returns the signed directed angle from this line to other, in radians
// (in (-π, π]) — the same quantity an [Angle] constraint drives.
func (l *Line) AngleTo(other *Line) float64 { return l.Geometry().AngleTo(other.Geometry()) }

// AddLine adds a line between two points and returns its handle.
func (s *Sketch) AddLine(start, end *Point) *Line {
	l := &Line{s: s, Start: start, End: end, id: len(s.ents)}
	s.ents = append(s.ents, l)
	return l
}

// Circle is a full circle with a center point and a solved radius.
type Circle struct {
	s            *Sketch
	Center       *Point
	ri           int // radius index into Sketch.vars
	id           int
	construction bool
	refState     // stale = radius freshness (center staleness is the center point's)
}

func (c *Circle) entity()              {}
func (c *Circle) entID() int           { return c.id }
func (c *Circle) IsConstruction() bool { return c.construction }
func (c *Circle) SetConstruction(v bool) {
	if !c.reference {
		c.construction = v
	}
}

// IsStale reports whether the circle's center or its radius is out of date with
// the 3D source.
func (c *Circle) IsStale() bool { return c.Center.IsStale() || c.stale }

// Geometry returns a fresh [geom.Circle] snapshot at the circle's current state.
func (c *Circle) Geometry() *geom.Circle { return geom.NewCircle(c.Center.Geometry(), c.r()) }

// R returns the circle's current radius.
func (c *Circle) R() float64 { return c.s.vars[c.ri] }

func (c *Circle) r() float64 { return c.s.vars[c.ri] }

func (c *Circle) centerPt() *Point { return c.Center }

// AddCircle adds a circle with the given center point and radius, allocating the
// radius variable, and returns its handle.
func (s *Sketch) AddCircle(center *Point, r float64) *Circle {
	c := &Circle{s: s, Center: center, ri: s.newVar(r), id: len(s.ents)}
	s.ents = append(s.ents, c)
	return c
}

// Arc is a circular arc swept counter-clockwise from Start to End about Center.
// Its radius is implied by the geometry; an internal constraint keeps the start
// and end equidistant from the center so the arc stays valid.
type Arc struct {
	s                  *Sketch
	Center, Start, End *Point
	id                 int
	construction       bool
	refState           // stale derived from center/start/end
}

func (a *Arc) entity()              {}
func (a *Arc) entID() int           { return a.id }
func (a *Arc) IsConstruction() bool { return a.construction }
func (a *Arc) SetConstruction(v bool) {
	if !a.reference {
		a.construction = v
	}
}

// IsStale reports whether any defining point is stale (derived).
func (a *Arc) IsStale() bool { return a.Center.IsStale() || a.Start.IsStale() || a.End.IsStale() }

// Geometry returns a fresh [geom.Arc] snapshot at the arc's current state.
func (a *Arc) Geometry() *geom.Arc {
	return geom.NewArc(a.Center.Geometry(), a.Start.Geometry(), a.End.Geometry())
}

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

// AddArc adds an arc swept counter-clockwise from start to end about center, and
// the internal radius-consistency constraint. Returns its handle.
func (s *Sketch) AddArc(center, start, end *Point) *Arc {
	a := &Arc{s: s, Center: center, Start: start, End: end, id: len(s.ents)}
	s.ents = append(s.ents, a)
	s.cons = append(s.cons, &arcRadius{a})
	return a
}

// Ellipse is a full ellipse: a center point plus solved semi-axes and rotation.
// Pin them with [NewSemiMajor], [NewSemiMinor] and [NewEllipseRotation]
// dimensions (the center is a regular point, grounded with [Sketch.Fix]).
type Ellipse struct {
	s              *Sketch
	Center         *Point
	rxi, ryi, roti int // var indices: semi-axes and rotation
	id             int
	construction   bool
	refState       // reference ellipses are a follow-up; stale derived from center
}

func (e *Ellipse) entity()              {}
func (e *Ellipse) entID() int           { return e.id }
func (e *Ellipse) IsConstruction() bool { return e.construction }
func (e *Ellipse) SetConstruction(v bool) {
	if !e.reference {
		e.construction = v
	}
}

// IsStale reports whether the ellipse's center is stale (derived; reference
// ellipses are not yet authorable).
func (e *Ellipse) IsStale() bool { return e.Center.IsStale() }

// Geometry returns a fresh [geom.Ellipse] snapshot at the ellipse's current
// state.
func (e *Ellipse) Geometry() *geom.Ellipse {
	return geom.NewEllipse(e.Center.Geometry(), e.rx(), e.ry(), e.rot())
}

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

// AddEllipse adds an ellipse with the given center point, semi-axes and rotation
// (radians), allocating their variables, and returns its handle.
func (s *Sketch) AddEllipse(center *Point, rx, ry, rotation float64) *Ellipse {
	e := &Ellipse{
		s: s, Center: center,
		rxi: s.newVar(rx), ryi: s.newVar(ry), roti: s.newVar(rotation),
		id: len(s.ents),
	}
	s.ents = append(s.ents, e)
	return e
}

// --- Errors -----------------------------------------------------------------

// ErrNotConverged is returned by [Sketch.Solve] when the solver fails to drive
// all constraints to within tolerance within the iteration budget.
var ErrNotConverged = errors.New("sketch: constraint solver did not converge")

// ErrForeignEntity is returned by [Sketch.WorldPolyline] when the entity is nil,
// a removed (dead) handle, or belongs to a different sketch.
var ErrForeignEntity = errors.New("sketch: entity is not a live member of this sketch")
