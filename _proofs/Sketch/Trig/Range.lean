module

public import Sketch.Trig.Reduce
public import Sketch.Trig.Series

/-!
# Sine and cosine over an interval, and at a point

Models of `sketch.sinCosRange` and `sketch.sinCosPoint`. Over an interval the endpoint enclosures give the hull
of a monotone stretch. An extremum of sine or cosine sits at a multiple of π/2, so wherever such a multiple may
lie in the interval (`halfPiMayLieIn`) the matching `±1` joins the hull. `sin_le_max_of_no_peak` and its three
siblings prove the calculus fact: with no multiple of the matching class in `[lo, hi]`, the function stays
between its endpoint values. At a point, `sinCosPoint` sums the series for `|x| ≤ 64` and otherwise encloses
the reduced interval, which `reduceTurns_sound` ties back to `x`.
-/

public section

namespace Sketch.Trig

open Real Set

/-! ### Extrema of sine and cosine -/

/-- A continuous `f` that exceeds both endpoint values at `θ ∈ [lo, hi]` has an interior local maximum at
least `f θ`. -/
theorem exists_interior_max {f : ℝ → ℝ} (hf : Continuous f) {lo hi θ : ℝ} (h1 : lo ≤ θ) (h2 : θ ≤ hi)
    (h : max (f lo) (f hi) < f θ) : ∃ c, lo < c ∧ c < hi ∧ IsLocalMax f c ∧ f θ ≤ f c := by
  obtain ⟨c, hc, hmax⟩ := isCompact_Icc.exists_isMaxOn ⟨θ, h1, h2⟩ hf.continuousOn
  have hθ : f θ ≤ f c := hmax ⟨h1, h2⟩
  have hlo : lo ≠ c := fun e => by rw [← e] at hθ; linarith [le_max_left (f lo) (f hi)]
  have hhi : hi ≠ c := fun e => by rw [← e] at hθ; linarith [le_max_right (f lo) (f hi)]
  have hc1 : lo < c := lt_of_le_of_ne hc.1 hlo
  have hc2 : c < hi := lt_of_le_of_ne hc.2 (Ne.symm hhi)
  exact ⟨c, hc1, hc2, hmax.isLocalMax (Icc_mem_nhds hc1 hc2), hθ⟩

/-- A differentiable `f` whose critical points are each either a multiple `k·π/2` with `k ≡ j (mod 4)` or a
point where `f` is at most `m ≤ f lo`: with no such multiple in `[lo, hi]`, `f` stays at most its larger
endpoint value. -/
theorem le_max_of_no_peak {f : ℝ → ℝ} (hf : Differentiable ℝ f) (j : ℤ) (m : ℝ)
    (hcrit : ∀ c, deriv f c = 0 → ∃ k : ℤ, c = k * π / 2 ∧ (k % 4 = j ∨ f c ≤ m))
    {lo hi θ : ℝ} (hm : m ≤ f lo) (h1 : lo ≤ θ) (h2 : θ ≤ hi)
    (hk : ∀ k : ℤ, lo ≤ k * π / 2 → k * π / 2 ≤ hi → k % 4 ≠ j) : f θ ≤ max (f lo) (f hi) := by
  by_contra hlt
  push Not at hlt
  obtain ⟨c, hc1, hc2, hmax, hθ⟩ := exists_interior_max hf.continuous h1 h2 hlt
  obtain ⟨k, rfl, hk'⟩ := hcrit _ hmax.deriv_eq_zero
  rcases hk' with hj | hle
  · exact hk k hc1.le hc2.le hj
  · linarith [le_max_left (f lo) (f hi)]

/-- Sine and cosine at the multiples of π/2, by `k mod 4`. -/
theorem sin_cos_int_mul_pi_div_two (k : ℤ) :
    sin (k * π / 2) = (if k % 4 = 1 then 1 else if k % 4 = 3 then -1 else 0) ∧
    cos (k * π / 2) = (if k % 4 = 0 then 1 else if k % 4 = 2 then -1 else 0) := by
  have hk : (k : ℝ) * π / 2 = ((k % 4 : ℤ) : ℝ) * π / 2 + ((k / 4 : ℤ) : ℝ) * (2 * π) := by
    have : (k : ℝ) = ((k % 4 : ℤ) : ℝ) + 4 * ((k / 4 : ℤ) : ℝ) := by
      have := Int.emod_add_mul_ediv k 4
      exact_mod_cast this.symm
    rw [this]; ring
  rw [hk, sin_add_int_mul_two_pi, cos_add_int_mul_two_pi]
  have h : k % 4 = 0 ∨ k % 4 = 1 ∨ k % 4 = 2 ∨ k % 4 = 3 := by omega
  rcases h with h | h | h | h <;> rw [h]
  · simp
  · simp
  · simp
  · have e : ((3 : ℤ) : ℝ) * π / 2 = π / 2 + π := by push_cast; ring
    rw [e, sin_add_pi, cos_add_pi]
    simp

/-- The critical points of sine are the odd multiples of π/2; sine is `-1` at those with `k ≡ 3 (mod 4)`. -/
theorem sin_crit (c : ℝ) (h : cos c = 0) : ∃ k : ℤ, c = k * π / 2 ∧ (k % 4 = 1 ∨ sin c ≤ -1) := by
  obtain ⟨n, hn⟩ := cos_eq_zero_iff.mp h
  refine ⟨2 * n + 1, by rw [hn]; push_cast; ring, ?_⟩
  have hs := (sin_cos_int_mul_pi_div_two (2 * n + 1)).1
  rcases Int.emod_two_eq_zero_or_one n with h2 | h2
  · left; omega
  · right
    have h3 : (2 * n + 1) % 4 = 3 := by omega
    rw [hn, show (2 * (n : ℝ) + 1) * π / 2 = ((2 * n + 1 : ℤ) : ℝ) * π / 2 by push_cast; ring, hs]
    simp [h3]

/-- The critical points of `-sin` are the odd multiples of π/2; `-sin` is `-1` at those with `k ≡ 1 (mod 4)`. -/
theorem neg_sin_crit (c : ℝ) (h : cos c = 0) : ∃ k : ℤ, c = k * π / 2 ∧ (k % 4 = 3 ∨ -sin c ≤ -1) := by
  obtain ⟨n, hn⟩ := cos_eq_zero_iff.mp h
  refine ⟨2 * n + 1, by rw [hn]; push_cast; ring, ?_⟩
  have hs := (sin_cos_int_mul_pi_div_two (2 * n + 1)).1
  rcases Int.emod_two_eq_zero_or_one n with h2 | h2
  · right
    have h3 : (2 * n + 1) % 4 = 1 := by omega
    rw [hn, show (2 * (n : ℝ) + 1) * π / 2 = ((2 * n + 1 : ℤ) : ℝ) * π / 2 by push_cast; ring, hs]
    simp [h3]
  · left; omega

/-- The critical points of cosine are the even multiples of π/2; cosine is `-1` at those with `k ≡ 2 (mod 4)`. -/
theorem cos_crit (c : ℝ) (h : sin c = 0) : ∃ k : ℤ, c = k * π / 2 ∧ (k % 4 = 0 ∨ cos c ≤ -1) := by
  obtain ⟨n, hn⟩ := sin_eq_zero_iff.mp h
  refine ⟨2 * n, by rw [← hn]; push_cast; ring, ?_⟩
  have hc := (sin_cos_int_mul_pi_div_two (2 * n)).2
  rcases Int.emod_two_eq_zero_or_one n with h2 | h2
  · left; omega
  · right
    have h3 : (2 * n) % 4 = 2 := by omega
    rw [← hn, show (n : ℝ) * π = ((2 * n : ℤ) : ℝ) * π / 2 by push_cast; ring, hc]
    simp [h3]

/-- The critical points of `-cos` are the even multiples of π/2; `-cos` is `-1` at those with `k ≡ 0 (mod 4)`. -/
theorem neg_cos_crit (c : ℝ) (h : sin c = 0) : ∃ k : ℤ, c = k * π / 2 ∧ (k % 4 = 2 ∨ -cos c ≤ -1) := by
  obtain ⟨n, hn⟩ := sin_eq_zero_iff.mp h
  refine ⟨2 * n, by rw [← hn]; push_cast; ring, ?_⟩
  have hc := (sin_cos_int_mul_pi_div_two (2 * n)).2
  rcases Int.emod_two_eq_zero_or_one n with h2 | h2
  · right
    have h3 : (2 * n) % 4 = 0 := by omega
    rw [← hn, show (n : ℝ) * π = ((2 * n : ℤ) : ℝ) * π / 2 by push_cast; ring, hc]
    simp [h3]
  · left; omega

/-- With no `k·π/2`, `k ≡ 1 (mod 4)`, in `[lo, hi]`, sine stays at most its larger endpoint value. -/
theorem sin_le_max_of_no_peak {lo hi θ : ℝ} (h1 : lo ≤ θ) (h2 : θ ≤ hi)
    (hk : ∀ k : ℤ, lo ≤ k * π / 2 → k * π / 2 ≤ hi → k % 4 ≠ 1) : sin θ ≤ max (sin lo) (sin hi) :=
  le_max_of_no_peak differentiable_sin 1 (-1) (fun c hc => sin_crit c (by simpa using hc))
    (neg_one_le_sin lo) h1 h2 hk

/-- With no `k·π/2`, `k ≡ 3 (mod 4)`, in `[lo, hi]`, sine stays at least its smaller endpoint value. -/
theorem min_le_sin_of_no_trough {lo hi θ : ℝ} (h1 : lo ≤ θ) (h2 : θ ≤ hi)
    (hk : ∀ k : ℤ, lo ≤ k * π / 2 → k * π / 2 ≤ hi → k % 4 ≠ 3) : min (sin lo) (sin hi) ≤ sin θ := by
  have := le_max_of_no_peak (f := fun y => -sin y) differentiable_sin.neg 3 (-1)
    (fun c hc => neg_sin_crit c (by simpa using hc)) (by linarith [sin_le_one lo]) h1 h2 hk
  simp only [max_neg_neg] at this
  linarith

/-- With no `k·π/2`, `k ≡ 0 (mod 4)`, in `[lo, hi]`, cosine stays at most its larger endpoint value. -/
theorem cos_le_max_of_no_peak {lo hi θ : ℝ} (h1 : lo ≤ θ) (h2 : θ ≤ hi)
    (hk : ∀ k : ℤ, lo ≤ k * π / 2 → k * π / 2 ≤ hi → k % 4 ≠ 0) : cos θ ≤ max (cos lo) (cos hi) :=
  le_max_of_no_peak differentiable_cos 0 (-1) (fun c hc => cos_crit c (by simpa using hc))
    (neg_one_le_cos lo) h1 h2 hk

/-- With no `k·π/2`, `k ≡ 2 (mod 4)`, in `[lo, hi]`, cosine stays at least its smaller endpoint value. -/
theorem min_le_cos_of_no_trough {lo hi θ : ℝ} (h1 : lo ≤ θ) (h2 : θ ≤ hi)
    (hk : ∀ k : ℤ, lo ≤ k * π / 2 → k * π / 2 ≤ hi → k % 4 ≠ 2) : min (cos lo) (cos hi) ≤ cos θ := by
  have := le_max_of_no_peak (f := fun y => -cos y) differentiable_cos.neg 2 (-1)
    (fun c hc => neg_cos_crit c (by simpa using hc)) (by linarith [cos_le_one lo]) h1 h2 hk
  simp only [max_neg_neg] at this
  linarith

/-! ### `sinCosRange` -/

/-- Go's `((k % 4) + 4) % 4`, with Go's truncating `%`. -/
@[expose] def quadrant (k : ℤ) : ℤ := ((k.tmod 4) + 4).tmod 4

theorem quadrant_eq (k : ℤ) : quadrant k = k % 4 := by
  unfold quadrant
  rw [Int.tmod_eq_emod, Int.tmod_eq_emod]
  split_ifs <;> simp_all <;> omega

/-- The `switch` of `sketch.sinCosRange`: quadrant 0 raises cosine's upper end to 1, 1 sine's, 2 lowers
cosine's lower end to -1, 3 sine's. -/
@[expose] noncomputable def bumpExtremum (j : ℤ) (sc : Ival × Ival) : Ival × Ival :=
  if j = 0 then (sc.1, ⟨sc.2.lo, 1⟩)
  else if j = 1 then (⟨sc.1.lo, 1⟩, sc.2)
  else if j = 2 then (sc.1, ⟨-1, sc.2.hi⟩)
  else if j = 3 then (⟨-1, sc.1.hi⟩, sc.2)
  else sc

/-- The integers `kmin, kmin+1, …, kmax` of the loop in `sketch.sinCosRange`. -/
@[expose] def kRange (kmin kmax : ℤ) : List ℤ :=
  (List.range (kmax - kmin + 1).toNat).map fun i : ℕ => kmin + (i : ℤ)

theorem mem_kRange {kmin kmax k : ℤ} (h1 : kmin ≤ k) (h2 : k ≤ kmax) : k ∈ kRange kmin kmax := by
  unfold kRange
  rw [List.mem_map]
  refine ⟨(k - kmin).toNat, List.mem_range.mpr ?_, ?_⟩
  · omega
  · rw [Int.toNat_of_nonneg (by omega)]
    omega

/-- One pass of the loop in `sketch.sinCosRange`: bump the extremum of `k`'s quadrant when `k·π/2` may lie
in `q`. -/
@[expose] noncomputable def rangeStep (R : Rounding) (q : Ival) (sc : Ival × Ival) (k : ℤ) : Ival × Ival :=
  if halfPiMayLieIn R k q then bumpExtremum (quadrant k) sc else sc

theorem bump_sin_lo (i : ℤ) (sc : Ival × Ival) :
    (bumpExtremum i sc).1.lo = if i = 3 then -1 else sc.1.lo := by
  unfold bumpExtremum; split_ifs <;> simp_all

theorem bump_sin_hi (i : ℤ) (sc : Ival × Ival) :
    (bumpExtremum i sc).1.hi = if i = 1 then 1 else sc.1.hi := by
  unfold bumpExtremum; split_ifs <;> simp_all

theorem bump_cos_lo (i : ℤ) (sc : Ival × Ival) :
    (bumpExtremum i sc).2.lo = if i = 2 then -1 else sc.2.lo := by
  unfold bumpExtremum; split_ifs <;> simp_all

theorem bump_cos_hi (i : ℤ) (sc : Ival × Ival) :
    (bumpExtremum i sc).2.hi = if i = 0 then 1 else sc.2.hi := by
  unfold bumpExtremum; split_ifs <;> simp_all

/-- Model of `sketch.sinCosRange`. `P` is the point enclosure (`sinCosPoint`); `wide`, `kmin` and `kmax` are
the float64 decisions of `TrigEnv`. -/
@[expose] noncomputable def sinCosRange (R : Rounding) (P : ℝ → Option (Ival × Ival)) (q : Ival)
    (wide : Bool) (kmin kmax : ℤ) : Option (Ival × Ival) :=
  match P q.lo with
  | none => none
  | some (sa, ca) =>
    if q.lo = q.hi then some (sa, ca)
    else
      match P q.hi with
      | none => none
      | some (sb, cb) =>
        if wide then some (⟨-1, 1⟩, ⟨-1, 1⟩)
        else
          some ((kRange kmin kmax).foldl (rangeStep R q) (hull sa sb, hull ca cb))

/-- Each field of the loop result is either its bumped value, or its initial value with no bump of its
quadrant applied. -/
theorem fold_field (R : Rounding) (q : Ival) (g : Ival × Ival → ℝ) (j : ℤ) (v : ℝ)
    (hbump : ∀ i sc, g (bumpExtremum i sc) = if i = j then v else g sc) :
    ∀ (l : List ℤ) (sc0 : Ival × Ival),
      g (l.foldl (rangeStep R q) sc0) = v ∨
      (g (l.foldl (rangeStep R q) sc0) = g sc0 ∧ ∀ k ∈ l, halfPiMayLieIn R k q = true → quadrant k ≠ j) := by
  intro l
  induction l with
  | nil => intro sc0; right; simp
  | cons k l ih =>
    intro sc0
    rw [List.foldl_cons]
    rcases ih (rangeStep R q sc0 k) with h | ⟨h, hl⟩
    · left; exact h
    · by_cases hm : halfPiMayLieIn R k q = true
      · by_cases hq : quadrant k = j
        · left; rw [h, rangeStep, ite_eq_left hm, hbump, ite_eq_left hq]
        · right
          refine ⟨by rw [h, rangeStep, ite_eq_left hm, hbump, ite_eq_right hq], ?_⟩
          intro k' hk' hm'
          rcases List.mem_cons.mp hk' with rfl | hk'
          · exact hq
          · exact hl k' hk' hm'
      · right
        refine ⟨by rw [h, rangeStep, ite_eq_right hm], ?_⟩
        intro k' hk' hm'
        rcases List.mem_cons.mp hk' with rfl | hk'
        · exact absurd hm' hm
        · exact hl k' hk' hm'

/-- **Soundness of the interval enclosure.** If the point enclosure `P` is sound and the float64 `k` range
covers every multiple of π/2 in `q`, the model of `sketch.sinCosRange` encloses `sin θ` and `cos θ` for every
`θ` in `q`. Assumptions: A22, A24, A27 (the `k` range, as `hk`), A28. -/
theorem sinCosRange_sound (R : Rounding) (P : ℝ → Option (Ival × Ival)) (q : Ival) (wide : Bool)
    (kmin kmax : ℤ) (hP : ∀ y S C, P y = some (S, C) → S.Mem (sin y) ∧ C.Mem (cos y))
    (hk : wide = false → (P q.lo).isSome → (P q.hi).isSome →
      ∀ k : ℤ, q.lo ≤ k * π / 2 → k * π / 2 ≤ q.hi → kmin ≤ k ∧ k ≤ kmax)
    {S C : Ival} (h : sinCosRange R P q wide kmin kmax = some (S, C)) :
    ∀ θ, q.lo ≤ θ → θ ≤ q.hi → S.Mem (sin θ) ∧ C.Mem (cos θ) := by
  intro θ h1 h2
  unfold sinCosRange at h
  split at h
  · exact absurd h (by simp)
  rename_i sa ca hPa
  obtain ⟨hsa, hca⟩ := hP _ _ _ hPa
  by_cases heq : q.lo = q.hi
  · simp only [heq, ↓reduceIte, Option.some.injEq, Prod.mk.injEq] at h
    obtain ⟨rfl, rfl⟩ := h
    have : θ = q.lo := le_antisymm (heq ▸ h2) h1
    subst this
    exact ⟨hsa, hca⟩
  simp only [heq, ↓reduceIte] at h
  split at h
  · exact absurd h (by simp)
  rename_i sb cb hPb
  obtain ⟨hsb, hcb⟩ := hP _ _ _ hPb
  cases hw' : wide
  swap
  · simp only [hw', ↓reduceIte, Option.some.injEq, Prod.mk.injEq] at h
    obtain ⟨rfl, rfl⟩ := h
    exact ⟨⟨neg_one_le_sin θ, sin_le_one θ⟩, ⟨neg_one_le_cos θ, cos_le_one θ⟩⟩
  simp only [hw', Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at h
  -- every multiple of π/2 in q is in the loop range and passes the test
  have hin : ∀ k : ℤ, q.lo ≤ k * π / 2 → k * π / 2 ≤ q.hi →
      k ∈ kRange kmin kmax ∧ halfPiMayLieIn R k q = true := by
    intro k hk1 hk2
    obtain ⟨hk3, hk4⟩ := hk hw' (by simp [hPa]) (by simp [hPb]) k hk1 hk2
    exact ⟨mem_kRange hk3 hk4, halfPiMayLieIn_of_mem R k q hk1 hk2⟩
  have hno : ∀ j : ℤ, (∀ k ∈ kRange kmin kmax, halfPiMayLieIn R k q = true → quadrant k ≠ j) →
      ∀ k : ℤ, q.lo ≤ k * π / 2 → k * π / 2 ≤ q.hi → k % 4 ≠ j := by
    intro j hall k hk1 hk2
    obtain ⟨hmem, hmay⟩ := hin k hk1 hk2
    rw [← quadrant_eq]
    exact hall k hmem hmay
  have fsl := fold_field R q (fun sc => sc.1.lo) 3 (-1) bump_sin_lo (kRange kmin kmax) (hull sa sb, hull ca cb)
  have fsh := fold_field R q (fun sc => sc.1.hi) 1 1 bump_sin_hi (kRange kmin kmax) (hull sa sb, hull ca cb)
  have fcl := fold_field R q (fun sc => sc.2.lo) 2 (-1) bump_cos_lo (kRange kmin kmax) (hull sa sb, hull ca cb)
  have fch := fold_field R q (fun sc => sc.2.hi) 0 1 bump_cos_hi (kRange kmin kmax) (hull sa sb, hull ca cb)
  rw [h] at fsl fsh fcl fch
  simp only [hull] at fsl fsh fcl fch
  refine ⟨⟨?_, ?_⟩, ⟨?_, ?_⟩⟩
  · rcases fsl with e | ⟨e, hall⟩
    · rw [e]; exact neg_one_le_sin θ
    · rw [e]
      have := min_le_sin_of_no_trough h1 h2 (hno 3 hall)
      exact le_trans (min_le_min hsa.1 hsb.1) this
  · rcases fsh with e | ⟨e, hall⟩
    · rw [e]; exact sin_le_one θ
    · rw [e]
      have := sin_le_max_of_no_peak h1 h2 (hno 1 hall)
      exact this.trans (max_le_max hsa.2 hsb.2)
  · rcases fcl with e | ⟨e, hall⟩
    · rw [e]; exact neg_one_le_cos θ
    · rw [e]
      have := min_le_cos_of_no_trough h1 h2 (hno 2 hall)
      exact le_trans (min_le_min hca.1 hcb.1) this
  · rcases fch with e | ⟨e, hall⟩
    · rw [e]; exact cos_le_one θ
    · rw [e]
      have := cos_le_max_of_no_peak h1 h2 (hno 0 hall)
      exact this.trans (max_le_max hca.2 hcb.2)

/-! ### `sinCosPoint` -/

/-- Model of `sketch.sinCosPoint`, with `maxTrigArg = 2^40` and `maxTaylorArg = 64` (A22). Deliberate
differences: the point cache is omitted (A26), the direct sum is `sinCosSeriesBig` at the precision
`E.fracBits x` where Go runs `sinCosSeriesPair` (A150) or `sinCosSeriesWords` (A25) first, and `fuel` bounds both the recursion through `sinCosRange` and the series
loop; `none` stands for a run the Go code would continue. -/
@[expose] noncomputable def sinCosPoint (R : Rounding) (E : TrigEnv) : ℕ → ℝ → Option (Ival × Ival)
  | fuel, x =>
    if ¬|x| ≤ 2 ^ 40 then none
    else if |x| > 64 then
      match fuel with
      | 0 => none
      | f + 1 =>
        let q := reduceTurns R x (E.turns x)
        sinCosRange R (sinCosPoint R E f) q (E.wide q) (E.kmin q) (E.kmax q)
    else sinCosSeriesBig R (E.fracBits x) x (E.stop x) fuel

theorem sinCosPoint_bound (R : Rounding) (E : TrigEnv) (fuel : ℕ) (x : ℝ)
    (h : (sinCosPoint R E fuel x).isSome) : |x| ≤ 2 ^ 40 := by
  by_contra hc
  unfold sinCosPoint at h
  simp [hc] at h

/-- **Soundness of the certified sine and cosine.** Whenever the model of `sketch.sinCosPoint` returns, the first
interval contains `sin x` and the second `cos x`, for every real `x`: directly through `sinCosSeriesBig_sound`
for `|x| ≤ 64`, and through `reduceTurns_sound` and `sinCosRange_sound` beyond. Covers `sketch.sinCosPoint`,
`sketch.sinCosSeriesBig`, `sketch.reduceTurns`, `sketch.sinCosRange`, `sketch.halfPiMayLieIn` and their
helpers. Assumptions: A20 to A28, and A150 for the two-word sums. -/
theorem sinCosPoint_sound (R : Rounding) (E : TrigEnv) :
    ∀ (fuel : ℕ) (x : ℝ) (S C : Ival), sinCosPoint R E fuel x = some (S, C) →
      S.Mem (sin x) ∧ C.Mem (cos x) := by
  intro fuel
  induction fuel with
  | zero =>
    intro x S C h
    unfold sinCosPoint at h
    split_ifs at h with h1 h2
    exact sinCosSeriesBig_sound R _ x _ _ S C h
  | succ f ih =>
    intro x S C h
    unfold sinCosPoint at h
    split_ifs at h with h1 h2
    · simp only at h
      set q := reduceTurns R x (E.turns x)
      obtain ⟨hmem, hsin, hcos⟩ := reduceTurns_sound R x (E.turns x)
      have hk : E.wide q = false → (sinCosPoint R E f q.lo).isSome → (sinCosPoint R E f q.hi).isSome →
          ∀ k : ℤ, q.lo ≤ k * π / 2 → k * π / 2 ≤ q.hi → E.kmin q ≤ k ∧ k ≤ E.kmax q :=
        fun hw ha hb => E.k_cover q hw (sinCosPoint_bound R E f _ ha) (sinCosPoint_bound R E f _ hb)
      have := sinCosRange_sound R (sinCosPoint R E f) q (E.wide q) (E.kmin q) (E.kmax q) ih hk h _
        hmem.1 hmem.2
      rw [hsin, hcos] at this
      exact this
    · exact sinCosSeriesBig_sound R _ x _ _ S C h

end Sketch.Trig

end
