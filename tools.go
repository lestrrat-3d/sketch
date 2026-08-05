package sketch

import (
	"errors"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/units"
)

// Sketch-modification tools mutate committed geometry by the build-then-replace
// pattern: they read the current geometry via each entity's Geometry snapshot,
// compute the replacement with the geom math layer, build the new geometry from
// sketch points (reusing the originals' surviving points so neighbouring
// geometry stays attached), and retire the originals with RemoveEntity.
// Constraints that referenced a replaced entity by identity are dropped with it
// (the removal cascade); constraints on the surviving shared points are kept.
// Replaced handles are dead.

const epsTool = 1e-9

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
	// CreateChamfer, CreatePatternRect, CreatePatternCircular, CreateOffset) when an input is
	// reference geometry, which is externally locked and must not be modified or
	// re-derived. Trim/Extend/Break report it by returning false, and CreateMirror
	// (which has no error return) by returning nil.
	ErrReferenceGeometry = errors.New("sketch: cannot modify reference geometry")
	// ErrUnsupportedEntity is returned by the pattern tools (CreatePatternRect,
	// CreatePatternCircular) when the seed holds an entity kind the copy funnel
	// cannot reproduce. Only lines, circles and arcs are copied: every other
	// kind needs the transform's rotation or reflection applied to a shape the
	// point-relinking interface does not carry. CreateMirror (which has no error
	// return) reports it by returning nil.
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
// a valid tool input.
func (s *Sketch) foreignInput(ents ...Entity) bool {
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

// Break splits a committed line or arc at the projection of (x, y) onto it,
// replacing the original with two entities that share a fresh vertex point at
// the split and reporting them. It returns false (changing nothing) when e is
// not a line or arc, or the projection falls outside the segment / arc sweep.
// It also returns false when e is not a live entity of this sketch (nil, dead,
// or foreign) or is defined by a point that is not a live point of this sketch.
// The original entity handle and any constraints that referenced it
// are gone; constraints on the surviving endpoints remain.
func (s *Sketch) Break(e Entity, x, y float64) (Entity, Entity, bool) {
	if s.foreignInput(e) {
		return nil, nil, false // nil, removed, or another sketch's entity
	}
	if e.IsReference() {
		return nil, nil, false // reference geometry is locked; cannot be split
	}
	switch t := e.(type) {
	case *Line:
		sl := t.Geometry()
		_, param := geom.ClosestPointOnLine(sl, geom.NewPoint(x, y))
		if param <= epsTool || param >= 1-epsTool {
			return nil, nil, false
		}
		mid := s.CreatePoint(sl.Start.X+param*(sl.End.X-sl.Start.X), sl.Start.Y+param*(sl.End.Y-sl.Start.Y))
		l1, l2 := s.CreateLine(t.Start, mid), s.CreateLine(mid, t.End)
		s.RemoveEntity(t)
		return l1, l2, true
	case *Arc:
		sa := t.Geometry()
		cx, cy, r := sa.Center.X, sa.Center.Y, sa.Radius()
		ang := math.Atan2(y-cy, x-cx)
		split := geom.NewPoint(cx+r*math.Cos(ang), cy+r*math.Sin(ang))
		if !sa.Contains(split) || near(split, sa.Start) || near(split, sa.End) {
			return nil, nil, false
		}
		mid := s.CreatePoint(split.X, split.Y)
		a1, a2 := s.CreateArc(t.Center, t.Start, mid), s.CreateArc(t.Center, mid, t.End)
		s.RemoveEntity(t)
		return a1, a2, true
	}
	return nil, nil, false
}

// Trim shortens line l by removing the portion adjacent to an endpoint up to
// its nearest crossing with another entity, choosing the end nearest the pick
// point (x, y). It returns the shortened replacement line and true, or false
// (changing nothing) when l has no crossing on the picked side, or the pick
// sits on an interior portion bounded by crossings on both sides (which would
// split the line — use Break instead), or when l is not a live line of this
// sketch (nil, dead, or foreign) or has an endpoint that is not a live point of
// this sketch. The original handle is dead.
func (s *Sketch) Trim(l *Line, x, y float64) (*Line, bool) {
	if s.foreignInput(l) {
		return nil, false // nil, removed, or another sketch's line
	}
	if l.IsReference() {
		return nil, false // reference geometry is locked; cannot be trimmed
	}
	sl := l.Geometry()
	_, pick := geom.ClosestPointOnLine(sl, geom.NewPoint(x, y))
	hits := s.lineCrossings(sl, l, true)

	loT, hiT := 0.0, 1.0
	var loP, hiP *geom.Point
	for _, h := range hits {
		_, ht := geom.ClosestPointOnLine(sl, h)
		if ht <= epsTool || ht >= 1-epsTool {
			continue
		}
		if ht < pick && ht > loT {
			loT, loP = ht, h
		}
		if ht > pick && ht < hiT {
			hiT, hiP = ht, h
		}
	}

	switch {
	case loP == nil && hiP != nil: // crossing only above the pick: cut the start stub
		nl := s.CreateLine(s.CreatePoint(hiP.X, hiP.Y), l.End)
		s.RemoveEntity(l)
		return nl, true
	case hiP == nil && loP != nil: // crossing only below the pick: cut the end stub
		nl := s.CreateLine(l.Start, s.CreatePoint(loP.X, loP.Y))
		s.RemoveEntity(l)
		return nl, true
	}
	return nil, false
}

// Extend lengthens line l from the given endpoint (l.Start or l.End) to its
// nearest crossing with another entity beyond that end, replacing l and
// returning the lengthened line. It returns false (changing nothing) when end
// is neither endpoint or no entity lies beyond it, and when l, either of its
// endpoints, or end is not a live handle of this sketch (nil, dead, or
// foreign). The original handle is dead.
func (s *Sketch) Extend(l *Line, end *Point) (*Line, bool) {
	if s.foreignInput(l) || !s.owns(end) {
		return nil, false // nil, removed, or another sketch's handle
	}
	if l.IsReference() {
		return nil, false // reference geometry is locked; cannot be extended
	}
	if end != l.Start && end != l.End {
		return nil, false
	}
	sl := l.Geometry()
	hits := s.lineCrossings(sl, l, false)

	fromStart := end == l.Start
	bestT := math.Inf(1)
	var best *geom.Point
	for _, h := range hits {
		_, ht := geom.ClosestPointOnLine(sl, h)
		var beyond float64
		if fromStart {
			if ht >= -epsTool {
				continue // not beyond the Start end
			}
			beyond = -ht
		} else {
			if ht <= 1+epsTool {
				continue // not beyond the End end
			}
			beyond = ht - 1
		}
		if beyond < bestT {
			bestT, best = beyond, h
		}
	}
	if best == nil {
		return nil, false
	}
	np := s.CreatePoint(best.X, best.Y)
	var nl *Line
	if fromStart {
		nl = s.CreateLine(np, l.End)
	} else {
		nl = s.CreateLine(l.Start, np)
	}
	s.RemoveEntity(l)
	return nl, true
}

// lineCrossings returns the plane-local intersection points of sl with every
// entity except skip. When segment is true, line cutters are intersected as
// bounded segments (for trimming); otherwise the infinite line through sl is
// used and a line cutter contributes a point only where it falls within the
// cutter's own segment (for extending). Circle and arc cutters always use the
// infinite line through sl; the caller filters by position along sl.
func (s *Sketch) lineCrossings(sl *geom.Line, skip Entity, segment bool) []*geom.Point {
	var out []*geom.Point
	for _, e := range s.ents {
		if e == skip {
			continue
		}
		switch t := e.(type) {
		case *Line:
			ol := t.Geometry()
			if segment {
				if p, ok := geom.SegmentIntersection(sl, ol); ok {
					out = append(out, p)
				}
				continue
			}
			p, ok := geom.LineLineIntersection(sl, ol)
			if !ok {
				continue
			}
			if _, u := geom.ClosestPointOnLine(ol, p); u >= -epsTool && u <= 1+epsTool {
				out = append(out, p)
			}
		case *Circle:
			out = append(out, geom.LineCircleIntersections(sl, t.Geometry())...)
		case *Arc:
			out = append(out, geom.LineArcIntersections(sl, t.Geometry())...)
		}
	}
	return out
}

// near reports whether two generic points share a location within tolerance.
func near(a, b *geom.Point) bool { return math.Hypot(a.X-b.X, a.Y-b.Y) <= epsTool }

// --- fillet / chamfer -------------------------------------------------------

// Fillet groups the geometry created by [Sketch.CreateFillet]: the rounding arc,
// the two shortened legs that meet it, the contact points T1 (on L1) and T2
// (on L2), and the editable radius dimension.
type Fillet struct {
	Arc    *Arc
	L1, L2 *Line
	T1, T2 *Point
	Radius *Distance
}

// CreateFillet rounds the corner shared by lines l1 and l2 with a tangent arc of
// radius r. The shared corner is removed and each leg is shortened to its
// contact point; the arc is held tangent to both legs and its radius pinned by
// an editable dimension, so editing [Fillet.Radius] and re-solving keeps the
// rounding tangent. The original legs (and any constraints that referenced them
// as entities — horizontal, angle, …) are replaced; re-apply such constraints
// to the returned [Fillet.L1]/[Fillet.L2] if needed. Constraints on the
// surviving far endpoints are kept. Returns [ErrForeignEntity] when either line
// is not a live entity of this sketch (nil, dead, or foreign) or has an
// endpoint that is not a live point of this sketch, [ErrNoSharedCorner] or
// [ErrFilletInfeasible] — in each case without modifying the sketch.
func (s *Sketch) CreateFillet(l1, l2 *Line, r float64) (*Fillet, error) {
	if s.foreignInput(l1, l2) {
		return nil, ErrForeignEntity
	}
	if l1.IsReference() || l2.IsReference() {
		return nil, ErrReferenceGeometry
	}
	corner := sharedPoint(l1, l2)
	if corner == nil {
		return nil, ErrNoSharedCorner
	}
	far1, far2 := otherSketchEnd(l1, corner), otherSketchEnd(l2, corner)

	cc := geom.NewPoint(corner.x(), corner.y())
	copy1 := geom.NewLine(far1.Geometry(), cc)
	copy2 := geom.NewLine(far2.Geometry(), cc)
	arc, ok := geom.Fillet(copy1, copy2, r)
	if !ok {
		return nil, ErrFilletInfeasible
	}
	c1, c2 := otherEnd(copy1, copy1.Start), otherEnd(copy2, copy2.Start)

	t1, t2 := s.CreatePoint(c1.X, c1.Y), s.CreatePoint(c2.X, c2.Y)
	ctr := s.CreatePoint(arc.Center.X, arc.Center.Y)
	nL1 := s.orientLeg(l1, corner, far1, t1)
	nL2 := s.orientLeg(l2, corner, far2, t2)
	nArc := s.CreateArc(ctr, t1, t2)
	if arc.Start != c1 { // geom.Fillet took the minor arc the other way
		nArc = s.CreateArc(ctr, t2, t1)
	}

	s.AddConstraint(NewTangent(nL1, nArc), NewTangent(nL2, nArc))
	rad := NewDistance(nArc.Center, nArc.Start, r)
	s.AddConstraint(rad)

	s.RemoveEntity(l1)
	s.RemoveEntity(l2)
	s.RemovePoint(corner)

	return &Fillet{Arc: nArc, L1: nL1, L2: nL2, T1: t1, T2: t2, Radius: rad}, nil
}

// Chamfer groups the geometry created by [Sketch.CreateChamfer]: the cut line, the
// two shortened legs, the contact points T1/T2, and the editable setback
// dimensions D1/D2 (each the surviving distance from a far endpoint to its
// contact).
type Chamfer struct {
	Cut    *Line
	L1, L2 *Line
	T1, T2 *Point
	D1, D2 *Distance
}

// CreateChamfer cuts the corner shared by lines l1 and l2 with a straight line
// whose contacts sit d from the corner along each leg. The corner is removed
// and each leg shortened to its contact; the contacts are pinned by editable
// distance dimensions from the far endpoints (D1/D2), so editing them and
// re-solving moves the chamfer parametrically. Constraint handling matches
// [Sketch.CreateFillet]. Returns [ErrForeignEntity] when either line is not a
// live entity of this sketch (nil, dead, or foreign) or has an endpoint that is
// not a live point of this sketch, [ErrNoSharedCorner] or
// [ErrChamferInfeasible] — in each case without modifying the sketch.
func (s *Sketch) CreateChamfer(l1, l2 *Line, d float64) (*Chamfer, error) {
	if s.foreignInput(l1, l2) {
		return nil, ErrForeignEntity
	}
	if l1.IsReference() || l2.IsReference() {
		return nil, ErrReferenceGeometry
	}
	corner := sharedPoint(l1, l2)
	if corner == nil {
		return nil, ErrNoSharedCorner
	}
	far1, far2 := otherSketchEnd(l1, corner), otherSketchEnd(l2, corner)
	len1 := math.Hypot(far1.x()-corner.x(), far1.y()-corner.y())
	len2 := math.Hypot(far2.x()-corner.x(), far2.y()-corner.y())

	cc := geom.NewPoint(corner.x(), corner.y())
	copy1 := geom.NewLine(far1.Geometry(), cc)
	copy2 := geom.NewLine(far2.Geometry(), cc)
	cut, ok := geom.Chamfer(copy1, copy2, d)
	if !ok {
		return nil, ErrChamferInfeasible
	}
	c1, c2 := otherEnd(copy1, copy1.Start), otherEnd(copy2, copy2.Start)

	t1, t2 := s.CreatePoint(c1.X, c1.Y), s.CreatePoint(c2.X, c2.Y)
	nL1 := s.orientLeg(l1, corner, far1, t1)
	nL2 := s.orientLeg(l2, corner, far2, t2)
	nCut := s.CreateLine(t1, t2)
	if cut.Start != c1 {
		nCut = s.CreateLine(t2, t1)
	}

	d1, d2 := NewDistance(far1, t1, len1-d), NewDistance(far2, t2, len2-d)
	s.AddConstraint(d1, d2)

	s.RemoveEntity(l1)
	s.RemoveEntity(l2)
	s.RemovePoint(corner)

	return &Chamfer{Cut: nCut, L1: nL1, L2: nL2, T1: t1, T2: t2, D1: d1, D2: d2}, nil
}

// sharedPoint returns the point shared by both lines, or nil if they do not
// meet at an endpoint.
func sharedPoint(l1, l2 *Line) *Point {
	switch {
	case l1.Start == l2.Start || l1.Start == l2.End:
		return l1.Start
	case l1.End == l2.Start || l1.End == l2.End:
		return l1.End
	}
	return nil
}

// otherSketchEnd returns the endpoint of l that is not known.
func otherSketchEnd(l *Line, known *Point) *Point {
	if l.Start == known {
		return l.End
	}
	return l.Start
}

// otherEnd returns the endpoint of generic line l that is not known.
func otherEnd(l *geom.Line, known *geom.Point) *geom.Point {
	if l.Start == known {
		return l.End
	}
	return l.Start
}

// orientLeg builds the replacement leg for orig, putting the fresh contact
// point where the corner used to be and reusing the surviving far point.
func (s *Sketch) orientLeg(orig *Line, corner, far, contact *Point) *Line {
	if orig.Start == corner {
		return s.CreateLine(contact, far)
	}
	return s.CreateLine(far, contact)
}

// --- mirror -----------------------------------------------------------------

// Mirror groups the geometry created by [Sketch.CreateMirror]: the source entities
// (left in place), their mirrored copies, and the constraints that keep each
// copy the reflection of its source — symmetric constraints on every point pair
// plus an equal-radius constraint per circle. Editing a source and re-solving
// moves its copy.
type Mirror struct {
	Originals   []Entity
	Copies      []Entity
	Constraints []Constraint
}

// containsReference reports whether any entity is reference geometry.
func containsReference(ents []Entity) bool {
	for _, e := range ents {
		if e != nil && e.IsReference() {
			return true
		}
	}
	return false
}

// unsupportedSeed reports the first entity in ents whose kind the copy funnel
// (Sketch.instantiate) cannot reproduce, and whether there was one.
//
// It is an ALLOW-LIST with a refusing default, not a deny-list of the kinds that
// are dropped today. An entity type added later is refused by this switch
// without being named in it, so the worst a forgotten update can do is refuse a
// kind the funnel could in fact copy — never silently drop one it cannot. The
// deny-list direction fails the other way: an unnamed new type would pass the
// screen, fall through instantiate's switch, and reinstate exactly the silent
// drop this exists to close.
//
// Every caller must call it BEFORE committing anything, so a refusal leaves the
// sketch unchanged. A check inside instantiate would come too late:
// CreatePatternRect calls instantiate once per cell, so cell (1,0)'s points and
// dimensions are already committed by the time cell (2,0) refuses.
func unsupportedSeed(ents []Entity) (Entity, bool) {
	for _, e := range ents {
		switch e.(type) {
		case *Line, *Circle, *Arc:
		default:
			return e, true
		}
	}
	return nil, false
}

// CreateMirror reflects each entity across the infinite line through axis,
// creating a linked copy. Only lines, circles and arcs can be copied; it
// returns nil if the seed holds any other entity kind. Copies share a mirror
// point wherever their sources share a vertex, so a connected source chain
// mirrors to a connected copy. Each source point and its copy are tied with
// [NewSymmetric] (about axis); circles additionally get [NewEqualRadius], and
// arcs are reversed to stay counter-clockwise. The sources are left untouched.
// Returns nil if any source or the axis is reference geometry, is not a live
// entity of this sketch (nil, dead, or foreign), or is defined by a point that
// is not a live point of this sketch.
func (s *Sketch) CreateMirror(ents []Entity, axis *Line) *Mirror {
	if s.foreignInput(ents...) || s.foreignInput(axis) {
		return nil // nil, removed, or another sketch's entity
	}
	if containsReference(ents) || axis.IsReference() {
		return nil // reference geometry is locked; cannot be mirrored
	}
	if _, ok := unsupportedSeed(ents); ok {
		return nil // an entity kind the copy funnel cannot reproduce
	}
	gaxis := axis.Geometry()
	copyOf := map[*Point]*Point{}
	grp := &Pattern{Seed: ents}
	link := func(src *Point) *Point {
		if cp, ok := copyOf[src]; ok {
			return cp
		}
		mp := geom.MirrorPoint(src.Geometry(), gaxis)
		cp := s.CreatePoint(mp.X, mp.Y)
		cp.SetConstruction(src.IsConstruction())
		copyOf[src] = cp
		c := NewSymmetric(src, cp, axis)
		s.AddConstraint(c)
		grp.Constraints = append(grp.Constraints, c)
		return cp
	}
	// Reflection reverses arc sweep, so copies are committed start/end-swapped.
	s.instantiate(ents, link, true, grp)
	return &Mirror{Originals: ents, Copies: grp.Instances, Constraints: grp.Constraints}
}

// --- patterns ---------------------------------------------------------------

// Pattern groups the geometry created by [Sketch.CreatePatternRect] and
// [Sketch.CreatePatternCircular]: the seed entities (left in place), every copy
// instance, the constraints tying each copy to the seed, and — for a circular
// pattern — the construction spokes that hold the angular spacing.
type Pattern struct {
	Seed        []Entity
	Instances   []Entity
	Constraints []Constraint
	Spokes      []*Line
}

// CreatePatternRect tiles the seed entities into an nx-by-ny grid with spacing dx
// (along x) and dy (along y). The seed occupies cell (0,0); every other cell
// holds a copy whose points are tied to the seed's corresponding points by
// horizontal and vertical distance dimensions (and an equal-radius constraint
// per circle), so the whole grid follows when the seed is moved or resized.
// It returns [ErrForeignEntity] if a seed entity, or a point defining one, is
// not live in this sketch, [ErrInvalidShape] if nx or ny is below 1, and
// [ErrUnsupportedEntity] if the seed holds an entity kind other than a line,
// circle or arc — only those three can be copied, so the refusal is what keeps
// the limit from silently shrinking the pattern.
func (s *Sketch) CreatePatternRect(ents []Entity, nx, ny int, dx, dy float64) (*Pattern, error) {
	if s.foreignInput(ents...) {
		return nil, ErrForeignEntity
	}
	if containsReference(ents) {
		return nil, ErrReferenceGeometry
	}
	if nx < 1 || ny < 1 {
		return nil, fmt.Errorf("%w: CreatePatternRect requires nx,ny >= 1, got %d,%d", ErrInvalidShape, nx, ny)
	}
	if e, ok := unsupportedSeed(ents); ok {
		return nil, fmt.Errorf("%w: CreatePatternRect cannot copy %T; only *sketch.Line, *sketch.Circle and *sketch.Arc are copied", ErrUnsupportedEntity, e)
	}
	p := &Pattern{Seed: ents}
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			if i == 0 && j == 0 {
				continue // the seed itself
			}
			ox, oy := float64(i)*dx, float64(j)*dy
			copyOf := map[*Point]*Point{}
			link := func(src *Point) *Point {
				if cp, ok := copyOf[src]; ok {
					return cp
				}
				cp := s.CreatePoint(src.x()+ox, src.y()+oy)
				cp.SetConstruction(src.IsConstruction())
				copyOf[src] = cp
				hd, vd := NewHorizontalDistance(src, cp, ox), NewVerticalDistance(src, cp, oy)
				s.AddConstraint(hd, vd)
				p.Constraints = append(p.Constraints, hd, vd)
				return cp
			}
			s.instantiate(ents, link, false, p)
		}
	}
	return p, nil
}

// CreatePatternCircular copies the seed entities n times at equal angular steps
// (2π/n) about center; cell 0 is the seed. Each copy point is tied to its
// source by a construction spoke from center constrained equal in length and at
// the instance's angle, so the ring follows when the seed is moved. It returns
// [ErrForeignEntity] if a seed entity, a point defining one, or the center is
// not a live handle of this sketch, [ErrInvalidShape] if n is below 2, and
// [ErrUnsupportedEntity] if the seed holds an entity kind other than a line,
// circle or arc — only those three can be copied, so the refusal is what keeps
// the limit from silently shrinking the pattern.
func (s *Sketch) CreatePatternCircular(ents []Entity, center *Point, n int) (*Pattern, error) {
	if s.foreignInput(ents...) || !s.owns(center) {
		return nil, ErrForeignEntity
	}
	if containsReference(ents) {
		return nil, ErrReferenceGeometry
	}
	if n < 2 {
		return nil, fmt.Errorf("%w: CreatePatternCircular requires n >= 2, got %d", ErrInvalidShape, n)
	}
	if e, ok := unsupportedSeed(ents); ok {
		return nil, fmt.Errorf("%w: CreatePatternCircular cannot copy %T; only *sketch.Line, *sketch.Circle and *sketch.Arc are copied", ErrUnsupportedEntity, e)
	}
	step := 2 * math.Pi / float64(n)
	p := &Pattern{Seed: ents}
	srcSpoke := map[*Point]*Line{}
	spokeFor := func(src *Point) *Line {
		if sp, ok := srcSpoke[src]; ok {
			return sp
		}
		sp := s.CreateLine(center, src)
		sp.SetConstruction(true)
		srcSpoke[src] = sp
		p.Spokes = append(p.Spokes, sp)
		return sp
	}
	for k := 1; k < n; k++ {
		ang := step * float64(k)
		gcenter := center.Geometry()
		copyOf := map[*Point]*Point{}
		link := func(src *Point) *Point {
			if cp, ok := copyOf[src]; ok {
				return cp
			}
			rp := geom.RotatePoint(src.Geometry(), gcenter, ang)
			cp := s.CreatePoint(rp.X, rp.Y)
			cp.SetConstruction(src.IsConstruction())
			copyOf[src] = cp
			ssrc := spokeFor(src)
			scp := s.CreateLine(center, cp)
			scp.SetConstruction(true)
			p.Spokes = append(p.Spokes, scp)
			eq := NewEqual(ssrc, scp)
			an := NewAngle(ssrc, scp, 0)
			_ = an.SetValue(units.Radians(ang))
			s.AddConstraint(eq, an)
			p.Constraints = append(p.Constraints, eq, an)
			return cp
		}
		s.instantiate(ents, link, false, p)
	}
	return p, nil
}

// --- offset -----------------------------------------------------------------

// OffsetGroup groups the geometry created by [Sketch.CreateOffset]: the source
// lines (left in place), their offset copies, and the per-segment offset
// constraints. Use [OffsetGroup.Set] to retarget the whole offset distance at
// once and re-solve.
type OffsetGroup struct {
	Source  []Entity
	Copies  []Entity
	Offsets []*Offset
}

// Set retargets every segment's offset distance to d. Call [Sketch.Solve]
// afterwards to apply it.
func (g *OffsetGroup) Set(d float64) {
	for _, o := range g.Offsets {
		o.Set(d)
	}
}

// CreateOffset creates a parallel offset of a chain of lines at signed distance d
// (positive on the left of each segment's start→end direction). Each source
// line gets an offset copy held parallel at distance d by an [Offset]
// constraint; offset segments that meet at a shared source corner share a
// single offset point, which the two constraints pull to the offset
// intersection — so editing the distance keeps a mitred chain. Non-line
// entities in ents are skipped. It returns [ErrForeignEntity] if a source
// entity, or a point defining one, is not live in this sketch, and
// [ErrInvalidShape] if a source line has zero length (no defined offset
// direction).
func (s *Sketch) CreateOffset(ents []Entity, d float64) (*OffsetGroup, error) {
	if s.foreignInput(ents...) {
		return nil, ErrForeignEntity
	}
	if containsReference(ents) {
		return nil, ErrReferenceGeometry
	}
	// Validate every source line up front so a failure leaves the sketch
	// unchanged (no partial geometry committed).
	for _, e := range ents {
		l, ok := e.(*Line)
		if !ok {
			continue
		}
		if math.Hypot(l.End.x()-l.Start.x(), l.End.y()-l.Start.y()) == 0 {
			return nil, fmt.Errorf("%w: CreateOffset source line has zero length", ErrInvalidShape)
		}
	}
	g := &OffsetGroup{Source: ents}
	copyOf := map[*Point]*Point{}
	offsetPt := func(src *Point, nx, ny float64) *Point {
		if cp, ok := copyOf[src]; ok {
			return cp
		}
		cp := s.CreatePoint(src.x()+d*nx, src.y()+d*ny)
		copyOf[src] = cp
		return cp
	}
	for _, e := range ents {
		l, ok := e.(*Line)
		if !ok {
			continue
		}
		dx, dy := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		n := math.Hypot(dx, dy) // nonzero: validated above
		nx, ny := -dy/n, dx/n   // left normal of the segment direction
		qa, qb := offsetPt(l.Start, nx, ny), offsetPt(l.End, nx, ny)
		dst := s.CreateLine(qa, qb)
		off := NewOffset(l, dst, d)
		s.AddConstraint(off)
		g.Copies = append(g.Copies, dst)
		g.Offsets = append(g.Offsets, off)
	}
	return g, nil
}

// instantiate builds one copy of every entity in ents using link to map each
// source point to its copy, appending the copies (and any radius constraints)
// to grp; copies inherit their source's construction flag. swapArc reverses arc
// orientation (used by mirror, not by patterns).
func (s *Sketch) instantiate(ents []Entity, link func(*Point) *Point, swapArc bool, grp *Pattern) {
	// Only line/circle/arc are copied: the rotated-shape entities (ellipse,
	// elliptical arc) and splines need the transform's rotation/reflection
	// applied to their shape — information the point-relinking interface here
	// does not carry — so the modification tools do not yet pattern/mirror them.
	// Every caller screens the seed with unsupportedSeed first, so an unhandled
	// type is refused before anything is committed rather than dropped here.
	for _, e := range ents {
		switch t := e.(type) {
		case *Line:
			cl := s.CreateLine(link(t.Start), link(t.End))
			cl.SetConstruction(t.IsConstruction())
			grp.Instances = append(grp.Instances, cl)
		case *Circle:
			cir := s.CreateCircle(link(t.Center), t.r())
			cir.SetConstruction(t.IsConstruction())
			eq := NewEqualRadius(t, cir)
			s.AddConstraint(eq)
			grp.Constraints = append(grp.Constraints, eq)
			grp.Instances = append(grp.Instances, cir)
		case *Arc:
			cc, cs, ce := link(t.Center), link(t.Start), link(t.End)
			if swapArc {
				cs, ce = ce, cs
			}
			ca := s.CreateArc(cc, cs, ce)
			ca.SetConstruction(t.IsConstruction())
			grp.Instances = append(grp.Instances, ca)
		}
	}
}
