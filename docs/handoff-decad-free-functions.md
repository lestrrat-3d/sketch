# Handoff: sketch modification tools become package-level functions

Audience: whoever maintains `decad` and `decad-gallery` when they bump their `github.com/lestrrat-3d/sketch` pin.
Design and full rationale: `docs/curve-modify-tools-design.md` in the sketch repository. Nothing here is merged yet;
the changes land in that design's increments 0 to 12, one PR each.

## What changes in sketch's API

Increment 0 removes the ten modification-tool methods on `*sketch.Sketch` and adds package-level functions that take
the sketch first. It is a compile-time break for every caller.

| Old method call | New function call |
|---|---|
| `s.Break(e, x, y)` → `(Entity, Entity, bool)` | `sketch.Break(s, e, x, y)` → `(T, T, bool)`, `T` is `*Line` or `*Arc` |
| `s.Trim(l, x, y)` → `(*Line, bool)` | `sketch.Trim(s, e, x, y)` → `([]T, bool)` |
| — | `sketch.TrimCircle(s, c, x, y)` → `([]*Arc, bool)` (increment 6) |
| `s.Extend(l, end)` → `(*Line, bool)` | `sketch.Extend(s, e, end)` → `(T, bool)` |
| `s.CreateFillet(l1, l2, r)` → `(*Fillet, error)` | `sketch.CreateFillet(s, e1, e2, corner, r)` → `(*Fillet[A, B], error)` |
| `s.CreateChamfer(l1, l2, d)` | `sketch.CreateChamfer(s, l1, l2, d)` |
| `s.CreateMirror(ents, axis)` → `*Mirror` | `sketch.CreateMirror(s, ents, axis)` → `(*Mirror, error)` |
| `s.CreatePatternRect(ents, nx, ny, dx, dy)` | `sketch.CreatePatternRect(s, ents, nx, ny, dx, dy)` |
| `s.CreatePatternCircular(ents, center, n)` | `sketch.CreatePatternCircular(s, ents, center, n)` |
| `s.CreatePatternCircularRing(ents, center, n, start, end)` | `sketch.CreatePatternCircularRing(s, ents, center, n, start, end)` |
| `s.CreateOffset(ents, d)` | `sketch.CreateOffset(s, ents, d)` |

Type changes a caller sees:

- `sketch.OpenCurve` (`*Line | *Arc`) is the type constraint of `Break`, `Trim`, `Extend` and `CreateFillet`. A caller
  holding a `sketch.Entity` switches on its kind before calling them.
- `sketch.Fillet` is generic, `Fillet[A, B OpenCurve]`; for two lines it is `*sketch.Fillet[*sketch.Line,
  *sketch.Line]`, and `L1`/`L2` are `*Line`.
- `CreateFillet` takes the shared corner `*Point` explicitly.
- `CreateMirror` returns `ErrForeignEntity`, `ErrReferenceGeometry` or `ErrUnsupportedEntity` where it returned nil.
- Every function refuses a nil `*Sketch` instead of panicking.

## Behaviour changes with no compile error

Later increments change what the same calls do, to match Fusion. A caller compiles unchanged and may see different
geometry:

| Increment | Change |
|---|---|
| 1 | `Trim` and `Extend` on a line move the end of the SAME handle instead of replacing the line; constraints on the line survive. A middle pick returns two new pieces; a line with no crossing is deleted and `[]T{}` returned. |
| 2 | `CreateFillet` and `CreateChamfer` keep the input legs as `L1`/`L2`; constraints on the legs survive. |
| 3 | A trimmed or extended end gets a point-on-curve constraint to its cutter, or shares the cutter's end point when the crossing is there. DOF after a trim is lower than before. |
| 5 | `CreateOffset`: positive distance is to the RIGHT of the flow (today: left of each line). Flow runs in the order of the `ents` list. Free chain ends are pinned to the source normals, so an open offset reaches DOF 0. |
| 11 | `CreateOffset` returns `ErrUnsupportedEntity` for kinds other than line, arc and circle, which it skipped before; circles and arcs are offset. |

None of these changes `TExact` semantics. Increment 3 makes more trim vertices shared points, so more profile edges at
trim ends report exact cut parameters.

## What each repository needs to adopt

Searched read-only on the date of this note.

| Repository | Pin | Calls to the old methods | Change needed after bumping past increment 0 |
|---|---|---|---|
| `decad` | `8e7da692d5d8` | None. Its `Trim`, `Extend`, `Fillet`, `Chamfer` and `Offset` are its own body functions. | None for the API break. Re-run its sketch-backed tests after increments 1, 3 and 5, which change sketch geometry for the same calls. |
| `decad-gallery` | `c5a4c1dbb01a` | `models/cycloidal/loop.go:43`, `s.CreatePatternCircularRing(splines, center, n, start, end)` | Change to `sketch.CreatePatternCircularRing(s, splines, center, n, start, end)`. No behaviour change for patterns. |
| `kinetograph`, `mtilt` | older pins | None. | None. |

No GitHub issue was filed for this. Carry it out when the pins are bumped.
