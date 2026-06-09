// Command hexagon builds a regular hexagon entirely from geometric and
// dimensional constraints — no vertex is positioned by hand beyond a rough
// initial guess — then solves it and writes SVG, DXF and JSON.
package main

import (
	"fmt"
	"math"
	"os"

	"github.com/lestrrat-3d/sketch"
)

func main() {
	s := sketch.New()

	const side = 30.0
	const n = 6

	// Rough initial guesses on a circle; the solver finds the exact hexagon.
	pts := make([]*sketch.Point, n)
	for i := range pts {
		a := float64(i)/float64(n)*2*math.Pi + 0.15 // perturbed
		pts[i] = s.AddPoint(40*math.Cos(a)+5, 40*math.Sin(a)-3)
	}

	lines := make([]*sketch.Line, n)
	for i := range lines {
		lines[i] = s.AddLine(pts[i], pts[(i+1)%n])
	}

	// Ground one vertex, make the first edge horizontal, and dimension it.
	s.Lock(pts[0], 0, 0)
	s.Horizontal(lines[0])
	s.Distance(pts[0], pts[1], side)

	// Every edge equal in length and every interior turn 60° (exterior angle).
	for i := 1; i < n; i++ {
		s.Equal(lines[0], lines[i])
	}
	for i := 0; i < n-1; i++ {
		s.Angle(lines[i], lines[i+1], math.Pi/3)
	}

	res, err := s.Solve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "solve:", err)
		os.Exit(1)
	}
	fmt.Printf("solved in %d iterations, residual %.2e, DOF %d, redundant %d\n",
		res.Iterations, res.Residual, res.DOF, res.Redundant)
	for i, p := range pts {
		fmt.Printf("  p%d = (%7.3f, %7.3f)\n", i, p.X(), p.Y())
	}

	write := func(name, data string) {
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", name)
	}

	svg, _ := s.SVG(sketch.DefaultSVGOptions())
	write("hexagon.svg", svg)
	dxf, _ := s.DXF()
	write("hexagon.dxf", dxf)
	js, _ := s.MarshalJSON()
	write("hexagon.json", string(js))
}
