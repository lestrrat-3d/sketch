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
  the flat `vars` vector via the existing `CreatePoint` path, the solver moves
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
  (mirrors `CreatePolygon`'s contract for invalid construction).
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
- `sketch.Spline` via `CreateSpline(g *geom.Spline)`: commits the control points
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

## Tangent-to-spline (`NewTangentToSpline`)

A line tangent to a spline reuses the bounded contact-parameter `t` machinery
(plus the box slacks `w0,w1`). The committed residual is five rows:

- **contact on the carrier line** (length): signed perpendicular distance from
  `S(t)` to the *infinite* line through the segment — the line is treated as its
  carrier, matching `tangentLineCircle` and `NewPointOnLine`; only the finite
  spline side is bounded (to `[0,1]`).
- **parallel** (dimensionless): `cross(d̂, Ŝ'(t))` = `sin` of the angle between the
  line direction and the spline tangent, zero when parallel.
- the two **box rows** `t=w0²`, `1−t=w1²`.
- a **no-cusp guard** `|S'(t)|/scale − epsTan = ws²` (extra slack `ws`,
  scale = control-box diagonal): at a cusp the tangent direction is undefined and
  `cross(d̂, 0)=0` would falsely bless any line, so the guard makes a sub-`epsTan`
  speed infeasible. A zero-length line is rejected outright in the parallel row.

`S'(t)` is the **analytic** `geom.EvalCubicBSplineDeriv` (a numerical tangent
inside the residual would be a nested finite difference the outer numerical
Jacobian re-differentiates). Seeding (`allocVars`) is a dense multi-start
minimizing `(contact/scale)² + parallel²` (skipping near-cusps) + golden-section
refine — distance-only or parallelism-only seeds each miss a common case. DOF: a
free line goes 4→3 (one removed). Multiple tangencies are an existential
choice the probe layer can surface; a line tangent *at* a point shared with a
point-on-spline would need a combined contact object owning one `t` (independent
constraints own independent `t`) — not in scope.

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

## Profiles

A `geom.Spline` is a `geom.Curve` (`Endpoints()` = first/last control points, which
a clamped cubic passes through), so it participates in the `geom.Regions` planar
arrangement: it is sampled to a polyline (`max(64, 16·(n−3))` segments) like an arc
or ellipse, its fragment area is the sampled bulge (`signedPolyArea`, not exact),
and `Sketch.buildProfiles` feeds it through the shared `*geom.Point` map so its
endpoints join adjacent curves. Self-crossing detection is spline-specific: the
arrangement's same-source skip is lifted for non-adjacent segments of one spline,
and an endpoint-touch between two such segments counts as a self-touch (the exact
crossing can land on a sample vertex), so a self-intersecting cubic is flagged
`SelfIntersecting` rather than blessed.

## Closed (periodic) splines

`ClosedSpline` (`CreateClosedSpline`, ≥3 control points) is a separate entity from the
open `Spline`: a smooth closed loop, C2 across the seam, over an **exact cyclic
uniform cubic basis** — `geom.EvalPeriodicCubicBSpline` blends the four cyclic
controls `P[i..i+3]` (indices mod n) per unit span with the standard uniform cubic
weights, reducing `t` modulo 1 so `Eval(0) == Eval(1)`. (The wrap trick of feeding
the clamped basis an augmented control list is *not* periodic — the clamped
evaluator pins the ends — so a real cyclic basis is used instead.) It carries no
solver vars and no internal constraints, like the open spline.

Because it bounds a region on its own it is a `geom.ClosedCurve`, **not** a
`geom.Curve` — it has no `Endpoints()`. `ClosedCurve` is sealed with an unexported
marker so the open `*Spline` (which also has a `Polyline` method) cannot
accidentally satisfy it. `buildProfiles` routes a closed spline to the arrangement's
`closed` argument (its own component, like a circle/ellipse), with sampled bulge
area. Self-crossing detection reuses the open-spline same-source test extended to
`srcClosedSpline`; the periodic seam (the first sampled segment meeting the last) is
the param-`{0,1}` closure already skipped by the endpoint-meeting branch, so a
self-crossing closed loop is flagged `SelfIntersecting` while a simple one is not.
Serialized as a distinct `"closed_spline"` type (an older reader rejects it rather
than misloading it as open); exported as a sampled path (SVG/PNG) and a closed
`LWPOLYLINE` (DXF). Point-on / tangent constraints on a closed spline are a deferred
follow-up (they need periodic-witness handling, not the clamped `t∈[0,1]` box).

## Fit-point (interpolating) splines

`FitSpline` (`CreateFitSpline`, ≥2 fit points) is a separate entity whose curve passes
*through* its fit points, unlike the control-point `Spline` whose polygon only
approximates. The load-bearing decision is that the **fit points are the durable
solver handles** (ordinary sketch points), and the interpolating curve is recomputed
from their current coordinates on every evaluation — so the curve keeps interpolating
them even after the solver moves them, with no new solver vars and no internal
constraints. (Deriving control points once at build time was rejected: the solver
would then move the *controls*, and the interpolation invariant would drift.)

The interpolation is a **natural cubic spline** (zero second derivative at the ends —
no hidden tangent inputs, and two points evaluate as a straight line) with
**chord-length parameterization** (avoids overshoot on unevenly spaced points). The
per-coordinate second derivatives come from a tridiagonal **Thomas** solve
(`geom.EvalFitSpline` one-off; `SampleFitSpline` / the arrangement build one
`fitEvaluator` and reuse it across samples, so the solve runs once, not per sample).
Consecutive coincident fit points are collapsed (a zero-length chord has no
parameter); an all-coincident set is degenerate. It is an open `geom.Curve`
(endpoints = first/last fit point, which it passes through exactly), so it slots into
the open-spline arrangement path — sampled area, same-source self-crossing — and the
fit points join adjacent geometry by shared-`*Point` identity. Serialized as a
distinct `"fit_spline"` type; exported as a sampled path (SVG/PNG) and an open
`LWPOLYLINE` (DXF — the derived controls are not clamped-uniform, so no native
`SPLINE`). Point-on / tangent constraints on a fit spline are a deferred follow-up
(the interpolation solve and chord parameters shift as the solver moves the fit
points — real solver work, not just an overload).

## Out of scope (recorded)

- Point-on / tangent constraints on a *closed* or *fit-point* spline (deferred).
- Not-a-knot / clamped-tangent fit-spline end conditions (natural is the v1 default).
- Custom knots, weights (NURBS).
- Splitting/trim of splines.

## Testing plan

- `geom`: `Eval(0)`/`Eval(1)` hit the first/last control points; the curve at
  t=0.5 of a symmetric control polygon lies on the symmetry axis; a known
  4-point case matches the closed-form cubic Bézier (a clamped cubic B-spline
  with exactly 4 control points *is* the Bézier — strong oracle).
- `sketch`: control points respond to constraints (fix + dimension, solve,
  assert solved `Eval` values); `CreateSpline` idempotent; JSON round-trip
  (entity + control-point references, degree checked); SVG `<path>` present;
  DXF `SPLINE` present with correct counts.
