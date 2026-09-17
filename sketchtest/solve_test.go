package sketchtest_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

func TestSolveReturnsResult(t *testing.T) {
	s, _, _ := constrainedRectangle(t)
	result := sketchtest.Solve(t, s)
	require.True(t, result.Converged)
	require.Zero(t, result.DOF)
}

func TestSolveRejectsNilSketch(t *testing.T) {
	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.Solve(tb, nil)
	})
	require.Equal(t, "sketchtest.Solve: s must not be nil", message)
}

func TestSolveReportsConvergenceFailure(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewDistance(a, b, 20))

	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.Solve(tb, s)
	})
	require.Contains(t, message, sketch.ErrNotConverged.Error())
}
