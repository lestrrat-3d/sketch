package sketch

import (
	"errors"
	"math"
	"math/big"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch/geom"
)

// ErrConeFitCrossing means that one transverse crossing could not be proved.
var ErrConeFitCrossing = errors.New("sketch: fit spline lacks one certified cone crossing")

// ConeFitCrossing certifies one transverse crossing of an ideal fitted spline
// and a positive cone section. TBounds uses FitSpline's natural normalized
// parameter. The proof treats the stored binary64 interpolant coefficients
// as exact rationals; a consumer must charge the difference between that
// ideal curve and floating-point evaluation or recorded geometry separately.
// IsStale checks the source points, sketch, and plane frame.
type ConeFitCrossing struct {
	spline           *FitSpline
	points           []*Point
	cone             ConeSection
	sketch           *Sketch
	frame            r3.Frame
	revision         uint64
	pieces           []coneFitPiece
	root             int
	localLo, localHi *big.Rat
	t, lo, hi        float64
	before, after    ConeSide
}

// Spline returns the certified source entity.
func (c ConeFitCrossing) Spline() *FitSpline { return c.spline }

// Cone returns the cone supplied to certification.
func (c ConeFitCrossing) Cone() ConeSection { return c.cone }

// SketchRevision returns the source sketch revision.
func (c ConeFitCrossing) SketchRevision() uint64 { return c.revision }

// T returns a rounded parameter inside the certified crossing bracket.
func (c ConeFitCrossing) T() float64 { return c.t }

// TBounds encloses the unique crossing in FitSpline's normalized parameter.
func (c ConeFitCrossing) TBounds() (float64, float64) { return c.lo, c.hi }

// BeforeSide returns the strict side occupied before the crossing, or zero
// when the source certificate is stale.
func (c ConeFitCrossing) BeforeSide() ConeSide {
	if c.IsStale() {
		return 0
	}
	return c.before
}

// AfterSide returns the strict side occupied after the crossing, or zero
// when the source certificate is stale.
func (c ConeFitCrossing) AfterSide() ConeSide {
	if c.IsStale() {
		return 0
	}
	return c.after
}

// IsStale reports whether the source curve or its sketch has changed.
func (c ConeFitCrossing) IsStale() bool {
	if c.sketch == nil || c.spline == nil || !c.sketch.ownsConeCurve(c.spline) ||
		c.sketch.Revision() != c.revision || len(c.spline.Fit) != len(c.points) {
		return true
	}
	for i, point := range c.points {
		if c.spline.Fit[i] != point {
			return true
		}
	}
	frame, err := c.sketch.Plane().Frame()
	return err != nil || frame.Origin() != c.frame.Origin() ||
		frame.U() != c.frame.U() || frame.V() != c.frame.V()
}

type coneFitPoly []*big.Rat

type coneFitPiece struct {
	p0, p1, total *big.Rat
	xy            [2]coneFitPoly
	h, f, df      coneFitPoly
}

func coneFitZero() *big.Rat { return new(big.Rat) }

func coneFitPolyAdd(a, b coneFitPoly) coneFitPoly {
	n := max(len(a), len(b))
	r := make(coneFitPoly, n)
	for i := range r {
		r[i] = coneFitZero()
		if i < len(a) {
			r[i].Add(r[i], a[i])
		}
		if i < len(b) {
			r[i].Add(r[i], b[i])
		}
	}
	return r
}

func coneFitPolySub(a, b coneFitPoly) coneFitPoly {
	n := max(len(a), len(b))
	r := make(coneFitPoly, n)
	for i := range r {
		r[i] = coneFitZero()
		if i < len(a) {
			r[i].Add(r[i], a[i])
		}
		if i < len(b) {
			r[i].Sub(r[i], b[i])
		}
	}
	return r
}

func coneFitPolyScale(a coneFitPoly, k *big.Rat) coneFitPoly {
	r := make(coneFitPoly, len(a))
	for i := range a {
		r[i] = coneMul(a[i], k)
	}
	return r
}

func coneFitPolyMul(a, b coneFitPoly) coneFitPoly {
	r := make(coneFitPoly, len(a)+len(b)-1)
	for i := range r {
		r[i] = coneFitZero()
	}
	for i, x := range a {
		for j, y := range b {
			r[i+j].Add(r[i+j], coneMul(x, y))
		}
	}
	return r
}

func coneFitPolyDeriv(a coneFitPoly) coneFitPoly {
	if len(a) < 2 {
		return coneFitPoly{coneFitZero()}
	}
	r := make(coneFitPoly, len(a)-1)
	for i := range r {
		r[i] = coneMul(a[i+1], big.NewRat(int64(i+1), 1))
	}
	return r
}

func coneFitPolyEval(a coneFitPoly, t *big.Rat) *big.Rat {
	r := coneFitZero()
	for i := len(a) - 1; i >= 0; i-- {
		r = coneAdd(coneMul(r, t), a[i])
	}
	return r
}

func coneFitBinomial(n, k int) int64 {
	r := int64(1)
	for i := 1; i <= k; i++ {
		r = r * int64(n-k+i) / int64(i)
	}
	return r
}

// coneFitBernstein returns exact degree-n Bernstein coefficients of a power
// polynomial on [a,b]. Their convex hull encloses every value on that range.
func coneFitBernstein(p coneFitPoly, a, b *big.Rat) coneFitPoly {
	n := len(p) - 1
	// Compose p(a+(b-a)u) by Horner's rule.
	affine := coneFitPoly{a, coneSub(b, a)}
	q := coneFitPoly{coneFitZero()}
	for i := n; i >= 0; i-- {
		q = coneFitPolyAdd(coneFitPolyMul(q, affine), coneFitPoly{p[i]})
	}
	r := make(coneFitPoly, n+1)
	for k := range r {
		r[k] = coneFitZero()
		for i := 0; i <= k; i++ {
			weight := big.NewRat(coneFitBinomial(k, i), coneFitBinomial(n, i))
			r[k].Add(r[k], coneMul(q[i], weight))
		}
	}
	return r
}

func coneFitCoeffBounds(p coneFitPoly, a, b *big.Rat) (*big.Rat, *big.Rat) {
	coeff := coneFitBernstein(p, a, b)
	lo, hi := coeff[0], coeff[0]
	for _, v := range coeff[1:] {
		if v.Cmp(lo) < 0 {
			lo = v
		}
		if v.Cmp(hi) > 0 {
			hi = v
		}
	}
	return lo, hi
}

func coneFitBezier(fi *geom.FitInterpolant, i int) coneBezier {
	h := coneSub(coneRat(fi.Params[i+1]), coneRat(fi.Params[i]))
	h2 := coneMul(h, h)
	var b coneBezier
	for axis := range 2 {
		p0, p1 := coneRat(fi.Points[i][axis]), coneRat(fi.Points[i+1][axis])
		m0, m1 := coneRat(fi.SecondDerivs[i][axis]), coneRat(fi.SecondDerivs[i+1][axis])
		c0 := p0
		c1 := coneSub(coneSub(p1, p0), coneMul(h2,
			coneMul(coneAdd(coneMul(coneRat(2), m0), m1), big.NewRat(1, 6))))
		c2 := coneMul(h2, coneMul(m0, big.NewRat(1, 2)))
		c3 := coneMul(h2, coneMul(coneSub(m1, m0), big.NewRat(1, 6)))
		b.controls[0][axis] = c0
		b.controls[1][axis] = coneAdd(c0, coneMul(c1, big.NewRat(1, 3)))
		b.controls[2][axis] = coneAdd(coneAdd(c0, coneMul(c1, big.NewRat(2, 3))), coneMul(c2, big.NewRat(1, 3)))
		b.controls[3][axis] = coneAdd(coneAdd(c0, c1), coneAdd(c2, c3))
	}
	return b
}

func coneFitPower(b coneBezier, axis int) coneFitPoly {
	p := [4]*big.Rat{b.controls[0][axis], b.controls[1][axis], b.controls[2][axis], b.controls[3][axis]}
	return coneFitPoly{
		p[0],
		coneMul(big.NewRat(3, 1), coneSub(p[1], p[0])),
		coneMul(big.NewRat(3, 1), coneAdd(coneSub(p[0], coneMul(big.NewRat(2, 1), p[1])), p[2])),
		coneAdd(coneSub(coneMul(big.NewRat(3, 1), p[1]), p[0]),
			coneSub(p[3], coneMul(big.NewRat(3, 1), p[2]))),
	}
}

func coneFitMakePiece(fi *geom.FitInterpolant, i int, form coneSideForm) coneFitPiece {
	b := coneFitBezier(fi, i)
	piece := coneFitPiece{p0: coneRat(fi.Params[i]), p1: coneRat(fi.Params[i+1]),
		total: coneRat(fi.Params[len(fi.Params)-1])}
	piece.xy = [2]coneFitPoly{coneFitPower(b, 0), coneFitPower(b, 1)}
	return coneFitResidual(piece, form)
}

func coneFitResidual(piece coneFitPiece, form coneSideForm) coneFitPiece {
	var q [3]coneFitPoly
	for j := range q {
		q[j] = coneFitPolyAdd(coneFitPoly{form.base[j]},
			coneFitPolyAdd(coneFitPolyScale(piece.xy[0], form.u[j]),
				coneFitPolyScale(piece.xy[1], form.v[j])))
	}
	piece.h = coneFitPolyAdd(coneFitPolyAdd(coneFitPolyScale(q[0], form.axis[0]),
		coneFitPolyScale(q[1], form.axis[1])), coneFitPolyScale(q[2], form.axis[2]))
	cx := coneFitPolySub(coneFitPolyScale(q[1], form.axis[2]), coneFitPolyScale(q[2], form.axis[1]))
	cy := coneFitPolySub(coneFitPolyScale(q[2], form.axis[0]), coneFitPolyScale(q[0], form.axis[2]))
	cz := coneFitPolySub(coneFitPolyScale(q[0], form.axis[1]), coneFitPolyScale(q[1], form.axis[0]))
	piece.f = coneFitPolySub(coneFitPolyAdd(coneFitPolyAdd(coneFitPolyMul(cx, cx),
		coneFitPolyMul(cy, cy)), coneFitPolyMul(cz, cz)),
		coneFitPolyScale(coneFitPolyMul(piece.h, piece.h), form.slope2))
	piece.df = coneFitPolyDeriv(piece.f)
	return piece
}

type coneFitRoot struct {
	index  int
	lo, hi *big.Rat
	loSign int
}

type coneFitSearch struct {
	pieces []coneFitPiece
	root   *coneFitRoot
	budget int
}

func (s *coneFitSearch) visit(index int, lo, hi *big.Rat, depth int) bool {
	if s.budget == 0 {
		return false
	}
	s.budget--
	piece := s.pieces[index]
	hlo, _ := coneFitCoeffBounds(piece.h, lo, hi)
	if hlo.Sign() <= 0 {
		return false
	}
	flo, fhi := coneFitCoeffBounds(piece.f, lo, hi)
	if flo.Sign() > 0 || fhi.Sign() < 0 {
		return true
	}
	a, b := coneFitPolyEval(piece.f, lo), coneFitPolyEval(piece.f, hi)
	if a.Sign() == 0 || b.Sign() == 0 {
		return false
	}
	dlo, dhi := coneFitCoeffBounds(piece.df, lo, hi)
	if a.Sign() != b.Sign() && ((dlo.Sign() >= 0 && dhi.Sign() > 0 && a.Sign() < 0) ||
		(dhi.Sign() <= 0 && dlo.Sign() < 0 && a.Sign() > 0)) {
		if s.root != nil {
			return false
		}
		s.root = &coneFitRoot{index: index, lo: lo, hi: hi, loSign: a.Sign()}
		return true
	}
	if depth == 0 {
		return false
	}
	mid := coneMul(coneAdd(lo, hi), big.NewRat(1, 2))
	if coneFitPolyEval(piece.f, mid).Sign() == 0 {
		return false
	}
	return s.visit(index, lo, mid, depth-1) && s.visit(index, mid, hi, depth-1)
}

func (p coneFitPiece) global(local *big.Rat) *big.Rat {
	return new(big.Rat).Quo(coneAdd(p.p0, coneMul(coneSub(p.p1, p.p0), local)), p.total)
}

func (p coneFitPiece) local(global *big.Rat) *big.Rat {
	return new(big.Rat).Quo(coneSub(coneMul(global, p.total), p.p0), coneSub(p.p1, p.p0))
}

func coneFitFloatDown(r *big.Rat) float64 {
	v, _ := r.Float64()
	if math.IsInf(v, 0) {
		return v
	}
	if coneRat(v).Cmp(r) > 0 {
		return math.Nextafter(v, math.Inf(-1))
	}
	return v
}

func coneFitFloatUp(r *big.Rat) float64 {
	v, _ := r.Float64()
	if math.IsInf(v, 0) {
		return v
	}
	if coneRat(v).Cmp(r) < 0 {
		return math.Nextafter(v, math.Inf(1))
	}
	return v
}

// CertifyConeFitCrossing proves exactly one transverse crossing of the
// complete ideal spline with the positive cone section. The spline must not
// be nil. All coefficients use exact rationals derived from its solved
// binary64 interpolant, frame, and cone. It does not bound the displacement
// from FitSpline.Eval or a downstream mesh; consumers charge that separately.
// Inconclusive cases are refused.
func (s *Sketch) CertifyConeFitCrossing(fit *FitSpline, cone ConeSection) (ConeFitCrossing, error) {
	if s == nil || fit == nil || !s.ownsConeCurve(fit) ||
		!coneFinite(cone.Apex.X, cone.Apex.Y, cone.Apex.Z,
			cone.Axis.X, cone.Axis.Y, cone.Axis.Z, cone.Slope) ||
		cone.Slope <= 0 || cone.Axis.Len() == 0 {
		return ConeFitCrossing{}, ErrConeFitCrossing
	}
	frame, err := s.Plane().Frame()
	if err != nil {
		return ConeFitCrossing{}, ErrConeFitCrossing
	}
	fi, err := fit.Interpolant()
	if err != nil || fi == nil || len(fi.Points) < 2 || len(fi.Points) != len(fi.Params) ||
		len(fi.Points) != len(fi.SecondDerivs) || fi.Params[0] != 0 ||
		fi.Params[len(fi.Params)-1] <= 0 {
		return ConeFitCrossing{}, ErrConeFitCrossing
	}
	form := newConeSideForm(frame, cone)
	pieces := make([]coneFitPiece, 0, len(fi.Points)-1)
	for i := 0; i+1 < len(fi.Points); i++ {
		if !(fi.Params[i+1] > fi.Params[i]) {
			return ConeFitCrossing{}, ErrConeFitCrossing
		}
		pieces = append(pieces, coneFitMakePiece(fi, i, form))
	}
	search := coneFitSearch{pieces: pieces, budget: 8192}
	for i := range pieces {
		if !search.visit(i, big.NewRat(0, 1), big.NewRat(1, 1), 24) {
			return ConeFitCrossing{}, ErrConeFitCrossing
		}
	}
	if search.root == nil {
		return ConeFitCrossing{}, ErrConeFitCrossing
	}
	root := search.root
	piece := pieces[root.index]
	lo, hi := root.lo, root.hi
	for step := 0; step < 256; step++ {
		mid := coneMul(coneAdd(lo, hi), big.NewRat(1, 2))
		sign := coneFitPolyEval(piece.f, mid).Sign()
		if sign == 0 {
			lo, hi = mid, mid
			break
		}
		if sign == root.loSign {
			lo = mid
		} else {
			hi = mid
		}
		if step >= 52 && coneBracketAtFloatPrecision(piece.global(lo), piece.global(hi)) {
			break
		}
	}
	globalLo, globalHi := piece.global(lo), piece.global(hi)
	dlo, dhi := coneFitCoeffBounds(piece.df, lo, hi)
	if dlo.Sign() <= 0 && dhi.Sign() >= 0 {
		return ConeFitCrossing{}, ErrConeFitCrossing
	}
	tlo, thi := coneFitFloatDown(globalLo), coneFitFloatUp(globalHi)
	t, _ := coneMul(coneAdd(globalLo, globalHi), big.NewRat(1, 2)).Float64()
	if !(t > 0 && t < 1 && tlo >= 0 && thi <= 1 &&
		thi-tlo <= 4*(math.Nextafter(t, math.Inf(1))-t)) {
		return ConeFitCrossing{}, ErrConeFitCrossing
	}
	before, after := ConeInside, ConeOutside
	if root.loSign > 0 {
		before, after = after, before
	}
	return ConeFitCrossing{sketch: s, spline: fit, points: append([]*Point(nil), fit.Fit...),
		cone: cone, frame: frame, revision: s.Revision(), pieces: pieces, root: root.index,
		localLo: lo, localHi: hi, t: t, lo: tlo, hi: thi, before: before, after: after}, nil
}

func (c ConeFitCrossing) rangeBounds(lo, hi float64, selectPoly func(coneFitPiece) coneFitPoly) (*big.Rat, *big.Rat, bool) {
	if c.IsStale() || !coneFinite(lo, hi) || lo < 0 || hi > 1 || lo > hi {
		return nil, nil, false
	}
	a, b := coneRat(lo), coneRat(hi)
	var low, high *big.Rat
	for _, piece := range c.pieces {
		start, end := piece.global(big.NewRat(0, 1)), piece.global(big.NewRat(1, 1))
		if b.Cmp(start) < 0 || a.Cmp(end) > 0 {
			continue
		}
		left, right := a, b
		if left.Cmp(start) < 0 {
			left = start
		}
		if right.Cmp(end) > 0 {
			right = end
		}
		pl, ph := coneFitCoeffBounds(selectPoly(piece), piece.local(left), piece.local(right))
		if low == nil || pl.Cmp(low) < 0 {
			low = pl
		}
		if high == nil || ph.Cmp(high) > 0 {
			high = ph
		}
	}
	return low, high, low != nil && high != nil
}

// PointBounds encloses every ideal 2D spline point in the requested normalized
// parameter range. The returned float bounds are rounded outward.
func (c ConeFitCrossing) PointBounds(lo, hi float64) ([2]float64, [2]float64, bool) {
	var lower, upper [2]float64
	for axis := range 2 {
		a, b, ok := c.rangeBounds(lo, hi, func(p coneFitPiece) coneFitPoly { return p.xy[axis] })
		if !ok {
			return [2]float64{}, [2]float64{}, false
		}
		lower[axis], upper[axis] = coneFitFloatDown(a), coneFitFloatUp(b)
		if !coneFinite(lower[axis], upper[axis]) {
			return [2]float64{}, [2]float64{}, false
		}
	}
	return lower, upper, true
}

// TangentBounds encloses du/dt and dv/dt on the requested normalized range.
// It returns outward-rounded bounds of the ideal fitted interpolant.
func (c ConeFitCrossing) TangentBounds(lo, hi float64) ([2]float64, [2]float64, bool) {
	var lower, upper [2]float64
	for axis := range 2 {
		a, b, ok := c.rangeBounds(lo, hi, func(p coneFitPiece) coneFitPoly {
			return coneFitPolyScale(coneFitPolyDeriv(p.xy[axis]),
				new(big.Rat).Quo(p.total, coneSub(p.p1, p.p0)))
		})
		if !ok {
			return [2]float64{}, [2]float64{}, false
		}
		lower[axis], upper[axis] = coneFitFloatDown(a), coneFitFloatUp(b)
		if !coneFinite(lower[axis], upper[axis]) {
			return [2]float64{}, [2]float64{}, false
		}
	}
	return lower, upper, true
}

// DerivativeBound proves one nonzero sign for dF/dt over the requested range.
// F is the cone's implicit residual. minAbs is an exact lower bound.
func (c ConeFitCrossing) DerivativeBound(lo, hi float64) (int, *big.Rat) {
	a, b, ok := c.rangeBounds(lo, hi, func(p coneFitPiece) coneFitPoly {
		return coneFitPolyScale(p.df, new(big.Rat).Quo(p.total, coneSub(p.p1, p.p0)))
	})
	if !ok {
		return 0, nil
	}
	if a.Sign() > 0 {
		return 1, new(big.Rat).Set(a)
	}
	if b.Sign() < 0 {
		return -1, new(big.Rat).Neg(b)
	}
	return 0, nil
}

func coneFitSignedMargin(p coneFitPiece, a, b *big.Rat, sign, depth int, budget *int) *big.Rat {
	if *budget == 0 {
		return nil
	}
	*budget--
	hlo, _ := coneFitCoeffBounds(p.h, a, b)
	if hlo.Sign() <= 0 {
		return nil
	}
	flo, fhi := coneFitCoeffBounds(p.f, a, b)
	margin := flo
	if sign < 0 {
		margin = new(big.Rat).Neg(fhi)
	}
	if margin.Sign() > 0 {
		return margin
	}
	dlo, dhi := coneFitCoeffBounds(p.df, a, b)
	if dlo.Sign() > 0 || dhi.Sign() < 0 {
		fa, fb := coneFitPolyEval(p.f, a), coneFitPolyEval(p.f, b)
		if sign < 0 {
			fa.Neg(fa)
			fb.Neg(fb)
		}
		if fa.Sign() > 0 && fb.Sign() > 0 {
			if fa.Cmp(fb) < 0 {
				return fa
			}
			return fb
		}
	}
	if depth == 0 {
		return nil
	}
	mid := coneMul(coneAdd(a, b), big.NewRat(1, 2))
	left := coneFitSignedMargin(p, a, mid, sign, depth-1, budget)
	if left == nil {
		return nil
	}
	right := coneFitSignedMargin(p, mid, b, sign, depth-1, budget)
	if right == nil {
		return nil
	}
	if left.Cmp(right) < 0 {
		return left
	}
	return right
}

func (c ConeFitCrossing) margin(start, end float64, sign int) *big.Rat {
	if c.IsStale() || !coneFinite(start, end) || start < 0 || end > 1 || start > end {
		return nil
	}
	a, b := coneRat(start), coneRat(end)
	var result *big.Rat
	budget := 8192
	for _, p := range c.pieces {
		ps, pe := p.global(big.NewRat(0, 1)), p.global(big.NewRat(1, 1))
		if b.Cmp(ps) < 0 || a.Cmp(pe) > 0 {
			continue
		}
		left, right := a, b
		if left.Cmp(ps) < 0 {
			left = ps
		}
		if right.Cmp(pe) > 0 {
			right = pe
		}
		margin := coneFitSignedMargin(p, p.local(left), p.local(right), sign, 24, &budget)
		if margin == nil {
			return nil
		}
		if result == nil || margin.Cmp(result) < 0 {
			result = margin
		}
	}
	return result
}

// MarginBefore returns an exact positive lower bound on |F| through the
// requested parameter, provided t lies before the crossing bracket.
// The complete open piece has no positive uniform margin at its crossing end.
func (c ConeFitCrossing) MarginBefore(t float64) *big.Rat {
	if !(t >= 0 && t < c.lo) {
		return nil
	}
	sign := 1
	if c.before == ConeInside {
		sign = -1
	}
	return c.margin(0, t, sign)
}

// MarginAfter returns an exact positive lower bound on |F| from the
// requested parameter onward, provided t lies after the crossing bracket.
func (c ConeFitCrossing) MarginAfter(t float64) *big.Rat {
	if !(t > c.hi && t <= 1) {
		return nil
	}
	sign := 1
	if c.after == ConeInside {
		sign = -1
	}
	return c.margin(t, 1, sign)
}
