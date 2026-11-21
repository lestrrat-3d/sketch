module

public import Sketch.Cone.Bezier

/-!
# Cone side of a fitted spline

Models `sketch.coneFitBezier` (`cone_fit_crossing.go`) and `sketch.coneSideForm.fitSide` (`cone_side.go`).
A fitted spline's ideal interpolant is a natural cubic spline: on span `i`, with `h` the span width,
`p₀`, `p₁` the span's end points and `m₀`, `m₁` their second derivatives, each coordinate is
`c₀ + u·(c₁ + u·(c₂ + u·c₃))` for `u ∈ [0, 1]` with
`c₀ = p₀`, `c₁ = (p₁ − p₀) − h²·(2m₀ + m₁)/6`, `c₂ = h²·m₀/2`, `c₃ = h²·(m₁ − m₀)/6`. This is the exact form
`geom.ExactFitSpan` documents and `geom.cubicSpanCoeffs` computes (A33).

`coneFitBezier_eval` proves the Bézier control points `coneFitBezier` builds trace exactly that cubic.
`fitSide_sound` proves a side `fitSide` returns holds, with the returned positive margin, at every point of
every span.
-/

public section

namespace Sketch.Cone

/-- The exact fields of a `geom.FitInterpolant`: span parameters, active points and second derivatives. -/
structure FitData where
  params : List ℚ
  points : List (Fin 2 → ℚ)
  derivs : List (Fin 2 → ℚ)

namespace FitData

/-- Width of span `i`. -/
@[expose] def width (fi : FitData) (i : ℕ) : ℚ := fi.params.getD (i + 1) 0 - fi.params.getD i 0

/-- Monomial coefficient `k` of span `i`, coordinate `axis`, per `geom.ExactFitSpan`. -/
@[expose] def coeff (fi : FitData) (i : ℕ) (axis : Fin 2) : Fin 4 → ℚ :=
  let h := fi.width i
  let p0 := fi.points.getD i (fun _ => 0) axis
  let p1 := fi.points.getD (i + 1) (fun _ => 0) axis
  let m0 := fi.derivs.getD i (fun _ => 0) axis
  let m1 := fi.derivs.getD (i + 1) (fun _ => 0) axis
  ![p0, (p1 - p0) - h * h * ((2 * m0 + m1) / 6), h * h * (m0 / 2), h * h * ((m1 - m0) / 6)]

/-- Coordinate `axis` of span `i` of the ideal interpolant at local parameter `u ∈ [0, 1]`. -/
@[expose] def spanPoint (fi : FitData) (i : ℕ) (u : ℚ) (axis : Fin 2) : ℚ :=
  fi.coeff i axis 0 + u * (fi.coeff i axis 1 + u * (fi.coeff i axis 2 + u * fi.coeff i axis 3))

end FitData

/-- Model of `sketch.coneFitBezier`: the cubic Bézier control points of span `i`, per axis. -/
@[expose] def coneFitBezier (fi : FitData) (i : ℕ) : ConeBezier :=
  ⟨fun k axis =>
    let h := fi.width i
    let h2 := h * h
    let p0 := fi.points.getD i (fun _ => 0) axis
    let p1 := fi.points.getD (i + 1) (fun _ => 0) axis
    let m0 := fi.derivs.getD i (fun _ => 0) axis
    let m1 := fi.derivs.getD (i + 1) (fun _ => 0) axis
    let c0 := p0
    let c1 := (p1 - p0) - h2 * ((2 * m0 + m1) * (1 / 6))
    let c2 := h2 * (m0 * (1 / 2))
    let c3 := h2 * ((m1 - m0) * (1 / 6))
    ![c0, c0 + c1 * (1 / 3), (c0 + c1 * (2 / 3)) + c2 * (1 / 3), (c0 + c1) + (c2 + c3)] k⟩

/-- The Bézier curve of `coneFitBezier` is span `i` of the ideal interpolant. -/
theorem coneFitBezier_eval (fi : FitData) (i : ℕ) (u : ℚ) (axis : Fin 2) :
    (coneFitBezier fi i).eval u axis = fi.spanPoint i u axis := by
  simp only [coneFitBezier, ConeBezier.eval, FitData.spanPoint, FitData.coeff, Matrix.cons_val_zero,
    Matrix.cons_val_one, Matrix.cons_val_two, Matrix.cons_val_three, Matrix.head_cons, Matrix.tail_cons]
  ring

namespace ConeSideForm

/-- One iteration of the span loop in `sketch.coneSideForm.fitSide`. `none` is an early `return 0, nil`.
The Go code compares a nil `margin` with a non-nil `minMargin` only if `bezierSide` pairs a side with a nil
margin, which it never does (`bezierSide_margin`); the model refuses there. -/
@[expose] def fitStep (f : ConeSideForm) (fi : FitData) (st : Option (ConeSide × Option ℚ × ℕ)) (i : ℕ) :
    Option (ConeSide × Option ℚ × ℕ) :=
  match st with
  | none => none
  | some (result, minMargin, budget) =>
    if fi.width i ≤ 0 then none
    else
      let r := f.bezierSide (coneFitBezier fi i) 12 budget
      if r.1 = .zero ∨ (result ≠ .zero ∧ result ≠ r.1) then none
      else
        match minMargin, r.2.1 with
        | none, mg => some (r.1, mg, r.2.2)
        | some mm, some mg => some (r.1, some (if mg < mm then mg else mm), r.2.2)
        | some _, none => none

/-- Model of `sketch.coneSideForm.fitSide`: every span shares one budget of 2048 box tests, all spans must
prove the same side, and the smallest margin wins. -/
@[expose] def fitSide (f : ConeSideForm) (fi : FitData) : ConeSide × Option ℚ :=
  let n := fi.points.length
  if n < 2 ∨ fi.derivs.length ≠ n ∨ fi.params.length ≠ n then (.zero, none)
  else
    match (List.range (n - 1)).foldl (f.fitStep fi) (some (.zero, none, 2048)) with
    | some (result, minMargin, _) => (result, minMargin)
    | none => (.zero, none)

/-- `boxSide` pairs every side with a margin. -/
theorem boxSide_margin (f : ConeSideForm) (u v : ConeInterval) (h : (f.boxSide u v).1 ≠ .zero) :
    ∃ m, (f.boxSide u v).2 = some m := by
  unfold boxSide at h ⊢
  split_ifs at h ⊢ <;> first | exact absurd rfl h | exact ⟨_, rfl⟩

/-- `bezierSide` pairs every side with a margin. -/
theorem bezierSide_margin (f : ConeSideForm) :
    ∀ (depth budget : ℕ) (b : ConeBezier), (f.bezierSide b depth budget).1 ≠ .zero →
      ∃ m, (f.bezierSide b depth budget).2.1 = some m := by
  intro depth
  induction depth with
  | zero =>
    intro budget b h
    cases budget with
    | zero => exact absurd rfl h
    | succ n =>
      by_cases hr : (f.boxSide b.box.1 b.box.2).1 ≠ .zero
      · simp only [bezierSide, hr, ne_eq, not_false_eq_true, ↓reduceIte]
        exact boxSide_margin f _ _ hr
      · simp only [bezierSide, hr, ↓reduceIte] at h
        exact absurd rfl h
  | succ d ih =>
    intro budget b h
    cases budget with
    | zero => exact absurd rfl h
    | succ n =>
      by_cases hr : (f.boxSide b.box.1 b.box.2).1 ≠ .zero
      · simp only [bezierSide, hr, ne_eq, not_false_eq_true, ↓reduceIte]
        exact boxSide_margin f _ _ hr
      · rw [ne_eq, not_not] at hr
        simp only [bezierSide, hr, ne_eq, not_true_eq_false, ↓reduceIte] at h ⊢
        generalize f.bezierSide b.halves.1 d n = left at h ⊢
        generalize f.bezierSide b.halves.2 d left.2.2 = right at h ⊢
        obtain ⟨a, ma, bl⟩ := left
        obtain ⟨c, mc, br⟩ := right
        by_cases hac : a = c
        swap
        · simp [hac] at h
        subst hac
        by_cases ha0 : a = .zero
        · simp [ha0] at h
        cases ma with
        | none => simp [ha0] at h
        | some x =>
          cases mc with
          | none => simp [ha0] at h
          | some y => simp [ha0]

/-- The loop invariant of `fitSide` after `k` spans: either no span has been read, or every span read so far
proves one nonzero side with the current positive margin. -/
@[expose] def FitInv (f : ConeSideForm) (fi : FitData) (k : ℕ) (st : Option (ConeSide × Option ℚ × ℕ)) :
    Prop :=
  ∀ result mm bud, st = some (result, mm, bud) →
    (k = 0 ∧ result = .zero ∧ mm = none) ∨
      (result ≠ .zero ∧ ∃ m, mm = some m ∧ 0 < m ∧
        ∀ i < k, ∀ u, 0 ≤ u → u ≤ 1 → f.SideAt result m (fi.spanPoint i u 0) (fi.spanPoint i u 1))

theorem fitInv_step (f : ConeSideForm) (fi : FitData) (k : ℕ) (st : Option (ConeSide × Option ℚ × ℕ))
    (hk : f.FitInv fi k st) : f.FitInv fi (k + 1) (f.fitStep fi st k) := by
  intro result' mm' bud' hst
  cases st with
  | none => simp [fitStep] at hst
  | some s =>
    obtain ⟨result, mm, bud⟩ := s
    have hinv := hk result mm bud rfl
    simp only [fitStep] at hst
    split_ifs at hst with hw hrej
    generalize hr : f.bezierSide (coneFitBezier fi k) 12 bud = r at hst hrej
    obtain ⟨side, mg, bud2⟩ := r
    simp only [not_or, not_and, not_not] at hrej
    obtain ⟨hside, hsame⟩ := hrej
    obtain ⟨mgv, hmg⟩ := bezierSide_margin f 12 bud (coneFitBezier fi k) (by rw [hr]; exact hside)
    rw [hr] at hmg
    simp only at hmg
    subst hmg
    obtain ⟨hpos, -, hall⟩ := bezierSide_sound f 12 bud (coneFitBezier fi k) hr
    have hspan : ∀ u, 0 ≤ u → u ≤ 1 → f.SideAt side mgv (fi.spanPoint k u 0) (fi.spanPoint k u 1) := by
      intro u hu0 hu1
      have := hall u hu0 hu1
      rwa [coneFitBezier_eval, coneFitBezier_eval] at this
    rcases hinv with ⟨rfl, rfl, rfl⟩ | ⟨hres0, m0, rfl, hm0, hall0⟩
    · simp only [Option.some.injEq, Prod.mk.injEq] at hst
      obtain ⟨rfl, rfl, -⟩ := hst
      refine Or.inr ⟨hside, mgv, rfl, hpos, fun i hi u hu0 hu1 => ?_⟩
      have : i = 0 := by omega
      subst this
      exact hspan u hu0 hu1
    · have hrs : result = side := hsame hres0
      subst hrs
      simp only [Option.some.injEq, Prod.mk.injEq] at hst
      obtain ⟨rfl, rfl, -⟩ := hst
      have hl0 : (if mgv < m0 then mgv else m0) ≤ m0 := by split_ifs <;> linarith
      have hl1 : (if mgv < m0 then mgv else m0) ≤ mgv := by split_ifs <;> linarith
      refine Or.inr ⟨hres0, _, rfl, by split_ifs <;> assumption, fun i hi u hu0 hu1 => ?_⟩
      rcases Nat.lt_succ_iff_lt_or_eq.mp hi with hi | rfl
      · exact sideAt_mono hl0 (hall0 i hi u hu0 hu1)
      · exact sideAt_mono hl1 (hspan u hu0 hu1)

theorem fitInv_foldl (f : ConeSideForm) (fi : FitData) (k : ℕ) :
    f.FitInv fi k ((List.range k).foldl (f.fitStep fi) (some (.zero, none, 2048))) := by
  induction k with
  | zero =>
    intro result mm bud h
    simp only [List.range_zero, List.foldl_nil, Option.some.injEq, Prod.mk.injEq] at h
    obtain ⟨rfl, rfl, -⟩ := h
    exact Or.inl ⟨rfl, rfl, rfl⟩
  | succ k ih =>
    rw [List.range_succ, List.foldl_append, List.foldl_cons, List.foldl_nil]
    exact fitInv_step f fi k _ ih

/-- A side `fitSide` returns holds at every point of every span of the ideal interpolant, with the returned
margin, which is positive. Covers `sketch.coneSideForm.fitSide`, `sketch.coneFitBezier`,
`sketch.coneSideForm.bezierSide`, `coneBezier.box`, `coneBezier.halves` and `sketch.coneSideForm.boxSide`;
assumptions A30, A31, A33. -/
theorem fitSide_sound {f : ConeSideForm} {fi : FitData} {side : ConeSide} {m : ℚ}
    (hres : f.fitSide fi = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧
      ∀ i, i + 1 < fi.points.length → ∀ u, 0 ≤ u → u ≤ 1 →
        f.SideAt side m (fi.spanPoint i u 0) (fi.spanPoint i u 1) := by
  simp only [fitSide] at hres
  split_ifs at hres with hn
  · simp at hres
  have hinv := fitInv_foldl f fi (fi.points.length - 1)
  generalize (List.range (fi.points.length - 1)).foldl (f.fitStep fi) (some (.zero, none, 2048)) = st
    at hres hinv
  cases st with
  | none => simp at hres
  | some s =>
    obtain ⟨result, mm, bud⟩ := s
    simp only [Prod.mk.injEq] at hres
    obtain ⟨rfl, rfl⟩ := hres
    rcases hinv _ _ _ rfl with ⟨-, -, h⟩ | ⟨hs, m', hm', hpos, hall⟩
    · simp at h
    · simp only [Option.some.injEq] at hm'
      subst hm'
      exact ⟨hpos, hs, fun i hi u hu0 hu1 => hall i (by omega) u hu0 hu1⟩

end ConeSideForm

end Sketch.Cone

end
