package sketch

import (
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

// Params returns the sketch's parameter table, creating an empty one on first
// use. Define parameters on it and bind dimensions to expressions with
// [Sketch.Bind].
func (s *Sketch) Params() *param.Table {
	if s.params == nil {
		s.params = param.New()
	}
	return s.params
}

// SetParams attaches an external parameter table to the sketch, replacing any
// existing one. Passing nil detaches parameters entirely.
func (s *Sketch) SetParams(t *param.Table) { s.params = t }

// Bind drives a dimension's value from a parameter expression that is
// re-evaluated against the sketch's parameter table before every solve. The
// expression is parsed immediately so syntax errors surface here; references it
// contains are resolved at solve time.
//
//	s.Params().Set("width", "120")
//	w := s.Distance(a, b, 0)
//	s.Bind(w, "width")
func (s *Sketch) Bind(d Dimension, expr string) error {
	if _, err := param.Parse(expr); err != nil {
		return err
	}
	s.Params() // ensure a table exists so the binding is always evaluated
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
