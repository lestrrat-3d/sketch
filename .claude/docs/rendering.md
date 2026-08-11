# Rendering overlays — `annotate.go`, `frame.go`

Detail moved out of CLAUDE.md's architecture table. Read before adding an annotation overlay, changing DOF colouring, or touching the frame/grid/watermark render path.

## Question router

| Your question | Section |
|---|---|
| Which overlays exist and what are their defaults? | `annotate.go` — overview |
| What does the status badge show on a skipped report? | `writeStatusBadge` must branch on skipped analysis |
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
(`Verify` DOF/Status/Solvable card), `WithProfileFill` (valid `Profiles()`
regions only, canonical sort for determinism),
`WithAnnotationColor`/`WithAnnotationScale`.

### `writeStatusBadge` must branch on skipped analysis

**`writeStatusBadge` is the ONE overlay that reads a `VerificationReport`, and
it must branch on that report's SKIPPED-ANALYSIS state**: on a report `Verify`
stopped early — a nil/corrupt/foreign handle, or non-finite geometry, both
causes alike — `DOF`, `Status` and `Solvable` hold unevaluated zero values the
report's own doc comment says are not verdicts, so rendering them paints a
number on the drawing for a sketch nothing analysed (`DOF 0` for geometry with
free degrees of freedom). The card names the incomplete state instead, in the
same visual shape, and one text serves both causes because the claim it makes is
only that no analysis stands behind it. `TestStatusBadgeSkippedAnalysis`
(`annotate_test.go`) pins a subtest per cause, each on a fixture whose
COORDINATES stay finite so the exporter's own `ErrNonFiniteGeometry` refusal
cannot fire and the render really reaches the badge. The other overlays read no
report — `WithDOFColoring`, `WithConflicts` and `WithProfileFill` compute live
(`movableVars`, `Diagnose`, `Profiles`) — but that is not the same as being safe
on this state.

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
