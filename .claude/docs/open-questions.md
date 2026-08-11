# Open design questions

Moved verbatim out of CLAUDE.md. These are unsettled design variables. If you resolve one, record the decision here.

## Question router

| Your question | Section |
|---|---|
| Are parameters/expressions settled? | Parameters & expressions |
| Which curve types exist and what confines a point to them? | Geometry coverage |
| Is rank/DOF analysis scale-invariant? | Solver evolution |
| What diagnostics exist, what is still missing? | Constraint diagnostics & UX |
| What annotation work is deferred? | Constraint/dimension visualization |
| How far does unit-kind algebra go? | Units |
| What does removal cascade? | Entity/constraint removal |
| What is out of scope for the 2D layer? | 2D → 3D |

Navigation only — the sections below are the authority.

## Parameters & expressions

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

## Geometry coverage

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

### Closed (periodic) splines

  **Closed (periodic) splines** are in as a separate `ClosedSpline` entity
  (`CreateClosedSpline`, ≥3 control points) over an exact cyclic uniform cubic basis
  (`geom.EvalPeriodicCubicBSpline`) — a smooth C2 loop that bounds a region on its
  own (a sealed `geom.ClosedCurve`, not a `Curve`), with periodic-ring
  self-crossing detection and `closed_spline` serialization. **The modulo-1
  reduction has ONE owner, `periodicSpan`**, which every periodic evaluator reads
  its span index and span-local parameter from, and it CLAMPS that index into
  `[0, n)`: the upper bound is the floating-point seam (a tiny negative `t` reduces
  to exactly 1), the lower bound is what makes a parameter the reduction cannot
  place — a NaN, or an infinity, whose reduction is a NaN — an in-range index
  instead of an out-of-range one. Since the span-local parameter is NaN whatever
  index such a `t` lands on, the evaluators answer NaN, the answer the open
  `Spline`, the `FitSpline`, the `NURBS` and the `Conic` all give a NaN parameter,
  and the one a public `Eval` with no error return can give at all. A point can be
  confined to it with `NewPointOnClosedSpline`: the **periodic witness** — a single
  foot parameter `t` aux variable with NO `[0,1]` box (a loop has no endpoints, so
  `t` is unbounded and `S(t)=S(t+1)`), committed residual just the two length
  membership rows `P−S(t)`. A line can be made tangent to it with
  `NewTangentToClosedSpline` — the same periodic witness (unbounded `t` plus a
  no-cusp slack `ws`, no box), three rows: contact on the line's carrier (length),
  parallel to the analytic periodic tangent `S'(t)`
  (`geom.EvalPeriodicCubicBSplineDeriv`, dimensionless), and the no-cusp guard.

### Fit-point (interpolating) splines

  **Fit-point (interpolating) splines** are in as a separate `FitSpline` entity
  (`CreateFitSpline`, ≥2 fit points) whose curve passes *through* the fit points: the
  fit points are the durable solver handles and a natural-cubic interpolant
  (chord-length parameterization, Thomas tridiagonal solve in
  `geom.EvalFitSpline`/`SampleFitSpline`) is recomputed from their current
  coordinates per evaluation, so the curve keeps interpolating them as the solver
  moves them — no new solver vars. An open `Curve` (endpoints = first/last fit
  point) participating in profiles like the open spline, `fit_spline` serialization.
  The built interpolant is **exported** (`geom.FitInterpolant` via
  `geom.FitSpline.Interpolant`/`sketch.FitSpline.Interpolant`/`geom.NewFitInterpolant`;
  active points + cumulative chord parameters + natural-cubic second derivatives,
  with `Spans` converting them to per-span monomial cubics), so a consumer that
  integrates or records the EXACT curve reads its defining data instead of chording
  it. The values are COPIED from the built evaluator rather than recomputed, and
  `FitInterpolant.Eval` runs that same evaluator, so the export cannot drift from
  `Eval`; `Spans` is the same cubic summed differently, so it agrees to rounding
  rather than bit for bit. **A span is published in its own NORMALIZED parameter**
  `u = (p − PStart)/h`, evaluated as `X[0] + u·(X[1] + u·(X[2] + u·X[3]))` over
  `u ∈ [0,1]`, and that is what makes "to rounding" a bound RELATIVE to the curve at
  every span width. In the absolute `p − PStart` the higher coefficients carry `1/h`
  and `1/h²`: on a wide span they underflow to zero while the curve stays perfectly
  ordinary, so `Spans` describes a different curve from `Eval` with every coefficient
  finite and nothing to flag it (measured on constructor-built fit points reaching
  `4e307`: an 11.6% disagreement at a span's own midpoint, one term of `−3.57e306`
  dropped from a value of `3.07e307`, and a whole span whose x coefficients are zero
  while `Eval` returns `5e-101`). The conversion multiplies by `h²`, and each product
  is associated as `h·(h·term)`, NEVER as `(h·h)·term` — the bare `h·h` is already
  `+Inf` at those widths while the answer is representable, and it returns as an
  infinite coefficient or, where the term is zero, as a NaN.
  **Its fields are exported, so a hand-built value can
  violate the precondition**, and `Eval`, `EvalDeriv` and `Spans` must then all read
  ONE coherent prefix of it — the unexported `size` helper, whose rule
  `FitInterpolant`'s own doc comment defines: the leading run whose `Params` start at
  0, stay finite and strictly increase, whose coordinates are finite, and **whose
  spans have finite coefficients**. A reader that computed its own bound
  instead would let two exported views of one value describe different curves, and a
  bad span width reaches a consumer as a `+Inf` coefficient or a NaN parameter it
  integrates with no signal. The coefficient clause is not implied by the parameter
  one: a positive, finite and increasing `h` can still be wide enough for `h²·m` to
  overflow (`Params{0, 1e200}` over a nonzero second derivative), and a coefficient
  that is not a number describes no curve at all. **The clause judges the coefficients
  `Spans` PUBLISHES** — `spanFinite` runs that same conversion — so a change to the
  published form cannot leave the gate certifying a form no caller reads.
  **The prefix bounds the DATA a reader reads, never the VALUE it computes from that
  data, and the doc comments say so rather than claiming more**: `Spans` cannot publish
  a coefficient that is not a number, while `Eval` and `EvalDeriv` can still return an
  infinity — a coordinate plus its span's cubic term, `Eval`'s own `(term·h)·h` (1.5x
  the `h²·m₀/2` coefficient the same span publishes, at the middle of a span whose two
  second derivatives are equal), and above all a TANGENT, since `dS/dt = total·dS/dp`
  and the total chord parameter multiplies. **Tightening `size` until no reader can
  return an infinity is NOT open**: the `4e307` zig-zag fixture is CONSTRUCTOR-built,
  its `Eval` and spans are exact, its tangent is out of range over whole stretches, and
  the pre-existing `EvalFitSplineDeriv` returns the same infinity from the same fit
  points — so the value is a fact about the curve, and refusing it would cut a whole
  curve to one point, the truncation `ErrNonFiniteFitInterpolant` exists to prevent
  (`TestFitInterpolantValueOutsideFloatingPointReadsInfinite` pins all four cases).
  **The prefix rule is for HAND-BUILT values and must
  never shorten what the evaluator produced**, so all three constructors export
  through ONE gate that refuses — `geom.ErrNonFiniteFitInterpolant`, hence the
  `(*FitInterpolant, error)` signature on every one of them — any built value the rule
  would shorten. `newFitEvaluator` accumulates chord length in floating point, so
  coordinates that are each finite (two fit points `1e308` apart, reachable through
  `s.CreatePoint`/`s.CreateFitSpline` with every call returning nil) can overflow a
  parameter or leave it not increasing; truncating there publishes a ONE-POINT
  interpolant, no spans and a zero tangent for a curve `FitSpline.Eval` still
  evaluates whole, so a consumer integrating `Spans()` reads zero area with no error
  anywhere.
  A point can be confined to it with `NewPointOnFitSpline` (the bounded foot
  parameter `t∈[0,1]` with a slack box, exactly like `NewPointOnSpline`, since the
  curve has endpoints). A line can be made tangent to it with
  `NewTangentToFitSpline` — the bounded-`t` witness like `NewTangentToSpline` (five
  rows: contact, parallel to the analytic natural-cubic tangent
  `geom.EvalFitSplineDeriv`, two box rows, no-cusp guard). (Both point-on seeds use
  `geom.NearestParamPeriodicCubicBSpline`/`NearestParamFitSpline`; both tangent
  seeds share `seedTangentParam` with the open spline.)

### Ellipses and elliptical arcs

  Ellipses are in
  (center point + rx/ry/rotation vars; `NewPointOnEllipse` uses a
  Sampson-normalized residual — |F|/|∇F| — to stay in length units).
  **Elliptical arcs** are in as a geometry primitive (`CreateEllipticalArc`:
  center + start/end points + rx/ry/rotation vars, two internal on-ellipse
  constraints pinning the endpoints, eccentric-angle sweep, exact-segment area
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

### Conic–conic tangency

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

### Conics

  **Conics** are in as a geometry primitive (`CreateConic(start, apex, end, rho)` —
  a rational quadratic Bézier; `rho ∈ (0,1)` sets fullness, `ρ<0.5` ellipse /
  `ρ=0.5` parabola / `ρ>0.5` hyperbola arc, apex weight `w=ρ/(1−ρ)`). An open
  `Curve` like the elliptical arc: authorable, profile-participating (exact
  whole-curve area via `conicBulgeSpan`), serializable (`"conic"`), exportable
  (native degree-2 rational `SPLINE` with weights `1,w,1`, incl. world-space).
  `rho` is a single free solver var (a free conic is DOF 7). A point can be
  confined to it (`NewPointOnConic` — bounded foot parameter `t∈[0,1]` witness like
  `NewPointOnSpline`) and a line made tangent (`NewTangentToConic` — the five-row
  bounded-`t` witness like `NewTangentToSpline`, using the analytic
  `geom.Conic.EvalDeriv`). *Follow-ups:* a rho dimension, analytic line/conic &
  conic/conic intersections.

### NURBS

  **NURBS** are in as a geometry primitive (`CreateNURBS(degree, control, weights,
  knots)` — a general non-uniform **rational** B-spline of arbitrary degree over a
  clamped/open knot vector; design in `docs/nurbs-design.md`). Control points are
  ordinary sketch points (the durable handles); **knots, weights and degree are
  stored structural data, NOT solver vars** (a free NURBS is DOF `2(n+1)`, mirroring
  `Spline` — promoting weights to dimensionable vars is a follow-up). An open
  `Curve` (clamped endpoints = first/last control point) that participates in
  profiles, with the de-Boor kernel (`geom/nurbs.go`:
  `findSpan`/`basisFuns`/`dersBasisFuns`/`Eval`/`EvalDeriv`, general-degree, new and
  separate from the uniform-cubic `EvalCubicBSpline`), exact/numerically-exact area
  (see the area note), spline-family **self-crossing detection** (a high-degree NURBS
  can loop, unlike the convex-hull-bounded conic), `"nurbs"` serialization, and
  **native DXF `SPLINE`** export (degree + knots + rational weights, incl.
  world-space). It does **not** replace the uniform-cubic `Spline` (the ergonomic
  common case). A point can be confined to it (`NewPointOnNURBS`) and a line made
  tangent (`NewTangentToNURBS`) — the same bounded-`t∈[0,1]` witnesses as the
  spline, working in normalized `t` mapped to the knot domain via `NURBS.Domain()`,
  with the analytic `geom.NURBS.EvalDeriv`. *Follow-ups:* knot-insertion/refinement
  tools, periodic/unclamped NURBS, weight dimensions, analytic NURBS intersections,
  and a possible later re-expression of `Spline` on the NURBS kernel.
  Slots/fillet/chamfer exist as compound builders and `geom` template helpers.

## Solver evolution

- **Solver evolution.** Numerical Jacobian is fine at current scale. **The
  rank/DOF/redundancy/free-point analysis is scale- and unit-invariant**: all
  three dependency mechanisms — `rankAnalysisOf` (DOF/CheckConstraint),
  `conflictAnalysis` (redundancy/conflict attribution), and `movableVars`
  (FreePoints) — run on the **same physically nondimensional Jacobian** `A =
  Drow·J·Dcol` (the `scaledJacobian` builder shared with the conditioning gate),
  with one structural cutoff `rankZeroTol = 1e-9` applied to `A`. Linear dependency
  is exactly column-scale invariant, so the bug was only the numerical threshold
  meeting raw mixed-unit magnitudes; nondimensionalizing fixes it. The structural
  cutoff is DISTINCT from the conditioning trust gate — "structurally
  rank-deficient" (a true null direction) and "full-rank but numerically fragile"
  (a tiny but nonzero singular value) are different questions, so a DOF-0 sketch
  can still be untrustworthy by conditioning. Two near-singularity signals build on
  this. (1) An **advisory** `RankMargin` (`rankAnalysis.margin`): the multiplicative
  distance of the structural rank decision from `rankZeroTol`, now scale-invariant
  (computed on `A`). It does **NOT** gate `Trustworthy()` — it measures the
  STRUCTURAL rank-decision margin (could DOF flip), a coarser, different question
  than the gate. (2) The **scale-invariant conditioning gate** (`conditioning.go`) — the
  measure that DOES gate `Trustworthy()`. It builds a physically nondimensional
  Jacobian `A = Drow·J·Dcol` (length rows ×1/L, length columns ×L, with L the
  bounding-box diagonal and every other row/column ×1) and reports
  `Conditioning = σ_min(A)/σ_max(A)` via a one-sided Jacobi SVD (never `AᵀA`,
  which squares the condition number into fp noise). It is unit- and
  scale-invariant (same value at 1×/1000×/inch, centred for the FD pass so it is
  also translation-invariant), so a dimensionless threshold is a sound pass/fail
  gate: a DOF-0 sketch whose constraint set is near-dependent (e.g. a point pinned
  by two ≈parallel lines) reads untrustworthy. The threshold is **tolerance-derived**
  — `max(1e-6, 4·√tolerance)` — because a slack-encoded inequality at its active
  boundary only resolves its slack to `≈√tolerance` (column norm `2w ≤ 2·√tol`
  bounds `σ_min`), so the gate must sit above that floor or a slack flat-spot slips
  through; a looser `WithTolerance` raises the gate in step. Computed only
  for a DOF-0 candidate (an under-constrained sketch is genuinely singular by its
  free DOF, a separate verdict → `Conditioning` left +Inf). Scaling is by
  *physical kind*, NOT data-dependent column/row equilibration: equilibration
  would hide a near-zero slack/aux column (a real near-rank-loss). Row kinds come
  from a centralized `condRowKinds` table mirroring each constraint's `residual()`
  rows; the only length-kind aux variables are the conic-tangency contact-witness
  coordinates. Design in `docs/conditioning-gate-design.md`. The rank/DOF analysis
  now runs on the same nondimensional `A` (see the scale-invariance note above), so
  DOF/redundancy/free-points are scale-invariant too. Still open: analytic
  Jacobians for speed/accuracy; equation decomposition (solve independent
  constraint clusters separately); a per-sketch solve tolerance (the solver's
  absolute tolerance — not the rank analysis — is what breaks down at extreme
  geometry scales ≳1e6); and better over-constrained diagnostics (identify *which*
  constraints conflict, not just a count).

## Constraint diagnostics & UX

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
  coloring answer). `Sketch.ProbeConfigurations(ctx)` (`probe.go`; design in
  `docs/ambiguity-probe-design.md`) covers the discrete side DOF analysis
  cannot see: a deterministic multi-start probe that searches for the multiple
  configurations a fully-constrained sketch may admit (mirror flips, tangent
  side choices). It is a falsifier — finding ≥2 configurations proves
  ambiguity; finding 1 never certifies uniqueness. `Sketch.Verify(ctx)`
  (`verify.go`) aggregates all of the above into one non-mutating
  `VerificationReport` (solvability, DOF, `Status`, redundant constraints,
  conflict sets, free points, profiles, opt-in ambiguity via `WithProbe`) for
  the headless-oracle use case. Conflict sets are reported via `ConflictSet`
  (`diagnose.go`): for each conflicting constraint, the earlier *independent*
  constraints whose Jacobian rows linearly combine to reproduce the violated
  row — a true set, not just the later duplicate. `RedundantConstraints`,
  `Diagnose` and `Verify` share one `conflictAnalysis` pass so the partition and
  attribution never diverge. Per-entity constrained status is in
  (`Sketch.EntityIsFullyConstrained` — points + intrinsic shape vars). Still open:
  an `AddConstraint` option that auto-rejects, probe-level tolerance/budget
  options, and folding the ellipse rx/ry-swap symmetry into the probe's duplicate
  metric.

## Higher-level interfaces

- **Higher-level interfaces.** A text DSL + CLI, and eventually an interactive
  GUI (e.g. Ebiten), are anticipated layers. They should consume the public API
  only.

## Constraint/dimension visualization

- **Constraint/dimension visualization.** *Largely resolved* (`annotate.go`;
  design in `docs/constraint-visualization-design.md`). Annotated SVG renders
  dimensions, geometric-constraint glyphs, DOF coloring, conflict highlighting, a
  status badge and profile fill (all opt-in, default off); `internal/cmd/genimages`
  regenerates the README gallery heroes with an in-sync test. The renderer is
  **internal** (type-switches the unexported constraint types). A **constraint-level
  public introspection API** is in (`introspect.go`: `ConstraintKind`/`ConstraintRefs`/
  `ConstraintResiduals`/`IsInternal`) plus optional entity/constraint **names** with
  first-match lookup (`names.go`), giving a DSL/GUI/tests a durable, type-free handle
  on constraints. *Open follow-ups:* the richer **annotation-descriptor API**
  (`Sketch.Annotations()` returning typed exported descriptors — kind + referenced
  entity handles + dimension value/unit/driven — extracted from this renderer's data
  model, so a consumer gets the rendering-level data, not just the constraint graph;
  the north-star "programmability over UI" endpoint), an
  **ambiguity/probe overlay** and a **modification-tools before/after** hero (both
  need a shared-bounds multi-state compositor genimages does not yet have — the
  current heroes are single-state), **rich PNG annotation text** (the rasterizer
  has no font), **global dimension-layout / collision avoidance**, and
  path-outlined glyphs for maximal SVG font portability.

## Units

- **Units.** *Resolved (units).* The `units` module provides typed units, a
  unit-carrying `Value`, and a default-units `System`. Sketch dimensions and
  `param` parameters both carry units; the solver stays in base units and all
  conversion is delegated to the library. **Expression kind algebra is in**
  (`param/kind.go`): `param` tracks unit *kind* (whatever kind a parameter's
  declared unit carries — length, angle, mass, …, or dimensionless) through
  expression arithmetic via a static `kindOf` walk — an identifier's kind
  is its declared unit's kind — and rejects incompatible combinations
  (`length+angle`, `length*length` (rejected as a compound kind), `1/length`
  inverse, `sqrt`/trig of a dimensioned value, …) with `param.ErrIncompatibleKind`.
  Addition allows angle/dimensionless mixing (radians are physically
  dimensionless, so `theta + pi/2` is an angle; a length never mixes with a bare
  number), and a parameter's declared unit kind is checked against its
  expression's kind (an angle expression cannot masquerade as a length parameter).
  `Table.EvalValue` returns the kind-carrying value; `Sketch.evalDimension` uses it
  to reject a compound expression that mixes kinds or whose kind ≠ the dimension's
  (not just a direct single-parameter reference); `Verify(ctx)` runs a non-mutating
  parameter-validation pass exposing `ParametersValid`/`ParameterErrors`, which
  gate `Trustworthy()` — so a unit-kind bug hidden in an expression is no longer
  silently blessed. *Limited on purpose:* this is **kind** algebra, not full
  **dimensional** algebra. The `units` module can now represent compound kinds
  (`Area`, `Volume`, …), but `param` deliberately does **not** compose them: a
  dimensioned product/quotient (`length*length`, `1/length`, `length+angle`) is
  *rejected* rather than represented. Parameters exist to drive dimensions
  (lengths / angles) and plane offsets; a compound kind has no consumer in
  sketch, so it stays outside the expression algebra. Revisit only if such a
  consumer (e.g. an area-dimension constraint) is ever added. Custom `SetFunc`
  functions are dimensionless-only (typed custom functions are a follow-up). **The DXF
  exporter honours the display `System`** (`dxf.go`: length fields in the
  display length unit + `$INSUNITS`/`$MEASUREMENT`); SVG/PNG stay unitless raster/
  vector renders by design. *Open follow-ups:*
  should points/coordinates expose unit-carrying accessors. *Note:* the entire read surface
  — coordinate accessors and the measurement queries (`DistanceTo`/`AngleTo`/…)
  — currently returns raw base-unit `float64` (mm/radians), matching the
  solver's currency. Making reads unit-carrying is the deferred all-or-nothing
  decision above; it should be done across the whole surface, not piecemeal.

## Entity/constraint removal

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

## Tolerances

- **Tolerances.** Still a fixed solver tolerance. Per-sketch
  tolerance/precision remains open.

## Persistence stability

- **Persistence stability.** *Partially resolved:* documents carry
  `"version": 1`; legacy (unversioned) documents load, newer-versioned ones
  are rejected. Still open: an actual migration story when version 2 arrives,
  and schema compatibility guarantees.

## 2D → 3D

- **2D → 3D.** *Partially resolved* (`plane.go`/`world.go`/the `r3` module; design in
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
