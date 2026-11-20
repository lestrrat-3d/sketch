module

public import Sketch.Trig.Pi
public import Sketch.Trig.Rounding

/-!
# Whole-turn reduction and the π/2 brackets

Models of `sketch.ratInterval`, `sketch.reduceTurns`, `sketch.halfPiRats` and `sketch.halfPiMayLieIn`. All four
compute with exact rationals over the bracket `piLo < π < piHi` (`Sketch.Trig.Pi`), so each holds its exact
target for the true π: `reduceTurns` encloses `x - 2πk` for any integer `k`, `halfPiRats` brackets `k·π/2`,
and `halfPiMayLieIn` answers true whenever `k·π/2` lies in `q`. Only the final conversions to float64 round,
through `Rounding` (A24). The table `halfPiTable` that `halfPiMayLieIn` reads for `|k| ≤ halfPiTableK` is a
package variable, so its initializer is assumption A22; `halfPiEntry` is that initializer.
-/

public section

namespace Sketch.Trig

open Real

/-- Model of `sketch.ratInterval`. -/
@[expose] def ratInterval (R : Rounding) (lo hi : ℝ) : Ival := ⟨R.ratLower lo, R.ratUpper hi⟩

/-- Model of `sketch.reduceTurns`. `k` is the float64 `int64(math.Round(x / (2 * math.Pi)))`
(`TrigEnv.turns`); the result is sound for any integer `k`. -/
@[expose] noncomputable def reduceTurns (R : Rounding) (x : ℝ) (k : ℤ) : Ival :=
  let kk : ℝ := 2 * k
  let a := x - kk * piHi
  let b := x - kk * piLo
  if k < 0 then ratInterval R b a else ratInterval R a b

/-- **Soundness of the whole-turn reduction.** The model of `sketch.reduceTurns` (through
`sketch.ratInterval`) encloses `x - k·2π` for the true π and any integer `k`, so its sine and cosine are those
of `x`. Assumptions: A20 (exact `big.Rat` arithmetic), A22 (the literals `piLo`, `piHi`), A24 (`ratLower`,
`ratUpper`), A28 (float64 values as reals). -/
theorem reduceTurns_sound (R : Rounding) (x : ℝ) (k : ℤ) :
    (reduceTurns R x k).Mem (x - k * (2 * π)) ∧
      sin (x - k * (2 * π)) = sin x ∧ cos (x - k * (2 * π)) = cos x := by
  refine ⟨?_, sin_sub_int_mul_two_pi x k, cos_sub_int_mul_two_pi x k⟩
  have hlo := piLo_lt_pi
  have hhi := pi_lt_piHi
  unfold reduceTurns ratInterval
  split_ifs with hk
  · have hk' : (k : ℝ) < 0 := by exact_mod_cast hk
    refine ⟨(R.ratLower_le _).trans ?_, le_trans ?_ (R.le_ratUpper _)⟩ <;> nlinarith
  · have hk' : (0 : ℝ) ≤ k := by exact_mod_cast not_lt.mp hk
    refine ⟨(R.ratLower_le _).trans ?_, le_trans ?_ (R.le_ratUpper _)⟩ <;> nlinarith

/-- Model of `sketch.halfPiRats`: the exact bracket `(a, b)` of `k·π/2`. -/
@[expose] noncomputable def halfPiRats (k : ℤ) : ℝ × ℝ :=
  let half : ℝ := k / 2
  let a := half * piLo
  let b := half * piHi
  if k < 0 then (b, a) else (a, b)

/-- The model of `sketch.halfPiRats` brackets `k·π/2` for the true π. Assumptions: A20, A22. -/
theorem halfPiRats_bracket (k : ℤ) : (halfPiRats k).1 ≤ k * π / 2 ∧ k * π / 2 ≤ (halfPiRats k).2 := by
  have hlo := piLo_lt_pi
  have hhi := pi_lt_piHi
  unfold halfPiRats
  split_ifs with hk
  · have hk' : (k : ℝ) < 0 := by exact_mod_cast hk
    constructor <;> nlinarith
  · have hk' : (0 : ℝ) ≤ k := by exact_mod_cast not_lt.mp hk
    constructor <;> nlinarith

/-- Model of `sketch.halfPiBracket`. -/
structure HalfPiBracket where
  lo : ℝ
  hi : ℝ
  loExact : Bool
  hiExact : Bool

/-- The entry `halfPiTable[k + halfPiTableK]` as the initializer of `sketch.halfPiTable` builds it from
`halfPiRats k` (A22). -/
@[expose] noncomputable def halfPiEntry (R : Rounding) (k : ℤ) : HalfPiBracket :=
  ⟨R.ratLower (halfPiRats k).1, R.ratUpper (halfPiRats k).2, R.ratExact (halfPiRats k).1,
    R.ratExact (halfPiRats k).2⟩

/-- Model of `sketch.halfPiMayLieIn` with `halfPiTableK = 128` (A22). -/
@[expose] noncomputable def halfPiMayLieIn (R : Rounding) (k : ℤ) (q : Ival) : Bool :=
  if k < -128 ∨ k > 128 then
    decide (q.lo ≤ (halfPiRats k).2) && decide ((halfPiRats k).1 ≤ q.hi)
  else
    let br := halfPiEntry R k
    let bBelow := decide (br.hi < q.lo) || (!br.hiExact && decide (br.hi = q.lo))
    let aAbove := decide (br.lo > q.hi) || (!br.loExact && decide (br.lo = q.hi))
    !bBelow && !aAbove

/-- **The π/2 test never misses.** The model of `sketch.halfPiMayLieIn` answers true whenever `k·π/2` lies in
`q` for the true π, on both the exact path and the table path. Assumptions: A20, A22 (the table initializer
and `halfPiTableK`), A24 (`ratLower`, `ratUpper` and the `exact` flag), A28. -/
theorem halfPiMayLieIn_of_mem (R : Rounding) (k : ℤ) (q : Ival) (h1 : q.lo ≤ k * π / 2)
    (h2 : k * π / 2 ≤ q.hi) : halfPiMayLieIn R k q = true := by
  obtain ⟨ha, hb⟩ := halfPiRats_bracket k
  unfold halfPiMayLieIn
  split_ifs
  · simp only [Bool.and_eq_true, decide_eq_true_eq]
    exact ⟨h1.trans hb, ha.trans h2⟩
  · have hhi := R.le_ratUpper (halfPiRats k).2
    have hlo := R.ratLower_le (halfPiRats k).1
    have hb1 : ¬R.ratUpper (halfPiRats k).2 < q.lo := not_lt.mpr (h1.trans (hb.trans hhi))
    have ha1 : ¬R.ratLower (halfPiRats k).1 > q.hi := not_lt.mpr (hlo.trans (ha.trans h2))
    have hb2 : R.ratExact (halfPiRats k).2 = false → R.ratUpper (halfPiRats k).2 ≠ q.lo :=
      fun hx heq => R.ratUpper_ne _ hx (le_antisymm (by rw [heq]; exact h1.trans hb) hhi)
    have ha2 : R.ratExact (halfPiRats k).1 = false → R.ratLower (halfPiRats k).1 ≠ q.hi :=
      fun hx heq => R.ratLower_ne _ hx (le_antisymm hlo (by rw [heq]; exact ha.trans h2))
    have hB : (decide ((halfPiEntry R k).hi < q.lo) ||
        (!(halfPiEntry R k).hiExact && decide ((halfPiEntry R k).hi = q.lo))) = false := by
      rw [Bool.or_eq_false_iff]
      refine ⟨decide_eq_false hb1, ?_⟩
      cases hx : (halfPiEntry R k).hiExact
      · rw [Bool.not_false, Bool.true_and]
        exact decide_eq_false (hb2 hx)
      · rw [Bool.not_true, Bool.false_and]
    have hA : (decide ((halfPiEntry R k).lo > q.hi) ||
        (!(halfPiEntry R k).loExact && decide ((halfPiEntry R k).lo = q.hi))) = false := by
      rw [Bool.or_eq_false_iff]
      refine ⟨decide_eq_false ha1, ?_⟩
      cases hx : (halfPiEntry R k).loExact
      · rw [Bool.not_false, Bool.true_and]
        exact decide_eq_false (ha2 hx)
      · rw [Bool.not_true, Bool.false_and]
    simp only [hB, hA, Bool.not_false, Bool.and_self]

end Sketch.Trig

end
