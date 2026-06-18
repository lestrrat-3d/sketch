# Splines — Design

Status: **implemented** (v1: `geom/spline.go`, `spline.go`; tests in
`geom/spline_test.go`, `spline_test.go`). This document scopes a v1 that fits
the existing architecture exactly, and records the v2 path for the parts that
don't.

## Choice: control-point clamped cubic B-spline

Fusion offers fit-point and control-point splines. v1 ships the
**control-point open cubic B-spline with a clamped uniform knot vector**,
because it is the variant whose unknowns are *already* the engine's native
currency:

- The control points are ordinary sketch points. Committing a spline commits
  its control points exactly like a line commits its endpoints — they land in
  the flat `vars` vector via the existing `AddPoint` path, the solver moves
  them, `Fix` grounds them, `WithGoal` drags them, and every existing
  point-based constraint (coincident, distance, symmetric, …) applies to them
  with **zero new solver machinery**.
- Clamping means the curve starts at the first control point and ends at the
  last, with end tangents along the first/last control-polygon legs. So
  endpoint coincidence is point coincidence, and end-tangency is a parallel
  constraint on a construction line over the first/last leg — both already
  exist.
- No internal constraints are needed: any control polygon is a valid spline
  (unlike the arc, which needs its radius-consistency residual).

Fit-point splines (curve interpolates the points) are deferred: they are a
*construction* convenience (solve a tridiagonal system for control points at
build time) that can layer on later without touching the solver.

## Shape

- `geom.Spline`: `Control []*Point`, `Construction bool`. Degree is fixed at
  3; `NewSpline(control ...*Point)` panics with fewer than 4 control points
  (mirrors `AddPolygon`'s contract for invalid construction).
- Knots: clamped uniform — `[0,0,0,0, 1/(n−3), …, (n−4)/(n−3), 1,1,1,1]` for
  n control points. Not stored; derived from n. (Custom knots/weights — NURBS
  — are out of scope.)
- Evaluation: `Eval(t float64) (float64, float64)` for t ∈ [0,1] (clamped),
  via Cox–de Boor basis functions. **At t = 1 return the last control point
  directly** — the standard half-open basis convention makes every degree-0
  basis zero at the trailing multiplicity-4 knot, so naive Cox–de Boor returns
  (0,0) there; the shortcut is valid because the knot vector is always
  clamped. The basis lives in one place in `geom` and is shared by the sketch
  layer through an exported helper
  (`geom.EvalCubicBSpline(ctrl [][2]float64, t float64)`), since the sketch
  must evaluate at *solved* coordinates while `geom.Spline.Eval` uses template
  coordinates. `Polyline(segments int)` samples for rendering/bounds, like
  `arcPolyline`.
- `sketch.Spline` via `AddSpline(g *geom.Spline)`: commits the control points
  (idempotent, shared like all points), holds `Control []*Point` bound
  handles, exposes `Eval`/`Polyline` over solved coordinates. No new vars on
  the spline itself; no internal constraints. `Sketch` gains a
  `splOf map[*geom.Spline]*Spline` for the usual Add idempotency.

## Constraints in v1 — deliberately none new

Everything a v1 user needs is composition:

- **Endpoint attachment**: coincident (or shared point) with other geometry.
- **End tangency**: a construction line over the first leg (P0→P1) +
  `NewParallel` against the neighboring line. Document the recipe.
- **Shaping**: distance/symmetry/goal constraints on interior control points.

## Point-on-spline (`NewPointOnSpline`)

A B-spline has no implicit `F(x,y)=0`, so curve membership is the existential
`P = S(t)`: the constraint owns the foot-point parameter `t` as an **auxiliary
solver variable** (a foot-point search inside the residual would be a
discontinuous argmin that fights the numerical Jacobian), allocated by the
`allocVars(*Sketch)` hook. `t` is bounded to `[0,1]` by a **slack-encoded box**
(two more aux vars `w0,w1`, rows `t=w0²` and `1−t=w1²`) so an out-of-range `t`
is genuinely infeasible rather than silently absorbed by `Eval`'s endpoint
clamp. The committed residual is four rows: `P.x−S.x(t)`, `P.y−S.y(t)`,
`t−w0²`, `(1−t)−w1²` — a free point on a fixed spline keeps exactly one sliding
DOF (5 unknowns, 4 independent rows).

Load-bearing decisions:

- **Aux vars are not serialized** (house convention): `allocVars` re-seeds `t`
  by a robust foot-point projection on load — dense per-segment polyline
  projection (`geom.NearestParamCubicBSpline`) plus golden-section refinement,
  not nearest-sample. For a self-intersecting / near-self-touching spline two
  foot points can tie, so a reloaded sketch may witness membership at a
  different `t` than the original solve; it is still a valid witness (residual
  0), so **solvability is preserved** — only the specific `t` may differ. (If
  that determinism ever matters, serializing `t` as a warm-start in
  `jsonConstraint.Value` is the recorded escape hatch.)
- **`CheckConstraint` probes the committed form.** The arc-slack pattern does
  not transfer: an arc's on-circle row is meaningful before `allocVars`, but a
  spline's contact rows are meaningless without the free `t`. So `CheckConstraint`
  **temporarily allocates a candidate's aux vars** (any constraint with the
  `allocVars` hook), ranks the real committed rows with those vars exposed as free
  unknowns, then rolls back — keeping the check non-mutating. This is general (it
  also makes the arc/tangent probes faithful) and needs no special probe residual.
  *Known limitation:* two point-on-spline on the same point are redundant only
  **nonlinearly** (`S(t1)=S(t2)` forces `t1=t2` only at the solution), so the
  local rank analysis is **not guaranteed** to flag the duplicate (it may, when
  both foot seeds coincide). It is harmless — the sketch stays solvable with one
  sliding DOF; the duplicate just adds an unused second witness. An exact
  same-point duplicate could be caught by a semantic scan if a guarantee is wanted.

**Tangent-to-spline is the recorded follow-up**: the same bounded-`t` machinery
plus the spline tangent `S'(t)` — rows "contact on the line" and
"cross(line dir, `S'(t)`) = 0", guarding `|S'|→0` (a cusp is not a tangent).
A line tangent *at* a shared point needs a combined contact object owning one
bounded `t`, since independent constraints own independent `t`.

## Serialization & export

- JSON: entity `"spline"` with `points` = control-point ids (already the
  schema's reference style) and `degree: 3` written for forward compatibility
  (readers reject other degrees for now).
- SVG: sampled polyline `<path>`, same approach as arcs. The existing
  `WithArcSegments` option governs fidelity — do **not** add a separate
  spline-segments option. Exact cubic-Bézier conversion is a possible
  refinement.
- DXF: `SPLINE` entity (R13+, like the ELLIPSE already emitted): degree,
  knot/control counts, knot values, control points.
- Bounds (for SVG framing): polyline sample — the control polygon's convex
  hull would overshoot.

## Solver interplay (why this is "splines in the solver")

The solver sees control-point coordinates as unknowns, so dimensions and
constraints on control points reshape the curve through the normal solve.
The acceptance test for that claim: fix one end, dimension the control
polygon, solve, and assert `Eval` against independently computed B-spline
values.

## Out of scope (recorded)

- Fit-point splines (build-time convenience layer).
- Point-on/tangent-to spline constraints (v2; aux-parameter design above).
- Closed/periodic splines, custom knots, weights (NURBS).
- Spline participation in `geom.Loops`/profiles (needs endpoints — could join
  chains as a `Curve` once needed; one line of code, deferred until profiles
  consumers exist).
- Splitting/trim of splines.

## Testing plan

- `geom`: `Eval(0)`/`Eval(1)` hit the first/last control points; the curve at
  t=0.5 of a symmetric control polygon lies on the symmetry axis; a known
  4-point case matches the closed-form cubic Bézier (a clamped cubic B-spline
  with exactly 4 control points *is* the Bézier — strong oracle).
- `sketch`: control points respond to constraints (fix + dimension, solve,
  assert solved `Eval` values); `AddSpline` idempotent; JSON round-trip
  (entity + control-point references, degree checked); SVG `<path>` present;
  DXF `SPLINE` present with correct counts.
