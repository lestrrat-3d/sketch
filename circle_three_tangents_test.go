package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestCreateCircleThreeTangents(t *testing.T) {
	t.Run("incircle, fully constrained and parametric", func(t *testing.T) {
		s := newSketch(t)
		a, b, c := s.CreatePoint(0, 0), s.CreatePoint(12, 0), s.CreatePoint(0, 9)
		ab, bc, ca := s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, a)
		base := sketch.NewDistance(a, b, 12)
		s.AddConstraint(
			sketch.NewCoincident(a, s.Origin()),
			sketch.NewHorizontal(ab),
			sketch.NewVertical(ca),
			base,
			sketch.NewDistance(a, c, 9),
		)
		require.Equal(t, 0, s.DOF(), "the triangle is fully constrained")

		circle, err := s.CreateCircleThreeTangents(ab, bc, ca, 2, 2)
		require.NoError(t, err)
		require.InDelta(t, 3, circle.Center.X(), 1e-9, "center x")
		require.InDelta(t, 3, circle.Center.Y(), 1e-9, "center y")
		require.InDelta(t, 3, circle.R(), 1e-9, "inradius (12 + 9 - 15) / 2")
		require.Equal(t, 0, s.DOF(), "the circle adds no freedom")

		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res.DOF)
		require.Empty(t, s.RedundantConstraints())
		require.Nil(t, s.Verify(t.Context()).Check())
		require.Len(t, s.Profiles(), 2, "the disk and the triangle around it")

		base.Set(16)
		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		want := (16 + 9 - math.Hypot(16, 9)) / 2 // 3.3212...
		require.InDelta(t, want, circle.R(), 1e-6, "inradius follows the triangle")
		require.InDelta(t, want, circle.Center.X(), 1e-6, "center x")
		require.InDelta(t, want, circle.Center.Y(), 1e-6, "center y")

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

	t.Run("hint picks an excircle", func(t *testing.T) {
		s := newSketch(t)
		a, b, c := s.CreatePoint(0, 0), s.CreatePoint(12, 0), s.CreatePoint(0, 9)
		ab, bc, ca := s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, a)
		circle, err := s.CreateCircleThreeTangents(ab, bc, ca, 10, 10)
		require.NoError(t, err)
		require.InDelta(t, 18, circle.Center.X(), 1e-9, "center x")
		require.InDelta(t, 18, circle.Center.Y(), 1e-9, "center y")
		require.InDelta(t, 18, circle.R(), 1e-9, "exradius across BC")
	})

	t.Run("refusals", func(t *testing.T) {
		s := newSketch(t)
		a, b, c := s.CreatePoint(0, 0), s.CreatePoint(12, 0), s.CreatePoint(0, 9)
		ab, bc, ca := s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, a)
		top := s.CreateLine(s.CreatePoint(0, 20), s.CreatePoint(12, 20)) // parallel to ab
		q := s.CreatePoint(5, 5)
		z := s.CreateLine(q, q) // zero length
		// Three lines through (0, 0).
		d1 := s.CreateLine(s.CreatePoint(-5, -5), s.CreatePoint(5, 5))
		counts := func() [3]int { return [3]int{len(s.Points()), len(s.Entities()), len(s.Constraints())} }
		before := counts()
		fl := foreignLine(t, 0, 0, 12, 0)
		cases := []struct {
			name       string
			l1, l2, l3 *sketch.Line
			hx, hy     float64
			want       error
		}{
			{"foreign line", fl, bc, ca, 2, 2, sketch.ErrForeignEntity},
			{"nil line", ab, nil, ca, 2, 2, sketch.ErrForeignEntity},
			{"same line twice", ab, ab, ca, 2, 2, sketch.ErrInvalidShape},
			{"zero-length line", ab, z, ca, 2, 2, sketch.ErrInvalidShape},
			{"parallel pair", ab, top, ca, 2, 2, sketch.ErrInvalidShape},
			{"hint across a vertex", ab, bc, ca, -1, -1, sketch.ErrInvalidShape},
			{"hint on a line", ab, bc, ca, 5, 0, sketch.ErrInvalidShape},
			{"concurrent lines", ab, ca, d1, 3, 1, sketch.ErrInvalidShape},
			{"NaN hint", ab, bc, ca, math.NaN(), 2, sketch.ErrInvalidShape},
			{"Inf hint", ab, bc, ca, 2, math.Inf(1), sketch.ErrInvalidShape},
		}
		for _, tc := range cases {
			circle, err := s.CreateCircleThreeTangents(tc.l1, tc.l2, tc.l3, tc.hx, tc.hy)
			require.ErrorIs(t, err, tc.want, tc.name)
			require.Nil(t, circle, tc.name)
			require.Equal(t, before, counts(), "%s: the sketch is unchanged", tc.name)
		}
	})
}
