package sketch

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/units"
)

// Sketch holds solver-bound geometry and the constraints that relate it. All
// scalar unknowns (point coordinates, circle radii, ellipse axes/rotation) live
// in a single flat parameter vector so the constraint solver can treat the whole
// sketch as one nonlinear system.
//
// Geometry is authored directly against the sketch: [Sketch.CreatePoint] takes
// coordinates and returns a durable [Point] handle; the curve builders
// ([Sketch.CreateLine], [Sketch.CreateCircle], [Sketch.CreateArc], …) take those points.
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

	// origin is the sketch's own origin point (see [Sketch.Origin]). It is
	// deliberately NOT in points: keeping it out leaves that slice the authored
	// id space it has always been, so no existing id, document or point count
	// shifts. Its two vars are fixed for the sketch's life, so freeVars never
	// selects them and the solver, rank analysis and conditioning never see it.
	origin *Point

	world    *World                // owning world (every sketch belongs to one)
	params   *param.Table          // drives bound dimensions; shared with the world
	sys      units.System          // default length/angle units
	pl       *Plane                // placement; nil reads as the world XY datum
	refSeals map[Entity][]*Point   // reference entity -> its construction-time defining points (topology seal)
	conNames map[Constraint]string // optional constraint labels (see names.go); not on the constraint itself

	entUIDs   map[Entity]uint64 // entity -> its instance identity (see addEntity); in-memory only, never serialized
	nextEntID uint64            // monotonically increasing uid source; never rewound, never reused
}

// addEntity commits a freshly built entity: it stamps the entity with a stable
// INSTANCE IDENTITY (its uid) and appends it to the entity slice. Every entity
// builder goes through here, and it is the ONLY place a uid is ever stamped —
// so "every entity in s.ents carries a nonzero uid" is an invariant established
// at entry, never repaired later. [Sketch.Revision] only READS the uid (see
// Sketch.entUID): fingerprinting a sketch must not mutate it, or a read-looking
// call would race with itself.
//
// The uid exists because an entity's positional id is NOT an identity: removal
// splices and renumbers (see removal.go), so removing an entity and creating an
// identical one puts a DIFFERENT instance at the same id with the same type,
// points and shape. A [Profile] hands out LIVE entity handles (Profile.Entities,
// BoundaryEdge.Entity), so a consumer that records a profile structurally would
// then hold a handle the sketch no longer owns while [Profile.IsStale] reported
// fresh. Hashing the uid in [Sketch.Revision] makes that swap visible.
//
// uids are never reused: the counter only ever increases, and removal deletes
// the map entry without rewinding it. That holds ACROSS an in-place rebuild too
// — [Sketch.UnmarshalJSON] resets the sketch struct but carries the counter over,
// so the rebuilt entities get fresh uids above the retired ones. The uid is
// in-memory state, not document content: it is never serialized (a loaded
// document's uids are assigned, not read), which is exactly why the counter must
// survive the reset rather than be restored from the document.
func (s *Sketch) addEntity(e Entity) {
	if s.entUIDs == nil {
		s.entUIDs = map[Entity]uint64{}
	}
	s.nextEntID++
	s.entUIDs[e] = s.nextEntID
	s.ents = append(s.ents, e)
}

// newSketch is the shared constructor used by [World.CreateSketch] and the
// document loaders. Every sketch belongs to a world; obtain one with
// [World.CreateSketch] on a plane from [World.XY]/[World.XZ]/[World.YZ] (or a
// created plane).
func newSketch(plane *Plane) *Sketch {
	s := &Sketch{sys: units.Metric(), pl: plane}
	s.initOrigin()
	return s
}

// originPointID is the origin point's id. It is NEGATIVE on purpose: every
// authored point's id is its position in s.points, and the origin is not in that
// slice, so it needs an id no authored point can ever hold. It doubles as the
// origin's serialized point reference — see [Sketch.pointRef].
const originPointID = -1

// initOrigin creates the sketch's origin point: two solver variables at (0, 0),
// both fixed for the sketch's whole life. Every construction path must call it
// (see [newSketch] and the loaders), so that [Sketch.Origin] is never nil for a
// sketch the package built.
//
// It is deliberately NOT lazy. A lazily-created origin would make a read a
// mutator — the bug removed from [Sketch.Revision] — and would race two readers
// of one sketch.
func (s *Sketch) initOrigin() {
	p := &Point{s: s, xi: s.newVar(0), yi: s.newVar(0), id: originPointID}
	s.fixed[p.xi] = true
	s.fixed[p.yi] = true
	s.origin = p
}

// Origin returns the sketch's origin point: a point at the plane origin (0, 0)
// that the engine provides, the solver never moves, and geometry can be
// constrained to like any other point.
//
// It exists before anything is drawn and is grounded from the start, so it is the
// anchor a sketch ties itself to:
//
//	p := s.CreatePoint(0, 0)
//	s.AddConstraint(sketch.NewCoincident(p, s.Origin()))
//
// That REPLACES [Sketch.Fix] at the anchor rather than adding a second way to
// ground. Constraining to the origin keeps the tie inside the parameter model —
// it is a constraint like any other, visible to [Sketch.Diagnose] and removable
// with [Sketch.RemoveConstraint] — whereas fixing a point writes the solver's
// fixed flags directly, which no constraint diagnostic can see.
//
// It is NOT in [Sketch.Points], is never serialized as a point, and cannot be
// removed, unfixed or moved: it is engine-provided rather than authored, and its
// coordinates are the plane origin by definition. A constraint referencing it IS
// serialized, and a document carrying one declares a schema version older readers
// reject rather than mis-load.
//
// It is nil only for a zero-value [Sketch] built outside the package, which is
// not a usable sketch; obtain one from [World.CreateSketch].
func (s *Sketch) Origin() *Point { return s.origin }

// Plane returns the construction plane the sketch is drawn on. A sketch created
// without an explicit placement reads as the world XY datum.
func (s *Sketch) Plane() *Plane { return s.plane() }

// World returns the world that owns this sketch.
func (s *Sketch) World() *World { return s.world }

// plane returns the sketch's placement, defaulting a nil placement to the
// owning world's XY datum (or, for a worldless zero-value sketch, a throwaway
// world's XY). The default is a zero-value/unmarshal safety net so world
// read-out never dereferences a nil plane; it is not a license for a v2
// document to omit placement (the loader rejects that).
func (s *Sketch) plane() *Plane {
	if s.pl != nil {
		return s.pl
	}
	if s.world != nil {
		return s.world.XY()
	}
	return NewWorld().XY() // zero-value safety net: a worldless sketch reads as world-XY
}

func (s *Sketch) newVar(v float64) int {
	s.vars = append(s.vars, v)
	s.fixed = append(s.fixed, false)
	return len(s.vars) - 1
}

// Points returns the points in creation order.
//
// The returned slice is a COPY: the elements are the sketch's live *Point
// handles (mutate a point through its own API — MoveTo, Fix — as usual), but
// writing to a SLOT of the returned slice does not reach the sketch. See
// [Sketch.Entities] for why the copy is load-bearing.
func (s *Sketch) Points() []*Point { return slices.Clone(s.points) }

// Entities returns the lines, circles, arcs and ellipses in creation order.
//
// The returned slice is a COPY: the elements are the sketch's live [Entity]
// handles, but writing to a SLOT of the returned slice does not reach the
// sketch. That copy is load-bearing, not politeness. Handing out the backing
// array would let a caller do
//
//	s.Entities()[i] = &sketch.Line{Start: a, End: b}
//
// — the entity types are exported with exported fields — and splice an entity
// into the sketch that never passed through the addEntity funnel, so it carries
// no instance identity (uid), no allocated solver vars, and no id matching its
// slot. Every invariant the engine rests on (id == slice position, entity-owned
// vars, the [Sketch.Revision] fingerprint that makes [Profile.IsStale] work) is
// bypassed at once. Entities enter the sketch only through the builders.
func (s *Sketch) Entities() []Entity { return slices.Clone(s.ents) }

// Constraints returns the constraints in creation order.
//
// The returned slice is a COPY: the elements are the sketch's live [Constraint]
// values, but writing to a SLOT of the returned slice does not reach the sketch.
// Reordering or duplicating the backing slice would silently shift the
// row->constraint attribution every diagnostic (RedundantConstraints, Diagnose,
// ConflictSet) derives from residuals(); constraints enter and leave through
// [Sketch.AddConstraint] / [Sketch.RemoveConstraint] only.
func (s *Sketch) Constraints() []Constraint { return slices.Clone(s.cons) }

// worldPolylineSegments is the per-curve sampling density of [Sketch.WorldPolyline].
const worldPolylineSegments = 32

// WorldPolyline samples entity e in world space: its plane-local polyline
// (sampled through the geom samplers, so it agrees with what the exporters draw,
// though each exporter carries its own type switch rather than calling this
// path) lifted through the sketch plane's frame. It is the additive 3D read path
// for placing 2D geometry in 3D; it does not change what the 2D exporters emit.
// e must be a live entity of this sketch
// ([ErrForeignEntity] otherwise); it errors for a degenerate or removed plane
// (well-formed planes never error) and for an unsupported entity type.
func (s *Sketch) WorldPolyline(e Entity) ([]r3.Vec, error) {
	local, err := s.localPolyline(e)
	if err != nil {
		return nil, err
	}
	f, err := s.plane().Frame()
	if err != nil {
		return nil, err
	}
	out := make([]r3.Vec, len(local))
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
	case *EllipticalArc:
		return t.Geometry().Polyline(worldPolylineSegments), nil
	case *Spline:
		return t.Polyline(worldPolylineSegments), nil
	case *ClosedSpline:
		return t.Polyline(worldPolylineSegments), nil
	case *FitSpline:
		return t.Polyline(worldPolylineSegments), nil
	case *Conic:
		return t.Polyline(worldPolylineSegments), nil
	case *NURBS:
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
// with [Sketch.CreatePoint] and share it between entities to express topology.
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
// no-op on reference geometry (the two categories are mutually exclusive) and on
// the sketch's [Sketch.Origin], which is not drawn geometry at all.
func (p *Point) SetConstruction(v bool) {
	if !p.reference && !p.isOrigin() {
		p.construction = v
	}
}

// isOrigin reports whether this point is its sketch's origin — the one point the
// grounding and coordinate setters refuse. It compares identity against the
// sketch's own origin rather than testing the id, so a point from ANOTHER sketch
// is never mistaken for this one's.
func (p *Point) isOrigin() bool { return p != nil && p.s != nil && p.s.origin == p }

// Geometry returns a fresh [geom.Point] snapshot at the point's current
// coordinates.
func (p *Point) Geometry() *geom.Point { return geom.NewPoint(p.x(), p.y()) }

// World returns the point's world-space coordinates: its plane-local (x, y)
// lifted through the sketch plane's frame, in base units (millimetres). For a
// degenerate or removed plane it returns the zero vector; use [Point.WorldErr]
// to detect that case (well-formed planes never error).
func (p *Point) World() r3.Vec {
	f, err := p.s.plane().Frame()
	if err != nil {
		return r3.Vec{}
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

// CreatePoint adds a point at (x, y), allocating its solver variables, and returns
// its handle. Share the returned point between entities to make them meet.
func (s *Sketch) CreatePoint(x, y float64) *Point {
	p := &Point{s: s, xi: s.newVar(x), yi: s.newVar(y), id: len(s.points)}
	s.points = append(s.points, p)
	return p
}

// MoveTo moves a point to (x, y). This sets the solver's starting guess for the
// point and has no effect once constraints pin it down. It is a no-op on
// reference geometry, whose coordinates are externally locked — re-feed those
// with [Sketch.RefreshReference] — and on the sketch's [Sketch.Origin], which is
// the plane origin by definition.
func (p *Point) MoveTo(x, y float64) {
	if p.reference || p.isOrigin() {
		return
	}
	p.s.vars[p.xi] = x
	p.s.vars[p.yi] = y
}

// The grounding-guard note. Fix/Unfix/FixEntity/UnfixEntity/EntityFixed index
// s.fixed by a handle's own variable indices, so they must screen the handle
// first. Those indices are only meaningful in the sketch that allocated them: a
// LARGE foreign index runs off s.fixed and panics, and a SMALL one is worse — it
// silently grounds or releases THIS sketch's unrelated variable while the passed
// handle is untouched, with nothing anywhere to flag it. A nil handle panics on
// the dereference.
//
// The predicate is s.owns for a point and [Sketch.foreignInput] for an entity —
// the same ones scanReferenceIntegrity, checkNoForeignRefs and the modification
// tools use, so the grounding API cannot diverge from what [Sketch.Verify]
// reports and MarshalJSON refuses. owns carries the origin exception, so
// [Sketch.Origin] and geometry drawn from it stay groundable; foreignInput
// screens an entity's DEFINING POINTS as well as the entity, since entity fields
// are exported and a point can be rewired to another sketch's *Point behind an
// entity this sketch owns.
//
// The refusal is a silent no-op (false for the reporting EntityFixed) because
// none of the five has an error return, and it matches the shape already used
// for handles these methods cannot act on — [Point.MoveTo], Unfix and
// [Point.SetConstruction] on the origin.

// Fix grounds a point at its current location so the solver will not move it.
// To ground a point at a specific location, move it first: p.MoveTo(x, y) then
// s.Fix(p).
//
// Ground, don't pin. Fix exactly one anchor point — the sketch's tie to the
// origin (p.MoveTo(0, 0); s.Fix(p)) — and remove the remaining rotational
// freedom with a single orientation constraint (a horizontal or vertical line).
// Locate every other point with geometric and dimensional constraints, not by
// fixing its coordinates: a fixed coordinate is outside the parameter model, so
// it cannot be driven by a parameter (see [Sketch.Bind]) and will not reflow
// when a driving dimension changes. Fixing interior or non-origin points — or
// more than the single origin anchor — is a non-parametric anti-pattern.
//
// It is a no-op on a point this sketch does not own — nil, a removed handle, or
// another sketch's point.
func (s *Sketch) Fix(p *Point) {
	if !s.owns(p) {
		return // nil, dead, or another sketch's point: see the grounding-guard note
	}
	s.fixed[p.xi] = true
	s.fixed[p.yi] = true
}

// Unfix releases a previously grounded point so the solver may move it again. It
// is a no-op on a point this sketch does not own (nil, a removed handle, or
// another sketch's point), on reference geometry, whose lock cannot be lifted
// through the grounding API, and on the sketch's [Sketch.Origin], which is
// grounded for the sketch's whole life.
func (s *Sketch) Unfix(p *Point) {
	if !s.owns(p) {
		return // nil, dead, or another sketch's point: see the grounding-guard note
	}
	if p.reference || p.isOrigin() {
		return
	}
	s.fixed[p.xi] = false
	s.fixed[p.yi] = false
}

// IsFixed reports whether the point is grounded.
func (p *Point) IsFixed() bool { return p.s.fixed[p.xi] && p.s.fixed[p.yi] }

// entityPoints returns an entity's defining points (endpoints, center, control
// points), read from the entity's own exported fields. It is the SINGLE
// definition of "which points define this entity", and every consumer goes
// through it: grounding ([Sketch.FixEntity]/[Sketch.UnfixEntity]/
// [Sketch.EntityFixed]), the removal cascade, serialization, the reference
// lock-integrity and reachability scan, the [Sketch.Revision] fingerprint,
// per-entity DOF attribution, and conflict coloring. Keeping one definition is
// load-bearing: a second copy agreeing today would let a new entity type — or a
// new point on an existing one — be added to one and forgotten in the other,
// and the consumers of the forgotten copy fail SILENTLY (Verify stops flagging
// a foreign point, Revision stops moving, so a stale Profile reads fresh). A
// new entity type MUST get a case here.
func entityPoints(e Entity) []*Point {
	switch t := e.(type) {
	case *Line:
		return []*Point{t.Start, t.End}
	case *Circle:
		return []*Point{t.Center}
	case *Arc:
		return []*Point{t.Center, t.Start, t.End}
	case *Ellipse:
		return []*Point{t.Center}
	case *EllipticalArc:
		return []*Point{t.Center, t.Start, t.End}
	case *Spline:
		return t.Control
	case *ClosedSpline:
		return t.Control
	case *FitSpline:
		return t.Fit
	case *Conic:
		return []*Point{t.Start, t.Apex, t.End}
	case *NURBS:
		return t.Control
	}
	return nil
}

// varKind is the physical kind of the quantity a solver variable holds. The
// ambiguity probe perturbs by it and measures configuration distance in it
// ([Sketch.varKinds], configSep), and the conditioning gate scales the
// Jacobian's columns by it (condVarScales).
type varKind uint8

const (
	varCoordinate    varKind = iota // point x/y (the default)
	varRadius                       // circle radius, ellipse semi-axes
	varAngle                        // ellipse rotation
	varDimensionless                // a bounded ratio: a conic's fullness rho ∈ (0, 1)
)

// shapeVar is one intrinsic shape variable of an entity: its index into the
// sketch's variable vector, and the kind of quantity it holds.
type shapeVar struct {
	index int
	kind  varKind
}

// entityShapeVars returns the intrinsic shape variables an entity owns beyond
// its defining points — a circle's radius, an ellipse's or elliptical arc's
// semi-axes and rotation, a conic's fullness rho — each with the physical kind
// of the quantity it holds. A line, an arc and the spline families own none:
// their shape is fixed by their points, an arc's radius being the derived
// distance from its center to its start.
//
// It is the SINGLE definition of "which scalar variables does this entity own",
// and every consumer goes through it: grounding
// ([Sketch.FixEntity]/[Sketch.UnfixEntity]/[Sketch.EntityFixed]), the removal
// cascade's variable retirement, per-entity DOF attribution
// (entityMovable, behind [Sketch.EntityIsFullyConstrained]) and the
// variable-kind table ([Sketch.varKinds]) the ambiguity probe perturbs by and
// the conditioning gate scales columns by. Keeping one definition is
// load-bearing in the same way it is for entityPoints: a second copy agreeing
// today would let a new entity type — or a new variable on an existing one — be
// added to one and forgotten in the other, and the forgotten copy's consumers
// fail SILENTLY (FixEntity leaves a grounded entity able to change shape,
// RemoveEntity leaves a dead entity's variable free in the rank analysis, the
// probe perturbs a radius as if it were a coordinate) with the build, vet, lint
// and test gates all green. A new entity type owning any scalar variable of its
// own MUST get a case here, each variable with its kind.
//
// The entity set here is exactly the one the predecessor entitySizeVars carried,
// a conic's rho included (`git log -S entitySizeVars -- sketch.go` reaches it),
// so pairing each variable with its kind changed which variables an entity
// grounds for no type: a free conic reaches DOF() == 0 under FixEntity, which
// TestFixEntityGroundsEveryVariable pins.
func entityShapeVars(e Entity) []shapeVar {
	switch t := e.(type) {
	case *Circle:
		return []shapeVar{{t.ri, varRadius}}
	case *Ellipse:
		return []shapeVar{{t.rxi, varRadius}, {t.ryi, varRadius}, {t.roti, varAngle}}
	case *EllipticalArc:
		return []shapeVar{{t.rxi, varRadius}, {t.ryi, varRadius}, {t.roti, varAngle}}
	case *Conic:
		return []shapeVar{{t.rhoi, varDimensionless}}
	}
	return nil
}

// FixEntity grounds all of an entity's variables — its defining points and any
// shape variables (a circle's radius, an ellipse's semi-axes and rotation) — so
// the solver holds the whole entity rigid at its current shape and location. It
// is the entity-level counterpart of [Sketch.Fix].
//
// It is a no-op on an entity this sketch does not own — nil, a removed handle,
// another sketch's entity, or one of this sketch's entities with a defining
// point rewired to a point this sketch does not own.
func (s *Sketch) FixEntity(e Entity) {
	if s.foreignInput(e) {
		return // see the grounding-guard note above [Sketch.Fix]
	}
	for _, p := range entityPoints(e) {
		s.fixed[p.xi] = true
		s.fixed[p.yi] = true
	}
	for _, v := range entityShapeVars(e) {
		s.fixed[v.index] = true
	}
}

// UnfixEntity releases an entity's variables previously grounded with
// [Sketch.FixEntity]. It is a no-op on reference geometry; it also leaves
// untouched any point the entity shares whose grounding the grounding API cannot
// lift — a reference-locked point (locked externally) and the sketch's
// [Sketch.Origin] (grounded for the sketch's whole life), exactly as
// [Sketch.Unfix] refuses both. It is likewise a no-op on an entity this sketch
// does not own — nil, a removed handle, another sketch's entity, or one of this
// sketch's entities with a defining point rewired to a point this sketch does
// not own.
func (s *Sketch) UnfixEntity(e Entity) {
	if s.foreignInput(e) {
		return // see the grounding-guard note above [Sketch.Fix]
	}
	if e.IsReference() {
		return
	}
	for _, p := range entityPoints(e) {
		if p.reference || p.isOrigin() {
			continue // externally-locked reference point, or the always-grounded origin
		}
		s.fixed[p.xi] = false
		s.fixed[p.yi] = false
	}
	for _, v := range entityShapeVars(e) {
		s.fixed[v.index] = false
	}
}

// EntityFixed reports whether all of an entity's variables are grounded. It
// reports false for an entity this sketch does not own — nil, a removed handle,
// another sketch's entity, or one of this sketch's entities with a defining
// point rewired to a point this sketch does not own — the same answer it already
// gives for an entity with no variables at all.
func (s *Sketch) EntityFixed(e Entity) bool {
	if s.foreignInput(e) {
		return false // see the grounding-guard note above [Sketch.Fix]
	}
	pts := entityPoints(e)
	shape := entityShapeVars(e)
	if len(pts) == 0 && len(shape) == 0 {
		return false
	}
	for _, p := range pts {
		if !s.fixed[p.xi] || !s.fixed[p.yi] {
			return false
		}
	}
	for _, v := range shape {
		if !s.fixed[v.index] {
			return false
		}
	}
	return true
}

// --- Entities ---------------------------------------------------------------

// Entity is a line, circle, arc, ellipse or spline in a sketch. Construction
// status is a settable per-entity property; reference status (externally-locked
// 3D-snapshot geometry with a source id and staleness) is set at creation by the
// CreateReference… constructors and is read-only. The optional, non-unique name
// is a settable label that survives JSON round-trips and is looked up with
// [Sketch.EntityByName].
type Entity interface {
	entity()
	// isNil reports whether the interface holds a nil concrete pointer — a
	// typed nil, e.g. the *Line in NewHorizontal(nil), which `e == nil` misses
	// and every other method on this interface would panic on. The body is
	// always `return x == nil` for receiver x; it MUST NOT dereference the
	// receiver. Being on the sealed interface is the point: a new entity type
	// that forgets it fails to compile at [Sketch.addEntity], where a forgotten
	// case in a type switch would have compiled and panicked at run time
	// instead (see [isNilEntity]).
	isNil() bool
	entID() int
	IsConstruction() bool
	SetConstruction(v bool)
	IsReference() bool
	Source() string
	IsStale() bool
	Name() string
	SetName(name string)
}

// Circular is a sketch entity with a center point and a radius: a [*Circle] or
// an [*Arc]. Constraints that relate centers and radii — [NewTangent],
// [NewTangentCircles], [NewEqualRadius] — accept either.
type Circular interface {
	Entity
	R() float64
	centerPt() *Point
}

// Elliptical is a sketch entity whose shape is an ellipse: a [*Ellipse] or an
// [*EllipticalArc]. The semi-axis and rotation dimensions ([NewSemiMajor],
// [NewSemiMinor], [NewEllipseRotation]) accept either; for an elliptical arc
// they constrain its underlying ellipse's shape (not its sweep). Like
// [Circular], it exposes the value accessors a consumer needs.
type Elliptical interface {
	Entity
	Rx() float64
	Ry() float64
	Rotation() float64
	centerPt() *Point
}

// Line is a straight segment between two sketch points.
type Line struct {
	s            *Sketch
	Start, End   *Point
	id           int
	construction bool
	named        // optional label
	refState     // stale derived from the endpoints
}

func (l *Line) entity()              {}
func (l *Line) isNil() bool          { return l == nil }
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

// CreateLine adds a line between two points and returns its handle.
func (s *Sketch) CreateLine(start, end *Point) *Line {
	l := &Line{s: s, Start: start, End: end, id: len(s.ents)}
	s.addEntity(l)
	return l
}

// Circle is a full circle with a center point and a solved radius.
type Circle struct {
	s            *Sketch
	Center       *Point
	ri           int // radius index into Sketch.vars
	id           int
	construction bool
	named        // optional label
	refState     // stale = radius freshness (center staleness is the center point's)
}

func (c *Circle) entity()              {}
func (c *Circle) isNil() bool          { return c == nil }
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

// CreateCircle adds a circle with the given center point and radius, allocating the
// radius variable, and returns its handle.
func (s *Sketch) CreateCircle(center *Point, r float64) *Circle {
	c := &Circle{s: s, Center: center, ri: s.newVar(r), id: len(s.ents)}
	s.addEntity(c)
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
	named              // optional label
	refState           // stale derived from center/start/end
}

func (a *Arc) entity()              {}
func (a *Arc) isNil() bool          { return a == nil }
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

// CreateArc adds an arc swept counter-clockwise from start to end about center, and
// the internal radius-consistency constraint. Returns its handle.
func (s *Sketch) CreateArc(center, start, end *Point) *Arc {
	a := &Arc{s: s, Center: center, Start: start, End: end, id: len(s.ents)}
	s.addEntity(a)
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
	named          // optional label
	refState       // reference ellipses are a follow-up; stale derived from center
}

func (e *Ellipse) entity()              {}
func (e *Ellipse) isNil() bool          { return e == nil }
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

func (e *Ellipse) centerPt() *Point { return e.Center }

// CreateEllipse adds an ellipse with the given center point, semi-axes and rotation
// (radians), allocating their variables, and returns its handle.
func (s *Sketch) CreateEllipse(center *Point, rx, ry, rotation float64) *Ellipse {
	e := &Ellipse{
		s: s, Center: center,
		rxi: s.newVar(rx), ryi: s.newVar(ry), roti: s.newVar(rotation),
		id: len(s.ents),
	}
	s.addEntity(e)
	return e
}

// EllipticalArc is an arc on an ellipse: a center point plus solved semi-axes
// and rotation (like [Ellipse]), restricted to the counter-clockwise sweep from
// Start to End. The two boundary points lie on the ellipse — pinned by internal
// constraints auto-added at construction — and the swept extent is measured in
// the ellipse's eccentric angle.
type EllipticalArc struct {
	s                  *Sketch
	Center, Start, End *Point
	rxi, ryi, roti     int // var indices: semi-axes and rotation
	id                 int
	construction       bool
	named              // optional label
	refState           // reference elliptical arcs are a follow-up
}

func (e *EllipticalArc) entity()              {}
func (e *EllipticalArc) isNil() bool          { return e == nil }
func (e *EllipticalArc) entID() int           { return e.id }
func (e *EllipticalArc) IsConstruction() bool { return e.construction }
func (e *EllipticalArc) SetConstruction(v bool) {
	if !e.reference {
		e.construction = v
	}
}

// IsStale reports whether any defining point is stale (derived).
func (e *EllipticalArc) IsStale() bool {
	return e.Center.IsStale() || e.Start.IsStale() || e.End.IsStale()
}

// Geometry returns a fresh [geom.EllipticalArc] snapshot at the current state.
func (e *EllipticalArc) Geometry() *geom.EllipticalArc {
	return geom.NewEllipticalArc(e.Center.Geometry(), e.Start.Geometry(), e.End.Geometry(), e.rx(), e.ry(), e.rot())
}

// Rx and Ry return the current semi-axes along the local x and y axes; Rotation
// returns the local frame's rotation (radians counter-clockwise).
func (e *EllipticalArc) Rx() float64       { return e.s.vars[e.rxi] }
func (e *EllipticalArc) Ry() float64       { return e.s.vars[e.ryi] }
func (e *EllipticalArc) Rotation() float64 { return e.s.vars[e.roti] }

func (e *EllipticalArc) rx() float64  { return e.s.vars[e.rxi] }
func (e *EllipticalArc) ry() float64  { return e.s.vars[e.ryi] }
func (e *EllipticalArc) rot() float64 { return e.s.vars[e.roti] }

func (e *EllipticalArc) centerPt() *Point { return e.Center }

// StartParam, EndParam and Sweep return the endpoints' eccentric angles and the
// counter-clockwise eccentric-angle sweep in (0, 2π].
func (e *EllipticalArc) StartParam() float64 { return e.Geometry().StartParam() }
func (e *EllipticalArc) EndParam() float64   { return e.Geometry().EndParam() }
func (e *EllipticalArc) Sweep() float64      { return e.Geometry().Sweep() }

// CreateEllipticalArc adds an elliptical arc on the ellipse (center, rx, ry,
// rotation) swept counter-clockwise from start to end. It allocates the shape
// variables and auto-adds two internal constraints pinning start and end onto
// the ellipse; for the arc to be valid, start and end should already lie on (or
// near) it. Returns the handle.
func (s *Sketch) CreateEllipticalArc(center, start, end *Point, rx, ry, rotation float64) *EllipticalArc {
	e := &EllipticalArc{
		s: s, Center: center, Start: start, End: end,
		rxi: s.newVar(rx), ryi: s.newVar(ry), roti: s.newVar(rotation),
		id: len(s.ents),
	}
	s.addEntity(e)
	s.cons = append(s.cons, &ellipticalArcOn{e, start}, &ellipticalArcOn{e, end})
	return e
}

// Conic is a conic arc: a rational quadratic Bézier through Start and End with
// apex control point Apex (the intersection of the endpoint tangents) and a
// fullness parameter Rho in (0, 1). Rho is a solver variable (like an ellipse's
// semi-axis), so a later increment can dimension or constrain it; until then it
// is a free degree of freedom. Rho < 0.5 yields an ellipse arc, 0.5 a parabola,
// and > 0.5 a hyperbola arc. The conic carries no internal constraints — it pins
// no point onto an implicit curve.
type Conic struct {
	s                *Sketch
	Start, Apex, End *Point
	rhoi             int // var index of the fullness rho, kept in (0, 1)
	id               int
	construction     bool
	named            // optional label
	refState         // reference conics are a follow-up; stale derived from points
}

func (c *Conic) entity()              {}
func (c *Conic) isNil() bool          { return c == nil }
func (c *Conic) entID() int           { return c.id }
func (c *Conic) IsConstruction() bool { return c.construction }
func (c *Conic) SetConstruction(v bool) {
	if !c.reference {
		c.construction = v
	}
}

// IsStale reports whether any defining point is stale (derived).
func (c *Conic) IsStale() bool {
	return c.Start.IsStale() || c.Apex.IsStale() || c.End.IsStale()
}

// Geometry returns a fresh [geom.Conic] snapshot at the current state.
func (c *Conic) Geometry() *geom.Conic {
	return geom.NewConic(c.Start.Geometry(), c.Apex.Geometry(), c.End.Geometry(), c.rho())
}

// Rho returns the conic's current fullness parameter (in (0, 1)).
func (c *Conic) Rho() float64 { return c.s.vars[c.rhoi] }

func (c *Conic) rho() float64 { return c.s.vars[c.rhoi] }

// Eval returns the conic curve point at parameter t in [0, 1]; Eval(0) = Start,
// Eval(1) = End.
func (c *Conic) Eval(t float64) (float64, float64) { return c.Geometry().Eval(t) }

// Polyline samples the solved conic from Start to End at segments+1 points.
func (c *Conic) Polyline(segments int) [][2]float64 { return c.Geometry().Polyline(segments) }

// CreateConic adds a conic arc — a rational quadratic Bézier — through start and end
// with apex control point apex and fullness rho. It allocates rho as a solver
// variable and returns the handle. It returns [ErrInvalidShape] if rho is not in
// the open interval (0, 1) or any point is nil.
func (s *Sketch) CreateConic(start, apex, end *Point, rho float64) (*Conic, error) {
	if start == nil || apex == nil || end == nil {
		return nil, fmt.Errorf("%w: CreateConic requires non-nil start, apex and end points", ErrInvalidShape)
	}
	if !(rho > 0 && rho < 1) {
		return nil, fmt.Errorf("%w: CreateConic rho must be in (0, 1), got %v", ErrInvalidShape, rho)
	}
	c := &Conic{
		s: s, Start: start, Apex: apex, End: end,
		rhoi: s.newVar(rho),
		id:   len(s.ents),
	}
	s.addEntity(c)
	return c, nil
}

// --- Errors -----------------------------------------------------------------

// ErrNotConverged is returned by [Sketch.Solve] when the solver fails to drive
// all constraints to within tolerance within the iteration budget.
var ErrNotConverged = errors.New("sketch: constraint solver did not converge")

// ErrForeignEntity is returned by [Sketch.WorldPolyline] and by the
// error-returning modification tools ([Sketch.CreateFillet],
// [Sketch.CreateChamfer], [Sketch.CreatePatternRect],
// [Sketch.CreatePatternCircular], [Sketch.CreateOffset]) when a handle is nil, a
// removed (dead) handle, or belongs to a different sketch. The tools with no
// error return report the same condition as [Sketch.Trim], [Sketch.Extend] and
// [Sketch.Break] do — false — or, for [Sketch.CreateMirror], nil.
var ErrForeignEntity = errors.New("sketch: entity is not a live member of this sketch")
