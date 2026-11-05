package sketch

import (
	"math"

	"github.com/lestrrat-3d/sketch/geom"
)

// pointOnEllipticalArc confines a point to an elliptical arc: on the arc's
// ellipse (Sampson residual), AND within its eccentric-angle sweep. The sweep
// confinement reuses the interior-tangency slack pattern — a slack-encoded
// inequality keeping the point's eccentric direction inside the sweep — so a
// point on the full ellipse but off the arc is reported unsolvable.
type pointOnEllipticalArc struct {
	P        *Point
	A        *EllipticalArc
	auxOwner     // set by allocVars: the sketch these variable indices belong to
	slack    int // sweep slack var index; -1 = not yet allocated
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

// conicCoords returns the conic's three defining-point coordinates (Start, Apex,
// End) at the current solved configuration.
func conicCoords(c *Conic) ([2]float64, [2]float64, [2]float64) {
	return [2]float64{c.Start.x(), c.Start.y()},
		[2]float64{c.Apex.x(), c.Apex.y()},
		[2]float64{c.End.x(), c.End.y()}
}

// conicScale returns a length scale for a conic: the diagonal of the box bounding
// its three control points (Start, Apex, End), floored to 1, used to make the
// no-cusp and zero-line tangency thresholds scale-relative. It packages the
// three-point destructure that bboxDiagonal alone cannot express at the call site.
func conicScale(c *Conic) float64 {
	start, apex, end := conicCoords(c)
	return bboxDiagonal([][2]float64{start, apex, end})
}

// pointOnConic confines a point to a conic arc (a rational quadratic Bézier). A
// conic has the parametric form C(t), t ∈ [0,1], with no implicit F(x,y)=0, so —
// exactly like [pointOnSpline] — membership is existential: the constraint owns
// the foot parameter t as a bounded aux solver variable (t = w0², 1−t = w1²) so
// out-of-range t is infeasible rather than silently clamped. Committed residual
// (4 rows): P.x−C.x(t), P.y−C.y(t) (length), t−w0², (1−t)−w1² (the [0,1] box).
// A free point on a fixed conic then has 5 unknowns (P.x,P.y,t,w0,w1) and 4
// independent rows — one sliding DOF, as a point on a 1-D curve should. The aux
// vars are not serialized; allocVars re-seeds t by foot-point projection on load.
type pointOnConic struct {
	P            *Point
	C            *Conic
	auxOwner         // set by allocVars: the sketch these variable indices belong to
	tvar, w0, w1 int // foot parameter + box slacks; -1 = not yet allocated
}

// NewPointOnConic forces a point to lie on a conic arc. The point may sit anywhere
// along the curve; the attachment parameter is solved for. (To pin the point to an
// endpoint, make it coincident with the conic's Start or End, which it passes
// through.)
func NewPointOnConic(p *Point, c *Conic) Constraint {
	return &pointOnConic{P: p, C: c, tvar: -1, w0: -1, w1: -1}
}

func (c *pointOnConic) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent: re-adding the handle must not leak fresh aux vars
	}
	start, apex, end := conicCoords(c.C)
	t := geom.NearestParamConic(start, apex, end, c.C.rho(), c.P.x(), c.P.y())
	allocBoxParam(s, t, &c.tvar, &c.w0, &c.w1)
}

func (c *pointOnConic) retireVars(s *Sketch) {
	retireBoxParam(s, &c.tvar, &c.w0, &c.w1)
}

func (c *pointOnConic) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	t := c.s.vars[c.tvar]
	g := c.C.Geometry()
	sx, sy := g.Eval(clamp01(t)) // bound rows below, not the clamp, enforce [0,1]
	return pointOnCurveRows(out, c.P.x(), c.P.y(), sx, sy, t, c.s.vars[c.w0], c.s.vars[c.w1])
}

// tangentToConic forces a line tangent to a conic arc. Tangency is existential
// over the contact parameter t ∈ [0,1] (the same bounded witness as
// [pointOnConic]: t plus the box slacks w0,w1). The committed residual is five
// rows — exactly like [tangentToSpline]: the contact C(t) on the line's infinite
// carrier (signed perpendicular distance, length); the line direction parallel to
// the analytic conic tangent C'(t) (a dimensionless sin-of-angle); the two box
// rows; and a no-cusp guard |C'(t)|/scale ≥ epsTan (slack ws). C'(t) is the
// analytic geom.Conic.EvalDeriv (a numerical tangent inside the residual would be
// a nested finite difference the Jacobian re-differentiates).
type tangentToConic struct {
	L                *Line
	C                *Conic
	auxOwner             // set by allocVars: the sketch these variable indices belong to
	tvar, w0, w1, ws int // contact parameter, box slacks, speed-guard slack; -1 = unallocated
}

// NewTangentToConic forces a line tangent to a conic arc. The line is its infinite
// carrier (the contact may fall anywhere along it); the contact parameter on the
// conic is solved for and confined to the curve. A line tangent at more than one
// parameter witnesses one of them ([Sketch.ProbeConfigurations] can surface the
// alternates).
func NewTangentToConic(l *Line, c *Conic) Constraint {
	return &tangentToConic{L: l, C: c, tvar: -1, w0: -1, w1: -1, ws: -1}
}

func (c *tangentToConic) allocVars(s *Sketch) {
	c.s = s
	if c.tvar >= 0 {
		return // idempotent
	}
	t := c.seedParam()
	g := c.C.Geometry()
	spx, spy := g.EvalDeriv(clamp01(t))
	speed := norm(spx, spy)
	c.tvar = s.newVar(t)
	c.w0 = s.newVar(slackFor(t))
	c.w1 = s.newVar(slackFor(1 - t))
	c.ws = s.newVar(slackFor(speed/conicScale(c.C) - splineEpsTan))
}

func (c *tangentToConic) retireVars(s *Sketch) {
	if c.tvar >= 0 {
		s.retireVar(c.tvar)
		s.retireVar(c.w0)
		s.retireVar(c.w1)
		s.retireVar(c.ws)
		c.tvar, c.w0, c.w1, c.ws = -1, -1, -1, -1
	}
}

func (c *tangentToConic) seedParam() float64 {
	g := c.C.Geometry()
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	eval := func(t float64) (float64, float64) { return g.Eval(t) }
	deriv := func(t float64) (float64, float64) { return g.EvalDeriv(t) }
	return seedTangentParam(eval, deriv, 64, false, conicScale(c.C), ax, ay, dx, dy, norm(dx, dy))
}

func (c *tangentToConic) residual(out []float64) []float64 {
	if c.tvar < 0 {
		return out // not yet parameterized; allocVars runs at commit (and in CheckConstraint)
	}
	tv := c.s.vars[c.tvar]
	t := clamp01(tv)
	g := c.C.Geometry()
	sx, sy := g.Eval(t)
	spx, spy := g.EvalDeriv(t)
	ax, ay := c.L.Start.x(), c.L.Start.y()
	dx, dy := c.L.End.x()-ax, c.L.End.y()-ay
	dlen := norm(dx, dy)
	w0, w1, ws := c.s.vars[c.w0], c.s.vars[c.w1], c.s.vars[c.ws]
	// Scale is recomputed every evaluation, not snapshotted: a free or reshaped
	// conic must use its current size so the scale-relative thresholds stay valid.
	scale := conicScale(c.C)
	return tangentCurveRows(out, sx, sy, spx, spy, ax, ay, dx, dy, dlen, scale, ws, true, tv, w0, w1)
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
	return circleBoundary(a.c.Center.x(), a.c.Center.y(), a.c.r(), 0, 2*math.Pi, n, true)
}

// circleBoundary samples n points along a circular path centered at (cx,cy) of
// radius r, spanning [start, start+sweep]. A closed (whole-circle) sample steps
// by sweep/n (the last point is one step short of wrapping onto the first); an
// open (arc) sample steps by sweep/(n-1) so both endpoints are inclusive.
func circleBoundary(cx, cy, r, start, sweep float64, n int, closed bool) [][2]float64 {
	denom := float64(n - 1)
	if closed {
		denom = float64(n)
	}
	pts := make([][2]float64, n)
	for i := range pts {
		t := start + sweep*float64(i)/denom
		pts[i] = [2]float64{cx + r*math.Cos(t), cy + r*math.Sin(t)}
	}
	return pts
}

// ellipseBoundary samples n points along an elliptical path centered at (cx,cy)
// with semi-axes (rx,ry) rotated by rot, spanning eccentric angles
// [start, start+sweep]. The closed/open denominator convention matches
// circleBoundary.
func ellipseBoundary(cx, cy, rx, ry, rot, start, sweep float64, n int, closed bool) [][2]float64 {
	cosr, sinr := math.Cos(rot), math.Sin(rot)
	denom := float64(n - 1)
	if closed {
		denom = float64(n)
	}
	pts := make([][2]float64, n)
	for i := range pts {
		t := start + sweep*float64(i)/denom
		lx, ly := rx*math.Cos(t), ry*math.Sin(t)
		pts[i] = [2]float64{cx + cosr*lx - sinr*ly, cy + sinr*lx + cosr*ly}
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
	return ellipseBoundary(a.e.Center.x(), a.e.Center.y(), a.e.rx(), a.e.ry(), a.e.rot(), 0, 2*math.Pi, n, true)
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
	ux, uy := unitDir(px, py, x.a.Center.x(), x.a.Center.y())
	return arcInSweepExcess(x.a, ux, uy), true
}
func (x arcConic) endpoints() []*Point { return []*Point{x.a.Start, x.a.End} }
func (x arcConic) boundary(n int) [][2]float64 {
	cx, cy := x.a.Center.x(), x.a.Center.y()
	start := math.Atan2(x.a.Start.y()-cy, x.a.Start.x()-cx)
	// open: span the swept portion, endpoints included.
	return circleBoundary(cx, cy, x.a.R(), start, x.a.Sweep(), n, false)
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
	return ellipseBoundary(x.a.Center.x(), x.a.Center.y(), x.a.rx(), x.a.ry(), x.a.rot(), x.a.StartParam(), x.a.Sweep(), n, false)
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
	auxOwner              // set by allocVars: the sketch these variable indices belong to
	px, py         int    // contact-witness coordinates; -1 = unallocated (and in shared-endpoint mode)
	wSide          int    // internal/external branch slack (allocated in both branches)
	slackA, slackB int    // sweep slacks for arc operands; -1 = full conic (no sweep)
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
	scale := bboxDiagonal(sa, sb)
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

// --- midpoint / symmetric ---------------------------------------------------

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
// CreateEllipticalArc that pins a boundary point onto the arc's ellipse via the
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
	L        *Line
	E        Elliptical
	shared   *Point // shared contact endpoint (endpoint tangency); nil otherwise
	auxOwner        // set by allocVars: the sketch these variable indices belong to
	slack    int    // sweep slack var index; -1 = none (full ellipse or endpoint)
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
	if ablen < 1e-9 {
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
		if degenerateEllipse || ablen < 1e-9 {
			return append(out, 1)
		}
		nx, ny := ellipseNormalAt(c.shared, c.E)
		return append(out, endpointPerpResidual(abx, aby, ablen, nx, ny))
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
