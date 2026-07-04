# Constraint & dimension visualization — annotated rendering + README gallery

## Context

The README embeds compiled example *code* (via `<!-- INCLUDE -->` +
`internal/cmd/genreadme`) but contains **zero rendered pictures**. A first-time
reader cannot see what a solved sketch looks like, and — worse — cannot see the
constraints/dimensions or the *verification* that make this a constraint-solving
verification oracle rather than a drawing library. The README's own "Why this
exists" sells DOF analysis, conflict/redundancy detection, ambiguity probing and
"does the solved geometry match intent." The gallery must lead with **that**
story, not with decorative geometry. `docs/fusion-gap-analysis.md:181` already
lists "under-constrained visualization data" (Fusion's blue/black DOF coloring)
as a parity gap.

This increment ships an **annotation-rendering layer** (dimensions, geometric
glyphs, DOF coloring, conflict/status overlays) plus an **image pipeline + README
gallery**, delivered in explicit stages (see "Staging").

## Goals / Non-goals

**Goals**
- Render dimensional constraints as CAD-style dimensions (line + extensions +
  arrowheads + value label in the dimension's own unit).
- Render geometric constraints as small badge glyphs anchored to referenced
  geometry.
- **Visualize verification**: DOF coloring (free vs. constrained), conflicting-
  constraint highlighting, a solved-status badge, and an ambiguity two-up.
- Opt-in via render options; **default off** so existing exporter output and
  golden tests stay byte-identical.
- A committed, always-in-sync image set + README gallery that makes a first-timer
  understand the *product* (verification + parametric) — not just the drawing.
- Correctness-observable per north-star: tests assert output contains expected
  markers/labels/colors, not merely "it rendered".

**Non-goals (this increment)**
- A public constraint-introspection API (Deferred — internal renderer ships
  first; public surface extracted from it later).
- Interactive/GUI concerns (hit-testing, drag handles, hover).
- Collision-free professional dimension layout; a bounded heuristic suffices.
- Rich PNG annotation text (PNG rasterizer has no real font — SVG is the
  annotated target; see PNG note).
- Exhaustive capability coverage in the gallery (units/3D/reference get at most
  one figure each or are deferred; breadth is staged, not big-bang).

## Architecture decision: internal renderer first

`Constraint` exposes only a private `residual()`; geometric constraints
(`horizontal`, `parallel`, …) are **unexported concrete types** returned as the
interface, so nothing outside the package can introspect kind or referents.
Dimensional handles are exported with exported entity fields (`Distance.P1/P2`,
`Radius.C`, `Angle.L1/L2`, `symmetric.Axis`, …) plus `Target()`/`Driven()`, but
there is no uniform accessor. The `Circular`/`Elliptical` sealed interfaces
expose `R()` / `Rx()/Ry()/Rotation()`; the **center accessor `centerPt()` is
unexported** — fine, because the renderer lives inside the package.

Decision: **build the annotation renderer inside package `sketch`**, type-
switching on the concrete constraint types (next to `svg.go`, which already
type-switches on entities). This avoids committing to a public API shape before
we know what the renderer needs. A public introspection API
(`Sketch.Annotations()` returning typed descriptors) is the north-star-aligned
follow-up (Deferred + CLAUDE.md open questions), **extracted from** this
renderer's proven per-constraint data model rather than guessed up front.

## Coordinate space & the y-flip (load-bearing correctness)

`svg.go` flips y **per coordinate** in `ty` (no SVG group transform, scale 1:1),
and already has to manually negate rotation for the one directional element today
(the `<ellipse>`: `svg.go:297-303`, "negate the angle"). Every annotation this
increment adds is directional and hits the same trap. **Primary rule: compute
annotation key *points* in sketch coordinates, map them through `tx`/`ty`, and
derive every screen direction/arrowhead/arc from the mapped points.** Because
`tx`/`ty` are a pure translate + y-flip at scale 1:1 (screen units == sketch mm),
a direction built from two mapped points is already correctly oriented — **no
per-case sign flip.** Consequences:
- **Arrowheads / dimension lines**: build endpoints as sketch points, map via
  `tx`/`ty`, then form the head from the screen-space delta. No negation.
- **Angle arc**: build the arc from its two mapped endpoint directions (the
  screen-space line midpoint directions), so it sweeps correctly with **no**
  manual negation. Do NOT also negate — that double-flips.
- **Manual negation is required ONLY when you emit an SVG `rotate(θ)` transform or
  draw from a scalar `center+sweep`** — i.e. the `<ellipse>` case and, if used,
  the `EllipseRotation` glyph built from `E.Rotation()` directly. Prefer the
  point-mapping form (compute the rim/axis endpoint as a sketch point, map it) to
  avoid needing the flip at all.
- **Text** is never mirrored (no group transform), so labels read normally.

## What renders today (unchanged baseline)

`svg.go` / `png.go`, shared config via `applyRenderOption`:
- Geometry: line, circle, arc, elliptical arc, ellipse, spline family, conic,
  NURBS (`<line>`/`<circle>`/`<ellipse>` or sampled `<path>`).
- Point markers: red `#d93025` free, black `#202124` fixed (toggle
  `WithShowPoints`).
- Construction: dashed grey `#bbbbbb`. Reference: solid orange `#e8731a`.
- Y-axis flipped (math orientation, y up).

The annotation pass is **additive** and gated behind options that default off.
**Z-order (resolved):** geometry first, then point markers, then all annotations
(dimensions, glyphs, badges) **on top** — annotations must never be occluded by
geometry. DOF coloring is the exception: it *replaces* the stroke/fill color of
existing geometry/point primitives inline (not an overlay).

## Render options

Add `SVGPNGOption`s (follow the `svgPNGOption` combined-marker pattern so one
value flows to both `SVG` and `PNG`):

| Option | Default | Effect |
|---|---|---|
| `WithDimensions(bool)` | `false` | draw dimensional constraints |
| `WithConstraints(bool)` | `false` | draw geometric-constraint glyphs |
| `WithDOFColoring(bool)` | `false` | color free vs. constrained points/entities (blue/black); free points also rendered **hollow** (redundant non-color channel) |
| `WithConflicts(bool)` | `false` | highlight conflicting constraints' geometry in red (from `Diagnose()`/`ConflictSet()`) |
| `WithStatusBadge(bool)` | `false` | draw a small text badge: `DOF=n`, `fully/under/over-constrained`, `converged` |
| `WithProfileFill(bool)` | `false` | translucent fill under **valid** closed regions from `Profiles()` |
| `WithAnnotationColor(string)` | `#5f6368` | dimension lines / glyph stroke |
| `WithAnnotationScale(float64)` | `1.0` | multiplies glyph/text/arrow size (else derived from bbox diagonal) |

All defaults off ⇒ current output verbatim. Sizing derives from the drawing's
bbox diagonal (`s.bounds()`/`renderBounds`) so annotations scale with the sketch
at any coordinate scale; `WithAnnotationScale` tunes it.

## The annotation model

Internal, unexported. The pass walks `s.cons` / `s.points` / `s.ents` **in slice
order** (never a `map[Entity]…` in the emit path — see Determinism) and appends
annotation primitives, then emits SVG.

### Label formatting

A dimension label is NOT `units.Value.String()` verbatim: that yields `30 deg`
(the Degree unit's symbol is the ASCII `"deg"`, `units/unit.go`), and there is no
`⌀` for diameters. Add a small internal `dimLabel(d Dimension) string` that reads
`Target()` and prettifies for display: `deg`→`°`, diameter prefixed `⌀`, radius
prefixed `R`. Driven dimensions are wrapped in parentheses and tinted lighter
(`Dimension.Driven()`). Load-bearing symbols (`⌀`, `°`, `R`) are emitted as
`<text>`; if font portability proves flaky on GitHub (see Portability), fall back
to `<path>` outlines for the glyph characters.

### Dimensions (gated by `WithDimensions`)

Draw: two **extension lines** from the measured features, one **dimension line**
with **arrowheads**, a **text label** at the dimension-line midpoint.

| Handle | Fields | Placement |
|---|---|---|
| `Distance` | `P1,P2 *Point` | dim line parallel to screen P1→P2, offset perpendicular by `gap = k·bboxDiag` |
| `HorizontalDistance` | `P1,P2` | horizontal dim line offset below/above the span |
| `VerticalDistance` | `P1,P2` | vertical dim line offset left/right |
| `DistancePointLine` | `P *Point`, `L *Line` | foot of perpendicular P→L via `geom.ClosestPointOnLine`; extension segment P→foot; direction floored with `norm()`; if foot ≈ P (distance ≈0) skip arrowheads, draw a leader + label only |
| `DistancePointCircle`/`DistanceLineCircle`/`DistancePointArc`/`DistanceLineArc` | `P/L` + `C/A` | leader from feature to the nearest circle/arc point, gap label |
| `DistanceLines` | `L1,L2 *Line` | the constraint forces the lines parallel → dim line perpendicular between the two, anchored at L1's screen midpoint |
| `Offset` | `Src,Dst *Line` | signed offset leader between the (parallel) lines |
| `Radius` | `C Circular` | radial leader center→rim; rim direction = arc mid-angle for an `*Arc` (stays on the drawn sweep), any fixed direction for a full circle; label `R…` |
| `Diameter` | `C Circular` | for a full circle, diametral line through center; for an `*Arc`, a radial leader at the arc mid-angle labeled `⌀…` (a full diameter chord would leave the sweep) |
| `Angle` | `L1,L2 *Line` | see below — anchored via midpoints/bisector, NOT a carrier intersection |
| `ArcLength` | `A *Arc` | leader to arc mid-angle point, length label |
| `SemiMajor`/`SemiMinor` | `E Elliptical` | leader from center along the (screen-negated) major/minor axis to the rim |
| `EllipseRotation` | `E Elliptical` | small angle glyph at center between the x-axis and the (screen-negated) major axis |

**`Angle` and `perpendicular` never solve a carrier intersection.** `Angle`
(`constraint.go`) is defined on the two lines' *direction vectors* and admits
parallel/anti-parallel targets (`0°`,`180°`); `perpendicular` never forces the
segments to meet. A carrier intersection is therefore undefined (parallel →
Inf/NaN) or far off-canvas. Placement instead uses the two lines' **screen
midpoints and directions**: draw the angle arc/right-angle square anchored on the
bisector near the midpoints, and when `|cross(dir1,dir2)| → 0` (parallel) fall
back to a plain label between the midpoints — never divide by a near-zero cross.

Missing math helpers go in `geom` (context-agnostic): `geom.ClosestPointOnLine`
and `geom.LineLineIntersection` already exist in `geom/intersect.go`; add an
arc-midpoint-direction helper there (none exists today). The renderer reads shape
values through `Geometry()` snapshots and the sealed-interface accessors.

### Geometric glyphs (gated by `WithConstraints`)

A small badge near the referenced entity's screen midpoint, offset along the
entity normal, with a **per-anchor stacking offset** so multiple glyphs on one
entity don't overlap. The stacking counter is computed from slice-order position
(e.g. a small `[]struct{anchor; n}` scan or an index-keyed count), **never a
`map[Entity]int`**, to keep emit order deterministic.

| Constraint (concrete type) | Fields | Glyph |
|---|---|---|
| `horizontal`/`vertical` | `L` | `H`/`V` tick at line mid |
| `horizontalPoints`/`verticalPoints` | `P1,P2` | `H`/`V` at the pair midpoint |
| `parallel` | `L1,L2` | matching single-chevron on each line mid |
| `perpendicular` | `L1,L2` | right-angle square anchored via midpoints/bisector (no carrier intersection) |
| `pointOnLine`/`pointOnCircle`/`pointOnEllipse` | `P`,target | small ring at P |
| `collinear` | `L1,L2` | collinear bar on both |
| `coincident` | `P1,P2` | concentric ring at the shared location |
| `midpoint`/`midpointOf` | `P,L` / `Mid,P1,P2` | midpoint tick |
| `symmetric`/`symmetricLines` | pair + `Axis *Line` | mirror glyph on each + axis emphasis |
| `concentric` | `C1,C2 Circular` | concentric double-ring at shared center |
| `equalLines` | `L1,L2` | equal tick `=` on each; stacked `=`/`==`/`≡` per equal-group to distinguish groups |
| `equalRadii` | `C1,C2 Circular` | equal tick near each rim |
| `tangent*` families | line/curve or curve/curve | tangent `T` badge at the contact |

**Tangency glyph anchor:** anchor curve tangency (spline/conic/NURBS) glyphs on
the **line-operand's screen midpoint** — a stable, always-defined point that
needs no solve state. Rationale: `SVG()`/`PNG()` receive no `Solve` `Result` and
the `Sketch` stores no converged flag, so a render-time convergence signal is not
available; the aux contact var `S(t)` is only meaningful after a converged solve
and can sit anywhere on the curve otherwise, so it is **not** used for placement.
For line/circle and endpoint tangency the contact point is directly computable
and used. Glyphs are informational, never load-bearing. (Anchoring on the solved
contact witness — via a stored converged flag or a render-time residual-norm
check — is a Deferred refinement.)

Glyphs render as tiny `<path>`/`<text>` groups via a shared
`glyph(kind, x, y, size, color)` helper. Ticks/squares/chevrons are `<path>`;
letters use `<text>` (with the Portability fallback).

### DOF coloring & verification overlays

- `WithDOFColoring`: recolor points via `Point.IsFullyConstrained()` — points
  that are **not fully constrained** render **blue + hollow**, fully constrained
  render **black + filled** (shape channel redundant with color for colorblind
  safety). Entities with any not-fully-constrained point render blue. Uses
  `FreePoints()`/`IsFullyConstrained()`. **Note the term overload:** "free" here =
  `IsFullyConstrained()==false`, distinct from the baseline point marker's free =
  *ungrounded* (`IsFixed()==false`); DOF coloring **overrides** the baseline
  red/black point colors when on.
- `WithConflicts`: from `Diagnose()`/`ConflictSet()`, stroke the geometry of
  conflicting constraints in red and badge them. Reaching that geometry requires
  an in-package type-switch on the concrete constraint types (the interface
  exposes no referents) — consistent with the in-package renderer decision.
- `WithStatusBadge`: a corner `<text>` card — `DOF=n`,
  `fully/under/over-constrained`, `solvable=…` — from `s.DOF()`/`Verify()`. The
  "converged/solvable" line comes from `VerificationReport.Solvable` (there is no
  field literally named `converged`); status from `VerificationReport.Status`.

**Color precedence:** when both `WithDOFColoring` and `WithConflicts` are on,
**conflict-red wins** over DOF-blue for a given entity. All three overlays are
computed non-mutatingly from existing diagnostics.

## Overlap & determinism

- Per-anchor stacking (above) prevents same-entity glyph pileup.
- Dimension lines use `gap = k·bboxDiag`; multiple dims on the same feature pair
  stack by incrementing the gap.
- No global collision solver (documented limitation; a layout pass is a
  follow-up).
- **Determinism (byte-stable output) requires all of:** no timestamps/randomness;
  fixed float formatting (existing `trimFloat`/`f` at 4 dp); **map-free emit**
  (walk `s.cons`/`s.points`/`s.ents` in slice order; no `map[Entity]…` in the
  path); and **stable region order** for `WithProfileFill` — sort `Profiles()`
  output by a **total** key (min-corner x, then min-corner y, then area, then
  boundary-edge count as a final tiebreak) before emit rather than trusting
  arrangement/DCEL walk order. State these in the generator so an implementer does
  not reach for a pointer-keyed map.

## Profile fill (`WithProfileFill`)

`Profiles()` requires a solved sketch and can return regions with `Valid=false` /
`SelfIntersecting=true`, or an empty set. Fill **only `Valid` regions** with a
translucent color under the geometry (even-odd fill for holes); skip invalid/
self-intersecting boundaries (filling a self-crossing polygon renders artifacts).
This doubles as the first visual for the profile engine, which today has no
rendering at all.

## Image pipeline + README gallery

### Generator: `internal/cmd/genimages`

New stdlib-only command mirroring `genreadme`. Constructs the curated sketches
in-process (reusing example builders where practical so pictures track the
compiled examples), solves them, renders **annotated** SVG per the image's intent,
and writes committed `docs/images/*.svg`. Wired into `//go:generate` alongside
`genreadme` (`generate.go`). Output is deterministic (see Determinism).

**Multi-state heroes need a compose step (explicit genimages machinery).**
`SVG()` renders a single sketch state and self-derives its own viewBox
(`renderBounds`), so two independently-rendered states have mismatched coordinate
frames — an overlay would be misaligned. The `ambiguity.svg` (two probe configs,
ghost + solid) and `tools.svg` (before → after) heroes therefore require a small
genimages-internal composer: pose each state (probe configs via
`Configuration.PointXY(p)`, which returns coords with **no re-solve or mutation**;
tool states via the actual tool call), compute a **union bbox** across states,
and render every state into one shared viewBox with ghost styling (reduced
opacity / dashed) for the non-primary state. This composer is a listed deliverable
of the pipeline stage. Fallback if it proves heavy: emit them as separate
side-by-side files (like `dof-*`/`parametric-*`) and lay them out two-up in the
README.

### Images-in-sync guard (replaces the discredited "images don't drift" claim)

Images ARE generated from example builders and WILL drift. Add a test
(`genimages` sync test) that regenerates every hero into a temp dir and
**byte-compares against the committed `docs/images/*.svg`**, failing on
divergence — the same "generated artifact must be regenerated & committed"
discipline `genreadme` implies. This is the staleness guard; `go generate` must
reproduce the committed files exactly.

### Format: committed SVG

SVG under `docs/images/`, referenced from README by relative path. Scalable,
tiny, diff-friendly, crisp glyph/text. **Assumption (external, not verifiable
in-repo): GitHub renders committed SVGs referenced from a README via relative
path.** If that fails in practice, `genimages` also emits PNG heroes (stdlib
rasterizer) as a fallback and the README references PNG — decided at gallery-
wiring time, not now.

### Portability

GitHub/arbitrary SVG viewers substitute fonts and may drop unicode glyphs
(`⌀`,`°`,`≡`,chevrons). For the committed **hero** images, load-bearing glyphs
are emitted as `<path>` outlines (extend the tick/square approach to `R`/`⌀`/
letters) so they render identically everywhere. Live `SVG()` output for
programmatic callers may keep `<text>` for brevity.

### Curated images — verification & parametric first

The gallery leads with the *product*. Every generated hero is embedded (grouped
below); we do not generate assets we don't surface.

| File | Sketch | Shows |
|---|---|---|
| `dof-underconstrained.svg` / `dof-constrained.svg` | same sketch, two-up | DOF coloring: free (blue/hollow) vs. fully constrained (black), status badge |
| `conflict.svg` | over-constrained sketch | conflicting constraints highlighted red (`ConflictSet`) |
| `ambiguity.svg` | tangent/mirror-flip sketch | two `ProbeConfigurations()` solutions overlaid (ghost + solid), "same constraints, two solutions" |
| `parametric-before.svg` / `parametric-after.svg` | quickstart, width 20→35 | parametric re-solve as a before/after pair, dimensions shown |
| `hexagon.svg` | examples hexagon | equal-length ticks, parallel/angle glyphs + dimensions |
| `fillet.svg` | fillet example | tangent glyphs + radius dimension (modification-tool result) |
| `tools.svg` | pattern/mirror/offset before→after | modification tools (the most self-explanatory capability) |
| `profiles.svg` | profiles example | filled valid closed regions (net area, holes) |
| `geometry.svg` | spline + ellipse + arc in a profile | curved-geometry variety |
| `constraints-legend.svg` | synthetic | one of each geometric glyph with captions (decoder ring) |

Secondary/optional (only if cheap; else Deferred): a `units.svg` (same sketch
dimensioned in mm and in) and a `reference.svg` (orange locked edges beside
solved geometry).

### README changes

- Add a **Gallery** section after "Why this exists", led by the DOF/conflict/
  ambiguity/parametric visuals (the verification story), then constraints/tools/
  profiles.
- Inline the relevant image beside its section: parametric two-up by Quick start
  / Parameters, legend by Constraints, `profiles.svg` by Profiles.
- **Per-hero captions naming the constraints shown** (e.g. "‹ = parallel, |=| =
  equal length") — the legend alone is insufficient; glyph + meaning must be
  co-located.
- **Descriptive alt text per image**, spelled out at authoring time (not empty/
  lazy) — the audience explicitly includes agents/humans eyeballing output.
- Hand-placed relative `![alt](docs/images/x.svg)` links; `genreadme` stays
  code-only (no new marker). The **sync test**, not `genreadme`, guards drift.
- **Fix stale prose:** the Profiles section currently says boundaries that cross
  without sharing a point "are not subdivided into regions (yet)" — the
  arrangement engine now does bare-crossing subdivision, holes/nesting, net area
  and validity. Refresh that paragraph so picture and text agree.

### Accessibility

- Redundant non-color channels: free vs. fixed points = **hollow vs. filled**
  (not color alone); construction stays dashed vs. reference solid; conflict adds
  a badge, not only red.
- Descriptive alt text (above).
- Note a colorblind-safe check on the chosen blue/black/red/orange/grey set.

## Staging (what ships, in order — each stage lands with its own tests)

1. **Option plumbing + dimensions** — `WithDimensions`, `dimLabel`, the linear/
   radial/diameter/angle/arc-length/semi-axis dimension renderers, screen-space
   rule, golden-baseline guard (default-off byte-identical). Ships value alone.
2. **Geometric glyphs** — `WithConstraints`, the glyph library + per-anchor
   stacking, witness-anchor gating.
3. **Verification overlays** — `WithDOFColoring`, `WithConflicts`,
   `WithStatusBadge`, `WithProfileFill` (valid-regions-only).
4. **Pipeline + gallery** — `internal/cmd/genimages`, committed heroes, the
   in-sync test, README gallery + prose fixes + alt text.

## Tests (external `xxx_test`, `testify/require`, assert on output)

- `annotate_test.go`: per dimension kind and per geometric glyph kind, build a
  minimal sketch, render with the relevant option, assert the output contains the
  expected marker signature (label text `20 mm` / `30°` after `dimLabel`, an
  arrowhead path, a glyph group). Assert **default-off output is byte-identical**
  to today's (golden regression guard).
- Degenerate cases: `Angle`/`perpendicular` on parallel lines render finite
  coordinates (no NaN/Inf); `DistancePointLine` at ≈0 distance does not emit a
  zero-length arrow; `Radius`/`Diameter` on an `*Arc` anchor within the sweep.
- Driven dimensions render parenthesized; DOF coloring marks free points hollow/
  blue and constrained black; conflict highlighting reddens the right geometry.
- Sizing scales with bbox (render at 1× and 1000×, glyph size proportional).
- `WithProfileFill` fills only `Valid` regions.
- `genimages` in-sync test: regenerate to temp dir, byte-compare every hero
  against committed `docs/images/*.svg`; assert each parses as XML and is
  non-empty.

## Deferred (explicit follow-ups, recorded in CLAUDE.md open questions)

- **Public introspection API** — `Sketch.Annotations()` / per-constraint
  `Describe()` returning typed exported descriptors (kind + referenced entity
  handles + dimension value/unit/driven), extracted from this renderer's data
  model; consumed by a future DSL/GUI and by tests.
- **Global dimension-layout / collision avoidance.**
- **Rich PNG annotation text** (needs a bitmap/vector font); SVG is the annotated
  target now.
- **Full capability breadth in the gallery** — units, 3D world/planes
  (`WorldPolyline`), reference geometry beyond the optional secondary figures.
- **Interactive concerns** (hit-testing, drag handles) — UI layer.

## Implementation status

Shipped (`annotate.go`, `internal/cmd/genimages`): all four render stages —
dimensions, geometric glyphs, verification overlays (DOF coloring, conflicts,
status badge, profile fill) — plus the generator, the in-sync + well-formed
tests, and a README gallery of ten single-state / before-after hero SVGs
(`quickstart`, `hexagon`, `dof-underconstrained`/`dof-constrained`, `conflict`,
`parametric-before`/`parametric-after`, `profiles`, `fillet-before`/
`fillet-after`).

Deferred to a follow-up (recorded in CLAUDE.md open questions): the **shared-
bounds multi-state compositor** and therefore the **ambiguity overlay** and a
single-file **tools before→after overlay** (tools ship as a two-up pair instead);
the **constraints legend** image; the **public introspection API**; rich PNG
annotation text; path-outlined glyphs.

## Verification

- `gofmt`, `go vet`, `go test ./...` clean.
- Golden baseline: existing SVG/PNG tests unchanged (annotations default off).
- `go generate ./...` reproduces `docs/images/*.svg` byte-for-byte; the in-sync
  test fails on drift.
- README renders the gallery on GitHub (relative SVG links resolve; PNG fallback
  if not).
