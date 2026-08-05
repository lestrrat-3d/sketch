package geom_test

import (
	"math"
	"sort"
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
// (TestNaNControlCoordinateNURBSReportsNonFiniteGeometry) poisons a
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

// fitSplineCrossingSquare returns a hump-shaped fit-point spline crossing a
// 10x10 square at (0,0)-(10,10), together with the square itself, poisoning
// the fit point at index nanAt with NaN (nanAt < 0 leaves every point
// finite). Unlike a NURBS control point or a spline interior knot, a
// non-finite fit point does NOT poison every evaluated sample: newFitEvaluator
// collapses consecutive fit points closer than fitChordEps into one, and that
// comparison (math.Hypot(...) > fitChordEps) is FALSE against a NaN, so a
// non-finite point reads as "coincident with its predecessor" and is silently
// DROPPED before the evaluator computes anything. The curve then interpolates
// a different, perfectly finite curve through the remaining points, so
// densify's own evaluated-sample screen — which is what catches every other
// curve family — never sees a non-finite value to catch.
func fitSplineCrossingSquare(t *testing.T, nanAt int) []geom.Curve {
	t.Helper()
	pts := [][2]float64{{-2, 1}, {2, 9}, {5, 8}, {8, 9}, {12, 1}}
	fit := make([]*geom.Point, len(pts))
	for i, p := range pts {
		x := p[0]
		if i == nanAt {
			x = math.NaN()
		}
		fit[i] = geom.NewPoint(x, p[1])
	}
	fs, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	return append([]geom.Curve{fs}, square(0, 0, 10)...)
}

// fitSplineTailNaN is fitSplineCrossingSquare's tail-poisoned variant: every
// fit point from index 2 through the last is NaN, collapsing the curve to a
// single point rather than just truncating its last leg.
func fitSplineTailNaN(t *testing.T) []geom.Curve {
	t.Helper()
	pts := [][2]float64{{-2, 1}, {2, 9}, {5, 8}, {8, 9}, {12, 1}}
	fit := make([]*geom.Point, len(pts))
	for i, p := range pts {
		x := p[0]
		if i >= 2 {
			x = math.NaN()
		}
		fit[i] = geom.NewPoint(x, p[1])
	}
	fs, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	return append([]geom.Curve{fs}, square(0, 0, 10)...)
}

// TestNaNFitPointMakesRegionsDegenerate pins the defect this fix closes: a
// non-finite fit point is silently dropped by newFitEvaluator's own
// coincidence filter (see fitSplineCrossingSquare) rather than ever being
// sampled, so before the fix the arrangement never saw a non-finite value and
// reported a wrong-but-plausible region split (an interior NaN), a truncated
// curve reading as a whole clean square (a NaN at the last fit point), or a
// curve collapsed to a point (NaN through the whole tail) — every one of them
// with Degenerate=false. After the fix, fitSplineCoords screens the raw fit
// points before newFitEvaluator ever runs, so all three flag the arrangement
// instead.
func TestNaNFitPointMakesRegionsDegenerate(t *testing.T) {
	tests := []struct {
		name   string
		curves func(t *testing.T) []geom.Curve
	}{
		{"interior fit point", func(t *testing.T) []geom.Curve { return fitSplineCrossingSquare(t, 2) }},
		{"last fit point", func(t *testing.T) []geom.Curve { return fitSplineCrossingSquare(t, 4) }},
		{"whole tail", fitSplineTailNaN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arr := geom.Regions(tt.curves(t), nil)
			require.True(t, arr.Degenerate, "a non-finite fit point must flag the arrangement, not vanish silently")
		})
	}
}

// TestFiniteFitSplineCrossingSquareGivesTwoRegions is the healthy control: the
// same hump-shaped curve with every fit point finite cleanly splits the
// square into two regions of markedly different area (the curve dips low
// near the left edge before rising, so the split is nowhere near 50/50), and
// is NOT degenerate. This is the converged answer the NaN cases above must
// NOT silently substitute with (a clean 100-area square, or a plausible but
// wrong split).
func TestFiniteFitSplineCrossingSquareGivesTwoRegions(t *testing.T) {
	curves := fitSplineCrossingSquare(t, -1)
	arr := geom.Regions(curves, nil)
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 2, "the crossing fit spline splits the square into two regions")
	areas := make([]float64, len(arr.Regions))
	total := 0.0
	for i, r := range arr.Regions {
		areas[i] = r.Area
		total += r.Area
	}
	sort.Float64s(areas)
	require.InDelta(t, 14.22, areas[0], 0.01, "the smaller region, cut off by the low dip near the left edge")
	require.InDelta(t, 85.78, areas[1], 0.01, "the larger region, spanning the rest of the square")
	require.InDelta(t, 100, total, 1e-6, "areas still sum exactly to the whole square")
}
