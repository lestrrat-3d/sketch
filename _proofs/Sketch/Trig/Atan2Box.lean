module

public import Sketch.Trig.Atan2

/-!
# Direction angles over a rectangle: `atan2Box` and `shiftNear`

Models of `sketch.atan2Box` and `sketch.shiftNear`. `atan2Box` encloses the direction of `(d, c)` over every
point of the rectangle `c × d` that misses the origin. Such a rectangle lies in an open half-plane
`d > 0`, `d < 0`, `c > 0` or `c < 0`, and on that half-plane an arctangent formula `ψ` is a direction angle of
every point (`Branch.rep`), takes values in an open interval of width `π` (`Branch.near`), and is extremal at
the corners the Go code reads (`Branch.lower`, `Branch.upper`): the ratio inside the arctangent is monotone in
each coordinate separately.

The Go code brackets each corner with `sketch.atan2Point`, shifts the bracket by at most one whole turn toward
the float direction `ref` of the centre, and refuses a bracket more than 3 rad from `ref`. Those float decisions
are inputs here. Two facts about them are hypotheses carried by assumption A72: `ref` lies within `1/10` rad of
a direction angle of some point of the rectangle, and a bracket that passes the 3 rad test lies within
`3 + 2^-20` of `ref`. With them every corner's bracket holds the direction angle on one branch `ψ + 2πn`, so
their hull holds `ψ + 2πn` at every point of the rectangle (`atan2Box_sound`).
-/

public section

namespace Sketch.Trig

open Real Sketch.Float
open Sketch.Interval (iadd isub ihull iadd_sound isub_sound ihull_sound_left ihull_sound_right)

/-! ### Corners -/

/-- The corners `sketch.atan2Box` reads, as `(c, d)` pairs: all four when a coordinate endpoint is zero (or no
strict half-plane test holds), otherwise the two extreme corners its half-plane case selects. -/
@[expose] noncomputable def boxCorners (c d : Sketch.Interval.Ival) : List (ℝ × ℝ) :=
  let four := [(c.lo, d.lo), (c.lo, d.hi), (c.hi, d.lo), (c.hi, d.hi)]
  if c.lo = 0 ∨ c.hi = 0 ∨ d.lo = 0 ∨ d.hi = 0 then four
  else if 0 < d.lo ∨ d.hi < 0 then
    let minC := if d.hi < 0 then c.hi else c.lo
    let maxC := if d.hi < 0 then c.lo else c.hi
    [(minC, if 0 < minC then d.hi else d.lo), (maxC, if 0 < maxC then d.lo else d.hi)]
  else if 0 < c.lo ∨ c.hi < 0 then
    let minD := if c.hi < 0 then d.lo else d.hi
    let maxD := if c.hi < 0 then d.hi else d.lo
    [(if 0 < minD then c.lo else c.hi, minD), (if 0 < maxD then c.hi else c.lo, maxD)]
  else four

theorem boxCorners_ne_nil (c d : Sketch.Interval.Ival) : boxCorners c d ≠ [] := by
  unfold boxCorners
  split_ifs <;> simp

/-- Every corner read lies in the rectangle. -/
theorem boxCorners_mem {c d : Sketch.Interval.Ival} (hc : c.lo ≤ c.hi) (hd : d.lo ≤ d.hi) :
    ∀ u ∈ boxCorners c d, c.Mem u.1 ∧ d.Mem u.2 := by
  have cl : c.Mem c.lo := ⟨le_rfl, hc⟩
  have ch : c.Mem c.hi := ⟨hc, le_rfl⟩
  have dl : d.Mem d.lo := ⟨le_rfl, hd⟩
  have dh : d.Mem d.hi := ⟨hd, le_rfl⟩
  intro u hu
  unfold boxCorners at hu
  split_ifs at hu <;> simp only [List.mem_cons, List.not_mem_nil, or_false] at hu <;>
    rcases hu with rfl | rfl | rfl | rfl <;> refine ⟨?_, ?_⟩ <;> dsimp only <;> (try split_ifs) <;> assumption

/-! ### Direction angles on a half-plane -/

/-- On `x > 0`, `arctan (y / x)` is a direction angle of `(x, y)`. -/
theorem rep_arctan {x y : ℝ} (hx : 0 < x) : Rep (arctan (y / x)) x y := by
  have hs : 0 < √(1 + (y / x) ^ 2) := Real.sqrt_pos.mpr (by positivity)
  refine ⟨x * √(1 + (y / x) ^ 2), by positivity, ?_, ?_⟩
  · rw [cos_arctan]
    field_simp
  · rw [sin_arctan]
    field_simp

/-- Negating a vector adds a half-turn to its direction. -/
theorem rep_of_neg {θ x y : ℝ} (h : Rep θ (-x) (-y)) : Rep (θ + π) x y := by
  obtain ⟨r, hr, hx, hy⟩ := h
  exact ⟨r, hr, by rw [cos_add_pi]; linarith, by rw [sin_add_pi]; linarith⟩

/-- Rotating `(x, y)` a quarter-turn clockwise to `(y, -x)` subtracts a quarter-turn from its direction. -/
theorem rep_of_rot {θ x y : ℝ} (h : Rep θ y (-x)) : Rep (θ + π / 2) x y := by
  obtain ⟨r, hr, hx, hy⟩ := h
  exact ⟨r, hr, by rw [cos_add_pi_div_two]; linarith, by rw [sin_add_pi_div_two]; linarith⟩

/-- Rotating `(x, y)` a quarter-turn counter-clockwise to `(-y, x)` adds a quarter-turn to its direction. -/
theorem rep_of_rot' {θ x y : ℝ} (h : Rep θ (-y) x) : Rep (θ - π / 2) x y := by
  obtain ⟨r, hr, hx, hy⟩ := h
  exact ⟨r, hr, by rw [cos_sub_pi_div_two]; linarith, by rw [sin_sub_pi_div_two]; linarith⟩

/-! ### Extremes of a ratio over a box -/

section Ratio

variable {n1 n2 m1 m2 n m : ℝ} (hm1 : 0 < m1) (hm : m1 ≤ m ∧ m ≤ m2) (hn : n1 ≤ n ∧ n ≤ n2)
include hm1 hm hn

theorem ratio_lo_of_nonneg (h : 0 ≤ n1) : n1 / m2 ≤ n / m := by
  rw [div_le_div_iff₀ (by linarith) (by linarith)]
  nlinarith

theorem ratio_lo_of_nonpos (h : n1 ≤ 0) : n1 / m1 ≤ n / m := by
  rw [div_le_div_iff₀ hm1 (by linarith)]
  nlinarith

theorem ratio_hi_of_nonneg (h : 0 ≤ n2) : n / m ≤ n2 / m1 := by
  rw [div_le_div_iff₀ (by linarith) hm1]
  nlinarith

theorem ratio_hi_of_nonpos (h : n2 ≤ 0) : n / m ≤ n2 / m2 := by
  rw [div_le_div_iff₀ (by linarith) (by linarith)]
  nlinarith

end Ratio

/-! ### Branches -/

/-- A continuous direction-angle branch on the rectangle `c × d`, extremal at the corners `boxCorners` reads.
`ψ cv dv` is a direction angle of `(dv, cv)`. -/
structure Branch (c d : Sketch.Interval.Ival) where
  ψ : ℝ → ℝ → ℝ
  ω : ℝ
  rep : ∀ cv dv, c.Mem cv → d.Mem dv → Rep (ψ cv dv) dv cv
  near : ∀ cv dv, c.Mem cv → d.Mem dv → |ψ cv dv - ω| < π / 2
  lower : ∀ cv dv, c.Mem cv → d.Mem dv → ∃ u ∈ boxCorners c d, ψ u.1 u.2 ≤ ψ cv dv
  upper : ∀ cv dv, c.Mem cv → d.Mem dv → ∃ u ∈ boxCorners c d, ψ cv dv ≤ ψ u.1 u.2

theorem abs_arctan_add_sub_lt (t a : ℝ) : |arctan t + a - a| < π / 2 := by
  rw [add_sub_cancel_right, abs_lt]
  exact ⟨neg_pi_div_two_lt_arctan t, arctan_lt_pi_div_two t⟩

/-- The branch on `d > 0`: `ψ = arctan (c / d)`. -/
noncomputable def branchDPos (c d : Sketch.Interval.Ival) (hd : 0 < d.lo)
    (hlo : (c.lo, if 0 < c.lo then d.hi else d.lo) ∈ boxCorners c d)
    (hhi : (c.hi, if 0 < c.hi then d.lo else d.hi) ∈ boxCorners c d) : Branch c d where
  ψ cv dv := arctan (cv / dv) + 0
  ω := 0
  rep cv dv _ hdv := by rw [add_zero]; exact rep_arctan (hd.trans_le hdv.1)
  near cv dv _ _ := abs_arctan_add_sub_lt _ _
  lower cv dv hcv hdv := by
    refine ⟨_, hlo, add_le_add_left (arctan_strictMono.monotone ?_) _⟩
    dsimp only
    split_ifs with h
    · exact ratio_lo_of_nonneg hd ⟨hdv.1, hdv.2⟩ ⟨hcv.1, hcv.2⟩ h.le
    · exact ratio_lo_of_nonpos hd ⟨hdv.1, hdv.2⟩ ⟨hcv.1, hcv.2⟩ (not_lt.mp h)
  upper cv dv hcv hdv := by
    refine ⟨_, hhi, add_le_add_left (arctan_strictMono.monotone ?_) _⟩
    dsimp only
    split_ifs with h
    · exact ratio_hi_of_nonneg hd ⟨hdv.1, hdv.2⟩ ⟨hcv.1, hcv.2⟩ h.le
    · exact ratio_hi_of_nonpos hd ⟨hdv.1, hdv.2⟩ ⟨hcv.1, hcv.2⟩ (not_lt.mp h)

/-- The branch on `d < 0`: `ψ = arctan ((-c) / (-d)) + π`. -/
noncomputable def branchDNeg (c d : Sketch.Interval.Ival) (hd : d.hi < 0)
    (hlo : (c.hi, if 0 < c.hi then d.hi else d.lo) ∈ boxCorners c d)
    (hhi : (c.lo, if 0 < c.lo then d.lo else d.hi) ∈ boxCorners c d) : Branch c d where
  ψ cv dv := arctan ((-cv) / (-dv)) + π
  ω := π
  rep cv dv _ hdv := rep_of_neg (rep_arctan (by linarith [hdv.2]))
  near cv dv _ _ := abs_arctan_add_sub_lt _ _
  lower cv dv hcv hdv := by
    have hm1 : 0 < -d.hi := by linarith
    have hm : -d.hi ≤ -dv ∧ -dv ≤ -d.lo := ⟨by linarith [hdv.2], by linarith [hdv.1]⟩
    have hn : -c.hi ≤ -cv ∧ -cv ≤ -c.lo := ⟨by linarith [hcv.2], by linarith [hcv.1]⟩
    refine ⟨_, hlo, add_le_add_left (arctan_strictMono.monotone ?_) _⟩
    dsimp only
    split_ifs with h
    · exact ratio_lo_of_nonpos hm1 hm hn (by linarith)
    · exact ratio_lo_of_nonneg hm1 hm hn (by linarith [not_lt.mp h])
  upper cv dv hcv hdv := by
    have hm1 : 0 < -d.hi := by linarith
    have hm : -d.hi ≤ -dv ∧ -dv ≤ -d.lo := ⟨by linarith [hdv.2], by linarith [hdv.1]⟩
    have hn : -c.hi ≤ -cv ∧ -cv ≤ -c.lo := ⟨by linarith [hcv.2], by linarith [hcv.1]⟩
    refine ⟨_, hhi, add_le_add_left (arctan_strictMono.monotone ?_) _⟩
    dsimp only
    split_ifs with h
    · exact ratio_hi_of_nonpos hm1 hm hn (by linarith)
    · exact ratio_hi_of_nonneg hm1 hm hn (by linarith [not_lt.mp h])

/-- The branch on `c > 0`: `ψ = arctan ((-d) / c) + π/2`. -/
noncomputable def branchCPos (c d : Sketch.Interval.Ival) (hc : 0 < c.lo)
    (hlo : (if 0 < d.hi then c.lo else c.hi, d.hi) ∈ boxCorners c d)
    (hhi : (if 0 < d.lo then c.hi else c.lo, d.lo) ∈ boxCorners c d) : Branch c d where
  ψ cv dv := arctan ((-dv) / cv) + π / 2
  ω := π / 2
  rep cv dv hcv _ := rep_of_rot (rep_arctan (hc.trans_le hcv.1))
  near cv dv _ _ := abs_arctan_add_sub_lt _ _
  lower cv dv hcv hdv := by
    have hn : -d.hi ≤ -dv ∧ -dv ≤ -d.lo := ⟨by linarith [hdv.2], by linarith [hdv.1]⟩
    refine ⟨_, hlo, add_le_add_left (arctan_strictMono.monotone ?_) _⟩
    dsimp only
    split_ifs with h
    · exact ratio_lo_of_nonpos hc ⟨hcv.1, hcv.2⟩ hn (by linarith)
    · exact ratio_lo_of_nonneg hc ⟨hcv.1, hcv.2⟩ hn (by linarith [not_lt.mp h])
  upper cv dv hcv hdv := by
    have hn : -d.hi ≤ -dv ∧ -dv ≤ -d.lo := ⟨by linarith [hdv.2], by linarith [hdv.1]⟩
    refine ⟨_, hhi, add_le_add_left (arctan_strictMono.monotone ?_) _⟩
    dsimp only
    split_ifs with h
    · exact ratio_hi_of_nonpos hc ⟨hcv.1, hcv.2⟩ hn (by linarith)
    · exact ratio_hi_of_nonneg hc ⟨hcv.1, hcv.2⟩ hn (by linarith [not_lt.mp h])

/-- The branch on `c < 0`: `ψ = arctan (d / (-c)) - π/2`. -/
noncomputable def branchCNeg (c d : Sketch.Interval.Ival) (hc : c.hi < 0)
    (hlo : (if 0 < d.lo then c.lo else c.hi, d.lo) ∈ boxCorners c d)
    (hhi : (if 0 < d.hi then c.hi else c.lo, d.hi) ∈ boxCorners c d) : Branch c d where
  ψ cv dv := arctan (dv / (-cv)) + -(π / 2)
  ω := -(π / 2)
  rep cv dv hcv _ := by
    rw [← sub_eq_add_neg]
    exact rep_of_rot' (rep_arctan (by linarith [hcv.2]))
  near cv dv _ _ := abs_arctan_add_sub_lt _ _
  lower cv dv hcv hdv := by
    have hm1 : 0 < -c.hi := by linarith
    have hm : -c.hi ≤ -cv ∧ -cv ≤ -c.lo := ⟨by linarith [hcv.2], by linarith [hcv.1]⟩
    refine ⟨_, hlo, add_le_add_left (arctan_strictMono.monotone ?_) _⟩
    dsimp only
    split_ifs with h
    · exact ratio_lo_of_nonneg hm1 hm ⟨hdv.1, hdv.2⟩ h.le
    · exact ratio_lo_of_nonpos hm1 hm ⟨hdv.1, hdv.2⟩ (not_lt.mp h)
  upper cv dv hcv hdv := by
    have hm1 : 0 < -c.hi := by linarith
    have hm : -c.hi ≤ -cv ∧ -cv ≤ -c.lo := ⟨by linarith [hcv.2], by linarith [hcv.1]⟩
    refine ⟨_, hhi, add_le_add_left (arctan_strictMono.monotone ?_) _⟩
    dsimp only
    split_ifs with h
    · exact ratio_hi_of_nonneg hm1 hm ⟨hdv.1, hdv.2⟩ h.le
    · exact ratio_hi_of_nonpos hm1 hm ⟨hdv.1, hdv.2⟩ (not_lt.mp h)

/-- Every nonempty rectangle that misses the origin has a branch extremal at the corners `boxCorners`
reads. -/
theorem exists_branch {c d : Sketch.Interval.Ival} (hc : c.lo ≤ c.hi) (hd : d.lo ≤ d.hi)
    (horig : ¬(c.lo ≤ 0 ∧ 0 ≤ c.hi ∧ d.lo ≤ 0 ∧ 0 ≤ d.hi)) : Nonempty (Branch c d) := by
  have four : ∀ x y, (x = c.lo ∨ x = c.hi) → (y = d.lo ∨ y = d.hi) →
      (c.lo = 0 ∨ c.hi = 0 ∨ d.lo = 0 ∨ d.hi = 0) → (x, y) ∈ boxCorners c d := by
    intro x y hx hy hz
    unfold boxCorners
    rw [ite_eq_left hz]
    rcases hx with rfl | rfl <;> rcases hy with rfl | rfl <;> simp
  have side : 0 < d.lo ∨ d.hi < 0 ∨ 0 < c.lo ∨ c.hi < 0 := by
    by_contra h
    push Not at h
    exact horig ⟨h.2.2.1, h.2.2.2, h.1, h.2.1⟩
  by_cases hz : c.lo = 0 ∨ c.hi = 0 ∨ d.lo = 0 ∨ d.hi = 0
  · rcases side with h | h | h | h
    · exact ⟨branchDPos c d h (four _ _ (Or.inl rfl) (by split_ifs <;> simp) hz)
        (four _ _ (Or.inr rfl) (by split_ifs <;> simp) hz)⟩
    · exact ⟨branchDNeg c d h (four _ _ (Or.inr rfl) (by split_ifs <;> simp) hz)
        (four _ _ (Or.inl rfl) (by split_ifs <;> simp) hz)⟩
    · exact ⟨branchCPos c d h (four _ _ (by split_ifs <;> simp) (Or.inr rfl) hz)
        (four _ _ (by split_ifs <;> simp) (Or.inl rfl) hz)⟩
    · exact ⟨branchCNeg c d h (four _ _ (by split_ifs <;> simp) (Or.inl rfl) hz)
        (four _ _ (by split_ifs <;> simp) (Or.inr rfl) hz)⟩
  by_cases hD : 0 < d.lo ∨ d.hi < 0
  · have e : boxCorners c d = [((if d.hi < 0 then c.hi else c.lo),
        if 0 < (if d.hi < 0 then c.hi else c.lo) then d.hi else d.lo),
        ((if d.hi < 0 then c.lo else c.hi), if 0 < (if d.hi < 0 then c.lo else c.hi) then d.lo else d.hi)] := by
      unfold boxCorners
      rw [ite_eq_right hz, ite_eq_left hD]
    by_cases hpos : 0 < d.lo
    · have hn : ¬d.hi < 0 := by linarith
      simp only [hn, ↓reduceIte] at e
      exact ⟨branchDPos c d hpos (by rw [e]; simp) (by rw [e]; simp)⟩
    · have hneg : d.hi < 0 := hD.resolve_left hpos
      simp only [hneg, ↓reduceIte] at e
      exact ⟨branchDNeg c d hneg (by rw [e]; simp) (by rw [e]; simp)⟩
  · have hC : 0 < c.lo ∨ c.hi < 0 := by
      rcases side with h | h | h | h
      · exact absurd (Or.inl h) hD
      · exact absurd (Or.inr h) hD
      · exact Or.inl h
      · exact Or.inr h
    have e : boxCorners c d = [(if 0 < (if c.hi < 0 then d.lo else d.hi) then c.lo else c.hi,
        if c.hi < 0 then d.lo else d.hi),
        (if 0 < (if c.hi < 0 then d.hi else d.lo) then c.hi else c.lo, if c.hi < 0 then d.hi else d.lo)] := by
      unfold boxCorners
      rw [ite_eq_right hz, ite_eq_right hD, ite_eq_left hC]
    by_cases hpos : 0 < c.lo
    · have hn : ¬c.hi < 0 := by linarith
      simp only [hn, ↓reduceIte] at e
      exact ⟨branchCPos c d hpos (by rw [e]; simp) (by rw [e]; simp)⟩
    · have hneg : c.hi < 0 := hC.resolve_left hpos
      simp only [hneg, ↓reduceIte] at e
      exact ⟨branchCNeg c d hneg (by rw [e]; simp) (by rw [e]; simp)⟩

/-! ### `atan2Box` -/

variable (M : FloatModel) (R : Rounding)

/-- The float64 decisions of `sketch.atan2Box`, taken as inputs: `ref` is `math.Atan2(c.mid(), d.mid())`;
for a corner bracket `phi`, `above phi` is `phi.mid()-ref > math.Pi`, `below phi` is
`phi.mid()-ref < -math.Pi`, and `far phi` is `math.Abs(phi.mid()-ref) > 3` on the shifted bracket. -/
structure BoxEnv where
  ref : ℝ
  above : Sketch.Interval.Ival → Bool
  below : Sketch.Interval.Ival → Bool
  far : Sketch.Interval.Ival → Bool

/-- The `switch` of `sketch.atan2Box` that moves a corner bracket by at most one whole turn toward `ref`. -/
@[expose] noncomputable def turnShift (env : BoxEnv) (phi : Sketch.Interval.Ival) : Sketch.Interval.Ival :=
  if env.above phi then isub M phi (toIv (twoPiTimes R 1))
  else if env.below phi then iadd M phi (toIv (twoPiTimes R 1))
  else phi

/-- One pass of the corner loop of `sketch.atan2Box`: bracket the corner `u = (c, d)` with `A`
(`sketch.atan2Point`), shift it, and refuse it when it lies more than 3 rad from `ref`. -/
@[expose] noncomputable def cornerAngle (A : ℝ → ℝ → Option Sketch.Interval.Ival) (env : BoxEnv)
    (u : ℝ × ℝ) : Option Sketch.Interval.Ival :=
  match A u.1 u.2 with
  | none => none
  | some phi =>
    let p := turnShift M R env phi
    if env.far p then none else some p

/-- The hull the corner loop accumulates over the remaining corners. -/
@[expose] noncomputable def hullRest (f : ℝ × ℝ → Option Sketch.Interval.Ival) :
    Sketch.Interval.Ival → List (ℝ × ℝ) → Option Sketch.Interval.Ival
  | acc, [] => some acc
  | acc, u :: us =>
    match f u with
    | none => none
    | some p => hullRest f (ihull acc p) us

/-- Model of `sketch.atan2Box`. `A` is the corner bracket (`sketch.atan2Point`); the float decisions are the
inputs of `BoxEnv`. -/
@[expose] noncomputable def atan2Box (A : ℝ → ℝ → Option Sketch.Interval.Ival) (env : BoxEnv)
    (c d : Sketch.Interval.Ival) : Option Sketch.Interval.Ival :=
  if c.lo ≤ 0 ∧ 0 ≤ c.hi ∧ d.lo ≤ 0 ∧ 0 ≤ d.hi then none
  else
    match boxCorners c d with
    | [] => some ⟨0, 0⟩
    | u :: us =>
      match cornerAngle M R A env u with
      | none => none
      | some p => hullRest (cornerAngle M R A env) p us

theorem hullRest_sound (f : ℝ × ℝ → Option Sketch.Interval.Ival) :
    ∀ (us : List (ℝ × ℝ)) (acc out : Sketch.Interval.Ival), hullRest f acc us = some out →
      (∀ x, acc.Mem x → out.Mem x) ∧ ∀ u ∈ us, ∃ p, f u = some p ∧ ∀ x, p.Mem x → out.Mem x := by
  intro us
  induction us with
  | nil =>
    intro acc out h
    simp only [hullRest, Option.some.injEq] at h
    subst h
    exact ⟨fun _ hx => hx, by simp⟩
  | cons u us ih =>
    intro acc out h
    unfold hullRest at h
    split at h
    · exact absurd h (by simp)
    rename_i p hp
    obtain ⟨hacc, hrest⟩ := ih _ _ h
    refine ⟨fun x hx => hacc x (ihull_sound_left p hx), ?_⟩
    intro w hw
    rcases List.mem_cons.mp hw with rfl | hw
    · exact ⟨p, hp, fun x hx => hacc x (ihull_sound_right acc hx)⟩
    · exact hrest w hw

/-- A shifted bracket still holds a direction angle. -/
theorem turnShift_rep (env : BoxEnv) {phi : Sketch.Interval.Ival} {a x y : ℝ} (ha : phi.Mem a)
    (hr : Rep a x y) : ∃ b, (turnShift M R env phi).Mem b ∧ Rep b x y := by
  have h2π : (toIv (twoPiTimes R 1)).Mem ((1 : ℤ) * (2 * π)) := twoPiTimes_sound R 1
  unfold turnShift
  split_ifs
  · refine ⟨a - (1 : ℤ) * (2 * π), isub_sound M ha h2π, ?_⟩
    have := rep_add_int_mul hr (-1)
    push_cast at this ⊢
    convert this using 1
    ring
  · exact ⟨a + (1 : ℤ) * (2 * π), iadd_sound M ha h2π, rep_add_int_mul hr 1⟩
  · exact ⟨a, ha, hr⟩

/-- **Soundness of the rectangle enclosure.** Whenever the model of `sketch.atan2Box` returns an interval, it
contains a direction angle of `(dv, cv)` for every `cv ∈ c` and `dv ∈ d`, so it contains
`Complex.arg (dv + cv·i)` modulo `2π` at every point of the rectangle. `hA` is the soundness of the corner
bracket (`atan2Point_sound`). `href` and `hfar` are the two facts about the float decisions that assumption A72
states: `ref` lies within `1/10` rad of a direction angle of some point of the rectangle, and a shifted corner
bracket that passes the 3 rad test lies within `3 + 2^-20` of `ref`. Covers `sketch.atan2Box`,
`sketch.twoPiTimes`, `sketch.isub`, `sketch.iadd` and `sketch.ihull`. Assumptions: A1 to A3, A6, A20, A22,
A24, A28, A70, A72. -/
theorem atan2Box_sound (A : ℝ → ℝ → Option Sketch.Interval.Ival)
    (hA : ∀ c0 d0 I, A c0 d0 = some I → ∃ θ, I.Mem θ ∧ Rep θ d0 c0) (env : BoxEnv)
    (c d : Sketch.Interval.Ival)
    (href : ∃ mc md θ, c.Mem mc ∧ d.Mem md ∧ Rep θ md mc ∧ |env.ref - θ| ≤ 1 / 10)
    (hfar : ∀ c0 d0 I a, A c0 d0 = some I → env.far (turnShift M R env I) = false →
      (turnShift M R env I).Mem a → |a - env.ref| ≤ 3 + 1 / 2 ^ 20)
    {out : Sketch.Interval.Ival} (h : atan2Box M R A env c d = some out) :
    ∀ cv dv, c.Mem cv → d.Mem dv → ∃ θ, out.Mem θ ∧ Rep θ dv cv := by
  obtain ⟨mc, md, θm, hmc, hmd, hθm, hrefm⟩ := href
  have hc : c.lo ≤ c.hi := hmc.1.trans hmc.2
  have hd : d.lo ≤ d.hi := hmd.1.trans hmd.2
  unfold atan2Box at h
  split_ifs at h with horig
  -- every corner's bracket is inside `out`
  have hcorner : ∀ u ∈ boxCorners c d, ∃ p, cornerAngle M R A env u = some p ∧ ∀ x, p.Mem x → out.Mem x := by
    split at h
    · rename_i hnil
      exact absurd hnil (boxCorners_ne_nil c d)
    rename_i u us hus
    split at h
    · exact absurd h (by simp)
    rename_i p hp
    obtain ⟨hacc, hrest⟩ := hullRest_sound _ us p out h
    intro w hw
    rw [hus] at hw
    rcases List.mem_cons.mp hw with rfl | hw
    · exact ⟨p, hp, hacc⟩
    · exact hrest w hw
  obtain ⟨B⟩ := exists_branch hc hd horig
  have hπ := pi_gt_d2
  -- `ref`'s branch
  obtain ⟨n, hn⟩ := rep_unique (B.rep mc md hmc hmd) hθm
  have hnear_m := abs_lt.mp (B.near mc md hmc hmd)
  have href' := abs_le.mp hrefm
  -- each corner's direction on that branch lies in `out`
  have hon : ∀ u ∈ boxCorners c d, out.Mem (B.ψ u.1 u.2 + n * (2 * π)) := by
    intro u hu
    obtain ⟨huc, hud⟩ := boxCorners_mem hc hd u hu
    obtain ⟨p, hp, hpout⟩ := hcorner u hu
    unfold cornerAngle at hp
    split at hp
    · exact absurd hp (by simp)
    rename_i phi hphi
    dsimp only at hp
    split_ifs at hp with hf
    simp only [Option.some.injEq] at hp
    subst hp
    obtain ⟨a0, ha0, hr0⟩ := hA _ _ _ hphi
    obtain ⟨a, ha, hra⟩ := turnShift_rep M R env ha0 hr0
    have hfar' := abs_le.mp (hfar _ _ _ a hphi (by simpa using hf) ha)
    obtain ⟨k, hk⟩ := rep_unique (B.rep u.1 u.2 huc hud) hra
    have hnear_u := abs_lt.mp (B.near u.1 u.2 huc hud)
    have hkn : k = n := by
      have hpow : (0 : ℝ) < 1 / 2 ^ 20 := by positivity
      have hpow' : (1 : ℝ) / 2 ^ 20 < 1 / 100 := by norm_num
      by_contra hne
      rcases lt_or_gt_of_ne hne with hlt | hgt
      · have h1 : (k : ℝ) ≤ n - 1 := by exact_mod_cast Int.le_sub_one_of_lt hlt
        have h2 : 2 * π ≤ 2 * π * ((n : ℝ) - k) := by nlinarith
        nlinarith
      · have h1 : (n : ℝ) ≤ k - 1 := by exact_mod_cast Int.le_sub_one_of_lt hgt
        have h2 : 2 * π ≤ 2 * π * ((k : ℝ) - n) := by nlinarith
        nlinarith
    rw [← hkn, ← hk]
    exact hpout a ha
  intro cv dv hcv hdv
  obtain ⟨u, hu, hlo⟩ := B.lower cv dv hcv hdv
  obtain ⟨u', hu', hhi⟩ := B.upper cv dv hcv hdv
  refine ⟨B.ψ cv dv + n * (2 * π), ⟨?_, ?_⟩, rep_add_int_mul (B.rep cv dv hcv hdv) n⟩
  · linarith [(hon u hu).1]
  · linarith [(hon u' hu').2]

/-- **Rectangle enclosure on binary64.** `sketch.atan2Box` with `sketch.atan2Point` and the certified
`sketch.sinCosPoint` of `interval_trig.go` as its corner bracket: a returned interval contains a direction angle
of every point of the rectangle. Covers `sketch.atan2Box`, `sketch.atan2Point`, `sketch.twoPiTimes`,
`sketch.sinCosPoint` and every function they reach. Assumptions: A1 to A6, A20 to A28, A70 to A72. -/
theorem atan2Box_sound_binary64 (E : TrigEnv) (fuel : ℕ) (aenv : Atan2Env) (env : BoxEnv)
    (c d : Sketch.Interval.Ival)
    (href : ∃ mc md θ, c.Mem mc ∧ d.Mem md ∧ Rep θ md mc ∧ |env.ref - θ| ≤ 1 / 10)
    (hfar : ∀ c0 d0 I a, atan2Point M (sinCosPoint (Rounding.ofFloat M) E fuel) aenv c0 d0 = some I →
      env.far (turnShift M (Rounding.ofFloat M) env I) = false →
      (turnShift M (Rounding.ofFloat M) env I).Mem a → |a - env.ref| ≤ 3 + 1 / 2 ^ 20)
    {out : Sketch.Interval.Ival}
    (h : atan2Box M (Rounding.ofFloat M) (atan2Point M (sinCosPoint (Rounding.ofFloat M) E fuel) aenv) env c d =
      some out) :
    ∀ cv dv, c.Mem cv → d.Mem dv → ∃ θ, out.Mem θ ∧ Rep θ dv cv :=
  atan2Box_sound M (Rounding.ofFloat M) _ (fun c0 d0 _ hI => atan2Point_sound_binary64 M E fuel aenv c0 d0 hI)
    env c d href hfar h

/-! ### `shiftNear` -/

/-- Model of `sketch.shiftNear`. `k` is the float64 decision `math.Round((iv.mid() - ref) / (2 * math.Pi))`
as an integer, `none` when it is NaN. -/
@[expose] noncomputable def shiftNear (iv : Sketch.Interval.Ival) (k : Option ℤ) : Sketch.Interval.Ival :=
  match k with
  | none => iv
  | some k => if k = 0 ∨ 2 ^ 20 < |k| then iv else isub M iv (toIv (twoPiTimes R k))

/-- **`shiftNear` moves by whole turns.** The model of `sketch.shiftNear` moves every member of its input by
the same whole number of turns, whatever the float decision `k`, so a direction angle in the input gives a
direction angle in the output. Covers `sketch.shiftNear`, `sketch.twoPiTimes` and `sketch.isub`. Assumptions:
A1 to A3, A20, A22, A24, A28, A70. -/
theorem shiftNear_sound (iv : Sketch.Interval.Ival) (k : Option ℤ) :
    ∃ j : ℤ, ∀ x, iv.Mem x → (shiftNear M R iv k).Mem (x + j * (2 * π)) := by
  unfold shiftNear
  rcases k with _ | k
  · exact ⟨0, fun x hx => by simpa using hx⟩
  · dsimp only
    split_ifs
    · exact ⟨0, fun x hx => by simpa using hx⟩
    · refine ⟨-k, fun x hx => ?_⟩
      have := isub_sound M hx (show (toIv (twoPiTimes R k)).Mem _ from twoPiTimes_sound R k)
      push_cast
      convert this using 1
      ring

/-- A direction angle in the input of `sketch.shiftNear` gives one in its output. -/
theorem shiftNear_rep {iv : Sketch.Interval.Ival} (k : Option ℤ) {a x y : ℝ} (ha : iv.Mem a)
    (hr : Rep a x y) : ∃ b, (shiftNear M R iv k).Mem b ∧ Rep b x y := by
  obtain ⟨j, hj⟩ := shiftNear_sound M R iv k
  exact ⟨a + j * (2 * π), hj a ha, rep_add_int_mul hr j⟩

end Sketch.Trig

end
