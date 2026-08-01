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
`Degenerate`, which gates `Verify(ctx).Trustworthy()` false. Two consequences make
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
   `analyticCrossHosted` + `contactsResolved` + `sampledCrossingsExplained`
   consistency gate). Analytic authority
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

   *Internal-tangency finding (why it is increment-7-level, not a focused increment).*
   The current behaviour is **sound**: the consistency gate flags EVERY internal
   tangency `Degenerate` (the inner sampled polygon pokes OUTSIDE the outer near the
   contact — inherent to tangency — so a sampled crossing is left with no analytic
   contact to explain it). At the oracle's default sampling the underlying result is in fact
   correct (regions=2, exact π·R² area via the circular-segment correction + hole
   assignment), it is simply flagged. So `Sketch.Verify` never blesses a wrong internal
   tangency (a broad sweep: blessed=0, flagged=72, false-valids=0).
   Two blessing attempts both fail, for the SAME root cause: (a) relaxing
   `externalCurvedTangency` to record an exactPortVert → the exact-ordering face walk
   double-counts the inner at tiny-inner/coarse-spt; (b) merely SUPPRESSING the
   count-gate flag (no exact ordering, rely on hole assignment) → also double-counts at
   tiny-inner/coarse-spt, because the inner's poke-out moves the hole-assignment
   containment probe outside the coarse outer polygon, so the inner is not subtracted
   (outer reads as the full disk π·R², total π·R²+π·r²). The poke-out is the load-bearing
   obstacle: it is intrinsic to a tangency sampled as chords, and it breaks BOTH the
   shared-vertex face walk and the disjoint-style hole assignment. The robust fix is
   **exact curve fragments** between event params (the increment-7 full curved DCEL),
   which eliminate the poke-out entirely — not a closed-containment or area-consistency
   gate bolted onto the sampled map (an area gate also fails to compose with other
   geometry, since `total == π·R_outer²` only holds for an isolated pair). Until then
   internal tangency stays conservatively `Degenerate` — sound, niche, deferred.
   (A separate low-level caveat, NOT reachable through the oracle: `geom.Regions` called
   directly with an explicit `WithSegmentsPerTurn(4|5)` can bless the double-count,
   because the inner is so coarse the poke-out crossings vanish and the gate sees
   nothing. `Sketch.Profiles` always uses the adaptive default (~64+), so the oracle is
   unaffected.)

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

   *Scope refinement (the load-bearing insight that de-risks this increment).* The
   region **AREA is already exact** — `makeCycle` computes `signedPolyArea(chord) +
   Σ bulge`, where each curved fragment's bulge is an analytic, sampling-independent
   correction (`chordArcCorrection`/`chordEllipseCorrection`/`splineBulge`) keyed on
   the fragment's natural-param range, not on the polyline density. So increment 7 is
   NOT about area; it is purely about making the **topology decisions** exact. Only
   three topology decisions are still sampled, and the deferred cases (internal
   tangency, curve/curve crossings) fail on exactly two of them:
   - **Crossing incidence** — for line/circle/arc this is already analytic
     (`analyticEvents`); the count/incidence gate only *flags* when the sampled
     chords disagree (a poke-out spurious crossing, a sub-sample cap). The exact
     verdict already exists; the gate is a sampled cross-check.
   - **Containment for hole assignment** — `extract` assigns a hole to a face via
     `pointInPolygon(probe, face.dense)` on the CHORD densification. This is the
     load-bearing failure for internal tangency: the inner circle's chord polygon
     pokes outside the outer's chord polygon near the contact, so the interior
     probe falls in the cut-off sliver and the inner is not subtracted (double
     count). An **exact point-in-region test** (winding number summing each
     boundary fragment's exact subtended angle — closed form for line/circle/arc)
     is immune to the poke-out.
   - **Rotation system at shared vertices** — already exact (`sortExactPorts`,
     increment 3).

   So the tractable path to bless the deferred cases is NOT a blind `tinySeg`
   rewrite but two targeted exactness upgrades: (a) an **exact containment** test
   for hole assignment, and (b) **trusting the analytic crossing/tangency verdict**
   for curve/curve handled pairs (suppress the sampled count-gate flag once
   containment is exact, since the poke-out crossings are an artifact the exact
   verdict already classifies as a tangency/clean miss). Staged plan: **§7a — DONE**
   (`exactPointInRegion` ray-cast containment + internal-tangency blessing: the count
   gate and merged-vertex flag are skipped for an internal curved tangency
   (`internalCurvedTangency`), the shared contact is port-ordered like an external
   one, and hole assignment uses the exact ray-cast so the inner nests into the outer
   — annulus π·(R²−r²) + inner disk π·r², exact at every sampling, tiny inner and
   merged/cardinal contact included; disjoint-nested and mixed line+curve containment
   unchanged, the whole-uncut-circle seam handled). **§7b — designed below**: lift
   the curve/curve crossing deferral behind the same exact-containment +
   analytic-authority basis. §7c (only if needed) replace `BoundaryEdge.Polyline`
   topology with exact fragments for the residual ellipse/spline cases. Each stage
   is independently testable against the soundness invariant (blessed ⇒ correct,
   else `Degenerate`).

### §7b — lift the curve/curve crossing deferral (design)

Status: **designed, not implemented**. Sibling design:
`docs/coincident-carrier-resolution-design.md` resolves the coincident-carrier
(`evOverlap`) case this section does not touch — a transverse crossing and a
coincident carrier are different `analyticEvents` classifications with
different fixes. A curve/curve transverse crossing (both
sources circle/arc) is the last case increment 2 left on the sampled path
(the `nCross > 0 && isCurvedKind(…) && isCurvedKind(…)` deferral branch in
`analyticPrepass`, `geom/arrange.go`; "Scope of analytic authority" above): the
sampled topology is already correct, but every fragment either source contributes
reports `TExact = false` (`cut.exact` stays `false` on a sampled cut —
`geom/arrange.go:186-195`), which blocks any consumer whose admission gate
requires `TExact` before it will record a fragment structurally. Probe case B
in `.tmp/decad-2d-region-asks/probe/main.go` (a chord circle crossing the hub
circle) is the concrete demonstration: right region count and areas, `TExact`
false on every fragment of both circles.

#### The tension in the existing text, and its resolution

Increment 3's own wording (above) says lifting this needs a richer
"hostability certificate" — fragment incidence, full **port order** at every
event vertex, and closed containment. The §7 scope refinement, written later,
narrows this to two remaining exactness upgrades (containment, done in §7a;
"trusting the analytic crossing verdict", not yet done) and explicitly folds
port ordering into "already exact (`sortExactPorts`, increment 3)" — treating
it as *covered*, not as a remaining requirement for the crossing case.

Reading the code resolves which is right, because it answers a narrower
question than "does the certificate hold in general": **does a transverse
crossing ever need exact port ordering at all?** `sortExactPorts` exists to
break a tie — at a certified tangency the two curves' chord departure angles
from a shared vertex are equal (a double root: same point, same direction), so
`buildGraph`'s plain chord-angle sort cannot tell which pairing of incoming/
outgoing half-edges keeps the two loops apart, and branch-swaps them. A
transverse crossing has no such tie: two distinct curves crossing at a point
depart in four genuinely different chord directions (a `evCross` is a simple
root, not a double one), so at ANY sampling density the four chord angles at
the crossing vertex already order correctly — this is exactly why the sampled
path resolves curve/curve crossings correctly today, with no analytic help at
all. `useExactPorts` (`geom/arrange.go`) already encodes this scope:
it applies exact tangent ordering only at a vertex in `exactPortVerts`, which
`analyticPrepass` populates *only* for a certified tangency contact (its lone
`a.exactPortVerts = append(…)` site) — never for a crossing. So the "full port
order at every event vertex" clause of the increment-3 certificate was written before
the tangency/crossing distinction was drawn this finely; a crossing vertex
never needed it, and nothing here proposes adding it.

What a crossing DOES need — incidence (does the sampled map actually cross
where the exact geometry does) and containment (if the crossing produces a
nested/hole relationship, is the hole assigned correctly) — is already built,
and built **pair-generically**: `analyticCrossHosted` / `contactsResolved` /
`sampledCrossingsExplained` (`geom/arrange.go`) run today for every
*line-involved* curved pair reaching the consistency gate (the `if` in
`analyticPrepass` that calls all three, same file), and none of their logic
special-cases "one operand is a line" — `sampledCrossingsExplained`'s per-source
tolerance already takes `segLen` from EITHER source when
it is curved, exactly the shape a two-curved-source pair needs. Containment is
`exactPointInRegion` (§7a, done), also pair-generic. So the §7 scope
refinement's narrower basis is the one the code already supports: **lift the
deferral by routing curve/curve crossings through the SAME gate the
line-involved path already uses**, not by building a new certificate.

#### Mechanism

In `analyticPrepass` (`geom/arrange.go`), the block

    if nCross > 0 && isCurvedKind(si.kind) && isCurvedKind(sj.kind) {
        if ambiguous { ... flagDegenerate ... }
        continue
    }

currently exits before the pair is marked `handled` and before the
consistency gate runs. The lift removes this special case so a curve/curve
pair with `nCross > 0` falls through to the same path a line/circle or
line/arc pair already takes: `a.handled[[2]int{i,j}] = struct{}{}`, the
`analyticCrossHosted`/`contactsResolved`/`sampledCrossingsExplained` gate
(unmodified), and — for each `evCross` — `applyAnalyticCut` on both sources at
the shared exact event point (unmodified; `applyAnalyticCut` already handles
this generically by source index, not by kind). A pair that fails the gate
still `flagDegenerate`s, exactly as a line-involved pair does today — the
conservative fallback is unchanged, only the *class* of pair reaching it
grows. No change is needed to `split`, `makeCycle`, `vertexCertifies`, or
`BoundaryEdge`/`cycFrag` construction: they already treat an exact cut on a
circle/arc source generically (the `TExact`/`Whole` machinery documented on
`geom.BoundaryEdge.TExact` and in the `CLAUDE.md` `profiles.go` row does not
distinguish "the other source was a line" from "the other source was a
circle").

#### The open question the design does not resolve by reading alone

The three-part gate is written pair-generically, but it has never been
*exercised* against curve/curve data with analytic authority taken — the
`round-2` regression (`TestAnalyticCircleCircleSecantDeferredToSampled`,
`geom/arrange_analytic_test.go:136-186`) and the ~18%-at-spt-16 false-flag
measurement cited in "Scope of analytic authority" above were both measured
*before* `analyticCrossHosted`/`contactsResolved`/`sampledCrossingsExplained`
existed in their current form (they were the reason increment 2 deferred
curve/curve crossings in the first place, and increment 2 predates the gate).
Whether the gate — built and tuned against line-involved pairs — rejects the
round-2 geometry and holds the false-flag rate near the sampled path's ~0%
when applied unmodified to curve/curve pairs is not something reading the
source settles; it is what implementing the mechanism above and running it
against exactly those two adversarial cases establishes. This is recorded as
an open decision below, not left as an unstated risk.

#### Acceptance criteria (repository terms)

- `geom.Regions` on probe case B's five entities returns the same region
  count and areas it does today, with every `TExact` flipped from `false` to
  `true` (mirrors the note's own acceptance statement).
- `Sketch.Profiles()` on the equivalent sketch reports `TExact = true` on
  every `BoundaryEdge` bounded by the newly-analytic crossing, with `Partial`
  unchanged (topology is not supposed to move).
- Coarse-vs-fine sampling agreement: `geom.WithSegmentsPerTurn` swept across a
  wide range produces the identical region count and area for a blessed
  curve/curve crossing (the existing invariant in "Invariants every increment
  must hold" below, now covering the newly-lifted case).
- Scale invariance: unchanged from the existing invariant list, now asserted for
  curve/curve pairs specifically. Reversal invariance is not on this list and
  cannot be: the operands here are circles and arcs, and neither has a reversed
  representation (`geom.Arc` is `{Center, Start, End}` with no direction flag and
  `Sweep() ∈ (0, 2π]`, so swapping an arc's endpoints builds the complementary
  arc, a different curve).
- The round-2 fusion regression (`TestAnalyticCircleCircleSecantDeferredToSampled`'s
  exact geometry) stays non-degenerate with the correct three regions across
  the same `spt` sweep, now via the analytic-authoritative path rather than
  the sampled fallback it currently exercises.
- The false-flag rate on well-separated curve/curve crossings (the sweep the
  same test already runs at `spt >= 8`) does not regress from the sampled
  path's baseline — a crossing that was clean before the lift must still read
  clean after it.

#### Tests

- `geom/arrange_analytic_test.go`: extend
  `TestAnalyticCircleCircleSecantDeferredToSampled` (or add a sibling with a
  name reflecting the new behavior, e.g. `TestAnalyticCircleCircleCrossingExact`)
  to assert `arr.Degenerate == false`, the same region/area invariants it
  checks today, AND `TExact == true` on every returned fragment of both
  sources — the round-2 geometry and the transverse-band sweep it already
  exercises are the adversarial cases this lift must survive, so reuse them
  rather than writing new geometry.
- `geom/arrange_analytic_test.go`: a new test asserting `TExact` flips to
  `true` on a plain, well-separated circle/circle secant (mirrors
  `TestAnalyticCircleChordSamplingStable`'s sampling-stability assertion but
  for a curve/curve pair) across a `WithSegmentsPerTurn` sweep.
- `profiles_test.go` (root package): a sketch-level equivalent of probe case
  B, asserting `Sketch.Profiles()` reports `TExact = true` — the consumer-
  facing surface the note's admission gate actually reads.
- `geom/arrange_analytic_test.go`: a false-flag sweep over well-separated
  circle/circle and arc/arc crossings at varying angle/distance/`spt` (the
  same shape as the existing internal-tangent and shallow-secant sweeps in
  this file), asserting `Degenerate == false` for every case that was clean
  under the sampled path — the regression guard for the 18%-at-spt-16 number
  cited in "Scope of analytic authority".

#### Open decisions

- **Whether the existing gate, applied unmodified, is sufficient for
  curve/curve pairs, or needs strengthening.** The code-level argument above
  (crossings never needed port ordering; incidence/containment are already
  pair-generic) argues the unmodified gate suffices. This is not certified by
  reading; it is certified by implementing the mechanism above and running it
  against the round-2 regression and the false-flag sweep (the two tests
  above are the acceptance evidence, not a separate follow-up). **If either
  fails** — the gate blesses the round-2 fusion, or the false-flag rate does
  not return to the sampled path's baseline — the fallback is to hold the
  deferral for the specific sub-case the failure isolates (e.g. gate the lift
  on an additional geometric condition, such as excluding near-internal pairs
  where the round-2 case lives) rather than abandoning the lift entirely,
  since the acceptance tests above pin exactly which cases must stay sound.

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
*tangency*) the prepass still runs a **three-part consistency gate**, to reject the
disk-vanishing failure where a coarse polyline does not reach a crossing the exact
geometry has. The first two parts are **threshold-free and scale-invariant**
(parametric `segEps` only); the third measures against the sampling's own chord
length, which shrinks with density.

Which of the first two applies to a contact turns on whether it INSERTS a vertex
(`crossNeedsSampledWitness`, answered by `cutSite` — the same call
`applyAnalyticCut` acts on, so the gate can never expect something the cut phase
does not do):

1. **Incidence** — an `evCross` that inserts a new vertex on BOTH sources must be
   *witnessed on its own host segment-pair*: the segment of source `i` carrying
   `e.ti` and the segment of `j` carrying `e.tj` must themselves cross
   (`analyticCrossHosted` via `segContaining` + `segsCrossInterior`). This is the
   disk-vanishing case: the injected point sits off the chord by up to the sagitta,
   and the sampled crossing is the evidence that bending the polygon there does not
   push it through the other curve.
2. **Resolution** — a contact the sampled map ALREADY has a vertex for (at a
   source's own endpoint, or at an interior sample vertex) inserts nothing, so it
   has nothing to witness and no witness is possible: a contact at a segment
   boundary is not interior to that segment. Every contact must instead be
   SEPARATED by the sampling — two inside one chord of a curved source is a
   sub-sample cap (`contactsResolved`).
3. **Explanation** — no sampled crossing may be left over: each must sit within one
   curved chord of some analytic contact (`sampledCrossingsExplained`). A crossing
   with nothing behind it is the chord approximation disagreeing with the geometry
   — two arcs that merely touch slicing through each other — and the face walk has
   no vertex for it. The chord bound is what the approximation's own error spans,
   and a line contributes none (its polyline IS the line).

Failing any part `flagDegenerate`. Pure line/line pairs are exempt (lines reproduce
exactly, so sampled == analytic — a clean shallow crossing is never false-flagged).
This is the conservative escape hatch the tangency contract already mandates,
extended from tangencies to line/curve secants: when the sampled DCEL cannot
faithfully host the exact crossings, refuse rather than bless.

Requiring a sampled witness for a contact that inserts no vertex is what once
false-flagged everyday arrangements — a line ending exactly on a circle (the gear
flank meeting its root circle, and what a point-on-circle constraint builds), a
chord crossing where the sampling happens to put a vertex, a corner join — since
that witness can never exist. Parts 2 and 3 are what keep the relaxation sound: a
line whose BOTH ends lie on a circle inside ONE chord runs through the sliver
outside the polygon, and blessing it collapses the map.

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
- Input order does not change region areas/counts, and neither does reversing a
  curve that HAS a reversed representation (a line, a spline). An arc does not:
  `geom.Arc` is `{Center, Start, End}` with no direction flag and `Sweep() ∈
  (0, 2π]`, so swapping its endpoints builds the complementary arc rather than
  the same one authored backwards.
- `Degenerate` always forces `ProfilesValid=false` and therefore `Trustworthy=false`.
- A clean supported tangency does not set `Degenerate` (once the port handling lands;
  conservatively `Degenerate` at a merged cycle-bearing vertex until then).
