package geom

import "math"

// Point is a 2D point definition. Its coordinates are the template/initial
// values; committing it to a sketch produces a separate solver point that may
// move, leaving this Point unchanged.
type Point struct {
	X, Y         float64
	Name         string
	Construction bool
}

// NewPoint returns a point at (x, y).
func NewPoint(x, y float64) *Point { return &Point{X: x, Y: y} }

// Line is a straight segment between two points.
type Line struct {
	Start, End   *Point
	Construction bool
}

// NewLine returns a line between start and end.
func NewLine(start, end *Point) *Line { return &Line{Start: start, End: end} }

// Length returns the distance between the line's endpoints.
func (l *Line) Length() float64 { return math.Hypot(l.End.X-l.Start.X, l.End.Y-l.Start.Y) }

// Circle is a full circle defined by a center point and a radius.
type Circle struct {
	Center       *Point
	Radius       float64
	Construction bool
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
	Center       *Point
	Rx, Ry       float64
	Rotation     float64
	Construction bool
}

// NewEllipse returns an ellipse with the given center, semi-axes and rotation.
func NewEllipse(center *Point, rx, ry, rotation float64) *Ellipse {
	return &Ellipse{Center: center, Rx: rx, Ry: ry, Rotation: rotation}
}

// Arc is a circular arc swept counter-clockwise from Start to End about Center.
type Arc struct {
	Center, Start, End *Point
	Construction       bool
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
