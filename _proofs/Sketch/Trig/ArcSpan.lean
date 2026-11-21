module

public import Sketch.Trig.Atan2Box

/-!
# Certified arc span bounds

Model of `sketch.certifiedArcSpanBounds`, the machinery behind `sketch.Arc.CertifiedSpanBounds`
(`docs/arc-span-bounds-design.md`). The arc's ideal circular carrier has the exact centre `(cx, cy)`, the exact
radius `r = |Start − Center|`, and parameter `t` at angle `α + t·σ`, where `α` is a direction angle of
`Start − Center` and `σ ∈ (0, 2π)` is the counter-clockwise sweep to a direction angle of `End − Center`. That
sweep is unique and the point does not depend on which `α` is chosen (`arc_carrier_unique`).

The Go code brackets both directions with `sketch.atan2Box`, lets the float sweep choose a whole-turn shift of
the end bracket, and accepts only when the interval sweep lies strictly inside `(0, 2π)`. The theorem holds for
whichever shift the float code picks. The ownership, staleness and non-finite refusals are not modeled: the
model returns in every case the Go code returns, and the theorem covers all of them.
-/

public section

namespace Sketch.Trig

open Real Sketch.Float
open Sketch.Interval (pt iadd isub imul isqr isqrtNonNeg iadd_sound isub_sound imul_sound isqr_sound
  isqrtNonNeg_sound pt_sound)

/-! ### The carrier -/

/-- The counter-clockwise sweep between two directions is unique in `(0, 2π)`, and the carrier point at every
parameter does not depend on the direction angle chosen for the start. -/
theorem arc_carrier_unique {sx sy ex ey α σ α' σ' : ℝ} (hα : Rep α sx sy) (hσ : 0 < σ ∧ σ < 2 * π)
    (he : Rep (α + σ) ex ey) (hα' : Rep α' sx sy) (hσ' : 0 < σ' ∧ σ' < 2 * π) (he' : Rep (α' + σ') ex ey)
    (t : ℝ) : cos (α' + t * σ') = cos (α + t * σ) ∧ sin (α' + t * σ') = sin (α + t * σ) := by
  obtain ⟨k, hk⟩ := rep_unique hα hα'
  obtain ⟨m, hm⟩ := rep_unique he he'
  have hkm : m = k := by
    by_contra hne
    rcases lt_or_gt_of_ne hne with hlt | hgt
    · have h1 : (m : ℝ) ≤ k - 1 := by exact_mod_cast Int.le_sub_one_of_lt hlt
      have : 2 * π ≤ 2 * π * ((k : ℝ) - m) := by nlinarith [pi_pos]
      nlinarith
    · have h1 : (k : ℝ) ≤ m - 1 := by exact_mod_cast Int.le_sub_one_of_lt hgt
      have : 2 * π ≤ 2 * π * ((m : ℝ) - k) := by nlinarith [pi_pos]
      nlinarith
  subst hkm
  have hσeq : σ' = σ := by linarith
  subst hσeq
  have e : α' + t * σ' = α + t * σ' + m * (2 * π) := by rw [hk]; ring
  rw [e, cos_add_int_mul_two_pi, sin_add_int_mul_two_pi]
  exact ⟨rfl, rfl⟩

/-! ### `certifiedArcSpanBounds` -/

variable (M : FloatModel) (R : Rounding)

/-- Model of `sketch.certifiedArcSpanBounds`. `B` is the rectangle enclosure of a direction
(`sketch.atan2Box`) and `S` the interval enclosure of sine and cosine (`sketch.sinCosRange`). `current` is the
ownership and staleness test, and `shiftOf start end` the float64 whole-turn decision
`int64(math.Round((start.mid() + a.Sweep() - end.mid()) / (2 * math.Pi)))`. Coordinates are the exact reals of
the defining points' float64 values; the non-finite refusals are omitted (A2). -/
@[expose] noncomputable def certifiedArcSpanBounds
    (B : Sketch.Interval.Ival → Sketch.Interval.Ival → Option Sketch.Interval.Ival)
    (S : Sketch.Interval.Ival → Option (Ival × Ival)) (current : Bool)
    (shiftOf : Sketch.Interval.Ival → Sketch.Interval.Ival → ℤ) (cx cy sx sy ex ey lo hi : ℝ) :
    Option (Sketch.Interval.Ival × Sketch.Interval.Ival) :=
  if current = false then none
  else if lo < 0 ∨ hi > 1 ∨ lo > hi then none
  else
    let startX := isub M (pt sx) (pt cx)
    let startY := isub M (pt sy) (pt cy)
    let endX := isub M (pt ex) (pt cx)
    let endY := isub M (pt ey) (pt cy)
    match B startY startX with
    | none => none
    | some start =>
      match B endY endX with
      | none => none
      | some en =>
        let shift := shiftOf start en
        if shift < -2 ∨ shift > 2 then none
        else
          let delta := isub M (iadd M en (toIv (twoPiTimes R shift))) start
          if delta.lo ≤ 0 ∨ delta.hi ≥ (twoPiTimes R 1).lo then none
          else
            let radius := isqrtNonNeg M (iadd M (isqr M startX) (isqr M startY))
            if radius.lo ≤ 0 then none
            else
              let angles := iadd M start (imul M ⟨lo, hi⟩ delta)
              match S angles with
              | none => none
              | some (sn, cs) =>
                some (iadd M (pt cx) (imul M radius (toIv cs)), iadd M (pt cy) (imul M radius (toIv sn)))

/-- **Soundness of the arc span bounds.** Whenever the model of `sketch.certifiedArcSpanBounds` returns boxes
`X × Y`, the arc has a counter-clockwise sweep `σ ∈ (0, 2π)` from a direction angle `α` of `Start − Center` to
one of `End − Center`, and for every such `α` and `σ` and every `t ∈ [lo, hi]` the carrier point
`Center + r·(cos (α + t·σ), sin (α + t·σ))`, with `r = |Start − Center|`, lies in `X × Y`. `hB` is the soundness
of the rectangle enclosure (`atan2Box_sound`) and `hS` that of the interval sine and cosine
(`sinCosRange_sound`). Covers `sketch.certifiedArcSpanBounds`, `sketch.twoPiTimes` and its interval helpers.
Assumptions: A1 to A6, A20, A22, A24, A28, A70. -/
theorem certifiedArcSpanBounds_sound
    (B : Sketch.Interval.Ival → Sketch.Interval.Ival → Option Sketch.Interval.Ival)
    (hB : ∀ c d out cv dv, B c d = some out → c.Mem cv → d.Mem dv → ∃ θ, out.Mem θ ∧ Rep θ dv cv)
    (S : Sketch.Interval.Ival → Option (Ival × Ival))
    (hS : ∀ q sn cs, S q = some (sn, cs) → ∀ θ, q.Mem θ → sn.Mem (sin θ) ∧ cs.Mem (cos θ))
    (current : Bool) (shiftOf : Sketch.Interval.Ival → Sketch.Interval.Ival → ℤ) (cx cy sx sy ex ey lo hi : ℝ)
    {X Y : Sketch.Interval.Ival}
    (h : certifiedArcSpanBounds M R B S current shiftOf cx cy sx sy ex ey lo hi = some (X, Y)) :
    (∃ α σ, Rep α (sx - cx) (sy - cy) ∧ (0 < σ ∧ σ < 2 * π) ∧ Rep (α + σ) (ex - cx) (ey - cy)) ∧
    ∀ α σ, Rep α (sx - cx) (sy - cy) → (0 < σ ∧ σ < 2 * π) → Rep (α + σ) (ex - cx) (ey - cy) →
      ∀ t, lo ≤ t → t ≤ hi →
        X.Mem (cx + √((sx - cx) ^ 2 + (sy - cy) ^ 2) * cos (α + t * σ)) ∧
        Y.Mem (cy + √((sx - cx) ^ 2 + (sy - cy) ^ 2) * sin (α + t * σ)) := by
  unfold certifiedArcSpanBounds at h
  split_ifs at h with hcur hrange
  dsimp only at h
  split at h
  · exact absurd h (by simp)
  rename_i start hstart
  split at h
  · exact absurd h (by simp)
  rename_i en hen
  split_ifs at h with hshift hdelta hrad
  split at h
  · exact absurd h (by simp)
  rename_i sn cs hsc
  simp only [Option.some.injEq, Prod.mk.injEq] at h
  obtain ⟨rfl, rfl⟩ := h
  have hsX := isub_sound M (pt_sound sx) (pt_sound cx)
  have hsY := isub_sound M (pt_sound sy) (pt_sound cy)
  obtain ⟨s', hs'mem, hs'⟩ := hB _ _ _ _ _ hstart hsY hsX
  obtain ⟨e', he'mem, he'⟩ := hB _ _ _ _ _ hen (isub_sound M (pt_sound ey) (pt_sound cy))
    (isub_sound M (pt_sound ex) (pt_sound cx))
  set k := shiftOf start en
  set σ' := (e' + k * (2 * π)) - s'
  have hdmem : (isub M (iadd M en (toIv (twoPiTimes R k))) start).Mem σ' :=
    isub_sound M (iadd_sound M he'mem (show (toIv (twoPiTimes R k)).Mem _ from twoPiTimes_sound R k)) hs'mem
  have h2π := (twoPiTimes_sound R 1).1
  push_cast at h2π
  push Not at hdelta
  have hσ' : 0 < σ' ∧ σ' < 2 * π :=
    ⟨lt_of_lt_of_le hdelta.1 hdmem.1, by linarith [hdmem.2, hdelta.2]⟩
  have hend' : Rep (s' + σ') (ex - cx) (ey - cy) := by
    rw [show s' + σ' = e' + k * (2 * π) by ring]
    exact rep_add_int_mul he' k
  refine ⟨⟨s', σ', hs', hσ', hend'⟩, ?_⟩
  intro α σ hα hσ he t ht1 ht2
  obtain ⟨hcos, hsin⟩ := arc_carrier_unique hs' hσ' hend' hα hσ he t
  rw [hcos, hsin]
  have hr := isqrtNonNeg_sound M (iadd_sound M (isqr_sound M hsX) (isqr_sound M hsY))
    (add_nonneg (mul_self_nonneg (sx - cx)) (mul_self_nonneg (sy - cy)))
  rw [show (sx - cx) * (sx - cx) + (sy - cy) * (sy - cy) = (sx - cx) ^ 2 + (sy - cy) ^ 2 by ring] at hr
  have hang := iadd_sound M hs'mem
    (imul_sound M (show (⟨lo, hi⟩ : Sketch.Interval.Ival).Mem t from ⟨ht1, ht2⟩) hdmem)
  obtain ⟨hsn, hcs⟩ := hS _ _ _ hsc _ hang
  exact ⟨iadd_sound M (pt_sound cx) (imul_sound M hr (show (toIv cs).Mem _ from hcs)),
    iadd_sound M (pt_sound cy) (imul_sound M hr (show (toIv sn).Mem _ from hsn))⟩

/-! ### On binary64 -/

/-- The interval sine and cosine `sketch.sinCosRange` reads, with its float64 decisions from `TrigEnv`, as
`sketch.certifiedArcSpanBounds` and `sketch.sinCosPoint` call it. -/
@[expose] noncomputable def sinCosRangeOf (E : TrigEnv) (fuel : ℕ) (q : Sketch.Interval.Ival) :
    Option (Ival × Ival) :=
  let q' : Ival := ⟨q.lo, q.hi⟩
  sinCosRange R (sinCosPoint R E fuel) q' (E.wide q') (E.kmin q') (E.kmax q')

/-- `sinCosRangeOf` encloses sine and cosine over its interval. Assumptions: A20 to A28. -/
theorem sinCosRangeOf_sound (E : TrigEnv) (fuel : ℕ) :
    ∀ q sn cs, sinCosRangeOf R E fuel q = some (sn, cs) → ∀ θ, q.Mem θ → sn.Mem (sin θ) ∧ cs.Mem (cos θ) := by
  intro q sn cs h θ hθ
  exact sinCosRange_sound R (sinCosPoint R E fuel) _ _ _ _ (sinCosPoint_sound R E fuel)
    (fun hw ha hb => E.k_cover _ hw (sinCosPoint_bound R E fuel _ ha) (sinCosPoint_bound R E fuel _ hb)) h θ
    hθ.1 hθ.2

/-- The two facts about `sketch.atan2Box`'s float decisions that assumption A72 states, for every rectangle
that misses the origin: `ref` lies within `1/10` rad of a direction angle of a point of a nonempty rectangle,
and a shifted corner bracket that passes the 3 rad test lies within `3 + 2^-20` of `ref`. `envOf c d` is the
`BoxEnv` of the call on `c × d`. -/
structure BoxDecisions (A : ℝ → ℝ → Option Sketch.Interval.Ival)
    (envOf : Sketch.Interval.Ival → Sketch.Interval.Ival → BoxEnv) : Prop where
  ref_near : ∀ c d : Sketch.Interval.Ival, ¬(c.lo ≤ 0 ∧ 0 ≤ c.hi ∧ d.lo ≤ 0 ∧ 0 ≤ d.hi) →
    (∃ mc md, c.Mem mc ∧ d.Mem md) →
    ∃ mc md θ, c.Mem mc ∧ d.Mem md ∧ Rep θ md mc ∧ |(envOf c d).ref - θ| ≤ 1 / 10
  bracket_near : ∀ (c d : Sketch.Interval.Ival) c0 d0 I a, A c0 d0 = some I →
    (envOf c d).far (turnShift M R (envOf c d) I) = false → (turnShift M R (envOf c d) I).Mem a →
    |a - (envOf c d).ref| ≤ 3 + 1 / 2 ^ 20

/-- `sketch.atan2Box` as its callers run it: with the float decisions of the call on `c × d`. -/
@[expose] noncomputable def atan2BoxOf (A : ℝ → ℝ → Option Sketch.Interval.Ival)
    (envOf : Sketch.Interval.Ival → Sketch.Interval.Ival → BoxEnv) (c d : Sketch.Interval.Ival) :
    Option Sketch.Interval.Ival :=
  atan2Box M R A (envOf c d) c d

/-- `atan2BoxOf` is a sound rectangle enclosure under assumption A72 (`BoxDecisions`). -/
theorem atan2BoxOf_sound (A : ℝ → ℝ → Option Sketch.Interval.Ival)
    (hA : ∀ c0 d0 I, A c0 d0 = some I → ∃ θ, I.Mem θ ∧ Rep θ d0 c0)
    (envOf : Sketch.Interval.Ival → Sketch.Interval.Ival → BoxEnv) (hD : BoxDecisions M R A envOf) :
    ∀ c d out cv dv, atan2BoxOf M R A envOf c d = some out → c.Mem cv → d.Mem dv →
      ∃ θ, out.Mem θ ∧ Rep θ dv cv := by
  intro c d out cv dv h hcv hdv
  have horig : ¬(c.lo ≤ 0 ∧ 0 ≤ c.hi ∧ d.lo ≤ 0 ∧ 0 ≤ d.hi) := by
    intro ho
    unfold atan2BoxOf atan2Box at h
    rw [ite_eq_left ho] at h
    exact absurd h (by simp)
  exact atan2Box_sound M R A hA (envOf c d) c d (hD.ref_near c d horig ⟨cv, dv, hcv, hdv⟩)
    (hD.bracket_near c d) h cv dv hcv hdv

/-- **Arc span bounds on binary64.** `sketch.certifiedArcSpanBounds` with the `interval_trig.go` enclosures:
`sketch.atan2Box` over `sketch.atan2Point` and `sketch.sinCosPoint`, and `sketch.sinCosRange`. Whenever it returns
boxes `X × Y`, the counter-clockwise sweep `σ ∈ (0, 2π)` exists and every carrier point with parameter in
`[lo, hi]` lies in `X × Y`, as in `certifiedArcSpanBounds_sound`. Covers `sketch.certifiedArcSpanBounds` and every
function it reaches in `interval_trig.go` and `interval.go`. Assumptions: A1 to A6, A20 to A28, A70 to A72. -/
theorem certifiedArcSpanBounds_sound_binary64 (E : TrigEnv) (fuel : ℕ) (aenv : Atan2Env)
    (envOf : Sketch.Interval.Ival → Sketch.Interval.Ival → BoxEnv)
    (hD : BoxDecisions M (Rounding.ofFloat M) (atan2Point M (sinCosPoint (Rounding.ofFloat M) E fuel) aenv) envOf)
    (current : Bool) (shiftOf : Sketch.Interval.Ival → Sketch.Interval.Ival → ℤ) (cx cy sx sy ex ey lo hi : ℝ)
    {X Y : Sketch.Interval.Ival}
    (h : certifiedArcSpanBounds M (Rounding.ofFloat M)
      (atan2BoxOf M (Rounding.ofFloat M) (atan2Point M (sinCosPoint (Rounding.ofFloat M) E fuel) aenv) envOf)
      (sinCosRangeOf (Rounding.ofFloat M) E fuel) current shiftOf cx cy sx sy ex ey lo hi = some (X, Y)) :
    (∃ α σ, Rep α (sx - cx) (sy - cy) ∧ (0 < σ ∧ σ < 2 * π) ∧ Rep (α + σ) (ex - cx) (ey - cy)) ∧
    ∀ α σ, Rep α (sx - cx) (sy - cy) → (0 < σ ∧ σ < 2 * π) → Rep (α + σ) (ex - cx) (ey - cy) →
      ∀ t, lo ≤ t → t ≤ hi →
        X.Mem (cx + √((sx - cx) ^ 2 + (sy - cy) ^ 2) * cos (α + t * σ)) ∧
        Y.Mem (cy + √((sx - cx) ^ 2 + (sy - cy) ^ 2) * sin (α + t * σ)) :=
  certifiedArcSpanBounds_sound M (Rounding.ofFloat M) _
    (atan2BoxOf_sound M (Rounding.ofFloat M) _ (fun c0 d0 _ hI => atan2Point_sound_binary64 M E fuel aenv c0 d0 hI)
      envOf hD)
    _ (sinCosRangeOf_sound (Rounding.ofFloat M) E fuel) current shiftOf cx cy sx sy ex ey lo hi h

end Sketch.Trig

end
