package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

// TestProfilesCoincidentEmittedLinesKeepBothProfiles is a seed-16 fuzz reduction of
// the fu89 coincident-emitted-edge defect, at the sketch layer: an arc plus two lines
// that each start a few billionths of a unit from the arc's own start point (rather
// than sharing it exactly, as a fuzzer's float perturbation would produce) and end at
// two distinct far points. With the two lines sharing the arc's start point exactly,
// Sketch.Profiles() publishes two profiles (12.2569389349084 and 0.141589935698213);
// with the jittered starts below dedupCoincidentStraightEdges (geom/arrange.go) it
// published only the smaller one, silently dropping the big profile with every
// profile it did publish still reporting Valid=true.
func TestProfilesCoincidentEmittedLinesKeepBothProfiles(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(6.42203922038517, 6.002014049507521)
	arcStart := s.CreatePoint(4.360854739734771, 7.634484604376455)
	arcEnd := s.CreatePoint(7.3114299349605485, 8.476367648453161)
	s.CreateArc(center, arcStart, arcEnd)

	// The two line starts are NOT the same *Point as arcStart — each is a few
	// billionths away, reproducing the fuzzer's float perturbation rather than a
	// genuinely shared corner.
	line1Start := s.CreatePoint(4.360854750178806, 7.63448462148625)
	line2Start := s.CreatePoint(4.360854740845146, 7.634484600917623)
	s.CreateLine(line1Start, s.CreatePoint(8.769796894569144, 4.818174753361967))
	s.CreateLine(line2Start, s.CreatePoint(8.793814668499206, 4.867059072642093))

	profiles := s.Profiles()
	require.Len(t, profiles, 2)
	for _, p := range profiles {
		sketchtest.IsValidProfile(t, p)
	}

	sketchtest.MeasuresProfileArea(t, smallerProfile(profiles), 0.141589935698213, sketchtest.Within(1e-6))
	sketchtest.MeasuresProfileArea(t, largerProfile(profiles), 12.2569389349084, sketchtest.Within(1e-6))

	report := s.Verify(t.Context())
	require.True(t, report.ProfilesValid)
}

// TestProfilesCoincidentEmittedLinesMatchEveryOrder is the sketch-layer order-
// independence check for the same seed-16 reduction: authoring the arc and the two
// lines in a different order must publish the same two profiles.
func TestProfilesCoincidentEmittedLinesMatchEveryOrder(t *testing.T) {
	build := func(order [3]int) []float64 {
		s := newSketch(t)
		center := s.CreatePoint(6.42203922038517, 6.002014049507521)
		arcStart := s.CreatePoint(4.360854739734771, 7.634484604376455)
		arcEnd := s.CreatePoint(7.3114299349605485, 8.476367648453161)
		line1Start := s.CreatePoint(4.360854750178806, 7.63448462148625)
		line2Start := s.CreatePoint(4.360854740845146, 7.634484600917623)
		line1End := s.CreatePoint(8.769796894569144, 4.818174753361967)
		line2End := s.CreatePoint(8.793814668499206, 4.867059072642093)
		steps := [3]func(){
			func() { s.CreateArc(center, arcStart, arcEnd) },
			func() { s.CreateLine(line1Start, line1End) },
			func() { s.CreateLine(line2Start, line2End) },
		}
		for _, i := range order {
			steps[i]()
		}
		profiles := s.Profiles()
		areas := []float64{profiles[0].Area, profiles[1].Area}
		if areas[0] > areas[1] {
			areas[0], areas[1] = areas[1], areas[0]
		}
		return areas
	}

	perms := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	want := build(perms[0])
	require.Len(t, want, 2)
	for _, perm := range perms[1:] {
		got := build(perm)
		require.Lenf(t, got, len(want), "order %v", perm)
		for i := range want {
			require.InDeltaf(t, want[i], got[i], want[i]*1e-6+1e-12, "order %v area %d", perm, i)
		}
	}
}

func smallerProfile(profiles []*sketch.Profile) *sketch.Profile {
	if profiles[0].Area <= profiles[1].Area {
		return profiles[0]
	}
	return profiles[1]
}

func largerProfile(profiles []*sketch.Profile) *sketch.Profile {
	if profiles[0].Area >= profiles[1].Area {
		return profiles[0]
	}
	return profiles[1]
}
