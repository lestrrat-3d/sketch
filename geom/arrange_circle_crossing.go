package geom

import "math"

// LineCircleCrossing identifies one unmerged analytic secant in one arrangement.
// Regions creates it; both incident source edges carry the same pointer.
type LineCircleCrossing struct {
	sources   [2]int // line, then circle
	point     [2]float64
	branch    int8 // -1 low, +1 high on the line's natural parameter
	allBounds [][4]float64
	arc       *LineArcCrossing
}

// LineArcCrossing identifies one unmerged analytic line/arc secant. The
// arrangement retains one shared private token across every incident fragment.
type LineArcCrossing struct{ core *LineCircleCrossing }

func (c *LineArcCrossing) SourceIndices() [2]int { return c.core.SourceIndices() }
func (c *LineArcCrossing) Point() [2]float64     { return c.core.Point() }
func (c *LineArcCrossing) Branch() int8          { return c.core.Branch() }
func (c *LineArcCrossing) SourceCount() int      { return c.core.SourceCount() }
func (c *LineArcCrossing) SourceBound(index int) ([4]float64, bool) {
	return c.core.SourceBound(index)
}

// SourceIndices returns the line and circle input positions, in that order.
func (c *LineCircleCrossing) SourceIndices() [2]int { return c.sources }

// Point returns the arrangement's rounded contact point.
func (c *LineCircleCrossing) Point() [2]float64 { return c.point }

// Branch returns -1 for the low line root or +1 for the high line root.
func (c *LineCircleCrossing) Branch() int8 { return c.branch }

// SourceCount returns the number of full-carrier boxes in this arrangement.
// Callers bound and check cancellation during any traversal they perform.
func (c *LineCircleCrossing) SourceCount() int { return len(c.allBounds) }

// SourceBound returns one full-carrier box by value. It does not expose the
// shared arrangement slice for mutation or copy an unbounded source list.
func (c *LineCircleCrossing) SourceBound(index int) ([4]float64, bool) {
	if index < 0 || index >= len(c.allBounds) {
		return [4]float64{}, false
	}
	return c.allBounds[index], true
}

func (a *arranger) lineCircleCrossing(i, j int, e xEvent) *LineCircleCrossing {
	if e.circleBranch == 0 || !a.exactAllowed {
		return nil
	}
	line, circle := i, j
	if a.sources[j].kind == srcLine {
		line, circle = j, i
	}
	if a.sources[line].kind != srcLine ||
		(a.sources[circle].kind != srcCircle && a.sources[circle].kind != srcArc) {
		return nil
	}
	if !a.circleBoundsReady {
		a.circleBoundsReady, a.circleBoundsValid = true, true
		a.circleBounds = make([][4]float64, len(a.sources))
		for k := range a.sources {
			box, ok := fullSourceBox(&a.sources[k])
			if !ok {
				a.circleBoundsValid = false
				break
			}
			a.circleBounds[k] = box
		}
	}
	if !a.circleBoundsValid {
		return nil
	}
	c := &LineCircleCrossing{
		sources: [2]int{line, circle}, point: [2]float64{e.x, e.y},
		branch: e.circleBranch, allBounds: a.circleBounds,
	}
	if a.sources[circle].kind == srcArc {
		c.arc = &LineArcCrossing{core: c}
	}
	return c
}

func fullSourceBox(s *source) ([4]float64, bool) {
	var box [4]float64
	switch s.kind {
	case srcLine:
		box = [4]float64{math.Min(s.ax, s.bx), math.Min(s.ay, s.by),
			math.Max(s.ax, s.bx), math.Max(s.ay, s.by)}
	case srcCircle, srcArc:
		if s.r <= 0 {
			return box, false
		}
		// The source bits are exact dyadics for the proof. Float addition can
		// round an extremum inward, so include one adjacent float on each side.
		box = [4]float64{
			math.Nextafter(s.cx-s.r, math.Inf(-1)),
			math.Nextafter(s.cy-s.r, math.Inf(-1)),
			math.Nextafter(s.cx+s.r, math.Inf(1)),
			math.Nextafter(s.cy+s.r, math.Inf(1)),
		}
	default:
		return box, false
	}
	for _, v := range box {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return box, false
		}
	}
	return box, true
}

func circleCrossingAtVertex(b cut, exact bool, verts *vertexTable, v int) *LineCircleCrossing {
	if !exact || b.circleCross == nil {
		return nil
	}
	x, y := verts.coord(v)
	if x != b.circleCross.point[0] || y != b.circleCross.point[1] {
		return nil
	}
	return b.circleCross
}

// certifyLineCircleCrossings removes a token unless its final vertex has
// exactly four incident fragments, two from each source and none from another.
func (a *arranger) certifyLineCircleCrossings() {
	type contact struct {
		cross  *LineCircleCrossing
		seen   int
		counts [2]int
		valid  bool
		known  bool
	}
	// Indexed by vertex id.
	nv := len(a.verts.xs)
	for _, e := range a.edges {
		nv = max(nv, e.u+1, e.v+1)
	}
	contacts := make([]contact, nv)
	visit := func(v, src int, cross *LineCircleCrossing) {
		a.budget.step(1)
		c := &contacts[v]
		if !c.known {
			*c = contact{cross: cross, valid: cross != nil, known: true}
		}
		c.seen++
		if cross == nil || cross != c.cross {
			c.valid = false
			return
		}
		switch src {
		case cross.sources[0]:
			c.counts[0]++
		case cross.sources[1]:
			c.counts[1]++
		default:
			c.valid = false
		}
	}
	for _, e := range a.edges {
		a.budget.step(1)
		visit(e.u, e.src, e.circleU)
		visit(e.v, e.src, e.circleV)
	}
	for i := range a.edges {
		a.budget.step(1)
		e := &a.edges[i]
		for _, end := range []struct {
			v     int
			cross **LineCircleCrossing
		}{{e.u, &e.circleU}, {e.v, &e.circleV}} {
			a.budget.step(1)
			c := &contacts[end.v]
			if !c.valid || c.seen != 4 || c.counts != [2]int{2, 2} {
				*end.cross = nil
			}
		}
	}
}
