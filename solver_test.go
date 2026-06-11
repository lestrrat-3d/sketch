package sketch_test

import (
	"math"
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

// --- solver behavioral contracts (docs/acceptance-tests.md §3) --------------
//
// These tests promote properties of the current solver — minimum-norm steps,
// branch stability, finiteness — from implementation accidents to contracts.
// A solver change that breaks one of these changes how every downstream tool
// feels to use; that is a design decision, not a refactor.

// TestMinimalMotionOnEdit: editing one dimension must not disturb geometry the
// edit does not implicate. The Levenberg (absolute) damping gives the
// minimum-norm step for under-constrained sketches, which is what makes this
// hold; see the solver invariants in CLAUDE.md.
func TestMinimalMotionOnEdit(t *testing.T) {
	s := sketch.New()
	// An under-constrained bar from a grounded origin.
	a := addPt(s, 0, 0)
	s.Fix(a)
	b := addPt(s, 10, 0)
	w := addDist(s, a, b, 10)

	// A satisfied, floating cluster far away; nothing ties it to the bar.
	c := addPt(s, 100, 100)
	d := addPt(s, 108, 100)
	addDist(s, c, d, 8)
	mustSolve(t, s)

	w.Set(14)
	mustSolve(t, s)
	require.InDelta(t, 14, pointDist(a, b), 1e-6, "edited dimension applied")
	// The bar stretches along its own axis (the constraint gradient)…
	require.InDelta(t, 14, b.X(), 1e-6, "b moves radially")
	require.InDelta(t, 0, b.Y(), 1e-6, "b does not wander off axis")
	// …and the unrelated cluster does not drift at all.
	require.InDelta(t, 100, c.X(), 1e-9, "far cluster c.X untouched")
	require.InDelta(t, 100, c.Y(), 1e-9, "far cluster c.Y untouched")
	require.InDelta(t, 108, d.X(), 1e-9, "far cluster d.X untouched")
	require.InDelta(t, 100, d.Y(), 1e-9, "far cluster d.Y untouched")
}

// TestNoFlipOnLargeEdit: a drastic dimension edit applied in one step must
// keep the solution branch — the rectangle grows, it does not mirror through
// its grounded corner.
func TestNoFlipOnLargeEdit(t *testing.T) {
	s, w, b, c, d := newRectangle(t)
	mustSolve(t, s)

	w.Set(500) // 25× in a single step
	mustSolve(t, s)
	require.InDelta(t, 500, b.X(), 1e-6, "width grew in the original direction")
	require.InDelta(t, 500, c.X(), 1e-6, "c followed")
	require.InDelta(t, 12, c.Y(), 1e-6, "height (and its sign) preserved")
	require.InDelta(t, 12, d.Y(), 1e-6, "no mirror through the grounded corner")
}

// TestNearestSolutionPreserved: where a constraint admits several solution
// branches (a circle tangent to a line can sit on either side), the solver
// keeps the branch the geometry starts on — across the initial solve and
// across re-solves after edits.
func TestNearestSolutionPreserved(t *testing.T) {
	s := sketch.New()
	// A vertical line along x = 0.
	a := addPt(s, 0, 0)
	b := addPt(s, 0, 10)
	s.Fix(a)
	s.Fix(b)
	line := addLn(s, a, b)

	// A circle starting on the left side; tangency admits x = ±r.
	o := addPt(s, -5, 3)
	circ := addCir(s, o, 2)
	r := sketch.NewRadius(circ, 3)
	s.AddConstraint(r, sketch.NewTangent(line, circ))

	mustSolve(t, s)
	require.InDelta(t, -3, o.X(), 1e-6, "circle settles on its starting (left) side")

	r.Set(4) // editing the radius must not teleport it across the line
	mustSolve(t, s)
	require.InDelta(t, -4, o.X(), 1e-6, "circle stays left after the edit")
}

// TestSolverNeverReturnsNaN: contradictory or degenerate input is reported as
// an error (or solved through), never as NaN/Inf coordinates. A GUI layer
// redraws from these values on every event; one NaN poisons the canvas.
func TestSolverNeverReturnsNaN(t *testing.T) {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

	t.Run("contradictory dimensions", func(t *testing.T) {
		s := sketch.New()
		a := addPt(s, 0, 0)
		s.Fix(a)
		b := addPt(s, 3, 1)
		addDist(s, a, b, 5)
		addDist(s, a, b, 8) // cannot both hold

		res, err := s.Solve()
		require.ErrorIs(t, err, sketch.ErrNotConverged, "contradiction reported as an error")
		require.True(t, finite(res.Residual), "residual stays finite")
		require.True(t, finite(b.X()), "b.X stays finite")
		require.True(t, finite(b.Y()), "b.Y stays finite")
	})
	t.Run("degenerate zero-length line", func(t *testing.T) {
		s := sketch.New()
		// A zero-length line: both endpoints at the same spot. Residuals that
		// divide by its length must stay finite (norm() floors away from zero)
		// so the solver can pull the points apart.
		a := addPt(s, 0, 0)
		s.Fix(a)
		b := addPt(s, 0, 0)
		l1 := addLn(s, a, b)

		c := addPt(s, 5, 0)
		d := addPt(s, 5, 5)
		s.Fix(c)
		s.Fix(d)
		l2 := addLn(s, c, d)

		s.AddConstraint(sketch.NewPerpendicular(l1, l2))
		addDist(s, a, b, 5)

		_, err := s.Solve() // converging is welcome, NaN is not
		if err != nil {
			require.ErrorIs(t, err, sketch.ErrNotConverged, "only the documented error")
		}
		require.True(t, finite(b.X()), "b.X stays finite")
		require.True(t, finite(b.Y()), "b.Y stays finite")
	})
}

// TestSolveDeterministic: the same sketch built the same way solves to
// bit-identical coordinates. Residual assembly iterates slices in creation
// order — no map-order dependence may creep in.
func TestSolveDeterministic(t *testing.T) {
	s1, _, _, _, _ := newRectangle(t)
	mustSolve(t, s1)
	s2, _, _, _, _ := newRectangle(t)
	mustSolve(t, s2)

	for i, p := range s1.Points() {
		q := s2.Points()[i]
		require.Equalf(t, p.X(), q.X(), "point %d X bit-identical", i)
		require.Equalf(t, p.Y(), q.Y(), "point %d Y bit-identical", i)
	}
}

// TestScaleInvariance: the same shape solves to proportionally identical
// coordinates whether it is drawn at 0.01mm or metre scale (coordinates are
// always base-unit millimetres).
func TestScaleInvariance(t *testing.T) {
	build := func(scale float64) (*sketch.Sketch, *sketch.Point, *sketch.Point) {
		s := sketch.New()
		a := addPt(s, 0, 0)
		b := addPt(s, 18*scale, 2*scale) // rough guesses, like newRectangle
		c := addPt(s, 17*scale, 11*scale)
		d := addPt(s, 1*scale, 13*scale)
		ab := addLn(s, a, b)
		bc := addLn(s, b, c)
		dc := addLn(s, d, c)
		ad := addLn(s, a, d)
		s.Fix(a)
		s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
		addDist(s, a, b, 20*scale)
		addDist(s, a, d, 12*scale)
		return s, b, c
	}

	for _, scale := range []float64{0.01, 1, 1000} {
		s, b, c := build(scale)
		mustSolve(t, s)
		require.InEpsilonf(t, 20*scale, b.X(), 1e-9, "b.X at scale %v", scale)
		require.InEpsilonf(t, 20*scale, c.X(), 1e-9, "c.X at scale %v", scale)
		require.InEpsilonf(t, 12*scale, c.Y(), 1e-9, "c.Y at scale %v", scale)
	}
}
