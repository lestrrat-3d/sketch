package examples_test

import (
	"context"
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_circleThreeTangents inscribes a circle in a triangle, as
// Fusion's three-tangent circle does. The hint point picks the circle: inside
// the triangle gives the incircle, across a side gives that side's excircle.
// The circle adds no degree of freedom, so it follows the triangle when a side
// is re-dimensioned.
func Example_sketch_circleThreeTangents() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	a, b, c := s.CreatePoint(0, 0), s.CreatePoint(12, 0), s.CreatePoint(0, 9)
	ab, bc, ca := s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, a)
	base := sketch.NewDistance(a, b, 12)
	s.AddConstraint(
		sketch.NewCoincident(a, s.Origin()),
		sketch.NewHorizontal(ab),
		sketch.NewVertical(ca),
		base,
		sketch.NewDistance(a, c, 9),
	)

	circle, err := s.CreateCircleThreeTangents(ab, bc, ca, 2, 2)
	if err != nil {
		fmt.Printf("failed to create circle: %s\n", err)
		return
	}
	report := func(label string) {
		res, err := s.Solve(context.Background())
		if err != nil {
			fmt.Printf("failed to solve: %s\n", err)
			return
		}
		fmt.Printf("%s: center (%.4f, %.4f), radius %.4f, DOF %d\n",
			label, circle.Center.X(), circle.Center.Y(), circle.R(), res.DOF)
	}
	report("base 12")
	base.Set(16)
	report("base 16")

	// Output:
	// base 12: center (3.0000, 3.0000), radius 3.0000, DOF 0
	// base 16: center (3.3212, 3.3212), radius 3.3212, DOF 0
}
