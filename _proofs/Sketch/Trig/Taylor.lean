module

public import Mathlib

/-!
# Taylor remainder of sine and cosine

The tail bound that `sketch.sinCosSeriesBig` adds to both sides of its sums. The Taylor coefficients of sine
and cosine at 0 are `0, 1, 0, -1` and `1, 0, -1, 0` by `n mod 4` (the `switch n % 4` of the Go loop), and the
Lagrange form of the remainder bounds what the first `N` terms leave out by `|x|^N / N!` for every real `x` and
every `N`, because every derivative of sine and cosine lies in `[-1, 1]`.
-/

public section

namespace Sketch.Trig

open Real Nat

/-- The coefficient of `x^n/n!` in the Taylor series of sine at 0: `0, 1, 0, -1` by `n mod 4`. -/
@[expose] def sinSign (n : ℕ) : ℤ :=
  match n % 4 with
  | 1 => 1
  | 3 => -1
  | _ => 0

/-- The coefficient of `x^n/n!` in the Taylor series of cosine at 0: `1, 0, -1, 0` by `n mod 4`. -/
@[expose] def cosSign (n : ℕ) : ℤ :=
  match n % 4 with
  | 0 => 1
  | 2 => -1
  | _ => 0

theorem sinSign_succ (n : ℕ) : sinSign (n + 1) = cosSign n := by
  have h : n % 4 = 0 ∨ n % 4 = 1 ∨ n % 4 = 2 ∨ n % 4 = 3 := by omega
  rcases h with h | h | h | h <;> simp [sinSign, cosSign, Nat.add_mod, h]

theorem cosSign_succ (n : ℕ) : cosSign (n + 1) = -sinSign n := by
  have h : n % 4 = 0 ∨ n % 4 = 1 ∨ n % 4 = 2 ∨ n % 4 = 3 := by omega
  rcases h with h | h | h | h <;> simp [sinSign, cosSign, Nat.add_mod, h]

theorem abs_sinSign_le (n : ℕ) : |sinSign n| ≤ 1 := by
  have h : n % 4 = 0 ∨ n % 4 = 1 ∨ n % 4 = 2 ∨ n % 4 = 3 := by omega
  rcases h with h | h | h | h <;> simp [sinSign, h]

theorem abs_cosSign_le (n : ℕ) : |cosSign n| ≤ 1 := by
  have h : n % 4 = 0 ∨ n % 4 = 1 ∨ n % 4 = 2 ∨ n % 4 = 3 := by omega
  rcases h with h | h | h | h <;> simp [cosSign, h]

/-- The `n`-th derivatives of sine and cosine at 0 are `sinSign n` and `cosSign n`. -/
theorem iteratedDeriv_sin_cos_zero (n : ℕ) :
    iteratedDeriv n sin 0 = sinSign n ∧ iteratedDeriv n cos 0 = cosSign n := by
  induction n with
  | zero => simp [sinSign, cosSign]
  | succ n ih =>
    rw [iteratedDeriv_add_one_sin, iteratedDeriv_add_one_cos, Pi.neg_apply, ih.1, ih.2,
      sinSign_succ, cosSign_succ]
    push_cast
    exact ⟨rfl, rfl⟩

/-- The Lagrange remainder of a function whose every derivative lies in `[-1, 1]`: the Taylor polynomial of
degree `n` at 0 misses `f x` by at most `|x|^(n+1)/(n+1)!`. -/
theorem abs_sub_taylor_le_of_deriv_bound {f : ℝ → ℝ} (hf : ContDiff ℝ ⊤ f)
    (hb : ∀ k y, |iteratedDeriv k f y| ≤ 1) (x : ℝ) (n : ℕ) :
    |f x - ∑ k ∈ Finset.range (n + 1), iteratedDeriv k f 0 * x ^ k / k !| ≤ |x| ^ (n + 1) / (n + 1)! := by
  by_cases hx : x = 0
  · subst hx
    rw [Finset.sum_range_succ']
    simp
  have hne : (0 : ℝ) ≠ x := Ne.symm hx
  obtain ⟨x', -, hx'⟩ :=
    taylor_mean_remainder_lagrange_iteratedDeriv (n := n) hne (hf.of_le le_top).contDiffOn
  have hT : taylorWithinEval f n (Set.uIcc 0 x) 0 x =
      ∑ k ∈ Finset.range (n + 1), iteratedDeriv k f 0 * x ^ k / k ! := by
    rw [taylor_within_apply]
    refine Finset.sum_congr rfl fun k _ => ?_
    rw [iteratedDerivWithin_eq_iteratedDeriv (uniqueDiffOn_uIcc hne)
      ((hf.of_le le_top).contDiffAt) Set.left_mem_uIcc]
    simp only [sub_zero, smul_eq_mul]
    field_simp
  rw [← hT, hx', sub_zero, abs_div, abs_mul, abs_pow, abs_of_pos (by positivity : (0 : ℝ) < (n + 1)!)]
  gcongr
  calc |iteratedDeriv (n + 1) f x'| * |x| ^ (n + 1) ≤ 1 * |x| ^ (n + 1) := by
        gcongr
        exact hb _ _
    _ = |x| ^ (n + 1) := one_mul _

/-- The first `N` terms of the Taylor series of sine and of cosine at 0 each miss the function by at most
`|x|^N / N!`, for every real `x` and every `N`. -/
theorem abs_sin_cos_sub_taylor_le (x : ℝ) (N : ℕ) :
    |sin x - ∑ j ∈ Finset.range N, (sinSign j : ℝ) * x ^ j / j !| ≤ |x| ^ N / N ! ∧
    |cos x - ∑ j ∈ Finset.range N, (cosSign j : ℝ) * x ^ j / j !| ≤ |x| ^ N / N ! := by
  cases N with
  | zero => simp [abs_sin_le_one, abs_cos_le_one]
  | succ n =>
    have hs := abs_sub_taylor_le_of_deriv_bound contDiff_sin abs_iteratedDeriv_sin_le_one x n
    have hc := abs_sub_taylor_le_of_deriv_bound contDiff_cos abs_iteratedDeriv_cos_le_one x n
    simp only [fun k => (iteratedDeriv_sin_cos_zero k).1, fun k => (iteratedDeriv_sin_cos_zero k).2] at hs hc
    exact ⟨hs, hc⟩

end Sketch.Trig

end
