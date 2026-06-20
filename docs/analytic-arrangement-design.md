# Analytic Arrangement — Design & Increment Plan

Status: **in progress** — increment 1 (the analytic event kernel) is implemented
(`geom/arrange_events.go`); the rest is the roadmap below. Resolves the
"analytic (non-sampled) arrangement" open follow-up of the Profile/region engine
(`docs/verification-roadmap.md`).

## The problem

`geom.Regions` (`geom/arrange.go`) builds the planar arrangement by **sampling**
every curve to a polyline and detecting crossings with a segment-segment test
(`segParams`). A near-tangent polyline crossing (`p.sin < 1e-3`) is flagged
`Degenerate`, which gates `Verify().Trustworthy()` false. Two consequences make
the oracle reject *valid* sketches (false negatives):

- A clean **tangency** (two circles touching at one point, a line tangent to a
  circle) reads as a near-tangent degeneracy.
- A clean but **shallow** transverse crossing reads as near-tangent.

And the sampled topology can be mis-resolved at shallow crossings or near misses,
which an oracle must never silently bless.

The fix is to detect crossings **analytically** (exact closed-form intersections)
for the curve kinds that have a closed form (line / circle / arc — already present
as standalone helpers in `geom/intersect.go` but unused by the arrangement), so the
arrangement can classify a contact precisely: a transverse crossing splits the
topology; a clean tangency is a non-splitting contact (not a degeneracy); a
coincident overlap or a genuinely unresolvable case is reported `Degenerate`.
Curves with no closed form (ellipse, spline) keep the sampled fallback.

## Architecture (the target and the path)

**Target:** a full curved arrangement — exact intersection events, exact
source-parameter fragments, exact tangent ordering at vertices, polylines only for
rendering.

**Path:** hybrid first, but built so that for supported source pairs the analytic
kernel is **authoritative** (a hybrid that merely injects analytic points while
letting sampled `segParams` decide topology is a dead end — `buildGraph` still
embeds sampled chords). Tolerances are **scale-relative** (consistent with the
nondimensional rank/conditioning work), with separate thresholds for root
classification, vertex merge, and angular ordering.

### Tangency contract

A clean analytic tangency is **one contact, crossing-parity zero** — never a
transverse crossing. Two externally tangent circles must yield two clean disk
regions (no lens/sliver, no `Degenerate`); a tangent line prunes away leaving one
disk. BUT the current planar map cannot safely represent a tangency where the
contact canonicalizes as a **shared vertex between two cycle-bearing sources**:
`buildGraph` sorts outgoing half-edges by chord angle, and at a tangency those
angles tie, so the face walk can branch-swap the loops. So:

- clean analytic tangent ⇒ no cut, no near-angle degeneracy;
- if the tangent contact would merge into a shared cycle-bearing vertex ⇒
  conservatively `flagDegenerate` (true tangent-**port** handling is a later
  increment);
- a tangent line that is an open/dangling spur against a circle ⇒ no-cut is fine
  (the line is pruned, the circle stays one disk).

Same-component interior tangency is a **self-touch** → `SelfIntersections`, not
`Degenerate`.

## Increment plan

1. **Analytic event kernel** — *done* (`geom/arrange_events.go`). `analyticEvents(si,
   sj, scale)` returns the exact contacts between two line/circle/arc sources:
   `{evCross, evTangent, evOverlap}`, an `ambiguous` flag, and the natural param
   `t∈[0,1]` on each source; arc-sweep clipped; scale-relative two-band
   classification (a tight *certify* band → tangent, a wider band → clean
   miss/secant, the zone between → ambiguous). Unsupported kinds return `ok=false`.
   White-box tested in `geom/arrange_events_internal_test.go`.

2. **Analytic-authoritative wiring** — *next*. A pre-pass over source pairs makes
   supported pairs analytic-authoritative; sampled `segParams` is skipped for them.
   See "Wiring design" below.

3. **Exact tangent/port ordering** — replace chord departure angles at analytic
   vertices with exact source tangents; add tangent-port handling so a shared
   tangent vertex no longer branch-swaps — this is what lets increment 2's
   conservative `flagDegenerate` become a real tangent blessing.

4. **Analytic overlap / self-intersection coverage** for supported primitives
   (coincident lines, duplicate/overlapping arcs, identical circles, same-source
   arc/circle self-touch) before the sampled fallback.

5. **Ellipse phase 1** — line/ellipse and line/elliptical-arc (a quadratic in
   ellipse-local coordinates). Bless clean tangents; keep ambiguous degenerate.

6. **Ellipse phase 2** — circle/ellipse and ellipse/ellipse via a certified
   conic-conic kernel (root-residual checks, arc filtering); if it cannot be made
   robust, do not pretend it is exact.

7. **Full curved DCEL** — replace `tinySeg` topology with exact curve fragments
   between event params; face traversal on exact tangents/ports;
   `BoundaryEdge.Polyline` becomes an output artifact only.

## Wiring design (increment 2)

**The cut-record caveat (load-bearing).** Keeping `tinySeg.cuts []float64` is NOT
sound for analytic circle/arc cuts: `split()` reconstructs the vertex by **chord
interpolation** from the local float, so two exact curve params from different
sources generally produce two *different* chord points and the crossing does not
merge into one vertex. The cut payload must carry the local sort param, the exact
source param, AND the exact event `(x,y)`; `split()` emits the analytic cut vertex
at the **shared exact event point** so both sources land on one canonical vertex.
`buildGraph` stays unchanged.

**Pre-pass shape:**
1. After `densify`, build `sourceSegs[src] -> []segIndex` from each `tinySeg.{pa,pb}`.
2. For each source pair `srcA<srcB`, call `analyticEvents`.
3. `ok=false` → do nothing (sampled fallback handles it).
4. `ok=true` → mark `handled[pair]=true`.
5. `ambiguous` or any `evOverlap` → `flagDegenerate`.
6. each `evCross` → map `ti/tj` to the containing tiny segment and add an exact cut
   record (with the shared event point); replicate self-intersection (below).
7. each `evTangent` → no cut, bypass the `p.sin<1e-3` heuristic, subject to the
   conservative merged-vertex rule (tangency contract above).
8. In the existing segment loop, skip pairs where `si.src != sj.src && handled[pair]`;
   keep same-source spline logic unchanged.

**Self-intersection preservation.** For an analytic `evCross` between *different*
sources, replicate the current core/component check: require
`a.core[srcA] && a.core[srcB]`, `a.comp[srcA]==a.comp[srcB]`, suppress if the
component is in `a.notSimple` (keeps square-with-diagonals branched, not
self-intersecting), suppress endpoint-endpoint contacts (a normal join), else append
to `selfX` and mark `selfXc[comp]`. (Line/circle/arc never self-cross as a single
source, so only cross-source same-component matters.)

**Cut-mapping rules.** Source-param semantics, not sampled-crossing semantics: an
event at an open endpoint (`t=0/1`) uses the existing vertex (no cut); a closed-circle
seam (`t≈0/1`) is topologically interior so mark the source split even without a new
record; a segment-boundary event adds no duplicate; a segment-interior event adds one
exact cut record; dedup by source param / event point. `srcCut` must mean "source was
topologically split by a crossing," not merely "a local cut was appended."

**Tests (increment 2):** shallow line-line crossing (clean `evCross`, not
`Degenerate`); a circle cut by a chord under coarse vs fine `WithSegmentsPerTurn`
(same two regions + cap area — analytic cuts are sampling-stable); a tangent
line+circle (one disk, line pruned, no degeneracy); non-merged externally tangent
circles (two disks, no degeneracy); merged-vertex tangent circles (`Degenerate=true`
until the port handling of increment 3). Watch: bowtie/self-intersection, bowtie+spur,
square-with-diagonals, circle-chord half-disk, overlapping rectangles, nested-square
hole, collinear-overlap degeneracy, spline self-intersection/fallback.

## Invariants every increment must hold

- All existing profile/region/self-intersection/degenerate tests pass.
- Supported pairs are analytic-authoritative; unsupported pairs stay sampled.
- Coarse vs fine sampling gives the same topology for analytically-covered pairs.
- Scaling geometry tiny/huge does not change classification (scale-relative bands).
- Input order and curve reversal do not change region areas/counts.
- `Degenerate` always forces `ProfilesValid=false` and therefore `Trustworthy=false`.
- A clean supported tangency does not set `Degenerate` (once the port handling lands;
  conservatively `Degenerate` at a merged cycle-bearing vertex until then).
