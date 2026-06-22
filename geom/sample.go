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

// Polyline samples the elliptical arc from Start to End along its
// counter-clockwise eccentric-angle sweep at segments+1 points (minimum 2),
// applying the local-frame rotation.
func (e *EllipticalArc) Polyline(segments int) [][2]float64 {
	if segments < 2 {
		segments = 2
	}
	cosr, sinr := math.Cos(e.Rotation), math.Sin(e.Rotation)
	start := e.StartParam()
	sweep := e.Sweep()
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		ang := start + sweep*float64(i)/float64(segments)
		lx, ly := e.Rx*math.Cos(ang), e.Ry*math.Sin(ang)
		pts[i] = [2]float64{e.Center.X + lx*cosr - ly*sinr, e.Center.Y + lx*sinr + ly*cosr}
	}
	// The interior is on the ellipse; pin the ends to the exact boundary points
	// (which a caller may not have placed perfectly on the ellipse) so the
	// sampled curve joins its neighbours by shared-endpoint identity.
	pts[0] = [2]float64{e.Start.X, e.Start.Y}
	pts[segments] = [2]float64{e.End.X, e.End.Y}
	return pts
}

// Polyline samples the conic from Start to End at segments+1 points (minimum 2
// segments) in the curve parameter t. The endpoints are pinned to the exact
// Start/End points so the sampled curve joins its neighbours by shared-endpoint
// identity.
func (c *Conic) Polyline(segments int) [][2]float64 {
	if segments < 2 {
		segments = 2
	}
	pts := make([][2]float64, segments+1)
	for i := 0; i <= segments; i++ {
		x, y := c.Eval(float64(i) / float64(segments))
		pts[i] = [2]float64{x, y}
	}
	pts[0] = [2]float64{c.Start.X, c.Start.Y}
	pts[segments] = [2]float64{c.End.X, c.End.Y}
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
