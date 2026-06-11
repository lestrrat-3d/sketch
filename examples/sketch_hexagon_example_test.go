package examples_test

import (
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
)

// Example_sketch_hexagon builds a regular hexagon entirely from geometric and
// dimensional constraints — no vertex is positioned by hand beyond a rough
// initial guess — then solves it and reports the vertices.
func Example_sketch_hexagon() {
	s := sketch.New()

	const side = 30.0
	const n = 6

	// Generic geometry: vertices (rough guesses on a circle) and edges.
	gp := make([]*geom.Point, n)
	for i := range gp {
		a := float64(i)/float64(n)*2*math.Pi + 0.15 // perturbed
		gp[i] = geom.NewPoint(40*math.Cos(a)+5, 40*math.Sin(a)-3)
	}

	// Commit the edges (each pulls in its endpoints) and grab the bound points.
	lines := make([]*sketch.Line, n)
	for i := range lines {
		lines[i] = s.AddLine(geom.NewLine(gp[i], gp[(i+1)%n]))
	}
	pts := make([]*sketch.Point, n)
	for i := range pts {
		pts[i] = s.AddPoint(gp[i]) // idempotent: returns the already-bound point
	}

	// Ground one vertex, make the first edge horizontal, and dimension it.
	pts[0].MoveTo(0, 0)
	s.Fix(pts[0])
	s.AddConstraint(sketch.NewHorizontal(lines[0]))
	s.AddConstraint(sketch.NewDistance(pts[0], pts[1], side))

	// Every edge equal in length and every interior turn 60° (exterior angle).
	for i := 1; i < n; i++ {
		s.AddConstraint(sketch.NewEqual(lines[0], lines[i]))
	}
	for i := 0; i < n-1; i++ {
		s.AddConstraint(sketch.NewAngle(lines[i], lines[i+1], 60)) // degrees
	}

	res, err := s.Solve()
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	// Round for printing only, mapping a residual-sized -0.000 onto +0.000 so
	// the output stays exact.
	r := func(v float64) float64 { return math.Round(v*1000)/1000 + 0 }
	fmt.Printf("DOF %d, redundant %d\n", res.DOF, res.Redundant)
	for i, p := range pts {
		fmt.Printf("p%d = (%7.3f, %7.3f)\n", i, r(p.X()), r(p.Y()))
	}

	// The two redundant equations are the hexagon's closure: five 60° turns
	// and five equal sides already force the sixth of each.

	// Output:
	// DOF 0, redundant 2
	// p0 = (  0.000,   0.000)
	// p1 = ( 30.000,   0.000)
	// p2 = ( 45.000,  25.981)
	// p3 = ( 30.000,  51.962)
	// p4 = (  0.000,  51.962)
	// p5 = (-15.000,  25.981)
}
