package sketch

import "math"

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
	L        *Line
	C        Circular
	shared   *Point // shared contact endpoint (endpoint tangency); nil otherwise
	auxOwner        // set by allocVars: the sketch these variable indices belong to
	slack    int    // sweep slack var index; -1 = none (circle or endpoint)
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

// endpointPerpResidual is the shared tail of the endpoint-tangency branch: the
// cos of the angle between the line direction (abx,aby), of length ablen, and
// the outward normal (nx,ny) at the shared point — zero when the line is
// perpendicular to the normal (i.e. tangent). The caller guards a zero-length
// line first; the normal is normalized here.
func endpointPerpResidual(abx, aby, ablen, nx, ny float64) float64 {
	return (abx*nx + aby*ny) / (ablen * norm(nx, ny))
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
		if ablen < 1e-9 {
			return append(out, 1)
		}
		dx, dy := c.shared.x()-ctr.x(), c.shared.y()-ctr.y()
		return append(out, endpointPerpResidual(abx, aby, ablen, dx, dy))
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
	auxOwner     // set by allocVars: the sketch these variable indices belong to
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

	// Endpoint tangency: the shared point is the contact (an arc endpoint, on
	// both circles), and the circles are tangent there exactly when both centers
	// lie on one line through it — on the same side for internal tangency, on
	// opposite sides for external. One dimensionless row measures that as an
	// angle. The center-distance row below is NOT usable here: with the shared
	// point on both circles the triangle inequality makes it non-negative, so
	// tangency is its minimum and its gradient vanishes at the solution — the
	// row adds no rank and reads as redundant. The contact is in both sweeps by
	// inclusivity, so no sweep slack row is needed.
	if c.shared != nil {
		return append(out, sharedTangentAngle(c.shared, o1, o2, c.Internal))
	}

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

// sharedTangentAngle is the endpoint-tangency residual of two circular
// entities that share the contact point x: the signed angle, wrapped into
// (−π, π], from the radius direction x−o1 to x−o2 (internal) or to o2−x
// (external). It is zero exactly when the two circles are tangent at x on the
// requested side, and ±π on the wrong side. A zero-length radius or coincident
// centers have no tangent direction, so they return 1 rather than an angle that
// could read as satisfied.
func sharedTangentAngle(x, o1, o2 *Point, internal bool) float64 {
	u1x, u1y := x.x()-o1.x(), x.y()-o1.y()
	u2x, u2y := x.x()-o2.x(), x.y()-o2.y()
	if math.Hypot(u1x, u1y) < 1e-9 || math.Hypot(u2x, u2y) < 1e-9 ||
		math.Hypot(o2.x()-o1.x(), o2.y()-o1.y()) < 1e-9 {
		return 1
	}
	if !internal {
		u2x, u2y = -u2x, -u2y
	}
	return math.Atan2(u1x*u2y-u1y*u2x, u1x*u2x+u1y*u2y)
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
