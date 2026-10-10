package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestUnionProfilesKeepsEmbeddedGearBoundary(t *testing.T) {
	for _, tc := range []struct {
		name          string
		teeth, points int
		pressure      float64
	}{
		{name: "30 teeth", teeth: 30, points: 5, pressure: math.Pi / 6},
		{name: "60 teeth", teeth: 60, points: 15, pressure: math.Pi / 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const module = 0.5
			pitch := module * float64(tc.teeth) / 2
			base := pitch * math.Cos(tc.pressure)
			root := (module*float64(tc.teeth) - 2.5*module) / 2
			tip := (module*float64(tc.teeth) + 2*module) / 2
			involute := func(radius float64) (float64, float64) {
				a := math.Acos(base / radius)
				v := math.Tan(a)
				return base * (math.Cos(v) + v*math.Sin(v)), base * (math.Sin(v) - v*math.Cos(v))
			}
			px, py := involute(pitch)
			rotation := math.Pi/(2*float64(tc.teeth)) - math.Atan2(-py, px)
			world := sketch.NewWorld()
			s, err := world.CreateSketch(world.XY())
			require.NoError(t, err)
			center := s.CreatePoint(0, 0)
			for tooth := 0; tooth < tc.teeth; tooth++ {
				angle := 2 * math.Pi * float64(tooth) / float64(tc.teeth)
				left, right := make([]*sketch.Point, tc.points), make([]*sketch.Point, tc.points)
				for i := range left {
					radius := base + (tip-base)*float64(i)/float64(tc.points-1)
					x, y := involute(radius)
					xl, yl := x*math.Cos(rotation)+y*math.Sin(rotation), x*math.Sin(rotation)-y*math.Cos(rotation)
					lx, ly := xl*math.Cos(angle)-yl*math.Sin(angle), xl*math.Sin(angle)+yl*math.Cos(angle)
					rx, ry := xl*math.Cos(angle)+yl*math.Sin(angle), xl*math.Sin(angle)-yl*math.Cos(angle)
					left[i], right[i] = s.CreatePoint(lx, ly), s.CreatePoint(rx, ry)
				}
				_, err := s.CreateFitSpline(right...)
				require.NoError(t, err)
				s.CreateArc(center, right[tc.points-1], left[tc.points-1])
				_, err = s.CreateFitSpline(left...)
				require.NoError(t, err)
			}
			s.CreateCircle(center, root)
			regions := s.Profiles()
			indices := make([]int, len(regions))
			for i := range regions {
				indices[i] = i
			}
			union, err := s.UnionProfiles(indices...)
			require.NoError(t, err)
			if tc.teeth == 30 {
				again, err := s.UnionProfiles(indices...)
				require.NoError(t, err)
				require.Equal(t, union, again)
			}
			require.True(t, union.Valid)
			require.Empty(t, union.Holes)
			require.Len(t, union.Outer, 4*tc.teeth)
			require.Equal(t, indices, union.UnionRegionIndices())
			var fits, tips, roots int
			var largest float64
			for _, edge := range union.Outer {
				require.True(t, edge.TExact)
				for _, p := range edge.Polyline {
					largest = math.Max(largest, math.Hypot(p[0], p[1]))
				}
				switch edge.Entity.(type) {
				case *sketch.FitSpline:
					fits++
					require.True(t, edge.Partial)
					require.Greater(t, edge.TStart, 0.0)
					require.Equal(t, 1.0, edge.TEnd)
				case *sketch.Arc:
					tips++
				case *sketch.Circle:
					roots++
				default:
					t.Fatalf("unexpected boundary entity %T", edge.Entity)
				}
			}
			require.Equal(t, 2*tc.teeth, fits)
			require.Equal(t, tc.teeth, tips)
			require.Equal(t, tc.teeth, roots)
			require.InDelta(t, tip, largest, 1e-9)
		})
	}
}

func TestUnionProfilesRejectsDisconnectedRegions(t *testing.T) {
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	s.CreateCircle(s.CreatePoint(0, 0), 1)
	s.CreateCircle(s.CreatePoint(5, 0), 1)
	require.Len(t, s.Profiles(), 2)
	_, err = s.UnionProfiles(0, 1)
	require.ErrorContains(t, err, "disconnected")
}
