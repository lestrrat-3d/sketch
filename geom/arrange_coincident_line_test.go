package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// rect returns the four sides of the axis-aligned rectangle (x0,y0)-(x1,y1) as
// bottom, right, top, left lines, walked counter-clockwise through four shared
// corner points.
func rect(x0, y0, x1, y1 float64) []geom.Curve {
	a := geom.NewPoint(x0, y0)
	b := geom.NewPoint(x1, y0)
	c := geom.NewPoint(x1, y1)
	d := geom.NewPoint(x0, y1)
	return []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
}

// requireLineEdgesReproduce asserts every boundary edge is exact and, being a line
// edge, that evaluating its source at TStart/TEnd lands on the edge's own polyline
// ends in walk order — the checkable meaning TExact promises.
func requireLineEdgesReproduce(t *testing.T, curves []geom.Curve, edges []geom.BoundaryEdge) {
	t.Helper()
	for _, e := range edges {
		require.Truef(t, e.TExact, "edge of source %d [%v, %v]", e.SourceIndex, e.TStart, e.TEnd)
		l, ok := curves[e.SourceIndex].(*geom.Line)
		require.True(t, ok)
		at := func(tt float64) [2]float64 {
			return [2]float64{l.Start.X + tt*(l.End.X-l.Start.X), l.Start.Y + tt*(l.End.Y-l.Start.Y)}
		}
		first, last := at(e.TStart), at(e.TEnd)
		if e.Reversed {
			first, last = last, first
		}
		require.InDeltaSlice(t, first[:], e.Polyline[0][:], 1e-12)
		require.InDeltaSlice(t, last[:], e.Polyline[len(e.Polyline)-1][:], 1e-12)
	}
}

// sharedWallEdges returns, per region, the outer edges lying on the carrier x = 10
// (both polyline ends there).
func sharedWallEdges(arr *geom.Arrangement) [][]geom.BoundaryEdge {
	out := make([][]geom.BoundaryEdge, len(arr.Regions))
	for i, r := range arr.Regions {
		for _, e := range r.Outer {
			if e.Polyline[0][0] == 10 && e.Polyline[len(e.Polyline)-1][0] == 10 {
				out[i] = append(out[i], e)
			}
		}
	}
	return out
}

// TestCoincidentLineCarriersResolve pins the shared-wall scenes a downstream boolean
// builds by drawing both operands' lines into one arrangement: two rectangles whose
// walls lie on one carrier line and overlap in a span of positive length. Each scene
// must arrange into valid cells with every bound exact, and the cells' areas must sum
// to the area of the union of the two rectangles. Both input orders are run, so the
// named wall is the first rectangle's in one and the second's in the other.
func TestCoincidentLineCarriersResolve(t *testing.T) {
	cases := []struct {
		name      string
		second    [4]float64
		wantAreas []float64
	}{
		{name: "whole wall shared", second: [4]float64{10, 0, 20, 10}, wantAreas: []float64{100, 100}},
		{name: "interior sub-span", second: [4]float64{10, 2, 20, 8}, wantAreas: []float64{60, 100}},
		{name: "wall inside a longer carrier", second: [4]float64{10, -2, 20, 12}, wantAreas: []float64{100, 140}},
		{name: "overlapping with collinear floor and roof", second: [4]float64{9, 0, 20, 10}, wantAreas: []float64{10, 90, 100}},
	}
	for _, tc := range cases {
		for _, variant := range []struct {
			name             string
			secondFirst, ccw bool
		}{
			{name: "authored order", ccw: true},
			{name: "second rectangle first", secondFirst: true, ccw: true},
			{name: "second rectangle clockwise", ccw: false},
		} {
			secondFirst := variant.secondFirst
			t.Run(tc.name+"/"+variant.name, func(t *testing.T) {
				first := rect(0, 0, 10, 10)
				second := rect(tc.second[0], tc.second[1], tc.second[2], tc.second[3])
				if !variant.ccw {
					// Reverse every side, so each shared wall runs the same way as the
					// first rectangle's instead of against it.
					for i, c := range second {
						l := c.(*geom.Line)
						second[i] = geom.NewLine(l.End, l.Start)
					}
				}
				curves := append(first, second...)
				if secondFirst {
					curves = append(second, first...)
				}
				arr := geom.Regions(curves, nil)
				require.False(t, arr.Degenerate, "degeneracies: %v", arr.Degeneracies)
				require.InDeltaSlice(t, tc.wantAreas, sortedAreas(arr), 1e-9)
				for _, r := range arr.Regions {
					require.False(t, r.Degenerate)
					require.False(t, r.SelfIntersecting)
					require.Empty(t, r.Holes)
					requireLineEdgesReproduce(t, curves, r.Outer)
				}
			})
		}
	}
}

// TestCoincidentLineCarrierSpanIsOneEdgePerCell pins how the shared span is
// reported: ONE edge on each of the two cells it separates, both naming the
// lower-indexed of the two coincident lines and walking it in opposite senses, while
// the losing line contributes only its parts outside the span, each with an exact,
// certified range.
func TestCoincidentLineCarrierSpanIsOneEdgePerCell(t *testing.T) {
	t.Run("interior sub-span names the longer wall", func(t *testing.T) {
		// Rectangle A's right wall (index 1) runs (10,0)->(10,10); rectangle B's left
		// wall (index 7) runs (10,8)->(10,2), wholly inside it, so B's wall emits
		// nothing. Cell A walks its right wall end to end, so the span coalesces into
		// one whole edge there; cell B walks only the span.
		curves := append(rect(0, 0, 10, 10), rect(10, 2, 20, 8)...)
		arr := geom.Regions(curves, nil)
		require.False(t, arr.Degenerate)
		walls := sharedWallEdges(arr)
		require.Len(t, walls, 2)

		var spans []geom.BoundaryEdge
		for _, w := range walls {
			require.Len(t, w, 1, "the carrier is one edge on each cell")
			spans = append(spans, w[0])
		}
		if !spans[0].Whole {
			spans[0], spans[1] = spans[1], spans[0]
		}
		require.Equal(t, 1, spans[0].SourceIndex)
		require.Equal(t, 1, spans[1].SourceIndex, "the losing wall emits no edge")
		require.Equal(t, [2]float64{0, 1}, [2]float64{spans[0].TStart, spans[0].TEnd})
		require.Equal(t, [2]float64{0.2, 0.8}, [2]float64{spans[1].TStart, spans[1].TEnd})
		require.NotEqual(t, spans[0].Reversed, spans[1].Reversed, "the two cells walk it in opposite senses")
	})

	t.Run("wall inside a longer carrier keeps the losing line's outer parts", func(t *testing.T) {
		// Rectangle A's right wall (index 1) runs (10,0)->(10,10) and is named.
		// Rectangle B's left wall (index 7) runs (10,12)->(10,-2), so it loses the
		// middle 10 units and keeps [0, 1/7] and [6/7, 1] of its own parameter.
		curves := append(rect(0, 0, 10, 10), rect(10, -2, 20, 12)...)
		arr := geom.Regions(curves, nil)
		require.False(t, arr.Degenerate)

		var losing [][2]float64
		named := 0
		for _, w := range sharedWallEdges(arr) {
			for _, e := range w {
				switch e.SourceIndex {
				case 1:
					require.True(t, e.Whole, "the named wall is the whole span")
					named++
				case 7:
					require.False(t, e.Whole)
					losing = append(losing, [2]float64{e.TStart, e.TEnd})
				}
			}
		}
		require.Equal(t, 2, named, "the named wall bounds both cells")
		require.ElementsMatch(t, [][2]float64{{0, 1.0 / 7}, {6.0 / 7, 1}}, losing)
	})
}

// TestCoincidentLineCarrierNearCollinearStaysDegenerate pins the refusal band: a
// pair of walls collinear only to within the classification band, and not at
// round-off, is NOT resolved. The arrangement reports it Degenerate, as the circular
// case does for carriers equal only within its own classification band.
func TestCoincidentLineCarrierNearCollinearStaysDegenerate(t *testing.T) {
	const off = 1e-9 // far inside the vertex-merge band, far outside the identity band
	curves := append(rect(0, 0, 10, 10), rect(10+off, 2, 20, 8)...)
	arr := geom.Regions(curves, nil)
	require.True(t, arr.Degenerate)
}

// TestCoincidentLineCarrierDistantSceneStaysDegenerate pins the carrier-local half of
// the identity gate. A wall offset by 1e-8 is inside the scene half of the gate once a
// line 1e5 units away stretches the scene, but it is a visible fraction of the
// walls' own length; it must stay refused however far the rest of the scene reaches.
func TestCoincidentLineCarrierDistantSceneStaysDegenerate(t *testing.T) {
	const off = 1e-8
	curves := append(rect(0, 0, 10, 10), rect(10+off, 2, 20, 8)...)
	curves = append(curves, geom.NewLine(geom.NewPoint(1e5, 0), geom.NewPoint(1e5, 1)))
	arr := geom.Regions(curves, nil)
	require.True(t, arr.Degenerate)
}

// TestCoincidentLineCarrierDanglingLineStaysDegenerate pins the closed-loop gate: a
// line lying on a square's edge but joined to nothing at either end bounds no cell,
// so the overlap is an authoring defect and stays Degenerate, while the same span
// shared by two closed outlines resolves.
func TestCoincidentLineCarrierDanglingLineStaysDegenerate(t *testing.T) {
	curves := append(rect(0, 0, 10, 10), geom.NewLine(geom.NewPoint(2, 0), geom.NewPoint(8, 0)))
	arr := geom.Regions(curves, nil)
	require.True(t, arr.Degenerate)
	require.Len(t, arr.Regions, 1)
	require.InDelta(t, 100, arr.Regions[0].Area, 1e-9)
}

// TestCoincidentLineCarrierOneLoopDoublingBackStaysDegenerate pins the other half of
// the shared-wall gate: two edges of ONE simple closed loop sharing a span are that
// loop doubling back over itself, not a wall between two outlines, and stay
// Degenerate. The smallest such loop is a line drawn there and back; a notch whose
// floor runs back along the loop's own bottom edge is the other.
func TestCoincidentLineCarrierOneLoopDoublingBackStaysDegenerate(t *testing.T) {
	t.Run("there and back", func(t *testing.T) {
		a, b := geom.NewPoint(0, 0), geom.NewPoint(10, 0)
		arr := geom.Regions([]geom.Curve{geom.NewLine(a, b), geom.NewLine(b, a)}, nil)
		require.True(t, arr.Degenerate)
		require.Empty(t, arr.Regions)
	})
	t.Run("notch along its own edge", func(t *testing.T) {
		p := []*geom.Point{
			geom.NewPoint(0, 0), geom.NewPoint(10, 0), geom.NewPoint(10, 5), geom.NewPoint(6, 5),
			geom.NewPoint(6, 0), geom.NewPoint(4, 0), geom.NewPoint(4, 5), geom.NewPoint(0, 5),
		}
		var curves []geom.Curve
		for i := range p {
			curves = append(curves, geom.NewLine(p[i], p[(i+1)%len(p)]))
		}
		arr := geom.Regions(curves, nil)
		require.True(t, arr.Degenerate)
		for _, r := range arr.Regions {
			require.True(t, r.Degenerate)
		}
	})
}

// TestCoincidentLineCarrierCompetingCutStaysDegenerate pins the postcondition the
// resolution is settled by. An unrelated line ends on the named wall 5e-9 below the
// span's lower end, closer than split's per-segment dedup separates two cuts, and it
// is passed in before the second rectangle, so its cut is the one the dedup keeps.
// The span's lower end then never becomes a vertex of the named wall, the window is
// withdrawn, and the pair reports Degenerate rather than suppressing the losing wall
// against a boundary that was not emitted. With the line moved clear of the span's
// end, the same scene resolves.
func TestCoincidentLineCarrierCompetingCutStaysDegenerate(t *testing.T) {
	build := func(gap float64) *geom.Arrangement {
		curves := rect(0, 0, 10, 10)
		curves = append(curves, geom.NewLine(geom.NewPoint(5, 1-gap), geom.NewPoint(10, 2-gap)))
		curves = append(curves, rect(10, 2, 20, 8)...)
		return geom.Regions(curves, nil, geom.WithVertexMerge(1e-12))
	}
	require.True(t, build(5e-9).Degenerate, "a boundary the dedup dropped must withdraw the window")

	arr := build(1e-3)
	require.False(t, arr.Degenerate, "degeneracies: %v", arr.Degeneracies)
	require.InDeltaSlice(t, []float64{60, 100}, sortedAreas(arr), 1e-9)
}

// TestCoincidentLineCarrierRotatedWall runs the shared-wall scene on a carrier that
// is not axis-aligned, built by rotating both rectangles about the origin, so the
// coordinates carry round-off and the identity gate is exercised on real arithmetic.
func TestCoincidentLineCarrierRotatedWall(t *testing.T) {
	rot := func(x, y float64) *geom.Point {
		c, s := math.Cos(0.5), math.Sin(0.5)
		return geom.NewPoint(c*x-s*y, s*x+c*y)
	}
	// One point per coordinate, shared by both rectangles where they coincide.
	pts := map[[2]float64]*geom.Point{}
	pt := func(x, y float64) *geom.Point {
		k := [2]float64{x, y}
		if p, ok := pts[k]; ok {
			return p
		}
		pts[k] = rot(x, y)
		return pts[k]
	}
	loop := func(x0, y0, x1, y1 float64) []geom.Curve {
		a, b, c, d := pt(x0, y0), pt(x1, y0), pt(x1, y1), pt(x0, y1)
		return []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
	}
	curves := append(loop(0, 0, 10, 10), loop(10, 2, 20, 8)...)
	arr := geom.Regions(curves, nil)
	require.False(t, arr.Degenerate, "degeneracies: %v", arr.Degeneracies)
	require.InDeltaSlice(t, []float64{60, 100}, sortedAreas(arr), 1e-9)
	for _, r := range arr.Regions {
		require.False(t, r.Degenerate)
		requireLineEdgesReproduce(t, curves, r.Outer)
	}
}
