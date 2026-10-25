package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// arcLeavesAlong reports the dot product of the arc's unit tangent at p (the
// direction the arc moves away from p) with the unit direction (dx, dy).
func arcLeavesAlong(a *sketch.Arc, p *sketch.Point, dx, dy float64) float64 {
	rx, ry := p.X()-a.Center.X(), p.Y()-a.Center.Y()
	// Counter-clockwise travel direction at p is the radius turned left.
	tx, ty := -ry, rx
	if p == a.End { // leaving from End means travelling clockwise
		tx, ty = ry, -rx
	}
	tl, dl := math.Hypot(tx, ty), math.Hypot(dx, dy)
	return (tx*dx + ty*dy) / (tl * dl)
}

func TestCreateArcTangent(t *testing.T) {
	cases := []struct {
		name      string
		ey        float64
		wantStart bool // at is the arc's Start (left turn)
		centerY   float64
		solvedY   float64 // center and end y after the radius is driven to 8
	}{
		{"left turn", 5, true, 5, 8},
		{"right turn", -5, false, -5, -8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			p0, p1 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
			l := s.CreateLine(p0, p1)
			s.AddConstraint(
				sketch.NewCoincident(p0, s.Origin()),
				sketch.NewHorizontal(l),
				sketch.NewDistance(p0, p1, 10),
			)
			require.Equal(t, 0, s.DOF(), "the line is fully constrained")

			arc, err := s.CreateArcTangent(l, p1, 15, tc.ey)
			require.NoError(t, err)
			if tc.wantStart {
				require.Same(t, p1, arc.Start)
			} else {
				require.Same(t, p1, arc.End)
			}
			require.InDelta(t, 10, arc.Center.X(), 1e-9, "center x")
			require.InDelta(t, tc.centerY, arc.Center.Y(), 1e-9, "center y")
			require.InDelta(t, 5, arc.R(), 1e-9, "radius")
			require.InDelta(t, math.Pi/2, arc.Sweep(), 1e-9, "quarter turn")
			require.InDelta(t, 1, arcLeavesAlong(arc, p1, 1, 0), 1e-9, "leaves p1 along +x")
			require.Equal(t, 2, s.DOF(), "the end point is free")

			end := arc.End
			if !tc.wantStart {
				end = arc.Start
			}
			radius := sketch.NewRadius(arc, 8)
			s.AddConstraint(radius, sketch.NewHorizontalPoints(arc.Center, end))
			res, err := s.Solve(t.Context())
			require.NoError(t, err)
			require.Equal(t, 0, res.DOF, "fully constrained")
			require.Empty(t, s.RedundantConstraints())
			require.InDelta(t, 10, arc.Center.X(), 1e-6, "center x")
			require.InDelta(t, tc.solvedY, arc.Center.Y(), 1e-6, "center y")
			require.InDelta(t, 18, end.X(), 1e-6, "end x")
			require.InDelta(t, tc.solvedY, end.Y(), 1e-6, "end y")
			require.Greater(t, arcLeavesAlong(arc, p1, 1, 0), 0.999999, "still a smooth continuation, not a cusp")
			require.Nil(t, s.Verify(t.Context()).Check())

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

	t.Run("from the line's start", func(t *testing.T) {
		s := newSketch(t)
		p0, p1 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
		l := s.CreateLine(p0, p1)
		arc, err := s.CreateArcTangent(l, p0, -5, -5)
		require.NoError(t, err)
		require.Same(t, p0, arc.Start, "travelling -x, (-5, -5) is a left turn")
		require.InDelta(t, 0, arc.Center.X(), 1e-9, "center x")
		require.InDelta(t, -5, arc.Center.Y(), 1e-9, "center y")
		require.InDelta(t, 1, arcLeavesAlong(arc, p0, -1, 0), 1e-9, "leaves p0 along -x")
	})

	t.Run("refusals", func(t *testing.T) {
		s := newSketch(t)
		p0, p1 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
		l := s.CreateLine(p0, p1)
		q := s.CreatePoint(3, 3)
		z := s.CreateLine(q, q) // zero length
		counts := func() [3]int { return [3]int{len(s.Points()), len(s.Entities()), len(s.Constraints())} }
		before := counts()
		fl := foreignLine(t, 0, 0, 10, 0)
		cases := []struct {
			name string
			l    *sketch.Line
			at   *sketch.Point
			x, y float64
			want error
		}{
			{"foreign line", fl, fl.End, 15, 5, sketch.ErrForeignEntity},
			{"nil line", nil, p1, 15, 5, sketch.ErrForeignEntity},
			{"at not an endpoint", l, q, 15, 5, sketch.ErrInvalidShape},
			{"zero-length line", z, q, 15, 5, sketch.ErrInvalidShape},
			{"end ahead on the line", l, p1, 20, 0, sketch.ErrInvalidShape},
			{"end behind on the line", l, p1, -5, 0, sketch.ErrInvalidShape},
			{"end on at", l, p1, 10, 0, sketch.ErrInvalidShape},
			{"NaN end", l, p1, math.NaN(), 5, sketch.ErrInvalidShape},
			{"Inf end", l, p1, 15, math.Inf(-1), sketch.ErrInvalidShape},
		}
		for _, tc := range cases {
			arc, err := s.CreateArcTangent(tc.l, tc.at, tc.x, tc.y)
			require.ErrorIs(t, err, tc.want, tc.name)
			require.Nil(t, arc, tc.name)
			require.Equal(t, before, counts(), "%s: the sketch is unchanged", tc.name)
		}
	})
}
