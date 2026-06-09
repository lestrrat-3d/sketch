package units

import "math"

// Kind is the physical quantity a unit measures. The library is deliberately
// limited to the kinds a 2D sketch needs.
type Kind int

const (
	// Dimensionless is a pure number (ratios, counts, multipliers).
	Dimensionless Kind = iota
	// Length is a linear distance; its base unit is the millimetre.
	Length
	// Angle is a planar angle; its base unit is the radian.
	Angle
)

// String returns a human-readable name for the kind.
func (k Kind) String() string {
	switch k {
	case Length:
		return "length"
	case Angle:
		return "angle"
	default:
		return "dimensionless"
	}
}

// Unit is a unit of measure. Units are values, compared by identity of their
// (symbol, kind, factor); obtain them from the package constants such as
// [Millimeter] or [Degree], or register new ones with [Define]. There is no
// way to name a unit by a bare string in the value-building API.
type Unit struct {
	symbol string
	kind   Kind
	factor float64 // magnitude * factor == magnitude in the kind's base unit
}

// Symbol returns the unit's short symbol (e.g. "mm"); it is empty for the
// dimensionless unit.
func (u Unit) Symbol() string { return u.symbol }

// Kind returns the quantity the unit measures.
func (u Unit) Kind() Kind { return u.kind }

// Factor returns the multiplier that converts a magnitude in this unit to the
// kind's base unit.
func (u Unit) Factor() float64 { return u.factor }

// String returns the unit's symbol, or "(dimensionless)" for [One].
func (u Unit) String() string {
	if u.symbol == "" {
		return "(dimensionless)"
	}
	return u.symbol
}

// The built-in units. Base units (factor 1) are Millimeter and Radian.
var (
	// One is the dimensionless unit.
	One = define("", Dimensionless, 1)

	// Length units (base: millimetre).
	Millimeter = define("mm", Length, 1)
	Centimeter = define("cm", Length, 10)
	Meter      = define("m", Length, 1000)
	Inch       = define("in", Length, 25.4)
	Foot       = define("ft", Length, 304.8)
	Thou       = define("thou", Length, 0.0254) // a.k.a. mil; 1/1000 inch

	// Angle units (base: radian).
	Radian = define("rad", Angle, 1)
	Degree = define("deg", Angle, math.Pi/180)
)

// BaseUnit returns the base unit for a kind (factor 1).
func BaseUnit(k Kind) Unit {
	switch k {
	case Length:
		return Millimeter
	case Angle:
		return Radian
	default:
		return One
	}
}

// registry maps symbols back to units for serialization and lookup.
var registry = map[string]Unit{}

func define(symbol string, kind Kind, factor float64) Unit {
	u := Unit{symbol: symbol, kind: kind, factor: factor}
	registry[symbol] = u
	return u
}

// Define registers and returns a new unit measuring kind, whose magnitudes
// convert to the kind's base unit by multiplying by factorToBase. It enables
// callers to extend the built-in set (e.g. a "yard"). The symbol must be unique.
func Define(symbol string, kind Kind, factorToBase float64) Unit {
	return define(symbol, kind, factorToBase)
}

// Lookup returns the unit previously registered for symbol. It is intended for
// deserialization; prefer the typed [Unit] constants in normal code.
func Lookup(symbol string) (Unit, bool) {
	u, ok := registry[symbol]
	return u, ok
}
