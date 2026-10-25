# Creation Variants — Design

Status: **proposed**. Closes the "creation variants" gap against the Fusion sketch environment
(`docs/fusion-gap-analysis.md`, "Geometry primitives" → "Slot" and "Rectangle / polygon constructors"):
three-point arc, tangent arc, three-tangent circle, inscribed and circumscribed polygon, arc slot, and
center-to-center versus overall-length slot.

## Question router

| Your question | Section |
|---|---|
| Which variants get a builder, and which a recipe? | Decision summary |
| Do the builders create dimensions? | Shared rules for every builder |
| What does builder X create, and what is its DOF? | The section named after the builder |
| Why does the arc slot not use `NewTangentCircles`? | Shared-endpoint arc tangency adds no rank |
| What does each PR touch, and what proves it done? | Increments |
| What still needs a human decision? | Open questions |

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

The evidence for each choice follows. Every "scratch run" figure comes from a throwaway module under the
worktree's `.tmp/` that built each variant through the public API; it is not committed.

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
  The scratch run reached DOF 0, no redundancy, a clean `Check()` and a JSON reload at DOF 0 for both.
- **Overall-length and center-point slot → recipe.** Two tip points on the slot axis and on the caps make the
  tip-to-tip length one `NewDistance`, and a midpoint on the axis gives the center-point form. The scratch run
  reached DOF 0 with no redundancy, and editing the overall length from 40 to 50 moved the tips to ±25.
- **Arc slot → builder.** It needs 7 points, 4 arcs, 2 construction lines and 4 holding constraints, with a
  fixed start/end order per arc. The obvious formulation with `NewTangentCircles` is wrong (see
  "Shared-endpoint arc tangency adds no rank").
- **Three-point arc slot → recipe.** Once `geom.ArcThroughPoints` exists, it is two calls.

No variant needs a new constraint kind. Every builder emits only existing entity kinds and constraint
kinds, so the JSON schema, both document versions, `constraintArity`, `constraintRefs`, `condRowKinds` and
`certEquationsOf` are unchanged.

## Shared rules for every builder

- **No builder creates a dimension.** This matches `CreateRectangle`/`CreatePolygon`/`CreateSlot` and
  Fusion's default (`createWidthDimension=False`). Each section below states the DOF the builder leaves and
  the exact dimension set the tests add to reach DOF 0.
- **No builder calls `Fix`.** Tests and examples ground with `NewCoincident(p, s.Origin())` and one
  orientation constraint, per "Author for parametricity" in `CLAUDE.md`.
- **A refused call changes nothing.** Every builder computes and validates all coordinates before its first
  `CreatePoint`. A refusal leaves `len(s.Points())`, `len(s.Entities())` and `len(s.Constraints())` unchanged,
  and every refusal test asserts all three.
- **Handles that come from the sketch are screened.** A builder that takes a `*Line` calls
  `s.foreignInput(...)` (the guard every tool calls, `.claude/docs/sketch-core.md`, "A tool added later MUST
  call the guard explicitly") and returns `ErrForeignEntity` on a nil, removed or foreign line or a line with a
  foreign endpoint. A builder that takes a `*Point` screens it with `s.owns` and returns `ErrForeignEntity`.
  `owns` accepts `s.Origin()`, so an arc may start at the origin.
- **Reference lines are accepted.** `CreateFillet` returns `ErrReferenceGeometry` because it removes its
  legs. None of these builders modifies its input, so none of them refuses a reference line.
- **Non-finite scalars are refused** with `ErrInvalidShape`. A NaN passes every `<=` and `==` guard as false,
  so the screen runs first.
- **Tolerances are relative.** "Zero" below means a magnitude at or below `1e-9` times the input's own
  length scale, the same magnitude as `epsTool` (`tools.go`) and `geom`'s `epsIntersect`.
- **Handles are not serialized.** The created points, entities and constraints are ordinary sketch content,
  and a reload yields the same geometry and constraints without the handle, as with `Slot`. The scratch run
  marshalled and reloaded every builder's output and got the same entity and constraint counts and DOF 0.
- **The solution branch is held by the seed.** Every tangency here is unsigned. The builder places the
  geometry exactly on the intended branch, and the local solver stays on it while dimensions change
  continuously. This is the contract `CreateFillet` already has.
- **Code location.** Production code goes in `compound.go` (sugar over the primitive builders, per its
  header comment). `geom.ArcThroughPoints` goes in `geom/modify.go` beside `geom.Fillet`. Each builder's tests
  go in a new external test file (`package sketch_test`, `testify/require`), so increments do not edit the
  same test file.

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
as `CreateFillet` does with `geom.Fillet`. It returns false when a coordinate is non-finite, when two points
coincide, or when the three are collinear (`|cross(end−start, mid−start)| <= 1e-9·|end−start|·|mid−start|`).

`CreateArcThreePoints` mirrors Fusion's `addByThreePoints`. The endpoints are existing points, so the arc can
share them with neighbouring lines. The middle point is coordinates only and is not kept, as in Fusion.

- **Creates:** one point (the center at the circumcenter) and one arc, `CreateArc(center, start, end)` or
  `CreateArc(center, end, start)` depending on the swap. The returned arc's `Start` is therefore `end` when the
  middle point lies clockwise; the doc comment says so.
- **Constraints:** only the arc's internal `arcRadius` row.
- **DOF added:** +1 (center 2 vars, 1 row). With free endpoints the arc has 5 DOF: the four endpoint
  coordinates and the bulge.
- **Fully constrained in the test by:** `NewCoincident(center, s.Origin())`, `NewRadius(arc, 12)`,
  `NewHorizontalPoints(center, start)`, `NewHorizontalPoints(center, end)`. The scratch run reached DOF 0, no
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
  variable.
- **DOF added:** +2 (4 vars, 2 rows), the end point's position. A scratch run of the same construction
  measured 6 DOF for a free line plus the arc, and DOF 0 after
  `NewCoincident(p0, s.Origin())`, `NewHorizontal(l)`, `NewDistance(p0, p1, 10)`, `NewRadius(arc, 8)`,
  `NewHorizontalPoints(center, e)`, for both turn directions.
- **Errors:** `ErrForeignEntity` from `foreignInput(l)`. `ErrInvalidShape` when `at` is neither endpoint of
  `l`, when `l` has zero length, when `x`/`y` is non-finite, or when `cross(d, v)` is zero. That last case
  covers `(x, y) == at` and every target on the line's carrier, where no circle is tangent at `at` and passes
  through the target.
- **Known limit.** The endpoint-tangency row is satisfied by both the smooth continuation and the arc that
  doubles back over the line (a cusp). The seed is the smooth one, and the test asserts that after a dimension
  edit the arc's tangent at `at` still has a positive dot product with `d`.
- **Out of scope: tangent arc from an arc endpoint.** It would need `NewTangentCircles` with a shared endpoint,
  which adds no rank today (next section). It is listed under Open questions.

## Shared-endpoint arc tangency adds no rank

A scratch run built two arcs sharing an endpoint, tangent at that point (external: centers (0, 0) and
(10, 0), radii 5, shared point (5, 0); internal: centers (0, 0) and (3, 0)), and added
`NewTangentCircles(a1, a2, internal)`. In both cases `DOF()` was 8 before and 8 after, and
`RedundantConstraints()` reported 1.

The cause is the residual. The endpoint branch of `tangentCircles.residual` (`constraint.go`) returns only
`|O1O2| − (r1 + r2)` (or `− |r1 − r2|`). When the shared point lies on both circles, the triangle inequality
makes that quantity non-negative, so tangency is its minimum, and its gradient at the solution is a
combination of the two `arcRadius` rows. The constraint holds the shape only to second order and adds no rank.
No existing test checks DOF for this branch: `TestTangentArcArc` and `TestTangentSharedArcsRespectInternal`
check radii and convergence only.

The arc slot below does not use that branch. It expresses each cap's tangency as collinearity of the contact
point with the two centers, using `NewPointOnLine` on a construction spoke, the same move `CreateSlot` makes
with its perpendicular spokes. Repairing the branch is a change to an existing constraint's residual and is
out of scope here (Open questions).

## `CreateArcSlot` — arc slot

```go
// ArcSlot groups the geometry created by [Sketch.CreateArcSlot].
type ArcSlot struct {
	Center       *Point  // center of the slot's centerline arc
	C1, C2       *Point  // cap centers: the centerline's start and end
	Outer, Inner *Arc    // concentric flanks about Center
	A1, A2       *Arc    // semicircular end caps about C1 and C2
	Spokes       []*Line // construction Center→C1 and Center→C2, each holding its cap's two contacts
}

func (s *Sketch) CreateArcSlot(cx, cy, x1, y1, x2, y2, r float64) (*ArcSlot, error)
```

The centerline runs counter-clockwise about `(cx, cy)` from `(x1, y1)` to the direction of `(x2, y2)`. Like
`CreateArc`, the second point sets only the end direction, and the centerline radius is
`R = |(x1, y1) − (cx, cy)|`. `r` is the cap radius (half the slot width), the same parameter `CreateSlot`
takes. A clockwise slot is built by swapping the two points.

- **Creates (7 points):** `Center`; `C1` at angle θ1 and `C2` at angle θ2 on radius `R`; contacts
  `P1o, P1i` at radii `R + r` and `R − r` on the ray through `C1`, and `P2o, P2i` on the ray through `C2`.
- **Creates (4 arcs, 2 lines):**

  | Entity | Call | Why this order |
  |---|---|---|
  | `Outer` | `CreateArc(Center, P1o, P2o)` | Counter-clockwise from θ1 to θ2. |
  | `Inner` | `CreateArc(Center, P1i, P2i)` | Same. |
  | `A1` | `CreateArc(C1, P1i, P1o)` | The half-turn passes θ1 − 90°, away from the slot body. |
  | `A2` | `CreateArc(C2, P2o, P2i)` | The half-turn passes θ2 + 90°, away from the slot body. |
  | `Spokes[0]` | `CreateLine(Center, C1)`, construction | Holds `C1`'s collinearity rows and the angle dimension. |
  | `Spokes[1]` | `CreateLine(Center, C2)`, construction | Same for `C2`. |

  The outline `P1o → Outer → P2o → A2 → P2i → Inner (reversed) → P1i → A1 → P1o` is one closed loop.
- **Constraints:** the four internal `arcRadius` rows, plus `NewPointOnLine(P1o, Spokes[0])`,
  `NewPointOnLine(P1i, Spokes[0])`, `NewPointOnLine(P2o, Spokes[1])`, `NewPointOnLine(P2i, Spokes[1])`.
- **Why that is the arc slot.** Both contacts of a cap lie on the line through `Center` and the cap center, so
  the cap circle and the flank circle meet at a point collinear with both centers, which is tangency. The cap's
  `arcRadius` makes `P1o` and `P1i` symmetric about `C1`. `Outer`'s and `Inner`'s `arcRadius` rows force
  `|Center C2| = |Center C1|` and equal cap radii, so neither needs its own constraint.
- **DOF left: 6** (14 vars, 8 rows): center (2), centerline radius, start angle, sweep, cap radius. The scratch
  run measured 6 with no redundancy at sweeps of 30°, 90° and 270°.
- **Fully constrained in the test by:** `NewCoincident(Center, s.Origin())`, `NewHorizontal(Spokes[0])`,
  `NewAngle(Spokes[0], Spokes[1], sweep)`, `NewDistance(Center, C1, R)`, `NewRadius(A1, r)`. The scratch run
  reached DOF 0, a clean `Check()`, one profile whose area matched `sweep·R·2r + πr²` to 1e-4 (216.7699 at
  R = 20, r = 3, 90°), and a JSON reload at DOF 0. Editing R to 25 and r to 4 moved `C2` to (0, 25) with
  `Outer.R() = 29` and `Inner.R() = 21`. Adding 20° to the angle dimension gave a 110° sweep.
- **Errors:** `ErrInvalidShape` when any argument is non-finite, when `R` is zero, when `(x2, y2)` coincides
  with the center, when `r <= 0` or `r >= R` (the inner flank would have no radius), when the computed sweep is
  a full turn (θ2 equals θ1), or when the sweep exceeds 180° and `R·sin(sweep/2) <= r` (the two caps overlap
  across the gap, and the outline crosses itself).
- **After creation** a dimension edit can still drive the caps into overlap. `Profiles()` then reports the
  region `Valid = false`, which `Trustworthy()` already reads.
- **Three-point arc slot (recipe):** `g, ok := geom.ArcThroughPoints(p1, pm, p2)`, then
  `s.CreateArcSlot(g.Center.X, g.Center.Y, g.Start.X, g.Start.Y, g.End.X, g.End.Y, r)`.

## `CreateCircleThreeTangents` — three-tangent circle

```go
func (s *Sketch) CreateCircleThreeTangents(l1, l2, l3 *Line, hx, hy float64) (*Circle, error)
```

The circle is tangent to the three lines' infinite carriers. That is the meaning of `NewTangent` for a circle
operand, and the meaning Fusion documents for `addByThreeTangents`. The hint `(hx, hy)` picks the region the
circle is created in.

- **Seed math.** Each line gets a unit left normal `n_i` and offset `d_i = n_i·Start_i`. The hint's side of
  each line is `σ_i = sign(n_i·h − d_i)`. The builder solves the 3×3 linear system `n_i·c − σ_i·ρ = d_i` for
  the center `c` and `ρ`. A hint inside the triangle gives the incircle, and a hint across one side gives that
  side's excircle. A hint in a region across a vertex gives `ρ < 0`, because no tangent circle lies there.
- **Creates:** one point (the center) and one circle of radius `ρ`.
- **Constraints:** `NewTangent(l1, c)`, `NewTangent(l2, c)`, `NewTangent(l3, c)`. Each is one length row:
  the absolute center-to-carrier distance minus the radius.
- **DOF added: 0** (3 vars, 3 rows). The circle is fully determined by the lines. A scratch run on the triangle
  A(0, 0), B(12, 0), C(0, 9), grounded by `NewCoincident(A, s.Origin())`, `NewHorizontal(AB)`,
  `NewVertical(CA)`, `NewDistance(A, B, 12)` and `NewDistance(A, C, 9)`, reached DOF 0, no redundancy, two
  profiles and a clean `Check()`. Seeds at (3, 3, r 3), (3.5, 2.8, r 2.5), (6, 4, r 0.5) and (1, 1, r 0.5) all
  converged to the incircle at (3, 3), r = 3. Editing AB to 16 moved it to (3.3212, 3.3212), r = 3.3212, which
  matches `(16 + 9 − √337)/2`.
- **Errors:** `ErrForeignEntity` from `foreignInput(l1, l2, l3)`. `ErrInvalidShape` when two arguments are the
  same line, when a line has zero length, when two lines are parallel (`|n_i × n_j|` zero; Fusion refuses this
  input too), when the hint is non-finite or lies on a carrier, when the system is singular, or when `ρ` is
  zero or negative. Zero `ρ` covers three concurrent lines; negative `ρ` covers a hint across a vertex.
- **Parallel pairs** have tangent circles (two parallel lines and a transversal have two), and the scratch run
  showed the three tangent rows stay independent there (DOF 12 for three free lines plus the circle). The
  refusal is a scope choice, listed under Open questions.

## Recipes: polygon and slot variants

One example file shows the variants that need no new API. Every value below comes from the scratch run.

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
  add no DOF (the slot keeps 5). With a midpoint of the axis on the origin, a horizontal axis, `L = 40` and the
  width dimension `NewDistance(C1, L1.Start, 5)`, the result is DOF 0 with tips at ±20. Editing `L` to 50
  moves the tips to ±25.
- **Center-point slot:** the same construction, with `NewMidpoint(mid, axis)` and the dimension measured from
  `mid`. This is the "midpoint of the axis" step above.
- The example's comment names the alternative without tip points: bind `NewDistance(C1, C2)` to the
  expression `L − W` and the width dimension to `W / 2` (verified in the same scratch run).

## Tests every builder ships

Each builder's test file contains, at minimum:

1. **Exact placement.** The returned geometry matches the analytic seed to 1e-9 before any solve
   (center, radius, `Start`/`End` identity, sweep).
2. **DOF.** `s.DOF()` after the builder equals the "DOF added" or "DOF left" figure above.
3. **Full constraint and reflow.** Ground through `s.Origin()` (never `Fix`), add the dimension set above,
   then assert `res.DOF == 0`, `len(s.RedundantConstraints()) == 0` and solved coordinates to 1e-6. Edit one
   dimension, re-solve, and assert the moved coordinates.
4. **Verify.** `s.Verify(ctx).Check()` is nil on the fully constrained sketch. For closed outlines (arc slot,
   three-tangent circle in its triangle) assert the profile count and area.
5. **Refusals.** One subtest per error listed for the builder. Each uses `require.ErrorIs`, a nil handle, and
   unchanged point, entity and constraint counts. Foreign-handle cases use the `foreignLine` helper pattern
   from `tools_ownership_test.go`.
6. **JSON round trip.** Marshal, unmarshal into a fresh `Sketch`, then assert equal entity and constraint
   counts and DOF 0 after solving, as `TestJSONRoundTripSlot` does.
7. **Creation-path invariant.** Add one call to the "compound builders" subtest in `revision_internal_test.go`.
   That test asserts every creation path stamps its entities, and it is the repo's one `package sketch` test
   file.

Each increment also adds one example, `examples/sketch_<topic>_example_test.go` in `package examples_test`,
named `Example_sketch_<topic>`, whose `// Output:` block prints the solved values listed in the builder's
section. `README.md` includes only `examples/sketch_readme_example_test.go`, so these examples are not
embedded. Each PR still runs `go generate ./...` and confirms it produces no diff.

## Increments

Each increment is one PR. They are ordered by dependency and merge one after another; increments 3 and 4 do
not depend on 2 but edit the same `compound.go`, `CLAUDE.md` and gap-analysis lines. Each PR passes
`gofmt`, `go vet` and the repo's lint before push. CI runs the full suite.

Every increment from 2 on also edits `CLAUDE.md` (the `compound.go` row of "Architecture at a glance" and the
`ErrInvalidShape` builder list under "Conventions") and `docs/fusion-gap-analysis.md` (marks its variant
closed).

### Increment 1 — shape recipes example

- **Scope:** the polygon (inscribed, circumscribed) and slot (overall-length, center-point) recipes. No
  production change.
- **Files:** `examples/sketch_shape_recipes_example_test.go` (new, `Example_sketch_polygonScribed` and
  `Example_sketch_slotOverall`); `docs/fusion-gap-analysis.md`.
- **Acceptance:** `go test ./examples -run 'Example_sketch_(polygonScribed|slotOverall)$'` passes with the
  values in "Recipes: polygon and slot variants".

### Increment 2 — `geom.ArcThroughPoints` and `CreateArcThreePoints`

- **Files:** `geom/modify.go`; `geom/toolkit_test.go`; `compound.go`; `arc_three_points_test.go` (new);
  `revision_internal_test.go`; `examples/sketch_arc_three_points_example_test.go` (new).
- **Acceptance:** `go test ./geom -run TestArcThroughPoints` covers the counter-clockwise, swapped,
  collinear, coincident and non-finite cases. `go test . -run TestCreateArcThreePoints` covers all seven test
  kinds above, including `s.Origin()` as `start`.

### Increment 3 — `CreateArcTangent`

- **Files:** `compound.go`; `arc_tangent_test.go` (new); `revision_internal_test.go`;
  `examples/sketch_arc_tangent_example_test.go` (new).
- **Acceptance:** `go test . -run TestCreateArcTangent` covers left and right turns, `at` equal to `l.Start`
  and to `l.End`, the positive-dot continuation check after a radius edit, and every refusal.

### Increment 4 — `CreateCircleThreeTangents`

- **Files:** `compound.go`; `circle_three_tangents_test.go` (new); `revision_internal_test.go`;
  `examples/sketch_circle_three_tangents_example_test.go` (new).
- **Acceptance:** `go test . -run TestCreateCircleThreeTangents`. Hint (2, 2) gives center (3, 3), r = 3,
  which moves to r = 3.3212 at AB = 16. Hint (10, 10) gives center (18, 18), r = 18. Hint (−1, −1) and a
  parallel pair give `ErrInvalidShape`, and every other refusal has its subtest.

### Increment 5 — `ArcSlot` and `CreateArcSlot`

- **Depends on:** increment 2, for `geom.ArcThroughPoints` in the three-point recipe.
- **Files:** `compound.go`; `arc_slot_test.go` (new); `revision_internal_test.go`;
  `examples/sketch_arc_slot_example_test.go` (new, center-point form plus the three-point recipe).
- **Acceptance:** `go test . -run TestCreateArcSlot`. DOF is 6 at 30°, 90° and 270°. After the five
  dimensions, DOF is 0 and the profile area matches `sweep·R·2r + πr²`. The R, r and angle edits move the
  geometry as listed in the builder's section. Every refusal has its subtest, including cap overlap at a 350°
  sweep with R = 20, r = 3.

## Open questions

1. **Repair shared-endpoint `NewTangentCircles`?**
   - *Leave it.* No extra work. Every sketch that uses it today reports one more DOF and one more redundant
     constraint than it has, and a tangent arc from an arc endpoint stays unbuilt.
   - *Repair it in its own PR before any arc-endpoint builder.* Replace the endpoint row with a dimensionless
     collinearity row (normalized cross product of `shared − O1` and `shared − O2`), keeping the
     internal/external side test. DOF and redundancy become correct for every user. The cost is a residual
     change to an existing kind: its `condRowKinds` classification moves from length to dimensionless, and
     the shared-endpoint tests gain DOF and redundancy assertions. The JSON schema does not change.
   - Recommendation: repair it, because the oracle currently reports wrong DOF for valid sketches.
2. **Should builders create dimensions on request?**
   - *No (this design).* The API stays the same size and matches the existing compound builders. A caller
     writes 3 to 5 `AddConstraint` calls to reach DOF 0.
   - *Functional options such as `WithSlotWidth`.* This matches Fusion's optional width, length and angle
     dimensions. It costs a new option group per builder, plus a test per option.
   - Recommendation: no, because no existing builder does it and the option can be added later without
     breaking callers.
3. **Arc-slot size parameter.** `r` (cap radius, as `CreateSlot` takes) keeps the two slot builders
   consistent. `w` (full width, as Fusion takes) matches Fusion argument for argument but makes the two slot
   builders disagree. Recommendation: `r`.
4. **Parallel pairs in `CreateCircleThreeTangents`.** Refusing matches Fusion and keeps the seed a single 3×3
   solve. Supporting them covers two parallel lines plus a transversal (two circles, picked by the hint) at
   the cost of a second seed path and its tests. Recommendation: refuse now, because Fusion refuses it too.
