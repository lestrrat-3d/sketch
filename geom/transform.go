package geom

import "math"

// Coordinate transforms produce fresh points from existing ones. They are the
// math layer under the mirror and pattern sketch tools: transform the points of
// an entity's geometry snapshot to position a copy, then the sketch tools commit
// the copy and link it to the original with constraints. Like the rest of geom
// they are pure coordinate math and never touch a sketch.

// MirrorPoint returns the reflection of p across the infinite line through axis.
// A degenerate axis (zero length) yields a copy of p.
func MirrorPoint(p *Point, axis *Line) *Point {
	ax, ay := axis.Start.X, axis.Start.Y
	dx, dy := axis.End.X-ax, axis.End.Y-ay
	dd := dx*dx + dy*dy
	if dd == 0 {
		return NewPoint(p.X, p.Y)
	}
	// Foot of the perpendicular from p onto the axis, then reflect through it.
	t := ((p.X-ax)*dx + (p.Y-ay)*dy) / dd
	fx, fy := ax+t*dx, ay+t*dy
	return NewPoint(2*fx-p.X, 2*fy-p.Y)
}

// TranslatePoint returns p shifted by (dx, dy).
func TranslatePoint(p *Point, dx, dy float64) *Point {
	return NewPoint(p.X+dx, p.Y+dy)
}

// RotatePoint returns p rotated counter-clockwise by ang radians about center.
func RotatePoint(p *Point, center *Point, ang float64) *Point {
	cos, sin := math.Cos(ang), math.Sin(ang)
	dx, dy := p.X-center.X, p.Y-center.Y
	return NewPoint(center.X+dx*cos-dy*sin, center.Y+dx*sin+dy*cos)
}
