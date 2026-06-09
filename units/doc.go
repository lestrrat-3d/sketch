// Package units is a small, self-contained units-of-measure library for the
// quantities a 2D sketcher cares about: lengths and angles (plus dimensionless
// scalars).
//
// Units are explicitly typed: you refer to them through the package's [Unit]
// constants (e.g. [Millimeter], [Inch], [Degree]) rather than by string. A
// [Value] pairs a magnitude with a unit and knows how to convert itself to any
// compatible unit:
//
//	w := units.Millimeters(100)
//	in, _ := w.In(units.Inch) // 3.937...
//	fmt.Println(w)            // "100 mm"
//
// A [System] records the current default length and angle units, used for
// presenting base-unit quantities back to a chosen unit.
//
// Base units are millimetre for [Length] and radian for [Angle]; every unit
// stores its conversion factor to that base. The package has no dependencies
// outside the standard library and is intended to be reusable on its own.
package units
