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
	require.Equal(t, 1, countKind(s, "point_on_line"), "the new end is tied to the cutter")
	require.Equal(t, dof-1, s.DOF(), "a fresh end replaces the removed one, less the pin")
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

// A fillet shortens its legs in place, as Fusion's fillet does: the legs keep
// their handles and the constraints that name them, so the sketch stays fully
// constrained without re-pinning anything.
func TestFilletKeepsLegs(t *testing.T) {
	s := newSketch(t)
	a, corner, b := s.CreatePoint(0, 10), s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	l1, l2 := s.CreateLine(a, corner), s.CreateLine(corner, b)
	s.AddConstraint(sketch.NewCoincident(corner, s.Origin()), sketch.NewVertical(l1), sketch.NewHorizontal(l2),
		sketch.NewDistance(a, corner, 10), sketch.NewDistance(corner, b, 10))
	require.Equal(t, 0, s.DOF())

	f, err := sketch.CreateFillet(s, l1, l2, corner, 3)
	require.NoError(t, err)
	require.Same(t, l1, f.L1)
	require.Same(t, l2, f.L2)
	require.Same(t, a, l1.Start, "the far end is kept")
	require.Same(t, f.T1, l1.End, "the corner end moved to the contact")
	require.NotContains(t, s.Points(), corner, "the corner point is removed")
	require.Equal(t, 1, countKind(s, "vertical"))
	require.Equal(t, 1, countKind(s, "horizontal"))
	require.Equal(t, 0, countKind(s, "coincident"), "the constraint on the corner goes with it")

	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 0, f.T1.X(), 1e-6, "still on the vertical leg")
	require.InDelta(t, 0, f.T2.Y(), 1e-6, "still on the horizontal leg")
	require.InDelta(t, 3, f.Arc.R(), 1e-6)
}

// A chamfer keeps its legs the same way.
func TestChamferKeepsLegs(t *testing.T) {
	s := newSketch(t)
	a, corner, b := s.CreatePoint(0, 10), s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	l1, l2 := s.CreateLine(a, corner), s.CreateLine(corner, b)
	s.AddConstraint(sketch.NewVertical(l1), sketch.NewHorizontal(l2))

	c, err := sketch.CreateChamfer(s, l1, l2, 3)
	require.NoError(t, err)
	require.Same(t, l1, c.L1)
	require.Same(t, l2, c.L2)
	require.Same(t, c.T1, l1.End)
	require.Same(t, c.T2, l2.Start)
	require.Equal(t, 1, countKind(s, "vertical"))
	require.Equal(t, 1, countKind(s, "horizontal"))
	require.Len(t, s.Entities(), 3, "two legs and the cut")
}
