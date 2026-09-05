# Solver elimination, phase A — result record

Completion record for the "solver elimination" proof-performance task
(`fusion360-gear-generator/docs/proof-optimization/solver-elimination.md`),
phase A only. Phase B (Cholesky/LDLT or another solver) was not started and
remains out of scope.

## What was tested

| Item | Value |
|---|---|
| Repository | `github.com/lestrrat-3d/sketch` |
| Worktree | `.worktrees/refactor-solver-elimination`, branch `refactor-solver-elimination` |
| Base commit | `840215e35ce8890e82051c91b9486d5760d36cfe` |
| Candidate commit | `8779e17` (this branch's single commit) |
| Gear generator | `e73ada38282c99f58d4a228bee870f68795258c9`, clean; `proof/go.mod` pins sketch `v0.0.0-20260905042130-840215e35ce8` |
| Toolchain | go1.26.8 linux/amd64 (`GOTOOLCHAIN=auto`) |
| Machine | AMD Ryzen 9 7900X3D, 24 hardware threads, default `GOMAXPROCS` |

The machine was shared with other agents' builds throughout (load average
around 13). Every timing below is a median over interleaved samples, with the
full sample range reported beside it.

## Investigation

The kernel was instrumented temporarily (never committed) and run over the
bevel proof case `^TestGearProfiles$/^shaft_angle_60$`:

| Measurement | Value |
|---|---|
| Calls to `solveLinearInto` | 4,940, every one of them 128×128 |
| Row multipliers formed | 40,145,237 |
| Multipliers exactly zero | 18,004,101 (44.8%) |
| Inner-loop element updates | 3,492,676,726 |
| Element updates in a zero-multiplier row | 1,813,417,909 (51.9%) |
| Calls whose working copy held a negative zero | 511 of 4,940 (10.3%) |
| Rows holding a negative zero | 1,116 of 632,320 (0.18%) |
| Pivots whose suffix held a NaN or infinity | 0 |

Contract facts the shortcut had to respect:

- The kernel's two callers are `Sketch.lm` (the damped normal equations) and
  `rowCombo` in `diagnose.go` (an upper-triangular Gram matrix). `Solve` does
  not screen geometry for non-finite values before calling `lm`, so a
  non-finite matrix can reach the kernel, and finite input can still overflow
  during elimination.
- On refusal the kernel returns before back substitution, so `x` is untouched.
  `TestSolveLinearEliminationRefusalLeavesXUnmodified` now asserts that rather
  than leaving it to be read off the control flow.
- Pivoting permutes the scratch matrix's ROWS, and `lm` reuses one scratch
  across every damping trial of every iteration. Every element is rewritten
  from A and b on entry, so nothing may be carried in those rows between calls
  — which is why the negative-zero verdict is recomputed per call rather than
  cached.

## What shipped

`solveLinearInto` skips a row whose elimination multiplier is exactly zero,
under two guards, and reads its two rows through slices taken once per row.

- **Pivot-suffix finiteness**, re-established once per pivot. Zero times an
  infinity is NaN, which the plain update writes and a skip would drop.
- **No negative zero in the working copy**, decided on the copy-in pass.
  Subtracting an exact zero is the identity on every float64 except -0, whose
  sign the subtrahend decides. Elimination cannot create a negative zero, so one
  scan settles the question for the whole call; one negative zero anywhere costs
  that call its shortcut.

The result is bit-identical to the previous elimination, which is the bar the
task set: the LM step this feeds decides which configuration a solve and every
`ProbeConfigurations` restart lands in, so an ulp of difference is not
acceptable. `solver_elimination_test.go` holds an unchanged private copy of the
old algorithm (`referenceSolveLinearInto`) and compares the success result,
every bit of `x`, every bit of the working matrix, and the untouched state of
the caller's A and b, over the task's regression matrix plus 200 deterministic
pseudo-random systems and the damped normal equations of three real solved
sketches.

## Prototypes measured and rejected

Measured by the flat CPU time of `solveLinearInto` in the bevel focused proof,
against a baseline of 2.84 s flat (67.3% of a 4.22 s sample):

| Variant | Flat time | Verdict |
|---|---|---|
| Unguarded `if f == 0 { continue }` (the handoff's prototype) | not timed | Rejected: wrong |
| Per-entry preservation — skip, but re-apply the update to entries that are themselves zero | 3.36 s | Rejected: a regression |
| Cached row slices alone, no skip | 2.81 s | Rejected on its own: within noise |
| Skip with the original per-element indexing | 2.21 s | Superseded |
| **Skip plus cached row slices (shipped)** | **1.05 s** | Accepted |

One profile sample per variant, taken in one session; the shipped kernel
re-profiled at 1.38 s later, and the focused-case timings below are the
five-sample evidence the acceptance rests on.

The unguarded prototype diverges from the reference on 8 of 24 hazard fixtures,
6 of them in `x` — the negative-zero and overflow cases the task file predicted.

The per-entry variant is what the task file proposed as the conservative
starting point. It is correct, and it is slower than doing nothing. In a sparse
matrix most entries of a zero-multiplier row are themselves zero, so the
per-entry test still performs most of the updates and adds a branch to each: the
skip's value is that it avoids a row's whole memory traffic, and a variant that
still reads every entry gives that up.

A caution on the microbenchmark: two byte-identical kernels compiled into the
same binary differed by about 2%, but structurally similar variants differed by
up to 33% purely from where the compiler placed and padded their inner loops.
Cross-variant microbenchmark deltas were therefore treated as weak evidence, and
the choice between candidates was made on the real proof's CPU profile.

## Kernel benchmark

`BenchmarkSolveLinearElimination`, 7 interleaved samples at `-benchtime=0.3s`,
medians in ns/op. Zero allocations on both paths.

| Fixture | Reference | Candidate | Change |
|---|---:|---:|---:|
| banded normal equations, 32 | 9,491 | 4,574 | -51.8% |
| banded normal equations, 128 | 477,022 | 157,187 | -67.0% |
| banded normal equations, 256 | 3,713,574 | 1,054,476 | -71.6% |
| banded with a negative zero, 32 | 8,649 | 5,296 | -38.8% |
| banded with a negative zero, 128 | 471,606 | 254,492 | -46.0% |
| banded with a negative zero, 256 | 3,740,805 | 1,923,791 | -48.6% |
| dense, 32 | 8,656 | 6,263 | -27.6% |
| dense, 128 | 487,980 | 288,829 | -40.8% |
| dense, 256 | 3,898,399 | 2,100,033 | -46.1% |

The banded fixture is shaped to the measured bevel structure — 128 variables,
46.5% of its multipliers zero against the 44.8% measured — and
`TestSolveLinearEliminationBenchmarkSystemsAreRepresentative` fails if it drifts
outside that window. The dense fixture has no zero multiplier at all, so it
measures what the guards cost where the shortcut never fires; there is no
regression on it.

## Validation

| Command | Result |
|---|---|
| `go test -count=1 -run 'Solve\|Solver\|Probe\|NonFinite\|NormalEquations\|Elimination' ./...` | pass, 193 test/subtest runs, the five new `TestSolveLinearElimination…` among them |
| `go test -count=1 ./...` | pass, all packages |
| `go vet ./...` | clean |
| `golangci-lint run` | 0 issues |
| `gofmt -l` | clean |

## Proof measurements

Focused cases, five interleaved samples of the pinned baseline and of the
candidate through a temporary `candidate.mod` replace, medians in seconds:

| Case | Baseline | Range | Candidate | Range | Change |
|---|---:|---|---:|---|---:|
| `bevelgear ^TestGearProfiles$/^shaft_angle_60$` | 3.59 | 3.49–3.80 | 2.41 | 2.37–2.53 | -32.9% |
| `helicalgear ^TestTwistedGearProfile$/^default_M1_N17_helix14.5$` | 3.23 | 3.20–3.40 | 2.95 | 2.93–3.05 | -8.7% |

CPU profile of the bevel focused case: `solveLinearInto` falls from 2.84 s flat
(67.3% of a 4.22 s sample) to 1.38 s flat (53.3% of a 2.59 s sample).

The complete proof suite was run three times for each side
(`go test -mod=readonly -count=1 -json ./...`, against
`-modfile .tmp/candidate.mod` for the candidate). All six runs exited 0 with
empty stderr, and all three pairings compare identical: 539 test events, 525
passing and 14 skipped, the same 9 passing and 1 skipped packages, with no
missing, added, or changed verdict. Per-package medians of the three rounds, in
seconds:

| Package | Baseline | Range | Candidate | Range | Change |
|---|---:|---|---:|---|---:|
| bevelgear | 131.35 | 126.88–131.73 | 113.28 | 111.86–117.06 | -13.8% |
| spurgear | 61.13 | 60.58–63.71 | 51.24 | 51.12–53.43 | -16.2% |
| cycloidal | 49.40 | 48.90–50.78 | 47.89 | 47.23–49.81 | -3.1% |
| helicalgear | 45.50 | 45.03–46.52 | 40.66 | 40.63–42.41 | -10.6% |
| herringbonegear | 36.69 | 36.29–37.23 | 28.95 | 28.64–29.75 | -21.1% |

The ranges are disjoint on every package except cycloidal, whose 3.1% sits
inside the noise and should not be claimed.

Allocations do not move. The kernel itself is pinned at zero allocations on a
caller-supplied scratch by `TestSolveLinearEliminationNoAllocation`, and at
proof level the dominant allocator is `lmWorkspace.alloc`, unchanged code, whose
sampled totals over the focused bevel case were 40.74/40.77/40.46 MB on the
baseline against 47.73/36.33 MB on the candidate — a spread that straddles the
baseline rather than a shift.

The dependency-pin integration (`go get` for the upstream commit, the tracked
`go.mod`/`go.sum` bump, `bash proof/run_test.sh`) was NOT performed: it needs a
pushed commit, and this task was scoped to local work with no push and no PR.
The candidate was exercised only through the temporary `candidate.mod` replace,
which is what the protocol prescribes before a pin changes.

## Open points

- The negative-zero guard is whole-matrix, so one such entry costs a call its
  shortcut. In the profiled workload that was 10.3% of calls; a per-row verdict
  would recover them but needs storage the kernel does not have, which would
  mean a scratch parameter the callers must supply. Left as a possible follow-on
  rather than folded in here.
- Phase B (Cholesky/LDLT or another solver) is untouched, and its admissible
  matrix class, fallback, residual checks and acceptable externally visible
  differences remain unspecified.
- `rankAnalysisOfMatrix` and `movableVars` run their own partial-pivot
  eliminations and were left alone; phase A's scope was the linear-system
  kernel.
