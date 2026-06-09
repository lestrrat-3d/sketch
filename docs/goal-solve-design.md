# Goal-Solve (Soft Targets) — Design

Status: **implemented** (`WithGoal` in `solver.go`; tests in `goal_test.go`).
Supersedes the earlier drag-solve design: dragging is the motivating use case,
but the engine primitive is more general and deliberately knows nothing about
gestures.

## Problem

Interactive manipulation — Fusion's grab-a-point-and-pull — needs the solver to
answer one question continuously: *given desired positions for some points,
find the closest feasible configuration*. Every constraint must keep holding
exactly; the targeted points should get as close to their targets as the
remaining degrees of freedom allow. Pulling past what the constraints permit
must not error — the geometry settles at the nearest feasible spot.

## Engine/UI split (the design decision)

Only the projection is engine work. Everything that decides *which* points get
targets and *what* the targets are is UI policy:

| Engine (this design) | UI layer (out of scope) |
|---|---|
| Closest-feasible projection given (point → target) pairs | Hit-testing, screen→sketch transforms |
| Constraints win over targets, no error when unreachable | Snapping (grid, geometry) — pre-snap the target |
| Warm-started incremental re-solve | Gesture policy: drag a line by its body — translate or rotate? Drag a circle's rim — radius or center? |
| Minimum-norm motion of untargeted geometry | Drag session lifecycle (pointer down/move/up) |

Encoding gesture policy in the engine (`StartDrag(entity)`) would leak UI
decisions into the solver layer — the library-first principle in CLAUDE.md says
exactly not to. Any UI's inputs reduce to the same contract: a set of
(point → target) pairs, re-solved as they change. That contract is stable now,
before any GUI exists; the gesture vocabulary on top is not, and the engine
does not try to define it. Even gestures that sound like they need more — a
rotation handle, a modifier-key axis lock — reduce to it: the UI computes the
rotated/axis-clamped positions itself and issues plain point goals.

## API

Goals are a `SolveOption`, following the existing option conventions:

```go
res, err := s.Solve(sketch.WithGoal(p, x, y))            // one goal
res, err := s.Solve(sketch.WithGoal(p1, x1, y1),
                    sketch.WithGoal(p2, x2, y2))         // any number
```

- One point with one goal *is* a drag step. A marquee drag is several goals.
  "Translate this line" is the UI issuing goals for both endpoints under its
  own policy.
- Goals are per-call and transient: they exist only inside that `Solve`
  invocation. Nothing is stored on the sketch, nothing serializes, and a
  subsequent plain `Solve` is unaffected.
- Warm-starting needs no API: every solve already starts from the current
  geometry, so calling `Solve(WithGoal(p, …))` per pointer-move event is the
  incremental re-solve. For interactive cadence the caller can bound work with
  the existing `WithMaxIterations`.
- A goal on a fixed (grounded) point is legal and inert — the point has no free
  variables to move. No error: the UI should not need to special-case
  grounding before issuing goals.
- Multiple goals on the same point are legal; equal-weight least squares
  settles on their average. This falls out of the algebra — the implementation
  must not add an explicit averaging or deduplication step.
- Goal coordinates are in base units (mm), like point coordinates everywhere
  in the API.

### Option plumbing (pin this down)

`WithGoal` is an ordinary `SolveOption` whose value is a triple:

```go
type goalTarget struct {
	p      *Point
	tx, ty float64
}
```

`solveConfig` gains `goals []goalTarget`, and the option-folding switch
**appends** (`o.goals = append(o.goals, …)`) — it must not assign, or repeated
`WithGoal`s silently become last-write-wins and the "any number of goals"
contract breaks.

## Mechanics: augmented rows, not constraints

A goal contributes two extra least-squares rows during that solve only:

```
w·(p.x − tx),  w·(p.y − ty)        // w = 1e-3, length units
```

Crucially, goals are **not** `Constraint`s and never enter `s.cons`:

- `DOF()`, `Result.DOF`, `Result.Redundant` and `RedundantConstraints()` are
  computed from the hard-constraint Jacobian only. A goal must not change any
  of them (a drag must not make a sketch look constrained).
- The CLAUDE.md row-mapping invariant (`RedundantConstraints` mirrors
  `residuals()` row order) stays trivially intact because goal rows never
  appear in `residuals()` — they are appended only inside `Solve`'s own
  iteration, after the hard rows.
- Implementation shape — **two phases** (this is load-bearing, found the hard
  way during implementation):
  1. *Pull phase* (only when goals are present): an LM pass on the augmented
     system `[hard residuals | goal rows]` (`m = mh + 2g` rows; cost,
     step-acceptance and Jacobian all use the augmented evaluation
     consistently). This moves geometry toward the targets — but plain
     weighted least squares **trades the hard constraints off against the
     goal** at the optimum of an *unreachable* target: the residual balance
     leaves the constraints violated by O(w²·pull) ≈ 10⁻⁵ mm for a 10 mm
     pull, far above the 10⁻¹⁰ solver tolerance. A single augmented pass can
     therefore never satisfy "constraints hold exactly".
  2. *Polish phase* (always; the only phase without goals): an LM pass on the
     hard residuals alone, projecting the geometry back onto the constraint
     manifold. The correction is O(w²·pull) — negligible against the goal
     motion — and converges in a few iterations from the phase-1 warm start.
     Goal attainment is preserved; constraints end up exact.
- `rank`/`DOF`/`refreshDriven` keep calling the unchanged hard-only
  `residuals()`; goal rows exist only inside phase 1's evaluator.
- The pure-goal case (no hard constraints at all) works through the same
  structure: phase 1 moves the free point to the target, phase 2 is an
  immediate no-op (zero hard rows). `Solve` must not early-return before
  phase 1 just because there are no hard residuals.

Why the existing solver machinery carries this:

- **Levenberg (absolute) damping** already yields the minimum-norm step for
  under-constrained systems, so untargeted free geometry moves as little as
  possible — the expected interaction feel.
- **Residual unit-normalization** means one fixed weight is meaningful across
  sketches: hard residuals are O(1) in mm or dimensionless, so the goal's
  `w²` ≈ 10⁻⁶ relative contribution to JᵀJ keeps phase 1's constraint
  violation small (it is then the polish phase, not the weight, that makes
  constraints exact — see above).

### Convergence semantics

`Result.Residual` and `Result.Converged` are computed from the **hard rows
only**. A nonzero goal row at the optimum is the *expected* outcome of an
unreachable target, not a failure: `Solve` with goals returns success whenever
the constraints hold, wherever the targeted point ended up. The LM loop
terminates in that situation via its no-improvement break (total cost is at a
minimum); the iteration budget is the backstop.

Implementation notes the loop structure forces:

- In the pull phase, the early-exit test (`√cost ≤ tolerance`) runs on the
  *augmented* cost. With an unreachable target it never fires; the phase exits
  via the no-improvement break (the augmented minimum) and hands off to the
  polish phase.
- `Result.Residual` and `Converged` are computed from a **fresh hard-only
  `residuals()` call after both phases** — never from any phase's augmented
  cost.
- `Result.Iterations` is the sum across phases and does not distinguish "hit
  the iteration budget" from "no-improvement break"; each phase gets the full
  `WithMaxIterations` budget. Acceptable: the meaningful signal for a caller
  is hard-only `Converged`. Revisit only if an interactive consumer
  demonstrates the need.
- Goals never mask a broken sketch: if the hard constraints cannot be
  satisfied (over-constrained/contradictory), the polish phase fails the
  hard-only judgment and `Solve` returns `ErrNotConverged` exactly as it would
  without goals.

### Weight

Fixed `w = 1e-3`, unexported. The weight itself is dimensionless — the goal
residual already carries length units from `(p.x − tx)`. The `w² ≈ 10⁻⁶
relative` argument assumes hard residuals are O(1) in millimetres, i.e.
sketches of roughly unit-to-hundreds-of-mm extent; sub-millimetre or
metre-scale sketches shift that ratio. That is a known, accepted limitation:
revisit with adaptive scaling (by sketch extent) only if profiling or a real
consumer shows it matters. Not public API until then.

## Interactions with existing machinery

- **Driven dimensions** refresh after every solve, including goal solves —
  live measurement readout during a drag for free.
- **Parameters**: `ApplyParameters` runs per solve, as today. Cheap; revisit
  only if profiling says otherwise.
- **Concurrency**: unchanged — a `Sketch` is a mutable document and `Solve`
  mutates it; one goroutine per sketch. The UI layer serializes pointer
  events.

## Testing plan (no GUI needed)

- **Projection**: point constrained on a fixed line, goal off the line → point
  lands at the perpendicular projection of the target.
- **Constraints win**: fully dimensioned 20×12 rectangle, goal pulls a corner
  to (30, 30) → corner stays at (20, 12), `Converged` true, no error.
- **Tracking**: free (under-constrained) point follows a sequence of goal
  positions, each solve converging from the previous solution and landing on
  the target (within tolerance scaled by the soft weight).
- **Fixed point**: goal on a grounded point → geometry unchanged, no error.
- **No residue**: after a goal solve, `DOF`/`Redundant` match the no-goal
  values, a subsequent plain `Solve` does not move geometry, and the JSON
  output contains no trace of the goal (goals have no serialized form; solved
  coordinates agree with a never-dragged equivalent sketch to solver
  tolerance, though not necessarily bit-exactly).

## Deferred to the UI layer

Hit-testing, snapping, gesture policy for entity dragging, and any
`StartDrag`/`To`/`End` session convenience. A future GUI can build that handle
in its own package purely on `Solve(WithGoal(…))` — if it turns out every UI
rebuilds the identical thing, promoting a tiny helper into the engine is a
one-evening, backward-compatible addition. The reverse (removing a
wrong-shaped session API from the engine) is not.

## Open questions

- Should `Result` report goal attainment (achieved positions or per-goal
  residual) so rubber-band UIs can draw the "you asked / you got" gap without
  re-measuring? Cheap to add to `Result` later; omitted for now.
- Goals on non-point unknowns (a circle's radius) — `WithGoal` is
  point-specific; a radius goal would be a new option if a consumer ever wants
  rim-dragging to feel like radius-dragging. Deliberately not designed now.
