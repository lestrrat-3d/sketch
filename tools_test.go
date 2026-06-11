package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestBreakLine(t *testing.T) {
	s := sketch.New()
	a, b := s.AddPoint(0, 0), s.AddPoint(10, 0)
	l := s.AddLine(a, b)

	e1, e2, ok := s.Break(l, 4, 0)
	require.True(t, ok)
	l1, l2 := e1.(*sketch.Line), e2.(*sketch.Line)

	require.Len(t, s.Entities(), 2, "original replaced by two segments")
	require.Same(t, l1.End, l2.Start, "the two halves share the split vertex")
	require.Same(t, a, l1.Start, "far endpoints are preserved")
	require.Same(t, b, l2.End)
	require.InDelta(t, 4, l1.End.X(), 1e-9)
	require.InDelta(t, 0, l1.End.Y(), 1e-9)
}

func TestBreakLineRejectsEndpointPick(t *testing.T) {
	s := sketch.New()
	l := s.AddLine(s.AddPoint(0, 0), s.AddPoint(10, 0))
	_, _, ok := s.Break(l, 0, 0) // pick at the start endpoint
	require.False(t, ok)
	require.Len(t, s.Entities(), 1, "nothing changed")
}

func TestBreakArc(t *testing.T) {
	s := sketch.New()
	arc := s.AddArc(s.AddPoint(0, 0), s.AddPoint(5, 0), s.AddPoint(0, 5))

	e1, e2, ok := s.Break(arc, 4, 4) // pick near the 45° point
	require.True(t, ok)
	a1, a2 := e1.(*sketch.Arc), e2.(*sketch.Arc)

	require.Same(t, a1.Center, a2.Center, "halves share the center")
	require.Same(t, a1.End, a2.Start, "halves share the split point")
	require.InDelta(t, 5/math.Sqrt2, a1.End.X(), 1e-9)
	require.InDelta(t, 5/math.Sqrt2, a1.End.Y(), 1e-9)
	require.Len(t, s.Constraints(), 2, "one internal arcRadius per new arc, original dropped")
}

func TestBreakIsParametric(t *testing.T) {
	s := sketch.New()
	a, b := s.AddPoint(0, 0), s.AddPoint(10, 0)
	s.Fix(a)
	l := s.AddLine(a, b)
	e1, _, ok := s.Break(l, 6, 0)
	require.True(t, ok)
	l1 := e1.(*sketch.Line)

	// Dimension the first half; the shared split vertex must move to satisfy it.
	d := sketch.NewDistance(l1.Start, l1.End, 6)
	s.AddConstraint(d)
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 6, l1.Length(), 1e-6)

	d.Set(3)
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 3, l1.Length(), 1e-6)
}

func TestTrimEndStub(t *testing.T) {
	s := sketch.New()
	h := s.AddLine(s.AddPoint(-5, 0), s.AddPoint(5, 0))
	s.AddLine(s.AddPoint(2, -5), s.AddPoint(2, 5)) // vertical cutter at x=2

	nl, ok := s.Trim(h, 4, 0) // pick near the right (End) stub
	require.True(t, ok)
	require.InDelta(t, -5, nl.Start.X(), 1e-9)
	require.InDelta(t, 2, nl.End.X(), 1e-9)
	require.InDelta(t, 0, nl.End.Y(), 1e-9)
	require.Len(t, s.Entities(), 2)
}

func TestTrimStartStub(t *testing.T) {
	s := sketch.New()
	h := s.AddLine(s.AddPoint(-5, 0), s.AddPoint(5, 0))
	s.AddLine(s.AddPoint(2, -5), s.AddPoint(2, 5))

	nl, ok := s.Trim(h, -4, 0) // pick near the left (Start) stub
	require.True(t, ok)
	require.InDelta(t, 2, nl.Start.X(), 1e-9)
	require.InDelta(t, 5, nl.End.X(), 1e-9)
}

func TestTrimInteriorRejected(t *testing.T) {
	s := sketch.New()
	h := s.AddLine(s.AddPoint(-5, 0), s.AddPoint(5, 0))
	s.AddLine(s.AddPoint(-2, -5), s.AddPoint(-2, 5)) // cutter left of pick
	s.AddLine(s.AddPoint(2, -5), s.AddPoint(2, 5))   // cutter right of pick

	_, ok := s.Trim(h, 0, 0) // pick on the interior portion
	require.False(t, ok, "trimming a bounded interior portion would split the line")
}

func TestTrimNoCrossing(t *testing.T) {
	s := sketch.New()
	h := s.AddLine(s.AddPoint(0, 0), s.AddPoint(5, 0))
	_, ok := s.Trim(h, 2, 0)
	require.False(t, ok)
}

func TestExtendToCutter(t *testing.T) {
	s := sketch.New()
	l := s.AddLine(s.AddPoint(0, 0), s.AddPoint(3, 0))
	s.AddLine(s.AddPoint(6, -5), s.AddPoint(6, 5)) // cutter at x=6

	nl, ok := s.Extend(l, l.End)
	require.True(t, ok)
	require.InDelta(t, 0, nl.Start.X(), 1e-9, "kept end unchanged")
	require.InDelta(t, 6, nl.End.X(), 1e-9)
	require.InDelta(t, 0, nl.End.Y(), 1e-9)
}

func TestExtendNothingBeyond(t *testing.T) {
	s := sketch.New()
	l := s.AddLine(s.AddPoint(0, 0), s.AddPoint(3, 0))
	s.AddLine(s.AddPoint(-6, -5), s.AddPoint(-6, 5)) // cutter behind the Start end

	_, ok := s.Extend(l, l.End) // nothing beyond the End end
	require.False(t, ok)
}

func TestAddFillet(t *testing.T) {
	s := sketch.New()
	// Vertical leg A(0,10)->corner; horizontal leg corner->B(10,0).
	a, corner, b := s.AddPoint(0, 10), s.AddPoint(0, 0), s.AddPoint(10, 0)
	l1, l2 := s.AddLine(a, corner), s.AddLine(corner, b)

	f, err := s.AddFillet(l1, l2, 3)
	require.NoError(t, err)

	// Ground the far ends and hold the leg directions, then solve.
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewVertical(f.L1), sketch.NewHorizontal(f.L2))
	res, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "fully constrained after grounding the legs")

	require.InDelta(t, 3, f.Arc.R(), 1e-6)
	require.InDelta(t, 0, f.T1.X(), 1e-6)
	require.InDelta(t, 3, f.T1.Y(), 1e-6)
	require.InDelta(t, 3, f.T2.X(), 1e-6)
	require.InDelta(t, 0, f.T2.Y(), 1e-6)
	require.InDelta(t, 3, f.Arc.Center.X(), 1e-6)
	require.InDelta(t, 3, f.Arc.Center.Y(), 1e-6)

	// Editing the radius keeps tangency: center-to-leg distance stays == R.
	f.Radius.Set(2)
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 2, f.Arc.R(), 1e-6)
	require.InDelta(t, 2, f.Arc.Center.X(), 1e-6, "tangent to the vertical leg x=0")
	require.InDelta(t, 2, f.Arc.Center.Y(), 1e-6, "tangent to the horizontal leg y=0")
}

func TestAddFilletNoSharedCorner(t *testing.T) {
	s := sketch.New()
	l1 := s.AddLine(s.AddPoint(0, 0), s.AddPoint(1, 0))
	l2 := s.AddLine(s.AddPoint(5, 5), s.AddPoint(6, 6))
	_, err := s.AddFillet(l1, l2, 1)
	require.ErrorIs(t, err, sketch.ErrNoSharedCorner)
	require.Len(t, s.Entities(), 2, "nothing changed")
}

func TestAddFilletInfeasible(t *testing.T) {
	s := sketch.New()
	corner := s.AddPoint(0, 0)
	l1, l2 := s.AddLine(s.AddPoint(0, 5), corner), s.AddLine(corner, s.AddPoint(5, 0))
	_, err := s.AddFillet(l1, l2, 100) // radius far larger than the legs
	require.ErrorIs(t, err, sketch.ErrFilletInfeasible)
}

func TestAddChamfer(t *testing.T) {
	s := sketch.New()
	a, corner, b := s.AddPoint(0, 10), s.AddPoint(0, 0), s.AddPoint(10, 0)
	l1, l2 := s.AddLine(a, corner), s.AddLine(corner, b)

	c, err := s.AddChamfer(l1, l2, 3)
	require.NoError(t, err)

	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewVertical(c.L1), sketch.NewHorizontal(c.L2))
	res, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF)

	require.InDelta(t, 0, c.T1.X(), 1e-6)
	require.InDelta(t, 3, c.T1.Y(), 1e-6)
	require.InDelta(t, 3, c.T2.X(), 1e-6)
	require.InDelta(t, 0, c.T2.Y(), 1e-6)
	require.InDelta(t, math.Sqrt(18), c.Cut.Length(), 1e-6)

	// D1 is the far-endpoint setback (A->T1); pulling it to 5 puts T1 at (0,5).
	c.D1.Set(5)
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, c.T1.X(), 1e-6)
	require.InDelta(t, 5, c.T1.Y(), 1e-6)
}

func TestAddFilletJSONRoundTrip(t *testing.T) {
	s := sketch.New()
	a, corner, b := s.AddPoint(0, 10), s.AddPoint(0, 0), s.AddPoint(10, 0)
	l1, l2 := s.AddLine(a, corner), s.AddLine(corner, b)
	_, err := s.AddFillet(l1, l2, 3)
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), len(s.Entities()))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "tangent+radius survive, arcRadius not doubled")
}

func TestMirrorLine(t *testing.T) {
	s := sketch.New()
	axis := s.AddLine(s.AddPoint(0, -5), s.AddPoint(0, 5)) // the y axis
	src := s.AddLine(s.AddPoint(2, 1), s.AddPoint(4, 3))

	m := s.AddMirror([]sketch.Entity{src}, axis)
	require.Len(t, m.Copies, 1)
	cp := m.Copies[0].(*sketch.Line)

	// Reflection across the y axis negates x; copy points are placed there.
	require.InDelta(t, -2, cp.Start.X(), 1e-9)
	require.InDelta(t, 1, cp.Start.Y(), 1e-9)
	require.InDelta(t, -4, cp.End.X(), 1e-9)
	require.InDelta(t, 3, cp.End.Y(), 1e-9)
}

func TestMirrorTracksSource(t *testing.T) {
	s := sketch.New()
	ax1, ax2 := s.AddPoint(0, 0), s.AddPoint(0, 5) // grounded y axis
	axis := s.AddLine(ax1, ax2)
	s.Fix(ax1)
	s.Fix(ax2)

	p1, p2 := s.AddPoint(2, 1), s.AddPoint(4, 1)
	src := s.AddLine(p1, p2)
	m := s.AddMirror([]sketch.Entity{src}, axis)
	cp := m.Copies[0].(*sketch.Line)

	// Drive the source: fix p1, widen p1->p2 to 3 so p2 lands at (5,1).
	s.Fix(p1)
	d := sketch.NewHorizontalDistance(p1, p2, 3)
	s.AddConstraint(d)
	_, err := s.Solve()
	require.NoError(t, err)

	require.InDelta(t, 5, p2.X(), 1e-6, "source moved")
	require.InDelta(t, -2, cp.Start.X(), 1e-6, "copy tracks p1's mirror")
	require.InDelta(t, -5, cp.End.X(), 1e-6, "copy tracks p2's mirror")
}

func TestMirrorCircleLinksRadius(t *testing.T) {
	s := sketch.New()
	axis := s.AddLine(s.AddPoint(0, -5), s.AddPoint(0, 5))
	c := s.AddCircle(s.AddPoint(3, 0), 2)

	m := s.AddMirror([]sketch.Entity{c}, axis)
	cp := m.Copies[0].(*sketch.Circle)
	require.InDelta(t, -3, cp.Center.X(), 1e-9)
	require.InDelta(t, 2, cp.R(), 1e-9)

	// Constraints created: one symmetric (center) + one equal-radius.
	require.Len(t, m.Constraints, 2)
}

func TestMirrorSharedVertex(t *testing.T) {
	s := sketch.New()
	axis := s.AddLine(s.AddPoint(0, -5), s.AddPoint(0, 5))
	// Two lines sharing a vertex at (4,2).
	shared := s.AddPoint(4, 2)
	l1 := s.AddLine(s.AddPoint(2, 0), shared)
	l2 := s.AddLine(shared, s.AddPoint(6, 0))

	m := s.AddMirror([]sketch.Entity{l1, l2}, axis)
	c1, c2 := m.Copies[0].(*sketch.Line), m.Copies[1].(*sketch.Line)
	require.Same(t, c1.End, c2.Start, "the shared vertex mirrors to a single shared copy")
}

func TestMirrorJSONRoundTrip(t *testing.T) {
	s := sketch.New()
	axis := s.AddLine(s.AddPoint(0, -5), s.AddPoint(0, 5))
	src := s.AddLine(s.AddPoint(2, 1), s.AddPoint(4, 3))
	s.AddMirror([]sketch.Entity{src}, axis)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), len(s.Entities()))
	require.Len(t, s2.Constraints(), len(s.Constraints()))
}

func TestPatternRect(t *testing.T) {
	s := sketch.New()
	seed := s.AddLine(s.AddPoint(0, 0), s.AddPoint(1, 0))

	p := s.AddPatternRect([]sketch.Entity{seed}, 2, 2, 5, 3)
	require.Len(t, p.Instances, 3, "2x2 grid minus the seed cell")

	// First instance is cell (1,0): the seed shifted by (5,0).
	inst := p.Instances[0].(*sketch.Line)
	require.InDelta(t, 5, inst.Start.X(), 1e-9)
	require.InDelta(t, 0, inst.Start.Y(), 1e-9)
	require.InDelta(t, 6, inst.End.X(), 1e-9)
}

func TestPatternRectTracksSeed(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	s.Fix(a)
	b := s.AddPoint(1, 0)
	seed := s.AddLine(a, b)
	p := s.AddPatternRect([]sketch.Entity{seed}, 2, 1, 5, 0)
	inst := p.Instances[0].(*sketch.Line)

	// Widen the seed; the copy must follow rigidly (offset stays 5).
	d := sketch.NewHorizontalDistance(a, b, 2)
	s.AddConstraint(d)
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 2, b.X(), 1e-6)
	require.InDelta(t, 5, inst.Start.X(), 1e-6)
	require.InDelta(t, 7, inst.End.X(), 1e-6, "copy of b tracks at b+5")
}

func TestPatternRectPanics(t *testing.T) {
	s := sketch.New()
	seed := s.AddLine(s.AddPoint(0, 0), s.AddPoint(1, 0))
	require.Panics(t, func() { s.AddPatternRect([]sketch.Entity{seed}, 0, 2, 5, 5) })
}

func TestPatternCircular(t *testing.T) {
	s := sketch.New()
	center := s.AddPoint(0, 0)
	s.Fix(center)
	c := s.AddCircle(s.AddPoint(5, 0), 1)

	p := s.AddPatternCircular([]sketch.Entity{c}, center, 4)
	require.Len(t, p.Instances, 3)
	_, err := s.Solve()
	require.NoError(t, err)

	c1 := p.Instances[0].(*sketch.Circle) // +90°
	require.InDelta(t, 0, c1.Center.X(), 1e-6)
	require.InDelta(t, 5, c1.Center.Y(), 1e-6)
	require.InDelta(t, 1, c1.R(), 1e-6, "radius tracks the seed")

	c2 := p.Instances[1].(*sketch.Circle) // +180°
	require.InDelta(t, -5, c2.Center.X(), 1e-6)
	require.InDelta(t, 0, c2.Center.Y(), 1e-6)
}

func TestPatternCircularPanics(t *testing.T) {
	s := sketch.New()
	center := s.AddPoint(0, 0)
	c := s.AddCircle(s.AddPoint(5, 0), 1)
	require.Panics(t, func() { s.AddPatternCircular([]sketch.Entity{c}, center, 1) })
}

func TestPatternRectJSONRoundTrip(t *testing.T) {
	s := sketch.New()
	seed := s.AddLine(s.AddPoint(0, 0), s.AddPoint(1, 0))
	s.AddPatternRect([]sketch.Entity{seed}, 2, 1, 5, 0)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), len(s.Entities()))
	require.Len(t, s2.Constraints(), len(s.Constraints()))
}

func TestOffsetLine(t *testing.T) {
	s := sketch.New()
	a, b := s.AddPoint(0, 0), s.AddPoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	src := s.AddLine(a, b)

	// Positive offset is the left normal of +x, i.e. +y.
	g := s.AddOffset([]sketch.Entity{src}, 2)
	dst := g.Copies[0].(*sketch.Line)
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 2, dst.Start.Y(), 1e-6)
	require.InDelta(t, 2, dst.End.Y(), 1e-6)

	// Editing the distance moves the copy.
	g.Set(5)
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, dst.Start.Y(), 1e-6)
	require.InDelta(t, 5, dst.End.Y(), 1e-6)
}

func TestOffsetChainMitresCorner(t *testing.T) {
	s := sketch.New()
	// L-shaped chain: (0,0)->(10,0)->(10,10).
	p0, p1, p2 := s.AddPoint(0, 0), s.AddPoint(10, 0), s.AddPoint(10, 10)
	s.Fix(p0)
	s.Fix(p1)
	s.Fix(p2)
	l1, l2 := s.AddLine(p0, p1), s.AddLine(p1, p2)

	g := s.AddOffset([]sketch.Entity{l1, l2}, 2)
	_, err := s.Solve()
	require.NoError(t, err)

	d1, d2 := g.Copies[0].(*sketch.Line), g.Copies[1].(*sketch.Line)
	require.Same(t, d1.End, d2.Start, "offset segments share the corner point")
	// Offset of l1 sits at y=2; offset of l2 at x=8; the shared corner is (8,2).
	require.InDelta(t, 8, d1.End.X(), 1e-6)
	require.InDelta(t, 2, d1.End.Y(), 1e-6)
}

func TestOffsetJSONRoundTrip(t *testing.T) {
	s := sketch.New()
	src := s.AddLine(s.AddPoint(0, 0), s.AddPoint(10, 0))
	s.AddOffset([]sketch.Entity{src}, 2)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), len(s.Entities()))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "offset constraint survives the round trip")
}

func TestBreakJSONRoundTrip(t *testing.T) {
	s := sketch.New()
	l := s.AddLine(s.AddPoint(0, 0), s.AddPoint(10, 0))
	_, _, ok := s.Break(l, 5, 0)
	require.True(t, ok)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), len(s.Entities()))
	require.Len(t, s2.Points(), len(s.Points()))
}
