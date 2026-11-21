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
// round-to-nearest mode and then steps one float outward with down or up.
// IEEE 754 makes +, −, ×, ÷ and sqrt correctly rounded, so the rounded result is
// within half an ulp of the exact one and the step outward always encloses it.
//
// Every product is written as an explicit float64(…) conversion: the Go spec
// lets an implementation fuse x*y+z into one rounding, and an explicit
// conversion is the documented way to forbid that. A fused result would still
// be within half an ulp, but keeping one rounding per operation keeps the
// argument above literally true on every architecture.

// down and up step one float toward −∞ and +∞ and return exactly what
// math.Nextafter(x, ∓Inf) returns, NaN included, but each is small enough to
// inline into the interval helpers. Both zeros step to the smallest subnormal
// of the step's sign. Past the finite range, the infinity the step points to
// is returned as is and NaN becomes math.NaN(). On every other input the bit
// pattern moves one unit: away from zero when the step leaves zero behind,
// toward zero otherwise; ±MaxFloat64 is ±Inf's neighbour, so the infinity the
// step points away from takes that path too. On every non-NaN input
// down(x) = -up(-x), the form the Lean model states.
// Lean proofs depend on this function; see _proofs/trace.json.
func down(x float64) float64 {
	if x == 0 {
		return -math.SmallestNonzeroFloat64
	}
	if !(x >= -math.MaxFloat64) {
		if x < 0 {
			return x
		}
		return math.NaN()
	}
	b := math.Float64bits(x)
	if int64(b) < 0 {
		return math.Float64frombits(b + 1)
	}
	return math.Float64frombits(b - 1)
}

// up steps one float toward +∞; see down.
// Lean proofs depend on this function; see _proofs/trace.json.
func up(x float64) float64 {
	if x == 0 {
		return math.SmallestNonzeroFloat64
	}
	if !(x <= math.MaxFloat64) {
		if x > 0 {
			return x
		}
		return math.NaN()
	}
	b := math.Float64bits(x)
	if int64(b) < 0 {
		return math.Float64frombits(b - 1)
	}
	return math.Float64frombits(b + 1)
}

// pt is the degenerate interval holding exactly v.
// Lean proofs depend on this function; see _proofs/trace.json.
func pt(v float64) Interval { return Interval{v, v} }

// iadd encloses a+b, rounding the lower endpoint down and the upper one up.
// Lean proofs depend on this function; see _proofs/trace.json.
func iadd(a, b Interval) Interval { return Interval{down(a.Lo + b.Lo), up(a.Hi + b.Hi)} }

// isub encloses a−b, rounding the lower endpoint down and the upper one up.
// Lean proofs depend on this function; see _proofs/trace.json.
func isub(a, b Interval) Interval { return Interval{down(a.Lo - b.Hi), up(a.Hi - b.Lo)} }

// ineg encloses −a; negation is exact, so no rounding step is needed.
// Lean proofs depend on this function; see _proofs/trace.json.
func ineg(a Interval) Interval { return Interval{-a.Hi, -a.Lo} }

// imul encloses a·b from the four rounded endpoint products.
// Lean proofs depend on this function; see _proofs/trace.json.
func imul(a, b Interval) Interval {
	p1 := float64(a.Lo * b.Lo)
	p2 := float64(a.Lo * b.Hi)
	p3 := float64(a.Hi * b.Lo)
	p4 := float64(a.Hi * b.Hi)
	return Interval{down(min(p1, p2, p3, p4)), up(max(p1, p2, p3, p4))}
}

// imulPt is imul(pt(y), b) with the two duplicate products dropped; its result
// is bit-identical to imul's because a degenerate factor repeats p1 and p2.
// Lean proofs depend on this function; see _proofs/trace.json.
func imulPt(y float64, b Interval) Interval {
	p1 := float64(y * b.Lo)
	p2 := float64(y * b.Hi)
	return Interval{down(min(p1, p2)), up(max(p1, p2))}
}

// isqr is a·a with the dependency kept: the square of an interval straddling
// zero starts at zero, which imul(a, a) would not see.
// Lean proofs depend on this function; see _proofs/trace.json.
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
// Lean proofs depend on this function; see _proofs/trace.json.
func isqrtNonNeg(a Interval) Interval {
	lo := max(a.Lo, 0)
	return Interval{max(down(math.Sqrt(lo)), 0), up(math.Sqrt(a.Hi))}
}

// ihull is the smallest interval containing both a and b.
// Lean proofs depend on this function; see _proofs/trace.json.
func ihull(a, b Interval) Interval { return Interval{min(a.Lo, b.Lo), max(a.Hi, b.Hi)} }

// isFinite reports whether both endpoints are finite and ordered. A NaN
// endpoint fails the ordering test, so this is the one screen the certified
// path needs against poisoned arithmetic. Given Lo <= Hi, Lo = +Inf forces
// Hi = +Inf and Hi = -Inf forces Lo = -Inf, so ruling out -Inf below and +Inf
// above rules out every infinite endpoint.
// Lean proofs depend on this function; see _proofs/trace.json.
func (i Interval) isFinite() bool {
	return -math.MaxFloat64 <= i.Lo && i.Lo <= i.Hi && i.Hi <= math.MaxFloat64
}

// within reports whether i lies inside o (not necessarily strictly).
func (i Interval) within(o Interval) bool { return o.Lo <= i.Lo && i.Hi <= o.Hi }

// strictlyInside reports whether i lies in the interior of o, the Krawczyk
// test's inclusion.
// Lean proofs depend on this function; see _proofs/trace.json.
func (i Interval) strictlyInside(o Interval) bool { return o.Lo < i.Lo && i.Hi < o.Hi }

// mid returns the float64 midpoint Lo + (Hi-Lo)/2.
// Lean proofs depend on this function; see _proofs/trace.json.
func (i Interval) mid() float64 { return i.Lo + (i.Hi-i.Lo)/2 }
