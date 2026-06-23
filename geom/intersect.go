package geom

import "math"

// epsIntersect guards near-parallel and near-tangent degeneracies in the
// intersection helpers. It is applied relative to the magnitudes involved.
const epsIntersect = 1e-9

// lineLineParams solves the two infinite lines through l1 and l2 for the
// parameters t (along l1) and u (along l2) of their crossing, returning ok=false
// when they are parallel (or a line is degenerate). The crossing point is
// l1.Start + t·(l1.End−l1.Start).
func lineLineParams(l1, l2 *Line) (t, u float64, ok bool) {
	x1, y1 := l1.Start.X, l1.Start.Y
	d1x, d1y := l1.End.X-x1, l1.End.Y-y1
	x2, y2 := l2.Start.X, l2.Start.Y
	d2x, d2y := l2.End.X-x2, l2.End.Y-y2
	den := d1x*d2y - d1y*d2x
	if math.Abs(den) <= epsIntersect*math.Hypot(d1x, d1y)*math.Hypot(d2x, d2y) {
		return 0, 0, false
	}
	t = ((x2-x1)*d2y - (y2-y1)*d2x) / den
	u = ((x2-x1)*d1y - (y2-y1)*d1x) / den
	return t, u, true
}

// LineLineIntersection returns the intersection of the two infinite lines
// through l1 and l2, or false when they are parallel (or a line is
// degenerate).
func LineLineIntersection(l1, l2 *Line) (*Point, bool) {
	t, _, ok := lineLineParams(l1, l2)
	if !ok {
		return nil, false
	}
	return NewPoint(l1.Start.X+t*(l1.End.X-l1.Start.X), l1.Start.Y+t*(l1.End.Y-l1.Start.Y)), true
}

// SegmentIntersection returns the intersection of the two segments, or false
// when they are parallel or the infinite-line intersection falls outside
// either segment. Endpoints count as intersecting.
func SegmentIntersection(l1, l2 *Line) (*Point, bool) {
	t, u, ok := lineLineParams(l1, l2)
	if !ok {
		return nil, false
	}
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return nil, false
	}
	return NewPoint(l1.Start.X+t*(l1.End.X-l1.Start.X), l1.Start.Y+t*(l1.End.Y-l1.Start.Y)), true
}

// ClosestPointOnLine returns the foot of the perpendicular from p to the
// infinite line through l, together with the parameter t locating that foot as
// l.Start + t·(l.End − l.Start). t ∈ [0, 1] means the foot lies within the
// segment; outside that range it lies on the extension beyond an endpoint. A
// degenerate (zero-length) line yields l.Start and t = 0.
func ClosestPointOnLine(l *Line, p *Point) (*Point, float64) {
	dx, dy := l.End.X-l.Start.X, l.End.Y-l.Start.Y
	dd := dx*dx + dy*dy
	if dd == 0 {
		return NewPoint(l.Start.X, l.Start.Y), 0
	}
	t := ((p.X-l.Start.X)*dx + (p.Y-l.Start.Y)*dy) / dd
	return NewPoint(l.Start.X+t*dx, l.Start.Y+t*dy), t
}

// LineCircleIntersections returns the points where the infinite line through
// l meets c: two for a secant, one for a tangent, none for a miss.
func LineCircleIntersections(l *Line, c *Circle) []*Point {
	dx, dy := l.End.X-l.Start.X, l.End.Y-l.Start.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		return nil
	}
	ux, uy := dx/length, dy/length
	// Project the center onto the line: foot = start + t·u.
	t := (c.Center.X-l.Start.X)*ux + (c.Center.Y-l.Start.Y)*uy
	fx, fy := l.Start.X+t*ux, l.Start.Y+t*uy
	d := math.Hypot(c.Center.X-fx, c.Center.Y-fy)
	switch {
	case d > c.Radius+epsIntersect*math.Max(1, c.Radius):
		return nil
	case d >= c.Radius-epsIntersect*math.Max(1, c.Radius):
		return []*Point{NewPoint(fx, fy)} // tangent (within tolerance)
	}
	h := math.Sqrt(c.Radius*c.Radius - d*d)
	return []*Point{
		NewPoint(fx-h*ux, fy-h*uy),
		NewPoint(fx+h*ux, fy+h*uy),
	}
}

// CircleCircleIntersections returns the points where the two circles meet:
// two for crossing circles, one for (internal or external) tangency, none
// when they are separate or one contains the other.
func CircleCircleIntersections(c1, c2 *Circle) []*Point {
	dx, dy := c2.Center.X-c1.Center.X, c2.Center.Y-c1.Center.Y
	d := math.Hypot(dx, dy)
	scale := math.Max(1, math.Max(c1.Radius, c2.Radius))
	if d <= epsIntersect*scale { // concentric (or identical) circles
		return nil
	}
	sum, diff := c1.Radius+c2.Radius, math.Abs(c1.Radius-c2.Radius)
	if d > sum+epsIntersect*scale || d < diff-epsIntersect*scale {
		return nil
	}
	ux, uy := dx/d, dy/d
	// a = distance from c1's center to the chord's foot along the axis.
	a := (d*d + c1.Radius*c1.Radius - c2.Radius*c2.Radius) / (2 * d)
	fx, fy := c1.Center.X+a*ux, c1.Center.Y+a*uy
	h2 := c1.Radius*c1.Radius - a*a
	if h2 <= epsIntersect*scale*scale {
		return []*Point{NewPoint(fx, fy)} // tangent (within tolerance)
	}
	h := math.Sqrt(h2)
	// Chord direction: the axis' left normal.
	return []*Point{
		NewPoint(fx-h*uy, fy+h*ux),
		NewPoint(fx+h*uy, fy-h*ux),
	}
}

// Contains reports whether p's angle about the arc's center lies within the
// arc's counter-clockwise sweep (endpoints inclusive, with a small angular
// tolerance). The point is not required to lie on the arc's circle.
func (a *Arc) Contains(p *Point) bool {
	ang := math.Atan2(p.Y-a.Center.Y, p.X-a.Center.X)
	d := math.Mod(ang-a.StartAngle(), 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	return d <= a.Sweep()+epsIntersect
}

// circle returns the arc's full circle, used to reduce arc intersections to
// circle intersections plus sweep filtering.
func (a *Arc) circle() *Circle { return &Circle{Center: a.Center, Radius: a.Radius()} }

// LineArcIntersections returns the points where the infinite line through l
// meets the arc — the line/circle intersections that fall within the arc's
// sweep.
func LineArcIntersections(l *Line, a *Arc) []*Point {
	return filterByArc(LineCircleIntersections(l, a.circle()), a)
}

// CircleArcIntersections returns the points where the circle meets the arc.
func CircleArcIntersections(c *Circle, a *Arc) []*Point {
	return filterByArc(CircleCircleIntersections(c, a.circle()), a)
}

// ArcArcIntersections returns the points where the two arcs meet — the
// circle/circle intersections that fall within both sweeps.
func ArcArcIntersections(a1, a2 *Arc) []*Point {
	return filterByArc(filterByArc(CircleCircleIntersections(a1.circle(), a2.circle()), a1), a2)
}

func filterByArc(pts []*Point, a *Arc) []*Point {
	var out []*Point
	for _, p := range pts {
		if a.Contains(p) {
			out = append(out, p)
		}
	}
	return out
}
