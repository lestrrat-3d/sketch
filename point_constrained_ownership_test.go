package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Point.IsFullyConstrained reads p.s.movableVars(), keyed by p's own variable
// indices. Those indices only mean something once p is known to be a live point
// of p.s, so an unscreened nil, zero-value, or removed handle is read wrong: a
// nil or zero-value p.s panics on the p.s dereference, and a removed point's
// retired-but-never-reclaimed indices read as "fully constrained" even though
// its former sketch still has free DOF. Unlike the entity twin
// (EntityIsFullyConstrained), there is no foreign-sketch hole here: the method
// always reads through the point's OWN sketch, so a point of another sketch is
// answered correctly by its own owner regardless of index size.

// A nil receiver panics on p.s without the guard.
func TestPointIsFullyConstrainedRefusesNilReceiver(t *testing.T) {
	var p *sketch.Point

	var got bool
	require.NotPanics(t, func() { got = p.IsFullyConstrained() })
	require.False(t, got)
}

// A PointByName miss hands back exactly this nil.
func TestPointIsFullyConstrainedRefusesPointByNameMiss(t *testing.T) {
	s := newSketch(t)
	require.Nil(t, s.PointByName("no such point"))

	var got bool
	require.NotPanics(t, func() { got = s.PointByName("no such point").IsFullyConstrained() })
	require.False(t, got)
}

// &sketch.Point{} is constructible from outside the package (every field is
// unexported) and has a nil p.s, which panics the same way without the guard.
func TestPointIsFullyConstrainedRefusesZeroValue(t *testing.T) {
	p := &sketch.Point{}

	var got bool
	require.NotPanics(t, func() { got = p.IsFullyConstrained() })
	require.False(t, got)
}

// A removed point's variables are retired (fixed[]=true, never reclaimed), so
// unscreened it would read "fully constrained" while its former sketch still has
// DOF 2.
func TestPointIsFullyConstrainedRefusesRemovedPoint(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(5, 5)
	s.CreatePoint(1, 1) // keeps the sketch's DOF nonzero after p is removed
	require.True(t, s.RemovePoint(p))
	require.Equal(t, 2, s.DOF(), "the sketch still has free DOF after removal")

	var got bool
	require.NotPanics(t, func() { got = p.IsFullyConstrained() })
	require.False(t, got, "a removed point never reads fully constrained")
}

// The refusals agree with the entity-level twin on the two inputs both methods
// can take: nil and a removed handle.
func TestPointIsFullyConstrainedAgreesWithEntityTwin(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		s := newSketch(t)
		var nilPoint *sketch.Point
		var nilEntity sketch.Entity
		require.Equal(t, s.EntityIsFullyConstrained(nilEntity), nilPoint.IsFullyConstrained())
	})

	t.Run("removed", func(t *testing.T) {
		s := newSketch(t)
		a := s.CreatePoint(1, 1)
		b := s.CreatePoint(2, 2)
		l := s.CreateLine(a, b)
		require.True(t, s.RemoveEntity(l))
		require.True(t, s.RemovePoint(a))
		require.True(t, s.RemovePoint(b))
		require.Equal(t, s.EntityIsFullyConstrained(l), a.IsFullyConstrained())
	})
}

// The load-bearing half: what the method reports for a point it can actually
// read is unchanged by the guard.
func TestPointIsFullyConstrainedOwnedHandlesUnchanged(t *testing.T) {
	t.Run("free point", func(t *testing.T) {
		s := newSketch(t)
		p := s.CreatePoint(1, 1)
		require.False(t, p.IsFullyConstrained())
	})

	t.Run("pinned point", func(t *testing.T) {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10, 0)
		l := s.CreateLine(a, b)
		s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
		s.AddConstraint(sketch.NewHorizontal(l))
		s.AddConstraint(sketch.NewDistance(a, b, 10))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.True(t, a.IsFullyConstrained())
		require.True(t, b.IsFullyConstrained())
	})

	t.Run("origin", func(t *testing.T) {
		s := newSketch(t)
		require.True(t, s.Origin().IsFullyConstrained())
	})

	t.Run("reference point", func(t *testing.T) {
		s := newSketch(t)
		rp := s.CreateReferencePoint(3, 4, "edge-1")
		require.True(t, rp.IsFullyConstrained(), "reference geometry is externally locked")
	})

	t.Run("point of another sketch, answered by its own owner", func(t *testing.T) {
		w := sketch.NewWorld()
		other, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		free := other.CreatePoint(1, 1)
		require.False(t, free.IsFullyConstrained(), "free in its own sketch")

		a := other.CreatePoint(0, 0)
		b := other.CreatePoint(10, 0)
		l := other.CreateLine(a, b)
		other.AddConstraint(sketch.NewCoincident(a, other.Origin()))
		other.AddConstraint(sketch.NewHorizontal(l))
		other.AddConstraint(sketch.NewDistance(a, b, 10))
		_, err = other.Solve(t.Context())
		require.NoError(t, err)
		require.True(t, a.IsFullyConstrained(), "pinned in its own sketch")
	})
}
