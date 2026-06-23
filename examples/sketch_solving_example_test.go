package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_solving solves a fully constrained sketch with tuned solver
// options and reports the fields the solver returns. DOF can also be queried
// directly, without moving any geometry.
func Example_sketch_solving() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())
	a := s.AddPoint(0, 0)
	b := s.AddPoint(30, 4)
	a.MoveTo(0, 0)
	s.Fix(a)
	l := s.AddLine(a, b)
	s.AddConstraint(sketch.NewHorizontal(l))
	s.AddConstraint(sketch.NewDistance(a, b, 30))

	res, err := s.Solve(
		sketch.WithMaxIterations(200),
		sketch.WithTolerance(1e-10),
	)
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("converged=%t DOF=%d redundant=%d\n", res.Converged, res.DOF, res.Redundant)
	fmt.Printf("s.DOF()=%d\n", s.DOF())

	// Output:
	// converged=true DOF=0 redundant=0
	// s.DOF()=0
}
