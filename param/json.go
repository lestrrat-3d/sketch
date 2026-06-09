package param

import (
	"encoding/json"
	"fmt"

	"github.com/lestrrat-3d/sketch/units"
)

// jsonEntry is the on-disk form of a single parameter. A literal value carries
// its magnitude in Value plus a Unit symbol; an expression carries Expr plus an
// optional result Unit symbol (empty == dimensionless).
type jsonEntry struct {
	Name    string   `json:"name"`
	Expr    string   `json:"expr,omitempty"`
	Value   *float64 `json:"value,omitempty"`
	Unit    string   `json:"unit,omitempty"`
	Literal bool     `json:"literal,omitempty"`
}

// MarshalJSON serializes the table as an ordered list of parameters, preserving
// definition order. Constants and functions are not serialized.
func (t *Table) MarshalJSON() ([]byte, error) {
	list := make([]jsonEntry, 0, len(t.order))
	for _, name := range t.order {
		e := t.entries[name]
		je := jsonEntry{Name: name, Unit: e.unit.Symbol()}
		if e.lit != nil {
			mag := e.lit.Mag()
			je.Value = &mag
			je.Literal = true
		} else {
			je.Expr = e.src
		}
		list = append(list, je)
	}
	return json.Marshal(list)
}

// UnmarshalJSON rebuilds the table from the list produced by [Table.MarshalJSON],
// resetting any existing parameters. Standard constants and functions are
// reinstated. Expressions are parsed (syntax is validated) but not evaluated.
func (t *Table) UnmarshalJSON(data []byte) error {
	var list []jsonEntry
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	t.entries = map[string]*entry{}
	t.order = nil
	if t.consts == nil {
		t.consts = defaultConsts()
	}
	if t.funcs == nil {
		t.funcs = defaultFuncs()
	}
	for _, je := range list {
		u := units.One
		if je.Unit != "" {
			lu, ok := units.Lookup(je.Unit)
			if !ok {
				return fmt.Errorf("param: unknown unit %q", je.Unit)
			}
			u = lu
		}
		switch {
		case je.Literal && je.Value != nil:
			if err := t.SetValue(je.Name, units.New(*je.Value, u)); err != nil {
				return err
			}
		default:
			if err := t.SetExpr(je.Name, je.Expr, u); err != nil {
				return err
			}
		}
	}
	return nil
}
