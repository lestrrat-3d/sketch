module

public import Sketch.Cone.Bernstein

/-!
# Root search of the cone crossing

Models `sketch.coneFitSearch.visit`, the piece loop that calls it, and the bisection loop of
`sketch.Sketch.CertifyConeFitCrossing` (`cone_fit_crossing.go`). Each piece carries its cone half polynomial
`h`, its implicit function polynomial `f` and `f`'s derivative `df` on the local parameter `u ∈ [0, 1]`.

`visit` bounds `h` and `f` on `[lo, hi]` by their Bernstein coefficients. It refuses unless `h > 0`, accepts
when `f` is strictly signed, records a root when `f` changes sign between the ends and `df` keeps one sign,
and otherwise bisects. `visit_sound` proves what a successful visit means: `h > 0` on `[lo, hi]`, and either `f`
has no zero there and the root slot is unchanged, or the slot was empty and now holds a bracket around the
only zero of `f` on `[lo, hi]`. `searchInv_foldl` lifts this to the piece loop, and `bisect_sound` proves the
bisection keeps the unique zero inside its bracket. Zeros are real numbers: `evalR` evaluates at real `u`.
-/

public section

namespace Sketch.Cone

open Polynomial

/-- Model of the `h`, `f`, `df`, `p0`, `p1`, `total` and `xy` fields of `sketch.coneFitPiece`. -/
structure ConeFitPiece where
  p0 : ℚ
  p1 : ℚ
  total : ℚ
  xy : Fin 2 → List ℚ
  h : List ℚ
  f : List ℚ
  df : List ℚ

/-- Model of `sketch.coneFitRoot`. -/
structure ConeFitRoot where
  index : ℕ
  lo : ℚ
  hi : ℚ
  loSign : ℤ

/-- Model of `big.Rat.Sign`. -/
@[expose] def coneSign (a : ℚ) : ℤ := if 0 < a then 1 else if a < 0 then -1 else 0

/-- Model of `sketch.coneFitSearch.visit`. The search state is the budget and the root slot; the Go method
mutates them in place, the model threads them through and returns them with the result. `&&` short-circuits as
in Go. -/
@[expose] def visit (index : ℕ) (piece : ConeFitPiece) :
    ℕ → ℚ → ℚ → ℕ × Option ConeFitRoot → Bool × (ℕ × Option ConeFitRoot)
  | _, _, _, (0, root) => (false, (0, root))
  | depth, lo, hi, (budget + 1, root) =>
    if (coneFitCoeffBounds piece.h lo hi).1 ≤ 0 then (false, (budget, root))
    else if 0 < (coneFitCoeffBounds piece.f lo hi).1 ∨ (coneFitCoeffBounds piece.f lo hi).2 < 0 then
      (true, (budget, root))
    else if coneFitPolyEval piece.f lo = 0 ∨ coneFitPolyEval piece.f hi = 0 then (false, (budget, root))
    else if coneSign (coneFitPolyEval piece.f lo) ≠ coneSign (coneFitPolyEval piece.f hi) ∧
        ((0 ≤ (coneFitCoeffBounds piece.df lo hi).1 ∧ 0 < (coneFitCoeffBounds piece.df lo hi).2 ∧
            coneFitPolyEval piece.f lo < 0) ∨
          ((coneFitCoeffBounds piece.df lo hi).2 ≤ 0 ∧ (coneFitCoeffBounds piece.df lo hi).1 < 0 ∧
            0 < coneFitPolyEval piece.f lo)) then
      match root with
      | some r => (false, (budget, some r))
      | none => (true, (budget, some ⟨index, lo, hi, coneSign (coneFitPolyEval piece.f lo)⟩))
    else
      match depth with
      | 0 => (false, (budget, root))
      | d + 1 =>
        if coneFitPolyEval piece.f ((lo + hi) * (1 / 2)) = 0 then (false, (budget, root))
        else
          match visit index piece d lo ((lo + hi) * (1 / 2)) (budget, root) with
          | (true, st) => visit index piece d ((lo + hi) * (1 / 2)) hi st
          | (false, st) => (false, st)

/-- A root record `r` for `piece` inside `[lo, hi]`: `r.lo < r.hi` lie in `[lo, hi]`, `f` has the sign
`r.loSign` at `r.lo` and the opposite sign at `r.hi`, and `f` has exactly one real zero on `[lo, hi]`, strictly
inside `(r.lo, r.hi)`. -/
@[expose] def RootOK (piece : ConeFitPiece) (r : ConeFitRoot) (lo hi : ℚ) : Prop :=
  lo ≤ r.lo ∧ r.lo < r.hi ∧ r.hi ≤ hi ∧ (r.loSign = 1 ∨ r.loSign = -1) ∧
    0 < (r.loSign : ℝ) * evalR piece.f r.lo ∧ (r.loSign : ℝ) * evalR piece.f r.hi < 0 ∧
    ∃ u : ℝ, (r.lo : ℝ) < u ∧ u < r.hi ∧ evalR piece.f u = 0 ∧
      ∀ t : ℝ, (lo : ℝ) ≤ t → t ≤ hi → evalR piece.f t = 0 → t = u

/-- What a successful `visit` of `[lo, hi]` proves, given the root slot before (`root`) and after (`root'`). -/
@[expose] def VisitOK (index : ℕ) (piece : ConeFitPiece) (lo hi : ℚ) (root root' : Option ConeFitRoot) :
    Prop :=
  (∀ t : ℝ, (lo : ℝ) ≤ t → t ≤ hi → 0 < evalR piece.h t) ∧
    ((root' = root ∧ ∀ t : ℝ, (lo : ℝ) ≤ t → t ≤ hi → evalR piece.f t ≠ 0) ∨
      (root = none ∧ ∃ r, root' = some r ∧ r.index = index ∧ RootOK piece r lo hi))

theorem coneSign_mul_pos {a : ℚ} (ha : a ≠ 0) : 0 < ((coneSign a : ℤ) : ℝ) * a := by
  unfold coneSign
  rcases lt_trichotomy a 0 with h | h | h
  · simp only [show ¬ 0 < a from not_lt.mpr h.le, h, ↓reduceIte]
    push_cast
    have : (a : ℝ) < 0 := by exact_mod_cast h
    linarith
  · exact absurd h ha
  · simp only [h, ↓reduceIte]
    push_cast
    exact_mod_cast (by linarith : (0 : ℚ) < 1 * a)

theorem coneSign_unit {a : ℚ} (ha : a ≠ 0) : coneSign a = 1 ∨ coneSign a = -1 := by
  unfold coneSign
  split_ifs with h1 h2
  · exact Or.inl rfl
  · exact Or.inr rfl
  · exact absurd (le_antisymm (not_lt.mp h1) (not_lt.mp h2)) ha

/-- A value whose sign is nonzero and not `σ ∈ {1, −1}` has the sign `−σ`. -/
theorem coneSign_ne_mul_neg {σ : ℤ} (hσ : σ = 1 ∨ σ = -1) {x : ℚ} (h0 : coneSign x ≠ 0) (hne : coneSign x ≠ σ) :
    (σ : ℝ) * x < 0 := by
  have hx : x ≠ 0 := by rintro rfl; simp [coneSign] at h0
  have hpos := coneSign_mul_pos hx
  rcases coneSign_unit hx with h | h <;> rcases hσ with rfl | rfl <;> rw [h] at hne hpos <;>
    simp_all

theorem coneSign_eq_mul_pos {σ : ℤ} {x : ℚ} (h0 : coneSign x ≠ 0) (heq : coneSign x = σ) :
    0 < (σ : ℝ) * x := by
  have hx : x ≠ 0 := by rintro rfl; simp [coneSign] at h0
  rw [← heq]
  exact coneSign_mul_pos hx

/-- Intermediate value theorem for `evalR`: a sign change on `[x, y]` gives a zero there. -/
theorem ivt_zero (g : List ℚ) (σ : ℝ) {x y : ℝ} (hxy : x ≤ y) (hx : 0 < σ * evalR g x)
    (hy : σ * evalR g y < 0) : ∃ z, x ≤ z ∧ z ≤ y ∧ evalR g z = 0 := by
  have hc : ContinuousOn (fun t => σ * evalR g t) (Set.Icc x y) :=
    (continuous_const.mul (continuous_evalR g)).continuousOn
  obtain ⟨z, ⟨hz0, hz1⟩, hz⟩ := intermediate_value_Icc' hxy hc ⟨hy.le, hx.le⟩
  have hσ : σ ≠ 0 := by rintro rfl; simp at hx
  exact ⟨z, hz0, hz1, (mul_eq_zero.mp hz).resolve_left hσ⟩

/-- With no zero between `x` and `y`, `σ·g` keeps its positive sign from `x` to `y`. -/
theorem sign_keep (g : List ℚ) (σ : ℝ) {x y : ℝ} (hnz : ∀ z ∈ Set.uIcc x y, evalR g z ≠ 0)
    (hx : 0 < σ * evalR g x) : 0 < σ * evalR g y := by
  by_contra hy
  have hσ : σ ≠ 0 := by rintro rfl; simp at hx
  have hy' : σ * evalR g y < 0 := lt_of_le_of_ne (not_lt.mp hy)
    (mul_ne_zero hσ (hnz y Set.right_mem_uIcc))
  rcases le_total x y with hxy | hxy
  · obtain ⟨z, h0, h1, hz⟩ := ivt_zero g σ hxy hx hy'
    exact hnz z (Set.mem_uIcc.mpr (Or.inl ⟨h0, h1⟩)) hz
  · obtain ⟨z, h0, h1, hz⟩ := ivt_zero g (-σ) hxy (by linarith) (by linarith)
    exact hnz z (Set.mem_uIcc.mpr (Or.inr ⟨h0, h1⟩)) hz

theorem monotoneOn_evalR (g : List ℚ) {lo hi : ℝ}
    (h : ∀ t, lo ≤ t → t ≤ hi → 0 ≤ evalR (coneFitPolyDeriv g) t) : MonotoneOn (evalR g) (Set.Icc lo hi) :=
  monotoneOn_of_deriv_nonneg (convex_Icc lo hi) (continuous_evalR g).continuousOn
    (fun t _ => (hasDerivAt_evalR g t).differentiableAt.differentiableWithinAt)
    (fun t ht => by
      rw [(hasDerivAt_evalR g t).deriv]
      have := interior_subset ht
      exact h t this.1 this.2)

theorem antitoneOn_evalR (g : List ℚ) {lo hi : ℝ}
    (h : ∀ t, lo ≤ t → t ≤ hi → evalR (coneFitPolyDeriv g) t ≤ 0) : AntitoneOn (evalR g) (Set.Icc lo hi) :=
  antitoneOn_of_deriv_nonpos (convex_Icc lo hi) (continuous_evalR g).continuousOn
    (fun t _ => (hasDerivAt_evalR g t).differentiableAt.differentiableWithinAt)
    (fun t ht => by
      rw [(hasDerivAt_evalR g t).deriv]
      have := interior_subset ht
      exact h t this.1 this.2)

/-- A polynomial that vanishes on a nondegenerate interval is zero everywhere. -/
theorem evalR_eq_zero_of_Icc (g : List ℚ) {x y : ℝ} (hxy : x < y) (h : ∀ z, x ≤ z → z ≤ y → evalR g z = 0)
    (t : ℝ) : evalR g t = 0 := by
  have hP : (toPoly g).map (algebraMap ℚ ℝ) = 0 := by
    apply eq_zero_of_infinite_isRoot
    refine (Set.Icc_infinite hxy).mono fun z hz => ?_
    simp only [Set.mem_ofPred_eq, IsRoot.def]
    exact h z hz.1 hz.2
  simp [evalR, hP]

/-- A monotone or antitone polynomial on `[lo, hi]` that is nonzero at `lo` has at most one zero there. -/
theorem zero_unique (g : List ℚ) {lo hi : ℝ}
    (hm : MonotoneOn (evalR g) (Set.Icc lo hi) ∨ AntitoneOn (evalR g) (Set.Icc lo hi)) (hlo : evalR g lo ≠ 0)
    {x y : ℝ} (hx : x ∈ Set.Icc lo hi) (hy : y ∈ Set.Icc lo hi) (hgx : evalR g x = 0) (hgy : evalR g y = 0) :
    x = y := by
  have key : ∀ x y, x ∈ Set.Icc lo hi → y ∈ Set.Icc lo hi → evalR g x = 0 → evalR g y = 0 → ¬ x < y := by
    intro x y hx hy hgx hgy hxy
    apply hlo
    refine evalR_eq_zero_of_Icc g hxy (fun z hz0 hz1 => ?_) lo
    have hz : z ∈ Set.Icc lo hi := ⟨le_trans hx.1 hz0, le_trans hz1 hy.2⟩
    rcases hm with hm | hm
    · exact le_antisymm (hgy ▸ hm hz hy hz1) (hgx ▸ hm hx hz hz0)
    · exact le_antisymm (hgx ▸ hm hx hz hz0) (hgy ▸ hm hz hy hz1)
  rcases lt_trichotomy x y with h | h | h
  · exact absurd h (key x y hx hy hgx hgy)
  · exact h
  · exact absurd h (key y x hy hx hgy hgx)

theorem visit_hpos (piece : ConeFitPiece) {lo hi : ℚ} (hle : lo ≤ hi)
    (h : ¬ (coneFitCoeffBounds piece.h lo hi).1 ≤ 0) :
    ∀ t : ℝ, (lo : ℝ) ≤ t → t ≤ hi → 0 < evalR piece.h t := by
  intro t ht0 ht1
  have := (coneFitCoeffBounds_mem_Icc piece.h hle ht0 ht1).1
  have hq : (0 : ℝ) < (coneFitCoeffBounds piece.h lo hi).1 := by exact_mod_cast not_le.mp h
  linarith

theorem visit_fnz (piece : ConeFitPiece) {lo hi : ℚ} (hle : lo ≤ hi)
    (h : 0 < (coneFitCoeffBounds piece.f lo hi).1 ∨ (coneFitCoeffBounds piece.f lo hi).2 < 0) :
    ∀ t : ℝ, (lo : ℝ) ≤ t → t ≤ hi → evalR piece.f t ≠ 0 := by
  intro t ht0 ht1
  have hb := coneFitCoeffBounds_mem_Icc piece.f hle ht0 ht1
  rcases h with h | h
  · have : (0 : ℝ) < (coneFitCoeffBounds piece.f lo hi).1 := by exact_mod_cast h
    linarith [hb.1]
  · have : ((coneFitCoeffBounds piece.f lo hi).2 : ℝ) < 0 := by exact_mod_cast h
    linarith [hb.2]

/-- The leaf where `visit` records a root: `f` changes sign between the ends and `df` keeps one sign. -/
theorem visit_leaf (index : ℕ) (piece : ConeFitPiece) (hdf : piece.df = coneFitPolyDeriv piece.f) {lo hi : ℚ}
    (hle : lo ≤ hi) (hz : ¬ (coneFitPolyEval piece.f lo = 0 ∨ coneFitPolyEval piece.f hi = 0))
    (hs : coneSign (coneFitPolyEval piece.f lo) ≠ coneSign (coneFitPolyEval piece.f hi) ∧
        ((0 ≤ (coneFitCoeffBounds piece.df lo hi).1 ∧ 0 < (coneFitCoeffBounds piece.df lo hi).2 ∧
            coneFitPolyEval piece.f lo < 0) ∨
          ((coneFitCoeffBounds piece.df lo hi).2 ≤ 0 ∧ (coneFitCoeffBounds piece.df lo hi).1 < 0 ∧
            0 < coneFitPolyEval piece.f lo))) :
    RootOK piece ⟨index, lo, hi, coneSign (coneFitPolyEval piece.f lo)⟩ lo hi := by
  set a := coneFitPolyEval piece.f lo with ha_def
  set b := coneFitPolyEval piece.f hi with hb_def
  obtain ⟨ha0, hb0⟩ := not_or.mp hz
  obtain ⟨hsab, hd⟩ := hs
  have hea : evalR piece.f lo = a := evalR_ratCast _ _
  have heb : evalR piece.f hi = b := evalR_ratCast _ _
  have hσa := coneSign_mul_pos ha0
  have hσ := coneSign_unit ha0
  have hσb : ((coneSign a : ℤ) : ℝ) * b < 0 := by
    rcases coneSign_unit hb0 with h | h <;> rcases hσ with h' | h'
    · exact absurd (h'.trans h.symm) hsab
    · have := coneSign_mul_pos hb0; rw [h] at this; rw [h']; push_cast at this ⊢; linarith
    · have := coneSign_mul_pos hb0; rw [h] at this; rw [h']; push_cast at this ⊢; linarith
    · exact absurd (h'.trans h.symm) hsab
  have hlt : lo < hi := by
    rcases eq_or_lt_of_le hle with h | h
    · exfalso; rw [hb_def, ← h] at hsab; exact hsab rfl
    · exact h
  have hleR : (lo : ℝ) ≤ hi := by exact_mod_cast hle
  have hderiv : ∀ t : ℝ, (lo : ℝ) ≤ t → t ≤ hi →
      ((coneFitCoeffBounds piece.df lo hi).1 : ℝ) ≤ evalR (coneFitPolyDeriv piece.f) t ∧
        evalR (coneFitPolyDeriv piece.f) t ≤ (coneFitCoeffBounds piece.df lo hi).2 := by
    intro t ht0 ht1
    rw [← hdf]
    exact coneFitCoeffBounds_mem_Icc piece.df hle ht0 ht1
  have hm : MonotoneOn (evalR piece.f) (Set.Icc lo hi) ∨ AntitoneOn (evalR piece.f) (Set.Icc lo hi) := by
    rcases hd with ⟨h1, -, -⟩ | ⟨h1, -, -⟩
    · refine Or.inl (monotoneOn_evalR _ fun t ht0 ht1 => le_trans ?_ (hderiv t ht0 ht1).1)
      exact_mod_cast h1
    · refine Or.inr (antitoneOn_evalR _ fun t ht0 ht1 => le_trans (hderiv t ht0 ht1).2 ?_)
      exact_mod_cast h1
  have hla : 0 < ((coneSign a : ℤ) : ℝ) * evalR piece.f lo := by rw [hea]; exact hσa
  have hlb : ((coneSign a : ℤ) : ℝ) * evalR piece.f hi < 0 := by rw [heb]; exact hσb
  obtain ⟨z, hz0, hz1, hzf⟩ := ivt_zero piece.f _ hleR hla hlb
  have hfl : evalR piece.f lo ≠ 0 := by rw [hea]; exact_mod_cast ha0
  have hfh : evalR piece.f hi ≠ 0 := by rw [heb]; exact_mod_cast hb0
  refine ⟨le_rfl, hlt, le_rfl, hσ, hla, hlb, z, ?_, ?_, hzf, fun t ht0 ht1 htf => ?_⟩
  · rcases eq_or_lt_of_le hz0 with h | h
    · exact absurd (h ▸ hzf) hfl
    · exact h
  · rcases eq_or_lt_of_le hz1 with h | h
    · exact absurd (h ▸ hzf) hfh
    · exact h
  · exact zero_unique piece.f hm hfl ⟨ht0, ht1⟩ ⟨hz0, hz1⟩ htf hzf

theorem rootOK_widen_right (piece : ConeFitPiece) {r : ConeFitRoot} {lo mid hi : ℚ} (hmh : mid ≤ hi)
    (hr : RootOK piece r lo mid) (hnz : ∀ t : ℝ, (mid : ℝ) ≤ t → t ≤ hi → evalR piece.f t ≠ 0) :
    RootOK piece r lo hi := by
  obtain ⟨h1, h2, h3, h4, h5, h6, u, hu0, hu1, huf, huniq⟩ := hr
  refine ⟨h1, h2, le_trans h3 hmh, h4, h5, h6, u, hu0, hu1, huf, fun t ht0 ht1 htf => ?_⟩
  rcases le_total t mid with h | h
  · exact huniq t ht0 h htf
  · exact absurd htf (hnz t h ht1)

theorem rootOK_widen_left (piece : ConeFitPiece) {r : ConeFitRoot} {lo mid hi : ℚ} (hlm : lo ≤ mid)
    (hr : RootOK piece r mid hi) (hnz : ∀ t : ℝ, (lo : ℝ) ≤ t → t ≤ mid → evalR piece.f t ≠ 0) :
    RootOK piece r lo hi := by
  obtain ⟨h1, h2, h3, h4, h5, h6, u, hu0, hu1, huf, huniq⟩ := hr
  refine ⟨le_trans hlm h1, h2, h3, h4, h5, h6, u, hu0, hu1, huf, fun t ht0 ht1 htf => ?_⟩
  rcases le_total t mid with h | h
  · exact absurd htf (hnz t ht0 h)
  · exact huniq t h ht1 htf

/-- Joining two successful visits of `[lo, mid]` and `[mid, hi]` gives the guarantee on `[lo, hi]`. -/
theorem visitOK_join (index : ℕ) (piece : ConeFitPiece) {lo mid hi : ℚ} (hlm : lo ≤ mid) (hmh : mid ≤ hi)
    {root root1 root2 : Option ConeFitRoot} (hl : VisitOK index piece lo mid root root1)
    (hr : VisitOK index piece mid hi root1 root2) : VisitOK index piece lo hi root root2 := by
  obtain ⟨hlh, hlz⟩ := hl
  obtain ⟨hrh, hrz⟩ := hr
  have hmR : (mid : ℝ) ≤ hi := by exact_mod_cast hmh
  have hlR : (lo : ℝ) ≤ mid := by exact_mod_cast hlm
  refine ⟨fun t ht0 ht1 => ?_, ?_⟩
  · rcases le_total t mid with h | h
    · exact hlh t ht0 h
    · exact hrh t h ht1
  rcases hlz with ⟨rfl, hlnz⟩ | ⟨hnone, r, rfl, hidx, hrok⟩
  · rcases hrz with ⟨rfl, hrnz⟩ | ⟨hnone, r, rfl, hidx, hrok⟩
    · refine Or.inl ⟨rfl, fun t ht0 ht1 => ?_⟩
      rcases le_total t mid with h | h
      · exact hlnz t ht0 h
      · exact hrnz t h ht1
    · exact Or.inr ⟨hnone, r, rfl, hidx, rootOK_widen_left piece hlm hrok hlnz⟩
  · rcases hrz with ⟨rfl, hrnz⟩ | ⟨h, -⟩
    · exact Or.inr ⟨hnone, r, rfl, hidx, rootOK_widen_right piece hmh hrok hrnz⟩
    · simp at h

/-- A `visit` of `[lo, hi]` that returns true proves `h > 0` on `[lo, hi]`, and either leaves the root slot
unchanged with `f` free of zeros on `[lo, hi]`, or fills an empty slot with a bracket around the only zero of
`f` on `[lo, hi]`. Covers `sketch.coneFitSearch.visit` and the Bernstein bound `coneFitCoeffBounds_mem`;
assumptions A30, A32. -/
theorem visit_sound (index : ℕ) (piece : ConeFitPiece) (hdf : piece.df = coneFitPolyDeriv piece.f) :
    ∀ (depth : ℕ) (lo hi : ℚ) (st st' : ℕ × Option ConeFitRoot), lo ≤ hi →
      visit index piece depth lo hi st = (true, st') → VisitOK index piece lo hi st.2 st'.2 := by
  intro depth
  induction depth with
  | zero =>
    intro lo hi st st' hle h
    obtain ⟨bud, root⟩ := st
    cases bud with
    | zero => simp [visit] at h
    | succ bud =>
      simp only [visit] at h
      split_ifs at h with h1 h2 h3 h4
      · simp at h
      · simp only [Prod.mk.injEq, true_and] at h
        subst h
        exact ⟨visit_hpos piece hle h1, Or.inl ⟨rfl, visit_fnz piece hle h2⟩⟩
      · simp at h
      · cases root with
        | some r => simp at h
        | none =>
          simp only [Prod.mk.injEq, true_and] at h
          subst h
          exact ⟨visit_hpos piece hle h1, Or.inr ⟨rfl, _, rfl, rfl, visit_leaf index piece hdf hle h3 h4⟩⟩
      · simp at h
  | succ d ih =>
    intro lo hi st st' hle h
    obtain ⟨bud, root⟩ := st
    cases bud with
    | zero => simp [visit] at h
    | succ bud =>
      simp only [visit] at h
      split_ifs at h with h1 h2 h3 h4 h5
      · simp at h
      · simp only [Prod.mk.injEq, true_and] at h
        subst h
        exact ⟨visit_hpos piece hle h1, Or.inl ⟨rfl, visit_fnz piece hle h2⟩⟩
      · simp at h
      · cases root with
        | some r => simp at h
        | none =>
          simp only [Prod.mk.injEq, true_and] at h
          subst h
          exact ⟨visit_hpos piece hle h1, Or.inr ⟨rfl, _, rfl, rfl, visit_leaf index piece hdf hle h3 h4⟩⟩
      · simp at h
      · have hlm : lo ≤ (lo + hi) * (1 / 2) := by linarith
        have hmh : (lo + hi) * (1 / 2) ≤ hi := by linarith
        generalize hl : visit index piece d lo ((lo + hi) * (1 / 2)) (bud, root) = left at h
        obtain ⟨ok, st1⟩ := left
        cases ok with
        | false => simp at h
        | true =>
          simp only at h
          have hL := ih lo _ (bud, root) st1 hlm hl
          have hR := ih _ hi st1 st' hmh h
          exact visitOK_join index piece hlm hmh hL hR

/-- One iteration of the piece loop in `sketch.Sketch.CertifyConeFitCrossing`: visit piece `i` on `[0, 1]`
with depth 24; `none` is a refusal. -/
@[expose] def searchStep (pieces : ℕ → ConeFitPiece) (st : Option (ℕ × Option ConeFitRoot)) (i : ℕ) :
    Option (ℕ × Option ConeFitRoot) :=
  match st with
  | none => none
  | some s =>
    match visit i (pieces i) 24 0 1 s with
    | (true, s') => some s'
    | (false, _) => none

/-- The piece loop invariant after `k` pieces: `h > 0` on every piece read, and either no zero of `f` yet, or
one root record whose bracket holds the only zero of `f` over every piece read. -/
@[expose] def SearchInv (pieces : ℕ → ConeFitPiece) (k : ℕ) (st : Option (ℕ × Option ConeFitRoot)) : Prop :=
  ∀ bud root, st = some (bud, root) →
    (∀ i < k, ∀ t : ℝ, 0 ≤ t → t ≤ 1 → 0 < evalR (pieces i).h t) ∧
    ((root = none ∧ ∀ i < k, ∀ t : ℝ, 0 ≤ t → t ≤ 1 → evalR (pieces i).f t ≠ 0) ∨
      (∃ r, root = some r ∧ r.index < k ∧ RootOK (pieces r.index) r 0 1 ∧
        ∀ i < k, ∀ t : ℝ, 0 ≤ t → t ≤ 1 → evalR (pieces i).f t = 0 → i = r.index))

theorem searchInv_step (pieces : ℕ → ConeFitPiece) (hdf : ∀ i, (pieces i).df = coneFitPolyDeriv (pieces i).f)
    (k : ℕ) (st : Option (ℕ × Option ConeFitRoot)) (hk : SearchInv pieces k st) :
    SearchInv pieces (k + 1) (searchStep pieces st k) := by
  intro bud' root' hst
  cases st with
  | none => simp [searchStep] at hst
  | some s =>
    obtain ⟨bud, root⟩ := s
    obtain ⟨hh, hz⟩ := hk bud root rfl
    simp only [searchStep] at hst
    generalize hv : visit k (pieces k) 24 0 1 (bud, root) = v at hst
    obtain ⟨ok, s'⟩ := v
    cases ok with
    | false => simp at hst
    | true =>
      simp only [Option.some.injEq] at hst
      subst hst
      obtain ⟨vh, vz⟩ := visit_sound k (pieces k) (hdf k) 24 0 1 (bud, root) (bud', root') zero_le_one hv
      simp only [Rat.cast_zero, Rat.cast_one] at vh vz
      refine ⟨fun i hi t ht0 ht1 => ?_, ?_⟩
      · rcases Nat.lt_succ_iff_lt_or_eq.mp hi with hi | rfl
        · exact hh i hi t ht0 ht1
        · exact vh t ht0 ht1
      rcases hz with ⟨rfl, hnz⟩ | ⟨r, rfl, hidx, hrok, hzr⟩
      · rcases vz with ⟨rfl, hvnz⟩ | ⟨-, r, rfl, hridx, hrok⟩
        · refine Or.inl ⟨rfl, fun i hi t ht0 ht1 => ?_⟩
          rcases Nat.lt_succ_iff_lt_or_eq.mp hi with hi | rfl
          · exact hnz i hi t ht0 ht1
          · exact hvnz t ht0 ht1
        · refine Or.inr ⟨r, rfl, by omega, hridx ▸ hrok, fun i hi t ht0 ht1 htf => ?_⟩
          rcases Nat.lt_succ_iff_lt_or_eq.mp hi with hi | rfl
          · exact absurd htf (hnz i hi t ht0 ht1)
          · exact hridx.symm
      · rcases vz with ⟨rfl, hvnz⟩ | ⟨h, -⟩
        · refine Or.inr ⟨r, rfl, by omega, hrok, fun i hi t ht0 ht1 htf => ?_⟩
          rcases Nat.lt_succ_iff_lt_or_eq.mp hi with hi | rfl
          · exact hzr i hi t ht0 ht1 htf
          · exact absurd htf (hvnz t ht0 ht1)
        · simp at h

theorem searchInv_foldl (pieces : ℕ → ConeFitPiece) (hdf : ∀ i, (pieces i).df = coneFitPolyDeriv (pieces i).f)
    (budget k : ℕ) : SearchInv pieces k ((List.range k).foldl (searchStep pieces) (some (budget, none))) := by
  induction k with
  | zero =>
    intro bud root h
    simp only [List.range_zero, List.foldl_nil, Option.some.injEq, Prod.mk.injEq] at h
    obtain ⟨-, rfl⟩ := h
    exact ⟨fun i hi => absurd hi (Nat.not_lt_zero i), Or.inl ⟨rfl, fun i hi => absurd hi (Nat.not_lt_zero i)⟩⟩
  | succ k ih =>
    rw [List.range_succ, List.foldl_append, List.foldl_cons, List.foldl_nil]
    exact searchInv_step pieces hdf k _ ih

/-- Model of the bisection loop of `sketch.Sketch.CertifyConeFitCrossing`, with the fuel counting the
remaining iterations of `for step := 0; step < 256; step++`. `stop step lo hi` stands for the early exit
`step >= 52 && coneBracketAtFloatPrecision(piece.global(lo), piece.global(hi))`; the theorems hold for every
`stop`, so that function is not modeled. -/
@[expose] def bisect (f : List ℚ) (σ : ℤ) (stop : ℕ → ℚ → ℚ → Bool) : ℕ → ℕ → ℚ → ℚ → ℚ × ℚ
  | 0, _, lo, hi => (lo, hi)
  | fuel + 1, step, lo, hi =>
    if coneSign (coneFitPolyEval f ((lo + hi) * (1 / 2))) = 0 then
      ((lo + hi) * (1 / 2), (lo + hi) * (1 / 2))
    else if coneSign (coneFitPolyEval f ((lo + hi) * (1 / 2))) = σ then
      if stop step ((lo + hi) * (1 / 2)) hi then ((lo + hi) * (1 / 2), hi)
      else bisect f σ stop fuel (step + 1) ((lo + hi) * (1 / 2)) hi
    else
      if stop step lo ((lo + hi) * (1 / 2)) then (lo, (lo + hi) * (1 / 2))
      else bisect f σ stop fuel (step + 1) lo ((lo + hi) * (1 / 2))

theorem bisect_final (f : List ℚ) (σ : ℤ) {rlo rhi lo hi : ℚ} {u : ℝ}
    (huniq : ∀ t : ℝ, (rlo : ℝ) ≤ t → t ≤ rhi → evalR f t = 0 → t = u) (h1 : rlo ≤ lo) (h2 : lo ≤ hi)
    (h3 : hi ≤ rhi) (hl : 0 < (σ : ℝ) * evalR f lo) (hh : (σ : ℝ) * evalR f hi < 0) :
    rlo ≤ lo ∧ lo ≤ hi ∧ hi ≤ rhi ∧ (lo : ℝ) ≤ u ∧ u ≤ hi := by
  have h2R : (lo : ℝ) ≤ hi := by exact_mod_cast h2
  obtain ⟨z, hz0, hz1, hz⟩ := ivt_zero f _ h2R hl hh
  have hzu := huniq z (le_trans (by exact_mod_cast h1) hz0) (le_trans hz1 (by exact_mod_cast h3)) hz
  subst hzu
  exact ⟨h1, h2, h3, hz0, hz1⟩

/-- The bisection keeps the unique zero `u` of `f` on `[rlo, rhi]` inside its bracket: starting from a bracket
in `[rlo, rhi]` with `σ·f > 0` at its low end and `σ·f < 0` at its high end, the returned bracket lies in
`[rlo, rhi]` and holds `u`. Covers the bisection loop of `sketch.Sketch.CertifyConeFitCrossing`; assumption
A30. -/
theorem bisect_sound (f : List ℚ) (σ : ℤ) (hσ : σ = 1 ∨ σ = -1) (stop : ℕ → ℚ → ℚ → Bool) {rlo rhi : ℚ} {u : ℝ}
    (huniq : ∀ t : ℝ, (rlo : ℝ) ≤ t → t ≤ rhi → evalR f t = 0 → t = u) :
    ∀ (fuel step : ℕ) (lo hi : ℚ), rlo ≤ lo → lo ≤ hi → hi ≤ rhi → 0 < (σ : ℝ) * evalR f lo →
      (σ : ℝ) * evalR f hi < 0 →
      rlo ≤ (bisect f σ stop fuel step lo hi).1 ∧
        (bisect f σ stop fuel step lo hi).1 ≤ (bisect f σ stop fuel step lo hi).2 ∧
        (bisect f σ stop fuel step lo hi).2 ≤ rhi ∧
        ((bisect f σ stop fuel step lo hi).1 : ℝ) ≤ u ∧ u ≤ (bisect f σ stop fuel step lo hi).2 := by
  intro fuel
  induction fuel with
  | zero =>
    intro step lo hi h1 h2 h3 hl hh
    exact bisect_final f σ huniq h1 h2 h3 hl hh
  | succ n ih =>
    intro step lo hi h1 h2 h3 hl hh
    set mid := (lo + hi) * (1 / 2) with hmid
    have hlm : lo ≤ mid := by rw [hmid]; linarith
    have hmh : mid ≤ hi := by rw [hmid]; linarith
    have hem : evalR f mid = coneFitPolyEval f mid := evalR_ratCast _ _
    simp only [bisect]
    rw [← hmid]
    split_ifs with hs0 hsσ hstop hstop
    · have hz : coneFitPolyEval f mid = 0 := by
        by_contra hne
        rcases coneSign_unit hne with h | h <;> rw [h] at hs0 <;> simp at hs0
      have hmu := huniq mid (by exact_mod_cast le_trans h1 hlm) (by exact_mod_cast le_trans hmh h3)
        (by rw [hem, hz]; simp)
      subst hmu
      exact ⟨le_trans h1 hlm, le_rfl, le_trans hmh h3, le_rfl, le_rfl⟩
    · have hp : 0 < (σ : ℝ) * evalR f mid := by rw [hem]; exact coneSign_eq_mul_pos hs0 hsσ
      exact bisect_final f σ huniq (le_trans h1 hlm) hmh h3 hp hh
    · have hp : 0 < (σ : ℝ) * evalR f mid := by rw [hem]; exact coneSign_eq_mul_pos hs0 hsσ
      exact ih (step + 1) mid hi (le_trans h1 hlm) hmh h3 hp hh
    · have hn : (σ : ℝ) * evalR f mid < 0 := by rw [hem]; exact coneSign_ne_mul_neg hσ hs0 hsσ
      exact bisect_final f σ huniq h1 hlm (le_trans hmh h3) hl hn
    · have hn : (σ : ℝ) * evalR f mid < 0 := by rw [hem]; exact coneSign_ne_mul_neg hσ hs0 hsσ
      exact ih (step + 1) lo mid h1 hlm (le_trans hmh h3) hl hn

end Sketch.Cone

end
