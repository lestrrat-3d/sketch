package sketch

import "math"

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

// add appends a constraint and returns it.
func (s *Sketch) add(c Constraint) Constraint {
	s.cons = append(s.cons, c)
	return c
}

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

// Coincident forces two points to occupy the same location.
func (s *Sketch) Coincident(p1, p2 *Point) Constraint {
	return s.add(&coincident{p1, p2})
}

// --- horizontal / vertical --------------------------------------------------

type horizontal struct{ L *Line }

func (c *horizontal) residual(out []float64) []float64 {
	return append(out, c.L.A.y()-c.L.B.y())
}

// Horizontal forces a line to be horizontal.
func (s *Sketch) Horizontal(l *Line) Constraint { return s.add(&horizontal{l}) }

type vertical struct{ L *Line }

func (c *vertical) residual(out []float64) []float64 {
	return append(out, c.L.A.x()-c.L.B.x())
}

// Vertical forces a line to be vertical.
func (s *Sketch) Vertical(l *Line) Constraint { return s.add(&vertical{l}) }

// --- parallel / perpendicular ----------------------------------------------

type parallel struct{ L1, L2 *Line }

func (c *parallel) residual(out []float64) []float64 {
	d1x, d1y := dir(c.L1)
	d2x, d2y := dir(c.L2)
	// normalized cross product == sin(angle), dimensionless
	return append(out, (d1x*d2y-d1y*d2x)/(norm(d1x, d1y)*norm(d2x, d2y)))
}

// Parallel forces two lines to be parallel.
func (s *Sketch) Parallel(l1, l2 *Line) Constraint { return s.add(&parallel{l1, l2}) }

type perpendicular struct{ L1, L2 *Line }

func (c *perpendicular) residual(out []float64) []float64 {
	d1x, d1y := dir(c.L1)
	d2x, d2y := dir(c.L2)
	// normalized dot product == cos(angle), dimensionless
	return append(out, (d1x*d2x+d1y*d2y)/(norm(d1x, d1y)*norm(d2x, d2y)))
}

// Perpendicular forces two lines to be perpendicular.
func (s *Sketch) Perpendicular(l1, l2 *Line) Constraint { return s.add(&perpendicular{l1, l2}) }

// --- collinear / point-on --------------------------------------------------

type pointOnLine struct {
	P *Point
	L *Line
}

func (c *pointOnLine) residual(out []float64) []float64 {
	// signed perpendicular distance from P to the line (length units)
	ax, ay := c.L.A.x(), c.L.A.y()
	abx, aby := c.L.B.x()-ax, c.L.B.y()-ay
	apx, apy := c.P.x()-ax, c.P.y()-ay
	return append(out, (abx*apy-aby*apx)/norm(abx, aby))
}

// PointOnLine forces a point to lie on the infinite line through a segment.
func (s *Sketch) PointOnLine(p *Point, l *Line) Constraint { return s.add(&pointOnLine{p, l}) }

// Collinear forces two lines to share the same infinite line by placing both
// endpoints of the second line on the first.
func (s *Sketch) Collinear(l1, l2 *Line) Constraint {
	s.add(&pointOnLine{l2.A, l1})
	return s.add(&pointOnLine{l2.B, l1})
}

type pointOnCircle struct {
	P *Point
	C *Circle
}

func (c *pointOnCircle) residual(out []float64) []float64 {
	dx := c.P.x() - c.C.Center.x()
	dy := c.P.y() - c.C.Center.y()
	return append(out, norm(dx, dy)-c.C.r()) // length units
}

// PointOnCircle forces a point to lie on a circle.
func (s *Sketch) PointOnCircle(p *Point, c *Circle) Constraint { return s.add(&pointOnCircle{p, c}) }

// --- midpoint / symmetric ---------------------------------------------------

type midpoint struct {
	P *Point
	L *Line
}

func (c *midpoint) residual(out []float64) []float64 {
	return append(out,
		c.P.x()-(c.L.A.x()+c.L.B.x())/2,
		c.P.y()-(c.L.A.y()+c.L.B.y())/2,
	)
}

// Midpoint forces a point to be the midpoint of a line.
func (s *Sketch) Midpoint(p *Point, l *Line) Constraint { return s.add(&midpoint{p, l}) }

type symmetric struct {
	P1, P2 *Point
	Axis   *Line
}

func (c *symmetric) residual(out []float64) []float64 {
	ax, ay := c.Axis.A.x(), c.Axis.A.y()
	abx, aby := c.Axis.B.x()-ax, c.Axis.B.y()-ay
	// midpoint of P1P2 lies on the axis
	mx := (c.P1.x()+c.P2.x())/2 - ax
	my := (c.P1.y()+c.P2.y())/2 - ay
	axisLen := norm(abx, aby)
	onAxis := (abx*my - aby*mx) / axisLen // midpoint's distance off the axis
	// P1P2 is perpendicular to the axis
	perp := ((c.P2.x()-c.P1.x())*abx + (c.P2.y()-c.P1.y())*aby) / axisLen
	return append(out, onAxis, perp)
}

// Symmetric forces two points to be mirror images across an axis line.
func (s *Sketch) Symmetric(p1, p2 *Point, axis *Line) Constraint {
	return s.add(&symmetric{p1, p2, axis})
}

// --- concentric -------------------------------------------------------------

// Concentric forces two circles to share a center.
func (s *Sketch) Concentric(c1, c2 *Circle) Constraint {
	return s.add(&coincident{c1.Center, c2.Center})
}

// --- equal ------------------------------------------------------------------

type equalLines struct{ L1, L2 *Line }

func (c *equalLines) residual(out []float64) []float64 {
	return append(out, c.L1.Length()-c.L2.Length()) // length units
}

// Equal forces two lines to have equal length.
func (s *Sketch) Equal(l1, l2 *Line) Constraint { return s.add(&equalLines{l1, l2}) }

type equalRadii struct{ C1, C2 *Circle }

func (c *equalRadii) residual(out []float64) []float64 {
	return append(out, c.C1.r()-c.C2.r())
}

// EqualRadius forces two circles to have equal radius.
func (s *Sketch) EqualRadius(c1, c2 *Circle) Constraint { return s.add(&equalRadii{c1, c2}) }

// --- tangent ----------------------------------------------------------------

type tangentLineCircle struct {
	L *Line
	C *Circle
}

func (c *tangentLineCircle) residual(out []float64) []float64 {
	// |distance(center, line)| − r, in length units
	ax, ay := c.L.A.x(), c.L.A.y()
	abx, aby := c.L.B.x()-ax, c.L.B.y()-ay
	acx, acy := c.C.Center.x()-ax, c.C.Center.y()-ay
	cross := abx*acy - aby*acx
	return append(out, math.Abs(cross)/norm(abx, aby)-c.C.r())
}

// Tangent forces a line to be tangent to a circle.
func (s *Sketch) Tangent(l *Line, c *Circle) Constraint { return s.add(&tangentLineCircle{l, c}) }

type tangentCircles struct {
	C1, C2   *Circle
	Internal bool
}

func (c *tangentCircles) residual(out []float64) []float64 {
	d := dist(c.C1.Center, c.C2.Center)
	var sum float64
	if c.Internal {
		sum = math.Abs(c.C1.r() - c.C2.r())
	} else {
		sum = c.C1.r() + c.C2.r()
	}
	return append(out, d-sum) // length units
}

// TangentCircles forces two circles to be tangent. When internal is true the
// circles are internally tangent (one inside the other); otherwise they are
// externally tangent.
func (s *Sketch) TangentCircles(c1, c2 *Circle, internal bool) Constraint {
	return s.add(&tangentCircles{c1, c2, internal})
}

// --- dimensional constraints ------------------------------------------------

// Distance is an editable point-to-point distance dimension.
//
// Like every dimension type, its driving Value may instead be bound to a
// parameter expression with [Sketch.Bind]; the bound expression (Expr) is
// re-evaluated against the sketch's parameter table before each solve.
type Distance struct {
	P1, P2 *Point
	Value  float64
	Expr   string // bound parameter expression; empty when the value is literal
}

func (c *Distance) residual(out []float64) []float64 {
	return append(out, dist(c.P1, c.P2)-c.Value)
}

// Set changes the driving value of the dimension and clears any parameter
// binding. Call [Sketch.Solve] again to apply it.
func (c *Distance) Set(v float64)          { c.Value = v; c.Expr = "" }
func (c *Distance) value() float64         { return c.Value }
func (c *Distance) setValue(v float64)     { c.Value = v }
func (c *Distance) driverExpr() string     { return c.Expr }
func (c *Distance) setDriverExpr(e string) { c.Expr = e }

// Distance constrains the straight-line distance between two points.
func (s *Sketch) Distance(p1, p2 *Point, d float64) *Distance {
	c := &Distance{P1: p1, P2: p2, Value: d}
	s.add(c)
	return c
}

// HorizontalDistance is an editable signed horizontal (Δx) dimension.
type HorizontalDistance struct {
	P1, P2 *Point
	Value  float64
	Expr   string
}

func (c *HorizontalDistance) residual(out []float64) []float64 {
	return append(out, c.P2.x()-c.P1.x()-c.Value)
}

// Set changes the driving value of the dimension and clears any binding.
func (c *HorizontalDistance) Set(v float64)          { c.Value = v; c.Expr = "" }
func (c *HorizontalDistance) value() float64         { return c.Value }
func (c *HorizontalDistance) setValue(v float64)     { c.Value = v }
func (c *HorizontalDistance) driverExpr() string     { return c.Expr }
func (c *HorizontalDistance) setDriverExpr(e string) { c.Expr = e }

// HorizontalDistance constrains the signed horizontal distance (x2−x1).
func (s *Sketch) HorizontalDistance(p1, p2 *Point, d float64) *HorizontalDistance {
	c := &HorizontalDistance{P1: p1, P2: p2, Value: d}
	s.add(c)
	return c
}

// VerticalDistance is an editable signed vertical (Δy) dimension.
type VerticalDistance struct {
	P1, P2 *Point
	Value  float64
	Expr   string
}

func (c *VerticalDistance) residual(out []float64) []float64 {
	return append(out, c.P2.y()-c.P1.y()-c.Value)
}

// Set changes the driving value of the dimension and clears any binding.
func (c *VerticalDistance) Set(v float64)          { c.Value = v; c.Expr = "" }
func (c *VerticalDistance) value() float64         { return c.Value }
func (c *VerticalDistance) setValue(v float64)     { c.Value = v }
func (c *VerticalDistance) driverExpr() string     { return c.Expr }
func (c *VerticalDistance) setDriverExpr(e string) { c.Expr = e }

// VerticalDistance constrains the signed vertical distance (y2−y1).
func (s *Sketch) VerticalDistance(p1, p2 *Point, d float64) *VerticalDistance {
	c := &VerticalDistance{P1: p1, P2: p2, Value: d}
	s.add(c)
	return c
}

// Radius is an editable radius dimension.
type Radius struct {
	C     *Circle
	Value float64
	Expr  string
}

func (c *Radius) residual(out []float64) []float64 {
	return append(out, c.C.r()-c.Value)
}

// Set changes the driving value of the dimension and clears any binding.
func (c *Radius) Set(v float64)          { c.Value = v; c.Expr = "" }
func (c *Radius) value() float64         { return c.Value }
func (c *Radius) setValue(v float64)     { c.Value = v }
func (c *Radius) driverExpr() string     { return c.Expr }
func (c *Radius) setDriverExpr(e string) { c.Expr = e }

// Radius constrains a circle's radius.
func (s *Sketch) Radius(c *Circle, r float64) *Radius {
	rc := &Radius{C: c, Value: r}
	s.add(rc)
	return rc
}

// Diameter is an editable diameter dimension.
type Diameter struct {
	C     *Circle
	Value float64
	Expr  string
}

func (c *Diameter) residual(out []float64) []float64 {
	return append(out, 2*c.C.r()-c.Value)
}

// Set changes the driving value of the dimension and clears any binding.
func (c *Diameter) Set(v float64)          { c.Value = v; c.Expr = "" }
func (c *Diameter) value() float64         { return c.Value }
func (c *Diameter) setValue(v float64)     { c.Value = v }
func (c *Diameter) driverExpr() string     { return c.Expr }
func (c *Diameter) setDriverExpr(e string) { c.Expr = e }

// Diameter constrains a circle's diameter.
func (s *Sketch) Diameter(c *Circle, d float64) *Diameter {
	dc := &Diameter{C: c, Value: d}
	s.add(dc)
	return dc
}

// Angle is an editable angle dimension between two lines, measured from L1 to
// L2 in radians.
type Angle struct {
	L1, L2 *Line
	Value  float64
	Expr   string
}

func (c *Angle) residual(out []float64) []float64 {
	d1x, d1y := dir(c.L1)
	d2x, d2y := dir(c.L2)
	cross := d1x*d2y - d1y*d2x
	dot := d1x*d2x + d1y*d2y
	ang := math.Atan2(cross, dot)
	r := ang - c.Value
	// wrap into (-π, π] so the residual is continuous
	for r > math.Pi {
		r -= 2 * math.Pi
	}
	for r <= -math.Pi {
		r += 2 * math.Pi
	}
	return append(out, r)
}

// Set changes the driving value of the dimension (radians) and clears any
// binding.
func (c *Angle) Set(v float64)          { c.Value = v; c.Expr = "" }
func (c *Angle) value() float64         { return c.Value }
func (c *Angle) setValue(v float64)     { c.Value = v }
func (c *Angle) driverExpr() string     { return c.Expr }
func (c *Angle) setDriverExpr(e string) { c.Expr = e }

// Angle constrains the angle from line l1 to line l2 (radians).
func (s *Sketch) Angle(l1, l2 *Line, radians float64) *Angle {
	c := &Angle{L1: l1, L2: l2, Value: radians}
	s.add(c)
	return c
}

// --- geometry helpers -------------------------------------------------------

func dir(l *Line) (dx, dy float64) { return l.B.x() - l.A.x(), l.B.y() - l.A.y() }

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
