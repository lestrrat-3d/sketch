package geom

import "slices"

// Sub-range loop elements (docs/loop-containment-design.md, "Sub-range elements").

// coverSpan marks, in spanEdges, every edge the span covers, and returns the defect
// that makes the span unusable, if any.
//
// The span covers the edges its source emits whose two parameters lie inside
// [Lo, Hi]. An edge that straddles an end is not covered, so the first or last
// covered edge then misses the end and the span has no exact bound there. Each end must be the exact parameter of an edge bound: a parameter that is
// only equal to a sampled bound by chance would place the end at a vertex it was not
// cut at. Edges are joined by their vertex ids, so a pruned edge or an edge the
// source did not emit leaves a break and the span reads as a gap.
func (g *FaceGraph) coverSpan(el LoopElement, spanEdges []int) LoopDefect {
	type run struct {
		edge             int
		lo, hi           float64
		loExact, hiExact bool
		loVert, hiVert   int
	}
	var runs []run
	for i, e := range g.Edges {
		if e.Src != el.Source {
			continue
		}
		r := run{edge: i, lo: e.PU, hi: e.PV, loExact: e.ExactU, hiExact: e.ExactV, loVert: e.U, hiVert: e.V}
		if e.PV < e.PU {
			r = run{edge: i, lo: e.PV, hi: e.PU, loExact: e.ExactV, hiExact: e.ExactU, loVert: e.V, hiVert: e.U}
		}
		if r.lo >= el.Lo && r.hi <= el.Hi {
			runs = append(runs, r)
		}
	}
	if len(runs) == 0 {
		return LoopDefectSpanBound
	}
	slices.SortFunc(runs, func(a, b run) int {
		switch {
		case a.lo < b.lo:
			return -1
		case a.lo > b.lo:
			return 1
		}
		return 0
	})
	first, last := runs[0], runs[len(runs)-1]
	if first.lo != el.Lo || !first.loExact || last.hi != el.Hi || !last.hiExact {
		return LoopDefectSpanBound
	}
	for k := 1; k < len(runs); k++ {
		if runs[k-1].hiVert != runs[k].loVert {
			return LoopDefectGap
		}
	}
	for _, r := range runs {
		spanEdges[r.edge]++
	}
	return LoopDefectNone
}

// oneCycle reports whether the covered edges form one connected piece.
func (g *FaceGraph) oneCycle(covered []bool) bool {
	parent := map[int]int{}
	var find func(int) int
	find = func(v int) int {
		p, ok := parent[v]
		if !ok {
			parent[v] = v
			return v
		}
		if p == v {
			return v
		}
		root := find(p)
		parent[v] = root
		return root
	}
	for i, e := range g.Edges {
		if !covered[i] {
			continue
		}
		parent[find(e.U)] = find(e.V)
	}
	roots := map[int]struct{}{}
	for v := range parent {
		roots[find(v)] = struct{}{}
	}
	return len(roots) == 1
}
