package sketch

import (
	"fmt"
	"math"
)

// Non-finite geometry (a NaN or infinite value reaching a solver variable) is
// screened on the GEOMETRY, never on a Jacobian built from it. The rank/DOF/
// conditioning passes all nondimensionalize through the same finite-difference
// pass (scaledJacobian, conditioning.go), which centers every point on the
// centroid positionShift computes over ALL of them: one non-finite coordinate
// makes that centroid non-finite, and the "if d != 0" guard around it passes a
// NaN shift unconditionally, so every OTHER point's residuals are evaluated at
// poisoned coordinates too — the corruption is not local to the offending
// point's own constraints. The partial-pivot elimination that then decides
// rank (rankAnalysisOfMatrix, movableVars) reads a non-finite pivot two
// different ways depending on where it lands: never selected (an undercount,
// since "v > best" is false against a NaN best) or, once selected, never
// rejected either ("best < rankZeroTol" is equally false against a NaN best,
// so it is accepted) — so no rank computed from a poisoned matrix is
// trustworthy in EITHER direction. A Jacobian-level guard also cannot catch
// the sharpest case: a DOF-0 candidate with every point fixed builds a
// perfectly finite, zero-column matrix, and [Sketch.conditioning]'s own
// len(free)==0 shortcut returns +Inf (maximal trust) without ever looking at
// a value. So the screen has to run over s.vars itself, before any Jacobian
// is built from it, and it has to cover every variable regardless of
// [Sketch.fixed] — a FIXED poisoned point still poisons positionShift's
// centroid, and so still corrupts the analysis of every other point's
// constraints.
//
// THE SCREEN IS CARRIED BY THE FOUR PRIMITIVES EVERY VERDICT IN THIS FAMILY
// DERIVES FROM, never by each reader calling it. [Sketch.movableVars]
// (diagnose.go), [Sketch.rank]/[Sketch.committedRankAnalysis] (solver.go),
// [Sketch.conditioning] (conditioning.go) and [Sketch.conflictAnalysis]
// (diagnose.go) each return a final `ok` that is
// false exactly when [Sketch.hasNonFiniteVars] is true, so a new reader must
// ACCOUNT for the screen at the call site instead of never meeting it. Go cannot
// require that a method be called; it can require that a second return be
// handled, which is the same enforcement shape the sealed Entity.isNil uses at
// the addEntity funnel. Its enforcement stops there: a reader may still discard
// the screen with the blank identifier, so this is a chokepoint that makes the
// screen unmissable, NOT a proof that no reader can ignore it. Each caller then
// translates ok==false into ITS OWN non-blessing answer — a refusal where there
// is an error return ([Sketch.CheckConstraint], [Sketch.ProbeConfigurations]),
// the documented not-computed sentinel where there is one ([Result.DOF]), the
// maximum-ignorance value where there is neither ([Sketch.DOF],
// [Sketch.FreePoints], [Sketch.Diagnose], [Sketch.RedundantConstraints]), false
// for the per-handle bools, and every point and
// entity drawn as free for the DOF colouring. Those answers are legitimately
// different and are deliberately NOT unified; what is unified is the fact behind
// them. The one branch that must ask the screen directly is a caller whose rank
// pass does not run at all (no residual rows), since there is no second return
// to carry it there.

// nonFiniteFinding is what [Sketch.nonFiniteVars] found: the points, entities,
// dimension constraints and aux-variable-owning constraints whose solver
// variable or driving value is not a finite number. The zero value reports
// nothing was found ([nonFiniteFinding.found]).
type nonFiniteFinding struct {
	points   []*Point
	entities []Entity
	dims     []Dimension
	cons     []Constraint // committed constraints holding a non-finite aux var
}

// found reports whether the scan found anything.
func (f nonFiniteFinding) found() bool {
	return len(f.points) > 0 || len(f.entities) > 0 || len(f.dims) > 0 || len(f.cons) > 0
}

// nonFinite reports whether v is NaN or infinite.
func nonFinite(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }

// nonFiniteVars scans this sketch's OWN geometry — never a Jacobian built from
// it — for a non-finite (NaN or infinite) solver variable, dimension target or
// constraint-owned auxiliary variable, naming every point, entity and
// constraint it finds one on. See the package-level note above for why the
// screen belongs here rather than on the matrix.
//
// Every point's x/y is checked regardless of [Sketch.fixed] (including the
// sketch's own [Sketch.Origin], which can never actually go non-finite through
// the public API, but is checked anyway for the same reason positionShift
// shifts it: consistency with everything else the centroid pass touches).
// Every entity's intrinsic shape variable ([entityShapeVars] — a circle's
// radius, an ellipse's semi-axes and rotation, a conic's rho) is checked the
// same way; a line, an arc and the spline families own none, so they can only
// ever be named through a poisoned defining point.
//
// EVERY dimension's target is checked, DRIVEN ONES INCLUDED, and that is not
// implied by the poisoned-analysis argument above: a driven dimension measures
// the geometry rather than driving it, so residuals() skips it and its target
// never reaches a Jacobian at all. It is scanned because the question this
// screen answers is "does this sketch hold a non-finite value the oracle must
// not bless", not only "is the rank pass poisoned" — and a non-finite driven
// target breaks a stated invariant on its own: [Sketch.MarshalJSON] fails on it
// (encoding/json rejects NaN), the exporters refuse it with
// [ErrNonFiniteGeometry], yet with the target excluded [Sketch.Verify] reported
// the sketch fully constrained and trustworthy — a sketch the report blessed
// and no writer could write. It is also PERMANENT: refreshDriven recomputes the
// measurement as d.base()+r[0], both non-finite, so no solve repairs it.
//
// EVERY committed constraint's LIVE auxiliary variables (a spline foot
// parameter, a tangency slack, a conic-tangency contact witness — see
// [auxVars], the one definition of which var indices those are) are checked
// too, via [Sketch.vars] directly rather than through the points/entities/
// dimensions above: an aux var is seeded FINITE at allocVars time but is then
// a free unknown the solver moves like any other, so it can go non-finite on
// its own account with every authored point, entity and dimension target still
// finite (an extreme-but-finite [ArcLength] target driving its unwrapped-sweep
// theta to NaN is the reported case). A constraint with a poisoned aux var is
// recorded once, in cons, regardless of how many of its own indices are
// affected — the same one-entry-per-offender shape entities already use for
// entityShapeVars.
//
// A constraint's aux indices are read ONLY when [auxOwnerOf] names THIS
// sketch — never for one still unallocated (indices sentinelled at -1) and
// never for one a DIFFERENT sketch allocated. [Sketch.AddConstraint] still
// appends a foreign constraint to s.cons so its own [ErrForeignHandle]
// survives (see the parameter-model note), and that constraint's stored
// indices address the DONOR's, not this sketch's, variable vector — often
// longer, since it was committed there first. Indexing s.vars by them
// unconditionally is exactly the "large index runs off it and panics" failure
// the parameter-model note documents for grounding, and nonFiniteVars is
// called UNCONDITIONALLY by [Sketch.Verify], even on a report already headed
// for the ForeignHandles early-out, so this screen has no reference-integrity
// scan ahead of it to rely on for safety.
//
// The cost of that reach is that the bare reads ([Sketch.DOF] and its siblings)
// answer with maximum ignorance, and Verify skips its analysis, for a sketch
// whose rank pass would in fact have been sound. That is the conservative
// direction and the one every other member of this family already takes; the
// alternative — a second, narrower notion of "non-finite" — would give one fact
// two answers, which is the defect this screen exists to remove.
// [Sketch.CheckConstraint] keeps its own driven-CANDIDATE exemption, which is a
// different question: an uncommitted driven candidate is never ranked, so no
// verdict about IT is computed from this sketch's geometry.
func (s *Sketch) nonFiniteVars() nonFiniteFinding {
	var f nonFiniteFinding
	for _, p := range s.points {
		if nonFinite(s.vars[p.xi]) || nonFinite(s.vars[p.yi]) {
			f.points = append(f.points, p)
		}
	}
	if s.origin != nil && (nonFinite(s.vars[s.origin.xi]) || nonFinite(s.vars[s.origin.yi])) {
		f.points = append(f.points, s.origin)
	}
	for _, e := range s.ents {
		for _, v := range entityShapeVars(e) {
			if nonFinite(s.vars[v.index]) {
				f.entities = append(f.entities, e)
				break
			}
		}
	}
	for _, c := range s.cons {
		if d, ok := c.(Dimension); ok && nonFinite(d.base()) {
			f.dims = append(f.dims, d)
		}
		if auxOwnerOf(c) != s {
			continue // unallocated, or allocated by a DIFFERENT sketch — see below
		}
		idx, n := auxVars(c)
		for i := 0; i < n; i++ {
			if nonFinite(s.vars[idx[i]]) {
				f.cons = append(f.cons, c)
				break
			}
		}
	}
	return f
}

// hasNonFiniteVars is the cheap boolean form of [Sketch.nonFiniteVars] and the
// screen the four analysis primitives ([Sketch.movableVars], [Sketch.rank] and
// its committed analysis, [Sketch.conditioning], [Sketch.conflictAnalysis])
// carry in their final return.
// It answers the same question over the same four sources in the same order,
// but allocates nothing and stops at the first non-finite value, so a per-handle
// read pays a scan of the sketch's points, entities and constraints and no
// heap traffic — a cost strictly dominated, in every caller, by the Jacobian
// rebuild the same call performs. [auxVars] is the one that makes this true: it
// hands back a fixed-size array rather than a slice, so walking a committed
// aux-var constraint's indices costs no heap allocation even on the ordinary,
// entirely finite path this method runs on every rank/DOF/conditioning call.
//
// It must stay in step with nonFiniteVars: the two are one fact, and a caller
// that gets false here and a finding there would see a primitive report ok while
// [Sketch.Verify] reports the geometry non-finite.
func (s *Sketch) hasNonFiniteVars() bool {
	for _, p := range s.points {
		if nonFinite(s.vars[p.xi]) || nonFinite(s.vars[p.yi]) {
			return true
		}
	}
	if s.origin != nil && (nonFinite(s.vars[s.origin.xi]) || nonFinite(s.vars[s.origin.yi])) {
		return true
	}
	for _, e := range s.ents {
		for _, v := range entityShapeVars(e) {
			if nonFinite(s.vars[v.index]) {
				return true
			}
		}
	}
	for _, c := range s.cons {
		if d, ok := c.(Dimension); ok && nonFinite(d.base()) {
			return true
		}
		if auxOwnerOf(c) != s {
			continue // unallocated, or allocated by a DIFFERENT sketch — see below
		}
		idx, n := auxVars(c)
		for i := 0; i < n; i++ {
			if nonFinite(s.vars[idx[i]]) {
				return true
			}
		}
	}
	return false
}

// maxAuxVars is the largest number of auxiliary indices any single
// aux-var-owning constraint type currently declares ([tangentConics]: px, py,
// wSide, slackA, slackB). [auxVars] sizes its fixed-size return array from
// it, so a new type needing more than this many indices must grow the
// constant too — the array is the ONLY place those indices are collected, so
// a stale constant silently drops the overflow index rather than failing loud.
const maxAuxVars = 5

// auxVars writes the auxiliary solver-variable indices c currently holds into
// idx and returns the count filled, n — 0 when c owns none, or owns some but
// none are allocated yet (every index field sentinelled at -1 until allocVars
// runs). It is the ONE definition of "which s.vars indices does this
// constraint's aux state span", read by
// [Sketch.nonFiniteVars]/[Sketch.hasNonFiniteVars] so a poisoned aux variable
// is screened wherever in s.vars it lives, mirroring [entityShapeVars] for
// entities' intrinsic shape variables. It reads only the int index fields
// every aux-var-owning type declares beside its embedded auxOwner — never an
// operand pointer — so it is safe to call on a constraint holding a nil or
// foreign operand.
//
// idx is a fixed-size ARRAY, not a slice, and n is the number of its leading
// entries that are valid: a caller ranges `for i := 0; i < n; i++`, never over
// idx itself, since idx[n:] holds stale zeros from a shorter-lived case. This
// is what keeps the call allocation-free — [Sketch.hasNonFiniteVars] makes one
// call per committed aux-var-owning constraint on every rank/DOF/conditioning
// read, on the ordinary, entirely finite path those reads run on far more
// often than the poisoned one, so a heap slice here was a per-constraint,
// per-call allocation with nothing wrong to show for it.
//
// The returned indices are meaningful only in the sketch [auxOwnerOf] names as
// the allocator: this function does not check that itself, so a caller MUST
// screen with auxOwnerOf first, exactly as [Sketch.nonFiniteVars] does — a
// foreign constraint's indices address a DIFFERENT sketch's (often longer)
// variable vector, and indexing this sketch's s.vars by them unconditionally
// runs off the end of it.
//
// A new aux-var-owning constraint type MUST get a case here, or its variable
// escapes this screen with the build, vet, lint and test gates all green — the
// same failure shape entityShapeVars documents for a forgotten entity type.
func auxVars(c Constraint) (idx [maxAuxVars]int, n int) {
	add := func(v int) {
		if v >= 0 {
			idx[n] = v
			n++
		}
	}
	switch t := c.(type) {
	case *pointOnArc:
		add(t.slack)
	case *pointOnEllipticalArc:
		add(t.slack)
	case *pointOnSpline:
		add(t.tvar)
		add(t.w0)
		add(t.w1)
	case *tangentToSpline:
		add(t.tvar)
		add(t.w0)
		add(t.w1)
		add(t.ws)
	case *pointOnClosedSpline:
		add(t.tvar)
	case *pointOnFitSpline:
		add(t.tvar)
		add(t.w0)
		add(t.w1)
	case *tangentToClosedSpline:
		add(t.tvar)
		add(t.ws)
	case *tangentToFitSpline:
		add(t.tvar)
		add(t.w0)
		add(t.w1)
		add(t.ws)
	case *pointOnConic:
		add(t.tvar)
		add(t.w0)
		add(t.w1)
	case *tangentToConic:
		add(t.tvar)
		add(t.w0)
		add(t.w1)
		add(t.ws)
	case *pointOnNURBS:
		add(t.tvar)
		add(t.w0)
		add(t.w1)
	case *tangentToNURBS:
		add(t.tvar)
		add(t.w0)
		add(t.w1)
		add(t.ws)
	case *tangentConics:
		add(t.px)
		add(t.py)
		add(t.wSide)
		add(t.slackA)
		add(t.slackB)
	case *symmetricArcs:
		add(t.slack)
	case *tangentLineCircle:
		add(t.slack)
	case *tangentCircles:
		add(t.slack1)
		add(t.slack2)
	case *tangentLineEllipse:
		add(t.slack)
	case *DistancePointArc:
		add(t.slack)
	case *DistanceLineArc:
		add(t.slack)
	case *ArcLength:
		add(t.theta)
	}
	return
}

// nonFiniteError is the refusal the two calls that CAN refuse return —
// [Sketch.CheckConstraint] and [Sketch.ProbeConfigurations] — naming what the
// scan found. One builder so the two cannot describe the same finding
// differently.
func (s *Sketch) nonFiniteError() error {
	nf := s.nonFiniteVars()
	return fmt.Errorf("%w: %d points, %d entities, %d dimensions, %d constraints",
		ErrNonFiniteGeometry, len(nf.points), len(nf.entities), len(nf.dims), len(nf.cons))
}
