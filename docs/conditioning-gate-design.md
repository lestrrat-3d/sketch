# Scale-Invariant Conditioning Gate — Design

Status: **implemented** (`conditioning.go`; tests in `conditioning_test.go` and
`rankmargin_test.go`). Resolves the "scale-invariant conditioning gate" open
item under "Solver evolution" in `CLAUDE.md`.

## The problem

A sketch can be structurally clean — solvable, DOF 0, no redundant or conflicting
constraints — yet **numerically near-singular**: its constraints are nearly (but
not exactly) linearly dependent, so the DOF-0 verdict rests on a knife edge. The
canonical case is a point pinned by two lines that are ≈parallel: the two
`point-on-line` rows are almost the same equation, the intersection is
ill-defined, and a tiny perturbation flips the geometry far. An oracle must not
bless such a sketch.

The existing **`RankMargin`** sees this — it reports how close the rank-deciding
pivot sits to the hard `rankEps = 1e-9` threshold — but it **cannot gate trust**.
The Jacobian's pivots are *raw*: its rows mix physical units (length residuals in
mm vs dimensionless sin/cos residuals) and its columns mix units (length
coordinates/radii vs radian angles vs dimensionless slacks). So a pivot magnitude
— and its distance to `1e-9` — changes when the same construction is drawn at a
different scale or in different units. `1e-9` is not a unit-invariant cutoff;
thresholding it as pass/fail is unsound (a prior attempt produced concrete
scale-dependent counterexamples). `RankMargin` stays advisory.

## The measure

Build a physically **nondimensional** Jacobian and threshold its reciprocal
condition number — a quantity that is identical for similar sketches at any scale
or unit.

    A = Drow · J · Dcol
    Conditioning = σ_min(A) / σ_max(A)

with `L` the bounding-box diagonal of the geometry (floored to 1):

- **Column scale `Dcol`** — `L` for length-kind variables (point coordinates,
  circle radii / ellipse semi-axes, and the conic-tangency contact-witness
  coordinates), `1` for dimensionless variables (ellipse rotation, every slack /
  spline-parameter aux).
- **Row scale `Drow`** — `1/L` for length-kind residual rows, `1` for
  dimensionless rows.

Every entry of `A` is then dimensionless: a length-row × length-col entry picks
up `L·(1/L) = 1`; a dimensionless-row × length-col entry is already scale-free;
etc. Under a uniform rescale of the geometry by `k`, `J` and `L` both transform so
that `A` is unchanged, so `Conditioning` is **scale- and unit-invariant**. It is
also **translation-invariant**: every residual is a function of coordinate
*differences*, so the finite-difference pass first re-centres the geometry about
its centroid (restoring the exact original values afterward — `Verify` never
mutates) to keep the scale-relative FD step `condFDStep·L` from vanishing into
floating-point cancellation for geometry placed far from the origin.

`Conditioning` is the smallest singular value relative to the largest. A healthy
fully-constrained sketch sits at `O(0.01)`–`O(1)`; a near-singular one is small in
proportion to the dependency (e.g. two lines `δ` rad apart give `≈ δ/2`). The gate
fires below a **tolerance-derived** threshold:

    Trustworthy() requires Conditioning >= conditioningGate(tolerance)
    conditioningGate(tol) = max(condTrustBase, condSlackFactor·√tol)
                          = max(1e-6, 4·√tol)

The `1e-6` base clears the central-difference derivative noise floor (~`1e-9`–`1e-8`
at `condFDStep = 1e-7`). The `4·√tol` term handles **slack-encoded inequalities at
their active boundary** (see below): such a slack only resolves to `≈√tol`, so its
column norm `2w ≤ 2·√tol` upper-bounds `σ_min`; the gate must sit above that floor
or a slack flat-spot slips through. At the default tolerance `1e-10` the effective
gate is `4e-5`. A looser `Verify(WithTolerance(…))` raises the gate accordingly, so
trust is never granted on a tolerance too coarse to resolve the singularity — the
threshold and the solve tolerance are kept consistent rather than independent.

### Why physical-kind scaling, not equilibration

Column-equilibration (scaling each column to unit norm, à la van der Sluis) is
automatic and would handle the per-variable unit problem — but it is **unsound**
here for two reasons. (1) It leaves the *row* unit-mixing untouched (whole-sketch
scaling acts on rows too, a left diagonal that does not cancel in singular-value
ratios). (2) It would *hide* a genuine near-rank-loss in a slack/aux column: a
slack row like `t − w²` has derivative `−2w`, which → 0 as the slack approaches
its boundary; equilibration would renormalize that vanishing column back up to
unit norm and mask the singularity. Fixed *physical-kind* scaling (a slack column
keeps scale `1`, so its `−2w` stays small) preserves the detection. The same
argument rules out row-equilibration (it would mask a near-zero, i.e. ineffective,
constraint row).

### Why σ via one-sided Jacobi SVD, not AᵀA

The gate decides around `σ ≈ 1e-6`. Forming the Gram matrix `AᵀA` squares the
condition number, moving the decision to `≈ 1e-12` — too close to double-precision
noise for an oracle. A one-sided Jacobi SVD on `A` directly computes small
singular values to high relative accuracy with no external dependency.

## Applicability and gating

`Conditioning` is meaningful only for a **DOF-0 candidate**. An under-constrained
sketch (DOF > 0) is *genuinely* singular by its free degrees of freedom — a
separate, already-reported verdict — so `Verify` leaves `Conditioning = +Inf`
(not applicable) there rather than reporting a misleading 0. The gate therefore
never double-counts under-constraint, redundancy, or conflicts (each already gates
`Trustworthy` on its own); it adds exactly the "barely full rank" case the
structural checks call fully constrained.

`RankMargin` is kept, unchanged, as the raw advisory signal; `Conditioning` is the
unit-invariant gating one.

## Row- and column-kind classification

- **Variable kinds.** Point coordinates and `varKinds`' radius/semi-axis vars are
  length (scale `L`); ellipse rotation is dimensionless. Aux variables default to
  dimensionless — correct for every slack and curve-parameter aux. The **only**
  length-kind aux variables are the conic-tangency contact-witness coordinates
  (`tangentConics.px/py`, literal positions), tagged explicitly.
- **Row kinds.** A centralized `condRowKinds` switch classifies each residual row
  a constraint contributes, mirroring its `residual()` row structure exactly —
  including the aux-allocation-gated rows (a committed constraint has its sweep /
  box / branch slacks allocated, so those dimensionless rows are present) and the
  driven-dimension skip. It is centralized (not a method per constraint) so the
  whole table is reviewable in one place. **Drift guard:** if the row-kind table
  does not align with the residual rows (a constraint kind missing from
  `condRowKinds`), `Conditioning` is `NaN`, which fails the trust gate — an
  unclassified constraint reads *untrustworthy* (the safe direction), never a
  false pass. Adding a constraint therefore requires a `condRowKinds` case (noted
  in the `CLAUDE.md` new-constraint checklist).

A length residual carries length units (a distance gap, a coordinate difference, a
signed perpendicular distance, a Sampson `|F|/|∇F|` membership, `R·θ`); a
dimensionless residual is a sin/cos/normalized-dot, or a slack-box / branch /
sweep equation. Constraints can mix both within one `residual()` (e.g. conic
tangency: length membership rows + dimensionless normal/branch/sweep rows), which
is why per-row (not per-constraint) classification is required.

## Tests

- **Scale/unit/translation invariance** (the headline): a healthy fixture and a
  mixed length/dimensionless fixture give `Conditioning` identical to 1e-9 across
  1×, 1000×, 25.4× (inch) and large translations, while raw `RankMargin`
  demonstrably moves — the property that disqualifies `RankMargin` from gating.
- **Gating**: a point pinned by two lines `δ` rad apart passes at `δ = 1e-2`,
  fails at `δ = 1e-7`; `Conditioning ≈ δ/2` and the verdict is scale-invariant.
- **Not-applicable**: an under-constrained sketch reports `Conditioning = +Inf`.
- **Tolerance-derived gate**: a slack-free near-parallel pin with a fixed
  `Conditioning = 1e-4` is trustworthy at a tight tolerance (gate `4e-6`) and gated
  out at a loose one (gate `4e-4`) — a constant threshold could not do both.
- **Slack flat-spot**: a point-on-arc with the contact driven to the sweep boundary
  (slack `w → 0`) is never trustworthy, at the default tolerance or a looser one.
- **Aux-constraint classification**: DOF-0 sketches exercising the slack-gated and
  unwrapped-sweep aux rows yield a finite `Conditioning` (never the NaN
  classification-gap sentinel).
