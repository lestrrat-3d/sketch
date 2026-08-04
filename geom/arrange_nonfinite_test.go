package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// nanKnotCrossingNURBS returns a degree-3 NURBS whose interior knot is NaN,
// together with control points collinear along y=5, so the curve is an exact
// straight line from (-2,5) to (12,5) — crossing a 10x10 square at
// x0,y0=(0,0) clean through the middle. NewNURBS validates nothing by design,
// which is how a NaN knot reaches this layer; the sketch layer's CreateNURBS
// rejects a non-finite knot outright, so its own end-to-end fixture
// (TestNaNControlCoordinateNURBSCrossingSquareIsInvalidProfile) poisons a
// control-point COORDINATE instead and reaches the same arrangement.
func nanKnotCrossingNURBS(t *testing.T, nan bool) *geom.NURBS {
	t.Helper()
	ctrl := []*geom.Point{
		geom.NewPoint(-2, 5), geom.NewPoint(1, 5), geom.NewPoint(4, 5),
		geom.NewPoint(7, 5), geom.NewPoint(12, 5),
	}
	knots := geom.ClampedUniformKnots(len(ctrl), 3)
	require.Equal(t, []float64{0, 0, 0, 0, 0.5, 1, 1, 1, 1}, knots, "interior knot at index 4")
	if nan {
		knots[4] = math.NaN()
	}
	return geom.NewNURBS(3, ctrl, knots, nil)
}

// TestNaNKnotNURBSMakesRegionsDegenerate pins the defect this fix closes: a
// NURBS whose interior knot is NaN evaluates to NaN at every sample (every
// ordered comparison against NaN is false), so before the fix it contributed no
// vertex, cut or edge to the arrangement at all — it vanished, and the square it
// crosses was reported as ONE clean region of area 100 with Degenerate=false.
//
// After the fix, densify samples the whole source before trusting it, finds the
// non-finite samples, and drops the source as degenerate — flagging the
// arrangement rather than silently reporting the wrong region count.
func TestNaNKnotNURBSMakesRegionsDegenerate(t *testing.T) {
	nb := nanKnotCrossingNURBS(t, true)
	curves := append([]geom.Curve{nb}, square(0, 0, 10)...)
	arr := geom.Regions(curves, nil)
	require.True(t, arr.Degenerate, "a NaN-knot source must flag the arrangement, not vanish silently")
}

// TestFiniteControlNURBSCrossingSquareGivesTwoRegions is the healthy control:
// the same crossing curve with an ordinary finite interior knot cleanly splits
// the square into two ~50 area regions, and is NOT degenerate. This is the
// converged answer the NaN-knot case above must NOT silently produce (one region
// of area 100).
func TestFiniteControlNURBSCrossingSquareGivesTwoRegions(t *testing.T) {
	nb := nanKnotCrossingNURBS(t, false)
	curves := append([]geom.Curve{nb}, square(0, 0, 10)...)
	arr := geom.Regions(curves, nil)
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 2, "the crossing line splits the square into two halves")
	// The crossing is a sampled (not analytic) NURBS/line contact, so its cut
	// parameter converges with sampling rather than being exact — hence the
	// looser tolerance than a closed-form crossing would need.
	total := 0.0
	for _, r := range arr.Regions {
		require.InDelta(t, 50, r.Area, 0.01, "each half is ~10 x 5")
		total += r.Area
	}
	require.InDelta(t, 100, total, 1e-6, "areas still sum exactly to the whole square")
}
