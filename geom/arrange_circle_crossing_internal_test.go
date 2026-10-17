package geom

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLineCircleOtherSourceBoxRoundsOutward(t *testing.T) {
	center := float64(1 << 53)
	box, ok := fullSourceBox(circleSrc(center, 0, 1))
	require.True(t, ok)
	low := new(big.Rat).Sub(new(big.Rat).SetFloat64(center), big.NewRat(1, 1))
	high := new(big.Rat).Add(new(big.Rat).SetFloat64(center), big.NewRat(1, 1))
	require.LessOrEqual(t, new(big.Rat).SetFloat64(box[0]).Cmp(low), 0)
	require.GreaterOrEqual(t, new(big.Rat).SetFloat64(box[2]).Cmp(high), 0)
	require.Less(t, box[0], center)
	require.Greater(t, box[2], center)
	require.False(t, math.IsInf(box[0], 0))
}

func TestLineCircleBranchFollowsNaturalLineDirection(t *testing.T) {
	for _, line := range []*source{
		lineSrc(-12, -2, 12, -2), lineSrc(12, -2, -12, -2),
	} {
		for _, reversePair := range []bool{false, true} {
			first, second := line, circleSrc(0, 0, 10)
			if reversePair {
				first, second = second, first
			}
			events, ambiguous, handled := analyticEvents(first, second, 24)
			require.True(t, handled)
			require.False(t, ambiguous)
			require.Len(t, events, 2)
			require.Equal(t, int8(-1), events[0].circleBranch)
			require.Equal(t, int8(1), events[1].circleBranch)
			if line.ax < line.bx {
				require.Less(t, events[0].x, events[1].x)
			} else {
				require.Greater(t, events[0].x, events[1].x)
			}
		}
	}
}

func TestLineCircleTokenRequiresFourUnmergedArms(t *testing.T) {
	makeEdges := func(token *LineCircleCrossing) []arrEdge {
		return []arrEdge{
			{u: 0, v: 1, src: 0, circleU: token},
			{u: 0, v: 2, src: 0, circleU: token},
			{u: 0, v: 3, src: 1, circleU: token},
			{u: 0, v: 4, src: 1, circleU: token},
		}
	}
	token := &LineCircleCrossing{sources: [2]int{0, 1}, branch: 1}
	a := &arranger{edges: makeEdges(token)}
	a.certifyLineCircleCrossings()
	for _, e := range a.edges {
		require.Same(t, token, e.circleU)
	}
	a.edges = append(makeEdges(token), arrEdge{u: 0, v: 5, src: 2})
	a.certifyLineCircleCrossings()
	for _, e := range a.edges {
		require.Nil(t, e.circleU)
	}
	a.edges = makeEdges(token)
	a.edges[3].circleU = nil
	a.certifyLineCircleCrossings()
	for _, e := range a.edges {
		require.Nil(t, e.circleU)
	}
}

func TestLineCircleTokenRejectsArcSource(t *testing.T) {
	a := &arranger{
		sources:      []source{*lineSrc(-12, -2, 12, -2), *arcSrc(0, 0, 10, 0, 3)},
		exactAllowed: true,
	}
	require.Nil(t, a.lineCircleCrossing(0, 1, xEvent{
		x: 9, y: -2, kind: evCross, circleBranch: 1,
	}))
}
