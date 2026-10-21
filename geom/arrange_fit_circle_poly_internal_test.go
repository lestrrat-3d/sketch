package geom

// In-package because polyMul and bernstein are unexported exact-arithmetic
// kernels; the property under test is that the integer-scaled forms equal the
// term-by-term Rat definitions they replaced, value for value.

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func refPolyMul(a, b []*big.Rat) []*big.Rat {
	out := make([]*big.Rat, len(a)+len(b)-1)
	for i := range out {
		out[i] = rat(0)
	}
	for i, x := range a {
		for j, y := range b {
			out[i+j] = add(out[i+j], mul(x, y))
		}
	}
	return out
}

func refBernstein(p []*big.Rat) []*big.Rat {
	n := len(p) - 1
	out := make([]*big.Rat, len(p))
	for k := range out {
		out[k] = rat(0)
		for i := 0; i <= k; i++ {
			factor := new(big.Rat).SetFrac(big.NewInt(binomial(k, i)), big.NewInt(binomial(n, i)))
			out[k] = add(out[k], mul(p[i], factor))
		}
	}
	return out
}

// randomFitCoeffs draws the denominators exactFitPieces produces: a float64
// mantissa over 1, 2, 3 or 6.
func randomFitCoeffs(rng *rand.Rand, n int) []*big.Rat {
	out := make([]*big.Rat, n)
	for i := range out {
		v := rat((rng.Float64() - 0.5) * 1e3)
		if rng.Intn(5) == 0 {
			v = rat(0)
		}
		out[i] = quo(v, rat([]float64{1, 2, 3, 6}[rng.Intn(4)]))
	}
	return out
}

func requireRatsEqual(t *testing.T, want, got []*big.Rat) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		require.Zero(t, want[i].Cmp(got[i]), "coefficient %d", i)
	}
}

func TestExactPolyKernelsMatchTermByTerm(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 200 {
		a, b := randomFitCoeffs(rng, 4), randomFitCoeffs(rng, 4)
		requireRatsEqual(t, refPolyMul(a, b), polyMul(a, b))
		f := randomFitCoeffs(rng, 7)
		requireRatsEqual(t, refBernstein(f), bernstein(f))
		cross := randomFitCoeffs(rng, 4)
		requireRatsEqual(t, refBernstein(cross), bernstein(cross))
		df := randomFitCoeffs(rng, 6)
		requireRatsEqual(t, refBernstein(df), bernstein(df))
	}
}
