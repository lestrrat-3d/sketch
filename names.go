package sketch

// Names & lookup. Points, entities and constraints can carry optional,
// non-unique labels that survive JSON round-trips — the durable way to refer to
// "the top edge" across a save/load boundary or from a DSL/GUI. Lookups return
// the first match in creation order. Points carry their own name field (see
// [Point.SetName]); entities embed [named]; constraint labels live on the sketch
// (a constraint is handed out only as the [Constraint] interface, so its label
// cannot live on the value).

// named is embedded by every [Entity] to carry its optional label. It is a
// separate type so the ten entity structs share one implementation of the name
// accessors rather than repeating them.
type named struct{ name string }

// Name returns the entity's optional label, or "" if unnamed.
func (n *named) Name() string { return n.name }

// SetName sets the entity's optional label. The empty string clears it.
func (n *named) SetName(name string) { n.name = name }

// SetConstraintName attaches an optional label to a constraint that has already
// been added to this sketch. Setting the empty string clears the label. A label
// is only meaningful for a live, user-facing constraint, so the call is a no-op
// for a constraint not in this sketch (nothing [Sketch.ConstraintByName] could
// resolve) and for an internal (auto-added) constraint (which never serializes,
// so its label could not survive a round-trip) — either would otherwise leave a
// dangling map entry that lookup can't find and JSON can't persist.
func (s *Sketch) SetConstraintName(c Constraint, name string) {
	if name == "" {
		delete(s.conNames, c) // clearing is always honored, even for a stale key
		return
	}
	if _, internal := c.(internalConstraint); internal {
		return
	}
	if !containsConstraint(s.cons, c) {
		return
	}
	if s.conNames == nil {
		s.conNames = make(map[Constraint]string)
	}
	s.conNames[c] = name
}

// ConstraintName returns the constraint's optional label, or "" if unnamed.
func (s *Sketch) ConstraintName(c Constraint) string { return s.conNames[c] }

// PointByName returns the first point (in creation order) with the given name,
// or nil if none matches. The empty name never matches. Names are not required
// to be unique.
func (s *Sketch) PointByName(name string) *Point {
	if name == "" {
		return nil
	}
	for _, p := range s.points {
		if p.name == name {
			return p
		}
	}
	return nil
}

// EntityByName returns the first entity (in creation order) with the given
// name, or nil if none matches. The empty name never matches. Names are not
// required to be unique.
func (s *Sketch) EntityByName(name string) Entity {
	if name == "" {
		return nil
	}
	for _, e := range s.ents {
		if e.Name() == name {
			return e
		}
	}
	return nil
}

// ConstraintByName returns the first constraint (in creation order) with the
// given name, or nil if none matches. The empty name never matches. Names are
// not required to be unique.
func (s *Sketch) ConstraintByName(name string) Constraint {
	if name == "" {
		return nil
	}
	for _, c := range s.cons {
		if s.conNames[c] == name {
			return c
		}
	}
	return nil
}
