# Loop Containment — Design

Status: **implemented** (`loops.go`, `geom/facegraph.go`, `geom` option `WithFaceGraph`). It builds on the pair ledger in
`docs/pair-certificate-design.md`: a face is certified only where the ledger's planar-map certificate holds.

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
- A loop element that is a sub-range of an entity. See "Room for sub-range elements".

## Public API

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
	Reason ContainmentReason // ReasonNone unless State is ContainmentUnproven
}

func (p *Profile) Containment(l *Loop) Containment
func (p *Profile) Containments() []Containment // one per live loop, in Loops() order
```

States are `ContainmentUndeclared` (the zero value), `ContainmentInside`, `ContainmentOutside` and
`ContainmentUnproven`.

**Declaration.** `DeclareLoop` takes the entities in chain order. Every error it returns wraps `ErrInvalidLoop`, and
each cause has its own sentinel that `errors.Is` tells apart: `ErrLoopEmpty` (no entity), `ErrLoopNilEntity` (a nil
entity), `ErrLoopForeignEntity` (an entity not in this sketch, or removed from it) and `ErrLoopDuplicateEntity` (an
entity named twice in one loop). It does not check that the chain closes: the sketch is solved later, so closure is
judged each time `Profiles()` runs, from current coordinates. A single circle is a closed chain on its own.

**Reading a verdict.** `Containment` is defined on a profile built by `Sketch.Profiles()`. The zero value of
`ContainmentState` is `ContainmentUndeclared`, so a loop that was not declared when the profile was built, a loop from
another sketch, a removed loop, a nil loop, and a profile made by `UnionProfiles` all read `Undeclared` and can never be
mistaken for `Outside`. A stale profile (`Profile.IsStale`) reads `Unproven` with `ReasonStale`.
`Containments` returns the same answers for all live loops in the order `Loops()` returns them at that moment.

**A region with holes.** `Containment` describes the whole face of the profile: its outer boundary minus its holes. A
face standing in the hole of another face is a different profile, and reads `Inside` only the loops that contain it.

**Reasons.**

| Reason | Meaning |
|---|---|
| `ReasonGap` | The chain does not close, two consecutive entities do not share an end coordinate bit-for-bit, or an edge of the loop was pruned as a dangling spur. |
| `ReasonSelfCrossing` | The loop meets itself: a vertex has other than two loop edges, or two entities of the loop draw one span. |
| `ReasonKind` | An entity of the loop is not a line, circle, arc or fit spline, or is not drawn (construction). |
| `ReasonComponent` | The face lies in a part of the arrangement the pair ledger left unproven, the scene's exactness gate is closed, or the face is reachable only through such parts. |
| `ReasonCoincident` | The walk found the loop inconsistent or self-crossing while the arrangement was `Degenerate`, which is how a refused coincident-carrier overlap shows. |
| `ReasonInconsistent` | The parity check found a contradiction around a cycle with no `Degenerate` condition to explain it. |
| `ReasonInvalid` | The profile itself is not valid (`Profile.Valid` is false). |
| `ReasonStale` | The profile no longer describes its sketch. |

When several apply, the profile's own state wins first (`ReasonStale`, then `ReasonInvalid`), then the loop's
(`ReasonKind` or `ReasonGap` from the closure check, then the walk's defects), then `ReasonComponent`.

**Removal.** `RemoveEntity` removes every loop that names the entity; the loop handle reads `Removed() == true`
afterwards. `RemoveLoop` removes one loop and reports whether it was present.

**Lifetime.** Loops are transient state on the `Sketch`. They are not in the JSON documents and not in
`Sketch.Revision`, so declaring a loop changes no existing fingerprint and no existing profile becomes stale by it. A
profile built before the declaration reads `Undeclared` for that loop.

**No loops, no change.** `Profiles()` asks the arrangement for its face graph only when at least one live loop is
declared. A sketch with none does the same work and returns the same values as before.

### Room for sub-range elements

A loop may later need an element that is a parameter range of an entity (a trimmed segment of a profile that was itself
a union or a trim). `DeclareLoop(entities ...Entity)` takes whole entities and stays as it is. A later
`DeclareLoopSpans` can take a slice of `{Entity, TStart, TEnd}` elements and share the machinery below: the walk already
works on edges, so a span would cover only the arrangement edges whose parameter range it contains. Nothing in the
present types prevents it.

## The certificate

### Why parity is enough

Every edge of a loop is an arrangement edge, so no face crosses a loop boundary. Each face is wholly inside or wholly
outside each simple closed loop. The unbounded face is outside every loop. Crossing an arrangement edge that belongs to
loop `L` changes the face from inside `L` to outside `L` or back. So a breadth-first walk over the faces, from the
unbounded face and across shared edges, flips the bit for `L` at each edge `L` covers and leaves it unchanged at
every other edge. No orientation, winding number or area is used.

After the walk, every edge between two reached faces (or a reached face and the unbounded one) is checked: the two
sides must differ in exactly the bit of `L` if `L` covers the edge and agree otherwise. A mismatch makes the loop
`Unproven` for every face. This check is defence in depth: the closure, pruning and degree checks below catch the loops
a test could construct, and no test builds a loop that passes them and fails the parity check.

A face with holes needs nothing extra. A hole boundary is made of edges that also bound the face standing in the
hole, so nested components are reached by the same walk.

### What the geom layer reports

`geom.Regions(curves, closed, geom.WithFaceGraph())` also returns `Arrangement.FaceGraph`. Without the option the field
is nil and the arrangement does the same work as before. `BoundaryEdge` is unchanged.

- `FaceGraph.Faces[i]` describes `Arrangement.Regions[i]`. `Faithful` is true when the exactness gate is open, the region
  is neither degenerate nor self-intersecting, and no source on its boundary is in `exactRefused`. This is the pair
  ledger's certificate that the planar map around the region is not fused.
- `FaceGraph.Edges` has one `FaceEdge` per arrangement edge that survived pruning. `A` and `B` name the faces on its two
  sides: a region index, `FaceOuter` (the unbounded face) or `FaceUnknown` (a cycle below the area floor, a hole no face
  was assigned, or a cycle of zero area). `U` and `V` are the vertex ids. `Covers` lists every source that draws the edge.
- `FaceGraph.Pruned` lists the sources that lost an edge to pruning, because an edge dangling off the map bounds no face.

### Coincident-edge attribution

The coincident-carrier resolution (`docs/coincident-carrier-resolution-design.md`) emits one edge for a stretch that
two sources both draw. It names one source for the emitted edge and suppresses the other over the overlap window.

The rule: **the covering set of an edge is its emitting source, plus every source suppressed in favour of that source
over a window that contains the edge, plus every duplicate line `dedupCoincidentStraightEdges` merged into it.** The
window test evaluates the edge's parameter midpoint on the carrier (`losingCovers`); a line window is tested in the
suppressed line's own parameter by projecting that point onto it. A refused overlap suppresses nothing, so it adds no
covering source, and it flags the arrangement `Degenerate`.

Two loops that share a whole entity need no resolution: both contain that entity, so both toggle at its edges. Two
loops that draw the same span with different entities (a root arc on a disc circle) toggle through the covering set.

### Per-loop validity, before the walk

For each live loop, from the current coordinates:

- **Kind.** Every entity is a drawn line, circle, arc or fit spline, else `ReasonKind`.
- **Closed chain.** Entities consecutive in the list share an end coordinate bit-for-bit, each entity is entered at one
  end and left at the other, and the last meets the first. The check tries both ends of the first entity. A single circle
  is closed; a circle among other entities is not. A chain that fails is `ReasonGap`.
- **Pruned edges.** A loop with any source in `FaceGraph.Pruned` is `ReasonGap`: the arrangement dropped a piece of it.
- **Simple.** Every vertex touched by the loop has exactly two loop edges, and no edge is covered by two of the loop's
  sources. Otherwise `ReasonSelfCrossing`. Parity would give the even-odd interior for a self-crossing loop, which is not
  the interior the consumer means, so it is refused.

A loop that fails any of these is `Unproven` for every face. The walk still runs for the other loops.

### Per (face, loop) verdict

After the walk, face `F` and loop `L` get `Inside` or `Outside` from the bit of `L` at `F` when `L` passed the checks
and consistency test, `F` is reached from the unbounded face through faces that are all faithful, and `F` is valid.
A face that is not faithful is not walked through: a wrong map there could carry a wrong bit onward. A face reachable
only through unfaithful faces is `Unproven` with `ReasonComponent`. An `Unproven` pair therefore never changes another
pair's verdict; it can only make another pair `Unproven` by cutting the only route to it.

## Refusal cases

Each is `Unproven`, never a guess:

- A loop with an ellipse, conic, NURBS, control-point spline, closed spline or construction entity (`ReasonKind`).
- An open chain, a mismatched join (even one unit in the last place), or a dangling piece (`ReasonGap`).
- A loop that touches itself at a vertex, or draws one span twice (`ReasonSelfCrossing`).
- A face whose boundary includes a source the pair ledger left unproven: a tangent or overlapping spline, a spline end
  resting on a line or circle interior, two splines sharing a non-identical end, work-limit exhaustion; or any scene whose
  exactness gate is closed because it holds a source the ledger does not cover (`ReasonComponent`).
- A refused coincident overlap (`ReasonCoincident`).
- A face that is invalid, for instance one the arranger flagged degenerate (`ReasonInvalid`).
- A profile that is stale (`ReasonStale`).

## Cost

The face graph is built in one pass over the arrangement edges, and the walk is linear in edges per loop. Measured on
the disc and 24 regions with 15-point fit splines and root arcs (145 sources, 25 loops), `Sketch.Profiles()` on an AMD
Ryzen 9 7900X3D, three runs of ten iterations:

| Loops declared | ns per call |
|---|---|
| none | 262 400 000 to 263 800 000 |
| 25 | 271 700 000 to 274 500 000 |

Declaring the loops adds about 10 ms, 4 percent. The pair ledger is the larger cost in such a scene and `Profiles()`
already pays it.

## Tests

`loops_test.go` and `loops_oracle_test.go` (package `sketch_test`), `geom/facegraph_test.go` (package `geom_test`):

| Scene | Checked |
|---|---|
| A disc plus N closed regions, each of two lines starting on the circle, two fit splines, a tip arc and a root arc lying on the circle; N = 8, 12, 17, 24 with 4 and 15 fit points | N + 1 profiles. The disc profile is Inside the disc loop and Outside every region loop; each region profile is Inside its own loop and Outside the disc and every other region loop. |
| Two overlapping rectangles | A only, the overlap, B only. |
| Nested rectangles | The face around the hole is Inside the outer loop and Outside the inner; the face in the hole is Inside both. |
| Two loops sharing a whole line | Each face Inside its own loop and Outside the other. |
| An open chain declared as a loop | `ReasonGap`; an intact loop beside it is still judged. |
| A join one unit in the last place off | `ReasonGap`. |
| Two squares meeting at a corner, declared as one eight-line loop | `ReasonSelfCrossing`. |
| An ellipse loop, and an ellipse elsewhere in the scene | `ReasonKind`; the scene's exactness gate closes, so another loop reads `Unproven`. |
| Declaration errors | Each of the four sentinels, and `errors.Is` against `ErrInvalidLoop`. |
| Profile built before the declaration, nil, removed and foreign loops; a stale profile; entity removal | `Undeclared`, `Undeclared`, `ReasonStale`, loop removed. |
| The same loop declared from another start, reversed, or both | Identical verdicts. |
| A sketch with loops declared and the same sketch without | Identical regions, areas, ranges and exactness flags. |
| `geom.Regions` with and without `WithFaceGraph` | Identical regions and chains. |
| 120 random scenes of circles, rotated rectangles and triangles | 2 408 verdicts judged, none disagrees with each shape's own analytic membership test at a point well inside the face. |

Three deliberately broken steps each fail at least one test: the starting side of the unbounded face, the covering set,
and the faithfulness skip. (The parity consistency check has no failing test, as stated above.)

### Not yet tested

- Two arcs overlapping in two windows (a refused coincident overlap).
- A spline tangent to a line of another loop.
- Fit-spline loops in the random oracle.

## Open decisions

- **Not serialized.** Loops are call-time state. If a consumer wants them saved with the document, the schema needs a
  version change.
- **Sub-range elements.** See "Room for sub-range elements".
- **Loops over 64.** The walk uses a pass per loop, so there is no mask-width limit.
