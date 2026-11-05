package sketch

import (
	"math"
	"math/big"
	"slices"
)

type boundaryEdgeSnapshot struct {
	entity                    Entity
	partial, reversed, tExact bool
	tStart, tEnd              float64
	polyline                  [][2]float64
}

func (e BoundaryEdge) matchesSnapshot() bool {
	x := e.snapshot
	return x != nil && e.Entity == x.entity && e.Partial == x.partial &&
		e.Reversed == x.reversed && e.TExact == x.tExact &&
		e.TStart == x.tStart && e.TEnd == x.tEnd &&
		slices.Equal(e.Polyline, x.polyline)
}

// CertifyLineLineJunction returns the exact binary64 crossing at the walked end
// of left and start of right. It succeeds only for two distinct live line edges
// from one Profiles or Chains arrangement and one unmerged analytic crossing.
// The returned point is solved from the lines' endpoint float bits, not from
// the arrangement's rounded parameters or sampled polyline node.
func (s *Sketch) CertifyLineLineJunction(left, right BoundaryEdge) ([2]float64, bool) {
	if s == nil || left.sketch != s || right.sketch != s ||
		left.revision != right.revision || left.revision != s.Revision() ||
		left.crossEnd == nil || left.crossEnd != right.crossStart ||
		!left.matchesSnapshot() || !right.matchesSnapshot() ||
		!left.TExact || !right.TExact ||
		left.Entity != left.sourceEntity || right.Entity != right.sourceEntity ||
		len(left.Polyline) < 2 || len(right.Polyline) < 2 {
		return [2]float64{}, false
	}
	cross := left.crossEnd
	pair := cross.SourceIndices()
	if left.sourceIndex != pair[0] && left.sourceIndex != pair[1] {
		return [2]float64{}, false
	}
	if right.sourceIndex != pair[0] && right.sourceIndex != pair[1] ||
		left.sourceIndex == right.sourceIndex {
		return [2]float64{}, false
	}
	lineA, okA := left.Entity.(*Line)
	lineB, okB := right.Entity.(*Line)
	if !okA || !okB || !s.ownsEntity(lineA) || !s.ownsEntity(lineB) {
		return [2]float64{}, false
	}
	approx := cross.Point()
	if left.Polyline[len(left.Polyline)-1] != approx || right.Polyline[0] != approx {
		return [2]float64{}, false
	}
	point, ok := exactLineCrossing(lineA, lineB)
	if !ok || !nearLineCrossingEvent(point, approx, lineA, lineB) {
		return [2]float64{}, false
	}
	return point, true
}

func exactLineCrossing(a, b *Line) ([2]float64, bool) {
	coords := [...]float64{
		a.Start.X(), a.Start.Y(), a.End.X(), a.End.Y(),
		b.Start.X(), b.Start.Y(), b.End.X(), b.End.Y(),
	}
	r := make([]*big.Rat, len(coords))
	for i, v := range coords {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return [2]float64{}, false
		}
		r[i] = new(big.Rat).SetFloat64(v)
	}
	sub := func(x, y *big.Rat) *big.Rat { return new(big.Rat).Sub(x, y) }
	mul := func(x, y *big.Rat) *big.Rat { return new(big.Rat).Mul(x, y) }
	add := func(x, y *big.Rat) *big.Rat { return new(big.Rat).Add(x, y) }
	ax, ay, bx, by := r[0], r[1], r[2], r[3]
	cx, cy, dx, dy := r[4], r[5], r[6], r[7]
	ux, uy := sub(bx, ax), sub(by, ay)
	vx, vy := sub(dx, cx), sub(dy, cy)
	den := sub(mul(ux, vy), mul(uy, vx))
	if den.Sign() == 0 {
		return [2]float64{}, false
	}
	wx, wy := sub(cx, ax), sub(cy, ay)
	t := new(big.Rat).Quo(sub(mul(wx, vy), mul(wy, vx)), den)
	u := new(big.Rat).Quo(sub(mul(wx, uy), mul(wy, ux)), den)
	zero, one := new(big.Rat), new(big.Rat).SetInt64(1)
	if t.Cmp(zero) < 0 || t.Cmp(one) > 0 || u.Cmp(zero) < 0 || u.Cmp(one) > 0 {
		return [2]float64{}, false
	}
	x := add(ax, mul(t, ux))
	y := add(ay, mul(t, uy))
	fx, exactX := x.Float64()
	fy, exactY := y.Float64()
	if !exactX || !exactY || math.IsInf(fx, 0) || math.IsInf(fy, 0) {
		return [2]float64{}, false
	}
	return [2]float64{fx, fy}, true
}

func nearLineCrossingEvent(point, event [2]float64, a, b *Line) bool {
	scale := 0.0
	for _, v := range [...]float64{
		a.Start.X(), a.Start.Y(), a.End.X(), a.End.Y(),
		b.Start.X(), b.Start.Y(), b.End.X(), b.End.Y(),
	} {
		scale = math.Max(scale, math.Abs(v))
	}
	ulp := math.Nextafter(scale, math.Inf(1)) - scale
	return !math.IsInf(ulp, 0) &&
		math.Abs(point[0]-event[0]) <= 32*ulp &&
		math.Abs(point[1]-event[1]) <= 32*ulp
}
