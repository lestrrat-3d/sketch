module

public import Mathlib

/-!
# Abstract binary64 model

Abstract model of the finite binary64 floats the sketch engine's certified path computes with. Overflow to
`±Inf` is not modeled: every rounded result is a finite float.

* `F`   the finite floats, closed under negation, containing 0.
* `rn`  round-to-nearest: a float at least as close to `x` as any float.
* `nu`  the next float up: the least float strictly above `f`.
* `δ`   the smallest positive float; every sum of two floats is 0 or at least `δ` in magnitude, because
        all floats are multiples of `δ`.

`nd f = -nu (-f)` is the next float down, the greatest float strictly below `f`.

`Sketch.Float.gridModel` shows the fields are satisfiable. A concrete binary64 instance would discharge the
float assumptions of `.claude/docs/proofs.md`; theorems that take `M : FloatModel` need no edit.
-/

public section

namespace Sketch.Float

/-- Abstract finite binary64 arithmetic: a set of floats, round-to-nearest and the next float up. -/
structure FloatModel where
  F : Set ℝ
  rn : ℝ → ℝ
  nu : ℝ → ℝ
  δ : ℝ
  zero_mem : (0 : ℝ) ∈ F
  neg_mem : ∀ a ∈ F, -a ∈ F
  δ_pos : 0 < δ
  δ_mem : δ ∈ F
  rn_mem : ∀ x, rn x ∈ F
  rn_near : ∀ x, ∀ f ∈ F, |rn x - x| ≤ |f - x|
  nu_mem : ∀ f ∈ F, nu f ∈ F
  lt_nu : ∀ f ∈ F, f < nu f
  nu_min : ∀ f ∈ F, ∀ g ∈ F, f < g → nu f ≤ g
  add_grid : ∀ a ∈ F, ∀ b ∈ F, a + b = 0 ∨ δ ≤ |a + b|

namespace FloatModel
variable (M : FloatModel)

/-- The next float down: the greatest float strictly below `f`. -/
@[expose] def nd (f : ℝ) : ℝ := -M.nu (-f)

/-- Rounding to nearest then stepping up never lands below `x`. -/
theorem up_ge (x : ℝ) : x ≤ M.nu (M.rn x) := by
  by_contra h
  push Not at h
  have hf := M.rn_mem x
  have hlt := M.lt_nu _ hf
  have hnu := M.nu_mem _ hf
  have hnear := M.rn_near x _ hnu
  have h1 : M.rn x < x := by linarith
  rw [abs_of_neg (by linarith : M.rn x - x < 0),
      abs_of_neg (by linarith : M.nu (M.rn x) - x < 0)] at hnear
  linarith

/-- Rounding a non-negative real to nearest gives a non-negative float. -/
theorem rn_nonneg {x : ℝ} (hx : 0 ≤ x) : 0 ≤ M.rn x := by
  have h0 := M.rn_near x 0 M.zero_mem
  have e : |(0:ℝ) - x| = x := by rw [zero_sub, abs_neg, abs_of_nonneg hx]
  rw [e] at h0
  by_contra h
  push Not at h
  rw [abs_of_neg (by linarith : M.rn x - x < 0)] at h0
  linarith

/-- The next float down of a float is a float. -/
theorem nd_mem {f : ℝ} (hf : f ∈ M.F) : M.nd f ∈ M.F :=
  M.neg_mem _ (M.nu_mem _ (M.neg_mem _ hf))

/-- The next float down of a float lies strictly below it. -/
theorem nd_lt {f : ℝ} (hf : f ∈ M.F) : M.nd f < f := by
  have := M.lt_nu _ (M.neg_mem _ hf)
  unfold nd
  linarith

/-- Rounding to nearest then stepping down never lands above `x`. -/
theorem down_le (x : ℝ) : M.nd (M.rn x) ≤ x := by
  by_contra h
  push Not at h
  have hf := M.rn_mem x
  have hlt := M.nd_lt hf
  have hnear := M.rn_near x _ (M.nd_mem hf)
  rw [abs_of_pos (by linarith : 0 < M.rn x - x),
      abs_of_pos (by linarith : 0 < M.nd (M.rn x) - x)] at hnear
  linarith

/-- The next float up is monotone on floats. -/
theorem nu_mono {f g : ℝ} (hf : f ∈ M.F) (hg : g ∈ M.F) (hfg : f ≤ g) : M.nu f ≤ M.nu g := by
  rcases hfg.lt_or_eq with h | h
  · exact (M.nu_min f hf g hg h).trans (M.lt_nu g hg).le
  · rw [h]

/-- The next float down is monotone on floats. -/
theorem nd_mono {f g : ℝ} (hf : f ∈ M.F) (hg : g ∈ M.F) (hfg : f ≤ g) : M.nd f ≤ M.nd g := by
  have := M.nu_mono (M.neg_mem _ hg) (M.neg_mem _ hf) (neg_le_neg hfg)
  unfold nd
  linarith

end FloatModel

end Sketch.Float

end
