module

public import Sketch.Cone.Poly

/-!
# Bernstein enclosure of a polynomial

Proves the bound `sketch.coneFitCoeffBounds` (`cone_fit_crossing.go`) publishes for a polynomial of any degree,
and models `sketch.coneFitPolyDeriv`.

`toPoly` reads a coefficient list as a Mathlib polynomial over `ℚ`, and `evalR` evaluates it at a real
parameter. The list helpers of `Sketch/Cone/Poly.lean` match polynomial addition, subtraction, scaling and
multiplication (`toPoly_add` and the rest), and `toPoly_coneFitPolyDeriv` shows `coneFitPolyDeriv` is the
derivative.

`coneFitBinomial_eq` proves the integer recurrence of `coneFitBinomial` gives `Nat.choose`. `eval_eq_sum_bw`
proves the power-to-Bernstein conversion `coneFitBernstein` uses: a polynomial of degree at most `n` is
`∑ₖ bₖ·C(n,k)·uᵏ·(1−u)ⁿ⁻ᵏ` with `bₖ = ∑_{i≤k} qᵢ·C(k,i)/C(n,i)`. The Bernstein weights are nonnegative on
`[0, 1]` and sum to 1, so `coneFitCoeffBounds_mem` proves every value of `p` on `[a, b]` lies between the least
and greatest coefficient.
-/

public section

namespace Sketch.Cone

open Polynomial

/-- The polynomial with coefficient list `a`, lowest degree first. -/
@[expose] noncomputable def toPoly : List ℚ → ℚ[X]
  | [] => 0
  | c :: a => C c + X * toPoly a

theorem toPoly_eval (a : List ℚ) (t : ℚ) : (toPoly a).eval t = coneFitPolyEval a t := by
  induction a with
  | nil => simp [toPoly]
  | cons c a ih => simp only [toPoly, eval_add, eval_C, eval_mul, eval_X, ih, coneFitPolyEval_cons]; ring

theorem toPoly_coeff (a : List ℚ) (i : ℕ) : (toPoly a).coeff i = a.getD i 0 := by
  induction a generalizing i with
  | nil => simp [toPoly]
  | cons c a ih =>
    cases i with
    | zero => simp [toPoly]
    | succ i => simp [toPoly, coeff_X_mul, ih]

theorem toPoly_add (a b : List ℚ) : toPoly (coneFitPolyAdd a b) = toPoly a + toPoly b :=
  Polynomial.funext fun t => by simp only [eval_add, toPoly_eval, coneFitPolyEval_add]

theorem toPoly_sub (a b : List ℚ) : toPoly (coneFitPolySub a b) = toPoly a - toPoly b :=
  Polynomial.funext fun t => by simp only [eval_sub, toPoly_eval, coneFitPolyEval_sub]

theorem toPoly_scale (a : List ℚ) (k : ℚ) : toPoly (coneFitPolyScale a k) = toPoly a * C k :=
  Polynomial.funext fun t => by simp only [eval_mul, eval_C, toPoly_eval, coneFitPolyEval_scale]

theorem toPoly_mul (a b : List ℚ) : toPoly (coneFitPolyMul a b) = toPoly a * toPoly b :=
  Polynomial.funext fun t => by simp only [eval_mul, toPoly_eval, coneFitPolyEval_mul]

/-- The degree of `toPoly a` is less than the length of `a`. -/
theorem toPoly_natDegree_le (a : List ℚ) : (toPoly a).natDegree ≤ a.length - 1 := by
  rw [natDegree_le_iff_coeff_eq_zero]
  intro N hN
  rw [toPoly_coeff, List.getD_eq_default]
  omega

/-- Evaluation of the coefficient list `a` at a real parameter. -/
@[expose] noncomputable def evalR (a : List ℚ) (t : ℝ) : ℝ := ((toPoly a).map (algebraMap ℚ ℝ)).eval t

@[simp] theorem evalR_nil (t : ℝ) : evalR [] t = 0 := by simp [evalR, toPoly]

@[simp] theorem evalR_cons (c : ℚ) (a : List ℚ) (t : ℝ) : evalR (c :: a) t = evalR a t * t + c := by
  simp only [evalR, toPoly, Polynomial.map_add, Polynomial.map_mul, map_C, map_X, eval_add, eval_C,
    eval_mul, eval_X, eq_ratCast]
  ring

theorem evalR_add (a b : List ℚ) (t : ℝ) : evalR (coneFitPolyAdd a b) t = evalR a t + evalR b t := by
  simp only [evalR, toPoly_add, Polynomial.map_add, eval_add]

theorem evalR_sub (a b : List ℚ) (t : ℝ) : evalR (coneFitPolySub a b) t = evalR a t - evalR b t := by
  simp only [evalR, toPoly_sub, Polynomial.map_sub, eval_sub]

theorem evalR_scale (a : List ℚ) (k : ℚ) (t : ℝ) : evalR (coneFitPolyScale a k) t = evalR a t * k := by
  simp only [evalR, toPoly_scale, Polynomial.map_mul, map_C, eval_mul, eval_C, eq_ratCast]

theorem evalR_mul (a b : List ℚ) (t : ℝ) : evalR (coneFitPolyMul a b) t = evalR a t * evalR b t := by
  simp only [evalR, toPoly_mul, Polynomial.map_mul, eval_mul]

/-- At a rational parameter, `evalR` is the exact Horner value `coneFitPolyEval`. -/
theorem evalR_ratCast (a : List ℚ) (t : ℚ) : evalR a t = (coneFitPolyEval a t : ℝ) := by
  induction a with
  | nil => simp
  | cons c a ih => rw [evalR_cons, ih, coneFitPolyEval_cons]; push_cast; ring

theorem continuous_evalR (a : List ℚ) : Continuous (evalR a) :=
  ((toPoly a).map (algebraMap ℚ ℝ)).continuous

/-- Model of `sketch.coneFitPolyDeriv`: `r[i] = a[i+1]·(i+1)`, or `[0]` for a constant. -/
@[expose] def coneFitPolyDeriv (a : List ℚ) : List ℚ :=
  if a.length < 2 then [0]
  else (List.range (a.length - 1)).map fun i => a.getD (i + 1) 0 * ((i + 1 : ℕ) : ℚ)

/-- `coneFitPolyDeriv` is the derivative. -/
theorem toPoly_coneFitPolyDeriv (a : List ℚ) : toPoly (coneFitPolyDeriv a) = derivative (toPoly a) := by
  ext n
  rw [coeff_derivative, toPoly_coeff, toPoly_coeff, coneFitPolyDeriv]
  split_ifs with h
  · rw [List.getD_eq_default a 0 (show a.length ≤ n + 1 by omega)]
    cases n <;> simp
  · by_cases hn : n < a.length - 1
    · simp [List.getD_eq_getElem?_getD, hn]
    · rw [List.getD_eq_default _ _ (by simp; omega), List.getD_eq_default _ _ (by omega)]
      simp

/-- The real derivative of `evalR a` is `evalR (coneFitPolyDeriv a)`. -/
theorem hasDerivAt_evalR (a : List ℚ) (t : ℝ) : HasDerivAt (evalR a) (evalR (coneFitPolyDeriv a) t) t := by
  have h := ((toPoly a).map (algebraMap ℚ ℝ)).hasDerivAt t
  rw [derivative_map, ← toPoly_coneFitPolyDeriv] at h
  exact h

theorem coneFitBinomial_foldl (n k j : ℕ) (hj : j ≤ k) :
    (List.range j).foldl (fun r i => r * (n - k + (i + 1)) / (i + 1)) 1 = (n - k + j).choose j := by
  induction j with
  | zero => simp
  | succ j ih =>
    rw [List.range_succ, List.foldl_append, List.foldl_cons, List.foldl_nil, ih (by omega)]
    have e := Nat.add_one_mul_choose_eq (n - k + j) j
    rw [show n - k + (j + 1) = n - k + j + 1 by omega, Nat.mul_comm, e,
      Nat.mul_div_cancel _ (Nat.succ_pos j)]

/-- `coneFitBinomial n k` is `C(n, k)` for `k ≤ n` (the range A32 states). -/
theorem coneFitBinomial_eq (n k : ℕ) (hk : k ≤ n) : coneFitBinomial n k = n.choose k := by
  rw [coneFitBinomial, coneFitBinomial_foldl n k k le_rfl, Nat.sub_add_cancel hk]

/-- The Bernstein weight `C(n,k)·uᵏ·(1−u)ⁿ⁻ᵏ`. -/
@[expose] noncomputable def bw (n k : ℕ) (u : ℝ) : ℝ := (n.choose k : ℝ) * u ^ k * (1 - u) ^ (n - k)

theorem bw_sum (n : ℕ) (u : ℝ) : ∑ k ∈ Finset.range (n + 1), bw n k u = 1 := by
  have h := add_pow u (1 - u) n
  rw [add_sub_cancel, one_pow] at h
  rw [h]
  exact Finset.sum_congr rfl fun k _ => by simp only [bw]; ring

theorem bw_nonneg (n k : ℕ) {u : ℝ} (hu0 : 0 ≤ u) (hu1 : u ≤ 1) : 0 ≤ bw n k u := by
  have : 0 ≤ 1 - u := by linarith
  unfold bw
  positivity

/-- A monomial `uⁱ` with `i ≤ n` in the degree-`n` Bernstein basis. -/
theorem pow_eq_sum_bw (n i : ℕ) (hi : i ≤ n) (u : ℝ) :
    u ^ i = ∑ k ∈ Finset.range (n + 1), ((k.choose i : ℝ) / (n.choose i : ℝ)) * bw n k u := by
  have hn : (n.choose i : ℝ) ≠ 0 := by exact_mod_cast (Nat.choose_pos hi).ne'
  have hterm : ∀ k ∈ Finset.range (n + 1), ((k.choose i : ℝ) / n.choose i) * bw n k u =
      if i ≤ k then ((n - i).choose (k - i) : ℝ) * u ^ k * (1 - u) ^ (n - k) else 0 := by
    intro k hk
    split_ifs with hik
    · have hc : (n.choose k : ℝ) * k.choose i = n.choose i * (n - i).choose (k - i) := by
        exact_mod_cast Nat.choose_mul hik
      rw [div_mul_eq_mul_div, div_eq_iff hn]
      unfold bw
      linear_combination (u ^ k * (1 - u) ^ (n - k)) * hc
    · rw [Nat.choose_eq_zero_of_lt (not_le.mp hik)]
      simp
  rw [Finset.sum_congr rfl hterm, ← Finset.sum_range_add_sum_Ico _ (show i ≤ n + 1 by omega),
    Finset.sum_eq_zero (fun k hk => ite_eq_right (by simp only [Finset.mem_range] at hk; omega)), zero_add,
    Finset.sum_Ico_eq_sum_range, show n + 1 - i = n - i + 1 by omega]
  have hj : ∀ j ∈ Finset.range (n - i + 1),
      (if i ≤ i + j then ((n - i).choose (i + j - i) : ℝ) * u ^ (i + j) * (1 - u) ^ (n - (i + j)) else 0) =
        u ^ i * (u ^ j * (1 - u) ^ (n - i - j) * (n - i).choose j) := by
    intro j _
    rw [ite_eq_left (Nat.le_add_right i j), Nat.add_sub_cancel_left, show n - (i + j) = n - i - j by omega,
      pow_add]
    ring
  rw [Finset.sum_congr rfl hj, ← Finset.mul_sum, ← add_pow, add_sub_cancel, one_pow, mul_one]

/-- Power-to-Bernstein conversion: a real polynomial of degree at most `n` is the Bernstein combination of the
coefficients `bₖ = ∑_{i≤k} P.coeff i · C(k,i)/C(n,i)`. -/
theorem eval_eq_sum_bw (P : ℝ[X]) (n : ℕ) (hP : P.natDegree ≤ n) (u : ℝ) :
    P.eval u = ∑ k ∈ Finset.range (n + 1),
      (∑ i ∈ Finset.range (k + 1), P.coeff i * ((k.choose i : ℝ) / (n.choose i : ℝ))) * bw n k u := by
  rw [eval_eq_sum_range' (Nat.lt_succ_of_le hP)]
  have h1 : ∀ i ∈ Finset.range (n + 1), P.coeff i * u ^ i =
      ∑ k ∈ Finset.range (n + 1), P.coeff i * ((k.choose i : ℝ) / n.choose i) * bw n k u := by
    intro i hi
    rw [pow_eq_sum_bw n i (by simp only [Finset.mem_range] at hi; omega) u, Finset.mul_sum]
    simp only [mul_assoc]
  rw [Finset.sum_congr rfl h1, Finset.sum_comm]
  refine Finset.sum_congr rfl fun k hk => ?_
  rw [Finset.sum_mul]
  symm
  apply Finset.sum_subset (Finset.range_subset_range.mpr (by simp only [Finset.mem_range] at hk; omega))
  intro i hi hni
  simp only [Finset.mem_range, not_lt] at hi hni
  rw [Nat.choose_eq_zero_of_lt (by omega)]
  simp

theorem foldl_add_range (f : ℕ → ℚ) (m : ℕ) (c : ℚ) :
    (List.range m).foldl (fun acc i => acc + f i) c = c + ∑ i ∈ Finset.range m, f i := by
  induction m with
  | zero => simp
  | succ m ih => rw [List.range_succ, List.foldl_append, ih, Finset.sum_range_succ]; simp; ring

theorem coneFitPolyEval_eq_sum (a : List ℚ) (m : ℕ) (hm : a.length ≤ m) (t : ℚ) :
    coneFitPolyEval a t = ∑ i ∈ Finset.range m, a.getD i 0 * t ^ i := by
  induction a generalizing m with
  | nil => simp
  | cons c a ih =>
    obtain ⟨m, rfl⟩ : ∃ m', m = m' + 1 := ⟨m - 1, by simp at hm; omega⟩
    rw [coneFitPolyEval_cons, ih m (by simp at hm; omega), Finset.sum_range_succ', Finset.sum_mul]
    simp [pow_succ, mul_assoc]

/-- The Horner composition `p(a + (b−a)·u)` `sketch.coneFitBernstein` builds before the basis change. -/
@[expose] def bernsteinQ (p : List ℚ) (a b : ℚ) : List ℚ :=
  (List.range (p.length - 1 + 1)).reverse.foldl
    (fun q i => coneFitPolyAdd (coneFitPolyMul q [a, b - a]) [p.getD i 0]) [0]

theorem coneFitBernstein_eq (p : List ℚ) (a b : ℚ) :
    coneFitBernstein p a b = (List.range (p.length - 1 + 1)).map fun k =>
      (List.range (k + 1)).foldl (fun acc i => acc + (bernsteinQ p a b).getD i 0 *
        ((coneFitBinomial k i : ℚ) / (coneFitBinomial (p.length - 1) i : ℚ))) 0 := rfl

theorem bernsteinQ_foldl (p : List ℚ) (a b t : ℚ) (m : ℕ) (q0 : List ℚ) :
    coneFitPolyEval ((List.range m).reverse.foldl
      (fun q i => coneFitPolyAdd (coneFitPolyMul q [a, b - a]) [p.getD i 0]) q0) t =
      coneFitPolyEval q0 t * (a + (b - a) * t) ^ m +
        ∑ i ∈ Finset.range m, p.getD i 0 * (a + (b - a) * t) ^ i := by
  induction m generalizing q0 with
  | zero => simp
  | succ m ih =>
    rw [List.range_succ, List.reverse_append, List.reverse_singleton, List.singleton_append, List.foldl_cons,
      ih, coneFitPolyEval_add, coneFitPolyEval_mul, Finset.sum_range_succ]
    simp only [coneFitPolyEval_cons, coneFitPolyEval_nil]
    ring

theorem bernsteinQ_eval (p : List ℚ) (a b t : ℚ) :
    coneFitPolyEval (bernsteinQ p a b) t = coneFitPolyEval p (a + (b - a) * t) := by
  rw [bernsteinQ, bernsteinQ_foldl, coneFitPolyEval_eq_sum p (p.length - 1 + 1) (by omega)]
  simp

theorem toPoly_bernsteinQ (p : List ℚ) (a b : ℚ) :
    toPoly (bernsteinQ p a b) = (toPoly p).comp (C (b - a) * X + C a) :=
  Polynomial.funext fun t => by
    rw [eval_comp, toPoly_eval, toPoly_eval, bernsteinQ_eval]
    simp only [eval_add, eval_mul, eval_C, eval_X]
    ring_nf

theorem evalR_bernsteinQ (p : List ℚ) (a b : ℚ) (u : ℝ) :
    evalR (bernsteinQ p a b) u = evalR p (a + (b - a) * u) := by
  simp only [evalR, toPoly_bernsteinQ, Polynomial.map_comp, eval_comp, Polynomial.map_add,
    Polynomial.map_mul, map_C, map_X, eval_add, eval_mul, eval_C, eval_X, eq_ratCast]
  push_cast
  ring_nf

theorem bernsteinQ_natDegree (p : List ℚ) (a b : ℚ) :
    (toPoly (bernsteinQ p a b)).natDegree ≤ p.length - 1 := by
  rw [toPoly_bernsteinQ]
  refine le_trans natDegree_comp_le ?_
  have h1 := toPoly_natDegree_le p
  have h2 : (C (b - a) * X + C a).natDegree ≤ 1 := natDegree_linear_le
  calc (toPoly p).natDegree * (C (b - a) * X + C a).natDegree ≤ (p.length - 1) * 1 :=
        Nat.mul_le_mul h1 h2
    _ = p.length - 1 := Nat.mul_one _

/-- `p(a + (b−a)·u)` is the Bernstein combination of the coefficients `coneFitBernstein p a b` lists. -/
theorem coneFitBernstein_sum (p : List ℚ) (a b : ℚ) (u : ℝ) :
    ∃ g : ℕ → ℚ, coneFitBernstein p a b = (List.range (p.length - 1 + 1)).map g ∧
      evalR p (a + (b - a) * u) = ∑ k ∈ Finset.range (p.length - 1 + 1), (g k : ℝ) * bw (p.length - 1) k u := by
  refine ⟨_, coneFitBernstein_eq p a b, ?_⟩
  set n := p.length - 1
  set q := bernsteinQ p a b
  set Q := (toPoly q).map (algebraMap ℚ ℝ)
  have hQ : Q.natDegree ≤ n := le_trans (natDegree_map_le) (bernsteinQ_natDegree p a b)
  rw [← evalR_bernsteinQ]
  change Q.eval u = _
  rw [eval_eq_sum_bw Q n hQ u]
  refine Finset.sum_congr rfl fun k hk => ?_
  have hkn : k ≤ n := by simp only [Finset.mem_range] at hk; omega
  rw [foldl_add_range, zero_add]
  push_cast
  congr 1
  refine Finset.sum_congr rfl fun i hi => ?_
  have hik : i ≤ k := by simp only [Finset.mem_range] at hi; omega
  rw [coneFitBinomial_eq k i hik, coneFitBinomial_eq n i (le_trans hik hkn)]
  simp [Q, coeff_map, toPoly_coeff]

theorem bounds_foldl (l : List ℚ) (lo hi : ℚ) :
    (l.foldl (fun acc v => (if v < acc.1 then v else acc.1, if acc.2 < v then v else acc.2)) (lo, hi)).1 ≤ lo ∧
      hi ≤ (l.foldl (fun acc v => (if v < acc.1 then v else acc.1, if acc.2 < v then v else acc.2)) (lo, hi)).2 ∧
      ∀ v ∈ l,
        (l.foldl (fun acc v => (if v < acc.1 then v else acc.1, if acc.2 < v then v else acc.2)) (lo, hi)).1 ≤ v ∧
        v ≤ (l.foldl (fun acc v => (if v < acc.1 then v else acc.1, if acc.2 < v then v else acc.2)) (lo, hi)).2 := by
  induction l generalizing lo hi with
  | nil => simp
  | cons x xs ih =>
    simp only [List.foldl_cons, List.mem_cons, forall_eq_or_imp]
    obtain ⟨h1, h2, h3⟩ := ih (if x < lo then x else lo) (if hi < x then x else hi)
    refine ⟨le_trans h1 (by split_ifs <;> linarith), le_trans (by split_ifs <;> linarith) h2,
      ⟨le_trans h1 (by split_ifs <;> linarith), le_trans (by split_ifs <;> linarith) h2⟩, h3⟩

/-- Every coefficient of `coneFitBernstein p a b` lies between the two bounds `coneFitCoeffBounds` returns. -/
theorem coneFitCoeffBounds_le (p : List ℚ) (a b : ℚ) :
    ∀ v ∈ coneFitBernstein p a b, (coneFitCoeffBounds p a b).1 ≤ v ∧ v ≤ (coneFitCoeffBounds p a b).2 := by
  unfold coneFitCoeffBounds
  generalize coneFitBernstein p a b = l
  cases l with
  | nil => simp
  | cons x xs =>
    simp only [List.tail_cons, List.headD_cons, List.mem_cons, forall_eq_or_imp]
    obtain ⟨h1, h2, h3⟩ := bounds_foldl xs x x
    exact ⟨⟨h1, h2⟩, h3⟩

/-- Bernstein enclosure: for every `u ∈ [0, 1]`, the value `p(a + (b−a)·u)` lies between the least and greatest
Bernstein coefficient `coneFitCoeffBounds p a b` returns, for a polynomial `p` of any degree. Covers
`sketch.coneFitCoeffBounds`, `sketch.coneFitBernstein`, `sketch.coneFitBinomial` and the `coneFitPoly`
arithmetic helpers; assumptions A30, A32. -/
theorem coneFitCoeffBounds_mem (p : List ℚ) (a b : ℚ) {u : ℝ} (hu0 : 0 ≤ u) (hu1 : u ≤ 1) :
    ((coneFitCoeffBounds p a b).1 : ℝ) ≤ evalR p (a + (b - a) * u) ∧
      evalR p (a + (b - a) * u) ≤ (coneFitCoeffBounds p a b).2 := by
  obtain ⟨g, hg, hsum⟩ := coneFitBernstein_sum p a b u
  set n := p.length - 1
  have hb : ∀ k ∈ Finset.range (n + 1),
      ((coneFitCoeffBounds p a b).1 : ℝ) ≤ g k ∧ (g k : ℝ) ≤ (coneFitCoeffBounds p a b).2 := by
    intro k hk
    have := coneFitCoeffBounds_le p a b (g k) (by rw [hg]; exact List.mem_map.mpr ⟨k, by simpa using hk, rfl⟩)
    exact ⟨by exact_mod_cast this.1, by exact_mod_cast this.2⟩
  have hw := bw_sum n u
  rw [hsum]
  constructor
  · calc ((coneFitCoeffBounds p a b).1 : ℝ)
        = ∑ k ∈ Finset.range (n + 1), ((coneFitCoeffBounds p a b).1 : ℝ) * bw n k u := by
          rw [← Finset.mul_sum, hw, mul_one]
      _ ≤ _ := Finset.sum_le_sum fun k hk => mul_le_mul_of_nonneg_right (hb k hk).1 (bw_nonneg n k hu0 hu1)
  · calc _ ≤ ∑ k ∈ Finset.range (n + 1), ((coneFitCoeffBounds p a b).2 : ℝ) * bw n k u :=
          Finset.sum_le_sum fun k hk => mul_le_mul_of_nonneg_right (hb k hk).2 (bw_nonneg n k hu0 hu1)
      _ = _ := by rw [← Finset.mul_sum, hw, mul_one]

/-- The Bernstein enclosure on the real interval `[a, b]`: every value of `p` there lies between the two
bounds `coneFitCoeffBounds p a b` returns. -/
theorem coneFitCoeffBounds_mem_Icc (p : List ℚ) {a b : ℚ} (hab : a ≤ b) {t : ℝ} (ht0 : (a : ℝ) ≤ t)
    (ht1 : t ≤ b) :
    ((coneFitCoeffBounds p a b).1 : ℝ) ≤ evalR p t ∧ evalR p t ≤ (coneFitCoeffBounds p a b).2 := by
  rcases eq_or_lt_of_le hab with h | h
  · subst h
    have ht : t = (a : ℝ) + ((a : ℝ) - a) * 0 := by linarith
    rw [ht]
    exact coneFitCoeffBounds_mem p a a le_rfl zero_le_one
  · have hba : (0 : ℝ) < (b : ℝ) - a := by exact_mod_cast sub_pos.mpr h
    have ht : t = (a : ℝ) + ((b : ℝ) - a) * ((t - a) / (b - a)) := by
      field_simp; ring
    rw [ht]
    refine coneFitCoeffBounds_mem p a b (div_nonneg (by linarith) hba.le) ?_
    rw [div_le_one hba]; linarith

end Sketch.Cone

end
