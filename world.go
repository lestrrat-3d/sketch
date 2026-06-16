package sketch

import "github.com/lestrrat-3d/sketch/space"

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
}

// NewWorld returns a world seeded with the three standard datum planes.
func NewWorld() *World {
	w := &World{}
	w.planes = []*Plane{
		{def: planeDef{kind: planeXY}, owner: w, id: 0, name: "XY"},
		{def: planeDef{kind: planeXZ}, owner: w, id: 1, name: "XZ"},
		{def: planeDef{kind: planeYZ}, owner: w, id: 2, name: "YZ"},
	}
	return w
}

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

// PlaneFromFrame adds a world-owned plane positioned by an explicit frame. It
// returns [space.ErrDegenerateFrame] when f is invalid.
func (w *World) PlaneFromFrame(f space.Frame) (*Plane, error) {
	if !f.IsValid() {
		return nil, space.ErrDegenerateFrame
	}
	return w.addPlane(planeDef{kind: planeFrame, frame: f}, ""), nil
}

// PlaneFromPoints adds a world-owned plane through three world points (see
// [PlaneFromPoints]). It returns [space.ErrDegenerateFrame] for collinear points.
func (w *World) PlaneFromPoints(a, b, c space.Vec3) (*Plane, error) {
	if _, err := frameFromPoints(a, b, c); err != nil {
		return nil, err
	}
	return w.addPlane(planeDef{kind: planePoints, a: a, b: b, c: c}, ""), nil
}

// OffsetPlane adds a world-owned plane parallel to base, its origin shifted by
// dist along base's normal. base must be a live plane of this world, else
// [ErrForeignPlane].
func (w *World) OffsetPlane(base *Plane, dist float64) (*Plane, error) {
	if !w.owns(base) {
		return nil, ErrForeignPlane
	}
	return w.addPlane(planeDef{kind: planeOffset, base: base, dist: dist}, ""), nil
}

// Sketch creates a new, empty sketch placed on plane and owned by this world.
// plane must be a live plane of this world, else [ErrForeignPlane].
func (w *World) Sketch(plane *Plane) (*Sketch, error) {
	if !w.owns(plane) {
		return nil, ErrForeignPlane
	}
	s := newSketch(plane)
	w.sketches = append(w.sketches, s)
	return s, nil
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
