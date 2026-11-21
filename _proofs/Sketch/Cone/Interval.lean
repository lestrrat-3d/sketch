module

public import Mathlib

/-!
# Exact rational intervals

Models `sketch.coneInterval` and its operations `sketch.coneIntervalOf`, `coneInterval.add`,
`coneInterval.sub`, `coneInterval.scale` and `coneInterval.square` (`cone_side.go`). The Go bounds are
`*big.Rat` values, so each operation is exact rational arithmetic (A30) and no rounding is modeled.

Each operation is proved inclusion-sound: when `x ∈ a` and `y ∈ b`, the exact result of the matching
arithmetic on `x` and `y` lies in the result interval (`mem_add`, `mem_sub`, `mem_scale`, `mem_square`).
-/

public section

namespace Sketch.Cone

/-- Model of `sketch.coneInterval`: the closed rational interval `[lo, hi]`. -/
structure ConeInterval where
  lo : ℚ
  hi : ℚ

namespace ConeInterval

/-- `x` lies in `[a.lo, a.hi]`. -/
@[expose] def Mem (a : ConeInterval) (x : ℚ) : Prop := a.lo ≤ x ∧ x ≤ a.hi

/-- Model of `coneInterval.add`. -/
@[expose] def add (a b : ConeInterval) : ConeInterval := ⟨a.lo + b.lo, a.hi + b.hi⟩

/-- Model of `coneInterval.sub`. -/
@[expose] def sub (a b : ConeInterval) : ConeInterval := ⟨a.lo - b.hi, a.hi - b.lo⟩

end ConeInterval

/-- Model of `sketch.coneIntervalOf`: orders two bounds (`a.Cmp(b) <= 0` keeps them as given). -/
@[expose] def coneIntervalOf (a b : ℚ) : ConeInterval := if a ≤ b then ⟨a, b⟩ else ⟨b, a⟩

namespace ConeInterval

/-- Model of `coneInterval.scale`. -/
@[expose] def scale (a : ConeInterval) (k : ℚ) : ConeInterval := coneIntervalOf (a.lo * k) (a.hi * k)

/-- Model of `coneInterval.square`. -/
@[expose] def square (a : ConeInterval) : ConeInterval :=
  if 0 ≤ a.lo then ⟨a.lo * a.lo, a.hi * a.hi⟩
  else if a.hi ≤ 0 then ⟨a.hi * a.hi, a.lo * a.lo⟩
  else if a.lo * a.lo < a.hi * a.hi then ⟨0, a.hi * a.hi⟩
  else ⟨0, a.lo * a.lo⟩

/-- A one-point interval `{x, x}` holds `x`. -/
theorem mem_point (x : ℚ) : (⟨x, x⟩ : ConeInterval).Mem x := ⟨le_refl x, le_refl x⟩

/-- Both bounds given to `coneIntervalOf` lie in its result. -/
theorem mem_coneIntervalOf (a b : ℚ) :
    (coneIntervalOf a b).Mem a ∧ (coneIntervalOf a b).Mem b := by
  unfold coneIntervalOf Mem
  split_ifs with h
  · exact ⟨⟨le_refl a, h⟩, ⟨h, le_refl b⟩⟩
  · exact ⟨⟨by linarith, le_refl a⟩, ⟨le_refl b, by linarith⟩⟩

/-- Every value between the two bounds given to `coneIntervalOf`, in either order, lies in its result. -/
theorem mem_coneIntervalOf_between {a b x : ℚ} (h : (a ≤ x ∧ x ≤ b) ∨ (b ≤ x ∧ x ≤ a)) :
    (coneIntervalOf a b).Mem x := by
  unfold coneIntervalOf Mem
  split_ifs <;> rcases h with ⟨h1, h2⟩ | ⟨h1, h2⟩ <;> constructor <;> linarith

/-- `add` is inclusion-sound. -/
theorem mem_add {a b : ConeInterval} {x y : ℚ} (hx : a.Mem x) (hy : b.Mem y) : (a.add b).Mem (x + y) :=
  ⟨by simp only [add]; linarith [hx.1, hy.1], by simp only [add]; linarith [hx.2, hy.2]⟩

/-- `sub` is inclusion-sound. -/
theorem mem_sub {a b : ConeInterval} {x y : ℚ} (hx : a.Mem x) (hy : b.Mem y) : (a.sub b).Mem (x - y) :=
  ⟨by simp only [sub]; linarith [hx.1, hy.2], by simp only [sub]; linarith [hx.2, hy.1]⟩

/-- `scale` is inclusion-sound for every factor sign. -/
theorem mem_scale {a : ConeInterval} {x : ℚ} (k : ℚ) (hx : a.Mem x) : (a.scale k).Mem (x * k) := by
  obtain ⟨h1, h2⟩ := hx
  unfold scale
  apply mem_coneIntervalOf_between
  rcases le_total 0 k with hk | hk
  · exact Or.inl ⟨mul_le_mul_of_nonneg_right h1 hk, mul_le_mul_of_nonneg_right h2 hk⟩
  · exact Or.inr ⟨mul_le_mul_of_nonpos_right h2 hk, mul_le_mul_of_nonpos_right h1 hk⟩

/-- `square` is inclusion-sound: it holds `x·x` for every `x` in the interval, across all four branches. -/
theorem mem_square {a : ConeInterval} {x : ℚ} (hx : a.Mem x) : a.square.Mem (x * x) := by
  obtain ⟨h1, h2⟩ := hx
  unfold square Mem
  split_ifs with ha hb hc
  · exact ⟨by nlinarith, by nlinarith⟩
  · exact ⟨by nlinarith, by nlinarith⟩
  · refine ⟨mul_self_nonneg x, ?_⟩
    rcases le_total 0 x with h0 | h0 <;> nlinarith
  · refine ⟨mul_self_nonneg x, ?_⟩
    rcases le_total 0 x with h0 | h0 <;> nlinarith

/-- The Go "lower `lo` when `p < lo`" step is `min`. -/
theorem ite_lt_eq_min (p a : ℚ) : (if p < a then p else a) = min p a := by
  split_ifs with h
  · exact (min_eq_left h.le).symm
  · exact (min_eq_right (le_of_not_gt h)).symm

/-- The Go "raise `hi` when `p > hi`" step is `max`. -/
theorem ite_gt_eq_max (p a : ℚ) : (if a < p then p else a) = max a p := by
  split_ifs with h
  · exact (max_eq_right h.le).symm
  · exact (max_eq_left (le_of_not_gt h)).symm

end ConeInterval

end Sketch.Cone

end
