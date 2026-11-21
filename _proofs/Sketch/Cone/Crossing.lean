module

public import Sketch.Cone.Certify
public import Sketch.Cone.Search

/-!
# Certified crossing of a fitted spline and a cone

Models `sketch.Sketch.CertifyConeFitCrossing` (`cone_fit_crossing.go`) with `sketch.coneFitPower`,
`sketch.coneFitMakePiece` and `coneFitPiece.global`. Each span `i` of the ideal interpolant becomes a piece:
its Bézier control points (`coneFitBezier`) in power form (`coneFitPower`), and the cone half polynomial `h`,
implicit function polynomial `f` and derivative `df` of the curve (`coneFitResidual`). The search
(`Sketch/Cone/Search.lean`) visits every piece on `[0, 1]`, the bisection narrows the root bracket, and a
final integer Bernstein bound on `df` over the narrowed bracket (`intBounds`) must exclude zero.

The ideal spline is evaluated at real parameters: `FitData.spanPointR` is span `i` at local `u ∈ ℝ`, and
`fitCurveR` maps it to the world through the plane frame. `worldFR` and `worldHR` (`Sketch/Cone/Real.lean`)
are the cone implicit function and half coordinate of `Sketch/Cone/Form.lean` at a real world point.

`certifyConeFitCrossing_sound` states the guarantee of a returned certificate: every point of the ideal
spline lies in the positive cone half; the spline meets the cone surface at exactly one parameter `u*`, inside
the certified local bracket of the root span; it crosses transversally there (nonzero derivative of `F` along
the curve); the normalized parameter of `u*` lies in `TBounds`; and every point before the crossing lies
strictly on `BeforeSide`, every point after it strictly on `AfterSide`.
-/

public section

namespace Sketch.Cone

/-- Model of `sketch.coneFitPower`: the power-basis coefficients of one axis of a cubic Bézier curve. -/
@[expose] def coneFitPower (b : ConeBezier) (axis : Fin 2) : List ℚ :=
  [b.controls 0 axis, 3 * (b.controls 1 axis - b.controls 0 axis),
    3 * ((b.controls 0 axis - 2 * b.controls 1 axis) + b.controls 2 axis),
    (3 * b.controls 1 axis - b.controls 0 axis) + (b.controls 3 axis - 3 * b.controls 2 axis)]

/-- Model of `sketch.coneFitMakePiece` and the `coneFitResidual` call it makes: the span parameters, the curve
polynomials of span `i`, and the residual polynomials `h`, `f` and `df = coneFitPolyDeriv f`. -/
@[expose] def coneFitMakePiece (fi : FitData) (i : ℕ) (form : ConeSideForm) : ConeFitPiece where
  p0 := fi.params.getD i 0
  p1 := fi.params.getD (i + 1) 0
  total := fi.params.getD (fi.params.length - 1) 0
  xy := ![coneFitPower (coneFitBezier fi i) 0, coneFitPower (coneFitBezier fi i) 1]
  h := coneFitResidualH form (coneFitPower (coneFitBezier fi i) 0) (coneFitPower (coneFitBezier fi i) 1)
  f := coneFitResidualF form (coneFitPower (coneFitBezier fi i) 0) (coneFitPower (coneFitBezier fi i) 1)
  df := coneFitPolyDeriv
    (coneFitResidualF form (coneFitPower (coneFitBezier fi i) 0) (coneFitPower (coneFitBezier fi i) 1))

/-- `coneFitResidual` sets `df` to the derivative of `f`. -/
theorem coneFitMakePiece_df (fi : FitData) (i : ℕ) (form : ConeSideForm) :
    (coneFitMakePiece fi i form).df = coneFitPolyDeriv (coneFitMakePiece fi i form).f := by
  simp only [coneFitMakePiece]

/-- Model of `coneFitPiece.global`: the normalized spline parameter of local parameter `l`. -/
@[expose] def ConeFitPiece.global (p : ConeFitPiece) (l : ℚ) : ℚ := (p.p0 + (p.p1 - p.p0) * l) / p.total

namespace FitData

/-- Coordinate `axis` of span `i` of the ideal interpolant at a real local parameter `u`. -/
@[expose] noncomputable def spanPointR (fi : FitData) (i : ℕ) (u : ℝ) (axis : Fin 2) : ℝ :=
  fi.coeff i axis 0 + u * (fi.coeff i axis 1 + u * (fi.coeff i axis 2 + u * fi.coeff i axis 3))

/-- The normalized spline parameter of local parameter `u` on span `i`: `(params[i] + width·u)/params[last]`. -/
@[expose] noncomputable def globalT (fi : FitData) (i : ℕ) (u : ℝ) : ℝ :=
  ((fi.params.getD i 0 : ℝ) + (fi.width i : ℝ) * u) / (fi.params.getD (fi.params.length - 1) 0 : ℝ)

end FitData

theorem evalR_coneFitPower (fi : FitData) (i : ℕ) (u : ℝ) (axis : Fin 2) :
    evalR (coneFitPower (coneFitBezier fi i) axis) u = fi.spanPointR i u axis := by
  simp only [coneFitPower, coneFitBezier, evalR_cons, evalR_nil, FitData.spanPointR, FitData.coeff,
    Matrix.cons_val_zero, Matrix.cons_val_one, Matrix.cons_val_two, Matrix.cons_val_three, Matrix.head_cons,
    Matrix.tail_cons]
  push_cast
  ring

/-- The spans join: the end of span `i` is the start of span `i + 1`, the active point `i + 1`. -/
theorem spanPointR_join (fi : FitData) (i : ℕ) (axis : Fin 2) :
    fi.spanPointR i 1 axis = fi.spanPointR (i + 1) 0 axis := by
  simp only [FitData.spanPointR, FitData.coeff, Matrix.cons_val_zero, Matrix.cons_val_one, Matrix.cons_val_two,
    Matrix.cons_val_three, Matrix.head_cons, Matrix.tail_cons]
  push_cast
  ring

theorem evalR_coneFitResidualQ (form : ConeSideForm) (px py : List ℚ) (j : Fin 3) (t : ℝ) :
    evalR (coneFitResidualQ form px py j) t = form.qR (evalR px t) (evalR py t) j := by
  simp only [coneFitResidualQ, evalR_add, evalR_scale, evalR_cons, evalR_nil, ConeSideForm.qR]
  ring

theorem evalR_coneFitResidualH (form : ConeSideForm) (px py : List ℚ) (t : ℝ) :
    evalR (coneFitResidualH form px py) t = form.hR (evalR px t) (evalR py t) := by
  simp only [coneFitResidualH, evalR_add, evalR_scale, evalR_coneFitResidualQ, ConeSideForm.hR]

theorem evalR_coneFitResidualF (form : ConeSideForm) (px py : List ℚ) (t : ℝ) :
    evalR (coneFitResidualF form px py) t = form.FR (evalR px t) (evalR py t) := by
  simp only [coneFitResidualF, evalR_add, evalR_sub, evalR_scale, evalR_mul, evalR_coneFitResidualQ,
    evalR_coneFitResidualH, ConeSideForm.FR]

/-- The world point of span `i` of the ideal interpolant at real local parameter `u`. -/
@[expose] noncomputable def fitCurveR (o U V : Fin 3 → ℚ) (fi : FitData) (i : ℕ) (u : ℝ) : Fin 3 → ℝ :=
  worldPointR o U V (fi.spanPointR i u 0) (fi.spanPointR i u 1)

theorem piece_f_eval (o U V A a : Fin 3 → ℚ) (s : ℚ) (fi : FitData) (i : ℕ) (u : ℝ) :
    evalR (coneFitMakePiece fi i (newConeSideForm o U V A a s)).f u = worldFR A a s (fitCurveR o U V fi i u) := by
  simp only [coneFitMakePiece, evalR_coneFitResidualF, evalR_coneFitPower, newConeSideForm_FR, fitCurveR]

theorem piece_h_eval (o U V A a : Fin 3 → ℚ) (s : ℚ) (fi : FitData) (i : ℕ) (u : ℝ) :
    evalR (coneFitMakePiece fi i (newConeSideForm o U V A a s)).h u = worldHR A a (fitCurveR o U V fi i u) := by
  simp only [coneFitMakePiece, evalR_coneFitResidualH, evalR_coneFitPower, newConeSideForm_hR, fitCurveR]

/-- Side `side` holds strictly for the implicit function value `v`: `v > 0` outside, `v < 0` inside. -/
@[expose] def StrictSide (side : ConeSide) (v : ℝ) : Prop :=
  match side with
  | .outside => 0 < v
  | .inside => v < 0
  | .zero => False

/-- Sign propagation along the joined pieces: with one zero `u` of the piece functions over `N` pieces, in
piece `R` between `lo` and `hi`, every point before it keeps the sign `σ` of `lo` and every point after it the
sign of `hi`. -/
theorem sides_sound (F : ℕ → List ℚ) {N R : ℕ} (hR : R < N) (σ : ℝ) {lo hi u : ℝ} (h0 : 0 ≤ lo)
    (hlo : lo < u) (hhi : u < hi) (h1 : hi ≤ 1) (hslo : 0 < σ * evalR (F R) lo)
    (hshi : σ * evalR (F R) hi < 0)
    (hZ : ∀ i < N, ∀ t : ℝ, 0 ≤ t → t ≤ 1 → evalR (F i) t = 0 → i = R ∧ t = u)
    (hJ : ∀ i, i + 1 < N → evalR (F i) 1 = evalR (F (i + 1)) 0) :
    (∀ i < N, ∀ t : ℝ, 0 ≤ t → t ≤ 1 → (i < R ∨ (i = R ∧ t < u)) → 0 < σ * evalR (F i) t) ∧
      (∀ i < N, ∀ t : ℝ, 0 ≤ t → t ≤ 1 → (R < i ∨ (i = R ∧ u < t)) → σ * evalR (F i) t < 0) := by
  have inBefore : ∀ t : ℝ, 0 ≤ t → t < u → 0 < σ * evalR (F R) t := by
    intro t ht0 htu
    refine sign_keep (F R) σ (fun z hz hzf => ?_) hslo
    have hz' : 0 ≤ z ∧ z < u := by
      rcases Set.mem_uIcc.mp hz with ⟨a, b⟩ | ⟨a, b⟩ <;> constructor <;> linarith
    have := (hZ R hR z hz'.1 (by linarith) hzf).2
    linarith
  have inAfter : ∀ t : ℝ, t ≤ 1 → u < t → 0 < -σ * evalR (F R) t := by
    intro t ht1 hut
    refine sign_keep (F R) (-σ) (fun z hz hzf => ?_) (by linarith)
    have hz' : u < z ∧ z ≤ 1 := by
      rcases Set.mem_uIcc.mp hz with ⟨a, b⟩ | ⟨a, b⟩ <;> constructor <;> linarith
    have := (hZ R hR z (by linarith) hz'.2 hzf).2
    linarith
  -- A piece other than `R` keeps one sign on `[0, 1]`.
  have keep : ∀ i < N, i ≠ R → ∀ (τ x t : ℝ), 0 ≤ x → x ≤ 1 → 0 ≤ t → t ≤ 1 → 0 < τ * evalR (F i) x →
      0 < τ * evalR (F i) t := by
    intro i hi hne τ x t hx0 hx1 ht0 ht1 hx
    refine sign_keep (F i) τ (fun z hz hzf => ?_) hx
    have hz' : 0 ≤ z ∧ z ≤ 1 := by
      rcases Set.mem_uIcc.mp hz with ⟨a, b⟩ | ⟨a, b⟩ <;> constructor <;> linarith
    exact hne (hZ i hi z hz'.1 hz'.2 hzf).1
  have below : ∀ k i, i + k + 1 = R → ∀ t : ℝ, 0 ≤ t → t ≤ 1 → 0 < σ * evalR (F i) t := by
    intro k
    induction k with
    | zero =>
      intro i hi t ht0 ht1
      have hs : 0 < σ * evalR (F (i + 1)) 0 := by rw [show i + 1 = R by omega]; exact inBefore 0 le_rfl (by linarith)
      rw [← hJ i (by omega)] at hs
      exact keep i (by omega) (by omega) σ 1 t zero_le_one le_rfl ht0 ht1 hs
    | succ k ih =>
      intro i hi t ht0 ht1
      have hs := ih (i + 1) (by omega) 0 le_rfl zero_le_one
      rw [← hJ i (by omega)] at hs
      exact keep i (by omega) (by omega) σ 1 t zero_le_one le_rfl ht0 ht1 hs
  have above : ∀ k i, i = R + k + 1 → i < N → ∀ t : ℝ, 0 ≤ t → t ≤ 1 → 0 < -σ * evalR (F i) t := by
    intro k
    induction k with
    | zero =>
      intro i hi hiN t ht0 ht1
      have hs := inAfter 1 le_rfl (by linarith)
      rw [hJ R (by omega), show R + 1 = i by omega] at hs
      exact keep i hiN (by omega) (-σ) 0 t le_rfl zero_le_one ht0 ht1 hs
    | succ k ih =>
      intro i hi hiN t ht0 ht1
      have hs := ih (R + k + 1) rfl (by omega) 1 zero_le_one le_rfl
      rw [hJ (R + k + 1) (by omega), show R + k + 1 + 1 = i by omega] at hs
      exact keep i hiN (by omega) (-σ) 0 t le_rfl zero_le_one ht0 ht1 hs
  refine ⟨fun i _ t ht0 ht1 hpos => ?_, fun i hi t ht0 ht1 hpos => ?_⟩
  · rcases hpos with h | ⟨rfl, h⟩
    · exact below (R - i - 1) i (by omega) t ht0 ht1
    · exact inBefore t ht0 h
  · rcases hpos with h | ⟨rfl, h⟩
    · have := above (i - R - 1) i (by omega) hi t ht0 ht1
      linarith
    · have := inAfter t ht1 h
      linarith

/-- The certificate fields of `sketch.ConeFitCrossing` the guarantee speaks about: the root span, the local
bracket `localLo`, `localHi`, the bounds `TBounds` returns, and the two sides. -/
structure ConeFitCrossingCert where
  root : ℕ
  localLo : ℚ
  localHi : ℚ
  tLo : ℚ
  tHi : ℚ
  before : ConeSide
  after : ConeSide

/-- Model of `sketch.Sketch.CertifyConeFitCrossing`. The frame origin `o`, plane axes `U`, `V`, cone apex `A`,
axis `a`, slope `s` and the interpolant fields are the exact values of the Go floats (A31). The Go screens of
the sketch, the cone data and the frame only refuse, so the model omits them; it keeps the interpolant shape
checks. `stop` stands for the bisection's float-precision exit, and `down`, `up` for `coneFitFloatDown` and
`coneFitFloatUp` (A60). The final float checks on `t`, `tlo` and `thi` only refuse and are omitted, as is the
rounded midpoint `T`. So Go returns a certificate only when the model does, with these fields. -/
@[expose] def certifyConeFitCrossing (o U V A a : Fin 3 → ℚ) (s : ℚ) (fi : FitData)
    (stop : ℕ → ℚ → ℚ → Bool) (down up : ℚ → ℚ) : Option ConeFitCrossingCert :=
  if fi.points.length < 2 ∨ fi.params.length ≠ fi.points.length ∨ fi.derivs.length ≠ fi.points.length ∨
      fi.params.getD 0 0 ≠ 0 ∨ fi.params.getD (fi.params.length - 1) 0 ≤ 0 ∨
      ∃ i < fi.points.length - 1, fi.width i ≤ 0 then none
  else
    match (List.range (fi.points.length - 1)).foldl
        (searchStep fun i => coneFitMakePiece fi i (newConeSideForm o U V A a s)) (some (8192, none)) with
    | some (_, some r) =>
      if (intBounds (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).df
            (bisect (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).f r.loSign stop 256 0 r.lo r.hi).1
            (bisect (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).f r.loSign stop 256 0 r.lo r.hi).2).1
          ≤ 0 ∧
        0 ≤ (intBounds (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).df
            (bisect (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).f r.loSign stop 256 0 r.lo r.hi).1
            (bisect (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).f r.loSign stop 256 0 r.lo r.hi).2).2
      then none
      else some
        { root := r.index
          localLo := (bisect (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).f r.loSign stop 256 0
            r.lo r.hi).1
          localHi := (bisect (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).f r.loSign stop 256 0
            r.lo r.hi).2
          tLo := down ((coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).global
            (bisect (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).f r.loSign stop 256 0
              r.lo r.hi).1)
          tHi := up ((coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).global
            (bisect (coneFitMakePiece fi r.index (newConeSideForm o U V A a s)).f r.loSign stop 256 0
              r.lo r.hi).2)
          before := if 0 < r.loSign then .outside else .inside
          after := if 0 < r.loSign then .inside else .outside }
    | _ => none

theorem globalT_mono (fi : FitData) (i : ℕ) (hw : 0 < fi.width i)
    (ht : 0 < fi.params.getD (fi.params.length - 1) 0) {x y : ℝ} (hxy : x ≤ y) :
    fi.globalT i x ≤ fi.globalT i y := by
  unfold FitData.globalT
  have hwR : (0 : ℝ) < fi.width i := by exact_mod_cast hw
  have htR : (0 : ℝ) < fi.params.getD (fi.params.length - 1) 0 := by exact_mod_cast ht
  gcongr

theorem piece_global_eq (fi : FitData) (i : ℕ) (form : ConeSideForm) (l : ℚ) :
    (((coneFitMakePiece fi i form).global l : ℚ) : ℝ) = fi.globalT i l := by
  simp only [ConeFitPiece.global, coneFitMakePiece, FitData.globalT, FitData.width]
  push_cast
  ring

/-- A certificate from `CertifyConeFitCrossing` proves exactly one transverse crossing of the ideal fitted
spline with the positive cone section, in world coordinates. For every span `i` and local parameter
`u ∈ [0, 1]`, the world point `fitCurveR o U V fi i u` lies in the positive cone half. The cone function
`worldFR` vanishes at exactly one span and parameter, the root span `c.root` at `u*`, which lies in the local
bracket `[c.localLo, c.localHi]` strictly inside `(0, 1)`. The derivative of `worldFR` along the curve at `u*`
is nonzero. The normalized parameter of `u*` lies in `[c.tLo, c.tHi]`. Every point before the crossing
(an earlier span, or the root span before `u*`) lies strictly on `c.before`, every point after it strictly on
`c.after`, and the two sides differ. Covers `sketch.Sketch.CertifyConeFitCrossing`,
`sketch.coneFitSearch.visit`, `sketch.coneFitMakePiece`, `sketch.coneFitPower`, `sketch.coneFitResidual`,
`sketch.coneFitPolyDeriv`, `coneFitPiece.global`, `sketch.coneFitBezier`, `sketch.newConeSideForm`, the
integer Bernstein bound `coneFitIntBounds_mem` (`sketch.coneFitIntBernstein`, `sketch.coneFitIntBounds`,
`sketch.coneFitFactorial`), the `coneFitPoly` helpers and the sign test `sketch.coneFitIntPoly`,
`sketch.coneFitIntSign`; assumptions A30, A31, A32, A33, A60, A100, A110, A111. -/
theorem certifyConeFitCrossing_sound {o U V A a : Fin 3 → ℚ} {s : ℚ} {fi : FitData}
    {stop : ℕ → ℚ → ℚ → Bool} {down up : ℚ → ℚ} (hdown : ∀ r, down r ≤ r) (hup : ∀ r, r ≤ up r)
    {c : ConeFitCrossingCert} (h : certifyConeFitCrossing o U V A a s fi stop down up = some c) :
    c.root + 1 < fi.points.length ∧
      (∀ i, i + 1 < fi.points.length → ∀ u : ℝ, 0 ≤ u → u ≤ 1 → 0 < worldHR A a (fitCurveR o U V fi i u)) ∧
      ∃ ustar : ℝ, (c.localLo : ℝ) ≤ ustar ∧ ustar ≤ c.localHi ∧ 0 < ustar ∧ ustar < 1 ∧
        worldFR A a s (fitCurveR o U V fi c.root ustar) = 0 ∧
        (∀ i, i + 1 < fi.points.length → ∀ u : ℝ, 0 ≤ u → u ≤ 1 →
          worldFR A a s (fitCurveR o U V fi i u) = 0 → i = c.root ∧ u = ustar) ∧
        (∃ d : ℝ, d ≠ 0 ∧ HasDerivAt (fun u => worldFR A a s (fitCurveR o U V fi c.root u)) d ustar) ∧
        (c.tLo : ℝ) ≤ fi.globalT c.root ustar ∧ fi.globalT c.root ustar ≤ c.tHi ∧
        c.before ≠ c.after ∧
        (∀ i, i + 1 < fi.points.length → ∀ u : ℝ, 0 ≤ u → u ≤ 1 → (i < c.root ∨ (i = c.root ∧ u < ustar)) →
          StrictSide c.before (worldFR A a s (fitCurveR o U V fi i u))) ∧
        (∀ i, i + 1 < fi.points.length → ∀ u : ℝ, 0 ≤ u → u ≤ 1 → (c.root < i ∨ (i = c.root ∧ ustar < u)) →
          StrictSide c.after (worldFR A a s (fitCurveR o U V fi i u))) := by
  unfold certifyConeFitCrossing at h
  split_ifs at h with hscreen
  simp only [not_or, not_lt, not_le, not_exists, not_and] at hscreen
  obtain ⟨hn, hpl, -, -, htot, hwid⟩ := hscreen
  set form := newConeSideForm o U V A a s with hform
  set pieces := fun i => coneFitMakePiece fi i form with hpieces
  have hinv := searchInv_foldl pieces (fun i => coneFitMakePiece_df fi i form) 8192 (fi.points.length - 1)
  generalize hst : (List.range (fi.points.length - 1)).foldl (searchStep pieces) (some (8192, none)) = st at h hinv
  rcases st with _ | ⟨bud, _ | r⟩
  · simp at h
  · simp at h
  obtain ⟨hh, hz⟩ := hinv bud (some r) rfl
  obtain ⟨r', hr', hidx, hrok, hzr⟩ := hz.resolve_left (fun h => by simp at h)
  simp only [Option.some.injEq] at hr'
  subst hr'
  simp only at h
  set R := r.index with hRdef
  set piece := coneFitMakePiece fi R form with hpiece
  obtain ⟨hlo0, hlohi, hhi1, hσ, hslo, hshi, ustar, hu0, hu1, huf, huniq⟩ := hrok
  have hbis := bisect_sound piece.f r.loSign hσ stop (u := ustar)
    (fun t ht0 ht1 htf => huniq t (le_trans (by exact_mod_cast hlo0) ht0)
      (le_trans ht1 (by exact_mod_cast hhi1)) htf) 256 0 r.lo r.hi le_rfl hlohi.le le_rfl hslo hshi
  set br := bisect piece.f r.loSign stop 256 0 r.lo r.hi with hbr
  obtain ⟨hb1, hb12, hb2, hbu1, hbu2⟩ := hbis
  by_cases hd : (intBounds piece.df br.1 br.2).1 ≤ 0 ∧ 0 ≤ (intBounds piece.df br.1 br.2).2
  · rw [ite_eq_left hd] at h
    simp at h
  rw [ite_eq_right hd] at h
  simp only [Option.some.injEq] at h
  subst h
  simp only
  have hNR : R + 1 < fi.points.length := by omega
  have hfeq : ∀ i u, evalR (pieces i).f u = worldFR A a s (fitCurveR o U V fi i u) :=
    fun i u => piece_f_eval o U V A a s fi i u
  have hheq : ∀ i u, evalR (pieces i).h u = worldHR A a (fitCurveR o U V fi i u) :=
    fun i u => piece_h_eval o U V A a s fi i u
  have hZ : ∀ i < fi.points.length - 1, ∀ t : ℝ, 0 ≤ t → t ≤ 1 → evalR (pieces i).f t = 0 →
      i = R ∧ t = ustar := by
    intro i hi t ht0 ht1 htf
    have hiR := hzr i hi t ht0 ht1 htf
    subst hiR
    exact ⟨rfl, huniq t (by simpa using ht0) (by simpa using ht1) htf⟩
  have hJ : ∀ i, i + 1 < fi.points.length - 1 → evalR (pieces i).f 1 = evalR (pieces (i + 1)).f 0 := by
    intro i _
    rw [hfeq, hfeq]
    simp only [fitCurveR, spanPointR_join]
  have hlo0R : (0 : ℝ) ≤ r.lo := by exact_mod_cast hlo0
  have hhi1R : (r.hi : ℝ) ≤ 1 := by exact_mod_cast hhi1
  obtain ⟨hbefore, hafter⟩ := sides_sound (fun i => (pieces i).f) (N := fi.points.length - 1) (by omega)
    (r.loSign : ℝ) hlo0R hu0 hu1 hhi1R hslo hshi hZ hJ
  have hdne : evalR piece.df ustar ≠ 0 := by
    have hb12R : br.1 ≤ br.2 := hb12
    obtain ⟨sc, hsc, hm'⟩ := coneFitIntBounds_mem piece.df hb12R
    have hm := hm' ustar hbu1 hbu2
    intro hz
    rw [hz, mul_zero] at hm
    rcases not_and_or.mp hd with hd | hd
    · have : (0 : ℝ) < (intBounds piece.df br.1 br.2).1 := by exact_mod_cast not_le.mp hd
      simp only [intBounds] at this
      linarith [hm.1]
    · have : ((intBounds piece.df br.1 br.2).2 : ℝ) < 0 := by exact_mod_cast not_le.mp hd
      simp only [intBounds] at this
      linarith [hm.2]
  refine ⟨hNR, fun i hi u hu0' hu1' => ?_, ustar, hbu1, hbu2, by linarith, by linarith, ?_, ?_, ?_, ?_, ?_, ?_,
    ?_, ?_⟩
  · rw [← hheq]
    exact hh i (by omega) u hu0' hu1'
  · rw [← hfeq]; exact huf
  · intro i hi u hu0' hu1' hf0
    rw [← hfeq] at hf0
    exact hZ i (by omega) u hu0' hu1' hf0
  · refine ⟨evalR piece.df ustar, hdne, ?_⟩
    have hfun : (fun u => worldFR A a s (fitCurveR o U V fi R u)) = evalR piece.f := by
      funext u; rw [← hfeq]
    rw [hfun, hpiece, coneFitMakePiece_df]
    exact hasDerivAt_evalR _ ustar
  · have hw : 0 < fi.width R := hwid R (by omega)
    have h1 := hdown (piece.global br.1)
    have h2 : (piece.global br.1 : ℝ) ≤ fi.globalT R ustar := by
      rw [hpiece, piece_global_eq]; exact globalT_mono fi R hw htot hbu1
    calc ((down (piece.global br.1) : ℚ) : ℝ) ≤ (piece.global br.1 : ℝ) := by exact_mod_cast h1
      _ ≤ _ := h2
  · have hw : 0 < fi.width R := hwid R (by omega)
    have h1 := hup (piece.global br.2)
    have h2 : fi.globalT R ustar ≤ (piece.global br.2 : ℝ) := by
      rw [hpiece, piece_global_eq]; exact globalT_mono fi R hw htot hbu2
    calc fi.globalT R ustar ≤ (piece.global br.2 : ℝ) := h2
      _ ≤ ((up (piece.global br.2) : ℚ) : ℝ) := by exact_mod_cast h1
  · split_ifs <;> simp
  · intro i hi u hu0' hu1' hord
    have hs := hbefore i (by omega) u hu0' hu1' hord
    rw [hfeq] at hs
    rcases hσ with h | h <;> rw [h] at hs ⊢ <;> simp [StrictSide] at hs ⊢ <;> linarith
  · intro i hi u hu0' hu1' hord
    have hs := hafter i (by omega) u hu0' hu1' hord
    rw [hfeq] at hs
    rcases hσ with h | h <;> rw [h] at hs ⊢ <;> simp [StrictSide] at hs ⊢ <;> linarith

end Sketch.Cone

end
