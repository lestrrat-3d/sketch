package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// countKind returns how many of the sketch's constraints have the given kind.
func countKind(s *sketch.Sketch, kind string) int {
	n := 0
	for _, c := range s.Constraints() {
		if sketch.ConstraintKind(c) == kind {
			n++
		}
	}
	return n
}

// Trimming an end keeps the line's handle and the constraints that name it, as
// Fusion's trim modifies the original curve; the trimmed-away end point goes.
func TestTrimEndKeepsHandle(t *testing.T) {
	s := newSketch(t)
	oldEnd := s.CreatePoint(5, 0)
	h := s.CreateLine(s.CreatePoint(-5, 0), oldEnd)
	s.CreateLine(s.CreatePoint(2, -5), s.CreatePoint(2, 5)) // vertical cutter at x=2
	s.AddConstraint(sketch.NewHorizontal(h))
	dof := s.DOF()

	res, ok := sketch.Trim(s, h, 4, 0)
	require.True(t, ok)
	require.Equal(t, []*sketch.Line{h}, res, "the same handle comes back")
	require.InDelta(t, 2, h.End.X(), 1e-9)
	require.InDelta(t, 0, h.End.Y(), 1e-9)
	require.NotContains(t, s.Points(), oldEnd, "the trimmed-away end point is removed")
	require.Equal(t, 1, countKind(s, "horizontal"), "the constraint on the line survives")
	require.Equal(t, dof, s.DOF(), "a fresh end replaces the removed one")
}

// Extending keeps the handle too, as Fusion's extend returns the modified curve.
func TestExtendKeepsHandle(t *testing.T) {
	s := newSketch(t)
	l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(3, 0))
	s.CreateLine(s.CreatePoint(6, -5), s.CreatePoint(6, 5))
	s.AddConstraint(sketch.NewHorizontal(l))

	got, ok := sketch.Extend(s, l, l.End)
	require.True(t, ok)
	require.Same(t, l, got)
	require.InDelta(t, 6, l.End.X(), 1e-9)
	require.Len(t, s.Points(), 4, "the old end point is removed")
	require.Equal(t, 1, countKind(s, "horizontal"))
}

// A tangency whose shared contact point is trimmed off the line becomes a
// tangency anywhere on the two curves: the handle stays and still holds.
func TestTrimRefreshesSharedTangency(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(5, 0)
	arc := s.CreateArc(s.CreatePoint(0, 0), p, s.CreatePoint(0, 5))
	l := s.CreateLine(p, s.CreatePoint(5, -10))
	tan := sketch.NewTangent(l, arc)
	s.AddConstraint(tan)
	require.Len(t, sketch.ConstraintResiduals(tan), 1, "endpoint tangency: one row")
	s.CreateLine(s.CreatePoint(0, -2), s.CreatePoint(10, -2)) // cutter at y=-2

	res, ok := sketch.Trim(s, l, 5, -1)
	require.True(t, ok)
	require.Equal(t, []*sketch.Line{l}, res)
	require.InDelta(t, -2, l.Start.Y(), 1e-9)
	require.Same(t, p, arc.Start, "the arc keeps the old contact point")
	require.Contains(t, s.Constraints(), tan)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	rows := sketch.ConstraintResiduals(tan)
	require.Len(t, rows, 2, "tangency anywhere: circle row plus sweep row")
	for _, r := range rows {
		require.InDelta(t, 0, r, 1e-9)
	}
}
