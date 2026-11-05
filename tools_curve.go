package sketch

import (
	"cmp"
	"math"
	"slices"

	"github.com/lestrrat-3d/sketch/geom"
)

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
