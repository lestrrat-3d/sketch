// This file, like jacobian_inventory_test.go, is a deliberate INTERNAL test
// (package sketch): the claim under test is that the local residual Jacobian
// reproduces the dense one BIT FOR BIT, and no public call can observe either.
// Solve reports converged geometry, and two Jacobians differing in the last ulp
// still converge to geometry that passes every public assertion. Exporting the
// two builders to serve a test would be worse.
package sketch

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// jacFixture is one sketch the dense and local Jacobians are compared on. The
// list deliberately spans every structural feature the plan has to handle:
// several residual rows per constraint, internal constraints, driven
// dimensions, zero-row constraints, shared points, shape variables (radii,
// ellipse axes and rotation), spline geometry, auxiliary tangency and foot
// variables, and extreme finite scales.
type jacFixture struct {
	name  string
	build func(t *testing.T) *Sketch
}

func jacFixtures() []jacFixture {
	return []jacFixture{
		{"grounded_rectangle", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			// A shared corner is literally one point, so this fixture carries
			// the shared-point case: four points, four lines.
			p := []*Point{s.CreatePoint(0, 0), s.CreatePoint(5.3, 0.2), s.CreatePoint(5.1, 3.4), s.CreatePoint(0.2, 3.1)}
			l := []*Line{
				s.CreateLine(p[0], p[1]), s.CreateLine(p[1], p[2]),
				s.CreateLine(p[2], p[3]), s.CreateLine(p[3], p[0]),
			}
			s.AddConstraint(NewCoincident(p[0], s.Origin()))
			s.AddConstraint(NewHorizontal(l[0]))
			s.AddConstraint(NewParallel(l[0], l[2]))
			s.AddConstraint(NewParallel(l[1], l[3]))
			s.AddConstraint(NewPerpendicular(l[0], l[1]))
			s.AddConstraint(NewDistance(p[0], p[1], 6))
			s.AddConstraint(NewDistance(p[1], p[2], 4))
			return s
		}},
		{"circle_and_arc_tangency", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			ci := s.CreateCircle(s.CreatePoint(0, 0), 2)
			s.AddConstraint(NewRadius(ci, 2.5))
			a := s.CreateArc(s.CreatePoint(8, 0), s.CreatePoint(10, 0), s.CreatePoint(8, 2))
			l := s.CreateLine(s.CreatePoint(4, 2.4), s.CreatePoint(12, 2.4))
			s.AddConstraint(NewTangent(l, a)) // interior tangency: aux slack + sweep row
			s.AddConstraint(NewArcLength(a, 3.4))
			return s
		}},
		{"spline_and_conic", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			sp, err := s.CreateSpline(s.CreatePoint(0, 0), s.CreatePoint(1, 2), s.CreatePoint(3, 2), s.CreatePoint(4, 0))
			require.NoError(t, err)
			s.AddConstraint(NewPointOnSpline(s.CreatePoint(2, 1.4), sp))
			co, err := s.CreateConic(s.CreatePoint(6, 0), s.CreatePoint(8, 3), s.CreatePoint(10, 0), 0.5)
			require.NoError(t, err)
			s.AddConstraint(NewPointOnConic(s.CreatePoint(8, 1.4), co))
			return s
		}},
		{"ellipse_shape_vars", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0.1)
			s.AddConstraint(NewSemiMajor(e, 5))
			s.AddConstraint(NewSemiMinor(e, 1.5))
			s.AddConstraint(NewEllipseRotation(e, 20))
			ea := s.CreateEllipticalArc(s.CreatePoint(9, 0), s.CreatePoint(12, 0), s.CreatePoint(9, 2), 3, 2, 0)
			s.AddConstraint(NewPointOnEllipticalArc(s.CreatePoint(11, 1.4), ea))
			return s
		}},
		{"driven_and_zero_row", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			a, b := s.CreatePoint(0, 0), s.CreatePoint(3, 1)
			s.AddConstraint(NewCoincident(a, s.Origin()))
			s.AddConstraint(NewDistance(a, b, 4))
			// A driven dimension contributes no residual row at all: the plan
			// must skip it exactly as residuals() does, or every row after it
			// is attributed to the wrong constraint.
			d := NewDistance(a, b, 0)
			d.SetDriven(true)
			s.AddConstraint(d)
			s.AddConstraint(NewHorizontalPoints(a, b))
			return s
		}},
		{"extreme_finite_scale", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			a, b := s.CreatePoint(1e8, -3e8), s.CreatePoint(4e8, 2.5e8)
			s.AddConstraint(NewDistance(a, b, 7e8))
			ci := s.CreateCircle(s.CreatePoint(-2e8, 1e8), 1e7)
			s.AddConstraint(NewDistancePointCircle(a, ci, 3e8))
			return s
		}},
	}
}

// newJacobianMatrix returns a fresh m×n matrix, the shape both builders write.
func newJacobianMatrix(m, n int) [][]float64 {
	out := make([][]float64, m)
	for i := range out {
		out[i] = make([]float64, n)
	}
	return out
}

// requireLocalMatchesDense builds both Jacobians at the sketch's current
// configuration and requires every entry to be the same float64 BIT PATTERN,
// not merely the same value: a Jacobian differing in the last ulp still steers
// the Levenberg–Marquardt step, and with it which configuration a solve — and
// every [Sketch.ProbeConfigurations] restart — lands in.
func requireLocalMatchesDense(t *testing.T, s *Sketch) {
	t.Helper()
	free := s.freeVars()
	r := s.residuals(nil)
	m, n := len(r), len(free)
	require.Positive(t, m, "fixture produces no residual rows")
	require.Positive(t, n, "fixture has no free variables")

	before := append([]float64(nil), s.vars...)
	want := newJacobianMatrix(m, n)
	s.jacobianInto(want, free, m, s.residuals, nil, nil)

	plan, ok := s.newResidualPlan(len(s.residuals(nil)))
	require.True(t, ok, "every fixture's constraints must be classified")
	got := newJacobianMatrix(m, n)
	require.True(t, s.jacobianLocalInto(got, free, m, plan, r), "the local builder refused a supported fixture")

	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			require.Equalf(t, math.Float64bits(want[i][j]), math.Float64bits(got[i][j]),
				"J[%d][%d]: dense %v, local %v", i, j, want[i][j], got[i][j])
		}
	}
	for i, v := range before {
		require.Equal(t, math.Float64bits(v), math.Float64bits(s.vars[i]),
			"variable %d was not restored to its original bit pattern", i)
	}
}

func TestLocalJacobianMatchesDense(t *testing.T) {
	for _, f := range jacFixtures() {
		t.Run(f.name, func(t *testing.T) {
			s := f.build(t)
			// As authored, mid-solve, and at the solved configuration: the
			// dependency structure is the same at all three, the VALUES are not.
			requireLocalMatchesDense(t, s)

			_, err := s.Solve(t.Context(), WithMaxIterations(3))
			_ = err // a partial solve is fine; the comparison is what matters
			requireLocalMatchesDense(t, s)

			_, _ = s.Solve(t.Context())
			requireLocalMatchesDense(t, s)
		})
	}
}

// TestLocalJacobianMatchesDenseAtPerturbedStates walks each fixture through a
// deterministic sequence of displaced configurations, so the comparison is not
// resting on the one configuration the builder happened to author.
func TestLocalJacobianMatchesDenseAtPerturbedStates(t *testing.T) {
	for _, f := range jacFixtures() {
		t.Run(f.name, func(t *testing.T) {
			s := f.build(t)
			// splitmix64, the generator the ambiguity probe already uses, so
			// the displacement sequence is fixed and reproducible.
			seed := uint64(0x9E3779B97F4A7C15)
			next := func() float64 {
				seed += 0x9E3779B97F4A7C15
				z := seed
				z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
				z = (z ^ (z >> 27)) * 0x94D049BB133111EB
				z ^= z >> 31
				return float64(z>>11)/float64(uint64(1)<<53)*2 - 1
			}
			for round := 0; round < 5; round++ {
				for i := range s.vars {
					if !s.fixed[i] {
						s.vars[i] += next() * 0.75
					}
				}
				requireLocalMatchesDense(t, s)
			}
		})
	}
}

// TestLocalSolveMatchesDenseSolve is the same claim one level up: two identical
// sketches solved through the same lm loop, one with the residual plan and one
// without, end at the same variable vector BIT FOR BIT. It is the comparison
// that covers what a Jacobian-entry check cannot — the damping trials, the
// accept/reject decisions and the iteration count that follow from the step.
//
// It also carries the between-solve edits the plan has to survive: a bound
// parameter's value changed between solves, and a constraint and an entity
// added and removed between them. The plan is rebuilt inside every lm call, so
// each of those is a fresh plan over a changed constraint list.
func TestLocalSolveMatchesDenseSolve(t *testing.T) {
	solve := func(s *Sketch, mode jacobianMode) {
		o := defaultSolveConfig()
		_, err := s.lm(context.Background(), s.freeVars(), s.residuals, mode, o.maxIterations, o.tolerance)
		require.NoError(t, err)
	}
	requireSameVars := func(t *testing.T, a, b *Sketch, what string) {
		t.Helper()
		require.Equal(t, len(a.vars), len(b.vars), "%s: variable counts diverged", what)
		for i := range a.vars {
			require.Equalf(t, math.Float64bits(a.vars[i]), math.Float64bits(b.vars[i]),
				"%s: variable %d differs (%v vs %v)", what, i, a.vars[i], b.vars[i])
		}
	}

	for _, f := range jacFixtures() {
		t.Run(f.name, func(t *testing.T) {
			local, dense := f.build(t), f.build(t)
			solve(local, localJacobian)
			solve(dense, denseJacobian)
			requireSameVars(t, local, dense, "first solve")

			// An entity and a constraint added between solves: a longer
			// variable vector and a longer constraint list, so a plan carried
			// over from the first solve would be stale. It is not carried over.
			addMore := func(s *Sketch) {
				c := s.CreateCircle(s.CreatePoint(2, 2), 1)
				s.AddConstraint(NewRadius(c, 1.75))
			}
			addMore(local)
			addMore(dense)
			solve(local, localJacobian)
			solve(dense, denseJacobian)
			requireSameVars(t, local, dense, "after adding geometry")

			// And removed again: the removal cascade retires variables and
			// splices the constraint list.
			removeLast := func(s *Sketch) {
				ents := s.Entities()
				require.True(t, s.RemoveEntity(ents[len(ents)-1]))
			}
			removeLast(local)
			removeLast(dense)
			solve(local, localJacobian)
			solve(dense, denseJacobian)
			requireSameVars(t, local, dense, "after removing geometry")
		})
	}
}

// TestLocalSolveMatchesDenseAcrossParameterEdits pins the bound-parameter half
// separately, because a bound dimension's target is refreshed by
// ApplyParameters BEFORE the solve and is not a solver variable — so an edit
// moves what the residual reads without moving any dependency, and the plan
// must neither notice nor care.
func TestLocalSolveMatchesDenseAcrossParameterEdits(t *testing.T) {
	build := func(t *testing.T) (*Sketch, *Distance) {
		t.Helper()
		s := newInventorySketch(t)
		require.NoError(t, s.Params().Set("width", "40"))
		a, b := s.CreatePoint(0, 0), s.CreatePoint(30, 0)
		s.AddConstraint(NewCoincident(a, s.Origin()))
		s.AddConstraint(NewHorizontalPoints(a, b))
		d := NewDistance(a, b, 30)
		require.NoError(t, s.Bind(d, s.Params(), "width"))
		s.AddConstraint(d)
		return s, d
	}
	local, _ := build(t)
	dense, _ := build(t)

	for _, expr := range []string{"40", "17.5", "63.25"} {
		require.NoError(t, local.Params().Set("width", expr))
		require.NoError(t, dense.Params().Set("width", expr))
		require.NoError(t, local.ApplyParameters())
		require.NoError(t, dense.ApplyParameters())
		o := defaultSolveConfig()
		_, err := local.lm(context.Background(), local.freeVars(), local.residuals, localJacobian, o.maxIterations, o.tolerance)
		require.NoError(t, err)
		_, err = dense.lm(context.Background(), dense.freeVars(), dense.residuals, denseJacobian, o.maxIterations, o.tolerance)
		require.NoError(t, err)
		for i := range local.vars {
			require.Equalf(t, math.Float64bits(local.vars[i]), math.Float64bits(dense.vars[i]),
				"width=%s: variable %d differs", expr, i)
		}
	}
}

// TestLocalJacobianRefusesNonFiniteResiduals pins the screen the bit-identity
// argument rests on. Where a residual row is not finite the dense pass computes
// ∞ − ∞ or NaN − NaN — NaN, not zero — for every column, including columns the
// row does not depend on. Clearing those entries would replace a NaN a
// structurally independent row genuinely produces, so one non-finite base
// residual costs the whole call its plan.
func TestLocalJacobianRefusesNonFiniteResiduals(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		s := newInventorySketch(t)
		a, b := s.CreatePoint(0, 0), s.CreatePoint(3, 0)
		s.AddConstraint(NewDistance(a, b, 4))
		c := s.CreateCircle(s.CreatePoint(9, 0), 2)
		s.AddConstraint(NewRadius(c, 3))

		free := s.freeVars()
		plan, ok := s.newResidualPlan(len(s.residuals(nil)))
		require.True(t, ok)
		r := s.residuals(nil)
		require.True(t, s.jacobianLocalInto(newJacobianMatrix(len(r), len(free)), free, len(r), plan, r),
			"the finite control must be accepted")

		b.MoveTo(bad, 0)
		r = s.residuals(nil)
		require.False(t, s.jacobianLocalInto(newJacobianMatrix(len(r), len(free)), free, len(r), plan, r),
			"a non-finite base residual must send the whole call to the dense builder")
	}
}

// TestLocalJacobianRefusesForeignEvaluatorShapes pins the two ways a caller can
// hand lm a residual vector the plan does not describe: the goal-augmented
// system ([Sketch.goalResiduals], two extra rows per goal) and any other
// arbitrary evaluator. Both are caught by the row count, which is why the mode
// at the call site is a belt-and-braces statement rather than the only defence.
func TestLocalJacobianRefusesForeignEvaluatorShapes(t *testing.T) {
	s := newInventorySketch(t)
	a, b := s.CreatePoint(0, 0), s.CreatePoint(3, 0)
	s.AddConstraint(NewCoincident(a, s.Origin()))
	s.AddConstraint(NewDistance(a, b, 4))

	free := s.freeVars()
	plan, ok := s.newResidualPlan(len(s.residuals(nil)))
	require.True(t, ok)

	goals := []goalTarget{{p: b, tx: 4, ty: 1}}
	aug := s.goalResiduals(nil, goals)
	require.Len(t, aug, plan.m+2, "the goal system must be wider than the committed rows")
	require.False(t, s.jacobianLocalInto(newJacobianMatrix(len(aug), len(free)), free, len(aug), plan, aug),
		"the goal-augmented system must not be evaluated through the plan")

	// And the goal solve itself still works, on the dense path it takes.
	res, err := s.Solve(t.Context(), WithGoal(b, 4, 1))
	require.NoError(t, err)
	require.True(t, res.Converged)
	require.InDelta(t, 4.0, b.DistanceTo(a), 1e-9, "the hard constraint still holds exactly")
}

// TestLocalJacobianRefusesUnclassifiedConstraint pins the whole-plan refusal
// for a constraint [constraintRefs] does not list — the shape a new constraint
// type added without its removal.go case takes. An empty dependency set would
// zero every entry of a real row, so the plan refuses to be built at all rather
// than treating "names nothing" as "reads nothing".
func TestLocalJacobianRefusesUnclassifiedConstraint(t *testing.T) {
	s := newInventorySketch(t)
	a, b := s.CreatePoint(0, 0), s.CreatePoint(3, 0)
	s.AddConstraint(NewDistance(a, b, 4))
	require.True(t, hasPlan(s), "the control must be classified")

	s.AddConstraint(&unlistedConstraint{p: b})
	require.False(t, hasPlan(s), "an unlisted constraint kind must refuse the whole plan")

	// The solve still runs, on the dense builder, and still satisfies the rows
	// the unlisted constraint contributes.
	res, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, res.Converged)
	require.InDelta(t, 1.5, b.Y(), 1e-9)
}

func hasPlan(s *Sketch) bool {
	_, ok := s.newResidualPlan(len(s.residuals(nil)))
	return ok
}

// unlistedConstraint stands in for a constraint type added without a
// constraintRefs case: it reads a real point, and constraintRefs reports
// nothing for it.
type unlistedConstraint struct{ p *Point }

func (c *unlistedConstraint) residual(out []float64) []float64 {
	return append(out, c.p.y()-1.5)
}

// TestLocalJacobianRestoresVariableOnRowCountMismatch pins the restore on the
// refusal path, on BOTH perturbations. A constraint whose row count no longer
// matches the plan aborts the build mid-column, and the variable it was
// perturbing has to go back to its exact original bit pattern before the dense
// builder runs over the same sketch.
func TestLocalJacobianRestoresVariableOnRowCountMismatch(t *testing.T) {
	for _, side := range []string{"positive", "negative"} {
		t.Run(side, func(t *testing.T) {
			s := newInventorySketch(t)
			a, b := s.CreatePoint(0, 0), s.CreatePoint(3, 0)
			s.AddConstraint(NewCoincident(a, s.Origin()))
			s.AddConstraint(NewDistance(a, b, 4))

			free := s.freeVars()
			r := s.residuals(nil)
			plan, ok := s.newResidualPlan(len(s.residuals(nil)))
			require.True(t, ok)

			// Swap the last constraint for one that produces the recorded row
			// count on the perturbation being tested and a different count on
			// the other, so exactly one of the two evaluations trips the guard.
			k := len(plan.cons) - 1
			base := s.vars[b.xi]
			plan.cons[k] = &rowCountShiftConstraint{s: s, vi: b.xi, base: base, growAbove: side == "negative"}

			before := append([]float64(nil), s.vars...)
			require.False(t, s.jacobianLocalInto(newJacobianMatrix(len(r), len(free)), free, len(r), plan, r))
			for i, v := range before {
				require.Equalf(t, math.Float64bits(v), math.Float64bits(s.vars[i]),
					"variable %d was not restored after the refusal", i)
			}
		})
	}
}

// rowCountShiftConstraint produces one row on one side of base and two on the
// other, so a central-difference pass sees its row count change between the +h
// and −h evaluations.
type rowCountShiftConstraint struct {
	s         *Sketch
	vi        int
	base      float64
	growAbove bool
}

func (c *rowCountShiftConstraint) residual(out []float64) []float64 {
	out = append(out, c.s.vars[c.vi]-c.base)
	if (c.s.vars[c.vi] > c.base) == c.growAbove {
		out = append(out, 0)
	}
	return out
}

// TestLocalJacobianClearsStaleWorkspaceEntries pins the explicit clear. The
// caller's matrix is reused across every outer iteration of one lm call, and
// the local builder writes only the entries a column's constraints own — so a
// column that held nonzeros in one iteration and becomes structurally empty in
// the next must come back all zeros, not the previous iteration's values.
func TestLocalJacobianClearsStaleWorkspaceEntries(t *testing.T) {
	s := newInventorySketch(t)
	a, b := s.CreatePoint(0, 0), s.CreatePoint(3, 0)
	s.AddConstraint(NewCoincident(a, s.Origin()))
	d := NewDistance(a, b, 4)
	s.AddConstraint(d)

	free := s.freeVars()
	r := s.residuals(nil)
	J := newJacobianMatrix(len(r), len(free))

	plan, ok := s.newResidualPlan(len(s.residuals(nil)))
	require.True(t, ok)
	require.True(t, s.jacobianLocalInto(J, free, len(r), plan, r))
	nonzero := false
	for i := range J {
		for j := range J[i] {
			if J[i][j] != 0 {
				nonzero = true
			}
		}
	}
	require.True(t, nonzero, "the first build must leave nonzeros to go stale")

	// Drive the same matrix with a plan whose only constraint is driven, so
	// every column is now structurally empty.
	d.SetDriven(true)
	s.AddConstraint(NewHorizontalPoints(a, b))
	r2 := s.residuals(nil)
	plan2, ok := s.newResidualPlan(len(s.residuals(nil)))
	require.True(t, ok)
	J2 := newJacobianMatrix(len(r2), len(free))
	for i := range J2 {
		for j := range J2[i] {
			J2[i][j] = 7 // stale values a previous iteration could have left
		}
	}
	require.True(t, s.jacobianLocalInto(J2, free, len(r2), plan2, r2))

	dense := newJacobianMatrix(len(r2), len(free))
	s.jacobianInto(dense, free, len(r2), s.residuals, nil, nil)
	for i := range dense {
		for j := range dense[i] {
			require.Equalf(t, math.Float64bits(dense[i][j]), math.Float64bits(J2[i][j]),
				"J[%d][%d] kept a stale value", i, j)
		}
	}
}

// TestResidualPlanCutsEvaluationWork is the WORK assertion the timing runs rest
// on, expressed as a count rather than a wall clock: the plan's own structure
// says how many residual rows one Jacobian build evaluates, and on a sketch of
// realistic size that has to be a small fraction of the dense pass's m·n.
func TestResidualPlanCutsEvaluationWork(t *testing.T) {
	s := newInventorySketch(t)
	prev := s.CreatePoint(0, 0)
	first := prev
	s.AddConstraint(NewCoincident(first, s.Origin()))
	// A chain of 40 segments: each constraint touches two consecutive points,
	// so the Jacobian is banded — the shape a real profile sketch has.
	for i := 1; i <= 40; i++ {
		p := s.CreatePoint(float64(i), 0.1*float64(i%3))
		s.AddConstraint(NewDistance(prev, p, 1))
		s.AddConstraint(NewHorizontalPoints(prev, p))
		prev = p
	}

	free := s.freeVars()
	m := len(s.residuals(nil))
	plan, ok := s.newResidualPlan(len(s.residuals(nil)))
	require.True(t, ok)

	var localRows int
	for _, vi := range free {
		for _, k := range plan.consFor(vi) {
			localRows += plan.count[k]
		}
	}
	denseRows := m * len(free)
	require.Positive(t, localRows)
	require.Lessf(t, localRows*10, denseRows,
		"the plan evaluates %d rows per perturbation pass against the dense %d — under a 10x cut it is not worth its own construction",
		localRows, denseRows)
}

// buildRectangleChain returns a chain of n rectangles, each grounded to the
// previous one's B corner and carrying its own width/height dimension — the
// same shape as bench_test.go's large fixture, rebuilt here because that one
// lives in the external test package.
func buildRectangleChain(tb testing.TB, n int) *Sketch {
	tb.Helper()
	w := NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(tb, err)
	var prev *Rectangle
	x := 0.0
	for k := 0; k < n; k++ {
		r := s.CreateRectangle(x, 0, x+20, 12)
		s.AddConstraint(NewDistance(r.A, r.B, 20), NewDistance(r.A, r.D, 12))
		if prev == nil {
			s.AddConstraint(NewCoincident(r.A, s.Origin()))
		} else {
			s.AddConstraint(NewCoincident(r.A, prev.B))
		}
		prev = r
		x += 20
	}
	if _, err := s.Solve(context.Background()); err != nil {
		tb.Fatal(err)
	}
	return s
}

// BenchmarkJacobianBuild measures one Jacobian build both ways on the same
// solved sketch: the dense pass that reevaluates every residual row per
// variable, and the local pass that reevaluates only the rows a variable
// reaches. The chain lengths bracket the size a real gear profile reaches.
func BenchmarkJacobianBuild(b *testing.B) {
	for _, n := range []int{4, 40} {
		s := buildRectangleChain(b, n)
		free := s.freeVars()
		r := s.residuals(nil)
		m := len(r)
		J := newJacobianMatrix(m, len(free))
		plan, ok := s.newResidualPlan(len(s.residuals(nil)))
		require.True(b, ok)

		b.Run(fmt.Sprintf("dense/%drect", n), func(b *testing.B) {
			b.ReportAllocs()
			var rp, rm []float64
			for b.Loop() {
				s.jacobianInto(J, free, m, s.residuals, rp, rm)
			}
		})
		b.Run(fmt.Sprintf("local/%drect", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if !s.jacobianLocalInto(J, free, m, plan, r) {
					b.Fatal("the local builder refused")
				}
			}
		})
		b.Run(fmt.Sprintf("plan-build/%drect", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, ok := s.newResidualPlan(len(s.residuals(nil))); !ok {
					b.Fatal("plan refused")
				}
			}
		})
	}
}

// TestProbeMatchesDenseProbe drives the SAME multi-start search both ways and
// requires the configurations to match bit for bit, in the same order. The
// probe's whole value is that its result is deterministic and reproducible, and
// it is where the gear proofs spend most of their solver time, so the local
// Jacobian's bit-identity has to be shown of the search itself and not only of
// one matrix. Each fixture is a DOF-0 sketch, the probe's precondition.
func TestProbeMatchesDenseProbe(t *testing.T) {
	fixtures := []struct {
		name  string
		build func(t *testing.T) *Sketch
	}{
		{"mirror_triangle", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			a, b, apex := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(5, 4)
			s.AddConstraint(NewCoincident(a, s.Origin()))
			s.AddConstraint(NewHorizontalPoints(a, b))
			s.AddConstraint(NewDistance(a, b, 10))
			s.AddConstraint(NewDistance(a, apex, 7))
			s.AddConstraint(NewDistance(b, apex, 7))
			return s
		}},
		{"tangent_arc_and_circle", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			c := s.CreateCircle(s.CreatePoint(0, 0), 3)
			s.AddConstraint(NewCoincident(c.Center, s.Origin()))
			s.AddConstraint(NewRadius(c, 3))
			p := s.CreatePoint(4, 2)
			s.AddConstraint(NewDistancePointCircle(p, c, 2))
			s.AddConstraint(NewHorizontalDistance(c.Center, p, 4))
			return s
		}},
		{"rectangle_with_diagonal", func(t *testing.T) *Sketch {
			s := newInventorySketch(t)
			r := s.CreateRectangle(0, 0, 20, 12)
			s.AddConstraint(NewCoincident(r.A, s.Origin()))
			s.AddConstraint(NewDistance(r.A, r.B, 20))
			s.AddConstraint(NewDistance(r.A, r.D, 12))
			return s
		}},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			local, dense := f.build(t), f.build(t)
			for _, s := range []*Sketch{local, dense} {
				_, err := s.Solve(t.Context())
				require.NoError(t, err)
			}
			cfg := defaultProbeConfig()
			lr, lerr := local.probeConfigurations(t.Context(), cfg, localJacobian)
			dr, derr := dense.probeConfigurations(t.Context(), cfg, denseJacobian)
			require.Equal(t, derr, lerr)
			require.NoError(t, lerr)
			require.Len(t, lr.Configurations, len(dr.Configurations),
				"the plan changed how many configurations the search finds")
			for i := range lr.Configurations {
				lv, dv := lr.Configurations[i].vars, dr.Configurations[i].vars
				require.Len(t, lv, len(dv))
				for j := range lv {
					require.Equalf(t, math.Float64bits(dv[j]), math.Float64bits(lv[j]),
						"configuration %d, variable %d differs (%v vs %v)", i, j, dv[j], lv[j])
				}
			}
			// The sketch is restored either way, so the two also agree on what
			// they left behind.
			for i := range local.vars {
				require.Equal(t, math.Float64bits(dense.vars[i]), math.Float64bits(local.vars[i]))
			}
		})
	}
}
