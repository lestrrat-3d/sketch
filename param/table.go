package param

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// Sentinel errors returned (wrapped) by the package. Use [errors.Is] to test
// for them.
var (
	// ErrUndefined indicates a reference to a name (parameter, constant or
	// function) that has not been defined.
	ErrUndefined = errors.New("param: undefined name")
	// ErrCycle indicates a cyclic chain of parameter references.
	ErrCycle = errors.New("param: cyclic reference")
	// ErrInvalidName indicates an illegal parameter name.
	ErrInvalidName = errors.New("param: invalid name")
)

// Func is a function callable from expressions. It receives already-evaluated
// arguments and is responsible for validating its own arity.
type Func func(args []float64) (float64, error)

type entry struct {
	name string
	src  string // original source text
	expr Expr   // parsed expression
}

// Table is a collection of named parameters that may reference one another
// through expressions. The zero value is not usable; create a Table with [New].
//
// A Table is not safe for concurrent use.
type Table struct {
	entries map[string]*entry
	order   []string // insertion order
	consts  map[string]float64
	funcs   map[string]Func
}

// New returns an empty parameter table preloaded with the standard constants
// (pi, tau, e, phi) and math functions.
func New() *Table {
	return &Table{
		entries: map[string]*entry{},
		consts:  defaultConsts(),
		funcs:   defaultFuncs(),
	}
}

// Set defines or redefines a parameter from an expression string. The
// expression is parsed immediately (syntax errors are returned now) but not
// evaluated, so it may reference parameters defined later.
func (t *Table) Set(name, expr string) error {
	if !isIdent(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	e, err := Parse(expr)
	if err != nil {
		return err
	}
	t.put(&entry{name: name, src: expr, expr: e})
	return nil
}

// SetValue defines or redefines a parameter as a literal number.
func (t *Table) SetValue(name string, v float64) error {
	if !isIdent(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	t.put(&entry{name: name, src: strconv.FormatFloat(v, 'g', -1, 64), expr: &numberExpr{v: v}})
	return nil
}

func (t *Table) put(e *entry) {
	if _, ok := t.entries[e.name]; !ok {
		t.order = append(t.order, e.name)
	}
	t.entries[e.name] = e
}

// Get evaluates the named parameter, resolving any parameters it depends on.
func (t *Table) Get(name string) (float64, error) {
	ctx := &evalContext{t: t, memo: map[string]float64{}, inProgress: map[string]bool{}}
	return ctx.resolve(name)
}

// MustGet is like [Table.Get] but panics on error. Intended for tests and
// statically known-good tables.
func (t *Table) MustGet(name string) float64 {
	v, err := t.Get(name)
	if err != nil {
		panic(err)
	}
	return v
}

// Eval evaluates an ad-hoc expression against the table without storing it.
func (t *Table) Eval(expr string) (float64, error) {
	e, err := Parse(expr)
	if err != nil {
		return 0, err
	}
	ctx := &evalContext{t: t, memo: map[string]float64{}, inProgress: map[string]bool{}}
	return e.eval(ctx)
}

// Has reports whether a parameter (not a constant or function) is defined.
func (t *Table) Has(name string) bool { _, ok := t.entries[name]; return ok }

// Delete removes a parameter. It is a no-op if the parameter does not exist.
// Note that other parameters referencing it will then fail to evaluate.
func (t *Table) Delete(name string) {
	if _, ok := t.entries[name]; !ok {
		return
	}
	delete(t.entries, name)
	for i, n := range t.order {
		if n == name {
			t.order = append(t.order[:i], t.order[i+1:]...)
			break
		}
	}
}

// Names returns the parameter names in the order they were first defined.
func (t *Table) Names() []string {
	out := make([]string, len(t.order))
	copy(out, t.order)
	return out
}

// Source returns the original expression text of a parameter.
func (t *Table) Source(name string) (string, bool) {
	e, ok := t.entries[name]
	if !ok {
		return "", false
	}
	return e.src, true
}

// Dependencies returns the names of parameters directly referenced by the named
// parameter's expression, sorted. Constants and functions are not included.
func (t *Table) Dependencies(name string) ([]string, error) {
	e, ok := t.entries[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUndefined, name)
	}
	set := map[string]struct{}{}
	e.expr.refs(set)
	var deps []string
	for n := range set {
		if _, isConst := t.consts[n]; isConst {
			continue
		}
		deps = append(deps, n)
	}
	sort.Strings(deps)
	return deps, nil
}

// Validate evaluates every parameter and returns the first error encountered,
// surfacing undefined references and cycles across the whole table.
func (t *Table) Validate() error {
	for _, name := range t.order {
		if _, err := t.Get(name); err != nil {
			return fmt.Errorf("parameter %q: %w", name, err)
		}
	}
	return nil
}

// SetFunc registers (or replaces) a function available to expressions.
func (t *Table) SetFunc(name string, fn Func) { t.funcs[name] = fn }

// SetConst registers (or replaces) a named constant. Parameters of the same
// name take precedence over constants.
func (t *Table) SetConst(name string, v float64) { t.consts[name] = v }

// evalContext carries per-evaluation state: a memo of already-computed
// parameter values and the set currently being computed (for cycle detection).
type evalContext struct {
	t          *Table
	memo       map[string]float64
	inProgress map[string]bool
}

func (ctx *evalContext) resolve(name string) (float64, error) {
	if v, ok := ctx.memo[name]; ok {
		return v, nil
	}
	e, ok := ctx.t.entries[name]
	if !ok {
		// Fall back to constants for bare identifiers.
		if v, isConst := ctx.t.consts[name]; isConst {
			return v, nil
		}
		return 0, fmt.Errorf("%w: %q", ErrUndefined, name)
	}
	if ctx.inProgress[name] {
		return 0, fmt.Errorf("%w: %q", ErrCycle, name)
	}
	ctx.inProgress[name] = true
	v, err := e.expr.eval(ctx)
	delete(ctx.inProgress, name)
	if err != nil {
		return 0, err
	}
	ctx.memo[name] = v
	return v, nil
}
