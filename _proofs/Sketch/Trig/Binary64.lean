module

public import Sketch.Float.Model
public import Sketch.Trig.Range

/-!
# Outward conversions to binary64

Models of `sketch.ratLower`, `sketch.ratUpper`, `sketch.fixedLower` and `sketch.fixedUpper` over the abstract
binary64 model `Sketch.Float.FloatModel`. `big.Rat.Float64` and `big.Float.Float64` return the round-to-nearest
float `M.rn` of their exact value and report on which side it lies (A23, A24); `sketch.down` and `sketch.up`
are the next float down and up (`FloatModel.nd`, `FloatModel.nu`). Together they satisfy every field of
`Rounding` (`Rounding.ofFloat`), so `sinCosPoint_sound_binary64` holds with no conversion hypothesis left.
-/

public section

namespace Sketch.Trig

open Real Sketch.Float

variable (M : FloatModel)

/-- Round to nearest leaves a float unchanged. -/
theorem rn_of_mem {f : ℝ} (hf : f ∈ M.F) : M.rn f = f := by
  have h := M.rn_near f f hf
  rw [sub_self, abs_zero] at h
  exact sub_eq_zero.mp (abs_nonpos_iff.mp h)

/-- The `exact` flag of `big.Rat.Float64`: the nearest float equals the rational (A24). -/
@[expose] noncomputable def ratExact (r : ℝ) : Bool := decide (M.rn r = r)

/-- Model of `sketch.ratLower`: `r.Float64()` is `M.rn r` with flag `ratExact` (A24); `down` is `M.nd`. -/
@[expose] noncomputable def ratLower (r : ℝ) : ℝ :=
  if ratExact M r then M.rn r else if M.rn r > r then M.nd (M.rn r) else M.rn r

/-- Model of `sketch.ratUpper`: `r.Float64()` is `M.rn r` with flag `ratExact` (A24); `up` is `M.nu`. -/
@[expose] noncomputable def ratUpper (r : ℝ) : ℝ :=
  if ratExact M r then M.rn r else if M.rn r < r then M.nu (M.rn r) else M.rn r

/-- Model of `sketch.fixedLower`: the exact value `v·2^-192` (`SetInt`, `SetMantExp`), its `Float64` result
`M.rn` with accuracy `Above` exactly when that result exceeds the value (A23), and `down` as `M.nd`. -/
@[expose] noncomputable def fixedLower (v : ℤ) : ℝ :=
  let y : ℝ := v / 2 ^ 192
  if M.rn y > y then M.nd (M.rn y) else M.rn y

/-- Model of `sketch.fixedUpper`: as `fixedLower`, with accuracy `Below` and `up` as `M.nu` (A23). -/
@[expose] noncomputable def fixedUpper (v : ℤ) : ℝ :=
  let y : ℝ := v / 2 ^ 192
  if M.rn y < y then M.nu (M.rn y) else M.rn y

/-- `sketch.ratLower` rounds down. Assumptions A24, A28. -/
theorem ratLower_le (r : ℝ) : ratLower M r ≤ r := by
  unfold ratLower ratExact
  split_ifs with h1 h2
  · exact (of_decide_eq_true h1).le
  · exact M.down_le r
  · exact not_lt.mp h2

/-- `sketch.ratUpper` rounds up. Assumptions A24, A28. -/
theorem le_ratUpper (r : ℝ) : r ≤ ratUpper M r := by
  unfold ratUpper ratExact
  split_ifs with h1 h2
  · exact (of_decide_eq_true h1).ge
  · exact M.up_ge r
  · exact not_lt.mp h2

/-- When `big.Rat.Float64` reports inexact, `sketch.ratLower` does not return the rational itself.
Assumptions A24, A28. -/
theorem ratLower_ne (r : ℝ) (h : ratExact M r = false) : ratLower M r ≠ r := by
  have hne : M.rn r ≠ r := of_decide_eq_false h
  unfold ratLower
  rw [h]
  simp only [Bool.false_eq_true, ↓reduceIte]
  split_ifs
  · intro e
    exact hne (rn_of_mem M (e ▸ M.nd_mem (M.rn_mem r)))
  · exact hne

/-- When `big.Rat.Float64` reports inexact, `sketch.ratUpper` does not return the rational itself.
Assumptions A24, A28. -/
theorem ratUpper_ne (r : ℝ) (h : ratExact M r = false) : ratUpper M r ≠ r := by
  have hne : M.rn r ≠ r := of_decide_eq_false h
  unfold ratUpper
  rw [h]
  simp only [Bool.false_eq_true, ↓reduceIte]
  split_ifs
  · intro e
    exact hne (rn_of_mem M (e ▸ M.nu_mem _ (M.rn_mem r)))
  · exact hne

/-- `sketch.fixedLower` rounds `v·2^-192` down. Assumptions A23, A28. -/
theorem fixedLower_le (v : ℤ) : fixedLower M v ≤ (v : ℝ) / 2 ^ 192 := by
  unfold fixedLower
  dsimp only
  split_ifs with h
  · exact M.down_le _
  · exact not_lt.mp h

/-- `sketch.fixedUpper` rounds `v·2^-192` up. Assumptions A23, A28. -/
theorem le_fixedUpper (v : ℤ) : (v : ℝ) / 2 ^ 192 ≤ fixedUpper M v := by
  unfold fixedUpper
  dsimp only
  split_ifs with h
  · exact M.up_ge _
  · exact not_lt.mp h

/-- The binary64 conversions of `interval_trig.go` as a `Rounding`. -/
noncomputable def Rounding.ofFloat : Rounding where
  ratLower := Sketch.Trig.ratLower M
  ratUpper := Sketch.Trig.ratUpper M
  ratExact := Sketch.Trig.ratExact M
  fixedLower := Sketch.Trig.fixedLower M
  fixedUpper := Sketch.Trig.fixedUpper M
  ratLower_le := Sketch.Trig.ratLower_le M
  le_ratUpper := Sketch.Trig.le_ratUpper M
  ratLower_ne := Sketch.Trig.ratLower_ne M
  ratUpper_ne := Sketch.Trig.ratUpper_ne M
  fixedLower_le := Sketch.Trig.fixedLower_le M
  le_fixedUpper := Sketch.Trig.le_fixedUpper M

/-- **Certified sine and cosine on binary64.** With the conversions of `interval_trig.go` modeled on the
abstract binary64 floats, whenever the model of `sketch.sinCosPoint` returns, its first interval contains
`sin x` and its second `cos x`. Covers `sketch.sinCosPoint` and every function it reaches, including
`sketch.ratLower`, `sketch.ratUpper`, `sketch.fixedLower` and `sketch.fixedUpper`. Assumptions: A20 to A28. -/
theorem sinCosPoint_sound_binary64 (E : TrigEnv) (fuel : ℕ) (x : ℝ) (S C : Ival)
    (h : sinCosPoint (Rounding.ofFloat M) E fuel x = some (S, C)) : S.Mem (sin x) ∧ C.Mem (cos x) :=
  sinCosPoint_sound (Rounding.ofFloat M) E fuel x S C h

end Sketch.Trig

end
