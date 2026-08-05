package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_freeformSharedPoint shows the one supported way to give a
// free-form curve's contact with another entity a whole (uncut) boundary edge:
// author the contact as a shared *Point instead of letting the curves cross.
//
// A sketch holding a FitSpline anywhere reports TExact = false on every
// BoundaryEdge (the whole-scene analytic-kind gate; see
// docs/analytic-arrangement-design.md, "The all-analytic gate"), so meeting
// that gate is not the point here. Partial is the flag that decides whether a
// trim needs certifying in the first place: a whole edge (Partial = false) has
// no crossing to certify, whatever TExact says.
func Example_sketch_freeformSharedPoint() {
	// build draws the top half of a circle (an Arc from (10,0) to (-10,0)) and
	// a FitSpline "flank" alongside it. When shared is false, the flank's
	// endpoints land past the arc, crossing it transversally twice. When
	// shared is true, the flank's endpoints ARE the arc's own Start/End
	// points — the same *Point, not merely equal coordinates — so the curves
	// meet without crossing at all.
	build := func(shared bool) (*sketch.Sketch, error) {
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		if err != nil {
			return nil, err
		}

		center := s.CreatePoint(0, 0)
		start := s.CreatePoint(10, 0)
		end := s.CreatePoint(-10, 0)
		s.CreateArc(center, start, end)

		if shared {
			mid := s.CreatePoint(0, -4)
			if _, err := s.CreateFitSpline(start, mid, end); err != nil {
				return nil, err
			}
			return s, nil
		}

		left := s.CreatePoint(-20, 4)
		midTop := s.CreatePoint(0, 4)
		right := s.CreatePoint(20, 4)
		if _, err := s.CreateFitSpline(left, midTop, right); err != nil {
			return nil, err
		}
		return s, nil
	}

	report := func(label string, s *sketch.Sketch) {
		profiles := s.Profiles()
		whole, partial := 0, 0
		for _, p := range profiles {
			for _, e := range p.Outer {
				if e.Partial {
					partial++
				} else {
					whole++
				}
			}
		}
		fmt.Printf("%s: profiles=%d whole edges=%d partial edges=%d\n", label, len(profiles), whole, partial)
	}

	crossing, err := build(false)
	if err != nil {
		fmt.Printf("failed to build the crossing scene: %s\n", err)
		return
	}
	shared, err := build(true)
	if err != nil {
		fmt.Printf("failed to build the shared-point scene: %s\n", err)
		return
	}

	report("crossing", crossing)
	report("shared point", shared)

	// Output:
	// crossing: profiles=1 whole edges=0 partial edges=2
	// shared point: profiles=1 whole edges=2 partial edges=0
}
