package sketch

import (
	"math"
	"slices"
)

// The local residual Jacobian.
//
// [Sketch.jacobianInto] is the DEFINITION of the numerical Jacobian: for each
// free variable it perturbs that variable and reevaluates the WHOLE residual
// vector twice. Most of that work is wasted. A constraint's residual reads only
// the variables of the geometry it references, so perturbing a variable that
// constraint never reads reproduces its rows bit for bit; the difference the
// dense pass then forms for those rows is an exact zero.
//
// [residualPlan] records which rows each committed constraint owns and which
// constraints each variable reaches, and [Sketch.jacobianLocalInto] uses it to
// reevaluate only the affected constraints per variable. Everything else in the
// column is written as an exact zero, which is what the dense pass computes
// there. The two are BIT-IDENTICAL on the path the plan is used, and the plan
// refuses (returning false, so the caller runs the dense pass) on every input
// where that equality is not provable — see jacobianLocalInto.
//
// The plan is per-CALL state. It is built inside one [Sketch.lm] call, from the
// constraint list and the variable indices as they stand then, and is never
// stored on the Sketch and never reused across public calls — the same rule the
// shared committed Jacobian follows (see committedJacobian in conditioning.go).

// jacobianMode says which Jacobian builder one [Sketch.lm] call may use. It is
// passed EXPLICITLY by each call site rather than inferred from the eval
// callback: a residual plan is only sound for the committed constraint rows in
// residuals() order, and comparing Go function values to recognize that
// evaluator is neither legal nor reliable (a method value is a fresh closure at
// every reference).
type jacobianMode uint8

const (
	// denseJacobian reevaluates the whole residual vector per variable. It is
	// the default and the only mode valid for an arbitrary eval callback —
	// goal-augmented systems ([Sketch.goalResiduals]) included.
	denseJacobian jacobianMode = iota
	// localJacobian permits the residual plan. A call site may pass it ONLY
	// when its eval callback is [Sketch.residuals] itself, evaluating this
	// sketch's committed constraints in s.cons order with the driven-dimension
	// skip. Passing it for any other callback would attribute rows to the wrong
	// constraints.
	localJacobian
)

// residualPlan is the row inventory and variable→constraint index one
// [Sketch.lm] call uses to evaluate only the residual rows a perturbation can
// move. It describes [Sketch.residuals] at the configuration it was built at:
// cons holds the participating constraints in that iteration order (driven
// dimensions skipped, exactly as residuals() skips them), and cons[k] owns rows
// offset[k]..offset[k]+count[k].
type residualPlan struct {
	cons   []Constraint
	offset []int
	count  []int
	m      int // total rows, == the row count residuals() produced at build time

	// varCons is a compressed index over the sketch's variable vector: the
	// positions in cons that variable vi reaches are
	// varCons[varStart[vi]:varStart[vi+1]], ascending, each listed once.
	varStart []int
	varCons  []int

	// rp and rm are full-length row scratch, reused across variables and
	// iterations. Only the rows of the constraints a variable reaches are
	// written on any one pass, so the rest hold values from an earlier variable
	// — which is why the derivative loop reads only those same rows.
	rp, rm []float64
}

// consFor returns the ascending positions in pl.cons whose constraint reads
// variable vi.
func (pl *residualPlan) consFor(vi int) []int {
	if vi < 0 || vi+1 >= len(pl.varStart) {
		return nil
	}
	return pl.varCons[pl.varStart[vi]:pl.varStart[vi+1]]
}

// evalRows evaluates the constraints at positions ks into dst at their recorded
// row offsets, and reports whether every one produced exactly the row count the
// plan recorded. Each constraint appends into a sub-slice whose CAPACITY is its
// recorded count, so a constraint producing more rows than recorded appends into
// a fresh array of its own instead of overwriting the next constraint's rows;
// the length check then catches it and the caller falls back to the dense pass.
func (pl *residualPlan) evalRows(dst []float64, ks []int) bool {
	for _, k := range ks {
		off, cnt := pl.offset[k], pl.count[k]
		if len(pl.cons[k].residual(dst[off:off:off+cnt])) != cnt {
			return false
		}
	}
	return true
}

// constraintVarIndices appends to out every solver-variable index constraint c's
// residual can read, and reports whether that set is COMPLETE and this sketch's
// to read. It is the dependency inventory the residual plan rests on, and it is
// assembled from the definitions that already own each half of the question
// rather than from a table of its own: [constraintRefs] for the operands a
// constraint names (removal.go — the same switch the removal cascade walks, so a
// new constraint type cannot reach here unlisted), [entityPoints] and
// [entityShapeVars] for the variables an entity operand owns (sketch.go — the
// single definitions of both), and [auxVars] for the constraint's own auxiliary
// unknowns (nonfinite.go — the single definition of those). Every residual in
// constraint.go reads its variables through exactly these: it reaches point
// coordinates and entity shape variables through its operand handles, and reads
// s.vars directly ONLY at its own aux indices.
//
// It reports FALSE — and the caller then uses the dense Jacobian, which needs no
// dependency information at all — rather than guessing, on each way the set
// could be wrong:
//
//   - A constraint naming NOTHING. [constraintRefs] returns (nil, nil) both for
//     a type that has no case in its switch (a type added without one, the
//     failure that would otherwise hand the plan an empty dependency set and
//     silently zero every entry of a real row) and for a nil interface value.
//     Every constraint the package defines names at least one operand.
//   - An operand this sketch does not own — nil, typed-nil, removed, or another
//     sketch's — screened with [Sketch.owns] / [Sketch.ownsEntity], the same
//     predicates scanReferenceIntegrity and foreignInput use. Its indices
//     address a different vector, or none.
//   - Auxiliary variables ANOTHER sketch allocated ([auxOwnerOf] naming
//     something other than s while [auxVars] reports live indices). That is the
//     rewired-operand state the parameter model documents: the operands read
//     local while the stored aux indices still address the donor's vector.
func (s *Sketch) constraintVarIndices(c Constraint, out []int) ([]int, bool) {
	pts, ents := constraintRefs(c)
	if len(pts) == 0 && len(ents) == 0 {
		return out, false
	}
	for _, p := range pts {
		if !s.owns(p) {
			return out, false
		}
		out = append(out, p.xi, p.yi)
	}
	for _, e := range ents {
		if !s.ownsEntity(e) {
			return out, false
		}
		for _, p := range entityPoints(e) {
			if !s.owns(p) {
				return out, false
			}
			out = append(out, p.xi, p.yi)
		}
		for _, v := range entityShapeVars(e) {
			out = append(out, v.index)
		}
	}
	idx, n := auxVars(c)
	if n > 0 {
		if auxOwnerOf(c) != s {
			return out, false
		}
		for i := 0; i < n; i++ {
			out = append(out, idx[i])
		}
	}
	return out, true
}

// newResidualPlan builds the residual plan for [Sketch.residuals] at the current
// configuration, or reports false when any committed constraint's dependency set
// cannot be determined (see [Sketch.constraintVarIndices]) — a whole-plan
// refusal, so the caller's Jacobian is the dense one and no partial plan has to
// be reconciled with it.
//
// The row inventory is MEASURED, never predicted: each participating constraint
// is evaluated once, in the same order and with the same driven-dimension skip
// residuals() uses, and its rows are recorded from what it actually produced.
// That is one extra residual pass per plan build, against the 2·n passes one
// dense Jacobian costs, and it keeps the plan from carrying a second copy of
// each constraint's row structure that could drift from residual().
//
// rows is the caller's residual row count, used only to size the row buffers
// ahead of the pass; a plan whose own measured count differs from it is still
// built, and jacobianLocalInto refuses it. The build is kept allocation-lean —
// every buffer is sized once from the constraint and variable counts, the
// per-constraint variable list is sorted and deduplicated IN PLACE, and the
// occurrence counts double as the fill cursors — because it is paid once per
// [Sketch.lm] call, including on the small sketches where one dense Jacobian is
// only a few microseconds and a careless plan would cost more than it saves.
func (s *Sketch) newResidualPlan(rows int) (*residualPlan, bool) {
	nc := len(s.cons)
	pl := &residualPlan{
		cons:   make([]Constraint, 0, nc),
		offset: make([]int, 0, nc),
		count:  make([]int, 0, nc),
	}
	counts := make([]int, len(s.vars))
	// flat holds each constraint's deduplicated variable indices, concatenated;
	// flatStart[k] is where cons[k]'s run begins. Four indices per constraint is
	// the commonest shape (two points), so it is the growth seed.
	flat := make([]int, 0, 4*nc)
	flatStart := make([]int, 0, nc+1)
	buf := make([]float64, 0, rows)
	for _, c := range s.cons {
		if d, ok := c.(Dimension); ok && d.Driven() {
			continue // mirrors residuals(): a driven dimension contributes no row
		}
		start := len(flat)
		vs, ok := s.constraintVarIndices(c, flat)
		if !ok {
			return nil, false
		}
		// One entry per variable: a constraint naming the same variable twice
		// (a zero-length line, a distance from a point to itself) would
		// otherwise have its rows evaluated and written twice for that column.
		flat = vs
		slices.Sort(flat[start:])
		flat = flat[:start+len(slices.Compact(flat[start:]))]
		for _, vi := range flat[start:] {
			if vi < 0 || vi >= len(counts) {
				return nil, false // an index outside this sketch's vector
			}
			counts[vi]++
		}

		before := len(buf)
		buf = c.residual(buf)
		pl.cons = append(pl.cons, c)
		pl.offset = append(pl.offset, before)
		pl.count = append(pl.count, len(buf)-before)
		flatStart = append(flatStart, start)
	}
	flatStart = append(flatStart, len(flat))
	pl.m = len(buf)

	// counts becomes the per-variable fill cursor once the row starts are known,
	// so the compressed index needs no second array to build.
	pl.varStart = make([]int, len(counts)+1)
	sum := 0
	for i, n := range counts {
		pl.varStart[i] = sum
		counts[i] = sum
		sum += n
	}
	pl.varStart[len(counts)] = sum
	pl.varCons = make([]int, sum)
	for k := range pl.cons {
		for _, vi := range flat[flatStart[k]:flatStart[k+1]] {
			pl.varCons[counts[vi]] = k
			counts[vi]++
		}
	}

	pl.rp = make([]float64, pl.m)
	pl.rm = make([]float64, pl.m)
	return pl, true
}

// jacobianLocalInto fills the m×n matrix J with the same Jacobian
// [Sketch.jacobianInto] computes for [Sketch.residuals], reevaluating only the
// constraints each perturbed variable reaches, and reports whether it did. On
// false J may hold partial values and the caller MUST run the dense pass, which
// overwrites every entry.
//
// base is the residual vector at the CURRENT configuration — the value the
// caller already holds, not one this function evaluates.
//
// # Why the result is bit-identical
//
// The dense pass writes J[i][j] = (rp[i] − rm[i])·inv for every row i. For a row
// whose constraint does not read variable j, rp[i] and rm[i] are computed from
// identical inputs by identical code, so they are the same float64 and their
// difference is +0; inv is positive (h ≥ 1e-7 > 0), or +0 when h overflows, and
// +0 times either is +0. So the dense pass writes exactly +0 into every entry
// this one leaves cleared, and it evaluates every affected row with the same
// arithmetic on the same values as the dense pass does — each constraint's
// residual is a function of its own inputs alone, and writing its rows at a
// different position in a buffer changes none of them.
//
// That argument needs the unaffected rows to be FINITE, and the base scan below
// is what establishes it: an unaffected row holds its base value at both
// perturbations, so a finite base value stays finite. Where it does not, the
// dense pass computes ∞ − ∞ or NaN − NaN — NaN, not zero — and clearing the
// entry would REPLACE a NaN that a structurally independent row genuinely
// produces. One non-finite base residual therefore costs the whole call its
// plan, not just that row.
//
// The step size, the variable order, the row order, the ±h evaluation order and
// the exact restoration of the original bit pattern are all the dense pass's,
// unchanged.
func (s *Sketch) jacobianLocalInto(J [][]float64, free []int, m int, pl *residualPlan, base []float64) bool {
	if pl == nil || pl.m != m || len(base) != m {
		return false
	}
	for _, v := range base {
		if nonFinite(v) {
			return false
		}
	}
	for i := 0; i < m; i++ {
		clear(J[i])
	}
	for j, vi := range free {
		ks := pl.consFor(vi)
		if len(ks) == 0 {
			continue // no residual row moves with this variable: the column is zero
		}
		orig := s.vars[vi]
		h := 1e-7 * (1 + math.Abs(orig))
		s.vars[vi] = orig + h
		okp := pl.evalRows(pl.rp, ks)
		if okp {
			s.vars[vi] = orig - h
			okp = pl.evalRows(pl.rm, ks)
		}
		// Restore before every exit, the refusal included: a constraint whose
		// row count moved must leave the sketch exactly as the dense pass would.
		s.vars[vi] = orig
		if !okp {
			return false
		}
		inv := 1.0 / (2 * h)
		for _, k := range ks {
			off, cnt := pl.offset[k], pl.count[k]
			for i := off; i < off+cnt; i++ {
				J[i][j] = (pl.rp[i] - pl.rm[i]) * inv
			}
		}
	}
	return true
}
