# Rendering overlays — `annotate.go`, `frame.go`

Detail moved out of CLAUDE.md's architecture table. Read before adding an annotation overlay, changing DOF colouring, or touching the frame/grid/watermark render path.

## Question router

| Your question | Section |
|---|---|
| Which overlays exist and what are their defaults? | `annotate.go` — overview |
| What does the status badge show on a skipped report? | `writeStatusBadge` must branch on skipped analysis |
| Where does a named entity's label get drawn? | `WithLabels` takes its entity anchor from `entityPoints` |
| How does a label avoid the geometry and other labels? | `WithLabels` searches for each name's position |
| Why doesn't the status badge call `Sketch.Verify`? | `badgeVerify` computes only what the badge renders |
| How does DOF colouring behave on non-finite geometry? | `WithDOFColoring` marks everything free when refused |
| How is annotation geometry mapped to screen space? | The load-bearing annotation-geometry rule |
| What does a framed render always carry? | `frame.go` — windowed framing |

Navigation only — the sections below are the authority.

## `annotate.go` — annotation overlays

### Overview

Annotation-rendering overlay for `Sketch.SVG` (in-package so it can type-switch
the unexported constraint types). Opt-in `SVGPNGOption`s, all **default off** so
baseline output stays byte-identical: `WithDimensions` (CAD dimension lines +
arrowheads + unit label via `dimText`, driven ones parenthesized),
`WithConstraints` (geometric-constraint glyph badges, per-anchor slice-order
stacking — no `map[Entity]`), `WithDOFColoring` (free = blue+hollow circle,
grounded/`IsFixed` = green filled square (`colorFixed`) so the origin anchor
reads distinctly, other constrained = black filled circle; points via
`movableVars`, entities via `entityMovable` so a circle with a free radius reads
blue — the per-entity `Sketch.EntityIsFullyConstrained`), `WithPixelWidth`
(display px, viewBox unchanged), `WithConflicts` (conflicting geometry red via
`Diagnose` + `constraintRefs`; conflict-red > DOF-blue), `WithStatusBadge`
(DOF/Status/Solvable card via `badgeVerify`, see below), `WithProfileFill`
(valid `Profiles()` regions only, canonical sort for determinism), `WithLabels`
(the names points and entities carry, see below),
`WithAnnotationColor`/`WithAnnotationScale`.

### `WithLabels` searches for each name's position

**Where a name goes is searched for, not fixed** (`labels.go`). A fixed offset
does not survive a real drawing: on a sketch naming two dozen points inside one
small figure, a name pinned up and to the right of every marker lands on the next
marker, on a construction line, or on another name. `labelPlacer` tries a ring of
positions about the anchor — the first choice, then the eight compass directions
at one step and again at two — and keeps the lowest-scoring one. The score
weights what the box sits on: off the canvas (1000) beats a name (100) beats a
marker (10) beats the drawing's own curves (1), plus a small rank term (0.01 per
candidate) that keeps an uncrowded drawing on its first choice and makes every
tie resolve the same way on every run. Nothing is ever dropped: when every
position collides the least bad one is still drawn, because a crowded label says
more than no label.

**A name gets a leader when it was MOVED or when it has a RIVAL.** Moved is the
obvious case: the name is no longer where a reader looks for it. The rival case
is the one the real drawing taught — a name at its own first choice, up and to
the right of its dot, is also up and to the left of the next dot, and on a
lattice of two dozen points proximity cannot pair it at all. `needsLeader` calls
a marker a rival when it comes within `leaderRivalRatio` of the name's own
distance to its anchor. A name with neither problem gets none, so an uncrowded
drawing stays clean.

**A leadered name is stood off far enough to carry its leader** (`leaderMinReach`
arrowheads, re-searched at `outerRingStep`). At one step of travel the line is a
few pixels and the head is smaller still, which is how the first version of this
failed; a name that needs a line drawn to it is not being read by its position
anyway, so the extra step costs nothing.

**A name and its leader are cleared against the page** with a halo: the text is
written twice, once thickened in the background colour, and each leader line is
painted over a wider casing of it. Without that, a hairline laid along a dashed
construction line is lost in the dashes — and on a lattice that is exactly where
leaders run. A transparent page (`WithBackground("none")`) gets no halo, since
there is no colour to clear with. The halo copies are `<text>` elements carrying
a `stroke`, which is how a reader of the output tells them from the names.

**The leader is a CAD note leader — underline, line, arrowhead — and all three
parts are load-bearing.** A bare line from the text to the point was tried first
and failed on the real drawing it was built for: at one step of travel the
visible segment is a few pixels, and the reader cannot see which end belongs to
which name. The UNDERLINE binds the line to its own text, so the line leaves the
word rather than the space near it; the ARROWHEAD says which of several nearby
dots is meant. The head is sized down on a short leader
(`leaderArrowShare`) so it can never be longer than the line carrying it, and
both lines are drawn at `leaderStrokeFraction` of the geometry's stroke so the
annotation does not read as another edge.

Leaders are not obstacles for the names placed after them, which is a limitation
rather than a decision: scoring against them would make each placement depend on
the leaders of every earlier one.

Scoring is against BOXES, so the text's width has to be guessed —
`labelWidthPerRune`, deliberately generous, since a box too wide only moves a
name that would have just fitted while one too narrow lets two names overlap
after the search reported they would not. The exporter writes text for the
viewer's own font, so no true advance width is knowable here. The geometry a name
avoids comes from `entityPolyline`, the one sampling switch in the package, which
the PNG rasterizer draws through as well.

### `WithLabels` takes its entity anchor from `entityPoints`

**An entity's label reads off the mean of `entityPoints(e)`, not a position a
type switch in the renderer assigns it.** That accessor is the one grounding and
the removal cascade already read an entity's defining points through, so a new
entity type gets an anchor by satisfying the contract it has to satisfy anyway; a
switch here would compile fine while silently anchoring the new type at the
origin, or not at all. The mean is the midpoint of a line, the centre of a circle
or ellipse, and the average of a spline's control points. An entity
`entityPoints` does not know reports no anchor and is skipped.

### A point's label and an entity's are told apart by STYLE

**An entity's name is italic and a point's is upright, and that is the channel
the distinction rests on.** Position cannot carry it: the search moves a name to
wherever there is room, so a point's name and an entity's can end up in the same
relation to their anchors. The two kinds do start from different first choices —
a point's beside its marker, which it has to clear, and an entity's centred on an
anchor no marker occupies — but that is a preference, not a promise. Both go
through the one `nameText` emitter, so nothing but the italic flag and the
candidate ring can differ between them.

### `writeStatusBadge` must branch on skipped analysis

**`writeStatusBadge` is the ONE overlay driven by verification state, and it
must branch on the SKIPPED-ANALYSIS case**: on a nil/corrupt/foreign handle, or
non-finite geometry, both causes alike, DOF/Status/Solvable hold unevaluated
zero values that are not verdicts, so rendering them paints a number on the
drawing for a sketch nothing analysed (`DOF 0` for geometry with free degrees
of freedom). The card names the incomplete state instead, in the same visual
shape, and one text serves both causes because the claim it makes is only that
no analysis stands behind it. `TestStatusBadgeSkippedAnalysis`
(`annotate_test.go`) pins a subtest per cause, each on a fixture whose
COORDINATES stay finite so the exporter's own `ErrNonFiniteGeometry` refusal
cannot fire and the render really reaches the badge. The other overlays read no
report — `WithDOFColoring`, `WithConflicts` and `WithProfileFill` compute live
(`movableVars`, `Diagnose`, `Profiles`) — but that is not the same as being safe
on this state.

### `badgeVerify` computes only what the badge renders

**`writeStatusBadge` does not call `Sketch.Verify`.** The card shows only
DOF, Status, Solvable and whether analysis ran; `Verify` additionally builds
conditioning, free points, profiles, parameter validity and (opt-in) the
ambiguity probe, of which `Sketch.Profiles`' arrangement pass alone dominates a
`Verify` call's cost on any sketch with curved or many-entity geometry — real
cost for values the badge never reads.

**The two cannot drift, because they are not two computations.** Both call
`Sketch.verifyCore` (`verify.go`), which owns the whole DOF/Status/Solvable
pass: `scanReferenceIntegrity` + `scanReferenceStaleness`, the non-finite screen
and its early-out, the residual norm against the tolerance, the one shared
`buildCommittedJacobian`, `rankAnalysisOn` + `dof`, and `conflictAnalysisOn` for
the conflicting/redundant sets. `badgeVerify` (`annotate.go`) calls it, runs
`classifyStatus` on the report, and stops; `Verify` calls it and carries on with
the rest. **Every field `classifyStatus` consults is filled inside
`verifyCore`**, so a new one reaches both callers at once — put anything the
status verdict depends on there, and anything only a full report needs after it.
`Conditioning` is deliberately outside: it gates `Trustworthy()`, never
`classifyStatus`, so the badge does not pay a singular-value pass for a number
it does not show. `TestStatusBadgeMatchesVerify` (`badge_verify_parity_test.go`)
pins the two paths as agreeing across under-constrained, fully constrained,
conflicting, redundant and invalid-profile fixtures;
`TestStatusBadgeSkippedAnalysis` (`annotate_test.go`) pins the skipped-analysis
card on both of its causes.

### `WithDOFColoring` marks everything free when refused

**`WithDOFColoring` marks EVERY point and entity FREE when `movableVars`
refuses** (non-finite geometry), rather than inventing a fourth colour: the
overlay's own vocabulary already has a value meaning "not proven constrained"
(blue + hollow), a new one would be a render-only answer to the fact the whole
screen exists to give ONE answer to, and no legend, hero or consumer knows how
to read it — while marking everything free makes the drawing agree with
`FreePoints` (which names every point, grounded ones included) and with both
per-handle bools on the same geometry. Rendering the computed colours instead
paints "fully constrained" black on geometry those three reads call free: on a
finite rectangle whose committed DRIVING dimension target is `Set(NaN)` every
coordinate is finite, so `bbox.finite()` never fires and the render succeeds.
The free marker wins over the grounded green square in `svg.go`, so the whole
drawing reads uniformly with no per-case code, and `WithStatusBadge` is what
names the incomplete state in words.

### The load-bearing annotation-geometry rule

**Load-bearing rule:** all annotation geometry computes key points in sketch
coords, maps through `tx`/`ty`, and derives every screen direction/arrowhead/arc
from the mapped points — no per-case y-flip sign negation (only the `<ellipse>`
`rotate()` still negates). Design in `docs/constraint-visualization-design.md`.
Consumed by `internal/cmd/genimages` (regenerates the committed
`docs/images/*.svg` README gallery heroes; an in-sync test byte-compares a
regeneration).

## `frame.go` — windowed framing

Windowed framing for `Sketch.SVG` (opt-in, default off → byte-identical
baseline): `WithFrame` (outer padding + border rect; the sketch's `margin`
becomes the frame→geometry gap), `WithGrid` (origin-aligned background grid,
`niceStep` auto spacing 1/2/5×10ⁿ, emphasized x=0/y=0 axes; implies a frame),
`WithGridSpacing`, `WithFramePadding`. A framed render **always** carries the
fixed provenance watermark `WatermarkText` (= `github.com/lestrrat-3d/sketch`)
in the bottom outer padding — not an option, and no commit hash, so output is
fully deterministic and the in-sync test is a plain byte compare (a new commit
no longer churns the gallery). `SVG` adds an outer `pad` (shifts `tx`/`ty`,
grows the viewBox); grid+frame draw before geometry, watermark on top.
