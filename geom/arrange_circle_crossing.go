package geom

import "math"

// LineCircleCrossing identifies one unmerged analytic secant in one arrangement.
// Regions creates it; both incident source edges carry the same pointer.
type LineCircleCrossing struct {
	sources   [2]int // line, then circle
	point     [2]float64
	branch    int8 // -1 low, +1 high on the line's natural parameter
	allBounds [][4]float64
}

// SourceIndices returns the line and circle input positions, in that order.
func (c *LineCircleCrossing) SourceIndices() [2]int { return c.sources }

// Point returns the arrangement's rounded contact point.
func (c *LineCircleCrossing) Point() [2]float64 { return c.point }

// Branch returns -1 for the low line root or +1 for the high line root.
func (c *LineCircleCrossing) Branch() int8 { return c.branch }

// OtherSourceBounds returns full-carrier boxes for the arrangement's other
// sources. A caller can conservatively refuse a box overlapping its proof box.
func (c *LineCircleCrossing) OtherSourceBounds() [][4]float64 {
	out := make([][4]float64, 0, max(0, len(c.allBounds)-2))
	for i, box := range c.allBounds {
		if i != c.sources[0] && i != c.sources[1] {
			out = append(out, box)
		}
	}
	return out
}

func (a *arranger) lineCircleCrossing(i, j int, e xEvent) *LineCircleCrossing {
	if e.circleBranch == 0 || !a.exactAllowed {
		return nil
	}
	line, circle := i, j
	if a.sources[j].kind == srcLine {
		line, circle = j, i
	}
	if a.sources[line].kind != srcLine || a.sources[circle].kind != srcCircle {
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
	}
	contacts := make(map[int]*contact)
	visit := func(v, src int, cross *LineCircleCrossing) {
		c := contacts[v]
		if c == nil {
			c = &contact{cross: cross, valid: cross != nil}
			contacts[v] = c
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
		visit(e.u, e.src, e.circleU)
		visit(e.v, e.src, e.circleV)
	}
	for i := range a.edges {
		e := &a.edges[i]
		for _, end := range []struct {
			v     int
			cross **LineCircleCrossing
		}{{e.u, &e.circleU}, {e.v, &e.circleV}} {
			c := contacts[end.v]
			if !c.valid || c.seen != 4 || c.counts != [2]int{2, 2} {
				*end.cross = nil
			}
		}
	}
}
