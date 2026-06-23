package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// A closed-spline profile reports the exact enclosed area through Sketch.Profiles
// — the area now flows from the analytic ½∫(x·y′−y·x′) integral, matching a
// dense-polyline reference rather than the old sampled bulge.
func TestProfileClosedSplineAreaExact(t *testing.T) {
	s := newSketch(t)
	pts := []*sketch.Point{
		s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(5, 3),
		s.CreatePoint(2, 5), s.CreatePoint(-1, 3),
	}
	_, err := s.CreateClosedSpline(pts...)
	require.NoError(t, err)

	profiles := s.Profiles()
	require.Len(t, profiles, 1)
	require.True(t, profiles[0].Valid)

	ctrl := [][2]float64{{0, 0}, {4, 0}, {5, 3}, {2, 5}, {-1, 3}}
	ring, err := geom.SamplePeriodicCubicBSpline(ctrl, 200000)
	require.NoError(t, err)
	var sum float64
	n := len(ring) - 1 // drop the repeated closing point
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		sum += ring[i][0]*ring[j][1] - ring[j][0]*ring[i][1]
	}
	require.InDelta(t, math.Abs(sum/2), profiles[0].Area, 1e-6)
}
