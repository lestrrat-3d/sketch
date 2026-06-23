package examples_test

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_nurbs authors a general rational NURBS — the classic rational
// quadratic that traces an exact quarter circle (control points (1,0),(1,1),(0,1)
// with weights 1, 1/√2, 1 over the clamped knot vector {0,0,0,1,1,1}) — closes it
// back to the origin with two lines, and reads back the NURBS-bounded region's
// exact sector area, π/4. The curve degree, knots and weights are stored
// structural data, so a free NURBS has DOF 2·(n+1) — only its control points move.
func Example_sketch_nurbs() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())
	p0 := s.AddPoint(1, 0)
	p1 := s.AddPoint(1, 1)
	p2 := s.AddPoint(0, 1)

	c, err := s.AddNURBS(2,
		[]*sketch.Point{p0, p1, p2},
		[]float64{1, 1 / math.Sqrt2, 1}, // rational weights → an exact circle arc
		[]float64{0, 0, 0, 1, 1, 1},     // clamped quadratic knot vector
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	o := s.AddPoint(0, 0)
	s.AddLine(p2, o) // (0,1) → origin
	s.AddLine(o, p0) // origin → (1,0)

	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("degree %d, rational %t, dof %d\n", c.Degree(), c.Rational(), s.DOF())

	// The curve lies on the unit circle: a midpoint sample is at radius 1.
	mx, my := c.Eval(0.5)
	fmt.Printf("midpoint radius %.4f\n", math.Hypot(mx, my))

	profiles := s.Profiles()
	fmt.Printf("regions: %d, area %.4f (pi/4 = %.4f)\n",
		len(profiles), profiles[0].Area, math.Pi/4)

	// Output:
	// degree 2, rational true, dof 8
	// midpoint radius 1.0000
	// regions: 1, area 0.7854 (pi/4 = 0.7854)
}
