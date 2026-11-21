module

public import Mathlib

/-!
# Intervals, outward conversions and float-computed decisions

The pieces of `interval_trig.go` the trigonometric models share. Float64 values are read as the reals they
denote (assumption A28), so an `Interval` is a pair of reals, `sketch.clampUnit` and `sketch.ihull` are `max`
and `min` on them.

`Rounding` collects what the proofs need from the four conversions that round an exact value to a float64:
`sketch.ratLower` and `sketch.ratUpper` (exact rational to float, A24) and `sketch.fixedLower` and
`sketch.fixedUpper` (fixed-point integer to float, A23). Each field is a hypothesis carried by an assumption
row, not a model of the Go code. `TrigEnv` collects the decisions the Go code computes in float64 and the
proofs take as inputs: the series' fixed-point precision and stopping test, the number of whole turns
`sketch.reduceTurns` removes, and `sketch.sinCosRange`'s width test and `k` range. Only the `k` range carries a hypothesis (A27); soundness
holds whatever the other decisions are.

`Rounding.exact` and `TrigEnv.exact` show the fields are jointly satisfiable, so no theorem about an
arbitrary `Rounding` or `TrigEnv` holds only because the structure is inconsistent.
-/

public section

namespace Sketch.Trig

open Real

/-- Model of `sketch.Interval`: endpoints read as reals (A28). -/
structure Ival where
  lo : ℝ
  hi : ℝ

/-- `y` lies in the closed interval `I`. -/
@[expose] def Ival.Mem (I : Ival) (y : ℝ) : Prop := I.lo ≤ y ∧ y ≤ I.hi

/-- Model of `sketch.clampUnit`. -/
@[expose] noncomputable def clampUnit (i : Ival) : Ival := ⟨max i.lo (-1), min i.hi 1⟩

/-- Model of `sketch.ihull`. -/
@[expose] noncomputable def hull (a b : Ival) : Ival := ⟨min a.lo b.lo, max a.hi b.hi⟩

/-- `sketch.clampUnit` keeps every value of `[-1, 1]` that its input interval holds. -/
theorem clampUnit_mem {i : Ival} {y : ℝ} (h : i.Mem y) (h1 : -1 ≤ y) (h2 : y ≤ 1) : (clampUnit i).Mem y :=
  ⟨max_le h.1 h1, le_min h.2 h2⟩

/-- `sketch.ihull` holds every value either input holds. -/
theorem hull_mem_left {a b : Ival} {y : ℝ} (h : a.Mem y) : (hull a b).Mem y :=
  ⟨(min_le_left _ _).trans h.1, h.2.trans (le_max_left _ _)⟩

/-- `sketch.ihull` holds every value either input holds. -/
theorem hull_mem_right {a b : Ival} {y : ℝ} (h : b.Mem y) : (hull a b).Mem y :=
  ⟨(min_le_right _ _).trans h.1, h.2.trans (le_max_right _ _)⟩

/-- The outward conversions of `interval_trig.go`, by the properties the proofs use.

* `ratLower r ≤ r ≤ ratUpper r` for an exact rational `r` (A24).
* `ratExact r` is the `exact` flag of `big.Rat.Float64`; when it is false, neither conversion returns `r`
  itself (A24).
* `fixedLower F v ≤ v / 2^F ≤ fixedUpper F v` for a fixed-point integer `v` counting `2^-F` (A23). -/
structure Rounding where
  ratLower : ℝ → ℝ
  ratUpper : ℝ → ℝ
  ratExact : ℝ → Bool
  fixedLower : ℕ → ℤ → ℝ
  fixedUpper : ℕ → ℤ → ℝ
  ratLower_le : ∀ r, ratLower r ≤ r
  le_ratUpper : ∀ r, r ≤ ratUpper r
  ratLower_ne : ∀ r, ratExact r = false → ratLower r ≠ r
  ratUpper_ne : ∀ r, ratExact r = false → ratUpper r ≠ r
  fixedLower_le : ∀ (F : ℕ) (v : ℤ), fixedLower F v ≤ (v : ℝ) / 2 ^ F
  le_fixedUpper : ∀ (F : ℕ) (v : ℤ), (v : ℝ) / 2 ^ F ≤ fixedUpper F v

/-- Exact conversions satisfy every `Rounding` field. -/
noncomputable def Rounding.exact : Rounding where
  ratLower r := r
  ratUpper r := r
  ratExact _ := true
  fixedLower F v := (v : ℝ) / 2 ^ F
  fixedUpper F v := (v : ℝ) / 2 ^ F
  ratLower_le _ := le_rfl
  le_ratUpper _ := le_rfl
  ratLower_ne _ h := absurd h (by simp)
  ratUpper_ne _ h := absurd h (by simp)
  fixedLower_le _ _ := le_rfl
  le_fixedUpper _ _ := le_rfl

/-- The decisions the Go code computes in float64, taken as inputs.

* `fracBits x`: `sketch.seriesFracBits(x)`, the precision `F` of the series' fixed-point integers.
* `stop x n bound s c`: the break test of `sketch.sinCosSeriesBig` on the term index, the tail bound and the
  two sums: `float64(n) > ax+1` and either `bound < eps` or `bound < relMax` with `bound·2^trigRelBits` at
  most `|s|` and `|c|`, where `eps` and `relMax` are powers of two set by `fracBits x`.
* `turns x`: `int64(math.Round(x / (2 * math.Pi)))` in `sketch.reduceTurns`.
* `wide q`: `q.Hi-q.Lo > 7` in `sketch.sinCosRange`.
* `kmin q`, `kmax q`: `int(math.Floor(q.Lo/(math.Pi/2))) - 1` and `int(math.Ceil(q.Hi/(math.Pi/2))) + 1`.
* `k_cover`: every multiple `k·π/2` of a narrow `q` whose ends are within `2^40` has `kmin q ≤ k ≤ kmax q`
  (A27). -/
structure TrigEnv where
  fracBits : ℝ → ℕ
  stop : ℝ → ℕ → ℤ → ℤ → ℤ → Bool
  turns : ℝ → ℤ
  wide : Ival → Bool
  kmin : Ival → ℤ
  kmax : Ival → ℤ
  k_cover : ∀ q : Ival, wide q = false → |q.lo| ≤ 2 ^ 40 → |q.hi| ≤ 2 ^ 40 →
    ∀ k : ℤ, q.lo ≤ k * π / 2 → k * π / 2 ≤ q.hi → kmin q ≤ k ∧ k ≤ kmax q

/-- The exact floor and ceiling of `q / (π/2)` satisfy `TrigEnv.k_cover`. -/
noncomputable def TrigEnv.exact : TrigEnv where
  fracBits _ := 192
  stop _ _ _ _ _ := true
  turns _ := 0
  wide _ := false
  kmin q := ⌊q.lo / (π / 2)⌋
  kmax q := ⌈q.hi / (π / 2)⌉
  k_cover q _ _ _ k h1 h2 := by
    have hp : (0 : ℝ) < π / 2 := by positivity
    constructor
    · refine Int.floor_le_iff.mpr ?_
      have : q.lo / (π / 2) ≤ k := by
        rw [div_le_iff₀ hp]; linarith
      linarith
    · refine Int.le_ceil_iff.mpr ?_
      have : (k : ℝ) ≤ q.hi / (π / 2) := by
        rw [le_div_iff₀ hp]; linarith
      linarith

end Sketch.Trig

end
