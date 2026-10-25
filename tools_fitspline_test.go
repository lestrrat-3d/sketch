package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// lobePoint is the i-th of seven points (i = 0..6) of one lobe of a wavy ring of
// radius 40, spanning one angular step starting at angle base. The first and
// last radii are equal, so lobe k+1's first point lands where lobe k's last one
// does and the lobes close into a ring.
func lobePoint(centerX, centerY, base, step float64, i int) (float64, float64) {
	r := 40 + 3*math.Sin(2*math.Pi*float64(i)/6)
	a := base + step*float64(i)/6
	return centerX + r*math.Cos(a), centerY + r*math.Sin(a)
}

// buildPatternedRing draws ONE lobe as three fit splines, dimensions its first
// six points from the center, patterns it n times about the center, and closes
// the ring with the single coincident the pattern needs. center is nil for the
// sketch origin.
func buildPatternedRing(t *testing.T, cx, cy float64, n int) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	var center *sketch.Point
	if cx == 0 && cy == 0 {
		center = s.Origin()
	} else {
		center = s.CreatePoint(cx, cy)
		s.AddConstraint(
			sketch.NewHorizontalDistance(s.Origin(), center, cx),
			sketch.NewVerticalDistance(s.Origin(), center, cy),
		)
	}
	step := 2 * math.Pi / float64(n)
	pts := make([]*sketch.Point, 7)
	for i := range pts {
		x, y := lobePoint(cx, cy, 0, step, i)
		pts[i] = s.CreatePoint(x, y)
	}
	// The last point is left undimensioned: the closing coincident places it.
	for _, p := range pts[:6] {
		s.AddConstraint(
			sketch.NewHorizontalDistance(center, p, p.X()-cx),
			sketch.NewVerticalDistance(center, p, p.Y()-cy),
		)
	}
	var seed []sketch.Entity
	for _, run := range [][]*sketch.Point{pts[0:3], pts[2:5], pts[4:7]} {
		fs, err := s.CreateFitSpline(run...)
		require.NoError(t, err)
		seed = append(seed, fs)
	}
	pat, err := sketch.CreatePatternCircular(s, seed, center, n)
	require.NoError(t, err)
	require.Len(t, pat.Instances, 3*(n-1), "three fit splines per copy")

	// Copy 1's first point is the third entity's... find it through the copy's
	// first fit spline.
	firstCopy, ok := pat.Instances[0].(*sketch.FitSpline)
	require.True(t, ok)
	s.AddConstraint(sketch.NewCoincident(pts[6], firstCopy.Fit[0]))

	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	return s
}

// referenceRing draws the same ring point by point, with every lobe's points
// authored directly and neighbouring lobes sharing their joint.
func referenceRing(t *testing.T, cx, cy float64, n int) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	step := 2 * math.Pi / float64(n)
	var first *sketch.Point
	var joint *sketch.Point
	for k := 0; k < n; k++ {
		pts := make([]*sketch.Point, 7)
		for i := range pts {
			switch {
			case i == 0 && joint != nil:
				pts[i] = joint
			case i == 6 && k == n-1:
				pts[i] = first
			default:
				x, y := lobePoint(cx, cy, step*float64(k), step, i)
				pts[i] = s.CreatePoint(x, y)
			}
		}
		if k == 0 {
			first = pts[0]
		}
		joint = pts[6]
		for _, run := range [][]*sketch.Point{pts[0:3], pts[2:5], pts[4:7]} {
			_, err := s.CreateFitSpline(run...)
			require.NoError(t, err)
		}
	}
	return s
}

func TestPatternCircularCopiesFitSplines(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cx, cy float64
		n      int
	}{
		{"off-origin center, 15 lobes", 30, 20, 15},
		{"origin center, 48 lobes", 0, 0, 48},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := buildPatternedRing(t, tc.cx, tc.cy, tc.n)

			rep := s.Verify(t.Context())
			require.Equal(t, sketch.FullyConstrained, rep.Status)
			require.Zero(t, rep.DOF)
			require.Empty(t, rep.Redundant, "no redundant constraint")
			require.Empty(t, rep.Conflicts)

			profiles := s.Profiles()
			require.Len(t, profiles, 1, "one closed ring")
			require.True(t, profiles[0].Valid)

			ref := referenceRing(t, tc.cx, tc.cy, tc.n).Profiles()
			require.Len(t, ref, 1)
			require.InDelta(t, ref[0].Area, profiles[0].Area, 1e-6*ref[0].Area,
				"the patterned ring encloses the same area as the one drawn point by point")
		})
	}
}

// TestPatternCopiesFitSplineShape pins that a copy is the spline THROUGH the
// transformed points — the same curve rotated, reflected or translated — by
// comparing sampled points, and that the construction flag is copied.
func TestPatternCopiesFitSplineShape(t *testing.T) {
	build := func(t *testing.T) (*sketch.Sketch, *sketch.FitSpline) {
		t.Helper()
		s := newSketch(t)
		fit := []*sketch.Point{
			s.CreatePoint(10, 0), s.CreatePoint(14, 3), s.CreatePoint(19, 2), s.CreatePoint(24, 7), s.CreatePoint(30, 4),
		}
		fs, err := s.CreateFitSpline(fit...)
		require.NoError(t, err)
		fs.SetConstruction(true)
		return s, fs
	}

	t.Run("circular", func(t *testing.T) {
		s, seed := build(t)
		pat, err := sketch.CreatePatternCircular(s, []sketch.Entity{seed}, s.Origin(), 5)
		require.NoError(t, err)
		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		for k, e := range pat.Instances {
			cp := e.(*sketch.FitSpline)
			require.True(t, cp.IsConstruction())
			ang := 2 * math.Pi * float64(k+1) / 5
			want, got := seed.Polyline(40), cp.Polyline(40)
			require.Len(t, got, len(want))
			for i := range want {
				wx := want[i][0]*math.Cos(ang) - want[i][1]*math.Sin(ang)
				wy := want[i][0]*math.Sin(ang) + want[i][1]*math.Cos(ang)
				require.InDelta(t, wx, got[i][0], 1e-7)
				require.InDelta(t, wy, got[i][1], 1e-7)
			}
		}
	})

	t.Run("rect", func(t *testing.T) {
		s, seed := build(t)
		pat, err := sketch.CreatePatternRect(s, []sketch.Entity{seed}, 3, 1, 50, 0)
		require.NoError(t, err)
		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		for k, e := range pat.Instances {
			cp := e.(*sketch.FitSpline)
			want, got := seed.Polyline(40), cp.Polyline(40)
			for i := range want {
				require.InDelta(t, want[i][0]+50*float64(k+1), got[i][0], 1e-7)
				require.InDelta(t, want[i][1], got[i][1], 1e-7)
			}
		}
	})

	t.Run("mirror", func(t *testing.T) {
		s, seed := build(t)
		axis := s.CreateLine(s.CreatePoint(0, -10), s.CreatePoint(0, 10))
		m, err := sketch.CreateMirror(s, []sketch.Entity{seed}, axis)
		require.NoError(t, err)
		require.NotNil(t, m)
		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		cp := m.Copies[0].(*sketch.FitSpline)
		want, got := seed.Polyline(40), cp.Polyline(40)
		for i := range want {
			require.InDelta(t, -want[i][0], got[i][0], 1e-7)
			require.InDelta(t, want[i][1], got[i][1], 1e-7)
		}
	})
}

// TestPatternSharedFitPointsShareCopies pins that two fit splines sharing a fit
// point copy to two fit splines sharing the copied point.
func TestPatternSharedFitPointsShareCopies(t *testing.T) {
	s := newSketch(t)
	a, b, c, d, e := s.CreatePoint(10, 0), s.CreatePoint(12, 3), s.CreatePoint(15, 4), s.CreatePoint(18, 3), s.CreatePoint(20, 0)
	f1, err := s.CreateFitSpline(a, b, c)
	require.NoError(t, err)
	f2, err := s.CreateFitSpline(c, d, e)
	require.NoError(t, err)
	pat, err := sketch.CreatePatternCircular(s, []sketch.Entity{f1, f2}, s.Origin(), 3)
	require.NoError(t, err)
	require.Len(t, pat.Instances, 4)
	c1 := pat.Instances[0].(*sketch.FitSpline)
	c2 := pat.Instances[1].(*sketch.FitSpline)
	require.Same(t, c1.Fit[2], c2.Fit[0], "the shared fit point is copied once")
}

// TestPatternFitSplineJSONRoundTrip pins that a patterned ring reloads as the
// same sketch. A pattern leaves no record of its own: its copies are ordinary
// fit splines, points, construction spokes and constraints, all of which
// already serialize.
func TestPatternFitSplineJSONRoundTrip(t *testing.T) {
	s := buildPatternedRing(t, 30, 20, 15)
	want := s.Profiles()
	require.Len(t, want, 1)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), len(s.Entities()))
	require.Len(t, s2.Constraints(), len(s.Constraints()))

	rep := s2.Verify(t.Context())
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.Zero(t, rep.DOF)
	got := s2.Profiles()
	require.Len(t, got, 1)
	require.InDelta(t, want[0].Area, got[0].Area, 1e-9*want[0].Area)
}

// buildSharedRing is buildPatternedRing closed by CreatePatternCircularRing: the
// seed's last point is the junction, and no coincident is added.
func buildSharedRing(t *testing.T, cx, cy float64, n int) (*sketch.Sketch, *sketch.Pattern) {
	t.Helper()
	s := newSketch(t)
	var center *sketch.Point
	if cx == 0 && cy == 0 {
		center = s.Origin()
	} else {
		center = s.CreatePoint(cx, cy)
		s.AddConstraint(
			sketch.NewHorizontalDistance(s.Origin(), center, cx),
			sketch.NewVerticalDistance(s.Origin(), center, cy),
		)
	}
	step := 2 * math.Pi / float64(n)
	pts := make([]*sketch.Point, 7)
	for i := range pts {
		x, y := lobePoint(cx, cy, 0, step, i)
		pts[i] = s.CreatePoint(x, y)
	}
	for _, p := range pts[:6] {
		s.AddConstraint(
			sketch.NewHorizontalDistance(center, p, p.X()-cx),
			sketch.NewVerticalDistance(center, p, p.Y()-cy),
		)
	}
	var seed []sketch.Entity
	for _, run := range [][]*sketch.Point{pts[0:3], pts[2:5], pts[4:7]} {
		fs, err := s.CreateFitSpline(run...)
		require.NoError(t, err)
		seed = append(seed, fs)
	}
	pat, err := sketch.CreatePatternCircularRing(s, seed, center, n, pts[0], pts[6])
	require.NoError(t, err)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	return s, pat
}

func TestPatternCircularRingSharesJunctions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cx, cy float64
		n      int
	}{
		{"off-origin center, 15 lobes", 30, 20, 15},
		{"origin center, 48 lobes", 0, 0, 48},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, pat := buildSharedRing(t, tc.cx, tc.cy, tc.n)
			require.Len(t, pat.Instances, 3*(tc.n-1))

			// Every junction is one shared *Point, around the whole ring.
			var splines []*sketch.FitSpline
			for _, e := range s.Entities() {
				if fs, ok := e.(*sketch.FitSpline); ok {
					splines = append(splines, fs)
				}
			}
			require.Len(t, splines, 3*tc.n)
			for i, fs := range splines {
				next := splines[(i+1)%len(splines)]
				require.Same(t, fs.Fit[len(fs.Fit)-1], next.Fit[0], "spline %d ends where spline %d starts", i, (i+1)%len(splines))
			}

			// No orphan point: every point is a fit point of some spline, or the
			// ring's center (the sketch origin is not among the sketch's points).
			used := map[*sketch.Point]struct{}{}
			for _, fs := range splines {
				for _, p := range fs.Fit {
					used[p] = struct{}{}
				}
			}
			for _, p := range s.Points() {
				_, isFit := used[p]
				require.True(t, isFit || (tc.cx != 0 && p.X() == tc.cx && p.Y() == tc.cy), "orphan point %d", p.ID())
			}
			rep := s.Verify(t.Context())
			require.Equal(t, sketch.FullyConstrained, rep.Status)
			require.Zero(t, rep.DOF)
			require.Empty(t, rep.Redundant)
			require.Empty(t, rep.Conflicts)

			profiles := s.Profiles()
			require.Len(t, profiles, 1)
			require.True(t, profiles[0].Valid)
			outer := profiles[0].Outer
			for i, edge := range outer {
				require.True(t, edge.TExact, "edge %d is trimmed at an exact parameter", i)
				next := outer[(i+1)%len(outer)]
				last := edge.Polyline[len(edge.Polyline)-1]
				require.Equal(t, last, next.Polyline[0], "edge %d closes exactly onto edge %d", i, (i+1)%len(outer))
			}

			ref := referenceRing(t, tc.cx, tc.cy, tc.n).Profiles()
			require.Len(t, ref, 1)
			require.InDelta(t, ref[0].Area, profiles[0].Area, 1e-6*ref[0].Area)
		})
	}
}

func TestPatternCircularRingRefusals(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	a, b, c := s.CreatePoint(10, 0), s.CreatePoint(12, 2), s.CreatePoint(10, 4)
	fs, err := s.CreateFitSpline(a, b)
	require.NoError(t, err)
	seed := []sketch.Entity{fs}
	before := len(s.Entities())

	_, err = sketch.CreatePatternCircularRing(s, seed, center, 4, a, a)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "start == end")
	_, err = sketch.CreatePatternCircularRing(s, seed, center, 4, a, c)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "end defines no seed entity")
	_, err = sketch.CreatePatternCircularRing(s, seed, center, 4, a, center)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "end is the center")
	_, err = sketch.CreatePatternCircularRing(s, seed, center, 1, a, b)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "n < 2")
	require.Len(t, s.Entities(), before, "a refusal commits nothing")

	other := newSketch(t)
	_, err = sketch.CreatePatternCircularRing(s, seed, center, 4, a, other.CreatePoint(1, 1))
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
}
