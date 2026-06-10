package geom

import "math"

// Modification helpers operate on generic (template) geometry, before it is
// committed to a sketch. They are the math layer under sketcher tools like
// trim, fillet and chamfer: build or adjust templates here, then commit the
// result with the sketch's Add… methods (adding shape-holding constraints as
// needed). They never touch a sketch — committed sketch geometry cannot be
// re-topologized today (entities cannot be removed), so these helpers are the
// supported way to shape geometry first.

// SplitLineAt splits l at p — assumed to lie on l — returning the two
// segments (l.Start→p and p→l.End) sharing p. l itself is not modified.
func SplitLineAt(l *Line, p *Point) (*Line, *Line) {
	return NewLine(l.Start, p), NewLine(p, l.End)
}

// sharedCorner identifies the endpoint pointer shared by both lines and the
// far endpoints of each. Lines must share the *Point itself, not merely have
// coincident coordinates.
func sharedCorner(l1, l2 *Line) (*Point, *Point, *Point, bool) {
	switch {
	case l1.Start == l2.Start:
		return l1.Start, l1.End, l2.End, true
	case l1.Start == l2.End:
		return l1.Start, l1.End, l2.Start, true
	case l1.End == l2.Start:
		return l1.End, l1.Start, l2.End, true
	case l1.End == l2.End:
		return l1.End, l1.Start, l2.Start, true
	}
	return nil, nil, nil, false
}

// replaceEndpoint swaps whichever endpoint of l is old for new.
func replaceEndpoint(l *Line, old, niu *Point) {
	if l.Start == old {
		l.Start = niu
		return
	}
	l.End = niu
}

// cornerInfo is the contact geometry shared by Fillet and Chamfer: the corner
// point, unit directions from the corner toward each far endpoint, the
// available leg lengths, and the corner half-angle.
type cornerInfo struct {
	c          *Point
	u1x, u1y   float64
	u2x, u2y   float64
	len1, len2 float64
	half       float64
}

// corner analyzes the corner shared by l1 and l2. ok is false when the lines
// do not share an endpoint pointer, a leg is degenerate, or the lines are
// collinear.
func corner(l1, l2 *Line) (cornerInfo, bool) {
	c, a, b, ok := sharedCorner(l1, l2)
	if !ok {
		return cornerInfo{}, false
	}
	len1 := math.Hypot(a.X-c.X, a.Y-c.Y)
	len2 := math.Hypot(b.X-c.X, b.Y-c.Y)
	if len1 == 0 || len2 == 0 {
		return cornerInfo{}, false
	}
	ci := cornerInfo{
		c:    c,
		u1x:  (a.X - c.X) / len1,
		u1y:  (a.Y - c.Y) / len1,
		u2x:  (b.X - c.X) / len2,
		u2y:  (b.Y - c.Y) / len2,
		len1: len1,
		len2: len2,
	}
	cross := ci.u1x*ci.u2y - ci.u1y*ci.u2x
	if math.Abs(cross) <= epsIntersect { // collinear corner: nothing to round
		return cornerInfo{}, false
	}
	dot := ci.u1x*ci.u2x + ci.u1y*ci.u2y
	ci.half = math.Acos(math.Max(-1, math.Min(1, dot))) / 2
	return ci, true
}

// Fillet rounds the corner where l1 and l2 share an endpoint with a tangent
// arc of radius r. Each line's shared endpoint is replaced by a fresh point
// at the arc's contact location (the original corner point is left dangling
// and the lines no longer touch); the returned arc connects the two contact
// points, sweeping counter-clockwise across the corner. ok is false when the
// lines do not share an endpoint pointer, are collinear, r is not positive,
// or r is too large for either leg.
func Fillet(l1, l2 *Line, r float64) (*Arc, bool) {
	if r <= 0 {
		return nil, false
	}
	ci, ok := corner(l1, l2)
	if !ok {
		return nil, false
	}
	t := r / math.Tan(ci.half) // contact distance from the corner along each leg
	if t > ci.len1 || t > ci.len2 {
		return nil, false
	}
	// Center sits on the angle bisector, r/sin(θ/2) from the corner.
	bx, by := ci.u1x+ci.u2x, ci.u1y+ci.u2y
	bn := math.Hypot(bx, by)
	ox := ci.c.X + (bx/bn)*(r/math.Sin(ci.half))
	oy := ci.c.Y + (by/bn)*(r/math.Sin(ci.half))

	t1 := NewPoint(ci.c.X+t*ci.u1x, ci.c.Y+t*ci.u1y)
	t2 := NewPoint(ci.c.X+t*ci.u2x, ci.c.Y+t*ci.u2y)
	replaceEndpoint(l1, ci.c, t1)
	replaceEndpoint(l2, ci.c, t2)

	center := NewPoint(ox, oy)
	arc := NewArc(center, t1, t2)
	if arc.Sweep() > math.Pi { // take the minor arc across the corner
		arc = NewArc(center, t2, t1)
	}
	return arc, true
}

// Chamfer cuts the corner where l1 and l2 share an endpoint with a straight
// line whose contact points sit d along each leg from the corner. Each line's
// shared endpoint is replaced by a fresh contact point (the original corner
// point is left dangling); the returned line connects the contacts. ok is
// false when the lines do not share an endpoint pointer, are collinear, d is
// not positive, or d exceeds either leg.
func Chamfer(l1, l2 *Line, d float64) (*Line, bool) {
	if d <= 0 {
		return nil, false
	}
	ci, ok := corner(l1, l2)
	if !ok {
		return nil, false
	}
	if d > ci.len1 || d > ci.len2 {
		return nil, false
	}
	t1 := NewPoint(ci.c.X+d*ci.u1x, ci.c.Y+d*ci.u1y)
	t2 := NewPoint(ci.c.X+d*ci.u2x, ci.c.Y+d*ci.u2y)
	replaceEndpoint(l1, ci.c, t1)
	replaceEndpoint(l2, ci.c, t2)
	return NewLine(t1, t2), true
}
