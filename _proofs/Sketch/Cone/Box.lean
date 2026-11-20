module

public import Sketch.Cone.Form
public import Sketch.Cone.Interval

/-!
# Cone side of a box

Models `sketch.coneSideForm.boxSide` (`cone_side.go`). It evaluates the cone form over the axis-aligned
plane box `u × v` with the exact interval operations of `Sketch.Cone.Interval`, refuses unless the half
coordinate's lower bound is positive, and returns `outside` with margin `residual.lo` or `inside` with margin
`−residual.hi` when the residual interval excludes zero.

`boxSide_sound` proves that a returned side holds, with the returned positive margin, at every point of the
box.
-/

public section

namespace Sketch.Cone

namespace ConeSideForm

/-- Interval of `q i` over the box, as `boxSide` builds it: `{base, base} + u·u_i + v·v_i`. -/
@[expose] def boxQ (f : ConeSideForm) (u v : ConeInterval) (i : Fin 3) : ConeInterval :=
  ((⟨f.base i, f.base i⟩ : ConeInterval).add (u.scale (f.u i))).add (v.scale (f.v i))

/-- Interval of the half coordinate `h` over the box. -/
@[expose] def boxH (f : ConeSideForm) (u v : ConeInterval) : ConeInterval :=
  (((f.boxQ u v 0).scale (f.axis 0)).add ((f.boxQ u v 1).scale (f.axis 1))).add
    ((f.boxQ u v 2).scale (f.axis 2))

/-- Interval of `F` over the box: `cx² + cy² + cz² − h²·slope²`. -/
@[expose] def boxResidual (f : ConeSideForm) (u v : ConeInterval) : ConeInterval :=
  let q := f.boxQ u v
  let cx := ((q 1).scale (f.axis 2)).sub ((q 2).scale (f.axis 1))
  let cy := ((q 2).scale (f.axis 0)).sub ((q 0).scale (f.axis 2))
  let cz := ((q 0).scale (f.axis 1)).sub ((q 1).scale (f.axis 0))
  ((cx.square.add cy.square).add cz.square).sub ((f.boxH u v).square.scale f.slope2)

/-- Model of `sketch.coneSideForm.boxSide`. The Go code builds the residual only after the half check; the
model names it up front, which changes nothing since the computation is pure. -/
@[expose] def boxSide (f : ConeSideForm) (u v : ConeInterval) : ConeSide × Option ℚ :=
  if (f.boxH u v).lo ≤ 0 then (.zero, none)
  else if 0 < (f.boxResidual u v).lo then (.outside, some (f.boxResidual u v).lo)
  else if (f.boxResidual u v).hi < 0 then (.inside, some (-(f.boxResidual u v).hi))
  else (.zero, none)

theorem mem_boxQ {f : ConeSideForm} {u v : ConeInterval} {x y : ℚ} (hx : u.Mem x) (hy : v.Mem y)
    (i : Fin 3) : (f.boxQ u v i).Mem (f.q x y i) :=
  ConeInterval.mem_add (ConeInterval.mem_add (ConeInterval.mem_point _) (ConeInterval.mem_scale _ hx))
    (ConeInterval.mem_scale _ hy)

theorem mem_boxH {f : ConeSideForm} {u v : ConeInterval} {x y : ℚ} (hx : u.Mem x) (hy : v.Mem y) :
    (f.boxH u v).Mem (f.h x y) :=
  ConeInterval.mem_add (ConeInterval.mem_add (ConeInterval.mem_scale _ (mem_boxQ hx hy 0))
    (ConeInterval.mem_scale _ (mem_boxQ hx hy 1))) (ConeInterval.mem_scale _ (mem_boxQ hx hy 2))

theorem mem_boxResidual {f : ConeSideForm} {u v : ConeInterval} {x y : ℚ} (hx : u.Mem x)
    (hy : v.Mem y) : (f.boxResidual u v).Mem (f.F x y) := by
  have q := mem_boxQ (f := f) hx hy
  have hcx := ConeInterval.mem_square
    (ConeInterval.mem_sub (ConeInterval.mem_scale (f.axis 2) (q 1)) (ConeInterval.mem_scale (f.axis 1) (q 2)))
  have hcy := ConeInterval.mem_square
    (ConeInterval.mem_sub (ConeInterval.mem_scale (f.axis 0) (q 2)) (ConeInterval.mem_scale (f.axis 2) (q 0)))
  have hcz := ConeInterval.mem_square
    (ConeInterval.mem_sub (ConeInterval.mem_scale (f.axis 1) (q 0)) (ConeInterval.mem_scale (f.axis 0) (q 1)))
  have hh := ConeInterval.mem_scale f.slope2 (ConeInterval.mem_square (mem_boxH (f := f) hx hy))
  exact ConeInterval.mem_sub (ConeInterval.mem_add (ConeInterval.mem_add hcx hcy) hcz) hh

/-- A side `boxSide` returns holds at every point of the box `u × v`, with the returned margin, which is
positive: the point is in the positive cone half, and `F ≥ m` (`outside`) or `F ≤ −m` (`inside`). Covers
`sketch.coneSideForm.boxSide` and the `coneInterval` operations; exact rational arithmetic is A30. -/
theorem boxSide_sound {f : ConeSideForm} {u v : ConeInterval} {side : ConeSide} {m : ℚ}
    (hres : f.boxSide u v = (side, some m)) :
    0 < m ∧ side ≠ .zero ∧ ∀ x y, u.Mem x → v.Mem y → f.SideAt side m x y := by
  unfold boxSide at hres
  split_ifs at hres with h0 h1 h2
  · simp at hres
  · simp only [Prod.mk.injEq, Option.some.injEq] at hres
    obtain ⟨rfl, rfl⟩ := hres
    refine ⟨h1, by decide, fun x y hx hy => ⟨?_, ?_⟩⟩
    · linarith [(mem_boxH (f := f) hx hy).1]
    · exact (mem_boxResidual (f := f) hx hy).1
  · simp only [Prod.mk.injEq, Option.some.injEq] at hres
    obtain ⟨rfl, rfl⟩ := hres
    refine ⟨by linarith, by decide, fun x y hx hy => ⟨?_, ?_⟩⟩
    · linarith [(mem_boxH (f := f) hx hy).1]
    · change f.F x y ≤ - -(f.boxResidual u v).hi
      rw [neg_neg]
      exact (mem_boxResidual (f := f) hx hy).2
  · simp at hres

end ConeSideForm

end Sketch.Cone

end
