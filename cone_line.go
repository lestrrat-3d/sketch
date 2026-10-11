package sketch

import (
	"errors"
	"math"
	"math/big"

	"github.com/lestrrat-3d/r3"
)

// ConeSection identifies a right circular cone in the sketch's world frame.
// Apex, Axis, and Slope must be copied from the consumer's cone source. Axis
// points toward the cone's positive, growing-radius half; Slope is tan(half-angle).
// Axis need not be exactly unit length.
type ConeSection struct {
	Apex  r3.Vec
	Axis  r3.Vec
	Slope float64
}

// ConeLineCrossing certifies one transverse crossing of a sketch line and a
// cone section. T() approximates the line parameter at the crossing, enclosed
// by TBounds(). TExact() means the unique true parameter is isolated to
// machine precision; T() is the nearest available approximation.
// The source line, sketch revision, and cone data travel with the certificate so
// a consumer can reject a stale trim or a cone other than its own source.
type ConeLineCrossing struct {
	line      *Line
	start     *Point
	end       *Point
	cone      ConeSection
	revision  uint64
	t, lo, hi float64
	point     [2]float64
	sketch    *Sketch
	frame     r3.Frame
}

// Line returns the exact sketch entity named by this certificate.
func (c ConeLineCrossing) Line() *Line { return c.line }

// Cone returns the cone source supplied at certification time.
func (c ConeLineCrossing) Cone() ConeSection { return c.cone }

// SketchRevision returns the source sketch revision at certification time.
func (c ConeLineCrossing) SketchRevision() uint64 { return c.revision }

// T returns the floating approximation to the isolated true line parameter.
func (c ConeLineCrossing) T() float64 { return c.t }

// TBounds returns outward-rounded bounds on the true line parameter.
func (c ConeLineCrossing) TBounds() (float64, float64) { return c.lo, c.hi }

// Point returns the line evaluation at T in sketch coordinates.
func (c ConeLineCrossing) Point() [2]float64 { return c.point }

// TExact reports whether the certificate remains live and bounds the true
// parameter to machine precision. A zero-value certificate returns false.
func (c ConeLineCrossing) TExact() bool {
	return !c.IsStale() && c.lo < c.hi && c.t >= c.lo && c.t <= c.hi
}

// IsStale reports whether the line, sketch, or plane frame changed after certification.
func (c ConeLineCrossing) IsStale() bool {
	if c.sketch == nil || c.line == nil || c.line.Start == nil || c.line.End == nil ||
		c.line.Start != c.start || c.line.End != c.end || c.line.IsConstruction() ||
		c.line.Start.s != c.sketch || c.line.End.s != c.sketch || c.line.IsStale() ||
		c.line.s != c.sketch || c.sketch.entUIDs[c.line] == 0 ||
		c.sketch.Revision() != c.revision {
		return true
	}
	frame, err := c.sketch.Plane().Frame()
	return err != nil || frame.Origin() != c.frame.Origin() ||
		frame.U() != c.frame.U() || frame.V() != c.frame.V()
}

var ErrConeLineCrossing = errors.New("sketch: cone and line lack one certified transverse crossing")

// CertifyConeLineCrossing proves that line crosses the positive cone section
// exactly once in its open domain. It refuses tangencies, endpoint contacts,
// misses, two crossings, and invalid source geometry. All polynomial signs are
// computed as exact rationals over the stored binary64 sketch frame, line, and
// cone fields. The single root is then bracketed to machine precision.
func (s *Sketch) CertifyConeLineCrossing(line *Line, cone ConeSection) (ConeLineCrossing, error) {
	if s == nil || line == nil || line.s != s || line.Start == nil || line.End == nil ||
		line.Start.s != s || line.End.s != s || line.IsStale() ||
		s.entUIDs[line] == 0 || line.IsConstruction() ||
		!coneFinite(cone.Apex.X, cone.Apex.Y, cone.Apex.Z,
			cone.Axis.X, cone.Axis.Y, cone.Axis.Z, cone.Slope) ||
		cone.Slope <= 0 || cone.Axis.Len() == 0 {
		return ConeLineCrossing{}, ErrConeLineCrossing
	}
	frame, err := s.Plane().Frame()
	if err != nil {
		return ConeLineCrossing{}, ErrConeLineCrossing
	}
	a, b := line.Start.Geometry(), line.End.Geometry()
	dx, dy := b.X-a.X, b.Y-a.Y
	if !coneFinite(a.X, a.Y, b.X, b.Y, dx, dy) || (a.X == b.X && a.Y == b.Y) {
		return ConeLineCrossing{}, ErrConeLineCrossing
	}
	origin, u, v := frame.Origin(), frame.U(), frame.V()
	// Build Q(t) = plane(a+t(b-a)) - cone.Apex without rounded world
	// intermediates. Every coefficient below is an exact rational expression of
	// the caller's stored binary64 source values.
	axis := [3]*big.Rat{coneRat(cone.Axis.X), coneRat(cone.Axis.Y), coneRat(cone.Axis.Z)}
	var q, d [3]*big.Rat
	du, dv := coneSub(coneRat(b.X), coneRat(a.X)), coneSub(coneRat(b.Y), coneRat(a.Y))
	for i, values := range [3][4]float64{
		{origin.X, u.X, v.X, cone.Apex.X},
		{origin.Y, u.Y, v.Y, cone.Apex.Y},
		{origin.Z, u.Z, v.Z, cone.Apex.Z},
	} {
		q[i] = coneSub(coneAdd(coneAdd(coneRat(values[0]),
			coneMul(coneRat(values[1]), coneRat(a.X))),
			coneMul(coneRat(values[2]), coneRat(a.Y))), coneRat(values[3]))
		d[i] = coneAdd(coneMul(coneRat(values[1]), du),
			coneMul(coneRat(values[2]), dv))
	}
	// F(t) = |Q(t) x Axis|^2 - Slope^2 (Q(t) dot Axis)^2.
	// This also handles a source axis whose binary64 length is not exactly one.
	cross := func(x [3]*big.Rat) [3]*big.Rat {
		return [3]*big.Rat{
			coneSub(coneMul(x[1], axis[2]), coneMul(x[2], axis[1])),
			coneSub(coneMul(x[2], axis[0]), coneMul(x[0], axis[2])),
			coneSub(coneMul(x[0], axis[1]), coneMul(x[1], axis[0])),
		}
	}
	dot := func(x, y [3]*big.Rat) *big.Rat {
		return coneAdd(coneAdd(coneMul(x[0], y[0]), coneMul(x[1], y[1])), coneMul(x[2], y[2]))
	}
	cq, cd := cross(q), cross(d)
	hq, hd := dot(q, axis), dot(d, axis)
	slope2 := coneMul(coneRat(cone.Slope), coneRat(cone.Slope))
	coef := [3]*big.Rat{
		coneSub(dot(cq, cq), coneMul(slope2, coneMul(hq, hq))),
		coneSub(coneMul(coneRat(2), dot(cq, cd)),
			coneMul(coneRat(2), coneMul(slope2, coneMul(hq, hd)))),
		coneSub(dot(cd, cd), coneMul(slope2, coneMul(hd, hd))),
	}
	// Both endpoints must lie on the cone's positive half. The axial function
	// is linear, so this also proves the whole line interval lies on that half.
	if hq.Sign() <= 0 || coneAdd(hq, hd).Sign() <= 0 {
		return ConeLineCrossing{}, ErrConeLineCrossing
	}
	f0 := coef[0]
	f1 := coneAdd(coneAdd(coef[0], coef[1]), coef[2])
	if f0.Sign() == 0 || f1.Sign() == 0 || f0.Sign() == f1.Sign() {
		return ConeLineCrossing{}, ErrConeLineCrossing
	}
	// Opposite nonzero endpoint signs on a quadratic prove exactly one simple
	// root in (0,1); a tangent or a second crossing cannot satisfy this gate.
	lo, hi := new(big.Rat), new(big.Rat).SetInt64(1)
	loSign := f0.Sign()
	for step := range 256 {
		mid := coneMul(coneAdd(lo, hi), new(big.Rat).SetFrac64(1, 2))
		f := coneAdd(coneAdd(coef[0], coneMul(coef[1], mid)),
			coneMul(coef[2], coneMul(mid, mid)))
		if f.Sign() == 0 {
			lo, hi = mid, mid
			break
		}
		if f.Sign() == loSign {
			lo = mid
		} else {
			hi = mid
		}
		if step >= 52 && coneBracketAtFloatPrecision(lo, hi) {
			break
		}
	}
	tmid := coneMul(coneAdd(lo, hi), new(big.Rat).SetFrac64(1, 2))
	t, _ := tmid.Float64()
	tlo, _ := lo.Float64()
	thi, _ := hi.Float64()
	tlo = math.Nextafter(tlo, math.Inf(-1))
	thi = math.Nextafter(thi, math.Inf(1))
	ulp := math.Nextafter(t, math.Inf(1)) - t
	if !(t > 0 && t < 1 && thi-tlo <= 4*ulp) {
		return ConeLineCrossing{}, ErrConeLineCrossing
	}
	px, py := a.X+dx*t, a.Y+dy*t
	if !coneFinite(px, py) {
		return ConeLineCrossing{}, ErrConeLineCrossing
	}
	return ConeLineCrossing{
		line: line, start: line.Start, end: line.End, cone: cone, revision: s.Revision(),
		t: t, lo: tlo, hi: thi,
		point:  [2]float64{px, py},
		sketch: s, frame: frame,
	}, nil
}

func coneBracketAtFloatPrecision(lo, hi *big.Rat) bool {
	tmid := coneMul(coneAdd(lo, hi), new(big.Rat).SetFrac64(1, 2))
	t, _ := tmid.Float64()
	if !(t > 0 && t < 1) {
		return false
	}
	left, _ := lo.Float64()
	right, _ := hi.Float64()
	left = math.Nextafter(left, math.Inf(-1))
	right = math.Nextafter(right, math.Inf(1))
	return right-left <= 4*(math.Nextafter(t, math.Inf(1))-t)
}

func coneFinite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func coneRat(v float64) *big.Rat     { return new(big.Rat).SetFloat64(v) }
func coneAdd(a, b *big.Rat) *big.Rat { return new(big.Rat).Add(a, b) }
func coneSub(a, b *big.Rat) *big.Rat { return new(big.Rat).Sub(a, b) }
func coneMul(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }
