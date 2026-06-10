# Acceptance test catalog

The tests this module must pass to credibly claim "Fusion-like parametric 2D
sketching." This is the target specification, not the current state: it covers
behavior that exists, behavior that exists but is untested, and behavior that
requires API that has not been built yet.

Status tags:

- **[exists]** — implemented and covered; listed only where it anchors a
  regression contract worth protecting.
- **[gap]** — the API exists today but the behavior has no test.
- **[new]** — requires new API; a proposed signature is given. These tests
  define the *shape* of the future API and should be adjusted (not discarded)
  when the real design lands.
- **[new spec]** — no new API needed, but the test encodes a behavioral
  *promise* (solver feel, robustness) that is currently an implementation
  accident rather than a contract.

Every behavioral test asserts on solved coordinates / residuals / DOF — never
just "it ran" (see CLAUDE.md, "Correctness is observable").

## 1. Constraint completeness (Fusion's constraint palette)

Fusion's palette: coincident, collinear, concentric, midpoint, fix, parallel,
perpendicular, horizontal/vertical, tangent, equal, symmetry, curvature (G2
smooth). One behavioral test per constraint.

| Test | Status | Asserts |
|---|---|---|
| `TestCoincidentMergesBehavior` | [gap] | Two free points + `NewCoincident` → identical coords after solve. |
| `TestParallelLines` | [gap] | Cross product of directions ≈ 0; lines do **not** collapse onto each other (parallel must not become collinear). |
| `TestCollinearLines` | [gap] | All four endpoints on one infinite line; dimensioned lengths preserved. |
| `TestPointOnCircle` | [gap] | `dist(p, center) == r` after solve; point retains one sliding DOF. |
| `TestMidpoint` | [gap] | Point lands at segment midpoint; dragging an endpoint via goal keeps it at midpoint. |
| `TestEqualLines` | [gap] | `NewEqual` directly (today only exercised through `AddPolygon` internals). |
| `TestDiameterDimension` | [gap] | `NewDiameter` drives `2r`; `.Set(d)` + re-solve updates radius. |
| `TestUnfixRestoresFreedom` | [gap] | `Fix` drops DOF; `Unfix` restores it and the solver may move the point again. |
| `TestHorizontalVerticalPoints` | [new] | Fusion allows horizontal/vertical between two **points**, not just on a line. Proposed: `NewHorizontalPoints(p1, p2 *Point)`, `NewVerticalPoints(p1, p2 *Point)`. |
| `TestTangentLineEllipse` | [new] | `NewTangent(line, ellipse)` — open item in CLAUDE.md geometry coverage. |
| `TestTangentSplines` | [new] | G1 tangency at a spline endpoint to a line/arc. |
| `TestCurvatureSmooth` | [new] | G2 "smooth" constraint between spline and arc (Fusion's curvature constraint). |
| `TestPointOnSpline` | [new] | The recorded v2 design (aux-parameter `allocVars` hook, see `docs/spline-design.md`): point constrained to the curve, slides along it under a goal. |

Each [gap] constraint also needs JSON round-trip coverage — see §8's
`TestJSONRoundTripAllConstraintKinds`, which closes the untested
`rebuildConstraint` branches (parallel, collinear, midpoint, pointOnCircle,
diameter) in one table.

## 2. Dimensions (Fusion's dimension tool)

| Test | Status | Asserts |
|---|---|---|
| `TestAlignedVsHorizontalDistance` | [gap] | Pin `NewHorizontalDistance` / `NewVerticalDistance` behaviorally: aligned = 5 with horizontal = 4 → vertical component = 3. |
| `TestArcLengthDimension` | [new] | Fusion dimensions arc length, not just radius. Proposed: `NewArcLength(arc *Arc, v float64) *ArcLength` — drives `sweep·r`. |
| `TestAngleQuadrants` | [new] | Angle dimension resolves to the *selected* quadrant and stays there across edits; the solver must not flip to the supplement on re-solve. Proposed: quadrant selector on `NewAngle` (e.g. `NewAngleAt(l1, l2, deg, Quadrant)`). |
| `TestDimensionToArcTangentEdge` | [new] | Distance from a point/line to a circle's near/far quadrant (Fusion's tangent-edge dimensioning). Proposed: `NewDistancePointCircle(p, c, mode)` with `Nearest`/`Farthest`/`Center`. |
| `TestDrivingToDrivenConversion` | [new] | Flipping an existing dimension `SetDriven(true)` frees a DOF; flipping back re-constrains at the *current measured* value. |
| `TestOverconstrainingDimensionRejected` | [new] | Adding a driving dimension to a fully-constrained sketch fails (or auto-offers driven) — see §4 — never silently accepted. |

## 3. Solver behavior under real-world editing

Where Fusion-likeness lives or dies: users edit a dimension and expect
everything else to stay put as much as possible.

| Test | Status | Asserts |
|---|---|---|
| `TestMinimalMotionOnEdit` | [new spec] | Under-constrained rectangle, edit width 10→12: untouched geometry far away must not drift (max displacement of unrelated points < ε). Promotes the minimum-norm Levenberg step from implementation detail to user-visible promise. |
| `TestNoFlipOnLargeEdit` | [new spec] | Rectangle width edited 10→500 in one step keeps orientation (no mirror/inversion). May require internal stepping; the test pins the contract. |
| `TestNearestSolutionPreserved` | [new spec] | A tangent circle on the left side of a line stays left after a re-solve; the solver picks the solution branch nearest the current state. |
| `TestDragSmoothness` | [gap] | Drag a vertex of a constrained parallelogram through ~50 incremental `WithGoal` targets: every intermediate solve converges, residuals < tol (no mid-drag explosion). |
| `TestScaleInvariance` | [new spec] | The same sketch at 0.01 mm scale and at 10 m scale (mm base units) both converge to proportionally identical solutions. |
| `TestSolverNeverReturnsNaN` | [new spec] | Contradictory constraints (distance 5 AND distance 8 between the same points): `Solve` returns an error; vars stay finite — never NaN/Inf. |
| `TestSolveDeterministic` | [new spec] | Same input → bit-identical output across two runs (no map-order dependence in residual assembly). |
| `TestSolveOptionsRespected` | [gap] | `WithMaxIterations(1)` on a hard problem returns a non-converged error; `WithTolerance` actually changes acceptance. |
| `TestSolvePerformanceEnvelope` | [new spec] | A ~200-entity / ~400-constraint sketch (chained four-bar linkages) solves within a CI-safe budget. Canary for "we now need analytic Jacobians / decomposition" (open question). |

## 4. Constraint diagnostics (Fusion's over-constrained dialog)

Fusion **refuses** to add a constraint that would over-constrain, and names the
conflict. The engine equivalent:

| Test | Status | Asserts |
|---|---|---|
| `TestAddConstraintRejectsOverconstraint` | [new] | Proposed: `s.CheckConstraint(c) error` or `s.AddConstraint(c, sketch.WithReject())`. Adding a redundant/conflicting constraint to a fully-constrained sketch returns `ErrOverconstrained` listing the conflicting set; sketch unchanged on rejection. |
| `TestConflictingVsRedundant` | [new] | Distinguishes the two (open question in CLAUDE.md): duplicate `distance=5, distance=5` → redundant; `distance=5, distance=8` → conflicting. Proposed: `s.Diagnose()` returning `{Redundant, Conflicting []Constraint}`. |
| `TestRemainingDOFAttribution` | [new] | Fusion colors under-constrained geometry blue. Proposed: `s.FreeEntities()` / `s.IsFullyConstrained(ent)` — a rectangle missing one dimension reports exactly the entities that can still move. |
| `TestRedundantConstraints` | [exists] | Keep as the regression anchor for the row↔constraint mapping (must mirror `residuals()` exactly, including the driven skip). |

## 5. Sketch editing tools (the biggest missing layer)

CLAUDE.md notes these are "expressible" via the geom toolkit + `RemoveEntity`
but not built. Fusion users live in trim/offset/mirror. Every tool test asserts
three things: the resulting **geometry** is right, the resulting **constraint
graph** is right (tangencies/coincidences present, DOF as expected), and a
subsequent **dimension edit + Solve** behaves parametrically. The third
assertion is what separates a CAD tool from a drawing program.

| Test | Status | Asserts |
|---|---|---|
| `TestTrimLineAtIntersection` | [new] | `s.Trim(line, nearPoint)` — committed line crossing a circle: trim removes the segment on `nearPoint`'s side, keeps constraints on the surviving portion, splices coincidence at the cut. |
| `TestExtendToNextCurve` | [new] | `s.Extend(line, end)` — endpoint extends to the nearest intersecting curve. |
| `TestBreakAtPoint` | [new] | `s.Break(ent, at)` — one line becomes two sharing a coincident point; dimensions referencing the original resolve sensibly or error explicitly. |
| `TestFilletCommittedCorner` | [new] | `s.FilletCorner(l1, l2, r)` — replaces the shared endpoint, adds the arc with tangent constraints to both lines + a radius dimension; editing `r` and re-solving keeps tangency (parametric fillet, exactly Fusion's behavior). |
| `TestChamferCommittedCorner` | [new] | Same shape with distance dimensions. |
| `TestOffsetChain` | [new] | `s.Offset(entities, d)` — offset a connected chain (lines + arcs) by `d`: the result is a parallel chain bound by an offset **constraint**, so editing the original moves the offset copy (Fusion's offset is constrained, not a frozen copy). Needs a real `OffsetConstraint` — the deepest new solver work in this catalog. |
| `TestMirror` | [new] | `s.Mirror(entities, axisLine)` — mirrored copies created with symmetric constraints; editing the original re-solves the mirror side. |
| `TestRectangularPattern` | [new] | `s.PatternRect(entities, nx, ny, dx, dy)` — instances follow the seed. |
| `TestCircularPattern` | [new] | `s.PatternCircular(entities, center, n)` — equal angular spacing held by constraints; editing the seed propagates. |

## 6. Geometry coverage gaps

| Test | Status | Asserts |
|---|---|---|
| `TestEllipticalArc` | [new] | Open item in CLAUDE.md: `AddEllipticalArc` with start/end angles; profile detection treats it as a boundary curve. |
| `TestFitPointSpline` | [new] | Fusion's default spline interpolates *through* fit points (current implementation is control-point B-spline). Proposed: `geom.NewFitSpline(pts)` — curve passes through every point; constraining a fit point reshapes the curve locally. |
| `TestThreePointRectangle` | [new] | `AddRectangle3Pt` / `AddRectangleCenter` — Fusion's rectangle variants; each carries the right shape-holding constraint set. |
| `TestConstructionGeometry` | [gap→new] | `WithConstruction` exists only as an SVG option; the engine needs a first-class flag: `s.SetConstruction(ent, true)`. Construction entities are excluded from `Profiles()` (claimed in CLAUDE.md — pin it), still participate in constraints (centerline symmetry axis), survive JSON, render dashed in SVG. |
| `TestIntersectionPoint` | [new] | `s.AddIntersectionPoint(e1, e2)` — committed point constrained to remain at the intersection as the sketch re-solves. |

## 7. Profiles (what feeds extrude later)

`Profiles()` returns closed-region boundaries today. Fusion's profile detection
is more demanding:

| Test | Status | Asserts |
|---|---|---|
| `TestProfileNestedRegions` | [new spec] | Circle inside a rectangle → three pickable results: the ring (rect minus circle), the disc, the outer rect. At minimum the engine must report containment (which loops are inside which). Proposed: `Profile.Children()` or `Profiles()` returning a containment tree. |
| `TestProfileSharedEdge` | [new spec] | Two rectangles sharing one edge → two profiles, shared edge in both. |
| `TestProfileSelfIntersecting` | [new spec] | Figure-eight from lines → either two profiles or a defined error, never a garbage loop. |
| `TestProfileOpenChainExcluded` | [gap] | A loop with a 0.1 mm gap is **not** a profile (`geom.Loops` is identity-based, so this should hold — pin it; it also documents *why* coincidence-by-coordinates isn't enough). |
| `TestProfileArea` | [new] | `Profile.Area()` / `Centroid()` with signed orientation — needed the moment profiles feed extrude, and makes profile tests numerically assertable. |
| `TestProfilesUpdateAfterSolve` | [gap] | Profiles recomputed after a dimension edit reflect the new geometry. |

## 8. Parametrics, units, persistence (mostly hardening)

| Test | Status | Asserts |
|---|---|---|
| `TestJSONRoundTripAllConstraintKinds` | [gap] | Table-driven: a sketch containing **every** constraint kind round-trips, re-solves, residuals match. Automatically catches a forgotten marshal case for future constraints. |
| `TestParameterRenamePropagates` | [new] | `t.Rename("width", "w")` updates dependent expressions and sketch bindings, or errors listing dependents — Fusion's parameter dialog does the former. |
| `TestDeleteParameterInUse` | [gap] | Deleting a param referenced by a bound dimension errors with the dependents named (`Table.Delete` exists; the binding interaction is untested). |
| `TestApplyParametersPublic` | [gap] | The exported entry point, called directly. |
| `TestUnbind` | [gap] | Exported, untested. |
| `TestSolveErrorNamesParameter` | [new] | Open follow-up in CLAUDE.md: when a bound expression makes a sketch unsolvable, the error identifies the dimension/parameter. |
| `TestExpressionKindTracking` | [new spec] | `"width + angle"` errors at eval (currently documented as *not* caught — this test encodes the target behavior for the open question). |
| `TestJSONFixedPoint` | [gap] | `marshal(unmarshal(marshal(s)))` byte-identical — cheap, catches id drift. |
| `TestJSONForwardMigration` | [new] | A version-2 fixture with a recorded migration loads as version 1 + migration. Write it the day version 2 exists; the fixture file *is* the test. |
| `TestRoundTripPreservesSolvedState` | [gap] | Load → residuals already < tol without re-solving (solved coordinates, not just structure, are persisted). |

## 9. Export fidelity

Current SVG/DXF tests are substring smoke tests. The minimum bar for a CAD
tool:

| Test | Status | Asserts |
|---|---|---|
| `TestSVGGeometryAccuracy` | [new spec] | Parse the emitted SVG, extract the rectangle's coordinates, compare to solved coordinates within tolerance — not just "contains `<line`". |
| `TestDXFRoundTripThroughReader` | [new spec] | Emitted DXF re-read by a DXF parser (test-only dependency) yields matching entity counts and coordinates — the guarantee that downstream CAM/CAD tools accept the output. |
| `TestSVGOptions` | [gap] | Each SVG option changes output as documented (8 options, zero tests today). |
| `TestExportRespectsDisplayUnits` | [new] | Open question in CLAUDE.md: an imperial sketch exports inch-scaled DXF (`$INSUNITS`) — encode the decision when made. |

## Priority order

1. **§4 + §3** — over-constraint rejection, conflict diagnosis, minimal-motion
   / no-flip / nearest-solution. These define the Fusion *feel* and constrain
   solver evolution before more geometry piles on; cheap to write against the
   current API and expensive to retrofit.
2. **§5 `Offset`** — the single biggest feature gap (needs a real constraint
   type, not just a builder).
3. **[gap] tier in §1, §2, §8, §9** — closes real holes against existing API;
   especially `TestJSONRoundTripAllConstraintKinds`, which covers five
   currently-untested `rebuildConstraint` branches.

When a [new] item gets built, move its test description here to [exists] (or
delete the row if the shipped design diverged and the real tests live in code).
