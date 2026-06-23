package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestDistancePointCircle(t *testing.T) {
	// A point on the x-axis is held a signed radial distance from a fixed r=5
	// circle: +3 puts it at |P−C| = 8 (outside), −2 at |P−C| = 3 (inside).
	for _, tc := range []struct {
		name string
		d    float64
		want float64
	}{
		{"outside", 3, 8},
		{"inside", -2, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			cc := s.CreatePoint(0, 0)
			circle := s.CreateCircle(cc, 5)
			s.FixEntity(circle)
			p := s.CreatePoint(10, 0)
			s.AddConstraint(sketch.NewHorizontalPoints(p, cc)) // p.y = 0
			s.AddConstraint(sketch.NewDistancePointCircle(p, circle, tc.d))

			_, err := s.Solve()
			require.NoError(t, err)
			require.InDelta(t, tc.want, p.X(), 1e-6, "radial gap from the circle edge")
		})
	}
}

func TestDistanceLineCircle(t *testing.T) {
	// A horizontal line above a fixed r=5 circle (center origin) is held at a
	// tangent gap d: dist(center, line) = r + d, so a horizontal line settles at
	// y = 5 + d. d=0 is tangency.
	for _, tc := range []struct {
		name string
		d    float64
		want float64
	}{
		{"tangent", 0, 5},
		{"gap", 2, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			circle := s.CreateCircle(s.CreatePoint(0, 0), 5)
			s.FixEntity(circle)
			p1 := s.CreatePoint(-10, 10)
			p2 := s.CreatePoint(10, 10)
			line := s.CreateLine(p1, p2)
			s.AddConstraint(sketch.NewHorizontal(line))
			s.AddConstraint(sketch.NewDistanceLineCircle(line, circle, tc.d))

			_, err := s.Solve()
			require.NoError(t, err)
			require.InDelta(t, tc.want, p1.Y(), 1e-6, "line at center-distance r+d above the circle")
			require.InDelta(t, p1.Y(), p2.Y(), 1e-9, "still horizontal")
		})
	}
}

func TestDistanceCircleDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	cc := s.CreatePoint(0, 0)
	circle := s.CreateCircle(cc, 5)
	s.FixEntity(circle)
	p := s.CreatePoint(10, 0)
	s.AddConstraint(sketch.NewHorizontalPoints(p, cc))
	require.Equal(t, 1, s.DOF(), "the point slides along the x-axis")

	con := sketch.NewDistancePointCircle(p, circle, 3)
	s.AddConstraint(con)
	require.Equal(t, 0, s.DOF(), "the distance removes the remaining DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 1, s.DOF(), "removal restores the DOF")
}

func TestDistancePointCircleDriven(t *testing.T) {
	// A driven (reference) distance-point-circle measures the radial gap of fixed
	// geometry: P at (8,0), r=5 circle at origin → gap 3.
	s := newSketch(t)
	circle := s.CreateCircle(s.CreatePoint(0, 0), 5)
	s.FixEntity(circle)
	p := s.CreatePoint(8, 0)
	s.Fix(p)
	dim := sketch.NewDistancePointCircle(p, circle, 0)
	dim.SetDriven(true)
	s.AddConstraint(dim)

	_, err := s.Solve()
	require.NoError(t, err)
	require.True(t, dim.Driven())
	require.InDelta(t, 3, dim.Target().Mag(), 1e-6, "measures |P−C| − r")
}

func TestDistanceCircleRoundTrip(t *testing.T) {
	s := newSketch(t)
	cc := s.CreatePoint(0, 0)
	circle := s.CreateCircle(cc, 5)
	s.FixEntity(circle)
	p := s.CreatePoint(10, 0)
	s.AddConstraint(sketch.NewHorizontalPoints(p, cc))
	s.AddConstraint(sketch.NewDistancePointCircle(p, circle, 3))
	p1 := s.CreatePoint(-10, 10)
	p2 := s.CreatePoint(10, 10)
	line := s.CreateLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewDistanceLineCircle(line, circle, 2))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive reload")
	_, err = s2.Solve()
	require.NoError(t, err)
	for i, p := range s.Points() {
		require.InDeltaf(t, p.X(), s2.Points()[i].X(), 1e-6, "point %d X", i)
		require.InDeltaf(t, p.Y(), s2.Points()[i].Y(), 1e-6, "point %d Y", i)
	}
}
