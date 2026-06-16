package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_world places a 2D sketch on a construction plane inside a 3D
// world and reads its geometry back in world coordinates. The sketch is solved
// in plane-local 2D exactly as always; the world coordinates are a derived
// read-out through the plane's frame.
func Example_sketch_world() {
	w := sketch.NewWorld()

	// A construction plane parallel to XY, lifted 5 units along its normal (+Z).
	top, err := w.OffsetPlane(w.XY(), 5)
	if err != nil {
		fmt.Printf("failed to make plane: %s\n", err)
		return
	}

	// A 4×3 rectangle on that plane, authored in plane-local (u, v) coordinates
	// between two opposite corners.
	s, err := w.Sketch(top)
	if err != nil {
		fmt.Printf("failed to make sketch: %s\n", err)
		return
	}
	rect := s.AddRectangle(0, 0, 4, 3)
	s.Fix(rect.A)

	if _, err := s.Solve(); err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}

	// Local coordinates are unchanged 2D; World lifts them into the 3D world.
	far := rect.C // the far corner at local (4, 3)
	fmt.Printf("local=(%.0f,%.0f) world=(%.0f,%.0f,%.0f)\n",
		far.X(), far.Y(),
		far.World().X, far.World().Y, far.World().Z)

	// The plane's normal points along +Z.
	f, _ := top.Frame()
	n := f.N()
	fmt.Printf("normal=(%.0f,%.0f,%.0f)\n", n.X, n.Y, n.Z)

	// Output:
	// local=(4,3) world=(4,3,5)
	// normal=(0,0,1)
}
