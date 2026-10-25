package sketch

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/units"
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

// Break splits a committed line or arc at the projection of (x, y) onto it,
// replacing the original with two curves of the same type that share a fresh
// vertex point at the split, and returns them. It returns false (changing
// nothing) when the projection falls outside the segment / arc sweep, when s is
// nil, when e is reference geometry, or when e is not a live entity of s (nil,
// dead, or foreign) or is defined by a point that is not a live point of s. The
// original handle and any constraints that referenced it are gone; constraints
// on the surviving endpoints remain.
func Break[T OpenCurve](s *Sketch, e T, x, y float64) (T, T, bool) {
	var zero T
	ent := asEntity(e)
	if s.foreignInput(ent) {
		return zero, zero, false // nil, removed, or another sketch's entity
	}
	if ent.IsReference() {
		return zero, zero, false // reference geometry is locked; cannot be split
	}
	switch t := ent.(type) {
	case *Line:
		sl := t.Geometry()
		_, param := geom.ClosestPointOnLine(sl, geom.NewPoint(x, y))
		if param <= epsTool || param >= 1-epsTool {
			return zero, zero, false
		}
		mid := s.CreatePoint(sl.Start.X+param*(sl.End.X-sl.Start.X), sl.Start.Y+param*(sl.End.Y-sl.Start.Y))
		l1, l2 := s.CreateLine(t.Start, mid), s.CreateLine(mid, t.End)
		s.RemoveEntity(t)
		return asCurve[T](l1), asCurve[T](l2), true
	case *Arc:
		sa := t.Geometry()
		cx, cy, r := sa.Center.X, sa.Center.Y, sa.Radius()
		ang := math.Atan2(y-cy, x-cx)
		split := geom.NewPoint(cx+r*math.Cos(ang), cy+r*math.Sin(ang))
		if !sa.Contains(split) || near(split, sa.Start) || near(split, sa.End) {
			return zero, zero, false
		}
		mid := s.CreatePoint(split.X, split.Y)
		a1, a2 := s.CreateArc(t.Center, t.Start, mid), s.CreateArc(t.Center, mid, t.End)
		s.RemoveEntity(t)
		return asCurve[T](a1), asCurve[T](a2), true
	}
	return zero, zero, false
}

// Trim removes the portion of e between the two crossings with other entities
// that bound the pick point (x, y), as Fusion's trim does; e's own end points
// bound the portion where no crossing does. Picking an end portion moves that end
// of e in place and returns []T{e}: the handle and the constraints that name it
// survive, and the old end point is removed when nothing else uses it. Picking a
// middle portion deletes e and returns the two remaining pieces, which share e's
// far end points. If e crosses nothing, Trim deletes e (and its end points no
// other entity uses) and returns an empty slice. It returns nil and false,
// changing nothing, when s is nil, when e is not a live entity of s (nil, dead,
// or foreign) or has an end point that is not a live point of s, or when e is
// reference geometry. Lines and arcs are both handled; a circle is trimmed by
// [TrimCircle].
func Trim[T OpenCurve](s *Sketch, e T, x, y float64) ([]T, bool) {
	ent := asEntity(e)
	if s.foreignInput(ent) {
		return nil, false // nil, removed, or another sketch's entity
	}
	if ent.IsReference() {
		return nil, false // reference geometry is locked; cannot be trimmed
	}
	var pieces []Entity
	if l, ok := ent.(*Line); ok {
		for _, p := range s.trimLine(l, x, y) {
			pieces = append(pieces, p)
		}
	} else if a, ok := ent.(*Arc); ok {
		for _, p := range s.trimArc(a, x, y) {
			pieces = append(pieces, p)
		}
	}
	out := make([]T, len(pieces))
	for i, p := range pieces {
		out[i] = asCurve[T](p)
	}
	return out, true
}

// trimLine is Trim for a screened line.
func (s *Sketch) trimLine(l *Line, x, y float64) []*Line {
	sl := l.Geometry()
	_, pick := geom.ClosestPointOnLine(sl, geom.NewPoint(x, y))
	hits := s.lineCrossings(sl, l, true)

	loT, hiT := 0.0, 1.0
	var loP, hiP *crossing
	for i := range hits {
		h := &hits[i]
		_, ht := geom.ClosestPointOnLine(sl, h.at)
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
	case loP == nil && hiP == nil: // nothing crosses the picked portion: delete the line
		start, end := l.Start, l.End
		s.RemoveEntity(l)
		s.RemovePoint(start)
		s.RemovePoint(end)
		return []*Line{}
	case loP == nil: // crossing only above the pick: cut the start stub in place
		s.moveEnd(l, l.Start, s.pointOnCutter(*hiP))
		return []*Line{l}
	case hiP == nil: // crossing only below the pick: cut the end stub in place
		s.moveEnd(l, l.End, s.pointOnCutter(*loP))
		return []*Line{l}
	}
	// Crossings on both sides: delete the line and keep the two outer pieces.
	first := s.CreateLine(l.Start, s.pointOnCutter(*loP))
	second := s.CreateLine(s.pointOnCutter(*hiP), l.End)
	s.RemoveEntity(l)
	return []*Line{first, second}
}

// trimArc is Trim for a screened arc: the same three outcomes as trimLine,
// measured by the angle from the arc's start along its sweep.
func (s *Sketch) trimArc(a *Arc, x, y float64) []*Arc {
	ga := a.Geometry()
	sweep := ga.Sweep()
	pick := ga.AngleFromStart(geom.NewPoint(x, y)) / sweep
	if pick > 1 { // the pick's direction is off the arc: take the nearer end
		if (pick-1)*sweep < 2*math.Pi-pick*sweep {
			pick = 1
		} else {
			pick = 0
		}
	}
	hits := s.circleCrossings(geom.NewCircle(ga.Center, ga.Radius()), a)

	loT, hiT := 0.0, 1.0
	var loP, hiP *crossing
	for i := range hits {
		h := &hits[i]
		ht := ga.AngleFromStart(h.at) / sweep
		if ht <= epsTool || ht >= 1-epsTool {
			continue // an end of the arc, or off the sweep
		}
		if ht < pick && ht > loT {
			loT, loP = ht, h
		}
		if ht > pick && ht < hiT {
			hiT, hiP = ht, h
		}
	}

	switch {
	case loP == nil && hiP == nil: // nothing crosses the picked portion: delete the arc
		center, start, end := a.Center, a.Start, a.End
		s.RemoveEntity(a)
		s.RemovePoint(start)
		s.RemovePoint(end)
		s.RemovePoint(center)
		return []*Arc{}
	case loP == nil: // crossing only after the pick: cut the start stub in place
		s.moveEnd(a, a.Start, s.pointOnCutter(*hiP))
		return []*Arc{a}
	case hiP == nil: // crossing only before the pick: cut the end stub in place
		s.moveEnd(a, a.End, s.pointOnCutter(*loP))
		return []*Arc{a}
	}
	// Crossings on both sides: delete the arc and keep the two outer pieces.
	first := s.CreateArc(a.Center, a.Start, s.pointOnCutter(*loP))
	second := s.CreateArc(a.Center, s.pointOnCutter(*hiP), a.End)
	s.RemoveEntity(a)
	return []*Arc{first, second}
}

// TrimCircle removes the portion of c between the two crossings with other
// entities that bound the pick point (x, y), as Fusion's trim of a closed curve
// does: c is deleted and the remaining arc is returned, both of its ends tied to
// their cutters. The radius, diameter, concentric and equal-radius constraints
// that named c are moved to the arc, so their handles stay valid; every other
// constraint naming c is removed with it. With fewer than two distinct
// crossings c is deleted (with its center point when nothing else uses it) and
// an empty slice returned. It returns nil and false, changing nothing, when s is
// nil, when c or its center is not live in s, when c is reference geometry, or
// when the pick lies exactly on a crossing.
func TrimCircle(s *Sketch, c *Circle, x, y float64) ([]*Arc, bool) {
	if s.foreignInput(c) {
		return nil, false // nil, removed, or another sketch's circle
	}
	if c.IsReference() {
		return nil, false // reference geometry is locked; cannot be trimmed
	}
	gc := c.Geometry()
	angle := func(p *geom.Point) float64 {
		v := math.Atan2(p.Y-gc.Center.Y, p.X-gc.Center.X)
		if v < 0 {
			v += 2 * math.Pi
		}
		return v
	}
	hits := s.circleCrossings(gc, c)
	slices.SortFunc(hits, func(p, q crossing) int { return cmp.Compare(angle(p.at), angle(q.at)) })
	// Merge crossings that coincide (two cutters through one point, a tangent).
	var distinct []crossing
	for _, h := range hits {
		if len(distinct) > 0 && (angle(h.at)-angle(distinct[len(distinct)-1].at))*gc.Radius <= epsTool {
			continue
		}
		distinct = append(distinct, h)
	}
	if n := len(distinct); n > 1 && (angle(distinct[0].at)+2*math.Pi-angle(distinct[n-1].at))*gc.Radius <= epsTool {
		distinct = distinct[:n-1] // the last crossing wraps onto the first
	}
	if len(distinct) < 2 {
		center := c.Center
		s.RemoveEntity(c)
		s.RemovePoint(center)
		return []*Arc{}, true
	}
	pick := angle(geom.NewPoint(x, y))
	n := len(distinct)
	for i := range distinct {
		lo, hi := angle(distinct[i].at), angle(distinct[(i+1)%n].at)
		if math.Abs(pick-lo)*gc.Radius <= epsTool {
			return nil, false // the pick is on a crossing: no portion is picked
		}
		inside := lo < pick && pick < hi
		if i == n-1 { // the portion that wraps through angle 0
			inside = pick > lo || pick < hi
		}
		if !inside {
			continue
		}
		// Keep the arc running counter-clockwise from hi round to lo.
		arc := s.CreateArc(c.Center, s.pointOnCutter(distinct[(i+1)%n]), s.pointOnCutter(distinct[i]))
		s.moveCircleConstraints(c, arc)
		s.RemoveEntity(c)
		return []*Arc{arc}, true
	}
	return nil, false
}

// moveCircleConstraints rewrites to arc the operand of every radius, diameter,
// concentric and equal-radius constraint that names c. None of the four owns an
// auxiliary variable, and none depends on which part of the circle survives.
func (s *Sketch) moveCircleConstraints(c *Circle, arc *Arc) {
	for _, k := range s.cons {
		switch t := k.(type) {
		case *Radius:
			if t.C == Circular(c) {
				t.C = arc
			}
		case *Diameter:
			if t.C == Circular(c) {
				t.C = arc
			}
		case *concentric:
			if t.C1 == Circular(c) {
				t.C1 = arc
			}
			if t.C2 == Circular(c) {
				t.C2 = arc
			}
		case *equalRadii:
			if t.C1 == Circular(c) {
				t.C1 = arc
			}
			if t.C2 == Circular(c) {
				t.C2 = arc
			}
		}
	}
}

// circleCrossings returns where the full circle c meets every entity except
// skip, with the entity met: a line cutter within its own segment, a circle, or
// an arc within its sweep.
func (s *Sketch) circleCrossings(c *geom.Circle, skip Entity) []crossing {
	var out []crossing
	for _, e := range s.ents {
		if e == skip {
			continue
		}
		switch t := e.(type) {
		case *Line:
			ol := t.Geometry()
			for _, p := range geom.LineCircleIntersections(ol, c) {
				if _, u := geom.ClosestPointOnLine(ol, p); u >= -epsTool && u <= 1+epsTool {
					out = append(out, crossing{p, t})
				}
			}
		case *Circle:
			for _, p := range geom.CircleCircleIntersections(c, t.Geometry()) {
				out = append(out, crossing{p, t})
			}
		case *Arc:
			for _, p := range geom.CircleArcIntersections(c, t.Geometry()) {
				out = append(out, crossing{p, t})
			}
		}
	}
	return out
}

// replaceEnd points e's end old (its Start or End) at np instead. It is the
// in-place edit Fusion makes when it trims, extends, fillets or chamfers a curve:
// the entity keeps its identity, so every constraint that names it survives.
func replaceEnd(e Entity, old, np *Point) {
	if l, ok := e.(*Line); ok {
		if l.Start == old {
			l.Start = np
		} else {
			l.End = np
		}
		return
	}
	if a, ok := e.(*Arc); ok {
		if a.Start == old {
			a.Start = np
		} else {
			a.End = np
		}
	}
}

// moveEnd moves e's end old to np in place, removes old when no entity uses it
// any more, and refreshes the endpoint tangencies that named old as their contact.
func (s *Sketch) moveEnd(e Entity, old, np *Point) {
	replaceEnd(e, old, np)
	s.RemovePoint(old)
	s.refreshSharedEndpoints()
}

// refreshSharedEndpoints re-derives the shared contact point of every endpoint
// tangency after an in-place end move. A tangency whose operands no longer share
// an end point becomes a tangency anywhere on the two curves: its cached point is
// cleared and allocVars allocates the sweep slack that form needs, so the caller's
// handle stays valid. A conic tangency in that state is removed instead, because
// its other form needs contact-witness variables that only a fresh constraint
// allocates consistently.
func (s *Sketch) refreshSharedEndpoints() {
	var stale []Constraint
	for _, c := range s.cons {
		switch t := c.(type) {
		case *tangentLineCircle:
			if a, ok := t.C.(*Arc); ok && t.shared != nil && sharedPointLineArc(t.L, a) != t.shared {
				t.shared = sharedPointLineArc(t.L, a)
				if t.shared == nil {
					t.allocVars(s)
				}
			}
		case *tangentCircles:
			if t.shared != nil && sharedPointCirculars(t.C1, t.C2) != t.shared {
				t.shared = sharedPointCirculars(t.C1, t.C2)
				if t.shared == nil {
					t.allocVars(s)
				}
			}
		case *tangentLineEllipse:
			if a, ok := t.E.(*EllipticalArc); ok && t.shared != nil && sharedPointLineEllipticalArc(t.L, a) != t.shared {
				t.shared = sharedPointLineEllipticalArc(t.L, a)
				if t.shared == nil {
					t.allocVars(s)
				}
			}
		case *tangentConics:
			if t.shared != nil && sharedConicEndpoint(t.A, t.B) != t.shared {
				stale = append(stale, c)
			}
		}
	}
	for _, c := range stale {
		s.RemoveConstraint(c)
	}
}

// Extend lengthens e from the given endpoint (its Start or End) to its nearest
// crossing with another entity beyond that end, and returns the result, of the
// same type as e. It returns false (changing nothing) when end is neither
// endpoint or no entity lies beyond it; when s is nil; when e is reference
// geometry; and when e, either of its endpoints, or end is not a live handle of
// s (nil, dead, or foreign). As Fusion's extend does, it moves end in place and
// returns e itself: the handle and the constraints that name it survive, and
// the old end point is removed when nothing else uses it. An arc is extended
// along its own circle; an extension that would reach the arc's other end (and
// close it into a circle, which Fusion returns) is refused with false.
func Extend[T OpenCurve](s *Sketch, e T, end *Point) (T, bool) {
	var zero T
	ent := asEntity(e)
	if s.foreignInput(ent) || !s.owns(end) {
		return zero, false // nil, removed, or another sketch's handle
	}
	if ent.IsReference() {
		return zero, false // reference geometry is locked; cannot be extended
	}
	var got Entity
	ok := false
	if l, isLine := ent.(*Line); isLine {
		got, ok = s.extendLine(l, end)
	} else if a, isArc := ent.(*Arc); isArc {
		got, ok = s.extendArc(a, end)
	}
	if !ok {
		return zero, false
	}
	return asCurve[T](got), true
}

// extendArc is Extend for a screened arc: it lengthens a along its own circle
// from end to the nearest crossing beyond it.
func (s *Sketch) extendArc(a *Arc, end *Point) (*Arc, bool) {
	if end != a.Start && end != a.End {
		return nil, false
	}
	ga := a.Geometry()
	sweep := ga.Sweep()
	hits := s.circleCrossings(geom.NewCircle(ga.Center, ga.Radius()), a)

	fromStart := end == a.Start
	bestGap := math.Inf(1)
	var best *crossing
	for i := range hits {
		h := &hits[i]
		phi := ga.AngleFromStart(h.at)
		// The gap is the angle swept beyond the chosen end to reach the hit:
		// clockwise from Start, counter-clockwise from End.
		gap := phi - sweep
		if fromStart {
			gap = 2*math.Pi - phi
		}
		if gap <= epsTool || gap >= 2*math.Pi-sweep-epsTool {
			continue // on the arc itself, or only reachable past the other end
		}
		if gap < bestGap {
			bestGap, best = gap, h
		}
	}
	if best == nil {
		return nil, false
	}
	s.moveEnd(a, end, s.pointOnCutter(*best))
	return a, true
}

// extendLine is Extend for a screened line.
func (s *Sketch) extendLine(l *Line, end *Point) (*Line, bool) {
	if end != l.Start && end != l.End {
		return nil, false
	}
	sl := l.Geometry()
	hits := s.lineCrossings(sl, l, false)

	fromStart := end == l.Start
	bestT := math.Inf(1)
	var best *crossing
	for i := range hits {
		h := &hits[i]
		_, ht := geom.ClosestPointOnLine(sl, h.at)
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
	s.moveEnd(l, end, s.pointOnCutter(*best))
	return l, true
}

// crossing is a point where a trimmed or extended curve meets another entity,
// and that entity (the cutter).
type crossing struct {
	at     *geom.Point
	cutter Entity
}

// pointOnCutter returns the sketch point a trimmed or extended end moves to. A
// crossing on one of the cutter's own end points reuses that point, so the two
// curves share it. Otherwise it creates a point at the crossing and ties it to
// the cutter with a point-on-curve constraint, as Fusion's trim and extend
// create constraints by default.
func (s *Sketch) pointOnCutter(c crossing) *Point {
	for _, p := range cutterEnds(c.cutter) {
		if near(p.Geometry(), c.at) {
			return p
		}
	}
	np := s.CreatePoint(c.at.X, c.at.Y)
	s.pinToCutter(np, c.cutter)
	return np
}

// cutterEnds returns the end points of a line or arc cutter. A circle has none.
func cutterEnds(e Entity) []*Point {
	switch t := e.(type) {
	case *Line:
		return []*Point{t.Start, t.End}
	case *Arc:
		return []*Point{t.Start, t.End}
	}
	return nil
}

// pinToCutter ties p to the cutter it was created on.
func (s *Sketch) pinToCutter(p *Point, cutter Entity) {
	switch t := cutter.(type) {
	case *Line:
		s.AddConstraint(NewPointOnLine(p, t))
	case *Circle:
		s.AddConstraint(NewPointOnCircle(p, t))
	case *Arc:
		s.AddConstraint(NewPointOnArc(p, t))
	}
}

// lineCrossings returns where sl meets every entity except skip, with the
// entity met. When segment is true, line cutters are intersected as bounded
// segments (for trimming); otherwise the infinite line through sl is used and a
// line cutter contributes a point only where it falls within the cutter's own
// segment (for extending). Circle and arc cutters always use the infinite line
// through sl; the caller filters by position along sl.
func (s *Sketch) lineCrossings(sl *geom.Line, skip Entity, segment bool) []crossing {
	var out []crossing
	for _, e := range s.ents {
		if e == skip {
			continue
		}
		switch t := e.(type) {
		case *Line:
			ol := t.Geometry()
			if segment {
				if p, ok := geom.SegmentIntersection(sl, ol); ok {
					out = append(out, crossing{p, t})
				}
				continue
			}
			p, ok := geom.LineLineIntersection(sl, ol)
			if !ok {
				continue
			}
			if _, u := geom.ClosestPointOnLine(ol, p); u >= -epsTool && u <= 1+epsTool {
				out = append(out, crossing{p, t})
			}
		case *Circle:
			for _, p := range geom.LineCircleIntersections(sl, t.Geometry()) {
				out = append(out, crossing{p, t})
			}
		case *Arc:
			for _, p := range geom.LineArcIntersections(sl, t.Geometry()) {
				out = append(out, crossing{p, t})
			}
		}
	}
	return out
}

// near reports whether two generic points share a location within tolerance.
func near(a, b *geom.Point) bool { return math.Hypot(a.X-b.X, a.Y-b.Y) <= epsTool }

// --- fillet / chamfer -------------------------------------------------------

// Fillet groups the geometry created by [CreateFillet]: the rounding arc, the
// two shortened legs that meet it (of the same types as the inputs), the contact
// points T1 (on L1) and T2 (on L2), and the editable radius dimension.
type Fillet[A, B OpenCurve] struct {
	Arc    *Arc
	L1     A
	L2     B
	T1, T2 *Point
	Radius *Distance
}

// CreateFillet rounds the corner where e1 and e2 meet with a tangent arc of
// radius r. corner must be an endpoint of both curves. The corner is removed and
// each leg is shortened to its contact point in place, as Fusion's fillet does:
// [Fillet.L1] and [Fillet.L2] are e1 and e2 themselves, and the constraints that
// name them (horizontal, angle, …) survive. The arc is held tangent to both legs
// and its radius pinned by an editable dimension, so editing [Fillet.Radius] and
// re-solving keeps the rounding tangent. Constraints on the corner point are
// removed with it; constraints on the far endpoints are kept. Lines and arcs are
// handled in any combination: a line leg gets [NewTangent], an arc leg
// [NewTangentCircles] (internal when the fillet lies inside the leg's circle),
// both at the shared contact point. Returns
// [ErrForeignEntity] when s is nil or either curve or corner is not a live
// handle of s (nil, dead, or foreign) or a curve has an endpoint that is not a
// live point of s, [ErrReferenceGeometry], [ErrNoSharedCorner] or
// [ErrFilletInfeasible] — in each case without modifying the sketch.
func CreateFillet[A, B OpenCurve](s *Sketch, e1 A, e2 B, corner *Point, r float64) (*Fillet[A, B], error) {
	ent1, ent2 := asEntity(e1), asEntity(e2)
	if s.foreignInput(ent1, ent2) || !s.owns(corner) {
		return nil, ErrForeignEntity
	}
	if ent1.IsReference() || ent2.IsReference() {
		return nil, ErrReferenceGeometry
	}
	if ent1 == ent2 || !slices.Contains(cutterEnds(ent1), corner) || !slices.Contains(cutterEnds(ent2), corner) {
		return nil, ErrNoSharedCorner
	}
	fg, ok := filletContacts(ent1, ent2, corner, r)
	if !ok {
		return nil, ErrFilletInfeasible
	}

	t1, t2 := s.CreatePoint(fg.p1.X, fg.p1.Y), s.CreatePoint(fg.p2.X, fg.p2.Y)
	ctr := s.CreatePoint(fg.center.X, fg.center.Y)
	replaceEnd(ent1, corner, t1)
	replaceEnd(ent2, corner, t2)
	nArc := s.CreateArc(ctr, t1, t2)
	if !fg.startsAtP1 { // the minor arc runs from the second contact
		nArc = s.CreateArc(ctr, t2, t1)
	}

	s.AddConstraint(tangentToFillet(ent1, nArc, fg.inside1), tangentToFillet(ent2, nArc, fg.inside2))
	rad := NewDistance(nArc.Center, nArc.Start, r)
	s.AddConstraint(rad)

	s.RemovePoint(corner)
	s.refreshSharedEndpoints()

	return &Fillet[A, B]{Arc: nArc, L1: e1, L2: e2, T1: t1, T2: t2, Radius: rad}, nil
}

// filletGeometry is a fillet computed on solved-coordinate copies: the contacts
// on the first and second leg, the center, which contact the minor arc starts
// at, and whether the fillet lies inside each leg's circle.
type filletGeometry struct {
	p1, p2, center   *geom.Point
	startsAtP1       bool
	inside1, inside2 bool
}

// filletContacts runs the geom fillet math for two legs (each a *Line or an
// *Arc) meeting at corner: geom.Fillet for two lines, FilletLineArc or
// FilletArcArc otherwise.
func filletContacts(e1, e2 Entity, corner *Point, r float64) (filletGeometry, bool) {
	cc := geom.NewPoint(corner.x(), corner.y())
	// legLine copies a line leg as the geom helpers expect it: kept end first,
	// the corner (one shared geom point) last.
	legLine := func(l *Line) *geom.Line { return geom.NewLine(otherSketchEnd(l, corner).Geometry(), cc) }
	l1, isLine1 := e1.(*Line)
	l2, isLine2 := e2.(*Line)
	if isLine1 && isLine2 {
		copy1, copy2 := legLine(l1), legLine(l2)
		arc, ok := geom.Fillet(copy1, copy2, r)
		if !ok {
			return filletGeometry{}, false
		}
		c1, c2 := otherEnd(copy1, copy1.Start), otherEnd(copy2, copy2.Start)
		return filletGeometry{p1: c1, p2: c2, center: arc.Center, startsAtP1: arc.Start == c1}, true
	}
	a1, _ := e1.(*Arc)
	a2, _ := e2.(*Arc)
	var cf geom.CornerFillet
	var ok bool
	switch {
	case isLine1:
		cf, ok = geom.FilletLineArc(legLine(l1), a2.Geometry(), a2.Start == corner, r)
	case isLine2:
		cf, ok = geom.FilletLineArc(legLine(l2), a1.Geometry(), a1.Start == corner, r)
		cf.T1, cf.T2, cf.Inside1, cf.Inside2 = cf.T2, cf.T1, cf.Inside2, cf.Inside1
	default:
		cf, ok = geom.FilletArcArc(a1.Geometry(), a1.Start == corner, a2.Geometry(), a2.Start == corner, r)
	}
	if !ok {
		return filletGeometry{}, false
	}
	return filletGeometry{
		p1: cf.T1, p2: cf.T2, center: cf.Arc.Center,
		startsAtP1: cf.Arc.Start == cf.T1, inside1: cf.Inside1, inside2: cf.Inside2,
	}, true
}

// tangentToFillet is the endpoint tangency between a fillet arc and one of its
// legs, which share the contact point.
func tangentToFillet(leg Entity, fillet *Arc, inside bool) Constraint {
	if l, ok := leg.(*Line); ok {
		return NewTangent(l, fillet)
	}
	a, _ := leg.(*Arc)
	return NewTangentCircles(a, fillet, inside)
}

// Chamfer groups the geometry created by [CreateChamfer]: the cut line, the
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
// and each leg shortened to its contact in place, as Fusion's chamfer does:
// [Chamfer.L1] and [Chamfer.L2] are l1 and l2 themselves. The contacts are
// pinned by editable distance dimensions from the far endpoints (D1/D2), so
// editing them and re-solving moves the chamfer parametrically. Constraint
// handling matches [CreateFillet]. Returns [ErrForeignEntity] when s is nil or either line is not
// a live entity of s (nil, dead, or foreign) or has an endpoint that is not a
// live point of s, [ErrReferenceGeometry], [ErrNoSharedCorner] or
// [ErrChamferInfeasible] — in each case without modifying the sketch.
func CreateChamfer(s *Sketch, l1, l2 *Line, d float64) (*Chamfer, error) {
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
	replaceEnd(l1, corner, t1)
	replaceEnd(l2, corner, t2)
	nCut := s.CreateLine(t1, t2)
	if cut.Start != c1 {
		nCut = s.CreateLine(t2, t1)
	}

	d1, d2 := NewDistance(far1, t1, len1-d), NewDistance(far2, t2, len2-d)
	s.AddConstraint(d1, d2)

	s.RemovePoint(corner)
	s.refreshSharedEndpoints()

	return &Chamfer{Cut: nCut, L1: l1, L2: l2, T1: t1, T2: t2, D1: d1, D2: d2}, nil
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

// --- offset -----------------------------------------------------------------

// OffsetGroup groups the geometry created by [CreateOffset]: the source
// entities (left in place), their offset copies (the line and arc copies in
// the order of the source, then the circle copies in order), the per-line
// offset constraints, the per-arc/circle radial offsets, and the constraints
// that hold the copy's ends and tangent joints in place (point-on-normal pins
// and line–arc tangencies). Use [OffsetGroup.Set] to retarget the whole offset
// distance at once and re-solve.
type OffsetGroup struct {
	Source      []Entity
	Copies      []Entity
	Offsets     []*Offset
	Radials     []*RadialOffset
	Constraints []Constraint
	signs       []float64 // per Offset: its target is signs[i]·d
	radialSigns []float64 // per RadialOffset: its target is radialSigns[i]·d
}

// Set retargets the whole offset to distance d (positive to the right of the
// flow, as [CreateOffset] takes it). Call [Sketch.Solve] afterwards to apply it.
func (g *OffsetGroup) Set(d float64) {
	for i, o := range g.Offsets {
		o.Set(g.signs[i] * d)
	}
	for i, r := range g.Radials {
		r.Set(g.radialSigns[i] * d)
	}
}

// CreateOffset creates the offset of a chain of lines and arcs, and of
// circles, at signed distance d, as Fusion's offset does: positive d is to the
// right of the flow, negative to the left. The flow of a chain runs from its
// first member in ents toward the next member in ents that shares an end point
// with it; a lone line or arc flows from Start to End, and a circle flows
// counter-clockwise, so positive d offsets a circle outward.
//
// Each line copy is held parallel by an [Offset] and each arc or circle copy
// shares its source's center and is held at the source radius plus or minus d
// by a [RadialOffset]. Copies that meet at a shared source point share one copy
// point. Where two members meet at an angle, that point is pulled to where the
// two offset curves cross (a mitre). Where they meet tangentially and one of
// them is an arc, the point is held on the arc's normal at the source point
// ([NewPointOnNormal]); the line beside it is held tangent to the arc copy
// instead of by an [Offset], and the second arc of a tangent arc–arc joint
// takes its radius from the joint, so no joint is constrained twice. Each free
// end of a chain is held on the source normal at the source end. The copy is
// thus fully determined by the source and d.
//
// It returns [ErrForeignEntity] if s is nil or a source entity, or a point
// defining one, is not live in s, [ErrReferenceGeometry] for reference
// geometry, [ErrUnsupportedEntity] for an entity other than a line, an arc or a
// circle, and [ErrInvalidShape] if a source line has zero length, an end point
// is shared by three or more members (not a chain), the chain folds back on
// itself at a joint (a cusp), a copy's radius would not be positive, the offset
// curves do not cross near a corner, or a closed chain has only tangent arc–arc
// joints (offset the circle instead) — in each case without modifying the
// sketch.
func CreateOffset(s *Sketch, ents []Entity, d float64) (*OffsetGroup, error) {
	if s.foreignInput(ents...) {
		return nil, ErrForeignEntity
	}
	if containsReference(ents) {
		return nil, ErrReferenceGeometry
	}
	if e, ok := unsupportedOffset(ents); ok {
		return nil, fmt.Errorf("%w: CreateOffset cannot offset %T; only *sketch.Line, *sketch.Arc and *sketch.Circle are offset", ErrUnsupportedEntity, e)
	}
	// Plan everything up front so a failure leaves the sketch unchanged (no
	// partial geometry committed).
	plan, err := planOffset(ents, d)
	if err != nil {
		return nil, err
	}
	g := &OffsetGroup{Source: ents}
	copyOf := map[*Point]*Point{}
	for _, q := range plan.points {
		at := plan.seeds[q]
		copyOf[q] = s.CreatePoint(at.X, at.Y)
	}
	dsts := make([]Entity, len(plan.members))
	for i, m := range plan.members {
		if m.line != nil {
			dsts[i] = s.CreateLine(copyOf[m.line.Start], copyOf[m.line.End])
		} else {
			dsts[i] = s.CreateArc(m.arc.Center, copyOf[m.arc.Start], copyOf[m.arc.End])
		}
		g.Copies = append(g.Copies, dsts[i])
	}
	add := func(c Constraint) {
		s.AddConstraint(c)
		g.Constraints = append(g.Constraints, c)
	}
	for i, m := range plan.members {
		if m.arc != nil {
			if plan.radiusFromJoint[i] {
				continue // a tangent arc–arc joint before it already fixes its radius
			}
			dst, _ := dsts[i].(*Arc)
			ro := NewRadialOffset(m.arc, dst, m.flow*d)
			s.AddConstraint(ro)
			g.Radials = append(g.Radials, ro)
			g.radialSigns = append(g.radialSigns, m.flow)
			continue
		}
		dst, _ := dsts[i].(*Line)
		switch arcs := plan.tangentArcs[i]; len(arcs) {
		case 0:
			// Offset's target is left-positive along the line's own direction;
			// the right of the flow is the left of the line when the flow runs
			// against it.
			off := NewOffset(m.line, dst, -m.flow*d)
			s.AddConstraint(off)
			g.Offsets = append(g.Offsets, off)
			g.signs = append(g.signs, -m.flow)
		case 1:
			arcDst, _ := dsts[arcs[0]].(*Arc)
			add(NewTangent(dst, arcDst))
		}
	}
	for _, pin := range plan.pins {
		add(plan.members[pin.member].pin(copyOf[pin.at], pin.at))
	}
	s.offsetCircles(g, ents, d)
	return g, nil
}

// offsetMember is one line or arc of an offset chain and its flow: +1 when the
// flow runs along the member's own Start→End direction, -1 against it.
type offsetMember struct {
	e    Entity
	line *Line
	arc  *Arc
	flow float64
}

// pin holds cp on the member's normal at its source point at.
func (m offsetMember) pin(cp, at *Point) Constraint {
	if m.line != nil {
		return NewPointOnNormal(cp, m.line, at)
	}
	return NewPointOnNormal(cp, m.arc, at)
}

func (m offsetMember) ends() [2]*Point {
	if m.line != nil {
		return [2]*Point{m.line.Start, m.line.End}
	}
	return [2]*Point{m.arc.Start, m.arc.End}
}

// flowStart and flowEnd are the end points the flow enters and leaves by.
func (m offsetMember) flowStart() *Point {
	e := m.ends()
	if m.flow > 0 {
		return e[0]
	}
	return e[1]
}

func (m offsetMember) flowEnd() *Point {
	e := m.ends()
	if m.flow > 0 {
		return e[1]
	}
	return e[0]
}

// tangent returns the unit tangent at q (a point of the member) along the flow.
func (m offsetMember) tangent(q *Point) (float64, float64) {
	if m.line != nil {
		dx, dy := m.line.End.x()-m.line.Start.x(), m.line.End.y()-m.line.Start.y()
		n := math.Hypot(dx, dy)
		return m.flow * dx / n, m.flow * dy / n
	}
	rx, ry := q.x()-m.arc.Center.x(), q.y()-m.arc.Center.y()
	r := math.Hypot(rx, ry)
	return m.flow * -ry / r, m.flow * rx / r // counter-clockwise tangent, along the flow
}

// carrier returns the member's offset curve at distance d to the right of the
// flow: a line shifted sideways, or a concentric circle.
func (m offsetMember) carrier(d float64) (*geom.Line, *geom.Circle) {
	if m.line != nil {
		tx, ty := m.tangent(m.line.Start)
		nx, ny := ty*d, -tx*d // right of the flow
		return geom.NewLine(geom.NewPoint(m.line.Start.x()+nx, m.line.Start.y()+ny),
			geom.NewPoint(m.line.End.x()+nx, m.line.End.y()+ny)), nil
	}
	return nil, geom.NewCircle(m.arc.Center.Geometry(), m.arc.R()+m.flow*d)
}

// offsetPin holds the copy of the source point at on the normal of member at.
type offsetPin struct {
	member int
	at     *Point
}

// offsetPlan is everything CreateOffset commits, computed without touching the
// sketch.
type offsetPlan struct {
	members         []offsetMember
	points          []*Point               // source end points, in first-seen order
	seeds           map[*Point]*geom.Point // where each copy point starts
	tangentArcs     [][]int                // per line member: the arc members it meets tangentially
	radiusFromJoint []bool                 // per arc member: its radius comes from a tangent arc–arc joint
	pins            []offsetPin
}

// planOffset validates the chain members of ents and plans their copies.
func planOffset(ents []Entity, d float64) (*offsetPlan, error) {
	p := &offsetPlan{seeds: map[*Point]*geom.Point{}}
	for _, e := range ents {
		if l, ok := e.(*Line); ok {
			if math.Hypot(l.End.x()-l.Start.x(), l.End.y()-l.Start.y()) == 0 {
				return nil, fmt.Errorf("%w: CreateOffset source line has zero length", ErrInvalidShape)
			}
			p.members = append(p.members, offsetMember{e: e, line: l})
		}
		if a, ok := e.(*Arc); ok {
			p.members = append(p.members, offsetMember{e: e, arc: a})
		}
		if c, ok := e.(*Circle); ok && c.r()+d <= 0 {
			return nil, fmt.Errorf("%w: CreateOffset: the offset circle's radius would be %g", ErrInvalidShape, c.r()+d)
		}
	}
	ends := make([][2]*Point, len(p.members))
	for i, m := range p.members {
		ends[i] = m.ends()
	}
	flow, err := offsetFlow(ends)
	if err != nil {
		return nil, err
	}
	at := map[*Point][]int{}
	for i := range p.members {
		p.members[i].flow = flow[i]
		for _, q := range ends[i] {
			if len(at[q]) == 0 {
				p.points = append(p.points, q)
			}
			at[q] = append(at[q], i)
		}
	}
	for _, m := range p.members {
		if m.arc != nil && m.arc.R()+m.flow*d <= 0 {
			return nil, fmt.Errorf("%w: CreateOffset: the offset arc's radius would be %g", ErrInvalidShape, m.arc.R()+m.flow*d)
		}
	}
	p.tangentArcs = make([][]int, len(p.members))
	p.radiusFromJoint = make([]bool, len(p.members))
	for _, q := range p.points {
		ms := at[q]
		if len(ms) == 1 { // a free end: on the source normal
			m := p.members[ms[0]]
			tx, ty := m.tangent(q)
			p.seeds[q] = geom.NewPoint(q.x()+ty*d, q.y()-tx*d)
			p.pins = append(p.pins, offsetPin{member: ms[0], at: q})
			continue
		}
		in, out := ms[0], ms[1] // in arrives at q, out leaves it
		if p.members[in].flowEnd() != q {
			in, out = out, in
		}
		a, b := p.members[in], p.members[out]
		ax, ay := a.tangent(q)
		bx, by := b.tangent(q)
		cross, dot := ax*by-ay*bx, ax*bx+ay*by
		if math.Abs(cross) <= 1e-6 {
			if dot < 0 {
				return nil, fmt.Errorf("%w: CreateOffset: the chain folds back on itself at (%g, %g)", ErrInvalidShape, q.x(), q.y())
			}
			// Tangent joint: both offset curves touch at the shared normal.
			p.seeds[q] = geom.NewPoint(q.x()+ay*d, q.y()-ax*d)
			switch {
			case a.arc != nil && b.arc != nil:
				p.radiusFromJoint[out] = true
				p.pins = append(p.pins, offsetPin{member: in, at: q})
			case a.arc != nil:
				p.tangentArcs[out] = append(p.tangentArcs[out], in)
				p.pins = append(p.pins, offsetPin{member: in, at: q})
			case b.arc != nil:
				p.tangentArcs[in] = append(p.tangentArcs[in], out)
				p.pins = append(p.pins, offsetPin{member: out, at: q})
			}
			continue
		}
		seed, ok := mitre(a, b, q, d)
		if !ok {
			return nil, fmt.Errorf("%w: CreateOffset: the offset curves do not cross near (%g, %g)", ErrInvalidShape, q.x(), q.y())
		}
		p.seeds[q] = seed
	}
	if allTangentArcLoop(p, at) {
		return nil, fmt.Errorf("%w: CreateOffset: a closed chain of arcs joined only tangentially has no free radius; offset the circle instead", ErrInvalidShape)
	}
	return p, nil
}

// mitre returns where the offset curves of a (arriving) and b (leaving) cross,
// nearest the point a's offset reaches at the source joint q.
func mitre(a, b offsetMember, q *Point, d float64) (*geom.Point, bool) {
	ax, ay := a.tangent(q)
	near := geom.NewPoint(q.x()+ay*d, q.y()-ax*d)
	la, ca := a.carrier(d)
	lb, cb := b.carrier(d)
	var hits []*geom.Point
	switch {
	case la != nil && lb != nil:
		if h, ok := geom.LineLineIntersection(la, lb); ok {
			hits = []*geom.Point{h}
		}
	case la != nil:
		hits = geom.LineCircleIntersections(la, cb)
	case lb != nil:
		hits = geom.LineCircleIntersections(lb, ca)
	default:
		hits = geom.CircleCircleIntersections(ca, cb)
	}
	var best *geom.Point
	for _, h := range hits {
		if best == nil || math.Hypot(h.X-near.X, h.Y-near.Y) < math.Hypot(best.X-near.X, best.Y-near.Y) {
			best = h
		}
	}
	return best, best != nil
}

// allTangentArcLoop reports a closed chain whose every joint is a tangent
// arc–arc joint. Around such a loop the arc radii fix one another, so one of
// the arcs' internal radius rows is always dependent.
func allTangentArcLoop(p *offsetPlan, at map[*Point][]int) bool {
	seen := map[int]bool{}
	for start := range p.members {
		if seen[start] {
			continue
		}
		closed, allTangent := true, true
		stack := []int{start}
		seen[start] = true
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if p.members[i].arc == nil || !p.radiusFromJoint[i] {
				allTangent = false
			}
			for _, q := range p.members[i].ends() {
				if len(at[q]) < 2 {
					closed = false
				}
				for _, j := range at[q] {
					if !seen[j] {
						seen[j] = true
						stack = append(stack, j)
					}
				}
			}
		}
		if closed && allTangent {
			return true
		}
	}
	return false
}

// unsupportedOffset reports the first entity in ents CreateOffset cannot
// offset. Like unsupportedSeed it is an allow-list with a refusing default, so
// an entity type added later is refused rather than silently skipped.
func unsupportedOffset(ents []Entity) (Entity, bool) {
	for _, e := range ents {
		switch e.(type) {
		case *Line, *Arc, *Circle:
		default:
			return e, true
		}
	}
	return nil, false
}

// offsetCircles adds the copies of the circles in ents: each shares its
// source's center and a RadialOffset holds its radius at the source radius plus
// d (a circle flows counter-clockwise, so positive d is outward).
func (s *Sketch) offsetCircles(g *OffsetGroup, ents []Entity, d float64) {
	for _, e := range ents {
		c, ok := e.(*Circle)
		if !ok {
			continue
		}
		dst := s.CreateCircle(c.Center, c.r()+d)
		ro := NewRadialOffset(c, dst, d)
		s.AddConstraint(ro)
		g.Copies = append(g.Copies, dst)
		g.Radials = append(g.Radials, ro)
		g.radialSigns = append(g.radialSigns, 1)
	}
}

// offsetFlow returns, per member (given by its two end points), +1 when the
// offset flow runs along the member's own Start→End direction and -1 when it
// runs against it. Members sharing end points form a chain. A chain's flow runs
// from its first member (in the given order) toward the next member in that
// order that shares a point with it, as Fusion's offset flows from curve 1 to
// curve 2; a lone member flows from Start to End. An end point shared by three
// or more members is not a chain: [ErrInvalidShape].
func offsetFlow(ends [][2]*Point) ([]float64, error) {
	at := map[*Point][]int{}
	for i, e := range ends {
		at[e[0]] = append(at[e[0]], i)
		at[e[1]] = append(at[e[1]], i)
	}
	for _, e := range ends {
		for _, p := range e {
			if len(at[p]) > 2 {
				return nil, fmt.Errorf("%w: CreateOffset: an end point is shared by %d members, which is not a chain", ErrInvalidShape, len(at[p]))
			}
		}
	}
	// other returns the member other than i that uses p, or -1.
	other := func(i int, p *Point) int {
		for _, j := range at[p] {
			if j != i {
				return j
			}
		}
		return -1
	}
	flow := make([]float64, len(ends))
	// walk signs the members reached from member `from` through point p: forward
	// means the flow enters the next member at p, backward that it leaves it there.
	walk := func(from int, p *Point, forward bool) {
		for {
			j := other(from, p)
			if j < 0 || flow[j] != 0 {
				return
			}
			start, end := ends[j][0], ends[j][1]
			if (forward && start == p) || (!forward && end == p) {
				flow[j] = 1
			} else {
				flow[j] = -1
			}
			if start == p {
				p = end
			} else {
				p = start
			}
			from = j
		}
	}
	for i, e := range ends {
		if flow[i] != 0 {
			continue // already reached from an earlier member of its chain
		}
		toStart, toEnd := other(i, e[0]), other(i, e[1])
		flow[i] = 1
		if toStart >= 0 && (toEnd < 0 || toStart < toEnd) {
			flow[i] = -1 // the earliest neighbour is at Start: the flow leaves through Start
		}
		exit, entry := e[1], e[0]
		if flow[i] < 0 {
			exit, entry = e[0], e[1]
		}
		walk(i, exit, true)
		walk(i, entry, false)
	}
	return flow, nil
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
