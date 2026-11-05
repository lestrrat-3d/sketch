package sketch

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/units"
)

// --- dimensional constraints ------------------------------------------------

// dimBase is embedded by every dimensional constraint. It holds the driving
// target as a unit-carrying [units.Value] (or, when bound, the parameter
// expression that produces it) and the kind of quantity the dimension measures.
type dimBase struct {
	kind       units.Kind
	target     units.Value
	baseTarget float64 // target in base units, updated with target
	deflt      bool    // target's unit is a placeholder, replaced by the sketch default on add
	expr       string  // bound parameter expression; empty when the value is literal
	driven     bool    // reference dimension: measures the geometry instead of driving it
}

// Kind reports whether the dimension measures a length or an angle.
func (d *dimBase) Kind() units.Kind { return d.kind }

// Target returns the dimension's current driving value as a unit-carrying
// value.
func (d *dimBase) Target() units.Value { return d.target }

func (d *dimBase) setTarget(v units.Value) {
	d.target = v
	d.baseTarget = v.Base()
}

// Set changes the driving magnitude, keeping the dimension's current unit, and
// clears any parameter binding. Call [Sketch.Solve] again to apply it.
func (d *dimBase) Set(v float64) {
	d.setTarget(units.New(v, d.target.Unit()))
	d.expr = ""
}

// SetValue sets the driving value to a typed quantity (which must measure the
// dimension's kind) and clears any binding. The value keeps its own unit; no
// conversion takes place here — the units library converts on demand (e.g. via
// [dimBase.base] for the solver).
func (d *dimBase) SetValue(v units.Value) error {
	if err := d.checkKind(v); err != nil {
		return err
	}
	d.setTarget(v)
	d.deflt = false
	d.expr = ""
	return nil
}

// checkKind reports an error if v does not measure the dimension's kind.
func (d *dimBase) checkKind(v units.Value) error {
	if v.Kind() != d.kind {
		return fmt.Errorf("sketch: cannot set %s dimension from a %s value", d.kind, v.Kind())
	}
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
		d.setTarget(units.New(d.target.Mag(), s.unitFor(d.kind)))
		d.deflt = false
	}
}

// base returns the target in the kind's base unit (mm or rad) for the solver.
func (d *dimBase) base() float64 { return d.baseTarget }

// setResolved stores a value produced by evaluating a parameter binding. A
// quantity keeps its own unit; a dimensionless result is taken to already be in
// the dimension's current unit.
func (d *dimBase) setResolved(v units.Value) error {
	if v.Kind() == units.Dimensionless {
		d.setTarget(units.New(v.Mag(), d.target.Unit()))
		return nil
	}
	if err := d.checkKind(v); err != nil {
		return err
	}
	d.setTarget(v)
	return nil
}

func (d *dimBase) driverExpr() string     { return d.expr }
func (d *dimBase) setDriverExpr(e string) { d.expr = e }

// restore sets the target verbatim from a deserialized magnitude and unit. It
// reinstates saved state and does not convert.
func (d *dimBase) restore(mag float64, u units.Unit) {
	d.setTarget(units.New(mag, u))
	d.deflt = false
}

// lengthDim and angleDim build a detached dimension whose unit is a placeholder
// (the metric default) to be resolved to the sketch's default unit on add.
func lengthDim(v float64) dimBase {
	d := dimBase{kind: units.Length, deflt: true}
	d.setTarget(units.Millimeters(v))
	return d
}

func angleDim(v float64) dimBase {
	d := dimBase{kind: units.Angle, deflt: true}
	d.setTarget(units.Degrees(v))
	return d
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
	P        *Point
	A        *Arc
	auxOwner     // set by allocVars: the sketch these variable indices belong to
	slack    int // sweep slack var index; -1 = none (driven or not yet allocated)
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

// auxAllocated reports whether this dimension currently holds its sweep slack.
// It is what foreignAllocation asks after the owner pointer, since a driven
// dimension records an owner while owning no variable.
func (c *DistancePointArc) auxAllocated() bool { return c.slack >= 0 }

// setDrivenAux is the shared driving↔driven toggle scaffold for the aux-var
// dimensions (DistancePointArc/DistanceLineArc/ArcLength). It flips *driven,
// then — only for a committed dimension (membership in s.cons, not merely a
// non-nil s, since a CheckConstraint probe sets s via a temporary allocVars
// without registering and must not leak an orphan variable) — retires the aux
// variable when switching to driven and re-allocates it (via realloc) when
// switching back to driving and the aux index is currently unallocated. retire
// and realloc carry each dimension's own aux field and realloc seed.
func setDrivenAux(c Constraint, s *Sketch, driven *bool, v bool, auxIdx int, retire, realloc func()) {
	if v == *driven {
		return
	}
	*driven = v
	if s == nil || !containsConstraint(s.cons, c) {
		return
	}
	if v {
		retire()
	} else if auxIdx < 0 {
		realloc()
	}
}

// SetDriven toggles reference (driven) mode and keeps the sweep slack consistent:
// a driven dimension contributes no residual rows, so its slack would be an
// unconstrained free DOF — switching to driven retires it, switching back
// re-allocates it. Mirrors [ArcLength.SetDriven]; the committed test guards
// against a CheckConstraint probe (which sets c.s via a temporary allocVars
// without registering the dimension) leaking an orphan variable.
func (c *DistancePointArc) SetDriven(v bool) {
	setDrivenAux(c, c.s, &c.driven, v, c.slack, func() { c.retireVars(c.s) }, func() {
		ux, uy := arcRadialDir(c.P, c.A.Center)
		c.slack = c.s.newVar(slackFor(arcInSweepExcess(c.A, ux, uy)))
	})
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
	L        *Line
	A        *Arc
	auxOwner // set by allocVars: the sketch these variable indices belong to
	slack    int
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

// auxAllocated mirrors [DistancePointArc.auxAllocated].
func (c *DistanceLineArc) auxAllocated() bool { return c.slack >= 0 }

// SetDriven mirrors [DistancePointArc.SetDriven].
func (c *DistanceLineArc) SetDriven(v bool) {
	setDrivenAux(c, c.s, &c.driven, v, c.slack, func() { c.retireVars(c.s) }, func() {
		ux, uy := lineFootDir(c.L, c.A.Center)
		c.slack = c.s.newVar(slackFor(arcInSweepExcess(c.A, ux, uy)))
	})
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
	return unitDir(p.x(), p.y(), center.x(), center.y())
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

// RadialOffset is an editable radius-difference dimension: it drives Dst's
// radius to Src's radius plus d, so positive d makes Dst larger. It does not hold
// the two curves concentric: [CreateOffset] gives the copy the source's center
// point, and a direct caller shares the center the same way or adds
// [NewConcentric].
type RadialOffset struct {
	dimBase
	Src, Dst Circular
}

func (c *RadialOffset) residual(out []float64) []float64 {
	return append(out, c.Dst.R()-c.Src.R()-c.base()) // length units
}

// NewRadialOffset constrains dst's radius to src's radius plus d (positive d is
// outward). Both operands are circles or arcs; an arc's radius is read from its
// Start, as [Arc.R] does.
func NewRadialOffset(src, dst Circular, d float64) *RadialOffset {
	return &RadialOffset{dimBase: lengthDim(d), Src: src, Dst: dst}
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
	A        *Arc
	auxOwner     // set by allocVars: the sketch these variable indices belong to
	theta    int // unwrapped-sweep aux var index; -1 = not yet allocated
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

// auxAllocated reports whether this dimension currently holds its unwrapped-sweep
// variable, mirroring [DistancePointArc.auxAllocated].
func (c *ArcLength) auxAllocated() bool { return c.theta >= 0 }

// SetDriven toggles reference (driven) mode and keeps the unwrapped-sweep aux
// variable consistent: a driven dimension contributes no residual rows, so its
// theta would be an unconstrained free DOF — switching to driven retires it, and
// switching back to driving re-allocates it. Until the dimension is actually
// committed (registered in the sketch) it only records the flag; allocVars then
// honors it. Membership — not merely c.s != nil — is the committed test, because
// CheckConstraint sets c.s via a temporary allocVars without registering the
// dimension; mutating s.vars there would leak an orphan variable.
func (c *ArcLength) SetDriven(v bool) {
	setDrivenAux(c, c.s, &c.driven, v, c.theta, func() { c.retireVars(c.s) }, func() {
		c.theta = c.s.newVar(c.A.Sweep())
	})
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
	r1 := wrapPi(math.Atan2(cross, dot) - theta)
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
	// wrap the residual into (-π, π] so it stays continuous; target in base
	// (radian) units.
	r := wrapPi(ang - c.base())
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
	r := wrapPi(c.E.Rotation() - c.base())
	return append(out, r)
}

// NewEllipseRotation constrains the rotation of an ellipse's local frame: a
// signed angle measured counterclockwise from the global +x axis, wrapping
// modulo a full turn. The value a is interpreted in the sketch's default angle
// unit once added. It accepts a [*Ellipse] or an [*EllipticalArc].
func NewEllipseRotation(e Elliptical, a float64) *EllipseRotation {
	return &EllipseRotation{dimBase: angleDim(a), E: e}
}
