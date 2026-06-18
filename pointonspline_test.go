package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// archSpline builds a fixed 4-control-point cubic B-spline arch and returns it
// with its control points already grounded (a rigid curve to attach points to).
func archSpline(s *sketch.Sketch) *sketch.Spline {
	c0 := s.AddPoint(0, 0)
	c1 := s.AddPoint(2, 4)
	c2 := s.AddPoint(6, 4)
	c3 := s.AddPoint(8, 0)
	sp, err := s.AddSpline(c0, c1, c2, c3)
	if err != nil {
		panic(err)
	}
	for _, c := range []*sketch.Point{c0, c1, c2, c3} {
		s.Fix(c)
	}
	return sp
}

// distToSpline returns the perpendicular distance from p to a densely sampled
// polyline of the spline (segment projection, so it is accurate between samples).
func distToSpline(p *sketch.Point, sp *sketch.Spline) float64 {
	poly := sp.Polyline(400)
	best := math.Inf(1)
	for i := 1; i < len(poly); i++ {
		ax, ay := poly[i-1][0], poly[i-1][1]
		bx, by := poly[i][0], poly[i][1]
		dx, dy := bx-ax, by-ay
		seg2 := dx*dx + dy*dy
		u := 0.0
		if seg2 > 0 {
			u = math.Max(0, math.Min(1, ((p.X()-ax)*dx+(p.Y()-ay)*dy)/seg2))
		}
		cx, cy := ax+u*dx, ay+u*dy
		if d := math.Hypot(p.X()-cx, p.Y()-cy); d < best {
			best = d
		}
	}
	return best
}

func TestPointOnSpline(t *testing.T) {
	s := sketch.New()
	sp := archSpline(s)
	p := s.AddPoint(4, 1) // below the arch's interior; pulled up onto it
	s.AddConstraint(sketch.NewPointOnSpline(p, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToSpline(p, sp), 1e-4, "point pulled onto the spline")
	require.Greater(t, p.Y(), 1.0, "moved up onto the interior of the arch")
}

func TestPointOnSplineConfinedToRange(t *testing.T) {
	// A point started well beyond the spline's end must attach at the endpoint
	// (the last control point a clamped spline passes through), not extrapolate
	// past it — the slack box keeps the foot parameter within [0, 1].
	s := sketch.New()
	sp := archSpline(s)
	p := s.AddPoint(20, -5) // far past the (8,0) end
	s.AddConstraint(sketch.NewPointOnSpline(p, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToSpline(p, sp), 1e-4, "still on the curve")
	require.LessOrEqual(t, p.X(), 8.01, "not extrapolated past the clamped end")
	require.InDelta(t, 8, p.X(), 0.1, "attached at the end, near the last control point")
	require.InDelta(t, 0, p.Y(), 0.1)
}

func TestPointOnSplineDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	sp := archSpline(s) // control points fixed
	p := s.AddPoint(4, 1)
	require.Equal(t, 2, s.DOF(), "the free point has two DOF")

	con := sketch.NewPointOnSpline(p, sp)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "on a 1-D spline the point keeps one sliding DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestPointOnSplineCheckConstraint(t *testing.T) {
	s := sketch.New()
	sp := archSpline(s)
	p := s.AddPoint(4, 1)
	// Adding point-on-spline to a free point removes one DOF — it is not
	// over-constraining, so the pre-commit probe must accept it.
	require.NoError(t, s.CheckConstraint(sketch.NewPointOnSpline(p, sp)))
}

func TestPointOnSplineDuplicateIsHarmless(t *testing.T) {
	// Documented limitation: two point-on-spline on the same point are redundant,
	// but the redundancy is *nonlinear* — each owns an independent foot parameter
	// and S(t1)=S(t2) only forces t1=t2 at the solved point, which rank-based
	// analysis (CheckConstraint, local to the call-time config) cannot see. So the
	// duplicate is not flagged; it is harmless, though — the sketch stays solvable
	// and keeps the one sliding DOF (both witnesses converge to the same foot).
	s := sketch.New()
	sp := archSpline(s)
	p := s.AddPoint(4, 1)
	s.AddConstraint(sketch.NewPointOnSpline(p, sp))
	s.AddConstraint(sketch.NewPointOnSpline(p, sp))

	_, err := s.Solve()
	require.NoError(t, err, "the redundant duplicate does not break solvability")
	require.InDelta(t, 0, distToSpline(p, sp), 1e-4, "point still on the curve")
	require.Equal(t, 1, s.DOF(), "still one sliding DOF, not mis-counted to zero")
}

func TestPointOnSplineRoundTrip(t *testing.T) {
	s := sketch.New()
	sp := archSpline(s)
	p := s.AddPoint(4, 1)
	s.AddConstraint(sketch.NewPointOnSpline(p, sp))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve()
	require.NoError(t, err)
}
