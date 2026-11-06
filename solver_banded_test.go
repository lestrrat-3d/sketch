package sketch

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func bandedTestSystem(n, bandwidth int) ([][]float64, []float64) {
	a := make([][]float64, n)
	g := make([]float64, n)
	for i := range a {
		a[i] = make([]float64, n)
	}
	for i := range a {
		a[i][i] = 8 + float64(i)/10
		g[i] = float64(i%7-3) / 5
		for j := i + 1; j < n && j <= i+bandwidth; j++ {
			v := float64((i*3+j*5)%9-4) / 20
			a[i][j] = v
			a[j][i] = v
		}
	}
	return a, g
}

func setStructuralBand(ws *lmWorkspace, n, bandwidth int) {
	ws.structPairStart = make([]int, n+1)
	ws.structPairCols = nil
	for i := 0; i < n; i++ {
		ws.structPairStart[i] = len(ws.structPairCols)
		for j := i; j < n && j <= i+bandwidth; j++ {
			ws.structPairCols = append(ws.structPairCols, j)
		}
	}
	ws.structPairStart[n] = len(ws.structPairCols)
}

func TestBandedEliminationMatchesPivotingKernelBits(t *testing.T) {
	for _, bandwidth := range []int{0, 1, 3, maxBandedEliminationBandwidth} {
		t.Run("bandwidth_"+strconv.Itoa(bandwidth), func(t *testing.T) {
			const n = 36
			a, g := bandedTestSystem(n, bandwidth)
			ws := &lmWorkspace{}
			setStructuralBand(ws, n, bandwidth)
			gotBandwidth, ok := structuralUpperBandwidth(ws, n)
			require.True(t, ok)
			mu := 0.125

			wantM := newEliminationScratch(n)
			wantX := sentinelVector(n)
			wantNoNegZero := fillDampedSystem(wantM, a, g, n, mu)
			require.True(t, wantNoNegZero)
			wantOK := eliminateInto(wantM, n, wantNoNegZero, wantX, nil)

			gotM := newEliminationScratch(n)
			gotX := sentinelVector(n)
			gotOK := eliminateLMSystemInto(gotM, a, g, n, mu, gotBandwidth, true, gotX, nil)
			require.Equal(t, wantOK, gotOK)
			for i := range wantX {
				requireSameBitsf(t, wantX[i], gotX[i], "bandwidth %d: x[%d]", bandwidth, i)
			}
			requireSameMatrixBits(t, "banded elimination", wantM, gotM, n)
		})
	}
}

func TestBandedEliminationFallbackRebuildsDampedSystem(t *testing.T) {
	tests := []struct {
		name string
		a    [][]float64
		g    []float64
		band int
	}{
		{
			name: "pivot row swap",
			a:    [][]float64{{0, 1}, {1, 2}},
			g:    []float64{1, -1},
			band: 1,
		},
		{
			name: "support guard",
			a:    [][]float64{{4, 1}, {1, 5}},
			g:    []float64{1, 2},
			band: 0,
		},
		{
			name: "negative zero guard",
			a:    [][]float64{{4, math.Copysign(0, -1)}, {math.Copysign(0, -1), 5}},
			g:    []float64{1, 2},
			band: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := len(tc.g)
			ws := &lmWorkspace{}
			setStructuralBand(ws, n, tc.band)
			bandwidth, ok := structuralUpperBandwidth(ws, n)
			require.True(t, ok)
			mu := 1e-12

			wantM := newEliminationScratch(n)
			wantX := sentinelVector(n)
			wantNoNegZero := fillDampedSystem(wantM, tc.a, tc.g, n, mu)
			wantOK := eliminateInto(wantM, n, wantNoNegZero, wantX, nil)

			gotM := newEliminationScratch(n)
			gotX := sentinelVector(n)
			gotOK := eliminateLMSystemInto(gotM, tc.a, tc.g, n, mu, bandwidth, true, gotX, nil)
			require.Equal(t, wantOK, gotOK)
			for i := range wantX {
				requireSameBitsf(t, wantX[i], gotX[i], "%s: x[%d]", tc.name, i)
			}
			requireSameMatrixBits(t, tc.name, wantM, gotM, n)
		})
	}
}
