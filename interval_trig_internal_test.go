package sketch

// In-package on purpose (see revision_internal_test.go for the repo's stance):
// the properties under test are the outward-rounding guarantees of the
// certified path's private interval and trigonometric kernels. The consumer-
// visible half of the contract (Enclose's boxes holding the closed-form poses)
// is covered by enclose_test.go.

import (
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// ratSinCosOracle encloses sin(x) and cos(x) as exact rationals by summing the
// Taylor series in big.Rat until a term past the peak drops below 2^-120,
// taking that term as the tail bound. It shares the series with sinCosPoint but
// none of its fixed-point error bookkeeping, which is the part under test.
func ratSinCosOracle(x float64) (*big.Rat, *big.Rat, *big.Rat, *big.Rat) {
	return ratSinCosOracleAt(new(big.Rat).SetFloat64(x), new(big.Rat))
}

// ratSinCosOracleAt is ratSinCosOracle at an exact rational xr known only to
// within ±slack of the argument wanted: sine and cosine move by at most the
// argument's change, so slack widens every bound by itself.
func ratSinCosOracleAt(xr, slack *big.Rat) (*big.Rat, *big.Rat, *big.Rat, *big.Rat) {
	ax, _ := new(big.Rat).Abs(xr).Float64()
	eps := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 120))
	s, c := new(big.Rat), new(big.Rat)
	t := big.NewRat(1, 1)
	abs := new(big.Rat)
	for n := 0; ; n++ {
		if float64(n) > ax+1 && abs.Abs(t).Cmp(eps) < 0 {
			break
		}
		switch n % 4 {
		case 0:
			c.Add(c, t)
		case 1:
			s.Add(s, t)
		case 2:
			c.Sub(c, t)
		case 3:
			s.Sub(s, t)
		}
		t.Mul(t, xr)
		t.Quo(t, new(big.Rat).SetInt64(int64(n+1)))
	}
	r := new(big.Rat).Abs(t)
	r.Add(r, slack)
	return clampRat(new(big.Rat).Sub(s, r)), clampRat(new(big.Rat).Add(s, r)),
		clampRat(new(big.Rat).Sub(c, r)), clampRat(new(big.Rat).Add(c, r))
}

// clampRat limits an oracle bound to [−1, 1], where every sine and cosine lies;
// the tail bound alone can push cos(1e-300)'s upper bound past 1.
func clampRat(r *big.Rat) *big.Rat {
	one := big.NewRat(1, 1)
	if r.Cmp(one) > 0 {
		return one
	}
	if m := big.NewRat(-1, 1); r.Cmp(m) < 0 {
		return m
	}
	return r
}

func ratOf(v float64) *big.Rat { return new(big.Rat).SetFloat64(v) }

// requireEncloses asserts the float interval iv contains the exact rational
// interval [lo, hi].
func requireEncloses(t *testing.T, iv Interval, lo, hi *big.Rat, msg string, args ...any) {
	t.Helper()
	margs := append([]any{msg}, args...)
	require.LessOrEqual(t, ratOf(iv.Lo).Cmp(lo), 0, margs...)
	require.GreaterOrEqual(t, ratOf(iv.Hi).Cmp(hi), 0, margs...)
}

func ulp(v float64) float64 { return math.Nextafter(math.Abs(v), math.Inf(1)) - math.Abs(v) }

func TestSinCosPointEnclosesExactValue(t *testing.T) {
	xs := []float64{0, 1e-300, -1e-300, 0.5, math.Pi / 2, math.Pi, -math.Pi, 3, 10, -33.3, 63.99, -64}
	rng := rand.New(rand.NewSource(1))
	for range 40 {
		xs = append(xs, (rng.Float64()*2-1)*8)
	}
	for _, x := range xs {
		sin, cos, ok := sinCosPoint(x)
		require.True(t, ok, "x=%v is in range", x)
		sLo, sHi, cLo, cHi := ratSinCosOracle(x)
		requireEncloses(t, sin, sLo, sHi, "sin(%v)", x)
		requireEncloses(t, cos, cLo, cHi, "cos(%v)", x)
		require.LessOrEqual(t, sin.Hi-sin.Lo, 2*ulp(sin.mid())+0x1p-100, "sin(%v) is tight", x)
		require.LessOrEqual(t, cos.Hi-cos.Lo, 2*ulp(cos.mid())+0x1p-100, "cos(%v) is tight", x)
	}
	for _, x := range []float64{math.Nextafter(maxTrigArg, math.Inf(1)), -2 * maxTrigArg, math.NaN(), math.Inf(1)} {
		_, _, ok := sinCosPoint(x)
		require.False(t, ok, "x=%v is refused", x)
	}
}

// machinPi encloses π as [lo, hi] from Machin's formula π = 16·atan(1/5) −
// 4·atan(1/239), each arctangent summed in exact rationals until a term drops
// below 2^-400 and that term taken as the tail bound. It shares nothing with
// piLo and piHi, so it checks the reduction's bracket independently.
func machinPi() (*big.Rat, *big.Rat) {
	eps := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 400))
	atanInv := func(n int64) (*big.Rat, *big.Rat) {
		sum := new(big.Rat)
		pow := big.NewRat(1, n) // 1/n^(2k+1)
		n2 := big.NewRat(1, n*n)
		for k := int64(0); ; k++ {
			term := new(big.Rat).Quo(pow, big.NewRat(2*k+1, 1))
			if term.Cmp(eps) < 0 {
				return sum, term
			}
			if k%2 == 0 {
				sum.Add(sum, term)
			} else {
				sum.Sub(sum, term)
			}
			pow.Mul(pow, n2)
		}
	}
	a, ea := atanInv(5)
	b, eb := atanInv(239)
	mid := new(big.Rat).Sub(new(big.Rat).Mul(big.NewRat(16, 1), a), new(big.Rat).Mul(big.NewRat(4, 1), b))
	err := new(big.Rat).Add(new(big.Rat).Mul(big.NewRat(16, 1), ea), new(big.Rat).Mul(big.NewRat(4, 1), eb))
	return new(big.Rat).Sub(mid, err), new(big.Rat).Add(mid, err)
}

func TestSinCosPointReducesWholeTurns(t *testing.T) {
	pLo, pHi := machinPi()
	require.True(t, pLo.Cmp(piHi) <= 0 && piLo.Cmp(pHi) <= 0, "the two π brackets agree")
	xs := []float64{64.0000001, -100, 1e6, -1e6, math.Pi/2 + 40*math.Pi, 1e12, -maxTrigArg, maxTrigArg}
	rng := rand.New(rand.NewSource(2))
	for range 20 {
		xs = append(xs, (rng.Float64()*2-1)*1e9)
	}
	for _, x := range xs {
		sin, cos, ok := sinCosPoint(x)
		require.True(t, ok, "x=%v is in range", x)
		// The oracle evaluates at x − 2πk for Machin's π, a rational within
		// |2k|·(pHi − pLo) of the reduced argument.
		k := int64(math.Round(x / (2 * math.Pi)))
		kk := big.NewRat(2*k, 1)
		r := new(big.Rat).Sub(ratOf(x), new(big.Rat).Mul(kk, pLo))
		slack := new(big.Rat).Abs(new(big.Rat).Mul(kk, new(big.Rat).Sub(pHi, pLo)))
		// Truncating r to a multiple of 2^-400 keeps the oracle's sums short; the
		// truncation joins the slack.
		unit := new(big.Int).Lsh(big.NewInt(1), 400)
		num := new(big.Int).Quo(new(big.Int).Mul(r.Num(), unit), r.Denom())
		r.SetFrac(num, unit)
		slack.Add(slack, new(big.Rat).SetFrac(big.NewInt(1), unit))
		sLo, sHi, cLo, cHi := ratSinCosOracleAt(r, slack)
		requireEncloses(t, sin, sLo, sHi, "sin(%v)", x)
		requireEncloses(t, cos, cLo, cHi, "cos(%v)", x)
		require.Less(t, sin.Hi-sin.Lo, 4e-15, "sin(%v) is tight", x)
		require.Less(t, cos.Hi-cos.Lo, 4e-15, "cos(%v) is tight", x)
	}
	t.Run("R3 sin(1e6) against math.Sin", func(t *testing.T) {
		sin, cos, ok := sinCosPoint(1e6)
		require.True(t, ok, "1e6 rad is in range")
		require.True(t, sin.Contains(math.Sin(1e6)), "math.Sin(1e6) lies in %v", sin)
		require.True(t, cos.Contains(math.Cos(1e6)), "math.Cos(1e6) lies in %v", cos)
		require.Less(t, sin.Hi-sin.Lo, 1e-9, "the sine enclosure is under 1e-9 wide")
	})
	t.Run("an interval past 64 rad", func(t *testing.T) {
		q := Interval{40 * math.Pi, 40*math.Pi + math.Pi/2}
		sin, cos, ok := sinCosRange(q)
		require.True(t, ok, "the range is in bounds")
		require.Equal(t, 1.0, sin.Hi, "π/2 + 40π lies inside, so sin reaches 1")
		require.Equal(t, 1.0, cos.Hi, "40π lies inside, so cos reaches 1")
		require.InDelta(t, 0, sin.Lo, 1e-13, "sin starts at 0")
		require.InDelta(t, 0, cos.Lo, 1e-13, "cos ends at 0")
	})
}

func TestSinCosRangeIncludesInteriorExtrema(t *testing.T) {
	sin, cos, ok := sinCosRange(Interval{1.5, 1.7})
	require.True(t, ok, "range in bounds")
	require.Equal(t, 1.0, sin.Hi, "π/2 lies inside, so sin reaches 1")
	require.Less(t, cos.Lo, 0.0, "cos changes sign across π/2")

	sin, cos, ok = sinCosRange(Interval{3, 3.3})
	require.True(t, ok, "range in bounds")
	require.Equal(t, -1.0, cos.Lo, "π lies inside, so cos reaches −1")
	require.Less(t, sin.Lo, 0.0, "sin changes sign across π")

	sin, cos, ok = sinCosRange(Interval{-1.6, -1.5})
	require.True(t, ok, "range in bounds")
	require.Equal(t, -1.0, sin.Lo, "−π/2 lies inside, so sin reaches −1")
	require.Greater(t, cos.Hi, 0.0, "cos is positive near −π/2")

	sin, cos, ok = sinCosRange(Interval{0.1, 0.2})
	require.True(t, ok, "range in bounds")
	require.Less(t, sin.Hi, 1.0, "no extremum inside")
	require.Less(t, cos.Hi, 1.0, "no extremum inside")
	for _, q := range []float64{0.1, 0.15, 0.2} {
		require.True(t, sin.Contains(math.Sin(q)) && cos.Contains(math.Cos(q)), "q=%v is enclosed", q)
	}
}

func TestAtan2PointBracketsDirection(t *testing.T) {
	quarter := func(k int64) (*big.Rat, *big.Rat) {
		f := big.NewRat(k, 4)
		a, b := new(big.Rat).Mul(f, piLo), new(big.Rat).Mul(f, piHi)
		if k < 0 {
			a, b = b, a
		}
		return a, b
	}
	for _, tc := range []struct {
		c, d float64
		k    int64 // the exact angle is k·π/4
	}{{1, 1, 1}, {1, 0, 2}, {0, 1, 0}, {-1, -1, -3}, {1, -1, 3}, {-5, 5, -1}} {
		iv, ok := atan2Point(tc.c, tc.d)
		require.True(t, ok, "(%v, %v) has a direction", tc.c, tc.d)
		lo, hi := quarter(tc.k)
		requireEncloses(t, iv, lo, hi, "atan2(%v, %v) = %d·π/4", tc.c, tc.d, tc.k)
	}
	iv, ok := atan2Point(0, -1)
	require.True(t, ok, "the negative x axis has a direction")
	lo, hi := quarter(4)
	requireEncloses(t, shiftNear(iv, 3), lo, hi, "atan2(0, −1) = π")

	_, ok = atan2Point(0, 0)
	require.False(t, ok, "the zero vector has no direction")

	rng := rand.New(rand.NewSource(2))
	for range 200 {
		c, d := rng.NormFloat64()*50, rng.NormFloat64()*50
		iv, ok := atan2Point(c, d)
		require.True(t, ok, "(%v, %v) has a direction", c, d)
		sl, cl, ok := sinCosPoint(iv.Lo)
		require.True(t, ok)
		sh, ch, ok := sinCosPoint(iv.Hi)
		require.True(t, ok)
		require.Greater(t, isub(imul(cl, pt(c)), imul(sl, pt(d))).Lo, 0.0)
		require.Less(t, isub(imul(ch, pt(c)), imul(sh, pt(d))).Hi, 0.0)
		require.True(t, iv.Contains(math.Atan2(c, d)), "the float estimate lies in the bracket")
		require.Less(t, iv.Hi-iv.Lo, 1e-13, "the bracket is tight")
	}
}

func TestAtan2BoxAcrossTheCut(t *testing.T) {
	for _, tc := range []struct{ c, d Interval }{
		{Interval{1, 2}, Interval{3, 5}},
		{Interval{1, 2}, Interval{-5, -3}},
		{Interval{-2, -1}, Interval{3, 5}},
		{Interval{-2, -1}, Interval{-5, -3}},
		{Interval{-1, 1}, Interval{3, 5}},
		{Interval{-1, 1}, Interval{-5, -3}},
		{Interval{3, 5}, Interval{-1, 1}},
		{Interval{-5, -3}, Interval{-1, 1}},
	} {
		got, ok := atan2Box(tc.c, tc.d)
		require.True(t, ok)
		for _, c := range []float64{tc.c.Lo, tc.c.Hi} {
			for _, d := range []float64{tc.d.Lo, tc.d.Hi} {
				corner, ok := atan2Point(c, d)
				require.True(t, ok)
				corner = shiftNear(corner, got.mid())
				require.LessOrEqual(t, got.Lo, corner.Lo)
				require.GreaterOrEqual(t, got.Hi, corner.Hi)
			}
		}
	}

	iv, ok := atan2Box(Interval{-1e-3, 1e-3}, Interval{-1.01, -0.99})
	require.True(t, ok, "the box misses the origin")
	iv = shiftNear(iv, math.Pi)
	require.True(t, iv.Contains(math.Pi), "the box straddles the negative x axis")
	require.Less(t, iv.Hi-iv.Lo, 3e-3, "the enclosure stays narrow across the cut")

	_, ok = atan2Box(Interval{-1, 1}, Interval{-1, 1})
	require.False(t, ok, "a box around the origin has no direction")
}

func TestIntervalArithmeticEnclosesExactResults(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	draw := func() Interval {
		a, b := rng.NormFloat64()*1e3, rng.NormFloat64()*1e3
		return Interval{min(a, b), max(a, b)}
	}
	exact := func(op func(x, y *big.Rat) *big.Rat, a, b Interval) (*big.Rat, *big.Rat) {
		var lo, hi *big.Rat
		for _, x := range []float64{a.Lo, a.Hi} {
			for _, y := range []float64{b.Lo, b.Hi} {
				v := op(ratOf(x), ratOf(y))
				if lo == nil || v.Cmp(lo) < 0 {
					lo = v
				}
				if hi == nil || v.Cmp(hi) > 0 {
					hi = v
				}
			}
		}
		return lo, hi
	}
	for range 500 {
		a, b := draw(), draw()
		lo, hi := exact(func(x, y *big.Rat) *big.Rat { return new(big.Rat).Add(x, y) }, a, b)
		requireEncloses(t, iadd(a, b), lo, hi, "add")
		lo, hi = exact(func(x, y *big.Rat) *big.Rat { return new(big.Rat).Sub(x, y) }, a, b)
		requireEncloses(t, isub(a, b), lo, hi, "sub")
		lo, hi = exact(func(x, y *big.Rat) *big.Rat { return new(big.Rat).Mul(x, y) }, a, b)
		requireEncloses(t, imul(a, b), lo, hi, "mul")
		sq := isqr(a)
		for _, x := range []float64{a.Lo, a.Hi} {
			v := new(big.Rat).Mul(ratOf(x), ratOf(x))
			requireEncloses(t, sq, v, v, "sqr endpoint")
		}
		if a.Lo <= 0 && 0 <= a.Hi {
			require.Equal(t, 0.0, sq.Lo, "a square straddling zero starts at zero")
		}
	}
	r := isqrtNonNeg(Interval{2, 2})
	two := big.NewRat(2, 1)
	require.Negative(t, new(big.Rat).Mul(ratOf(r.Lo), ratOf(r.Lo)).Cmp(two), "√2's lower bound squares below 2")
	require.Positive(t, new(big.Rat).Mul(ratOf(r.Hi), ratOf(r.Hi)).Cmp(two), "√2's upper bound squares above 2")
}
