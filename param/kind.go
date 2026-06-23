package param

import (
	"errors"
	"fmt"

	"github.com/lestrrat-3d/sketch/units"
)

// ErrIncompatibleKind indicates an expression mixes unit kinds in a way the
// engine cannot represent — adding a length to an angle, multiplying two
// lengths (no area unit), inverting a unit, and so on. Kind algebra tracks
// length / angle / dimensionless through arithmetic; it is NOT full dimensional
// algebra (there are no area or inverse-length units). Use [errors.Is].
var ErrIncompatibleKind = errors.New("param: incompatible unit kinds")

// kindOf computes the unit kind an expression evaluates to, validating that
// every operation combines compatible kinds. Identifier kinds are STATIC — a
// parameter's kind is its declared unit's kind, a constant is dimensionless — so
// this is a pure structural walk needing no evaluation or cycle handling.

func (e *numberExpr) kindOf(*Table) (units.Kind, error) { return units.Dimensionless, nil }

func (e *identExpr) kindOf(t *Table) (units.Kind, error) {
	if ent, ok := t.entries[e.name]; ok {
		return ent.unit.Kind(), nil
	}
	if _, ok := t.consts[e.name]; ok {
		return units.Dimensionless, nil
	}
	return 0, fmt.Errorf("%w: %q", ErrUndefined, e.name)
}

func (e *unaryExpr) kindOf(t *Table) (units.Kind, error) { return e.x.kindOf(t) } // +/- preserve kind

func (e *binaryExpr) kindOf(t *Table) (units.Kind, error) {
	ka, err := e.x.kindOf(t)
	if err != nil {
		return 0, err
	}
	kb, err := e.y.kindOf(t)
	if err != nil {
		return 0, err
	}
	switch e.op {
	case '+', '-':
		// length+length is a length; angle+angle, and angle+dimensionless (radians
		// are physically dimensionless, so theta + pi/2 is an angle), combine to an
		// angle. length+angle and length+dimensionless (e.g. width + 5) are kind
		// errors — a length never mixes with a bare number or an angle.
		k, ok := combineAddSub(ka, kb)
		if !ok {
			return 0, fmt.Errorf("%w: cannot %c a %s and a %s", ErrIncompatibleKind, e.op, ka.String(), kb.String())
		}
		return k, nil
	case '*':
		// A dimensioned quantity may be scaled by a dimensionless one; two
		// dimensioned quantities would need an area / compound unit, which the
		// engine does not have.
		switch {
		case ka == units.Dimensionless:
			return kb, nil
		case kb == units.Dimensionless:
			return ka, nil
		default:
			return 0, fmt.Errorf("%w: cannot multiply a %s by a %s (no compound unit)", ErrIncompatibleKind, ka.String(), kb.String())
		}
	case '/':
		// kind/dimensionless preserves the kind; same-kind/same-kind is a
		// dimensionless ratio; dimensionless/kind (an inverse unit) and mixed
		// kinds are rejected.
		switch {
		case kb == units.Dimensionless:
			return ka, nil
		case ka == kb:
			return units.Dimensionless, nil
		default:
			return 0, fmt.Errorf("%w: cannot divide a %s by a %s", ErrIncompatibleKind, ka.String(), kb.String())
		}
	case '%':
		k, ok := combineAddSub(ka, kb)
		if !ok {
			return 0, fmt.Errorf("%w: cannot take a %s modulo a %s", ErrIncompatibleKind, ka.String(), kb.String())
		}
		return k, nil
	case '^':
		// Raising to a power needs a dimensionless base and exponent (a length^2
		// would be an area).
		if ka != units.Dimensionless || kb != units.Dimensionless {
			return 0, fmt.Errorf("%w: '^' requires dimensionless operands, got %s ^ %s", ErrIncompatibleKind, ka.String(), kb.String())
		}
		return units.Dimensionless, nil
	}
	return 0, fmt.Errorf("param: unknown operator %q", e.op)
}

func (e *callExpr) kindOf(t *Table) (units.Kind, error) {
	ks := make([]units.Kind, len(e.args))
	for i, a := range e.args {
		k, err := a.kindOf(t)
		if err != nil {
			return 0, err
		}
		ks[i] = k
	}
	if rule, ok := funcKindRules[e.name]; ok {
		return rule(e.name, ks)
	}
	// A custom function (registered via SetFunc) has no kind signature, so it is
	// dimensionless-only in v1: typed custom functions are a follow-up.
	if _, ok := t.funcs[e.name]; ok {
		return allDimensionless(e.name, ks)
	}
	return 0, fmt.Errorf("%w: function %q", ErrUndefined, e.name)
}

// --- function kind rules ----------------------------------------------------

type kindRule func(name string, ks []units.Kind) (units.Kind, error)

// allDimensionless requires every argument to be dimensionless and returns a
// dimensionless result. Arity is left to the value-evaluation pass.
func allDimensionless(name string, ks []units.Kind) (units.Kind, error) {
	for _, k := range ks {
		if k != units.Dimensionless {
			return 0, fmt.Errorf("%w: %s expects dimensionless arguments, got a %s", ErrIncompatibleKind, name, k.String())
		}
	}
	return units.Dimensionless, nil
}

// preserveKind returns the single argument's kind unchanged (abs).
func preserveKind(name string, ks []units.Kind) (units.Kind, error) {
	if len(ks) != 1 {
		return units.Dimensionless, nil // defer the arity error to evaluation
	}
	return ks[0], nil
}

// combineAddSub returns the kind of an additive combination (+, -, %, and the
// same-kind functions). Two equal kinds combine to that kind; an angle and a
// dimensionless number combine to an angle (radians are physically
// dimensionless). A length never combines with a bare number or an angle. ok is
// false for an incompatible pairing.
func combineAddSub(a, b units.Kind) (units.Kind, bool) {
	if a == b {
		return a, true
	}
	if (a == units.Angle && b == units.Dimensionless) || (a == units.Dimensionless && b == units.Angle) {
		return units.Angle, true
	}
	return 0, false
}

// sameKind requires the arguments to combine additively (min/max/clamp/hypot/
// atan2 over like quantities), allowing the angle/dimensionless mixing of
// combineAddSub, and returns the combined kind.
func sameKind(name string, ks []units.Kind) (units.Kind, error) {
	if len(ks) == 0 {
		return units.Dimensionless, nil
	}
	acc := ks[0]
	for _, k := range ks[1:] {
		c, ok := combineAddSub(acc, k)
		if !ok {
			return 0, fmt.Errorf("%w: %s expects arguments of one kind, got a %s and a %s", ErrIncompatibleKind, name, acc.String(), k.String())
		}
		acc = c
	}
	return acc, nil
}

// angleOrScalarToScalar is the forward-trig rule: the argument is an angle (or a
// dimensionless number of radians), the result is a dimensionless ratio.
func angleOrScalarToScalar(name string, ks []units.Kind) (units.Kind, error) {
	if len(ks) != 1 {
		return units.Dimensionless, nil
	}
	if ks[0] != units.Angle && ks[0] != units.Dimensionless {
		return 0, fmt.Errorf("%w: %s expects an angle, got a %s", ErrIncompatibleKind, name, ks[0].String())
	}
	return units.Dimensionless, nil
}

// scalarToAngle is the inverse-trig rule: a dimensionless argument, an angle
// result.
func scalarToAngle(name string, ks []units.Kind) (units.Kind, error) {
	if _, err := allDimensionless(name, ks); err != nil {
		return 0, err
	}
	return units.Angle, nil
}

// funcKindRules maps each built-in function to its kind signature. Functions not
// listed here (custom ones) are treated as dimensionless-only.
var funcKindRules = map[string]kindRule{
	"sin": angleOrScalarToScalar, "cos": angleOrScalarToScalar, "tan": angleOrScalarToScalar,
	"asin": scalarToAngle, "acos": scalarToAngle, "atan": scalarToAngle,
	"atan2": func(name string, ks []units.Kind) (units.Kind, error) {
		if _, err := sameKind(name, ks); err != nil {
			return 0, err
		}
		return units.Angle, nil
	},
	"sqrt": allDimensionless, "cbrt": allDimensionless,
	"abs":   preserveKind,
	"floor": allDimensionless, "ceil": allDimensionless, "round": allDimensionless,
	"trunc": allDimensionless, "sign": allDimensionless,
	"exp": allDimensionless, "ln": allDimensionless, "log": allDimensionless,
	"log10": allDimensionless, "log2": allDimensionless,
	"rad": func(name string, ks []units.Kind) (units.Kind, error) {
		// rad interprets a bare (dimensionless) number of degrees as an angle.
		if _, err := allDimensionless(name, ks); err != nil {
			return 0, err
		}
		return units.Angle, nil
	},
	"deg": func(name string, ks []units.Kind) (units.Kind, error) {
		// deg converts an angle to a dimensionless number of degrees.
		if len(ks) == 1 && ks[0] != units.Angle && ks[0] != units.Dimensionless {
			return 0, fmt.Errorf("%w: deg expects an angle, got a %s", ErrIncompatibleKind, ks[0].String())
		}
		return units.Dimensionless, nil
	},
	"pow": allDimensionless,
	"mod": sameKind,
	"min": sameKind, "max": sameKind, "hypot": sameKind, "clamp": sameKind,
}
