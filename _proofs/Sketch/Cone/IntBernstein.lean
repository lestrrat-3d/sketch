module

public import Sketch.Cone.Bernstein

/-!
# Integer Bernstein bounds

Models `sketch.coneFitIntPoly`, `sketch.coneFitFactorial`, `sketch.coneFitIntBernstein` and
`sketch.coneFitIntBounds` (`cone_fit_crossing.go`), the bounds `sketch.coneFitSearch.visit` and the final
derivative check of `sketch.Sketch.CertifyConeFitCrossing` read.

`coneFitIntPoly` scales a rational polynomial to integers by the least common multiple `D` of its
denominators. `coneFitIntBernstein P a b` composes `q(u) = Σ P[i]·(l + w·u)^i·d^(n−i)` with `a = l/d`,
`b = h/d`, `w = h − l`, `d = den(a)·den(b)`, then converts to degree-`n` Bernstein coefficients with the integer
weights `C(k,i)·i!·(n−i)! = n!·C(k,i)/C(n,i)`. `coneFitIntBernstein_sum` proves the Bernstein combination of the
result is `n!·d^(n+1)/d` times `P(a + (b−a)·u)`, a positive multiple, so `coneFitIntBounds_mem` gives one positive
`s` with every value of `s·p` on `[a, b]` between the least and greatest coefficient: the signs `visit` reads are
the signs of the exact Bernstein bounds. Integer values are modeled as the rationals they equal (A110).
-/

public section

namespace Sketch.Cone

open Polynomial

/-- Model of the denominator loop of `sketch.coneFitIntPoly`: `d ← d·(den / gcd(d, den))` from `d = 1`, the
least common multiple of the denominators (A100). -/
@[expose] def coneFitIntDen (p : List ℚ) : ℕ := p.foldl (fun d c => d * (c.den / Nat.gcd d c.den)) 1

/-- Model of `sketch.coneFitIntPoly`: coefficient `i` is `num·(D / den)` of `p[i]`, with `D = coneFitIntDen p`
(A100). -/
@[expose] def coneFitIntPoly (p : List ℚ) : List ℤ :=
  p.map fun c => c.num * ((coneFitIntDen p / c.den : ℕ) : ℤ)

theorem coneFitPolyEval_map_mul (p : List ℚ) (D t : ℚ) :
    coneFitPolyEval (p.map (· * D)) t = coneFitPolyEval p t * D := by
  induction p with
  | nil => simp
  | cons c p ih => simp only [List.map_cons, coneFitPolyEval_cons, ih]; ring

theorem coneFitIntDen_foldl (p : List ℚ) (d : ℕ) (hd : 0 < d) :
    0 < p.foldl (fun d c => d * (c.den / Nat.gcd d c.den)) d ∧
      d ∣ p.foldl (fun d c => d * (c.den / Nat.gcd d c.den)) d ∧
      ∀ c ∈ p, c.den ∣ p.foldl (fun d c => d * (c.den / Nat.gcd d c.den)) d := by
  induction p generalizing d with
  | nil => simp [hd]
  | cons c p ih =>
    have hstep : d * (c.den / Nat.gcd d c.den) = Nat.lcm d c.den :=
      (Nat.mul_div_assoc d (Nat.gcd_dvd_right d c.den)).symm
    simp only [List.foldl_cons, hstep, List.mem_cons, forall_eq_or_imp]
    obtain ⟨h1, h2, h3⟩ := ih (Nat.lcm d c.den) (Nat.lcm_pos hd c.den_pos)
    exact ⟨h1, (Nat.dvd_lcm_left d c.den).trans h2, (Nat.dvd_lcm_right d c.den).trans h2, h3⟩

theorem coneFitIntPoly_cast (p : List ℚ) :
    (coneFitIntPoly p).map (Int.cast : ℤ → ℚ) = p.map (· * (coneFitIntDen p : ℚ)) := by
  obtain ⟨-, -, hdvd⟩ := coneFitIntDen_foldl p 1 one_pos
  rw [coneFitIntPoly, List.map_map]
  refine List.map_congr_left fun c hc => ?_
  have hc' : c.den ∣ coneFitIntDen p := hdvd c hc
  simp only [Function.comp_apply, Int.cast_mul, Int.cast_natCast]
  rw [Nat.cast_div hc' (by exact_mod_cast c.den_nz)]
  have hden : (c.den : ℚ) ≠ 0 := by exact_mod_cast c.den_nz
  conv_rhs => rw [← Rat.num_div_den c]
  field_simp


/-- Model of `sketch.coneFitFactorial`: `r ← r·i` for `i = 1, …, n` from `r = 1` (A110). -/
@[expose] def coneFitFactorial (n : ℕ) : ℕ := (List.range n).foldl (fun r i => r * (i + 1)) 1

theorem coneFitFactorial_eq (n : ℕ) : coneFitFactorial n = n.factorial := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp only [coneFitFactorial, List.range_succ, List.foldl_append, List.foldl_cons, List.foldl_nil] at ih ⊢
    rw [ih, Nat.factorial_succ, Nat.mul_comm]

/-- One pass of the composition loop of `sketch.coneFitIntBernstein`: `next[j] += q[j]·l`,
`next[j+1] += q[j]·w`, `next[0] += c·pw`, then `pw ← pw·d`. -/
@[expose] def coneFitIntStep (l w d c : ℚ) (s : List ℚ × ℚ) : List ℚ × ℚ :=
  (coneFitPolyAdd (coneFitPolyAdd (coneFitPolyScale s.1 l) (0 :: coneFitPolyScale s.1 w)) [c * s.2], s.2 * d)

/-- The composition loop of `sketch.coneFitIntBernstein`, from the highest coefficient: `(q, pw)` from `([], 1)`. -/
@[expose] def coneFitIntCompose (P : List ℚ) (l w d : ℚ) : List ℚ × ℚ := P.foldr (coneFitIntStep l w d) ([], 1)

/-- Model of `sketch.coneFitIntBernstein` for a nonempty `P` (Go indexes `b[0]` of the result): with
`l = num(a)·den(b)`, `w = num(b)·den(a) − l` and `d = den(a)·den(b)`, the composition `q`, then entry `k` is
`Σ_{i≤k} q[i]·C(k,i)·i!·(n−i)!` (A110, A111). -/
@[expose] def coneFitIntBernstein (P : List ℤ) (a b : ℚ) : List ℚ :=
  let n := P.length - 1
  let q := (coneFitIntCompose (P.map (Int.cast : ℤ → ℚ)) ((a.num : ℚ) * b.den)
    ((b.num : ℚ) * a.den - (a.num : ℚ) * b.den) ((a.den : ℚ) * b.den)).1
  (List.range (n + 1)).map fun k =>
    (List.range (k + 1)).foldl (fun acc i =>
      acc + q.getD i 0 * ((coneFitBinomial k i * coneFitFactorial i * coneFitFactorial (n - i) : ℕ) : ℚ)) 0

/-- Model of `sketch.coneFitIntBounds`: the least and greatest entry, in the Go loop order. -/
@[expose] def coneFitIntBounds (B : List ℚ) : ℚ × ℚ :=
  B.tail.foldl (fun acc v => (if v < acc.1 then v else acc.1, if acc.2 < v then v else acc.2))
    (B.headD 0, B.headD 0)

theorem coneFitPolyAdd_length (a b : List ℚ) : (coneFitPolyAdd a b).length = max a.length b.length := by
  induction a generalizing b with
  | nil => simp [coneFitPolyAdd]
  | cons x xs ih =>
    cases b with
    | nil => simp [coneFitPolyAdd]
    | cons y ys => simp [coneFitPolyAdd, ih]

theorem coneFitIntCompose_snd (P : List ℚ) (l w d : ℚ) : (coneFitIntCompose P l w d).2 = d ^ P.length := by
  induction P with
  | nil => simp [coneFitIntCompose]
  | cons c P ih =>
    simp only [coneFitIntCompose, List.foldr_cons, coneFitIntStep, List.length_cons] at ih ⊢
    rw [ih, pow_succ]

theorem coneFitIntCompose_length (P : List ℚ) (l w d : ℚ) :
    (coneFitIntCompose P l w d).1.length = P.length := by
  induction P with
  | nil => simp [coneFitIntCompose]
  | cons c P ih =>
    simp only [coneFitIntCompose, List.foldr_cons, coneFitIntStep, coneFitPolyAdd_length, coneFitPolyScale,
      List.length_map, List.length_cons, List.length_nil] at ih ⊢
    rw [ih]
    omega

theorem coneFitIntCompose_eval (P : List ℚ) (l w d : ℚ) (hd : d ≠ 0) (u : ℝ) :
    evalR (coneFitIntCompose P l w d).1 u * d = (d : ℝ) ^ P.length * evalR P ((l + w * u) / d) := by
  have hdR : (d : ℝ) ≠ 0 := by exact_mod_cast hd
  have hx : (d : ℝ) * ((l + w * u) / d) = l + w * u := mul_div_cancel₀ _ hdR
  induction P with
  | nil => simp [coneFitIntCompose]
  | cons c P ih =>
    have hs := coneFitIntCompose_snd P l w d
    simp only [coneFitIntCompose] at ih hs
    simp only [coneFitIntCompose, List.foldr_cons, coneFitIntStep, evalR_add, evalR_scale, evalR_cons, evalR_nil,
      hs, List.length_cons]
    push_cast
    linear_combination ((l : ℝ) + w * u) * ih - ((d : ℝ) ^ P.length * evalR P ((l + w * u) / d)) * hx

/-- The integer weight `C(k,i)·i!·(n−i)!` is `n!·C(k,i)/C(n,i)` for `i ≤ k ≤ n` (A111 keeps it in `int64`). -/
theorem coneFitIntWeight_eq {n k i : ℕ} (hik : i ≤ k) (hkn : k ≤ n) :
    ((coneFitBinomial k i * coneFitFactorial i * coneFitFactorial (n - i) : ℕ) : ℝ) =
      (n.factorial : ℝ) * ((k.choose i : ℝ) / (n.choose i : ℝ)) := by
  rw [coneFitBinomial_eq k i hik, coneFitFactorial_eq, coneFitFactorial_eq]
  have h := Nat.choose_mul_factorial_mul_factorial (le_trans hik hkn)
  have hc : (n.choose i : ℝ) ≠ 0 := by exact_mod_cast (Nat.choose_pos (le_trans hik hkn)).ne'
  rw [← h]
  push_cast
  field_simp

/-- The Bernstein combination of `coneFitIntBernstein P a b` is `n!·d^(n+1)/d` times `P(a + (b−a)·u)`, with
`d = den(a)·den(b)`. -/
theorem coneFitIntBernstein_sum (P : List ℤ) (a b : ℚ) (u : ℝ) :
    ∃ g : ℕ → ℚ, coneFitIntBernstein P a b = (List.range (P.length - 1 + 1)).map g ∧
      ∑ k ∈ Finset.range (P.length - 1 + 1), (g k : ℝ) * bw (P.length - 1) k u =
        (P.length - 1).factorial * (((a.den : ℚ) * b.den : ℚ) : ℝ) ^ P.length / (((a.den : ℚ) * b.den : ℚ) : ℝ) *
          evalR (P.map (Int.cast : ℤ → ℚ)) (a + (b - a) * u) := by
  refine ⟨_, rfl, ?_⟩
  set n := P.length - 1 with hn
  set l : ℚ := (a.num : ℚ) * b.den
  set w : ℚ := (b.num : ℚ) * a.den - (a.num : ℚ) * b.den
  set d : ℚ := (a.den : ℚ) * b.den with hd
  set P' := P.map (Int.cast : ℤ → ℚ)
  set q := (coneFitIntCompose P' l w d).1 with hq
  have hdpos : (0 : ℚ) < d := by positivity
  have hlen : q.length = P.length := by rw [hq, coneFitIntCompose_length, List.length_map]
  set Q := (toPoly q).map (algebraMap ℚ ℝ)
  have hQ : Q.natDegree ≤ n := le_trans natDegree_map_le (le_trans (toPoly_natDegree_le q) (by omega))
  have hE := coneFitIntCompose_eval P' l w d hdpos.ne' u
  rw [← hq] at hE
  have hx : ((l : ℝ) + w * u) / d = a + (b - a) * u := by
    have ha : (a : ℝ) = a.num / a.den := by exact_mod_cast (Rat.num_div_den a).symm
    have hb : (b : ℝ) = b.num / b.den := by exact_mod_cast (Rat.num_div_den b).symm
    have h1 : (a.den : ℝ) ≠ 0 := by exact_mod_cast a.den_nz
    have h2 : (b.den : ℝ) ≠ 0 := by exact_mod_cast b.den_nz
    simp only [l, w, d]
    push_cast
    rw [ha, hb]
    field_simp
  rw [hx, List.length_map] at hE
  have hsum : ∑ k ∈ Finset.range (n + 1),
      (((List.range (k + 1)).foldl (fun acc i =>
        acc + q.getD i 0 * ((coneFitBinomial k i * coneFitFactorial i * coneFitFactorial (n - i) : ℕ) : ℚ)) 0 :
          ℚ) : ℝ) * bw n k u = (n.factorial : ℝ) * Q.eval u := by
    rw [eval_eq_sum_bw Q n hQ u, Finset.mul_sum]
    refine Finset.sum_congr rfl fun k hk => ?_
    have hkn : k ≤ n := by simp only [Finset.mem_range] at hk; omega
    rw [foldl_add_range, zero_add, ← mul_assoc]
    congr 1
    push_cast
    rw [Finset.mul_sum]
    refine Finset.sum_congr rfl fun i hi => ?_
    have hik : i ≤ k := by simp only [Finset.mem_range] at hi; omega
    have hw := coneFitIntWeight_eq (n := n) hik hkn
    push_cast at hw
    rw [hw]
    simp only [Q, coeff_map, toPoly_coeff, eq_ratCast]
    ring
  rw [hsum]
  have hQe : Q.eval u = evalR q u := rfl
  rw [hQe]
  have hdR : (d : ℝ) ≠ 0 := by exact_mod_cast hdpos.ne'
  have hE' : evalR q u = (d : ℝ) ^ P.length * evalR P' (a + (b - a) * u) / d := by
    rw [eq_div_iff hdR]
    exact hE
  rw [hE']
  ring

theorem coneFitIntBounds_le (B : List ℚ) :
    ∀ v ∈ B, (coneFitIntBounds B).1 ≤ v ∧ v ≤ (coneFitIntBounds B).2 := by
  unfold coneFitIntBounds
  cases B with
  | nil => simp
  | cons x xs =>
    simp only [List.tail_cons, List.headD_cons, List.mem_cons, forall_eq_or_imp]
    obtain ⟨h1, h2, h3⟩ := bounds_foldl xs x x
    exact ⟨⟨h1, h2⟩, h3⟩

/-- Integer Bernstein enclosure: there is one `s > 0` such that, for every `t ∈ [a, b]`, `s·p(t)` lies between
the least and greatest entry of `coneFitIntBernstein (coneFitIntPoly p) a b`. A bound's sign is therefore a
sign of `p` on all of `[a, b]`. Covers `sketch.coneFitIntPoly`, `sketch.coneFitIntBernstein`,
`sketch.coneFitFactorial`, `sketch.coneFitIntBounds` and `sketch.coneFitBinomial`; assumptions A30, A32, A100,
A110, A111. -/
theorem coneFitIntBounds_mem (p : List ℚ) {a b : ℚ} (hab : a ≤ b) :
    ∃ s : ℝ, 0 < s ∧ ∀ t : ℝ, (a : ℝ) ≤ t → t ≤ b →
      ((coneFitIntBounds (coneFitIntBernstein (coneFitIntPoly p) a b)).1 : ℝ) ≤ s * evalR p t ∧
        s * evalR p t ≤ (coneFitIntBounds (coneFitIntBernstein (coneFitIntPoly p) a b)).2 := by
  obtain ⟨hD, -, -⟩ := coneFitIntDen_foldl p 1 one_pos
  set P := coneFitIntPoly p
  set n := P.length - 1 with hn
  set d : ℚ := (a.den : ℚ) * b.den
  set D : ℚ := (coneFitIntDen p : ℚ)
  have hDpos : (0 : ℝ) < (D : ℝ) := by simp only [D]; exact_mod_cast hD
  have hdpos : (0 : ℝ) < (d : ℝ) := by simp only [d]; push_cast; positivity
  refine ⟨(n.factorial : ℝ) * (d : ℝ) ^ P.length / d * D, by positivity, fun t ht0 ht1 => ?_⟩
  set B := coneFitIntBernstein P a b
  have hP : ∀ x : ℝ, evalR (P.map (Int.cast : ℤ → ℚ)) x = evalR p x * D := fun x => by
    rw [coneFitIntPoly_cast]
    exact evalR_scale p _ x
  -- `t = a + (b − a)·u` with `u ∈ [0, 1]`.
  obtain ⟨u, hu0, hu1, rfl⟩ : ∃ u : ℝ, 0 ≤ u ∧ u ≤ 1 ∧ t = a + (b - a) * u := by
    rcases eq_or_lt_of_le hab with h | h
    · subst h
      exact ⟨0, le_rfl, zero_le_one, by linarith⟩
    · have hba : (0 : ℝ) < (b : ℝ) - a := by exact_mod_cast sub_pos.mpr h
      refine ⟨(t - a) / (b - a), div_nonneg (by linarith) hba.le, (div_le_one hba).mpr (by linarith), ?_⟩
      field_simp
      ring
  obtain ⟨g, hg, hsum⟩ := coneFitIntBernstein_sum P a b u
  have hval : (n.factorial : ℝ) * (d : ℝ) ^ P.length / d * D * evalR p (a + (b - a) * u) =
      ∑ k ∈ Finset.range (n + 1), (g k : ℝ) * bw n k u := by
    rw [hsum, hP]
    simp only [d]
    ring
  rw [hval]
  have hb : ∀ k ∈ Finset.range (n + 1),
      ((coneFitIntBounds B).1 : ℝ) ≤ g k ∧ (g k : ℝ) ≤ (coneFitIntBounds B).2 := by
    intro k hk
    have hkn : k < P.length - 1 + 1 := by simp only [Finset.mem_range] at hk; omega
    have := coneFitIntBounds_le B (g k) (by
      rw [show B = List.map g (List.range (P.length - 1 + 1)) from hg]
      exact List.mem_map.mpr ⟨k, List.mem_range.mpr hkn, rfl⟩)
    exact ⟨by exact_mod_cast this.1, by exact_mod_cast this.2⟩
  have hw := bw_sum n u
  constructor
  · calc ((coneFitIntBounds B).1 : ℝ) = ∑ k ∈ Finset.range (n + 1), ((coneFitIntBounds B).1 : ℝ) * bw n k u := by
          rw [← Finset.mul_sum, hw, mul_one]
      _ ≤ _ := Finset.sum_le_sum fun k hk => mul_le_mul_of_nonneg_right (hb k hk).1 (bw_nonneg n k hu0 hu1)
  · calc _ ≤ ∑ k ∈ Finset.range (n + 1), ((coneFitIntBounds B).2 : ℝ) * bw n k u :=
          Finset.sum_le_sum fun k hk => mul_le_mul_of_nonneg_right (hb k hk).2 (bw_nonneg n k hu0 hu1)
      _ = _ := by rw [← Finset.mul_sum, hw, mul_one]

end Sketch.Cone

end
