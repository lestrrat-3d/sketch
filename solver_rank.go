package sketch

import "math"

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
// (see non_finite.go): the second result is false — and the analysis is not run —
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
	L := s.lengthScale()
	rowKinds := s.residualRowKinds()
	colScale := s.condVarScales(L)
	matrix := newMatrixScratch(len(rowKinds), len(free))
	defer matrix.release()
	s.scaledJacobianFill(matrix.Rows, localJacobian, free, s.residuals, rowKinds, colScale, L)
	return rankAnalysisOfMatrix(matrix.Rows, len(free)), true
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
	c := cloneMatrixScratch(cj.A)
	defer c.release()
	return rankAnalysisOfMatrix(c.Rows, len(cj.free))
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
