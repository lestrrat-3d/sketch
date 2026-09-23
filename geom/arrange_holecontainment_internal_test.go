package geom

import (
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
