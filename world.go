package sketch

import (
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/space"
)

// World is the 3D document root: it owns the construction planes positioned in
// world space and the sketches drawn on them. A fresh World is seeded with the
// three standard datum planes (XY, XZ, YZ) at ids 0, 1, 2.
//
// The constraint engine stays at the core — each [Sketch] still owns its own 2D
// solver, units and parameters. World is a thin spatial owner: it positions
// sketches in 3D and is the serialization root for multi-sketch documents.
//
// A World is not safe for concurrent use.
type World struct {
	planes   []*Plane
	sketches []*Sketch
	params   *param.Table // global parameters shared by every world-owned sketch
}

// NewWorld returns a world seeded with the three standard datum planes and an
// empty global parameter table.
func NewWorld() *World {
	w := &World{params: param.New()}
	w.planes = []*Plane{
		{def: planeDef{kind: planeXY}, owner: w, id: 0, name: "XY"},
		{def: planeDef{kind: planeXZ}, owner: w, id: 1, name: "XZ"},
		{def: planeDef{kind: planeYZ}, owner: w, id: 2, name: "YZ"},
	}
	return w
}

// Params returns the world's global parameter table — shared by every sketch the
// world creates, so a single parameter (e.g. a global thickness) can drive
// dimensions across multiple sketches. Bind a dimension to it with
// `s.Bind(dim, w.Params(), expr)`; a world-owned sketch's own `s.Params()` is
// already this same table.
func (w *World) Params() *param.Table { return w.params }

// XY returns the world's standard XY datum plane.
func (w *World) XY() *Plane { return w.planes[0] }

// XZ returns the world's standard XZ datum plane.
func (w *World) XZ() *Plane { return w.planes[1] }

// YZ returns the world's standard YZ datum plane.
func (w *World) YZ() *Plane { return w.planes[2] }

// Planes returns the world's planes in id order. The slice must not be modified.
func (w *World) Planes() []*Plane { return w.planes }

// Sketches returns the world's sketches in creation order. The slice must not be
// modified.
func (w *World) Sketches() []*Sketch { return w.sketches }

// owns reports whether p is a live member of this world. The w.planes[p.id] == p
// clause rejects a removed (tombstoned) handle whose owner field may be stale.
func (w *World) owns(p *Plane) bool {
	return p != nil && p.owner == w && !p.removed &&
		p.id >= 0 && p.id < len(w.planes) && w.planes[p.id] == p
}

// addPlane registers a new plane definition and returns its handle.
func (w *World) addPlane(def planeDef, name string) *Plane {
	p := &Plane{def: def, owner: w, id: len(w.planes), name: name}
	w.planes = append(w.planes, p)
	return p
}

// CreatePlaneFromFrame adds a world-owned plane positioned by an explicit frame.
// It returns [space.ErrDegenerateFrame] when f is invalid.
func (w *World) CreatePlaneFromFrame(f space.Frame) (*Plane, error) {
	if !f.IsValid() {
		return nil, space.ErrDegenerateFrame
	}
	return w.addPlane(planeDef{kind: planeFrame, frame: f}, ""), nil
}

// CreatePlaneFromPoints adds a world-owned plane through three world points:
// origin a, U along a→b, N along (a→b)×(a→c), V = N×U. It returns
// [space.ErrDegenerateFrame] for collinear points.
func (w *World) CreatePlaneFromPoints(a, b, c space.Vec3) (*Plane, error) {
	if _, err := frameFromPoints(a, b, c); err != nil {
		return nil, err
	}
	return w.addPlane(planeDef{kind: planePoints, a: a, b: b, c: c}, ""), nil
}

// CreateOffsetPlane adds a world-owned plane parallel to base, its origin shifted
// by dist along base's normal. base must be a live plane of this world, else
// [ErrForeignPlane].
func (w *World) CreateOffsetPlane(base *Plane, dist float64) (*Plane, error) {
	if !w.owns(base) {
		return nil, ErrForeignPlane
	}
	return w.addPlane(planeDef{kind: planeOffset, base: base, dist: dist}, ""), nil
}

// CreateSketch creates a new, empty sketch placed on plane and owned by this
// world. plane must be a live plane of this world, else [ErrForeignPlane].
func (w *World) CreateSketch(plane *Plane) (*Sketch, error) {
	if !w.owns(plane) {
		return nil, ErrForeignPlane
	}
	s := newSketch(plane)
	s.world = w
	s.params = w.params // share the world's global parameter table
	w.sketches = append(w.sketches, s)
	return s, nil
}

// BindOffsetPlane drives an offset plane's distance from a length expression
// evaluated against the world's parameters (re-evaluated whenever the plane's
// frame is computed). plane must be a live offset plane of this world. The
// expression is parsed immediately (syntax errors surface here); the names it
// references and its length kind are validated when the frame is computed or by
// [World.Verify]. The expression must evaluate to a length.
func (w *World) BindOffsetPlane(plane *Plane, expr string) error {
	if !w.owns(plane) {
		return ErrForeignPlane
	}
	if plane.def.kind != planeOffset {
		return ErrNotOffsetPlane
	}
	if _, err := param.Parse(expr); err != nil {
		return err
	}
	plane.def.distExpr = expr
	return nil
}

// UnbindOffsetPlane removes an offset plane's parameter binding, freezing the
// currently resolved distance as the new literal so the plane stays where it is
// (rather than reverting to the literal it had before binding). It is a no-op for
// an unbound plane, and returns the evaluation error if the bound expression
// cannot currently be resolved.
func (w *World) UnbindOffsetPlane(plane *Plane) error {
	if !w.owns(plane) {
		return ErrForeignPlane
	}
	if plane.def.kind != planeOffset {
		return ErrNotOffsetPlane
	}
	if plane.def.distExpr != "" {
		dist, err := plane.offsetDist()
		if err != nil {
			return err
		}
		plane.def.dist = dist
	}
	plane.def.distExpr = ""
	return nil
}

// ApplyParameters validates the world's global parameters and every
// parameter-driven plane, then applies the bound parameters of every sketch. It
// does NOT solve the sketches and does NOT cache plane distances (a parameter
// edit is reflected on the next frame computation). It returns the first error
// encountered.
func (w *World) ApplyParameters() error {
	if w.params != nil {
		if err := w.params.Validate(); err != nil {
			return err
		}
	}
	for _, p := range w.planes {
		if p.removed {
			continue
		}
		if _, err := p.Frame(); err != nil { // evaluates a bound offset distance
			return err
		}
	}
	for _, s := range w.sketches {
		if err := s.ApplyParameters(); err != nil {
			return err
		}
	}
	return nil
}

// WorldVerificationReport is the aggregate oracle verdict for a [World]: the
// shared parameters, every plane's frame, and every sketch. It is the multi-
// sketch counterpart of [VerificationReport].
type WorldVerificationReport struct {
	// Sketches holds each sketch's report, in creation order.
	Sketches []*VerificationReport
	// ParametersValid is true when the shared global parameter table validates
	// (no undefined references, cycles, or kind errors). ParameterErrors holds the
	// first such error when it is false.
	ParametersValid bool
	ParameterErrors []error
	// PlaneErrors lists planes whose frame cannot be computed — a degenerate
	// definition or a bad (missing-name / wrong-kind) parameter-driven offset.
	PlaneErrors []error
}

// Trustworthy reports whether the whole world passes: valid shared parameters,
// every plane frame computable, and every sketch [VerificationReport.Trustworthy].
func (r *WorldVerificationReport) Trustworthy() bool {
	if !r.ParametersValid || len(r.PlaneErrors) > 0 {
		return false
	}
	for _, sr := range r.Sketches {
		if !sr.Trustworthy() {
			return false
		}
	}
	return true
}

// Verify aggregates verification across the world: it validates the shared
// parameter table, computes every plane's frame (catching a bad
// parameter-driven offset), and verifies every sketch. It is non-mutating. Any
// [VerifyOption]s are forwarded to each sketch's [Sketch.Verify].
func (w *World) Verify(options ...VerifyOption) *WorldVerificationReport {
	rep := &WorldVerificationReport{ParametersValid: true}
	if w.params != nil {
		if err := w.params.Validate(); err != nil {
			rep.ParametersValid = false
			rep.ParameterErrors = append(rep.ParameterErrors, err)
		}
	}
	for _, p := range w.planes {
		if p.removed {
			continue
		}
		if _, err := p.Frame(); err != nil {
			rep.PlaneErrors = append(rep.PlaneErrors, err)
		}
	}
	for _, s := range w.sketches {
		rep.Sketches = append(rep.Sketches, s.Verify(options...))
	}
	return rep
}

// RemovePlane removes a plane from the world. It refuses ([ErrStandardDatum])
// the seeded XY/XZ/YZ datums, and refuses ([ErrPlaneInUse]) any plane a sketch
// is placed on or another plane uses as a base. On success it splices the plane
// out, renumbers the later plane ids to stay dense, and tombstones the removed
// handle so it can never be reused.
func (w *World) RemovePlane(p *Plane) error {
	if !w.owns(p) {
		return ErrForeignPlane
	}
	if p.id < 3 {
		return ErrStandardDatum
	}
	for _, s := range w.sketches {
		if s.pl == p {
			return ErrPlaneInUse
		}
	}
	for _, other := range w.planes {
		if other.def.kind == planeOffset && other.def.base == p {
			return ErrPlaneInUse
		}
	}
	idx := p.id
	w.planes = append(w.planes[:idx], w.planes[idx+1:]...)
	for i := idx; i < len(w.planes); i++ {
		w.planes[i].id = i
	}
	p.removed = true
	p.owner = nil
	p.id = -1
	return nil
}
