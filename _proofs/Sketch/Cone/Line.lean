module

public import Sketch.Cone.Interval
public import Sketch.Cone.Poly

/-!
# Cone side of a line

Models `sketch.coneSideForm.lineSide` (`cone_side.go`). The segment from `(sx, sy)` to `(ex, ey)` is
`x(t) = sx + (ex − sx)·t`, `y(t) = sy + (ey − sy)·t` on `t ∈ [0, 1]`. `lineSide` builds the cone residual
polynomials `h(t)` (degree 1) and `F(t)` (degree 2) with `coneFitResidual`, refuses unless the Bernstein lower
bound of `h` on `[0, 1]` is positive, and bounds `F` on `[0, 1]` by its two end values and, when the vertex
`−F₁/(2·F₂)` lies strictly inside `(0, 1)`, its vertex value.

`lineRange_bounds` proves a quadratic on `[0, 1]` lies between that minimum and maximum: its extremes are at an
endpoint or at its vertex. `lineSide_sound` proves a returned side holds, with the returned positive margin, at
every point of the segment.
-/

public section

namespace Sketch.Cone

/-- The bounds `lineSide` computes for the residual quadratic `fp` on `[0, 1]`: the end values, widened by the
vertex value when `fp[2] ≠ 0` and the vertex `−fp[1]/(2·fp[2])` lies in `(0, 1)`. -/
@[expose] def lineRange (fp : List ℚ) : ℚ × ℚ :=
  let f0 := coneFitPolyEval fp 0
  let f1 := coneFitPolyEval fp 1
  let lo := (coneIntervalOf f0 f1).lo
  let hi := (coneIntervalOf f0 f1).hi
  if fp.getD 2 0 ≠ 0 then
    let vertex := -fp.getD 1 0 / (2 * fp.getD 2 0)
    if 0 < vertex ∧ vertex < 1 then
      let value := coneFitPolyEval fp vertex
      (if value < lo then value else lo, if hi < value then value else hi)
    else (lo, hi)
  else (lo, hi)

namespace ConeSideForm

/-- Model of `sketch.coneSideForm.lineSide` on the endpoint coordinates it reads, which are the exact values
of the Go floats (A31). The Go finiteness check is that premise; the coincident-endpoint refusal is kept. -/
@[expose] def lineSide (f : ConeSideForm) (sx sy ex ey : ℚ) : ConeSide × Option ℚ :=
  if sx = ex ∧ sy = ey then (.zero, none)
  else
    let px := [sx, ex - sx]
    let py := [sy, ey - sy]
    if (coneFitCoeffBounds (coneFitResidualH f px py) 0 1).1 ≤ 0 then (.zero, none)
    else
      let r := lineRange (coneFitResidualF f px py)
      if 0 < r.1 then (.outside, some r.1)
      else if r.2 < 0 then (.inside, some (-r.2))
      else (.zero, none)

end ConeSideForm

theorem coneIntervalOf_lo (a b : ℚ) : (coneIntervalOf a b).lo = min a b := by
  unfold coneIntervalOf; split_ifs with h
  · exact (min_eq_left h).symm
  · exact (min_eq_right (le_of_not_ge h)).symm

theorem coneIntervalOf_hi (a b : ℚ) : (coneIntervalOf a b).hi = max a b := by
  unfold coneIntervalOf; split_ifs with h
  · exact (max_eq_right h).symm
  · exact (max_eq_left (le_of_not_ge h)).symm

theorem coneFitPolyEval_linear (a b t : ℚ) : coneFitPolyEval [a, b] t = a + b * t := by
  simp only [coneFitPolyEval_cons, coneFitPolyEval_nil]; ring

/-- The quadratic `c0 + c1·s + c2·s²`. -/
@[expose] def quad (c0 c1 c2 s : ℚ) : ℚ := c0 + c1 * s + c2 * s * s

theorem coneFitPolyEval_quadratic (c0 c1 c2 t : ℚ) :
    coneFitPolyEval [c0, c1, c2] t = quad c0 c1 c2 t := by
  simp only [coneFitPolyEval_cons, coneFitPolyEval_nil, quad]; ring

theorem getD_three_one (c0 c1 c2 : ℚ) : [c0, c1, c2].getD 1 0 = c1 := rfl

theorem getD_three_two (c0 c1 c2 : ℚ) : [c0, c1, c2].getD 2 0 = c2 := rfl

theorem lineRange_three (c0 c1 c2 : ℚ) :
    lineRange [c0, c1, c2] =
      if c2 ≠ 0 then
        (if 0 < -c1 / (2 * c2) ∧ -c1 / (2 * c2) < 1 then
          (min (quad c0 c1 c2 (-c1 / (2 * c2))) (min (quad c0 c1 c2 0) (quad c0 c1 c2 1)),
            max (max (quad c0 c1 c2 0) (quad c0 c1 c2 1)) (quad c0 c1 c2 (-c1 / (2 * c2))))
        else (min (quad c0 c1 c2 0) (quad c0 c1 c2 1), max (quad c0 c1 c2 0) (quad c0 c1 c2 1)))
      else (min (quad c0 c1 c2 0) (quad c0 c1 c2 1), max (quad c0 c1 c2 0) (quad c0 c1 c2 1)) := by
  simp only [lineRange, coneFitPolyEval_quadratic, coneIntervalOf_lo, coneIntervalOf_hi, getD_three_one,
    getD_three_two, ConeInterval.ite_lt_eq_min, ConeInterval.ite_gt_eq_max]

/-- A quadratic `c0 + c1·t + c2·t²` on `[0, 1]` lies between the two bounds `lineRange` returns: its minimum
and maximum on `[0, 1]` are at an endpoint or at its vertex. -/
theorem lineRange_bounds (c0 c1 c2 t : ℚ) (ht0 : 0 ≤ t) (ht1 : t ≤ 1) :
    (lineRange [c0, c1, c2]).1 ≤ coneFitPolyEval [c0, c1, c2] t ∧
      coneFitPolyEval [c0, c1, c2] t ≤ (lineRange [c0, c1, c2]).2 := by
  rw [lineRange_three, coneFitPolyEval_quadratic]
  set g := quad c0 c1 c2 with hg
  have hs : 0 ≤ 1 - t := by linarith
  -- The chord over [0, 1] minus the curve is c2·t·(1−t).
  have chord : (1 - t) * g 0 + t * g 1 - g t = c2 * (t * (1 - t)) := by simp only [hg, quad]; ring
  have htt : 0 ≤ t * (1 - t) := mul_nonneg ht0 hs
  have hmax : (1 - t) * g 0 + t * g 1 ≤ max (g 0) (g 1) := by
    have := mul_nonneg hs (sub_nonneg.2 (le_max_left (g 0) (g 1)))
    have := mul_nonneg ht0 (sub_nonneg.2 (le_max_right (g 0) (g 1)))
    nlinarith
  have hmin : min (g 0) (g 1) ≤ (1 - t) * g 0 + t * g 1 := by
    have := mul_nonneg hs (sub_nonneg.2 (min_le_left (g 0) (g 1)))
    have := mul_nonneg ht0 (sub_nonneg.2 (min_le_right (g 0) (g 1)))
    nlinarith
  have cvx : 0 ≤ c2 → g t ≤ max (g 0) (g 1) := fun h => by nlinarith [mul_nonneg h htt]
  have ccv : c2 ≤ 0 → min (g 0) (g 1) ≤ g t := fun h => by nlinarith [mul_nonpos_of_nonpos_of_nonneg h htt]
  by_cases h2 : c2 = 0
  · simp only [ne_eq, h2, not_true_eq_false, ↓reduceIte]
    exact ⟨ccv h2.le, cvx h2.ge⟩
  simp only [ne_eq, h2, not_false_eq_true, ↓reduceIte]
  set v := -c1 / (2 * c2) with hv
  have hv2 : c1 = -(2 * c2 * v) := by rw [hv]; field_simp
  have vert : g t - g v = c2 * (t - v) ^ 2 := by simp only [hg, quad]; rw [hv2]; ring
  have e0 : g t - g 0 = c2 * (t * (t - 2 * v)) := by simp only [hg, quad]; rw [hv2]; ring
  have e1 : g t - g 1 = c2 * ((1 - t) * (2 * v - 1 - t)) := by simp only [hg, quad]; rw [hv2]; ring
  have sq := sq_nonneg (t - v)
  rcases lt_or_gt_of_ne h2 with hneg | hpos
  · split_ifs with hin
    · refine ⟨le_trans (min_le_right _ _) (ccv hneg.le), ?_⟩
      have : g t ≤ g v := by nlinarith [mul_nonpos_of_nonpos_of_nonneg hneg.le sq]
      exact le_trans this (le_max_right _ _)
    · refine ⟨ccv hneg.le, ?_⟩
      rcases not_and_or.mp hin with h0 | h1
      · have := mul_nonneg ht0 (show 0 ≤ t - 2 * v by linarith [not_lt.mp h0])
        have : g t ≤ g 0 := by nlinarith [mul_nonpos_of_nonpos_of_nonneg hneg.le this]
        exact le_trans this (le_max_left _ _)
      · have := mul_nonneg hs (show 0 ≤ 2 * v - 1 - t by linarith [not_lt.mp h1])
        have : g t ≤ g 1 := by nlinarith [mul_nonpos_of_nonpos_of_nonneg hneg.le this]
        exact le_trans this (le_max_right _ _)
  · split_ifs with hin
    · refine ⟨?_, le_trans (cvx hpos.le) (le_max_left _ _)⟩
      have : g v ≤ g t := by nlinarith [mul_nonneg hpos.le sq]
      exact le_trans (min_le_left _ _) this
    · refine ⟨?_, cvx hpos.le⟩
      rcases not_and_or.mp hin with h0 | h1
      · have := mul_nonneg ht0 (show 0 ≤ t - 2 * v by linarith [not_lt.mp h0])
        have : g 0 ≤ g t := by nlinarith [mul_nonneg hpos.le this]
        exact le_trans (min_le_left _ _) this
      · have := mul_nonneg hs (show 0 ≤ 2 * v - 1 - t by linarith [not_lt.mp h1])
        have : g 1 ≤ g t := by nlinarith [mul_nonneg hpos.le this]
        exact le_trans (min_le_right _ _) this

theorem coneFitResidualH_linear (f : ConeSideForm) (a b c d : ℚ) :
    ∃ h0 h1, coneFitResidualH f [a, b] [c, d] = [h0, h1] := ⟨_, _, rfl⟩

theorem coneFitResidualF_linear (f : ConeSideForm) (a b c d : ℚ) :
    ∃ c0 c1 c2, coneFitResidualF f [a, b] [c, d] = [c0, c1, c2] := ⟨_, _, _, rfl⟩

namespace ConeSideForm

/-- A side `lineSide` returns holds at every point `(sx + (ex − sx)·t, sy + (ey − sy)·t)`, `t ∈ [0, 1]`, of
the segment, with the returned margin, which is positive. Covers `sketch.coneSideForm.lineSide` and the
`coneFitPoly` helpers it calls (`coneFitResidual`, `coneFitCoeffBounds`, `coneFitBernstein`,
`coneFitBinomial`, `coneFitPolyEval` and the arithmetic helpers); assumptions A30, A31, A32. -/
theorem lineSide_sound {f : ConeSideForm} {sx sy ex ey : ℚ} {side : ConeSide} {m : ℚ}
    (hres : f.lineSide sx sy ex ey = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧
      ∀ t, 0 ≤ t → t ≤ 1 → f.SideAt side m (sx + (ex - sx) * t) (sy + (ey - sy) * t) := by
  obtain ⟨h0, h1, hH⟩ := coneFitResidualH_linear f sx (ex - sx) sy (ey - sy)
  obtain ⟨c0, c1, c2, hF⟩ := coneFitResidualF_linear f sx (ex - sx) sy (ey - sy)
  have hpt : ∀ t, f.h (sx + (ex - sx) * t) (sy + (ey - sy) * t) = coneFitPolyEval [h0, h1] t ∧
      f.F (sx + (ex - sx) * t) (sy + (ey - sy) * t) = coneFitPolyEval [c0, c1, c2] t := by
    intro t
    rw [← hH, ← hF, coneFitResidual_h, coneFitResidual_F, coneFitPolyEval_linear, coneFitPolyEval_linear]
    exact ⟨rfl, rfl⟩
  simp only [lineSide, hH, hF] at hres
  split_ifs at hres with hab hlo hpos hneg
  · simp at hres
  · simp at hres
  · simp only [Prod.mk.injEq, Option.some.injEq] at hres
    obtain ⟨rfl, rfl⟩ := hres
    refine ⟨hpos, by decide, fun t ht0 ht1 => ⟨?_, ?_⟩⟩
    · rw [(hpt t).1]; linarith [coneFitCoeffBounds_linear h0 h1 t ht0 ht1]
    · rw [(hpt t).2]; exact (lineRange_bounds c0 c1 c2 t ht0 ht1).1
  · simp only [Prod.mk.injEq, Option.some.injEq] at hres
    obtain ⟨rfl, rfl⟩ := hres
    refine ⟨by linarith, by decide, fun t ht0 ht1 => ⟨?_, ?_⟩⟩
    · rw [(hpt t).1]; linarith [coneFitCoeffBounds_linear h0 h1 t ht0 ht1]
    · rw [(hpt t).2, neg_neg]; exact (lineRange_bounds c0 c1 c2 t ht0 ht1).2
  · simp at hres

end ConeSideForm

end Sketch.Cone

end
