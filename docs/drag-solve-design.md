# Drag-Solve API — Design

Status: **proposed** (not yet implemented). Prerequisite for any interactive
layer (gap-analysis priority item 5).

## Problem

Fusion's defining interaction: grab a point and pull. The solver re-solves
continuously so every constraint keeps holding while the grabbed point follows
the cursor as closely as the constraints allow. Pulling past what the
constraints permit must not error — the geometry should settle at the closest
feasible configuration, never explode or snap back.

## Approach: soft target residuals

Add two temporary, low-weight residuals for the dragged point:

```
w·(p.x − tx),  w·(p.y − ty)        // w ≪ 1, length units
```

Hard constraints keep their unit-normalized residuals, so with `w` small the
least-squares solution satisfies them essentially exactly and spends the
*remaining* DOF minimizing distance to the target. This composes with two
existing solver properties:

- **Levenberg (absolute) damping** already yields the minimum-norm step for
  under-constrained sketches, so un-dragged free geometry moves as little as
  possible — the expected "drag feel".
- Residual unit-normalization means one global weight works: the soft residual
  is in length units like every other length residual.

Rejected alternative: temporarily `Fix` the point at the cursor and solve.
Over-constrains the sketch for any cursor position off the feasible manifold,
turning the common case (user drags past a limit) into a non-converging solve.

## API

```go
drag := s.StartDrag(p)            // p *Point; returns *Drag
res, err := drag.To(x, y)         // set target, re-solve (warm-started), report
drag.End()                        // remove the soft residuals
```

- `*Drag` is a handle owning the temporary soft-target constraint. `To` may be
  called repeatedly (every pointer move); each call solves from the previous
  solution, which converges in a few iterations for small cursor deltas.
- `To` uses a small iteration budget (default ~25; option-overridable with the
  existing `SolveOption`s) and does **not** return `ErrNotConverged` merely
  because the target is unreachable — the soft residual is *expected* to stay
  nonzero. Convergence is judged on the hard-constraint residuals only.
- `End` removes the soft residuals and leaves the geometry where it settled.
  A final plain `Solve` is unnecessary (hard constraints already hold) but
  harmless.
- The soft-target constraint is `internalConstraint`-style transient state: it
  is never serialized, and it must be excluded from `DOF`/`Redundant`/
  `RedundantConstraints` accounting (it is not a user constraint).
- Dragging entities (a line by its body, a circle by its rim) is layered on
  later as multiple point targets; the engine primitive is the point drag.

## Weight selection

Start with fixed `w = 1e-3`. Hard-constraint residuals are O(1) in
mm/dimensionless; the soft contribution to JᵀJ is `w²` smaller, so constraint
satisfaction degrades by ~1e-6 relative — below the solver tolerance. If
profiling shows sluggish convergence on large sketches, revisit with an
adaptive weight (scale by the sketch's bounding-box diagonal). Do not expose
the weight publicly until a real consumer needs it.

## Interactions with existing machinery

- **`residuals()`/row-mapping invariant** (see CLAUDE.md): the soft residuals
  are appended via the same constraint list, so `RedundantConstraints` must
  skip the drag constraint the same way it skips driven dimensions —
  one shared "contributes no user equations" predicate is the likely
  refactor.
- **Driven dimensions** refresh after every `To` solve (they piggyback on
  `Solve`); this is what a GUI wants — live measurement readout during drag.
- **Parameters**: `ApplyParameters` runs per `To` call. Cheap today; if it
  shows up in profiles, cache until `Set`/`Bind` invalidates.
- **Concurrency**: a `Sketch` is a mutable document; `Solve` and therefore
  `To` mutate it. Like `Solve`, drag is single-goroutine per sketch. The GUI
  layer owns serialization of pointer events.

## Testing plan (no GUI needed)

- Rectangle with width/height dimensions, drag corner C outward → C stays at
  (w, h): hard constraints win, soft target unreachable, no error.
- Under-constrained point on a line, drag it off the line → point lands at the
  perpendicular projection of the cursor (closest feasible).
- Drag step sequence (simulated pointer path) → each step converges within the
  iteration budget from the previous solution (warm-start works).
- `End` then `Solve` → identical geometry (drag leaves no residue), JSON
  round-trip contains no trace of the drag.

## Open questions

- Should `To` report the achieved point separately from `*Result` (e.g.
  `(x, y)` actually reached) for cursor-rubber-banding UIs?
- Snapping (to grid, to other geometry) — GUI-layer concern, but `To` could
  accept a pre-snapped target without engine changes. Confirm nothing more is
  needed.
- Multi-point drag (marquee-select then drag) — N soft targets; same machinery,
  API shape TBD.
