package geom

import "math"

type analyticLineBox struct {
	ux, uy                 float64
	minX, minY, maxX, maxY float64
	valid                  bool
}

func analyticLineBoxOf(o operand) analyticLineBox {
	if !o.isLine {
		return analyticLineBox{}
	}
	for _, v := range [...]float64{o.ax, o.ay, o.bx, o.by} {
		if !analyticFinite(v) {
			return analyticLineBox{}
		}
	}
	dx, dy := o.bx-o.ax, o.by-o.ay
	length := math.Hypot(dx, dy)
	if !(length > 0) || !analyticFinite(length) {
		return analyticLineBox{}
	}
	reach := arcParamEps * length
	if !analyticFinite(reach) {
		return analyticLineBox{}
	}
	return analyticLineBox{
		ux: dx / length, uy: dy / length,
		minX: math.Min(o.ax, o.bx) - reach, minY: math.Min(o.ay, o.by) - reach,
		maxX: math.Max(o.ax, o.bx) + reach, maxY: math.Max(o.ay, o.by) + reach,
		valid: true,
	}
}

// analyticLineBoxMargin bounds classification bands and solve roundoff for the
// scene. The endpoint-parameter extension is already included in each box.
// The solve-error allowance uses the filter's minimum accepted sine (1e-6), so
// it also covers every retained crossing's sensitivity to the rounded solve.
func analyticLineBoxMargin(sources []source, scale, merge float64, budget *arrangeBudget) (float64, bool) {
	coord := 0.0
	for i := range sources {
		if budget != nil {
			budget.step(1)
		}
		if sources[i].kind == srcLine {
			s := &sources[i]
			coord = math.Max(coord, math.Max(math.Max(math.Abs(s.ax), math.Abs(s.ay)),
				math.Max(math.Abs(s.bx), math.Abs(s.by))))
		}
	}
	// The kernel forms cross products and squared lengths in float64. Keep very
	// large-coordinate scenes on that path because its intermediate overflow
	// behavior is part of what the filter must preserve.
	if coord > math.Sqrt(math.MaxFloat64)/8 {
		return 0, false
	}
	band := math.Max(scale*mergeEps, merge)
	roundoff := 64 * 0x1p-52 * coord / 1e-6
	margin := band + roundoff
	return margin, analyticFinite(margin) && margin < math.MaxFloat64/2
}

// analyticLineBoxesDisjoint proves that two finite line sources cannot produce
// an event accepted by lineLineEvents. False keeps the full kernel path.
func analyticLineBoxesDisjoint(a, b analyticLineBox) bool {
	if !a.valid || !b.valid {
		return false
	}
	if !(a.maxX < b.minX || b.maxX < a.minX || a.maxY < b.minY || b.maxY < a.minY) {
		return false
	}
	// Near-parallel solves and the kernel's collinearity branch have large
	// parameter sensitivity. Keep them on the full path after the cheap box test.
	sin := math.Abs(a.ux*b.uy - a.uy*b.ux)
	return sin >= 1e-6
}

func analyticLineBoxExpand(box analyticLineBox, margin float64) analyticLineBox {
	if !box.valid {
		return box
	}
	box.minX -= margin
	box.minY -= margin
	box.maxX += margin
	box.maxY += margin
	return box
}

func analyticFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
