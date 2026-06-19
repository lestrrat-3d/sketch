# Conic–conic tangency

Tangency between two conics that have no closed-form distance: an ellipse with a
circle, or two ellipses. (Circle↔circle stays the existing closed-form
`NewTangentCircles`; line↔conic stays `NewTangent`/`NewTangentEllipse`.)

## Why a contact-point witness

A circle and an ellipse, or two ellipses, have no closed-form point-to-curve
distance, so tangency is verified existentially with a **contact-point witness**
`P` — two auxiliary solver coordinates. For two regular implicit conics
`F_A = 0`, `F_B = 0` with non-vanishing gradients, the curves are first-order
(G1) tangent at `P` iff:

- `F_A(P) = 0` and `F_B(P) = 0` (P lies on both), and
- `∇F_A(P) ∥ ∇F_B(P)` (their tangent lines coincide), i.e. `cross(n̂_A, n̂_B) = 0`.

A transverse crossing cannot satisfy parallel normals at a shared point (its
tangent lines differ), so there is no false-bless from "normals happen to align"
— once both membership rows hold, parallel normals *is* tangency. This is local
G1 tangency: it does not forbid another intersection elsewhere, and it accepts
osculating / inflectional tangent crossings (those are still first-order
tangent). A "single kiss, no other crossing" requirement would be a separate
global constraint, not smuggled into this local one.

A pure distance-stationarity formulation is rejected: at zero separation the
stationarity of squared distance is vacuous, so a transverse intersection would
be falsely accepted unless the parallel-normal row is present anyway — at which
point the contact-point form is simpler and reuses the existing implicit
ellipse/circle residuals.

## Committed residual (4 rows)

Aux vars: the witness `px, py` and an internal/external branch slack `wSide`
(not serialized; re-seeded on load, like every aux-var constraint).

1. `membership_A(P)` — length: `sampsonEllipse` for an ellipse, `|P−C|−R` for a
   circle.
2. `membership_B(P)` — length: same per B's type.
3. `cross(n̂_A, n̂_B)` — dimensionless `sin` of the tangent angle, 0 when aligned.
   `n_A,n_B` are outward normals (`ellipseNormalXY`; `P−center` for a circle).
4. `σ·dot(n̂_A, n̂_B) − wSide²` — dimensionless, `σ = +1` internal / `−1` external.

**The branch row is load-bearing**, not a mere seed bias. Without it,
`NewTangent…(…, true)` and `…(…, false)` would be the *same* equation after
serialization / reload / a different solve basin — unacceptable for an oracle
that must report which tangency it verified. At an external contact the outward
normals are antiparallel (`dot = −1`); internal, parallel (`dot = +1`); the
slack inequality `σ·dot ≥ 0` enforces the chosen branch. It is a slack
inequality, **not** `dot − σ = 0`: the latter has a zero Jacobian at the
solution (`d cos θ/dθ = 0` at `θ = 0,π`), so the rank probe would read it as a
dependent row; `−w²` keeps a real first-order row via `∂/∂w = −2w` with `w = 1`.

Degenerate conics have no tangent direction; the residual keeps the membership
rows but forces the parallel row to a clearly-nonzero `1`, so they are never
blessed (arity stays 4). The thresholds are sign-independent (`abs`) and, for the
ellipse, **must match the residual floor**: `sampsonEllipse` and `ellipseNormalXY`
floor the axis squares at `1e-12`, so an ellipse semi-axis below `√1e-12 = 1e-6`
(`ellipseAxisEps`) would silently solve against a floored surrogate rather than
the authored ellipse — those are rejected as degenerate. A circle's radius is not
floored in its membership `|P−C|−R`, so its degeneracy floor is `conicEps = 1e-9`.

## DOF

Two free ellipses: 10 conic vars + 3 aux (`px,py,wSide`) − 4 rows = 9, so
tangency removes exactly one DOF. The two membership rows pin the witness onto
both curves; the parallel row supplies the contact-location condition; the side
row is independent through `wSide`.

## Seeding

Branch-aware boundary sampling: sample both conic boundaries (96 points each),
score each pair by proximity² + cross² with a large penalty for the wrong
internal/external side, and seed `P` at the midpoint of the best pair (balancing
the two membership residuals). `wSide = slackFor(σ·dot(n̂_A,n̂_B))` at the seed.
The solver refines from there; `ProbeConfigurations` can surface alternate
tangency configurations after a baseline solve.

## API

Typed constructors over the **sealed `Circular`/`Elliptical` interfaces**, one
private `tangentConics` implementation reached through a small `conic` adapter
(`onResidual`/`normalAt`/`degenerate`/`boundary`/`sweepExcess`), dispatched by
`conicOf`:

- `NewTangentEllipseCircular(e Elliptical, c Circular, internal bool) Constraint`
  — `e` is an `*Ellipse` or `*EllipticalArc`, `c` a `*Circle` or `*Arc`.
- `NewTangentEllipses(e1, e2 Elliptical, internal bool) Constraint`

Circle↔circle is deliberately *not* routed here (it keeps its closed-form
`NewTangentCircles`). JSON uses two type strings (`tangent_ellipse_circle`,
`tangent_ellipses`) carrying the two entity ids and the `internal` flag; aux
vars are not serialized. `CheckConstraint` already probes aux-var constraints in
committed form (temporary allocation), so no special-casing is needed.

## Arc operands (sweep confinement)

An arc operand (`*Arc` / `*EllipticalArc`) lies on its underlying circle/ellipse
(the same membership and normal as the full conic), plus a per-arc slack-encoded
**sweep row** `sweepExcess(P) − w² = 0` (`arcInSweepExcess` for a circular arc on
the contact direction `P−center`; `ellipticalArcSweepExcess` on the eccentric
direction for an elliptical arc) confining the witness contact to the swept
portion — so a tangent to the underlying full conic *off* the arc is reported
unsolvable. The slack adds one var and one row per arc operand, net DOF
unchanged; `boundary()` for an arc samples only its swept extent so seeding stays
in-sweep. Committed arity stays constant (the sweep rows are gated on the slack,
which is always allocated for an arc operand).

## Recorded follow-ups

- **Shared-endpoint branch**: when two arc operands share the exact endpoint
  `*Point`, enforce tangency *at that endpoint* (parallel + side rows there, no
  free witness, no membership/sweep rows — endpoints are already on their curves
  via the internal `arcRadius` / `ellipticalArcOn` constraints), mirroring
  `tangentLineEllipse`'s shared-endpoint case. Until then the free witness may
  pick a different (valid, in-sweep) tangency than the shared endpoint.
- **Same-entity / exact-overlap policy**: identical conics are tangent
  everywhere (rank-degenerate). Passing the same concrete entity (or an exact
  duplicate) as both operands is an unsupported precondition; an explicit reject
  / duplicate-overlap detector is a recorded follow-up.
