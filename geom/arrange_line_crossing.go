package geom

// LineLineCrossing identifies one analytic crossing of two distinct input lines.
// Only Regions creates witnesses. A witness is shared by the two sides of the
// crossing within one arrangement and is absent when any contact merged into it.
type LineLineCrossing struct {
	sources [2]int
	point   [2]float64
}

// SourceIndices returns the two Regions input positions that made the crossing.
func (c *LineLineCrossing) SourceIndices() [2]int { return c.sources }

// Point returns the arrangement's rounded event point. Consumers requiring an
// exactly representable crossing must solve the source lines in exact arithmetic.
func (c *LineLineCrossing) Point() [2]float64 { return c.point }

func crossingAtVertex(b cut, exact bool, verts *vertexTable, v int) *LineLineCrossing {
	if !exact || b.cross == nil {
		return nil
	}
	x, y := verts.coord(v)
	if x != b.cross.point[0] || y != b.cross.point[1] {
		return nil
	}
	return b.cross
}

// certifyLineLineCrossings only retains a token when the graph vertex has four
// incident fragments, two from each event source, all carrying that same token.
// A sampled cut, source endpoint, third line, or nearby analytic event merged by
// the vertex table removes the token from every incident edge.
func (a *arranger) certifyLineLineCrossings() {
	type vertexContact struct {
		cross  *LineLineCrossing
		seen   int
		counts [2]int
		valid  bool
	}
	contacts := make(map[int]*vertexContact)
	visit := func(v, src int, cross *LineLineCrossing) {
		a.budget.step(1)
		c := contacts[v]
		if c == nil {
			c = &vertexContact{cross: cross, valid: cross != nil}
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
		a.budget.step(1)
		visit(e.u, e.src, e.crossU)
		visit(e.v, e.src, e.crossV)
	}
	for i := range a.edges {
		a.budget.step(1)
		e := &a.edges[i]
		for _, end := range []struct {
			v     int
			cross **LineLineCrossing
		}{{e.u, &e.crossU}, {e.v, &e.crossV}} {
			a.budget.step(1)
			c := contacts[end.v]
			if !c.valid || c.seen != 4 || c.counts != [2]int{2, 2} {
				*end.cross = nil
			}
		}
	}
}
