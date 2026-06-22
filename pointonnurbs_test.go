package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// archNURBS builds a fixed degree-2 NURBS arch (3 control points, clamped uniform
// knots → domain [0,1]) bulging upward, with its control points grounded.
func archNURBS(s *sketch.Sketch) *sketch.NURBS {
	c0 := s.AddPoint(0, 0)
	c1 := s.AddPoint(4, 8)
	c2 := s.AddPoint(8, 0)
	c, err := s.AddNURBS(2, []*sketch.Point{c0, c1, c2}, nil, sketch.ClampedUniformKnots(3, 2))
	if err != nil {
		panic(err)
	}
	for _, p := range []*sketch.Point{c0, c1, c2} {
		s.Fix(p)
	}
	return c
}

// distToNURBS returns the perpendicular distance from p to a densely sampled
// polyline of the NURBS curve (segment projection, accurate between samples).
func distToNURBS(p *sketch.Point, c *sketch.NURBS) float64 {
	poly := c.Polyline(400)
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

func TestPointOnNURBS(t *testing.T) {
	s := sketch.New()
	c := archNURBS(s)
	p := s.AddPoint(4, 1) // below the arch interior; pulled up onto it
	s.AddConstraint(sketch.NewPointOnNURBS(p, c))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToNURBS(p, c), 1e-4, "point pulled onto the NURBS")
	require.Greater(t, p.Y(), 1.0, "moved up onto the interior of the arch")
}

func TestPointOnNURBSConfinedToRange(t *testing.T) {
	// A point started beyond the curve's end must attach at the endpoint, not
	// extrapolate past it — the slack box keeps the foot parameter within [0,1].
	s := sketch.New()
	c := archNURBS(s)
	p := s.AddPoint(12, -2) // past the (8,0) end
	s.AddConstraint(sketch.NewPointOnNURBS(p, c))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToNURBS(p, c), 1e-4, "still on the curve")
	require.LessOrEqual(t, p.X(), 8.01, "not extrapolated past the clamped end")
	require.InDelta(t, 8, p.X(), 0.2, "attached near the end (last control point)")
	require.InDelta(t, 0, p.Y(), 0.2)
}

func TestPointOnNURBSDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	c := archNURBS(s) // control points fixed
	p := s.AddPoint(4, 1)
	require.Equal(t, 2, s.DOF(), "the free point has two DOF")

	con := sketch.NewPointOnNURBS(p, c)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "on a 1-D NURBS the point keeps one sliding DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestPointOnNURBSCheckConstraint(t *testing.T) {
	s := sketch.New()
	c := archNURBS(s)
	p := s.AddPoint(4, 1)
	require.NoError(t, s.CheckConstraint(sketch.NewPointOnNURBS(p, c)))
}

func TestPointOnNURBSRoundTrip(t *testing.T) {
	s := sketch.New()
	c := archNURBS(s)
	p := s.AddPoint(4, 1)
	s.AddConstraint(sketch.NewPointOnNURBS(p, c))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload (no doubling)")
	_, err = s2.Solve()
	require.NoError(t, err)
	require.Equal(t, 1, s2.DOF(), "reloaded sketch keeps one sliding DOF (aux vars re-seeded)")
}

func TestPointOnNURBSEntityRemovalCascades(t *testing.T) {
	s := sketch.New()
	c := archNURBS(s)
	p := s.AddPoint(4, 1)
	con := sketch.NewPointOnNURBS(p, c)
	s.AddConstraint(con)
	require.Len(t, s.Constraints(), 1)

	require.True(t, s.RemoveEntity(c), "the NURBS is removed")
	require.Empty(t, s.Constraints(), "removing the NURBS cascades the point-on-NURBS constraint")
}

func TestTangentToNURBS(t *testing.T) {
	// A horizontal line above the arch becomes tangent at the peak (the point with a
	// horizontal tangent), so it settles at the curve's max height.
	s := sketch.New()
	c := archNURBS(s)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToNURBS(line, c))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, p1.Y(), p2.Y(), 1e-9, "line stays horizontal")

	best := math.Inf(-1)
	for _, q := range c.Polyline(400) {
		best = math.Max(best, q[1])
	}
	require.InDelta(t, best, p1.Y(), 1e-3, "tangent at the peak (horizontal tangent)")
	require.InDelta(t, 0, lineGapToNURBS(p1, p2, c), 1e-3, "line touches the curve")
}

// lineGapToNURBS returns the minimum perpendicular distance from the curve to the
// infinite line through p1,p2 — zero when the line touches the curve.
func lineGapToNURBS(p1, p2 *sketch.Point, c *sketch.NURBS) float64 {
	ax, ay := p1.X(), p1.Y()
	dx, dy := p2.X()-ax, p2.Y()-ay
	dlen := math.Hypot(dx, dy)
	best := math.Inf(1)
	for _, q := range c.Polyline(400) {
		d := math.Abs(dx*(q[1]-ay)-dy*(q[0]-ax)) / dlen
		if d < best {
			best = d
		}
	}
	return best
}

func TestTangentToNURBSDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	c := archNURBS(s) // control points fixed
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	require.Equal(t, 4, s.DOF(), "a free line has four DOF")

	con := sketch.NewTangentToNURBS(s.AddLine(p1, p2), c)
	s.AddConstraint(con)
	require.Equal(t, 3, s.DOF(), "tangency removes one DOF (the slacks net out)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 4, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestTangentToNURBSCheckConstraint(t *testing.T) {
	s := sketch.New()
	c := archNURBS(s)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	require.NoError(t, s.CheckConstraint(sketch.NewTangentToNURBS(s.AddLine(p1, p2), c)))
}

func TestTangentToNURBSRoundTrip(t *testing.T) {
	s := sketch.New()
	c := archNURBS(s)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToNURBS(line, c))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive reload (no doubling)")
	_, err = s2.Solve()
	require.NoError(t, err)
}
