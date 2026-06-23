package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// archConic builds a fixed conic arc (rho 0.5 → a parabola arc) bulging upward,
// with its defining points grounded — a rigid curve to attach points to.
func archConic(s *sketch.Sketch) *sketch.Conic {
	start := s.AddPoint(0, 0)
	apex := s.AddPoint(4, 6)
	end := s.AddPoint(8, 0)
	c, err := s.AddConic(start, apex, end, 0.5)
	if err != nil {
		panic(err)
	}
	for _, p := range []*sketch.Point{start, apex, end} {
		s.Fix(p)
	}
	return c
}

// distToConic returns the perpendicular distance from p to a densely sampled
// polyline of the conic (segment projection, accurate between samples).
func distToConic(p *sketch.Point, c *sketch.Conic) float64 {
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

func TestPointOnConic(t *testing.T) {
	s := newSketch(t)
	c := archConic(s)
	p := s.AddPoint(4, 1) // below the arch interior; pulled up onto it
	s.AddConstraint(sketch.NewPointOnConic(p, c))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToConic(p, c), 1e-4, "point pulled onto the conic")
	require.Greater(t, p.Y(), 1.0, "moved up onto the interior of the arch")
}

func TestPointOnConicConfinedToRange(t *testing.T) {
	// A point started well beyond the conic's end must attach at the endpoint, not
	// extrapolate past it — the slack box keeps the foot parameter within [0,1].
	s := newSketch(t)
	c := archConic(s)
	p := s.AddPoint(20, -5) // far past the (8,0) end
	s.AddConstraint(sketch.NewPointOnConic(p, c))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToConic(p, c), 1e-4, "still on the curve")
	require.LessOrEqual(t, p.X(), 8.01, "not extrapolated past the end")
	require.InDelta(t, 8, p.X(), 0.1, "attached at the end")
	require.InDelta(t, 0, p.Y(), 0.1)
}

func TestPointOnConicDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	c := archConic(s) // defining points fixed; the conic's rho is still a free var
	p := s.AddPoint(4, 1)
	// Two DOF from the free point plus one from the conic's free rho parameter.
	require.Equal(t, 3, s.DOF(), "the free point (2) plus the conic's free rho (1)")

	con := sketch.NewPointOnConic(p, c)
	s.AddConstraint(con)
	// Confining the point to the curve removes exactly one of the point's two DOF —
	// it keeps one sliding DOF along the curve; rho stays free.
	require.Equal(t, 2, s.DOF(), "on a 1-D conic the point keeps one sliding DOF (rho still free)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 3, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestPointOnConicCheckConstraint(t *testing.T) {
	s := newSketch(t)
	c := archConic(s)
	p := s.AddPoint(4, 1)
	require.NoError(t, s.CheckConstraint(sketch.NewPointOnConic(p, c)))
}

func TestPointOnConicRoundTrip(t *testing.T) {
	s := newSketch(t)
	c := archConic(s)
	p := s.AddPoint(4, 1)
	s.AddConstraint(sketch.NewPointOnConic(p, c))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload (no doubling)")
	_, err = s2.Solve()
	require.NoError(t, err)
	require.Equal(t, 2, s2.DOF(), "reloaded sketch keeps the sliding DOF + free rho (aux vars re-seeded)")
}

func TestPointOnConicEntityRemovalCascades(t *testing.T) {
	s := newSketch(t)
	c := archConic(s)
	p := s.AddPoint(4, 1)
	con := sketch.NewPointOnConic(p, c)
	s.AddConstraint(con)
	require.Len(t, s.Constraints(), 1)

	require.True(t, s.RemoveEntity(c), "the conic is removed")
	require.Empty(t, s.Constraints(), "removing the conic cascades the point-on-conic constraint")
}

// conicTangentDirAtLineContact returns the unit tangent of the conic at the
// contact parameter — the curve parameter whose point has minimum perpendicular
// distance to the infinite line through p1,p2 (i.e. the tangency point). The
// tangent is a tight central difference on the analytic Eval there, used to check
// the line is parallel to the contact tangent.
func conicTangentDirAtLineContact(c *sketch.Conic, p1, p2 *sketch.Point) (float64, float64) {
	ax, ay := p1.X(), p1.Y()
	dx, dy := p2.X()-ax, p2.Y()-ay
	dlen := math.Hypot(dx, dy)
	bestT, best := 0.0, math.Inf(1)
	const n = 4000
	for i := 0; i <= n; i++ {
		tt := float64(i) / n
		qx, qy := c.Eval(tt)
		if gap := math.Abs(dx*(qy-ay)-dy*(qx-ax)) / dlen; gap < best {
			best, bestT = gap, tt
		}
	}
	const h = 1e-6
	t0, t1 := math.Max(0, bestT-h), math.Min(1, bestT+h)
	x0, y0 := c.Eval(t0)
	x1, y1 := c.Eval(t1)
	tx, ty := x1-x0, y1-y0
	m := math.Hypot(tx, ty)
	return tx / m, ty / m
}

func TestTangentToConic(t *testing.T) {
	// A horizontal line above the arch becomes tangent at the peak (the point with a
	// horizontal tangent), so it settles at the conic's max height.
	s := newSketch(t)
	c := archConic(s)
	p1 := s.AddPoint(-2, 2.5)
	p2 := s.AddPoint(10, 2.5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToConic(line, c))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, p1.Y(), p2.Y(), 1e-9, "line stays horizontal")

	best := math.Inf(-1)
	for _, q := range c.Polyline(400) {
		best = math.Max(best, q[1])
	}
	require.InDelta(t, best, p1.Y(), 1e-3, "tangent at the peak (horizontal tangent)")
	require.InDelta(t, 0, lineGapToConic(p1, p2, c), 1e-3, "line touches the curve")
	// The line is parallel to the conic's analytic tangent at the contact point.
	tx, ty := conicTangentDirAtLineContact(c, p1, p2)
	require.InDelta(t, 0, ty, 1e-2, "contact tangent is horizontal (parallel to the line)")
	require.InDelta(t, 1, math.Abs(tx), 1e-2)
}

// lineGapToConic returns the minimum perpendicular distance from the conic to the
// infinite line through p1,p2 — zero when the line touches the curve.
func lineGapToConic(p1, p2 *sketch.Point, c *sketch.Conic) float64 {
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

func TestTangentToConicDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	c := archConic(s) // defining points fixed
	p1 := s.AddPoint(-2, 2.5)
	p2 := s.AddPoint(10, 2.5)
	// Four DOF from the free line plus one from the conic's free rho parameter.
	require.Equal(t, 5, s.DOF(), "a free line (4) plus the conic's free rho (1)")

	con := sketch.NewTangentToConic(s.AddLine(p1, p2), c)
	s.AddConstraint(con)
	require.Equal(t, 4, s.DOF(), "tangency removes one DOF (the slacks net out; rho still free)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 5, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestTangentToConicCheckConstraint(t *testing.T) {
	s := newSketch(t)
	c := archConic(s)
	p1 := s.AddPoint(-2, 2.5)
	p2 := s.AddPoint(10, 2.5)
	require.NoError(t, s.CheckConstraint(sketch.NewTangentToConic(s.AddLine(p1, p2), c)))
}

func TestTangentToConicRoundTrip(t *testing.T) {
	s := newSketch(t)
	c := archConic(s)
	p1 := s.AddPoint(-2, 2.5)
	p2 := s.AddPoint(10, 2.5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToConic(line, c))
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
