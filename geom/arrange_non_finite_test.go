package geom_test

import (
	"encoding/json"
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

// polygonLines closes the given corners into a loop of lines sharing their
// corner points.
func polygonLines(corners ...[2]float64) []geom.Curve {
	pts := make([]*geom.Point, len(corners))
	for i, c := range corners {
		pts[i] = geom.NewPoint(c[0], c[1])
	}
	out := make([]geom.Curve, 0, len(pts))
	for i := range pts {
		out = append(out, geom.NewLine(pts[i], pts[(i+1)%len(pts)]))
	}
	return out
}

// TestRegionsOverflowedExtentIsDegenerate pins the first magnitude screen: every
// coordinate of this diamond is finite, but its bounding-box extent (2e308)
// overflows float64, so the scene scale — which the merge tolerance, the
// identity bands and the magnitude screen depend on — is not a
// measurement. Before the extent screen, the substitute scale of 1 silently
// allowed Area=+Inf, Degenerate=false and exact bounds on every edge.
func TestRegionsOverflowedExtentIsDegenerate(t *testing.T) {
	arr := geom.Regions(polygonLines(
		[2]float64{1e308, 0}, [2]float64{0, 1}, [2]float64{-1e308, 0}, [2]float64{0, -1}), nil)
	require.True(t, arr.Degenerate, "an overflowed scene extent must flag the arrangement")
	require.Len(t, arr.Regions, 1, "the region is still published, flagged rather than dropped")
	r := arr.Regions[0]
	require.True(t, math.IsInf(r.Area, 1), "area %v", r.Area)
	require.True(t, r.Degenerate, "the condition is unattributable, so it reaches the region")
	require.False(t, r.SelfIntersecting, "it is the magnitude, not the boundary")
	for i, e := range r.Outer {
		require.False(t, e.TExact, "edge %d: exact bounds are withheld with the scale", i)
	}
}

// TestRegionsOverflowedAreaFloorIsDegenerate pins the second magnitude screen:
// a scene whose extent is finite but past about 1.34e154, where both the
// scene-wide magnitude screen and the local cycle floor overflow to +Inf.
// The cycle then fails both classification comparisons. Before the screen both
// scenes published ZERO regions with
// Degenerate=false — the triangle's area is itself infinite, but the thin
// rectangle's (2e304) is finite and far above any floor, so it was a real region
// dropped with no flag at all.
func TestRegionsOverflowedAreaFloorIsDegenerate(t *testing.T) {
	tests := []struct {
		name   string
		curves []geom.Curve
	}{
		{"triangle at 1e155", polygonLines([2]float64{0, 0}, [2]float64{1e155, 0}, [2]float64{0, 1e155})},
		{"rectangle 2e154 by 1e150", polygonLines(
			[2]float64{0, 0}, [2]float64{2e154, 0}, [2]float64{2e154, 1e150}, [2]float64{0, 1e150})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arr := geom.Regions(tt.curves, nil)
			require.True(t, arr.Degenerate, "an uncomputable area floor must flag the arrangement")
			require.Empty(t, arr.Regions, "no cycle can clear an infinite floor; the flag is what reports it")
		})
	}
}

// TestRegionsLargeFiniteSceneIsUnchanged is the control for both screens above:
// one decade below the floor band, a 1e154 triangle and a 1e154-by-1e150
// rectangle publish their finite areas exactly as before, with no flag. The
// screens report a magnitude that is NOT finite; they do not narrow what a
// finite scene may measure.
func TestRegionsLargeFiniteSceneIsUnchanged(t *testing.T) {
	tests := []struct {
		name   string
		curves []geom.Curve
		area   float64
	}{
		{"triangle at 1e154", polygonLines([2]float64{0, 0}, [2]float64{1e154, 0}, [2]float64{0, 1e154}), 5e307},
		{"rectangle 1e154 by 1e150", polygonLines(
			[2]float64{0, 0}, [2]float64{1e154, 0}, [2]float64{1e154, 1e150}, [2]float64{0, 1e150}), 1e304},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arr := geom.Regions(tt.curves, nil)
			require.False(t, arr.Degenerate)
			require.Empty(t, arr.Degeneracies)
			require.Len(t, arr.Regions, 1)
			r := arr.Regions[0]
			require.InEpsilon(t, tt.area, r.Area, 1e-12)
			require.False(t, r.Degenerate)
			for i, e := range r.Outer {
				require.True(t, e.TExact, "edge %d: an all-line scene keeps its exact bounds", i)
			}
		})
	}
}

// equalRadiusLensArea is the closed-form area of the lens two EQUAL-radius circles
// cut from each other at center distance d: 2r²·acos(d/2r) − (d/2)·√((2r−d)(2r+d)).
// It is staged so no three radius-scale factors ever multiply, which is what lets it
// stay exact at the radii the two tests below use — twoCircleUnionArea's
// (−d+R+r)(d+R−r)(d−R+r)(d+R+r) form cannot, by the same overflow those tests pin.
func equalRadiusLensArea(r, d float64) float64 {
	return 2*r*r*math.Acos(d/(2*r)) - (d/2)*math.Sqrt((2*r-d)*(2*r+d))
}

// TestRegionsCircleSecantAtLargeRadiusIsExact pins the circle/circle secant's
// magnitude envelope to the ~1.34e154 scene band the rest of the arrangement
// carries (see TestRegionsLargeFiniteSceneIsUnchanged). The half-chord is a product
// of four radius-scale factors, so computing it before taking any square root
// overflows at the fourth root of MaxFloat64 — about 1.16e77, some 77 decades below
// that band. The overflow does NOT flag: the resulting (NaN, ±Inf) crossing events
// survive the sweep filter, fail certification, and the pair falls through to the
// sampled path, which publishes a WRONG lens area with Degenerate=false. Staging the
// two square roots separately keeps each argument bounded by about 4·max².
func TestRegionsCircleSecantAtLargeRadiusIsExact(t *testing.T) {
	const r = 1e80
	circles := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), r),
		geom.NewCircle(geom.NewPoint(r, 0), r),
	}
	arr := geom.Regions(nil, circles, geom.WithSegmentsPerTurn(64))
	require.False(t, arr.Degenerate, "two circles at d=r is a clean transverse crossing at any radius in band")
	require.Len(t, arr.Regions, 3, "two equal circles crossing are two lune caps plus the lens")
	areas := make([]float64, len(arr.Regions))
	for i, rg := range arr.Regions {
		areas[i] = rg.Area
		require.Falsef(t, rg.Degenerate, "region %d", i)
	}
	sort.Float64s(areas)
	require.InEpsilon(t, equalRadiusLensArea(r, r), areas[0], 1e-12,
		"the lens is the closed-form area, not the sampled fallback's")
}

// TestRegionsArcSecantAtLargeRadiusPublishesTheLens is the ARC face of the same
// overflow. The operand kind changes the symptom, not the cause: for arcs the sweep
// filter drops the non-finite crossing events instead of passing them on, a
// downstream gate catches the loss, and the scene reads Degenerate with NO region at
// all — where the circle pair above reads clean and wrong. One formulation fixes
// both, so both are pinned; a change that reopens only one half must fail here.
func TestRegionsArcSecantAtLargeRadiusPublishesTheLens(t *testing.T) {
	const r = 1e80
	c1, c2 := geom.NewPoint(0, 0), geom.NewPoint(r, 0)
	at := func(c *geom.Point, ang float64) *geom.Point {
		return geom.NewPoint(c.X+r*math.Cos(ang), c.Y+r*math.Sin(ang))
	}
	// Half-circle sweeps: each crossing (at ±60° on c1, 120°/240° on c2) is interior
	// to both arcs, so the lens is bounded and the four tails are open chains.
	arcs := []geom.Curve{
		geom.NewArc(c1, at(c1, -math.Pi/2), at(c1, math.Pi/2)),
		geom.NewArc(c2, at(c2, math.Pi/2), at(c2, 3*math.Pi/2)),
	}
	arr := geom.Regions(arcs, nil, geom.WithSegmentsPerTurn(64))
	require.False(t, arr.Degenerate, "the two arcs cross transversally twice")
	require.Len(t, arr.Regions, 1, "the only bounded face is the lens")
	require.False(t, arr.Regions[0].Degenerate)
	require.InEpsilon(t, equalRadiusLensArea(r, r), arr.Regions[0].Area, 1e-12)
}

// TestRegionsCircleSecantOverflowedCenterIsFinite pins the companion half of the
// staged half-chord above. Staging bounds `half`, but the radical-line offset
// `aDist = (d² + a.r² − b.r²)/(2d)` still forms d² and so overflows above
// sqrt(MaxFloat64) — here d is 1e154, and aDist is +Inf while half stays finite
// at ~1.64e153. A guard that tested only `half` therefore passed an (+Inf, NaN)
// center straight through into an emitted crossing, which reached
// Arrangement.Degeneracies and made the whole published Arrangement
// unmarshalable ("json: unsupported value: +Inf"). Testing the emitted center
// instead turns it back into the same ambiguous refusal analyticPrepass already
// records.
//
// NOTE on what is deliberately NOT asserted: with the guard this scene publishes
// one region whose area is exactly the UNCUT small disk, π·(2e153)². That is
// geometrically wrong — the two circles genuinely cross, so the small disk should
// be cut — and it is reported as such: both the arrangement and the region carry
// Degenerate=true, telling a consumer not to trust it. The contract this test
// pins is that every published magnitude is finite and the report survives
// serialization, NOT that the area is correct.
func TestRegionsCircleSecantOverflowedCenterIsFinite(t *testing.T) {
	circles := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 9e153),
		geom.NewCircle(geom.NewPoint(1e154, 0), 2e153),
	}
	arr := geom.Regions(nil, circles, geom.WithSegmentsPerTurn(64))
	require.True(t, arr.Degenerate, "the pair is past the magnitude band and must be flagged")
	require.NotEmpty(t, arr.Degeneracies, "the refusal is reported, not silent")
	for i, d := range arr.Degeneracies {
		require.Falsef(t, math.IsInf(d[0], 0) || math.IsNaN(d[0]), "degeneracy %d x = %v", i, d[0])
		require.Falsef(t, math.IsInf(d[1], 0) || math.IsNaN(d[1]), "degeneracy %d y = %v", i, d[1])
	}
	require.Len(t, arr.Regions, 1, "the flagged small disk is still published")
	rg := arr.Regions[0]
	require.True(t, rg.Degenerate, "the region carries the flag, so a consumer is told not to trust its area")
	require.InEpsilon(t, math.Pi*2e153*2e153, rg.Area, 1e-12,
		"the uncut small disk — reported under the flag, not a correct lens split")
	_, err := json.Marshal(arr)
	require.NoError(t, err, "a non-finite degeneracy point makes the whole Arrangement unmarshalable")
}
