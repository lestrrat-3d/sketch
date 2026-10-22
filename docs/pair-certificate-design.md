# Pair Certificate — Design

Status: **Stage 1 implemented** (`geom/arrange_pair_ledger.go`, `geom/arrange_pair_geometry.go`). **Stage 2a
(spline × circle crossings) implemented** (`geom/arrange_pair_crossing.go`). The rest of Stage 2 is proposed and
needs its own go-ahead. This document extends the whole-scene exactness gate described in
`docs/analytic-arrangement-design.md` (§7b, "The whole-scene gate") so that fit splines can sit in a scene with lines,
arcs and circles and still publish `TExact = true` on the fragments whose parameters are proven.

## The problem

`BoundaryEdge.TExact` is `true` only when the arranger can prove the reported `TStart`/`TEnd` are the exact
parameters of the cut points. A consumer that records a fragment structurally (decad's `UnionProfiles` path) refuses
any `Partial` fragment with `TExact = false`.

The arranger decided exactness once for the whole scene (`exactAllowed`, `geom/arrange.go`). It is `true` when every
source is a line, circle or arc. It also becomes `true` when `certifyFitCircleScene`
(`geom/arrange_fit_circle.go`) matches one fixed shape: exactly one circle, 2K fit splines and K tip arcs, no other
source. Any other scene that holds a fit spline read `TExact = false` on every fragment, including the lines, arcs
and circles in it.

Measured on `main` at `f07b327` with a synthetic scene (a root circle, 24 closed regions, each closed by two
radial lines, two 15-point fit splines, a tip arc and a root arc lying on the circle; 145 sources):

| Scene | Sources | `Profiles()` time | `TExact` on the union's outer edges |
|---|---|---|---|
| Each fit spline replaced by 14 straight segments | 769 | 34 ms | 768 of 768 true |
| Fit splines kept (before Stage 1) | 145 | 208 ms | 0 of 144 true |

The two scenes have the same topology, so the gate and not the geometry withheld exactness. Without exact circle
fragments, `UnionProfiles` also returned `profile union cannot certify a circle split` for the variant whose root side
is closed by the circle alone (`profile_union.go`, `unionCircleEndpoint`).

## Goals and non-goals

Goals:

- Certify, pair by pair, the exact relation between a fit spline and any other source, with no assumption about what
  the scene depicts.
- Withhold exactness only from the sources whose relation to some other source is unproven, not from the scene.
- Use no tolerance chosen by looking at a particular scene. Every limit is derived from binary64, from the count of
  stored numbers, or from the vertex-merge distance the planar map already welds by.

Non-goals:

- Changing how a crossing involving a fit spline is trimmed, or any area, topology or `Degenerate` result. The
  certificate changes the `TExact` flag only.
- Certifying NURBS, control-point splines, closed splines, conics or ellipses. Those keep the kind gate.
- A new public API in Stages 1 and 2.

## Terms

A **piece** is one cubic Bézier segment of a fit spline. `certifyFitCircleScene` already converts a fit interpolant
into pieces with exact rational control points (`exactFitPieces`, memoized by `memoFitPieces`).

A **hull** is the convex hull of a piece's four Bézier control points. The piece lies inside it.

A **pair verdict** is the proven relation between two sources:

| Verdict | Meaning |
|---|---|
| `Disjoint` | The two sources share no point. |
| `EndpointOnly` | The sources share exactly the listed end points. Each end is the source's own parameter 0 or 1. |
| `Simple` | Self entry only. The spline never meets itself. |
| `Unproven` | The certificate could not decide. The reason is recorded. |
| `Crossing` | The sources cross transversally at exactly the listed spline parameters. Implemented for a spline against a circle (Stage 2a). |

`Disjoint`, `EndpointOnly`, `Simple` and `Crossing` are facts about the exact curves. A verdict is **audited** when
the sampled polylines of the pair agree with it (see "Audit against the sampled map").

## The pair ledger

The ledger has one entry per unordered source pair that includes a fit spline, plus one self entry per fit spline.
A pair of two line/circle/arc sources keeps the closed-form kernel (`analyticEvents`) and has no ledger entry.

`buildPairLedger` runs at the end of `analyticPrepass`, after `certifyFitCircleScene`. It does nothing when
`exactAllowed` is already true (the narrower certificate matched), when `exactVeto` is set (a source was dropped, or
the scene extent overflowed), or when a source is not a line, circle, arc or fit spline. Otherwise it fills the
ledger and sets `exactAllowed`. `certifyFitCircleScene` stays in place and runs first. It can be retired only
after Stage 2 gives the ledger per-range verdicts and the corpus shows the ledger reproduces every bound the function
publishes.

### Which sources lose exactness

An `Unproven` pair can hide a contact, so the sources reachable from it through contacts lose exactness, the same
rule `refuseExactOnFusedMap` applies to a refused curve/curve crossing. The component is built from three edge kinds:
analytic events of handled pairs, sampled contacts (`sampledContacts`), and `Unproven` ledger entries. The result is
written into the existing per-source array `exactRefused`.

A component with no `Unproven` entry keeps every exact bound. So a spline far from the rest of the scene, and the rest
of the scene, stay exact. The cost is that a scene whose regions all touch a common circle is one component, so one
`Unproven` pair anywhere in it withholds exactness from the whole scene. Per-range verdicts in Stage 2 narrow that.

### Audit against the sampled map

The arranger still builds topology from sampled chords. A proven exact fact is useful for `TExact` only when the
sampled map agrees with it. `auditPairLedger` runs inside `refuseExactOnFusedMap`, after the sampled loop:

- `Disjoint`: the pair has no sampled contact. A gap smaller than the chord-deviation bounds of the two sources can
  still produce a chord crossing, and the pair is then `Unproven` with reason `audit`.
- `EndpointOnly`: every sampled contact lies within the vertex-merge distance (`a.merge`) of a shared end.
- `Simple`: no audit. The existing self-contact checks of the sampled loop and the near-miss guard apply as before.

Hidden crossings, the reason the kind gate exists, cannot occur in a pair whose verdict is `Disjoint` or
`EndpointOnly`, because those verdicts account for every point the exact curves share. The near-miss guard
(`geom/nearmiss.go`) stays active.

## Stage 1 — `Disjoint`, `EndpointOnly` and `Simple` (implemented)

Pair kinds: fit spline × line, fit spline × circle, fit spline × arc, fit spline × fit spline, and a fit spline
against itself.

### Exact forms

A spline is its exact rational Bézier pieces. A line is the segment between its stored end coordinates. A circle
or arc is the full carrier circle with its stored float64 centre and radius. A circle or arc is used as the full
circle by the Bernstein test below. An arc whose sweep is smaller than a half turn also carries a **wedge**: the
region swept by the rays from its centre through its two end directions, each rotated outward by `arcWedgeSlack`
(2^-40). The slack is far above the few-ulp error of `math.Cos` and `math.Sin`, which are the only inexact inputs. A
hull whose four control points lie strictly on the outer side of one bounding ray misses the arc even where its circle
continues.

### Disjoint proof

`judge` tries these tests in order and stops at the first that separates the pair:

1. The spline's bounding box against the target (`farFromBox`). For a line it is a separating-axis test of the box
   corners against the segment. For a circle or arc it is the box-against-circle test (box wholly outside or wholly
   inside) and the wedge test on the corners. For a spline target it is a box test. An arc with a wedge is first tested
   against its own box (`arcBounds`: the box of its end points and the axis extremes its sweep passes, over the sweep
   widened by twice the wedge slack, padded by 2^-36 of the radius), not the box of its whole circle. The circle's box
   overlaps every source inside it and each overlap costs exact wedge arithmetic: on 128 regions with root arcs this
   took `Profiles()` from 1.40 s to 0.85 s.
2. Per piece, the piece box against the target box, then the hull against the target: separating axes (the two
   coordinate axes and the normal of every pair of control points) for a line or a spline hull; for a circle or arc, the
   box-against-circle test, the wedge test, and the sign of the degree-6 Bernstein coefficients of
   `|P(u) - c|² - r²`. All coefficients of one strict sign mean the piece misses the circle.
3. A hull that still overlaps is split at `u = 1/2` by de Casteljau and each half is tested again. A spline against a
   spline splits both hulls. The depth limit is `ledgerMaxDepth` = 52, the number of mantissa bits in binary64: two
   stored curves closer than one part in 2^52 of their extent cannot be told apart from the stored coordinates.
   A pair still overlapping at depth 52 is `Unproven` with reason `resolution`.

All arithmetic is exact (`math/big` rationals). No test uses a margin.

For a circle target (not an arc) steps 2 and 3 do not run: Stage 2a settles the pair by exact root counting.

### EndpointOnly proof

A **shared end** is an end of the spline that lies within `weldIdentEps * scale` of an end of the other source. The
planar map welds ends that close, and the tooth certificate uses the same distance. A spline against a spline
additionally needs the two end coordinates to be bit-identical; otherwise the pair is `Unproven` with reason
`end-mismatch`. A circle has no end, so a spline end resting on a circle is never a shared end.

Let `A` be the spline and `B` the other source. Around each shared end of `A` (and of `B` when it is a spline) the
end piece is cut at the parameter `2^-k`:

- The **far part** of `A`, everything but the cut-off ends, must be disjoint from the whole of `B`, by the Disjoint
  proof above. When `B` is a spline, the far part of `B` must also be disjoint from the whole of `A`.
- The **near-end part** of `A` (the cut-off end sub-curves) must meet `B` only at the end.
  - Against a line, circle or arc, `localCarrierOK` proves it. The function that is zero on the carrier (the signed
    distance to the line, or `|P-c|²-r²`) is written in Bernstein form along the sub-curve. When every difference
    of consecutive coefficients has one strict sign, the function is strictly monotone, so the sub-curve meets the
    carrier at most once. The function changes by at least `m` per unit of the parameter, so a root is within
    `|f(apex)| / m` of the apex. The curve moves at most `S` per unit of the parameter, where `S` is three times the
    longest control-polygon edge in the L1 norm. The test requires `|f(apex)| * S <= merge * m`, so the one contact lies
    within the vertex-merge distance of the end and welds to it.
  - Against a spline, `conesApart` proves it. A cubic lies in the cone from its end control point through the other
    three. Two cones from the same apex that meet only at the apex hold curves that meet only there. The cone of a piece
    is found by exact cross products. A cone of a half turn or more, or one with only zero vectors, fails.

The cut index `k` is the smallest one, from 1 (2 when both ends of a one-piece spline are cut), for which the near-end
part passes. A smaller cut leaves a larger far part's neighbourhood clear, and growing `k` only enlarges the far part,
which can only make its disjointness harder. So the far part is tested once, at that `k`. If the near-end test fails for
every `k` up to 52 the pair is `Unproven` with reason `tangent`. A smooth join of two splines in opposite directions
at a shared end passes the cone test. Two splines leaving a point in the same direction do not.

### Simple self entry

A cubic spline whose every Bézier control vector has a strictly positive dot product with its chord (end point minus
start point) has a coordinate that strictly increases along the curve, so it cannot meet itself. That is
`verdictSimple`. Any other spline is `Unproven` with reason `self-contact`. The test is conservative: a spline that
turns more than a half turn, or that wanders against its chord, is refused even when it is simple. Stage 2 can
replace it with piece-pair tests.

### What Stage 1 delivers

A scene in which every spline relation is `Disjoint` or `EndpointOnly`, and every spline is `Simple`, publishes exact
bounds on all fragments, including the circle fragments that `UnionProfiles` splits. That is the scene
decad-gallery-c8 described: a disc circle plus N closed regions, each of two lines that start on the circle, two fit
splines that start outside it and never cross it, a tip arc, and optionally a root arc lying on the circle. The test
`TestUnionProfilesCertifiesSplineSections` builds it for N = 8, 12, 17 and 24, with 4 and 15 fit points per flank, with
and without the root arc, and requires `UnionProfiles` to succeed with every `Partial` edge `TExact`.

### Measured cost

`BenchmarkProfilesSplineSections` (`profile_union_spline_sections_test.go`): `Sketch.Profiles()` on 24 regions with
15-point splines and root arcs (145 sources), AMD Ryzen 9 7900X3D, Go 1.26.8, three runs of ten iterations each.

| Build | ns per `Profiles()` call |
|---|---|
| `main` at `f07b327` (no exact bounds on this scene) | 205 100 000 to 206 900 000 |
| Stage 1 | 258 600 000 to 260 400 000 |

Stage 1 adds about 54 ms, 26 percent, to a call. The same scene at other region counts, one `Profiles()` call
each (so less precise than the benchmark):

| Regions | Sources | `main` | Stage 1 |
|---|---|---|---|
| 24 | 145 | 211 ms | 300 ms |
| 60 | 361 | 1 096 ms | 1 450 ms |
| 120 | 721 | 4 178 ms | 5 401 ms |

The added time grows with the base time, 25 to 42 percent, because the sampled arrangement itself grows with the
square of the source count. The spline-involved pairs of this scene number 5 832 (48 splines
against 97 other sources, 1 128 spline pairs, and 48 self entries). The source-level box tests resolve nearly all of
them without looking at a piece.

## Stage 2a — spline × circle `Crossing` (implemented)

Pair kinds: fit spline × circle. For a circle target `judge` runs `crossCircle` right after the whole-spline box
test, in place of the Stage 1 avoidance proof. The exact root count settles every pair that is not a touch with one
test per piece, where the avoidance proof halves a hull to `ledgerMaxDepth` around each crossing. On a scene of 48
regions with 15-point flanks, every flank crossing the circle once, this took `Profiles()` from 2.40 s to 1.19 s. A
circle has no end, so there is no `EndpointOnly` case to lose. Lines, arcs and splines keep the avoidance proof.

### Root isolation

For each exact piece, `|P(u) - c|^2 - r^2` is a degree-6 polynomial in `u`. `circleCrossings` converts it to
Bernstein form and runs `isolateRoots` on `[0, 1]`:

1. All coefficients of one strict sign: no root on the interval.
2. Otherwise the consecutive coefficient differences are the Bernstein coefficients of the derivative up to a
   positive factor. When they are all of one strict sign, the polynomial is strictly monotone, so the interval holds
   at most one root. The coefficient sequence is then monotone and not of one sign, so its two end values differ in
   sign and the root exists. An end value of exactly zero is a root on the interval boundary, which the routine
   refuses. The interval becomes a `ratBracket`.
3. Otherwise the interval is split at 1/2 by de Casteljau, to depth `ledgerMaxDepth`. A pair that needs more depth
   is `Unproven`.

A strictly monotone bracket means the crossing is transversal. A touch, a double root and two roots closer than one
part in 2^52 never isolate, so they stay `Unproven` with reason `resolution`. An empty root list proves the pair
`Disjoint`.

### The reported parameter

`roundedBracketRoot` returns the binary64 `u` nearest to the root and a proof that it is the nearest. A float
bisection followed by three Newton steps proposes `u`. For `u` and each of its two neighbouring floats, two exact
evaluations at the midpoints to the neighbouring floats must have opposite signs, and both midpoints must lie inside
the bracket (`confirmedRoot`). When no candidate is confirmed, `bisectedRoot` halves the bracket with exact
evaluations, up to 1100 times, until the bracket lies inside one rounding cell. The Newton steps matter for a root a
hair from a piece joint, where the bisection cannot reach the relative accuracy a cell needs. The global parameter is `t0 + u*(t1 - t0)`, the same formula `fitRadiusCrossing` uses.

A root within rounding of a piece joint rounds onto the joint's own parameter (`u` = 0 or 1). That is a valid
cut: the evaluated point is the joint, within `weldIdentEps` of the circle. Two roots that round to one global
parameter make the pair `Unproven`.

### Injection

`injectCrossings` evaluates each parameter, requires the point to lie within `weldIdentEps * scale` of the
circle, builds `evCross` events and asks `analyticCrossingsCertified` whether the sampled polylines have the same
crossing incidence. On success it records the events in `a.events`, marks the pair `specialHandled` (so the
sampled loop adds no crossing of its own and `nearMissGuard` skips it), and cuts both sources at the crossing
points. On failure nothing is changed and the pair is `Unproven`. The verdict is `Crossing` only after the
injection.

The audit has nothing to compare for a `Crossing` pair, because the sampled loop never records a contact for a
handled pair. A `Crossing` pair joins its component through its events, like any handled pair with events, and
does not withdraw exactness from it.

### Work limit

`crossCircle` takes a new `workMeter` of the same size as the pair's avoidance meter. The exact bisections of
`bisectedRoot` are not metered: each runs at most 1100 halvings and only for a root within rounding of a bracket
end.

### Tests

- `TestPairLedgerCrossingAgreesWithA256BitOracle`: 120 random scenes of a spline and a circle centred near the
  curve. Every `Crossing` verdict must have as many crossings as the sign changes of the circle's carrier function
  along the exact pieces in 256-bit arithmetic, each reported parameter inside its sign change's sample interval,
  and each evaluated point on the circle.
- `TestPairLedgerConstructedRelations`: a circle crossing the arch (`Crossing`), a circle touching the apex and a
  circle through a fit point (`Unproven`).
- `TestPairLedgerCrossingRoundsOntoJoint`: a circle through a fit point up to one rounding.
- `TestSplineCrossingCircleOncePerFlankIsExact` and `TestSplineCrossingCircleTwiceIsExact` (`geom.Regions`): n = 3,
  8 and 24 rotated copies of a region whose two 4- or 15-point flanks cross the circle (a fit point of the 15-point
  flank lies within rounding of the circle), and an arch cut twice by one circle. Every edge is `TExact`.
- `TestUnionProfilesCertifiesEmbeddedSplineSections` (`UnionProfiles`): the same scene for n = 3 to 32. The union
  has `4n` outer edges, `3n` of them `Partial` and all `TExact`. The profile areas sum to the union area, and the
  `n` rotated copies of each face have one area to 1e-9.
- A mutation check: each of four broken proof steps (the monotone test always true, the cell check always true,
  the confirmation of the estimate skipped, the rootless test never taken) makes at least one test fail.

## Stage 2b — the rest of `Crossing` (proposed)

Pair kinds: the same as Stage 1.

Stage 2a covers the circle case. A transversal crossing of the remaining pair kinds needs a proof that the pair
meets at exactly one point inside a sub-box, and a root bracket.
The method is Bernstein subdivision of the overlapping pieces until each surviving piece pair satisfies a uniqueness
test:

- Spline piece × line: the signed distance to the line, as a cubic in `u`, has a derivative whose Bernstein
  coefficients are of one sign, so the piece crosses the carrier at most once, and the crossing is a sign change of
  the endpoint values.
- Spline piece × arc: the circle case above, with each root accepted only when it lies inside the arc's sweep and
  clear of the arc's ends.
- Spline piece × spline piece: the tangent direction ranges of the two sub-pieces, as angular intervals from their
  derivative control points, share no direction. Then the Jacobian of the intersection system has one sign, the system
  has at most one root in the box, and a sign change of the endpoint residuals shows it has exactly one.

The root is refined by rational bisection to the binary64 parameter pair (`fitRoundedRoot` does this for one
variable). The `Crossing` verdict stores the bracket so a downstream consumer can recompute the point.

Stage 2 also adds **range verdicts**. `fitExactAbove` keeps the part of a flank above the root circle exact and the
part below inexact, because neighbouring flanks can meet below it. A general rule needs the same split: a spline's
`Unproven` pair marks only the parameter interval where the proof failed, and the fragments outside that interval stay
exact. This changes the exactness record from a per-source flag to a per-source set of intervals. It is the largest
design change in Stage 2 and the reason Stage 2 can retire `certifyFitCircleScene`.

Stage 2 would also let an arc test use its sweep when the pair touches the circle only outside it, and would let a
spline end rest on the interior of a line or circle.

## Stage 3 — other free-form kinds (not planned)

Control-point splines, closed splines and NURBS reduce to rational or polynomial Bézier pieces after knot insertion.
The Stage 1 and 2 proofs apply to polynomial pieces unchanged. Rational pieces change the hull bound (positive
weights keep the convex-hull property) and the polynomials in the monotone and Bernstein tests. No consumer has asked
for this stage.

## Work limit and cancellation

The work limit counts units, never time, so a verdict is identical on every machine. A unit is one exact separation
test on one pair of hulls or boxes, or one candidate cut. Each pair has a `workMeter` with
`4 * ledgerMaxDepth * pieces` units, where `pieces` is the piece count of the spline times that of the other source
(1 for a line, circle or arc). One contact region costs at most 52 levels of four children for each pair of starting
pieces. Reaching the limit makes the pair `Unproven` with reason `budget`. Nothing panics and no error is returned,
because `Profiles()` and `UnionProfiles` return no error for exactness. The consequence is a larger inexact component,
never a wrong exact bound.

The ledger also calls `arrangeBudget.step` for each unit. That budget is `nil` on the ordinary path and carries the
context and the charge callback only in the bounded line scene, which holds no fit spline. So `Profiles()`,
`Chains()` and `UnionProfiles`, which take no context, cannot be cancelled through the certificate; the per-pair limit
is its only bound. A context-taking entry point would be a public API change. It is not part of Stage 1 or 2, and the
decision is held until the Stage 1 benchmark above has been reviewed.

## Test generator and corpus

The soundness property is: **a pair certified `Disjoint`, `EndpointOnly` or `Simple` has exactly that relation.** A
false certification is a failure. A missing certification is a completeness rate and is asserted only for the
constructed classes below.

### Implemented in Stage 1

- `TestPairLedgerConstructedRelations`: a fit arch against lines, circles and arcs built to be far, a hair above the
  apex, through the apex, tangent at the apex, a hair below the apex, sharing one or both ends, standing on the interior,
  and arcs inside and outside the sweep. Each has a stated verdict. The test stops before the audit (`proveOnly`), so it
  tests the proof and not the sampled-map check that follows it.
- `TestPairLedgerSplinesSharingAnEnd`: two splines leaving one point at an angle, a smooth join, a duplicate, and one
  that runs back through the first.
- `TestPairLedgerSelfEntry`: a simple spline and a spiral.
- `TestPairLedgerNeverCertifiesAMeetingPair`: 400 random scenes of a fit spline against a line, circle, arc or second
  spline, a third of them sharing an end. When a pair is certified, a dense polyline oracle (400 segments per curve and
  a segment-crossing test, sharing no code with the certificate) must find no crossing outside the shared ends.
- `TestPairLedgerAgreesWithA256BitContactOracle`: 150 random scenes of a spline against a line or circle. Every
  certified pair is checked by evaluating the exact pieces in 256-bit `big.Float` arithmetic at 401 parameters per
  piece and looking for a sign change of the target's carrier function (signed distance, or `|P-c|²-r²`) outside the
  shared ends. This is a second oracle in a different form from the dense polyline one.
- `TestPairLedgerAtOneUlpFromTouching`: 20 splines, each with a horizontal line placed from three units in the last
  place below the exact maximum (found in 256-bit arithmetic) to three above. A line at or below the maximum meets the
  curve and must never be certified `Disjoint` or `EndpointOnly`. Lines above it may be certified; the test logs how many were.
- `TestConesApart` and `TestSplitAndSubHullAgreeWithEvaluation`: the cone and subdivision primitives.
- `arrange_pair_ledger_test.go`: end to end through `geom.Regions`. A fit arch closed by a chord is exact; a parked
  spline keeps the circles beside it exact; an unproven crossing withholds exactness from its component and leaves a
  distant component exact; a line standing on the spline's interior withholds exactness; the verdicts do not depend on
  authoring order.
- `TestUnionProfilesCertifiesSplineSections`: the downstream consumer's scene, 16 variants.

A mutation check supports the property tests: each of five deliberately broken proof steps (separating axes always
true, the carrier test always true, the wedge test always true, the box-against-circle test always outside, the cone
test always apart) makes at least one of these tests fail.

### Not yet implemented

- An arrangement oracle: the same scene at a very high `WithSegmentsPerTurn`, comparing region count, areas and
  `Degenerate` wherever the certificate publishes an exact bound.
- Random placement (rotation, translation, scale) and several shuffles of authoring order over the random scenes.
- Non-tooth scene families through `UnionProfiles` (a spiral of splines, nested loops, a comb of lines against
  splines).

Each belongs to the corpus that must pass before Stage 2.

### Failure output

A failing corpus case prints the scene index, the verdict and the reason. It never formats an arrangement, a `Profile`
or an `exactFitPiece` slice, because the failure path of a deep-equality helper can allocate a copy of the whole
scene.

## What stays inexact

Every case below is `Unproven` with the named reason. It costs the `TExact` flag on the affected component and nothing
else: topology, areas, `Degenerate` and the reported `TStart`/`TEnd` are unchanged.

| Reason | Case |
|---|---|
| `resolution` | The hulls still overlap at depth 52: the pair touches, crosses, overlaps, or comes closer than binary64 resolves. Includes a spline end resting on the interior of another source. A spline and a circle with a touch, a root on a piece joint, or a crossing the sampled map does not confirm also read `resolution`; a spline against a line, arc or spline that crosses reads it until Stage 2b. |
| `tangent` | A shared end whose near-end part could not be separated: the two leave the end in the same direction. |
| `end-mismatch` | Two splines share an end whose coordinates are not bit-identical. |
| `audit` | The sampled map recorded a contact the proof does not allow, for example a chord crossing where the exact curves are closer than the chord deviation. A higher sampling density usually lifts it. |
| `self-contact` | The spline's control vectors do not all advance along its chord. |
| `budget` | The pair's work limit was reached. |
| `degenerate` | A spline or target that is not finite or has no exact form. |
| not in the ledger | The scene holds an ellipse, elliptical arc, conic, control-point spline, closed spline or NURBS. The old kind gate applies and every fragment reads `TExact = false`. |

## Open decisions

- **Cancellation.** A context-taking `Profiles`/`UnionProfiles` would make the certificate cancellable. It is a public
  API change in a v0 package. Held; revisit with the Stage 1 numbers.
- **Range verdicts.** Stage 2 changes the exactness record from a flag to intervals. Skipping it means
  `certifyFitCircleScene` stays and a scene with one unproven pair loses exactness on its whole component.
- **Other free-form kinds.** Stage 3 has no requester.
