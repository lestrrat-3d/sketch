package geom_test

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// orderPermutations is every ordering of three items, the unit the weld tests
// permute: the three curves whose endpoints form the near-coincident cluster.
var orderPermutations = [6][3]int{
	{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0},
}

// TestWeldIsAuthoringOrderIndependent pins the canonical weld order: a scene whose
// near-coincident cluster SPANS MORE than the merge tolerance must publish the same
// region count and the same areas however its curves were authored.
//
// The cluster is what makes it bite. The vertex table welds a point onto the first
// vertex within the merge tolerance of it and keeps that vertex's coordinates, so for a
// cluster wider than the tolerance the choice of representative decides which members
// weld at all — and fed in authoring order, that choice was the caller's drawing order.
// Both scenes below hold three points spaced 0.9e-6 apart on a scene 10 units across,
// where the default tolerance is 1e-6: adjacent members weld, the outer two do not, so
// the representative decides whether the third curve joins the map or dangles and is
// pruned. splitFragments canonicalizes in lexicographic order instead, so the
// representative is a property of the geometry.
//
// This pins the WELD only. Order dependence upstream of it is by design and untouched:
// intersect's pair enumeration, the keep-the-first cut dedup, and the coincident-carrier
// rule that names the lower-indexed source. Neither scene has a crossing or a coincident
// carrier, so the weld is the only thing under test here.
func TestWeldIsAuthoringOrderIndependent(t *testing.T) {
	t.Run("three spokes into a triangle", func(t *testing.T) {
		// A triangle 10 units across, with a spoke from each corner to the hub near
		// (0, 1). The three hub endpoints are 0.9e-6 apart, so the outer two are
		// 1.8e-6 apart — past the 1e-6 tolerance this scene's scale derives.
		fixed := []geom.Curve{
			geom.NewLine(geom.NewPoint(-5, -5), geom.NewPoint(5, -5)),
			geom.NewLine(geom.NewPoint(5, -5), geom.NewPoint(0, 5)),
			geom.NewLine(geom.NewPoint(0, 5), geom.NewPoint(-5, -5)),
		}
		spokes := [3]geom.Curve{
			geom.NewLine(geom.NewPoint(-5, -5), geom.NewPoint(0, 1)),
			geom.NewLine(geom.NewPoint(5, -5), geom.NewPoint(0, 1+0.9e-6)),
			geom.NewLine(geom.NewPoint(0, 5), geom.NewPoint(0, 1+1.8e-6)),
		}
		requireSameRegionsEveryOrder(t, fixed, spokes)
	})

	t.Run("parallel sides with a brace", func(t *testing.T) {
		// A 10x10 square — two parallel pairs — braced along its diagonal. The left
		// side, the bottom side and the brace all start at the bottom-left corner,
		// 0.9e-6 apart along x, so the left side and the brace are 1.8e-6 apart.
		fixed := []geom.Curve{
			geom.NewLine(geom.NewPoint(10, 0), geom.NewPoint(10, 10)),
			geom.NewLine(geom.NewPoint(0, 10), geom.NewPoint(10, 10)),
		}
		corner := [3]geom.Curve{
			geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(0, 10)),
			geom.NewLine(geom.NewPoint(0.9e-6, 0), geom.NewPoint(10, 0)),
			geom.NewLine(geom.NewPoint(1.8e-6, 0), geom.NewPoint(10, 10)),
		}
		requireSameRegionsEveryOrder(t, fixed, corner)
	})
}

// requireSameRegionsEveryOrder runs geom.Regions over the fixed curves followed by the
// three permutable ones in each of the six orders, and requires every ordering to
// publish the same region count and the same sorted areas.
func requireSameRegionsEveryOrder(t *testing.T, fixed []geom.Curve, permutable [3]geom.Curve) {
	t.Helper()

	var wantCount int
	var wantAreas []float64
	var wantOrder string
	for _, perm := range orderPermutations {
		curves := append(append([]geom.Curve(nil), fixed...),
			permutable[perm[0]], permutable[perm[1]], permutable[perm[2]])
		arr := geom.Regions(curves, nil)
		requireExactBoundsReproduce(t, curves, nil, arr)
		areas := sortedAreas(arr)
		order := fmt.Sprint(perm)
		if wantAreas == nil {
			wantCount, wantAreas, wantOrder = len(arr.Regions), areas, order
			require.NotZerof(t, wantCount, "order %s must publish at least one region", order)
			continue
		}
		require.Equalf(t, wantCount, len(arr.Regions),
			"region count differs between authoring orders %s and %s", wantOrder, order)
		for i := range wantAreas {
			require.InDeltaf(t, wantAreas[i], areas[i], 1e-9,
				"region area %d differs between authoring orders %s and %s", i, wantOrder, order)
		}
	}
}

func sortedAreas(arr *geom.Arrangement) []float64 {
	areas := make([]float64, 0, len(arr.Regions))
	for _, r := range arr.Regions {
		areas = append(areas, math.Abs(r.Area))
	}
	sort.Float64s(areas)
	return areas
}
