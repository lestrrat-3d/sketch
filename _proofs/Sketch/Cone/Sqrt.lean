module

public import Sketch.Cone.Real
public import Sketch.Cone.Wedge

/-!
# Exact square-root bounds, unit enclosures and interval products

Models `sketch.coneSqrtBounds`, `sketch.coneUnit` and `sketch.coneMulInterval` (`cone_side_arc.go`).
`coneSqrtBounds` brackets `√q` between two rationals built from the integer square root of `q·4^k`, with no
float step; `coneUnit` turns that bracket into coordinate-wise intervals around `v/|v|`; `coneMulInterval`
is the product interval: two end products when the first factor is nonnegative, else the hull of the four
corner products.

`coneSqrtBounds_sound`, `coneUnit_sound` and `coneMulInterval_memR` prove each one inclusion-sound over the
reals. The `math/big` integer steps are exact (A51) and the rational steps are exact (A30, A51).
-/

public section

namespace Sketch.Cone

/-- The precision exponent `k` of `sketch.coneSqrtBounds`: `coneSqrtBits − (num.BitLen() − den.BitLen())/2`,
with `Nat.size` as `big.Int.BitLen` and Go's truncating `int` division as `Int.tdiv`. -/
@[expose] def sqrtK (q : ℚ) : ℤ := 100 - Int.tdiv ((Nat.size q.num.toNat : ℤ) - (Nat.size q.den : ℤ)) 2

/-- The shifted numerator of `sketch.coneSqrtBounds`: `num·4^k` when `k ≥ 0` (`Lsh` by `2k`). -/
@[expose] def sqrtNum (q : ℚ) : ℕ := if 0 ≤ sqrtK q then q.num.toNat * 2 ^ (2 * (sqrtK q).toNat) else q.num.toNat

/-- The shifted denominator of `sketch.coneSqrtBounds`: `den·4^(−k)` when `k < 0`. -/
@[expose] def sqrtDen (q : ℚ) : ℕ := if 0 ≤ sqrtK q then q.den else q.den * 2 ^ (2 * (-sqrtK q).toNat)

/-- `⌊√⌊q·4^k⌋⌋` as `sketch.coneSqrtBounds` computes it: `Quo` of positive integers is `ℕ` division and
`big.Int.Sqrt` is `Nat.sqrt`. -/
@[expose] def sqrtRoot (q : ℚ) : ℕ := Nat.sqrt (sqrtNum q / sqrtDen q)

/-- The scale `2^(−k)` of `sketch.coneSqrtBounds`. -/
@[expose] def sqrtScale (q : ℚ) : ℚ := if 0 ≤ sqrtK q then 1 / 2 ^ (sqrtK q).toNat else 2 ^ (-sqrtK q).toNat

/-- Model of `sketch.coneSqrtBounds`, through `sqrtK`, `sqrtNum`, `sqrtDen`, `sqrtRoot` and `sqrtScale`
(A51). Returns `some (lo, hi)` exactly when the Go function reports true. -/
@[expose] def coneSqrtBounds (q : ℚ) : Option (ℚ × ℚ) :=
  if q ≤ 0 then none
  else if sqrtRoot q = 0 then none
  else some (sqrtRoot q * sqrtScale q, (sqrtRoot q + 1) * sqrtScale q)

/-- The integer square root of `⌊c⌋`, scaled by `scale = 1/t`, brackets `√(c/t²)`. -/
theorem sqrt_bracket {m n root : ℕ} {q t scale : ℚ} (hn : 0 < n) (hroot : root = Nat.sqrt (m / n))
    (hr0 : root ≠ 0) (hc : (m : ℚ) / n = q * (t * t)) (hst : scale * t = 1) (hs : 0 < scale) :
    0 < root * scale ∧ 0 < (root + 1) * scale ∧ (root * scale) * (root * scale) ≤ q ∧
      q ≤ ((root + 1) * scale) * ((root + 1) * scale) := by
  have h1 : root ^ 2 ≤ m / n := hroot ▸ Nat.sqrt_le' (m / n)
  have h2 : m / n < (root + 1) ^ 2 := hroot ▸ Nat.lt_succ_sqrt' (m / n)
  have hnq : (0 : ℚ) < n := by exact_mod_cast hn
  have hfl : ((m / n : ℕ) : ℚ) ≤ (m : ℚ) / n := Nat.cast_div_le
  have hcl : (m : ℚ) / n < ((m / n : ℕ) : ℚ) + 1 := by
    have hdm := Nat.div_add_mod m n
    have hml := Nat.mod_lt m hn
    have e : (m : ℚ) = n * ((m / n : ℕ) : ℚ) + ((m % n : ℕ) : ℚ) := by exact_mod_cast hdm.symm
    have hml' : ((m % n : ℕ) : ℚ) < n := by exact_mod_cast hml
    rw [div_lt_iff₀ hnq, e]
    nlinarith
  have h1' : ((root : ℚ)) ^ 2 ≤ ((m / n : ℕ) : ℚ) := by exact_mod_cast h1
  have h2' : ((m / n : ℕ) : ℚ) + 1 ≤ ((root : ℚ) + 1) ^ 2 := by exact_mod_cast h2
  have hrpos : (0 : ℚ) < root := by exact_mod_cast Nat.pos_of_ne_zero hr0
  have hq : q = (m : ℚ) / n * (scale * scale) := by
    rw [hc]
    have : q * (t * t) * (scale * scale) = q * ((scale * t) * (scale * t)) := by ring
    rw [this, hst]
    ring
  refine ⟨mul_pos hrpos hs, by positivity, ?_, ?_⟩
  · rw [hq]
    have : (root : ℚ) * root ≤ (m : ℚ) / n := by nlinarith
    have hss : 0 ≤ scale * scale := by positivity
    calc (root * scale) * (root * scale) = (root * root) * (scale * scale) := by ring
      _ ≤ (m : ℚ) / n * (scale * scale) := mul_le_mul_of_nonneg_right this hss
  · rw [hq]
    have : (m : ℚ) / n ≤ ((root : ℚ) + 1) * ((root : ℚ) + 1) := by nlinarith
    have hss : 0 ≤ scale * scale := by positivity
    calc (m : ℚ) / n * (scale * scale) ≤ (((root : ℚ) + 1) * ((root : ℚ) + 1)) * (scale * scale) :=
          mul_le_mul_of_nonneg_right this hss
      _ = ((root + 1) * scale) * ((root + 1) * scale) := by ring

/-- `coneSqrtBounds` brackets `√q`: `0 < lo`, `0 < hi`, `lo² ≤ q ≤ hi²`. Covers `sketch.coneSqrtBounds`;
A30, A51. -/
theorem coneSqrtBounds_sound {q lo hi : ℚ} (h : coneSqrtBounds q = some (lo, hi)) :
    0 < lo ∧ 0 < hi ∧ lo * lo ≤ q ∧ q ≤ hi * hi := by
  unfold coneSqrtBounds at h
  split_ifs at h with hq hr
  simp only [Option.some.injEq, Prod.mk.injEq] at h
  obtain ⟨rfl, rfl⟩ := h
  have hqpos : 0 < q := lt_of_not_ge hq
  have hnum : 0 < q.num := Rat.num_pos.2 hqpos
  have hnumc : ((q.num.toNat : ℕ) : ℚ) = (q.num : ℚ) := by
    rw [show ((q.num.toNat : ℕ) : ℚ) = (((q.num.toNat : ℕ) : ℤ) : ℚ) by norm_cast, Int.toNat_of_nonneg hnum.le]
  have hqd : (q.num : ℚ) / q.den = q := Rat.num_div_den q
  by_cases h0 : 0 ≤ sqrtK q
  · have hsc : sqrtScale q = 1 / 2 ^ (sqrtK q).toNat := by simp only [sqrtScale, h0, ↓reduceIte]
    rw [hsc]
    refine sqrt_bracket (q := q) (t := 2 ^ (sqrtK q).toNat) (m := sqrtNum q) (n := sqrtDen q)
      (by simp only [sqrtDen, h0, ↓reduceIte]; exact q.den_pos) rfl hr ?_ ?_ (by positivity)
    · simp only [sqrtNum, sqrtDen, h0, ↓reduceIte]
      generalize (sqrtK q).toNat = K
      push_cast
      rw [hnumc]
      conv_rhs => rw [← hqd]
      field_simp
      ring
    · field_simp
  · have hsc : sqrtScale q = 2 ^ (-sqrtK q).toNat := by simp only [sqrtScale, h0, ↓reduceIte]
    rw [hsc]
    refine sqrt_bracket (q := q) (t := 1 / 2 ^ (-sqrtK q).toNat) (m := sqrtNum q) (n := sqrtDen q)
      (by simp only [sqrtDen, h0, ↓reduceIte]; positivity) rfl hr ?_ ?_ (by positivity)
    · simp only [sqrtNum, sqrtDen, h0, ↓reduceIte]
      generalize (-sqrtK q).toNat = K
      push_cast
      rw [hnumc]
      conv_rhs => rw [← hqd]
      field_simp
      ring
    · field_simp

/-- Model of `sketch.coneUnit`. `Inv` is `x⁻¹` (A51). -/
@[expose] def coneUnit (v : ℚ × ℚ) : Option (ConeInterval × ConeInterval) :=
  match coneSqrtBounds (v.1 * v.1 + v.2 * v.2) with
  | none => none
  | some (lo, hi) =>
    let inv : ConeInterval := ⟨hi⁻¹, lo⁻¹⟩
    some (inv.scale v.1, inv.scale v.2)

/-- The real ray of an exact rational ray. -/
@[expose] def rayR (v : ℚ × ℚ) : ℝ × ℝ := ((v.1 : ℝ), (v.2 : ℝ))

/-- The bracket `coneSqrtBounds` returns for `|v|²` holds `|v|`, and `|v| > 0`. -/
theorem coneSqrtBounds_memR_normR {v : ℚ × ℚ} {lo hi : ℚ}
    (h : coneSqrtBounds (v.1 * v.1 + v.2 * v.2) = some (lo, hi)) :
    0 < (lo : ℝ) ∧ (⟨lo, hi⟩ : ConeInterval).MemR (normR (rayR v)) := by
  obtain ⟨hlo, hhi, hlo2, hhi2⟩ := coneSqrtBounds_sound h
  have hq : normR (rayR v) = Real.sqrt (((v.1 * v.1 + v.2 * v.2 : ℚ) : ℝ)) := by
    simp only [normR, rayR]
    push_cast
    rfl
  have hloR : (0 : ℝ) < lo := by exact_mod_cast hlo
  have hhiR : (0 : ℝ) < hi := by exact_mod_cast hhi
  have hlo2R : ((lo : ℝ)) * lo ≤ ((v.1 * v.1 + v.2 * v.2 : ℚ) : ℝ) := by exact_mod_cast hlo2
  have hhi2R : ((v.1 * v.1 + v.2 * v.2 : ℚ) : ℝ) ≤ (hi : ℝ) * hi := by exact_mod_cast hhi2
  refine ⟨hloR, ?_, ?_⟩
  · rw [hq]
    exact Real.le_sqrt_of_sq_le (by nlinarith)
  · rw [hq]
    exact Real.sqrt_le_iff.2 ⟨hhiR.le, by nlinarith⟩

/-- `coneUnit` encloses `v/|v|` coordinate-wise. Covers `sketch.coneUnit` and `sketch.coneSqrtBounds`;
A30, A51. -/
theorem coneUnit_sound {v : ℚ × ℚ} {ix iy : ConeInterval} (h : coneUnit v = some (ix, iy)) :
    ix.MemR (unitR (rayR v)).1 ∧ iy.MemR (unitR (rayR v)).2 := by
  unfold coneUnit at h
  split at h
  · simp at h
  · rename_i lo hi hb
    simp only [Option.some.injEq, Prod.mk.injEq] at h
    obtain ⟨rfl, rfl⟩ := h
    obtain ⟨hloR, hs1, hs2⟩ := coneSqrtBounds_memR_normR hb
    have hnpos : 0 < normR (rayR v) := lt_of_lt_of_le hloR hs1
    have hinv : (⟨hi⁻¹, lo⁻¹⟩ : ConeInterval).MemR (normR (rayR v))⁻¹ := by
      unfold ConeInterval.MemR
      push_cast
      exact ⟨inv_anti₀ hnpos hs2, inv_anti₀ hloR hs1⟩
    exact ⟨ConeInterval.memR_scale v.1 hinv, ConeInterval.memR_scale v.2 hinv⟩

/-- One step of the Go loop in `coneMulInterval`: lower `lo` when `x < lo`, raise `hi` when `x > hi`. -/
@[expose] def mulStep (lh : ℚ × ℚ) (x : ℚ) : ℚ × ℚ :=
  (if x < lh.1 then x else lh.1, if lh.2 < x then x else lh.2)

/-- Model of `sketch.coneMulInterval`. When `a.lo ≥ 0`, every value of `a` is nonnegative, so the lower end is
`b.lo` times `a.lo` (`a.hi` when `b.lo < 0`) and the upper end is `b.hi` times `a.hi` (`a.lo` when `b.hi < 0`).
Otherwise it is the hull of the four corner products, in the Go loop order. -/
@[expose] def coneMulInterval (a b : ConeInterval) : ConeInterval :=
  if 0 ≤ a.lo then ⟨(if b.lo < 0 then a.hi else a.lo) * b.lo, (if b.hi < 0 then a.lo else a.hi) * b.hi⟩
  else
    let p0 := a.lo * b.lo
    let r := mulStep (mulStep (mulStep (p0, p0) (a.lo * b.hi)) (a.hi * b.lo)) (a.hi * b.hi)
    ⟨r.1, r.2⟩

theorem mulStep_eq (lh : ℚ × ℚ) (x : ℚ) : mulStep lh x = (min x lh.1, max lh.2 x) := by
  simp only [mulStep, ConeInterval.ite_lt_eq_min, ConeInterval.ite_gt_eq_max]

/-- A product of a real between `l` and `h` lies between the two end products. -/
theorem mul_between {l h x y : ℝ} (h1 : l ≤ x) (h2 : x ≤ h) :
    min (l * y) (h * y) ≤ x * y ∧ x * y ≤ max (l * y) (h * y) := by
  rcases le_total 0 y with hy | hy
  · exact ⟨min_le_of_left_le (mul_le_mul_of_nonneg_right h1 hy),
      le_max_of_le_right (mul_le_mul_of_nonneg_right h2 hy)⟩
  · exact ⟨min_le_of_right_le (mul_le_mul_of_nonpos_right h2 hy),
      le_max_of_le_left (mul_le_mul_of_nonpos_right h1 hy)⟩

/-- `coneMulInterval` is inclusion-sound for reals. Covers `sketch.coneMulInterval`; A30. -/
theorem coneMulInterval_memR {a b : ConeInterval} {x y : ℝ} (hx : a.MemR x) (hy : b.MemR y) :
    (coneMulInterval a b).MemR (x * y) := by
  obtain ⟨hx1, hx2⟩ := hx
  obtain ⟨hy1, hy2⟩ := hy
  by_cases ha : 0 ≤ a.lo
  · have hx0 : (0 : ℝ) ≤ x := le_trans (by exact_mod_cast ha) hx1
    have hlo := mul_le_mul_of_nonneg_left hy1 hx0
    have hhi := mul_le_mul_of_nonneg_left hy2 hx0
    simp only [coneMulInterval, ha, ↓reduceIte, ConeInterval.MemR]
    constructor
    · split_ifs with hb
      · have hb' : (b.lo : ℝ) < 0 := by exact_mod_cast hb
        push_cast
        nlinarith
      · have hb' : (0 : ℝ) ≤ b.lo := by exact_mod_cast not_lt.mp hb
        push_cast
        nlinarith
    · split_ifs with hb
      · have hb' : (b.hi : ℝ) < 0 := by exact_mod_cast hb
        push_cast
        nlinarith
      · have hb' : (0 : ℝ) ≤ b.hi := by exact_mod_cast not_lt.mp hb
        push_cast
        nlinarith
  obtain ⟨l1, u1⟩ := mul_between (y := y) hx1 hx2
  obtain ⟨l2, u2⟩ := mul_between (y := (a.lo : ℝ)) hy1 hy2
  obtain ⟨l3, u3⟩ := mul_between (y := (a.hi : ℝ)) hy1 hy2
  simp only [coneMulInterval, ha, ↓reduceIte, mulStep_eq, ConeInterval.MemR]
  push_cast
  rw [min_le_iff] at l1 l2 l3
  rw [le_max_iff] at u1 u2 u3
  constructor
  · simp only [min_le_iff]
    rcases l1 with l1 | l1
    · rcases l2 with l2 | l2
      · right; right; right; linarith
      · right; right; left; linarith
    · rcases l3 with l3 | l3
      · right; left; linarith
      · left; linarith
  · simp only [le_max_iff]
    rcases u1 with u1 | u1
    · rcases u2 with u2 | u2
      · left; left; left; linarith
      · left; left; right; linarith
    · rcases u3 with u3 | u3
      · left; right; linarith
      · right; linarith

end Sketch.Cone

end
