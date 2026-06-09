// Command parametric demonstrates driving a sketch from a parameter table:
// a rectangular plate with a centered hole whose dimensions are all defined by
// expressions. Changing a single parameter and re-solving updates everything.
package main

import (
	"fmt"
	"os"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
)

func main() {
	s := sketch.New()

	// Generic geometry: four corners + a center point for the hole.
	gA := geom.NewPoint(0, 0)
	gB := geom.NewPoint(10, 1)
	gC := geom.NewPoint(9, 6)
	gD := geom.NewPoint(1, 5)
	gO := geom.NewPoint(5, 3)

	// Commit it into the sketch, getting solver-bound handles.
	a := s.AddPoint(gA)
	b := s.AddPoint(gB)
	d := s.AddPoint(gD)
	o := s.AddPoint(gO)

	ab := s.AddLine(geom.NewLine(gA, gB))
	bc := s.AddLine(geom.NewLine(gB, gC))
	dc := s.AddLine(geom.NewLine(gD, gC))
	ad := s.AddLine(geom.NewLine(gA, gD))
	hole := s.AddCircle(geom.NewCircle(gO, 1))

	// Geometric constraints: grounded origin, axis-aligned rectangle.
	s.Lock(a, 0, 0)
	s.AddConstraint(
		sketch.NewHorizontal(ab),
		sketch.NewHorizontal(dc),
		sketch.NewVertical(ad),
		sketch.NewVertical(bc),
	)

	// Parameters: a single driving width as a typed length; everything else is
	// derived from it. Geometry solves in base millimetres regardless of the
	// units the parameters are expressed in.
	p := param.New()
	p.SetValue("width", units.Millimeters(120))
	p.SetExpr("height", "width * 0.6", units.Millimeter)
	p.SetExpr("hole_d", "min(width, height) / 3", units.Millimeter)

	// Add each dimension, then bind it to an expression evaluated against p.
	bind := func(d sketch.Dimension, expr string) {
		s.AddConstraint(d)
		if err := s.Bind(d, p, expr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	bind(sketch.NewDistance(a, b, 0), "width")
	bind(sketch.NewDistance(a, d, 0), "height")
	bind(sketch.NewHorizontalDistance(a, o, 0), "width / 2") // hole centered
	bind(sketch.NewVerticalDistance(a, o, 0), "height / 2")
	bind(sketch.NewRadius(hole, 0), "hole_d / 2")

	report := func(label string) {
		res, err := s.Solve()
		if err != nil {
			fmt.Fprintln(os.Stderr, label, "solve:", err)
			os.Exit(1)
		}
		w, _ := p.GetValue("width")
		fmt.Printf("%s: width=%s -> plate %.1f x %.1f mm, hole d=%.1f at (%.0f, %.0f), DOF %d\n",
			label, w, b.X(), d.Y(), 2*hole.R(), o.X(), o.Y(), res.DOF)
		svg, _ := s.SVG(sketch.DefaultSVGOptions())
		name := "plate_" + label + ".svg"
		os.WriteFile(name, []byte(svg), 0o644)
		fmt.Println("  wrote", name)
	}

	report("a") // width = 120 mm

	// Change the one driving parameter — and express it in inches. The units
	// library converts; height and hole follow automatically.
	p.SetValue("width", units.Inches(8))
	report("b")
}
