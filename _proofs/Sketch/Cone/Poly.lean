module

public import Sketch.Cone.Form

/-!
# Exact rational polynomials of the cone residual

Models the `sketch.coneFitPoly` helpers that `sketch.coneSideForm.lineSide` calls (`cone_fit_crossing.go`):
`coneFitPolyAdd`, `coneFitPolySub`, `coneFitPolyScale`, `coneFitPolyMul`, `coneFitPolyEval`,
`coneFitResidual`, `coneFitBinomial`, `coneFitBernstein` and `coneFitCoeffBounds`. A polynomial is its
coefficient list, lowest degree first, as in Go; coefficients are exact rationals (A30).

The evaluation lemmas (`coneFitPolyEval_add` and the rest) give `coneFitResidual_h` and
`coneFitResidual_F`: the residual polynomials evaluate to the cone form's `h` and `F` at the curve point.
`coneFitCoeffBounds_linear` proves the lower bound of a degree-1 polynomial on `[0, 1]` is at most every value
there, which is the only Bernstein bound `lineSide` uses.
-/

public section

namespace Sketch.Cone

/-- Model of `sketch.coneFitPolyAdd`: coefficient-wise sum, as long as the longer input. -/
@[expose] def coneFitPolyAdd : List ℚ → List ℚ → List ℚ
  | [], b => b
  | a, [] => a
  | x :: xs, y :: ys => (x + y) :: coneFitPolyAdd xs ys

/-- Model of `sketch.coneFitPolySub`: coefficient-wise difference, as long as the longer input. -/
@[expose] def coneFitPolySub : List ℚ → List ℚ → List ℚ
  | [], b => b.map (fun y => 0 - y)
  | a, [] => a
  | x :: xs, y :: ys => (x - y) :: coneFitPolySub xs ys

/-- Model of `sketch.coneFitPolyScale`. -/
@[expose] def coneFitPolyScale (a : List ℚ) (k : ℚ) : List ℚ := a.map (· * k)

/-- Model of `sketch.coneFitPolyMul` for nonempty inputs. Go accumulates `r[i+j] += a[i]·b[j]` in a double
loop; the model builds the same convolution recursively, which gives the same coefficients in exact
arithmetic. -/
@[expose] def coneFitPolyMul : List ℚ → List ℚ → List ℚ
  | [], _ => []
  | [x], b => coneFitPolyScale b x
  | x :: xs, b => coneFitPolyAdd (coneFitPolyScale b x) (0 :: coneFitPolyMul xs b)

/-- Model of `sketch.coneFitPolyEval`: Horner's rule from the highest coefficient, `r ← r·t + a[i]`. -/
@[expose] def coneFitPolyEval (a : List ℚ) (t : ℚ) : ℚ := a.foldr (fun c r => r * t + c) 0

@[simp] theorem coneFitPolyEval_nil (t : ℚ) : coneFitPolyEval [] t = 0 := rfl

@[simp] theorem coneFitPolyEval_cons (c : ℚ) (a : List ℚ) (t : ℚ) :
    coneFitPolyEval (c :: a) t = coneFitPolyEval a t * t + c := rfl

theorem coneFitPolyEval_add (a b : List ℚ) (t : ℚ) :
    coneFitPolyEval (coneFitPolyAdd a b) t = coneFitPolyEval a t + coneFitPolyEval b t := by
  induction a generalizing b with
  | nil => simp [coneFitPolyAdd]
  | cons x xs ih =>
    cases b with
    | nil => simp [coneFitPolyAdd]
    | cons y ys => simp only [coneFitPolyAdd, coneFitPolyEval_cons, ih]; ring

theorem coneFitPolyEval_sub (a b : List ℚ) (t : ℚ) :
    coneFitPolyEval (coneFitPolySub a b) t = coneFitPolyEval a t - coneFitPolyEval b t := by
  induction a generalizing b with
  | nil =>
    induction b with
    | nil => simp [coneFitPolySub]
    | cons y ys ih =>
      simp only [coneFitPolySub, List.map_cons, coneFitPolyEval_cons, coneFitPolyEval_nil] at ih ⊢
      rw [ih]; ring
  | cons x xs ih =>
    cases b with
    | nil => simp [coneFitPolySub]
    | cons y ys => simp only [coneFitPolySub, coneFitPolyEval_cons, ih]; ring

theorem coneFitPolyEval_scale (a : List ℚ) (k t : ℚ) :
    coneFitPolyEval (coneFitPolyScale a k) t = coneFitPolyEval a t * k := by
  induction a with
  | nil => simp [coneFitPolyScale]
  | cons x xs ih =>
    simp only [coneFitPolyScale, List.map_cons, coneFitPolyEval_cons] at ih ⊢
    rw [ih]; ring

theorem coneFitPolyEval_mul (a b : List ℚ) (t : ℚ) :
    coneFitPolyEval (coneFitPolyMul a b) t = coneFitPolyEval a t * coneFitPolyEval b t := by
  induction a with
  | nil => simp [coneFitPolyMul]
  | cons x xs ih =>
    cases xs with
    | nil => simp only [coneFitPolyMul, coneFitPolyEval_scale, coneFitPolyEval_cons, coneFitPolyEval_nil]; ring
    | cons y ys =>
      simp only [coneFitPolyMul] at ih ⊢
      rw [coneFitPolyEval_add, coneFitPolyEval_scale, coneFitPolyEval_cons, ih]
      simp only [coneFitPolyEval_cons]
      ring

/-- Model of the `h` and `f` fields `sketch.coneFitResidual` sets from the curve polynomials `x(t)`, `y(t)`.
The other fields of `coneFitPiece` are not read by `lineSide` and are not modeled. -/
@[expose] def coneFitResidualQ (form : ConeSideForm) (px py : List ℚ) (j : Fin 3) : List ℚ :=
  coneFitPolyAdd [form.base j] (coneFitPolyAdd (coneFitPolyScale px (form.u j)) (coneFitPolyScale py (form.v j)))

/-- Model of `piece.h` in `sketch.coneFitResidual`. -/
@[expose] def coneFitResidualH (form : ConeSideForm) (px py : List ℚ) : List ℚ :=
  let q := coneFitResidualQ form px py
  coneFitPolyAdd (coneFitPolyAdd (coneFitPolyScale (q 0) (form.axis 0)) (coneFitPolyScale (q 1) (form.axis 1)))
    (coneFitPolyScale (q 2) (form.axis 2))

/-- Model of `piece.f` in `sketch.coneFitResidual`. -/
@[expose] def coneFitResidualF (form : ConeSideForm) (px py : List ℚ) : List ℚ :=
  let q := coneFitResidualQ form px py
  let h := coneFitResidualH form px py
  let cx := coneFitPolySub (coneFitPolyScale (q 1) (form.axis 2)) (coneFitPolyScale (q 2) (form.axis 1))
  let cy := coneFitPolySub (coneFitPolyScale (q 2) (form.axis 0)) (coneFitPolyScale (q 0) (form.axis 2))
  let cz := coneFitPolySub (coneFitPolyScale (q 0) (form.axis 1)) (coneFitPolyScale (q 1) (form.axis 0))
  coneFitPolySub (coneFitPolyAdd (coneFitPolyAdd (coneFitPolyMul cx cx) (coneFitPolyMul cy cy))
    (coneFitPolyMul cz cz)) (coneFitPolyScale (coneFitPolyMul h h) form.slope2)

theorem coneFitResidualQ_eval (form : ConeSideForm) (px py : List ℚ) (j : Fin 3) (t : ℚ) :
    coneFitPolyEval (coneFitResidualQ form px py j) t =
      form.q (coneFitPolyEval px t) (coneFitPolyEval py t) j := by
  simp only [coneFitResidualQ, coneFitPolyEval_add, coneFitPolyEval_scale, coneFitPolyEval_cons,
    coneFitPolyEval_nil, ConeSideForm.q]
  ring

/-- The `h` polynomial evaluates to the cone half coordinate at the curve point. -/
theorem coneFitResidual_h (form : ConeSideForm) (px py : List ℚ) (t : ℚ) :
    coneFitPolyEval (coneFitResidualH form px py) t =
      form.h (coneFitPolyEval px t) (coneFitPolyEval py t) := by
  simp only [coneFitResidualH, coneFitPolyEval_add, coneFitPolyEval_scale, coneFitResidualQ_eval,
    ConeSideForm.h]

/-- The `f` polynomial evaluates to the cone implicit function at the curve point. -/
theorem coneFitResidual_F (form : ConeSideForm) (px py : List ℚ) (t : ℚ) :
    coneFitPolyEval (coneFitResidualF form px py) t =
      form.F (coneFitPolyEval px t) (coneFitPolyEval py t) := by
  simp only [coneFitResidualF, coneFitPolyEval_add, coneFitPolyEval_sub, coneFitPolyEval_scale,
    coneFitPolyEval_mul, coneFitResidualQ_eval, coneFitResidual_h, ConeSideForm.F, ConeSideForm.cx,
    ConeSideForm.cy, ConeSideForm.cz]

/-- Model of `sketch.coneFitBinomial`: `r ← r·(n−k+i)/i` for `i = 1..k`, in integer arithmetic (A32). -/
@[expose] def coneFitBinomial (n k : ℕ) : ℕ :=
  (List.range k).foldl (fun r i => r * (n - k + (i + 1)) / (i + 1)) 1

/-- Model of `sketch.coneFitBernstein`: compose `p(a + (b−a)·u)` by Horner's rule, then convert to degree-`n`
Bernstein coefficients `r[k] = Σ_{i≤k} q[i]·C(k,i)/C(n,i)`. -/
@[expose] def coneFitBernstein (p : List ℚ) (a b : ℚ) : List ℚ :=
  let n := p.length - 1
  let affine : List ℚ := [a, b - a]
  let q := (List.range (n + 1)).reverse.foldl
    (fun q i => coneFitPolyAdd (coneFitPolyMul q affine) [p.getD i 0]) [0]
  (List.range (n + 1)).map fun k =>
    (List.range (k + 1)).foldl
      (fun acc i => acc + q.getD i 0 * ((coneFitBinomial k i : ℚ) / (coneFitBinomial n i : ℚ))) 0

/-- Model of `sketch.coneFitCoeffBounds`: the least and greatest Bernstein coefficient. Go indexes
`coeff[0]`, so an empty list panics there; the model reads `0` instead, a path no certificate reaches. -/
@[expose] def coneFitCoeffBounds (p : List ℚ) (a b : ℚ) : ℚ × ℚ :=
  let coeff := coneFitBernstein p a b
  coeff.tail.foldl (fun acc v => (if v < acc.1 then v else acc.1, if acc.2 < v then v else acc.2))
    (coeff.headD 0, coeff.headD 0)

/-- For a degree-1 polynomial `h0 + h1·t` on `[0, 1]`, the lower bound `coneFitCoeffBounds` returns is at most
the value at every `t` in `[0, 1]`. -/
theorem coneFitCoeffBounds_linear (h0 h1 t : ℚ) (ht0 : 0 ≤ t) (ht1 : t ≤ 1) :
    (coneFitCoeffBounds [h0, h1] 0 1).1 ≤ coneFitPolyEval [h0, h1] t := by
  have hb : coneFitBernstein [h0, h1] 0 1 = [h0, h0 + h1] := by
    simp [coneFitBernstein, coneFitBinomial, coneFitPolyAdd, coneFitPolyMul, coneFitPolyScale,
      List.range_succ]
  simp only [coneFitCoeffBounds, hb, List.tail_cons, List.headD_cons, List.foldl_cons, List.foldl_nil,
    coneFitPolyEval_cons, coneFitPolyEval_nil]
  split_ifs <;> nlinarith

end Sketch.Cone

end
