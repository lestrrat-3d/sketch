package examples_test

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_enclose certifies the pose of a four-bar linkage. The crank
// angle drives the loop; the follower angle is whatever closes it. Enclose
// proves an interval for the follower that holds the EXACT solution, with that
// solution the only one in the certified box, first at one crank angle and
// then across a quarter turn. A crank angle where the linkage cannot assemble
// is refused, and a refusal never carries an enclosure.
func Example_sketch_enclose() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	// Ground O2→O4 (100), crank O2→A (30), coupler A→B (80), follower O4→B (70),
	// seeded with B above the ground line.
	o2 := s.CreatePoint(0, 0)
	o4 := s.CreatePoint(100, 0)
	a := s.CreatePoint(0, 30)
	b := s.CreatePoint(72, 64)
	ground := s.CreateLine(o2, o4)
	crankLine := s.CreateLine(o2, a)
	s.CreateLine(a, b)
	followerLine := s.CreateLine(o4, b)
	crank := sketch.NewAngle(ground, crankLine, 90)
	followerLength := sketch.NewDistance(o4, b, 70)
	follower := sketch.NewAngle(ground, followerLine, 0)
	follower.SetDriven(true) // measured, not driving
	s.AddConstraint(
		sketch.NewCoincident(o2, s.Origin()),
		sketch.NewHorizontal(ground),
		sketch.NewDistance(o2, o4, 100),
		sketch.NewDistance(o2, a, 30),
		sketch.NewDistance(a, b, 80),
		followerLength,
		crank,
		follower,
	)
	if _, err := s.Solve(context.Background()); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	deg := func(v float64) float64 { return v * 180 / math.Pi }

	// One crank angle, stated in radians (the base unit).
	e, err := s.Enclose(context.Background(), crank, math.Pi/2, math.Pi/2)
	if err != nil {
		fmt.Printf("failed to enclose: %s\n", err)
		return
	}
	f, _ := e.Driven(follower)
	fmt.Printf("follower at 90°: %.4f°\n", deg(f.Lo))

	// A quarter turn: the follower dips to its minimum and comes back, and the
	// interval holds every angle it takes on the way.
	e, err = s.Enclose(context.Background(), crank, 0, math.Pi/2)
	if err != nil {
		fmt.Printf("failed to enclose: %s\n", err)
		return
	}
	f, _ = e.Driven(follower)
	fmt.Printf("follower over [0°, 90°]: holds 101.537°: %v, holds 113.325°: %v\n",
		f.Contains(101.537*math.Pi/180), f.Contains(113.325*math.Pi/180))

	// With a 20 mm follower the coupler and follower together (100 mm) cannot
	// span the 104.4 mm from A to O4 at 90°, so no pose exists and the request
	// is refused.
	followerLength.Set(20)
	e, err = s.Enclose(context.Background(), crank, math.Pi/2, math.Pi/2)
	fmt.Printf("short follower: refused %v, enclosure %v\n", errors.Is(err, sketch.ErrNotConverged), e)
	// Output:
	// follower at 90°: 113.3250°
	// follower over [0°, 90°]: holds 101.537°: true, holds 113.325°: true
	// short follower: refused true, enclosure <nil>
}
