package geom_test

import (
	"math"
	"math/rand"
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
func boundsOf(pts [][2]float64) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		minX, minY = math.Min(minX, p[0]), math.Min(minY, p[1])
		maxX, maxY = math.Max(maxX, p[0]), math.Max(maxY, p[1])
	}
	return
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
