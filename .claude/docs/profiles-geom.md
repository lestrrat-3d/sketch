# Profiles and the `geom` package

Detail moved out of CLAUDE.md. Read before touching `Sketch.Profiles`, `BoundaryEdge` exactness, or anything under `geom/` — especially the arrangement engine.

## Question router

| Your question | Section |
|---|---|
| When is a `BoundaryEdge` range exact? | The whole-sketch gate comes first |
| Why did exactness disappear scene-wide? | The whole-scene kind gate `exactAllowed` |
| When is a profile invalid or stale? | `Valid` is per-region / A profile is a snapshot |
| What does `geom.Regions` return? | The planar-arrangement / region engine |
| Which crossings are analytic? | Analytic crossing detection |
| Why was a clean crossing refused? | Curve/curve transverse crossing authority |
| Why is a region flagged degenerate? | Chord-deviation degeneracy bounds |
| Two curves lying on the same carrier? | Coincident-carrier overlap resolution |
| What do `geom`'s constructors validate? | `geom` constructors are value holders |
| Which segment pairs does `intersect` even look at? | The broad-phase reach (`intersect`'s pair enumeration) |
| Why does `sampledCrossingsExplained` skip some segment pairs? | The box reject in `sampledCrossingsExplained` |

Navigation only — the sections below are the authority.

## `profiles.go` — closed planar regions

### Overview

`Sketch.Profiles()`: closed planar regions via the `geom` arrangement engine —
bare-crossing subdivision, holes/nesting, net area, and per-region validity
(self-intersecting/degenerate). `Profile` carries `Outer`/`Holes`
(`BoundaryEdge`s, whole or fragment), `Area`, `Valid`, `SelfIntersecting`;
construction excluded, reference geometry included.

### `Valid` is per-region

**`Valid` is per-REGION,
including its degeneracy half**: a region is invalid when an unresolvable
condition reaches ITS OWN boundary curves (or is unattributable), so a sketch
can hold both valid and invalid profiles and trouble in one corner no longer
invalidates a disk across the sketch; `Verify`'s `ProfilesValid` stays the
arrangement-wide verdict, so it can be false with `InvalidProfiles` empty (a
condition that produced no region). Internal `buildProfiles` also surfaces
arrangement degeneracy to `Verify`.

### A profile is a snapshot and can go stale

**A `Profile` is a snapshot, so it can go
stale**: each carries its origin `Sketch()` (freshly allocated every call —
pointer identity can never prove provenance) and the `Revision()` it was built
at, and `Profile.IsStale()` says the sketch has moved under it. A consumer that
turns a profile into a solid MUST check — extruding a stale profile silently
builds the old shape with no error anywhere.

### `TStart`/`TEnd`/`TExact` — the sub-range an edge covers

A `BoundaryEdge` also reports
**which** sub-range it covers — `TStart`/`TEnd` (normalized `t∈[0,1]` in the
entity's *natural* direction, so `TStart<TEnd` and `Reversed` alone carries walk
order; never wrapping) plus **`TExact`**, which is the load-bearing half: it is
true only when BOTH bounds come from the closed-form kernel, a sample vertex, or
the curve's own endpoint.

### The whole-sketch gate comes first

**A WHOLE-SKETCH gate comes first**: exact bounds are
published only when EVERY entity the profile pass sees is a line/circle/arc, so
one ellipse/elliptical-arc/conic/spline/closed-spline/fit-spline/NURBS anywhere
makes every `BoundaryEdge` of every profile read `TExact=false` — the lines,
circles and arcs beside it included, however far apart they sit (`exactAllowed`,
in the `geom` section: a free-form entity is only ever chords, so it can hide a
crossing between two samples and leave the certified pairs publishing the fused
profile set as exact — the near-miss guard now reports such a map `Degenerate`,
but it certifies nothing where it stays silent, so exactness keeps the kind
gate). Within an all line/circle/arc sketch the closed-form kernel runs on any
pair of those three, so **every** contact involving an
ellipse/elliptical-arc/conic/spline/NURBS — *even against a plain line, and even
when it is a tangency* — is sampled and reports `TExact=false` on its own
account too: the topology is right but the parameter only converges with
sampling. A crossing between two CURVED sources (circle/arc against circle/arc)
is exact only when the arrangement's incidence certificate passes (see the
`geom` section); anything that fails it falls back to the sampled path and
reports `TExact=false` — a sampling too coarse to resolve the crossing (raising
the density recovers it), a contact at an open curve's own endpoint (which NO
density certifies), a contact inside the sampling's parameter window of a sample
vertex but away from it in position (a contact that IS a sample vertex, at
round-off, stays exact), and the two sampled polylines MEETING anywhere other
than at the crossing points themselves — an endpoint of one resting on the
interior of the other's chord, a pass-through landing on a sample vertex, or a
shared stretch of chord, each a place the sampled map has a contact the exact
geometry does not. Below the density at which the sampled path resolves such a
crossing ITSELF, the crossing is missing from the map and the regions it
separates are fused, so exactness is withdrawn from every edge of the affected
connected component, certified pairs included (`refuseExactOnFusedMap`, in the
`geom` section) — and the crossing a free-form entity can hide is covered by the
whole-sketch gate above, so an all-exact profile set also says no crossing is
missing from it. A consumer that records a profile structurally or emits CAD
from it must branch on `TExact`, never trust the range blindly.

### `Partial` and `TExact` come from the emitted fragment

`Partial` and
`TExact` are answered from **the emitted fragment itself**, never from a
per-source "was this curve cut anywhere" proxy (which outlives pruning and
reports a phantom `Partial` on a whole curve) and never from a numeric compare
of the range against `[0,1]` (which cannot tell a bound that *is* the curve's
end from a crossing that landed `1e-10` away — and would bless a sampled-bounded
fragment as the whole curve, the unsafe direction). Instead each bound carries
its **provenance** (`cut.srcEnd` → `arrEdge.endU/endV` → `frag`): it is either
the curve's own domain end (an open curve's endpoint, or a closed curve's seam)
or a cut/weld, and `makeCycle` sets `Whole` iff **both** surviving outer bounds
are the curve's own ends — decided *after* pruning and coalescing, so a contact
whose partner is pruned away, and a closed curve cut once (one edge leaving the
contact and returning to it, so its seam is what bounds it), both correctly read
whole. `split`'s dedup **ANDs** provenance as well as exactness into a
coincident boundary, so a cut landing on a domain end is a cut; the one
conservative corner is a closed curve whose single cut lands ON the seam, which
then reads `Partial` over `[0,1]` — never a false `Whole`. Exactness is tracked
per `cut` (`cut.exact`): `split`'s dedup **ANDs** coincident boundaries (a
boundary a sampled cut lands on is only as trustworthy as that cut), and
`makeCycle`'s fragment coalescing carries the **surviving outer bounds'**
exactness (an interior boundary that coalesces away is not reported, so it is
not folded in). The vertex table welds by **distance** while the crossing tests
decide in **parameter** space, so a source can be split in the graph with no cut
record at all: `taintMergedEndpoints` pushes an `exact:false` marker at any
sample vertex two different sources weld at, and — since a weld happens for
analytic pairs too — `auditMergedEndpoints` does the same for a `handled` pair's
welds *unless* one of that pair's analytic events places a CONTACT there — the
set `eventContacts` owns, which for a resolved coincident-carrier overlap is the
window's two boundary points and NOT the event's own midpoint (an exact cut is
never laundered into a sampled one, and a distance weld is never laundered into
an exact cut).

### `vertexCertifies` — final exactness certification

**Bound exactness is finally certified after canonicalization by
`vertexCertifies`: TExact holds only when the graph vertex IS the bound's own
point (`eval(param)`) within round-off — the merge tolerance is never used for
the exactness decision, only for welding/topology.** That round-off band is
bounded by TWO yardsticks at once, `weldIdentEps·scale` AND the SOURCE-local
`weldIdentEps·source.extent` (that source's own polyline bounding box, the scene
formula applied to one curve), the same two-band shape `carriersIdentical` uses;
`endpointReproduces`, which decides a segment endpoint bound by the same
question, carries both for the same reason. On the scene band alone a distant
unrelated object widened what counts as identity HERE: a circle of radius 5
whose exact crossing welds `1e-9` onto one of its own sample vertices reports
that bound inexact when drawn with its chord alone, and ONE line parked at
`x=1000` flipped it to exact — through `Sketch.Profiles()` with no options —
publishing a parameter that misses its own polyline endpoint by the whole
`1e-9`. The gap is a displacement along the curve whose parameter is being
certified, so the curve's own size is what it is judged against
(`TestExactBoundIdentityBandIsSourceLocal`).

## The `geom` package (slated for extraction)

### Scope and the transient/sketch split

`geom/` holds **transient geometry** — plain `Point`/`Line`/`Circle`/`Arc`
definitions, *coordinates only*, no document state (no construction flag, no
name), no sketch/solver/constraints. It is the engine's `adsk.core` analog: a
pure math layer and the **snapshot type** that a sketch entity hands back from
its `Geometry()` accessor. It is **not** an input you hold and commit — sketch
geometry is authored directly from points (see "Building blocks vs sketch
geometry" below). It must not import `sketch`; the arrow is `sketch -> geom`,
never the reverse. Production code is standard-library-only (tests use
`testify/require`); intended to move to its own module later.

### `geom` constructors are value holders

**`geom` constructors are value holders; the arrangement engine validates at
the point of use.** Of the twelve package-level `New…` constructors, eight —
`NewPoint`, `NewLine`, `NewCircle`, `NewEllipse`, `NewArc`, `NewEllipticalArc`,
`NewConic`, `NewNURBS` — validate nothing and have no error return; a nil
point, a non-finite coordinate or a degenerate radius all construct cleanly.
The other four are narrower than general input validation, not an exception to
it: `NewSpline`/`NewClosedSpline`/`NewFitSpline` check only the point COUNT
their kernel needs (`ErrTooFewControlPoints`/`ErrTooFewClosedControlPoints`/
`ErrTooFewFitPoints`) — a precondition the evaluator itself cannot express,
never a check on a point's coordinates — and `NewFitInterpolant` validates a
BUILT interpolant's finiteness (`ErrNonFiniteFitInterpolant`), not its input
fit coordinates. Everything else is caught later, at the point of use, by a
small, named net: `posFinite` (a usable radius/semi-axis), `nurbsValid` (NURBS
structure — nil control points, a malformed knot vector), `fitSplineCoords`
(a nil or non-finite fit point, screened before `newFitEvaluator` can drop
one), the per-kind extent guards in `newArranger` (an all-coincident
control/fit-point set that is a point, not a curve), and `densify`'s
`finitePt` as the last net over every evaluated sample (see below).

### The construction toolkit

It also carries the **construction toolkit** (`intersect.go`, `modify.go`,
`transform.go`): line/circle/arc intersections (arc cases reduce to circle
cases filtered by `Arc.Contains`), `ClosestPointOnLine`, `SplitLineAt`/
`SplitArcAt`, `Fillet`/`Chamfer` (which replace a shared endpoint with fresh
contact points and return the connecting arc/line), and the `MirrorPoint`/
`TranslatePoint`/`RotatePoint` transforms. These compute on transient geometry;
the *mutating* sketch-level tools in `tools.go` (`Trim`/`Extend`/`Break`/
`CreateFillet`/`CreateChamfer`/`CreateMirror`/`CreatePatternRect`/`CreatePatternCircular`/
`CreateOffset`) feed them an entity's `Geometry()` snapshot, then build the
replacement from sketch points and retire the originals with `RemoveEntity`.

### The planar-arrangement / region engine

It also holds the **planar-arrangement / region engine** (`region.go`,
`arrange.go`, `area.go`): `geom.Regions(curves, closed)` builds a polyline-
approximated planar arrangement of lines/arcs/circles/ellipses/elliptical-arcs/
splines/closed-splines/fit-splines, splitting at bare crossings, and returns the
bounded
`Region`s (each an
outer boundary loop +
holes, with a net `Area` and source-curve `BoundaryEdge` back-references) plus
soundness signals — a per-`Region` `Degenerate` (an unresolvable condition reaching
THAT region: one involving a curve its own boundary is built from, or one no curve
could be blamed for — attribution is by SOURCE, never by where the condition's
representative point landed, since several of those points are only a midpoint
between two sources; the arrangement-wide `Degenerate` stays the flag a consumer
gates trustworthiness on, since it also covers a condition that produced no region),
`SelfIntersections` (only for a single simple closed loop —
every shared vertex degree 2 — judged on the pruned core, so a branched/
subdivided wire is *not* flagged; a spline is the one source whose *own* polyline
is tested for self-crossings, since a cubic can loop) and `Degenerate`
(collinear-overlap, near-tangent uncertainty, a NEAR MISS the chord-deviation
bounds cannot separate — see `nearMissGuard` below — or a source whose EVALUATED
SAMPLES are non-finite). The last is `densify`'s own finiteness check
(`finitePt`): a NaN/Inf sample compares false against everything, so an
unchecked source would contribute no vertex, cut or edge at all and silently
vanish from the arrangement — the curve disappears, and whatever it would have
subdivided reads as one clean, wrongly-sized region with `Degenerate=false`.
Reachable from stored data a constructor's own validation does not catch (e.g. a
NURBS control-point coordinate gone NaN after construction — `CreatePoint` has no
error return and the solver moves those coordinates freely, so `CreateNURBS` cannot
guard this the way it guards knot finiteness — or a NURBS built directly via
`geom.NewNURBS`, which is deliberately unvalidated). `densify` samples the whole
source into a buffer FIRST and drops it as
degenerate (`srcDegenerate` + `flagDegenerate`, `exactAllowed` forced false
scene-wide, the same rule an unusable input curve already gets in `newArranger`)
if any point is non-finite, because it is the ONLY place that sees evaluated
samples — a check over stored control points/knots/weights cannot catch a value
that only goes non-finite once evaluated. **The fit-point spline is the one
exception, screened earlier instead:** `newFitEvaluator` collapses two
consecutive fit points closer than `fitChordEps` via
`math.Hypot(...) > fitChordEps`, a comparison that is FALSE against a NaN, so a
non-finite fit point reads as "coincident with its predecessor" and is
silently DROPPED before the evaluator computes a single sample — the curve
then interpolates a different, perfectly finite curve through the remaining
points, so by the time `densify` samples it there is no non-finite value left
to catch. `fitSplineCoords` therefore screens every raw fit point for
finiteness itself, the same place it already screens for a nil point, closing
the gap before `newFitEvaluator` ever runs.

### The broad-phase reach (`intersect`'s pair enumeration)

`intersect` (`geom/arrange.go`) does not test every pair of tiny segments against
`segParams`/`collinearOverlap`/`forEachMergedEnd`. It first asks `candidatePairs`
(`geom/arrange_broadphase.go`) for, per segment `i`, the ascending list of `j > i`
whose bounding boxes — each expanded by that segment's own **reach** — overlap.
`candidatePairs` may only ever return a SUPERSET of the pairs those three
predicates fire on; they remain the sole deciders of what actually happens once a
pair is visited. `intersect`'s loop body is unchanged by this — only which pairs it
visits is.

A tiny segment's reach is `a.merge + broadPhaseRel·(its own chord length)`, with
`broadPhaseRel = 1e-3`. Three bounds, all relative to a chord's own length, decide
that constant, and it is sized by the largest:

- `segParams`' `segEps = 1e-9` on the normalized chord parameter, so a hit lies
  within `segEps·length` of each chord;
- `collinearOverlap`'s parallel/perpendicular bands (`1e-9`/`1e-7`, relative to the
  chord lengths);
- the rounding slop of `segParams`' intersection solve. That solve accepts a pair
  down to `|d1×d2| = 1e-12·|d1||d2|`, and at that near-parallel limit the computed
  chord parameters carry an absolute error of order `eps/1e-12 ≈ 4e-4` relative to
  the chord lengths. This is the term that sets the constant — it is three orders of
  magnitude above `segEps`, so a reach sized only against the exact-arithmetic
  tolerances would not cover it.

`forEachMergedEnd` welds two endpoints within `a.merge` of each other, which the
`a.merge` term of the reach covers directly: an endpoint inside that distance of
another segment's endpoint sits inside that segment's OWN raw box expanded by
`a.merge`, so the two expanded boxes overlap. Expanding by reach only ever ADMITS
more pairs than the exact tests would separately accept, so the broad phase cannot
drop a pair the loop body would have acted on.

The third bound holds for geometry whose coordinates are within a few chord lengths
of the origin. Far from it the cancellation in that solve grows with the coordinate
magnitude, and no relative reach bounds it — but `segParams`' own hit point, chord
parameters and crossing angle are meaningless in that regime too, so the arrangement
is already unreliable there and the broad phase adds no failure mode of its own.

The enumeration itself is a sort-and-sweep on each segment's box minX, with ties
broken by original index so the sweep is deterministic: segments are visited in
that order, an active set holds every segment whose box could still overlap a
later one's, and a pair is recorded (under the smaller of the two original
indices) on a y-axis overlap once the x-overlap is implied by the sweep order. Each
`cand[i]` is sorted ascending before `intersect` sees it, so the `(i, cand[i])` walk
visits pairs in the same relative order the exhaustive `(i, j)` scan did — this
matters because `splitFragments` dedups near-equal cut parameters keeping the
first, and `sort.Slice` is unstable. `TestBroadPhaseIsSuperset`
(`geom/arrange_broadphase_internal_test.go`) checks the superset property against
`segParams`/`collinearOverlap`/`forEachMergedEnd` directly, the ascending-order
contract, and agreement with a naive box-overlap scan, over representative fixtures
and 200 seeded random scenes.

A dense scene (many mutually overlapping long segments, each with a large active
set through most of the sweep) stays close to the same cost as the exhaustive scan;
the win is for sparse curve-heavy scenes, where the active set stays small.

Every tiny segment's reach-expanded box (`segBoxOf`) is computed once and cached on
the arranger (`segBoxCache`, populated on first use, valid for the arranger's whole
life since `densify` is the only appender to `a.segs` and it runs before `intersect`).
`candidatePairs`' sweep and `sampledCrossingsExplained`'s box reject (below) share the
one cache instead of each computing its own boxes.

### The box reject in `sampledCrossingsExplained`

`sampledCrossingsExplained` (the third part of the curved-pair consistency gate,
above) is a CONJUNCTION over every pair of tiny segments from the two sources: it
returns `false` — refusing the pair, so `analyticPrepass` flags it `Degenerate` — the
first time it finds a sampled crossing with no analytic contact behind it, and `true`
only once every such crossing has been checked and explained. Skipping a segment pair
therefore drops it from that conjunction; a pair that would have crossed and gone
unexplained is the one pair that must never be skipped, since dropping it silently
flips a correct `false` (refuse, flag degenerate) into a wrongly-blessed `true`. This
is the opposite direction from `candidatePairs`' own guard: that one only ever gates a
side effect (whether `intersect`'s loop body runs at all for a pair), so widening it
can only add work, never change an answer; this one gates the answer itself, so the
reject must never be able to exclude a pair `segsCrossInteriorAt` would have found a
crossing on. The only pair safe to skip is one that provably cannot cross at all —
skipping it removes nothing from the conjunction either way.

The reject reuses the SAME machinery `candidatePairs` already proves safe for exactly
this reason: a pair is skipped only when its two segments' `segBoxCache` boxes (each
already expanded by `segReach` = `a.merge + broadPhaseRel·(chord length)`) fail
`boxesOverlap`. `segsCrossInteriorAt`'s positive set (an interior hit, `ti`/`tj`
strictly inside `(segEps, 1-segEps)`) is a subset of plain `segParams`' positive set
(endpoints included), and `TestBroadPhaseIsSuperset` already proves those
reach-expanded boxes are a superset of everything `segParams` accepts — so they are
necessarily also a superset of `segsCrossInteriorAt`'s stricter interior-only set,
with no new constant and no new proof needed.
`TestSampledCrossingsExplainedBoxRejectAgrees`
(`geom/arrange_broadphase_internal_test.go`) checks the box-guarded verdict against an
unguarded brute-force reference over representative fixtures and 300 seeded random
scenes.

### Exact region area per curve type

Region area is exact for
**every** curve type: line/arc/circle (shoelace + exact circular-segment
correction), ellipse/elliptical-arc (`chordEllipseCorrection` = ½·rx·ry·(Δφ −
sin Δφ), the elliptical analog, rotation/translation-invariant), and **splines**
(`splineBulge`: the exact ½∫(x·y′−y·x′) of the fragment's piecewise cubic via
3-point Gauss–Legendre per knot span — exact because the integrand is degree-5 —
needing analytic spline derivatives `EvalCubicBSplineDeriv`/
`EvalPeriodicCubicBSplineDeriv`/`fitEvaluator.derivAt`), and **conics**
(`conicBulgeSpan`: the rational-quadratic-Bézier closed form — the moment swept
from `start` minus `triangle(start,P(t0),P(t1))`, i.e. the exact sub-arc/sub-chord
area; whole-curve `conicBulge` is the `[0,1]` case), and **NURBS**
(`nurbsBulgeSpan`/`nurbsMoment`: `splineBulge`'s shape — per-knot-span moment +
chord-closure — with `p`-point Gauss exact for a non-rational degree-`p` curve and
10-point adaptive bisection for the rational/`p>10` case, integrating the true
curve). So the reported `Area` is
sampling-independent **for a whole curve** (and for one split only at an analytic
line/circle/arc crossing, curve/curve included once certified); a curve split at a
*sampled* crossing (anything involving an ellipse/spline/conic/NURBS, or an
uncertified curve/curve crossing) has an approximate cut parameter,
so its area *converges* with sampling rather than being exact — the correct
topology with a convergent area, never a false bless.

### Analytic crossing detection

**Crossing detection is becoming
analytic** (`arrange_events.go` + the `analyticPrepass` in `arrange.go`; design +
increment plan in `docs/analytic-arrangement-design.md`): the exact closed-form
contact (`Cross`/`Tangent`/`Overlap`) is authoritative **only for a pair whose BOTH
sources are a line, circle or arc** (`analyticKind`; `analyticEvents` returns
`ok=false` otherwise) — and, within that pair set, for line-involved crossings and all
tangencies. The rule is about the PAIR, never about "a line was involved": a line ×
ellipse/conic/spline/NURBS contact — **including a tangency** — has no closed form
here and stays on the sampled path. For an analytic pair the sampled `segParams` is
skipped, cuts land on the exact intersection point (`cut{t,px,py}`, so two sources
merge to one vertex), and the oracle no longer false-flags clean tangencies (tangent
line+circle → one disk; non-merged tangent circles → two disks) or shallow crossings
as `Degenerate`.

### The three-part consistency gate

**Curve/curve transverse crossings (both circle/arc) take analytic authority only
behind their own incidence certificate** (`analyticCrossingsCertified`, below); an
uncertified pair falls back to the sampled path, which is what the three-part gate
below does NOT judge. For every other handled curved pair a **three-part consistency
gate** keeps it sound at coarse sampling, and which of the first two applies turns
on whether the contact INSERTS a vertex (`crossNeedsSampledWitness`, answered by the
single `cutSite` the cut phase itself acts on, so the gate can never demand what the
cut phase does not do). A crossing that inserts one on BOTH sources must be
**witnessed** on its own host segment-pair (`analyticCrossHosted`) — that is the
disk-vanishing case, where the injected point sits off the chord by up to the
sagitta. A contact the sampled map ALREADY has a vertex for (a source's own
endpoint, or an interior sample vertex) inserts nothing, so no witness is possible —
a contact at a segment boundary is not interior to that segment — and it is instead
required to be **resolved**: two contacts inside ONE chord of a curved source is a
sub-sample cap (`contactsResolved`). Third, every sampled crossing must be
**explained** by a contact within one curved chord of it (`sampledCrossingsExplained`)
— a leftover crossing is the chord approximation disagreeing with the geometry, and
the face walk has no vertex for it. Failing any, it is conservatively `Degenerate`
rather than a vanished disk. **Demanding a witness where none can exist
is what once false-flagged everyday geometry** — a line ending exactly on a circle
(the gear flank meeting its root circle, and what `NewPointOnCircle` builds), a chord
crossing where the sampling happens to put a vertex, a corner join — so a contact
that inserts no vertex must never be measured against the sampled crossing count.

### Exact tangent-port ordering

**Exact tangent-port ordering (increment 3, partial):** at a
certified analytic tangency contact (`exactPortVerts`) the DCEL rotation system
orders coincident-tangent half-edges by exact source tangent + signed **curvature**
(`source.differential`/`portKey`/`sortExactPorts`, an exact lexicographic order with no
epsilon in the comparator — `dirParallelEps` enters only the same-ray clustering and
the osculation flag, both of which compare directions by dot sign and scaled cross
magnitude)
instead of chord angle, so a **merged-vertex EXTERNAL circle/arc tangency** is now
blessed as two clean disks (opposite curvature separates the loops) rather than
flagged. Used ONLY at those certified contacts — at a sampled crossing vertex the
edges are chords, so chord ordering is what matches the traversed geometry.

### Internal (containment) tangency

**Internal (containment) tangency is now also blessed (increment 7 §7a):** the
shared contact gets the same exact tangent-port ordering, and hole assignment uses
an **exact point-in-region** test (`exactPointInRegion`: a ray-cast with closed-form
circle/arc crossings, immune to the chord poke-out that defeated the sampled
`pointInPolygon` near the contact), so the inner cycle nests into the outer as an
annulus + inner disk — exact at every sampling, tiny inner included. Line-involved
merged tangency and genuine osculation stay conservatively `Degenerate`. Ellipse/
spline pairs keep the sampled fallback (exact containment falls back to the chord
polygon for them). `Sketch.Profiles()` is its consumer.

### Curve/curve transverse crossing authority (§7b)

**Curve/curve TRANSVERSE crossing authority (§7b) rests on its own incidence
certificate, NOT on the three-part gate** (`analyticCrossingsCertified` in
`geom/arrange.go`). The three-part gate does not carry over to a pair of sampled
curves, and the failing part is `analyticCrossHosted`: it looks for the sampled
witness on the very segment pair carrying the crossing's two source parameters,
while the sampled crossing sits off the exact one by roughly sagitta/sin(crossing
angle). With a line operand only ONE grid can be off (a line's polyline IS the line,
one segment covering it) — measured ~0.3% false flags; with two sampled curves both
can, and the miss rate aliases against the two grids instead of falling with density
(measured: the round-2 pair still flagged at `spt=128`, ~7% of a well-separated
angle/distance sweep flagged, isolated `spt` values failing into the hundreds).
The certificate asks the question directly instead, on the geometry the cut phase
emits: splice each exact crossing point into BOTH polylines at the site `cutSite`
reports (`postCutPolyline`, so the gate can never expect what the cut phase does not
do — including WHICH vertex an uncut contact maps to, which is why `cutSite` reports
the contact's real segment-local parameter at a source END, 1 at an open end, and not
a placeholder 0 that named the far end of the last segment), then require **three**
things. The first cut of this certificate carried (1), a weaker (3) that PASSED an
endpoint contact, and no (2) at all, and a review found both holes.
(1) The spliced polylines meet ONLY at those points —
**every contact between a segment of one and a segment of the other belongs to a
crossing point BOTH segments are INCIDENT to** (`polylinesMeetOnlyAtSharedVertices`),
since a handled pair's sampled crossings are never recorded, so a leftover one has no
vertex and the face walk fuses the regions on either side (the round-2 bug).
**Membership is decided COMBINATORIALLY, by shared incidence** — contact `k` is the
vertex `ci[k]` of one polyline and `cj[k]` of the other (`postCutPolyline`'s second
return), and a contacting segment pair passes exactly when some `k` is an end of both
segments. Two segments sharing an endpoint meet only there unless they are collinear,
and `collinearOverlap` refuses collinearity first, so shared incidence admits exactly
the contacts that ARE an injected crossing point — **with no tolerance, no scene scale
and no chord length in the verdict**. Comparing the contact's POSITION against the
injected points needs a band, and the only one available here is `weldIdentEps·scale`,
the whole SCENE's bbox extent: unlike `contactIsVertex` (chord-local) and
`vertexCertifies` (source-local), this part has no single source or vertex to state a
local yardstick against, so a distant unrelated object widened what counts as one point
HERE — two circles crossing at `0.01` rad `5e-13` past a sample vertex are refused alone
and certified once a line is drawn 60 units away, and the difference reaches
`geom.Regions` (`TestAnalyticShallowCrossingCertificateIsSceneIndependent`). A
chord-local band cannot replace it: `segParams`'s chord/chord intersection sits off the
true crossing by roughly the sagitta over the sine of the crossing angle, which no chord
length bounds. **Incidence is by INDEX, never by WHERE the contact sits along its host
segments** — a third question, which excused a contact for landing within `segEps` of
either segment's END and so admitted two shapes of contact that carry no node in
the map: a source's own ENDPOINT resting on the INTERIOR of the other's chord (parameter
1 on one segment, excused, while the polylines really do meet there —
`TestAnalyticCurveCrossingEndpointOnChordNotCertified`, where certifying published one
region where the sampled map has two), and a genuine transverse PASS-THROUGH that lands
on a sample vertex of one polyline (`TestAnalyticCurveCrossingAtSampleVertexNotCertified`,
the same fusion by the other door). PARALLEL pairs never reach `segParams` at all —
it rejects them on the determinant before any range test — so a **collinear
OVERLAP** arrives as silence; `collinearOverlap` refuses it, an overlap of positive
length being no transverse crossing, and refusing it FIRST is also what lets a shared
endpoint stand for "these two segments meet nowhere else". A **CLOSED** source's seam
vertex is repeated at index 0 and at the end of its polyline, and the two copies differ
by round-off, so only the closed flag can recognize them as one (`segIncident`): both
segments meeting at the seam are incident to a contact reported there, and reading only
the index it came back as would refuse the pair for its own second neighbour. (2) Each
contact IS the polyline vertex it was mapped to, within the same round-off identity
band `vertexCertifies` uses (`contactIsVertex`). A spliced point satisfies this by
construction; a contact mapped onto an EXISTING sample vertex need not, because
`cutSite` decides that in the source's PARAMETER (within `segEps` of a segment
boundary) and a parameter that close still admits a POSITION gap orders of magnitude
above round-off — certifying one with a real gap published the vertex's own sample
fraction `i/n` as an exact bound for a crossing that is not there, with no second net,
since certification exempts the pair from the taint passes. A contact that IS the
vertex passes and keeps its exact bound: the sample fraction and the true crossing
parameter are then the same number. **That band is bounded by the vertex's own CHORD as
well as by the scene**, the same two-yardstick shape `carriersIdentical` uses and for the
same reason: `a.scale` is the whole scene's bbox extent, so an unrelated object far away
widens it — with `r=5` the verdict flips at a scene extent of about `24.5·r`, and ONE
construction line parked ~100 units off turned a contact `1.1e-10` from its vertex, which
the same pair correctly refuses when drawn alone, into a certified one publishing the
sample fraction `0.125` as the exact crossing parameter, reachable through
`Sketch.Profiles()` with no options at all
(`TestAnalyticContactAtVertexBandIsChordLocal`). The chord is the only length the
mapping decision is stated in, and no distant object can inflate it. (3) The four chord
departures at each injected
point ALTERNATE between the sources (`portsCross`), in the same rotation order
`buildGraph` sorts by — meeting at a point is not crossing at it. A contact at an open
source's own ENDPOINT contributes ONE departure, so the four cannot alternate and it
is refused: the curve stops there, the certificate has no evidence about it, and
blessing it certified nothing while the injected cut bent the polylines through each
other and left a disk with **zero regions** at `degenerate=false` — the sharpest
failure this gate exists to prevent. Parts (1) and (3) are **threshold-free**: no
tolerance, no chord bound, no crossing-angle floor. Only (2) compares a position, and it
decides only "one point or two", with the existing identity band — no crossing-angle or
chord-length threshold enters any of the three. The **fallback is the SAMPLED path, never a degeneracy**: an
uncertified pair is left unhandled, so it keeps the sampled
topology with `TExact=false`. Shared incidence is **not a strict superset** of the
position test it replaced: where an exact crossing is spliced within round-off of an
existing sample vertex, the splice leaves a near-degenerate segment and the other
polyline meets BOTH of that vertex's neighbours at what the map welds into one graph
vertex, which incidence sees as two segments and refuses. That refusal is on the safe
side (sampled fallback, no exact bound published), and it was measured at 2 of 20000
evaluations in a sweep aimed squarely at that regime — against 133 refusals the same
sweep lifts — and 0 of 5568 in an ordinary circle/circle and arc/arc sweep. Over the
transverse circle/circle band certification is still 100% from
`spt=16` (`Sketch.Profiles` samples at 256); an endpoint contact is refused at EVERY
density, by construction rather than by coarseness, and a contact inside the parameter
window of a sample vertex but away from it in position is refused at whatever densities
place it there (a contact that IS a sample vertex, at round-off, is certified).

### `refuseExactOnFusedMap` — exactness on a fused map

**That fallback is sound only while the sampled path RESOLVES the refused crossing,
and `refuseExactOnFusedMap` is what makes the difference observable.** Below the
density where the two chord polylines meet at all, the crossing is missing from the
map entirely, the regions it separates FUSE, and the other pairs of the same component
— certified on their own merits and cut exactly — then publish that fused map as
`TExact` on every bound. The wrong region count there is the sampled path's own
pre-existing density limit (it moves with `WithSegmentsPerTurn` with or without any
analytic authority, and this pass does not repair it); blessing it is what the pass
refuses. So each refused crossing is recorded (`deferredCross`) and reconciled after
the sampled loop against the contacts that loop actually made (`sampledContacts` — a
chord/chord crossing or a weld of two sample vertices, within one host chord of the
crossing, the same bound `sampledCrossingsExplained` uses). A crossing with no such
contact withdraws exactness from every source of its CONNECTED COMPONENT (`split`
forces `exactU`/`exactV` false through `exactRefused`) — the component, not the pair,
because a fused crossing moves the face boundaries of every cycle it takes part in, so
a fragment of any source reachable through contacts is describing the fused map. Only
the exactness FLAG is withdrawn: topology, areas and degeneracy are untouched. Judging
a represented crossing unrepresented costs that flag and nothing else, so the
reconciliation bound is deliberately the tight one.

### The whole-scene kind gate `exactAllowed`

**A pair the kernel never classified is answered a level up, by the WHOLE-SCENE gate
`exactAllowed`**: an exact bound is published only when EVERY source in the arrangement
is a line, circle or arc, so one ellipse/elliptical-arc/conic/spline/closed-spline/
fit-spline/NURBS anywhere makes every bound of that arrangement read `TExact=false` —
the analytic sources beside it included, however far apart they sit, and the free-form
curve's own uncut whole edge included. The reason it is a KIND gate and not a distance
or deviation test: a free-form source reaches the map only as chords, so a lobe between
two consecutive samples crosses another curve entirely between them — measured on a
knot-clustered degree-3 NURBS at the ADAPTIVE DEFAULT density, a midpoint-sampled
deviation of `2.1e-05` against a true `4.7e-01` maximum on the same segment, with the
resulting wrong-but-all-exact map surfacing through `Sketch.Profiles()` with no options
at all. Any per-segment deviation ESTIMATE used as a reach is the same bug with a wider
constant; the kind gate needs no threshold, and in an all-analytic scene there is no
sampled-only pair for it to bite on. **`nearMissGuard` now reports that fused map as
`Degenerate`** (below), but it does not lift this gate — it says where a crossing cannot
be RULED OUT, never that the crossing set is right where it stays silent, and a
free-form crossing's parameter is a sampled one whatever the topology. The accepted cost
is exactness on the analytic sources sharing a scene with a free-form one — topology,
areas and the reported ranges are untouched — and lifting it needs a sampler that
certifies its own per-source deviation, not a wider estimate at the point of use.

### Chord-deviation degeneracy bounds (`nearMissGuard`)

**The DEGENERACY half is answered by proven per-family chord-deviation bounds**
(`geom/nearmiss.go`; design in `docs/analytic-arrangement-design.md`): every tiny
segment carries an upper bound on how far its source departs from that segment's own
chord (`arranger.segDev`, filled by `densify`) — the exact sagitta for a circle/arc, the
`h²·M/8` linear-interpolation bound for the ellipse family (whose second derivative is
bounded by `max(rx,ry)` in the eccentric angle), and the CONVEX HULL of the sub-span's
own control polygon for the conic and the whole spline family (de Casteljau for the
conic and the cubic pieces, Boehm knot insertion to multiplicity `p` for spline/NURBS,
in homogeneous coordinates so it holds for the rational curves too). Distance to a chord
segment is convex, so its maximum over a hull is attained at a hull point — that is what
makes these BOUNDS and not estimates, the distinction a prior attempt got wrong by using
each segment's MIDPOINT deviation as if it bounded the span, measured at a 22195x
underestimate. Two segments whose chords approach within the SUM of their bounds may
have the true curves crossing between them, twice, so the chords need not cross and
nothing is recorded; where no contact the sampled map made (`sampledContacts`) sits
within that same band of the approach, the pair is flagged. The window is the BAND, not
one chord — a chord window is what `sampledCrossingsExplained`/`sampledRepresents` use
to locate a contact the kernel already found, and used here it forgives a grazing lens
narrower than one chord (measured: 30 of 45 hider × partner combinations left unflagged
with a wrong region count). Scoped to pairs with at least one free-form source, so an
all-analytic scene is untouched and pays nothing. **The guard sets a FLAG and nothing
else** — region counts, areas and every reported parameter range are byte-identical to
what the same geometry produced before it (measured across 900 mixed-family scenes).
That flag-only PROPERTY is enforced by
`TestNearMissGuardLeavesCorrectAnswersAlone` and `TestProfilesHiddenCrossingIsInvalid`;
what no fixture re-derives is the SAMPLE SIZE those two numbers report. **This file's
two words for that distinction are load-bearing and used consistently**: *measured*
marks a one-off observation nothing re-runs, *pins* marks a named test that enforces
the claim. No figure anywhere in this file or under `docs/` carries a date, so a lone
dated one would read as the only current number among a dozen stale ones.

### What the near-miss guard does not answer

**What it does not answer**: whether the recorded crossing COUNT is right. A lens
narrower than the band whose chords DO cross is explained by its own crossing and stays
silent, so a sub-sample cap is still unflagged where the chords meet — 27% of wrong
region counts are flagged over that matrix. Requiring the explaining contact to be
RESOLVED as well (`contactsResolved`'s rule, transposed to the sampled path) covers all
of them and was measured to cost an 8.5% false-flag rate on ordinary geometry, so it was
not taken.

### What joins a component

**What joins a component is a CONTACT, never a classification**: a handled pair with
NO event means the kernel looked and found the two sources never meet, so unioning on
the presence of its `a.events` key alone collapsed every analytic source in the scene
into one component and let a single refused crossing withdraw exactness scene-wide.

### Regression tests

Regressions: `TestAnalyticCurveCrossingNeverBlessedWrong` (blessed ⇒ correct over the
band), `TestAnalyticArcEndpointOnCrossingCircleKeepsRegion` (the vanished disk, at
both the `t=0` and the `t=1` endpoint),
`TestAnalyticNearSampleVertexCrossingNotBlessedExact` (an exact bound is a domain end
or a closed-form crossing parameter, never a sample fraction),
`TestAnalyticCrossingAtSampleVertexBlessedExact` (its converse: a contact that IS a
sample vertex keeps its exact bound),
`TestAnalyticFusedCurveCrossingNotBlessedExact` (three circles whose third crossing is
below the sampling: an all-exact arrangement must be the converged one, at the adaptive
default density too),
`TestAnalyticFusedComponentLeavesUntouchedClusterExact` (a cluster 100 units away keeps
its exactness), `TestFreeFormSourceWithholdsExactBoundsSceneWide` (the scene gate: an
ellipse clipping one of two certified circles, the same scene at a density that resolves
the clip, one parked curve of every free-form family, and the all-analytic control that
keeps its exact bounds), `TestBoundaryEdgeExactInvariantFreeform` (the same gate over
each family's own whole edge),
`TestAnalyticCurveCrossingEndpointOnChordNotCertified` and
`TestAnalyticCurveCrossingAtSampleVertexNotCertified` (the two shapes of contact a
segment-end band waved through, each publishing one region where the sampled map has
two), `TestAnalyticContactAtVertexBandIsChordLocal` (the same pair reaches the same
verdict with and without a distant unrelated line),
`TestAnalyticShallowCrossingCertificateIsSceneIndependent` (a shallow crossing beside a
sample vertex reaches the same verdict, region count and areas with no line, a line 60
units away and one 200 away, and those areas are the converged ones),
`TestAnalyticEndpointOnChordRefusalSurvivesSceneInflation` (its converse: the
endpoint-on-chord refusal is untouched by a line 10000 units away) and the internal
`TestPolylinesMeetOnlyAtContacts` (the incidence predicate itself, including the
collinear overlap `segParams` never reports and a contact at a closed source's seam).

### Coincident-carrier overlap resolution

**Coincident-CARRIER overlaps (same center, same radius — e.g. a gear tooth's root
arc lying exactly on its hub circle) are RESOLVED, not flagged, when at least one
operand is a PARTIAL arc and the overlap is a single contiguous angular window**
(design in `docs/coincident-carrier-resolution-design.md`): both sources are cut at
the window's two boundary points (exact — each is one operand's own domain end or
the other's) and the LOSING (higher-indexed) source's edges over that window are
suppressed in `split()` in favor of the NAMED (lower-indexed) source, which —
because `Regions` indexes every open curve before every closed one — is the arc
whenever the pair is arc-vs-full-circle. **Classification and resolution have
different gates, and conflating them is the trap.** A pair is CLASSIFIED
coincident by the existing scale-relative `tangentCertify`/`tangentBand` bands,
reused unchanged; it is admitted for RESOLUTION only by `carriersIdentical`, which
bounds `centerDistance + |Δr|` by TWO bands at once — `weldIdentEps·scale` (the scene
half of the identity band `vertexCertifies` uses) AND the carrier-local
`weldIdentEps·max(r_a, r_b)`. That is load-bearing, not belt-and-braces:
`resolveCoincidentOverlap` computes both boundary points on ONE operand's carrier
and stamps the cuts `exact:true` on BOTH sources, so the certify band (three orders
looser) would place a cut off both true carriers and no downstream check would
catch it — `vertexCertifies` compares the graph vertex against the cut's own stored
point, which is where the cut put it. **The carrier-local band is what keeps the gate
about the PAIR**: `scale` is the whole scene's bbox extent, so an unrelated object
far away inflates it (a scene reaching `x=1e15` gives a global band of `1e3`), and on
the global band alone an `r=2` arc and an `r=1` circle read identical and a
suppression window is recorded over the whole circle carrier — a resolution reached
with no carrier near any other. The
centre separation is the quantity under test, so it enters the offset, never the
tolerance. Also unconditionally `Degenerate`: a
coincident LINE carrier; a multi-window overlap (`coincidentArcOverlap` reports only
the longest window, a limit inherited rather than fixed, so it refuses on ANY second
window of positive length — tested against zero, never against `arcParamEps`, since
only ONE suppression window is recorded and a dismissed second span would be emitted
twice with no flag left to warn); and two fully-coincident COMPLETE carriers, where
"complete" is the GEOMETRIC question `operand.coversFullTurn` asks, never the
`fullCircle` FLAG — `wrapSweep` maps a non-positive delta to a full turn, so
`NewArc(c, p, p)` is a 2π arc that the flag calls partial, and keyed on the flag it
paired with a real circle and suppressed the whole circle carrier. **Recording a
window is a CLAIM, and it is settled by a POSTCONDITION rather than predicted by a
precondition** — `certifySuppression`, which runs inside `split()` after
`splitFragments` has deduped every tiny segment's boundaries and canonicalized the
survivors, with every cut on every segment in hand. Each window's two boundary points
must resolve to a graph vertex bounding a fragment of the LOSING source AND one of the
NAMED source — the SAME vertex on both, and the two DISTINCT — or the window is
WITHDRAWN and the pair flagged `Degenerate` from there. Identity is by VERTEX, never
by distance; the merge tolerance only locates which vertex a point belongs to. The
prediction is what kept failing, by a different route each time: `applyAnalyticCut`
records nothing when `cutSite` judges — in PARAMETER space — that a vertex is already
there, and `split`'s per-segment dedup then drops any boundary a COMPETING cut from an
unrelated pair lands within `segEps` of, a global operation over that segment that no
per-boundary check can see. Withdrawing is safe in the way suppressing against an
absent boundary is not: a withdrawal is a `Degenerate` flag, while the suppression
deleted the hair that closes the region and reported `Degenerate=false` with no region
at all. The window's two
BOUNDARY points are this event's contact points (`eventContacts`), so the weld audit
`auditMergedEndpoints`/`eventExplains` must read them: `xEvent.x/y` is only the
window MIDPOINT, a locator for a degeneracy flag and never a cut site, and answering
the audit from it alone tainted the exact cuts the resolution had just made whenever
an overlap boundary welded onto a sample vertex (an everyday alignment — a full
carrier's sample vertices sit at every `2π/spt`), reporting `TExact=false` on the
merged fragment. The three-part crossing-consistency gate above is exempted for a
resolved-overlap pair (`isOverlapPair` in `analyticPrepass`): the two sources'
sampled polylines cross each other constantly along the whole shared arc, an
artifact of the coincidence itself that the gate was never built to judge — the
resolution's own soundness argument is sampling-density-independent (`split`
suppresses by the recorded ANGULAR WINDOW about the shared centre, not by segment
count) and does not depend on that gate at all. **That window is tested EXACTLY, at
the source EVALUATED at a fragment's PARAMETER midpoint and with NO outward slop**:
a window that survives certification has two boundaries that ARE emitted fragment
bounds, so no emitted fragment straddles one and one interior point answers for the
whole fragment. The fragment's CHORD midpoint is not
that point — `densify` floors a source at two tiny segments, so a coincident circle
at a low `WithSegmentsPerTurn` is two semicircle fragments whose chords are
diameters, putting each chord midpoint ON the carrier centre, where the window's
angle test reads `atan2(0,0)=0`; both halves then read as angle 0, both were
suppressed, and the disk vanished. Meanwhile a fragment OUTSIDE the window can sit arbitrarily close to
it, since the losing source's gap beyond the overlap is a real span of any width and
is the only thing left to close a region when the overlap covers nearly the whole
carrier. An outward slop of `arcParamEps` deleted exactly that fragment for every gap
up to twice it, and the region vanished with `Degenerate=false`.
