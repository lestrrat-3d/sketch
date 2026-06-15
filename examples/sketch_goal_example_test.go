package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_goal demonstrates a soft target (the primitive behind drag
// interactions): hard constraints always win, and the goal only pulls whatever
// freedom is left over.
func Example_sketch_goal() {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(2, 2)
	a.MoveTo(0, 0)
	s.Fix(a)
	l := s.AddLine(a, b)
	s.AddConstraint(sketch.NewHorizontal(l)) // b must stay on the x-axis (y = 0)

	// Drag b toward (7, 5). The horizontal constraint pins y to 0; the goal is
	// free to pull the remaining x degree of freedom to 7.
	res, err := s.Solve(sketch.WithGoal(b, 7, 5))
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("b=(%.0f,%.0f) DOF=%d\n", b.X(), b.Y(), res.DOF)

	// Output:
	// b=(7,0) DOF=1
}
