package sketch

import (
	"fmt"
	"math"
	"slices"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/units"
)

// --- mirror -----------------------------------------------------------------

// Mirror groups the geometry created by [CreateMirror]: the source entities
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
		case *Line, *Circle, *Arc, *FitSpline:
		default:
			return e, true
		}
	}
	return nil, false
}

// CreateMirror reflects each entity across the infinite line through axis,
// creating a linked copy. Only lines, circles, arcs and fit splines can be
// copied. Copies share a mirror point wherever their sources share a vertex, so a
// connected source chain mirrors to a connected copy. Each source point and its
// copy are tied with [NewSymmetric] (about axis); circles additionally get
// [NewEqualRadius], and arcs are reversed to stay counter-clockwise. The sources
// are left untouched. It returns [ErrForeignEntity] when s is nil or a source,
// the axis, or a point defining one is not live in s; [ErrReferenceGeometry]
// when a source or the axis is reference geometry; and [ErrUnsupportedEntity]
// when the seed holds any other entity kind — in each case without modifying
// the sketch.
func CreateMirror(s *Sketch, ents []Entity, axis *Line) (*Mirror, error) {
	if s.foreignInput(ents...) || s.foreignInput(axis) {
		return nil, ErrForeignEntity
	}
	if containsReference(ents) || axis.IsReference() {
		return nil, ErrReferenceGeometry
	}
	if e, ok := unsupportedSeed(ents); ok {
		return nil, fmt.Errorf("%w: CreateMirror cannot copy %T; only *sketch.Line, *sketch.Circle, *sketch.Arc and *sketch.FitSpline are copied", ErrUnsupportedEntity, e)
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
	return &Mirror{Originals: ents, Copies: grp.Instances, Constraints: grp.Constraints}, nil
}

// --- patterns ---------------------------------------------------------------

// Pattern groups the geometry created by [CreatePatternRect] and
// [CreatePatternCircular]: the seed entities (left in place), every copy
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
// It returns [ErrForeignEntity] if s is nil or a seed entity, or a point
// defining one, is not live in s, [ErrInvalidShape] if nx or ny is below 1, and
// [ErrUnsupportedEntity] if the seed holds an entity kind other than a line,
// circle, arc or fit spline — only those four can be copied, so the refusal is
// what keeps the limit from silently shrinking the pattern.
func CreatePatternRect(s *Sketch, ents []Entity, nx, ny int, dx, dy float64) (*Pattern, error) {
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
		return nil, fmt.Errorf("%w: CreatePatternRect cannot copy %T; only *sketch.Line, *sketch.Circle, *sketch.Arc and *sketch.FitSpline are copied", ErrUnsupportedEntity, e)
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
// [ErrForeignEntity] if s is nil or a seed entity, a point defining one, or the
// center is not a live handle of s, [ErrInvalidShape] if n is below 2, and
// [ErrUnsupportedEntity] if the seed holds an entity kind other than a line,
// circle, arc or fit spline — only those four can be copied, so the refusal is
// what keeps the limit from silently shrinking the pattern.
func CreatePatternCircular(s *Sketch, ents []Entity, center *Point, n int) (*Pattern, error) {
	return s.createPatternCircular("CreatePatternCircular", ents, center, n, nil, nil)
}

// CreatePatternCircularRing is [CreatePatternCircular] for a seed that is
// one link of a closed ring: a chain of entities running from the point start to
// the point end, where end is start turned by one step (2π/n) about center. The
// copies are joined to each other and to the seed by SHARING that junction point
// instead of by a coincident constraint, exactly as a ring drawn point by point
// is: the copy k+1 chain starts at the very *Point the copy k chain ends at,
// copy 1 starts at the seed's end, and the last copy ends at the seed's start.
// No coincident is needed, no point is left over, and a profile cut at a
// junction is cut at the shared point's exact coordinates.
//
// The seed's end is not a free point: the tool places it with a spoke from
// center held equal in length to start's and at the step angle from it, so a
// caller must NOT dimension it (one more constraint would be redundant). Draw it
// where it belongs; the solver does not need to move it.
//
// start and end must be distinct, live points of s other than center, and each
// must define at least one seed entity, else [ErrInvalidShape]. All the other
// errors are CreatePatternCircular's, and n below 2 is refused the same way.
func CreatePatternCircularRing(s *Sketch, ents []Entity, center *Point, n int, start, end *Point) (*Pattern, error) {
	if s == nil || !s.owns(start) || !s.owns(end) {
		return nil, ErrForeignEntity
	}
	if start == end || start == center || end == center {
		return nil, fmt.Errorf("%w: CreatePatternCircularRing needs start, end and center to be distinct points", ErrInvalidShape)
	}
	return s.createPatternCircular("CreatePatternCircularRing", ents, center, n, start, end)
}

// createPatternCircular builds both circular patterns. start and end are nil for
// a plain pattern; for a ring they are the seed's first and last junction points
// (see [CreatePatternCircularRing]).
func (s *Sketch) createPatternCircular(name string, ents []Entity, center *Point, n int, start, end *Point) (*Pattern, error) {
	if s.foreignInput(ents...) || !s.owns(center) {
		return nil, ErrForeignEntity
	}
	if containsReference(ents) {
		return nil, ErrReferenceGeometry
	}
	if n < 2 {
		return nil, fmt.Errorf("%w: %s requires n >= 2, got %d", ErrInvalidShape, name, n)
	}
	if e, ok := unsupportedSeed(ents); ok {
		return nil, fmt.Errorf("%w: %s cannot copy %T; only *sketch.Line, *sketch.Circle, *sketch.Arc and *sketch.FitSpline are copied", ErrUnsupportedEntity, name, e)
	}
	ring := start != nil
	if ring && (!seedDefinedBy(ents, start) || !seedDefinedBy(ents, end)) {
		return nil, fmt.Errorf("%w: %s start and end must each define a seed entity", ErrInvalidShape, name)
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
	// tie holds the end of spokeTo at the angle ang from sfrom, at the same
	// distance from the center.
	tie := func(sfrom, spokeTo *Line, ang float64) {
		eq := NewEqual(sfrom, spokeTo)
		an := NewAngle(sfrom, spokeTo, 0)
		_ = an.SetValue(units.Radians(ang))
		s.AddConstraint(eq, an)
		p.Constraints = append(p.Constraints, eq, an)
	}
	if ring {
		// The seed's end is start turned by one step.
		sstart := spokeFor(start)
		tie(sstart, spokeFor(end), step)
	}
	gcenter := center.Geometry()
	var prevEnd *Point // the previous copy's image of end, the next copy's start
	for k := 1; k < n; k++ {
		ang := step * float64(k)
		copyOf := map[*Point]*Point{}
		if ring {
			// Junctions are shared, never re-created: they carry the constraints
			// their first creation gave them, and a second set would be redundant.
			if k == 1 {
				copyOf[start] = end
			} else {
				copyOf[start] = prevEnd
			}
			if k == n-1 {
				copyOf[end] = start
			}
		}
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
			tie(ssrc, scp, ang)
			return cp
		}
		s.instantiate(ents, link, false, p)
		if ring {
			prevEnd = copyOf[end]
		}
	}
	return p, nil
}

// seedDefinedBy reports whether pt is one of the defining points of a seed entity.
func seedDefinedBy(ents []Entity, pt *Point) bool {
	for _, e := range ents {
		if slices.Contains(entityPoints(e), pt) {
			return true
		}
	}
	return false
}

// instantiate builds one copy of every entity in ents using link to map each
// source point to its copy, appending the copies (and any radius constraints)
// to grp; copies inherit their source's construction flag. swapArc reverses arc
// orientation (used by mirror, not by patterns).
func (s *Sketch) instantiate(ents []Entity, link func(*Point) *Point, swapArc bool, grp *Pattern) {
	// Only line/circle/arc/fit spline are copied: the rotated-shape entities
	// (ellipse, elliptical arc) and the other splines need the transform's
	// rotation/reflection applied to their shape — information the
	// point-relinking interface here does not carry — so the modification tools
	// do not yet pattern/mirror them.
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
		case *FitSpline:
			// A fit spline is defined by its fit points alone: the natural cubic
			// through them is parametrized by chord length, which a rotation, a
			// reflection and a translation all preserve, so the copy through the
			// linked points IS the transformed spline and needs no constraint of
			// its own. Shared fit points share their copies through link.
			fit := make([]*Point, len(t.Fit))
			for i, p := range t.Fit {
				fit[i] = link(p)
			}
			// The fit-point count and non-nil links meet CreateFitSpline's minimums.
			cf, _ := s.CreateFitSpline(fit...)
			cf.SetConstruction(t.IsConstruction())
			grp.Instances = append(grp.Instances, cf)
		}
	}
}
