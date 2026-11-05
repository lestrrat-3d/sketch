package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// quarterArc (defined in pointonarc_test.go) builds a fixed first-quadrant arc:
// center at the origin, radius 5, sweeping CCW from (5,0) to (0,5).

func radius(p *sketch.Point, c *sketch.Point) float64 {
	return math.Hypot(p.X()-c.X(), p.Y()-c.Y())
}

func TestDistancePointArcInSweep(t *testing.T) {
	// A free point seeded inside the sweep at radius ~6; the gap dimension pulls it
	// to radius R+2 = 7 along an in-sweep direction.
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(4.2, 4.2) // ~45°, radius ~5.9
	s.AddConstraint(sketch.NewDistancePointArc(p, arc, 2))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, s.Verify(t.Context()).Solvable)
	require.InDelta(t, 7, radius(p, arc.Center), 1e-6, "radial gap = 2 → |P−C| = R+2")
	// the radial foot stays inside the [0°,90°] sweep
	ang := math.Atan2(p.Y()-arc.Center.Y(), p.X()-arc.Center.X())
	require.GreaterOrEqual(t, ang, -1e-9)
	require.LessOrEqual(t, ang, math.Pi/2+1e-9)
}

func TestDistancePointArcInsideTarget(t *testing.T) {
	// A negative target places the point inside the carrier circle by |d|.
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(2.1, 2.1) // ~45°, radius ~3, inside
	s.AddConstraint(sketch.NewDistancePointArc(p, arc, -2))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 3, radius(p, arc.Center), 1e-6, "radial gap = −2 → |P−C| = R−2")
}

func TestDistancePointArcOutOfSweepRejected(t *testing.T) {
	// The point sits exactly on the carrier circle (gap row satisfied) but at 180°,
	// outside the [0°,90°] sweep. The sweep row cannot be satisfied, so the oracle
	// must report it unsolvable rather than measuring to the nearer endpoint.
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(-5, 0) // on the radius-5 circle, angle 180°
	s.Fix(p)
	s.AddConstraint(sketch.NewDistancePointArc(p, arc, 0))
	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify(t.Context()).Solvable, "carrier-on but off-sweep is not blessed")
}

func TestDistancePointArcAtCenterRejected(t *testing.T) {
	// A point pinned at the arc center has no radial direction. For a reflex (≥ π)
	// sweep the floored zero direction would otherwise read as in-sweep, so the
	// degenerate config must be explicitly reported unsolvable — even with the
	// target −R that makes the radial-gap row vanish.
	s := newSketch(t)
	o := s.CreatePoint(0, 0)
	start := s.CreatePoint(5, 0)
	end := s.CreatePoint(0, -5) // CCW from (5,0) to (0,−5) is a 270° reflex sweep
	s.Fix(o)
	s.Fix(start)
	s.Fix(end)
	arc := s.CreateArc(o, start, end)
	p := s.CreatePoint(0, 0) // exactly at the center
	s.Fix(p)
	s.AddConstraint(sketch.NewDistancePointArc(p, arc, -arc.R()))
	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify(t.Context()).Solvable, "a point at the arc center is degenerate, not blessed")
}

func TestDistanceLineArcTangentInSweep(t *testing.T) {
	// A near-vertical free line seeded close to x=5 is pulled to tangency with the
	// arc at its (5,0) end — the foot direction +x is in-sweep (inclusive).
	s := newSketch(t)
	arc := quarterArc(s)
	l := s.CreateLine(s.CreatePoint(4.7, -2), s.CreatePoint(4.7, 9))
	s.AddConstraint(sketch.NewDistanceLineArc(l, arc, 0))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, s.Verify(t.Context()).Solvable)
	// perpendicular distance from the center to the solved line equals R
	ax, ay := l.Start.X(), l.Start.Y()
	abx, aby := l.End.X()-ax, l.End.Y()-ay
	h := math.Abs(abx*(arc.Center.Y()-ay)-aby*(arc.Center.X()-ax)) / math.Hypot(abx, aby)
	require.InDelta(t, arc.R(), h, 1e-6, "tangent: dist(center,line) = R")
}

func TestDistanceLineArcOutOfSweepRejected(t *testing.T) {
	// A horizontal line at y=−5 is tangent to the carrier circle at (0,−5), angle
	// 270°, outside the [0°,90°] sweep. The gap row is satisfied but the sweep row
	// is not, so it is unsolvable.
	s := newSketch(t)
	arc := quarterArc(s)
	l := s.CreateLine(s.CreatePoint(-3, -5), s.CreatePoint(8, -5))
	s.Fix(l.Start)
	s.Fix(l.End)
	s.AddConstraint(sketch.NewDistanceLineArc(l, arc, 0))
	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify(t.Context()).Solvable)
}

func TestDistancePointArcDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(4.2, 4.2)
	require.Equal(t, 2, s.DOF(), "free point against a fixed arc")
	con := sketch.NewDistancePointArc(p, arc, 2)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "gap row pins the radius; the sweep slack nets zero")
	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal retires the sweep slack cleanly")
}

func TestDistancePointArcDriven(t *testing.T) {
	// A driven (reference) dimension measures the signed radial gap and owns no
	// sweep slack, so toggling driven does not leak a free DOF.
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(7, 7)
	s.Fix(p)
	con := sketch.NewDistancePointArc(p, arc, 0)
	s.AddConstraint(con)
	con.SetDriven(true)
	require.Equal(t, 0, s.DOF(), "all geometry fixed; driven dim contributes no var")
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	want := math.Hypot(7, 7) - arc.R() // |P−C| − R
	require.InDelta(t, want, con.Target().Base(), 1e-9, "driven dim measures the radial gap")
}

func TestDistancePointArcDrivenToggleDOF(t *testing.T) {
	// Toggling driving↔driven moves the sweep slack in and out, so the DOF tracks
	// it exactly (no orphaned variable left behind).
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(4.2, 4.2)
	con := sketch.NewDistancePointArc(p, arc, 2)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF())
	con.SetDriven(true)
	require.Equal(t, 2, s.DOF(), "driven: slack retired, point fully free again")
	con.SetDriven(false)
	require.Equal(t, 1, s.DOF(), "driving again: slack re-allocated")
}

func TestDistanceArcCheckConstraintNonMutating(t *testing.T) {
	// CheckConstraint probes an aux-var dimension in committed form by temporarily
	// allocating its slack, then rolls back — leaving the DOF untouched.
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(4.2, 4.2)
	before := s.DOF()
	err := s.CheckConstraint(sketch.NewDistancePointArc(p, arc, 2))
	require.NoError(t, err)
	require.Equal(t, before, s.DOF(), "CheckConstraint must not mutate the sketch")
}

func TestDistanceArcRoundTripDriven(t *testing.T) {
	s := newSketch(t)
	arc := quarterArc(s)
	p := s.CreatePoint(7, 7)
	s.Fix(p)
	con := sketch.NewDistancePointArc(p, arc, 0)
	s.AddConstraint(con)
	con.SetDriven(true)
	l := s.CreateLine(s.CreatePoint(5, -2), s.CreatePoint(5, 9)) // tangent at the (5,0) end
	s.Fix(l.Start)
	s.Fix(l.End)
	s.AddConstraint(sketch.NewDistanceLineArc(l, arc, 0))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()))
	_, err = s2.Solve(t.Context())
	require.NoError(t, err)
	// the driven flag survives, and the reloaded dim still measures the gap
	var drivenSeen bool
	for _, c := range s2.Constraints() {
		if d, ok := c.(*sketch.DistancePointArc); ok && d.Driven() {
			drivenSeen = true
			require.InDelta(t, math.Hypot(7, 7)-arc.R(), d.Target().Base(), 1e-9)
		}
	}
	require.True(t, drivenSeen, "a driven distance_point_arc round-trips")
}

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

			_, err := s.Solve(t.Context())
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

			_, err := s.Solve(t.Context())
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

	_, err := s.Solve(t.Context())
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
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive reload")
	_, err = s2.Solve(t.Context())
	require.NoError(t, err)
	for i, p := range s.Points() {
		require.InDeltaf(t, p.X(), s2.Points()[i].X(), 1e-6, "point %d X", i)
		require.InDeltaf(t, p.Y(), s2.Points()[i].Y(), 1e-6, "point %d Y", i)
	}
}
