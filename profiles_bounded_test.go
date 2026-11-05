package sketch_test

import (
	"context"
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestProfilesLinesBoundedRealSquare(t *testing.T) {
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	points := [4]*sketch.Point{
		s.CreatePoint(0, 0), s.CreatePoint(2, 0),
		s.CreatePoint(2, 2), s.CreatePoint(0, 2),
	}
	for i := range points {
		s.CreateLine(points[i], points[(i+1)%len(points)])
	}
	var charged uint64
	profiles, err := s.ProfilesLinesBounded(t.Context(), func(n uint64) error {
		require.LessOrEqual(t, n, uint64(256))
		charged += n
		return nil
	})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, 4.0, profiles[0].Area)
	require.Len(t, profiles[0].Outer, 4)
	require.True(t, profiles[0].Valid)
	require.Positive(t, charged)
	requireMatchingProfiles(t, profiles, s.Profiles())
}

func TestProfilesLinesBoundedMatchesLineScenes(t *testing.T) {
	scenes := []struct {
		name  string
		loops [][][2]float64
		open  [][2][2]float64
	}{
		{
			name: "crossing loops",
			loops: [][][2]float64{
				{{0, 0}, {2, 0}, {2, 2}, {0, 2}},
				{{1, 1}, {3, 1}, {3, 3}, {1, 3}},
			},
		},
		{
			name: "coincident wall",
			loops: [][][2]float64{
				{{0, 0}, {2, 0}, {2, 2}, {0, 2}},
				{{2, 0}, {4, 0}, {4, 2}, {2, 2}},
			},
		},
		{
			name:  "T junction",
			loops: [][][2]float64{{{0, 0}, {2, 0}, {2, 2}, {0, 2}}},
			open:  [][2][2]float64{{{1, 0}, {1, 1}}},
		},
		{
			name:  "near weld",
			loops: [][][2]float64{{{0, 0}, {2, 0}, {2, 2}, {0, 2}}},
			open:  [][2][2]float64{{{2 + 1e-7, 2}, {3, 3}}},
		},
	}
	for _, tc := range scenes {
		t.Run(tc.name, func(t *testing.T) {
			s := lineScene(t, tc.loops, tc.open)
			want := s.Profiles()
			got, err := s.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
			require.NoError(t, err)
			requireMatchingProfiles(t, got, want)
		})
	}
}

func TestProfilesLinesBoundedCarriesLineJunctions(t *testing.T) {
	s := lineScene(t, [][][2]float64{
		{{-5, -5}, {5, -5}, {5, 5}, {-5, 5}},
		{{3, -2}, {7, -2}, {7, 2}, {3, 2}},
	}, nil)
	bounded, err := s.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
	require.NoError(t, err)
	count := func(profiles []*sketch.Profile) int {
		certified := 0
		for _, profile := range profiles {
			if !profile.Valid {
				continue
			}
			for _, loop := range append([][]sketch.BoundaryEdge{profile.Outer}, profile.Holes...) {
				for i := range loop {
					point, ok := s.CertifyLineLineJunction(loop[i], loop[(i+1)%len(loop)])
					if !ok {
						continue
					}
					require.Equal(t, 5.0, point[0])
					require.Contains(t, []float64{-2, 2}, point[1])
					certified++
				}
			}
		}
		return certified
	}
	got := count(bounded)
	require.Positive(t, got)
	require.Equal(t, count(s.Profiles()), got)
}

func TestProfilesLinesBoundedRefusesAndCancelsWithoutProfiles(t *testing.T) {
	s := lineScene(t, [][][2]float64{{{0, 0}, {2, 0}, {2, 2}, {0, 2}}}, nil)
	var total uint64
	_, err := s.ProfilesLinesBounded(t.Context(), func(n uint64) error {
		total += n
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, total, uint64(128))

	wantErr := errors.New("shared budget spent")
	got, err := s.ProfilesLinesBounded(t.Context(), func(uint64) error { return wantErr })
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, got)
	var failedAt uint64
	got, err = s.ProfilesLinesBounded(t.Context(), func(n uint64) error {
		if failedAt+n > total/2 {
			return wantErr
		}
		failedAt += n
		return nil
	})
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, got)
	require.Greater(t, failedAt, uint64(64))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var spent uint64
	got, err = s.ProfilesLinesBounded(ctx, func(n uint64) error {
		spent += n
		if spent > total-16 {
			cancel()
		}
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)
	require.Greater(t, spent, total-16)
	preCanceled, stop := context.WithCancel(t.Context())
	stop()
	got, err = s.ProfilesLinesBounded(preCanceled, func(uint64) error {
		t.Fatal("canceled call charged work")
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)

	var noContext context.Context // exercise the documented nil-context refusal
	got, err = s.ProfilesLinesBounded(noContext, func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)
	got, err = s.ProfilesLinesBounded(t.Context(), nil)
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)
}

func TestProfilesLinesBoundedNestedHoleAndLateRefusal(t *testing.T) {
	s := lineScene(t, [][][2]float64{
		{{0, 0}, {8, 0}, {8, 8}, {0, 8}},
		{{2, 2}, {2, 6}, {6, 6}, {6, 2}},
	}, nil)
	var total uint64
	got, err := s.ProfilesLinesBounded(t.Context(), func(n uint64) error {
		require.LessOrEqual(t, n, uint64(256))
		total += n
		return nil
	})
	require.NoError(t, err)
	requireMatchingProfiles(t, got, s.Profiles())
	require.Greater(t, total, uint64(128))
	refused := errors.New("late work refusal")
	var spent uint64
	got, err = s.ProfilesLinesBounded(t.Context(), func(n uint64) error {
		if n > total-16-spent {
			return refused
		}
		spent += n
		return nil
	})
	require.ErrorIs(t, err, refused)
	require.Nil(t, got)
	require.GreaterOrEqual(t, spent*20, total*19)
}

func TestProfilesLinesBoundedSceneAdmission(t *testing.T) {
	var absent *sketch.Sketch
	got, err := absent.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)

	s := lineScene(t, nil, nil)
	for i := 0; i < 129; i++ {
		s.CreatePoint(float64(i), 0)
	}
	got, err = s.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)

	s = lineScene(t, nil, nil)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(1, 0)
	construction := s.CreateLine(a, b)
	construction.SetConstruction(true)
	got, err = s.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)

	s = lineScene(t, nil, nil)
	p := s.CreatePoint(0, 0)
	s.CreateCircle(p, 1)
	got, err = s.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)

	s = lineScene(t, nil, nil)
	p = s.CreatePoint(math.NaN(), 0)
	s.CreateLine(p, s.CreatePoint(1, 0))
	got, err = s.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)

	s = lineScene(t, nil, nil)
	other := lineScene(t, nil, nil)
	s.CreateLine(s.CreatePoint(0, 0), other.CreatePoint(1, 0))
	got, err = s.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)

	s = lineScene(t, nil, nil)
	a = s.CreatePoint(0, 0)
	b = s.CreatePoint(1, 0)
	for i := 0; i < 65; i++ {
		s.CreateLine(a, b)
	}
	got, err = s.ProfilesLinesBounded(t.Context(), func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got)
}

func TestProfilesLinesBoundedSixtyFourLineLoop(t *testing.T) {
	loop := make([][2]float64, 64)
	for i := range loop {
		angle := 2 * math.Pi * float64(i) / float64(len(loop))
		loop[i] = [2]float64{10 * math.Cos(angle), 10 * math.Sin(angle)}
	}
	s := lineScene(t, [][][2]float64{loop}, nil)
	want := s.Profiles()
	var spent uint64
	budgetErr := errors.New("65,536 work units spent")
	got, err := s.ProfilesLinesBounded(t.Context(), func(n uint64) error {
		if n > 65536-spent {
			return budgetErr
		}
		spent += n
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.True(t, got[0].Valid)
	requireMatchingProfiles(t, got, want)
	require.LessOrEqual(t, spent, uint64(65536))
}

func lineScene(t *testing.T, loops [][][2]float64, open [][2][2]float64) *sketch.Sketch {
	t.Helper()
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	for _, loop := range loops {
		points := make([]*sketch.Point, len(loop))
		for i, xy := range loop {
			points[i] = s.CreatePoint(xy[0], xy[1])
		}
		for i := range points {
			s.CreateLine(points[i], points[(i+1)%len(points)])
		}
	}
	for _, segment := range open {
		a := s.CreatePoint(segment[0][0], segment[0][1])
		b := s.CreatePoint(segment[1][0], segment[1][1])
		s.CreateLine(a, b)
	}
	return s
}

func requireMatchingProfiles(t *testing.T, got, want []*sketch.Profile) {
	t.Helper()
	require.Equal(t, len(want), len(got))
	for i := range want {
		g, w := got[i], want[i]
		require.Equal(t, w.Area, g.Area)
		require.Equal(t, w.Valid, g.Valid)
		require.Equal(t, w.SelfIntersecting, g.SelfIntersecting)
		require.Equal(t, w.Revision(), g.Revision())
		require.True(t, w.Sketch() == g.Sketch())
		require.Equal(t, len(w.Entities), len(g.Entities))
		for j := range w.Entities {
			require.True(t, w.Entities[j] == g.Entities[j])
		}
		requireMatchingEdges(t, g.Outer, w.Outer)
		require.Equal(t, len(w.Holes), len(g.Holes))
		for j := range w.Holes {
			requireMatchingEdges(t, g.Holes[j], w.Holes[j])
		}
	}
}

func requireMatchingEdges(t *testing.T, got, want []sketch.BoundaryEdge) {
	t.Helper()
	require.Equal(t, len(want), len(got))
	for i := range want {
		g, w := got[i], want[i]
		require.True(t, w.Entity == g.Entity)
		require.Equal(t, w.Partial, g.Partial)
		require.Equal(t, w.Reversed, g.Reversed)
		require.Equal(t, w.TStart, g.TStart)
		require.Equal(t, w.TEnd, g.TEnd)
		require.Equal(t, w.TExact, g.TExact)
		require.Equal(t, len(w.Polyline), len(g.Polyline))
		for j := range w.Polyline {
			for k := range w.Polyline[j] {
				require.True(t, math.Float64bits(w.Polyline[j][k]) == math.Float64bits(g.Polyline[j][k]))
			}
		}
	}
}

func twoRectangleCapScene(t *testing.T, prism, loft [4]float64) *sketch.Sketch {
	t.Helper()
	s := lineScene(t, [][][2]float64{
		{{prism[0], prism[1]}, {prism[2], prism[1]},
			{prism[2], prism[3]}, {prism[0], prism[3]}},
		{{loft[0], loft[1]}, {loft[2], loft[1]},
			{loft[2], loft[3]}, {loft[0], loft[3]}},
	}, nil)
	for _, p := range s.Points() {
		s.Fix(p)
	}
	return s
}

func twoRectangleCapScenes(t *testing.T) [2]*sketch.Sketch {
	t.Helper()
	prism := [4]float64{0, 0, 10, 10}
	return [2]*sketch.Sketch{
		twoRectangleCapScene(t, prism, [4]float64{8, 4, 12, 6}),
		twoRectangleCapScene(t, prism, [4]float64{8, 4, 13, 6}),
	}
}

func TestProfilesTwoRectangleCapsBoundedMatchesLiveProfiles(t *testing.T) {
	scenes := twoRectangleCapScenes(t)
	var charged uint64
	paired, err := sketch.ProfilesTwoRectangleCapsBounded(t.Context(), scenes, func(n uint64) error {
		require.LessOrEqual(t, n, uint64(256))
		charged += n
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, charged)
	require.Less(t, charged, uint64(65_536))
	for i, s := range scenes {
		require.Len(t, paired[i], 3)
		requireSameTwoRectangleProfiles(t, paired[i], s.Profiles())
		junctions := map[float64]bool{}
		for _, profile := range paired[i] {
			require.True(t, profile.Sketch() == s)
			require.Equal(t, s.Revision(), profile.Revision())
			require.False(t, profile.IsStale())
			for j, left := range profile.Outer {
				right := profile.Outer[(j+1)%len(profile.Outer)]
				point, ok := s.CertifyLineLineJunction(left, right)
				if !ok {
					continue
				}
				require.Equal(t, 10.0, point[0])
				require.Contains(t, []float64{4, 6}, point[1])
				junctions[point[1]] = true
			}
		}
		require.True(t, junctions[4])
		require.True(t, junctions[6])
		s.Points()[0].MoveTo(-1, 0)
		require.True(t, paired[i][0].IsStale())
	}
}

func TestProfilesTwoRectangleCapsBoundedRefusesUnsupportedScenes(t *testing.T) {
	check := func(t *testing.T, scenes [2]*sketch.Sketch) {
		t.Helper()
		got, err := sketch.ProfilesTwoRectangleCapsBounded(t.Context(), scenes,
			func(uint64) error { return nil })
		require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
		require.Nil(t, got[0])
		require.Nil(t, got[1])
	}
	t.Run("wrong count", func(t *testing.T) {
		scenes := twoRectangleCapScenes(t)
		points := scenes[0].Points()
		scenes[0].CreateLine(points[0], points[1])
		check(t, scenes)
	})
	t.Run("wrong kind", func(t *testing.T) {
		scenes := twoRectangleCapScenes(t)
		require.True(t, scenes[0].RemoveEntity(scenes[0].Entities()[7]))
		scenes[0].CreateCircle(scenes[0].Points()[4], 1)
		require.Len(t, scenes[0].Entities(), 8)
		check(t, scenes)
	})
	t.Run("ungrounded point", func(t *testing.T) {
		scenes := twoRectangleCapScenes(t)
		scenes[0].Unfix(scenes[0].Points()[0])
		check(t, scenes)
	})
	t.Run("nonfinite point", func(t *testing.T) {
		scenes := twoRectangleCapScenes(t)
		scenes[0].Points()[0].MoveTo(math.NaN(), 0)
		check(t, scenes)
	})
	t.Run("oversized point", func(t *testing.T) {
		scenes := twoRectangleCapScenes(t)
		scenes[0].Points()[0].MoveTo(1e7, 0)
		check(t, scenes)
	})
	t.Run("pair mismatch", func(t *testing.T) {
		scenes := twoRectangleCapScenes(t)
		scenes[1] = twoRectangleCapScene(t, [4]float64{0, 0, 10, 10},
			[4]float64{7, 4, 13, 6})
		check(t, scenes)
	})
	t.Run("same scene", func(t *testing.T) {
		scenes := twoRectangleCapScenes(t)
		scenes[1] = scenes[0]
		check(t, scenes)
	})
}

func TestProfilesTwoRectangleCapsBoundedBudgetAndCancellation(t *testing.T) {
	scenes := twoRectangleCapScenes(t)
	var total uint64
	_, err := sketch.ProfilesTwoRectangleCapsBounded(t.Context(), scenes, func(n uint64) error {
		total += n
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, total, uint64(256))
	for _, reserve := range []uint64{16, 104} {
		spent := uint64(0)
		budgetErr := errors.New("caller budget exhausted")
		got, callErr := sketch.ProfilesTwoRectangleCapsBounded(t.Context(), scenes, func(n uint64) error {
			if n == reserve {
				return budgetErr
			}
			spent += n
			return nil
		})
		require.ErrorIs(t, callErr, budgetErr)
		require.Nil(t, got[0])
		require.Nil(t, got[1])
		require.Less(t, spent, total)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var spent uint64
	got, err := sketch.ProfilesTwoRectangleCapsBounded(ctx, scenes, func(n uint64) error {
		if spent+n > total-8 {
			cancel()
			return ctx.Err()
		}
		spent += n
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got[0])
	require.Nil(t, got[1])
	require.Greater(t, spent*10, total*9)
}

func TestProfilesTwoRectangleCapsBoundedFiniteExtremes(t *testing.T) {
	prism := [4]float64{0, 0, 524288, 524288}
	scenes := [2]*sketch.Sketch{
		twoRectangleCapScene(t, prism, [4]float64{262144, 131072, 786432, 393216}),
		twoRectangleCapScene(t, prism, [4]float64{262144, 131072, 786432, 393216}),
	}
	paired, err := sketch.ProfilesTwoRectangleCapsBounded(t.Context(), scenes,
		func(uint64) error { return nil })
	require.NoError(t, err)
	for i, s := range scenes {
		requireSameTwoRectangleProfiles(t, paired[i], s.Profiles())
	}
	// The finite input floor is admitted by the coordinate screen. This
	// particular scene may still refuse later endpoint reconstruction.
	tiny := math.Ldexp(1, -256)
	nearFloor := [4]float64{tiny, 0, 524288, 524288}
	scenes = [2]*sketch.Sketch{
		twoRectangleCapScene(t, nearFloor, [4]float64{262144, 131072, 786432, 393216}),
		twoRectangleCapScene(t, nearFloor, [4]float64{262144, 131072, 917504, 393216}),
	}
	got, err := sketch.ProfilesTwoRectangleCapsBounded(t.Context(), scenes,
		func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got[0])
	require.Nil(t, got[1])
}

func requireSameTwoRectangleProfiles(t *testing.T, got, want []*sketch.Profile) {
	t.Helper()
	require.Len(t, got, len(want))
	used := make([]bool, len(want))
	for _, profile := range got {
		found := false
		for i, ordinary := range want {
			if used[i] || profile.Area != ordinary.Area || profile.Valid != ordinary.Valid ||
				profile.SelfIntersecting != ordinary.SelfIntersecting ||
				profile.Revision() != ordinary.Revision() ||
				profile.Sketch() != ordinary.Sketch() ||
				len(profile.Holes) != len(ordinary.Holes) ||
				!sameTwoRectangleCycle(profile.Outer, ordinary.Outer) {
				continue
			}
			used[i], found = true, true
			break
		}
		require.True(t, found, "bounded profile has no matching ordinary directed boundary")
	}
}

func sameTwoRectangleCycle(a, b []sketch.BoundaryEdge) bool {
	if len(a) != len(b) {
		return false
	}
	for start := range b {
		matched := true
		for i := range a {
			if !sameTwoRectangleEdge(a[i], b[(start+i)%len(b)]) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func sameTwoRectangleEdge(a, b sketch.BoundaryEdge) bool {
	if a.Entity != b.Entity || a.Partial != b.Partial || a.Reversed != b.Reversed ||
		a.TStart != b.TStart || a.TEnd != b.TEnd || a.TExact != b.TExact ||
		len(a.Polyline) != len(b.Polyline) {
		return false
	}
	for i, point := range a.Polyline {
		for axis, coordinate := range point {
			if math.Float64bits(coordinate) != math.Float64bits(b.Polyline[i][axis]) {
				return false
			}
		}
	}
	return true
}

func convexCapScenes(t *testing.T) [2]*sketch.Sketch {
	t.Helper()
	prism := [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	lofts := [2][][2]float64{
		{{8, 4}, {12, 4}, {13, 6}, {8, 6}},
		{{8, 4}, {13, 4}, {14, 6}, {8, 6}},
	}
	var scenes [2]*sketch.Sketch
	for i := range scenes {
		scenes[i] = lineScene(t, [][][2]float64{prism, lofts[i]}, nil)
	}
	return scenes
}

func TestProfilesTwoLineCapsBoundedRefusalAndCancellation(t *testing.T) {
	scenes := convexCapScenes(t)
	var total uint64
	_, err := sketch.ProfilesTwoLineCapsBounded(t.Context(), scenes, func(n uint64) error {
		total += n
		if total > 4096 {
			t.Fatal("paired cap publication exceeded the 4,096 work wall")
		}
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, total, uint64(128))

	refused := errors.New("late paired work refusal")
	var spent uint64
	got, err := sketch.ProfilesTwoLineCapsBounded(t.Context(), scenes, func(n uint64) error {
		if spent+n > total-16 {
			return refused
		}
		spent += n
		return nil
	})
	require.ErrorIs(t, err, refused)
	require.Nil(t, got[0])
	require.Nil(t, got[1])
	require.Greater(t, spent, total/2)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	spent = 0
	got, err = sketch.ProfilesTwoLineCapsBounded(ctx, scenes, func(n uint64) error {
		spent += n
		if spent > total-16 {
			cancel()
		}
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got[0])
	require.Nil(t, got[1])
	require.Greater(t, spent, total-16)
}

func TestProfilesTwoLineCapsBoundedUsesGeneralArrangerForOtherScenes(t *testing.T) {
	loop := [][][2]float64{{{0, 0}, {2, 0}, {2, 2}, {0, 2}}}
	scenes := [2]*sketch.Sketch{lineScene(t, loop, nil), lineScene(t, loop, nil)}
	var spent uint64
	profiles, err := sketch.ProfilesTwoLineCapsBounded(t.Context(), scenes, func(n uint64) error {
		spent += n
		if spent > 4096 {
			t.Fatal("general square pair exceeded the focused work wall")
		}
		return nil
	})
	require.NoError(t, err)
	for i, cap := range profiles {
		require.Len(t, cap, 1)
		require.Equal(t, 4.0, cap[0].Area)
		require.Same(t, scenes[i], cap[0].Sketch())
	}
}

func TestProfilesTwoLineCapsBoundedConvexTrapezoids(t *testing.T) {
	scenes := convexCapScenes(t)
	var spent uint64
	profiles, err := sketch.ProfilesTwoLineCapsBounded(t.Context(), scenes, func(n uint64) error {
		require.LessOrEqual(t, n, uint64(256))
		spent += n
		if spent > 4096 {
			t.Fatal("paired cap publication exceeded the 4,096 work wall")
		}
		return nil
	})
	require.NoError(t, err)
	require.LessOrEqual(t, spent, uint64(4096))
	for i, cap := range profiles {
		require.Len(t, cap, 3)
		want := [2][3]float64{{96, 4, 5}, {96, 4, 7}}[i]
		junctions := map[[2]float64]struct{}{}
		for j, region := range cap {
			require.True(t, region.Valid)
			require.Equal(t, want[j], region.Area)
			require.Same(t, scenes[i], region.Sketch())
			require.False(t, region.IsStale())
			require.Empty(t, region.Holes)
			for k, edge := range region.Outer {
				require.True(t, edge.TExact)
				require.Len(t, edge.Polyline, 2)
				next := region.Outer[(k+1)%len(region.Outer)]
				point, ok := scenes[i].CertifyLineLineJunction(edge, next)
				if ok {
					junctions[point] = struct{}{}
				}
			}
		}
		require.Contains(t, junctions, [2]float64{10, 4})
		require.Contains(t, junctions, [2]float64{10, 6})
	}
	t.Logf("paired cap publication charged %d visits", spent)
}

func fmaCapRectangle(x0, y0, x1, y1 float64) [][2]float64 {
	return [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}

func fmaCapSeparate(a, b, c float64) float64 {
	// Round the exact product before adding, even when the compiler can fuse
	// a source expression into a single multiply-add instruction.
	product := new(big.Rat).Mul(new(big.Rat).SetFloat64(a),
		new(big.Rat).SetFloat64(b))
	roundedProduct, _ := product.Float64()
	sum := new(big.Rat).Add(new(big.Rat).SetFloat64(roundedProduct),
		new(big.Rat).SetFloat64(c))
	roundedSum, _ := sum.Float64()
	return roundedSum
}

func TestProfilesTwoLineCapsBoundedFMAProducers(t *testing.T) {
	eps := math.Ldexp(1, -52)
	nearEnd := eps / (1 + 2*eps)
	if math.FMA(3.0/5.0, 5, -3) == 0 ||
		math.FMA(nearEnd, 1+2*eps, -eps) == 0 ||
		fmaCapSeparate(nearEnd, 1+2*eps, -eps) != 0 {
		t.Fatal("fixtures do not distinguish fused and separate endpoint evaluation")
	}
	cases := []struct {
		name  string
		prism [4]float64
		lofts [2][4]float64
		cuts  [2][2][2]float64
	}{
		{
			name:  "left_x",
			prism: [4]float64{0, 0, 10, 10},
			lofts: [2][4]float64{{-3, 3, 2, 7}, {-3, 2, 2, 8}},
			cuts:  [2][2][2]float64{{{0, 3}, {0, 7}}, {{0, 2}, {0, 8}}},
		},
		{
			name:  "translated_524",
			prism: [4]float64{0, -eps, 120, 1 + eps},
			lofts: [2][4]float64{
				{119, 0, 163 - 896*eps, 1},
				{119, 0, 165 - 1024*eps, 1},
			},
			cuts: [2][2][2]float64{{{120, 0}, {120, 1}}, {{120, 0}, {120, 1}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var scenes [2]*sketch.Sketch
			for cap := range scenes {
				l := tc.lofts[cap]
				scenes[cap] = lineScene(t, [][][2]float64{
					fmaCapRectangle(tc.prism[0], tc.prism[1], tc.prism[2], tc.prism[3]),
					fmaCapRectangle(l[0], l[1], l[2], l[3]),
				}, nil)
			}
			var charged uint64
			profiles, err := sketch.ProfilesTwoLineCapsBounded(t.Context(), scenes,
				func(n uint64) error {
					if charged+n > 4096 {
						t.Fatalf("shared source work exceeds 4096")
					}
					charged += n
					return nil
				})
			if err != nil {
				t.Fatalf("real paired producer: %v, charged=%d", err, charged)
			}
			t.Logf("paired producer charged %d of 4096 steps", charged)
			for cap, found := range profiles {
				if len(found) != 3 {
					t.Fatalf("cap %d: profiles=%d", cap, len(found))
				}
				seen := map[[2]float64]bool{}
				for _, p := range found {
					if !p.Valid || len(p.Holes) != 0 {
						t.Fatalf("cap %d: invalid or holed profile", cap)
					}
					for edge := range p.Outer {
						current := p.Outer[edge]
						if !current.TExact {
							t.Fatalf("cap %d: inexact source edge", cap)
						}
						next := p.Outer[(edge+1)%len(p.Outer)]
						point, ok := scenes[cap].CertifyLineLineJunction(current, next)
						if ok {
							seen[point] = true
						}
					}
				}
				for _, point := range tc.cuts[cap] {
					if !seen[point] {
						t.Fatalf("cap %d: missing certified cut (%g,%g)",
							cap, point[0], point[1])
					}
				}
			}
		})
	}
}
