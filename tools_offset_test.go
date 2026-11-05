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

// A circle offsets to a concentric copy sharing its center; positive is outward,
// as in Fusion, and Set retargets the copy.
func TestOffsetCircle(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	s.Fix(center)
	src := s.CreateCircle(center, 10)
	s.AddConstraint(sketch.NewRadius(src, 10))

	g, err := sketch.CreateOffset(s, []sketch.Entity{src}, 2)
	require.NoError(t, err)
	dst := g.Copies[0].(*sketch.Circle)
	require.Same(t, center, dst.Center)
	require.Len(t, g.Radials, 1)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF())
	require.InDelta(t, 12, dst.R(), 1e-6)

	g.Set(-3)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, dst.R(), 1e-6)
}

// A lone arc offsets to a concentric arc whose ends stay on the source normals.
func TestOffsetLoneArc(t *testing.T) {
	s := newSketch(t)
	center, start, end := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(0, 10)
	for _, p := range []*sketch.Point{center, start, end} {
		s.Fix(p)
	}
	src := s.CreateArc(center, start, end)

	g, err := sketch.CreateOffset(s, []sketch.Entity{src}, 2)
	require.NoError(t, err)
	dst := g.Copies[0].(*sketch.Arc)
	require.Same(t, center, dst.Center)
	require.Len(t, g.Constraints, 2, "both ends are held on the source normals")
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF())
	require.InDelta(t, 12, dst.Start.X(), 1e-6)
	require.InDelta(t, 0, dst.Start.Y(), 1e-6)
	require.InDelta(t, 0, dst.End.X(), 1e-6)
	require.InDelta(t, 12, dst.End.Y(), 1e-6)

	g.Set(1)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 11, dst.R(), 1e-6)
	require.InDelta(t, 11, dst.Start.X(), 1e-6)
}

// Refusals leave the sketch unchanged.
func TestOffsetCurveRefusals(t *testing.T) {
	cases := []struct {
		name  string
		build func(s *sketch.Sketch) []sketch.Entity
		want  error
	}{
		{"ellipse", func(s *sketch.Sketch) []sketch.Entity {
			return []sketch.Entity{s.CreateEllipse(s.CreatePoint(0, 0), 5, 3, 0)}
		}, sketch.ErrUnsupportedEntity},
		{"radius would vanish", func(s *sketch.Sketch) []sketch.Entity {
			return []sketch.Entity{s.CreateCircle(s.CreatePoint(0, 0), 2)}
		}, sketch.ErrInvalidShape},
		{"cusp", func(s *sketch.Sketch) []sketch.Entity {
			p := s.CreatePoint(10, 0)
			return []sketch.Entity{s.CreateLine(s.CreatePoint(0, 0), p), s.CreateLine(p, s.CreatePoint(5, 0))}
		}, sketch.ErrInvalidShape},
		{"closed loop of tangent arcs", func(s *sketch.Sketch) []sketch.Entity {
			c, a, b := s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(-5, 0)
			return []sketch.Entity{s.CreateArc(c, a, b), s.CreateArc(c, b, a)}
		}, sketch.ErrInvalidShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			ents := tc.build(s)
			pts, n, cons := len(s.Points()), len(s.Entities()), len(s.Constraints())

			_, err := sketch.CreateOffset(s, ents, -3)
			require.ErrorIs(t, err, tc.want)
			require.Len(t, s.Points(), pts)
			require.Len(t, s.Entities(), n)
			require.Len(t, s.Constraints(), cons)
		})
	}
}
