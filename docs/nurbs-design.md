# NURBS curve — a Curve-parity geometry primitive

## Context

The Tier-2 **Curve parity** row (`docs/verification-roadmap.md`) lists
*conic/NURBS import representation* as the remaining curve gap. The **conic** half
shipped (`docs/conic-design.md`). This increment ships the **NURBS** half: a
general **non-uniform rational B-spline** of arbitrary degree, so the engine can
represent and verify a sketch containing a real CAD NURBS curve (a freeform
spline, a degree-elevated edge, a rational arc).

It mirrors how the existing `Spline`/`ClosedSpline`/`FitSpline` and the `Conic`
shipped: an authorable, profile-participating, serializable, exportable **open
curve**. Constraints *on* a NURBS (point-on, tangency) and editing tools (knot
insertion) are explicit follow-ups — this ships the entity.

**It does NOT replace the existing `Spline`** (clamped uniform cubic, non-
rational). NURBS is the general primitive; the uniform cubic spline stays as the
ergonomic common case. A future cleanup *could* re-express `Spline` on the NURBS
core, but that is out of scope here.

## Representation

A NURBS of degree `p`:

```
        Σ N_{i,p}(t)·w_i·P_i
C(t) = ──────────────────────── ,   over a clamped knot vector U
          Σ N_{i,p}(t)·w_i
```

- `P_i` — control points (n+1 of them): ordinary **sketch points** (the durable
  solver handles, exactly like `Spline`).
- `w_i` — per-control weights (> 0). `w_i ≡ 1` ⇒ non-rational (a plain B-spline).
- `N_{i,p}` — degree-`p` B-spline basis over the knot vector `U` (length
  `m+1 = n+p+2`), **clamped** (first and last `p+1` knots equal) so the curve is
  an OPEN curve with endpoints `C(t_first)=P_0`, `C(t_last)=P_n`.

### Sketch entity

```go
type NURBS struct {
    s            *Sketch
    Control      []*Point
    degree       int
    knots        []float64
    weights      []float64
    id           int
    construction bool
    refState
}
```

`CreateNURBS(degree int, control []*Point, weights, knots []float64) (*NURBS, error)`:

- Validates: `degree ≥ 1`; `len(control) ≥ degree+1`; `len(knots) =
  len(control)+degree+1`; knots non-decreasing and **clamped**; `len(weights) =
  len(control)` with every `w_i > 0` (or `weights == nil` ⇒ all 1); no nil point.
  Any violation → `ErrInvalidShape` (no panic).
- A convenience `ClampedUniformKnots(n, degree)` helper generates the common
  knot vector so callers rarely hand-write one.

**Knots and weights are stored STRUCTURAL data, not solver vars** — only the
control points are solver handles (mirrors `Spline`, which adds no vars). DOF of a
free NURBS = `2·(n+1)`. (Promoting weights to dimensionable vars is a follow-up,
like a rho dimension for the conic; rarely needed and would bloat the var
vector.) Accessors: `Degree()`, `Knots()` (copy), `Weights()` (copy), `Control`,
`Geometry()`, `Rational()` (any weight ≠ 1).

### Transient geometry (`geom`)

`geom.NURBS{Degree int; Control []*Point; Knots, Weights []float64}` plus the
standard B-spline kernels (The NURBS Book algorithms), **general degree**, new
and self-contained (the existing `EvalCubicBSpline` is uniform-cubic-specific and
untouched):

- `findSpan(n, p, u, U)` — knot-span binary search (A2.1).
- `basisFuns(span, u, p, U)` — the `p+1` nonzero basis values (A2.2).
- `dersBasisFuns(span, u, p, U)` — basis + first derivatives via
  `N'_{g,p} = p·(N_{g,p-1}/(U[g+p]−U[g]) − N_{g+1,p-1}/(U[g+p+1]−U[g+1]))`.
- `NURBS.Eval(t)` — rational point via homogeneous `(Σ N·w·P, Σ N·w)` then divide.
- `NURBS.EvalDeriv(t)` — `(X'W − XW')/W²` (and Y), from `dersBasisFuns`.
- `Polyline(n)`, `Endpoints()` → first/last control point (open `Curve`).

**Verified numerically (scratch, now removed):** a rational quadratic NURBS with
knots `{0,0,0,1,1,1}`, control `(1,0),(1,1),(0,1)`, weights `1,1/√2,1` traces an
**exact** quarter circle (max `|r−1| = 2e-16`); the derivative passes the moment
cross-check (below).

## Area: two tiers, both sampling-independent

A curved fragment's bulge (signed area between the sub-arc and its sub-chord) is
the conic/spline analog: the moment `½∫(x·y′ − y·x′) dt` over the fragment minus
`triangle(start, P(t0), P(t1))` (the same arc-vs-subchord correction proven for
the conic). The moment is integrated **per knot span** (the curve is only
piecewise rational), via Gauss–Legendre:

- **Non-rational** (`w_i ≡ 1`): the integrand is a polynomial of degree `2p−1` on
  each span, so **`p`-point Gauss is exact**. (At `p=3` this is the existing
  cubic `splineBulge`'s 3-point rule.) Verified exact to 2e-16 for a cubic.
- **Rational**: the integrand is a smooth rational function; a fixed high-order
  rule (10-point) with **adaptive subdivision to a tolerance** (~1e-12) gives a
  *numerically* exact, **sampling-independent** area (it integrates the TRUE
  curve, not the arrangement polyline). Verified: the quarter-circle's area =
  `π/4` to 7e-12.

This is honest about the engine's "exact area" invariant: a non-rational NURBS
(and any whole NURBS split only at an analytic line/circle/arc crossing) is
**exact**; a rational NURBS is **numerically exact** (tolerance ~1e-12); and a
NURBS **split at a sampled line/NURBS or curve/curve crossing** has an
approximate cut parameter, so its area **converges** with sampling — exactly like
the ellipse/spline/conic families (line/NURBS intersections are not analytic).
Never a false bless: the topology is correct, the area convergent.

## Arrangement & soundness (`geom/arrange.go`)

A new `srcNURBS` source kind, mirroring `srcSpline` (an **open** curve that **can
self-cross** at high degree): `source` carries degree/knots/weights/control;
`at(t)` evaluates via the kernel; the open-curves builder assigns the kind;
`makeCycle`'s area case adds `nurbsBulgeSpan`. Crossing detection stays **sampled**
(no analytic line/NURBS intersection), sound like splines. **Self-intersection:
NURBS IS in the spline-family self-crossing self-test** (a high-degree curve can
loop — unlike the convex-hull-bounded conic/arc), so a self-crossing NURBS is
flagged `SelfIntersecting`.

## The integration sites (mirrors the conic/spline precedent)

- **geom**: `NURBS` type + kernels (`nurbs.go` new file) + `Eval`/`EvalDeriv`/
  `Polyline`/`Endpoints`; arrange.go `srcNURBS` + `at` + kind-assignment + area
  case + `nurbsBulgeSpan` + the self-crossing self-test entry.
- **sketch.go**: `NURBS` entity + `CreateNURBS` + `ClampedUniformKnots` + accessors;
  `localPolyline`, `entityPoints` (all control points), `entitySizeVars` (none —
  no shape vars).
- **json.go**: marshal/rebuild `"nurbs"` (control ids + degree + knots + weights).
- **Exporters**: svg.go/png.go sampled draw + bounds; **dxf.go** a **native
  `SPLINE`** — degree (71), knots (40×, count 72), control points (10/20/30,
  count 73), weights (41×) + rational flag (70 bit 4) when rational; world-space
  via the existing `putWCS` path.
- **profiles.go**: open curve into `curves`.
- **removal.go**: `renumberEntity`, `entityUsesPoint` (any control point),
  `RemoveEntity` (no vars to retire — control points are shared sketch points,
  kept like a line's).
- **tools.go**: no `varKind` entry (no shape vars).
- **reference.go**: `isNilEntity`, `entityPoints`.
- **conditioning.go / probe.go**: no new var kind (no shape vars), untouched.

## Deferred (explicit follow-ups)

Point-on-NURBS and line/NURBS & NURBS/NURBS tangency (analytic foot-point /
contact witness); **knot insertion / refinement / degree elevation** editing
tools; **periodic / unclamped** NURBS (this increment is clamped/open);
weight dimensions; analytic line/NURBS and NURBS/NURBS intersections in the
arrangement (today sampled). Re-expressing the existing `Spline` on the NURBS
kernel is a possible later cleanup, not part of this increment.

## Tests (external `xxx_test`, testify/require, assert solved geometry)

- Kernel: a rational-quadratic NURBS reproduces an **exact circle arc**
  (`|r−1|` ~1e-15 at many `t`); a non-rational single-span equals the plain Bézier;
  `Eval(t_first)=P_0`, `Eval(t_last)=P_n`; clamped-knot partition-of-unity.
- Area: non-rational NURBS region area is **exact** and sampling-independent
  (cubic); a rational quarter-circle-arc region nets the circular-segment area to
  ~1e-10 and is sampling-independent (assert across `WithSegmentsPerTurn`).
- Self-intersection: a deliberately looping high-degree NURBS reports
  `SelfIntersecting`.
- Profile participation; JSON round-trip (degree/knots/weights/points; no
  doubling); SVG/PNG/DXF export contains it; DXF carries degree/knots/weights and
  rebuilds to the same curve; world-space DXF round-trips control points.
- `CreateNURBS` validation table (bad degree / knot count / unclamped / non-monotone
  knots / non-positive or wrong-count weights / too-few control points / nil) →
  `ErrInvalidShape`. Free-NURBS DOF = `2(n+1)`. `RemoveEntity` keeps the points.
- An executable `examples/` example with `// Output:`.

## Verification

Work in `.worktrees/feat-nurbs`, `GOCACHE=$PWD/.tmp/go-build`. `gofmt -l .`,
`go vet ./...`, `go test ./...` clean; `go generate ./...` for the README.
Adversarial review focused on: the basis/derivative kernel (span boundaries,
multiplicity, the clamped ends), the two-tier area (non-rational exactness, the
rational adaptive tolerance, the split-fragment triangle correction), the
self-intersection inclusion, DXF SPLINE round-trip fidelity, and that every
integration site got its case.
