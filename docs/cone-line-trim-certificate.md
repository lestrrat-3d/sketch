# Cone and line trim certificate

`Sketch.CertifyConeLineCrossing` certifies one crossing between an authored
`*Line` and the section of a right circular cone on the sketch plane. The
caller passes the cone apex, axis, and slope in world coordinates. The method
reads the sketch's current plane frame and line endpoints.

The method expresses the line in world coordinates as `Q(t)` relative to the
cone apex. It forms the quadratic

```text
F(t) = |Q(t) × Axis|² − Slope² (Q(t) · Axis)²
```

using exact rational arithmetic over the stored binary64 inputs. Both line
endpoints must lie on the cone's positive axial half. Nonzero, opposite signs
of `F(0)` and `F(1)` prove that the quadratic has exactly one simple root in
the open line interval. A tangent, miss, two crossings, or endpoint contact
returns `ErrConeLineCrossing`.

The method bisects that root with rational arithmetic and rounds the final
bounds outward. `TExact()` reports true while the line, sketch revision, and
plane frame still match and the returned interval encloses the true parameter within
machine precision. `T()` is a floating approximation inside `TBounds()`.
`Point()` evaluates the authored line at `T()` in sketch coordinates.

The certificate records the exact line handle, sketch revision, plane frame, and a copy of
the cone parameters. A consumer must compare `Cone()` to the cone of the body
it is joining, check `TExact()`, and check `Line()` against the boundary edge
it intends to trim. For a finite cone face, the consumer must check that the
portion it keeps lies within that face's axial range. A point-loft ray may
meet the infinite cone on its far plane outside the finite face and still
meet the finite face after end trimming. A cone with nonzero radius
at its reported origin must be converted to its actual apex before calling
this API. The certificate proves one
line contact; it does not change `Sketch.Profiles()` or certify contacts of
other entities in the scene.

The real 8/8 bevel gear's root cone intersects its two straight tooth-root
connectors at line parameters approximately `0.9007081542485348` and
`0.09929184575146503`. Both fitted flanks and the tip arc are whole edges.
The gallery producer probe passes the actual blank cone to this API and checks
both certificates. The two far-plane contacts have `x=4.928387493693819`.
The toe and heel tools retain these rays from `x=3.2559223176554566` to
`x=4.883883476483184`, matching the blank's finite root segment endpoints.

## Whole-curve side certificate

`Sketch.CertifyConeCurveSide` checks a whole `*Line`, `*FitSpline` or `*Arc`
against the same cone source. It returns `ConeOutside` when the cone implicit
function is strictly positive over the complete algebraic source curve, or
`ConeInside` when strictly negative. A curve that touches the cone, crosses it, or is an arc of a half turn
or more returns `ErrConeCurveSide`. The certificate exposes its
exact `Entity()` handle and copied `Cone()`. `Side()` returns zero after a
source, sketch, or plane-frame change. `ImplicitMargin()` returns an exact
rational lower bound on the absolute cone function, in squared length units
times the squared axis length.

For each fit-spline span, the method derives the exact rational cubic Bézier
controls from Sketch's stored natural-cubic interpolant. The control hull
contains the whole span. It tests the cone function over the hull's coordinate
box using rational interval arithmetic and bisects a hull when the box is too
wide to decide. A depth limit of 12 refuses an unresolved span.
The complete fit proof visits at most 2,048 hull boxes across all spans.

The arc proof has two stages. The first covers a short counter-clockwise arc
from the upper-left to the lower-left quadrant through the negative local X
direction. It bounds the arc inside one coordinate box by checking the radius
and endpoint rays as exact rationals, then tests that box with the same cone
interval arithmetic. The real 8/8 tooth's two whole fitted flanks and tip arc
certify `ConeOutside`; its whole root arc certifies `ConeInside`. An arc this
stage certifies keeps its certificate and margin.

The second stage covers any counter-clockwise arc, at any position and
orientation, whose sweep is strictly less than a half turn. The bevel root
fillet (an arc tangent to a radial flank and to a run-out line) needs it. The
carrier is the exact circle `Arc.CertifiedSpanBounds` encloses: `Center` and
`Start` fix the radius and `End` fixes only the final direction. The sweep test
is the exact sign of the cross product of the start and end rays, so a zero
sweep, a half turn or more, and a clockwise reading all refuse.

The proof halves the angular wedge between the two rays, splitting along a ray
whose position inside the wedge is proved by exact cross products. For each
wedge it encloses the unit direction by exact rational square-root bounds. A
unit-circle coordinate is monotone between its axis extremes, so it ranges
between the two end values unless the wedge holds an axis direction, whose
coordinate then reaches exactly plus or minus one. The radius interval times
that range gives a coordinate box, rounded outward to binary64 bounds, which
the exact `boxSide` test decides. Every wedge must give the same strict side,
and the margin is the smallest one. A wedge still undecided after 12 halvings,
or any proof needing more than 2,048 boxes, refuses. An arc that touches the
section, or hugs it closer than 12 halvings can resolve, refuses.

No solve tolerance is part of this proof. It reads the stored `Start`, `End` and
`Center` coordinates as exact rationals and encloses the carrier they define, so
it does not depend on how tightly the sketch solved the arc's radii. A consumer
that checks an arc's start and end radii against each other (decad allows 1024
ulps) must tighten its own `WithTolerance` for that.

The fit certificate describes the algebraic interpolant defined by the stored
binary64 fit data. `FitSpline.Eval` uses floating arithmetic and can round a
near-contact point onto the cone even when the algebraic curve has a positive
margin. A consumer must compare `ImplicitMargin()` with its bound for that
evaluation or mesh displacement and with any difference between the reported
cone and the finite body meridian before using the sign to change topology.
