module

public import Mathlib

/-!
# Soundness of the parametric Krawczyk test

Models the accepting branch of `sketch.certSystem.krawczyk`. For a square system `F(x, p) = 0` in the `n`
free variables, a float point `x̃`, any real matrix `Y`, and a box `X`, the Go code evaluates

`K(X) = x̃ − Y·F(x̃, Q) + (I − Y·F_x(X, Q))·(X − x̃)`

in outward-rounded interval arithmetic and accepts when every row of `K` lies strictly inside the same row of
`X` (`sketch.Interval.strictlyInside`). The parameter `p` ranges over a set `Q`: the driving value together with
every `WithTargetRange` target and every `WithFixedBox` coordinate.

The interval evaluations are not modeled. Their results enter as real bounds (`cLo`/`cHi`, `CLo`/`CHi`,
`kLo`/`kHi`), and the facts the proof needs about them are the fields of `KrawczykRun.Assumptions`. The proof does
not use Brouwer's theorem: the strict inclusion bounds every row sum of `|I − Y·F_x|`, weighted by the widths of
`X`, below that row's width, so `x ↦ x − Y·F(x, p)` is a contraction of `X` in the weighted max-norm and the
Banach fixed-point theorem applies.

The ε-inflation loop that chooses `X` is not modeled: the theorems hold for whichever pass accepts. Continuity of
the solution in `p` (the implicit function theorem step in the Go doc comment) is not proved here.
-/

public section

namespace Sketch.Enclose

open Matrix Set

/-- Model of `sketch.Interval.strictlyInside`: `[kLo, kHi]` lies in the interior of `[xLo, xHi]`. -/
@[expose] def strictlyInside (kLo kHi xLo xHi : ℝ) : Prop := xLo < kLo ∧ kHi < xHi

/-- The point `x` lies in the box with corners `lo` and `hi`. -/
@[expose] def inBox {n : ℕ} (lo hi x : Fin n → ℝ) : Prop := ∀ i, x i ∈ Icc (lo i) (hi i)

/-- The quantities one accepted pass of `sketch.certSystem.krawczyk` works with, restricted to the free variables.
`F` maps `n` unknowns to `n` equations: the system is square (assumption A15). -/
structure KrawczykRun (n : ℕ) (P : Type*) where
  /-- The certified equations `F(x, p)` of `sketch.certSystem.evalInto`, one row per free variable. -/
  F : (Fin n → ℝ) → P → (Fin n → ℝ)
  /-- The Jacobian matrix `∂F/∂x` at `(x, p)`. -/
  Fx : (Fin n → ℝ) → P → Matrix (Fin n) (Fin n) ℝ
  /-- Every parameter value the pass covers: the driving range with every ranged target and fixed box. -/
  Q : Set P
  /-- The float solution `x̃`. -/
  xt : Fin n → ℝ
  /-- The float approximate inverse; the proof uses no property of it. -/
  Y : Matrix (Fin n) (Fin n) ℝ
  /-- Lower corner of the box `X`. -/
  xLo : Fin n → ℝ
  /-- Upper corner of the box `X`. -/
  xHi : Fin n → ℝ
  /-- Lower ends of `c = −Y·F(x̃, Q)`. -/
  cLo : Fin n → ℝ
  /-- Upper ends of `c = −Y·F(x̃, Q)`. -/
  cHi : Fin n → ℝ
  /-- Lower ends of `C = I − Y·F_x(X, Q)`, the Go loop's `cij`. -/
  CLo : Matrix (Fin n) (Fin n) ℝ
  /-- Upper ends of `C = I − Y·F_x(X, Q)`, the Go loop's `cij`. -/
  CHi : Matrix (Fin n) (Fin n) ℝ
  /-- Lower corner of the Krawczyk image `K`. -/
  kLo : Fin n → ℝ
  /-- Upper corner of the Krawczyk image `K`. -/
  kHi : Fin n → ℝ

namespace KrawczykRun

variable {n : ℕ} {P : Type*} (R : KrawczykRun n P)

/-- Model of the acceptance test of `sketch.certSystem.krawczyk`: every row of `K` lies strictly inside the same
row of `X`. -/
@[expose] def accepts : Prop := ∀ i, strictlyInside (R.kLo i) (R.kHi i) (R.xLo i) (R.xHi i)

/-- The facts about the interval evaluations that the proof assumes. Each field names its row in
`.claude/docs/proofs.md`. -/
structure Assumptions : Prop where
  /-- A10: `F(·, p)` is differentiable at every point of `X` with Jacobian `Fx`. -/
  deriv : ∀ p ∈ R.Q, ∀ x, inBox R.xLo R.xHi x →
    HasFDerivAt (fun z => R.F z p) (LinearMap.toContinuousLinearMap (Matrix.toLin' (R.Fx x p))) x
  /-- A11: the computed `c` encloses `−Y·F(x̃, p)` for every `p ∈ Q`. -/
  residual : ∀ p ∈ R.Q, ∀ i, -(R.Y *ᵥ R.F R.xt p) i ∈ Icc (R.cLo i) (R.cHi i)
  /-- A12: the computed `C` encloses `I − Y·F_x(x, p)` for every `x ∈ X` and `p ∈ Q`. -/
  slope : ∀ p ∈ R.Q, ∀ x, inBox R.xLo R.xHi x →
    ∀ i j, (1 - R.Y * R.Fx x p) i j ∈ Icc (R.CLo i j) (R.CHi i j)
  /-- A13: the computed `K` encloses `x̃ + c + C·(x − x̃)` for every `c`, `C` inside their computed bounds and every
  `x ∈ X`. -/
  image : ∀ c : Fin n → ℝ, (∀ i, c i ∈ Icc (R.cLo i) (R.cHi i)) →
    ∀ C : Matrix (Fin n) (Fin n) ℝ, (∀ i j, C i j ∈ Icc (R.CLo i j) (R.CHi i j)) →
    ∀ x, inBox R.xLo R.xHi x → ∀ i, R.xt i + c i + (C *ᵥ (x - R.xt)) i ∈ Icc (R.kLo i) (R.kHi i)
  /-- A14: the float solution lies in `X`. -/
  xt_mem : inBox R.xLo R.xHi R.xt

/-- The Krawczyk map `g(x) = x − Y·F(x, p)`; its fixed points are the zeros of `F(·, p)` once `Y` is nonsingular. -/
@[expose] noncomputable def g (p : P) (x : Fin n → ℝ) : Fin n → ℝ := x - R.Y *ᵥ R.F x p

/-- The magnitude bound `max(|CLo|, |CHi|)` of one entry of `C`. -/
@[expose] noncomputable def mag (i j : Fin n) : ℝ := max |R.CLo i j| |R.CHi i j|

/-- The width of row `j` of `X`. -/
@[expose] noncomputable def width (j : Fin n) : ℝ := R.xHi j - R.xLo j

theorem mag_nonneg (i j : Fin n) : 0 ≤ R.mag i j := le_max_of_le_left (abs_nonneg _)

theorem mulVec_apply (C : Matrix (Fin n) (Fin n) ℝ) (v : Fin n → ℝ) (i : Fin n) :
    (C *ᵥ v) i = ∑ j, C i j * v j := rfl

/-- The box is convex: the segment between two of its points stays inside. -/
theorem inBox_segment {x y : Fin n → ℝ} (hx : inBox R.xLo R.xHi x) (hy : inBox R.xLo R.xHi y)
    {t : ℝ} (ht : t ∈ Icc (0 : ℝ) 1) : inBox R.xLo R.xHi (y + t • (x - y)) := by
  intro j
  obtain ⟨h0, h1⟩ := ht
  obtain ⟨hxl, hxh⟩ := hx j
  obtain ⟨hyl, hyh⟩ := hy j
  have h1' : 0 ≤ 1 - t := by linarith
  simp only [Pi.add_apply, Pi.smul_apply, Pi.sub_apply, smul_eq_mul]
  constructor
  · nlinarith [mul_le_mul_of_nonneg_left hxl h0, mul_le_mul_of_nonneg_left hyl h1']
  · nlinarith [mul_le_mul_of_nonneg_left hxh h0, mul_le_mul_of_nonneg_left hyh h1']

variable {R}

/-- Mean value theorem, one row at a time: the change of row `i` of `g` between two points of `X` is row `i` of
`(I − Y·F_x(ξ, p))·(x − y)` for some `ξ ∈ X`. -/
theorem g_sub_eq (hA : R.Assumptions) {p : P} (hp : p ∈ R.Q) {x y : Fin n → ℝ}
    (hx : inBox R.xLo R.xHi x) (hy : inBox R.xLo R.xHi y) (i : Fin n) :
    ∃ ξ, inBox R.xLo R.xHi ξ ∧ R.g p x i - R.g p y i = ((1 - R.Y * R.Fx ξ p) *ᵥ (x - y)) i := by
  set z : ℝ → Fin n → ℝ := fun t => y + t • (x - y) with hzdef
  have hz : ∀ t ∈ Icc (0 : ℝ) 1, inBox R.xLo R.xHi (z t) := fun t ht => R.inBox_segment hx hy ht
  have hzd : ∀ t, HasDerivAt z (x - y) t := by
    intro t
    have := ((hasDerivAt_id' t).smul_const (x - y)).const_add y
    rwa [one_smul] at this
  set φ : ℝ → ℝ := fun t => R.g p (z t) i with hφ
  set φ' : ℝ → ℝ := fun t => ((1 - R.Y * R.Fx (z t) p) *ᵥ (x - y)) i with hφ'
  have hderiv : ∀ t ∈ Icc (0 : ℝ) 1, HasDerivAt φ (φ' t) t := by
    intro t ht
    have hF := (hA.deriv p hp (z t) (hz t ht)).comp_hasDerivAt t (hzd t)
    have hFk : ∀ k, HasDerivAt (fun s => R.F (z s) p k) ((R.Fx (z t) p *ᵥ (x - y)) k) t := by
      intro k
      have := (hasDerivAt_pi.1 hF) k
      simpa [Function.comp_def, Matrix.toLin'_apply] using this
    have hsum : HasDerivAt (fun s => ∑ k, R.Y i k * R.F (z s) p k)
        (∑ k, R.Y i k * (R.Fx (z t) p *ᵥ (x - y)) k) t :=
      HasDerivAt.fun_sum fun k _ => (hFk k).const_mul (R.Y i k)
    have hzi : HasDerivAt (fun s => z s i) ((x - y) i) t := (hasDerivAt_pi.1 (hzd t)) i
    have hall := hzi.sub hsum
    have hfun : φ = fun s => z s i - ∑ k, R.Y i k * R.F (z s) p k := by
      funext s
      simp [hφ, g, mulVec_apply]
    rw [hfun]
    convert hall using 1
    change ((1 - R.Y * R.Fx (z t) p) *ᵥ (x - y)) i = (x - y) i - ∑ k, R.Y i k * (R.Fx (z t) p *ᵥ (x - y)) k
    rw [Matrix.sub_mulVec, Matrix.one_mulVec, ← Matrix.mulVec_mulVec]
    rfl
  have hcont : ContinuousOn φ (Icc 0 1) := fun t ht => (hderiv t ht).continuousAt.continuousWithinAt
  obtain ⟨t, ht, heq⟩ := exists_hasDerivAt_eq_slope φ φ' zero_lt_one hcont
    (fun t ht => hderiv t (Ioo_subset_Icc_self ht))
  refine ⟨z t, hz t (Ioo_subset_Icc_self ht), ?_⟩
  have h1 : z 1 = x := by simp [hzdef]
  have h0 : z 0 = y := by simp [hzdef]
  simp only [hφ, h1, h0, sub_zero, div_one] at heq
  rw [← heq]

/-- `g` maps `X` into the Krawczyk image `K`. -/
theorem g_mem_image (hA : R.Assumptions) {p : P} (hp : p ∈ R.Q) {x : Fin n → ℝ}
    (hx : inBox R.xLo R.xHi x) (i : Fin n) : R.g p x i ∈ Icc (R.kLo i) (R.kHi i) := by
  obtain ⟨ξ, hξ, heq⟩ := g_sub_eq hA hp hx hA.xt_mem i
  have hgxt : R.g p R.xt i = R.xt i + -(R.Y *ᵥ R.F R.xt p) i := by
    simp only [g, Pi.sub_apply]
    ring
  have himg := hA.image (fun i => -(R.Y *ᵥ R.F R.xt p) i) (hA.residual p hp) (1 - R.Y * R.Fx ξ p)
    (hA.slope p hp ξ hξ) x hx i
  have hval : R.g p x i = R.xt i + -(R.Y *ᵥ R.F R.xt p) i + ((1 - R.Y * R.Fx ξ p) *ᵥ (x - R.xt)) i := by
    linarith
  rw [hval]
  exact himg

/-- The weighted Lipschitz bound of one row of `g` on `X`. -/
theorem abs_g_sub_le (hA : R.Assumptions) {p : P} (hp : p ∈ R.Q) {x y : Fin n → ℝ}
    (hx : inBox R.xLo R.xHi x) (hy : inBox R.xLo R.xHi y) (i : Fin n) :
    |R.g p x i - R.g p y i| ≤ ∑ j, R.mag i j * |x j - y j| := by
  obtain ⟨ξ, hξ, heq⟩ := g_sub_eq hA hp hx hy i
  rw [heq, mulVec_apply]
  refine (Finset.abs_sum_le_sum_abs _ _).trans (Finset.sum_le_sum fun j _ => ?_)
  rw [abs_mul, Pi.sub_apply]
  obtain ⟨hl, hh⟩ := hA.slope p hp ξ hξ i j
  exact mul_le_mul_of_nonneg_right (abs_le_max_abs_abs hl hh) (abs_nonneg _)

/-- The strict inclusion bounds every weighted row sum of `|C|` below that row's width. -/
theorem sum_mag_mul_width_lt (hA : R.Assumptions) (hacc : R.accepts) {p : P} (hp : p ∈ R.Q) (i : Fin n) :
    ∑ j, R.mag i j * R.width j < R.width i := by
  have hCle : ∀ i j, R.CLo i j ≤ R.CHi i j := fun i j =>
    (hA.slope p hp R.xt hA.xt_mem i j).1.trans (hA.slope p hp R.xt hA.xt_mem i j).2
  have hXle : ∀ j, R.xLo j ≤ R.xHi j := fun j => (hA.xt_mem j).1.trans (hA.xt_mem j).2
  -- The entry of largest magnitude in each cell of `C`.
  set Cs : Matrix (Fin n) (Fin n) ℝ := fun i j => if |R.CLo i j| ≤ |R.CHi i j| then R.CHi i j else R.CLo i j
    with hCs
  have hCsmem : ∀ i j, Cs i j ∈ Icc (R.CLo i j) (R.CHi i j) := by
    intro i j
    simp only [hCs]
    split_ifs
    · exact ⟨hCle i j, le_rfl⟩
    · exact ⟨le_rfl, hCle i j⟩
  have hCsabs : ∀ i j, |Cs i j| = R.mag i j := by
    intro i j
    simp only [hCs, mag]
    split_ifs with h
    · exact (max_eq_right h).symm
    · exact (max_eq_left (not_le.1 h).le).symm
  -- The corners of `X` that maximize and minimize row `i` of `C·(x − x̃)`.
  set dp : Fin n → ℝ := fun j => if 0 ≤ Cs i j then R.xHi j else R.xLo j with hdp
  set dm : Fin n → ℝ := fun j => if 0 ≤ Cs i j then R.xLo j else R.xHi j with hdm
  have hdpmem : inBox R.xLo R.xHi dp := by
    intro j
    simp only [hdp]
    split_ifs
    · exact ⟨hXle j, le_rfl⟩
    · exact ⟨le_rfl, hXle j⟩
  have hdmmem : inBox R.xLo R.xHi dm := by
    intro j
    simp only [hdm]
    split_ifs
    · exact ⟨le_rfl, hXle j⟩
    · exact ⟨hXle j, le_rfl⟩
  set c : Fin n → ℝ := fun i => -(R.Y *ᵥ R.F R.xt p) i
  have hp' := hA.image c (hA.residual p hp) Cs hCsmem dp hdpmem i
  have hm' := hA.image c (hA.residual p hp) Cs hCsmem dm hdmmem i
  have hdiff : (Cs *ᵥ (dp - R.xt)) i - (Cs *ᵥ (dm - R.xt)) i = ∑ j, R.mag i j * R.width j := by
    rw [mulVec_apply, mulVec_apply, ← Finset.sum_sub_distrib]
    refine Finset.sum_congr rfl fun j _ => ?_
    rw [← hCsabs i j, width]
    simp only [hdp, hdm, Pi.sub_apply]
    split_ifs with h
    · rw [abs_of_nonneg h]; ring
    · rw [abs_of_neg (not_le.1 h)]; ring
  obtain ⟨hlo, hhi⟩ := hacc i
  rw [width]
  linarith [hp'.1, hp'.2, hm'.1, hm'.2]

/-- Every row of `X` has positive width. -/
theorem width_pos (hA : R.Assumptions) (hacc : R.accepts) {p : P} (hp : p ∈ R.Q) (j : Fin n) :
    0 < R.width j := by
  have hk := g_mem_image hA hp hA.xt_mem j
  obtain ⟨hlo, hhi⟩ := hacc j
  rw [width]
  linarith [hk.1, hk.2]

/-- A vector whose every entry is bounded by its weighted `|C|` row sum is zero. -/
theorem eq_zero_of_abs_le (hA : R.Assumptions) (hacc : R.accepts) {p : P} (hp : p ∈ R.Q)
    {v : Fin n → ℝ} (hv : ∀ i, |v i| ≤ ∑ j, R.mag i j * |v j|) : v = 0 := by
  by_contra hne
  obtain ⟨j0, hj0⟩ : ∃ j, v j ≠ 0 := by
    by_contra h
    push Not at h
    exact hne (funext h)
  have hw := width_pos hA hacc hp
  obtain ⟨i0, -, hmax⟩ := Finset.exists_max_image Finset.univ (fun i => |v i| / R.width i)
    ⟨j0, Finset.mem_univ _⟩
  set t := |v i0| / R.width i0 with ht
  have htpos : 0 < t := lt_of_lt_of_le (div_pos (abs_pos.2 hj0) (hw j0)) (hmax j0 (Finset.mem_univ _))
  have hle : ∀ j, |v j| ≤ t * R.width j := fun j =>
    (div_le_iff₀ (hw j)).1 (hmax j (Finset.mem_univ _))
  have hvi0 : |v i0| = t * R.width i0 := by
    rw [ht, div_mul_cancel₀ _ (hw i0).ne']
  have hsum := sum_mag_mul_width_lt hA hacc hp i0
  have : |v i0| ≤ t * ∑ j, R.mag i0 j * R.width j := by
    calc |v i0| ≤ ∑ j, R.mag i0 j * |v j| := hv i0
      _ ≤ ∑ j, R.mag i0 j * (t * R.width j) :=
          Finset.sum_le_sum fun j _ => mul_le_mul_of_nonneg_left (hle j) (R.mag_nonneg i0 j)
      _ = t * ∑ j, R.mag i0 j * R.width j := by
          rw [Finset.mul_sum]
          exact Finset.sum_congr rfl fun j _ => by ring
  have := mul_lt_mul_of_pos_left hsum htpos
  linarith

/-- `Y·F_x(ξ, p)` is nonsingular for every `ξ ∈ X` and `p ∈ Q`. -/
theorem det_Y_mul_Fx_ne_zero (hA : R.Assumptions) (hacc : R.accepts) {p : P} (hp : p ∈ R.Q)
    {ξ : Fin n → ℝ} (hξ : inBox R.xLo R.xHi ξ) : (R.Y * R.Fx ξ p).det ≠ 0 := by
  intro hdet
  obtain ⟨v, hv0, hv⟩ := Matrix.exists_mulVec_eq_zero_iff.2 hdet
  refine hv0 (eq_zero_of_abs_le hA hacc hp fun i => ?_)
  have hveq : v = (1 - R.Y * R.Fx ξ p) *ᵥ v := by
    rw [Matrix.sub_mulVec, Matrix.one_mulVec, hv, sub_zero]
  calc |v i| = |((1 - R.Y * R.Fx ξ p) *ᵥ v) i| := by rw [← hveq]
    _ ≤ ∑ j, R.mag i j * |v j| := by
        rw [mulVec_apply]
        refine (Finset.abs_sum_le_sum_abs _ _).trans (Finset.sum_le_sum fun j _ => ?_)
        rw [abs_mul]
        obtain ⟨hl, hh⟩ := hA.slope p hp ξ hξ i j
        exact mul_le_mul_of_nonneg_right (abs_le_max_abs_abs hl hh) (abs_nonneg _)

/-- `g` has a fixed point in `X`: Banach's theorem in coordinates scaled by the widths of `X`. -/
theorem exists_g_fixed (hA : R.Assumptions) (hacc : R.accepts) {p : P} (hp : p ∈ R.Q) :
    ∃ x, inBox R.xLo R.xHi x ∧ R.g p x = x := by
  have hw := width_pos hA hacc hp
  set B : Set (Fin n → ℝ) := {u | inBox R.xLo R.xHi (fun j => R.width j * u j)} with hB
  have hBc : IsClosed B := by
    have : B = ⋂ j, ({u : Fin n → ℝ | R.xLo j ≤ R.width j * u j} ∩ {u | R.width j * u j ≤ R.xHi j}) := by
      ext u
      simp [hB, inBox]
    rw [this]
    refine isClosed_iInter fun j => IsClosed.inter ?_ ?_
    · exact isClosed_le continuous_const (by fun_prop)
    · exact isClosed_le (by fun_prop) continuous_const
  have : CompleteSpace B := hBc.completeSpace_coe
  have hxtB : (fun j => R.xt j / R.width j) ∈ B := by
    intro j
    simp only
    rw [mul_div_cancel₀ _ (hw j).ne']
    exact hA.xt_mem j
  have : Nonempty B := ⟨⟨_, hxtB⟩⟩
  have hmaps : ∀ u ∈ B, (fun j => R.g p (fun j => R.width j * u j) j / R.width j) ∈ B := by
    intro u hu j
    simp only
    rw [mul_div_cancel₀ _ (hw j).ne']
    have hk := g_mem_image hA hp hu j
    exact ⟨(hacc j).1.le.trans hk.1, hk.2.trans (hacc j).2.le⟩
  let H : B → B := fun u => ⟨_, hmaps u.1 u.2⟩
  have hrow : ∀ i, 0 ≤ (∑ j, R.mag i j * R.width j) / R.width i := fun i =>
    div_nonneg (Finset.sum_nonneg fun j _ => mul_nonneg (R.mag_nonneg i j) (hw j).le) (hw i).le
  let L : NNReal := Finset.univ.sup fun i => ⟨(∑ j, R.mag i j * R.width j) / R.width i, hrow i⟩
  have hL : L < 1 := by
    refine (Finset.sup_lt_iff (by simp)).2 fun i _ => ?_
    have h1 : ((⟨_, hrow i⟩ : NNReal) : ℝ) < ((1 : NNReal) : ℝ) := by
      change (∑ j, R.mag i j * R.width j) / R.width i < 1
      rw [div_lt_one (hw i)]
      exact sum_mag_mul_width_lt hA hacc hp i
    exact NNReal.coe_lt_coe.1 h1
  have hLi : ∀ i, (∑ j, R.mag i j * R.width j) / R.width i ≤ (L : ℝ) := fun i => by
    have : (⟨(∑ j, R.mag i j * R.width j) / R.width i, hrow i⟩ : NNReal) ≤ L :=
      Finset.le_sup (f := fun i => (⟨(∑ j, R.mag i j * R.width j) / R.width i, hrow i⟩ : NNReal))
        (Finset.mem_univ i)
    exact_mod_cast this
  have hLip : LipschitzWith L H := by
    refine LipschitzWith.of_dist_le_mul fun u v => ?_
    rw [Subtype.dist_eq, Subtype.dist_eq]
    refine (dist_pi_le_iff (by positivity)).2 fun i => ?_
    simp only [H, Real.dist_eq]
    rw [← sub_div, abs_div, abs_of_pos (hw i), div_le_iff₀ (hw i)]
    calc |R.g p (fun j => R.width j * u.1 j) i - R.g p (fun j => R.width j * v.1 j) i|
        ≤ ∑ j, R.mag i j * |R.width j * u.1 j - R.width j * v.1 j| := abs_g_sub_le hA hp u.2 v.2 i
      _ = ∑ j, R.mag i j * R.width j * dist (u.1 j) (v.1 j) := by
          refine Finset.sum_congr rfl fun j _ => ?_
          rw [← mul_sub, abs_mul, abs_of_pos (hw j), Real.dist_eq]
          ring
      _ ≤ ∑ j, R.mag i j * R.width j * dist u.1 v.1 :=
          Finset.sum_le_sum fun j _ => mul_le_mul_of_nonneg_left (dist_le_pi_dist _ _ j)
            (mul_nonneg (R.mag_nonneg i j) (hw j).le)
      _ = (∑ j, R.mag i j * R.width j) / R.width i * dist u.1 v.1 * R.width i := by
          rw [← Finset.sum_mul, div_mul_eq_mul_div, div_mul_cancel₀ _ (hw i).ne']
      _ ≤ L * dist u.1 v.1 * R.width i :=
          mul_le_mul_of_nonneg_right (mul_le_mul_of_nonneg_right (hLi i) dist_nonneg) (hw i).le
  have hC : ContractingWith L H := ⟨hL, hLip⟩
  set u := ContractingWith.fixedPoint H hC
  have hfix : H u = u := ContractingWith.fixedPoint_isFixedPt hC
  refine ⟨fun j => R.width j * u.1 j, u.2, ?_⟩
  funext j
  have hj := congrArg (fun v : B => v.1 j) hfix
  simp only [H] at hj
  rw [div_eq_iff (hw j).ne'] at hj
  rw [hj, mul_comm]

/-- Soundness of the parametric Krawczyk test. When `sketch.certSystem.krawczyk` accepts, that is when every row
of `K(X)` lies strictly inside the same row of `X` (`sketch.Interval.strictlyInside`), then for every parameter
value `p ∈ Q` (the driving value jointly with every ranged target and fixed box) exactly one zero of `F(·, p)` lies
in `X`, and every zero of `F(·, p)` in `X` lies in `K`. Covers `sketch.certSystem.krawczyk` and
`sketch.Interval.strictlyInside`. The hypotheses carry assumptions A10 to A14 (`hA`); the square shape of `F` is
A15. Continuity of the zero in `p` is not proved. -/
theorem krawczyk_sound (hA : R.Assumptions) (hacc : R.accepts) {p : P} (hp : p ∈ R.Q) :
    (∃! x, inBox R.xLo R.xHi x ∧ R.F x p = 0) ∧
      ∀ x, inBox R.xLo R.xHi x → R.F x p = 0 → inBox R.kLo R.kHi x := by
  have hfixF : ∀ x, R.F x p = 0 → R.g p x = x := fun x h => by simp [g, h]
  have hYdet : R.Y.det ≠ 0 := by
    have := det_Y_mul_Fx_ne_zero hA hacc hp hA.xt_mem
    rw [Matrix.det_mul] at this
    exact left_ne_zero_of_mul this
  refine ⟨?_, fun x hx hF i => ?_⟩
  · obtain ⟨x, hx, hgx⟩ := exists_g_fixed hA hacc hp
    refine ⟨x, ⟨hx, ?_⟩, fun y ⟨hy, hFy⟩ => ?_⟩
    · have hYF : R.Y *ᵥ R.F x p = 0 := by
        have : x - R.Y *ᵥ R.F x p = x := hgx
        simpa using this
      by_contra hne
      exact hYdet (Matrix.exists_mulVec_eq_zero_iff.1 ⟨_, hne, hYF⟩)
    · have hd : y - x = 0 := eq_zero_of_abs_le hA hacc hp fun i => by
        have := abs_g_sub_le hA hp hy hx i
        rw [hfixF y hFy, hgx] at this
        simpa using this
      exact sub_eq_zero.1 hd
  · have := g_mem_image hA hp hx i
    rwa [hfixF x hF] at this

/-- The strict inclusion also proves the approximate inverse `Y` and every Jacobian `F_x(x, p)` with `x ∈ X`,
`p ∈ Q` nonsingular. Covers `sketch.certSystem.krawczyk`; the hypotheses carry assumptions A10 to A14 (`hA`). -/
theorem krawczyk_nonsingular (hA : R.Assumptions) (hacc : R.accepts) {p : P} (hp : p ∈ R.Q) :
    R.Y.det ≠ 0 ∧ ∀ x, inBox R.xLo R.xHi x → (R.Fx x p).det ≠ 0 := by
  refine ⟨?_, fun x hx => ?_⟩
  · have := det_Y_mul_Fx_ne_zero hA hacc hp hA.xt_mem
    rw [Matrix.det_mul] at this
    exact left_ne_zero_of_mul this
  · have := det_Y_mul_Fx_ne_zero hA hacc hp hx
    rw [Matrix.det_mul] at this
    exact right_ne_zero_of_mul this

end KrawczykRun

end Sketch.Enclose

end
