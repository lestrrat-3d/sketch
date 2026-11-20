module

public import Mathlib

/-!
# Pipeline smoke test

Holds one trivial theorem so the Lake build, the `mk_all` import check and the axiom audit have a
declaration to check before the first real proof lands. It models no Go function. Delete it when a
real theorem exists under `Sketch/`.
-/

public section

namespace Sketch.Smoke

/-- The squared distance between two points of the plane is symmetric. -/
theorem sqDist_symm (x₁ y₁ x₂ y₂ : ℝ) :
    (x₁ - x₂) ^ 2 + (y₁ - y₂) ^ 2 = (x₂ - x₁) ^ 2 + (y₂ - y₁) ^ 2 := by
  ring

end Sketch.Smoke

end
