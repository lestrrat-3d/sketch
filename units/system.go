package units

// System records the current default units for the kinds a sketch uses: a
// default length unit and a default angle unit. It is how the library "knows"
// which units to present results in.
type System struct {
	Length Unit // default length unit
	Angle  Unit // default angle unit
}

// Metric returns a system using millimetres and degrees — a common CAD default.
func Metric() System { return System{Length: Millimeter, Angle: Degree} }

// SI returns a system using metres and radians.
func SI() System { return System{Length: Meter, Angle: Radian} }

// Imperial returns a system using inches and degrees.
func Imperial() System { return System{Length: Inch, Angle: Degree} }

// UnitFor returns the system's default unit for a kind.
func (s System) UnitFor(k Kind) Unit {
	switch k {
	case Length:
		return s.Length
	case Angle:
		return s.Angle
	default:
		return One
	}
}

// In returns v's magnitude expressed in the system's default unit for v's kind.
func (s System) In(v Value) float64 {
	m, err := v.In(s.UnitFor(v.Kind()))
	if err != nil {
		return v.mag
	}
	return m
}

// Display returns v converted to the system's default unit for its kind.
func (s System) Display(v Value) Value {
	c, err := v.Convert(s.UnitFor(v.Kind()))
	if err != nil {
		return v
	}
	return c
}

// LengthFromBase wraps a base-unit (millimetre) length in the system's default
// length unit.
func (s System) LengthFromBase(base float64) Value { return FromBase(base, s.Length) }

// AngleFromBase wraps a base-unit (radian) angle in the system's default angle
// unit.
func (s System) AngleFromBase(base float64) Value { return FromBase(base, s.Angle) }
