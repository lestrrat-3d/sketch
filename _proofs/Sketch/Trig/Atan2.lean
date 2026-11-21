module

public import Sketch.Trig.Binary64
public import Sketch.Interval.Arith

/-!
# Direction angles: `twoPiTimes` and `atan2Point`

Models of `sketch.twoPiTimes` and `sketch.atan2Point`. A direction angle of a nonzero vector `(x, y)` is any
real `θ` with `(x, y) = r·(cos θ, sin θ)` for some `r > 0` (`Rep`); the direction angles of one vector differ
by whole turns, and each one equals `Complex.arg (x + y·i)` modulo `2π` (`rep_eq_arg`).

`sketch.atan2Point` takes `math.Atan2(c, d)` as an untrusted estimate `φ` and accepts `[lo, hi]` around it only
when interval bounds show the cross product `c·cos θ − d·sin θ` positive at `lo` and negative at `hi`. That
cross product is `|(d, c)|·sin(α − θ)` for a direction angle `α` of `(d, c)`, so a sign change between `lo` and
`hi` puts a direction angle of `(d, c)` inside `[lo, hi]`, with no assumption on the width of the bracket
(`exists_rep_of_cross`). The bounds come from one certified sine and cosine pair at `φ` and a second-order
Taylor remainder (`crossAt_sound`).

The interval helpers are those of `Sketch.Interval.Arith` over the abstract binary64 model `M`; the sine and
cosine enclosures and `twoPiTimes`'s rationals are those of `Sketch.Trig` over a `Rounding`.
-/

public section

namespace Sketch.Trig

open Real Sketch.Float
open Sketch.Interval (pt iadd isub ineg imul isqr up iadd_sound isub_sound ineg_sound imul_sound isqr_sound
  pt_sound up_sound)

/-! ### Direction angles -/

/-- `θ` is a direction angle of the vector `(x, y)`: `(x, y) = r·(cos θ, sin θ)` for some `r > 0`. -/
@[expose] def Rep (θ x y : ℝ) : Prop := ∃ r, 0 < r ∧ x = r * cos θ ∧ y = r * sin θ

/-- A direction angle stays one after adding whole turns. -/
theorem rep_add_int_mul {θ x y : ℝ} (h : Rep θ x y) (k : ℤ) : Rep (θ + k * (2 * π)) x y := by
  obtain ⟨r, hr, hx, hy⟩ := h
  exact ⟨r, hr, by rw [cos_add_int_mul_two_pi]; exact hx, by rw [sin_add_int_mul_two_pi]; exact hy⟩

/-- A direction angle of `(x, y)` is `Complex.arg (x + y·i)` modulo `2π`, the value `math.Atan2(y, x)`
approximates (A40). -/
theorem rep_eq_arg {θ x y : ℝ} (h : Rep θ x y) : (θ : Real.Angle) = Complex.arg ⟨x, y⟩ := by
  obtain ⟨r, hr, hx, hy⟩ := h
  have hz : (⟨x, y⟩ : ℂ) = (r : ℂ) * ((cos θ : ℂ) + (sin θ : ℂ) * Complex.I) := by
    apply Complex.ext <;> simp [hx, hy, Complex.cos_ofReal_re, Complex.sin_ofReal_re]
  have := Complex.arg_mul_cos_add_sin_mul_I_coe_angle hr (θ : Real.Angle)
  rw [Real.Angle.cos_coe, Real.Angle.sin_coe] at this
  rw [hz, this]

/-- Two direction angles of one vector differ by whole turns. -/
theorem rep_unique {θ θ' x y : ℝ} (h : Rep θ x y) (h' : Rep θ' x y) : ∃ k : ℤ, θ' = θ + k * (2 * π) := by
  obtain ⟨k, hk⟩ := Real.Angle.angle_eq_iff_two_pi_dvd_sub.mp ((rep_eq_arg h).trans (rep_eq_arg h').symm)
  exact ⟨-k, by push_cast; linear_combination -hk⟩

/-- Every nonzero vector has a direction angle, its argument. -/
theorem rep_arg {x y : ℝ} (h : ¬(y = 0 ∧ x = 0)) : Rep (Complex.arg ⟨x, y⟩) x y := by
  set z : ℂ := ⟨x, y⟩ with hzdef
  have hz : z ≠ 0 := fun h0 => h ⟨congrArg Complex.im h0, congrArg Complex.re h0⟩
  have e := Complex.norm_mul_cos_add_sin_mul_I z
  refine ⟨‖z‖, norm_pos_iff.mpr hz, ?_, ?_⟩
  · have := congrArg Complex.re e
    simp only [Complex.mul_re, Complex.ofReal_re, Complex.ofReal_im, Complex.add_re, Complex.add_im,
      Complex.mul_im, Complex.I_re, Complex.I_im, Complex.cos_ofReal_re, Complex.sin_ofReal_re,
      Complex.cos_ofReal_im, Complex.sin_ofReal_im, hzdef] at this
    linarith
  · have := congrArg Complex.im e
    simp only [Complex.mul_re, Complex.ofReal_re, Complex.ofReal_im, Complex.add_re, Complex.add_im,
      Complex.mul_im, Complex.I_re, Complex.I_im, Complex.cos_ofReal_re, Complex.sin_ofReal_re,
      Complex.cos_ofReal_im, Complex.sin_ofReal_im, hzdef] at this
    linarith

/-- **A sign change of the cross product brackets a direction angle.** If `c·cos θ − d·sin θ` is positive at
`lo` and negative at `hi ≥ lo`, a direction angle of `(d, c)` lies in `[lo, hi]`. No bound on `hi − lo` is
needed: the cross product is `|(d, c)|·sin(α − θ)`, and the first `θ ≥ lo` with `θ ≡ α` comes before the sine
turns negative. -/
theorem exists_rep_of_cross {c d lo hi : ℝ} (hle : lo ≤ hi) (hlo : 0 < c * cos lo - d * sin lo)
    (hhi : c * cos hi - d * sin hi < 0) : ∃ θ, lo ≤ θ ∧ θ ≤ hi ∧ Rep θ d c := by
  have hnz : ¬(c = 0 ∧ d = 0) := by
    rintro ⟨rfl, rfl⟩
    simp at hlo
  obtain ⟨r, hr, hd, hc⟩ := rep_arg hnz
  set α := Complex.arg ⟨d, c⟩
  have hf : ∀ θ, c * cos θ - d * sin θ = r * sin (α - θ) := by
    intro θ
    rw [hc, hd, sin_sub]
    ring
  have hp : (0 : ℝ) < 2 * π := two_pi_pos
  set β := toIcoMod hp 0 (α - lo) with hβdef
  have hβ : β ∈ Set.Ico 0 (2 * π) := by
    have := toIcoMod_mem_Ico hp 0 (α - lo)
    rwa [zero_add, ← hβdef] at this
  set k := toIcoDiv hp 0 (α - lo)
  have hβeq : β = α - lo - k * (2 * π) := by
    rw [hβdef, ← self_sub_toIcoDiv_zsmul hp 0 (α - lo), zsmul_eq_mul]
  have hsinβ : sin β = sin (α - lo) := by
    rw [hβeq, sin_sub_int_mul_two_pi]
  have hsin_lo : 0 < sin (α - lo) := by
    have := hf lo
    rw [this] at hlo
    exact pos_of_mul_pos_right hlo hr.le
  have hsin_hi : sin (α - hi) < 0 := by
    have := hf hi
    rw [this] at hhi
    exact neg_of_mul_neg_right hhi hr.le
  have hβpos : 0 < β := by
    rcases hβ.1.lt_or_eq with h | h
    · exact h
    · rw [← hsinβ, ← h, sin_zero] at hsin_lo
      exact absurd hsin_lo (lt_irrefl 0)
  have hβpi : β < π := by
    by_contra hge
    push Not at hge
    have : sin (β - π) ≥ 0 := sin_nonneg_of_nonneg_of_le_pi (by linarith) (by linarith [hβ.2])
    rw [sin_sub_pi] at this
    linarith
  refine ⟨lo + β, by linarith, ?_, ?_⟩
  · by_contra hgt
    push Not at hgt
    have h1 : 0 < sin (β - (hi - lo)) := sin_pos_of_pos_of_lt_pi (by linarith) (by linarith)
    have h2 : sin (β - (hi - lo)) = sin (α - hi) := by
      rw [show β - (hi - lo) = (α - hi) - k * (2 * π) by rw [hβeq]; ring, sin_sub_int_mul_two_pi]
    linarith
  · have e : lo + β = α + ((-k : ℤ) : ℝ) * (2 * π) := by rw [hβeq]; push_cast; ring
    rw [e]
    exact rep_add_int_mul ⟨r, hr, hd, hc⟩ (-k)

/-! ### Rounding to nearest is monotone -/

/-- Round to nearest never reverses an order: from `rn_near` alone, `rn y < rn x` with `x ≤ y` would put both
`x` and `y` at the midpoint of the two floats. -/
theorem rn_mono (M : FloatModel) {x y : ℝ} (h : x ≤ y) : M.rn x ≤ M.rn y := by
  by_contra hlt
  push Not at hlt
  have h1 := sq_le_sq.mpr (M.rn_near x (M.rn y) (M.rn_mem y))
  have h2 := sq_le_sq.mpr (M.rn_near y (M.rn x) (M.rn_mem x))
  have hxy : M.rn x + M.rn y ≤ 2 * x := by nlinarith
  have hyx : 2 * y ≤ M.rn x + M.rn y := by nlinarith
  have hxy' : x = y := le_antisymm h (by linarith)
  rw [hxy'] at hlt
  exact lt_irrefl _ hlt

/-! ### `twoPiTimes` -/

/-- Model of `sketch.twoPiTimes`: the exact rationals `2k·piLo` and `2k·piHi` in order, rounded outward once
by `sketch.ratInterval`. -/
@[expose] noncomputable def twoPiTimes (R : Rounding) (k : ℤ) : Ival :=
  let kk : ℝ := 2 * k
  let a := kk * piLo
  let b := kk * piHi
  if k < 0 then ratInterval R b a else ratInterval R a b

/-- **`twoPiTimes` encloses whole turns.** The model of `sketch.twoPiTimes` contains `k·2π` for the true π.
Covers `sketch.twoPiTimes` and `sketch.ratInterval`. Assumptions: A20, A22 (`piLo`, `piHi`), A24, A28, and
A70 (`2*k` does not overflow `int64`). -/
theorem twoPiTimes_sound (R : Rounding) (k : ℤ) : (twoPiTimes R k).Mem (k * (2 * π)) := by
  have hlo := piLo_lt_pi
  have hhi := pi_lt_piHi
  unfold twoPiTimes ratInterval
  split_ifs with hk
  · have hk' : (k : ℝ) < 0 := by exact_mod_cast hk
    refine ⟨(R.ratLower_le _).trans ?_, le_trans ?_ (R.le_ratUpper _)⟩ <;> nlinarith
  · have hk' : (0 : ℝ) ≤ k := by exact_mod_cast not_lt.mp hk
    refine ⟨(R.ratLower_le _).trans ?_, le_trans ?_ (R.le_ratUpper _)⟩ <;> nlinarith

/-! ### Second-order Taylor bounds -/

/-- Cosine misses its tangent line at `p` by at most `h²/2`. -/
theorem abs_cos_shift_le (p h : ℝ) : |cos (p + h) - (cos p - sin p * h)| ≤ h ^ 2 / 2 := by
  have hf : ContDiff ℝ ⊤ (fun y => cos (p + y)) := contDiff_cos.comp (contDiff_const.add contDiff_id)
  have hb : ∀ k y, |iteratedDeriv k (fun y => cos (p + y)) y| ≤ 1 := by
    intro k y
    rw [iteratedDeriv_comp_const_add]
    exact abs_iteratedDeriv_cos_le_one _ _
  have := abs_sub_taylor_le_of_deriv_bound hf hb h 1
  simp only [Finset.sum_range_succ, iteratedDeriv_comp_const_add, iteratedDeriv_zero, iteratedDeriv_one,
    Real.deriv_cos, add_zero, pow_zero, pow_one, Nat.factorial_zero, Nat.factorial_one, Nat.cast_one, div_one,
    mul_one] at this
  norm_num at this
  rwa [sub_eq_add_neg (cos p)]

/-- Sine misses its tangent line at `p` by at most `h²/2`. -/
theorem abs_sin_shift_le (p h : ℝ) : |sin (p + h) - (sin p + cos p * h)| ≤ h ^ 2 / 2 := by
  have hf : ContDiff ℝ ⊤ (fun y => sin (p + y)) := contDiff_sin.comp (contDiff_const.add contDiff_id)
  have hb : ∀ k y, |iteratedDeriv k (fun y => sin (p + y)) y| ≤ 1 := by
    intro k y
    rw [iteratedDeriv_comp_const_add]
    exact abs_iteratedDeriv_sin_le_one _ _
  have := abs_sub_taylor_le_of_deriv_bound hf hb h 1
  simp only [Finset.sum_range_succ, iteratedDeriv_comp_const_add, iteratedDeriv_zero, iteratedDeriv_one,
    Real.deriv_sin, add_zero, pow_zero, pow_one, Nat.factorial_zero, Nat.factorial_one, Nat.cast_one, div_one,
    mul_one] at this
  norm_num at this
  exact this

/-- The cross product `c·cos θ − d·sin θ` misses its tangent line at `p` by at most `(|c| + |d|)·h²/2`. -/
theorem abs_cross_shift_le (c d p h : ℝ) :
    |(c * cos (p + h) - d * sin (p + h)) - ((c * cos p - d * sin p) + h * -(c * sin p + d * cos p))| ≤
      |c| * (h ^ 2 / 2) + |d| * (h ^ 2 / 2) := by
  have e : (c * cos (p + h) - d * sin (p + h)) - ((c * cos p - d * sin p) + h * -(c * sin p + d * cos p)) =
      c * (cos (p + h) - (cos p - sin p * h)) - d * (sin (p + h) - (sin p + cos p * h)) := by ring
  rw [e]
  refine (abs_sub _ _).trans (add_le_add ?_ ?_) <;> rw [abs_mul]
  · exact mul_le_mul_of_nonneg_left (abs_cos_shift_le p h) (abs_nonneg c)
  · exact mul_le_mul_of_nonneg_left (abs_sin_shift_le p h) (abs_nonneg d)

/-! ### `atan2Point` -/

variable (M : FloatModel)

/-- A `Sketch.Trig` interval as a `Sketch.Interval` interval; both model `sketch.Interval`. -/
@[expose] def toIv (I : Ival) : Sketch.Interval.Ival := ⟨I.lo, I.hi⟩

/-- The closure `crossAt` of `sketch.atan2Point`: the cross product at `angle` from its value `base` and
slope `slope` at the estimate `phi`, widened by the second-order remainder `(|c| + |d|)·h²/2`. The float
division `isqr(h).Hi / 2` is `M.rn` of the exact quotient (A71). -/
@[expose] noncomputable def crossAt (c d phi : ℝ) (base slope : Sketch.Interval.Ival) (angle : ℝ) :
    Sketch.Interval.Ival :=
  let h := isub M (pt angle) (pt phi)
  let halfSq : Sketch.Interval.Ival := ⟨0, up M (M.rn ((isqr M h).hi / 2))⟩
  let tail := iadd M (imul M (pt |c|) halfSq) (imul M (pt |d|) halfSq)
  iadd M (iadd M base (imul M h slope)) ⟨-tail.hi, tail.hi⟩

/-- `crossAt` contains the exact cross product at `angle`, given enclosures of its value and slope at `phi`. -/
theorem crossAt_sound (c d phi : ℝ) {base slope : Sketch.Interval.Ival}
    (hbase : base.Mem (c * cos phi - d * sin phi)) (hslope : slope.Mem (-(c * sin phi + d * cos phi)))
    (angle : ℝ) : (crossAt M c d phi base slope angle).Mem (c * cos angle - d * sin angle) := by
  unfold crossAt
  dsimp only
  set h := angle - phi
  set hI := isub M (pt angle) (pt phi)
  set hs : Sketch.Interval.Ival := ⟨0, up M (M.rn ((isqr M hI).hi / 2))⟩
  set tl := iadd M (imul M (pt |c|) hs) (imul M (pt |d|) hs)
  set R := (c * cos angle - d * sin angle) - ((c * cos phi - d * sin phi) + h * -(c * sin phi + d * cos phi))
  have hh : hI.Mem h := isub_sound M (pt_sound angle) (pt_sound phi)
  have hsq : hs.Mem (h ^ 2 / 2) := by
    refine ⟨by positivity, ?_⟩
    have := (isqr_sound M hh).2
    refine le_trans ?_ (up_sound M _)
    rw [sq]
    linarith
  have htail : tl.Mem (|c| * (h ^ 2 / 2) + |d| * (h ^ 2 / 2)) :=
    iadd_sound M (imul_sound M (pt_sound |c|) hsq) (imul_sound M (pt_sound |d|) hsq)
  have hrem : |R| ≤ |c| * (h ^ 2 / 2) + |d| * (h ^ 2 / 2) := by
    have := abs_cross_shift_le c d phi h
    rwa [show phi + h = angle by ring] at this
  have hR : (⟨-tl.hi, tl.hi⟩ : Sketch.Interval.Ival).Mem R :=
    ⟨by linarith [neg_abs_le R, htail.2], by linarith [le_abs_self R, htail.2]⟩
  have := iadd_sound M (iadd_sound M hbase (imul_sound M hh hslope)) hR
  convert this using 1
  ring

/-- The bracket loop of `sketch.atan2Point` over the passes `ks`: pass `k` tries `[rn(φ − δ k), rn(φ + δ k)]`
and returns it when the cross product is positive at the lower end and negative at the upper end. -/
@[expose] noncomputable def atan2Search (c d phi : ℝ) (base slope : Sketch.Interval.Ival) (delta : ℕ → ℝ) :
    List ℕ → Option Sketch.Interval.Ival
  | [] => none
  | k :: ks =>
    let lo := M.rn (phi - delta k)
    let hi := M.rn (phi + delta k)
    if 0 < (crossAt M c d phi base slope lo).lo ∧ (crossAt M c d phi base slope hi).hi < 0 then some ⟨lo, hi⟩
    else atan2Search c d phi base slope delta ks

/-- The float64 decisions of `sketch.atan2Point`, taken as inputs: the estimate `atan2 c d` is
`math.Atan2(c, d)` (any real; it is never trusted), and `delta φ k` is `math.Ldexp((1+math.Abs(φ))*1e-15, 4*k)`,
which is never negative (A71). -/
structure Atan2Env where
  atan2 : ℝ → ℝ → ℝ
  delta : ℝ → ℕ → ℝ
  delta_nonneg : ∀ φ k, 0 ≤ delta φ k

/-- Model of `sketch.atan2Point`. `P` is the point enclosure of sine and cosine (`sketch.sinCosPoint`).
Deliberate differences: `math.Atan2`'s NaN result is not modeled (A28), and the estimate and half-widths are
the inputs of `Atan2Env`. -/
@[expose] noncomputable def atan2Point (P : ℝ → Option (Ival × Ival)) (env : Atan2Env) (c d : ℝ) :
    Option Sketch.Interval.Ival :=
  if c = 0 ∧ d = 0 then none
  else
    let phi := env.atan2 c d
    match P phi with
    | none => none
    | some (s, co) =>
      let base := isub M (imul M (pt c) (toIv co)) (imul M (pt d) (toIv s))
      let slope := ineg (iadd M (imul M (pt c) (toIv s)) (imul M (pt d) (toIv co)))
      atan2Search M c d phi base slope (env.delta phi) (List.range 6)

theorem atan2Search_sound (c d phi : ℝ) {base slope : Sketch.Interval.Ival}
    (hbase : base.Mem (c * cos phi - d * sin phi)) (hslope : slope.Mem (-(c * sin phi + d * cos phi)))
    (delta : ℕ → ℝ) (hδ : ∀ k, 0 ≤ delta k) :
    ∀ (ks : List ℕ) {I : Sketch.Interval.Ival}, atan2Search M c d phi base slope delta ks = some I →
      ∃ θ, I.Mem θ ∧ Rep θ d c := by
  intro ks
  induction ks with
  | nil => intro I h; exact absurd h (by simp [atan2Search])
  | cons k ks ih =>
    intro I h
    unfold atan2Search at h
    dsimp only at h
    split_ifs at h with hacc
    · simp only [Option.some.injEq] at h
      subst h
      have hl := crossAt_sound M c d phi hbase hslope (M.rn (phi - delta k))
      have hu := crossAt_sound M c d phi hbase hslope (M.rn (phi + delta k))
      have hle : M.rn (phi - delta k) ≤ M.rn (phi + delta k) := rn_mono M (by linarith [hδ k])
      obtain ⟨θ, h1, h2, hθ⟩ := exists_rep_of_cross hle (lt_of_lt_of_le hacc.1 hl.1)
        (lt_of_le_of_lt hu.2 hacc.2)
      exact ⟨θ, ⟨h1, h2⟩, hθ⟩
    · exact ih h

/-- **Soundness of the direction bracket.** Whenever the model of `sketch.atan2Point` returns an interval, it
contains a direction angle of the vector `(d, c)`, so it contains `Complex.arg (d + c·i)` modulo `2π`
(`rep_eq_arg`), whatever value `math.Atan2` returned and however wide the bracket is. `hP` is the soundness of
the sine and cosine enclosure (`sinCosPoint_sound`). Covers `sketch.atan2Point` and its interval helpers
(`sketch.isub`, `sketch.iadd`, `sketch.ineg`, `sketch.imul`, `sketch.isqr`, `sketch.up`, `sketch.pt`).
Assumptions: A1 to A6 (interval helpers), A28, A71 (`Atan2Env`, the halving division). -/
theorem atan2Point_sound (P : ℝ → Option (Ival × Ival))
    (hP : ∀ y S C, P y = some (S, C) → S.Mem (sin y) ∧ C.Mem (cos y)) (env : Atan2Env) (c d : ℝ)
    {I : Sketch.Interval.Ival} (h : atan2Point M P env c d = some I) : ∃ θ, I.Mem θ ∧ Rep θ d c := by
  unfold atan2Point at h
  split_ifs at h with hz
  dsimp only at h
  split at h
  · exact absurd h (by simp)
  rename_i s co hPs
  obtain ⟨hs, hc⟩ := hP _ _ _ hPs
  have hbase := isub_sound M (imul_sound M (pt_sound c) (show (toIv co).Mem _ from hc))
    (imul_sound M (pt_sound d) (show (toIv s).Mem _ from hs))
  have hslope := ineg_sound (iadd_sound M (imul_sound M (pt_sound c) (show (toIv s).Mem _ from hs))
    (imul_sound M (pt_sound d) (show (toIv co).Mem _ from hc)))
  exact atan2Search_sound M c d _ hbase hslope _ (env.delta_nonneg _) _ h

/-- **Direction bracket on binary64.** `sketch.atan2Point` with the certified `sketch.sinCosPoint` of
`interval_trig.go`: a returned interval contains a direction angle of `(d, c)`. Covers `sketch.atan2Point`,
`sketch.sinCosPoint` and every function they reach. Assumptions: A1 to A6, A20 to A28, A71. -/
theorem atan2Point_sound_binary64 (E : TrigEnv) (fuel : ℕ) (env : Atan2Env) (c d : ℝ)
    {I : Sketch.Interval.Ival}
    (h : atan2Point M (sinCosPoint (Rounding.ofFloat M) E fuel) env c d = some I) :
    ∃ θ, I.Mem θ ∧ Rep θ d c :=
  atan2Point_sound M _ (sinCosPoint_sound (Rounding.ofFloat M) E fuel) env c d h

end Sketch.Trig

end
