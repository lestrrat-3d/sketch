package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Every modification tool refuses a nil sketch with its refusal shape instead of
// panicking. The handles come from a live sketch so only the nil sketch is wrong.
func TestToolsRefuseNilSketch(t *testing.T) {
	var nilSketch *sketch.Sketch
	live := newSketch(t)
	corner := live.CreatePoint(0, 0)
	l1 := live.CreateLine(corner, live.CreatePoint(10, 0))
	l2 := live.CreateLine(corner, live.CreatePoint(0, 10))
	axis := live.CreateLine(live.CreatePoint(-5, -5), live.CreatePoint(-5, 5))
	seed := []sketch.Entity{l1}

	a, b, ok := sketch.Break(nilSketch, l1, 5, 0)
	require.False(t, ok)
	require.Nil(t, a)
	require.Nil(t, b)

	res, ok := sketch.Trim(nilSketch, l1, 5, 0)
	require.False(t, ok)
	require.Nil(t, res)

	ext, ok := sketch.Extend(nilSketch, l1, l1.End)
	require.False(t, ok)
	require.Nil(t, ext)

	f, err := sketch.CreateFillet(nilSketch, l1, l2, corner, 2)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, f)

	c, err := sketch.CreateChamfer(nilSketch, l1, l2, 2)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, c)

	m, err := sketch.CreateMirror(nilSketch, seed, axis)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, m)

	p, err := sketch.CreatePatternRect(nilSketch, seed, 2, 1, 20, 0)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, p)

	p, err = sketch.CreatePatternCircular(nilSketch, seed, corner, 3)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, p)

	p, err = sketch.CreatePatternCircularRing(nilSketch, seed, corner, 3, l1.Start, l1.End)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, p)

	g, err := sketch.CreateOffset(nilSketch, seed, 2)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, g)

	require.Len(t, live.Entities(), 3, "the live sketch is untouched")
}

// A typed result keeps its concrete type: no assertion is needed on the halves
// of a broken arc or on the legs of a line fillet.
func TestToolsReturnConcreteTypes(t *testing.T) {
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))
	a1, a2, ok := sketch.Break(s, arc, 4, 4)
	require.True(t, ok)
	require.Same(t, a1.End, a2.Start)

	corner := s.CreatePoint(20, 0)
	l1 := s.CreateLine(corner, s.CreatePoint(30, 0))
	l2 := s.CreateLine(corner, s.CreatePoint(20, 10))
	f, err := sketch.CreateFillet(s, l1, l2, corner, 2)
	require.NoError(t, err)
	require.InDelta(t, 22, f.L1.Start.X(), 1e-9, "L1 is a *sketch.Line")

	_, err = sketch.CreateFillet(s, f.L1, a1, f.L1.End, 2)
	require.ErrorIs(t, err, sketch.ErrNoSharedCorner, "the line and the arc share no end point")
}
