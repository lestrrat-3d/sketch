# CLAUDE.md

Guidance for working in this repository. The project is young and many design
decisions are still open — this file captures the **vision**, the **invariants
worth protecting**, and the **questions still unsettled**. Read it before making
structural changes, and update it when a design variable gets resolved.

## Reading this file

This file is the router. It carries the vision, the universal invariants, and a
map of where each subsystem's detail lives. **Read the linked `.claude/docs/`
file BEFORE working in that area** — the detail was moved there so this file
stays cheap to load, not because it is optional.

| Area | Trigger | Doc |
|------|---------|-----|
| Entities & grounding | Adding an entity type, touching `entityPoints`/`entityShapeVars`/`Entity.isNil`, grounding, or `Sketch.Revision` | `.claude/docs/sketch-core.md` |
| Modification tools | Adding or changing a tool in `tools.go` (trim/extend/break/fillet/chamfer/mirror/pattern/offset) | `.claude/docs/sketch-core.md` |
| Diagnostics & verification | Touching rank/DOF, conflict/redundancy analysis, `Verify`, `Check`/`Trustworthy`, the probe, or the non-finite screen | `.claude/docs/diagnostics.md` |
| Profiles & geometry | Anything under `geom/`, `Sketch.Profiles`, `BoundaryEdge`/`TExact`, or the arrangement engine | `.claude/docs/profiles-geom.md` |
| Export & serialization | Changing an exporter, the JSON schema, a document version, or reference resolution | `.claude/docs/serialization.md` |
| Constraints | Adding a constraint with auxiliary variables, or changing `AddConstraint`/`CheckConstraint`/introspection | `.claude/docs/constraints.md` |
| Rendering overlays | Adding an annotation overlay, DOF colouring, the status badge, frame/grid/watermark | `.claude/docs/rendering.md` |
| External modules | Touching `r3` frames/planes, `units` conversion, or the `param` engine | `.claude/docs/modules.md` |
| Design decisions | Resolving or recording an unsettled design variable | `.claude/docs/open-questions.md` |

## What this is

A standalone, fully programmable **parametric 2D sketch engine** in Go, in the
spirit of the sketch environment in Autodesk Fusion. You build geometry in code,
relate it with geometric and dimensional constraints, and a numerical solver
moves the geometry so every constraint holds at once. Dimensions are editable,
so sketches are fully parametric.

The library is the foundation. A DSL/CLI, a GUI, and richer geometry are
expected to be built **on top of** this engine, not woven into it.

The **north-star use case** is a *headless sketch verification oracle*: a coding
agent authors a sketch and verifies it (solvability, constraint status, conflict
sets, closed profiles) before a human executes the equivalent in CAD software.
The roadmap toward that goal, and the **sketch/3D separation contract** (this
layer verifies against 3D-derived geometry it is *given*; it never *computes* it
from a solid — the seam is first-class reference geometry), live in
`docs/verification-roadmap.md`.

## North-star principles

1. **Library-first, engine at the core.** The constraint engine is the product.
   Everything else (rendering, serialization, future DSL/GUI) is a consumer of
   it and must not leak back into the solver's design.
2. **Curated dependencies.** The engine leans on the standard library plus a
   short, deliberate dependency list — do not add modules to `go.mod` without
   recording the decision here. Current approved dependencies:
   - `github.com/lestrrat-go/option/v3` — functional-options API. Used by the
     root `sketch` package only (`Sketch.SVG`, `Sketch.Solve`). The `geom`
     package keeps its **production** code standard-library-only, and `param`'s
     only production dependency is the `units` module, so both stay
     independently extractable.
   - `github.com/lestrrat-3d/r3` — the 3D coordinate-math layer (`r3.Vec`,
     `r3.Frame`), a standalone module of its own. Used by the root `sketch`
     package only (`plane.go`, `world.go`, `sketch.go`, the exporters); see
     "The `r3` module" in `.claude/docs/modules.md`.
   - `github.com/lestrrat-3d/units` — the units-of-measure layer (`units.Unit`,
     `units.Value`, `units.System`), a standalone module of its own. Used by the
     root `sketch` package and by `param`; see "The `units` module" in
     `.claude/docs/modules.md`.
   - `github.com/stretchr/testify/require` — test assertions, **test code only**
     (all packages). Never imported by production code.

   Keeping the runtime surface this small keeps the engine embeddable anywhere.
3. **Programmability over UI.** The API is the primary interface. Anything a
   user can do interactively should be expressible in code first.
4. **Correctness is observable.** Every capability ships with a test that
   asserts on solved coordinates / residuals, not just "it ran".

## Architecture at a glance

| File | Responsibility | Detail |
|---|---|---|
| `sketch.go` | `Sketch`, solver-bound geometry (`Point`/`Line`/`Circle`/`Arc`/`Ellipse`) authored from points, the parameter model, grounding, construction flag, `Geometry()` snapshots, the always-grounded origin point, and the contracts a new entity type must satisfy. | `.claude/docs/sketch-core.md` → "`sketch.go` — Sketch, solver-bound geometry, grounding" |
| `compound.go` | Compound shape builders (`CreateRectangle`/`CreatePolygon`/`CreateSlot`): primitives + shape-holding constraints, returned as a grouping handle (handle itself is not serialized). | — |
| `tools.go` | Sketch-modification tools on committed geometry (`Trim`/`Extend`/`Break`, `CreateFillet`/`CreateChamfer`, `CreateMirror`, `CreatePatternRect`/`CreatePatternCircular`, `CreateOffset`): build-then-replace via the `geom` toolkit + `RemoveEntity`. Design in `docs/modification-tools-design.md`. Every tool screens its inputs and its pattern/mirror seed. | `.claude/docs/sketch-core.md` → "`tools.go` — sketch-modification tools" |
| `profiles.go` | `Sketch.Profiles()`: closed planar regions via the `geom` arrangement engine — bare-crossing subdivision, holes/nesting, net area, per-region validity, and the `TStart`/`TEnd`/`TExact` sub-range contract. | `.claude/docs/profiles-geom.md` → "`profiles.go` — closed planar regions" |
| `revision.go` | `Sketch.Revision()`: a fingerprint over the var vector, the entity set, per-entity instance identity, defining points and shape state — compare for equality only, never order. | `.claude/docs/sketch-core.md` → "`revision.go` — the `Sketch.Revision` fingerprint" |
| `constraint.go` | `Constraint` interface and every constraint's residual + the public `New…` constructors. | — |
| `introspect.go` | Constraint introspection over the sealed `Constraint` interface: `ConstraintKind`, `ConstraintRefs`, `ConstraintResiduals`, `IsInternal`. Read-only, package-level, and NOT sharing one nil-safety contract. | `.claude/docs/constraints.md` → "`introspect.go` — constraint introspection" |
| `names.go` | Optional, non-unique labels + first-match lookup: the embedded `named` (every `Entity`'s `Name`/`SetName`), constraint labels held on the sketch (`SetConstraintName`/`ConstraintName` in `conNames`, since a constraint is only ever an interface value), and `PointByName`/`EntityByName`/`ConstraintByName`. Names survive JSON round-trips (`name` on `jsonPoint`/`jsonEntity`/`jsonConstraint`) and are purged by the removal cascade. | — |
| `solver.go` | Levenberg–Marquardt solver, numerical Jacobian, DOF/redundancy (rank) analysis. Carries the non-finite screen; `DOF` answers with maximum ignorance rather than refusing. | `.claude/docs/diagnostics.md` → "`solver.go` — solver, Jacobian, rank analysis" |
| `jacobian.go` | The local residual Jacobian: the per-`lm`-call `residualPlan` (row inventory + variable→constraint index) and `jacobianLocalInto`, which reevaluates only the constraints a perturbed variable reaches. Bit-identical to `jacobianInto`, with a whole-plan refusal wherever that equality is not provable. | `.claude/docs/diagnostics.md` → "`jacobian.go` — the local residual Jacobian" |
| `diagnose.go` | Constraint diagnostics: `conflictAnalysis`, `Diagnose`, `ConflictSet`, `CheckConstraint`, `FreePoints`/`Point.IsFullyConstrained`, `Sketch.EntityIsFullyConstrained`. Design in `docs/diagnostics-design.md`. | `.claude/docs/diagnostics.md` → "`diagnose.go` — constraint diagnostics" |
| `verify.go` | `Sketch.Verify(ctx, ...VerifyOption) *VerificationReport`: the headless-oracle aggregation layer, `Check()`/`Trustworthy()`, and the skipped-analysis contract. | `.claude/docs/diagnostics.md` → "`verify.go` — the headless-oracle report" |
| `reference.go` | Reference geometry — the sketch/3D separation keystone: read-only, externally-locked 2D snapshots of 3D-derived geometry (`CreateReferencePoint`/`CreateReferenceLine`/`CreateReferenceArc`/`CreateReferenceCircle`) carrying a `source` id + staleness; locked via `fixed[]`, a topology seal (`refSeals`), `RefreshReference`/`RefreshReferenceCircle`/`MarkStale`, and the Verify integrity/staleness/reachability scan. Design in `docs/reference-geometry-design.md`. | — |
| `probe.go` | `Sketch.ProbeConfigurations`: multi-solution ambiguity probe — a deterministic multi-start falsifier. Design in `docs/ambiguity-probe-design.md`. | `.claude/docs/diagnostics.md` → "`probe.go` — the ambiguity probe" |
| `plane.go` / `world.go` | 3D world & construction planes. `Plane` (datum = `r3.Frame` derived from a stored definition), `World` (the mandatory document root: owns planes + sketches, datum accessors `XY`/`XZ`/`YZ`, plane builders `CreatePlaneFromFrame`/`CreatePlaneFromPoints`/`CreateOffsetPlane`, `CreateSketch`, `RemovePlane`). Design in `docs/3d-planes-design.md`. | "The world & planes" below |
| `annotate.go` | Annotation-rendering overlay for `Sketch.SVG` (in-package so it can type-switch the unexported constraint types). Opt-in `SVGPNGOption`s, all default off so baseline output stays byte-identical. Design in `docs/constraint-visualization-design.md`. | `.claude/docs/rendering.md` → "`annotate.go` — annotation overlays" |
| `frame.go` | Windowed framing for `Sketch.SVG` (opt-in, default off → byte-identical baseline): `WithFrame`, `WithGrid`, `WithGridSpacing`, `WithFramePadding`, and the fixed provenance watermark. | `.claude/docs/rendering.md` → "`frame.go` — windowed framing" |
| `svg.go` / `png.go` / `dxf.go` / `json.go` / `json_world.go` | Exporters / serialization. All three exporters refuse rather than emit a non-finite or out-of-range value (`ErrNonFiniteGeometry`); `json_world.go` is the v2 `World`/`Plane` serialization + the `kind`-discriminator preflight. | `.claude/docs/serialization.md` → "Exporters — `svg.go` / `png.go` / `dxf.go`" |
| `geom/` | **Self-contained** context-agnostic 2D geometry (own package). | `.claude/docs/profiles-geom.md` → "The `geom` package (slated for extraction)" |
| `param/` | **Self-contained** parameter & expression engine (own package). | `.claude/docs/modules.md` → "The `param` package (slated for extraction)" |
| `examples/` | Executable Go examples (`Example_sketch_…` in `package examples_test`, `go test`-verified `// Output:` blocks) that double as living documentation. Never `package main` programs. | — |

### The world & planes (`plane.go`/`world.go`)

The 2D solver is **untouched**: a `Sketch` still solves in plane-local 2D. A
`Plane` carries an `r3.Frame` *computed from a stored definition* (its
provenance — the single source of truth; `Frame()` recomputes, no memoization).
A `World` is the **mandatory document root**: it owns planes (datums at ids
0/1/2) + sketches + **one shared `param.Table`** (`World.Params()`) and is the
serialization root. **Every sketch belongs to a world** — there is no standalone
sketch/plane constructor anymore. Obtain a sketch with `World.CreateSketch(plane)`
on a plane from `World.XY()`/`XZ()`/`YZ()` or a created plane
(`World.CreatePlaneFromFrame`/`CreatePlaneFromPoints`/`CreateOffsetPlane`). The
**verb convention** is settled: `New*` = package-level standalone constructor (only
`NewWorld` remains at the sketch layer); `Create*` = a World method that
manufactures a new owned object; `Add*` = a method that attaches an existing
object (the sketch's geometry builders). Load-bearing rules:

- **Global parameters are world-shared.** `World.CreateSketch(plane)` seeds the
  new sketch with `s.params = w.params`, so one global parameter drives dimensions
  across sketches. Because every sketch shares the world's (non-nil) table from
  creation, `Bind` a dimension to `s.Params()` (== `w.Params()`); binding to a
  *different* table is `ErrTableMismatch`. **Offset planes are parameter-driven**
  (`World.BindOffsetPlane(p, expr)` → a length expression on `planeDef.distExpr`,
  kind-checked, re-evaluated on every `Frame()` call with NO cache so an edit
  reflows immediately; wrong-kind surfaces through `Frame()`). `World.Verify(ctx)` →
  `WorldVerificationReport` aggregates the shared table, every plane frame, and
  each sketch's report. World docs are **v3** (top-level `parameters` + plane
  `dist_expr`); a legacy v2 world migrates by promoting identical per-sketch
  tables, rejecting conflicting ones.

- **Placement is mandatory but nil-safe.** `Sketch.plane()` defaults a nil
  placement to the owning world's `XY()` datum (zero-value/unmarshal safety net) —
  but a v2 `kind:"sketch"` document with no `plane` is **rejected**
  (`ErrMissingPlane`), not defaulted. A single-sketch document loads as an
  **implicit one-sketch world** owning the inlined plane (`datumPlaneFromJSON`
  maps the inlined datum onto the world's XY/XZ/YZ or a created frame/points
  plane; an offset plane is rejected, as only a world document carries the base it
  would reference).
- **All planes are world-owned** (XY/XZ/YZ datums, `CreatePlaneFromFrame`,
  `CreatePlaneFromPoints`, and derived `CreateOffsetPlane`). Derived planes need an
  owner + id, so they (like all planes now) exist only through a `World`.
- **`RemovePlane` mirrors `RemovePoint`**: refuses standard datums and in-use
  planes (a sketch on it, or another plane's base), else splices + renumbers ids
  densely and **tombstones** the handle (`removed=true`, `owner=nil`, `id=-1`).
  A tombstone is rejected everywhere (`owns` checks `w.planes[p.id]==p`).
- World coordinates are a read-only derived surface (`Point.World`,
  `Sketch.WorldPolyline`), raw base-unit mm like the rest of the read surface.
  `WorldPolyline` samples via the centralized curve samplers in `geom/sample.go`
  (the exporters delegate to the same math; their output is unchanged).

### External modules and extractable packages

`github.com/lestrrat-3d/r3` (3D coordinate math) and
`github.com/lestrrat-3d/units` (units of measure) live in their own
modules/repos; `param/` is a self-contained package slated for extraction. The
dependency arrows are `sketch -> r3`, `sketch -> units` and `sketch -> param`,
never the reverse.

Detail — `r3`'s scope and frame-orthonormality rule, `units`' `Kind` algebra and
the "all conversion lives there" rule, and `param`'s import restriction — is in
`.claude/docs/modules.md`.

### Building blocks vs. sketch geometry (load-bearing)

The model follows Fusion's transient-geometry / sketch-entity split.
**Transient geometry** (`geom.Point`/`Line`/…) is pure coordinate math: a
building block for the math layer and the **snapshot** an entity returns from
`Geometry()`. It carries no document state and is never committed. **Sketch
geometry** (`sketch.Point`/`Line`/…) is the durable, solver-bound entity, and
the only handle you hold. You author it directly: `s.CreatePoint(x, y)` returns a
`*Point`; the curve builders `s.CreateLine(p1, p2)`/`CreateCircle(center, r)`/
`CreateArc(c, s, e)`/`CreateEllipse(center, rx, ry, rot)`/`CreateSpline(pts…)` take those
points. **Topology is expressed by sharing a `*Point`** between entities (a
shared corner is literally one point) — there is no generic-pointer identity
map and no idempotency; each `Add…` makes a fresh entity. Constraints reference
**sketch** geometry, so they never reference un-committed geometry —
`AddConstraint` just registers them. To read an entity's current shape as a
transient value, call `Geometry()` (a fresh snapshot at the solved coords);
`geom.NewX` is for math and snapshots, never as sketch input.

### The parameter model (load-bearing)

All scalar unknowns — every point's `x`/`y`, plus whichever intrinsic shape
variables `entityShapeVars` reports for an entity's type (a circle's radius being
one example) — live in one flat vector on the `Sketch` (`vars []float64`, with a
parallel `fixed []bool`). Sketch primitives hold **indices** into that vector (no
geom back-reference). The solver reads/perturbs the vector directly. Grounding
(`fixed`) is per-point on the sketch (`s.Fix`/`Unfix`); construction status is a
settable per-entity property (`entity.SetConstruction`). Any new geometry that
introduces unknowns must allocate them via `newVar` in its `Add…` method and
reference them by index so the solver sees them automatically.

**Author for parametricity: ground, don't pin** (guidance for every consumer of
the engine, and for code the engine itself ships). Anchor a sketch by
**constraining one point to `s.Origin()`** — the engine-provided, always-grounded
point every sketch owns — rather than by calling `s.Fix`; remove the remaining
rotational DOF with **one** orientation constraint (a horizontal/vertical line),
and locate every other point with geometric + dimensional constraints on the
geometry — never by fixing its coordinates. A point the drawing does not otherwise
constrain is made determinate the same way, by one coincidence to the origin,
rather than by pinning it or calling it reference geometry. `s.Fix` remains for
the cases a constraint cannot express, and is still bounded by the rule below. A fixed coordinate is outside the parameter model: it cannot be
`Bind`-driven and will not reflow when a driving dimension changes, so pinning
interior points, non-origin points, or more than the single origin anchor is a
non-parametric anti-pattern (the SolidWorks/Fusion "avoid the Fix/Ground
constraint" rule). Fixing a second full point already over-grounds by one DOF.
The gallery builders `groundedRect`/`hexagon` follow this. (Reference geometry is
the deliberate exception — it is externally locked via `fixed[]` by design; see
`reference.go`.) A corollary: an **unsolvable over-constraint has no satisfying
configuration, so it must distort when solved** — never "fix" that distortion by
pinning points; that fakes the geometry.

A **constraint** may also own auxiliary variables when it genuinely needs them
(the arc-tangency sweep slack, and the arc-length dimension's unwrapped-sweep
variable). It allocates them in an
`allocVars(*Sketch)` method — a hook `AddConstraint` calls (the same shape as
`resolveUnit`), so it runs on initial commit and on load (rebuild goes through
`AddConstraint`). Aux vars are retired on removal via a `retireVars(*Sketch)`
hook and are **not serialized** — they are recomputed from the solved geometry
when `allocVars` re-runs on load. This is the deliberate, narrow exception to
"constraints own no vars"; ship it only with the constraint that needs it.

Two screens decide whether `AddConstraint`/`CheckConstraint` will PARAMETERIZE a
constraint: reference ownership (`foreignConstraint`) and foreign aux-variable
allocation (`foreignAllocation`). Either alone leaves a hole, and the removal
path asks a third question again.

Detail in `.claude/docs/constraints.md`, section "The two doors that
parameterize a constraint". Read it before adding any constraint that owns
auxiliary variables.

### Invariants the solver depends on

- **Residuals are unit-normalized.** Length-like residuals are in length units;
  angle/parallel/perpendicular residuals are dimensionless (`sin`/`cos` of the
  angle). This is what keeps the normal equations well-conditioned across mixed
  constraint types — it is the difference between the hexagon example solving
  exactly and getting stuck in a distorted local minimum. **When adding a
  constraint, match this convention** (divide cross/dot products by the relevant
  lengths; use `norm()` which floors away from zero). Do not introduce residuals
  in length² or length⁴.
- **Damping is Levenberg (absolute), scaled by the max diagonal of JᵀJ**, not
  per-element Marquardt scaling. This gives the minimum-norm step for
  rank-deficient / under-constrained sketches. Don't revert to `λ·A[i][i]`.
- **The Jacobian is numerical** (central differences). Simple and robust; see
  the open questions for when this might change.
- **DOF/redundancy analysis recomputes the Jacobian at the call-time
  configuration.** `rank()`/`DOF()` rebuild J via `scaledJacobian` when called — after
  `Solve` that is the *solved* point. NEVER reuse the Solve loop's
  last-iteration Jacobian for rank analysis: it is stale (evaluated one step
  before convergence) and yields wrong DOF/redundancy counts. A single `Verify`
  call builds that Jacobian once and shares it among its own analyses; it is
  never cached across calls.
- **Driven (reference) dimensions contribute no residuals.** `residuals()`
  skips any `Dimension` with `Driven() == true`, and `refreshDriven()` writes
  the measured value back into the dimension's target after every `Solve`.
  Anything that maps residual rows back to constraints (e.g.
  `RedundantConstraints`) MUST mirror `residuals()`'s iteration exactly —
  including the driven skip — or row↔constraint attribution silently shifts.
- **Goals (`WithGoal`) are transient solver rows, never constraints.** They
  exist only inside `Solve`'s pull phase; `residuals()`, `rank`, `DOF` and
  `RedundantConstraints` never see them. Goal solves are **two-phase** (pull
  on the augmented system, then polish on hard residuals only) because plain
  weighted least squares trades constraints off against an unreachable goal by
  O(w²·pull) — far above tolerance. Don't collapse the phases back into one:
  the polish pass is what makes "constraints win exactly" true.
- **The solver works in base units** (millimetre coordinates, radian angles).
  Dimensions carry a `units.Value`; their residual uses `Target().Base()` to
  reach base units. Unit conversion happens *only* in the `units` library
  (`Value.Base`/`In`/`Convert`/`FromBase`) — never by relabelling a magnitude
  (turning "1 deg" into "1 rad" is a bug). Bare-float constructors interpret
  their number in the sketch's default unit for that kind (`Sketch.Units`).

### Serialization invariants

Points and entities are referenced by their **id, which always equals their
current slice position**; removal splices and renumbers, so documents stay
dense. A sketch document is `"version": 2`, a world document `"version": 3`,
both with an explicit `"kind"`. Internal constraints are never serialized. A
foreign reference is refused rather than written.

Detail — the origin's reserved id `-1` and `jsonOriginVersion`, the foreign-
reference refusal, the version/kind preflight, and the `param` table's
definition order — is in `.claude/docs/serialization.md`, section
"Serialization invariants". Read it before changing the schema or a version.

### Profiles and `geom`

`geom/` holds transient geometry (coordinates only, no document state), the
construction toolkit, and the planar-arrangement / region engine that
`Sketch.Profiles()` consumes. It must not import `sketch`; the arrow is
`sketch -> geom`, never the reverse. Production code is standard-library-only.

Detail — the arrangement engine, exactness certificates, `TExact` semantics and
the degeneracy guards — is in `.claude/docs/profiles-geom.md`, sections "The
`geom` package (slated for extraction)" and "`profiles.go` — closed planar
regions". Read it before touching anything under `geom/`.

## Conventions

- `gofmt`, `go vet`, and `go test ./...` must all be clean before committing.
- **README code blocks are generated, not hand-written.** They are embedded from
  the compiled `examples/` tests via `<!-- INCLUDE(file[,Func]) -->` markers and
  expanded by `internal/cmd/genreadme` (stdlib-only). After changing any example
  referenced by the README, run `go generate ./...` and commit the regenerated
  `README.md` with the code. Never edit the embedded blocks by hand.
- **Optional settings use functional options**, not options structs. Each option
  group defines a typed marker interface (`SVGOption`, `SolveOption`) embedding
  `option.Interface` plus a private wrapper, `ident…` marker structs, and `With…`
  constructors; the consumer folds them into a private `…Config` struct seeded
  from a `default…Config()`. See `svg.go` / `solver.go`. The typed interface
  keeps each option group distinct (an `SVGOption` can't be passed to `Solve`).
  An option shared by several consumers follows the jwx combined-interface
  pattern: a concatenated-name interface whose concrete type carries every
  relevant marker method (`SVGPNGOption` in `svg.go` satisfies both `SVGOption`
  and `PNGOption`, so one `WithMargin(…)` value flows into either exporter;
  `SolveVerifyOption` in `solver.go` satisfies both `SolveOption` and
  `VerifyOption`, so one `WithTolerance(…)` value flows into either — keeping the
  solver's convergence target and `Verify`'s solvability threshold consistent).
- **An accessor that returns an internal slice returns a `slices.Clone` COPY of
  it** — `Sketch.Points`/`Entities`/`Constraints` and `World.Planes`/`Sketches`.
  The ELEMENTS are the live handles (that is the point: you get the real `*Point`
  / `Entity` / `*Plane` and mutate it through its own API); only the slice is
  copied, so a caller cannot write through a SLOT into engine state. The backing
  slices *are* the id spaces (id == slice position), the entity slice is stamped
  only by the `addEntity` funnel, and the constraint order is the row→constraint
  attribution every diagnostic rests on — a write-through would bypass all three
  at once (`s.Entities()[i] = &sketch.Line{…}` splices in an entity with no uid,
  no vars and no id, blinding `Sketch.Revision` and dangling every `Profile`
  handle). Engine-internal code iterates `s.ents`/`s.points`/`s.cons` directly, so
  the copy is never paid on a hot path. A `Profile`'s `Entities`/`Outer`/`Holes`
  are NOT this case: a profile is a freshly-allocated snapshot the consumer owns.
- **Tests use `testify/require`** (never `assert`) and live in **external
  `xxx_test` packages** — they exercise only the exported API. If a test needs
  to observe internal state, add a documented exported accessor rather than
  reaching into unexported fields (e.g. `Sketch.Points`, `Point.ID`,
  `Point.Geometry`, `DriverExpr`). No named return values, including in tests.
  Author geometry with the real builders (`s.CreatePoint(x,y)`, `s.CreateLine(a,b)`,
  …) directly in tests — do not wrap them in trivial 1:1 helpers; explicit is
  better.
- Geometry is authored against the sketch from points (`s.CreatePoint` then
  `s.CreateLine`/`CreateCircle`/`CreateArc`/`CreateEllipse`/`CreateSpline`); constraints come
  from package-level `New…` functions (the `New` prefix is forced for the
  dimensional ones because their concrete handle types — `Distance`, `Radius`,
  `Angle`, … — already own the bare name; keep all constructors consistent) and
  are registered with `s.AddConstraint`. `geom.NewX` is only for math/snapshots,
  never sketch input.
- Constraints reference **sketch** geometry (`*sketch.Point`/`*sketch.Line`/…),
  not transient `geom` values; the residual reads solved values through it.
  Constraints that relate centers/radii take the sealed `Circular` interface (`*Circle` or
  `*Arc`); an arc's radius is the derived `dist(Start, Center)`, so such
  residuals need no radius variable. **Arc tangency enforces the sweep** (the
  tangent must touch the arc, not just its full circle, or the oracle would
  bless a tangent that misses it). Two cases in `constraint.go`:
  *endpoint tangency* — operands that share the contact point (the fillet/slot
  case, detected by shared `*Point`) — is one clean equality (line ⊥ radius, or
  centers collinear, at the shared point), no aux var. *Interior tangency* pins
  tangency to the full circle **and** adds a slack-encoded inequality
  (`dot(contactDir, midDir) ≥ cos(sweep/2)`) keeping the contact in the sweep.
  The slack is an auxiliary solver variable (see the parameter-model note on the
  `allocVars` hook).
- Public dimensional constructors return concrete handles (`*Distance`, etc.)
  with `.Set`/`.SetValue`; geometric constructors return the `Constraint`
  interface.
- **Public constructors validate input by returning errors, never panicking.**
  The shape/pattern builders (`CreatePolygon`/`CreateSlot`/`CreateSpline`/
  `CreatePatternRect`/`CreatePatternCircular` → `ErrInvalidShape`),
  `World.CreateSketch`/`CreateOffsetPlane` (`ErrForeignPlane`), and the plane/frame
  constructors (`World.CreatePlaneFromFrame`/`CreatePlaneFromPoints`,
  `r3.ErrDegenerateFrame`, `geom.ErrTooFewControlPoints`) all return
  `(…, error)`. **Production code contains no explicit panics.** Even the pure
  spline-family math kernels (`geom.EvalCubicBSpline`/`SampleCubicBSpline`/
  `EvalCubicBSplineDeriv`/`NearestParamCubicBSpline` and the periodic/fit-point
  analogs) return a trailing `error` — the per-family sentinel
  (`ErrTooFewControlPoints`/`ErrTooFewClosedControlPoints`/`ErrTooFewFitPoints`)
  on too-few points — rather than panicking. Their callers inside the engine
  feed construction-guaranteed-valid input (the `Add…`/`New…` constructors
  already enforce the minimums), so those call sites discard the error with `_`;
  the error path is the public contract for direct kernel callers.
- Keep exported API documented with Go doc comments; primitives expose value
  accessors (`X()`, `Y()`, `R()`, …), a `Geometry()` snapshot, and measurement
  queries (`Point.DistanceTo`/`DistanceToLine`, `Line.AngleTo`), while
  index-backed fields stay unexported. Measurement math lives in `geom`
  (`geom/measure.go`); the sketch entities delegate through `Geometry()`.
- New constraints: add the residual, the `New…` constructor, a case in the JSON
  marshal/unmarshal switches, an arg-count entry in `constraintArity`
  (`json.go` — so the decoder validates references before indexing), a case in
  `constraintRefs` (`removal.go` — or the removal cascade silently misses it), a
  case in `condRowKinds` (`conditioning.go` — classify each residual row as
  length/dimensionless for the conditioning gate; an unclassified constraint makes
  the conditioning measure NaN, which fails the trust gate fail-safe — never a
  false pass — but a healthy sketch using it would then read untrustworthy), and a
  test asserting on the solved geometry.

## Open design questions

These are unsettled. If you resolve one, record the decision in
`.claude/docs/open-questions.md` — that file is the authority, and it also
records the ones already resolved (parameters, geometry coverage, solver
scale-invariance, diagnostics, visualization, units, removal, 2D → 3D).

## Status

Core engine + constraint set + solver (with DOF/redundancy analysis) +
SVG/DXF/JSON export + sketch-modification tools (`tools.go`:
trim/extend/break/fillet/chamfer/mirror/pattern/offset on committed geometry) +
3D world & construction planes (the `r3` module, `plane.go`, `world.go`: 2D
sketches placed on planes in a 3D world, local↔world transform, v2
serialization) +
unified verification (`verify.go`: `Sketch.Verify` aggregating solvability,
DOF/status, conflict sets, free points, profiles + profile validity, opt-in
ambiguity) +
reference geometry (`reference.go`: locked, externally-sourced 2D snapshots with
provenance + staleness — the sketch/3D separation keystone) +
the profile/region engine (`geom/arrange.go` + `profiles.go`: planar
arrangement of sketch geometry into closed regions with bare-crossing
subdivision, holes/nesting, net area, and self-intersection/degeneracy validity
gating `Trustworthy()`) are implemented and tested.
