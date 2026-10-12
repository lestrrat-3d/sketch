package sketch

import (
	"errors"
	"math"
	"math/big"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch/geom"
)

// ConeSide is the strict side of a positive cone section occupied by a whole curve.
type ConeSide uint8

const (
	ConeOutside ConeSide = iota + 1 // positive cone implicit function
	ConeInside                      // negative cone implicit function
)

// ErrConeCurveSide means the requested whole-curve side cannot be proved.
var ErrConeCurveSide = errors.New("sketch: curve lacks one certified cone side")

// ConeCurveSide certifies that one whole Line, ideal FitSpline interpolant, or
// bounded Arc lies strictly on one side of the cone section. IsStale checks the source
// point handles, sketch, and plane frame before the claim is used.
type ConeCurveSide struct {
	entity   Entity
	points   []*Point
	cone     ConeSection
	side     ConeSide
	margin   *big.Rat
	revision uint64
	frame    r3.Frame
	sketch   *Sketch
}

// Entity returns the whole source curve named by the certificate.
func (c ConeCurveSide) Entity() Entity { return c.entity }

// Cone returns the cone source supplied at certification time.
func (c ConeCurveSide) Cone() ConeSection { return c.cone }

// ImplicitMargin returns an exact positive lower bound on |F| for the ideal
// source curve. The caller owns the returned copy. Any bound for evaluating or
// recording that curve must be charged separately before consuming this sign.
func (c ConeCurveSide) ImplicitMargin() *big.Rat {
	if c.IsStale() || c.margin == nil {
		return nil
	}
	return new(big.Rat).Set(c.margin)
}

// Side returns ConeOutside or ConeInside for a live certificate.
func (c ConeCurveSide) Side() ConeSide {
	if c.IsStale() {
		return 0
	}
	return c.side
}

// IsStale reports whether the source curve, sketch, or plane frame changed.
func (c ConeCurveSide) IsStale() bool {
	if c.sketch == nil || !c.sketch.ownsConeCurve(c.entity) ||
		c.sketch.Revision() != c.revision {
		return true
	}
	switch e := c.entity.(type) {
	case *FitSpline:
		if len(e.Fit) != len(c.points) {
			return true
		}
		for i, p := range e.Fit {
			if p != c.points[i] {
				return true
			}
		}
	case *Arc:
		if len(c.points) != 3 || e.Center != c.points[0] ||
			e.Start != c.points[1] || e.End != c.points[2] {
			return true
		}
	case *Line:
		if len(c.points) != 2 || e.Start != c.points[0] || e.End != c.points[1] {
			return true
		}
	default:
		return true
	}
	frame, err := c.sketch.Plane().Frame()
	return err != nil || frame.Origin() != c.frame.Origin() ||
		frame.U() != c.frame.U() || frame.V() != c.frame.V()
}

func (s *Sketch) ownsConeCurve(entity Entity) bool {
	if s == nil || entity == nil || s.entUIDs[entity] == 0 {
		return false
	}
	switch e := entity.(type) {
	case *FitSpline:
		if e == nil || e.s != s || len(e.Fit) < 2 {
			return false
		}
		for _, point := range e.Fit {
			if point == nil || point.s != s || point.IsStale() {
				return false
			}
		}
		return !e.IsConstruction()
	case *Arc:
		return e != nil && e.s == s && e.Center != nil && e.Start != nil && e.End != nil &&
			e.Center.s == s && e.Start.s == s && e.End.s == s && !e.IsStale() &&
			!e.IsConstruction()
	case *Line:
		return e != nil && e.s == s && e.Start != nil && e.End != nil &&
			e.Start.s == s && e.End.s == s && !e.IsStale() && !e.IsConstruction()
	}
	return false
}

// CertifyConeCurveSide proves one whole line, fitted spline, or narrow arc has a
// strict side of the positive cone section. It refuses any curve whose exact
// enclosure touches the cone or spans both sides. The proof uses rational
// Bezier hulls for fitted splines, a quadratic for lines, and a bounded circular sector for arcs.
func (s *Sketch) CertifyConeCurveSide(entity Entity, cone ConeSection) (ConeCurveSide, error) {
	if !s.ownsConeCurve(entity) ||
		!coneFinite(cone.Apex.X, cone.Apex.Y, cone.Apex.Z,
			cone.Axis.X, cone.Axis.Y, cone.Axis.Z, cone.Slope) ||
		cone.Slope <= 0 || cone.Axis.Len() == 0 {
		return ConeCurveSide{}, ErrConeCurveSide
	}
	frame, err := s.Plane().Frame()
	if err != nil {
		return ConeCurveSide{}, ErrConeCurveSide
	}
	form := newConeSideForm(frame, cone)
	var side ConeSide
	var margin *big.Rat
	var points []*Point
	switch curve := entity.(type) {
	case *FitSpline:
		fi, err := curve.Interpolant()
		if err != nil {
			return ConeCurveSide{}, ErrConeCurveSide
		}
		side, margin = form.fitSide(fi)
		points = append([]*Point(nil), curve.Fit...)
	case *Arc:
		side, margin = form.arcSide(curve)
		points = []*Point{curve.Center, curve.Start, curve.End}
	case *Line:
		side, margin = form.lineSide(curve)
		points = []*Point{curve.Start, curve.End}
	}
	if side == 0 || margin == nil || margin.Sign() <= 0 {
		return ConeCurveSide{}, ErrConeCurveSide
	}
	return ConeCurveSide{entity: entity, points: points, cone: cone, side: side, margin: margin, revision: s.Revision(),
		frame: frame, sketch: s}, nil
}

type coneInterval struct{ lo, hi *big.Rat }

func coneIntervalOf(a, b *big.Rat) coneInterval {
	if a.Cmp(b) <= 0 {
		return coneInterval{a, b}
	}
	return coneInterval{b, a}
}

func (a coneInterval) add(b coneInterval) coneInterval {
	return coneInterval{coneAdd(a.lo, b.lo), coneAdd(a.hi, b.hi)}
}

func (a coneInterval) sub(b coneInterval) coneInterval {
	return coneInterval{coneSub(a.lo, b.hi), coneSub(a.hi, b.lo)}
}

func (a coneInterval) scale(k *big.Rat) coneInterval {
	return coneIntervalOf(coneMul(a.lo, k), coneMul(a.hi, k))
}

func (a coneInterval) square() coneInterval {
	a2, b2 := coneMul(a.lo, a.lo), coneMul(a.hi, a.hi)
	if a.lo.Sign() >= 0 {
		return coneInterval{a2, b2}
	}
	if a.hi.Sign() <= 0 {
		return coneInterval{b2, a2}
	}
	if a2.Cmp(b2) < 0 {
		return coneInterval{coneRat(0), b2}
	}
	return coneInterval{coneRat(0), a2}
}

type coneSideForm struct {
	base, u, v [3]*big.Rat
	axis       [3]*big.Rat
	slope2     *big.Rat
}

func newConeSideForm(frame r3.Frame, cone ConeSection) coneSideForm {
	o, u, v := frame.Origin(), frame.U(), frame.V()
	return coneSideForm{
		base: [3]*big.Rat{coneSub(coneRat(o.X), coneRat(cone.Apex.X)),
			coneSub(coneRat(o.Y), coneRat(cone.Apex.Y)), coneSub(coneRat(o.Z), coneRat(cone.Apex.Z))},
		u:      [3]*big.Rat{coneRat(u.X), coneRat(u.Y), coneRat(u.Z)},
		v:      [3]*big.Rat{coneRat(v.X), coneRat(v.Y), coneRat(v.Z)},
		axis:   [3]*big.Rat{coneRat(cone.Axis.X), coneRat(cone.Axis.Y), coneRat(cone.Axis.Z)},
		slope2: coneMul(coneRat(cone.Slope), coneRat(cone.Slope)),
	}
}

// boxSide uses exact interval operations on the axis-aligned box enclosing a
// curve piece. A strict result proves the sign at every point in that box.
func (f coneSideForm) boxSide(u, v coneInterval) (ConeSide, *big.Rat) {
	var q [3]coneInterval
	for i := range q {
		base := coneInterval{f.base[i], f.base[i]}
		q[i] = base.add(u.scale(f.u[i])).add(v.scale(f.v[i]))
	}
	h := q[0].scale(f.axis[0]).add(q[1].scale(f.axis[1])).add(q[2].scale(f.axis[2]))
	if h.lo.Sign() <= 0 { // the whole box must occupy the positive cone half
		return 0, nil
	}
	cx := q[1].scale(f.axis[2]).sub(q[2].scale(f.axis[1]))
	cy := q[2].scale(f.axis[0]).sub(q[0].scale(f.axis[2]))
	cz := q[0].scale(f.axis[1]).sub(q[1].scale(f.axis[0]))
	residual := cx.square().add(cy.square()).add(cz.square()).sub(h.square().scale(f.slope2))
	if residual.lo.Sign() > 0 {
		return ConeOutside, residual.lo
	}
	if residual.hi.Sign() < 0 {
		return ConeInside, new(big.Rat).Neg(residual.hi)
	}
	return 0, nil
}

type coneBezier struct{ controls [4][2]*big.Rat }

func (b coneBezier) box() (coneInterval, coneInterval) {
	var lo, hi [2]*big.Rat
	for axis := range 2 {
		lo[axis], hi[axis] = b.controls[0][axis], b.controls[0][axis]
		for _, point := range b.controls[1:] {
			if point[axis].Cmp(lo[axis]) < 0 {
				lo[axis] = point[axis]
			}
			if point[axis].Cmp(hi[axis]) > 0 {
				hi[axis] = point[axis]
			}
		}
	}
	return coneInterval{lo[0], hi[0]}, coneInterval{lo[1], hi[1]}
}

func (b coneBezier) halves() (coneBezier, coneBezier) {
	var left, right coneBezier
	half := new(big.Rat).SetFrac64(1, 2)
	for axis := range 2 {
		p := [4]*big.Rat{b.controls[0][axis], b.controls[1][axis],
			b.controls[2][axis], b.controls[3][axis]}
		q0 := coneMul(coneAdd(p[0], p[1]), half)
		q1 := coneMul(coneAdd(p[1], p[2]), half)
		q2 := coneMul(coneAdd(p[2], p[3]), half)
		r0 := coneMul(coneAdd(q0, q1), half)
		r1 := coneMul(coneAdd(q1, q2), half)
		mid := coneMul(coneAdd(r0, r1), half)
		left.controls[0][axis], left.controls[1][axis] = p[0], q0
		left.controls[2][axis], left.controls[3][axis] = r0, mid
		right.controls[0][axis], right.controls[1][axis] = mid, r1
		right.controls[2][axis], right.controls[3][axis] = q2, p[3]
	}
	return left, right
}

func (f coneSideForm) bezierSide(b coneBezier, depth int, budget *int) (ConeSide, *big.Rat) {
	if *budget == 0 {
		return 0, nil
	}
	*budget = *budget - 1
	u, v := b.box()
	if side, margin := f.boxSide(u, v); side != 0 {
		return side, margin
	}
	if depth == 0 {
		return 0, nil
	}
	left, right := b.halves()
	a, ma := f.bezierSide(left, depth-1, budget)
	c, mc := f.bezierSide(right, depth-1, budget)
	if a != c {
		return 0, nil
	}
	if a == 0 || ma == nil || mc == nil {
		return 0, nil
	}
	if ma.Cmp(mc) <= 0 {
		return a, ma
	}
	return a, mc
}

func (f coneSideForm) fitSide(fi *geom.FitInterpolant) (ConeSide, *big.Rat) {
	if fi == nil || len(fi.Points) < 2 || len(fi.SecondDerivs) != len(fi.Points) ||
		len(fi.Params) != len(fi.Points) {
		return 0, nil
	}
	var result ConeSide
	var minMargin *big.Rat
	budget := 2048 // one bounded proof budget shared by all fitted spans
	for i := 0; i+1 < len(fi.Points); i++ {
		h := coneSub(coneRat(fi.Params[i+1]), coneRat(fi.Params[i]))
		if h.Sign() <= 0 {
			return 0, nil
		}
		b := coneFitBezier(fi, i)
		side, margin := f.bezierSide(b, 12, &budget)
		if side == 0 || result != 0 && result != side {
			return 0, nil
		}
		result = side
		if minMargin == nil || margin.Cmp(minMargin) < 0 {
			minMargin = margin
		}
	}
	return result, minMargin
}

func (f coneSideForm) lineSide(line *Line) (ConeSide, *big.Rat) {
	a, b := line.Start.Geometry(), line.End.Geometry()
	if !coneFinite(a.X, a.Y, b.X, b.Y) || (a.X == b.X && a.Y == b.Y) {
		return 0, nil
	}
	piece := coneFitResidual(coneFitPiece{xy: [2]coneFitPoly{
		{coneRat(a.X), coneSub(coneRat(b.X), coneRat(a.X))},
		{coneRat(a.Y), coneSub(coneRat(b.Y), coneRat(a.Y))},
	}}, f)
	zero, one := big.NewRat(0, 1), big.NewRat(1, 1)
	hlo, _ := coneFitCoeffBounds(piece.h, zero, one)
	if hlo.Sign() <= 0 {
		return 0, nil
	}
	f0, f1 := coneFitPolyEval(piece.f, zero), coneFitPolyEval(piece.f, one)
	lo, hi := coneIntervalOf(f0, f1).lo, coneIntervalOf(f0, f1).hi
	// A quadratic has at most one interior extremum.
	if piece.f[2].Sign() != 0 {
		vertex := new(big.Rat).Quo(new(big.Rat).Neg(piece.f[1]),
			coneMul(big.NewRat(2, 1), piece.f[2]))
		if vertex.Sign() > 0 && vertex.Cmp(one) < 0 {
			value := coneFitPolyEval(piece.f, vertex)
			if value.Cmp(lo) < 0 {
				lo = value
			}
			if value.Cmp(hi) > 0 {
				hi = value
			}
		}
	}
	if lo.Sign() > 0 {
		return ConeOutside, lo
	}
	if hi.Sign() < 0 {
		return ConeInside, new(big.Rat).Neg(hi)
	}
	return 0, nil
}

// arcSide covers the real bevel-tooth arc class: a short CCW sweep from the
// upper-left quadrant through negative local X into the lower-left quadrant.
// This sector lies in a proven rectangle, which boxSide tests exactly.
func (f coneSideForm) arcSide(arc *Arc) (ConeSide, *big.Rat) {
	cx, cy := arc.Center.X(), arc.Center.Y()
	sx, sy := arc.Start.X()-cx, arc.Start.Y()-cy
	ex, ey := arc.End.X()-cx, arc.End.Y()-cy
	r := arc.R()
	if !coneFinite(cx, cy, sx, sy, ex, ey, r) || !(r > 0 && sx < 0 && ex < 0 &&
		sy > 0 && ey < 0 && arc.Sweep() > 0 && arc.Sweep() < math.Pi/2) {
		return 0, nil
	}
	// The actual circular curve uses r=Arc.R(), whereas its end point may have
	// a slightly different radius after solving. Inflate both endpoint Y values
	// and verify the bound against each normalized endpoint ray exactly.
	delta := 1e-12 * r
	if !(delta > 0) || !coneFinite(delta) {
		return 0, nil
	}
	Y := math.Max(math.Abs(sy), math.Abs(ey)) + delta
	U := math.Max(sx, ex) + delta
	if !(U < 0 && Y > 0 && coneFinite(U, Y)) {
		return 0, nil
	}
	r2, y2, u2 := coneMul(coneRat(r), coneRat(r)),
		coneMul(coneRat(Y), coneRat(Y)), coneMul(coneRat(U), coneRat(U))
	if coneAdd(u2, y2).Cmp(r2) >= 0 {
		return 0, nil
	}
	for _, p := range [2][2]float64{{sx, sy}, {ex, ey}} {
		length2 := coneAdd(coneMul(coneRat(p[0]), coneRat(p[0])),
			coneMul(coneRat(p[1]), coneRat(p[1])))
		want := coneMul(r2, coneMul(coneRat(p[1]), coneRat(p[1])))
		if coneMul(y2, length2).Cmp(want) <= 0 {
			return 0, nil
		}
	}
	u := coneInterval{coneSub(coneRat(cx), coneRat(r)), coneAdd(coneRat(cx), coneRat(U))}
	v := coneInterval{coneSub(coneRat(cy), coneRat(Y)), coneAdd(coneRat(cy), coneRat(Y))}
	return f.boxSide(u, v)
}
