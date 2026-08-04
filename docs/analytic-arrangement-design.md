# Analytic Arrangement — Design & Increment Plan

Status: **in progress** — increments 1 (the analytic event kernel,
`geom/arrange_events.go`) and 2 (the analytic-authoritative wiring,
`geom/arrange.go`) are implemented, and increment 3 (exact tangent/port ordering)
is **partly** in — a merged-vertex EXTERNAL circle/arc tangency is now blessed as
two disks via curvature-ordered ports; internal/containment tangency is blessed
via §7a's exact containment; osculation and line-involved merged tangency remain
deferred. **Curve/curve transverse crossings are no longer deferred**: §7b is
implemented, so a circle/arc × circle/arc crossing takes analytic authority
whenever its own incidence certificate passes, and falls back to the sampled path
otherwise. Exact parameter bounds are additionally gated on the WHOLE SCENE being
line/circle/arc (§7b, "The all-analytic gate"), so a scene containing any
free-form curve reports `TExact = false` everywhere. The rest is the roadmap
below. Resolves the "analytic (non-sampled) arrangement" open follow-up of the
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
   sampling-stable. Increment 2 left **curve/curve transverse crossings on the sampled
   path**; §7b lifted that behind an incidence certificate of their own (see "Scope of
   analytic authority", which records both the original reasoning and what replaced
   it). A line-involved curved pair whose exact crossing
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
   (deferred): line-involved merged tangency and a genuine **osculation** (equal
   tangent AND equal curvature). **Internal/containment** tangency is blessed by §7a's
   exact containment, and curve/curve transverse **crossing** authority by §7b's
   incidence certificate — both implemented. The richer per-event **hostability
   certificate** once sketched here — fragment **incidence** (the emitted straight
   fragments have no extra/missing crossings vs the analytic event set), full **port
   order** at every event vertex, and **closed containment** (a nested/internally-
   tangent inner cycle certified inside the outer) — was narrowed by §7b: a transverse
   crossing needs incidence, not port order (see §7b), and containment landed in §7a.

   *Internal-tangency finding (what made it increment-7-level, and how §7a resolved
   it).* The load-bearing obstacle was the **poke-out**: the inner sampled polygon
   crosses OUTSIDE the outer near the contact, which is intrinsic to a tangency drawn
   as chords, and it breaks two things at once — the shared-vertex face walk and the
   disjoint-style hole assignment. That is why neither cheap blessing worked. (a)
   Relaxing `externalCurvedTangency` to record an exactPortVert left the exact-ordering
   face walk double-counting the inner at tiny-inner/coarse-spt. (b) Merely SUPPRESSING
   the consistency-gate flag (no exact ordering, rely on hole assignment) double-counted
   there too, because the poke-out moves the hole-assignment containment probe outside
   the coarse outer polygon and the inner is never subtracted (outer reads as the full
   disk π·R², total π·R²+π·r²). Bolting a closed-containment or area-consistency gate
   onto the sampled map is no fix either: an area gate does not compose with other
   geometry, since `total == π·R_outer²` only holds for an isolated pair.
   §7a removes the obstacle rather than working around it. Hole assignment stopped
   reading the chord polygon at all — `exactPointInRegion`'s ray-cast is immune to the
   poke-out — so the same exact tangent-port ordering that blesses the external case
   blesses this one. `internalCurvedTangency` exempts the pair from the consistency
   gate and certifies its shared contact, and the inner nests into the outer as an
   annulus π·(R²−r²) plus an inner disk π·r², exact at every sampling
   (`TestAnalyticInternalTangentBlessed` and `TestAnalyticInternalTangentTinyInnerBlessed`
   pin it; `TestAnalyticInternalTangentNeverBlessedWrong` pins blessed ⇒ correct). The
   low-level `WithSegmentsPerTurn(4|5)` double-count the sampled containment could once
   bless went with it — measured at two regions netting π·R² down to `densify`'s
   two-segment floor.

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
   three topology decisions are still sampled, and the cases §7a and §7b went on to
   bless (internal tangency, curve/curve crossings) turned on exactly two of them:
   - **Crossing incidence** — for line/circle/arc this is already analytic
     (`analyticEvents`); the three-part consistency gate only *flags* when the sampled
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
   for curve/curve handled pairs (suppress the sampled consistency-gate flag once
   containment is exact, since the poke-out crossings are an artifact the exact
   verdict already classifies as a tangency/clean miss). Staged plan: **§7a — DONE**
   (`exactPointInRegion` ray-cast containment + internal-tangency blessing: the
   consistency gate and merged-vertex flag are skipped for an internal curved tangency
   (`internalCurvedTangency`), the shared contact is port-ordered like an external
   one, and hole assignment uses the exact ray-cast so the inner nests into the outer
   — annulus π·(R²−r²) + inner disk π·r², exact at every sampling, tiny inner and
   merged/cardinal contact included; disjoint-nested and mixed line+curve containment
   unchanged, the whole-uncut-circle seam handled). **§7b — DONE, described below**:
   the curve/curve crossing deferral is lifted behind a per-pair incidence
   certificate, with exact parameter bounds gated on an all-analytic scene. §7c (only
   if needed) replace `BoundaryEdge.Polyline`
   topology with exact fragments for the residual ellipse/spline cases. Each stage
   is independently testable against the soundness invariant (blessed ⇒ correct,
   else `Degenerate`).

### §7b — the curve/curve crossing lift (implemented)

Status: **implemented** (`analyticCrossingsCertified` and the `exactAllowed` scene
gate, `geom/arrange.go`). The design reasoning below is kept because the shipped
mechanism differs from what it proposed: reusing the line-involved three-part gate
did NOT hold for two sampled curves, so the pair takes analytic authority behind its
own incidence certificate instead, and exact bounds carry a whole-scene precondition
the original design did not anticipate. Both are described under "What shipped".
Sibling design:
`docs/coincident-carrier-resolution-design.md` resolves the coincident-carrier
(`evOverlap`) case this section does not touch — a transverse crossing and a
coincident carrier are different `analyticEvents` classifications with
different fixes. A curve/curve transverse crossing (both sources circle/arc) was
the last case increment 2 left on the sampled path: the sampled topology was
already correct, but every fragment either source contributed reported
`TExact = false`, which blocks any consumer whose admission gate requires
`TExact` before it will record a fragment structurally.

#### What shipped

**The per-pair incidence certificate** (`analyticCrossingsCertified`). A
curve/curve pair with at least one `evCross` takes analytic authority only when
splicing its exact crossing points into BOTH sampled polylines — at the site the
cut phase itself uses (`postCutPolyline` over `cutSite`) — leaves the sampled map
with the same crossing incidence. Three conditions, checked on the spliced
polylines: they meet ONLY at those points — every contact between a segment of one
and a segment of the other belongs to a crossing point BOTH segments are incident to
(`polylinesMeetOnlyAtSharedVertices`); each contact IS the polyline vertex it was
mapped to, within the identity band `vertexCertifies` uses, bounded by the vertex's
own chord as well as by the scene (`contactIsVertex`); and the four chord departures
at each injected point ALTERNATE between the sources (`portsCross`), so meeting at a
point is distinguished from crossing at it. The first and third are threshold-free;
only the second compares a position, and it decides only "one point or two". A
contact at an open source's own endpoint contributes three departures, not four, and
is refused by construction at every density.

The first condition is decided COMBINATORIALLY, by shared incidence: contact `k` is
the vertex `ci[k]` of one polyline and `cj[k]` of the other (`postCutPolyline`'s
second return), and a contacting segment pair passes exactly when some `k` is an end
of both segments. Two segments sharing an endpoint meet only there unless they are
collinear, and `collinearOverlap` refuses collinearity first, so shared incidence
admits exactly the contacts that ARE an injected crossing point — with no tolerance,
no scene scale and no chord length in the verdict. Comparing the contact's POSITION
against the injected points needs a band, and the only one available is
`weldIdentEps·scale`, the whole scene's bounding-box extent: unlike `contactIsVertex`
(chord-local) and `vertexCertifies` (source-local), this condition has no single
source or vertex to state a local yardstick against, so a distant unrelated object
widened what counted as one point here — measured, two circles crossing at 0.01 rad
5e-13 past a sample vertex are refused alone and certified once a line is drawn 60
units away, and the difference reaches `geom.Regions`. A chord-local band cannot
replace it either: the chord/chord intersection `segParams` computes is displaced
from the true crossing by roughly the sagitta over the sine of the crossing angle,
which no chord length bounds.

Deciding it by position ALONG THE HOST SEGMENTS is a third, different question — it
admits contacts that carry no node in the map: a source's own endpoint resting on the
interior of the other's chord, and a transverse pass-through landing on a sample
vertex of one polyline. Parallel pairs are covered separately — `segParams` rejects
them on the determinant before any range test, so a collinear overlap reached that
check as silence — by `collinearOverlap`, an overlap of positive length being no
transverse crossing.

Shared incidence is not a strict superset of the position test. Where an exact
crossing is spliced within round-off of an existing sample vertex, the splice leaves
a near-degenerate segment, and the other polyline then meets BOTH of that vertex's
neighbours at what the map welds into one graph vertex; incidence sees two segments
and refuses. The refusal is on the safe side — the pair takes the sampled fallback
and publishes no exact bound — and it was measured at 2 of 20000 evaluations in a
sweep aimed squarely at that regime, against 133 refusals it lifts, and 0 of 5568 in
an ordinary circle/circle and arc/arc sweep.

The second condition's band is bounded by TWO yardsticks, the scene's extent and the
vertex's own chord, the shape `carriersIdentical` uses in the coincident-carrier
design and for the same reason. On the scene extent alone an unrelated distant object
widens it: with `r = 5` the verdict flips at a scene extent of about `24.5·r`, so one
construction line ~100 units away certified a contact `1.1e-10` off its vertex that
the same pair refuses when drawn alone, publishing the vertex's sample fraction as an
exact crossing parameter. The chord is the only length the mapping decision is stated
in, and nothing outside the pair can inflate it.

`vertexCertifies` and `endpointReproduces` — the final exactness audit, after
canonicalization — carry the same two yardsticks, the scene's extent and the SOURCE's
own extent (`source.extent`, that source's polyline bounding box, the scene formula
applied to one curve). The same defect reached them: a circle of radius 5 whose exact
crossing welds `1e-9` onto one of its own sample vertices reports that bound inexact
when drawn with its chord alone, and one unrelated line at `x = 1000` flipped it to
exact — through `Sketch.Profiles()` with no options at all, publishing a parameter that
misses its own polyline endpoint by the whole `1e-9`. The gap is a displacement along
the curve whose parameter is being certified, so the curve's own size is what it is
judged against (`TestExactBoundIdentityBandIsSourceLocal`, and the white-box
`TestExactIdentityBandIsSourceLocal` / `TestEndpointReproductionBandIsSourceLocal`).
Nothing else moves: topology, areas, `TStart`/`TEnd`, `Whole` and `Degenerate` are
untouched, and a scene of line/circle/arc geometry with nothing distant in it keeps
every exact bound it had.

**The fallback is the sampled path, never a degeneracy.** An uncertified pair is
left unhandled exactly as before the lift, so it keeps the sampled topology with
`TExact = false`, and no arrangement blessed before the lift is refused after it.
A refused crossing is recorded (`deferredCross`) and reconciled after the sampled
loop against the contacts that loop actually made (`sampledRepresents`); a crossing
the sampled map does not carry withdraws exactness from every source of its
connected component (`refuseExactOnFusedMap`), since a fused crossing moves the
face boundaries of every cycle it takes part in.

**The all-analytic gate** (`exactAllowed`). Exact bounds are published only when
EVERY source in the arrangement is a line, circle or arc. One ellipse, elliptical
arc, conic, spline, closed spline, fit spline or NURBS anywhere in the scene makes
every bound of that arrangement report `TExact = false` — the lines, circles and
arcs beside it included, however far apart they sit, and the free-form curve's own
uncut whole edge included.

The gate exists because a free-form source reaches the map only as chords. A curve
with a lobe between two consecutive samples crosses another curve entirely between
them — measured on a knot-clustered degree-3 NURBS at the default sampling: a
midpoint-sampled deviation of 2.1e-05 against a true 4.7e-01 maximum on the same
segment. The crossing is then missing from the map, the regions it separates fuse,
and the scene's certified pairs publish the fused map with every bound exact.
Gating on the SOURCE KINDS answers that with no threshold; in an all-analytic scene
there is no sampled-only pair at all, so only the refused-crossing reconciliation
above remains, and its bound errs toward withdrawing exactness.

The near-miss guard below now reports that fusion as `Degenerate`, but it does not
lift this gate: it says where a crossing CANNOT be ruled out, not that the crossing
set is right where it stays silent, and a free-form crossing's parameter is a
sampled one whatever the topology.

**The coverage cost is understood and accepted**: a scene containing any free-form
curve loses exact bounds everywhere in that scene, so what is given up is exactness
on the analytic sources sharing the scene with it. Topology, areas and the reported
ranges are untouched. Lifting it needs a sampler that certifies its own per-source
deviation (a change to `densify`), not a wider estimate at the point of use — a
separate change, not planned here.

**The near-miss guard** (`geom/nearmiss.go`, `nearMissGuard`). The gate above
withholds *exactness* from a scene whose map may be missing a crossing; this
reports the missing crossing itself as `Degenerate`, so `Region.Degenerate`,
`Arrangement.Degenerate`, `Profile.Valid`, `VerificationReport.ProfilesValid` and
`Trustworthy()` stop blessing a region set that may have lost one.

Every tiny segment carries a PROVEN upper bound on how far its source departs from
that segment's own chord (`arranger.segDev`, filled by `densify`):

| family | bound | why it is sound |
|---|---|---|
| line | 0 | the polyline IS the line |
| circle, arc | `r·(1−cos(Δθ/2))` | the exact sagitta; `densify` floors at two segments so `Δθ ≤ π` |
| ellipse, elliptical arc | `Δφ²·max(rx,ry)/8` | linear-interpolation error `h²·M/8`; the second derivative in the eccentric angle is `−rx·cos φ·u − ry·sin φ·v`, norm ≤ `max(rx,ry)` everywhere |
| conic | max distance from the sub-span's rational-quadratic control points to the chord | de Casteljau in homogeneous coordinates; positive weights make every curve point a convex combination of the projected controls |
| spline, NURBS | same, over the sub-span's control points after inserting both bounds to multiplicity `p` | Boehm knot insertion refines the representation, not the curve; the convex hull holds for the rational curve too |
| closed spline, fit spline | same, over each overlapped piece's cubic Bézier control points | the periodic uniform basis and the natural-cubic piece each convert to Bézier exactly |

Distance to a chord segment is convex, so its maximum over a convex hull is
attained at a hull point — that is what makes the hull bounds bounds and not
estimates. A family that could prove none would report unbounded and flag.

Two segments whose chords approach within the SUM of their bounds may have the true
curves crossing between them — twice, so the chords need not cross and nothing is
recorded. Where no contact the sampled map recorded (`sampledContacts`: a
chord/chord crossing, or a weld of two sample vertices) sits within that same band
of the approach, the pair is flagged. The window is the band, not one chord: a
chord window forgives a grazing lens narrower than one chord, and over the hider ×
partner matrix left the wrong region count unflagged in 30 of 45 combinations.

Scoped to pairs with at least one free-form source. Line/circle/arc pairs are
classified by the closed-form kernel and need no band, so an all-analytic scene is
untouched and pays nothing — the loop never reaches its segments.

**What it does not answer.** It asks whether a crossing was RECORDED near an
approach, not whether the recorded crossing COUNT is right. A lens narrower than
the band whose chords do cross is explained by its own crossing and stays silent,
so the wrong region count that comes from a sub-sample cap is still unflagged at
densities where the chords meet. Measured over the hider × partner matrix at 45
separations each, 27% of the wrong region counts are flagged. Requiring the
explaining contact to be RESOLVED as well (the `contactsResolved` rule, transposed
to the sampled path) covers all of them, and was measured to cost an 8.5%
false-flag rate on ordinary geometry — plain fit-spline arches closed by a chord —
so it was not taken.

#### Design record: the tension in the earlier text, and its resolution

*(The remainder of this section is the design as written before implementation.
Where it and "What shipped" disagree, "What shipped" is the code.)*

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

#### Mechanism (proposed; superseded by the certificate)

This proposal — route curve/curve crossings through the line-involved three-part
gate unchanged — was tried and did not hold: `analyticCrossHosted` looks for the
sampled witness on the very segment pair carrying the crossing's two source
parameters, while the sampled crossing sits off the exact one by roughly
sagitta/sin(crossing angle). With two sampled curves both grids can be off, so the
miss rate aliases against them instead of falling with density (~7% of a
well-separated angle/distance sweep flagged, isolated `spt` values failing into the
hundreds). The shipped pair certificate asks the incidence question directly on the
post-cut polylines instead; see "What shipped".

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

#### The open question the design did not resolve by reading alone (answered: no)

The three-part gate is written pair-generically, but it has never been
*exercised* against curve/curve data with analytic authority taken — the
`round-2` regression (`TestAnalyticCircleCircleSecantDeferredToSampled`, since
renamed `TestAnalyticCircleCircleCrossingCertified` in
`geom/arrange_analytic_test.go`) and the ~18%-at-spt-16 false-flag
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

*Answered by implementing it: the unmodified gate is NOT sufficient.* It kept the
round-2 geometry sound but false-flagged well-separated crossings at a rate that did
not fall with density, so the lift ships behind its own incidence certificate.

#### Acceptance criteria (as written before implementation)

The first two were written before the all-analytic gate existed and hold only for a
scene whose every source is a line, circle or arc. In a scene containing a free-form
curve, `TExact` stays `false` on every bound by design — see "What shipped".

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

#### Tests (as proposed)

What shipped, in `geom/arrange_analytic_test.go` unless noted:
`TestAnalyticCircleCircleCrossingCertified` (the round-2 geometry, now certified),
`TestAnalyticArcArcCrossingCertified`, `TestAnalyticCurveCrossingNeverBlessedWrong`
(the blessed ⇒ correct sweep), `TestAnalyticFusedCurveCrossingNotBlessedExact` and
`TestAnalyticFusedComponentLeavesUntouchedClusterExact` (the fused-map withdrawal),
`TestFreeFormSourceWithholdsExactBoundsSceneWide` (the all-analytic gate and its
coverage cost), `TestBoundaryEdgeExactInvariantFreeform`
(`geom/arrange_exactness_test.go`), and the sketch-level pair in
`profiles_params_test.go` ("a circle/circle crossing is exact" and "a free-form
entity withholds exact bounds across the whole sketch").

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

#### Open decisions (resolved)

- **Whether the existing gate, applied unmodified, is sufficient for
  curve/curve pairs, or needs strengthening.** *Resolved: it is not; the lift
  ships behind its own incidence certificate (see "What shipped").* The original
  reasoning follows. The code-level argument above
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

- **Curve/curve transverse crossings** (BOTH sources circle/arc, ≥1 `evCross`) take
  analytic authority behind **their own incidence certificate**
  (`analyticCrossingsCertified`, §7b) — not through the three-part gate below, which
  does not carry to a pair of sampled curves. A pair the certificate refuses is left
  unhandled and the sampled loop processes it, exactly as increment 2 did for every
  such pair: the sampled DCEL resolves the topology and the fragments report
  `TExact = false`. Increment 2 deferred the whole class because injecting exact cuts
  through that gate was either **unsound** (two equal-count coarse crossings at the
  *wrong* locations fuse three regions into one — a real round-2 bug) or
  **over-conservative** (a valid well-separated crossing whose sampled crossing sits
  one chord segment off the analytic param gets false-flagged — ~7% of a
  well-separated sweep, against the sampled path's 0%); the certificate is what
  answers both. A genuinely ambiguous verdict still `flagDegenerate`s.
- **Line-involved crossings + all tangencies** keep analytic authority (the wins:
  shallow line/line not degenerate, tangent line+circle → one disk, non-merged
  tangent circles → two disks, chord-through-circle exact area). A line operand is
  reproduced exactly, so its sampled crossing tracks the analytic one — there is no
  wrong-location failure mode and no over-conservatism (measured ~0.3%, all genuine
  near-tangents).

For a handled pair with a curved source that the §7b certificate does not own (i.e.
line/circle, line/arc, or a curved *tangency*) the prepass still runs a **three-part
consistency gate**, to reject the
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
- An exact parameter bound is published only when EVERY source in the arrangement is a
  line, circle or arc; one free-form source withholds `TExact` scene-wide (§7b).
- Coarse vs fine sampling gives the same topology for analytically-covered pairs —
  or, where the coarse sampled map cannot host the exact crossings, the
  three-part consistency gate (incidence, resolution, explanation) makes it
  conservatively `Degenerate`. A *blessed* curved pair always has the same
  (correct) topology across sampling; the verdict never blesses a wrong/empty
  topology.
- Scaling geometry tiny/huge does not change classification (scale-relative bands).
- Input order does not change region areas/counts, and neither does reversing a
  curve that HAS a reversed representation (a line, a spline). An arc does not:
  `geom.Arc` is `{Center, Start, End}` with no direction flag and `Sweep() ∈
  (0, 2π]`, so swapping its endpoints builds the complementary arc rather than
  the same one authored backwards.
- `Degenerate` always forces `ProfilesValid=false` and therefore `Trustworthy=false`.
- A clean supported tangency does not set `Degenerate` (once the port handling lands;
  conservatively `Degenerate` at a merged cycle-bearing vertex until then).
