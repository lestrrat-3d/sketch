package geom_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func twoRectangleCurves(prism, loft [4]float64) []geom.Curve {
	curves := make([]geom.Curve, 0, 8)
	for _, box := range [2][4]float64{prism, loft} {
		points := [4]*geom.Point{
			geom.NewPoint(box[0], box[1]), geom.NewPoint(box[2], box[1]),
			geom.NewPoint(box[2], box[3]), geom.NewPoint(box[0], box[3]),
		}
		for i := range points {
			curves = append(curves, geom.NewLine(points[i], points[(i+1)%len(points)]))
		}
	}
	return curves
}

func TestRegionsTwoRectangleCapsBoundedCertifiedCells(t *testing.T) {
	prism := [4]float64{0, 0, 10, 10}
	scenes := [2][]geom.Curve{
		twoRectangleCurves(prism, [4]float64{8, 4, 12, 6}),
		twoRectangleCurves(prism, [4]float64{8, 4, 13, 6}),
	}
	var charged uint64
	arrangements, err := geom.RegionsTwoRectangleCapsBounded(t.Context(), scenes, func(n uint64) error {
		charged += n
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, charged)
	for i, arr := range arrangements {
		require.NotNil(t, arr)
		require.False(t, arr.Degenerate)
		require.Empty(t, arr.Chains)
		require.Len(t, arr.Regions, 3)
		areas := make([]float64, 0, 3)
		for _, region := range arr.Regions {
			require.False(t, region.Degenerate)
			require.Empty(t, region.Holes)
			areas = append(areas, region.Area)
			for _, edge := range region.Outer {
				require.GreaterOrEqual(t, edge.SourceIndex, 0)
				require.Less(t, edge.SourceIndex, 8)
				require.True(t, edge.TExact)
				require.Less(t, edge.TStart, edge.TEnd)
				require.Len(t, edge.Polyline, 2)
			}
		}
		sort.Float64s(areas)
		if i == 0 {
			require.Equal(t, []float64{4, 4, 96}, areas)
		} else {
			require.Equal(t, []float64{4, 6, 96}, areas)
		}
	}
}

func TestRegionsTwoRectangleCapsBoundedRefusesBeforePublication(t *testing.T) {
	prism := [4]float64{0, 0, 10, 10}
	scenes := [2][]geom.Curve{
		twoRectangleCurves(prism, [4]float64{8, 4, 12, 6}),
		twoRectangleCurves(prism, [4]float64{8, 4, 13, 6}),
	}
	budgetErr := errors.New("caller budget exhausted")
	got, err := geom.RegionsTwoRectangleCapsBounded(t.Context(), scenes, func(n uint64) error {
		if n == 104 {
			return budgetErr
		}
		return nil
	})
	require.ErrorIs(t, err, budgetErr)
	require.Nil(t, got[0])
	require.Nil(t, got[1])

	bad := scenes
	bad[1] = append([]geom.Curve(nil), scenes[1]...)
	bad[1][7] = geom.NewArc(geom.NewPoint(8, 5),
		geom.NewPoint(9, 5), geom.NewPoint(8, 6))
	got, err = geom.RegionsTwoRectangleCapsBounded(t.Context(), bad,
		func(uint64) error { return nil })
	require.ErrorIs(t, err, geom.ErrUnsupportedLineScene)
	require.Nil(t, got[0])
	require.Nil(t, got[1])
}
