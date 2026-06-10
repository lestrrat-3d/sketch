# Entity & Constraint Removal — Design

Status: **implemented** (`removal.go`; tests in `removal_test.go`; version
field in `json.go`). Resolves the "entity/constraint removal" open question
in CLAUDE.md, which blocks the mutating sketch tools (trim/fillet of committed
geometry, deleting a constraint from a future UI). Decided together with the
first step of JSON schema versioning, per that open question.

## The constraint to respect

Entities and points are referenced by their **index** (`id`): JSON writes
points/entities in slice order and constraints reference them by those
positions; `UnmarshalJSON` rebuilds in the same order. Any removal scheme must
keep that representation coherent.

## Decision: splice + renumber, retire variables

Of the three candidate schemes (tombstones, save-time remapping, generation
handles), choose **immediate splice + id renumbering**:

- On removal, the point/entity is spliced out of its slice and the `id` field
  of every later point/entity is decremented to match its new position. The
  serialization invariant becomes "ids equal *current* slice position"
  (today's "creation index" is the special case of never removing). Renumber
  cost is O(n) per removal — irrelevant at sketch scale. **Points and
  entities are independent id spaces**: `Point.id` indexes `s.points`,
  entity ids index `s.ents`; removing from one renumbers only that slice.
- Tombstones were rejected because every consumer (exporters, profiles,
  solver row mapping) would need a liveness filter forever; save-time
  remapping was rejected because in-memory ids and on-disk ids would diverge,
  which is exactly the kind of dual bookkeeping that breeds bugs.
- **Solver variables are retired, not reclaimed**: a removed point/circle/
  ellipse marks its `vars` slots as `fixed` (grounded) and abandons them. The
  solver and DOF analysis only consult free variables, so retired slots are
  invisible; the `vars` slice grows monotonically over a sketch's life.
  Compaction (remapping every index on every primitive) buys nothing at this
  scale and risks index bugs — explicitly rejected for now. Marshalled JSON
  never contains retired slots (it writes per-point/entity values), so reload
  naturally compacts.

## API and cascade rules

```go
func (s *Sketch) RemoveConstraint(c Constraint) bool
func (s *Sketch) RemoveEntity(e Entity) bool
func (s *Sketch) RemovePoint(p *Point) bool
```

- `RemoveConstraint` splices the constraint out; reports whether it was
  present. Internal (auto-added) constraints can be removed this way too —
  they are recreated only by their `Add…` constructor, so removing one is
  permanent for that entity; documented, not prevented (an entity cascade is
  the normal path).
- `RemoveEntity` cascades: every constraint referencing the entity (including
  its internal constraints, e.g. an arc's radius-consistency residual) is
  removed first; the entity's own scalar vars are retired; the entity is
  spliced, later entities renumbered, and a type switch deletes the entry
  from the matching generic→bound map (`lnOf`/`cirOf`/`arcOf`/`elOf`/
  `splOf`). **Var ownership, exhaustively**: only `*Circle` (`ri`) and
  `*Ellipse` (`rxi`, `ryi`, `roti`) own entity vars to retire; `*Line`,
  `*Arc` and `*Spline` own none — their coordinates belong to their points,
  which survive removal. **Its points are not removed** — points are
  first-class and may be shared; orphaned points are harmless and can be
  removed explicitly.
- `RemovePoint` is conservative: it **fails (returns false) when any entity
  still uses the point** (as endpoint, center, or spline control point) —
  removing load-bearing geometry implicitly would corrupt entities. The
  spline membership check is a linear scan of `Control` (the same point may
  legally appear more than once). With no entity user, it removes constraints
  referencing the point, retires the point's two vars (`xi`, `yi` in the
  var-indexed `fixed` array), splices and renumbers `s.points`, and deletes
  the `ptOf` entry.
- Removing things not in this sketch returns false, no-op.
- A removed handle is dead: using it afterward is undefined behavior
  (accessors read retired/stale slots). The generic→bound map entry
  (`ptOf`/`lnOf`/…) is deleted, so re-`Add`ing the same generic geometry
  creates a fresh, independent instance.

### Reference discovery

One package-level `constraintRefs(c Constraint) ([]*Point, []Entity)` type
switch — the same single-switch pattern as `marshalConstraint` — enumerates
every constraint type's references. `RemoveEntity`/`RemovePoint` consult it
with **concrete pointer identity** (a `*Radius` references exactly its
`*Circle`; no interface widening — removing an arc never cascades a circle's
constraints). A new constraint type that misses this switch fails loudly: add
a doc note in CLAUDE.md's "new constraints" checklist.

Entity-internal references count too: the switch MUST include the internal
`*arcRadius` case (returning its `a` in the entity slice), or removing an arc
strands its radius-consistency residual in `s.cons` reading a dead handle.

Constraints that reference a *point of* a removed entity but not the entity
itself (e.g. a distance dimension to a line's endpoint) are **kept** — the
point survives, so the constraint remains valid.

## Serialization: version field

`jsonSketch` gains `Version int` with **no omitempty** — always written as 1.
On read: 0 (field absent — legacy documents) and 1 load; anything greater is
rejected with an error naming the version, so future formats fail loudly
instead of mis-loading. This is the versioning hook the persistence open
question asks for. Removal itself needs no schema change — after
splice+renumber, marshal writes a dense, coherent document, and constraints
serialize the **post-removal** indices (the round-trip test must assert
against renumbered positions, not creation-time ones).

## Interactions

- **Solver/DOF**: retired vars are `fixed`, so `freeVars` skips them; DOF and
  rank are unaffected. No solver changes.
- **Row mapping** (`residuals()`/`RedundantConstraints`): unchanged — removal
  edits `s.cons` between solves, never during one.
- **Profiles/loops**: operate on live slices; nothing extra.
- **Goals**: a goal on a removed point is the caller's bug (dead handle);
  same class as constraining one.

## What this unblocks (not in this change)

Mutating sketch tools — trim/extend/fillet on committed geometry — become
expressible: build replacement geometry with the `geom` toolkit, `Add` it,
`RemoveEntity` the originals, re-attach constraints. Those tools ship
separately.

## Testing plan

- Remove a constraint → solve reflects it (a previously pinned DOF is free
  again); removing twice returns false.
- Remove an entity → its constraints (including internal ones: arc) are gone
  (count check), its vars no longer count toward DOF (circle/ellipse), later
  entity ids renumbered (assert via JSON round-trip referencing a
  later-created entity in a constraint).
- Remove a point used by a line → false, nothing changes. Remove an orphan
  point → gone, constraints on it gone, DOF drops by 2.
- Re-`Add` the same generic geometry after removal → fresh instance, not the
  dead handle.
- JSON round-trip after removals: document loads, ids coherent, solve
  matches; `"version": 1` present; a legacy document without `version` still
  loads.
- Cascade keeps unrelated constraints: dimension to a removed line's endpoint
  survives (the point remains).
