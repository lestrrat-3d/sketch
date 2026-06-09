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
* Export to **SVG** (visual inspection), **DXF** (CAD interchange) and **JSON**
  (save / load).

```go
import "github.com/lestrrat-3d/sketch"
```

## Quick start

Geometry and constraints are **constructed** detached (`sketch.NewPoint`,
`sketch.NewLine`, `sketch.NewDistance`, …) and then **committed** to the sketch
as a separate step (`s.AddPoint`, `s.AddLine`, `s.AddConstraint`). Adding a line
or a constraint idempotently pulls in any geometry it references, so you only
have to add the top-level objects.

```go
s := sketch.New()

a := sketch.NewPoint(0, 0)
b := sketch.NewPoint(18, 2) // rough guesses; the solver finds the exact positions
c := sketch.NewPoint(17, 11)
d := sketch.NewPoint(1, 13)

ab := s.AddLine(sketch.NewLine(a, b)) // commits the line and its two points
bc := s.AddLine(sketch.NewLine(b, c))
dc := s.AddLine(sketch.NewLine(d, c))
ad := s.AddLine(sketch.NewLine(a, d))

s.Lock(a, 0, 0)           // ground a corner at the origin
s.AddConstraint(
	sketch.NewHorizontal(ab),
	sketch.NewHorizontal(dc),
	sketch.NewVertical(ad),
	sketch.NewVertical(bc),
)
width := sketch.NewDistance(a, b, 20) // driving dimensions
height := sketch.NewDistance(a, d, 12)
s.AddConstraint(width, height)

res, err := s.Solve()
// res.DOF == 0  -> fully constrained
// b == (20, 0), c == (20, 12), d == (0, 12)

width.Set(35) // edit a dimension ...
s.Solve()     // ... and re-solve: the rectangle is now 35 x 12

svg, _ := s.SVG(sketch.DefaultSVGOptions())
dxf, _ := s.DXF()
data, _ := s.MarshalJSON()
_ = res
_ = height
```

See [`examples/hexagon`](examples/hexagon) for a complete program that builds a
regular hexagon entirely from constraints and writes SVG/DXF/JSON.

## Geometry

Construct with the `New…` functions, then commit with the matching `Add…`:

| Construct | Commit | Description |
|---|---|---|
| `sketch.NewPoint(x, y)` | `s.AddPoint(p)` | A free point; its coordinates are solved for. |
| `sketch.NewLine(a, b)` | `s.AddLine(l)` | A segment between two points. |
| `sketch.NewCircle(center, r)` | `s.AddCircle(c)` | A circle with a center point and radius. |
| `sketch.NewArc(center, start, end)` | `s.AddArc(a)` | An arc swept counter-clockwise from `start` to `end`. |

`AddLine`/`AddCircle`/`AddArc` commit any referenced points first and return the
committed object; they are idempotent. `s.AddConstraint(...)` is variadic and
likewise commits any geometry a constraint references. A detached object is
fully inspectable (`p.X()`, `l.Length()`) before it is added.

Grounding:

* `s.Fix(p)` — pin a point at its current location.
* `s.Lock(p, x, y)` — move a point to `(x, y)` and pin it.
* `s.Unfix(p)` — release a pinned point.

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
expression language supports arithmetic, `^`, parentheses, constants (`pi`,
`tau`, `e`, `phi`) and functions (`sin`, `sqrt`, `min`/`max`, `hypot`, `clamp`,
…); see [`examples/parametric`](examples/parametric).

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
