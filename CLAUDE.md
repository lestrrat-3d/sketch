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
2. **Zero external dependencies.** Standard library only. This is a deliberate
   constraint — do not add modules to `go.mod` without an explicit decision
   recorded here. It keeps the engine embeddable anywhere.
3. **Programmability over UI.** The API is the primary interface. Anything a
   user can do interactively should be expressible in code first.
4. **Correctness is observable.** Every capability ships with a test that
   asserts on solved coordinates / residuals, not just "it ran".

## Architecture at a glance

| File | Responsibility |
|---|---|
| `sketch.go` | `Sketch`, primitives (`Point`/`Line`/`Circle`/`Arc`), the parameter model, grounding. |
| `constraint.go` | `Constraint` interface and every constraint's residual + the public constructor methods. |
| `solver.go` | Levenberg–Marquardt solver, numerical Jacobian, DOF/redundancy (rank) analysis. |
| `svg.go` / `dxf.go` / `json.go` | Exporters / serialization. |
| `param/` | **Self-contained** parameter & expression engine (own package). |
| `units/` | **Self-contained** units-of-measure library (own package). |
| `examples/` | Runnable programs that double as living documentation. |

### The `units` package (slated for extraction)

`units/` is a standalone units-of-measure library: typed [Unit] constants
(metric + imperial length, deg/rad angle — never strings), a [Value] type that
pairs a magnitude with its unit and converts between compatible units, and a
[System] holding the current default length/angle units. Base units are
millimetre and radian. **All unit conversion lives here** — no other package
re-implements factor math. It must not import `sketch` or `param`; the
dependency arrows are `sketch -> units` and `param -> units`, never the reverse.
Like `param`, it is intended to move to its own module later.

### The `param` package (slated for extraction)

`param/` is a standalone parameter/expression engine: a `Table` of named
parameters holding literals or expressions (`width = height * 1.5`), with a
lexer/parser/evaluator, functions, constants, forward references and cycle
detection. **It must not import anything from the `sketch` package or rely on
the rest of the repo** — it is intended to move into its own module/repository
later, so the dependency arrow only ever points *into* it. Keep it standard-
library-only and independently testable.

### Construction vs. committing (load-bearing)

Geometry and constraints are **constructed detached** (`NewPoint`, `NewLine`,
`NewCircle`, `NewArc`, and `NewDistance`/`NewHorizontal`/… for constraints) and
then **committed** to a sketch as a separate step (`AddPoint`, `AddLine`,
`AddCircle`, `AddArc`, `AddConstraint`). The two operations are deliberately
distinct. Adders are **idempotent** and **cascade**: `AddLine` commits the
line's points first, `AddConstraint` commits any geometry the constraint
references. A detached object holds its own values (`Point.x0/y0`,
`Circle.r0`) and is fully usable (`X()`, `Length()`, `R()`) before being added;
once added it is index-backed (see below). New geometry/constraints must keep
this split — constructors allocate nothing on a sketch; `Add…` does the
committing — and `addConstraintGeometry` (in `parameters.go`) must learn the
new constraint's geometry references so the cascade stays complete.

### The parameter model (load-bearing)

All scalar unknowns — point `x`/`y`, circle radius — live in one flat vector on
the `Sketch` (`vars []float64`, with a parallel `fixed []bool`). Primitives hold
**indices** into that vector once committed (before that they hold their own
values). The solver reads/perturbs the vector directly. Any new geometry that
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
- **The solver works in base units** (millimetre coordinates, radian angles).
  Dimensions carry a `units.Value`; their residual uses `Target().Base()` to
  reach base units. Unit conversion happens *only* in the `units` library
  (`Value.Base`/`In`/`Convert`/`FromBase`) — never by relabelling a magnitude
  (turning "1 deg" into "1 rad" is a bug). Bare-float constructors interpret
  their number in the sketch's default unit for that kind (`Sketch.Units`).

### Serialization invariants

- Points and entities are referenced by their **creation index** (`id`). JSON
  round-trips rely on `UnmarshalJSON` recreating them in the same order so those
  indices line up. Preserve creation order when reconstructing.
- **Internal constraints** (those implementing `internalConstraint`, e.g. the
  arc radius-consistency constraint auto-added by `AddArc`) are *not* serialized
  — they're recreated by the constructor on load. New auto-added constraints
  must follow this pattern or round-trips will double them.

## Conventions

- `gofmt`, `go vet`, and `go test ./...` must all be clean before committing.
- Constructors are package-level `New…` functions (the `New` prefix is forced
  for the dimensional ones because their concrete handle types — `Distance`,
  `Radius`, `Angle`, … — already own the bare name; keep all constructors
  consistent). `New…` constructs detached; `s.Add…`/`s.AddConstraint` commits.
- Public dimensional constructors return concrete handles (`*Distance`, etc.)
  with `.Set`/`.SetValue`; geometric constructors return the `Constraint`
  interface.
- Keep exported API documented with Go doc comments; primitives expose value
  accessors (`X()`, `Y()`, `R()`) while the index-backed fields stay unexported.
- New constraints: add the residual, the `New…` constructor, a case in
  `addConstraintGeometry` so its geometry cascades, a case in the JSON
  marshal/unmarshal switches, and a test asserting on the solved geometry.

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
- **Geometry coverage.** Splines/B-splines, ellipses, slots, fillet/chamfer and
  offset helpers are not yet present. Splines in particular interact with the
  solver (control points as unknowns).
- **Solver evolution.** Numerical Jacobian is fine at current scale. Open:
  analytic Jacobians for speed/accuracy, equation decomposition (solve
  independent constraint clusters separately), and better diagnostics for
  over-constrained sketches (identify *which* constraints conflict, not just a
  count).
- **Constraint diagnostics & UX.** Today we report DOF and a redundancy count.
  A real sketcher wants to point at the specific redundant/conflicting
  constraint and at the remaining free DOF.
- **Higher-level interfaces.** A text DSL + CLI, and eventually an interactive
  GUI (e.g. Ebiten), are anticipated layers. They should consume the public API
  only.
- **Units.** *Resolved (units).* The `units` package provides typed units, a
  unit-carrying `Value`, and a default-units `System`. Sketch dimensions and
  `param` parameters both carry units; the solver stays in base units and all
  conversion is delegated to the library. *Limited on purpose:* there is no
  full dimensional algebra through expressions — `param` evaluates magnitudes in
  base units and a parameter's declared unit tags the result; kind mismatches
  are caught at the sketch-binding boundary, not inside every expression. *Open
  follow-ups:* should expressions track kind through arithmetic (catch mm+deg
  mid-expression); should points/coordinates expose unit-carrying accessors;
  should exporters honour the display `System`.
- **Tolerances.** Still a fixed solver tolerance. Per-sketch
  tolerance/precision remains open.
- **Persistence stability.** The JSON schema is not yet versioned. Before anyone
  depends on it, decide on a version field and compatibility policy.
- **2D → 3D.** Out of scope for now, but the API shouldn't paint us into a
  corner if profiles later feed extrude/revolve operations.

## Status

Core engine + constraint set + solver (with DOF/redundancy analysis) +
SVG/DXF/JSON export are implemented and tested. Active branch:
`claude/2d-sketch-tool-go-c73sfs`.
