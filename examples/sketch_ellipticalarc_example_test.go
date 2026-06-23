package examples_test

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_ellipticalArc authors an elliptical arc — an ellipse restricted
// to a start→end sweep — closes it with a chord, and reads back the half-ellipse
// region's area from the profile engine.
func Example_sketch_ellipticalArc() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	end := s.CreatePoint(-4, 0)
	// Top half of an ellipse with semi-axes 4 and 2.
	ea := s.CreateEllipticalArc(c, start, end, 4, 2, 0)
	s.CreateLine(ea.End, ea.Start) // chord along the major axis

	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("semi-axes %.0f x %.0f, sweep %.0f deg\n",
		ea.Rx(), ea.Ry(), ea.Sweep()*180/math.Pi)

	profiles := s.Profiles()
	fmt.Printf("regions: %d, area %.2f\n", len(profiles), profiles[0].Area)

	// Output:
	// semi-axes 4 x 2, sweep 180 deg
	// regions: 1, area 12.57
}
