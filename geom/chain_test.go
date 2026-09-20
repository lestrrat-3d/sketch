package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// chainPoints is the chain's own walk, as points: the first edge's polyline
// followed by each later one minus its duplicated joint.
func chainPoints(c *geom.Chain) [][2]float64 {
	var out [][2]float64
	for i, e := range c.Edges {
		if i > 0 {
			out = append(out, e.Polyline[1:]...)
			continue
		}
		out = append(out, e.Polyline...)
	}
	return out
}

func TestChainsOpenPolyline(t *testing.T) {
	a := geom.NewPoint(0, 0)
	b := geom.NewPoint(10, 0)
	c := geom.NewPoint(10, 6)
	arr := geom.Regions([]geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c)}, nil)

	require.Empty(t, arr.Regions, "an open run bounds nothing")
	require.Len(t, arr.Chains, 1)
	ch := arr.Chains[0]
	require.Len(t, ch.Edges, 2)
	require.Equal(t, [][2]float64{{0, 0}, {10, 0}, {10, 6}}, chainPoints(ch))
	require.InDelta(t, 16, ch.Length, 1e-9)
	require.False(t, ch.SelfIntersecting)
	require.False(t, ch.Degenerate)
	for i, e := range ch.Edges {
		require.True(t, e.Whole, "edge %d is a whole line", i)
		require.True(t, e.TExact)
	}
}

func TestChainsNoneForAClosedRegion(t *testing.T) {
	arr := geom.Regions(square(0, 0, 10), nil)
	require.Len(t, arr.Regions, 1)
	require.Empty(t, arr.Chains, "every edge bounds the region")
}

func TestChainsSpurOffARegion(t *testing.T) {
	// A square with a tail leaving one corner: the region keeps its four sides,
	// the tail is the one chain, and the corner (degree 3) is where they part.
	curves := square(0, 0, 10)
	curves = append(curves, geom.NewLine(geom.NewPoint(10, 10), geom.NewPoint(16, 14)))
	arr := geom.Regions(curves, nil)

	require.Len(t, arr.Regions, 1)
	require.InDelta(t, 100, arr.Regions[0].Area, 1e-9, "the spur adds no area")
	require.Len(t, arr.Chains, 1)
	require.Len(t, arr.Chains[0].Edges, 1)
	require.InDelta(t, math.Hypot(6, 4), arr.Chains[0].Length, 1e-9)
	require.Equal(t, [][2]float64{{10, 10}, {16, 14}}, chainPoints(arr.Chains[0]))
}

func TestChainsCutAtABranchVertex(t *testing.T) {
	hub := geom.NewPoint(0, 0)
	arr := geom.Regions([]geom.Curve{
		geom.NewLine(hub, geom.NewPoint(10, 0)),
		geom.NewLine(hub, geom.NewPoint(-10, 0)),
		geom.NewLine(hub, geom.NewPoint(0, 10)),
	}, nil)

	require.Len(t, arr.Chains, 3, "no walk crosses a degree-3 vertex")
	for _, ch := range arr.Chains {
		require.Len(t, ch.Edges, 1)
		require.InDelta(t, 10, ch.Length, 1e-9)
	}
}

func TestChainsCrossingLinesAreCutAtTheCrossing(t *testing.T) {
	l1 := geom.NewLine(geom.NewPoint(-5, 0), geom.NewPoint(5, 0))
	l2 := geom.NewLine(geom.NewPoint(0, -5), geom.NewPoint(0, 5))
	arr := geom.Regions([]geom.Curve{l1, l2}, nil)

	require.Empty(t, arr.Regions)
	require.Len(t, arr.Chains, 4, "four half-lines")
	for _, ch := range arr.Chains {
		require.Len(t, ch.Edges, 1)
		require.False(t, ch.Edges[0].Whole, "each is a fragment of its line")
		require.InDelta(t, 5, ch.Length, 1e-9)
	}
}

func TestChainsOrderIsIndependentOfInputOrder(t *testing.T) {
	mk := func() []geom.Curve {
		return []geom.Curve{
			geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(4, 0)),
			geom.NewLine(geom.NewPoint(20, 3), geom.NewPoint(26, 3)),
			geom.NewLine(geom.NewPoint(-9, 1), geom.NewPoint(-4, 1)),
		}
	}
	forward := geom.Regions(mk(), nil)
	shuffled := mk()
	shuffled[0], shuffled[2] = shuffled[2], shuffled[0]
	reordered := geom.Regions(shuffled, nil)

	require.Len(t, forward.Chains, 3)
	require.Equal(t, len(forward.Chains), len(reordered.Chains))
	for i := range forward.Chains {
		require.Equal(t, chainPoints(forward.Chains[i]), chainPoints(reordered.Chains[i]),
			"chain %d: the published order is stated in coordinates", i)
	}
	require.Equal(t, [2]float64{-9, 1}, chainPoints(forward.Chains[0])[0], "leftmost first")
}

// TestChainsWithIdenticalWalksKeepSourceOrder pins the tie chainLess leaves, and
// the seam it leaves it at. Three coincident lines walk one and the same
// polyline, so no coordinate ranks them; what this package publishes is the
// SourceIndex order, unchanged by which curve was handed in first, and a caller
// with an identity of its own settles the rest (sketch.Sketch.Chains does, by
// entity name).
func TestChainsWithIdenticalWalksKeepSourceOrder(t *testing.T) {
	arr := geom.Regions([]geom.Curve{
		geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(10, 0)),
		geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(10, 0)),
		geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(10, 0)),
	}, nil)

	require.Len(t, arr.Chains, 3)
	require.True(t, arr.Degenerate, "coincident duplicates are an unresolvable overlap")
	var srcs []int
	for _, ch := range arr.Chains {
		require.Equal(t, [][2]float64{{0, 0}, {10, 0}}, chainPoints(ch), "one walk, three times")
		require.True(t, ch.Degenerate)
		require.Len(t, ch.Edges, 1)
		srcs = append(srcs, ch.Edges[0].SourceIndex)
	}
	require.Equal(t, []int{0, 1, 2}, srcs, "the tie keeps SourceIndex order, whatever it maps to")
}

func TestChainsWalkFromTheSmallerEnd(t *testing.T) {
	// Authored right-to-left; published left-to-right, so two arrangements of the
	// same drawing publish the same walk.
	arr := geom.Regions([]geom.Curve{geom.NewLine(geom.NewPoint(9, 2), geom.NewPoint(1, 2))}, nil)
	require.Len(t, arr.Chains, 1)
	require.Equal(t, [][2]float64{{1, 2}, {9, 2}}, chainPoints(arr.Chains[0]))
	require.True(t, arr.Chains[0].Edges[0].Reversed, "the walk runs against the line's natural direction")
	require.Equal(t, 0.0, arr.Chains[0].Edges[0].TStart, "the range stays in the natural direction")
	require.Equal(t, 1.0, arr.Chains[0].Edges[0].TEnd)
}

func TestChainsArcLengthIsClosedForm(t *testing.T) {
	// A quarter circle of radius 5, joined to nothing.
	arr := geom.Regions([]geom.Curve{
		geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(5, 0), geom.NewPoint(0, 5)),
	}, nil)
	require.Len(t, arr.Chains, 1)
	require.InDelta(t, 5*math.Pi/2, arr.Chains[0].Length, 1e-12, "exact, not the chord sum")
}
