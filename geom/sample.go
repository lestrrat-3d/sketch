package geom

import "math"

// Polyline returns the line's two endpoints. It exists so every curve type
// offers a uniform Polyline sampler.
func (l *Line) Polyline() [][2]float64 {
	return [][2]float64{{l.Start.X, l.Start.Y}, {l.End.X, l.End.Y}}
}

// Polyline samples the full circle counter-clockwise at segments+1 points
// (minimum 2 segments), the first and last coinciding.
func (c *Circle) Polyline(segments int) [][2]float64 {
	if segments < 2 {
		segments = 2
	}
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		ang := 2 * math.Pi * float64(i) / float64(segments)
		pts[i] = [2]float64{c.Center.X + c.Radius*math.Cos(ang), c.Center.Y + c.Radius*math.Sin(ang)}
	}
	return pts
}

// Polyline samples the arc from its start to its end along the counter-clockwise
// sweep at segments+1 points (minimum 2 segments).
func (a *Arc) Polyline(segments int) [][2]float64 {
	if segments < 2 {
		segments = 2
	}
	r := a.Radius()
	start := a.StartAngle()
	sweep := a.Sweep()
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		ang := start + sweep*float64(i)/float64(segments)
		pts[i] = [2]float64{a.Center.X + r*math.Cos(ang), a.Center.Y + r*math.Sin(ang)}
	}
	return pts
}

// Polyline samples the full ellipse counter-clockwise at segments+1 points
// (minimum 2 segments), applying its local-frame rotation.
func (e *Ellipse) Polyline(segments int) [][2]float64 {
	if segments < 2 {
		segments = 2
	}
	cosr, sinr := math.Cos(e.Rotation), math.Sin(e.Rotation)
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		ang := 2 * math.Pi * float64(i) / float64(segments)
		lx, ly := e.Rx*math.Cos(ang), e.Ry*math.Sin(ang)
		pts[i] = [2]float64{e.Center.X + lx*cosr - ly*sinr, e.Center.Y + lx*sinr + ly*cosr}
	}
	return pts
}
