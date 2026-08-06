# Constraints — introspection and the parameterization screens

Detail moved out of CLAUDE.md. Read before adding a constraint that owns auxiliary variables, changing `AddConstraint`/`CheckConstraint`, or relying on the introspection API's nil behaviour.

## Question router

| Your question | Section |
|---|---|
| Does an introspection call panic on a nil operand? | Nil-safety is recorded per constraint type |
| Why can a residual slice be empty? | `ConstraintResiduals` may return an empty slice |
| How do I screen an entity element for nil? | A nil operand is not always a nil element |
| Why was my constraint committed but not parameterized? | Reference ownership — `foreignConstraint` |
| Why was my constraint dropped outright? | Foreign allocation — `foreignAllocation` |
| Why was an aux variable never retired? | The removal path asks the owner pointer alone |

Navigation only — the sections below are the authority.

## `introspect.go` — constraint introspection

### Overview

Constraint introspection over the sealed `Constraint` interface:
`ConstraintKind` (stable type id — **derived from `marshalConstraint`** so it
never drifts from the JSON schema; internal constraints report
`arc_radius`/`elliptical_arc_on`), `ConstraintRefs` (the referenced
points+entities, via `constraintRefs`), `ConstraintResiduals`
(`c.residual(nil)`), `IsInternal`. All four are read-only, package-level — but
they do **not** share one nil-safety contract, and **only three statements about
it are uniform**: `ConstraintKind` and `IsInternal` never panic on any input a
caller can construct (a type switch and a type assertion; the type switch reads
one operand's TYPE and no operand's coordinates); `ConstraintRefs` **panics on a
typed nil** (it dereferences the concrete constraint to read its operand fields)
and is safe on every other shape; and `ConstraintResiduals` is the one never to
call on a handle that did not come from `Sketch.Constraints`.

### Nil-safety is recorded per constraint type

**Everything finer is per constraint TYPE, decided in that type's `residual`
method (`constraint.go`) and its `constraintRefs` case (`removal.go`) — neither
of them in `introspect.go` — so it is RECORDED rather than stated as a rule**:
`TestIntrospectionNilOperandOutcomes` (`introspect_nil_contract_test.go`) builds
one constraint from every public `New…` constructor with nil operands — two rows
for a sealed-interface (`Circular`/`Elliptical`) operand, since an untyped and a
typed nil behave differently — calls all four functions under `recover` and
asserts each outcome against a checked-in table.

### The constructor list is parsed from `constraint.go`

**The constructor list is PARSED from `constraint.go`** (top-level `func New…`,
the way `entity_switches_test.go` parses for `entity()`), so a constructor added
later fails the test until its outcomes are recorded; without that anchor it
escapes the contract silently, which is how the empty-residual families below
went unnoticed. Two facts a reader of `introspect.go` alone would get wrong.

### (1) `ConstraintResiduals` may return an empty slice

(1) **`ConstraintResiduals` does not always panic on a nil operand**: the twelve
aux-variable constraints (the point-on and tangent-to families of spline/closed
spline/fit spline/conic/NURBS, plus
`NewTangentEllipseCircular`/`NewTangentEllipses`) return an **EMPTY slice**,
their residual returning early while the aux variables are unallocated, before
any operand is read — and the trigger is not the nil operand: those same types
return an empty slice with entirely VALID operands until `AddConstraint` commits
them, so the empty slice sits on the ORDINARY path. **A residual check must
REJECT a zero-length result rather than pass it.**

### (2) A nil operand is not always a nil element

(2) **A nil operand is not always a nil element**: a nil POINT is (`pt == nil`
is true), but a nil ENTITY passed to a CONCRETE pointer parameter (`*Line`,
`*Circle`, … — most constructors) is boxed into a non-nil interface, so `ent ==
nil` is **FALSE** and any method call on it panics; only a sealed-interface
parameter handed an UNTYPED nil stores a nil interface. So
`NewRadius((*Circle)(nil), 1)` gives an element where `e == nil` is false while
`reflect.ValueOf(e).IsNil()` is true — screen an entity element with
`reflect.ValueOf(e).IsNil()` behind an `e != nil` check, never with `e == nil`
alone. `NewTangentEllipseCircular(nil, nil, false)` and `NewTangentEllipses(nil,
nil, false)` panic INSIDE the constructor on untyped nils, before any
introspection call; that is a recorded outcome, not a test failure. On a handle
whose operand was since **removed** from the sketch `ConstraintResiduals` does
not panic at all — the removed variable is retired (grounded, not reclaimed), so
it silently returns a stale residual computed from the leftover value. That
silent case, not the panics, is the one a caller can be harmed by; guarding it
was rejected because the only refusal value (an empty `[]float64`) reads as
"satisfied" to a caller folding residuals against tolerance AND already says
"not committed yet" on the path above, so a guard overloads one value with two
conditions a caller cannot tell apart, and because parity with the other three
would need typed-nil detection for the sealed `Constraint` interface enforced at
a single funnel the way `Entity.isNil` is enforced at `addEntity` — no such
funnel exists for constraints.

## The two doors that parameterize a constraint

### Reference ownership — `foreignConstraint`

**The two doors that decide whether to PARAMETERIZE a constraint — `AddConstraint`
(`resolveUnit` + `allocVars`) and `CheckConstraint` — screen on TWO questions, and
either alone leaves a hole.** The first is reference ownership,
`Sketch.foreignConstraint` (`parameters.go`):
`constraintRefs` for the operands, then `owns` for a point and
`ownsEntity` for an entity — the SAME predicates `scanReferenceIntegrity`,
`checkNoForeignRefs` and `foreignInput` use, so the screen cannot diverge from what
`Verify` reports and `MarshalJSON` refuses, and it carries the origin exception. The
hooks WRITE to the constraint, and `allocVars` binds the constraint's sketch pointer
BEFORE its own idempotence guard, so it rebinds even when it allocates nothing while
the stored indices still address the sketch that allocated them. Run on a handle
another sketch already committed, that hands the DONOR's constraint the receiving
sketch's variable vector: a large index runs off it and PANICS the donor's `DOF` and
`Verify`, a small one silently reads a stranger's coordinates and flips the donor from
underconstrained to overconstrained with a conflicting constraint, and `retireVars` on
the removal path grounds an unrelated variable of the receiver while resetting the
donor's indices, dropping a row from a residual the donor still evaluates. **The damage
lands on a sketch that owns every one of its own handles**, so nothing in ITS report can
see it — `ForeignHandles` is a statement about the RECEIVING sketch. `AddConstraint`
still **appends** a foreign constraint and skips only the hooks: dropping it would erase
the receiving sketch's own `ErrForeignHandle` and make the constraint vanish with nothing
anywhere to flag it. `CheckConstraint` instead refuses with an error wrapping
`ErrForeignHandle` before probing, which is what makes its documented non-mutating
contract true of the donor too (the variable rollback is installed only when the probe
actually allocated, so an already-committed candidate does not have its live aux vars
retired).

### Foreign allocation — `foreignAllocation`

**The second question is whether the constraint HOLDS aux variables ANOTHER
sketch allocated — `foreignAllocation`, which asks the owner pointer through
`allocatedBy()` AND a live allocation through `auxAllocated()` — and reference ownership
does not imply it** — a rewire in the OTHER
direction, an exported operand field pointed at the RECEIVER's own geometry after a commit
elsewhere, makes every handle the constraint names local, so the reference screen passes
while the stored indices still address the donor's vector, and `allocVars` rebinds the
donor's constraint to the receiver: the same panic at a large index and the same silent
aliasing at a small one, now with `ForeignHandles` reading FALSE in the receiver's report.
**Here the refusal DROPS the constraint rather than committing it unparameterized**, the
deliberate difference from the reference-foreign case: an appended row keeps reading the
donor's vector across sketches at every residual call, which is the leak the screen closes,
and the receiver owns every handle it names so its report would say nothing. Nothing is
lost — the rewired operand that let it reach the door is what the DONOR's reference scan
reports as `ErrForeignHandle`, so no extra `Verify` signal is needed for it.
`CheckConstraint` refuses the same candidate with a wrapped `ErrForeignHandle`.

### The owner pointer alone is not that question

**The owner pointer ALONE is not that question, and asking it alone DROPS constraints a
receiver has every right to take.** A constraint records an owner while holding no
allocation by two routes that need no removal at all: a DRIVEN dimension contributes no
residual rows, so it owns no aux variable, and `SetDriven(true)` on a committed one retires
that variable and leaves the pointer set; and a dimension ALREADY driven when it is
committed is bound by `allocVars` — which writes the pointer ahead of its own
driven/idempotence guard — while allocating nothing, so no post-commit `SetDriven` call is
involved at all. In both, no stored index addresses anyone's vector, so there is nothing to
read across sketches and nothing to refuse. **The live half is derived from the aux INDEX
FIELDS the constraint already carries**, through an unexported `auxAllocated() bool`
declared beside each type's `allocVars` (`theta >= 0` on `ArcLength`, `slack >= 0` on
`DistancePointArc` and `DistanceLineArc`): the fact follows the allocation itself, so there
is no new state and no clearing site to forget. **A type that does NOT declare the accessor
is read as ALLOCATED**, keeping the owner-pointer-only refusal for it rather than letting it
slip through unscreened. Two shapes deliberately NOT taken: keying the clearing to
`SetDriven` misses the already-driven-at-commit route entirely, and clearing the pointer in
`SetDriven`'s retire closure breaks the toggle BACK to driving — `setDrivenAux` reads that
pointer to find the sketch to re-allocate in, so a committed `ArcLength` toggled driven →
driving would silently fall back to the wrap-discontinuous `Sweep()` residual the aux
variable exists to avoid.

### Retirement ends the ownership it recorded

**Retirement also ENDS the ownership it recorded**:
`clearAuxOwner` (on `auxOwner`) is called from `retireConstraintVars` and from the probe's
rollback, which is what carries a type that reports no allocation of its own — for the three
that do, the index-derived read already answers a remove-here / add-there move. The probe
clears only what it bound — it records whether
the candidate was unowned BEFORE `allocVars`, since that hook binds the pointer ahead of
its own idempotence guard and so rebinds on the paths that allocate nothing, which the
variable rollback's gate does not cover.

### The removal path asks the owner pointer alone

**The REMOVAL path asks the OWNER POINTER
alone** — `retireConstraintVars`
(`removal.go`) retires only when THIS sketch is the one that ALLOCATED the variables, read
through `allocatedBy()` on the embedded `auxOwner` every `allocVars` writes before it
allocates anything. Reference ownership is not that answer, and the two disagree in both
directions: an exported operand field rewired AFTER the commit — to another sketch's handle,
or to a dead handle of this one, which `owns` reports the same way — makes a constraint this
sketch parameterized read foreign, so the reference screen skipped retirement and grounded
that variable forever, a phantom free DOF the sketch's own report cannot see. Both removal
doors reach it, `RemoveConstraint` and the `RemoveEntity`/`RemovePoint` cascade alike, since
the cascade matches on ONE operand while a DIFFERENT one may be the rewired one. `auxOwner`
is EMBEDDED rather than declared per type so the accessor travels with the field an aux-var
constraint has to declare anyway (`c.s` still resolves through promotion); it is the one
site a future aux-var type could still bypass by declaring its own pointer, which is the
accepted cost of adding no new state — the alternative, a set on the sketch recording what
`AddConstraint` parameterized, is forget-proof but stores the same fact twice.
`constraint_ownership_test.go` pins the donor's DOF, status and residuals across both index
regimes, the `CheckConstraint` refusal, the receiving sketch's surviving `ErrForeignHandle`,
and the unchanged allocate/solve/retire behaviour of own-geometry aux-var owners;
`constraint_retire_ownership_test.go` pins the removal half — DOF back to its pre-add value
after a rewire to a foreign handle, to a dead one, and through the `RemoveEntity` cascade on
an unrewired operand, over all three externally reachable aux-var constraints (`ArcLength`,
`DistancePointArc`, `DistanceLineArc`), plus the converse that a constraint foreign when
ADDED is still not retired. `constraint_alloc_ownership_test.go` pins the allocation half —
the drop and both sketches' unchanged state at both index regimes, the receiver's own point
left unmoved by a solve where the stale index aliased it, the `CheckConstraint` refusal, the
two `clearAuxOwner` guards (a removed constraint and a probed candidate both still
commit elsewhere, matching a dimension authored there from scratch), and the DRIVEN half —
a dimension driven after its commit and one driven before it both commit in the receiver,
over `ArcLength` and `DistancePointArc`, plus the driven → driving round trip on a committed
dimension still producing both residual rows, which is what clearing the pointer in the
retire closure would break.
