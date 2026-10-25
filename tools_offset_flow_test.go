package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// The flow of a chain follows the order of the list, not each line's own
// direction: two lines drawn toward their shared corner still offset to one
// side.
func TestOffsetFlowFollowsListOrder(t *testing.T) {
	s := newSketch(t)
	p0, p1, p2 := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(10, 10)
	s.Fix(p0)
	s.Fix(p1)
	s.Fix(p2)
	l1 := s.CreateLine(p0, p1) // runs along the flow
	l2 := s.CreateLine(p2, p1) // drawn against the flow

	g, err := sketch.CreateOffset(s, []sketch.Entity{l1, l2}, 2)
	require.NoError(t, err)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	d1, d2 := g.Copies[0].(*sketch.Line), g.Copies[1].(*sketch.Line)
	require.Same(t, d1.End, d2.End, "the copies share the corner point")
	require.InDelta(t, 12, d1.End.X(), 1e-6, "right of the flow p0→p1→p2, as for a chain drawn in order")
	require.InDelta(t, -2, d1.End.Y(), 1e-6)
	require.InDelta(t, 12, d2.Start.X(), 1e-6)
	require.InDelta(t, 10, d2.Start.Y(), 1e-6, "the free end sits on the normal at p2")
	require.Equal(t, 0, s.DOF())

	g.Set(1)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 11, d1.End.X(), 1e-6)
	require.InDelta(t, -1, d1.End.Y(), 1e-6)
}

// A closed loop has no free end, so no end is pinned, and the copy mitres at
// every corner.
func TestOffsetClosedLoop(t *testing.T) {
	s := newSketch(t)
	a, b, c, d := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(10, 10), s.CreatePoint(0, 10)
	for _, p := range []*sketch.Point{a, b, c, d} {
		s.Fix(p)
	}
	loop := []sketch.Entity{s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, d), s.CreateLine(d, a)}

	// The loop runs counter-clockwise, so its right is outside.
	g, err := sketch.CreateOffset(s, loop, 1)
	require.NoError(t, err)
	require.Empty(t, g.Constraints, "a closed loop has no free end")
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF())
	first := g.Copies[0].(*sketch.Line)
	require.InDelta(t, -1, first.Start.X(), 1e-6)
	require.InDelta(t, -1, first.Start.Y(), 1e-6)
}

// Three lines meeting at one point are not a chain.
func TestOffsetRefusesBranch(t *testing.T) {
	s := newSketch(t)
	hub := s.CreatePoint(0, 0)
	ents := []sketch.Entity{
		s.CreateLine(hub, s.CreatePoint(10, 0)),
		s.CreateLine(hub, s.CreatePoint(0, 10)),
		s.CreateLine(hub, s.CreatePoint(-10, 0)),
	}
	pts, cons := len(s.Points()), len(s.Constraints())

	_, err := sketch.CreateOffset(s, ents, 1)
	require.ErrorIs(t, err, sketch.ErrInvalidShape)
	require.Len(t, s.Points(), pts)
	require.Len(t, s.Constraints(), cons)
}
