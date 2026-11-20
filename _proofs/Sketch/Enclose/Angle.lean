module

public import Mathlib

/-!
# Zero sets of the certified angle and distance equations

`sketch.Sketch.Enclose` does not read the solver's residuals. It restates each constraint as an
equation with the same zero set inside the tested box (`sketch.certEquationsOf`). This file proves that
rule for the two restatements that are not plain products. The signed angle residual
`sketch.Angle.residual`, `wrapPi(atan2(cross, dot) − θ)`, is replaced by `sketch.certAngle.eval`,
`cross·cos θ − dot·sin θ`, together with the side condition `sketch.certAngle.side`,
`cross·sin θ + dot·cos θ > 0`. The point-to-point distance residual `sketch.Distance.residual`,
`|p₁ − p₂| − d`, is replaced by `sketch.certDistance.eval`, `|p₁ − p₂|² − d²`, for `d ≥ 0`.

Everything is modeled over `ℝ` (assumption A40 in `.claude/docs/proofs.md`): float arithmetic is exact,
`math.Atan2(y, x)` is `Complex.arg (x + y·i)`, `math.Hypot` is the square root of the sum of squares, and
`math.Mod` is the truncated remainder. The Go equations evaluate interval enclosures of these values
(A41); the models here are the exact values those intervals enclose.
-/

public section

namespace Sketch.Enclose

open Real

/-- A sketch point's solved coordinates. -/
structure Pt where
  /-- The x coordinate. -/
  x : ℝ
  /-- The y coordinate. -/
  y : ℝ

/-- A sketch line, by the solved coordinates of its `Start` and `End` points. -/
structure Line where
  /-- `Start`'s x coordinate. -/
  sx : ℝ
  /-- `Start`'s y coordinate. -/
  sy : ℝ
  /-- `End`'s x coordinate. -/
  ex : ℝ
  /-- `End`'s y coordinate. -/
  ey : ℝ

/-! ## The standard-library functions, as assumption A40 states them -/

/-- `math.Atan2(y, x)` over the reals: the argument of `x + y·i`, in `(−π, π]`, with `atan2 0 0 = 0`.
Signed zeros are not modeled. -/
@[expose] noncomputable def atan2 (y x : ℝ) : ℝ :=
  Complex.arg ⟨x, y⟩

/-- Truncation toward zero, the integer quotient `math.Mod` uses. -/
@[expose] noncomputable def goTrunc (x : ℝ) : ℤ :=
  if 0 ≤ x then ⌊x⌋ else ⌈x⌉

/-- `math.Mod(x, y)` over the reals: `x − y·trunc(x / y)`. -/
@[expose] noncomputable def goMod (x y : ℝ) : ℝ :=
  x - y * goTrunc (x / y)

/-! ## The solver's residuals -/

/-- Model of `sketch.wrapPi`. The Go constant `2*math.Pi` is `2π`. -/
@[expose] noncomputable def wrapPi (a : ℝ) : ℝ :=
  let m := goMod a (2 * π)
  if m > π then m - 2 * π else if m ≤ -π then m + 2 * π else m

/-- Model of `sketch.dir`: a line's `End − Start`. -/
@[expose] def dir (l : Line) : ℝ × ℝ :=
  (l.ex - l.sx, l.ey - l.sy)

/-- The cross product `ux·vy − uy·vx`, which the Go functions compute inline. -/
@[expose] def cross (ux uy vx vy : ℝ) : ℝ :=
  ux * vy - uy * vx

/-- The dot product `ux·vx + uy·vy`, which the Go functions compute inline. -/
@[expose] def dot (ux uy vx vy : ℝ) : ℝ :=
  ux * vx + uy * vy

/-- The signed angle from `l1`'s direction to `l2`'s, `atan2(cross, dot)`, as `sketch.Angle.residual`
measures it. -/
@[expose] noncomputable def signedAngle (l1 l2 : Line) : ℝ :=
  atan2 (cross (dir l1).1 (dir l1).2 (dir l2).1 (dir l2).2)
    (dot (dir l1).1 (dir l1).2 (dir l2).1 (dir l2).2)

/-- Model of `sketch.Angle.residual`: `wrapPi(atan2(cross, dot) − θ)`, with `θ` the target in radians. -/
@[expose] noncomputable def angleResidual (l1 l2 : Line) (θ : ℝ) : ℝ :=
  wrapPi (signedAngle l1 l2 - θ)

/-- Model of `sketch.dist`: `math.Hypot` of the coordinate differences. -/
@[expose] noncomputable def dist (p q : Pt) : ℝ :=
  √((p.x - q.x) ^ 2 + (p.y - q.y) ^ 2)

/-- Model of `sketch.Distance.residual`: `dist(P1, P2) − d`, with `d` the target in base units. -/
@[expose] noncomputable def distanceResidual (p1 p2 : Pt) (d : ℝ) : ℝ :=
  dist p1 p2 - d

/-! ## The certified equations -/

/-- Model of `sketch.certAngle.parts`: the two lines' `End − Start` vectors `(ux, uy, vx, vy)`. -/
@[expose] def certAngleParts (l1 l2 : Line) : ℝ × ℝ × ℝ × ℝ :=
  (l1.ex - l1.sx, l1.ey - l1.sy, l2.ex - l2.sx, l2.ey - l2.sy)

/-- Model of `sketch.certAngle.trig`: `(sin θ, cos θ)` for the angle `θ` the row reads. The Go method
returns enclosures of these values (A41). -/
@[expose] noncomputable def certAngleTrig (θ : ℝ) : ℝ × ℝ :=
  (Real.sin θ, Real.cos θ)

/-- Model of `sketch.certAngle.eval`: `cross·cos θ − dot·sin θ`. -/
@[expose] noncomputable def certAngleEval (l1 l2 : Line) (θ : ℝ) : ℝ :=
  let p := certAngleParts l1 l2
  let t := certAngleTrig θ
  cross p.1 p.2.1 p.2.2.1 p.2.2.2 * t.2 - dot p.1 p.2.1 p.2.2.1 p.2.2.2 * t.1

/-- The value `sketch.certAngle.side` tests: `cross·sin θ + dot·cos θ`. -/
@[expose] noncomputable def certAngleSideValue (l1 l2 : Line) (θ : ℝ) : ℝ :=
  let p := certAngleParts l1 l2
  let t := certAngleTrig θ
  cross p.1 p.2.1 p.2.2.1 p.2.2.2 * t.1 + dot p.1 p.2.1 p.2.2.1 p.2.2.2 * t.2

/-- Model of `sketch.certAngle.side` at one configuration: the side value is positive. The Go method
returns true only when the lower end of an enclosure of the side value over the whole box is positive
(A41), so it holds at every configuration of the box. -/
@[expose] noncomputable def certAngleSide (l1 l2 : Line) (θ : ℝ) : Prop :=
  0 < certAngleSideValue l1 l2 θ

/-- Model of `sketch.certDistance.eval`: `|P1 − P2|² − d²`, with `d` the target or driving value. -/
@[expose] def certDistanceEval (p1 p2 : Pt) (d : ℝ) : ℝ :=
  (p1.x - p2.x) ^ 2 + (p1.y - p2.y) ^ 2 - d ^ 2

/-! ## `wrapPi` -/

private lemma goTrunc_intCast (k : ℤ) : goTrunc (k : ℝ) = k := by
  unfold goTrunc
  split_ifs <;> simp

/-- `sketch.wrapPi` returns 0 exactly at the whole turns: `wrapPi a = 0` if and only if `a = 2π·k` for
an integer `k`. Covers `sketch.wrapPi` under A40. -/
theorem wrapPi_eq_zero_iff (a : ℝ) : wrapPi a = 0 ↔ ∃ k : ℤ, a = 2 * π * k := by
  have hπ := Real.pi_pos
  constructor
  · intro h
    set t : ℤ := goTrunc (a / (2 * π))
    have hm : goMod a (2 * π) = a - 2 * π * t := rfl
    simp only [wrapPi, hm] at h
    by_cases h1 : a - 2 * π * t > π
    · rw [ite_eq_left h1] at h
      exact ⟨t + 1, by push_cast; linarith⟩
    rw [ite_eq_right h1] at h
    by_cases h2 : a - 2 * π * t ≤ -π
    · rw [ite_eq_left h2] at h
      exact ⟨t - 1, by push_cast; linarith⟩
    rw [ite_eq_right h2] at h
    exact ⟨t, by linarith⟩
  · rintro ⟨k, rfl⟩
    have hdiv : 2 * π * (k : ℝ) / (2 * π) = k := by field_simp
    have hm : goMod (2 * π * k) (2 * π) = 0 := by
      simp only [goMod, hdiv, goTrunc_intCast]
      ring
    simp only [wrapPi, hm]
    split_ifs <;> linarith

/-! ## The signed angle -/

/-- The angle residual is zero exactly when the signed angle from `l1` to `l2` equals `θ` modulo `2π`.
Covers `sketch.Angle.residual`, `sketch.wrapPi` and `sketch.dir` under A40. -/
theorem angleResidual_eq_zero_iff (l1 l2 : Line) (θ : ℝ) :
    angleResidual l1 l2 θ = 0 ↔ (signedAngle l1 l2 : Real.Angle) = θ := by
  rw [angleResidual, wrapPi_eq_zero_iff, Real.Angle.angle_eq_iff_two_pi_dvd_sub]

/-- The core identity in the complex plane: for `z = dot + cross·i`, the equation
`cross·cos θ − dot·sin θ = 0` with `cross·sin θ + dot·cos θ > 0` holds exactly when `z ≠ 0` and
`arg z = θ` modulo `2π`. -/
theorem restated_angle_iff (c d θ : ℝ) :
    (c * Real.cos θ - d * Real.sin θ = 0 ∧ 0 < c * Real.sin θ + d * Real.cos θ) ↔
      ((⟨d, c⟩ : ℂ) ≠ 0 ∧ (Complex.arg ⟨d, c⟩ : Real.Angle) = θ) := by
  have hsc := Real.sin_sq_add_cos_sq θ
  constructor
  · rintro ⟨heq, hpos⟩
    set r := c * Real.sin θ + d * Real.cos θ with hr
    have hd : d = r * Real.cos θ := by linear_combination (-Real.sin θ) * heq - d * hsc
    have hc : c = r * Real.sin θ := by linear_combination Real.cos θ * heq - c * hsc
    have hz : (⟨d, c⟩ : ℂ) = (r : ℂ) * ((Real.cos θ : ℂ) + (Real.sin θ : ℂ) * Complex.I) := by
      apply Complex.ext
      · simp only [Complex.mul_re, Complex.mul_im, Complex.add_re, Complex.add_im, Complex.ofReal_re,
          Complex.ofReal_im, Complex.I_re, Complex.I_im]
        rw [hd]
        ring
      · simp only [Complex.mul_re, Complex.mul_im, Complex.add_re, Complex.add_im, Complex.ofReal_re,
          Complex.ofReal_im, Complex.I_re, Complex.I_im]
        rw [hc]
        ring
    refine ⟨?_, ?_⟩
    · intro h0
      have h1 : d = 0 := congrArg Complex.re h0
      have h2 : c = 0 := congrArg Complex.im h0
      rw [h1, h2] at hr
      linarith
    · have h := Complex.arg_mul_cos_add_sin_mul_I_coe_angle hpos (θ : Real.Angle)
      rw [Real.Angle.cos_coe, Real.Angle.sin_coe] at h
      rw [hz]
      exact h
  · rintro ⟨hz, harg⟩
    have hcos : Real.cos θ = d / ‖(⟨d, c⟩ : ℂ)‖ := by
      have := congrArg Real.Angle.cos harg
      rw [Real.Angle.cos_coe, Real.Angle.cos_coe, Complex.cos_arg hz] at this
      exact this.symm
    have hsin : Real.sin θ = c / ‖(⟨d, c⟩ : ℂ)‖ := by
      have := congrArg Real.Angle.sin harg
      rw [Real.Angle.sin_coe, Real.Angle.sin_coe, Complex.sin_arg] at this
      exact this.symm
    have hn : 0 < ‖(⟨d, c⟩ : ℂ)‖ := norm_pos_iff.mpr hz
    have hsq : ‖(⟨d, c⟩ : ℂ)‖ ^ 2 = d ^ 2 + c ^ 2 := by
      rw [Complex.sq_norm, Complex.normSq_mk]
      ring
    rw [hcos, hsin]
    refine ⟨by ring, ?_⟩
    have : c * (c / ‖(⟨d, c⟩ : ℂ)‖) + d * (d / ‖(⟨d, c⟩ : ℂ)‖) = ‖(⟨d, c⟩ : ℂ)‖ := by
      field_simp
      linarith
    rw [this]
    exact hn

/-- `dot + cross·i` is nonzero exactly when both vectors are nonzero, by Lagrange's identity
`dot² + cross² = |u|²·|v|²`. -/
theorem dotCross_ne_zero_iff (ux uy vx vy : ℝ) :
    (⟨dot ux uy vx vy, cross ux uy vx vy⟩ : ℂ) ≠ 0 ↔ (ux, uy) ≠ 0 ∧ (vx, vy) ≠ 0 := by
  have key : dot ux uy vx vy ^ 2 + cross ux uy vx vy ^ 2 = (ux ^ 2 + uy ^ 2) * (vx ^ 2 + vy ^ 2) := by
    simp only [dot, cross]
    ring
  have hz : (⟨dot ux uy vx vy, cross ux uy vx vy⟩ : ℂ) = 0 ↔
      dot ux uy vx vy = 0 ∧ cross ux uy vx vy = 0 := by
    simp [Complex.ext_iff]
  have hsum : ∀ a b : ℝ, a ^ 2 + b ^ 2 = 0 ↔ a = 0 ∧ b = 0 := by
    intro a b
    constructor
    · intro h
      exact ⟨by nlinarith [sq_nonneg a, sq_nonneg b], by nlinarith [sq_nonneg a, sq_nonneg b]⟩
    · rintro ⟨rfl, rfl⟩
      ring
  have hdc : dot ux uy vx vy = 0 ∧ cross ux uy vx vy = 0 ↔
      dot ux uy vx vy ^ 2 + cross ux uy vx vy ^ 2 = 0 := (hsum _ _).symm
  rw [Ne, hz, hdc, key, mul_eq_zero, hsum, hsum, Ne, Ne, Prod.mk_eq_zero, Prod.mk_eq_zero]
  tauto

/-- The certified angle row and its side condition hold together exactly when both lines have nonzero
length and the signed angle from `l1` to `l2` equals `θ` modulo `2π`. The side condition selects the `θ`
branch: it never certifies the antipodal `θ + π`. Covers `sketch.certAngle.eval`,
`sketch.certAngle.side`, `sketch.certAngle.parts`, `sketch.certAngle.trig` and `sketch.dir` under A40
and A41. -/
theorem certAngle_iff (l1 l2 : Line) (θ : ℝ) :
    (certAngleEval l1 l2 θ = 0 ∧ certAngleSide l1 l2 θ) ↔
      (dir l1 ≠ 0 ∧ dir l2 ≠ 0 ∧ (signedAngle l1 l2 : Real.Angle) = θ) := by
  have h := restated_angle_iff (cross (dir l1).1 (dir l1).2 (dir l2).1 (dir l2).2)
    (dot (dir l1).1 (dir l1).2 (dir l2).1 (dir l2).2) θ
  rw [dotCross_ne_zero_iff] at h
  simpa only [certAngleEval, certAngleSide, certAngleSideValue, certAngleParts, certAngleTrig,
    signedAngle, atan2, dir, and_assoc] using h

/-- The SAME zero set rule for the angle: wherever the side condition holds, the certified angle row is
zero exactly when the solver's angle residual is. `sketch.certAngle.side` checks the condition over the
whole tested box, so the two zero sets agree inside it. Covers `sketch.certAngle.eval`,
`sketch.certAngle.side`, `sketch.certAngle.parts`, `sketch.certAngle.trig`, `sketch.Angle.residual`,
`sketch.wrapPi` and `sketch.dir`; hypothesis `hside` is the side condition, which A41 carries from the
Go check to every configuration of the box. -/
theorem certAngle_zero_iff_angleResidual_zero (l1 l2 : Line) (θ : ℝ)
    (hside : certAngleSide l1 l2 θ) :
    certAngleEval l1 l2 θ = 0 ↔ angleResidual l1 l2 θ = 0 := by
  rw [angleResidual_eq_zero_iff]
  constructor
  · intro h0
    exact ((certAngle_iff l1 l2 θ).mp ⟨h0, hside⟩).2.2
  · intro harg
    have hz : dir l1 ≠ 0 ∧ dir l2 ≠ 0 := by
      have hne : (⟨dot (dir l1).1 (dir l1).2 (dir l2).1 (dir l2).2,
          cross (dir l1).1 (dir l1).2 (dir l2).1 (dir l2).2⟩ : ℂ) ≠ 0 := by
        intro h0
        have h1 : dot (dir l1).1 (dir l1).2 (dir l2).1 (dir l2).2 = 0 := congrArg Complex.re h0
        have h2 : cross (dir l1).1 (dir l1).2 (dir l2).1 (dir l2).2 = 0 := congrArg Complex.im h0
        simp only [certAngleSide, certAngleSideValue, certAngleParts, certAngleTrig] at hside
        simp only [dir] at h1 h2
        rw [h1, h2] at hside
        simp at hside
      rw [dotCross_ne_zero_iff] at hne
      simpa only [Prod.mk.eta] using hne
    exact ((certAngle_iff l1 l2 θ).mpr ⟨hz.1, hz.2, harg⟩).1

/-- Inside a box on which the side condition holds everywhere, the certified angle row and the solver's
angle residual have the same zero set. Covers the same Go functions as
`certAngle_zero_iff_angleResidual_zero`; hypothesis `hbox` is A41. -/
theorem certAngle_zero_set_eq_on_box (B : Set (Line × Line × ℝ))
    (hbox : ∀ x ∈ B, certAngleSide x.1 x.2.1 x.2.2) :
    {x ∈ B | certAngleEval x.1 x.2.1 x.2.2 = 0} = {x ∈ B | angleResidual x.1 x.2.1 x.2.2 = 0} := by
  ext x
  simp only [Set.mem_sep_iff]
  constructor
  · rintro ⟨hx, h⟩
    exact ⟨hx, (certAngle_zero_iff_angleResidual_zero _ _ _ (hbox x hx)).mp h⟩
  · rintro ⟨hx, h⟩
    exact ⟨hx, (certAngle_zero_iff_angleResidual_zero _ _ _ (hbox x hx)).mpr h⟩

/-- Without the side condition the row alone does not pick a branch: it vanishes at `θ + π` whenever it
vanishes at `θ`, and the side value changes sign. This is why `sketch.certAngle.side` is needed. -/
theorem certAngle_add_pi (l1 l2 : Line) (θ : ℝ) :
    certAngleEval l1 l2 (θ + π) = -certAngleEval l1 l2 θ ∧
      certAngleSideValue l1 l2 (θ + π) = -certAngleSideValue l1 l2 θ := by
  simp only [certAngleEval, certAngleSideValue, certAngleTrig, Real.sin_add_pi, Real.cos_add_pi]
  constructor <;> ring

/-! ## The point-to-point distance -/

/-- The SAME zero set rule for the distance: for a target `d ≥ 0`, the squared row
`|P1 − P2|² − d²` is zero exactly when the solver's residual `dist(P1, P2) − d` is. Covers
`sketch.certDistance.eval`, `sketch.Distance.residual` and `sketch.dist`; hypothesis `hd` is A42. -/
theorem certDistance_zero_iff_distanceResidual_zero (p1 p2 : Pt) (d : ℝ) (hd : 0 ≤ d) :
    certDistanceEval p1 p2 d = 0 ↔ distanceResidual p1 p2 d = 0 := by
  have hS : 0 ≤ (p1.x - p2.x) ^ 2 + (p1.y - p2.y) ^ 2 := by positivity
  rw [certDistanceEval, distanceResidual, dist, sub_eq_zero, sub_eq_zero, Real.sqrt_eq_iff_eq_sq hS hd]

/-- Without `d ≥ 0` the rule fails: a negative target makes the solver's residual nonzero everywhere,
while the squared row reads `d` and `−d` alike. This is why `sketch.certEquationsOf` and
`sketch.rangeBase` refuse a distance target that is not positive. -/
theorem certDistance_neg_target (p1 p2 : Pt) (d : ℝ) (hd : d < 0) :
    distanceResidual p1 p2 d ≠ 0 ∧ certDistanceEval p1 p2 d = certDistanceEval p1 p2 (-d) := by
  refine ⟨?_, by simp only [certDistanceEval]; ring⟩
  rw [distanceResidual, dist]
  have := Real.sqrt_nonneg ((p1.x - p2.x) ^ 2 + (p1.y - p2.y) ^ 2)
  intro h
  linarith

end Sketch.Enclose

end
