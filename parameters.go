package sketch

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/units"
)

// Dimension is a dimensional constraint (distance, radius, angle, …) whose
// driving value carries a unit and may be set literally or bound to a parameter
// expression. All of [Distance], [HorizontalDistance], [VerticalDistance],
// [DistancePointLine], [DistanceLines], [Radius], [Diameter], [Angle],
// [SemiMajor], [SemiMinor] and [EllipseRotation] satisfy it.
type Dimension interface {
	Constraint
	// Kind reports the quantity the dimension measures (length or angle).
	Kind() units.Kind
	// Target returns the current driving value, carrying its unit.
	Target() units.Value
	// Set replaces the magnitude (keeping the unit) and clears any binding.
	Set(float64)
	// SetValue replaces the value with a typed quantity of the dimension's kind.
	SetValue(units.Value) error
	// SetDriven toggles between driving the geometry and measuring it.
	SetDriven(bool)
	// Driven reports whether this is a driven (reference) dimension.
	Driven() bool

	base() float64
	setResolved(units.Value) error
	restore(float64, units.Unit)
	driverExpr() string
	setDriverExpr(string)
}

// DriverExpr returns the parameter expression bound to a dimension, or "" if
// its value is a literal.
func DriverExpr(d Dimension) string { return d.driverExpr() }

// ErrTableMismatch is returned by [Sketch.Bind] when a dimension is bound to a
// different parameter table than the one already in use by the sketch. All
// bound dimensions in a sketch share a single table.
var ErrTableMismatch = errors.New("sketch: dimensions must be bound to the same parameter table")

// Units returns the sketch's unit system (its default length and angle units).
// New sketches default to [units.Metric] (millimetres and degrees).
func (s *Sketch) Units() units.System { return s.sys }

// SetUnits sets the sketch's default length and angle units, used to interpret
// bare-float dimension values and to present results.
func (s *Sketch) SetUnits(sys units.System) { s.sys = sys }

func (s *Sketch) lengthUnit() units.Unit {
	if s.sys.Length.Kind() != units.Length {
		return units.Millimeter
	}
	return s.sys.Length
}

func (s *Sketch) angleUnit() units.Unit {
	if s.sys.Angle.Kind() != units.Angle {
		return units.Degree
	}
	return s.sys.Angle
}

func (s *Sketch) unitFor(k units.Kind) units.Unit {
	switch k {
	case units.Length:
		return s.lengthUnit()
	case units.Angle:
		return s.angleUnit()
	default:
		return units.One
	}
}

// foreignConstraint reports whether c references a point or entity this sketch
// does not own — another sketch's handle, or a dead one this sketch removed.
//
// It is the first of the two screens the doors that decide whether to
// PARAMETERIZE a constraint apply — [Sketch.AddConstraint] (resolveUnit +
// allocVars) and [Sketch.CheckConstraint]. It answers who owns the geometry the
// constraint relates, which is the only question there is for a constraint no
// sketch has parameterized yet; foreignAllocation answers the other half, whether
// the constraint still HOLDS auxiliary variables and which sketch allocated them.
// The removal path asks the owner half of that alone — see retireConstraintVars.
//
// Those hooks WRITE to the constraint: allocVars
// binds the constraint's sketch pointer before its own idempotence guard runs,
// so it rebinds even when it allocates nothing, while the constraint's stored
// variable indices still address the sketch that allocated them. Run on a handle
// another sketch already committed, that hands the donor's constraint this
// sketch's variable vector — a large index runs off it and panics the donor's
// DOF and Verify, and a small one silently reads a stranger's coordinates, both
// in a sketch that owns every one of its own handles and so has nothing in its
// own report to flag.
//
// The screen is the pair the rest of the engine already uses: constraintRefs for
// the operands, then [Sketch.owns] for a point and [Sketch.ownsEntity] for an
// entity — the SAME predicates scanReferenceIntegrity sets ForeignHandles from,
// checkNoForeignRefs refuses to marshal, and foreignInput screens tool inputs
// with, so this guard cannot diverge from what [Sketch.Verify] reports. `owns`
// carries the origin exception, so a constraint to [Sketch.Origin] (deliberately
// absent from s.points) is not foreign.
//
// A nil operand is skipped rather than reported foreign: [Sketch.Verify] splits
// a nil reference out as a corrupt one, not a foreign one, and neither hook
// dereferences it.
func (s *Sketch) foreignConstraint(c Constraint) bool {
	pts, ents := constraintRefs(c)
	for _, p := range pts {
		if p != nil && !s.owns(p) {
			return true
		}
	}
	for _, e := range ents {
		if !isNilEntity(e) && !s.ownsEntity(e) {
			return true
		}
	}
	return false
}

// corruptConstraint reports whether c cannot be read at all: a nil constraint, a
// typed nil (a nil pointer of a concrete constraint type boxed in the interface),
// or a live constraint holding a nil point or entity operand.
//
// It is the corrupt half of what foreignConstraint deliberately does not answer.
// The operand test is the same pair scanReferenceIntegrity uses to set Verify's
// nil-corrupt signal — p == nil for a point, isNilEntity for an entity, which
// catches the typed nil a concrete-pointer parameter boxes into a non-nil
// interface — so the two cannot diverge. The receiver test needs reflection
// because the sealed Constraint interface carries no isNil method (unlike
// [Entity]); it runs only on this refusal path, never on a hot one.
func corruptConstraint(c Constraint) bool {
	if isNilConstraint(c) {
		return true
	}
	pts, ents := constraintRefs(c)
	for _, p := range pts {
		if p == nil {
			return true
		}
	}
	for _, e := range ents {
		if isNilEntity(e) {
			return true
		}
	}
	return false
}

// isNilConstraint reports whether c is a nil constraint or a typed nil — a nil
// pointer of a concrete constraint type boxed into a non-nil interface, which a
// caller can write for any exported handle type (`var d *Distance`).
func isNilConstraint(c Constraint) bool {
	if c == nil {
		return true
	}
	v := reflect.ValueOf(c)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// auxOwnerOf reports the sketch that allocated a constraint's auxiliary solver
// variables, or nil when the constraint owns none or no sketch has allocated
// them yet. It reads the allocatedBy accessor the embedded auxOwner carries.
func auxOwnerOf(c Constraint) *Sketch {
	a, ok := c.(interface{ allocatedBy() *Sketch })
	if !ok {
		return nil
	}
	return a.allocatedBy()
}

// clearAuxOwnerOf forgets a constraint's allocating sketch. It is a no-op for a
// constraint owning no auxiliary variables.
func clearAuxOwnerOf(c Constraint) {
	if a, ok := c.(interface{ clearAuxOwner() }); ok {
		a.clearAuxOwner()
	}
}

// auxAllocatedOf reports whether a constraint currently HOLDS auxiliary solver
// variables, read from the aux index fields the constraint already carries
// (theta on [ArcLength], the sweep slack on [DistancePointArc] and
// [DistanceLineArc]) through the unexported auxAllocated accessor each of them
// declares beside its allocVars.
//
// A type that does not declare the accessor is reported ALLOCATED. That default
// is deliberate: it keeps the refusal below exactly as wide as it was for every
// type that has not stated the fact, so a new aux-var type is screened on its
// owner pointer alone rather than slipping through unscreened.
func auxAllocatedOf(c Constraint) bool {
	a, ok := c.(interface{ auxAllocated() bool })
	if !ok {
		return true
	}
	return a.auxAllocated()
}

// foreignAllocation reports whether c HOLDS auxiliary solver variables that a
// DIFFERENT sketch than s allocated, which is the second question the two
// parameterizing doors must ask.
//
// Reference ownership does not answer it. An exported operand field rewired to
// THIS sketch's geometry after the constraint was committed elsewhere passes
// foreignConstraint — every handle it names is local — while its stored indices
// still address the donor's variable vector. allocVars writes the constraint's
// sketch pointer ahead of its own idempotence guard, so parameterizing such a
// constraint rebinds the donor's constraint to this vector while the indices stay
// the donor's: a large index runs off this vector and panics both sketches' DOF
// and Verify, and a small one resolves onto one of this sketch's own coordinates,
// which the solver then drags with nothing anywhere to flag it.
//
// The owner pointer ALONE is not that question either, and asking it alone
// refuses candidates a receiver has every right to take. A constraint can record
// an owner while holding no live allocation, by two routes that need no removal:
// a driven dimension owns no aux variable (it contributes no residual rows), so
// [ArcLength.SetDriven] retires theta and leaves the pointer behind, and a
// dimension already driven at commit time is bound by allocVars — which writes
// the pointer before its own driven/idempotence guard — while allocating nothing
// at all. In both the stored indices address no vector, so there is nothing to
// read across sketches and nothing to refuse. Deriving the fact from the index
// fields covers both without new state and without a clearing site to forget:
// the answer follows the allocation itself. Clearing the pointer in the retire
// closure instead is NOT the fix — setDrivenAux reads that pointer to find the
// sketch to re-allocate in, so a cleared one silently drops a committed
// dimension's coupling row when it toggles back to driving.
func (s *Sketch) foreignAllocation(c Constraint) bool {
	owner := auxOwnerOf(c)
	return owner != nil && owner != s && auxAllocatedOf(c)
}

// AddConstraint commits one or more constraints to the sketch. Constraints
// reference solver-bound geometry (the [Point]/[Line]/[Circle] handles returned
// by the Add methods), which is therefore already committed. Dimensional
// constraints created from a bare float adopt the sketch's default unit for
// their kind here.
//
// A constraint referencing another sketch's geometry is committed as written but
// never parameterized (see foreignConstraint), so committing it cannot corrupt
// the sketch that owns the geometry. This sketch then reports it exactly as
// before: [Sketch.Verify] flags ForeignHandles and [Sketch.MarshalJSON] refuses
// to write it.
//
// A constraint still HOLDING auxiliary solver variables another sketch allocated
// is instead ignored entirely — not committed (see foreignAllocation). Its indices
// address that sketch's variable vector, so an appended row would read across
// sketches at every residual call. A constraint that merely records such a sketch
// while holding no live allocation — a driven dimension owns no auxiliary
// variable — is committed normally, since it addresses no other vector.
//
// A nil candidate, or a typed nil (a nil pointer of a concrete constraint type
// boxed in the interface), is DROPPED rather than committed — it names no
// geometry, so no report could describe it, and committing it would panic every
// later pass that reads it (residuals, Solve, Verify, Diagnose,
// RedundantConstraints) far from the call that made the mistake. A live
// constraint holding a nil point or entity operand IS still committed, the same
// treatment a reference-foreign constraint gets, so [Sketch.Verify] stays loud
// about it; only its resolveUnit/allocVars hooks are skipped, since they
// dereference the operands' coordinates.
func (s *Sketch) AddConstraint(cs ...Constraint) {
	for _, c := range cs {
		// Re-adding an already-committed handle is a no-op: a constraint must
		// appear at most once, or it would double-count its residual and (for
		// aux-backed constraints) its solver variables.
		if containsConstraint(s.cons, c) {
			continue
		}
		// The constraint is appended either way. Dropping a foreign one instead
		// would erase the ErrForeignHandle this sketch's report carries and make
		// the constraint vanish with nothing anywhere to flag it; skipping only the
		// hooks leaves the donor's state alone AND keeps this sketch loud. An
		// unparameterized constraint contributes only its unparameterized rows —
		// every aux-var residual gates on its own index — and Verify stops at the
		// reference-integrity scan before reading residuals at all.
		// A candidate that cannot be read at all — a nil constraint, or a typed
		// nil — is DROPPED, the treatment foreignAllocation already gets below.
		// Committing it (what an untyped nil got today, since every hook's type
		// assertion simply fails) poisons every later pass: residuals dereferences
		// it, so Solve, Verify, Diagnose and RedundantConstraints all panic, far
		// from the call that made the mistake and inside the oracle entry point
		// itself. Nothing is lost by dropping it: it names no geometry, so there is
		// nothing for a report to flag.
		if isNilConstraint(c) {
			continue
		}
		// A constraint holding a NIL OPERAND is still committed, like a
		// reference-foreign one, so Verify stays loud about it
		// (scanReferenceIntegrity reports the nil-corrupt topology and Check
		// returns ErrVerificationIncomplete) — but its hooks are skipped, because
		// allocVars reads the operands' coordinates and panics on the nil. Without
		// the skip the aux-variable types are the one family that cannot reach the
		// committed state that design intends for them.
		if !corruptConstraint(c) && !s.foreignConstraint(c) {
			// The second question: whose variable vector do the constraint's
			// auxiliary indices address? A constraint another sketch allocated is
			// DROPPED here rather than committed unparameterized, which is the
			// deliberate difference from the reference-foreign case above. An
			// appended row would keep reading the donor's vector across sketches
			// at every residual call — the very leak this screen closes — and this
			// sketch owns every handle the constraint names, so nothing in its own
			// report would say so. Nothing is lost by dropping it: the donor still
			// holds the constraint, and the rewired operand that let it reach this
			// door is what the DONOR's reference scan reports as ErrForeignHandle.
			if s.foreignAllocation(c) {
				continue
			}
			if d, ok := c.(interface{ resolveUnit(*Sketch) }); ok {
				d.resolveUnit(s)
			}
			// A constraint that needs auxiliary solver variables (e.g. an arc
			// tangency's sweep slack) allocates them here — the same hook shape as
			// resolveUnit. It runs on load too, since rebuild goes through
			// AddConstraint; the loader builds every reference against the receiving
			// sketch, so a rebuilt constraint is never foreign.
			if a, ok := c.(interface{ allocVars(*Sketch) }); ok {
				a.allocVars(s)
			}
		}
		s.cons = append(s.cons, c)
	}
}

// Params returns the parameter table the sketch's dimensions are bound against,
// or nil if no dimension has been bound yet. The table is supplied explicitly
// at [Sketch.Bind] time.
func (s *Sketch) Params() *param.Table { return s.params }

// Bind drives a dimension's value from an expression evaluated against the
// given parameter table before every solve. The table is required and becomes
// the sketch's table; binding another dimension against a different table
// returns [ErrTableMismatch]. The expression is parsed immediately so syntax
// errors surface here; the names it references are resolved at solve time.
//
//	p := param.New()
//	p.SetValue("width", units.Millimeters(120))
//	w := s.Distance(a, b, 0)
//	s.Bind(w, p, "width")
func (s *Sketch) Bind(d Dimension, table *param.Table, expr string) error {
	if table == nil {
		return fmt.Errorf("sketch: Bind requires a non-nil parameter table")
	}
	if s.params != nil && s.params != table {
		return ErrTableMismatch
	}
	if _, err := param.Parse(expr); err != nil {
		return err
	}
	s.params = table
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
//
// When a dimension is bound directly to a single named parameter, the
// parameter's kind is checked against the dimension's kind so that, for
// example, an angle parameter cannot silently drive a length.
func (s *Sketch) ApplyParameters() error {
	if s.params == nil {
		return nil
	}
	for _, c := range s.cons {
		d, ok := c.(Dimension)
		if !ok || d.Driven() {
			continue // a driven dimension measures; an expression cannot drive it
		}
		expr := d.driverExpr()
		if expr == "" {
			continue
		}
		v, err := s.evalDimension(d, expr)
		if err != nil {
			return err
		}
		if err := d.setResolved(v); err != nil {
			return fmt.Errorf("sketch: applying dimension expression %q: %w", expr, err)
		}
	}
	return nil
}

// evalDimension evaluates a bound expression to a unit-carrying value for the
// dimension's kind. When the expression is a direct parameter reference the
// parameter's value (and unit) is used and its kind checked; otherwise the
// expression is evaluated to a base-unit magnitude and tagged with the
// dimension's base unit. All conversion is left to the units library.
func (s *Sketch) evalDimension(d Dimension, expr string) (units.Value, error) {
	if name := strings.TrimSpace(expr); s.params.Has(name) {
		v, err := s.params.GetValue(name)
		if err != nil {
			return units.Value{}, fmt.Errorf("sketch: evaluating dimension parameter %q: %w", name, err)
		}
		if v.Kind() != units.Dimensionless && v.Kind() != d.Kind() {
			return units.Value{}, fmt.Errorf("sketch: %s dimension bound to %s parameter %q", d.Kind(), v.Kind(), name)
		}
		return v, nil
	}
	// A compound expression is evaluated with unit-kind tracking: mixing kinds
	// incompatibly (a length plus an angle) is rejected, and a well-formed
	// expression whose kind does not match the dimension (an angle expression
	// driving a length dimension) is rejected too. A purely dimensionless
	// expression still drives the dimension, tagged with its base unit.
	v, err := s.params.EvalValue(expr)
	if err != nil {
		return units.Value{}, fmt.Errorf("sketch: evaluating dimension expression %q: %w", expr, err)
	}
	if v.Kind() == units.Dimensionless {
		bu, ok := units.BaseUnit(d.Kind())
		if !ok {
			return units.Value{}, fmt.Errorf("sketch: %s dimension has no base unit", d.Kind())
		}
		return units.FromBase(v.Base(), bu), nil
	}
	if v.Kind() != d.Kind() {
		return units.Value{}, fmt.Errorf("sketch: %s dimension bound to %s expression %q", d.Kind(), v.Kind(), expr)
	}
	return v, nil
}
