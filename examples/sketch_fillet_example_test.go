package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
)

// Example_sketch_fillet rounds the corner of an L-shaped pair of lines, then
// edits the fillet radius and re-solves. The rounding arc stays tangent to both
// legs, so changing the radius slides the contacts and recentres the arc — a
// parametric fillet, not a one-shot edit.
func Example_sketch_fillet() {
	s := sketch.New()

	// Vertical leg A(0,10)->corner; horizontal leg corner->B(10,0).
	gA, gCorner, gB := geom.NewPoint(0, 10), geom.NewPoint(0, 0), geom.NewPoint(10, 0)
	a := s.AddPoint(gA)
	b := s.AddPoint(gB)
	l1 := s.AddLine(geom.NewLine(gA, gCorner))
	l2 := s.AddLine(geom.NewLine(gCorner, gB))

	f, err := s.AddFillet(l1, l2, 3)
	if err != nil {
		fmt.Printf("failed to fillet: %s\n", err)
		return
	}

	// Ground the far ends and hold the leg directions; the corner is now an arc.
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewVertical(f.L1), sketch.NewHorizontal(f.L2))

	res, err := s.Solve()
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("R=%.1f center=(%.0f,%.0f) DOF=%d\n", f.Arc.R(), f.Arc.Center.X(), f.Arc.Center.Y(), res.DOF)

	// Shrink the radius; the arc re-centres while staying tangent to both legs.
	f.Radius.Set(2)
	res, err = s.Solve()
	if err != nil {
		fmt.Printf("failed to re-solve: %s\n", err)
		return
	}
	fmt.Printf("R=%.1f center=(%.0f,%.0f) DOF=%d\n", f.Arc.R(), f.Arc.Center.X(), f.Arc.Center.Y(), res.DOF)

	// Output:
	// R=3.0 center=(3,3) DOF=0
	// R=2.0 center=(2,2) DOF=0
}
