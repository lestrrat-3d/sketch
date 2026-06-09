// Command parametric demonstrates driving a sketch from a parameter table:
// a rectangular plate with a centered hole whose dimensions are all defined by
// expressions. Changing a single parameter and re-solving updates everything.
package main

import (
	"fmt"
	"os"

	"github.com/lestrrat-3d/sketch"
)

func main() {
	s := sketch.New()

	// Geometry: four corners + a center point for the hole.
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 1)
	c := s.AddPoint(9, 6)
	d := s.AddPoint(1, 5)
	o := s.AddPoint(5, 3)

	ab, bc, dc, ad := s.AddLine(a, b), s.AddLine(b, c), s.AddLine(d, c), s.AddLine(a, d)
	hole := s.AddCircle(o, 1)

	// Geometric constraints: grounded origin, axis-aligned rectangle.
	s.Lock(a, 0, 0)
	s.Horizontal(ab)
	s.Horizontal(dc)
	s.Vertical(ad)
	s.Vertical(bc)

	// Parameters: a single driving width, everything else derived from it.
	p := s.Params()
	p.Set("width", "120")
	p.Set("height", "width * 0.6")
	p.Set("hole_d", "min(width, height) / 3")

	// Bind dimensions to expressions.
	bind := func(d sketch.Dimension, expr string) {
		if err := s.Bind(d, expr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	bind(s.Distance(a, b, 0), "width")
	bind(s.Distance(a, d, 0), "height")
	bind(s.HorizontalDistance(a, o, 0), "width / 2") // hole centered
	bind(s.VerticalDistance(a, o, 0), "height / 2")
	bind(s.Radius(hole, 0), "hole_d / 2")

	report := func(label string) {
		res, err := s.Solve()
		if err != nil {
			fmt.Fprintln(os.Stderr, label, "solve:", err)
			os.Exit(1)
		}
		fmt.Printf("%s: plate %.0f x %.0f, hole d=%.1f at (%.0f, %.0f), DOF %d\n",
			label, b.X(), d.Y(), 2*hole.R(), o.X(), o.Y(), res.DOF)
		svg, _ := s.SVG(sketch.DefaultSVGOptions())
		name := "plate_" + label + ".svg"
		os.WriteFile(name, []byte(svg), 0o644)
		fmt.Println("  wrote", name)
	}

	report("a") // width = 120

	p.Set("width", "200") // change ONE parameter ...
	report("b")           // ... height and hole follow automatically
}
