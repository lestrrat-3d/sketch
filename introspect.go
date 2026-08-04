package sketch

// Constraint introspection. Geometric constraints are unexported types handed
// out only as the [Constraint] interface, so these package-level functions are
// how a caller — a diagnostic loop, a DSL/GUI, an exporter — asks any constraint
// what it is, what geometry it touches and how far it is from being satisfied,
// without holding the concrete type. All four are read-only — none of them
// mutates the constraint or the sketch — but they do NOT share one nil-safety
// contract, and a caller relying on that would be relying on something false.
// [ConstraintKind] and [IsInternal] are safe for every input shape: an untyped
// nil, a typed nil, a live constraint with a nil operand, or a handle whose
// operand has since been removed from the sketch. [ConstraintRefs] is safe for
// an untyped nil but PANICS on a typed nil, because it dereferences the
// concrete constraint to read its operand fields. [ConstraintResiduals] is the
// least safe of the four: it panics on both nil shapes, since evaluating a
// residual means reading operand coordinates. See each function's own comment
// for why. The asymmetry is deliberate, not an oversight — see
// [ConstraintResiduals]'s comment for why it is not guarded.

// ConstraintKind returns a stable, machine-readable identifier for the
// constraint's type — the same string used as "type" in the sketch's JSON
// serialization (e.g. "coincident", "point_on_line", "distance"). It shares its
// mapping with the serializer (via constraintKind), so it never drifts from the
// on-disk schema. The internal constraints auto-added by the geometry builders
// (never serialized) report their own stable kinds — "arc_radius" and
// "elliptical_arc_on". An unknown constraint type reports "". It is a pure type
// switch on c itself and never dereferences an operand, so it is SAFE for
// every input shape: an untyped nil, a typed nil, a constraint with a nil
// operand, and a handle whose operand has since been removed from the sketch.
func ConstraintKind(c Constraint) string {
	return constraintKind(c)
}

// ConstraintRefs returns the sketch points and entities the constraint
// references, in the order its constructor declared them. The returned slices
// are fresh on every call; either may be empty. It is safe to call on an
// UNTYPED nil constraint (c == nil): the type switch behind it matches no
// case, so it returns (nil, nil). It is NOT safe on a TYPED nil (a nil pointer
// of a concrete constraint type stored in the Constraint interface, e.g. a
// `var d *Distance` passed as-is) — the matching switch case dereferences the
// concrete pointer to read its operand fields, so a typed nil panics. On a
// live constraint holding a nil operand (e.g. one built with a nil *Point),
// it does not panic — the nil operand comes back as a nil element of the
// returned slice, unexamined. On a handle whose operand has since been
// removed from the sketch, it is also safe: it still just reads the pointer
// field, which is the removed (but non-nil) handle.
func ConstraintRefs(c Constraint) ([]*Point, []Entity) {
	return constraintRefs(c)
}

// ConstraintResiduals evaluates the constraint's residual equations at the
// current configuration — call after [Sketch.Solve] to inspect solved values.
// Residuals follow the solver's normalization: length-like equations are in
// base length units (mm), angular ones are dimensionless. A satisfied
// constraint's residuals are all within the solve tolerance of zero. Driven
// (reference) dimensions evaluate too, even though the solver ignores them.
//
// This is the one function of the four that REQUIRES a non-nil constraint
// whose operands are all non-nil: it evaluates the constraint, which means
// reading its operands' current coordinates, so it panics on an untyped nil,
// a typed nil, and a live constraint holding a nil operand — none of that is
// guarded. On a handle whose operand has since been removed from the sketch,
// it does NOT panic: the removed variable is retired (grounded, not
// reclaimed), so the read still succeeds, but the returned residual is
// computed from that stale, no-longer-live value without any indication that
// anything is wrong. That silent case is the one a caller can actually be
// harmed by.
//
// This asymmetry with [ConstraintKind]/[ConstraintRefs]/[IsInternal] is
// deliberate and is not being closed with a guard. A guard's only available
// refusal value is an empty []float64, which any caller folding "are all
// residuals within tolerance" reads as satisfied — trading a loud panic for
// exactly the silent false-positive this library's oracle design exists to
// prevent. Reaching parity with the other three would also need typed-nil
// detection for the sealed Constraint interface the way [Entity] has it via
// its unexported isNil method, enforced at a single funnel
// ([Sketch.AddConstraint]) the way [Sketch.addEntity] enforces isNil — no such
// funnel exists for constraints, so that is a materially larger change than
// this function on its own.
func ConstraintResiduals(c Constraint) []float64 {
	return c.residual(nil)
}

// IsInternal reports whether c is an auto-added internal constraint (e.g. the
// arc radius-consistency constraint added by [Sketch.CreateArc], or the
// on-ellipse constraints added by [Sketch.CreateEllipticalArc]). Internal
// constraints are never serialized — they are recreated by their entity's
// Create… constructor on load. It is a type ASSERTION against c's method set,
// which tests the constraint's dynamic type without calling any method or
// dereferencing any field, so it is SAFE for every input shape: an untyped
// nil, a typed nil, a constraint with a nil operand, and a handle whose
// operand has since been removed from the sketch.
func IsInternal(c Constraint) bool {
	_, ok := c.(internalConstraint)
	return ok
}
