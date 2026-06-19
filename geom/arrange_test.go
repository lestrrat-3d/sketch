package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// square returns four lines sharing corner points, winding counter-clockwise.
func square(x0, y0, side float64) []geom.Curve {
	a := geom.NewPoint(x0, y0)
	b := geom.NewPoint(x0+side, y0)
	c := geom.NewPoint(x0+side, y0+side)
	d := geom.NewPoint(x0, y0+side)
	return []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
}

func TestRegionsSquare(t *testing.T) {
	arr := geom.Regions(square(0, 0, 10), nil)
	require.Len(t, arr.Regions, 1, "one bounded region")
	r := arr.Regions[0]
	require.InDelta(t, 100, r.Area, 1e-9, "area = side^2")
	require.Empty(t, r.Holes, "no holes")
	require.False(t, r.SelfIntersecting)
	require.Len(t, r.Outer, 4, "four boundary edges")
	require.Empty(t, arr.SelfIntersections)
	for _, e := range r.Outer {
		require.True(t, e.Whole, "each edge is a whole line")
	}
}

func TestRegionsCrossingLinesNoRegion(t *testing.T) {
	// An X: two segments that cross but enclose nothing.
	l1 := geom.NewLine(geom.NewPoint(-5, -5), geom.NewPoint(5, 5))
	l2 := geom.NewLine(geom.NewPoint(-5, 5), geom.NewPoint(5, -5))
	arr := geom.Regions([]geom.Curve{l1, l2}, nil)
	require.Empty(t, arr.Regions, "crossing lines bound no region")
}

func TestRegionsOverlappingRectangles(t *testing.T) {
	// Two axis-aligned rectangles overlapping in [3,6]x[2,4] (area 6).
	rect := func(x0, y0, x1, y1 float64) []geom.Curve {
		a := geom.NewPoint(x0, y0)
		b := geom.NewPoint(x1, y0)
		c := geom.NewPoint(x1, y1)
		d := geom.NewPoint(x0, y1)
		return []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
	}
	var curves []geom.Curve
	curves = append(curves, rect(0, 0, 6, 4)...)
	curves = append(curves, rect(3, 2, 9, 6)...)
	arr := geom.Regions(curves, nil)
	require.Len(t, arr.Regions, 3, "two lunes + the overlap")
	var total float64
	for _, r := range arr.Regions {
		total += r.Area
		require.False(t, r.SelfIntersecting)
	}
	require.InDelta(t, 24+24-6, total, 1e-9, "areas partition the union")
}

func TestRegionsNestedSquareHole(t *testing.T) {
	var curves []geom.Curve
	curves = append(curves, square(0, 0, 10)...) // outer
	curves = append(curves, square(3, 3, 4)...)  // inner (side 4), fully inside
	arr := geom.Regions(curves, nil)
	require.Len(t, arr.Regions, 2, "annulus + inner disk")
	var withHole, inner *geom.Region
	for _, r := range arr.Regions {
		if len(r.Holes) == 1 {
			withHole = r
		} else {
			inner = r
		}
	}
	require.NotNil(t, withHole, "the annulus carries one hole")
	require.NotNil(t, inner, "the inner disk is a separate region")
	require.InDelta(t, 100-16, withHole.Area, 1e-9, "annulus net area")
	require.InDelta(t, 16, inner.Area, 1e-9, "inner region area")
}

func TestRegionsBowtieSelfIntersection(t *testing.T) {
	// A self-crossing quadrilateral: A-B-C-D-A where AB crosses CD.
	a := geom.NewPoint(0, 0)
	b := geom.NewPoint(4, 4)
	c := geom.NewPoint(4, 0)
	d := geom.NewPoint(0, 4)
	curves := []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
	arr := geom.Regions(curves, nil)
	require.NotEmpty(t, arr.SelfIntersections, "the boundary crosses itself")
	for _, r := range arr.Regions {
		require.True(t, r.SelfIntersecting, "regions derive from a self-intersecting boundary")
	}
}

func TestRegionsHalfDiskArcLoop(t *testing.T) {
	// A top semicircle arc closed by its diameter line — a half disk. Exercises
	// an arc inside a chain loop and the exact arc area.
	right := geom.NewPoint(5, 0)
	left := geom.NewPoint(-5, 0)
	ctr := geom.NewPoint(0, 0)
	arc := geom.NewArc(ctr, right, left) // CCW from (5,0) to (-5,0): top half
	line := geom.NewLine(left, right)
	arr := geom.Regions([]geom.Curve{line, arc}, nil)
	require.Len(t, arr.Regions, 1, "one half-disk region")
	r := arr.Regions[0]
	require.InDelta(t, 0.5*math.Pi*25, r.Area, 1e-3, "half the disk area")
	require.Len(t, r.Outer, 2, "a line edge and an arc edge")
	srcs := map[int]bool{}
	for _, e := range r.Outer {
		require.True(t, e.Whole, "both edges span their whole source")
		srcs[e.SourceIndex] = true
	}
	require.Equal(t, map[int]bool{0: true, 1: true}, srcs, "edges back-reference both sources")
}

func TestRegionsSquareWithDiagonals(t *testing.T) {
	// A square plus both diagonals (sharing corners) is a branched wire, not a
	// simple self-crossing loop: it subdivides into four triangles and must NOT
	// be reported self-intersecting.
	a := geom.NewPoint(0, 0)
	b := geom.NewPoint(10, 0)
	c := geom.NewPoint(10, 10)
	d := geom.NewPoint(0, 10)
	curves := []geom.Curve{
		geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a),
		geom.NewLine(a, c), geom.NewLine(b, d),
	}
	arr := geom.Regions(curves, nil)
	require.Len(t, arr.Regions, 4, "four triangles")
	require.Empty(t, arr.SelfIntersections, "a branched wire is not self-intersecting")
	var total float64
	for _, r := range arr.Regions {
		require.False(t, r.SelfIntersecting)
		total += r.Area
	}
	require.InDelta(t, 100, total, 1e-9)
}

func TestRegionsEllipseArea(t *testing.T) {
	e := geom.NewEllipse(geom.NewPoint(1, 2), 4, 2, 0.3)
	arr := geom.Regions(nil, []geom.ClosedCurve{e})
	require.Len(t, arr.Regions, 1, "a lone ellipse is one region")
	require.InDelta(t, math.Pi*4*2, arr.Regions[0].Area, 1e-2, "pi*rx*ry")
}

func TestRegionsCollinearOverlapDegenerate(t *testing.T) {
	// A square with a duplicate segment lying on its bottom edge.
	curves := square(0, 0, 10)
	curves = append(curves, geom.NewLine(geom.NewPoint(2, 0), geom.NewPoint(8, 0)))
	arr := geom.Regions(curves, nil)
	require.True(t, arr.Degenerate, "coincident edges are degenerate")
	require.NotEmpty(t, arr.Degeneracies)
}

func TestRegionsDanglingSpurPruned(t *testing.T) {
	// A square with a line spur sticking out of a corner. The spur bounds no
	// region and is pruned; the square's region is unaffected.
	corner := geom.NewPoint(0, 0)
	b := geom.NewPoint(10, 0)
	c := geom.NewPoint(10, 10)
	d := geom.NewPoint(0, 10)
	curves := []geom.Curve{
		geom.NewLine(corner, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, corner),
		geom.NewLine(corner, geom.NewPoint(-5, -5)), // spur
	}
	arr := geom.Regions(curves, nil)
	require.Len(t, arr.Regions, 1, "spur contributes no region")
	require.InDelta(t, 100, arr.Regions[0].Area, 1e-9)
}

func TestRegionsNilInputDegenerateNoPanic(t *testing.T) {
	require.NotPanics(t, func() {
		arr := geom.Regions([]geom.Curve{&geom.Line{}}, nil) // nil endpoints
		require.True(t, arr.Degenerate)
	})
}

func TestRegionsBowtieWithSpurStillSelfIntersecting(t *testing.T) {
	// A self-crossing bowtie with a dangling spur attached at a corner. The spur
	// makes that corner degree 3, but it is pruned to a simple loop; the
	// self-intersection must still be detected (not masked by the spur).
	a := geom.NewPoint(0, 0)
	b := geom.NewPoint(4, 4)
	c := geom.NewPoint(4, 0)
	d := geom.NewPoint(0, 4)
	curves := []geom.Curve{
		geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a),
		geom.NewLine(a, geom.NewPoint(-3, -3)), // spur off corner a
	}
	arr := geom.Regions(curves, nil)
	require.NotEmpty(t, arr.SelfIntersections, "spur must not mask the self-crossing")
}

func TestRegionsZeroRadiusDegenerate(t *testing.T) {
	// A zero-radius circle (and a zero-axis ellipse) encloses no area; the
	// arrangement must report it degenerate, not silently clean.
	c := geom.NewCircle(geom.NewPoint(0, 0), 0)
	arr := geom.Regions(nil, []geom.ClosedCurve{c})
	require.True(t, arr.Degenerate, "a zero-radius circle is degenerate")

	e := geom.NewEllipse(geom.NewPoint(0, 0), 4, 0, 0)
	arr = geom.Regions(nil, []geom.ClosedCurve{e})
	require.True(t, arr.Degenerate, "a zero-axis ellipse is degenerate")
}

func TestRegionsCircleCutByChord(t *testing.T) {
	// A horizontal line at y=3 crosses a circle r=5 at x=±4, splitting it into
	// a minor cap (above) and the major region (below).
	circle := geom.NewCircle(geom.NewPoint(0, 0), 5)
	line := geom.NewLine(geom.NewPoint(-8, 3), geom.NewPoint(8, 3))
	arr := geom.Regions([]geom.Curve{line}, []geom.ClosedCurve{circle})
	require.Len(t, arr.Regions, 2, "chord splits the disk in two")
	var total float64
	for _, r := range arr.Regions {
		total += r.Area
	}
	require.InDelta(t, 25*math.Pi, total, 1e-2, "the two pieces sum to the disk")
	// minor cap area = r^2/2 (theta - sin theta), theta = 2 acos(3/5)
	theta := 2 * math.Acos(3.0/5.0)
	cap := 0.5 * 25 * (theta - math.Sin(theta))
	var minA float64 = math.Inf(1)
	for _, r := range arr.Regions {
		minA = math.Min(minA, r.Area)
	}
	require.InDelta(t, cap, minA, 1e-2, "minor cap area")
}

// A spline now participates in the arrangement: an open spline plus a chord that
// closes back to its endpoints bounds a region with a sampled (positive) area.
func TestRegionsSplineWithChord(t *testing.T) {
	a := geom.NewPoint(0, 0)
	c1 := geom.NewPoint(1, 2)
	c2 := geom.NewPoint(3, 2)
	b := geom.NewPoint(4, 0)
	sp, err := geom.NewSpline(a, c1, c2, b)
	require.NoError(t, err)
	chord := geom.NewLine(b, a) // shares the endpoint points a, b

	arr := geom.Regions([]geom.Curve{sp, chord}, nil)
	require.Len(t, arr.Regions, 1, "spline + chord bound one region")
	require.Greater(t, arr.Regions[0].Area, 0.0, "sampled bulge gives positive area")
	require.False(t, arr.Regions[0].SelfIntersecting)
	require.Empty(t, arr.SelfIntersections)
}

// A single cubic Bézier whose control polygon loops self-crosses; closed by a
// chord the arrangement must report the self-intersection.
func TestRegionsSelfIntersectingSpline(t *testing.T) {
	p0 := geom.NewPoint(0, 0)
	p1 := geom.NewPoint(-4.0/3.0, -5.0/12.0)
	p2 := geom.NewPoint(-4.0/3.0, -3.0/2.0)
	p3 := geom.NewPoint(0, 3.0/4.0)
	sp, err := geom.NewSpline(p0, p1, p2, p3)
	require.NoError(t, err)
	chord := geom.NewLine(p3, p0)

	arr := geom.Regions([]geom.Curve{sp, chord}, nil)
	require.NotEmpty(t, arr.SelfIntersections, "the spline crosses itself")
	var sawSelfX bool
	for _, r := range arr.Regions {
		if r.SelfIntersecting {
			sawSelfX = true
		}
	}
	require.True(t, sawSelfX, "a region inherits the self-intersection")
}

func TestRegionsDegenerateSpline(t *testing.T) {
	// Four coincident control points: a point, not a curve. Must be flagged
	// degenerate rather than silently dropped.
	p := geom.NewPoint(2, 2)
	sp, err := geom.NewSpline(p, p, p, p)
	require.NoError(t, err)
	arr := geom.Regions([]geom.Curve{sp}, nil)
	require.True(t, arr.Degenerate, "an all-coincident spline has no extent")
}
