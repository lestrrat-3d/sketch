package examples_test

import (
	"context"
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_origin shows the sketch origin as the anchor: a point the engine
// provides, that the solver never moves, and that geometry is tied to with an
// ordinary constraint rather than with Sketch.Fix.
//
// Two things follow from it being a constraint. The tie is visible to the
// diagnostics and removable like any other relation, and a point the drawing does
// not otherwise constrain — the leftover a shared builder creates — is made
// determinate by one coincidence rather than by pinning a coordinate.
func Example_sketch_origin() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	// A rectangle anchored at the origin: its corner is CONSTRAINED there, and one
	// horizontal edge takes the rotational freedom.
	corner := s.CreatePoint(2, 3)
	across := s.CreatePoint(22, 3)
	up := s.CreatePoint(2, 15)
	bottom := s.CreateLine(corner, across)
	side := s.CreateLine(corner, up)

	s.AddConstraint(
		sketch.NewCoincident(corner, s.Origin()),
		sketch.NewHorizontal(bottom),
		sketch.NewPerpendicular(bottom, side),
		sketch.NewDistance(corner, across, 20),
		sketch.NewDistance(corner, up, 12),
	)

	// A point nothing else constrains — grounded on the origin too, so the sketch
	// is fully constrained without pinning a coordinate.
	spare := s.CreatePoint(7, 7)
	s.AddConstraint(sketch.NewCoincident(spare, s.Origin()))

	if _, err := s.Solve(context.Background()); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	rep := s.Verify(context.Background())
	fmt.Printf("status=%s DOF=%d trustworthy=%t\n", rep.Status, rep.DOF, rep.Trustworthy())
	// Distances rather than raw coordinates: a solved coordinate at zero can land
	// on either side of it, and a magnitude reads the same either way.
	fmt.Printf("corner to origin: %.0f, rectangle %.0f x %.0f\n",
		corner.DistanceTo(s.Origin()), corner.DistanceTo(across), corner.DistanceTo(up))

	// Nothing was pinned: the grounding is constraints all the way down.
	pinned := 0
	for _, p := range s.Points() {
		if p.IsFixed() {
			pinned++
		}
	}
	fmt.Printf("pinned points: %d, authored points: %d\n", pinned, len(s.Points()))

	// The origin itself is engine-provided: never in Points, always grounded.
	fmt.Printf("origin at (%.0f, %.0f), fixed=%t\n",
		s.Origin().X(), s.Origin().Y(), s.Origin().IsFixed())

	// Output:
	// status=fully constrained DOF=0 trustworthy=true
	// corner to origin: 0, rectangle 20 x 12
	// pinned points: 0, authored points: 4
	// origin at (0, 0), fixed=true
}
