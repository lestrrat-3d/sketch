package main

import (
	"context"

	"github.com/lestrrat-3d/sketch"
)

// goalDrag illustrates a soft target (the drag primitive): point b is dragged
// toward a goal up and to the right, but a horizontal constraint pins it to the
// x-axis — so only the horizontal freedom is pulled and b settles directly below
// the goal. The goal and the blocked vertical pull are drawn as construction
// geometry (dashed grey). Coordinates are the example's (×20 for a legible size).
func goalDrag() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())

	a := s.CreatePoint(0, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	b := s.CreatePoint(140, 0) // solved: x pulled to the goal's x, y held at 0
	ab := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewHorizontal(ab))

	// The goal target (7,5) and the vertical pull the constraint blocks.
	target := s.CreatePoint(140, 100)
	pull := s.CreateLine(b, target)
	pull.SetConstruction(true)
	marker := s.CreateCircle(target, 7)
	marker.SetConstruction(true)

	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithConstraints(true))...)
}
