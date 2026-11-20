module

public import Sketch.Float.Model

/-!
# Outward-rounded interval arithmetic

Models the interval helpers of `interval.go`: `sketch.down`, `sketch.up`, `sketch.pt`, `sketch.iadd`,
`sketch.isub`, `sketch.ineg`, `sketch.imul`, `sketch.imulPt`, `sketch.isqr`, `sketch.isqrtNonNeg`,
`sketch.ihull` and `sketch.Interval.strictlyInside`. An endpoint is the exact real value of its float64. Each
rounded Go operation is `M.rn` of the exact real result, `up` is the next float up and `down` the next float down.

Each soundness theorem states that the result interval contains the exact real result for every choice of
operands from the input intervals.
-/

public section

namespace Sketch.Interval

open Sketch.Float

/-- Model of `sketch.Interval`: the closed interval `[lo, hi]`, endpoints read as exact reals. Named `Ival`
because `Sketch.Interval.Interval` repeats the namespace. -/
structure Ival where
  lo : ℝ
  hi : ℝ

/-- `x` lies in the closed interval `[a.lo, a.hi]`. -/
@[expose] def Ival.Mem (a : Ival) (x : ℝ) : Prop := a.lo ≤ x ∧ x ≤ a.hi

variable (M : FloatModel)

/-- Model of `sketch.down`: the next float down. -/
@[expose] def down (x : ℝ) : ℝ := M.nd x

/-- Model of `sketch.up`: the next float up. -/
@[expose] def up (x : ℝ) : ℝ := M.nu x

/-- Model of `sketch.pt`. -/
@[expose] def pt (v : ℝ) : Ival := ⟨v, v⟩

/-- Model of `sketch.iadd`. -/
@[expose] def iadd (a b : Ival) : Ival :=
  ⟨down M (M.rn (a.lo + b.lo)), up M (M.rn (a.hi + b.hi))⟩

/-- Model of `sketch.isub`. -/
@[expose] def isub (a b : Ival) : Ival :=
  ⟨down M (M.rn (a.lo - b.hi)), up M (M.rn (a.hi - b.lo))⟩

/-- Model of `sketch.ineg`. Float negation is exact. -/
@[expose] def ineg (a : Ival) : Ival := ⟨-a.hi, -a.lo⟩

/-- Model of `sketch.imul`. Go's four-argument `min`/`max` is folded left to right. -/
@[expose] noncomputable def imul (a b : Ival) : Ival :=
  let p1 := M.rn (a.lo * b.lo)
  let p2 := M.rn (a.lo * b.hi)
  let p3 := M.rn (a.hi * b.lo)
  let p4 := M.rn (a.hi * b.hi)
  ⟨down M (min (min (min p1 p2) p3) p4), up M (max (max (max p1 p2) p3) p4)⟩

/-- Model of `sketch.imulPt`. -/
@[expose] noncomputable def imulPt (y : ℝ) (b : Ival) : Ival :=
  let p1 := M.rn (y * b.lo)
  let p2 := M.rn (y * b.hi)
  ⟨down M (min p1 p2), up M (max p1 p2)⟩

/-- Model of `sketch.isqr`. -/
@[expose] noncomputable def isqr (a : Ival) : Ival :=
  let lo := M.rn (a.lo * a.lo)
  let hi := M.rn (a.hi * a.hi)
  if 0 ≤ a.lo then ⟨down M lo, up M hi⟩
  else if a.hi ≤ 0 then ⟨down M hi, up M lo⟩
  else ⟨0, up M (max lo hi)⟩

/-- Model of `sketch.isqrtNonNeg`. `math.Sqrt` is `M.rn` of the exact square root. -/
@[expose] noncomputable def isqrtNonNeg (a : Ival) : Ival :=
  let lo := max a.lo 0
  ⟨max (down M (M.rn (Real.sqrt lo))) 0, up M (M.rn (Real.sqrt a.hi))⟩

/-- Model of `sketch.ihull`. -/
@[expose] noncomputable def ihull (a b : Ival) : Ival := ⟨min a.lo b.lo, max a.hi b.hi⟩

/-- Model of `sketch.Interval.strictlyInside`: `i` lies in the interior of `o`. -/
@[expose] def strictlyInside (i o : Ival) : Prop := o.lo < i.lo ∧ i.hi < o.hi

/-! ## Rounding steps -/

/-- Rounding to nearest then stepping down with `sketch.down` never lands above the exact value.
Covers `sketch.down`. Assumptions A1, A2, A3. -/
theorem down_sound (x : ℝ) : down M (M.rn x) ≤ x := M.down_le x

/-- Rounding to nearest then stepping up with `sketch.up` never lands below the exact value.
Covers `sketch.up`. Assumptions A1, A2, A3. -/
theorem up_sound (x : ℝ) : x ≤ up M (M.rn x) := M.up_ge x

/-- A float at most the rounding of `c` steps down to at most `c`. -/
theorem down_le_of_le_rn {f c : ℝ} (hf : f ∈ M.F) (h : f ≤ M.rn c) : down M f ≤ c :=
  (M.nd_mono hf (M.rn_mem c) h).trans (M.down_le c)

/-- A float at least the rounding of `c` steps up to at least `c`. -/
theorem le_up_of_rn_le {f c : ℝ} (hf : f ∈ M.F) (h : M.rn c ≤ f) : c ≤ up M f :=
  (M.up_ge c).trans (M.nu_mono (M.rn_mem c) hf h)

theorem min_mem {f g : ℝ} (hf : f ∈ M.F) (hg : g ∈ M.F) : min f g ∈ M.F := by
  rcases min_choice f g with h | h <;> rw [h] <;> assumption

theorem max_mem {f g : ℝ} (hf : f ∈ M.F) (hg : g ∈ M.F) : max f g ∈ M.F := by
  rcases max_choice f g with h | h <;> rw [h] <;> assumption

/-! ## Products of interval points -/

theorem min_mul_le {a b x : ℝ} (y : ℝ) (hx : a ≤ x ∧ x ≤ b) : min (a * y) (b * y) ≤ x * y := by
  rcases le_total 0 y with hy | hy
  · exact (min_le_left _ _).trans (mul_le_mul_of_nonneg_right hx.1 hy)
  · exact (min_le_right _ _).trans (mul_le_mul_of_nonpos_right hx.2 hy)

theorem le_max_mul {a b x : ℝ} (y : ℝ) (hx : a ≤ x ∧ x ≤ b) : x * y ≤ max (a * y) (b * y) := by
  rcases le_total 0 y with hy | hy
  · exact (mul_le_mul_of_nonneg_right hx.2 hy).trans (le_max_right _ _)
  · exact (mul_le_mul_of_nonpos_right hx.1 hy).trans (le_max_left _ _)

/-- A product of points of `[a, b]` and `[c, d]` is at least the least corner product. -/
theorem min_corner_le_mul {a b c d x y : ℝ} (hx : a ≤ x ∧ x ≤ b) (hy : c ≤ y ∧ y ≤ d) :
    min (min (min (a * c) (a * d)) (b * c)) (b * d) ≤ x * y := by
  have ha : min (a * c) (a * d) ≤ a * y := by
    have := min_mul_le a hy
    rwa [mul_comm c, mul_comm d, mul_comm y] at this
  have hb : min (b * c) (b * d) ≤ b * y := by
    have := min_mul_le b hy
    rwa [mul_comm c, mul_comm d, mul_comm y] at this
  have h1 : min (min (min (a * c) (a * d)) (b * c)) (b * d) ≤ a * y :=
    ((min_le_left _ _).trans (min_le_left _ _)).trans ha
  have h2 : min (min (min (a * c) (a * d)) (b * c)) (b * d) ≤ b * y :=
    (le_min ((min_le_left _ _).trans (min_le_right _ _)) (min_le_right _ _)).trans hb
  exact (le_min h1 h2).trans (min_mul_le y hx)

/-- A product of points of `[a, b]` and `[c, d]` is at most the greatest corner product. -/
theorem mul_le_max_corner {a b c d x y : ℝ} (hx : a ≤ x ∧ x ≤ b) (hy : c ≤ y ∧ y ≤ d) :
    x * y ≤ max (max (max (a * c) (a * d)) (b * c)) (b * d) := by
  have ha : a * y ≤ max (a * c) (a * d) := by
    have := le_max_mul a hy
    rwa [mul_comm c, mul_comm d, mul_comm y] at this
  have hb : b * y ≤ max (b * c) (b * d) := by
    have := le_max_mul b hy
    rwa [mul_comm c, mul_comm d, mul_comm y] at this
  have h1 : a * y ≤ max (max (max (a * c) (a * d)) (b * c)) (b * d) :=
    ha.trans ((le_max_left _ _).trans (le_max_left _ _))
  have h2 : b * y ≤ max (max (max (a * c) (a * d)) (b * c)) (b * d) :=
    hb.trans (max_le ((le_max_right _ _).trans (le_max_left _ _)) (le_max_right _ _))
  exact (le_max_mul y hx).trans (max_le h1 h2)

/-! ## Soundness of each helper -/

/-- `sketch.pt` holds exactly its value. Covers `sketch.pt`. No assumptions. -/
theorem pt_sound (v : ℝ) : (pt v).Mem v := ⟨le_rfl, le_rfl⟩

/-- The outward-rounded sum contains `x + y` for every `x ∈ a` and `y ∈ b`.
Covers `sketch.iadd`, `sketch.down`, `sketch.up`. Assumptions A1, A2, A3. -/
theorem iadd_sound {a b : Ival} {x y : ℝ} (hx : a.Mem x) (hy : b.Mem y) :
    (iadd M a b).Mem (x + y) :=
  ⟨(down_sound M _).trans (by linarith [hx.1, hy.1]),
   (by linarith [hx.2, hy.2] : x + y ≤ a.hi + b.hi).trans (up_sound M _)⟩

/-- The outward-rounded difference contains `x - y` for every `x ∈ a` and `y ∈ b`.
Covers `sketch.isub`, `sketch.down`, `sketch.up`. Assumptions A1, A2, A3. -/
theorem isub_sound {a b : Ival} {x y : ℝ} (hx : a.Mem x) (hy : b.Mem y) :
    (isub M a b).Mem (x - y) :=
  ⟨(down_sound M _).trans (by linarith [hx.1, hy.2]),
   (by linarith [hx.2, hy.1] : x - y ≤ a.hi - b.lo).trans (up_sound M _)⟩

/-- The negated interval contains `-x` for every `x ∈ a`. Covers `sketch.ineg`. Assumption A6. -/
theorem ineg_sound {a : Ival} {x : ℝ} (hx : a.Mem x) : (ineg a).Mem (-x) :=
  ⟨neg_le_neg hx.2, neg_le_neg hx.1⟩

/-- The outward-rounded product contains `x * y` for every `x ∈ a` and `y ∈ b`.
Covers `sketch.imul`, `sketch.down`, `sketch.up`. Assumptions A1, A2, A3, A4, A6. -/
theorem imul_sound {a b : Ival} {x y : ℝ} (hx : a.Mem x) (hy : b.Mem y) :
    (imul M a b).Mem (x * y) := by
  have m1 := M.rn_mem (a.lo * b.lo)
  have m2 := M.rn_mem (a.lo * b.hi)
  have m3 := M.rn_mem (a.hi * b.lo)
  have m4 := M.rn_mem (a.hi * b.hi)
  have hmin := min_mem M (min_mem M (min_mem M m1 m2) m3) m4
  have hmax := max_mem M (max_mem M (max_mem M m1 m2) m3) m4
  refine ⟨?_, ?_⟩
  · refine le_trans ?_ (min_corner_le_mul hx hy)
    refine le_min (le_min (le_min ?_ ?_) ?_) ?_ <;> refine down_le_of_le_rn M hmin ?_
    · exact ((min_le_left _ _).trans (min_le_left _ _)).trans (min_le_left _ _)
    · exact ((min_le_left _ _).trans (min_le_left _ _)).trans (min_le_right _ _)
    · exact (min_le_left _ _).trans (min_le_right _ _)
    · exact min_le_right _ _
  · refine le_trans (mul_le_max_corner hx hy) ?_
    refine max_le (max_le (max_le ?_ ?_) ?_) ?_ <;> refine le_up_of_rn_le M hmax ?_
    · exact (le_max_left _ _).trans ((le_max_left _ _).trans (le_max_left _ _))
    · exact (le_max_right _ _).trans ((le_max_left _ _).trans (le_max_left _ _))
    · exact (le_max_right _ _).trans (le_max_left _ _)
    · exact le_max_right _ _

/-- `sketch.imulPt y b` equals `sketch.imul (pt y) b`: the two dropped products repeat the kept ones.
Covers `sketch.imulPt`, `sketch.imul`, `sketch.pt`. No assumptions beyond the model. -/
theorem imulPt_eq_imul_pt (y : ℝ) (b : Ival) : imulPt M y b = imul M (pt y) b := by
  simp only [imulPt, imul, pt]
  rw [min_eq_left (min_le_left _ _), min_eq_left (min_le_right _ _),
      max_eq_left (le_max_left _ _), max_eq_left (le_max_right _ _)]

/-- The outward-rounded scaled interval contains `y * z` for every `z ∈ b`.
Covers `sketch.imulPt`, `sketch.down`, `sketch.up`. Assumptions A1, A2, A3, A4, A6. -/
theorem imulPt_sound (y : ℝ) {b : Ival} {z : ℝ} (hz : b.Mem z) : (imulPt M y b).Mem (y * z) := by
  rw [imulPt_eq_imul_pt]
  exact imul_sound M (pt_sound y) hz

/-- The outward-rounded square contains `x * x` for every `x ∈ a`, including a lower endpoint of 0 when `a`
straddles zero. Covers `sketch.isqr`, `sketch.down`, `sketch.up`. Assumptions A1, A2, A3, A4, A6. -/
theorem isqr_sound {a : Ival} {x : ℝ} (hx : a.Mem x) : (isqr M a).Mem (x * x) := by
  obtain ⟨h1, h2⟩ := hx
  have mlo := M.rn_mem (a.lo * a.lo)
  have mhi := M.rn_mem (a.hi * a.hi)
  unfold isqr
  split_ifs with hlo hhi
  · refine ⟨(down_sound M _).trans (mul_self_le_mul_self hlo h1), ?_⟩
    exact (mul_self_le_mul_self (hlo.trans h1) h2).trans (up_sound M _)
  · have hx0 : x ≤ 0 := h2.trans hhi
    refine ⟨(down_sound M _).trans ?_, ?_⟩
    · have := mul_self_le_mul_self (neg_nonneg.2 hhi) (neg_le_neg h2)
      rwa [neg_mul_neg, neg_mul_neg] at this
    · refine le_trans ?_ (up_sound M _)
      have := mul_self_le_mul_self (neg_nonneg.2 hx0) (neg_le_neg h1)
      rwa [neg_mul_neg, neg_mul_neg] at this
  · refine ⟨mul_self_nonneg x, ?_⟩
    have hm := max_mem M mlo mhi
    rcases le_total 0 x with h0 | h0
    · exact (mul_self_le_mul_self h0 h2).trans (le_up_of_rn_le M hm (le_max_right _ _))
    · refine le_trans ?_ (le_up_of_rn_le M hm (le_max_left _ _))
      have := mul_self_le_mul_self (neg_nonneg.2 h0) (neg_le_neg h1)
      rwa [neg_mul_neg, neg_mul_neg] at this

/-- For an interval whose exact quantity `x` is non-negative, the outward-rounded square root contains `√x`.
Covers `sketch.isqrtNonNeg`, `sketch.down`, `sketch.up`. Assumptions A2, A3, A5, A6. -/
theorem isqrtNonNeg_sound {a : Ival} {x : ℝ} (hx : a.Mem x) (h0 : 0 ≤ x) :
    (isqrtNonNeg M a).Mem (Real.sqrt x) := by
  obtain ⟨h1, h2⟩ := hx
  refine ⟨max_le ((down_sound M _).trans (Real.sqrt_le_sqrt (max_le h1 h0))) (Real.sqrt_nonneg x), ?_⟩
  exact (Real.sqrt_le_sqrt h2).trans (up_sound M _)

/-- The hull contains every point of its first operand. Covers `sketch.ihull`. Assumption A6. -/
theorem ihull_sound_left {a : Ival} (b : Ival) {x : ℝ} (hx : a.Mem x) : (ihull a b).Mem x :=
  ⟨(min_le_left _ _).trans hx.1, hx.2.trans (le_max_left _ _)⟩

/-- The hull contains every point of its second operand. Covers `sketch.ihull`. Assumption A6. -/
theorem ihull_sound_right (a : Ival) {b : Ival} {x : ℝ} (hx : b.Mem x) : (ihull a b).Mem x :=
  ⟨(min_le_right _ _).trans hx.1, hx.2.trans (le_max_right _ _)⟩

/-- When `i` is strictly inside `o`, every point of `i` lies in the open interval `(o.lo, o.hi)`.
Covers `sketch.Interval.strictlyInside`. Assumption A6. -/
theorem strictlyInside_sound {i o : Ival} (h : strictlyInside i o) {x : ℝ} (hx : i.Mem x) :
    o.lo < x ∧ x < o.hi :=
  ⟨h.1.trans_le hx.1, hx.2.trans_lt h.2⟩

end Sketch.Interval

end
