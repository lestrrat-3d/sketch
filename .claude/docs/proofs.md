# Lean proofs

`_proofs/` holds machine-checked Lean 4 proofs about the sketch engine, plus a Go checker that fails when a
covered Go function changes without its theorem being re-read. The proofs are about Lean models of Go functions
under the assumptions table below. They are not proofs about compiled Go.

Covered so far: soundness of the outward-rounded interval helpers in `interval.go`
(`_proofs/Sketch/Interval/Arith.lean`) over the abstract binary64 model `_proofs/Sketch/Float/Model.lean`; the
zero sets of the angle and distance enclosure equations (`_proofs/Sketch/Enclose/Angle.lean`); the accepting
branch of the parametric Krawczyk test (`_proofs/Sketch/Enclose/Krawczyk.lean`); the certified sine and
cosine enclosures of `interval_trig.go` (`_proofs/Sketch/Trig/`), including a 40-digit bracket of π; its
direction-angle enclosures `atan2Point`, `atan2Box` and `shiftNear`, and the arc span bounds
`certifiedArcSpanBounds` behind `Arc.CertifiedSpanBounds` (`Atan2.lean`, `Atan2Box.lean`, `ArcSpan.lean` in
`_proofs/Sketch/Trig/`); the driven-dimension evaluators of `certDrivenOf` in `enclose_system.go` and the
whole-turn shift of a driven angle in `encloseRun.piece` (`_proofs/Sketch/Enclose/Driven.lean`);
`Sketch.CertifyConeCurveSide` in `cone_side.go` and `cone_side_arc.go` for lines, fitted splines and arcs; and,
in `cone_fit_crossing.go`, the Bernstein enclosure of a polynomial of any degree, its integer scaled form
`coneFitIntBernstein`, and the crossing certificate of `Sketch.CertifyConeFitCrossing` (`_proofs/Sketch/Cone/`); the exact rational crossing of two segments, `exactLineCrossing` in
`profile_junction_line_line.go` (`_proofs/Sketch/Arrange/LineCrossing.lean`); and the equality of `Sketch.jacobianLocalInto` with the dense
`Sketch.jacobianInto` whenever it does not refuse, with every refusal condition listed
(`_proofs/Sketch/Jacobian/Local.lean`). The rest of the arrangement engine in `geom/arrange*.go` is not covered.
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
  Sketch/Arrange/LineCrossing.lean exactLineCrossing: the exact segment crossing is on both segments and unique
  Sketch/Cone/*.lean         cone_side.go, cone_side_arc.go: exact intervals, box and line tests, Bézier
                             subdivision, fit spans, real-point box test, wedge geometry, square-root bounds,
                             arc proofs, entry; cone_fit_crossing.go: Bernstein enclosure, integer Bernstein
                             bounds, root search, bisection, crossing entry
  Sketch/Enclose/Angle.lean zero sets of the angle and distance enclosure equations
  Sketch/Enclose/Driven.lean driven-dimension evaluators over a box and the driven angle's whole-turn shift
  Sketch/Enclose/Krawczyk.lean soundness of the accepting branch of the Krawczyk test
  Sketch/Float/Model.lean    FloatModel: abstract finite binary64 (rn, nu, nd, δ) + rounding lemmas
  Sketch/Float/Grid.lean     gridModel: proves FloatModel's fields jointly satisfiable
  Sketch/Jacobian/Local.lean dense vs local central-difference Jacobian, refusal conditions
  Sketch/Interval/Arith.lean models + soundness theorems for interval.go's helpers
  Sketch/Trig/*.lean         interval_trig.go: π bracket, Taylor tail, fixed-point series, reduction, range,
                             direction angles (Atan2, Atan2Box), arc span bounds (ArcSpan)
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
| A2 | No NaN, no infinity, no overflow: every interval endpoint and every rounded intermediate on the modeled path is a finite float. `±Inf` results are not modeled | `FloatModel.F` holds only finite floats; `rn_mem`, `nu_mem` always land in `F` | The certified path refuses a residual or Krawczyk image with a non-finite or unordered endpoint (`Interval.isFinite` in `enclose_system.go`, `enclose_fold.go`); a fixed box must pass it too (`enclose.go`). `TestIsFiniteScreen` (`interval_internal_test.go`) pins the screen to both ends finite and ordered. Outside the model, a rounded `±Inf` endpoint only widens the interval | `interval.go` `down`, `up`, `iadd`, `isub`, `imul`, `imulPt`, `isqr`, `isqrtNonNeg`, `Interval.isFinite`; `interval_trig.go` `certifiedArcSpanBounds` |
| A3 | On a finite input, `up(x)` returns the least float above `x` and `down(x)` the greatest float below `x`, equal to `-up(-x)`: `Float64bits` ±1, `±math.SmallestNonzeroFloat64` at zero | `FloatModel.nu`, `nu_mem`, `lt_nu`, `nu_min`; `FloatModel.nd f = -nu (-f)`, `neg_mem` | `TestDownUpMatchNextafter` (`interval_internal_test.go`) against `math.Nextafter` and `-up(-x)` | `interval.go` `down`, `up`, `iadd`, `isub`, `imul`, `imulPt`, `isqr`, `isqrtNonNeg` |
| A4 | Each product written as `float64(x*y)` is rounded once: the explicit conversion forbids fusing it into an FMA | each product is one `M.rn (x * y)` | Go spec "Floating-point operators" (explicit conversion rounds). Dated check (Go 1.27.3, `GOARCH=arm64 go build -gcflags=-S .`): every `FMADDD`/`FMSUBD`/`FNMADDD`/`FNMSUBD` attributed to `interval.go` is on `Interval.mid`'s line, none in a covered helper; in `enclose_system.go` they are on `krawczyk`'s ε-inflation line (which needs only `x̃ ∈ X`, A14) and in `invertMatrixInto` (`Y` is arbitrary), none in `krawczyk`'s sums | `interval.go` `imul`, `imulPt`, `isqr`; `enclose_system.go` `certSystem.krawczyk` |
| A5 | `math.Sqrt(x)` on finite `x ≥ 0` returns the round-to-nearest value of `√x` | `M.rn (Real.sqrt x)` in `isqrtNonNeg` | `math/sqrt.go` "Return correctly rounded sqrt"; hardware square root is IEEE 754 correctly rounded | `interval.go` `isqrtNonNeg` |
| A6 | On finite float64 values, negation is exact and `<`, `<=`, `>=`, built-in `min`/`max` agree with the real operations (signed zero aside) | real `-`, `<`, `≤`, `min`, `max` | Go spec "Comparison operators", "Min and max"; IEEE 754 negation flips the sign bit | `interval.go` `ineg`, `imul`, `imulPt`, `isqr`, `isqrtNonNeg`, `ihull`, `Interval.strictlyInside` |
| A10 | Each certified equation `F(·, p)` is differentiable at every point of the box `X`, for every parameter value `p`, and its Jacobian is the matrix of the closed-form partials each `certEquation.jac` writes | `KrawczykRun.Assumptions.deriv` | Code reading: each `certEquation.eval` is a polynomial in the coordinates (coordinate differences, cross and dot products, squared distance) with the parameter's value or sine and cosine as constants, and its `jac` writes that polynomial's partials (`writeVecPartials`) | `enclose_system.go certSystem.jacobianInto`, `certSystem.krawczyk` |
| A11 | For every parameter value `p` in `Q` (the driving value with every ranged target and fixed box), the interval `c[i]` that `krawczyk` computes contains row `i` of `−Y·F(x̃, p)`: `evalInto` over `base` encloses every equation, `trig` encloses `sin`/`cos` over `Q`, and `c[i]` is the float sum of `−Y_ij` times the endpoints of `F[j]`, widened by its A160 bound and rounded outward with `down`, `up` | `KrawczykRun.Assumptions.residual` | A160; `interval.go` header comment (IEEE 754 correctly rounded `+`, `−`, `×` plus one outward step); `docs/certified-enclosure-design.md` "Rounding"; `interval_trig_internal_test.go` | `enclose_system.go certSystem.evalInto`, `certSystem.trig`, `certSystem.krawczyk`, `sumGain`; `interval.go down`, `up`, `pt` |
| A12 | For every `x` in `X` and `p` in `Q`, entry `(i, j)` of `I − Y·F_x(x, p)` lies in `[lo − e, hi + e]`, where `[lo, hi]` is the float sum `krawczyk`'s loop forms for cell `(i, j)` from `δ_ij` and `−Y_ik` times the endpoints of each marked `J[k][j]`, and `e` is that sum's A160 bound. The code never forms `lo − e` or `hi + e`; they are the real `CLo`, `CHi`. `jacobianInto` over `eX` encloses every partial, a cell `jacWriter.add` did not mark in the current epoch is an exact zero partial (`marks[k*n+j]` matches `add`'s `row*width+col` with `width = n`), and the loop lists exactly the rows of column `j` marked in that epoch | `KrawczykRun.Assumptions.slope` | Code reading of `jacWriter.add` and `krawczyk`; A160 | `enclose_system.go certSystem.jacobianInto`, `jacWriter.add`, `certSystem.trig`, `certSystem.krawczyk`, `sumGain`; `interval.go pt` |
| A13 | The computed `K[i]` contains `x̃_i + c_i + Σ_j C_ij·(x_j − x̃_j)` for every real `c`, `C` inside the bounds of A11 and A12 and every `x` in `X`, and every endpoint on the accepting path is a finite float. Rounding is monotone, so each cell has `lo ≤ hi`, and `C_ij·d` lies in `[lo, hi]·D[j]` widened by `e·max(−D[j].Lo, D[j].Hi)`. The row's float sum of `c_i` and the extreme endpoint products of `[lo, hi]·D[j]` is widened once by a bound that covers its own A160 bound and every cell's `e·max(−D[j].Lo, D[j].Hi)`, then `x̃_i` is added with `down`, `up` | `KrawczykRun.Assumptions.image` | A160; code reading of `krawczyk`. `krawczyk` refuses a non-finite `c` or `K`; an infinite end of `X` makes `D` infinite, and a product or sum of it gives NaN or ±Inf, so `K` is refused | `enclose_system.go certSystem.krawczyk`, `sumGain`; `interval.go isub`, `down`, `up`, `pt` |
| A14 | The float solution `x̃` lies in the box `X` | `KrawczykRun.Assumptions.xt_mem` | Code reading of `krawczyk`'s inflation step: `X[vi] = [down(x̃ + z.Lo), up(x̃ + z.Hi)]` with `z.Lo ≤ 0 ≤ z.Hi`; round to nearest is monotone, so `x̃ + z.Lo` rounds to at most `x̃`, and `down` only lowers it (`up` mirrors) | `enclose_system.go certSystem.krawczyk`; `interval.go down`, `up` |
| A15 | The certified system is square: as many equations as free variables | `KrawczykRun.F : (Fin n → ℝ) → P → (Fin n → ℝ)` | `encloseRun.run` calls `checkStructure` before its first `krawczyk` call; it refuses unless the certified row count equals the residual row count `m`, the rank equals the free count `n`, and `m ≤ n`, so `m = n`. Neither count depends on `q` | `enclose.go encloseRun.checkStructure`; `enclose_system.go certSystem.krawczyk` |
| A20 | `math/big` arithmetic is exact: `big.Int` `Mul`, `Add`, `Sub`, `Abs`, `Lsh`, `Cmp`, and `big.Rat` `SetFloat64` (finite input), `NewRat`, `Mul`, `Sub`, `Cmp`; `big.Int.Rsh` floors (arithmetic shift); `Quo` and `QuoRem` truncate toward zero, with a remainder of the dividend's sign | `goShift` floors with `/` on `ℤ`; `goTermStep`, `goErrStep` use `Int.tdiv`, `Int.tmod`; rationals are exact reals in `reduceTurns`, `halfPiRats`, `halfPiMayLieIn` | `math/big` docs for `Int.Rsh`, `Int.Quo`, `Int.QuoRem`, `Rat` | `interval_trig.go` `sinCosSeriesBig`, `reduceTurns`, `halfPiRats`, `halfPiMayLieIn`, `twoPiTimes` |
| A21 | For a finite float64 `x`, `math.Frexp` and `int64(frac * (1 << 53))` give integers `m` and `e = exp - 53` with `x = m·2^e` exactly | hypothesis `hx : x = m * 2 ^ e` of `goTermStep_eq`, `goErrStep_eq` | `frac` has 53 significant bits (comment at the `big.NewInt` line); `math.Frexp` docs | `interval_trig.go` `sinCosSeriesBig` |
| A22 | The package values the hashes do not cover equal the Lean model: `piLo`, `piHi` (40-decimal literals `ratLit` parses exactly), `maxTaylorArg = 64`, `maxTrigArg = 1 << 40`, `halfPiTableK = 128`, and the `halfPiTable` initializer: entry `k + halfPiTableK` is `{ratLower(a), ratUpper(b), a.Float64()'s exact flag, b.Float64()'s exact flag}` for `a, b := halfPiRats(k)`. The series precision is not one of them: the model takes it as `TrigEnv.fracBits` and holds for every value | `piLo`, `piHi`; the literals `64`, `2 ^ 40`, `128`, and `32` (`halfPiTableK/4`) in the defs; `halfPiEntry` | the `var` and `const` blocks and the `halfPiTable` initializer of `interval_trig.go`; `TestHalfPiMayLieInMatchesRats`, `TestTwoPiTimesMatchesRats` | `interval_trig.go` `sinCosPoint`, `sinCosSeriesBig`, `reduceTurns`, `halfPiRats`, `halfPiMayLieIn`, `twoPiTimes` |
| A23 | `big.Float.SetInt` and `SetMantExp(f, -fb)` are exact, and `Float64` returns the round-to-nearest float64 of the value with accuracy `Above` exactly when that float exceeds it and `Below` exactly when it is less; `down` and `up` are the next float down and up | `fixedLower`, `fixedUpper` in `Sketch/Trig/Binary64.lean` (`M.rn`, `M.nd`, `M.nu`); the fields `Rounding.fixedLower_le`, `le_fixedUpper` | `math/big` docs for `Float.SetInt` (precision 0 takes the integer's bit length), `SetMantExp`, `Float64`; `interval.go` `down`, `up` | `interval_trig.go` `fixedLower`, `fixedUpper`, `sinCosSeriesBig` |
| A24 | `big.Rat.Float64` returns the round-to-nearest float64 of the rational and reports `exact` exactly when that float equals it; `down` and `up` are the next float down and up | `ratExact`, `ratLower`, `ratUpper` in `Sketch/Trig/Binary64.lean`; the fields `Rounding.ratLower_le`, `le_ratUpper`, `ratLower_ne`, `ratUpper_ne` | `math/big` docs for `Rat.Float64`; `interval.go` `down`, `up` | `interval_trig.go` `ratLower`, `ratUpper`, `ratInterval`, `reduceTurns`, `halfPiMayLieIn`, `twoPiTimes` |
| A25 | `sinCosSeriesWords` either reports false or returns the intervals `sinCosSeriesBig` returns for the same `x` at `fb = trigFracBits` | the model of `sinCosPoint` calls `sinCosSeriesBig` | `TestSinCosSeriesWordsMatchesBig`, `TestSeriesIntWidthOps`; the step-by-step argument in the doc comment of `sinCosSeriesWords` | `interval_trig.go` `sinCosSeriesWords`, `sinCosPoint` |
| A26 | A `trigPointCache` hit returns the intervals the series computed for the same `x`: the key is the float64 bits, and each entry is immutable and published through an `atomic.Pointer` | the model of `sinCosPoint` has no cache | the body of `sinCosPoint` | `interval_trig.go` `sinCosPoint` |
| A27 | In `sinCosRange`, when `q.Hi-q.Lo > 7` is false and both ends lie within `2^40`, the float64 `kmin` and `kmax` satisfy `kmin ≤ k ≤ kmax` for every integer `k` with `k·π/2` in `q` | `TrigEnv.k_cover` | `q.Lo/(math.Pi/2)` differs from `2·q.Lo/π` by far less than 1 for `\|q.Lo\| ≤ 2^40`, and the loop widens each end by 1 | `interval_trig.go` `sinCosRange` |
| A28 | Float64 values are the reals they denote and satisfy the `Sketch.Float.FloatModel` fields (finite floats, round to nearest, next float up); NaN and infinities never reach the models, as `sinCosPoint` rejects them first; comparisons, negation, `math.Abs` and built-in `min`/`max` are exact; the int64 `2*k` of `reduceTurns` and the `int` loop bounds of `sinCosRange` do not overflow for `\|x\| ≤ 2^40` | `Ival` endpoints are `ℝ`; real `<`, `≤`, `max`, `min`; `FloatModel` | Go spec "Comparison operators", "Min and max"; the `!(math.Abs(x) <= maxTrigArg)` guard of `sinCosPoint` | `interval_trig.go` `sinCosPoint`, `sinCosRange`, `reduceTurns`, `clampUnit`, `halfPiMayLieIn`, `ratLower`, `ratUpper`, `fixedLower`, `fixedUpper`, `sinCosSeriesBig`, `atan2Point`, `atan2Box`, `shiftNear`, `certifiedArcSpanBounds` |
| A30 | `math/big.Rat` arithmetic is exact: `Add`, `Sub`, `Mul`, `Quo`, `Neg`, `Cmp`, `Sign`, `SetFrac64` and `NewRat` give the exact rational result, and the cone-side code never mutates a `*big.Rat` after sharing it (each helper allocates its result) | ℚ arithmetic in every `Sketch.Cone` model | Go `math/big` docs; code reading of `coneAdd`, `coneSub`, `coneMul` and the `coneFitPoly` helpers | `cone_line.go` `coneAdd`, `coneSub`, `coneMul`; the `cone_side.go`, `cone_side_arc.go` and `cone_fit_crossing.go` entries |
| A31 | `coneRat(v)` (`big.Rat.SetFloat64`) is exactly `v` for finite `v`. For non-finite `v` it returns nil and the next arithmetic call panics, so no certificate is returned | Model inputs are ℚ: the `lineSide` endpoints, the `newConeSideForm` arguments, the `FitData` fields, the `certifyConeFitCrossing` arguments | Go `math/big` docs for `SetFloat64` | `cone_line.go` `coneRat`; `newConeSideForm`, `coneSideForm.lineSide`, `coneSideForm.fitSide`, `coneFitBezier`, `coneFitMakePiece`, `coneSideForm.arcSide`, `coneSideForm.arcQuadrantSide`, `coneSideForm.arcWedgeSide`, `coneArcWedge.box`, `coneArcWedge.side`, `coneOutward`, `Sketch.CertifyConeCurveSide`, `Sketch.CertifyConeFitCrossing` |
| A32 | `coneFitBinomial(n, k)` runs with `0 <= k <= n` and no `int64` overflow, so its truncating division equals ℕ division (`lineSide` calls it with `n <= 1`; `coneFitIntBernstein` with `k <= n <= 6`, the degree of `f`) | `coneFitBinomial` over ℕ; `coneFitBinomial_eq` proves it is `Nat.choose` | Code reading of the call sites in `coneFitBernstein` and `coneFitIntBernstein` | `cone_fit_crossing.go` `coneFitBinomial`, `coneFitBernstein`, `coneFitCoeffBounds`, `coneFitIntBernstein`, `coneFitSearch.visit`, `Sketch.CertifyConeFitCrossing`; `coneSideForm.lineSide` |
| A33 | Span `i` of a `FitInterpolant` returned by `FitSpline.Interpolant` is, per coordinate, `c0 + u·(c1 + u·(c2 + u·c3))` on `u ∈ [0, 1]` with `c0 = p0`, `c1 = (p1−p0) − h²(2m0+m1)/6`, `c2 = h²m0/2`, `c3 = h²(m1−m0)/6` over the exact `Params`, `Points` and `SecondDerivs`, and the returned value is read whole | `FitData.spanPoint`, `FitData.spanPointR` | `geom.ExactFitSpan` doc comment; `exactFitPieces` computes these coefficients | `geom/arrange_fit_circle.go` `exactFitPieces`; `coneSideForm.fitSide`, `Sketch.CertifyConeCurveSide`, `Sketch.CertifyConeFitCrossing` |
| A60 | On the accepted path of `CertifyConeFitCrossing`, `coneFitFloatDown(r) <= r <= coneFitFloatUp(r)`: `big.Rat.Float64` returns the nearest float64, and when that float lies on the wrong side of `r`, `math.Nextafter` moves one float outward, which cannot pass `r`. An out-of-range `r` gives an infinity, which the final check `tlo >= 0 && thi <= 1` refuses | hypotheses `hdown`, `hup` of `certifyConeFitCrossing_sound` on the model's `down`, `up` | Go `math/big` docs for `Rat.Float64`; `math` docs for `Nextafter`; A31 for the exact `coneRat(v)` comparison | `cone_fit_crossing.go` `coneFitFloatDown`, `coneFitFloatUp`, `Sketch.CertifyConeFitCrossing` |
| A34 | In `exactLineCrossing`, `big.Rat.SetFloat64` of a finite float is exactly that float (the `IsNaN`/`IsInf` guard runs first); the `sub`, `mul`, `add` closures each allocate a fresh `*big.Rat` and never mutate a shared one; `Quo` runs only after `den.Sign() != 0`; a true `exactX`/`exactY` from `Float64` means the returned float equals the rational, so the `IsInf` check never fires | ℚ arithmetic in `exactLineCrossing`; hypothesis `ExactFlag` (A24) | Go `math/big` docs for `SetFloat64`, `Quo`, `Float64`; code reading of the closures | `profile_junction_line_line.go` `exactLineCrossing` |
| A40 | Exact real model of the angle and distance residuals and restatements: float `+`/`−`/`×` are exact; `math.Atan2(y, x)` is `Complex.arg (x + y·i)` (signed zeros not modeled; `Atan2(0, 0) = 0`); `math.Hypot(a, b)` is `√(a² + b²)`; `math.Mod(x, y)` is `x − y·trunc(x / y)`; the constant `2*math.Pi` is `2π` | defs `atan2`, `goTrunc`, `goMod`, `wrapPi`, `dist` and every model def in `Sketch/Enclose/Angle.lean` are over `ℝ` | Go `math` docs for `Atan2`, `Hypot`, `Mod`; code reading. Rounding is outside the model: the theorems state the zero sets of the exact equations | `constraint.go dir`, `dist`; `constraint_dimension.go Angle.residual`, `Distance.residual`; `enclose_system.go certAngle.eval`, `certAngle.parts`, `certAngle.side`, `certDistance.eval`, `certDrivenOf`; `probe.go wrapPi` |
| A41 | The intervals `certAngle.eval`, `certAngle.side` and `certDistance.eval` compute (`isub`, `iadd`, `imul`, `isqr`, and `certAngle.trig`'s `sinCosRange` or `sinCosPoint` enclosures) contain the exact real value at every configuration of the box and every angle in the range, so `side` returning true means the real side value is positive at every configuration of the box | hypotheses `hside` of `certAngle_zero_iff_angleResidual_zero` and `hbox` of `certAngle_zero_set_eq_on_box`; `certAngleTrig` returns the exact `(sin θ, cos θ)` | Outward rounding in `interval.go`; proven bounds in `interval_trig.go`; their tests | `enclose_system.go certAngle.eval`, `certAngle.parts`, `certAngle.side`, `certAngle.trig`, `certDistance.eval` |
| A42 | Every distance value a `certDistance` row reads is positive: a non-driver target, every value of a `WithTargetRange` range, and every value of the driving range | hypothesis `hd : 0 ≤ d` of `certDistance_zero_iff_distanceResidual_zero` | `certEquationsOf` refuses a target whose lower end is not positive; `rangeBase` refuses a distance range with `lo <= 0`; `foldKrawczyk` refuses a driving-value box whose lower end is not positive | `enclose_system.go certDistance.eval` |
| A70 | `int64` arithmetic in the whole-turn helpers does not overflow: `twoPiTimes(k)` forms `4*k` only on its table path (`\|k\| ≤ 32`), and `2*k` only for `\|k\| ≤ 2^20` (`shiftNear`) or `\|k\| ≤ 2` (`certifiedArcSpanBounds`, `atan2Box`), and `int64(k)` of an integral float64 with `\|k\| ≤ 2^20` is exact | `twoPiTimes` takes `k : ℤ`; `shiftNear` takes its decision as `Option ℤ` | The guards `math.Abs(k) > 1<<20` in `shiftNear` and `shift < -2 \|\| shift > 2` in `certifiedArcSpanBounds`; `atan2Box` calls `twoPiTimes(1)` | `interval_trig.go` `twoPiTimes`, `shiftNear`, `atan2Box`, `certifiedArcSpanBounds`; `enclose.go` `encloseRun.piece` |
| A71 | In `atan2Point`, `math.Atan2(c, d)` is some real number, never trusted for a bound (NaN is refused before use); `math.Ldexp((1+math.Abs(phi))*1e-15, 4*k)` is not negative; `math.Abs` is exact; `isqr(h).Hi / 2` is the round-to-nearest value of the exact quotient | `Atan2Env.atan2`, `Atan2Env.delta` and its field `delta_nonneg`; `M.rn (x / 2)` in `crossAt` | The `math.IsNaN(phi)` check; `math.Ldexp` scales the positive product `(1+\|phi\|)·1e-15` by `2^(4k)`; Go spec "Arithmetic operators" (IEEE 754 division) | `interval_trig.go` `atan2Point` |
| A72 | `atan2Box`'s float decisions on a rectangle that misses the origin: (a) `ref = math.Atan2(c.mid(), d.mid())` lies within 1/10 rad of a direction angle of some point of the rectangle; (b) every point of a corner bracket, after the whole-turn shift, whose float test `math.Abs(phi.mid()-ref) > 3` is false lies within `3 + 2^-20` of `ref` | hypotheses `href`, `hfar` of `atan2Box_sound`; the fields of `BoxDecisions` | (a) Under A2 both midpoints are finite; `Lo + (Hi-Lo)/2` rounded to nearest lies in `[Lo, Hi]`, so `(c.mid(), d.mid())` is a point of the rectangle, and `math.Atan2` is accurate to a few ulps. (b) `math.Atan2` returns `\|phi\| ≤ π`, so `atan2Point`'s bracket is at most `2·2^20·(1+π)·1e-15` wide plus rounding (under `1e-8`); the shift by `twoPiTimes(1)` widens it by under `1e-15`; `mid`, the subtraction and `math.Abs` round by under `1e-15` | `interval_trig.go` `atan2Box`, `certifiedArcSpanBounds`; `interval.go` `Interval.mid`; `enclose_system.go` `certDrivenOf`; `enclose.go` `encloseRun.piece` |
| A50 | `big.Rat.Float64` returns the round-to-nearest float64 of a rational, and `math.Nextafter(f, ±Inf)` returns the adjacent float64 below or above `f`. A result that overflows to `±Inf` is refused by `coneFinite` and is not modeled. The float divider of `coneSplit` (`math.Hypot`, division) is unconstrained: only its exact cross-product check is relied on | `ConeArcEnv` fields `rn_mem`, `rn_near`, `prev_mem`, `prev_lt`, `next_mem`, `lt_next`; the unconstrained field `divider` | Go `math/big` docs for `Rat.Float64`; `math` docs for `Nextafter`; the `coneFinite` checks in `coneOutward` and `coneSplit` | `cone_side_arc.go` `coneOutward`, `coneSplit`, `coneArcWedge.box`, `coneArcWedge.side`, `coneSideForm.arcWedgeSide`; `cone_side.go` `coneSideForm.arcSide`, `Sketch.CertifyConeCurveSide` |
| A51 | The `math/big` steps of `coneSqrtBounds` and `coneUnit` are exact: `big.Rat` `Num`, `Denom` (lowest terms, positive denominator), `SetFrac`, `SetInt`, `Inv`; `big.Int` `Set`, `Lsh`, `Add`, `BitLen` (the bit length, `Nat.size`), `Quo` (of positive operands, `ℕ` division) and `Sqrt` (the floor square root, `Nat.sqrt`). Go's `int` division truncates, and the shift counts `2*k` fit in `uint` | `sqrtK`, `sqrtNum`, `sqrtDen`, `sqrtRoot`, `sqrtScale`; `⁻¹` in `coneUnit` | Go `math/big` docs for `Int.Sqrt`, `Int.BitLen`, `Int.Lsh`, `Int.Quo`, `Rat.Inv`, `Rat.SetFrac`; Go spec "Integer operators" | `cone_side_arc.go` `coneSqrtBounds`, `coneUnit`, `coneArcWedge.side`, `coneSideForm.arcWedgeSide`; `cone_side.go` `coneSideForm.arcQuadrantSide`, `coneSideForm.arcSide`, `Sketch.CertifyConeCurveSide` |
| A52 | `arcQuadrantSide` computes finite floats `Y` and `U` from `Arc.R()` and the float differences `Start − Center`, `End − Center`, and its comparisons `U < 0`, `Y > 0` on them are exact. Every other check and the box use the exact rays and the exact radius bracket. The float guard (finite values, `r > 0`, the float difference signs, `Arc.Sweep()`) and the `delta` check only refuse and are not modeled | `arcQuadrantSide` takes `Y`, `U` as given rationals; `ConeArcData` fields `Y`, `U` | Go spec "Comparison operators"; code reading of `arcQuadrantSide` | `cone_side.go` `coneSideForm.arcQuadrantSide`, `coneSideForm.arcSide`, `Sketch.CertifyConeCurveSide` |
| A100 | The `math/big` steps of `coneFitIntPoly` and `coneFitIntSign` are exact: `big.Rat` `Num` and `Denom` are the numerator and the positive denominator in lowest terms; `big.Int` `GCD` of positive operands is the greatest common divisor, `Quo` of positive operands is `ℕ` division, and `Mul`, `Add`, `Sign` are the exact integer results | `Rat.num`, `Rat.den`, `Nat.gcd`, `ℕ` division and `ℤ` arithmetic in `coneFitIntDen`, `coneFitIntPoly`, `coneFitIntEval`, `coneFitIntSign` | Go `math/big` docs for `Rat.Num`, `Rat.Denom`, `Int.GCD`, `Int.Quo`, `Int.Mul`, `Int.Add`, `Int.Sign` | `cone_fit_crossing.go` `coneFitIntPoly`, `coneFitIntSign`, `Sketch.CertifyConeFitCrossing` |
| A110 | The `math/big` steps of `coneFitIntBernstein` and `coneFitIntBounds` are exact: `big.Int` `Mul`, `Add`, `Sub`, `SetInt64` and `Cmp` give the exact integer results, and `big.Rat` `Num`, `Denom` are the numerator and positive denominator in lowest terms. The integers are modeled as the rationals they equal. `coneFitIntBernstein` runs on a nonempty polynomial, as `b[0]` of its result is read | ℚ arithmetic in `coneFitIntStep`, `coneFitIntCompose`, `coneFitIntBernstein`, `coneFitIntBounds`; `Rat.num`, `Rat.den` | Go `math/big` docs for `Int.Mul`, `Int.Add`, `Int.Sub`, `Int.SetInt64`, `Int.Cmp`, `Rat.Num`, `Rat.Denom`; `coneFitPolyDeriv` returns `[0]` for a constant, so `h`, `f`, `df` are nonempty | `cone_fit_crossing.go` `coneFitIntBernstein`, `coneFitIntBounds`, `coneFitSearch.visit`, `Sketch.CertifyConeFitCrossing` |
| A111 | `coneFitFactorial(n)` and the weight `coneFitBinomial(k,i)·coneFitFactorial(i)·coneFitFactorial(n−i)` in `coneFitIntBernstein` do not overflow `int64`: `n <= 6`, the degree of `f`, so the weight is at most `n! = 720` | `coneFitFactorial` and the weight over ℕ; `coneFitFactorial_eq` proves it is `Nat.factorial` | The weight equals `n!·C(k,i)/C(n,i) <= n!` for `i <= k <= n`; the degrees come from `coneFitResidual` | `cone_fit_crossing.go` `coneFitFactorial`, `coneFitIntBernstein` |
| A150 | `sinCosSeriesPair` either reports false or returns the intervals `sinCosSeriesBig` returns for the same `x` at `fb = pairFracBits`: on `2^-40 ≤ \|x\| ≤ pairMaxArg` every integer the series forms at that precision fits its two words, so its 128-bit steps form the same integers | the model of `sinCosPoint` calls `sinCosSeriesBig` at `TrigEnv.fracBits x` | `TestSinCosSeriesWordsMatchesBig` (the pair half of `requireSameSeries`); the step-by-step argument and the size bounds in the doc comment of `sinCosSeriesPair` | `interval_trig.go` `sinCosSeriesPair`, `sinCosPoint` |
| A160 | A float sum formed left to right from `L` terms, each exact or one product of floats rounded once to nearest, differs from the exact sum of the exact terms by at most `γ_L·P + 2L·η`, with `P` the sum of the rounded terms' magnitudes, `γ_L = L·u/(1 − L·u)`, `u = 2^-53`, `η = 2^-1075`. `krawczyk` charges `up(up(t·sumGain(M)) + k·2^-1074)`, with `t` the float sum of the terms' magnitudes, `M` at least two more than the longest sum, and `k·2^-1074` at least `2η` per rounded product the sums hold; that covers the bound and the rounding of `t` while `L·u ≤ 2^-30`. A cell's magnitude sum starts at `2^-990` (or at `1` on the diagonal), so the cell's underflow, `2L·η`, is a negligible fraction of `γ_L·t` and is charged with it after the multiplication by `max(−D[j].Lo, D[j].Hi)` | `KrawczykRun.Assumptions.residual`, `slope`, `image` take the bounds as given | Higham, *Accuracy and Stability of Numerical Algorithms*, 2nd ed.: §2.1 (a product rounds as `xy(1 + δ) + ε` with `\|δ\| ≤ u`, `\|ε\| ≤ η`; a sum underflows exactly) and Lemma 3.1 with eq. (3.4); A1, A4. `L ≤ n + 2` for `n` free variables, and `L·u ≤ 2^-30` needs `n` near `2^23`, a Jacobian of `2^46` cells | `enclose_system.go certSystem.krawczyk`, `sumGain` |
| A43 | On float64 values, `a - a` is `+0` for finite `a`, and `+0 * b` is `+0` for finite `b`; `nonFinite` is the complement of "finite"; each constraint's residual is a pure function of the variables it reads, so equal inputs give equal float rows | `Ops.sub_self`, `Ops.zero_mul`, `Ops.fin`; `Residual.F` is a function | IEEE 754 binary64; `nonFinite` in `non_finite.go`; the `1/(2h)` finiteness check in `jacobianLocalInto` | `jacobian.go Sketch.jacobianLocalInto`; `solver.go Sketch.jacobianInto` |
| A44 | For a constraint the plan accepts, every solver variable its `residual` reads is in the set `constraintVarIndices` returns: operand points and entity points and shape variables, plus its own auxiliary variables | `hcover : R.reads k ⊆ P.idx k` for accepted `k` | Code reading of `constraintRefs`, `entityPoints`, `entityShapeVars`, `auxVars`; `jacobian_inventory_test.go` (all constraint kinds); the four refusals (names nothing, foreign operand, foreign auxiliary variables, index out of range) in `constraintVarIndices` and `buildResidualPlan` | `jacobian.go Sketch.constraintVarIndices`, `Sketch.buildResidualPlan` |
| A45 | Row `i` of the plan belongs to the constraint whose measured `[offset, offset+count)` range holds it, in `residuals()` order with driven dimensions skipped; `evalRows` returning true means each reached constraint produced exactly its recorded rows at the perturbed point, equal to those rows of `residuals()`; `consFor` lists the constraints whose index set holds the variable; `s.vars[vi]` is restored exactly after each column | `Residual.own`, `Residual.F`, `Plan.consFor`, `evalOK`, `hbase : base i = F x i` | Code reading of `buildResidualPlan`, `evalRows` (capacity-limited append plus length check) and `consFor`; `jacobian_test.go` | `jacobian.go Sketch.jacobianLocalInto`, `Sketch.buildResidualPlan`, `residualPlan.consFor`, `residualPlan.evalRows`; `solver.go Sketch.jacobianInto` |

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
