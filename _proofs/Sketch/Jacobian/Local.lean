module

public import Mathlib

/-!
# Local residual Jacobian equals the dense one

Models `sketch.Sketch.jacobianInto` (the dense central-difference Jacobian) and `sketch.Sketch.jacobianLocalInto`
(`jacobian.go`) over one abstract value type `α` standing for float64, with an abstract residual function and a
variable-to-constraint index (`residualPlan`).

Both builders write the same expression `(F(x + h·e_v)[i] − F(x − h·e_v)[i])·inv` into entry `(i, j)`. The
dense one writes it for every row. The local one writes it for the rows of the constraints variable `v`
reaches, and writes zero elsewhere. `jacobianLocal_eq_dense` proves they agree whenever the local one does not
refuse, from one independence hypothesis: a constraint whose index set omits `v` does not read `v`. That
hypothesis follows from the plan's acceptance checks and assumption A44. `jacobianLocal_isSome_iff` lists every
refusal condition. `jacobianLocal_eq_none_or_dense` states the result with no side conditions beyond A43 to A45.
-/

public section

namespace Sketch.Jacobian

/-- The float64 operations a Jacobian column uses, and the two IEEE facts the zero columns rely on (A43). -/
structure Ops (α : Type) where
  /-- Not NaN and not infinite: the complement of `nonFinite`. -/
  fin : α → Prop
  zero : α
  sub : α → α → α
  mul : α → α → α
  /-- `h = 1e-7 * (1 + |orig|)`. -/
  step : α → α
  /-- `orig + h`. -/
  plus : α → α → α
  /-- `orig - h`. -/
  minus : α → α → α
  /-- `1.0 / (2 * h)`. -/
  inv : α → α
  sub_self : ∀ a, fin a → sub a a = zero
  zero_mul : ∀ b, fin b → mul zero b = zero

/-- An abstract residual vector of `m` rows over `nc` constraints. `own i` is the constraint that owns row `i`,
and `reads k` the variables constraint `k` reads (A45). A row is a function of the variables its owner reads. -/
structure Residual (α : Type) (m nc : ℕ) where
  F : (ℕ → α) → Fin m → α
  own : Fin m → Fin nc
  reads : Fin nc → Set ℕ
  F_reads : ∀ i x y, (∀ v ∈ reads (own i), x v = y v) → F x i = F y i

/-- Model of `residualPlan`: the computed index set of each constraint (`constraintVarIndices`), the four
reasons it refuses, the measured row total, and the index itself. -/
structure Plan (nc : ℕ) where
  /-- Variables `constraintVarIndices` returns for constraint `k`. -/
  idx : Fin nc → Set ℕ
  /-- The constraint names at least one operand. -/
  named : Fin nc → Prop
  /-- Every operand is owned by this sketch. -/
  owned : Fin nc → Prop
  /-- Auxiliary variables, if any, were allocated by this sketch. -/
  auxOwned : Fin nc → Prop
  /-- Every index is inside the variable vector. -/
  inRange : Fin nc → Prop
  /-- `pl.m`. -/
  rows : ℕ

namespace Plan
variable {nc : ℕ}

/-- `buildResidualPlan` accepts constraint `k`. -/
@[expose] def accepted (P : Plan nc) (k : Fin nc) : Prop :=
  P.named k ∧ P.owned k ∧ P.auxOwned k ∧ P.inRange k

/-- `consFor v`: the constraints variable `v` reaches. -/
@[expose] def consFor (P : Plan nc) (v : ℕ) : Set (Fin nc) := {k | v ∈ P.idx k}

end Plan

namespace Ops
variable {α : Type} (O : Ops α)

/-- Entry `(i, v)` of the central difference, written by both builders. -/
@[expose] def quot {m nc : ℕ} (R : Residual α m nc) (x : ℕ → α) (v : ℕ) (i : Fin m) : α :=
  O.mul (O.sub (R.F (Function.update x v (O.plus (x v) (O.step (x v)))) i)
    (R.F (Function.update x v (O.minus (x v) (O.step (x v)))) i)) (O.inv (O.step (x v)))

end Ops

namespace Ops
variable {α : Type} (O : Ops α) {m nc n : ℕ}

/-- Model of `sketch.Sketch.jacobianInto`: every row of every free column. -/
@[expose] def jacobianDense (R : Residual α m nc) (x : ℕ → α) (free : Fin n → ℕ) : Fin m → Fin n → α :=
  fun i j => O.quot R x (free j) i

/-- Every condition under which `jacobianLocalInto` does not refuse. `baseLen` is `len(base)`, `evalOK v` is the
row-count check `evalRows` makes on the perturbed rows of variable `v`. -/
@[expose] def Guards (P : Plan nc) (x : ℕ → α) (free : Fin n → ℕ) (baseLen : ℕ)
    (base : Fin m → α) (evalOK : ℕ → Prop) : Prop :=
  P.rows = m ∧ baseLen = m ∧ (∀ k, P.accepted k) ∧ (∀ i, O.fin (base i)) ∧
    (∀ j, O.fin (O.inv (O.step (x (free j))))) ∧ ∀ j, (P.consFor (free j)).Nonempty → evalOK (free j)

open Classical in
/-- Model of `sketch.Sketch.jacobianLocalInto`: `none` is a refusal (the caller runs the dense pass). The
column of a variable no constraint reaches is zero. -/
@[expose] noncomputable def jacobianLocal (R : Residual α m nc) (P : Plan nc) (x : ℕ → α) (free : Fin n → ℕ)
    (baseLen : ℕ) (base : Fin m → α) (evalOK : ℕ → Prop) : Option (Fin m → Fin n → α) :=
  if O.Guards P x free baseLen base evalOK then
    some fun i j => if R.own i ∈ P.consFor (free j) then O.quot R x (free j) i else O.zero
  else none

/-- `jacobianLocalInto` refuses exactly when one of: the plan's row total or the base length is not `m`; a
constraint is unclassified (names nothing, foreign operand, foreign auxiliary variables, index out of range);
a base residual is non-finite; a free variable's `1/(2h)` is non-finite; a reached variable's perturbed rows
have a different count than the plan recorded. -/
theorem jacobianLocal_isSome_iff (R : Residual α m nc) (P : Plan nc) (x : ℕ → α) (free : Fin n → ℕ)
    (baseLen : ℕ) (base : Fin m → α) (evalOK : ℕ → Prop) :
    (O.jacobianLocal R P x free baseLen base evalOK).isSome ↔
      O.Guards P x free baseLen base evalOK := by
  unfold jacobianLocal
  split_ifs with h <;> simp [h]

/-- A constraint the plan does not accept makes `jacobianLocalInto` refuse. -/
theorem jacobianLocal_none_of_unaccepted (R : Residual α m nc) (P : Plan nc) (x : ℕ → α) (free : Fin n → ℕ)
    (baseLen : ℕ) (base : Fin m → α) (evalOK : ℕ → Prop) {k : Fin nc} (hk : ¬ P.accepted k) :
    O.jacobianLocal R P x free baseLen base evalOK = none := by
  unfold jacobianLocal
  split_ifs with h
  · exact absurd h (fun h => hk (h.2.2.1 k))
  · rfl

/-- A non-finite base residual makes `jacobianLocalInto` refuse. -/
theorem jacobianLocal_none_of_nonfinite_base (R : Residual α m nc) (P : Plan nc) (x : ℕ → α)
    (free : Fin n → ℕ) (baseLen : ℕ) (base : Fin m → α) (evalOK : ℕ → Prop) {i : Fin m}
    (hi : ¬ O.fin (base i)) : O.jacobianLocal R P x free baseLen base evalOK = none := by
  unfold jacobianLocal
  split_ifs with h
  · exact absurd h (fun h => hi (h.2.2.2.1 i))
  · rfl

/-- A free variable whose `1/(2h)` is non-finite (a NaN variable) makes `jacobianLocalInto` refuse, even when
no constraint reaches it. -/
theorem jacobianLocal_none_of_nonfinite_inv (R : Residual α m nc) (P : Plan nc) (x : ℕ → α)
    (free : Fin n → ℕ) (baseLen : ℕ) (base : Fin m → α) (evalOK : ℕ → Prop) {j : Fin n}
    (hj : ¬ O.fin (O.inv (O.step (x (free j))))) :
    O.jacobianLocal R P x free baseLen base evalOK = none := by
  unfold jacobianLocal
  split_ifs with h
  · exact absurd h (fun h => hj (h.2.2.2.2.1 j))
  · rfl

/-- A plan or base vector whose length is not `m` makes `jacobianLocalInto` refuse. -/
theorem jacobianLocal_none_of_length (R : Residual α m nc) (P : Plan nc) (x : ℕ → α) (free : Fin n → ℕ)
    (baseLen : ℕ) (base : Fin m → α) (evalOK : ℕ → Prop) (hlen : ¬ (P.rows = m ∧ baseLen = m)) :
    O.jacobianLocal R P x free baseLen base evalOK = none := by
  unfold jacobianLocal
  split_ifs with h
  · exact absurd h (fun h => hlen ⟨h.1, h.2.1⟩)
  · rfl

/-- A reached variable whose perturbed rows change count makes `jacobianLocalInto` refuse. -/
theorem jacobianLocal_none_of_evalRows (R : Residual α m nc) (P : Plan nc) (x : ℕ → α) (free : Fin n → ℕ)
    (baseLen : ℕ) (base : Fin m → α) (evalOK : ℕ → Prop) {j : Fin n}
    (hne : (P.consFor (free j)).Nonempty) (hj : ¬ evalOK (free j)) :
    O.jacobianLocal R P x free baseLen base evalOK = none := by
  unfold jacobianLocal
  split_ifs with h
  · exact absurd h (fun h => hj (h.2.2.2.2.2 j hne))
  · rfl

/-- Independence: on an accepted plan, a constraint whose index set omits `v` does not depend on `v`, so a
perturbation of `v` leaves its rows unchanged. Uses A44. -/
theorem row_indep (R : Residual α m nc) (P : Plan nc)
    (hcover : ∀ k, P.accepted k → R.reads k ⊆ P.idx k) (hacc : ∀ k, P.accepted k) (x : ℕ → α) (v : ℕ) (t : α)
    (i : Fin m) (hi : R.own i ∉ P.consFor v) : R.F (Function.update x v t) i = R.F x i := by
  apply R.F_reads
  intro w hw
  have hne : w ≠ v := by
    rintro rfl
    exact hi (hcover _ (hacc _) hw)
  simp [Function.update_of_ne hne]

/-- A zero column entry is what the dense pass computes: a row its constraint does not reach, with a finite
base value and a finite `inv`, gives `(r − r)·inv = 0`. -/
theorem quot_eq_zero_of_unreached (R : Residual α m nc) (P : Plan nc)
    (hcover : ∀ k, P.accepted k → R.reads k ⊆ P.idx k) (hacc : ∀ k, P.accepted k) (x : ℕ → α) (v : ℕ)
    (i : Fin m) (hi : R.own i ∉ P.consFor v) (hfin : O.fin (R.F x i)) (hinv : O.fin (O.inv (O.step (x v)))) :
    O.quot R x v i = O.zero := by
  unfold Ops.quot
  rw [row_indep R P hcover hacc x v _ i hi, row_indep R P hcover hacc x v _ i hi, O.sub_self _ hfin]
  exact O.zero_mul _ hinv

/-- The local Jacobian is the dense Jacobian whenever `jacobianLocalInto` does not refuse: each entry is the
same expression `(F(x+h) − F(x−h))·inv`, either evaluated (a reached row) or zero where the dense pass computes
`(r − r)·inv = 0`. Covers `sketch.Sketch.jacobianLocalInto` and `sketch.Sketch.jacobianInto`. Hypotheses carry
A43 (`Ops`), A44 (`hcover`) and A45 (`R`, `hbase`). -/
theorem jacobianLocal_eq_dense (R : Residual α m nc) (P : Plan nc)
    (hcover : ∀ k, P.accepted k → R.reads k ⊆ P.idx k) (x : ℕ → α) (free : Fin n → ℕ) (baseLen : ℕ)
    (base : Fin m → α) (hbase : ∀ i, base i = R.F x i) (evalOK : ℕ → Prop) {J : Fin m → Fin n → α}
    (hJ : O.jacobianLocal R P x free baseLen base evalOK = some J) : J = O.jacobianDense R x free := by
  have hg : O.Guards P x free baseLen base evalOK := by
    rw [← jacobianLocal_isSome_iff, hJ]; rfl
  unfold jacobianLocal at hJ
  simp only [hg, ite_true] at hJ
  obtain rfl := Option.some.inj hJ
  funext i j
  unfold jacobianDense
  by_cases hmem : R.own i ∈ P.consFor (free j)
  · simp [hmem]
  · simp only [hmem, ite_false]
    exact (O.quot_eq_zero_of_unreached R P hcover hg.2.2.1 x (free j) i hmem (by rw [← hbase]; exact hg.2.2.2.1 i)
      (hg.2.2.2.2.1 j)).symm

/-- For every input, `jacobianLocalInto` either refuses or returns the dense Jacobian. -/
theorem jacobianLocal_eq_none_or_dense (R : Residual α m nc) (P : Plan nc)
    (hcover : ∀ k, P.accepted k → R.reads k ⊆ P.idx k) (x : ℕ → α) (free : Fin n → ℕ) (baseLen : ℕ)
    (base : Fin m → α) (hbase : ∀ i, base i = R.F x i) (evalOK : ℕ → Prop) :
    O.jacobianLocal R P x free baseLen base evalOK = none ∨
      O.jacobianLocal R P x free baseLen base evalOK = some (O.jacobianDense R x free) := by
  cases h : O.jacobianLocal R P x free baseLen base evalOK with
  | none => exact Or.inl rfl
  | some J => exact Or.inr (by rw [jacobianLocal_eq_dense O R P hcover x free baseLen base hbase evalOK h])

end Ops

end Sketch.Jacobian

end
