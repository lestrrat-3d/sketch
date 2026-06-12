# Multi-Solution Ambiguity Probe — Design

Status: **implemented** (`probe.go`; tests in `probe_test.go`). Resolves part
of the "Constraint diagnostics & UX" open question in `CLAUDE.md`: the
multi-solution / configuration-ambiguity signal that DOF analysis cannot give.

## The problem

A fully constrained sketch (DOF 0) can still admit several **discrete** valid
configurations: a triangle apex above or below its base, a tangent circle on
either side of a line, a mirrored corner. `DOF`, `Diagnose` and
`CheckConstraint` are blind to this — they analyze the continuous degrees of
freedom at one configuration. Which branch a solve realizes is decided by the
seed (the starting coordinates), so a sketch that "solves correctly" can flip
to a mirror configuration under a different seed. This is the dominant failure
mode for programmatically generated sketches: every unsigned constraint
(tangent, point–line distance, symmetric, …) leaves a branch open.

`ProbeConfigurations` is the diagnostic: it searches for alternative
configurations by re-solving from perturbed seeds, and reports the distinct
converged results. The intended workflow is lint-like — if the probe finds
more than one configuration, pin the intended branch with a signed constraint
(`NewAngle`, `NewOffset`, `NewHorizontalDistance`/`NewVerticalDistance`)
rather than relying on the seed.

## Falsifier, not certifier

The probe **proves ambiguity** when it finds ≥ 2 configurations; it can
**never prove uniqueness** — a result of 1 only means no alternative was found
within the probe budget. Every name and doc comment uses "configurations
found", never "all solutions". Exhaustive branch enumeration (homotopy
continuation, algebraic decomposition) could certify uniqueness but is far
outside the engine's numerical scale and dependency budget.

*Rejected alternative:* naming the entry point `ProbeSolutions` — "solutions"
implies the complete set, which a heuristic multi-start search cannot deliver.

## API

- `Sketch.ProbeConfigurations(options ...ProbeOption) (*ProbeResult, error)`,
  called after `Solve` like the other diagnostics; analysis is local to the
  call-time configuration and dimension targets.
- `ProbeResult.Configurations` — baseline (call-time, converged) first, then
  alternatives in deterministic probe order. `Ambiguous()` ⇔ length > 1.
- `Configuration` — a full variable-vector snapshot. `PointXY(p)` reads
  without touching the sketch; `Apply()` writes the snapshot back as seeds
  (the one sanctioned mutation), so callers can re-solve/export each branch.
- Options: `WithRestarts(n)` (default 12), `WithSeed(v)` (stream selector).
  No probe-level iteration/tolerance options — the solver defaults are used.
- Errors: `ErrNotConverged` when the baseline cannot be solved;
  `ErrUnderconstrained` (wrapped, with the DOF count) when DOF > 0.

*Rejected alternative:* returning a degraded result for under-constrained
sketches instead of an error. On a solution continuum every restart converges
somewhere slightly different and clustering would fabricate spurious
"configurations"; failing loudly steers the caller to constrain first.

## Mutation contract

The variable vector is snapshotted on entry and restored by a deferred copy on
every exit path. The probe calls the internal `lm` loop directly — never
`Solve` — so parameter bindings are not re-evaluated and `refreshDriven` never
writes a probed configuration's measurements into driven dimensions. Goals
stay invisible structurally (`WithGoal` is a `SolveOption`). Internal
constraints (arc radius consistency) participate via `residuals` like any
other constraint.

## Perturbation scheme

Every round: reset to baseline → perturb free variables only → `lm` → keep if
converged and distinct. Scale comes from the baseline bounding-box diagonal
(`s.bounds()`, fallback 1).

1. **Structured mirrors.** Reflect every free point coordinate across each
   candidate axis: the infinite lines through every line entity and through
   every pair of fixed points (capped at `probeMaxAxes`). Mirror branches
   reflect across constraint-defined axes — a triangle apex flips across the
   line through its fixed base points even when no line entity joins them.
   Flipping about the bounding-box center alone is not enough: it can land
   geometry exactly on the symmetry saddle (zero gradient), where the solver
   cannot leave the ridge.
2. **Global flips.** Free point coordinates reflected about the bounding-box
   center (x, y, and 180°) — catches mirrors not aligned with any axis from 1.
3. **Pseudo-random restarts** (default 12). Offsets from a hand-rolled
   splitmix64 stream keyed by (seed, restart, variable index); amplitude
   cycles ¼, ½, 1 × the bbox diagonal for multi-scale coverage. Radius
   variables stay positive (a negative radius fights `norm()`'s floor); the
   ellipse-rotation variable is offset within ±π instead of by length. Each
   restart perturbs from the baseline, never the previous restart, so rounds
   are independent of acceptance history.

Non-converged rounds are discarded silently — wild seeds failing to converge
is expected, not an error.

*Rejected alternative:* `math/rand` with a fixed source. The repo has zero
rand usage; a six-line self-contained generator makes the determinism
self-evident and independent of any random-stream stability guarantee across
Go versions.

## Clustering

A candidate is a duplicate iff its distance to **any** accepted configuration
is below `separationTol` (1e-6, relative). The metric is the max over free
variables of the per-variable relative separation: coordinates and radii
relative to the bbox diagonal; angle variables wrapped into (−π, π], folded by
π (a rotation of π maps an ellipse onto itself) and relative to π. Solver
noise sits below ~1e-8 relative while real branches differ at feature scale,
so the threshold has orders of magnitude of margin on both sides.

## Still open (recorded in CLAUDE.md)

- Options for the separation tolerance and the per-round solver budget (waits
  on the repo-wide per-sketch tolerance question).
- The ellipse rx/ry-swap ± π/2 self-symmetry is not folded by the metric (it
  couples three variables), so such pairs over-report as distinct.
- Reporting *which* entities differ between two configurations (the diff, not
  just the existence of a branch).
- Splines: control points are ordinary points and are probed as such; no
  spline-specific branch structure is exploited.
