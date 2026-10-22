package sketch_test

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// splineSectionScene builds a root disc and n identical closed regions placed
// evenly about the origin. Each region is made of six curves:
//
//	line    from a point ON the circle to the first fit point
//	spline  through fitPoints points (one side)
//	arc     about the origin from the last fit point of one side to the other's
//	spline  through fitPoints points (the other side)
//	line    from the first fit point of that side back to the circle
//	arc     about the origin between the two circle points, lying on the circle
//	        (only when rootArc is set)
//
// Every point is fixed. No curve crosses the circle or another curve. With
// rootArc unset the circle closes the region.
func splineSectionScene(t testing.TB, n, fitPoints int, rootArc bool) *sketch.Sketch {
	t.Helper()
	const rootRadius = 3.0
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	center := s.Origin()
	pitch := math.Pi / float64(n)
	for k := range n {
		rot := 2 * math.Pi * float64(k) / float64(n)
		point := func(r, a float64) *sketch.Point {
			p := s.CreatePoint(r*math.Cos(a+rot), r*math.Sin(a+rot))
			s.Fix(p)
			return p
		}
		a := 0.4 * pitch
		rootRight, rootLeft := point(rootRadius, -a), point(rootRadius, a)
		right := make([]*sketch.Point, fitPoints)
		left := make([]*sketch.Point, fitPoints)
		for i := range fitPoints {
			f := float64(i) / float64(fitPoints-1)
			r := 3.35 + f*(4.0-3.35)
			half := a * (1 - 0.4*f)
			right[i], left[i] = point(r, -half), point(r, half)
		}
		s.CreateLine(rootRight, right[0])
		_, err := s.CreateFitSpline(right...)
		require.NoError(t, err)
		s.CreateArc(center, right[fitPoints-1], left[fitPoints-1])
		_, err = s.CreateFitSpline(left...)
		require.NoError(t, err)
		s.CreateLine(left[0], rootLeft)
		if rootArc {
			s.CreateArc(center, rootRight, rootLeft)
		}
	}
	circle := s.CreateCircle(center, rootRadius)
	s.AddConstraint(sketch.NewRadius(circle, rootRadius))
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	return s
}

func validProfileIndices(s *sketch.Sketch) []int {
	var valid []int
	for i, p := range s.Profiles() {
		if p.Valid {
			valid = append(valid, i)
		}
	}
	return valid
}

// TestUnionProfilesCertifiesSplineSections pins that the union of a disc and closed
// regions bounded by lines, fit splines and arcs publishes an exact parameter on
// every Partial edge, which a consumer needs before it records the fragment.
func TestUnionProfilesCertifiesSplineSections(t *testing.T) {
	for _, n := range []int{8, 12, 17, 24} {
		for _, fit := range []int{4, 15} {
			for _, rootArc := range []bool{false, true} {
				t.Run(fmt.Sprintf("n=%d/fit=%d/rootArc=%v", n, fit, rootArc), func(t *testing.T) {
					s := splineSectionScene(t, n, fit, rootArc)
					valid := validProfileIndices(s)
					require.Len(t, valid, n+1, "the disc and one region each")
					u, err := s.UnionProfiles(valid...)
					require.NoError(t, err)
					require.Len(t, u.Outer, 6*n)
					partial := 0
					for _, e := range u.Outer {
						if !e.Partial {
							continue
						}
						partial++
						require.Truef(t, e.TExact, "%T partial edge t=[%v %v]", e.Entity, e.TStart, e.TEnd)
					}
					require.Equal(t, n, partial, "one circle fragment between neighbouring regions")
				})
			}
		}
	}
}

// embeddedSplineSectionScene builds a root disc and n identical regions placed evenly
// about the origin. Each region is a chord, two fit splines of fitPoints points that
// start inside the disc and cross its circle once, and a tip arc about the origin.
// Every point is fixed and no two regions meet.
func embeddedSplineSectionScene(t testing.TB, n, fitPoints int) *sketch.Sketch {
	t.Helper()
	const (
		rootRadius = 3.0
		innerR     = 2.6
		outerR     = 4.0
	)
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	center := s.Origin()
	pitch := math.Pi / float64(n)
	for k := range n {
		rot := 2 * math.Pi * float64(k) / float64(n)
		point := func(r, a float64) *sketch.Point {
			p := s.CreatePoint(r*math.Cos(a+rot), r*math.Sin(a+rot))
			s.Fix(p)
			return p
		}
		a := 0.4 * pitch
		right := make([]*sketch.Point, fitPoints)
		left := make([]*sketch.Point, fitPoints)
		for i := range fitPoints {
			f := float64(i) / float64(fitPoints-1)
			r := innerR + f*(outerR-innerR)
			half := a * (1 - 0.4*f)
			right[i], left[i] = point(r, -half), point(r, half)
		}
		s.CreateLine(left[0], right[0])
		_, err := s.CreateFitSpline(right...)
		require.NoError(t, err)
		s.CreateArc(center, right[fitPoints-1], left[fitPoints-1])
		_, err = s.CreateFitSpline(left...)
		require.NoError(t, err)
	}
	circle := s.CreateCircle(center, rootRadius)
	s.AddConstraint(sketch.NewRadius(circle, rootRadius))
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	return s
}

// TestUnionProfilesCertifiesEmbeddedSplineSections pins the union of every profile of
// a scene whose splines cross the root circle: each flank fragment above the circle
// and each circle fragment between neighbouring regions publishes an exact parameter,
// and the n rotated copies keep the same area.
func TestUnionProfilesCertifiesEmbeddedSplineSections(t *testing.T) {
	cases := []struct{ n, fit int }{{3, 4}, {3, 15}, {8, 15}, {24, 4}, {24, 15}, {128, 15}}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("n=%d/fit=%d", tc.n, tc.fit), func(t *testing.T) {
			if tc.n > 24 && testing.Short() {
				t.Skip("the large scene takes tens of seconds")
			}
			s := embeddedSplineSectionScene(t, tc.n, tc.fit)
			profiles := s.Profiles()
			valid := validProfileIndices(s)
			// n lobes under the circle, n teeth above it and the middle of the disc.
			require.Len(t, valid, 2*tc.n+1)
			u, err := s.UnionProfiles(valid...)
			require.NoError(t, err)
			require.Len(t, u.Outer, 4*tc.n, "each tooth contributes two flanks, a tip arc and a circle fragment")
			partial := 0
			for _, e := range u.Outer {
				if !e.Partial {
					continue
				}
				partial++
				require.Truef(t, e.TExact, "%T partial edge t=[%v %v]", e.Entity, e.TStart, e.TEnd)
			}
			require.Equal(t, 3*tc.n, partial, "two flank fragments and one circle fragment per tooth")

			areas := make([]float64, 0, len(valid))
			sum := 0.0
			for _, i := range valid {
				areas = append(areas, profiles[i].Area)
				sum += profiles[i].Area
			}
			require.InEpsilon(t, sum, u.Area, 1e-9, "the profiles partition the union")
			slices.Sort(areas)
			// The disc's middle is the largest face. The 2n others fall into two groups of
			// n congruent faces, so the n rotated copies of one region have one area each.
			for _, group := range [][]float64{areas[:tc.n], areas[tc.n : 2*tc.n]} {
				for _, a := range group {
					require.InEpsilon(t, group[0], a, 1e-9)
				}
			}
		})
	}
}

// BenchmarkProfilesSplineSections measures Profiles on 24 regions of 15-point splines
// (145 sources with the root arcs).
func BenchmarkProfilesSplineSections(b *testing.B) {
	s := splineSectionScene(b, 24, 15, true)
	b.ResetTimer()
	for range b.N {
		_ = s.Profiles()
	}
}
