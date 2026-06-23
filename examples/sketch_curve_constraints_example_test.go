package examples_test

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_point_on_conic confines a free point to a fixed conic arc. The
// point starts below the arch and is pulled up onto the curve, keeping one
// sliding degree of freedom along it.
func Example_sketch_point_on_conic() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(4, 6)
	end := s.CreatePoint(8, 0)
	c, err := s.CreateConic(start, apex, end, 0.5) // 0.5 → parabola
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, p := range []*sketch.Point{start, apex, end} {
		s.Fix(p)
	}

	p := s.CreatePoint(4, 1) // below the arch interior
	s.AddConstraint(sketch.NewPointOnConic(p, c))

	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}

	// The point now lies on the curve and slid up onto its interior.
	// The point now lies on the curve. The sketch keeps two free DOF: the point's
	// one sliding DOF along the curve, plus the conic's free fullness parameter rho.
	cx, cy := c.Eval(0.5)
	fmt.Printf("on curve: %t\n", math.Hypot(p.X()-cx, p.Y()-cy) < 1e-6)
	fmt.Printf("free DOF: %d\n", s.DOF())

	// Output:
	// on curve: true
	// free DOF: 2
}

// Example_sketch_tangent_to_nurbs makes a horizontal line tangent to a fixed
// NURBS arch. The line settles at the curve's peak — the one contact whose
// tangent is horizontal.
func Example_sketch_tangent_to_nurbs() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())
	c0 := s.CreatePoint(0, 0)
	c1 := s.CreatePoint(4, 8)
	c2 := s.CreatePoint(8, 0)
	c, err := s.CreateNURBS(2, []*sketch.Point{c0, c1, c2}, nil, sketch.ClampedUniformKnots(3, 2))
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, p := range []*sketch.Point{c0, c1, c2} {
		s.Fix(p)
	}

	a := s.CreatePoint(-2, 3.5)
	b := s.CreatePoint(10, 3.5)
	line := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToNURBS(line, c))

	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}

	// The peak of this curve is at the parameter midpoint, y = 4.
	_, peakY := c.Eval(0.5)
	fmt.Printf("tangent at peak y=%.1f: %t\n", peakY, math.Abs(a.Y()-peakY) < 1e-3)

	// Output:
	// tangent at peak y=4.0: true
}
