# Units & parameters

## Units

Dimensions and parameters carry units via the standalone [`units`](../../units)
package. Units are **typed** — you use `units.Millimeter`, `units.Inch`,
`units.Degree`, … rather than strings — and a `units.Value` knows its own unit
and converts only through the library (no magnitude relabelling):

<!-- INCLUDE(../../examples/sketch_units_example_test.go,Example_sketch_units) -->
```go
// Example_sketch_units shows that dimensions carry typed units while the solver
// works in base millimetres: a distance set in inches solves to its millimetre
// equivalent, and conversion happens only through the units library.
func Example_sketch_units() {
  // A units.Value knows its own unit and converts through the library.
  w := units.Inches(4)
  mm, err := w.In(units.Millimeter)
  if err != nil {
    fmt.Printf("failed to convert: %s\n", err)
    return
  }
  fmt.Printf("%s = %.1f mm\n", w, mm)

  // A dimension carries a unit; internally the solver stays in millimetres.
  world := sketch.NewWorld()
  s, _ := world.CreateSketch(world.XY())
  a := s.CreatePoint(0, 0)
  b := s.CreatePoint(50, 0)
  a.MoveTo(0, 0)
  s.Fix(a)
  s.AddConstraint(sketch.NewHorizontal(s.CreateLine(a, b)))

  d := sketch.NewDistance(a, b, 0)
  s.AddConstraint(d)
  if err := d.SetValue(units.Inches(4)); err != nil { // 4 in -> 101.6 mm
    fmt.Printf("failed to set value: %s\n", err)
    return
  }
  if _, err := s.Solve(); err != nil {
    fmt.Printf("failed to solve: %s\n", err)
    return
  }
  fmt.Printf("|ab| = %.1f mm\n", b.X())

  // Output:
  // 4 in = 101.6 mm
  // |ab| = 101.6 mm
}
```
source: [../../examples/sketch_units_example_test.go](../../examples/sketch_units_example_test.go)
<!-- END INCLUDE -->

The solver works in base units (millimetre, radian); a dimension's residual
converts its target with `Target().Base()`. A bare-float constructor value
adopts the sketch's default unit for that kind when the constraint is added.
Default systems come from `units.Metric()` (mm/deg), `units.SI()` (m/rad) and
`units.Imperial()` (in/deg); mixing kinds (e.g. adding a length to an angle)
returns `units.ErrIncompatible`, and `units.Define` registers custom units.

## Parameters & expressions

Every dimension can be **driven by an expression** instead of a literal. You
supply a parameter table (the [`param`](../../param) package) when binding a
dimension; a bound dimension is re-evaluated against that table before every
solve, so changing one parameter cascades through everything that depends on it.
Parameters carry units too:

<!-- INCLUDE(../../examples/sketch_parametric_example_test.go) -->
```go
package examples_test

import (
  "errors"
  "fmt"

  "github.com/lestrrat-3d/sketch"
  "github.com/lestrrat-3d/sketch/units"
)

// Example_sketch_parametric drives a sketch from a parameter table: a
// rectangular plate with a centered hole whose dimensions are all defined by
// expressions. Changing a single parameter and re-solving updates everything.
func Example_sketch_parametric() {
  w := sketch.NewWorld()
  s, _ := w.CreateSketch(w.XY())

  // Four corners + a center point for the hole (rough initial guesses).
  a := s.CreatePoint(0, 0)
  b := s.CreatePoint(10, 1)
  c := s.CreatePoint(9, 6)
  d := s.CreatePoint(1, 5)
  o := s.CreatePoint(5, 3)

  ab := s.CreateLine(a, b)
  bc := s.CreateLine(b, c)
  dc := s.CreateLine(d, c)
  ad := s.CreateLine(a, d)
  hole := s.CreateCircle(o, 1)

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
  // derived from it. The world's shared parameter table drives the sketch.
  // Geometry solves in base millimetres regardless of the units the parameters
  // are expressed in.
  p := w.Params()
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
source: [../../examples/sketch_parametric_example_test.go](../../examples/sketch_parametric_example_test.go)
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
