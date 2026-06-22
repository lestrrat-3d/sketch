package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// Guards the per-fragment bulge correction: a fragment that does not start at
// t=0 must contribute the area between its sub-arc and ITS chord — NOT the moment
// swept from the conic's start point (which over-counts by triangle(start, P(t0),
// P(t1))). Without that triangle subtraction the split cap is off by ~4 units.
//
// The cut is the vertical line through the parabola's symmetric apex (x=2): by
// symmetry the crossing lands at the exact split parameter (t=0.5) at every
// sampling, which ISOLATES the bulge correction from the sampled-crossing drift.
// A general (asymmetric) line/conic split has an approximate cut parameter, so
// its area only CONVERGES with sampling — like a split ellipse/spline — and is
// not asserted exact here. So this is a bulge-correction guard, not a claim that
// every split conic is exact.
func TestConicSplitBulgeNetsWholeCap(t *testing.T) {
	// Parabola cap: start (0,0), apex (2,4), end (4,0), rho 0.5 (w=1).
	// Cap area = |(a×b)|/3 with a=(2,4), b=(4,0): |2·0−4·4|/3 = 16/3.
	const want = 16.0 / 3.0
	start := geom.NewPoint(0, 0)
	apex := geom.NewPoint(2, 4)
	end := geom.NewPoint(4, 0)

	for _, spt := range []int{32, 64, 128, 256} {
		conic := geom.NewConic(start, apex, end, 0.5)
		base := geom.NewLine(end, start)                                 // the chord, closing the cap
		split := geom.NewLine(geom.NewPoint(2, -1), geom.NewPoint(2, 5)) // vertical, crosses conic + base
		arr := geom.Regions([]geom.Curve{conic, base, split}, nil, geom.WithSegmentsPerTurn(spt))

		var total float64
		for _, r := range arr.Regions {
			total += math.Abs(r.Area)
		}
		require.InDeltaf(t, want, total, 1e-9,
			"split parabola cap nets the whole-cap area at spt=%d (got %.6f)", spt, total)
	}
}
