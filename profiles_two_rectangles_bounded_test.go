package sketch_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

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
