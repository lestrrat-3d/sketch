package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

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
