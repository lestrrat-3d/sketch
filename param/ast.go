package param

import (
	"fmt"
	"math"
	"strconv"

	"github.com/lestrrat-3d/sketch/units"
)

// Expr is a parsed expression node. Expr values are immutable once produced by
// [Parse]; the same tree can be evaluated repeatedly.
type Expr interface {
	eval(ctx *evalContext) (float64, error)
	// kindOf computes the unit kind the expression evaluates to, validating that
	// every operation combines compatible kinds (see kind.go). It is a static
	// structural walk: identifier kinds come from their declared unit.
	kindOf(t *Table) (units.Kind, error)
	// refs records the identifiers referenced by the expression.
	refs(out map[string]struct{})
	// String returns a canonical, re-parseable rendering of the expression.
	String() string
}

type numberExpr struct{ v float64 }

func (e *numberExpr) eval(*evalContext) (float64, error) { return e.v, nil }
func (e *numberExpr) refs(map[string]struct{})           {}
func (e *numberExpr) String() string                     { return strconv.FormatFloat(e.v, 'g', -1, 64) }

type identExpr struct{ name string }

func (e *identExpr) eval(ctx *evalContext) (float64, error) { return ctx.resolve(e.name) }
func (e *identExpr) refs(out map[string]struct{})           { out[e.name] = struct{}{} }
func (e *identExpr) String() string                         { return e.name }

type unaryExpr struct {
	op rune // '+' or '-'
	x  Expr
}

func (e *unaryExpr) eval(ctx *evalContext) (float64, error) {
	v, err := e.x.eval(ctx)
	if err != nil {
		return 0, err
	}
	if e.op == '-' {
		return -v, nil
	}
	return v, nil
}
func (e *unaryExpr) refs(out map[string]struct{}) { e.x.refs(out) }
func (e *unaryExpr) String() string               { return fmt.Sprintf("%c%s", e.op, e.x) }

type binaryExpr struct {
	op   rune // '+','-','*','/','%','^'
	x, y Expr
}

func (e *binaryExpr) eval(ctx *evalContext) (float64, error) {
	a, err := e.x.eval(ctx)
	if err != nil {
		return 0, err
	}
	b, err := e.y.eval(ctx)
	if err != nil {
		return 0, err
	}
	switch e.op {
	case '+':
		return a + b, nil
	case '-':
		return a - b, nil
	case '*':
		return a * b, nil
	case '/':
		if b == 0 {
			return 0, fmt.Errorf("param: division by zero")
		}
		return a / b, nil
	case '%':
		if b == 0 {
			return 0, fmt.Errorf("param: modulo by zero")
		}
		return math.Mod(a, b), nil
	case '^':
		return math.Pow(a, b), nil
	}
	return 0, fmt.Errorf("param: unknown operator %q", e.op)
}
func (e *binaryExpr) refs(out map[string]struct{}) { e.x.refs(out); e.y.refs(out) }
func (e *binaryExpr) String() string               { return fmt.Sprintf("(%s %c %s)", e.x, e.op, e.y) }

type callExpr struct {
	name string
	args []Expr
}

func (e *callExpr) eval(ctx *evalContext) (float64, error) {
	fn, ok := ctx.t.funcs[e.name]
	if !ok {
		return 0, fmt.Errorf("%w: function %q", ErrUndefined, e.name)
	}
	vals := make([]float64, len(e.args))
	for i, a := range e.args {
		v, err := a.eval(ctx)
		if err != nil {
			return 0, err
		}
		vals[i] = v
	}
	return fn(vals)
}
func (e *callExpr) refs(out map[string]struct{}) {
	for _, a := range e.args {
		a.refs(out)
	}
}
func (e *callExpr) String() string {
	s := e.name + "("
	for i, a := range e.args {
		if i > 0 {
			s += ", "
		}
		s += a.String()
	}
	return s + ")"
}
