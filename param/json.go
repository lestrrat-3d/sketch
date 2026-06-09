package param

import "encoding/json"

// jsonEntry is the on-disk form of a single parameter.
type jsonEntry struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

// MarshalJSON serializes the table as an ordered list of name/expression
// pairs, preserving definition order. Constants and functions are not
// serialized.
func (t *Table) MarshalJSON() ([]byte, error) {
	list := make([]jsonEntry, 0, len(t.order))
	for _, name := range t.order {
		list = append(list, jsonEntry{Name: name, Expr: t.entries[name].src})
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
		if err := t.Set(je.Name, je.Expr); err != nil {
			return err
		}
	}
	return nil
}
