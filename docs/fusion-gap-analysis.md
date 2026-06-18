# Fusion-Compatible Sketcher: Feature Gap Analysis

Gap analysis of this engine against the Autodesk Fusion sketch environment. The
engine baseline: primitives (point/line/circle/arc/ellipse/spline), geometric +
dimensional constraints, LM solver with DOF/redundancy counts, `param` table,
`units`, SVG/DXF/JSON export.

This is the exhaustive feature-by-feature checklist. For the goal-oriented,
prioritized roadmap toward the headless verification oracle — and the sketch/3D
separation contract — see `docs/verification-roadmap.md`.

## Geometry primitives

**Have:** point, line, circle, arc.

**Missing**, roughly in order of how often Fusion users reach for them:

- ~~**Ellipse**~~ — *closed*: `geom.NewEllipse`/`AddEllipse` with a
  center point plus semi-axis/rotation unknowns, `NewPointOnEllipse`
  (Sampson-normalized residual), `NewSemiMajor`/`NewSemiMinor`/
  `NewEllipseRotation` dimensions, JSON/SVG/DXF support.
- ~~**Elliptical arc**~~ — *closed (geometry primitive)*: `AddEllipticalArc`
  (center + start/end points + rx/ry/rotation vars), with two internal
  on-ellipse constraints pinning the endpoints (Sampson residual);
  eccentric-angle `Sweep`, `geom.EllipticalArc` sampling, profile/arrangement
  participation as an open curve (sampled-bulge area), JSON round-trip, and
  SVG/PNG/native-DXF-ELLIPSE export, and **shape dimensions** (`NewSemiMajor`/
  `NewSemiMinor`/`NewEllipseRotation` widened to the sealed `Elliptical`
  interface accepting a `*Ellipse` or `*EllipticalArc`). Still open (follow-ups):
  sweep-confined point-on / tangency on an elliptical arc (tangency needs
  **tangency to an ellipse** — no closed-form distance; a foot-point iteration
  or an auxiliary contact-point variable), reference elliptical arcs, and
  trim/split.
- ~~**Splines**~~ — *v1 closed*: control-point clamped cubic
  B-splines (`geom.NewSpline`/`AddSpline`); control points are ordinary
  sketch points, so constraints/dimensions/goals reshape the curve with no
  new solver machinery (design: `docs/spline-design.md`). Still open:
  fit-point splines, point-on/tangent-to-spline constraints (v2
  aux-parameter design recorded in the design doc), closed/periodic splines.
- ~~**Slot** (straight)~~ — *closed*: `AddSlot` (two arcs + two flanks;
  equal cap radii + perpendicular construction spokes at the contact points —
  perpendicularity implies tangency *and* pins the contact point, which a plain
  tangent constraint does not). Arc slot still open.
- ~~**Rectangle / polygon constructors**~~ — *closed*: `AddRectangle`
  (H/V constraints) and `AddPolygon` (equal sides + equal construction spokes).
- **Construction geometry flag** — already existed (`.Construction` on any
  entity; rendered dashed, separate DXF layer).

## Constraints

The geometric set is already close to Fusion's. Remaining gaps:

- ~~**Tangent: line–arc, arc–arc, arc–circle**~~ — *closed*: the
  `Circular` interface (`*Circle` | `*Arc`) generalized `NewTangent`,
  `NewTangentCircles` and `NewEqualRadius` to accept arcs. Arc tangency
  **enforces the sweep**: shared-endpoint (fillet) tangency is a clean
  perpendicular/collinear equality, and interior tangency adds a slack-encoded
  in-sweep inequality, so a line tangent to the full circle but not touching the
  arc is reported unsolvable (no false-positive tangency).
- ~~**Radius / diameter / concentric on arcs**~~ — *closed*: `NewRadius`,
  `NewDiameter` and `NewConcentric` take the `Circular` interface (`*Circle` |
  `*Arc`), so an arc's radius/diameter is dimensionable and arcs are concentric
  with circles or each other. An arc operand must have a nonzero radius (its
  start ≠ its center) or the radius residual has no gradient.
- ~~**Horizontal / vertical between bare points**~~ — *closed*:
  `NewHorizontalPoints` / `NewVerticalPoints` force two points to share a y / x
  without a connecting line (the line-entity forms `NewHorizontal`/`NewVertical`
  remain).
- ~~**Midpoint of a bare point pair**~~ — *closed*: `NewMidpointOf(mid,
  p1, p2)` complements the line-entity `NewMidpoint`.
- ~~**Point-on-arc**~~ — *closed*: `NewPointOnArc` confines a point to the arc's
  circle **and** its sweep, reusing the interior-tangency slack-encoded sweep
  inequality, so a point on the full circle but off the arc is reported
  unsolvable. Point-on-spline (and a unified coincident-to-curve) remain open.
- ~~**Symmetric for whole entities**~~ — *partially closed*: `NewSymmetricLines`
  (endpoint-for-endpoint mirror) and `NewSymmetricCircles` (centers symmetric +
  equal radius). Arc symmetry is still open — a reflection reverses an arc's
  sweep, so it must swap and mirror the endpoints, not yet modelled.
- **Equal for line↔arc mixed** — Fusion's "equal" across a line and an arc
  equates line length to arc *length*. Now feasible by composing the arc-length
  aux variable (`NewArcLength`) with a line's length — a small follow-up. Line-
  line (length) and circle/arc-radius equality already exist
  (`NewEqual`/`NewEqualRadius`).
- ~~**Fix/ground a whole entity**~~ — *closed*: `Sketch.FixEntity`/`UnfixEntity`/
  `EntityFixed` ground all of an entity's variables (points + circle radius /
  ellipse axes); `UnfixEntity` leaves shared reference-locked points untouched.
- **Coincident point-to-entity** — Fusion's coincident subsumes
  point-on-line/point-on-curve under one name; the pieces exist, this is
  naming/UX for the future DSL.

### Dimensional gaps

- ~~**Arc length dimension**~~ — *closed*: `NewArcLength`. The discontinuous
  `R·Sweep()` is replaced by an auxiliary unwrapped-sweep variable driving
  `R·theta = L`, pinned to the geometry by a branch-selecting wrapped-angle
  coupling row — `(Δ − theta)` wrapped into `(−π, π]`, dimensionless, like the
  Angle dimension — reusing the tangency sweep slack's `allocVars`/`retireVars`
  lifecycle. Drive-only in v1 (a driven dimension contributes no residual, which
  would orphan the aux var); driven/reference arc-length is a follow-up.
- ~~**Distance point↔line**~~ — *closed*: `NewDistancePointLine`.
- ~~**Distance line↔line**~~ — *closed*: `NewDistanceLines` (two
  residuals; forces parallelism, no separate parallel constraint needed).
- **Distance to circle/arc tangent** (Fusion's dimension-to-tangent option).
- ~~**Driven (reference) dimensions**~~ — *closed*:
  `Dimension.SetDriven(true)`; excluded from residuals, target refreshed to
  the measured value after every solve, `driven` flag serialized.

## Sketch-modification tools

These are what make it feel like a sketcher rather than a constraint solver.
*Closed* — all of trim/extend/break, fillet/chamfer, mirror and
patterns are built in `tools.go` via the build-then-replace pattern (geom
toolkit + `RemoveEntity`); offset added a new `Offset` constraint. Design in
`docs/modification-tools-design.md`; tests in `tools_test.go`.

- **Trim / extend / break** — *closed*: `Trim`/`Extend`/`Break` on
  committed geometry (geom layer — `LineLineIntersection`,
  `LineCircleIntersections`, `CircleCircleIntersections`, arc variants,
  `SplitLineAt`/`SplitArcAt`, `ClosestPointOnLine` — plus the sketch-level
  replace tools).
- **Offset** — *closed*: `AddOffset` offsets a chain at a signed
  distance; the new `Offset` constraint keeps each segment parallel at distance
  d and mitres shared corners at the offset intersection, so editing
  `OffsetGroup.Set(d)` moves the copy. (Arc/concentric chain offset still open.)
- **Fillet / chamfer** — *closed*: `AddFillet` / `AddChamfer` on
  committed corners (arc/cut + tangency/coincidence + editable radius/setback
  dimensions), wrapping the `geom.Fillet`/`geom.Chamfer` template helpers.
- **Mirror** — *closed*: `AddMirror` creates mirrored copies *with
  symmetric constraints attached* (plus equal-radius for circles) so they stay
  linked.
- **Rectangular / circular patterns** — *closed*: `AddPatternRect` /
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
  closed*: `Sketch.RedundantConstraints()` maps dependent Jacobian rows
  back to constraints (later-added duplicates are the ones reported). Still
  open: distinguishing conflicting from merely redundant, and add-time
  rejection (below).
- **Over-constrained rejection at add-time** — Fusion checks the *new*
  constraint against current rank before accepting it. Cheap as an opt-in API
  (e.g. `AddConstraintChecked`).
- **Under-constrained visualization data** — Fusion shows unconstrained
  geometry in blue. API equivalent: report which variables/entities still have
  free DOF (null-space of J).
- ~~**Dragging**~~ — *engine side closed*: `Solve(WithGoal(p, x, y))`
  pulls a point toward a target while constraints hold exactly (two-phase:
  augmented pull + hard-only polish; see `docs/goal-solve-design.md`). Gesture
  policy (entity dragging semantics, snapping, hit-testing) deliberately stays
  in the future UI layer.

## Profiles & regions

Fusion's whole reason for sketching is closed profiles that feed extrude.
*Closed:* `Sketch.Profiles()` runs the `geom` planar-arrangement engine
(`geom.Regions`) over all non-construction lines/arcs/circles/ellipses.
**Bare-crossing subdivision** (boundaries that intersect without sharing a point
are split into faces), **holes/nesting** (a shape inside another is a hole + a
separate region), **net area** and **winding/orientation** (outer CCW, holes CW)
are all in. Each `Profile` carries `Outer`/`Holes` boundary edges (whole or
fragment), `Area`, and validity: a **self-intersecting** boundary (a simple
closed loop crossing itself) or a **degenerate** arrangement (coincident edges,
near-tangent uncertainty) reports `Valid=false` and makes
`VerificationReport.Trustworthy()` false. *Open:* splines in profiles; exact
ellipse-fragment area (currently sampled); an analytic (non-sampled) arrangement
for tighter tolerance on near-tangencies.

## Parameters

- Parameters with units and expressions — largely done.
- **Parameter dependency reporting** (which dimensions a parameter drives) and
  solve-failure attribution to a parameter — listed follow-up in CLAUDE.md,
  worth doing before a DSL.

## Suggested priority order

1. ~~**Rectangle/polygon/slot compound constructors**~~ — *done*
   (`AddRectangle`/`AddPolygon`/`AddSlot` in `compound.go`).
2. ~~**Tangent/equal coverage for arcs + point↔line and line↔line distance
   dimensions**~~ — *done*.
3. ~~**Driven dimensions**~~ — *done*.
4. ~~**Redundant-constraint identification**~~ — *done*
   (`RedundantConstraints()`; conflicting-vs-redundant still open).
5. ~~**Drag-solve API**~~ — *done* as goal-solve
   (`Solve(WithGoal(…))`; design in `docs/goal-solve-design.md`).
6. ~~**Offset/fillet/trim**~~ — *done* (all sketch-modification tools
   in `tools.go`: trim/extend/break, fillet/chamfer, mirror, patterns, offset;
   design in `docs/modification-tools-design.md`),
   then ~~**ellipse**~~ (*done*; the elliptical-arc primitive is in too —
   ellipse tangency still open), then ~~**profiles/region engine**~~ (*done* — bare-crossing
   subdivision, holes/nesting, area, self-intersection validity), with
   ~~**splines**~~ (*v1 done*; fit-point and point-on-spline
   constraints still open).

Entity/constraint removal is *done*
(`RemoveConstraint`/`RemoveEntity`/`RemovePoint`; design in
`docs/removal-design.md`; documents now carry a schema version).
