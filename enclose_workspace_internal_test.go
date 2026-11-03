package sketch

import (
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

	first, _, why := sys.krawczyk([]float64{1}, pt(1), 1, []int{0}, []int{0}, ws)
	require.NotEmpty(t, first, why)
	want := slices.Clone(first)

	second, _, why := sys.krawczyk([]float64{2}, pt(2), 2, []int{0}, []int{0}, ws)
	require.NotEmpty(t, second, why)
	require.NotEqual(t, want, second, "the second call reuses the result workspace")
	require.Equal(t, want, first, "the first returned box remains unchanged")
}
