package geom

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

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
