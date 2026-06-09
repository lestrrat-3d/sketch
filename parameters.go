package sketch

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
)

// Dimension is a dimensional constraint (distance, radius, angle, …) whose
// driving value carries a unit and may be set literally or bound to a parameter
// expression. All of [Distance], [HorizontalDistance], [VerticalDistance],
// [Radius], [Diameter] and [Angle] satisfy it.
type Dimension interface {
	Constraint
	// Kind reports the quantity the dimension measures (length or angle).
	Kind() units.Kind
	// Target returns the current driving value, carrying its unit.
	Target() units.Value
	// Set replaces the magnitude (keeping the unit) and clears any binding.
	Set(float64)
	// SetValue replaces the value with a typed quantity of the dimension's kind.
	SetValue(units.Value) error

	base() float64
	setResolved(units.Value) error
	restore(float64, units.Unit)
	driverExpr() string
	setDriverExpr(string)
}

// DriverExpr returns the parameter expression bound to a dimension, or "" if
// its value is a literal.
func DriverExpr(d Dimension) string { return d.driverExpr() }

// ErrTableMismatch is returned by [Sketch.Bind] when a dimension is bound to a
// different parameter table than the one already in use by the sketch. All
// bound dimensions in a sketch share a single table.
var ErrTableMismatch = errors.New("sketch: dimensions must be bound to the same parameter table")

// Units returns the sketch's unit system (its default length and angle units).
// New sketches default to [units.Metric] (millimetres and degrees).
func (s *Sketch) Units() units.System { return s.sys }

// SetUnits sets the sketch's default length and angle units, used to interpret
// bare-float dimension values and to present results.
func (s *Sketch) SetUnits(sys units.System) { s.sys = sys }

func (s *Sketch) lengthUnit() units.Unit {
	if s.sys.Length.Kind() != units.Length {
		return units.Millimeter
	}
	return s.sys.Length
}

func (s *Sketch) angleUnit() units.Unit {
	if s.sys.Angle.Kind() != units.Angle {
		return units.Degree
	}
	return s.sys.Angle
}

func (s *Sketch) unitFor(k units.Kind) units.Unit {
	switch k {
	case units.Length:
		return s.lengthUnit()
	case units.Angle:
		return s.angleUnit()
	default:
		return units.One
	}
}

// AddConstraint commits one or more constraints to the sketch. Constraints
// reference solver-bound geometry (the [Point]/[Line]/[Circle] handles returned
// by the Add methods), which is therefore already committed. Dimensional
// constraints created from a bare float adopt the sketch's default unit for
// their kind here.
func (s *Sketch) AddConstraint(cs ...Constraint) {
	for _, c := range cs {
		if d, ok := c.(interface{ resolveUnit(*Sketch) }); ok {
			d.resolveUnit(s)
		}
		s.cons = append(s.cons, c)
	}
}

// Params returns the parameter table the sketch's dimensions are bound against,
// or nil if no dimension has been bound yet. The table is supplied explicitly
// at [Sketch.Bind] time.
func (s *Sketch) Params() *param.Table { return s.params }

// Bind drives a dimension's value from an expression evaluated against the
// given parameter table before every solve. The table is required and becomes
// the sketch's table; binding another dimension against a different table
// returns [ErrTableMismatch]. The expression is parsed immediately so syntax
// errors surface here; the names it references are resolved at solve time.
//
//	p := param.New()
//	p.SetValue("width", units.Millimeters(120))
//	w := s.Distance(a, b, 0)
//	s.Bind(w, p, "width")
func (s *Sketch) Bind(d Dimension, table *param.Table, expr string) error {
	if table == nil {
		return fmt.Errorf("sketch: Bind requires a non-nil parameter table")
	}
	if s.params != nil && s.params != table {
		return ErrTableMismatch
	}
	if _, err := param.Parse(expr); err != nil {
		return err
	}
	s.params = table
	d.setDriverExpr(expr)
	return nil
}

// Unbind removes a dimension's parameter binding, leaving its current value in
// place as a literal.
func (s *Sketch) Unbind(d Dimension) { d.setDriverExpr("") }

// ApplyParameters evaluates every bound dimension against the parameter table
// and writes the result into the dimension's value. It is called automatically
// at the start of [Sketch.Solve]; call it directly if you need the bound values
// applied without solving. It is a no-op when no parameter table is attached.
//
// When a dimension is bound directly to a single named parameter, the
// parameter's kind is checked against the dimension's kind so that, for
// example, an angle parameter cannot silently drive a length.
func (s *Sketch) ApplyParameters() error {
	if s.params == nil {
		return nil
	}
	for _, c := range s.cons {
		d, ok := c.(Dimension)
		if !ok {
			continue
		}
		expr := d.driverExpr()
		if expr == "" {
			continue
		}
		v, err := s.evalDimension(d, expr)
		if err != nil {
			return err
		}
		if err := d.setResolved(v); err != nil {
			return fmt.Errorf("sketch: applying dimension expression %q: %w", expr, err)
		}
	}
	return nil
}

// evalDimension evaluates a bound expression to a unit-carrying value for the
// dimension's kind. When the expression is a direct parameter reference the
// parameter's value (and unit) is used and its kind checked; otherwise the
// expression is evaluated to a base-unit magnitude and tagged with the
// dimension's base unit. All conversion is left to the units library.
func (s *Sketch) evalDimension(d Dimension, expr string) (units.Value, error) {
	if name := strings.TrimSpace(expr); s.params.Has(name) {
		v, err := s.params.GetValue(name)
		if err != nil {
			return units.Value{}, fmt.Errorf("sketch: evaluating dimension parameter %q: %w", name, err)
		}
		if v.Kind() != units.Dimensionless && v.Kind() != d.Kind() {
			return units.Value{}, fmt.Errorf("sketch: %s dimension bound to %s parameter %q", d.Kind(), v.Kind(), name)
		}
		return v, nil
	}
	base, err := s.params.Eval(expr)
	if err != nil {
		return units.Value{}, fmt.Errorf("sketch: evaluating dimension expression %q: %w", expr, err)
	}
	return units.FromBase(base, units.BaseUnit(d.Kind())), nil
}
