package main

import "github.com/lestrrat-3d/sketch"

// constraintShowcase draws a small figure carrying several geometric constraints
// so their glyphs are visible: two concentric circles (◎), a point on the outer
// circle (•), and two lines tangent to it (T) that are perpendicular to each
// other (⊥). The geometry is authored already satisfying the constraints, so the
// solve just confirms it.
func constraintShowcase() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())

	o1 := s.CreatePoint(45, 42)
	o1.MoveTo(45, 42)
	s.Fix(o1)
	c1 := s.CreateCircle(o1, 22)
	c2 := s.CreateCircle(s.CreatePoint(45, 42), 11)
	s.AddConstraint(sketch.NewConcentric(c1, c2))

	p := s.CreatePoint(45, 20) // bottom of the outer circle
	s.AddConstraint(sketch.NewPointOnCircle(p, c1))

	top := s.CreateLine(s.CreatePoint(12, 64), s.CreatePoint(78, 64))   // tangent at the top
	right := s.CreateLine(s.CreatePoint(67, 10), s.CreatePoint(67, 74)) // tangent at the right
	s.AddConstraint(
		sketch.NewTangent(top, c1),
		sketch.NewTangent(right, c1),
		sketch.NewPerpendicular(top, right),
	)

	if _, err := s.Solve(); err != nil {
		return "", err
	}
	return s.SVG(withAnn(
		sketch.WithConstraints(true),
		sketch.WithShowPoints(false),
	)...)
}
