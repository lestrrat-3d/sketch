package main

// Guide images aid the docs/guide/* topic pages. They use the same framed,
// gridded gallery styling (annStyle) as the README heroes; showcase figures hide
// point markers so the geometry reads cleanly.

import "github.com/lestrrat-3d/sketch"

// geometryPrimitives lays out one of each common builder — line, circle, arc,
// ellipse, spline — so the reader can see the vocabulary at a glance.
func geometryPrimitives() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())

	// Top row: line, circle, arc.
	s.CreateLine(s.CreatePoint(0, 48), s.CreatePoint(42, 78))
	s.CreateCircle(s.CreatePoint(78, 62), 15)
	arcCenter := s.CreatePoint(120, 48)
	s.CreateArc(arcCenter, s.CreatePoint(146, 48), s.CreatePoint(120, 74))

	// Bottom row: ellipse, spline.
	s.CreateEllipse(s.CreatePoint(30, 15), 24, 11, 0.35)
	spline, err := s.CreateSpline(
		s.CreatePoint(78, 4), s.CreatePoint(94, 30),
		s.CreatePoint(116, 2), s.CreatePoint(136, 28), s.CreatePoint(152, 6),
	)
	if err != nil {
		return "", err
	}
	_ = spline
	return s.SVG(withAnn(sketch.WithShowPoints(false))...)
}

// compoundShapes shows the three compound builders — rectangle, polygon and slot
// — each a whole shape (primitives plus the constraints that hold it).
func compoundShapes() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())

	s.CreateRectangle(0, 8, 60, 52)
	if _, err := s.CreatePolygon(112, 30, 6, 26); err != nil {
		return "", err
	}
	if _, err := s.CreateSlot(168, 30, 232, 30, 16); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithShowPoints(false))...)
}
