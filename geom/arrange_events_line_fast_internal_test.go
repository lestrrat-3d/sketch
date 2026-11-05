package geom

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// referenceLineLineEvents is lineLineEvents as it stood when it computed both
// segment lengths for every pair. The length-free fast path must agree with it
// on every pair, near-parallel ones included. Its products carry the same
// explicit float64 conversions as lineLineEvents, so no FMA fusion can make the
// two paths round differently on an architecture that fuses.
func referenceLineLineEvents(a, b operand, scale float64) ([]xEvent, bool) {
	d1x, d1y := a.bx-a.ax, a.by-a.ay
	d2x, d2y := b.bx-b.ax, b.by-b.ay
	den := float64(d1x*d2y) - float64(d1y*d2x)
	l1 := math.Hypot(d1x, d1y)
	l2 := math.Hypot(d2x, d2y)
	if l1 == 0 || l2 == 0 {
		return nil, true // a zero-length "line" is degenerate
	}
	if sin := math.Abs(den) / (l1 * l2); sin < lineParallelEps {
		// Parallel. Distinct carriers never meet. A shared carrier is a degenerate
		// duplicate edge ONLY where the two segments actually overlap; collinear but
		// disjoint segments do not meet.
		perp := math.Abs(float64((b.ax-a.ax)*d1y)-float64((b.ay-a.ay)*d1x)) / l1
		if perp >= scale*mergeEps {
			return nil, false // parallel, distinct carriers
		}
		tb0, tb1 := a.lineParam(b.ax, b.ay), a.lineParam(b.bx, b.by)
		lo, hi := math.Min(tb0, tb1), math.Max(tb0, tb1)
		ov0, ov1 := math.Max(0, lo), math.Min(1, hi)
		if ov1-ov0 <= arcParamEps {
			// Disjoint, or touching only at a shared endpoint — that is a normal join
			// (a corner), not a degenerate overlap.
			return nil, false
		}
		mid := (ov0 + ov1) / 2 // a point inside the positive-length overlap, in a's param
		x, y := a.ax+float64(mid*d1x), a.ay+float64(mid*d1y)
		e := xEvent{x: x, y: y, ti: mid, tj: b.lineParam(x, y), kind: evOverlap}
		// The overlap's two ends are each an endpoint of one of the two lines: a's
		// own end where b reaches past it, b's endpoint where b stops inside a. A pair
		// whose carriers are the same line at round-off is RESOLVABLE (see
		// docs/coincident-carrier-resolution-design.md, "Coincident line carriers"):
		// both ends are reported so analyticPrepass can cut both lines there and
		// suppress the losing one over the shared span. A pair collinear only within
		// the classification band above keeps the nil extent and stays a degeneracy.
		loX, loY := a.ax, a.ay
		if lo > 0 {
			loX, loY = b.ax, b.ay
			if tb1 < tb0 {
				loX, loY = b.bx, b.by
			}
		}
		hiX, hiY := a.bx, a.by
		if hi < 1 {
			hiX, hiY = b.bx, b.by
			if tb1 < tb0 {
				hiX, hiY = b.ax, b.ay
			}
		}
		if linesIdentical(a, b, [2][2]float64{{loX, loY}, {hiX, hiY}}, scale) {
			e.overlap = &overlapExtent{
				loX: loX, loY: loY, loTi: a.lineParam(loX, loY), loTj: b.lineParam(loX, loY),
				hiX: hiX, hiY: hiY, hiTi: a.lineParam(hiX, hiY), hiTj: b.lineParam(hiX, hiY),
			}
		}
		return []xEvent{e}, false
	}
	t := (float64((b.ax-a.ax)*d2y) - float64((b.ay-a.ay)*d2x)) / den
	x, y := a.ax+float64(t*d1x), a.ay+float64(t*d1y)
	return []xEvent{{
		x: x, y: y,
		ti:   a.lineParam(x, y),
		tj:   b.lineParam(x, y),
		kind: evCross,
	}}, false
}

func TestLineLineEventsMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(0x4c4c4556, 5))
	coord := func(scale float64) float64 { return (rng.Float64() - 0.5) * scale }
	for trial := 0; trial < 20000; trial++ {
		scale := math.Pow(10, float64(rng.IntN(9)-4))
		a := operand{isLine: true, ax: coord(scale), ay: coord(scale), bx: coord(scale), by: coord(scale)}
		var b operand
		switch rng.IntN(4) {
		case 0: // nearly parallel to a
			k := coord(2)
			eps := math.Pow(10, -float64(rng.IntN(14)))
			b = operand{isLine: true, ax: coord(scale), ay: coord(scale)}
			b.bx = b.ax + k*(a.bx-a.ax) + eps*coord(scale)
			b.by = b.ay + k*(a.by-a.ay) + eps*coord(scale)
		case 1: // collinear
			b = operand{isLine: true, ax: a.ax + 0.3*(a.bx-a.ax), ay: a.ay + 0.3*(a.by-a.ay),
				bx: a.ax + 1.7*(a.bx-a.ax), by: a.ay + 1.7*(a.by-a.ay)}
		case 2: // degenerate point
			b = operand{isLine: true, ax: coord(scale), ay: coord(scale)}
			b.bx, b.by = b.ax, b.ay
		default:
			b = operand{isLine: true, ax: coord(scale), ay: coord(scale), bx: coord(scale), by: coord(scale)}
		}
		want, wantAmb := referenceLineLineEvents(a, b, scale)
		got, gotAmb := lineLineEvents(a, b, scale)
		require.Equal(t, wantAmb, gotAmb, "trial %d ambiguous", trial)
		require.Equal(t, want, got, "trial %d events", trial)
	}
}
