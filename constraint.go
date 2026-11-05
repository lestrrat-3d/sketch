package sketch

import (
	"math"
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

// auxOwner records the sketch that parameterized a constraint owning auxiliary
// solver variables. Every allocVars implementation begins by writing the sketch
// it is given here, so the pointer names the variable vector the constraint's
// stored indices address — the one and only sketch those indices mean anything
// in. It is embedded (rather than declared as a plain field on each type) so
// that the accessor below travels with the field an aux-var constraint has to
// declare anyway: a new type that copies the field from its neighbours gets the
// accessor for free, and `c.s` keeps working through promotion.
type auxOwner struct {
	s *Sketch
}

// allocatedBy reports the sketch that allocated this constraint's auxiliary
// variables, or nil if none has. It is read by retireConstraintVars, which must
// retire those variables in that sketch and no other; see the comment there for
// why reference ownership is the wrong question on the removal path.
func (a auxOwner) allocatedBy() *Sketch { return a.s }

// clearAuxOwner forgets the allocating sketch, and must be called wherever those
// variables are given back — retireConstraintVars on the removal path, and
// [Sketch.CheckConstraint]'s rollback for the allocation a probe made. The
// pointer records where the stored indices mean something, so leaving it behind
// a retirement states an ownership that no longer exists: the two doors that
// parameterize a constraint refuse a candidate another sketch HOLDS an allocation
// from, and on a stale pointer they would refuse a constraint whose variables were
// handed back, making a legitimate remove-here / add-there sequence fail. That
// refusal reads the constraint's own aux index fields first (auxAllocatedOf), so a
// type declaring auxAllocated is already covered whatever this pointer says; the
// clearing is what carries a type that does not declare it, for which the pointer
// is the whole answer.
func (a *auxOwner) clearAuxOwner() { a.s = nil }

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

// pointOnNormal holds P on the line through At that is normal to C. C is a
// *Line or an *Arc (the constructor's type constraint admits nothing else).
type pointOnNormal struct {
	P, At *Point
	C     Entity
}

func (c *pointOnNormal) residual(out []float64) []float64 {
	if l, ok := c.C.(*Line); ok {
		// signed distance of P from the perpendicular to l through At (length units)
		ux, uy := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		dx, dy := c.P.x()-c.At.x(), c.P.y()-c.At.y()
		return append(out, (dx*ux+dy*uy)/norm(ux, uy))
	}
	if a, ok := c.C.(*Arc); ok {
		// signed distance of P from the radial line through the center and At
		rx, ry := c.At.x()-a.Center.x(), c.At.y()-a.Center.y()
		px, py := c.P.x()-a.Center.x(), c.P.y()-a.Center.y()
		return append(out, (rx*py-ry*px)/norm(rx, ry))
	}
	return append(out, 1)
}

// NewPointOnNormal holds p on the line through at that is normal to c: for a
// line, the perpendicular to its direction through at; for an arc, the radial
// line through its center and at. The residual is the distance of p from that
// line, in length units. The offset tools use it to keep an offset end on the
// source curve's normal at the source end.
func NewPointOnNormal[T OpenCurve](p *Point, c T, at *Point) Constraint {
	return &pointOnNormal{P: p, At: at, C: asEntity(c)}
}

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
	P        *Point
	A        *Arc
	auxOwner     // set by allocVars: the sketch these variable indices belong to
	slack    int // sweep slack var index; -1 = not yet allocated
}

// NewPointOnArc forces a point to lie on an arc — on its circle and within its
// counter-clockwise sweep. A point that lies on the arc's full circle but
// outside the sweep is reported unsolvable, not on the arc.
func NewPointOnArc(p *Point, a *Arc) Constraint { return &pointOnArc{P: p, A: a, slack: -1} }

// unitDir returns the unit direction from (cx,cy) toward (px,py), using the
// floored norm() so a coincident pair yields a finite (near-zero) vector rather
// than NaN.
func unitDir(px, py, cx, cy float64) (float64, float64) {
	dx, dy := px-cx, py-cy
	n := norm(dx, dy)
	return dx / n, dy / n
}

func (c *pointOnArc) contactDir() (float64, float64) {
	return unitDir(c.P.x(), c.P.y(), c.A.Center.x(), c.A.Center.y())
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
	A1, A2   *Arc
	Axis     *Line
	auxOwner     // set by allocVars: the sketch these variable indices belong to
	slack    int // branch slack var; -1 = unallocated
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
