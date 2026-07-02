# User guide

Task-focused guides for building with the `sketch` engine. Start with the
[README](../../README.md) for the overview and quick start, then dive into a
topic here.

* [Geometry](geometry.md) — the builders, grounding, construction geometry,
  compound shapes, the `geom` shaping toolkit, and removal.
* [Constraints](constraints.md) — the geometric and dimensional constraint set,
  sign conventions, driven dimensions, and introspection/naming.
* [Units & parameters](units-and-parameters.md) — typed units and the
  expression engine that lets one parameter drive a whole sketch.
* [Profiles](profiles.md) — closed-region detection with exact areas and hole
  nesting.
* [Solving & goals](solving.md) — running the solver, reading its report,
  verification, interactive drag goals, and how the solver works.

The full API reference lives on
[pkg.go.dev](https://pkg.go.dev/github.com/lestrrat-3d/sketch). Design notes and
internals are in the sibling files under [`docs`](..).
