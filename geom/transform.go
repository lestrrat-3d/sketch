package geom

import "math"

// Coordinate transforms produce fresh template points from existing ones. They
// are the math layer under the mirror and pattern sketch tools: transform the
// points of an entity to position a copy, commit the copy, then link the
// original and copy with constraints. Like the rest of geom they never touch a
// sketch and carry the source point's Construction flag onto the result.

// MirrorPoint returns the reflection of p across the infinite line through axis.
// A degenerate axis (zero length) yields a copy of p.
func MirrorPoint(p *Point, axis *Line) *Point {
	ax, ay := axis.Start.X, axis.Start.Y
	dx, dy := axis.End.X-ax, axis.End.Y-ay
	dd := dx*dx + dy*dy
	if dd == 0 {
		return clone(p)
	}
	// Foot of the perpendicular from p onto the axis, then reflect through it.
	t := ((p.X-ax)*dx + (p.Y-ay)*dy) / dd
	fx, fy := ax+t*dx, ay+t*dy
	out := NewPoint(2*fx-p.X, 2*fy-p.Y)
	out.Construction = p.Construction
	return out
}

// TranslatePoint returns p shifted by (dx, dy).
func TranslatePoint(p *Point, dx, dy float64) *Point {
	out := NewPoint(p.X+dx, p.Y+dy)
	out.Construction = p.Construction
	return out
}

// RotatePoint returns p rotated counter-clockwise by ang radians about center.
func RotatePoint(p *Point, center *Point, ang float64) *Point {
	cos, sin := math.Cos(ang), math.Sin(ang)
	dx, dy := p.X-center.X, p.Y-center.Y
	out := NewPoint(center.X+dx*cos-dy*sin, center.Y+dx*sin+dy*cos)
	out.Construction = p.Construction
	return out
}

// clone returns a fresh copy of p (coordinates, name and Construction flag).
func clone(p *Point) *Point {
	out := NewPoint(p.X, p.Y)
	out.Name = p.Name
	out.Construction = p.Construction
	return out
}
