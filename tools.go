package sketch

import (
	"errors"
)

// Sketch-modification tools are package-level functions that take the sketch
// first. Where Fusion modifies the original curve (trimming or extending an end)
// they move that curve's end point in place, so the handle and every constraint
// that names it survive. Where Fusion deletes the curve (trimming its middle, or
// a curve with no crossing) they build the replacement from sketch points
// (reusing the originals' surviving points so neighbouring geometry stays
// attached) and retire the original with RemoveEntity; constraints that
// referenced it by identity are dropped with it (the removal cascade). Every
// tool refuses a nil sketch.

const epsTool = 1e-9

// OpenCurve is the set of curve types the modification tools split, trim,
// extend and fillet: curves with two end points. Use it only as a type
// constraint.
type OpenCurve interface{ *Line | *Arc }

// asEntity returns e as an Entity. A typed-nil e gives a non-nil interface
// holding the nil pointer, which foreignInput refuses.
func asEntity[T OpenCurve](e T) Entity {
	ent, _ := any(e).(Entity)
	return ent
}

// asCurve returns e as the curve type T, or T's zero value when e holds a
// different type.
func asCurve[T OpenCurve](e Entity) T {
	t, _ := any(e).(T)
	return t
}

// Errors returned by the corner-modification tools.
var (
	// ErrNoSharedCorner is returned when two lines passed to CreateFillet or
	// CreateChamfer do not share an endpoint point.
	ErrNoSharedCorner = errors.New("sketch: lines do not share a corner point")
	// ErrFilletInfeasible is returned when the fillet radius is not positive,
	// the legs are collinear, or the radius is too large for either leg.
	ErrFilletInfeasible = errors.New("sketch: fillet radius does not fit the corner")
	// ErrChamferInfeasible is returned when the chamfer setback is not positive,
	// the legs are collinear, or the setback exceeds either leg.
	ErrChamferInfeasible = errors.New("sketch: chamfer distance does not fit the corner")
	// ErrReferenceGeometry is returned by the modification tools (CreateFillet,
	// CreateChamfer, CreateMirror, CreatePatternRect, CreatePatternCircular,
	// CreateOffset) when an input is reference geometry, which is externally
	// locked and must not be modified or re-derived. Trim/Extend/Break report it
	// by returning false.
	ErrReferenceGeometry = errors.New("sketch: cannot modify reference geometry")
	// ErrUnsupportedEntity is returned by the copy tools (CreateMirror,
	// CreatePatternRect, CreatePatternCircular) when the seed holds an entity kind
	// the copy funnel cannot reproduce. Only lines, circles, arcs and fit splines
	// are copied: every
	// other kind needs the transform's rotation or reflection applied to a shape the
	// point-relinking interface does not carry.
	ErrUnsupportedEntity = errors.New("sketch: entity kind cannot be copied by this tool")
)

// foreignInput reports whether any of the entities is unusable as a tool input —
// the entity itself not live in this sketch (nil, a removed handle, or one
// belonging to a different sketch), or any of its DEFINING POINTS not a live
// point of this sketch.
//
// Every tool below must call it before touching its inputs. The tools work by
// build-then-replace: they read the entity's defining points and manufacture new
// geometry and constraints in THIS sketch from them. Fed a foreign entity they
// would splice another sketch's points into this one — a handle Verify reports
// as ErrForeignHandle and MarshalJSON refuses to write. Fed a removed one they
// would resurrect deleted geometry, which nothing downstream flags at all.
//
// The POINT half is not covered by the entity half, and is the sharper case.
// Entity fields (Line.Start, …) are exported, so an entity this sketch owns can
// have a point rewired to another sketch's *Point and still pass ownsEntity,
// which is positional over s.ents and never looks at points. Most tools then
// carry that point into the new geometry, where Verify and MarshalJSON still
// catch it — but Trim consuming the trimmed-away end, and CreateFillet /
// CreateChamfer consuming the shared corner, REPLACE the foreign point with
// fresh local points and retire the originals, so the evidence is erased: a
// sketch the oracle rejected becomes one it accepts, built from another
// sketch's coordinates with nothing left to flag.
//
// The scan goes through entityPoints, the ONE definition of an entity's
// defining points, so a new entity type or a new point slot is screened with no
// second list to keep in step. Ownership is s.owns, the same predicate Verify's
// integrity scan and MarshalJSON use — it carries the origin exception, so
// geometry drawn from [Sketch.Origin] (deliberately absent from s.points) stays
// a valid tool input. A nil sketch owns nothing, so every input is foreign to it.
func (s *Sketch) foreignInput(ents ...Entity) bool {
	if s == nil {
		return true
	}
	for _, e := range ents {
		if !s.ownsEntity(e) {
			return true
		}
		for _, p := range entityPoints(e) {
			if !s.owns(p) {
				return true
			}
		}
	}
	return false
}
