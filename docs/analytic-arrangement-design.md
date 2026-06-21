# Analytic Arrangement — Design & Increment Plan

Status: **in progress** — increments 1 (the analytic event kernel,
`geom/arrange_events.go`) and 2 (the analytic-authoritative wiring,
`geom/arrange.go`) are implemented, and increment 3 (exact tangent/port ordering)
is **partly** in — a merged-vertex EXTERNAL circle/arc tangency is now blessed as
two disks via curvature-ordered ports; internal/containment, osculation, and
curve/curve crossing authority remain deferred. The rest is the roadmap below.
Resolves the "analytic (non-sampled) arrangement" open follow-up of the
Profile/region engine (`docs/verification-roadmap.md`).

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

2. **Analytic-authoritative wiring** — *done* (`geom/arrange.go`: `analyticPrepass`,
   the `cut{t,px,py}` exact-point record, the handled-pair skip, the
   `sampledCrossCount` + `analyticCrossHosted` consistency gate). Analytic authority
   is taken for **line-involved crossings and all tangencies**: the oracle no longer
   false-flags clean shallow crossings or clean tangencies (tangent line+circle → one
   disk; non-merged tangent circles → two disks) and line/circle cuts are
   sampling-stable. **Curve/curve transverse crossings are deferred to the sampled
   path** (see "Scope of analytic authority"): their sampled topology is already
   correct, and exact cuts there are unsound-or-over-conservative until increment 3,
   so injecting them is net-negative. A line-involved curved pair whose exact crossing
   the coarse sampled map cannot host (a sub-sample cap, or a crossing the polyline
   never reaches) is conservatively `Degenerate` via the gate, never a blessed wrong
   topology. Tangencies that would merge into a shared cycle-bearing vertex are
   conservatively `Degenerate` (see the tangency contract) pending increment 3. Tested
   in `geom/arrange_analytic_test.go`. See "Wiring design" below.

3. **Exact tangent/port ordering** — *partly done* (`geom/arrange.go`:
   `source.differential`, `portKey`, `sortExactPorts`, `useExactPorts`,
   `externalCurvedTangency`). At a certified analytic tangency contact the rotation
   system orders coincident-tangent ports by exact source tangent + signed
   **curvature** (`sortExactPorts` clusters same-ray ports into direction buckets,
   then sorts by an EXACT lexicographic key (groupAngle, curvature, index) — a
   transitive strict-weak order, no epsilon in the comparator; the seam-free
   half-plane + cross-product direction compare is used only for clustering and the
   osculation flag) instead of chord angle, so a shared tangent vertex no longer
   branch-swaps. The increment-2 conservative `flagDegenerate` for a **merged-vertex
   EXTERNAL circle/arc tangency** is lifted: it is blessed as two clean disks at
   every sampling (opposite curvature sign separates the loops). **Load-bearing scope
   rule:** exact ordering is used ONLY at the certified tangency contacts
   (`exactPortVerts`), never at a sampled crossing vertex — there the edges are
   *chords*, so chord ordering is what matches the polyline geometry the face walk
   traverses; ordering those by exact tangents corrupts the map. Still `Degenerate`
   (deferred): **internal/containment** tangency, line-involved merged tangency, a
   genuine **osculation** (equal tangent AND equal curvature), and curve/curve
   transverse **crossing** authority (still deferred to the sampled path — lifting it
   needs the post-split fragment
   certificate below). The richer per-event **hostability certificate** that would
   bless those — fragment **incidence** (the emitted straight fragments have no
   extra/missing crossings vs the analytic event set), full **port order** at every
   event vertex, and **closed containment** (a nested/internally-tangent inner cycle
   certified inside the outer) — is the remaining increment-3+ work.

   *Internal-tangency finding (why it is NOT a quick gate-relaxation):* an experiment
   that simply let `externalCurvedTangency` accept internal tangencies too gets the
   exact ordering *right* in the common case (e.g. outer r=6 + inner r=3 → a single
   annulus face π·(R²−r²) + the inner disk π·r², exact at every spt). But a sweep
   found it **blessed-wrong** at **tiny inner + coarse spt** (r/R≈0.05, spt 5–6): the
   shared-contact-vertex face walk fails to subtract the inner, so the outer reads as
   the FULL disk π·R² and the inner disk is double-counted (total π·R²+π·r²). This is
   a NEW failure of the tangent face walk, *not* pre-existing — disjoint nested
   circles at the same tiny-inner/coarse-spt are correctly hole-assigned (verified
   `bad=0` on the sampled path). So internal tangency stays `Degenerate`; lifting it
   needs a real **closed-containment certificate** (or an area-consistency gate:
   total must equal π·R_outer²), not just dropping the `d > max(r)` guard.

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

**Scope of analytic authority (load-bearing).** Injecting an *exact* analytic cut
into a *coarse* sampled chord is only safe when the sampled polyline can host the
crossing. The decisive split is by operand kind:

- **Curve/curve transverse crossings** (BOTH sources circle/arc, ≥1 `evCross`) are
  **deferred to the sampled path** — they are *not* taken as analytic-authoritative
  (not marked `handled`; the sampled loop processes them). The sampled DCEL already
  resolves their topology correctly (the pre-analytic behaviour, byte-identical to a
  no-wiring build), so exact cuts buy only exact *area*, and until increment 3's
  tangent-port certificate that exactness cannot be admitted without being either
  **unsound** (two equal-count coarse crossings at the *wrong* locations fuse three
  regions into one — a real round-2 bug) or **over-conservative** (a valid
  well-separated crossing whose sampled crossing sits one chord segment off the
  analytic param gets false-flagged — an ~18%-at-spt-16 false-degenerate rate, a
  regression versus the sampled path's 0%). Both are worse than deferring. A
  genuinely ambiguous verdict still `flagDegenerate`s. Exact-area curve/curve
  crossings are increment 3.
- **Line-involved crossings + all tangencies** keep analytic authority (the wins:
  shallow line/line not degenerate, tangent line+circle → one disk, non-merged
  tangent circles → two disks, chord-through-circle exact area). A line operand is
  reproduced exactly, so its sampled crossing tracks the analytic one — there is no
  wrong-location failure mode and no over-conservatism (measured ~0.3%, all genuine
  near-tangents).

For a handled pair with a curved source (i.e. line/circle, line/arc, or a curved
*tangency*) the prepass still runs a **two-part consistency gate**, both parts
**threshold-free and scale-invariant** (parametric `segEps` only, no coordinate
tolerance), to reject the disk-vanishing failure where a coarse polyline does not
reach a crossing the exact geometry has:

1. **Count** — `sampledCrossCount(i,j)` (transverse hits strictly interior to BOTH
   sampled segments; a tangential touch at a shared vertex is interior to only one,
   so it is *not* counted) must equal the number of analytic `evCross` events.
2. **Incidence** — each analytic `evCross` must be *witnessed on its own host
   segment-pair*: the segment of source `i` carrying `e.ti` and the segment of `j`
   carrying `e.tj` must themselves cross (`analyticCrossHosted` via `segContaining`
   + `segsCrossInterior`).

Failing either part `flagDegenerate`. Pure line/line pairs are exempt (lines
reproduce exactly, so sampled == analytic — a clean shallow crossing is never
false-flagged). This is the conservative escape hatch the tangency contract already
mandates, extended from tangencies to line/curve secants: when the sampled DCEL
cannot faithfully host the exact crossings, refuse rather than bless.

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
- Coarse vs fine sampling gives the same topology for analytically-covered pairs —
  or, where the coarse sampled map cannot host the exact crossings, the
  count-consistency gate makes it conservatively `Degenerate`. A *blessed* curved
  pair always has the same (correct) topology across sampling; the verdict never
  blesses a wrong/empty topology.
- Scaling geometry tiny/huge does not change classification (scale-relative bands).
- Input order and curve reversal do not change region areas/counts.
- `Degenerate` always forces `ProfilesValid=false` and therefore `Trustworthy=false`.
- A clean supported tangency does not set `Degenerate` (once the port handling lands;
  conservatively `Degenerate` at a merged cycle-bearing vertex until then).
