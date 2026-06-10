# Fusion-Compatible Sketcher: Feature Gap Analysis

Gap analysis of this engine against the Autodesk Fusion sketch environment.
Snapshot date: 2026-06-10. Baseline at time of writing: 4 primitives
(point/line/circle/arc), 15 geometric + 6 dimensional constraints, LM solver
with DOF/redundancy counts, `param` table, `units`, SVG/DXF/JSON export.

## Geometry primitives

**Have:** point, line, circle, arc.

**Missing**, roughly in order of how often Fusion users reach for them:

- **Ellipse / elliptical arc** — straightforward unknowns (center, two radii,
  rotation), but adds several new tangent/point-on residual cases.
- **Splines** — Fusion has fit-point splines and control-point splines. The big
  one architecturally: control points become solver unknowns, and
  point-on-spline / tangent-to-spline residuals need curve evaluation inside
  the residual function. Already flagged in CLAUDE.md open questions.
- ~~**Slot** (straight)~~ — *closed 2026-06*: `AddSlot` (two arcs + two flanks;
  equal cap radii + perpendicular construction spokes at the contact points —
  perpendicularity implies tangency *and* pins the contact point, which a plain
  tangent constraint does not). Arc slot still open.
- ~~**Rectangle / polygon constructors**~~ — *closed 2026-06*: `AddRectangle`
  (H/V constraints) and `AddPolygon` (equal sides + equal construction spokes).
- **Construction geometry flag** — already existed (`.Construction` on any
  entity; rendered dashed, separate DXF layer).

## Constraints

The geometric set is already close to Fusion's. Remaining gaps:

- ~~**Tangent: line–arc, arc–arc, arc–circle**~~ — *closed 2026-06*: the
  `Circular` interface (`*Circle` | `*Arc`) generalized `NewTangent`,
  `NewTangentCircles` and `NewEqualRadius` to accept arcs. Tangency treats an
  arc as its full circle (sweep not enforced).
- **Point-on-arc / point-on-curve** generalization (eventually
  point-on-spline). The `Circular` interface is the natural vehicle for
  `pointOnCircle`/`concentric`/`Radius`/`Diameter` too.
- **Symmetric for lines/circles/arcs** — point symmetry exists; Fusion symmetry
  applies to whole entities.
- **Equal for line↔arc mixed** — Fusion's "equal" works across lines and
  arcs/circles.
- **Fix/ground as a constraint-like toggle on any entity** — per-point
  grounding exists; this is mostly API surface.
- **Coincident point-to-entity** — Fusion's coincident subsumes
  point-on-line/point-on-curve under one name; the pieces exist, this is
  naming/UX for the future DSL.

### Dimensional gaps

- **Arc length dimension.**
- ~~**Distance point↔line**~~ — *closed 2026-06*: `NewDistancePointLine`.
- ~~**Distance line↔line**~~ — *closed 2026-06*: `NewDistanceLines` (two
  residuals; forces parallelism, no separate parallel constraint needed).
- **Distance to circle/arc tangent** (Fusion's dimension-to-tangent option).
- ~~**Driven (reference) dimensions**~~ — *closed 2026-06*:
  `Dimension.SetDriven(true)`; excluded from residuals, target refreshed to
  the measured value after every solve, `driven` flag serialized.

## Sketch-modification tools

These are what make it feel like a sketcher rather than a constraint solver.
The math layer now lives in `geom` (intersections + template modification);
the *mutating* versions on committed sketch geometry are blocked on
entity/constraint removal — sketches are append-only with creation-indexed
ids (see CLAUDE.md open questions).

- **Trim / extend / break** — *geom layer closed 2026-06*: line/circle/arc
  intersections (`LineLineIntersection`, `LineCircleIntersections`,
  `CircleCircleIntersections`, arc variants via `Arc.Contains`) plus
  `SplitLineAt`. Sketch-level trim of committed geometry remains open
  (removal prerequisite).
- **Offset** — offset a chain of curves with a single driving dimension; in
  Fusion the offset is itself a constraint, so the offset curve follows the
  original. (Single line/circle offsets are expressible today via
  `NewDistanceLines` and concentric + radius; the chain tool remains open.)
- **Fillet / chamfer** — *geom layer closed 2026-06*: `geom.Fillet` /
  `geom.Chamfer` shape templates before committing (replace the shared corner
  with contact points, return the tangent arc / cut line). Committed-geometry
  versions remain open (removal prerequisite).
- **Mirror** — creates mirrored copies *with symmetric constraints attached* so
  they stay linked.
- **Rectangular / circular patterns** — copies with pattern constraints (count
  and spacing can be parametric).
- **Project / intersect** — out of scope until 3D exists; worth a placeholder
  concept: entities whose geometry is externally driven and fully fixed
  ("reference" entities).

## Solver & diagnostics

- ~~**Identify which constraint is redundant/conflicting**~~ — *partially
  closed 2026-06*: `Sketch.RedundantConstraints()` maps dependent Jacobian rows
  back to constraints (later-added duplicates are the ones reported). Still
  open: distinguishing conflicting from merely redundant, and add-time
  rejection (below).
- **Over-constrained rejection at add-time** — Fusion checks the *new*
  constraint against current rank before accepting it. Cheap as an opt-in API
  (e.g. `AddConstraintChecked`).
- **Under-constrained visualization data** — Fusion shows unconstrained
  geometry in blue. API equivalent: report which variables/entities still have
  free DOF (null-space of J).
- ~~**Dragging**~~ — *engine side closed 2026-06*: `Solve(WithGoal(p, x, y))`
  pulls a point toward a target while constraints hold exactly (two-phase:
  augmented pull + hard-only polish; see `docs/goal-solve-design.md`). Gesture
  policy (entity dragging semantics, snapping, hit-testing) deliberately stays
  in the future UI layer.

## Profiles & regions

Fusion's whole reason for sketching is closed profiles that feed extrude. The
engine eventually needs **loop detection**: find closed regions formed by the
non-construction curves, including regions bounded by parts of curves. Pure
computational geometry, sketch-independent — a natural future `geom` citizen.
Without it the "2D → 3D someday" door stays shut.

## Parameters

- Parameters with units and expressions — largely done.
- **Parameter dependency reporting** (which dimensions a parameter drives) and
  solve-failure attribution to a parameter — listed follow-up in CLAUDE.md,
  worth doing before a DSL.

## Suggested priority order

1. ~~**Rectangle/polygon/slot compound constructors**~~ — *done 2026-06*
   (`AddRectangle`/`AddPolygon`/`AddSlot` in `compound.go`).
2. ~~**Tangent/equal coverage for arcs + point↔line and line↔line distance
   dimensions**~~ — *done 2026-06*.
3. ~~**Driven dimensions**~~ — *done 2026-06*.
4. ~~**Redundant-constraint identification**~~ — *done 2026-06*
   (`RedundantConstraints()`; conflicting-vs-redundant still open).
5. ~~**Drag-solve API**~~ — *done 2026-06* as goal-solve
   (`Solve(WithGoal(…))`; design in `docs/goal-solve-design.md`).
6. **Offset/fillet/trim** (*geom math layer done 2026-06*; sketch-level
   mutation blocked on entity removal), then **ellipse**, then
   **profiles/loop detection**, with **splines** last (largest solver impact).

Splines still deserve a design doc before code, as does entity/constraint
removal (the prerequisite for mutating sketch tools).
