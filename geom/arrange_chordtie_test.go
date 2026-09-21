package geom_test

import (
	"math"
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
// traverses as one segment, and the rectangle's own region collapsed — 0 regions
// where main publishes one of area 100.00053587936401.
//
// Keying a straight port by its EMITTED CHORD is what closes it, so this test fails
// if a line is ever ordered by its source direction again. The door is additionally
// gated on at least one tied member being curved, but that gate alone did not close
// this class — see TestWeldedArcAndLineKeepBothFaces for the case it missed.
//
// What it pins is the LOSS, not a correct answer for this scene. The base itself is
// order-dependent here — over 12 input orders it returns one region in 4 and none in
// 8 — and the scene holds two pairs of edges that are emitted coincident with nothing
// flagging them, so one region is not established as right either. The assertion
// therefore fixes one order of a scene the engine cannot yet answer consistently;
// that underlying gap is tracked separately, and a coincident-emitted-edge check in
// buildGraph is its root fix.
func TestWeldedParallelLinesKeepTheirRegion(t *testing.T) {
	p := geom.NewPoint
	curves := []geom.Curve{
		geom.NewLine(p(0, 0), p(10, 0)),
		geom.NewLine(p(10, 0), p(10, 10)),
		geom.NewLine(p(10, 10), p(0, 10)),
		geom.NewLine(p(0, 10), p(0, 0)),
		geom.NewLine(p(0, 0.00071958824190816381), p(10, -0.00033642033343779087)),
		geom.NewLine(p(0, 0.00067299698496589252), p(10, 0.00049740054268733382)),
	}
	arr := geom.Regions(curves, nil, geom.WithVertexMerge(0.002))
	require.Len(t, arr.Regions, 1)
	require.InDelta(t, 100.00053587936401, arr.Regions[0].Area, 1e-9)
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
// Both areas match what main publishes; what this pins is that neither face is lost.
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
	require.Len(t, arr.Regions, 2)
	require.InDelta(t, 1.3340102688318081e-12, arr.Regions[0].Area, 1e-24)
	require.InDelta(t, 7.5630521335706837e-13, arr.Regions[1].Area, 1e-24)
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

// TestDoubledPairAnswersEveryOrderAlike pins ORDER STABILITY on that scene, and
// deliberately does not pin the count. The engine gets this class wrong: the true
// face count is 4 and every build published something else, so asserting a count here
// would make a wrong answer load-bearing and the eventual repair would have to delete
// it. What this change did achieve is that the answer no longer moves with input
// order — the intermediate version published 4 in seven orders and 3 in two — so that
// is what is asserted, and the count is logged for whoever fixes it.
//
// Reaching the true 4 needs the map to stop holding two edges where the geometry has
// one, which is a coincident-emitted-edge check in buildGraph and its own follow-up.
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
	baseAreas := sortedAreas(base)
	t.Logf("count=%d areas=%v (logged, not asserted: the true count is 4)", len(base.Regions), baseAreas)

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
