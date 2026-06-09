# sketch

A standalone, fully programmable **parametric 2D sketch engine** for Go, in the
spirit of the sketch environment in Autodesk Fusion.

You build geometry — points, lines, circles, arcs — in code, tie it together
with geometric and dimensional **constraints**, and a numerical solver moves the
geometry so that every constraint is satisfied at once. Because dimensions are
ordinary editable values, sketches are fully parametric: change a dimension,
re-solve, and the geometry updates.

* Pure Go, **zero dependencies** (standard library only).
* Levenberg–Marquardt geometric constraint solver with degrees-of-freedom and
  redundancy analysis.
* A rich, Fusion-like constraint set.
* Export to **SVG** (visual inspection), **DXF** R12 (CAD interchange) and
  **JSON** (lossless save / load round-trip).

```go
import "github.com/lestrrat-3d/sketch"
```

## Quick start

Geometry comes in two layers. **Generic geometry** (the standalone
[`geom`](geom) package: `geom.Point`, `geom.Line`, `geom.Circle`, `geom.Arc`) is
context-agnostic — just coordinates, no solver. You **commit** it into a sketch
with the `Add…` methods, which return solver-bound handles (`sketch.Point`,
`sketch.Line`, …). The same generic geometry can be committed into several
independent sketches.

```go
import (
	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
)

// Generic geometry — reusable, no sketch yet.
a := geom.NewPoint(0, 0)
b := geom.NewPoint(18, 2) // rough guesses; the solver finds the exact positions
c := geom.NewPoint(17, 11)
d := geom.NewPoint(1, 13)

s := sketch.New()
ab := s.AddLine(geom.NewLine(a, b)) // commits the line and its two points
bc := s.AddLine(geom.NewLine(b, c))
dc := s.AddLine(geom.NewLine(d, c))
ad := s.AddLine(geom.NewLine(a, d))

ab.Start.MoveTo(0, 0) // move a corner to the origin and ground it
s.Fix(ab.Start)
s.AddConstraint(
	sketch.NewHorizontal(ab),
	sketch.NewHorizontal(dc),
	sketch.NewVertical(ad),
	sketch.NewVertical(bc),
)
width := sketch.NewDistance(ab.Start, ab.End, 20) // driving dimensions
height := sketch.NewDistance(ad.Start, ad.End, 12)
s.AddConstraint(width, height)

res, err := s.Solve()
// res.DOF == 0  -> fully constrained; ab.End == (20, 0), bc.End == (20, 12)

width.Set(35) // edit a dimension ...
s.Solve()     // ... and re-solve: the rectangle is now 35 x 12

svg, _ := s.SVG(sketch.DefaultSVGOptions())
dxf, _ := s.DXF()
data, _ := s.MarshalJSON()
_, _, _ = res, height, c
```

See [`examples/hexagon`](examples/hexagon) for a complete program that builds a
regular hexagon entirely from constraints and writes SVG/DXF/JSON.

## Geometry

Construct generic geometry with `geom.New…`, then commit it with the matching
`Add…` to get a solver-bound handle:

| Generic ([`geom`](geom)) | Commit | Bound handle |
|---|---|---|
| `geom.NewPoint(x, y)` | `s.AddPoint(p)` | `*sketch.Point` (coordinates are solved for) |
| `geom.NewLine(a, b)` | `s.AddLine(l)` | `*sketch.Line` |
| `geom.NewCircle(center, r)` | `s.AddCircle(c)` | `*sketch.Circle` |
| `geom.NewArc(center, start, end)` | `s.AddArc(a)` | `*sketch.Arc` |

`AddLine`/`AddCircle`/`AddArc` commit any referenced generic points first and
return the bound object; all `Add…` are idempotent (a generic primitive maps to
one bound instance per sketch). A bound handle exposes solved values (`p.X()`,
`l.Length()`, `c.R()`) and the generic geometry it came from (`p.Generic()`).

Grounding (per-sketch — the same generic point may be fixed in one sketch and
free in another):

* `p.MoveTo(x, y)` — move a point to `(x, y)` (sets the solver's starting guess).
* `s.Fix(p)` — pin a point at its current location.
* `s.Unfix(p)` — release a pinned point.

To ground a point at a specific location, move it first: `p.MoveTo(x, y)` then
`s.Fix(p)`.

Any entity's `.Construction` field can be set to mark it as construction
geometry (rendered dashed/grey, exported to a separate DXF layer).

## Constraints

Construct a constraint with its `New…` function and commit it with
`s.AddConstraint(...)`.

**Geometric**

`NewCoincident`, `NewHorizontal`, `NewVertical`, `NewParallel`,
`NewPerpendicular`, `NewPointOnLine`, `NewCollinear`, `NewPointOnCircle`,
`NewMidpoint`, `NewSymmetric`, `NewConcentric`, `NewEqual` (line lengths),
`NewEqualRadius`, `NewTangent` (line–circle), `NewTangentCircles` (circle–circle,
internal or external).

**Dimensional** (editable; each carries a unit and has a `.Set`/`.SetValue`)

`NewDistance`, `NewHorizontalDistance`, `NewVerticalDistance`, `NewRadius`,
`NewDiameter`, `NewAngle` (between two lines).

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

```go
p := param.New()
p.SetValue("width", units.Millimeters(120))       // a typed length
p.SetExpr("height", "width * 0.6", units.Millimeter)
p.SetExpr("hole_d", "min(width, height) / 3", units.Millimeter)

width := sketch.NewDistance(a, b, 0)
height := sketch.NewDistance(a, d, 0)
hr := sketch.NewRadius(hole, 0)
s.AddConstraint(width, height, hr)
s.Bind(width, p, "width")
s.Bind(height, p, "height")
s.Bind(hr, p, "hole_d / 2")

s.Solve()                              // 120 x 72, hole d 24 (mm)
p.SetValue("width", units.Inches(8))   // change ONE parameter, in inches ...
s.Solve()                              // ... 203.2 x 121.9, hole d 40.6 (mm)

// Calling a dimension's .Set(v) overrides and unbinds it.
```

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
…). Register your own with `table.SetFunc` / `table.SetConst`. See
[`examples/parametric`](examples/parametric).

## Solving

```go
res, err := s.Solve()                       // default settings
res, err := s.Solve(sketch.SolveOptions{    // or tune them
    MaxIterations: 200,
    Tolerance:     1e-10,
})
```

`Solve` reports:

* `res.Converged` — whether all constraints were satisfied within tolerance.
* `res.DOF` — remaining degrees of freedom (`0` means fully constrained).
* `res.Redundant` — number of redundant/conflicting constraint equations.
* `res.Iterations`, `res.Residual`.

`s.DOF()` reports the current degrees of freedom without moving any geometry.

If the solver cannot satisfy the constraints (typically an over-constrained or
contradictory sketch) `Solve` returns `ErrNotConverged` together with the
partial result.

### How it works

All scalar unknowns (point coordinates, circle radii) form one parameter
vector. Each constraint contributes one or more residual equations, normalized
to consistent units (lengths in length units, angles dimensionless) so the
system stays well conditioned. A Levenberg–Marquardt least-squares solver with
a numerical Jacobian drives the residuals to zero; the rank of the Jacobian
gives the degree-of-freedom and redundancy analysis.

## License

See the repository for license details.
