module

public import Sketch.Cone.Box

/-!
# Cone side at real points

The arc paths of `sketch.Sketch.CertifyConeCurveSide` certify points of a circle, whose coordinates are
real, not rational. This file restates the cone form of `Sketch.Cone.Form` and the box test of
`Sketch.Cone.Box` (`sketch.coneSideForm.boxSide`) at real points: `ConeInterval.MemR` is membership of a
real in a rational interval, the `coneInterval` operations stay inclusion-sound for reals, and
`boxSide_sound_real` proves that a side `boxSide` returns holds at every real point of the box.

`sideAtR_newConeSideForm` carries a side at a real plane point to the world point `o + x·U + y·V`, for the
cone implicit function `worldFR`, which agrees with `worldF` at rational points (`worldFR_ratCast`).
-/

public section

namespace Sketch.Cone

namespace ConeInterval

/-- The real `x` lies in `[a.lo, a.hi]`. -/
@[expose] def MemR (a : ConeInterval) (x : ℝ) : Prop := (a.lo : ℝ) ≤ x ∧ x ≤ a.hi

/-- A one-point interval `{x, x}` holds `x`. -/
theorem memR_point (x : ℚ) : (⟨x, x⟩ : ConeInterval).MemR x := ⟨le_refl _, le_refl _⟩

/-- Every real between the two bounds given to `coneIntervalOf`, in either order, lies in its result. -/
theorem memR_coneIntervalOf_between {a b : ℚ} {x : ℝ}
    (h : ((a : ℝ) ≤ x ∧ x ≤ b) ∨ ((b : ℝ) ≤ x ∧ x ≤ a)) : (coneIntervalOf a b).MemR x := by
  unfold coneIntervalOf MemR
  split_ifs with hab
  · have hab' : (a : ℝ) ≤ b := by exact_mod_cast hab
    rcases h with ⟨h1, h2⟩ | ⟨h1, h2⟩ <;> constructor <;> linarith
  · have hab' : (b : ℝ) ≤ a := by exact_mod_cast (le_of_not_ge hab)
    rcases h with ⟨h1, h2⟩ | ⟨h1, h2⟩ <;> constructor <;> linarith

/-- `add` is inclusion-sound for reals. -/
theorem memR_add {a b : ConeInterval} {x y : ℝ} (hx : a.MemR x) (hy : b.MemR y) :
    (a.add b).MemR (x + y) := by
  obtain ⟨h1, h2⟩ := hx
  obtain ⟨h3, h4⟩ := hy
  simp only [add, MemR, Rat.cast_add]
  constructor <;> linarith

/-- `sub` is inclusion-sound for reals. -/
theorem memR_sub {a b : ConeInterval} {x y : ℝ} (hx : a.MemR x) (hy : b.MemR y) :
    (a.sub b).MemR (x - y) := by
  obtain ⟨h1, h2⟩ := hx
  obtain ⟨h3, h4⟩ := hy
  simp only [sub, MemR, Rat.cast_sub]
  constructor <;> linarith

/-- `scale` is inclusion-sound for reals and every factor sign. -/
theorem memR_scale {a : ConeInterval} {x : ℝ} (k : ℚ) (hx : a.MemR x) : (a.scale k).MemR (x * k) := by
  obtain ⟨h1, h2⟩ := hx
  unfold scale
  apply memR_coneIntervalOf_between
  push_cast
  rcases le_total 0 (k : ℝ) with hk | hk
  · exact Or.inl ⟨mul_le_mul_of_nonneg_right h1 hk, mul_le_mul_of_nonneg_right h2 hk⟩
  · exact Or.inr ⟨mul_le_mul_of_nonpos_right h2 hk, mul_le_mul_of_nonpos_right h1 hk⟩

/-- `square` is inclusion-sound for reals. -/
theorem memR_square {a : ConeInterval} {x : ℝ} (hx : a.MemR x) : a.square.MemR (x * x) := by
  obtain ⟨h1, h2⟩ := hx
  unfold square MemR
  split_ifs with ha hb hc
  · have ha' : (0 : ℝ) ≤ a.lo := by exact_mod_cast ha
    push_cast
    exact ⟨by nlinarith, by nlinarith⟩
  · have ha' : (a.lo : ℝ) < 0 := by exact_mod_cast (lt_of_not_ge ha)
    have hb' : (a.hi : ℝ) ≤ 0 := by exact_mod_cast hb
    push_cast
    exact ⟨by nlinarith, by nlinarith⟩
  · have hc' : (a.lo : ℝ) * a.lo < a.hi * a.hi := by exact_mod_cast hc
    push_cast
    refine ⟨mul_self_nonneg x, ?_⟩
    rcases le_total 0 x with h0 | h0 <;> nlinarith
  · have hc' : (a.hi : ℝ) * a.hi ≤ a.lo * a.lo := by exact_mod_cast (le_of_not_gt hc)
    push_cast
    refine ⟨mul_self_nonneg x, ?_⟩
    rcases le_total 0 x with h0 | h0 <;> nlinarith

end ConeInterval

namespace ConeSideForm

/-- World offset from the apex, component `i`, of the real plane point `(x, y)`. -/
@[expose] noncomputable def qR (f : ConeSideForm) (x y : ℝ) (i : Fin 3) : ℝ :=
  (f.base i : ℝ) + x * f.u i + y * f.v i

/-- Cone half coordinate at the real plane point `(x, y)`. -/
@[expose] noncomputable def hR (f : ConeSideForm) (x y : ℝ) : ℝ :=
  f.qR x y 0 * f.axis 0 + f.qR x y 1 * f.axis 1 + f.qR x y 2 * f.axis 2

/-- Cone implicit function `|q×a|² − slope²·h²` at the real plane point `(x, y)`. -/
@[expose] noncomputable def FR (f : ConeSideForm) (x y : ℝ) : ℝ :=
  (f.qR x y 1 * f.axis 2 - f.qR x y 2 * f.axis 1) * (f.qR x y 1 * f.axis 2 - f.qR x y 2 * f.axis 1) +
    (f.qR x y 2 * f.axis 0 - f.qR x y 0 * f.axis 2) * (f.qR x y 2 * f.axis 0 - f.qR x y 0 * f.axis 2) +
    (f.qR x y 0 * f.axis 1 - f.qR x y 1 * f.axis 0) * (f.qR x y 0 * f.axis 1 - f.qR x y 1 * f.axis 0) -
    f.hR x y * f.hR x y * f.slope2

/-- `SideAt` at a real plane point. -/
@[expose] def SideAtR (f : ConeSideForm) (side : ConeSide) (m : ℚ) (x y : ℝ) : Prop :=
  0 < f.hR x y ∧
    match side with
    | .outside => (m : ℝ) ≤ f.FR x y
    | .inside => f.FR x y ≤ -(m : ℝ)
    | .zero => False

/-- A smaller margin keeps a side at a real point. -/
theorem sideAtR_mono {f : ConeSideForm} {side : ConeSide} {m m' : ℚ} {x y : ℝ} (hm : m' ≤ m)
    (hs : f.SideAtR side m x y) : f.SideAtR side m' x y := by
  obtain ⟨hh, hs⟩ := hs
  have hm' : (m' : ℝ) ≤ m := by exact_mod_cast hm
  refine ⟨hh, ?_⟩
  cases side with
  | zero => exact hs
  | outside => exact le_trans hm' hs
  | inside => simp only at hs ⊢; linarith

theorem memR_boxQ {f : ConeSideForm} {u v : ConeInterval} {x y : ℝ} (hx : u.MemR x) (hy : v.MemR y)
    (i : Fin 3) : (f.boxQ u v i).MemR (f.qR x y i) :=
  ConeInterval.memR_add (ConeInterval.memR_add (ConeInterval.memR_point _) (ConeInterval.memR_scale _ hx))
    (ConeInterval.memR_scale _ hy)

theorem memR_boxH {f : ConeSideForm} {u v : ConeInterval} {x y : ℝ} (hx : u.MemR x) (hy : v.MemR y) :
    (f.boxH u v).MemR (f.hR x y) :=
  ConeInterval.memR_add (ConeInterval.memR_add (ConeInterval.memR_scale _ (memR_boxQ hx hy 0))
    (ConeInterval.memR_scale _ (memR_boxQ hx hy 1))) (ConeInterval.memR_scale _ (memR_boxQ hx hy 2))

theorem memR_boxResidual {f : ConeSideForm} {u v : ConeInterval} {x y : ℝ} (hx : u.MemR x)
    (hy : v.MemR y) : (f.boxResidual u v).MemR (f.FR x y) := by
  have q := memR_boxQ (f := f) hx hy
  have hcx := ConeInterval.memR_square
    (ConeInterval.memR_sub (ConeInterval.memR_scale (f.axis 2) (q 1)) (ConeInterval.memR_scale (f.axis 1) (q 2)))
  have hcy := ConeInterval.memR_square
    (ConeInterval.memR_sub (ConeInterval.memR_scale (f.axis 0) (q 2)) (ConeInterval.memR_scale (f.axis 2) (q 0)))
  have hcz := ConeInterval.memR_square
    (ConeInterval.memR_sub (ConeInterval.memR_scale (f.axis 1) (q 0)) (ConeInterval.memR_scale (f.axis 0) (q 1)))
  have hh := ConeInterval.memR_scale f.slope2 (ConeInterval.memR_square (memR_boxH (f := f) hx hy))
  exact ConeInterval.memR_sub (ConeInterval.memR_add (ConeInterval.memR_add hcx hcy) hcz) hh

/-- A side `boxSide` returns holds at every real point of the box `u × v`, with the returned margin, which
is positive. The real form of `boxSide_sound`. Covers `sketch.coneSideForm.boxSide` and the `coneInterval`
operations; exact rational arithmetic is A30. -/
theorem boxSide_sound_real {f : ConeSideForm} {u v : ConeInterval} {side : ConeSide} {m : ℚ}
    (hres : f.boxSide u v = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧ ∀ x y : ℝ, u.MemR x → v.MemR y → f.SideAtR side m x y := by
  unfold boxSide at hres
  split_ifs at hres with h0 h1 h2
  · simp at hres
  · simp only [Prod.mk.injEq, Option.some.injEq] at hres
    obtain ⟨rfl, rfl⟩ := hres
    have h0' : (0 : ℝ) < (f.boxH u v).lo := by exact_mod_cast (lt_of_not_ge h0)
    refine ⟨h1, by decide, fun x y hx hy => ⟨?_, ?_⟩⟩
    · linarith [(memR_boxH (f := f) hx hy).1]
    · exact (memR_boxResidual (f := f) hx hy).1
  · simp only [Prod.mk.injEq, Option.some.injEq] at hres
    obtain ⟨rfl, rfl⟩ := hres
    have h0' : (0 : ℝ) < (f.boxH u v).lo := by exact_mod_cast (lt_of_not_ge h0)
    refine ⟨by linarith, by decide, fun x y hx hy => ⟨?_, ?_⟩⟩
    · linarith [(memR_boxH (f := f) hx hy).1]
    · change f.FR x y ≤ -((-(f.boxResidual u v).hi : ℚ) : ℝ)
      push_cast
      rw [neg_neg]
      exact (memR_boxResidual (f := f) hx hy).2
  · simp at hres

end ConeSideForm

/-- The world point `o + x·U + y·V` of the real plane point `(x, y)`. -/
@[expose] noncomputable def worldPointR (o U V : Fin 3 → ℚ) (x y : ℝ) : Fin 3 → ℝ :=
  fun i => (o i : ℝ) + x * U i + y * V i

/-- The cone implicit function `|(P−A)×a|² − s²·((P−A)·a)²` at the real world point `P`. -/
@[expose] noncomputable def worldFR (A a : Fin 3 → ℚ) (s : ℚ) (P : Fin 3 → ℝ) : ℝ :=
  (((P 1 - A 1) * a 2 - (P 2 - A 2) * a 1) ^ 2 + ((P 2 - A 2) * a 0 - (P 0 - A 0) * a 2) ^ 2 +
      ((P 0 - A 0) * a 1 - (P 1 - A 1) * a 0) ^ 2) -
    (s : ℝ) ^ 2 * ((P 0 - A 0) * a 0 + (P 1 - A 1) * a 1 + (P 2 - A 2) * a 2) ^ 2

/-- The cone half coordinate `(P−A)·a` at the real world point `P`. -/
@[expose] noncomputable def worldHR (A a : Fin 3 → ℚ) (P : Fin 3 → ℝ) : ℝ :=
  (P 0 - A 0) * a 0 + (P 1 - A 1) * a 1 + (P 2 - A 2) * a 2

/-- At a rational world point, `worldFR` is the exact `worldF`. -/
theorem worldFR_ratCast (A a : Fin 3 → ℚ) (s : ℚ) (P : Fin 3 → ℚ) :
    worldFR A a s (fun i => (P i : ℝ)) = (worldF A a s P : ℝ) := by
  simp only [worldFR, worldF, dot]
  push_cast
  ring

theorem newConeSideForm_FR (o U V A a : Fin 3 → ℚ) (s : ℚ) (x y : ℝ) :
    (newConeSideForm o U V A a s).FR x y = worldFR A a s (worldPointR o U V x y) := by
  simp only [ConeSideForm.FR, ConeSideForm.hR, ConeSideForm.qR, newConeSideForm, worldFR, worldPointR]
  push_cast
  ring

theorem newConeSideForm_hR (o U V A a : Fin 3 → ℚ) (s : ℚ) (x y : ℝ) :
    (newConeSideForm o U V A a s).hR x y = worldHR A a (worldPointR o U V x y) := by
  simp only [ConeSideForm.hR, ConeSideForm.qR, newConeSideForm, worldHR, worldPointR]
  push_cast
  ring

/-- At a rational point, `worldHR` is `worldH`. -/
theorem worldHR_cast (A a : Fin 3 → ℚ) (P : Fin 3 → ℚ) :
    worldHR A a (fun i => (P i : ℝ)) = worldH A a P := by
  simp only [worldHR, worldH, dot]
  push_cast
  ring

/-- Side `side` holds with margin `m` at the real world point `P`, as `WorldSideAt` states it at rational
points. -/
@[expose] def WorldSideAtR (A a : Fin 3 → ℚ) (s : ℚ) (side : ConeSide) (m : ℚ) (P : Fin 3 → ℝ) : Prop :=
  0 < worldHR A a P ∧
    match side with
    | .outside => (m : ℝ) ≤ worldFR A a s P
    | .inside => worldFR A a s P ≤ -(m : ℝ)
    | .zero => False

/-- The plane form at a real plane point evaluates the world cone function at its world point. -/
theorem sideAtR_newConeSideForm {o U V A a : Fin 3 → ℚ} {s : ℚ} {side : ConeSide} {m : ℚ} {x y : ℝ}
    (h : (newConeSideForm o U V A a s).SideAtR side m x y) :
    WorldSideAtR A a s side m (worldPointR o U V x y) := by
  have hF := newConeSideForm_FR o U V A a s x y
  have hH := newConeSideForm_hR o U V A a s x y
  unfold ConeSideForm.SideAtR at h
  unfold WorldSideAtR
  rw [← hF, ← hH]
  exact h

end Sketch.Cone

end
