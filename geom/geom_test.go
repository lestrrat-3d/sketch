package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestLineLength(t *testing.T) {
	l := geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(3, 4))
	require.InDelta(t, 5, l.Length(), 1e-9, "length")
}

func TestArcMetrics(t *testing.T) {
	a := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(5, 0), geom.NewPoint(0, 5))
	require.InDelta(t, 5, a.Radius(), 1e-9, "radius")
	require.InDelta(t, 0, a.StartAngle(), 1e-9, "start angle")
	require.InDelta(t, math.Pi/2, a.EndAngle(), 1e-9, "end angle")
	require.InDelta(t, math.Pi/2, a.Sweep(), 1e-9, "sweep")
}

func TestSharedPointIdentity(t *testing.T) {
	v := geom.NewPoint(1, 1)
	l1 := geom.NewLine(geom.NewPoint(0, 0), v)
	l2 := geom.NewLine(v, geom.NewPoint(2, 2))
	require.Same(t, l1.End, l2.Start, "shared vertex should be the same *Point")
}

func TestFullCircleSweep(t *testing.T) {
	// start == end means a full turn, reported as 2π rather than 0.
	a := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(1, 0), geom.NewPoint(1, 0))
	require.InDelta(t, 2*math.Pi, a.Sweep(), 1e-9, "full sweep")
}
