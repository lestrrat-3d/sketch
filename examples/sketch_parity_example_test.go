package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_parity shows the constraint/dimension parity batch: horizontal
// and vertical relations between bare points (no connecting line), a midpoint
// over a bare point pair, and radius dimensions / concentric relations reaching
// an arc through the same Circular interface a circle already uses.
func Example_sketch_parity() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	// Two post tops forced level without a line joining them, plus a keystone
	// pinned exactly between them.
	left := s.CreatePoint(0, 4)
	right := s.CreatePoint(10, -1) // starts uneven
	s.Fix(left)
	s.AddConstraint(sketch.NewHorizontalPoints(left, right))
	s.AddConstraint(sketch.NewHorizontalDistance(left, right, 10)) // span the gap
	key := s.CreatePoint(3, 9)
	s.AddConstraint(sketch.NewMidpointOf(key, left, right))

	// An arch arc whose radius is driven directly: NewRadius reaches the arc
	// through the Circular interface. The center is fixed; horizontal/vertical
	// relations lay the endpoints on the axes, and the radius dimension sizes it.
	c := s.CreatePoint(0, 0)
	s.Fix(c)
	start := s.CreatePoint(3, 0)
	end := s.CreatePoint(0, 3)
	arc := s.CreateArc(c, start, end)
	s.AddConstraint(sketch.NewHorizontalPoints(c, start)) // start on the x axis
	s.AddConstraint(sketch.NewVerticalPoints(c, end))     // end on the y axis
	s.AddConstraint(sketch.NewRadius(arc, 5))

	// A bolt hole concentric with the arch, sized by its own radius dimension.
	hole := s.CreateCircle(s.CreatePoint(2, 1), 1)
	s.AddConstraint(sketch.NewConcentric(hole, arc))
	s.AddConstraint(sketch.NewRadius(hole, 2))

	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("right top level at (%.0f, %.0f)\n", right.X(), right.Y())
	fmt.Printf("keystone at (%.0f, %.0f)\n", key.X(), key.Y())
	fmt.Printf("arc radius %.0f, endpoints (%.0f, %.0f) and (%.0f, %.0f)\n",
		arc.R(), start.X(), start.Y(), end.X(), end.Y())
	fmt.Printf("hole radius %.0f, concentric at (%.0f, %.0f)\n",
		hole.R(), hole.Center.X(), hole.Center.Y())

	// Output:
	// right top level at (10, 4)
	// keystone at (5, 4)
	// arc radius 5, endpoints (5, 0) and (0, 5)
	// hole radius 2, concentric at (0, 0)
}
