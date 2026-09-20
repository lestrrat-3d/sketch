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

// TestWeldRepresentativeBitsAreOrderIndependent pins the half of the canonical weld
// order that an ordinary float compare cannot see: the COORDINATES the shared vertex is
// published with, compared bit for bit.
//
// The lexicographic pre-pass orders boundary points by (x, y), and for float64 the ONE
// pair of distinct values that compares equal on both coordinates is a negative zero
// against a positive zero — every other distinct pair is separated by `<` on one
// coordinate or the other. A tie leaves the relative order to slices.SortFunc, which is
// unstable, so the representative of that cluster still depended on the order the caller
// authored the curves in, and canon keeps the representative's coordinates.
//
// Reaching the vertex table with the sign intact is what each scene below has to
// arrange, and the two sources do it differently. An elliptical arc PINS its ends to the
// authored Start/End, so its coordinate arrives verbatim. A line's is recomputed as
// ax + t*(bx-ax), which at t=0 keeps a negative zero only when the direction component
// is itself negative (-0 + -0 = -0) and loses it when the line runs the other way
// (-0 + +0 = +0) — so a line scene reproduces this on one direction and not on the
// other, and a test built from lines drawn the wrong way passes against the untied
// comparator and proves nothing.
//
// Both scenes publish the same region count and the same area in either order, so the
// published vertex is the only thing that separates them.
func TestWeldRepresentativeBitsAreOrderIndependent(t *testing.T) {
	negZero := math.Copysign(0, -1)

	t.Run("arc pinned to a negative-zero start", func(t *testing.T) {
		// A half disk: the arc's start is authored at (-0, -0) and the closing line's
		// end at (+0, +0). Before the bit tie-break this published
		// 0x8000000000000000 in order [arc, line] and 0x0 in order [line, arc].
		arc := geom.NewEllipticalArc(
			geom.NewPoint(1, 0), geom.NewPoint(negZero, negZero), geom.NewPoint(2, 0), 1, 1, 0)
		line := geom.NewLine(geom.NewPoint(2, 0), geom.NewPoint(0, 0))
		requireSameVertexBits(t,
			[]geom.Curve{arc, line}, []geom.Curve{line, arc})
	})

	t.Run("line drawn away from a negative-zero start", func(t *testing.T) {
		// A triangle with its apex at the origin, reached by two lines that both run
		// AWAY from it in negative x: the lerp then keeps the authored sign, so the
		// left line delivers (-0, -0) and the right one (+0, +0).
		left := geom.NewLine(geom.NewPoint(negZero, negZero), geom.NewPoint(-1, -1))
		right := geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(1, -1))
		base := geom.NewLine(geom.NewPoint(-1, -1), geom.NewPoint(1, -1))
		requireSameVertexBits(t,
			[]geom.Curve{left, right, base}, []geom.Curve{right, left, base})
	})
}

// requireSameVertexBits requires two authoring orders of the same drawing to publish one
// region of the same area with bit-identical boundary vertices.
func requireSameVertexBits(t *testing.T, forward, reversed []geom.Curve) {
	t.Helper()

	a := geom.Regions(forward, nil, geom.WithVertexMerge(1e-6))
	b := geom.Regions(reversed, nil, geom.WithVertexMerge(1e-6))
	require.Len(t, a.Regions, 1, "the drawing is one region in its first authoring order")
	require.Len(t, b.Regions, 1, "the drawing is one region in its reversed authoring order")
	require.InDelta(t, math.Abs(a.Regions[0].Area), math.Abs(b.Regions[0].Area), 1e-12,
		"the region has the same area in either authoring order")
	require.Equal(t, publishedVertexBits(a), publishedVertexBits(b),
		"the welded vertex must carry the same coordinate BITS in either authoring order")
}

// publishedVertexBits returns the raw bit pattern of every boundary-edge endpoint the
// arrangement publishes, as sorted "x,y" pairs. Sorting makes it comparable across
// authoring orders, which renumber the sources and so reorder the edges; the bits make
// it sensitive to a signed zero, which `==` is not.
func publishedVertexBits(arr *geom.Arrangement) []string {
	var bits []string
	note := func(loop []geom.BoundaryEdge) {
		for _, e := range loop {
			for _, p := range [][2]float64{e.Polyline[0], e.Polyline[len(e.Polyline)-1]} {
				bits = append(bits, fmt.Sprintf("%#016x,%#016x",
					math.Float64bits(p[0]), math.Float64bits(p[1])))
			}
		}
	}
	for _, r := range arr.Regions {
		note(r.Outer)
		for _, h := range r.Holes {
			note(h)
		}
	}
	sort.Strings(bits)
	return bits
}
