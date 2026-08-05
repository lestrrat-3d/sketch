package sketch

import "math"

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
// ever be named through a poisoned defining point. Every non-driven
// dimension's target is checked too — a driven dimension MEASURES the
// geometry rather than driving it, so it contributes no residual row and a
// stale/non-finite target there is not a defect this sketch introduced.
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
		if !ok || d.Driven() {
			continue
		}
		if nonFinite(d.base()) {
			f.dims = append(f.dims, d)
		}
	}
	return f
}

// hasNonFiniteVars is the cheap boolean form of [Sketch.nonFiniteVars], used by
// the bare reads that have no error return and so cannot refuse ([Sketch.DOF],
// [Sketch.FreePoints], [Sketch.Diagnose], [Sketch.RedundantConstraints]) — see
// their doc comments for the deliberate maximum-ignorance answer each gives
// instead.
func (s *Sketch) hasNonFiniteVars() bool { return s.nonFiniteVars().found() }
