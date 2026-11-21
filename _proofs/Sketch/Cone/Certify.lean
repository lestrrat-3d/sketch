module

public import Sketch.Cone.Arc
public import Sketch.Cone.Fit
public import Sketch.Cone.Line

/-!
# Whole-curve cone side certificate

Models `sketch.Sketch.CertifyConeCurveSide` (`cone_side.go`). The Go function screens its inputs (sketch
ownership, finite cone data, positive slope, nonzero axis, a readable plane frame, a buildable interpolant);
each screen only refuses, so the model omits them. It then builds the exact form with `newConeSideForm`,
dispatches on the curve kind, and refuses unless the side is nonzero and the margin is present and positive.

`certifyConeCurveSide_line_sound` and `certifyConeCurveSide_fit_sound` state the published guarantee in world
coordinates: every point of the curve, mapped to the world through the plane frame, lies in the positive cone
half and on the certified side, with `|F| ≥ margin` for the cone implicit function `F` of `worldF`.

`certifyConeCurveSide_arc_sound` states what an arc certificate proves: the same guarantee on the exact arc
when the wedge proof produced it, or on the circle of radius `Arc.R()` through the rounded endpoint rays when
the quadrant proof produced it. `certifyConeCurveSide_arc_wedge_sound` is the exact-arc guarantee whenever
the quadrant proof refuses.
-/

public section

namespace Sketch.Cone

/-- The curve sources: a line by its endpoint coordinates, a fitted spline by its interpolant, or an arc by its
data together with the float operations its path uses. -/
inductive ConeCurve where
  | line (sx sy ex ey : ℚ)
  | fit (fi : FitData)
  | arc (E : ConeArcEnv) (d : ConeArcData)

/-- World point `o + x·U + y·V` of plane point `(x, y)`. -/
@[expose] def worldPoint (o U V : Fin 3 → ℚ) (x y : ℚ) : Fin 3 → ℚ := fun i => o i + x * U i + y * V i

/-- Side `side` holds with margin `m` at world point `P` for the cone with apex `A`, axis `a` and slope `s`. -/
@[expose] def WorldSideAt (A a : Fin 3 → ℚ) (s : ℚ) (side : ConeSide) (m : ℚ) (P : Fin 3 → ℚ) : Prop :=
  0 < worldH A a P ∧
    match side with
    | .outside => m ≤ worldF A a s P
    | .inside => worldF A a s P ≤ -m
    | .zero => False

theorem sideAt_newConeSideForm {o U V A a : Fin 3 → ℚ} {s : ℚ} {side : ConeSide} {m x y : ℚ}
    (h : (newConeSideForm o U V A a s).SideAt side m x y) :
    WorldSideAt A a s side m (worldPoint o U V x y) := by
  unfold ConeSideForm.SideAt at h
  unfold WorldSideAt worldPoint
  rw [← newConeSideForm_h o U V A a s x y, ← newConeSideForm_F o U V A a s x y]
  exact h

/-- The side and margin the curve-kind dispatch in `sketch.Sketch.CertifyConeCurveSide` computes. -/
@[expose] def curveSide (form : ConeSideForm) : ConeCurve → ConeSide × Option ℚ
  | .line sx sy ex ey => form.lineSide sx sy ex ey
  | .fit fi => form.fitSide fi
  | .arc E d => form.arcSide E d

/-- Model of `sketch.Sketch.CertifyConeCurveSide`. The frame origin `o`,
plane axes `U`, `V`, cone apex `A`, axis `a` and slope `s` are the exact values of the Go floats (A31). The
result is `some (side, margin)` exactly when the Go function returns a certificate. -/
@[expose] def certifyConeCurveSide (o U V A a : Fin 3 → ℚ) (s : ℚ) (curve : ConeCurve) :
    Option (ConeSide × ℚ) :=
  match curveSide (newConeSideForm o U V A a s) curve with
  | (side, some m) => if side = .zero ∨ m ≤ 0 then none else some (side, m)
  | (_, none) => none

theorem certifyConeCurveSide_some {o U V A a : Fin 3 → ℚ} {s : ℚ} {curve : ConeCurve} {side : ConeSide}
    {m : ℚ} (h : certifyConeCurveSide o U V A a s curve = some (side, m)) :
    curveSide (newConeSideForm o U V A a s) curve = (side, some m) := by
  unfold certifyConeCurveSide at h
  generalize curveSide (newConeSideForm o U V A a s) curve = r at h ⊢
  obtain ⟨side', m'⟩ := r
  cases m' with
  | none => simp at h
  | some m' =>
    simp only at h
    split_ifs at h
    simp only [Option.some.injEq, Prod.mk.injEq] at h
    obtain ⟨rfl, rfl⟩ := h
    rfl

/-- A certificate for the line from `(sx, sy)` to `(ex, ey)` proves every point of the segment, in world
coordinates, lies in the positive cone half and strictly on the certified side with `|F| ≥ margin > 0`.
Covers `sketch.Sketch.CertifyConeCurveSide`, `sketch.newConeSideForm`, `sketch.coneSideForm.lineSide` and the
helpers `lineSide_sound` lists; assumptions A30, A31, A32. -/
theorem certifyConeCurveSide_line_sound {o U V A a : Fin 3 → ℚ} {s sx sy ex ey : ℚ} {side : ConeSide} {m : ℚ}
    (h : certifyConeCurveSide o U V A a s (.line sx sy ex ey) = some (side, m)) :
    0 < m ∧ side ≠ .zero ∧ ∀ t, 0 ≤ t → t ≤ 1 →
      WorldSideAt A a s side m (worldPoint o U V (sx + (ex - sx) * t) (sy + (ey - sy) * t)) := by
  obtain ⟨hm, hs, hall⟩ := ConeSideForm.lineSide_sound (certifyConeCurveSide_some h)
  exact ⟨hm, hs, fun t ht0 ht1 => sideAt_newConeSideForm (hall t ht0 ht1)⟩

/-- A certificate for a fitted spline proves every point of every span of its ideal interpolant, in world
coordinates, lies in the positive cone half and strictly on the certified side with `|F| ≥ margin > 0`.
Covers `sketch.Sketch.CertifyConeCurveSide`, `sketch.newConeSideForm`, `sketch.coneSideForm.fitSide` and the
functions `fitSide_sound` lists; assumptions A30, A31, A33. -/
theorem certifyConeCurveSide_fit_sound {o U V A a : Fin 3 → ℚ} {s : ℚ} {fi : FitData} {side : ConeSide}
    {m : ℚ} (h : certifyConeCurveSide o U V A a s (.fit fi) = some (side, m)) :
    0 < m ∧ side ≠ .zero ∧ ∀ i, i + 1 < fi.points.length → ∀ u, 0 ≤ u → u ≤ 1 →
      WorldSideAt A a s side m (worldPoint o U V (fi.spanPoint i u 0) (fi.spanPoint i u 1)) := by
  obtain ⟨hm, hs, hall⟩ := ConeSideForm.fitSide_sound (certifyConeCurveSide_some h)
  exact ⟨hm, hs, fun i hi u hu0 hu1 => sideAt_newConeSideForm (hall i hi u hu0 hu1)⟩

/-- Side `side` holds with margin `m`, in world coordinates, at every point of the circle of radius `ρ`
about plane point `(cx, cy)` whose direction lies in the closed wedge from `a` to `b`. -/
@[expose] def WorldArcSideAt (o U V A a : Fin 3 → ℚ) (s : ℚ) (side : ConeSide) (m cx cy : ℚ) (ρ : ℝ)
    (wa wb : ℝ × ℝ) : Prop :=
  ∀ w : ℝ × ℝ, w ≠ 0 → InWedge wa wb w →
    WorldSideAtR A a s side m (worldPointR o U V (circlePoint cx cy ρ w).1 (circlePoint cx cy ρ w).2)

theorem worldArcSideAt_of {o U V A a : Fin 3 → ℚ} {s : ℚ} {side : ConeSide} {m cx cy : ℚ} {ρ : ℝ}
    {wa wb : ℝ × ℝ} (h : (newConeSideForm o U V A a s).ArcSideAt side m cx cy ρ wa wb) :
    WorldArcSideAt o U V A a s side m cx cy ρ wa wb :=
  fun w hw hin => sideAtR_newConeSideForm (h w hw hin)

/-- A certificate for an arc proves, in world coordinates, that the points of one circle piece lie in the
positive cone half and strictly on the certified side with `|F| ≥ margin > 0`. When the quadrant proof
(`arcQuadrantSide`) produced it, the piece is the circle of radius `d.r` (the float `Arc.R()`) from the ray
`(d.dsx, d.dsy)` to the ray `(d.dex, d.dey)` (the float differences `Start − Center`, `End − Center`); this is
not the exact arc. Otherwise the piece is the exact arc: radius `|Start − Center|`, from the Start ray
counter-clockwise to the End ray, with a sweep strictly between zero and a half turn. Covers
`sketch.Sketch.CertifyConeCurveSide`, `sketch.newConeSideForm` and the functions `arcSide_sound` lists;
assumptions A30, A31, A50, A51, A52. -/
theorem certifyConeCurveSide_arc_sound {o U V A a : Fin 3 → ℚ} {s : ℚ} {E : ConeArcEnv} {d : ConeArcData}
    {side : ConeSide} {m : ℚ} (h : certifyConeCurveSide o U V A a s (.arc E d) = some (side, m)) :
    0 < m ∧ side ≠ .zero ∧
      (((newConeSideForm o U V A a s).arcQuadrant d).1 ≠ .zero ∧
          WorldArcSideAt o U V A a s side m d.cx d.cy d.r (rayR (d.dsx, d.dsy)) (rayR (d.dex, d.dey)) ∨
        ((newConeSideForm o U V A a s).arcQuadrant d).1 = .zero ∧
          0 < coneCross (d.sx - d.cx, d.sy - d.cy) (d.ex - d.cx, d.ey - d.cy) ∧
          WorldArcSideAt o U V A a s side m d.cx d.cy (normR (rayR (d.sx - d.cx, d.sy - d.cy)))
            (rayR (d.sx - d.cx, d.sy - d.cy)) (rayR (d.ex - d.cx, d.ey - d.cy))) := by
  obtain ⟨hm, hs, hor⟩ := ConeSideForm.arcSide_sound (certifyConeCurveSide_some h)
  refine ⟨hm, hs, ?_⟩
  rcases hor with ⟨hq, hall⟩ | ⟨hq, hc, hall⟩
  · exact Or.inl ⟨hq, worldArcSideAt_of hall⟩
  · exact Or.inr ⟨hq, hc, worldArcSideAt_of hall⟩

/-- When the quadrant proof refuses, an arc certificate proves that every point of the exact arc (radius
`|Start − Center|`, from the Start ray counter-clockwise to the End ray, sweep strictly between zero and a
half turn), in world coordinates, lies in the positive cone half and strictly on the certified side with
`|F| ≥ margin > 0`. Covers `sketch.Sketch.CertifyConeCurveSide`, `sketch.newConeSideForm` and the functions
`arcWedgeSide_sound` lists; assumptions A30, A31, A50, A51. -/
theorem certifyConeCurveSide_arc_wedge_sound {o U V A a : Fin 3 → ℚ} {s : ℚ} {E : ConeArcEnv}
    {d : ConeArcData} {side : ConeSide} {m : ℚ}
    (hq : ((newConeSideForm o U V A a s).arcQuadrant d).1 = .zero)
    (h : certifyConeCurveSide o U V A a s (.arc E d) = some (side, m)) :
    0 < m ∧ side ≠ .zero ∧ 0 < coneCross (d.sx - d.cx, d.sy - d.cy) (d.ex - d.cx, d.ey - d.cy) ∧
      WorldArcSideAt o U V A a s side m d.cx d.cy (normR (rayR (d.sx - d.cx, d.sy - d.cy)))
        (rayR (d.sx - d.cx, d.sy - d.cy)) (rayR (d.ex - d.cx, d.ey - d.cy)) := by
  obtain ⟨hm, hs, hor⟩ := certifyConeCurveSide_arc_sound h
  rcases hor with ⟨hq', -⟩ | ⟨-, hc, hall⟩
  · exact absurd hq hq'
  · exact ⟨hm, hs, hc, hall⟩

end Sketch.Cone

end
