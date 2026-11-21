module

public import Sketch.Cone.Sqrt

/-!
# Arc side proofs

Models the arc path of `sketch.Sketch.CertifyConeCurveSide`: `sketch.coneSideForm.arcSide` (`cone_side.go`)
tries `sketch.coneSideForm.arcQuadrantSide` first, then the general wedge proof
`sketch.coneSideForm.arcWedgeSide` (`cone_side_arc.go`), which halves the angular wedge with
`sketch.coneArcWedge.side` until each piece's box (`sketch.coneArcWedge.box`) has one sign under
`sketch.coneSideForm.boxSide`.

An arc is the circle about the center through the start point, traced counter-clockwise from the start ray
to the end ray. A point of it is `circlePoint cx cy ρ w` for a nonzero direction `w` in the closed wedge from
the start ray to the end ray (`InWedge`), with `ρ = |Start − Center|` (`ArcSideAt`).

* `arcWedgeSide_sound`: a side `arcWedgeSide` returns holds, with its positive margin, at every point of the
  exact arc: radius `|Start − Center|`, directions from `Start − Center` to `End − Center`.
* `arcQuadrantSide_sound`: a side `arcQuadrantSide` returns holds at every point of the circle of radius `r`
  (the float `Arc.R()`) through the rays of the float differences `Start − Center` and `End − Center` it
  computes. That circle is not the exact arc: `Arc.R()` and the differences are rounded, and the box has no
  slack at the left end `cx − r`, so an exact arc point can sit just outside the box.
* `arcSide_sound`: `arcSide`'s result holds on one of the two curves above, and on the exact arc whenever
  `arcQuadrantSide` refused.

The float steps of `coneArcWedge.box` (`coneOutward`) and `coneSplit` are modeled through `ConeArcEnv`.
-/

public section

namespace Sketch.Cone

/-- Model of `sketch.coneRay.cross`: the cross product of two exact rays. -/
@[expose] def coneCross (r o : ℚ × ℚ) : ℚ := r.1 * o.2 - r.2 * o.1

theorem coneCross_cast (r o : ℚ × ℚ) : (coneCross r o : ℝ) = crossR (rayR r) (rayR o) := by
  simp only [coneCross, crossR, rayR]
  push_cast
  ring

/-- The float operations of the general arc path. `F` holds the finite float64 values as exact rationals.
`rn` is `big.Rat.Float64` (round to nearest), `prev` and `next` are `math.Nextafter` toward `−Inf` and
`+Inf`, and `divider` is the float computation of `sketch.coneSplit` before its exact check (A50). The
non-finite refusals of `coneOutward` and `coneSplit` only refuse, so the model omits them. -/
structure ConeArcEnv where
  F : Set ℚ
  rn : ℚ → ℚ
  prev : ℚ → ℚ
  next : ℚ → ℚ
  divider : ℚ × ℚ → ℚ × ℚ → Option (ℚ × ℚ)
  rn_mem : ∀ x, rn x ∈ F
  rn_near : ∀ x, ∀ f ∈ F, |rn x - x| ≤ |f - x|
  prev_mem : ∀ f ∈ F, prev f ∈ F
  prev_lt : ∀ f ∈ F, prev f < f
  next_mem : ∀ f ∈ F, next f ∈ F
  lt_next : ∀ f ∈ F, f < next f

namespace ConeArcEnv

variable (E : ConeArcEnv)

/-- Rounding to nearest, then one float down, never lands above `x`. -/
theorem prev_rn_le (x : ℚ) : E.prev (E.rn x) ≤ x := by
  by_contra hc
  push Not at hc
  have hf := E.rn_mem x
  have hlt := E.prev_lt _ hf
  have hnear := E.rn_near x _ (E.prev_mem _ hf)
  rw [abs_of_pos (by linarith : 0 < E.rn x - x), abs_of_pos (by linarith : 0 < E.prev (E.rn x) - x)] at hnear
  linarith

/-- Rounding to nearest, then one float up, never lands below `x`. -/
theorem le_next_rn (x : ℚ) : x ≤ E.next (E.rn x) := by
  by_contra hc
  push Not at hc
  have hf := E.rn_mem x
  have hlt := E.lt_next _ hf
  have hnear := E.rn_near x _ (E.next_mem _ hf)
  rw [abs_of_neg (by linarith : E.rn x - x < 0), abs_of_neg (by linarith : E.next (E.rn x) - x < 0)] at hnear
  linarith

end ConeArcEnv

/-- Model of `sketch.coneOutward`: each bound rounds to the nearest float, then steps one float outward when
the rounded value lies inside `[lo, hi]` (A50). -/
@[expose] def coneOutward (E : ConeArcEnv) (i : ConeInterval) : ConeInterval :=
  ⟨if i.lo < E.rn i.lo then E.prev (E.rn i.lo) else E.rn i.lo,
   if E.rn i.hi < i.hi then E.next (E.rn i.hi) else E.rn i.hi⟩

/-- `coneOutward` keeps every real of its input. Covers `sketch.coneOutward`; A31, A50. -/
theorem coneOutward_memR {E : ConeArcEnv} {i : ConeInterval} {x : ℝ} (hx : i.MemR x) :
    (coneOutward E i).MemR x := by
  obtain ⟨h1, h2⟩ := hx
  have hlo : (coneOutward E i).lo ≤ i.lo := by
    simp only [coneOutward]
    split_ifs with h
    · exact E.prev_rn_le _
    · exact le_of_not_gt h
  have hhi : i.hi ≤ (coneOutward E i).hi := by
    simp only [coneOutward]
    split_ifs with h
    · exact E.le_next_rn _
    · exact le_of_not_gt h
  have hlo' : ((coneOutward E i).lo : ℝ) ≤ i.lo := by exact_mod_cast hlo
  have hhi' : (i.hi : ℝ) ≤ (coneOutward E i).hi := by exact_mod_cast hhi
  exact ⟨le_trans hlo' h1, le_trans h2 hhi'⟩

/-- Model of `sketch.coneSplit`: the float divider, kept only when exact cross products put it strictly
inside the wedge. -/
@[expose] def coneSplit (E : ConeArcEnv) (a b : ℚ × ℚ) : Option (ℚ × ℚ) :=
  match E.divider a b with
  | none => none
  | some m => if coneCross a m ≤ 0 ∨ coneCross m b ≤ 0 then none else some m

/-- A divider `coneSplit` returns lies strictly inside the wedge. Covers `sketch.coneSplit`; A30. -/
theorem coneSplit_sound {E : ConeArcEnv} {a b m : ℚ × ℚ} (h : coneSplit E a b = some m) :
    0 < coneCross a m ∧ 0 < coneCross m b := by
  unfold coneSplit at h
  split at h
  · simp at h
  · rename_i m' _
    split_ifs at h with hc
    simp only [Option.some.injEq] at h
    subst h
    push Not at hc
    exact hc

/-- Model of `sketch.coneWedgeHas`: `e` lies in the closed wedge from `a` to `b`. -/
@[expose] def coneWedgeHas (a b e : ℚ × ℚ) : Bool := decide (0 ≤ coneCross a e) && decide (0 ≤ coneCross e b)

theorem not_coneWedgeHas {a b e : ℚ × ℚ} (h : coneWedgeHas a b e = false) :
    ¬(0 ≤ crossR (rayR a) (rayR e) ∧ 0 ≤ crossR (rayR e) (rayR b)) := by
  rintro ⟨h1, h2⟩
  rw [← coneCross_cast] at h1 h2
  have h1' : 0 ≤ coneCross a e := by exact_mod_cast h1
  have h2' : 0 ≤ coneCross e b := by exact_mod_cast h2
  simp [coneWedgeHas, h1', h2'] at h

/-- One coordinate range of `sketch.coneArcWedge.box`: the hull of the endpoint enclosures `ua`, `ub`,
widened to `1` when the wedge holds the positive axis ray `pos` and to `−1` when it holds `neg`. -/
@[expose] def wedgeSpan (a b : ℚ × ℚ) (ua ub : ConeInterval) (pos neg : ℚ × ℚ) : ConeInterval :=
  ⟨if coneWedgeHas a b neg then -1 else if ub.lo < ua.lo then ub.lo else ua.lo,
   if coneWedgeHas a b pos then 1 else if ua.hi < ub.hi then ub.hi else ua.hi⟩

theorem wedgeSpan_memR {a b pos neg : ℚ × ℚ} {ua ub : ConeInterval} {wc ac bc : ℝ}
    (hua : ua.MemR ac) (hub : ub.MemR bc) (hlo1 : -1 ≤ wc) (hhi1 : wc ≤ 1)
    (hpos : coneWedgeHas a b pos = false → wc ≤ max ac bc)
    (hneg : coneWedgeHas a b neg = false → min ac bc ≤ wc) :
    (wedgeSpan a b ua ub pos neg).MemR wc := by
  obtain ⟨ha1, ha2⟩ := hua
  obtain ⟨hb1, hb2⟩ := hub
  unfold wedgeSpan ConeInterval.MemR
  rw [ConeInterval.ite_lt_eq_min, ConeInterval.ite_gt_eq_max]
  constructor
  · cases hn : coneWedgeHas a b neg
    · have := hneg hn
      simp only [Bool.false_eq_true, ↓reduceIte, Rat.cast_min]
      rw [min_le_iff] at this ⊢
      rcases this with h | h
      · right; linarith
      · left; linarith
    · simp only [↓reduceIte, Rat.cast_neg, Rat.cast_one]
      exact hlo1
  · cases hp : coneWedgeHas a b pos
    · have := hpos hp
      simp only [Bool.false_eq_true, ↓reduceIte, Rat.cast_max]
      rw [le_max_iff] at this ⊢
      rcases this with h | h
      · left; linarith
      · right; linarith
    · simp only [↓reduceIte, Rat.cast_one]
      exact hhi1

/-- Point of the circle of radius `ρ` about `(cx, cy)` in direction `w`: `(cx, cy) + ρ·w/|w|`. -/
@[expose] noncomputable def circlePoint (cx cy : ℚ) (ρ : ℝ) (w : ℝ × ℝ) : ℝ × ℝ :=
  (cx + ρ * (unitR w).1, cy + ρ * (unitR w).2)

namespace ConeSideForm

/-- Side `side` holds with margin `m` at every point of the circle of radius `ρ` about `(cx, cy)` whose
direction lies in the closed wedge from `a` to `b`. -/
@[expose] def ArcSideAt (f : ConeSideForm) (side : ConeSide) (m cx cy : ℚ) (ρ : ℝ) (a b : ℝ × ℝ) : Prop :=
  ∀ w : ℝ × ℝ, w ≠ 0 → InWedge a b w → f.SideAtR side m (circlePoint cx cy ρ w).1 (circlePoint cx cy ρ w).2

/-- Model of `sketch.coneArcWedge.box`: the coordinate ranges of the unit directions (`wedgeSpan`), scaled
by the radius interval with `coneMulInterval`, shifted by the center, widened by `coneOutward`, then tested
by `boxSide`. -/
@[expose] def wedgeBox (E : ConeArcEnv) (f : ConeSideForm) (cx cy : ℚ) (radius : ConeInterval)
    (a b : ℚ × ℚ) (ua ub : ConeInterval × ConeInterval) : ConeSide × Option ℚ :=
  let u := coneMulInterval radius (wedgeSpan a b ua.1 ub.1 (1, 0) (-1, 0))
  let v := coneMulInterval radius (wedgeSpan a b ua.2 ub.2 (0, 1) (0, -1))
  f.boxSide (coneOutward E ⟨u.lo + cx, u.hi + cx⟩) (coneOutward E ⟨v.lo + cy, v.hi + cy⟩)

theorem memR_shift {i : ConeInterval} {x : ℝ} (c : ℚ) (hx : i.MemR x) :
    (⟨i.lo + c, i.hi + c⟩ : ConeInterval).MemR (c + x) := by
  obtain ⟨h1, h2⟩ := hx
  unfold ConeInterval.MemR
  push_cast
  constructor <;> linarith

/-- A side `wedgeBox` returns holds at every point of the circle piece between rays `a` and `b`, for every
radius in `radius`. Covers `sketch.coneArcWedge.box`, `sketch.coneWedgeHas`, `sketch.coneMulInterval`,
`sketch.coneOutward` and `sketch.coneSideForm.boxSide`; A30, A31, A50. -/
theorem wedgeBox_sound {E : ConeArcEnv} {f : ConeSideForm} {cx cy : ℚ} {radius : ConeInterval}
    {a b : ℚ × ℚ} {ua ub : ConeInterval × ConeInterval} {side : ConeSide} {m : ℚ} {ρ : ℝ}
    (hab : 0 < coneCross a b) (hρ : radius.MemR ρ) (hua : coneUnit a = some ua) (hub : coneUnit b = some ub)
    (h : f.wedgeBox E cx cy radius a b ua ub = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧ f.ArcSideAt side m cx cy ρ (rayR a) (rayR b) := by
  obtain ⟨hm, hs, hall⟩ := boxSide_sound_real h
  refine ⟨hm, hs, fun w hw hin => hall _ _ ?_ ?_⟩
  · have hab' : 0 < crossR (rayR a) (rayR b) := by rw [← coneCross_cast]; exact_mod_cast hab
    obtain ⟨ha0, hb0⟩ := ne_zero_of_crossR_pos hab'
    obtain ⟨hua1, -⟩ := coneUnit_sound (ix := ua.1) (iy := ua.2) hua
    obtain ⟨hub1, -⟩ := coneUnit_sound (ix := ub.1) (iy := ub.2) hub
    obtain ⟨hb1, hb2, -, -⟩ := unitR_bounds hw
    refine coneOutward_memR (memR_shift cx (coneMulInterval_memR hρ (wedgeSpan_memR hua1 hub1 hb1 hb2
      (fun hp => x_le_max_of_not_has ha0 hb0 hw hab' hin ?_)
      (fun hn => x_ge_min_of_not_has ha0 hb0 hw hab' hin ?_))))
    · simpa [rayR] using not_coneWedgeHas hp
    · simpa [rayR] using not_coneWedgeHas hn
  · have hab' : 0 < crossR (rayR a) (rayR b) := by rw [← coneCross_cast]; exact_mod_cast hab
    obtain ⟨ha0, hb0⟩ := ne_zero_of_crossR_pos hab'
    obtain ⟨-, hua2⟩ := coneUnit_sound (ix := ua.1) (iy := ua.2) hua
    obtain ⟨-, hub2⟩ := coneUnit_sound (ix := ub.1) (iy := ub.2) hub
    obtain ⟨-, -, hb1, hb2⟩ := unitR_bounds hw
    refine coneOutward_memR (memR_shift cy (coneMulInterval_memR hρ (wedgeSpan_memR hua2 hub2 hb1 hb2
      (fun hp => y_le_max_of_not_has ha0 hb0 hw hab' hin ?_)
      (fun hn => y_ge_min_of_not_has ha0 hb0 hw hab' hin ?_))))
    · simpa [rayR] using not_coneWedgeHas hp
    · simpa [rayR] using not_coneWedgeHas hn

/-- Model of `sketch.coneArcWedge.side`. The shared `budget` field is threaded as a natural number and
returned as the third component; it is decremented only when nonzero. Both halves must prove the same
side; the smaller margin wins. The last `match` arm is unreachable: a nonzero side always carries a
margin. -/
@[expose] def wedgeSide (E : ConeArcEnv) (f : ConeSideForm) (cx cy : ℚ) (radius : ConeInterval) :
    ℚ × ℚ → ℚ × ℚ → ℕ → ℕ → ConeSide × Option ℚ × ℕ
  | _, _, _, 0 => (.zero, none, 0)
  | a, b, depth, budget + 1 =>
    match coneUnit a, coneUnit b with
    | some ua, some ub =>
      let r := f.wedgeBox E cx cy radius a b ua ub
      if r.1 ≠ .zero then (r.1, r.2, budget)
      else
        match depth with
        | 0 => (.zero, none, budget)
        | d + 1 =>
          match coneSplit E a b with
          | none => (.zero, none, budget)
          | some m =>
            let left := wedgeSide E f cx cy radius a m d budget
            if left.1 = .zero then (.zero, none, left.2.2)
            else
              let right := wedgeSide E f cx cy radius m b d left.2.2
              if right.1 ≠ left.1 then (.zero, none, right.2.2)
              else
                match left.2.1, right.2.1 with
                | some m0, some m1 => (left.1, some (if m0 ≤ m1 then m0 else m1), right.2.2)
                | _, _ => (.zero, none, right.2.2)
    | _, _ => (.zero, none, budget)

/-- A side `wedgeSide` returns holds, with the returned positive margin, at every point of the circle piece
between rays `a` and `b` (`a × b > 0`), for every radius in `radius`. Covers `sketch.coneArcWedge.side`,
`sketch.coneUnit`, `sketch.coneSplit` and the functions `wedgeBox_sound` lists; A30, A31, A50, A51. -/
theorem wedgeSide_sound (E : ConeArcEnv) (f : ConeSideForm) (cx cy : ℚ) (radius : ConeInterval) {ρ : ℝ}
    (hρ : radius.MemR ρ) :
    ∀ (depth budget : ℕ) (a b : ℚ × ℚ) {side : ConeSide} {m : ℚ} {budget' : ℕ}, 0 < coneCross a b →
      f.wedgeSide E cx cy radius a b depth budget = (side, some m, budget') →
      0 < m ∧ side ≠ .zero ∧ f.ArcSideAt side m cx cy ρ (rayR a) (rayR b) := by
  intro depth
  induction depth with
  | zero =>
    intro budget a b side m budget' hab hres
    cases budget with
    | zero => simp [wedgeSide] at hres
    | succ n =>
      simp only [wedgeSide] at hres
      split at hres
      · rename_i ua ub hua hub
        split_ifs at hres with hr
        · simp only [Prod.mk.injEq] at hres
          obtain ⟨h1, h2, -⟩ := hres
          exact wedgeBox_sound hab hρ hua hub (Prod.ext h1 h2)
        · simp at hres
      · simp at hres
  | succ d ih =>
    intro budget a b side m budget' hab hres
    cases budget with
    | zero => simp [wedgeSide] at hres
    | succ n =>
      simp only [wedgeSide] at hres
      split at hres
      · rename_i ua ub hua hub
        by_cases hr : (f.wedgeBox E cx cy radius a b ua ub).1 ≠ .zero
        · simp only [hr, ne_eq, not_false_eq_true, ↓reduceIte, Prod.mk.injEq] at hres
          obtain ⟨h1, h2, -⟩ := hres
          exact wedgeBox_sound hab hρ hua hub (Prod.ext h1 h2)
        · rw [ne_eq, not_not] at hr
          simp only [hr, ne_eq, not_true_eq_false, ↓reduceIte] at hres
          split at hres
          · simp at hres
          · rename_i mm hsplit
            obtain ⟨ham, hmb⟩ := coneSplit_sound hsplit
            generalize hl : f.wedgeSide E cx cy radius a mm d n = left at hres
            obtain ⟨s0, m0, bl⟩ := left
            by_cases h0 : s0 = .zero
            · simp [h0] at hres
            simp only [h0, ↓reduceIte] at hres
            generalize hrt : f.wedgeSide E cx cy radius mm b d bl = right at hres
            obtain ⟨s1, m1, br⟩ := right
            by_cases h10 : s1 = s0
            swap
            · simp [h10] at hres
            subst h10
            cases m0 with
            | none => simp at hres
            | some x =>
              cases m1 with
              | none => simp at hres
              | some y =>
                simp only [not_true_eq_false, ↓reduceIte, Prod.mk.injEq, Option.some.injEq] at hres
                obtain ⟨rfl, rfl, -⟩ := hres
                obtain ⟨hx, -, hlall⟩ := ih n a mm ham hl
                obtain ⟨hy, -, hrall⟩ := ih bl mm b hmb hrt
                have hmx : (if x ≤ y then x else y) ≤ x := by split_ifs <;> linarith
                have hmy : (if x ≤ y then x else y) ≤ y := by split_ifs <;> linarith
                refine ⟨by split_ifs <;> assumption, h0, fun w hw hin => ?_⟩
                rcases inWedge_split (m := rayR mm) hin with hl' | hr'
                · exact sideAtR_mono hmx (hlall w hw hl')
                · exact sideAtR_mono hmy (hrall w hw hr')
      · simp at hres

/-- Model of `sketch.coneSideForm.arcWedgeSide`. `cx`, `cy`, `sx`, `sy`, `ex`, `ey` are the exact values of
the Center, Start and End coordinates (A31). The radius interval brackets `|Start − Center|`; the proof
starts at depth `coneArcDepth = 12` with budget `coneArcBudget = 2048`. -/
@[expose] def arcWedgeSide (E : ConeArcEnv) (f : ConeSideForm) (cx cy sx sy ex ey : ℚ) :
    ConeSide × Option ℚ :=
  let start : ℚ × ℚ := (sx - cx, sy - cy)
  let stop : ℚ × ℚ := (ex - cx, ey - cy)
  if coneCross start stop ≤ 0 then (.zero, none)
  else
    match coneSqrtBounds (start.1 * start.1 + start.2 * start.2) with
    | none => (.zero, none)
    | some (rLo, rHi) =>
      let r := f.wedgeSide E cx cy ⟨rLo, rHi⟩ start stop 12 2048
      (r.1, r.2.1)

/-- A side `arcWedgeSide` returns holds, with the returned positive margin, at every point of the exact arc:
the circle about Center of radius `|Start − Center|`, from the Start ray counter-clockwise to the End ray,
whose sweep lies strictly between zero and a half turn. Covers `sketch.coneSideForm.arcWedgeSide`,
`sketch.coneSqrtBounds` and the functions `wedgeSide_sound` lists; A30, A31, A50, A51. -/
theorem arcWedgeSide_sound {E : ConeArcEnv} {f : ConeSideForm} {cx cy sx sy ex ey : ℚ} {side : ConeSide}
    {m : ℚ} (h : f.arcWedgeSide E cx cy sx sy ex ey = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧ 0 < coneCross (sx - cx, sy - cy) (ex - cx, ey - cy) ∧
      f.ArcSideAt side m cx cy (normR (rayR (sx - cx, sy - cy))) (rayR (sx - cx, sy - cy))
        (rayR (ex - cx, ey - cy)) := by
  unfold arcWedgeSide at h
  dsimp only at h
  split_ifs at h with hc
  · simp at h
  split at h
  · simp at h
  · rename_i rLo rHi hb
    obtain ⟨-, hρ⟩ := coneSqrtBounds_memR_normR (v := (sx - cx, sy - cy)) hb
    have hab : 0 < coneCross (sx - cx, sy - cy) (ex - cx, ey - cy) := lt_of_not_ge hc
    generalize hw : f.wedgeSide E cx cy ⟨rLo, rHi⟩ (sx - cx, sy - cy) (ex - cx, ey - cy) 12 2048 = r at h
    obtain ⟨s, mm, b'⟩ := r
    simp only [Prod.mk.injEq] at h
    obtain ⟨rfl, rfl⟩ := h
    obtain ⟨hm, hs, hall⟩ := wedgeSide_sound E f cx cy ⟨rLo, rHi⟩ hρ 12 2048 _ _ hab hw
    exact ⟨hm, hs, hab, hall⟩

/-- Model of `sketch.coneSideForm.arcQuadrantSide`. `cx`, `cy` are the Center coordinates; `sx`, `sy`, `ex`,
`ey` are the float differences `Start − Center` and `End − Center` the Go code computes, `r` is the float
`Arc.R()`, and `Y`, `U` are the floats it computes from them; the model takes all of them as given exact
values (A31, A52). The refusals on non-finite values, on `Arc.Sweep()` and on `delta` only refuse, so the
model omits them. -/
@[expose] def arcQuadrantSide (f : ConeSideForm) (cx cy sx sy ex ey r Y U : ℚ) : ConeSide × Option ℚ :=
  if ¬(0 < r ∧ sx < 0 ∧ ex < 0 ∧ 0 < sy ∧ ey < 0) then (.zero, none)
  else if ¬(U < 0 ∧ 0 < Y) then (.zero, none)
  else if r * r ≤ U * U + Y * Y then (.zero, none)
  else if Y * Y * (sx * sx + sy * sy) ≤ r * r * (sy * sy) then (.zero, none)
  else if Y * Y * (ex * ex + ey * ey) ≤ r * r * (ey * ey) then (.zero, none)
  else f.boxSide ⟨cx - r, cx + U⟩ ⟨cy - Y, cy + Y⟩

/-- The endpoint check of `arcQuadrantSide`: at the unit direction of `(px, py)`, the circle of radius `r`
has `|y| < Y` and, in the left half plane, `x < U`. -/
theorem quadrant_endpoint {px py r Y U : ℚ} (hpx : px < 0) (hr : 0 < r) (hU : U < 0) (hY : 0 < Y)
    (hcirc : U * U + Y * Y < r * r) (hck : r * r * (py * py) < Y * Y * (px * px + py * py)) :
    (r : ℝ) * (unitR (rayR (px, py))).1 < U ∧ -(Y : ℝ) < r * (unitR (rayR (px, py))).2 ∧
      (r : ℝ) * (unitR (rayR (px, py))).2 < Y := by
  have hne : rayR (px, py) ≠ 0 := by
    intro h
    have : (px : ℝ) = 0 := congrArg Prod.fst h
    have : px = 0 := by exact_mod_cast this
    linarith
  have hn := normR_pos hne
  have hn2 := normR_sq (rayR (px, py))
  have hsq := unitR_sq hne
  simp only [unitR, rayR] at hn hn2 hsq ⊢
  have hpxR : (px : ℝ) < 0 := by exact_mod_cast hpx
  have hrR : (0 : ℝ) < r := by exact_mod_cast hr
  have hUR : (U : ℝ) < 0 := by exact_mod_cast hU
  have hYR : (0 : ℝ) < Y := by exact_mod_cast hY
  have hcircR : (U : ℝ) * U + Y * Y < r * r := by exact_mod_cast hcirc
  have hckR : (r : ℝ) * r * (py * py) < Y * Y * (px * px + py * py) := by exact_mod_cast hck
  set N := normR ((px : ℝ), (py : ℝ))
  have hN0 : N ≠ 0 := hn.ne'
  have hy2 : ((r : ℝ) * (N⁻¹ * py)) ^ 2 < (Y : ℝ) ^ 2 := by
    have e : ((r : ℝ) * (N⁻¹ * py)) ^ 2 * (N * N) = r * r * (py * py) := by
      field_simp
    have hNN : 0 < N * N := mul_pos hn hn
    have : ((r : ℝ) * (N⁻¹ * py)) ^ 2 * (N * N) < (Y : ℝ) ^ 2 * (N * N) := by
      rw [e, hn2]
      nlinarith
    exact lt_of_mul_lt_mul_right this hNN.le
  have hx2 : ((U : ℝ)) ^ 2 < ((r : ℝ) * (N⁻¹ * px)) ^ 2 := by
    have : ((r : ℝ) * (N⁻¹ * px)) ^ 2 = r ^ 2 - ((r : ℝ) * (N⁻¹ * py)) ^ 2 := by
      have : ((r : ℝ) * (N⁻¹ * px)) ^ 2 + ((r : ℝ) * (N⁻¹ * py)) ^ 2 = r ^ 2 := by
        rw [show ((r : ℝ) * (N⁻¹ * px)) ^ 2 + ((r : ℝ) * (N⁻¹ * py)) ^ 2 =
          r ^ 2 * ((N⁻¹ * px) ^ 2 + (N⁻¹ * py) ^ 2) by ring, hsq, mul_one]
      linarith
    rw [this]
    nlinarith
  have hxneg : (r : ℝ) * (N⁻¹ * px) < 0 :=
    mul_neg_of_pos_of_neg hrR (mul_neg_of_pos_of_neg (inv_pos.2 hn) hpxR)
  refine ⟨?_, ?_, ?_⟩
  · nlinarith
  · nlinarith
  · nlinarith

/-- A side `arcQuadrantSide` returns holds, with the returned positive margin, at every point of the circle
of radius `r` about `(cx, cy)` from the ray `(sx, sy)` counter-clockwise to the ray `(ex, ey)`. With the Go
inputs, `r` is the float `Arc.R()` and the rays are the float differences, so this circle is not the exact
arc of `arcWedgeSide_sound`. Covers `sketch.coneSideForm.arcQuadrantSide` and
`sketch.coneSideForm.boxSide`; A30, A31, A52. -/
theorem arcQuadrantSide_sound {f : ConeSideForm} {cx cy sx sy ex ey r Y U : ℚ} {side : ConeSide} {m : ℚ}
    (h : f.arcQuadrantSide cx cy sx sy ex ey r Y U = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧ f.ArcSideAt side m cx cy r (rayR (sx, sy)) (rayR (ex, ey)) := by
  unfold arcQuadrantSide at h
  split_ifs at h with h1 h2 h3 h4 h5
  all_goals try cases h
  simp only [not_le] at h3 h4 h5
  obtain ⟨hr, hsx, hex, hsy, hey⟩ := h1
  obtain ⟨hU, hY⟩ := h2
  obtain ⟨hm, hs, hall⟩ := boxSide_sound_real h
  refine ⟨hm, hs, fun w hw hin => hall _ _ ?_ ?_⟩
  all_goals
    have hab : 0 < crossR (rayR (sx, sy)) (rayR (ex, ey)) := by
      rw [← coneCross_cast]
      have : 0 < coneCross (sx, sy) (ex, ey) := by
        simp only [coneCross]
        nlinarith
      exact_mod_cast this
    obtain ⟨ha0, hb0⟩ := ne_zero_of_crossR_pos hab
    obtain ⟨hsU, hsY1, hsY2⟩ := quadrant_endpoint hsx hr hU hY h3 h4
    obtain ⟨heU, heY1, heY2⟩ := quadrant_endpoint hex hr hU hY h3 h5
    obtain ⟨hb1, hb2, hb3, hb4⟩ := unitR_bounds hw
    have hrR : (0 : ℝ) < r := by exact_mod_cast hr
    have hsxR : (sx : ℝ) < 0 := by exact_mod_cast hsx
    have hsyR : (0 : ℝ) < sy := by exact_mod_cast hsy
    have hexR : (ex : ℝ) < 0 := by exact_mod_cast hex
  · have hxmax := x_le_max_of_not_has ha0 hb0 hw hab hin (by
      simp only [rayR, crossR]
      rintro ⟨h, -⟩
      linarith)
    unfold ConeInterval.MemR circlePoint
    push_cast
    constructor
    · nlinarith
    · rw [le_max_iff] at hxmax
      rcases hxmax with hx | hx <;> nlinarith
  · have hymax := y_le_max_of_not_has ha0 hb0 hw hab hin (by
      simp only [rayR, crossR]
      rintro ⟨h, -⟩
      linarith)
    have hymin := y_ge_min_of_not_has ha0 hb0 hw hab hin (by
      simp only [rayR, crossR]
      rintro ⟨-, h⟩
      linarith)
    unfold ConeInterval.MemR circlePoint
    push_cast
    constructor
    · rw [min_le_iff] at hymin
      rcases hymin with hy | hy <;> nlinarith
    · rw [le_max_iff] at hymax
      rcases hymax with hy | hy <;> nlinarith

end ConeSideForm

/-- The arc inputs of `sketch.coneSideForm.arcSide`: the exact Center, Start and End coordinates (A31), and
the floats `arcQuadrantSide` computes from them: the differences `dsx`, `dsy`, `dex`, `dey`, the radius
`r = Arc.R()`, and `Y`, `U` (A52). -/
structure ConeArcData where
  cx : ℚ
  cy : ℚ
  sx : ℚ
  sy : ℚ
  ex : ℚ
  ey : ℚ
  dsx : ℚ
  dsy : ℚ
  dex : ℚ
  dey : ℚ
  r : ℚ
  Y : ℚ
  U : ℚ

namespace ConeSideForm

/-- The `arcQuadrantSide` call of `arcSide`. -/
@[expose] def arcQuadrant (f : ConeSideForm) (d : ConeArcData) : ConeSide × Option ℚ :=
  f.arcQuadrantSide d.cx d.cy d.dsx d.dsy d.dex d.dey d.r d.Y d.U

/-- Model of `sketch.coneSideForm.arcSide`: the quadrant proof when it returns a side, else the wedge
proof. -/
@[expose] def arcSide (E : ConeArcEnv) (f : ConeSideForm) (d : ConeArcData) : ConeSide × Option ℚ :=
  if (f.arcQuadrant d).1 ≠ .zero then f.arcQuadrant d
  else f.arcWedgeSide E d.cx d.cy d.sx d.sy d.ex d.ey

/-- A side `arcSide` returns holds with its positive margin either on the circle `arcQuadrantSide` tests
(radius `Arc.R()`, rays of the float differences), when that proof returned the side, or else on the exact
arc (radius `|Start − Center|`, from the Start ray to the End ray, sweep in `(0, π)`). Covers
`sketch.coneSideForm.arcSide` and the functions `arcQuadrantSide_sound` and `arcWedgeSide_sound` list;
A30, A31, A50, A51, A52. -/
theorem arcSide_sound {E : ConeArcEnv} {f : ConeSideForm} {d : ConeArcData} {side : ConeSide} {m : ℚ}
    (h : f.arcSide E d = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧
      (((f.arcQuadrant d).1 ≠ .zero ∧
          f.ArcSideAt side m d.cx d.cy d.r (rayR (d.dsx, d.dsy)) (rayR (d.dex, d.dey))) ∨
        ((f.arcQuadrant d).1 = .zero ∧ 0 < coneCross (d.sx - d.cx, d.sy - d.cy) (d.ex - d.cx, d.ey - d.cy) ∧
          f.ArcSideAt side m d.cx d.cy (normR (rayR (d.sx - d.cx, d.sy - d.cy)))
            (rayR (d.sx - d.cx, d.sy - d.cy)) (rayR (d.ex - d.cx, d.ey - d.cy)))) := by
  unfold arcSide at h
  split_ifs at h with hq
  · obtain ⟨hm, hs, hall⟩ := arcQuadrantSide_sound (f := f) h
    exact ⟨hm, hs, Or.inl ⟨hq, hall⟩⟩
  · obtain ⟨hm, hs, hc, hall⟩ := arcWedgeSide_sound h
    push Not at hq
    exact ⟨hm, hs, Or.inr ⟨hq, hc, hall⟩⟩

end ConeSideForm

end Sketch.Cone

end
