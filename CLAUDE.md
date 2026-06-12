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
| `profiles.go` | `Sketch.Profiles()`: closed-region boundaries (loops of lines/arcs via `geom.Loops` + standalone circles/ellipses), construction geometry excluded. |
| `constraint.go` | `Constraint` interface and every constraint's residual + the public `New…` constructors. |
| `solver.go` | Levenberg–Marquardt solver, numerical Jacobian, DOF/redundancy (rank) analysis. |
| `diagnose.go` | Constraint diagnostics: `Diagnose` (redundant vs conflicting), `CheckConstraint` (pre-commit over-constraint rejection), `FreePoints`/`Point.IsFullyConstrained` (free-DOF attribution). Design in `docs/diagnostics-design.md`. |
| `probe.go` | `Sketch.ProbeConfigurations`: multi-solution ambiguity probe — deterministic multi-start search (structured mirrors + splitmix64 restarts) for the discrete configurations a DOF-0 sketch admits. A falsifier: ≥2 found proves ambiguity, 1 never proves uniqueness. Design in `docs/ambiguity-probe-design.md`. |
| `svg.go` / `dxf.go` / `json.go` | Exporters / serialization. |
| `geom/` | **Self-contained** context-agnostic geometry (own package). |
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
- The document carries `"version": 1` (`jsonVersion`). Absent (legacy) and
  current versions load; newer versions are rejected with an error rather
  than mis-loaded. Bump `jsonVersion` and add read-side migration for schema
  changes.
- **Internal constraints** (those implementing `internalConstraint`, e.g. the
  arc radius-consistency constraint auto-added by `AddArc`) are *not* serialized
  — they're recreated by the constructor on load. New auto-added constraints
  must follow this pattern or round-trips will double them.
- **The `param` table serializes in definition order.** Its JSON preserves the
  order parameters were defined so forward references and reload stay
  reproducible. Keep that order on marshal/unmarshal.

## Conventions

- `gofmt`, `go vet`, and `go test ./...` must all be clean before committing.
- **Optional settings use functional options**, not options structs. Each option
  group defines a typed marker interface (`SVGOption`, `SolveOption`) embedding
  `option.Interface` plus a private wrapper, `ident…` marker structs, and `With…`
  constructors; the consumer folds them into a private `…Config` struct seeded
  from a `default…Config()`. See `svg.go` / `solver.go`. The typed interface
  keeps each option group distinct (an `SVGOption` can't be passed to `Solve`).
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
  residuals need no radius variable.
- Public dimensional constructors return concrete handles (`*Distance`, etc.)
  with `.Set`/`.SetValue`; geometric constructors return the `Constraint`
  interface.
- Keep exported API documented with Go doc comments; primitives expose value
  accessors (`X()`, `Y()`, `R()`, …), a `Geometry()` snapshot, and measurement
  queries (`Point.DistanceTo`/`DistanceToLine`, `Line.AngleTo`), while
  index-backed fields stay unexported. Measurement math lives in `geom`
  (`geom/measure.go`); the sketch entities delegate through `Geometry()`.
- New constraints: add the residual, the `New…` constructor, a case in the JSON
  marshal/unmarshal switches, a case in `constraintRefs` (`removal.go` — or
  the removal cascade silently misses it), and a test asserting on the solved
  geometry.

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
  new solver machinery; see `docs/spline-design.md` — point-on-spline/tangency
  is the recorded v2 via an aux-parameter `allocVars` hook). Ellipses are in
  (center point + rx/ry/rotation vars; `NewPointOnEllipse` uses a
  Sampson-normalized residual — |F|/|∇F| — to stay in length units;
  tangency-to-ellipse and elliptical arcs are still open).
  Slots/fillet/chamfer exist as compound builders and `geom` template helpers.
- **Solver evolution.** Numerical Jacobian is fine at current scale. Open:
  analytic Jacobians for speed/accuracy, equation decomposition (solve
  independent constraint clusters separately), and better diagnostics for
  over-constrained sketches (identify *which* constraints conflict, not just a
  count).
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
  ambiguity; finding 1 never certifies uniqueness. Still open: reporting the
  full conflict *set* (the earlier constraints a conflicting one fights),
  per-entity constrained status, an `AddConstraint` option that auto-rejects,
  probe-level tolerance/budget options, and folding the ellipse rx/ry-swap
  symmetry into the probe's duplicate metric.
- **Higher-level interfaces.** A text DSL + CLI, and eventually an interactive
  GUI (e.g. Ebiten), are anticipated layers. They should consume the public API
  only.
- **Units.** *Resolved (units).* The `units` package provides typed units, a
  unit-carrying `Value`, and a default-units `System`. Sketch dimensions and
  `param` parameters both carry units; the solver stays in base units and all
  conversion is delegated to the library. *Limited on purpose:* there is no
  full dimensional algebra through expressions — `param` evaluates magnitudes in
  base units and a parameter's declared unit tags the result; kind mismatches
  are caught at the sketch-binding boundary, not inside every expression. Only a
  *direct* parameter reference (`s.Bind(dim, t, "width")`) carries the
  parameter's unit and is kind-checked against the dimension; a compound
  expression (`"width * 2"`) is evaluated to a base-unit magnitude and tagged
  with the dimension's base unit, so a kind error hidden inside an expression is
  not caught. *Open
  follow-ups:* should expressions track kind through arithmetic (catch mm+deg
  mid-expression); should points/coordinates expose unit-carrying accessors;
  should exporters honour the display `System`. *Note:* the entire read surface
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
- **2D → 3D.** Out of scope for now, but the API shouldn't paint us into a
  corner if profiles later feed extrude/revolve operations.

## Status

Core engine + constraint set + solver (with DOF/redundancy analysis) +
SVG/DXF/JSON export + sketch-modification tools (`tools.go`:
trim/extend/break/fillet/chamfer/mirror/pattern/offset on committed geometry)
are implemented and tested. Active branch:
`claude/2d-sketch-tool-go-c73sfs`.
