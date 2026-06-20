package sketch

import "math"

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
//   - Dcol scales each length-kind variable column by L (point coordinates,
//     circle radii / ellipse semi-axes, and the conic-tangency contact-witness
//     coordinates) and leaves dimensionless columns (ellipse rotation, every
//     slack / spline-parameter aux) at 1.
//   - Drow scales each length-kind residual row by 1/L and leaves dimensionless
//     rows at 1.
// Every entry of A is then dimensionless and invariant under a uniform rescale of
// the geometry (a length-row/length-col entry picks up L·(1/L)=1; a
// dimensionless-row/length-col entry is already scale-free; etc.). The measure is
//
//   Conditioning = σ_min(A) / σ_max(A)
//
// the smallest singular value relative to the largest, computed by a one-sided
// Jacobi SVD (never via AᵀA, which would square the condition number into
// floating-point noise). It is evaluated only for an otherwise fully-constrained
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
// L for length-kind variables (point coordinates; circle radii / ellipse
// semi-axes via varKinds; the conic-tangency contact-witness coordinates), and 1
// for dimensionless variables (ellipse rotation and every slack / spline-parameter
// aux — which varKinds would otherwise leave defaulted to coordinate).
func (s *Sketch) condVarScales(L float64) []float64 {
	scale := make([]float64, len(s.vars))
	for i := range scale {
		scale[i] = 1 // dimensionless default; covers slack/parameter aux vars
	}
	for _, p := range s.points {
		scale[p.xi] = L
		scale[p.yi] = L
	}
	for i, k := range s.varKinds() {
		switch k {
		case varRadius:
			scale[i] = L
		case varAngle:
			scale[i] = 1
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
// non-positional variables (radii, angles, slacks, parameters). Used only to
// keep the conditioning finite-difference well-conditioned far from the origin;
// the translation does not change any residual.
func (s *Sketch) positionShift() []float64 {
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
	for _, c := range s.cons {
		if tc, ok := c.(*tangentConics); ok && tc.px >= 0 {
			shift[tc.px] = cx
			shift[tc.py] = cy
		}
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

// conditioningMatrix builds the nondimensional A = Drow·J·Dcol over the free
// variables and the current residual rows, using a scale-aware finite-difference
// step. Returns nil if the row-kind table does not align with the residual rows
// (a defensive guard; the alignment is asserted by test).
func (s *Sketch) conditioningMatrix(free []int, m int, L float64) [][]float64 {
	kinds := s.residualRowKinds()
	if len(kinds) != m {
		return nil
	}
	colScale := s.condVarScales(L)
	rowScale := make([]float64, m)
	for i, k := range kinds {
		if k == rowLength {
			rowScale[i] = 1 / L
		} else {
			rowScale[i] = 1
		}
	}

	// Center the geometry about its centroid for the finite-difference pass. Every
	// residual is built from coordinate DIFFERENCES, so a uniform translation
	// leaves the Jacobian unchanged in exact arithmetic — but it keeps coordinate
	// magnitudes at O(L) so the scale-relative step condFDStep·L does not vanish
	// into floating-point cancellation for geometry placed far from the origin
	// (the property the conditioning measure must be translation-invariant). The
	// shift is restored before returning.
	shift := s.positionShift()
	saved := make([]float64, len(shift))
	for i, d := range shift {
		if d != 0 {
			saved[i] = s.vars[i]
			s.vars[i] -= d
		}
	}
	defer func() {
		// Restore the EXACT original bit pattern, not x−c+c (which would leave a
		// rounding residue and make Verify a mutator).
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
		rp = s.residuals(rp)
		s.vars[vi] = orig - h
		rm = s.residuals(rm)
		s.vars[vi] = orig
		inv := 1.0 / (2 * h)
		for i := 0; i < m; i++ {
			A[i][j] = rowScale[i] * (rp[i] - rm[i]) * inv * cs
		}
	}
	return A
}

// conditioning returns the scale-invariant reciprocal condition number
// σ_min(A)/σ_max(A) of the nondimensional constraint Jacobian at the current
// configuration. It returns +Inf when there is nothing to measure (no free
// variables or no rows) and 0 when the matrix is numerically singular or the
// row-kind table is misaligned. Intended for a DOF-0 candidate; the caller gates
// on it only then.
func (s *Sketch) conditioning() float64 {
	free := s.freeVars()
	m := len(s.residuals(nil))
	if len(free) == 0 || m == 0 {
		return math.Inf(1)
	}
	A := s.conditioningMatrix(free, m, s.lengthScale())
	if A == nil {
		// The row-kind table did not align with the residual rows — a classification
		// gap (a constraint kind missing from condRowKinds). Return NaN, distinct
		// from a genuinely-singular 0; NaN fails the trust gate (NaN >= τ is false),
		// so an unclassified constraint reads as untrustworthy, never falsely blessed.
		return math.NaN()
	}
	sv := jacobiSingularValues(A)
	if len(sv) == 0 {
		return math.Inf(1)
	}
	smax, smin := sv[0], sv[0]
	for _, v := range sv {
		if v > smax {
			smax = v
		}
		if v < smin {
			smin = v
		}
	}
	if smax == 0 {
		return 0
	}
	return smin / smax
}

// jacobiSingularValues returns the singular values of an m×n matrix via a
// one-sided Jacobi SVD: orthogonalize the columns by Jacobi plane rotations, then
// the singular values are the converged column norms. One-sided Jacobi computes
// small singular values to high relative accuracy and needs no AᵀA (which would
// square the condition number). The input is not modified.
func jacobiSingularValues(A [][]float64) []float64 {
	m := len(A)
	if m == 0 {
		return nil
	}
	n := len(A[0])
	if n == 0 {
		return nil
	}
	// Work on a column-major copy so column operations are cache-friendly.
	U := make([][]float64, n)
	for j := 0; j < n; j++ {
		col := make([]float64, m)
		for i := 0; i < m; i++ {
			col[i] = A[i][j]
		}
		U[j] = col
	}
	const maxSweeps = 60
	const eps = 1e-15
	for sweep := 0; sweep < maxSweeps; sweep++ {
		rotated := false
		for p := 0; p < n-1; p++ {
			for q := p + 1; q < n; q++ {
				var alpha, beta, gamma float64
				up, uq := U[p], U[q]
				for i := 0; i < m; i++ {
					alpha += up[i] * up[i]
					beta += uq[i] * uq[i]
					gamma += up[i] * uq[i]
				}
				if alpha == 0 || beta == 0 || math.Abs(gamma) <= eps*math.Sqrt(alpha*beta) {
					continue
				}
				// Jacobi rotation zeroing the (p,q) inner product.
				zeta := (beta - alpha) / (2 * gamma)
				tval := math.Copysign(1, zeta) / (math.Abs(zeta) + math.Sqrt(1+zeta*zeta))
				cval := 1 / math.Sqrt(1+tval*tval)
				sval := cval * tval
				for i := 0; i < m; i++ {
					a, b := up[i], uq[i]
					up[i] = cval*a - sval*b
					uq[i] = sval*a + cval*b
				}
				rotated = true
			}
		}
		if !rotated {
			break
		}
	}
	sv := make([]float64, n)
	for j := 0; j < n; j++ {
		var nrm float64
		for i := 0; i < m; i++ {
			nrm += U[j][i] * U[j][i]
		}
		sv[j] = math.Sqrt(nrm)
	}
	return sv
}
