# Certified arc span bounds

`Arc.CertifiedSpanBounds(lo, hi)` encloses the ideal circular carrier over
normalized counter-clockwise parameters `0 <= lo <= hi <= 1`. It returns two
outward-rounded `Interval` values in sketch-local millimetres. The method
reads the live Sketch `Arc`; callers must separately authenticate any profile
edge, partial range, or source revision they intend to use.

## Carrier and parameter

The circle has the exact float coordinates of `Center` as its centre and the
exact distance from `Center` to `Start` as its radius. `End` names its final
direction. Parameter `t` follows the counter-clockwise angular sweep from
`Start` to `End`: angle `start + t * sweep`. This is the circular carrier used
by `Arc.Geometry()`. The pinned `End` may differ radially from the ideal carrier;
callers that include the pinned endpoint must bound that difference separately.

The method refuses a removed or stale Arc, a non-finite defining point, a
zero or undecidable radius, a full turn or ambiguous sweep, and an invalid
parameter interval. It returns no box on refusal. The box does not certify
profile topology, an intersection parameter, or that another curve misses
the arc.

## Bound construction

- Subtract centre from both defining points with outward-rounded intervals.
- Bracket each direction with `atan2Box`; shift `End` by a whole turn and
  accept the branch only when its sweep is strictly inside `(0, 2π)`.
- Bound the radius by the interval square root of the start-vector squares.
- Form the angular slice with interval arithmetic, then call `sinCosRange`.
  That function includes every possible sine or cosine extremum in the slice.
- Multiply the trigonometric intervals by the radius and add the centre with
  outward rounding. Refuse any non-finite output.

The float `Arc.Sweep()` chooses only the candidate whole-turn shift. The
interval comparison proves the chosen branch; a float angle never grants a
bound. `CertifiedSpanBounds` performs no Sketch solve and does not change its
revision.
