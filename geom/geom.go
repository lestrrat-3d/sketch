package geom

import "math"

// Point is a 2D point: pure transient geometry, a lightweight coordinate
// wrapper used as math input/output and as the snapshot type returned by a
// sketch entity's Geometry method. It carries no document state.
type Point struct {
	X, Y float64
}

// NewPoint returns a point at (x, y).
func NewPoint(x, y float64) *Point { return &Point{X: x, Y: y} }

// Line is a straight segment between two points.
type Line struct {
	Start, End *Point
}

// NewLine returns a line between start and end.
func NewLine(start, end *Point) *Line { return &Line{Start: start, End: end} }

// Length returns the distance between the line's endpoints.
func (l *Line) Length() float64 { return math.Hypot(l.End.X-l.Start.X, l.End.Y-l.Start.Y) }

// Circle is a full circle defined by a center point and a radius.
type Circle struct {
	Center *Point
	Radius float64
}

// NewCircle returns a circle with the given center and radius.
func NewCircle(center *Point, radius float64) *Circle {
	return &Circle{Center: center, Radius: radius}
}

// Ellipse is a full ellipse: a center, semi-axis lengths Rx and Ry along its
// local x and y axes, and the rotation (radians, counter-clockwise) of that
// local frame in sketch coordinates. Rx is conventionally the major semi-axis
// but this is not enforced; the axes are simply the local x and y.
type Ellipse struct {
	Center   *Point
	Rx, Ry   float64
	Rotation float64
}

// NewEllipse returns an ellipse with the given center, semi-axes and rotation.
func NewEllipse(center *Point, rx, ry, rotation float64) *Ellipse {
	return &Ellipse{Center: center, Rx: rx, Ry: ry, Rotation: rotation}
}

// Arc is a circular arc swept counter-clockwise from Start to End about Center.
type Arc struct {
	Center, Start, End *Point
}

// NewArc returns an arc swept counter-clockwise from start to end about center.
func NewArc(center, start, end *Point) *Arc {
	return &Arc{Center: center, Start: start, End: end}
}

// Radius returns the arc's radius (distance from center to start).
func (a *Arc) Radius() float64 { return math.Hypot(a.Start.X-a.Center.X, a.Start.Y-a.Center.Y) }

// StartAngle returns the angle (radians) of the start point about the center.
func (a *Arc) StartAngle() float64 { return math.Atan2(a.Start.Y-a.Center.Y, a.Start.X-a.Center.X) }

// EndAngle returns the angle (radians) of the end point about the center.
func (a *Arc) EndAngle() float64 { return math.Atan2(a.End.Y-a.Center.Y, a.End.X-a.Center.X) }

// wrapSweep returns the counter-clockwise angular sweep from start to end,
// wrapped into (0, 2π].
func wrapSweep(start, end float64) float64 {
	d := math.Mod(end-start, 2*math.Pi)
	if d <= 0 {
		d += 2 * math.Pi
	}
	return d
}

// Sweep returns the counter-clockwise sweep angle in (0, 2π].
func (a *Arc) Sweep() float64 {
	return wrapSweep(a.StartAngle(), a.EndAngle())
}

// EllipticalArc is an arc on an ellipse: an ellipse (center, semi-axes Rx/Ry,
// local-frame Rotation) restricted to the counter-clockwise sweep from Start to
// End. Start and End lie on the ellipse; the swept extent is measured in the
// ellipse's eccentric angle (the parameter t in (Rx·cos t, Ry·sin t)).
type EllipticalArc struct {
	Center, Start, End *Point
	Rx, Ry             float64
	Rotation           float64
}

// NewEllipticalArc returns an elliptical arc swept counter-clockwise (in
// eccentric angle) from start to end on the ellipse (center, rx, ry, rotation).
func NewEllipticalArc(center, start, end *Point, rx, ry, rotation float64) *EllipticalArc {
	return &EllipticalArc{Center: center, Start: start, End: end, Rx: rx, Ry: ry, Rotation: rotation}
}

// eccentric returns the ellipse's eccentric angle of a world point p — the
// parameter t such that p ≈ Center + R(rot)·(Rx·cos t, Ry·sin t).
func (e *EllipticalArc) eccentric(p *Point) float64 {
	cosr, sinr := math.Cos(e.Rotation), math.Sin(e.Rotation)
	dx, dy := p.X-e.Center.X, p.Y-e.Center.Y
	lx := cosr*dx + sinr*dy // local-frame coordinates
	ly := -sinr*dx + cosr*dy
	return math.Atan2(ly/floor(e.Ry), lx/floor(e.Rx))
}

// StartParam and EndParam return the eccentric angles of the endpoints.
func (e *EllipticalArc) StartParam() float64 { return e.eccentric(e.Start) }
func (e *EllipticalArc) EndParam() float64   { return e.eccentric(e.End) }

// Sweep returns the counter-clockwise eccentric-angle sweep in (0, 2π].
func (e *EllipticalArc) Sweep() float64 {
	return wrapSweep(e.StartParam(), e.EndParam())
}

// Endpoints returns the elliptical arc's start and end points, so it satisfies
// the open-curve Curve interface.
func (e *EllipticalArc) Endpoints() (*Point, *Point) { return e.Start, e.End }

// Conic is a conic arc represented exactly as a rational quadratic Bézier: two
// endpoints Start and End, an apex control point Apex (the intersection of the
// endpoint tangents), and a fullness parameter Rho in (0, 1). Rho selects the
// conic family — Rho < 0.5 is an ellipse arc, Rho = 0.5 a parabola, Rho > 0.5 a
// hyperbola arc — via the apex weight w = Rho/(1−Rho) (the endpoints have weight
// 1). The curve passes through Start and End and is tangent to Start→Apex and
// End→Apex.
type Conic struct {
	Start, Apex, End *Point
	Rho              float64
}

// NewConic returns a conic arc through start and end with apex control point
// apex and fullness rho.
func NewConic(start, apex, end *Point, rho float64) *Conic {
	return &Conic{Start: start, Apex: apex, End: end, Rho: rho}
}

// weight returns the apex Bézier weight w = rho/(1−rho); rho is kept in (0, 1)
// by the constructor, so the denominator never vanishes (floored for safety).
func (c *Conic) weight() float64 { return c.Rho / floor(1-c.Rho) }

// Eval returns the curve point at parameter t in [0, 1]. Eval(0) = Start and
// Eval(1) = End.
func (c *Conic) Eval(t float64) (float64, float64) {
	w := c.weight()
	u := 1 - t
	b0, b1, b2 := u*u, 2*u*t*w, t*t
	den := b0 + b1 + b2
	x := (b0*c.Start.X + b1*c.Apex.X + b2*c.End.X) / den
	y := (b0*c.Start.Y + b1*c.Apex.Y + b2*c.End.Y) / den
	return x, y
}

// EvalDeriv returns the analytic first derivative dP/dt of the rational
// quadratic at parameter t in [0, 1]. It is exact (the quotient rule on the
// homogeneous numerator and denominator), so a tangent or area integrand built
// on it carries no nested finite difference.
func (c *Conic) EvalDeriv(t float64) (float64, float64) {
	w := c.weight()
	u := 1 - t
	b0, b1, b2 := u*u, 2*u*t*w, t*t
	den := b0 + b1 + b2
	// Numerator N(t) and denominator W(t) derivatives.
	db0, db1, db2 := -2*u, 2*w*(1-2*t), 2*t
	dden := db0 + db1 + db2
	nx := b0*c.Start.X + b1*c.Apex.X + b2*c.End.X
	ny := b0*c.Start.Y + b1*c.Apex.Y + b2*c.End.Y
	dnx := db0*c.Start.X + db1*c.Apex.X + db2*c.End.X
	dny := db0*c.Start.Y + db1*c.Apex.Y + db2*c.End.Y
	dx := (dnx*den - nx*dden) / (den * den)
	dy := (dny*den - ny*dden) / (den * den)
	return dx, dy
}

// floor returns v away from zero so divisions by a degenerate semi-axis stay
// finite (matching the solver's norm convention).
func floor(v float64) float64 {
	if math.Abs(v) < 1e-12 {
		if v < 0 {
			return -1e-12
		}
		return 1e-12
	}
	return v
}
