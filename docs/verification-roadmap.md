# Sketch Verification — Roadmap & Sketch/3D Separation Contract

Supplemental design/roadmap doc. It frames the project's north-star *use case*
and the architectural contract that keeps it a sketch-only library. It
complements `docs/fusion-gap-analysis.md` (the feature-by-feature Fusion-parity
tracker) — this doc is goal-oriented and prioritized; the gap-analysis is the
exhaustive checklist many items here reference.

## Goal: a headless sketch verification oracle

A coding agent authors a parametric 2D sketch (the kind drawn in the sketch
environment of CAD software such as Autodesk Fusion) and uses this library to
**verify the sketch is correct before a human executes the equivalent work in
the real application**. To serve that, the library must:

1. **Faithfully represent** any sketch a Fusion user/agent could create
   (geometry, constraints, dimensions, parameters, units, placement).
2. **Report the signals an agent needs to trust it** — solvability,
   under/fully/over-constrained status, *which* constraints conflict, remaining
   free DOF (and the entities holding them), discrete ambiguity, closed
   profiles, and parameter/unit validity.

**An oracle must never emit a false "valid."** A missing feature (cannot
represent X) is recoverable; a *false positive* (blesses an invalid sketch) is
not. Soundness gaps therefore outrank coverage gaps in priority.

## The separation contract (load-bearing)

The library is **sketches only**; a 3D-bodies layer is planned *on top*. That
split stays clean **if and only if** this library only ever **verifies against**
3D-derived geometry it is *given*, and **never computes** it:

- **Accept** 3D-derived inputs as snapshots/contracts: a frozen plane frame, a
  projected edge handed in as a read-only 2D curve with a source id.
- **Never derive** 2D geometry from a solid (project/intersect, recompute a face
  frame as a body changes). That needs a B-rep kernel and belongs in the layer
  above.

**Verdict: feasible — with one caveat.** The split holds as long as the missing
primitive below is added. The leak points are all *associativity to 3D*, and
each is clean when this layer holds the *result*, not the *computation*:

| Fusion feature | Clean when… | Breaks the split when… |
|---|---|---|
| Sketch on datum/offset/3-point plane | already supported (`World`, `Plane`) | never |
| Sketch on a body **face** | the face frame is passed as a `Plane` (frozen) | this layer must track the face as the body changes |
| **Project / include / intersect** 3D edges | the resulting 2D curves are passed as reference geometry | this layer must compute the projection from a solid |
| Pierce constraint | reduces to coincidence with a supplied reference point | live associativity to a 3D curve is required |
| Plane through edge / tangent to face | the resolved frame is supplied | the frame must be recomputed from B-rep |

**The primitive that keeps the contract clean — now present: first-class
reference geometry** (`reference.go`, design in
`docs/reference-geometry-design.md`) — read-only 2D entities (points/curves)
whose position is **externally locked**, carrying a **source id** and a
**staleness** flag. The per-entity *construction* flag is **not** this:
construction geometry is solver-driven; reference geometry is externally locked.
"Sketch on a face" and "projected edges" are now *"you give us the snapshot"* —
correct layering; auto-recompute-on-body-change is the 3D layer re-feeding
snapshots via `RefreshReference`/`MarkStale`.

**Minimal 3D concepts that must live at or below this layer:** orthonormal
frames/planes (present — `space.Frame`, `Plane`, `World`), reference entities
with provenance (**present — the keystone**), and component/world transforms.
Solid faces, edge topology, NURBS surfaces, and projection/intersection
algorithms stay above.

## Capabilities today (the oracle baseline)

- **Geometry:** point, line, circle, circular arc, full ellipse, control-point
  cubic B-spline; per-entity construction flag.
- **Constraints:** coincident, horizontal/vertical, parallel, perpendicular,
  collinear, point-on-line/circle/ellipse, midpoint, point-symmetric,
  concentric, equal-length, equal-radius, tangent (line/arc, arc/arc) with
  **arc-sweep enforcement** — the contact must lie within the arc's sweep
  (interior tangency via a slack-encoded inequality, shared-endpoint tangency via
  a perpendicular/collinear equality), so a tangent that touches only the full
  circle (not the arc) is reported unsolvable rather than falsely blessed.
- **Dimensions:** distance (point/point, H/V, point-line, line-line), offset,
  radius, diameter, angle, ellipse axes/rotation; driven (reference) dimensions;
  parameter-bound dimensions (`param` table + expressions).
- **Diagnostics:** LM solve with DOF/redundancy rank analysis; `Diagnose`
  (redundant vs conflicting) with `ConflictSet` attribution (the earlier
  constraints a violated one fights); `CheckConstraint` (add-time
  over-constraint rejection); `FreePoints`/`Point.IsFullyConstrained`;
  `ProbeConfigurations` (discrete-ambiguity falsifier); **`Verify`** — one
  non-mutating call aggregating all of these into a `VerificationReport`
  (solvability, DOF, `Status`, redundant constraints, conflict sets, free
  points, profiles, opt-in ambiguity via `WithProbe`), the agent-facing oracle
  entry point.
- **Profiles/regions:** a planar arrangement of all non-construction geometry
  into closed regions — bare-crossing subdivision, holes/nesting, net area,
  winding/orientation, and self-intersection/degeneracy validity that gates the
  oracle verdict (construction excluded; reference geometry included).
- **Placement & I/O:** `World`/`Plane` 3D placement with local↔world readout;
  JSON v2 round-trip (sketch + world); SVG/PNG/DXF export; units system.
- **Reference geometry:** the separation keystone — read-only, externally-locked
  2D snapshots of 3D-derived geometry (`AddReferencePoint`/`Line`/`Arc`/`Circle`)
  carrying a source id + staleness, verified *against* (pierce/coincidence,
  projected-edge profiles) but never computed. `Verify` reports stale/broken/
  foreign references and a `Trustworthy()` verdict that refuses an out-of-date
  snapshot. Design in `docs/reference-geometry-design.md`.

## Roadmap (prioritized for the verification goal)

### Tier 1 — highest leverage

*All Tier-1 items are shipped* (unified `VerificationReport`, arc-sweep tangency
soundness, and reference geometry — the separation keystone). The frontier is now
Tier-2 representation fidelity.

### Tier 2 — representation fidelity

| Item | Why it matters | Effort |
|---|---|---|
| ~~**Profile/region engine**~~ — *shipped* | `Profiles()` now runs a planar arrangement (`geom.Regions`): bare-crossing subdivision, nested loops/holes + containment, winding/orientation, net area, and **self-intersection detection** (a malformed region reports `Valid=false` and gates `Verify().Trustworthy()`), plus a degeneracy (coincident-edge / near-tangent) uncertainty signal. The oracle no longer blesses a self-intersecting or unresolvable profile. **Splines now participate** (`geom.Spline` is a `Curve`: sampled to a polyline like arcs/ellipses, sampled fragment area, and a spline-only same-source crossing test so a self-crossing cubic is flagged `SelfIntersecting` even when the crossing lands on a sample vertex). Open follow-ups: exact ellipse-fragment area, an analytic (non-sampled) arrangement. | XL |
| ~~**Constraint/dimension parity**~~ — *shipped* | Shipped: H/V between points, generalized midpoint, radius/diameter & concentric on arcs (batch 1); entity Fix/ground (`FixEntity`), symmetric lines & circles (batch 2); arc-length dimension (`NewArcLength`, continuous-sweep aux variable — batch 3); point-on-arc (`NewPointOnArc`, sweep-confined — batch 4); **driven (reference) arc-length** (`ArcLength.SetDriven` — measures `R·Sweep()` with no aux var, toggling the unwrapped-sweep variable in/out); **line↔arc Equal** (`NewEqualLineArc` — equates a line's length to an arc's swept length `R·Sweep()`; one length row, no aux variable — `Sweep()` is canonical in `(0,2π]`, so an over-length line is soundly rejected rather than matched by a spurious multi-turn parameter); **distance to a circular edge** (`NewDistancePointCircle` = signed radial gap `|P−C|−r`; `NewDistanceLineCircle` = tangent gap `dist(center,line)−r`, 0 = tangency) — the "tangent-edge distance" item, full circles; **distance to an arc edge** (`NewDistancePointArc`/`NewDistanceLineArc` — the same signed carrier-gap residual plus a slack-encoded sweep row confining the radial/near-side contact to the arc's sweep, so a gap whose nearest carrier contact falls off the swept portion is reported unsolvable rather than silently measured to an endpoint; the aux slack is dropped in driven/reference mode, toggled in/out by `SetDriven` like `ArcLength`); **arc symmetry** (`NewSymmetricArcs` — mirrors arc a2 onto a1 across an axis with the endpoints SWAPPED so the reflected arc still sweeps CCW and matches a1's `Sweep()`; to avoid the 1-redundancy a full second point-mirror would have against the arcs' intrinsic radius constraints, the far endpoint is pinned onto the reflected radial line + a slack-encoded same-ray branch row, so the constraint adds no spurious redundancy). **Ordinate/baseline/chained dimensions are N/A** — they are authoring patterns over the signed `HorizontalDistance`/`VerticalDistance` dimensions sharing a datum (baseline/ordinate, x and/or y) or measured end-to-end (chained); they add no solver rows, DOF, or conflict model of their own, and the oracle already reports their solvability/DOF/redundancy/conflicts per member dimension (a chain + a consistent baseline over the same span is reported *redundant*; a contradictory one *conflicting*, blamed against the chain — see `ordinate_test.go`). A dedicated API would only be GUI sugar and would worsen diagnostics by moving blame from the offending dimension to an artificial group; for read-only callouts mark the member dimension `SetDriven(true)`. (Angle-quadrant selection is likewise N/A — the signed `Angle` dimension subsumes Fusion's unsigned quadrant choice: the target directly sets the directed angle.) | S–L |
| **Curve parity** *(in progress)* | Shipped: the **elliptical arc** geometry primitive (`AddEllipticalArc` — authorable, solvable, profile-participating, round-tripping, exportable) plus its **shape dimensions** (`NewSemiMajor`/`NewSemiMinor`/`NewEllipseRotation` widened to a sealed `Elliptical` interface) **sweep-confined point-on** (`NewPointOnEllipticalArc`), and **line tangency to an ellipse / elliptical arc** (`NewTangentEllipse` — closed-form local-frame tangent condition, sweep-confined contact for arcs, endpoint-tangency branch), and **point-on-spline** (`NewPointOnSpline` — existential `P=S(t)` with the foot parameter as a bounded aux variable via a slack-encoded `[0,1]` box, robust foot-point re-seeding on load; `CheckConstraint` probes aux-var constraints in committed form by temporarily allocating their vars), and **tangent-to-spline** (`NewTangentToSpline` — same bounded contact-`t` machinery; contact-on-carrier-line + parallel-to-analytic-`S'(t)` rows + a scale-relative no-cusp guard), and **conic–conic tangency** (`NewTangentEllipseCircular`/`NewTangentEllipses` over the sealed `Circular`/`Elliptical` interfaces — contact-point witness, parallel-normals row, hard internal/external branch slack, degenerate guards, and per-arc-operand slack-encoded sweep confinement; no closed-form distance), with the **shared-endpoint branch** (two arc operands sharing an exact endpoint `*Point` — tangency enforced at that point via two rows, no witness, no membership/sweep rows), and **splines in profiles** (a spline now participates in the `geom.Regions` planar arrangement, so a spline-bounded region is a reported, area-bearing, validity-checked profile — see the Profile/region engine row). Remaining: fit-point/closed splines, conic/NURBS import representation, sketch text outlines. (Explicit sketch-point entities are effectively N/A — `*Point` is already first-class: id-bearing, serialized, named, construction/reference-flagged, dimensionable, SVG/PNG-rendered; only a DXF `POINT` emission is missing.) | M–XL |

### Tier 3 — important, second wave

| Item | Why it matters | Effort |
|---|---|---|
| **Importers / round-trip fidelity** | Verify an *existing* sketch, not only one authored in this API (see the workflow question below). DXF/SVG recover geometry only — constraints need a Fusion-export→JSON bridge. | L–XL |
| **Unit-aware expression algebra** | `param` evaluates base-unit magnitudes; a kind error hidden inside a compound expression (`width + angle`) is not caught. | L |
| **World/global parameters & parameter-driven planes** | Real models share user parameters across sketches and drive construction planes. `World` keeps params per-sketch today. | M |
| **Solver robustness & export fidelity** | Analytic Jacobian rows / decomposition / conditioning reports for large agent-generated sketches; world-space DXF; display-unit metadata on exports. | L–M |

## Workflow: how a sketch enters the oracle (resolved)

Sketches are **authored directly in this Go API** — the tool prototypes the
parametric-sketch approach before a human builds the equivalent Fusion add-in. So
no importer is needed and the roadmap above stands as written: **importers stay
Tier 3** (a Fusion-export→our-JSON bridge would only be a must-have if sketches
*entered* from Fusion scripts, which they do not).

## Relationship to other docs

- `docs/fusion-gap-analysis.md` — exhaustive feature-by-feature Fusion-parity
  tracker; the source of truth for which individual items are done/open.
- `docs/3d-planes-design.md` — the `World`/`Plane` placement layer this
  separation contract builds on (and where the reference-geometry seam attaches).
- `docs/diagnostics-design.md`, `docs/ambiguity-probe-design.md` — the diagnostic
  building blocks a `VerificationReport` aggregates.
