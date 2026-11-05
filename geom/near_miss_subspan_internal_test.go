package geom

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBezierSubSpanScratchMatchesBezierSubSpan holds the stack-buffer sub-span
// to the allocating one, bit for bit, over curves of every degree the buffers
// take and the spans (empty, clamped, whole, interior) the callers pass.
func TestBezierSubSpanScratchMatchesBezierSubSpan(t *testing.T) {
	rng := rand.New(rand.NewPCG(0x53554253, 4))
	var sa, sb [maxSubSpanDegree][3]float64
	for trial := 0; trial < 5000; trial++ {
		n := 2 + rng.IntN(maxSubSpanDegree+2) // includes sizes past the buffers
		cp := make([][3]float64, n)
		for i := range cp {
			w := 0.5 + rng.Float64()
			cp[i] = [3]float64{w * rng.NormFloat64(), w * rng.NormFloat64(), w}
		}
		v0, v1 := rng.Float64()*1.4-0.2, rng.Float64()*1.4-0.2
		if rng.IntN(5) == 0 {
			v0, v1 = 0, 1
		}
		want := bezierSubSpan(append([][3]float64(nil), cp...), v0, v1)
		got := bezierSubSpanScratch(append([][3]float64(nil), cp...), v0, v1, &sa, &sb)
		require.Len(t, got, len(want), "trial %d", trial)
		for i := range want {
			for c := range 3 {
				require.Equal(t, math.Float64bits(want[i][c]), math.Float64bits(got[i][c]), "trial %d [%d][%d]", trial, i, c)
			}
		}
	}
}
