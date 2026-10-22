package geom

import (
	"math"
	"slices"
)

// The face graph is the arrangement's adjacency, published for loop containment
// (docs/loop-containment-design.md). It is built only under [WithFaceGraph], so an
// arrangement requested without it does the same work and returns the same values
// as before.

// Faces other than a region of [Arrangement.Regions] are named by these values in
// [FaceEdge].
const (
	// FaceOuter names the unbounded face: the one outside every component.
	FaceOuter = -1
	// FaceUnknown names a side whose face the arrangement does not report: a cycle
	// below the area floor, a hole no face was assigned, or a cycle of zero area.
	FaceUnknown = -2
)

// WithFaceGraph asks [Regions] to also publish [Arrangement.FaceGraph].
func WithFaceGraph() Option {
	return func(c *arrangeConfig) { c.faceGraph = true }
}

// FaceGraph is the adjacency of an arrangement's faces.
type FaceGraph struct {
	// Faces has one entry per [Arrangement.Regions] element, in the same order.
	Faces []FaceNode
	// Edges has one entry per arrangement edge that survived pruning.
	Edges []FaceEdge
	// Pruned lists the sources that lost at least one edge to pruning: a dangling
	// spur that bounds no face. A closed loop never loses an edge this way.
	Pruned []int
}

// FaceNode is the soundness record of one region.
type FaceNode struct {
	// Faithful is true when the planar map around the region can be trusted: the
	// exactness gate is open, no source on its boundary is in a component whose map
	// may be fused, and the region is neither degenerate nor self-intersecting.
	Faithful bool
}

// FaceEdge is one arrangement edge with the two faces on its sides.
type FaceEdge struct {
	// A and B are the faces on the two sides: an index into [FaceGraph.Faces],
	// [FaceOuter] or [FaceUnknown].
	A, B int
	// U and V are the edge's two vertex ids.
	U, V int
	// Covers lists, sorted and without repeats, every source that draws the edge:
	// the source the edge is emitted from, every source suppressed in favour of it
	// by a resolved coincident-carrier overlap, and every duplicate line merged into
	// it.
	Covers []int
	// Src is the source the edge is emitted from, and PU and PV are that source's
	// natural parameters at U and V. They may run either way along the edge. ExactU
	// and ExactV report whether each parameter is the TRUE parameter of its vertex
	// (the same meaning as [BoundaryEdge.TExact], per bound). A sub-range loop element
	// is matched against these.
	Src            int
	PU, PV         float64
	ExactU, ExactV bool
}

// buildFaceGraph reads the arrangement state extract has just finished. cycleFace
// maps a cycle id to a region index, FaceOuter or FaceUnknown.
func (a *arranger) buildFaceGraph(arr *Arrangement, cycleFace []int) *FaceGraph {
	g := &FaceGraph{
		Faces: make([]FaceNode, len(arr.Regions)),
		Edges: make([]FaceEdge, len(a.edges)),
	}
	for i, reg := range arr.Regions {
		a.budget.step(1)
		g.Faces[i].Faithful = a.regionFaithful(reg)
	}
	merged := a.mergedLineSources()
	for ei, e := range a.edges {
		a.budget.step(1)
		covers := []int{e.src}
		covers = append(covers, a.losingCovers(e)...)
		if e.src >= 0 && a.sources[e.src].kind == srcLine {
			covers = append(covers, merged[lineKey(e.u, e.v)]...)
		}
		slices.Sort(covers)
		covers = slices.Compact(covers)
		g.Edges[ei] = FaceEdge{
			A: cycleFace[a.cycleOf[2*ei]], B: cycleFace[a.cycleOf[2*ei+1]],
			U: e.u, V: e.v, Covers: covers,
			Src: e.src, PU: e.pu, PV: e.pv, ExactU: e.exactU, ExactV: e.exactV,
		}
	}
	seen := map[int]struct{}{}
	for _, e := range a.pruned {
		a.budget.step(1)
		if _, ok := seen[e.src]; !ok {
			seen[e.src] = struct{}{}
			g.Pruned = append(g.Pruned, e.src)
		}
	}
	slices.Sort(g.Pruned)
	return g
}

// regionFaithful reports whether the planar map around a region can be trusted.
func (a *arranger) regionFaithful(reg *Region) bool {
	if !a.exactAllowed || reg.Degenerate || reg.SelfIntersecting {
		return false
	}
	ok := true
	check := func(edges []BoundaryEdge) {
		for _, e := range edges {
			if a.exactRefused != nil && a.exactRefused[e.SourceIndex] {
				ok = false
			}
		}
	}
	check(reg.Outer)
	for _, h := range reg.Holes {
		check(h)
	}
	return ok
}

type lineVertexKey struct{ u, v int }

func lineKey(u, v int) lineVertexKey {
	if u > v {
		u, v = v, u
	}
	return lineVertexKey{u, v}
}

// mergedLineSources maps the endpoints of a kept line edge to the sources of the
// duplicate lines dedupCoincidentStraightEdges dropped in its favour.
func (a *arranger) mergedLineSources() map[lineVertexKey][]int {
	plan := a.dedupPlan
	if plan == nil {
		return nil
	}
	out := map[lineVertexKey][]int{}
	for _, g := range plan.groups {
		a.budget.step(1)
		for _, i := range g.idxs {
			if _, dropped := plan.drop[i]; dropped {
				k := lineKey(g.u, g.v)
				out[k] = append(out[k], plan.original[i].src)
			}
		}
	}
	return out
}

// losingCovers returns the sources a resolved coincident-carrier overlap suppressed
// in favour of the edge's source over a window that holds the edge.
func (a *arranger) losingCovers(e arrEdge) []int {
	var out []int
	mid := (e.pu + e.pv) / 2
	for k := range a.overlaps {
		a.budget.step(1)
		o := &a.overlaps[k]
		if o.refused || o.named != e.src {
			continue
		}
		named := &a.sources[o.named]
		if w, ok := o.win.(paramWindow); ok {
			// A line window is in the losing line's own parameter.
			loser := &a.sources[o.losing]
			p := named.at(mid)
			dx, dy := loser.bx-loser.ax, loser.by-loser.ay
			den := dx*dx + dy*dy
			if den == 0 || math.IsNaN(den) {
				continue
			}
			t := ((p[0]-loser.ax)*dx + (p[1]-loser.ay)*dy) / den
			if w.covers(loser, t) {
				out = append(out, o.losing)
			}
			continue
		}
		if o.win.covers(named, mid) {
			out = append(out, o.losing)
		}
	}
	return out
}

// LoopWalk is the outcome of the parity walk for one loop.
type LoopWalk struct {
	// Defect is non-zero when the loop cannot be judged at all.
	Defect LoopDefect
	// Side[i] is 1 when face i is inside the loop, 0 when outside, and -1 when the
	// walk did not reach it through faithful faces.
	Side []int8
}

// LoopDefect says why a loop has no verdict anywhere.
type LoopDefect uint8

const (
	LoopDefectNone LoopDefect = iota
	// LoopDefectGap: an edge of the loop was pruned as a dangling spur.
	LoopDefectGap
	// LoopDefectSelfCrossing: two sources of the loop draw one edge, or a vertex
	// has other than two loop edges.
	LoopDefectSelfCrossing
	// LoopDefectInconsistent: the parity of the loop's edges around some cycle is odd.
	LoopDefectInconsistent
	// LoopDefectSpanBound: a span end of the loop is not an exact bound of an
	// arrangement edge of its source, or the span holds no edge.
	LoopDefectSpanBound
)

// LoopElement is one element of a loop: the whole of a source, or the part of it
// between two of its natural parameters.
type LoopElement struct {
	// Source is the arrangement source index.
	Source int
	// Span is false for the whole source. When true, Lo < Hi bound the element in the
	// source's natural parameter (the one [BoundaryEdge.TStart] is in).
	Span   bool
	Lo, Hi float64
}

// WalkLoops runs the parity walk of docs/loop-containment-design.md. loops[k]
// lists the sources of loop k, each taken whole. See [FaceGraph.WalkLoopElements].
func (g *FaceGraph) WalkLoops(loops [][]int) []LoopWalk {
	elems := make([][]LoopElement, len(loops))
	for k, srcs := range loops {
		for _, s := range srcs {
			elems[k] = append(elems[k], LoopElement{Source: s})
		}
	}
	return g.WalkLoopElements(elems)
}

// WalkLoopElements runs the parity walk for loops given as elements. The unbounded
// face is outside every loop, and crossing an edge covered by loop k changes the side
// of k. The walk does not enter an unfaithful face or an unknown side. A loop whose
// edges do not agree around some cycle, or whose edges the map prunes or draws twice,
// gets a Defect and no sides.
//
// A whole element covers every edge its source draws (see [FaceEdge.Covers]). A span
// covers the edges its source emits whose parameters lie inside the span. Its two
// ends must be exact bounds of edges, the edges must run without a break from one
// end to the other, and a loop with a span must form one cycle.
func (g *FaceGraph) WalkLoopElements(loops [][]LoopElement) []LoopWalk {
	n := len(g.Faces)
	out := make([]LoopWalk, len(loops))
	for k, elems := range loops {
		out[k] = g.walkLoop(elems, n)
	}
	return out
}

func (g *FaceGraph) walkLoop(elems []LoopElement, n int) LoopWalk {
	w := LoopWalk{Side: make([]int8, n)}
	for i := range w.Side {
		w.Side[i] = -1
	}
	var srcs []int
	hasSpan := false
	for _, el := range elems {
		if el.Span {
			hasSpan = true
			continue
		}
		srcs = append(srcs, el.Source)
		if _, found := slices.BinarySearch(g.Pruned, el.Source); found {
			w.Defect = LoopDefectGap
			return w
		}
	}
	spanEdges := make([]int, len(g.Edges))
	for _, el := range elems {
		if !el.Span {
			continue
		}
		defect := g.coverSpan(el, spanEdges)
		if defect != LoopDefectNone {
			w.Defect = defect
			return w
		}
	}
	in := func(s int) bool { return slices.Contains(srcs, s) }
	covered := make([]bool, len(g.Edges))
	degree := map[int]int{}
	for i, e := range g.Edges {
		count := spanEdges[i]
		for _, s := range e.Covers {
			if in(s) {
				count++
			}
		}
		switch {
		case count > 1:
			w.Defect = LoopDefectSelfCrossing
			return w
		case count == 1:
			covered[i] = true
			degree[e.U]++
			degree[e.V]++
		}
	}
	for _, d := range degree {
		if d != 2 {
			w.Defect = LoopDefectSelfCrossing
			return w
		}
	}
	if hasSpan && !g.oneCycle(covered) {
		w.Defect = LoopDefectGap
		return w
	}
	// Breadth-first from the unbounded face. side is the parity so far.
	side := make([]int8, n)
	reached := make([]bool, n)
	adj := make([][]int, n) // edge indices incident to each region
	var outerEdges []int
	for i, e := range g.Edges {
		for _, f := range [2]int{e.A, e.B} {
			if f >= 0 {
				adj[f] = append(adj[f], i)
			}
		}
		if e.A == FaceOuter || e.B == FaceOuter {
			outerEdges = append(outerEdges, i)
		}
	}
	faithful := func(f int) bool { return f >= 0 && g.Faces[f].Faithful }
	var queue []int
	step := func(ei, from int, fromSide int8) {
		e := g.Edges[ei]
		to := e.A
		if e.A == from {
			to = e.B
		}
		if !faithful(to) || reached[to] {
			return
		}
		next := fromSide
		if covered[ei] {
			next ^= 1
		}
		reached[to] = true
		side[to] = next
		queue = append(queue, to)
	}
	for _, ei := range outerEdges {
		e := g.Edges[ei]
		to := e.A
		if to == FaceOuter {
			to = e.B
		}
		if !faithful(to) || reached[to] {
			continue
		}
		var s int8
		if covered[ei] {
			s = 1
		}
		reached[to] = true
		side[to] = s
		queue = append(queue, to)
	}
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		for _, ei := range adj[f] {
			step(ei, f, side[f])
		}
	}
	// Every edge between two reached faces, or a reached face and the outside, must
	// agree with the covering parity.
	for i, e := range g.Edges {
		var sa, sb int8
		switch {
		case e.A == FaceOuter:
			sa = 0
		case e.A >= 0 && reached[e.A]:
			sa = side[e.A]
		default:
			continue
		}
		switch {
		case e.B == FaceOuter:
			sb = 0
		case e.B >= 0 && reached[e.B]:
			sb = side[e.B]
		default:
			continue
		}
		want := int8(0)
		if covered[i] {
			want = 1
		}
		if sa^sb != want {
			w.Defect = LoopDefectInconsistent
			return w
		}
	}
	for f := range w.Side {
		if reached[f] {
			w.Side[f] = side[f]
		}
	}
	return w
}
