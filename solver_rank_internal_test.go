package sketch

// In-package because rankAnalysisOfMatrix is unexported, and the property worth
// pinning is that its trimmed elimination returns exactly what the full
// Gauss-Jordan elimination it replaced returns.

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// fullRankAnalysisOfMatrix is the reference: partial-pivot Gauss-Jordan
// elimination that updates every other row over every column right of the
// pivot.
func fullRankAnalysisOfMatrix(A [][]float64, n int) rankAnalysis {
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

// rankTestMatrix builds an m by n matrix with a band of nonzeros, some
// duplicated and scaled rows (so columns get rejected), some entries just
// around rankZeroTol, and optionally non-finite or huge entries.
func rankTestMatrix(rng *rand.Rand, m, n int, special bool) [][]float64 {
	A := make([][]float64, m)
	reach := 1 + rng.Intn(4)
	for i := range A {
		A[i] = make([]float64, n)
		if n == 0 {
			continue
		}
		lo := rng.Intn(n)
		for c := lo; c < min(n, lo+reach); c++ {
			switch rng.Intn(6) {
			case 0:
				A[i][c] = 0
			case 1:
				A[i][c] = rankZeroTol * (0.5 + rng.Float64())
			default:
				A[i][c] = rng.NormFloat64()
			}
		}
		if i > 0 && rng.Intn(5) == 0 {
			src := A[rng.Intn(i)]
			k := rng.NormFloat64()
			for c := range A[i] {
				A[i][c] = k * src[c]
			}
		}
		if special && rng.Intn(8) == 0 {
			c := rng.Intn(n)
			switch rng.Intn(5) {
			case 0:
				A[i][c] = math.NaN()
			case 1:
				A[i][c] = math.Inf(1 - 2*rng.Intn(2))
			case 2:
				A[i][c] = 1e300
			case 3:
				A[i][c] = math.Copysign(0, -1)
			default:
				A[i][c] = 1e-300
			}
		}
	}
	return A
}

func TestRankAnalysisOfMatrixMatchesFullElimination(t *testing.T) {
	rng := rand.New(rand.NewSource(0x4a11))
	for i := 0; i < 4000; i++ {
		m, n := rng.Intn(24), rng.Intn(24)
		A := rankTestMatrix(rng, m, n, i%3 == 0)
		want := fullRankAnalysisOfMatrix(cloneRows(A), n)
		got := rankAnalysisOfMatrix(cloneRows(A), n)
		require.Equal(t, want.rank, got.rank, "case %d", i)
		require.Equal(t, math.Float64bits(want.minAcceptedPivot), math.Float64bits(got.minAcceptedPivot), "case %d", i)
		require.Equal(t, math.Float64bits(want.maxRejectedPivot), math.Float64bits(got.maxRejectedPivot), "case %d", i)
	}
}
