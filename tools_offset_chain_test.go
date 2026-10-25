package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// fixAll grounds every point the sketch holds, so the DOF that remains belongs
// to the offset copy alone. The sources carry no constraints of their own; only
// a source arc's internal radius row, its points grounded, reads as redundant.
func fixAll(s *sketch.Sketch) {
	for _, p := range s.Points() {
		s.Fix(p)
	}
}

// requireDeterminedOffset solves, then requires the copy to be fully
// determined with no redundant constraint of its own, before and after Set(d2).
func requireDeterminedOffset(t *testing.T, s *sketch.Sketch, g *sketch.OffsetGroup, d2 float64) {
	t.Helper()
	sources := map[sketch.Entity]struct{}{}
	for _, e := range g.Source {
		sources[e] = struct{}{}
	}
	check := func() {
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, s.DOF())
		for _, c := range s.RedundantConstraints() {
			_, ents := sketch.ConstraintRefs(c)
			require.True(t, sketch.IsInternal(c), "only a grounded source arc's radius row may read redundant")
			require.Contains(t, sources, ents[0])
		}
	}
	check()
	g.Set(d2)
	check()
}

// A slot: two lines and two half circles, every joint tangent.
func TestOffsetChainSlot(t *testing.T) {
	s := newSketch(t)
	c1, c2 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	p1l, p1r := s.CreatePoint(0, 2), s.CreatePoint(0, -2)
	p2l, p2r := s.CreatePoint(10, 2), s.CreatePoint(10, -2)
	bottom := s.CreateLine(p1r, p2r)
	right := s.CreateArc(c2, p2r, p2l)
	top := s.CreateLine(p2l, p1l)
	left := s.CreateArc(c1, p1l, p1r)
	fixAll(s)

	// The loop runs counter-clockwise, so positive is outside.
	g, err := sketch.CreateOffset(s, []sketch.Entity{bottom, right, top, left}, 1)
	require.NoError(t, err)
	require.Empty(t, g.Offsets, "both lines take their place from the arcs")
	require.Len(t, g.Radials, 2)
	requireDeterminedOffset(t, s, g, 2)

	cb, cr := g.Copies[0].(*sketch.Line), g.Copies[1].(*sketch.Arc)
	require.InDelta(t, -4, cb.Start.Y(), 1e-6, "bottom copy at distance 2")
	require.InDelta(t, -4, cb.End.Y(), 1e-6)
	require.InDelta(t, 4, cr.R(), 1e-6)
	require.Same(t, c2, cr.Center)
}

// A rounded rectangle: four lines and four fillet arcs, every joint tangent.
func TestOffsetChainRoundedRectangle(t *testing.T) {
	s := newSketch(t)
	pt := s.CreatePoint
	b0, b1 := pt(2, 0), pt(18, 0)
	r0, r1 := pt(20, 2), pt(20, 18)
	t0, t1 := pt(18, 20), pt(2, 20)
	l0, l1 := pt(0, 18), pt(0, 2)
	ents := []sketch.Entity{
		s.CreateLine(b0, b1), s.CreateArc(pt(18, 2), b1, r0),
		s.CreateLine(r0, r1), s.CreateArc(pt(18, 18), r1, t0),
		s.CreateLine(t0, t1), s.CreateArc(pt(2, 18), t1, l0),
		s.CreateLine(l0, l1), s.CreateArc(pt(2, 2), l1, b0),
	}
	fixAll(s)

	g, err := sketch.CreateOffset(s, ents, 1)
	require.NoError(t, err)
	requireDeterminedOffset(t, s, g, 1)

	bottom := g.Copies[0].(*sketch.Line)
	require.InDelta(t, -1, bottom.Start.Y(), 1e-6)
	require.InDelta(t, 2, bottom.Start.X(), 1e-6)
	require.InDelta(t, 3, g.Copies[1].(*sketch.Arc).R(), 1e-6)
}

// A rectangle with one filleted corner: the two lines beside the fillet are
// held tangent to its copy, the other two by Offset, and the corners mitre.
func TestOffsetChainOneFilletedCorner(t *testing.T) {
	s := newSketch(t)
	pt := s.CreatePoint
	o, b1, r0, tr, tl := pt(0, 0), pt(18, 0), pt(20, 2), pt(20, 20), pt(0, 20)
	ents := []sketch.Entity{
		s.CreateLine(o, b1), s.CreateArc(pt(18, 2), b1, r0),
		s.CreateLine(r0, tr), s.CreateLine(tr, tl), s.CreateLine(tl, o),
	}
	fixAll(s)

	g, err := sketch.CreateOffset(s, ents, 1)
	require.NoError(t, err)
	require.Len(t, g.Offsets, 2)
	require.Len(t, g.Radials, 1)
	require.Equal(t, 2, countKind(s, "tangent_line_circle"))
	requireDeterminedOffset(t, s, g, 1)

	top := g.Copies[3].(*sketch.Line)
	require.InDelta(t, 21, top.Start.X(), 1e-6, "the top-right corner mitres at (21, 21)")
	require.InDelta(t, 21, top.Start.Y(), 1e-6)
	require.InDelta(t, -1, top.End.X(), 1e-6)
	require.InDelta(t, 21, top.End.Y(), 1e-6)
}

// An open chain of a line and an arc meeting tangentially: both free ends are
// held on the source normals, so the copy is determined.
func TestOffsetChainOpenLineArc(t *testing.T) {
	s := newSketch(t)
	joint := s.CreatePoint(10, 0)
	line := s.CreateLine(s.CreatePoint(0, 0), joint)
	arc := s.CreateArc(s.CreatePoint(10, 5), joint, s.CreatePoint(15, 5))
	fixAll(s)

	g, err := sketch.CreateOffset(s, []sketch.Entity{line, arc}, 1)
	require.NoError(t, err)
	requireDeterminedOffset(t, s, g, 1)

	cl, ca := g.Copies[0].(*sketch.Line), g.Copies[1].(*sketch.Arc)
	require.InDelta(t, 0, cl.Start.X(), 1e-6)
	require.InDelta(t, -1, cl.Start.Y(), 1e-6)
	require.Same(t, cl.End, ca.Start)
	require.InDelta(t, 6, ca.R(), 1e-6)
	require.InDelta(t, 16, ca.End.X(), 1e-6)
	require.InDelta(t, 5, ca.End.Y(), 1e-6)
}
