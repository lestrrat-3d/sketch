package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_profiles shows the profile/region engine: overlapping and
// nested geometry is arranged into closed regions with holes and area, and the
// verification report flags whether every region is a valid (extrudable)
// profile.
func Example_sketch_profiles() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())
	s.AddRectangle(0, 0, 40, 30)       // a plate
	s.AddCircle(s.AddPoint(20, 15), 5) // a bolt hole inside it

	profiles := s.Profiles()
	// The plate is the region carrying the circular hole; the hole interior is
	// itself a separate disk region.
	var plate, disk *sketch.Profile
	for _, p := range profiles {
		if len(p.Holes) == 1 {
			plate = p
		} else {
			disk = p
		}
	}

	fmt.Printf("regions: %d\n", len(profiles))
	fmt.Printf("plate: %d sides, %d hole, area %.1f\n", len(plate.Entities), len(plate.Holes), plate.Area)
	fmt.Printf("bolt-hole disk area: %.1f\n", disk.Area)

	rep := s.Verify()
	fmt.Printf("profiles valid: %t\n", rep.ProfilesValid)

	// Output:
	// regions: 2
	// plate: 4 sides, 1 hole, area 1121.5
	// bolt-hole disk area: 78.5
	// profiles valid: true
}
