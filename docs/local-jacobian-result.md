# Local residual Jacobian — result record

Completion record for the "local Jacobians" proof-performance task
(`fusion360-gear-generator/docs/proof-optimization/local-jacobians.md`). The
first implementation is complete. The rank/conditioning Jacobian
(`scaledJacobian`, `conditioning.go`) is deliberately untouched, and analytic
derivatives remain a separate later design, both as the task file requires.

## What was tested

| Item | Value |
|---|---|
| Repository | `github.com/lestrrat-3d/sketch` |
| Worktree | `.worktrees/refactor-local-residual-jacobian`, branch `refactor-local-residual-jacobian` |
| Base commit | `e283d1b` — "skip zero-multiplier rows in the elimination (#112)", the merged solver-elimination result |
| Gear generator | `e73ada38282c99f58d4a228bee870f68795258c9`, clean |
| Gear worktree | `fusion360-gear-generator/.worktrees/perf-local-residual-jacobian` |
| Toolchain | go1.26.1 linux/amd64 |
| Machine | AMD Ryzen 9 7900X3D, 24 hardware threads, default `GOMAXPROCS` |

The proof pins sketch at `840215e`, one commit behind the base used here. Both
sides of every comparison replace sketch with a source directory — the baseline
with the root checkout at `e283d1b`, the candidate with this worktree — so the
elimination change is present in both and only this change differs. `decad`,
`solidlens`, `r3` and `units` stay at their recorded pins on both sides.

The machine was shared with other agents' builds throughout (load average 12–15).
Every timing below is a median over samples interleaved in both orders, with the
full range beside it.

## Mechanism

`Sketch.jacobianInto` reevaluates the whole residual vector twice per free
variable. A constraint's residual reads only the variables of the geometry it
references, so most of those rows are recomputed from unchanged inputs.

`jacobian.go` adds a `residualPlan` — built once per `Sketch.lm` call — holding
each committed constraint's row offset and count plus a compressed
variable→constraint index, and `Sketch.jacobianLocalInto`, which perturbs a
variable and reevaluates only the constraints that variable reaches. Everything
else in the column is cleared.

### Why the result is bit-identical

A row whose constraint does not read the perturbed variable is computed from
identical inputs by identical code at both perturbations, so `rp[i]` and `rm[i]`
are the same float64 and their difference is `+0`; `inv` is positive (or `+0`
when the step overflows), and `+0` times either is `+0`. That is exactly what
the cleared entry holds. Affected rows are evaluated with the same arithmetic on
the same values in the same order, at a different position in a buffer, which
changes none of them. The step size, variable order, row order, ±h evaluation
order and exact bit-pattern restoration are the dense pass's, unchanged.

The argument needs unaffected rows to be finite, so the builder scans the
caller's base residual vector and refuses the whole call on a NaN or infinity:
there the dense pass computes `∞ − ∞` or `NaN − NaN`, and clearing would replace
a NaN a structurally independent row genuinely produces.

### Where it refuses

Every refusal is whole-call and falls through to the dense build, so the mode
selects an optimization and never a different result:

- A constraint `constraintRefs` (`removal.go`) does not list — the shape a new
  constraint type added without its removal-cascade case takes. An empty
  dependency set would zero every entry of a real row, so "names nothing" is
  never read as "reads nothing".
- An operand this sketch does not own (nil, typed-nil, removed, foreign),
  screened with `owns`/`ownsEntity`, and auxiliary variables another sketch
  allocated (`auxOwnerOf` naming another sketch while `auxVars` reports live
  indices).
- A residual whose row count differs from the plan's record. The variable is
  restored to its exact original bit pattern first, on both the +h and the −h
  side.
- A residual vector the plan does not describe, which is what catches the
  goal-augmented system (`goalResiduals`, two extra rows per goal).

### Where the mode is passed

`Sketch.lm` takes an explicit private `jacobianMode`. `Sketch.Solve`'s polish
phase and `Sketch.probeConfigurations` pass `localJacobian`; the goal phase
passes `denseJacobian`. The evaluator is never recognized by comparing Go
function values.

`ProbeConfigurations` was split so its body takes the mode as a parameter. That
is what lets a test drive the same multi-start search both ways and compare the
configurations bit for bit, and the probe is where the gear proofs spend most of
their solver time.

## Dependency inventory

`jacobian_inventory_test.go` carries the checked-in table required by the task
file: 57 constraint kinds (every public `New…` constructor plus the two internal
constraints), each with its residual row count, the distinct point / shape / aux
variable counts its residual reaches, whether those are static or aux-gated, and
whether the local evaluator supports it. **All 57 are supported**; none routes to
the dense fallback.

The list is anchored against `constraint.go`'s constructor declarations by
`TestConstraintDependencyTableCoversEveryKind`, so a constructor added later
fails the suite until it is classified.

`TestConstraintResidualsIgnoreUnlistedVariables` is the load-bearing half: for
every kind, it perturbs every variable the dependency set does NOT name — at
three step sizes, on a sketch carrying unrelated geometry — and requires the
constraint's residual rows to come back bit-identical. It also requires that
perturbing a listed variable does move a row, so a fixture cannot pass by being
insensitive to everything.

## Correctness evidence

| Check | Result |
|---|---|
| `gofmt -l .` | clean |
| `go vet ./...` | clean |
| `go test ./... -count=1` | all packages pass |
| `golangci-lint run --timeout=5m` (v2.12.2, the CI version) | 0 issues |
| Gear proof verdict comparison | identical |

The verdict comparison ran the full proof suite both ways
(`go test -mod=mod -modfile … -count=1 -json ./...`) and compared by
`(Package, Test)`: **539 test/subtest events on each side, 525 pass and 14 skip,
no missing test, no added skip, no new failure, no extra test, and identical
package-level outcomes.**

New tests, all comparing against the dense builder rather than against a
tolerance:

- `TestLocalJacobianMatchesDense` and `…AtPerturbedStates` compare every
  Jacobian entry by `math.Float64bits` over six fixtures (shared points, shape
  variables, splines and conics, ellipse axes and rotation, a driven dimension
  contributing zero rows, extreme finite scales), as authored, mid-solve, at the
  solved configuration, and through five deterministic displaced states each.
- `TestLocalSolveMatchesDenseSolve` runs the same `lm` loop both ways on
  identical sketches and requires bit-identical variable vectors, including
  after geometry is added and removed between solves.
- `TestLocalSolveMatchesDenseAcrossParameterEdits` does the same across bound
  parameter edits.
- `TestProbeMatchesDenseProbe` drives the same multi-start search both ways over
  three DOF-0 fixtures and requires the same configurations, in the same order,
  bit for bit.
- `TestLocalJacobianRefusesNonFiniteResiduals`, `…RefusesForeignEvaluatorShapes`,
  `…RefusesUnclassifiedConstraint`, `…RestoresVariableOnRowCountMismatch` (both
  perturbation sides) and `…ClearsStaleWorkspaceEntries` pin the refusal paths,
  the restoration and the explicit clear.
- `TestResidualPlanCutsEvaluationWork` is the work assertion, expressed as a
  residual-row count rather than a wall clock.

## Timings

### Kernel (in-package benchmark, `BenchmarkJacobianBuild`)

One Jacobian build on a solved chain of rectangles, median of 3 × 500 iterations:

| Fixture | Dense | Local | Plan build |
|---|---:|---:|---:|
| 4 rectangles | 15.0 µs, 12 allocs | 1.9 µs, 0 allocs | 4.5 µs, 62 allocs |
| 40 rectangles | 1421 µs, 20 allocs | 31.7 µs, 0 allocs | 41 µs, 462 allocs |

The local build allocates nothing: its row scratch and the plan's index live for
the whole `lm` call. The plan is built once per call, so at 40 rectangles it is
repaid within the first iteration and at 4 rectangles within the second. Its
remaining allocations are `constraintRefs`'s own per-constraint slices, which
were left alone rather than duplicated into a second dependency switch.

### Gear proofs

Isolated package runs, alternated in both orders, three samples each:

| Package | Baseline median (range) | Candidate median (range) | Delta |
|---|---:|---:|---:|
| `./helicalgear` focused case | 3.456 s (3.336–3.542) | 1.606 s (1.600–1.633) | −53% |
| `./bevelgear` focused case | 2.852 s (2.700–2.856) | 1.536 s (1.528–1.595) | −46% |
| `./bevelgear` (whole package) | 105.9 s (105.8–107.8) | 92.1 s (90.9–99.8) | −13% |
| `./spurgear` | 46.3 s (46.3–50.2) | 45.4 s (39.2–45.8) | −13% at the median of the run pairs |
| `./herringbonegear` | 26.0 s (25.6–28.2) | 21.2 s (20.7–21.4) | −18% |
| `./cycloidal` | 45.9 s (43.2–48.0) | 43.3 s (42.1–46.3) | ranges overlap; no measurable change |

The focused cases are the ones the task file names:
`^TestTwistedGearProfile$/^default_M1_N17_helix14.5$` and
`^TestGearProfiles$/^shaft_angle_60$`.

Full-suite runs (packages in parallel, so per-package figures there are
scheduling-sensitive): wall clock 2m05.6s → 2m03.3s, user CPU 6m18.9s → 5m44.8s
(−9%). `bevelgear` is the long pole at ~108 s and holds the wall clock.

Profiler evidence on the helical/herringbone shape: before the change,
`jacobianInto` was 32.6% of the herringbone profile's CPU, all of it under
`ProbeConfigurations`. After it, the plan carries that path and the residual
evaluation drops out of the top of the profile.

## An observed measurement hazard

Adding this file to the package moved `solveLinearInto` from 8.31 s to 13.67 s
in the herringbone profile — a 64% slowdown of a function whose source is
untouched — while the plan was not yet enabled on the probe path and therefore
did nothing in that package at all. Two controls confirmed it: forcing
`denseJacobian` at the Solve call site, and removing the `jacobianLocalInto`
call from `lm` entirely, both left the regression in place, and the isolated
`BenchmarkSolveLinearElimination` showed no consistent difference between the
two binaries.

It is a link-layout effect on an alignment-sensitive hot loop, the same
sensitivity the elimination task already measured (a loop-layout change alone
moved that kernel by 2×). It is worth knowing for the next measurement on this
package: **a whole-package gear timing can move ±15% from an unrelated source
change, so a small delta there is not evidence either way.** The deltas reported
above are large enough, and repeat in both interleave orders, to sit clear of it.

## Not done, and why

- The rank/conditioning Jacobian (`scaledJacobian`) still reevaluates every row
  per variable. The task file scopes it out of this change, and it uses a
  different step, different scaling and an augmented candidate-aware system.
- Analytic derivatives remain a separate later design.
- Plan construction still allocates one or two slices per constraint through
  `constraintRefs`. Removing them means a second, non-allocating dependency
  switch beside the one the removal cascade already owns, which is exactly the
  duplication the inventory's soundness rests on not having. It is not on any
  hot path: the plan is built once per `lm` call, not once per iteration.
