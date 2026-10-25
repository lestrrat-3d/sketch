# Creation Variants — Design

Status: **implemented**. Closes two open items of `docs/fusion-gap-analysis.md`,
"Geometry primitives": "Arc slot" and "Creation variants" (three-point arc, tangent arc, three-tangent
circle, inscribed and circumscribed polygon, center-to-center versus overall-length slot). Step-by-step
implementation tasks are in `docs/creation-variants-tasks.md`.

## Question router

| Your question | Section |
|---|---|
| Which variants get a builder, and which a recipe? | Decision summary |
| Which choices were made by the maintainer? | Recorded decisions |
| Do the builders create dimensions? | Shared rules for every builder |
| What does builder X create, and what is its DOF? | The section named after the builder |
| What is wrong with shared-endpoint `NewTangentCircles`? | Increment 0 — shared-endpoint arc tangency |
| What does each PR touch, and what proves it done? | Increments |

Navigation only. The sections below are the authority.

## Current state

- `Sketch.CreateArc(center, start, end)` sweeps counter-clockwise from `start` to `end` and adds one internal
  `arcRadius` row (`|End−Center| = |Start−Center|`). An arc owns 6 point coordinates and 1 internal row, so a
  free arc has 5 DOF.
- `Sketch.CreateSlot(x1, y1, x2, y2, r)` (`compound.go`) builds the straight, center-to-center slot: two cap
  arcs, two flanks, four construction spokes held perpendicular to the flanks, and `NewEqualRadius` on the caps.
  It leaves 5 DOF (both centers and the radius) and creates no dimension.
- `Sketch.CreatePolygon(cx, cy, n, r)` builds a regular polygon with circumradius `r`, its first vertex at
  angle 0, held by equal sides and equal construction spokes. It leaves 4 DOF (position, rotation, size) and
  creates no dimension.
- `Sketch.CreateFillet` (`tools.go`) is the only existing builder that computes a tangent arc. It takes the
  arc's exact position from `geom.Fillet`, swaps start and end when the minor arc runs the other way
  (`if arc.Start != c1`), and holds tangency with `NewTangent` at the shared contact point.
- `NewTangentCircles` on two arcs that share an endpoint adds no rank. See "Increment 0".

The Fusion API (queried through the bundled Fusion API reference) exposes `SketchArcs.addByThreePoints(start,
point, end)` with the middle point given as bare coordinates, `SketchCircles.addByThreeTangents(l1, l2, l3,
hintPoint)` that refuses parallel lines and picks one of the candidate circles by the region holding the hint
point, `SketchLines.addScribedPolygon(center, edgeCount, angle, radius, isInscribed)`, and
`Sketch.addCenterToCenterSlot`/`addOverallSlot`/`addCenterPointSlot`. Each slot method creates a dimension
only when asked (`createWidthDimension` defaults to `False`). The API has no arc-slot and no tangent-arc
method; both exist only as UI commands.

## Decision summary

| Variant | Outcome |
|---|---|
| Three-point arc | Builder `CreateArcThreePoints` + math helper `geom.ArcThroughPoints` |
| Tangent arc from a line endpoint | Builder `CreateArcTangent` |
| Three-tangent circle | Builder `CreateCircleThreeTangents` |
| Inscribed / circumscribed polygon | Recipe in an example |
| Center-to-center slot | Already built (`CreateSlot`) |
| Overall-length slot, center-point slot | Recipe in an example |
| Arc slot (center-point form) | Builder `CreateArcSlot` |
| Arc slot (three-point form) | Recipe: `geom.ArcThroughPoints` then `CreateArcSlot` |

The evidence for each choice follows. Every "prototype" figure comes from a throwaway copy of the repository
under the worktree's `.tmp/` in which increments 0 to 5 were implemented exactly as
`docs/creation-variants-tasks.md` specifies; on that copy `go test ./...`, `go vet ./...` and
`golangci-lint run ./...` (v2.12.2) all passed. Figures marked "before increment 0" come from the
unmodified tree.

- **Three-point arc → builder.** `CreateArc` is counter-clockwise only. A caller who passes start and end in
  drawing order gets the complementary arc whenever the middle point lies clockwise, and nothing reports it.
  `CreateFillet` needed the same swap.
- **Tangent arc → builder.** The center sits on the line's normal at the endpoint and on the perpendicular
  bisector of the endpoint and the target, and the turn direction decides the start/end order. Both are easy
  to get wrong by hand.
- **Three-tangent circle → builder.** Three tangent rows have up to four solutions (the incircle and three
  excircles), and the solver keeps whichever one the seed is nearest. On the 12-9-15 triangle, a seed at
  center (9, 9), radius 9 converged to the excircle at (18, 18), radius 18, with every check clean. Only an
  exactly computed seed picks the solution deterministically.
- **Polygon variants → recipe.** Both are `CreatePolygon` plus one existing dimension:
  `NewDistance(center, vertex, R)` for inscribed, `NewDistancePointLine(center, side, a)` for circumscribed.
  The prototype example reached DOF 0 for both.
- **Overall-length and center-point slot → recipe.** Two tip points on the slot axis and on the caps make the
  tip-to-tip length one `NewDistance`, and a midpoint on the axis gives the center-point form. The prototype
  reached DOF 0, and editing the overall length from 40 to 50 moved the tips from ±20 to ±25.
- **Arc slot → builder.** It needs 7 points, 4 arcs, 2 construction lines and 4 tangency constraints, with a
  fixed start/end order per arc.
- **Three-point arc slot → recipe.** Once `geom.ArcThroughPoints` exists, it is two calls.

No builder needs a new constraint kind. Increment 0 changes the residual of an existing kind and leaves its
JSON form unchanged, so the JSON schema, both document versions, `constraintArity` and `constraintRefs` are
unchanged. `condRowKinds` changes for one branch (Increment 0).

## Recorded decisions

The maintainer decided these on 2026-10-25.

1. **Repair shared-endpoint `NewTangentCircles`** in its own PR (increment 0), merged before the builder that
   depends on it (`CreateArcSlot`, increment 5).
2. **Builders create no dimensions.** This matches `CreateRectangle`/`CreatePolygon`/`CreateSlot` and Fusion's
   default (`createWidthDimension=False`).
3. **`CreateArcSlot` takes `r`, the cap radius** (half the slot width), the same parameter `CreateSlot` takes.
4. **`CreateCircleThreeTangents` refuses parallel lines**, as Fusion's `addByThreeTangents` does.

## Shared rules for every builder

- **No builder creates a dimension** (Recorded decisions, item 2). Each section below states the DOF the
  builder leaves and the exact dimension set the tests add to reach DOF 0.
- **No builder calls `Fix`.** Tests and examples ground with `NewCoincident(p, s.Origin())` and one
  orientation constraint, per "Author for parametricity" in `CLAUDE.md`.
- **A refused call changes nothing.** Every builder validates all inputs before its first `CreatePoint`. A
  refusal leaves `len(s.Points())`, `len(s.Entities())` and `len(s.Constraints())` unchanged, and every
  refusal test asserts all three.
- **Handles that come from the sketch are screened.** A builder that takes a `*Line` calls
  `s.foreignInput(...)` (the guard every tool calls, `.claude/docs/sketch-core.md`, "A tool added later MUST
  call the guard explicitly") and returns `ErrForeignEntity` on a nil, removed or foreign line or a line with a
  foreign endpoint. A builder that takes a `*Point` screens it with `s.owns` and returns `ErrForeignEntity`.
  `owns` accepts `s.Origin()`, so an arc may start at the origin.
- **Reference lines are accepted.** `CreateFillet` returns `ErrReferenceGeometry` because it removes its
  legs. None of these builders modifies its input, so none of them refuses a reference line.
- **Non-finite scalars are refused** with `ErrInvalidShape`, through one helper `allFinite`. A NaN passes
  every `<=` and `==` guard as false, so the screen runs first.
- **Tolerances.** Lengths, hint distances, parallelism (the sine between two unit normals) and the sweep are
  compared against `epsTool` (`1e-9`, `tools.go`). `geom.ArcThroughPoints` tests collinearity relative to
  the input: `|cross| <= 1e-9·|end−start|·|mid−start|`.
- **Handles are not serialized.** The created points, entities and constraints are ordinary sketch content,
  and a reload yields the same geometry and constraints without the handle, as with `Slot`. The prototype
  tests marshal and reload every builder's output and get the same entity and constraint counts and DOF 0.
- **The solution branch is held by the seed.** Every tangency here is unsigned or side-only. The builder places
  the geometry exactly on the intended branch, and the local solver stays on it while dimensions change
  continuously. This is the contract `CreateFillet` already has.
- **Code location.** Production code goes in `compound.go` (sugar over the primitive builders, per its
  header comment). `geom.ArcThroughPoints` goes in `geom/modify.go` beside `geom.Fillet`. Each builder's tests
  go in a new external test file (`package sketch_test`, `testify/require`), so increments do not edit the
  same test file.

## Increment 0 — shared-endpoint arc tangency

### Root cause

Before increment 0, two arcs sharing an endpoint and already tangent there (external: centers (0, 0) and
(10, 0), radii 5, shared point (5, 0); internal: centers (0, 0) and (3, 0)) gave `DOF()` = 8 both before and
after `NewTangentCircles(a1, a2, internal)`, and `RedundantConstraints()` reported 1. A solve that relied on
the constraint stopped 2.5·10⁻⁵ (external) and 1.7·10⁻⁵ (internal) away from the tangent position.

The endpoint branch of `tangentCircles.residual` (`constraint.go`) appended only the center-distance row
`|O1O2| − (r1 + r2)` (internal: `|O1O2| − |r1 − r2|`). The shared point X lies on both circles, so the
triangle inequality makes that row non-negative in every configuration the arcs can take, and tangency is its
minimum. The row's gradient at the solution is therefore a combination of the two `arcRadius` rows: rank
analysis sees no new equation, and the solver's step toward tangency shrinks as it gets closer. No existing
test checked DOF on this branch: `TestTangentArcArc` uses no shared point, and
`TestTangentSharedArcsRespectInternal` grounds every point and checks only convergence.

### Corrected residual

Two circles through X are tangent at X exactly when both centers lie on one line through X: on the same side
of X for internal tangency, on opposite sides for external. The endpoint branch now returns one row, the
signed angle wrapped into (−π, π] from `X − O1` to `X − O2` (internal) or to `O2 − X` (external):

```
u1 = X − O1,  u2 = X − O2,  if external: u2 = −u2
row = atan2(cross(u1, u2), dot(u1, u2))
```

The row is zero exactly at tangency on the requested side and ±π on the other side, so the side check that
`TestTangentSharedArcsRespectInternal` pins still fails the wrong side. A zero-length radius or coincident
centers return the constant 1, so a degenerate pair never reads as satisfied. The row is dimensionless, which
matches the angle convention of `NewAngle` and the arc-length coupling row. The arity stays 1, the row reads
only X, O1 and O2 (a subset of the variables the old row read), and no aux variable is involved.

### What else changes

- **`condRowKinds` (`conditioning.go`).** The endpoint branch's row changes from `rowLength` to
  `rowDimensionless`. The interior branch keeps its `rowLength` row and its sweep rows.
- **JSON.** Unchanged: the constraint is still written as `tangent_circles` with its two entity ids and the
  `internal` flag. Documents written before increment 0 load unchanged. A loaded sketch that was solved
  before reaches the same tangent configuration; its reported DOF drops by 1 and its redundancy count by 1 per
  shared-endpoint tangency, which are the corrections.
- **`Sketch.Enclose`.** `tangentCircles` has no case in `certEquationsOf` (`enclose_system.go`), so a sketch
  holding one is refused with `ErrUncertifiedConstraint` before and after the change.
- **Local Jacobian (`jacobian.go`).** The row count per constraint is unchanged and the row reads a subset of
  the operands' variables. The prototype passed the whole suite, including `jacobian_inventory_test.go`.

### Tests (before → after, measured on the prototype)

- `TestTangentCirclesSharedEndpointRank`: DOF 8 → 7 after adding the tangency, `RedundantConstraints()`
  empty, external and internal. Before increment 0 it fails with DOF 8.
- `TestTangentCirclesSharedEndpointSolves`: a free second arc driven onto tangency reaches DOF 0, no
  redundancy, and its center on the line through the first center and the shared point within 1e-6
  (external: center (0, 8), end (3, 8); internal: center (0, 2), end (3, 2)). Before increment 0 the center
  stops 2.5·10⁻⁵ off that line.
- `TestTangentCirclesSharedEndpointCoincidentCenters`: the residual of a pair with coincident centers is 1.
  Before increment 0 it is 5.
- The existing suite passes unchanged on the prototype.

## `CreateArcThreePoints` — three-point arc

```go
// geom/modify.go
func ArcThroughPoints(start, mid, end *Point) (*Arc, bool)

// compound.go
func (s *Sketch) CreateArcThreePoints(start *Point, mx, my float64, end *Point) (*Arc, error)
```

`geom.ArcThroughPoints` returns the counter-clockwise arc through all three points. Its `Center` is a new
point at the circumcenter. `Start`/`End` are the caller's `start`/`end` pointers when `mid` lies on the
counter-clockwise path from `start` to `end`, and swapped otherwise, so a caller detects the swap by pointer,
as `CreateFillet` does with `geom.Fillet`. It returns false when a point is nil or has a non-finite
coordinate, when two points coincide, or when the three are collinear.

`CreateArcThreePoints` mirrors Fusion's `addByThreePoints`. The endpoints are existing points, so the arc can
share them with neighbouring lines. The middle point is coordinates only and is not kept, as in Fusion.

- **Creates:** one point (the center at the circumcenter) and one arc, `CreateArc(center, start, end)` or
  `CreateArc(center, end, start)` depending on the swap. The returned arc's `Start` is therefore `end` when the
  middle point lies clockwise; the doc comment says so.
- **Constraints:** only the arc's internal `arcRadius` row.
- **DOF added:** +1 (center 2 vars, 1 row). With free endpoints the arc has 5 DOF: the four endpoint
  coordinates and the bulge.
- **Fully constrained in the test by:** `NewCoincident(center, s.Origin())`, `NewRadius(arc, 12)`,
  `NewHorizontalPoints(center, start)`, `NewHorizontalPoints(center, end)`. The prototype reached DOF 0, no
  redundancy, start (12, 0), end (−12, 0).
- **Errors:** `ErrForeignEntity` when `start` or `end` fails `owns`. `ErrInvalidShape` when `start == end`,
  when `mx`/`my` is non-finite, or when `geom.ArcThroughPoints` returns false.
- **To keep the through-point:** create a point at `(mx, my)` and add `NewPointOnArc(p, arc)`. The doc comment
  names this. Building it in would add a point and an aux variable that Fusion does not have.

## `CreateArcTangent` — tangent arc from a line endpoint

```go
func (s *Sketch) CreateArcTangent(l *Line, at *Point, x, y float64) (*Arc, error)
```

The arc leaves `at` (which must be `l.Start` or `l.End`) tangent to `l` and continuing in the line's direction
of travel `d = at − other end`, and ends at `(x, y)`.

- **Seed math.** `n` is the left unit normal of `d` and `v = (x, y) − at`. The center is
  `at + k·n` with `k = |v|² / (2·(n·v))`, and the radius is `|k|`. When `cross(d, v) > 0` the arc turns left
  and is `CreateArc(center, at, e)`. Otherwise it turns right and is `CreateArc(center, e, at)`.
- **Creates:** two points (the end `e` at `(x, y)` and the center) and one arc.
- **Constraints:** the internal `arcRadius` row and `NewTangent(l, arc)`. The arc and the line share `at`, so
  `NewTangent` takes its endpoint-tangency branch, one dimensionless row (line ⊥ radius at `at`) with no aux
  variable. That branch does not have increment 0's defect.
- **DOF added:** +2 (4 vars, 2 rows), the end point's position. On a line grounded by
  `NewCoincident(p0, s.Origin())`, `NewHorizontal(l)` and `NewDistance(p0, p1, 10)`, adding
  `NewRadius(arc, 8)` and `NewHorizontalPoints(center, e)` reached DOF 0 in the prototype for both turn
  directions.
- **Errors:** `ErrForeignEntity` from `foreignInput(l)`. `ErrInvalidShape` when `at` is neither endpoint of
  `l`, when `l` has zero length, when `x`/`y` is non-finite, or when `cross(d, v)` is zero. That last case
  covers `(x, y) == at` and every target on the line's carrier, where no circle is tangent at `at` and passes
  through the target.
- **Known limit.** The endpoint-tangency row is satisfied by both the smooth continuation and the arc that
  doubles back over the line (a cusp). The seed is the smooth one, and the test asserts that after a dimension
  edit the arc still leaves `at` along `d`.
- **Follow-up, not in this plan: tangent arc from an arc endpoint.** It would use `NewTangentCircles` with a
  shared endpoint, which increment 0 makes usable.

## `CreateCircleThreeTangents` — three-tangent circle

```go
func (s *Sketch) CreateCircleThreeTangents(l1, l2, l3 *Line, hx, hy float64) (*Circle, error)
```

The circle is tangent to the three lines' infinite carriers. That is the meaning of `NewTangent` for a circle
operand, and the meaning Fusion documents for `addByThreeTangents`. The hint `(hx, hy)` picks the region the
circle is created in.

- **Seed math.** Each line gets a unit left normal `n_i` and offset `d_i = n_i·Start_i`. The hint's side of
  each line is `σ_i = sign(n_i·h − d_i)`. The builder solves the 3×3 linear system `n_i·c − σ_i·ρ = d_i` for
  the center `c` and `ρ` by Cramer's rule. A hint inside the triangle gives the incircle, and a hint across
  one side gives that side's excircle. A hint in a region across a vertex gives `ρ < 0`, because no tangent
  circle lies there.
- **Creates:** one point (the center) and one circle of radius `ρ`.
- **Constraints:** `NewTangent(l1, c)`, `NewTangent(l2, c)`, `NewTangent(l3, c)`. Each is one length row:
  the absolute center-to-carrier distance minus the radius.
- **DOF added: 0** (3 vars, 3 rows). On the triangle A(0, 0), B(12, 0), C(0, 9), grounded by
  `NewCoincident(A, s.Origin())`, `NewHorizontal(AB)`, `NewVertical(CA)`, `NewDistance(A, B, 12)` and
  `NewDistance(A, C, 9)`, the prototype reached DOF 0, no redundancy, two profiles and a clean `Check()`, with
  the incircle at (3, 3), r = 3. Editing AB to 16 moved it to (3.3212, 3.3212), r = 3.3212, which matches
  `(16 + 9 − √337)/2`. Hint (10, 10) gives the excircle at (18, 18), r = 18.
- **Errors:** `ErrForeignEntity` from `foreignInput(l1, l2, l3)`. `ErrInvalidShape` when two arguments are the
  same line, when a line has zero length, when `hx`/`hy` is non-finite, when the hint lies on a carrier, when
  two lines are parallel (Recorded decisions, item 4), when the system is singular, or when `ρ <= epsTool`.
  The last case covers three concurrent lines (ρ = 0) and a hint across a vertex (ρ < 0).

## `CreateArcSlot` — arc slot

```go
// ArcSlot groups the geometry created by [Sketch.CreateArcSlot].
type ArcSlot struct {
	Center       *Point  // center of the slot's centerline arc
	C1, C2       *Point  // cap centers: the centerline's start and end
	Outer, Inner *Arc    // concentric flanks about Center
	A1, A2       *Arc    // semicircular end caps about C1 and C2
	Spokes       []*Line // construction lines Center→C1 and Center→C2, for dimensioning
}

func (s *Sketch) CreateArcSlot(cx, cy, x1, y1, x2, y2, r float64) (*ArcSlot, error)
```

The centerline runs counter-clockwise about `(cx, cy)` from `(x1, y1)` toward the direction of `(x2, y2)`.
Like `CreateArc`, the second point sets only the end direction, and the centerline radius is
`R = |(x1, y1) − (cx, cy)|`. `r` is the cap radius (Recorded decisions, item 3). A clockwise slot is built by
swapping the two points.

- **Creates (7 points):** `Center`; `C1` at angle θ1 and `C2` at angle θ2 on radius `R`; contacts
  `P1o, P1i` at radii `R + r` and `R − r` on the ray through `C1`, and `P2o, P2i` on the ray through `C2`.
- **Creates (4 arcs, 2 lines):**

  | Entity | Call | Why this order |
  |---|---|---|
  | `Outer` | `CreateArc(Center, P1o, P2o)` | Counter-clockwise from θ1 to θ2. |
  | `Inner` | `CreateArc(Center, P1i, P2i)` | Same. |
  | `A1` | `CreateArc(C1, P1i, P1o)` | The half-turn passes θ1 − 90°, away from the slot body. |
  | `A2` | `CreateArc(C2, P2o, P2i)` | The half-turn passes θ2 + 90°, away from the slot body. |
  | `Spokes[0]` | `CreateLine(Center, C1)`, construction | Dimension handle for the sweep and radius. |
  | `Spokes[1]` | `CreateLine(Center, C2)`, construction | Dimension handle for the sweep. |

  The outline `P1o → Outer → P2o → A2 → P2i → Inner (reversed) → P1i → A1 → P1o` is one closed loop.
- **Constraints:** the four internal `arcRadius` rows, plus `NewTangentCircles(A1, Outer, true)`,
  `NewTangentCircles(A1, Inner, false)`, `NewTangentCircles(A2, Outer, true)` and
  `NewTangentCircles(A2, Inner, false)`. Each pair shares an endpoint, so each tangency is the corrected
  one-row endpoint branch from increment 0. The spokes carry no constraint.
- **Why that is the arc slot.** Each cap is tangent to `Outer` at `P1o` (or `P2o`) and to `Inner` at `P1i`
  (or `P2i`), so both of its contacts lie on the line through `Center` and its own center. The cap's
  `arcRadius` row makes the two contacts symmetric about the cap center, and `Outer`'s and `Inner`'s
  `arcRadius` rows then force `|Center C2| = |Center C1|` and equal cap radii, so neither needs its own
  constraint.
- **DOF left: 6** (14 vars, 8 rows): center (2), centerline radius, start angle, sweep, cap radius. The
  prototype measured 6 with no redundancy at sweeps of 30°, 90° and 270°. Before increment 0 the same
  construction measured DOF 10 with 4 redundant constraints.
- **Fully constrained in the test by:** `NewCoincident(Center, s.Origin())`, `NewHorizontal(Spokes[0])`,
  `NewAngle(Spokes[0], Spokes[1], sweep)`, `NewDistance(Center, C1, R)`, `NewRadius(A1, r)`. The prototype
  reached DOF 0, a clean `Check()`, one profile whose area matched `sweep·R·2r + πr²` to 1e-4 (216.7699 at
  R = 20, r = 3, 90°), and a JSON reload at DOF 0. Editing R to 25 and r to 4 moved `C2` to R = 25 on the
  same ray with `Outer.R() = 29` and `Inner.R() = 21`. Adding 20° to the angle dimension added 20° to the
  outer sweep.
- **Errors:** `ErrInvalidShape` when any argument is non-finite, when `(x1, y1)` or `(x2, y2)` lies within
  `epsTool` of the center, when `r <= 0` or `r >= R` (the inner flank would have no radius), when the computed
  sweep is within `epsTool` of 0 or of a full turn, or when the sweep exceeds 180° and `R·sin(sweep/2) <= r`
  (the two caps overlap across the gap, and the outline crosses itself).
- **After creation** a dimension edit can still drive the caps into overlap. `Profiles()` then reports the
  region `Valid = false`, which `Trustworthy()` already reads.
- **Three-point arc slot (recipe):** `g, ok := geom.ArcThroughPoints(p1, pm, p2)`, then
  `s.CreateArcSlot(g.Center.X, g.Center.Y, g.Start.X, g.Start.Y, g.End.X, g.End.Y, r)`.

## Recipes: polygon and slot variants

One example file shows the variants that need no new API. The values are the example's verified output.

- **Inscribed polygon** (Fusion `isInscribed=true`; the angle sets a corner):
  `CreatePolygon(0, 0, 6, 10)`, `NewCoincident(p.Center, s.Origin())`,
  `NewHorizontalPoints(p.Center, p.Vertices[0])`, `NewDistance(p.Center, p.Vertices[0], 10)`. Result:
  DOF 0, first vertex (10, 0), side 10.
- **Circumscribed polygon** (Fusion `isInscribed=false`): seed with circumradius `a / cos(π/n)`, then replace
  the vertex dimension with `NewDistancePointLine(p.Center, p.Sides[0], a)`. With a = 10 the result is
  DOF 0, first vertex (11.5470, 0), side 11.5470.
- **Overall-length slot:** `CreateSlot`, a construction axis line `C1 → C2`, tip points `T1`, `T2` with
  `NewPointOnLine(Ti, axis)` and `NewPointOnArc(Ti, Ai)`, then `NewDistance(T1, T2, L)`. Each cap sweeps only
  the half away from the slot, so the sweep confinement of `NewPointOnArc` admits only the far tip. The tips
  add no DOF (the slot keeps 5). With the axis midpoint on the origin, a horizontal axis, `L = 40` and the
  width dimension `NewDistance(C1, L1.Start, 5)`, the result is DOF 0 with tips at ±20. Editing `L` to 50
  moves the tips to ±25 and the centers to ±20.
- **Center-point slot:** the `NewMidpoint(mid, axis)` plus `NewCoincident(mid, s.Origin())` step above.
- The example's comment names the alternative without tip points: bind `NewDistance(C1, C2)` to the
  expression `L − W` and the width dimension to `W / 2` through the parameter table.

## Tests every builder ships

Each builder's test file contains, at minimum:

1. **Exact placement.** The returned geometry matches the analytic seed to 1e-9 before any solve
   (center, radius, `Start`/`End` identity, sweep).
2. **DOF.** `s.DOF()` after the builder equals the "DOF added" or "DOF left" figure above.
3. **Full constraint and reflow.** Ground through `s.Origin()` (never `Fix`), add the dimension set above,
   then assert `res.DOF == 0`, `s.RedundantConstraints()` empty and solved coordinates to 1e-6. Edit one
   dimension, re-solve, and assert the moved coordinates.
4. **Verify.** `s.Verify(ctx).Check()` is nil on the fully constrained sketch. For closed outlines (arc slot,
   three-tangent circle in its triangle) assert the profile count, and the area for the arc slot.
5. **Refusals.** One table row per error listed for the builder. Each asserts `require.ErrorIs`, a nil
   handle, and unchanged point, entity and constraint counts. Foreign handles come from the existing
   `foreignLine` (`tools_ownership_test.go`) and `foreignPoint` (`grounding_ownership_test.go`) helpers.
6. **JSON round trip.** Marshal, unmarshal into a fresh `Sketch`, then assert equal entity and constraint
   counts and DOF 0 after solving, as `TestJSONRoundTripSlot` does.
7. **Creation-path invariant.** One call in the "compound builders" subtest of `TestEntityUIDInvariant`
   (`revision_internal_test.go`), which asserts every creation path stamps its entities. It is the repo's one
   `package sketch` test file.

Each increment also adds one example, `examples/sketch_<topic>_example_test.go` in `package examples_test`,
whose `// Output:` block prints the solved values above. `README.md` and the two `docs/guide` pages include
only examples that already exist, so these examples are not embedded. Each PR still runs the CI "generated
docs are in sync" command and confirms it produces no diff.

## Increments

Each increment is one PR, created from `origin/main` after the previous one in the merge order has merged.
Step-by-step instructions are in `docs/creation-variants-tasks.md`.

| # | Increment | Depends on |
|---|---|---|
| 0 | Repair shared-endpoint `NewTangentCircles` | — |
| 1 | Shape recipes example (polygon, slot) | — |
| 2 | `geom.ArcThroughPoints` and `CreateArcThreePoints` | — |
| 3 | `CreateArcTangent` | 2 (`allFinite`, the `geom` import in `compound.go`) |
| 4 | `CreateCircleThreeTangents` | 2 (`allFinite`) |
| 5 | `ArcSlot` and `CreateArcSlot` | 0 (tangency rank), 2 (`allFinite`, `geom.ArcThroughPoints` in the example) |

Merge order: 0, 1, 2, 3, 4, 5. Increments 0, 1 and 2 have no dependency on each other. They are still merged
in order because each later increment edits the same lines of `CLAUDE.md` and
`docs/fusion-gap-analysis.md`, and the tasks file gives those edits against the text the previous increment
leaves.

## Follow-ups outside this plan

- A tangent arc from an arc endpoint (`CreateArcTangent` taking a `Circular`), using the repaired
  shared-endpoint `NewTangentCircles`.
- The conic shared-endpoint tangency branch (`tangentConics`) was not measured for the same defect.
