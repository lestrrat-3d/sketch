package sketch

import (
	"context"
	"math"

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
		aug := func(buf []float64) []float64 { return s.goalResiduals(buf, o.goals) }
		di, err := s.lm(ctx, free, aug, o.maxIterations, o.tolerance)
		iters += di
		solveErr = err
	}
	if solveErr == nil {
		di, err := s.lm(ctx, free, s.residuals, o.maxIterations, o.tolerance)
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
// gradient, the damped trial matrix and its solveLinearInto scratch, and the
// residual buffers — so the loop and its trials mutate fixed-size buffers in
// place instead of allocating fresh ones every iteration/trial. Sized once from
// m (residual rows) and n (free variables), which are fixed for the life of one
// lm call.
type lmWorkspace struct {
	J, A, damped, M [][]float64
	// JT is J transposed into one flat m*n buffer, column-major: column j of J
	// occupies the contiguous run JT[j*m:(j+1)*m]. The normal equations read
	// each column end to end (see normalEquationsInto), which J's row-major
	// [][]float64 layout cannot serve — there a column walk strides across m
	// separately allocated rows. One transpose per Jacobian pays for n*(n+1)/2
	// column pairs plus n gradient dot products.
	JT            []float64
	g, rhs, delta []float64
	rp, rm, rNew  []float64
	r             []float64

	// The sparsity pattern of one Jacobian, in the two forms
	// normalEquationsSparseInto reads it. colRows holds, for each column i, the
	// ascending rows k with JT[i*m+k] != 0, in colRows[colStart[i]:colStart[i+1]];
	// that is the list the accumulation walks. colBits is the same information as
	// one bitmap per column, colWords uint64 words each, which answers "do these
	// two columns share a nonzero row?" in a word-at-a-time AND instead of a walk
	// — the question asked once per column PAIR, so it must not cost per row.
	// Both are rebuilt from scratch every iteration (see normalEquationsSparseInto
	// on why a pattern is never carried over) and both are sized for a fully dense
	// m×n Jacobian, so no rebuild allocates.
	colRows, colStart []int
	colBits           []uint64
	colWords          int
}

// alloc sizes every buffer but r for m residual rows and n free variables. It is
// called from inside the outer loop, past the termination checks, rather than
// before it: a solve that is already converged (or has nothing free to move)
// never builds a Jacobian, and would otherwise pay several m×n and n×n
// allocations to do nothing.
func (ws *lmWorkspace) alloc(m, n int) {
	ws.J = make([][]float64, m)
	ws.JT = make([]float64, m*n)
	ws.A = make([][]float64, n)
	ws.damped = make([][]float64, n)
	ws.M = make([][]float64, n)
	ws.g = make([]float64, n)
	ws.rhs = make([]float64, n)
	ws.delta = make([]float64, n)
	ws.rp = make([]float64, 0, m)
	ws.rm = make([]float64, 0, m)
	ws.rNew = make([]float64, 0, m)
	// Worst case is a fully dense Jacobian, so the pattern rebuild in
	// normalEquationsSparseInto only ever appends into capacity it already holds.
	ws.colRows = make([]int, 0, m*n)
	ws.colStart = make([]int, n+1)
	ws.colWords = (m + 63) / 64
	ws.colBits = make([]uint64, n*ws.colWords)
	for i := range ws.J {
		ws.J[i] = make([]float64, n)
	}
	for i := range ws.A {
		ws.A[i] = make([]float64, n)
		ws.damped[i] = make([]float64, n)
		ws.M[i] = make([]float64, n+1)
	}
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
func (s *Sketch) lm(ctx context.Context, free []int, eval func([]float64) []float64, maxIterations int, tolerance float64) (int, error) {
	n := len(free)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r := eval(nil)
	m := len(r)
	if m == 0 {
		return 0, nil
	}

	// eval(nil) already returned a fresh buffer; no need to copy it in. The rest
	// of the workspace is sized lazily below, once an iteration actually needs it.
	ws := &lmWorkspace{r: r}

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
		if ws.J == nil {
			ws.alloc(m, n)
		}

		s.jacobianInto(ws.J, free, m, eval, ws.rp, ws.rm)
		transposeInto(ws.JT, ws.J, m, n)
		normalEquationsSparseInto(ws, m, n)
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
			damped, rhs := ws.damped, ws.rhs
			for i := 0; i < n; i++ {
				copy(damped[i], A[i])
				damped[i][i] += mu + 1e-12 // Levenberg damping + numerical floor
				rhs[i] = -g[i]
			}
			if !solveLinearInto(ws.M, damped, rhs, ws.delta) {
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

// DOF reports the remaining degrees of freedom of the sketch at its current
// configuration (0 means fully constrained). It does not move any geometry.
//
// On non-finite geometry (a NaN or infinite point coordinate, entity shape
// variable, dimension target, or constraint-owned auxiliary variable — see
// [Sketch.nonFiniteVars]) the rank pass
// this method otherwise depends on cannot be trusted in either direction: a
// non-finite pivot is neither correctly selected nor correctly rejected by a
// plain partial-pivot comparison (see rankAnalysisOfMatrix), so it can read as
// either too many or too few degrees of freedom. DOF has no error return and
// so cannot refuse; it answers with MAXIMUM IGNORANCE instead — the sketch's
// TOTAL variable count, grounded variables included, as if neither a
// constraint nor a [Sketch.Fix] had ever been applied.
//
// The total count, never the free-variable count: [Sketch.freeVars] filters on
// the grounding flags, so on an all-grounded sketch it is empty and the
// free-variable count collapses onto 0 — exactly the value that reads as fully
// constrained, on exactly the fixture this screen exists for (a line with both
// endpoints grounded beside a grounded stray point at (NaN, NaN)). Grounding
// IS a constraint, so a value that counts it is not maximum ignorance. The
// total count cannot collapse that way: every sketch owns an origin
// contributing two variables, so it is always at least 2, and it stays a
// non-negative count inside this method's documented domain, so a caller doing
// arithmetic or comparison on it is never handed a sentinel it does not expect.
//
// This is a deliberate design choice, not an oversight: call [Sketch.Verify] to
// learn that the condition was found at all
// ([VerificationReport.NonFinitePoints] and its siblings).
func (s *Sketch) DOF() int {
	// Asked directly rather than only through the rank pass below: a sketch with
	// no residual rows never reaches that pass, so there is no second return to
	// carry the screen on that branch — and the free-variable count it would
	// return collapses to 0 on an all-grounded sketch, which is the reading this
	// screen exists to prevent.
	if s.hasNonFiniteVars() {
		return len(s.vars)
	}
	free := s.freeVars()
	m := len(s.residuals(nil))
	if m == 0 {
		return len(free)
	}
	rk, ok := s.rank(free, m)
	if !ok {
		return len(s.vars)
	}
	d := len(free) - rk
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
//
// On non-finite geometry (see [Sketch.DOF]'s doc comment for why) it answers
// with the same MAXIMUM-IGNORANCE value [Sketch.Diagnose] does: every
// constraint the dependency analysis could ever flag, so the flat list and the
// partition Diagnose refines it into keep naming one set.
func (s *Sketch) RedundantConstraints() []Constraint {
	flagged, _, analysed := s.conflictAnalysis()
	if !analysed {
		return s.unprovenConstraints()
	}
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

// transposeInto fills jt with the column-major transpose of the m×n row-major
// matrix j: jt[c*m+r] holds j[r][c], so column c of j lands in the contiguous
// run jt[c*m:(c+1)*m]. jt must hold at least m*n entries and j at least m rows
// of n columns; [lmWorkspace.alloc] sizes both from the same m and n.
//
// It only MOVES float64 values, so it introduces no arithmetic and cannot
// perturb a single bit of what the caller then computes from jt.
func transposeInto(jt []float64, j [][]float64, m, n int) {
	for r := 0; r < m; r++ {
		row := j[r]
		for c := 0; c < n; c++ {
			jt[c*m+r] = row[c]
		}
	}
}

// normalEquationsInto accumulates the normal equations of the least-squares
// step into the caller's buffers: a = JᵀJ (n×n) and g = Jᵀr (n), read from the
// column-major transpose jt that [transposeInto] produced and the residual
// vector r. a is fully overwritten and g likewise, so neither needs clearing
// between iterations.
//
// Every entry is a dot product of two Jacobian COLUMNS, and jt holds each
// column as one contiguous run — the reason the transpose is worth its own
// pass. Reading the columns out of the row-major J instead walks m separately
// allocated rows per entry, which is a cache miss per term rather than per line
// and is what dominated the solver profile.
//
// The arithmetic is deliberately unchanged from the row-major double loop this
// replaces, term for term and in the same ascending k: same two factors, same
// running sum from positive zero, no compensated summation, no skipped zero
// terms, no reassociation. Float addition is neither associative nor
// commutative, so any of those would move result bits; the equivalence is
// pinned bit for bit by TestNormalEquationsColumnLayoutMatchesReference.
//
// a is symmetric, so only its upper triangle is accumulated and each sum is
// mirrored into the lower one. IEEE multiplication is commutative and the
// order over k does not depend on i or j, so the mirrored entry is bit-
// identical to what a full double loop would have computed for (j, i).
//
// [Sketch.lm] no longer calls this directly: [normalEquationsSparseInto] does
// the same accumulation over the Jacobian's nonzeros and falls back here on
// non-finite input. This stays the definition of the result both must produce,
// and the "no skipped zero terms" rule above is a rule about THIS loop — the
// sparse kernel's doc comment says on what grounds it may skip.
func normalEquationsInto(a [][]float64, g []float64, jt []float64, r []float64, m, n int) {
	for i := 0; i < n; i++ {
		ci := jt[i*m : (i+1)*m]
		for j := i; j < n; j++ {
			cj := jt[j*m : (j+1)*m]
			var sum float64
			for k := 0; k < m; k++ {
				sum += ci[k] * cj[k]
			}
			a[i][j] = sum
			a[j][i] = sum
		}
		var gs float64
		for k := 0; k < m; k++ {
			gs += ci[k] * r[k]
		}
		g[i] = gs
	}
}

// normalEquationsSparseInto accumulates the same a = JᵀJ and g = Jᵀr that
// [normalEquationsInto] does, over the STRUCTURAL NONZEROS of each Jacobian
// column only, and produces bit-identical results on finite input. A sketch
// Jacobian is sparse — a constraint's residual row touches only the few
// variables of the geometry it references — so most of the products the dense
// kernel forms are exact zeros it spends a multiply and an add on anyway.
//
// The equivalence is exact, not approximate, and rests on two facts. Adding an
// exact zero to a float64 that is not negative zero returns that float64
// unchanged, for every finite value and for both infinities. And the running
// sum starts at positive zero and can never BECOME negative zero: round-to-
// nearest returns positive zero for every exact cancellation, and positive zero
// plus either signed zero is positive zero. So a term whose product is exactly
// zero contributes nothing to the sum it would have joined, and dropping it
// leaves every remaining term in its original ascending-k position. That is why
// this kernel may skip terms where the doc comment on normalEquationsInto
// forbids it: skipping is not a reassociation.
//
// A product is exactly zero only when one factor is zero and the OTHER IS
// FINITE — zero times infinity is NaN, which the dense kernel would propagate
// and this one would drop. Hence the finiteness guard: a NaN or infinity
// anywhere in jt or r sends the whole call to the dense kernel, whose result is
// then the one bit pattern both paths agree on. The `!= 0` tests treat negative
// zero as zero, which is correct — its products are exact zeros too.
//
// The pattern is rebuilt from scratch on every call. It is NOT carried over
// from the previous iteration: the Jacobian is a finite-difference
// approximation, so an entry that was exactly zero for one variable position
// can be nonzero at the next, and a stale pattern would silently drop a real
// term.
func normalEquationsSparseInto(ws *lmWorkspace, m, n int) {
	jt, r := ws.JT, ws.r
	for i := 0; i < m*n; i++ {
		if nonFinite(jt[i]) {
			normalEquationsInto(ws.A, ws.g, jt, r, m, n)
			return
		}
	}
	for k := 0; k < m; k++ {
		if nonFinite(r[k]) {
			normalEquationsInto(ws.A, ws.g, jt, r, m, n)
			return
		}
	}

	// Column i's nonzero rows, read off the contiguous column runs of jt.
	colRows, colStart := ws.colRows[:0], ws.colStart[:n+1]
	for i := 0; i < n; i++ {
		colStart[i] = len(colRows)
		ci := jt[i*m : (i+1)*m]
		for k := 0; k < m; k++ {
			if ci[k] != 0 {
				colRows = append(colRows, k)
			}
		}
	}
	colStart[n] = len(colRows)

	// The same pattern as one bitmap per column, so the "share a row?" question
	// below costs a handful of word ANDs rather than a walk over either column.
	words := ws.colWords
	colBits := ws.colBits[:n*words]
	clear(colBits)
	for i := 0; i < n; i++ {
		bits := colBits[i*words : (i+1)*words]
		for _, k := range colRows[colStart[i]:colStart[i+1]] {
			bits[k/64] |= 1 << (k % 64)
		}
	}

	for i := 0; i < n; i++ {
		ci := jt[i*m : (i+1)*m]
		rowsI := colRows[colStart[i]:colStart[i+1]]
		bi := colBits[i*words : (i+1)*words]

		for j := i; j < n; j++ {
			// Every slot this column owns is written on both sides of the
			// diagonal, whether or not the pair contributes anything: the mirror
			// a[j][i] is written nowhere else, since column j only ever reaches
			// a[j][c] and a[c][j] for c >= j. Skipping the write on a pair that
			// shares no row would leave the previous iteration's sum in place.
			bj := colBits[j*words : (j+1)*words]
			shares := false
			for w := range bi {
				if bi[w]&bj[w] != 0 {
					shares = true
					break
				}
			}
			if !shares {
				// Every term of this sum is an exact zero added to a sum that
				// started at positive zero, so the dense kernel writes positive
				// zero here too.
				ws.A[i][j] = 0
				ws.A[j][i] = 0
				continue
			}

			// Only column i's nonzero rows are walked, in ascending order. Where
			// column j is zero at one of them the product is an exact zero, so
			// including it is the identity this whole kernel rests on — testing
			// for it would cost more than the multiply it saves.
			//
			// A column with no zero at all takes the straight-through loop
			// instead: the same terms in the same order, read without the index
			// indirection. A real sketch produces such a column whenever one
			// variable is touched by every residual row.
			cj := jt[j*m : (j+1)*m]
			var sum float64
			if len(rowsI) == m {
				for k := 0; k < m; k++ {
					sum += ci[k] * cj[k]
				}
			} else {
				for _, k := range rowsI {
					sum += ci[k] * cj[k]
				}
			}
			ws.A[i][j] = sum
			ws.A[j][i] = sum
		}
		// The diagonal needs no special case: a column with any nonzero shares a
		// row with itself, and an all-zero column takes the positive-zero branch,
		// which is what the dense kernel computes for it.

		var gs float64
		for _, k := range rowsI {
			gs += ci[k] * r[k]
		}
		ws.g[i] = gs
	}
}

// rankZeroTol is the STRUCTURAL rank cutoff: a pivot of the NONDIMENSIONAL
// Jacobian A = Drow·J·Dcol (see scaledJacobian) below this is treated as zero, so
// its column adds no rank. Because A is dimensionless and scale/unit invariant,
// this cutoff is meaningful at any scale — the rank/DOF/redundancy/free-point
// verdicts no longer move with the geometry's size or units. It is DISTINCT from
// the conditioning trust gate (conditioningGate): "structurally rank-deficient"
// (a true null direction) and "full-rank but numerically fragile" (a tiny but
// nonzero singular value) are different questions, so a DOF-0 sketch can still be
// untrustworthy by conditioning.
const rankZeroTol = 1e-9

// rankAnalysis holds a rank estimate plus the pivot magnitudes that decided it —
// the smallest pivot accepted as rank-bearing and the largest column pivot
// rejected as below the threshold (all on the nondimensional A). These bound how
// fragile the rank/DOF verdict is to perturbation (see rankAnalysis.margin).
type rankAnalysis struct {
	rank             int
	minAcceptedPivot float64 // smallest accepted pivot (>= rankZeroTol); +Inf when none accepted
	maxRejectedPivot float64 // largest rejected column pivot (< rankZeroTol); 0 when none rejected
}

// margin reports the multiplicative distance of the closest pivot decision from
// rankZeroTol: how many times above the threshold the smallest accepted pivot is,
// and how many times below it the largest rejected pivot is, taking the worse
// (closer) side. A large margin means the structural rank decision is far from the
// cutoff and so robust; a small one means a tiny perturbation could flip the rank
// (hence the DOF / redundancy verdict). Computed on the nondimensional A, it is now
// scale-invariant. A system with neither accepted nor rejected pivots (no free
// vars or no rows) is vacuously well-separated (+Inf).
func (ra rankAnalysis) margin() float64 {
	accepted := math.Inf(1)
	if !math.IsInf(ra.minAcceptedPivot, 1) {
		accepted = ra.minAcceptedPivot / rankZeroTol
	}
	rejected := math.Inf(1)
	if ra.maxRejectedPivot > 0 {
		rejected = rankZeroTol / ra.maxRejectedPivot
	}
	return math.Min(accepted, rejected)
}

// rank estimates the rank of the constraint Jacobian at the current configuration
// (scale-invariant: Gaussian elimination on the nondimensional A). m is the row
// count, unused beyond documenting the caller's residual size.
//
// The second result is false when this sketch's geometry is non-finite, in which
// case the rank is not returned at all: see [Sketch.committedRankAnalysis].
func (s *Sketch) rank(free []int, m int) (int, bool) {
	ra, ok := s.committedRankAnalysis(free)
	return ra.rank, ok
}

// committedRankAnalysis runs the scale-invariant rank analysis over the committed
// constraint rows (residuals(), driven dims skipped).
//
// It is one of the three primitives that CARRY the non-finite-geometry screen
// (see nonfinite.go): the second result is false — and the analysis is not run —
// when [Sketch.hasNonFiniteVars] holds, so a caller cannot read a rank built from
// a poisoned Jacobian without handling the refusal. No rank from such a matrix is
// trustworthy in either direction: a non-finite pivot is neither correctly
// selected nor correctly rejected by a plain partial-pivot comparison, so it
// reads as too many or too few degrees of freedom depending only on where it
// lands.
func (s *Sketch) committedRankAnalysis(free []int) (rankAnalysis, bool) {
	if s.hasNonFiniteVars() {
		return rankAnalysis{}, false
	}
	cj := committedJacobian{free: free, A: s.committedScaledJacobian(free)}
	return s.rankAnalysisOn(cj), true
}

// rankAnalysisOn runs the same partial-pivot elimination as
// [Sketch.committedRankAnalysis] over a prebuilt [committedJacobian] — the core
// [Sketch.Verify] calls directly so its rank/DOF and RankMargin fields share
// ONE Jacobian build with the conditioning, conflict and free-point analyses of
// the same call, instead of each rebuilding it (see committedJacobian's doc
// comment in conditioning.go). The matrix is cloned first because elimination
// mutates it in place, and cj.A may still be read by another consumer of the
// same committedJacobian after this call returns.
func (s *Sketch) rankAnalysisOn(cj committedJacobian) rankAnalysis {
	return rankAnalysisOfMatrix(cloneMatrix(cj.A), len(cj.free))
}

// rankAnalysisOf computes the rank and the deciding pivot magnitudes on the
// NONDIMENSIONAL Jacobian A = Drow·J·Dcol (scaledJacobian), so the structural
// rankZeroTol cutoff is scale/unit invariant. eval produces the residual rows,
// rowKinds classifies each, colScale gives each variable's column scale, L the
// length scale. Gaussian elimination with partial pivoting; a column whose best
// available pivot is below rankZeroTol does not increase the rank (and its pivot
// is recorded as the largest rejected one). Generalized over (eval, rowKinds,
// colScale) so [Sketch.CheckConstraint] can rank an augmented, candidate-aware
// system.
func (s *Sketch) rankAnalysisOf(free []int, eval func([]float64) []float64, rowKinds []rowKind, colScale []float64, L float64, extraPos ...[2]int) rankAnalysis {
	return rankAnalysisOfMatrix(s.scaledJacobian(free, eval, rowKinds, colScale, L, extraPos...), len(free))
}

// pivotAbs is the ABS used by the partial-pivot search in [rankAnalysisOfMatrix]
// and [Sketch.movableVars] — the two elimination loops that decide the
// rank/DOF/free-point verdicts. It is DEFENCE IN DEPTH, a hard stop against a
// non-finite (NaN or infinite) matrix entry, behind [Sketch.nonFiniteVars]
// screening the geometry those matrices are built from before either loop is
// ever reached from a public entry point (Verify, CheckConstraint, DOF,
// FreePoints, Diagnose all check it first) — it does not make the RESULT of an
// already-poisoned matrix meaningful, only keeps the two pivot comparisons
// from silently picking the wrong branch should one ever be reached
// unscreened.
//
// math.Abs alone lets a non-finite entry decide either comparison the wrong
// way: "v > best" is false whenever either operand is NaN, so a poisoned SEED
// value at (row,col) — which starts out as best — blocks every legitimate
// finite pivot below it in the same column from ever being selected (an
// UNDERCOUNT: the column is wrongly rejected as rank-free even though a good
// pivot was available). And once a non-finite value IS selected as the
// current best (the seed, or nothing else in the column beat it), "best <
// rankZeroTol" is equally false against a NaN, so it sails through the
// rejection check and is ACCEPTED as a pivot (an OVERCOUNT, with NaN then
// eliminated into the rest of the matrix). Reporting 0 for a non-finite entry
// makes it read as the worst possible candidate on both comparisons: it can
// never win the "v > best" search over a finite value, and if 0 does end up
// selected (every candidate in the column non-finite) it is correctly
// rejected by "best < rankZeroTol".
func pivotAbs(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Abs(v)
}

// rankAnalysisOfMatrix runs the partial-pivot Gaussian elimination of
// [Sketch.rankAnalysisOf] on a prebuilt nondimensional Jacobian A over n free
// variables (columns). Split out so the committed path and the candidate-aware
// path share one elimination kernel while building A through different preludes.
func rankAnalysisOfMatrix(A [][]float64, n int) rankAnalysis {
	m := len(A)
	ra := rankAnalysis{minAcceptedPivot: math.Inf(1)}
	row := 0
	for col := 0; col < n && row < m; col++ {
		// Find a pivot in this column at or below the current row.
		piv := row
		best := pivotAbs(A[row][col])
		for r := row + 1; r < m; r++ {
			if v := pivotAbs(A[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < rankZeroTol {
			if best > ra.maxRejectedPivot {
				ra.maxRejectedPivot = best
			}
			continue
		}
		if best < ra.minAcceptedPivot {
			ra.minAcceptedPivot = best
		}
		A[row], A[piv] = A[piv], A[row]
		for r := 0; r < m; r++ {
			if r == row {
				continue
			}
			f := A[r][col] / A[row][col]
			if f == 0 {
				continue
			}
			for c := col; c < n; c++ {
				A[r][c] -= f * A[row][c]
			}
		}
		row++
	}
	ra.rank = row
	return ra
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
//   - The working copy must hold NO NEGATIVE ZERO. Subtracting an exact zero is
//     the identity on every float64 except -0, where the subtrahend's sign
//     decides: -0 − (+0) is -0, but -0 − (-0) is +0. Skipping would keep a -0
//     the update turns positive, and that sign reaches x through the back
//     substitution. Elimination can never CREATE a negative zero — a
//     subtraction yields one only from a -0 minuend, an exact cancellation
//     rounds to +0, and no nonzero difference of two float64s underflows (both
//     are integer multiples of 2⁻¹⁰⁷⁴, so their difference is too) — so the
//     single scan below, over the copy every later value descends from, settles
//     the question for the whole call. It is answered on the copy-in pass,
//     where the rows are in cache exactly once, and one negative zero anywhere
//     costs the call its shortcut rather than risking a sign. That is rare: it
//     needs a gradient component of exactly zero, which the profiled workload
//     produced in 10.3% of its calls.
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
		if !noNegZero {
			continue
		}
		for _, v := range row[:n+1] {
			if math.Float64bits(v) == negZeroBits {
				noNegZero = false
				break
			}
		}
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
			return false
		}
		M[col], M[piv] = M[piv], M[col]

		pivot := M[col][col : n+1]
		skipZero := noNegZero
		if skipZero {
			for _, v := range pivot {
				if nonFinite(v) {
					skipZero = false
					break
				}
			}
		}
		for r := col + 1; r < n; r++ {
			row := M[r][col : n+1]
			f := row[0] / pivot[0]
			if skipZero && f == 0 {
				continue
			}
			for c, v := range pivot {
				row[c] -= f * v
			}
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

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
