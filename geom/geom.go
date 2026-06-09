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
	A, B         *Point
	Construction bool
}

// NewLine returns a line between two points.
func NewLine(a, b *Point) *Line { return &Line{A: a, B: b} }

// Length returns the distance between the line's endpoints.
func (l *Line) Length() float64 { return math.Hypot(l.B.X-l.A.X, l.B.Y-l.A.Y) }

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
