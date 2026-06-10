package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestSolveOptions(t *testing.T) {
	t.Run("max iterations", func(t *testing.T) {
		// The rectangle fixture starts from deliberately rough guesses; one
		// Levenberg–Marquardt iteration cannot reach the 1e-10 tolerance, so
		// the budget must surface as ErrNotConverged.
		s, _, _, _, _ := newRectangle(t)
		res, err := s.Solve(sketch.WithMaxIterations(1))
		require.ErrorIs(t, err, sketch.ErrNotConverged, "one iteration is not enough")
		require.False(t, res.Converged, "not converged")
		require.LessOrEqual(t, res.Iterations, 1, "iteration budget honored")

		// The same sketch solves with the default budget.
		mustSolve(t, s)
	})
	t.Run("tolerance", func(t *testing.T) {
		// A huge tolerance accepts the rough initial guess as-is: convergence
		// is declared before the first iteration and nothing moves.
		s, _, b, _, _ := newRectangle(t)
		res, err := s.Solve(sketch.WithTolerance(1e6))
		require.NoError(t, err)
		require.True(t, res.Converged, "anything is within a 1e6 tolerance")
		require.Equal(t, 0, res.Iterations, "no iterations needed")
		require.InDelta(t, 18, b.X(), 1e-9, "geometry untouched at the loose tolerance")
	})
}

// TestDragSmoothness emulates the GUI drag interaction: a vertex of a
// constrained parallelogram is pulled through many small goal targets, the way
// pointer-move events arrive. Every intermediate solve must converge with the
// shape-holding constraints intact — a mid-drag divergence would make
// interactive dragging unusable.
func TestDragSmoothness(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	s.Fix(a)
	s.Fix(b)
	ab := addLn(s, a, b)

	d := addPt(s, 2, 5)
	c := addPt(s, 12, 5)
	dc := addLn(s, d, c)
	s.AddConstraint(sketch.NewParallel(ab, dc), sketch.NewEqual(ab, dc))
	mustSolve(t, s)

	const steps = 50
	for i := 1; i <= steps; i++ {
		f := float64(i) / steps
		tx := 2 + 10*f // d: (2,5) -> (12,15)
		ty := 5 + 10*f
		res, err := s.Solve(sketch.WithGoal(d, tx, ty))
		require.NoErrorf(t, err, "drag step %d", i)
		require.Truef(t, res.Converged, "drag step %d converged", i)

		d1x, d1y := lineDir(ab)
		d2x, d2y := lineDir(dc)
		require.InDeltaf(t, 0, d1x*d2y-d1y*d2x, 1e-6, "parallel held during drag (step %d)", i)
		require.InDeltaf(t, 10, pointDist(d, c), 1e-6, "equal length held during drag (step %d)", i)
	}

	// The path stayed inside the feasible region, so the drag ends with the
	// vertex at the pointer.
	require.InDelta(t, 12, d.X(), 1e-3, "d.X tracks the pointer")
	require.InDelta(t, 15, d.Y(), 1e-3, "d.Y tracks the pointer")
}
