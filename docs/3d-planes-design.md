# 3D World & Construction Planes — Design

Status: **implemented** — `space/` (`Vec3`/`Frame`), `plane.go` (`Plane` +
definitions), `world.go` (`World`, placement, `RemovePlane`), sketch placement
and `Point.World`/`Sketch.WorldPolyline` in `sketch.go`, serialization v2 in
`json.go`/`json_world.go`; tests in `space/frame_test.go`, `world_test.go`,
`world_json_test.go`. Puts the 2D sketch engine inside a real 3D world: a world
coordinate system, construction planes positioned/oriented in it, 2D sketches
drawn on those planes, and a first-class bidirectional local↔world transform.

**Surfaces (NURBS / analytic) and free 3D-sketch geometry are out of scope** —
recorded as future work at the end. This design changes **nothing** in the
constraint solver.

## Model overview

```
World                      ── canonical document root; owns the world frame
 ├─ Planes  []*Plane       ── datum planes positioned in the world (XY/XZ/YZ + derived)
 └─ Sketches []*Sketch     ── each born on exactly one plane; 2D solver inside
```

- **World coordinates are pure 3D** `(x, y, z)`.
- A **construction plane** carries its own 3D local frame (origin + axes),
  defined either directly in world coordinates or hierarchically against a base
  plane.
- A **sketch** is 2D `(u, v)` geometry *on* a plane. The solver only ever sees
  2D local coordinates; world `(x, y, z)` is a **derived read-out** through the
  plane's transform.
- The engine stays at the core: a `Sketch` still owns the flat `vars` vector,
  the LM solver, **its own units and parameter table**. `World` is a thin
  spatial owner and the serialization root.

This mirrors Fusion's normal-sketch model exactly and is purely additive to the
solver. Dependency arrows: `sketch → space` (new) and the existing
`sketch → geom`, `sketch → param`, `sketch → units`. The 3D layer is a consumer
of the engine and never leaks back into it (north-star principle #1).

## Why the solver is untouched

The flat `vars` vector, LM loop, numerical Jacobian, rank/DOF analysis, and all
~21 constraint residuals stay **2D**, in plane-local coordinates. Their
unit-normalization invariant (the thing that keeps the normal equations
well-conditioned) is preserved because nothing about them changes. 3D is a
*placement transform* layered on top, not a generalization of the point model.

Everything a user authors operates in **plane-local 2D**: `AddPoint`, the curve
builders, every constraint and dimension, the modification tools, and
`Solve(WithGoal(p, x, y))`. World coordinates are read-only. A world-space drag
(projecting a world target onto the plane → a future `WithWorldGoal`) and
plane-parameter recompute outside `Solve` are *consumer-layer* concerns, not
solver changes. Free 3D geometry (points gaining a `z` var, 3D-branching
residuals) is a separate future design, deliberately not folded in.

## Package layout

One new self-contained math package, following the `geom`/`param`/`units`
precedent (stdlib-only production code, `testify/require` in tests, intended to
be independently extractable).

| Package | Holds | Imports |
|---|---|---|
| `space/` (new) | 3D vector math (`Vec3`) and the orthonormal coordinate `Frame`; the local↔world transform lives here and nowhere else. | stdlib only |
| `sketch` (root) | `World`, `Plane` (datum = frame + definition), sketch placement, `Point.World()`. | `→ space` |

`space` is to 3D what `geom` is to 2D: pure coordinate math, no document state.
`World` and `Plane` live in the root `sketch` package because they are
document-level concepts and `Sketch` holds a `*Plane` directly — keeping them
together avoids an import cycle (`World → Sketch → Plane → space`, all
one-directional). No new `go.mod` entries; everything is hand-rolled.

(Naming: `space` is the working name; alternatives `r3`/`geom3`. `geom` is the
*2D* transient layer slated for extraction, so 3D math must not be folded into
it — see "Why two math layers".)

## The `space` package

**`Vec3`** — the 3D analog of `geom.Point`:

```go
type Vec3 struct{ X, Y, Z float64 }
```

with stdlib-only helpers: `Add`, `Sub`, `Scale`, `Dot`, `Cross`, `Len`,
`Normalize`. `Normalize` returns `(Vec3, bool)` — `false` for a (near-)zero
vector — so callers cannot silently fabricate a non-unit axis (the solver's
`norm()` floors against divide-by-zero, which is **not** the same as producing a
unit vector; do not conflate them).

**`Frame`** — the local↔world bridge, a right-handed orthonormal frame. **Fields
are unexported** so a `Frame` value cannot be built or mutated into an invalid
state; it is constructor-only:

```go
type Frame struct { origin, u, v Vec3 } // invariant: u,v orthonormal; n = u×v
func (f Frame) Origin() Vec3 { return f.origin }
func (f Frame) U() Vec3 { return f.u }
func (f Frame) V() Vec3 { return f.v }
func (f Frame) N() Vec3 { return f.u.Cross(f.v) } // normal, derived — never stored
```

- Store **only** `origin, u, v`; `N` is derived (`u × v`). Single source of
  truth — there is no field that can disagree with the normal.
- `NewFrame(origin, u, v Vec3) (Frame, error)` orthonormalizes via Gram–Schmidt
  (keep `u`; make `v ⟂ u`; normalize) and returns `ErrDegenerateFrame` when `u`
  is zero or `u,v` are collinear (the projection leaves nothing to normalize).
  It is the only constructor — there is no panicking `Must` variant; even the
  compile-time-known standard datums go through `NewFrame` and propagate the
  (never-hit) error.
- **The zero value `Frame{}` is invalid, not bypassable-into-use.** Go always
  permits `var f space.Frame` / `space.Frame{}` (all-zero axes), so the
  invariant cannot be "no invalid value exists" — it is "no invalid value is
  ever *accepted*". `Frame` exposes `IsValid() bool` (axes unit and orthogonal),
  and **every public boundary that consumes a caller-supplied frame validates
  it**: `PlaneFromFrame(f)` (below) returns `ErrDegenerateFrame` on `!f.IsValid()`.
  Transform methods on a zero frame are a caller bug, the same way calling a
  method on a `nil` map is — they are not a way to smuggle a bad frame into a
  `Plane`.
- Transforms:
  - `ToWorld(local Vec3) Vec3` = `origin + u·U + v·V + w·N()`
  - `ToWorldUV(u, v float64) Vec3` = the 2D-point convenience (`w = 0`)
  - `ToLocal(world Vec3) Vec3` = dot `(world − origin)` with `u, v, N()`; the
    third component is the **signed distance off the plane**.

**Frame invariants** (the `space`-layer analog of the solver invariants —
load-bearing):

- **No invalid frame is ever accepted into a `Plane`** — enforced at the
  boundary (`NewFrame` returns an error; `PlaneFromFrame` rejects `!IsValid()`),
  not by pretending the zero value cannot be spelled. Unexported fields keep
  *external* packages from hand-rolling a non-orthonormal frame; the zero value
  is the one exception and is caught by validation.
- **The inverse transform is the transpose, NEVER a general matrix solve.**
  Orthonormality makes `ToLocal` three dot products — exact and cheap. This is
  *why* orthonormality is an invariant.
- **Round-trip is exact to tolerance:** `ToLocal(ToWorld(p)) == p`. First
  acceptance test.
- Right-handedness ties the sketch's 2D orientation to the world: the package's
  existing Y-up, CCW-positive convention (see `doc.go`) maps to CCW about `+N` in
  world, so arc winding, profile winding, and a future extrude normal stay
  unambiguous.

## Construction planes

A `Plane` is a datum entity: a `space.Frame` plus a **definition** recording how
the frame is derived. The definition is the **single source of truth**; the
frame is computed from it on demand. There is no independently-settable frame
field, so a plane can never disagree with its own provenance.

```go
type Plane struct {
    def     planeDef // provenance; the only geometric state
    owner   *World   // world that owns it; nil for a standalone (engine-only) plane
    id      int      // slice position within owner.planes; -1 when standalone
    removed bool     // tombstone set by RemovePlane; a dead handle
    name    string
}
func (p *Plane) Frame() (space.Frame, error) { /* compute from def, recursively through base */ }
```

`Frame()` **recomputes** every call (recursing into a base plane for derived
definitions). **v1 does not memoize** — recomputation is a handful of dot
products and sidesteps the cache-invalidation problem entirely (a parametric
offset change is reflected immediately, with no dirty-graph to maintain).
Memoization with a dependency-DAG dirty bit is a follow-up only if profiling
demands it. `Frame()` returns an error because a definition can be degenerate even though
the *constructors* reject bad input up front: a definition reconstructed from a
corrupt/hand-edited document, or (later) a parameter-driven input that drives a
base frame to a singular configuration, can only be caught at compute time.

Definitions fall into two coordinate-space categories — **named explicitly so
"local vs world" is never ambiguous**:

**(A) World-frame datums** — expressed directly in **world** coordinates, no
base plane:

| Constructor | Definition | Frame |
|---|---|---|
| `WorldXY()` / `WorldXZ()` / `WorldYZ()` | standard datum | origin `(0,0,0)`; axes per table below |
| `PlaneFromFrame(f space.Frame)` | explicit frame | `f` verbatim; errors on `!f.IsValid()` |
| `PlaneFromPoints(a, b, c space.Vec3)` | three **world** points | origin `a`, `U` along `a→b`, `N` from `(a→b)×(a→c)`, `V = N×U`; errors on collinear points |

Standard datum axes (all right-handed):

| Plane | U | V | N (= U×V) |
|---|---|---|---|
| XY | +X | +Y | +Z |
| XZ | +X | +Z | −Y |
| YZ | +Y | +Z | +X |

**(B) Derived planes** — expressed in the **local** frame of a base plane, so
"construction planes handle 3D local coordinates" holds; `Frame()` composes
`base.Frame()` with the local transform:

| Constructor | Definition | Frame |
|---|---|---|
| `OffsetPlane(base, dist)` | base + signed offset | same axes; origin shifted `dist` along `base.N()` |
| `PlaneAtAngle(base, axis, angle)` *(later)* | base + rotation about an in-plane axis | rotated frame |

**Derived planes exist only inside a `World`.** A base reference needs an owner
to validate against and an id to serialize, so derived planes are created **only**
through world-scoped builders (`World.OffsetPlane`, …) — there is no package-level
derived-plane constructor. Standalone (owner-less) planes are therefore always
**category (A) world-frame datums**; an engine-only user who wants a tilted plane
uses `PlaneFromFrame`/`PlaneFromPoints`, and reaches for a `World` only when they
want planes defined *relative to other planes*. This keeps the standalone path —
and its serialization — free of base chains.

**Parametric planes — the v1 boundary.** A derived plane's scalar input (e.g.
offset `dist`) may be numeric or, later, parameter/dimension-driven; changing it
makes `Frame()` return a new frame and therefore moves the world coordinates of
everything on dependent sketches. But planes are **not** part of any solver var
vector: each sketch solves independently in its own 2D space, and planes are
fixed datums at solve time. This explicitly excludes cross-sketch /
cross-plane constraints (e.g. a sketch point coincident with an edge projected
from another plane). That needs a global 3D solver or projected *reference*
geometry (the "Project/intersect" placeholder in `docs/fusion-gap-analysis.md`).
Deferred; the `planeDef` recompute is the seam where it would attach.

## The `World` root, plane ownership & removal

Planes are **created through the world**, which stamps ownership and assigns the
id. This closes the "nil / foreign / shared `*Plane`" holes — a sketch can never
be placed on a plane the world does not own, and one `*Plane` can never belong
to two worlds.

```go
func NewWorld() *World                                  // seeded with XY/XZ/YZ datums (ids 0,1,2)
func (w *World) XY() *Plane                              // the seeded standard datums
func (w *World) XZ() *Plane
func (w *World) YZ() *Plane
func (w *World) PlaneFromFrame(f space.Frame) (*Plane, error) // world-owned; errors on invalid f
func (w *World) PlaneFromPoints(a, b, c space.Vec3) (*Plane, error)
func (w *World) OffsetPlane(base *Plane, dist float64) (*Plane, error) // base must be owned by w
func (w *World) Sketch(plane *Plane) (*Sketch, error)    // plane must be owned by w (else ErrForeignPlane)
func (w *World) RemovePlane(p *Plane) error
func (w *World) Planes() []*Plane
func (w *World) Sketches() []*Sketch
```

- A fresh `World` already contains the three standard datums (Fusion parity),
  permanently at ids 0/1/2.
- Builders that take a `base`, and `World.Sketch`, validate **live membership**,
  not just `owner`: `p != nil && p.owner == w && p.id >= 0 && p.id < len(w.planes)
  && w.planes[p.id] == p`. The `w.planes[p.id] == p` clause is what rejects a
  *removed* handle (see below) whose `owner` might otherwise look right; foreign
  or removed planes return `ErrForeignPlane`.
- Base references form a **DAG, naturally creation-ordered** (you need a base
  before you can offset from it, so a base's id is always `<` its dependents').

**`RemovePlane` mirrors the existing `RemovePoint` contract** (refuse while in
use; splice + renumber) rather than inventing new semantics:

- **Refuses** (`ErrPlaneInUse`) when any sketch is placed on `p`, or any other
  plane uses `p` as its base. Like `RemovePoint` refusing while an entity uses
  the point — no silent cascade that deletes a user's sketches.
- **Refuses** (`ErrStandardDatum`) for the three seeded datums — origin planes
  are foundational and undeletable (as in Fusion).
- On success: splice `p` from `w.planes`, **renumber the later plane ids to stay
  dense and equal to slice position** (exactly the `removal.go` rule for
  points/entities, extended to a third id space), and **set the tombstone**
  `p.removed = true` (also `p.owner = nil`, `p.id = -1`). This matches
  `removal.go`'s "removed handles are dead" rule. In-memory references to
  *surviving* planes are pointers (`Sketch.plane`, a derived plane's `base`), so
  they stay valid across renumbering; ids are a serialization concern, re-derived
  from position.
- **A tombstone (`removed == true`) is distinct from a live standalone plane**
  (`owner == nil && !removed`) — the two must not share state, or a removed world
  plane would masquerade as a usable standalone plane. Every entry point rejects
  a tombstone: the world liveness check fails it (`w.planes[p.id] == p` is false
  after splicing), `NewOn` returns an error for it, and `Frame()`/serialization
  treat it as the dead handle it is.

**Engine-at-core escape hatch.** A `Sketch` is still usable without a World:
package-level `WorldXY()`/`WorldXZ()`/`WorldYZ()`/`PlaneFromFrame`/
`PlaneFromPoints` return **owner-less** planes (`owner == nil`, `id == -1`), and
`sketch.NewOn(plane *Plane) (*Sketch, error)` builds a world-less sketch on one
(`New()` is `NewOn(WorldXY())` without the error). There is no unplaced sketch — but
see the zero-value note below for how that survives Go's zero value and
`json.Unmarshal`.

**`NewOn` accepts only live owner-less planes** and returns an error for a
world-owned plane (`p.owner != nil` → `ErrWorldOwnedPlane`) **or a tombstone**
(`p.removed` → `ErrPlaneRemoved`). Placing a world-owned plane outside
`World.Sketch` would leave the sketch absent from `w.sketches`, so `RemovePlane`
could not see the plane is in use; accepting a tombstone would resurrect a dead
handle. Sketches on world-owned planes go through `World.Sketch`; sketches on
standalone planes go through `NewOn`. (A future helper can clone a world-owned
plane's definition into an owner-less plane for engine-only reuse; not v1.)

## Sketch placement & local↔world read-out

- `Sketch` gains a `plane *Plane`. `s.Plane()` returns it.
- **Zero-value / unmarshal safety.** "Mandatory placement" cannot be enforced by
  constructors alone — `var s sketch.Sketch` and `json.Unmarshal(data, &s)`
  bypass them — so a nil plane defaults to `WorldXY()` at the single internal
  access point `s.plane()`. This default is **strictly an in-memory safety net**
  (zero-value struct, so `Point.World()` can never panic), **not** a license for
  a v2 document to omit placement. The load paths are explicit:
  - **Legacy** (missing `kind`, version absent/0/1): no `plane` field exists →
    loads as world-XY. A 2D sketch *is* a world-XY sketch — the correct
    interpretation.
  - **v2 `kind:"sketch"`**: the inline `plane` object is **required**; a missing,
    null, or invalid `plane` is **rejected** (`ErrMissingPlane`), never silently
    defaulted.
  `New`/`NewOn`/`World.Sketch` always set the plane explicitly. The world-XY
  default is a real plane, not a sentinel.
- `Point.X()`, `Point.Y()` — unchanged: the **local** `(u, v)` the solver owns.
- `Point.World() space.Vec3` = lift `(p.X(), p.Y())` through `s.plane().Frame()`.
  (If `Frame()` errors — only possible for a degenerate derived plane — `World()`
  returns the origin and the error surfaces via a sibling `WorldErr` accessor;
  well-formed planes never error.)

**World read-out is points + lift — no parallel 3D curve types.** A circle on a
tilted plane is a planar curve embedded in 3D, but we do **not** introduce
`Circle3D`/`Arc3D`. The plane's `Frame` is all that is needed to place any 2D
entity in 3D. This requires a **uniform 2D sampler covering every entity type**
(line/circle/arc/ellipse/spline), which does not exist yet: today only `Spline`
exposes a public `Polyline`, while curves are sampled by *private* helpers spread
across the exporters (arcs in `svg.go`, circles/ellipses in `png.go`). v1 adds
one entity-agnostic read path for **3D consumers only** —
`Sketch.WorldPolyline(e Entity) ([]space.Vec3, error)`, backed by a local
`[]geom.Point` sampler — that maps each sample through the frame and returns the
`Frame()` error for a degenerate plane (mirroring `Point.World`/`WorldErr`).
`space` never imports `geom`; the lift bridge lives in `sketch`. The shared
sampling math is centralized in `geom` (extending the existing polyline helpers)
so the exporters' private samplers and the world sampler compute identically.
This does **not** change exporter output (see "Export & rendering").

**Units & parameters stay per-sketch (v1).** The current model — each `Sketch`
owns its `units.System` and one `param.Table`, with all of a sketch's dimensions
sharing that table — is **unchanged**. `World` does **not** own a global units
system or parameter table; `SetUnits`/`Params`/`Bind`/`ApplyParameters` remain
sketch-scoped. (A world-level *default* `System` handed to newly created sketches,
and cross-sketch shared parameters, are recorded follow-ups — deliberately not v1,
to avoid reworking the parameter-binding model.) Consistent with the current read
surface, `Point.World()` returns raw base-unit millimetres; making reads
unit-carrying stays the CLAUDE.md all-or-nothing decision.

## Why two math layers (`geom` 2D + `space` 3D)

They are **disjoint** and meet at exactly one seam: `Frame.ToWorldUV(u, v)`
consumes two bare floats (the solved local coordinates) and returns a `Vec3`.
`space` imports nothing but stdlib — not even `geom`. Modelling 2D as a special
case of 3D (z = 0 everywhere) is **rejected**: it would force a risky rewrite of
the entire 2D solver/constraint set for no near-term benefit and would
compromise `geom`'s standalone extraction. The engine stays 2D; `space` is the
placement math beside it.

## Serialization (correct-first, version 2)

Two document shapes share the schema version but are **disambiguated by an
explicit `"kind"` discriminator** so they can never be silently mis-loaded
(today `Sketch.UnmarshalJSON` ignores unknown fields, so a world document fed to
it would otherwise rebuild as an *empty* sketch):

- **World document:** `{ "kind": "world", "version": 2, "planes": [...], "sketches": [...] }`.
  The `planes` array is **dense and complete**: the three standard datums are
  always written explicitly at `planes[0:3]` (XY, XZ, YZ) and **validated on
  load** (right kind, expected axes), so sketch `plane` ids and derived `base`
  ids — which are slice positions — line up exactly as on save. There is no
  implicit/elided datum scheme. Each sketch object keeps **all** its existing
  fields (points, entities, constraints, `units`, `parameters`) plus a `plane`
  id reference into `planes`.
- **Standalone sketch:** `{ "kind": "sketch", "version": 2, …existing fields…,
  "plane": {…inline world-frame datum…} }`. A standalone plane is always a
  category-(A) world-frame datum (derived planes need a `World`), so the inline
  `plane` is a single self-contained definition — **no base, no chain, no id
  references**. This is what makes the standalone path simple.

Both shapes decode their sketch payload through **one shared, internal
`jsonSketchBody`** (kind-less, version-less: points, entities, constraints,
units, parameters). The wire shapes are flat, differing only in wrapper fields:

- standalone document = `{kind, version}` + `jsonSketchBody` fields inline +
  `plane` (an inline world-frame datum object);
- world sketch element = `jsonSketchBody` fields inline + `plane` (an int id
  into the world's `planes`) — call it `jsonWorldSketch`.

Public `Sketch.UnmarshalJSON` preflights the standalone wrapper (kind/version),
resolves the inline `plane`, then calls the shared body decoder.
`World.UnmarshalJSON` decodes each `jsonWorldSketch`, resolves `plane` against
the already-built `planes`, and calls the **same** validate-then-build body path.
Reference validation and constraint reconstruction thus live in exactly one
place for both document shapes — public `Sketch.UnmarshalJSON` is never
re-entered for a world sketch element.

Both loaders **preflight the raw top-level object** before the typed unmarshal
(today's decoders silently ignore unknown fields — `json.go` — so a world
document fed to `Sketch.UnmarshalJSON` would otherwise rebuild as an *empty*
sketch). The preflight rules:

- `version >= 2` **requires** a `kind`; an unknown `kind` is rejected
  (`ErrWrongDocumentKind`).
- `Sketch.UnmarshalJSON` rejects `kind == "world"`; `World.UnmarshalJSON` rejects
  `kind == "sketch"`.
- A **missing** `kind` is only legal for a **legacy** document (`version`
  absent/0/1) *and only when it carries no v2-only top-level key* — `plane`,
  `planes`, or `sketches` (and any future discriminator-shaped field). Such a
  document loads as a standalone world-XY sketch — the *correct* interpretation,
  not a shim. A missing-`kind` object carrying any of those keys is **rejected**
  rather than mis-parsed (`plane` matters as much as `planes`/`sketches`: a
  legacy decoder would silently drop it and lose the placement, exactly the bug
  the discriminator exists to prevent).

`jsonVersion` bumps 1 → 2; newer versions are still rejected.

Plane serialization (single source of truth on disk too):

- A plane writes its **definition**, discriminated by kind:
  `worldXY`/`worldXZ`/`worldYZ` / `frame{origin,u,v}` / `points{a,b,c}` (world
  datums, the only kinds a standalone plane can be) / `offset{base_id, dist}`
  (derived; `base_id` is an int index into the world's `planes`). The
  `offset{base_id,…}` form appears **only in a world document** — standalone
  sketches never contain a derived plane, so there is no inline-base form and no
  decoder ambiguity.
- The frame is **recomputed on load**, never trusted from disk.
- **Load-time reference validation (a pass before indexing).** The v2 decoder
  validates *all* references up front rather than indexing blindly: in a world
  document every `base_id` must point to an **earlier** plane (the DAG is
  creation-ordered) and every sketch `plane` id must be in range; a
  forward/cyclic `base_id` or out-of-range id is rejected. (This also tightens
  the existing point/entity/constraint reference handling, which currently
  indexes directly — the v2 decoder gains an explicit validate-then-index step.)

The internal-constraint and definition-order serialization invariants in
CLAUDE.md carry over unchanged.

## Export & rendering

- **2D exporters are untouched.** Each keeps its current strategy: SVG emits
  native `<line>`/`<circle>`/`<ellipse>` and samples arcs/splines; PNG rasterizes
  via sampled curve helpers; DXF emits native entities. The new `WorldPolyline`
  sampler is an **additive 3D read path** — the exporters do not route through it,
  so their byte output is unchanged. (The only shared change is centralizing the
  *curve-sampling math* in `geom` so the exporters' private samplers and
  `WorldPolyline` compute identically; the exporters' element choices are
  unaffected.)
- **World-space DXF is non-trivial — not a coordinate swap.** Current `dxf.go`
  emits `z = 0` plus 2D OCS arc angles and ellipse axes; a tilted world circle/
  arc/ellipse needs OCS extrusion vectors or true 3D entities. `Sketch.DXF` stays
  plane-local; a `World.DXF3D` is **separate deferred work**, not a "cheap option".
- **3D viewing is a follow-up.** Rendering several sketches/planes together needs
  a projection: a minimal `View` (eye/target/up, orthographic first) mapping
  world→screen so the stdlib PNG rasterizer can emit a 3D sanity-check image for
  agents.

## Naming clean-up (prerequisite, not optional)

The codebase already overloads "world" and "axis" for *sketch-local* concepts;
these become wrong once a real world frame exists and must be reworded as part of
this work:

- `tools.go` "world-frame intersection" → **plane-local** intersection.
- `compound.go` "axis-aligned" (rectangles) → aligned to the **plane's local
  axes**.
- Audit `doc.go` "Coordinates are Y-up …" to state it describes **plane-local**
  coordinates.

## Build order

One coherent design, built in dependency order (each step independently testable;
steps 1–3 touch no existing solver/constraint code beyond the naming clean-up):

1. **`space`** — `Vec3` (`Normalize` → `(Vec3,bool)`), `Frame` (unexported
   fields, `NewFrame` error path — no `Must` variant), transforms + orthonormality/
   round-trip/degenerate-rejection tests.
2. **`Plane`** — world-frame datums (`WorldXY/XZ/YZ`, `PlaneFromFrame` with
   `IsValid` validation, `PlaneFromPoints`), `planeDef` provenance, recompute
   `Frame() (Frame, error)`, tests. (Derived planes land with the `World` in
   step 3, since a base reference needs an owner.)
3. **`World` + placement** — `NewWorld` (seeded datums), world-scoped plane
   builders incl. derived `OffsetPlane` with live-membership validation,
   `World.Sketch`, `RemovePlane` (in-use/standard-datum refusal + id renumbering
   + tombstone), `sketch.NewOn` (live ownerless only), `New()` = `NewOn(WorldXY())`,
   nil-plane default, `Point.World()`, the `WorldPolyline` sampler. Naming
   clean-up lands here.
4. **Serialization v2** — `"kind"` discriminator + preflight/reject paths, the
   shared internal `jsonSketchBody`, world document root with datums at
   `planes[0:3]`, plane definitions with reference (DAG) load-validation,
   standalone inline world-frame datum, legacy→world-XY load.
5. **(Follow-up)** 3D `View`/projection for multi-sketch world PNG/SVG; world DXF.

## Open questions

- **Parametric planes in a global solve / cross-plane constraints.** Out of
  scope; `planeDef` recompute is the attach seam.
- **World-level units & shared parameters.** v1 keeps both per-sketch; a default
  `System` for new sketches and cross-sketch shared tables are deferred.
- **Frame memoization.** v1 recomputes; revisit only under profiling, and only
  with an explicit dependency-DAG dirty graph.
- **World drag.** `WithWorldGoal` (project a world target onto the plane) is a
  later consumer-layer addition; the solver stays plane-local.
- **3D rendering / camera model & world DXF (OCS vs 3D entities).**
- **Package naming:** `space` vs `r3` vs `geom3`.
- **Free 3D-sketch mode** (points gain a `z` var, reduced 3D constraint set):
  a separate, more invasive future design.

## Testing plan (correctness observable)

- **`space`:** `ToLocal(ToWorld(p))` round-trips to tolerance; `NewFrame` from
  skew inputs returns an orthonormal/right-handed frame (`N == U×V`, unit axes);
  `NewFrame` with zero/collinear axes returns `ErrDegenerateFrame`;
  `Frame{}.IsValid()` is false and `PlaneFromFrame(space.Frame{})` errors; known
  maps — on `WorldXZ()`, local `(1,0)→(1,0,0)` and `(0,1)→(0,0,1)`.
- **`Plane`:** an offset plane composes its base's frame — its origin shifts
  along `base.N()` by `dist` (so `OffsetPlane(WorldXY(), d)` shifts world `z` by
  `d`, leaving `x,y`); `PlaneFromPoints` yields the expected normal and rejects
  collinear points; a definition reconstructs an identical frame.
- **`World`/Sketch:** a unit square authored on `XZ()` has the expected world
  coordinates; `New()` equals `NewOn(WorldXY())`; a zero-value/unmarshaled
  or legacy-unmarshaled sketch reads as world-XY and `Point.World()` does not
  panic, while a v2 `kind:"sketch"` with no `plane` is rejected (`ErrMissingPlane`);
  `World.Sketch`
  with a foreign plane errors and `NewOn` errors on a world-owned plane; the
  full existing 2D suite passes unchanged.
- **`RemovePlane`:** refuses a plane under a sketch or used as a base; refuses a
  standard datum; on a free plane, splices and renumbers ids densely; pointer
  references to survivors stay valid; a removed handle fails the liveness check
  (rejected by `World.Sketch`/`OffsetPlane`).
- **Serialization:** a world document round-trips (datums at `planes[0:3]` +
  derived `offset{base_id}` planes + sketches + plane id references + per-sketch
  units/params); a standalone sketch round-trips with a single inline
  category-(A) world-frame datum (no base, no chain); a standalone document
  carrying a derived/`offset`/nested-`base` plane is rejected (unsupported);
  `Sketch.UnmarshalJSON` rejects a `kind:"world"` document and a missing-`kind`
  object carrying **any** v2-only key — separate cases for `plane`, `planes`, and
  `sketches`; a forward/cyclic `base_id` is rejected; a legacy v1 document loads
  as a world-XY sketch.
