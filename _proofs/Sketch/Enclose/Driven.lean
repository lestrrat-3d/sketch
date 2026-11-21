module

public import Sketch.Trig.ArcSpan
public import Sketch.Enclose.Angle

/-!
# Driven dimensions over a box

Models of the evaluators `sketch.certDrivenOf` returns, which `sketch.Sketch.Enclose` runs over each piece's box
to enclose every driven dimension's measured value, and of the angle step of `sketch.encloseRun.piece`, which
moves a driven angle's interval by whole turns with `sketch.shiftNear`.

A box gives each point coordinate an interval (`e.px(p)`, `e.py(p)`; here `PtBox`). For every configuration
whose coordinates lie in the box, the horizontal, vertical and point-to-point distance evaluators contain the
exact measured value `x2 − x1`, `y2 − y1` and `dist(P1, P2)`, and the angle evaluator contains a value equal to
the signed angle `atan2(cross, dot)` modulo `2π`: the value `sketch.Angle.residual` measures, as
`Sketch.Enclose.signedAngle` models it. The angle evaluator is `sketch.atan2Box` over the cross and dot product
intervals of `sketch.certAngle.parts`; its soundness is a hypothesis (`atan2Box_sound`), discharged on binary64
in `drivenAngle_sound_binary64`.
-/

public section

namespace Sketch.Enclose

open Real Sketch.Float
open Sketch.Trig (Rep rep_eq_arg)
open Sketch.Interval (iadd isub imul isqr isqrtNonNeg iadd_sound isub_sound imul_sound isqr_sound
  isqrtNonNeg_sound)

variable (M : FloatModel)

/-- The box intervals of one point's coordinates, `e.px(p)` and `e.py(p)` in `sketch.certDrivenOf`. -/
structure PtBox where
  /-- The x coordinate's interval. -/
  x : Sketch.Interval.Ival
  /-- The y coordinate's interval. -/
  y : Sketch.Interval.Ival

/-- The point `p` lies in the box `b`. -/
@[expose] def PtBox.Mem (b : PtBox) (p : Pt) : Prop := b.x.Mem p.x ∧ b.y.Mem p.y

/-! ### Distances -/

/-- Model of the `HorizontalDistance` evaluator of `sketch.certDrivenOf`: `isub(e.px(P2), e.px(P1))`. -/
@[expose] noncomputable def drivenHorizontal (b1 b2 : PtBox) : Sketch.Interval.Ival := isub M b2.x b1.x

/-- Model of the `VerticalDistance` evaluator of `sketch.certDrivenOf`: `isub(e.py(P2), e.py(P1))`. -/
@[expose] noncomputable def drivenVertical (b1 b2 : PtBox) : Sketch.Interval.Ival := isub M b2.y b1.y

/-- Model of the `Distance` evaluator of `sketch.certDrivenOf`: the interval square root of the sum of the
squared coordinate differences. -/
@[expose] noncomputable def drivenDistance (b1 b2 : PtBox) : Sketch.Interval.Ival :=
  isqrtNonNeg M (iadd M (isqr M (isub M b1.x b2.x)) (isqr M (isub M b1.y b2.y)))

/-- **Driven horizontal distance.** For every pair of points in their boxes, the `HorizontalDistance` evaluator
of `sketch.certDrivenOf` contains the measured value `x2 − x1`. Covers `sketch.certDrivenOf` and `sketch.isub`.
Assumptions A1 to A3. -/
theorem drivenHorizontal_sound {b1 b2 : PtBox} {p1 p2 : Pt} (h1 : b1.Mem p1) (h2 : b2.Mem p2) :
    (drivenHorizontal M b1 b2).Mem (p2.x - p1.x) :=
  isub_sound M h2.1 h1.1

/-- **Driven vertical distance.** For every pair of points in their boxes, the `VerticalDistance` evaluator of
`sketch.certDrivenOf` contains the measured value `y2 − y1`. Covers `sketch.certDrivenOf` and `sketch.isub`.
Assumptions A1 to A3. -/
theorem drivenVertical_sound {b1 b2 : PtBox} {p1 p2 : Pt} (h1 : b1.Mem p1) (h2 : b2.Mem p2) :
    (drivenVertical M b1 b2).Mem (p2.y - p1.y) :=
  isub_sound M h2.2 h1.2

/-- **Driven point-to-point distance.** For every pair of points in their boxes, the `Distance` evaluator of
`sketch.certDrivenOf` contains the measured value `dist(P1, P2)`. Covers `sketch.certDrivenOf`, `sketch.isub`,
`sketch.isqr`, `sketch.iadd` and `sketch.isqrtNonNeg`. Assumptions A1 to A6, A40 (`math.Hypot`). -/
theorem drivenDistance_sound {b1 b2 : PtBox} {p1 p2 : Pt} (h1 : b1.Mem p1) (h2 : b2.Mem p2) :
    (drivenDistance M b1 b2).Mem (dist p1 p2) := by
  have := isqrtNonNeg_sound M (iadd_sound M (isqr_sound M (isub_sound M h1.1 h2.1))
    (isqr_sound M (isub_sound M h1.2 h2.2)))
    (add_nonneg (mul_self_nonneg (p1.x - p2.x)) (mul_self_nonneg (p1.y - p2.y)))
  unfold dist
  rw [show (p1.x - p2.x) ^ 2 + (p1.y - p2.y) ^ 2 = (p1.x - p2.x) * (p1.x - p2.x) + (p1.y - p2.y) * (p1.y - p2.y)
    by ring]
  exact this

/-! ### Angles -/

/-- Model of the `Angle` evaluator of `sketch.certDrivenOf`: the direction vectors of `sketch.certAngle.parts`,
their cross and dot product intervals, and `B` (`sketch.atan2Box`) over the rectangle `cross × dot`. The boxes
are those of the two lines' `Start` and `End` points. -/
@[expose] noncomputable def drivenAngle
    (B : Sketch.Interval.Ival → Sketch.Interval.Ival → Option Sketch.Interval.Ival) (s1 e1 s2 e2 : PtBox) :
    Option Sketch.Interval.Ival :=
  let ux := isub M e1.x s1.x
  let uy := isub M e1.y s1.y
  let vx := isub M e2.x s2.x
  let vy := isub M e2.y s2.y
  B (isub M (imul M ux vy) (imul M uy vx)) (iadd M (imul M ux vx) (imul M uy vy))

/-- **Driven angle.** Whenever the `Angle` evaluator of `sketch.certDrivenOf` returns an interval, it contains,
for every configuration of the two lines whose points lie in their boxes, a value equal to the signed angle from
`l1` to `l2` modulo `2π`. `hB` is the soundness of the rectangle enclosure (`atan2Box_sound`). Covers
`sketch.certDrivenOf` and its interval helpers. Assumptions: A1 to A6, A40. -/
theorem drivenAngle_sound
    (B : Sketch.Interval.Ival → Sketch.Interval.Ival → Option Sketch.Interval.Ival)
    (hB : ∀ c d out cv dv, B c d = some out → c.Mem cv → d.Mem dv → ∃ θ, out.Mem θ ∧ Rep θ dv cv)
    {s1 e1 s2 e2 : PtBox} {l1 l2 : Line} (hs1 : s1.Mem ⟨l1.sx, l1.sy⟩) (he1 : e1.Mem ⟨l1.ex, l1.ey⟩)
    (hs2 : s2.Mem ⟨l2.sx, l2.sy⟩) (he2 : e2.Mem ⟨l2.ex, l2.ey⟩) {I : Sketch.Interval.Ival}
    (h : drivenAngle M B s1 e1 s2 e2 = some I) : ∃ θ, I.Mem θ ∧ (θ : Real.Angle) = signedAngle l1 l2 := by
  have hux := isub_sound M he1.1 hs1.1
  have huy := isub_sound M he1.2 hs1.2
  have hvx := isub_sound M he2.1 hs2.1
  have hvy := isub_sound M he2.2 hs2.2
  have hcross := isub_sound M (imul_sound M hux hvy) (imul_sound M huy hvx)
  have hdot := iadd_sound M (imul_sound M hux hvx) (imul_sound M huy hvy)
  obtain ⟨θ, hθ, hrep⟩ := hB _ _ _ _ _ h hcross hdot
  refine ⟨θ, hθ, ?_⟩
  rw [rep_eq_arg hrep]
  rfl

/-- **Driven angle after the whole-turn shift.** `sketch.encloseRun.piece` passes a driven angle's interval
through `sketch.shiftNear`; the result still contains a value equal to the signed angle modulo `2π` for every
configuration in the box, whatever the float decision `k`. Covers `sketch.encloseRun.piece`,
`sketch.certDrivenOf` and `sketch.shiftNear`. Assumptions: A1 to A6, A20, A22, A24, A28, A40, A70. -/
theorem drivenAngle_piece_sound (R : Sketch.Trig.Rounding)
    (B : Sketch.Interval.Ival → Sketch.Interval.Ival → Option Sketch.Interval.Ival)
    (hB : ∀ c d out cv dv, B c d = some out → c.Mem cv → d.Mem dv → ∃ θ, out.Mem θ ∧ Rep θ dv cv)
    {s1 e1 s2 e2 : PtBox} {l1 l2 : Line} (hs1 : s1.Mem ⟨l1.sx, l1.sy⟩) (he1 : e1.Mem ⟨l1.ex, l1.ey⟩)
    (hs2 : s2.Mem ⟨l2.sx, l2.sy⟩) (he2 : e2.Mem ⟨l2.ex, l2.ey⟩) {I : Sketch.Interval.Ival}
    (h : drivenAngle M B s1 e1 s2 e2 = some I) (k : Option ℤ) :
    ∃ θ, (Sketch.Trig.shiftNear M R I k).Mem θ ∧ (θ : Real.Angle) = signedAngle l1 l2 := by
  obtain ⟨θ, hθ, harg⟩ := drivenAngle_sound M B hB hs1 he1 hs2 he2 h
  obtain ⟨j, hj⟩ := Sketch.Trig.shiftNear_sound M R I k
  refine ⟨θ + j * (2 * π), hj θ hθ, ?_⟩
  rw [← harg, Real.Angle.coe_add, show (j : ℝ) * (2 * π) = (j : ℤ) • (2 * π) by rw [zsmul_eq_mul],
    Real.Angle.coe_zsmul, Real.Angle.coe_two_pi, smul_zero, add_zero]

/-- **Driven angle on binary64.** The `Angle` evaluator of `sketch.certDrivenOf` with `sketch.atan2Box` over
`sketch.atan2Point` and the certified `sketch.sinCosPoint`, followed by `sketch.shiftNear` in
`sketch.encloseRun.piece`: the piece's interval contains a value equal to the signed angle modulo `2π` for every
configuration in the box. `hD` is assumption A72. Covers `sketch.certDrivenOf`, `sketch.encloseRun.piece` and
every function they reach in `interval_trig.go` and `interval.go`. Assumptions: A1 to A6, A20 to A28, A40,
A70 to A72. -/
theorem drivenAngle_sound_binary64 (E : Sketch.Trig.TrigEnv) (fuel : ℕ) (aenv : Sketch.Trig.Atan2Env)
    (envOf : Sketch.Interval.Ival → Sketch.Interval.Ival → Sketch.Trig.BoxEnv)
    (hD : Sketch.Trig.BoxDecisions M (Sketch.Trig.Rounding.ofFloat M)
      (Sketch.Trig.atan2Point M (Sketch.Trig.sinCosPoint (Sketch.Trig.Rounding.ofFloat M) E fuel) aenv) envOf)
    {s1 e1 s2 e2 : PtBox} {l1 l2 : Line} (hs1 : s1.Mem ⟨l1.sx, l1.sy⟩) (he1 : e1.Mem ⟨l1.ex, l1.ey⟩)
    (hs2 : s2.Mem ⟨l2.sx, l2.sy⟩) (he2 : e2.Mem ⟨l2.ex, l2.ey⟩) {I : Sketch.Interval.Ival}
    (h : drivenAngle M (Sketch.Trig.atan2BoxOf M (Sketch.Trig.Rounding.ofFloat M)
      (Sketch.Trig.atan2Point M (Sketch.Trig.sinCosPoint (Sketch.Trig.Rounding.ofFloat M) E fuel) aenv) envOf)
      s1 e1 s2 e2 = some I) (k : Option ℤ) :
    ∃ θ, (Sketch.Trig.shiftNear M (Sketch.Trig.Rounding.ofFloat M) I k).Mem θ ∧
      (θ : Real.Angle) = signedAngle l1 l2 :=
  drivenAngle_piece_sound M _ _
    (Sketch.Trig.atan2BoxOf_sound M _ _
      (fun c0 d0 _ hI => Sketch.Trig.atan2Point_sound_binary64 M E fuel aenv c0 d0 hI) envOf hD)
    hs1 he1 hs2 he2 h k

end Sketch.Enclose

end
