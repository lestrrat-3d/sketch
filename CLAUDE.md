# CLAUDE.md

Guidance for working in this repository. The project is young and many design
decisions are still open — this file captures the **vision**, the **invariants
worth protecting**, and the **questions still unsettled**. Read it before making
structural changes, and update it when a design variable gets resolved.

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
     root `sketch` package only (`Sketch.SVG`, `Sketch.Solve`). The `geom`,
     `param` and `units` packages keep their **production** code standard-
     library-only so they stay independently extractable.
   - `github.com/stretchr/testify/require` — test assertions, **test code only**
     (all packages). Never imported by production code.

   Keeping the runtime surface this small keeps the engine embeddable anywhere.
   (Historical note: the project started zero-dependency; the two entries above
   were adopted deliberately to follow house style — typed functional options
   and `require`-based tests.)
3. **Programmability over UI.** The API is the primary interface. Anything a
   user can do interactively should be expressible in code first.
4. **Correctness is observable.** Every capability ships with a test that
   asserts on solved coordinates / residuals, not just "it ran".

## Architecture at a glance

| File | Responsibility |
|---|---|
| `sketch.go` | `Sketch`, solver-bound geometry (`Point`/`Line`/`Circle`/`Arc`/`Ellipse`) authored from points, the parameter model, grounding, construction flag, `Geometry()` snapshots. |
| `compound.go` | Compound shape builders (`AddRectangle`/`AddPolygon`/`AddSlot`): primitives + shape-holding constraints, returned as a grouping handle (handle itself is not serialized). |
| `tools.go` | Sketch-modification tools on committed geometry (`Trim`/`Extend`/`Break`, `AddFillet`/`AddChamfer`, `AddMirror`, `AddPatternRect`/`AddPatternCircular`, `AddOffset`): build-then-replace via the `geom` toolkit + `RemoveEntity`. Design in `docs/modification-tools-design.md`. |
| `profiles.go` | `Sketch.Profiles()`: closed planar regions via the `geom` arrangement engine — bare-crossing subdivision, holes/nesting, net area, and per-region validity (self-intersecting/degenerate). `Profile` carries `Outer`/`Holes` (`BoundaryEdge`s, whole or fragment), `Area`, `Valid`, `SelfIntersecting`; construction excluded, reference geometry included. Internal `buildProfiles` also surfaces arrangement degeneracy to `Verify`. |
| `constraint.go` | `Constraint` interface and every constraint's residual + the public `New…` constructors. |
| `solver.go` | Levenberg–Marquardt solver, numerical Jacobian, DOF/redundancy (rank) analysis. |
| `diagnose.go` | Constraint diagnostics: `conflictAnalysis` (the shared dependency pass behind `RedundantConstraints`/`Diagnose`/`Verify`), `Diagnose` (redundant vs conflicting), `ConflictSet` (a conflicting constraint + the earlier ones it fights), `CheckConstraint` (pre-commit over-constraint rejection), `FreePoints`/`Point.IsFullyConstrained` (free-DOF attribution). Design in `docs/diagnostics-design.md`. |
| `verify.go` | `Sketch.Verify(...VerifyOption) *VerificationReport`: the headless-oracle aggregation layer — one non-mutating call gathering solvability, DOF, `Status`, redundant constraints, conflict sets, free points, profiles + their validity (`ProfilesValid`/`InvalidProfiles` — self-intersecting/degenerate regions gate `Trustworthy()`), stale/broken/foreign reference signals, parameter unit-kind validity (`ParametersValid`), the **advisory** `RankMargin` (how far the rank/DOF decision sits from the hard pivot threshold — a fragility hint; scale-dependent, so it does NOT gate `Trustworthy()`), `Trustworthy()`, and (opt-in via `WithProbe`) discrete ambiguity. A pure consumer of the diagnostic building blocks. |
| `reference.go` | Reference geometry — the sketch/3D separation keystone: read-only, externally-locked 2D snapshots of 3D-derived geometry (`AddReferencePoint`/`AddReferenceLine`/`AddReferenceArc`/`AddReferenceCircle`) carrying a `source` id + staleness; locked via `fixed[]`, a topology seal (`refSeals`), `RefreshReference`/`RefreshReferenceCircle`/`MarkStale`, and the Verify integrity/staleness/reachability scan. Design in `docs/reference-geometry-design.md`. |
| `probe.go` | `Sketch.ProbeConfigurations`: multi-solution ambiguity probe — deterministic multi-start search (structured mirrors + splitmix64 restarts) for the discrete configurations a DOF-0 sketch admits. A falsifier: ≥2 found proves ambiguity, 1 never proves uniqueness. Design in `docs/ambiguity-probe-design.md`. |
| `plane.go` / `world.go` | 3D world & construction planes. `Plane` (datum = `space.Frame` derived from a stored definition), package-level world-frame datum constructors, `World` (owns planes + sketches, plane builders incl. derived `OffsetPlane`, `RemovePlane`). Design in `docs/3d-planes-design.md`. |
| `svg.go` / `png.go` / `dxf.go` / `json.go` / `json_world.go` | Exporters / serialization. `png.go` is a stdlib-only rasterizer (`image/png`) so agents/tools that read raster images can sanity-check sketches; visually equivalent to the SVG output. `json_world.go` is the v2 `World`/`Plane` serialization + the `kind`-discriminator preflight. |
| `geom/` | **Self-contained** context-agnostic 2D geometry (own package). |
| `space/` | **Self-contained** 3D coordinate math (own package): `Vec3` + orthonormal `Frame` with the local↔world transform. |
| `param/` | **Self-contained** parameter & expression engine (own package). |
| `units/` | **Self-contained** units-of-measure library (own package). |
| `examples/` | Executable Go examples (`Example_sketch_…` in `package examples_test`, `go test`-verified `// Output:` blocks) that double as living documentation. Never `package main` programs. |

### The `geom` package (slated for extraction)

`geom/` holds **transient geometry** — plain `Point`/`Line`/`Circle`/`Arc`
definitions, *coordinates only*, no document state (no construction flag, no
name), no sketch/solver/constraints. It is the engine's `adsk.core` analog: a
pure math layer and the **snapshot type** that a sketch entity hands back from
its `Geometry()` accessor. It is **not** an input you hold and commit — sketch
geometry is authored directly from points (see "Building blocks vs sketch
geometry" below). It must not import `sketch`; the arrow is `sketch -> geom`,
never the reverse. Production code is standard-library-only (tests use
`testify/require`); intended to move to its own module later.

It also carries the **construction toolkit** (`intersect.go`, `modify.go`,
`transform.go`): line/circle/arc intersections (arc cases reduce to circle
cases filtered by `Arc.Contains`), `ClosestPointOnLine`, `SplitLineAt`/
`SplitArcAt`, `Fillet`/`Chamfer` (which replace a shared endpoint with fresh
contact points and return the connecting arc/line), and the `MirrorPoint`/
`TranslatePoint`/`RotatePoint` transforms. These compute on transient geometry;
the *mutating* sketch-level tools in `tools.go` (`Trim`/`Extend`/`Break`/
`AddFillet`/`AddChamfer`/`AddMirror`/`AddPatternRect`/`AddPatternCircular`/
`AddOffset`) feed them an entity's `Geometry()` snapshot, then build the
replacement from sketch points and retire the originals with `RemoveEntity`.

It also holds the **planar-arrangement / region engine** (`region.go`,
`arrange.go`, `area.go`): `geom.Regions(curves, closed)` builds a polyline-
approximated planar arrangement of lines/arcs/circles/ellipses/elliptical-arcs/
splines/closed-splines/fit-splines, splitting at bare crossings, and returns the
bounded
`Region`s (each an
outer boundary loop +
holes, with a net `Area` and source-curve `BoundaryEdge` back-references) plus
soundness signals — `SelfIntersections` (only for a single simple closed loop —
every shared vertex degree 2 — judged on the pruned core, so a branched/
subdivided wire is *not* flagged; a spline is the one source whose *own* polyline
is tested for self-crossings, since a cubic can loop) and `Degenerate`
(collinear-overlap or near-tangent uncertainty). Region area is exact for
line/arc/circle (shoelace + exact circular-segment correction), sampled for
ellipses and splines. `Sketch.Profiles()` is its consumer.

### The `space` package (slated for extraction)

`space/` is the 3D analog of `geom`: a self-contained coordinate-math layer with
no document state. It holds `Vec3` and the orthonormal right-handed `Frame`
(origin + unit axes `U`,`V`; normal `N()` = `U`×`V`, derived not stored). The
local↔world transform lives **only** here (`Frame.ToWorldUV`/`ToWorld`/`ToLocal`,
the inverse being the transpose — never a matrix solve). It imports nothing but
stdlib (not even `geom`); the arrow is `sketch -> space`, never the reverse.

- **Frames are ALWAYS orthonormal**, enforced at the boundary: `NewFrame`
  orthonormalizes and returns `ErrDegenerateFrame` on zero/collinear axes; the
  zero value `Frame{}` is invalid (`IsValid` is false) and every public consumer
  of a caller-supplied frame rejects it (`PlaneFromFrame`). Don't add a path that
  stores an unvalidated frame.
- `Vec3.Normalize` returns `(Vec3, bool)` — it never fabricates a unit vector
  from zero. This is **not** the solver's `norm()` floor; don't conflate them.

### The world & planes (`plane.go`/`world.go`)

The 2D solver is **untouched**: a `Sketch` still solves in plane-local 2D. A
`Plane` carries a `space.Frame` *computed from a stored definition* (its
provenance — the single source of truth; `Frame()` recomputes, no memoization).
A `World` owns planes (datums at ids 0/1/2) + sketches + **one shared
`param.Table`** (`World.Params()`) and is the multi-sketch serialization root; the
engine stays usable standalone (`sketch.NewOn(plane)` on an owner-less world-frame
datum). Load-bearing rules:

- **Global parameters are world-shared.** `World.Sketch(plane)` seeds the new
  sketch with `s.params = w.params`, so one global parameter drives dimensions
  across sketches. The per-sketch `Bind`/`ErrTableMismatch` invariant is untouched
  (a world sketch already points at the shared table; binding it to another fails
  naturally). Standalone `sketch.New()` keeps its own lazy table. **Offset planes
  are parameter-driven** (`World.BindOffsetPlane(p, expr)` → a length expression on
  `planeDef.distExpr`, kind-checked, re-evaluated on every `Frame()` call with NO
  cache so an edit reflows immediately; wrong-kind surfaces through `Frame()`).
  `World.Verify()` → `WorldVerificationReport` aggregates the shared table, every
  plane frame, and each sketch's report. World docs are **v3** (top-level
  `parameters` + plane `dist_expr`); a legacy v2 world migrates by promoting
  identical per-sketch tables, rejecting conflicting ones.

- **Placement is mandatory but nil-safe.** `Sketch.plane()` defaults a nil
  placement to `WorldXY()` (zero-value/unmarshal safety net) — but a v2
  `kind:"sketch"` document with no `plane` is **rejected** (`ErrMissingPlane`),
  not defaulted.
- **Standalone/owner-less planes are world-frame datums only** (XY/XZ/YZ,
  `PlaneFromFrame`, `PlaneFromPoints`). Derived planes (`OffsetPlane`) exist
  **only** through a `World` (a base reference needs an owner + id). `NewOn`
  returns an error (`ErrWorldOwnedPlane`/`ErrPlaneRemoved`) for a world-owned or
  removed plane.
- **`RemovePlane` mirrors `RemovePoint`**: refuses standard datums and in-use
  planes (a sketch on it, or another plane's base), else splices + renumbers ids
  densely and **tombstones** the handle (`removed=true`, `owner=nil`, `id=-1`).
  A tombstone is distinct from a live standalone plane and is rejected
  everywhere (`owns` checks `w.planes[p.id]==p`).
- World coordinates are a read-only derived surface (`Point.World`,
  `Sketch.WorldPolyline`), raw base-unit mm like the rest of the read surface.
  `WorldPolyline` samples via the centralized curve samplers in `geom/sample.go`
  (the exporters delegate to the same math; their output is unchanged).

### The `units` package (slated for extraction)

`units/` is a standalone units-of-measure library: typed [Unit] constants
(metric + imperial length, deg/rad angle — never strings), a [Value] type that
pairs a magnitude with its unit and converts between compatible units, and a
[System] holding the current default length/angle units (`Metric`/`SI`/
`Imperial`). Base units are millimetre and radian. Every unit has a [Kind]
(length/angle/dimensionless); conversion and `Value` arithmetic are
kind-checked and return [ErrIncompatible] on a mismatch — units are NEVER
silently relabelled. New units register via [Define]/[Lookup] (also the
serialization hook). **All unit conversion lives here** — no other package
re-implements factor math. It must not import `sketch` or `param`; the
dependency arrows are `sketch -> units` and `param -> units`, never the reverse.
Like `param`, it is intended to move to its own module later.

### The `param` package (slated for extraction)

`param/` is a standalone parameter/expression engine: a `Table` of named
parameters holding literals or expressions (`width = height * 1.5`), with a
lexer/parser/evaluator, functions, constants, forward references and cycle
detection. **It must not import anything from the `sketch` package or rely on
the rest of the repo** — it is intended to move into its own module/repository
later, so the dependency arrow only ever points *into* it. Keep its production
code standard-library-only (tests may use `testify/require`) and independently
testable.

### Building blocks vs. sketch geometry (load-bearing)

The model follows Fusion's transient-geometry / sketch-entity split.
**Transient geometry** (`geom.Point`/`Line`/…) is pure coordinate math: a
building block for the math layer and the **snapshot** an entity returns from
`Geometry()`. It carries no document state and is never committed. **Sketch
geometry** (`sketch.Point`/`Line`/…) is the durable, solver-bound entity, and
the only handle you hold. You author it directly: `s.AddPoint(x, y)` returns a
`*Point`; the curve builders `s.AddLine(p1, p2)`/`AddCircle(center, r)`/
`AddArc(c, s, e)`/`AddEllipse(center, rx, ry, rot)`/`AddSpline(pts…)` take those
points. **Topology is expressed by sharing a `*Point`** between entities (a
shared corner is literally one point) — there is no generic-pointer identity
map and no idempotency; each `Add…` makes a fresh entity. Constraints reference
**sketch** geometry, so they never reference un-committed geometry —
`AddConstraint` just registers them. To read an entity's current shape as a
transient value, call `Geometry()` (a fresh snapshot at the solved coords);
`geom.NewX` is for math and snapshots, never as sketch input.

### The parameter model (load-bearing)

All scalar unknowns — point `x`/`y`, circle radius, ellipse semi-axes/rotation
— live in one flat vector on the `Sketch` (`vars []float64`, with a parallel
`fixed []bool`). Sketch primitives hold **indices** into that vector (no
geom back-reference). The solver reads/perturbs the vector directly. Grounding
(`fixed`) is per-point on the sketch (`s.Fix`/`Unfix`); construction status is a
settable per-entity property (`entity.SetConstruction`). Any new geometry that
introduces unknowns must allocate them via `newVar` in its `Add…` method and
reference them by index so the solver sees them automatically.

A **constraint** may also own auxiliary variables when it genuinely needs them
(the arc-tangency sweep slack, and the arc-length dimension's unwrapped-sweep
variable). It allocates them in an
`allocVars(*Sketch)` method — a hook `AddConstraint` calls (the same shape as
`resolveUnit`), so it runs on initial commit and on load (rebuild goes through
`AddConstraint`). Aux vars are retired on removal via a `retireVars(*Sketch)`
hook and are **not serialized** — they are recomputed from the solved geometry
when `allocVars` re-runs on load. This is the deliberate, narrow exception to
"constraints own no vars"; ship it only with the constraint that needs it.

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
  configuration.** `rank()`/`DOF()` rebuild J via `jacobian` when called — after
  `Solve` that is the *solved* point. NEVER reuse the Solve loop's
  last-iteration Jacobian for rank analysis: it is stale (evaluated one step
  before convergence) and yields wrong DOF/redundancy counts.
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

- Points and entities are referenced by their **id, which always equals their
  current slice position** (`s.points`/`s.ents` — two independent id spaces).
  Removal splices and renumbers the later ids (`removal.go`), so marshalled
  documents stay dense and coherent; `UnmarshalJSON` recreates in order so the
  indices line up. Never let an `id` field and slice position diverge.
- A **sketch** document carries `"version": 2` (`jsonVersion`); a **world**
  document carries `"version": 3` (`jsonWorldVersion`, ahead because a world adds
  top-level shared `parameters` + plane `dist_expr` an older reader would silently
  drop) plus an explicit `"kind"` (`"sketch"` | `"world"`). Both loaders
  **preflight** the raw top-level object (today's typed unmarshal ignores unknown
  fields, so a world doc fed to `Sketch.UnmarshalJSON` would otherwise rebuild
  empty) and **check kind before version** (a world doc handed to the sketch loader
  is a wrong-kind error, not a wrong-version one): a v2 doc requires
  `kind`; a wrong/unknown `kind` is `ErrWrongDocumentKind`; a legacy (kind-less,
  version absent/0/1) doc must carry no v2-only key (`plane`/`planes`/`sketches`)
  and loads as a world-XY sketch. A v3 world carries the shared param table at the
  top level (world sketches no longer serialize their own); a legacy v2 world
  migrates per-sketch tables (identical → promote, conflicting → reject). Both shapes decode their payload through one
  shared `jsonSketchBody` (`buildFromBody`) so reference handling lives in one
  place. A plane serializes its **definition** (recomputed on load, never trusted
  from disk); a world's derived `offset{base_id}` must reference an **earlier**
  plane. Newer versions are rejected. Bump `jsonVersion` + add read-side
  migration for schema changes.
- **Internal constraints** (those implementing `internalConstraint`, e.g. the
  arc radius-consistency constraint auto-added by `AddArc`) are *not* serialized
  — they're recreated by the constructor on load. New auto-added constraints
  must follow this pattern or round-trips will double them.
- **The `param` table serializes in definition order.** Its JSON preserves the
  order parameters were defined so forward references and reload stay
  reproducible. Keep that order on marshal/unmarshal.

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
- **Tests use `testify/require`** (never `assert`) and live in **external
  `xxx_test` packages** — they exercise only the exported API. If a test needs
  to observe internal state, add a documented exported accessor rather than
  reaching into unexported fields (e.g. `Sketch.Points`, `Point.ID`,
  `Point.Geometry`, `DriverExpr`). No named return values, including in tests.
  Author geometry with the real builders (`s.AddPoint(x,y)`, `s.AddLine(a,b)`,
  …) directly in tests — do not wrap them in trivial 1:1 helpers; explicit is
  better.
- Geometry is authored against the sketch from points (`s.AddPoint` then
  `s.AddLine`/`AddCircle`/`AddArc`/`AddEllipse`/`AddSpline`); constraints come
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
  The shape/pattern builders (`AddPolygon`/`AddSlot`/`AddSpline`/
  `AddPatternRect`/`AddPatternCircular` → `ErrInvalidShape`), `NewOn`
  (`ErrWorldOwnedPlane`/`ErrPlaneRemoved`), and the plane/frame constructors
  (`space.ErrDegenerateFrame`, `geom.ErrTooFewControlPoints`) all return
  `(…, error)`. Only pure math kernels whose precondition is guaranteed by their
  constructor (`geom.EvalCubicBSpline`/`SampleCubicBSpline`) may still panic —
  like an out-of-range index, not input validation.
- Keep exported API documented with Go doc comments; primitives expose value
  accessors (`X()`, `Y()`, `R()`, …), a `Geometry()` snapshot, and measurement
  queries (`Point.DistanceTo`/`DistanceToLine`, `Line.AngleTo`), while
  index-backed fields stay unexported. Measurement math lives in `geom`
  (`geom/measure.go`); the sketch entities delegate through `Geometry()`.
- New constraints: add the residual, the `New…` constructor, a case in the JSON
  marshal/unmarshal switches, an arg-count entry in `constraintArity`
  (`json.go` — so the decoder validates references before indexing), a case in
  `constraintRefs` (`removal.go` — or the removal cascade silently misses it),
  and a test asserting on the solved geometry.

## Open design questions (the "many variables")

These are unsettled. If you resolve one, record the decision here.

- **Parameters & expressions.** *Resolved.* The `param` engine is wired into
  the sketcher: the caller supplies a `param.Table` explicitly at bind time via
  `s.Bind(dim, table, expr)` (the table is required, and all of a sketch's
  dimensions must share one table — `ErrTableMismatch` otherwise). `s.Params()`
  returns whatever table the bindings established (nil if none). Bound
  dimensions are re-evaluated by `ApplyParameters` at the start of every
  `Solve`; a manual `.Set(v)` clears the binding. Parameters and per-dimension
  expressions are serialized in the sketch JSON. The dependency arrow is
  `sketch -> param`, never the reverse. *Possible follow-ups:* parameter units,
  and reporting which parameter a solve failure came from.
- **Geometry coverage.** *Largely resolved.* Splines are in as clamped
  uniform cubic B-splines whose control points are ordinary sketch points (no
  new solver machinery; see `docs/spline-design.md`). A point can be confined to
  a spline with `NewPointOnSpline`: the foot-point parameter `t` is an auxiliary
  solver variable (no implicit `F(x,y)=0` exists for a B-spline, so membership is
  the existential `P = S(t)` — two length rows), bounded to `[0,1]` by a
  slack-encoded box (`t=w0²`, `1−t=w1²`) so out-of-range `t` is infeasible rather
  than silently clamped. The aux vars are not serialized (re-seeded on load by
  foot-point projection). `CheckConstraint` probes any aux-var constraint in its
  committed form — it temporarily allocates the candidate's aux vars, ranks the
  real rows, then rolls back (non-mutating). (A documented limitation: two
  point-on-spline on the same point are redundant only nonlinearly, so the local
  rank analysis is not guaranteed to flag the duplicate; it stays harmless.)
  A line can be made tangent to a spline with `NewTangentToSpline` (same bounded
  contact-parameter `t` machinery): the committed residual is five rows — contact
  `S(t)` on the line's infinite carrier (signed perpendicular distance, length),
  the line direction parallel to the analytic spline tangent `S'(t)`
  (`geom.EvalCubicBSplineDeriv`, dimensionless `sin`), the two box rows, and a
  scale-relative no-cusp guard `|S'(t)|/scale ≥ epsTan` (slack `ws`) so the oracle
  never blesses "tangent" where the tangent direction is undefined; a zero-length
  line is rejected outright. `S'(t)` is analytic on purpose — a numerical tangent
  inside the residual would be a nested finite difference the Jacobian
  re-differentiates.
  **Closed (periodic) splines** are in as a separate `ClosedSpline` entity
  (`AddClosedSpline`, ≥3 control points) over an exact cyclic uniform cubic basis
  (`geom.EvalPeriodicCubicBSpline`) — a smooth C2 loop that bounds a region on its
  own (a sealed `geom.ClosedCurve`, not a `Curve`), with periodic-ring
  self-crossing detection and `closed_spline` serialization. Point-on/tangent
  constraints on a closed spline are a deferred follow-up (periodic witness).
  **Fit-point (interpolating) splines** are in as a separate `FitSpline` entity
  (`AddFitSpline`, ≥2 fit points) whose curve passes *through* the fit points: the
  fit points are the durable solver handles and a natural-cubic interpolant
  (chord-length parameterization, Thomas tridiagonal solve in
  `geom.EvalFitSpline`/`SampleFitSpline`) is recomputed from their current
  coordinates per evaluation, so the curve keeps interpolating them as the solver
  moves them — no new solver vars. An open `Curve` (endpoints = first/last fit
  point) participating in profiles like the open spline, `fit_spline` serialization.
  Point-on/tangent constraints on it are a deferred follow-up.
  Ellipses are in
  (center point + rx/ry/rotation vars; `NewPointOnEllipse` uses a
  Sampson-normalized residual — |F|/|∇F| — to stay in length units).
  **Elliptical arcs** are in as a geometry primitive (`AddEllipticalArc`:
  center + start/end points + rx/ry/rotation vars, two internal on-ellipse
  constraints pinning the endpoints, eccentric-angle sweep, sampled-bulge area
  in the arrangement). Its shape is dimensionable via the sealed `Elliptical`
  interface (`NewSemiMajor`/`NewSemiMinor`/`NewEllipseRotation` accept a
  `*Ellipse` or an `*EllipticalArc`). A point can be confined to an elliptical
  arc with `NewPointOnEllipticalArc` (on the ellipse via the Sampson residual,
  within the eccentric sweep via a slack inequality, mirroring `pointOnArc`). A
  line can be made tangent to an ellipse or elliptical arc with
  `NewTangentEllipse` (sealed `Elliptical` operand, mirroring `NewTangent` for
  circles): the closed-form condition `√((u·rx)²+(v·ry)²)=|c|` on the line's
  local-frame normal — no foot-point iteration — plus, for an arc, the same
  slack inequality confining the contact to the eccentric sweep (and an
  endpoint-tangency branch when the line shares a boundary point).
  **Conic–conic tangency** (no closed-form distance; design in
  `docs/conic-tangency-design.md`): `NewTangentEllipseCircular(e Elliptical, c
  Circular, …)` and `NewTangentEllipses(e1, e2 Elliptical, …)` over the sealed
  interfaces, so each operand is a circle, **arc**, ellipse, or **elliptical
  arc**. A contact-point witness (aux coords) on both curves with parallel
  outward normals (`cross(n̂_A,n̂_B)=0`), a **hard** internal/external branch row
  `σ·dot(n̂_A,n̂_B)−wSide²` (the flag must be an enforced equation, not a seed, or
  the oracle could not tell the branches apart), degenerate-conic guards, and —
  per arc operand — a slack-encoded **sweep row** confining the contact to the
  swept portion (so a tangent to the underlying full conic off the arc is
  rejected). When two **arc** operands share an exact endpoint `*Point` the
  **shared-endpoint branch** enforces tangency *at* that point — `parallel` +
  internal/external branch rows there, no free witness and no membership/sweep
  rows (an endpoint is already on both curves and in-sweep by definition).
  Slots/fillet/chamfer exist as compound builders and `geom` template helpers.
- **Solver evolution.** Numerical Jacobian is fine at current scale. An
  **advisory** rank-margin diagnostic is in (`solver.go` `rankAnalysisOf`): the
  rank/DOF verdict turns on a hard `rankEps = 1e-9` pivot threshold, so `Verify()`
  reports `RankMargin` — how close the deciding pivots sit to it — as a fragility
  hint. It does **NOT** gate `Trustworthy()`: the raw pivots are scale-dependent
  (angle-constraint derivatives grow with line length), so it is not a
  unit-invariant condition number and the same construction at different scales
  gives different margins. Open: a **scale-invariant conditioning gate** via
  principled per-var-kind (length/angle/slack) column scaling — the right way to
  actually gate trust on near-singularity, deliberately deferred because raw
  un-normalized gating proved unsound; analytic Jacobians for speed/accuracy;
  equation decomposition (solve independent constraint clusters separately); and
  better diagnostics for over-constrained sketches (identify *which* constraints
  conflict, not just a count).
- **Constraint diagnostics & UX.** *Largely resolved* (`diagnose.go`; design
  in `docs/diagnostics-design.md`). `Sketch.RedundantConstraints()` identifies
  dependent constraints (creation order decides: of two duplicates the later
  one is reported; the row→constraint mapping mirrors `residuals()`).
  `Sketch.Diagnose()` partitions them into redundant (dependent, satisfied)
  vs conflicting (dependent, violated — residual > 1e-8 at the call-time
  configuration). `Sketch.CheckConstraint(c)` rank-probes a candidate without
  committing it and returns `ErrOverconstrained` if any of its equations is
  dependent — the engine half of Fusion's "refuse the over-constraining
  gesture". `Sketch.FreePoints()`/`Point.IsFullyConstrained()` attribute the
  remaining DOF to points via the Jacobian null space (the blue/black
  coloring answer). `Sketch.ProbeConfigurations()` (`probe.go`; design in
  `docs/ambiguity-probe-design.md`) covers the discrete side DOF analysis
  cannot see: a deterministic multi-start probe that searches for the multiple
  configurations a fully-constrained sketch may admit (mirror flips, tangent
  side choices). It is a falsifier — finding ≥2 configurations proves
  ambiguity; finding 1 never certifies uniqueness. `Sketch.Verify()`
  (`verify.go`) aggregates all of the above into one non-mutating
  `VerificationReport` (solvability, DOF, `Status`, redundant constraints,
  conflict sets, free points, profiles, opt-in ambiguity via `WithProbe`) for
  the headless-oracle use case. Conflict sets are reported via `ConflictSet`
  (`diagnose.go`): for each conflicting constraint, the earlier *independent*
  constraints whose Jacobian rows linearly combine to reproduce the violated
  row — a true set, not just the later duplicate. `RedundantConstraints`,
  `Diagnose` and `Verify` share one `conflictAnalysis` pass so the partition and
  attribution never diverge. Still open: per-entity (not just per-point)
  constrained status, an `AddConstraint` option that auto-rejects, probe-level
  tolerance/budget options, and folding the ellipse rx/ry-swap symmetry into the
  probe's duplicate metric.
- **Higher-level interfaces.** A text DSL + CLI, and eventually an interactive
  GUI (e.g. Ebiten), are anticipated layers. They should consume the public API
  only.
- **Units.** *Resolved (units).* The `units` package provides typed units, a
  unit-carrying `Value`, and a default-units `System`. Sketch dimensions and
  `param` parameters both carry units; the solver stays in base units and all
  conversion is delegated to the library. **Expression kind algebra is in**
  (`param/kind.go`): `param` tracks unit *kind* (length/angle/dimensionless)
  through expression arithmetic via a static `kindOf` walk — an identifier's kind
  is its declared unit's kind — and rejects incompatible combinations
  (`length+angle`, `length*length` since there is no area unit, `1/length`
  inverse, `sqrt`/trig of a dimensioned value, …) with `param.ErrIncompatibleKind`.
  Addition allows angle/dimensionless mixing (radians are physically
  dimensionless, so `theta + pi/2` is an angle; a length never mixes with a bare
  number), and a parameter's declared unit kind is checked against its
  expression's kind (an angle expression cannot masquerade as a length parameter).
  `Table.EvalValue` returns the kind-carrying value; `Sketch.evalDimension` uses it
  to reject a compound expression that mixes kinds or whose kind ≠ the dimension's
  (not just a direct single-parameter reference); `Verify()` runs a non-mutating
  parameter-validation pass exposing `ParametersValid`/`ParameterErrors`, which
  gate `Trustworthy()` — so a unit-kind bug hidden in an expression is no longer
  silently blessed. *Limited on purpose:* this is **kind** algebra, not full
  **dimensional** algebra — there are no area/inverse units, so those products are
  *rejected* rather than represented; custom `SetFunc` functions are
  dimensionless-only (typed custom functions are a follow-up). *Open follow-ups:*
  should points/coordinates expose unit-carrying accessors; should exporters
  honour the display `System`. *Note:* the entire read surface
  — coordinate accessors and the measurement queries (`DistanceTo`/`AngleTo`/…)
  — currently returns raw base-unit `float64` (mm/radians), matching the
  solver's currency. Making reads unit-carrying is the deferred all-or-nothing
  decision above; it should be done across the whole surface, not piecemeal.
- **Entity/constraint removal.** *Resolved.*
  `RemoveConstraint`/`RemoveEntity`/`RemovePoint` (`removal.go`; design in
  `docs/removal-design.md`): splice + id renumbering, entity-owned vars
  retired (marked fixed, never reclaimed — reload compacts), constraint
  cascade via the `constraintRefs` switch (includes internal `arcRadius`),
  points kept on entity removal, `RemovePoint` refuses while an entity uses
  the point. Removed handles are dead. This unblocked the mutating sketch
  tools (trim/extend/break/fillet/chamfer/mirror/pattern/offset of committed
  geometry), now built in `tools.go` (design in
  `docs/modification-tools-design.md`).
- **Tolerances.** Still a fixed solver tolerance. Per-sketch
  tolerance/precision remains open.
- **Persistence stability.** *Partially resolved:* documents carry
  `"version": 1`; legacy (unversioned) documents load, newer-versioned ones
  are rejected. Still open: an actual migration story when version 2 arrives,
  and schema compatibility guarantees.
- **2D → 3D.** *Partially resolved* (`plane.go`/`world.go`/`space/`; design in
  `docs/3d-planes-design.md`). 2D sketches now live on construction planes inside
  a 3D `World`, with a bidirectional local↔world transform (`Point.World`,
  `Sketch.WorldPolyline`). The 2D solver is unchanged — 3D is a placement layer.
  The **sketch/3D separation keystone is in place**: reference geometry
  (`reference.go`, design in `docs/reference-geometry-design.md`) holds frozen
  snapshots of 3D-derived geometry (projected edges, pierced vertices) — locked,
  with a source id + staleness — so this layer verifies *against* given 3D
  geometry and never *computes* it. Still **out of scope** (above this layer):
  surfaces (NURBS/analytic), free 3D-sketch geometry (points with a `z` var),
  cross-sketch/cross-plane constraints (the `planeDef` recompute is the seam),
  3D rendering, and the projection/intersection algorithms that *produce* the
  reference snapshots. Profiles feeding extrude/revolve remain a future consumer
  of `Sketch.WorldPolyline`.

## Status

Core engine + constraint set + solver (with DOF/redundancy analysis) +
SVG/DXF/JSON export + sketch-modification tools (`tools.go`:
trim/extend/break/fillet/chamfer/mirror/pattern/offset on committed geometry) +
3D world & construction planes (`space/`, `plane.go`, `world.go`: 2D sketches
placed on planes in a 3D world, local↔world transform, v2 serialization) +
unified verification (`verify.go`: `Sketch.Verify` aggregating solvability,
DOF/status, conflict sets, free points, profiles + profile validity, opt-in
ambiguity) +
reference geometry (`reference.go`: locked, externally-sourced 2D snapshots with
provenance + staleness — the sketch/3D separation keystone) +
the profile/region engine (`geom/arrange.go` + `profiles.go`: planar
arrangement of sketch geometry into closed regions with bare-crossing
subdivision, holes/nesting, net area, and self-intersection/degeneracy validity
gating `Trustworthy()`) are implemented and tested.
