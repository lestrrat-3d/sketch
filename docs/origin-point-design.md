# The sketch origin point — design

Status: **implemented** (`Sketch.Origin`, `sketch.go`).

## The problem

A Fusion sketch owns a fixed point at its plane origin (`sketch.originPoint`). It
exists before anything is drawn, the solver never moves it, and geometry is
grounded by constraining to it. This engine had no equivalent, and told a caller
to invent one instead: `p.MoveTo(0, 0); s.Fix(p)`.

Two things followed from the anchor being invented rather than provided.

**The grounding rule could not be enforced or reported.** `Fix` grounds any point
it is handed, and none of them is distinguished, so a sketch that pins three
points mid-drawing reads exactly like a correctly anchored one — DOF 0, no
conflicts, trustworthy.

**A sketch Fusion can prove had no proof here.** A point that a drawing does not
otherwise constrain — the leftover a shared builder creates — is made determinate
in Fusion by one coincidence to the origin. Without an origin the alternatives
were to `Fix` it (an anti-pattern for a point that is not the anchor) or to
declare it reference geometry (a claim that its position came from outside the
sketch, which is false). A sketch and its proof then disagree about what is
grounded, and a passing proof can correspond to a sketch Fusion calls
under-constrained.

## The shape

`Sketch.Origin() *Point` returns a point the constructor creates at (0, 0) with
both solver variables fixed for the sketch's life. It is usable as a constraint
target exactly like any other point — that was the one stated requirement, since
the goal is to REPLACE `Fix` at the anchor rather than to add a second way to
ground.

```go
p := s.CreatePoint(0, 0)
s.AddConstraint(sketch.NewCoincident(p, s.Origin()))
```

## Decisions

### The origin is NOT in `s.points`

This is the load-bearing one. A point's `id` is its position in `s.points`, and
serialized entities and constraints reference points by that id. Putting the
origin in the slice would shift every authored point's id by one, so every
existing document would need a migration, every point count would change, and
every consumer enumerating `Points()` would see something it did not create.

Keeping it out leaves the authored id space exactly as it was. `Points()` returns
what the caller created; `Origin()` is the only door to the origin.

The cost is that a handful of internal passes iterate `s.points` and therefore do
not see it. Each was checked:

- `freeVars` filters on `fixed`, so the origin's variables never enter the
  Jacobian, the rank analysis or the conditioning measure. DOF is unaffected.
- `FreePoints`, the probe, the renderers and the name lookups iterate `s.points`,
  which is the wanted behaviour: the origin is not free, not perturbed, not drawn
  and not authored.
- `owns` needed the exception. It decides ownership positionally, so without it a
  constraint to the origin read as a FOREIGN handle and aborted verification.
- `positionShift` needed the exception too — see below.

### The finite-difference translation must move the origin with everything else

`positionShift` translates every position by the authored centroid to keep the
finite differences well-conditioned far from the origin. That is only sound
because a rigid translation leaves every residual invariant.

A position left behind is not a rigid translation. With the origin outside
`s.points` it was left at (0, 0) while the geometry moved, which silently changed
the sketch the derivatives measured: for a sketch with one authored point
constrained to the origin, the shift collapsed the two onto each other, and the
engine reported a phantom redundant constraint and a lost degree of freedom.

The fix keeps the centroid over the authored points — so an origin-free sketch
shifts by exactly what it always did — and applies that same shift to the origin.
`TestOriginAnchorMatchesAFixedAnchor` pins the symptom: anchoring on the origin
and anchoring by fixing a point at (0, 0) must give the same verdict.

### The origin is implicit in the document, but a reference to it is not

The origin is recreated by the constructor on load, like an internal constraint,
so it is never serialized as a point and no existing document changes. A
*reference* to it does serialize, as the reserved point id `-1`.

Both shapes of point reference count, and missing either one is the same bug. A
constraint writes its operands' ids, and an ENTITY writes its defining points'
ids — so a line drawn from the origin puts the reserved id in the document with no
constraint involved. Detection therefore walks entities through
`entityPoints` and constraints through `constraintRefs`, the same two
accessors `marshalBody` serializes from, so a type cannot be written by one and
missed by the other.

That id is a schema change: an older reader cannot resolve it. So a document
containing one declares `jsonOriginVersion`, which older builds reject.

The version is stamped **on demand**, not unconditionally. A document that never
references the origin is byte-identical to what earlier builds wrote and stays
readable by them. The number a document declares is the oldest reader that can
read it faithfully, not the newest writer that produced it. A world document
takes the maximum over its sketches: a document is only as readable as its
least-readable part.

### The origin refuses the mutators

`MoveTo`, `Unfix`, `UnfixEntity` (for an entity drawn from it) and
`SetConstruction` are no-ops on it, and `RemovePoint` refuses it (it is not in the
slice, so the lookup already fails). Its coordinates are the plane origin by
definition, and its grounding is what makes it an anchor. `UnfixEntity` is the
easy one to forget: it releases every defining point of an entity, so an entity
with the origin as an endpoint is a second door onto the same variables, and it
refuses the origin for the same reason `Unfix` does.

`Fix` is deliberately still allowed on the sketch's own origin and is simply
redundant — it is already fixed — rather than being made an error for one point.
That is `owns` answering, not an exception carved into `Fix`: the whole grounding
API (`Fix`/`Unfix`/`FixEntity`/`UnfixEntity`/`EntityFixed`) screens its handle
through `owns` / `foreignInput` first, and `owns` carries the origin exception, so
this sketch's origin and geometry drawn from it stay groundable while ANOTHER
sketch's origin is refused like any other borrowed point.

### Each sketch owns its own origin

`isOrigin` compares identity against the sketch's own origin rather than testing
the id, so another sketch's origin is not mistaken for this one's. Constraining
to another sketch's origin is a cross-sketch reference and reads as a foreign
handle, exactly like any other borrowed point.

Serialization has to enforce the same thing, and identity is exactly what it
loses. `pointRef` resolves the reserved id against the RECEIVING sketch, so a
borrowed origin written as a bare `-1` comes back as the reader's OWN origin:
the reloaded document would hold an ordinary local relation where the original had
a foreign handle, and `Verify` would have nothing left to report. So `marshalBody`
refuses a foreign reference instead of writing it (`checkNoForeignRefs`, wrapping
`ErrForeignHandle`), for constraint operands and entity points alike.

The origin is the sharpest case rather than a special one. It is the reference
that rebinds with no id collision at all, since the reserved id resolves against
the reader by construction; an ordinary foreign point rebinds whenever its
positional id happens to name a local point, which small ids usually do. The guard
therefore screens ownership in general — `Sketch.owns` for a point, `ownsEntity` for
a constraint's entity operand — and `Sketch.MarshalJSON` and `World.MarshalJSON` both
return an error for a sketch holding one.

`owns` is the predicate because it is the same one `scanReferenceIntegrity` uses to
set `ForeignHandles`, so marshal and `Verify` cannot diverge about which handles a
sketch owns. It also carries the origin exception, so a sketch's OWN origin stays
owned and still serializes as the reserved id; only a borrowed origin is refused.

Screening the point's own sketch pointer instead would be weaker, because it passes a
DEAD point — one `RemovePoint` spliced out. That point's sketch pointer still names
this sketch while its `id` is stale, and the stale id fails in two shapes. The splice
renumbers, so the id usually names a DIFFERENT live point by the time the document is
written: the reload binds the reference to that point, silently, with nothing left to
flag. When the removed point was the LAST one the id is out of range instead, so the
document marshals cleanly and then fails to load.

That refusal is bounded to exactly the sketches `Verify` already reports as
`ErrForeignHandle`.

## Known consequence

The origin's two variables occupy the first two slots of the parameter vector, so
every later variable index shifts by two. `ProbeConfigurations` keys its
pseudo-random restarts on the variable index, so a sketch's probe explores a
different set of perturbations than it did before this change and may report a
different NUMBER of configurations. The probe is a falsifier whose count is a
lower bound, so `Ambiguous()` is unaffected in meaning; only the count moves.

## Not done here

Reporting grounding that does not follow the "ground, don't pin" rule — telling
"grounded at the origin" apart from "pinned somewhere convenient" — is the other
half of the original report. It is now expressible, since the origin exists to
report against, but it needs its own decision about what counts as a violation
and whether it gates `Trustworthy()` or is advisory like `RankMargin`.
