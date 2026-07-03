package main

import (
	"context"

	"github.com/lestrrat-3d/sketch"
)

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

	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(
		sketch.WithConstraints(true),
		sketch.WithShowPoints(false),
	)...)
}

// dimensionShowcase renders the variety of dimensional constraints as CAD
// dimensions: a linear distance and an angle on a pair of lines, a radius on one
// circle and a diameter on another. Targets match the authored geometry, so the
// labels read true without solving.
func dimensionShowcase() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())

	// Distance + angle on two lines sharing a vertex.
	v := s.CreatePoint(24, 22)
	base := s.CreateLine(v, s.CreatePoint(114, 22)) // 90 long, horizontal
	arm := s.CreateLine(v, s.CreatePoint(93.9, 60)) // ~30° from the base
	s.AddConstraint(sketch.NewDistance(v, base.End, 90))
	s.AddConstraint(sketch.NewAngle(base, arm, 30))

	// Radius and diameter on two circles.
	s.AddConstraint(sketch.NewRadius(s.CreateCircle(s.CreatePoint(40, 92), 20), 20))
	s.AddConstraint(sketch.NewDiameter(s.CreateCircle(s.CreatePoint(128, 92), 18), 36))

	return s.SVG(withAnn(
		sketch.WithDimensions(true),
		sketch.WithShowPoints(false),
	)...)
}

// drivenDimension contrasts driving and driven (reference) dimensions: a right
// triangle whose two legs are driving distances, and whose hypotenuse is a driven
// dimension — it constrains nothing and instead reports the measured value,
// rendered parenthesized after the solve refreshes it.
func drivenDimension() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())

	a := s.CreatePoint(0, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	b := s.CreatePoint(80, 0)
	c := s.CreatePoint(0, 60)
	ab := s.CreateLine(a, b)
	s.CreateLine(b, c)
	ca := s.CreateLine(c, a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewPerpendicular(ab, ca))
	s.AddConstraint(
		sketch.NewDistance(a, b, 80), // driving leg
		sketch.NewDistance(c, a, 60), // driving leg
	)
	hyp := sketch.NewDistance(b, c, 0)
	hyp.SetDriven(true) // reference: measures 100, rendered parenthesized
	s.AddConstraint(hyp)

	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(
		sketch.WithDimensions(true),
		sketch.WithShowPoints(false),
	)...)
}
