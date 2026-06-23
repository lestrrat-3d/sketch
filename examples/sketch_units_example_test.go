package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/units"
)

// Example_sketch_units shows that dimensions carry typed units while the solver
// works in base millimetres: a distance set in inches solves to its millimetre
// equivalent, and conversion happens only through the units library.
func Example_sketch_units() {
	// A units.Value knows its own unit and converts through the library.
	w := units.Inches(4)
	mm, err := w.In(units.Millimeter)
	if err != nil {
		fmt.Printf("failed to convert: %s\n", err)
		return
	}
	fmt.Printf("%s = %.1f mm\n", w, mm)

	// A dimension carries a unit; internally the solver stays in millimetres.
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(50, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(s.CreateLine(a, b)))

	d := sketch.NewDistance(a, b, 0)
	s.AddConstraint(d)
	if err := d.SetValue(units.Inches(4)); err != nil { // 4 in -> 101.6 mm
		fmt.Printf("failed to set value: %s\n", err)
		return
	}
	if _, err := s.Solve(); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("|ab| = %.1f mm\n", b.X())

	// Output:
	// 4 in = 101.6 mm
	// |ab| = 101.6 mm
}
