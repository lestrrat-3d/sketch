package examples_test

import (
	"context"
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_arcTangent continues a line with a tangent arc, as Fusion's
// tangent arc does. The arc leaves the line's end in the line's direction and
// stays tangent to it when re-dimensioned: driving the radius from 5 to 8 moves
// the center along the line's normal.
func Example_sketch_arcTangent() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	p0, p1 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	l := s.CreateLine(p0, p1)
	s.AddConstraint(
		sketch.NewCoincident(p0, s.Origin()),
		sketch.NewHorizontal(l),
		sketch.NewDistance(p0, p1, 10),
	)

	arc, err := s.CreateArcTangent(l, p1, 15, 5) // a left turn up to (15, 5)
	if err != nil {
		fmt.Printf("failed to create arc: %s\n", err)
		return
	}
	fmt.Printf("center (%.1f, %.1f), radius %.1f, DOF %d\n", arc.Center.X(), arc.Center.Y(), arc.R(), s.DOF())

	s.AddConstraint(sketch.NewRadius(arc, 8), sketch.NewHorizontalPoints(arc.Center, arc.End))
	res, err := s.Solve(context.Background())
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("radius 8: center (%.1f, %.1f), end (%.1f, %.1f), DOF %d\n",
		arc.Center.X(), arc.Center.Y(), arc.End.X(), arc.End.Y(), res.DOF)

	// Output:
	// center (10.0, 5.0), radius 5.0, DOF 2
	// radius 8: center (10.0, 8.0), end (18.0, 8.0), DOF 0
}
