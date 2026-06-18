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

// Sweep returns the counter-clockwise sweep angle in (0, 2π].
func (a *Arc) Sweep() float64 {
	d := math.Mod(a.EndAngle()-a.StartAngle(), 2*math.Pi)
	if d <= 0 {
		d += 2 * math.Pi
	}
	return d
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
	d := math.Mod(e.EndParam()-e.StartParam(), 2*math.Pi)
	if d <= 0 {
		d += 2 * math.Pi
	}
	return d
}

// Endpoints returns the elliptical arc's start and end points, so it satisfies
// the open-curve Curve interface.
func (e *EllipticalArc) Endpoints() (*Point, *Point) { return e.Start, e.End }

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
