package examples_test

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_probeConfigurations shows how a fully constrained sketch can
// still be configuration-ambiguous, and how the ambiguity probe surfaces the
// alternative branch. Two unsigned distances pin a triangle apex to DOF 0,
// yet the mirror image below the base satisfies them just as well — exactly
// the kind of sketch that silently flips when seeded differently.
func Example_sketch_probeConfigurations() {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	apex := s.AddPoint(5, 3) // seeded above the base
	d1 := sketch.NewDistance(a, apex, 8)
	d2 := sketch.NewDistance(b, apex, 8)
	s.AddConstraint(d1, d2)

	res, err := s.Solve()
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("DOF %d\n", res.DOF)

	// DOF 0 looks safe, but the probe proves the branch is seed-dependent.
	pr, err := s.ProbeConfigurations()
	if err != nil {
		fmt.Printf("failed to probe: %s\n", err)
		return
	}
	fmt.Printf("ambiguous: %v\n", pr.Ambiguous())
	r := func(v float64) float64 { return math.Round(v*1000)/1000 + 0 }
	for i, c := range pr.Configurations {
		x, y := c.PointXY(apex)
		fmt.Printf("configuration %d: apex = (%.3f, %.3f)\n", i, r(x), r(y))
	}

	// Pin the branch by replacing the unsigned distances with signed Δx/Δy
	// dimensions: the same apex, but the mirror image no longer satisfies
	// the constraints.
	s.RemoveConstraint(d1)
	s.RemoveConstraint(d2)
	s.AddConstraint(sketch.NewHorizontalDistance(a, apex, 5))
	s.AddConstraint(sketch.NewVerticalDistance(a, apex, math.Sqrt(8*8-5*5)))
	if _, err := s.Solve(); err != nil {
		fmt.Printf("failed to re-solve: %s\n", err)
		return
	}
	pr, err = s.ProbeConfigurations()
	if err != nil {
		fmt.Printf("failed to probe: %s\n", err)
		return
	}
	fmt.Printf("after pinning, ambiguous: %v\n", pr.Ambiguous())

	// Output:
	// DOF 0
	// ambiguous: true
	// configuration 0: apex = (5.000, 6.245)
	// configuration 1: apex = (5.000, -6.245)
	// after pinning, ambiguous: false
}
