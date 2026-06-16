package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_verify shows the headless verification oracle: one Verify call
// aggregates every trust signal an agent needs — solvability, degrees of
// freedom, the constraint conflict set, free points and closed profiles —
// across a sketch as it goes from under-constrained to fully constrained to
// contradictory.
func Example_sketch_verify() {
	report := func(label string, rep *sketch.VerificationReport) {
		fmt.Printf("%s: status=%s solvable=%t DOF=%d free-points=%d profiles=%d conflicts=%d\n",
			label, rep.Status, rep.Solvable, rep.DOF, len(rep.FreePoints), len(rep.Profiles), len(rep.Conflicts))
	}

	s := sketch.New()
	r := s.AddRectangle(0, 0, 20, 12)
	s.Fix(r.A)

	// A rectangle held only by its shape constraints: closed, but its size is
	// free to move.
	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}
	report("shape only ", s.Verify())

	// Dimension the width and height: now nothing can move.
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))
	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}
	report("dimensioned", s.Verify())

	// Add a second width dimension that disagrees with the first. The solver can
	// no longer converge, and Verify names the conflict and the earlier
	// dimension it fights.
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 25))
	s.Solve() // expected to fail: the dimensions contradict
	rep := s.Verify()
	report("contradicted", rep)
	fmt.Printf("the conflicting dimension fights %d earlier constraint(s)\n", len(rep.Conflicts[0].With))

	// Output:
	// shape only : status=underconstrained solvable=true DOF=2 free-points=3 profiles=1 conflicts=0
	// dimensioned: status=fully constrained solvable=true DOF=0 free-points=0 profiles=1 conflicts=0
	// contradicted: status=overconstrained solvable=false DOF=0 free-points=0 profiles=1 conflicts=1
	// the conflicting dimension fights 1 earlier constraint(s)
}
