package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestCreateArcThreePoints(t *testing.T) {
	t.Run("counter-clockwise placement", func(t *testing.T) {
		s := newSketch(t)
		start, end := s.CreatePoint(10, 0), s.CreatePoint(-10, 0)
		arc, err := s.CreateArcThreePoints(start, 0, 10, end)
		require.NoError(t, err)
		require.Same(t, start, arc.Start, "start kept as Start")
		require.Same(t, end, arc.End, "end kept as End")
		require.InDelta(t, 0, arc.Center.X(), 1e-9, "center x")
		require.InDelta(t, 0, arc.Center.Y(), 1e-9, "center y")
		require.InDelta(t, 10, arc.R(), 1e-9, "radius")
		require.InDelta(t, math.Pi, arc.Sweep(), 1e-9, "sweep")
		require.Len(t, s.Points(), 3, "start, end and the new center")
		require.Equal(t, 5, s.DOF(), "a free arc")
	})

	t.Run("clockwise mid swaps the ends", func(t *testing.T) {
		s := newSketch(t)
		start, end := s.CreatePoint(10, 0), s.CreatePoint(-10, 0)
		arc, err := s.CreateArcThreePoints(start, 0, -10, end)
		require.NoError(t, err)
		require.Same(t, end, arc.Start, "end becomes Start")
		require.Same(t, start, arc.End, "start becomes End")
		require.InDelta(t, math.Pi, arc.Sweep(), 1e-9, "sweep through (0, -10)")
		require.InDelta(t, -math.Pi/2, arc.StartAngle()+arc.Sweep()/2-2*math.Pi, 1e-9, "midpoint of the sweep at -90 degrees")
	})

	t.Run("fully constrained and parametric", func(t *testing.T) {
		s := newSketch(t)
		start, end := s.CreatePoint(10, 0), s.CreatePoint(-10, 0)
		arc, err := s.CreateArcThreePoints(start, 0, 10, end)
		require.NoError(t, err)
		radius := sketch.NewRadius(arc, 12)
		s.AddConstraint(
			sketch.NewCoincident(arc.Center, s.Origin()),
			radius,
			sketch.NewHorizontalPoints(arc.Center, start),
			sketch.NewHorizontalPoints(arc.Center, end),
		)
		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res.DOF, "fully constrained")
		require.Empty(t, s.RedundantConstraints())
		require.InDelta(t, 12, start.X(), 1e-6, "start x")
		require.InDelta(t, 0, start.Y(), 1e-6, "start y")
		require.InDelta(t, -12, end.X(), 1e-6, "end x")
		require.InDelta(t, 0, end.Y(), 1e-6, "end y")
		require.Nil(t, s.Verify(t.Context()).Check())

		radius.Set(15)
		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 15, start.X(), 1e-6, "start follows the radius")
		require.InDelta(t, -15, end.X(), 1e-6, "end follows the radius")

		data, err := json.Marshal(s)
		require.NoError(t, err)
		var s2 sketch.Sketch
		require.NoError(t, json.Unmarshal(data, &s2))
		require.Len(t, s2.Entities(), len(s.Entities()))
		require.Len(t, s2.Constraints(), len(s.Constraints()))
		res2, err := s2.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res2.DOF, "reloaded DOF")
	})

	t.Run("origin as start", func(t *testing.T) {
		s := newSketch(t)
		end := s.CreatePoint(10, 0)
		arc, err := s.CreateArcThreePoints(s.Origin(), 5, 5, end)
		require.NoError(t, err)
		require.Same(t, end, arc.Start, "(5, 5) lies clockwise of origin -> (10, 0)")
		require.Same(t, s.Origin(), arc.End)
		require.InDelta(t, 5, arc.Center.X(), 1e-9, "center x")
		require.InDelta(t, 0, arc.Center.Y(), 1e-9, "center y")
	})

	t.Run("refusals", func(t *testing.T) {
		s := newSketch(t)
		a, b := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
		counts := func() [3]int { return [3]int{len(s.Points()), len(s.Entities()), len(s.Constraints())} }
		before := counts()
		cases := []struct {
			name   string
			start  *sketch.Point
			mx, my float64
			end    *sketch.Point
			want   error
		}{
			{"foreign start", foreignPoint(t, 0, 0, 0), 5, 5, b, sketch.ErrForeignEntity},
			{"foreign end", a, 5, 5, foreignPoint(t, 0, 10, 0), sketch.ErrForeignEntity},
			{"nil end", a, 5, 5, nil, sketch.ErrForeignEntity},
			{"same start and end", a, 5, 5, a, sketch.ErrInvalidShape},
			{"collinear", a, 5, 0, b, sketch.ErrInvalidShape},
			{"mid on start", a, 0, 0, b, sketch.ErrInvalidShape},
			{"mid on end", a, 10, 0, b, sketch.ErrInvalidShape},
			{"NaN mid", a, math.NaN(), 5, b, sketch.ErrInvalidShape},
			{"Inf mid", a, 5, math.Inf(1), b, sketch.ErrInvalidShape},
		}
		for _, tc := range cases {
			arc, err := s.CreateArcThreePoints(tc.start, tc.mx, tc.my, tc.end)
			require.ErrorIs(t, err, tc.want, tc.name)
			require.Nil(t, arc, tc.name)
			require.Equal(t, before, counts(), "%s: the sketch is unchanged", tc.name)
		}
	})
}
