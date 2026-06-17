# Fusion-Compatible Sketcher: Feature Gap Analysis

Gap analysis of this engine against the Autodesk Fusion sketch environment.
Snapshot date: 2026-06-10. Baseline at time of writing: 4 primitives
(point/line/circle/arc), 15 geometric + 6 dimensional constraints, LM solver
with DOF/redundancy counts, `param` table, `units`, SVG/DXF/JSON export.

This is the exhaustive feature-by-feature checklist. For the goal-oriented,
prioritized roadmap toward the headless verification oracle — and the sketch/3D
separation contract — see `docs/verification-roadmap.md`.

## Geometry primitives

**Have:** point, line, circle, arc.

**Missing**, roughly in order of how often Fusion users reach for them:

- ~~**Ellipse**~~ — *closed 2026-06*: `geom.NewEllipse`/`AddEllipse` with a
  center point plus semi-axis/rotation unknowns, `NewPointOnEllipse`
  (Sampson-normalized residual), `NewSemiMajor`/`NewSemiMinor`/
  `NewEllipseRotation` dimensions, JSON/SVG/DXF support. Still open:
  **elliptical arcs**, and **tangency to an ellipse** (no closed-form
  distance; needs a foot-point iteration or an auxiliary contact-point
  variable).
- ~~**Splines**~~ — *v1 closed 2026-06*: control-point clamped cubic
  B-splines (`geom.NewSpline`/`AddSpline`); control points are ordinary
  sketch points, so constraints/dimensions/goals reshape the curve with no
  new solver machinery (design: `docs/spline-design.md`). Still open:
  fit-point splines, point-on/tangent-to-spline constraints (v2
  aux-parameter design recorded in the design doc), closed/periodic splines.
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
  `NewTangentCircles` and `NewEqualRadius` to accept arcs. Arc tangency
  **enforces the sweep**: shared-endpoint (fillet) tangency is a clean
  perpendicular/collinear equality, and interior tangency adds a slack-encoded
  in-sweep inequality, so a line tangent to the full circle but not touching the
  arc is reported unsolvable (no false-positive tangency).
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
*Closed 2026-06* — all of trim/extend/break, fillet/chamfer, mirror and
patterns are built in `tools.go` via the build-then-replace pattern (geom
toolkit + `RemoveEntity`); offset added a new `Offset` constraint. Design in
`docs/modification-tools-design.md`; tests in `tools_test.go`.

- **Trim / extend / break** — *closed 2026-06*: `Trim`/`Extend`/`Break` on
  committed geometry (geom layer — `LineLineIntersection`,
  `LineCircleIntersections`, `CircleCircleIntersections`, arc variants,
  `SplitLineAt`/`SplitArcAt`, `ClosestPointOnLine` — plus the sketch-level
  replace tools).
- **Offset** — *closed 2026-06*: `AddOffset` offsets a chain at a signed
  distance; the new `Offset` constraint keeps each segment parallel at distance
  d and mitres shared corners at the offset intersection, so editing
  `OffsetGroup.Set(d)` moves the copy. (Arc/concentric chain offset still open.)
- **Fillet / chamfer** — *closed 2026-06*: `AddFillet` / `AddChamfer` on
  committed corners (arc/cut + tangency/coincidence + editable radius/setback
  dimensions), wrapping the `geom.Fillet`/`geom.Chamfer` template helpers.
- **Mirror** — *closed 2026-06*: `AddMirror` creates mirrored copies *with
  symmetric constraints attached* (plus equal-radius for circles) so they stay
  linked.
- **Rectangular / circular patterns** — *closed 2026-06*: `AddPatternRect` /
  `AddPatternCircular` create copies rigidly tied to the seed by distance /
  construction-spoke constraints, so the field follows the seed. (A single
  shared-parameter spacing knob is a recorded follow-up.)
- ~~**Project / intersect**~~ — the *representation* is in: **reference
  geometry** (`reference.go`) — read-only, externally-locked entities with a
  source id + staleness, the snapshot a `Project`/`Include`/`Intersect` would
  hand in. Computing the projection from a solid stays above this layer (the
  separation contract); here you author the resulting 2D curve/point as
  reference geometry and verify against it.

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

Fusion's whole reason for sketching is closed profiles that feed extrude.
*Partially closed 2026-06*: `geom.Loops` finds closed chains of lines/arcs
connected by shared endpoint identity, and `Sketch.Profiles()` returns those
loops plus every non-construction circle/ellipse as `Profile` values. Still
open: **region subdivision at bare crossings** — boundaries that intersect
without sharing a point are not split into faces (the curve-splitting math
exists in `geom`'s intersection toolkit; the subdivision algorithm does not),
and loop **area/winding** classification (outer boundary vs hole).

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
6. ~~**Offset/fillet/trim**~~ — *done 2026-06* (all sketch-modification tools
   in `tools.go`: trim/extend/break, fillet/chamfer, mirror, patterns, offset;
   design in `docs/modification-tools-design.md`),
   then ~~**ellipse**~~ (*done 2026-06*; elliptical arcs and ellipse tangency
   still open), then ~~**profiles/loop detection**~~ (*shared-endpoint loops
   done 2026-06*; region subdivision at bare crossings still open), with
   ~~**splines**~~ (*v1 done 2026-06*; fit-point and point-on-spline
   constraints still open).

Entity/constraint removal is *done 2026-06*
(`RemoveConstraint`/`RemoveEntity`/`RemovePoint`; design in
`docs/removal-design.md`; documents now carry a schema version).
