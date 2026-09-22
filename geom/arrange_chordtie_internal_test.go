package geom

import (
	"math"
	"testing"
)

func TestMidpointCoordinateOppositeExtremes(t *testing.T) {
	for _, endpoints := range [][2]float64{
		{-math.MaxFloat64, math.MaxFloat64},
		{math.MaxFloat64, -math.MaxFloat64},
	} {
		if got := midpointCoordinate(endpoints[0], endpoints[1]); got != 0 {
			t.Fatalf("midpointCoordinate(%g, %g) = %g, want 0", endpoints[0], endpoints[1], got)
		}
	}
}
