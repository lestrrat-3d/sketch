package sketch

import (
	"errors"
	"fmt"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/space"
	"github.com/lestrrat-3d/sketch/units"
)

// Plane-related errors.
var (
	// ErrForeignPlane is returned when a plane handed to a [World] method is
	// nil, owned by a different world, or a removed (dead) handle.
	ErrForeignPlane = errors.New("sketch: plane is not a live member of this world")
	// ErrPlaneInUse is returned by [World.RemovePlane] when a sketch is placed
	// on the plane or another plane uses it as a base.
	ErrPlaneInUse = errors.New("sketch: plane is in use")
	// ErrStandardDatum is returned by [World.RemovePlane] for the seeded XY/XZ/YZ
	// datum planes, which are foundational and cannot be removed.
	ErrStandardDatum = errors.New("sketch: standard datum planes cannot be removed")
	// ErrPlaneRemoved is returned by [Plane.Frame] and [NewOn] for a removed
	// (tombstoned) plane handle.
	ErrPlaneRemoved = errors.New("sketch: plane has been removed")
	// ErrWorldOwnedPlane is returned by [NewOn] when given a world-owned plane:
	// use [World.Sketch] for those.
	ErrWorldOwnedPlane = errors.New("sketch: plane is owned by a world; use World.Sketch")
	// ErrNotOffsetPlane is returned by [World.BindOffsetPlane] for a plane that is
	// not a derived offset plane (only an offset plane has a distance to drive).
	ErrNotOffsetPlane = errors.New("sketch: plane is not an offset plane")
)

// planeKind identifies how a plane's frame is derived.
type planeKind int

const (
	planeXY     planeKind = iota // standard datum: U=+X V=+Y N=+Z
	planeXZ                      // standard datum: U=+X V=+Z N=−Y
	planeYZ                      // standard datum: U=+Y V=+Z N=+X
	planeFrame                   // explicit world frame
	planePoints                  // three world points
	planeOffset                  // derived: base plane offset along its normal
)

// planeDef is a plane's provenance — the single source of truth from which its
// frame is computed. Only the fields relevant to kind are populated.
type planeDef struct {
	kind     planeKind
	frame    space.Frame // planeFrame
	a, b, c  space.Vec3  // planePoints
	base     *Plane      // planeOffset
	dist     float64     // planeOffset literal distance
	distExpr string      // planeOffset: a length expression over the world's params; empty = literal dist
}

// Plane is a construction (datum) plane: a 3D coordinate frame positioned in a
// [World], on which a [Sketch] is drawn. Its frame is computed from a stored
// definition (its provenance), so the plane can never disagree with how it was
// built; create one through the package-level world-frame constructors
// ([WorldXY], [PlaneFromFrame], [PlaneFromPoints]) for engine-only use, or
// through a [World] (which additionally allows derived planes such as
// [World.OffsetPlane]).
type Plane struct {
	def     planeDef
	owner   *World // nil for a standalone (engine-only) plane
	id      int    // slice position within owner.planes; -1 when standalone
	removed bool   // tombstone set by World.RemovePlane; a dead handle
	name    string
}

// WorldXY returns a standalone (owner-less) XY datum plane: origin at the world
// origin, U=+X, V=+Y, N=+Z. It is the default placement for [New].
func WorldXY() *Plane { return &Plane{def: planeDef{kind: planeXY}, id: -1, name: "XY"} }

// WorldXZ returns a standalone XZ datum plane: U=+X, V=+Z, N=−Y.
func WorldXZ() *Plane { return &Plane{def: planeDef{kind: planeXZ}, id: -1, name: "XZ"} }

// WorldYZ returns a standalone YZ datum plane: U=+Y, V=+Z, N=+X.
func WorldYZ() *Plane { return &Plane{def: planeDef{kind: planeYZ}, id: -1, name: "YZ"} }

// PlaneFromFrame returns a standalone plane positioned by an explicit frame. It
// returns [space.ErrDegenerateFrame] when f is not a valid orthonormal frame
// (including the zero value).
func PlaneFromFrame(f space.Frame) (*Plane, error) {
	if !f.IsValid() {
		return nil, space.ErrDegenerateFrame
	}
	return &Plane{def: planeDef{kind: planeFrame, frame: f}, id: -1}, nil
}

// PlaneFromPoints returns a standalone plane through three world points: origin
// a, U along a→b, N along (a→b)×(a→c), V = N×U. It returns
// [space.ErrDegenerateFrame] when the points are collinear.
func PlaneFromPoints(a, b, c space.Vec3) (*Plane, error) {
	if _, err := frameFromPoints(a, b, c); err != nil {
		return nil, err
	}
	return &Plane{def: planeDef{kind: planePoints, a: a, b: b, c: c}, id: -1}, nil
}

// Name returns the plane's optional label.
func (p *Plane) Name() string { return p.name }

// SetName sets the plane's optional label.
func (p *Plane) SetName(name string) { p.name = name }

// ID returns the plane's stable index within its [World], or -1 if it is a
// standalone (owner-less) or removed plane.
func (p *Plane) ID() int { return p.id }

// Frame returns the plane's coordinate frame, recomputed from its definition
// (recursing into a base plane for derived planes). It returns an error for a
// removed plane or a degenerate definition.
func (p *Plane) Frame() (space.Frame, error) {
	if p.removed {
		return space.Frame{}, ErrPlaneRemoved
	}
	switch p.def.kind {
	case planeXY, planeXZ, planeYZ:
		return datumFrame(p.def.kind)
	case planeFrame:
		if !p.def.frame.IsValid() {
			return space.Frame{}, space.ErrDegenerateFrame
		}
		return p.def.frame, nil
	case planePoints:
		return frameFromPoints(p.def.a, p.def.b, p.def.c)
	case planeOffset:
		base := p.def.base
		if base == nil || base.removed {
			return space.Frame{}, ErrPlaneRemoved
		}
		bf, err := base.Frame()
		if err != nil {
			return space.Frame{}, err
		}
		dist, err := p.offsetDist()
		if err != nil {
			return space.Frame{}, err
		}
		origin := bf.Origin().Add(bf.N().Scale(dist))
		return space.NewFrame(origin, bf.U(), bf.V())
	}
	return space.Frame{}, fmt.Errorf("sketch: unknown plane definition kind %d", p.def.kind)
}

// offsetDist resolves an offset plane's distance: a bound length expression
// evaluated against the owning world's parameters (re-evaluated on every call, so
// a parameter edit is reflected immediately), or the literal dist when unbound.
// A bound expression must evaluate to a length — an angle or dimensionless result
// is rejected (the literal dist API is the unitless escape hatch).
func (p *Plane) offsetDist() (float64, error) {
	if p.def.distExpr == "" {
		return p.def.dist, nil
	}
	if p.owner == nil || p.owner.params == nil {
		return 0, fmt.Errorf("sketch: offset plane expression %q has no parameter table", p.def.distExpr)
	}
	v, err := p.owner.params.EvalValue(p.def.distExpr)
	if err != nil {
		return 0, fmt.Errorf("sketch: offset plane distance %q: %w", p.def.distExpr, err)
	}
	if v.Kind() != units.Length {
		return 0, fmt.Errorf("%w: offset plane distance %q is %s, want length", param.ErrIncompatibleKind, p.def.distExpr, v.Kind())
	}
	return v.Base(), nil
}

// datumFrame returns the frame for a standard datum kind. The axes are
// compile-time-known orthonormal, so NewFrame never actually errors here; the
// error is propagated rather than panicked.
func datumFrame(k planeKind) (space.Frame, error) {
	switch k {
	case planeXY:
		return space.NewFrame(space.Vec3{}, space.NewVec3(1, 0, 0), space.NewVec3(0, 1, 0))
	case planeXZ:
		return space.NewFrame(space.Vec3{}, space.NewVec3(1, 0, 0), space.NewVec3(0, 0, 1))
	case planeYZ:
		return space.NewFrame(space.Vec3{}, space.NewVec3(0, 1, 0), space.NewVec3(0, 0, 1))
	}
	return space.Frame{}, fmt.Errorf("sketch: %d is not a datum kind", k)
}

// frameFromPoints builds the frame for a three-point plane definition.
func frameFromPoints(a, b, c space.Vec3) (space.Frame, error) {
	return space.NewFrame(a, b.Sub(a), c.Sub(a))
}
