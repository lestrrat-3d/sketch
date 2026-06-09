package units

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrIncompatible is returned when an operation mixes units of different kinds
// (for example adding a length to an angle).
var ErrIncompatible = errors.New("units: incompatible kinds")

// Value is a magnitude paired with the [Unit] it is expressed in. The zero
// Value is 0 of the dimensionless unit [One].
type Value struct {
	mag  float64
	unit Unit
}

// New returns a Value of mag in unit u.
func New(mag float64, u Unit) Value { return Value{mag: mag, unit: u} }

// FromBase returns a Value equal to base (expressed in u's base unit), but
// carried in unit u. For example FromBase(1000, Meter) is 1 m.
func FromBase(base float64, u Unit) Value { return Value{mag: base / u.factor, unit: u} }

// Convenience constructors for the built-in units.
func Millimeters(x float64) Value { return Value{x, Millimeter} }
func Centimeters(x float64) Value { return Value{x, Centimeter} }
func Meters(x float64) Value      { return Value{x, Meter} }
func Inches(x float64) Value      { return Value{x, Inch} }
func Feet(x float64) Value        { return Value{x, Foot} }
func Thous(x float64) Value       { return Value{x, Thou} }
func Degrees(x float64) Value     { return Value{x, Degree} }
func Radians(x float64) Value     { return Value{x, Radian} }
func Scalar(x float64) Value      { return Value{x, One} }

// Mag returns the magnitude in the value's own unit.
func (v Value) Mag() float64 { return v.mag }

// Unit returns the value's unit.
func (v Value) Unit() Unit { return v.unit }

// Kind returns the kind of quantity the value measures.
func (v Value) Kind() Kind { return v.unit.kind }

// Base returns the magnitude expressed in the kind's base unit (mm for length,
// rad for angle).
func (v Value) Base() float64 { return v.mag * v.unit.factor }

// In returns the magnitude expressed in unit u, or [ErrIncompatible] if u
// measures a different kind.
func (v Value) In(u Unit) (float64, error) {
	if v.unit.kind != u.kind {
		return 0, fmt.Errorf("%w: cannot express %s in %s", ErrIncompatible, v.unit.kind, u.kind)
	}
	return v.mag * v.unit.factor / u.factor, nil
}

// Convert returns the same quantity carried in unit u.
func (v Value) Convert(u Unit) (Value, error) {
	m, err := v.In(u)
	if err != nil {
		return Value{}, err
	}
	return Value{m, u}, nil
}

// Add returns v + o, expressed in v's unit. The operands must be the same kind.
func (v Value) Add(o Value) (Value, error) {
	m, err := o.In(v.unit)
	if err != nil {
		return Value{}, err
	}
	return Value{v.mag + m, v.unit}, nil
}

// Sub returns v − o, expressed in v's unit. The operands must be the same kind.
func (v Value) Sub(o Value) (Value, error) {
	m, err := o.In(v.unit)
	if err != nil {
		return Value{}, err
	}
	return Value{v.mag - m, v.unit}, nil
}

// Scale returns v multiplied by a dimensionless factor.
func (v Value) Scale(f float64) Value { return Value{v.mag * f, v.unit} }

// Neg returns −v.
func (v Value) Neg() Value { return Value{-v.mag, v.unit} }

// Equal reports whether v and o represent the same quantity to within tol of
// the kind's base unit. Values of different kinds are never equal.
func (v Value) Equal(o Value, tol float64) bool {
	if v.unit.kind != o.unit.kind {
		return false
	}
	d := v.Base() - o.Base()
	if d < 0 {
		d = -d
	}
	return d <= tol
}

// String renders the value as "<magnitude> <symbol>" (just the number for
// dimensionless values).
func (v Value) String() string {
	n := strconv.FormatFloat(v.mag, 'g', -1, 64)
	if v.unit.kind == Dimensionless {
		return n
	}
	return n + " " + v.unit.symbol
}
