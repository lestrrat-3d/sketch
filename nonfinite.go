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
// THE SCREEN IS CARRIED BY THE THREE PRIMITIVES EVERY VERDICT IN THIS FAMILY
// DERIVES FROM, never by each reader calling it. [Sketch.movableVars]
// (diagnose.go), [Sketch.rank]/[Sketch.committedRankAnalysis] (solver.go) and
// [Sketch.conditioning] (conditioning.go) each return a second `ok` that is
// false exactly when [Sketch.hasNonFiniteVars] is true, so a reader that ignores
// the screen does not compile. Go cannot require that a method be called; it can
// require that a second return be handled, which is the same enforcement shape
// the sealed Entity.isNil uses at the addEntity funnel. Each caller then
// translates ok==false into ITS OWN non-blessing answer — a refusal where there
// is an error return ([Sketch.CheckConstraint], [Sketch.ProbeConfigurations]),
// the documented not-computed sentinel where there is one ([Result.DOF]), the
// maximum-ignorance value where there is neither ([Sketch.DOF],
// [Sketch.FreePoints]), false for the per-handle bools, and every point and
// entity drawn as free for the DOF colouring. Those answers are legitimately
// different and are deliberately NOT unified; what is unified is the fact behind
// them. The one branch that must ask the screen directly is a caller whose rank
// pass does not run at all (no residual rows), since there is no second return
// to carry it there.

// nonFiniteFinding is what [Sketch.nonFiniteVars] found: the points, entities
// and dimension constraints whose solver variable or driving value is not a
// finite number. The zero value reports nothing was found ([nonFiniteFinding.found]).
type nonFiniteFinding struct {
	points   []*Point
	entities []Entity
	dims     []Dimension
}

// found reports whether the scan found anything.
func (f nonFiniteFinding) found() bool {
	return len(f.points) > 0 || len(f.entities) > 0 || len(f.dims) > 0
}

// nonFinite reports whether v is NaN or infinite.
func nonFinite(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }

// nonFiniteVars scans this sketch's OWN geometry — never a Jacobian built from
// it — for a non-finite (NaN or infinite) solver variable or dimension target,
// naming every point and entity it finds one on. See the package-level note
// above for why the screen belongs here rather than on the matrix.
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
// The cost of that reach is that the bare reads ([Sketch.DOF] and its siblings)
// answer with maximum ignorance, and Verify skips its analysis, for a sketch
// whose rank pass would in fact have been sound. That is the conservative
// direction and the one every other member of this family already takes; the
// alternative — a second, narrower notion of "non-finite" — would give one fact
// two answers, which is the defect this screen exists to remove.
// [Sketch.CheckConstraint] keeps its own driven-CANDIDATE exemption, which is a
// different question: an uncommitted driven candidate is never ranked, so no
// verdict about IT is computed from this sketch's geometry.
//
// Auxiliary constraint variables (a spline foot parameter, a tangency slack,
// a conic-tangency contact witness) are not scanned directly: every one of
// them is seeded from the points/entities/dimensions above, so a screen over
// those three already catches every non-finite value reachable through the
// public API.
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
		d, ok := c.(Dimension)
		if !ok {
			continue
		}
		if nonFinite(d.base()) {
			f.dims = append(f.dims, d)
		}
	}
	return f
}

// hasNonFiniteVars is the cheap boolean form of [Sketch.nonFiniteVars] and the
// screen the three analysis primitives ([Sketch.movableVars], [Sketch.rank] and
// its committed analysis, [Sketch.conditioning]) carry in their second return.
// It answers the same question over the same three sources in the same order,
// but allocates nothing and stops at the first non-finite value, so a per-handle
// read pays a scan of the sketch's points, entities and constraints and no
// heap traffic — a cost strictly dominated, in every caller, by the Jacobian
// rebuild the same call performs.
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
	}
	return false
}

// nonFiniteError is the refusal the two calls that CAN refuse return —
// [Sketch.CheckConstraint] and [Sketch.ProbeConfigurations] — naming what the
// scan found. One builder so the two cannot describe the same finding
// differently.
func (s *Sketch) nonFiniteError() error {
	nf := s.nonFiniteVars()
	return fmt.Errorf("%w: %d points, %d entities, %d dimensions",
		ErrNonFiniteGeometry, len(nf.points), len(nf.entities), len(nf.dims))
}
