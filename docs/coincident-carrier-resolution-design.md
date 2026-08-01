# Coincident Carrier Resolution — Design

Status: **implemented** (`geom/arrange_events.go`'s coincident-carrier branch +
`geom/arrange.go`'s `resolveCoincidentOverlap`/`split` suppression). Companion to
`docs/analytic-arrangement-design.md` — that document's §7b covers the sibling
curve/curve crossing lift; this one is the "new arrangement semantics" the
crossing lift does not touch (source splitting at overlap ends, edge
deduplication, a `SourceIndex` decision, area accounting across a merged
edge), scoped to its own file per that document's own sizing note. Resolves
ask 2 of `.tmp/decad-2d-region-asks/README.md` (a downstream consumer's
request; the note itself is not committed to this repository).

## The problem

Two arrangement sources with the same circle/arc carrier (same center, same
radius) are classified `evOverlap` and unconditionally `flagDegenerate`d
(the `evOverlap` arm of `analyticPrepass`, `geom/arrange.go`, fed by
`circleCircleEvents`'s coincident-carrier branch and `coincidentArcOverlap`,
`geom/arrange_events.go`).
This is not a rare input: it is the **normal case** for a gear tooth, whose
root arc is by construction an arc of the hub circle it sits on — every tooth
in a 12–45-tooth gear repeats it. `TestAnalyticSameCarrierArcs`
(`geom/arrange_analytic_test.go`) already exercises the same code path
for two arcs sharing a carrier; the flag is correct there too, for the same
reason.

The flag is not merely conservative noise: `geom.Regions` still returns region
structures for a sketch containing a coincident-carrier pair — with plausible
areas, even `TExact = true` edges — but the region set does not describe the
drawn shape. Probe case C (`.tmp/decad-2d-region-asks/probe/main.go`, a root
arc exactly on a hub circle closed by two flank lines and a tip line) is the
concrete demonstration:

    region 0 area=314.159265 outer: [src=0 whole t=0..1] [src=4 t=0.0477..0.9523]
    region 1 area=11.864262  outer: [src=1 whole] [src=4 t=0..0.0477] [src=4 t=0.9523..1] [src=3 whole] [src=2 whole]

Both regions' *areas* are already the physically correct values (314.159265 =
π·10² for the hub disk, 11.864262 matching the tooth area probe case E reports
when the same tooth is built without a coincident root arc at all) — each
walked loop closes on itself and integrates correctly, because the two
coincident sources are, pointwise, the same curve. What is unsound is the
**boundary attribution**: region 0's loop uses `src=0` (the root arc) for the
tooth's angular span and `src=4` (the hub circle) for the rest, while region
1's loop uses `src=4` fragments for that SAME angular span instead of `src=0`.
The two regions do not share a common, consistently-named edge — each
independently re-derives the coincident span from a DIFFERENT source, so a
consumer mapping `SourceIndex` back to its own entity (`Sketch.Profiles()`'s
`entityFor`, `profiles.go:279-284`) sees the root arc attributed to the hub
region and hub-circle fragments attributed to the tooth region. Only the
arrangement-wide `Degenerate` flag — which the resolution below still leaves
in place for the genuinely unresolvable cases — stands between a consumer and
that wrong attribution today.

## Scope

This design resolves a coincident pair where **at least one source is an arc**
(has its own finite domain ends) — circle/arc and arc/arc coincidence. This is
what `coincidentArcOverlap` already computes a finite, positive-length overlap
window for, and it is exactly the gear's shape (a root arc on a full hub
circle). Two explicitly out-of-scope cases, both left flagged `Degenerate`
exactly as today:

- **Two fully-coincident COMPLETE carriers** (both sources cover the whole
  turn, so neither contributes a finite overlap boundary — the "overlap" is the
  entire circle). This is a duplicate-entity degeneracy with no natural cut
  points; resolving it is a distinct, smaller problem (drop one full duplicate,
  replicate any THIRD-party cut from one carrier onto the other) that the gear
  workload never exercises and this design does not attempt. **The test is
  geometric, never the `fullCircle` flag** (`operand.coversFullTurn`): `geom`'s
  `wrapSweep` maps a non-positive angular delta to a full turn, so
  `NewArc(c, p, p)` is a 2π arc — a complete carrier the flag, set only for a
  `srcCircle`, calls partial. Keyed on the flag, such an arc pairs with a real
  circle, resolves, and suppresses the whole circle carrier.
- **Coincident line carriers** (`lineLineEvents`'s overlap branch, gated by
  `mergeEps` rather than `tangentCertify`/`tangentBand`). A different code
  path, a different tolerance, and not what the note's gear workload needs;
  left for a future design if a consumer asks for it.
- **A pair whose sweeps overlap in more than one disjoint angular window.**
  `coincidentArcOverlap` reports only the single longest contiguous overlap — a
  pre-existing scope limit this design inherits rather than fixes — so it
  refuses a pair with **any** second window of positive length. The test is
  against zero, not against `arcParamEps`: resolution records exactly ONE
  suppression window, so a second span dismissed as sub-epsilon would still be
  emitted by both sources as an un-deduplicated coincident boundary, and the
  `Degenerate` flag that used to warn the consumer would be gone. (Dropping a
  sub-epsilon overlap remains sound for the WHOLE overlap, where nothing is
  resolved and nothing is suppressed — that is the disjoint / endpoint-touch
  case.) The gear workload has exactly one overlap window per pair by
  construction (one root arc, one hub circle).
- **A carrier match that holds only within the classification band.** See "The
  refusal band" below: classification as coincident is scale-relative to
  `tangentCertify`, but RESOLUTION additionally requires the two carriers to be
  the same curve at round-off.
- **An overlap boundary the cut phase cannot materialize as a split.** The
  suppression in step 4 is only sound while every emitted fragment lies wholly
  inside the window or wholly outside it, which needs both boundaries to be
  boundaries of the losing source's own fragments. `applyAnalyticCut` records
  nothing when `cutSite` judges — in PARAMETER space — that a vertex is already
  there, so at coarse sampling a boundary can sit a whole vertex-merge tolerance
  away from the nearest sample vertex and still split nothing. Step 2 verifies
  both boundaries rather than assuming them, and a pair failing that check is
  refused like any other out-of-scope overlap.

## Mechanism

### Detection (classification unchanged, resolution gated tighter)

`circleCircleEvents`'s coincident-carrier branch already certifies "same
center, same radius" scale-relatively (`d <= certify && |a.r-b.r| <= certify`,
`certify = scale*tangentCertify`) and `coincidentArcOverlap` already confirms
the two operands' swept angular ranges actually intersect with positive
length, returning `over=false` for disjoint or endpoint-only sweeps (a normal
join, not a degeneracy — `TestAnalyticSameCarrierArcs` pins this). Nothing
here changes that CLASSIFICATION; the design changes what `analyticPrepass`
does with a positive-length overlap.

Resolution is gated more tightly than classification, by `carriersIdentical`:
the two carriers must be the same curve **at round-off**, not merely within the
classification band. See "The refusal band" for why, and for the exact bound.

### Resolution

1. **Extend `coincidentArcOverlap`** (or add a sibling) to return the overlap
   window's two boundary points, not just the representative midpoint it
   returns today — `bestMid ± bestLen/2` in the shared angular frame, mapped
   to `(x,y)` on the shared carrier exactly as the existing midpoint is. Each
   boundary angle is, by construction, either one operand's own domain end (an
   arc's `phi0` or `phi0+sweep`) or the other operand's — never an iteratively
   solved root, matching the note's own framing ("exact — they are the arc's
   own domain ends, interior parameters on the circle").
2. **Cut both sources at both boundary points** via the existing
   `applyAnalyticCut(src, t, x, y)` (`geom/arrange.go`), using each
   operand's own `circleParam(x,y)` for `t` — unmodified; a boundary that
   lands on a source's own domain end is already a no-op through the existing
   `cutSite`/`atSourceEnd` check, exactly as an ordinary `evCross` at a shared
   endpoint is today.

   **First VERIFY that all four cuts materialize** (`overlapBoundariesSplit`),
   and refuse the whole resolution when any does not — the premise step 4's
   exact window test rests on, made a fact rather than an assertion.
   `applyAnalyticCut` is free to record nothing: `cutSite` declines a cut
   whenever the boundary's local chord parameter falls within `segEps` of a
   segment end, or past an open source's domain end, both PARAMETER tests. At
   coarse sampling a boundary can clear neither bar — no cut recorded, and no
   sample vertex within the merge tolerance of the boundary POINT either — so
   the span stays one fragment and step 4 suppresses a fragment that reaches
   outside the window, deleting the hair that closes the region with nothing
   flagged. The check is therefore by distance: a boundary is materialized when
   the cut phase records a cut for it, or when a segment end lies within the
   vertex table's own merge tolerance of the boundary point (exactly when the
   graph treats the two as one vertex). Two boundaries landing on ONE tiny
   segment must also stay farther apart than `segEps` in that segment's local
   parameter, or `split`'s dedup collapses them into a single boundary and the
   fragment between them is never emitted.

   The window's two boundary points are therefore this event's **contact
   points**, and every consumer asking "where does this event place a contact"
   must read them — in particular `eventExplains`, the weld audit
   `auditMergedEndpoints` runs over a handled pair. The event's own `x`/`y` is
   the window MIDPOINT, a locator for a degeneracy flag and never a cut site,
   so answering that audit from the midpoint alone taints the exact cuts this
   step just made, whenever an overlap boundary happens to weld onto a sample
   vertex of the other source (an everyday alignment: at `spt` segments per
   turn a full carrier's sample vertices sit at every `2π/spt`). The symptom is
   `TExact = false` on the surviving merged fragment — conservative, but a
   direct miss of the acceptance criterion below.
3. **Name one source** for the shared span (see "The `SourceIndex` decision"
   below) and record, on the arranger, a suppressed **angular window**
   (`angularWindow{cx, cy, angLo, width}`, `geom/arrange.go`) for the OTHER
   (losing) source — the window's extent as an absolute angle about the shared
   carrier's centre, which `coincidentArcOverlap` already computes and
   `overlapExtent` already carries.

   The window is recorded in ANGLE space rather than as the losing source's own
   natural-parameter range `[tLo, tHi]`, and the two are not interchangeable in
   the direction that matters here. A natural-parameter range needs per-source
   sign and wrap bookkeeping — the two sources sweep independently, and a closed
   carrier's range wraps through its seam — while the angle is one physical
   quantity both sources agree on, because the resolution already requires them
   to share a centre. One source can also lose against SEVERAL named sources (a
   hub circle against every tooth's root arc), so the record is a slice of
   windows per losing source, each tested independently.
4. **Suppress the losing source's edge over that window in `split()`.** `split`
   (`geom/arrange.go`) already canonicalizes both boundary points of every
   tiny-segment sub-range into vertices before appending an `arrEdge`; the only
   new step is a check, immediately before that append, for whether the fragment
   lies inside a window recorded for `s.src` — if so, skip the append. The
   fragment is tested at **the source evaluated at its parameter midpoint**
   (`fragmentSuppressed`), a point on the source whatever the fragment's extent.
   Its CHORD midpoint is not a substitute, even though it is on the shared
   carrier's angular bisector of the fragment's two ends: `densify` floors a
   source at two tiny segments, so a coincident circle at a low
   `WithSegmentsPerTurn` is two semicircle fragments whose chords are diameters,
   and each chord midpoint is the carrier CENTRE — where the window's angle test
   reads `atan2(0,0) = 0` and answers about nothing. Both semicircles then read
   as angle 0, both are suppressed, and the disk vanishes with
   `Degenerate = false`.

   **The window is tested exactly, with no outward slop, and testing one
   interior point is equivalent to testing the whole sub-range** — but only
   because of step 2. Both window boundaries are verified fragment boundaries on
   the losing source, so no emitted fragment straddles one: every fragment is
   wholly inside the window or wholly outside it, and its midpoint answers for
   all of it. Slop
   breaks that equivalence in the unsafe direction. A fragment inside the window
   sits at least half its own angular extent clear of either end and needs no
   margin to be recognised, while a fragment OUTSIDE can be arbitrarily close:
   the losing source's own gap beyond the overlap is a real span of any width,
   and when the overlap covers nearly the whole carrier that gap fragment is the
   only thing left to close the region. An outward slop of `arcParamEps`
   swallowed exactly that fragment for every gap up to twice it, and the region
   vanished with `Degenerate = false` — the failure this step is written against
   (`TestAnalyticCoincidentCarrierNearFullGapStaysClosed`).

   The two boundary points are canonicalized regardless (both sources were cut
   at the SAME exact `(x,y)` event point in step 2, so `vertexTable.canon` welds
   them to one shared vertex whether or not an edge is ultimately kept for this
   source there), so the named source's own edge over the identical span still
   has valid endpoints to attach to. No change to `prune`, `buildGraph`, or
   `extract`: they see exactly the edge set `split` hands them, exactly as
   today.
5. **Mark the pair `handled`, do not `flagDegenerate`.** This is the same
   bookkeeping an ordinary `evCross`/`evTangent` pair already gets; only the
   `evOverlap` arm of `analyticPrepass`'s event-kind switch
   (`geom/arrange.go`) changes, from an unconditional
   `flagDegenerate` to this resolution when the overlap is a positive-length,
   single-window overlap with at least one non-complete arc operand and
   round-off-identical carriers — the scope above. A pair failing that scope
   (ambiguous, both operands covering the full turn, multi-window, a carrier
   match only within the classification band, or a window boundary step 2
   cannot materialize as a split) still flags exactly as today.

### The `SourceIndex` decision

**Name the lower of the two source indices** (`min(i,j)`, the pair-iteration
order `analyticPrepass` already uses — `for i ...; for j := i+1 ...`) and
report a single `SourceIndex` per `BoundaryEdge` for the merged span — no
signature change to `BoundaryEdge`/`cycFrag`/`arrEdge`, all of which already
carry exactly one `src`/`SourceIndex` field end to end
(`geom/arrange.go`'s `arrEdge`/`cycFrag`, `geom/region.go`'s `BoundaryEdge`,
`profiles.go`'s `BoundaryEdge`). The design commits to naming one rather than reporting
both:

- `BoundaryEdge` (and `Sketch`'s `BoundaryEdge`) name one source throughout
  the rest of the pipeline. Reporting two would be a new, asymmetric shape
  (which of two fields is "primary"?) for exactly one situation, while every
  existing consumer — including `Sketch.Profiles()`'s `entityFor`
  (`profiles.go:279-284`), which maps a bare `SourceIndex` straight to one
  `Entity` — already assumes one. The note itself says either shape works for
  its consumer ("decad can consume either... the choice is sketch's to
  specify, not decad's to infer"); naming one is the choice with no
  propagated signature change.
- **The rule is deterministic without new state.** `SourceIndex` is already
  positional (`Regions`'s own doc: "indexes curves for an open curve, or
  `len(curves)+k` for the k-th entry of closed"), so "the lower of the pair"
  requires no new tie-break heuristic — it falls out of the existing
  iteration order. It is reproducible for a fixed input order (the same
  sketch, marshalled and reloaded, or the same `geom.Regions` call repeated,
  always names the same source), which is the determinism property that
  matters; it is not, and cannot be, invariant to *which* entity a caller
  happens to author first — no positional index scheme could be.
- **It happens to prefer the arc over the full circle in the gear's own case,
  for free.** `Regions`' `SourceIndex` numbers every open curve (a `Curve`,
  including an arc) before every closed curve (a `ClosedCurve`, a circle):
  `len(curves)+k` for the k-th closed entry is always numerically larger than
  any open-curve index. So for exactly the circle/arc coincidence this design
  targets, "lower index" always resolves to the arc — the finite, swept
  entity whose extent actually matches the merged edge's role in that
  boundary — never the full circle. For an arc/arc pair (both open curves)
  it is simply "whichever was passed first to `Regions`," a stable tie-break
  with no comparable semantic reading either way.
- `TStart`/`TEnd`/`TExact` on the merged edge are reported entirely in the
  NAMED source's own natural-parameter direction, exactly as for any other
  fragment — `Whole` is `false` (the merged span is, by construction, a
  strict sub-range unless the whole named source happens to equal the
  overlap), and `TExact` is `true` on both bounds (both are exact cuts from
  step 2, on the named source's own parameterization). The losing source
  contributes nothing to any `BoundaryEdge` for this span — not a second,
  suppressed fragment, not a zero-length placeholder.

## Area accounting

No new formula. `makeCycle`'s bulge computation (`geom/arrange.go`)
reads `s.r` and the fragment's swept angle `(f.pEnd-f.pStart)*s.sweep` from
whichever source the surviving fragment names — after suppression, that is
always the named source for the merged span. Because the condition that admits
the pair for resolution requires equal center AND equal radius at round-off
(`carriersIdentical`, see "The refusal band"), the circular-segment correction
(`chordArcCorrection(r, Δangle)`) over the shared span is numerically
identical whichever of the two sources it is evaluated against; naming one
over the other changes nothing about the number `makeCycle` computes, only
which `SourceIndex` labels it. The existing exact-area guarantee (`CLAUDE.md`:
"Region area is exact for every curve type... an arc/circle via shoelace +
exact circular-segment correction") therefore carries over to a merged edge
unchanged, with no new test needed for the arithmetic itself — only for the
`SourceIndex`/attribution behavior around it (see "Tests" below).

## The refusal band

`circleCircleEvents`'s existing scale-relative bands are reused unchanged for
CLASSIFICATION — no new constant there. `certify = scale*tangentCertify` is the
"same carrier" threshold; anything from there out to `band = scale*tangentBand`
is the `ambiguous` classification, which `analyticPrepass` already turns into
`flagDegenerate` — unchanged by this design. Carriers whose center distance or
radius difference sits in that outer band read as "equal within noise but not
equal," and stay refused rather than merged.

**RESOLUTION is gated tighter than classification, at the identity band**
(`carriersIdentical`), and this is the load-bearing half. Merging on a tolerance
rather than on real equality would place the resolved cut's event point
somewhere between the two sources' true carriers, off BOTH of them, which is
precisely the kind of manufactured exactness `TExact`'s contract forbids — and
nothing downstream catches it, because `vertexCertifies` compares the graph
vertex against the cut's own stored point, which is where the cut put it. The
certify band alone is far too loose for that: `tangentCertify` is three orders
above `weldIdentEps`, so a pair coincident only to within it resolves into a
fragment whose reported bounds evaluate visibly off the emitted polyline.

The bound is exact, not a heuristic. `resolveCoincidentOverlap` computes both
window boundary points on ONE operand's carrier and cuts both sources there, so
a boundary point `P` misses the other operand's carrier by
`||P − b.center| − b.r| ≤ d + |a.r − b.r|` (with `d` the center distance).
`carriersIdentical` bounds that SUM, and it bounds it TWICE:

- by `weldIdentEps·scale` — the same identity band `vertexCertifies` uses to
  decide whether a graph vertex IS a bound's own point — so a resolved bound's
  reported parameter really does reproduce the emitted geometry; and
- by `weldIdentEps·max(a.r, b.r)`, a band built from the two CARRIERS
  themselves.

The second is what makes the gate a statement about the pair. `scale` is the
whole scene's bounding-box extent, so any object anywhere inflates it: a scene
that also reaches out to `x = 1e15` carries a global band of `1e3`, at which an
`r = 2` carrier and an `r = 1` carrier — a whole unit apart, and classified
coincident because the certify band grew with the same scale — read as the same
curve, and resolution suppresses one of them outright. That is the false bless
this whole section forbids, reached without any carrier being near any other
(`TestAnalyticCoincidentCarrierDistantSceneStaysDegenerate`). A band scaled to
the carriers' own radius cannot be widened from across the scene.

The centre separation `d` is the quantity under test, so it belongs in the
offset and never in the tolerance: a band that grew with `d` would admit
carriers in proportion to how far apart they are.

The conservative direction is sound because the alternative — resolving a
near-but-not-equal pair as if it were exact — cannot be distinguished downstream
from a genuine coincidence once written into a `TExact = true` fragment, while
a false refusal costs only a `Degenerate` verdict a caller can act on (loosen
the geometry, or accept the flag). Real coincident geometry clears both halves
of the identity gate comfortably: a hub circle of radius `r` and a root arc
whose radius is derived as `hypot(start − center)` differ by a couple of ulps of
`r`, ~4 orders inside `weldIdentEps·r` and further still inside
`weldIdentEps·scale`. `TestAnalyticNearCoincidentCirclesAmbiguous`
(`geom/arrange_events_internal_test.go`) already pins the outer band's behavior
at the event-kernel level for concentric circles; this design adds the analogous
case with a genuine arc sweep, plus an in-certify-band case (see "Tests").

## Determinism

- **Input order:** the named source is a deterministic function of the input
  order (`min(i,j)` of the pair's positions), so a fixed sketch or a fixed
  `geom.Regions(curves, closed)` call always names the same source on reload
  or repeat. It is not, and is not claimed to be, invariant to reordering the
  input list — no positional `SourceIndex` scheme can be (see the
  `SourceIndex` decision above).
- **Curve reversal:** reversing an entity's own authored direction (building
  an arc from its End to its Start) changes only that source's own natural
  parameter direction (`Reversed`, `TStart<TEnd`), not its position in the
  input list, so it never changes which source is named.
- **Sampling density:** the resolution is analytic end to end — the overlap
  boundary points come from closed-form angles, not samples, and suppression
  in `split()` acts on the recorded ANGULAR WINDOW, not a segment count. A
  finer `WithSegmentsPerTurn` changes how many tiny-segment pieces the losing
  source's overlap span is chopped into before suppression, but every one of
  those pieces falls inside the same window and is dropped — the resolved
  topology, the named source, and the reported area do not move with sampling
  density. What density CAN change is whether the pair is resolved at all: the
  boundaries have to materialize as splits (step 2), and a boundary a coarse
  sampling cannot place is refused outright. That direction is safe — a refusal
  is a `Degenerate` flag, never a quietly different region set — and the
  threshold is a property of the cut machinery, not of the window.

## Acceptance criteria (repository terms)

- `geom.Regions` on probe case C's five entities returns
  `arrangementDegenerate=false` and the two intended regions (hub, area
  π·100; tooth, area ≈11.864262 — both already the numerically correct values
  today; what changes is `Degenerate` and the boundary attribution below).
- The coincident span is reported exactly once per adjoining region, both
  times under the SAME `SourceIndex` (the lower of the coincident pair) —
  never one region naming the arc and the other naming the circle for the
  identical physical curve.
- Every bound of the merged edge is `TExact = true`, at every alignment of the
  overlap boundaries against the losing source's sample vertices (neither, one,
  or both landing on one).
- `Sketch.Profiles()` on the equivalent sketch reports the merged edge's
  `Entity` consistently across both adjoining `Profile`s, and neither
  `Profile.Entities` includes the discarded (non-named) entity for that span.
- Curve reversal and `WithSegmentsPerTurn` density do not change region count,
  area, or which source is named. Input order does not change region count or
  area either, and names the lower input position in every order — so the named
  ENTITY does change when the inputs are reordered, which is the `min(i,j)` rule
  working as specified and not a defect (see "Determinism"; no positional
  `SourceIndex` scheme can be reorder-invariant).
- A carrier pair equal within noise but not at round-off still reports
  `arrangementDegenerate=true`, both in the outer ambiguous band and INSIDE the
  certify band — the resolution never fires outside the identity band.
- Adding an unrelated, distant curve to a scene never turns a refusal into a
  resolution: a pair whose carriers differ by a visible fraction of their own
  radius stays `arrangementDegenerate=true` however far the rest of the scene
  reaches (the carrier-local half of the identity gate).
- An overlap window covering all but a hair of the losing source still leaves
  that hair emitted, so the region closes: a unit circle and a coincident arc
  with a real angular gap report one disk of area π and
  `arrangementDegenerate=false`, for gaps down to `arcParamEps` (below which the
  arc is a complete carrier and the pair is refused instead). The lower end
  additionally needs the gap's two cuts to materialize as distinct boundaries —
  they must each split the losing source and stay farther apart than a `segEps`
  fraction of one chord, which holds from 16 segments per turn up. Below that
  density the pair is REFUSED (`arrangementDegenerate=true`), never resolved
  against boundaries that do not exist: step 2 checks it.
- A coincident circle at a `WithSegmentsPerTurn` low enough to sample it as two
  semicircles still reports the resolved disk, at any distance of the shared
  centre from the origin — the fragment classification never reads a chord
  midpoint.
- A full-turn operand (a 2π `Arc` as much as a `Circle`) paired with another
  full carrier still reports `arrangementDegenerate=true`.
- A pair with a second overlap window of any positive length, however far below
  `arcParamEps`, still reports `arrangementDegenerate=true`.
- A direct sketch authoring of probe case C's five entities in one sketch
  (no arrangement call at all) is `Verify(ctx).Trustworthy()` where it reads
  false today — the note's own framing ("gear-like sections cannot even be
  authored directly and verified").

## Tests

- `geom/arrange_events_internal_test.go`: extend
  `TestAnalyticSameCarrierArcs` (or add a sibling) to assert the OVERLAP
  boundary points `coincidentArcOverlap` (or its extended form) returns are
  the arcs' own domain ends, exactly, for the finite-overlap cases it already
  builds.
- `geom/arrange_analytic_test.go`: a new test built on probe case C's exact
  geometry, asserting `Degenerate == false`, region count and areas as above,
  and — walking both regions' `Outer` — that the merged span's `SourceIndex`
  agrees between them and matches the lower of the two coincident sources.
  Sweep `WithSegmentsPerTurn` across a wide range and assert the result is
  unchanged (the "Determinism" invariant).
- `geom/arrange_analytic_test.go`: an arc/arc coincident-carrier case
  (`TestAnalyticSameCarrierArcs`'s `overlapping` sub-case is currently only
  asserted `Degenerate`; add a sibling once resolution lands, asserting the
  resolved region set) plus a case where the two arcs' roles are swapped in
  the input list, to pin that the NAMED source follows input order rather
  than which arc happens to be geometrically "outer."
- `geom/arrange_analytic_test.go`: a near-coincident-but-not-certified case
  (radius or center offset by `1.5*tangentBand*scale`, in the ambiguous band)
  built with a genuine arc sweep (not the bare concentric circles
  `TestAnalyticNearCoincidentCirclesAmbiguous` already covers), asserting
  `Degenerate == true` — the outer refusal-band regression guard
  (`TestAnalyticCoincidentCarrierNearCertifyStaysDegenerate`).
- `geom/arrange_analytic_test.go`: probe case C's geometry with the root arc's
  radius offset by a delta INSIDE the certify band but well above either half of
  the identity band, asserting `Degenerate == true` and that no surviving
  bound claims an exactness it cannot reproduce — the identity-band guard for
  the case classification alone would let through
  (`TestAnalyticCoincidentCarrierInCertifyBandStaysDegenerate`).
- `geom/arrange_analytic_test.go`: a full-turn `Arc` (`NewArc(c, p, p)`) against
  a coincident `Circle`, and against another full-turn `Arc`, asserting
  `Degenerate == true` — the complete-carrier exclusion keyed on geometry rather
  than on the `fullCircle` flag
  (`TestAnalyticCoincidentCarrierFullTurnArcStaysDegenerate`).
- `geom/arrange_analytic_test.go`: two same-carrier arcs whose second overlap
  window is positive but far below `arcParamEps`, asserting `Degenerate == true`
  (`TestAnalyticCoincidentCarrierSubEpsilonSecondWindowStaysDegenerate`).
- `geom/arrange_analytic_test.go`: probe case C swept over tooth placements that
  put neither / one / both overlap boundaries on a hub SAMPLE vertex, asserting
  `TExact` on every bound — the weld-audit guard for step 2's contact points
  (`TestAnalyticCoincidentCarrierBoundOnSampleVertexStaysExact`).
- `geom/arrange_analytic_test.go`: an `r=2` arc and an `r=1` circle sharing a
  centre, with and without a distant line stretching the scene to `x = 1e15`,
  asserting `Degenerate == true` with the line and an ordinary unit disk without
  it — the carrier-local half of the identity gate
  (`TestAnalyticCoincidentCarrierDistantSceneStaysDegenerate`).
- `geom/arrange_analytic_test.go`: a unit circle and a coincident arc leaving a
  real angular gap, swept from just above `arcParamEps` upward, asserting one
  region of area π at every gap — the no-slop suppression guard, plus the
  sub-`arcParamEps` gap asserted still refused as a complete carrier
  (`TestAnalyticCoincidentCarrierNearFullGapStaysClosed`).
- `geom/arrange_analytic_test.go`: a coincident circle sampled as two semicircles
  (`WithSegmentsPerTurn` at and below 2, where `densify`'s floor bites), swept over
  shared-centre offsets, asserting the resolved disk at every one — the
  chord-midpoint guard for step 4's fragment classification
  (`TestAnalyticCoincidentCarrierHalfTurnFragmentSurvives`).
- `geom/arrange_analytic_test.go`: a near-full overlap whose boundaries fall within
  `segEps` of the losing source's seam vertex while sitting well outside the vertex
  merge tolerance, asserting `Degenerate == true` below the density where the cuts
  land and the unchanged resolved disk above it — the materialization guard for
  step 2 (`TestAnalyticCoincidentCarrierUnsplitBoundaryStaysDegenerate`).
- `profiles_test.go` (root package): a sketch-level equivalent of probe case
  C, asserting `Verify(ctx).Trustworthy()` and consistent `Entity` attribution
  across the hub and tooth profiles.

## Open decisions

- **Whether "lower `SourceIndex`" is the right long-term naming rule**, versus
  an explicit kind-based rule ("prefer an arc over a circle" stated directly
  rather than derived from index ordering). The two coincide for every
  circle/arc pair today (closed curves always index after open curves), so
  this is not observable from the gear workload; it would only diverge if a
  future source kind changed that ordering guarantee. Revisit only if
  `Regions`' indexing convention (curves before closed) is ever revisited for
  an unrelated reason — until then, deriving the rule from existing index
  order costs no new state and is the design's choice.
- **Two fully-coincident complete carriers, a multi-window overlap, and a
  carrier match holding only within the classification band** stay out of
  scope and `Degenerate` (see "Scope"). Revisit if a consumer's workload
  needs any — none is exercised by the gear use case this design was
  written against.
