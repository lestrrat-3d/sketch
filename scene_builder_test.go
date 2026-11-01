package sketch_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func buildMeteredPrimitives(builder *sketch.SketchSceneBuilder) error {
	center, err := builder.CreatePoint(0, 0)
	if err != nil {
		return err
	}
	start, err := builder.CreatePoint(5, 0)
	if err != nil {
		return err
	}
	end, err := builder.CreatePoint(0, 5)
	if err != nil {
		return err
	}
	if _, err := builder.CreateLine(start, end); err != nil {
		return err
	}
	if _, err := builder.CreateCircle(center, 5); err != nil {
		return err
	}
	_, err = builder.CreateArc(center, start, end)
	return err
}

func TestCreateSketchContextChargesExactAndOneShortBudgets(t *testing.T) {
	newWorld := func() *sketch.World { return sketch.NewWorld() }
	measure := &profileBudget{limits: [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64}}
	world := newWorld()
	got, err := world.CreateSketchContext(t.Context(), world.XY(), measure, buildMeteredPrimitives)
	require.NoError(t, err)
	require.Len(t, got.Points(), 3)
	require.Len(t, got.Entities(), 3)
	require.Len(t, got.Constraints(), 1)
	require.Equal(t, []*sketch.Sketch{got}, world.Sketches())

	for i, total := range measure.totals {
		limits := [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64}
		limits[i] = total
		world = newWorld()
		got, err = world.CreateSketchContext(t.Context(), world.XY(), &profileBudget{limits: limits}, buildMeteredPrimitives)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Len(t, world.Sketches(), 1)

		limits[i] = total - 1
		world = newWorld()
		got, err = world.CreateSketchContext(t.Context(), world.XY(), &profileBudget{limits: limits}, buildMeteredPrimitives)
		require.ErrorIs(t, err, errProfileBudgetTestRefused)
		require.Nil(t, got)
		require.Empty(t, world.Sketches())
	}
}

func TestCreateSketchContextCancellationDoesNotPublishPartialSketch(t *testing.T) {
	world := sketch.NewWorld()
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	budget := &profileBudget{
		limits: [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64},
		onCharge: func(work, _, _ uint64) error {
			if work == 0 {
				return nil
			}
			calls++
			if calls == 5 {
				cancel()
			}
			return nil
		},
	}
	got, err := world.CreateSketchContext(ctx, world.XY(), budget, buildMeteredPrimitives)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)
	require.Empty(t, world.Sketches())
}

func TestCreateSketchContextCallbackFailureDoesNotPublishSketch(t *testing.T) {
	world := sketch.NewWorld()
	want := errors.New("scene recipe stopped")
	budget := &profileBudget{limits: [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64}}
	got, err := world.CreateSketchContext(t.Context(), world.XY(), budget, func(builder *sketch.SketchSceneBuilder) error {
		_, createErr := builder.CreatePoint(1, 2)
		if createErr != nil {
			return createErr
		}
		return want
	})
	require.ErrorIs(t, err, want)
	require.Nil(t, got)
	require.Empty(t, world.Sketches())
}

func TestCreateSketchContextSharesBudgetWithProfileArrangement(t *testing.T) {
	world := sketch.NewWorld()
	budget := &profileBudget{limits: [3]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64}}
	s, err := world.CreateSketchContext(t.Context(), world.XY(), budget, func(builder *sketch.SketchSceneBuilder) error {
		points := make([]*sketch.Point, 0, 4)
		for _, xy := range [][2]float64{{0, 0}, {10, 0}, {10, 5}, {0, 5}} {
			point, createErr := builder.CreatePoint(xy[0], xy[1])
			if createErr != nil {
				return createErr
			}
			points = append(points, point)
		}
		for i, point := range points {
			if _, createErr := builder.CreateLine(point, points[(i+1)%len(points)]); createErr != nil {
				return createErr
			}
		}
		return nil
	})
	require.NoError(t, err)
	constructionWork := budget.totals[0]
	profiles, err := s.ProfilesContext(t.Context(), budget)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Greater(t, budget.totals[0], constructionWork)
}
