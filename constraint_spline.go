package sketch

import (
	"math"

	"github.com/lestrrat-3d/sketch/geom"
)

// pointOnSpline confines a point to a cubic B-spline. A B-spline has no implicit
// equation F(x,y)=0 — only the parametric form S(t), t ∈ [0,1] — so membership is
// existential: the constraint owns the foot-point parameter t as an auxiliary
// solver variable (a foot-point search inside the residual would be a
// discontinuous argmin that fights the numerical Jacobian) and the solver moves t
// as part of the same system. t is bounded to [0,1] by a slack-encoded box
// (t = w0², 1−t = w1²) so that out-of-range t is genuinely infeasible rather than
// silently absorbed by Eval's endpoint clamp; w0,w1 are two more aux variables.
//
// Committed residual (4 rows): P.x−S.x(t), P.y−S.y(t), t−w0², (1−t)−w1². A free
// point on a fixed spline then has 5 unknowns (P.x,P.y,t,w0,w1) and 4 independent
// rows — one sliding DOF, as a point on a 1-D curve should.
//
// The aux vars are not serialized; allocVars re-seeds t by foot-point projection
// on load (mirroring the arc-sweep slack). For a self-intersecting or near-self-
// touching spline two foot points can tie, so a reloaded sketch may witness
// membership at a different t than the original solve — still a valid witness
// (residual 0), so solvability is preserved; only the specific t may differ.
//
// [Sketch.CheckConstraint] probes this constraint in its committed form (it
// temporarily allocates the aux vars), so a pre-commit check sees the real rows.
// Note a limitation it shares with any parametric-witness curve constraint: two
// point-on-spline on the same point are redundant only nonlinearly (S(t1)=S(t2)
// ⟹ t1=t2 holds just at the solution), so the local rank analysis is not
// guaranteed to flag the duplicate (it may, when both foot seeds coincide). The
// duplicate is harmless either way — the sketch stays solvable with one sliding
// DOF. (An exact same-point duplicate could be caught by a semantic scan, if a
// guarantee is ever wanted.)
type pointOnSpline struct {
	P            *Point
	Sp           *Spline
	auxOwner         // set by allocVars: the sketch these variable indices belong to
	tvar, w0, w1 int // foot parameter + box slacks; -1 = not yet allocated
}

// NewPointOnSpline forces a point to lie on a cubic B-spline. The point may sit
// anywhere along the curve; the attachment parameter is solved for. (To pin the
// point to an endpoint, make it coincident with the first or last control point,
// which a clamped spline passes through.)
func NewPointOnSpline(p *Point, sp *Spline) Constraint {
	return &pointOnSpline{P: p, Sp: sp, tvar: -1, w0: -1, w1: -1}
}

func (c *pointOnSpline) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent: re-adding the handle must not leak fresh aux vars
	}
	// control-point count is guaranteed >=4 by the Spline constructor (CreateSpline).
	t, _ := geom.NearestParamCubicBSpline(c.Sp.controlCoords(), c.P.x(), c.P.y())
	allocBoxParam(s, t, &c.tvar, &c.w0, &c.w1)
}

func (c *pointOnSpline) retireVars(s *Sketch) {
	retireBoxParam(s, &c.tvar, &c.w0, &c.w1)
}

// pointOnCurveRows appends the four membership+box rows shared by every
// bounded-parameter point-on-curve witness (open spline, fit spline, conic,
// NURBS): the point P=(px,py) lies on the evaluated curve point (sx,sy) — two
// length rows — and the foot parameter t is confined to [0,1] by the slack box
// (t = w0², 1−t = w1²).
func pointOnCurveRows(out []float64, px, py, sx, sy, t, w0, w1 float64) []float64 {
	return append(out,
		px-sx,       // on the curve (x), length units
		py-sy,       // on the curve (y), length units
		t-w0*w0,     // t ≥ 0  (t = w0²)
		(1-t)-w1*w1, // t ≤ 1  (1−t = w1²)
	)
}

// allocBoxParam allocates the bounded foot-parameter trio (the parameter seeded
// to t, plus its two box slacks w0 = √t, w1 = √(1−t)) and stores their indices
// in the supplied pointers. Shared by the bounded point-on-curve witnesses.
func allocBoxParam(s *Sketch, t float64, tvar, w0, w1 *int) {
	*tvar = s.newVar(t)
	*w0 = s.newVar(slackFor(t))     // t = w0²  → w0 = √t
	*w1 = s.newVar(slackFor(1 - t)) // 1−t = w1² → w1 = √(1−t)
}

// retireBoxParam retires the bounded foot-parameter trio and resets the index
// pointers to -1. Shared by the bounded point-on-curve witnesses.
func retireBoxParam(s *Sketch, tvar, w0, w1 *int) {
	if *tvar >= 0 {
		s.retireVar(*tvar)
		s.retireVar(*w0)
		s.retireVar(*w1)
		*tvar, *w0, *w1 = -1, -1, -1
	}
}

func (c *pointOnSpline) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	t := c.s.vars[c.tvar]
	sx, sy := c.Sp.Eval(clamp01(t)) // bound rows below, not the clamp, enforce [0,1]
	return pointOnCurveRows(out, c.P.x(), c.P.y(), sx, sy, t, c.s.vars[c.w0], c.s.vars[c.w1])
}

// clamp01 clamps t to the unit interval.
func clamp01(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// Scale-relative thresholds for spline tangency: a contact parameter whose
// tangent speed |S'(t)| falls below splineEpsTan·scale is treated as a cusp (no
// tangent direction), and a line shorter than splineEpsLine·scale has no
// direction. Both are floored well above the solver tolerance (1e-10).
const (
	splineEpsTan  = 1e-9
	splineEpsLine = 1e-9
)

// tangentToSpline forces a line tangent to a cubic B-spline. Tangency is
// existential over the contact parameter t ∈ [0,1] (the same bounded witness as
// pointOnSpline: t plus the box slacks w0,w1). The committed residual is five
// rows: the contact S(t) on the line's infinite carrier (signed perpendicular
// distance, length units, like tangentLineCircle); the line direction parallel
// to the spline tangent S'(t) (a dimensionless sin-of-angle); the two box rows;
// and a no-cusp guard |S'(t)|/scale ≥ epsTan (slack ws) so the oracle never
// blesses "tangent" where the tangent direction is undefined. S'(t) is the
// analytic geom.EvalCubicBSplineDeriv (a numerical tangent inside the residual
// would be a nested finite difference the Jacobian re-differentiates).
type tangentToSpline struct {
	L                *Line
	Sp               *Spline
	auxOwner             // set by allocVars: the sketch these variable indices belong to
	tvar, w0, w1, ws int // contact parameter, box slacks, speed-guard slack; -1 = unallocated
}

// NewTangentToSpline forces a line tangent to a cubic B-spline. The line is its
// infinite carrier (the contact may fall anywhere along it, as for circles and
// arcs); the contact parameter on the spline is solved for and confined to the
// curve. A line tangent to the spline at more than one parameter witnesses one
// of them; [Sketch.ProbeConfigurations] can surface the alternates.
func NewTangentToSpline(l *Line, sp *Spline) Constraint {
	return &tangentToSpline{L: l, Sp: sp, tvar: -1, w0: -1, w1: -1, ws: -1}
}

func (c *tangentToSpline) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent
	}
	t := c.seedParam()
	spx, spy, _ := geom.EvalCubicBSplineDeriv(c.Sp.controlCoords(), clamp01(t))
	speed := norm(spx, spy)
	c.tvar = s.newVar(t)
	c.w0 = s.newVar(slackFor(t))
	c.w1 = s.newVar(slackFor(1 - t))
	c.ws = s.newVar(slackFor(speed/bboxDiagonal(c.Sp.controlCoords()) - splineEpsTan))
}

func (c *tangentToSpline) retireVars(s *Sketch) {
	if c.tvar >= 0 {
		s.retireVar(c.tvar)
		s.retireVar(c.w0)
		s.retireVar(c.w1)
		s.retireVar(c.ws)
		c.tvar, c.w0, c.w1, c.ws = -1, -1, -1, -1
	}
}

func (c *tangentToSpline) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	tv := c.s.vars[c.tvar]
	t := clamp01(tv)
	sx, sy := c.Sp.Eval(t)
	spx, spy, _ := geom.EvalCubicBSplineDeriv(c.Sp.controlCoords(), t)
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	dlen := norm(dx, dy)
	w0, w1, ws := c.s.vars[c.w0], c.s.vars[c.w1], c.s.vars[c.ws]
	// Scale is recomputed every evaluation, not snapshotted: a free or reshaped
	// spline must use its current size so the scale-relative thresholds stay valid.
	scale := bboxDiagonal(c.Sp.controlCoords())
	return tangentCurveRows(out, sx, sy, spx, spy, ax, ay, dx, dy, dlen, scale, ws, true, tv, w0, w1)
}

// pointOnClosedSpline confines a point to a closed (periodic) cubic B-spline.
// Like [pointOnSpline] the membership is existential — the constraint owns the
// foot parameter t as an aux solver variable — but a periodic loop has NO
// endpoints, so t needs no [0,1] box: it is a single unbounded periodic variable
// (S(t) = S(t+1)), and the committed residual is just the two length rows
// P.x−S.x(t), P.y−S.y(t). A free point on a fixed closed spline then has 3
// unknowns (P.x, P.y, t) and 2 rows — one sliding DOF around the loop, as a point
// on a 1-D curve should. The aux var is not serialized (re-seeded on load by
// foot-point projection). It shares pointOnSpline's nonlinear-redundancy caveat:
// two point-on-closed-spline on the same point are redundant only at the
// solution, so local rank analysis is not guaranteed to flag the duplicate (it
// stays harmless — the sketch keeps its one sliding DOF).
type pointOnClosedSpline struct {
	P        *Point
	Sp       *ClosedSpline
	auxOwner     // set by allocVars: the sketch these variable indices belong to
	tvar     int // foot parameter; -1 = not yet allocated
}

// NewPointOnClosedSpline forces a point to lie on a closed (periodic) cubic
// B-spline. The point may sit anywhere along the loop; the attachment parameter
// is solved for.
func NewPointOnClosedSpline(p *Point, sp *ClosedSpline) Constraint {
	return &pointOnClosedSpline{P: p, Sp: sp, tvar: -1}
}

func (c *pointOnClosedSpline) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent: re-adding the handle must not leak a fresh aux var
	}
	t, _ := geom.NearestParamPeriodicCubicBSpline(c.Sp.controlCoords(), c.P.x(), c.P.y())
	c.tvar = s.newVar(t)
}

func (c *pointOnClosedSpline) retireVars(s *Sketch) {
	if c.tvar >= 0 {
		s.retireVar(c.tvar)
		c.tvar = -1
	}
}

func (c *pointOnClosedSpline) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	sx, sy := c.Sp.Eval(c.s.vars[c.tvar]) // periodic Eval wraps t internally
	return append(out,
		c.P.x()-sx, // on the curve (x), length units
		c.P.y()-sy, // on the curve (y), length units
	)
}

// pointOnFitSpline confines a point to a fit-point (interpolating) spline. The
// curve has endpoints (its first and last fit point), so — exactly like
// [pointOnSpline] — the foot parameter t ∈ [0,1] is a bounded aux variable with a
// slack-encoded box (t = w0², 1−t = w1²) keeping out-of-range t infeasible rather
// than silently clamped. Committed residual (4 rows): P.x−S.x(t), P.y−S.y(t),
// t−w0², (1−t)−w1². The interpolant is recomputed from the fit points each
// evaluation, so the membership tracks them as the solver moves the fit points.
type pointOnFitSpline struct {
	P            *Point
	Sp           *FitSpline
	auxOwner         // set by allocVars: the sketch these variable indices belong to
	tvar, w0, w1 int // foot parameter + box slacks; -1 = not yet allocated
}

// NewPointOnFitSpline forces a point to lie on a fit-point (interpolating)
// spline. The point may sit anywhere along the curve; the attachment parameter is
// solved for. (To pin the point to an endpoint, make it coincident with the first
// or last fit point, which the curve passes through.)
func NewPointOnFitSpline(p *Point, sp *FitSpline) Constraint {
	return &pointOnFitSpline{P: p, Sp: sp, tvar: -1, w0: -1, w1: -1}
}

func (c *pointOnFitSpline) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent
	}
	t, _ := geom.NearestParamFitSpline(c.Sp.fitCoords(), c.P.x(), c.P.y())
	allocBoxParam(s, t, &c.tvar, &c.w0, &c.w1)
}

func (c *pointOnFitSpline) retireVars(s *Sketch) {
	retireBoxParam(s, &c.tvar, &c.w0, &c.w1)
}

func (c *pointOnFitSpline) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	t := c.s.vars[c.tvar]
	sx, sy := c.Sp.Eval(clamp01(t)) // bound rows below, not the clamp, enforce [0,1]
	return pointOnCurveRows(out, c.P.x(), c.P.y(), sx, sy, t, c.s.vars[c.w0], c.s.vars[c.w1])
}

// bboxDiagonal returns the diagonal of the bounding box of every supplied
// coordinate set, floored to 1. It is the scale-relative reference for the
// spline/NURBS-tangency no-cusp and zero-line thresholds and for proximity
// normalization.
func bboxDiagonal(sets ...[][2]float64) float64 {
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, set := range sets {
		for _, p := range set {
			minx, maxx = math.Min(minx, p[0]), math.Max(maxx, p[0])
			miny, maxy = math.Min(miny, p[1]), math.Max(maxy, p[1])
		}
	}
	if d := math.Hypot(maxx-minx, maxy-miny); d > 1 {
		return d
	}
	return 1
}

// seedTangentParam finds a contact parameter for a spline-tangency witness: a
// dense multi-start minimizing the normalized tangency score (contact/scale)² +
// parallel², skipping near-cusp samples, then a golden-section refine. eval/deriv
// evaluate the curve and its tangent at a parameter; for a periodic curve t is
// wrapped into [0,1), otherwise clamped to [0,1]. Shared by the open, closed, and
// fit spline-tangency constraints.
func seedTangentParam(eval, deriv func(float64) (float64, float64), segs int, periodic bool, scale, ax, ay, dx, dy, dlen float64) float64 {
	if segs < 64 {
		segs = 64
	}
	score := func(t float64) float64 {
		sx, sy := eval(t)
		spx, spy := deriv(t)
		speed := norm(spx, spy)
		if speed < splineEpsTan*scale {
			return math.Inf(1) // skip cusps
		}
		h := (dx*(sy-ay) - dy*(sx-ax)) / dlen / scale
		a := (dx*spy - dy*spx) / (dlen * speed)
		return h*h + a*a
	}
	bestT, bestS := 0.0, math.Inf(1)
	for i := 0; i <= segs; i++ {
		t := float64(i) / float64(segs)
		if s := score(t); s < bestS {
			bestS, bestT = s, t
		}
	}
	span := 1.0 / float64(segs)
	lo, hi := bestT-span, bestT+span
	if !periodic {
		lo, hi = math.Max(0, lo), math.Min(1, hi)
	}
	const invphi = 0.6180339887498949
	cP, dP := hi-invphi*(hi-lo), lo+invphi*(hi-lo)
	fc, fd := score(cP), score(dP)
	for k := 0; k < 24; k++ {
		if fc < fd {
			hi, dP, fd = dP, cP, fc
			cP = hi - invphi*(hi-lo)
			fc = score(cP)
		} else {
			lo, cP, fc = cP, dP, fd
			dP = lo + invphi*(hi-lo)
			fd = score(dP)
		}
	}
	t := (lo + hi) / 2
	if periodic {
		return t - math.Floor(t)
	}
	return clamp01(t)
}

// tangentCurveRows appends the rows shared by every line-tangent-to-parametric-
// curve witness: the contact at the evaluated curve point (sx,sy) on the line's
// infinite carrier (signed perpendicular distance, length); the line direction
// (dx,dy) parallel to the curve tangent (spx,spy) (a dimensionless sin-of-angle,
// forced clearly nonzero for a zero-length line); the optional [0,1] box rows on
// the raw parameter tv (present for a bounded curve, absent for a periodic one);
// and the no-cusp guard |C'|/scale ≥ epsTan (slack ws). (ax,ay) is the line
// start, dlen the line length. The box rows, when present, are emitted between
// parallel and the no-cusp guard, matching the original per-curve row order.
func tangentCurveRows(out []float64, sx, sy, spx, spy, ax, ay, dx, dy, dlen, scale, ws float64, box bool, tv, w0, w1 float64) []float64 {
	speed := norm(spx, spy)
	// Contact: signed perpendicular distance from the curve point to the infinite
	// carrier line (length units).
	contact := (dx*(sy-ay) - dy*(sx-ax)) / dlen
	// Parallel: sin of the angle between the line direction and the curve tangent
	// (dimensionless), zero when parallel. A zero-length line has no direction —
	// reject with a clearly-nonzero residual rather than reading 0/0 as tangent.
	parallel := 1.0
	if math.Hypot(dx, dy) >= splineEpsLine*scale {
		parallel = (dx*spy - dy*spx) / (dlen * speed)
	}
	out = append(out,
		contact,  // on the carrier line (length)
		parallel, // line ∥ curve tangent (dimensionless)
	)
	if box {
		out = append(out,
			tv-w0*w0,     // t ≥ 0
			(1-tv)-w1*w1, // t ≤ 1
		)
	}
	return append(out, speed/scale-splineEpsTan-ws*ws) // |C'(t)| ≥ epsTan·scale (no cusp)
}

// tangentToClosedSpline forces a line tangent to a closed (periodic) cubic
// B-spline. Like [tangentToSpline] the contact parameter t is an existential aux
// variable, but the loop has NO endpoints, so t needs no [0,1] box (it is a single
// unbounded periodic variable). Committed residual is THREE rows: the contact S(t)
// on the line's infinite carrier (signed perpendicular distance, length), the line
// direction parallel to the analytic periodic tangent S'(t) (a dimensionless
// sin-of-angle), and a no-cusp guard |S'(t)|/scale ≥ epsTan (slack ws). S'(t) is
// the analytic geom.EvalPeriodicCubicBSplineDeriv.
type tangentToClosedSpline struct {
	L        *Line
	Sp       *ClosedSpline
	auxOwner     // set by allocVars: the sketch these variable indices belong to
	tvar, ws int // contact parameter (periodic, unbounded), speed-guard slack; -1 = unallocated
}

// NewTangentToClosedSpline forces a line tangent to a closed (periodic) cubic
// B-spline. The line is its infinite carrier; the contact parameter on the loop is
// solved for. A line tangent at more than one parameter witnesses one of them
// ([Sketch.ProbeConfigurations] can surface the alternates).
func NewTangentToClosedSpline(l *Line, sp *ClosedSpline) Constraint {
	return &tangentToClosedSpline{L: l, Sp: sp, tvar: -1, ws: -1}
}

func (c *tangentToClosedSpline) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent
	}
	t := c.seedParam()
	// control-point count is guaranteed >=3 by the ClosedSpline constructor.
	spx, spy, _ := geom.EvalPeriodicCubicBSplineDeriv(c.Sp.controlCoords(), t)
	speed := norm(spx, spy)
	c.tvar = s.newVar(t)
	c.ws = s.newVar(slackFor(speed/bboxDiagonal(c.Sp.controlCoords()) - splineEpsTan))
}

func (c *tangentToClosedSpline) retireVars(s *Sketch) {
	if c.tvar >= 0 {
		s.retireVar(c.tvar)
		s.retireVar(c.ws)
		c.tvar, c.ws = -1, -1
	}
}

func (c *tangentToClosedSpline) seedParam() float64 {
	ctrl := c.Sp.controlCoords()
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	eval := func(t float64) (float64, float64) {
		x, y, _ := geom.EvalPeriodicCubicBSpline(ctrl, t)
		return x, y
	}
	deriv := func(t float64) (float64, float64) {
		dx, dy, _ := geom.EvalPeriodicCubicBSplineDeriv(ctrl, t)
		return dx, dy
	}
	return seedTangentParam(eval, deriv, 16*len(ctrl), true, bboxDiagonal(ctrl), ax, ay, dx, dy, norm(dx, dy))
}

func (c *tangentToClosedSpline) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	ctrl := c.Sp.controlCoords()
	t := c.s.vars[c.tvar]
	sx, sy, _ := geom.EvalPeriodicCubicBSpline(ctrl, t)
	spx, spy, _ := geom.EvalPeriodicCubicBSplineDeriv(ctrl, t)
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	dlen := norm(dx, dy)
	ws := c.s.vars[c.ws]
	scale := bboxDiagonal(ctrl)
	return tangentCurveRows(out, sx, sy, spx, spy, ax, ay, dx, dy, dlen, scale, ws, false, 0, 0, 0)
}

// tangentToFitSpline forces a line tangent to a fit-point (interpolating) spline.
// The curve has endpoints, so — exactly like [tangentToSpline] — the contact
// parameter t ∈ [0,1] is bounded by a slack box, and the committed residual is
// five rows: contact on the carrier line (length), parallel to the analytic
// natural-cubic tangent S'(t) (dimensionless, geom.EvalFitSplineDeriv), the two
// box rows, and the no-cusp guard (slack ws).
type tangentToFitSpline struct {
	L                *Line
	Sp               *FitSpline
	auxOwner             // set by allocVars: the sketch these variable indices belong to
	tvar, w0, w1, ws int // contact parameter, box slacks, speed-guard slack; -1 = unallocated
}

// NewTangentToFitSpline forces a line tangent to a fit-point (interpolating)
// spline. The line is its infinite carrier; the contact parameter on the curve is
// solved for and confined to it.
func NewTangentToFitSpline(l *Line, sp *FitSpline) Constraint {
	return &tangentToFitSpline{L: l, Sp: sp, tvar: -1, w0: -1, w1: -1, ws: -1}
}

func (c *tangentToFitSpline) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent
	}
	t := c.seedParam()
	// fit-point count is guaranteed >=2 by the FitSpline constructor.
	spx, spy, _ := geom.EvalFitSplineDeriv(c.Sp.fitCoords(), clamp01(t))
	speed := norm(spx, spy)
	c.tvar = s.newVar(t)
	c.w0 = s.newVar(slackFor(t))
	c.w1 = s.newVar(slackFor(1 - t))
	c.ws = s.newVar(slackFor(speed/bboxDiagonal(c.Sp.fitCoords()) - splineEpsTan))
}

func (c *tangentToFitSpline) retireVars(s *Sketch) {
	if c.tvar >= 0 {
		s.retireVar(c.tvar)
		s.retireVar(c.w0)
		s.retireVar(c.w1)
		s.retireVar(c.ws)
		c.tvar, c.w0, c.w1, c.ws = -1, -1, -1, -1
	}
}

func (c *tangentToFitSpline) seedParam() float64 {
	fit := c.Sp.fitCoords()
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	eval := func(t float64) (float64, float64) {
		x, y, _ := geom.EvalFitSpline(fit, t)
		return x, y
	}
	deriv := func(t float64) (float64, float64) {
		dx, dy, _ := geom.EvalFitSplineDeriv(fit, t)
		return dx, dy
	}
	return seedTangentParam(eval, deriv, 16*len(fit), false, bboxDiagonal(fit), ax, ay, dx, dy, norm(dx, dy))
}

func (c *tangentToFitSpline) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	fit := c.Sp.fitCoords()
	tv := c.s.vars[c.tvar]
	t := clamp01(tv)
	sx, sy, _ := geom.EvalFitSpline(fit, t)
	spx, spy, _ := geom.EvalFitSplineDeriv(fit, t)
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	dlen := norm(dx, dy)
	w0, w1, ws := c.s.vars[c.w0], c.s.vars[c.w1], c.s.vars[c.ws]
	scale := bboxDiagonal(fit)
	return tangentCurveRows(out, sx, sy, spx, spy, ax, ay, dx, dy, dlen, scale, ws, true, tv, w0, w1)
}

// seedParam picks a contact parameter for the tangency witness: a dense
// multi-start over [0,1] minimizing the normalized tangency score
// (contact/scale)² + parallel², skipping near-cusp samples, then a golden-section
// refine. Distance-only or parallelism-only seeds each fail a common case (a
// transverse crossing, or a far-away parallel point), so the combined score is used.
func (c *tangentToSpline) seedParam() float64 {
	ctrl := c.Sp.controlCoords()
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	eval := func(t float64) (float64, float64) {
		x, y, _ := geom.EvalCubicBSpline(ctrl, t)
		return x, y
	}
	deriv := func(t float64) (float64, float64) {
		dx, dy, _ := geom.EvalCubicBSplineDeriv(ctrl, t)
		return dx, dy
	}
	return seedTangentParam(eval, deriv, 16*(len(ctrl)-3), false, bboxDiagonal(c.Sp.controlCoords()), ax, ay, dx, dy, norm(dx, dy))
}

// nurbsControlCoords returns the curve's control-point coordinates at the current
// solved configuration.
func nurbsControlCoords(c *NURBS) [][2]float64 {
	pts := make([][2]float64, len(c.Control))
	for i, p := range c.Control {
		pts[i] = [2]float64{p.x(), p.y()}
	}
	return pts
}

// pointOnNURBS confines a point to a NURBS curve. Like [pointOnConic]/[pointOnSpline]
// a NURBS has only the parametric form C(u), so membership is existential. The
// constraint works in a NORMALIZED foot parameter t ∈ [0,1] (mapped to the knot
// domain by u = lo + t·(hi−lo)), bounded by a slack box (t = w0², 1−t = w1²) so
// out-of-range t is infeasible. Committed residual (4 rows): P.x−C.x, P.y−C.y
// (length), t−w0², (1−t)−w1². A free point on a fixed NURBS then has 5 unknowns
// (P.x,P.y,t,w0,w1) and 4 independent rows — one sliding DOF. The aux vars are not
// serialized; allocVars re-seeds t by foot-point projection on load.
type pointOnNURBS struct {
	P            *Point
	C            *NURBS
	auxOwner         // set by allocVars: the sketch these variable indices belong to
	tvar, w0, w1 int // normalized foot parameter + box slacks; -1 = not yet allocated
}

// NewPointOnNURBS forces a point to lie on a NURBS curve. The point may sit
// anywhere along the curve; the attachment parameter is solved for. (To pin the
// point to an endpoint, make it coincident with the first or last control point,
// which a clamped curve passes through.)
func NewPointOnNURBS(p *Point, c *NURBS) Constraint {
	return &pointOnNURBS{P: p, C: c, tvar: -1, w0: -1, w1: -1}
}

func (c *pointOnNURBS) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent: re-adding the handle must not leak fresh aux vars
	}
	g := c.C.Geometry()
	t := geom.NearestParamNURBS(nurbsControlCoords(c.C), g.Degree, g.Knots, g.Weights, c.P.x(), c.P.y())
	allocBoxParam(s, t, &c.tvar, &c.w0, &c.w1)
}

func (c *pointOnNURBS) retireVars(s *Sketch) {
	retireBoxParam(s, &c.tvar, &c.w0, &c.w1)
}

func (c *pointOnNURBS) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	t := c.s.vars[c.tvar]
	g := c.C.Geometry()
	lo, hi := g.Domain()
	sx, sy := g.Eval(lo + clamp01(t)*(hi-lo)) // bound rows below, not the clamp, enforce [0,1]
	return pointOnCurveRows(out, c.P.x(), c.P.y(), sx, sy, t, c.s.vars[c.w0], c.s.vars[c.w1])
}

// tangentToNURBS forces a line tangent to a NURBS curve. Tangency is existential
// over the NORMALIZED contact parameter t ∈ [0,1] (mapped to the knot domain by
// u = lo + t·(hi−lo)), the same bounded witness as [pointOnNURBS]. The committed
// residual is five rows — exactly like [tangentToSpline]: the contact C(u) on the
// line's infinite carrier (signed perpendicular distance, length); the line
// direction parallel to the analytic NURBS tangent C'(u) (a dimensionless
// sin-of-angle — normalized, so invariant to the hi−lo chain factor); the two box
// rows; and a no-cusp guard |C'(u)|/scale ≥ epsTan (slack ws). C'(u) is the
// analytic geom.NURBS.EvalDeriv (a numerical tangent inside the residual would be
// a nested finite difference the Jacobian re-differentiates).
type tangentToNURBS struct {
	L                *Line
	C                *NURBS
	auxOwner             // set by allocVars: the sketch these variable indices belong to
	tvar, w0, w1, ws int // normalized contact parameter, box slacks, speed-guard slack; -1 = unallocated
}

// NewTangentToNURBS forces a line tangent to a NURBS curve. The line is its
// infinite carrier (the contact may fall anywhere along it); the contact parameter
// on the curve is solved for and confined to it. A line tangent at more than one
// parameter witnesses one of them ([Sketch.ProbeConfigurations] can surface the
// alternates).
func NewTangentToNURBS(l *Line, c *NURBS) Constraint {
	return &tangentToNURBS{L: l, C: c, tvar: -1, w0: -1, w1: -1, ws: -1}
}

func (c *tangentToNURBS) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent
	}
	t := c.seedParam()
	g := c.C.Geometry()
	lo, hi := g.Domain()
	spx, spy := g.EvalDeriv(lo + clamp01(t)*(hi-lo))
	speed := norm(spx, spy)
	c.tvar = s.newVar(t)
	c.w0 = s.newVar(slackFor(t))
	c.w1 = s.newVar(slackFor(1 - t))
	c.ws = s.newVar(slackFor(speed/bboxDiagonal(nurbsControlCoords(c.C)) - splineEpsTan))
}

func (c *tangentToNURBS) retireVars(s *Sketch) {
	if c.tvar >= 0 {
		s.retireVar(c.tvar)
		s.retireVar(c.w0)
		s.retireVar(c.w1)
		s.retireVar(c.ws)
		c.tvar, c.w0, c.w1, c.ws = -1, -1, -1, -1
	}
}

func (c *tangentToNURBS) seedParam() float64 {
	g := c.C.Geometry()
	lo, hi := g.Domain()
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	eval := func(t float64) (float64, float64) { return g.Eval(lo + t*(hi-lo)) }
	deriv := func(t float64) (float64, float64) { return g.EvalDeriv(lo + t*(hi-lo)) }
	return seedTangentParam(eval, deriv, 16*len(c.C.Control), false, bboxDiagonal(nurbsControlCoords(c.C)), ax, ay, dx, dy, norm(dx, dy))
}

func (c *tangentToNURBS) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	tv := c.s.vars[c.tvar]
	t := clamp01(tv)
	g := c.C.Geometry()
	lo, hi := g.Domain()
	u := lo + t*(hi-lo)
	sx, sy := g.Eval(u)
	// The (hi−lo) chain factor in C'(u) cancels in the normalized parallel ratio.
	spx, spy := g.EvalDeriv(u)
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	dlen := norm(dx, dy)
	w0, w1, ws := c.s.vars[c.w0], c.s.vars[c.w1], c.s.vars[c.ws]
	// Scale is recomputed every evaluation, not snapshotted: a free or reshaped
	// curve must use its current size so the scale-relative thresholds stay valid.
	scale := bboxDiagonal(nurbsControlCoords(c.C))
	return tangentCurveRows(out, sx, sy, spx, spy, ax, ay, dx, dy, dlen, scale, ws, true, tv, w0, w1)
}
