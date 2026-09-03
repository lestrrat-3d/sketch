package geom

import (
	"slices"
	"sort"
)

// broadPhaseRel is the relative reach added to every tiny segment's bounding box on
// top of the vertex-merge tolerance. It is sized by the LARGEST of three bounds, all
// relative to a chord's own length:
//
//   - segParams' segEps=1e-9 on the chord parameter (a hit lies within segEps·length
//     of each chord);
//   - collinearOverlap's 1e-7 perpendicular-gap band and 1e-9 overlap-length band;
//   - the rounding slop of segParams' own intersection solve. That solve accepts a
//     pair down to |d1×d2| = 1e-12·|d1||d2|, and at that near-parallel limit the
//     computed chord parameters carry an absolute error of order
//     eps/1e-12 ≈ 4e-4 relative to the chord lengths — three orders of magnitude
//     ABOVE segEps, and the term that actually sets this constant.
//
// 1e-3 dominates all three, so the reach only ever ADMITS more pairs than the exact
// tests would accept — see candidatePairs' doc comment and TestBroadPhaseIsSuperset.
//
// The third bound holds for geometry whose coordinates are within a few chord
// lengths of the origin. Far from it, the cancellation in that solve grows with the
// coordinate magnitude and no relative reach bounds it — but neither is segParams'
// own answer meaningful there, so the arrangement is already unreliable in that
// regime and the broad phase adds no failure mode of its own.
const broadPhaseRel = 1e-3

// segBox is an axis-aligned bounding box, already expanded by a segment's reach.
type segBox struct{ minX, minY, maxX, maxY float64 }

// segReach is tiny segment i's expansion beyond its raw chord bounding box: the
// vertex-merge distance (which is what forEachMergedEnd welds two DIFFERENT
// sources' endpoints within) plus broadPhaseRel times the segment's own chord
// length (which dominates segParams' and collinearOverlap's relative bands — see
// the proof in .claude/docs/profiles-geom.md, "The broad-phase reach").
func (a *arranger) segReach(i int) float64 {
	return a.merge + broadPhaseRel*a.segLen(i)
}

// segBoxOf returns tiny segment i's chord bounding box expanded by its reach.
func (a *arranger) segBoxOf(i int) segBox {
	s := &a.segs[i]
	minX, maxX := s.ax, s.bx
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := s.ay, s.by
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	r := a.segReach(i)
	return segBox{minX - r, minY - r, maxX + r, maxY + r}
}

// segBoxCache returns every tiny segment's reach-expanded bounding box, computing
// it once and caching it on the arranger. a.segs is fixed by the time this is first
// called (densify is the only appender, and it runs before intersect), so the boxes
// stay valid for the arranger's remaining passes. Shared by candidatePairs' sweep and
// sampledCrossingsExplained's box reject so neither recomputes a box the other
// already has.
func (a *arranger) segBoxCache() []segBox {
	if a.segBoxes == nil && len(a.segs) > 0 {
		a.segBoxes = make([]segBox, len(a.segs))
		for i := range a.segs {
			a.segBoxes[i] = a.segBoxOf(i)
		}
	}
	return a.segBoxes
}

// boxesOverlap reports whether two boxes overlap, treating a touch (shared edge or
// corner) as an overlap: the pair tests intersect's loop body guards on are
// tolerance-bounded equalities, so a box touch is exactly the boundary case those
// tests can still fire on.
func boxesOverlap(a, b segBox) bool {
	if a.maxX < b.minX || b.maxX < a.minX {
		return false
	}
	if a.maxY < b.minY || b.maxY < a.minY {
		return false
	}
	return true
}

// candidatePairs returns, for every tiny segment i, the ascending list of j > i
// whose reach-expanded boxes overlap i's. It is a conservative broad phase: the
// three predicates intersect's loop body gates every side effect on —
// forEachMergedEnd, segParams, collinearOverlap — never fire on a pair whose
// expanded boxes fail to overlap (see .claude/docs/profiles-geom.md, "The
// broad-phase reach", and TestBroadPhaseIsSuperset), so candidatePairs may only
// ever ADD pairs beyond what those predicates would accept, never drop one they
// would have fired on.
//
// Implementation is a standard sort-and-sweep on minX: segments are visited in
// ascending minX order (ties broken by original index, so the sweep itself is
// deterministic), and an "active" set holds every segment whose box could still
// overlap a later one's. Before a segment is inserted, any active segment whose
// maxX already sits below the new segment's minX is evicted — it can never
// overlap anything with an even larger minX, since minX values only increase
// through the sweep. Every remaining active segment is then checked for a y-axis
// overlap (the x-overlap is guaranteed by the sweep order) and, on a hit,
// recorded under the smaller of the two original indices. The result is finally
// sorted per-index so the caller's (i, cand[i]) walk visits pairs in the same
// relative order the old lexicographic (i, j) scan did — load-bearing because
// splitFragments dedups near-equal cut parameters keeping the first, and
// sort.Slice is unstable.
func (a *arranger) candidatePairs() [][]int {
	n := len(a.segs)
	cand := make([][]int, n)
	if n == 0 {
		return cand
	}
	boxes := a.segBoxCache()
	order := make([]int, n)
	for i := 0; i < n; i++ {
		order[i] = i
	}
	sort.Slice(order, func(x, y int) bool {
		bx, by := boxes[order[x]], boxes[order[y]]
		if bx.minX != by.minX {
			return bx.minX < by.minX
		}
		return order[x] < order[y]
	})
	active := make([]int, 0, n)
	for _, idx := range order {
		b := boxes[idx]
		kept := active[:0]
		for _, t := range active {
			if boxes[t].maxX < b.minX {
				continue
			}
			kept = append(kept, t)
		}
		active = kept
		for _, t := range active {
			if !boxesOverlap(b, boxes[t]) {
				continue
			}
			lo, hi := idx, t
			if lo > hi {
				lo, hi = hi, lo
			}
			cand[lo] = append(cand[lo], hi)
		}
		active = append(active, idx)
	}
	for i := range cand {
		slices.Sort(cand[i])
	}
	return cand
}
