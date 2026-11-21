module

public import Sketch.Trig.Rounding
public import Sketch.Trig.Taylor

/-!
# Fixed-point Taylor sums of sine and cosine

Model of `sketch.sinCosSeriesBig`. Every term `t_n = x^n/n!` is an integer count of `2^-192`. The loop adds
`t̂_n` into the sine or cosine sum by `n mod 4`, then forms `t̂_(n+1)` from `t̂_n` with one floor shift and one
truncating division, and its error bound `err_(n+1) = ⌈err_n·|x|/(n+1)⌉ + 2`.

The model states each step through the exact value `x` (`advance`). `goTermStep_eq` and `goErrStep_eq` show
the Go operations on the integer pair `x = m·2^e` give the same integers: `big.Int.Rsh` floors and `Quo` and
`QuoRem` truncate (A20), and `math.Frexp` gives that pair exactly (A21). The invariant `SeriesInv` says
`|t̂_n - 2^192·x^n/n!| ≤ err_n` and each sum is within its accumulated error of `2^192` times the Taylor
partial sum. The Lagrange remainder (`abs_sin_cos_sub_taylor_le`) bounds the omitted tail by `|x|^N/N!`
for every `N`, so the result is sound whatever the stopping test decides.
-/

public section

namespace Sketch.Trig

open Real Nat Finset

/-- The loop state of `sketch.sinCosSeriesBig`: the term index `n`, the term `t` and its error bound `tErr`,
the sums `s`, `c` and their error bounds `sErr`, `cErr`. -/
structure SeriesState where
  n : ℕ
  t : ℤ
  tErr : ℤ
  s : ℤ
  c : ℤ
  sErr : ℤ
  cErr : ℤ

/-- The state before the first term: `t = 2^trigFracBits` (one), everything else zero. -/
@[expose] def SeriesState.init : SeriesState := ⟨0, 2 ^ 192, 0, 0, 0, 0, 0⟩

/-- The `switch n % 4` of the loop in `sketch.sinCosSeriesBig`. -/
@[expose] def accumulate (st : SeriesState) : SeriesState :=
  match st.n % 4 with
  | 0 => { st with c := st.c + st.t, cErr := st.cErr + st.tErr }
  | 1 => { st with s := st.s + st.t, sErr := st.sErr + st.tErr }
  | 2 => { st with c := st.c - st.t, cErr := st.cErr + st.tErr }
  | _ => { st with s := st.s - st.t, sErr := st.sErr + st.tErr }

/-- The term update of `sketch.sinCosSeriesBig`, stated through the exact `x`:
`t ← ⌊t·x⌋ quo (n+1)` and `err ← ⌈err·|x|/(n+1)⌉ + 2`. `goTermStep_eq` and `goErrStep_eq` give the Go form. -/
@[expose] noncomputable def advance (x : ℝ) (st : SeriesState) : SeriesState :=
  { st with
    n := st.n + 1
    t := Int.tdiv ⌊(st.t : ℝ) * x⌋ (st.n + 1)
    tErr := ⌈(st.tErr : ℝ) * |x| / (st.n + 1)⌉ + 2 }

/-- The loop of `sketch.sinCosSeriesBig`. `stop n bound s c` is the break test (`TrigEnv.stop`) on the term
index, `bound = |t̂_n| + err_n` and the two sums; on a break the loop returns the state and `bound`.
Deliberate difference: `fuel` bounds the number of steps, and `none` stands for a run the Go loop would
continue. -/
@[expose] noncomputable def seriesLoop (x : ℝ) (stop : ℕ → ℤ → ℤ → ℤ → Bool) :
    ℕ → SeriesState → Option (SeriesState × ℤ)
  | fuel, st =>
    if stop st.n (|st.t| + st.tErr) st.s st.c then some (st, |st.t| + st.tErr)
    else
      match fuel with
      | 0 => none
      | f + 1 => seriesLoop x stop f (advance x (accumulate st))

/-- Model of `sketch.sinCosSeriesBig`: the loop, then each sum widened by its error bound plus the tail bound
on both sides, converted outward by `fixedLower` and `fixedUpper`, and clamped to `[-1, 1]`. -/
@[expose] noncomputable def sinCosSeriesBig (R : Rounding) (x : ℝ) (stop : ℕ → ℤ → ℤ → ℤ → Bool) (fuel : ℕ) :
    Option (Ival × Ival) :=
  match seriesLoop x stop fuel SeriesState.init with
  | none => none
  | some (st, bound) =>
    let sw := st.sErr + bound
    let cw := st.cErr + bound
    some (clampUnit ⟨R.fixedLower (st.s - sw), R.fixedUpper (st.s + sw)⟩,
      clampUnit ⟨R.fixedLower (st.c - cw), R.fixedUpper (st.c + cw)⟩)

/-! ### The Go integer operations -/

/-- The shift of `sketch.sinCosSeriesBig`: `Lsh` by `e` when `e ≥ 0`, else `Rsh` by `-e`, which floors (A20).
Lean's `/` on `ℤ` by a positive divisor is the floor. -/
@[expose] def goShift (p e : ℤ) : ℤ := if 0 ≤ e then p * 2 ^ e.toNat else p / 2 ^ (-e).toNat

/-- The Go term update: `t.Mul(t, m)`, the shift, `t.Quo(t, div)` (truncating, A20). -/
@[expose] def goTermStep (m e : ℤ) (n : ℕ) (t : ℤ) : ℤ := (goShift (t * m) e).tdiv (n + 1)

/-- The Go error update: `tErr.Mul(tErr, am)`, `Lsh` of `tErr` or of `den`, `QuoRem` (truncating, A20), one
more when the remainder is nonzero, then `+ 2`. -/
@[expose] def goErrStep (am e : ℤ) (n : ℕ) (err : ℤ) : ℤ :=
  let num := if 0 ≤ e then err * am * 2 ^ e.toNat else err * am
  let den : ℤ := if 0 ≤ e then (n + 1 : ℤ) else (n + 1) * 2 ^ (-e).toNat
  (if num.tmod den ≠ 0 then num.tdiv den + 1 else num.tdiv den) + 2

theorem two_zpow_of_nonneg {e : ℤ} (he : 0 ≤ e) : (2 : ℝ) ^ e = ((2 ^ e.toNat : ℤ) : ℝ) := by
  push_cast
  rw [← zpow_natCast, Int.toNat_of_nonneg he]

theorem two_zpow_of_neg {e : ℤ} (he : ¬0 ≤ e) : (2 : ℝ) ^ e = 1 / ((2 ^ (-e).toNat : ℤ) : ℝ) := by
  push_cast
  rw [← zpow_natCast, Int.toNat_of_nonneg (by omega), zpow_neg, one_div, inv_inv]

/-- The Go shift of `t·m` is the floor of `t·x` for `x = m·2^e`. -/
theorem goShift_eq (m e t : ℤ) (x : ℝ) (hx : x = m * 2 ^ e) : goShift (t * m) e = ⌊(t : ℝ) * x⌋ := by
  unfold goShift
  split_ifs with he
  · rw [hx, two_zpow_of_nonneg he]
    have : (t : ℝ) * (m * ((2 ^ e.toNat : ℤ) : ℝ)) = ((t * m * 2 ^ e.toNat : ℤ) : ℝ) := by push_cast; ring
    rw [this, Int.floor_intCast]
  · rw [hx, two_zpow_of_neg he]
    have hpos : (0 : ℤ) < 2 ^ (-e).toNat := by positivity
    have : (t : ℝ) * (m * (1 / ((2 ^ (-e).toNat : ℤ) : ℝ))) =
        ((t * m : ℤ) : ℝ) / ((2 ^ (-e).toNat : ℕ) : ℝ) := by push_cast; ring
    rw [this, Int.floor_div_natCast, Int.floor_intCast]
    push_cast
    rfl

/-- The Go term update equals the model's `advance` step for `x = m·2^e` (A20, A21). -/
theorem goTermStep_eq (m e : ℤ) (n : ℕ) (t : ℤ) (x : ℝ) (hx : x = m * 2 ^ e) :
    goTermStep m e n t = Int.tdiv ⌊(t : ℝ) * x⌋ (n + 1) := by
  unfold goTermStep
  rw [goShift_eq m e t x hx]

/-- A truncating quotient of a non-negative integer, plus one when the remainder is nonzero, is the ceiling. -/
theorem tdiv_ceil (num den : ℤ) (hnum : 0 ≤ num) (hden : 0 < den) :
    (if num.tmod den ≠ 0 then num.tdiv den + 1 else num.tdiv den) = ⌈(num : ℝ) / den⌉ := by
  have hdec : num = den * num.tdiv den + num.tmod den := by
    have := Int.tmod_add_tdiv_mul num den; linarith
  have hr0 : 0 ≤ num.tmod den := Int.tmod_nonneg den hnum
  have hr1 : num.tmod den < den := Int.tmod_lt_of_pos num hden
  have hd : (0 : ℝ) < den := by exact_mod_cast hden
  have hdecR : (num : ℝ) = den * num.tdiv den + num.tmod den := by exact_mod_cast hdec
  have hq : (num : ℝ) / den = num.tdiv den + (num.tmod den : ℝ) / den := by
    rw [hdecR, add_div, mul_div_cancel_left₀ _ hd.ne']
  symm
  rw [Int.ceil_eq_iff]
  have hr0' : (0 : ℝ) ≤ num.tmod den := by exact_mod_cast hr0
  have hr1' : (num.tmod den : ℝ) < den := by exact_mod_cast hr1
  have hfrac : (num.tmod den : ℝ) / den < 1 := by rw [div_lt_one hd]; exact hr1'
  split_ifs with hr
  · have hr' : (0 : ℝ) < num.tmod den := by
      have : 0 < num.tmod den := lt_of_le_of_ne hr0 (Ne.symm hr)
      exact_mod_cast this
    have : (0 : ℝ) < (num.tmod den : ℝ) / den := div_pos hr' hd
    push_cast
    constructor <;> linarith
  · push Not at hr
    rw [hq, hr]
    simp

/-- The Go error update equals the model's `advance` step for `x = m·2^e` and a non-negative bound (A20,
A21). -/
theorem goErrStep_eq (m e : ℤ) (n : ℕ) (err : ℤ) (x : ℝ) (hx : x = m * 2 ^ e) (herr : 0 ≤ err) :
    goErrStep |m| e n err = ⌈(err : ℝ) * |x| / (n + 1)⌉ + 2 := by
  have hax : |x| = |(m : ℝ)| * 2 ^ e := by
    rw [hx, abs_mul, abs_of_pos (zpow_pos (by norm_num : (0 : ℝ) < 2) e)]
  have hem : 0 ≤ err * |m| := mul_nonneg herr (abs_nonneg m)
  unfold goErrStep
  by_cases he : 0 ≤ e
  · simp only [he, ↓reduceIte]
    rw [tdiv_ceil _ _ (mul_nonneg hem (by positivity)) (by positivity)]
    congr 2
    rw [hax, two_zpow_of_nonneg he]
    push_cast
    ring
  · simp only [he, ↓reduceIte]
    rw [tdiv_ceil _ _ hem (by positivity)]
    congr 2
    rw [hax, two_zpow_of_neg he]
    push_cast
    field_simp

/-! ### The invariant -/

/-- The loop invariant: `0 ≤ err_n`, `|t̂_n - 2^192·x^n/n!| ≤ err_n`, and each sum within its error bound of
`2^192` times the Taylor partial sum of its function over the first `n` terms. -/
@[expose] def SeriesInv (x : ℝ) (st : SeriesState) : Prop :=
  0 ≤ st.tErr ∧
  |(st.t : ℝ) - 2 ^ 192 * x ^ st.n / st.n !| ≤ (st.tErr : ℝ) ∧
  |(st.s : ℝ) - 2 ^ 192 * ∑ j ∈ range st.n, (sinSign j : ℝ) * x ^ j / j !| ≤ (st.sErr : ℝ) ∧
  |(st.c : ℝ) - 2 ^ 192 * ∑ j ∈ range st.n, (cosSign j : ℝ) * x ^ j / j !| ≤ (st.cErr : ℝ)

theorem seriesInv_init (x : ℝ) : SeriesInv x SeriesState.init := by
  norm_num [SeriesInv, SeriesState.init]

/-- `accumulate` adds `sinSign n · t` and `cosSign n · t` to the sums and `|sign|·err` to their bounds. -/
theorem accumulate_eq (st : SeriesState) :
    accumulate st = { st with
      s := st.s + sinSign st.n * st.t
      sErr := st.sErr + |sinSign st.n| * st.tErr
      c := st.c + cosSign st.n * st.t
      cErr := st.cErr + |cosSign st.n| * st.tErr } := by
  have h : st.n % 4 = 0 ∨ st.n % 4 = 1 ∨ st.n % 4 = 2 ∨ st.n % 4 = 3 := by omega
  rcases h with h | h | h | h <;>
    simp [accumulate, sinSign, cosSign, h] <;> ring_nf

/-- A truncating quotient is within one of the exact quotient. -/
theorem abs_tdiv_sub_lt (p : ℤ) (n : ℕ) : |((Int.tdiv p (n + 1) : ℤ) : ℝ) - p / (n + 1)| < 1 := by
  have hd : (0 : ℤ) < n + 1 := by positivity
  have hdec : p = (n + 1) * Int.tdiv p (n + 1) + Int.tmod p (n + 1) := by
    have := Int.tmod_add_tdiv_mul p (n + 1); linarith
  have h1 : -((n : ℤ) + 1) < Int.tmod p (n + 1) := Int.lt_tmod_of_pos p hd
  have h2 : Int.tmod p (n + 1) < (n : ℤ) + 1 := Int.tmod_lt_of_pos p hd
  have hdr : (0 : ℝ) < n + 1 := by positivity
  have hp : (p : ℝ) = (n + 1) * (Int.tdiv p (n + 1) : ℝ) + (Int.tmod p (n + 1) : ℝ) := by
    exact_mod_cast hdec
  have e : ((Int.tdiv p (n + 1) : ℤ) : ℝ) - p / (n + 1) = -((Int.tmod p (n + 1) : ℝ) / (n + 1)) := by
    rw [hp]; field_simp; ring
  rw [e, abs_neg, abs_div, abs_of_pos hdr, div_lt_one hdr, abs_lt]
  have h1' : -((n : ℝ) + 1) < (Int.tmod p (n + 1) : ℝ) := by exact_mod_cast h1
  have h2' : (Int.tmod p (n + 1) : ℝ) < (n : ℝ) + 1 := by exact_mod_cast h2
  exact ⟨h1', h2'⟩

/-- One step of the term recursion keeps `|t̂ - 2^192·x^n/n!| ≤ err`. -/
theorem term_step (x : ℝ) (n : ℕ) (t err : ℤ)
    (herr : |(t : ℝ) - 2 ^ 192 * x ^ n / n !| ≤ (err : ℝ)) :
    |((Int.tdiv ⌊(t : ℝ) * x⌋ (n + 1) : ℤ) : ℝ) - 2 ^ 192 * x ^ (n + 1) / (n + 1)!| ≤
      ((⌈(err : ℝ) * |x| / (n + 1)⌉ + 2 : ℤ) : ℝ) := by
  set p := ⌊(t : ℝ) * x⌋ with hpdef
  set τ := 2 ^ 192 * x ^ n / n ! with hτ
  have hdr : (0 : ℝ) < n + 1 := by positivity
  have e1 := abs_tdiv_sub_lt p n
  have e2 : |(p : ℝ) - t * x| < 1 := by
    rw [abs_lt]
    constructor
    · linarith [Int.lt_floor_add_one ((t : ℝ) * x)]
    · linarith [Int.floor_le ((t : ℝ) * x)]
  have hτ' : 2 ^ 192 * x ^ (n + 1) / ((n + 1)! : ℝ) = τ * x / (n + 1) := by
    rw [hτ, Nat.factorial_succ]
    push_cast
    field_simp
    ring
  have key : ((Int.tdiv p (n + 1) : ℤ) : ℝ) - τ * x / (n + 1) =
      (((Int.tdiv p (n + 1) : ℤ) : ℝ) - p / (n + 1)) + ((p : ℝ) - t * x) / (n + 1) +
        ((t : ℝ) - τ) * x / (n + 1) := by
    field_simp
    ring
  rw [hτ', key]
  have b2 : |((p : ℝ) - t * x) / (n + 1)| ≤ 1 := by
    rw [abs_div, abs_of_pos hdr, div_le_one hdr]
    have : (1 : ℝ) ≤ n + 1 := by linarith [(Nat.cast_nonneg n : (0 : ℝ) ≤ n)]
    linarith
  have b3 : |((t : ℝ) - τ) * x / (n + 1)| ≤ (err : ℝ) * |x| / (n + 1) := by
    rw [abs_div, abs_mul, abs_of_pos hdr]
    gcongr
  have b4 : (err : ℝ) * |x| / (n + 1) ≤ ⌈(err : ℝ) * |x| / (n + 1)⌉ := Int.le_ceil _
  push_cast
  calc _ ≤ |((Int.tdiv p (n + 1) : ℤ) : ℝ) - p / (n + 1)| + |((p : ℝ) - t * x) / (n + 1)| +
        |((t : ℝ) - τ) * x / (n + 1)| := abs_add_three _ _ _
    _ ≤ _ := by linarith

/-- One loop step keeps the invariant. -/
theorem seriesInv_step (x : ℝ) (st : SeriesState) (h : SeriesInv x st) :
    SeriesInv x (advance x (accumulate st)) := by
  obtain ⟨h0, ht, hs, hc⟩ := h
  rw [accumulate_eq]
  simp only [SeriesInv, advance]
  have hsg := abs_sinSign_le st.n
  have hcg := abs_cosSign_le st.n
  refine ⟨?_, ?_, ?_, ?_⟩
  · have : (0 : ℤ) ≤ ⌈(st.tErr : ℝ) * |x| / (st.n + 1)⌉ :=
      Int.ceil_nonneg (by positivity)
    omega
  · exact term_step x st.n st.t st.tErr ht
  · rw [sum_range_succ]
    push_cast
    have e : (st.s : ℝ) + sinSign st.n * st.t -
        2 ^ 192 * (∑ j ∈ range st.n, (sinSign j : ℝ) * x ^ j / j ! + sinSign st.n * x ^ st.n / st.n !) =
        ((st.s : ℝ) - 2 ^ 192 * ∑ j ∈ range st.n, (sinSign j : ℝ) * x ^ j / j !) +
          (sinSign st.n : ℝ) * ((st.t : ℝ) - 2 ^ 192 * x ^ st.n / st.n !) := by ring
    rw [e]
    have hsg' : |(sinSign st.n : ℝ)| ≤ 1 := by exact_mod_cast hsg
    calc _ ≤ |(st.s : ℝ) - 2 ^ 192 * ∑ j ∈ range st.n, (sinSign j : ℝ) * x ^ j / j !| +
          |(sinSign st.n : ℝ)| * |(st.t : ℝ) - 2 ^ 192 * x ^ st.n / st.n !| := by
          rw [← abs_mul]; exact abs_add_le _ _
      _ ≤ _ := by
          have h0' : (0 : ℝ) ≤ st.tErr := by exact_mod_cast h0
          have := mul_le_mul (le_refl |(sinSign st.n : ℝ)|) ht (abs_nonneg _) (abs_nonneg _)
          linarith
  · rw [sum_range_succ]
    push_cast
    have e : (st.c : ℝ) + cosSign st.n * st.t -
        2 ^ 192 * (∑ j ∈ range st.n, (cosSign j : ℝ) * x ^ j / j ! + cosSign st.n * x ^ st.n / st.n !) =
        ((st.c : ℝ) - 2 ^ 192 * ∑ j ∈ range st.n, (cosSign j : ℝ) * x ^ j / j !) +
          (cosSign st.n : ℝ) * ((st.t : ℝ) - 2 ^ 192 * x ^ st.n / st.n !) := by ring
    rw [e]
    calc _ ≤ |(st.c : ℝ) - 2 ^ 192 * ∑ j ∈ range st.n, (cosSign j : ℝ) * x ^ j / j !| +
          |(cosSign st.n : ℝ)| * |(st.t : ℝ) - 2 ^ 192 * x ^ st.n / st.n !| := by
          rw [← abs_mul]; exact abs_add_le _ _
      _ ≤ _ := by
          have := mul_le_mul (le_refl |(cosSign st.n : ℝ)|) ht (abs_nonneg _) (abs_nonneg _)
          linarith

/-- Every state the loop returns satisfies the invariant, with `bound = |t̂| + err`. -/
theorem seriesLoop_inv (x : ℝ) (stop : ℕ → ℤ → ℤ → ℤ → Bool) :
    ∀ (fuel : ℕ) (st : SeriesState) (st' : SeriesState) (b : ℤ), SeriesInv x st →
      seriesLoop x stop fuel st = some (st', b) → SeriesInv x st' ∧ b = |st'.t| + st'.tErr := by
  intro fuel
  induction fuel with
  | zero =>
    intro st st' b h hl
    unfold seriesLoop at hl
    split_ifs at hl
    · simp only [Option.some.injEq, Prod.mk.injEq] at hl
      obtain ⟨rfl, rfl⟩ := hl
      exact ⟨h, rfl⟩
  | succ f ih =>
    intro st st' b h hl
    unfold seriesLoop at hl
    split_ifs at hl
    · simp only [Option.some.injEq, Prod.mk.injEq] at hl
      obtain ⟨rfl, rfl⟩ := hl
      exact ⟨h, rfl⟩
    · exact ih _ st' b (seriesInv_step x st h) hl

/-- A value `y` with `|2^192·y - s| ≤ w` lies between the outward conversions of `s - w` and `s + w`. -/
theorem fixed_bracket (R : Rounding) (y : ℝ) (s w : ℤ) (h : |2 ^ 192 * y - s| ≤ (w : ℝ)) :
    R.fixedLower (s - w) ≤ y ∧ y ≤ R.fixedUpper (s + w) := by
  have hU : (0 : ℝ) < 2 ^ 192 := by positivity
  rw [abs_le] at h
  constructor
  · refine (R.fixedLower_le _).trans ?_
    rw [div_le_iff₀ hU]
    push_cast
    linarith
  · refine le_trans ?_ (R.le_fixedUpper _)
    rw [le_div_iff₀ hU]
    push_cast
    linarith

/-- **Soundness of the fixed-point Taylor sums.** Whenever the model of `sketch.sinCosSeriesBig` returns, the
first interval contains `sin x` and the second `cos x`, for every real `x`, every stopping test and every
fuel. The Go integer operations match the model's steps by `goTermStep_eq` and `goErrStep_eq`. Assumptions:
A20 and A21 (the integer operations), A23 (`fixedLower`, `fixedUpper`), A28 (float64 values as reals). -/
theorem sinCosSeriesBig_sound (R : Rounding) (x : ℝ) (stop : ℕ → ℤ → ℤ → ℤ → Bool) (fuel : ℕ) (S C : Ival)
    (h : sinCosSeriesBig R x stop fuel = some (S, C)) : S.Mem (sin x) ∧ C.Mem (cos x) := by
  unfold sinCosSeriesBig at h
  split at h
  · exact absurd h (by simp)
  rename_i st bound hl
  simp only [Option.some.injEq, Prod.mk.injEq] at h
  obtain ⟨rfl, rfl⟩ := h
  obtain ⟨⟨h0, ht, hs, hc⟩, hb⟩ := seriesLoop_inv x stop fuel _ st bound (seriesInv_init x) hl
  obtain ⟨tsin, tcos⟩ := abs_sin_cos_sub_taylor_le x st.n
  have hU : (0 : ℝ) < 2 ^ 192 := by positivity
  -- the exact term bounds the tail: 2^192·|x|^n/n! ≤ |t̂| + err = bound
  have htail : 2 ^ 192 * (|x| ^ st.n / st.n !) ≤ (bound : ℝ) := by
    rw [hb]
    push_cast
    have e : 2 ^ 192 * (|x| ^ st.n / st.n !) = |2 ^ 192 * x ^ st.n / (st.n ! : ℝ)| := by
      rw [abs_div, abs_mul, abs_pow, abs_pow, abs_two,
        abs_of_pos (by positivity : (0 : ℝ) < st.n !)]
      ring
    rw [e]
    have := abs_sub_abs_le_abs_sub (2 ^ 192 * x ^ st.n / (st.n ! : ℝ)) (st.t : ℝ)
    rw [abs_sub_comm] at ht
    linarith
  have key : ∀ (f sum : ℝ) (acc err : ℤ), |f - sum| ≤ |x| ^ st.n / st.n ! →
      |(acc : ℝ) - 2 ^ 192 * sum| ≤ (err : ℝ) → |2 ^ 192 * f - acc| ≤ ((err + bound : ℤ) : ℝ) := by
    intro f sum acc err hf hacc
    have e : 2 ^ 192 * f - acc = 2 ^ 192 * (f - sum) - (acc - 2 ^ 192 * sum) := by ring
    rw [e]
    push_cast
    calc _ ≤ |2 ^ 192 * (f - sum)| + |(acc : ℝ) - 2 ^ 192 * sum| := abs_sub _ _
      _ ≤ _ := by
          rw [abs_mul, abs_of_pos hU]
          have := mul_le_mul_of_nonneg_left hf hU.le
          linarith
  obtain ⟨s1, s2⟩ := fixed_bracket R _ _ _ (key _ _ _ _ tsin hs)
  obtain ⟨c1, c2⟩ := fixed_bracket R _ _ _ (key _ _ _ _ tcos hc)
  exact ⟨clampUnit_mem ⟨s1, s2⟩ (neg_one_le_sin x) (sin_le_one x),
    clampUnit_mem ⟨c1, c2⟩ (neg_one_le_cos x) (cos_le_one x)⟩

end Sketch.Trig

end
