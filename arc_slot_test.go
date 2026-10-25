package sketch_test

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestCreateArcSlot(t *testing.T) {
	for _, sweep := range []float64{30, 90, 270} {
		t.Run(fmt.Sprintf("sweep %v", sweep), func(t *testing.T) {
			s := newSketch(t)
			rad := sweep * math.Pi / 180
			sl, err := s.CreateArcSlot(0, 0, 20, 0, 20*math.Cos(rad), 20*math.Sin(rad), 3)
			require.NoError(t, err)

			// Exact placement.
			require.InDelta(t, 20, sl.C1.X(), 1e-9, "C1 x")
			require.InDelta(t, 0, sl.C1.Y(), 1e-9, "C1 y")
			require.InDelta(t, 20*math.Cos(rad), sl.C2.X(), 1e-9, "C2 x")
			require.InDelta(t, 20*math.Sin(rad), sl.C2.Y(), 1e-9, "C2 y")
			require.InDelta(t, 23, sl.Outer.R(), 1e-9, "outer radius")
			require.InDelta(t, 17, sl.Inner.R(), 1e-9, "inner radius")
			require.InDelta(t, 3, sl.A1.R(), 1e-9, "cap 1 radius")
			require.InDelta(t, 3, sl.A2.R(), 1e-9, "cap 2 radius")
			require.InDelta(t, rad, sl.Outer.Sweep(), 1e-9, "outer sweep")
			require.InDelta(t, math.Pi, sl.A1.Sweep(), 1e-9, "cap 1 is a half turn")
			require.InDelta(t, math.Pi, sl.A2.Sweep(), 1e-9, "cap 2 is a half turn")
			require.Len(t, sl.Spokes, 2)
			require.True(t, sl.Spokes[0].IsConstruction())
			require.True(t, sl.Spokes[1].IsConstruction())
			require.Equal(t, 6, s.DOF(), "center, centerline radius, start angle, sweep, cap radius")
			require.Empty(t, s.RedundantConstraints())

			// Ground through the origin and dimension the five remaining freedoms.
			radius := sketch.NewDistance(sl.Center, sl.C1, 20)
			capR := sketch.NewRadius(sl.A1, 3)
			angle := sketch.NewAngle(sl.Spokes[0], sl.Spokes[1], sweep)
			s.AddConstraint(
				sketch.NewCoincident(sl.Center, s.Origin()),
				sketch.NewHorizontal(sl.Spokes[0]),
				angle, radius, capR,
			)
			res, err := s.Solve(t.Context())
			require.NoError(t, err)
			require.Equal(t, 0, res.DOF, "fully constrained")
			require.Empty(t, s.RedundantConstraints())
			require.Nil(t, s.Verify(t.Context()).Check())
			profiles := s.Profiles()
			require.Len(t, profiles, 1)
			require.InDelta(t, rad*20*6+math.Pi*9, profiles[0].Area, 1e-4, "sweep*R*2r + pi*r^2")

			// Parametric: radius, width and sweep all reflow.
			radius.Set(25)
			capR.Set(4)
			_, err = s.Solve(t.Context())
			require.NoError(t, err)
			require.InDelta(t, 25*math.Cos(rad), sl.C2.X(), 1e-6, "C2 x after R=25")
			require.InDelta(t, 25*math.Sin(rad), sl.C2.Y(), 1e-6, "C2 y after R=25")
			require.InDelta(t, 29, sl.Outer.R(), 1e-6, "outer radius")
			require.InDelta(t, 21, sl.Inner.R(), 1e-6, "inner radius")
			require.InDelta(t, 4, sl.A2.R(), 1e-6, "cap 2 follows cap 1")

			angle.Set(sweep + 20)
			_, err = s.Solve(t.Context())
			require.NoError(t, err)
			require.InDelta(t, rad+20*math.Pi/180, sl.Outer.Sweep(), 1e-6, "sweep follows the angle")

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
	}

	t.Run("refusals", func(t *testing.T) {
		s := newSketch(t)
		c350 := 20 * math.Cos(350*math.Pi/180)
		s350 := 20 * math.Sin(350*math.Pi/180)
		cases := []struct {
			name                      string
			cx, cy, x1, y1, x2, y2, r float64
		}{
			{"start on the center", 0, 0, 0, 0, 0, 20, 3},
			{"end on the center", 0, 0, 20, 0, 0, 0, 3},
			{"zero cap radius", 0, 0, 20, 0, 0, 20, 0},
			{"negative cap radius", 0, 0, 20, 0, 0, 20, -1},
			{"cap radius equal to the centerline radius", 0, 0, 20, 0, 0, 20, 20},
			{"full turn", 0, 0, 20, 0, 30, 0, 3},
			{"caps overlap across the gap", 0, 0, 20, 0, c350, s350, 3},
			{"NaN", 0, 0, math.NaN(), 0, 0, 20, 3},
			{"Inf", 0, 0, 20, 0, 0, 20, math.Inf(1)},
		}
		for _, tc := range cases {
			sl, err := s.CreateArcSlot(tc.cx, tc.cy, tc.x1, tc.y1, tc.x2, tc.y2, tc.r)
			require.ErrorIs(t, err, sketch.ErrInvalidShape, tc.name)
			require.Nil(t, sl, tc.name)
			require.Empty(t, s.Points(), "%s: no point created", tc.name)
			require.Empty(t, s.Entities(), "%s: no entity created", tc.name)
			require.Empty(t, s.Constraints(), "%s: no constraint created", tc.name)
		}
	})
}
