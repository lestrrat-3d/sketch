package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func ln(x1, y1, x2, y2 float64) *geom.Line {
	return geom.NewLine(geom.NewPoint(x1, y1), geom.NewPoint(x2, y2))
}

func cir(x, y, r float64) *geom.Circle {
	return geom.NewCircle(geom.NewPoint(x, y), r)
}

func TestLineLineIntersection(t *testing.T) {
	p, ok := geom.LineLineIntersection(ln(0, 0, 10, 0), ln(5, -5, 5, 5))
	require.True(t, ok, "crossing lines")
	require.InDelta(t, 5, p.X, 1e-12, "x")
	require.InDelta(t, 0, p.Y, 1e-12, "y")

	// Beyond the segments, but the infinite lines still cross.
	p, ok = geom.LineLineIntersection(ln(0, 0, 10, 0), ln(20, -5, 20, 5))
	require.True(t, ok, "infinite lines cross")
	require.InDelta(t, 20, p.X, 1e-12, "x beyond segment")

	_, ok = geom.LineLineIntersection(ln(0, 0, 10, 0), ln(0, 1, 10, 1))
	require.False(t, ok, "parallel lines")
}

func TestSegmentIntersection(t *testing.T) {
	p, ok := geom.SegmentIntersection(ln(0, 0, 10, 0), ln(5, -5, 5, 5))
	require.True(t, ok, "crossing segments")
	require.InDelta(t, 5, p.X, 1e-12, "x")

	_, ok = geom.SegmentIntersection(ln(0, 0, 10, 0), ln(20, -5, 20, 5))
	require.False(t, ok, "crossing point outside segment")

	p, ok = geom.SegmentIntersection(ln(0, 0, 10, 0), ln(10, 0, 10, 5))
	require.True(t, ok, "touching endpoints intersect")
	require.InDelta(t, 10, p.X, 1e-12, "endpoint x")
}

func TestLineCircleIntersections(t *testing.T) {
	pts := geom.LineCircleIntersections(ln(-10, 0, 10, 0), cir(0, 0, 5))
	require.Len(t, pts, 2, "secant")
	require.InDelta(t, -5, pts[0].X, 1e-12, "first hit x")
	require.InDelta(t, 5, pts[1].X, 1e-12, "second hit x")

	pts = geom.LineCircleIntersections(ln(-10, 5, 10, 5), cir(0, 0, 5))
	require.Len(t, pts, 1, "tangent")
	require.InDelta(t, 0, pts[0].X, 1e-9, "tangent x")
	require.InDelta(t, 5, pts[0].Y, 1e-9, "tangent y")

	require.Empty(t, geom.LineCircleIntersections(ln(-10, 6, 10, 6), cir(0, 0, 5)), "miss")
}

func TestCircleCircleIntersections(t *testing.T) {
	pts := geom.CircleCircleIntersections(cir(0, 0, 5), cir(8, 0, 5))
	require.Len(t, pts, 2, "crossing circles")
	require.InDelta(t, 4, pts[0].X, 1e-12, "x")
	require.InDelta(t, 3, math.Abs(pts[0].Y), 1e-12, "|y|")
	require.InDelta(t, -pts[0].Y, pts[1].Y, 1e-12, "symmetric")

	pts = geom.CircleCircleIntersections(cir(0, 0, 5), cir(10, 0, 5))
	require.Len(t, pts, 1, "externally tangent")
	require.InDelta(t, 5, pts[0].X, 1e-9, "tangent point")

	require.Empty(t, geom.CircleCircleIntersections(cir(0, 0, 5), cir(20, 0, 5)), "separate")
	require.Empty(t, geom.CircleCircleIntersections(cir(0, 0, 5), cir(1, 0, 2)), "contained")
	require.Empty(t, geom.CircleCircleIntersections(cir(0, 0, 5), cir(0, 0, 3)), "concentric")
}

func TestArcContains(t *testing.T) {
	// Quarter arc from (5,0) CCW to (0,5).
	a := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(5, 0), geom.NewPoint(0, 5))
	require.True(t, a.Contains(geom.NewPoint(3, 3)), "mid-sweep")
	require.True(t, a.Contains(geom.NewPoint(5, 0)), "start endpoint")
	require.True(t, a.Contains(geom.NewPoint(0, 5)), "end endpoint")
	require.False(t, a.Contains(geom.NewPoint(-5, 0)), "outside sweep")
	require.False(t, a.Contains(geom.NewPoint(3, -3)), "below axis")
}

func TestArcIntersections(t *testing.T) {
	quarter := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(5, 0), geom.NewPoint(0, 5))

	pts := geom.LineArcIntersections(ln(-10, 0, 10, 0), quarter)
	require.Len(t, pts, 1, "x-axis meets quarter arc once")
	require.InDelta(t, 5, pts[0].X, 1e-9, "at the start point")

	pts = geom.CircleArcIntersections(cir(8, 0, 5), quarter)
	require.Len(t, pts, 1, "only the upper crossing is on the arc")
	require.InDelta(t, 4, pts[0].X, 1e-9, "x")
	require.InDelta(t, 3, pts[0].Y, 1e-9, "y (upper)")

	// Mirror quarter arc about x=4: from (3,-3)... use a second quarter arc
	// covering the upper-left of its circle so exactly one crossing is shared.
	other := geom.NewArc(geom.NewPoint(8, 0), geom.NewPoint(4, 3), geom.NewPoint(8, 5))
	pts = geom.ArcArcIntersections(quarter, other)
	require.Len(t, pts, 1, "single shared crossing")
	require.InDelta(t, 4, pts[0].X, 1e-9, "x")
	require.InDelta(t, 3, pts[0].Y, 1e-9, "y")
}

func TestSplitLineAt(t *testing.T) {
	l := ln(0, 0, 10, 0)
	p := geom.NewPoint(4, 0)
	left, right := geom.SplitLineAt(l, p)
	require.Same(t, l.Start, left.Start, "left keeps start")
	require.Same(t, p, left.End, "left ends at split")
	require.Same(t, p, right.Start, "right starts at split")
	require.Same(t, l.End, right.End, "right keeps end")
	require.InDelta(t, 4, left.Length(), 1e-12, "left length")
	require.InDelta(t, 6, right.Length(), 1e-12, "right length")
}

func TestFillet(t *testing.T) {
	// L-corner at the origin: legs along +x and +y.
	c := geom.NewPoint(0, 0)
	l1 := geom.NewLine(geom.NewPoint(10, 0), c)
	l2 := geom.NewLine(c, geom.NewPoint(0, 10))

	arc, ok := geom.Fillet(l1, l2, 2)
	require.True(t, ok, "fillet succeeds")
	require.InDelta(t, 2, arc.Center.X, 1e-12, "center x")
	require.InDelta(t, 2, arc.Center.Y, 1e-12, "center y")
	require.InDelta(t, 2, arc.Radius(), 1e-12, "radius")
	require.InDelta(t, math.Pi/2, arc.Sweep(), 1e-9, "quarter sweep")

	// The legs were shortened to the contact points and detached from c.
	require.InDelta(t, 8, l1.Length(), 1e-12, "l1 shortened")
	require.InDelta(t, 8, l2.Length(), 1e-12, "l2 shortened")
	require.NotSame(t, c, l1.End, "l1 detached from corner")
	require.NotSame(t, c, l2.Start, "l2 detached from corner")

	// The arc bulges toward the corner: its midpoint is inside the L.
	mid := arc.StartAngle() + arc.Sweep()/2
	require.InDelta(t, 2-math.Sqrt2, arc.Center.X+2*math.Cos(mid), 1e-9, "arc midpoint x")
	require.InDelta(t, 2-math.Sqrt2, arc.Center.Y+2*math.Sin(mid), 1e-9, "arc midpoint y")
}

func TestFilletFailures(t *testing.T) {
	c := geom.NewPoint(0, 0)
	l1 := geom.NewLine(geom.NewPoint(10, 0), c)
	l2 := geom.NewLine(c, geom.NewPoint(0, 10))

	_, ok := geom.Fillet(l1, l2, 20)
	require.False(t, ok, "radius exceeds the legs")
	_, ok = geom.Fillet(l1, l2, 0)
	require.False(t, ok, "non-positive radius")

	disjoint := ln(20, 0, 30, 0)
	_, ok = geom.Fillet(l1, disjoint, 1)
	require.False(t, ok, "no shared endpoint")

	collinear := geom.NewLine(c, geom.NewPoint(-10, 0))
	_, ok = geom.Fillet(l1, collinear, 1)
	require.False(t, ok, "collinear corner")
}

func TestChamfer(t *testing.T) {
	c := geom.NewPoint(0, 0)
	l1 := geom.NewLine(geom.NewPoint(10, 0), c)
	l2 := geom.NewLine(c, geom.NewPoint(0, 10))

	cut, ok := geom.Chamfer(l1, l2, 3)
	require.True(t, ok, "chamfer succeeds")
	require.InDelta(t, 3, cut.Start.X+cut.Start.Y, 1e-12, "contact on a leg at distance 3")
	require.InDelta(t, 3, cut.End.X+cut.End.Y, 1e-12, "contact on the other leg at distance 3")
	require.InDelta(t, 3*math.Sqrt2, cut.Length(), 1e-12, "45° cut length")
	require.InDelta(t, 7, l1.Length(), 1e-12, "l1 shortened")
	require.InDelta(t, 7, l2.Length(), 1e-12, "l2 shortened")

	_, ok = geom.Chamfer(l1, l2, 20)
	require.False(t, ok, "distance exceeds the legs")
}
