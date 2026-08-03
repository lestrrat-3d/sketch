package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// EntityIsFullyConstrained answers from THIS sketch's null-space support, keyed
// by the passed handle's own variable indices. Those indices only mean something
// in the sketch that allocated them, so a handle the sketch does not own must be
// refused before it is read: an unscreened foreign, dead or nil handle is
// answered from variables nobody examined, and every one of those wrong answers
// blesses ("nothing here can move") rather than refuses.

// groundedSketch returns a sketch at DOF 0 — anchored to the origin, oriented by
// one horizontal, sized by one dimension. Its null-space support is empty, so an
// unscreened read of ANY handle through it reports "fully constrained".
func groundedSketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	l := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
	s.AddConstraint(sketch.NewHorizontal(l))
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF(), "the receiving sketch is fully constrained")
	return s
}

// foreignFreeCircle returns a live, totally free circle of a DIFFERENT sketch —
// three degrees of freedom, center and radius, constrained by nothing.
func foreignFreeCircle(t *testing.T) (*sketch.Sketch, *sketch.Circle) {
	t.Helper()
	o := newSketch(t)
	c := o.CreateCircle(o.CreatePoint(1, 1), 5)
	require.False(t, o.EntityIsFullyConstrained(c), "free in its own sketch")
	return o, c
}

// The load-bearing case: a genuinely free foreign circle read through a DOF-0
// sketch. Unscreened, the empty null-space support answers for indices this
// sketch never allocated and the free circle reads fully constrained.
func TestEntityIsFullyConstrainedRefusesForeignEntity(t *testing.T) {
	s := groundedSketch(t)
	_, c := foreignFreeCircle(t)

	var got bool
	require.NotPanics(t, func() { got = s.EntityIsFullyConstrained(c) })
	require.False(t, got, "an unowned entity is never reported fully constrained")
	require.False(t, s.EntityFixed(c), "the screened reader already answered false")
}

// The same refusal at the other index regime: a foreign entity whose variable
// indices run past the end of the receiving sketch's vectors.
func TestEntityIsFullyConstrainedRefusesForeignEntityAtBothIndexRegimes(t *testing.T) {
	for _, pad := range []int{0, 64} {
		t.Run(padNames[pad], func(t *testing.T) {
			s := groundedSketch(t)
			fl := foreignPaddedLine(t, pad)

			var got bool
			require.NotPanics(t, func() { got = s.EntityIsFullyConstrained(fl) })
			require.False(t, got)
			require.Equal(t, s.EntityFixed(fl), got, "the two bare-bool entity reads agree")
		})
	}
}

// The entity half of ownership does not imply the point half: entity fields are
// exported, so a point of an entity this sketch owns can be rewired to another
// sketch's point, whose index names an unrelated local variable. This one was
// already false before the screen — a large foreign index is simply absent from
// the local support — so it pins the screen leaving that answer alone.
func TestEntityIsFullyConstrainedRefusesEntityWithForeignPoint(t *testing.T) {
	s := groundedSketch(t)
	l := s.CreateLine(s.CreatePoint(1, 2), s.CreatePoint(3, 4))
	l.End = foreignPoint(t, 64, 9, 9)

	var got bool
	require.NotPanics(t, func() { got = s.EntityIsFullyConstrained(l) })
	require.False(t, got)
	require.Equal(t, s.EntityFixed(l), got, "the two bare-bool entity reads agree")
}

// A removed handle is dead: its variables are retired and its points' ids no
// longer name them.
func TestEntityIsFullyConstrainedRefusesRemovedHandle(t *testing.T) {
	s := groundedSketch(t)
	a := s.CreatePoint(20, 20)
	b := s.CreatePoint(30, 30)
	dead := s.CreateLine(a, b)
	require.True(t, s.RemoveEntity(dead))
	require.True(t, s.RemovePoint(a))
	require.True(t, s.RemovePoint(b))

	var got bool
	require.NotPanics(t, func() { got = s.EntityIsFullyConstrained(dead) })
	require.False(t, got)
	require.False(t, s.EntityFixed(dead))
}

// A nil handle comes back from a lookup miss — Sketch.EntityByName returns an
// untyped nil — and a typed nil is what a caller stores in an Entity variable it
// never assigned. Reading either dereferences a nil pointer in entityPoints.
func TestEntityIsFullyConstrainedRefusesNilHandle(t *testing.T) {
	s := groundedSketch(t)

	var nilEntity sketch.Entity
	var typedNil *sketch.Line // a non-nil interface holding a nil pointer

	require.Nil(t, s.EntityByName("no such entity"), "a lookup miss is an untyped nil")

	var got1, got2, got3 bool
	require.NotPanics(t, func() { got1 = s.EntityIsFullyConstrained(nilEntity) })
	require.NotPanics(t, func() { got2 = s.EntityIsFullyConstrained(typedNil) })
	require.NotPanics(t, func() { got3 = s.EntityIsFullyConstrained(s.EntityByName("no such entity")) })
	require.False(t, got1)
	require.False(t, got2)
	require.False(t, got3)

	require.Equal(t, s.EntityFixed(nilEntity), got1, "the two bare-bool entity reads agree")
	require.Equal(t, s.EntityFixed(typedNil), got2)
}

// The load-bearing half: only the refused inputs changed. What the method
// reports for handles the sketch owns is exactly what it was.
func TestEntityIsFullyConstrainedOwnedHandlesUnchanged(t *testing.T) {
	t.Run("free circle", func(t *testing.T) {
		s := newSketch(t)
		c := s.CreateCircle(s.CreatePoint(0, 0), 5)
		require.False(t, s.EntityIsFullyConstrained(c))
	})

	t.Run("free radius", func(t *testing.T) {
		s := newSketch(t)
		c := s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.AddConstraint(sketch.NewCoincident(c.Center, s.Origin()))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.False(t, s.EntityIsFullyConstrained(c), "the radius is still free")

		s.AddConstraint(sketch.NewRadius(c, 5))
		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		require.True(t, s.EntityIsFullyConstrained(c), "center and radius both pinned")
	})

	t.Run("entity drawn from the origin", func(t *testing.T) {
		s := newSketch(t)
		far := s.CreatePoint(10, 0)
		l := s.CreateLine(s.Origin(), far)
		require.False(t, s.EntityIsFullyConstrained(l), "the far end is free")

		s.AddConstraint(sketch.NewHorizontal(l))
		s.AddConstraint(sketch.NewDistance(s.Origin(), far, 10))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.True(t, s.EntityIsFullyConstrained(l),
			"origin-drawn geometry still answers normally — owns carries the origin exception")
	})

	t.Run("grounded line", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(1, 2), s.CreatePoint(3, 4))
		s.FixEntity(l)
		require.True(t, s.EntityIsFullyConstrained(l))
	})
}
