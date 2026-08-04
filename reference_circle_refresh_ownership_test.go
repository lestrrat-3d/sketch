package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// RefreshReferenceCircle used to screen its handle with `c.s != s`, which only
// tests which sketch a handle NAMES, not whether it is still LIVE (in range,
// and still the entity that slot holds). A dead or orphaned handle can keep
// `c.s == s` while its stored radius-variable index no longer names what it
// used to — so the weak screen let a removed, or index-orphaned, circle
// through instead of refusing it. The fix screens with `Sketch.ownsEntity`
// (the same predicate the sibling `RefreshReference` mirrors via `owns`, and
// that grounding/tools/Verify all share) and reports [sketch.ErrForeignEntity],
// keeping the live-but-not-reference check as a separate, still-distinguishable
// [sketch.ErrNotReference].

// refCircleSketch returns a sketch holding an ordinary "victim" point (used to
// prove a refused call touches nothing) plus a reference circle of the given
// radius, and the circle's handle.
func refCircleSketch(t *testing.T, r float64) (*sketch.Sketch, *sketch.Point, *sketch.Circle) {
	t.Helper()
	s := newSketch(t)
	victim := s.CreatePoint(10, 11)
	center := s.CreateReferencePoint(0, 0, "hole")
	c, err := s.CreateReferenceCircle(center, r, "hole")
	require.NoError(t, err)
	return s, victim, c
}

// pointCoords snapshots every point's coordinates, in id order, so a refused
// call can be checked to have moved none of them.
func pointCoords(s *sketch.Sketch) [][2]float64 {
	out := make([][2]float64, 0, len(s.Points()))
	for _, p := range s.Points() {
		out = append(out, [2]float64{p.X(), p.Y()})
	}
	return out
}

// A removed circle is dead: RemoveEntity retired its radius variable (marked
// fixed, never reclaimed) but the weak `c.s != s` screen still passed it,
// silently "succeeding" against that retired slot.
func TestRefreshReferenceCircleRejectsRemoved(t *testing.T) {
	s, victim, c := refCircleSketch(t, 5)
	require.True(t, s.RemoveEntity(c))
	before := pointCoords(s)

	err := s.RefreshReferenceCircle(c, 99)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Equal(t, before, pointCoords(s), "the refused call touched no live point")
	require.InDelta(t, 10, victim.X(), 1e-9)
	require.InDelta(t, 11, victim.Y(), 1e-9)
}

// A circle of a different sketch: c.s names the other sketch, so the weak
// screen already caught this — but reported ErrNotReference rather than
// ErrForeignEntity.
func TestRefreshReferenceCircleRejectsForeignSketch(t *testing.T) {
	s, victim, _ := refCircleSketch(t, 5)
	_, _, foreign := refCircleSketch(t, 7)
	before := pointCoords(s)

	err := s.RefreshReferenceCircle(foreign, 99)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Equal(t, before, pointCoords(s), "the refused call touched no live point")
	require.InDelta(t, 10, victim.X(), 1e-9)
	require.InDelta(t, 11, victim.Y(), 1e-9)
	require.InDelta(t, 7, foreign.R(), 1e-9, "the foreign circle itself is untouched")
}

// A nil *Circle. RefreshReferenceCircle's parameter is the concrete *Circle
// type (not the Entity interface), so there is only one nil shape a caller can
// pass here — but internally it reaches Sketch.ownsEntity as a non-nil Entity
// interface wrapping a nil *Circle (a typed nil), the same case
// TestReferenceVerifyTypedNilOperand exercises through WorldPolyline.
func TestRefreshReferenceCircleRejectsNil(t *testing.T) {
	s, victim, _ := refCircleSketch(t, 5)
	before := pointCoords(s)

	var nilCircle *sketch.Circle
	err := s.RefreshReferenceCircle(nilCircle, 99)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Equal(t, before, pointCoords(s), "the refused call touched no live point")
	require.InDelta(t, 10, victim.X(), 1e-9)
	require.InDelta(t, 11, victim.Y(), 1e-9)
}

// A live, non-reference circle must still be refused, and still distinguished
// from the ownership failures above: ErrNotReference, not ErrForeignEntity.
func TestRefreshReferenceCircleStillRejectsNonReference(t *testing.T) {
	s := newSketch(t)
	ordinary := s.CreateCircle(s.CreatePoint(0, 0), 4)
	before := pointCoords(s)

	err := s.RefreshReferenceCircle(ordinary, 99)
	require.ErrorIs(t, err, sketch.ErrNotReference)
	require.NotErrorIs(t, err, sketch.ErrForeignEntity)
	require.Equal(t, before, pointCoords(s), "the refused call touched no live point")
	require.InDelta(t, 4, ordinary.R(), 1e-9)
}

// A handle held across an in-place UnmarshalJSON of THIS sketch's own document.
// UnmarshalJSON resets the struct (*s = Sketch{...}) and rebuilds every entity
// as a fresh instance, so an old handle's c.s == s survives while c.id/c.ri no
// longer name what they used to — the weak `c.s != s` screen cannot see this at
// all, since the sketch pointer never changed.
func TestRefreshReferenceCircleRejectsOrphanedHandleAfterReload(t *testing.T) {
	t.Run("vector long enough to alias", func(t *testing.T) {
		// Reloading the SAME bytes recreates every entity in the same order, so
		// c.ri still lands in range — on the REBUILT circle's own radius
		// variable, a different live object at the same slot. The weak screen
		// passed this straight through, silently writing the new circle's
		// radius via the dead handle while clearing the DEAD object's stale
		// bit rather than the live one's.
		s, _, c := refCircleSketch(t, 5)

		data, err := json.Marshal(s)
		require.NoError(t, err)
		require.NoError(t, s.UnmarshalJSON(data))

		live, ok := s.Entities()[0].(*sketch.Circle)
		require.True(t, ok)
		require.NotSame(t, c, live, "the rebuild replaced the entity instance")

		err = s.RefreshReferenceCircle(c, 99)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.InDelta(t, 5, live.R(), 1e-9,
			"the refused call must not silently rewrite the rebuilt circle's own radius")
	})

	t.Run("vector short enough to panic", func(t *testing.T) {
		// Reload a smaller, unrelated document: the rebuilt sketch's variable
		// vector is too short to even contain c.ri, so the unguarded write
		// would panic (index out of range) rather than merely alias.
		s, _, c := refCircleSketch(t, 5)

		empty := newSketch(t)
		smallData, err := json.Marshal(empty)
		require.NoError(t, err)
		require.NoError(t, s.UnmarshalJSON(smallData))

		var refreshErr error
		require.NotPanics(t, func() { refreshErr = s.RefreshReferenceCircle(c, 99) })
		require.ErrorIs(t, refreshErr, sketch.ErrForeignEntity)
	})
}

// The load-bearing half: a live reference circle this sketch owns still
// refreshes exactly as before.
func TestRefreshReferenceCircleOwnedHandleUnchanged(t *testing.T) {
	s, victim, c := refCircleSketch(t, 5)
	s.MarkStale("hole")
	require.True(t, c.IsStale())

	require.NoError(t, s.RefreshReferenceCircle(c, 12))
	require.InDelta(t, 12, c.R(), 1e-9)
	require.True(t, c.IsStale(), "the center point's own stale bit is untouched by a radius refresh")
	require.NoError(t, s.RefreshReference(c.Center, 0, 0))
	require.False(t, c.IsStale(), "clears once every unit of the source is re-fed")
	require.InDelta(t, 10, victim.X(), 1e-9, "refreshing the circle does not move an unrelated point")
	require.InDelta(t, 11, victim.Y(), 1e-9)
}
