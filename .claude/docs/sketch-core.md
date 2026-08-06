# Sketch core — `sketch.go`, `revision.go`, `tools.go`

Detail moved out of CLAUDE.md's architecture table. Read before adding an entity type, touching grounding/ownership, changing `Sketch.Revision`, or adding a modification tool.

## Question router

| Your question | Section |
|---|---|
| What must a new entity type implement? | `Entity.isNil` — a new entity type MUST implement it |
| Which type switches must a new entity type touch? | `entity_switches_test.go` — every type switch a new entity type must touch |
| Which points define an entity? | `entityPoints` — the one definition of an entity's defining points |
| Which extra scalar variables does an entity own? | The one definition of an entity's extra scalar variables |
| Why is the origin not in `s.points`? | `s.points` deliberately excludes the origin |
| Why does grounding screen its handle? | The grounding API screens every handle for ownership |
| What does `Sketch.Revision` hash, and why? | `revision.go` — the `Sketch.Revision` fingerprint |
| Why does a modification tool refuse a handle? | `Sketch.foreignInput` — every tool screens its inputs |

Navigation only — the sections below are the authority.

## `sketch.go` — Sketch, solver-bound geometry, grounding

### Overview

`Sketch`, solver-bound geometry (`Point`/`Line`/`Circle`/`Arc`/`Ellipse`)
authored from points, the parameter model, grounding, construction flag,
`Geometry()` snapshots.

### The origin point

**Every sketch owns an origin point** (`Sketch.Origin()`, design in
`docs/origin-point-design.md`): a point at (0,0) the constructor creates with
both vars fixed for the sketch's life, so geometry is anchored by CONSTRAINING
to it rather than by `Fix` — which keeps the tie inside the constraint model,
where `Diagnose`/`RemoveConstraint` can see it.

### `s.points` deliberately excludes the origin

**It is deliberately NOT in `s.points`**: a point's `id` IS its slice position
and documents reference points by it, so putting the origin in the slice would
shift every authored id, every point count and every existing document. Keeping
it out leaves the authored id space untouched, and the passes that iterate
`s.points` (`FreePoints`, the probe, the renderers, `names.go`) correctly do not
see it. Two places needed an explicit exception, and both are load-bearing:
**`owns`** (it decides ownership positionally, so without it a constraint to the
origin reads as a FOREIGN handle and aborts verification) and
**`positionShift`** (the FD pass translates every position by the authored
centroid, and that is sound ONLY as a RIGID translation — leaving the origin
behind collapsed it onto a point constrained to it and invented a redundant
constraint; the centroid stays over the authored points so an origin-free sketch
shifts exactly as before). `freeVars` filters on `fixed`, so the origin's vars
never reach the Jacobian, rank analysis or conditioning, and DOF is unaffected.
`MoveTo`/`Unfix`/`SetConstruction` are no-ops on it and `RemovePoint` refuses it
— and so is **`UnfixEntity`** for an entity drawn from it, the easy one to
forget: it releases every defining point, so an entity with the origin as an
endpoint is a second door onto the same variables.

### The grounding API screens every handle for ownership

**The whole grounding API screens its handle for OWNERSHIP before indexing
`s.fixed` by it** — `Fix`/`Unfix` through `owns`,
`FixEntity`/`UnfixEntity`/`EntityFixed` through `foreignInput` (which screens
the entity through `ownsEntity` AND every defining point through `owns`, so a
point rewired behind an owned entity is caught too). A var index only means
something in the sketch that allocated it: a LARGE foreign index runs off
`s.fixed` and panics a documented non-panicking call, and a SMALL one is worse —
it silently grounds or releases THIS sketch's unrelated variable while the
passed handle is untouched, with nothing anywhere to flag it, and a nil handle
panics on the dereference. The refusal is a silent no-op (`false` for the
reporting `EntityFixed`, matching its existing answer for an entity with no
variables) because none of the five has an error return, the same shape
`MoveTo`/`Unfix`/`SetConstruction` already use. Using `owns` rather than a
positional check is what carries the ORIGIN exception, so `Sketch.Origin()` and
geometry drawn from it stay groundable — and it is the SAME predicate
`scanReferenceIntegrity`, `checkNoForeignRefs` and the modification tools use,
so grounding cannot diverge from what `Verify` reports and `MarshalJSON`
refuses. `grounding_ownership_test.go` pins each of the five against a foreign
handle at BOTH index regimes, against a dead and a nil (including typed-nil)
handle, against an owned entity with a rewired foreign point, and — the
load-bearing half — pins the owned-handle behaviour (point, origin, line, circle
radius, origin-drawn entity, reference geometry) unchanged. The plain builders
(`CreateLine` and friends) still take a foreign point silently — they have no
error return and manufacture nothing from its index.

### `entityPoints` — the one definition of an entity's defining points

**`entityPoints` is the ONE definition of "which points define this entity"** —
a package-level function here (its subsystem-local twin in `reference.go` is
gone), read by grounding, the removal cascade, serialization, `Verify`'s
reference/foreign-handle scan, the `Sketch.Revision` fingerprint, per-entity DOF
attribution and conflict coloring. A second copy is what a new entity type — or
a new point on an existing one — gets added to one of and forgotten in the
other, and the forgotten copy's consumers fail SILENTLY (`Verify` stops flagging
a foreign point; `Revision` stops moving, so a stale `Profile` reads fresh) with
the build, vet, lint and test gates all green. `entity_points_test.go` makes the
coverage mechanical rather than a convention: it reflects over each entity's
exported `*Point`/`[]*Point` fields — so a new point on an existing type needs
no test edit — and asserts, per slot, that rewiring it within the sketch moves
the revision (no solver var changes, so only the defining-point hash can) and
that rewiring it to another sketch's point is reported as a foreign handle. A
new entity TYPE must still be added to that test's sketch; nothing at run time
can enumerate the types implementing a sealed interface.

### The one definition of an entity's extra scalar variables

**`entityShapeVars` is the ONE definition of "which scalar variables this entity
owns BEYOND its points"** — a circle's radius, an ellipse's/elliptical arc's
semi-axes and rotation, a conic's rho, each paired with the variable's physical
KIND — read by grounding (`FixEntity`/`UnfixEntity`/`EntityFixed`), the removal
cascade's variable retirement, per-entity DOF attribution
(`EntityIsFullyConstrained`) and `varKinds`, the kind table the ambiguity probe
perturbs by and the conditioning gate scales columns by. It carries the KIND
precisely so `varKinds` needs no enumeration of its own: a second copy is what a
new entity type — or a new variable on an existing one — gets added to one of
and forgotten in the other, and the forgotten copy's consumers fail SILENTLY (a
grounded entity still free to change shape; a removed entity's variable still
free in the rank analysis; the probe perturbing a radius as if it were a
coordinate) with the build, vet, lint and test gates all green. A new entity
type owning any scalar variable MUST get a case there, each variable with its
kind. `entity_shape_vars_test.go` makes that coverage mechanical: it builds one
entity of every type — a list asserted against the entity set
`entity_switches_test.go` derives from the source — and requires `FixEntity` to
drive `DOF()` to 0 and `RemoveEntity` to leave no free variable behind. Both
verdicts come from the Jacobian's free columns rather than from the grounding
API, so a missing variable reads as a remaining degree of freedom, where
`FixEntity` and `EntityFixed` — both reading the same definition — would agree
with each other and report nothing. Restating this set (or the
`varKinds`/`entityStructuralState` sets derived from it) in prose is exactly
what drifted silently for weeks in the past, so `shape_var_pin_test.go`'s
`TestEntityShapeVarSetsArePinned` pins all three against the source with
`go/ast`; on the happy path it does nothing beyond that comparison, and on a
mismatch it hands back every registered restatement site's current text, marking
any whose anchor no longer resolves.

### `Entity.isNil` — a new entity type MUST implement it

**A new entity type MUST implement the sealed `Entity` interface's unexported
`isNil() bool`** (`return x == nil`, beside that type's `entity()`; it must
never dereference the receiver). It answers `isNilEntity`, which is how
`Verify`'s reference-integrity scan and `Sketch.ownsEntity` recognize a
TYPED-nil operand — a non-nil interface holding a nil pointer, what
`NewPointOnCircle(p, nil)` stores — before calling `entID()` on it and
panicking. The rule lives on the interface, not in a type switch, precisely so
forgetting it is a COMPILE error at `addEntity` (the total funnel, whose
parameter is `Entity`): as a switch case it was invisible to build, vet, lint
and the whole test suite, and the first consumer to hand the engine a typed nil
of the new type crashed a documented non-panicking oracle call. It is
unexported, so it is not a change to what an outside package can implement.

### Declaring the entity marker

**An entity type DECLARES ITS OWN marker directly on itself, with a POINTER
receiver** — `func (x *T) entity()` written on `T`, exactly as all ten existing
types do — and is matched in a type switch as `*T`, alone or in a
comma-separated list of such cases. **A type that acquires `Entity` by EMBEDDING
is not an entity type**: neither a struct promoting the marker from an embedded
base nor one embedding the `Entity` interface satisfies the contract, and
neither is a supported way to add geometry. The sealed `Circular`/`Elliptical`
interfaces are entity-carrying and a case may name one, but such a case never
counts as coverage — which concrete types it admits is a method set — so a
switch must enumerate the concrete types it handles, the way `conicOf` does.

### `entity_switches_test.go` — every type switch a new entity type must touch

**`entity_switches_test.go` lists every type SWITCH a new entity type must
touch**: it parses the package, derives the entity set from the `entity()`
declarations, and requires every switch naming at least one entity type to
handle all of them — `localPolyline`, `entityPoints`, `marshalBody`,
`renumberEntity`, `entityStructuralState`, `buildProfiles`, `SVG`, `PNG` and
`DXF` — with six deliberately-partial sites exempted by `(file, function)` and a
stated reason, so a switch added later is exhaustive by default and must be
listed to be excused. A case naming a NON-entity type contributes no coverage
and does not take the switch out of the audit; a switch legitimately mixing the
two is reported as partial and takes an exemption entry like any other. Only
`localPolyline` reports an unhandled type at run time; the rest fall through and
drop the entity SILENTLY (JSON loses it on round-trip, the exporters omit it,
`Profiles` excludes it, and `renumberEntity`'s miss kills a live handle after an
unrelated removal) with build, vet, lint and the whole suite green. The audit is
a SYNTACTIC check over `go/ast`, computing no method sets, so it recognizes the
supported forms above and nothing else: a value-receiver marker, a generic
receiver, an alias of a POINTER type (`type A = *Alpha`, matched `case A:`) and
an anonymous interface literal in a case clause all sit outside the contract and
are not recognized (a same-package type ALIAS of an entity — `type A = Alpha`,
matched `case *A:` — is resolved to the type it stands for). Recognizing them
would need `go/types`, measured at roughly +0.7s on this package and not taken.

## `revision.go` — the `Sketch.Revision` fingerprint

### Overview

`Sketch.Revision()`: a **fingerprint** (FNV-1a over the whole var vector + the
entity set/types/construction flags + per-entity **instance identity** +
per-entity defining points + per-entity **shape state**), not a counter —
compare for **equality only, never order**. Derived from state rather than
bumped per mutating method **on purpose**: there is no bump-site list to forget,
so a new mutation path cannot silently leave the revision stale. It must cover
everything `Profiles()` reads *and everything a `Profile` HOLDS* — coordinates
*and* topology *and* the construction flag (toggling construction changes the
region set while every coordinate stays put) *and* every shape value an entity
resolves *and* which entity INSTANCES the profile's live handles point at.

### Load-bearing rule — hash what is consumed and handed out

**Load-bearing rule: hash what `buildProfiles` CONSUMES and what a `Profile`
HANDS OUT, never a proxy for it** — not an id, not a var index, and not "it is
somewhere in `s.vars`". Three places that rule bites.

### (0) Entity instance identity

(0) **Entity instance identity:** a `Profile` stores LIVE entity handles
(`Profile.Entities`, `BoundaryEdge.Entity`) and a consumer records profiles
structurally against `Sketch.Entities()`, so the fingerprint covers WHICH
INSTANCE each handle is — a never-reused `uid` stamped by `Sketch.addEntity`
(the single funnel every entity builder goes through) from a monotonic counter,
dropped but never rewound by `RemoveEntity`. **`Revision` is a PURE READ** — it
writes nothing (no map, no counter, no lazy init), so concurrent
`Revision()`/`Profiles()` on one sketch are race-free and observing a sketch
never invalidates a `Profile` of it. A uid is stamped ONLY where an entity
enters the sketch (`addEntity`); "an entity in `s.ents` with no uid" is an
**invariant violation, not a condition to repair on read** — the funnel is total
and `Sketch.Entities()` hands out a `slices.Clone`, so it is unreachable through
the public API (`revision_internal_test.go` asserts the invariant across every
creation path: builders, compound builders, `tools.go`, reference geometry, both
loaders). `Sketch.entUID` is the pure lookup; if it ever met an unstamped entity
it hashes `unstampedUID` (`math.MaxUint64`), a value disjoint from every real
uid (which start at 1 and only increase), so an unstamped entity can never
fingerprint like a stamped one. Repairing on read instead — stamping during
`Revision` — is the bug that was removed: it made a read-looking method a
mutator and raced two concurrent readers on the uid map. The positional `id` is
NOT that identity: removal renumbers, and a `Line` owns no scalar var to retire,
so removing a line and creating an identical one leaves type, points, shape, id
and the var vector all identical while the profile's handles dangle. The uid is
in-memory state only — **never serialized**, and the counter **never rewinds,
including across an in-place rebuild**: `Sketch.UnmarshalJSON` resets the struct
(`*s = Sketch{…}`) but carries `nextEntID` over, so the rebuilt entities get
fresh uids above the retired ones. Restamping from 1 there would make a reload
of the SAME bytes reproduce the pre-rebuild revision while every entity handle a
held `Profile` owns dangles — a stale profile reading fresh on the load path.
Any new wholesale reset of a live `*Sketch` must carry the counter the same way
(`World.UnmarshalJSON` does not: it builds fresh `*Sketch` values, leaving the
old ones — and any profile of them — coherent).

### (1) Defining points

(1) **Defining points:** entity fields (`Line.Start`, …) are exported, so a
point can be rewired to a `*Point` of another sketch or to a removed handle
whose id a live point has since inherited; `buildProfiles` follows the pointer
(`Point.Geometry()` → that point's own `s.vars`) and shares one `geom.Point` per
distinct `*Point`. So each defining point is hashed as a triple — a first-seen
**sharing ordinal** (never the pointer address: not deterministic across runs),
its **id**, and the **coordinates the pointer resolves to**
(`math.Float64bits`).

### (2) Shape values

(2) **Shape values**, in `entityStructuralState` — a switch with a case per
entity type, hashing the value read through the SAME accessor `buildProfiles`
uses: `*Circle` `r()`; `*Ellipse`/`*EllipticalArc` `rx()`/`ry()`/`rot()`;
`*Conic` `rho()`; `*NURBS` `degree`/`knots`/`weights` (stored data the solver
cannot move at all). Hashing the var vector is NOT a substitute for the
solver-var ones: the vector covers the var VALUES, not the entity→var BINDING
(swap two circles' `ri` and the vector is untouched while the disks trade
radii). The var-vector hash stays as a cheap catch-all over solver state (aux
vars included). **A new entity type MUST get a case there** if `buildProfiles`
reads anything off it besides its points, or a change leaves the revision
identical and a stale `Profile` reads fresh. `revision_internal_test.go` is the
repo's one `package sketch` test file — a deliberate exception, since selector
rebinding is unreachable from the external API and exporting a rebinder to test
it would add the very footgun the fingerprint defends against.

## `tools.go` — sketch-modification tools

### Overview

Sketch-modification tools on committed geometry (`Trim`/`Extend`/`Break`,
`CreateFillet`/`CreateChamfer`, `CreateMirror`,
`CreatePatternRect`/`CreatePatternCircular`, `CreateOffset`): build-then-replace
via the `geom` toolkit + `RemoveEntity`. Design in
`docs/modification-tools-design.md`.

### `Sketch.foreignInput` — every tool screens its inputs

**Every tool screens its inputs through the ONE guard `Sketch.foreignInput`
before touching them**, plus `owns` for the non-entity handles
(`CreatePatternCircular`'s center, `Extend`'s end). A tool manufactures geometry
and constraints in THIS sketch out of the input's defining points, so an unowned
handle is not a no-op: a FOREIGN entity splices another sketch's points in (what
`Verify` reports as `ErrForeignHandle` and `MarshalJSON` refuses to write), and
a REMOVED one resurrects deleted geometry, which nothing downstream flags at
all.

### The guard screens both halves of an input

**The guard screens BOTH halves of an input — the entity through `ownsEntity`,
AND every one of its DEFINING POINTS through `owns`** — and the point half is
not implied by the entity half. `ownsEntity` is positional over `s.ents` and
never looks at points, while entity fields (`Line.Start`, …) are exported, so an
entity this sketch owns can have a point rewired to another sketch's `*Point`
and pass. Most tools then carry that point into the new geometry, where `Verify`
and `MarshalJSON` still catch it — but **three LAUNDER it**: `Trim` when the
trimmed-away side holds the foreign endpoint, and `CreateFillet`/`CreateChamfer`
when the foreign point IS the shared corner, all replace it with fresh local
points and retire the originals, so the handle stops being reachable,
`ForeignHandles` goes true→false and marshalling goes refused→succeeding. A
sketch the oracle rejected becomes one it accepts, its geometry computed from
another sketch's coordinates with nothing left anywhere to flag it. The scan
goes through **`entityPoints`**, the ONE definition of an entity's defining
points, so a new entity type or a new point slot is screened with no second list
to keep in step; ownership is **`owns`**, the SAME predicate
`scanReferenceIntegrity` and `checkNoForeignRefs` use, so the guard cannot
diverge from what `Verify` reports and `MarshalJSON` refuses — and it carries
the origin exception, so geometry drawn from `Sketch.Origin()` (deliberately
absent from `s.points`) stays trimmable and filletable.
`tools_ownership_test.go` pins a refusal per tool, the three laundering cases
(each asserting `ErrForeignHandle` survives the call), and the origin controls.

### A tool added later MUST call the guard explicitly

**A tool added later MUST call it explicitly** — one shared helper is what every
tool has to call, not a check per tool, but nothing enforces the call:
`foreignInput` is a plain method with no interface, funnel or static check
behind it, so a new tool that skips it builds and vets clean. (Contrast
`Entity.isNil`, which a new entity type cannot forget because the sealed
interface makes the omission a compile error at the total `addEntity` funnel; Go
has no equivalent for "a method must call this helper", and a tool is a free
method with an arbitrary signature.) The refusal reuses each signature's
existing reference-geometry shape — `ErrForeignEntity` from the `Create…` tools,
nil from `CreateMirror`, false from `Trim`/`Extend`/`Break` — so no signature
changes. The plain builders (`CreateLine` and friends) still take a foreign
point silently: they have no error return to refuse through.

### `unsupportedSeed` — the pattern/mirror seed allow-list

**Every pattern/mirror builder also screens its seed through the ONE allow-list
`unsupportedSeed` before committing anything.** Only `*Line`/`*Circle`/`*Arc`
can be copied, because the point-relinking interface `instantiate` builds a copy
through carries no shape transform — a rotated ellipse, a spline's control
polygon, and every other curved kind need the mirror's reflection or the
pattern's rotation applied to their SHAPE, not just relinked to copied points.
The screen is an ALLOW-LIST with a refusing default, so an entity type added
later is refused by it without being named — the worst a forgotten update can do
is refuse a kind `instantiate` could in fact copy, never silently drop one it
cannot. The refusal reuses each signature's existing shape:
`ErrUnsupportedEntity` from `CreatePatternRect`/`CreatePatternCircular`, nil
from `CreateMirror` (which has no error return, so a caller learns "refused" but
not which kind). `instantiate`'s own switch stays exactly as partial as before —
an unhandled kind reaching it is now unreachable rather than fixed, since every
caller screens first — and both switches carry an `entitySwitchExempt` entry in
`entity_switches_test.go`.
