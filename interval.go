package sketch

import "math"

// Interval is a closed interval [Lo, Hi] of real numbers whose endpoints are
// float64 values. Every Interval an [Enclosure] publishes is OUTWARD ROUNDED:
// read each endpoint as the exact rational number the float64 represents, and
// the exact real quantity it describes lies between them. No float operation
// on the endpoints is needed (or safe) afterwards.
type Interval struct {
	Lo, Hi float64
}

// Contains reports whether v lies in the closed interval.
func (i Interval) Contains(v float64) bool { return i.Lo <= v && v <= i.Hi }

// The helpers below are the certified path's interval arithmetic. Go has no
// rounding-mode control, so each operation computes its endpoints in the default
// round-to-nearest mode and then steps one float outward with math.Nextafter.
// IEEE 754 makes +, −, ×, ÷ and sqrt correctly rounded, so the rounded result is
// within half an ulp of the exact one and the step outward always encloses it.
//
// Every product is written as an explicit float64(…) conversion: the Go spec
// lets an implementation fuse x*y+z into one rounding, and an explicit
// conversion is the documented way to forbid that. A fused result would still
// be within half an ulp, but keeping one rounding per operation keeps the
// argument above literally true on every architecture.

// down and up step one float toward −∞ and +∞.
func down(x float64) float64 { return math.Nextafter(x, math.Inf(-1)) }
func up(x float64) float64   { return math.Nextafter(x, math.Inf(1)) }

// pt is the degenerate interval holding exactly v.
func pt(v float64) Interval { return Interval{v, v} }

func iadd(a, b Interval) Interval { return Interval{down(a.Lo + b.Lo), up(a.Hi + b.Hi)} }
func isub(a, b Interval) Interval { return Interval{down(a.Lo - b.Hi), up(a.Hi - b.Lo)} }
func ineg(a Interval) Interval    { return Interval{-a.Hi, -a.Lo} }

func imul(a, b Interval) Interval {
	p1 := float64(a.Lo * b.Lo)
	p2 := float64(a.Lo * b.Hi)
	p3 := float64(a.Hi * b.Lo)
	p4 := float64(a.Hi * b.Hi)
	return Interval{down(min(p1, p2, p3, p4)), up(max(p1, p2, p3, p4))}
}

// isqr is a·a with the dependency kept: the square of an interval straddling
// zero starts at zero, which imul(a, a) would not see.
func isqr(a Interval) Interval {
	lo := float64(a.Lo * a.Lo)
	hi := float64(a.Hi * a.Hi)
	switch {
	case a.Lo >= 0:
		return Interval{down(lo), up(hi)}
	case a.Hi <= 0:
		return Interval{down(hi), up(lo)}
	default:
		return Interval{0, up(max(lo, hi))}
	}
}

// isqrtNonNeg encloses √a for a quantity known to be non-negative (a sum of
// squares): a lower endpoint rounded below zero is clamped back to zero, which
// is sound only because the exact quantity cannot be negative.
func isqrtNonNeg(a Interval) Interval {
	lo := max(a.Lo, 0)
	return Interval{max(down(math.Sqrt(lo)), 0), up(math.Sqrt(a.Hi))}
}

func ihull(a, b Interval) Interval { return Interval{min(a.Lo, b.Lo), max(a.Hi, b.Hi)} }

// isFinite reports whether both endpoints are finite and ordered. A NaN
// endpoint fails the ordering test, so this is the one screen the certified
// path needs against poisoned arithmetic.
func (i Interval) isFinite() bool {
	return i.Lo <= i.Hi && !math.IsInf(i.Lo, 0) && !math.IsInf(i.Hi, 0)
}

// within reports whether i lies inside o (not necessarily strictly).
func (i Interval) within(o Interval) bool { return o.Lo <= i.Lo && i.Hi <= o.Hi }

// strictlyInside reports whether i lies in the interior of o, the Krawczyk
// test's inclusion.
func (i Interval) strictlyInside(o Interval) bool { return o.Lo < i.Lo && i.Hi < o.Hi }

func (i Interval) mid() float64 { return i.Lo + (i.Hi-i.Lo)/2 }
