package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// These tests lock in a soundness invariant confirmed by an adversarial audit: a
// curve that GENUINELY self-crosses (two far-apart branches meet) must never be
// blessed as a single clean simple region. The arrangement may either flag it
// (SelfIntersecting / Degenerate) or subdivide it at the crossing into the correct
// multiple regions — but it must not report one simple loop whose area ignores the
// crossing. The oracle's prime directive: never bless an actually-invalid profile.

// blessedClean reports whether the arrangement treated the input as a single clean
// simple region (no self-intersection signal, no degeneracy, exactly one region).
func blessedSingleClean(arr *geom.Arrangement) bool {
	if arr.Degenerate || len(arr.SelfIntersections) > 0 || len(arr.Regions) != 1 {
		return false
	}
	return !arr.Regions[0].SelfIntersecting
}

func TestAuditClosedSplineSelfCrossNeverBlessedSimple(t *testing.T) {
	// A "bowtie" control polygon makes the periodic cubic cross itself once — a real
	// figure-8. Across every sampling density it must be flagged, never blessed as one
	// clean simple region (which would report a wrong, crossing-ignoring area).
	mk := func() geom.ClosedCurve {
		cs, err := geom.NewClosedSpline(
			geom.NewPoint(-3, -2), geom.NewPoint(3, 2),
			geom.NewPoint(-3, 2), geom.NewPoint(3, -2))
		require.NoError(t, err)
		return cs
	}
	for _, spt := range []int{4, 6, 8, 12, 16, 32, 64, 128} {
		arr := geom.Regions(nil, []geom.ClosedCurve{mk()}, geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, blessedSingleClean(arr),
			"self-crossing closed spline blessed as one clean region at spt=%d", spt)
	}
}

func TestAuditSelfCrossingSplinesScanNeverBlessedSimple(t *testing.T) {
	// Sweep the fourth control point of a closed spline. For every configuration whose
	// curve GENUINELY self-crosses (verified by a far-branch closest-approach probe on
	// a dense sampling), no sampling density may bless it as a single clean simple
	// region. (Near-cusps — tight turns that do NOT cross — are excluded, since coarse
	// blessing those is correct and dense flagging them is merely conservative.)
	control := func(px, py float64) [][2]float64 {
		return [][2]float64{{-3, -2}, {3, 2}, {-3, 2}, {px, py}}
	}
	realSelfCross := func(ctrl [][2]float64) bool {
		const n = 2000
		pts := make([][2]float64, n)
		for i := 0; i < n; i++ {
			x, y, _ := geom.EvalPeriodicCubicBSpline(ctrl, float64(i)/float64(n))
			pts[i] = [2]float64{x, y}
		}
		sep := n / 8 // cyclic separation > 0.125 turn → genuinely far-apart branches
		min := math.Inf(1)
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				d := j - i
				if d > n-d {
					d = n - d
				}
				if d < sep {
					continue
				}
				dist := math.Hypot(pts[i][0]-pts[j][0], pts[i][1]-pts[j][1])
				if dist < min {
					min = dist
				}
			}
		}
		return min < 1e-3
	}

	checked := 0
	for xi := -10; xi <= 10; xi += 2 {
		for yi := -16; yi <= 4; yi += 2 {
			px, py := float64(xi)*0.5, float64(yi)*0.5
			ctrl := control(px, py)
			if !realSelfCross(ctrl) {
				continue
			}
			checked++
			cs, err := geom.NewClosedSpline(
				geom.NewPoint(ctrl[0][0], ctrl[0][1]), geom.NewPoint(ctrl[1][0], ctrl[1][1]),
				geom.NewPoint(ctrl[2][0], ctrl[2][1]), geom.NewPoint(ctrl[3][0], ctrl[3][1]))
			require.NoError(t, err)
			for _, spt := range []int{8, 16, 32, 64} {
				arr := geom.Regions(nil, []geom.ClosedCurve{cs}, geom.WithSegmentsPerTurn(spt))
				require.Falsef(t, blessedSingleClean(arr),
					"self-crossing spline p4=(%.1f,%.1f) blessed clean at spt=%d", px, py, spt)
			}
		}
	}
	require.Greater(t, checked, 10, "expected the sweep to cover several self-crossing configurations")
}
