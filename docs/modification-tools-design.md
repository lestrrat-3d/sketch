# Sketch-Modification Tools — Design

Status: **implemented** (`tools.go`; the new `Offset` constraint in
`constraint.go`; geom math in `geom/transform.go`, plus additions to
`geom/intersect.go` and `geom/modify.go`; tests in `tools_test.go` and
`geom/transform_test.go`). Closes the "sketch-modification tools" gaps in
`docs/fusion-gap-analysis.md` (trim/extend/break, fillet/chamfer, mirror,
patterns, offset) — the convenience tools that `docs/removal-design.md` left
"to ship separately".

## Problem

The engine could build and solve constrained geometry but not **edit committed
geometry**. The `geom` toolkit (`SplitLineAt`/`Fillet`/`Chamfer`,
intersections) only shaped *uncommitted templates*; once geometry was committed
its topology was frozen. Removal (`removal.go`) lifted that freeze but the
user-facing tools were never built. These tools are what make the engine feel
like a sketcher rather than a constraint solver.

## Decision: build-then-replace, math stays in `geom`

Every mutating tool follows one pattern:

1. Read the **current geometry** as a transient `geom` snapshot via each
   entity's `Geometry()` accessor (never stale template coordinates).
2. Compute the replacement with the `geom` math toolkit.
3. Build the new geometry from **sketch points**: reuse the originals'
   surviving `*sketch.Point` handles wherever a vertex must stay shared (so
   neighbouring geometry stays attached), and `s.AddPoint` only the genuinely
   new vertices (split points, fillet contacts). Attach the holding
   constraints, then retire the originals with `RemoveEntity`/`RemovePoint`.

(Note: this doc predates the pivot to the throwaway-geometry model — the tools
once reused `*geom.X` template pointers and relied on `Add…` idempotency;
they now take `*sketch.Point` directly, which is simpler and the reason the
private `solvedLine`/`solvedCircle`/`solvedArc` helpers are gone.)

| Engine (this change) | Deferred to a future UI |
|---|---|
| Topological edit + constraint attachment, driven by coordinates | Pick/hit-testing, hover preview, gesture semantics |
| `(x, y)` pick locations resolved to the nearest point on the entity | Snapping, magnetism |
| Constraints on a *replaced entity* are dropped; constraints on surviving points are kept | Constraint *transfer* (re-homing a horizontal/angle onto the replacement) |

The deliberate limitation: a tool that replaces a line (trim, fillet, chamfer)
drops constraints that referenced *that line entity* (horizontal, angle, …) via
the removal cascade; constraints on its endpoints survive because the points
survive. The returned handle exposes the replacement entities so the caller can
re-apply entity constraints. Constraint transfer is left to the UI layer.

## Naming

`Add…` (returns a non-serialized grouping handle, like `compound.go`) for tools
that **commit new geometry**: `AddFillet`, `AddChamfer`, `AddMirror`,
`AddPatternRect`, `AddPatternCircular`, `AddOffset`. Bare verbs for tools that
**shorten/remove**: `Trim`, `Extend`, `Break`.

## API

```go
// Replace tools (no new constraints): build-then-replace.
func (s *Sketch) Break(e Entity, x, y float64) (Entity, Entity, bool) // line | arc
func (s *Sketch) Trim(l *Line, x, y float64) (*Line, bool)
func (s *Sketch) Extend(l *Line, end *Point) (*Line, bool)

// Corner tools: arc/cut + tangency/coincidence + editable dimension.
func (s *Sketch) AddFillet(l1, l2 *Line, r float64) (*Fillet, error)
func (s *Sketch) AddChamfer(l1, l2 *Line, d float64) (*Chamfer, error)

// Copy tools: copies linked to the seed by constraints.
func (s *Sketch) AddMirror(ents []Entity, axis *Line) *Mirror
func (s *Sketch) AddPatternRect(ents []Entity, nx, ny int, dx, dy float64) *Pattern
func (s *Sketch) AddPatternCircular(ents []Entity, center *Point, n int) *Pattern
func (s *Sketch) AddOffset(ents []Entity, d float64) *OffsetGroup
```

## Mechanics per tool

- **Break** projects `(x, y)` onto the line (`geom.ClosestPointOnLine`) or arc
  (radial onto the circle, sweep-checked), splits with `geom.SplitLineAt`/
  `SplitArcAt` reusing the original endpoints, commits the two halves (they
  share a fresh split vertex by pointer), and removes the original.
- **Trim** collects the line's crossings with every other entity
  (`lineCrossings`), locates the interval around the pick, and replaces the line
  with the portion that keeps the far end. A pick on a portion bounded by
  crossings on *both* sides returns false (it would split — use Break).
- **Extend** intersects the infinite line through `l` with the other entities,
  picks the nearest crossing *beyond* the chosen end, and replaces `l` with the
  lengthened segment.
- **Fillet/Chamfer** find the shared corner point, run `geom.Fillet`/`Chamfer`
  on solved-coordinate copies (the copies share a throwaway corner so the geom
  helper's endpoint-replacement is harmless), then commit the shortened legs +
  arc/cut reusing the far generics and fresh contact points. The arc/cut shares
  its contact points with the legs by pointer (no coincidence constraint
  needed). Fillet adds `NewTangent` on both legs + a `NewDistance(center, T1)`
  radius dimension; Chamfer adds the cut line + two setback `NewDistance`
  dimensions from the far ends. Both remove the originals and the corner point.
- **Mirror** reflects each source point across the axis (`geom.MirrorPoint`),
  links source↔copy with `NewSymmetric`, and (for circles) `NewEqualRadius`;
  arcs are committed start/end-swapped to stay counter-clockwise. Shared source
  vertices map to a single shared copy (per-call `map[*Point]*Point`). Sources
  are untouched.
- **Patterns** translate (`geom.TranslatePoint`) or rotate
  (`geom.RotatePoint`) each source point per cell. Rectangular cells tie every
  copy point to its source with `NewHorizontalDistance`/`NewVerticalDistance`
  (rigid translate); circular cells tie each copy point to its source by a
  construction spoke from `center` constrained `NewEqual` in length and
  `NewAngle` at the cell angle (rigid rotate, the `AddPolygon` spoke idiom). A
  circle copy also gets `NewEqualRadius`. Either way a copy is a rigid image of
  the seed, so the whole field follows when the seed moves.
- **Offset** is the one new solver constraint. `Offset` drives a destination
  line to a **signed** perpendicular distance `d` from the source line's
  infinite line (positive on the left of the source direction). `AddOffset`
  makes one offset line per source segment with one `Offset` each; segments that
  share a source corner share an offset point, which the two constraints pull to
  the offset intersection — so chains mitre. `OffsetGroup.Set(d)` retargets the
  whole offset.

## The `Offset` constraint (new-constraint checklist)

Unlike `DistanceLines` (which re-derives the side each iteration to stay on the
starting side), `Offset` fixes the side via the signed left-normal, so it never
flips and corners resolve deterministically. Residual is two rows (each Dst
endpoint), in **length units** (cross product ÷ source length), per the
normalization invariant. Wired through the full checklist: residual +
`NewOffset` (constraint.go), `marshalConstraint`/`rebuildConstraint` case
(json.go, type `"offset"`), `constraintRefs` case (removal.go), tests.

## Serialization & export

The copy/replace tools add only ordinary points, entities and constraints, all
already serialized; the grouping handles (`Fillet`, `Mirror`, `Pattern`,
`OffsetGroup`, …) are **not** persisted, matching `compound.go`. The only new
on-disk type is the `"offset"` constraint. Exporters need no changes — they
render the committed geometry.

## Interactions

- **Solver/DOF**: replace tools edit `s.cons`/`s.ents` between solves, never
  during one — same contract as removal. Copy tools add fully-determined copies
  (each copy point's freedom is consumed by its linking constraints), so a
  grounded seed yields a grounded field.
- **Removal**: the tools are the first real consumers of
  `RemoveEntity`/`RemovePoint`; `RemovePoint` refusing while an entity still
  uses a corner (e.g. a third line shares it) is a safe no-op, leaving the point.

## Testing plan (implemented)

Every tool asserts the three-part mandate from `docs/acceptance-tests.md`:
resulting **geometry** (`InDelta` 1e-6), resulting **constraint graph** (DOF +
the expected tangent/coincident/symmetric/offset constraints present), and a
**dimension edit + re-Solve** behaving parametrically (radius/setback/spacing/
offset edits; goal-style source motion for mirror/pattern). Plus a JSON
round-trip per tool that commits geometry (counts survive, internals not
doubled). `geom/transform_test.go` covers the pure math.

## Out of scope (recorded)

- **Constraint transfer** onto replacements (UI concern).
- **Single-knob parametric spacing** for rectangular patterns via a shared
  parameter — today each cell carries its own distance dims (rigid to the seed);
  a `param.Table`-bound spacing is a clean follow-up.
- **Offset of arcs/circles** as a chain (concentric offset) — `AddOffset` skips
  non-line entities; a single circle offset is already expressible with
  `NewConcentric` + `NewRadius`.
- **Trim/Break of circles** (needs two break points) and ellipse mirroring.
