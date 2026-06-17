package examples_test

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_arcLength drives an arc by its swept length. The dimension uses
// a continuous-sweep formulation, so it works smoothly even past a half turn.
func Example_sketch_arcLength() {
	s := sketch.New()
	c := s.AddPoint(0, 0)
	start := s.AddPoint(4, 0)
	s.Fix(c)
	s.Fix(start) // radius 4, start on the +x axis
	end := s.AddPoint(0, 4)
	arc := s.AddArc(c, start, end)

	// Drive the swept length to 3π — at radius 4 that is a 135° sweep.
	s.AddConstraint(sketch.NewArcLength(arc, 3*math.Pi))
	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("radius %.0f, sweep %.0f deg, length %.4f\n",
		arc.R(), arc.Sweep()*180/math.Pi, arc.R()*arc.Sweep())

	// Output:
	// radius 4, sweep 135 deg, length 9.4248
}
