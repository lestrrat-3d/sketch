# CLAUDE.md

Guidance for working in this repository. The project is young and many design
decisions are still open — this file captures the **vision**, the **invariants
worth protecting**, and the **questions still unsettled**. Read it before making
structural changes, and update it when a design variable gets resolved.

## What this is

A standalone, fully programmable **parametric 2D sketch engine** in Go, in the
spirit of the sketch environment in Autodesk Fusion. You build geometry in code,
relate it with geometric and dimensional constraints, and a numerical solver
moves the geometry so every constraint holds at once. Dimensions are editable,
so sketches are fully parametric.

The library is the foundation. A DSL/CLI, a GUI, and richer geometry are
expected to be built **on top of** this engine, not woven into it.

The **north-star use case** is a *headless sketch verification oracle*: a coding
agent authors a sketch and verifies it (solvability, constraint status, conflict
sets, closed profiles) before a human executes the equivalent in CAD software.
The roadmap toward that goal, and the **sketch/3D separation contract** (this
layer verifies against 3D-derived geometry it is *given*; it never *computes* it
from a solid — the seam is first-class reference geometry), live in
`docs/verification-roadmap.md`.

## North-star principles

1. **Library-first, engine at the core.** The constraint engine is the product.
   Everything else (rendering, serialization, future DSL/GUI) is a consumer of
   it and must not leak back into the solver's design.
2. **Curated dependencies.** The engine leans on the standard library plus a
   short, deliberate dependency list — do not add modules to `go.mod` without
   recording the decision here. Current approved dependencies:
   - `github.com/lestrrat-go/option/v3` — functional-options API. Used by the
     root `sketch` package only (`Sketch.SVG`, `Sketch.Solve`). The `geom`
     package keeps its **production** code standard-library-only, and `param`'s
     only production dependency is the `units` module, so both stay
     independently extractable.
   - `github.com/lestrrat-3d/r3` — the 3D coordinate-math layer (`r3.Vec`,
     `r3.Frame`), a standalone module of its own. Used by the root `sketch`
     package only (`plane.go`, `world.go`, `sketch.go`, the exporters); see
     "The `r3` module" below.
   - `github.com/lestrrat-3d/units` — the units-of-measure layer (`units.Unit`,
     `units.Value`, `units.System`), a standalone module of its own. Used by the
     root `sketch` package and by `param`; see "The `units` module" below.
   - `github.com/stretchr/testify/require` — test assertions, **test code only**
     (all packages). Never imported by production code.

   Keeping the runtime surface this small keeps the engine embeddable anywhere.
3. **Programmability over UI.** The API is the primary interface. Anything a
   user can do interactively should be expressible in code first.
4. **Correctness is observable.** Every capability ships with a test that
   asserts on solved coordinates / residuals, not just "it ran".

## Architecture at a glance

| File | Responsibility |
|---|---|
| `sketch.go` | `Sketch`, solver-bound geometry (`Point`/`Line`/`Circle`/`Arc`/`Ellipse`) authored from points, the parameter model, grounding, construction flag, `Geometry()` snapshots. **Every sketch owns an origin point** (`Sketch.Origin()`, design in `docs/origin-point-design.md`): a point at (0,0) the constructor creates with both vars fixed for the sketch's life, so geometry is anchored by CONSTRAINING to it rather than by `Fix` — which keeps the tie inside the constraint model, where `Diagnose`/`RemoveConstraint` can see it. **It is deliberately NOT in `s.points`**: a point's `id` IS its slice position and documents reference points by it, so putting the origin in the slice would shift every authored id, every point count and every existing document. Keeping it out leaves the authored id space untouched, and the passes that iterate `s.points` (`FreePoints`, the probe, the renderers, `names.go`) correctly do not see it. Two places needed an explicit exception, and both are load-bearing: **`owns`** (it decides ownership positionally, so without it a constraint to the origin reads as a FOREIGN handle and aborts verification) and **`positionShift`** (the FD pass translates every position by the authored centroid, and that is sound ONLY as a RIGID translation — leaving the origin behind collapsed it onto a point constrained to it and invented a redundant constraint; the centroid stays over the authored points so an origin-free sketch shifts exactly as before). `freeVars` filters on `fixed`, so the origin's vars never reach the Jacobian, rank analysis or conditioning, and DOF is unaffected. `MoveTo`/`Unfix`/`SetConstruction` are no-ops on it and `RemovePoint` refuses it — and so is **`UnfixEntity`** for an entity drawn from it, the easy one to forget: it releases every defining point, so an entity with the origin as an endpoint is a second door onto the same variables. **`entityPoints` is the ONE definition of "which points define this entity"** — a package-level function here (its subsystem-local twin in `reference.go` is gone), read by grounding, the removal cascade, serialization, `Verify`'s reference/foreign-handle scan, the `Sketch.Revision` fingerprint, per-entity DOF attribution and conflict coloring. A second copy is what a new entity type — or a new point on an existing one — gets added to one of and forgotten in the other, and the forgotten copy's consumers fail SILENTLY (`Verify` stops flagging a foreign point; `Revision` stops moving, so a stale `Profile` reads fresh) with the build, vet, lint and test gates all green. `entity_points_test.go` makes the coverage mechanical rather than a convention: it reflects over each entity's exported `*Point`/`[]*Point` fields — so a new point on an existing type needs no test edit — and asserts, per slot, that rewiring it within the sketch moves the revision (no solver var changes, so only the defining-point hash can) and that rewiring it to another sketch's point is reported as a foreign handle. A new entity TYPE must still be added to that test's sketch; nothing at run time can enumerate the types implementing a sealed interface. **A new entity type MUST implement the sealed `Entity` interface's unexported `isNil() bool`** (`return x == nil`, beside that type's `entity()`; it must never dereference the receiver). It answers `isNilEntity`, which is how `Verify`'s reference-integrity scan and `Sketch.ownsEntity` recognize a TYPED-nil operand — a non-nil interface holding a nil pointer, what `NewPointOnCircle(p, nil)` stores — before calling `entID()` on it and panicking. The rule lives on the interface, not in a type switch, precisely so forgetting it is a COMPILE error at `addEntity` (the total funnel, whose parameter is `Entity`): as a switch case it was invisible to build, vet, lint and the whole test suite, and the first consumer to hand the engine a typed nil of the new type crashed a documented non-panicking oracle call. It is unexported, so it is not a change to what an outside package can implement. |
| `compound.go` | Compound shape builders (`CreateRectangle`/`CreatePolygon`/`CreateSlot`): primitives + shape-holding constraints, returned as a grouping handle (handle itself is not serialized). |
| `tools.go` | Sketch-modification tools on committed geometry (`Trim`/`Extend`/`Break`, `CreateFillet`/`CreateChamfer`, `CreateMirror`, `CreatePatternRect`/`CreatePatternCircular`, `CreateOffset`): build-then-replace via the `geom` toolkit + `RemoveEntity`. Design in `docs/modification-tools-design.md`. |
| `profiles.go` | `Sketch.Profiles()`: closed planar regions via the `geom` arrangement engine — bare-crossing subdivision, holes/nesting, net area, and per-region validity (self-intersecting/degenerate). `Profile` carries `Outer`/`Holes` (`BoundaryEdge`s, whole or fragment), `Area`, `Valid`, `SelfIntersecting`; construction excluded, reference geometry included. **`Valid` is per-REGION, including its degeneracy half**: a region is invalid when an unresolvable condition reaches ITS OWN boundary curves (or is unattributable), so a sketch can hold both valid and invalid profiles and trouble in one corner no longer invalidates a disk across the sketch; `Verify`'s `ProfilesValid` stays the arrangement-wide verdict, so it can be false with `InvalidProfiles` empty (a condition that produced no region). Internal `buildProfiles` also surfaces arrangement degeneracy to `Verify`. **A `Profile` is a snapshot, so it can go stale**: each carries its origin `Sketch()` (freshly allocated every call — pointer identity can never prove provenance) and the `Revision()` it was built at, and `Profile.IsStale()` says the sketch has moved under it. A consumer that turns a profile into a solid MUST check — extruding a stale profile silently builds the old shape with no error anywhere. A `BoundaryEdge` also reports **which** sub-range it covers — `TStart`/`TEnd` (normalized `t∈[0,1]` in the entity's *natural* direction, so `TStart<TEnd` and `Reversed` alone carries walk order; never wrapping) plus **`TExact`**, which is the load-bearing half: it is true only when BOTH bounds come from the closed-form kernel, a sample vertex, or the curve's own endpoint. The closed-form kernel runs only on a pair whose **BOTH** sources are line/circle/arc (`analyticKind`), so **every** contact involving an ellipse/elliptical-arc/conic/spline/NURBS — *even against a plain line, and even when it is a tangency* — and every curve/curve crossing is sampled, and those fragments report `TExact=false`: the topology is right but the parameter only converges with sampling. A consumer that records a profile structurally or emits CAD from it must branch on `TExact`, never trust the range blindly. `Partial` and `TExact` are answered from **the emitted fragment itself**, never from a per-source "was this curve cut anywhere" proxy (which outlives pruning and reports a phantom `Partial` on a whole curve) and never from a numeric compare of the range against `[0,1]` (which cannot tell a bound that *is* the curve's end from a crossing that landed `1e-10` away — and would bless a sampled-bounded fragment as the whole curve, the unsafe direction). Instead each bound carries its **provenance** (`cut.srcEnd` → `arrEdge.endU/endV` → `frag`): it is either the curve's own domain end (an open curve's endpoint, or a closed curve's seam) or a cut/weld, and `makeCycle` sets `Whole` iff **both** surviving outer bounds are the curve's own ends — decided *after* pruning and coalescing, so a contact whose partner is pruned away, and a closed curve cut once (one edge leaving the contact and returning to it, so its seam is what bounds it), both correctly read whole. `split`'s dedup **ANDs** provenance as well as exactness into a coincident boundary, so a cut landing on a domain end is a cut; the one conservative corner is a closed curve whose single cut lands ON the seam, which then reads `Partial` over `[0,1]` — never a false `Whole`. Exactness is tracked per `cut` (`cut.exact`): `split`'s dedup **ANDs** coincident boundaries (a boundary a sampled cut lands on is only as trustworthy as that cut), and `makeCycle`'s fragment coalescing carries the **surviving outer bounds'** exactness (an interior boundary that coalesces away is not reported, so it is not folded in). The vertex table welds by **distance** while the crossing tests decide in **parameter** space, so a source can be split in the graph with no cut record at all: `taintMergedEndpoints` pushes an `exact:false` marker at any sample vertex two different sources weld at, and — since a weld happens for analytic pairs too — `auditMergedEndpoints` does the same for a `handled` pair's welds *unless* one of that pair's analytic events sits at the contact (an exact cut is never laundered into a sampled one, and a distance weld is never laundered into an exact cut). **Bound exactness is finally certified after canonicalization by `vertexCertifies`: TExact holds only when the graph vertex IS the bound's own point (`eval(param)`) within round-off (`weldIdentEps·scale`) — the merge tolerance is never used for the exactness decision, only for welding/topology.** |
| `revision.go` | `Sketch.Revision()`: a **fingerprint** (FNV-1a over the whole var vector + the entity set/types/construction flags + per-entity **instance identity** + per-entity defining points + per-entity **shape state**), not a counter — compare for **equality only, never order**. Derived from state rather than bumped per mutating method **on purpose**: there is no bump-site list to forget, so a new mutation path cannot silently leave the revision stale. It must cover everything `Profiles()` reads *and everything a `Profile` HOLDS* — coordinates *and* topology *and* the construction flag (toggling construction changes the region set while every coordinate stays put) *and* every shape value an entity resolves *and* which entity INSTANCES the profile's live handles point at. **Load-bearing rule: hash what `buildProfiles` CONSUMES and what a `Profile` HANDS OUT, never a proxy for it** — not an id, not a var index, and not "it is somewhere in `s.vars`". Three places that rule bites. (0) **Entity instance identity:** a `Profile` stores LIVE entity handles (`Profile.Entities`, `BoundaryEdge.Entity`) and a consumer records profiles structurally against `Sketch.Entities()`, so the fingerprint covers WHICH INSTANCE each handle is — a never-reused `uid` stamped by `Sketch.addEntity` (the single funnel every entity builder goes through) from a monotonic counter, dropped but never rewound by `RemoveEntity`. **`Revision` is a PURE READ** — it writes nothing (no map, no counter, no lazy init), so concurrent `Revision()`/`Profiles()` on one sketch are race-free and observing a sketch never invalidates a `Profile` of it. A uid is stamped ONLY where an entity enters the sketch (`addEntity`); "an entity in `s.ents` with no uid" is an **invariant violation, not a condition to repair on read** — the funnel is total and `Sketch.Entities()` hands out a `slices.Clone`, so it is unreachable through the public API (`revision_internal_test.go` asserts the invariant across every creation path: builders, compound builders, `tools.go`, reference geometry, both loaders). `Sketch.entUID` is the pure lookup; if it ever met an unstamped entity it hashes `unstampedUID` (`math.MaxUint64`), a value disjoint from every real uid (which start at 1 and only increase), so an unstamped entity can never fingerprint like a stamped one. Repairing on read instead — stamping during `Revision` — is the bug that was removed: it made a read-looking method a mutator and raced two concurrent readers on the uid map. The positional `id` is NOT that identity: removal renumbers, and a `Line` owns no scalar var to retire, so removing a line and creating an identical one leaves type, points, shape, id and the var vector all identical while the profile's handles dangle. The uid is in-memory state only — **never serialized**, and the counter **never rewinds, including across an in-place rebuild**: `Sketch.UnmarshalJSON` resets the struct (`*s = Sketch{…}`) but carries `nextEntID` over, so the rebuilt entities get fresh uids above the retired ones. Restamping from 1 there would make a reload of the SAME bytes reproduce the pre-rebuild revision while every entity handle a held `Profile` owns dangles — a stale profile reading fresh on the load path. Any new wholesale reset of a live `*Sketch` must carry the counter the same way (`World.UnmarshalJSON` does not: it builds fresh `*Sketch` values, leaving the old ones — and any profile of them — coherent). (1) **Defining points:** entity fields (`Line.Start`, …) are exported, so a point can be rewired to a `*Point` of another sketch or to a removed handle whose id a live point has since inherited; `buildProfiles` follows the pointer (`Point.Geometry()` → that point's own `s.vars`) and shares one `geom.Point` per distinct `*Point`. So each defining point is hashed as a triple — a first-seen **sharing ordinal** (never the pointer address: not deterministic across runs), its **id**, and the **coordinates the pointer resolves to** (`math.Float64bits`). (2) **Shape values**, in `entityStructuralState` — a switch with a case per entity type, hashing the value read through the SAME accessor `buildProfiles` uses: `*Circle` `r()`; `*Ellipse`/`*EllipticalArc` `rx()`/`ry()`/`rot()`; `*Conic` `rho()`; `*NURBS` `degree`/`knots`/`weights` (stored data the solver cannot move at all). Hashing the var vector is NOT a substitute for the solver-var ones: the vector covers the var VALUES, not the entity→var BINDING (swap two circles' `ri` and the vector is untouched while the disks trade radii). The var-vector hash stays as a cheap catch-all over solver state (aux vars included). **A new entity type MUST get a case there** if `buildProfiles` reads anything off it besides its points, or a change leaves the revision identical and a stale `Profile` reads fresh. `revision_internal_test.go` is the repo's one `package sketch` test file — a deliberate exception, since selector rebinding is unreachable from the external API and exporting a rebinder to test it would add the very footgun the fingerprint defends against. |
| `constraint.go` | `Constraint` interface and every constraint's residual + the public `New…` constructors. |
| `introspect.go` | Constraint introspection over the sealed `Constraint` interface: `ConstraintKind` (stable type id — **derived from `marshalConstraint`** so it never drifts from the JSON schema; internal constraints report `arc_radius`/`elliptical_arc_on`), `ConstraintRefs` (the referenced points+entities, via `constraintRefs`), `ConstraintResiduals` (`c.residual(nil)`), `IsInternal`. All read-only, package-level. |
| `names.go` | Optional, non-unique labels + first-match lookup: the embedded `named` (every `Entity`'s `Name`/`SetName`), constraint labels held on the sketch (`SetConstraintName`/`ConstraintName` in `conNames`, since a constraint is only ever an interface value), and `PointByName`/`EntityByName`/`ConstraintByName`. Names survive JSON round-trips (`name` on `jsonPoint`/`jsonEntity`/`jsonConstraint`) and are purged by the removal cascade. |
| `solver.go` | Levenberg–Marquardt solver, numerical Jacobian, DOF/redundancy (rank) analysis. |
| `diagnose.go` | Constraint diagnostics: `conflictAnalysis` (the shared dependency pass behind `RedundantConstraints`/`Diagnose`/`Verify`), `Diagnose` (redundant vs conflicting), `ConflictSet` (a conflicting constraint + the earlier ones it fights), `CheckConstraint` (pre-commit over-constraint rejection), `FreePoints`/`Point.IsFullyConstrained` (per-point free-DOF attribution) + `Sketch.EntityIsFullyConstrained` (per-entity: an entity is free when any defining-point coord OR any intrinsic shape var — `entityShapeVars`: circle radius / ellipse rx,ry,rot / conic rho; none for line/arc/spline — is in the null-space support). Design in `docs/diagnostics-design.md`. |
| `verify.go` | `Sketch.Verify(ctx context.Context, ...VerifyOption) *VerificationReport`: the headless-oracle aggregation layer — one non-mutating call gathering solvability, DOF, `Status`, redundant constraints, conflict sets, free points, profiles + their validity (`ProfilesValid`/`InvalidProfiles` — self-intersecting/degenerate regions gate `Trustworthy()`), stale/broken/foreign reference signals, parameter unit-kind validity (`ParametersValid`), the **advisory** `RankMargin` (how far the STRUCTURAL rank/DOF decision sits from the rank-zero cutoff — a fragility hint; now scale-invariant, computed on the nondimensional Jacobian, but still does NOT gate `Trustworthy()` — it measures a coarser, different question than conditioning), the **scale-invariant** `Conditioning` (`conditioning.go`: the reciprocal condition number of the nondimensionalized Jacobian — this one DOES gate `Trustworthy()`, below a tolerance-derived `max(1e-6, 4·√tol)` threshold), `Trustworthy()`, and (opt-in via `WithProbe`) discrete ambiguity. A pure consumer of the diagnostic building blocks. **The trust verdict has ONE definition and two shapes**: `Check() Reasons` returns one wrapped sentinel per failed condition, in emission order (`ErrBrokenReference`, `ErrForeignHandle`, `ErrVerificationIncomplete`, `ErrUnsolvable`, `ErrNotFullyConstrained`, `ErrConflicting`, `ErrRedundant`, `ErrStaleReference`, `ErrInvalidProfile`, `ErrInvalidParameter`, `ErrNearSingular`, `ErrProbeIncomplete`, `ErrAmbiguous`), and `Trustworthy()` **is** `Check() == nil` rather than a second copy of the condition list — so a condition added to one is added to both and they cannot drift. `Reasons` is `error` + `Unwrap() []error`, so `errors.Is` matches through it AND a caller can waive ONE condition per reason (the reported case: a sketch built from unsigned constraints is ambiguous by design, and its consumer must still enforce everything else). **A new condition goes in `Check`, never in `Trustworthy`**, and it is fatal by default for every waiving caller, since it is not in their waiver list — which is the whole reason a caller must not hand-copy the verdict (a copy silently stops checking what is added next, and cannot reproduce the conditioning gate at all: `condGate` is unexported). **Every reason WRAPS its sentinel** — `fmt.Errorf("%w: …", sentinel)` carrying that condition's own specifics (the residual, the counts, the probe's error) — never the bare sentinel value, which answers `errors.Is` by identity while `errors.Unwrap` returns nil, breaking the `Reasons.Unwrap` contract. **`Check` asserts only conditions `Verify` actually evaluated**: a nil, corrupt or foreign handle stops `Verify` at the reference-integrity scan (it would panic the residual/profile passes), and that skip is recorded on the report, so `Check` reports the integrity reasons plus `ErrVerificationIncomplete` and nothing else — the fields the skipped passes never wrote hold zero values that are NOT verdicts, and reporting them would block a caller waiving `ErrForeignHandle` on failures nobody tested. The verdict still fails on that path in every case, including the one where a nil constraint operand is the only finding, since `ErrVerificationIncomplete` is itself a failed condition. `Check` returns a **literal nil** on the clean path — never a nil `*reasons` inside a non-nil interface, which would make `err != nil` true for a sound sketch (`verify_check_test.go` pins it). |
| `reference.go` | Reference geometry — the sketch/3D separation keystone: read-only, externally-locked 2D snapshots of 3D-derived geometry (`CreateReferencePoint`/`CreateReferenceLine`/`CreateReferenceArc`/`CreateReferenceCircle`) carrying a `source` id + staleness; locked via `fixed[]`, a topology seal (`refSeals`), `RefreshReference`/`RefreshReferenceCircle`/`MarkStale`, and the Verify integrity/staleness/reachability scan. Design in `docs/reference-geometry-design.md`. |
| `probe.go` | `Sketch.ProbeConfigurations`: multi-solution ambiguity probe — deterministic multi-start search (structured mirrors + splitmix64 restarts) for the discrete configurations a DOF-0 sketch admits. A falsifier: ≥2 found proves ambiguity, 1 never proves uniqueness. Design in `docs/ambiguity-probe-design.md`. |
| `plane.go` / `world.go` | 3D world & construction planes. `Plane` (datum = `r3.Frame` derived from a stored definition), `World` (the mandatory document root: owns planes + sketches, datum accessors `XY`/`XZ`/`YZ`, plane builders `CreatePlaneFromFrame`/`CreatePlaneFromPoints`/`CreateOffsetPlane`, `CreateSketch`, `RemovePlane`). Design in `docs/3d-planes-design.md`. |
| `annotate.go` | Annotation-rendering overlay for `Sketch.SVG` (in-package so it can type-switch the unexported constraint types). Opt-in `SVGPNGOption`s, all **default off** so baseline output stays byte-identical: `WithDimensions` (CAD dimension lines + arrowheads + unit label via `dimText`, driven ones parenthesized), `WithConstraints` (geometric-constraint glyph badges, per-anchor slice-order stacking — no `map[Entity]`), `WithDOFColoring` (free = blue+hollow circle, grounded/`IsFixed` = green filled square (`colorFixed`) so the origin anchor reads distinctly, other constrained = black filled circle; points via `movableVars`, entities via `entityMovable` so a circle with a free radius reads blue — the per-entity `Sketch.EntityIsFullyConstrained`), `WithPixelWidth` (display px, viewBox unchanged), `WithConflicts` (conflicting geometry red via `Diagnose` + `constraintRefs`; conflict-red > DOF-blue), `WithStatusBadge` (`Verify` DOF/Status/Solvable card), `WithProfileFill` (valid `Profiles()` regions only, canonical sort for determinism), `WithAnnotationColor`/`WithAnnotationScale`. **Load-bearing rule:** all annotation geometry computes key points in sketch coords, maps through `tx`/`ty`, and derives every screen direction/arrowhead/arc from the mapped points — no per-case y-flip sign negation (only the `<ellipse>` `rotate()` still negates). Design in `docs/constraint-visualization-design.md`. Consumed by `internal/cmd/genimages` (regenerates the committed `docs/images/*.svg` README gallery heroes; an in-sync test byte-compares a regeneration). |
| `frame.go` | Windowed framing for `Sketch.SVG` (opt-in, default off → byte-identical baseline): `WithFrame` (outer padding + border rect; the sketch's `margin` becomes the frame→geometry gap), `WithGrid` (origin-aligned background grid, `niceStep` auto spacing 1/2/5×10ⁿ, emphasized x=0/y=0 axes; implies a frame), `WithGridSpacing`, `WithFramePadding`. A framed render **always** carries the fixed provenance watermark `WatermarkText` (= `github.com/lestrrat-3d/sketch`) in the bottom outer padding — not an option, and no commit hash, so output is fully deterministic and the in-sync test is a plain byte compare (a new commit no longer churns the gallery). `SVG` adds an outer `pad` (shifts `tx`/`ty`, grows the viewBox); grid+frame draw before geometry, watermark on top. |
| `svg.go` / `png.go` / `dxf.go` / `json.go` / `json_world.go` | Exporters / serialization. `png.go` is a stdlib-only rasterizer (`image/png`) so agents/tools that read raster images can sanity-check sketches; visually equivalent to the SVG output (PNG annotation is a follow-up — SVG is the annotated target). `dxf.go` emits length fields in the sketch's **display length unit** (via the `units` library — angles/ratios/knots stay raw) with a matching `$INSUNITS`/`$MEASUREMENT` + `$EXTMIN`/`$EXTMAX` header, so a CAD importer reads the drawing at the right scale (metric output is unchanged). Coordinates are plane-**local** by default; `DXF(WithWorldSpace(true))` places geometry in 3D world coordinates via the plane frame — LINE/SPLINE/ELLIPSE in WCS, CIRCLE/ARC/LWPOLYLINE in the entity OCS (arbitrary-axis algorithm from the plane normal) + extrusion, arc angles recomputed in the OCS. `json_world.go` is the v2 `World`/`Plane` serialization + the `kind`-discriminator preflight. |
| `geom/` | **Self-contained** context-agnostic 2D geometry (own package). |
| `param/` | **Self-contained** parameter & expression engine (own package). |
| `examples/` | Executable Go examples (`Example_sketch_…` in `package examples_test`, `go test`-verified `// Output:` blocks) that double as living documentation. Never `package main` programs. |

### The `geom` package (slated for extraction)

`geom/` holds **transient geometry** — plain `Point`/`Line`/`Circle`/`Arc`
definitions, *coordinates only*, no document state (no construction flag, no
name), no sketch/solver/constraints. It is the engine's `adsk.core` analog: a
pure math layer and the **snapshot type** that a sketch entity hands back from
its `Geometry()` accessor. It is **not** an input you hold and commit — sketch
geometry is authored directly from points (see "Building blocks vs sketch
geometry" below). It must not import `sketch`; the arrow is `sketch -> geom`,
never the reverse. Production code is standard-library-only (tests use
`testify/require`); intended to move to its own module later.

It also carries the **construction toolkit** (`intersect.go`, `modify.go`,
`transform.go`): line/circle/arc intersections (arc cases reduce to circle
cases filtered by `Arc.Contains`), `ClosestPointOnLine`, `SplitLineAt`/
`SplitArcAt`, `Fillet`/`Chamfer` (which replace a shared endpoint with fresh
contact points and return the connecting arc/line), and the `MirrorPoint`/
`TranslatePoint`/`RotatePoint` transforms. These compute on transient geometry;
the *mutating* sketch-level tools in `tools.go` (`Trim`/`Extend`/`Break`/
`CreateFillet`/`CreateChamfer`/`CreateMirror`/`CreatePatternRect`/`CreatePatternCircular`/
`CreateOffset`) feed them an entity's `Geometry()` snapshot, then build the
replacement from sketch points and retire the originals with `RemoveEntity`.

It also holds the **planar-arrangement / region engine** (`region.go`,
`arrange.go`, `area.go`): `geom.Regions(curves, closed)` builds a polyline-
approximated planar arrangement of lines/arcs/circles/ellipses/elliptical-arcs/
splines/closed-splines/fit-splines, splitting at bare crossings, and returns the
bounded
`Region`s (each an
outer boundary loop +
holes, with a net `Area` and source-curve `BoundaryEdge` back-references) plus
soundness signals — a per-`Region` `Degenerate` (an unresolvable condition reaching
THAT region: one involving a curve its own boundary is built from, or one no curve
could be blamed for — attribution is by SOURCE, never by where the condition's
representative point landed, since several of those points are only a midpoint
between two sources; the arrangement-wide `Degenerate` stays the flag a consumer
gates trustworthiness on, since it also covers a condition that produced no region),
`SelfIntersections` (only for a single simple closed loop —
every shared vertex degree 2 — judged on the pruned core, so a branched/
subdivided wire is *not* flagged; a spline is the one source whose *own* polyline
is tested for self-crossings, since a cubic can loop) and `Degenerate`
(collinear-overlap or near-tangent uncertainty). Region area is exact for
**every** curve type: line/arc/circle (shoelace + exact circular-segment
correction), ellipse/elliptical-arc (`chordEllipseCorrection` = ½·rx·ry·(Δφ −
sin Δφ), the elliptical analog, rotation/translation-invariant), and **splines**
(`splineBulge`: the exact ½∫(x·y′−y·x′) of the fragment's piecewise cubic via
3-point Gauss–Legendre per knot span — exact because the integrand is degree-5 —
needing analytic spline derivatives `EvalCubicBSplineDeriv`/
`EvalPeriodicCubicBSplineDeriv`/`fitEvaluator.derivAt`), and **conics**
(`conicBulgeSpan`: the rational-quadratic-Bézier closed form — the moment swept
from `start` minus `triangle(start,P(t0),P(t1))`, i.e. the exact sub-arc/sub-chord
area; whole-curve `conicBulge` is the `[0,1]` case), and **NURBS**
(`nurbsBulgeSpan`/`nurbsMoment`: `splineBulge`'s shape — per-knot-span moment +
chord-closure — with `p`-point Gauss exact for a non-rational degree-`p` curve and
10-point adaptive bisection for the rational/`p>10` case, integrating the true
curve). So the reported `Area` is
sampling-independent **for a whole curve** (and for one split only at an analytic
line/circle/arc crossing); a curve split at a *sampled* crossing
(ellipse/spline/conic vs line, or curve/curve) has an approximate cut parameter,
so its area *converges* with sampling rather than being exact — the correct
topology with a convergent area, never a false bless. **Crossing detection is becoming
analytic** (`arrange_events.go` + the `analyticPrepass` in `arrange.go`; design +
increment plan in `docs/analytic-arrangement-design.md`): the exact closed-form
contact (`Cross`/`Tangent`/`Overlap`) is authoritative **only for a pair whose BOTH
sources are a line, circle or arc** (`analyticKind`; `analyticEvents` returns
`ok=false` otherwise) — and, within that pair set, for line-involved crossings and all
tangencies. The rule is about the PAIR, never about "a line was involved": a line ×
ellipse/conic/spline/NURBS contact — **including a tangency** — has no closed form
here and stays on the sampled path. For an analytic pair the sampled `segParams` is
skipped, cuts land on the exact intersection point (`cut{t,px,py}`, so two sources
merge to one vertex), and the oracle no longer false-flags clean tangencies (tangent
line+circle → one disk; non-merged tangent circles → two disks) or shallow crossings
as `Degenerate`.
**Curve/curve transverse crossings (both circle/arc) are deferred to the sampled
path** (behaviour byte-identical to a no-wiring build): their sampled topology is
already correct, and exact-cut injection there is unsound (coarse equal-count
crossings at wrong locations fuse three regions into one) or over-conservative
(a valid crossing one chord-segment off gets false-flagged) until increment 3's
tangent-port certificate. For a handled curved pair a **three-part consistency
gate** keeps it sound at coarse sampling, and which of the first two applies turns
on whether the contact INSERTS a vertex (`crossNeedsSampledWitness`, answered by the
single `cutSite` the cut phase itself acts on, so the gate can never demand what the
cut phase does not do). A crossing that inserts one on BOTH sources must be
**witnessed** on its own host segment-pair (`analyticCrossHosted`) — that is the
disk-vanishing case, where the injected point sits off the chord by up to the
sagitta. A contact the sampled map ALREADY has a vertex for (a source's own
endpoint, or an interior sample vertex) inserts nothing, so no witness is possible —
a contact at a segment boundary is not interior to that segment — and it is instead
required to be **resolved**: two contacts inside ONE chord of a curved source is a
sub-sample cap (`contactsResolved`). Third, every sampled crossing must be
**explained** by a contact within one curved chord of it (`sampledCrossingsExplained`)
— a leftover crossing is the chord approximation disagreeing with the geometry, and
the face walk has no vertex for it. Failing any, it is conservatively `Degenerate`
rather than a vanished disk. **Demanding a witness where none can exist
is what once false-flagged everyday geometry** — a line ending exactly on a circle
(the gear flank meeting its root circle, and what `NewPointOnCircle` builds), a chord
crossing where the sampling happens to put a vertex, a corner join — so a contact
that inserts no vertex must never be measured against the sampled crossing count. **Exact tangent-port ordering (increment 3, partial):** at a
certified analytic tangency contact (`exactPortVerts`) the DCEL rotation system
orders coincident-tangent half-edges by exact source tangent + signed **curvature**
(`source.differential`/`portKey`/`portLess`, a seam-free half-plane+cross compare)
instead of chord angle, so a **merged-vertex EXTERNAL circle/arc tangency** is now
blessed as two clean disks (opposite curvature separates the loops) rather than
flagged. Used ONLY at those certified contacts — at a sampled crossing vertex the
edges are chords, so chord ordering is what matches the traversed geometry.
**Internal (containment) tangency is now also blessed (increment 7 §7a):** the
shared contact gets the same exact tangent-port ordering, and hole assignment uses
an **exact point-in-region** test (`exactPointInRegion`: a ray-cast with closed-form
circle/arc crossings, immune to the chord poke-out that defeated the sampled
`pointInPolygon` near the contact), so the inner cycle nests into the outer as an
annulus + inner disk — exact at every sampling, tiny inner included. Line-involved
merged tangency, genuine osculation, and curve/curve crossing authority stay
conservatively `Degenerate`/deferred. Ellipse/spline pairs keep the sampled fallback
(exact containment falls back to the chord polygon for them). `Sketch.Profiles()` is
its consumer.

### The `r3` module (an external dependency)

`github.com/lestrrat-3d/r3` is the 3D analog of `geom`: a coordinate-math layer
for Euclidean 3-space with no document state, living in **its own module/repo**
(not in this tree). It holds `Vec` and the orthonormal right-handed `Frame`
(origin + unit axes `U`,`V`; normal `N()` = `U`×`V`, derived not stored). The
local↔world transform lives **only** there (`Frame.ToWorldUV`/`ToWorld`/`ToLocal`,
the inverse being the transpose — never a matrix solve). It imports nothing but
stdlib; the arrow is `sketch -> r3`, never the reverse.

- **`r3`'s scope is coordinates, not shapes.** Vectors, frames and the
  transforms between them belong there; 3D *shapes* (spheres, boxes, surfaces,
  solids) do not — they belong to a geometry layer above, which would import
  `r3`. Don't push shape types down into it to avoid a new package.
- **Frames are ALWAYS orthonormal**, enforced at the boundary: `NewFrame`
  orthonormalizes and returns `ErrDegenerateFrame` on zero/collinear axes; the
  zero value `Frame{}` is invalid (`IsValid` is false) and every public consumer
  of a caller-supplied frame rejects it (`World.CreatePlaneFromFrame`). Don't add
  a path that stores an unvalidated frame.
- `Vec.Normalize` returns `(Vec, bool)` — it never fabricates a unit vector
  from zero. This is **not** the solver's `norm()` floor; don't conflate them.
- A change spanning both repos needs a release of `r3` before this module can
  require it; a local `go.work` (gitignored) is the development seam.

### The world & planes (`plane.go`/`world.go`)

The 2D solver is **untouched**: a `Sketch` still solves in plane-local 2D. A
`Plane` carries an `r3.Frame` *computed from a stored definition* (its
provenance — the single source of truth; `Frame()` recomputes, no memoization).
A `World` is the **mandatory document root**: it owns planes (datums at ids
0/1/2) + sketches + **one shared `param.Table`** (`World.Params()`) and is the
serialization root. **Every sketch belongs to a world** — there is no standalone
sketch/plane constructor anymore. Obtain a sketch with `World.CreateSketch(plane)`
on a plane from `World.XY()`/`XZ()`/`YZ()` or a created plane
(`World.CreatePlaneFromFrame`/`CreatePlaneFromPoints`/`CreateOffsetPlane`). The
**verb convention** is settled: `New*` = package-level standalone constructor (only
`NewWorld` remains at the sketch layer); `Create*` = a World method that
manufactures a new owned object; `Add*` = a method that attaches an existing
object (the sketch's geometry builders). Load-bearing rules:

- **Global parameters are world-shared.** `World.CreateSketch(plane)` seeds the
  new sketch with `s.params = w.params`, so one global parameter drives dimensions
  across sketches. Because every sketch shares the world's (non-nil) table from
  creation, `Bind` a dimension to `s.Params()` (== `w.Params()`); binding to a
  *different* table is `ErrTableMismatch`. **Offset planes are parameter-driven**
  (`World.BindOffsetPlane(p, expr)` → a length expression on `planeDef.distExpr`,
  kind-checked, re-evaluated on every `Frame()` call with NO cache so an edit
  reflows immediately; wrong-kind surfaces through `Frame()`). `World.Verify(ctx)` →
  `WorldVerificationReport` aggregates the shared table, every plane frame, and
  each sketch's report. World docs are **v3** (top-level `parameters` + plane
  `dist_expr`); a legacy v2 world migrates by promoting identical per-sketch
  tables, rejecting conflicting ones.

- **Placement is mandatory but nil-safe.** `Sketch.plane()` defaults a nil
  placement to the owning world's `XY()` datum (zero-value/unmarshal safety net) —
  but a v2 `kind:"sketch"` document with no `plane` is **rejected**
  (`ErrMissingPlane`), not defaulted. A single-sketch document loads as an
  **implicit one-sketch world** owning the inlined plane (`datumPlaneFromJSON`
  maps the inlined datum onto the world's XY/XZ/YZ or a created frame/points
  plane; an offset plane is rejected, as only a world document carries the base it
  would reference).
- **All planes are world-owned** (XY/XZ/YZ datums, `CreatePlaneFromFrame`,
  `CreatePlaneFromPoints`, and derived `CreateOffsetPlane`). Derived planes need an
  owner + id, so they (like all planes now) exist only through a `World`.
- **`RemovePlane` mirrors `RemovePoint`**: refuses standard datums and in-use
  planes (a sketch on it, or another plane's base), else splices + renumbers ids
  densely and **tombstones** the handle (`removed=true`, `owner=nil`, `id=-1`).
  A tombstone is rejected everywhere (`owns` checks `w.planes[p.id]==p`).
- World coordinates are a read-only derived surface (`Point.World`,
  `Sketch.WorldPolyline`), raw base-unit mm like the rest of the read surface.
  `WorldPolyline` samples via the centralized curve samplers in `geom/sample.go`
  (the exporters delegate to the same math; their output is unchanged).

### The `units` module (an external dependency)

`github.com/lestrrat-3d/units` is a standalone units-of-measure library living
in **its own module/repo** (not in this tree): typed `Unit` constants (metric +
imperial length, deg/rad angle — never strings), a `Value` type that pairs a
magnitude with its unit and converts between compatible units, and a `System`
holding the current default length/angle units (`Metric`/`SI`/`Imperial`). Every
unit has a `Kind` — a **comparable dimension-exponent struct** over the base
dimensions length, mass and angle, not an int enum — so kinds compose:
`Kind.Mul`/`Div` (mirrored by `Value.Mul`/`Div`) build compound kinds (`Area`,
`Volume`, `Density`, `MomentOfInertia`, `SecondMomentOfArea`, …) from those
exponents. Every **named** kind — `Dimensionless`, `Length`, `Area`, `Volume`,
`Angle`, `Mass`, `Density`, `MomentOfInertia`, `SecondMomentOfArea` — has a
registered base unit via `BaseUnit(kind)`; millimetre and radian are the bases
for `Length` and `Angle`, the two kinds sketch's own currency (points, solver
vars) is denominated in. `BaseUnit` returns `(Unit, bool)`: an **unnamed** kind
(e.g. a bare `L⁻¹`, curvature) has no base unit, so the `ok` must be handled.
The two **sketch**-package call sites (`json.go`'s `dimUnit`, `parameters.go`'s
`evalDimension`) key off a `Dimension`'s own kind, which is always length or
angle, so `ok` is always true there — but they still return an error rather
than panic on the impossible `false`. `param.Table.EvalValue` (`param/table.go`)
is different: it keys off whatever kind the evaluated expression computes to —
any *named* kind a table parameter was declared with (length, angle, mass,
density, …) — so an unnamed kind is a real, reachable `ok == false` there,
surfaced as `param.ErrIncompatibleKind`. Conversion and `Value` arithmetic are kind-checked and return
`ErrIncompatible` on a mismatch — units are NEVER silently relabelled — and a
`Value` never carries negative zero. New units register via `Define`/`Lookup` (also
the serialization hook); `Define` **panics** on a malformed registration
(duplicate symbol, non-positive/non-finite factor, whitespace/non-ASCII/control
or leading-`[` symbol, overflowed kind) — so it is a build-time authoring call,
never fed user input. Sketch loads units through `Lookup` only, never `Define`,
so those panics are unreachable at runtime. `Value` also round-trips as text
(`MarshalText`/`UnmarshalText`, e.g. `"10 mm"`); sketch does not use it. It
imports nothing but stdlib.

- **All unit conversion lives there** — no other package re-implements factor
  math. Never relabel a magnitude to change its unit; go through
  `Value.Base`/`In`/`Convert`/`FromBase`.
- The dependency arrows are `sketch -> units` and `param -> units`, never the
  reverse — `units` knows nothing of sketches, parameters or documents.
- A change spanning both repos needs a release of `units` before this module can
  require it; a local `go.work` (gitignored) is the development seam.

### The `param` package (slated for extraction)

`param/` is a standalone parameter/expression engine: a `Table` of named
parameters holding literals or expressions (`width = height * 1.5`), with a
lexer/parser/evaluator, functions, constants, forward references and cycle
detection. **It must not import anything from the `sketch` package or rely on
the rest of the repo** — it is intended to move into its own module/repository
later, so the dependency arrow only ever points *into* it. Its production code
depends on the standard library plus the `units` module and nothing else (tests
may use `testify/require`); keep it independently testable.

### Building blocks vs. sketch geometry (load-bearing)

The model follows Fusion's transient-geometry / sketch-entity split.
**Transient geometry** (`geom.Point`/`Line`/…) is pure coordinate math: a
building block for the math layer and the **snapshot** an entity returns from
`Geometry()`. It carries no document state and is never committed. **Sketch
geometry** (`sketch.Point`/`Line`/…) is the durable, solver-bound entity, and
the only handle you hold. You author it directly: `s.CreatePoint(x, y)` returns a
`*Point`; the curve builders `s.CreateLine(p1, p2)`/`CreateCircle(center, r)`/
`CreateArc(c, s, e)`/`CreateEllipse(center, rx, ry, rot)`/`CreateSpline(pts…)` take those
points. **Topology is expressed by sharing a `*Point`** between entities (a
shared corner is literally one point) — there is no generic-pointer identity
map and no idempotency; each `Add…` makes a fresh entity. Constraints reference
**sketch** geometry, so they never reference un-committed geometry —
`AddConstraint` just registers them. To read an entity's current shape as a
transient value, call `Geometry()` (a fresh snapshot at the solved coords);
`geom.NewX` is for math and snapshots, never as sketch input.

### The parameter model (load-bearing)

All scalar unknowns — point `x`/`y`, circle radius, ellipse semi-axes/rotation
— live in one flat vector on the `Sketch` (`vars []float64`, with a parallel
`fixed []bool`). Sketch primitives hold **indices** into that vector (no
geom back-reference). The solver reads/perturbs the vector directly. Grounding
(`fixed`) is per-point on the sketch (`s.Fix`/`Unfix`); construction status is a
settable per-entity property (`entity.SetConstruction`). Any new geometry that
introduces unknowns must allocate them via `newVar` in its `Add…` method and
reference them by index so the solver sees them automatically.

**Author for parametricity: ground, don't pin** (guidance for every consumer of
the engine, and for code the engine itself ships). Anchor a sketch by
**constraining one point to `s.Origin()`** — the engine-provided, always-grounded
point every sketch owns — rather than by calling `s.Fix`; remove the remaining
rotational DOF with **one** orientation constraint (a horizontal/vertical line),
and locate every other point with geometric + dimensional constraints on the
geometry — never by fixing its coordinates. A point the drawing does not otherwise
constrain is made determinate the same way, by one coincidence to the origin,
rather than by pinning it or calling it reference geometry. `s.Fix` remains for
the cases a constraint cannot express, and is still bounded by the rule below. A fixed coordinate is outside the parameter model: it cannot be
`Bind`-driven and will not reflow when a driving dimension changes, so pinning
interior points, non-origin points, or more than the single origin anchor is a
non-parametric anti-pattern (the SolidWorks/Fusion "avoid the Fix/Ground
constraint" rule). Fixing a second full point already over-grounds by one DOF.
The gallery builders `groundedRect`/`hexagon` follow this. (Reference geometry is
the deliberate exception — it is externally locked via `fixed[]` by design; see
`reference.go`.) A corollary: an **unsolvable over-constraint has no satisfying
configuration, so it must distort when solved** — never "fix" that distortion by
pinning points; that fakes the geometry.

A **constraint** may also own auxiliary variables when it genuinely needs them
(the arc-tangency sweep slack, and the arc-length dimension's unwrapped-sweep
variable). It allocates them in an
`allocVars(*Sketch)` method — a hook `AddConstraint` calls (the same shape as
`resolveUnit`), so it runs on initial commit and on load (rebuild goes through
`AddConstraint`). Aux vars are retired on removal via a `retireVars(*Sketch)`
hook and are **not serialized** — they are recomputed from the solved geometry
when `allocVars` re-runs on load. This is the deliberate, narrow exception to
"constraints own no vars"; ship it only with the constraint that needs it.

### Invariants the solver depends on

- **Residuals are unit-normalized.** Length-like residuals are in length units;
  angle/parallel/perpendicular residuals are dimensionless (`sin`/`cos` of the
  angle). This is what keeps the normal equations well-conditioned across mixed
  constraint types — it is the difference between the hexagon example solving
  exactly and getting stuck in a distorted local minimum. **When adding a
  constraint, match this convention** (divide cross/dot products by the relevant
  lengths; use `norm()` which floors away from zero). Do not introduce residuals
  in length² or length⁴.
- **Damping is Levenberg (absolute), scaled by the max diagonal of JᵀJ**, not
  per-element Marquardt scaling. This gives the minimum-norm step for
  rank-deficient / under-constrained sketches. Don't revert to `λ·A[i][i]`.
- **The Jacobian is numerical** (central differences). Simple and robust; see
  the open questions for when this might change.
- **DOF/redundancy analysis recomputes the Jacobian at the call-time
  configuration.** `rank()`/`DOF()` rebuild J via `jacobian` when called — after
  `Solve` that is the *solved* point. NEVER reuse the Solve loop's
  last-iteration Jacobian for rank analysis: it is stale (evaluated one step
  before convergence) and yields wrong DOF/redundancy counts.
- **Driven (reference) dimensions contribute no residuals.** `residuals()`
  skips any `Dimension` with `Driven() == true`, and `refreshDriven()` writes
  the measured value back into the dimension's target after every `Solve`.
  Anything that maps residual rows back to constraints (e.g.
  `RedundantConstraints`) MUST mirror `residuals()`'s iteration exactly —
  including the driven skip — or row↔constraint attribution silently shifts.
- **Goals (`WithGoal`) are transient solver rows, never constraints.** They
  exist only inside `Solve`'s pull phase; `residuals()`, `rank`, `DOF` and
  `RedundantConstraints` never see them. Goal solves are **two-phase** (pull
  on the augmented system, then polish on hard residuals only) because plain
  weighted least squares trades constraints off against an unreachable goal by
  O(w²·pull) — far above tolerance. Don't collapse the phases back into one:
  the polish pass is what makes "constraints win exactly" true.
- **The solver works in base units** (millimetre coordinates, radian angles).
  Dimensions carry a `units.Value`; their residual uses `Target().Base()` to
  reach base units. Unit conversion happens *only* in the `units` library
  (`Value.Base`/`In`/`Convert`/`FromBase`) — never by relabelling a magnitude
  (turning "1 deg" into "1 rad" is a bug). Bare-float constructors interpret
  their number in the sketch's default unit for that kind (`Sketch.Units`).

### Serialization invariants

- Points and entities are referenced by their **id, which always equals their
  current slice position** (`s.points`/`s.ents` — two independent id spaces).
  Removal splices and renumbers the later ids (`removal.go`), so marshalled
  documents stay dense and coherent; `UnmarshalJSON` recreates in order so the
  indices line up. Never let an `id` field and slice position diverge.
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
  serializes from. Ownership is `p.s != s` for a point, NOT `owns`: the origin is
  deliberately absent from `s.points`, and a nil or dead handle is a separate fault the
  reference-integrity scan already reports (the entity half reuses `ownsEntity`, skipping
  a nil operand for the same reason). The refusal is bounded to what `Verify` already
  rejects, so a sketch its report calls trustworthy always marshals.
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
- **Internal constraints** (those implementing `internalConstraint`, e.g. the
  arc radius-consistency constraint auto-added by `CreateArc`) are *not* serialized
  — they're recreated by the constructor on load. New auto-added constraints
  must follow this pattern or round-trips will double them.
- **The `param` table serializes in definition order.** Its JSON preserves the
  order parameters were defined so forward references and reload stay
  reproducible. Keep that order on marshal/unmarshal.

## Conventions

- `gofmt`, `go vet`, and `go test ./...` must all be clean before committing.
- **README code blocks are generated, not hand-written.** They are embedded from
  the compiled `examples/` tests via `<!-- INCLUDE(file[,Func]) -->` markers and
  expanded by `internal/cmd/genreadme` (stdlib-only). After changing any example
  referenced by the README, run `go generate ./...` and commit the regenerated
  `README.md` with the code. Never edit the embedded blocks by hand.
- **Optional settings use functional options**, not options structs. Each option
  group defines a typed marker interface (`SVGOption`, `SolveOption`) embedding
  `option.Interface` plus a private wrapper, `ident…` marker structs, and `With…`
  constructors; the consumer folds them into a private `…Config` struct seeded
  from a `default…Config()`. See `svg.go` / `solver.go`. The typed interface
  keeps each option group distinct (an `SVGOption` can't be passed to `Solve`).
  An option shared by several consumers follows the jwx combined-interface
  pattern: a concatenated-name interface whose concrete type carries every
  relevant marker method (`SVGPNGOption` in `svg.go` satisfies both `SVGOption`
  and `PNGOption`, so one `WithMargin(…)` value flows into either exporter;
  `SolveVerifyOption` in `solver.go` satisfies both `SolveOption` and
  `VerifyOption`, so one `WithTolerance(…)` value flows into either — keeping the
  solver's convergence target and `Verify`'s solvability threshold consistent).
- **An accessor that returns an internal slice returns a `slices.Clone` COPY of
  it** — `Sketch.Points`/`Entities`/`Constraints` and `World.Planes`/`Sketches`.
  The ELEMENTS are the live handles (that is the point: you get the real `*Point`
  / `Entity` / `*Plane` and mutate it through its own API); only the slice is
  copied, so a caller cannot write through a SLOT into engine state. The backing
  slices *are* the id spaces (id == slice position), the entity slice is stamped
  only by the `addEntity` funnel, and the constraint order is the row→constraint
  attribution every diagnostic rests on — a write-through would bypass all three
  at once (`s.Entities()[i] = &sketch.Line{…}` splices in an entity with no uid,
  no vars and no id, blinding `Sketch.Revision` and dangling every `Profile`
  handle). Engine-internal code iterates `s.ents`/`s.points`/`s.cons` directly, so
  the copy is never paid on a hot path. A `Profile`'s `Entities`/`Outer`/`Holes`
  are NOT this case: a profile is a freshly-allocated snapshot the consumer owns.
- **Tests use `testify/require`** (never `assert`) and live in **external
  `xxx_test` packages** — they exercise only the exported API. If a test needs
  to observe internal state, add a documented exported accessor rather than
  reaching into unexported fields (e.g. `Sketch.Points`, `Point.ID`,
  `Point.Geometry`, `DriverExpr`). No named return values, including in tests.
  Author geometry with the real builders (`s.CreatePoint(x,y)`, `s.CreateLine(a,b)`,
  …) directly in tests — do not wrap them in trivial 1:1 helpers; explicit is
  better.
- Geometry is authored against the sketch from points (`s.CreatePoint` then
  `s.CreateLine`/`CreateCircle`/`CreateArc`/`CreateEllipse`/`CreateSpline`); constraints come
  from package-level `New…` functions (the `New` prefix is forced for the
  dimensional ones because their concrete handle types — `Distance`, `Radius`,
  `Angle`, … — already own the bare name; keep all constructors consistent) and
  are registered with `s.AddConstraint`. `geom.NewX` is only for math/snapshots,
  never sketch input.
- Constraints reference **sketch** geometry (`*sketch.Point`/`*sketch.Line`/…),
  not transient `geom` values; the residual reads solved values through it.
  Constraints that relate centers/radii take the sealed `Circular` interface (`*Circle` or
  `*Arc`); an arc's radius is the derived `dist(Start, Center)`, so such
  residuals need no radius variable. **Arc tangency enforces the sweep** (the
  tangent must touch the arc, not just its full circle, or the oracle would
  bless a tangent that misses it). Two cases in `constraint.go`:
  *endpoint tangency* — operands that share the contact point (the fillet/slot
  case, detected by shared `*Point`) — is one clean equality (line ⊥ radius, or
  centers collinear, at the shared point), no aux var. *Interior tangency* pins
  tangency to the full circle **and** adds a slack-encoded inequality
  (`dot(contactDir, midDir) ≥ cos(sweep/2)`) keeping the contact in the sweep.
  The slack is an auxiliary solver variable (see the parameter-model note on the
  `allocVars` hook).
- Public dimensional constructors return concrete handles (`*Distance`, etc.)
  with `.Set`/`.SetValue`; geometric constructors return the `Constraint`
  interface.
- **Public constructors validate input by returning errors, never panicking.**
  The shape/pattern builders (`CreatePolygon`/`CreateSlot`/`CreateSpline`/
  `CreatePatternRect`/`CreatePatternCircular` → `ErrInvalidShape`),
  `World.CreateSketch`/`CreateOffsetPlane` (`ErrForeignPlane`), and the plane/frame
  constructors (`World.CreatePlaneFromFrame`/`CreatePlaneFromPoints`,
  `r3.ErrDegenerateFrame`, `geom.ErrTooFewControlPoints`) all return
  `(…, error)`. **Production code contains no explicit panics.** Even the pure
  spline-family math kernels (`geom.EvalCubicBSpline`/`SampleCubicBSpline`/
  `EvalCubicBSplineDeriv`/`NearestParamCubicBSpline` and the periodic/fit-point
  analogs) return a trailing `error` — the per-family sentinel
  (`ErrTooFewControlPoints`/`ErrTooFewClosedControlPoints`/`ErrTooFewFitPoints`)
  on too-few points — rather than panicking. Their callers inside the engine
  feed construction-guaranteed-valid input (the `Add…`/`New…` constructors
  already enforce the minimums), so those call sites discard the error with `_`;
  the error path is the public contract for direct kernel callers.
- Keep exported API documented with Go doc comments; primitives expose value
  accessors (`X()`, `Y()`, `R()`, …), a `Geometry()` snapshot, and measurement
  queries (`Point.DistanceTo`/`DistanceToLine`, `Line.AngleTo`), while
  index-backed fields stay unexported. Measurement math lives in `geom`
  (`geom/measure.go`); the sketch entities delegate through `Geometry()`.
- New constraints: add the residual, the `New…` constructor, a case in the JSON
  marshal/unmarshal switches, an arg-count entry in `constraintArity`
  (`json.go` — so the decoder validates references before indexing), a case in
  `constraintRefs` (`removal.go` — or the removal cascade silently misses it), a
  case in `condRowKinds` (`conditioning.go` — classify each residual row as
  length/dimensionless for the conditioning gate; an unclassified constraint makes
  the conditioning measure NaN, which fails the trust gate fail-safe — never a
  false pass — but a healthy sketch using it would then read untrustworthy), and a
  test asserting on the solved geometry.

## Open design questions (the "many variables")

These are unsettled. If you resolve one, record the decision here.

- **Parameters & expressions.** *Resolved.* The `param` engine is wired into
  the sketcher: the caller supplies a `param.Table` explicitly at bind time via
  `s.Bind(dim, table, expr)` (the table is required, and all of a sketch's
  dimensions must share one table — `ErrTableMismatch` otherwise). `s.Params()`
  returns whatever table the bindings established (nil if none). Bound
  dimensions are re-evaluated by `ApplyParameters` at the start of every
  `Solve`; a manual `.Set(v)` clears the binding. Parameters and per-dimension
  expressions are serialized in the sketch JSON. The dependency arrow is
  `sketch -> param`, never the reverse. *Possible follow-ups:* parameter units,
  and reporting which parameter a solve failure came from.
- **Geometry coverage.** *Largely resolved.* Splines are in as clamped
  uniform cubic B-splines whose control points are ordinary sketch points (no
  new solver machinery; see `docs/spline-design.md`). A point can be confined to
  a spline with `NewPointOnSpline`: the foot-point parameter `t` is an auxiliary
  solver variable (no implicit `F(x,y)=0` exists for a B-spline, so membership is
  the existential `P = S(t)` — two length rows), bounded to `[0,1]` by a
  slack-encoded box (`t=w0²`, `1−t=w1²`) so out-of-range `t` is infeasible rather
  than silently clamped. The aux vars are not serialized (re-seeded on load by
  foot-point projection). `CheckConstraint` probes any aux-var constraint in its
  committed form — it temporarily allocates the candidate's aux vars, ranks the
  real rows, then rolls back (non-mutating). (A documented limitation: two
  point-on-spline on the same point are redundant only nonlinearly, so the local
  rank analysis is not guaranteed to flag the duplicate; it stays harmless.)
  A line can be made tangent to a spline with `NewTangentToSpline` (same bounded
  contact-parameter `t` machinery): the committed residual is five rows — contact
  `S(t)` on the line's infinite carrier (signed perpendicular distance, length),
  the line direction parallel to the analytic spline tangent `S'(t)`
  (`geom.EvalCubicBSplineDeriv`, dimensionless `sin`), the two box rows, and a
  scale-relative no-cusp guard `|S'(t)|/scale ≥ epsTan` (slack `ws`) so the oracle
  never blesses "tangent" where the tangent direction is undefined; a zero-length
  line is rejected outright. `S'(t)` is analytic on purpose — a numerical tangent
  inside the residual would be a nested finite difference the Jacobian
  re-differentiates.
  **Closed (periodic) splines** are in as a separate `ClosedSpline` entity
  (`CreateClosedSpline`, ≥3 control points) over an exact cyclic uniform cubic basis
  (`geom.EvalPeriodicCubicBSpline`) — a smooth C2 loop that bounds a region on its
  own (a sealed `geom.ClosedCurve`, not a `Curve`), with periodic-ring
  self-crossing detection and `closed_spline` serialization. A point can be
  confined to it with `NewPointOnClosedSpline`: the **periodic witness** — a single
  foot parameter `t` aux variable with NO `[0,1]` box (a loop has no endpoints, so
  `t` is unbounded and `S(t)=S(t+1)`), committed residual just the two length
  membership rows `P−S(t)`. A line can be made tangent to it with
  `NewTangentToClosedSpline` — the same periodic witness (unbounded `t` plus a
  no-cusp slack `ws`, no box), three rows: contact on the line's carrier (length),
  parallel to the analytic periodic tangent `S'(t)`
  (`geom.EvalPeriodicCubicBSplineDeriv`, dimensionless), and the no-cusp guard.
  **Fit-point (interpolating) splines** are in as a separate `FitSpline` entity
  (`CreateFitSpline`, ≥2 fit points) whose curve passes *through* the fit points: the
  fit points are the durable solver handles and a natural-cubic interpolant
  (chord-length parameterization, Thomas tridiagonal solve in
  `geom.EvalFitSpline`/`SampleFitSpline`) is recomputed from their current
  coordinates per evaluation, so the curve keeps interpolating them as the solver
  moves them — no new solver vars. An open `Curve` (endpoints = first/last fit
  point) participating in profiles like the open spline, `fit_spline` serialization.
  A point can be confined to it with `NewPointOnFitSpline` (the bounded foot
  parameter `t∈[0,1]` with a slack box, exactly like `NewPointOnSpline`, since the
  curve has endpoints). A line can be made tangent to it with
  `NewTangentToFitSpline` — the bounded-`t` witness like `NewTangentToSpline` (five
  rows: contact, parallel to the analytic natural-cubic tangent
  `geom.EvalFitSplineDeriv`, two box rows, no-cusp guard). (Both point-on seeds use
  `geom.NearestParamPeriodicCubicBSpline`/`NearestParamFitSpline`; both tangent
  seeds share `seedTangentParam` with the open spline.)
  Ellipses are in
  (center point + rx/ry/rotation vars; `NewPointOnEllipse` uses a
  Sampson-normalized residual — |F|/|∇F| — to stay in length units).
  **Elliptical arcs** are in as a geometry primitive (`CreateEllipticalArc`:
  center + start/end points + rx/ry/rotation vars, two internal on-ellipse
  constraints pinning the endpoints, eccentric-angle sweep, exact-segment area
  in the arrangement). Its shape is dimensionable via the sealed `Elliptical`
  interface (`NewSemiMajor`/`NewSemiMinor`/`NewEllipseRotation` accept a
  `*Ellipse` or an `*EllipticalArc`). A point can be confined to an elliptical
  arc with `NewPointOnEllipticalArc` (on the ellipse via the Sampson residual,
  within the eccentric sweep via a slack inequality, mirroring `pointOnArc`). A
  line can be made tangent to an ellipse or elliptical arc with
  `NewTangentEllipse` (sealed `Elliptical` operand, mirroring `NewTangent` for
  circles): the closed-form condition `√((u·rx)²+(v·ry)²)=|c|` on the line's
  local-frame normal — no foot-point iteration — plus, for an arc, the same
  slack inequality confining the contact to the eccentric sweep (and an
  endpoint-tangency branch when the line shares a boundary point).
  **Conic–conic tangency** (no closed-form distance; design in
  `docs/conic-tangency-design.md`): `NewTangentEllipseCircular(e Elliptical, c
  Circular, …)` and `NewTangentEllipses(e1, e2 Elliptical, …)` over the sealed
  interfaces, so each operand is a circle, **arc**, ellipse, or **elliptical
  arc**. A contact-point witness (aux coords) on both curves with parallel
  outward normals (`cross(n̂_A,n̂_B)=0`), a **hard** internal/external branch row
  `σ·dot(n̂_A,n̂_B)−wSide²` (the flag must be an enforced equation, not a seed, or
  the oracle could not tell the branches apart), degenerate-conic guards, and —
  per arc operand — a slack-encoded **sweep row** confining the contact to the
  swept portion (so a tangent to the underlying full conic off the arc is
  rejected). When two **arc** operands share an exact endpoint `*Point` the
  **shared-endpoint branch** enforces tangency *at* that point — `parallel` +
  internal/external branch rows there, no free witness and no membership/sweep
  rows (an endpoint is already on both curves and in-sweep by definition).
  **Conics** are in as a geometry primitive (`CreateConic(start, apex, end, rho)` —
  a rational quadratic Bézier; `rho ∈ (0,1)` sets fullness, `ρ<0.5` ellipse /
  `ρ=0.5` parabola / `ρ>0.5` hyperbola arc, apex weight `w=ρ/(1−ρ)`). An open
  `Curve` like the elliptical arc: authorable, profile-participating (exact
  whole-curve area via `conicBulgeSpan`), serializable (`"conic"`), exportable
  (native degree-2 rational `SPLINE` with weights `1,w,1`, incl. world-space).
  `rho` is a single free solver var (a free conic is DOF 7). A point can be
  confined to it (`NewPointOnConic` — bounded foot parameter `t∈[0,1]` witness like
  `NewPointOnSpline`) and a line made tangent (`NewTangentToConic` — the five-row
  bounded-`t` witness like `NewTangentToSpline`, using the analytic
  `geom.Conic.EvalDeriv`). *Follow-ups:* a rho dimension, analytic line/conic &
  conic/conic intersections.
  **NURBS** are in as a geometry primitive (`CreateNURBS(degree, control, weights,
  knots)` — a general non-uniform **rational** B-spline of arbitrary degree over a
  clamped/open knot vector; design in `docs/nurbs-design.md`). Control points are
  ordinary sketch points (the durable handles); **knots, weights and degree are
  stored structural data, NOT solver vars** (a free NURBS is DOF `2(n+1)`, mirroring
  `Spline` — promoting weights to dimensionable vars is a follow-up). An open
  `Curve` (clamped endpoints = first/last control point) that participates in
  profiles, with the de-Boor kernel (`geom/nurbs.go`:
  `findSpan`/`basisFuns`/`dersBasisFuns`/`Eval`/`EvalDeriv`, general-degree, new and
  separate from the uniform-cubic `EvalCubicBSpline`), exact/numerically-exact area
  (see the area note), spline-family **self-crossing detection** (a high-degree NURBS
  can loop, unlike the convex-hull-bounded conic), `"nurbs"` serialization, and
  **native DXF `SPLINE`** export (degree + knots + rational weights, incl.
  world-space). It does **not** replace the uniform-cubic `Spline` (the ergonomic
  common case). A point can be confined to it (`NewPointOnNURBS`) and a line made
  tangent (`NewTangentToNURBS`) — the same bounded-`t∈[0,1]` witnesses as the
  spline, working in normalized `t` mapped to the knot domain via `NURBS.Domain()`,
  with the analytic `geom.NURBS.EvalDeriv`. *Follow-ups:* knot-insertion/refinement
  tools, periodic/unclamped NURBS, weight dimensions, analytic NURBS intersections,
  and a possible later re-expression of `Spline` on the NURBS kernel.
  Slots/fillet/chamfer exist as compound builders and `geom` template helpers.
- **Solver evolution.** Numerical Jacobian is fine at current scale. **The
  rank/DOF/redundancy/free-point analysis is scale- and unit-invariant**: all
  three dependency mechanisms — `rankAnalysisOf` (DOF/CheckConstraint),
  `conflictAnalysis` (redundancy/conflict attribution), and `movableVars`
  (FreePoints) — run on the **same physically nondimensional Jacobian** `A =
  Drow·J·Dcol` (the `scaledJacobian` builder shared with the conditioning gate),
  with one structural cutoff `rankZeroTol = 1e-9` applied to `A`. Linear dependency
  is exactly column-scale invariant, so the bug was only the numerical threshold
  meeting raw mixed-unit magnitudes; nondimensionalizing fixes it. The structural
  cutoff is DISTINCT from the conditioning trust gate — "structurally
  rank-deficient" (a true null direction) and "full-rank but numerically fragile"
  (a tiny but nonzero singular value) are different questions, so a DOF-0 sketch
  can still be untrustworthy by conditioning. Two near-singularity signals build on
  this. (1) An **advisory** `RankMargin` (`rankAnalysis.margin`): the multiplicative
  distance of the structural rank decision from `rankZeroTol`, now scale-invariant
  (computed on `A`). It does **NOT** gate `Trustworthy()` — it measures the
  STRUCTURAL rank-decision margin (could DOF flip), a coarser, different question
  than the gate. (2) The **scale-invariant conditioning gate** (`conditioning.go`) — the
  measure that DOES gate `Trustworthy()`. It builds a physically nondimensional
  Jacobian `A = Drow·J·Dcol` (length rows ×1/L, length columns ×L, with L the
  bounding-box diagonal and every other row/column ×1) and reports
  `Conditioning = σ_min(A)/σ_max(A)` via a one-sided Jacobi SVD (never `AᵀA`,
  which squares the condition number into fp noise). It is unit- and
  scale-invariant (same value at 1×/1000×/inch, centred for the FD pass so it is
  also translation-invariant), so a dimensionless threshold is a sound pass/fail
  gate: a DOF-0 sketch whose constraint set is near-dependent (e.g. a point pinned
  by two ≈parallel lines) reads untrustworthy. The threshold is **tolerance-derived**
  — `max(1e-6, 4·√tolerance)` — because a slack-encoded inequality at its active
  boundary only resolves its slack to `≈√tolerance` (column norm `2w ≤ 2·√tol`
  bounds `σ_min`), so the gate must sit above that floor or a slack flat-spot slips
  through; a looser `WithTolerance` raises the gate in step. Computed only
  for a DOF-0 candidate (an under-constrained sketch is genuinely singular by its
  free DOF, a separate verdict → `Conditioning` left +Inf). Scaling is by
  *physical kind*, NOT data-dependent column/row equilibration: equilibration
  would hide a near-zero slack/aux column (a real near-rank-loss). Row kinds come
  from a centralized `condRowKinds` table mirroring each constraint's `residual()`
  rows; the only length-kind aux variables are the conic-tangency contact-witness
  coordinates. Design in `docs/conditioning-gate-design.md`. The rank/DOF analysis
  now runs on the same nondimensional `A` (see the scale-invariance note above), so
  DOF/redundancy/free-points are scale-invariant too. Still open: analytic
  Jacobians for speed/accuracy; equation decomposition (solve independent
  constraint clusters separately); a per-sketch solve tolerance (the solver's
  absolute tolerance — not the rank analysis — is what breaks down at extreme
  geometry scales ≳1e6); and better over-constrained diagnostics (identify *which*
  constraints conflict, not just a count).
- **Constraint diagnostics & UX.** *Largely resolved* (`diagnose.go`; design
  in `docs/diagnostics-design.md`). `Sketch.RedundantConstraints()` identifies
  dependent constraints (creation order decides: of two duplicates the later
  one is reported; the row→constraint mapping mirrors `residuals()`).
  `Sketch.Diagnose()` partitions them into redundant (dependent, satisfied)
  vs conflicting (dependent, violated — residual > 1e-8 at the call-time
  configuration). `Sketch.CheckConstraint(c)` rank-probes a candidate without
  committing it and returns `ErrOverconstrained` if any of its equations is
  dependent — the engine half of Fusion's "refuse the over-constraining
  gesture". `Sketch.FreePoints()`/`Point.IsFullyConstrained()` attribute the
  remaining DOF to points via the Jacobian null space (the blue/black
  coloring answer). `Sketch.ProbeConfigurations(ctx)` (`probe.go`; design in
  `docs/ambiguity-probe-design.md`) covers the discrete side DOF analysis
  cannot see: a deterministic multi-start probe that searches for the multiple
  configurations a fully-constrained sketch may admit (mirror flips, tangent
  side choices). It is a falsifier — finding ≥2 configurations proves
  ambiguity; finding 1 never certifies uniqueness. `Sketch.Verify(ctx)`
  (`verify.go`) aggregates all of the above into one non-mutating
  `VerificationReport` (solvability, DOF, `Status`, redundant constraints,
  conflict sets, free points, profiles, opt-in ambiguity via `WithProbe`) for
  the headless-oracle use case. Conflict sets are reported via `ConflictSet`
  (`diagnose.go`): for each conflicting constraint, the earlier *independent*
  constraints whose Jacobian rows linearly combine to reproduce the violated
  row — a true set, not just the later duplicate. `RedundantConstraints`,
  `Diagnose` and `Verify` share one `conflictAnalysis` pass so the partition and
  attribution never diverge. Per-entity constrained status is in
  (`Sketch.EntityIsFullyConstrained` — points + intrinsic shape vars). Still open:
  an `AddConstraint` option that auto-rejects, probe-level tolerance/budget
  options, and folding the ellipse rx/ry-swap symmetry into the probe's duplicate
  metric.
- **Higher-level interfaces.** A text DSL + CLI, and eventually an interactive
  GUI (e.g. Ebiten), are anticipated layers. They should consume the public API
  only.
- **Constraint/dimension visualization.** *Largely resolved* (`annotate.go`;
  design in `docs/constraint-visualization-design.md`). Annotated SVG renders
  dimensions, geometric-constraint glyphs, DOF coloring, conflict highlighting, a
  status badge and profile fill (all opt-in, default off); `internal/cmd/genimages`
  regenerates the README gallery heroes with an in-sync test. The renderer is
  **internal** (type-switches the unexported constraint types). A **constraint-level
  public introspection API** is in (`introspect.go`: `ConstraintKind`/`ConstraintRefs`/
  `ConstraintResiduals`/`IsInternal`) plus optional entity/constraint **names** with
  first-match lookup (`names.go`), giving a DSL/GUI/tests a durable, type-free handle
  on constraints. *Open follow-ups:* the richer **annotation-descriptor API**
  (`Sketch.Annotations()` returning typed exported descriptors — kind + referenced
  entity handles + dimension value/unit/driven — extracted from this renderer's data
  model, so a consumer gets the rendering-level data, not just the constraint graph;
  the north-star "programmability over UI" endpoint), an
  **ambiguity/probe overlay** and a **modification-tools before/after** hero (both
  need a shared-bounds multi-state compositor genimages does not yet have — the
  current heroes are single-state), **rich PNG annotation text** (the rasterizer
  has no font), **global dimension-layout / collision avoidance**, and
  path-outlined glyphs for maximal SVG font portability.
- **Units.** *Resolved (units).* The `units` module provides typed units, a
  unit-carrying `Value`, and a default-units `System`. Sketch dimensions and
  `param` parameters both carry units; the solver stays in base units and all
  conversion is delegated to the library. **Expression kind algebra is in**
  (`param/kind.go`): `param` tracks unit *kind* (whatever kind a parameter's
  declared unit carries — length, angle, mass, …, or dimensionless) through
  expression arithmetic via a static `kindOf` walk — an identifier's kind
  is its declared unit's kind — and rejects incompatible combinations
  (`length+angle`, `length*length` (rejected as a compound kind), `1/length`
  inverse, `sqrt`/trig of a dimensioned value, …) with `param.ErrIncompatibleKind`.
  Addition allows angle/dimensionless mixing (radians are physically
  dimensionless, so `theta + pi/2` is an angle; a length never mixes with a bare
  number), and a parameter's declared unit kind is checked against its
  expression's kind (an angle expression cannot masquerade as a length parameter).
  `Table.EvalValue` returns the kind-carrying value; `Sketch.evalDimension` uses it
  to reject a compound expression that mixes kinds or whose kind ≠ the dimension's
  (not just a direct single-parameter reference); `Verify(ctx)` runs a non-mutating
  parameter-validation pass exposing `ParametersValid`/`ParameterErrors`, which
  gate `Trustworthy()` — so a unit-kind bug hidden in an expression is no longer
  silently blessed. *Limited on purpose:* this is **kind** algebra, not full
  **dimensional** algebra. The `units` module can now represent compound kinds
  (`Area`, `Volume`, …), but `param` deliberately does **not** compose them: a
  dimensioned product/quotient (`length*length`, `1/length`, `length+angle`) is
  *rejected* rather than represented. Parameters exist to drive dimensions
  (lengths / angles) and plane offsets; a compound kind has no consumer in
  sketch, so it stays outside the expression algebra. Revisit only if such a
  consumer (e.g. an area-dimension constraint) is ever added. Custom `SetFunc`
  functions are dimensionless-only (typed custom functions are a follow-up). **The DXF
  exporter honours the display `System`** (`dxf.go`: length fields in the
  display length unit + `$INSUNITS`/`$MEASUREMENT`); SVG/PNG stay unitless raster/
  vector renders by design. *Open follow-ups:*
  should points/coordinates expose unit-carrying accessors. *Note:* the entire read surface
  — coordinate accessors and the measurement queries (`DistanceTo`/`AngleTo`/…)
  — currently returns raw base-unit `float64` (mm/radians), matching the
  solver's currency. Making reads unit-carrying is the deferred all-or-nothing
  decision above; it should be done across the whole surface, not piecemeal.
- **Entity/constraint removal.** *Resolved.*
  `RemoveConstraint`/`RemoveEntity`/`RemovePoint` (`removal.go`; design in
  `docs/removal-design.md`): splice + id renumbering, entity-owned vars
  retired (marked fixed, never reclaimed — reload compacts), constraint
  cascade via the `constraintRefs` switch (includes internal `arcRadius`),
  points kept on entity removal, `RemovePoint` refuses while an entity uses
  the point. Removed handles are dead. This unblocked the mutating sketch
  tools (trim/extend/break/fillet/chamfer/mirror/pattern/offset of committed
  geometry), now built in `tools.go` (design in
  `docs/modification-tools-design.md`).
- **Tolerances.** Still a fixed solver tolerance. Per-sketch
  tolerance/precision remains open.
- **Persistence stability.** *Partially resolved:* documents carry
  `"version": 1`; legacy (unversioned) documents load, newer-versioned ones
  are rejected. Still open: an actual migration story when version 2 arrives,
  and schema compatibility guarantees.
- **2D → 3D.** *Partially resolved* (`plane.go`/`world.go`/the `r3` module; design in
  `docs/3d-planes-design.md`). 2D sketches now live on construction planes inside
  a 3D `World`, with a bidirectional local↔world transform (`Point.World`,
  `Sketch.WorldPolyline`). The 2D solver is unchanged — 3D is a placement layer.
  The **sketch/3D separation keystone is in place**: reference geometry
  (`reference.go`, design in `docs/reference-geometry-design.md`) holds frozen
  snapshots of 3D-derived geometry (projected edges, pierced vertices) — locked,
  with a source id + staleness — so this layer verifies *against* given 3D
  geometry and never *computes* it. Still **out of scope** (above this layer):
  surfaces (NURBS/analytic), free 3D-sketch geometry (points with a `z` var),
  cross-sketch/cross-plane constraints (the `planeDef` recompute is the seam),
  3D rendering, and the projection/intersection algorithms that *produce* the
  reference snapshots. Profiles feeding extrude/revolve remain a future consumer
  of `Sketch.WorldPolyline`.

## Status

Core engine + constraint set + solver (with DOF/redundancy analysis) +
SVG/DXF/JSON export + sketch-modification tools (`tools.go`:
trim/extend/break/fillet/chamfer/mirror/pattern/offset on committed geometry) +
3D world & construction planes (the `r3` module, `plane.go`, `world.go`: 2D
sketches placed on planes in a 3D world, local↔world transform, v2
serialization) +
unified verification (`verify.go`: `Sketch.Verify` aggregating solvability,
DOF/status, conflict sets, free points, profiles + profile validity, opt-in
ambiguity) +
reference geometry (`reference.go`: locked, externally-sourced 2D snapshots with
provenance + staleness — the sketch/3D separation keystone) +
the profile/region engine (`geom/arrange.go` + `profiles.go`: planar
arrangement of sketch geometry into closed regions with bare-crossing
subdivision, holes/nesting, net area, and self-intersection/degeneracy validity
gating `Trustworthy()`) are implemented and tested.
