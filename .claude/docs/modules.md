# External modules and extractable packages — `r3`, `units`, `param`

Detail moved out of CLAUDE.md. Read before touching 3D frames/planes, unit conversion, or the parameter/expression engine — each has a dependency-direction rule that must not be broken.

## Question router

| Your question | Section |
|---|---|
| What may live in `r3`? | The `r3` module |
| Why must a frame be orthonormal? | The `r3` module |
| Where does unit conversion happen? | The `units` module |
| What is a unit `Kind` and when is `BaseUnit` false? | The `units` module |
| What may `param` import? | The `param` package |

Navigation only — the sections below are the authority.

## The `r3` module (an external dependency)

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

## The `units` module (an external dependency)

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

## The `param` package (slated for extraction)

`param/` is a standalone parameter/expression engine: a `Table` of named
parameters holding literals or expressions (`width = height * 1.5`), with a
lexer/parser/evaluator, functions, constants, forward references and cycle
detection. **It must not import anything from the `sketch` package or rely on
the rest of the repo** — it is intended to move into its own module/repository
later, so the dependency arrow only ever points *into* it. Its production code
depends on the standard library plus the `units` module and nothing else (tests
may use `testify/require`); keep it independently testable.
