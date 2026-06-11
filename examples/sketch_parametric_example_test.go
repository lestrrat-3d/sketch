package examples_test

import (
	"errors"
	"fmt"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
)

// Example_sketch_parametric drives a sketch from a parameter table: a
// rectangular plate with a centered hole whose dimensions are all defined by
// expressions. Changing a single parameter and re-solving updates everything.
func Example_sketch_parametric() {
	s := sketch.New()

	// Four corners + a center point for the hole (rough initial guesses).
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 1)
	c := s.AddPoint(9, 6)
	d := s.AddPoint(1, 5)
	o := s.AddPoint(5, 3)

	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	hole := s.AddCircle(o, 1)

	// Geometric constraints: grounded origin, axis-aligned rectangle.
	a.MoveTo(0, 0)
	s.Fix(a)
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
	if err := errors.Join(
		p.SetValue("width", units.Millimeters(120)),
		p.SetExpr("height", "width * 0.6", units.Millimeter),
		p.SetExpr("hole_d", "min(width, height) / 3", units.Millimeter),
	); err != nil {
		fmt.Printf("failed to define parameters: %s\n", err)
		return
	}

	// Add each dimension, then bind it to an expression evaluated against p.
	bind := func(dim sketch.Dimension, expr string) error {
		s.AddConstraint(dim)
		return s.Bind(dim, p, expr)
	}
	if err := errors.Join(
		bind(sketch.NewDistance(a, b, 0), "width"),
		bind(sketch.NewDistance(a, d, 0), "height"),
		bind(sketch.NewHorizontalDistance(a, o, 0), "width / 2"), // hole centered
		bind(sketch.NewVerticalDistance(a, o, 0), "height / 2"),
		bind(sketch.NewRadius(hole, 0), "hole_d / 2"),
	); err != nil {
		fmt.Printf("failed to bind dimensions: %s\n", err)
		return
	}

	report := func() error {
		res, err := s.Solve()
		if err != nil {
			return err
		}
		w, err := p.GetValue("width")
		if err != nil {
			return err
		}
		fmt.Printf("width=%s -> plate %.1f x %.1f mm, hole d=%.1f at (%.0f, %.0f), DOF %d\n",
			w, b.X(), d.Y(), 2*hole.R(), o.X(), o.Y(), res.DOF)
		return nil
	}

	if err := report(); err != nil { // width = 120 mm
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	// Change the one driving parameter — and express it in inches. The units
	// library converts; height and hole follow automatically.
	if err := p.SetValue("width", units.Inches(8)); err != nil {
		fmt.Printf("failed to update width: %s\n", err)
		return
	}
	if err := report(); err != nil {
		fmt.Printf("failed to solve after edit: %s\n", err)
		return
	}

	// Output:
	// width=120 mm -> plate 120.0 x 72.0 mm, hole d=24.0 at (60, 36), DOF 0
	// width=8 in -> plate 203.2 x 121.9 mm, hole d=40.6 at (102, 61), DOF 0
}
