# Fusion-Compatible Sketcher: Feature Gap Analysis

Gap analysis of this engine against the Autodesk Fusion sketch environment. This
file lists what Fusion offers that the engine does not yet do. Closed items are
named only so a reader can find the API; the design docs own the detail.

For the goal-oriented, prioritized roadmap toward the headless verification
oracle, and the sketch/3D separation contract, see `docs/verification-roadmap.md`.

## Geometry primitives

**Closed:** point, line, circle, arc, ellipse, elliptical arc, open spline
(`CreateSpline`), closed spline (`CreateClosedSpline`), fit-point spline
(`CreateFitSpline`), conic (`CreateConic`), NURBS (`CreateNURBS`), straight slot
(`CreateSlot`), rectangle (`CreateRectangle`), regular polygon (`CreatePolygon`),
three-point arc (`CreateArcThreePoints`), tangent arc (`CreateArcTangent`),
three-tangent circle (`CreateCircleThreeTangents`), arc slot (`CreateArcSlot`),
and reference geometry (point, line, arc, circle). Any entity can be flagged as
construction geometry. Inscribed and circumscribed polygons and overall-length or
center-point slots are recipes on existing builders
(`examples/sketch_shape_recipes_example_test.go`).

**Open:**

- **Reference elliptical arc, ellipse, spline and conic.** Only point, line, arc
  and circle have `CreateReference…` builders.

## Constraints

**Closed:** the geometric set (coincident, collinear, concentric, horizontal,
vertical, parallel, perpendicular, equal, midpoint, symmetric for points, lines,
circles and arcs, tangent, point-on-curve for every curve type) and the dimensional
set (distance between points, lines, and point/line to circle or arc, horizontal
and vertical distance, angle, radius, diameter, arc length, ellipse axes and rotation, offset).
Dimensions accept the `Circular` interface, so arcs work wherever circles do.
Driven (reference) dimensions are in (`SetDriven`). `Sketch.FixEntity` grounds a
whole entity.

**Open:**

- **Curvature continuity.** There is no smooth (G2) constraint between splines,
  or between a spline and an arc. Tangent-to-spline constrains direction only.
- **One coincident for every operand pair.** Fusion's single Coincident covers
  point-on-line and point-on-curve. Here those are separate constructors
  (`NewPointOnLine`, `NewPointOnArc`, …). Only the naming differs.

## Sketch-modification tools

**Closed:** `Trim`, `Extend`, `Break`, `CreateFillet`, `CreateChamfer`,
`CreateMirror`, `CreatePatternRect`, `CreatePatternCircular` and `CreateOffset`.
Design in `docs/modification-tools-design.md`.

**Open:**

- **Move, copy, rotate, scale and stretch.** The sketch has no API for any of them.
  Mirror and the two patterns are the only ways to copy geometry.
- **Offset of arcs and circles.** `CreateOffset` skips every entity that is not a
  line.
- **Trim, extend, fillet and chamfer on curves.** `Trim` and `Extend` take a
  `*Line`, and `CreateFillet` and `CreateChamfer` take two `*Line` operands.
  `Break` accepts any entity.
- **Project, include and intersect.** These stay out by design. The engine verifies
  against 2D curves you hand it as reference geometry and never computes them from
  a solid.

## Solver and diagnostics

**Closed:**

- `Sketch.Diagnose` separates redundant constraints from conflicting ones, and
  `ConflictSet` names the earlier constraints each conflicting constraint fights.
- `Sketch.CheckConstraint` tests a constraint against the current rank before
  `AddConstraint`. A caller who wants Fusion's refuse-the-gesture behavior calls it
  first. `AddConstraint` itself never refuses.
- `FreePoints`, `Point.IsFullyConstrained` and `EntityIsFullyConstrained` report
  free degrees of freedom, and `WithDOFColoring` draws them.
- `Solve(ctx, WithGoal(p, x, y))` pulls a point toward a target while the
  constraints hold. Design in `docs/goal-solve-design.md`.
- `Sketch.Verify` aggregates solvability, status, conflicts, profiles and chains.

**Open:**

- **Drag policy.** Entity-level dragging, snapping and hit-testing belong to a
  future UI layer, not to this engine.

## Profiles and regions

`Sketch.Profiles()` runs the `geom` planar arrangement over all non-construction
geometry. Lines, arcs, circles, ellipses, elliptical arcs, splines, closed splines,
fit splines, conics and NURBS all take part. Output covers crossing subdivision,
holes, nested regions, net area, orientation, and a validity flag that makes
`VerificationReport.Trustworthy()` false for self-intersecting or degenerate
boundaries. `Sketch.Chains()` publishes the open runs no region uses. Area
exactness differs per curve type; `.claude/docs/profiles-geom.md` records which
fragments are exact and which are sampled.

## Parameters

**Closed:** named parameters with units and expressions in a table shared by the
world, expressions bound to dimensions, and offset planes driven by a parameter.
`param.Table.Dependencies` lists the parameters a parameter's expression reads.

**Open:**

- **Parameter-to-dimension reporting.** No API lists the dimensions a parameter
  drives, or names the parameter behind a failed solve.

## Outside the engine's scope

- **3D sketches.** A sketch always lies on one plane of a `World`.
- **Text.** Sketch text outlines are not modelled.
- **Images, canvases and sketch blocks.** None exist.
- **Importers.** Sketches are authored through the API. DXF and SVG import would
  recover geometry but not constraints; `docs/verification-roadmap.md` owns the
  decision to defer importers.
