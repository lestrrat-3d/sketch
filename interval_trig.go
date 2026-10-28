package sketch

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"sync/atomic"
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

// Each slot holds one completed point enclosure. Collisions only cause a
// recomputation; the Taylor path still decides every result.
const trigPointCacheSize = 16384

type trigPointEntry struct {
	key      uint64
	sin, cos Interval
}

var trigPointCache [trigPointCacheSize]atomic.Pointer[trigPointEntry]

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
	key := math.Float64bits(x)
	slot := &trigPointCache[(key^(key>>32)^(key>>16))&(trigPointCacheSize-1)]
	if held := slot.Load(); held != nil && held.key == key {
		return held.sin, held.cos, true
	}
	sin, cos, fits := sinCosSeriesWords(x)
	if !fits {
		sin, cos = sinCosSeriesBig(x)
	}
	slot.Store(&trigPointEntry{key: key, sin: sin, cos: cos})
	return sin, cos, true
}

// sinCosSeriesBig sums sinCosPoint's series in math/big integers, for any
// |x| ≤ maxTaylorArg. sinCosSeriesWords computes the same integers in fixed
// width and is the one sinCosPoint runs; this path stands behind it for a sum
// that would not fit that width.
func sinCosSeriesBig(x float64) (Interval, Interval) {
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
	div, den, rem := new(big.Int), new(big.Int), new(big.Int)
	oneInt, twoInt := big.NewInt(1), big.NewInt(2)
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
		div.SetInt64(int64(n + 1))
		t.Mul(t, m)
		if e >= 0 {
			t.Lsh(t, uint(e))
		} else {
			t.Rsh(t, uint(-e))
		}
		t.Quo(t, div)
		// err ← ⌈err·|m|·2^e/(n+1)⌉ + 2. The remainder says
		// whether the exact quotient needs one more unit.
		tErr.Mul(tErr, am)
		den.Set(div)
		if e >= 0 {
			tErr.Lsh(tErr, uint(e))
		} else {
			den.Lsh(den, uint(-e))
		}
		tErr.QuoRem(tErr, den, rem)
		if rem.Sign() != 0 {
			tErr.Add(tErr, oneInt)
		}
		tErr.Add(tErr, twoInt)
	}
	// bound now holds |t̂_n| + err_n ≥ |t_n|, the tail bound for both series.
	sw := new(big.Int).Add(sErr, bound)
	cw := new(big.Int).Add(cErr, bound)
	sin := Interval{fixedLower(new(big.Int).Sub(s, sw)), fixedUpper(new(big.Int).Add(s, sw))}
	cos := Interval{fixedLower(new(big.Int).Sub(c, cw)), fixedUpper(new(big.Int).Add(c, cw))}
	return clampUnit(sin), clampUnit(cos)
}

// seriesWords is the width, in 64-bit words, of the integers
// sinCosSeriesWords sums in. For |x| ≤ maxTaylorArg every one of them stays far
// inside it: the exact term 2^256·|x|^n/n! peaks below 2^345 (at n = 64, as
// 64^64/64! < 2^89), the computed one exceeds it by under e^64 < 2^93, so a
// term is below 2^346 and its product with |m| < 2^53 below 2^399; the error
// bound obeys err ≤ err·|x|/(n+1) + 3 and stays below 3·e^64 < 2^95; and the
// series stops within 400 terms (past n = 128 every term at least halves), so
// each sum is below 2^355. 448 bits hold all of it, with the sign. The two
// multiplies still check their carry, and a sum that does not fit falls back
// to sinCosSeriesBig.
const seriesWords = 7

// seriesInt is a fixed-width integer in little-endian 64-bit words: a
// magnitude, or a two's-complement value for the signed sums.
type seriesInt [seriesWords]uint64

func (a *seriesInt) isZero() bool {
	for _, w := range a {
		if w != 0 {
			return false
		}
	}
	return true
}

// below reports whether the magnitude a is below 2^k.
func (a *seriesInt) below(k int) bool {
	for _, w := range a[k/64+1:] {
		if w != 0 {
			return false
		}
	}
	return a[k/64] < uint64(1)<<(k%64)
}

// mulWord sets a to a·m and reports whether the product fits.
func (a *seriesInt) mulWord(m uint64) bool {
	var carry uint64
	for i, w := range a {
		hi, lo := bits.Mul64(w, m)
		lo, c := bits.Add64(lo, carry, 0)
		a[i], carry = lo, hi+c
	}
	return carry == 0
}

// shrSticky sets the magnitude a to ⌊a/2^s⌋ and reports whether a set bit was
// shifted out.
func (a *seriesInt) shrSticky(s uint) bool {
	ws, bs := int(s/64), s%64
	if ws >= seriesWords {
		sticky := !a.isZero()
		*a = seriesInt{}
		return sticky
	}
	sticky := a[ws]&(uint64(1)<<bs-1) != 0
	for _, w := range a[:ws] {
		sticky = sticky || w != 0
	}
	for i := range a {
		var w uint64
		if j := i + ws; j < seriesWords {
			w = a[j] >> bs
			if bs != 0 && j+1 < seriesWords {
				w |= a[j+1] << (64 - bs)
			}
		}
		a[i] = w
	}
	return sticky
}

// divWord sets the magnitude a to ⌊a/d⌋ for a nonzero d and returns the
// remainder. Leading zero words divide to zero with no remainder, so the
// division starts at the top nonzero word.
func (a *seriesInt) divWord(d uint64) uint64 {
	var r uint64
	i := seriesWords - 1
	for i >= 0 && a[i] == 0 {
		i--
	}
	for ; i >= 0; i-- {
		a[i], r = bits.Div64(r, a[i], d)
	}
	return r
}

// addWord, add and sub wrap modulo 2^(64·seriesWords), which is exact
// two's-complement arithmetic while the result fits.
func (a *seriesInt) addWord(v uint64) {
	for i := range a {
		var c uint64
		a[i], c = bits.Add64(a[i], v, 0)
		if c == 0 {
			return
		}
		v = c
	}
}

func (a *seriesInt) add(b *seriesInt) {
	var c uint64
	for i := range a {
		a[i], c = bits.Add64(a[i], b[i], c)
	}
}

func (a *seriesInt) sub(b *seriesInt) {
	var c uint64
	for i := range a {
		a[i], c = bits.Sub64(a[i], b[i], c)
	}
}

// addSigned adds the magnitude b to a, or subtracts it when neg is set.
func (a *seriesInt) addSigned(b *seriesInt, neg bool) {
	if neg {
		a.sub(b)
		return
	}
	a.add(b)
}

// fixedFloat converts the two's-complement count of 2^-trigFracBits a to the
// float64 nearest it at or below (upward false) or at or above (upward true):
// the value fixedLower or fixedUpper returns for the same integer, as each is
// the one float with that property. The top 53 bits of the magnitude convert
// exactly, a nonzero magnitude being at least 2^-trigFracBits, far above the
// subnormals; the truncation they leave out moves the result one float
// outward unless it already lies on the requested side.
func (a seriesInt) fixedFloat(upward bool) float64 {
	neg := a[seriesWords-1]>>63 != 0
	if neg {
		var mag seriesInt
		mag.sub(&a)
		a = mag
	}
	top := seriesWords - 1
	for top >= 0 && a[top] == 0 {
		top--
	}
	if top < 0 {
		return 0
	}
	shift := max(64*top+bits.Len64(a[top])-53, 0)
	inexact := a.shrSticky(uint(shift))
	f := math.Ldexp(float64(a[0]), shift-trigFracBits)
	if neg {
		f = -f
	}
	switch {
	case !inexact || upward == neg:
		return f
	case upward:
		return up(f)
	default:
		return down(f)
	}
}

// sinCosSeriesWords is sinCosSeriesBig in seriesInt arithmetic, for any
// |x| ≤ maxTaylorArg. It reports false, having computed nothing it returns,
// when a product does not fit seriesWords. Otherwise every integer it forms
// equals the one sinCosSeriesBig forms at the same step, so the two return
// identical intervals:
//
//   - x = m·2^-sh with sh = 53 − exp ≥ 46, as |x| ≤ 64 leaves exp ≤ 7.
//   - big.Int's Rsh floors and its Quo truncates toward zero, so the next term
//     has the sign of t·m and the magnitude ⌊⌈|t·m|/2^sh⌉/(n+1)⌋ when t·m is
//     negative, ⌊⌊|t·m|/2^sh⌋/(n+1)⌋ otherwise; the shift's sticky bit tells
//     the two roundings apart.
//   - The error bound's ⌈err·|m|/((n+1)·2^sh)⌉ is taken as ⌈⌈err·|m|/2^sh⌉/(n+1)⌉,
//     the same integer, since ⌈⌈a/b⌉/c⌉ = ⌈a/(b·c)⌉ for positive integers.
//   - The sums take each term's sign through addSigned, and the final integers
//     convert through fixedFloat, whose result is fixedLower's or fixedUpper's.
func sinCosSeriesWords(x float64) (Interval, Interval, bool) {
	frac, exp := math.Frexp(x)
	m := int64(frac * (1 << 53)) // exact: frac has 53 significant bits
	mNeg := m < 0
	am := uint64(m)
	if mNeg {
		am = uint64(-m)
	}
	sh := uint(53 - exp)
	ax := math.Abs(x)

	var s, c, sErr, cErr, t, tErr, bound seriesInt
	t[trigFracBits/64] = uint64(1) << (trigFracBits % 64)
	tNeg := false
	for n := 0; ; n++ {
		bound = t
		bound.add(&tErr)
		if float64(n) > ax+1 && bound.below(trigFracBits-trigEpsBits) {
			break
		}
		switch n % 4 {
		case 0:
			c.addSigned(&t, tNeg)
			cErr.add(&tErr)
		case 1:
			s.addSigned(&t, tNeg)
			sErr.add(&tErr)
		case 2:
			c.addSigned(&t, !tNeg)
			cErr.add(&tErr)
		case 3:
			s.addSigned(&t, !tNeg)
			sErr.add(&tErr)
		}
		d := uint64(n + 1)
		pNeg := tNeg != mNeg
		if !t.mulWord(am) {
			return Interval{}, Interval{}, false
		}
		if t.shrSticky(sh) && pNeg {
			t.addWord(1)
		}
		t.divWord(d)
		tNeg = pNeg
		if !tErr.mulWord(am) {
			return Interval{}, Interval{}, false
		}
		if tErr.shrSticky(sh) {
			tErr.addWord(1)
		}
		if tErr.divWord(d) != 0 {
			tErr.addWord(1)
		}
		tErr.addWord(2)
	}
	sw, cw := sErr, cErr
	sw.add(&bound)
	cw.add(&bound)
	sLo, sHi, cLo, cHi := s, s, c, c
	sLo.sub(&sw)
	sHi.add(&sw)
	cLo.sub(&cw)
	cHi.add(&cw)
	sin := Interval{sLo.fixedFloat(false), sHi.fixedFloat(true)}
	cos := Interval{cLo.fixedFloat(false), cHi.fixedFloat(true)}
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

// halfPiBracket encloses k·π/2 over every π in [piLo, piHi]: the exact
// rationals a ≤ b are k/2·piLo and k/2·piHi in order, lo is the largest float
// at or below a and hi the smallest at or above b, and the exact flags say
// whether lo equals a and hi equals b.
type halfPiBracket struct {
	lo, hi           float64
	loExact, hiExact bool
}

// halfPiTableK bounds the multiples of π/2 held in halfPiTable, enough for
// every angle within about 200 rad of zero.
const halfPiTableK = 128

// halfPiTable holds halfPiBracket for every k in [−halfPiTableK,
// halfPiTableK], at index k + halfPiTableK.
var halfPiTable = func() [2*halfPiTableK + 1]halfPiBracket {
	var out [2*halfPiTableK + 1]halfPiBracket
	for k := -halfPiTableK; k <= halfPiTableK; k++ {
		a, b := halfPiRats(k)
		_, loExact := a.Float64()
		_, hiExact := b.Float64()
		out[k+halfPiTableK] = halfPiBracket{lo: ratLower(a), hi: ratUpper(b), loExact: loExact, hiExact: hiExact}
	}
	return out
}()

// halfPiRats returns the exact bracket [a, b] of k·π/2.
func halfPiRats(k int) (*big.Rat, *big.Rat) {
	half := big.NewRat(int64(k), 2)
	a := new(big.Rat).Mul(half, piLo)
	b := new(big.Rat).Mul(half, piHi)
	if k < 0 {
		a, b = b, a
	}
	return a, b
}

// halfPiMayLieIn reports whether the bracket [a, b] of k·π/2 meets q, that is
// neither b < q.Lo nor a > q.Hi, decided in exact arithmetic. Within the
// table the float bounds decide it with no rounding: for a float v, b < v
// holds exactly when hi < v if hi equals b, and when hi ≤ v otherwise, since
// no float lies in [b, hi) then; a > v mirrors it with lo.
func halfPiMayLieIn(k int, q Interval) bool {
	if k < -halfPiTableK || k > halfPiTableK {
		a, b := halfPiRats(k)
		return b.Cmp(new(big.Rat).SetFloat64(q.Lo)) >= 0 && a.Cmp(new(big.Rat).SetFloat64(q.Hi)) <= 0
	}
	br := &halfPiTable[k+halfPiTableK]
	bBelow := br.hi < q.Lo || !br.hiExact && br.hi == q.Lo
	aAbove := br.lo > q.Hi || !br.loExact && br.lo == q.Hi
	return !bBelow && !aAbove
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
	kmin := int(math.Floor(q.Lo/(math.Pi/2))) - 1
	kmax := int(math.Ceil(q.Hi/(math.Pi/2))) + 1
	for k := kmin; k <= kmax; k++ {
		if !halfPiMayLieIn(k, q) {
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
// product is positive) and strictly clockwise of hi's (negative). The cross
// product and its slope are enclosed at the float estimate using one certified
// sine/cosine pair; the second-order remainder at each end is bounded by
// (|c|+|d|)·h²/2. With hi−lo far below π, the two open half-turns overlap in
// exactly the arc (lo, hi).
func atan2Point(c, d float64) (Interval, bool) {
	if c == 0 && d == 0 {
		return Interval{}, false
	}
	phi := math.Atan2(c, d)
	if math.IsNaN(phi) {
		return Interval{}, false
	}
	sin, cos, ok := sinCosPoint(phi)
	if !ok {
		return Interval{}, false
	}
	base := isub(imul(pt(c), cos), imul(pt(d), sin))
	slope := ineg(iadd(imul(pt(c), sin), imul(pt(d), cos)))
	crossAt := func(angle float64) Interval {
		h := isub(pt(angle), pt(phi))
		halfSq := Interval{0, up(isqr(h).Hi / 2)}
		tail := iadd(imul(pt(math.Abs(c)), halfSq), imul(pt(math.Abs(d)), halfSq))
		return iadd(iadd(base, imul(h, slope)), Interval{-tail.Hi, tail.Hi})
	}
	for k := 0; k < 6; k++ {
		delta := math.Ldexp((1+math.Abs(phi))*1e-15, 4*k)
		lo, hi := phi-delta, phi+delta
		crossLo := crossAt(lo)
		crossHi := crossAt(hi)
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
// rays through its corners. Where no coordinate endpoint is zero, the signs
// of the angle's two partial derivatives fix two extreme corners; otherwise
// all four corners are read. Their brackets are shifted by whole turns to lie
// within a half-turn of the centre's direction. A corner more than 3 rad from the
// centre (a rectangle nearly surrounding the origin) is refused rather than
// trusted to the float shift decision.
func atan2Box(c, d Interval) (Interval, bool) {
	if c.Lo <= 0 && 0 <= c.Hi && d.Lo <= 0 && 0 <= d.Hi {
		return Interval{}, false
	}
	ref := math.Atan2(c.mid(), d.mid())
	corners := [4][2]float64{{c.Lo, d.Lo}, {c.Lo, d.Hi}, {c.Hi, d.Lo}, {c.Hi, d.Hi}}
	count := len(corners)
	switch {
	case c.Lo == 0 || c.Hi == 0 || d.Lo == 0 || d.Hi == 0:
	case d.Lo > 0 || d.Hi < 0:
		minC, maxC := c.Lo, c.Hi
		if d.Hi < 0 {
			minC, maxC = maxC, minC
		}
		minD, maxD := d.Lo, d.Hi
		if minC > 0 {
			minD = d.Hi
		}
		if maxC > 0 {
			maxD = d.Lo
		}
		corners[0], corners[1] = [2]float64{minC, minD}, [2]float64{maxC, maxD}
		count = 2
	case c.Lo > 0 || c.Hi < 0:
		minD, maxD := d.Hi, d.Lo
		if c.Hi < 0 {
			minD, maxD = maxD, minD
		}
		minC, maxC := c.Hi, c.Lo
		if minD > 0 {
			minC = c.Lo
		}
		if maxD > 0 {
			maxC = c.Hi
		}
		corners[0], corners[1] = [2]float64{minC, minD}, [2]float64{maxC, maxD}
		count = 2
	}
	var out Interval
	for i, cd := range corners[:count] {
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

// certifiedArcSpanBounds follows geom.Arc's radius and counter-clockwise
// angular branch. Point subtraction, direction angles, trigonometry and the
// final coordinates all carry outward-rounded intervals.
func certifiedArcSpanBounds(a *Arc, lo, hi float64) (x, y Interval, err error) {
	if a == nil || a.s == nil || !a.s.ownsEntity(a) || a.Center == nil ||
		a.Start == nil || a.End == nil || !a.s.owns(a.Center) ||
		!a.s.owns(a.Start) || !a.s.owns(a.End) || a.IsStale() {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc is not current", ErrNotCertified)
	}
	if math.IsNaN(lo) || math.IsNaN(hi) || math.IsInf(lo, 0) || math.IsInf(hi, 0) ||
		lo < 0 || hi > 1 || lo > hi {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc range [%g, %g] is invalid", ErrNotCertified, lo, hi)
	}
	cx, cy := a.Center.x(), a.Center.y()
	sx, sy := a.Start.x(), a.Start.y()
	ex, ey := a.End.x(), a.End.y()
	for _, v := range [...]float64{cx, cy, sx, sy, ex, ey} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Interval{}, Interval{}, fmt.Errorf("%w: arc has a non-finite point", ErrNonFiniteGeometry)
		}
	}
	startX, startY := isub(pt(sx), pt(cx)), isub(pt(sy), pt(cy))
	endX, endY := isub(pt(ex), pt(cx)), isub(pt(ey), pt(cy))
	start, ok := atan2Box(startY, startX)
	if !ok {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc start direction is undecidable", ErrNotCertified)
	}
	end, ok := atan2Box(endY, endX)
	if !ok {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc end direction is undecidable", ErrNotCertified)
	}
	// The float sweep chooses a whole-turn shift only. The exact interval
	// checks below decide whether that shift is the unique counter-clockwise
	// branch short of a full turn.
	shift := int64(math.Round((start.mid() + a.Sweep() - end.mid()) / (2 * math.Pi)))
	if shift < -2 || shift > 2 {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc turn is undecidable", ErrNotCertified)
	}
	delta := isub(iadd(end, twoPiTimes(shift)), start)
	if !delta.isFinite() || delta.Lo <= 0 || delta.Hi >= twoPiTimes(1).Lo {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc sweep is ambiguous or a full turn", ErrNotCertified)
	}
	radius := isqrtNonNeg(iadd(isqr(startX), isqr(startY)))
	if !radius.isFinite() || radius.Lo <= 0 {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc radius is undecidable", ErrNotCertified)
	}
	angles := iadd(start, imul(Interval{lo, hi}, delta))
	sin, cos, ok := sinCosRange(angles)
	if !ok {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc angle range is undecidable", ErrNotCertified)
	}
	x = iadd(pt(cx), imul(radius, cos))
	y = iadd(pt(cy), imul(radius, sin))
	if !x.isFinite() || !y.isFinite() {
		return Interval{}, Interval{}, fmt.Errorf("%w: arc coordinate range is undecidable", ErrNotCertified)
	}
	return x, y, nil
}
