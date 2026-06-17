# Reference Geometry — Design

The sketch/3D separation keystone. Read-only 2D entities whose coordinates are
**externally locked** — handed in by the layer above as a frozen snapshot of
3D-derived geometry (a projected edge, a pierced vertex) — carrying a **source
id** (provenance) and a **staleness** flag. This is the one primitive named in
`docs/verification-roadmap.md` that keeps the library "sketches only": it holds
the *result* of a projection/intersection, it never *computes* one.

## What problem it solves

The contract (`docs/verification-roadmap.md`): "sketch on a face" and "projected
/ include / intersect" are clean *iff* the resulting 2D geometry is **passed in**
as reference geometry, not derived from a solid. Reference geometry carries those
snapshots so the engine can verify a sketch *against* them — coincidence, pierce
(= coincidence with a supplied reference point), tangency to a projected edge —
without a B-rep kernel.

It is **not** the `construction` flag. Construction geometry is solver-driven
(the solver moves it); reference geometry is **externally locked** (the solver
must never move it; only the 3D layer re-feeds it).

### The 2D guardrail (separation contract)

The reference API is strictly **2D plane-local**: coordinates in, no `space.Vec3`,
no frame, no face/edge handles, no projection. The 3D layer does the world→local
projection (`space.Frame.ToLocal`) and hands in the already-projected `(x, y)`.
Do not add a refresh/authoring helper that takes 3D types — that would pull the
*computation* into this layer and break the split.

## Model — a property on existing entities, not a new kind

Reference is three fields on the existing `Point` and on each entity, mirroring
`construction`: `reference bool`, `source string` (opaque provenance id, stored
and round-tripped, never interpreted), `stale bool`. Reusing the existing types
(not a new entity kind) keeps constraints, removal, and the geom snapshot path
unchanged. v1 covers **point / line / arc / circle**; `Ellipse`/`Spline`
reference geometry is a follow-up (they own extra solver vars — see Locking).

Reference and construction are **mutually exclusive** categories. The reference
constructors never set construction; `SetConstruction(true)` is a no-op on
reference geometry; and the loader rejects a `reference:true, construction:true`
combination (the Verify integrity check also flags it broken) — otherwise a
reference entity would be silently skipped by `Profiles()` while passing the lock
checks.

## Locking — every solver var, not just points

A reference entity is immovable only if **every** solver variable its geometry
owns is `fixed`:

- **Points** (line/arc endpoints, circle center, pierce points): both coordinate
  vars fixed.
- **Circle radius** (`ri`): a circle owns a radius var that is *not* a point —
  it must be fixed too, or a constraint could resize a "locked" reference circle.
  (Arcs derive their radius from their fixed points, so no extra var. Ellipses
  own `rxi/ryi/roti` — deferred to the follow-up for that reason.)

Fixed vars never enter the Jacobian, never get a step, never count toward DOF, so
the solver is untouched. Coordinates are **plane-local** (the 3D layer projects).

### Read-only

The lock is load-bearing — if reference coordinates drifted, the snapshot would
no longer match its 3D source. So:

- `Unfix` and `MoveTo` are **no-ops** on reference points (the lock cannot be
  lifted through the grounding API).
- The mutating **modification tools** (`Trim`/`Extend`/`Break`/`AddFillet`/
  `AddChamfer`/`AddMirror`/`AddPattern…`/`AddOffset`) **reject** a reference
  entity input (`ErrReferenceGeometry`) — they would otherwise drop provenance or
  splice in solver-driven replacements.
- `Configuration.Apply` (the ambiguity-probe restore) copies only **non-fixed**
  vars, so applying an old probe configuration can never revert refreshed
  reference coordinates.
- The only sanctioned writer of reference coordinates is the refresh API below.

### Lock integrity is enforced *and* detected (the oracle's job)

The existing entity structs have **exported** defining fields (`Line.Start`,
`Circle.Center`, …) used pervasively across the codebase; unexporting them is a
package-wide breaking refactor out of scope here. So lock integrity is held by a
combination — prevention where cheap, **detection** everywhere else, which is
exactly the oracle's mandate (never emit a false valid):

- **Construction-time:** the reference entity constructors require defining
  points that are **live reference points of this sketch** (rejecting a foreign
  point from another sketch, or a dead/removed point — `ErrForeignPoint` — so a
  reference entity can never depend on geometry outside this sketch's
  lock/refresh/stale lifecycle), fix every owned var, and record a private
  **topology seal** — the exact set of defining points the 3D layer fed (kept in
  a sketch-level `refSeals map[Entity][]*Point`, cleared on removal). `Refresh*`
  likewise rejects a point this sketch does not own.
- **Load-time:** `reference` entity ⇒ all its defining points are reference and
  every owned var is fixed, the kind is supported (point/line/arc/circle), else
  the loader rejects the document; the seal is rebuilt from the validated load.
- **Verify-time integrity check:** a reference entity is **broken** if its
  current defining points (read from the exported fields) differ from its seal,
  or any of them is not a live reference-locked point of this sketch, or any
  owned var is not fixed, or it is also flagged construction. This catches
  rewiring to a free point, rewiring to a different reference point (the seal
  mismatches), a foreign/dead point, a half-locked reference, and a
  reference+construction mix. A broken reference makes the report **not
  trustworthy** — the geometry might move, but the oracle never blesses it.

## API

Reference points (locked, with provenance):

    AddReferencePoint(x, y float64, source string) *Point

Reference curves from **existing reference points** (so projected loops close
topologically by sharing `*Point`, and the entity can never be built on free
points):

    AddReferenceLine(p1, p2 *Point, source string) (*Line, error)
    AddReferenceArc(center, start, end *Point, source string) (*Arc, error)
    AddReferenceCircle(center *Point, r float64, source string) (*Circle, error)

Each errors (`ErrNotReference`) if a supplied point is not a reference point;
`AddReferenceCircle` fixes the radius var. Reads: `IsReference()/Source()/
IsStale()` on `Point` and `Entity`.

Refresh (the 3D layer's re-feed; the only coordinate writer):

    Sketch.RefreshReference(p *Point, x, y float64) error          // ErrNotReference otherwise
    Sketch.RefreshReferenceCircle(c *Circle, r float64) error      // radius re-feed

## Staleness — marked per source, cleared only by refresh

Staleness is **marked** on the **source** (the atomic provenance unit: when a 3D
edge changes, *all* geometry derived from it goes stale together) and **cleared**
only by actually re-feeding each item — there is no `ClearStale` that could
declare a source fresh before its geometry was refreshed, and no per-handle
`SetStale` that could let a multi-point edge's bits diverge:

    Sketch.MarkStale(source string)   // set the stale bit on EVERY reference point+entity with this source

The stale bit lives only on the **atomic re-fed units** — reference **points**
(their coordinates) and a reference **circle**'s radius — the things a refresh
call actually rewrites. A **line/arc owns no coordinate of its own**, so its
staleness is **derived** (`IsStale()` is true iff any defining point is stale; a
circle, iff its center is stale or its radius bit is set).

`MarkStale(source)` therefore marks the **atomic units**, resolving *both* a
point's source and an entity's source down to those units: it sets the bit on
every reference point with that source, and for every reference *entity* with
that source it marks the entity's **sealed defining points** stale (and, for a
circle, the radius bit). So even when a projected edge carries a different source
than its vertices, `MarkStale("edge")` still makes the line's points — and hence
the derived line — stale.

Clearing happens **per unit, only on refresh**: `RefreshReference(p, x, y)`
clears a point's bit, `RefreshReferenceCircle(c, r)` clears a circle's radius
bit, which automatically clears the derived entity staleness. A source is fully
fresh only once **every** unit it owns has been re-fed; a partial refresh leaves
stale units that `Verify` reports — premature trust is impossible.

## Profiles — reference geometry participates

`Profiles()` skips only construction; reference lines/arcs join the `geom.Loops`
input and reference circles are standalone profiles. A closed region may be
bounded partly or wholly by projected edges (the "cut to a projected edge" case),
closing topologically through shared reference `*Point`s — which the point-based
constructors make possible.

## Verify — staleness and broken references as trust signals

`VerificationReport` gains:

- `StaleReferences []Entity` and `StaleReferencePoints []*Point` — the currently-
  stale reference geometry (points tracked separately because a pierce point is
  not an `Entity`).
- `BrokenReferences []Entity` — reference entities failing the lock-integrity
  check, plus any entity (reference or not) whose defining point is a foreign/dead
  handle.
- `ForeignHandles bool` — set when any point/entity reachable from the sketch's
  entities or **constraints** is not live-owned by this sketch. A foreign
  reference point reached *only* through a constraint (e.g. coincident to a
  reference point of another sketch) would otherwise pull external stale/broken
  data into the verdict unseen, so Verify reuses `constraintRefs` and the entity
  operand lists to confirm every reachable operand is live-owned. (Cross-sketch
  references are unsupported; this surfaces them instead of silently trusting
  them.)

  This makes `constraintRefs` (`removal.go`) the **single source of truth** for
  *every* point a constraint reads — including **cached** ones. The tangent
  constraints cache their endpoint-tangency contact (`shared *Point`), which
  `constraintRefs` does not currently enumerate; a rewired/removed shared point
  could drive the residual unseen. This work **extends `constraintRefs` to
  include the tangents' `shared` point**, which also closes the matching gap in
  the removal cascade (the prior arc-sweep-tangency change left it out).
- `Stale bool` — any stale reference geometry present.
- `Trustworthy() bool` — the **canonical oracle verdict**: `Solvable &&
  Status == FullyConstrained && len(Conflicts) == 0 && len(Redundant) == 0 &&
  !Stale && len(BrokenReferences) == 0 && !ForeignHandles &&
  (Probe == nil || !Probe.Ambiguous())`.
  (The probe is opt-in; if it ran and found ambiguity the verdict fails, and if
  it was not run the verdict makes no uniqueness claim — same as today.)

`Status` stays constraint-structural (Under/Fully/Over), like before; staleness
and broken references are orthogonal trust axes. The report's documented verdict
is **`Trustworthy()`**, not raw `Status` — a stale or broken-reference sketch
cannot read as a clean pass through the canonical check. Populate the new fields
in `Verify` alongside `FreePoints`/`Profiles` by scanning points and entities
(non-mutating, no solver change).

## Serialization

`jsonPoint` gains `Reference`, `Source`, `Stale`; `jsonEntity` gains
`Reference`, `Source` (and `Stale` **only meaningful for a reference circle's
radius** — line/arc staleness is derived, so `stale:true` on a reference line/arc
is rejected on load). All `omitempty`, round-tripped like `construction`/`fixed`.
On load a reference point is re-locked (reference ⇒ fixed, plus the circle radius
var); the loader **rejects** a corrupt document — a `reference` entity whose
defining points are not all reference, also flagged construction, or of an
unsupported kind (`reference:true` on an ellipse/spline, which v1 cannot fully
lock) — and rebuilds the topology seal from the validated geometry. `source` is an opaque string (no cross-id resolution,
unlike a plane `BaseID`). Fields are additive and `omitempty`, so v2 documents
without them load unchanged.

## Removal

No new cases: reference entities reuse `Line`/`Circle`/`Arc`, so
`renumberEntity`, `constraintRefs`, and `entityUsesPoint` are untouched.
`RemovePoint` still refuses while an entity uses the point; a removed reference
point's vars are already `fixed`.

## Rendering

`svg.go`/`png.go`/`dxf.go` style reference geometry distinctly (a reference color;
a `REFERENCE` DXF layer), mirroring the construction styling paths so an agent
reading the output can tell locked snapshots from solver geometry.

## Testing plan (correctness observable)

- **Locked, all vars:** after `Solve`, a reference point's coords and a reference
  circle's radius are unchanged and excluded from DOF; a constraint that would
  resize the circle is reported conflicting, not satisfied by moving it.
- **Pierce:** a free sketch point made `Coincident` with a reference point solves
  *to* the reference coordinate (the reference does not move).
- **Read-only:** `Unfix`/`MoveTo` on a reference point are inert; a modification
  tool on a reference entity returns `ErrReferenceGeometry`; `Configuration.Apply`
  after a refresh does not revert reference coords.
- **Integrity:** rewiring `refLine.Start` to a free point *and* to a different
  reference point both make `Verify` report a broken reference and `Trustworthy()`
  false (the seal mismatches); a half-locked reference (an owned var left free) is
  broken too; a constraint or entity referencing a foreign reference point (from
  another sketch) sets `ForeignHandles` and fails `Trustworthy()`.
- **Staleness:** `MarkStale(source)` ⇒ `Verify().Stale`, the stale lists, and
  `Trustworthy() == false` even for a fully-constrained solvable sketch; multi-
  point edge items mark coherently; refreshing every item restores trust, while
  refreshing only some leaves the source stale.
- **Profiles:** a loop closed by sketch lines plus a reference line, and a wholly-
  reference loop, are both detected.
- **Round-trip:** reference/source/stale/lock survive JSON; a doc with a
  reference entity on free points, or `reference:true` on an ellipse/spline, is
  rejected.
- **Removal:** a reference entity and its points remove cleanly.
- An executable `examples/` example doubling as documentation.

## Open questions / follow-ups

- Reference `Ellipse`/`Spline` (own extra vars — same locking pattern as the
  circle radius, deferred).
- Entity-level batch refresh (re-feed a whole source's geometry in one call).
- Whether a stale reference should also differ visibly in exports.
