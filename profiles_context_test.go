package sketch_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

var errProfileBudgetTestRefused = errors.New("profile budget refused")

type profileBudget struct {
	limits   [3]uint64
	totals   [3]uint64
	onCharge func(work, records, bytes uint64) error
}

func (b *profileBudget) Charge(work, records, bytes uint64) error {
	values := [3]uint64{work, records, bytes}
	for i, value := range values {
		if value > b.limits[i]-b.totals[i] {
			return errProfileBudgetTestRefused
		}
	}
	if b.onCharge != nil {
		if err := b.onCharge(work, records, bytes); err != nil {
			return err
		}
	}
	for i, value := range values {
		b.totals[i] += value
	}
	return nil
}

func TestProfilesContextChargesSharedBudget(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(0, 0, 20, 12)

	measured := &profileBudget{limits: [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64}}
	profiles, err := s.ProfilesContext(t.Context(), measured)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Positive(t, measured.totals[0])
	require.Positive(t, measured.totals[1])
	require.Positive(t, measured.totals[2])

	for i, total := range measured.totals {
		limits := [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64}
		limits[i] = total
		got, err := s.ProfilesContext(t.Context(), &profileBudget{limits: limits})
		require.NoError(t, err)
		require.Len(t, got, 1)

		limits[i] = total - 1
		got, err = s.ProfilesContext(t.Context(), &profileBudget{limits: limits})
		require.ErrorIs(t, err, errProfileBudgetTestRefused)
		require.Nil(t, got)
	}
}

func TestProfilesContextHandlesGroupReplayBudget(t *testing.T) {
	s := newSketch(t)
	for i := range 32 {
		angle := -0.9 + 1.8*float64(i)/31
		offset := -30 + 60*float64(i)/31
		dx, dy := math.Cos(angle), math.Sin(angle)
		nx, ny := -dy, dx
		start := s.CreatePoint(nx*offset-100*dx, ny*offset-100*dy)
		end := s.CreatePoint(nx*offset+100*dx, ny*offset+100*dy)
		s.CreateLine(start, end)
	}
	budget := &profileBudget{limits: [3]uint64{1 << 22, 1 << 16, 1 << 26}}
	profiles, err := s.ProfilesContext(t.Context(), budget)
	require.NoError(t, err)
	require.NotEmpty(t, profiles)
	// Keep this above 1.5M so the fixture still exercises substantial work after sort deduplication.
	require.Greater(t, budget.totals[0], uint64(1_500_000))
	require.Greater(t, budget.totals[1], uint64(19_811))
	require.Greater(t, budget.totals[2], uint64(7_679_504))
	require.LessOrEqual(t, budget.totals[0], uint64(1<<22))
	require.LessOrEqual(t, budget.totals[1], uint64(1<<16))
	require.LessOrEqual(t, budget.totals[2], uint64(1<<26))
}

func TestProfilesContextStopsOnCancellation(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(0, 0, 20, 12)
	ctx, cancel := context.WithCancel(t.Context())
	budget := &profileBudget{
		limits: [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64},
		onCharge: func(work, _, _ uint64) error {
			if work > 0 {
				cancel()
			}
			return nil
		},
	}
	profiles, err := s.ProfilesContext(ctx, budget)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, profiles)
}

func TestProfilesContextArrangesClosedCurves(t *testing.T) {
	s := newSketch(t)
	s.CreateCircle(s.CreatePoint(0, 0), 5)
	profiles, err := s.ProfilesContext(t.Context(), &profileBudget{
		limits: [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64},
	})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Len(t, profiles[0].Entities, 1)
}

func TestProfilesContextReturnsNoProfilesForCancelledContext(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(0, 0, 20, 12)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	profiles, err := s.ProfilesContext(ctx, &profileBudget{
		limits: [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64},
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, profiles)
}

var _ sketch.ProfileBudget = (*profileBudget)(nil)
