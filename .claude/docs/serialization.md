# Export and serialization — `svg.go`, `png.go`, `dxf.go`, `json.go`, `json_world.go`

Detail moved out of CLAUDE.md. Read before changing an exporter, the JSON schema, a document version, or how references are written and resolved.

## Question router

| Your question | Section |
|---|---|
| Why does an exporter refuse to emit? | `SVG`/`PNG`/`DXF` refuse non-finite output |
| Where is the non-finite check enforced? | `SVG` funnels every value through one formatter |
| Why does PNG bound the pixel product? | `fitsPixelBudget` bounds the product |
| How are points and entities referenced? | Serialization invariants |
| What version does a document declare? | Document versions and the kind discriminator |
| Why is a foreign reference refused? | A foreign reference is refused rather than written |
| Why is the origin not serialized as a point? | The origin is implicit but a reference to it is not |

Navigation only — the sections below are the authority.

## Exporters — `svg.go` / `png.go` / `dxf.go`

### Overview

Exporters / serialization. `png.go` is a stdlib-only rasterizer (`image/png`) so
agents/tools that read raster images can sanity-check sketches; visually
equivalent to the SVG output (PNG annotation is a follow-up — SVG is the
annotated target). `dxf.go` emits length fields in the sketch's **display length
unit** (via the `units` library — angles/ratios/knots stay raw) with a matching
`$INSUNITS`/`$MEASUREMENT` + `$EXTMIN`/`$EXTMAX` header, so a CAD importer reads
the drawing at the right scale (metric output is unchanged). Coordinates are
plane-**local** by default; `DXF(WithWorldSpace(true))` places geometry in 3D
world coordinates via the plane frame — LINE/SPLINE/ELLIPSE in WCS,
CIRCLE/ARC/LWPOLYLINE in the entity OCS (arbitrary-axis algorithm from the plane
normal) + extrusion, arc angles recomputed in the OCS.

### `SVG`/`PNG`/`DXF` refuse non-finite output

**`SVG`/`PNG`/`DXF` all refuse rather than emit a value that is non-finite (NaN
or infinite) or outside the exporter's representable range, returning the shared
`ErrNonFiniteGeometry` sentinel.** The check is layered, and the two layers
answer different questions.

### `bbox.finite()` is a precondition, not the check

**`bbox.finite()`** (checked once via `renderBounds`/`bounds()`) is a cheap
early refusal with a good error locus, but it is a PRECONDITION over the
geometry that produced the box, not over what the exporter actually writes — a
curve sampled at a fixed density can still hide a poisoned span between samples,
and a perfectly finite box is no guarantee of finite OUTPUT: `SVG`'s own
`WithMargin`/`WithPixelWidth` and `PNG`'s `WithScale` each multiply a finite box
further downstream, and `DXF`'s display-unit conversion (`lengthMag`, via the
`units` library) can overflow a finite coordinate with no span arithmetic
involved at all. So the load-bearing check sits one layer lower, as a
POSTCONDITION over what each exporter actually writes.

### `SVG` funnels every value through one formatter

**`SVG` funnels every numeric value — its own body, and everything
`annotate.go`/`frame.go` write into the same output — through two funnels
sharing one `svgWriter.nonFinite` flag: the formatter `svgWriter.f`, and the
flag-only `svgWriter.note`.** `f` flags the shared `nonFinite` bit on NaN/Inf
and formats the value into the output; a value built in a scratch buffer later
embedded into the main one (a `<path>` "d" attribute, a loop's boundary) uses
`svgWriter.scratch()`, which shares that same flag, so building the fragment
still trips it. `SVG` checks the flag once at the end and refuses rather than
return the built document. Annotations and the watermark do write
caller-influenced TEXT into the same buffer (an entity name, a dimension's
unit-formatted value label): an entity name is never built from a float, so it
cannot false-trip `f`. A dimension's label IS built from a float — its target's
magnitude — but through `units.Value.String`, not `f`, since the rendered text
(e.g. "20 mm") is not a bare formatted float; `dimText` (a method on `annCtx`
precisely so it can reach the writer) calls `note` on the target's magnitude
before formatting it, so a NaN/Inf target still raises the flag rather than
rendering as the literal text "NaN mm"/"+Inf mm" with no refusal at all.

### `DXF` funnels every field through `pairf`

**`DXF` funnels every numeric field through its own single formatter, `pairf`**
(`pairL`/`putWCS`/`putOCS`/`putLW`/… all resolve to it, including a NURBS knot
vector written verbatim into the native `SPLINE` record), which sets a local
`nonFinite` flag the same way; `DXF` checks it before returning.

### `PNG` guards the pixel dimensions

**`PNG` has no textual output to check this way**, so its guard sits earlier:
the pixel width/height are computed in float and checked by `finitePixelDim` —
finite AND within `int32` range — BEFORE the `int()` conversion, since
converting an out-of-range or non-finite float is where the undefined value is
born and `image.NewNRGBA` is where it panics.

### `fitsPixelBudget` bounds the product

**`finitePixelDim` alone bounds each dimension, never their PRODUCT**: on
ordinary finite geometry a large `WithScale` can push both `pw` and `ph`
individually within `int32` while `4·pw·ph` (the NRGBA buffer's byte count)
still doesn't fit — panicking inside `image.NewNRGBA`'s own overflow check, or,
once the byte count clears `int64` but not available memory, dying with the
runtime's unrecoverable out-of-memory fatal error, which no `recover` can catch.
`fitsPixelBudget` closes that gap as a second check at the same site, before the
same `int()` conversion: the product computed in `float64` (never `int`, where
it would itself overflow before it could be tested) against a stated byte budget
`pngMaxPixelBytes` (`math.MaxInt32` bytes, 2 GiB, ~536M pixels) — a chosen
budget rather than merely Go's own int-overflow rule, since bounding only at
`4·pw·ph <= math.MaxInt64` still admits the unrecoverable-OOM regime. This
structure needs no enumeration of arithmetic sites (a new option that scales a
dimension further needs no new check, since whatever it computes still has to
pass through the one formatter or the one pixel-dimension check before reaching
output) and cannot be evaded by a new one. What it does NOT catch is a NaN that
corrupts a curve WITHOUT ever producing a non-finite written value or a
non-finite bounding box — silently wrong but perfectly FINITE geometry.

### `CreateNURBS` rejects a non-finite knot

`CreateNURBS` rejects a non-finite knot outright: a dedicated finiteness loop
runs FIRST, ahead of the non-decreasing/clamped/empty-domain checks. **It closes
TWO gaps those three leave open — an INTERIOR NaN, and an INFINITE CLAMPED RUN
at either end.** The non-decreasing `knots[i] < knots[i-1]` compare and the
empty-domain `knots[degree] >= knots[n]` compare are both false against a NaN,
so an interior NaN passed both and reached committed geometry silently; the
clamped compare is `!=`, which is TRUE against a NaN, so it already rejects a
NaN that lands INSIDE a clamped run — but it never examines an interior knot. An
infinite clamped run passes each of the three for its own reason: an all-equal
run is non-decreasing, `+Inf != +Inf` is false so the run reads as properly
clamped, and `knots[degree] >= knots[n]` is false when one side is an infinity
of the right sign — so `{0,0,0,1,+Inf,+Inf,+Inf}` and `{-Inf,-Inf,-Inf,1,2,2,2}`
were both ACCEPTED. An INTERIOR infinity is the one non-finite shape already
caught, by the non-decreasing compare (its finite neighbours put it out of
order); the finiteness loop rejects it too, so knot finiteness is one check
rather than split across two. `nurbs_entity_test.go`'s `TestNURBSValidation`
pins all five shapes, and the three the loop alone catches — the interior NaN
and the two infinite clamped runs — are exactly the rows that fail when the loop
is removed. It does NOT validate a control-point COORDINATE the same way, and
deliberately so: `CreatePoint` has no error return and the solver moves those
coordinates after construction, so a construction-time check there could never
hold as a precondition — the exporters' own postcondition checks are the right
guard for that case. `JSON` already refuses non-finite geometry via
`encoding/json`'s own NaN rejection, so it needed no change.
`ErrNonFiniteGeometry` is exported so a future `Verify` condition can reuse the
same sentinel for this fact. `json_world.go` is the v2 `World`/`Plane`
serialization + the `kind`-discriminator preflight.

## Serialization invariants

- Points and entities are referenced by their **id, which always equals their
  current slice position** (`s.points`/`s.ents` — two independent id spaces).
  Removal splices and renumbers the later ids (`removal.go`), so marshalled
  documents stay dense and coherent; `UnmarshalJSON` recreates in order so the
  indices line up. Never let an `id` field and slice position diverge.
### The origin is implicit but a reference to it is not

- **The origin point is implicit in the document, but a REFERENCE to it is not.**
  It is recreated by the constructor on load (like an internal constraint) and never
  serialized as a point, so no existing document changes; a reference to it
  serializes the reserved point id `-1`, which `pointRef` resolves and which older
  readers cannot. So a document carrying one declares `jsonOriginVersion` (4) — stamped
  **ON DEMAND**, never unconditionally, so a document that never touches the origin stays
  byte-identical to what earlier builds wrote and readable by them. The version a document
  declares is the OLDEST reader that can read it faithfully, not the newest writer that
  produced it; a world document takes the max over its sketches. `jsonMaxVersion` is what
  this build reads for either kind. **BOTH shapes of point reference count** — a
  constraint's operands AND an **entity's defining points**, since an entity writes its
  points' ids exactly as a constraint does, so a line drawn from the origin puts the
  reserved id in the document with no constraint involved. `referencesOrigin` therefore
  walks entities through `entityPoints` and constraints through `constraintRefs` —
  the same two accessors `marshalBody` serializes from, so a type cannot be written by one
  and missed by the other.
### A foreign reference is refused rather than written

- **A FOREIGN reference is refused rather than written** (`checkNoForeignRefs`, wrapping
  `ErrForeignHandle`, so `Sketch.MarshalJSON` and `World.MarshalJSON` both return an
  error): every reference is a bare id the loader resolves against the RECEIVING sketch,
  so writing one for another sketch's point or entity rebinds it to whatever local handle
  carries that number. The reload is CLEAN — a foreign handle `Verify` reports as
  `ErrForeignHandle` comes back as an ordinary local relation with nothing left to flag —
  so the round trip would turn a rejected sketch into a blessed one. **The origin is the
  sharpest case and needs no id collision at all**: it carries the reserved id `-1`, which
  `pointRef` resolves to the READER's own origin, so a borrowed origin ALWAYS rebinds;
  an ordinary foreign point rebinds whenever its positional id names a local point, which
  small ids usually do. **Both halves of a reference are screened** — points (a
  constraint's operands and an entity's defining points) and a constraint's ENTITY
  operands, through the same `entityPoints`/`constraintRefs` accessors `marshalBody`
  serializes from. **Ownership is `owns` for a point — the SAME predicate
  `scanReferenceIntegrity` uses to set `ForeignHandles`, so marshal and `Verify` cannot
  diverge.** A weaker screen on the point's own sketch pointer (`p.s != s`) passes a DEAD
  point of this sketch, one `RemovePoint` spliced out: its `s` still names this sketch
  while its `id` is stale, so the document writes the id a DIFFERENT live point has since
  inherited and the reload binds the reference to that point — silently, with nothing left
  to flag — or, when the removed point was the last one, writes an id out of range and
  produces a document that marshals cleanly and then fails to load. `owns` also carries the
  origin exception (the origin is deliberately absent from `s.points`, so a positional check
  alone would call it foreign). A nil point is screened out first rather than reported as
  foreign, since `Verify` splits a nil operand out as a corrupt reference (the entity half
  reuses `ownsEntity` and skips a nil operand for the same reason). The refusal is bounded
  to what `Verify` already rejects, so a sketch its report calls trustworthy always marshals.
### Document versions and the kind discriminator

- A **sketch** document carries `"version": 2` (`jsonVersion`); a **world**
  document carries `"version": 3` (`jsonWorldVersion`, ahead because a world adds
  top-level shared `parameters` + plane `dist_expr` an older reader would silently
  drop) plus an explicit `"kind"` (`"sketch"` | `"world"`). Both loaders
  **preflight** the raw top-level object (today's typed unmarshal ignores unknown
  fields, so a world doc fed to `Sketch.UnmarshalJSON` would otherwise rebuild
  empty) and **check kind before version** (a world doc handed to the sketch loader
  is a wrong-kind error, not a wrong-version one): a v2 doc requires
  `kind`; a wrong/unknown `kind` is `ErrWrongDocumentKind`; a legacy (kind-less,
  version absent/0/1) doc must carry no v2-only key (`plane`/`planes`/`sketches`)
  and loads as a world-XY sketch. A v3 world carries the shared param table at the
  top level (world sketches no longer serialize their own); a legacy v2 world
  migrates per-sketch tables (identical → promote, conflicting → reject). Both shapes decode their payload through one
  shared `jsonSketchBody` (`buildFromBody`) so reference handling lives in one
  place. A plane serializes its **definition** (recomputed on load, never trusted
  from disk); a world's derived `offset{base_id}` must reference an **earlier**
  plane. Newer versions are rejected. Bump `jsonVersion` + add read-side
  migration for schema changes.
### Internal constraints are not serialized

- **Internal constraints** (those implementing `internalConstraint`, e.g. the
  arc radius-consistency constraint auto-added by `CreateArc`) are *not* serialized
  — they're recreated by the constructor on load. New auto-added constraints
  must follow this pattern or round-trips will double them.
- **The `param` table serializes in definition order.** Its JSON preserves the
  order parameters were defined so forward references and reload stay
  reproducible. Keep that order on marshal/unmarshal.
