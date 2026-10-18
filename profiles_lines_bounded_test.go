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
