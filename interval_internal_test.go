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

// TestIsFiniteScreen pins Interval.isFinite to its definition, both endpoints
// finite and ordered, on every pair drawn from the boundary values.
func TestIsFiniteScreen(t *testing.T) {
	values := []float64{
		math.Inf(-1), -math.MaxFloat64, -1, math.Copysign(0, -1), 0, 1,
		math.MaxFloat64, math.Inf(1), math.NaN(),
	}
	for _, lo := range values {
		for _, hi := range values {
			want := lo <= hi && !math.IsInf(lo, 0) && !math.IsInf(hi, 0)
			require.Equal(t, want, Interval{lo, hi}.isFinite(), "[%v, %v]", lo, hi)
		}
	}
}

// benchIntervals returns n ordered intervals with random finite endpoints of
// mixed sign, so the benchmarks below see every sign case of imul and isqr.
func benchIntervals(n int) []Interval {
	rng := rand.New(rand.NewSource(2))
	out := make([]Interval, n)
	for i := range out {
		a, b := rng.NormFloat64()*10, rng.NormFloat64()*10
		out[i] = Interval{min(a, b), max(a, b)}
	}
	return out
}

// BenchmarkIntervalOps measures the outward-rounded helpers one at a time over
// a fixed table of operands. The sink keeps every result live.
func BenchmarkIntervalOps(b *testing.B) {
	const n = 1024
	xs := benchIntervals(n)
	ys := benchIntervals(n + 1)[1:]
	var sink Interval
	b.Run("up_down", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			x := xs[i%n]
			sink = Interval{down(x.Lo), up(x.Hi)}
		}
	})
	b.Run("iadd", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			sink = iadd(xs[i%n], ys[i%n])
		}
	})
	b.Run("isub", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			sink = isub(xs[i%n], ys[i%n])
		}
	})
	b.Run("imul", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			sink = imul(xs[i%n], ys[i%n])
		}
	})
	b.Run("imulPt", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			sink = imulPt(xs[i%n].Lo, ys[i%n])
		}
	})
	b.Run("isqr", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			sink = isqr(xs[i%n])
		}
	})
	b.Run("isFinite", func(b *testing.B) {
		ok := false
		for i := 0; b.Loop(); i++ {
			ok = xs[i%n].isFinite()
		}
		if !ok {
			sink = Interval{}
		}
	})
	_ = sink
}
