package geom

import (
	"math"
	"sort"
)

// buildChains publishes the open connected runs of arrangement edges that no
// region boundary uses — the edges prune dropped (dangling spurs and open
// trees), plus any surviving edge whose faces were not published — as [Chain]s.
//
// The candidate set is DERIVED, never named by the caller: an edge is a chain
// edge exactly when it is not a region-boundary edge, so a caller asking for
// both publications sees every edge of the arrangement exactly once and needs no
// selection rule of its own.
//
// The walk is cut at every vertex where it cannot continue unambiguously, and
// that question is asked of the WHOLE arrangement, not of the candidate set: a
// vertex where three lines meet, or where two curves cross, stops the walk even
// when the other edges there went to a region boundary. Otherwise a chain would
// quietly turn a corner at a point where the sketch branches, and its consumer
// would sweep a curve through a crossing the 2D answer had already resolved.
// Three lines meeting at a point therefore publish three chains: a refusal would
// lose edges a consumer can legitimately use, and a guess would publish a walk
// the geometry does not justify. A closed run is published as no chain — see
// [Chain].
func (a *arranger) buildChains(used []bool) []*Chain {
	cands := make([]arrEdge, 0, len(a.pruned))
	cands = append(cands, a.pruned...)
	for i, e := range a.edges {
		if !used[i] {
			cands = append(cands, e)
		}
	}
	if len(cands) == 0 {
		return nil
	}

	// deg counts EVERY arrangement edge at a vertex — the pruned ones, the ones a
	// region took, and the candidates alike — so it answers "does the sketch branch
	// here", which is the question the walk has to stop on.
	deg := map[int]int{}
	for _, e := range a.pruned {
		deg[e.u]++
		deg[e.v]++
	}
	for _, e := range a.edges {
		deg[e.u]++
		deg[e.v]++
	}
	inc := map[int][]int{} // vertex -> incident candidate edges, in candidate order
	for i, e := range cands {
		inc[e.u] = append(inc[e.u], i)
		inc[e.v] = append(inc[e.v], i)
	}
	// Every maximal run starts at a vertex the walk cannot pass through, so those
	// are the only starts worth trying. Which run an edge lands in does not depend
	// on the order they are tried — the runs partition the candidate edges — so
	// this order decides nothing but the order chains are discovered in, and
	// chainLess settles the published order anyway. Edges reachable from no such
	// vertex are a pure cycle, and are deliberately left unpublished.
	starts := make([]int, 0, len(inc))
	for v, list := range inc {
		if deg[v] != 2 || len(list) != 2 {
			starts = append(starts, v)
		}
	}
	sort.Ints(starts)

	taken := make([]bool, len(cands))
	var chains []*Chain
	for _, v := range starts {
		for _, ei := range inc[v] {
			if taken[ei] {
				continue
			}
			if c := a.walkChain(cands, inc, deg, taken, v, ei); c != nil {
				chains = append(chains, c)
			}
		}
	}
	sort.SliceStable(chains, func(i, j int) bool { return chainLess(chains[i], chains[j]) })
	return chains
}

// walkChain follows the run that leaves vertex from along candidate edge ei,
// through degree-2 vertices only, and publishes it as a chain. It returns nil
// for a run that closes back on its own start vertex, which is not an open
// chain.
func (a *arranger) walkChain(cands []arrEdge, inc map[int][]int, deg map[int]int, taken []bool, from, ei int) *Chain {
	var frags []boundaryFrag
	srcs := map[int]struct{}{}
	at := from
	for {
		taken[ei] = true
		e := cands[ei]
		srcs[e.src] = struct{}{}
		forward := e.u == at
		pStart, pEnd := e.pu, e.pv
		exStart, exEnd := e.exactU, e.exactV
		enStart, enEnd := e.endU, e.endV
		to := e.v
		if !forward {
			pStart, pEnd = e.pv, e.pu
			exStart, exEnd = e.exactV, e.exactU
			enStart, enEnd = e.endV, e.endU
			to = e.u
		}
		fx, fy := a.verts.coord(at)
		tx, ty := a.verts.coord(to)
		frags = appendBoundaryFrag(frags, e.src, pStart, pEnd, exStart, exEnd, enStart, enEnd,
			[2]float64{fx, fy}, [2]float64{tx, ty})

		at = to
		list := inc[at]
		if deg[at] != 2 || len(list) != 2 {
			// A free end, a branch or a crossing the walk must not guess its way
			// through, or a vertex where the only way on belongs to a region.
			break
		}
		next := list[0]
		if next == ei {
			next = list[1]
		}
		if taken[next] {
			break // the run closed back on itself
		}
		ei = next
	}
	if at == from {
		return nil // a closed run: a Chain is open by definition
	}

	c := &Chain{Edges: make([]BoundaryEdge, 0, len(frags))}
	for _, f := range frags {
		c.Edges = append(c.Edges, boundaryEdgeOf(f))
		c.Length += a.fragLength(f)
	}
	c.Edges = canonicalChainDirection(c.Edges)
	c.Degenerate = a.degenReaches(srcs)
	c.SelfIntersecting = a.chainSelfIntersects(c.Edges)
	return c
}

// fragLength is the arc length of one coalesced fragment: the closed form on the
// reported parameter range for a line, arc or circle, and the chord sum of the
// emitted polyline otherwise. A sampled length is an underestimate that converges
// with [WithSegmentsPerTurn], the same way a sampled parameter does — the exact
// alternative would need an arc-length integral per curve family, which nothing
// here has.
func (a *arranger) fragLength(f boundaryFrag) float64 {
	s := &a.sources[f.src]
	span := math.Abs(f.pEnd - f.pStart)
	switch s.kind {
	case srcLine:
		return math.Hypot(s.bx-s.ax, s.by-s.ay) * span
	case srcArc:
		return s.r * math.Abs(s.sweep) * span
	case srcCircle:
		return s.r * 2 * math.Pi * span
	}
	return polylineLength(f.dense)
}

func polylineLength(p [][2]float64) float64 {
	var total float64
	for i := 1; i < len(p); i++ {
		total += math.Hypot(p[i][0]-p[i-1][0], p[i][1]-p[i-1][1])
	}
	return total
}

// canonicalChainDirection picks one of a chain's two possible walks, so two
// arrangements of equal geometry publish equal chains whatever order their
// curves were handed in: the walk starts at the lexicographically smaller end
// point. Two ends at the same coordinate would be one vertex (they are welded),
// so the tie-break below can only be reached by an exact coordinate coincidence
// across two distinct vertices; it compares the whole walk, which is what
// actually differs.
func canonicalChainDirection(edges []BoundaryEdge) []BoundaryEdge {
	if len(edges) == 0 {
		return edges
	}
	fwd := chainDense(edges)
	start, end := fwd[0], fwd[len(fwd)-1]
	switch {
	case ptLess(start, end):
		return edges
	case ptLess(end, start):
		return reverseChain(edges)
	}
	rev := reverseChain(edges)
	if polylineLess(chainDense(rev), fwd) {
		return rev
	}
	return edges
}

// reverseChain walks the same edges the other way: the edge order reverses, each
// edge's Reversed flag flips and its polyline runs backwards. TStart/TEnd,
// TExact and Whole are properties of the source's NATURAL direction, so they are
// untouched — that is exactly what Reversed carries the walk order for.
func reverseChain(edges []BoundaryEdge) []BoundaryEdge {
	out := make([]BoundaryEdge, 0, len(edges))
	for i := len(edges) - 1; i >= 0; i-- {
		e := edges[i]
		e.Reversed = !e.Reversed
		pl := make([][2]float64, len(e.Polyline))
		for k, p := range e.Polyline {
			pl[len(pl)-1-k] = p
		}
		e.Polyline = pl
		out = append(out, e)
	}
	return out
}

// chainDense concatenates the walk's polylines into one, dropping the duplicated
// joint between consecutive edges.
func chainDense(edges []BoundaryEdge) [][2]float64 {
	var dense [][2]float64
	for i, e := range edges {
		if i > 0 && len(e.Polyline) > 0 {
			dense = append(dense, e.Polyline[1:]...)
			continue
		}
		dense = append(dense, e.Polyline...)
	}
	return dense
}

func ptLess(a, b [2]float64) bool {
	if a[0] != b[0] {
		return a[0] < b[0]
	}
	return a[1] < b[1]
}

func polylineLess(a, b [][2]float64) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return ptLess(a[i], b[i])
		}
	}
	return len(a) < len(b)
}

// chainLess is the published order: by start point, then end point, then the
// whole walk. It is stated in coordinates rather than in input order so the same
// drawing publishes the same chain list however its curves were ordered.
func chainLess(x, y *Chain) bool {
	xd, yd := chainDense(x.Edges), chainDense(y.Edges)
	if len(xd) == 0 || len(yd) == 0 {
		return len(xd) < len(yd)
	}
	if xd[0] != yd[0] {
		return ptLess(xd[0], yd[0])
	}
	if xe, ye := xd[len(xd)-1], yd[len(yd)-1]; xe != ye {
		return ptLess(xe, ye)
	}
	return polylineLess(xd, yd)
}

// chainSelfIntersects reports whether the chain's own emitted polyline crosses
// or touches itself away from its walk joints.
//
// A self-crossing the arrangement RESOLVED never reaches this test: the crossing
// is a vertex, the vertex has degree 4, and buildChains cuts the walk there — so
// what this catches is a contact the planar map does NOT have, which is the same
// condition [Arrangement.Degenerate] reports from the other side (a collinear
// overlap, a near-tangent approach, a crossing hidden between two samples of a
// free-form curve). Reporting it per chain is what lets a consumer reject that
// one chain rather than the whole sketch.
//
// A touch is measured against the arrangement's own merge distance, since a
// contact closer than that is one the map would have welded had it been between
// two boundaries; a crossing is decided by orientation sign, with no tolerance.
func (a *arranger) chainSelfIntersects(edges []BoundaryEdge) bool {
	p := chainDense(edges)
	// Consecutive segments always meet at their shared joint, so they are asked a
	// narrower question: does the walk turn back ALONG the segment it arrived on?
	// A hairpin that retraces its own path covers the same ground twice, which is
	// the self-touch a sweep cannot use; an ordinary corner, however sharp, does
	// not and is left alone.
	for i := 0; i+2 < len(p); i++ {
		if chordsRetrace(p[i], p[i+1], p[i+2]) {
			return true
		}
	}
	for i := 0; i+1 < len(p); i++ {
		for j := i + 2; j+1 < len(p); j++ {
			if !segBoxesTouch(p[i], p[i+1], p[j], p[j+1], a.merge) {
				continue
			}
			if segmentsMeet(p[i], p[i+1], p[j], p[j+1], a.merge) {
				return true
			}
		}
	}
	return false
}

// chordsRetrace reports whether the walk arriving at b along a→b leaves it along
// b→c collinearly and in the opposite direction, so the two chords lie on top of
// each other. Collinearity is judged relative to the two chord lengths, the same
// way every direction comparison in this package is.
func chordsRetrace(a, b, c [2]float64) bool {
	ux, uy := b[0]-a[0], b[1]-a[1]
	vx, vy := c[0]-b[0], c[1]-b[1]
	un, vn := math.Hypot(ux, uy), math.Hypot(vx, vy)
	if un == 0 || vn == 0 {
		return false
	}
	if math.Abs(ux*vy-uy*vx) > dirCollinearEps*un*vn {
		return false // not collinear: an ordinary corner
	}
	return ux*vx+uy*vy < 0 // collinear and reversed: the walk doubles back
}

// dirCollinearEps is the relative bound on sin(angle) below which two chords
// count as lying on one line.
const dirCollinearEps = 1e-9

func segBoxesTouch(a0, a1, b0, b1 [2]float64, eps float64) bool {
	return math.Min(a0[0], a1[0])-eps <= math.Max(b0[0], b1[0]) &&
		math.Min(b0[0], b1[0])-eps <= math.Max(a0[0], a1[0]) &&
		math.Min(a0[1], a1[1])-eps <= math.Max(b0[1], b1[1]) &&
		math.Min(b0[1], b1[1])-eps <= math.Max(a0[1], a1[1])
}

// segmentsMeet reports whether two closed segments cross transversally or come
// within eps of each other.
func segmentsMeet(a0, a1, b0, b1 [2]float64, eps float64) bool {
	d1, d2 := orient(b0, b1, a0), orient(b0, b1, a1)
	d3, d4 := orient(a0, a1, b0), orient(a0, a1, b1)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	return ptSegDist(a0, b0, b1) <= eps || ptSegDist(a1, b0, b1) <= eps ||
		ptSegDist(b0, a0, a1) <= eps || ptSegDist(b1, a0, a1) <= eps
}

func orient(a, b, c [2]float64) float64 {
	return (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
}

func ptSegDist(q, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	den := dx*dx + dy*dy
	if den == 0 {
		return math.Hypot(q[0]-a[0], q[1]-a[1])
	}
	t := ((q[0]-a[0])*dx + (q[1]-a[1])*dy) / den
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(q[0]-(a[0]+t*dx), q[1]-(a[1]+t*dy))
}
