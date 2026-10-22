package sketch_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// toothFlank is the right flank of one tooth, base circle to tip. The left flank
// mirrors it about the x axis.
type toothFlank struct {
	teeth int
	root  float64
	right [][2]float64
}

var toothFlanks = []toothFlank{
	{30, 6.875, [][2]float64{
		{6.458658127496257, -0.6879209199657577}, {6.841508054997349, -0.6401623767109843},
		{7.229470770932984, -0.5122396760880823}, {7.616880532442486, -0.32468644820586323},
		{7.999562566443748, -0.0836585055563398},
	}},
	{31, 7.125, [][2]float64{
		{6.675137859909099, -0.6995781237347055}, {7.066384996855529, -0.6506064356739989},
		{7.462738224012067, -0.5202226226366642}, {7.85852569254962, -0.3293510246876924},
		{8.249569546660835, -0.08427511380072772},
	}},
}

// toothScene copies the tooth teeth times about the origin. A tooth is the two flanks
// and a tip arc about the origin; with baseArc it also holds an arc about the origin
// from the right flank's first point to the left flank's. The flanks cross the root
// circle.
func toothScene(t *testing.T, d toothFlank, baseArc bool) *sketch.Sketch {
	t.Helper()
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	center := s.Origin()
	circle := s.CreateCircle(center, d.root)
	s.AddConstraint(sketch.NewRadius(circle, d.root))
	for k := range d.teeth {
		a := 2 * math.Pi * float64(k) / float64(d.teeth)
		c, sn := math.Cos(a), math.Sin(a)
		pt := func(p [2]float64) *sketch.Point {
			q := s.CreatePoint(p[0]*c-p[1]*sn, p[0]*sn+p[1]*c)
			s.Fix(q)
			return q
		}
		right := make([]*sketch.Point, len(d.right))
		left := make([]*sketch.Point, len(d.right))
		for i, p := range d.right {
			right[i], left[i] = pt(p), pt([2]float64{p[0], -p[1]})
		}
		_, err := s.CreateFitSpline(right...)
		require.NoError(t, err)
		s.CreateArc(center, right[len(right)-1], left[len(left)-1])
		_, err = s.CreateFitSpline(left...)
		require.NoError(t, err)
		if baseArc {
			s.CreateArc(center, right[0], left[0])
		}
	}
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	return s
}

// TestEmbeddedToothBasesOverlapNeighbours pins what a tooth whose base arc is wider than
// the pitch does. The half angle of the tooth at its base circle exceeds half the pitch,
// so the first flank point of one tooth lies inside the base arc of the next, the two
// arcs overlap on one circle, and the two flanks cross inside the root circle. That is
// an overlap in the input, so no profile is certified. The same teeth drawn without base
// arcs are the original gear outline and produce certified profiles and a certified
// union.
func TestEmbeddedToothBasesOverlapNeighbours(t *testing.T) {
	for _, d := range toothFlanks {
		t.Run(fmt.Sprintf("n=%d", d.teeth), func(t *testing.T) {
			half := math.Atan2(-d.right[0][1], d.right[0][0])
			require.Greater(t, half, math.Pi/float64(d.teeth), "adjacent base arcs overlap")

			overlapping := toothScene(t, d, true).Profiles()
			require.NotEmpty(t, overlapping)
			for i, p := range overlapping {
				require.Falsef(t, p.Valid, "profile %d of the overlapping scene", i)
			}

			s := toothScene(t, d, false)
			profiles := s.Profiles()
			require.Len(t, profiles, 2*d.teeth+1)
			var valid []int
			for i, p := range profiles {
				require.Truef(t, p.Valid, "profile %d", i)
				valid = append(valid, i)
			}
			u, err := s.UnionProfiles(valid...)
			require.NoError(t, err)
			require.Len(t, u.Outer, 4*d.teeth)
			for _, e := range u.Outer {
				if e.Partial {
					require.Truef(t, e.TExact, "%T t=[%v %v]", e.Entity, e.TStart, e.TEnd)
				}
			}
		})
	}
}
