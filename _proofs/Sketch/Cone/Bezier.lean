module

public import Sketch.Cone.Box

/-!
# Cone side of a cubic Bézier curve

Models `sketch.coneBezier`, `coneBezier.box`, `coneBezier.halves` and `sketch.coneSideForm.bezierSide`
(`cone_side.go`). A `coneBezier` holds four plane control points; its curve is the cubic Bézier curve
`B(t) = (1−t)³·P₀ + 3t(1−t)²·P₁ + 3t²(1−t)·P₂ + t³·P₃` on `t ∈ [0, 1]`.

`box_mem` is the convex-hull property: the box `box` returns holds every curve point. `halves_left` and
`halves_right` prove de Casteljau subdivision at `t = 1/2` exact: the two halves trace `B` on `[0, 1/2]` and
`[1/2, 1]`. `bezierSide_sound` proves a side `bezierSide` returns holds, with the returned positive margin, at
every curve point.
-/

public section

namespace Sketch.Cone

/-- Model of `sketch.coneBezier`: control point `i`, coordinate `axis`. -/
structure ConeBezier where
  controls : Fin 4 → Fin 2 → ℚ

namespace ConeBezier

/-- Coordinate `axis` of the cubic Bézier curve at parameter `t`. -/
@[expose] def eval (b : ConeBezier) (t : ℚ) (axis : Fin 2) : ℚ :=
  (1 - t) ^ 3 * b.controls 0 axis + 3 * t * (1 - t) ^ 2 * b.controls 1 axis +
    3 * t ^ 2 * (1 - t) * b.controls 2 axis + t ^ 3 * b.controls 3 axis

/-- Model of one axis of `coneBezier.box`: start at control 0, then lower `lo` on `p < lo` and raise `hi` on
`p > hi` for controls 1 to 3. -/
@[expose] def boxAxis (b : ConeBezier) (axis : Fin 2) : ConeInterval :=
  [b.controls 1 axis, b.controls 2 axis, b.controls 3 axis].foldl
    (fun acc p => ⟨if p < acc.lo then p else acc.lo, if acc.hi < p then p else acc.hi⟩)
    ⟨b.controls 0 axis, b.controls 0 axis⟩

/-- Model of `coneBezier.box`. -/
@[expose] def box (b : ConeBezier) : ConeInterval × ConeInterval := (b.boxAxis 0, b.boxAxis 1)

/-- Model of `coneBezier.halves`: de Casteljau subdivision at `t = 1/2`, per axis. -/
@[expose] def halves (b : ConeBezier) : ConeBezier × ConeBezier :=
  let half : ℚ := 1 / 2
  let p := b.controls
  let q0 := fun a => (p 0 a + p 1 a) * half
  let q1 := fun a => (p 1 a + p 2 a) * half
  let q2 := fun a => (p 2 a + p 3 a) * half
  let r0 := fun a => (q0 a + q1 a) * half
  let r1 := fun a => (q1 a + q2 a) * half
  let mid := fun a => (r0 a + r1 a) * half
  (⟨fun i a => ![p 0 a, q0 a, r0 a, mid a] i⟩, ⟨fun i a => ![mid a, r1 a, q2 a, p 3 a] i⟩)

/-- Convex-hull property: every curve point on `[0, 1]` lies in the box of the control points. -/
theorem boxAxis_mem (b : ConeBezier) (axis : Fin 2) {t : ℚ} (ht0 : 0 ≤ t) (ht1 : t ≤ 1) :
    (b.boxAxis axis).Mem (b.eval t axis) := by
  simp only [boxAxis, List.foldl_cons, List.foldl_nil, ConeInterval.ite_lt_eq_min, ConeInterval.ite_gt_eq_max,
    ConeInterval.Mem, eval]
  set c0 := b.controls 0 axis
  set c1 := b.controls 1 axis
  set c2 := b.controls 2 axis
  set c3 := b.controls 3 axis
  set lo := min c3 (min c2 (min c1 c0))
  set hi := max (max (max c0 c1) c2) c3
  have l0 : lo ≤ c0 := le_trans (min_le_right _ _) (le_trans (min_le_right _ _) (min_le_right _ _))
  have l1 : lo ≤ c1 := le_trans (min_le_right _ _) (le_trans (min_le_right _ _) (min_le_left _ _))
  have l2 : lo ≤ c2 := le_trans (min_le_right _ _) (min_le_left _ _)
  have l3 : lo ≤ c3 := min_le_left _ _
  have u0 : c0 ≤ hi := le_trans (le_max_left _ _) (le_trans (le_max_left _ _) (le_max_left _ _))
  have u1 : c1 ≤ hi := le_trans (le_max_right _ _) (le_trans (le_max_left _ _) (le_max_left _ _))
  have u2 : c2 ≤ hi := le_trans (le_max_right _ _) (le_max_left _ _)
  have u3 : c3 ≤ hi := le_max_right _ _
  have s : 0 ≤ 1 - t := by linarith
  have w0 : 0 ≤ (1 - t) ^ 3 := pow_nonneg s 3
  have w1 : 0 ≤ 3 * t * (1 - t) ^ 2 := by positivity
  have w2 : 0 ≤ 3 * t ^ 2 * (1 - t) := by positivity
  have w3 : 0 ≤ t ^ 3 := pow_nonneg ht0 3
  have klo : (1 - t) ^ 3 * c0 + 3 * t * (1 - t) ^ 2 * c1 + 3 * t ^ 2 * (1 - t) * c2 + t ^ 3 * c3 - lo =
      (1 - t) ^ 3 * (c0 - lo) + 3 * t * (1 - t) ^ 2 * (c1 - lo) + 3 * t ^ 2 * (1 - t) * (c2 - lo) +
        t ^ 3 * (c3 - lo) := by ring
  have khi : hi - ((1 - t) ^ 3 * c0 + 3 * t * (1 - t) ^ 2 * c1 + 3 * t ^ 2 * (1 - t) * c2 + t ^ 3 * c3) =
      (1 - t) ^ 3 * (hi - c0) + 3 * t * (1 - t) ^ 2 * (hi - c1) + 3 * t ^ 2 * (1 - t) * (hi - c2) +
        t ^ 3 * (hi - c3) := by ring
  constructor
  · have := add_nonneg (add_nonneg (add_nonneg (mul_nonneg w0 (sub_nonneg.2 l0)) (mul_nonneg w1 (sub_nonneg.2 l1)))
      (mul_nonneg w2 (sub_nonneg.2 l2))) (mul_nonneg w3 (sub_nonneg.2 l3))
    linarith
  · have := add_nonneg (add_nonneg (add_nonneg (mul_nonneg w0 (sub_nonneg.2 u0)) (mul_nonneg w1 (sub_nonneg.2 u1)))
      (mul_nonneg w2 (sub_nonneg.2 u2))) (mul_nonneg w3 (sub_nonneg.2 u3))
    linarith

/-- The box `box` returns holds the curve point at every `t ∈ [0, 1]`. -/
theorem box_mem (b : ConeBezier) {t : ℚ} (ht0 : 0 ≤ t) (ht1 : t ≤ 1) :
    b.box.1.Mem (b.eval t 0) ∧ b.box.2.Mem (b.eval t 1) :=
  ⟨b.boxAxis_mem 0 ht0 ht1, b.boxAxis_mem 1 ht0 ht1⟩

/-- The left half traces the curve on `[0, 1/2]`: its point at `s` is the curve point at `s/2`. -/
theorem halves_left (b : ConeBezier) (s : ℚ) (axis : Fin 2) :
    b.halves.1.eval s axis = b.eval (s / 2) axis := by
  simp only [halves, eval, Matrix.cons_val_zero, Matrix.cons_val_one, Matrix.cons_val_two,
    Matrix.cons_val_three, Matrix.head_cons, Matrix.tail_cons]
  ring

/-- The right half traces the curve on `[1/2, 1]`: its point at `s` is the curve point at `(1+s)/2`. -/
theorem halves_right (b : ConeBezier) (s : ℚ) (axis : Fin 2) :
    b.halves.2.eval s axis = b.eval ((1 + s) / 2) axis := by
  simp only [halves, eval, Matrix.cons_val_zero, Matrix.cons_val_one, Matrix.cons_val_two,
    Matrix.cons_val_three, Matrix.head_cons, Matrix.tail_cons]
  ring

end ConeBezier

namespace ConeSideForm

/-- Model of `sketch.coneSideForm.bezierSide`. The Go `*int` budget is threaded as a natural number and
returned as the third component; it starts nonnegative and is decremented only when nonzero, so it never goes
negative. The Go `depth` starts at 12 and is never negative. -/
@[expose] def bezierSide (f : ConeSideForm) : ConeBezier → ℕ → ℕ → ConeSide × Option ℚ × ℕ
  | _, _, 0 => (.zero, none, 0)
  | b, depth, budget + 1 =>
    let r := f.boxSide b.box.1 b.box.2
    if r.1 ≠ .zero then (r.1, r.2, budget)
    else
      match depth with
      | 0 => (.zero, none, budget)
      | d + 1 =>
        let left := f.bezierSide b.halves.1 d budget
        let right := f.bezierSide b.halves.2 d left.2.2
        if left.1 ≠ right.1 then (.zero, none, right.2.2)
        else if left.1 = .zero then (.zero, none, right.2.2)
        else
          match left.2.1, right.2.1 with
          | some ma, some mc => (left.1, some (if ma ≤ mc then ma else mc), right.2.2)
          | _, _ => (.zero, none, right.2.2)

/-- A side `bezierSide` returns holds at every point of the Bézier curve on `[0, 1]`, with the returned
margin, which is positive. Covers `sketch.coneSideForm.bezierSide`, `coneBezier.box`, `coneBezier.halves` and
`sketch.coneSideForm.boxSide`; exact rational arithmetic is A30. -/
theorem bezierSide_sound (f : ConeSideForm) :
    ∀ (depth budget : ℕ) (b : ConeBezier) {side : ConeSide} {m : ℚ} {budget' : ℕ},
      f.bezierSide b depth budget = (side, some m, budget') →
      0 < m ∧ side ≠ .zero ∧
        ∀ t, 0 ≤ t → t ≤ 1 → f.SideAt side m (b.eval t 0) (b.eval t 1) := by
  intro depth
  induction depth with
  | zero =>
    intro budget b side m budget' hres
    cases budget with
    | zero => simp [bezierSide] at hres
    | succ n =>
      simp only [bezierSide] at hres
      split_ifs at hres with hr
      · simp only [Prod.mk.injEq] at hres
        obtain ⟨h1, h2, -⟩ := hres
        obtain ⟨hm, hs, hall⟩ := boxSide_sound (Prod.ext h1 h2)
        exact ⟨hm, hs, fun t ht0 ht1 => hall _ _ (b.box_mem ht0 ht1).1 (b.box_mem ht0 ht1).2⟩
      · simp at hres
  | succ d ih =>
    intro budget b side m budget' hres
    cases budget with
    | zero => simp [bezierSide] at hres
    | succ n =>
      simp only [bezierSide] at hres
      by_cases hr : (f.boxSide b.box.1 b.box.2).1 ≠ .zero
      · simp only [hr, ne_eq, not_false_eq_true, ↓reduceIte, Prod.mk.injEq] at hres
        obtain ⟨h1, h2, -⟩ := hres
        obtain ⟨hm, hs, hall⟩ := boxSide_sound (Prod.ext h1 h2)
        exact ⟨hm, hs, fun t ht0 ht1 => hall _ _ (b.box_mem ht0 ht1).1 (b.box_mem ht0 ht1).2⟩
      · rw [ne_eq, not_not] at hr
        simp only [hr, ne_eq, not_true_eq_false, ↓reduceIte] at hres
        generalize hl : f.bezierSide b.halves.1 d n = left at hres
        generalize hrt : f.bezierSide b.halves.2 d left.2.2 = right at hres
        obtain ⟨a, ma, bl⟩ := left
        obtain ⟨c, mc, br⟩ := right
        by_cases hac : a = c
        swap
        · simp [hac] at hres
        subst hac
        by_cases ha0 : a = .zero
        · simp [ha0] at hres
        cases ma with
        | none => simp [ha0] at hres
        | some x =>
          cases mc with
          | none => simp [ha0] at hres
          | some y =>
            simp only [ha0, not_true_eq_false, ↓reduceIte, Prod.mk.injEq,
              Option.some.injEq] at hres
            obtain ⟨rfl, rfl, -⟩ := hres
            obtain ⟨hx, -, hlall⟩ := ih n b.halves.1 hl
            obtain ⟨hy, -, hrall⟩ := ih bl b.halves.2 hrt
            have hmx : (if x ≤ y then x else y) ≤ x := by split_ifs <;> linarith
            have hmy : (if x ≤ y then x else y) ≤ y := by split_ifs <;> linarith
            refine ⟨by split_ifs <;> assumption, ha0, fun t ht0 ht1 => ?_⟩
            rcases le_total t (1 / 2) with hh | hh
            · have := hlall (2 * t) (by linarith) (by linarith)
              rw [ConeBezier.halves_left, ConeBezier.halves_left,
                show 2 * t / 2 = t by ring] at this
              exact sideAt_mono hmx this
            · have := hrall (2 * t - 1) (by linarith) (by linarith)
              rw [ConeBezier.halves_right, ConeBezier.halves_right,
                show (1 + (2 * t - 1)) / 2 = t by ring] at this
              exact sideAt_mono hmy this

end ConeSideForm

end Sketch.Cone

end
