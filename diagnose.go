package sketch

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
)

// ErrOverconstrained is returned (wrapped) by [Sketch.CheckConstraint] when a
// candidate constraint would not add an independent equation — committing it
// would make the sketch redundant or conflicting.
var ErrOverconstrained = errors.New("sketch: constraint would over-constrain the sketch")

// ErrCorruptHandle is returned (wrapped) by [Sketch.CheckConstraint] for a
// candidate that cannot be read: a nil constraint, a typed-nil one, or a live
// one holding a nil operand. It is the corrupt-handle counterpart of
// [ErrForeignHandle] — the sketch may own every handle such a candidate names,
// in which case reporting it foreign would be a false statement. A candidate
// that is both corrupt and foreign is reported corrupt: the corruption screen
// runs first, and an unreadable candidate cannot be probed at all.
var ErrCorruptHandle = errors.New("sketch: corrupt handle")

// conflictTol separates a satisfied residual from a violated one when
// partitioning dependent constraints in [Sketch.Diagnose]. Residuals are
// unit-normalized (length units or dimensionless), and a converged solve
// leaves them at or below the solver tolerance (1e-10), so anything above
// this is a genuine violation rather than numerical noise.
const conflictTol = 1e-8

// Diagnosis partitions the constraints that contribute dependent equations at
// the current configuration. See [Sketch.Diagnose].
type Diagnosis struct {
	// Redundant constraints duplicate information the rest of the sketch
	// already pins down, and currently hold: removing one changes nothing.
	Redundant []Constraint
	// Conflicting constraints are dependent and violated: they fight other
	// constraints over the same degrees of freedom and the solver cannot
	// satisfy everything at once. At least one constraint in each conflict
	// must be removed or edited.
	Conflicting []Constraint
}

// Diagnose classifies the sketch's dependent constraints as redundant
// (consistent duplicates) or conflicting (mutually unsatisfiable), refining
// the flat list returned by [Sketch.RedundantConstraints]. Like that method
// it analyses the call-time configuration and follows creation order — of two
// constraints fighting over the same equation, the later-added one is
// reported. Call it after [Sketch.Solve]: a converged solve leaves dependent-
// but-satisfied constraints (redundant), a failed solve leaves dependent
// constraints with residuals the solver could not remove (conflicting).
//
// For the conflicting constraints together with the earlier constraints each
// one fights — the conflict set — call [Sketch.Verify], which reports the same
// partition plus that attribution.
//
// On non-finite geometry (see [Sketch.DOF]'s doc comment for why) the
// dependency analysis this method otherwise depends on cannot be trusted, so
// — like DOF and [Sketch.FreePoints] — it answers with MAXIMUM IGNORANCE
// rather than refuse: every constraint the analysis could ever flag (every
// one contributing residual rows, driven dimensions excluded since they
// contribute none and so can never be flagged) is reported as Conflicting,
// and Redundant is empty.
//
// An empty Diagnosis is NOT that answer, and is the reason this screen exists
// at all: a caller reads two empty lists as "no constraint problem found",
// the one direction a poisoned dependency analysis must never be allowed to
// report. Conflicting rather than Redundant because "removing one changes
// nothing" is itself a claim about geometry nothing here can support, while
// Conflicting says only that no constraint in this sketch has been proven
// consistent. This is a deliberate design choice, not an oversight: call
// [Sketch.Verify] to learn that the condition was found at all, and which
// geometry carries it.
func (s *Sketch) Diagnose() *Diagnosis {
	flagged, conflicts, analysed := s.conflictAnalysis()
	if !analysed {
		return &Diagnosis{Conflicting: s.unprovenConstraints()}
	}
	conflicting := make(map[Constraint]struct{}, len(conflicts))
	for _, cs := range conflicts {
		conflicting[cs.Constraint] = struct{}{}
	}
	d := &Diagnosis{}
	for _, c := range flagged {
		if _, bad := conflicting[c]; bad {
			d.Conflicting = append(d.Conflicting, c)
			continue
		}
		d.Redundant = append(d.Redundant, c)
	}
	return d
}

// unprovenConstraints is the maximum-ignorance constraint set [Sketch.Diagnose]
// and [Sketch.RedundantConstraints] report on non-finite geometry: every
// constraint the dependency analysis could ever flag. It mirrors residuals()'s
// iteration exactly — driven dimensions skipped, since they contribute no
// residual row and so can never be flagged by the real analysis either — so
// the fabricated answer names a set the honest answer is a subset of, never a
// constraint the analysis would not have looked at.
func (s *Sketch) unprovenConstraints() []Constraint {
	var out []Constraint
	for _, c := range s.cons {
		if d, ok := c.(Dimension); ok && d.Driven() {
			continue
		}
		out = append(out, c)
	}
	return out
}

// ConflictSet reports a conflicting constraint together with the earlier
// constraints whose equations it fights over the same degrees of freedom. A
// conflicting constraint is dependent on those earlier constraints (it adds no
// independent equation) yet violated at the current configuration: the solver
// cannot satisfy it and them at once. Resolving the conflict means removing or
// editing the conflicting constraint itself or any of its [ConflictSet.With]
// members.
type ConflictSet struct {
	// Constraint is the conflicting constraint. By creation order it is the
	// later-added one, mirroring [Sketch.RedundantConstraints].
	Constraint Constraint
	// With holds the earlier independent constraints it conflicts with, in
	// creation order. It is empty when the constraint is violated by grounded
	// geometry alone (its equation touches no free variable), leaving no other
	// constraint to fight.
	With []Constraint
}

// conflictAnalysis is the shared dependency analysis behind
// [Sketch.RedundantConstraints], [Sketch.Diagnose] and [Sketch.Verify]. It
// walks the constraint residual rows in creation order (mirroring residuals(),
// driven dimensions skipped), flags every constraint that contributes a row
// linearly dependent on the rows of earlier constraints at the call-time
// configuration, and for each flagged-and-violated constraint computes its
// conflict set: the earlier independent constraints whose rows combine to
// reproduce the violated row's direction.
//
// The first result lists every flagged constraint in first-seen order (the
// redundant and conflicting ones together, exactly what RedundantConstraints
// reports). The second lists only the conflicting ones (residual above
// conflictTol) with their attribution. The sketch is not modified.
//
// It is one of the three primitives that CARRY the non-finite-geometry screen
// (see nonfinite.go): the third result is false — and no analysis is run — when
// [Sketch.hasNonFiniteVars] holds, so a caller cannot read a dependency verdict
// built from a poisoned Jacobian without handling the refusal. The screen sits
// above the zero-row shortcut below, so it covers the branch where no Jacobian
// is built at all and no caller needs a second, direct check beside this one.
// The Gram–Schmidt projection that decides dependency here is as meaningless
// over a non-finite Jacobian as the partial-pivot elimination is: both of its
// tests ("scale < eps" and "rest <= eps*scale") are false whenever the row norm
// is NaN, so such a row is always accepted as independent and its NaN enters the
// basis, after which every later row projects to NaN and is accepted too — no
// constraint is flagged at all, which is exactly the "no problem found" reading
// the screen exists to prevent.
func (s *Sketch) conflictAnalysis() ([]Constraint, []ConflictSet, bool) {
	cj, ok := s.buildCommittedJacobian()
	if !ok {
		return nil, nil, false
	}
	flagged, conflicts := s.conflictAnalysisOn(cj)
	return flagged, conflicts, true
}

// conflictAnalysisOn runs the same dependency analysis as
// [Sketch.conflictAnalysis] over a prebuilt [committedJacobian] — the core
// [Sketch.Verify] calls directly so this pass shares ONE Jacobian build with
// the rank/DOF, conditioning and free-point analyses of the same call. It
// still evaluates the committed constraints' own residuals itself (to map each
// row to its owning constraint and to read the violation each one carries),
// since a committedJacobian carries only the Jacobian, not those raw residual
// values or the owner mapping.
func (s *Sketch) conflictAnalysisOn(cj committedJacobian) ([]Constraint, []ConflictSet) {
	// Map each residual row to the constraint that produced it, mirroring the
	// iteration (and therefore row) order of residuals().
	var owners []Constraint
	var probe []float64
	for _, c := range s.cons {
		if d, ok := c.(Dimension); ok && d.Driven() {
			continue
		}
		n0 := len(probe)
		probe = c.residual(probe)
		for i := n0; i < len(probe); i++ {
			owners = append(owners, c)
		}
	}
	m := len(owners)
	if m == 0 {
		return nil, nil
	}

	// Dependency analysis runs on the NONDIMENSIONAL Jacobian A = Drow·J·Dcol (the
	// same scaled basis as rank/DOF), so the structural cutoff is scale/unit
	// invariant and the redundancy verdict matches DOF at any geometry size. Row
	// kinds mirror residuals() iteration (and therefore owners) by construction.
	// Read-only below (only ever copied into v/accRows), so the shared cj.A needs
	// no clone the way rankAnalysisOn/movableVarsOn's in-place elimination does.
	J := cj.A

	// Incremental Gram–Schmidt over the Jacobian rows: a row that projects to
	// (numerically) zero against the rows accepted so far adds no independent
	// equation, so its constraint is dependent at this configuration. The
	// original (un-orthogonalized) accepted rows are kept alongside the
	// orthonormal basis so a dependent row can be expressed as their linear
	// combination — its conflict set.
	const eps = rankZeroTol
	var basis [][]float64   // orthonormal basis of accepted directions
	var accRows [][]float64 // accepted original rows, parallel to accIdx
	var accIdx []int        // owners-row index of each accepted row

	// probe now holds the full residual vector, parallel to owners (same
	// iteration order), so probe[i] is the residual of row i. A flagged
	// constraint is conflicting only when one of its own *dependent* rows is
	// violated — an independent but unsolved row (e.g. the still-free leg of a
	// partly-dependent multi-row constraint) is a solvability gap, not a
	// conflict of this constraint.
	res := probe

	var flagged []Constraint
	seen := make(map[Constraint]struct{})
	violated := make(map[Constraint]bool)
	fights := make(map[Constraint]map[Constraint]struct{})
	for i := 0; i < m; i++ {
		scale := math.Sqrt(dot(J[i], J[i]))
		dependent := scale < eps
		if !dependent {
			v := append([]float64(nil), J[i]...)
			for pass := 0; pass < 2; pass++ { // second pass re-orthogonalizes
				for _, b := range basis {
					p := dot(v, b)
					for k := range v {
						v[k] -= p * b[k]
					}
				}
			}
			rest := math.Sqrt(dot(v, v))
			if rest <= eps*scale {
				dependent = true
			} else {
				inv := 1 / rest
				for k := range v {
					v[k] *= inv
				}
				basis = append(basis, v)
				accRows = append(accRows, append([]float64(nil), J[i]...))
				accIdx = append(accIdx, i)
			}
		}
		if !dependent {
			continue
		}
		c := owners[i]
		if _, dup := seen[c]; !dup {
			seen[c] = struct{}{}
			flagged = append(flagged, c)
			fights[c] = make(map[Constraint]struct{})
		}
		if math.Abs(res[i]) > conflictTol {
			violated[c] = true
		}
		// Attribute this dependent row to the accepted earlier rows it combines.
		for _, a := range rowCombo(basis, accRows, J[i]) {
			if owner := owners[accIdx[a]]; owner != c {
				fights[c][owner] = struct{}{}
			}
		}
	}
	if len(flagged) == 0 {
		return nil, nil
	}

	// Split into redundant (every dependent row satisfied) vs conflicting (a
	// dependent row violated), building each conflict set in creation order.
	consIdx := make(map[Constraint]int, len(s.cons))
	for i, c := range s.cons {
		consIdx[c] = i
	}
	var conflicts []ConflictSet
	for _, c := range flagged {
		if !violated[c] {
			continue // redundant: a consistent duplicate
		}
		members := make([]Constraint, 0, len(fights[c]))
		for f := range fights[c] {
			members = append(members, f)
		}
		sort.Slice(members, func(i, j int) bool { return consIdx[members[i]] < consIdx[members[j]] })
		conflicts = append(conflicts, ConflictSet{Constraint: c, With: members})
	}
	return flagged, conflicts
}

// rowCombo expresses target as a linear combination of accRows (assumed
// linearly independent and to span target, which holds because the caller only
// passes a row already found dependent) and returns the indices into accRows
// whose coefficient is non-negligible — the accepted rows that actually
// participate. It returns nil when accRows is empty or target's gradient is
// numerically zero (a constraint touching no free variable participates with
// nothing).
//
// basis is the orthonormal Gram–Schmidt basis parallel to accRows (basis[a] is
// accRows[a] orthogonalized against the earlier accepted rows). Resolving the
// coefficients in that basis turns the system upper-triangular — B[j][a] =
// basis[j]·accRows[a] is zero for j>a, with diagonal entries equal to the
// acceptance norms (bounded below by the Gram–Schmidt tolerance) — which is far
// better conditioned than the normal equations accRows·accRowsᵀ, whose
// condition number is squared and can go singular for nearly parallel but still
// independent accepted rows.
func rowCombo(basis, accRows [][]float64, target []float64) []int {
	k := len(accRows)
	if k == 0 {
		return nil
	}
	// Solve B·coef = rhs with B[j][a] = basis[j]·accRows[a] (upper triangular)
	// and rhs[j] = basis[j]·target.
	B := make([][]float64, k)
	rhs := make([]float64, k)
	for j := 0; j < k; j++ {
		B[j] = make([]float64, k)
		for a := 0; a < k; a++ {
			B[j][a] = dot(basis[j], accRows[a])
		}
		rhs[j] = dot(basis[j], target)
	}
	coef, ok := solveLinear(B, rhs)
	if !ok {
		return nil
	}
	maxC := 0.0
	for _, v := range coef {
		if av := math.Abs(v); av > maxC {
			maxC = av
		}
	}
	if maxC == 0 {
		return nil
	}
	thr := math.Max(1e-9, 1e-6*maxC)
	var out []int
	for a, v := range coef {
		if math.Abs(v) >= thr {
			out = append(out, a)
		}
	}
	return out
}

// CheckConstraint reports whether committing c would over-constrain the
// sketch, without committing anything. It returns nil when every equation c
// contributes is independent of the existing constraints at the current
// configuration, and an error wrapping [ErrOverconstrained] otherwise — both
// a consistent duplicate and a contradiction are rejected, since neither adds
// an equation the sketch can use. Driven dimensions contribute no equations
// and always pass.
//
// It returns an error wrapping [ErrCorruptHandle], before any other screen runs,
// for a candidate that cannot be read at all: a nil candidate, a typed-nil one
// (a nil pointer of a concrete constraint type boxed in the interface), or a
// live one holding a nil point or entity operand. Every pass below dereferences
// the candidate, so this is what keeps the method's documented non-panicking,
// error-returning contract true of a corrupt candidate too; the sketch may own
// every handle such a candidate names, in which case [ErrForeignHandle] would
// be the wrong sentinel. A candidate that is both corrupt and foreign is
// reported corrupt too: this screen runs first, and the ownership defect
// surfaces on the re-check and in [Sketch.Verify].
//
// It returns an error wrapping [ErrForeignHandle], without probing, for a
// candidate referencing geometry this sketch does not own and for one whose
// auxiliary solver variables another sketch allocated. The probe parameterizes
// the candidate against THIS sketch's variable vector, which for a constraint
// another sketch already committed would rebind it away from its owner — so the
// refusal is what makes the documented "leaves the sketch untouched" true of the
// other sketch too. The two conditions are independent: an operand rewired to
// this sketch's geometry after a commit elsewhere passes the reference screen
// while its indices still address the other sketch's vector.
//
// It returns an error wrapping [ErrNonFiniteGeometry], without probing, in two
// cases. First, when the candidate ITSELF is a non-driven [Dimension] whose own
// target is not a finite number: an unscreened non-finite target parameterizes
// the augmented rank probe with a NaN row of its own, which a plain
// partial-pivot comparison never correctly accepts or rejects — so a poisoned
// candidate could pass even against an otherwise finite, EMPTY sketch. That is a
// false statement about the sketch, not merely a differently-phrased one, and
// this check runs before either rank pass reads the candidate. Second, when
// this sketch's OWN geometry already holds a non-finite (NaN or infinite) point
// coordinate, entity shape variable, dimension target, or constraint-owned
// auxiliary variable (see [Sketch.nonFiniteVars]) — not a property of the
// candidate c at all. The rank probe below ranks c against the sketch's
// existing committed rows on the same nondimensional Jacobian [Sketch.Verify]
// refuses to build over such geometry: a NaN pivot is neither correctly
// selected nor correctly rejected by a plain partial-pivot comparison, so the
// probe can silently rank a genuine duplicate as independent and accept it.
// Unlike Verify this method has no report field to record a skip on, so it
// refuses outright with the same sentinel rather than return a fabricated
// verdict. A DRIVEN dimension is the one candidate NEITHER screen reaches: it
// returns nil above, before either check runs or the probe allocates or ranks
// anything, and it contributes no residual row for a poisoned pivot to misrank
// — so refusing it would report a defect on a verdict the poisoned geometry
// never takes part in.
//
// Like [Sketch.DOF], the analysis is local to the call-time configuration;
// check against solved geometry (after [Sketch.Solve]) for the most reliable
// verdict. A caller that wants Fusion's behavior — refuse the gesture, leave
// the sketch untouched — calls this before [Sketch.AddConstraint].
func (s *Sketch) CheckConstraint(c Constraint) error {
	// Screened first: every pass below dereferences the candidate — foreignConstraint
	// reads its operand fields through constraintRefs, allocVars reads its operands'
	// coordinates, and residual reads all of them — so a nil candidate, a typed-nil
	// one or one holding a nil operand panics a documented error-returning call.
	if corruptConstraint(c) {
		return fmt.Errorf("%w: the candidate is nil or holds a nil operand", ErrCorruptHandle)
	}
	// Screened before the driven-dimension shortcut so the verdict on a foreign
	// handle does not depend on whether the candidate happens to drive anything.
	if s.foreignConstraint(c) {
		return fmt.Errorf("%w: the candidate references geometry this sketch does not own", ErrForeignHandle)
	}
	// The second half of the same guard: a candidate whose auxiliary variables
	// another sketch allocated addresses that sketch's variable vector, and
	// allocVars below would rebind it to this one while the indices stay stale.
	if s.foreignAllocation(c) {
		return fmt.Errorf("%w: the candidate's auxiliary variables were allocated by another sketch", ErrForeignHandle)
	}
	if d, ok := c.(Dimension); ok && d.Driven() {
		return nil // measures the geometry, constrains nothing
	}
	// The candidate's OWN target, independent of this sketch's geometry: a
	// non-finite target makes c's own residual row NaN, which the augmented rank
	// probe below never soundly accepts or rejects — so a poisoned candidate
	// could pass even against an otherwise finite, EMPTY sketch. That is a false
	// statement about the sketch, not a differently-phrased true one, so it is
	// screened before any allocation or residual evaluation touches it.
	if d, ok := c.(Dimension); ok && nonFinite(d.base()) {
		return fmt.Errorf("%w: the candidate's target is not a finite number", ErrNonFiniteGeometry)
	}
	// This sketch's OWN geometry, independent of c: a non-finite pivot in the
	// probe below is never soundly accepted or rejected either way, so a
	// candidate that genuinely over-constrains a poisoned sketch can pass. See
	// the doc comment above and [Sketch.nonFiniteVars].
	//
	// Below the driven shortcut, because that shortcut returns before allocVars,
	// c.residual and both rank passes: a driven dimension contributes no residual
	// row (residuals() skips it), so nothing about it is ever ranked and there is
	// no verdict for a poisoned pivot to corrupt. Refusing it would report a
	// defect on a call the poisoned geometry cannot reach.
	if s.hasNonFiniteVars() {
		return s.nonFiniteError()
	}
	// A constraint that owns auxiliary variables (the allocVars hook) parameterizes
	// its residual with variables allocated only at commit — a spline foot point, an
	// arc sweep slack. Probe it in its committed form: temporarily allocate those
	// variables (so the rank analysis sees the real rows and counts them as free
	// unknowns), then roll back, keeping the check non-mutating.
	//
	// The variable rollback is installed only when the probe actually allocated,
	// and that gate is load-bearing: allocVars is idempotent, so a candidate
	// already committed to this sketch returns without allocating, and retiring
	// unconditionally would strip the COMMITTED constraint's real aux variables
	// (dropping a row from a live residual).
	//
	// The OWNER pointer is rolled back on its own gate, because allocVars binds it
	// ahead of that idempotence guard and so rebinds even on the paths that
	// allocate nothing. Leaving it bound is not harmless: the screens above refuse
	// a candidate another sketch HOLDS an allocation from, and for a type that does
	// not report its own allocation (auxAllocated) that pointer is the whole answer,
	// so a probe that left it on an unallocated candidate would make the next
	// commit — to this sketch or any other — refuse a constraint no sketch has
	// parameterized. Restoring the
	// unowned state keeps this call non-mutating for the candidate as well as for
	// both sketches. A candidate this sketch already owns keeps its pointer, which
	// is the one a commit would bind anyway.
	if av, ok := c.(interface{ allocVars(*Sketch) }); ok {
		n := len(s.vars)
		unowned := auxOwnerOf(c) == nil
		av.allocVars(s)
		allocated := len(s.vars) > n
		if allocated || unowned {
			defer func() {
				if allocated {
					if rv, ok := c.(interface{ retireVars(*Sketch) }); ok {
						rv.retireVars(s) // also resets the candidate's indices to -1
					}
					s.vars = s.vars[:n]
					s.fixed = s.fixed[:n]
				}
				if unowned {
					clearAuxOwnerOf(c)
				}
			}()
		}
	}
	k := len(c.residual(nil))
	if k == 0 {
		return nil
	}
	free := s.freeVars()
	L := s.lengthScale()
	// Candidate-aware column scaling: condVarScales only marks the conic witness
	// coordinates of COMMITTED constraints, but c (temporarily allocated above) is
	// not yet committed, so mark its length-kind witness coords here too. Every
	// other aux is dimensionless (scale 1) by default.
	colScale := s.condVarScales(L)
	// The candidate's length-kind witness coords also need centering for the FD
	// pass (they are not in s.cons, so positionShift cannot find them): pass them as
	// extra positions, or the augmented-rank FD is linearized at inconsistent
	// coordinates far from the origin and a duplicate could slip through.
	var extraPos [][2]int
	if tc, ok := c.(*tangentConics); ok && tc.px >= 0 {
		colScale[tc.px] = L
		colScale[tc.py] = L
		extraPos = append(extraPos, [2]int{tc.px, tc.py})
	}
	rk0 := s.residualRowKinds()
	var r0 int
	if len(rk0) > 0 {
		r0 = s.rankAnalysisOf(free, s.residuals, rk0, colScale, L, extraPos...).rank
	}
	// Augmented system: the committed rows followed by c's own rows, with matching
	// row kinds (condRowKinds mirrors c.residual at its committed aux state).
	aug := func(buf []float64) []float64 { return c.residual(s.residuals(buf)) }
	rkAug := condRowKinds(c, append([]rowKind(nil), rk0...))
	r1 := s.rankAnalysisOf(free, aug, rkAug, colScale, L, extraPos...).rank
	if r1 < r0+k {
		return fmt.Errorf("%w: %d of its %d equations depend on existing constraints", ErrOverconstrained, r0+k-r1, k)
	}
	return nil
}

// FreePoints reports which points can still move — the under-constrained
// remainder of the sketch, in id order. A point is free when some first-order
// motion compatible with every constraint displaces it: the union of supports
// of a null-space basis of the constraint Jacobian at the current
// configuration. Grounded points are never free. An empty result on a solved
// sketch means it is fully constrained (DOF 0); this is the engine-level
// answer to "which geometry would a GUI color as under-constrained".
//
// On non-finite geometry (see [Sketch.DOF]'s doc comment for why) the null-
// space analysis this method otherwise depends on cannot be trusted, so —
// like DOF — it answers with MAXIMUM IGNORANCE rather than refuse: every point
// of the sketch, grounded ones included, mirroring DOF's total variable count
// exactly.
//
// Excluding the grounded points is NOT that answer, for the same reason DOF
// does not count free variables there: [Sketch.Fix] IS a constraint, so a
// value that credits it collapses to empty on an all-grounded sketch — the
// reading a caller gating on len(FreePoints())==0 takes as fully constrained,
// on exactly the fixture this screen exists for. The sketch's own origin is
// still excluded, as it is on the analysed path: it is not in s.points, and no
// public call can move it. This is a deliberate design choice, not an
// oversight: call [Sketch.Verify] to learn that the condition was found at all.
func (s *Sketch) FreePoints() []*Point {
	movable, analysed := s.movableVars()
	if !analysed {
		return slices.Clone(s.points)
	}
	return freePoints(s.points, movable)
}

// freePoints filters points to the ones [pointMovable] reports free, in the
// order given — the shared loop behind [Sketch.FreePoints] and (via
// [Sketch.movableVarsOn]) [Sketch.Verify]'s FreePoints field, so the two
// cannot diverge on how a null-space support is turned into a point list.
func freePoints(points []*Point, movable map[int]struct{}) []*Point {
	var out []*Point
	for _, p := range points {
		if pointMovable(p, movable) {
			out = append(out, p)
		}
	}
	return out
}

// IsFullyConstrained reports whether the point cannot move without violating
// a constraint (or is grounded). It is the per-point view of
// [Sketch.FreePoints].
//
// It reports false for a point this method cannot safely read — nil, the
// externally-constructible zero value (every field of [Point] is unexported,
// but a caller can still write `&Point{}`), or a removed handle — matching
// [Sketch.EntityIsFullyConstrained], the entity-level twin. A var index means
// something only in the sketch that allocated it: a removed point's indices
// were retired (marked fixed, never reclaimed), so an unscreened read would
// answer from variables the sketch no longer tracks as this point's, and a
// removed point would read fully constrained even though its former sketch
// still has free DOF. The refusal is false, the only answer that does not
// certify "this cannot move". Unlike the entity twin, there is no foreign-
// sketch case to screen: this method reads through the point's OWN sketch
// (`p.s`), so a point of another sketch is answered correctly by its owner at
// any index. The screen is `owns` — the same predicate the grounding API,
// `scanReferenceIntegrity` and `checkNoForeignRefs` use — so this cannot
// diverge from what [Sketch.Verify] reports, and it carries the origin
// exception, so [Sketch.Origin] still reads correctly.
//
// It reports false on non-finite geometry too (see [Sketch.DOF]'s doc comment
// for why the null-space analysis cannot be trusted there), which is the same
// MAXIMUM-IGNORANCE stance [Sketch.FreePoints] takes when it reports every
// point of the sketch: the per-point read and the aggregate one then agree
// rather than contradict each other, and neither answers in the blessing
// direction. Call [Sketch.Verify] to learn that the condition was found at all.
func (p *Point) IsFullyConstrained() bool {
	if p == nil || p.s == nil || !p.s.owns(p) {
		return false
	}
	movable, analysed := p.s.movableVars()
	if !analysed {
		return false
	}
	return !pointMovable(p, movable)
}

// EntityIsFullyConstrained reports whether none of an entity's degrees of
// freedom can move within the constraints: its defining points' coordinates and
// whichever intrinsic shape variables entityShapeVars reports for its type (a
// circle's radius being one example; an arc's radius is the derived distance
// from center to start, so it owns none). It is the per-entity companion to
// [Point.IsFullyConstrained] — the answer to "which curve would a GUI color as
// under-constrained".
//
// It reports false for an entity this sketch does not own — nil, a removed
// handle, another sketch's entity, or one of this sketch's entities with a
// defining point rewired to a point this sketch does not own — matching
// [Sketch.EntityFixed], the other bare-bool entity read.
//
// It reports false on non-finite geometry too (see [Sketch.DOF]'s doc comment
// for why the null-space analysis cannot be trusted there), which is the same
// MAXIMUM-IGNORANCE stance [Sketch.FreePoints] and [Point.IsFullyConstrained]
// take: the per-entity read then agrees with the aggregate one rather than
// contradicting it, and neither answers in the blessing direction. Call
// [Sketch.Verify] to learn that the condition was found at all.
func (s *Sketch) EntityIsFullyConstrained(e Entity) bool {
	// A variable index means something only in the sketch that allocated it, so an
	// unscreened handle is read against THIS sketch's null-space support: a foreign
	// entity's indices name unrelated local variables (a genuinely free foreign
	// circle read fully constrained through a DOF-0 sketch), a nil or typed-nil one
	// panics in entityPoints, and a removed one is answered from retired variables.
	// Every one of those errs toward BLESSING a handle the sketch never examined,
	// which is the direction an oracle must never fail in. The screen is
	// foreignInput — the same predicate the grounding API and the modification tools
	// use, so this cannot diverge from what Verify reports — and the refusal is
	// false, the only answer that does not certify "nothing here can move".
	if s.foreignInput(e) {
		return false
	}
	movable, analysed := s.movableVars()
	if !analysed {
		return false
	}
	return !entityMovable(e, movable)
}

// entityMovable reports whether any of an entity's defining-point coordinates or
// intrinsic shape variables is in movable (the Jacobian null-space support).
//
// It is deliberately UNSCREENED: its callers are [Sketch.EntityIsFullyConstrained],
// which screens first, and the DOF colouring in annotate.go, which iterates the
// sketch's own s.ents. Screening here would be dead weight on the render path.
func entityMovable(e Entity, movable map[int]struct{}) bool {
	for _, p := range entityPoints(e) {
		if _, ok := movable[p.xi]; ok {
			return true
		}
		if _, ok := movable[p.yi]; ok {
			return true
		}
	}
	for _, v := range entityShapeVars(e) {
		if _, ok := movable[v.index]; ok {
			return true
		}
	}
	return false
}

// movableVars identifies the free variables with a nonzero component in some
// null-space direction of the constraint Jacobian — the variables a
// constraint-preserving motion can change. Computed by reducing the Jacobian
// to reduced row-echelon form: each non-pivot column seeds a null-space basis
// vector with support on itself and on every pivot column its elimination
// touches.
//
// It is one of the three primitives that CARRY the non-finite-geometry screen
// (see nonfinite.go): the second result is false — and no elimination is run —
// when [Sketch.hasNonFiniteVars] holds, so a caller cannot read a null-space
// support computed from a poisoned Jacobian without handling the refusal. The
// elimination decides its pivots with exactly the comparisons that go the wrong
// way against a non-finite entry, so the support it would report is neither an
// over- nor an under-estimate but simply meaningless.
func (s *Sketch) movableVars() (map[int]struct{}, bool) {
	cj, ok := s.buildCommittedJacobian()
	if !ok {
		return nil, false
	}
	return s.movableVarsOn(cj), true
}

// movableVarsOn runs the same null-space elimination as [Sketch.movableVars]
// over a prebuilt [committedJacobian] — the core [Sketch.Verify] calls
// directly so its FreePoints field shares ONE Jacobian build with the
// rank/DOF, conditioning and conflict analyses of the same call. The matrix is
// cloned first because the elimination mutates it in place, and cj.A may still
// be read by another consumer of the same committedJacobian.
func (s *Sketch) movableVarsOn(cj committedJacobian) map[int]struct{} {
	free := cj.free
	movable := make(map[int]struct{})
	m := cj.m
	if m == 0 {
		for _, vi := range free {
			movable[vi] = struct{}{}
		}
		return movable
	}

	// Null-space support is computed on the NONDIMENSIONAL Jacobian A = Drow·J·Dcol
	// (the same scaled basis as rank/DOF), so the verdict matches DOF and is
	// scale-invariant. Positive diagonal column scaling preserves which variables a
	// null direction touches, so the free-point support is unchanged in exact
	// arithmetic — only the zero threshold becomes scale-invariant.
	J := cloneMatrix(cj.A)
	n := len(free)
	const eps = rankZeroTol
	isPivot := make([]bool, n)
	var pivotCols []int // pivotCols[r] = pivot column of row r
	row := 0
	for col := 0; col < n && row < m; col++ {
		piv := row
		best := pivotAbs(J[row][col])
		for r := row + 1; r < m; r++ {
			if v := pivotAbs(J[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < eps {
			continue
		}
		J[row], J[piv] = J[piv], J[row]
		inv := 1 / J[row][col]
		for c := col; c < n; c++ {
			J[row][c] *= inv
		}
		for r := 0; r < m; r++ {
			if r == row {
				continue
			}
			f := J[r][col]
			if f == 0 {
				continue
			}
			for c := col; c < n; c++ {
				J[r][c] -= f * J[row][c]
			}
		}
		isPivot[col] = true
		pivotCols = append(pivotCols, col)
		row++
	}

	for j := 0; j < n; j++ {
		if isPivot[j] {
			continue
		}
		// Null vector: x[j] = 1, x[pivotCols[r]] = -J[r][j].
		movable[free[j]] = struct{}{}
		for r := 0; r < row; r++ {
			if math.Abs(J[r][j]) > eps {
				movable[free[pivotCols[r]]] = struct{}{}
			}
		}
	}
	return movable
}

// pointMovable reports whether either of a point's coordinates is in movable
// (the Jacobian null-space support) — the point-level companion to
// [entityMovable], and the ONE definition the free-point reads share
// ([Sketch.FreePoints], [Point.IsFullyConstrained], the DOF colouring in
// annotate.go), so a point cannot read free through one and constrained through
// another.
//
// Like entityMovable it is deliberately UNSCREENED: every caller screens the
// handle and the geometry first, through [Sketch.movableVars]'s own second
// return.
func pointMovable(p *Point, movable map[int]struct{}) bool {
	if _, ok := movable[p.xi]; ok {
		return true
	}
	_, ok := movable[p.yi]
	return ok
}
