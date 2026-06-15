# sketch

A standalone, fully programmable **parametric 2D sketch engine** for Go, in the
spirit of the sketch environment in Autodesk Fusion.

You build geometry — points, lines, circles, arcs, ellipses, splines — in code,
tie it together with geometric and dimensional **constraints**, and a numerical
solver moves the geometry so that every constraint is satisfied at once. Because
dimensions are ordinary editable values, sketches are fully parametric: change a
dimension, re-solve, and the geometry updates.

## Why this exists

This engine is built to be **driven by an AI agent as a verification step
before it acts on real CAD software**. Constraint sketching is easy to get
subtly wrong — an under- or over-constrained profile, an ambiguous
configuration that can flip, a dimension that doesn't resolve the way you
intended. Rather than discover that inside Fusion (or another CAD tool) *after*
committing to an operation, an agent can reproduce the sketch here first and
check it programmatically: Does it fully constrain (`DOF == 0`)? Are any
constraints redundant or conflicting? Does it admit more than one valid
configuration? Does the solved geometry actually match the intended dimensions?
Only once the sketch is proven sound does the agent carry the plan into the CAD
package — and the SVG/PNG exporters let an agent or human eyeball the result
along the way.

* Pure Go. The production runtime depends only on the standard library plus
  `github.com/lestrrat-go/option/v3`; the `geom`, `param` and `units`
  subpackages are standard-library-only and independently extractable.
* Levenberg–Marquardt geometric constraint solver with degrees-of-freedom and
  redundancy analysis.
* A rich, Fusion-like constraint set, plus sketch-modification tools
  (trim/extend/break, fillet/chamfer, mirror, rectangular/circular patterns,
  offset).
* Verification diagnostics: redundant/conflicting constraint detection,
  free-DOF attribution, over-constraint rejection, and a multi-solution
  ambiguity probe.
* Units of measure and parameter/expression-driven dimensions.
* Profile (closed-region) detection.
* Export to **SVG** and **PNG** (visual inspection), **DXF** R12 (CAD
  interchange) and **JSON** (lossless save / load round-trip).

```go
import "github.com/lestrrat-3d/sketch"
```

## Quick start

You author geometry directly on the sketch from points: `s.AddPoint(x, y)`
returns a solver-bound `*sketch.Point`, and the curve builders
(`s.AddLine`/`AddCircle`/`AddArc`/`AddEllipse`/`AddSpline`) take those points.
Topology is expressed by sharing a point — the corner where two lines meet is
literally one `*Point`. Constrain the geometry, solve, edit a dimension, and
re-solve.

<!-- INCLUDE(examples/sketch_readme_example_test.go) -->
```go
package examples_test

import (
  "fmt"

  "github.com/lestrrat-3d/sketch"
)

// Example_sketch_quickstart builds an axis-aligned rectangle entirely from
// constraints, edits one dimension and re-solves, then exports the result. It
// is the smallest end-to-end taste of the engine: author geometry from points,
// constrain it, solve, edit, re-solve, export.
func Example_sketch_quickstart() {
  s := sketch.New()

  // Four corners as rough initial guesses; the solver finds the exact spots.
  // Sharing a *Point between two lines is what makes a corner a corner.
  a := s.AddPoint(0, 0)
  b := s.AddPoint(18, 2)
  c := s.AddPoint(17, 11)
  d := s.AddPoint(1, 13)

  ab := s.AddLine(a, b)
  bc := s.AddLine(b, c)
  dc := s.AddLine(d, c)
  ad := s.AddLine(a, d)

  // Ground one corner at the origin so the sketch can't float away.
  a.MoveTo(0, 0)
  s.Fix(a)

  // Axis-align the four sides.
  s.AddConstraint(
    sketch.NewHorizontal(ab),
    sketch.NewHorizontal(dc),
    sketch.NewVertical(ad),
    sketch.NewVertical(bc),
  )

  // Driving dimensions: editable values that make the sketch parametric.
  width := sketch.NewDistance(a, b, 20)
  height := sketch.NewDistance(a, d, 12)
  s.AddConstraint(width, height)

  res, err := s.Solve()
  if err != nil {
    fmt.Printf("failed to solve: %s\n", err)
    return
  }
  fmt.Printf("DOF=%d b=(%.0f,%.0f) c=(%.0f,%.0f) d=(%.0f,%.0f)\n",
    res.DOF, b.X(), b.Y(), c.X(), c.Y(), d.X(), d.Y())

  // Edit a dimension and re-solve: the rectangle becomes 35 x 12.
  width.Set(35)
  if _, err := s.Solve(); err != nil {
    fmt.Printf("failed to re-solve: %s\n", err)
    return
  }
  fmt.Printf("after width.Set(35): b=(%.0f,%.0f) c=(%.0f,%.0f)\n",
    b.X(), b.Y(), c.X(), c.Y())

  // Export the solved sketch in several formats.
  svg, err := s.SVG()
  if err != nil {
    fmt.Printf("failed to render SVG: %s\n", err)
    return
  }
  dxf, err := s.DXF()
  if err != nil {
    fmt.Printf("failed to render DXF: %s\n", err)
    return
  }
  data, err := s.MarshalJSON()
  if err != nil {
    fmt.Printf("failed to marshal JSON: %s\n", err)
    return
  }
  fmt.Printf("exports non-empty: svg=%t dxf=%t json=%t\n", len(svg) > 0, len(dxf) > 0, len(data) > 0)

  // Output:
  // DOF=0 b=(20,0) c=(20,12) d=(0,12)
  // after width.Set(35): b=(35,0) c=(35,12)
  // exports non-empty: svg=true dxf=true json=true
}
```
source: [examples/sketch_readme_example_test.go](examples/sketch_readme_example_test.go)
<!-- END INCLUDE -->

The code blocks in this README are embedded from compiled, `go test`-verified
examples — see [Regenerating the README](#regenerating-the-readme). For more
worked programs — a constraint-built hexagon, a parametric plate, a parametric
fillet, an ambiguity probe — browse the [`examples`](examples) package.

## Geometry

Author geometry on the sketch from points; each builder returns a solver-bound
handle:

| Builder | Bound handle |
|---|---|
| `s.AddPoint(x, y)` | `*sketch.Point` (coordinates are solved for) |
| `s.AddLine(p1, p2)` | `*sketch.Line` |
| `s.AddCircle(center, r)` | `*sketch.Circle` |
| `s.AddArc(center, start, end)` | `*sketch.Arc` |
| `s.AddEllipse(center, rx, ry, rot)` | `*sketch.Ellipse` (semi-axes and rotation are solved for) |
| `s.AddSpline(p0, p1, p2, p3, …)` | `*sketch.Spline` (clamped cubic B-spline) |

The curve builders take points you have already added; sharing a `*Point`
between entities is how topology is expressed (a shared corner is one point),
and each `Add…` creates a fresh entity. A bound handle exposes solved values
(`p.X()`, `l.Length()`, `c.R()`, `e.Rx()`) and a transient [`geom`](geom)
snapshot of its current shape via `Geometry()`.

A spline's control points are ordinary sketch points: constrain, dimension,
ground or drag (`WithGoal`) them and the curve follows — the curve itself
carries no extra unknowns. Clamping means the curve starts/ends exactly at the
first/last control points with end tangents along the outer control-polygon
legs, so endpoint attachment is point coincidence and end tangency is a
`NewParallel` on a construction line over the first leg. `sp.Eval(t)` /
`sp.Polyline(n)` evaluate the solved curve.

Grounding:

* `p.MoveTo(x, y)` — move a point to `(x, y)` (sets the solver's starting guess).
* `s.Fix(p)` — pin a point at its current location.
* `s.Unfix(p)` — release a pinned point.

To ground a point at a specific location, move it first: `p.MoveTo(x, y)` then
`s.Fix(p)`.

Any entity can be marked as construction geometry with `e.SetConstruction(true)`
(rendered dashed/grey, exported to a separate DXF layer).

### Compound shapes

`s.AddRectangle(x1, y1, x2, y2)`, `s.AddPolygon(cx, cy, n, r)` and
`s.AddSlot(x1, y1, x2, y2, r)` build a whole shape — primitives plus the
constraints that hold it in shape (horizontal/vertical sides; equal sides and
equal construction spokes; equal cap radii and perpendicular contact spokes) —
and return a grouping handle with the bound parts. The pieces are ordinary
sketch geometry/constraints and serialize as such; position and size stay free
to ground and dimension.

### Shaping templates (the `geom` toolkit)

Generic geometry can be shaped *before* committing: `geom` provides
intersection math (`LineLineIntersection`, `SegmentIntersection`,
`LineCircleIntersections`, `CircleCircleIntersections`, and arc variants
filtered by `Arc.Contains`) plus modification helpers — `SplitLineAt`,
`Fillet` (replaces a shared corner with a tangent arc, shortening both legs)
and `Chamfer` (straight cut). Commit the result with the usual `Add…` calls,
adding constraints to keep the shape parametric (e.g. tangency spokes, as
`AddSlot` does).

### Removing geometry and constraints

`s.RemoveConstraint(c)`, `s.RemoveEntity(e)` and `s.RemovePoint(p)` undo
commits. Removing an entity cascades every constraint that references it
(including auto-added internal ones) but keeps its points — they may be
shared; remove orphans explicitly. `RemovePoint` refuses (returns false)
while any entity still uses the point. Removed handles are dead — discard
them; re-adding the same generic geometry creates a fresh instance. Sketch
documents carry a `"version"` field; legacy unversioned files still load.

## Constraints

Construct a constraint with its `New…` function and commit it with
`s.AddConstraint(...)`.

**Geometric**

`NewCoincident`, `NewHorizontal`, `NewVertical`, `NewParallel`,
`NewPerpendicular`, `NewPointOnLine`, `NewCollinear`, `NewPointOnCircle`,
`NewPointOnEllipse`, `NewMidpoint`, `NewSymmetric`, `NewConcentric`,
`NewEqual` (line lengths), `NewEqualRadius` (circles and/or arcs),
`NewTangent` (line to circle or arc), `NewTangentCircles` (circle/arc to
circle/arc, internal or external). Tangency treats an arc as its full circle —
the tangent point is not required to lie within the arc's sweep. Tangency to
an ellipse is not supported.

**Dimensional** (editable; each carries a unit and has a `.Set`/`.SetValue`)

`NewDistance`, `NewHorizontalDistance`, `NewVerticalDistance` (signed Δx/Δy),
`NewDistancePointLine` (perpendicular point↔line), `NewDistanceLines`
(perpendicular line↔line; forces the lines parallel), `NewOffset` (signed
parallel offset, positive on the left of the source line's direction),
`NewRadius`, `NewDiameter`, `NewAngle` (signed, counterclockwise from l1's
direction to l2's), `NewSemiMajor`/`NewSemiMinor` (ellipse semi-axes),
`NewEllipseRotation`.

Sign and side conventions matter: signed dimensions (`NewAngle`, `NewOffset`,
`NewHorizontalDistance`/`NewVerticalDistance`) pin a single configuration per
value, while unsigned constraints (`NewTangent`, `NewDistancePointLine`,
`NewDistanceLines`, `NewSymmetric`) keep whichever side the geometry starts
on. See "Orientation and sign conventions" in the
[package documentation](https://pkg.go.dev/github.com/lestrrat-3d/sketch).

Any dimension can be flipped to a **driven (reference) dimension** with
`.SetDriven(true)`: it stops constraining the geometry and instead reports the
measured value — after each `Solve` its `.Target()` holds the measurement in
the dimension's own unit. `.SetDriven(false)` turns it back into a driving
dimension, keeping the last measured value as the new target.

## Units

Dimensions and parameters carry units via the standalone [`units`](units)
package. Units are **typed** — you use `units.Millimeter`, `units.Inch`,
`units.Degree`, … rather than strings — and a `units.Value` knows its own unit
and converts only through the library (no magnitude relabelling):

```go
w := units.Inches(4)
mm, _ := w.In(units.Millimeter) // 101.6

s := sketch.New()               // default units: mm and degrees
s.SetUnits(units.Imperial())    // ... or inches and degrees

d := sketch.NewDistance(a, b, 0)
s.AddConstraint(d)
d.SetValue(units.Inches(4))     // solves to 101.6 mm internally
s.AddConstraint(sketch.NewAngle(l1, l2, 90)) // 90 in the default angle unit (degrees)
```

The solver works in base units (millimetre, radian); a dimension's residual
converts its target with `Target().Base()`. A bare-float constructor value
adopts the sketch's default unit for that kind when the constraint is added.
Default systems come from `units.Metric()` (mm/deg), `units.SI()` (m/rad) and
`units.Imperial()` (in/deg); mixing kinds (e.g. adding a length to an angle)
returns `units.ErrIncompatible`, and `units.Define` registers custom units.

## Parameters & expressions

Every dimension can be **driven by an expression** instead of a literal. You
supply a parameter table (the [`param`](param) package) when binding a
dimension; a bound dimension is re-evaluated against that table before every
solve, so changing one parameter cascades through everything that depends on it.
Parameters carry units too:

<!-- INCLUDE(examples/sketch_parametric_example_test.go) -->
```go
package examples_test

import (
  "errors"
  "fmt"

  "github.com/lestrrat-3d/sketch"
  "github.com/lestrrat-3d/sketch/param"
  "github.com/lestrrat-3d/sketch/units"
)

// Example_sketch_parametric drives a sketch from a parameter table: a
// rectangular plate with a centered hole whose dimensions are all defined by
// expressions. Changing a single parameter and re-solving updates everything.
func Example_sketch_parametric() {
  s := sketch.New()

  // Four corners + a center point for the hole (rough initial guesses).
  a := s.AddPoint(0, 0)
  b := s.AddPoint(10, 1)
  c := s.AddPoint(9, 6)
  d := s.AddPoint(1, 5)
  o := s.AddPoint(5, 3)

  ab := s.AddLine(a, b)
  bc := s.AddLine(b, c)
  dc := s.AddLine(d, c)
  ad := s.AddLine(a, d)
  hole := s.AddCircle(o, 1)

  // Geometric constraints: grounded origin, axis-aligned rectangle.
  a.MoveTo(0, 0)
  s.Fix(a)
  s.AddConstraint(
    sketch.NewHorizontal(ab),
    sketch.NewHorizontal(dc),
    sketch.NewVertical(ad),
    sketch.NewVertical(bc),
  )

  // Parameters: a single driving width as a typed length; everything else is
  // derived from it. Geometry solves in base millimetres regardless of the
  // units the parameters are expressed in.
  p := param.New()
  if err := errors.Join(
    p.SetValue("width", units.Millimeters(120)),
    p.SetExpr("height", "width * 0.6", units.Millimeter),
    p.SetExpr("hole_d", "min(width, height) / 3", units.Millimeter),
  ); err != nil {
    fmt.Printf("failed to define parameters: %s\n", err)
    return
  }

  // Add each dimension, then bind it to an expression evaluated against p.
  bind := func(dim sketch.Dimension, expr string) error {
    s.AddConstraint(dim)
    return s.Bind(dim, p, expr)
  }
  if err := errors.Join(
    bind(sketch.NewDistance(a, b, 0), "width"),
    bind(sketch.NewDistance(a, d, 0), "height"),
    bind(sketch.NewHorizontalDistance(a, o, 0), "width / 2"), // hole centered
    bind(sketch.NewVerticalDistance(a, o, 0), "height / 2"),
    bind(sketch.NewRadius(hole, 0), "hole_d / 2"),
  ); err != nil {
    fmt.Printf("failed to bind dimensions: %s\n", err)
    return
  }

  report := func() error {
    res, err := s.Solve()
    if err != nil {
      return err
    }
    w, err := p.GetValue("width")
    if err != nil {
      return err
    }
    fmt.Printf("width=%s -> plate %.1f x %.1f mm, hole d=%.1f at (%.0f, %.0f), DOF %d\n",
      w, b.X(), d.Y(), 2*hole.R(), o.X(), o.Y(), res.DOF)
    return nil
  }

  if err := report(); err != nil { // width = 120 mm
    fmt.Printf("failed to solve: %s\n", err)
    return
  }

  // Change the one driving parameter — and express it in inches. The units
  // library converts; height and hole follow automatically.
  if err := p.SetValue("width", units.Inches(8)); err != nil {
    fmt.Printf("failed to update width: %s\n", err)
    return
  }
  if err := report(); err != nil {
    fmt.Printf("failed to solve after edit: %s\n", err)
    return
  }

  // Output:
  // width=120 mm -> plate 120.0 x 72.0 mm, hole d=24.0 at (60, 36), DOF 0
  // width=8 in -> plate 203.2 x 121.9 mm, hole d=40.6 at (102, 61), DOF 0
}
```
source: [examples/sketch_parametric_example_test.go](examples/sketch_parametric_example_test.go)
<!-- END INCLUDE -->

Within an expression, parameters contribute their value in base units and
numeric literals are dimensionless; the declared unit (the third argument to
`SetExpr`) tags the result. Binding a length dimension directly to an angle
parameter is reported as an error at solve time.

The table is required at [`Bind`](https://pkg.go.dev/github.com/lestrrat-3d/sketch#Sketch.Bind)
time and all of a sketch's dimensions must share one table. Parameters, each
dimension's unit and bound expression, and the unit system are all included in
the sketch's JSON, so a parametric sketch reloads still parametric. The
expression language supports `+ - * / %`, right-associative `^`, unary `±`,
parentheses, numeric literals (including scientific notation), constants (`pi`,
`tau`, `e`, `phi`) and functions (`sin`, `sqrt`, `min`/`max`, `hypot`, `clamp`,
…). Register your own with `table.SetFunc` / `table.SetConst`.

## Profiles

`s.Profiles()` detects closed region boundaries: every non-construction circle
and ellipse, plus every closed loop of lines/arcs connected end-to-end through
shared points (`geom.Loops` underneath). Open chains and construction geometry
contribute nothing. Profiles are the input that future extrude/revolve
operations will consume. Boundaries that cross without sharing a point are not
subdivided into regions (yet).

## Solving

```go
res, err := s.Solve()                                   // default settings
res, err := s.Solve(                                    // or tune them
    sketch.WithMaxIterations(200),
    sketch.WithTolerance(1e-10),
)
```

`Solve` reports:

* `res.Converged` — whether all constraints were satisfied within tolerance.
* `res.DOF` — remaining degrees of freedom (`0` means fully constrained).
* `res.Redundant` — number of redundant/conflicting constraint equations.
* `res.Iterations`, `res.Residual`.

`s.DOF()` reports the current degrees of freedom without moving any geometry.
`s.RedundantConstraints()` identifies *which* constraints are redundant (or
conflicting) at the current configuration — of two duplicates, the later-added
one is reported.

If the solver cannot satisfy the constraints (typically an over-constrained or
contradictory sketch) `Solve` returns `ErrNotConverged` together with the
partial result.

### Goals (interactive dragging)

`Solve` accepts soft targets — the engine primitive behind drag interactions:

```go
res, err := s.Solve(sketch.WithGoal(p, x, y)) // pull p toward (x, y)
```

Constraints always win: the geometry settles at the closest feasible
configuration, and an unreachable target is not an error. Goals are transient
(per-call, never serialized, invisible to DOF/redundancy analysis). Issue one
goal per pointer-move event for dragging; several goals move whole selections.
Gesture policy (what dragging a line's body *means*) belongs to the UI layer —
see `docs/goal-solve-design.md`.

### How it works

All scalar unknowns (point coordinates, circle radii) form one parameter
vector. Each constraint contributes one or more residual equations, normalized
to consistent units (lengths in length units, angles dimensionless) so the
system stays well conditioned. A Levenberg–Marquardt least-squares solver with
a numerical Jacobian drives the residuals to zero; the rank of the Jacobian
gives the degree-of-freedom and redundancy analysis.

## Regenerating the README

The Go code blocks in this README are not hand-maintained — they are embedded
from the compiled, `go test`-verified examples in the [`examples`](examples)
package, so they cannot drift from the real API. Each block lives between a pair
of markers:

```
<!-- INCLUDE(examples/sketch_readme_example_test.go) -->
```go
package examples_test

import (
  "fmt"

  "github.com/lestrrat-3d/sketch"
)

// Example_sketch_quickstart builds an axis-aligned rectangle entirely from
// constraints, edits one dimension and re-solves, then exports the result. It
// is the smallest end-to-end taste of the engine: author geometry from points,
// constrain it, solve, edit, re-solve, export.
func Example_sketch_quickstart() {
  s := sketch.New()

  // Four corners as rough initial guesses; the solver finds the exact spots.
  // Sharing a *Point between two lines is what makes a corner a corner.
  a := s.AddPoint(0, 0)
  b := s.AddPoint(18, 2)
  c := s.AddPoint(17, 11)
  d := s.AddPoint(1, 13)

  ab := s.AddLine(a, b)
  bc := s.AddLine(b, c)
  dc := s.AddLine(d, c)
  ad := s.AddLine(a, d)

  // Ground one corner at the origin so the sketch can't float away.
  a.MoveTo(0, 0)
  s.Fix(a)

  // Axis-align the four sides.
  s.AddConstraint(
    sketch.NewHorizontal(ab),
    sketch.NewHorizontal(dc),
    sketch.NewVertical(ad),
    sketch.NewVertical(bc),
  )

  // Driving dimensions: editable values that make the sketch parametric.
  width := sketch.NewDistance(a, b, 20)
  height := sketch.NewDistance(a, d, 12)
  s.AddConstraint(width, height)

  res, err := s.Solve()
  if err != nil {
    fmt.Printf("failed to solve: %s\n", err)
    return
  }
  fmt.Printf("DOF=%d b=(%.0f,%.0f) c=(%.0f,%.0f) d=(%.0f,%.0f)\n",
    res.DOF, b.X(), b.Y(), c.X(), c.Y(), d.X(), d.Y())

  // Edit a dimension and re-solve: the rectangle becomes 35 x 12.
  width.Set(35)
  if _, err := s.Solve(); err != nil {
    fmt.Printf("failed to re-solve: %s\n", err)
    return
  }
  fmt.Printf("after width.Set(35): b=(%.0f,%.0f) c=(%.0f,%.0f)\n",
    b.X(), b.Y(), c.X(), c.Y())

  // Export the solved sketch in several formats.
  svg, err := s.SVG()
  if err != nil {
    fmt.Printf("failed to render SVG: %s\n", err)
    return
  }
  dxf, err := s.DXF()
  if err != nil {
    fmt.Printf("failed to render DXF: %s\n", err)
    return
  }
  data, err := s.MarshalJSON()
  if err != nil {
    fmt.Printf("failed to marshal JSON: %s\n", err)
    return
  }
  fmt.Printf("exports non-empty: svg=%t dxf=%t json=%t\n", len(svg) > 0, len(dxf) > 0, len(data) > 0)

  // Output:
  // DOF=0 b=(20,0) c=(20,12) d=(0,12)
  // after width.Set(35): b=(35,0) c=(35,12)
  // exports non-empty: svg=true dxf=true json=true
}
```
source: [examples/sketch_readme_example_test.go](examples/sketch_readme_example_test.go)
<!-- END INCLUDE -->
```

Regenerate every block in place (no network access required) with:

```sh
go generate ./...
# or, directly:
go run ./internal/cmd/genreadme README.md
```

An optional second argument embeds a single function instead of the whole file:
`<!-- INCLUDE(examples/foo_example_test.go,Example_sketch_bar) -->`. After
editing any embedded example, re-run the command and commit the regenerated
README alongside the code.

## License

This project is **source-available**, and is licensed under the
[PolyForm Noncommercial License 1.0.0](LICENSE).

* **Noncommercial use is free.** Individuals, hobby and personal projects,
  research, education, nonprofits, and government may use, modify, and
  redistribute it at no cost, subject to the license terms.
* **Commercial / business use requires a separate license.** Any use by or for
  a business, or for commercial advantage, is not permitted under the
  noncommercial license. To obtain a commercial license, reach out on Bluesky
  at [@lestrrat.bsky.social](https://bsky.app/profile/lestrrat.bsky.social).

### Contributions

This repository does **not** accept external pull requests.
