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
