# Curve Modification Tools — Design

Status: **proposed**. Moves every sketch-modification tool in `tools.go` (design in
`docs/modification-tools-design.md`) from a `*Sketch` method to a package-level function, makes the existing tools
behave like Fusion's, then extends trim, extend, fillet and offset from lines to arcs and circles. Closes the two open
gap-list items "Offset of arcs and circles" and "Trim, extend, fillet and chamfer on curves"
(`docs/fusion-gap-analysis.md`, "Sketch-modification tools"). The note for the downstream repositories is
`docs/handoff-decad-free-functions.md`.

The rule for every behaviour choice below is the user's: **work like Fusion as much as possible**. Each Fusion claim is
marked **verified** (read in the Fusion API reference through the `fusion:query-api` skill) or **assumed** (not stated
there; the least surprising behaviour was picked and is listed under "Assumptions to verify in Fusion").

| Your question | Section |
|---|---|
| What exists today, and what is missing? | Current state |
| What does Fusion do, and how sure are we? | Fusion behaviour |
| Which extensions ship, and which do not? | Scope decision |
| What do the tools look like as functions? | Increment 0 — tools become package-level functions |
| Who calls the old methods? | Callers of the old methods |
| How do trim, extend, fillet and chamfer keep constraints? | Modify in place, as Fusion does |
| What ties a trimmed end to its cutter? | Pin trimmed and extended ends to the cutter |
| How does offset behave like Fusion? | Offset like Fusion |
| How does each curve tool compute and commit its result? | Trim on arcs and circles, Extend on arcs, Fillet on curves, Offset of arcs and circles |
| Which new constraints are added? | The `PointOnNormal` constraint, The `RadialOffset` constraint |
| What happens to Revision, JSON, reference geometry, foreign handles? | Interactions |
| How is the work split into PRs? | Increments |
| What is still unconfirmed? | Assumptions to verify in Fusion |
| What are the exact implementation steps? | `docs/curve-modify-tools-tasks.md` |

## Current state

Read from `tools.go` on `origin/main` (`c69d5ab`). Every tool is a `*Sketch` method:

| Tool | Signature | Behaviour today |
|---|---|---|
| `Break` | `(s *Sketch) Break(e Entity, x, y float64) (Entity, Entity, bool)` | Splits a `*Line` or an `*Arc`; every other kind returns false. |
| `Trim` | `(s *Sketch) Trim(l *Line, x, y float64) (*Line, bool)` | Line only. Replaces the line; refuses a middle pick and a line with no crossing. |
| `Extend` | `(s *Sketch) Extend(l *Line, end *Point) (*Line, bool)` | Line only. Replaces the line. |
| `CreateFillet` | `(s *Sketch) CreateFillet(l1, l2 *Line, r float64) (*Fillet, error)` | Two lines sharing a corner `*Point`. Replaces both legs. |
| `CreateChamfer` | `(s *Sketch) CreateChamfer(l1, l2 *Line, d float64) (*Chamfer, error)` | Two lines sharing a corner `*Point`. Replaces both legs. |
| `CreateMirror` | `(s *Sketch) CreateMirror(ents []Entity, axis *Line) *Mirror` | Copies lines, circles, arcs, fit splines; nil on refusal. |
| `CreatePatternRect` | `(s *Sketch) CreatePatternRect(ents []Entity, nx, ny int, dx, dy float64) (*Pattern, error)` | Same four kinds. |
| `CreatePatternCircular` | `(s *Sketch) CreatePatternCircular(ents []Entity, center *Point, n int) (*Pattern, error)` | Same four kinds. |
| `CreatePatternCircularRing` | `(s *Sketch) CreatePatternCircularRing(ents []Entity, center *Point, n int, start, end *Point) (*Pattern, error)` | Same four kinds. |
| `CreateOffset` | `(s *Sketch) CreateOffset(ents []Entity, d float64) (*OffsetGroup, error)` | Lines only, others skipped. Positive `d` is left of each line's own direction. Chain ends slide. |

Replacing an entity drops every constraint that named it (the removal cascade), so a `Horizontal` on a trimmed line
or a `Radius` on a filleted arc disappears. `Trim` and `Extend` also leave the replaced endpoint in `s.points`, where
`FreePoints` reports it and `DOF` counts its two variables.

The `geom` package already has the intersection math the curve tools need: `LineCircleIntersections`,
`CircleCircleIntersections`, `LineArcIntersections`, `CircleArcIntersections`, `ArcArcIntersections`,
`(*Arc).Contains`, `SplitArcAt` and `SplitCircleAt`. It has no helper that returns where a point sits along an arc's
sweep, and no fillet math for anything but two lines (`geom.Fillet`, which also rewrites its input lines' endpoints).

## Fusion behaviour

| # | Behaviour | Status | Source |
|---|---|---|---|
| F1 | Trimming the start or end side of a line or open arc modifies the original entity and returns it. | Verified | `SketchCurve.trim` |
| F2 | Trimming the middle of a line or open arc deletes the original and returns two new entities. | Verified | `SketchCurve.trim` |
| F3 | Trimming a closed curve deletes the original and returns a new curve (a circle becomes an arc). | Verified | `SketchCurve.trim` |
| F4 | Trimming a curve with no intersections deletes it and returns an empty collection. | Verified | `SketchCurve.trim` |
| F5 | Trim and extend create constraints by default (`createConstraints: bool = True`). | Verified | `SketchCurve.trim`, `SketchCurve.extend` |
| F6 | The constraint F5 creates ties the new end point to the cutting curve (point on curve). | Assumed | The reference does not name the constraint. |
| F7 | Extending modifies and returns the original curve. | Verified | `SketchCurve.extend` |
| F8 | An extend that joins two curves deletes both and returns one; an arc extended into a full turn becomes a circle. | Verified | `SketchCurve.extend` |
| F9 | Sketch fillet takes any two open curves. | Verified | `SketchArcs.addFillet` |
| F10a | Chamfer shortens the input lines in place and returns only the chamfer line. | Verified ("the other end will be trimmed by the chamfer") | `SketchLines.addDistanceChamfer` |
| F10b | Fillet shortens the input curves in place and returns only the fillet arc. | Inferred: the call returns only the `SketchArc`, so the inputs survive as the legs | `SketchArcs.addFillet` |
| F11 | Sketch chamfer takes two lines only. | Verified | `SketchLines.addDistanceChamfer`, `addAngleChamfer` |
| F12 | Offset side follows the flow of the curve list, from curve 1 to curve 2. Positive is to the right, negative to the left. A circle flows counter-clockwise, so positive is outside. | Verified | `OffsetConstraintInput.offset` |
| F13 | With `isTopologyMatched` (default true), an offset that would change topology (an inward offset larger than an arc radius) fails. | Verified | `OffsetConstraintInput.isTopologyMatched` |
| F14 | One `OffsetConstraint` with one dimension drives the whole offset. | Verified | `OffsetConstraint`, `SketchOffsetCurvesDimension` |
| F15 | The offset curves are fully determined by the parents and the distance, ends included. | Assumed | `OffsetConstraint` exposes parents, children and one distance, and no per-end constraint. |
| F16 | Constraints on a curve F2/F3 delete are dropped, except radius, diameter, concentric and equal radius on a trimmed circle, which move to the arc. | Assumed | Not stated in the reference. |
| F17 | A tangency between two curves survives a trim that moves the touching end away; it becomes tangency anywhere on the curves. | Assumed | Fusion's tangent constraint relates two curves, not a point. |
| F18 | A single open curve flows from its start to its end. | Assumed | F12 covers lists of two or more curves and closed curves only. |

## Scope decision

| Extension | Decision | Reason |
|---|---|---|
| Tools as package-level functions | Ship first | Decided by the user; `decad` already uses free functions. Go forbids type parameters on methods, so only a function can return the caller's concrete curve type. |
| Trim and extend modify in place, trim the middle, delete with no crossing | Ship | F1, F2, F4, F7. Keeps constraints on the trimmed curve, as Fusion does. |
| Fillet and chamfer shorten their legs in place | Ship | F10a, F10b. Keeps constraints on the legs. |
| Pin trimmed and extended ends to the cutter | Ship | F5, F6. Decided by the user. |
| Offset sides and pinned ends like Fusion | Ship | F12, F15. Decided by the user. |
| Trim an arc, trim a circle, extend an arc | Ship | F1–F4, F7. `Trim`/`Extend` take a `*Line` today. |
| Extend that merges two curves, or closes an arc into a circle | No | F8 returns a different entity type than the input. This API refuses those two cases (deviation, listed under "Assumptions to verify in Fusion"). |
| Fillet line–arc and arc–arc | Ship | F9. A corner where a line meets an arc (a D-cut) cannot be filleted today. |
| Chamfer on curves | No | F11. |
| Offset circles, arcs and mixed chains | Ship | F12–F15. Today the tool skips non-lines without an error. |
| One dimension for a whole offset | No | F14 needs a constraint spanning many members. This design keeps one dimension per member, tied by `OffsetGroup.Set` (deviation, listed under "Assumptions to verify in Fusion"). |
| Break a circle | No | Needs two break points; `Trim` on a circle covers the profile use. |
| Ellipse, elliptical arc, spline, conic, NURBS | No | No closed-form split or fillet math exists for them in `geom`. The type constraint excludes them, and the slice-taking tools refuse them. |

Order: the function migration first, so the API breaks once. Then the Fusion behaviour changes on lines, each its own
PR because each changes existing output. Then the curve extensions, which inherit that behaviour. Offset goes last
because its constraint rules depend on how the source curves meet.

## Increment 0 — tools become package-level functions

### Rules for every migrated tool

1. **The sketch is the first parameter.** `s.Trim(l, x, y)` becomes `sketch.Trim(s, l, x, y)`. Names stay as they
   are. The package already declares types named `Fillet`, `Chamfer`, `Mirror`, `Pattern` and `Offset`, which rules
   out dropping the `Create` prefix.
2. **The old methods are removed**, not kept as wrappers. The breaking change is accepted.
3. **A nil sketch is refused** with the tool's refusal shape (false, or `ErrForeignEntity`). A nil receiver panicked at
   `s.ents` inside `foreignInput`; the guard keeps the "no panics" convention.
4. **Generics where the result has the input's type.** One exported constraint serves every curve tool:

   ```go
   // OpenCurve is the set of curve types the modification tools trim, extend, split and fillet:
   // curves with two end points. Use it only as a type constraint.
   type OpenCurve interface{ *Line | *Arc }
   ```

   The body switches on `any(e).(type)`. The constraint names `*Arc` from increment 0 so the signatures never change
   again. A kind a tool does not handle yet is refused at run time with the tool's refusal shape until its increment
   lands (rows below). A caller holding an `Entity` interface value switches on the kind before calling.
5. **Slice-taking tools stay non-generic.** Mirror, patterns and offset take heterogeneous `[]Entity`.
6. **Doc comments move with the code**, with `[Sketch.X]` links rewritten to `[X]` and the receiver sentence rewritten
   to name the parameter. Each comment states the nil-sketch refusal and which `OpenCurve` kinds are handled.

### New signatures

| Old method | New function | Handled in increment 0 |
|---|---|---|
| `s.Break(e Entity, x, y)` → `(Entity, Entity, bool)` | `Break[T OpenCurve](s *Sketch, e T, x, y float64) (T, T, bool)` | Line and arc, as today. |
| `s.Trim(l *Line, x, y)` → `(*Line, bool)` | `Trim[T OpenCurve](s *Sketch, e T, x, y float64) ([]T, bool)` | Line, today's semantics (one replacement, or a refusal). An arc returns `nil, false`. |
| — | `TrimCircle(s *Sketch, c *Circle, x, y float64) ([]*Arc, bool)` | Added in increment 6. |
| `s.Extend(l *Line, end *Point)` → `(*Line, bool)` | `Extend[T OpenCurve](s *Sketch, e T, end *Point) (T, bool)` | Line. An arc returns false. |
| `s.CreateFillet(l1, l2 *Line, r)` → `(*Fillet, error)` | `CreateFillet[A, B OpenCurve](s *Sketch, e1 A, e2 B, corner *Point, r float64) (*Fillet[A, B], error)` | Two lines. An arc returns `ErrUnsupportedEntity`. |
| `s.CreateChamfer(l1, l2 *Line, d)` | `CreateChamfer(s *Sketch, l1, l2 *Line, d float64) (*Chamfer, error)` | Lines only (F11). |
| `s.CreateMirror(ents, axis)` → `*Mirror` | `CreateMirror(s *Sketch, ents []Entity, axis *Line) (*Mirror, error)` | Unchanged copies; refusals return errors. |
| `s.CreatePatternRect(…)` | `CreatePatternRect(s *Sketch, ents []Entity, nx, ny int, dx, dy float64) (*Pattern, error)` | Unchanged. |
| `s.CreatePatternCircular(…)` | `CreatePatternCircular(s *Sketch, ents []Entity, center *Point, n int) (*Pattern, error)` | Unchanged. |
| `s.CreatePatternCircularRing(…)` | `CreatePatternCircularRing(s *Sketch, ents []Entity, center *Point, n int, start, end *Point) (*Pattern, error)` | Unchanged. |
| `s.CreateOffset(ents, d)` | `CreateOffset(s *Sketch, ents []Entity, d float64) (*OffsetGroup, error)` | Unchanged (lines, today's sign). |

Four changes go beyond moving the receiver, all made here so later increments do not break the API again:

- **`Break` returns `T`.** The halves have the input's type, so callers drop their type assertion. A non-curve
  `Entity` no longer compiles, which replaces the old documented false return for those kinds.
- **`Trim` returns `[]T`.** Fusion's trim returns zero, one or two curves (F1, F2, F4). Increment 0 returns one or a
  refusal; increment 1 adds the other two outcomes without a signature change.
- **`Fillet` becomes generic, and `CreateFillet` takes the corner.** The legs keep the input types:

  ```go
  // Fillet groups the geometry CreateFillet creates or keeps: the rounding arc, the two legs
  // (the input curves, shortened), the contacts T1 (on L1) and T2 (on L2), and the editable radius.
  type Fillet[A, B OpenCurve] struct {
  	Arc    *Arc
  	L1     A
  	L2     B
  	T1, T2 *Point
  	Radius *Distance
  }
  ```

  `f.L1` stays a `*Line` for a line–line fillet, so `NewVertical(f.L1)` in the tests and example still compiles. A
  line and an arc, or two arcs, can share both endpoints (a D-cut, a lens), so the corner is explicit from the start.
  `CreateChamfer` takes lines only, which share at most one endpoint, so it needs no corner.
- **`CreateMirror` gains an `error`**: `ErrForeignEntity`, `ErrReferenceGeometry`, `ErrUnsupportedEntity`, the
  sentinels the pattern tools already return.

The doc comment for `Trim` after increment 6, as the shape for the rest:

```go
// Trim removes the portion of e between the two crossings with other entities that bound the
// pick point (x, y), as Fusion's trim does. Picking an end portion shortens e in place and returns
// []T{e}; the moved end is tied to the cutter. Picking a middle portion deletes e and returns the
// two remaining pieces. If e crosses nothing, Trim deletes e and returns an empty slice. It
// returns nil and false, changing nothing, when s is nil, when e or one of its points is not live
// in s, or when e is reference geometry. Lines and arcs are both handled; see [TrimCircle].
func Trim[T OpenCurve](s *Sketch, e T, x, y float64) ([]T, bool)
```

### Callers of the old methods

Inside this repository (every one changes in increment 0):

| File | Calls |
|---|---|
| `tools_test.go` | 32 calls to all ten tools. |
| `tools_ownership_test.go` | 40 calls. Foreign, removed and typed-nil handles are typed variables, so inference works; an untyped `nil` needs `Break[*Line](s, nil, …)`. |
| `tools_unsupported_test.go` | 11 calls. `require.Nil(t, s.CreateMirror(…))` becomes `require.ErrorIs(t, err, ErrUnsupportedEntity)`. |
| `tools_fitspline_test.go` | 11 calls (patterns, mirror, ring). |
| `reference_test.go` | 5 calls. The `CreateMirror` refusal becomes `require.ErrorIs(t, err, ErrReferenceGeometry)`. |
| `revision_internal_test.go` | 6 calls (internal package, unqualified names). |
| `examples/sketch_fillet_example_test.go` | `s.CreateFillet(l1, l2, 3)` becomes `sketch.CreateFillet(s, l1, l2, corner, 3)`. The `// Output:` block does not change. |
| `internal/cmd/genimages/main.go` | `s.CreateFillet(r.BC, r.CD, 40)` gains `s` first and the shared corner. `go run ./internal/cmd/genimages docs/images` must leave `docs/images/fillet-*.svg` byte-identical. |
| `tools.go` | Calls between tools and comments. |
| Godoc links | `sketch.go` (`ErrForeignEntity`'s comment), the sentinel comments in `tools.go`. The two links in `compound.go` (`[Sketch.CreatePatternRect]`, `[Sketch.CreatePatternCircular]`) are fixed in increment 12 instead, because creation-variants increment 2 rewrites that comment. |

Agent and design docs that name the methods:

- `CLAUDE.md`: the `tools.go` row, and the verb-convention paragraph gains "sketch-modification tools are
  package-level functions taking the sketch first".
- `.claude/docs/sketch-core.md`: "A tool added later MUST call the guard explicitly" calls a tool "a free method", and
  the refusal list says "nil from `CreateMirror`".
- `.claude/docs/profiles-geom.md`, `docs/modification-tools-design.md`, `docs/acceptance-tests.md`,
  `docs/reference-geometry-design.md`, `docs/fusion-gap-analysis.md`.

No generated block calls a tool: `README.md` includes only `examples/sketch_readme_example_test.go`, and the guide
pages include the units, parametric, solving and goal examples. The PR still runs the README generator and the image
generator to show both come out unchanged (increment 2 is the one that changes an image: the fillet keeps its legs,
so `docs/images/fillet-after.svg` draws them in a different order).

Outside this repository (searched read-only: `decad`, `decad-gallery`, `joints`, `kinetograph`, `mtilt`):

| Repository | Pinned sketch version | Calls |
|---|---|---|
| `decad-gallery` | `c5a4c1dbb01a` | `models/cycloidal/loop.go:43`: `s.CreatePatternCircularRing(splines, center, n, start, end)`. |
| `decad` | `8e7da692d5d8` | None. Its `Trim`, `Extend`, `Fillet`, `Chamfer` and `Offset` are its own body functions. |
| `kinetograph`, `mtilt` | older pins | None. |
| `joints` | does not import sketch | None. |

Each repository pins a sketch pseudo-version, so nothing breaks until it bumps the pin.

## Modify in place, as Fusion does

Increments 1 and 2. Where Fusion modifies the original curve (F1, F7, F10a, F10b), the tool moves that curve's end
to a new point instead of replacing the curve. The handle stays live, its uid does not change, and every constraint
that names the entity stays: a `Horizontal` on a trimmed line, a `Radius` on a filleted arc leg, a `Concentric` on an
extended arc. This is how constraints carry across a trim, extend, fillet or chamfer, with no per-kind transfer code. Where
Fusion deletes the curve (F2, F3, F4), the tool deletes it and constraints that named it go with it (F16), except the
circle-to-arc case in increment 6.

### Mechanics

1. Create the new end point, set the entity's `Start` or `End` field to it (an internal write; the fields are
   exported, so `entityPoints` and `Revision` read the new point at once).
2. `RemovePoint` the old end point. It returns false and changes nothing while another entity still uses it.
   Constraints naming the old point go with it.
3. Refresh cached shared points (`refreshSharedEndpoints`). Four constraint types record, at construction, the
   endpoint their two operands share: `tangentLineCircle`, `tangentCircles`, `tangentLineEllipse` (line and
   elliptical arc) and `tangentConics`. After the move, each re-derives that point. If the operands no longer share
   one, the first three become an interior tangency: `shared` is cleared and `allocVars` allocates the sweep slack,
   so the caller's handle stays valid (F17). A `tangentConics` in that state is removed instead, because its other
   form needs contact-witness variables that only a fresh constraint allocates consistently.

### Effect per tool

| Tool | Before | After |
|---|---|---|
| `Trim`, end portion | Replacement line; constraints on the line dropped; old end left in `s.points`. | Same handle, end moved; constraints kept; old end removed. |
| `Trim`, middle portion | Refused (use `Break`). | `e` deleted; two new pieces sharing `e`'s far ends (F2). |
| `Trim`, no crossing | Refused. | `e` deleted, its unused end points removed, `[]T{}` returned (F4). |
| `Extend` | Replacement line. | Same handle, end moved. |
| `CreateFillet` (lines) | Both legs replaced, corner removed. | Both legs keep their handles, corner ends moved to `T1`/`T2`, corner removed (F10b). |
| `CreateChamfer` | Both legs replaced. | Both legs keep their handles (F10a). |

DOF: an end trim or an extend moves one end to a fresh point (+2) and removes the old one (−2), so DOF does not change
before pinning (increment 3). Today it rises by 2 because the old point stays. A middle trim adds two points (+4) and
keeps the far ends. A fillet or chamfer keeps its DOF change at 0, as today.

## Pin trimmed and extended ends to the cutter

Increment 3 (F5, F6). Every end point a trim or extend creates on a cutter gets one constraint to that cutter:

| Cutter | Constraint | Rows | Extra solver variables | DOF change for the end |
|---|---|---|---|---|
| Line | `NewPointOnLine(p, cutter)` | 1 (length) | 0 | −1 |
| Circle | `NewPointOnCircle(p, cutter)` | 1 (length) | 0 | −1 |
| Arc | `NewPointOnArc(p, cutter)` | 2 (on-circle length, in-sweep dimensionless) | 1 (the sweep slack, allocated by `allocVars`) | −1 |
| Any, crossing within `epsTool` of one of the cutter's end points | none: the trim reuses that end `*Point` | 0 | 0 | −2 (the fresh point is never created) |

`PointOnArc`'s slack is retired with the constraint by `retireVars`. When the crossing sits exactly on the cutter
arc's sweep boundary, the slack solves to 0, where its row's derivative vanishes; reusing the cutter's end point
(last row) covers that case. Ellipse and spline cutters still contribute no crossing, so nothing pins to them.
Reference cutters are pinned like any other: constraints may name reference geometry.

DOF before and after the whole trim, relative to the sketch before the call:

| Trim | Today | After increments 1 and 3 |
|---|---|---|
| Line or arc, end portion | +2 | −1 |
| Line or arc, middle portion | refused | +2 (two fresh points, +4; two pins, −2) |
| Circle with two crossings (increment 6) | refused | 0 (circle 3 → arc 5; two pins, −2) |
| Line, extend | +2 | −1 |

## Offset like Fusion

Increment 5 changes the line offset that exists today. Increments 11 and 12 apply the same rules to arcs.

- **Side (F12).** Positive `d` is to the right of the flow; negative to the left. A circle flows counter-clockwise, so
  positive is outside. Today's sign is the opposite (left-positive). The stored `Offset` constraint keeps its
  left-positive meaning, so existing documents read the same; `CreateOffset` writes `−σ·d` into it.
- **Flow (F12, F18).** Within each connected component of `ents`, the flow runs from the member that appears first in
  `ents` toward the next member in `ents` order that shares a point with it. A component of one open curve flows from
  its start to its end. A member traversed along its own start→end direction has `σ = +1`, otherwise `σ = −1`.
  `OffsetGroup` keeps each member's `σ` (unexported), and `Set(d)` writes the signed targets.
- **Ends pinned (F15).** Every free chain end `q'` gets `NewPointOnNormal(q', src, q)`: it stays on the source's
  normal at the source end `q`. A lone line offset goes from 2 DOF to 0. The pins go in a new
  `OffsetGroup.Constraints` field.
- **Topology matched (F13).** An offset that would make an arc or circle radius `<= 0` returns `ErrInvalidShape` at
  creation, as Fusion's default fails it.

### The `PointOnNormal` constraint

Increment 4. Pinning a line end to its normal has no existing constraint: `NewPerpendicular` on a construction spoke
from `q` to `q'` has zero length when `d` is 0. A new geometric constraint covers lines and arcs:

```go
// NewPointOnNormal holds p on the line through at that is normal to c: for a line, the
// perpendicular to its direction through at; for an arc, the radial line through its center and at.
func NewPointOnNormal[T OpenCurve](p *Point, c T, at *Point) Constraint
```

Residual (one row, length units): for a line, `dot(p − at, û)` with `û` the line's unit direction; for an arc,
`cross(at − C, p − C) / |at − C|`, the distance of `p` from the radial line. `norm()` floors the divisors. No auxiliary
variable. It also serves as the alignment row at tangent chain joints (increment 12), so no construction spokes are
needed.

| Site | Change |
|---|---|
| `constraint.go` | Type `pointOnNormal{P, At *Point; C Entity}`, `residual`, the generic constructor. |
| `json.go` | Type `"point_on_normal"`, marshal body, `constraintArity` `{2, 1}`, `rebuildConstraint` case that switches on the entity's kind. |
| `removal.go` | `constraintRefs` case `[]*Point{t.P, t.At}, []Entity{t.C}`. |
| `conditioning.go` | `condRowKinds`: one `rowLength`. |
| `annotate.go` | A badge at `P`, beside the `pointOnLine` case. |
| `introspect_test.go`, `introspect_nil_contract_test.go` | Kind row and nil-operand row. |
| `jacobian_inventory_test.go` | Line fixture, row `{"point_on_normal", 1, 8, 0, 0, "static", true}`, constructor map entry. |
| `enclose_system.go` | No case; `Enclose` refuses it with `ErrUncertifiedConstraint`. |

## Decisions that apply to every curve tool

1. **Modify in place where Fusion does; replace where Fusion deletes.** See "Modify in place, as Fusion does". New
   geometry reuses the original's `Center` `*Point` and every surviving end `*Point`, so neighbours stay attached.
2. **Curve support widens the increment 0 functions.** The one new function is `TrimCircle`: a circle trim returns
   `*Arc`, not the input type, so it cannot share `Trim`'s type parameter.
3. **Every tool calls `s.foreignInput` first**, and `s.owns` for each bare `*Point` argument, as
   `.claude/docs/sketch-core.md` "A tool added later MUST call the guard explicitly" requires.
4. **Reference geometry is refused as an input and accepted as a cutter.** A reference input returns the refusal
   (`nil, false` or `ErrReferenceGeometry`). A reference entity still cuts and still receives the pin constraint.
5. **Each new entity-kind switch is an allow-list with a refusing default**, the shape `unsupportedSeed` uses, and
   gets an `entitySwitchExempt` entry in `entity_switches_test.go`.
6. **No panics; validation before mutation.** Every refusal happens before the first `CreatePoint`, so a refused call
   leaves `Points()`, `Entities()` and `Constraints()` unchanged, as `TestOffsetZeroLengthIsAtomic` asserts today.

Error sentinels added to existing functions after increment 12:

| Function | Error | When |
|---|---|---|
| `CreateFillet` | `ErrForeignEntity` | `s` is nil, or `e1`, `e2`, a defining point of either, or `corner` is not live in `s`. |
| | `ErrReferenceGeometry` | Either curve is reference geometry. |
| | `ErrNoSharedCorner` | `corner` is not an endpoint of both curves. |
| | `ErrFilletInfeasible` | `r <= 0`; a leg has zero length or zero radius; the curves are tangent at the corner. |
| | `ErrFilletInfeasible` | No fillet circle has both contacts on the kept parts of the curves. |
| `CreateOffset` | `ErrUnsupportedEntity` | An entity is not a `*Line`, `*Arc` or `*Circle`. Today such entities are skipped. |
| | `ErrInvalidShape` | A zero-length line (unchanged), or an arc or circle whose offset radius would be `<= 0` (F13). |
| | `ErrInvalidShape` | A source point shared by three or more members, or a cusp joint. |
| | `ErrInvalidShape` | Offset curves that do not meet at a corner joint. |
| | `ErrInvalidShape` | A closed chain whose every joint is a tangent arc–arc joint. |

`ErrInvalidShape` messages name the condition through `fmt.Errorf("%w: …")`, as `CreatePatternRect` does.

## Trim on arcs and circles

Increment 6.

### Crossings on a circular carrier

A new helper `circleCrossings(s *Sketch, c *geom.Circle, skip Entity)` returns where the full circle `c` meets every
other entity, together with the cutter of each hit (the pin needs it):

- a line cutter through `geom.LineCircleIntersections`, keeping points whose `ClosestPointOnLine` parameter is in
  `[0, 1]`;
- a circle cutter through `geom.CircleCircleIntersections`;
- an arc cutter through `geom.CircleArcIntersections(c, cutter)`.

`lineCrossings` gains the same cutter-per-hit return in increment 3. Ellipses and splines contribute no crossing. A
concentric equal-radius cutter yields no points (`CircleCircleIntersections` returns nil for concentric circles). A
tangent cutter contributes one point.

`geom` gains one helper, and `(*Arc).Contains` is rewritten to call it:

```go
// AngleFromStart returns the counter-clockwise angle from a's start direction to p's direction
// about a's center, in [0, 2π). p is inside the sweep when the result is <= a.Sweep().
func (a *Arc) AngleFromStart(p *Point) float64
```

### `Trim` on an arc

1. Refuse on a nil sketch, a foreign input or a reference input.
2. Map each crossing to `t = AngleFromStart(hit) / Sweep()`. Drop hits with `t <= epsTool`, `t >= 1 - epsTool`, or
   outside the sweep.
3. Map the pick to `t` by its direction from the center, and find the nearest crossings below and above it.
4. Crossing only above: move `Start` to it (in place). Only below: move `End`. Both: delete the arc, create
   `CreateArc(C, Start, lo)` and `CreateArc(C, hi, End)`. Neither: delete the arc and return `[]*Arc{}`.
5. Pin each new end to its cutter (increment 3 table).

### `TrimCircle`

```go
// TrimCircle removes the portion of c between the two crossings that bound the pick (x, y), as
// Fusion's trim of a closed curve does: c is deleted and the remaining arc is returned, with both
// ends tied to their cutters and c's radius, diameter, concentric and equal-radius constraints
// moved to it. With fewer than two crossings c is deleted and an empty slice returned.
func TrimCircle(s *Sketch, c *Circle, x, y float64) ([]*Arc, bool)
```

1. Refuse on a nil sketch, a foreign input or a reference input.
2. Sort crossing angles; merge crossings closer than `epsTool` in arc length.
3. Fewer than two distinct crossings: delete `c`, return `[]*Arc{}` (F4 for zero; assumed for one, F16).
4. Find consecutive crossings `θa < θpick < θb` (cyclic). A pick exactly on a crossing returns `nil, false`.
5. Create `CreateArc(c.Center, P(θb), P(θa))` and pin both ends.
6. Move constraints (F16): every `Radius`, `Diameter`, `concentric` and `equalRadii` that names `c` has that operand
   rewritten to the new arc before `RemoveEntity(c)`. None of the four owns an auxiliary variable, and none depends on
   which part of the circle survives. The caller's handles stay valid. Every other constraint naming `c` (tangency,
   point-on-circle, distances to the circle) is dropped by the cascade, because it may refer to the removed part.
7. `RemoveEntity(c)` retires the radius variable.

DOF: the circle's 3 variables become the arc's 6 less its `arcRadius` row (5), less two pins (3). A moved `Radius`
removes one more, exactly as it did on the circle. Net change 0.

## Extend on arcs

Increment 7. `Extend(s, a, end)` with an `*Arc` lengthens `a` along its own circle from `end` to the nearest crossing
beyond it, in place.

1. Refuse on a nil sketch, a foreign or reference input, an unowned `end`, or an `end` that is neither endpoint.
2. From `a.End` the gap to a hit is the counter-clockwise angle from `EndAngle()`; from `a.Start`, the clockwise angle
   from `StartAngle()`.
3. Keep hits with gap in `(epsTool, 2π − Sweep() − epsTool)`. A hit at or past the other end would close the arc; F8
   returns a circle there, and this API returns false (deviation).
4. Move `end` to the nearest hit in place, `RemovePoint` the old end, pin the new one.

## Fillet on curves

Increments 8 and 9.

### `geom` helpers

`geom.Fillet` rewrites its input lines' endpoints, so the curve versions are new pure functions:

```go
// CornerFillet is a fillet arc of radius r rounding the corner where two curves meet.
type CornerFillet struct {
	Arc              *Arc   // minor arc; Arc.Start/Arc.End are T1 and T2 (pointer-equal), in CCW order
	T1, T2           *Point // contact on the first and the second curve
	Inside1, Inside2 bool   // fillet circle lies inside curve i's circle (always false for a line)
}

// FilletLineArc: l.End is the corner. atStart says whether the corner is a.Start (else a.End).
func FilletLineArc(l *Line, a *Arc, atStart bool, r float64) (CornerFillet, bool)

// FilletArcArc: each flag says which end of that arc is the corner.
func FilletArcArc(a1 *Arc, atStart1 bool, a2 *Arc, atStart2 bool, r float64) (CornerFillet, bool)
```

The fillet center `O` sits at distance `r` from each curve: on one of a line's two parallels at `r`, or on an arc's
concentric circle of radius `R + r` (outside) or `R − r` (inside, when `R > r`). The helpers intersect every pair of
carriers with `LineCircleIntersections` and `CircleCircleIntersections` (up to 8 candidates). A candidate is valid when
both contacts lie on the parts that survive: strictly between the line's far end and the corner, or inside the arc's
sweep away from the corner end. Among valid candidates the helpers return the one whose farther contact is closest to
the corner. They return false when `r <= 0`, a curve is degenerate, the two corner coordinates differ by more than
`epsIntersect`, the curves are tangent at the corner (`|cross| <= epsIntersect` between unit tangents), or no candidate
is valid. Concentric arcs sharing a corner are tangent there and fall in that case.

### `CreateFillet` with arcs

1. Screen inputs (sentinel table). Build oriented `geom` copies, call `geom.Fillet`, `FilletLineArc` (operands swapped
   as needed) or `FilletArcArc`; false maps to `ErrFilletInfeasible`.
2. Create `T1`, `T2` and the center. Move each leg's corner end to its contact in place (F10b). An arc leg keeps its
   center and far end, so its `arcRadius` row holds `T` on its circle.
3. Create the fillet arc in the helper's orientation.
4. Add endpoint tangency per leg (shared contact, no slack): `NewTangent(leg, fillet)` for a line,
   `NewTangentCircles(leg, fillet, Inside_i)` for an arc.
5. Add `NewDistance(fillet.Center, fillet.Start, r)` as `Radius`.
6. `RemovePoint(corner)`, then refresh cached shared points.

DOF: +6 (`T1`, `T2`, center), −2 (corner), −4 rows (2 tangencies, fillet `arcRadius`, radius) = 0. Constraints on the
legs (a `Radius` on an arc leg, a `Horizontal` on a line leg) stay.

## Offset of arcs and circles

### The `RadialOffset` constraint

Increment 10. No existing constraint relates two radii by a difference:

```text
residual = Dst.R() - Src.R() - d        (one row, length units)
```

Positive `d` makes `Dst` larger, matching F12's "positive is outside" for a circle. The constraint does not hold the
curves concentric: `CreateOffset` makes the copy share the source's center `*Point`. A coincidence row on a shared
center would have a zero gradient and read as redundant. The godoc tells a direct caller to share the center or add
`NewConcentric`. No auxiliary variable.

| Site | Change |
|---|---|
| `constraint.go` | Type, `residual`, `NewRadialOffset(src, dst Circular, d float64) *RadialOffset`. |
| `json.go` | Type `"radial_offset"`, marshal body, `constraintArity` `{0, 2}`, `rebuildConstraint` through `circular`. |
| `removal.go` | `constraintRefs` case `nil, []Entity{t.Src, t.Dst}`. |
| `conditioning.go` | `condRowKinds`: one `rowLength`. |
| `annotate.go` | `dimension` case through `leaderToCircular`. |
| `introspect_test.go`, `introspect_nil_contract_test.go` | Kind row and nil-operand row. |
| `jacobian_inventory_test.go` | Two-circle fixture, row `{"radial_offset", 1, 4, 2, 0, "static", true}`, constructor map entry. |
| `enclose_system.go` | No case; refused with `ErrUncertifiedConstraint`, as `Offset` is. |

Neither new constraint bumps the document version. Earlier kinds such as `tangent_ellipses` were added under version
2, and an older reader refuses an unknown kind with `unknown constraint type`.

### Circles and lone arcs

Increment 11. For a source with no end shared with another member of `ents`:

- **Circle:** `CreateCircle(src.Center, R + d)` and `NewRadialOffset(src, dst, d)`. DOF added: 0.
- **Arc** (flow start→end, F18): ends seeded at `C + (R + d)·(P − C)/R`, `CreateArc(src.Center, s', e')`,
  `NewRadialOffset(src, dst, d)`, and `NewPointOnNormal` at both ends. Rows: `arcRadius`, `RadialOffset`, two normals
  = 4 on 4 variables. DOF added: 0.

`R + d <= 0` returns `ErrInvalidShape` (F13). An arc sharing an end with another member returns `ErrInvalidShape`
until increment 12. Entities other than line, arc and circle return `ErrUnsupportedEntity` through a new allow-list
`unsupportedOffset`.

### Offset chains

Increment 12.

#### Joint classes

At each joint the flow compares the unit tangent arriving along member A with the one leaving along member B:

| Class | Test | Meaning |
|---|---|---|
| Corner | `abs(cross) > 1e-6` | The offset curves cross. The joint copy is their intersection (the mitre point). |
| Tangent | `abs(cross) <= 1e-6`, `dot > 0` | G1 joint. The offset curves touch at `q + d·n_right(q)`. |
| Cusp | `abs(cross) <= 1e-6`, `dot < 0` | The chain folds back. Refused with `ErrInvalidShape`. |

For a corner angle θ the mitre point sits about `d/θ` from the joint, so a joint within 1e-6 of tangent is
ill-conditioned as a corner and well-conditioned as a tangent joint. A source point shared by three or more members
is refused with `ErrInvalidShape`.

#### Why tangent joints need their own rules

Each offset member contributes one "on my offset curve" row at each end: a line's two `Offset` rows, or an arc's
`arcRadius` plus `RadialOffset`. At a corner joint the two rows have different normals and fix the point. At a tangent
joint they share a normal: one is redundant, and the point slides along the tangent. So a tangent joint needs one row
along the tangent, and one normal row must go. An arc's internal `arcRadius` cannot be dropped, so at every tangent
joint touching an arc, the arc keeps the normal row and the line gives its row up.

#### Rules

- **Alignment at a tangent joint touching an arc:** `NewPointOnNormal(q', arc, q)`.
- **Free chain end:** `NewPointOnNormal(q', member, q)` (F15).
- **Line with no tangent arc joint:** `NewOffset(src, dst, −σd)`.
- **Line with exactly one tangent arc joint:** `NewTangent(dst, arcCopy)` at that joint (endpoint tangency, one
  dimensionless row, no slack), and no `Offset`. With the joint fixed by the arc side, this row sets the copy's
  direction, which puts its other end on the offset carrier.
- **Line with two tangent arc joints:** no constraint; both ends are fixed by the arc sides.
- **Arc:** `NewRadialOffset(src, dst, σd)`, except an arc whose joint with the previous member in flow order is a
  tangent arc–arc joint. That arc takes its radius from the joint point. In each run of arcs joined by tangent arc–arc
  joints, only the first carries the dimension.
- **Line–line tangent joint** (collinear consecutive lines): both lines keep their `Offset`, as today. One redundant row
  and one sliding DOF remain at that joint; the case is rare and is not changed.
- **Closed chain whose every joint is a tangent arc–arc joint** (a circle broken into arcs): refused with
  `ErrInvalidShape`. Around such a loop one `arcRadius` row is always dependent. Offset the circle instead.

| Source | Copy variables | Rows | Free DOF |
|---|---|---|---|
| Slot: 2 lines + 2 arcs, all joints tangent | 8 | 2 × (`arcRadius` + `RadialOffset`) + 4 alignments = 8 | 0 |
| Rounded rectangle: 4 lines + 4 fillet arcs | 16 | 4 × 2 + 8 alignments = 16 | 0 |
| Rectangle, one corner filleted | 10 | arc 2 + 2 alignments + 2 `Tangent` + 2 `Offset` × 2 rows = 10 | 0 |
| Open line–arc, tangent joint | 6 | `arcRadius` + `RadialOffset` + alignment + `Tangent` + 2 free-end normals = 6 | 0 |
| Open line chain (after increment 5) | 2 per point | `Offset` rows + 2 free-end normals | 0 |

Each copy point is seeded where the constraints put it: a free end or tangent joint at `q + d·n_right(q)`, a corner
joint at the intersection of the two offset carriers nearest `q` (`LineLineIntersection`,
`LineCircleIntersections`, `CircleCircleIntersections`; none returns `ErrInvalidShape`).

## Interactions

- **`Sketch.Revision`.** No change to `revision.go`. An in-place end move changes the entity's defining points, which
  `Revision` hashes, so a held `Profile` reads stale; the uid does not change. New entities enter through
  `addEntity`, so `revision_internal_test.go`'s uid invariant covers it with no new case.
- **JSON.** Only `"point_on_normal"` and `"radial_offset"` are new on disk. `Fillet` and `OffsetGroup` are not
  serialized. Each `Offset`/`RadialOffset` keeps its signed target across a reload.
- **Removal cascade.** In-place edits remove only the old end point. Deleting trims use `RemoveEntity`. Both new
  constraints are found by the cascade through their `constraintRefs` cases.
- **Foreign screens.** `foreignInput` covers every entity input, `owns` covers `Extend`'s `end` and `CreateFillet`'s
  `corner`. An end trim replaces the trimmed-away point with a fresh local one, and so does a fillet at its corner, so
  both stay in `TestToolsDoNotLaunderForeignPoint` for lines and are added for arcs. `TrimCircle` keeps the circle's
  only point (its center), and `Extend` replaces only `end`, which must pass `owns`.
- **Reference geometry.** Refused as input; used as a cutter and as a pin target.
- **Profiles.** Fillet, offset and reused cutter-end joints are shared `*Point`s, so their edges have exact cut
  parameters. A pinned trim end that is not a cutter end point is cut by intersection, as today.
- **Solver conventions.** Both new constraints are length rows. All tangencies the tools add are endpoint tangencies
  and allocate no slack, except the interior tangencies the shared-point refresh creates.

## Tests

All tests live in external `sketch_test`/`geom_test` packages and use `testify/require`, except the internal files the
repository already keeps (`revision_internal_test.go`, `jacobian_inventory_test.go`). Each tool test asserts geometry
(`InDelta` 1e-6 after a solve, 1e-9 for pure construction), the constraint graph (`DOF`, expected `ConstraintKind`s)
and a dimension edit with a re-solve (`docs/acceptance-tests.md`).

Fixtures with closed-form answers:

- **Line trim in place:** `TestTrimEndStub`'s sketch plus `NewHorizontal(h)`. After `Trim` the result is `[]*Line{h}`,
  `h.End` is at (2,0), the `Horizontal` is still in `Constraints()`, the old end is gone from `Points()`, and (after
  increment 3) a `point_on_line` names the cutter. DOF drops by 1.
- **Line middle trim:** `TestTrimInteriorRejected`'s sketch now returns two lines, (-5,0)→(-2,0) and (2,0)→(5,0).
- **No crossing:** `TestTrimNoCrossing`'s line is deleted and `[]*Line{}` returned.
- **Fillet keeps legs:** `TestCreateFillet` with `NewVertical(l1)` added before the fillet: `f.L1 == l1` and the
  `Vertical` survives.
- **Shared-point refresh:** a line tangent to an arc at a shared end, trimmed so that end moves: the tangent handle is
  still in `Constraints()`, now with its sweep slack, and solves.
- **Arc trim:** arc (5,0)→(0,5) about (0,0), cutter (3,-1)→(3,10) crosses at (3,4). A pick at (4.9,1) returns the same
  arc with `Start` at (3,4) and a `point_on_line` to the cutter.
- **TrimCircle:** circle r=5 at (0,0) with `NewRadius(c, 5)`; cutter (3,-10)→(3,10). A pick at (5,0) gives the arc
  (3,4)→(3,-4) with sweep `2π − 2·atan2(4,3)`; the same `*Radius` handle now names the arc; DOF unchanged. A tangent
  cutter (5,-10)→(5,10) alone deletes the circle.
- **Arc extend:** arc (5,0)→(0,5); cutter (-3,0)→(-3,10). Extending `End` reaches (-3,4). With only the cutter
  (2,2)→(6,6), which crosses inside the sweep, `Extend` returns false.
- **FilletLineArc:** arc about (0,0), R=10, corner at its `Start` (10,0), sweeping to (0,10); line (10,0)→(20,0). r=2
  gives `O = (√140, 2)`, `T1 = (√140, 0)`, `T2 = (5√140/6, 5/3)`, outside the arc's circle.
- **FilletArcArc:** arc 1 about (0,0) from (10,0) to (0,10); arc 2 about (10,10) from `(10−5√2, 10−5√2)` to (10,0);
  corner (10,0). r=2 gives `O = (5+√7, 5−√7)`, inside both circles; every other candidate has a contact outside a
  sweep.
- **PointOnNormal:** a point held on the normal of the line (0,0)→(10,0) at (10,0) solves to x = 10; on the normal of
  the arc about (0,0) at (5,0), to y = 0.
- **Line offset like Fusion:** `TestOffsetLine` with `d = 2` puts the copy at y = −2 (right of +x) at DOF 0.
  `TestOffsetChainMitresCorner` with `d = 2` puts the corner at (12,−2).
- **RadialOffset:** concentric circles, `NewRadius(src, 10)`, `NewRadialOffset(src, dst, 2)`: `dst.R() = 12`; target −3
  gives 7.
- **Offset chains:** a slot (two lines, two half circles), a rounded rectangle (four lines, four corner arcs), a
  rectangle with one rounded corner, and an open line–arc chain, each built from raw entities with every source point
  grounded, solve to DOF 0 before and after `Set`, and no constraint of the copy reads as redundant. Refusals with
  unchanged counts: a branch, a cusp, a closed loop of only tangent arcs, an offset radius `<= 0`, an ellipse.

## Increments

Each increment is one PR, mergeable alone. The exact steps, code, tests and commands for each are in
`docs/curve-modify-tools-tasks.md`; every increment there was built and tested in a prototype copy of this
repository. Merge them in numeric order.

| # | Branch | Increment | Depends on | Changes existing output |
|---|---|---|---|---|
| 0 | `refactor-tools-package-functions` | Tools become package-level functions | — | API only (plus nil-sketch refusal, `CreateMirror` errors) |
| 1 | `feat-trim-extend-in-place` | Trim and extend like Fusion on lines | 0 | Yes |
| 2 | `feat-fillet-chamfer-keep-legs` | Fillet and chamfer keep their legs | 1 | Yes (and `docs/images/fillet-after.svg` draw order) |
| 3 | `feat-trim-pin-to-cutter` | Pin trimmed and extended ends to the cutter | 2 | Yes |
| 4 | `feat-point-on-normal` | `PointOnNormal` constraint | 3 | No |
| 5 | `feat-offset-fusion-lines` | Line offset like Fusion (right-positive, list flow, pinned ends, branches refused) | 4 | Yes |
| 6 | `feat-trim-arc-circle` | `Trim` on arcs, `TrimCircle` with constraint move | 5 | No |
| 7 | `feat-extend-arc` | `Extend` on arcs | 6 | No |
| 8 | `feat-geom-fillet-curves` | `geom` fillet helpers | 7 | No |
| 9 | `feat-fillet-arcs` | `CreateFillet` with arcs | 8, and creation-variants increment 0 | No |
| 10 | `feat-radial-offset` | `RadialOffset` constraint | 9 | No |
| 11 | `feat-offset-circles-arcs` | `CreateOffset` for circles and lone arcs | 10 | Non-lines refused instead of skipped |
| 12 | `feat-offset-chains` | `CreateOffset` for chains of lines and arcs | 11 | Joined arcs offset instead of refused |

The "Depends on" column is the merge order the task file is written against. The true code dependencies are looser
(4, 8 and 10 touch nothing the increments before them change), but each task anchors its edits on the text the
previous increment leaves, so the order is kept linear.

Increment 9 needs the creation-variants fix `fix rank of shared-endpoint arc tangency`
(`docs/creation-variants-tasks.md` on its branch, increment 0). Without it, a `NewTangentCircles` between two arcs
that share their contact point reads as redundant, and the arc fillet's "DOF unchanged" test fails by one.

## Assumptions to verify in Fusion

Every item below is implemented as written. Each line ends with the check in Fusion that settles it.

1. **F6 — a trimmed or extended end gets a point-on-curve constraint to its cutter.** Check: draw two crossing lines,
   trim one, and list the constraints on the new end point.
2. **F15 — an offset's free ends sit on the source normals, so the copy is fully determined.** Check: offset one open
   line with the source fully constrained, and see whether the copy turns black (fully constrained).
3. **F16 — trimming a circle moves its radius, diameter, concentric and equal-radius constraints to the arc, and
   drops the rest; a middle trim drops the original's constraints.** Check: dimension a circle's diameter, make it
   concentric with another, trim it, and list what remains.
4. **F17 — a tangency survives when a trim moves the touching end off the curve.** Check: make a line tangent to an
   arc at a shared end, trim that end off, and see whether the tangent glyph remains.
5. **F18 — a single open curve flows from its start to its end, so positive offset goes to its right.** Check: offset
   one line drawn left to right by +5 mm and see whether the copy lands below it.
6. **Deviation from F8 — an extend that would merge two curves or close an arc into a circle is refused.** Check:
   extend an arc toward its own other end and see that Fusion returns a circle.
7. **Deviation from F14 — each offset member carries its own dimension, tied by `OffsetGroup.Set`.** Check: offset a
   chain and see that Fusion shows one offset dimension.
8. **`Break` keeps dropping constraints on the broken curve.** Check: break a horizontal line with the Break tool and
   see whether both halves keep the horizontal constraint.
9. **The corner of a fillet is named by an explicit point.** Fusion's `addFillet` picks the corner by two points near
   the kept ends. Check: none needed; the API shape differs on purpose, and the result is the same fillet.
10. **The arc chain refusals (a cusp, a point shared by three members, a closed loop of only tangent arcs).** Check:
    try each offset in Fusion and see whether it fails or produces a result.

## API choices made without asking

These are not Fusion behaviour; each is listed so it can be overridden.

- There is no chamfer on curves (Fusion's sketch chamfer takes lines only, F11).
- `CreateMirror` returns an error instead of nil.
- Every tool refuses a nil sketch instead of panicking.
- `RadialOffset`'s positive direction is outward (it matches F12's "positive is outside" for a circle).
- `Trim` returns `[]T` from increment 0, so the later Fusion behaviour (zero, one or two pieces) needs no second break.
