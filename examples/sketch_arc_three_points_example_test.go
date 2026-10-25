package examples_test

import (
	"context"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_arcThreePoints draws an arc through three positions, as
// Fusion's three-point arc does. The middle position only places the arc: the
// builder creates the center at the circumcenter, and the arc keeps its two
// endpoints. Because arcs sweep counter-clockwise, a middle position on the
// clockwise side makes the given end the arc's Start.
func Example_sketch_arcThreePoints() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	start, end := s.CreatePoint(10, 0), s.CreatePoint(-10, 0)
	arc, err := s.CreateArcThreePoints(start, 0, 10, end)
	if err != nil {
		fmt.Printf("failed to create arc: %s\n", err)
		return
	}
	fmt.Printf("center (%.1f, %.1f), radius %.1f, sweep %.0f deg, start kept: %v\n",
		arc.Center.X(), arc.Center.Y(), arc.R(), arc.Sweep()*180/math.Pi, arc.Start == start)

	radius := sketch.NewRadius(arc, 12)
	s.AddConstraint(
		sketch.NewCoincident(arc.Center, s.Origin()),
		radius,
		sketch.NewHorizontalPoints(arc.Center, start),
		sketch.NewHorizontalPoints(arc.Center, end),
	)
	res, err := s.Solve(context.Background())
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("radius 12: start (%.1f, %.1f), end (%.1f, %.1f), DOF %d\n",
		start.X(), start.Y(), end.X(), end.Y(), res.DOF)

	// Output:
	// center (0.0, 0.0), radius 10.0, sweep 180 deg, start kept: true
	// radius 12: start (12.0, 0.0), end (-12.0, 0.0), DOF 0
}
