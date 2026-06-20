package sketch

import (
	"math"

	"github.com/lestrrat-3d/sketch/units"
	"github.com/lestrrat-go/option/v3"
)

// Result reports the outcome of a [Sketch.Solve] call.
type Result struct {
	Converged  bool    // every residual is within the tolerance
	Iterations int     // outer Levenberg–Marquardt iterations performed
	Residual   float64 // Euclidean norm of the final residual vector
	DOF        int     // remaining degrees of freedom (0 == fully constrained)
	Redundant  int     // number of redundant/conflicting constraint equations
}

// SolveOption tunes the constraint solver. Construct values with the With…
// helpers; any option left unset falls back to a sensible default.
type SolveOption interface {
	option.Interface
	solveOption()
}

type solveOption struct{ option.Interface }

func (solveOption) solveOption() {}

// SolveVerifyOption is accepted by both [Sketch.Solve] and [Sketch.Verify]
// (the jwx combined-interface pattern, like SVGPNGOption): its concrete type
// carries both marker methods, so one [WithTolerance] value flows into either
// — the convergence threshold the solver targets and the threshold Verify
// judges solvability against are then guaranteed to agree.
type SolveVerifyOption interface {
	SolveOption
	VerifyOption
}

type solveVerifyOption struct{ option.Interface }

func (solveVerifyOption) solveOption()  {}
func (solveVerifyOption) verifyOption() {}

type (
	identMaxIterations struct{}
	identTolerance     struct{}
	identGoal          struct{}
)

// WithMaxIterations sets the outer Levenberg–Marquardt iteration budget.
func WithMaxIterations(v int) SolveOption { return solveOption{option.New(identMaxIterations{}, v)} }

// WithTolerance sets the convergence threshold on the residual norm. It is
// accepted by both [Sketch.Solve] (where it is the convergence target) and
// [Sketch.Verify] (where it is the threshold the Solvable verdict uses), so the
// two stay consistent.
func WithTolerance(v float64) SolveVerifyOption {
	return solveVerifyOption{option.New(identTolerance{}, v)}
}

// goalTarget is a transient soft target for one point, valid for a single
// Solve call. See docs/goal-solve-design.md.
type goalTarget struct {
	p      *Point
	tx, ty float64
}

// goalWeight scales goal residuals. It is dimensionless and small so that hard
// constraints always win; goals only steer degrees of freedom the constraints
// leave open.
const goalWeight = 1e-3

// WithGoal asks the solver to pull point p toward (x, y) — base units, like
// all point coordinates — while every constraint keeps holding exactly. Goals
// are soft: an unreachable target is not an error, the geometry settles at the
// closest feasible configuration. Pass several WithGoal options to target
// several points in one solve. A goal is transient — it exists only for that
// Solve call, is invisible to DOF/redundancy analysis, and never serializes.
// A goal on a fixed point is legal and inert.
//
// One goal per pointer-move event is the drag interaction: solves are
// warm-started from the current geometry, so repeated goal solves track a
// moving target cheaply. See docs/goal-solve-design.md.
func WithGoal(p *Point, x, y float64) SolveOption {
	return solveOption{option.New(identGoal{}, goalTarget{p: p, tx: x, ty: y})}
}

// solveConfig holds the resolved solver options.
type solveConfig struct {
	maxIterations int
	tolerance     float64
	goals         []goalTarget
}

func defaultSolveConfig() solveConfig {
	return solveConfig{maxIterations: 200, tolerance: 1e-10}
}

// Solve runs the constraint solver, moving non-grounded geometry until all
// constraints are satisfied. Called with no options it uses sensible defaults;
// override individual settings with the With… helpers.
//
// Solve warm-starts from the current coordinates: the positions geometry was
// authored at (or moved to with [Point.MoveTo]) are the solver's initial
// guess, and Solve converges to a valid configuration near them. It does not
// guarantee the *nearest* one — when the constraints admit several discrete
// configurations (see "Orientation and sign conventions" in the package doc),
// the realized branch follows the solver's descent path from the seed. Use
// [Sketch.ProbeConfigurations] to search for alternative configurations, and
// pin the intended branch with a signed constraint when one exists.
//
// Solve returns [ErrNotConverged] (along with the partial [Result]) if the
// residuals cannot be driven below the tolerance within the iteration budget,
// which usually means the sketch is over-constrained or contradictory.
func (s *Sketch) Solve(options ...SolveOption) (*Result, error) {
	o := defaultSolveConfig()
	for _, opt := range options {
		switch opt.Ident().(type) {
		case identMaxIterations:
			o.maxIterations = option.MustGet[int](opt)
		case identTolerance:
			o.tolerance = option.MustGet[float64](opt)
		case identGoal:
			// Append — repeated WithGoal options accumulate.
			o.goals = append(o.goals, option.MustGet[goalTarget](opt))
		}
	}

	// Refresh any dimensions driven by parameter expressions before solving.
	if err := s.ApplyParameters(); err != nil {
		return &Result{}, err
	}

	free := s.freeVars()
	n := len(free)

	// Goal solves run two phases. Phase 1 minimizes the augmented system
	// [hard residuals | goal rows], which moves toward the targets but — this
	// is plain weighted least squares — leaves the hard constraints violated
	// by O(w²·pull) at the optimum of an unreachable goal. Phase 2 (the only
	// phase when there are no goals) then polishes on the hard residuals
	// alone, projecting the geometry back onto the constraint manifold; the
	// correction is tiny relative to the goal motion, so goal attainment is
	// preserved while constraints end up holding exactly.
	var iters int
	if len(o.goals) > 0 {
		aug := func(buf []float64) []float64 { return s.goalResiduals(buf, o.goals) }
		iters += s.lm(free, aug, o.maxIterations, o.tolerance)
	}
	iters += s.lm(free, s.residuals, o.maxIterations, o.tolerance)

	s.refreshDriven()

	res := &Result{Iterations: iters}
	// Convergence is judged on the hard constraints only: a goal pulling
	// toward an unreachable target is the expected outcome, not a failure.
	rh := s.residuals(nil)
	mh := len(rh)
	res.Residual = math.Sqrt(dot(rh, rh))
	res.Converged = res.Residual <= o.tolerance

	if mh == 0 {
		res.DOF = n
		return res, nil
	}

	rank := s.rank(free, mh)
	res.DOF = n - rank
	if res.DOF < 0 {
		res.DOF = 0
	}
	res.Redundant = mh - rank
	if res.Redundant < 0 {
		res.Redundant = 0
	}

	if !res.Converged {
		return res, ErrNotConverged
	}
	return res, nil
}

// lm runs the Levenberg–Marquardt loop on the residual vector produced by
// eval, mutating the sketch's free variables in place, and reports the outer
// iterations performed. It terminates when the residual norm reaches the
// tolerance, when no damped step improves the cost (a minimum — possibly with
// nonzero residual, e.g. an unreachable goal), or when the budget runs out.
func (s *Sketch) lm(free []int, eval func([]float64) []float64, maxIterations int, tolerance float64) int {
	n := len(free)
	r := eval(nil)
	m := len(r)
	if m == 0 {
		return 0
	}

	cost := dot(r, r) // sum of squared residuals
	lambda := 1e-3
	var iter int
	for iter = 0; iter < maxIterations; iter++ {
		if math.Sqrt(cost) <= tolerance {
			break
		}
		if n == 0 {
			break // nothing free to move
		}

		J := s.jacobian(free, m, eval)
		// Normal equations: A = JᵀJ, g = Jᵀr.
		A := make([][]float64, n)
		g := make([]float64, n)
		for i := 0; i < n; i++ {
			A[i] = make([]float64, n)
			for j := 0; j < n; j++ {
				var sum float64
				for k := 0; k < m; k++ {
					sum += J[k][i] * J[k][j]
				}
				A[i][j] = sum
			}
			var gs float64
			for k := 0; k < m; k++ {
				gs += J[k][i] * r[k]
			}
			g[i] = gs
		}

		// Absolute damping scale. Using λ·max(diag) rather than λ·A[i][i]
		// regularizes every direction by the same amount, which keeps the
		// step well behaved (minimum-norm) for rank-deficient / under-
		// constrained systems where some diagonal entries are tiny.
		maxDiag := 0.0
		for i := 0; i < n; i++ {
			if A[i][i] > maxDiag {
				maxDiag = A[i][i]
			}
		}
		if maxDiag == 0 {
			maxDiag = 1
		}

		// Inner loop: adapt the damping λ until a step reduces the cost.
		improved := false
		for try := 0; try < 40; try++ {
			mu := lambda * maxDiag
			damped := make([][]float64, n)
			rhs := make([]float64, n)
			for i := 0; i < n; i++ {
				damped[i] = make([]float64, n)
				copy(damped[i], A[i])
				damped[i][i] += mu + 1e-12 // Levenberg damping + numerical floor
				rhs[i] = -g[i]
			}
			delta, ok := solveLinear(damped, rhs)
			if !ok {
				lambda *= 10
				continue
			}
			// Apply the trial step.
			for j, vi := range free {
				s.vars[vi] += delta[j]
			}
			rNew := eval(nil)
			costNew := dot(rNew, rNew)
			if costNew < cost {
				cost = costNew
				r = rNew
				lambda *= 0.5
				improved = true
				break
			}
			// Reject: undo and increase damping.
			for j, vi := range free {
				s.vars[vi] -= delta[j]
			}
			lambda *= 10
			if lambda > 1e12 {
				break
			}
		}
		if !improved {
			break
		}
	}
	return iter
}

// DOF reports the remaining degrees of freedom of the sketch at its current
// configuration (0 means fully constrained). It does not move any geometry.
func (s *Sketch) DOF() int {
	free := s.freeVars()
	m := len(s.residuals(nil))
	if m == 0 {
		return len(free)
	}
	d := len(free) - s.rank(free, m)
	if d < 0 {
		return 0
	}
	return d
}

// RedundantConstraints identifies which constraints contribute redundant or
// conflicting equations at the current configuration (typically called after
// [Sketch.Solve], like [Sketch.DOF]). Constraints are examined in creation
// order: an equation that is linearly dependent on the equations of earlier
// constraints marks its constraint as redundant, so of two duplicates the
// later-added one is reported. A constraint whose equations touch no free
// variable (e.g. a dimension between fully grounded points) is also reported —
// it either holds trivially or conflicts, and removing it never frees
// geometry. Driven dimensions contribute no equations and never appear. The
// result is nil when no redundancy exists.
//
// To separate the redundant constraints from the conflicting ones, and to learn
// which earlier constraints each conflicting one fights, call [Sketch.Diagnose]
// or [Sketch.Verify].
func (s *Sketch) RedundantConstraints() []Constraint {
	flagged, _ := s.conflictAnalysis()
	return flagged
}

func (s *Sketch) freeVars() []int {
	idx := make([]int, 0, len(s.vars))
	for i := range s.vars {
		if !s.fixed[i] {
			idx = append(idx, i)
		}
	}
	return idx
}

// residuals evaluates every constraint into a fresh slice (reusing buf's
// backing array when possible). Driven (reference) dimensions contribute no
// residual — they measure the geometry instead of constraining it.
func (s *Sketch) residuals(buf []float64) []float64 {
	buf = buf[:0]
	for _, c := range s.cons {
		if d, ok := c.(Dimension); ok && d.Driven() {
			continue
		}
		buf = c.residual(buf)
	}
	return buf
}

// goalResiduals evaluates the augmented residual vector: every hard constraint
// followed by two weighted soft rows per goal. Used only inside Solve — goals
// are not constraints and must stay invisible to DOF/rank/redundancy analysis.
func (s *Sketch) goalResiduals(buf []float64, goals []goalTarget) []float64 {
	buf = s.residuals(buf)
	for _, g := range goals {
		buf = append(buf, goalWeight*(g.p.x()-g.tx), goalWeight*(g.p.y()-g.ty))
	}
	return buf
}

// refreshDriven updates every driven dimension's target to the value measured
// from the current geometry, expressed in the dimension's own unit. Called at
// the end of [Sketch.Solve] so driven dimensions report the solved geometry.
func (s *Sketch) refreshDriven() {
	for _, c := range s.cons {
		d, ok := c.(Dimension)
		if !ok || !d.Driven() {
			continue
		}
		// A dimension's first residual is measured − target (in base units),
		// so the measurement is recovered as residual + target.
		r := c.residual(nil)
		if len(r) == 0 {
			continue
		}
		v := units.FromBase(d.base()+r[0], d.Target().Unit())
		d.restore(v.Mag(), v.Unit())
	}
}

// jacobian computes the m×n Jacobian of the residual vector produced by eval
// w.r.t. the free variables using central finite differences. Hard-constraint
// analysis passes s.residuals; Solve passes its (possibly goal-augmented)
// evaluator.
func (s *Sketch) jacobian(free []int, m int, eval func([]float64) []float64) [][]float64 {
	n := len(free)
	J := make([][]float64, m)
	for i := range J {
		J[i] = make([]float64, n)
	}
	// Reuse two residual buffers across columns instead of allocating fresh
	// slices for every perturbed variable.
	rp := make([]float64, 0, m)
	rm := make([]float64, 0, m)
	for j, vi := range free {
		orig := s.vars[vi]
		h := 1e-7 * (1 + math.Abs(orig))
		s.vars[vi] = orig + h
		rp = eval(rp)
		s.vars[vi] = orig - h
		rm = eval(rm)
		s.vars[vi] = orig
		inv := 1.0 / (2 * h)
		for i := 0; i < m; i++ {
			J[i][j] = (rp[i] - rm[i]) * inv
		}
	}
	return J
}

// rankEps is the pivot magnitude below which a Jacobian column is treated as
// rank-deficient. The hard rank/DOF verdict turns on this threshold, so how close
// the deciding pivots sit to it is the rank-margin trust signal (rankAnalysis).
const rankEps = 1e-9

// rankAnalysis holds a rank estimate plus the pivot magnitudes that decided it —
// the smallest pivot accepted as rank-bearing and the largest column pivot
// rejected as below the threshold. These bound how fragile the rank/DOF verdict
// is to perturbation (see rankAnalysis.margin).
type rankAnalysis struct {
	rank             int
	minAcceptedPivot float64 // smallest accepted pivot (>= rankEps); +Inf when none accepted
	maxRejectedPivot float64 // largest rejected column pivot (< rankEps); 0 when none rejected
}

// margin reports the multiplicative distance of the closest pivot decision from
// rankEps: how many times above the threshold the smallest accepted pivot is, and
// how many times below it the largest rejected pivot is, taking the worse (closer)
// side. A large margin means the rank decision is far from the cutoff and so
// robust; a small one means a tiny perturbation could flip the rank (hence the
// DOF / redundancy verdict). It is a trust signal for THIS rank decision, not a
// unit-invariant condition number. A system with neither accepted nor rejected
// pivots (no free vars or no rows) is vacuously well-separated (+Inf).
func (ra rankAnalysis) margin() float64 {
	accepted := math.Inf(1)
	if !math.IsInf(ra.minAcceptedPivot, 1) {
		accepted = ra.minAcceptedPivot / rankEps
	}
	rejected := math.Inf(1)
	if ra.maxRejectedPivot > 0 {
		rejected = rankEps / ra.maxRejectedPivot
	}
	return math.Min(accepted, rejected)
}

// rank estimates the rank of the hard-constraint Jacobian at the current
// configuration via Gaussian elimination with partial pivoting.
func (s *Sketch) rank(free []int, m int) int {
	return s.rankOf(free, m, s.residuals)
}

// rankOf is rank generalized over the residual evaluator, so callers can rank
// augmented systems (e.g. [Sketch.CheckConstraint] appends a candidate
// constraint's rows to the hard residuals).
func (s *Sketch) rankOf(free []int, m int, eval func([]float64) []float64) int {
	return s.rankAnalysisOf(free, m, eval).rank
}

// rankAnalysisOf computes the rank and the deciding pivot magnitudes. Gaussian
// elimination with partial pivoting; a column whose best available pivot is below
// rankEps does not increase the rank (and its pivot is recorded as the largest
// rejected one).
func (s *Sketch) rankAnalysisOf(free []int, m int, eval func([]float64) []float64) rankAnalysis {
	J := s.jacobian(free, m, eval)
	n := len(free)
	ra := rankAnalysis{minAcceptedPivot: math.Inf(1)}
	row := 0
	for col := 0; col < n && row < m; col++ {
		// Find a pivot in this column at or below the current row.
		piv := row
		best := math.Abs(J[row][col])
		for r := row + 1; r < m; r++ {
			if v := math.Abs(J[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < rankEps {
			if best > ra.maxRejectedPivot {
				ra.maxRejectedPivot = best
			}
			continue
		}
		if best < ra.minAcceptedPivot {
			ra.minAcceptedPivot = best
		}
		J[row], J[piv] = J[piv], J[row]
		for r := 0; r < m; r++ {
			if r == row {
				continue
			}
			f := J[r][col] / J[row][col]
			if f == 0 {
				continue
			}
			for c := col; c < n; c++ {
				J[r][c] -= f * J[row][c]
			}
		}
		row++
	}
	ra.rank = row
	return ra
}

// rankMargin reports the rank-decision margin of the hard-constraint Jacobian at
// the current configuration (see rankAnalysis.margin), over the same free
// variables and residual rows DOF uses. It is +Inf when there are no residual
// rows (nothing to rank).
func (s *Sketch) rankMargin() float64 {
	free := s.freeVars()
	m := len(s.residuals(nil))
	if m == 0 {
		return math.Inf(1)
	}
	return s.rankAnalysisOf(free, m, s.residuals).margin()
}

// solveLinear solves A·x = b for a square matrix using Gaussian elimination
// with partial pivoting. A and b are not modified. The second return is false
// if A is singular.
func solveLinear(A [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	M := make([][]float64, n)
	for i := range M {
		M[i] = make([]float64, n+1)
		copy(M[i], A[i])
		M[i][n] = b[i]
	}
	for col := 0; col < n; col++ {
		piv := col
		best := math.Abs(M[col][col])
		for r := col + 1; r < n; r++ {
			if v := math.Abs(M[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < 1e-15 {
			return nil, false
		}
		M[col], M[piv] = M[piv], M[col]
		for r := col + 1; r < n; r++ {
			f := M[r][col] / M[col][col]
			for c := col; c <= n; c++ {
				M[r][c] -= f * M[col][c]
			}
		}
	}
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := M[i][n]
		for c := i + 1; c < n; c++ {
			sum -= M[i][c] * x[c]
		}
		x[i] = sum / M[i][i]
	}
	return x, true
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
