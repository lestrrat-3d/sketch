# Coincident Carrier Resolution — Design

Status: **designed, not implemented**. Companion to
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
(`geom/arrange.go:1042-1043`, fed by `circleCircleEvents`'s coincident-carrier
branch and `coincidentArcOverlap`, `geom/arrange_events.go:159-182,299-310`).
This is not a rare input: it is the **normal case** for a gear tooth, whose
root arc is by construction an arc of the hub circle it sits on — every tooth
in a 12–45-tooth gear repeats it. `TestAnalyticSameCarrierArcs`
(`geom/arrange_analytic_test.go:188-214`) already exercises the same code path
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

- **Two fully-coincident full circles** (both sources unbounded sweeps, so
  neither contributes a finite overlap boundary — the "overlap" is the entire
  circle). This is a duplicate-entity degeneracy with no natural cut points;
  resolving it is a distinct, smaller problem (drop one full duplicate,
  replicate any THIRD-party cut from one carrier onto the other) that the gear
  workload never exercises and this design does not attempt.
- **Coincident line carriers** (`lineLineEvents`'s overlap branch,
  `geom/arrange_events.go:210-228`, gated by `mergeEps` rather than
  `tangentCertify`/`tangentBand`). A different code path, a different
  tolerance, and not what the note's gear workload needs; left for a future
  design if a consumer asks for it.
- **A pair whose sweeps overlap in more than one disjoint angular window.**
  `coincidentArcOverlap` already only reports the single longest contiguous
  overlap (`geom/arrange_events.go:169-176`) — a pre-existing scope limit this
  design inherits unchanged, not one it introduces. The gear workload has
  exactly one overlap window per pair by construction (one root arc, one hub
  circle).

## Mechanism

### Detection (unchanged)

`circleCircleEvents`'s coincident-carrier branch already certifies "same
center, same radius" scale-relatively (`d <= certify && |a.r-b.r| <= certify`,
`certify = scale*tangentCertify = scale*1e-9`,
`geom/arrange_events.go:299-310`) and `coincidentArcOverlap` already confirms
the two operands' swept angular ranges actually intersect with positive
length, returning `over=false` for disjoint or endpoint-only sweeps (a normal
join, not a degeneracy — `TestAnalyticSameCarrierArcs` pins this). Nothing
here changes that classification; the design changes only what
`analyticPrepass` does with a certified, positive-length overlap.

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
   `applyAnalyticCut(src, t, x, y)` (`geom/arrange.go:1331-1341`), using each
   operand's own `circleParam(x,y)` for `t` — unmodified; a boundary that
   lands on a source's own domain end is already a no-op through the existing
   `cutSite`/`atSourceEnd` check, exactly as an ordinary `evCross` at a shared
   endpoint is today.
3. **Name one source** for the shared span (see "The `SourceIndex` decision"
   below) and record, on the arranger, a suppressed natural-parameter range
   `[tLo, tHi]` for the OTHER (losing) source.
4. **Suppress the losing source's edge over that range in `split()`.** `split`
   (`geom/arrange.go:1648-1710`) already canonicalizes both boundary points of
   every tiny-segment sub-range into vertices before appending an `arrEdge`;
   the only new step is a check, immediately before that append, for whether
   the fragment's natural-parameter sub-range falls inside a suppressed range
   recorded for `s.src` — if so, skip the append. The two boundary points are
   canonicalized regardless (both sources were cut at the SAME exact `(x,y)`
   event point in step 2, so `vertexTable.canon` welds them to one shared
   vertex whether or not an edge is ultimately kept for this source there),
   so the named source's own edge over the identical span still has valid
   endpoints to attach to. No change to `prune`, `buildGraph`, or `extract`:
   they see exactly the edge set `split` hands them, exactly as today.
5. **Mark the pair `handled`, do not `flagDegenerate`.** This is the same
   bookkeeping an ordinary `evCross`/`evTangent` pair already gets; only the
   `evOverlap` arm of `analyticPrepass`'s event-kind switch
   (`geom/arrange.go:1040-1083`) changes, from an unconditional
   `flagDegenerate` to this resolution when the overlap is a certified,
   positive-length, single-window overlap with at least one arc operand — the
   scope above. A pair failing that scope (ambiguous, both-full-circle, or
   multi-window) still flags exactly as today.

### The `SourceIndex` decision

**Name the lower of the two source indices** (`min(i,j)`, the pair-iteration
order `analyticPrepass` already uses — `for i ...; for j := i+1 ...`) and
report a single `SourceIndex` per `BoundaryEdge` for the merged span — no
signature change to `BoundaryEdge`/`cycFrag`/`arrEdge`, all of which already
carry exactly one `src`/`SourceIndex` field end to end
(`geom/arrange.go:246-259,2158-2165`, `geom/region.go:17-95`,
`profiles.go:88-149`). The design commits to naming one rather than reporting
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

No new formula. `makeCycle`'s bulge computation (`geom/arrange.go:2281-2306`)
reads `s.r` and the fragment's swept angle `(f.pEnd-f.pStart)*s.sweep` from
whichever source the surviving fragment names — after suppression, that is
always the named source for the merged span. Because the certify condition
that classifies the pair coincident in the first place requires equal center
AND equal radius to within `tangentCertify`, the circular-segment correction
(`chordArcCorrection(r, Δangle)`) over the shared span is numerically
identical whichever of the two sources it is evaluated against; naming one
over the other changes nothing about the number `makeCycle` computes, only
which `SourceIndex` labels it. The existing exact-area guarantee (`CLAUDE.md`:
"Region area is exact for every curve type... an arc/circle via shoelace +
exact circular-segment correction") therefore carries over to a merged edge
unchanged, with no new test needed for the arithmetic itself — only for the
`SourceIndex`/attribution behavior around it (see "Tests" below).

## The refusal band

Reuse `circleCircleEvents`'s existing scale-relative bands unchanged — no new
constant. `certify = scale*tangentCertify` (`1e-9`) is the "same" threshold;
anything from there out to `band = scale*tangentBand` (`1e-6`) is today's
`ambiguous` classification, which `analyticPrepass` already turns into
`flagDegenerate` (`geom/arrange.go:1006-1010,1035-1039`) — unchanged by this
design. Carriers whose center distance or radius difference sits in that
band read as "equal within noise but not equal," and stay refused rather than
merged: merging on a tolerance rather than on certified equality would place
the resolved cut's exact event point somewhere between the two sources' true
carriers, off BOTH of them, which is precisely the kind of manufactured
exactness `TExact`'s contract (`geom/region.go:66-94`) forbids. The
conservative direction is sound because the alternative — resolving a
few-ulps-off pair as if it were exact — cannot be distinguished downstream
from a genuine coincidence once written into a `TExact = true` fragment, while
a false refusal costs only a `Degenerate` verdict a caller can act on (loosen
the geometry, or accept the flag). `TestAnalyticNearCoincidentCirclesAmbiguous`
(`geom/arrange_events_internal_test.go:175-186`) already pins the band's
behavior at the event-kernel level for concentric circles; this design adds
the analogous case with a genuine arc sweep (see "Tests").

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
  in `split()` acts on a natural-parameter RANGE, not a segment count. A
  finer `WithSegmentsPerTurn` changes how many tiny-segment pieces the losing
  source's overlap range is chopped into before suppression, but every one of
  those pieces falls inside the same recorded `[tLo, tHi]` and is dropped —
  the resolved topology, the named source, and the reported area do not move
  with sampling density.

## Acceptance criteria (repository terms)

- `geom.Regions` on probe case C's five entities returns
  `arrangementDegenerate=false` and the two intended regions (hub, area
  π·100; tooth, area ≈11.864262 — both already the numerically correct values
  today; what changes is `Degenerate` and the boundary attribution below).
- The coincident span is reported exactly once per adjoining region, both
  times under the SAME `SourceIndex` (the lower of the coincident pair) —
  never one region naming the arc and the other naming the circle for the
  identical physical curve.
- Every bound of the merged edge is `TExact = true`.
- `Sketch.Profiles()` on the equivalent sketch reports the merged edge's
  `Entity` consistently across both adjoining `Profile`s, and neither
  `Profile.Entities` includes the discarded (non-named) entity for that span.
- Input order, curve reversal, and `WithSegmentsPerTurn` density do not change
  region count, area, or which source is named (see "Determinism").
- A carrier pair inside the ambiguous band (equal within noise, not exactly)
  still reports `arrangementDegenerate=true` — the resolution never fires
  outside the certified case.
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
  (radius or center offset by `1.5*tangentCertify*scale`, inside the
  ambiguous band) built with a genuine arc sweep (not the bare concentric
  circles `TestAnalyticNearCoincidentCirclesAmbiguous` already covers),
  asserting `Degenerate == true` — the refusal-band regression guard.
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
- **Two-fully-coincident-full-circles and multi-window overlap** stay out of
  scope and `Degenerate` (see "Scope"). Revisit if a consumer's workload
  needs either — neither is exercised by the gear use case this design was
  written against.
