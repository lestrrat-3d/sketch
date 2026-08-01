package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// fusedTripleCircles is the scene TestAnalyticFusedCurveCrossingNotBlessedExact pins:
// three r=5 circles whose (0,1) and (0,2) crossings certify while the shallow (1,2)
// crossing is below the sampling until 24 segments per turn. It is reused here as the
// carrier of a fused map, so each fixture below differs from a known-good baseline only
// in the line/arc geometry under test.
func fusedTripleCircles() []geom.ClosedCurve {
	return []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 5),
		geom.NewCircle(geom.NewPoint(0.9620341521811382, -7.264319290590297), 5),
		geom.NewCircle(geom.NewPoint(-1.343538285914439, 2.4540578044489263), 5),
	}
}

// pointOnCircle is the point at angle ang on the circle of radius r about c.
func pointOnCircle(c *geom.Point, r, ang float64) *geom.Point {
	return geom.NewPoint(c.X+r*math.Cos(ang), c.Y+r*math.Sin(ang))
}

// TestAnalyticFusedComponentWithdrawsLineAndArcBounds covers the sampled-contact
// reconciliation over source families its two existing fixtures — both all-circle scenes
// — never reach: an ARC as an operand of the refused crossing itself, and a LINE, an arc
// and an arc-and-lines WIRE drawn into the fused component through a contact.
//
// The withdrawal is per CONNECTED COMPONENT and by SOURCE, so it has to reach every kind
// of source the component contains, not just the two curves whose crossing was lost. A
// line's own polyline IS the line, so nothing about a line is ever sampled — but the
// map it helps bound is still the fused one, and a fragment of it bounded by an exact cut
// of a certified pair would describe that fused map as certified. The same holds for an
// arc that merely crosses one of the fused circles.
//
// Each scene is asserted at three levels: the region count and per-region areas against
// the converged arrangement, per-SOURCE exactness at the densities where the crossing is
// missing (which sources lose their exact bounds and which keep them), and — at every
// density from 5 to 64 — the invariant that an all-exact arrangement IS the converged one.
// The wrong region count at a coarse density is the sampled path's own limit and is not
// asserted away; publishing it as exact is what must never happen.
func TestAnalyticFusedComponentWithdrawsLineAndArcBounds(t *testing.T) {
	// Measured on every scene below: the (1,2) crossing is missing from the sampled map
	// at 8/12/16 and resolved from 24 on. The resolved list is a sample of the densities
	// at which every pair of the scene also certifies.
	fusedDensity := []int{8, 12, 16}
	resolvedDensity := []int{24, 32, 64, 256}

	for _, tc := range []struct {
		name string
		// build returns a fresh scene: Regions cuts the sources it is handed, so a
		// fixture reused across densities has to be rebuilt each time.
		build func() ([]geom.Curve, []geom.ClosedCurve)
		// fusedRegions is the region count while the crossing is missing, convergedRegions
		// the count once it is resolved — measured stable from spt=24 through spt=2048,
		// with the in-test reference arrangement taken at 512.
		fusedRegions     int
		convergedRegions int
		// withdrawn lists the sources reachable through a contact from the refused
		// crossing; keepsExact the sources that are not.
		withdrawn  []int
		keepsExact []int
	}{
		{
			// The refused crossing's own operand is an ARC: circle 2 of the triple
			// replaced by an arc on the same carrier, spanning both of its crossings.
			// So the pair the reconciliation asks about is (arc, circle), not the
			// (circle, circle) both existing fixtures ask about.
			name: "arc as an operand of the refused crossing",
			build: func() ([]geom.Curve, []geom.ClosedCurve) {
				c := geom.NewPoint(-1.343538285914439, 2.4540578044489263)
				return []geom.Curve{
					geom.NewArc(c, pointOnCircle(c, 5, -2.6), pointOnCircle(c, 5, 2.6)),
				}, fusedTripleCircles()[:2]
			},
			fusedRegions:     4,
			convergedRegions: 6,
			withdrawn:        []int{0, 1, 2},
		},
		{
			// A LINE cutting circle 0 joins the fused component through that contact.
			// Nothing about the line itself is sampled, and its bounds are exact cuts of
			// a certified line/circle pair — but they bound the fused map.
			name: "line joined to the fused component",
			build: func() ([]geom.Curve, []geom.ClosedCurve) {
				return []geom.Curve{
					geom.NewLine(geom.NewPoint(-8, -1), geom.NewPoint(8, 1)),
				}, fusedTripleCircles()
			},
			fusedRegions:     8,
			convergedRegions: 10,
			withdrawn:        []int{0, 1, 2, 3},
		},
		{
			// A closed wire of three lines and an arc, crossing circle 0 along its top
			// and bottom edges: the component now contains every analytic source kind at
			// once, joined through contacts that are themselves certified.
			name: "arc-and-lines wire joined to the fused component",
			build: func() ([]geom.Curve, []geom.ClosedCurve) {
				p1, p2 := geom.NewPoint(2, -4), geom.NewPoint(6, -4)
				p3, p4 := geom.NewPoint(6, 4), geom.NewPoint(2, 4)
				return []geom.Curve{
					geom.NewLine(p1, p2),
					geom.NewArc(geom.NewPoint(6, 0), p2, p3),
					geom.NewLine(p3, p4),
					geom.NewLine(p4, p1),
				}, fusedTripleCircles()
			},
			fusedRegions:     11,
			convergedRegions: 13,
			withdrawn:        []int{0, 1, 2, 3, 4, 5, 6},
		},
		{
			// The scope half, over line and arc sources: a line and an arc cutting a
			// circle 100 units away share no contact with the fused component, so they
			// must keep the exact bounds their own certified pairs earned. Without this
			// case a withdrawal that fired scene-wide would satisfy every assertion above.
			name: "distant line and arc keep their exact bounds",
			build: func() ([]geom.Curve, []geom.ClosedCurve) {
				c := geom.NewPoint(100, 0)
				return []geom.Curve{
						geom.NewLine(geom.NewPoint(94, 1), geom.NewPoint(106, 1)),
						geom.NewArc(c, geom.NewPoint(103, 0), geom.NewPoint(97, 0)),
					}, append(fusedTripleCircles(),
						geom.NewCircle(c, 5))
			},
			fusedRegions:     8,
			convergedRegions: 10,
			withdrawn:        []int{2, 3, 4},
			keepsExact:       []int{0, 1, 5},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refCurves, refClosed := tc.build()
			ref := geom.Regions(refCurves, refClosed, geom.WithSegmentsPerTurn(512))
			require.False(t, ref.Degenerate, "the converged arrangement is clean")
			require.Len(t, ref.Regions, tc.convergedRegions, "the converged region count")
			requireExactBoundsReproduce(t, refCurves, refClosed, ref)
			refAreas, refExact := regionAreasDescending(ref)
			require.True(t, refExact, "every bound of the converged arrangement is exact")
			refTotal, _, _ := arrangementArea(ref)

			for _, spt := range fusedDensity {
				curves, closed := tc.build()
				arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
				require.Falsef(t, arr.Degenerate, "a shallow crossing is not a degeneracy at spt=%d", spt)
				requireExactBoundsReproduce(t, curves, closed, arr)
				require.Lenf(t, arr.Regions, tc.fusedRegions,
					"the sampled path's own density limit at spt=%d — the missing crossing fuses two regions", spt)

				total, _, partial := arrangementArea(arr)
				require.Truef(t, partial, "the certified pairs still cut their sources at spt=%d", spt)
				require.InDeltaf(t, refTotal, total, 1e-9,
					"fusing two regions does not change the net area at spt=%d", spt)

				exact := exactBySource(arr)
				require.Lenf(t, exact, len(tc.withdrawn)+len(tc.keepsExact),
					"every source bounds something at spt=%d", spt)
				for _, src := range tc.withdrawn {
					require.Falsef(t, exact[src],
						"source %d is in the fused component and must publish no exact bound at spt=%d", src, spt)
				}
				for _, src := range tc.keepsExact {
					require.Truef(t, exact[src],
						"source %d shares no contact with the fused component and keeps its exact bounds at spt=%d", src, spt)
				}
			}

			for _, spt := range resolvedDensity {
				curves, closed := tc.build()
				arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
				require.Falsef(t, arr.Degenerate, "the resolved arrangement is clean at spt=%d", spt)
				requireExactBoundsReproduce(t, curves, closed, arr)
				require.Lenf(t, arr.Regions, tc.convergedRegions, "the resolved region count at spt=%d", spt)

				areas, exact := regionAreasDescending(arr)
				require.Truef(t, exact, "the resolved map keeps its exact bounds at spt=%d", spt)
				for i, want := range refAreas {
					require.InDeltaf(t, want, areas[i], 1e-9,
						"resolved region %d at spt=%d matches the converged arrangement", i, spt)
				}
			}

			// The invariant across the whole band: whatever the region count does, an
			// arrangement whose every bound reads exact has to BE the converged one.
			for spt := 5; spt <= 64; spt++ {
				curves, closed := tc.build()
				arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
				if arr.Degenerate {
					continue // conservatively refused — sound
				}
				requireExactBoundsReproduce(t, curves, closed, arr)
				total, exact, partial := arrangementArea(arr)
				if !exact || !partial {
					continue // the sampled fallback: its region count is the sampling's limit, not a verdict
				}
				require.Lenf(t, arr.Regions, tc.convergedRegions,
					"an all-exact arrangement is the converged one; spt=%d blessed a fused map", spt)
				require.InDeltaf(t, refTotal, total, 1e-9, "an all-exact arrangement nets the converged area at spt=%d", spt)
			}
		})
	}
}
