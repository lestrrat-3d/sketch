package geom

import "math"

// Measurements query geometric quantities between transient primitives. They
// are the read side of the geometry math layer, joining Line.Length and the Arc
// angle accessors; sketch entities expose the same measurements on solved state
// by delegating through their Geometry snapshots.

// DistanceTo returns the Euclidean distance from p to other.
func (p *Point) DistanceTo(other *Point) float64 {
	return math.Hypot(other.X-p.X, other.Y-p.Y)
}

// DistanceToLine returns the perpendicular distance from p to the infinite line
// through l. A degenerate (zero-length) line falls back to the distance to its
// start point.
func (p *Point) DistanceToLine(l *Line) float64 {
	abx, aby := l.End.X-l.Start.X, l.End.Y-l.Start.Y
	n := math.Hypot(abx, aby)
	if n == 0 {
		return p.DistanceTo(l.Start)
	}
	apx, apy := p.X-l.Start.X, p.Y-l.Start.Y
	return math.Abs(abx*apy-aby*apx) / n
}

// AngleTo returns the signed directed angle from l to other, in radians, in the
// range (-π, π] — the same quantity an Angle constraint drives, so a line pair
// constrained to 45° measures math.Pi/4 here. Positive is counter-clockwise.
func (l *Line) AngleTo(other *Line) float64 {
	d1x, d1y := l.End.X-l.Start.X, l.End.Y-l.Start.Y
	d2x, d2y := other.End.X-other.Start.X, other.End.Y-other.Start.Y
	return math.Atan2(d1x*d2y-d1y*d2x, d1x*d2x+d1y*d2y)
}
