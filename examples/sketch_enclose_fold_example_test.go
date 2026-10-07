package examples_test

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_encloseFold drives a four-bar linkage whose crank cannot turn
// fully. Past acos(0.04) ≈ 87.7076° the coupler and follower together are too
// short to reach A, so the branch the drive follows turns back there. Enclose
// refuses the quarter turn with a *FoldError: a proof that the branch reaches
// a largest crank angle inside a narrow interval and never gets past it, so
// every larger crank angle is out of the linkage's reach on this branch.
func Example_sketch_encloseFold() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	// Ground O2→O4 (100), crank O2→A (50), coupler A→B (60), follower O4→B
	// (50), seeded at a crank angle of 0° with B above the ground line.
	o2 := s.CreatePoint(0, 0)
	o4 := s.CreatePoint(100, 0)
	a := s.CreatePoint(50, 0)
	b := s.CreatePoint(86, 48)
	ground := s.CreateLine(o2, o4)
	crankLine := s.CreateLine(o2, a)
	s.CreateLine(a, b)
	s.CreateLine(o4, b)
	crank := sketch.NewAngle(ground, crankLine, 0)
	s.AddConstraint(
		sketch.NewCoincident(o2, s.Origin()),
		sketch.NewHorizontal(ground),
		sketch.NewDistance(o2, o4, 100),
		sketch.NewDistance(o2, a, 50),
		sketch.NewDistance(a, b, 60),
		sketch.NewDistance(o4, b, 50),
		crank,
	)
	if _, err := s.Solve(context.Background()); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	deg := func(v float64) float64 { return v * 180 / math.Pi }

	_, err := s.Enclose(context.Background(), crank, 0, math.Pi/2)
	var fold *sketch.FoldError
	if !errors.As(err, &fold) {
		fmt.Printf("failed to prove the fold: %v\n", err)
		return
	}
	fmt.Printf("refused as not certified: %v\n", errors.Is(err, sketch.ErrNotCertified))
	fmt.Printf("the branch turns back at %.4f°\n", deg(fold.Fold.Lo))
	fmt.Printf("fold enclosure under 1e-9 rad: %v\n", fold.Fold.Hi-fold.Fold.Lo < 1e-9)
	fmt.Printf("90° is out of reach: %v\n", math.Pi/2 > fold.Fold.Hi)
	// Output:
	// refused as not certified: true
	// the branch turns back at 87.7076°
	// fold enclosure under 1e-9 rad: true
	// 90° is out of reach: true
}
