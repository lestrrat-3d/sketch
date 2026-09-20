package sketch_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

// profileOrderPermutations is every ordering of the three entities whose endpoints
// form the near-coincident cluster each scene below is built around.
var profileOrderPermutations = [6][3]int{
	{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0},
}

// TestProfilesAreAuthoringOrderIndependent is the weld's order-independence as
// Sketch.Profiles() publishes it: the same drawing, authored in a different order,
// must report the same number of profiles with the same areas.
//
// Each scene holds three curve endpoints 0.9e-6 apart on a sketch 10 units across,
// where the arrangement's default merge tolerance is 1e-6 — so the cluster spans more
// than the tolerance, adjacent members weld and the outer two do not. Which member
// represents the cluster therefore decides whether the third curve joins the map or
// dangles and is pruned, and before the arrangement canonicalized its boundary points
// in lexicographic order that choice was the order the entities were created in.
// Nothing flags the disagreement, so the profile set is the only place it shows.
func TestProfilesAreAuthoringOrderIndependent(t *testing.T) {
	t.Run("three spokes into a triangle", func(t *testing.T) {
		requireSameProfilesEveryOrder(t, func(s *sketch.Sketch, order [3]int) {
			a := s.CreatePoint(-5, -5)
			b := s.CreatePoint(5, -5)
			c := s.CreatePoint(0, 5)
			s.CreateLine(a, b)
			s.CreateLine(b, c)
			s.CreateLine(c, a)
			spokes := [3]func(){
				func() { s.CreateLine(a, s.CreatePoint(0, 1)) },
				func() { s.CreateLine(b, s.CreatePoint(0, 1+0.9e-6)) },
				func() { s.CreateLine(c, s.CreatePoint(0, 1+1.8e-6)) },
			}
			for _, i := range order {
				spokes[i]()
			}
		})
	})

	t.Run("parallel sides with a brace", func(t *testing.T) {
		requireSameProfilesEveryOrder(t, func(s *sketch.Sketch, order [3]int) {
			br := s.CreatePoint(10, 0)
			tr := s.CreatePoint(10, 10)
			tl := s.CreatePoint(0, 10)
			s.CreateLine(br, tr)
			s.CreateLine(tl, tr)
			corner := [3]func(){
				func() { s.CreateLine(s.CreatePoint(0, 0), tl) },
				func() { s.CreateLine(s.CreatePoint(0.9e-6, 0), br) },
				func() { s.CreateLine(s.CreatePoint(1.8e-6, 0), tr) },
			}
			for _, i := range order {
				corner[i]()
			}
		})
	})
}

// requireSameProfilesEveryOrder builds the scene once per authoring order and requires
// every order to publish the same profile count and the same sorted areas.
func requireSameProfilesEveryOrder(t *testing.T, build func(*sketch.Sketch, [3]int)) {
	t.Helper()

	var want []float64
	var wantOrder string
	for _, order := range profileOrderPermutations {
		s := newSketch(t)
		build(s, order)
		profiles := s.Profiles()
		sort.Slice(profiles, func(i, j int) bool { return profiles[i].Area < profiles[j].Area })
		label := fmt.Sprint(order)
		if want == nil {
			require.NotEmptyf(t, profiles, "order %s must publish at least one profile", label)
			for _, p := range profiles {
				want = append(want, p.Area)
			}
			wantOrder = label
			continue
		}
		require.Lenf(t, profiles, len(want),
			"profile count differs between authoring orders %s and %s", wantOrder, label)
		for i, p := range profiles {
			sketchtest.MeasuresProfileArea(t, p, want[i], sketchtest.Within(1e-9))
		}
	}
}
