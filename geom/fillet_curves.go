package geom

import "math"

// CornerFillet is the fillet arc that rounds the corner where two curves meet,
// as computed by [FilletLineArc] and [FilletArcArc]. Neither function changes its
// inputs: the caller shortens the curves to T1 and T2.
type CornerFillet struct {
	// Arc is the minor fillet arc. Its Start and End are T1 and T2 themselves
	// (pointer-equal), in counter-clockwise order.
	Arc *Arc
	// T1 and T2 are the contact points on the first and the second curve.
	T1, T2 *Point
	// Inside1 and Inside2 report whether the fillet circle lies inside the first
	// and the second curve's circle (internal tangency). They are always false
	// for a line.
	Inside1, Inside2 bool
}

// filletLeg is one curve of a filleted corner: a line whose End is the corner,
// or an arc whose Start (atStart) or End is the corner.
type filletLeg struct {
	line    *Line
	arc     *Arc
	atStart bool
}

func (g filletLeg) corner() *Point {
	switch {
	case g.line != nil:
		return g.line.End
	case g.atStart:
		return g.arc.Start
	default:
		return g.arc.End
	}
}

// degenerate reports a zero-length line or a zero-radius arc.
func (g filletLeg) degenerate() bool {
	if g.line != nil {
		return g.line.Length() == 0
	}
	return g.arc.Radius() == 0
}

// into returns the unit tangent at the corner pointing into the part of the leg
// that survives the fillet.
func (g filletLeg) into() (float64, float64) {
	c := g.corner()
	if g.line != nil {
		dx, dy := g.line.Start.X-c.X, g.line.Start.Y-c.Y
		n := math.Hypot(dx, dy)
		return dx / n, dy / n
	}
	r := g.arc.Radius()
	tx, ty := -(c.Y-g.arc.Center.Y)/r, (c.X-g.arc.Center.X)/r // counter-clockwise tangent
	if g.atStart {
		return tx, ty
	}
	return -tx, -ty
}

// carrier is a curve every point of which lies at the fillet radius from a leg:
// a line parallel to a line leg, or a circle concentric with an arc leg.
type carrier struct {
	line   *Line
	circle *Circle
	inside bool
}

func (g filletLeg) carriers(r float64) []carrier {
	if g.line != nil {
		dx, dy := g.line.End.X-g.line.Start.X, g.line.End.Y-g.line.Start.Y
		n := math.Hypot(dx, dy)
		nx, ny := -dy/n*r, dx/n*r
		shift := func(sx, sy float64) carrier {
			return carrier{line: NewLine(NewPoint(g.line.Start.X+sx, g.line.Start.Y+sy), NewPoint(g.line.End.X+sx, g.line.End.Y+sy))}
		}
		return []carrier{shift(nx, ny), shift(-nx, -ny)}
	}
	R := g.arc.Radius()
	out := []carrier{{circle: NewCircle(g.arc.Center, R+r)}}
	if R > r {
		out = append(out, carrier{circle: NewCircle(g.arc.Center, R-r), inside: true})
	}
	return out
}

// contact returns where a fillet circle centered at o touches the leg, and
// whether that point lies strictly inside the part of the leg the fillet keeps.
func (g filletLeg) contact(o *Point) (*Point, bool) {
	if g.line != nil {
		p, t := ClosestPointOnLine(g.line, o)
		return p, t > epsIntersect && t < 1-epsIntersect
	}
	a := g.arc
	d := math.Hypot(o.X-a.Center.X, o.Y-a.Center.Y)
	if d == 0 {
		return nil, false
	}
	R := a.Radius()
	p := NewPoint(a.Center.X+R*(o.X-a.Center.X)/d, a.Center.Y+R*(o.Y-a.Center.Y)/d)
	phi, sweep := a.AngleFromStart(p), a.Sweep()
	return p, phi > epsIntersect && phi < sweep-epsIntersect
}

// meet returns the points where two carriers cross.
func meet(c1, c2 carrier) []*Point {
	switch {
	case c1.line != nil && c2.line != nil:
		if p, ok := LineLineIntersection(c1.line, c2.line); ok {
			return []*Point{p}
		}
		return nil
	case c1.line != nil:
		return LineCircleIntersections(c1.line, c2.circle)
	case c2.line != nil:
		return LineCircleIntersections(c2.line, c1.circle)
	default:
		return CircleCircleIntersections(c1.circle, c2.circle)
	}
}

// filletCorner computes the fillet of radius r between two legs meeting at a
// corner. See [FilletLineArc] for the rules.
func filletCorner(g1, g2 filletLeg, r float64) (CornerFillet, bool) {
	if r <= 0 || g1.degenerate() || g2.degenerate() {
		return CornerFillet{}, false
	}
	c1, c2 := g1.corner(), g2.corner()
	if math.Hypot(c1.X-c2.X, c1.Y-c2.Y) > epsIntersect*math.Max(1, math.Hypot(c1.X, c1.Y)) {
		return CornerFillet{}, false // the curves do not meet at one corner
	}
	u1x, u1y := g1.into()
	u2x, u2y := g2.into()
	if math.Abs(u1x*u2y-u1y*u2x) <= epsIntersect {
		return CornerFillet{}, false // tangent at the corner: nothing to round
	}
	best := CornerFillet{}
	bestReach := math.Inf(1)
	for _, k1 := range g1.carriers(r) {
		for _, k2 := range g2.carriers(r) {
			for _, o := range meet(k1, k2) {
				t1, ok1 := g1.contact(o)
				t2, ok2 := g2.contact(o)
				if !ok1 || !ok2 {
					continue
				}
				reach := math.Max(math.Hypot(t1.X-c1.X, t1.Y-c1.Y), math.Hypot(t2.X-c1.X, t2.Y-c1.Y))
				if reach >= bestReach {
					continue
				}
				arc := NewArc(o, t1, t2)
				if arc.Sweep() > math.Pi {
					arc = NewArc(o, t2, t1)
				}
				best = CornerFillet{Arc: arc, T1: t1, T2: t2, Inside1: k1.inside, Inside2: k2.inside}
				bestReach = reach
			}
		}
	}
	return best, best.Arc != nil
}

// FilletLineArc computes the fillet arc of radius r that rounds the corner
// where line l and arc a meet. l.End is the corner and l.Start the end that is
// kept; atStart says whether the corner is a.Start (otherwise a.End). The
// fillet center is r from both curves; among the centers whose contacts lie
// strictly inside the parts of l and a the fillet keeps, the one whose farther
// contact is nearest the corner is chosen, so the fillet removes the least. It
// returns false when r is not positive, a curve is degenerate, the two corner
// coordinates differ, the curves are tangent at the corner, or no fillet fits.
func FilletLineArc(l *Line, a *Arc, atStart bool, r float64) (CornerFillet, bool) {
	return filletCorner(filletLeg{line: l}, filletLeg{arc: a, atStart: atStart}, r)
}

// FilletArcArc is [FilletLineArc] for two arcs: atStart1 and atStart2 say
// whether the corner is the Start of a1 and of a2 (otherwise their End).
func FilletArcArc(a1 *Arc, atStart1 bool, a2 *Arc, atStart2 bool, r float64) (CornerFillet, bool) {
	return filletCorner(filletLeg{arc: a1, atStart: atStart1}, filletLeg{arc: a2, atStart: atStart2}, r)
}
