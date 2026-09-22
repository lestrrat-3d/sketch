package geom_test

import (
	"cmp"
	"math"
	"slices"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// arcAt returns the point at angle deg on the r-radius carrier centred at the origin.
func arcAt(r, deg float64) *geom.Point {
	rad := deg * math.Pi / 180
	return geom.NewPoint(r*math.Cos(rad), r*math.Sin(rad))
}

// TestArcSpanningOneChordKeepsItsFaces pins the scene below: an arc fragment that
// reaches its two graph vertices with NO interior sample vertex between them emits a
// single edge whose chord IS the straight edge between the same two vertices, so both
// half-edges depart at a bit-identical angle. Before the exact-tangent tie-break, the
// rotation sort could not separate them, the next pointers stopped being a planar
// embedding, and the face walk returned one near-zero-area cycle over every half-edge
// instead of the bounded faces — losing them all with Degenerate false.
//
// No same-carrier pair is involved. The arc is the only curve, and the two chords
// close it, which is why this is the plainer of the two cases: it returns the wrong
// answer in EVERY input order rather than disagreeing between them.
func TestArcSpanningOneChordKeepsItsFaces(t *testing.T) {
	for _, r := range []float64{1e-4, 1, 10} {
		curves := []geom.Curve{
			geom.NewArc(geom.NewPoint(0, 0), arcAt(r, 0), arcAt(r, 15)),
			geom.NewLine(arcAt(r, 1), arcAt(r, 0)),
			geom.NewLine(arcAt(r, 15), arcAt(r, 0)),
		}
		arr := geom.Regions(curves, nil)
		// Degenerate is NOT asserted. Whether this scene trips a near-tangency
		// classification varies with the radius and with the platform's floating
		// point, and it is not the property at issue: the defect was losing the
		// faces, not mislabelling them. It is logged so a failure here is readable.
		t.Logf("r=%v degenerate=%v degeneracies=%d", r, arr.Degenerate, len(arr.Degeneracies))
		require.Lenf(t, arr.Regions, 2, "the inner chord splits the sector in two: r=%v", r)
	}
}

// TestSectorPairRegionsMatchEitherOrder pins the fu80 scene at the radii it was
// measured at. Two arcs on one carrier, the shorter lying wholly inside the longer,
// with both closing chords. Which arc is named decides whose sampling supplies the
// interior vertices over the shared span, so one order produced the single-edge tie
// this file's other test describes and published no regions at all, while the other
// published two — with Degenerate false either way.
//
// It pins these radii and these two orders, and is evidence for them rather than for
// order-independence in general.
func TestSectorPairRegionsMatchEitherOrder(t *testing.T) {
	for _, r := range []float64{1e-4, 1, 10} {
		short := geom.NewArc(geom.NewPoint(0, 0), arcAt(r, 0), arcAt(r, 1))
		long := geom.NewArc(geom.NewPoint(0, 0), arcAt(r, 0), arcAt(r, 15))
		chShort := geom.NewLine(arcAt(r, 1), arcAt(r, 0))
		chLong := geom.NewLine(arcAt(r, 15), arcAt(r, 0))

		shortFirst := geom.Regions([]geom.Curve{short, long, chShort, chLong}, nil)
		longFirst := geom.Regions([]geom.Curve{long, short, chShort, chLong}, nil)

		// The VALUE of Degenerate is not pinned, for the reason the other test in
		// this file gives, but the two orders must agree on it: a flag that flips
		// with input order is the same class of defect as a region count that does.
		t.Logf("r=%v shortFirst.degenerate=%v longFirst.degenerate=%v",
			r, shortFirst.Degenerate, longFirst.Degenerate)
		require.Equalf(t, shortFirst.Degenerate, longFirst.Degenerate,
			"the degeneracy verdict must not depend on which arc was passed first: r=%v", r)
		require.Lenf(t, shortFirst.Regions, 2, "short arc passed first: r=%v", r)
		require.Lenf(t, longFirst.Regions, len(shortFirst.Regions),
			"the region count must not depend on which arc was passed first: r=%v", r)

		for i := range shortFirst.Regions {
			require.InDeltaf(t, shortFirst.Regions[i].Area, longFirst.Regions[i].Area,
				math.Abs(shortFirst.Regions[i].Area)*1e-9+1e-15,
				"region %d area must not depend on input order: r=%v", i, r)
		}
	}
}

// TestWeldedParallelLinesKeepTheirRegion is the counter-example that the first
// version of the exact-tangent tie-break broke, found in review. Welding moves a
// fragment's endpoints onto other vertices, so a STRAIGHT fragment's emitted chord
// stops matching its own source direction: these two near-parallel lines weld to the
// rectangle's corners and emit one bit-identical chord while keeping different
// source tangents. Ordering that pair by their tangents reorders edges the face walk
// traverses as one segment, and the rectangle's own region collapsed.
//
// Keying a straight port by its EMITTED CHORD is what closes that class, so this test
// fails if a line is ever ordered by its source direction again. The door is
// additionally gated on at least one tied member being curved, but that gate alone
// did not close this class — see TestWeldedArcAndLineKeepBothFaces for the case it
// missed.
//
// This scene ALSO carries a coincident-emitted-edge condition: two straight
// fragments weld onto the same two graph vertices at both corners. Replacing that
// pair with one edge makes the survivor a bridge, so the tie cannot be removed
// without dropping the large region. The arrangement must keep the pair and report
// the unresolved ambiguity as degenerate.
func weldedParallelLinesScene() []geom.Curve {
	p := geom.NewPoint
	return []geom.Curve{
		geom.NewLine(p(0, 0), p(10, 0)),
		geom.NewLine(p(10, 0), p(10, 10)),
		geom.NewLine(p(10, 10), p(0, 10)),
		geom.NewLine(p(0, 10), p(0, 0)),
		geom.NewLine(p(0, 0.00071958824190816381), p(10, -0.00033642033343779087)),
		geom.NewLine(p(0, 0.00067299698496589252), p(10, 0.00049740054268733382)),
	}
}

func TestWeldedParallelLinesKeepTheirRegion(t *testing.T) {
	curves := weldedParallelLinesScene()
	arr := geom.Regions(curves, nil, geom.WithVertexMerge(0.002))
	require.True(t, arr.Degenerate)
	require.NotEmpty(t, arr.Regions)
	areas := sortedAreas(arr)
	require.InDelta(t, 100.00053587936401, areas[len(areas)-1], 0.01,
		"the unresolved bridge must not remove the large region; areas=%v", areas)
}

type regionTopologyArea struct {
	OuterEdges       int
	HoleEdges        []int
	Area             float64
	SelfIntersecting bool
	Degenerate       bool
}

func sortedRegionTopologyAreas(arr *geom.Arrangement) []regionTopologyArea {
	out := make([]regionTopologyArea, len(arr.Regions))
	for i, region := range arr.Regions {
		out[i].OuterEdges = len(region.Outer)
		out[i].Area = region.Area
		out[i].SelfIntersecting = region.SelfIntersecting
		out[i].Degenerate = region.Degenerate
		for _, hole := range region.Holes {
			out[i].HoleEdges = append(out[i].HoleEdges, len(hole))
		}
		slices.Sort(out[i].HoleEdges)
	}
	slices.SortFunc(out, func(a, b regionTopologyArea) int { return cmp.Compare(a.Area, b.Area) })
	return out
}

// TestWeldedParallelLinesMatchEveryOrder exhaustively checks the bridge scene.
// Restoring a tied straight pair must leave one bounded region with the same
// boundary topology and area in every caller order, including orders where a
// third collinear port shares the pair's angle at one endpoint.
func TestWeldedParallelLinesMatchEveryOrder(t *testing.T) {
	curves := weldedParallelLinesScene()
	base := geom.Regions(curves, nil, geom.WithVertexMerge(0.002))
	require.True(t, base.Degenerate)
	require.Len(t, base.Regions, 1)
	want := sortedRegionTopologyAreas(base)

	order := []int{0, 1, 2, 3, 4, 5}
	checked := 0
	var visit func(int)
	visit = func(pos int) {
		if pos == len(order) {
			ordered := make([]geom.Curve, len(order))
			for i, source := range order {
				ordered[i] = curves[source]
			}
			arr := geom.Regions(ordered, nil, geom.WithVertexMerge(0.002))
			require.Truef(t, arr.Degenerate, "order %v", order)
			require.Equalf(t, want, sortedRegionTopologyAreas(arr), "order %v", order)
			checked++
			return
		}
		for i := pos; i < len(order); i++ {
			order[pos], order[i] = order[i], order[pos]
			visit(pos + 1)
			order[pos], order[i] = order[i], order[pos]
		}
	}
	visit(0)
	require.Equal(t, 720, checked)
}

// TestWeldedArcAndLineKeepBothFaces is the second counter-example review found, and
// the reason a straight port is keyed by its emitted chord rather than its source
// direction. A MIXED tie — one curved member, one welded straight member — still
// opens the exact-port door, and once a ring is sorted exactly EVERY straight port in
// it is sorted that way, so the welded line was ordered by an authored ray the face
// walk never traverses. The 1.33e-12 face below was lost by the version of this fix
// that gated the door on curvature alone.
//
// The scene came out of a generated sweep, so its coordinates are kept bit-exact.
// dedupCoincidentStraightEdges also applies here: "inner" and part of "outer" weld
// onto the same two graph vertices over a short span. The canonical geometry key
// keeps the inner fragment, revealing a third sliver without dropping either of the
// two faces this regression originally protected.
func TestWeldedArcAndLineKeepBothFaces(t *testing.T) {
	p := geom.NewPoint
	inner := geom.NewLine(
		p(0.00013660030496290238, 2.4792462455065378e-06),
		p(0.0001360209071564295, 6.1409954694647045e-08),
	)
	outer := geom.NewLine(
		p(0.00013590193384173055, 1.2698037907665938e-05),
		p(0.00013649386721983748, 0),
	)
	arc := geom.NewArc(
		p(0, 0),
		p(0.00013649386721983748, 0),
		p(0.00013590193384173055, 1.2698037907665938e-05),
	)
	arr := geom.Regions([]geom.Curve{inner, outer, arc}, nil,
		geom.WithVertexMerge(6.0050600326758044e-07))
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 3)
	areas := sortedAreas(arr)
	require.InDeltaf(t, 4.706821854724171e-15, areas[0], 1e-24, "areas=%v", areas)
	require.InDeltaf(t, 7.563052133570684e-13, areas[1], 1e-24, "areas=%v", areas)
	require.InDeltaf(t, 1.3293034469770452e-12, areas[2], 1e-24, "areas=%v", areas)
}

// doubledPairScene is the adjudicator's scene B: an arc whose chord tie opens the
// exact-port door at u, plus a doubled straight pair from u to a far vertex v with a
// triangle on each side. The two u-v lines share u exactly and differ by eps at v, so
// they are one edge to the map and two to the sources. The tie and the doubling are
// at DIFFERENT vertices, which is what makes the pair reachable: the arc opens the
// door at u, and the pair is then ordered exactly at both of its ends.
func doubledPairScene(r, eps float64) []geom.Curve {
	at := func(deg float64) *geom.Point {
		rad := deg * math.Pi / 180
		return geom.NewPoint(r*math.Cos(rad), r*math.Sin(rad))
	}
	u, w, e := at(0), at(1), at(15)
	v := geom.NewPoint(u.X+2, u.Y-2)
	p := geom.NewPoint(u.X+0.5, u.Y-3)
	q := geom.NewPoint(u.X+3, u.Y+0.2)
	return []geom.Curve{
		geom.NewArc(geom.NewPoint(0, 0), u, e),
		geom.NewLine(w, u),
		geom.NewLine(e, u),
		geom.NewLine(u, v),
		geom.NewLine(geom.NewPoint(u.X, u.Y), geom.NewPoint(v.X, v.Y-eps)),
		geom.NewLine(v, p),
		geom.NewLine(p, u),
		geom.NewLine(v, q),
		geom.NewLine(q, u),
	}
}

// TestDoubledPairAnswersEveryOrderAlike pins both ORDER STABILITY and the CORRECT
// count on that scene. Before dedupCoincidentStraightEdges the engine got this class
// wrong in every order (an intermediate version of the exact-tangent tie-break
// published 4 in seven of nine orders and 3 in two); collapsing the doubled u-v pair
// to the lower-indexed source's edge removes the ambiguity at its root; the true
// face count of 4 is now reached, and reached alike in every order.
func TestDoubledPairAnswersEveryOrderAlike(t *testing.T) {
	curves := doubledPairScene(1, 2e-8)
	n := len(curves)

	orders := [][]geom.Curve{curves}
	rev := make([]geom.Curve, n)
	for i := range curves {
		rev[i] = curves[n-1-i]
	}
	orders = append(orders, rev)
	for k := 1; k < n; k++ {
		orders = append(orders, append(append([]geom.Curve(nil), curves[k:]...), curves[:k]...))
	}

	base := geom.Regions(orders[0], nil)
	require.Len(t, base.Regions, 4)
	baseAreas := sortedAreas(base)
	t.Logf("count=%d areas=%v", len(base.Regions), baseAreas)

	for i, o := range orders[1:] {
		arr := geom.Regions(o, nil)
		require.Lenf(t, arr.Regions, len(base.Regions),
			"order %d publishes a different region count than the first order", i+1)
		got := sortedAreas(arr)
		for j := range baseAreas {
			require.InDeltaf(t, baseAreas[j], got[j], math.Abs(baseAreas[j])*1e-9+1e-15,
				"order %d, region %d area differs from the first order", i+1, j)
		}
	}
}

// TestWeldedArcPortKeepsTheLargeFace is the third counter-example review found, and
// the reason a CURVED port is keyed by a direction anchored at its graph VERTEX
// rather than by its exact tangent. portKey takes that tangent at the PARAMETRIC
// endpoint, and welding moves the vertex off that point, so a fragment of chord
// length L on radius r welded by d lands on the wrong side of the chord it shares
// once L < sqrt(2*r*d). That is a threshold ordinary scenes cross, not a coincidence.
//
// This scene came from a review sweep at unit scale, not from a constructed corner:
// an arc of radius about 46.8 cut by a steep radial line, merge about 0.227. The
// version of this fix that re-keyed only STRAIGHT ports published just the
// 1.09e-05 sliver here and dropped the 1.689 face, unflagged.
func TestWeldedArcPortKeepsTheLargeFace(t *testing.T) {
	p0 := geom.NewPoint(46.755241754038238, 0)
	pN := geom.NewPoint(45.687112449242768, 9.9368197894903751)
	jp1 := geom.NewPoint(46.625632442338613, 0.80495336809061324)
	jp0 := geom.NewPoint(46.760958534174868, 0.15383907202403657)
	arr := geom.Regions([]geom.Curve{
		geom.NewLine(pN, p0),
		geom.NewLine(jp1, jp0),
		geom.NewArc(geom.NewPoint(0, 0), p0, pN),
	}, nil, geom.WithVertexMerge(0.22679552779758838))
	require.Len(t, arr.Regions, 2)
	require.InDelta(t, 1.6892916937051943, arr.Regions[0].Area, 1e-12)
	require.InDelta(t, 1.0935519663546845e-05, arr.Regions[1].Area, 1e-17)
}

// TestInnerTangentArcKeepsBothFaces is the regression that scoped the curved-port
// re-key to the SECOND door. At a certified analytic tangency contact the rotation
// system depends on every incident exact tangent being ONE ray, so sortExactPorts can
// cluster them and separate the loops by signed curvature. A vertex-anchored midpoint
// ray does not tie, the cluster breaks, and the inner arc sorts to the wrong side:
// keying every curved port that way published NO regions here, unflagged, in both
// input orders. Certified contacts therefore keep portKey's exact tangent.
func TestInnerTangentArcKeepsBothFaces(t *testing.T) {
	v := geom.NewPoint(1, 0)
	c2 := geom.NewPoint(0.5, 0)
	e := geom.NewPoint(0.5+0.5*math.Cos(15*math.Pi/180), 0.5*math.Sin(15*math.Pi/180))
	curves := []geom.Curve{geom.NewArc(c2, v, e), geom.NewLine(e, v)}
	closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), 1)}

	for _, reversed := range []bool{false, true} {
		in := curves
		if reversed {
			in = []geom.Curve{curves[1], curves[0]}
		}
		arr := geom.Regions(in, closed)
		require.Lenf(t, arr.Regions, 2, "reversed=%v", reversed)
		require.InDeltaf(t, 0.000372542837, arr.Regions[0].Area, 1e-9, "reversed=%v", reversed)
		require.InDeltaf(t, 3.14122011, arr.Regions[1].Area, 1e-7, "reversed=%v", reversed)
	}
}

// coincidentEmittedLinesScene is fu89's minimal reproduction: a 290-degree arc plus
// two near-parallel lines that share (within a jitter j) the arc's own start corner
// and run out to two DIFFERENT far points on the arc's own circle, 1 degree apart.
// The two lines genuinely intersect a few millionths of a unit from the shared
// corner, so each is cut there, and the map holds two edges — one from each line —
// between the corner vertex and that crossing vertex. Below the merge distance
// (jitter j well under it), those two stub edges weld onto the identical two graph
// vertices: the coincident-emitted-edge condition dedupCoincidentStraightEdges
// exists to collapse.
func coincidentEmittedLinesScene(j float64) []geom.Curve {
	deg := func(d float64) (float64, float64) {
		r := d * math.Pi / 180
		return 2 * math.Cos(r), 2 * math.Sin(r)
	}
	c150x, c150y := deg(150)
	c80x, c80y := deg(80)
	c340x, c340y := deg(340)
	c341x, c341y := deg(341)
	arc := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(c150x, c150y), geom.NewPoint(c80x, c80y))
	line1 := geom.NewLine(geom.NewPoint(c150x+j, c150y+j), geom.NewPoint(c340x, c340y))
	line2 := geom.NewLine(geom.NewPoint(c150x-j, c150y+j), geom.NewPoint(c341x, c341y))
	return []geom.Curve{arc, line1, line2}
}

// circularSegmentArea is the closed-form area of a circular segment of radius r
// swept through angle theta (radians): r²/2 · (theta − sin theta). Used so the big
// region's expected area is derived, not a second copy of the same literal the
// engine happens to print.
func circularSegmentArea(r, theta float64) float64 {
	return r * r / 2 * (theta - math.Sin(theta))
}

// TestCoincidentEmittedLinesKeepBothRegions pins fu89's minimal reproduction at a
// jitter inside the measured losing band ([5e-9, 1.8e-7]): below dedup, the two
// near-parallel lines' stub edges welded onto the identical pair of graph vertices,
// the fallback chord-angle sort broke that tie inconsistently between the two ends,
// and the face walk lost essentially every region (a scene worth 7.05 published
// 4.46e-10 with Degenerate false). The big region is checked against the CLOSED-FORM
// area of the 190-degree circular segment the arc-plus-chord actually bounds, not a
// second copy of the printed literal.
func TestCoincidentEmittedLinesKeepBothRegions(t *testing.T) {
	const j = 2e-8 // 14x below the scene's ~4e-7 merge distance; inside the losing band
	arr := geom.Regions(coincidentEmittedLinesScene(j), nil)
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 2)

	wantBig := circularSegmentArea(2, 190*math.Pi/180)
	areas := sortedAreas(arr)
	// The small region is the thin sliver right at the jittered corner, so its area is
	// far more sensitive to the jitter than the big region's — hence the looser delta
	// on it alone. Both stay close to the literals measured at j=0.
	require.InDelta(t, 0.0692282204591138, areas[0], 1e-6)
	require.InDelta(t, wantBig, areas[1], 1e-9)
}

type semanticBoundaryEdge struct {
	Source   int
	Whole    bool
	Reversed bool
	Polyline [][2]float64
	TStart   float64
	TEnd     float64
	TExact   bool
}

type semanticRegion struct {
	Outer            []semanticBoundaryEdge
	Holes            [][]semanticBoundaryEdge
	Area             float64
	SelfIntersecting bool
	Degenerate       bool
}

func semanticBoundary(edges []geom.BoundaryEdge, sourceAt [3]int) []semanticBoundaryEdge {
	out := make([]semanticBoundaryEdge, len(edges))
	for i, e := range edges {
		out[i] = semanticBoundaryEdge{
			Source:   sourceAt[e.SourceIndex],
			Whole:    e.Whole,
			Reversed: e.Reversed,
			Polyline: e.Polyline,
			TStart:   e.TStart,
			TEnd:     e.TEnd,
			TExact:   e.TExact,
		}
	}
	min := 0
	for i := 1; i < len(out); i++ {
		pi, pm := out[i].Polyline[0], out[min].Polyline[0]
		if c := cmp.Compare(pi[0], pm[0]); c < 0 ||
			(c == 0 && cmp.Compare(pi[1], pm[1]) < 0) ||
			(pi == pm && out[i].Source < out[min].Source) {
			min = i
		}
	}
	out = append(out[min:], out[:min]...)
	return out
}

func semanticRegionSnapshot(arr *geom.Arrangement, sourceAt [3]int) []semanticRegion {
	out := make([]semanticRegion, len(arr.Regions))
	for i, r := range arr.Regions {
		out[i] = semanticRegion{
			Outer:            semanticBoundary(r.Outer, sourceAt),
			Area:             r.Area,
			SelfIntersecting: r.SelfIntersecting,
			Degenerate:       r.Degenerate,
		}
		for _, h := range r.Holes {
			out[i].Holes = append(out[i].Holes, semanticBoundary(h, sourceAt))
		}
		slices.SortFunc(out[i].Holes, func(a, b []semanticBoundaryEdge) int {
			pa, pb := a[0].Polyline[0], b[0].Polyline[0]
			if c := cmp.Compare(pa[0], pb[0]); c != 0 {
				return c
			}
			return cmp.Compare(pa[1], pb[1])
		})
	}
	slices.SortFunc(out, func(a, b semanticRegion) int { return cmp.Compare(a.Area, b.Area) })
	return out
}

// TestCoincidentEmittedLinesMatchEveryOrder is the order-independence half of the
// fu89 regression. The same three curves, authored in every permutation, must
// publish bit-identical region and boundary fields after each SourceIndex is mapped
// back to the curve's semantic identity. It is cheap to check exhaustively at only
// three curves (six orders).
func TestCoincidentEmittedLinesMatchEveryOrder(t *testing.T) {
	curves := coincidentEmittedLinesScene(2e-8)
	perms := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}

	base := geom.Regions(curves, nil)
	require.Len(t, base.Regions, 2)
	baseRegions := semanticRegionSnapshot(base, [3]int{0, 1, 2})

	for _, perm := range perms {
		ordered := []geom.Curve{curves[perm[0]], curves[perm[1]], curves[perm[2]]}
		arr := geom.Regions(ordered, nil)
		require.Equalf(t, base.Degenerate, arr.Degenerate, "order %v", perm)
		require.Equalf(t, base.Degeneracies, arr.Degeneracies, "order %v", perm)
		require.Equalf(t, base.SelfIntersections, arr.SelfIntersections, "order %v", perm)
		require.Equalf(t, baseRegions, semanticRegionSnapshot(arr, perm), "order %v", perm)
	}
}

// TestCoincidentEmittedLinesSweep is the semantic sweep for the coincident-emitted-
// edge class beyond the minimal two-line reproduction: THREE straight fragments
// welded onto the identical pair of graph vertices (not just two), and a
// straight/curved pair sharing the same two vertices, which dedupCoincidentStraightEdges
// must leave alone since a curved fragment's chord is not interchangeable with a
// straight one's.
func TestCoincidentEmittedLinesSweep(t *testing.T) {
	t.Run("triple coincident straight edges", func(t *testing.T) {
		// Three near-parallel lines sharing an exact corner and fanning out to three
		// distinct far points on the same circle, so the map holds THREE edges (not
		// two) between the shared corner and each line's own nearby crossing with its
		// neighbors — the class dedupCoincidentStraightEdges collapses, not just the
		// pairwise case.
		deg := func(d float64) (float64, float64) {
			r := d * math.Pi / 180
			return 2 * math.Cos(r), 2 * math.Sin(r)
		}
		c150x, c150y := deg(150)
		c80x, c80y := deg(80)
		arc := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(c150x, c150y), geom.NewPoint(c80x, c80y))
		j := 2e-8
		far339x, far339y := deg(339)
		far340x, far340y := deg(340)
		far341x, far341y := deg(341)
		line1 := geom.NewLine(geom.NewPoint(c150x+j, c150y+j), geom.NewPoint(far339x, far339y))
		line2 := geom.NewLine(geom.NewPoint(c150x, c150y+j), geom.NewPoint(far340x, far340y))
		line3 := geom.NewLine(geom.NewPoint(c150x-j, c150y+j), geom.NewPoint(far341x, far341y))
		arr := geom.Regions([]geom.Curve{arc, line1, line2, line3}, nil)
		require.False(t, arr.Degenerate)
		require.NotEmpty(t, arr.Regions)
		total := 0.0
		for _, r := range arr.Regions {
			total += r.Area
		}
		// The three lines sweep 339-341 degrees, a 2-degree spread negligible against
		// the arc's own 290-degree sweep, so the published total must stay close to
		// what the two-line scene (chords to 340/341) already publishes as its total —
		// conserved area, not a face lost to the extra tie.
		two := geom.Regions(coincidentEmittedLinesScene(j), nil)
		wantTotal := 0.0
		for _, r := range two.Regions {
			wantTotal += r.Area
		}
		require.InDelta(t, wantTotal, total, 0.01)
	})

	t.Run("straight and curved sharing the same two vertices is left alone", func(t *testing.T) {
		// outer (a line) and arc share the EXACT same two endpoints — the
		// straight/curved analog of the coincident-emitted-edge condition — and must
		// NOT be collapsed: their curvature difference is real information the
		// existing exact-tangent-port door (not dedupCoincidentStraightEdges) uses to
		// separate the two faces it bounds. This is TestWeldedArcAndLineKeepBothFaces'
		// own "outer"/"arc" pair, isolated to just the two of them.
		p := geom.NewPoint
		outer := geom.NewLine(
			p(0.00013590193384173055, 1.2698037907665938e-05),
			p(0.00013649386721983748, 0),
		)
		arc := geom.NewArc(
			p(0, 0),
			p(0.00013649386721983748, 0),
			p(0.00013590193384173055, 1.2698037907665938e-05),
		)
		arr := geom.Regions([]geom.Curve{outer, arc}, nil)
		require.Len(t, arr.Regions, 1)
		require.Greater(t, arr.Regions[0].Area, 0.0)
	})
}
