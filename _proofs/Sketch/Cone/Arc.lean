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
* `arcQuadrantSide_sound`: a side `arcQuadrantSide` returns holds on the same exact arc. Its box is built from
  floats, but every check runs against the exact endpoint rays and an exact bracket `[rLo, rHi]` of the
  radius, and the box reaches `cx − rHi` on the left.
* `arcSide_sound`: `arcSide`'s result holds on the exact arc, whichever proof produced it.

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

/-- Model of `sketch.coneArcWedge.side`. `ua` and `ub` are the unit enclosures `coneUnit a` and `coneUnit b`
the caller computed; a divider's `coneUnit` is computed once and passed to both halves. The shared `budget`
field is threaded as a natural number and returned as the third component; it is decremented only when
nonzero. Both halves must prove the same side; the smaller margin wins. The last `match` arm is
unreachable: a nonzero side always carries a margin. -/
@[expose] def wedgeSide (E : ConeArcEnv) (f : ConeSideForm) (cx cy : ℚ) (radius : ConeInterval) :
    ℚ × ℚ → ℚ × ℚ → ConeInterval × ConeInterval → ConeInterval × ConeInterval → ℕ → ℕ →
      ConeSide × Option ℚ × ℕ
  | _, _, _, _, _, 0 => (.zero, none, 0)
  | a, b, ua, ub, depth, budget + 1 =>
    let r := f.wedgeBox E cx cy radius a b ua ub
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
            let left := wedgeSide E f cx cy radius a m ua um d budget
            if left.1 = .zero then (.zero, none, left.2.2)
            else
              let right := wedgeSide E f cx cy radius m b um ub d left.2.2
              if right.1 ≠ left.1 then (.zero, none, right.2.2)
              else
                match left.2.1, right.2.1 with
                | some m0, some m1 => (left.1, some (if m0 ≤ m1 then m0 else m1), right.2.2)
                | _, _ => (.zero, none, right.2.2)

/-- A side `wedgeSide` returns holds, with the returned positive margin, at every point of the circle piece
between rays `a` and `b` (`a × b > 0`), for every radius in `radius`. Covers `sketch.coneArcWedge.side`,
`sketch.coneUnit`, `sketch.coneSplit` and the functions `wedgeBox_sound` lists; A30, A31, A50, A51. -/
theorem wedgeSide_sound (E : ConeArcEnv) (f : ConeSideForm) (cx cy : ℚ) (radius : ConeInterval) {ρ : ℝ}
    (hρ : radius.MemR ρ) :
    ∀ (depth budget : ℕ) (a b : ℚ × ℚ) (ua ub : ConeInterval × ConeInterval) {side : ConeSide} {m : ℚ}
      {budget' : ℕ}, 0 < coneCross a b → coneUnit a = some ua → coneUnit b = some ub →
      f.wedgeSide E cx cy radius a b ua ub depth budget = (side, some m, budget') →
      0 < m ∧ side ≠ .zero ∧ f.ArcSideAt side m cx cy ρ (rayR a) (rayR b) := by
  intro depth
  induction depth with
  | zero =>
    intro budget a b ua ub side m budget' hab hua hub hres
    cases budget with
    | zero => simp [wedgeSide] at hres
    | succ n =>
      simp only [wedgeSide] at hres
      split_ifs at hres with hr
      · simp only [Prod.mk.injEq] at hres
        obtain ⟨h1, h2, -⟩ := hres
        exact wedgeBox_sound hab hρ hua hub (Prod.ext h1 h2)
      · simp at hres
  | succ d ih =>
    intro budget a b ua ub side m budget' hab hua hub hres
    cases budget with
    | zero => simp [wedgeSide] at hres
    | succ n =>
      simp only [wedgeSide] at hres
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
          split at hres
          · simp at hres
          · rename_i um hum
            generalize hl : f.wedgeSide E cx cy radius a mm ua um d n = left at hres
            obtain ⟨s0, m0, bl⟩ := left
            by_cases h0 : s0 = .zero
            · simp [h0] at hres
            simp only [h0, ↓reduceIte] at hres
            generalize hrt : f.wedgeSide E cx cy radius mm b um ub d bl = right at hres
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
                obtain ⟨hx, -, hlall⟩ := ih n a mm ua um ham hua hum hl
                obtain ⟨hy, -, hrall⟩ := ih bl mm b um ub hmb hum hub hrt
                have hmx : (if x ≤ y then x else y) ≤ x := by split_ifs <;> linarith
                have hmy : (if x ≤ y then x else y) ≤ y := by split_ifs <;> linarith
                refine ⟨by split_ifs <;> assumption, h0, fun w hw hin => ?_⟩
                rcases inWedge_split (m := rayR mm) hin with hl' | hr'
                · exact sideAtR_mono hmx (hlall w hw hl')
                · exact sideAtR_mono hmy (hrall w hw hr')

/-- Model of `sketch.coneSideForm.arcWedgeSide`. `cx`, `cy`, `sx`, `sy`, `ex`, `ey` are the exact values of
the Center, Start and End coordinates (A31). The radius interval brackets `|Start − Center|`; the proof
starts at depth `coneArcDepth = 12` with budget `coneArcBudget = 2048`, with the unit enclosures of the Start
and End rays. -/
@[expose] def arcWedgeSide (E : ConeArcEnv) (f : ConeSideForm) (cx cy sx sy ex ey : ℚ) :
    ConeSide × Option ℚ :=
  let start : ℚ × ℚ := (sx - cx, sy - cy)
  let stop : ℚ × ℚ := (ex - cx, ey - cy)
  if coneCross start stop ≤ 0 then (.zero, none)
  else
    match coneSqrtBounds (start.1 * start.1 + start.2 * start.2) with
    | none => (.zero, none)
    | some (rLo, rHi) =>
      match coneUnit start, coneUnit stop with
      | some us, some ue =>
        let r := f.wedgeSide E cx cy ⟨rLo, rHi⟩ start stop us ue 12 2048
        (r.1, r.2.1)
      | _, _ => (.zero, none)

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
    split at h
    · rename_i us ue hus hue
      generalize hw : f.wedgeSide E cx cy ⟨rLo, rHi⟩ (sx - cx, sy - cy) (ex - cx, ey - cy) us ue 12 2048 = r
        at h
      obtain ⟨s, mm, b'⟩ := r
      simp only [Prod.mk.injEq] at h
      obtain ⟨rfl, rfl⟩ := h
      obtain ⟨hm, hs, hall⟩ := wedgeSide_sound E f cx cy ⟨rLo, rHi⟩ hρ 12 2048 _ _ _ _ hab hus hue hw
      exact ⟨hm, hs, hab, hall⟩
    · simp at h

/-- The exact part of `sketch.coneSideForm.arcQuadrantSide`, on the exact endpoint rays `p` (Start − Center)
and `q` (End − Center) and the floats `Y`, `U`: the sign checks on the rays, the radius bracket
`[rLo, rHi]` of `|p|` from `coneSqrtBounds`, the circle and endpoint checks, then `boxSide` on
`[cx − rHi, cx + U] × [cy − Y, cy + Y]`. -/
@[expose] def quadrantBox (f : ConeSideForm) (cx cy : ℚ) (p q : ℚ × ℚ) (Y U : ℚ) : ConeSide × Option ℚ :=
  if ¬(p.1 < 0 ∧ 0 < p.2 ∧ q.1 < 0 ∧ q.2 < 0) then (.zero, none)
  else
    match coneSqrtBounds (p.1 * p.1 + p.2 * p.2) with
    | none => (.zero, none)
    | some (rLo, rHi) =>
      if rLo * rLo ≤ U * U + Y * Y then (.zero, none)
      else if Y * Y * (p.1 * p.1 + p.2 * p.2) ≤ rHi * rHi * (p.2 * p.2) then (.zero, none)
      else if Y * Y * (q.1 * q.1 + q.2 * q.2) ≤ rHi * rHi * (q.2 * q.2) then (.zero, none)
      else f.boxSide ⟨cx - rHi, cx + U⟩ ⟨cy - Y, cy + Y⟩

/-- Model of `sketch.coneSideForm.arcQuadrantSide`. `cx`, `cy`, `sx`, `sy`, `ex`, `ey` are the exact Center,
Start and End coordinates (A31). `Y` and `U` are the floats the Go code computes from `Arc.R()` and the float
differences; the model takes them as given exact values (A52). The float guard (finite values, `r > 0`, the
float difference signs, `Arc.Sweep()`) and the `delta` check only refuse, so the model omits them. -/
@[expose] def arcQuadrantSide (f : ConeSideForm) (cx cy sx sy ex ey Y U : ℚ) : ConeSide × Option ℚ :=
  if ¬(U < 0 ∧ 0 < Y) then (.zero, none)
  else f.quadrantBox cx cy (sx - cx, sy - cy) (ex - cx, ey - cy) Y U

/-- The endpoint check of `arcQuadrantSide`: for every radius `ρ` in `[rLo, rHi]`, the circle of radius `ρ`
at the unit direction of `(px, py)` has `|y| < Y` and, in the left half plane, `x < U`. -/
theorem quadrant_endpoint {px py rLo rHi Y U : ℚ} {ρ : ℝ} (hpx : px < 0) (hlo : 0 < rLo)
    (hρlo : (rLo : ℝ) ≤ ρ) (hρhi : ρ ≤ rHi) (hU : U < 0) (hY : 0 < Y)
    (hcirc : U * U + Y * Y < rLo * rLo) (hck : rHi * rHi * (py * py) < Y * Y * (px * px + py * py)) :
    ρ * (unitR (rayR (px, py))).1 < U ∧ -(Y : ℝ) < ρ * (unitR (rayR (px, py))).2 ∧
      ρ * (unitR (rayR (px, py))).2 < Y := by
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
  have hloR : (0 : ℝ) < rLo := by exact_mod_cast hlo
  have hρ : 0 < ρ := lt_of_lt_of_le hloR hρlo
  have hUR : (U : ℝ) < 0 := by exact_mod_cast hU
  have hYR : (0 : ℝ) < Y := by exact_mod_cast hY
  have hcircR : (U : ℝ) * U + Y * Y < rLo * rLo := by exact_mod_cast hcirc
  have hckR : (rHi : ℝ) * rHi * (py * py) < Y * Y * (px * px + py * py) := by exact_mod_cast hck
  set N := normR ((px : ℝ), (py : ℝ))
  have hN0 : N ≠ 0 := hn.ne'
  have hy2 : (ρ * (N⁻¹ * py)) ^ 2 < (Y : ℝ) ^ 2 := by
    have e : (ρ * (N⁻¹ * py)) ^ 2 * (N * N) = ρ * ρ * (py * py) := by
      field_simp
    have hNN : 0 < N * N := mul_pos hn hn
    have hρ2 : ρ * ρ * (py * py) ≤ (rHi : ℝ) * rHi * (py * py) :=
      mul_le_mul_of_nonneg_right (mul_self_le_mul_self hρ.le hρhi) (mul_self_nonneg _)
    have : (ρ * (N⁻¹ * py)) ^ 2 * (N * N) < (Y : ℝ) ^ 2 * (N * N) := by
      rw [e, hn2]
      nlinarith
    exact lt_of_mul_lt_mul_right this hNN.le
  have hx2 : ((U : ℝ)) ^ 2 < (ρ * (N⁻¹ * px)) ^ 2 := by
    have : (ρ * (N⁻¹ * px)) ^ 2 = ρ ^ 2 - (ρ * (N⁻¹ * py)) ^ 2 := by
      have : (ρ * (N⁻¹ * px)) ^ 2 + (ρ * (N⁻¹ * py)) ^ 2 = ρ ^ 2 := by
        rw [show (ρ * (N⁻¹ * px)) ^ 2 + (ρ * (N⁻¹ * py)) ^ 2 =
          ρ ^ 2 * ((N⁻¹ * px) ^ 2 + (N⁻¹ * py) ^ 2) by ring, hsq, mul_one]
      linarith
    have hρ2 : (rLo : ℝ) * rLo ≤ ρ ^ 2 := by nlinarith
    rw [this]
    nlinarith
  have hxneg : ρ * (N⁻¹ * px) < 0 :=
    mul_neg_of_pos_of_neg hρ (mul_neg_of_pos_of_neg (inv_pos.2 hn) hpxR)
  refine ⟨?_, ?_, ?_⟩
  · nlinarith
  · nlinarith
  · nlinarith

/-- A side `quadrantBox` returns holds, with the returned positive margin, at every point of the circle of
radius `|p|` about `(cx, cy)` from the ray `p` counter-clockwise to the ray `q`. -/
theorem quadrantBox_sound {f : ConeSideForm} {cx cy Y U : ℚ} {p q : ℚ × ℚ} {side : ConeSide} {m : ℚ}
    (hU : U < 0) (hY : 0 < Y) (h : f.quadrantBox cx cy p q Y U = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧ 0 < coneCross p q ∧
      f.ArcSideAt side m cx cy (normR (rayR p)) (rayR p) (rayR q) := by
  unfold quadrantBox at h
  split_ifs at h with h1
  all_goals try cases h
  split at h
  all_goals try cases h
  rename_i rLo rHi hb
  obtain ⟨hloR, hρlo, hρhi⟩ := coneSqrtBounds_memR_normR hb
  have hlo : 0 < rLo := by exact_mod_cast hloR
  split_ifs at h with h3 h4 h5
  all_goals try cases h
  simp only [not_le] at h3 h4 h5
  obtain ⟨hpx, hpy, hqx, hqy⟩ := h1
  have hcross : 0 < coneCross p q := by
    simp only [coneCross]
    nlinarith
  obtain ⟨hm, hs, hall⟩ := boxSide_sound_real h
  refine ⟨hm, hs, hcross, fun w hw hin => hall _ _ ?_ ?_⟩
  all_goals
    have hab : 0 < crossR (rayR p) (rayR q) := by
      rw [← coneCross_cast]
      exact_mod_cast hcross
    obtain ⟨ha0, hb0⟩ := ne_zero_of_crossR_pos hab
    obtain ⟨hsU, hsY1, hsY2⟩ := quadrant_endpoint (py := p.2) hpx hlo hρlo hρhi hU hY h3 (by linarith)
    obtain ⟨heU, heY1, heY2⟩ := quadrant_endpoint (py := q.2) hqx hlo hρlo hρhi hU hY h3 (by linarith)
    obtain ⟨hb1, hb2, hb3, hb4⟩ := unitR_bounds hw
    have hρ : 0 < normR (rayR p) := lt_of_lt_of_le hloR hρlo
    have hpxR : (p.1 : ℝ) < 0 := by exact_mod_cast hpx
    have hpyR : (0 : ℝ) < p.2 := by exact_mod_cast hpy
    have hqxR : (q.1 : ℝ) < 0 := by exact_mod_cast hqx
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

/-- A side `arcQuadrantSide` returns holds, with the returned positive margin, at every point of the exact
arc: the circle about Center of radius `|Start − Center|`, from the Start ray counter-clockwise to the End
ray, whose sweep lies strictly between zero and a half turn. Covers `sketch.coneSideForm.arcQuadrantSide`,
`sketch.coneSqrtBounds` and `sketch.coneSideForm.boxSide`; A30, A31, A51, A52. -/
theorem arcQuadrantSide_sound {f : ConeSideForm} {cx cy sx sy ex ey Y U : ℚ} {side : ConeSide} {m : ℚ}
    (h : f.arcQuadrantSide cx cy sx sy ex ey Y U = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧ 0 < coneCross (sx - cx, sy - cy) (ex - cx, ey - cy) ∧
      f.ArcSideAt side m cx cy (normR (rayR (sx - cx, sy - cy))) (rayR (sx - cx, sy - cy))
        (rayR (ex - cx, ey - cy)) := by
  unfold arcQuadrantSide at h
  split_ifs at h with h2
  all_goals try cases h
  obtain ⟨hU, hY⟩ := h2
  exact quadrantBox_sound hU hY h

end ConeSideForm

/-- The arc inputs of `sketch.coneSideForm.arcSide`: the exact Center, Start and End coordinates (A31), and
the floats `Y`, `U` that `arcQuadrantSide` computes (A52). -/
structure ConeArcData where
  cx : ℚ
  cy : ℚ
  sx : ℚ
  sy : ℚ
  ex : ℚ
  ey : ℚ
  Y : ℚ
  U : ℚ

namespace ConeSideForm

/-- The `arcQuadrantSide` call of `arcSide`. -/
@[expose] def arcQuadrant (f : ConeSideForm) (d : ConeArcData) : ConeSide × Option ℚ :=
  f.arcQuadrantSide d.cx d.cy d.sx d.sy d.ex d.ey d.Y d.U

/-- Model of `sketch.coneSideForm.arcSide`: the quadrant proof when it returns a side, else the wedge
proof. -/
@[expose] def arcSide (E : ConeArcEnv) (f : ConeSideForm) (d : ConeArcData) : ConeSide × Option ℚ :=
  if (f.arcQuadrant d).1 ≠ .zero then f.arcQuadrant d
  else f.arcWedgeSide E d.cx d.cy d.sx d.sy d.ex d.ey

/-- A side `arcSide` returns holds, with the returned positive margin, at every point of the exact arc
(radius `|Start − Center|`, from the Start ray counter-clockwise to the End ray, sweep in `(0, π)`), whichever
proof produced it. Covers `sketch.coneSideForm.arcSide` and the functions `arcQuadrantSide_sound` and
`arcWedgeSide_sound` list; A30, A31, A50, A51, A52. -/
theorem arcSide_sound {E : ConeArcEnv} {f : ConeSideForm} {d : ConeArcData} {side : ConeSide} {m : ℚ}
    (h : f.arcSide E d = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧ 0 < coneCross (d.sx - d.cx, d.sy - d.cy) (d.ex - d.cx, d.ey - d.cy) ∧
      f.ArcSideAt side m d.cx d.cy (normR (rayR (d.sx - d.cx, d.sy - d.cy)))
        (rayR (d.sx - d.cx, d.sy - d.cy)) (rayR (d.ex - d.cx, d.ey - d.cy)) := by
  unfold arcSide at h
  split_ifs at h with hq
  · exact arcQuadrantSide_sound (f := f) h
  · exact arcWedgeSide_sound h

end ConeSideForm

end Sketch.Cone

end
