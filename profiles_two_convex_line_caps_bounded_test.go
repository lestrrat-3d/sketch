package sketch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

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
