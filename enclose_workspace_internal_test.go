package sketch

import (
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKrawczykWorkspaceKeepsReturnedBoxes(t *testing.T) {
	sys := &certSystem{
		eqs: []certEquation{&certAffine{terms: []affineTerm{{vi: 0, coef: 1}}, useQ: true}},
		m:   1,
	}
	ws := newKrawczykWorkspace(1, 1, 1)

	first, firstX, why := sys.krawczyk([]float64{1}, pt(1), 1, []int{0}, []int{0}, ws)
	require.NotEmpty(t, first, why)
	want := slices.Clone(first)
	wantX := slices.Clone(firstX)
	control, controlX, why := sys.krawczyk([]float64{1}, pt(1), 1, []int{0}, []int{0}, newKrawczykWorkspace(1, 1, 1))
	require.NotEmpty(t, control, why)
	requireIntervalsBitsEqual(t, control, want, "the reused workspace returns the same K box")
	requireIntervalsBitsEqual(t, controlX, wantX, "the reused workspace returns the same X box")

	second, secondX, why := sys.krawczyk([]float64{2}, pt(2), 2, []int{0}, []int{0}, ws)
	require.NotEmpty(t, second, why)
	require.NotEqual(t, want, second, "the second call reuses the result workspace")
	require.NotEqual(t, wantX, secondX, "the second call reuses the X workspace")
	requireIntervalsBitsEqual(t, want, first, "the first returned box remains unchanged")
	requireIntervalsBitsEqual(t, wantX, firstX, "the first returned X box remains unchanged")
}

func requireIntervalsBitsEqual(t *testing.T, want, got []Interval, msg string) {
	t.Helper()
	require.Len(t, got, len(want), msg)
	for i := range want {
		require.Equal(t, [2]uint64{math.Float64bits(want[i].Lo), math.Float64bits(want[i].Hi)},
			[2]uint64{math.Float64bits(got[i].Lo), math.Float64bits(got[i].Hi)}, msg)
	}
}
