# Constraint Diagnostics — Design

Status: **implemented** (`diagnose.go`; tests in `diagnose_test.go`).
Resolves most of the "constraint diagnostics & UX" open question in CLAUDE.md:
conflicting vs. redundant classification, pre-commit over-constraint
rejection, and free-DOF attribution. Driven by §4 of
`docs/acceptance-tests.md` (Fusion refuses an over-constraining gesture and
colors under-constrained geometry blue; the engine must supply both answers).

## The three questions a sketcher UI asks

1. *This sketch won't solve / reports redundancy — which constraint is wrong?*
   → `Sketch.Diagnose()`
2. *The user is about to add a constraint — should the gesture be refused?*
   → `Sketch.CheckConstraint(c)`
3. *Which geometry should be drawn as still-movable?*
   → `Sketch.FreePoints()` / `Point.IsFullyConstrained()`

## Diagnose: partition dependents by violation

`RedundantConstraints()` already identifies constraints contributing linearly
dependent equations (creation order: the later of two duplicates is blamed),
but lumps consistent duplicates with contradictions. `Diagnose()` refines it:

- For each dependent constraint, evaluate its own residual at the current
  configuration. Max |residual| ≤ `conflictTol` (1e-8) → **Redundant**;
  above → **Conflicting**.
- Rationale: residuals are unit-normalized and a converged solve leaves them
  ≤ 1e-10, so 1e-8 cleanly separates "holds, just duplicated" from "the
  solver could not satisfy this". After a *failed* solve the LM minimum
  leaves the fighting constraints sharing the violation — the dependent
  (later-added) one carries a residual far above tolerance and is reported.
- The independent member of a conflict is deliberately *not* reported,
  mirroring the `RedundantConstraints` creation-order convention: earlier
  constraints win, later ones are the removal candidates.
- Call after `Solve` (converged or not); like `DOF`, the analysis is local to
  the call-time configuration.

Rejected alternative: classifying by *consistency of the linear system*
(comparing projected residuals during Gram–Schmidt). More machinery for the
same answer at this scale; the residual test reuses the invariant that
converged ⇒ residual ≈ 0.

## CheckConstraint: rank probe without commitment

`CheckConstraint(c)` ranks the hard-constraint Jacobian with and without `c`'s
rows appended (`rankOf`, the evaluator-generalized `rank`). If the augmented
rank gains fewer rows than `c` contributes, some equation is dependent —
committing `c` would create redundancy or conflict — and the call returns an
error wrapping `ErrOverconstrained`. Nothing is mutated; the UI refuses the
gesture and the sketch is untouched (Fusion's behavior).

- A consistent duplicate and a contradiction are *both* rejected: neither
  adds an equation the sketch can use. `Diagnose` is the tool for telling
  them apart after the fact.
- A constraint touching no free variable (between grounded points) ranks as
  fully dependent → rejected.
- Driven dimensions contribute no equations → always accepted.
- Caveat: rank is evaluated at the current configuration, so check against
  solved geometry for the most reliable verdict (same caveat as `DOF`).

## FreePoints: null-space support

A point can still move iff some first-order constraint-preserving motion
displaces it: the union of supports of a null-space basis of the Jacobian.
`movableVars` reduces J to reduced row-echelon form; each non-pivot column
seeds a null vector with support on itself and on every pivot column its
elimination touches. `FreePoints` maps movable variables back to points (id
order); `Point.IsFullyConstrained` is the per-point view. Grounded points are
never free (their variables are excluded from `freeVars`).

This is first-order analysis: a point at a singular configuration (e.g. the
apex of two just-tangent circles) may be reported free though finite motion is
blocked. Acceptable — Fusion's blue/black coloring has the same character.

## Still open (recorded in CLAUDE.md)

- Reporting the *full* conflict set (the earlier constraints a conflicting one
  fights), not just the later-added member.
- Per-entity constrained status (derive from point/var support — needs a
  decision on how entity-owned vars like radius map to "the entity can move").
- `AddConstraint` option to auto-reject (today the caller composes
  `CheckConstraint` + `AddConstraint`).
