package geom_test

import (
	"sort"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// weldedTieScene is the mechanism this file is about, built from it rather than
// sampled from a sweep: a 10x10 box crossed by n ties whose endpoints fall into two
// clusters, one on each side of the box, each cluster narrower than the merge
// distance. Every tie is therefore cut at the SAME two welded graph vertices and
// emits one fragment between them, so the map holds n edges where the traversed
// geometry has one. Their emitted chords are bit-identical, so the rotation sort's
// fallback key — the chord departure angle — is bit-identical too, and sort.Slice is
// unstable, so which tie the ring puts first comes from the caller's input order.
//
// The ties are separated by 3e-4 against a merge distance of 1e-3, far enough apart
// that no collinear-overlap or near-tangency classification fires on them: nothing
// else in the engine reports this scene.
func weldedTieScene(n int) []geom.Curve {
	p := geom.NewPoint
	curves := []geom.Curve{
		geom.NewLine(p(0, 0), p(10, 0)),
		geom.NewLine(p(10, 0), p(10, 10)),
		geom.NewLine(p(10, 10), p(0, 10)),
		geom.NewLine(p(0, 10), p(0, 0)),
	}
	for i := 0; i < n; i++ {
		y := 5 + 3e-4*float64(i)
		curves = append(curves, geom.NewLine(p(0, y), p(10, y)))
	}
	return curves
}

// inputOrders returns the scene in its own order, reversed, and in every rotation.
func inputOrders(curves []geom.Curve) [][]geom.Curve {
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
	return orders
}

// TestWeldedTiesReportDegenerateInEveryOrder pins the flag on the scene above, at two
// ties and at three, in every input order.
//
// What the scene publishes is still wrong and still order-dependent, and this test
// deliberately does not pin it: the two ties cut the box in two, and the arrangement
// reports the box alone. Before this check the same scene read Degenerate=false, so a
// consumer had nothing to branch on; the counts and areas are unchanged by it, and
// the repair that would make them right is the map holding one edge where the
// geometry has one.
func TestWeldedTiesReportDegenerateInEveryOrder(t *testing.T) {
	for _, ties := range []int{2, 3} {
		scene := weldedTieScene(ties)
		for i, o := range inputOrders(scene) {
			arr := geom.Regions(o, nil, geom.WithVertexMerge(1e-3))
			require.Truef(t, arr.Degenerate,
				"%d ties, input order %d: two sources emit one chord and nothing reported it", ties, i)
			require.NotEmptyf(t, arr.Degeneracies, "%d ties, input order %d", ties, i)
			t.Logf("ties=%d order=%d regions=%d degeneracies=%d", ties, i, len(arr.Regions), len(arr.Degeneracies))
		}
	}
}

// TestSeparatedTiesAreNotFlagged is the control for the test above: the same scene
// with the ties 20x the merge distance apart welds nothing, so each tie reaches its
// own pair of graph vertices, the box is cut into three regions and no degeneracy is
// reported. Without it the test above would pass against a check that flagged
// everything.
func TestSeparatedTiesAreNotFlagged(t *testing.T) {
	p := geom.NewPoint
	curves := append(weldedTieScene(0),
		geom.NewLine(p(0, 4), p(10, 4)),
		geom.NewLine(p(0, 6), p(10, 6)),
	)
	for i, o := range inputOrders(curves) {
		arr := geom.Regions(o, nil, geom.WithVertexMerge(1e-3))
		require.Falsef(t, arr.Degenerate, "input order %d", i)
		require.Lenf(t, arr.Regions, 3, "input order %d", i)
	}
}

// TestCoincidentEdgeVerdictMatchesEveryOrder pins the property the check exists to
// provide: its own verdict does not depend on the order the curves were passed in.
// A detector keyed on anything the input order decides — an edge index, which of a
// group's edges arrived first — reports a different set of conditions per order, and
// a degeneracy verdict that moves with input order is the same class of defect as a
// region count that does.
//
// Three ties is the case that separates the two: the vertex pair then carries three
// edges rather than two, so a detector that blames only the first arrival against
// each later one attributes a different pair of sources per order, while one keyed on
// the unordered vertex pair and the ports' own departure keys does not.
func TestCoincidentEdgeVerdictMatchesEveryOrder(t *testing.T) {
	scene := weldedTieScene(3)

	var wantDegenerate bool
	var wantPoints [][2]float64
	for i, o := range inputOrders(scene) {
		arr := geom.Regions(o, nil, geom.WithVertexMerge(1e-3))
		got := append([][2]float64(nil), arr.Degeneracies...)
		sort.Slice(got, func(a, b int) bool {
			if got[a][0] != got[b][0] {
				return got[a][0] < got[b][0]
			}
			return got[a][1] < got[b][1]
		})
		if i == 0 {
			wantDegenerate, wantPoints = arr.Degenerate, got
			require.True(t, wantDegenerate, "the scene under test must reach the check")
			continue
		}
		require.Equalf(t, wantDegenerate, arr.Degenerate, "input order %d", i)
		require.Lenf(t, got, len(wantPoints), "input order %d: degeneracy count", i)
		for k := range wantPoints {
			require.InDeltaf(t, wantPoints[k][0], got[k][0], 1e-9, "input order %d, degeneracy %d x", i, k)
			require.InDeltaf(t, wantPoints[k][1], got[k][1], 1e-9, "input order %d, degeneracy %d y", i, k)
		}
	}
}
