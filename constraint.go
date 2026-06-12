package sketch

import (
	"fmt"
	"math"

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

// --- concentric -------------------------------------------------------------

type concentric struct{ C1, C2 *Circle }

func (c *concentric) residual(out []float64) []float64 {
	return (&coincident{c.C1.Center, c.C2.Center}).residual(out)
}

// NewConcentric forces two circles to share a center.
func NewConcentric(c1, c2 *Circle) Constraint { return &concentric{c1, c2} }

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
	// Sampson distance: |F|/|∇F| for the implicit ellipse equation
	// F = (x'/rx)² + (y'/ry)² − 1 in the ellipse's local frame. This first-
	// order approximation of the true point-to-ellipse distance keeps the
	// residual in length units, per the normalization convention.
	e := c.E
	cosr, sinr := math.Cos(e.rot()), math.Sin(e.rot())
	dx, dy := c.P.x()-e.Center.x(), c.P.y()-e.Center.y()
	lx := cosr*dx + sinr*dy
	ly := -sinr*dx + cosr*dy
	rx2 := math.Max(e.rx()*e.rx(), 1e-12)
	ry2 := math.Max(e.ry()*e.ry(), 1e-12)
	fv := lx*lx/rx2 + ly*ly/ry2 - 1
	return append(out, fv/norm(2*lx/rx2, 2*ly/ry2))
}

// NewPointOnEllipse forces a point to lie on an ellipse.
func NewPointOnEllipse(p *Point, e *Ellipse) Constraint { return &pointOnEllipse{p, e} }

// --- tangent ----------------------------------------------------------------

type tangentLineCircle struct {
	L *Line
	C Circular
}

func (c *tangentLineCircle) residual(out []float64) []float64 {
	// |distance(center, line)| − r, in length units
	ctr := c.C.centerPt()
	ax, ay := c.L.Start.x(), c.L.Start.y()
	abx, aby := c.L.End.x()-ax, c.L.End.y()-ay
	acx, acy := ctr.x()-ax, ctr.y()-ay
	cross := abx*acy - aby*acx
	return append(out, math.Abs(cross)/norm(abx, aby)-c.C.R())
}

// NewTangent forces a line to be tangent to a circular entity (circle or arc).
// The tangency is unsigned: the circle stays on whichever side of the line it
// starts. An arc is treated as its full circle: the tangent point is not
// required to lie within the arc's sweep.
func NewTangent(l *Line, c Circular) Constraint { return &tangentLineCircle{l, c} }

type tangentCircles struct {
	C1, C2   Circular
	Internal bool
}

func (c *tangentCircles) residual(out []float64) []float64 {
	d := dist(c.C1.centerPt(), c.C2.centerPt())
	sum := c.C1.R() + c.C2.R()
	if c.Internal {
		sum = math.Abs(c.C1.R() - c.C2.R())
	}
	return append(out, d-sum) // length units
}

// NewTangentCircles forces two circular entities (circles or arcs) to be
// tangent. When internal is true they are internally tangent (one inside the
// other — which one is inside is decided by the radii and starting positions,
// not by the constraint); otherwise they are externally tangent. An arc is
// treated as its full circle: the tangent point is not required to lie within
// the arc's sweep.
func NewTangentCircles(c1, c2 Circular, internal bool) Constraint {
	return &tangentCircles{c1, c2, internal}
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
	C *Circle
}

func (c *Radius) residual(out []float64) []float64 {
	return append(out, c.C.r()-c.base())
}

// NewRadius constrains a circle's radius.
func NewRadius(c *Circle, r float64) *Radius {
	return &Radius{dimBase: lengthDim(r), C: c}
}

// Diameter is an editable diameter dimension.
type Diameter struct {
	dimBase
	C *Circle
}

func (c *Diameter) residual(out []float64) []float64 {
	return append(out, 2*c.C.r()-c.base())
}

// NewDiameter constrains a circle's diameter.
func NewDiameter(c *Circle, d float64) *Diameter {
	return &Diameter{dimBase: lengthDim(d), C: c}
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
	E *Ellipse
}

func (c *SemiMajor) residual(out []float64) []float64 {
	return append(out, c.E.rx()-c.base()) // length units
}

// NewSemiMajor constrains an ellipse's semi-axis along its local x axis.
func NewSemiMajor(e *Ellipse, r float64) *SemiMajor {
	return &SemiMajor{dimBase: lengthDim(r), E: e}
}

// SemiMinor is an editable dimension on an ellipse's semi-axis along its local
// y axis (the minor axis by convention; not enforced).
type SemiMinor struct {
	dimBase
	E *Ellipse
}

func (c *SemiMinor) residual(out []float64) []float64 {
	return append(out, c.E.ry()-c.base()) // length units
}

// NewSemiMinor constrains an ellipse's semi-axis along its local y axis.
func NewSemiMinor(e *Ellipse, r float64) *SemiMinor {
	return &SemiMinor{dimBase: lengthDim(r), E: e}
}

// EllipseRotation is an editable dimension on the rotation of an ellipse's
// local frame.
type EllipseRotation struct {
	dimBase
	E *Ellipse
}

func (c *EllipseRotation) residual(out []float64) []float64 {
	// wrap into (-π, π] so the residual stays continuous, like Angle
	r := math.Mod(c.E.rot()-c.base(), 2*math.Pi)
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
// unit once added.
func NewEllipseRotation(e *Ellipse, a float64) *EllipseRotation {
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
