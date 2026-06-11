package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestMirrorPoint(t *testing.T) {
	// Reflect across the x axis: (3, 2) -> (3, -2).
	axis := geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(1, 0))
	m := geom.MirrorPoint(geom.NewPoint(3, 2), axis)
	require.InDelta(t, 3, m.X, 1e-9)
	require.InDelta(t, -2, m.Y, 1e-9)

	// A point on the axis maps to itself.
	on := geom.MirrorPoint(geom.NewPoint(5, 0), axis)
	require.InDelta(t, 5, on.X, 1e-9)
	require.InDelta(t, 0, on.Y, 1e-9)

	// Reflect across the diagonal y = x: (1, 0) -> (0, 1).
	diag := geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(1, 1))
	d := geom.MirrorPoint(geom.NewPoint(1, 0), diag)
	require.InDelta(t, 0, d.X, 1e-9)
	require.InDelta(t, 1, d.Y, 1e-9)
}

func TestMirrorPointPreservesConstruction(t *testing.T) {
	p := geom.NewPoint(1, 1)
	p.Construction = true
	axis := geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(1, 0))
	require.True(t, geom.MirrorPoint(p, axis).Construction)
}

func TestTranslatePoint(t *testing.T) {
	m := geom.TranslatePoint(geom.NewPoint(2, 3), 5, -1)
	require.InDelta(t, 7, m.X, 1e-9)
	require.InDelta(t, 2, m.Y, 1e-9)
}

func TestRotatePoint(t *testing.T) {
	// Rotate (1, 0) by 90° about the origin -> (0, 1).
	m := geom.RotatePoint(geom.NewPoint(1, 0), geom.NewPoint(0, 0), math.Pi/2)
	require.InDelta(t, 0, m.X, 1e-9)
	require.InDelta(t, 1, m.Y, 1e-9)

	// Rotate about a non-origin center.
	n := geom.RotatePoint(geom.NewPoint(2, 1), geom.NewPoint(1, 1), math.Pi)
	require.InDelta(t, 0, n.X, 1e-9)
	require.InDelta(t, 1, n.Y, 1e-9)
}

func TestClosestPointOnLine(t *testing.T) {
	l := geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(10, 0))

	foot, tt := geom.ClosestPointOnLine(l, geom.NewPoint(4, 5))
	require.InDelta(t, 4, foot.X, 1e-9)
	require.InDelta(t, 0, foot.Y, 1e-9)
	require.InDelta(t, 0.4, tt, 1e-9)

	// Beyond the End endpoint: t > 1.
	_, beyond := geom.ClosestPointOnLine(l, geom.NewPoint(15, 2))
	require.Greater(t, beyond, 1.0)

	// Before the Start endpoint: t < 0.
	_, before := geom.ClosestPointOnLine(l, geom.NewPoint(-3, 2))
	require.Less(t, before, 0.0)
}

func TestSplitArcAt(t *testing.T) {
	// Quarter arc from (1,0) to (0,1) about origin; split at 45°.
	c := geom.NewPoint(0, 0)
	a := geom.NewArc(c, geom.NewPoint(1, 0), geom.NewPoint(0, 1))
	mid := geom.NewPoint(math.Sqrt2/2, math.Sqrt2/2)
	a1, a2 := geom.SplitArcAt(a, mid)

	require.Same(t, c, a1.Center)
	require.Same(t, c, a2.Center)
	require.Same(t, mid, a1.End)
	require.Same(t, mid, a2.Start)
	// Sweeps add up to the original.
	require.InDelta(t, a.Sweep(), a1.Sweep()+a2.Sweep(), 1e-9)
}

func TestSplitCircleAt(t *testing.T) {
	c := geom.NewCircle(geom.NewPoint(0, 0), 1)
	p, q := geom.NewPoint(1, 0), geom.NewPoint(-1, 0)
	a1, a2 := geom.SplitCircleAt(c, p, q)
	// The two arcs together cover the full circle.
	require.InDelta(t, 2*math.Pi, a1.Sweep()+a2.Sweep(), 1e-9)
}

func TestExtendLineTo(t *testing.T) {
	l := geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(5, 0))
	target := geom.NewPoint(10, 0)

	// Extend the End end: Start kept, End -> target.
	e := geom.ExtendLineTo(l, l.End, target)
	require.Same(t, l.Start, e.Start)
	require.Same(t, target, e.End)

	// Extend the Start end: End kept, Start -> target.
	s := geom.ExtendLineTo(l, l.Start, target)
	require.Same(t, target, s.Start)
	require.Same(t, l.End, s.End)
}
