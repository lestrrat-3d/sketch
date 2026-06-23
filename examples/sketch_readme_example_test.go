package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_quickstart builds an axis-aligned rectangle entirely from
// constraints, edits one dimension and re-solves, then exports the result. It
// is the smallest end-to-end taste of the engine: author geometry from points,
// constrain it, solve, edit, re-solve, export.
func Example_sketch_quickstart() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	// Four corners as rough initial guesses; the solver finds the exact spots.
	// Sharing a *Point between two lines is what makes a corner a corner.
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)

	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)

	// Ground one corner at the origin so the sketch can't float away.
	a.MoveTo(0, 0)
	s.Fix(a)

	// Axis-align the four sides.
	s.AddConstraint(
		sketch.NewHorizontal(ab),
		sketch.NewHorizontal(dc),
		sketch.NewVertical(ad),
		sketch.NewVertical(bc),
	)

	// Driving dimensions: editable values that make the sketch parametric.
	width := sketch.NewDistance(a, b, 20)
	height := sketch.NewDistance(a, d, 12)
	s.AddConstraint(width, height)

	res, err := s.Solve()
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("DOF=%d b=(%.0f,%.0f) c=(%.0f,%.0f) d=(%.0f,%.0f)\n",
		res.DOF, b.X(), b.Y(), c.X(), c.Y(), d.X(), d.Y())

	// Edit a dimension and re-solve: the rectangle becomes 35 x 12.
	width.Set(35)
	if _, err := s.Solve(); err != nil {
		fmt.Printf("failed to re-solve: %s\n", err)
		return
	}
	fmt.Printf("after width.Set(35): b=(%.0f,%.0f) c=(%.0f,%.0f)\n",
		b.X(), b.Y(), c.X(), c.Y())

	// Export the solved sketch in several formats.
	svg, err := s.SVG()
	if err != nil {
		fmt.Printf("failed to render SVG: %s\n", err)
		return
	}
	dxf, err := s.DXF()
	if err != nil {
		fmt.Printf("failed to render DXF: %s\n", err)
		return
	}
	data, err := s.MarshalJSON()
	if err != nil {
		fmt.Printf("failed to marshal JSON: %s\n", err)
		return
	}
	fmt.Printf("exports non-empty: svg=%t dxf=%t json=%t\n", len(svg) > 0, len(dxf) > 0, len(data) > 0)

	// Output:
	// DOF=0 b=(20,0) c=(20,12) d=(0,12)
	// after width.Set(35): b=(35,0) c=(35,12)
	// exports non-empty: svg=true dxf=true json=true
}
