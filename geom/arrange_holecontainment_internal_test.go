package geom

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollapsedTwoArcCycleFindsTrueInteriorProbe(t *testing.T) {
	a := &arranger{sources: []source{
		{kind: srcArc, cx: 0, cy: 0, r: 1, phi0: 0, sweep: math.Pi},
		{kind: srcArc, cx: 0, cy: 0, r: 1, phi0: math.Pi, sweep: math.Pi},
	}}
	c := &cycle{
		dense: [][2]float64{{1, 0}, {-1, 0}},
		frags: []cycFrag{{src: 0, pStart: 0, pEnd: 1}, {src: 1, pStart: 0, pEnd: 1}},
	}
	_, ok := interiorPoint(c.dense)
	require.False(t, ok, "a two-vertex polygon cannot provide an interior point")
	q, ok := a.cycleInteriorPoint(c)
	require.True(t, ok, "the true arcs enclose a disk")
	require.Less(t, q[0]*q[0]+q[1]*q[1], 1.0)
}

func TestCollapsedArcCircleCycleUsesTrueOrientation(t *testing.T) {
	const y = 0.0541523093145383
	a := &arranger{
		sources: []source{
			{kind: srcArc, cx: 5, cy: y, r: 5, phi0: math.Pi, sweep: math.Pi},
			{kind: srcCircle, cx: 10 - 1.7302966257556793, cy: y, r: 1.7302966257556793},
		},
		comp: []int{-1, -1},
		verts: vertexTable{
			xs: []float64{9.999782179763441, 10},
			ys: []float64{0.026697987341014192, y},
		},
		edges: []arrEdge{
			{src: 0, pu: 0.9982522785940858, pv: 1},
			{src: 1, pu: 0.9974746096431135, pv: 1},
		},
		halfs: []halfEdge{
			{from: 0, to: 1, edge: 0, forward: true},
			{from: 1, to: 0, edge: 1, forward: false},
		},
	}
	c := a.makeCycle([]int{0, 1})
	require.Len(t, c.dense, 2)
	require.InDelta(t, 1.30344e-6, c.area, 1e-10)
}

func lineCycleForBoundsTest(a *arranger, pts ...[2]float64) cycle {
	var c cycle
	for i, p := range pts {
		end := pts[(i+1)%len(pts)]
		a.sources = append(a.sources, source{
			kind: srcLine, ax: p[0], ay: p[1], bx: end[0], by: end[1],
		})
		c.frags = append(c.frags, cycFrag{src: len(a.sources) - 1, pStart: 0, pEnd: 1})
	}
	return c
}

func TestHoleLiesInFaceUsesCycleLocalRoundoff(t *testing.T) {
	a := &arranger{scale: 1e9}
	face := lineCycleForBoundsTest(a,
		[2]float64{-1100000, 0}, [2]float64{1100000, 8}, [2]float64{-1100000, 12})
	disjoint := lineCycleForBoundsTest(a,
		[2]float64{-1000000, -1e-6}, [2]float64{1000000, -1e-6}, [2]float64{0, 2})
	contained, ok := a.holeLiesInFace(&disjoint, &face)
	require.True(t, ok)
	require.False(t, contained, "a distant source cannot widen the postcondition past a local gap")

	outer := lineCycleForBoundsTest(a,
		[2]float64{-10, -10}, [2]float64{10, -10},
		[2]float64{10, 10}, [2]float64{-10, 10})
	inner := lineCycleForBoundsTest(a,
		[2]float64{-2, -2}, [2]float64{2, -2},
		[2]float64{2, 2}, [2]float64{-2, 2})
	contained, ok = a.holeLiesInFace(&inner, &outer)
	require.True(t, ok)
	require.True(t, contained, "a genuine nested hole still satisfies the postcondition")
}

func TestHoleLiesInFaceRejectsDisjointNestedBoxes(t *testing.T) {
	a := &arranger{sources: []source{
		{kind: srcLine, ax: -1100000, ay: 0, bx: 1100000, by: 8},
		{kind: srcLine, ax: 1100000, ay: 8, bx: -1100000, by: 12},
		{kind: srcLine, ax: -1100000, ay: 12, bx: -1100000, by: 0},
		{kind: srcLine, ax: -800000, ay: 0.2, bx: 800000, by: 0.2},
		{kind: srcLine, ax: 800000, ay: 0.2, bx: 0, by: 1.8},
		{kind: srcLine, ax: 0, ay: 1.8, bx: -800000, by: 0.2},
	}}
	face := &cycle{frags: []cycFrag{{src: 0, pEnd: 1}, {src: 1, pEnd: 1}, {src: 2, pEnd: 1}}}
	hole := &cycle{frags: []cycFrag{{src: 3, pEnd: 1}, {src: 4, pEnd: 1}, {src: 5, pEnd: 1}}}
	contained, ok := a.holeLiesInFace(hole, face)
	require.True(t, ok)
	require.False(t, contained)
}

func TestHoleLiesInFaceKeepsTangentCircle(t *testing.T) {
	a := &arranger{sources: []source{
		{kind: srcCircle, cx: 0, cy: 0, r: 5, sweep: 2 * math.Pi},
		{kind: srcCircle, cx: -4.8, cy: 0, r: 0.2, sweep: 2 * math.Pi},
	}}
	face := &cycle{frags: []cycFrag{{src: 0, pEnd: 1}}}
	hole := &cycle{frags: []cycFrag{{src: 1, pEnd: 1}}}
	contained, ok := a.holeLiesInFace(hole, face)
	require.True(t, ok)
	require.True(t, contained)
}

func TestHoleLiesInFaceKeepsNestedArcCycle(t *testing.T) {
	a := &arranger{sources: []source{
		{kind: srcCircle, cx: 0, cy: 0, r: 5, sweep: 2 * math.Pi},
		{kind: srcArc, cx: 0, cy: 0, r: 1, sweep: math.Pi},
		{kind: srcArc, cx: 0, cy: 0, r: 1, phi0: math.Pi, sweep: math.Pi},
	}}
	face := &cycle{frags: []cycFrag{{src: 0, pEnd: 1}}}
	hole := &cycle{frags: []cycFrag{{src: 1, pEnd: 1}, {src: 2, pEnd: 1}}}
	contained, ok := a.holeLiesInFace(hole, face)
	require.True(t, ok)
	require.True(t, contained)
}

func TestHoleExitsCircleFaceChecksFragmentRadialMaximum(t *testing.T) {
	a := &arranger{sources: []source{
		{kind: srcCircle, cx: 0, cy: 0, r: 5},
		{kind: srcLine, ax: 4.5, ay: 0, bx: 5.1, by: 0},
		{kind: srcCircle, cx: 4.5, cy: 0, r: 0.7},
		{kind: srcArc, cx: 4.5, cy: 0, r: 0.7, phi0: -math.Pi / 2, sweep: math.Pi},
		{kind: srcCircle, cx: 4.5, cy: 0, r: 0.4},
		{kind: srcArc, cx: 4.5, cy: 0, r: 0.7, phi0: math.Pi / 2, sweep: math.Pi},
	}}
	face := &cycle{frags: []cycFrag{{src: 0, pEnd: 1}}}
	for _, src := range []int{1, 2, 3} {
		hole := &cycle{frags: []cycFrag{{src: src, pEnd: 1}}}
		require.True(t, a.holeExitsCircleFace(hole, face), "source %d exceeds radius 5", src)
	}
	hole := &cycle{frags: []cycFrag{{src: 4, pEnd: 1}}}
	require.False(t, a.holeExitsCircleFace(hole, face))
	hole = &cycle{frags: []cycFrag{{src: 5, pEnd: 1}}}
	require.False(t, a.holeExitsCircleFace(hole, face), "the far radial point is outside this arc")
}

func notchedFace() ([]source, *cycle) {
	points := [][2]float64{{0, 0}, {10, 0}, {10, 10}, {6.9, 10},
		{6.9, 6.5}, {6.6, 6.5}, {6.6, 10}, {0, 10}}
	sources := make([]source, len(points))
	face := &cycle{frags: make([]cycFrag, len(points))}
	for i, p := range points {
		q := points[(i+1)%len(points)]
		sources[i] = source{kind: srcLine, ax: p[0], ay: p[1], bx: q[0], by: q[1]}
		face.frags[i] = cycFrag{src: i, pEnd: 1}
	}
	return sources, face
}

func TestHoleLiesInFaceChecksCurvedExcursionBetweenWitnesses(t *testing.T) {
	sources, face := notchedFace()
	sources = append(sources, source{kind: srcCircle, cx: 5, cy: 6.1, r: 2, sweep: 2 * math.Pi})
	a := &arranger{sources: sources}
	hole := &cycle{frags: []cycFrag{{src: len(sources) - 1, pEnd: 1}}}
	// The circle crosses the narrow notch between its cardinal points.
	contained, ok := a.holeLiesInFace(hole, face)
	require.True(t, ok)
	require.False(t, contained)
}

func TestHoleLiesInFaceChecksExcursionFromFragmentEndpoint(t *testing.T) {
	sources, face := notchedFace()
	sources = append(sources, source{kind: srcLine, ax: 6.6, ay: 7, bx: 6.9, by: 7})
	a := &arranger{sources: sources}
	hole := &cycle{frags: []cycFrag{{src: len(sources) - 1, pEnd: 1}}}
	// Both contacts are at the fragment's endpoints; its interior is outside.
	contained, ok := a.holeLiesInFace(hole, face)
	require.True(t, ok)
	require.False(t, contained)
}
