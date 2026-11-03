package sketch_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkEncloseCrankRockerTurn certifies one full crank turn of decad's
// crank-rocker, the shape of workload decad's loop chain asks for.
func BenchmarkEncloseCrankRockerTurn(b *testing.B) {
	fb := crankRocker(b, 0, true)
	b.ReportAllocs()
	for b.Loop() {
		_, err := fb.s.Enclose(b.Context(), fb.crank, 0, 2*math.Pi)
		require.NoError(b, err)
	}
}
