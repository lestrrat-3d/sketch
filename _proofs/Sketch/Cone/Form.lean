module

public import Mathlib

/-!
# Cone implicit function

Models `sketch.coneSideForm` and `sketch.newConeSideForm` (`cone_side.go`), the exact rational form
`CertifyConeCurveSide` evaluates. A plane point `(x, y)` maps to the world offset `q = base + x·u + y·v` from
the cone apex, with `base` the plane origin minus the apex and `u`, `v` the plane axes. With `a` the cone
axis, the cone half coordinate is `h = q·a` and the implicit function is `F = |q×a|² − slope²·h²`.

`ConeSide` mirrors the Go `ConeSide` values, and `ConeSideForm.SideAt` states what a certified side means at
one point: the point lies in the positive cone half (`0 < h`) and `F ≥ m` outside or `F ≤ −m` inside.
`worldF_eq` relates `F` to the distance from the axis, and `newConeSideForm_F` shows the plane form evaluates
the world cone function at the world point.
-/

public section

namespace Sketch.Cone

/-- Model of `sketch.ConeSide`: `zero` is the Go zero value, meaning no side. -/
inductive ConeSide where
  | zero
  | outside
  | inside
  deriving DecidableEq

/-- Model of `sketch.coneSideForm`. Each Go `[3]*big.Rat` is a function `Fin 3 → ℚ` (A30, A31). -/
structure ConeSideForm where
  base : Fin 3 → ℚ
  u : Fin 3 → ℚ
  v : Fin 3 → ℚ
  axis : Fin 3 → ℚ
  slope2 : ℚ

namespace ConeSideForm

/-- World offset from the apex, component `i`, of plane point `(x, y)`. -/
@[expose] def q (f : ConeSideForm) (x y : ℚ) (i : Fin 3) : ℚ := f.base i + x * f.u i + y * f.v i

/-- Cone half coordinate `q·a`; positive on the growing-radius half. -/
@[expose] def h (f : ConeSideForm) (x y : ℚ) : ℚ :=
  f.q x y 0 * f.axis 0 + f.q x y 1 * f.axis 1 + f.q x y 2 * f.axis 2

/-- First component of `q×a`. -/
@[expose] def cx (f : ConeSideForm) (x y : ℚ) : ℚ := f.q x y 1 * f.axis 2 - f.q x y 2 * f.axis 1

/-- Second component of `q×a`. -/
@[expose] def cy (f : ConeSideForm) (x y : ℚ) : ℚ := f.q x y 2 * f.axis 0 - f.q x y 0 * f.axis 2

/-- Third component of `q×a`. -/
@[expose] def cz (f : ConeSideForm) (x y : ℚ) : ℚ := f.q x y 0 * f.axis 1 - f.q x y 1 * f.axis 0

/-- The cone implicit function `|q×a|² − slope²·h²` at plane point `(x, y)`. -/
@[expose] def F (f : ConeSideForm) (x y : ℚ) : ℚ :=
  f.cx x y * f.cx x y + f.cy x y * f.cy x y + f.cz x y * f.cz x y - f.h x y * f.h x y * f.slope2

/-- Side `side` holds with margin `m` at plane point `(x, y)`: the point is in the positive cone half, and
`F ≥ m` for `outside` or `F ≤ −m` for `inside`. The `zero` side holds nowhere. -/
@[expose] def SideAt (f : ConeSideForm) (side : ConeSide) (m x y : ℚ) : Prop :=
  0 < f.h x y ∧
    match side with
    | .outside => m ≤ f.F x y
    | .inside => f.F x y ≤ -m
    | .zero => False

/-- A smaller margin keeps a side. -/
theorem sideAt_mono {f : ConeSideForm} {side : ConeSide} {m m' x y : ℚ} (hm : m' ≤ m)
    (hs : f.SideAt side m x y) : f.SideAt side m' x y := by
  obtain ⟨hh, hs⟩ := hs
  refine ⟨hh, ?_⟩
  cases side with
  | zero => exact hs
  | outside => exact le_trans hm hs
  | inside => simp only at hs ⊢; linarith

end ConeSideForm

/-- Dot product of two world vectors. -/
@[expose] def dot (a b : Fin 3 → ℚ) : ℚ := a 0 * b 0 + a 1 * b 1 + a 2 * b 2

/-- The cone implicit function at world point `P` for apex `A`, axis `a` and slope `s`:
`|(P−A)×a|² − s²·((P−A)·a)²`. -/
@[expose] def worldF (A a : Fin 3 → ℚ) (s : ℚ) (P : Fin 3 → ℚ) : ℚ :=
  let d : Fin 3 → ℚ := fun i => P i - A i
  (d 1 * a 2 - d 2 * a 1) ^ 2 + (d 2 * a 0 - d 0 * a 2) ^ 2 + (d 0 * a 1 - d 1 * a 0) ^ 2 -
    s ^ 2 * dot d a ^ 2

/-- The cone half coordinate `(P−A)·a` at world point `P`. -/
@[expose] def worldH (A a : Fin 3 → ℚ) (P : Fin 3 → ℚ) : ℚ := dot (fun i => P i - A i) a

/-- With `d = P − A`, `F = |a|²·|d|² − (1 + s²)·(d·a)²`. Dividing by `|a|²`, `F` is the squared distance of `P`
from the axis minus `s²` times its squared distance along the axis, scaled by `|a|²`: positive outside the
cone surface, zero on it, negative inside. -/
theorem worldF_eq (A a : Fin 3 → ℚ) (s : ℚ) (P : Fin 3 → ℚ) :
    worldF A a s P = dot a a * dot (fun i => P i - A i) (fun i => P i - A i) -
      (1 + s ^ 2) * worldH A a P ^ 2 := by
  simp only [worldF, worldH, dot]
  ring

/-- Model of `sketch.newConeSideForm`: the frame origin `o`, plane axes `U`, `V`, and the cone apex `A`, axis
`a` and slope `s` are the exact values of the Go floats (A31). -/
@[expose] def newConeSideForm (o U V A a : Fin 3 → ℚ) (s : ℚ) : ConeSideForm where
  base := fun i => o i - A i
  u := U
  v := V
  axis := a
  slope2 := s * s

/-- The plane form evaluates the world cone function at the world point `o + x·U + y·V`. -/
theorem newConeSideForm_F (o U V A a : Fin 3 → ℚ) (s x y : ℚ) :
    (newConeSideForm o U V A a s).F x y = worldF A a s (fun i => o i + x * U i + y * V i) := by
  simp only [ConeSideForm.F, ConeSideForm.cx, ConeSideForm.cy, ConeSideForm.cz, ConeSideForm.h,
    ConeSideForm.q, newConeSideForm, worldF, dot]
  ring

/-- The plane form's half coordinate is the world half coordinate at the world point `o + x·U + y·V`. -/
theorem newConeSideForm_h (o U V A a : Fin 3 → ℚ) (s x y : ℚ) :
    (newConeSideForm o U V A a s).h x y = worldH A a (fun i => o i + x * U i + y * V i) := by
  simp only [ConeSideForm.h, ConeSideForm.q, newConeSideForm, worldH, dot]
  ring

end Sketch.Cone

end
