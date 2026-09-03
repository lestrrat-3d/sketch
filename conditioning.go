package sketch

import (
	"math"
	"slices"
)

// Scale-invariant conditioning gate.
//
// The rank/DOF verdict (solver.go) turns on a hard pivot threshold against the
// RAW constraint Jacobian, whose entries mix physical units — length-residual
// rows vs dimensionless (sin/cos) rows, and length-variable columns (point
// coordinates, radii) vs dimensionless columns (angles, slacks). Its margin
// (`RankMargin`) therefore moves with the sketch's scale and units, so it cannot
// gate trust. This file builds a physically NONDIMENSIONAL Jacobian and reports a
// scale- and unit-invariant near-singularity measure that CAN gate
// [VerificationReport.Trustworthy].
//
// A = Drow · J · Dcol, where (with L the bounding-box diagonal):
//   - Dcol scales each length-kind variable column by L (point coordinates, the
//     entity shape variables varKinds reports as varRadius — a circle's radius
//     being one example — and the conic-tangency contact-witness coordinates)
//     and leaves every dimensionless column at 1 (the shape variables varKinds
//     reports as varAngle or varDimensionless, and every slack /
//     spline-parameter aux).
//   - Drow scales each length-kind residual row by 1/L and leaves dimensionless
//     rows at 1.
// Every entry of A is then dimensionless and invariant under a uniform rescale of
// the geometry (a length-row/length-col entry picks up L·(1/L)=1; a
// dimensionless-row/length-col entry is already scale-free; etc.). The measure is
//
//   Conditioning = σ_min(A) / σ_max(A)
//
// the smallest singular value relative to the largest, computed on A itself by
// Householder bidiagonalization plus Sturm bisection for just the two extremes
// (see [singularValueExtremes]; never via AᵀA, which would square the condition
// number into floating-point noise). It is evaluated only for an otherwise fully-constrained
// trust candidate (DOF 0); an under-constrained sketch is genuinely singular by
// its free DOF, a separate already-reported verdict, so its conditioning is left
// not-applicable (+Inf).

// condTrustBase is the floor of the conditioning trust threshold: the
// finite-difference noise floor. Central differences at condFDStep give
// derivative noise in the 1e-9..1e-8 neighborhood, so 1e-6 leaves a comfortable
// buffer. The effective threshold ([conditioningGate]) is raised above this in
// proportion to √tolerance.
const condTrustBase = 1e-6

// condSlackFactor sets the tolerance-derived term of the conditioning threshold.
// A slack-encoded inequality (g − w² = 0) resting at its ACTIVE boundary has
// g ≈ 0, so the solve only pins w to ≈√tolerance; the slack variable's column is
// then [0,…,−2w,…,0]ᵀ with norm 2w ≤ 2·√tolerance, which upper-bounds σ_min. The
// gate must sit above that floor or a near-singular active-constraint system
// slips through, so the threshold carries a 4·√tolerance term (factor > 2 for
// margin). Without it the gate would be unsound at the default tolerance (a
// boundary slack gives σ_min ≈ 1e-5 > the 1e-6 base).
const condSlackFactor = 4.0

// conditioningGate is the effective trust threshold for [VerificationReport.Conditioning]
// at the given solve/verify tolerance: max(condTrustBase, condSlackFactor·√tol).
// It is tolerance-derived so a looser tolerance (which lets active slacks rest
// farther from their boundary) cannot slip a near-singular system past the gate.
func conditioningGate(tolerance float64) float64 {
	return math.Max(condTrustBase, condSlackFactor*math.Sqrt(tolerance))
}

// condFDStep is the relative finite-difference step for the conditioning
// Jacobian; the absolute step is condFDStep·(column scale), so length variables
// are perturbed by ~condFDStep·L and dimensionless ones by ~condFDStep — a
// scale- and translation-invariant step (unlike the solver's generic
// 1e-7·(1+|value|)).
const condFDStep = 1e-7

// rowKind classifies a residual row's physical units for nondimensionalizing.
type rowKind uint8

const (
	rowLength        rowKind = iota // residual carries length units (mm)
	rowDimensionless                // residual is a pure number (sin/cos/ratio/slack)
)

// lengthScale returns the sketch's characteristic length L: the bounding-box
// diagonal of its geometry, floored to 1 when absent or degenerate (mirrors the
// probe's perturbation scale).
func (s *Sketch) lengthScale() float64 {
	if b, ok := s.bounds(); ok {
		if h := math.Hypot(b.maxX-b.minX, b.maxY-b.minY); h > 1e-12 {
			return h
		}
	}
	return 1.0
}

// condVarScales returns the nondimensionalizing column scale per variable index:
// L for length-kind variables (point coordinates; the entity shape variables
// varKinds reports as varRadius, a circle's radius being one example; the
// conic-tangency contact-witness coordinates), and 1 for dimensionless variables
// (the shape variables varKinds reports as varAngle or varDimensionless, and
// every slack / spline-parameter aux — which varKinds would otherwise leave
// defaulted to coordinate).
func (s *Sketch) condVarScales(L float64) []float64 {
	scale := make([]float64, len(s.vars))
	for i := range scale {
		scale[i] = 1 // dimensionless default; covers slack/parameter aux vars
	}
	for _, p := range s.points {
		scale[p.xi] = L
		scale[p.yi] = L
	}
	if s.origin != nil {
		// The origin's coordinates are length-kind like any other point's. Its
		// columns are dropped before the SVD (they are fixed), so this changes no
		// measure today; it keeps the table coherent for anything that reads it.
		scale[s.origin.xi] = L
		scale[s.origin.yi] = L
	}
	for i, k := range s.varKinds() {
		switch k {
		case varRadius:
			scale[i] = L
		case varAngle:
			scale[i] = 1
		case varDimensionless:
			scale[i] = 1 // a conic's rho is a bounded ratio
		}
	}
	// The only length-kind aux variables are the conic-tangency contact-witness
	// coordinates (literal x,y positions); every other aux is a dimensionless
	// slack or curve parameter.
	for _, c := range s.cons {
		if tc, ok := c.(*tangentConics); ok && tc.px >= 0 {
			scale[tc.px] = L
			scale[tc.py] = L
		}
	}
	return scale
}

// positionShift returns, per variable index, the centroid offset to subtract to
// center the geometry: the centroid x for every point's x-coordinate (and the
// conic-tangency witness x), the centroid y for every y-coordinate, and 0 for
// non-positional variables (radii, angles, slacks, parameters). extraXY lists
// additional {xIndex, yIndex} position-variable pairs to center too — used by
// [Sketch.CheckConstraint] for a candidate constraint's witness coordinates, which
// are length-kind positions not yet reachable through s.cons. Used only to keep the
// finite-difference well-conditioned far from the origin; the translation does not
// change any residual.
func (s *Sketch) positionShift(extraXY ...[2]int) []float64 {
	shift := make([]float64, len(s.vars))
	if len(s.points) == 0 {
		return shift
	}
	var cx, cy float64
	for _, p := range s.points {
		cx += s.vars[p.xi]
		cy += s.vars[p.yi]
	}
	n := float64(len(s.points))
	cx, cy = cx/n, cy/n
	for _, p := range s.points {
		shift[p.xi] = cx
		shift[p.yi] = cy
	}
	// The ORIGIN moves with everything else. The shift is only sound because it is
	// a RIGID translation — every residual is invariant under one — so a position
	// left behind is not a translation at all: it silently changes the sketch the
	// finite differences measure. (Leaving the origin unshifted collapsed it onto a
	// point constrained to it, which read as a rank loss and an invented redundant
	// constraint.) The centroid itself is still taken over the authored points
	// only, so the shift for an origin-free sketch is byte-identical to before.
	if s.origin != nil {
		shift[s.origin.xi] = cx
		shift[s.origin.yi] = cy
	}
	for _, c := range s.cons {
		if tc, ok := c.(*tangentConics); ok && tc.px >= 0 {
			shift[tc.px] = cx
			shift[tc.py] = cy
		}
	}
	for _, xy := range extraXY {
		shift[xy[0]] = cx
		shift[xy[1]] = cy
	}
	return shift
}

// residualRowKinds returns the physical kind of every residual row, in the exact
// order and count [Sketch.residuals] produces them — including the same driven-
// dimension skip, so row↔kind alignment never shifts (mirroring the contract that
// binds RedundantConstraints to residuals()).
func (s *Sketch) residualRowKinds() []rowKind {
	var out []rowKind
	for _, c := range s.cons {
		if d, ok := c.(Dimension); ok && d.Driven() {
			continue
		}
		out = condRowKinds(c, out)
	}
	return out
}

// condRowKinds appends the physical kind of each row constraint c contributes to
// residuals() at the current configuration. It mirrors each constraint's
// residual() row structure exactly, including the aux-allocation-gated rows (a
// committed constraint has its slack/parameter aux allocated, so those rows are
// present). A length row carries length units; a dimensionless row is a
// sin/cos/dot-ratio or a slack-box / branch / sweep equation. Kept centralized
// (rather than a method per constraint) so the whole table is reviewable in one
// place; a length-equality test guards it against drift from residuals().
func condRowKinds(c Constraint, out []rowKind) []rowKind {
	switch t := c.(type) {
	case *arcRadius:
		return append(out, rowLength)
	case *coincident:
		return append(out, rowLength, rowLength)
	case *horizontal:
		return append(out, rowLength)
	case *vertical:
		return append(out, rowLength)
	case *horizontalPoints:
		return append(out, rowLength)
	case *verticalPoints:
		return append(out, rowLength)
	case *parallel:
		return append(out, rowDimensionless)
	case *perpendicular:
		return append(out, rowDimensionless)
	case *pointOnLine:
		return append(out, rowLength)
	case *collinear:
		return append(out, rowLength, rowLength)
	case *pointOnCircle:
		return append(out, rowLength)
	case *pointOnArc:
		out = append(out, rowLength) // on the circle
		if t.slack >= 0 {
			out = append(out, rowDimensionless) // in the sweep
		}
		return out
	case *pointOnEllipticalArc:
		out = append(out, rowLength) // Sampson membership
		if t.slack >= 0 {
			out = append(out, rowDimensionless) // in the sweep
		}
		return out
	case *pointOnSpline:
		if t.tvar < 0 {
			return out // unparameterized: no rows
		}
		return append(out, rowLength, rowLength, rowDimensionless, rowDimensionless)
	case *pointOnClosedSpline:
		if t.tvar < 0 {
			return out
		}
		return append(out, rowLength, rowLength) // membership only; no box (periodic)
	case *pointOnFitSpline:
		if t.tvar < 0 {
			return out
		}
		return append(out, rowLength, rowLength, rowDimensionless, rowDimensionless)
	case *tangentToSpline:
		if t.tvar < 0 {
			return out
		}
		// contact(L), parallel(D), two box slacks(D), no-cusp(D)
		return append(out, rowLength, rowDimensionless, rowDimensionless, rowDimensionless, rowDimensionless)
	case *tangentToClosedSpline:
		if t.tvar < 0 {
			return out
		}
		// contact(L), parallel(D), no-cusp(D); no box (periodic)
		return append(out, rowLength, rowDimensionless, rowDimensionless)
	case *tangentToFitSpline:
		if t.tvar < 0 {
			return out
		}
		// contact(L), parallel(D), two box slacks(D), no-cusp(D)
		return append(out, rowLength, rowDimensionless, rowDimensionless, rowDimensionless, rowDimensionless)
	case *pointOnConic:
		if t.tvar < 0 {
			return out
		}
		return append(out, rowLength, rowLength, rowDimensionless, rowDimensionless)
	case *tangentToConic:
		if t.tvar < 0 {
			return out
		}
		// contact(L), parallel(D), two box slacks(D), no-cusp(D)
		return append(out, rowLength, rowDimensionless, rowDimensionless, rowDimensionless, rowDimensionless)
	case *pointOnNURBS:
		if t.tvar < 0 {
			return out
		}
		return append(out, rowLength, rowLength, rowDimensionless, rowDimensionless)
	case *tangentToNURBS:
		if t.tvar < 0 {
			return out
		}
		// contact(L), parallel(D), two box slacks(D), no-cusp(D)
		return append(out, rowLength, rowDimensionless, rowDimensionless, rowDimensionless, rowDimensionless)
	case *tangentConics:
		if t.wSide < 0 {
			return out
		}
		if t.shared != nil {
			return append(out, rowDimensionless, rowDimensionless) // parallel, branch
		}
		// membership on A,B (L,L); parallel, branch (D,D)
		out = append(out, rowLength, rowLength, rowDimensionless, rowDimensionless)
		if t.slackA >= 0 {
			out = append(out, rowDimensionless)
		}
		if t.slackB >= 0 {
			out = append(out, rowDimensionless)
		}
		return out
	case *midpoint:
		return append(out, rowLength, rowLength)
	case *midpointOf:
		return append(out, rowLength, rowLength)
	case *symmetric:
		return append(out, rowLength, rowLength)
	case *symmetricLines:
		return append(out, rowLength, rowLength, rowLength, rowLength)
	case *symmetricCircles:
		return append(out, rowLength, rowLength, rowLength)
	case *symmetricArcs:
		// centers(L,L), endpoint(L,L), ray-collinear(L); same-ray branch(D) once allocated
		out = append(out, rowLength, rowLength, rowLength, rowLength, rowLength)
		if t.slack >= 0 {
			out = append(out, rowDimensionless)
		}
		return out
	case *concentric:
		return append(out, rowLength, rowLength)
	case *equalLines:
		return append(out, rowLength)
	case *equalRadii:
		return append(out, rowLength)
	case *pointOnEllipse:
		return append(out, rowLength)
	case *ellipticalArcOn:
		return append(out, rowLength)
	case *tangentLineCircle:
		_, isArc := t.C.(*Arc)
		if isArc && t.shared != nil {
			return append(out, rowDimensionless) // endpoint: line ⊥ radius
		}
		out = append(out, rowLength) // tangent gap |h|−r
		if isArc && t.slack >= 0 {
			out = append(out, rowDimensionless) // in the sweep
		}
		return out
	case *tangentCircles:
		out = append(out, rowLength) // center-distance tangency
		if t.shared != nil {
			return out // endpoint: no sweep rows
		}
		if _, ok := t.C1.(*Arc); ok && t.slack1 >= 0 {
			out = append(out, rowDimensionless)
		}
		if _, ok := t.C2.(*Arc); ok && t.slack2 >= 0 {
			out = append(out, rowDimensionless)
		}
		return out
	case *tangentLineEllipse:
		_, isArc := t.E.(*EllipticalArc)
		if isArc && t.shared != nil {
			return append(out, rowDimensionless) // endpoint: line ⊥ normal
		}
		out = append(out, rowLength) // tangent condition
		if t.slack >= 0 {
			out = append(out, rowDimensionless) // in the sweep
		}
		return out
	case *Distance:
		return append(out, rowLength)
	case *HorizontalDistance:
		return append(out, rowLength)
	case *VerticalDistance:
		return append(out, rowLength)
	case *DistancePointLine:
		return append(out, rowLength)
	case *DistancePointCircle:
		return append(out, rowLength)
	case *DistanceLineCircle:
		return append(out, rowLength)
	case *DistancePointArc:
		out = append(out, rowLength) // radial gap
		if t.slack >= 0 {
			out = append(out, rowDimensionless) // in the sweep
		}
		return out
	case *DistanceLineArc:
		out = append(out, rowLength) // tangent gap
		if t.slack >= 0 {
			out = append(out, rowDimensionless)
		}
		return out
	case *DistanceLines:
		return append(out, rowLength, rowLength)
	case *Offset:
		return append(out, rowLength, rowLength)
	case *Radius:
		return append(out, rowLength)
	case *Diameter:
		return append(out, rowLength)
	case *ArcLength:
		out = append(out, rowLength) // swept length
		if t.theta >= 0 {
			out = append(out, rowDimensionless) // unwrapped-sweep pin
		}
		return out
	case *equalLineArc:
		return append(out, rowLength)
	case *Angle:
		return append(out, rowDimensionless)
	case *SemiMajor:
		return append(out, rowLength)
	case *SemiMinor:
		return append(out, rowLength)
	case *EllipseRotation:
		return append(out, rowDimensionless)
	default:
		// An unclassified constraint cannot be soundly nondimensionalized; the
		// caller treats a kind/row-count mismatch as not-trustworthy rather than
		// guessing. This is unreachable for the committed constraint set (guarded
		// by TestConditioningRowKindsCoverAllConstraints).
		return nil
	}
}

// committedJacobian is the nondimensional Jacobian A = Drow·J·Dcol over the
// committed rows and the free variables at ONE call-time configuration. It is
// built by a single public entry point ([Sketch.Verify], via
// [Sketch.buildCommittedJacobian]) and consumed inside that same call by the
// rank/DOF, conditioning, conflict and free-point analyses (their …On
// methods) instead of each of them rebuilding it; it is never stored on the
// Sketch, never keyed by [Sketch.Revision], and never outlives the call that
// built it (see the rank/DOF invariant in CLAUDE.md — the configuration must
// not move between build and use, and nothing between them does: see
// buildCommittedJacobian's doc comment).
type committedJacobian struct {
	free     []int     // the free variable indices the columns of A are over
	m        int       // rows residuals() produces at the call-time configuration
	rowKinds []rowKind // residualRowKinds(); length may differ from m only in the
	// unreachable classification-gap case conditioningOn's NaN path guards
	A [][]float64 // nil when m == 0 — no rows means nothing to build
}

// buildCommittedJacobian builds one [committedJacobian] for the committed
// constraint rows (residuals(), driven dims skipped) at the current
// configuration, screened by the same non-finite-geometry guard
// [Sketch.committedRankAnalysis], [Sketch.conflictAnalysis] and
// [Sketch.movableVars] each carry when called on their own: the second result
// is false — and no matrix is built — when [Sketch.hasNonFiniteVars] holds.
// The `Conditioning` report field reads straight off this same guarded call,
// via [Sketch.conditioningOn], since no standalone wrapper computes it outside
// [Sketch.Verify].
//
// Between this call and the last consumer that reads the returned value in the
// SAME Verify call, the only code that touches s.vars is [Sketch.scaledJacobian]
// itself, which perturbs one variable at a time and restores its exact original
// bit pattern before returning — so the configuration this Jacobian was built
// at cannot have moved by the time any consumer reads it. Nothing may cache
// this value beyond that one call: see [committedJacobian]'s own doc comment.
func (s *Sketch) buildCommittedJacobian() (committedJacobian, bool) {
	if s.hasNonFiniteVars() {
		return committedJacobian{}, false
	}
	free := s.freeVars()
	m := len(s.residuals(nil))
	cj := committedJacobian{free: free, m: m, rowKinds: s.residualRowKinds()}
	if m > 0 {
		cj.A = s.committedScaledJacobian(free)
	}
	return cj, true
}

// cloneMatrix returns a deep copy of A. A consumer whose elimination mutates
// its matrix in place ([rankAnalysisOfMatrix], [Sketch.movableVars]) clones a
// shared [committedJacobian]'s A before running, so it does not corrupt what
// another consumer of the same committedJacobian reads afterward; a consumer
// that only reads rows ([Sketch.conflictAnalysis], [singularValueExtremes])
// needs no clone.
func cloneMatrix(A [][]float64) [][]float64 {
	out := make([][]float64, len(A))
	for i, row := range A {
		out[i] = slices.Clone(row)
	}
	return out
}

// committedScaledJacobian builds the nondimensional A = Drow·J·Dcol over the
// committed residual rows (residuals(), driven dims skipped) and the free
// variables, at the call-time configuration. It is the shared builder prelude
// behind the rank/DOF, conflict, free-point and conditioning analyses — they all
// nondimensionalize with the same length scale, row kinds and column scales.
func (s *Sketch) committedScaledJacobian(free []int) [][]float64 {
	L := s.lengthScale()
	return s.scaledJacobian(free, s.residuals, s.residualRowKinds(), s.condVarScales(L), L)
}

// scaledJacobian builds the physically nondimensional Jacobian A = Drow·J·Dcol of
// the residual vector produced by eval, over the free variables — the common basis
// for the conditioning measure AND the scale-invariant rank/dependency analyses
// (rank/DOF, conflict, free-points). J is the central-difference Jacobian; each
// row is scaled by 1/L when length-kind (rowKinds[i]) and 1 otherwise, and each
// column by its variable's colScale, so every entry is dimensionless and invariant
// under a uniform geometry rescale. The geometry is centred about its centroid for
// the FD pass (residuals depend only on coordinate differences, so this is exact
// but keeps the scale-relative step condFDStep·colScale from vanishing into
// floating-point cancellation far from the origin); the shift is restored exactly,
// so the call does not mutate the sketch. rowKinds MUST have one entry per row eval
// produces; colScale MUST be indexed by variable index.
func (s *Sketch) scaledJacobian(free []int, eval func([]float64) []float64, rowKinds []rowKind, colScale []float64, L float64, extraPos ...[2]int) [][]float64 {
	m := len(rowKinds)
	rowScale := make([]float64, m)
	for i, k := range rowKinds {
		if k == rowLength {
			rowScale[i] = 1 / L
		} else {
			rowScale[i] = 1
		}
	}

	shift := s.positionShift(extraPos...)
	saved := make([]float64, len(shift))
	for i, d := range shift {
		if d != 0 {
			saved[i] = s.vars[i]
			s.vars[i] -= d
		}
	}
	defer func() {
		// Restore the EXACT original bit pattern, not x−c+c (which would leave a
		// rounding residue and mutate the sketch).
		for i, d := range shift {
			if d != 0 {
				s.vars[i] = saved[i]
			}
		}
	}()

	A := make([][]float64, m)
	for i := range A {
		A[i] = make([]float64, len(free))
	}
	rp := make([]float64, 0, m)
	rm := make([]float64, 0, m)
	for j, vi := range free {
		cs := colScale[vi]
		h := condFDStep * cs
		orig := s.vars[vi]
		s.vars[vi] = orig + h
		rp = eval(rp)
		s.vars[vi] = orig - h
		rm = eval(rm)
		s.vars[vi] = orig
		inv := 1.0 / (2 * h)
		for i := 0; i < m; i++ {
			A[i][j] = rowScale[i] * (rp[i] - rm[i]) * inv * cs
		}
	}
	return A
}

// conditioningOn returns the scale-invariant reciprocal condition number
// σ_min(A)/σ_max(A) of the nondimensional constraint Jacobian held in a
// prebuilt [committedJacobian] cj — the core [Sketch.Verify] calls directly so
// this measure shares ONE Jacobian build with the rank/DOF, conflict and
// free-point analyses of the same call. It returns +Inf when there is nothing
// to measure (no free variables or no rows) and 0 when the matrix is
// numerically singular or the row-kind table is misaligned. Intended for a
// DOF-0 candidate; the caller gates on it only then.
//
// The non-finite-geometry screen is carried by [Sketch.buildCommittedJacobian],
// which builds cj: a caller reaching this method already holds a cj built on
// finite geometry. That screen has to sit on the geometry rather than on the
// matrix precisely because of the case this method's own "nothing to measure"
// shortcut produces: an all-grounded sketch holding a NaN still builds a
// perfectly finite ZERO-COLUMN matrix, and the len(free)==0 shortcut below
// would hand the trust gate its BEST possible reading, +Inf, without ever
// looking at a value. It reads cj.A only ([singularValueExtremes] copies it
// into its own working matrix), so it needs no clone the way
// [Sketch.rankAnalysisOn] and [Sketch.movableVarsOn] do.
func (s *Sketch) conditioningOn(cj committedJacobian) float64 {
	if len(cj.free) == 0 || cj.m == 0 {
		return math.Inf(1)
	}
	if len(cj.rowKinds) != cj.m {
		// The row-kind table did not align with the residual rows — a classification
		// gap (a constraint kind missing from condRowKinds). NaN, distinct from a
		// genuinely-singular 0; NaN fails the trust gate (NaN >= τ is false), so an
		// unclassified constraint reads as untrustworthy, never falsely blessed.
		return math.NaN()
	}
	smax, smin, ok := singularValueExtremes(cj.A)
	if !ok {
		return math.Inf(1)
	}
	if smax == 0 {
		return 0
	}
	return smin / smax
}

// dblEps is the double-precision unit roundoff spacing (2⁻⁵²) and safeMin the
// smallest positive normal double; both are the constants LAPACK's bisection
// (dstebz) is written in terms of.
const (
	dblEps  = 0x1p-52
	safeMin = 0x1p-1022
)

// singularValueExtremes returns σ_max and σ_min of the m×n matrix A, taken over
// its n COLUMNS the way the column-orthogonalizing one-sided Jacobi SVD it
// replaces did: when m < n the n columns cannot be independent, so σ_min is
// exactly 0. The third result is false when A has no rows or no columns. A
// non-finite entry yields NaN for both values, which the trust gate reads as
// untrustworthy (NaN ≥ τ is false) — defence in depth behind the geometry
// screen in nonfinite.go, which stops Verify before a matrix is ever built.
//
// The measure needs only the two extreme singular values, never the full
// spectrum, so this is Householder bidiagonalization (one O(m·n²) pass, cost
// equivalent to roughly one Jacobi sweep, where the Jacobi SVD needed 8–17
// sweeps on the benchmark fixtures) followed by Sturm-sequence bisection on the
// Golub–Kahan tridiagonal form of the bidiagonal for exactly the largest and
// the smallest value (O(n) per bisection step). Bisection is monotone and
// certified — each step brackets the value by an eigenvalue count, so there is
// no shift strategy, no deflation and no convergence heuristic — and the whole
// pass has a fixed, input-independent cost.
//
// Accuracy: bidiagonalization is backward stable, so every computed singular
// value is within c·ε·σ_max of the exact one (Weyl), with c a small multiple of
// the matrix dimension; bisection then resolves each to its last bit. The
// reported ratio σ_min/σ_max therefore carries an ABSOLUTE error of c·ε — far
// below the finite-difference noise the Jacobian entries themselves carry
// (condFDStep gives ~1e-9..1e-8 relative derivative noise, an absolute
// σ_min error of that order relative to σ_max) and six orders of magnitude
// below the trust gate. Relative to the reported value it is c·ε/Conditioning:
// ~1e-13 for the healthy fixtures the golden tests pin and ~1e-8 at the gate
// itself. Never via AᵀA, which would square the condition number into
// floating-point noise. The input is not modified.
func singularValueExtremes(A [][]float64) (float64, float64, bool) {
	m := len(A)
	if m == 0 {
		return 0, 0, false
	}
	n := len(A[0])
	if n == 0 {
		return 0, 0, false
	}
	// W is a TALL (rows ≥ cols) row-major working copy — A itself when m ≥ n,
	// Aᵀ otherwise; the two share their nonzero singular values — backed by one
	// contiguous allocation so the reflector loops below stream through memory.
	rows, cols := m, n
	if m < n {
		rows, cols = n, m
	}
	buf := make([]float64, rows*cols)
	W := make([][]float64, rows)
	for i := range W {
		W[i] = buf[i*cols : (i+1)*cols]
	}
	finite := true
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			v := A[i][j]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				finite = false
			}
			if m >= n {
				W[i][j] = v
			} else {
				W[j][i] = v
			}
		}
	}
	if !finite {
		return math.NaN(), math.NaN(), true
	}
	d, e := bidiagonalize(W)
	smax, smin := bidiagonalExtremes(d, e)
	if m < n {
		smin = 0
	}
	return smax, smin, true
}

// bidiagonalize reduces the tall (rows ≥ cols) row-major matrix W IN PLACE to
// upper bidiagonal form by alternating left and right Householder reflections
// (Golub–Kahan), returning the diagonal d (len cols) and the superdiagonal e
// (len cols; e[cols−1] is unused and 0). Orthogonal transforms preserve
// singular values, so those of (d, e) are those of W. The reflectors are never
// accumulated — only the values are wanted — and the entries each reflection
// annihilates are never written back, since no later step reads them.
func bidiagonalize(W [][]float64) ([]float64, []float64) {
	rows, cols := len(W), len(W[0])
	d := make([]float64, cols)
	e := make([]float64, cols)
	v := make([]float64, rows)    // left reflector, rows k..rows−1
	u := make([]float64, cols)    // right reflector, columns k+1..cols−1
	dots := make([]float64, cols) // vᵀ·W[:, j] per column j, for the left update
	for k := 0; k < cols; k++ {
		// Left reflection H = I − 2vvᵀ/vᵀv sends column k (rows k..) to α·e₁.
		// Applied row by row so the inner loops run along the contiguous rows.
		var s float64
		for i := k; i < rows; i++ {
			s += W[i][k] * W[i][k]
		}
		if s > 0 {
			alpha := -math.Copysign(math.Sqrt(s), W[k][k])
			v[k] = W[k][k] - alpha // |v[k]| = |W[k][k]| + √s > 0, so vᵀv > 0
			vv := v[k] * v[k]
			for i := k + 1; i < rows; i++ {
				v[i] = W[i][k]
				vv += v[i] * v[i]
			}
			for j := k + 1; j < cols; j++ {
				dots[j] = 0
			}
			for i := k; i < rows; i++ {
				row, vi := W[i], v[i]
				for j := k + 1; j < cols; j++ {
					dots[j] += vi * row[j]
				}
			}
			for i := k; i < rows; i++ {
				row, f := W[i], 2*v[i]/vv
				for j := k + 1; j < cols; j++ {
					row[j] -= f * dots[j]
				}
			}
			d[k] = alpha
		}
		if k+1 >= cols {
			break
		}
		// Right reflection sends row k (columns k+1..) to α·e₁ᵀ. Row k itself is
		// never read again, so only the rows below it are updated.
		s = 0
		for j := k + 1; j < cols; j++ {
			s += W[k][j] * W[k][j]
		}
		if s == 0 {
			continue
		}
		alpha := -math.Copysign(math.Sqrt(s), W[k][k+1])
		u[k+1] = W[k][k+1] - alpha
		uu := u[k+1] * u[k+1]
		for j := k + 2; j < cols; j++ {
			u[j] = W[k][j]
			uu += u[j] * u[j]
		}
		for i := k + 1; i < rows; i++ {
			row := W[i]
			var dd float64
			for j := k + 1; j < cols; j++ {
				dd += row[j] * u[j]
			}
			f := 2 * dd / uu
			for j := k + 1; j < cols; j++ {
				row[j] -= f * u[j]
			}
		}
		e[k] = alpha
	}
	return d, e
}

// bidiagonalExtremes returns σ_max and σ_min of the n×n upper bidiagonal matrix
// with diagonal d and superdiagonal e. The symmetric 2n×2n Golub–Kahan matrix
// T = [[0, Bᵀ], [B, 0]], permuted to tridiagonal form, has zero diagonal and the
// off-diagonal sequence (d₀, e₀, d₁, e₁, …, d_{n−1}); its eigenvalues are ±σᵢ.
// For λ > 0 the Sturm count of eigenvalues below λ is therefore n + #{σᵢ < λ},
// so σ_max is where the count first reaches 2n and σ_min where it first
// reaches n+1, each found by bisection from a Gershgorin bound.
func bidiagonalExtremes(d, e []float64) (float64, float64) {
	n := len(d)
	b := make([]float64, 2*n-1)
	for k := 0; k < n; k++ {
		b[2*k] = d[k]
		if k+1 < n {
			b[2*k+1] = e[k]
		}
	}
	var bmax2, hi float64
	for i, x := range b {
		bmax2 = math.Max(bmax2, x*x)
		r := math.Abs(x) // Gershgorin row sum: |b[i−1]| + |b[i]|
		if i > 0 {
			r += math.Abs(b[i-1])
		}
		hi = math.Max(hi, r)
	}
	if bmax2 == 0 {
		return 0, 0
	}
	// pivmin is dstebz's pivot floor: the smallest |q| the recurrence keeps,
	// chosen so b²/q never overflows.
	pivmin := safeMin * math.Max(1, bmax2)
	// Pad the bound past its own rounding so the count there is a full 2n.
	hi = hi*(1+4*dblEps) + pivmin
	b2 := make([]float64, len(b))
	for i, x := range b {
		b2[i] = x * x
	}
	return bisectSingularValues(b2, pivmin, hi, 2*n, n+1)
}

// sturmCounts returns, for each of the two thresholds la and lb, the number of
// eigenvalues of the symmetric tridiagonal matrix with zero diagonal and
// squared off-diagonals b2 that lie below it, by the LDLᵀ pivot recurrence
// q_i = −λ − b_{i−1}²/q_{i−1} (the count of negative pivots, Sylvester's law
// of inertia). A pivot smaller than pivmin in magnitude is replaced by −pivmin
// exactly as LAPACK's dstebz does, which keeps the next quotient finite without
// changing the count. The two thresholds are evaluated in one loop on purpose:
// each recurrence is a serial chain of dependent divisions, so two independent
// chains interleaved run in nearly the time of one.
func sturmCounts(b2 []float64, pivmin, la, lb float64) (int, int) {
	qa, qb := -la, -lb
	if math.Abs(qa) < pivmin {
		qa = -pivmin
	}
	if math.Abs(qb) < pivmin {
		qb = -pivmin
	}
	ca, cb := 0, 0
	if qa < 0 {
		ca++
	}
	if qb < 0 {
		cb++
	}
	for _, x2 := range b2 {
		qa = -la - x2/qa
		qb = -lb - x2/qb
		if math.Abs(qa) < pivmin {
			qa = -pivmin
		}
		if math.Abs(qb) < pivmin {
			qb = -pivmin
		}
		if qa < 0 {
			ca++
		}
		if qb < 0 {
			cb++
		}
	}
	return ca, cb
}

// bisectSingularValues returns the smallest λ ≥ 0 at which the Sturm count
// reaches ta, and likewise for tb, bisecting [0, hi] for both in lockstep (hi
// must already satisfy both counts) until each bracket is a couple of ulps wide
// or has no representable midpoint left. A value of exactly 0 (a rank-deficient
// matrix) drives its bracket down through the subnormals, so the loop is also
// capped; at 4n flops per step the cap is still negligible next to the
// bidiagonalization.
func bisectSingularValues(b2 []float64, pivmin, hi float64, ta, tb int) (float64, float64) {
	const maxSteps = 1200
	loA, hiA := 0.0, hi
	loB, hiB := 0.0, hi
	for step := 0; step < maxSteps; step++ {
		midA, midB := 0.5*(loA+hiA), 0.5*(loB+hiB)
		doneA := hiA-loA <= 2*dblEps*hiA || midA <= loA || midA >= hiA
		doneB := hiB-loB <= 2*dblEps*hiB || midB <= loB || midB >= hiB
		if doneA && doneB {
			break
		}
		ca, cb := sturmCounts(b2, pivmin, midA, midB)
		switch {
		case doneA:
		case ca >= ta:
			hiA = midA
		default:
			loA = midA
		}
		switch {
		case doneB:
		case cb >= tb:
			hiB = midB
		default:
			loB = midB
		}
	}
	return 0.5 * (loA + hiA), 0.5 * (loB + hiB)
}
