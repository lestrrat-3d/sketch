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
		require.Falsef(t, arr.Degenerate, "r=%v", r)
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

		require.Falsef(t, shortFirst.Degenerate, "r=%v", r)
		require.Falsef(t, longFirst.Degenerate, "r=%v", r)
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
