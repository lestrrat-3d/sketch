package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Two arcs tangent at a shared endpoint: the tangency must remove exactly one
// degree of freedom and must not read as redundant.
func TestTangentCirclesSharedEndpointRank(t *testing.T) {
	cases := []struct {
		name     string
		internal bool
		c2x      float64
		e2x      float64
		e2y      float64
	}{
		{"external", false, 10, 10, 5},
		{"internal", true, 3, 3, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			c1 := s.CreatePoint(0, 0)
			s1 := s.CreatePoint(0, 5)
			p := s.CreatePoint(5, 0) // the shared endpoint, already tangent
			a1 := s.CreateArc(c1, s1, p)
			c2 := s.CreatePoint(tc.c2x, 0)
			e2 := s.CreatePoint(tc.e2x, tc.e2y)
			a2 := s.CreateArc(c2, p, e2)
			require.Equal(t, 8, s.DOF(), "two free arcs sharing one point")

			s.AddConstraint(sketch.NewTangentCircles(a1, a2, tc.internal))
			require.Equal(t, 7, s.DOF(), "tangency removes one degree of freedom")
			require.Empty(t, s.RedundantConstraints(), "tangency is not redundant")
		})
	}
}

// A shared-endpoint tangency drives a free arc onto the tangent configuration
// on the requested side.
func TestTangentCirclesSharedEndpointSolves(t *testing.T) {
	cases := []struct {
		name     string
		internal bool
		seedC2   [2]float64
		seedE2   [2]float64
		wantC2Y  float64
		wantE2   [2]float64
	}{
		{"external", false, [2]float64{1, 9}, [2]float64{3.5, 8.5}, 8, [2]float64{3, 8}},
		{"internal", true, [2]float64{1, 2.5}, [2]float64{3.5, 2.5}, 2, [2]float64{3, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			c1 := s.CreatePoint(0, 0)
			s1 := s.CreatePoint(5, 0)
			p := s.CreatePoint(0, 5)
			a1 := s.CreateArc(c1, s1, p)
			s.AddConstraint(
				sketch.NewCoincident(c1, s.Origin()),
				sketch.NewHorizontalPoints(c1, s1),
				sketch.NewDistance(c1, s1, 5),
				sketch.NewVerticalPoints(c1, p),
			)

			c2 := s.CreatePoint(tc.seedC2[0], tc.seedC2[1])
			e2 := s.CreatePoint(tc.seedE2[0], tc.seedE2[1])
			a2 := s.CreateArc(c2, p, e2)
			s.AddConstraint(
				sketch.NewTangentCircles(a1, a2, tc.internal),
				sketch.NewDistance(c2, p, 3),
				sketch.NewHorizontalPoints(c2, e2),
			)

			res, err := s.Solve(t.Context())
			require.NoError(t, err)
			require.Equal(t, 0, res.DOF, "fully constrained")
			require.Empty(t, s.RedundantConstraints())
			require.InDelta(t, 0, c2.X(), 1e-6, "c2 on the line through c1 and p")
			require.InDelta(t, tc.wantC2Y, c2.Y(), 1e-6, "c2 on the requested side of p")
			require.InDelta(t, tc.wantE2[0], e2.X(), 1e-6, "e2.X")
			require.InDelta(t, tc.wantE2[1], e2.Y(), 1e-6, "e2.Y")
			require.InDelta(t, 3, a2.R(), 1e-6, "a2 radius")
			require.InDelta(t, 5, a1.R(), 1e-6, "a1 radius")
		})
	}
}

// A degenerate shared-endpoint tangency (coincident centers) is never reported
// as satisfied.
func TestTangentCirclesSharedEndpointCoincidentCenters(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	p := s.CreatePoint(5, 0)
	a1 := s.CreateArc(c, s.CreatePoint(0, 5), p)
	a2 := s.CreateArc(c, p, s.CreatePoint(-5, 0))
	tan := sketch.NewTangentCircles(a1, a2, true)
	s.AddConstraint(tan)
	res := sketch.ConstraintResiduals(tan)
	require.Len(t, res, 1)
	require.InDelta(t, 1, math.Abs(res[0]), 1e-12, "coincident centers keep the residual at 1")
}
