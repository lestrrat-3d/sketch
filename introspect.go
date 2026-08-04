package sketch

// Constraint introspection. Geometric constraints are unexported types handed
// out only as the [Constraint] interface, so these package-level functions are
// how a caller — a diagnostic loop, a DSL/GUI, an exporter — asks any constraint
// what it is, what geometry it touches and how far it is from being satisfied,
// without holding the concrete type. All four are read-only — none of them
// mutates the constraint or the sketch — but they do NOT share one nil-safety
// contract, and a caller relying on that would be relying on something false.
//
// What holds uniformly, whatever the constraint:
//
//   - [ConstraintKind] and [IsInternal] never panic on any input a caller can
//     construct: an untyped nil, a typed nil, a live constraint with a nil
//     operand, or a handle whose operand has since been removed from the
//     sketch.
//   - [ConstraintRefs] PANICS on a typed nil, because it dereferences the
//     concrete constraint to read its operand fields, and is safe on every
//     other input shape. What a nil OPERAND comes back as is three-valued and
//     only sometimes a nil element — see its own comment.
//   - [ConstraintResiduals] is the one never to call on a handle you did not
//     get from [Sketch.Constraints].
//
// Anything finer than that is per constraint TYPE. It is decided in each
// type's residual method and constraintRefs case, not here, so it is RECORDED
// as measurements rather than stated as a rule: TestIntrospectionNilOperandOutcomes
// (introspect_nil_contract_test.go) builds one constraint from every public
// New… constructor with nil operands — for a sealed-interface operand, from
// both an untyped and a typed nil — and pins what all four functions then do.

// ConstraintKind returns a stable, machine-readable identifier for the
// constraint's type — the same string used as "type" in the sketch's JSON
// serialization (e.g. "coincident", "point_on_line", "distance"). It shares its
// mapping with the serializer (via constraintKind), so it never drifts from the
// on-disk schema. The internal constraints auto-added by the geometry builders
// (never serialized) report their own stable kinds — "arc_radius" and
// "elliptical_arc_on". An unknown constraint type reports "".
//
// It is a type switch on c itself; the one case that looks further — the
// tangent-conics case, which reads its operand's TYPE to tell a circular
// operand from an elliptical one — guards its own receiver against nil before
// reading it, and never reads an operand's coordinates. So it is SAFE for
// every input shape a caller can construct: an untyped nil, a typed nil of
// any constraint handle type, a constraint built with nil operands, and a
// handle whose operand has since been removed from the sketch.
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
// handle whose operand has since been removed from the sketch, it is safe: it
// still just reads the pointer field, which is the removed (but non-nil)
// handle.
//
// On a live constraint holding a NIL OPERAND it does not panic either, but
// what comes back is three-valued, and the commonest of the three does NOT
// compare equal to nil:
//
//   - A nil POINT operand is a genuine nil element: pts[i] == nil is true.
//   - A nil ENTITY operand passed to a CONCRETE pointer parameter (*Line,
//     *Circle, …) is boxed into a non-nil interface, so ents[i] == nil is
//     FALSE and calling any method on it panics. Most constructors take their
//     entity operands this way.
//   - A nil entity passed as an UNTYPED nil to a sealed-interface parameter
//     ([Circular] / [Elliptical], as in [NewRadius], [NewConcentric],
//     [NewTangentCircles]) stores a nil interface, so ents[i] == nil is true.
//
// So NewRadius((*Circle)(nil), 1) returns an entity element for which
// e == nil is false while reflect.ValueOf(e).IsNil() is true: a caller writing
// `if ref == nil { skip }` does NOT skip it, and its next method call panics.
// Screen an entity element with reflect.ValueOf(e).IsNil() behind an e != nil
// check, never with e == nil alone. Which constructor produces which shape is
// recorded per constructor in TestIntrospectionNilOperandOutcomes.
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
// This is the one function of the four to keep to constraints you got from
// [Sketch.Constraints]. It evaluates the constraint, which means reading its
// operands' current coordinates, so it panics on an untyped nil, on a typed
// nil, and — for most constraint types — on a live constraint holding a nil
// operand; none of that is guarded. That last case is NOT uniform: the
// aux-variable constraints (the point-on and tangent-to families of spline,
// closed spline, fit spline, conic and NURBS, plus [NewTangentEllipseCircular]
// and [NewTangentEllipses]) return an EMPTY slice instead, their residual
// returning early while the auxiliary solver variables are still unallocated,
// before any operand is read. Which constructor does which is recorded per
// constructor in TestIntrospectionNilOperandOutcomes.
//
// A residual check MUST therefore REJECT a zero-length result rather than pass
// it. An empty slice is not a nil-operand curiosity: those same aux-variable
// constraints return one with entirely VALID operands too, until
// [Sketch.AddConstraint] commits them and allocates their variables, so the
// empty slice sits on the ordinary path. (A nil operand can never reach the
// committed state for them, since allocVars panics inside AddConstraint, so an
// empty slice is the only answer they can give for one.)
//
// On a handle whose operand has since been removed from the sketch, it does
// NOT panic: the removed variable is retired (grounded, not reclaimed), so the
// read still succeeds, but the returned residual is computed from that stale,
// no-longer-live value without any indication that anything is wrong. That
// silent case is the one a caller can actually be harmed by.
//
// This asymmetry with [ConstraintKind]/[ConstraintRefs]/[IsInternal] is
// deliberate and is not being closed with a guard. A guard's only available
// refusal value is an empty []float64, which any caller folding "are all
// residuals within tolerance" reads as satisfied — and that value already
// says "this constraint is not committed yet" on the path above, so a guard
// would overload one value with two conditions a caller cannot tell apart,
// trading a loud panic for exactly the silent false-positive this library's
// oracle design exists to prevent. Reaching parity with the other three would
// also need typed-nil detection for the sealed Constraint interface the way
// [Entity] has it via its unexported isNil method, enforced at a single funnel
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
