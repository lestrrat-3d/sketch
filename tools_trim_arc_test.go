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
