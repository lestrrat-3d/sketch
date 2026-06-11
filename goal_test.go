package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestGoalProjection(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	line := s.AddLine(a, b)
	p := s.AddPoint(3, 0)
	s.AddConstraint(sketch.NewPointOnLine(p, line))

	res, err := s.Solve(sketch.WithGoal(p, 4, 5))
	require.NoError(t, err, "goal solve")
	require.True(t, res.Converged, "hard constraints hold")
	require.InDelta(t, 4, p.X(), 1e-5, "lands at the perpendicular projection")
	require.InDelta(t, 0, p.Y(), 1e-8, "stays on the line")
}

func TestGoalConstraintsWin(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	_, err := s.Solve()
	require.NoError(t, err)

	res, err := s.Solve(sketch.WithGoal(c, 30, 30)) // unreachable: w=20, h=12
	require.NoError(t, err, "unreachable goal is not an error")
	require.True(t, res.Converged, "hard constraints still hold")
	require.InDelta(t, 20, c.X(), 1e-6, "corner pinned by width")
	require.InDelta(t, 12, c.Y(), 1e-6, "corner pinned by height")
}

func TestGoalTracking(t *testing.T) {
	// A lone free point and no constraints: exercises the goal-only path
	// (no hard residuals) and warm-started tracking across a target path.
	s := sketch.New()
	p := s.AddPoint(0, 0)
	for _, tgt := range [][2]float64{{2, 1}, {5, 4}, {5, 9}, {-3, 2}} {
		res, err := s.Solve(sketch.WithGoal(p, tgt[0], tgt[1]))
		require.NoError(t, err, "tracking solve")
		require.True(t, res.Converged, "no hard constraints to violate")
		require.InDelta(t, tgt[0], p.X(), 1e-5, "tracks target x")
		require.InDelta(t, tgt[1], p.Y(), 1e-5, "tracks target y")
	}
}

func TestGoalMultiple(t *testing.T) {
	// Two goals translate a dimensioned line; length stays constrained.
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.AddLine(a, b)
	s.AddConstraint(sketch.NewDistance(a, b, 10))

	res, err := s.Solve(sketch.WithGoal(a, 5, 5), sketch.WithGoal(b, 15, 5))
	require.NoError(t, err, "two-goal solve")
	require.True(t, res.Converged, "length constraint holds")
	require.InDelta(t, 5, a.X(), 1e-5, "a.X")
	require.InDelta(t, 5, a.Y(), 1e-5, "a.Y")
	require.InDelta(t, 15, b.X(), 1e-5, "b.X")
	require.InDelta(t, 5, b.Y(), 1e-5, "b.Y")
	require.InDelta(t, 10, a.DistanceTo(b), 1e-6, "length preserved")
}

func TestGoalFixedPointInert(t *testing.T) {
	s := sketch.New()
	p := s.AddPoint(2, 3)
	s.Fix(p)

	res, err := s.Solve(sketch.WithGoal(p, 50, 50))
	require.NoError(t, err, "goal on fixed point is legal")
	require.True(t, res.Converged, "nothing to violate")
	require.InDelta(t, 2, p.X(), 1e-12, "grounded point does not move")
	require.InDelta(t, 3, p.Y(), 1e-12, "grounded point does not move")
}

func TestGoalLeavesNoResidue(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	plain, err := s.Solve()
	require.NoError(t, err)

	res, err := s.Solve(sketch.WithGoal(c, 30, 30))
	require.NoError(t, err, "goal solve")
	require.Equal(t, plain.DOF, res.DOF, "DOF unaffected by goal")
	require.Equal(t, plain.Redundant, res.Redundant, "redundancy unaffected by goal")
	require.Empty(t, s.RedundantConstraints(), "goal is not a constraint")

	// A subsequent plain solve does not move geometry.
	x, y := c.X(), c.Y()
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, x, c.X(), 1e-9, "plain re-solve stable")
	require.InDelta(t, y, c.Y(), 1e-9, "plain re-solve stable")

	// Nothing about the goal serializes.
	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	require.NotContains(t, string(data), "goal", "no goal artifacts in JSON")
}
