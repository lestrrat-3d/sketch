# Conic curve — a Curve-parity geometry primitive

## Context

The Tier-2 **Curve parity** row (`docs/verification-roadmap.md`) lists
*conic/NURBS import representation* as a remaining gap. This increment ships the
**conic** half: a conic arc the engine can author, verify (profile/region
participation), serialize, and export — so a sketch containing a conic (a Fusion
"conic curve", a parabola/hyperbola/ellipse arc) can be represented and checked.

It mirrors how `FitSpline`/`ClosedSpline` shipped: an authorable, profile-
participating, serializable, exportable **curve**. Constraints *on* a conic
(point-on, tangency, a rho dimension) are explicit follow-ups — this ships the
entity.

## Representation: a rational quadratic Bézier

A CAD conic arc is exactly a **rational quadratic Bézier**: two endpoints
`Start`/`End`, an apex control point `Apex` (the intersection of the endpoint
tangents), and a fullness parameter **rho** `ρ ∈ (0,1)`:

- `ρ = 0.5` → parabola, `ρ < 0.5` → ellipse arc, `ρ > 0.5` → hyperbola arc.

The Bézier weight on the apex is `w = ρ/(1−ρ)` (endpoints weight 1). The curve:

```
        (1−t)²·Start + 2(1−t)t·w·Apex + t²·End
P(t) = ───────────────────────────────────────── ,  t ∈ [0,1]
            (1−t)² + 2(1−t)t·w + t²
```

`P(0)=Start`, `P(1)=End`; the curve is tangent to `Start→Apex` and `End→Apex`.

### Sketch entity

```go
type Conic struct {
    s                *Sketch
    Start, Apex, End *Point
    rhoi             int // rho var index, kept in (0,1)
    id               int
    construction     bool
    refState
}
```

`AddConic(start, apex, end *Point, rho float64) (*Conic, error)` — validates
`rho ∈ (0,1)` (`ErrInvalidShape` otherwise) and allocates the rho var via
`newVar` (like `Ellipse`'s rx/ry/rot). DOF of a free conic: 6 (three points) + 1
(rho) = 7. Accessors: `Rho()`, `Start/Apex/End`, `Geometry()`. rho is a solver
var (not a fixed scalar) so a later increment can dimension/constrain it; for now
no constraint references it, so it is a free DOF (a free conic is under-
constrained exactly like a free ellipse).

### Transient geometry (`geom`)

`geom.Conic{Start, Apex, End Point; Rho float64}`:
- `Eval(t)` — the rational quadratic above (weight `w=ρ/(1−ρ)`).
- `EvalDeriv(t)` — analytic derivative (for the future tangent constraint; also
  lets the area integrand stay analytic, but area uses the closed form below).
- `Polyline(n)` — `n`-segment sample Start→End in `t`.
- `Endpoints()` → Start,End, so it is an **open** `geom.Curve` (like `Arc`).

## Exact area (preserves the "every fragment exact" invariant)

A sampled conic would make a conic-bounded region's area sampling-dependent,
regressing the invariant that every curved fragment's area is exact. The conic
bulge — the signed area between the curve and its chord `Start→End` — has a
closed form. With `Start` at the origin, `a = Apex−Start`, `b = End−Start`,
`c = 1−w`, `W(t) = 2c·t² − 2c·t + 1`:

```
bulge = w·(a×b)·∫₀¹ t²/W(t)² dt
```

The cross term reduces cleanly because `N×N′ = 2w·t²·(a×b)` for the numerator
`N(t)` of the homogeneous curve. The integral `I = ∫₀¹ t²/W² dt` is closed-form
via the substitution `u = t−½` (`W = 2c·u² + k`, `k = (1+w)/2`):

```
I = 2·( J2 + J0/4 ),   J0 = 1/(4kD) + F/(2k),   J2 = −1/(4αD) + F/(2α)
α = 2c,  D = α/4 + k,  F = ∫₀^{1/2} du/(αu²+k)
F = atan(½√(α/k))/√(αk)      (α>0, i.e. ρ<0.5, ellipse)
  = atanh(½√(−α/k))/√(−αk)   (α<0, i.e. ρ>0.5, hyperbola)
  = (½)/k                     (α→0, i.e. ρ=0.5, parabola)
```

At `w=1` (parabola) this gives `bulge = (a×b)/3 = ⅔·triangleArea`, the known
quadratic-Bézier result. Verified numerically against fine integration to ~1e-13
across ρ∈{0.17…0.83} (ellipse/parabola/hyperbola). Lives in
`geom/arrange.go` as `conicBulgeSpan(start, apex, end, w, t0, t1)` (whole-curve
`conicBulge` is the `[0,1]` case) next to `chordEllipseCorrection`, used by
`makeCycle`'s area case (the parabola branch is selected by a small-`α`
tolerance to avoid the `1/α` removable singularity). The **per-fragment** form
(needed when a crossing splits the conic) is the whole moment swept from `start`
minus `triangle(start, P(t0), P(t1))`, leaving the area between the sub-arc and
its sub-chord; for the whole curve `t0=0 ⇒ P(t0)=start`, so the triangle
vanishes.

*Exactness scope (matches the ellipse/spline families):* the bulge is exact
**given the fragment's parameter span**. A line/conic (or conic/conic) crossing
is detected on the **sampled** polyline (`analyticKind` covers only
line/circle/arc), so a conic **split** at such a crossing has an approximate cut
parameter — its area therefore *converges* with sampling rather than being exact,
exactly like a split ellipse or spline. A **whole** conic (and a conic split only
at an analytic line/circle/arc crossing) is exact and sampling-independent. This
is not a false-bless: the region is the correct topology with a convergent area,
never `Degenerate`-vs-valid confusion.

## Arrangement integration (`geom/arrange.go`)

A new `srcConic` source kind, mirroring `srcEllipticalArc` (an **open** curve):
`source` gets the conic params; `at(t)` evaluates via `geom.Conic.Eval`;
the *curves* (open) branch assigns the kind; `safeEndpoints`/`sampleParams`
cover it; `makeCycle`'s area case adds the `conicBulge` correction (so a region
bounded by a conic + chord has exact area). Crossing detection stays **sampled**
(no closed-form line/conic or conic/conic intersection yet — same posture as
splines and ellipses), which is sound: topology is sampled; a whole conic's area
is exact, and a split conic's area converges with sampling (see the area note).
Self-intersection: a single rational quadratic Bézier is convex-hull-bounded and
cannot self-cross, so (like an arc) it is not its own self-crossing source.

## The integration sites (each a new `case`, mechanical — mirrors elliptical arc)

- **sketch.go**: struct + `AddConic` + accessors; `localPolyline`,
  `entityPoints` (Start/Apex/End), `entitySizeVars` (rho).
- **geom**: `Conic` type (geom.go) + `Eval`/`EvalDeriv`/`Polyline` (sample.go) +
  `Endpoints` (loops.go); arrangement `srcConic` + `at` + kind-assignment +
  `makeCycle` area case + `conicBulge`.
- **json.go**: marshal/rebuild case `"conic"` (Start/Apex/End ids + rho);
  no internal constraints to recreate (unlike the elliptical arc — a conic pins
  no points onto an implicit curve), so simpler.
- **Exporters**: svg.go/png.go draw the sampled polyline (thin wrapper like
  splines); dxf.go emits a **native degree-2 NURBS** `SPLINE` — knots
  `[0,0,0,1,1,1]`, control points Start/Apex/End, **rational weights** `1,w,1`
  (group code 41), flag 70 bit 4 (rational). World-space: control points are
  ordinary points → reuse the existing `putWCS` path.
- **profiles.go**: feed it into `curves []geom.Curve` (open), like `Arc`.
- **removal.go**: `renumberEntity`, `entityUsesPoint`, `RemoveEntity` rho-var
  retirement. (No constraint-ref case — no conic constraint yet.)
- **tools.go**: `varKind` (rho → dimensionless) so DOF/conditioning are right.
- **reference.go**: `isNilEntity`, `entityPoints` cases (no reference conic).
- **conditioning.go**: no new constraint rows (rho is a free var, not a residual
  row), so `condRowKinds` is untouched; the rho **column** is dimensionless and
  handled by the default column scaling.

## Deferred (explicit follow-ups)

Point-on-conic and line/conic & conic/conic tangency (need the analytic
foot-point / contact witness like the spline/conic-tangency machinery); a **rho
dimension** (`NewRho` or reuse a generic scalar dimension); analytic line/conic
and conic/conic intersections in the arrangement (today sampled); the **NURBS**
half of the roadmap row (general non-uniform rational B-spline) — a separate
increment that can share this rational-curve evaluator.

## Tests (external `xxx_test`, testify/require, assert solved geometry)

- Author a conic; assert `Eval(0)=Start`, `Eval(1)=End`, tangent directions at
  the ends point along Start→Apex / End→Apex; a parabola (ρ=0.5) midpoint is the
  control-triangle's `¼·Start + ½·Apex + ¼·End`.
- Exact area: a conic + chord region's `Area` matches the closed form and is
  **sampling-independent** (assert equal across `WithSegmentsPerTurn` 16/64/256),
  for ρ = 0.3 / 0.5 / 0.8.
- It participates in a `Profile` as an open boundary curve (region closed by a
  conic + lines); area finite/positive; `Valid`.
- JSON round-trip (rho preserved; entity count stable; no doubled internal
  constraints).
- SVG/PNG/DXF export contains it; DXF carries the `SPLINE` with weights `1,w,1`;
  world-space DXF round-trips the control points.
- `FixEntity` grounds rho; `RemoveEntity` retires it.
- `AddConic` rejects ρ ∉ (0,1) (`ErrInvalidShape`).
- An executable `examples/` example with `// Output:`.

## Verification

Work in `.worktrees/feat-conic`, `GOCACHE=$PWD/.tmp/go-build`. `gofmt -l .`,
`go vet ./...`, `go test ./...` clean; `go generate ./...` for the README.
Adversarial review focused on: the exact-area closed form (the ρ-branch
boundaries near 0.5), the DOF/var accounting (rho as a free var), and that every
integration site got its case (a missed exporter/removal/profiles/arrangement
case is a silent gap).
