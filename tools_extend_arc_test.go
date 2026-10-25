package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

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
