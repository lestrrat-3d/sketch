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
		s := newSketch(t)
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
		res, err := s.Solve(sketch.WithMaxIterations(1))
		require.ErrorIs(t, err, sketch.ErrNotConverged, "one iteration is not enough")
		require.False(t, res.Converged, "not converged")
		require.LessOrEqual(t, res.Iterations, 1, "iteration budget honored")

		// The same sketch solves with the default budget.
		_, err = s.Solve()
		require.NoError(t, err)
	})
	t.Run("tolerance", func(t *testing.T) {
		// A huge tolerance accepts the rough initial guess as-is: convergence
		// is declared before the first iteration and nothing moves.
		s := newSketch(t)
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
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	ab := s.AddLine(a, b)

	d := s.AddPoint(2, 5)
	c := s.AddPoint(12, 5)
	dc := s.AddLine(d, c)
	s.AddConstraint(sketch.NewParallel(ab, dc), sketch.NewEqual(ab, dc))
	_, err := s.Solve()
	require.NoError(t, err)

	const steps = 50
	for i := 1; i <= steps; i++ {
		f := float64(i) / steps
		tx := 2 + 10*f // d: (2,5) -> (12,15)
		ty := 5 + 10*f
		res, err := s.Solve(sketch.WithGoal(d, tx, ty))
		require.NoErrorf(t, err, "drag step %d", i)
		require.Truef(t, res.Converged, "drag step %d converged", i)

		d1x, d1y := ab.End.X()-ab.Start.X(), ab.End.Y()-ab.Start.Y()
		d2x, d2y := dc.End.X()-dc.Start.X(), dc.End.Y()-dc.Start.Y()
		require.InDeltaf(t, 0, d1x*d2y-d1y*d2x, 1e-6, "parallel held during drag (step %d)", i)
		require.InDeltaf(t, 10, d.DistanceTo(c), 1e-6, "equal length held during drag (step %d)", i)
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
	s := newSketch(t)
	// An under-constrained bar from a grounded origin.
	a := s.AddPoint(0, 0)
	s.Fix(a)
	b := s.AddPoint(10, 0)
	w := sketch.NewDistance(a, b, 10)
	s.AddConstraint(w)

	// A satisfied, floating cluster far away; nothing ties it to the bar.
	c := s.AddPoint(100, 100)
	d := s.AddPoint(108, 100)
	s.AddConstraint(sketch.NewDistance(c, d, 8))
	_, err := s.Solve()
	require.NoError(t, err)

	w.Set(14)
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 14, a.DistanceTo(b), 1e-6, "edited dimension applied")
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
	s := newSketch(t)
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
	w := sketch.NewDistance(a, b, 20)
	s.AddConstraint(w)
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	_, err := s.Solve()
	require.NoError(t, err)

	w.Set(500) // 25× in a single step
	_, err = s.Solve()
	require.NoError(t, err)
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
	s := newSketch(t)
	// A vertical line along x = 0.
	a := s.AddPoint(0, 0)
	b := s.AddPoint(0, 10)
	s.Fix(a)
	s.Fix(b)
	line := s.AddLine(a, b)

	// A circle starting on the left side; tangency admits x = ±r.
	o := s.AddPoint(-5, 3)
	circ := s.AddCircle(o, 2)
	r := sketch.NewRadius(circ, 3)
	s.AddConstraint(r, sketch.NewTangent(line, circ))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, -3, o.X(), 1e-6, "circle settles on its starting (left) side")

	r.Set(4) // editing the radius must not teleport it across the line
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, -4, o.X(), 1e-6, "circle stays left after the edit")
}

// TestSolverNeverReturnsNaN: contradictory or degenerate input is reported as
// an error (or solved through), never as NaN/Inf coordinates. A GUI layer
// redraws from these values on every event; one NaN poisons the canvas.
func TestSolverNeverReturnsNaN(t *testing.T) {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

	t.Run("contradictory dimensions", func(t *testing.T) {
		s := newSketch(t)
		a := s.AddPoint(0, 0)
		s.Fix(a)
		b := s.AddPoint(3, 1)
		s.AddConstraint(sketch.NewDistance(a, b, 5))
		s.AddConstraint(sketch.NewDistance(a, b, 8)) // cannot both hold

		res, err := s.Solve()
		require.ErrorIs(t, err, sketch.ErrNotConverged, "contradiction reported as an error")
		require.True(t, finite(res.Residual), "residual stays finite")
		require.True(t, finite(b.X()), "b.X stays finite")
		require.True(t, finite(b.Y()), "b.Y stays finite")
	})
	t.Run("degenerate zero-length line", func(t *testing.T) {
		s := newSketch(t)
		// A zero-length line: both endpoints at the same spot. Residuals that
		// divide by its length must stay finite (norm() floors away from zero)
		// so the solver can pull the points apart.
		a := s.AddPoint(0, 0)
		s.Fix(a)
		b := s.AddPoint(0, 0)
		l1 := s.AddLine(a, b)

		c := s.AddPoint(5, 0)
		d := s.AddPoint(5, 5)
		s.Fix(c)
		s.Fix(d)
		l2 := s.AddLine(c, d)

		s.AddConstraint(sketch.NewPerpendicular(l1, l2))
		s.AddConstraint(sketch.NewDistance(a, b, 5))

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
	s1 := newSketch(t)
	a1 := s1.AddPoint(0, 0)
	b1 := s1.AddPoint(18, 2)
	c1 := s1.AddPoint(17, 11)
	d1 := s1.AddPoint(1, 13)
	s1.AddConstraint(sketch.NewHorizontal(s1.AddLine(a1, b1)), sketch.NewHorizontal(s1.AddLine(d1, c1)), sketch.NewVertical(s1.AddLine(a1, d1)), sketch.NewVertical(s1.AddLine(b1, c1)))
	a1.MoveTo(0, 0)
	s1.Fix(a1)
	s1.AddConstraint(sketch.NewDistance(a1, b1, 20))
	s1.AddConstraint(sketch.NewDistance(a1, d1, 12))
	_, err := s1.Solve()
	require.NoError(t, err)

	s2 := newSketch(t)
	a2 := s2.AddPoint(0, 0)
	b2 := s2.AddPoint(18, 2)
	c2 := s2.AddPoint(17, 11)
	d2 := s2.AddPoint(1, 13)
	s2.AddConstraint(sketch.NewHorizontal(s2.AddLine(a2, b2)), sketch.NewHorizontal(s2.AddLine(d2, c2)), sketch.NewVertical(s2.AddLine(a2, d2)), sketch.NewVertical(s2.AddLine(b2, c2)))
	a2.MoveTo(0, 0)
	s2.Fix(a2)
	s2.AddConstraint(sketch.NewDistance(a2, b2, 20))
	s2.AddConstraint(sketch.NewDistance(a2, d2, 12))
	_, err = s2.Solve()
	require.NoError(t, err)

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
		s := newSketch(t)
		a := s.AddPoint(0, 0)
		b := s.AddPoint(18*scale, 2*scale) // rough guesses, like newRectangle
		c := s.AddPoint(17*scale, 11*scale)
		d := s.AddPoint(1*scale, 13*scale)
		ab := s.AddLine(a, b)
		bc := s.AddLine(b, c)
		dc := s.AddLine(d, c)
		ad := s.AddLine(a, d)
		s.Fix(a)
		s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
		s.AddConstraint(sketch.NewDistance(a, b, 20*scale))
		s.AddConstraint(sketch.NewDistance(a, d, 12*scale))
		return s, b, c
	}

	for _, scale := range []float64{0.01, 1, 1000} {
		s, b, c := build(scale)
		_, err := s.Solve()
		require.NoError(t, err)
		require.InEpsilonf(t, 20*scale, b.X(), 1e-9, "b.X at scale %v", scale)
		require.InEpsilonf(t, 20*scale, c.X(), 1e-9, "c.X at scale %v", scale)
		require.InEpsilonf(t, 12*scale, c.Y(), 1e-9, "c.Y at scale %v", scale)
	}
}
