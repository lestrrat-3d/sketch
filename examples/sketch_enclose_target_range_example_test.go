package examples_test

import (
	"context"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_encloseTargetRange certifies a four-bar whose coupler must be
// exactly √6401 mm long, a length no float64 holds. NewDistance can only take
// the nearest float64, so the enclosure would describe a coupler up to half an
// ulp off. WithTargetRange states the length as the two floats around the
// root instead, and the enclosure's claims then hold for every length between
// them, the exact √6401 included.
func Example_sketch_encloseTargetRange() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	o2 := s.CreatePoint(0, 0)
	o4 := s.CreatePoint(100, 0)
	a := s.CreatePoint(0, 30)
	b := s.CreatePoint(72, 64)
	ground := s.CreateLine(o2, o4)
	crankLine := s.CreateLine(o2, a)
	s.CreateLine(a, b)
	followerLine := s.CreateLine(o4, b)
	crank := sketch.NewAngle(ground, crankLine, 90)
	coupler := sketch.NewDistance(a, b, math.Sqrt(6401))
	follower := sketch.NewAngle(ground, followerLine, 0)
	follower.SetDriven(true)
	s.AddConstraint(
		sketch.NewCoincident(o2, s.Origin()),
		sketch.NewHorizontal(ground),
		sketch.NewDistance(o2, o4, 100),
		sketch.NewDistance(o2, a, 30),
		coupler,
		sketch.NewDistance(o4, b, 70),
		crank,
		follower,
	)
	if _, err := s.Solve(context.Background()); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	// 6401 is exact and math.Sqrt rounds correctly, so one float step either
	// side of its result brackets the exact root.
	root := math.Sqrt(6401)
	lo, hi := math.Nextafter(root, 0), math.Nextafter(root, math.Inf(1))
	e, err := s.Enclose(context.Background(), crank, math.Pi/2, math.Pi/2,
		sketch.WithTargetRange(coupler, lo, hi))
	if err != nil {
		fmt.Printf("failed to enclose: %s\n", err)
		return
	}
	f, _ := e.Driven(follower)
	fmt.Printf("follower: %.4f°\n", f.Lo*180/math.Pi)
	fmt.Printf("width under 1e-12 rad: %v\n", f.Hi-f.Lo < 1e-12)
	// Output:
	// follower: 113.3199°
	// width under 1e-12 rad: true
}
