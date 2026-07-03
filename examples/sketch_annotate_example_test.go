package examples_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_annotate renders a sketch to SVG with the annotation options
// turned on: dimensions, geometric-constraint glyphs, degree-of-freedom coloring
// and a verification status badge. All annotation options default off, so plain
// SVG() output is unchanged; opting in overlays the constraint and verification
// information a first-time reader needs to understand the sketch.
func Example_sketch_annotate() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20, 0)
	c := s.CreatePoint(20, 12)
	d := s.CreatePoint(0, 12)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	cd := s.CreateLine(c, d)
	da := s.CreateLine(d, a)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(
		sketch.NewHorizontal(ab),
		sketch.NewHorizontal(cd),
		sketch.NewVertical(bc),
		sketch.NewVertical(da),
	)
	s.AddConstraint(sketch.NewDistance(a, b, 20), sketch.NewDistance(a, d, 12))
	if _, err := s.Solve(context.Background()); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	// Plain render: geometry and points only.
	plain, err := s.SVG()
	if err != nil {
		fmt.Printf("failed to render: %s\n", err)
		return
	}

	// Annotated render: dimensions, constraint glyphs and a status badge.
	annotated, err := s.SVG(
		sketch.WithDimensions(true),
		sketch.WithConstraints(true),
		sketch.WithStatusBadge(true),
	)
	if err != nil {
		fmt.Printf("failed to render: %s\n", err)
		return
	}

	fmt.Printf("plain has dimension label: %t\n", strings.Contains(plain, "20 mm"))
	fmt.Printf("annotated has dimension label: %t\n", strings.Contains(annotated, "20 mm"))
	fmt.Printf("annotated has H glyph: %t\n", strings.Contains(annotated, ">H<"))
	fmt.Printf("annotated has status badge: %t\n", strings.Contains(annotated, "fully constrained"))

	// Output:
	// plain has dimension label: false
	// annotated has dimension label: true
	// annotated has H glyph: true
	// annotated has status badge: true
}
