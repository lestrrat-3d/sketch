package geom_test

import (
	"math"
	"slices"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// leverForkScene is the fork end of a watch lever and the notch cut into it. The
// fork's gap round G and the notch semicircle B both end at a = (r, ky), where G
// touches B from inside; the corner round K touches B from outside there. All
// three centres lie on y = ky, so all three curves are vertical at a, and G and
// B leave a along one ray. The notch's straight walls start at a too.
func leverForkScene(th float64) []geom.Curve {
	const (
		ky  = 38.874650636459869
		r   = 1.6676711944252536
		gc  = 1.1338355972126268
		dr  = 0.59999999999999998
		tip = 42.149999999999999
		kc  = 2.3338355972126268
	)
	p := func(x, y float64) *geom.Point { return rotated(th, x, y) }
	a0, a1, a2 := p(3, 30), p(3, ky), p(r, ky)
	b0, b1, b2 := p(-3, 30), p(-3, ky), p(-r, ky)
	d1, d2, d3, d4 := p(dr, ky), p(dr, tip), p(-dr, tip), p(-dr, ky)
	u1, u2 := p(1, 44), p(-1, 44)
	return []geom.Curve{
		geom.NewLine(a0, a1),
		geom.NewArc(p(kc, ky), a1, a2),
		geom.NewArc(p(gc, ky), d1, a2),
		geom.NewLine(d1, d2),
		geom.NewArc(p(0, tip), d2, d3),
		geom.NewLine(d3, d4),
		geom.NewArc(p(-gc, ky), b2, d4),
		geom.NewArc(p(-kc, ky), b2, b1),
		geom.NewLine(b1, b0),
		geom.NewLine(b0, a0),
		geom.NewArc(p(0, ky), b2, a2),
		geom.NewLine(a2, u1),
		geom.NewLine(u1, u2),
		geom.NewLine(u2, b2),
	}
}

// TestSharedTangentEndKeepsCells pins the three cells of the fork and notch: the
// fork less the notch, their overlap (the dart and two crescents that narrow to
// a and its mirror), and the notch less the fork. Before the shared same-ray end
// was certified for exact port order, the face walk returned one cell with the
// union's area, reported valid and not degenerate.
func TestSharedTangentEndKeepsCells(t *testing.T) {
	for k := range 32 {
		th := float64(k) * math.Pi / 16
		testLeverForkCells(t, th)
	}
}

func testLeverForkCells(t *testing.T, th float64) {
	t.Helper()
	scene := leverForkScene(th)
	for _, reversed := range []bool{false, true} {
		in := slices.Clone(scene)
		if reversed {
			slices.Reverse(in)
		}
		arr := geom.Regions(in, nil)
		require.Falsef(t, arr.Degenerate, "th=%g reversed=%v", th, reversed)
		require.Lenf(t, arr.Regions, 3, "th=%g reversed=%v", th, reversed)
		areas := make([]float64, 0, len(arr.Regions))
		for _, region := range arr.Regions {
			areas = append(areas, region.Area)
		}
		slices.Sort(areas)
		require.InDeltaf(t, 7.969198, areas[0], 1e-5, "th=%g reversed=%v", th, reversed)
		require.InDeltaf(t, 10.072133, areas[1], 1e-5, "th=%g reversed=%v", th, reversed)
		require.InDeltaf(t, 50.273480, areas[2], 1e-5, "th=%g reversed=%v", th, reversed)
	}
}

// TestSharedEndLineArcSameRayKeepsChordOrder pins a line and an arc leaving a
// shared endpoint along one ray: the line's chord lies on the ray and the arc's
// first chord lies off it on the side the arc bends toward, so chord order is the
// curves' order and the scene publishes its one region without a flag.
func TestSharedEndLineArcSameRayKeepsChordOrder(t *testing.T) {
	p := geom.NewPoint
	// The quarter arc from (1, 0) to (0, 1) about the origin leaves (1, 0) along
	// +y, as does the line from (1, 0) to (1, 2).
	curves := []geom.Curve{
		geom.NewArc(p(0, 0), p(1, 0), p(0, 1)),
		geom.NewLine(p(1, 0), p(1, 2)),
		geom.NewLine(p(1, 2), p(0, 1)),
	}
	arr := geom.Regions(curves, nil)
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 1)
	require.InDelta(t, 1.5-math.Pi/4, arr.Regions[0].Area, 1e-9)
}

// rotated returns the point (x, y) turned by th about the origin.
func rotated(th, x, y float64) *geom.Point {
	s, c := math.Sincos(th)
	return geom.NewPoint(c*x-s*y, s*x+c*y)
}

// TestSharedEndExternalSameRayKeepsCells pins two arcs in external tangency that
// both start at P = (0, 0) along +y and bend apart, with two lines leaving P
// downward. The ring at P holds four ports. The shared end is certified for exact
// port order; the arcs' first chords also leave on opposite sides of the ray, so
// chord order agrees. Every rotation must publish the cusp region between the arcs
// and the triangle below.
func TestSharedEndExternalSameRayKeepsCells(t *testing.T) {
	for k := range 16 {
		th := float64(k) * math.Pi / 8
		pt := func(x, y float64) *geom.Point { return rotated(th, x, y) }
		curves := []geom.Curve{
			geom.NewArc(pt(-1, 0), pt(0, 0), pt(-1, 1)),
			geom.NewLine(pt(-1, 1), pt(1, 1)),
			geom.NewArc(pt(1, 0), pt(1, 1), pt(0, 0)),
			geom.NewLine(pt(0, 0), pt(0.5, -1)),
			geom.NewLine(pt(0.5, -1), pt(-0.5, -1)),
			geom.NewLine(pt(-0.5, -1), pt(0, 0)),
		}
		arr := geom.Regions(curves, nil)
		require.Falsef(t, arr.Degenerate, "th=%g", th)
		require.Lenf(t, arr.Regions, 2, "th=%g", th)
		areas := []float64{arr.Regions[0].Area, arr.Regions[1].Area}
		slices.Sort(areas)
		require.InDeltaf(t, 2-math.Pi/2, areas[0], 1e-9, "th=%g", th)
		require.InDeltaf(t, 0.5, areas[1], 1e-9, "th=%g", th)
	}
}

// TestSharedTangentEndWithSampledPortIsFlagged pins a certified cusp vertex whose
// ring also holds a conic: exact port order cannot run, so the scene and every
// region at that vertex are flagged rather than walked by chord order, which lost
// the disk and the crescent in some rotations with no flag.
func TestSharedTangentEndWithSampledPortIsFlagged(t *testing.T) {
	const (
		bigR  = 2.0
		small = 1.2
	)
	for k := range 8 {
		th := float64(k) * math.Pi / 4
		pt := func(x, y float64) *geom.Point { return rotated(th, x, y) }
		gc := bigR - small
		bs := pt(bigR*math.Cos(-1), bigR*math.Sin(-1))
		gs := pt(gc+small*math.Cos(-1), small*math.Sin(-1))
		curves := []geom.Curve{
			geom.NewArc(pt(0, 0), bs, pt(bigR, 0)),
			geom.NewArc(pt(0, 0), pt(bigR, 0), bs),
			geom.NewArc(pt(gc, 0), gs, pt(bigR, 0)),
			geom.NewLine(gs, bs),
			geom.NewConic(pt(bigR, 0), pt(bigR+0.5, 0.5), pt(bigR+1, 0), 0.5),
			geom.NewLine(pt(bigR+1, 0), pt(bigR, 0)),
		}
		arr := geom.Regions(curves, nil)
		require.Truef(t, arr.Degenerate, "th=%g", th)
		for _, region := range arr.Regions {
			require.Truef(t, region.Degenerate, "th=%g: every region at the flagged vertex reads degenerate", th)
		}
	}
}
