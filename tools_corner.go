package sketch

import (
	"math"
	"slices"

	"github.com/lestrrat-3d/sketch/geom"
)

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
