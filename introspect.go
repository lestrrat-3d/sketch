package sketch

// Constraint introspection. Geometric constraints are unexported types handed
// out only as the [Constraint] interface, so these package-level functions are
// how a caller — a diagnostic loop, a DSL/GUI, an exporter — asks any constraint
// what it is, what geometry it touches and how far it is from being satisfied,
// without holding the concrete type. All of them are read-only.

// ConstraintKind returns a stable, machine-readable identifier for the
// constraint's type — the same string used as "type" in the sketch's JSON
// serialization (e.g. "coincident", "point_on_line", "distance"). It shares its
// mapping with the serializer (via constraintKind), so it never drifts from the
// on-disk schema. The internal constraints auto-added by the geometry builders
// (never serialized) report their own stable kinds — "arc_radius" and
// "elliptical_arc_on". An unknown constraint type reports "". It is a pure query
// with no side effects: it never dereferences the constraint's operands, so it
// is safe to call even on a constraint constructed with nil operands.
func ConstraintKind(c Constraint) string {
	return constraintKind(c)
}

// ConstraintRefs returns the sketch points and entities the constraint
// references, in the order its constructor declared them. The returned slices
// are fresh on every call; either may be empty.
func ConstraintRefs(c Constraint) ([]*Point, []Entity) {
	return constraintRefs(c)
}

// ConstraintResiduals evaluates the constraint's residual equations at the
// current configuration — call after [Sketch.Solve] to inspect solved values.
// Residuals follow the solver's normalization: length-like equations are in
// base length units (mm), angular ones are dimensionless. A satisfied
// constraint's residuals are all within the solve tolerance of zero. Driven
// (reference) dimensions evaluate too, even though the solver ignores them.
func ConstraintResiduals(c Constraint) []float64 {
	return c.residual(nil)
}

// IsInternal reports whether c is an auto-added internal constraint (e.g. the
// arc radius-consistency constraint added by [Sketch.CreateArc], or the
// on-ellipse constraints added by [Sketch.CreateEllipticalArc]). Internal
// constraints are never serialized — they are recreated by their entity's
// Create… constructor on load.
func IsInternal(c Constraint) bool {
	_, ok := c.(internalConstraint)
	return ok
}
