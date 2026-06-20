package geom

import (
	"fmt"
	"math"
)

// nearestParamSampled seeds a foot-point parameter: the t whose eval(t) is
// closest to (px, py), found by a dense polyline broad phase (each segment
// projected onto, not just its sample points) followed by golden-section
// refinement within the best span. It is a robust seed for a solver aux
// variable, NOT an exact projection. For an open curve t is clamped to [0,1];
// for a periodic curve it is wrapped into [0,1).
func nearestParamSampled(eval func(float64) (float64, float64), segs int, periodic bool, px, py float64) float64 {
	if segs < 64 {
		segs = 64
	}
	bestT, bestD2 := 0.0, math.Inf(1)
	px0, py0 := eval(0)
	for i := 1; i <= segs; i++ {
		t1 := float64(i) / float64(segs)
		px1, py1 := eval(t1)
		// Project (px,py) onto the chord [(px0,py0),(px1,py1)], clamped to it,
		// and map the chord parameter back to a curve parameter in this span.
		dx, dy := px1-px0, py1-py0
		seg2 := dx*dx + dy*dy
		u := 0.0
		if seg2 > 0 {
			u = ((px-px0)*dx + (py-py0)*dy) / seg2
			if u < 0 {
				u = 0
			} else if u > 1 {
				u = 1
			}
		}
		t := (float64(i-1) + u) / float64(segs)
		cx, cy := eval(t)
		if d2 := (px-cx)*(px-cx) + (py-cy)*(py-cy); d2 < bestD2 {
			bestD2, bestT = d2, t
		}
		px0, py0 = px1, py1
	}
	span := 1.0 / float64(segs)
	lo, hi := bestT-span, bestT+span
	if !periodic {
		if lo < 0 {
			lo = 0
		}
		if hi > 1 {
			hi = 1
		}
	}
	const invphi = 0.6180339887498949
	dist2 := func(t float64) float64 {
		cx, cy := eval(t)
		return (px-cx)*(px-cx) + (py-cy)*(py-cy)
	}
	c, d := hi-invphi*(hi-lo), lo+invphi*(hi-lo)
	fc, fd := dist2(c), dist2(d)
	for k := 0; k < 24; k++ {
		if fc < fd {
			hi, d, fd = d, c, fc
			c = hi - invphi*(hi-lo)
			fc = dist2(c)
		} else {
			lo, c, fc = c, d, fd
			d = lo + invphi*(hi-lo)
			fd = dist2(d)
		}
	}
	t := (lo + hi) / 2
	if periodic {
		return t - math.Floor(t) // wrap into [0,1)
	}
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// NearestParamPeriodicCubicBSpline returns the parameter t ∈ [0, 1) whose curve
// point is closest to (px, py) on the closed (periodic) cubic B-spline. A robust
// seed for a foot-point aux variable, not an exact projection. It panics with
// fewer than 3 control points.
func NearestParamPeriodicCubicBSpline(ctrl [][2]float64, px, py float64) float64 {
	n := len(ctrl)
	if n < 3 {
		panic(fmt.Sprintf("geom: closed cubic B-spline needs at least 3 control points, got %d", n))
	}
	eval := func(t float64) (float64, float64) { return EvalPeriodicCubicBSpline(ctrl, t) }
	return nearestParamSampled(eval, 16*n, true, px, py)
}

// NearestParamFitSpline returns the parameter t ∈ [0, 1] whose curve point is
// closest to (px, py) on the natural-cubic interpolating spline through fit. A
// robust seed for a foot-point aux variable, not an exact projection. It panics
// with fewer than 2 fit points.
func NearestParamFitSpline(fit [][2]float64, px, py float64) float64 {
	if len(fit) < 2 {
		panic(fmt.Sprintf("geom: fit-point spline needs at least 2 fit points, got %d", len(fit)))
	}
	e := newFitEvaluator(fit) // build once, reuse across samples
	eval := func(t float64) (float64, float64) {
		p := e.at(t)
		return p[0], p[1]
	}
	return nearestParamSampled(eval, 16*len(fit), false, px, py)
}
