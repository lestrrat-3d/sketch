# Lean proofs

`_proofs/` holds machine-checked Lean 4 proofs about the sketch engine, plus a Go checker that fails when a
covered Go function changes without its theorem being re-read. The proofs are about Lean models of Go functions
under the assumptions table below. They are not proofs about compiled Go.

Covered so far: soundness of the outward-rounded interval helpers in `interval.go`
(`_proofs/Sketch/Interval/Arith.lean`) over the abstract binary64 model `_proofs/Sketch/Float/Model.lean`; the
zero sets of the angle and distance enclosure equations (`_proofs/Sketch/Enclose/Angle.lean`); the accepting
branch of the parametric Krawczyk test (`_proofs/Sketch/Enclose/Krawczyk.lean`); the certified sine and
cosine enclosures of `interval_trig.go` (`_proofs/Sketch/Trig/`), including a 40-digit bracket of π; and the
line and fitted-spline paths of `Sketch.CertifyConeCurveSide` in `cone_side.go` (`_proofs/Sketch/Cone/`).
`_proofs/trace.json` lists every covered Go function.

| Question | Section |
|---|---|
| Where files live | Directory layout |
| Lean/Mathlib pins, options, file conventions | Lean project |
| What the proofs assume and do NOT prove | Assumptions |
| Manifest format, hash, checker | Trace manifest, Checker |
| CI workflow | CI workflow |
| Adding a theorem for a new Go function | Recipe: cover a new Go function |
| Upgrading Lean or Mathlib | Upgrade Lean and Mathlib |
| Running everything locally | Run locally |

## Directory layout

```
_proofs/
  lakefile.toml          Lake config: package, lean_lib Sketch, leanOptions, Mathlib require
  lean-toolchain         leanprover/lean4:v4.36.0, equal to Mathlib's own at the pinned commit
  lake-manifest.json     exact commit of Mathlib and every transitive Lean package
  Sketch.lean            root module; `lake exe mk_all` writes it from every Sketch/**.lean
  Sketch/Cone/*.lean         cone_side.go: exact intervals, box and line tests, Bézier subdivision, fit spans, entry
  Sketch/Enclose/Angle.lean zero sets of the angle and distance enclosure equations
  Sketch/Enclose/Krawczyk.lean soundness of the accepting branch of the Krawczyk test
  Sketch/Float/Model.lean    FloatModel: abstract finite binary64 (rn, nu, nd, δ) + rounding lemmas
  Sketch/Float/Grid.lean     gridModel: proves FloatModel's fields jointly satisfiable
  Sketch/Interval/Arith.lean models + soundness theorems for interval.go's helpers
  Sketch/Trig/*.lean         interval_trig.go: π bracket, Taylor tail, fixed-point series, reduction, range
  go.mod                 module github.com/lestrrat-3d/sketch/_proofs; standard library only
  trace.json             theorem to Go function manifest
  tracecheck/            package main: manifest checker and its tests
```

- New area → new directory `Sketch/<Area>/`. One topic per file.
- `_proofs` starts with `_`, so the root module, `go vet ./...`, `golangci-lint` and `go list ./...` never see it.
  `.lake/` starts with `.`, so `go list ./...` inside `_proofs` skips it too.
- `.gitignore` covers `/_proofs/.lake/`. NEVER commit `.lake/`. A checkout that builds the proofs holds about
  7 GB there; remove `_proofs/.lake` when done.
- `_proofs/` is a nested Go module, so root `go mod tidy` ignores it. Run `go mod tidy` in `_proofs` after
  adding a Go import there (none today).

## Lean project

### Pinning

- `lean-toolchain` MUST equal `.lake/packages/mathlib/lean-toolchain`.
- `lakefile.toml` requires Mathlib by tag. `lake-manifest.json` pins the commit. `lake build` never updates
  the manifest.

### Build is warning-clean

- ALWAYS build with `lake build --wfail`. CI passes `--wfail` too. It fails on every warning, including
  `declaration uses sorry`.
- Fix a deprecation by using the replacement the warning names. NEVER silence it.
- Remove an unused hypothesis. NEVER prefix it with `_`.
- NEVER add `set_option ... false` to a file to silence a linter. A linter that is wrong for this project is
  disabled once in `lakefile.toml` with a comment. `lakefile.toml` disables `linter.style.header` (Mathlib's
  copyright block rule) and `linter.style.longLine` (Mathlib's 100-column rule).

### File conventions

Every file:

```lean
module

public import Mathlib            -- or public import Sketch.<...>

/-!
# <Title>

<What this file models, one paragraph. Name the Go functions by package-qualified name.>
-/

public section

namespace Sketch.<Area>
...
end Sketch.<Area>

end
```

- `module` + `public import` + `public section` are required. Without them Lake warns that `Mathlib` is
  designed for the module system, and `--wfail` fails on it.
- A model definition another file unfolds MUST be `@[expose]`.
- The `/-! # Title -/` docstring is required by Mathlib's header linter.
- NEVER write `axiom`, `sorry`, `admit` or `native_decide`. CI's axiom audit fails on all four.
- Every assumption is a hypothesis of a theorem or a field of a model structure.
- `lake exe mk_all` rewrites `Sketch.lean` with the header `module  -- shake: keep-all --deprecated_module: ignore`.
  Keep that line as it writes it; `lake exe mk_all --check` compares against it.

### Naming and docstrings

- Defs are `lowerCamelCase`; theorems are `snake_case` embedding def names. A Lean def mirroring Go
  `fooBar` is `fooBar`.
- A def mirroring a Go function has a docstring ``Model of `pkg.Func`.`` plus any deliberate difference.
- A top-level theorem docstring states the guarantee in words, the Go functions it covers by package-qualified
  name, and the assumption ids its hypotheses carry. File paths and hashes live only in `trace.json`.

## Assumptions

The proofs do NOT prove the rows below. A change that breaks a row can make a proved bound false while every
theorem still builds.

| Id | Assumption | Lean form | Go evidence | Covered Go entries |
|---|---|---|---|---|
| A1 | Float `+`, `-`, `*` on finite float64 operands return the round-to-nearest value of the exact real result (IEEE 754 binary64) | `FloatModel.rn`, `rn_mem`, `rn_near` | Go spec "Arithmetic operators" (IEEE 754 float64) | `interval.go` `iadd`, `isub`, `imul`, `imulPt`, `isqr` |
| A2 | No NaN, no infinity, no overflow: every interval endpoint and every rounded intermediate on the modeled path is a finite float. `±Inf` results are not modeled | `FloatModel.F` holds only finite floats; `rn_mem`, `nu_mem` always land in `F` | The certified path refuses a residual or Krawczyk image with a non-finite or unordered endpoint (`Interval.isFinite` in `enclose_system.go`, `enclose_fold.go`); a fixed box must pass it too (`enclose.go`). Outside the model, a rounded `±Inf` endpoint only widens the interval | `interval.go` `down`, `up`, `iadd`, `isub`, `imul`, `imulPt`, `isqr`, `isqrtNonNeg`, `Interval.isFinite` |
| A3 | On a finite input, `up(x)` returns the least float above `x` and `down(x)` the greatest float below `x`, equal to `-up(-x)`: `Float64bits` ±1, `±math.SmallestNonzeroFloat64` at zero | `FloatModel.nu`, `nu_mem`, `lt_nu`, `nu_min`; `FloatModel.nd f = -nu (-f)`, `neg_mem` | `TestDownUpMatchNextafter` (`interval_internal_test.go`) against `math.Nextafter` and `-up(-x)` | `interval.go` `down`, `up`, `iadd`, `isub`, `imul`, `imulPt`, `isqr`, `isqrtNonNeg` |
| A4 | Each product written as `float64(x*y)` is rounded once: the explicit conversion forbids fusing it into an FMA | each product is one `M.rn (x * y)` | Go spec "Floating-point operators" (explicit conversion rounds). Dated check (Go 1.27.3, `GOARCH=arm64 go build -gcflags=-S .`): every `FMADDD`/`FMSUBD`/`FNMADDD`/`FNMSUBD` attributed to `interval.go` is on `Interval.mid`'s line, none in a covered helper | `interval.go` `imul`, `imulPt`, `isqr` |
| A5 | `math.Sqrt(x)` on finite `x ≥ 0` returns the round-to-nearest value of `√x` | `M.rn (Real.sqrt x)` in `isqrtNonNeg` | `math/sqrt.go` "Return correctly rounded sqrt"; hardware square root is IEEE 754 correctly rounded | `interval.go` `isqrtNonNeg` |
| A6 | On finite float64 values, negation is exact and `<`, `<=`, `>=`, built-in `min`/`max` agree with the real operations (signed zero aside) | real `-`, `<`, `≤`, `min`, `max` | Go spec "Comparison operators", "Min and max"; IEEE 754 negation flips the sign bit | `interval.go` `ineg`, `imul`, `imulPt`, `isqr`, `isqrtNonNeg`, `ihull`, `Interval.strictlyInside` |
| A10 | Each certified equation `F(·, p)` is differentiable at every point of the box `X`, for every parameter value `p`, and its Jacobian is the matrix of the closed-form partials each `certEquation.jac` writes | `KrawczykRun.Assumptions.deriv` | Code reading: each `certEquation.eval` is a polynomial in the coordinates (coordinate differences, cross and dot products, squared distance) with the parameter's value or sine and cosine as constants, and its `jac` writes that polynomial's partials (`writeVecPartials`) | `enclose_system.go certSystem.jacobianInto`, `certSystem.krawczyk` |
| A11 | For every parameter value `p` in `Q` (the driving value with every ranged target and fixed box), the interval `c[i]` that `krawczyk` computes contains row `i` of `−Y·F(x̃, p)`: `evalInto` over `base` encloses every equation, `trig` encloses `sin`/`cos` over `Q`, and `iadd`, `imulPt`, `ineg` round outward | `KrawczykRun.Assumptions.residual` | `interval.go` header comment (IEEE 754 correctly rounded `+`, `−`, `×` plus one outward step); `docs/certified-enclosure-design.md` "Rounding"; `interval_trig_internal_test.go` | `enclose_system.go certSystem.evalInto`, `certSystem.trig`, `certSystem.krawczyk`; `interval.go iadd`, `imulPt`, `ineg`, `pt` |
| A12 | For every `x` in `X` and `p` in `Q`, the interval `cij` in `krawczyk`'s loop contains entry `(i, j)` of `I − Y·F_x(x, p)`: `jacobianInto` over `eX` encloses every partial, a cell `jacWriter.add` did not mark in the current epoch is an exact zero partial (`marks[k*n+j]` matches `add`'s `row*width+col` with `width = n`), and `isub`, `imulPt` round outward | `KrawczykRun.Assumptions.slope` | Code reading of `jacWriter.add` and `krawczyk`; rounding evidence as A11 | `enclose_system.go certSystem.jacobianInto`, `jacWriter.add`, `certSystem.trig`, `certSystem.krawczyk`; `interval.go isub`, `imulPt`, `iadd`, `pt` |
| A13 | The computed `K[i]` contains `x̃_i + c_i + Σ_j C_ij·(x_j − x̃_j)` for every real `c`, `C` inside their computed intervals and every `x` in `X`, and every endpoint on the accepting path is a finite float | `KrawczykRun.Assumptions.image` | Inclusion property of the outward-rounded `iadd`, `isub`, `imul`, `pt`. `krawczyk` refuses a non-finite `c` or `K`; an infinite end of `X` makes `D` infinite, and `imul` of it gives NaN (times 0) or ±Inf, so `K` is refused | `enclose_system.go certSystem.krawczyk`; `interval.go iadd`, `isub`, `imul`, `pt` |
| A14 | The float solution `x̃` lies in the box `X` | `KrawczykRun.Assumptions.xt_mem` | Code reading of `krawczyk`'s inflation step: `X[vi] = [down(x̃ + z.Lo), up(x̃ + z.Hi)]` with `z.Lo ≤ 0 ≤ z.Hi`; round to nearest is monotone, so `x̃ + z.Lo` rounds to at most `x̃`, and `down` only lowers it (`up` mirrors) | `enclose_system.go certSystem.krawczyk`; `interval.go down`, `up` |
| A15 | The certified system is square: as many equations as free variables | `KrawczykRun.F : (Fin n → ℝ) → P → (Fin n → ℝ)` | `encloseRun.run` calls `checkStructure` before its first `krawczyk` call; it refuses unless the certified row count equals the residual row count `m`, the rank equals the free count `n`, and `m ≤ n`, so `m = n`. Neither count depends on `q` | `enclose.go encloseRun.checkStructure`; `enclose_system.go certSystem.krawczyk` |
| A20 | `math/big` arithmetic is exact: `big.Int` `Mul`, `Add`, `Sub`, `Abs`, `Lsh`, `Cmp`, and `big.Rat` `SetFloat64` (finite input), `NewRat`, `Mul`, `Sub`, `Cmp`; `big.Int.Rsh` floors (arithmetic shift); `Quo` and `QuoRem` truncate toward zero, with a remainder of the dividend's sign | `goShift` floors with `/` on `ℤ`; `goTermStep`, `goErrStep` use `Int.tdiv`, `Int.tmod`; rationals are exact reals in `reduceTurns`, `halfPiRats`, `halfPiMayLieIn` | `math/big` docs for `Int.Rsh`, `Int.Quo`, `Int.QuoRem`, `Rat` | `interval_trig.go` `sinCosSeriesBig`, `reduceTurns`, `halfPiRats`, `halfPiMayLieIn` |
| A21 | For a finite float64 `x`, `math.Frexp` and `int64(frac * (1 << 53))` give integers `m` and `e = exp - 53` with `x = m·2^e` exactly | hypothesis `hx : x = m * 2 ^ e` of `goTermStep_eq`, `goErrStep_eq` | `frac` has 53 significant bits (comment at the `big.NewInt` line); `math.Frexp` docs | `interval_trig.go` `sinCosSeriesBig` |
| A22 | The package values the hashes do not cover equal the Lean model: `piLo`, `piHi` (40-decimal literals `ratLit` parses exactly), `trigFracBits = 256`, `maxTaylorArg = 64`, `maxTrigArg = 1 << 40`, `halfPiTableK = 128`, and the `halfPiTable` initializer: entry `k + halfPiTableK` is `{ratLower(a), ratUpper(b), a.Float64()'s exact flag, b.Float64()'s exact flag}` for `a, b := halfPiRats(k)` | `piLo`, `piHi`; the literals `2 ^ 256`, `64`, `2 ^ 40`, `128` in the defs; `halfPiEntry` | the `var` and `const` blocks and the `halfPiTable` initializer of `interval_trig.go`; `TestHalfPiMayLieInMatchesRats` | `interval_trig.go` `sinCosPoint`, `sinCosSeriesBig`, `reduceTurns`, `halfPiRats`, `halfPiMayLieIn` |
| A23 | `big.Float.SetInt` and `SetMantExp` are exact, and `Float64` returns the round-to-nearest float64 of the value with accuracy `Above` exactly when that float exceeds it and `Below` exactly when it is less; `down` and `up` are the next float down and up | `fixedLower`, `fixedUpper` in `Sketch/Trig/Binary64.lean` (`M.rn`, `M.nd`, `M.nu`); the fields `Rounding.fixedLower_le`, `le_fixedUpper` | `math/big` docs for `Float.SetInt` (precision 0 takes the integer's bit length), `SetMantExp`, `Float64`; `interval.go` `down`, `up` | `interval_trig.go` `fixedLower`, `fixedUpper`, `sinCosSeriesBig` |
| A24 | `big.Rat.Float64` returns the round-to-nearest float64 of the rational and reports `exact` exactly when that float equals it; `down` and `up` are the next float down and up | `ratExact`, `ratLower`, `ratUpper` in `Sketch/Trig/Binary64.lean`; the fields `Rounding.ratLower_le`, `le_ratUpper`, `ratLower_ne`, `ratUpper_ne` | `math/big` docs for `Rat.Float64`; `interval.go` `down`, `up` | `interval_trig.go` `ratLower`, `ratUpper`, `ratInterval`, `reduceTurns`, `halfPiMayLieIn` |
| A25 | `sinCosSeriesWords` either reports false or returns the intervals `sinCosSeriesBig` returns for the same `x` | the model of `sinCosPoint` calls `sinCosSeriesBig` | `TestSinCosSeriesWordsMatchesBig`; the step-by-step argument in the doc comment of `sinCosSeriesWords` | `interval_trig.go` `sinCosSeriesWords`, `sinCosPoint` |
| A26 | A `trigPointCache` hit returns the intervals the series computed for the same `x`: the key is the float64 bits, and each entry is immutable and published through an `atomic.Pointer` | the model of `sinCosPoint` has no cache | the body of `sinCosPoint` | `interval_trig.go` `sinCosPoint` |
| A27 | In `sinCosRange`, when `q.Hi-q.Lo > 7` is false and both ends lie within `2^40`, the float64 `kmin` and `kmax` satisfy `kmin ≤ k ≤ kmax` for every integer `k` with `k·π/2` in `q` | `TrigEnv.k_cover` | `q.Lo/(math.Pi/2)` differs from `2·q.Lo/π` by far less than 1 for `\|q.Lo\| ≤ 2^40`, and the loop widens each end by 1 | `interval_trig.go` `sinCosRange` |
| A28 | Float64 values are the reals they denote and satisfy the `Sketch.Float.FloatModel` fields (finite floats, round to nearest, next float up); NaN and infinities never reach the models, as `sinCosPoint` rejects them first; comparisons, negation, `math.Abs` and built-in `min`/`max` are exact; the int64 `2*k` of `reduceTurns` and the `int` loop bounds of `sinCosRange` do not overflow for `\|x\| ≤ 2^40` | `Ival` endpoints are `ℝ`; real `<`, `≤`, `max`, `min`; `FloatModel` | Go spec "Comparison operators", "Min and max"; the `!(math.Abs(x) <= maxTrigArg)` guard of `sinCosPoint` | `interval_trig.go` `sinCosPoint`, `sinCosRange`, `reduceTurns`, `clampUnit`, `halfPiMayLieIn`, `ratLower`, `ratUpper`, `fixedLower`, `fixedUpper`, `sinCosSeriesBig` |
| A30 | `math/big.Rat` arithmetic is exact: `Add`, `Sub`, `Mul`, `Quo`, `Neg`, `Cmp`, `Sign`, `SetFrac64` and `NewRat` give the exact rational result, and the cone-side code never mutates a `*big.Rat` after sharing it (each helper allocates its result) | ℚ arithmetic in every `Sketch.Cone` model | Go `math/big` docs; code reading of `coneAdd`, `coneSub`, `coneMul` and the `coneFitPoly` helpers | `cone_line.go` `coneAdd`, `coneSub`, `coneMul`; the `cone_side.go` and `cone_fit_crossing.go` entries |
| A31 | `coneRat(v)` (`big.Rat.SetFloat64`) is exactly `v` for finite `v`. For non-finite `v` it returns nil and the next arithmetic call panics, so no certificate is returned | Model inputs are ℚ: the `lineSide` endpoints, the `newConeSideForm` arguments, the `FitData` fields | Go `math/big` docs for `SetFloat64` | `cone_line.go` `coneRat`; `newConeSideForm`, `coneSideForm.lineSide`, `coneSideForm.fitSide`, `coneFitBezier`, `Sketch.CertifyConeCurveSide` |
| A32 | `coneFitBinomial(n, k)` runs with `0 <= k <= n` and no `int64` overflow, so its truncating division equals ℕ division (`lineSide` calls it with `n <= 1`) | `coneFitBinomial` over ℕ | Code reading of `coneFitBernstein`'s call sites | `cone_fit_crossing.go` `coneFitBinomial`, `coneFitBernstein`; `coneSideForm.lineSide` |
| A33 | Span `i` of a `FitInterpolant` returned by `FitSpline.Interpolant` is, per coordinate, `c0 + u·(c1 + u·(c2 + u·c3))` on `u ∈ [0, 1]` with `c0 = p0`, `c1 = (p1−p0) − h²(2m0+m1)/6`, `c2 = h²m0/2`, `c3 = h²(m1−m0)/6` over the exact `Params`, `Points` and `SecondDerivs`, and the returned value is read whole | `FitData.spanPoint` | `geom.ExactFitSpan` doc comment; `exactFitPieces` computes these coefficients | `geom/arrange_fit_circle.go` `exactFitPieces`; `coneSideForm.fitSide`, `Sketch.CertifyConeCurveSide` |
| A40 | Exact real model of the angle and distance residuals and restatements: float `+`/`−`/`×` are exact; `math.Atan2(y, x)` is `Complex.arg (x + y·i)` (signed zeros not modeled; `Atan2(0, 0) = 0`); `math.Hypot(a, b)` is `√(a² + b²)`; `math.Mod(x, y)` is `x − y·trunc(x / y)`; the constant `2*math.Pi` is `2π` | defs `atan2`, `goTrunc`, `goMod`, `wrapPi`, `dist` and every model def in `Sketch/Enclose/Angle.lean` are over `ℝ` | Go `math` docs for `Atan2`, `Hypot`, `Mod`; code reading. Rounding is outside the model: the theorems state the zero sets of the exact equations | `constraint.go dir`, `dist`; `constraint_dimension.go Angle.residual`, `Distance.residual`; `enclose_system.go certAngle.eval`, `certAngle.parts`, `certAngle.side`, `certDistance.eval`; `probe.go wrapPi` |
| A41 | The intervals `certAngle.eval`, `certAngle.side` and `certDistance.eval` compute (`isub`, `iadd`, `imul`, `isqr`, and `certAngle.trig`'s `sinCosRange` or `sinCosPoint` enclosures) contain the exact real value at every configuration of the box and every angle in the range, so `side` returning true means the real side value is positive at every configuration of the box | hypotheses `hside` of `certAngle_zero_iff_angleResidual_zero` and `hbox` of `certAngle_zero_set_eq_on_box`; `certAngleTrig` returns the exact `(sin θ, cos θ)` | Outward rounding in `interval.go`; proven bounds in `interval_trig.go`; their tests | `enclose_system.go certAngle.eval`, `certAngle.parts`, `certAngle.side`, `certAngle.trig`, `certDistance.eval` |
| A42 | Every distance value a `certDistance` row reads is positive: a non-driver target, every value of a `WithTargetRange` range, and every value of the driving range | hypothesis `hd : 0 ≤ d` of `certDistance_zero_iff_distanceResidual_zero` | `certEquationsOf` refuses a target whose lower end is not positive; `rangeBase` refuses a distance range with `lo <= 0`; `foldKrawczyk` refuses a driving-value box whose lower end is not positive | `enclose_system.go certDistance.eval` |

- Adding a row = new id `A<n+1>`. NEVER renumber. Delete a row only when no manifest entry cites it.
- The checker fails when a manifest entry cites an id with no row starting `| A<n> |` in this file.

## Trace manifest

`_proofs/trace.json`. Every sketch function on a proved path is an entry: the function a theorem models, and
each sketch function it calls whose behavior the proof relies on (as `assumes`). Standard-library calls are
covered by an assumption row, not an entry.

```json
{
  "entries": [
    {
      "file": "geom/example.go",
      "func": "Example",
      "role": "models",
      "theorems": ["Sketch/Area/Topic.lean:Sketch.Area.example_sound"],
      "assumptions": ["A1"],
      "sha256": "<64 hex>"
    }
  ]
}
```

| Field | Rule |
|---|---|
| `file` | Repo-relative path. Key is (`file`, `func`), so same-named functions in build-tag twins are separate entries. |
| `func` | `Name` for a function, `Type.Name` for a method (receiver type without `*`). |
| `role` | `models`: a Lean def mirrors it; needs at least 1 theorem. `assumes`: an assumption row states its behavior; needs at least 1 assumption. |
| `theorems` | `<file under _proofs>:<fully qualified name>`. |
| `assumptions` | Assumption ids. |
| `sha256` | See Hash definition. |

- Entries are sorted by (`file`, `func`) and have no duplicate keys. The file is pretty-printed with a 2-space
  indent and a trailing newline.
- Each covered function's doc comment ends with exactly this line:

```go
// Lean proofs depend on this function; see _proofs/trace.json.
```

- An undocumented covered function gets one line starting with its name that states what the code does.

## Checker

`_proofs/tracecheck` is a standard-library-only `package main`. Its tests use `testing` without testify, so
the module has no requirements and no `go.sum`.

### Hash definition

`sha256` of the exact source bytes from the `func` keyword through the closing `}`:
`src[fset.Position(fd.Pos()).Offset : fset.Position(fd.End()).Offset]` for the `*ast.FuncDecl`.

- It includes the receiver, signature, body and comments inside the body.
- It excludes the doc comment, so editing the pointer line or the doc text never changes the hash.
- It is whitespace-sensitive. Repo Go is gofmt'd, so this only matters on real edits.
- `go/parser` ignores build tags, so both files of a build-tag twin parse.

### Checks

All must pass. Each failure is one line naming file, func and the next action.

| # | Check | Failure line starts with |
|---|---|---|
| 1 | Manifest parses; fields valid; sorted; no duplicate keys | `<file> <func>: ...` |
| 2 | `file` exists; `func` found exactly once; hash matches | `<file> <func>: source changed; re-check <theorems> ...` |
| 3 | Each theorem's Lean file exists and has a line matching `^\s*(theorem\|lemma)\s+<last name component>\b` | `<file> <func>: theorem <name> not declared in _proofs/<lean file>` |
| 4 | Each assumption id has a row starting `\| A<n> \|` in this file | `<file> <func>: assumption A<n> has no row` |
| 5 | Each entry's Go doc comment contains `_proofs/trace.json` | `<file> <func>: doc comment lacks the pointer` |
| 6 | Reverse: every function in a repo `.go` file (skipping `_`/`.`-prefixed directories and `_test.go`) whose doc comment contains `_proofs/trace.json` is an entry | `<file> <func>: doc comment points to ... but the manifest has no entry` |
| 7 | Each entry's `file` appears single-quoted in `.github/workflows/proofs.yml` | `<file> <func>: '<file>' is missing from .github/workflows/proofs.yml` |

### CLI

Run from `_proofs`:

- `go run ./tracecheck` runs the checks and exits 0 or 1. `-repo <dir>` sets the repository root (default `..`).
- `go run ./tracecheck -update` rewrites the `sha256` of every existing entry whose function is found, prints
  each change, then runs every check. It NEVER adds or removes entries.
- Run `-update` ONLY after re-reading each affected theorem against the changed Go code. The PR diff of
  `trace.json` shows reviewers which theorems the change touched.

## CI workflow

`.github/workflows/proofs.yml` is separate from `ci.yml`, which does not run it. It is NOT a required check: a
path-filtered required check stays pending on PRs it skips. **NEVER merge a PR whose `Proofs` workflow failed**,
even when the main CI is green.

- Triggers: `pull_request` and `push` to `main` for the `paths` list (`_proofs/**`, the workflow, this file and
  each covered Go file), and `workflow_dispatch`. The `paths` list is a YAML anchor shared by both triggers.
  Check 7 reads the file as text, so a duplicated list passes as well.
- Job `trace` (`proofs trace`, 5-minute cap): checkout, `actions/setup-go` with `go-version-file: _proofs/go.mod`,
  then `go vet ./...`, `go test ./...` and `go run ./tracecheck` in `_proofs`.
- Job `lean` (`proofs build`, 30-minute cap): checkout, then `leanprover/lean-action` pinned by full commit SHA
  with the version in a trailing comment, as every action in this repository is. Dependabot's `github-actions`
  entry updates the pin.

| lean-action input | Value | Reason |
|---|---|---|
| `lake-package-directory` | `_proofs` | The Lake project is not at the repository root. |
| `use-github-cache` | `false` | A multi-GB `.lake` entry would evict `ci.yml`'s Go build caches. `lake exe cache get` downloads Mathlib's artifacts from Mathlib's servers instead. |
| `use-mathlib-cache` | `true` | Runs `lake exe cache get`. |
| `mk_all-check` | `true` | Fails when a `Sketch/**.lean` file is not imported by `Sketch.lean`, so `lake build` cannot skip it. |
| `build`, `build-args` | `true`, `--wfail` | Warnings fail the build. |
| `axiom-audit`, `axiom-audit-root` | `true`, `Sketch` | Fails when a `Sketch.*` declaration depends on an axiom outside `propext`, `Classical.choice`, `Quot.sound`: catches `sorry`, `admit`, `native_decide` and any `axiom`. |
| `test`, `lint`, `leanchecker` | `false` | No Lake test or lint driver is configured. `leanchecker` replays declarations the kernel already checked. |

lean-action runs elan's `elan-init.sh` from elan's `master` branch, which the action SHA does not pin. The Lean
version is pinned by `lean-toolchain`.

| Failure | Meaning | Action |
|---|---|---|
| `trace` red, "source changed" | A covered Go function changed | Re-check the theorem; fix the proof or the code; then `-update` |
| `trace` red, workflow path missing | A new entry's file is not in `paths` | Add it to the `paths` list in `proofs.yml` |
| `lean` red at build | Proof broken or new warning | Fix the Lean file |
| `lean` red at mk_all | New file not imported | `lake exe mk_all` in `_proofs`, commit `Sketch.lean` |
| `lean` red at axiom-audit | `sorry`/`axiom`/`native_decide` reached a `Sketch` declaration | Remove it |
| `lean` red at cache get | Mathlib cache server down, or artifacts for the pin no longer served | Rerun; if the artifacts are gone, upgrade Mathlib |

A workflow edit needs a token with the `workflow` scope to merge.

## Recipe: cover a new Go function

1. Read the Go function. List every sketch function it calls on the path the guarantee depends on.
2. Write an `@[expose]` Lean def mirroring its control flow in `Sketch/<Area>/<Topic>.lean`. Use ℚ/ℤ for exact
   arithmetic. A fact Lean cannot see (hardware, standard library, a callee you do not model) becomes a
   hypothesis or a model-structure field, plus a new assumption row. NEVER an `axiom`.
3. Prove the end-to-end theorem stating the published guarantee about the def. Write its docstring per the
   naming rules above.
4. `lake exe mk_all` (updates `Sketch.lean`), then `lake build --wfail`.
5. Add `trace.json` entries: `models` for the function, `assumes` for each relied-on callee. Add the pointer
   line to each. Put placeholder hashes of 64 zeros, run `go run ./tracecheck -update`, then
   `go run ./tracecheck`.
6. Add each new Go file to the `paths` list in `proofs.yml`.
7. Run the axiom audit (Run locally).

## Upgrade Lean and Mathlib

One PR per upgrade, nothing else in it.

1. Pick a Mathlib release tag `vX` (Mathlib tags match Lean releases).
2. Set `rev = "vX"` in `lakefile.toml`; in `_proofs` run `lake update mathlib`.
3. Copy `.lake/packages/mathlib/lean-toolchain` to `_proofs/lean-toolchain`.
4. `lake exe cache get`, then `lake build --wfail`. Fix deprecations and broken proofs; NEVER silence them.
5. Run the axiom audit and `go run ./tracecheck`.
6. Commit `lakefile.toml`, `lean-toolchain`, `lake-manifest.json` and the Lean fixes.

- NEVER run `lake update` without step 2's pinned `rev`; it moves every transitive package.
- Dependabot does not update Lean packages.

## Run locally

Put `~/.elan/bin` on `PATH`. In `_proofs`:

| Step | Command |
|---|---|
| Download Mathlib artifacts | `lake exe cache get` |
| Build, warnings fatal | `lake build --wfail` |
| Imports complete | `lake exe mk_all --check` |
| Trace checker tests | `go vet ./... && go test ./...` |
| Manifest check | `go run ./tracecheck` |

Axiom audit, as lean-action runs it: clone `https://github.com/leanprover-community/axiom-audit` at tag `v0.1.2`
into a scratch directory, check `HEAD` is `46024e005996495c65ef609368e11ab39c4222e3`, copy
`_proofs/lean-toolchain` over its `lean-toolchain`, run `lake build` there, then in `_proofs` run
`lake env <clone>/.lake/build/bin/axiom-audit --allow propext,Classical.choice,Quot.sound --root Sketch`.
