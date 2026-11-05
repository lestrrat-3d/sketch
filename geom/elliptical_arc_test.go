package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestEllipticalArcSampling(t *testing.T) {
	// Top half of an ellipse rx=4, ry=2: from (4,0) (param 0) to (-4,0) (param π).
	right := geom.NewPoint(4, 0)
	left := geom.NewPoint(-4, 0)
	ea := geom.NewEllipticalArc(geom.NewPoint(0, 0), right, left, 4, 2, 0)

	require.InDelta(t, 0, ea.StartParam(), 1e-9)
	require.InDelta(t, math.Pi, ea.EndParam(), 1e-9)
	require.InDelta(t, math.Pi, ea.Sweep(), 1e-9, "half turn")

	pts := ea.Polyline(64)
	require.InDelta(t, 4, pts[0][0], 1e-9, "starts at Start")
	require.InDelta(t, 0, pts[0][1], 1e-9)
	require.InDelta(t, -4, pts[len(pts)-1][0], 1e-9, "ends at End")
	require.InDelta(t, 0, pts[len(pts)-1][1], 1e-9)
	// Every sampled point lies on the ellipse (x/4)² + (y/2)² = 1, with y ≥ 0.
	for _, p := range pts {
		f := (p[0]/4)*(p[0]/4) + (p[1]/2)*(p[1]/2)
		require.InDelta(t, 1, f, 1e-9, "on the ellipse")
		require.GreaterOrEqual(t, p[1], -1e-9, "top half only")
	}
}

func TestEllipticalArcRegion(t *testing.T) {
	// A half-ellipse region closed by a chord along the major axis.
	right := geom.NewPoint(4, 0)
	left := geom.NewPoint(-4, 0)
	ea := geom.NewEllipticalArc(geom.NewPoint(0, 0), right, left, 4, 2, 0)
	line := geom.NewLine(left, right)

	arr := geom.Regions([]geom.Curve{line, ea}, nil)
	require.Len(t, arr.Regions, 1, "the arc plus its chord close one region")
	require.InDelta(t, 0.5*math.Pi*4*2, arr.Regions[0].Area, 1e-2, "half the ellipse area")
}
