package geom

// In-package because fitMemo is an unexported cache; the properties are that a
// hit never rebuilds, a different key never hits, and a full cache empties
// instead of growing.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFitMemoHitsMissesAndBoundsItself(t *testing.T) {
	var memo fitMemo[int]
	builds := 0
	build := func(v int) func() int {
		return func() int { builds++; return v }
	}
	require.Equal(t, 1, memo.get("a", build(1)))
	require.Equal(t, 1, memo.get("a", build(2)), "a hit returns the stored value")
	require.Equal(t, 1, builds)
	require.Equal(t, 3, memo.get("b", build(3)), "another key misses")
	require.Equal(t, 2, builds)

	for i := range fitMemoCap + 1 {
		memo.get(string(rune('A'+i%26))+string(rune(i)), build(i))
	}
	require.LessOrEqual(t, len(memo.m), fitMemoCap)
}

func TestFitInterpolantKeyCoversEveryInput(t *testing.T) {
	base := &FitInterpolant{
		Params:       []float64{0, 1, 2},
		Points:       [][2]float64{{0, 0}, {1, 0}, {2, 1}},
		SecondDerivs: [][2]float64{{0, 0}, {0.5, 0.25}, {0, 0}},
	}
	key := fitInterpolantKey(base)
	require.Equal(t, key, fitInterpolantKey(base))
	for name, mutate := range map[string]func(*FitInterpolant){
		"params": func(f *FitInterpolant) { f.Params[2] = 2.0000000000000004 },
		"points": func(f *FitInterpolant) { f.Points[1][1] = 1e-300 },
		"second": func(f *FitInterpolant) { f.SecondDerivs[1][0] = 0.5000000000000001 },
		"length": func(f *FitInterpolant) { f.Params = f.Params[:2] },
	} {
		clone := &FitInterpolant{
			Params:       append([]float64(nil), base.Params...),
			Points:       append([][2]float64(nil), base.Points...),
			SecondDerivs: append([][2]float64(nil), base.SecondDerivs...),
		}
		mutate(clone)
		require.NotEqual(t, key, fitInterpolantKey(clone), name)
	}
}
