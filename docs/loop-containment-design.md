# Loop Containment — Design

Status: **proposed**. Nothing here is implemented. It builds on the pair ledger in `docs/pair-certificate-design.md`
(PR #196), referred to here as the pair ledger. Read that document first.

## The problem

A consumer that combines closed shapes (union, cut, intersect of extrusions) works with several closed source loops,
each an ordered chain of sketch entities. `Sketch.Profiles()` returns the faces of the arrangement of all those
entities together. It does not say which declared loop each face lies inside. The consumer needs, for every face and
every loop, one of three answers it can rely on: inside, outside, or "cannot say" with a reason.

The consumer then takes, for a union, every face inside at least one loop, joins them with `UnionProfiles`, and refuses
any `Partial` edge that is not `TExact`. A wrong "inside" or "outside" is the failure that matters. "Cannot say" costs
only a refused operation.

## Goals and non-goals

Goals:

- Declare any number of loops before `Profiles()`; an entity may belong to several loops, and two loops may cover the
  same carrier (a root arc lying on a disc circle).
- Give each `Profile` an exact verdict per declared loop: `Inside`, `Outside`, or `Unproven` with a reason.
- Withhold only the pairs that cannot be certified: `Unproven` for one (face, loop) pair never changes another pair.
- Change nothing for a sketch that declares no loops. `Profiles()` output, cost and `Revision` stay as they are.
- Assume no shape: the proof uses edge incidence and parity only.

Non-goals:

- Serializing loops. They are not part of the JSON schema (see "Lifetime").
- A containment query for an arbitrary point. This is face-versus-loop only.
- Loops of ellipses, conics, NURBS or control-point splines. They are refused with a reason.

## Proposed public API

```go
// A Loop is a declared closed chain of entities of one Sketch.
type Loop struct{ /* unexported */ }

func (s *Sketch) DeclareLoop(entities ...Entity) (*Loop, error)
func (s *Sketch) Loops() []*Loop
func (s *Sketch) RemoveLoop(l *Loop) bool
func (l *Loop) Entities() []Entity
func (l *Loop) Removed() bool

// Containment is the verdict for one (profile, loop) pair.
type Containment struct {
	State  ContainmentState
	Reason ContainmentReason // zero unless State is ContainmentUnproven
}

type ContainmentState uint8

const (
	ContainmentUndeclared ContainmentState = iota // zero value
	ContainmentInside
	ContainmentOutside
	ContainmentUnproven
)

func (p *Profile) Containment(l *Loop) Containment
```

**Declaration.** `DeclareLoop` takes the entities in chain order. It returns `ErrInvalidLoop` for a nil entity, an
empty list, an entity that is not in this sketch, or the same entity twice in one list. It does not check that the
chain closes: the sketch is solved later, so closure is judged each time `Profiles()` runs, from current
coordinates. A single circle, ellipse or closed spline is a closed chain on its own. A declared loop is a handle, so a
sketch can hold many.

**Reading a verdict.** `Containment` is defined on a profile built by `Sketch.Profiles()`. The zero value of
`ContainmentState` is `ContainmentUndeclared`, so a loop that was not declared when the profile was built, a loop from
another sketch, a removed loop, a nil loop, and a profile made by `UnionProfiles` all read `Undeclared` and can never be
mistaken for `Outside`. A stale profile (`Profile.IsStale`) reads `Unproven` with `ReasonStale`.

**Reasons.**

| Reason | Meaning |
|---|---|
| `ReasonGap` | The chain does not close, or two consecutive entities do not share an end coordinate exactly. |
| `ReasonSelfCrossing` | The loop meets itself: two of its edges cross or touch, or two entities of the loop cover one carrier span. |
| `ReasonKind` | An entity of the loop is not a line, circle, arc or fit spline. |
| `ReasonComponent` | The face lies in a part of the arrangement the pair ledger left unproven, or the scene's exactness gate is closed. |
| `ReasonCoincident` | A coincident-carrier overlap involving the loop was refused (`Degenerate`). |
| `ReasonInconsistent` | The parity check found a contradiction around a cycle (see below), which means the loop does not bound what it should. |
| `ReasonInvalid` | The face itself is invalid (`Profile.Valid` is false). |
| `ReasonStale` | The profile no longer describes its sketch. |

**Removal.** `RemoveEntity` removes every loop that names the entity, as it already removes the constraints that name
it; the loop handle reads `Removed() == true` afterwards. `RemoveLoop` removes one loop and reports whether it was
present.

**Lifetime.** Loops are transient state on the `Sketch`, like constraint labels held outside the geometry. They are not
in the JSON documents and not in `Sketch.Revision`, so declaring a loop changes no existing fingerprint and no existing
profile becomes stale by it. A profile built before the declaration reads `Undeclared` for that loop.

**No loops, no change.** `Profiles()` runs the containment pass only when at least one live loop is declared. A sketch
with none does the same work and returns the same values as today.

## The certificate

### Why parity is enough

Every edge of a loop is an arrangement edge, so no face crosses a loop boundary. Each face is wholly inside or wholly
outside each simple closed loop. The unbounded face is outside every loop. Crossing an arrangement edge that belongs to
loop `L` changes the face from inside `L` to outside `L` or back. So a breadth-first walk over the faces, from the
unbounded face and across shared edges, flips the bit for `L` at each edge `L` covers and leaves it unchanged at
every other edge. No orientation, winding number or area is used.

Two faces reached through different paths must agree. After the walk, every edge is checked: the two faces beside it
must differ in exactly the bits of the loops that cover it. A contradiction around any cycle means a covering set is
wrong or a loop does not close. The bits of the loops in the contradiction become `Unproven` with
`ReasonInconsistent`, for every face. This check is the certificate that a loop is closed and simple as far as the
arrangement can see, in addition to the checks below.

A region with holes needs nothing extra. A hole boundary is made of edges that also bound the face standing in the
hole, so nested components are reached by the same walk.

### What the geom layer must report

The walk needs four things the arranger holds internally and `Region` does not expose:

1. **An edge identity.** Each `BoundaryEdge` gets an `EdgeID` shared by the two sides of one arrangement edge (the two
   half-edges). A side with no region (the unbounded face, or a face the arranger discarded) has an `EdgeID` that appears
   once.
2. **The covering sources of each edge.** A new `BoundaryEdge.Covers []int` lists every source index that draws the edge.
   See the next section.
3. **A flag for discarded faces.** If `extract` discarded a face (a degenerate or zero-area cycle), the edges around it
   are marked, and every face adjacent through such an edge is `Unproven` for every loop with `ReasonInvalid`.
4. **The faithfulness of each face**: whether every source on its boundary is outside `exactRefused` and the scene's
   `exactAllowed` holds. This is exactly the pair-ledger certificate that the planar map is not fused.

### Coincident-edge attribution

The coincident-carrier resolution (`docs/coincident-carrier-resolution-design.md`) emits one edge for a stretch that
two sources both draw. It names one source for the emitted edge and suppresses the other over the overlap window.
The edge's `SourceIndex` therefore names only one of them.

The rule: **the covering set of an edge is its named source plus every source suppressed in favour of that source
over a window that contains the edge.** `fragmentSuppressed` already decides the second part for a fragment; the new
pass records its inputs (`a.overlaps`, `a.suppressed`) per fragment instead of only discarding the fragment. Loops that
use a source are looked up through the sketch's entity-to-source map. A loop covers an edge when any covering source
belongs to the loop.

Two loops that share a whole entity need no resolution: they both contain that entity, so both toggle at its edges. Two
loops that draw the same span with different entities (a root arc on a disc circle) toggle through the covering set. If
the resolution refused the overlap (it flagged `Degenerate`: two disjoint windows, a carrier match inside the
classification band only, complete duplicate carriers), the loops that contain either source are `Unproven` with
`ReasonCoincident`, for every face. Within one loop, an edge covered by two of its own entities is `ReasonSelfCrossing`.

### Per-loop validity, before the walk

For each live loop, from the current coordinates:

- **Kind.** Every entity is a line, circle, arc or fit spline, else `ReasonKind`.
- **Closed chain.** For consecutive entities `i` and `i+1`, and for the last and first, one end coordinate of `i` is
  bit-identical to one end of `i+1`, and the chain through all of them uses each end once, so the chain visits every
  entity in order and closes. A circle (or closed entity) alone is a closed chain. A chain that fails is `ReasonGap`. The
  check uses shared coordinates exactly, with no tolerance, as decad specified.
- **Simple.** In the arrangement, every vertex touched by the loop has exactly two loop edges meeting, counted once
  each. A vertex with four, or an edge covered twice, is `ReasonSelfCrossing`. Parity would give the even-odd interior for
  a self-crossing loop, which is not the interior the consumer means, so it is refused.

A loop that fails any of these is `Unproven` for every face. The walk still runs for the other loops.

### Per (face, loop) verdict

After the walk, face `F` and loop `L` get `Inside` or `Outside` from the bit of `L` at `F` when all of these hold:

- `L` passed the validity checks and the consistency check;
- `F` is faithful, and `F` is reached from the unbounded face through faces that are all faithful;
- `F` is valid (`Profile.Valid`).

Otherwise the pair is `Unproven` with the first failing reason, in the order stale, invalid, kind, gap, self-crossing,
coincident, inconsistent, component. A face that is not faithful is also not walked through: a wrong map there could
carry a wrong bit onward. A face reachable only through unfaithful faces is `Unproven` with `ReasonComponent`.

Because the walk skips unfaithful faces, an `Unproven` pair never changes a verdict of another pair unless it cuts the
only route to it, and then the other pair is `Unproven` with `ReasonComponent` rather than wrong.

## Refusal cases

Each is `Unproven`, never a guess:

- A loop with an ellipse, conic, NURBS, control-point spline or closed spline (`ReasonKind`).
- An open chain, a mismatched join, or two entities that meet only near each other (`ReasonGap`).
- A bowtie, or a loop that touches itself at a vertex (`ReasonSelfCrossing`).
- A face whose boundary includes a source the pair ledger left unproven: a tangent or overlapping spline, a spline end
  resting on a line or circle interior, two splines sharing a non-identical end, work-limit exhaustion
  (`ReasonComponent`).
- A scene whose exactness gate is closed because it holds a source the ledger does not cover (`ReasonComponent`).
- A refused coincident overlap (`ReasonCoincident`).
- A face the arranger flagged degenerate (`ReasonInvalid`).
- A profile that is stale (`ReasonStale`).

## Cost

The pass is linear in the number of arrangement edges times the number of declared loops (a bit vector per face, one
XOR per edge). For the stated sizes (2 to 40 loops of 4 to 60 curves, up to 150 regions) that is some thousands of
edges and under 64 loops, so one `uint64` mask per face. More than 64 loops use a slice of words. The pair ledger is the
costly part of such a scene and is already paid by `Profiles()`. No benchmark exists yet.

## Build steps

1. **Geom plumbing.** `EdgeID` and `Covers` on `BoundaryEdge`; the discarded-face flag; faithfulness per face. Tests in
   package `geom_test` on small hand-built scenes. This is the least certain part: the edge emission code
   (`split`, `extract`) and the suppression bookkeeping must each report what they know.
2. **The walk.** A function in `geom` that takes the covering sets and a loop-to-source map and returns the bits and
   consistency result, with no `sketch` types.
3. **The sketch API.** `DeclareLoop`, `Loops`, `RemoveLoop`, `Containment`, the removal cascade, the closed-chain check,
   the `Profiles()` hook.

Each step is independently reviewable. Step 1 changes `BoundaryEdge` (two new fields); no existing field changes.

## Test plan

Non-gear shapes only, through the public API, asserting verdicts on the regions of each scene:

| Scene | Expected |
|---|---|
| Two overlapping rectangles A and B | Three regions: A only (Inside A, Outside B), the overlap (Inside both), B only. |
| An L from two rectangles sharing a whole side | Two regions; the shared side is covered by both loops; each region Inside its own loop and Outside the other. |
| Two loops sharing one whole entity | The shared entity toggles both loops. |
| A disc circle plus N closed regions, each closed by a root arc lying on the circle, with fit splines | N + 1 regions; each tooth region Inside its own loop and Outside the disc loop; the disc region Inside the disc loop. With the pair ledger certifying the scene. |
| Nested loops: a loop standing in the hole of another | The annulus is Inside the outer and Outside the inner; the inner face is Inside both. |
| An open chain declared as a loop | Every face `Unproven`, `ReasonGap`. |
| A mismatched join (end coordinates one ulp apart) | `ReasonGap`. |
| A bowtie loop | `ReasonSelfCrossing`. |
| A loop with an ellipse | `ReasonKind`. |
| A loop with a spline tangent to a line of another loop | The faces touching the pair are `ReasonComponent`; faces elsewhere in an independent component keep their verdicts. |
| Two arcs overlapping in two windows | `ReasonCoincident` for the loops containing either. |
| A loop declared, then an entity moved | `Profile.Containment` on the old profile reads `ReasonStale`. |
| A profile made by `UnionProfiles`, an undeclared loop, a loop from another sketch | `Undeclared`. |

Properties, over random scenes of rectangles, rotated polygons, circles and fit-spline blobs placed freely:

- **Soundness against an independent oracle.** For each region, take an interior sample point, and decide containment in
  each loop by even-odd ray casting on a dense polyline of that loop. Wherever the verdict is `Inside` or `Outside`,
  the oracle must agree. A wrong verdict is a failure; `Unproven` is allowed and counted. The sample point must be at least
  a stated distance from every boundary.
- **Order invariance.** The same loop declared from a different starting entity, in the opposite direction, or with the
  declarations in a different order gives the same verdicts. Authoring order of the entities gives the same verdicts.
- **Union consistency.** The area of the faces Inside at least one loop equals the area of the union of the loops
  computed by the oracle, within the oracle's sampling error, for scenes where every verdict is certified.
- **Mutation checks.** Break each of: the unbounded-face start, the covering set, the consistency check, the faithfulness
  skip, and confirm at least one test fails.
- **No-loop equivalence.** `Profiles()` output of a sketch with no loops is byte-identical before and after.

Failure output prints the scene index, the region index, the loop index and the two verdicts, never a profile or an
arrangement.

## Open decisions

- **Not serialized.** Loops are call-time state. If a consumer wants them saved with the document, the schema needs a
  version change.
- **Step 1 scope.** If the edge emission code cannot report covering sources without reshaping `split`, the work grows
  past "similar in size to Stage 1 of the pair ledger". This is not known until step 1 is attempted.
- **Loops over 64.** A slice-of-words mask is described but not needed by the stated sizes.
