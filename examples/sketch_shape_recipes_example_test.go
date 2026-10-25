package examples_test

import (
	"context"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_polygonScribed builds Fusion's inscribed and circumscribed
// polygons from CreatePolygon. The two differ only in which size is
// dimensioned: an inscribed polygon fixes the center-to-corner distance, a
// circumscribed one the center-to-side distance. CreatePolygon takes a
// circumradius, so the circumscribed one is seeded at a / cos(π/n).
func Example_sketch_polygonScribed() {
	const n, size = 6, 10.0
	for _, inscribed := range []bool{true, false} {
		w := sketch.NewWorld()
		s, _ := w.CreateSketch(w.XY())

		seed := size
		if !inscribed {
			seed = size / math.Cos(math.Pi/n)
		}
		p, err := s.CreatePolygon(0, 0, n, seed)
		if err != nil {
			fmt.Printf("failed to create polygon: %s\n", err)
			return
		}
		// Ground the center on the origin and put the first corner on +x.
		s.AddConstraint(
			sketch.NewCoincident(p.Center, s.Origin()),
			sketch.NewHorizontalPoints(p.Center, p.Vertices[0]),
		)
		if inscribed {
			s.AddConstraint(sketch.NewDistance(p.Center, p.Vertices[0], size))
		} else {
			s.AddConstraint(sketch.NewDistancePointLine(p.Center, p.Sides[0], size))
		}

		res, err := s.Solve(context.Background())
		if err != nil {
			fmt.Printf("failed to solve: %s\n", err)
			return
		}
		name := "circumscribed"
		if inscribed {
			name = "inscribed"
		}
		fmt.Printf("%s: first corner (%.4f, %.4f), side %.4f, DOF %d\n",
			name, p.Vertices[0].X(), p.Vertices[0].Y(), p.Sides[0].Length(), res.DOF)
	}

	// Output:
	// inscribed: first corner (10.0000, 0.0000), side 10.0000, DOF 0
	// circumscribed: first corner (11.5470, 0.0000), side 11.5470, DOF 0
}

// Example_sketch_slotOverall dimensions a slot by its overall length, as
// Fusion's overall slot does, and centers it on the origin, as Fusion's
// center-point slot does. CreateSlot builds the slot from its cap centers; two
// tip points, each held on the slot axis and on its cap, give the overall
// length a dimension of its own. Each cap sweeps only the half facing away from
// the slot, so the point-on-arc constraint admits only the far tip. (Without tip
// points, bind the center distance to "L - W" and the cap radius to "W / 2"
// through the parameter table instead.)
func Example_sketch_slotOverall() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	sl, err := s.CreateSlot(-15, 0, 15, 0, 5)
	if err != nil {
		fmt.Printf("failed to create slot: %s\n", err)
		return
	}
	axis := s.CreateLine(sl.C1, sl.C2)
	axis.SetConstruction(true)
	t1, t2 := s.CreatePoint(-20, 0), s.CreatePoint(20, 0)
	mid := s.CreatePoint(0, 0)
	overall := sketch.NewDistance(t1, t2, 40)
	s.AddConstraint(
		sketch.NewPointOnLine(t1, axis), sketch.NewPointOnArc(t1, sl.A1),
		sketch.NewPointOnLine(t2, axis), sketch.NewPointOnArc(t2, sl.A2),
		// Center-point placement: the axis midpoint sits on the origin.
		sketch.NewMidpoint(mid, axis), sketch.NewCoincident(mid, s.Origin()),
		sketch.NewHorizontal(axis),
		overall,
		sketch.NewDistance(sl.C1, sl.L1.Start, 5), // cap radius: width 10
	)

	report := func() {
		res, err := s.Solve(context.Background())
		if err != nil {
			fmt.Printf("failed to solve: %s\n", err)
			return
		}
		fmt.Printf("overall %.1f: tips %.1f..%.1f, centers %.1f..%.1f, DOF %d\n",
			overall.Target().Base(), t1.X(), t2.X(), sl.C1.X(), sl.C2.X(), res.DOF)
	}
	report()
	overall.Set(50)
	report()

	// Output:
	// overall 40.0: tips -20.0..20.0, centers -15.0..15.0, DOF 0
	// overall 50.0: tips -25.0..25.0, centers -20.0..20.0, DOF 0
}
