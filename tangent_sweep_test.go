package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A line tangent to an arc's full circle but not touching the arc's sweep is
// the false positive the oracle must reject. With the arc rigid (no freedom to
// bring the contact into the sweep), the constraint is unsatisfiable even
// though the line is a perfect tangent to the full circle.
func TestTangentLineArcOutOfSweepRejected(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b) // line y = 0

	center := s.AddPoint(5, 5)
	// Rigid radius-5 arc spanning the first quadrant [0°, 90°]. The line is
	// tangent to the full circle at (5,0) = 270°, outside the sweep.
	start := s.AddPoint(10, 5) // 0°
	end := s.AddPoint(5, 10)   // 90°
	arc := s.AddArc(center, start, end)
	s.Fix(center)
	s.Fix(start)
	s.Fix(end)
	s.AddConstraint(sketch.NewTangent(s.AddLine(a, b), arc))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "out-of-sweep tangent is unsatisfiable")

	rep := s.Verify()
	require.False(t, rep.Solvable, "the oracle must not bless an out-of-sweep tangent")
	require.Equal(t, sketch.Overconstrained, rep.Status)
	require.InDelta(t, 5, arc.R(), 1e-9, "the arc itself is unchanged (nothing was free to move)")
}

// The same line and circle, but the arc spans the bottom so the downward
// contact is within the sweep: now it is a genuine tangent and solves.
func TestTangentLineArcInSweepSatisfied(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)

	center := s.AddPoint(5, 5)
	s.Fix(center)
	start := s.AddPoint(1, 1) // ~225°
	end := s.AddPoint(9, 1)   // ~315°, sweep straddles 270°
	arc := s.AddArc(center, start, end)
	s.AddConstraint(sketch.NewTangent(s.AddLine(a, b), arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, arc.R(), 1e-6, "tangent contact lies within the sweep")
	require.True(t, s.Verify().Solvable)
}

// Arc↔circle tangency where the line of centers gives a contact direction
// outside the arc's sweep: even though the radii make the circles distance-
// tangent (d = r1 + r2 = 10), the sweep blocks it, so it is unsatisfiable.
func TestTangentCirclesArcOutOfSweepRejected(t *testing.T) {
	s := sketch.New()
	o := s.AddPoint(0, 0)
	s.Fix(o)
	circ := s.AddCircle(o, 8)
	s.AddConstraint(sketch.NewRadius(circ, 8))

	center := s.AddPoint(10, 0)
	start := s.AddPoint(12, 0) // 0°
	end := s.AddPoint(10, 2)   // 90°; contact toward the circle (180°) is out of sweep
	arc := s.AddArc(center, start, end)
	s.Fix(center)
	s.Fix(start)
	s.Fix(end)
	s.AddConstraint(sketch.NewTangentCircles(circ, arc, false))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "distance-tangent but the contact misses the arc")
	require.False(t, s.Verify().Solvable)
}

// Tangency at an arc endpoint (the contact sits exactly on the sweep boundary,
// the configuration fillets produce). It must solve cleanly and add no spurious
// degree of freedom or redundancy.
func TestTangentLineArcAtEndpointBoundary(t *testing.T) {
	s := sketch.New()
	center := s.AddPoint(0, 0)
	s.Fix(center)
	start := s.AddPoint(5, 0) // 0°
	end := s.AddPoint(0, 5)   // 90°
	arc := s.AddArc(center, start, end)
	s.Fix(start)
	s.Fix(end)

	// Vertical line through the arc's start point: tangent at the 0° endpoint.
	top := s.AddPoint(5, 8)
	line := s.AddLine(start, top)
	tc := sketch.NewTangent(line, arc)
	s.AddConstraint(tc)

	_, err := s.Solve()
	require.NoError(t, err, "endpoint tangency is satisfiable")
	require.InDelta(t, 5, top.X(), 1e-6, "the line stays tangent (vertical) at the endpoint")
	// The boundary contact keeps a nonzero gradient, so the tangent is a genuine
	// independent equation — not spuriously flagged redundant. (The rigid arc's
	// internal radius constraint is separately flagged, which is expected.)
	require.NotContains(t, s.RedundantConstraints(), tc, "endpoint tangent stays independent")
}

// Sweep enforcement is inherent to the constraint type, so it must survive a
// JSON round-trip: an out-of-sweep tangent stays unsolvable after reload.
func TestTangentArcSweepRoundTrip(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	center := s.AddPoint(5, 5)
	start := s.AddPoint(10, 5)
	end := s.AddPoint(5, 10)
	arc := s.AddArc(center, start, end)
	s.Fix(center)
	s.Fix(start)
	s.Fix(end)
	s.AddConstraint(sketch.NewTangent(s.AddLine(a, b), arc))

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var loaded sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &loaded))

	_, err = loaded.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "enforcement is preserved across serialization")
}

// A line through the arc's endpoint but NOT perpendicular to the radius there is
// not tangent — the oracle must reject it. (The earlier single-row clamp
// accepted any line through the endpoint, a false positive Codex flagged.)
func TestTangentLineThroughArcEndpointMustBeTangent(t *testing.T) {
	s := sketch.New()
	center := s.AddPoint(0, 0)
	start := s.AddPoint(5, 0) // 0deg, radius 5
	end := s.AddPoint(0, 5)   // 90deg
	arc := s.AddArc(center, start, end)
	s.Fix(center)
	s.Fix(start)
	s.Fix(end)

	// Line through the shared start point, fixed at a non-tangent (diagonal) angle.
	far := s.AddPoint(8, 5)
	s.Fix(far)
	s.AddConstraint(sketch.NewTangent(s.AddLine(start, far), arc))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "a non-perpendicular line through the endpoint is not tangent")
}

// With the far endpoint free, the shared-endpoint tangency is satisfiable: the
// solver pulls the line onto the perpendicular (the true tangent at the endpoint).
func TestTangentLineAtArcEndpointSolves(t *testing.T) {
	s := sketch.New()
	center := s.AddPoint(0, 0)
	start := s.AddPoint(5, 0)
	end := s.AddPoint(0, 5)
	arc := s.AddArc(center, start, end)
	s.Fix(center)
	s.Fix(start)
	s.Fix(end)

	far := s.AddPoint(8, 5) // free; pulled onto the vertical tangent at start
	s.AddConstraint(sketch.NewTangent(s.AddLine(start, far), arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, far.X(), 1e-6, "tangent at the endpoint is the vertical line x=5")
}

// Concentric circles of different radii are never tangent: the constraint must
// not read as satisfied (a degenerate false zero Codex flagged).
func TestTangentConcentricCirclesRejected(t *testing.T) {
	s := sketch.New()
	o := s.AddPoint(0, 0)
	s.Fix(o)
	c1 := s.AddCircle(o, 3)
	c2 := s.AddCircle(o, 5) // same center
	s.AddConstraint(sketch.NewRadius(c1, 3), sketch.NewRadius(c2, 5))
	s.AddConstraint(sketch.NewTangentCircles(c1, c2, false))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "concentric circles of different radii are not tangent")
}

// The interior-tangency sweep slack is recomputed on load (never serialized);
// the reloaded sketch still solves.
func TestTangentArcInteriorRoundTrip(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	center := s.AddPoint(5, 5)
	s.Fix(center)
	start := s.AddPoint(1, 1)
	end := s.AddPoint(9, 1)
	arc := s.AddArc(center, start, end)
	s.AddConstraint(sketch.NewTangent(s.AddLine(a, b), arc))
	if _, err := s.Solve(); err != nil {
		t.Fatalf("solve: %v", err)
	}
	require.InDelta(t, 5, arc.R(), 1e-6)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var loaded sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &loaded))
	_, err = loaded.Solve()
	require.NoError(t, err)
	require.True(t, loaded.Verify().Solvable)
}

// Removing an interior arc tangency retires its sweep slack; the sketch still
// solves cleanly afterwards (no dangling free variable).
func TestTangentArcSlackRetiredOnRemoval(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	center := s.AddPoint(5, 5)
	s.Fix(center)
	start := s.AddPoint(1, 1)
	end := s.AddPoint(9, 1)
	arc := s.AddArc(center, start, end)
	tc := sketch.NewTangent(s.AddLine(a, b), arc)
	s.AddConstraint(tc)
	if _, err := s.Solve(); err != nil {
		t.Fatalf("solve: %v", err)
	}

	require.True(t, s.RemoveConstraint(tc))
	require.NotContains(t, s.Constraints(), tc)
	_, err := s.Solve()
	require.NoError(t, err)
}

// CheckConstraint probes a candidate before it is committed (allocVars has not
// run), so an interior arc tangent must not dereference its unallocated slack.
func TestTangentArcCheckConstraintNoPanic(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	center := s.AddPoint(5, 5)
	s.Fix(center)
	start := s.AddPoint(1, 1)
	end := s.AddPoint(9, 1)
	arc := s.AddArc(center, start, end)
	require.NotPanics(t, func() {
		_ = s.CheckConstraint(sketch.NewTangent(s.AddLine(a, b), arc))
	})
}

// Coincident equal-radius circles are not internally tangent (they overlap, not
// touch); the constraint must not read as satisfied.
func TestTangentInternalEqualRadiusConcentricRejected(t *testing.T) {
	s := sketch.New()
	o := s.AddPoint(0, 0)
	s.Fix(o)
	c1 := s.AddCircle(o, 4)
	c2 := s.AddCircle(o, 4) // same center, equal radius
	s.AddConstraint(sketch.NewRadius(c1, 4), sketch.NewRadius(c2, 4))
	s.AddConstraint(sketch.NewTangentCircles(c1, c2, true))
	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "coincident equal circles are not internally tangent")
}

// A zero-length line sharing the arc's endpoint has no direction and is not
// tangent — it must be rejected, not read as a degenerate zero.
func TestTangentZeroLengthLineSharedRejected(t *testing.T) {
	s := sketch.New()
	center := s.AddPoint(0, 0)
	start := s.AddPoint(5, 0)
	end := s.AddPoint(0, 5)
	arc := s.AddArc(center, start, end)
	s.Fix(center)
	s.Fix(start)
	s.Fix(end)
	tip := s.AddPoint(5, 0) // coincident with start: zero-length line
	s.Fix(tip)
	s.AddConstraint(sketch.NewTangent(s.AddLine(start, tip), arc))
	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "a zero-length line is not tangent")
}

// A near-degenerate (sub-threshold length) line tangent to an arc must not panic
// the finite-difference Jacobian: the residual arity is constant regardless of
// where perturbations push the line length.
func TestTangentArcNearDegenerateLineNoPanic(t *testing.T) {
	s := sketch.New()
	center := s.AddPoint(0, 0)
	s.Fix(center)
	start := s.AddPoint(-3, -4)
	end := s.AddPoint(3, -4)
	arc := s.AddArc(center, start, end)
	p1 := s.AddPoint(0, -5)
	p2 := s.AddPoint(1e-9, -5) // length at the degeneracy threshold
	s.AddConstraint(sketch.NewTangent(s.AddLine(p1, p2), arc))
	require.NotPanics(t, func() { _, _ = s.Solve() })
}

// Committing the same constraint handle twice is a no-op (deduplicated), so it
// never double-counts the residual or leaks a second sweep slack: one commit,
// one removal clears it.
func TestTangentArcDoubleAddDeduped(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	center := s.AddPoint(5, 5)
	s.Fix(center)
	start := s.AddPoint(1, 1)
	end := s.AddPoint(9, 1)
	arc := s.AddArc(center, start, end)
	tc := sketch.NewTangent(s.AddLine(a, b), arc)
	s.AddConstraint(tc)
	s.AddConstraint(tc) // deduped: no second occurrence

	if _, err := s.Solve(); err != nil {
		t.Fatalf("solve: %v", err)
	}
	require.InDelta(t, 5, arc.R(), 1e-6)
	require.True(t, s.RemoveConstraint(tc), "the single occurrence is removed")
	require.False(t, s.RemoveConstraint(tc), "no duplicate occurrence remains")
}

// Removing then re-adding the same arc-tangent handle must allocate a FRESH
// sweep slack (the retired one is frozen); the re-added constraint still
// enforces in-sweep tangency after the geometry moves.
func TestTangentArcRemoveReAdd(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	center := s.AddPoint(5, 5)
	s.Fix(center)
	start := s.AddPoint(1, 1)
	end := s.AddPoint(9, 1)
	arc := s.AddArc(center, start, end)
	tc := sketch.NewTangent(s.AddLine(a, b), arc)
	s.AddConstraint(tc)
	if _, err := s.Solve(); err != nil {
		t.Fatalf("solve: %v", err)
	}

	require.True(t, s.RemoveConstraint(tc))
	s.AddConstraint(tc) // re-add: fresh slack, not the retired one

	start.MoveTo(2, 2) // move the arc; the fresh slack must track it
	end.MoveTo(8, 2)
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, arc.R(), 1e-6, "re-added tangent enforces in-sweep tangency with a fresh slack")
}

// Shared-endpoint arc tangency must honor internal vs external: two arcs
// externally tangent at their shared point satisfy external, not internal.
func TestTangentSharedArcsRespectInternal(t *testing.T) {
	build := func(internal bool) error {
		s := sketch.New()
		c1 := s.AddPoint(0, 0)
		s1 := s.AddPoint(0, 5)
		p := s.AddPoint(5, 0) // shared contact, between the centers -> external
		a1 := s.AddArc(c1, s1, p)
		c2 := s.AddPoint(10, 0)
		e2 := s.AddPoint(10, 5)
		a2 := s.AddArc(c2, p, e2) // shares p
		for _, pt := range []*sketch.Point{c1, s1, p, c2, e2} {
			s.Fix(pt)
		}
		s.AddConstraint(sketch.NewTangentCircles(a1, a2, internal))
		_, err := s.Solve()
		return err
	}
	require.NoError(t, build(false), "the arcs are externally tangent at the shared point")
	require.ErrorIs(t, build(true), sketch.ErrNotConverged, "they are not internally tangent")
}

// A feasible interior tangent SEEDED out-of-sweep must still solve: the solver
// rotates the free arc to bring the contact into the sweep. This exercises the
// nonzero slack seed — a zero seed leaves the sweep row's ∂/∂w = 0, a flat spot
// that traps the solve.
func TestTangentArcFeasibleSeededOutOfSweep(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b) // line y = 0

	center := s.AddPoint(5, 5)
	s.Fix(center)
	// Arc seeded spanning the first quadrant [0,90deg]; the downward tangent
	// contact (270deg) is OUT of sweep at the seed, but the free endpoints let
	// the solver rotate the arc so the contact comes in-sweep.
	start := s.AddPoint(8, 5) // 0deg
	end := s.AddPoint(5, 8)   // 90deg
	arc := s.AddArc(center, start, end)
	s.AddConstraint(sketch.NewTangent(s.AddLine(a, b), arc))

	_, err := s.Solve()
	require.NoError(t, err, "the feasible in-sweep tangent is reachable from the out-of-sweep seed")
	require.InDelta(t, 5, arc.R(), 1e-6)
	require.True(t, s.Verify().Solvable)
}
