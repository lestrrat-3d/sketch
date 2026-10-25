package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestArcAngleFromStart(t *testing.T) {
	// A quarter arc from 90° to 180° about (1, 1).
	a := geom.NewArc(geom.NewPoint(1, 1), geom.NewPoint(1, 3), geom.NewPoint(-1, 1))
	require.InDelta(t, 0, a.AngleFromStart(geom.NewPoint(1, 5)), 1e-12, "along the start direction")
	require.InDelta(t, math.Pi/4, a.AngleFromStart(geom.NewPoint(0, 2)), 1e-12)
	require.InDelta(t, math.Pi/2, a.AngleFromStart(geom.NewPoint(-4, 1)), 1e-12, "the end direction")
	require.InDelta(t, 3*math.Pi/2, a.AngleFromStart(geom.NewPoint(2, 1)), 1e-12, "counter-clockwise past the end")
	require.True(t, a.Contains(geom.NewPoint(0, 2)))
	require.False(t, a.Contains(geom.NewPoint(2, 1)))
}
