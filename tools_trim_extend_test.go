package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Trimming the start stub of an arc moves its Start in place to the crossing,
// keeps the center and End, and ties the new Start to the cutter.
func TestTrimArcEndStub(t *testing.T) {
	s := newSketch(t)
	center, end := s.CreatePoint(0, 0), s.CreatePoint(0, 5)
	arc := s.CreateArc(center, s.CreatePoint(5, 0), end)
	s.CreateLine(s.CreatePoint(3, -1), s.CreatePoint(3, 10)) // crosses the arc at (3, 4)

	res, ok := sketch.Trim(s, arc, 4.9, 1)
	require.True(t, ok)
	require.Equal(t, []*sketch.Arc{arc}, res)
	require.Same(t, center, arc.Center)
	require.Same(t, end, arc.End)
	require.InDelta(t, 3, arc.Start.X(), 1e-9)
	require.InDelta(t, 4, arc.Start.Y(), 1e-9)
	require.Equal(t, 1, countKind(s, "point_on_line"))
}

// Picking between two crossings deletes the arc and keeps the two outer pieces.
func TestTrimArcMiddle(t *testing.T) {
	s := newSketch(t)
	center, start, end := s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5)
	arc := s.CreateArc(center, start, end)
	s.CreateLine(s.CreatePoint(0, 2.5), s.CreatePoint(10, 2.5)) // crosses at 30°
	s.CreateLine(s.CreatePoint(2.5, 0), s.CreatePoint(2.5, 10)) // crosses at 60°

	res, ok := sketch.Trim(s, arc, 3.5, 3.5) // pick at 45°
	require.True(t, ok)
	require.Len(t, res, 2)
	require.Same(t, start, res[0].Start)
	require.InDelta(t, 5*math.Cos(math.Pi/6), res[0].End.X(), 1e-9)
	require.InDelta(t, 2.5, res[0].End.Y(), 1e-9)
	require.InDelta(t, 2.5, res[1].Start.X(), 1e-9)
	require.Same(t, end, res[1].End)
	require.Same(t, center, res[0].Center)
	require.Same(t, center, res[1].Center)
	require.NotContains(t, s.Entities(), sketch.Entity(arc))
}

// An arc that crosses nothing is deleted, with its points.
func TestTrimArcNoCrossingDeletes(t *testing.T) {
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))

	res, ok := sketch.Trim(s, arc, 4, 3)
	require.True(t, ok)
	require.Empty(t, res)
	require.Empty(t, s.Entities())
	require.Empty(t, s.Points())
}

// Trimming a circle deletes it and returns the remaining arc; its radius
// dimension moves to the arc and keeps driving it.
func TestTrimCircle(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	c := s.CreateCircle(center, 5)
	radius := sketch.NewRadius(c, 5)
	s.AddConstraint(radius)
	s.CreateLine(s.CreatePoint(3, -10), s.CreatePoint(3, 10)) // crosses at (3, ±4)
	dof := s.DOF()

	res, ok := sketch.TrimCircle(s, c, 5, 0)
	require.True(t, ok)
	require.Len(t, res, 1)
	arc := res[0]
	require.Same(t, center, arc.Center)
	require.InDelta(t, 3, arc.Start.X(), 1e-9)
	require.InDelta(t, 4, arc.Start.Y(), 1e-9)
	require.InDelta(t, 3, arc.End.X(), 1e-9)
	require.InDelta(t, -4, arc.End.Y(), 1e-9)
	require.InDelta(t, 2*math.Pi-2*math.Atan2(4, 3), arc.Sweep(), 1e-9)
	require.Equal(t, 2, countKind(s, "point_on_line"))
	_, ents := sketch.ConstraintRefs(radius)
	require.Equal(t, []sketch.Entity{arc}, ents, "the radius dimension now names the arc")
	require.Equal(t, dof, s.DOF())

	radius.Set(6)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 6, arc.R(), 1e-6)
}

// A circle with fewer than two crossings is deleted; a pick on a crossing picks
// nothing.
func TestTrimCircleEdgeCases(t *testing.T) {
	t.Run("tangent cutter deletes", func(t *testing.T) {
		s := newSketch(t)
		c := s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.CreateLine(s.CreatePoint(5, -10), s.CreatePoint(5, 10)) // touches at (5, 0)

		res, ok := sketch.TrimCircle(s, c, -5, 0)
		require.True(t, ok)
		require.Empty(t, res)
		require.Len(t, s.Entities(), 1, "only the cutter is left")
		require.Len(t, s.Points(), 2, "the circle's center is removed")
	})

	t.Run("pick on a crossing", func(t *testing.T) {
		s := newSketch(t)
		c := s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.CreateLine(s.CreatePoint(3, -10), s.CreatePoint(3, 10))

		res, ok := sketch.TrimCircle(s, c, 3, 4)
		require.False(t, ok)
		require.Nil(t, res)
		require.Len(t, s.Entities(), 2, "nothing changed")
	})
}

// Foreign and reference curves are refused with nothing changed.
func TestTrimArcAndCircleRefusals(t *testing.T) {
	other := newSketch(t)
	farc := other.CreateArc(other.CreatePoint(0, 0), other.CreatePoint(5, 0), other.CreatePoint(0, 5))
	fcircle := other.CreateCircle(other.CreatePoint(0, 0), 5)

	s := newSketch(t)
	s.CreateLine(s.CreatePoint(3, -10), s.CreatePoint(3, 10))
	res, ok := sketch.Trim(s, farc, 4.9, 1)
	require.False(t, ok)
	require.Nil(t, res)
	arcs, ok := sketch.TrimCircle(s, fcircle, 5, 0)
	require.False(t, ok)
	require.Nil(t, arcs)

	p1 := s.CreateReferencePoint(0, 0, "c")
	p2 := s.CreateReferencePoint(5, 0, "s")
	p3 := s.CreateReferencePoint(0, 5, "e")
	ref, err := s.CreateReferenceArc(p1, p2, p3, "edge")
	require.NoError(t, err)
	res, ok = sketch.Trim(s, ref, 4.9, 1)
	require.False(t, ok)
	require.Nil(t, res)
	require.Len(t, s.Entities(), 2)
}

// A trimmed end is tied to the cutter it was cut at, one constraint kind per
// cutter kind, so moving the cutter moves the end with it.
func TestTrimPinsEndToCutter(t *testing.T) {
	t.Run("line", func(t *testing.T) {
		s := newSketch(t)
		h := s.CreateLine(s.CreatePoint(-5, 0), s.CreatePoint(5, 0))
		cutter := s.CreateLine(s.CreatePoint(2, -5), s.CreatePoint(2, 5))
		dof := s.DOF()

		_, ok := sketch.Trim(s, h, 4, 0)
		require.True(t, ok)
		require.Equal(t, 1, countKind(s, "point_on_line"))
		require.Equal(t, dof-1, s.DOF())

		s.Fix(cutter.Start)
		s.Fix(cutter.End)
		cutter.Start.MoveTo(3, -5)
		cutter.End.MoveTo(3, 5)
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 3, h.End.X(), 1e-6, "the end follows the cutter")
	})

	t.Run("circle", func(t *testing.T) {
		s := newSketch(t)
		h := s.CreateLine(s.CreatePoint(-10, 0), s.CreatePoint(0, 0))
		s.CreateCircle(s.CreatePoint(0, 0), 5) // crosses h at (-5, 0)
		dof := s.DOF()

		_, ok := sketch.Trim(s, h, -2, 0)
		require.True(t, ok)
		require.InDelta(t, -5, h.End.X(), 1e-9)
		require.Equal(t, 1, countKind(s, "point_on_circle"))
		require.Equal(t, dof-1, s.DOF())
	})

	t.Run("arc", func(t *testing.T) {
		s := newSketch(t)
		h := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
		s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, -5), s.CreatePoint(5, 5)) // crosses h at (7.07, 0)
		dof := s.DOF()

		_, ok := sketch.Trim(s, h, 9, 0)
		require.True(t, ok)
		require.InDelta(t, 7.0710678118654755, h.End.X(), 1e-9)
		require.Equal(t, 1, countKind(s, "point_on_arc"))
		require.Equal(t, dof-1, s.DOF(), "two rows and one sweep slack variable: one degree of freedom less")
	})

	t.Run("cutter end point is shared", func(t *testing.T) {
		s := newSketch(t)
		h := s.CreateLine(s.CreatePoint(-5, 0), s.CreatePoint(5, 0))
		foot := s.CreatePoint(2, 0)
		s.CreateLine(foot, s.CreatePoint(2, 5)) // a T: the cutter ends on h
		dof := s.DOF()

		_, ok := sketch.Trim(s, h, 4, 0)
		require.True(t, ok)
		require.Same(t, foot, h.End, "the trim reuses the cutter's end point")
		require.Equal(t, 0, countKind(s, "point_on_line"))
		require.Equal(t, dof-2, s.DOF())
	})
}

// An extended end is tied to its cutter the same way.
func TestExtendPinsEndToCutter(t *testing.T) {
	s := newSketch(t)
	l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(3, 0))
	s.CreateLine(s.CreatePoint(6, -5), s.CreatePoint(6, 5))
	dof := s.DOF()

	_, ok := sketch.Extend(s, l, l.End)
	require.True(t, ok)
	require.Equal(t, 1, countKind(s, "point_on_line"))
	require.Equal(t, dof-1, s.DOF())
}

// A middle trim pins both new ends.
func TestTrimMiddlePinsBothEnds(t *testing.T) {
	s := newSketch(t)
	h := s.CreateLine(s.CreatePoint(-5, 0), s.CreatePoint(5, 0))
	s.CreateLine(s.CreatePoint(-2, -5), s.CreatePoint(-2, 5))
	s.CreateLine(s.CreatePoint(2, -5), s.CreatePoint(2, 5))
	dof := s.DOF()

	res, ok := sketch.Trim(s, h, 0, 0)
	require.True(t, ok)
	require.Len(t, res, 2)
	require.Equal(t, 2, countKind(s, "point_on_line"))
	require.Equal(t, dof+2, s.DOF(), "two fresh points (+4), two pins (-2)")
}

// Extending an arc's End moves it in place along the arc's circle to the
// nearest crossing beyond it, tied to the cutter.
func TestExtendArcToCutter(t *testing.T) {
	s := newSketch(t)
	center, start := s.CreatePoint(0, 0), s.CreatePoint(5, 0)
	arc := s.CreateArc(center, start, s.CreatePoint(0, 5))
	s.CreateLine(s.CreatePoint(-3, 0), s.CreatePoint(-3, 10)) // meets the circle at (-3, 4)

	got, ok := sketch.Extend(s, arc, arc.End)
	require.True(t, ok)
	require.Same(t, arc, got)
	require.Same(t, center, arc.Center)
	require.Same(t, start, arc.Start)
	require.InDelta(t, -3, arc.End.X(), 1e-9)
	require.InDelta(t, 4, arc.End.Y(), 1e-9)
	require.Equal(t, 1, countKind(s, "point_on_line"))
}

// Extending the Start runs clockwise.
func TestExtendArcStart(t *testing.T) {
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))
	s.CreateLine(s.CreatePoint(0, -3), s.CreatePoint(10, -3)) // meets the circle at (4, -3)

	_, ok := sketch.Extend(s, arc, arc.Start)
	require.True(t, ok)
	require.InDelta(t, 4, arc.Start.X(), 1e-9)
	require.InDelta(t, -3, arc.Start.Y(), 1e-9)
}

// A cutter reachable only by wrapping past the other end is refused: Fusion
// would close the arc into a circle, which Extend cannot return.
func TestExtendArcRefusesWrap(t *testing.T) {
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))
	s.CreateLine(s.CreatePoint(2, 2), s.CreatePoint(6, 6)) // crosses inside the sweep at 45°
	pts, cons := len(s.Points()), len(s.Constraints())

	got, ok := sketch.Extend(s, arc, arc.End)
	require.False(t, ok)
	require.Nil(t, got)
	require.Len(t, s.Points(), pts)
	require.Len(t, s.Constraints(), cons)
	require.InDelta(t, 5, arc.End.Y(), 1e-12, "the arc is unchanged")
}
