package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

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
