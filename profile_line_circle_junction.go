package sketch

import (
	"context"
	"errors"
	"math"
	"math/big"

	"github.com/lestrrat-3d/sketch/geom"
)

// ErrJunctionWorkLimit reports that a Circle-only junction proof exhausted its
// fixed arithmetic budget without establishing a certificate or a definite miss.
var ErrJunctionWorkLimit = errors.New("sketch: junction proof work limit")

// ErrInvalidJunctionContext reports a nil proof context.
var ErrInvalidJunctionContext = errors.New("sketch: nil junction context")

var errJunctionMiss = errors.New("sketch: uncertifiable junction")
var errJunctionNeedMore = errors.New("sketch: junction interval needs refinement")

// RationalBracket encloses an exact value between two rational bounds. Bounds
// returns independent copies, so callers cannot change a certificate.
type RationalBracket struct{ lo, hi *big.Rat }

// Bounds returns fresh copies of the lower and upper bounds.
func (b RationalBracket) Bounds() (lo, hi *big.Rat) {
	if b.lo == nil || b.hi == nil {
		return nil, nil
	}
	return new(big.Rat).Set(b.lo), new(big.Rat).Set(b.hi)
}

// JunctionSourceKind names the two source kinds this certificate supports.
type JunctionSourceKind uint8

const (
	// JunctionLine identifies a line source.
	JunctionLine JunctionSourceKind = iota + 1
	// JunctionCircle identifies a complete circle source.
	JunctionCircle
	// JunctionArc identifies a first-quadrant quarter-arc source.
	JunctionArc
)

// JunctionSource records one authenticated source and its directed edge range.
type JunctionSource struct {
	Index     int
	Kind      JunctionSourceKind
	TStart    float64
	TEnd      float64
	Reversed  bool
	AtWalkEnd bool
}

// LineCircleJunction is a revision-bound proof of one paired line-circle
// secant. It authenticates source and parameter enclosures, not a representable
// output vertex or a downstream solid construction.
type LineCircleJunction struct {
	sources [2]JunctionSource
	point   [2]RationalBracket
	params  [2]RationalBracket
}

// Sources returns the two boundary sources in walked order.
func (w LineCircleJunction) Sources() [2]JunctionSource { return w.sources }

// Point returns exact rational enclosures for the crossing coordinates.
func (w LineCircleJunction) Point() (x, y RationalBracket) { return w.point[0], w.point[1] }

// Parameters returns enclosures in the boundary sources' natural domains.
func (w LineCircleJunction) Parameters() (left, right RationalBracket) {
	return w.params[0], w.params[1]
}

type junctionLimits struct {
	rootRounds       int
	atanTerms        int
	endpointBits     int
	intermediateBits int
}

var standardJunctionLimits = junctionLimits{128, 128, 256, 8192}

// A proof copies at most 256 unrelated source boxes (8 KiB of box values).
// Larger scenes exhaust this proof's work budget before any source is skipped.
const maxJunctionOtherSources = 256

type junctionInput struct {
	line                 [4]float64
	circle               [3]float64
	event                [2]float64
	lineTrim, circleTrim float64
	branch               int8
	otherBounds          [][4]float64
}

// CertifyLineCircleJunction certifies a strict interior Line/Circle secant at
// the walked end of left and start of right. The edges must be unchanged,
// partial, exact snapshots from this Sketch's current Profiles or Chains
// arrangement. A geometric miss returns (zero, false, nil); cancellation and
// bounded proof exhaustion return their respective errors. Arc is unsupported.
func (s *Sketch) CertifyLineCircleJunction(ctx context.Context, left, right BoundaryEdge) (LineCircleJunction, bool, error) {
	if ctx == nil {
		return LineCircleJunction{}, false, ErrInvalidJunctionContext
	}
	if err := ctx.Err(); err != nil {
		return LineCircleJunction{}, false, err
	}
	if s == nil || left.sketch != s || right.sketch != s ||
		left.revision != right.revision || left.revision != s.Revision() ||
		left.junctionWalk == nil || left.junctionWalk != right.junctionWalk ||
		(left.junctionIndex+1 != right.junctionIndex &&
			(!left.junctionWalk.closed || left.junctionIndex != left.junctionWalk.length-1 || right.junctionIndex != 0)) ||
		left.circleEnd == nil || left.circleEnd != right.circleStart ||
		!left.matchesSnapshot() || !right.matchesSnapshot() ||
		!left.Partial || !right.Partial || !left.TExact || !right.TExact ||
		left.Entity != left.sourceEntity || right.Entity != right.sourceEntity ||
		len(left.Polyline) < 2 || len(right.Polyline) < 2 {
		return LineCircleJunction{}, false, nil
	}
	cross := left.circleEnd
	pair := cross.SourceIndices()
	if left.sourceIndex == right.sourceIndex ||
		(left.sourceIndex != pair[0] && left.sourceIndex != pair[1]) ||
		(right.sourceIndex != pair[0] && right.sourceIndex != pair[1]) {
		return LineCircleJunction{}, false, nil
	}
	event := cross.Point()
	if left.Polyline[len(left.Polyline)-1] != event || right.Polyline[0] != event {
		return LineCircleJunction{}, false, nil
	}
	otherBounds, err := copyJunctionOtherBounds(ctx, cross, maxJunctionOtherSources)
	if err != nil {
		if errors.Is(err, errJunctionMiss) {
			return LineCircleJunction{}, false, nil
		}
		return LineCircleJunction{}, false, err
	}
	lineEdge, circleEdge := left, right
	if left.sourceIndex == pair[1] {
		lineEdge, circleEdge = right, left
	}
	line, lineOK := lineEdge.Entity.(*Line)
	circle, circleOK := circleEdge.Entity.(*Circle)
	if !lineOK || !circleOK || !s.ownsEntity(line) || !s.ownsEntity(circle) {
		return LineCircleJunction{}, false, nil
	}
	in := junctionInput{
		line:   [4]float64{line.Start.X(), line.Start.Y(), line.End.X(), line.End.Y()},
		circle: [3]float64{circle.Center.X(), circle.Center.Y(), circle.R()},
		event:  event, branch: cross.Branch(), otherBounds: otherBounds,
		lineTrim:   walkedJunctionParam(lineEdge, lineEdge.sourceIndex == left.sourceIndex),
		circleTrim: walkedJunctionParam(circleEdge, circleEdge.sourceIndex == left.sourceIndex),
	}
	point, params, err := certifyLineCircleProof(ctx, in, standardJunctionLimits)
	if err != nil {
		if errors.Is(err, errJunctionMiss) {
			return LineCircleJunction{}, false, nil
		}
		return LineCircleJunction{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return LineCircleJunction{}, false, err
	}
	makeSource := func(e BoundaryEdge, atEnd bool) JunctionSource {
		kind := JunctionLine
		if _, ok := e.Entity.(*Circle); ok {
			kind = JunctionCircle
		}
		return JunctionSource{
			Index: e.sourceIndex, Kind: kind, TStart: e.TStart, TEnd: e.TEnd,
			Reversed: e.Reversed, AtWalkEnd: atEnd,
		}
	}
	w := LineCircleJunction{
		sources: [2]JunctionSource{makeSource(left, true), makeSource(right, false)},
		point:   point,
	}
	if left.sourceIndex == pair[0] {
		w.params = params
	} else {
		w.params = [2]RationalBracket{params[1], params[0]}
	}
	return w, true, nil
}

func copyJunctionOtherBounds(ctx context.Context, cross *geom.LineCircleCrossing,
	limit int) ([][4]float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pair := cross.SourceIndices()
	count := cross.SourceCount()
	if count < 2 || pair[0] < 0 || pair[1] < 0 || pair[0] == pair[1] ||
		pair[0] >= count || pair[1] >= count {
		return nil, errJunctionMiss
	}
	if limit < 0 || count-2 > limit {
		return nil, ErrJunctionWorkLimit
	}
	out := make([][4]float64, 0, count-2)
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i == pair[0] || i == pair[1] {
			continue
		}
		box, ok := cross.SourceBound(i)
		if !ok {
			return nil, errJunctionMiss
		}
		out = append(out, box)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func walkedJunctionParam(e BoundaryEdge, atEnd bool) float64 {
	if atEnd != e.Reversed {
		return e.TEnd
	}
	return e.TStart
}

type junctionMath struct {
	cancelErr func() error
	limits    junctionLimits
}

func (m junctionMath) check() error {
	if err := m.cancelErr(); err != nil {
		return err
	}
	return nil
}

func (m junctionMath) finiteRat(v float64) (*big.Rat, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errJunctionMiss
	}
	r := new(big.Rat).SetFloat64(v)
	if !m.fits(r) {
		return nil, ErrJunctionWorkLimit
	}
	return r, nil
}

func (m junctionMath) fits(r *big.Rat) bool {
	return r.Num().BitLen() <= m.limits.intermediateBits &&
		r.Denom().BitLen() <= m.limits.intermediateBits
}

func (m junctionMath) add(a, b *big.Rat) (*big.Rat, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	gcd := new(big.Int).GCD(nil, nil, a.Denom(), b.Denom())
	aFactor := new(big.Int).Quo(a.Denom(), gcd)
	bFactor := new(big.Int).Quo(b.Denom(), gcd)
	if max(a.Num().BitLen()+bFactor.BitLen(), b.Num().BitLen()+aFactor.BitLen())+1 > m.limits.intermediateBits ||
		aFactor.BitLen()+b.Denom().BitLen() > m.limits.intermediateBits {
		return nil, ErrJunctionWorkLimit
	}
	r := new(big.Rat).Add(a, b)
	if !m.fits(r) {
		return nil, ErrJunctionWorkLimit
	}
	return r, nil
}

func (m junctionMath) sub(a, b *big.Rat) (*big.Rat, error) {
	return m.add(a, new(big.Rat).Neg(b))
}

func (m junctionMath) mul(a, b *big.Rat) (*big.Rat, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	if a.Num().BitLen()+b.Num().BitLen() > m.limits.intermediateBits ||
		a.Denom().BitLen()+b.Denom().BitLen() > m.limits.intermediateBits {
		return nil, ErrJunctionWorkLimit
	}
	r := new(big.Rat).Mul(a, b)
	if !m.fits(r) {
		return nil, ErrJunctionWorkLimit
	}
	return r, nil
}

func (m junctionMath) div(a, b *big.Rat) (*big.Rat, error) {
	if b.Sign() == 0 {
		return nil, errJunctionMiss
	}
	if err := m.check(); err != nil {
		return nil, err
	}
	if a.Num().BitLen()+b.Denom().BitLen() > m.limits.intermediateBits ||
		a.Denom().BitLen()+b.Num().BitLen() > m.limits.intermediateBits {
		return nil, ErrJunctionWorkLimit
	}
	r := new(big.Rat).Quo(a, b)
	if !m.fits(r) {
		return nil, ErrJunctionWorkLimit
	}
	return r, nil
}

type junctionInterval struct{ lo, hi *big.Rat }

func ratInt(v int64) *big.Rat { return new(big.Rat).SetInt64(v) }

func (m junctionMath) dyadic(v *big.Rat, bits int, upper bool) (*big.Rat, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	if bits < 0 || v.Num().BitLen()+bits > m.limits.intermediateBits {
		return nil, ErrJunctionWorkLimit
	}
	den := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	num := new(big.Int).Lsh(new(big.Int).Set(v.Num()), uint(bits))
	q, rem := new(big.Int).DivMod(num, v.Denom(), new(big.Int))
	if upper && rem.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	if q.BitLen() > m.limits.endpointBits || den.BitLen() > m.limits.endpointBits {
		return nil, ErrJunctionWorkLimit
	}
	return new(big.Rat).SetFrac(q, den), nil
}

func (m junctionMath) round(lo, hi *big.Rat, bits int) (junctionInterval, error) {
	l, err := m.dyadic(lo, bits, false)
	if err != nil {
		return junctionInterval{}, err
	}
	h, err := m.dyadic(hi, bits, true)
	if err != nil {
		return junctionInterval{}, err
	}
	return junctionInterval{l, h}, nil
}

func (m junctionMath) sqrtInterval(v *big.Rat, bits int) (junctionInterval, error) {
	if v.Sign() < 0 || bits < 0 {
		return junctionInterval{}, errJunctionMiss
	}
	if err := m.check(); err != nil {
		return junctionInterval{}, err
	}
	if v.Num().BitLen()+2*bits > m.limits.intermediateBits {
		return junctionInterval{}, ErrJunctionWorkLimit
	}
	scaled := new(big.Int).Lsh(new(big.Int).Set(v.Num()), uint(2*bits))
	quotient := new(big.Int).Quo(scaled, v.Denom())
	root := new(big.Int).Sqrt(quotient)
	den := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	lo := new(big.Rat).SetFrac(new(big.Int).Set(root), den)
	if root.BitLen() > m.limits.endpointBits || den.BitLen() > m.limits.endpointBits {
		return junctionInterval{}, ErrJunctionWorkLimit
	}
	if 2*root.BitLen()+v.Denom().BitLen() <= m.limits.intermediateBits {
		squared := new(big.Int).Mul(new(big.Int).Set(root), root)
		squared.Mul(squared, v.Denom())
		if squared.Cmp(scaled) == 0 {
			return junctionInterval{lo, new(big.Rat).Set(lo)}, nil
		}
	}
	hi := new(big.Rat).SetFrac(new(big.Int).Add(root, big.NewInt(1)), den)
	if lo.Num().BitLen() > m.limits.endpointBits || hi.Num().BitLen() > m.limits.endpointBits ||
		lo.Denom().BitLen() > m.limits.endpointBits || hi.Denom().BitLen() > m.limits.endpointBits {
		return junctionInterval{}, ErrJunctionWorkLimit
	}
	return junctionInterval{lo, hi}, nil
}

func (m junctionMath) quadraticRoot(b, twoA *big.Rat, sqrt junctionInterval, branch int8) (junctionInterval, error) {
	minusB := new(big.Rat).Neg(b)
	var loNum, hiNum *big.Rat
	var err error
	if branch < 0 {
		loNum, err = m.sub(minusB, sqrt.hi)
		if err != nil {
			return junctionInterval{}, err
		}
		hiNum, err = m.sub(minusB, sqrt.lo)
	} else {
		loNum, err = m.add(minusB, sqrt.lo)
		if err != nil {
			return junctionInterval{}, err
		}
		hiNum, err = m.add(minusB, sqrt.hi)
	}
	if err != nil {
		return junctionInterval{}, err
	}
	lo, err := m.div(loNum, twoA)
	if err != nil {
		return junctionInterval{}, err
	}
	hi, err := m.div(hiNum, twoA)
	if err != nil {
		return junctionInterval{}, err
	}
	return m.round(lo, hi, 128)
}

func (m junctionMath) affine(base, delta *big.Rat, t junctionInterval) (junctionInterval, error) {
	loT, hiT := t.lo, t.hi
	if delta.Sign() < 0 {
		loT, hiT = hiT, loT
	}
	loMul, err := m.mul(delta, loT)
	if err != nil {
		return junctionInterval{}, err
	}
	hiMul, err := m.mul(delta, hiT)
	if err != nil {
		return junctionInterval{}, err
	}
	lo, err := m.add(base, loMul)
	if err != nil {
		return junctionInterval{}, err
	}
	hi, err := m.add(base, hiMul)
	if err != nil {
		return junctionInterval{}, err
	}
	return m.round(lo, hi, 128)
}

func (m junctionMath) subInterval(a, b junctionInterval) (junctionInterval, error) {
	lo, err := m.sub(a.lo, b.hi)
	if err != nil {
		return junctionInterval{}, err
	}
	hi, err := m.sub(a.hi, b.lo)
	if err != nil {
		return junctionInterval{}, err
	}
	return m.round(lo, hi, 128)
}

func (m junctionMath) addInterval(a, b junctionInterval) (junctionInterval, error) {
	lo, err := m.add(a.lo, b.lo)
	if err != nil {
		return junctionInterval{}, err
	}
	hi, err := m.add(a.hi, b.hi)
	if err != nil {
		return junctionInterval{}, err
	}
	return m.round(lo, hi, 128)
}

func (m junctionMath) mulPositive(a, b junctionInterval) (junctionInterval, error) {
	if a.lo.Sign() < 0 || b.lo.Sign() < 0 {
		return junctionInterval{}, errJunctionMiss
	}
	lo, err := m.mul(a.lo, b.lo)
	if err != nil {
		return junctionInterval{}, err
	}
	hi, err := m.mul(a.hi, b.hi)
	if err != nil {
		return junctionInterval{}, err
	}
	return m.round(lo, hi, 128)
}

func (m junctionMath) divPositive(a, b junctionInterval, bits int) (junctionInterval, error) {
	if a.lo.Sign() < 0 || b.lo.Sign() <= 0 {
		return junctionInterval{}, errJunctionMiss
	}
	lo, err := m.div(a.lo, b.hi)
	if err != nil {
		return junctionInterval{}, err
	}
	hi, err := m.div(a.hi, b.lo)
	if err != nil {
		return junctionInterval{}, err
	}
	return m.round(lo, hi, bits)
}

func (m junctionMath) hullFloat(box junctionInterval, v float64) (junctionInterval, error) {
	r, err := m.finiteRat(v)
	if err != nil {
		return junctionInterval{}, err
	}
	lo, hi := box.lo, box.hi
	if r.Cmp(lo) < 0 {
		lo = r
	}
	if r.Cmp(hi) > 0 {
		hi = r
	}
	return m.round(lo, hi, 128)
}

func (m junctionMath) widthCheck(box junctionInterval, reported float64, limit *big.Rat) (junctionInterval, error) {
	r, err := m.finiteRat(reported)
	if err != nil {
		return junctionInterval{}, err
	}
	// A proven separation beyond the fixed width is a geometric miss even if
	// more arithmetic work remains available.
	if r.Cmp(box.lo) < 0 {
		gap, err := m.sub(box.lo, r)
		if err != nil {
			return junctionInterval{}, err
		}
		if gap.Cmp(limit) > 0 {
			return junctionInterval{}, errJunctionMiss
		}
	} else if r.Cmp(box.hi) > 0 {
		gap, err := m.sub(r, box.hi)
		if err != nil {
			return junctionInterval{}, err
		}
		if gap.Cmp(limit) > 0 {
			return junctionInterval{}, errJunctionMiss
		}
	}
	hull, err := m.hullFloat(box, reported)
	if err != nil {
		return junctionInterval{}, err
	}
	width, err := m.sub(hull.hi, hull.lo)
	if err != nil {
		return junctionInterval{}, err
	}
	if width.Cmp(limit) > 0 {
		return junctionInterval{}, errJunctionNeedMore
	}
	return hull, nil
}

func intervalBracket(i junctionInterval) RationalBracket {
	return RationalBracket(i)
}

func (m junctionMath) atanSeries(z *big.Rat) (junctionInterval, error) {
	if z.Sign() < 0 || z.Cmp(new(big.Rat).SetFrac64(1, 2)) > 0 {
		return junctionInterval{}, errJunctionMiss
	}
	n := min(48, m.limits.atanTerms)
	n -= n % 2
	if n < 2 {
		return junctionInterval{}, ErrJunctionWorkLimit
	}
	z2, err := m.mul(z, z)
	if err != nil {
		return junctionInterval{}, err
	}
	power, sum := new(big.Rat).Set(z), ratInt(0)
	for k := 0; k < n; k++ {
		if err := m.check(); err != nil {
			return junctionInterval{}, err
		}
		term, err := m.div(power, ratInt(int64(2*k+1)))
		if err != nil {
			return junctionInterval{}, err
		}
		if k%2 == 0 {
			sum, err = m.add(sum, term)
		} else {
			sum, err = m.sub(sum, term)
		}
		if err != nil {
			return junctionInterval{}, err
		}
		power, err = m.mul(power, z2)
		if err != nil {
			return junctionInterval{}, err
		}
	}
	// An even alternating partial sum is below atan(z); the next positive
	// term bounds the omitted tail above it.
	tail, err := m.div(power, ratInt(int64(2*n+1)))
	if err != nil {
		return junctionInterval{}, err
	}
	hi, err := m.add(sum, tail)
	if err != nil {
		return junctionInterval{}, err
	}
	return m.round(sum, hi, 128)
}

func (m junctionMath) scalePositive(a junctionInterval, n int64) (junctionInterval, error) {
	lo, err := m.mul(a.lo, ratInt(n))
	if err != nil {
		return junctionInterval{}, err
	}
	hi, err := m.mul(a.hi, ratInt(n))
	if err != nil {
		return junctionInterval{}, err
	}
	return m.round(lo, hi, 128)
}

func (m junctionMath) piInterval() (junctionInterval, error) {
	a, err := m.atanSeries(new(big.Rat).SetFrac64(1, 5))
	if err != nil {
		return junctionInterval{}, err
	}
	b, err := m.atanSeries(new(big.Rat).SetFrac64(1, 239))
	if err != nil {
		return junctionInterval{}, err
	}
	a, err = m.scalePositive(a, 16)
	if err != nil {
		return junctionInterval{}, err
	}
	b, err = m.scalePositive(b, 4)
	if err != nil {
		return junctionInterval{}, err
	}
	return m.subInterval(a, b)
}

func (m junctionMath) angleParameter(point [2]junctionInterval, center [2]*big.Rat,
	pi junctionInterval) (junctionInterval, error) {
	x, err := m.subInterval(point[0], junctionInterval{center[0], center[0]})
	if err != nil {
		return junctionInterval{}, err
	}
	y, err := m.subInterval(point[1], junctionInterval{center[1], center[1]})
	if err != nil {
		return junctionInterval{}, err
	}
	if x.lo.Sign() == 0 && x.hi.Sign() == 0 || y.lo.Sign() == 0 && y.hi.Sign() == 0 {
		return junctionInterval{}, errJunctionMiss
	}
	if x.lo.Sign() <= 0 && x.hi.Sign() >= 0 || y.lo.Sign() <= 0 && y.hi.Sign() >= 0 {
		return junctionInterval{}, errJunctionNeedMore
	}
	xNeg, yNeg := x.hi.Sign() < 0, y.hi.Sign() < 0
	if xNeg {
		x = junctionInterval{new(big.Rat).Neg(x.hi), new(big.Rat).Neg(x.lo)}
	}
	if yNeg {
		y = junctionInterval{new(big.Rat).Neg(y.hi), new(big.Rat).Neg(y.lo)}
	}
	swapped := false
	var z junctionInterval
	switch {
	case x.hi.Cmp(y.lo) < 0:
		swapped = true
		z, err = m.divPositive(x, y, 64)
	case y.hi.Cmp(x.lo) < 0:
		z, err = m.divPositive(y, x, 64)
	default:
		return junctionInterval{}, errJunctionNeedMore
	}
	if err != nil {
		return junctionInterval{}, err
	}
	if z.lo.Sign() < 0 || z.hi.Cmp(ratInt(1)) > 0 {
		return junctionInterval{}, errJunctionMiss
	}
	loSquare, err := m.mul(z.lo, z.lo)
	if err != nil {
		return junctionInterval{}, err
	}
	hiSquare, err := m.mul(z.hi, z.hi)
	if err != nil {
		return junctionInterval{}, err
	}
	loRadicand, err := m.add(ratInt(1), loSquare)
	if err != nil {
		return junctionInterval{}, err
	}
	hiRadicand, err := m.add(ratInt(1), hiSquare)
	if err != nil {
		return junctionInterval{}, err
	}
	loRoot, err := m.sqrtInterval(loRadicand, 128)
	if err != nil {
		return junctionInterval{}, err
	}
	hiRoot, err := m.sqrtInterval(hiRadicand, 128)
	if err != nil {
		return junctionInterval{}, err
	}
	den, err := m.addInterval(junctionInterval{ratInt(1), ratInt(1)},
		junctionInterval{loRoot.lo, hiRoot.hi})
	if err != nil {
		return junctionInterval{}, err
	}
	half, err := m.divPositive(z, den, 64)
	if err != nil {
		return junctionInterval{}, err
	}
	loAtan, err := m.atanSeries(half.lo)
	if err != nil {
		return junctionInterval{}, err
	}
	hiAtan, err := m.atanSeries(half.hi)
	if err != nil {
		return junctionInterval{}, err
	}
	quarter, err := m.scalePositive(junctionInterval{loAtan.lo, hiAtan.hi}, 2)
	if err != nil {
		return junctionInterval{}, err
	}
	if swapped {
		halfPi, err := m.divPositive(pi, junctionInterval{ratInt(2), ratInt(2)}, 128)
		if err != nil {
			return junctionInterval{}, err
		}
		quarter, err = m.subInterval(halfPi, quarter)
		if err != nil {
			return junctionInterval{}, err
		}
	}
	var theta junctionInterval
	switch {
	case !xNeg && !yNeg:
		theta = quarter
	case xNeg && !yNeg:
		theta, err = m.subInterval(pi, quarter)
	case xNeg && yNeg:
		theta, err = m.addInterval(pi, quarter)
	default:
		twoPi, e := m.scalePositive(pi, 2)
		if e != nil {
			return junctionInterval{}, e
		}
		theta, err = m.subInterval(twoPi, quarter)
	}
	if err != nil {
		return junctionInterval{}, err
	}
	twoPi, err := m.scalePositive(pi, 2)
	if err != nil {
		return junctionInterval{}, err
	}
	normalized, err := m.divPositive(theta, twoPi, 128)
	if err != nil {
		return junctionInterval{}, err
	}
	if normalized.lo.Sign() <= 0 || normalized.hi.Cmp(ratInt(1)) >= 0 {
		return junctionInterval{}, errJunctionMiss
	}
	return normalized, nil
}

func certifyLineCircleProof(ctx context.Context, in junctionInput,
	limits junctionLimits) ([2]RationalBracket, [2]RationalBracket, error) {
	var emptyPoint, emptyParams [2]RationalBracket
	m := junctionMath{cancelErr: ctx.Err, limits: limits}
	if err := m.check(); err != nil {
		return emptyPoint, emptyParams, err
	}
	if len(in.otherBounds) > maxJunctionOtherSources {
		return emptyPoint, emptyParams, ErrJunctionWorkLimit
	}
	if in.branch != -1 && in.branch != 1 {
		return emptyPoint, emptyParams, errJunctionMiss
	}
	var r [7]*big.Rat
	for i, v := range [...]float64{
		in.line[0], in.line[1], in.line[2], in.line[3],
		in.circle[0], in.circle[1], in.circle[2],
	} {
		var err error
		r[i], err = m.finiteRat(v)
		if err != nil {
			return emptyPoint, emptyParams, err
		}
	}
	if r[6].Sign() <= 0 {
		return emptyPoint, emptyParams, errJunctionMiss
	}
	dx, err := m.sub(r[2], r[0])
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	dy, err := m.sub(r[3], r[1])
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	ox, err := m.sub(r[0], r[4])
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	oy, err := m.sub(r[1], r[5])
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	dx2, err := m.mul(dx, dx)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	dy2, err := m.mul(dy, dy)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	a, err := m.add(dx2, dy2)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	if a.Sign() <= 0 {
		return emptyPoint, emptyParams, errJunctionMiss
	}
	bdx, err := m.mul(dx, ox)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	bdy, err := m.mul(dy, oy)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	bHalf, err := m.add(bdx, bdy)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	b, err := m.mul(ratInt(2), bHalf)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	ox2, err := m.mul(ox, ox)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	oy2, err := m.mul(oy, oy)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	c, err := m.add(ox2, oy2)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	r2, err := m.mul(r[6], r[6])
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	c, err = m.sub(c, r2)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	b2, err := m.mul(b, b)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	ac, err := m.mul(a, c)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	fourAC, err := m.mul(ratInt(4), ac)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	discriminant, err := m.sub(b2, fourAC)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	if discriminant.Sign() <= 0 {
		return emptyPoint, emptyParams, errJunctionMiss
	}
	twoA, err := m.mul(ratInt(2), a)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	scale := ratInt(1)
	for _, v := range r {
		abs := new(big.Rat).Abs(v)
		if abs.Cmp(scale) > 0 {
			scale = abs
		}
	}
	pointLimit, err := m.div(scale, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 48)))
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	paramLimit := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 48))
	pi, err := m.piInterval()
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	if limits.rootRounds <= 0 {
		return emptyPoint, emptyParams, ErrJunctionWorkLimit
	}
	for bits := 1; bits <= limits.rootRounds && bits <= 128; bits++ {
		if err := m.check(); err != nil {
			return emptyPoint, emptyParams, err
		}
		sqrt, err := m.sqrtInterval(discriminant, bits)
		if err != nil {
			return emptyPoint, emptyParams, err
		}
		low, err := m.quadraticRoot(b, twoA, sqrt, -1)
		if err != nil {
			return emptyPoint, emptyParams, err
		}
		high, err := m.quadraticRoot(b, twoA, sqrt, 1)
		if err != nil {
			return emptyPoint, emptyParams, err
		}
		if bits < 64 && bits < limits.rootRounds {
			continue
		}
		selected, other := low, high
		if in.branch > 0 {
			selected, other = high, low
		}
		point, params, err := m.certifyRoot(in, r, dx, dy, selected, other,
			pointLimit, paramLimit, pi)
		switch {
		case err == nil:
			return point, params, nil
		case errors.Is(err, errJunctionNeedMore):
			continue
		default:
			return emptyPoint, emptyParams, err
		}
	}
	if err := ctx.Err(); err != nil {
		return emptyPoint, emptyParams, err
	}
	return emptyPoint, emptyParams, ErrJunctionWorkLimit
}

func (m junctionMath) certifyRoot(in junctionInput, r [7]*big.Rat, dx, dy *big.Rat,
	selected, other junctionInterval, pointLimit, paramLimit *big.Rat,
	pi junctionInterval) ([2]RationalBracket, [2]RationalBracket, error) {
	var emptyPoint, emptyParams [2]RationalBracket
	zero, one := ratInt(0), ratInt(1)
	if selected.hi.Cmp(zero) <= 0 || selected.lo.Cmp(one) >= 0 {
		return emptyPoint, emptyParams, errJunctionMiss
	}
	if selected.lo.Cmp(zero) <= 0 || selected.hi.Cmp(one) >= 0 ||
		selected.lo.Cmp(other.hi) <= 0 && selected.hi.Cmp(other.lo) >= 0 {
		return emptyPoint, emptyParams, errJunctionNeedMore
	}
	lineParam, err := m.widthCheck(selected, in.lineTrim, paramLimit)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	px, err := m.affine(r[0], dx, selected)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	py, err := m.affine(r[1], dy, selected)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	px, err = m.widthCheck(px, in.event[0], pointLimit)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	py, err = m.widthCheck(py, in.event[1], pointLimit)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	if err := m.checkOtherSources(px, py, in.otherBounds); err != nil {
		return emptyPoint, emptyParams, err
	}
	otherX, err := m.affine(r[0], dx, other)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	otherY, err := m.affine(r[1], dy, other)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	if px.lo.Cmp(otherX.hi) <= 0 && px.hi.Cmp(otherX.lo) >= 0 &&
		py.lo.Cmp(otherY.hi) <= 0 && py.hi.Cmp(otherY.lo) >= 0 {
		return emptyPoint, emptyParams, errJunctionNeedMore
	}
	center := [2]*big.Rat{r[4], r[5]}
	circleParam, err := m.angleParameter([2]junctionInterval{px, py}, center, pi)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	circleParam, err = m.widthCheck(circleParam, in.circleTrim, paramLimit)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	otherCircleParam, err := m.angleParameter([2]junctionInterval{otherX, otherY}, center, pi)
	if err != nil {
		return emptyPoint, emptyParams, err
	}
	if circleParam.lo.Cmp(otherCircleParam.hi) <= 0 &&
		circleParam.hi.Cmp(otherCircleParam.lo) >= 0 {
		return emptyPoint, emptyParams, errJunctionNeedMore
	}
	return [2]RationalBracket{intervalBracket(px), intervalBracket(py)},
		[2]RationalBracket{intervalBracket(lineParam), intervalBracket(circleParam)}, nil
}

func (m junctionMath) checkOtherSources(px, py junctionInterval, boxes [][4]float64) error {
	if err := m.check(); err != nil {
		return err
	}
	if len(boxes) > maxJunctionOtherSources {
		return ErrJunctionWorkLimit
	}
	for _, box := range boxes {
		if err := m.check(); err != nil {
			return err
		}
		var bounds [4]*big.Rat
		for i, v := range box {
			var err error
			bounds[i], err = m.finiteRat(v)
			if err != nil {
				return err
			}
		}
		if px.hi.Cmp(bounds[0]) >= 0 && px.lo.Cmp(bounds[2]) <= 0 &&
			py.hi.Cmp(bounds[1]) >= 0 && py.lo.Cmp(bounds[3]) <= 0 {
			return errJunctionMiss
		}
	}
	return m.check()
}
