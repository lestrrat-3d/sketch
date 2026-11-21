module

public import Mathlib

/-!
# Circular wedges

Plane geometry behind the arc paths of `sketch.Sketch.CertifyConeCurveSide` (`cone_side.go`,
`cone_side_arc.go`), stated without trigonometry. A direction `w` lies in the closed counter-clockwise wedge
from `a` to `b` when `a × w ≥ 0` and `w × b ≥ 0` (`InWedge`); with `a × b > 0` the wedge is narrower than a
half turn, and its nonzero directions are exactly the directions of the circular arc from `a` to `b`. The
point of the circle of radius `ρ` about `(cx, cy)` in direction `w` is `(cx, cy) + ρ·w/|w|` (`unitR`).

`x_le_max_of_not_has` is the fact `sketch.coneArcWedge.box` relies on: over such a wedge, a coordinate of the
unit direction reaches beyond the larger of its two endpoint values only when the wedge contains the matching
axis direction, as `sketch.coneWedgeHas` tests. The three other axis directions follow by rotation
(`x_ge_min_of_not_has`, `y_le_max_of_not_has`, `y_ge_min_of_not_has`). `inWedge_split` shows the two halves
`sketch.coneArcWedge.side` recurses on cover the wedge.
-/

public section

namespace Sketch.Cone

/-- The 2D cross product `a × b`. -/
@[expose] def crossR (a b : ℝ × ℝ) : ℝ := a.1 * b.2 - a.2 * b.1

/-- `w` lies in the closed counter-clockwise wedge from `a` to `b`. -/
@[expose] def InWedge (a b w : ℝ × ℝ) : Prop := 0 ≤ crossR a w ∧ 0 ≤ crossR w b

/-- Euclidean length `|w|`. -/
@[expose] noncomputable def normR (w : ℝ × ℝ) : ℝ := Real.sqrt (w.1 * w.1 + w.2 * w.2)

/-- The unit vector `w/|w|`. -/
@[expose] noncomputable def unitR (w : ℝ × ℝ) : ℝ × ℝ := ((normR w)⁻¹ * w.1, (normR w)⁻¹ * w.2)

theorem normR_pos {w : ℝ × ℝ} (hw : w ≠ 0) : 0 < normR w := by
  unfold normR
  apply Real.sqrt_pos.2
  rcases w with ⟨x, y⟩
  by_contra h
  push Not at h
  have hx : x = 0 := by nlinarith [mul_self_nonneg x, mul_self_nonneg y]
  have hy : y = 0 := by nlinarith [mul_self_nonneg x, mul_self_nonneg y]
  exact hw (by simp [hx, hy])

theorem normR_sq (w : ℝ × ℝ) : normR w * normR w = w.1 * w.1 + w.2 * w.2 :=
  Real.mul_self_sqrt (by nlinarith [mul_self_nonneg w.1, mul_self_nonneg w.2])

/-- A unit vector has squared length 1. -/
theorem unitR_sq {w : ℝ × ℝ} (hw : w ≠ 0) : (unitR w).1 ^ 2 + (unitR w).2 ^ 2 = 1 := by
  have hn := normR_pos hw
  have h2 := normR_sq w
  simp only [unitR]
  field_simp
  linarith

/-- Normalizing scales a cross product by a positive factor. -/
theorem crossR_unitR (a b : ℝ × ℝ) :
    crossR (unitR a) (unitR b) = (normR a)⁻¹ * (normR b)⁻¹ * crossR a b := by
  simp only [crossR, unitR]
  ring

theorem crossR_unitR_pos {a b : ℝ × ℝ} (ha : a ≠ 0) (hb : b ≠ 0) (h : 0 < crossR a b) :
    0 < crossR (unitR a) (unitR b) := by
  rw [crossR_unitR]
  have := normR_pos ha
  have := normR_pos hb
  positivity

theorem crossR_unitR_nonneg {a b : ℝ × ℝ} (ha : a ≠ 0) (hb : b ≠ 0) (h : 0 ≤ crossR a b) :
    0 ≤ crossR (unitR a) (unitR b) := by
  rw [crossR_unitR]
  have := normR_pos ha
  have := normR_pos hb
  positivity

/-- Every coordinate of a unit vector lies in `[−1, 1]`. -/
theorem unitR_bounds {w : ℝ × ℝ} (hw : w ≠ 0) :
    -1 ≤ (unitR w).1 ∧ (unitR w).1 ≤ 1 ∧ -1 ≤ (unitR w).2 ∧ (unitR w).2 ≤ 1 := by
  have h := unitR_sq hw
  refine ⟨?_, ?_, ?_, ?_⟩ <;> nlinarith [sq_nonneg (unitR w).1, sq_nonneg (unitR w).2]

/-- With `a × b ≠ 0`, `(a × b)·w = (w × b)·a + (a × w)·b`, first coordinate. -/
theorem crossR_decomp_fst (a b w : ℝ × ℝ) :
    crossR a b * w.1 = crossR w b * a.1 + crossR a w * b.1 := by
  simp only [crossR]
  ring

/-- With `a × b ≠ 0`, `(a × b)·w = (w × b)·a + (a × w)·b`, second coordinate. -/
theorem crossR_decomp_snd (a b w : ℝ × ℝ) :
    crossR a b * w.2 = crossR w b * a.2 + crossR a w * b.2 := by
  simp only [crossR]
  ring

/-- For unit `p`, `s`: `(s₁ − p₁)(1 + p·s) = −(p₂ + s₂)(p × s)`. -/
theorem unit_step (p s : ℝ × ℝ) (hp : p.1 ^ 2 + p.2 ^ 2 = 1) (hs : s.1 ^ 2 + s.2 ^ 2 = 1) :
    (s.1 - p.1) * (1 + (p.1 * s.1 + p.2 * s.2)) = -(p.2 + s.2) * crossR p s := by
  simp only [crossR]
  linear_combination (-s.1) * hp + p.1 * hs

/-- For unit `p`, `s` and a third vector `r` with `p × r > 0` and `s × r ≥ 0`, `1 + p·s > 0`: `s` is not
`−p`. -/
theorem one_add_dot_pos {p s r : ℝ × ℝ} (hp : p.1 ^ 2 + p.2 ^ 2 = 1) (hs : s.1 ^ 2 + s.2 ^ 2 = 1)
    (hpr : 0 < crossR p r) (hsr : 0 ≤ crossR s r) : 0 < 1 + (p.1 * s.1 + p.2 * s.2) := by
  by_contra h
  push Not at h
  have hsum : (p.1 + s.1) ^ 2 + (p.2 + s.2) ^ 2 = 2 * (1 + (p.1 * s.1 + p.2 * s.2)) := by
    linear_combination hp + hs
  have h1 : p.1 + s.1 = 0 := by nlinarith [sq_nonneg (p.1 + s.1), sq_nonneg (p.2 + s.2)]
  have h2 : p.2 + s.2 = 0 := by nlinarith [sq_nonneg (p.1 + s.1), sq_nonneg (p.2 + s.2)]
  have : crossR s r = -crossR p r := by
    simp only [crossR]
    have e1 : s.1 = -p.1 := by linarith
    have e2 : s.2 = -p.2 := by linarith
    rw [e1, e2]
    ring
  linarith

/-- For unit `p`, `s`, `r` (the same fact with `p` and `r` swapped in the cross products). -/
theorem one_add_dot_pos' {p s r : ℝ × ℝ} (hs : s.1 ^ 2 + s.2 ^ 2 = 1) (hr : r.1 ^ 2 + r.2 ^ 2 = 1)
    (hpr : 0 < crossR p r) (hps : 0 ≤ crossR p s) : 0 < 1 + (s.1 * r.1 + s.2 * r.2) := by
  by_contra h
  push Not at h
  have hsum : (s.1 + r.1) ^ 2 + (s.2 + r.2) ^ 2 = 2 * (1 + (s.1 * r.1 + s.2 * r.2)) := by
    linear_combination hs + hr
  have h1 : s.1 + r.1 = 0 := by nlinarith [sq_nonneg (s.1 + r.1), sq_nonneg (s.2 + r.2)]
  have h2 : s.2 + r.2 = 0 := by nlinarith [sq_nonneg (s.1 + r.1), sq_nonneg (s.2 + r.2)]
  have : crossR p r = -crossR p s := by
    simp only [crossR]
    have e1 : r.1 = -s.1 := by linarith
    have e2 : r.2 = -s.2 := by linarith
    rw [e1, e2]
    ring
  linarith

/-- The case `p₂ > 0` of `x_le_max`. -/
theorem x_le_max_of_upper {p r s : ℝ × ℝ} (hp : p.1 ^ 2 + p.2 ^ 2 = 1) (hr : r.1 ^ 2 + r.2 ^ 2 = 1)
    (hs : s.1 ^ 2 + s.2 ^ 2 = 1) (hpr : 0 < crossR p r) (hps : 0 ≤ crossR p s) (hsr : 0 ≤ crossR s r)
    (hp2 : 0 < p.2) : s.1 ≤ max p.1 r.1 := by
  rcases le_or_gt 0 (p.2 + s.2) with h | h
  · have e := unit_step p s hp hs
    have hd := one_add_dot_pos hp hs hpr hsr
    have : (s.1 - p.1) * (1 + (p.1 * s.1 + p.2 * s.2)) ≤ 0 := by
      rw [e]
      nlinarith
    have : s.1 - p.1 ≤ 0 := by
      by_contra hc
      push Not at hc
      nlinarith
    exact le_max_of_le_left (by linarith)
  · have hr2 : r.2 < 0 := by
      by_contra hc
      push Not at hc
      have := crossR_decomp_snd p r s
      nlinarith
    have e := unit_step s r hs hr
    have hd := one_add_dot_pos' hs hr hpr hps
    have : (r.1 - s.1) * (1 + (s.1 * r.1 + s.2 * r.2)) ≥ 0 := by
      rw [e]
      nlinarith
    have : 0 ≤ r.1 - s.1 := by
      by_contra hc
      push Not at hc
      nlinarith
    exact le_max_of_le_right (by linarith)

/-- Over the unit directions of a wedge `p → r` narrower than a half turn, the first coordinate stays at or
below the larger endpoint value unless the wedge contains `+x` (`p₂ ≤ 0 ∧ r₂ ≥ 0`). -/
theorem x_le_max {p r s : ℝ × ℝ} (hp : p.1 ^ 2 + p.2 ^ 2 = 1) (hr : r.1 ^ 2 + r.2 ^ 2 = 1)
    (hs : s.1 ^ 2 + s.2 ^ 2 = 1) (hpr : 0 < crossR p r) (hps : 0 ≤ crossR p s) (hsr : 0 ≤ crossR s r)
    (hn : 0 < p.2 ∨ r.2 < 0) : s.1 ≤ max p.1 r.1 := by
  rcases hn with h | h
  · exact x_le_max_of_upper hp hr hs hpr hps hsr h
  · have := x_le_max_of_upper (p := (r.1, -r.2)) (r := (p.1, -p.2)) (s := (s.1, -s.2))
      (by simp only; linarith) (by simp only; linarith) (by simp only; linarith)
      (by simp only [crossR] at hpr ⊢; linarith) (by simp only [crossR] at hsr ⊢; linarith)
      (by simp only [crossR] at hps ⊢; linarith) (by simp only; linarith)
    simpa only [max_comm] using this

/-- `x_le_max` for the rays `a`, `b`, `w` themselves, through their unit vectors, with the wedge test
`sketch.coneWedgeHas` against `+x = (1, 0)` false. -/
theorem x_le_max_of_not_has {a b w : ℝ × ℝ} (ha : a ≠ 0) (hb : b ≠ 0) (hw : w ≠ 0)
    (hab : 0 < crossR a b) (hin : InWedge a b w) (hn : ¬(0 ≤ crossR a (1, 0) ∧ 0 ≤ crossR (1, 0) b)) :
    (unitR w).1 ≤ max (unitR a).1 (unitR b).1 := by
  apply x_le_max (unitR_sq ha) (unitR_sq hb) (unitR_sq hw) (crossR_unitR_pos ha hb hab)
    (crossR_unitR_nonneg ha hw hin.1) (crossR_unitR_nonneg hw hb hin.2)
  have hna := normR_pos ha
  have hnb := normR_pos hb
  have hn' : a.2 ≤ 0 → b.2 < 0 := fun h => by
    by_contra hc
    push Not at hc
    exact hn ⟨by simp only [crossR]; linarith, by simp only [crossR]; linarith⟩
  simp only [unitR]
  by_cases h : a.2 ≤ 0
  · right
    have := hn' h
    have : (normR b)⁻¹ * b.2 < 0 := mul_neg_of_pos_of_neg (inv_pos.2 hnb) this
    exact this
  · left
    push Not at h
    exact mul_pos (inv_pos.2 hna) h

theorem crossR_neg (a b : ℝ × ℝ) : crossR (-a.1, -a.2) (-b.1, -b.2) = crossR a b := by
  simp only [crossR]
  ring

/-- The `−x = (−1, 0)` case: the first coordinate stays at or above the smaller endpoint value unless the
wedge contains `−x`. -/
theorem x_ge_min_of_not_has {a b w : ℝ × ℝ} (ha : a ≠ 0) (hb : b ≠ 0) (hw : w ≠ 0)
    (hab : 0 < crossR a b) (hin : InWedge a b w) (hn : ¬(0 ≤ crossR a (-1, 0) ∧ 0 ≤ crossR (-1, 0) b)) :
    min (unitR a).1 (unitR b).1 ≤ (unitR w).1 := by
  have hp := unitR_sq ha
  have hr := unitR_sq hb
  have hs := unitR_sq hw
  have h := x_le_max (p := (-(unitR a).1, -(unitR a).2)) (r := (-(unitR b).1, -(unitR b).2))
    (s := (-(unitR w).1, -(unitR w).2)) (by simp only; linarith) (by simp only; linarith)
    (by simp only; linarith) (by rw [crossR_neg]; exact crossR_unitR_pos ha hb hab)
    (by rw [crossR_neg]; exact crossR_unitR_nonneg ha hw hin.1)
    (by rw [crossR_neg]; exact crossR_unitR_nonneg hw hb hin.2) ?_
  · simp only at h
    rw [min_le_iff]
    rw [le_max_iff] at h
    rcases h with h | h
    · left; linarith
    · right; linarith
  · have hna := normR_pos ha
    have hnb := normR_pos hb
    have hn' : 0 ≤ a.2 → 0 < b.2 := fun h => by
      by_contra hc
      push Not at hc
      exact hn ⟨by simp only [crossR]; linarith, by simp only [crossR]; linarith⟩
    simp only [unitR]
    by_cases h : 0 ≤ a.2
    · right
      have := hn' h
      have : 0 < (normR b)⁻¹ * b.2 := mul_pos (inv_pos.2 hnb) this
      linarith
    · left
      push Not at h
      have : (normR a)⁻¹ * a.2 < 0 := mul_neg_of_pos_of_neg (inv_pos.2 hna) h
      linarith

theorem crossR_rot (a b : ℝ × ℝ) : crossR (a.2, -a.1) (b.2, -b.1) = crossR a b := by
  simp only [crossR]
  ring

theorem crossR_rot' (a b : ℝ × ℝ) : crossR (-a.2, a.1) (-b.2, b.1) = crossR a b := by
  simp only [crossR]
  ring

/-- The `+y = (0, 1)` case: the second coordinate stays at or below the larger endpoint value unless the
wedge contains `+y`. -/
theorem y_le_max_of_not_has {a b w : ℝ × ℝ} (ha : a ≠ 0) (hb : b ≠ 0) (hw : w ≠ 0)
    (hab : 0 < crossR a b) (hin : InWedge a b w) (hn : ¬(0 ≤ crossR a (0, 1) ∧ 0 ≤ crossR (0, 1) b)) :
    (unitR w).2 ≤ max (unitR a).2 (unitR b).2 := by
  have hp := unitR_sq ha
  have hr := unitR_sq hb
  have hs := unitR_sq hw
  apply x_le_max (p := ((unitR a).2, -(unitR a).1)) (r := ((unitR b).2, -(unitR b).1))
    (s := ((unitR w).2, -(unitR w).1)) (by simp only; linarith) (by simp only; linarith)
    (by simp only; linarith) (by rw [crossR_rot]; exact crossR_unitR_pos ha hb hab)
    (by rw [crossR_rot]; exact crossR_unitR_nonneg ha hw hin.1)
    (by rw [crossR_rot]; exact crossR_unitR_nonneg hw hb hin.2)
  have hna := normR_pos ha
  have hnb := normR_pos hb
  have hn' : 0 ≤ a.1 → 0 < b.1 := fun h => by
    by_contra hc
    push Not at hc
    exact hn ⟨by simp only [crossR]; linarith, by simp only [crossR]; linarith⟩
  simp only [unitR]
  by_cases h : 0 ≤ a.1
  · right
    have := hn' h
    have : 0 < (normR b)⁻¹ * b.1 := mul_pos (inv_pos.2 hnb) this
    linarith
  · left
    push Not at h
    have : (normR a)⁻¹ * a.1 < 0 := mul_neg_of_pos_of_neg (inv_pos.2 hna) h
    linarith

/-- The `−y = (0, −1)` case: the second coordinate stays at or above the smaller endpoint value unless the
wedge contains `−y`. -/
theorem y_ge_min_of_not_has {a b w : ℝ × ℝ} (ha : a ≠ 0) (hb : b ≠ 0) (hw : w ≠ 0)
    (hab : 0 < crossR a b) (hin : InWedge a b w) (hn : ¬(0 ≤ crossR a (0, -1) ∧ 0 ≤ crossR (0, -1) b)) :
    min (unitR a).2 (unitR b).2 ≤ (unitR w).2 := by
  have hp := unitR_sq ha
  have hr := unitR_sq hb
  have hs := unitR_sq hw
  have h := x_le_max (p := (-(unitR a).2, (unitR a).1)) (r := (-(unitR b).2, (unitR b).1))
    (s := (-(unitR w).2, (unitR w).1)) (by simp only; linarith) (by simp only; linarith)
    (by simp only; linarith) (by rw [crossR_rot']; exact crossR_unitR_pos ha hb hab)
    (by rw [crossR_rot']; exact crossR_unitR_nonneg ha hw hin.1)
    (by rw [crossR_rot']; exact crossR_unitR_nonneg hw hb hin.2) ?_
  · simp only at h
    rw [min_le_iff]
    rw [le_max_iff] at h
    rcases h with h | h
    · left; linarith
    · right; linarith
  · have hna := normR_pos ha
    have hnb := normR_pos hb
    have hn' : a.1 ≤ 0 → b.1 < 0 := fun h => by
      by_contra hc
      push Not at hc
      exact hn ⟨by simp only [crossR]; linarith, by simp only [crossR]; linarith⟩
    simp only [unitR]
    by_cases h : a.1 ≤ 0
    · right
      have := hn' h
      have : (normR b)⁻¹ * b.1 < 0 := mul_neg_of_pos_of_neg (inv_pos.2 hnb) this
      exact this
    · left
      push Not at h
      exact mul_pos (inv_pos.2 hna) h

/-- A divider `m` strictly inside the wedge `a → b` splits it: every direction of the wedge lies in
`a → m` or in `m → b`. -/
theorem inWedge_split {a b m w : ℝ × ℝ} (hin : InWedge a b w) :
    InWedge a m w ∨ InWedge m b w := by
  rcases le_or_gt 0 (crossR w m) with h | h
  · exact Or.inl ⟨hin.1, h⟩
  · refine Or.inr ⟨?_, hin.2⟩
    have : crossR m w = -crossR w m := by simp only [crossR]; ring
    linarith

/-- A ray in the wedge `a → b` with `a × b > 0` keeps both endpoints nonzero. -/
theorem ne_zero_of_crossR_pos {a b : ℝ × ℝ} (h : 0 < crossR a b) : a ≠ 0 ∧ b ≠ 0 := by
  constructor
  · rintro rfl
    simp [crossR] at h
  · rintro rfl
    simp [crossR] at h

end Sketch.Cone

end
