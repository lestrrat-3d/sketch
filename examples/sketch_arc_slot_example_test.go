package examples_test

import (
	"context"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
)

// Example_sketch_arcSlot builds Fusion's center-point arc slot and then its
// three-point form. CreateArcSlot takes the centerline's center, its start and
// a point in its end direction, plus the cap radius (half the width). Five
// dimensions fully constrain it; the three-point form is geom.ArcThroughPoints
// feeding the same builder.
func Example_sketch_arcSlot() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	sl, err := s.CreateArcSlot(0, 0, 20, 0, 0, 20, 3) // 90° about the origin, width 6
	if err != nil {
		fmt.Printf("failed to create arc slot: %s\n", err)
		return
	}
	fmt.Printf("created: DOF %d\n", s.DOF())

	radius := sketch.NewDistance(sl.Center, sl.C1, 20)
	s.AddConstraint(
		sketch.NewCoincident(sl.Center, s.Origin()),
		sketch.NewHorizontal(sl.Spokes[0]),
		sketch.NewAngle(sl.Spokes[0], sl.Spokes[1], 90),
		radius,
		sketch.NewRadius(sl.A1, 3),
	)
	report := func(label string) {
		res, err := s.Solve(context.Background())
		if err != nil {
			fmt.Printf("failed to solve: %s\n", err)
			return
		}
		fmt.Printf("%s: outer %.1f, inner %.1f, area %.4f, DOF %d\n",
			label, sl.Outer.R(), sl.Inner.R(), s.Profiles()[0].Area, res.DOF)
	}
	report("R 20")
	radius.Set(25)
	report("R 25")

	// Three-point form: the centerline through (20, 0), (0, 20) and (-20, 0).
	s2, _ := w.CreateSketch(w.XY())
	g, ok := geom.ArcThroughPoints(geom.NewPoint(20, 0), geom.NewPoint(0, 20), geom.NewPoint(-20, 0))
	if !ok {
		fmt.Println("collinear centerline")
		return
	}
	sl2, err := s2.CreateArcSlot(g.Center.X, g.Center.Y, g.Start.X, g.Start.Y, g.End.X, g.End.Y, 3)
	if err != nil {
		fmt.Printf("failed to create arc slot: %s\n", err)
		return
	}
	fmt.Printf("three-point: center (%.1f, %.1f), sweep %.0f deg\n",
		sl2.Center.X(), sl2.Center.Y(), sl2.Outer.Sweep()*180/math.Pi)

	// Output:
	// created: DOF 6
	// R 20: outer 23.0, inner 17.0, area 216.7699, DOF 0
	// R 25: outer 28.0, inner 22.0, area 263.8938, DOF 0
	// three-point: center (0.0, 0.0), sweep 180 deg
}
