# `sketchtest` — Design

Status: **implemented** (`sketchtest/`).

## Purpose

`sketchtest` is the test kit for callers of `sketch`. It turns the package's
multi-field solve, verification, profile, and geometry contracts into failures
that name the failed condition and the observed values.

The package follows `decadtest`'s test-helper shape without copying its solid-
model concepts:

- Every helper takes `testing.TB` first, calls `tb.Helper()`, and reports a
  failed assertion with `tb.Fatalf`.
- Helpers return useful successful results so one call can both assert and
  continue the test.
- Production files import no assertion framework. Only `sketchtest`'s own
  external tests use `testify/require`.
- Failure messages are owned in one formatting file.

The import path is:

```go
import "github.com/lestrrat-3d/sketch/sketchtest"
```

`sketchtest` imports `sketch`, `r3`, `option`, and the standard library. It
never imports `decad`, so it does not reverse the `decad -> sketch` dependency.

## Boundaries

`sketchtest` asserts public `sketch` contracts. It does not expose package
internals or duplicate engine decisions.

- `VerificationReport.Check` remains the only definition of a trustworthy
  report. Helpers consume its reasons; they do not rebuild the condition list.
- `VerificationReport.Analysed` remains the guard for fields that hold ambiguous
  zero values when verification stops early.
- `Profile.Valid`, `Profile.IsStale`, and `BoundaryEdge.TExact` remain separate
  facts. No helper combines them into a new validity rule.
- `ConstraintResiduals` remains the source of residual values.
- Pointer identity remains the identity of points, constraints, and entities.

The package does not hide the sketch lifecycle. `Solve` mutates geometry;
`Verify` and every assertion after it are read-only. `IsTrustworthy` verifies
the current configuration and never solves it first.

## API

### Execution

```go
func Solve(
    tb testing.TB,
    s *sketch.Sketch,
    opts ...sketch.SolveOption,
) *sketch.Result

func Verify(
    tb testing.TB,
    s *sketch.Sketch,
    opts ...sketch.VerifyOption,
) *sketch.VerificationReport

func IsTrustworthy(
    tb testing.TB,
    s *sketch.Sketch,
    opts ...sketch.VerifyOption,
) *sketch.VerificationReport
```

`Solve` passes `tb.Context()` and the options to `Sketch.Solve`. It fails on a
nil sketch or any returned error, including `ErrNotConverged` and context
cancellation. It returns the successful `Result` unchanged.

`Verify` fails only on a nil sketch, then returns `Sketch.Verify(tb.Context(),
opts...)`. An untrustworthy report is a successful result from this helper so a
test can inspect its reasons.

`IsTrustworthy` calls `Verify`, then fails unless `report.Check()` is nil. It
prints every reason from `Check`, in the order supplied by the report, and
returns the report on success.

`Sketch.WithTolerance` implements both `SolveOption` and `VerifyOption`. A test
that overrides the tolerance creates it once and passes the same value to
`Solve` and `Verify` or `IsTrustworthy`.

### Verification reports

```go
func HasStatus(
    tb testing.TB,
    report *sketch.VerificationReport,
    want sketch.Status,
)

func HasDOF(tb testing.TB, report *sketch.VerificationReport, want int)

func FindReasons(
    tb testing.TB,
    report *sketch.VerificationReport,
    sentinel error,
) []error

func HasOnlyReasons(
    tb testing.TB,
    report *sketch.VerificationReport,
    allowed ...error,
)

func FindConflict(
    tb testing.TB,
    report *sketch.VerificationReport,
    constraint sketch.Constraint,
) *sketch.ConflictSet

func HasFreePoints(
    tb testing.TB,
    report *sketch.VerificationReport,
    want ...*sketch.Point,
)
```

`HasStatus`, `HasDOF`, `FindConflict`, and `HasFreePoints` first require an
analysed report. They fail with the report's `Check` reasons when analysis was
skipped instead of treating a zero-value field as a verdict.

`FindReasons` ranges over `report.Check().Unwrap()` and returns every reason for
which `errors.Is(reason, sentinel)` is true. It fails when none match. This
preserves the detail error that wraps the sentinel.

`HasOnlyReasons` fails when any actual reason matches none of the allowed
sentinels. Passing no allowed sentinels requires `Check()` to return nil. It
does not require every allowed sentinel to appear; the name means the report
contains no other reason.

`FindConflict` matches the report's `ConflictSet.Constraint` by exact interface
identity. `HasFreePoints` compares exact point membership without making slice
order part of the assertion; direct field comparison remains available when a
test specifically covers the documented ID order.

### Profiles

```go
func SingleProfile(
    tb testing.TB,
    report *sketch.VerificationReport,
) *sketch.Profile

func IsValidProfile(tb testing.TB, profile *sketch.Profile)
func IsCurrentProfile(tb testing.TB, profile *sketch.Profile)
func HasExactCuts(tb testing.TB, profile *sketch.Profile)
```

`SingleProfile` first requires an analysed report, then requires exactly one
entry in `report.Profiles`. It does not require `ProfilesValid` or
`Profile.Valid`; tests for invalid profile behavior must still be able to fetch
the profile.

`IsValidProfile` asserts only `profile.Valid`. `IsCurrentProfile` asserts only
`!profile.IsStale()`.

`HasExactCuts` checks every outer and hole edge whose `Partial` field is true
and fails when any such edge has `TExact == false`. Whole edges need no trim,
so their `TExact` value does not affect this helper.

These assertions stay separate because they answer different questions. A
valid profile can be stale, and a valid current profile can carry an
approximate cut that a structural downstream consumer must reject.

### Numeric geometry

```go
type Option interface {
    option.Interface
    sketchtestOption()
}

func Within(abs float64) Option
func WithinRel(rel float64) Option

func Measures(
    tb testing.TB,
    what string,
    got, want float64,
    opts ...Option,
)

func MeasuresPoint(
    tb testing.TB,
    point *sketch.Point,
    x, y float64,
    opts ...Option,
)

func MeasuresWorldPoint(
    tb testing.TB,
    point *sketch.Point,
    want r3.Vec,
    opts ...Option,
)

func MeasuresProfileArea(
    tb testing.TB,
    profile *sketch.Profile,
    want float64,
    opts ...Option,
)

func Satisfies(
    tb testing.TB,
    constraint sketch.Constraint,
    opts ...Option,
)
```

Unlike `decad.Measurement`, a `sketch` numeric read carries no proven error
bound. `sketchtest` therefore cannot add a library-owned bound to the author's
oracle. Every numeric call requires either `Within` or `WithinRel`; omitting
both is a test-author error. A later option replaces an earlier option because
they are alternate tolerance forms.

`Within(abs)` accepts `abs(got-want) <= abs`. `WithinRel(rel)` accepts
`abs(got-want) <= rel*abs(want)`. Both values must be finite and non-negative.
A zero expected value normally needs `Within`, since a relative tolerance then
resolves to zero. Non-finite observed or expected values always fail.

Numeric values follow the public `sketch` read surface: local coordinates and
lengths are base-unit millimetres, angles are radians, and profile areas are
square millimetres. The options use the same scalar unit as the compared
values. They do not take `units.Value`, because that would imply kind
information that the observed `float64` does not carry.

`MeasuresPoint` applies one tolerance to X and Y and names the failed
coordinate. `MeasuresWorldPoint` does the same for X, Y, and Z.
`MeasuresProfileArea` names the profile area. `Satisfies` obtains
`sketch.ConstraintResiduals` and requires every residual to measure zero. Its
constraint must come from `Sketch.Constraints`, matching
`ConstraintResiduals`' public safety contract.

## Failure messages

Every exported helper checks its direct pointer arguments for nil. A nil
interface is rejected; a typed-nil value follows Go's normal method behavior
and is not detected with reflection.

Messages follow these forms:

```text
sketchtest.Solve: s must not be nil
solve: Sketch.Solve failed: sketch: constraint solver did not converge
verify: report is not trustworthy:
  [1] sketch: not fully constrained: underconstrained (DOF 2)
point[3] x: got 12.004, want 12, difference 0.004 exceeds absolute tolerance 0.001
profile outer edge[2]: partial boundary has approximate parameters
```

A named point renders as `point[3] "tip"`; an unnamed point renders as
`point[3]`. A profile passed to a profile helper renders as `profile`.

Formatting code decides only how a value is displayed. Assertion files decide
pass or fail.

## Package layout

| File | Responsibility |
|---|---|
| `doc.go` | Package contract and boundaries |
| `options.go` | Numeric comparison options |
| `numbers.go` | Scalar, point, world-point, area, and residual comparisons |
| `solve.go` | `Solve` |
| `verify.go` | Verification, reason, conflict, and free-point assertions |
| `profiles.go` | Profile assertions |
| `format.go` | All failure-message formatting |

Files may be combined while small. They split at these responsibility lines
rather than by implementation order.

## Tests

`TestRectangleFlow` exercises one real caller flow using only public APIs. It
creates a rectangle in a real `sketch.World`, constrains it to `Sketch.Origin`,
solves it, verifies the trustworthy report, checks a solved corner, and checks
the returned profile's validity, freshness, and exact cuts.

Focused tests use a recording `testing.TB`, following `decadtest`'s pattern, to
prove failure behavior. They cover nil inputs, skipped verification analysis,
missing and extra reasons, pointer-identity mismatches, stale profiles,
approximate partial edges, non-finite numbers, absent numeric options, and
wrong tolerances.

Tests use `package sketchtest_test`. No `examples/` entry is added: an
`Example*` function has no `testing.TB` to pass to any helper, so package docs
and executable tests are the usage record.

## Deferred

The first release does not include:

- fixture builders for rectangles, slots, splines, or intentionally broken
  sketches;
- golden-file update or exporter comparison helpers;
- wrappers for every `VerificationReport`, `Result`, entity, or constraint
  field;
- automatic reason waivers inside `IsTrustworthy`;
- helpers that apply probe configurations or otherwise mutate a sketch beyond
  `Solve`;
- helpers for `geom`, whose pure values need no solver or verification
  lifecycle.

Add a helper after at least two external tests repeat the same public-contract
assertion and need the same failure message. Direct field checks remain the
right choice for one-off behavior.
