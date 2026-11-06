package sketch

import (
	"context"
	"math"
	"slices"
	"sync"

	"github.com/lestrrat-3d/units"
	"github.com/lestrrat-go/option/v3"
)

// Result reports the outcome of a [Sketch.Solve] call.
//
// When Solve returns a context error (cancellation or deadline), the DOF/rank
// analysis is skipped, so DOF and Redundant are set to -1 to mark them "not
// computed" — never trust them as 0 in that case. Iterations, Residual and
// Converged reflect whatever solving happened before the context ended: a
// cancellation caught mid-solve carries the partial result, but a context
// already done on entry (before any iteration ran) leaves them at their zero
// values — Iterations 0, Residual 0, Converged false. A 0 Residual on that
// early-exit path means "no residual was measured", not "the sketch is
// satisfied", so pair it with the returned error before reading it.
//
// DOF and Redundant are -1 for a second reason, and this one comes back with a
// NIL error: the sketch's own geometry holds a non-finite (NaN or infinite)
// point coordinate, entity shape variable, dimension target, or constraint-
// owned auxiliary variable (see [Sketch.nonFiniteVars]), so no rank the
// analysis could compute is trustworthy
// in either direction. Iterations, Residual and Converged still report what the
// solver measured. A caller gating on `res.DOF == 0 && res.Converged` must
// therefore treat -1 as a refusal, not as a small number: call [Sketch.Verify]
// to learn which geometry carries the condition.
type Result struct {
	Converged  bool    // every residual is within the tolerance
	Iterations int     // outer Levenberg–Marquardt iterations performed
	Residual   float64 // Euclidean norm of the final residual vector
	DOF        int     // remaining degrees of freedom (0 == fully constrained; -1 == not computed, see above)
	Redundant  int     // number of redundant/conflicting constraint equations (-1 == not computed)
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
//
// The ctx argument bounds the solve: its cancellation or deadline aborts the
// run. Solve checks it before every outer Levenberg–Marquardt iteration (i.e.
// before each Jacobian build), before each damping trial, and between its
// internal phases; when ctx is done it stops early and returns the partial
// [Result] together with a non-nil error that wraps ctx.Err() (so
// errors.Is(err, context.DeadlineExceeded) or context.Canceled matches). Pass
// context.Background() for an unbounded solve.
//
// The check is at iteration granularity, not mid-iteration: a Jacobian build or
// residual evaluation already in progress runs to completion before the next
// check, so a deadline caps solve time to within one outer iteration's work
// rather than instantly. Together with the step-count cap ([WithMaxIterations])
// this is the intended bound on solve *time* when a sketch may come from an
// untrusted source.
func (s *Sketch) Solve(ctx context.Context, options ...SolveOption) (*Result, error) {
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

	// Nothing has been mutated yet, so an already-cancelled context short-
	// circuits before any work. DOF/Redundant are -1 (not computed) per the
	// Result contract, since no analysis ran.
	if err := ctx.Err(); err != nil {
		return &Result{DOF: -1, Redundant: -1}, err
	}

	// Refresh any dimensions driven by parameter expressions before solving.
	if err := s.ApplyParameters(); err != nil {
		return &Result{DOF: -1, Redundant: -1}, err
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
	var solveErr error
	if len(o.goals) > 0 {
		// The augmented system carries goal rows that are not constraints, so it
		// has no residual plan: dense, the mode every arbitrary eval takes.
		aug := func(buf []float64) []float64 { return s.goalResiduals(buf, o.goals) }
		di, err := s.lm(ctx, free, aug, denseJacobian, o.maxIterations, o.tolerance)
		iters += di
		solveErr = err
	}
	if solveErr == nil {
		di, err := s.lm(ctx, free, s.residuals, localJacobian, o.maxIterations, o.tolerance)
		iters += di
		solveErr = err
	}

	s.refreshDriven()

	res := &Result{Iterations: iters}
	// Convergence is judged on the hard constraints only: a goal pulling
	// toward an unreachable target is the expected outcome, not a failure.
	rh := s.residuals(nil)
	mh := len(rh)
	res.Residual = math.Sqrt(dot(rh, rh))
	res.Converged = res.Residual <= o.tolerance

	// Also honor a cancellation that raced the final lm iteration: lm can return
	// a nil error (it converged or ran out of budget) at the same moment ctx is
	// cancelled, so re-check ctx here before spending the rank pass.
	if solveErr == nil {
		solveErr = ctx.Err()
	}

	// If the context ended mid-solve, report the partial result (iterations +
	// residual) and skip the expensive DOF/rank pass — it is a Jacobian rebuild
	// that the caller asked us to stop doing. DOF/Redundant are marked not
	// computed (-1) so a cancelled solve never reads as fully constrained.
	if solveErr != nil {
		res.DOF = -1
		res.Redundant = -1
		return res, solveErr
	}

	if mh == 0 {
		// No constraint rows means no rank pass, so the non-finite screen has no
		// second return to ride in on here; ask it directly, exactly as
		// [Sketch.DOF] does in the same branch. Without it n — the FREE variable
		// count — is 0 on an all-grounded sketch, and the result reads DOF 0,
		// converged, for geometry holding a NaN.
		if s.hasNonFiniteVars() {
			res.DOF, res.Redundant = -1, -1
			return res, nil
		}
		res.DOF = n
		return res, nil
	}

	rank, analysed := s.rank(free, mh)

	// The rank pass is itself a Jacobian rebuild — the one remaining chunk of
	// bounded work. A deadline that expires while it runs must still surface as a
	// context error rather than a normal DOF result, or the cancellation contract
	// leaks at exactly the phase the pre-pass check above meant to guard. The
	// just-computed rank is discarded so a context error keeps DOF/Redundant == -1,
	// consistent with the mid-solve cancellation path.
	if err := ctx.Err(); err != nil {
		res.DOF = -1
		res.Redundant = -1
		return res, err
	}

	// Non-finite geometry leaves no rank worth reporting in either direction (see
	// [Sketch.committedRankAnalysis]), so DOF and Redundant take the SAME
	// not-computed sentinel a cancelled solve already uses — this Result's own
	// documented refusal value, so no signature and no caller contract changes.
	// Reporting the poisoned numbers instead made the result byte-identical to the
	// finite sketch's, and a caller gating on `res.DOF == 0 && res.Converged` read
	// it as a clean pass. Convergence and Residual stay as measured: they are the
	// solver's own report on the rows it evaluated, not a verdict built from the
	// Jacobian, and a poisoned residual shows up in them on its own account.
	if !analysed {
		// The refusal is this Result's own not-computed sentinel WITH A NIL ERROR,
		// returned ahead of the convergence verdict below — the same shape the
		// no-residual-rows branch above already uses. Falling through to
		// ErrNotConverged instead would preempt it: poisoned geometry usually
		// fails to converge too, so the refusal became indistinguishable from an
		// ordinary contradictory sketch, and a caller matching on
		// [ErrNonFiniteGeometry] through [Sketch.Verify] saw a plain convergence
		// failure here. Converged, Residual and Iterations stay as measured — they
		// are the solver's own account of the rows it evaluated, not a verdict
		// built from the Jacobian.
		res.DOF, res.Redundant = -1, -1
		return res, nil
	}

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

// lmWorkspace holds every buffer one [Sketch.lm] call needs across its outer
// iterations and damping trials — the Jacobian, the normal-equation matrix and
// gradient, the damped trial matrix (the augmented elimination scratch), and the
// residual buffers — so the loop and its trials mutate fixed-size buffers in
// place instead of allocating fresh ones every iteration/trial. Sized once from
// m (residual rows) and n (free variables), which are fixed for the life of one
// lm call.
type lmWorkspace struct {
	J, A, M [][]float64
	// JT is J transposed into one flat m*n buffer, column-major: column j of J
	// occupies the contiguous run JT[j*m:(j+1)*m]. The normal equations read
	// each column end to end (see normalEquationsInto), which J's row-major
	// [][]float64 layout cannot serve — there a column walk strides across m
	// separately allocated rows. One transpose per Jacobian pays for n*(n+1)/2
	// column pairs plus n gradient dot products.
	JT           []float64
	g, delta     []float64
	rp, rm, rNew []float64
	r            []float64

	// The sparsity pattern of one Jacobian, in the two forms
	// normalEquationsSparseInto reads it. colRows holds, for each column i, the
	// ascending rows k with JT[i*m+k] != 0, in colRows[colStart[i]:colStart[i+1]];
	// that is the list the accumulation walks. colBits is the same information as
	// one bitmap per column, colWords uint64 words each, which answers "do these
	// two columns share a nonzero row?" in a word-at-a-time AND instead of a walk
	// — the question asked once per column PAIR, so it must not cost per row.
	// Both are rebuilt from scratch every iteration (see normalEquationsSparseInto
	// on why a pattern is never carried over); colRows grows to the densest
	// pattern seen and is kept.
	colRows, colStart []int
	colBits           []uint64
	colWords          int
	nz                []int

	// structRows/structStart/structBits describe the structural superset of
	// each Jacobian column for a successful local residual plan. They are built
	// once per lm call and remain separate from the numeric-pattern buffers so
	// a refused local Jacobian cannot overwrite them. structPairCols/Start hold
	// the ascending supported upper-triangle column pairs for that superset.
	structRows, structStart []int
	structBits              []uint64
	structPairCols          []int
	structPairStart         []int
	structDensity           int
	structuralReady         bool

	// The slabs the J, A and M row views are cut from.
	jBack, aBack, mBack []float64
}

// lmPool recycles lmWorkspace values across [Sketch.lm] calls, so a sequence of
// solves reuses the same Jacobian, normal-equation and elimination slabs instead
// of allocating them afresh. Reuse is sound because every buffer is either fully
// overwritten or cleared before it is read each iteration: J is cleared by
// jacobianLocalInto or written whole by jacobianInto, JT by transposeInto, A and
// g by the normal-equation kernels, M row by row before each elimination, and
// the pattern buffers by the pattern rebuild.
var lmPool sync.Pool

// getLMWorkspace returns a workspace with reusable storage for one lm call.
func getLMWorkspace() *lmWorkspace {
	ws, _ := lmPool.Get().(*lmWorkspace)
	if ws == nil {
		ws = &lmWorkspace{}
	}
	return ws
}

// release returns ws to the pool. Its residual vector and every matrix row view
// alias storage that stays with the workspace.
func (ws *lmWorkspace) release() {
	ws.r = ws.r[:0]
	ws.rNew = ws.rNew[:0]
	lmPool.Put(ws)
}

// growF64 returns buf resliced to length k, reallocating only when its capacity
// is too small. The contents are unspecified.
func growF64(buf []float64, k int) []float64 {
	if cap(buf) >= k {
		return buf[:k]
	}
	return make([]float64, k)
}

// rowViews points rows at the consecutive width-wide runs of slab. The rows are
// only ever indexed, never resliced past their length.
func rowViews(rows [][]float64, slab []float64, count, width int) [][]float64 {
	rows = slices.Grow(rows[:0], count)[:count]
	for i := range rows {
		rows[i] = slab[i*width : (i+1)*width : (i+1)*width]
	}
	return rows
}

// alloc sizes every buffer but r for m residual rows and n free variables. It is
// called from inside the outer loop, past the termination checks, rather than
// before it: a solve that is already converged (or has nothing free to move)
// never builds a Jacobian, and would otherwise pay several m×n and n×n
// allocations to do nothing. The buffers are those the workspace already holds
// when they are large enough.
func (ws *lmWorkspace) alloc(m, n int) {
	ws.structuralReady = false
	ws.jBack = growF64(ws.jBack, m*n)
	ws.aBack = growF64(ws.aBack, n*n)
	ws.mBack = growF64(ws.mBack, n*(n+1))
	ws.J = rowViews(ws.J, ws.jBack, m, n)
	ws.A = rowViews(ws.A, ws.aBack, n, n)
	ws.M = rowViews(ws.M, ws.mBack, n, n+1)
	ws.JT = growF64(ws.JT, m*n)
	ws.g = growF64(ws.g, n)
	ws.delta = growF64(ws.delta, n)
	ws.rp = growF64(ws.rp, m)[:0]
	ws.rm = growF64(ws.rm, m)[:0]
	ws.rNew = growF64(ws.rNew, m)[:0]
	if cap(ws.nz) < n+1 {
		ws.nz = make([]int, 0, n+1)
	}
	// A Jacobian is sparse, so the pattern starts small and grows by append; the
	// grown buffer is kept for the next rebuild.
	if cap(ws.colRows) < 4*m {
		ws.colRows = make([]int, 0, 4*m)
	}
	if cap(ws.colStart) < n+1 {
		ws.colStart = make([]int, n+1)
	}
	ws.colStart = ws.colStart[:n+1]
	ws.colWords = (m + 63) / 64
	if cap(ws.colBits) < n*ws.colWords {
		ws.colBits = make([]uint64, n*ws.colWords)
	}
	ws.colBits = ws.colBits[:n*ws.colWords]
	if cap(ws.structStart) < n+1 {
		ws.structStart = make([]int, n+1)
	}
	ws.structStart = ws.structStart[:n+1]
	if cap(ws.structBits) < n*ws.colWords {
		ws.structBits = make([]uint64, n*ws.colWords)
	}
	ws.structBits = ws.structBits[:n*ws.colWords]
	if cap(ws.structPairStart) < n+1 {
		ws.structPairStart = make([]int, n+1)
	}
	ws.structPairStart = ws.structPairStart[:n+1]
	ws.structPairCols = ws.structPairCols[:0]
}

// lm runs the Levenberg–Marquardt loop on the residual vector produced by
// eval, mutating the sketch's free variables in place, and reports the outer
// iterations performed and any context error. It terminates when the residual
// norm reaches the tolerance, when no damped step improves the cost (a minimum —
// possibly with nonzero residual, e.g. an unreachable goal), or when the budget
// runs out (returning a nil error), or early when ctx is cancelled (returning
// ctx.Err()). ctx is checked before the initial residual evaluation, once per
// outer iteration before the (expensive) Jacobian build, and at the top of the
// damping trial loop — always at a point where no unaccepted trial step is
// applied, so the geometry is left at the last accepted configuration.
//
// mode says whether this call may build a residual plan and take the local
// Jacobian (jacobian.go). It is a property of the CALLER's eval, so the caller
// states it: localJacobian is legal only when eval is [Sketch.residuals] over
// this sketch's committed constraints. denseJacobian, the zero value, is always
// legal and is what an arbitrary eval gets. The plan itself may still refuse —
// on a dependency it cannot determine, on a row count that moved, on non-finite
// residuals — and every refusal falls through to the dense build, so the mode
// selects an optimization, never a different result.
func (s *Sketch) lm(ctx context.Context, free []int, eval func([]float64) []float64, mode jacobianMode, maxIterations int, tolerance float64) (int, error) {
	return s.lmPlanned(ctx, free, eval, mode, nil, maxIterations, tolerance)
}

// lmPlanned is [Sketch.lm] with a caller-held residual plan. A non-nil shared
// plan, built by [Sketch.newResidualPlan] over this sketch's committed
// constraints, replaces the per-call build, so a caller that runs many solves
// over one unchanging constraint list pays for the plan once. It is only
// honoured in localJacobian mode, and jacobianLocalInto still refuses it
// wherever its row inventory no longer matches, so the result is bit-identical
// to the per-call build. A plan carries scratch buffers: one lm call at a time.
func (s *Sketch) lmPlanned(ctx context.Context, free []int, eval func([]float64) []float64, mode jacobianMode, shared *residualPlan, maxIterations int, tolerance float64) (int, error) {
	n := len(free)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	ws := getLMWorkspace()
	defer ws.release()
	r := eval(ws.r[:0])
	ws.r = r
	m := len(r)
	if m == 0 {
		return 0, nil
	}

	// The initial evaluation reuses the workspace's residual buffer. The rest of
	// the workspace is sized lazily below, once an iteration actually needs it.
	var plan *residualPlan
	sized := false

	cost := dot(ws.r, ws.r) // sum of squared residuals
	lambda := 1e-3
	var iter int
	for iter = 0; iter < maxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return iter, err
		}
		if math.Sqrt(cost) <= tolerance {
			break
		}
		if n == 0 {
			break // nothing free to move
		}
		if !sized {
			sized = true
			ws.alloc(m, n)
			// Built here rather than before the loop for the same reason the
			// workspace is: a solve that is already converged, or has nothing
			// free to move, never builds a Jacobian and must not pay for a plan
			// it will not use. Built ONCE per lm call — the constraint list and
			// the variable indices cannot change while the loop runs — and a
			// refusal is remembered by leaving plan nil rather than retried.
			switch {
			case mode != localJacobian:
			case shared != nil:
				plan = shared
			default:
				plan, _ = s.newResidualPlan(m)
			}
			if plan != nil && plan.m == m {
				ws.structuralReady = buildStructuralSupport(ws, plan, free, m, n)
			}
		}

		localBuilt := s.jacobianLocalInto(ws.J, free, m, plan, ws.r)
		if !localBuilt {
			s.jacobianInto(ws.J, free, m, eval, ws.rp, ws.rm)
		}
		transposeInto(ws.JT, ws.J, m, n)
		if localBuilt && ws.structuralReady {
			normalEquationsStructuralInto(ws, m, n)
		} else {
			normalEquationsSparseInto(ws, m, n)
		}
		A, g := ws.A, ws.g

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
			// Safe cancellation point: no trial step is applied yet this pass
			// (a rejected step was already reverted below), so the geometry is
			// at the last accepted configuration. This bounds the damping loop,
			// whose 40 trials each cost a full residual evaluation.
			if err := ctx.Err(); err != nil {
				return iter, err
			}
			mu := lambda * maxDiag
			noNegZero := true
			for i := 0; i < n; i++ {
				row := ws.M[i]
				copy(row, A[i])
				row[i] += mu + 1e-12 // Levenberg damping + numerical floor
				row[n] = -g[i]
				if noNegZero && hasNegZero(row[:n]) {
					noNegZero = false
				}
			}
			if !eliminateInto(ws.M, n, noNegZero, ws.delta, ws.nz) {
				lambda *= 10
				continue
			}
			delta := ws.delta
			// Apply the trial step.
			for j, vi := range free {
				s.vars[vi] += delta[j]
			}
			rNew := eval(ws.rNew[:0])
			costNew := dot(rNew, rNew)
			if costNew < cost {
				cost = costNew
				// Swap, never alias: rNew becomes the accepted r, and the
				// previous r becomes the scratch buffer the next trial's eval
				// writes into.
				ws.r, ws.rNew = rNew, ws.r
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
	return iter, nil
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

// jacobianInto fills the prebuilt m×n matrix J with the Jacobian of the
// residual vector produced by eval w.r.t. the free variables, using central
// finite differences. rp and rm are scratch residual buffers (any length,
// reused via their backing array across every perturbed variable); passing
// them in lets a caller that evaluates many Jacobians in a loop (the
// [lmWorkspace] case, [Sketch.lm]) reuse the same two buffers instead of
// allocating a pair per call.
func (s *Sketch) jacobianInto(J [][]float64, free []int, m int, eval func([]float64) []float64, rp, rm []float64) {
	for j, vi := range free {
		orig := s.vars[vi]
		h := 1e-7 * (1 + math.Abs(orig))
		s.vars[vi] = orig + h
		rp = eval(rp[:0])
		s.vars[vi] = orig - h
		rm = eval(rm[:0])
		s.vars[vi] = orig
		inv := 1.0 / (2 * h)
		for i := 0; i < m; i++ {
			J[i][j] = (rp[i] - rm[i]) * inv
		}
	}
}

// solveLinear solves A·x = b for a square matrix using Gaussian elimination
// with partial pivoting. A and b are not modified. The second return is false
// if A is singular. A thin wrapper over [solveLinearInto] that allocates its
// own scratch matrix and result slice; used directly by [rowCombo]
// (diagnose.go), which solves a small system once rather than in a loop.
func solveLinear(A [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	M := make([][]float64, n)
	for i := range M {
		M[i] = make([]float64, n+1)
	}
	x := make([]float64, n)
	if !solveLinearInto(M, A, b, x) {
		return nil, false
	}
	return x, true
}

// negZeroBits is the IEEE-754 bit pattern of negative zero. The question below
// is asked against it rather than with `v == 0 && math.Signbit(v)` because ==
// reports the two zeros as equal, and their sign bit is exactly what is at
// stake.
const negZeroBits = 1 << 63

// solveLinearInto solves A·x = b for a square matrix using Gaussian
// elimination with partial pivoting, writing the result into x and using M (an
// n×(n+1) scratch matrix) as its augmented working copy — the body
// [solveLinear] used to allocate fresh every call. A and b are not modified;
// M's rows may be reordered by pivoting, but every element is overwritten from
// A/b before elimination reads it, so a caller reusing M across calls (the
// [lmWorkspace] case) needs no reset between them. Returns false if A is
// singular, leaving x unmodified.
//
// # The zero-multiplier shortcut
//
// The systems this kernel meets are mostly zeros. [Sketch.lm] hands it JᵀJ
// damped on the diagonal, and a sketch Jacobian is sparse — a constraint's
// residual row touches only the few variables of the geometry it references —
// while rowCombo (diagnose.go) hands it an upper-triangular Gram matrix. In the
// gear-profile workload this kernel was profiled on, 44.8% of the elimination
// multipliers were exactly zero, and the row update each of them scales changes
// nothing. Skipping such a row skips its whole memory traffic too, which is
// most of what it costs.
//
// The skip must be BIT-IDENTICAL to the update it replaces, not merely equal to
// within an ulp: the LM step this feeds decides which configuration a solve,
// and every [Sketch.ProbeConfigurations] restart, lands in. The update is
// `M[r][c] -= f*M[col][c]`; with f exactly ±0 it can only ever leave M[r][c]
// alone or flip a zero's sign, and two guards rule out each of the two ways it
// could do more:
//
//   - The pivot row's suffix must be FINITE at this pivot. Zero times an
//     infinity (or a NaN) is NaN, which the plain update writes into the target
//     row and a skip would drop. This is a property of the pivot ROW, so it is
//     answered by one scan of the suffix the row updates then read repeatedly —
//     once per pivot, never once per row. It cannot be hoisted out of the pivot
//     loop either: a row that started finite can overflow to an infinity during
//     elimination.
//   - The working copy's MATRIX part must hold NO NEGATIVE ZERO. Subtracting an
//     exact zero is the identity on every float64 except -0, where the
//     subtrahend's sign decides: -0 − (+0) is -0, but -0 − (-0) is +0. Skipping
//     would keep a -0 the update turns positive, and that sign reaches x through
//     the back substitution. Elimination can never CREATE a negative zero — a
//     subtraction yields one only from a -0 minuend, an exact cancellation
//     rounds to +0, and no nonzero difference of two float64s underflows (both
//     are integer multiples of 2⁻¹⁰⁷⁴, so their difference is too) — so the
//     single scan on the copy-in pass, over the copy every later value descends
//     from, settles the question for the whole call. One negative zero in the
//     matrix costs the call its shortcut rather than risking a sign.
//
// The right-hand side column is outside that scan. A zero gradient component
// makes it a negative zero without touching the matrix (the profiled workload
// produced one in 10.3% of its calls), and no matrix entry ever reads it, so the
// shortcut leaves the matrix part to the zero-skipping update and applies the
// plain `row[n] -= f*pivot[n]` to the right-hand side of every row, skipped or
// not. That keeps the right-hand side bit-identical whatever its signs and
// non-finite values are.
//
// The test is deliberately EXACT: a zero multiplier is `f == 0`, never a
// small-magnitude test. A tiny nonzero f still changes the row, and an epsilon
// would silently substitute a different matrix for the one the caller passed.
// Note that `f == 0` also covers a nonzero numerator whose quotient UNDERFLOWED
// to zero; its products are exact zeros too, so it is equally safe.
//
// The row updates read their two rows through slices taken once per row rather
// than indexing M[r] and M[col] per element. That is the same arithmetic on the
// same values in the same ascending order — M[col] is never written while the
// rows below it are updated, and no two rows of a scratch matrix share a
// backing array — with the redundant slice-header loads and bounds checks
// lifted out of the innermost loop. On its own that rewrite measured as noise;
// paired with the skip it halved the kernel again, because a shortened loop
// pays the per-row overhead far more often.
func solveLinearInto(M [][]float64, A [][]float64, b, x []float64) bool {
	n := len(b)
	noNegZero := true
	for i := 0; i < n; i++ {
		row := M[i]
		copy(row, A[i])
		row[n] = b[i]
		if noNegZero && hasNegZero(row[:n]) {
			noNegZero = false
		}
	}
	// The nonzero-column scratch lives on the stack for the sizes a sketch
	// meets, so the kernel allocates nothing on a caller-supplied M. A larger
	// system gets it from the heap inside eliminateInto.
	var stack [256]int
	return eliminateInto(M, n, noNegZero, x, stack[:0])
}

// hasNegZero reports whether any entry of row is negative zero.
func hasNegZero(row []float64) bool {
	for _, v := range row {
		if math.Float64bits(v) == negZeroBits {
			return true
		}
	}
	return false
}

// eliminateInto is the elimination and back substitution of [solveLinearInto]
// over an already-filled n×(n+1) augmented matrix M. noNegZero says that no
// entry of M's n×n matrix part is negative zero, which the zero-multiplier shortcut needs (see
// [solveLinearInto]). A caller that builds M itself ([Sketch.lm]) saves the
// intermediate copy; it must report noNegZero exactly as the copy-in pass of
// solveLinearInto would.
func eliminateInto(M [][]float64, n int, noNegZero bool, x []float64, nz []int) bool {
	if cap(nz) < n+1 {
		nz = make([]int, 0, n+1)
	}
	nz = nz[:0]
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
			return false
		}
		M[col], M[piv] = M[piv], M[col]

		pivot := M[col][col : n+1]
		last := len(pivot) - 1
		skipZero := noNegZero
		nz = nz[:0]
		if skipZero {
			for c, v := range pivot[:last] {
				if nonFinite(v) {
					skipZero = false
					break
				}
				if v != 0 {
					nz = append(nz, c)
				}
			}
		}
		// The matrix columns of a pivot row that hold a zero are skipped: they
		// would subtract f*0, an exact ±0, from a finite entry that is never -0
		// (skipZero), which changes nothing. The multiplier must be finite for that
		// product to be a zero. The right-hand side column is a separate case: a
		// zero gradient component makes it a negative zero without touching the
		// matrix, and nothing in the matrix ever reads it, so it is updated for
		// every row, skipped or not, exactly as the plain update would.
		for r := col + 1; r < n; r++ {
			row := M[r][col : n+1]
			f := row[0] / pivot[0]
			if skipZero && !nonFinite(f) {
				if f != 0 {
					subtractScaledAt(row, pivot, nz, f)
				}
				row[last] -= f * pivot[last]
				continue
			}
			subtractScaled(row, pivot, f)
		}
	}
	for i := n - 1; i >= 0; i-- {
		sum := M[i][n]
		for c := i + 1; c < n; c++ {
			sum -= M[i][c] * x[c]
		}
		x[i] = sum / M[i][i]
	}
	return true
}

// subtractScaledAt does row[c] -= f*pivot[c] for the listed columns only.
func subtractScaledAt(row, pivot []float64, cols []int, f float64) {
	for _, c := range cols {
		row[c] -= f * pivot[c]
	}
}

// subtractScaled does row[c] -= f*pivot[c] for every column of pivot.
func subtractScaled(row, pivot []float64, f float64) {
	row = row[:len(pivot)]
	for c, v := range pivot {
		row[c] -= f * v
	}
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
