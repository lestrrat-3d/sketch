package main

import "github.com/lestrrat-3d/sketch"

// profilesSubdivision shows bare-crossing subdivision: two circles that cross
// without sharing a point are split into three separate faces (the two outer
// lunes and the central lens), each shaded by WithProfileFill.
func profilesSubdivision() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	s.CreateCircle(s.CreatePoint(42, 40), 32)
	s.CreateCircle(s.CreatePoint(78, 40), 32)
	return s.SVG(withAnn(
		sketch.WithProfileFill(true),
		sketch.WithShowPoints(false),
	)...)
}
