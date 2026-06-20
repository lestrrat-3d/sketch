package sketch

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/sketch/units"
)

// Constraint is a geometric or dimensional relationship between primitives.
// Each constraint contributes one or more scalar residual equations that the
// solver drives to zero. Concrete constraint types are unexported, but
// dimensional constraints are returned as exported handles (e.g. [*Distance])
// so their driving value can be edited and the sketch re-solved.
type Constraint interface {
	// residual appends this constraint's residual equations to out.
	residual(out []float64) []float64
}

// internalConstraint marks constraints that are created automatically and
// should not be serialized (they are recreated on load).
type internalConstraint interface{ internal() }

// --- arc consistency (internal) --------------------------------------------

type arcRadius struct{ a *Arc }

func (c *arcRadius) internal() {}
func (c *arcRadius) residual(out []float64) []float64 {
	a := c.a
	return append(out, dist(a.Start, a.Center)-dist(a.End, a.Center)) // length units
}

// --- coincident -------------------------------------------------------------

type coincident struct{ P1, P2 *Point }

func (c *coincident) residual(out []float64) []float64 {
	return append(out, c.P1.x()-c.P2.x(), c.P1.y()-c.P2.y())
}

// NewCoincident forces two points to occupy the same location.
func NewCoincident(p1, p2 *Point) Constraint { return &coincident{p1, p2} }

// --- horizontal / vertical --------------------------------------------------

type horizontal struct{ L *Line }

func (c *horizontal) residual(out []float64) []float64 {
	return append(out, c.L.Start.y()-c.L.End.y())
}

// NewHorizontal forces a line to be horizontal.
func NewHorizontal(l *Line) Constraint { return &horizontal{l} }

type vertical struct{ L *Line }

func (c *vertical) residual(out []float64) []float64 {
	return append(out, c.L.Start.x()-c.L.End.x())
}

// NewVertical forces a line to be vertical.
func NewVertical(l *Line) Constraint { return &vertical{l} }

type horizontalPoints struct{ P1, P2 *Point }

func (c *horizontalPoints) residual(out []float64) []float64 {
	return append(out, c.P1.y()-c.P2.y()) // length units
}

// NewHorizontalPoints forces two points to share a y coordinate (the segment
// between them is horizontal). Unlike [NewHorizontal] it needs no line entity,
// so it applies to bare points or the endpoints of different entities.
func NewHorizontalPoints(p1, p2 *Point) Constraint { return &horizontalPoints{p1, p2} }

type verticalPoints struct{ P1, P2 *Point }

func (c *verticalPoints) residual(out []float64) []float64 {
	return append(out, c.P1.x()-c.P2.x()) // length units
}

// NewVerticalPoints forces two points to share an x coordinate (the segment
// between them is vertical). Unlike [NewVertical] it needs no line entity, so it
// applies to bare points or the endpoints of different entities.
func NewVerticalPoints(p1, p2 *Point) Constraint { return &verticalPoints{p1, p2} }

// --- parallel / perpendicular ----------------------------------------------

type parallel struct{ L1, L2 *Line }

func (c *parallel) residual(out []float64) []float64 {
	d1x, d1y := dir(c.L1)
	d2x, d2y := dir(c.L2)
	// normalized cross product == sin(angle), dimensionless
	return append(out, (d1x*d2y-d1y*d2x)/(norm(d1x, d1y)*norm(d2x, d2y)))
}

// NewParallel forces two lines to be parallel. Relative direction is not
// constrained: antiparallel lines (pointing opposite ways) also satisfy it,
// and the solver keeps whichever the geometry starts closer to.
func NewParallel(l1, l2 *Line) Constraint { return &parallel{l1, l2} }

type perpendicular struct{ L1, L2 *Line }

func (c *perpendicular) residual(out []float64) []float64 {
	d1x, d1y := dir(c.L1)
	d2x, d2y := dir(c.L2)
	// normalized dot product == cos(angle), dimensionless
	return append(out, (d1x*d2x+d1y*d2y)/(norm(d1x, d1y)*norm(d2x, d2y)))
}

// NewPerpendicular forces two lines to be perpendicular.
func NewPerpendicular(l1, l2 *Line) Constraint { return &perpendicular{l1, l2} }

// --- collinear / point-on --------------------------------------------------

type pointOnLine struct {
	P *Point
	L *Line
}

func (c *pointOnLine) residual(out []float64) []float64 {
	// signed perpendicular distance from P to the line (length units)
	ax, ay := c.L.Start.x(), c.L.Start.y()
	abx, aby := c.L.End.x()-ax, c.L.End.y()-ay
	apx, apy := c.P.x()-ax, c.P.y()-ay
	return append(out, (abx*apy-aby*apx)/norm(abx, aby))
}

// NewPointOnLine forces a point to lie on the infinite line through a segment.
func NewPointOnLine(p *Point, l *Line) Constraint { return &pointOnLine{p, l} }

type collinear struct{ L1, L2 *Line }

func (c *collinear) residual(out []float64) []float64 {
	// both endpoints of L2 lie on the infinite line through L1
	out = (&pointOnLine{c.L2.Start, c.L1}).residual(out)
	out = (&pointOnLine{c.L2.End, c.L1}).residual(out)
	return out
}

// NewCollinear forces two lines to share the same infinite line.
func NewCollinear(l1, l2 *Line) Constraint { return &collinear{l1, l2} }

type pointOnCircle struct {
	P *Point
	C *Circle
}

func (c *pointOnCircle) residual(out []float64) []float64 {
	dx := c.P.x() - c.C.Center.x()
	dy := c.P.y() - c.C.Center.y()
	return append(out, norm(dx, dy)-c.C.r()) // length units
}

// NewPointOnCircle forces a point to lie on a circle.
func NewPointOnCircle(p *Point, c *Circle) Constraint { return &pointOnCircle{p, c} }

// pointOnArc confines a point to an arc: on the arc's circle, AND within its
// sweep. The sweep confinement reuses the interior-tangency machinery — a
// slack-encoded inequality keeping the point's direction from the center inside
// the arc's sweep — so a point on the full circle but off the arc is reported
// unsolvable rather than blessed (the same soundness arc tangency enforces).
type pointOnArc struct {
	P     *Point
	A     *Arc
	s     *Sketch // set by allocVars, for slack access
	slack int     // sweep slack var index; -1 = not yet allocated
}

// NewPointOnArc forces a point to lie on an arc — on its circle and within its
// counter-clockwise sweep. A point that lies on the arc's full circle but
// outside the sweep is reported unsolvable, not on the arc.
func NewPointOnArc(p *Point, a *Arc) Constraint { return &pointOnArc{P: p, A: a, slack: -1} }

func (c *pointOnArc) contactDir() (float64, float64) {
	dx := c.P.x() - c.A.Center.x()
	dy := c.P.y() - c.A.Center.y()
	n := norm(dx, dy)
	return dx / n, dy / n
}

func (c *pointOnArc) allocVars(s *Sketch) {
	c.s = s
	if c.slack >= 0 {
		return // idempotent: re-adding the handle must not leak a second slack
	}
	ux, uy := c.contactDir()
	c.slack = s.newVar(slackFor(arcInSweepExcess(c.A, ux, uy)))
}

func (c *pointOnArc) retireVars(s *Sketch) {
	if c.slack >= 0 {
		s.retireVar(c.slack)
		c.slack = -1 // reset so re-adding the handle allocates a fresh slack
	}
}

func (c *pointOnArc) residual(out []float64) []float64 {
	dx := c.P.x() - c.A.Center.x()
	dy := c.P.y() - c.A.Center.y()
	out = append(out, norm(dx, dy)-c.A.R()) // on the circle, length units
	// Gating the sweep row on the slack keeps a committed constraint's arity
	// constant across solver iterations (the finite-difference Jacobian requires
	// it); only a bare residual call before allocVars sees just the on-circle row.
	if c.slack >= 0 {
		ux, uy := c.contactDir()
		w := c.s.vars[c.slack]
		out = append(out, arcInSweepExcess(c.A, ux, uy)-w*w)
	}
	return out
}

// pointOnEllipticalArc confines a point to an elliptical arc: on the arc's
// ellipse (Sampson residual), AND within its eccentric-angle sweep. The sweep
// confinement reuses the interior-tangency slack pattern — a slack-encoded
// inequality keeping the point's eccentric direction inside the sweep — so a
// point on the full ellipse but off the arc is reported unsolvable.
type pointOnEllipticalArc struct {
	P     *Point
	A     *EllipticalArc
	s     *Sketch // set by allocVars, for slack access
	slack int     // sweep slack var index; -1 = not yet allocated
}

// NewPointOnEllipticalArc forces a point to lie on an elliptical arc — on its
// ellipse and within its counter-clockwise eccentric-angle sweep. A point on the
// arc's full ellipse but outside the sweep is reported unsolvable.
func NewPointOnEllipticalArc(p *Point, a *EllipticalArc) Constraint {
	return &pointOnEllipticalArc{P: p, A: a, slack: -1}
}

// eccentricDir returns the unit eccentric direction (cos t, sin t) of the point
// on the arc's ellipse — the local-frame coordinates scaled by 1/rx and 1/ry,
// then normalized. It is the natural-parameter analog of an arc's contactDir.
func (c *pointOnEllipticalArc) eccentricDir() (float64, float64) {
	return eccentricDirXY(c.P.x(), c.P.y(), c.A)
}

// eccentricDirXY is eccentricDir for an arbitrary world point (px,py): the
// point's local-frame coordinates scaled by 1/rx and 1/ry (sign-preserving floor,
// matching geom.EllipticalArc's eccentric() so the in-sweep test agrees with the
// rendered arc even if a semi-axis is negative), then normalized.
func eccentricDirXY(px, py float64, a *EllipticalArc) (float64, float64) {
	cosr, sinr := math.Cos(a.rot()), math.Sin(a.rot())
	dx, dy := px-a.Center.x(), py-a.Center.y()
	lx := cosr*dx + sinr*dy
	ly := -sinr*dx + cosr*dy
	ex, ey := lx/axisFloor(a.rx()), ly/axisFloor(a.ry())
	n := norm(ex, ey)
	return ex / n, ey / n
}

// axisFloor returns v away from zero (preserving sign) so a division by a
// degenerate semi-axis stays finite; mirrors geom's floor for the eccentric
// parametrization.
func axisFloor(v float64) float64 {
	if math.Abs(v) < 1e-12 {
		if v < 0 {
			return -1e-12
		}
		return 1e-12
	}
	return v
}

func (c *pointOnEllipticalArc) allocVars(s *Sketch) {
	c.s = s
	if c.slack >= 0 {
		return // idempotent: re-adding the handle must not leak a second slack
	}
	ux, uy := c.eccentricDir()
	c.slack = s.newVar(slackFor(ellipticalArcSweepExcess(c.A, ux, uy)))
}

func (c *pointOnEllipticalArc) retireVars(s *Sketch) {
	if c.slack >= 0 {
		s.retireVar(c.slack)
		c.slack = -1 // reset so re-adding the handle allocates a fresh slack
	}
}

func (c *pointOnEllipticalArc) residual(out []float64) []float64 {
	a := c.A
	out = append(out, sampsonEllipse(c.P.x(), c.P.y(), a.Center.x(), a.Center.y(), a.rx(), a.ry(), a.rot()))
	// The sweep row is gated on the slack so a committed constraint's arity is
	// constant across solver iterations (as pointOnArc).
	if c.slack >= 0 {
		ux, uy := c.eccentricDir()
		w := c.s.vars[c.slack]
		out = append(out, ellipticalArcSweepExcess(a, ux, uy)-w*w)
	}
	return out
}

// ellipticalArcSweepExcess returns dot(eccentricDir, midDir) − cos(sweep/2) for
// the unit eccentric direction (ux,uy): ≥ 0 exactly when the point's eccentric
// angle lies within the arc's counter-clockwise sweep. midDir is the start
// eccentric direction advanced by half the sweep; the dot test is smooth and
// free of angle-wrap (the same shape as arcInSweepExcess, in eccentric angle).
func ellipticalArcSweepExcess(a *EllipticalArc, ux, uy float64) float64 {
	half := a.Sweep() / 2
	mid := a.StartParam() + half
	return ux*math.Cos(mid) + uy*math.Sin(mid) - math.Cos(half)
}

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
	s            *Sketch // set by allocVars, for aux-var access
	tvar, w0, w1 int     // foot parameter + box slacks; -1 = not yet allocated
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
	t := geom.NearestParamCubicBSpline(c.Sp.controlCoords(), c.P.x(), c.P.y())
	c.tvar = s.newVar(t)
	c.w0 = s.newVar(slackFor(t))     // t = w0²  → w0 = √t
	c.w1 = s.newVar(slackFor(1 - t)) // 1−t = w1² → w1 = √(1−t)
}

func (c *pointOnSpline) retireVars(s *Sketch) {
	if c.tvar >= 0 {
		s.retireVar(c.tvar)
		s.retireVar(c.w0)
		s.retireVar(c.w1)
		c.tvar, c.w0, c.w1 = -1, -1, -1
	}
}

func (c *pointOnSpline) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	t := c.s.vars[c.tvar]
	sx, sy := c.Sp.Eval(clamp01(t)) // bound rows below, not the clamp, enforce [0,1]
	w0, w1 := c.s.vars[c.w0], c.s.vars[c.w1]
	return append(out,
		c.P.x()-sx,  // on the curve (x), length units
		c.P.y()-sy,  // on the curve (y), length units
		t-w0*w0,     // t ≥ 0  (t = w0²)
		(1-t)-w1*w1, // t ≤ 1  (1−t = w1²)
	)
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
	s                *Sketch
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
	spx, spy := geom.EvalCubicBSplineDeriv(c.Sp.controlCoords(), clamp01(t))
	speed := norm(spx, spy)
	c.tvar = s.newVar(t)
	c.w0 = s.newVar(slackFor(t))
	c.w1 = s.newVar(slackFor(1 - t))
	c.ws = s.newVar(slackFor(speed/splineScale(c.Sp) - splineEpsTan))
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
	spx, spy := geom.EvalCubicBSplineDeriv(c.Sp.controlCoords(), t)
	speed := norm(spx, spy)
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	dlen := norm(dx, dy)
	w0, w1, ws := c.s.vars[c.w0], c.s.vars[c.w1], c.s.vars[c.ws]
	// Scale is recomputed every evaluation, not snapshotted: a free or reshaped
	// spline must use its current size so the scale-relative thresholds stay valid.
	scale := splineScale(c.Sp)

	// Contact: signed perpendicular distance from S(t) to the infinite carrier
	// line (length units).
	contact := (dx*(sy-ay) - dy*(sx-ax)) / dlen
	// Parallel: sin of the angle between the line direction and the spline tangent
	// (dimensionless), zero when parallel. A zero-length line has no direction —
	// reject with a clearly-nonzero residual rather than reading 0/0 as tangent.
	parallel := 1.0
	if math.Hypot(dx, dy) >= splineEpsLine*scale {
		parallel = (dx*spy - dy*spx) / (dlen * speed)
	}
	return append(out,
		contact,                        // on the carrier line (length)
		parallel,                       // line ∥ spline tangent (dimensionless)
		tv-w0*w0,                       // t ≥ 0
		(1-tv)-w1*w1,                   // t ≤ 1
		speed/scale-splineEpsTan-ws*ws, // |S'(t)| ≥ epsTan·scale (no cusp)
	)
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
	P    *Point
	Sp   *ClosedSpline
	s    *Sketch // set by allocVars, for aux-var access
	tvar int     // foot parameter; -1 = not yet allocated
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
	t := geom.NearestParamPeriodicCubicBSpline(c.Sp.controlCoords(), c.P.x(), c.P.y())
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
	s            *Sketch // set by allocVars, for aux-var access
	tvar, w0, w1 int     // foot parameter + box slacks; -1 = not yet allocated
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
	t := geom.NearestParamFitSpline(c.Sp.fitCoords(), c.P.x(), c.P.y())
	c.tvar = s.newVar(t)
	c.w0 = s.newVar(slackFor(t))
	c.w1 = s.newVar(slackFor(1 - t))
}

func (c *pointOnFitSpline) retireVars(s *Sketch) {
	if c.tvar >= 0 {
		s.retireVar(c.tvar)
		s.retireVar(c.w0)
		s.retireVar(c.w1)
		c.tvar, c.w0, c.w1 = -1, -1, -1
	}
}

func (c *pointOnFitSpline) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	t := c.s.vars[c.tvar]
	sx, sy := c.Sp.Eval(clamp01(t)) // bound rows below, not the clamp, enforce [0,1]
	w0, w1 := c.s.vars[c.w0], c.s.vars[c.w1]
	return append(out,
		c.P.x()-sx,  // on the curve (x), length units
		c.P.y()-sy,  // on the curve (y), length units
		t-w0*w0,     // t ≥ 0  (t = w0²)
		(1-t)-w1*w1, // t ≤ 1  (1−t = w1²)
	)
}

// splineScale returns a length scale for a spline: its control-box diagonal,
// floored to 1, used to make the no-cusp and zero-line thresholds scale-relative.
func splineScale(sp *Spline) float64 {
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, p := range sp.Control {
		minx, maxx = math.Min(minx, p.x()), math.Max(maxx, p.x())
		miny, maxy = math.Min(miny, p.y()), math.Max(maxy, p.y())
	}
	if d := math.Hypot(maxx-minx, maxy-miny); d > 1 {
		return d
	}
	return 1
}

// seedParam picks a contact parameter for the tangency witness: a dense
// multi-start over [0,1] minimizing the normalized tangency score
// (contact/scale)² + parallel², skipping near-cusp samples, then a golden-section
// refine. Distance-only or parallelism-only seeds each fail a common case (a
// transverse crossing, or a far-away parallel point), so the combined score is used.
func (c *tangentToSpline) seedParam() float64 {
	ctrl := c.Sp.controlCoords()
	n := len(ctrl)
	segs := 16 * (n - 3)
	if segs < 64 {
		segs = 64
	}
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	dlen := norm(dx, dy)
	scale := splineScale(c.Sp)
	score := func(t float64) float64 {
		sx, sy := geom.EvalCubicBSpline(ctrl, t)
		spx, spy := geom.EvalCubicBSplineDeriv(ctrl, t)
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
	lo, hi := math.Max(0, bestT-span), math.Min(1, bestT+span)
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
	return clamp01((lo + hi) / 2)
}

// --- conic-conic tangency ---------------------------------------------------
//
// Two conics (an ellipse with a circle, or two ellipses) have no closed-form
// distance, so tangency is verified by a CONTACT-POINT WITNESS P (two auxiliary
// coordinates): P lies on both curves and their tangent lines coincide there.
// For regular conics this exactly characterizes first-order (G1) tangency — a
// transverse crossing cannot satisfy parallel normals at a shared point.
//
// Committed residual (4 rows): membership_A(P), membership_B(P) (length, zero on
// the curve), cross(n̂_A, n̂_B) (dimensionless, zero when the tangents align), and
// a hard internal/external branch row σ·dot(n̂_A, n̂_B) − wSide² (σ = +1 internal,
// −1 external) — the branch must be an enforced equation, not just a seed, or the
// internal and external constraints would be indistinguishable to the oracle.
// A degenerate conic (zero radius / semi-axis) has no tangent and is rejected.
//
// v1 covers FULL conics only; arc operands (with sweep confinement) and the
// shared-endpoint branch are a recorded follow-up (docs/conic-tangency-design.md).

// conicEps is the floor below which a circle's radius is degenerate (no defined
// tangent). The circle membership |P−C|−R does not floor R, so 1e-9 suffices.
const conicEps = 1e-9

// ellipseAxisEps is the floor below which an ellipse semi-axis is degenerate for
// tangency. It must match the axis² floor (1e-12) that sampsonEllipse and
// ellipseNormalXY apply: below √1e-12 = 1e-6 those residuals silently solve
// against a floored surrogate rather than the authored ellipse, so such an axis
// is rejected as degenerate to keep the policy and the residual consistent.
const ellipseAxisEps = 1e-6

// conic is the operand adapter for tangentConics: a circle, arc, ellipse, or
// elliptical arc. Arc operands also report a sweep-confinement excess so the
// contact is held within the swept portion (a tangent to the underlying full
// conic off the arc is not blessed).
type conic interface {
	ent() Entity
	onResidual(px, py float64) float64          // membership (length, 0 on the curve)
	normalAt(px, py float64) (float64, float64) // outward normal at (px,py), not unit
	degenerate() bool                           // a zero-size conic has no tangent
	boundary(n int) [][2]float64                // n boundary samples (for seeding)
	// sweepExcess returns dot(contactDir, midDir) − cos(sweep/2) at (px,py) and
	// ok=true for an arc operand (≥ 0 exactly when the contact is within the
	// sweep); ok=false for a full conic, which needs no confinement.
	sweepExcess(px, py float64) (float64, bool)
	// endpoints returns an arc operand's boundary points (for the shared-endpoint
	// branch); nil for a full conic.
	endpoints() []*Point
}

type circleConic struct{ c *Circle }

func (a circleConic) ent() Entity { return a.c }
func (a circleConic) onResidual(px, py float64) float64 {
	return norm(px-a.c.Center.x(), py-a.c.Center.y()) - a.c.r()
}
func (a circleConic) normalAt(px, py float64) (float64, float64) {
	return px - a.c.Center.x(), py - a.c.Center.y()
}
func (a circleConic) degenerate() bool                           { return math.Abs(a.c.r()) < conicEps }
func (a circleConic) sweepExcess(px, py float64) (float64, bool) { return 0, false }
func (a circleConic) endpoints() []*Point                        { return nil }
func (a circleConic) boundary(n int) [][2]float64 {
	cx, cy, r := a.c.Center.x(), a.c.Center.y(), a.c.r()
	pts := make([][2]float64, n)
	for i := range pts {
		t := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = [2]float64{cx + r*math.Cos(t), cy + r*math.Sin(t)}
	}
	return pts
}

type ellipseConic struct{ e *Ellipse }

func (a ellipseConic) ent() Entity { return a.e }
func (a ellipseConic) onResidual(px, py float64) float64 {
	return sampsonEllipse(px, py, a.e.Center.x(), a.e.Center.y(), a.e.rx(), a.e.ry(), a.e.rot())
}
func (a ellipseConic) normalAt(px, py float64) (float64, float64) {
	return ellipseNormalXY(px, py, a.e.Center.x(), a.e.Center.y(), a.e.rx(), a.e.ry(), a.e.rot())
}
func (a ellipseConic) degenerate() bool {
	return math.Abs(a.e.rx()) < ellipseAxisEps || math.Abs(a.e.ry()) < ellipseAxisEps
}
func (a ellipseConic) sweepExcess(px, py float64) (float64, bool) { return 0, false }
func (a ellipseConic) endpoints() []*Point                        { return nil }
func (a ellipseConic) boundary(n int) [][2]float64 {
	cx, cy, rx, ry := a.e.Center.x(), a.e.Center.y(), a.e.rx(), a.e.ry()
	cosr, sinr := math.Cos(a.e.rot()), math.Sin(a.e.rot())
	pts := make([][2]float64, n)
	for i := range pts {
		t := 2 * math.Pi * float64(i) / float64(n)
		lx, ly := rx*math.Cos(t), ry*math.Sin(t)
		pts[i] = [2]float64{cx + cosr*lx - sinr*ly, cy + sinr*lx + cosr*ly}
	}
	return pts
}

// arcConic is a circular arc operand: it lies on its circle (same membership and
// normal as circleConic) but confines the contact to the sweep.
type arcConic struct{ a *Arc }

func (x arcConic) ent() Entity { return x.a }
func (x arcConic) onResidual(px, py float64) float64 {
	return norm(px-x.a.Center.x(), py-x.a.Center.y()) - x.a.R()
}
func (x arcConic) normalAt(px, py float64) (float64, float64) {
	return px - x.a.Center.x(), py - x.a.Center.y()
}
func (x arcConic) degenerate() bool { return math.Abs(x.a.R()) < conicEps }
func (x arcConic) sweepExcess(px, py float64) (float64, bool) {
	dx, dy := px-x.a.Center.x(), py-x.a.Center.y()
	n := norm(dx, dy)
	return arcInSweepExcess(x.a, dx/n, dy/n), true
}
func (x arcConic) endpoints() []*Point { return []*Point{x.a.Start, x.a.End} }
func (x arcConic) boundary(n int) [][2]float64 {
	cx, cy, r := x.a.Center.x(), x.a.Center.y(), x.a.R()
	start := math.Atan2(x.a.Start.y()-cy, x.a.Start.x()-cx)
	sweep := x.a.Sweep()
	pts := make([][2]float64, n)
	for i := range pts {
		t := start + sweep*float64(i)/float64(n-1) // span the swept portion, endpoints included
		pts[i] = [2]float64{cx + r*math.Cos(t), cy + r*math.Sin(t)}
	}
	return pts
}

// ellipticalArcConic is an elliptical-arc operand: it lies on its ellipse (same
// Sampson membership and normal as ellipseConic) but confines the contact to the
// eccentric-angle sweep.
type ellipticalArcConic struct{ a *EllipticalArc }

func (x ellipticalArcConic) ent() Entity { return x.a }
func (x ellipticalArcConic) onResidual(px, py float64) float64 {
	return sampsonEllipse(px, py, x.a.Center.x(), x.a.Center.y(), x.a.rx(), x.a.ry(), x.a.rot())
}
func (x ellipticalArcConic) normalAt(px, py float64) (float64, float64) {
	return ellipseNormalXY(px, py, x.a.Center.x(), x.a.Center.y(), x.a.rx(), x.a.ry(), x.a.rot())
}
func (x ellipticalArcConic) degenerate() bool {
	return math.Abs(x.a.rx()) < ellipseAxisEps || math.Abs(x.a.ry()) < ellipseAxisEps
}
func (x ellipticalArcConic) sweepExcess(px, py float64) (float64, bool) {
	ux, uy := eccentricDirXY(px, py, x.a)
	return ellipticalArcSweepExcess(x.a, ux, uy), true
}
func (x ellipticalArcConic) endpoints() []*Point { return []*Point{x.a.Start, x.a.End} }
func (x ellipticalArcConic) boundary(n int) [][2]float64 {
	cx, cy, rx, ry := x.a.Center.x(), x.a.Center.y(), x.a.rx(), x.a.ry()
	cosr, sinr := math.Cos(x.a.rot()), math.Sin(x.a.rot())
	start := x.a.StartParam()
	sweep := x.a.Sweep()
	pts := make([][2]float64, n)
	for i := range pts {
		t := start + sweep*float64(i)/float64(n-1)
		lx, ly := rx*math.Cos(t), ry*math.Sin(t)
		pts[i] = [2]float64{cx + cosr*lx - sinr*ly, cy + sinr*lx + cosr*ly}
	}
	return pts
}

// conicOf wraps a sealed Circular (*Circle/*Arc) or Elliptical (*Ellipse/
// *EllipticalArc) operand in its conic adapter.
func conicOf(e Entity) conic {
	switch t := e.(type) {
	case *Circle:
		return circleConic{t}
	case *Arc:
		return arcConic{t}
	case *Ellipse:
		return ellipseConic{t}
	case *EllipticalArc:
		return ellipticalArcConic{t}
	}
	return nil
}

type tangentConics struct {
	A, B           conic
	Internal       bool
	shared         *Point // shared arc endpoint (shared-endpoint branch); nil otherwise
	s              *Sketch
	px, py         int // contact-witness coordinates; -1 = unallocated (and in shared-endpoint mode)
	wSide          int // internal/external branch slack (allocated in both branches)
	slackA, slackB int // sweep slacks for arc operands; -1 = full conic (no sweep)
}

func newTangentConics(a, b conic, internal bool) *tangentConics {
	return &tangentConics{
		A: a, B: b, Internal: internal, shared: sharedConicEndpoint(a, b),
		px: -1, py: -1, wSide: -1, slackA: -1, slackB: -1,
	}
}

// sharedConicEndpoint returns the boundary point two arc operands share by
// pointer identity (the fillet-corner case), or nil if either is a full conic or
// they share none.
func sharedConicEndpoint(a, b conic) *Point {
	for _, pa := range a.endpoints() {
		for _, pb := range b.endpoints() {
			if pa == pb {
				return pa
			}
		}
	}
	return nil
}

// NewTangentEllipseCircular forces an elliptical entity (an [*Ellipse] or
// [*EllipticalArc]) and a circular entity (a [*Circle] or [*Arc]) to be tangent.
// internal selects an internal contact (one curve inside the other, outward
// normals aligned) versus external (normals opposed); the branch is enforced, not
// merely seeded. For an arc operand the contact is confined to its sweep, so a
// tangent to the underlying full conic off the arc is reported unsolvable. Both
// must be non-degenerate.
func NewTangentEllipseCircular(e Elliptical, c Circular, internal bool) Constraint {
	return newTangentConics(conicOf(e), conicOf(c), internal)
}

// NewTangentEllipses forces two elliptical entities (each an [*Ellipse] or
// [*EllipticalArc]) to be tangent (internal/external and arc-sweep confinement as
// in [NewTangentEllipseCircular]).
func NewTangentEllipses(e1, e2 Elliptical, internal bool) Constraint {
	return newTangentConics(conicOf(e1), conicOf(e2), internal)
}

func (c *tangentConics) sigma() float64 {
	if c.Internal {
		return 1
	}
	return -1
}

func (c *tangentConics) allocVars(s *Sketch) {
	c.s = s
	if c.wSide >= 0 {
		return // idempotent: wSide is allocated in both branches
	}
	if c.shared != nil {
		// Shared-endpoint tangency: the contact is the shared point (on both arcs
		// by construction), so only the internal/external branch slack is needed.
		c.wSide = s.newVar(slackFor(c.sigma() * c.dotNormals(c.shared.x(), c.shared.y())))
		return
	}
	px, py := c.seedContact()
	c.px = s.newVar(px)
	c.py = s.newVar(py)
	c.wSide = s.newVar(slackFor(c.sigma() * c.dotNormals(px, py)))
	if ea, ok := c.A.sweepExcess(px, py); ok {
		c.slackA = s.newVar(slackFor(ea))
	}
	if eb, ok := c.B.sweepExcess(px, py); ok {
		c.slackB = s.newVar(slackFor(eb))
	}
}

func (c *tangentConics) retireVars(s *Sketch) {
	if c.wSide < 0 {
		return
	}
	s.retireVar(c.wSide)
	if c.px >= 0 {
		s.retireVar(c.px)
		s.retireVar(c.py)
	}
	if c.slackA >= 0 {
		s.retireVar(c.slackA)
	}
	if c.slackB >= 0 {
		s.retireVar(c.slackB)
	}
	c.px, c.py, c.wSide, c.slackA, c.slackB = -1, -1, -1, -1, -1
}

// dotNormals returns dot(n̂_A, n̂_B) at (px,py); 0 if either normal is degenerate.
func (c *tangentConics) dotNormals(px, py float64) float64 {
	nax, nay := c.A.normalAt(px, py)
	nbx, nby := c.B.normalAt(px, py)
	la, lb := norm(nax, nay), norm(nbx, nby)
	return (nax*nbx + nay*nby) / (la * lb)
}

func (c *tangentConics) residual(out []float64) []float64 {
	if c.wSide < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	wSide := c.s.vars[c.wSide]
	if c.shared != nil {
		// Shared-endpoint tangency at the shared point S: the two arcs' tangents
		// coincide there (parallel normals) plus the internal/external branch. No
		// membership rows (S is on both curves via their internal arcRadius /
		// ellipticalArcOn constraints) and no sweep rows (an endpoint is in-sweep by
		// definition). A degenerate operand forces the parallel row nonzero.
		sx, sy := c.shared.x(), c.shared.y()
		if c.A.degenerate() || c.B.degenerate() {
			return append(out, 1, c.sigma()-wSide*wSide)
		}
		nax, nay := c.A.normalAt(sx, sy)
		nbx, nby := c.B.normalAt(sx, sy)
		la, lb := norm(nax, nay), norm(nbx, nby)
		parallel := (nax*nby - nay*nbx) / (la * lb)
		dot := (nax*nbx + nay*nby) / (la * lb)
		return append(out, parallel, c.sigma()*dot-wSide*wSide)
	}
	px, py := c.s.vars[c.px], c.s.vars[c.py]
	rA := c.A.onResidual(px, py)
	rB := c.B.onResidual(px, py)
	// A degenerate conic (zero radius / semi-axis) has no tangent direction: keep
	// the membership rows but force the parallel row clearly nonzero so it is never
	// blessed; the row count is unchanged (the sweep rows below still apply).
	if c.A.degenerate() || c.B.degenerate() {
		out = append(out, rA, rB, 1, c.sigma()-wSide*wSide)
	} else {
		nax, nay := c.A.normalAt(px, py)
		nbx, nby := c.B.normalAt(px, py)
		la, lb := norm(nax, nay), norm(nbx, nby)
		parallel := (nax*nby - nay*nbx) / (la * lb) // sin(angle), 0 when tangents align
		dot := (nax*nbx + nay*nby) / (la * lb)
		out = append(out,
			rA,                        // P on A (length)
			rB,                        // P on B (length)
			parallel,                  // tangents parallel (dimensionless)
			c.sigma()*dot-wSide*wSide, // internal/external branch (dimensionless)
		)
	}
	// Sweep rows confine the contact to each arc operand's swept portion. Gating on
	// the slack keeps a committed constraint's arity constant.
	if c.slackA >= 0 {
		ea, _ := c.A.sweepExcess(px, py)
		w := c.s.vars[c.slackA]
		out = append(out, ea-w*w)
	}
	if c.slackB >= 0 {
		eb, _ := c.B.sweepExcess(px, py)
		w := c.s.vars[c.slackB]
		out = append(out, eb-w*w)
	}
	return out
}

// seedContact picks a contact-witness point by branch-aware boundary sampling:
// over sampled boundary points of both conics it minimizes proximity² + cross² and
// penalizes the wrong internal/external side, then returns the midpoint of the best
// pair (balancing the two membership residuals). The solver refines from there.
func (c *tangentConics) seedContact() (float64, float64) {
	sigma := c.sigma()
	sa := c.A.boundary(96)
	sb := c.B.boundary(96)
	scale := boundaryScale(sa, sb)
	bx, by, best := sa[0][0], sa[0][1], math.Inf(1)
	for _, pa := range sa {
		nax, nay := c.A.normalAt(pa[0], pa[1])
		la := norm(nax, nay)
		for _, pb := range sb {
			nbx, nby := c.B.normalAt(pb[0], pb[1])
			lb := norm(nbx, nby)
			cross := (nax*nby - nay*nbx) / (la * lb)
			d := norm(pa[0]-pb[0], pa[1]-pb[1]) / scale
			score := d*d + cross*cross
			if sigma*(nax*nbx+nay*nby)/(la*lb) <= 0 {
				score += 10 // wrong branch: bias away from it
			}
			if score < best {
				best = score
				bx, by = (pa[0]+pb[0])/2, (pa[1]+pb[1])/2
			}
		}
	}
	return bx, by
}

// boundaryScale returns a characteristic length for the two sample sets (the
// diagonal of their combined bounding box, floored to 1) to normalize proximity.
func boundaryScale(sa, sb [][2]float64) float64 {
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, set := range [][][2]float64{sa, sb} {
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

// --- midpoint / symmetric ---------------------------------------------------

type midpoint struct {
	P *Point
	L *Line
}

func (c *midpoint) residual(out []float64) []float64 {
	return append(out,
		c.P.x()-(c.L.Start.x()+c.L.End.x())/2,
		c.P.y()-(c.L.Start.y()+c.L.End.y())/2,
	)
}

// NewMidpoint forces a point to be the midpoint of a line.
func NewMidpoint(p *Point, l *Line) Constraint { return &midpoint{p, l} }

type midpointOf struct{ Mid, P1, P2 *Point }

func (c *midpointOf) residual(out []float64) []float64 {
	return append(out,
		c.Mid.x()-(c.P1.x()+c.P2.x())/2,
		c.Mid.y()-(c.P1.y()+c.P2.y())/2,
	)
}

// NewMidpointOf forces mid to be the midpoint of the segment between p1 and p2.
// Unlike [NewMidpoint] it takes a bare point pair rather than a line entity, so
// it relates points that no single [Line] connects.
func NewMidpointOf(mid, p1, p2 *Point) Constraint { return &midpointOf{mid, p1, p2} }

type symmetric struct {
	P1, P2 *Point
	Axis   *Line
}

func (c *symmetric) residual(out []float64) []float64 {
	ax, ay := c.Axis.Start.x(), c.Axis.Start.y()
	abx, aby := c.Axis.End.x()-ax, c.Axis.End.y()-ay
	// midpoint of P1P2 lies on the axis
	mx := (c.P1.x()+c.P2.x())/2 - ax
	my := (c.P1.y()+c.P2.y())/2 - ay
	axisLen := norm(abx, aby)
	onAxis := (abx*my - aby*mx) / axisLen // midpoint's distance off the axis
	// P1P2 is perpendicular to the axis
	perp := ((c.P2.x()-c.P1.x())*abx + (c.P2.y()-c.P1.y())*aby) / axisLen
	return append(out, onAxis, perp)
}

// NewSymmetric forces two points to be mirror images across an axis line.
// Which point ends up on which side of the axis is not constrained — each
// keeps the side it starts on. The degenerate configuration with both points
// coincident on the axis also satisfies the constraint.
func NewSymmetric(p1, p2 *Point, axis *Line) Constraint { return &symmetric{p1, p2, axis} }

type symmetricLines struct {
	L1, L2 *Line
	Axis   *Line
}

func (c *symmetricLines) residual(out []float64) []float64 {
	// Endpoint-for-endpoint mirror: Start↔Start and End↔End across the axis.
	out = (&symmetric{c.L1.Start, c.L2.Start, c.Axis}).residual(out)
	out = (&symmetric{c.L1.End, c.L2.End, c.Axis}).residual(out)
	return out
}

// NewSymmetricLines forces line l2 to be the mirror image of l1 across the axis,
// matched endpoint-for-endpoint (l1.Start↔l2.Start, l1.End↔l2.End). To mirror
// with the opposite endpoint correspondence, swap l2's endpoints when authoring.
// The axis must be a non-degenerate (non-zero-length) line.
func NewSymmetricLines(l1, l2, axis *Line) Constraint { return &symmetricLines{l1, l2, axis} }

type symmetricCircles struct {
	C1, C2 *Circle
	Axis   *Line
}

func (c *symmetricCircles) residual(out []float64) []float64 {
	// Centers mirror across the axis, and the radii are equal.
	out = (&symmetric{c.C1.Center, c.C2.Center, c.Axis}).residual(out)
	return append(out, c.C1.R()-c.C2.R()) // length units
}

// NewSymmetricCircles forces two circles to be mirror images across the axis:
// their centers are symmetric and their radii equal. The axis must be a
// non-degenerate line.
func NewSymmetricCircles(c1, c2 *Circle, axis *Line) Constraint {
	return &symmetricCircles{c1, c2, axis}
}

// symmetricArcs forces arc a2 to be the mirror image of arc a1 across the axis.
// Reflection reverses orientation, so to keep a2 a valid CCW arc the endpoints
// are SWAPPED: a2.Start mirrors a1.End and a2.End mirrors a1.Start (with the
// centers mirrored straight across). See the residual for the row layout and why
// the second endpoint is pinned via a radial line + branch slack rather than a
// second full point-mirror.
type symmetricArcs struct {
	A1, A2 *Arc
	Axis   *Line
	s      *Sketch // set by allocVars, for slack access
	slack  int     // branch slack var; -1 = unallocated
}

func (c *symmetricArcs) allocVars(s *Sketch) {
	c.s = s
	if c.slack >= 0 {
		return // idempotent
	}
	c.slack = s.newVar(slackFor(c.branchCos()))
}

func (c *symmetricArcs) retireVars(s *Sketch) {
	if c.slack >= 0 {
		s.retireVar(c.slack)
		c.slack = -1 // reset so re-adding the handle allocates a fresh slack
	}
}

// mirror reflects (px,py) across the infinite axis line. A degenerate
// (zero-length) axis is floored to keep the result finite (documented
// precondition: the axis must be non-degenerate).
func (c *symmetricArcs) mirror(px, py float64) (float64, float64) {
	ax, ay := c.Axis.Start.x(), c.Axis.Start.y()
	bx, by := c.Axis.End.x()-ax, c.Axis.End.y()-ay
	n2 := bx*bx + by*by
	if n2 < 1e-24 {
		n2 = 1e-24
	}
	t := ((px-ax)*bx + (py-ay)*by) / n2
	fx, fy := ax+t*bx, ay+t*by // foot of the perpendicular on the axis
	return 2*fx - px, 2*fy - py
}

// branchVecs returns the a2 center→end vector d and the a2 center→T vector r,
// where T = mirror(a1.Start). The remaining endpoint a2.End must coincide with T;
// d and r are the inputs to the collinearity (row 5) and same-ray (row 6) rows.
func (c *symmetricArcs) branchVecs() (float64, float64, float64, float64) {
	tx, ty := c.mirror(c.A1.Start.x(), c.A1.Start.y())
	cx, cy := c.A2.Center.x(), c.A2.Center.y()
	return c.A2.End.x() - cx, c.A2.End.y() - cy, tx - cx, ty - cy
}

// branchCos returns the cosine between d and r — +1 when a2.End is on T's ray
// (the wanted branch), −1 on the antipodal ray. Used to seed the branch slack.
func (c *symmetricArcs) branchCos() float64 {
	dx, dy, rx, ry := c.branchVecs()
	return (dx*rx + dy*ry) / (norm(dx, dy) * norm(rx, ry))
}

func (c *symmetricArcs) residual(out []float64) []float64 {
	// Rows 1-2: the centers mirror across the axis.
	out = (&symmetric{c.A1.Center, c.A2.Center, c.Axis}).residual(out)
	// Rows 3-4: one endpoint fully mirrored, swapped (reflection reverses sweep, so
	// a2.Start is the mirror of a1.End).
	out = (&symmetric{c.A1.End, c.A2.Start, c.Axis}).residual(out)
	// Rows 5-6: the remaining endpoint a2.End must equal T = mirror(a1.Start). A
	// second full point-mirror here would be 1-redundant with the two arcs'
	// internal radius constraints (mirroring is an isometry, so a2's own radius is
	// already implied) and would pollute the redundancy report. Instead pin a2.End
	// onto the radial line through T from a2.Center — its distance comes from a2's
	// arcRadius — and add a slack-encoded branch keeping it on T's ray, not the
	// antipode.
	dx, dy, rx, ry := c.branchVecs()
	nr := norm(rx, ry)
	out = append(out, (dx*ry-dy*rx)/nr) // row 5: collinear with T's ray (length)
	if c.slack < 0 {
		return out // pre-allocVars bare call (e.g. before commit)
	}
	w := c.s.vars[c.slack]
	nd := norm(dx, dy)
	out = append(out, (dx*rx+dy*ry)/(nd*nr)-w*w) // row 6: same ray, not antipode
	return out
}

// NewSymmetricArcs forces arc a2 to be the mirror image of arc a1 across the
// axis. Because a reflection reverses an arc's CCW sweep, the mirror swaps the
// endpoints — a2.Start ends up at the mirror of a1.End and a2.End at the mirror
// of a1.Start — so the mirrored arc sweeps the correct way and matches a1's
// Sweep(). The axis must be a non-degenerate line, the arcs must have nonzero
// radius, and a1 and a2 must be distinct arcs (self-symmetry is a different
// relation). The radii need no explicit equality: it follows from the mirrored
// centers and endpoints together with each arc's intrinsic radius constraint.
//
// Branch selection is local: a2.End is kept on the mirror's ray (not the
// antipodal ray on the same circle) by a slack-encoded row whose gradient is flat
// exactly at the antipode, so a2 seeded precisely on the antipodal configuration
// may stall as unsolvable rather than flip. Seed a2 near its intended mirror.
func NewSymmetricArcs(a1, a2 *Arc, axis *Line) Constraint {
	return &symmetricArcs{A1: a1, A2: a2, Axis: axis, slack: -1}
}

// --- concentric -------------------------------------------------------------

type concentric struct{ C1, C2 Circular }

func (c *concentric) residual(out []float64) []float64 {
	return (&coincident{c.C1.centerPt(), c.C2.centerPt()}).residual(out)
}

// NewConcentric forces two circular entities (circles or arcs) to share a
// center.
func NewConcentric(c1, c2 Circular) Constraint { return &concentric{c1, c2} }

// --- equal ------------------------------------------------------------------

type equalLines struct{ L1, L2 *Line }

func (c *equalLines) residual(out []float64) []float64 {
	return append(out, c.L1.Length()-c.L2.Length()) // length units
}

// NewEqual forces two lines to have equal length.
func NewEqual(l1, l2 *Line) Constraint { return &equalLines{l1, l2} }

type equalRadii struct{ C1, C2 Circular }

func (c *equalRadii) residual(out []float64) []float64 {
	return append(out, c.C1.R()-c.C2.R()) // length units
}

// NewEqualRadius forces two circular entities (circles or arcs) to have equal
// radius.
func NewEqualRadius(c1, c2 Circular) Constraint { return &equalRadii{c1, c2} }

// --- ellipse ----------------------------------------------------------------

type pointOnEllipse struct {
	P *Point
	E *Ellipse
}

func (c *pointOnEllipse) residual(out []float64) []float64 {
	e := c.E
	return append(out, sampsonEllipse(c.P.x(), c.P.y(), e.Center.x(), e.Center.y(), e.rx(), e.ry(), e.rot()))
}

// sampsonEllipse returns the Sampson distance of point (px,py) to the ellipse
// centered at (cx,cy) with semi-axes rx,ry and local-frame rotation rot: the
// |F|/|∇F| first-order approximation of the true point-to-ellipse distance for
// the implicit equation F = (x'/rx)² + (y'/ry)² − 1 in the ellipse's local
// frame. It stays in length units, per the normalization convention.
func sampsonEllipse(px, py, cx, cy, rx, ry, rot float64) float64 {
	cosr, sinr := math.Cos(rot), math.Sin(rot)
	dx, dy := px-cx, py-cy
	lx := cosr*dx + sinr*dy
	ly := -sinr*dx + cosr*dy
	rx2 := math.Max(rx*rx, 1e-12)
	ry2 := math.Max(ry*ry, 1e-12)
	fv := lx*lx/rx2 + ly*ly/ry2 - 1
	return fv / norm(2*lx/rx2, 2*ly/ry2)
}

// NewPointOnEllipse forces a point to lie on an ellipse.
func NewPointOnEllipse(p *Point, e *Ellipse) Constraint { return &pointOnEllipse{p, e} }

// ellipticalArcOn is the internal constraint auto-added (twice) by
// AddEllipticalArc that pins a boundary point onto the arc's ellipse via the
// Sampson residual. It is not serialized — recreated by the constructor on load.
type ellipticalArcOn struct {
	ea *EllipticalArc
	p  *Point
}

func (c *ellipticalArcOn) internal() {}
func (c *ellipticalArcOn) residual(out []float64) []float64 {
	e := c.ea
	return append(out, sampsonEllipse(c.p.x(), c.p.y(), e.Center.x(), e.Center.y(), e.rx(), e.ry(), e.rot()))
}

// --- tangent ----------------------------------------------------------------
//
// Arc tangency confines the contact to the arc's sweep, not its full circle (an
// oracle must not bless a tangent that misses the arc). Two cases:
//
//   - Endpoint tangency — the operands share the contact point (the fillet/slot
//     case): a single clean equality (line ⊥ radius at the shared point, or the
//     centers collinear through it). No auxiliary variable.
//   - Interior tangency — the contact is a determined interior point (the foot
//     of the perpendicular, or the point on the line of centers): the residual
//     pins tangency to the full circle and adds a slack-encoded inequality
//     keeping the contact inside the sweep (dot(contactDir, midDir) ≥
//     cos(sweep/2)). The slack variable is allocated by allocVars when the
//     constraint is committed and retired on removal; it is not serialized
//     (recomputed from the geometry on load).

type tangentLineCircle struct {
	L      *Line
	C      Circular
	shared *Point  // shared contact endpoint (endpoint tangency); nil otherwise
	s      *Sketch // set by allocVars, for slack access
	slack  int     // sweep slack var index; -1 = none (circle or endpoint)
}

// NewTangent forces a line to be tangent to a circular entity (circle or arc).
// The tangency is unsigned: the circle stays on whichever side of the line it
// starts. For an arc the contact point must lie within the arc's sweep — a line
// tangent to the arc's full circle but not touching the arc is reported
// unsolvable. When the line shares an endpoint with the arc, tangency is
// enforced at that shared point.
func NewTangent(l *Line, c Circular) Constraint {
	t := &tangentLineCircle{L: l, C: c, slack: -1}
	if a, ok := c.(*Arc); ok {
		t.shared = sharedPointLineArc(l, a)
	}
	return t
}

func (c *tangentLineCircle) allocVars(s *Sketch) {
	c.s = s
	a, ok := c.C.(*Arc)
	// Idempotent: skip a plain circle / endpoint tangency, or a slack already
	// allocated (re-adding the same handle must not leak a second aux var).
	if !ok || c.shared != nil || c.slack >= 0 {
		return
	}
	ux, uy := lineFootDir(c.L, a.Center)
	c.slack = s.newVar(slackFor(arcInSweepExcess(a, ux, uy)))
}

func (c *tangentLineCircle) retireVars(s *Sketch) {
	if c.slack >= 0 {
		s.retireVar(c.slack)
		c.slack = -1 // reset so re-adding the handle allocates a fresh slack
	}
}

func (c *tangentLineCircle) residual(out []float64) []float64 {
	ctr := c.C.centerPt()
	ax, ay := c.L.Start.x(), c.L.Start.y()
	abx, aby := c.L.End.x()-ax, c.L.End.y()-ay
	ablen := norm(abx, aby)
	r := c.C.R()
	a, isArc := c.C.(*Arc)

	// Endpoint tangency: line perpendicular to the radius at the shared point —
	// cos of the line/radius angle, zero when perpendicular (dimensionless). A
	// degenerate (zero-length) line has no direction and is never tangent.
	if isArc && c.shared != nil {
		if math.Hypot(abx, aby) < 1e-9 {
			return append(out, 1)
		}
		dx, dy := c.shared.x()-ctr.x(), c.shared.y()-ctr.y()
		return append(out, (abx*dx+aby*dy)/(ablen*norm(dx, dy)))
	}

	// signed perpendicular distance from the center to the line
	h := (abx*(ctr.y()-ay) - aby*(ctr.x()-ax)) / ablen
	if !isArc {
		return append(out, math.Abs(h)-r) // circle
	}

	// Interior arc tangency: tangent to the circle (|h|−r, = −r for a degenerate
	// line, so never blessed), plus — once the sweep slack is allocated — the
	// contact within the sweep via the slack-encoded inequality dot(u,m) −
	// cos(half) = w². Gating the sweep row on the slack keeps a committed
	// constraint's arity constant across solver iterations (the finite-difference
	// Jacobian requires it).
	out = append(out, math.Abs(h)-r)
	if c.slack >= 0 {
		ux, uy := lineFootDir(c.L, a.Center)
		w := c.s.vars[c.slack]
		out = append(out, arcInSweepExcess(a, ux, uy)-w*w)
	}
	return out
}

type tangentCircles struct {
	C1, C2   Circular
	Internal bool
	shared   *Point
	s        *Sketch
	slack1   int // sweep slack for C1 if it is an interior-contact arc; -1 else
	slack2   int
}

// NewTangentCircles forces two circular entities (circles or arcs) to be
// tangent. When internal is true they are internally tangent (one inside the
// other — which one is inside is decided by the radii and starting positions,
// not by the constraint); otherwise they are externally tangent. For an arc
// operand the contact point must lie within the arc's sweep — a full-circle
// tangent that does not touch the arc is reported unsolvable. When the two arcs
// share an endpoint, tangency is enforced at that shared point.
func NewTangentCircles(c1, c2 Circular, internal bool) Constraint {
	return &tangentCircles{C1: c1, C2: c2, Internal: internal, shared: sharedPointCirculars(c1, c2), slack1: -1, slack2: -1}
}

func (c *tangentCircles) allocVars(s *Sketch) {
	c.s = s
	if c.shared != nil {
		return // endpoint tangency: collinearity, no slack
	}
	g1x, g1y, g2x, g2y := tangentContactDirs(c.C1, c.C2, c.Internal)
	// Idempotent: only allocate a slack that has not been allocated yet, so
	// re-adding the same handle does not leak a second aux var.
	if a, ok := c.C1.(*Arc); ok && c.slack1 < 0 {
		c.slack1 = s.newVar(slackFor(arcInSweepExcess(a, g1x, g1y)))
	}
	if a, ok := c.C2.(*Arc); ok && c.slack2 < 0 {
		c.slack2 = s.newVar(slackFor(arcInSweepExcess(a, g2x, g2y)))
	}
}

func (c *tangentCircles) retireVars(s *Sketch) {
	if c.slack1 >= 0 {
		s.retireVar(c.slack1)
		c.slack1 = -1 // reset so re-adding the handle allocates a fresh slack
	}
	if c.slack2 >= 0 {
		s.retireVar(c.slack2)
		c.slack2 = -1
	}
}

func (c *tangentCircles) residual(out []float64) []float64 {
	o1, o2 := c.C1.centerPt(), c.C2.centerPt()
	r1, r2 := c.C1.R(), c.C2.R()
	dx, dy := o2.x()-o1.x(), o2.y()-o1.y()
	base := norm(dx, dy) - (r1 + r2)
	if c.Internal {
		base = norm(dx, dy) - math.Abs(r1-r2)
		// Internal tangency needs distinct radii; coincident equal-radius circles
		// can only overlap, never touch at a single point, so keep that residual
		// nonzero rather than reading the degenerate d−0 = 0 as tangent.
		if math.Hypot(dx, dy) < 1e-9 && math.Abs(r1-r2) < 1e-9 {
			base = math.Max(r1, r2)
		}
	}
	out = append(out, base)

	// Endpoint tangency: the shared point is the contact (an arc endpoint, on
	// both circles). The base residual alone — which already honors internal vs
	// external — pins both the tangency and the side there, and the contact is
	// in the sweep by inclusivity, so no sweep slack row is needed.
	if c.shared != nil {
		return out
	}

	// Interior tangency: keep each arc operand's contact within its sweep. The
	// arity is held constant (the contact directions stay finite for concentric
	// centers via norm's floor) so the finite-difference Jacobian never sees a
	// row-count change.
	g1x, g1y, g2x, g2y := tangentContactDirs(c.C1, c.C2, c.Internal)
	if a, ok := c.C1.(*Arc); ok && c.slack1 >= 0 {
		w := c.s.vars[c.slack1]
		out = append(out, arcInSweepExcess(a, g1x, g1y)-w*w)
	}
	if a, ok := c.C2.(*Arc); ok && c.slack2 >= 0 {
		w := c.s.vars[c.slack2]
		out = append(out, arcInSweepExcess(a, g2x, g2y)-w*w)
	}
	return out
}

// arcInSweepExcess returns dot(contactDir, midDir) − cos(sweep/2) for the unit
// contact direction (ux, uy); it is ≥ 0 exactly when the contact lies within
// the arc's counter-clockwise sweep. midDir is the start direction rotated CCW
// by half the sweep; the dot test is smooth and free of angle-wrap.
func arcInSweepExcess(a *Arc, ux, uy float64) float64 {
	cx, cy := a.Center.x(), a.Center.y()
	sl := norm(a.Start.x()-cx, a.Start.y()-cy)
	sxh, syh := (a.Start.x()-cx)/sl, (a.Start.y()-cy)/sl
	half := a.Sweep() / 2
	cosH, sinH := math.Cos(half), math.Sin(half)
	mx := sxh*cosH - syh*sinH
	my := sxh*sinH + syh*cosH
	return ux*mx + uy*my - cosH
}

// slackFor returns the initial slack w for a sweep row. In-sweep (excess > 0) it
// is sqrt(excess), leaving the row satisfied. Out-of-sweep it is the nonzero
// sqrt(|excess|) (floored) rather than 0: seeding w = 0 leaves the row's
// ∂/∂w = −2w = 0, a flat spot that can trap a feasible sketch when other
// constraints later move the contact in-sweep. The solver is free to move w to
// any value from there; this only avoids the degenerate starting point.
func slackFor(excess float64) float64 {
	w := math.Sqrt(math.Abs(excess))
	if w < 1e-3 {
		w = 1e-3
	}
	return w
}

// lineFootDir returns the unit direction from center toward the foot of the
// perpendicular dropped onto the infinite line l — the contact direction a
// line↔arc tangency is judged against. A degenerate (zero-length) line yields
// (0, 0).
func lineFootDir(l *Line, center *Point) (float64, float64) {
	ax, ay := l.Start.x(), l.Start.y()
	abx, aby := l.End.x()-ax, l.End.y()-ay
	ablen := norm(abx, aby)
	h := (abx*(center.y()-ay) - aby*(center.x()-ax)) / ablen
	nx, ny := -aby/ablen, abx/ablen
	if h < 0 {
		return nx, ny
	}
	return -nx, -ny
}

// tangentContactDirs returns the unit contact direction from each circular's
// center along the line of centers: external tangency has the contacts facing
// each other, internal has them on the same side (toward the larger surface).
func tangentContactDirs(c1, c2 Circular, internal bool) (float64, float64, float64, float64) {
	o1, o2 := c1.centerPt(), c2.centerPt()
	dx, dy := o2.x()-o1.x(), o2.y()-o1.y()
	d := norm(dx, dy)
	dirx, diry := dx/d, dy/d
	if internal {
		sgn := 1.0
		if c1.R() < c2.R() {
			sgn = -1
		}
		return sgn * dirx, sgn * diry, sgn * dirx, sgn * diry
	}
	return dirx, diry, -dirx, -diry
}

// sharedPointLineArc returns the point the line and arc share as an endpoint
// (the tangent contact for endpoint tangency), or nil if they share none.
func sharedPointLineArc(l *Line, a *Arc) *Point {
	for _, lp := range []*Point{l.Start, l.End} {
		if lp == a.Start || lp == a.End {
			return lp
		}
	}
	return nil
}

// sharedPointCirculars returns the endpoint two arcs share (nil if either
// operand is a circle, which has no endpoints, or they share none).
func sharedPointCirculars(c1, c2 Circular) *Point {
	a1, ok1 := c1.(*Arc)
	a2, ok2 := c2.(*Arc)
	if !ok1 || !ok2 {
		return nil
	}
	for _, p := range []*Point{a1.Start, a1.End} {
		if p == a2.Start || p == a2.End {
			return p
		}
	}
	return nil
}

// --- tangent to an ellipse ---------------------------------------------------
//
// A line is tangent to a local-frame ellipse x²/rx² + y²/ry² = 1 iff
// (u·rx)² + (v·ry)² = c², where (u, v) is the line's unit normal expressed in
// the ellipse's rotated frame and c the signed center-to-line distance. The
// length-normalized residual is √((u·rx)² + (v·ry)²) − |c| — a closed form, so
// (unlike point-to-ellipse distance) no foot-point iteration is needed. The two
// cases mirror tangentLineCircle:
//
//   - Endpoint tangency — line shares a boundary point with an elliptical arc:
//     the line is ⊥ the ellipse's outward normal at that point. No aux var.
//   - Interior tangency — the contact is the determined tangent point. For an
//     elliptical arc a slack-encoded inequality keeps that contact inside the
//     eccentric-angle sweep, exactly like pointOnEllipticalArc.

type tangentLineEllipse struct {
	L      *Line
	E      Elliptical
	shared *Point  // shared contact endpoint (endpoint tangency); nil otherwise
	s      *Sketch // set by allocVars, for slack access
	slack  int     // sweep slack var index; -1 = none (full ellipse or endpoint)
}

// NewTangentEllipse forces a line to be tangent to an elliptical entity (an
// [*Ellipse] or [*EllipticalArc]). The tangency is unsigned: the ellipse stays
// on whichever side of the line it starts. For an elliptical arc the contact
// point must lie within the arc's eccentric-angle sweep — a line tangent to the
// arc's full ellipse but not touching the swept portion is reported unsolvable.
// When the line shares an endpoint with the arc, tangency is enforced at that
// shared point.
func NewTangentEllipse(l *Line, e Elliptical) Constraint {
	t := &tangentLineEllipse{L: l, E: e, slack: -1}
	if a, ok := e.(*EllipticalArc); ok {
		t.shared = sharedPointLineEllipticalArc(l, a)
	}
	return t
}

func (c *tangentLineEllipse) allocVars(s *Sketch) {
	c.s = s
	a, ok := c.E.(*EllipticalArc)
	// Idempotent: skip a full ellipse / endpoint tangency, or a slack already
	// allocated (re-adding the same handle must not leak a second aux var).
	if !ok || c.shared != nil || c.slack >= 0 {
		return
	}
	ux, uy := c.contactEccentricDir()
	c.slack = s.newVar(slackFor(ellipticalArcSweepExcess(a, ux, uy)))
}

func (c *tangentLineEllipse) retireVars(s *Sketch) {
	if c.slack >= 0 {
		s.retireVar(c.slack)
		c.slack = -1 // reset so re-adding the handle allocates a fresh slack
	}
}

// localNormal returns the line's unit normal in the ellipse's rotated local
// frame (u, v) and the signed perpendicular distance h = (center − A)·n from the
// ellipse center to the line (the tangentLineCircle convention). ok is false for
// a degenerate (zero-length) line.
func (c *tangentLineEllipse) localNormal() (float64, float64, float64, bool) {
	l := c.L
	ax, ay := l.Start.x(), l.Start.y()
	abx, aby := l.End.x()-ax, l.End.y()-ay
	ablen := norm(abx, aby)
	if math.Hypot(abx, aby) < 1e-9 {
		return 0, 0, 0, false
	}
	nx, ny := -aby/ablen, abx/ablen
	ctr := c.E.centerPt()
	h := (abx*(ctr.y()-ay) - aby*(ctr.x()-ax)) / ablen
	cosr, sinr := math.Cos(c.E.Rotation()), math.Sin(c.E.Rotation())
	u := cosr*nx + sinr*ny
	v := -sinr*nx + cosr*ny
	return u, v, h, true
}

// ellipseContactDir returns the eccentric unit direction (cos θ, sin θ) of the
// tangent contact point for local-frame line normal (u, v) at signed distance h.
// The contact lies on the −h side of the center (the line itself is at +(−h)·n),
// so the eccentric direction is (−h·u·rx, −h·v·ry) normalized — matching the
// (lx/rx, ly/ry)-normalized convention pointOnEllipticalArc.eccentricDir uses.
func ellipseContactDir(u, v, h, rx, ry float64) (float64, float64) {
	ex, ey := -h*u*rx, -h*v*ry
	n := norm(ex, ey)
	return ex / n, ey / n
}

func (c *tangentLineEllipse) contactEccentricDir() (float64, float64) {
	u, v, h, ok := c.localNormal()
	if !ok {
		return 1, 0
	}
	return ellipseContactDir(u, v, h, c.E.Rx(), c.E.Ry())
}

func (c *tangentLineEllipse) residual(out []float64) []float64 {
	a, isArc := c.E.(*EllipticalArc)
	rx, ry := c.E.Rx(), c.E.Ry()
	// A degenerate ellipse (a zero/near-zero semi-axis — a segment or a point) has
	// no well-defined tangent line, so tangency to it must never be blessed. Axes
	// are ordinary solver vars (not guaranteed positive), so the test is on
	// magnitude. Floored above the solver tolerance (1e-10).
	degenerateEllipse := math.Abs(rx) < 1e-9 || math.Abs(ry) < 1e-9

	// Endpoint tangency: line ⊥ the ellipse's outward normal at the shared
	// boundary point — cos of the line/normal angle, zero when perpendicular
	// (dimensionless). A degenerate line or ellipse is never tangent.
	if isArc && c.shared != nil {
		l := c.L
		abx, aby := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		ablen := norm(abx, aby)
		if degenerateEllipse || math.Hypot(abx, aby) < 1e-9 {
			return append(out, 1)
		}
		nx, ny := ellipseNormalAt(c.shared, c.E)
		return append(out, (abx*nx+aby*ny)/(ablen*norm(nx, ny)))
	}

	// Interior tangency. A degenerate line (no direction) or degenerate ellipse
	// makes the tangency row a clearly-nonzero positive value (floored to 1 and
	// sign-independent via hypot, so it cannot read as ~0 for a zero/negative
	// semi-axis) that is never blessed; the contact then falls back to (1, 0).
	// Otherwise it is √((u·rx)²+(v·ry)²) − |h|. The row count is unchanged either
	// way, so the per-constraint arity stays constant for the finite-difference
	// Jacobian.
	u, v, h, ok := c.localNormal()
	if !ok || degenerateEllipse {
		out = append(out, math.Max(math.Hypot(rx, ry), 1))
	} else {
		out = append(out, math.Hypot(u*rx, v*ry)-math.Abs(h))
	}

	// Once the sweep slack is allocated, confine the contact within the arc's
	// eccentric sweep via the slack-encoded inequality dot(u, m) - cos(half) = w*w.
	// Gating the sweep row on the slack keeps a committed constraint's arity
	// constant across solver iterations (the finite-difference Jacobian requires it).
	if c.slack >= 0 {
		ux, uy := ellipseContactDir(u, v, h, rx, ry)
		w := c.s.vars[c.slack]
		out = append(out, ellipticalArcSweepExcess(a, ux, uy)-w*w)
	}
	return out
}

// ellipseNormalAt returns the ellipse's outward normal at the point p (assumed
// on the ellipse), in world coordinates: the local gradient (lx/rx², ly/ry²)
// rotated back to world. Not unit-length; callers normalize.
func ellipseNormalAt(p *Point, e Elliptical) (float64, float64) {
	return ellipseNormalXY(p.x(), p.y(), e.centerPt().x(), e.centerPt().y(), e.Rx(), e.Ry(), e.Rotation())
}

// ellipseNormalXY returns the ellipse's outward normal at world point (px,py):
// the local gradient (lx/rx², ly/ry²) rotated back to world. Not unit-length.
// The axis squares are floored (as in sampsonEllipse) so a degenerate (zero
// semi-axis) ellipse yields a finite normal rather than NaN — the conic-tangency
// degenerate guard then rejects it cleanly instead of poisoning the residual.
func ellipseNormalXY(px, py, cx, cy, rx, ry, rot float64) (float64, float64) {
	cosr, sinr := math.Cos(rot), math.Sin(rot)
	dx, dy := px-cx, py-cy
	lx := cosr*dx + sinr*dy
	ly := -sinr*dx + cosr*dy
	gx := lx / math.Max(rx*rx, 1e-12)
	gy := ly / math.Max(ry*ry, 1e-12)
	return cosr*gx - sinr*gy, sinr*gx + cosr*gy
}

// sharedPointLineEllipticalArc returns the point the line and elliptical arc
// share as an endpoint (the contact for endpoint tangency), or nil if none.
func sharedPointLineEllipticalArc(l *Line, a *EllipticalArc) *Point {
	for _, lp := range []*Point{l.Start, l.End} {
		if lp == a.Start || lp == a.End {
			return lp
		}
	}
	return nil
}

// --- dimensional constraints ------------------------------------------------

// dimBase is embedded by every dimensional constraint. It holds the driving
// target as a unit-carrying [units.Value] (or, when bound, the parameter
// expression that produces it) and the kind of quantity the dimension measures.
type dimBase struct {
	kind   units.Kind
	target units.Value
	deflt  bool   // target's unit is a placeholder, replaced by the sketch default on add
	expr   string // bound parameter expression; empty when the value is literal
	driven bool   // reference dimension: measures the geometry instead of driving it
}

// Kind reports whether the dimension measures a length or an angle.
func (d *dimBase) Kind() units.Kind { return d.kind }

// Target returns the dimension's current driving value as a unit-carrying
// value.
func (d *dimBase) Target() units.Value { return d.target }

// Set changes the driving magnitude, keeping the dimension's current unit, and
// clears any parameter binding. Call [Sketch.Solve] again to apply it.
func (d *dimBase) Set(v float64) {
	d.target = units.New(v, d.target.Unit())
	d.expr = ""
}

// SetValue sets the driving value to a typed quantity (which must measure the
// dimension's kind) and clears any binding. The value keeps its own unit; no
// conversion takes place here — the units library converts on demand (e.g. via
// [dimBase.base] for the solver).
func (d *dimBase) SetValue(v units.Value) error {
	if v.Kind() != d.kind {
		return fmt.Errorf("sketch: cannot set %s dimension from a %s value", d.kind, v.Kind())
	}
	d.target = v
	d.deflt = false
	d.expr = ""
	return nil
}

// SetDriven toggles the dimension between driving (a solver constraint) and
// driven (a reference dimension). A driven dimension contributes no residual —
// it does not constrain the geometry — and after every [Sketch.Solve] its
// [dimBase.Target] is refreshed to the measured value. Switching back to
// driving keeps the last measured value as the new driving target.
func (d *dimBase) SetDriven(v bool) { d.driven = v }

// Driven reports whether the dimension is a driven (reference) dimension.
func (d *dimBase) Driven() bool { return d.driven }

// resolveUnit, called when the dimension is added to a sketch, replaces a
// placeholder unit (from a bare-float constructor) with the sketch's default
// unit for the kind, keeping the magnitude. This is how a bare number takes on
// the sketch's chosen unit; it is an assignment of intent, not a conversion.
func (d *dimBase) resolveUnit(s *Sketch) {
	if d.deflt {
		d.target = units.New(d.target.Mag(), s.unitFor(d.kind))
		d.deflt = false
	}
}

// base returns the target in the kind's base unit (mm or rad) for the solver.
func (d *dimBase) base() float64 { return d.target.Base() }

// setResolved stores a value produced by evaluating a parameter binding. A
// quantity keeps its own unit; a dimensionless result is taken to already be in
// the dimension's current unit.
func (d *dimBase) setResolved(v units.Value) error {
	if v.Kind() == units.Dimensionless {
		d.target = units.New(v.Mag(), d.target.Unit())
		return nil
	}
	if v.Kind() != d.kind {
		return fmt.Errorf("sketch: cannot set %s dimension from a %s value", d.kind, v.Kind())
	}
	d.target = v
	return nil
}

func (d *dimBase) driverExpr() string     { return d.expr }
func (d *dimBase) setDriverExpr(e string) { d.expr = e }

// restore sets the target verbatim from a deserialized magnitude and unit. It
// reinstates saved state and does not convert.
func (d *dimBase) restore(mag float64, u units.Unit) {
	d.target = units.New(mag, u)
	d.deflt = false
}

// lengthDim and angleDim build a detached dimension whose unit is a placeholder
// (the metric default) to be resolved to the sketch's default unit on add.
func lengthDim(v float64) dimBase {
	return dimBase{kind: units.Length, target: units.Millimeters(v), deflt: true}
}

func angleDim(v float64) dimBase {
	return dimBase{kind: units.Angle, target: units.Degrees(v), deflt: true}
}

// Distance is an editable point-to-point distance dimension.
//
// Like every dimension type, its driving value may instead be bound to a
// parameter expression with [Sketch.Bind]; the binding is re-evaluated against
// the sketch's parameter table before each solve.
type Distance struct {
	dimBase
	P1, P2 *Point
}

func (c *Distance) residual(out []float64) []float64 {
	return append(out, dist(c.P1, c.P2)-c.base())
}

// NewDistance constrains the straight-line distance between two points. The
// value d is interpreted in the sketch's default length unit once added.
func NewDistance(p1, p2 *Point, d float64) *Distance {
	return &Distance{dimBase: lengthDim(d), P1: p1, P2: p2}
}

// HorizontalDistance is an editable signed horizontal (Δx) dimension.
type HorizontalDistance struct {
	dimBase
	P1, P2 *Point
}

func (c *HorizontalDistance) residual(out []float64) []float64 {
	return append(out, c.P2.x()-c.P1.x()-c.base())
}

// NewHorizontalDistance constrains the signed horizontal distance (x2−x1).
func NewHorizontalDistance(p1, p2 *Point, d float64) *HorizontalDistance {
	return &HorizontalDistance{dimBase: lengthDim(d), P1: p1, P2: p2}
}

// VerticalDistance is an editable signed vertical (Δy) dimension.
type VerticalDistance struct {
	dimBase
	P1, P2 *Point
}

func (c *VerticalDistance) residual(out []float64) []float64 {
	return append(out, c.P2.y()-c.P1.y()-c.base())
}

// NewVerticalDistance constrains the signed vertical distance (y2−y1).
func NewVerticalDistance(p1, p2 *Point, d float64) *VerticalDistance {
	return &VerticalDistance{dimBase: lengthDim(d), P1: p1, P2: p2}
}

// DistancePointLine is an editable perpendicular distance dimension between a
// point and the infinite line through a [Line]'s endpoints.
type DistancePointLine struct {
	dimBase
	P *Point
	L *Line
}

func (c *DistancePointLine) residual(out []float64) []float64 {
	// |distance(point, line)| − d, in length units
	ax, ay := c.L.Start.x(), c.L.Start.y()
	abx, aby := c.L.End.x()-ax, c.L.End.y()-ay
	apx, apy := c.P.x()-ax, c.P.y()-ay
	cross := abx*apy - aby*apx
	return append(out, math.Abs(cross)/norm(abx, aby)-c.base())
}

// NewDistancePointLine constrains the perpendicular distance from a point to
// the infinite line through l. The distance is unsigned: the point stays on
// whichever side of the line it starts.
func NewDistancePointLine(p *Point, l *Line, d float64) *DistancePointLine {
	return &DistancePointLine{dimBase: lengthDim(d), P: p, L: l}
}

// DistancePointCircle is an editable distance dimension between a point and a
// circle's edge: the signed radial gap |P−C| − r, positive when the point is
// outside the circle and negative inside. A target of 0 places the point on the
// circle; the sign of the target chooses the side, so no separate side flag is
// needed. (Arc edges, whose nearest point may be a sweep endpoint, are a
// follow-up — this is the full circle.)
type DistancePointCircle struct {
	dimBase
	P *Point
	C *Circle
}

func (c *DistancePointCircle) residual(out []float64) []float64 {
	return append(out, norm(c.P.x()-c.C.Center.x(), c.P.y()-c.C.Center.y())-c.C.r()-c.base())
}

// NewDistancePointCircle constrains the signed radial distance from a point to a
// circle's edge (|P−center| − radius): positive outside, negative inside. The
// value d is interpreted in the sketch's default length unit once added.
func NewDistancePointCircle(p *Point, circle *Circle, d float64) *DistancePointCircle {
	return &DistancePointCircle{dimBase: lengthDim(d), P: p, C: circle}
}

// DistanceLineCircle is an editable distance dimension between an (infinite) line
// and a circle's edge: the tangent gap dist(center, line) − r. A target of 0
// makes the line tangent to the circle; a positive target keeps the line that far
// clear of the circle, a negative target lets it cut through. The line is treated
// as its infinite carrier (like [NewDistancePointLine] / [NewTangent]); the
// distance is unsigned in the perpendicular sense, so the circle stays on
// whichever side of the line it starts. (Arc edges are a follow-up.)
type DistanceLineCircle struct {
	dimBase
	L *Line
	C *Circle
}

func (c *DistanceLineCircle) residual(out []float64) []float64 {
	ax, ay := c.L.Start.x(), c.L.Start.y()
	abx, aby := c.L.End.x()-ax, c.L.End.y()-ay
	cross := abx*(c.C.Center.y()-ay) - aby*(c.C.Center.x()-ax)
	return append(out, math.Abs(cross)/norm(abx, aby)-c.C.r()-c.base())
}

// NewDistanceLineCircle constrains the distance from a circle's edge to the
// infinite line through l (perpendicular distance from the center to the line,
// minus the radius). A target of 0 is tangency. The value d is interpreted in the
// sketch's default length unit once added.
func NewDistanceLineCircle(l *Line, circle *Circle, d float64) *DistanceLineCircle {
	return &DistanceLineCircle{dimBase: lengthDim(d), L: l, C: circle}
}

// DistancePointArc is an editable distance dimension between a point and an
// arc's edge: the signed radial gap |P−C| − R like [DistancePointCircle], but
// confined to the arc's sweep. The dimension measures the point against the arc's
// circular carrier *and* requires the radial foot (the direction from the center
// toward P) to lie within the sweep — so a point whose nearest carrier point
// falls off the swept portion is reported unsolvable rather than silently
// measured to whichever endpoint happens to be closer. (To dimension to an
// endpoint, use [NewDistance] to that endpoint explicitly.) A target of 0 places
// the point on the arc edge; the sign of the target chooses inside vs outside.
//
// The sweep confinement is a slack-encoded inequality whose slack is an auxiliary
// solver variable, allocated when the dimension is committed and retired on
// removal (not serialized — recomputed from the geometry on load), exactly like
// [NewPointOnArc]. A driven (reference) dimension contributes no residual rows, so
// it owns no slack (one would be left unconstrained); it measures the signed
// radial gap directly, like a driven [ArcLength].
type DistancePointArc struct {
	dimBase
	P     *Point
	A     *Arc
	s     *Sketch // set by allocVars, for slack access
	slack int     // sweep slack var index; -1 = none (driven or not yet allocated)
}

// NewDistancePointArc constrains the signed radial distance from a point to an
// arc's edge (|P−center| − radius), with the radial foot confined to the arc's
// sweep: positive outside, negative inside, 0 on the edge. The arc must have a
// nonzero radius. The value d is interpreted in the sketch's default length unit
// once added.
func NewDistancePointArc(p *Point, a *Arc, d float64) *DistancePointArc {
	return &DistancePointArc{dimBase: lengthDim(d), P: p, A: a, slack: -1}
}

func (c *DistancePointArc) allocVars(s *Sketch) {
	c.s = s
	if c.driven || c.slack >= 0 {
		return // driven: no residual rows, so no aux var. Else idempotent.
	}
	ux, uy := arcRadialDir(c.P, c.A.Center)
	c.slack = s.newVar(slackFor(arcInSweepExcess(c.A, ux, uy)))
}

func (c *DistancePointArc) retireVars(s *Sketch) {
	if c.slack >= 0 {
		s.retireVar(c.slack)
		c.slack = -1 // reset so re-adding the handle allocates a fresh slack
	}
}

// SetDriven toggles reference (driven) mode and keeps the sweep slack consistent:
// a driven dimension contributes no residual rows, so its slack would be an
// unconstrained free DOF — switching to driven retires it, switching back
// re-allocates it. Mirrors [ArcLength.SetDriven]; the committed test guards
// against a CheckConstraint probe (which sets c.s via a temporary allocVars
// without registering the dimension) leaking an orphan variable.
func (c *DistancePointArc) SetDriven(v bool) {
	if v == c.driven {
		return
	}
	c.driven = v
	if c.s == nil || !committedDim(c.s, c) {
		return
	}
	if v {
		c.retireVars(c.s)
	} else if c.slack < 0 {
		ux, uy := arcRadialDir(c.P, c.A.Center)
		c.slack = c.s.newVar(slackFor(arcInSweepExcess(c.A, ux, uy)))
	}
}

func (c *DistancePointArc) residual(out []float64) []float64 {
	cx, cy := c.A.Center.x(), c.A.Center.y()
	dx, dy := c.P.x()-cx, c.P.y()-cy
	rhoRaw := math.Hypot(dx, dy)
	rho := norm(dx, dy)
	// Row 0 (length): the signed radial gap, which refreshDriven reads for a driven
	// dimension and which residuals() skips entirely for one.
	out = append(out, rho-c.A.R()-c.base())
	if c.slack < 0 {
		return out // driven (reference) dimension, or a bare call before allocVars
	}
	w := c.s.vars[c.slack]
	// Row 1 (dimensionless): confine the radial foot to the arc's sweep via the
	// slack-encoded inequality. Gating on the slack keeps the committed arity
	// constant for the finite-difference Jacobian.
	//
	// A point AT the center has no radial direction — `arcInSweepExcess` of the
	// floored zero vector is −cos(sweep/2), which is ≥ 0 for a sweep ≥ π and would
	// falsely bless the (degenerate, direction-undefined) configuration. Detect it
	// on the raw magnitude and force the row strictly negative so it is reported
	// unsolvable instead, regardless of sweep.
	if rhoRaw < 1e-9 {
		return append(out, -1-w*w)
	}
	ux, uy := dx/rhoRaw, dy/rhoRaw
	return append(out, arcInSweepExcess(c.A, ux, uy)-w*w)
}

// DistanceLineArc is an editable distance dimension between an (infinite) line
// and an arc's edge: the tangent gap dist(center, line) − R like
// [DistanceLineCircle], confined to the arc's sweep. As with [DistancePointArc]
// the near-side carrier contact (the foot of the perpendicular from the center)
// must lie within the sweep, so a line tangent to the arc's full circle off the
// swept portion is reported unsolvable. A target of 0 makes the line tangent to
// the arc itself; a positive target keeps the line that far clear, a negative
// target lets it cut the carrier (signed carrier penetration, not the true
// Euclidean distance to the bounded arc). The line is treated as its infinite
// carrier; the distance is unsigned in the perpendicular sense.
type DistanceLineArc struct {
	dimBase
	L     *Line
	A     *Arc
	s     *Sketch
	slack int
}

// NewDistanceLineArc constrains the distance from an arc's edge to the infinite
// line through l (perpendicular distance from the center to the line, minus the
// radius), with the near-side carrier contact confined to the arc's sweep. A
// target of 0 is tangency to the arc. The arc must have a nonzero radius.
func NewDistanceLineArc(l *Line, a *Arc, d float64) *DistanceLineArc {
	return &DistanceLineArc{dimBase: lengthDim(d), L: l, A: a, slack: -1}
}

func (c *DistanceLineArc) allocVars(s *Sketch) {
	c.s = s
	if c.driven || c.slack >= 0 {
		return
	}
	ux, uy := lineFootDir(c.L, c.A.Center)
	c.slack = s.newVar(slackFor(arcInSweepExcess(c.A, ux, uy)))
}

func (c *DistanceLineArc) retireVars(s *Sketch) {
	if c.slack >= 0 {
		s.retireVar(c.slack)
		c.slack = -1
	}
}

// SetDriven mirrors [DistancePointArc.SetDriven].
func (c *DistanceLineArc) SetDriven(v bool) {
	if v == c.driven {
		return
	}
	c.driven = v
	if c.s == nil || !committedDim(c.s, c) {
		return
	}
	if v {
		c.retireVars(c.s)
	} else if c.slack < 0 {
		ux, uy := lineFootDir(c.L, c.A.Center)
		c.slack = c.s.newVar(slackFor(arcInSweepExcess(c.A, ux, uy)))
	}
}

func (c *DistanceLineArc) residual(out []float64) []float64 {
	ax, ay := c.L.Start.x(), c.L.Start.y()
	abx, aby := c.L.End.x()-ax, c.L.End.y()-ay
	cross := abx*(c.A.Center.y()-ay) - aby*(c.A.Center.x()-ax)
	// Row 0 (length): the tangent gap |h| − R, where h is the signed perpendicular
	// distance from the center to the infinite line.
	out = append(out, math.Abs(cross)/norm(abx, aby)-c.A.R()-c.base())
	if c.slack < 0 {
		return out
	}
	// Row 1 (dimensionless): confine the near-side carrier contact to the sweep.
	ux, uy := lineFootDir(c.L, c.A.Center)
	w := c.s.vars[c.slack]
	return append(out, arcInSweepExcess(c.A, ux, uy)-w*w)
}

// arcRadialDir returns the unit direction from the center toward p — the radial
// foot direction a point↔arc distance is judged against for sweep confinement. It
// uses the floored norm() so a point at the center yields a finite (near-zero)
// vector rather than NaN; such a degenerate configuration then fails the sweep
// row rather than poisoning the residual.
func arcRadialDir(p, center *Point) (float64, float64) {
	dx, dy := p.x()-center.x(), p.y()-center.y()
	d := norm(dx, dy)
	return dx / d, dy / d
}

// committedDim reports whether dimension c is registered in sketch s (as opposed
// to merely having had c.s set by a CheckConstraint probe's temporary allocVars).
func committedDim(s *Sketch, c Constraint) bool {
	for _, cc := range s.cons {
		if cc == c {
			return true
		}
	}
	return false
}

// DistanceLines is an editable distance dimension between two lines. It
// contributes two residuals — the distance from each endpoint of L2 to the
// infinite line through L1 — so satisfying it forces the lines parallel at the
// given separation; no separate parallel constraint is needed.
type DistanceLines struct {
	dimBase
	L1, L2 *Line
}

func (c *DistanceLines) residual(out []float64) []float64 {
	// Signed distance of both L2 endpoints from L1, oriented so the first
	// endpoint's current side counts as positive — this keeps the geometry on
	// the side it starts while rejecting the crossing configuration where the
	// endpoints sit at distance d on opposite sides. Length units ×2.
	ax, ay := c.L1.Start.x(), c.L1.Start.y()
	abx, aby := c.L1.End.x()-ax, c.L1.End.y()-ay
	n := norm(abx, aby)
	d1 := (abx*(c.L2.Start.y()-ay) - aby*(c.L2.Start.x()-ax)) / n
	d2 := (abx*(c.L2.End.y()-ay) - aby*(c.L2.End.x()-ax)) / n
	sign := 1.0
	if d1 < 0 {
		sign = -1
	}
	return append(out, sign*d1-c.base(), sign*d2-c.base())
}

// NewDistanceLines constrains the perpendicular distance between two lines,
// forcing them parallel in the process. The distance is unsigned: L2 stays on
// whichever side of L1 it starts.
func NewDistanceLines(l1, l2 *Line, d float64) *DistanceLines {
	return &DistanceLines{dimBase: lengthDim(d), L1: l1, L2: l2}
}

// Offset is an editable signed offset dimension: it drives the destination line
// Dst to sit at signed perpendicular distance d from the infinite line through
// the source line Src, with positive d on the left of Src's start→end
// direction. Unlike [DistanceLines] the side never flips, so it is the building
// block for parallel offsets — including chains, where a corner point shared by
// two offset segments is pulled to the intersection of both offsets.
type Offset struct {
	dimBase
	Src, Dst *Line
}

func (c *Offset) residual(out []float64) []float64 {
	// Signed perpendicular distance (left-positive) of each Dst endpoint from
	// the infinite line through Src, minus the signed target. Length units.
	ax, ay := c.Src.Start.x(), c.Src.Start.y()
	abx, aby := c.Src.End.x()-ax, c.Src.End.y()-ay
	n := norm(abx, aby)
	d := c.base()
	s1 := (abx*(c.Dst.Start.y()-ay) - aby*(c.Dst.Start.x()-ax)) / n
	s2 := (abx*(c.Dst.End.y()-ay) - aby*(c.Dst.End.x()-ax)) / n
	return append(out, s1-d, s2-d)
}

// NewOffset constrains line dst to be the parallel offset of src at signed
// distance d (positive on the left of src's direction).
func NewOffset(src, dst *Line, d float64) *Offset {
	return &Offset{dimBase: lengthDim(d), Src: src, Dst: dst}
}

// Radius is an editable radius dimension.
type Radius struct {
	dimBase
	C Circular
}

func (c *Radius) residual(out []float64) []float64 {
	return append(out, c.C.R()-c.base())
}

// NewRadius constrains a circular entity's radius. It accepts a [*Circle] or an
// [*Arc] (an arc's radius is the distance from its center to its endpoints). For
// an arc the start point must not coincide with the center: a zero-radius arc
// has no radius gradient, so the solver cannot grow it toward a positive target.
func NewRadius(c Circular, r float64) *Radius {
	return &Radius{dimBase: lengthDim(r), C: c}
}

// Diameter is an editable diameter dimension.
type Diameter struct {
	dimBase
	C Circular
}

func (c *Diameter) residual(out []float64) []float64 {
	return append(out, 2*c.C.R()-c.base())
}

// NewDiameter constrains a circular entity's diameter. It accepts a [*Circle] or
// an [*Arc]. As with [NewRadius], an arc operand must have a nonzero radius (its
// start must not coincide with its center) or the solver has no gradient to act
// on.
func NewDiameter(c Circular, d float64) *Diameter {
	return &Diameter{dimBase: lengthDim(d), C: c}
}

// ArcLength is an editable dimension on an arc's swept length (radius × sweep
// angle). Driving the swept length cannot use the naive residual R·Sweep() − L
// because Sweep() jumps from 2π to 0 as the end crosses the start (a
// discontinuous Jacobian). Instead the dimension owns one auxiliary solver
// variable — the *unwrapped* sweep angle theta — driving R·theta = L and pinning
// theta to the geometry with a continuous coupling row (see allocVars/residual).
//
// ArcLength can be a driving or a driven (reference/measuring) dimension. A
// driven dimension contributes no residual rows, so it owns no unwrapped-sweep
// auxiliary variable (one would be left unconstrained); the measured length is
// recovered directly as R·Sweep() through the pre-allocation residual branch that
// refreshDriven reads. [ArcLength.SetDriven] manages that aux variable's lifecycle
// across a toggle.
type ArcLength struct {
	dimBase
	A     *Arc
	s     *Sketch // set by allocVars, for theta access
	theta int     // unwrapped-sweep aux var index; -1 = not yet allocated
}

// NewArcLength constrains an arc's swept length — its radius times its
// counter-clockwise sweep angle. The value is interpreted in the sketch's
// default length unit once added. The arc must have a nonzero radius (its start
// must not coincide with its center), like [NewRadius].
func NewArcLength(a *Arc, length float64) *ArcLength {
	return &ArcLength{dimBase: lengthDim(length), A: a, theta: -1}
}

func (c *ArcLength) allocVars(s *Sketch) {
	c.s = s
	if c.driven || c.theta >= 0 {
		return // driven: no residual rows, so no aux var. Else idempotent.
	}
	c.theta = s.newVar(c.A.Sweep()) // seed to the current (solved) sweep
}

func (c *ArcLength) retireVars(s *Sketch) {
	if c.theta >= 0 {
		s.retireVar(c.theta)
		c.theta = -1 // reset so re-adding the handle allocates a fresh aux var
	}
}

// SetDriven toggles reference (driven) mode and keeps the unwrapped-sweep aux
// variable consistent: a driven dimension contributes no residual rows, so its
// theta would be an unconstrained free DOF — switching to driven retires it, and
// switching back to driving re-allocates it. Until the dimension is actually
// committed (registered in the sketch) it only records the flag; allocVars then
// honors it. Membership — not merely c.s != nil — is the committed test, because
// CheckConstraint sets c.s via a temporary allocVars without registering the
// dimension; mutating s.vars there would leak an orphan variable.
func (c *ArcLength) SetDriven(v bool) {
	if v == c.driven {
		return
	}
	c.driven = v
	if !c.committed() {
		return
	}
	if v {
		c.retireVars(c.s)
	} else if c.theta < 0 {
		c.theta = c.s.newVar(c.A.Sweep())
	}
}

// committed reports whether this dimension is registered in its sketch (as
// opposed to merely having had c.s set by a CheckConstraint probe's temporary
// allocVars).
func (c *ArcLength) committed() bool {
	if c.s == nil {
		return false
	}
	for _, cc := range c.s.cons {
		if cc == c {
			return true
		}
	}
	return false
}

func (c *ArcLength) residual(out []float64) []float64 {
	cx, cy := c.A.Center.x(), c.A.Center.y()
	sx0, sy0 := c.A.Start.x()-cx, c.A.Start.y()-cy
	ex, ey := c.A.End.x()-cx, c.A.End.y()-cy
	r := norm(sx0, sy0)
	if c.theta < 0 {
		// No unwrapped-sweep var: either a driven (reference) dimension — where this
		// single measured−target row is what refreshDriven reads (residuals() skips
		// driven dims, so it never enters the solve) — or a bare residual call before
		// allocVars. Sweep()'s wrap discontinuity is harmless for a pure measurement.
		return append(out, r*c.A.Sweep()-c.base())
	}
	theta := c.s.vars[c.theta]
	// Row 0: drive the swept length (length units), via the unwrapped sweep.
	out = append(out, r*theta-c.base())
	// Row 1 (dimensionless): pin theta to the geometry's unwrapped sweep. Δ is
	// the principal signed angle from the start ray to the end ray; the residual
	// is (Δ − theta) wrapped into (−π, π], so it is zero only at the correct
	// branch (theta ≡ Δ mod 2π) — NOT at the antipodal theta ≡ Δ + π — with a
	// clean ∂/∂theta = −1 at the solution. (A plain sin(Δ − theta) would vanish on
	// the wrong branch too.) Δ's atan2 jumps 2π at a half-turn (a semicircle
	// target), but the mod-2π wrap absorbs that jump, so row 1 stays continuous
	// there. The residual is only discontinuous if theta would have to move more
	// than π from Δ's branch — which never happens while the end is free (end and
	// theta move together, keeping Δ − theta near 0). This mirrors the Angle
	// dimension's wrapped-angle residual.
	cross := sx0*ey - sy0*ex
	dot := sx0*ex + sy0*ey
	r1 := math.Mod(math.Atan2(cross, dot)-theta, 2*math.Pi)
	if r1 > math.Pi {
		r1 -= 2 * math.Pi
	} else if r1 <= -math.Pi {
		r1 += 2 * math.Pi
	}
	return append(out, r1)
}

// equalLineArc forces a line's length to equal an arc's swept length R·Sweep().
// Arc.Sweep() is canonical in (0, 2π], so the single residual R·Sweep() − Length()
// is sound: a line longer than the arc's full circumference (2πR) cannot be
// matched and is correctly reported unsolvable. No unwrapped-sweep auxiliary
// variable is used (unlike ArcLength's *driving* a target) — that would admit a
// multi-turn theta satisfying the equation while the real swept length differs.
// The only cost is a Jacobian discontinuity at the exact full-circle point, where
// Sweep() steps 2π↔0⁺; that is a rare configuration. The arc must have a nonzero
// radius, like NewArcLength.
type equalLineArc struct {
	L *Line
	A *Arc
}

// NewEqualLineArc forces a line's length to equal an arc's swept length
// (radius × counter-clockwise sweep angle). The arc must have a nonzero radius.
func NewEqualLineArc(l *Line, a *Arc) Constraint {
	return &equalLineArc{l, a}
}

func (c *equalLineArc) residual(out []float64) []float64 {
	return append(out, c.A.R()*c.A.Sweep()-c.L.Length()) // length units
}

// Angle is an editable signed angle dimension between two lines, measured
// counterclockwise from L1's start→end direction to L2's.
type Angle struct {
	dimBase
	L1, L2 *Line
}

func (c *Angle) residual(out []float64) []float64 {
	d1x, d1y := dir(c.L1)
	d2x, d2y := dir(c.L2)
	cross := d1x*d2y - d1y*d2x
	dot := d1x*d2x + d1y*d2y
	ang := math.Atan2(cross, dot)
	// wrap the residual into (-π, π] so it stays continuous
	r := math.Mod(ang-c.base(), 2*math.Pi) // target in base (radian) units
	if r > math.Pi {
		r -= 2 * math.Pi
	} else if r <= -math.Pi {
		r += 2 * math.Pi
	}
	return append(out, r)
}

// NewAngle constrains the angle from line l1 to line l2. The angle is signed:
// it is measured counterclockwise from l1's start→end direction to l2's, so a
// and -a pin mirror-image configurations and swapping a line's endpoints
// flips the measurement. Values wrap modulo a full turn (270° ≡ −90°). Unlike
// an unsigned dimension, a signed angle admits a single configuration — to
// put the geometry on the other side, negate the value rather than reseeding.
//
// The value a is interpreted in the sketch's default angle unit (degrees for
// [units.Metric]) once added; use [Angle.SetValue] with a typed quantity such
// as units.Radians for another unit.
func NewAngle(l1, l2 *Line, a float64) *Angle {
	return &Angle{dimBase: angleDim(a), L1: l1, L2: l2}
}

// SemiMajor is an editable dimension on an ellipse's semi-axis along its local
// x axis (the major axis by convention; not enforced).
type SemiMajor struct {
	dimBase
	E Elliptical
}

func (c *SemiMajor) residual(out []float64) []float64 {
	return append(out, c.E.Rx()-c.base()) // length units
}

// NewSemiMajor constrains the semi-axis along an ellipse's local x axis. It
// accepts a [*Ellipse] or an [*EllipticalArc].
func NewSemiMajor(e Elliptical, r float64) *SemiMajor {
	return &SemiMajor{dimBase: lengthDim(r), E: e}
}

// SemiMinor is an editable dimension on an ellipse's semi-axis along its local
// y axis (the minor axis by convention; not enforced).
type SemiMinor struct {
	dimBase
	E Elliptical
}

func (c *SemiMinor) residual(out []float64) []float64 {
	return append(out, c.E.Ry()-c.base()) // length units
}

// NewSemiMinor constrains the semi-axis along an ellipse's local y axis. It
// accepts a [*Ellipse] or an [*EllipticalArc].
func NewSemiMinor(e Elliptical, r float64) *SemiMinor {
	return &SemiMinor{dimBase: lengthDim(r), E: e}
}

// EllipseRotation is an editable dimension on the rotation of an ellipse's
// local frame.
type EllipseRotation struct {
	dimBase
	E Elliptical
}

func (c *EllipseRotation) residual(out []float64) []float64 {
	// wrap into (-π, π] so the residual stays continuous, like Angle
	r := math.Mod(c.E.Rotation()-c.base(), 2*math.Pi)
	if r > math.Pi {
		r -= 2 * math.Pi
	} else if r <= -math.Pi {
		r += 2 * math.Pi
	}
	return append(out, r)
}

// NewEllipseRotation constrains the rotation of an ellipse's local frame: a
// signed angle measured counterclockwise from the global +x axis, wrapping
// modulo a full turn. The value a is interpreted in the sketch's default angle
// unit once added. It accepts a [*Ellipse] or an [*EllipticalArc].
func NewEllipseRotation(e Elliptical, a float64) *EllipseRotation {
	return &EllipseRotation{dimBase: angleDim(a), E: e}
}

// --- geometry helpers -------------------------------------------------------

func dir(l *Line) (float64, float64) { return l.End.x() - l.Start.x(), l.End.y() - l.Start.y() }

func dist(a, b *Point) float64 { return math.Hypot(a.x()-b.x(), a.y()-b.y()) }

// norm returns the Euclidean length of (x, y), floored away from zero so that
// residuals which divide by a length stay finite for degenerate geometry.
func norm(x, y float64) float64 {
	n := math.Hypot(x, y)
	if n < 1e-12 {
		return 1e-12
	}
	return n
}
