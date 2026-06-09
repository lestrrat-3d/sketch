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

// NewParallel forces two lines to be parallel.
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

type equalRadii struct{ C1, C2 *Circle }

func (c *equalRadii) residual(out []float64) []float64 {
	return append(out, c.C1.r()-c.C2.r())
}

// NewEqualRadius forces two circles to have equal radius.
func NewEqualRadius(c1, c2 *Circle) Constraint { return &equalRadii{c1, c2} }

// --- tangent ----------------------------------------------------------------

type tangentLineCircle struct {
	L *Line
	C *Circle
}

func (c *tangentLineCircle) residual(out []float64) []float64 {
	// |distance(center, line)| − r, in length units
	ax, ay := c.L.Start.x(), c.L.Start.y()
	abx, aby := c.L.End.x()-ax, c.L.End.y()-ay
	acx, acy := c.C.Center.x()-ax, c.C.Center.y()-ay
	cross := abx*acy - aby*acx
	return append(out, math.Abs(cross)/norm(abx, aby)-c.C.r())
}

// NewTangent forces a line to be tangent to a circle.
func NewTangent(l *Line, c *Circle) Constraint { return &tangentLineCircle{l, c} }

type tangentCircles struct {
	C1, C2   *Circle
	Internal bool
}

func (c *tangentCircles) residual(out []float64) []float64 {
	d := dist(c.C1.Center, c.C2.Center)
	sum := c.C1.r() + c.C2.r()
	if c.Internal {
		sum = math.Abs(c.C1.r() - c.C2.r())
	}
	return append(out, d-sum) // length units
}

// NewTangentCircles forces two circles to be tangent. When internal is true the
// circles are internally tangent (one inside the other); otherwise they are
// externally tangent.
func NewTangentCircles(c1, c2 *Circle, internal bool) Constraint {
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

// Angle is an editable angle dimension between two lines, measured from L1 to
// L2.
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

// NewAngle constrains the angle from line l1 to line l2. The value a is
// interpreted in the sketch's default angle unit (degrees for [units.Metric])
// once added; use [Angle.SetValue] with a typed quantity such as units.Radians
// for another unit.
func NewAngle(l1, l2 *Line, a float64) *Angle {
	return &Angle{dimBase: angleDim(a), L1: l1, L2: l2}
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
