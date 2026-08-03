package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Fix, Unfix, FixEntity, UnfixEntity and EntityFixed index the sketch's fixed[]
// vector by the passed handle's own variable indices. Those indices only mean
// anything in the sketch that allocated them, so a handle the sketch does not
// own must be refused before it is indexed: a large foreign index runs off the
// vector and panics, and a small one silently grounds or releases THIS sketch's
// unrelated variable while the passed handle is untouched.

// groundingState records whether each of the sketch's points is grounded, in id
// order, together with its coordinates — the state a refused call must leave
// exactly as it found it.
func groundingState(s *sketch.Sketch) [][3]float64 {
	out := make([][3]float64, 0, len(s.Points()))
	for _, p := range s.Points() {
		fixed := 0.0
		if p.IsFixed() {
			fixed = 1
		}
		out = append(out, [3]float64{p.X(), p.Y(), fixed})
	}
	return out
}

// foreignPoint returns a live point of a DIFFERENT sketch in its own world. With
// pad extra points created first, its variable indices sit far past the end of a
// small receiving sketch's vectors.
func foreignPoint(t *testing.T, pad int, x, y float64) *sketch.Point {
	t.Helper()
	w := sketch.NewWorld()
	o, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	for i := 0; i < pad; i++ {
		o.CreatePoint(float64(i), float64(i))
	}
	return o.CreatePoint(x, y)
}

// foreignPaddedLine returns a live line of a DIFFERENT sketch, its defining
// points allocated after pad unrelated points so their variable indices sit far
// past the end of a small receiving sketch's vectors.
func foreignPaddedLine(t *testing.T, pad int) *sketch.Line {
	t.Helper()
	w := sketch.NewWorld()
	o, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	for i := 0; i < pad; i++ {
		o.CreatePoint(float64(i), float64(i))
	}
	return o.CreateLine(o.CreatePoint(20, 20), o.CreatePoint(30, 30))
}

// padNames names the two index regimes the subtests run in: a foreign index
// small enough to be in range in the receiving sketch (the silent cross-mutation)
// and one past its end (the panic).
var padNames = map[int]string{0: "small foreign index", 64: "large foreign index"}

func TestGroundingRejectsForeignPoint(t *testing.T) {
	// A small foreign index is in range in the receiving sketch, so the unguarded
	// call grounds the receiving sketch's own unrelated point instead. A large one
	// runs off fixed[] and panics.
	for _, pad := range []int{0, 64} {
		t.Run("Fix/"+padNames[pad], func(t *testing.T) {
			s := newSketch(t)
			a := s.CreatePoint(1, 2)
			b := s.CreatePoint(3, 4)
			s.CreateLine(a, b)
			fp := foreignPoint(t, pad, 7, 8)
			before := groundingState(s)

			require.NotPanics(t, func() { s.Fix(fp) })
			require.Equal(t, before, groundingState(s), "receiving sketch untouched")
			require.False(t, fp.IsFixed(), "the passed point is untouched too")
		})

		t.Run("Unfix/"+padNames[pad], func(t *testing.T) {
			s := newSketch(t)
			a := s.CreatePoint(1, 2)
			b := s.CreatePoint(3, 4)
			s.CreateLine(a, b)
			s.Fix(a)
			s.Fix(b)
			fp := foreignPoint(t, pad, 7, 8)
			before := groundingState(s)

			require.NotPanics(t, func() { s.Unfix(fp) })
			require.Equal(t, before, groundingState(s), "receiving sketch untouched")
			require.False(t, fp.IsFixed())
		})
	}
}

func TestGroundingRejectsForeignEntity(t *testing.T) {
	for _, pad := range []int{0, 64} {
		t.Run("FixEntity/"+padNames[pad], func(t *testing.T) {
			s := newSketch(t)
			s.CreateLine(s.CreatePoint(1, 2), s.CreatePoint(3, 4))
			fl := foreignPaddedLine(t, pad)
			before := groundingState(s)

			require.NotPanics(t, func() { s.FixEntity(fl) })
			require.Equal(t, before, groundingState(s), "receiving sketch untouched")
			require.False(t, fl.Start.IsFixed(), "the passed entity is untouched too")
			require.False(t, fl.End.IsFixed())
		})

		t.Run("UnfixEntity/"+padNames[pad], func(t *testing.T) {
			s := newSketch(t)
			a := s.CreatePoint(1, 2)
			b := s.CreatePoint(3, 4)
			l := s.CreateLine(a, b)
			s.FixEntity(l)
			fl := foreignPaddedLine(t, pad)
			before := groundingState(s)

			require.NotPanics(t, func() { s.UnfixEntity(fl) })
			require.Equal(t, before, groundingState(s), "receiving sketch untouched")
			require.True(t, s.EntityFixed(l), "the local entity stays grounded")
		})

		t.Run("EntityFixed/"+padNames[pad], func(t *testing.T) {
			s := newSketch(t)
			s.CreateLine(s.CreatePoint(1, 2), s.CreatePoint(3, 4))
			fl := foreignPaddedLine(t, pad)
			before := groundingState(s)

			var got bool
			require.NotPanics(t, func() { got = s.EntityFixed(fl) })
			require.False(t, got, "an unowned entity is never reported grounded")
			require.Equal(t, before, groundingState(s))
		})
	}
}

// A grounded foreign entity must not read as grounded through another sketch:
// its variable indices name that other sketch's fixed[] slots.
func TestEntityFixedRefusesGroundedForeignEntity(t *testing.T) {
	w := sketch.NewWorld()
	o, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	fl := o.CreateLine(o.CreatePoint(1, 1), o.CreatePoint(2, 2))
	o.FixEntity(fl)
	require.True(t, o.EntityFixed(fl), "grounded in its own sketch")

	s := newSketch(t)
	require.False(t, s.EntityFixed(fl), "but never through a sketch that does not own it")
}

// The entity half of ownership does not imply the point half: entity fields are
// exported, so a point of an entity this sketch owns can be rewired to another
// sketch's point. Grounding it would index this sketch's fixed[] with that
// point's foreign index.
func TestGroundingRejectsEntityWithForeignPoint(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(1, 2)
	b := s.CreatePoint(3, 4)
	l := s.CreateLine(a, b)
	l.End = foreignPoint(t, 64, 9, 9)
	before := groundingState(s)

	require.NotPanics(t, func() { s.FixEntity(l) })
	require.Equal(t, before, groundingState(s))
	require.NotPanics(t, func() { s.UnfixEntity(l) })
	require.Equal(t, before, groundingState(s))

	var got bool
	require.NotPanics(t, func() { got = s.EntityFixed(l) })
	require.False(t, got)
}

// A removed handle is dead: its id no longer names it, and the point that
// inherited that id is a different point entirely.
func TestGroundingRejectsRemovedHandle(t *testing.T) {
	t.Run("point", func(t *testing.T) {
		s := newSketch(t)
		dead := s.CreatePoint(1, 2)
		a := s.CreatePoint(3, 4)
		b := s.CreatePoint(5, 6)
		s.CreateLine(a, b)
		require.True(t, s.RemovePoint(dead))
		before := groundingState(s)

		require.NotPanics(t, func() { s.Fix(dead) })
		require.Equal(t, before, groundingState(s))
		require.NotPanics(t, func() { s.Unfix(dead) })
		require.Equal(t, before, groundingState(s))
	})

	t.Run("entity", func(t *testing.T) {
		s := newSketch(t)
		a := s.CreatePoint(1, 2)
		b := s.CreatePoint(3, 4)
		dead := s.CreateLine(a, b)
		require.True(t, s.RemoveEntity(dead))
		before := groundingState(s)

		require.NotPanics(t, func() { s.FixEntity(dead) })
		require.Equal(t, before, groundingState(s))
		require.NotPanics(t, func() { s.UnfixEntity(dead) })
		require.Equal(t, before, groundingState(s))
		require.False(t, s.EntityFixed(dead))
	})
}

func TestGroundingRejectsNilHandle(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(1, 2)
	b := s.CreatePoint(3, 4)
	s.CreateLine(a, b)
	s.Fix(a)
	before := groundingState(s)

	var nilPoint *sketch.Point
	var nilEntity sketch.Entity
	var typedNil *sketch.Line // a non-nil interface holding a nil pointer

	require.NotPanics(t, func() { s.Fix(nil) })
	require.NotPanics(t, func() { s.Fix(nilPoint) })
	require.NotPanics(t, func() { s.Unfix(nil) })
	require.NotPanics(t, func() { s.Unfix(nilPoint) })
	require.NotPanics(t, func() { s.FixEntity(nilEntity) })
	require.NotPanics(t, func() { s.FixEntity(typedNil) })
	require.NotPanics(t, func() { s.UnfixEntity(nilEntity) })
	require.NotPanics(t, func() { s.UnfixEntity(typedNil) })

	var got1, got2 bool
	require.NotPanics(t, func() { got1 = s.EntityFixed(nilEntity) })
	require.NotPanics(t, func() { got2 = s.EntityFixed(typedNil) })
	require.False(t, got1)
	require.False(t, got2)

	require.Equal(t, before, groundingState(s), "no nil call touched the sketch")
}

// The load-bearing half: only the refused inputs move. Everything the five
// methods do for handles the sketch owns is exactly what it was.
func TestGroundingOwnedHandlesUnchanged(t *testing.T) {
	t.Run("point", func(t *testing.T) {
		s := newSketch(t)
		a := s.CreatePoint(1, 2)
		b := s.CreatePoint(3, 4)
		s.CreateLine(a, b)

		require.False(t, a.IsFixed())
		s.Fix(a)
		require.True(t, a.IsFixed())
		require.False(t, b.IsFixed(), "only the passed point is grounded")
		s.Unfix(a)
		require.False(t, a.IsFixed())
	})

	t.Run("origin", func(t *testing.T) {
		s := newSketch(t)
		o := s.Origin()
		require.True(t, o.IsFixed(), "the origin is grounded for the sketch's life")

		s.Fix(o)
		require.True(t, o.IsFixed())
		s.Unfix(o)
		require.True(t, o.IsFixed(), "Unfix still refuses to release the origin")
	})

	t.Run("line", func(t *testing.T) {
		s := newSketch(t)
		a := s.CreatePoint(1, 2)
		b := s.CreatePoint(3, 4)
		l := s.CreateLine(a, b)

		require.False(t, s.EntityFixed(l))
		s.FixEntity(l)
		require.True(t, s.EntityFixed(l))
		require.True(t, a.IsFixed())
		require.True(t, b.IsFixed())
		s.UnfixEntity(l)
		require.False(t, s.EntityFixed(l))
		require.False(t, a.IsFixed())
		require.False(t, b.IsFixed())
	})

	t.Run("circle radius", func(t *testing.T) {
		s := newSketch(t)
		c := s.CreateCircle(s.CreatePoint(0, 0), 5)

		s.Fix(c.Center)
		require.False(t, s.EntityFixed(c), "the radius variable is still free")
		s.FixEntity(c)
		require.True(t, s.EntityFixed(c))
		s.UnfixEntity(c)
		require.False(t, s.EntityFixed(c))
	})

	t.Run("entity drawn from the origin", func(t *testing.T) {
		s := newSketch(t)
		far := s.CreatePoint(10, 0)
		l := s.CreateLine(s.Origin(), far)

		s.FixEntity(l)
		require.True(t, far.IsFixed())
		require.True(t, s.EntityFixed(l))
		s.UnfixEntity(l)
		require.False(t, far.IsFixed())
		require.True(t, s.Origin().IsFixed(), "UnfixEntity never releases the origin")
	})

	t.Run("reference geometry", func(t *testing.T) {
		s := newSketch(t)
		rp1 := s.CreateReferencePoint(0, 0, "edge-1")
		rp2 := s.CreateReferencePoint(10, 0, "edge-1")
		rl, err := s.CreateReferenceLine(rp1, rp2, "edge-1")
		require.NoError(t, err)

		require.True(t, s.EntityFixed(rl), "reference geometry is externally locked")
		s.Unfix(rp1)
		require.True(t, rp1.IsFixed(), "Unfix still refuses reference geometry")
		s.UnfixEntity(rl)
		require.True(t, s.EntityFixed(rl), "UnfixEntity still refuses it too")
	})
}
