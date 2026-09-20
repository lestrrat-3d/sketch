package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_chains shows the open counterpart of the profile engine: the
// geometry that encloses nothing is published as ordered open chains, which is
// what an open-curve operation (sweeping a line into a ribbon, revolving an open
// profile into an uncapped shell) consumes.
//
// The two publications partition the sketch. The closed rectangle is a profile
// and contributes no chain; the open run beside it is a chain and contributes no
// profile.
func Example_sketch_chains() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())
	s.CreateRectangle(0, 0, 40, 30) // closes a region

	// An open run: a line into a quarter arc, joined by a shared point.
	a := s.CreatePoint(60, 0)
	b := s.CreatePoint(70, 0)
	end := s.CreatePoint(80, 10)
	s.CreateLine(a, b)
	s.CreateArc(s.CreatePoint(70, 10), b, end)

	fmt.Printf("profiles: %d\n", len(s.Profiles()))

	chains := s.Chains()
	fmt.Printf("chains: %d\n", len(chains))

	chain := chains[0]
	fmt.Printf("entities on the chain: %d\n", len(chain.Entities))
	fmt.Printf("length: %.3f mm\n", chain.Length)
	fmt.Printf("valid: %t, self-intersecting: %t\n", chain.Valid, chain.SelfIntersecting)

	// Each edge carries the same trim contract a profile's boundary carries, so a
	// consumer can reject an inexact fragment before recording it.
	for i, e := range chain.Edges {
		fmt.Printf("edge %d: %T whole=%t exact=%t\n", i, e.Entity, !e.Partial, e.TExact)
	}

	// A chain is a snapshot of the geometry at one instant, exactly as a profile
	// is: check this before sweeping one.
	fmt.Printf("stale: %t\n", chain.IsStale())

	// Output:
	// profiles: 1
	// chains: 1
	// entities on the chain: 2
	// length: 25.708 mm
	// valid: true, self-intersecting: false
	// edge 0: *sketch.Line whole=true exact=true
	// edge 1: *sketch.Arc whole=true exact=true
	// stale: false
}
