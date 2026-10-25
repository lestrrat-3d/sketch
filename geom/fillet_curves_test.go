package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestFilletLineArc(t *testing.T) {
	// Arc about the origin, R=10, from its corner Start (10,0) to (0,10); the line
	// runs from its kept end (20,0) to the same corner.
	a := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(10, 0), geom.NewPoint(0, 10))
	l := geom.NewLine(geom.NewPoint(20, 0), geom.NewPoint(10, 0))

	f, ok := geom.FilletLineArc(l, a, true, 2)
	require.True(t, ok)
	require.InDelta(t, math.Sqrt(140), f.Arc.Center.X, 1e-9)
	require.InDelta(t, 2, f.Arc.Center.Y, 1e-9)
	require.InDelta(t, math.Sqrt(140), f.T1.X, 1e-9)
	require.InDelta(t, 0, f.T1.Y, 1e-9)
	require.InDelta(t, 5*math.Sqrt(140)/6, f.T2.X, 1e-9)
	require.InDelta(t, 5.0/3, f.T2.Y, 1e-9)
	require.False(t, f.Inside1)
	require.False(t, f.Inside2, "the fillet sits outside the arc's circle")
	require.InDelta(t, 2, f.Arc.Radius(), 1e-9)
	require.True(t, (f.Arc.Start == f.T1 && f.Arc.End == f.T2) || (f.Arc.Start == f.T2 && f.Arc.End == f.T1))
	require.LessOrEqual(t, f.Arc.Sweep(), math.Pi)
	require.InDelta(t, 20, l.Start.X, 0, "the inputs are not changed")
	require.InDelta(t, 10, a.Start.X, 0)
}

func TestFilletArcArc(t *testing.T) {
	a1 := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(10, 0), geom.NewPoint(0, 10))
	h := 10 - 5*math.Sqrt2
	a2 := geom.NewArc(geom.NewPoint(10, 10), geom.NewPoint(h, h), geom.NewPoint(10, 0))

	f, ok := geom.FilletArcArc(a1, true, a2, false, 2)
	require.True(t, ok)
	ox, oy := 5+math.Sqrt(7), 5-math.Sqrt(7)
	require.InDelta(t, ox, f.Arc.Center.X, 1e-9)
	require.InDelta(t, oy, f.Arc.Center.Y, 1e-9)
	require.InDelta(t, ox*10/8, f.T1.X, 1e-9)
	require.InDelta(t, oy*10/8, f.T1.Y, 1e-9)
	require.InDelta(t, 10+(ox-10)*10/8, f.T2.X, 1e-9)
	require.InDelta(t, 10+(oy-10)*10/8, f.T2.Y, 1e-9)
	require.True(t, f.Inside1)
	require.True(t, f.Inside2)
}

func TestFilletCurvesRefuse(t *testing.T) {
	a := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(10, 0), geom.NewPoint(0, 10))
	l := geom.NewLine(geom.NewPoint(20, 0), geom.NewPoint(10, 0))

	_, ok := geom.FilletLineArc(l, a, true, 0)
	require.False(t, ok, "radius must be positive")
	_, ok = geom.FilletLineArc(l, a, true, 50)
	require.False(t, ok, "radius too large for the legs")
	tangent := geom.NewLine(geom.NewPoint(10, -10), geom.NewPoint(10, 0)) // continues the arc's tangent
	_, ok = geom.FilletLineArc(tangent, a, true, 2)
	require.False(t, ok, "tangent at the corner: nothing to round")
	elsewhere := geom.NewLine(geom.NewPoint(20, 5), geom.NewPoint(11, 5))
	_, ok = geom.FilletLineArc(elsewhere, a, true, 2)
	require.False(t, ok, "the curves do not meet at the corner")
}
