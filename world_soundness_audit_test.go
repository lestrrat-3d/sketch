package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// These tests lock in soundness invariants confirmed by an adversarial audit of the
// World / multi-sketch verification layer: World.Verify().Trustworthy() must be false
// whenever ANY component is compromised — an individually-untrustworthy sketch, an
// invalid shared parameter table, or an uncomputable plane frame. (The wrong-kind
// offset-plane case is covered by TestOffsetPlaneWrongKindRejected.)

func TestAuditWorldUntrustworthySketchPropagates(t *testing.T) {
	// A world that contains a sketch with conflicting constraints must not be
	// trustworthy — the per-sketch verdict propagates up.
	w := sketch.NewWorld()
	s, err := w.Sketch(w.XY())
	require.NoError(t, err)
	a, b := s.AddPoint(0, 0), s.AddPoint(10, 0)
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.AddConstraint(sketch.NewDistance(a, b, 20)) // conflict
	s.Solve()

	rep := w.Verify()
	require.False(t, rep.Sketches[0].Trustworthy(), "the conflicted sketch is untrustworthy")
	require.False(t, rep.Trustworthy(), "a world with an untrustworthy sketch is untrustworthy")
}

func TestAuditWorldSelfIntersectingProfilePropagates(t *testing.T) {
	// A self-intersecting profile (a figure-8 closed spline) in a world sketch makes the
	// whole world untrustworthy.
	w := sketch.NewWorld()
	s, err := w.Sketch(w.XY())
	require.NoError(t, err)
	_, err = s.AddClosedSpline(
		s.AddPoint(-3, -2), s.AddPoint(3, 2), s.AddPoint(-3, 2), s.AddPoint(3, -2))
	require.NoError(t, err)
	s.Solve()
	require.False(t, w.Verify().Trustworthy(), "a world with a self-intersecting profile is untrustworthy")
}

func TestAuditWorldCyclicParametersRejected(t *testing.T) {
	// A cycle in the shared parameter table must fail validation and gate the world.
	w := sketch.NewWorld()
	require.NoError(t, w.Params().Set("a", "b + 1"))
	require.NoError(t, w.Params().Set("b", "a + 1"))
	rep := w.Verify()
	require.False(t, rep.ParametersValid, "a parameter cycle is invalid")
	require.False(t, rep.Trustworthy(), "a world with invalid shared parameters is untrustworthy")
}

func TestAuditWorldUndefinedOffsetParameterRejected(t *testing.T) {
	// An offset plane bound to an undefined parameter cannot compute its frame; the
	// world must surface a plane error and be untrustworthy.
	w := sketch.NewWorld()
	op, err := w.OffsetPlane(w.XY(), 0)
	require.NoError(t, err)
	require.NoError(t, w.BindOffsetPlane(op, "nonexistent_param"))
	rep := w.Verify()
	require.NotEmpty(t, rep.PlaneErrors, "an undefined offset parameter is a plane error")
	require.False(t, rep.Trustworthy(), "a world with an uncomputable plane frame is untrustworthy")
}
