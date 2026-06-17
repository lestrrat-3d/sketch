package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestFixEntityCircle(t *testing.T) {
	s := sketch.New()
	o := s.AddPoint(3, 4)
	c := s.AddCircle(o, 5)
	s.FixEntity(c)
	require.True(t, s.EntityFixed(c), "center and radius are grounded")

	// A point made coincident with the center must move TO it, not drag it.
	p := s.AddPoint(0, 0)
	s.AddConstraint(sketch.NewCoincident(p, o))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 3, o.X(), 1e-6, "fixed center held")
	require.InDelta(t, 4, o.Y(), 1e-6, "fixed center held")
	require.InDelta(t, 5, c.R(), 1e-6, "fixed radius held")
	require.InDelta(t, 3, p.X(), 1e-6, "the free point moved to the center")
	require.InDelta(t, 4, p.Y(), 1e-6)
}

func TestUnfixEntity(t *testing.T) {
	s := sketch.New()
	o := s.AddPoint(0, 0)
	c := s.AddCircle(o, 2)
	s.FixEntity(c)
	require.True(t, s.EntityFixed(c))
	s.UnfixEntity(c)
	require.False(t, s.EntityFixed(c), "released")

	// Now a radius dimension can drive it again.
	s.AddConstraint(sketch.NewRadius(c, 9))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 9, c.R(), 1e-6, "radius moved after unfix")
}

func TestUnfixEntityPreservesReferenceLock(t *testing.T) {
	s := sketch.New()
	rp := s.AddReferencePoint(5, 5, "edge1") // externally locked
	require.True(t, rp.IsFixed())
	free := s.AddPoint(0, 0)
	l := s.AddLine(rp, free) // an ordinary line sharing the reference point
	s.FixEntity(l)
	s.UnfixEntity(l)
	require.True(t, rp.IsFixed(), "the shared reference point keeps its lock")
	require.False(t, free.IsFixed(), "the ordinary endpoint was released")
}

func TestSymmetricLines(t *testing.T) {
	s := sketch.New()
	// Axis = the y-axis (the line x=0); reflection negates x.
	ax := s.AddPoint(0, 0)
	ay := s.AddPoint(0, 10)
	s.Fix(ax)
	s.Fix(ay)
	axis := s.AddLine(ax, ay)

	a := s.AddPoint(2, 1)
	b := s.AddPoint(5, 3)
	s.Fix(a)
	s.Fix(b)
	l1 := s.AddLine(a, b)

	c := s.AddPoint(-1, 1) // free, becomes the mirror of a
	d := s.AddPoint(-4, 2) // free, becomes the mirror of b
	l2 := s.AddLine(c, d)
	s.AddConstraint(sketch.NewSymmetricLines(l1, l2, axis))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, -2, c.X(), 1e-6, "mirror of a across x=0")
	require.InDelta(t, 1, c.Y(), 1e-6)
	require.InDelta(t, -5, d.X(), 1e-6, "mirror of b across x=0")
	require.InDelta(t, 3, d.Y(), 1e-6)
}

func TestSymmetricCircles(t *testing.T) {
	s := sketch.New()
	ax := s.AddPoint(0, 0)
	ay := s.AddPoint(0, 10)
	s.Fix(ax)
	s.Fix(ay)
	axis := s.AddLine(ax, ay)

	o1 := s.AddPoint(3, 2)
	c1 := s.AddCircle(o1, 4)
	s.FixEntity(c1)

	o2 := s.AddPoint(-1, 0)
	c2 := s.AddCircle(o2, 1)
	s.AddConstraint(sketch.NewSymmetricCircles(c1, c2, axis))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, -3, o2.X(), 1e-6, "center mirrored across x=0")
	require.InDelta(t, 2, o2.Y(), 1e-6)
	require.InDelta(t, 4, c2.R(), 1e-6, "radius equalised")
}
