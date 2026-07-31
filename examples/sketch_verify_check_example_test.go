package examples_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_verify_check shows how to gate on the oracle verdict while
// waiving ONE of its conditions.
//
// A sketch built from unsigned constraints — three distances on a triangle —
// is fully constrained yet admits the mirrored apex, so the ambiguity probe
// finds a second configuration and Trustworthy is false. A consumer that models
// this deliberately (Fusion behaves the same way, letting the seed pick the
// branch) still wants every other condition enforced. Check reports the failed
// conditions as data, so the waiver is decided per reason: a condition added to
// the verdict in a later release arrives as another reason and is fatal by
// default, because it is not in the waiver list.
func Example_sketch_verify_check() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(5, 6)
	s.Fix(a)
	ab := s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, a)
	s.AddConstraint(sketch.NewHorizontal(ab))
	s.AddConstraint(
		sketch.NewDistance(a, b, 10),
		sketch.NewDistance(a, c, 8),
		sketch.NewDistance(b, c, 7),
	)
	if _, err := s.Solve(context.Background()); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	rep := s.Verify(context.Background(), sketch.WithProbe())
	fmt.Printf("status=%s trustworthy=%t\n", rep.Status, rep.Trustworthy())

	// Gate on everything except ambiguity, which this scheme has by design.
	var blocking []error
	if err := rep.Check(); err != nil {
		for _, reason := range err.Unwrap() {
			if errors.Is(reason, sketch.ErrAmbiguous) {
				continue
			}
			blocking = append(blocking, reason)
		}
	}
	fmt.Printf("blocking reasons: %d\n", len(blocking))

	// The alternatives are reported rather than treated as a failure.
	fmt.Printf("configurations found: %d\n", len(rep.Probe.Configurations))

	// A sketch that fails a condition the caller does NOT waive still blocks, and
	// the reason names which one.
	s.CreateLine(s.CreatePoint(20, 0), s.CreatePoint(30, 0)) // nothing pins it
	if _, err := s.Solve(context.Background()); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	if err := s.Verify(context.Background()).Check(); err != nil {
		fmt.Println(errors.Is(err, sketch.ErrNotFullyConstrained))
	}

	// Output:
	// status=fully constrained trustworthy=false
	// blocking reasons: 0
	// configurations found: 2
	// true
}
