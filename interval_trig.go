package sketch

import (
	"math"
	"math/big"
)

// The certified path needs sine, cosine and the direction angle with PROVEN
// bounds. The math package's Sin/Cos/Atan2 are accurate to an ulp or two in
// practice, but that is a measured property, not a guarantee, so nothing here
// trusts them for a bound. Sine and cosine of a float64 are evaluated from
// their Taylor series in exact rational arithmetic (math/big), with the
// alternating-series remainder added on both sides; only the final conversion
// to float64 rounds, and it rounds outward. The direction angle is then
// BRACKETED rather than computed: math.Atan2 proposes a value, and the exact
// sine and cosine of two nearby floats prove the direction lies between them.

// ratLit parses a decimal literal known to be valid at compile time.
func ratLit(s string) *big.Rat {
	r, _ := new(big.Rat).SetString(s)
	return r
}

// piLo and piHi bracket π: the first 40 decimals, truncated, and that value
// plus 1e-40. They are read-only operands; nothing may write through them.
var (
	piLo = ratLit("3.1415926535897932384626433832795028841971")
	piHi = ratLit("3.1415926535897932384626433832795028841972")
)

// maxTaylorArg bounds the arguments the Taylor series is summed at directly. It
// keeps the series short (the terms shrink once n exceeds |x|); a larger
// argument is first reduced by whole turns.
//
// maxTrigArg bounds the arguments the certified path evaluates sine and cosine
// at, about 1.75e11 turns. The reduction subtracts 2πk for an integer k below
// 2^38, so the 1e-40 width of the π bracket costs under 1e-28 rad; beyond the
// bound an angle is refused.
const (
	maxTaylorArg = 64
	maxTrigArg   = 1 << 40
)

// trigFracBits is the fixed-point precision of the Taylor sums: every term is
// held as an integer count of 2^-trigFracBits. trigEpsBits ends the series once
// a term is certainly below 2^-trigEpsBits, far beneath float64 resolution near
// the values sine and cosine take.
const (
	trigFracBits = 256
	trigEpsBits  = 110
)

// ratLower and ratUpper convert an exact rational to the nearest float64 at or
// below / at or above it.
func ratLower(r *big.Rat) float64 {
	f, exact := r.Float64()
	if exact {
		return f
	}
	if new(big.Rat).SetFloat64(f).Cmp(r) > 0 {
		return down(f)
	}
	return f
}

func ratUpper(r *big.Rat) float64 {
	f, exact := r.Float64()
	if exact {
		return f
	}
	if new(big.Rat).SetFloat64(f).Cmp(r) < 0 {
		return up(f)
	}
	return f
}

// ratInterval encloses [lo, hi] in float64, rounding each end outward.
func ratInterval(lo, hi *big.Rat) Interval { return Interval{ratLower(lo), ratUpper(hi)} }

// fixedLower and fixedUpper convert a fixed-point integer (a count of
// 2^-trigFracBits) to the nearest float64 at or below / at or above it.
func fixedLower(v *big.Int) float64 {
	f := new(big.Float).SetInt(v)
	f.SetMantExp(f, -trigFracBits)
	r, acc := f.Float64()
	if acc == big.Above {
		return down(r)
	}
	return r
}

func fixedUpper(v *big.Int) float64 {
	f := new(big.Float).SetInt(v)
	f.SetMantExp(f, -trigFracBits)
	r, acc := f.Float64()
	if acc == big.Below {
		return up(r)
	}
	return r
}

// sinCosPoint encloses sin(x) and cos(x) for the exact value of x. It reports
// false for a non-finite x or one beyond maxTrigArg. An x beyond maxTaylorArg
// is reduced by whole turns first (see reduceTurns); every other x is summed
// directly.
//
// The terms t_n = x^n/n! are summed into the two series by n mod 4 until a term
// both lies past the peak (n > |x|+1, so every later term of either series is
// smaller in magnitude than the one before it) and is certainly below
// 2^-trigEpsBits. That first omitted term then bounds what is left of BOTH
// series: each is alternating with decreasing terms from there on, so its tail
// is no larger than its own first omitted term, and both of those are at most
// |t_n|.
//
// The terms are fixed-point integers. x is exactly m·2^e with m an integer, so
// t_{n+1} = t_n·m·2^e/(n+1) costs one exact multiply, one shift and one integer
// division; the shift and the division each lose under one unit. The error
// carried by the computed term is tracked alongside it as an integer bound,
// err_{n+1} = ⌈err_n·|x|/(n+1)⌉ + 2, and every bound the sums accumulate is
// added to both sides before the final outward conversion.
func sinCosPoint(x float64) (Interval, Interval, bool) {
	if !(math.Abs(x) <= maxTrigArg) {
		return Interval{}, Interval{}, false
	}
	if math.Abs(x) > maxTaylorArg {
		return sinCosRange(reduceTurns(x))
	}
	frac, exp := math.Frexp(x)
	m := big.NewInt(int64(frac * (1 << 53))) // exact: frac has 53 significant bits
	e := exp - 53
	am := new(big.Int).Abs(m)
	ax := math.Abs(x)

	one := new(big.Int).Lsh(big.NewInt(1), trigFracBits)
	eps := new(big.Int).Lsh(big.NewInt(1), trigFracBits-trigEpsBits)
	s, c := new(big.Int), new(big.Int)
	sErr, cErr := new(big.Int), new(big.Int)
	t := new(big.Int).Set(one)
	tErr := new(big.Int)
	bound := new(big.Int)
	for n := 0; ; n++ {
		bound.Abs(t)
		bound.Add(bound, tErr)
		if float64(n) > ax+1 && bound.Cmp(eps) < 0 {
			break
		}
		switch n % 4 {
		case 0:
			c.Add(c, t)
			cErr.Add(cErr, tErr)
		case 1:
			s.Add(s, t)
			sErr.Add(sErr, tErr)
		case 2:
			c.Sub(c, t)
			cErr.Add(cErr, tErr)
		case 3:
			s.Sub(s, t)
			sErr.Add(sErr, tErr)
		}
		// t ← ⌊t·m·2^e⌉/(n+1), losing under one unit in the shift and under
		// one in the division.
		div := big.NewInt(int64(n + 1))
		t.Mul(t, m)
		if e >= 0 {
			t.Lsh(t, uint(e))
		} else {
			t.Rsh(t, uint(-e))
		}
		t.Quo(t, div)
		// err ← ⌈err·|m|·2^e/(n+1)⌉ + 2, as ⌈num/den⌉ = ⌊(num+den−1)/den⌋.
		num := tErr.Mul(tErr, am)
		den := div
		if e >= 0 {
			num.Lsh(num, uint(e))
		} else {
			den = new(big.Int).Lsh(div, uint(-e))
		}
		num.Add(num, den)
		num.Sub(num, big.NewInt(1))
		tErr.Quo(num, den)
		tErr.Add(tErr, big.NewInt(2))
	}
	// bound now holds |t̂_n| + err_n ≥ |t_n|, the tail bound for both series.
	sw := new(big.Int).Add(sErr, bound)
	cw := new(big.Int).Add(cErr, bound)
	sin := Interval{fixedLower(new(big.Int).Sub(s, sw)), fixedUpper(new(big.Int).Add(s, sw))}
	cos := Interval{fixedLower(new(big.Int).Sub(c, cw)), fixedUpper(new(big.Int).Add(c, cw))}
	return clampUnit(sin), clampUnit(cos), true
}

// reduceTurns encloses x − 2πk, for k the integer nearest x/2π, over every π in
// [piLo, piHi]. Sine and cosine take the same value at x and at x − 2πk for
// the exact π, so the sine and cosine of the result enclose those of x. Any
// integer k would do; the nearest one leaves |x − 2πk| ≤ π plus rounding, well
// inside maxTaylorArg. The endpoints are exact rationals rounded outward once.
func reduceTurns(x float64) Interval {
	k := int64(math.Round(x / (2 * math.Pi)))
	kk := big.NewRat(2*k, 1)
	xr := new(big.Rat).SetFloat64(x)
	a := new(big.Rat).Sub(xr, new(big.Rat).Mul(kk, piHi))
	b := new(big.Rat).Sub(xr, new(big.Rat).Mul(kk, piLo))
	if k < 0 {
		a, b = b, a
	}
	return ratInterval(a, b)
}

// clampUnit intersects an enclosure of a sine or cosine with [-1, 1], which is
// sound because the exact value cannot leave it.
func clampUnit(i Interval) Interval { return Interval{max(i.Lo, -1), min(i.Hi, 1)} }

// sinCosRange encloses sin and cos over every θ in q. The endpoint values give
// the hull of a monotone stretch; an extremum of either function sits at a
// multiple of π/2, and wherever such a multiple MIGHT lie in q (judged against
// the rigorous π bracket, so an undecided case counts as inside) the matching
// ±1 joins the hull.
func sinCosRange(q Interval) (Interval, Interval, bool) {
	sa, ca, ok := sinCosPoint(q.Lo)
	if !ok {
		return Interval{}, Interval{}, false
	}
	if q.Lo == q.Hi {
		return sa, ca, true
	}
	sb, cb, ok := sinCosPoint(q.Hi)
	if !ok {
		return Interval{}, Interval{}, false
	}
	sin, cos := ihull(sa, sb), ihull(ca, cb)
	if q.Hi-q.Lo > 7 {
		return Interval{-1, 1}, Interval{-1, 1}, true
	}
	loR := new(big.Rat).SetFloat64(q.Lo)
	hiR := new(big.Rat).SetFloat64(q.Hi)
	kmin := int(math.Floor(q.Lo/(math.Pi/2))) - 1
	kmax := int(math.Ceil(q.Hi/(math.Pi/2))) + 1
	for k := kmin; k <= kmax; k++ {
		half := big.NewRat(int64(k), 2)
		a := new(big.Rat).Mul(half, piLo)
		b := new(big.Rat).Mul(half, piHi)
		if k < 0 {
			a, b = b, a
		}
		if b.Cmp(loR) < 0 || a.Cmp(hiR) > 0 {
			continue
		}
		switch ((k % 4) + 4) % 4 {
		case 0:
			cos.Hi = 1
		case 1:
			sin.Hi = 1
		case 2:
			cos.Lo = -1
		case 3:
			sin.Lo = -1
		}
	}
	return sin, cos, true
}

// twoPiTimes encloses k·2π.
func twoPiTimes(k int64) Interval {
	kk := big.NewRat(2*k, 1)
	a := new(big.Rat).Mul(kk, piLo)
	b := new(big.Rat).Mul(kk, piHi)
	if k < 0 {
		a, b = b, a
	}
	return ratInterval(a, b)
}

// atan2Point brackets the direction angle of the vector (d, c) — the angle
// math.Atan2(c, d) approximates — for the exact values of c and d. The result
// contains φ + 2πk for some integer k, where φ is that exact angle; it is a
// statement about the direction, which is all a mod-2π reading needs.
//
// A candidate [lo, hi] around the float estimate is accepted when the exact
// direction is strictly counterclockwise of lo's unit vector (their cross
// product is positive) and strictly clockwise of hi's (negative). Each
// condition alone selects an open half-turn; with hi−lo far below π, the two
// half-turns overlap in exactly the open arc (lo, hi).
func atan2Point(c, d float64) (Interval, bool) {
	if c == 0 && d == 0 {
		return Interval{}, false
	}
	phi := math.Atan2(c, d)
	if math.IsNaN(phi) {
		return Interval{}, false
	}
	for k := 0; k < 6; k++ {
		delta := math.Ldexp((1+math.Abs(phi))*1e-15, 4*k)
		lo, hi := phi-delta, phi+delta
		sl, cl, ok := sinCosPoint(lo)
		if !ok {
			return Interval{}, false
		}
		sh, ch, ok := sinCosPoint(hi)
		if !ok {
			return Interval{}, false
		}
		crossLo := isub(imul(cl, pt(c)), imul(sl, pt(d)))
		crossHi := isub(imul(ch, pt(c)), imul(sh, pt(d)))
		if crossLo.Lo > 0 && crossHi.Hi < 0 {
			return Interval{lo, hi}, true
		}
	}
	return Interval{}, false
}

// atan2Box encloses the direction angle of (d, c) over every point of the
// rectangle c × d, refusing when the rectangle touches the origin (where the
// angle is undefined). A convex set that misses the origin is seen from it
// under an angle below a half-turn, and the extreme directions are supporting
// rays through its corners, so the hull of the four corner brackets — each
// shifted by whole turns to lie within a half-turn of the centre's direction —
// is the enclosure. A corner more than 3 rad from the centre (a rectangle
// nearly surrounding the origin) is refused rather than trusted to the float
// shift decision.
func atan2Box(c, d Interval) (Interval, bool) {
	if c.Lo <= 0 && 0 <= c.Hi && d.Lo <= 0 && 0 <= d.Hi {
		return Interval{}, false
	}
	ref := math.Atan2(c.mid(), d.mid())
	corners := [4][2]float64{{c.Lo, d.Lo}, {c.Lo, d.Hi}, {c.Hi, d.Lo}, {c.Hi, d.Hi}}
	var out Interval
	for i, cd := range corners {
		phi, ok := atan2Point(cd[0], cd[1])
		if !ok {
			return Interval{}, false
		}
		switch m := phi.mid() - ref; {
		case m > math.Pi:
			phi = isub(phi, twoPiTimes(1))
		case m < -math.Pi:
			phi = iadd(phi, twoPiTimes(1))
		}
		if math.Abs(phi.mid()-ref) > 3 {
			return Interval{}, false
		}
		if i == 0 {
			out = phi
			continue
		}
		out = ihull(out, phi)
	}
	return out, true
}

// shiftNear moves an angle enclosure by whole turns so its midpoint lies
// within half a turn of ref. Every member keeps its value mod 2π.
func shiftNear(iv Interval, ref float64) Interval {
	k := math.Round((iv.mid() - ref) / (2 * math.Pi))
	if k == 0 || math.IsNaN(k) || math.Abs(k) > 1<<20 {
		return iv
	}
	return isub(iv, twoPiTimes(int64(k)))
}
