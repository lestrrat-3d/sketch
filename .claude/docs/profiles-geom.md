# Profiles and the `geom` package

Detail moved out of CLAUDE.md. Read before touching `Sketch.Profiles`, `BoundaryEdge` exactness, or anything under `geom/` — especially the arrangement engine.

## Question router

| Your question | Section |
|---|---|
| When is a `BoundaryEdge` range exact? | The whole-sketch gate comes first |
| Why did exactness disappear scene-wide? | The whole-scene kind gate `exactAllowed` |
| When is a profile invalid or stale? | `Valid` is per-region / A profile is a snapshot |
| Where does OPEN geometry go? | `chains.go` — open boundary chains |
| Why did my chain get cut in two? | Where a chain walk stops |
| What decides the order chains come back in? | Direction, order, length and validity |
| What does `geom.Regions` return? | The planar-arrangement / region engine |
| Which crossings are analytic? | Analytic crossing detection |
| Why was a clean crossing refused? | Curve/curve transverse crossing authority |
| Why is a region flagged degenerate? | Chord-deviation degeneracy bounds |
| Why did a huge but finite scene read degenerate, or publish `Area=+Inf`? | The magnitude screen |
| Two curves lying on the same carrier? | Coincident-carrier overlap resolution |
| Why did one drawing publish different regions in a different authoring order? | The canonical weld order |
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
(`TestExactBoundIdentityBandIsSourceLocal`). The vertex it compares against is the one
the canonical weld order put there — see the next section.

### The canonical weld order

**`splitFragments` canonicalizes every boundary point of every tiny segment in ONE
order — `canonPointCompare`, lexicographic by `(x, y)` VALUE with the coordinates' raw
bit patterns as the final tie-break — before it builds a single fragment**, so for a
given deduped boundary-point multiset the welded vertex table — each vertex's
coordinates and its id — is a property of those points alone, not of the order the
curves were passed in. It does NOT make that multiset order-independent; the cut set
upstream still is not (see "It orders the WELD only." below). `vertexTable.canon` is unchanged and still decides identity by
distance: it welds a point onto the first vertex within `a.merge` of it and keeps that
vertex's coordinates, so the FIRST member of a near-coincident cluster to arrive
represents it. Feeding canon in segment order made that member whichever curve the caller
drew first.

**What makes it work is that the order is TOTAL**, not merely that it is a sort: the
sort is unstable, so the relative order of any pair the comparator leaves tied is the
sorter's to choose, and that choice can still depend on the collection order — the
authoring order the pre-pass exists to remove. The bit tie-break is what
closes that, and it is needed for exactly one KIND of pair among the coordinates that
can reach this sort — a negative zero against a positive zero, which `==` reports equal
on both `x` and `y`; every other pair the sort can see is separated by `<` on one coordinate or
the other. A half
disk whose elliptical arc starts at `(-0, -0)` and whose closing line ends at `(+0, +0)`
published its shared vertex with bits `0x8000000000000000` drawn one way and `0x0` drawn
the other, at one region and the same area either way — which is what
`TestWeldRepresentativeBitsMatchEveryOrder` asserts on, BITWISE, since `-0.0 == +0.0`
hides the difference from an ordinary equality assertion. **Reaching the vertex table
with the sign intact is what a reproduction has to arrange**, and that is why the scene
uses an elliptical arc: an arc pins its ends to the authored Start/End, so the
coordinate arrives verbatim, while a line's is recomputed as `ax + t·(bx-ax)` and at
`t=0` keeps a negative zero only when the direction component is itself negative
(`-0 + -0 = -0`), losing it when the line runs the other way (`-0 + +0 = +0`). A line
scene reproduces the defect on that direction and not on the other, so a test built from
lines can pass against the untied comparator and prove nothing.

**The cost was confined to the sign bit, but it was not invisible to a caller.** Region
count, `Degenerate`, `SelfIntersecting`, `Area`, the edge count, `Reversed`, `Whole`,
`TStart`/`TEnd`/`TExact` and every other coordinate were identical between the two
orders. A consumer reading the coordinate as a number saw no difference — but `%v` and
`strconv` render `-0`, so a golden file, a hash or any byte-exact comparison flipped with
authoring order, and an `atan2` or a division on that coordinate came out with the
opposite sign.

`NaN` is the other value `<` cannot order, and it cannot reach this sort: `densify`
drops any source with a non-finite evaluated sample as `srcDegenerate` before it emits a
tiny segment, and every boundary point is one of those segments' endpoints, a bounded
affine combination of two of them (`segParams` confines its hit to the chords), or a
closed-form intersection of sources that survived that screen. `cmp.Compare` puts a
`NaN` ahead of every number but reports two `NaN`s equal, so the raw-bit tie-break —
not `cmp.Compare` — is what would separate distinct payloads; the comparator is total without resting on the
screen. The bits are consulted ONLY after both value compares tie, so every pair the
value compare already ordered keeps that order — the tie-break decides the ±0 pair and
nothing else, and among points whose coordinates compare equal the cluster publishes the
one with the smallest raw bits, `x` first then `y`, so `+0` wins on `x`, and on `y` only
when `x` already ties; a cluster of `(-0, +0)` and `(+0, -0)` publishes `(+0, -0)`.

**A cluster whose span EXCEEDS `merge` is where the order shows.** Three curve endpoints
`0.9e-6` apart on a scene 10 units across (where the default tolerance is `1e-6`) weld
pairwise but not end to end, so the representative decides whether the third curve joins
the map or dangles and is pruned: a triangle with a spoke to each of those three points
published 2 regions in one authoring order and 3 in another, with `Degenerate` false and
`ProfilesValid` true in both, so nothing flagged the disagreement. Pinned by
`TestWeldOfWideClusterMatchesEveryOrder` (through `geom.Regions`) and
`TestProfilesOfWideClusterMatchEveryOrder` (through `Sketch.Profiles()`), each over all
six orderings of the three clustered curves.

**The weld keeps its bound: every point welded into a vertex lies within `merge` of that
vertex's coordinates.** Only the order canon sees changed, never its rule. Two pieces of
the exactness machinery are written against that bound and keep their arguments verbatim
— `boundVertexAt`'s `d > a.merge` reject, and `eventExplains`' soundness argument. A
union-find weld pooling a whole transitive cluster onto one representative drops the
bound, and is NOT what this does.

**It orders the WELD only.** Order dependence upstream of it is by design and untouched:
`intersect`'s pair enumeration, `splitFragments`' keep-the-first cut dedup, and the
coincident-carrier rule that names the lower-indexed source. A permutation can still
change the cut set those produce, and the pre-pass claims nothing about that. **But
"everything left is upstream" holds only while the comparator is TOTAL.** The ±0 tie sat
INSIDE the weld order this section settles, not upstream of it, so a list of accepted
upstream dependences is not by itself an account of what a permutation can still change.
Any change to the ordering key owes that question again: is any pair of DISTINCT points
left for the unstable sort to decide? What the pre-pass
also does not answer is whether a cluster spanning more than `merge` should weld at all
— that case has no correct answer, and the canonical order makes the verdict repeatable
rather than right. No flag reports it.

## `chains.go` — open boundary chains

### Overview

`Sketch.Chains()` is the arrangement's OPEN publication, beside `Profiles()`:
ordered open runs of `BoundaryEdge` (`sketch.Chain`, built from `geom.Chain` in
`geom/chain.go`) over the edges no published region boundary uses. One
`buildProfiles` call runs one arrangement and returns both, so nothing arranges
twice and no edge is a region-boundary edge AND a chain edge. Every edge of an
OPEN run is one or the other; the only edge set neither publishes is the closed
run described below. The candidate set is DERIVED, not named by the caller:
`prune()` keeps the edges it drops in `arranger.pruned`
(spurs and open trees), `extract` marks the cycles it actually publishes through
`arranger.cycleOf`, and the chain pass takes the complement. Construction
geometry is excluded exactly as it is from `Profiles()`; reference geometry
participates.

### Where a chain walk stops

The walk is cut at every vertex whose degree over the WHOLE arrangement is not 2
— the pruned edges, the region-boundary edges and the candidates all counted.
Counting only the candidates would let a chain turn a corner at a crossing whose
other branches went to a region, publishing a walk through a point the
arrangement had already resolved as a branch. So three lines meeting at a point
are three chains, and two crossing lines are four. A run that closes back on its
own start vertex is published as NO chain, and so is a component whose every
vertex has degree 2: a `Chain` is open by definition, a closed loop that bounds
anything is a `Profile`, and one that bounds nothing (its area under `extract`'s
local cycle floor) is already absent from the region set, so publishing nothing loses
nothing the region pass was reporting.

### One edge vocabulary, one coalescing rule

`boundaryFrag`/`appendBoundaryFrag`/`boundaryEdgeOf` (in `geom/arrange.go`) are
shared by `makeCycle` and the chain walk, so `Whole`/`Reversed`/`TStart`/`TEnd`/
`TExact` mean the same thing in both publications BY CONSTRUCTION rather than by
two implementations agreeing. Every rule stated above for a region boundary's
edge — the whole-sketch kind gate, the per-bound provenance behind `Whole`, the
fused-map withdrawal — applies unchanged to a chain edge.

### What `Verify` does with them

`Sketch.Verify` reports the chains it already computed (`VerificationReport.
Chains`/`InvalidChains`, from the same `buildProfiles` call) and asserts nothing
about them: `Check`/`Trustworthy()` gain no chain condition. See
`.claude/docs/diagnostics.md` → "`Chains`/`InvalidChains` are reported, never
asserted" for why, which is the authority on that decision.

### Direction, order, length and validity

A chain with two free ends admits two walks, so the published one starts at the
lexicographically smaller end point (`canonicalChainDirection`), and the chains
are ordered by start point, then end point, then the whole walk
(`chainLess`). Both are stated in COORDINATES, never in entity order — a promise
about the SET published and each chain's own walk, never about a chain's INDEX
(a consumer compares a held chain against a fresh one by content or by handle),
and a promise about what the chain layer ADDS, not about the arrangement it
ranks. Where the arrangement reads source position — `resolveCoincidentOverlap` naming the
lower-indexed of two coincident-carrier sources for their shared span, and the cut set
upstream of the weld (`intersect`'s pair enumeration and `segBoundaries`' keep-the-first
dedup) — a chain inherits that choice exactly as a region boundary does. The weld itself is
not one of them: `splitFragments` canonicalizes every boundary point by `canonPointCompare`
before `vertexTable.canon` sees it, so for a fixed boundary-point multiset the vertex table
is the same however the curves were authored, and only a moved cut set can still move the
weld. These predate chains and are shared with `Sketch.Profiles`; a source-position-free
rule for the carrier naming is a `geom` change and lands on both publications at once.

`chainLess` is PARTIAL on purpose, and the tie it leaves is settled one layer
up. Two chains walking the identical polyline — coincident duplicate geometry,
already reported `Degenerate` — compare equal in every coordinate there is, and
`geom` has no identity beyond coordinates to rank them by, so its stable sort
leaves them in `SourceIndex` order. `Sketch.Chains` closes that: `orderChains`
(`chains.go`) sorts the published list with **`compareChains`, the ONE ordered
comparison over a published chain**. It asks the coordinates first, by
`chainLess`'s own rule, so it REFINES the arrangement's order and never disturbs
it, and then ranks on every other source-independent property the chain
publishes, in the fixed precedence `compareChains` itself states — read it there
rather than from a second copy here.

Two of those rungs consume NAMES (an entity's, and those of the points it is
defined from), and they are consumed for ORDER only, never for content: nothing
a `Chain` publishes is derived from a name, and `Sketch.Revision` hashes none of
them. Renaming therefore re-ranks the published list while every held `Chain`
stays fresh — see "Load-bearing rule — hash what is consumed and handed out" in
`.claude/docs/sketch-core.md`, which owns that scoping.

Entity identity is deliberately not consulted: the `Entity` pointer, and the
entity id behind it, ARE the authoring order. Two chains equal on every rung stay
tied and are interchangeable — nothing published about them differs, which is
what makes that claim true rather than merely stated.

A property added to `Chain` or `BoundaryEdge` belongs on a rung of its own in
`compareChains`; that is the single place this layer ranks a chain. A new
COORDINATE rule is the other half and lives in `geom/chain.go`'s `chainLess`, so
changing either means reading both — the two compose into the published order.

`Length` is closed-form for a
line/arc/circle fragment and otherwise the chord sum of the SOURCE sampled over
the reported range, on `densify`'s own parameter grid with the fragment's bounds
pinned (a convergent underestimate), published whatever `TExact` says, the same
way `Region.Area` is. Neither branch measures the emitted polyline: a fragment
end is a welded vertex up to the merge distance off its own curve, so a chord
drawn to it would make `Length` OVERestimate by that much. `Length` and
`Polyline` are allowed to disagree by the merge distance for exactly that
reason. `Degenerate` reuses `degenReaches`, the attribution rule
`Region.Degenerate` uses. `SelfIntersecting` is measured on the chain's own
emitted polyline (a collinear reversal at a joint, or any non-adjacent pair of
its chords meeting within the arrangement's merge distance); a self-crossing the
arrangement RESOLVED never reaches it, since that is a degree-4 vertex the walk
is cut at.

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
one), the per-kind extent guards in `newArranger` (a source that has collapsed
to a point rather than a curve: a line whose endpoints coincide, a conic whose
start, apex and end span no extent, or an all-coincident control/fit-point set —
each screened by `splineExtent`, the bounding-box DIAGONAL over that source's
whole defining point set, against the same absolute `1e-9`, since the scene
scale is not known until `densify` has run, and each recording an UNATTRIBUTABLE
degeneracy, because the curve is dropped before it can form an edge; an arc or
elliptical arc with `Start == End` is NOT this case, it sweeps a full turn),
`densify`'s `finitePt` as the last net over every evaluated sample (see below),
and past it the MAGNITUDE screen over what is computed FROM finite samples — the
scene extent, the area floor, every cycle area and every chain `Length` (see
"The magnitude screen" below).

That extent is measured over the SET, never as each point's distance from the
first. A conic whose apex and end straddle its start by `0.9e-9` each has both
inside that distance while the set spans `1.8e-9`, and a per-point screen is not
even monotonic in the extent it claims to measure: it admits a set spanning
`1.01e-9` and refuses one spanning `1.9e-9`. The error runs one way only —
`extent < 1e-9` implies every start-relative distance is under `1e-9` too — so a
per-point screen lets nothing degenerate through; what it costs is a FALSE
degeneracy, which invalidates every region in the arrangement
(`TestRegionsZeroExtentLineAndConicDegenerate`).

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

### The magnitude screen

**Finite samples can still overflow what is computed from them, and `finitePt`
never sees that**: every input coordinate finite, and an accumulated area,
length or scene extent past float64. Three sites past `finitePt` flag it, each
as an UNATTRIBUTABLE degeneracy (`flagDegenerate(0, 0)`, so by `degenReaches` it
reaches every region and every chain). `densify` flags a scene extent that is
`+Inf`; finite samples can overflow the bounding-box subtraction. The merge
tolerance and identity bands depend on that scale, so it withholds exact bounds
scene-wide and KEEPS the substitute scale of 1 for later passes. `extract`
flags when `scale²·1e-12` is `+Inf` as a scene-wide magnitude screen: past a
scene extent of about `1.34e154`, crossing determinants can also overflow.
It flags any cycle whose own area is NaN or infinite, and any cycle whose LOCAL
area floor is `+Inf`. When the scene extent already overflowed and `densify`
substituted scale=1, `extract` uses that fallback floor to keep publishing an
infinite-area face with the flag. `walkChain` flags a chain whose accumulated
`Length` is non-finite (a `(0,0)→(1.7e308,1.7e308)` line overflows inside
`fragLength`'s hypot with the scene extent still FINITE). `extract` re-stamps
`Arrangement.Degenerate`, every `Region.Degenerate` and every
`Chain.Degenerate` after `buildChains` when the walk raised one.

**Classify each cycle with its own area floor**: `cycleAreaFloor` uses the cycle's
sampled bounding-box extent, plus the full sampled extent of curved sources on
its boundary, squared and multiplied by `1e-12`. A curved source's extent keeps
the threshold above sampling slivers on its small fragments; a straight source
contributes only its boundary fragment. Hole containment uses the larger of the
face and hole floors. An unrelated open line at `x=1e10` cannot remove two
triangle regions near the origin (`TestRegionsDistantOpenLineDoesNotHideLocalFaces`).
A line at `x=1e7` cannot suppress a nested square's hole
(`TestRegionsDistantOpenLineDoesNotHideHole`). The existing curved-source sliver
control is `TestNearMissHiddenCrossingIsDegenerate`. The scene-wide magnitude
screen sets a flag only; it does not raise any cycle's classification floor.

**The accepted cost is the scene-wide magnitude band**: a scene whose extent
exceeds about `1.34e154` reads degenerate even where its own published lengths
are finite — two lines at `8e307` publish `Length=1.6e308` with
`Degenerate=true`. `segParams` decides sampled contacts on
`d1x·d2y − d1y·d2x`, a product of two chord lengths that can overflow in that
band. Pinned by `TestRegionsOverflowedExtentIsDegenerate`,
`TestRegionsOverflowedAreaFloorIsDegenerate` and
`TestChainsOverflowedLengthIsDegenerate` (`geom`, the last also pinning the
accepted cost), and at the sketch layer by `TestProfilesNonFiniteMagnitudeIsInvalid`
and `TestChainsOverflowedLengthIsInvalid` (`ProfilesValid=false`,
`ErrInvalidProfile`, never `ErrNonFiniteGeometry` — the geometry IS finite); the
control `TestRegionsLargeFiniteSceneIsUnchanged` holds a `1e154` triangle and a
`1e154 × 1e150` rectangle at their finite areas with no flag.

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
one cache instead of each computing its own boxes. `sourceBoxCache` also unions those
cached boxes per source for the source-pair reject below.

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
this reason. First, a source pair is skipped when the unions of its segments'
`segBoxCache` boxes fail `boxesOverlap`; disjoint unions prove that no contained
segment-box pair overlaps. Otherwise, a segment pair is skipped only when its two
cached boxes (each already expanded by `segReach` =
`a.merge + broadPhaseRel·(chord length)`) fail `boxesOverlap`.
`segsCrossInteriorAt`'s positive set (an interior hit, `ti`/`tj` strictly inside
`(segEps, 1-segEps)`) is a subset of plain `segParams`' positive set (endpoints
included), and `TestBroadPhaseIsSuperset` already proves those reach-expanded boxes
are a superset of everything `segParams` accepts — so they are necessarily also a
superset of `segsCrossInteriorAt`'s stricter interior-only set, with no new constant
and no new proof needed.
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
flagged. A vertex reaches this ordering only when its WHOLE ring is exact, and then
through one of two doors: a certified contact, or a ring where two chord departure
angles are EQUAL as `float64` while their exact tangent rays genuinely differ. At a
sampled crossing vertex the edges are chords, so chord ordering is what matches the
traversed geometry, and neither door admits one.

**The second door is what an arc fragment with no interior sample vertex needs.**
Such a fragment emits ONE edge between its two graph vertices, so its chord IS a
straight edge between the same two vertices and both depart at a bit-identical
angle. The rotation sort's fallback is `sort.Slice` on that angle, which is unstable
and cannot separate them; at one of the two vertices it orders the arc ahead of the
chord where counter-clockwise order needs the reverse, the `next` pointers stop being
a planar embedding, and the face walk returns one near-zero-area cycle over every
half-edge instead of the faces. Every bounded region disappears with `Degenerate`
false — `extract` has no postcondition on its own output, so nothing catches it.
`TestArcSpanningOneChordKeepsItsFaces` and `TestSectorPairRegionsMatchEitherOrder`
pin the two shapes it took.

**A STRAIGHT port is keyed by the chord it EMITS, never by its source direction, and
that — not the door's gate — is what makes exact ordering safe.** `portKey` answers a
`srcLine` with the authored endpoint delta, and welding moves a fragment's ends onto
other vertices, so that delta can name a ray the face walk never traverses. Once a
ring is sorted exactly, EVERY straight port in it is sorted that way, so gating which
ties may OPEN the door never protected the straight ports inside an already-open one.
Two rounds of review found faces lost that way: first a rectangle collapsing to no
region at all, then a mixed tie of one arc and one welded line losing a `1.33e-12`
face while keeping its neighbour. Both are pinned in `geom/arrange_chordtie_test.go`.
**A CURVED port is keyed the same way, by a direction anchored at its graph vertex.**
`portKey`'s exact tangent is taken at the PARAMETRIC endpoint, and welding moves the
vertex off that point, so the tangent describes a departure from somewhere the walk
no longer visits. The failure has a threshold rather than being incidental: a fragment
of chord length `L` on radius `r`, welded by `d`, crosses to the wrong side of the
chord it shares once `L < sqrt(2*r*d)`. `curvedPortDir` therefore aims from the vertex
at the fragment's own parametric midpoint. That needs no bound: both ends of an edge
aim at the SAME point from opposite sides, so their rays are mirrored however the
weld moved them, and `splitFragments` emits at most one edge per sampler step, so an
edge can never span a whole closed curve — a circle cut once, whose midpoint would be
the antipode, is not reachable.

**The re-key applies at the SECOND door only.** At a certified tangency contact the
ordering rests on every incident exact tangent being ONE ray, so `sortExactPorts` can
cluster them and separate the loops by signed curvature. Midpoint rays do not tie, the
cluster breaks, and an inner tangent arc sorts to the wrong side — an outer circle
with an inner tangent arc published NO regions at all, unflagged, until this was
scoped. Certified contacts keep `portKey`'s tangent, and only the second door, which
is exactly where welding can have moved a vertex off the parametric endpoint, gets the
vertex-anchored ray. `TestInnerTangentArcKeepsBothFaces` pins it.
`TestWeldedArcPortKeepsTheLargeFace` pins a unit-scale scene from a review sweep where
keying only the straight ports dropped a `1.689` face and kept a `1.09e-05` sliver,
unflagged.

One consequence is deliberate and worth expecting. Two straight ports that emit ONE
chord now carry the same tangent ray and equal (zero) curvature, so
`sortExactPorts`' osculation check sees them as adjacent same-ray ports with
indistinguishable curvature and flags the pair — attributed to both sources, since it
passes both. That is the coincident-emitted-edge condition being reported rather than
a new false positive: on a 3000-scene sweep of a box plus welded near-parallel lines,
154 scenes became `Degenerate` that were not before, all of them scenes where two
sources genuinely emit one chord. An ordinary scene is unaffected — a 4000-scene
line/arc/circle sweep with no forced welding is bit-identical to before.

**The door additionally requires at least one of the tied pair to be CURVED.** Two STRAIGHT fragments that emit one chord are the same segment in the
traversed map, so their source tangents say where each line would run rather than
what the face walk walks, and ordering by them corrupts the map — the failure the
scope rule exists to prevent. WELDING is what makes that reachable: it moves a
fragment's endpoints onto other vertices, so a straight fragment's emitted chord stops
matching its own source direction, and two near-parallel lines welded to the same
corners emit one identical chord while keeping different source tangents. Without the
curvature requirement, a 10x10 rectangle plus two such lines at `WithVertexMerge(0.002)`
published 0 regions where main published one of area `100.00053587936401` — the same
collapse this section describes, caused by the repair for it. A curved fragment is the
opposite case: its chord is a secant of a curve departing along another ray, so the
tangent carries the geometry the chord lost.
**A DOUBLED straight pair welded at both ends needs a separate repair, because
ordering cannot fix it: `useExactPorts`/`sortExactPorts` decide a ROTATION ORDER, and
two lines between the same two vertices, separated by less than the merge distance,
are one edge to the map and two to the sources — no order of the tied pair is more
correct than the other, since the map genuinely cannot tell them apart.** The repair
is `dedupCoincidentStraightEdges` (`geom/arrange.go`, run at the top of `buildGraph`):
before the rotation system is built, every arrangement edge is grouped by its
canonical (post-weld) vertex pair, and a group of two or more STRAIGHT (`srcLine`)
edges over the SAME pair is provisionally collapsed to one. The survivor is selected
by a canonical key over the authored line and emitted fragment, both normalized to
ignore source direction; `SourceIndex` breaks the tie only when those geometries are
identical. Reordering distinct source lines therefore retains the same geometric
source and the same complete boundary fields.
Two points determine a line, so any two straight edges sharing both endpoints are
provably the identical traversed chord; dropping the redundant copy removes the tie
rather than trying to order it. Reached through `Sketch.Profiles()`,
`TestProfilesCoincidentEmittedLinesKeepBothProfiles` pins a seed-16 reduction where
the pre-repair engine silently dropped a `12.26`-area profile down to just its
`0.14`-area neighbor, with `Degenerate` false and every published profile still
`Valid`. Measured on the adjudicator's scene B (truth 4 faces), over all nine input
orders: the repair reaches 4 in every order (`TestDoubledPairAnswersEveryOrderAlike`),
where the curvature-only exact-port fix above reached only 3 or 5.

The repair is scoped to a straight/straight tie on purpose. A CURVED fragment's chord
is a secant of a curve departing along a different ray than a straight one sharing its
endpoints, so a curved member of a tied pair carries information a merge would
destroy, and `useExactPorts`' curvature-gated door already orders that case correctly
— `dedupCoincidentStraightEdges` only ever groups edges whose source is `srcLine`, so
a straight/curved or curved/curved tie never enters a group at all. A coincident-
CARRIER pair (an arc on its hub circle) is resolved earlier still, in `split()` via
`resolveCoincidentOverlap`, and never reaches `buildGraph` as two edges over the
shared span. A duplicate SAMPLED fragment (ellipse/spline/conic/NURBS) is untouched
for the same reason `useExactPorts` already leaves it on chord order: it never
qualifies for exact ordering, so the ambiguity this repair targets cannot arise for it
in the first place. A tied group ISOLATED at both its vertices — nothing else in the
arrangement touches either one — is also left alone: that is a fully duplicate open
curve, the same line authored more than once, and `chains.go` deliberately reports it
as one `Chain` PER source rather than one for the cluster (see "coincident duplicate
geometry" there). Collapsing an isolated group would silently drop chains a consumer
already relies on seeing one per curve; the coincident-emitted-edge condition this
repair targets is always EMBEDDED, with some other curve also meeting the pair at one
or both of its shared vertices, which a plain vertex-degree check (equal to the tied
group's own size, or not) tells apart from the isolated case.

**The collapse is withdrawn when its survivor becomes a bridge in the provisional
rotation graph.** `buildGraph` first wires the graph with every eligible group reduced
to its canonical survivor. If both directed half-edges of a survivor belong to the
same face walk, the edge is carrying a boundary passage that the removed parallel
edge completed. The group is restored, its sources are reported `Degenerate`, and the
graph is rewired; this repeats because restoring one group can expose the bridge use
of another. `TestWeldedParallelLinesKeepTheirRegion` pins that the large region is not
silently removed. The flag is required because the retained coincident edges still
have no unique rotation order. `TestWeldedArcAndLineKeepBothFaces` is the non-bridge
control: its canonical survivor keeps both original faces, plus the separate sliver
between them, without a degeneracy.

A SAMPLED source is not covered, since the ring is not all-exact —
there the arc edge genuinely is the chord, so a reorder would be wrong and the
honest verdict is a degeneracy flag, which is tracked separately.

### Internal (containment) tangency

**Internal (containment) tangency is now also blessed (increment 7 §7a):** the
shared contact gets the same exact tangent-port ordering, and hole assignment uses
an **exact point-in-region** test (`exactPointInRegion`: a ray-cast with closed-form
circle/arc crossings, immune to the chord poke-out that defeated the sampled
`pointInPolygon` near the contact), so the inner cycle nests into the outer as an
annulus + inner disk — exact at every sampling, tiny inner included. Line-involved
merged tangency and genuine osculation stay conservatively `Degenerate`. Ellipse/
spline pairs keep the sampled fallback (exact containment falls back to the chord
polygon for them). The ray-cast probe moves by a fraction of the HOLE cycle's
narrower sampled span, and the moved point must remain inside the sampled hole
boundary and its exact boundary when that test is available. A distant source
cannot move it into a disjoint face.
`Sketch.Profiles()` is its consumer.

**The assignment carries a postcondition, not just the probe.** For a hole and
face whose boundaries are line/circle/arc only, `holeLiesInFace` re-derives both
cycles' exact bounding boxes straight from their fragments' closed-form geometry
(`exactFragBounds`/`cycleBounds` — a circle/arc fragment's box also checks
whichever of the four cardinal angles its sweep covers) and requires the hole's
box inside the face's before the assignment is published. A genuinely nested
hole's box is always inside its face's, so this can only ever reject a wrong
assignment, never a real one; a rejection leaves the hole unassigned (as if no
face had contained it) and flags the arrangement `Degenerate`, attributed to
both cycles' sources. It is skipped (nothing to check) once an ellipse/spline is
part of either boundary — the same coverage boundary `exactPointInRegion`
already has.

**Its slack is source-local AND derived, and that is what makes it a check at
all.** The box comparison forgives the sum of the two boxes' own evaluation
round-off, `cycleBoundsRoundoff` of each cycle. That is an ABSOLUTE bound read
off the arithmetic `exactFragBounds` performs — a per-kind ulp count (see
`boundsRoundoffUlpsLine`/`boundsRoundoffUlpsArc`, which carry the derivation)
times the largest defining number across that cycle's OWN fragments (a line's
endpoint coordinates, a circle/arc's centre and radius), which bounds the
magnitude its extrema are evaluated at. Everything above that is a real gap and
is rejected.

The postcondition's local slack stays independent of probe placement. A slack
stated against `a.scale` grew past a real `1.0` gap between two local triangles
when an unrelated line sat at `x=1e9`; a tuned fraction of the local magnitude
still swallowed a `5e-4` gap. `TestHoleLiesInFaceUsesCycleLocalRoundoff` checks
the postcondition directly, including a `1e-6` gap and a real nested hole.
`TestRegionsHoleContainmentSlackIsLocalToTheTwoCycles` and
`TestRegionsHoleContainmentRejectsANearGapSeparation` now check that the local
probe leaves disjoint triangles clean without relying on a rejected assignment.

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

**Input ORDER is a public variable of the resolved case, and the godoc on
`Sketch.Profiles` + `BoundaryEdge.Entity` (`profiles.go`) and on `geom.Regions` +
`geom.BoundaryEdge.SourceIndex` owns the consumer-facing statement of it.** Reordering
one pair decides which of the two is named, and EVERYTHING the report says about that
span follows from the naming. Among the outputs that move are the source index or
entity, `TStart`/`TEnd`, `Whole`/`Partial`, the `Polyline`, how the boundary is cut into
edges, which sources appear on the boundary at all, and the number and area of the
regions; that is a set of examples, not an inventory. The godoc states it as a blanket caveat — treat
the whole report for such a scene as order-dependent, and read the span's source off the
edge rather than looking for a source you expect.

**Do not narrow that into a list of what does and does not move.** Four review rounds
each enumerated the effects and each was falsified by a sharper scene: identical sweeps
leave `TStart`/`TEnd` and `Whole` unmoved, because a bound is a fraction of the named
source's own sweep and equal sweeps make the fraction equal; source presence turns on
whether an unsuppressed remainder survives `prune()` rather than on whether a source
extends outside the overlap; and region count and area both move as well (see the
`fu80`/`fu81` follow-ups, which are pre-existing engine defects rather than anything the
caveat documents). Nothing in the code establishes order-independence as an invariant
anywhere.
`TestAnalyticCoincidentCarrierNamingIsOrderDependent` is a regression pin on two
concrete scenes — two arcs sharing a START, and two arcs `0..160°`/`0..170°` closed by a
chord PAST the short one — and is evidence for those scenes only, never for a universal.
