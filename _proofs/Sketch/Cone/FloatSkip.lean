module

public import Sketch.Cone.Arc
public import Sketch.Cone.Bezier
public import Sketch.Interval.Arith

/-!
# Float prefilter of the cone box proofs

Models the float prefilter of `cone_side_float.go`. It sits in front of the exact box test of
`sketch.coneSideForm.bezierSide` (through `sketch.coneSideForm.floatSkip`) and of `sketch.coneArcWedge.side`
(through `sketch.coneArcWedge.boxZero`). When it reports a skip, the exact box test is not run and the piece
is bisected as if the exact test had found no side.

Each exact rational endpoint is tracked by a float interval that encloses it (`Fiv`, built from the
outward-rounded helpers of `Sketch.Interval`). `zeroBox_sound` proves that when the float enclosures of the
box report a skip, the exact `boxSide` returns `(zero, none)`. `boxZeroW_sound` proves the same for the exact
`wedgeBox`, which adds the unit-direction ranges, the radius product, the shift by the center and
`coneOutward`. `bezierSideP_eq` and `wedgeSideP_eq` then prove the Go call sites, which skip the exact box when
the prefilter fires, compute exactly the result of the exact-only models `bezierSide` and `wedgeSide`: the
prefilter changes no side and no margin.

Overflow and NaN are not modeled (A2): the Go code refuses, and does not skip, on a non-finite enclosure.
-/

public section

namespace Sketch.Cone

open Sketch.Float Sketch.Interval

variable (M : FloatModel)

/-! ## Float enclosures of exact intervals -/

/-- Model of `sketch.fiv`: float intervals enclosing the exact lower and upper endpoints of a `ConeInterval`. -/
structure Fiv where
  lo : Ival
  hi : Ival

/-- The float enclosures `a` hold the endpoints of the exact interval `c`. -/
@[expose] def Fiv.Enc (a : Fiv) (c : ConeInterval) : Prop := a.lo.Mem (c.lo : ℝ) ∧ a.hi.Mem (c.hi : ℝ)

/-- Model of `sketch.ratEnclose`. `big.Rat.Float64` is `M.rn` of the rational and its exact flag is
`M.rn r = r` (A24); an exact result is a point, any other is widened one float each way. -/
@[expose] noncomputable def ratEnclose (r : ℚ) : Ival :=
  if M.rn r = r then pt (M.rn r) else ⟨down M (M.rn r), up M (M.rn r)⟩

/-- The enclosure of a rational holds it. Covers `sketch.ratEnclose`; A24, A1-A3. -/
theorem ratEnclose_sound (r : ℚ) : (ratEnclose M r).Mem (r : ℝ) := by
  unfold ratEnclose
  split_ifs with h
  · rw [h]; exact pt_sound _
  · exact ⟨down_sound M _, up_sound M _⟩

/-- Model of `sketch.fivAdd`. -/
@[expose] def fivAdd (a b : Fiv) : Fiv := ⟨iadd M a.lo b.lo, iadd M a.hi b.hi⟩

/-- Model of `sketch.fivSub`. -/
@[expose] def fivSub (a b : Fiv) : Fiv := ⟨isub M a.lo b.hi, isub M a.hi b.lo⟩

/-- Model of `sketch.ivMin`. -/
@[expose] def ivMin (a b : Ival) : Ival := ⟨min a.lo b.lo, min a.hi b.hi⟩

/-- Model of `sketch.ivMax`. -/
@[expose] def ivMax (a b : Ival) : Ival := ⟨max a.lo b.lo, max a.hi b.hi⟩

/-- Model of `sketch.fivScale`. -/
@[expose] noncomputable def fivScale (a : Fiv) (k : Ival) : Fiv :=
  ⟨ivMin (imul M a.lo k) (imul M a.hi k), ivMax (imul M a.lo k) (imul M a.hi k)⟩

/-- Model of `sketch.fivSquare`. The non-finite refusal only refuses, so the model omits it. -/
@[expose] noncomputable def fivSquare (a : Fiv) : Fiv :=
  if 0 ≤ a.lo.lo then ⟨isqr M a.lo, isqr M a.hi⟩
  else if a.lo.hi < 0 ∧ a.hi.hi ≤ 0 then ⟨isqr M a.hi, isqr M a.lo⟩
  else if a.lo.hi < 0 ∧ 0 < a.hi.lo then ⟨pt 0, ivMax (isqr M a.lo) (isqr M a.hi)⟩
  else ⟨⟨0, max (isqr M a.lo).hi (isqr M a.hi).hi⟩, ⟨0, max (isqr M a.lo).hi (isqr M a.hi).hi⟩⟩

/-- The minimum of a point of `a` and a point of `b` lies in `ivMin a b`. Covers `sketch.ivMin`; A6. -/
theorem ivMin_mem {a b : Ival} {x y : ℝ} (hx : a.Mem x) (hy : b.Mem y) : (ivMin a b).Mem (min x y) :=
  ⟨min_le_min hx.1 hy.1, min_le_min hx.2 hy.2⟩

/-- The maximum of a point of `a` and a point of `b` lies in `ivMax a b`. Covers `sketch.ivMax`; A6. -/
theorem ivMax_mem {a b : Ival} {x y : ℝ} (hx : a.Mem x) (hy : b.Mem y) : (ivMax a b).Mem (max x y) :=
  ⟨max_le_max hx.1 hy.1, max_le_max hx.2 hy.2⟩

/-- `fivAdd` encloses the exact interval sum. Covers `sketch.fivAdd`; A1-A3. -/
theorem enc_add {a b : Fiv} {c d : ConeInterval} (ha : a.Enc c) (hb : b.Enc d) :
    (fivAdd M a b).Enc (c.add d) := by
  refine ⟨?_, ?_⟩
  · simp only [ConeInterval.add, Rat.cast_add]; exact iadd_sound M ha.1 hb.1
  · simp only [ConeInterval.add, Rat.cast_add]; exact iadd_sound M ha.2 hb.2

/-- `fivSub` encloses the exact interval difference. Covers `sketch.fivSub`; A1-A3. -/
theorem enc_sub {a b : Fiv} {c d : ConeInterval} (ha : a.Enc c) (hb : b.Enc d) :
    (fivSub M a b).Enc (c.sub d) := by
  refine ⟨?_, ?_⟩
  · simp only [ConeInterval.sub, Rat.cast_sub]; exact isub_sound M ha.1 hb.2
  · simp only [ConeInterval.sub, Rat.cast_sub]; exact isub_sound M ha.2 hb.1

theorem coneIntervalOf_eq (a b : ℚ) : coneIntervalOf a b = ⟨min a b, max a b⟩ := by
  unfold coneIntervalOf
  split_ifs with h
  · rw [min_eq_left h, max_eq_right h]
  · have h' := le_of_not_ge h
    rw [min_eq_right h', max_eq_left h']

/-- `fivScale` encloses the exact scaled interval for a factor `k` of either sign. Covers `sketch.fivScale`,
`sketch.ivMin`, `sketch.ivMax`; A1-A4, A6. -/
theorem enc_scale {a : Fiv} {c : ConeInterval} {kk : Ival} {k : ℚ} (ha : a.Enc c) (hk : kk.Mem (k : ℝ)) :
    (fivScale M a kk).Enc (c.scale k) := by
  have p := imul_sound M ha.1 hk
  have q := imul_sound M ha.2 hk
  have hmin := ivMin_mem p q
  have hmax := ivMax_mem p q
  unfold ConeInterval.scale
  rw [coneIntervalOf_eq]
  refine ⟨?_, ?_⟩
  · simpa [fivScale, Rat.cast_min, Rat.cast_mul] using hmin
  · simpa [fivScale, Rat.cast_max, Rat.cast_mul] using hmax

/-- `fivSquare` encloses the exact interval square, across all four branches of the Go code and of
`coneInterval.square`. Covers `sketch.fivSquare`, `sketch.ivMax`; A1-A4, A6. -/
theorem enc_square {a : Fiv} {c : ConeInterval} (ha : a.Enc c) : (fivSquare M a).Enc c.square := by
  obtain ⟨hx, hy⟩ := ha
  have l := isqr_sound M hx
  have h := isqr_sound M hy
  by_cases g1 : 0 ≤ a.lo.lo
  · have e1 : 0 ≤ c.lo := by
      have : (0 : ℝ) ≤ c.lo := le_trans g1 hx.1
      exact_mod_cast this
    unfold fivSquare ConeInterval.square
    simp only [eq_true g1, eq_true e1, ↓reduceIte]
    simp only [Fiv.Enc, Rat.cast_mul]
    exact ⟨l, h⟩
  by_cases g2 : a.lo.hi < 0 ∧ a.hi.hi ≤ 0
  · have e1 : ¬ 0 ≤ c.lo := by
      have : (c.lo : ℝ) < 0 := lt_of_le_of_lt hx.2 g2.1
      exact not_le.2 (by exact_mod_cast this)
    have e2 : c.hi ≤ 0 := by
      have : (c.hi : ℝ) ≤ 0 := le_trans hy.2 g2.2
      exact_mod_cast this
    unfold fivSquare ConeInterval.square
    simp only [eq_false g1, eq_true g2, eq_false e1, eq_true e2, ↓reduceIte]
    simp only [Fiv.Enc, Rat.cast_mul]
    exact ⟨h, l⟩
  by_cases g3 : a.lo.hi < 0 ∧ 0 < a.hi.lo
  · have e1 : ¬ 0 ≤ c.lo := by
      have : (c.lo : ℝ) < 0 := lt_of_le_of_lt hx.2 g3.1
      exact not_le.2 (by exact_mod_cast this)
    have e2 : ¬ c.hi ≤ 0 := by
      have : (0 : ℝ) < c.hi := lt_of_lt_of_le g3.2 hy.1
      exact not_le.2 (by exact_mod_cast this)
    have hm := ivMax_mem l h
    unfold fivSquare ConeInterval.square
    simp only [eq_false g1, eq_false g2, eq_true g3, eq_false e1, eq_false e2, ↓reduceIte]
    by_cases e3 : c.lo * c.lo < c.hi * c.hi
    · simp only [eq_true e3, ↓reduceIte]
      have e3' : ((c.lo : ℝ) * c.lo) < (c.hi : ℝ) * c.hi := by exact_mod_cast e3
      refine ⟨⟨by simp [pt], by simp [pt]⟩, ?_⟩
      have : max ((c.lo : ℝ) * c.lo) (c.hi * c.hi) = c.hi * c.hi := max_eq_right e3'.le
      rw [this] at hm
      simpa using hm
    · simp only [eq_false e3, ↓reduceIte]
      have e3' : ((c.hi : ℝ) * c.hi) ≤ (c.lo : ℝ) * c.lo := by exact_mod_cast le_of_not_gt e3
      refine ⟨⟨by simp [pt], by simp [pt]⟩, ?_⟩
      have : max ((c.lo : ℝ) * c.lo) (c.hi * c.hi) = c.lo * c.lo := max_eq_left e3'
      rw [this] at hm
      simpa using hm
  · have hxx := mul_self_nonneg (c.lo : ℝ)
    have hyy := mul_self_nonneg (c.hi : ℝ)
    have hx2 : (c.lo : ℝ) * c.lo ≤ max (isqr M a.lo).hi (isqr M a.hi).hi :=
      l.2.trans (le_max_left _ _)
    have hy2 : (c.hi : ℝ) * c.hi ≤ max (isqr M a.lo).hi (isqr M a.hi).hi :=
      h.2.trans (le_max_right _ _)
    unfold fivSquare ConeInterval.square
    simp only [eq_false g1, eq_false g2, eq_false g3, ↓reduceIte]
    unfold Fiv.Enc Ival.Mem
    split_ifs <;> push_cast
    all_goals
      refine ⟨⟨?_, ?_⟩, ⟨?_, ?_⟩⟩ <;> first | assumption | linarith

/-! ## The float form and the box test -/

/-- Model of `sketch.coneFloatForm`: a float enclosure of each field of a `ConeSideForm`. -/
structure FloatForm where
  base : Fin 3 → Ival
  u : Fin 3 → Ival
  v : Fin 3 → Ival
  axis : Fin 3 → Ival
  slope2 : Ival

/-- Every field of the form lies in its float enclosure. -/
@[expose] def FloatForm.Enc (g : FloatForm) (f : ConeSideForm) : Prop :=
  (∀ i, (g.base i).Mem (f.base i : ℝ)) ∧ (∀ i, (g.u i).Mem (f.u i : ℝ)) ∧ (∀ i, (g.v i).Mem (f.v i : ℝ)) ∧
    (∀ i, (g.axis i).Mem (f.axis i : ℝ)) ∧ g.slope2.Mem (f.slope2 : ℝ)

/-- Model of `sketch.newConeFloatForm`. The non-finite refusal only disables the prefilter, so the model
omits it. -/
@[expose] noncomputable def newConeFloatForm (f : ConeSideForm) : FloatForm where
  base := fun i => ratEnclose M (f.base i)
  u := fun i => ratEnclose M (f.u i)
  v := fun i => ratEnclose M (f.v i)
  axis := fun i => ratEnclose M (f.axis i)
  slope2 := ratEnclose M f.slope2

/-- The enclosures `newConeFloatForm` builds hold every field of the form. Covers `sketch.newConeFloatForm`,
`sketch.ratEnclose`; A24, A1-A3. -/
theorem newConeFloatForm_enc (f : ConeSideForm) : (newConeFloatForm M f).Enc f :=
  ⟨fun _ => ratEnclose_sound M _, fun _ => ratEnclose_sound M _, fun _ => ratEnclose_sound M _,
    fun _ => ratEnclose_sound M _, ratEnclose_sound M _⟩

/-- Float enclosure of `boxQ`, as `zeroBox` builds it. -/
@[expose] noncomputable def fboxQ (g : FloatForm) (u v : Fiv) (i : Fin 3) : Fiv :=
  fivAdd M (fivAdd M ⟨g.base i, g.base i⟩ (fivScale M u (g.u i))) (fivScale M v (g.v i))

/-- Float enclosure of `boxH`. -/
@[expose] noncomputable def fboxH (g : FloatForm) (u v : Fiv) : Fiv :=
  fivAdd M (fivAdd M (fivScale M (fboxQ M g u v 0) (g.axis 0)) (fivScale M (fboxQ M g u v 1) (g.axis 1)))
    (fivScale M (fboxQ M g u v 2) (g.axis 2))

/-- Float enclosure of `boxResidual`. -/
@[expose] noncomputable def fboxResidual (g : FloatForm) (u v : Fiv) : Fiv :=
  let q := fboxQ M g u v
  let cx := fivSub M (fivScale M (q 1) (g.axis 2)) (fivScale M (q 2) (g.axis 1))
  let cy := fivSub M (fivScale M (q 2) (g.axis 0)) (fivScale M (q 0) (g.axis 2))
  let cz := fivSub M (fivScale M (q 0) (g.axis 1)) (fivScale M (q 1) (g.axis 0))
  fivSub M (fivAdd M (fivAdd M (fivSquare M cx) (fivSquare M cy)) (fivSquare M cz))
    (fivScale M (fivSquare M (fboxH M g u v)) g.slope2)

/-- Model of `coneFloatForm.zeroBox`. Go also refuses when an enclosure is not finite; the model omits that
refusal, which only turns a true into a false (A2). -/
@[expose] def zeroBox (g : FloatForm) (u v : Fiv) : Prop :=
  (fboxH M g u v).lo.hi ≤ 0 ∨
    ((fboxResidual M g u v).lo.hi ≤ 0 ∧ 0 ≤ (fboxResidual M g u v).hi.lo)

theorem enc_fboxQ {g : FloatForm} {f : ConeSideForm} (hg : g.Enc f) {u v : Fiv} {cu cv : ConeInterval}
    (hu : u.Enc cu) (hv : v.Enc cv) (i : Fin 3) : (fboxQ M g u v i).Enc (f.boxQ cu cv i) :=
  enc_add M (enc_add M ⟨hg.1 i, hg.1 i⟩ (enc_scale M hu (hg.2.1 i))) (enc_scale M hv (hg.2.2.1 i))

theorem enc_fboxH {g : FloatForm} {f : ConeSideForm} (hg : g.Enc f) {u v : Fiv} {cu cv : ConeInterval}
    (hu : u.Enc cu) (hv : v.Enc cv) : (fboxH M g u v).Enc (f.boxH cu cv) :=
  enc_add M (enc_add M (enc_scale M (enc_fboxQ M hg hu hv 0) (hg.2.2.2.1 0))
    (enc_scale M (enc_fboxQ M hg hu hv 1) (hg.2.2.2.1 1))) (enc_scale M (enc_fboxQ M hg hu hv 2) (hg.2.2.2.1 2))

theorem enc_fboxResidual {g : FloatForm} {f : ConeSideForm} (hg : g.Enc f) {u v : Fiv} {cu cv : ConeInterval}
    (hu : u.Enc cu) (hv : v.Enc cv) : (fboxResidual M g u v).Enc (f.boxResidual cu cv) := by
  have q := enc_fboxQ M hg hu hv
  have ax := hg.2.2.2.1
  exact enc_sub M (enc_add M (enc_add M
    (enc_square M (enc_sub M (enc_scale M (q 1) (ax 2)) (enc_scale M (q 2) (ax 1))))
    (enc_square M (enc_sub M (enc_scale M (q 2) (ax 0)) (enc_scale M (q 0) (ax 2)))))
    (enc_square M (enc_sub M (enc_scale M (q 0) (ax 1)) (enc_scale M (q 1) (ax 0)))))
    (enc_scale M (enc_square M (enc_fboxH M hg hu hv)) hg.2.2.2.2)

/-- When the float prefilter reports a skip, the exact `boxSide` finds no side on the box whose endpoints the
enclosures `u`, `v` hold. Either the half coordinate's exact lower bound is at most the float upper bound
`(fboxH).lo.hi ≤ 0`, or the exact residual interval straddles zero. Covers `sketch.coneFloatForm.zeroBox`,
`sketch.fivAdd`, `sketch.fivSub`, `sketch.fivScale`, `sketch.fivSquare`, `sketch.ivMin`, `sketch.ivMax`;
A1-A4, A6, A2. -/
theorem zeroBox_sound {g : FloatForm} {f : ConeSideForm} (hg : g.Enc f) {u v : Fiv} {cu cv : ConeInterval}
    (hu : u.Enc cu) (hv : v.Enc cv) (hz : zeroBox M g u v) : f.boxSide cu cv = (.zero, none) := by
  have hH := enc_fboxH M hg hu hv
  have hR := enc_fboxResidual M hg hu hv
  unfold ConeSideForm.boxSide
  rcases hz with hz | ⟨hz1, hz2⟩
  · have : (f.boxH cu cv).lo ≤ 0 := by
      have h1 : ((f.boxH cu cv).lo : ℝ) ≤ 0 := le_trans hH.1.2 hz
      exact_mod_cast h1
    simp [this]
  · have h1 : (f.boxResidual cu cv).lo ≤ 0 := by
      have : ((f.boxResidual cu cv).lo : ℝ) ≤ 0 := le_trans hR.1.2 hz1
      exact_mod_cast this
    have h2 : 0 ≤ (f.boxResidual cu cv).hi := by
      have : (0 : ℝ) ≤ (f.boxResidual cu cv).hi := le_trans hz2 hR.2.1
      exact_mod_cast this
    split_ifs with c1 c2 c3
    · rfl
    · exact absurd c2 (not_lt.2 h1)
    · exact absurd c3 (not_lt.2 h2)
    · rfl

/-! ## The Bézier call site -/

/-- Model of `sketch.coneSideForm.floatSkip`: false without a prefilter, else `zeroBox` on the float
enclosures of the box endpoints. -/
@[expose] def floatSkip (fp : Option FloatForm) (u v : ConeInterval) : Prop :=
  match fp with
  | none => False
  | some g => zeroBox M g ⟨ratEnclose M u.lo, ratEnclose M u.hi⟩ ⟨ratEnclose M v.lo, ratEnclose M v.hi⟩

/-- When `floatSkip` holds, the exact `boxSide` finds no side. Covers `sketch.coneSideForm.floatSkip`,
`sketch.ratEnclose` and the functions `zeroBox_sound` lists; A24, A1-A4, A6, A2. -/
theorem floatSkip_sound {fp : Option FloatForm} {f : ConeSideForm} (hg : ∀ g, fp = some g → g.Enc f)
    {u v : ConeInterval} (h : floatSkip M fp u v) : f.boxSide u v = (.zero, none) := by
  cases fp with
  | none => exact h.elim
  | some g =>
    exact zeroBox_sound M (hg g rfl) ⟨ratEnclose_sound M _, ratEnclose_sound M _⟩
      ⟨ratEnclose_sound M _, ratEnclose_sound M _⟩ h

open Classical in
/-- Model of `sketch.coneSideForm.bezierSide` with the float prefilter: the exact box test runs only when
`floatSkip` is false, and a skip counts as no side. Otherwise the same as `bezierSide`. -/
@[expose] noncomputable def bezierSideP (fp : Option FloatForm) (f : ConeSideForm) :
    ConeBezier → ℕ → ℕ → ConeSide × Option ℚ × ℕ
  | _, _, 0 => (.zero, none, 0)
  | b, depth, budget + 1 =>
    let r : ConeSide × Option ℚ :=
      if floatSkip M fp b.box.1 b.box.2 then (.zero, none) else f.boxSide b.box.1 b.box.2
    if r.1 ≠ .zero then (r.1, r.2, budget)
    else
      match depth with
      | 0 => (.zero, none, budget)
      | d + 1 =>
        let left := bezierSideP fp f b.halves.1 d budget
        let right := bezierSideP fp f b.halves.2 d left.2.2
        if left.1 ≠ right.1 then (.zero, none, right.2.2)
        else if left.1 = .zero then (.zero, none, right.2.2)
        else
          match left.2.1, right.2.1 with
          | some ma, some mc => (left.1, some (if ma ≤ mc then ma else mc), right.2.2)
          | _, _ => (.zero, none, right.2.2)

open Classical in
/-- The Go `bezierSide`, which skips the exact box when the float prefilter fires, computes exactly the
result of the exact-only model `bezierSide`: the same side, margin and remaining budget. Covers
`sketch.coneSideForm.bezierSide`, `sketch.coneSideForm.floatSkip`; the assumptions of `floatSkip_sound`. -/
theorem bezierSideP_eq {fp : Option FloatForm} {f : ConeSideForm} (hg : ∀ g, fp = some g → g.Enc f) :
    ∀ (depth budget : ℕ) (b : ConeBezier), bezierSideP M fp f b depth budget = f.bezierSide b depth budget := by
  have hr : ∀ b : ConeBezier, (if floatSkip M fp b.box.1 b.box.2 then ((.zero, none) : ConeSide × Option ℚ)
      else f.boxSide b.box.1 b.box.2) = f.boxSide b.box.1 b.box.2 := by
    intro b
    split_ifs with hs
    · exact (floatSkip_sound M hg hs).symm
    · rfl
  intro depth
  induction depth with
  | zero =>
    intro budget b
    cases budget with
    | zero => simp [bezierSideP, ConeSideForm.bezierSide]
    | succ n => simp only [bezierSideP, ConeSideForm.bezierSide, hr]
  | succ d ih =>
    intro budget b
    cases budget with
    | zero => simp [bezierSideP, ConeSideForm.bezierSide]
    | succ n =>
      simp only [bezierSideP, ConeSideForm.bezierSide, hr, ih]
      rfl

/-! ## The wedge call site -/

/-- Model of the sign of a rational as `big.Rat.Sign`. -/
@[expose] def sgn (q : ℚ) : ℤ := if 0 < q then 1 else if q < 0 then -1 else 0

/-- Model of `sketch.axisCrossSign`: the sign of the cross product `a × e` for an axis direction `e`. -/
@[expose] def axisCrossSign (a : ℚ × ℚ) (e : ℤ × ℤ) : ℤ :=
  if e.2 ≠ 0 then sgn a.1 * e.2 else -sgn a.2 * e.1

/-- Model of `sketch.axisInWedge`. -/
@[expose] def axisInWedge (a b : ℚ × ℚ) (e : ℤ × ℤ) : Bool :=
  decide (0 ≤ axisCrossSign a e) && decide (axisCrossSign b e ≤ 0)

/-- For the four axis directions, `axisInWedge` agrees with the exact `coneWedgeHas`. Covers
`sketch.axisInWedge`, `sketch.axisCrossSign`; A30. -/
theorem axisInWedge_eq (a b : ℚ × ℚ) :
    axisInWedge a b (1, 0) = coneWedgeHas a b (1, 0) ∧ axisInWedge a b (-1, 0) = coneWedgeHas a b (-1, 0) ∧
    axisInWedge a b (0, 1) = coneWedgeHas a b (0, 1) ∧ axisInWedge a b (0, -1) = coneWedgeHas a b (0, -1) := by
  rcases lt_trichotomy a.1 0 with h1 | h1 | h1 <;> rcases lt_trichotomy a.2 0 with h2 | h2 | h2 <;>
  rcases lt_trichotomy b.1 0 with h3 | h3 | h3 <;> rcases lt_trichotomy b.2 0 with h4 | h4 | h4 <;>
  grind [axisInWedge, axisCrossSign, sgn, coneWedgeHas, coneCross]

/-- Model of the span of one coordinate in `sketch.coneArcWedge.boxZero`: the hull of the endpoint
enclosures, widened to `1` or `-1` when the wedge holds the axis direction. -/
@[expose] noncomputable def fspan (a b : ℚ × ℚ) (fa fb : Fiv) (pos neg : ℤ × ℤ) : Fiv :=
  ⟨if axisInWedge a b neg then pt (-1) else ivMin fa.lo fb.lo,
   if axisInWedge a b pos then pt 1 else ivMax fa.hi fb.hi⟩

/-- `fspan` encloses the exact `wedgeSpan` when the axis tests agree. -/
theorem enc_fspan {a b : ℚ × ℚ} {fa fb : Fiv} {ua ub : ConeInterval} {pos neg : ℤ × ℤ} {pos' neg' : ℚ × ℚ}
    (hp : axisInWedge a b pos = coneWedgeHas a b pos') (hn : axisInWedge a b neg = coneWedgeHas a b neg')
    (ha : fa.Enc ua) (hb : fb.Enc ub) : (fspan a b fa fb pos neg).Enc (wedgeSpan a b ua ub pos' neg') := by
  unfold fspan wedgeSpan Fiv.Enc
  rw [hp, hn]
  rw [ConeInterval.ite_lt_eq_min, ConeInterval.ite_gt_eq_max]
  have hmin := ivMin_mem ha.1 hb.1
  have hmax := ivMax_mem ha.2 hb.2
  rw [min_comm ub.lo ua.lo]
  refine ⟨?_, ?_⟩
  · cases coneWedgeHas a b neg'
    · simpa [Rat.cast_min] using hmin
    · simp [pt, Ival.Mem]
  · cases coneWedgeHas a b pos'
    · simpa [Rat.cast_max] using hmax
    · simp [pt, Ival.Mem]

/-- Model of the radius product in `sketch.coneArcWedge.boxZero`: `coneMulInterval` for a nonnegative radius
bracket, each end one corner product chosen by the sign of the span end. `none` is the Go `return false`. -/
@[expose] noncomputable def spanMul (rad s : Fiv) : Option Fiv :=
  if 0 ≤ rad.lo.lo ∧ (0 ≤ s.lo.lo ∨ s.lo.hi < 0) ∧ (0 ≤ s.hi.lo ∨ s.hi.hi < 0) then
    some ⟨if 0 ≤ s.lo.lo then imul M rad.lo s.lo else imul M rad.hi s.lo,
          if 0 ≤ s.hi.lo then imul M rad.hi s.hi else imul M rad.lo s.hi⟩
  else none

/-- A product `spanMul` returns encloses the exact `coneMulInterval`. Covers `sketch.coneArcWedge.boxZero`
(its radius product), `sketch.coneMulInterval`; A1-A4, A6. -/
theorem enc_spanMul {rad s r : Fiv} {radius b : ConeInterval} (hrad : rad.Enc radius) (hs : s.Enc b)
    (h : spanMul M rad s = some r) : r.Enc (coneMulInterval radius b) := by
  unfold spanMul at h
  by_cases hc : 0 ≤ rad.lo.lo ∧ (0 ≤ s.lo.lo ∨ s.lo.hi < 0) ∧ (0 ≤ s.hi.lo ∨ s.hi.hi < 0)
  · obtain ⟨h0, hl, hh⟩ := hc
    simp only [eq_true (And.intro h0 (And.intro hl hh)), ↓reduceIte, Option.some.injEq] at h
    subst h
    have h0' : 0 ≤ radius.lo := by
      have : (0 : ℝ) ≤ radius.lo := le_trans h0 hrad.1.1
      exact_mod_cast this
    unfold coneMulInterval
    simp only [eq_true h0', ↓reduceIte]
    refine ⟨?_, ?_⟩
    · rcases hl with hl | hl
      · have hb : ¬ b.lo < 0 := by
          have : (0 : ℝ) ≤ b.lo := le_trans hl hs.1.1
          exact not_lt.2 (by exact_mod_cast this)
        simp only [eq_true hl, eq_false hb, ↓reduceIte]
        push_cast
        exact imul_sound M hrad.1 hs.1
      · have hb : b.lo < 0 := by
          have : (b.lo : ℝ) < 0 := lt_of_le_of_lt hs.1.2 hl
          exact_mod_cast this
        have hl' : ¬ 0 ≤ s.lo.lo := by
          intro h'
          have : (0 : ℝ) ≤ b.lo := le_trans h' hs.1.1
          linarith [(by exact_mod_cast hb : (b.lo : ℝ) < 0)]
        simp only [eq_false hl', eq_true hb, ↓reduceIte]
        push_cast
        exact imul_sound M hrad.2 hs.1
    · rcases hh with hh | hh
      · have hb : ¬ b.hi < 0 := by
          have : (0 : ℝ) ≤ b.hi := le_trans hh hs.2.1
          exact not_lt.2 (by exact_mod_cast this)
        simp only [eq_true hh, eq_false hb, ↓reduceIte]
        push_cast
        exact imul_sound M hrad.2 hs.2
      · have hb : b.hi < 0 := by
          have : (b.hi : ℝ) < 0 := lt_of_le_of_lt hs.2.2 hh
          exact_mod_cast this
        have hh' : ¬ 0 ≤ s.hi.lo := by
          intro h'
          have : (0 : ℝ) ≤ b.hi := le_trans h' hs.2.1
          linarith [(by exact_mod_cast hb : (b.hi : ℝ) < 0)]
        simp only [eq_false hh', eq_true hb, ↓reduceIte]
        push_cast
        exact imul_sound M hrad.1 hs.2
  · simp only [eq_false hc, ↓reduceIte] at h
    exact absurd h (by simp)

/-- The float operations of `ConeArcEnv` are the `FloatModel`'s: `big.Rat.Float64` is `M.rn`, and
`math.Nextafter` toward `∓Inf` is `M.nd`, `M.nu` on floats (A200). -/
structure EnvAgrees (E : ConeArcEnv) : Prop where
  rn : ∀ q : ℚ, (E.rn q : ℝ) = M.rn q
  prev : ∀ q : ℚ, (q : ℝ) ∈ M.F → (E.prev q : ℝ) = M.nd q
  next : ∀ q : ℚ, (q : ℝ) ∈ M.F → (E.next q : ℝ) = M.nu q

/-- A float below a float `g` is at most the next float down from `g`. -/
theorem le_nd_of_lt {xl g : ℝ} (hxl : xl ∈ M.F) (hg : g ∈ M.F) (h : xl < g) : xl ≤ M.nd g := by
  have := M.nu_min (-g) (M.neg_mem _ hg) (-xl) (M.neg_mem _ hxl) (by linarith)
  unfold FloatModel.nd
  linarith

/-- `coneOutward` keeps its lower bound within the floats: at least a float `xl` at most the input's lower
bound, and at most that bound. Covers `sketch.coneOutward`; A24, A50, A200. -/
theorem outward_lo {E : ConeArcEnv} (hE : EnvAgrees M E) {i : ConeInterval} {xl : ℝ} (hxl : xl ∈ M.F)
    (h1 : xl ≤ i.lo) : xl ≤ (coneOutward E i).lo ∧ ((coneOutward E i).lo : ℝ) ≤ i.lo := by
  have hr := hE.rn i.lo
  have hrm : (E.rn i.lo : ℝ) ∈ M.F := by rw [hr]; exact M.rn_mem _
  unfold coneOutward
  dsimp only
  split_ifs with h
  · have hlt : (i.lo : ℝ) < E.rn i.lo := by exact_mod_cast h
    refine ⟨?_, by exact_mod_cast E.prev_rn_le i.lo⟩
    show xl ≤ ((E.prev (E.rn i.lo) : ℚ) : ℝ)
    rw [hE.prev _ hrm, hr]
    exact le_nd_of_lt M hxl (M.rn_mem _) (by rw [hr] at hlt; linarith)
  · have hle : (E.rn i.lo : ℝ) ≤ i.lo := by exact_mod_cast le_of_not_gt h
    refine ⟨?_, hle⟩
    have hn := M.rn_near i.lo xl hxl
    rw [← hr] at hn
    rw [abs_of_nonpos (by linarith : (E.rn i.lo : ℝ) - i.lo ≤ 0), abs_of_nonpos (by linarith : xl - i.lo ≤ 0)] at hn
    linarith

/-- `coneOutward` keeps its upper bound within the floats: at least the input's upper bound, and at most a
float `xh` at least that bound. Covers `sketch.coneOutward`; A24, A50, A200. -/
theorem outward_hi {E : ConeArcEnv} (hE : EnvAgrees M E) {i : ConeInterval} {xh : ℝ} (hxh : xh ∈ M.F)
    (h1 : (i.hi : ℝ) ≤ xh) : (i.hi : ℝ) ≤ (coneOutward E i).hi ∧ ((coneOutward E i).hi : ℝ) ≤ xh := by
  have hr := hE.rn i.hi
  have hrm : (E.rn i.hi : ℝ) ∈ M.F := by rw [hr]; exact M.rn_mem _
  unfold coneOutward
  dsimp only
  split_ifs with h
  · have hlt : (E.rn i.hi : ℝ) < i.hi := by exact_mod_cast h
    refine ⟨by exact_mod_cast E.le_next_rn i.hi, ?_⟩
    show ((E.next (E.rn i.hi) : ℚ) : ℝ) ≤ xh
    rw [hE.next _ hrm, hr]
    rw [hr] at hlt
    exact M.nu_min _ (M.rn_mem _) xh hxh (by linarith)
  · have hle : (i.hi : ℝ) ≤ E.rn i.hi := by exact_mod_cast le_of_not_gt h
    refine ⟨hle, ?_⟩
    have hn := M.rn_near i.hi xh hxh
    rw [← hr] at hn
    rw [abs_of_nonneg (by linarith : 0 ≤ (E.rn i.hi : ℝ) - i.hi), abs_of_nonneg (by linarith : 0 ≤ xh - i.hi)] at hn
    linarith

theorem iadd_lo_mem (a b : Ival) : (iadd M a b).lo ∈ M.F := M.nd_mem (M.rn_mem _)

theorem iadd_hi_mem (a b : Ival) : (iadd M a b).hi ∈ M.F := M.nu_mem _ (M.rn_mem _)

/-- Widening to the float outward bounds keeps the enclosure of the box endpoints, so `coneOutward` of an
enclosed interval is enclosed by the same floats. Covers `sketch.coneOutward`; A24, A50, A200. -/
theorem enc_outward {E : ConeArcEnv} (hE : EnvAgrees M E) {x : Fiv} {i : ConeInterval} (hx : x.Enc i)
    (hlo : x.lo.lo ∈ M.F) (hhi : x.hi.hi ∈ M.F) : x.Enc (coneOutward E i) := by
  obtain ⟨l1, l2⟩ := outward_lo M hE hlo hx.1.1
  obtain ⟨h1, h2⟩ := outward_hi M hE hhi hx.2.2
  exact ⟨⟨l1, l2.trans hx.1.2⟩, ⟨hx.2.1.trans h1, h2⟩⟩

/-- Model of `sketch.coneArcFloat`: the form enclosure, the radius bracket and the center. -/
structure ArcFloat where
  form : FloatForm
  radius : Fiv
  cx : Ival
  cy : Ival

/-- The arc float state holds the exact form, radius bracket and center. -/
@[expose] def ArcFloat.Enc (w : ArcFloat) (f : ConeSideForm) (radius : ConeInterval) (cx cy : ℚ) : Prop :=
  w.form.Enc f ∧ w.radius.Enc radius ∧ w.cx.Mem (cx : ℝ) ∧ w.cy.Mem (cy : ℝ)

/-- Model of `sketch.coneSideForm.newConeArcFloat`. The non-finite and negative-radius refusals only disable
the prefilter (`spanMul` repeats the sign test), so the model omits them. The center floats are the exact
values `cx`, `cy` (A31). -/
@[expose] noncomputable def newConeArcFloat (fp : Option FloatForm) (rLo rHi cx cy : ℚ) : Option ArcFloat :=
  fp.map fun g => ⟨g, ⟨ratEnclose M rLo, ratEnclose M rHi⟩, pt cx, pt cy⟩

/-- The state `newConeArcFloat` builds holds the exact form, radius bracket and center. Covers
`sketch.coneSideForm.newConeArcFloat`, `sketch.ratEnclose`; A24, A31. -/
theorem newConeArcFloat_enc {fp : Option FloatForm} {f : ConeSideForm} (hg : ∀ g, fp = some g → g.Enc f)
    (rLo rHi cx cy : ℚ) : ∀ w, newConeArcFloat M fp rLo rHi cx cy = some w → w.Enc f ⟨rLo, rHi⟩ cx cy := by
  intro w hw
  cases fp with
  | none => simp [newConeArcFloat] at hw
  | some g =>
    simp only [newConeArcFloat, Option.map_some, Option.some.injEq] at hw
    subst hw
    exact ⟨hg g rfl, ⟨ratEnclose_sound M _, ratEnclose_sound M _⟩, pt_sound _, pt_sound _⟩

/-- Model of `sketch.unitEnclose` for the pair `coneUnit` returns. -/
@[expose] noncomputable def unitEnclose (u : ConeInterval × ConeInterval) : Fiv × Fiv :=
  (⟨ratEnclose M u.1.lo, ratEnclose M u.1.hi⟩, ⟨ratEnclose M u.2.lo, ratEnclose M u.2.hi⟩)

/-- The enclosures `unitEnclose` builds hold the endpoints of a unit enclosure. Covers `sketch.unitEnclose`,
`sketch.ratEnclose`; A24. -/
theorem unitEnclose_enc (u : ConeInterval × ConeInterval) :
    (unitEnclose M u).1.Enc u.1 ∧ (unitEnclose M u).2.Enc u.2 :=
  ⟨⟨ratEnclose_sound M _, ratEnclose_sound M _⟩, ⟨ratEnclose_sound M _, ratEnclose_sound M _⟩⟩

/-- Model of `sketch.coneArcWedge.boxZero`. `none` results of `spanMul` are the Go `return false`. -/
@[expose] noncomputable def boxZeroW (w : ArcFloat) (a b : ℚ × ℚ) (ua ub : ConeInterval × ConeInterval) : Prop :=
  match spanMul M w.radius (fspan a b (unitEnclose M ua).1 (unitEnclose M ub).1 (1, 0) (-1, 0)),
    spanMul M w.radius (fspan a b (unitEnclose M ua).2 (unitEnclose M ub).2 (0, 1) (0, -1)) with
  | some s0, some s1 => zeroBox M w.form (fivAdd M s0 ⟨w.cx, w.cx⟩) (fivAdd M s1 ⟨w.cy, w.cy⟩)
  | _, _ => False

/-- When the float prefilter reports a skip, the exact wedge box finds no side. The exact box scales the
unit-direction ranges by the radius bracket, shifts by the center and widens with `coneOutward`; each step's
endpoints are enclosed by the floats of `boxZeroW`. Covers `sketch.coneArcWedge.boxZero`,
`sketch.axisInWedge`, `sketch.unitEnclose`, `sketch.coneArcWedge.box`, `sketch.coneOutward`,
`sketch.coneMulInterval` and the functions `zeroBox_sound` lists; A24, A30, A50, A200, A1-A4, A6, A2. -/
theorem boxZeroW_sound {E : ConeArcEnv} (hE : EnvAgrees M E) {f : ConeSideForm} {w : ArcFloat}
    {cx cy : ℚ} {radius : ConeInterval} (hw : w.Enc f radius cx cy) (a b : ℚ × ℚ)
    (ua ub : ConeInterval × ConeInterval) (hz : boxZeroW M w a b ua ub) :
    f.wedgeBox E cx cy radius a b ua ub = (.zero, none) := by
  obtain ⟨hg, hrad, hcx, hcy⟩ := hw
  obtain ⟨p1, p2, p3, p4⟩ := axisInWedge_eq a b
  obtain ⟨a1, a2⟩ := unitEnclose_enc M ua
  obtain ⟨b1, b2⟩ := unitEnclose_enc M ub
  unfold boxZeroW at hz
  split at hz
  · rename_i s0 s1 h0 h1
    have e0 := enc_spanMul M hrad (enc_fspan p1 p2 a1 b1) h0
    have e1 := enc_spanMul M hrad (enc_fspan p3 p4 a2 b2) h1
    have x0 := enc_add M e0 (⟨hcx, hcx⟩ : (⟨w.cx, w.cx⟩ : Fiv).Enc ⟨cx, cx⟩)
    have x1 := enc_add M e1 (⟨hcy, hcy⟩ : (⟨w.cy, w.cy⟩ : Fiv).Enc ⟨cy, cy⟩)
    have o0 := enc_outward M hE x0 (iadd_lo_mem M _ _) (iadd_hi_mem M _ _)
    have o1 := enc_outward M hE x1 (iadd_lo_mem M _ _) (iadd_hi_mem M _ _)
    exact zeroBox_sound M hg o0 o1 hz
  · exact hz.elim

/-- Model of the skip test in `sketch.coneArcWedge.side`: false without prefilter state, else `boxZeroW`. -/
@[expose] noncomputable def boxSkip (afl : Option ArcFloat) (a b : ℚ × ℚ) (ua ub : ConeInterval × ConeInterval) :
    Prop :=
  match afl with
  | none => False
  | some w => boxZeroW M w a b ua ub

open Classical in
/-- Model of `sketch.coneArcWedge.side` with the float prefilter: the exact box test runs only when
`boxSkip` is false, and a skip counts as no side. Otherwise the same as `wedgeSide`. -/
@[expose] noncomputable def wedgeSideP (E : ConeArcEnv) (afl : Option ArcFloat) (f : ConeSideForm)
    (cx cy : ℚ) (radius : ConeInterval) :
    ℚ × ℚ → ℚ × ℚ → ConeInterval × ConeInterval → ConeInterval × ConeInterval → ℕ → ℕ →
      ConeSide × Option ℚ × ℕ
  | _, _, _, _, _, 0 => (.zero, none, 0)
  | a, b, ua, ub, depth, budget + 1 =>
    let r : ConeSide × Option ℚ :=
      if boxSkip M afl a b ua ub then (.zero, none) else f.wedgeBox E cx cy radius a b ua ub
    if r.1 ≠ .zero then (r.1, r.2, budget)
    else
      match depth with
      | 0 => (.zero, none, budget)
      | d + 1 =>
        match coneSplit E a b with
        | none => (.zero, none, budget)
        | some m =>
          match coneUnit m with
          | none => (.zero, none, budget)
          | some um =>
            let left := wedgeSideP E afl f cx cy radius a m ua um d budget
            if left.1 = .zero then (.zero, none, left.2.2)
            else
              let right := wedgeSideP E afl f cx cy radius m b um ub d left.2.2
              if right.1 ≠ left.1 then (.zero, none, right.2.2)
              else
                match left.2.1, right.2.1 with
                | some m0, some m1 => (left.1, some (if m0 ≤ m1 then m0 else m1), right.2.2)
                | _, _ => (.zero, none, right.2.2)

open Classical in
/-- The Go `coneArcWedge.side`, which skips the exact box when the float prefilter fires, computes exactly
the result of the exact-only model `wedgeSide`: the same side, margin and remaining budget. Covers
`sketch.coneArcWedge.side`, `sketch.coneArcWedge.boxZero`; the assumptions of `boxZeroW_sound`. -/
theorem wedgeSideP_eq {E : ConeArcEnv} (hE : EnvAgrees M E) {afl : Option ArcFloat} {f : ConeSideForm}
    {cx cy : ℚ} {radius : ConeInterval} (hw : ∀ w, afl = some w → w.Enc f radius cx cy) :
    ∀ (depth budget : ℕ) (a b : ℚ × ℚ) (ua ub : ConeInterval × ConeInterval),
      wedgeSideP M E afl f cx cy radius a b ua ub depth budget = f.wedgeSide E cx cy radius a b ua ub depth budget := by
  have hr : ∀ (a b : ℚ × ℚ) (ua ub : ConeInterval × ConeInterval),
      (if boxSkip M afl a b ua ub then ((.zero, none) : ConeSide × Option ℚ)
        else f.wedgeBox E cx cy radius a b ua ub) = f.wedgeBox E cx cy radius a b ua ub := by
    intro a b ua ub
    split_ifs with hs
    · cases afl with
      | none => exact hs.elim
      | some w => exact (boxZeroW_sound M hE (hw w rfl) a b ua ub hs).symm
    · rfl
  intro depth
  induction depth with
  | zero =>
    intro budget a b ua ub
    cases budget with
    | zero => simp [wedgeSideP, ConeSideForm.wedgeSide]
    | succ n => simp only [wedgeSideP, ConeSideForm.wedgeSide, hr]
  | succ d ih =>
    intro budget a b ua ub
    cases budget with
    | zero => simp [wedgeSideP, ConeSideForm.wedgeSide]
    | succ n =>
      simp only [wedgeSideP, ConeSideForm.wedgeSide, hr, ih]
      rfl

end Sketch.Cone

end
