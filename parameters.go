package sketch

import (
	"errors"
	"fmt"

	"github.com/lestrrat-3d/sketch/param"
)

// Dimension is a dimensional constraint (distance, radius, angle, …) whose
// driving value can be set literally or bound to a parameter expression. All
// of [Distance], [HorizontalDistance], [VerticalDistance], [Radius],
// [Diameter] and [Angle] satisfy it.
type Dimension interface {
	Constraint
	// Value reports the dimension's current driving value.
	value() float64
	setValue(float64)
	// driverExpr returns the bound parameter expression, or "" if literal.
	driverExpr() string
	setDriverExpr(string)
}

// Value returns the current driving value of a dimension.
func Value(d Dimension) float64 { return d.value() }

// DriverExpr returns the parameter expression bound to a dimension, or "" if
// its value is a literal.
func DriverExpr(d Dimension) string { return d.driverExpr() }

// ErrTableMismatch is returned by [Sketch.Bind] when a dimension is bound to a
// different parameter table than the one already in use by the sketch. All
// bound dimensions in a sketch share a single table.
var ErrTableMismatch = errors.New("sketch: dimensions must be bound to the same parameter table")

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
//	p.Set("width", "120")
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
		v, err := s.params.Eval(expr)
		if err != nil {
			return fmt.Errorf("sketch: evaluating dimension expression %q: %w", expr, err)
		}
		d.setValue(v)
	}
	return nil
}
