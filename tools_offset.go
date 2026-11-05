package sketch

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
)

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
