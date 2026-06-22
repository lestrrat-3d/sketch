package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_conic authors a conic arc — a rational quadratic Bézier through
// Start and End with an apex control point and a fullness rho — closes it with a
// chord, and reads back the conic-bounded region's exact area. With rho = 0.5
// the conic is a parabola, whose bulge over the chord is exactly one third of the
// control-triangle's signed area.
func Example_sketch_conic() {
	s := sketch.New()
	start := s.AddPoint(0, 0)
	apex := s.AddPoint(3, 4)
	end := s.AddPoint(6, 0)

	c, err := s.AddConic(start, apex, end, 0.5) // 0.5 → parabola
	if err != nil {
		fmt.Println(err)
		return
	}
	s.AddLine(c.End, c.Start) // chord closes the loop

	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("rho %.1f, endpoints (%.0f,%.0f)-(%.0f,%.0f)\n",
		c.Rho(), c.Start.X(), c.Start.Y(), c.End.X(), c.End.Y())

	profiles := s.Profiles()
	fmt.Printf("regions: %d, area %.2f\n", len(profiles), profiles[0].Area)

	// Output:
	// rho 0.5, endpoints (0,0)-(6,0)
	// regions: 1, area 8.00
}
