# Certified Enclosure of a Solved Configuration — Design

Status: **implemented** (`enclose.go`, `enclose_system.go`, `interval.go`,
`interval_trig.go`; tests in `enclose_test.go` and
`interval_trig_internal_test.go`; example in
`examples/sketch_enclose_example_test.go`). Requested by decad, which must
admit the pose of a closed-loop mechanism (a four-bar, a slider-crank) only on
a claim about the exact solution, never on a float residual.

## The problem

`Solve` returns float coordinates whose residual norm is at most the
tolerance (`1e-10` by default). `Verify` adds a rank verdict and a
conditioning threshold, and `ProbeConfigurations` can show that a second
configuration exists. None of them bounds the distance between the float
coordinates and the exact solution, and none of them proves that the solution
near the float coordinates is the only one there. A consumer that builds a
proof on top of a pose needs both facts as stated bounds.

`Sketch.Enclose` publishes them. For one driving dimension and a range of its
value, it returns intervals that contain the exact solution's coordinates
and the exact measured value of every driven dimension, together with a proof
that each box holds exactly one solution for every value in its range.

## API

```go
func (s *Sketch) Enclose(ctx context.Context, driver Dimension, lo, hi float64,
	options ...EncloseOption) (*Enclosure, error)
```

- `driver` is a driving `Angle`, `Distance`, `HorizontalDistance` or
  `VerticalDistance` committed to the sketch.
- `lo` and `hi` are base-unit values (mm or rad). Each float64 is read as the
  exact rational it represents; `lo == hi` asks about one value. Base units
  are required because converting degrees to radians in float64 is inexact.
- `WithContinuation(prev)` seeds the call from where `prev` ended and proves
  that the two enclosures follow one branch. `WithMaxPieces(n)` caps the
  number of pieces (default 4096).
- `WithTargetRange(d, lo, hi)` makes driving dimension `d`'s target the
  interval `[lo, hi]`, and `WithFixedBox(p, x, y)` makes fixed point `p`'s
  position the box `x × y`. See "Ranged targets and fixed boxes".

An `Enclosure` is an ordered list of `EnclosurePiece`s that together cover
`[lo, hi]`, adjacent pieces sharing an endpoint. A piece reports its
sub-range (`Range`), a box per point (`PointBox`), and an interval per driven
dimension (`Driven`). The enclosure reports the hull of those readings across
its pieces, and `IsStale` reports whether the sketch changed since the call.

Every `Interval` endpoint is outward rounded: read each float64 as the exact
rational it is, and the exact quantity lies between them.

## What a piece claims

For every driving value `q` in the piece's sub-range, with every other
dimension at its float64 target in base units (or at every value of its
`WithTargetRange` range) and every fixed point at its coordinates (or at every
position in its `WithFixedBox` box):

1. Exactly one exact solution of the constraint equations lies in the piece's
   box.
2. That solution moves continuously with `q` and with the ranged values.
3. Every coordinate it takes lies in the box, and every driven dimension's
   exact measured value lies in the piece's interval for it.

Adjacent pieces hold the same solution at their shared value (see "Tying
pieces together"), so across the whole range the solution is one continuous
function of `q`. Uniqueness is claimed inside each piece's box, never inside
the hull of several boxes. Nothing is claimed about solutions outside the
boxes.

A driven angle's interval is a reading mod 2π. The first piece is shifted by
whole turns to lie within half a turn of the dimension's current target, and
each later piece within half a turn of the piece before it, so the hull's
width is how far the angle travels across the range.

## The certified equations

The Krawczyk test needs an interval enclosure of each equation and of its
partial derivatives, so `Enclose` does not use the solver's residuals or its
central-difference Jacobian. It restates each supported constraint as an
equation with the same zero set inside the tested box and a closed-form
derivative:

| Constraint | Certified equation |
|---|---|
| coincident, horizontal, vertical, horizontal/vertical points | the coordinate differences |
| horizontal/vertical distance | `x2 − x1 − d` / `y2 − y1 − d` |
| point-on-line, collinear, parallel | cross product of the two direction vectors |
| perpendicular | dot product of the two direction vectors |
| distance | `|p − q|² − d²`, with `d > 0` |
| angle | `cross·cos θ − dot·sin θ`, with `cross·sin θ + dot·cos θ > 0` over the box |

The solver divides the cross and dot products by `norm()`, which never
returns zero, so its residual is zero exactly when the product is. A
distance residual `|p − q| − d` with `d > 0` is zero exactly when the squared
form is. A signed angle residual is zero exactly when the direction
`(dot, cross)` points along `(cos θ, sin θ)`, which is the equation together
with the inequality. The inequality is checked over the whole box, and it
also excludes `(cross, dot) = (0, 0)`, the one point where `atan2` returns a
convention instead of a direction.

Any other constraint kind anywhere in the sketch refuses the call with
`ErrUncertifiedConstraint`. A driven dimension must be an angle or one of the
three distances.

## The Krawczyk test

For the square system `F(x, q) = 0` (one equation per free variable), a float
solution `x̃` at the piece's midpoint `q̃`, and a float approximate inverse `Y`
of the Jacobian at `(x̃, q̃)`, `krawczyk` evaluates

```text
K(X) = x̃ − Y·F(x̃, Q) + (I − Y·F_x(X, Q))·(X − x̃)
```

in interval arithmetic over the piece's range `Q`. For any single `q` in `Q`,
the ordinary Krawczyk image is contained in `K(X)`. If `K(X)` lies strictly
inside `X`, every such image does, so for each `q` the system has exactly one
zero in `X` and it lies in `K(X)`. The same inclusion proves `Y` and every
Jacobian in `F_x(X, Q)` nonsingular, so the implicit function theorem gives
continuity in `q`. The published box is `K(X)`.

`X` is found by ε-inflation. It starts from `x̃ − Y·F(x̃, Q)`, and each pass
widens the previous image by 10% plus `1e-15·(1 + |x̃|)` and always keeps
`x̃` inside. After 20 passes without inclusion the piece fails.

## Ranged targets and fixed boxes

decad needs these options when the value a mechanism should have is not a
float64. A bar between two given pins has an irrational length, and a pin
rotated into a sketch plane gets rounded coordinates. Without the options the
enclosure describes the mechanism with the rounded floats.

`WithTargetRange(d, lo, hi)` applies to a driving `Distance` (`lo > 0`),
`HorizontalDistance`, `VerticalDistance` or `Angle` (within ±64 rad), other
than the driver. The equation of `d` reads the whole interval: a distance's
`d²` term becomes the square of the interval, an offset becomes the interval,
and an angle's `sin θ` and `cos θ` are enclosed over the interval with
`sinCosRange`. Those terms enter `F(x̃, Q, T)`, `F_x(X, Q, T)` and the angle's
side condition.

`WithFixedBox(p, x, y)` applies to a point `Fix` has grounded. Its two
variables stay out of the Jacobian's columns, as they do for every other fixed
point. Inside `krawczyk` they read their intervals in `F(x̃, ·)`, in the box
`X` that `F_x` is evaluated over, and in the published box `K`. The
approximate inverse `Y` is still built at the float point.

For any one choice of target `t` and position `p`, the single-value Krawczyk
image is contained in the image computed over the whole intervals. If that
image lies strictly inside `X`, each choice has exactly one solution in `X`.
The implicit function theorem then makes the solution continuous in `q`, `t`
and `p` together. The piece ties also hold per choice: the point box at a
shared value `b` covers every `(b, t, p)`, so for each choice both neighbours
hold the same solution at `b`.

The pieces split only the driving range. A ranged target or a fixed box keeps
its whole interval in every piece, so a wide one widens every box. A width the
test cannot close refuses with `ErrNotCertified`, as a wide `Q` does.

The float solves run with each ranged target at its interval's midpoint and
each boxed point at its box's center. `Enclose` writes a midpoint only when it
differs from the current value. A zero-width range on a dimension's own target
therefore reruns the call without the option, bit for bit.
`WithContinuation` requires the same ranges and boxes as the enclosure it
continues, because that enclosure's end box covers only its own.

| Request | Refusal |
|---|---|
| a target range on the driver, a driven dimension, a nil dimension, or another sketch's dimension | `ErrNotCertified` |
| `lo > hi`, a NaN or infinite endpoint, or a distance range with `lo <= 0` | `ErrNotCertified` |
| an angle range outside ±64 rad | `ErrNotCertified` |
| a dimension kind with no range form | `ErrUncertifiedConstraint` |
| a fixed box on a point that is not fixed, is nil, or belongs to another sketch | `ErrNotCertified` |
| a fixed box interval that is reversed or not finite | `ErrNotCertified` |
| the same dimension or point named twice | `ErrNotCertified` |
| a continuation whose ranges or boxes differ from the continued enclosure's | `ErrNotCertified` |

On the crank-rocker at `θ2 = 90°`, a coupler range of `[79, 81]` gives a
follower interval 1.69° wide. The exact follower moves 1.64° over that range.

## Rounding

Go has no rounding-mode control. Each interval operation (`interval.go`)
computes its endpoints in round-to-nearest and steps one float outward with
`math.Nextafter`. IEEE 754 rounds `+`, `−`, `×`, `÷` and `sqrt` correctly, so
the step always encloses the exact result. Each product is written as an
explicit `float64(…)` conversion, which the Go spec defines as the way to
forbid fusing a multiply and an add.

`math.Sin`, `math.Cos` and `math.Atan2` are not trusted for bounds
(`interval_trig.go`):

- `sinCosPoint` sums the Taylor series of `sin x` and `cos x` in 256-bit
  fixed-point integers, tracking an integer bound on the error each term
  carries. It stops at a term past the series' peak that is below `2^-110`,
  adds that term as the tail bound, and converts outward to float64. It
  refuses `|x| > 64`.
- `sinCosRange` encloses sine and cosine over an interval from the endpoint
  values, adding `±1` wherever a multiple of `π/2` might lie inside. That test
  uses a 40-decimal bracket of `π`.
- `atan2Point` brackets a direction instead of computing it. It takes
  `math.Atan2` as an estimate, and accepts `[lo, hi]` around it only when the
  exact sines and cosines of `lo` and `hi` show the vector strictly between
  them.
- `atan2Box` encloses the direction over a rectangle from its four corners.
  It refuses a rectangle that touches the origin.

`interval_trig_internal_test.go` checks the fixed-point sums against an
exact-rational evaluation, and the interval operations against exact rational
products.

## Pieces

`run` splits `[lo, hi]` adaptively. It solves at `lo` from the sketch's
current geometry, checks that the system is square and of full rank, and
certifies a point box at `lo`. It then tries pieces left to right. Each
attempt solves at the piece's midpoint (warm-started from the previous
endpoint), runs the parametric test, and solves and certifies a point box at
the piece's right end. A failed attempt halves the piece. A success doubles
the next attempt. A piece narrower than `2^-30` of the range, or a piece count
past the budget, refuses the call with `ErrNotCertified`.

### Tying pieces together

A piece is accepted only when the certified point boxes at both of its ends
lie inside its uniqueness box `X`. The box at the shared value `b` then holds
a solution at `b` that lies inside both neighbours' uniqueness boxes. Each of
them holds exactly one solution at `b`, so both hold this one, and the two
pieces continue one branch. `WithContinuation` reuses the same check: the
previous enclosure's point box at its right end must lie inside the new first
piece's uniqueness box.

### Tightness

A Krawczyk box over a sub-range overshoots the path the solution takes by a
term that shrinks with the square of the sub-range's width. A consumer charges
the whole width as travel, so `tightEnough` also splits a certified piece when
any free variable's box reaches beyond the hull of its two endpoint boxes by
more than 5% of that hull's width plus `1e-7` of the coordinate scale. The
excess also counts a variable's real excursion past its endpoints (a follower
turning back inside the piece), which shrinks the same way, so the splitting
ends.

On decad's fixtures the rule costs pieces and buys width:

| Fixture | Pieces | Hull width | Exact travel |
|---|---|---|---|
| slider-crank slider, `[30°, 60°]` (22.78 mm in 2 pieces without the rule) | 32 | 13.912 mm | 13.898 mm |
| crank-rocker follower, `[0°, 90°]` | 389 | 11.788° | 11.788° |
| crank-rocker follower, full turn | 1416 | 50.803° | 50.803° |

The full turn takes about 0.4 s.

## Refusals

A refusal returns a nil `*Enclosure` and an error wrapping one sentinel:

| Cause | Sentinel |
|---|---|
| the float solve at `lo` did not converge | `ErrNotConverged` |
| degrees of freedom remain with the driver held | `ErrUnderconstrained` |
| more equations than unknowns | `ErrRedundant` |
| an unsupported constraint kind is present | `ErrUncertifiedConstraint` |
| no piece could be certified over some sub-range, a driven value could not be enclosed, or the call is invalid | `ErrNotCertified` |
| non-finite geometry or a foreign operand | `ErrNonFiniteGeometry`, `ErrForeignHandle` |
| the context ended | `ctx.Err()` |

`Enclose` does not prove that a fold exists. A range holding a fold refuses
with `ErrNotCertified`, because no piece shrinks past the fold.

## Side effects and determinism

`Enclose` moves the sketch's variables, the driver's target and every ranged
target while it runs and restores them exactly before it returns, on success
and on refusal.
`Sketch.Revision` is unchanged afterwards. Two calls on the same state with
the same arguments return bit-identical enclosures, because every step is a
deterministic float or integer computation from the same inputs.

`IsStale` compares a fingerprint taken at the call with a fresh one. The
fingerprint covers `Sketch.Revision`, the grounding flags, and per constraint
its kind, its operands' variables, and a dimension's target and driven flag.
Changing a dimension's value makes a held enclosure stale before any solve.

## Consumer notes

- The boxes enclose the exact solution, not `Solve`'s float coordinates.
  Those satisfy the residual only to the solver's tolerance, so they can lie
  just outside a box that is `1e-13` wide. A float coordinate outside a box
  therefore does not disprove the claim.
- The table values in decad's hand-off are rounded to 4 decimals (degrees) or
  6 decimals (mm). A point box is narrower than that rounding, so containment
  of a table value holds only up to it.
- At `θ2 = 87.7076°` the non-Grashof four-bar is past its fold, which is at
  `acos(0.04) = 87.707557°`. No configuration exists there, so the point ask
  refuses with `ErrNotConverged`.

## Not covered

- Circles, arcs, ellipses, splines and their constraints have no certified
  form yet. Each would need a restated equation and its derivatives.
- Fold detection: a proof that a range holds a fold, as opposed to a refusal.
- Uniqueness over the hull of several pieces.
- Splitting a ranged target or a fixed box. Only the driving range is split
  into pieces.
- A tolerance option: the float solves use the solver's default tolerance and
  iteration budget, which only decide whether a certificate is attempted.
