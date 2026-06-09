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

```go
s := sketch.New()

a := s.AddPoint(0, 0)
b := s.AddPoint(18, 2)  // rough guesses; the solver finds the exact positions
c := s.AddPoint(17, 11)
d := s.AddPoint(1, 13)

ab, bc, dc, ad := s.AddLine(a, b), s.AddLine(b, c), s.AddLine(d, c), s.AddLine(a, d)

s.Lock(a, 0, 0)        // ground a corner at the origin
s.Horizontal(ab)
s.Horizontal(dc)
s.Vertical(ad)
s.Vertical(bc)
width  := s.Distance(a, b, 20) // driving dimensions
height := s.Distance(a, d, 12)

res, err := s.Solve()
// res.DOF == 0  -> fully constrained
// b == (20, 0), c == (20, 12), d == (0, 12)

width.Set(35)   // edit a dimension ...
s.Solve()       // ... and re-solve: the rectangle is now 35 x 12

svg, _ := s.SVG(sketch.DefaultSVGOptions())
dxf, _ := s.DXF()
data, _ := s.MarshalJSON()
_ = height
```

See [`examples/hexagon`](examples/hexagon) for a complete program that builds a
regular hexagon entirely from constraints and writes SVG/DXF/JSON.

## Geometry

| Constructor | Description |
|---|---|
| `s.AddPoint(x, y)` | A free point. Its coordinates are unknowns solved for. |
| `s.AddLine(a, b)` | A segment between two points. |
| `s.AddCircle(center, r)` | A circle with a center point and radius. |
| `s.AddArc(center, start, end)` | An arc swept counter-clockwise from `start` to `end`. |

Grounding:

* `s.Fix(p)` — pin a point at its current location.
* `s.Lock(p, x, y)` — move a point to `(x, y)` and pin it.
* `s.Unfix(p)` — release a pinned point.

Any entity's `.Construction` field can be set to mark it as construction
geometry (rendered dashed/grey, exported to a separate DXF layer).

## Constraints

**Geometric**

`Coincident`, `Horizontal`, `Vertical`, `Parallel`, `Perpendicular`,
`PointOnLine`, `Collinear`, `PointOnCircle`, `Midpoint`, `Symmetric`,
`Concentric`, `Equal` (line lengths), `EqualRadius`, `Tangent` (line–circle),
`TangentCircles` (circle–circle, internal or external).

**Dimensional** (editable; each returns a handle with a `.Set(value)` method)

`Distance`, `HorizontalDistance`, `VerticalDistance`, `Radius`, `Diameter`,
`Angle` (radians, between two lines).

## Parameters & expressions

Every dimension can be **driven by an expression** instead of a literal. You
supply a parameter table (the [`param`](param) package) when binding a
dimension; a bound dimension is re-evaluated against that table before every
solve, so changing one parameter cascades through everything that depends on it.

```go
p := param.New()
p.Set("width", "120")
p.Set("height", "width * 0.6")        // expressions may reference others
p.Set("hole_d", "min(width, height) / 3")

s.Bind(s.Distance(a, b, 0), p, "width")
s.Bind(s.Distance(a, d, 0), p, "height")
s.Bind(s.Radius(hole, 0), p, "hole_d / 2")

s.Solve()                 // width=120 -> height=72, hole d=24
p.Set("width", "200")     // change ONE parameter ...
s.Solve()                 // ... height=120 and hole d=40 follow

// Calling a dimension's .Set(v) overrides and unbinds it.
```

The table is required at [`Bind`](https://pkg.go.dev/github.com/lestrrat-3d/sketch#Sketch.Bind)
time and all of a sketch's dimensions must share one table. Parameters (and each
dimension's bound expression) are included in the sketch's JSON, so a parametric
sketch reloads still parametric, and `s.Params()` returns the restored table.
The expression language supports arithmetic, `^`, parentheses, constants (`pi`,
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
