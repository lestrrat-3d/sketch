package sketch

// In-package on purpose: down and up are private helpers. This test is the Go
// evidence for assumption A3 in .claude/docs/proofs.md: on every finite input,
// down and up return the neighbouring float that math.Nextafter returns.

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownUpMatchNextafter(t *testing.T) {
	inputs := []float64{
		0, math.Copysign(0, -1),
		math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		2 * math.SmallestNonzeroFloat64, -2 * math.SmallestNonzeroFloat64,
		0x1p-1022, -0x1p-1022, // smallest normal: the subnormal boundary
		1, -1, 0.1, -0.1, 0x1p52, -0x1p52, 0x1p53, -0x1p53,
		math.MaxFloat64, -math.MaxFloat64,
		math.Nextafter(math.MaxFloat64, 0), -math.Nextafter(math.MaxFloat64, 0),
	}
	rng := rand.New(rand.NewSource(1))
	for range 2000 {
		v := math.Float64frombits(rng.Uint64())
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		inputs = append(inputs, v)
	}
	for _, x := range inputs {
		wantDown := math.Nextafter(x, math.Inf(-1))
		wantUp := math.Nextafter(x, math.Inf(1))
		require.Equal(t, math.Float64bits(wantDown), math.Float64bits(down(x)), "down(%v)", x)
		require.Equal(t, math.Float64bits(wantUp), math.Float64bits(up(x)), "up(%v)", x)
		// down(x) = -up(-x), the form the Lean model uses.
		require.Equal(t, math.Float64bits(down(x)), math.Float64bits(-up(-x)), "down(%v) vs -up(-%v)", x, x)
	}
}
