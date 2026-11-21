package sketch

// In-package on purpose: coneFitIntPoly, coneFitIntSign, coneFitIntBernstein
// and coneMulInterval are private helpers. The Lean theorems coneFitIntSign_eq,
// coneFitIntBounds_mem and coneMulInterval_memR prove them; these tests check
// the Go code against the rational forms they replaced.

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func randomConeRat(rng *rand.Rand) *big.Rat {
	num := rng.Int63n(2001) - 1000
	den := int64(1) << rng.Intn(60)
	if rng.Intn(3) == 0 {
		den *= 3
	}
	return big.NewRat(num, den)
}

func TestConeFitIntSignMatchesRationalSign(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 2000 {
		p := make(coneFitPoly, 1+rng.Intn(7))
		for i := range p {
			p[i] = randomConeRat(rng)
		}
		a := coneFitIntPoly(p)
		for range 4 {
			x := randomConeRat(rng)
			require.Equal(t, coneFitPolyEval(p, x).Sign(), coneFitIntSign(a, x), "p=%v t=%v", p, x)
		}
		// A root of p: p(r) = 0 makes the sign zero.
		r := randomConeRat(rng)
		q := coneFitPolyMul(p, coneFitPoly{new(big.Rat).Neg(r), big.NewRat(1, 1)})
		require.Zero(t, coneFitIntSign(coneFitIntPoly(q), r))
	}
}

func TestConeMulIntervalMatchesCornerHull(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 2000 {
		a := coneIntervalOf(randomConeRat(rng), randomConeRat(rng))
		b := coneIntervalOf(randomConeRat(rng), randomConeRat(rng))
		got := coneMulInterval(a, b)
		lo, hi := coneMul(a.lo, b.lo), coneMul(a.lo, b.lo)
		for _, p := range []*big.Rat{coneMul(a.lo, b.hi), coneMul(a.hi, b.lo), coneMul(a.hi, b.hi)} {
			if p.Cmp(lo) < 0 {
				lo = p
			}
			if p.Cmp(hi) > 0 {
				hi = p
			}
		}
		require.Zero(t, got.lo.Cmp(lo), "a=%v b=%v", a, b)
		require.Zero(t, got.hi.Cmp(hi), "a=%v b=%v", a, b)
	}
}

func TestConeFitIntBernsteinScalesRationalBernstein(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 1000 {
		p := make(coneFitPoly, 1+rng.Intn(7))
		for i := range p {
			p[i] = randomConeRat(rng)
		}
		iv := coneIntervalOf(randomConeRat(rng), randomConeRat(rng))
		want := coneFitBernstein(p, iv.lo, iv.hi)
		got := coneFitIntBernstein(coneFitIntPoly(p), iv.lo, iv.hi)
		require.Len(t, got, len(want))
		var scale *big.Rat
		for k, w := range want {
			if w.Sign() != 0 {
				scale = new(big.Rat).Quo(new(big.Rat).SetInt(got[k]), w)
				break
			}
		}
		if scale == nil {
			for _, g := range got {
				require.Zero(t, g.Sign())
			}
			continue
		}
		require.Positive(t, scale.Sign(), "p=%v on %v", p, iv)
		for k, w := range want {
			require.Zero(t, new(big.Rat).SetInt(got[k]).Cmp(coneMul(scale, w)), "p=%v on %v entry %d", p, iv, k)
		}
		lo, hi := coneFitIntBounds(got)
		wlo, whi := coneFitCoeffBounds(p, iv.lo, iv.hi)
		require.Equal(t, wlo.Sign(), lo.Sign())
		require.Equal(t, whi.Sign(), hi.Sign())
	}
}
