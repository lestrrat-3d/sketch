package sketch

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRankAnalysisBelowPivotMatchesReference(t *testing.T) {
	tests := []struct {
		name string
		rows [][]float64
		n    int
	}{
		{
			name: "row swap",
			rows: [][]float64{{0, 1}, {1, 0}},
			n:    2,
		},
		{
			name: "skipped column",
			rows: [][]float64{{0, 1, 0}, {0, 0, 1}},
			n:    3,
		},
		{
			name: "near threshold",
			rows: [][]float64{
				{rankZeroTol, 0},
				{0, math.Nextafter(rankZeroTol, 0)},
			},
			n: 2,
		},
		{
			name: "signed zero",
			rows: [][]float64{{math.Copysign(0, -1), 1}, {0, math.Copysign(0, -1)}},
			n:    2,
		},
		{
			name: "nonfinite entries",
			rows: [][]float64{
				{math.NaN(), math.Inf(1)},
				{math.Inf(-1), 1},
				{0, 2},
			},
			n: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := referenceRankAnalysisOfMatrix(cloneRankMatrix(tt.rows), tt.n)
			got := rankAnalysisOfMatrix(cloneRankMatrix(tt.rows), tt.n)
			require.Equal(t, want, got)

			cj := committedJacobian{free: make([]int, tt.n), m: len(tt.rows)}
			require.Equal(t, dof(cj, want), dof(cj, got))
			require.Equal(t, margin(cj, want), margin(cj, got))
		})
	}
}

func referenceRankAnalysisOfMatrix(A [][]float64, n int) rankAnalysis {
	m := len(A)
	ra := rankAnalysis{minAcceptedPivot: math.Inf(1)}
	row := 0
	for col := 0; col < n && row < m; col++ {
		piv := row
		best := pivotAbs(A[row][col])
		for r := row + 1; r < m; r++ {
			if v := pivotAbs(A[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < rankZeroTol {
			if best > ra.maxRejectedPivot {
				ra.maxRejectedPivot = best
			}
			continue
		}
		if best < ra.minAcceptedPivot {
			ra.minAcceptedPivot = best
		}
		A[row], A[piv] = A[piv], A[row]
		for r := 0; r < m; r++ {
			if r == row {
				continue
			}
			f := A[r][col] / A[row][col]
			if f == 0 {
				continue
			}
			for c := col; c < n; c++ {
				A[r][c] -= f * A[row][c]
			}
		}
		row++
	}
	ra.rank = row
	return ra
}

func cloneRankMatrix(rows [][]float64) [][]float64 {
	copyRows := make([][]float64, len(rows))
	for i := range rows {
		copyRows[i] = append([]float64(nil), rows[i]...)
	}
	return copyRows
}
