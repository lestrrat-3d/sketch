package geom_test

import (
	"math"
	"math/rand"
	"sort"
	"strconv"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// TestRegionsArcLensDoesNotAdoptDisjointCircleAsHole pins the minimal
// reproduction that corroborated the arc-parameter-aliasing defect: two arcs
// forming a small lens, plus a circle whose bounding box is fully disjoint from
// the lens's. angInFragment's periodic wrap used to bring an arc-crossing angle
// into the fragment's own [pStart, pEnd) range by whole UNITS of the fragment's
// natural parameter (one unit = the arc's own sweep) rather than by whole
// PHYSICAL turns (2π), so a ray-cast root at an angle genuinely off the
// fragment's sweep aliased onto it and the far circle was wrongly published as
// the lens's hole — subtracting its area from the lens and double-publishing
// the circle's own area as a second region.
func TestRegionsArcLensDoesNotAdoptDisjointCircleAsHole(t *testing.T) {
	arc1 := geom.NewArc(
		geom.NewPoint(0.35092155472478259, -0.92392819187699393),
		geom.NewPoint(0.91737230032440398, -1.1556555089536711),
		geom.NewPoint(-0.17199298451132139, -0.6059275923092966),
	)
	arc2 := geom.NewArc(
		geom.NewPoint(0.34326886354944652, -0.6557519008634316),
		geom.NewPoint(0.32461533217526556, -1.0464592649851203),
		geom.NewPoint(-0.022307692217990949, -0.51663331660933887),
	)
	circle := geom.NewCircle(geom.NewPoint(-0.81323037456758496, -0.91951610002249584), 0.068365088974454882)

	arr := geom.Regions([]geom.Curve{arc1, arc2}, []geom.ClosedCurve{circle})
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 2, "the lens and the circle, each its own region")

	var lens *geom.Region
	for _, reg := range arr.Regions {
		if len(reg.Outer) == 2 { // the lens has two arc edges; the circle has one
			lens = reg
		}
	}
	require.NotNil(t, lens, "the two-arc lens must be one of the published regions")
	require.Empty(t, lens.Holes, "the disjoint circle must never be adopted as the lens's hole")
	require.InDelta(t, 0.01843175453393291, lens.Area, 1e-9)
}

// polyOf flattens a boundary (outer loop or one hole loop) into a single closed
// polyline, the way a consumer checking containment independently would.
func polyOf(edges []geom.BoundaryEdge) [][2]float64 {
	var out [][2]float64
	for _, e := range edges {
		for i, p := range e.Polyline {
			if i == 0 && len(out) > 0 {
				continue // edges share their join vertex
			}
			out = append(out, p)
		}
	}
	return out
}

// boundsOf returns the axis-aligned bounding box of a polyline.
func boundsOf(pts [][2]float64) (float64, float64, float64, float64) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		minX, minY = math.Min(minX, p[0]), math.Min(minY, p[1])
		maxX, maxY = math.Max(maxX, p[0]), math.Max(maxY, p[1])
	}
	return minX, minY, maxX, maxY
}

// boxesDisjoint reports whether two axis-aligned boxes share no point at all.
func boxesDisjoint(aMinX, aMinY, aMaxX, aMaxY, bMinX, bMinY, bMaxX, bMaxY float64) bool {
	return aMaxX < bMinX || bMaxX < aMinX || aMaxY < bMinY || bMaxY < aMinY
}

// holeSweepScene builds one random line/arc/circle scene, mirroring the
// investigation's differential sweep that found the defect (2 to 5 lines, 0 to
// 2 circles, 0 to 2 arcs, at one of three scene scales).
func holeSweepScene(rng *rand.Rand, span float64) ([]geom.Curve, []geom.ClosedCurve) {
	r := func() float64 { return (rng.Float64()*2 - 1) * span }
	var curves []geom.Curve
	for i, n := 0, 2+rng.Intn(4); i < n; i++ {
		curves = append(curves, geom.NewLine(geom.NewPoint(r(), r()), geom.NewPoint(r(), r())))
	}
	for i, n := 0, rng.Intn(3); i < n; i++ {
		cx, cy := r(), r()
		rad := rng.Float64() * span
		a0 := rng.Float64() * 2 * math.Pi
		a1 := a0 + rng.Float64()*2*math.Pi
		curves = append(curves, geom.NewArc(geom.NewPoint(cx, cy),
			geom.NewPoint(cx+rad*math.Cos(a0), cy+rad*math.Sin(a0)),
			geom.NewPoint(cx+rad*math.Cos(a1), cy+rad*math.Sin(a1))))
	}
	var closed []geom.ClosedCurve
	for i, n := 0, rng.Intn(3); i < n; i++ {
		closed = append(closed, geom.NewCircle(geom.NewPoint(r(), r()), rng.Float64()*span))
	}
	return curves, closed
}

// TestRegionsHoleNeverDisjointFromItsFace is the seeded sweep the fix must
// pass: no published hole's bounding box may be disjoint from the bounding box
// of the face it was assigned to. A disjoint pair is exactly the failure mode
// the arc-parameter-aliasing defect produced (a hole's polyline entirely
// outside its face), so this is a direct regression net for it, over a fixed,
// deterministic sample of the same scene family (lines/arcs/circles at three
// scales) the original 6000-scene differential sweep used. Kept small enough
// to run in well under a second.
func TestRegionsHoleNeverDisjointFromItsFace(t *testing.T) {
	rng := rand.New(rand.NewSource(20260922))
	scenes := 400
	scales := []float64{1, 10, 1000}
	violations := 0
	for i := 0; i < scenes; i++ {
		curves, closed := holeSweepScene(rng, scales[i%len(scales)])
		arr := geom.Regions(curves, closed)
		for _, reg := range arr.Regions {
			if len(reg.Holes) == 0 {
				continue
			}
			fMinX, fMinY, fMaxX, fMaxY := boundsOf(polyOf(reg.Outer))
			for _, hole := range reg.Holes {
				hMinX, hMinY, hMaxX, hMaxY := boundsOf(polyOf(hole))
				if boxesDisjoint(fMinX, fMinY, fMaxX, fMaxY, hMinX, hMinY, hMaxX, hMaxY) {
					violations++
					t.Errorf("scene %d: hole bbox [%v,%v]..[%v,%v] disjoint from face bbox [%v,%v]..[%v,%v]",
						i, hMinX, hMinY, hMaxX, hMaxY, fMinX, fMinY, fMaxX, fMaxY)
				}
			}
		}
	}
	require.Zero(t, violations)
}

// disjointTrianglesScene builds the containment scene: a face triangle, a
// second triangle whose whole bounding box sits gap below the face's minY, and
// one open line parked at x=1e9 touching neither.
func disjointTrianglesScene(gap float64) ([]geom.Curve, []geom.Curve, geom.Curve) {
	face := []geom.Curve{
		geom.NewLine(geom.NewPoint(-1100000, 0), geom.NewPoint(1100000, 8)),
		geom.NewLine(geom.NewPoint(1100000, 8), geom.NewPoint(-1100000, 12)),
		geom.NewLine(geom.NewPoint(-1100000, 12), geom.NewPoint(-1100000, 0)),
	}
	hole := []geom.Curve{
		geom.NewLine(geom.NewPoint(-1000000, -gap), geom.NewPoint(1000000, -gap)),
		geom.NewLine(geom.NewPoint(1000000, -gap), geom.NewPoint(0, 2)),
		geom.NewLine(geom.NewPoint(0, 2), geom.NewPoint(-1000000, -gap)),
	}
	return face, hole, geom.NewLine(geom.NewPoint(1e9, 0), geom.NewPoint(1e9, 1))
}

// TestRegionsHoleContainmentSlackIsLocalToTheTwoCycles checks that a distant
// line cannot move the probe out of the lower triangle and cause a false hole
// assignment or degeneracy. The triangles' boxes are apart by 1.0 in minY.
func TestRegionsHoleContainmentSlackIsLocalToTheTwoCycles(t *testing.T) {
	face, hole, distant := disjointTrianglesScene(1)
	curves := append(append([]geom.Curve{}, face...), hole...)
	curves = append(curves, distant)

	arr := geom.Regions(curves, nil, geom.WithVertexMerge(1e-9))
	require.Len(t, arr.Regions, 2, "the two triangles, each its own region")
	var areas []float64
	for i, reg := range arr.Regions {
		require.Empty(t, reg.Holes,
			"region %d: the two triangles are disjoint, so neither can hold the other as a hole", i)
		areas = append(areas, reg.Area)
	}
	sort.Float64s(areas)
	// Each triangle keeps its whole area: the face would read 1.02e7 with the
	// other triangle wrongly subtracted from it as a hole.
	require.InDelta(t, 3e6, areas[0], 1e-3)
	require.InDelta(t, 1.32e7, areas[1], 1e-3)
	require.False(t, arr.Degenerate, "the distant line must not affect either triangle")
	require.Empty(t, arr.Degeneracies)

	// The same two triangles with the distant line removed: identical regions and
	// areas, and no assignment to reject. What the distant line may never do is
	// change the verdict about two cycles it has nothing to do with.
	near := geom.Regions(append(append([]geom.Curve{}, face...), hole...), nil, geom.WithVertexMerge(1e-9))
	require.False(t, near.Degenerate, "without the distant line no hole is ever offered")
	require.Len(t, near.Regions, 2)
	var nearAreas []float64
	for _, reg := range near.Regions {
		require.Empty(t, reg.Holes)
		nearAreas = append(nearAreas, reg.Area)
	}
	sort.Float64s(nearAreas)
	require.Equal(t, nearAreas, areas, "the distant line must not change either triangle's area")
}

// TestRegionsHoleContainmentRejectsANearGapSeparation checks the same local
// probe rule with smaller gaps. Both gaps remain far above the two cycles'
// bounding-box evaluation round-off, so the triangles stay separate and clean.
func TestRegionsHoleContainmentRejectsANearGapSeparation(t *testing.T) {
	for _, gap := range []float64{5e-4, 1e-6} {
		t.Run(strconv.FormatFloat(gap, 'g', -1, 64), func(t *testing.T) {
			face, hole, distant := disjointTrianglesScene(gap)
			curves := append(append([]geom.Curve{}, face...), hole...)
			curves = append(curves, distant)

			arr := geom.Regions(curves, nil, geom.WithVertexMerge(1e-9))
			require.Len(t, arr.Regions, 2, "the two triangles, each its own region")
			var areas []float64
			for i, reg := range arr.Regions {
				require.Empty(t, reg.Holes,
					"region %d: the triangle's box minY=%v is below the face's minY=0, so it is no hole of it",
					i, -gap)
				areas = append(areas, reg.Area)
			}
			sort.Float64s(areas)
			// The face keeps its whole 1.32e7: with the other triangle wrongly
			// subtracted it read 1.11995e7 at gap=5e-4.
			require.InDelta(t, 2e6+1e6*gap, areas[0], 1e-3)
			require.InDelta(t, 1.32e7, areas[1], 1e-3)
			require.False(t, arr.Degenerate, "the distant line must not affect either triangle")
			require.Empty(t, arr.Degeneracies)
		})
	}
}

// TestRegionsKeepsHoleInsideFaceBoundaryRoundoff pins a genuinely nested hole
// that clears its face by less than the diameter of the hole itself: at the
// values the arrangement holds, centre distance 4.9999999949941412 plus radius
// 5e-9 stays 5.86e-12 inside the outer radius 5. It is the case a too-tight
// containment band would drop, and the hole must still publish. The two cycles'
// summed box allowance here is 8.88e-10 — narrower than the hole, so the
// analytic check classifies witnesses on the far side of it and accepts the
// nesting on positive evidence.
//
// The guard also has to survive the opposite reading. A band wide enough to
// swallow EVERY witness leaves it no counter-example, and reading that silence
// as a refusal dropped this hole and flagged the whole arrangement Degenerate.
// The scale-separated band is what keeps this scene and the exiting hole of
// TestRegionsRejectsHoleThatExitsItsFace on opposite sides of the line.
func TestRegionsKeepsHoleInsideFaceBoundaryRoundoff(t *testing.T) {
	outer := geom.NewCircle(geom.NewPoint(1e6, 0), 5)
	inner := geom.NewCircle(geom.NewPoint(1e6+5-5e-9, 0), 5e-9)

	arr := geom.Regions(nil, []geom.ClosedCurve{outer, inner},
		geom.WithVertexMerge(1e-12), geom.WithSegmentsPerTurn(64))

	require.False(t, arr.Degenerate, "a genuinely nested hole is not a degeneracy")
	require.Empty(t, arr.Degeneracies)
	require.Len(t, arr.Regions, 2, "the annulus and the tiny disk")

	var annulus *geom.Region
	for _, reg := range arr.Regions {
		if reg.Area > 1 {
			annulus = reg
		}
	}
	require.NotNil(t, annulus, "the outer circle must publish a region")
	require.Len(t, annulus.Holes, 1, "the tiny circle stays the outer region's hole")
	require.InDelta(t, 78.539816339744831, annulus.Area, 1e-9)
}

// TestRegionsRejectsHoleThatExitsItsFace pins the other side of the same band:
// a hole whose boundary genuinely LEAVES its face must never be published as
// that face's hole. The inner circle's centre distance 4.9999999980209395 plus
// its radius 5e-9 overshoots the outer radius 5 by 3.02e-9 — about seven orders
// of magnitude above the evaluation error of that comparison, so the exit is
// real. It is separated from the genuinely nested hole of
// TestRegionsKeepsHoleInsideFaceBoundaryRoundoff, which clears its face by
// 5.86e-12, by a factor of about 500; charging the tiny circle one magnitude of
// its 1e6 offset used to make the box allowance 1.4e-8 and swallow both.
func TestRegionsRejectsHoleThatExitsItsFace(t *testing.T) {
	outer := geom.NewCircle(geom.NewPoint(1e6, 0), 5)
	inner := geom.NewCircle(geom.NewPoint(1e6+5-2e-9, 0), 5e-9)

	arr := geom.Regions(nil, []geom.ClosedCurve{outer, inner},
		geom.WithVertexMerge(1e-12), geom.WithSegmentsPerTurn(64))

	require.True(t, arr.Degenerate, "a hole that exits its face is rejected on positive evidence")
	require.NotEmpty(t, arr.Degeneracies)
	for i, reg := range arr.Regions {
		require.Empty(t, reg.Holes,
			"region %d: the inner circle leaves the outer one, so it is no hole of it", i)
	}
}

// TestRegionsRejectsHoleExitAbsorbedByAdditiveBoxTolerance pins the exit that
// the containment guard's box comparison can only see when it is written as a
// positive difference. The hole's box overshoots the face's right edge
// 1000005 by exactly 8 ulps of that coordinate, and the two cycles' summed
// allowance is 7.63 of the same ulps, so the gap is genuinely outside the band
// and the hole must be rejected. Adding the allowance to the coordinate
// instead (fhi+tol) rounds the sum up to the full 8 ulps, which compares equal
// to the hole's bound: the old additive form could not tell this exit from a
// tangency and published the circle as a hole of a face it leaves.
//
// The centre is written in ulps rather than as a decimal literal because that
// is the whole point of the case — a decimal would hide which side of the
// half-ulp rounding step it falls on.
func TestRegionsRejectsHoleExitAbsorbedByAdditiveBoxTolerance(t *testing.T) {
	// 1e6 and the face's right edge 1000005 share a binade, hence an ulp.
	ulp := math.Nextafter(1e6, math.Inf(1)) - 1e6
	outer := geom.NewCircle(geom.NewPoint(1e6, 0), 5)
	inner := geom.NewCircle(geom.NewPoint(1e6+5-2e-9+8*ulp, 0), 2e-9)

	arr := geom.Regions(nil, []geom.ClosedCurve{outer, inner},
		geom.WithVertexMerge(1e-12), geom.WithSegmentsPerTurn(64))

	require.True(t, arr.Degenerate, "an 8-ulp exit against a 7.63-ulp allowance is positive evidence")
	require.NotEmpty(t, arr.Degeneracies)
	for i, reg := range arr.Regions {
		require.Empty(t, reg.Holes,
			"region %d: the inner circle leaves the outer one, so it is no hole of it", i)
	}
}

func TestRegionsHoleContainmentRejectsNestedBoxDisjointTriangle(t *testing.T) {
	face := []geom.Curve{
		geom.NewLine(geom.NewPoint(-1100000, 0), geom.NewPoint(1100000, 8)),
		geom.NewLine(geom.NewPoint(1100000, 8), geom.NewPoint(-1100000, 12)),
		geom.NewLine(geom.NewPoint(-1100000, 12), geom.NewPoint(-1100000, 0)),
	}
	hole := []geom.Curve{
		geom.NewLine(geom.NewPoint(-800000, 0.2), geom.NewPoint(800000, 0.2)),
		geom.NewLine(geom.NewPoint(800000, 0.2), geom.NewPoint(0, 1.8)),
		geom.NewLine(geom.NewPoint(0, 1.8), geom.NewPoint(-800000, 0.2)),
	}
	curves := append(append([]geom.Curve{}, face...), hole...)
	curves = append(curves, geom.NewLine(geom.NewPoint(1e9, 0), geom.NewPoint(1e9, 1)))
	arr := geom.Regions(curves, nil, geom.WithVertexMerge(1e-9))
	require.Len(t, arr.Regions, 2)
	var areas []float64
	for _, reg := range arr.Regions {
		require.Empty(t, reg.Holes)
		areas = append(areas, reg.Area)
	}
	sort.Float64s(areas)
	require.InDelta(t, 1.28e6, areas[0], 1e-3)
	require.InDelta(t, 1.32e7, areas[1], 1e-3)
}
