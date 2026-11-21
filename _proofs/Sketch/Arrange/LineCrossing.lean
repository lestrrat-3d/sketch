module

public import Mathlib

/-!
# Exact crossing of two line segments

Models `sketch.exactLineCrossing` (`profile_junction_line_line.go`), the exact rational solve behind
`Sketch.CertifyLineLineJunction`. The segments run `A→B` and `C→D`. With `u = B − A`, `v = D − C`,
`w = C − A` and `den = ux·vy − uy·vx`, the function refuses a zero `den` (parallel carriers), solves
`t = (wx·vy − wy·vx)/den` and `s = (wx·uy − wy·ux)/den`, refuses a parameter outside `[0, 1]`, forms
`A + t·u`, and returns it only when `big.Rat.Float64` reports both coordinates exact.

The model works over ℚ (A30, A34). `big.Rat.Float64` is the parameter `toF`: it returns the float as a
rational and the exact flag, and the hypothesis `ExactFlag` (A24) says a true flag means the float equals
the rational. The Go `IsInf` check is implied by an exact flag and is not modeled.

`exactLineCrossing_sound` proves a returned point lies on both segments at parameters in `[0, 1]`.
`exactLineCrossing_unique` proves it is the only point the two carrier lines share.
`exactLineCrossing_complete` proves any common point of the segments whose coordinates are exact floats
is returned, and `exactLineCrossing_parallel` proves the refusal of parallel carriers.
-/

public section

namespace Sketch.Arrange

/-- `big.Rat.Float64` as the rational value of the returned float and its exact flag. -/
abbrev RatFloat64 := ℚ → ℚ × Bool

/-- A24: a true exact flag means the returned float equals the rational. -/
def ExactFlag (toF : RatFloat64) : Prop := ∀ x : ℚ, (toF x).2 = true → (toF x).1 = x

/-- Model of `sketch.exactLineCrossing`, on the exact values of the eight finite coordinates (A34).
`none` is the Go `false` return; `some (x, y)` is the returned binary64 point read as rationals. -/
@[expose] def exactLineCrossing (toF : RatFloat64) (ax ay bx «by» cx cy dx dy : ℚ) : Option (ℚ × ℚ) :=
  let ux := bx - ax
  let uy := «by» - ay
  let vx := dx - cx
  let vy := dy - cy
  let den := ux * vy - uy * vx
  if den = 0 then none
  else
    let wx := cx - ax
    let wy := cy - ay
    let t := (wx * vy - wy * vx) / den
    let u := (wx * uy - wy * ux) / den
    if t < 0 ∨ 1 < t ∨ u < 0 ∨ 1 < u then none
    else
      let x := ax + t * ux
      let y := ay + t * uy
      if (toF x).2 = true ∧ (toF y).2 = true then some ((toF x).1, (toF y).1) else none

/-- `(px, py)` lies on segment `A→B` at parameter `t ∈ [0, 1]` and on segment `C→D` at `u ∈ [0, 1]`. -/
def OnBoth (ax ay bx «by» cx cy dx dy px py : ℚ) : Prop :=
  ∃ t u : ℚ, 0 ≤ t ∧ t ≤ 1 ∧ 0 ≤ u ∧ u ≤ 1 ∧
    ax + t * (bx - ax) = px ∧ ay + t * («by» - ay) = py ∧
    cx + u * (dx - cx) = px ∧ cy + u * (dy - cy) = py

/-- Parallel (or degenerate) carriers are refused. -/
theorem exactLineCrossing_parallel (toF : RatFloat64) (ax ay bx «by» cx cy dx dy : ℚ)
    (h : (bx - ax) * (dy - cy) - («by» - ay) * (dx - cx) = 0) :
    exactLineCrossing toF ax ay bx «by» cx cy dx dy = none := by
  simp [exactLineCrossing, h]

/-- A returned point lies on both segments, at parameters in `[0, 1]`: `sketch.exactLineCrossing` returns
a point only when the exact rational crossing is on both input segments and is the float it returns.
Hypotheses: A24 (`ExactFlag`), A30 and A34 (exact ℚ arithmetic in the model). -/
theorem exactLineCrossing_sound (toF : RatFloat64) (hF : ExactFlag toF)
    (ax ay bx «by» cx cy dx dy px py : ℚ)
    (h : exactLineCrossing toF ax ay bx «by» cx cy dx dy = some (px, py)) :
    OnBoth ax ay bx «by» cx cy dx dy px py := by
  unfold exactLineCrossing at h
  simp only at h
  split_ifs at h with hden hr hex
  obtain ⟨hx, hy⟩ := hex
  have hpx : (toF (ax + (((cx - ax) * (dy - cy) - (cy - ay) * (dx - cx)) /
      ((bx - ax) * (dy - cy) - («by» - ay) * (dx - cx))) * (bx - ax))).1 = px := by
    have := congrArg Prod.fst (Option.some.inj h)
    exact this
  have hpy : (toF (ay + (((cx - ax) * (dy - cy) - (cy - ay) * (dx - cx)) /
      ((bx - ax) * (dy - cy) - («by» - ay) * (dx - cx))) * («by» - ay))).1 = py := by
    have := congrArg Prod.snd (Option.some.inj h)
    exact this
  rw [hF _ hx] at hpx
  rw [hF _ hy] at hpy
  push Not at hr
  obtain ⟨ht0, ht1, hu0, hu1⟩ := hr
  refine ⟨_, _, ht0, ht1, hu0, hu1, hpx, hpy, ?_, ?_⟩ <;> [rw [← hpx]; rw [← hpy]] <;> field_simp <;> ring

/-- The returned point is the only point the two carrier lines share: any `(qx, qy)` on both infinite lines
equals it, because a nonzero determinant makes the crossing unique. -/
theorem exactLineCrossing_unique (toF : RatFloat64) (hF : ExactFlag toF)
    (ax ay bx «by» cx cy dx dy px py : ℚ)
    (h : exactLineCrossing toF ax ay bx «by» cx cy dx dy = some (px, py))
    (qx qy s v : ℚ)
    (h1 : ax + s * (bx - ax) = qx) (h2 : ay + s * («by» - ay) = qy)
    (h3 : cx + v * (dx - cx) = qx) (h4 : cy + v * (dy - cy) = qy) :
    qx = px ∧ qy = py := by
  have hden : (bx - ax) * (dy - cy) - («by» - ay) * (dx - cx) ≠ 0 := by
    intro h0
    rw [exactLineCrossing_parallel toF _ _ _ _ _ _ _ _ h0] at h
    simp at h
  obtain ⟨t, u, -, -, -, -, e1, e2, e3, e4⟩ := exactLineCrossing_sound toF hF _ _ _ _ _ _ _ _ _ _ h
  have hs : s = t := by
    have key : (s - t) * ((bx - ax) * (dy - cy) - («by» - ay) * (dx - cx)) = 0 := by
      have a1 : (s - t) * (bx - ax) = (v - u) * (dx - cx) := by linarith
      have a2 : (s - t) * («by» - ay) = (v - u) * (dy - cy) := by linarith
      calc (s - t) * ((bx - ax) * (dy - cy) - («by» - ay) * (dx - cx))
          = ((s - t) * (bx - ax)) * (dy - cy) - ((s - t) * («by» - ay)) * (dx - cx) := by ring
        _ = 0 := by rw [a1, a2]; ring
    rcases mul_eq_zero.mp key with k | k
    · linarith
    · exact absurd k hden
  subst hs
  constructor <;> linarith

/-- Completeness: if the segments share a point whose coordinates are exact floats and the carriers are not
parallel, `sketch.exactLineCrossing` returns that point. -/
theorem exactLineCrossing_complete (toF : RatFloat64) (hF : ExactFlag toF)
    (ax ay bx «by» cx cy dx dy qx qy : ℚ)
    (hden : (bx - ax) * (dy - cy) - («by» - ay) * (dx - cx) ≠ 0)
    (hq : OnBoth ax ay bx «by» cx cy dx dy qx qy)
    (hqx : (toF qx).2 = true) (hqy : (toF qy).2 = true) :
    exactLineCrossing toF ax ay bx «by» cx cy dx dy = some (qx, qy) := by
  obtain ⟨t, u, ht0, ht1, hu0, hu1, e1, e2, e3, e4⟩ := hq
  have hT : ((cx - ax) * (dy - cy) - (cy - ay) * (dx - cx)) /
      ((bx - ax) * (dy - cy) - («by» - ay) * (dx - cx)) = t := by
    rw [div_eq_iff hden]
    have h5 : cx - ax = t * (bx - ax) - u * (dx - cx) := by linarith
    have h6 : cy - ay = t * («by» - ay) - u * (dy - cy) := by linarith
    rw [h5, h6]; ring
  have hU : ((cx - ax) * («by» - ay) - (cy - ay) * (bx - ax)) /
      ((bx - ax) * (dy - cy) - («by» - ay) * (dx - cx)) = u := by
    rw [div_eq_iff hden]
    have h5 : cx - ax = t * (bx - ax) - u * (dx - cx) := by linarith
    have h6 : cy - ay = t * («by» - ay) - u * (dy - cy) := by linarith
    rw [h5, h6]; ring
  unfold exactLineCrossing
  simp only [hden, ite_false, hT, hU, e1, e2]
  have : ¬ (t < 0 ∨ 1 < t ∨ u < 0 ∨ 1 < u) := by
    push Not; exact ⟨ht0, ht1, hu0, hu1⟩
  simp only [this, ite_false, hqx, hqy, and_self, ite_true, hF _ hqx, hF _ hqy]

end Sketch.Arrange

end
