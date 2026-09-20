package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/units"
	"github.com/stretchr/testify/require"
)

// chainStart is the chain's first walked point, chainEnd its last — the two free
// ends, read off the emitted polylines rather than off the entities, so a
// reversed edge is followed the way the walk runs.
func chainStart(c *sketch.Chain) [2]float64 {
	return c.Edges[0].Polyline[0]
}

func chainEnd(c *sketch.Chain) [2]float64 {
	last := c.Edges[len(c.Edges)-1].Polyline
	return last[len(last)-1]
}

// TestChainsOpenPolyline is C1: three lines joined end to end, with no closed
// region anywhere, publish ONE chain of three whole edges in walk order.
func TestChainsOpenPolyline(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(10, 6)
	d := s.CreatePoint(2, 6)
	l1 := s.CreateLine(a, b)
	l2 := s.CreateLine(b, c)
	l3 := s.CreateLine(c, d)

	require.Empty(t, s.Profiles(), "an open polyline closes nothing")
	chains := s.Chains()
	require.Len(t, chains, 1, "one connected open run")
	ch := chains[0]
	require.Equal(t, []sketch.Entity{l1, l2, l3}, ch.Entities, "walk order, de-duplicated")
	require.Len(t, ch.Edges, 3, "one edge per line")
	for i, e := range ch.Edges {
		require.False(t, e.Partial, "edge %d spans its whole line", i)
		require.True(t, e.TExact, "an all line/circle/arc sketch publishes exact bounds")
		require.Equal(t, 0.0, e.TStart)
		require.Equal(t, 1.0, e.TEnd)
	}
	require.Equal(t, [2]float64{0, 0}, chainStart(ch), "walked from the lexicographically smaller end")
	require.Equal(t, [2]float64{2, 6}, chainEnd(ch))
	require.InDelta(t, 10+6+8, ch.Length, 1e-9, "arc length is exact for lines")
	require.True(t, ch.Valid)
	require.False(t, ch.SelfIntersecting)
	require.Same(t, s, ch.Sketch())
	require.False(t, ch.IsStale())
}

// TestChainsClosedLoopPublishesNoChain is C2: closing the same run with a fourth
// line moves every edge to the region publication, so no edge is ever reported
// by both.
func TestChainsClosedLoopPublishesNoChain(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(10, 6)
	d := s.CreatePoint(0, 6)
	s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, d)
	s.CreateLine(d, a)

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "the four lines close one region")
	require.Len(t, profiles[0].Outer, 4)
	require.Empty(t, s.Chains(), "every edge bounds the region")
}

// TestChainsCrossingLinesSplitAtTheCrossing is C3: two lines crossing at their
// midpoints are split by the arrangement, and the four resulting fragments are
// published as four chains — the crossing vertex has degree 4, so no walk
// continues through it. Each fragment carries its own sub-range.
func TestChainsCrossingLinesSplitAtTheCrossing(t *testing.T) {
	s := newSketch(t)
	h := s.CreateLine(s.CreatePoint(-5, 0), s.CreatePoint(5, 0))
	v := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))

	require.Empty(t, s.Profiles(), "a bare crossing encloses nothing")
	chains := s.Chains()
	require.Len(t, chains, 4, "four half-lines meet at the crossing")
	for _, ch := range chains {
		require.Len(t, ch.Edges, 1, "each walk stops at the degree-4 vertex")
		e := ch.Edges[0]
		require.True(t, e.Partial, "each edge is half of its line")
		require.True(t, e.TExact, "a line/line crossing is cut in closed form")
		require.InDelta(t, 0.5, e.TEnd-e.TStart, 1e-9, "half the parameter range")
		require.InDelta(t, 5, ch.Length, 1e-9, "half of a length-10 line")
		require.True(t, ch.Valid)
	}
	// Published in coordinate order, each walked from its smaller end.
	require.Equal(t, [2]float64{-5, 0}, chainStart(chains[0]))
	require.Equal(t, [2]float64{0, 0}, chainEnd(chains[0]))
	require.Equal(t, h, chains[0].Entities[0])
	require.Equal(t, v, chains[1].Entities[0], "the vertical line's lower half")
	require.Equal(t, [2]float64{0, -5}, chainStart(chains[1]))
}

// TestChainsLineJoinedToArc is C4: a line and an arc sharing a point publish one
// chain, and the arc edge's exactness there is what the same arc reports inside a
// region boundary.
func TestChainsLineJoinedToArc(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	start := s.CreatePoint(5, 0)
	end := s.CreatePoint(0, 5)
	arc := s.CreateArc(center, start, end)
	line := s.CreateLine(end, s.CreatePoint(-5, 5))

	chains := s.Chains()
	require.Len(t, chains, 1, "the shared point joins them into one walk")
	ch := chains[0]
	require.Len(t, ch.Edges, 2)
	require.ElementsMatch(t, []sketch.Entity{arc, line}, ch.Entities)

	var arcEdge sketch.BoundaryEdge
	for _, e := range ch.Edges {
		if e.Entity == arc {
			arcEdge = e
		}
	}
	require.False(t, arcEdge.Partial, "the whole arc is on the chain")
	require.InDelta(t, 5*math.Pi/2+5, ch.Length, 1e-9, "quarter circle plus the line")

	// The same arc, closed into a region by two lines: the boundary edge it
	// produces there reports the same exactness the chain edge reports.
	closed := newSketch(t)
	c2 := closed.CreatePoint(0, 0)
	s2 := closed.CreatePoint(5, 0)
	e2 := closed.CreatePoint(0, 5)
	arc2 := closed.CreateArc(c2, s2, e2)
	closed.CreateLine(e2, c2)
	closed.CreateLine(c2, s2)
	profiles := closed.Profiles()
	require.Len(t, profiles, 1)
	var regionArcEdge sketch.BoundaryEdge
	for _, e := range profiles[0].Outer {
		if e.Entity == arc2 {
			regionArcEdge = e
		}
	}
	require.Equal(t, regionArcEdge.TExact, arcEdge.TExact, "one exactness contract for both publications")
	require.True(t, arcEdge.TExact, "and in an all line/circle/arc sketch it holds")
}

// TestChainsFreeFormWithholdsExactness is C5: one free-form entity on the chain
// makes every edge of every chain inexact, by the same whole-sketch gate that
// governs a profile's boundary.
func TestChainsFreeFormWithholdsExactness(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	start := s.CreatePoint(6, 0)
	end := s.CreatePoint(0, 3)
	s.CreateEllipticalArc(center, start, end, 6, 3, 0)
	s.CreateLine(end, s.CreatePoint(-6, 3))

	chains := s.Chains()
	require.Len(t, chains, 1)
	for i, e := range chains[0].Edges {
		require.False(t, e.TExact, "edge %d: one free-form entity withholds exactness sketch-wide", i)
	}
	require.True(t, chains[0].Valid, "inexact bounds are not an invalidity")
}

// TestChainsSelfTouchIsInvalid is C6: a chain that doubles back over itself
// reports SelfIntersecting and is not valid.
func TestChainsSelfTouchIsInvalid(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.CreateLine(a, b)
	s.CreateLine(b, s.CreatePoint(2, 0)) // back along the first line

	chains := s.Chains()
	require.Len(t, chains, 1, "the shared point joins them into one walk")
	require.True(t, chains[0].SelfIntersecting, "the walk retraces its own path")
	require.False(t, chains[0].Valid)
}

// TestChainsResolvedSelfCrossingSplitsTheWalk pins the other half of that rule: a
// self-crossing the arrangement RESOLVES is a vertex, so the walk is cut there
// and the pieces are separate, valid chains rather than one flagged one.
func TestChainsResolvedSelfCrossingSplitsTheWalk(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(10, 8)
	d := s.CreatePoint(4, -2)
	s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, d) // crosses the first line at y = 0

	require.Len(t, s.Profiles(), 1, "the crossing closes a triangle")
	chains := s.Chains()
	require.Len(t, chains, 2, "the two tails outside the triangle")
	for _, ch := range chains {
		require.False(t, ch.SelfIntersecting, "each piece is a simple run")
		require.True(t, ch.Valid)
	}
}

// TestChainsUnattributableDegeneracyInvalidatesEvery is C7: a zero-radius circle
// is dropped before it reaches the arrangement, so what it would have subdivided
// is unknown and EVERY chain reads invalid — the rule Profile.Valid already
// applies.
func TestChainsUnattributableDegeneracyInvalidatesEvery(t *testing.T) {
	s := newSketch(t)
	s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
	s.CreateCircle(s.CreatePoint(40, 40), 0) // unusable input, far away

	chains := s.Chains()
	require.Len(t, chains, 1)
	require.False(t, chains[0].Valid, "an unattributable condition reaches every chain")
	require.False(t, chains[0].SelfIntersecting, "it is the degeneracy, not the walk")
}

// TestChainsAttributableDegeneracyIsScoped is its converse: a condition on curves
// a chain does not use leaves that chain valid.
func TestChainsAttributableDegeneracyIsScoped(t *testing.T) {
	s := newSketch(t)
	// Two coincident collinear lines far away: a flagged overlap on their curves.
	s.CreateLine(s.CreatePoint(100, 0), s.CreatePoint(110, 0))
	s.CreateLine(s.CreatePoint(100, 0), s.CreatePoint(110, 0))
	// An unrelated open run.
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(0, 10)
	s.CreateLine(a, b)

	var mine *sketch.Chain
	for _, ch := range s.Chains() {
		if chainStart(ch) == [2]float64{0, 0} && chainEnd(ch) == [2]float64{0, 10} {
			mine = ch
		}
	}
	require.NotNil(t, mine, "the unrelated run is published")
	require.True(t, mine.Valid, "the overlap is on curves this chain does not use")
}

// TestChainsGoStale is C8: a chain is a snapshot, and a solve that moves the
// geometry makes the held one detectably stale while a fresh call reports the
// moved geometry.
func TestChainsGoStale(t *testing.T) {
	s := newSketch(t)
	require.NoError(t, s.Params().SetValue("width", units.Millimeters(30)))
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(10, 6)
	line := s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewVerticalDistance(b, c, 6))
	s.AddConstraint(sketch.NewHorizontalDistance(b, c, 0))
	width := sketch.NewHorizontalDistance(a, b, 10)
	s.AddConstraint(width)
	require.NoError(t, s.Bind(width, s.Params(), "width"))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	held := s.Chains()
	require.Len(t, held, 1)
	require.False(t, held[0].IsStale())
	require.InDelta(t, 36, held[0].Length, 1e-6, "30 across plus 6 up")

	require.NoError(t, s.Params().SetValue("width", units.Millimeters(50)))
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, held[0].IsStale(), "the sketch moved under the held chain")
	require.InDelta(t, 36, held[0].Length, 1e-6, "the held chain still describes the old geometry")

	fresh := s.Chains()
	require.Len(t, fresh, 1)
	require.False(t, fresh[0].IsStale())
	require.InDelta(t, 56, fresh[0].Length, 1e-6, "the fresh chain reports the moved geometry")
}

// TestChainsSplitAtABranchVertex is C9: three lines meeting at a point are three
// chains, since no walk through a degree-3 vertex is justified.
func TestChainsSplitAtABranchVertex(t *testing.T) {
	s := newSketch(t)
	hub := s.CreatePoint(0, 0)
	s.CreateLine(hub, s.CreatePoint(10, 0))
	s.CreateLine(hub, s.CreatePoint(-10, 0))
	s.CreateLine(hub, s.CreatePoint(0, 10))

	chains := s.Chains()
	require.Len(t, chains, 3, "one chain per branch")
	for _, ch := range chains {
		require.Len(t, ch.Edges, 1)
		require.False(t, ch.Edges[0].Partial, "each branch is a whole line")
		require.InDelta(t, 10, ch.Length, 1e-9)
		require.True(t, ch.Valid)
	}
}

// TestChainsAreDeterministic is C10: arranging the same sketch twice publishes
// the same chains, in the same order and the same walk direction — which is what
// makes a consumer's snapshot comparison meaningful.
func TestChainsAreDeterministic(t *testing.T) {
	s := newSketch(t)
	hub := s.CreatePoint(0, 0)
	s.CreateLine(hub, s.CreatePoint(10, 0))
	s.CreateLine(hub, s.CreatePoint(-10, 4))
	s.CreateLine(hub, s.CreatePoint(0, 10))
	s.CreateArc(s.CreatePoint(20, 0), s.CreatePoint(25, 0), s.CreatePoint(20, 5))

	first, second := s.Chains(), s.Chains()
	require.Len(t, first, 4)
	require.Equal(t, len(first), len(second))
	for i := range first {
		require.Equal(t, first[i].Entities, second[i].Entities, "chain %d: same entities", i)
		require.Equal(t, first[i].Edges, second[i].Edges, "chain %d: same walk", i)
		require.Equal(t, first[i].Length, second[i].Length)
	}
	// Coordinate order, not entity order: the chain starting furthest left first.
	require.Equal(t, [2]float64{-10, 4}, chainStart(first[0]))
}

// TestChainsCoincidentWalksOrderByName pins the tie-break UNDER the coordinate
// order. Three coincident lines walk the identical polyline, so no coordinate
// ranks one above another, and with nothing further to say the published order
// would be the order the lines were authored in. The names on the entities rank
// them instead, so every authoring order publishes one and the same list.
func TestChainsCoincidentWalksOrderByName(t *testing.T) {
	published := func(authored []string) []string {
		s := newSketch(t)
		for _, name := range authored {
			line := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
			line.SetName(name)
		}
		chains := s.Chains()
		require.Len(t, chains, len(authored), "one chain per coincident line")
		out := make([]string, 0, len(chains))
		for _, ch := range chains {
			require.Len(t, ch.Entities, 1, "each chain is one line")
			require.Equal(t, [2]float64{0, 0}, chainStart(ch), "the same walk, every time")
			require.Equal(t, [2]float64{10, 0}, chainEnd(ch))
			require.False(t, ch.Valid, "a coincident overlap is a degenerate arrangement")
			out = append(out, ch.Entities[0].Name())
		}
		return out
	}
	want := []string{"A", "B", "C"}
	require.Equal(t, want, published([]string{"A", "B", "C"}))
	require.Equal(t, want, published([]string{"C", "A", "B"}))
	require.Equal(t, want, published([]string{"B", "C", "A"}))
	require.Equal(t, want, published([]string{"C", "B", "A"}))
}

// TestChainsCoincidentWalksKeepTheCoordinateOrder pins the other half of that
// rule: the name only ever settles a tie. Two chains the coordinates DO rank
// keep that ranking whatever their names say.
func TestChainsCoincidentWalksKeepTheCoordinateOrder(t *testing.T) {
	s := newSketch(t)
	right := s.CreateLine(s.CreatePoint(20, 0), s.CreatePoint(30, 0))
	right.SetName("A")
	left := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
	left.SetName("Z")

	chains := s.Chains()
	require.Len(t, chains, 2)
	require.Equal(t, []sketch.Entity{left}, chains[0].Entities, "leftmost first, name notwithstanding")
	require.Equal(t, []sketch.Entity{right}, chains[1].Entities)
}

// TestChainsExcludeConstruction pins that the two publications share one rule
// about construction geometry: it is excluded from both.
func TestChainsExcludeConstruction(t *testing.T) {
	s := newSketch(t)
	line := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
	guide := s.CreateLine(s.CreatePoint(0, 5), s.CreatePoint(10, 5))
	guide.SetConstruction(true)

	chains := s.Chains()
	require.Len(t, chains, 1, "the construction line publishes nothing")
	require.Equal(t, []sketch.Entity{line}, chains[0].Entities)

	guide.SetConstruction(false)
	require.Len(t, s.Chains(), 2, "clearing the flag admits it")
}

// TestChainsClosedRunPublishesNothing pins the closed-run rule: a loop that
// bounds no published region (here its area falls below the arrangement's floor
// beside a far larger scene) is published as no chain either. A Chain is open by
// definition.
func TestChainsClosedRunPublishesNothing(t *testing.T) {
	s := newSketch(t)
	s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(1e6, 0)) // sets the scene scale
	s.CreateCircle(s.CreatePoint(0, 10), 0.5)                // area far under the floor

	require.Empty(t, s.Profiles(), "the circle bounds no publishable region")
	chains := s.Chains()
	require.Len(t, chains, 1, "only the open line")
	require.Len(t, chains[0].Entities, 1)
	_, isLine := chains[0].Entities[0].(*sketch.Line)
	require.True(t, isLine, "a closed run is not published as a chain")
}

// TestChainsAndProfilesPartitionTheSketch pins the partition invariant on a mixed
// sketch: every entity edge is published exactly once, by one publication or the
// other.
func TestChainsAndProfilesPartitionTheSketch(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(0, 0, 20, 12)
	tail := s.CreateLine(s.CreatePoint(30, 0), s.CreatePoint(40, 0))
	s.CreateLine(s.CreatePoint(40, 0), s.CreatePoint(40, 9))

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "the rectangle")
	inRegion := map[sketch.Entity]struct{}{}
	for _, e := range profiles[0].Outer {
		inRegion[e.Entity] = struct{}{}
	}
	chains := s.Chains()
	require.Len(t, chains, 1, "the open tail")
	require.Len(t, chains[0].Edges, 2)
	for _, e := range chains[0].Edges {
		require.NotContains(t, inRegion, e.Entity, "an edge is published once, not twice")
	}
	require.Contains(t, chains[0].Entities, sketch.Entity(tail))
	require.InDelta(t, 19, chains[0].Length, 1e-9)
}
