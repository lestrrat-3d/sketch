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
- **Slot** (straight and arc slot) — in Fusion a compound: two arcs + two lines
  + auto-applied tangent/equal/concentric constraints. Good test of whether the
  "auto-added internal constraints" pattern generalizes.
- **Rectangle / polygon constructors** — also compounds (4 lines +
  perpendicular/horizontal constraints; n lines + equal + angle). Pure
  convenience layer over existing primitives; cheap wins.
- **Construction geometry flag** — load-bearing and cheap: any primitive can be
  marked construction-only so it participates in constraints but is excluded
  from profiles/export. Fusion sketching is unusable without this (symmetry
  axes, pitch circles).

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
- **Distance point↔line** (perpendicular distance).
- **Distance line↔line** (parallel lines).
- **Distance to circle/arc tangent** (Fusion's dimension-to-tangent option).
- **Driven (reference) dimensions** — a dimension that *measures* without
  constraining. Solver terms: excluded from residuals, evaluated after solve.
  Important for the DOF story and very cheap to add.

## Sketch-modification tools

None exist yet; these are what make it feel like a sketcher rather than a
constraint solver:

- **Trim / extend / break** — needs curve–curve intersection math (natural fit
  for `geom`).
- **Offset** — offset a chain of curves with a single driving dimension; in
  Fusion the offset is itself a constraint, so the offset curve follows the
  original.
- **Fillet / chamfer** between two sketch lines (inserts arc + tangent
  constraints).
- **Mirror** — creates mirrored copies *with symmetric constraints attached* so
  they stay linked.
- **Rectangular / circular patterns** — copies with pattern constraints (count
  and spacing can be parametric).
- **Project / intersect** — out of scope until 3D exists; worth a placeholder
  concept: entities whose geometry is externally driven and fully fixed
  ("reference" entities).

## Solver & diagnostics

- **Identify which constraint is redundant/conflicting** — counts are reported
  today; Fusion points at the offending constraint and refuses to add it
  interactively. QR/SVD on the Jacobian to find dependent rows gets most of the
  way. (Already in CLAUDE.md open questions.)
- **Over-constrained rejection at add-time** — Fusion checks the *new*
  constraint against current rank before accepting it. Cheap as an opt-in API
  (e.g. `AddConstraintChecked`).
- **Under-constrained visualization data** — Fusion shows unconstrained
  geometry in blue. API equivalent: report which variables/entities still have
  free DOF (null-space of J).
- **Dragging** — Fusion's defining interaction: grab a point, solver re-solves
  continuously with the dragged point as a soft target. Engine-side: a solve
  with an extra low-weight residual pulling toward the cursor. Needed before
  any GUI layer works.

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

1. **Construction-geometry flag** + rectangle/polygon/slot compound
   constructors — cheap, immediately makes examples look like real sketches.
2. ~~**Tangent/equal coverage for arcs**~~ (*done 2026-06*) + point↔line and
   line↔line distance dimensions — rounds out the constraint matrix.
3. **Driven dimensions** — small, big payoff for diagnostics.
4. **Redundant-constraint identification** — turns the existing rank analysis
   into a usable answer.
5. **Drag-solve API** — prerequisite for any interactive layer.
6. **Offset/fillet/trim**, then **ellipse**, then **profiles/loop detection**,
   with **splines** last (largest solver impact).

Items 1–4 fit the current architecture without structural change. Dragging and
splines deserve a design doc before code.
