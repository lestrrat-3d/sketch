package sketch

// In-package on purpose (see revision_internal_test.go for the repo's stance):
// the property under test is that the singular-value kernel behind
// [VerificationReport.Conditioning] — Householder bidiagonalization plus Sturm
// bisection, [singularValueExtremes] — agrees with the straightforward
// one-sided Jacobi SVD it replaced, on the raw matrix. The kernel is
// unexported and the right answer is not to export a linear-algebra routine
// just to test it. The consumer-visible half of the contract (the reported
// Conditioning, pinned to 1e-8 relative) is covered by golden_test.go and
// conditioning_test.go.

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// referenceSingularValues is the retired one-sided Jacobi SVD, verbatim: it
// orthogonalizes the n columns by plane rotations and reads the singular
// values off the converged column norms. It is the oracle here because it is
// simple enough to be obviously right, at the cost of O(sweeps·n²·m).
func referenceSingularValues(A [][]float64) []float64 {
	m := len(A)
	if m == 0 {
		return nil
	}
	n := len(A[0])
	if n == 0 {
		return nil
	}
	U := make([][]float64, n)
	for j := 0; j < n; j++ {
		col := make([]float64, m)
		for i := 0; i < m; i++ {
			col[i] = A[i][j]
		}
		U[j] = col
	}
	const maxSweeps = 60
	const eps = 1e-15
	for sweep := 0; sweep < maxSweeps; sweep++ {
		rotated := false
		for p := 0; p < n-1; p++ {
			for q := p + 1; q < n; q++ {
				var alpha, beta, gamma float64
				up, uq := U[p], U[q]
				for i := 0; i < m; i++ {
					alpha += up[i] * up[i]
					beta += uq[i] * uq[i]
					gamma += up[i] * uq[i]
				}
				if alpha == 0 || beta == 0 || math.Abs(gamma) <= eps*math.Sqrt(alpha*beta) {
					continue
				}
				zeta := (beta - alpha) / (2 * gamma)
				tval := math.Copysign(1, zeta) / (math.Abs(zeta) + math.Sqrt(1+zeta*zeta))
				cval := 1 / math.Sqrt(1+tval*tval)
				sval := cval * tval
				for i := 0; i < m; i++ {
					a, b := up[i], uq[i]
					up[i] = cval*a - sval*b
					uq[i] = sval*a + cval*b
				}
				rotated = true
			}
		}
		if !rotated {
			break
		}
	}
	sv := make([]float64, n)
	for j := 0; j < n; j++ {
		var nrm float64
		for i := 0; i < m; i++ {
			nrm += U[j][i] * U[j][i]
		}
		sv[j] = math.Sqrt(nrm)
	}
	return sv
}

// referenceExtremes returns σ_max and σ_min from the reference SVD.
func referenceExtremes(A [][]float64) (float64, float64) {
	sv := referenceSingularValues(A)
	smax, smin := sv[0], sv[0]
	for _, v := range sv {
		smax = math.Max(smax, v)
		smin = math.Min(smin, v)
	}
	return smax, smin
}

// The agreement the kernel claims against the reference on the same matrix:
// σ_max to 1e-12 relative; σ_min to 1e-12 ABSOLUTE relative to σ_max
// (bidiagonalization is backward stable, so its error is a small multiple of
// ε·σ_max whatever σ_min is); and hence the ratio to 1e-12 absolute, which is
// 1e-8 relative at the very bottom of the trust gate (1e-6 at the tightest
// tolerance) and better above it. On the gate's own region the relative bound
// is asserted directly, at 1e-10 — a hundred times tighter than the golden
// tests' 1e-8 pin.
const (
	svRelTol   = 1e-12
	svAbsTol   = 1e-12
	condRelTol = 1e-10
)

func requireExtremesAgree(t *testing.T, A [][]float64) {
	t.Helper()
	smaxRef, sminRef := referenceExtremes(A)
	smax, smin, ok := singularValueExtremes(A)
	require.True(t, ok)
	require.False(t, math.IsNaN(smax) || math.IsNaN(smin), "finite input gives finite values")
	require.InDelta(t, smaxRef, smax, svRelTol*smaxRef, "σ_max")
	require.InDelta(t, sminRef, smin, svAbsTol*smaxRef, "σ_min (absolute, relative to σ_max)")
	if smaxRef == 0 {
		require.Zero(t, smax)
		return
	}
	condRef, cond := sminRef/smaxRef, smin/smax
	require.InDelta(t, condRef, cond, svAbsTol, "Conditioning (absolute)")
	if condRef >= condTrustBase {
		require.InEpsilon(t, condRef, cond, condRelTol, "Conditioning (relative, gate region and above)")
	}
}

func randomMatrix(rng *rand.Rand, m, n int) [][]float64 {
	A := make([][]float64, m)
	for i := range A {
		A[i] = make([]float64, n)
		for j := range A[i] {
			A[i][j] = rng.NormFloat64()
		}
	}
	return A
}

// scaleColumns multiplies column j by 10^(exp·j/(n−1)), grading the columns
// geometrically down to 10^exp so the matrix is badly conditioned by scale.
func scaleColumns(A [][]float64, exp float64) {
	n := len(A[0])
	for i := range A {
		for j := range A[i] {
			A[i][j] *= math.Pow(10, exp*float64(j)/float64(n-1))
		}
	}
}

// chainJacobian returns the committed nondimensional Jacobian of a chain of
// rectangles — the shape BenchmarkVerify/large uses — so the kernel is also
// compared on a real sketch matrix (sparse, mixed row kinds, DOF 0).
func chainJacobian(t *testing.T, length int) [][]float64 {
	t.Helper()
	w := NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	var prev *Rectangle
	x := 0.0
	for k := 0; k < length; k++ {
		r := s.CreateRectangle(x, 0, x+20, 12)
		s.AddConstraint(NewDistance(r.A, r.B, 20), NewDistance(r.A, r.D, 12))
		if prev == nil {
			s.AddConstraint(NewCoincident(r.A, s.Origin()))
		} else {
			s.AddConstraint(NewCoincident(r.A, prev.B))
		}
		prev = r
		x += 20
	}
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	cj, ok := s.buildCommittedJacobian()
	require.True(t, ok)
	require.Len(t, cj.free, 8*length)
	require.Equal(t, 8*length, cj.m, "DOF 0: square")
	return cj.A
}

func TestSingularValueExtremesAgreeWithReference(t *testing.T) {
	rng := rand.New(rand.NewSource(20260903))

	t.Run("sketch jacobian", func(t *testing.T) {
		requireExtremesAgree(t, chainJacobian(t, 1))
		requireExtremesAgree(t, chainJacobian(t, 12))
	})
	t.Run("well conditioned", func(t *testing.T) {
		requireExtremesAgree(t, randomMatrix(rng, 50, 50))
		requireExtremesAgree(t, randomMatrix(rng, 100, 60))
		I := make([][]float64, 6)
		for i := range I {
			I[i] = make([]float64, 6)
			I[i][i] = 3
		}
		smax, smin, ok := singularValueExtremes(I)
		require.True(t, ok)
		require.InDelta(t, 3, smax, 1e-15)
		require.InDelta(t, 3, smin, 1e-15)
	})
	t.Run("badly conditioned", func(t *testing.T) {
		for _, exp := range []float64{-4, -6, -8, -12} {
			A := randomMatrix(rng, 60, 60)
			scaleColumns(A, exp)
			requireExtremesAgree(t, A)
		}
		// Two columns a hair apart: near-singular without any scale disparity.
		A := randomMatrix(rng, 40, 40)
		for i := range A {
			A[i][7] = A[i][2] + 1e-7*A[i][7]
		}
		requireExtremesAgree(t, A)
		smax, smin, _ := singularValueExtremes(A)
		require.Less(t, smin/smax, 1e-6, "reads below the trust gate")
		require.Greater(t, smin/smax, 1e-10)
	})
	t.Run("rank deficient", func(t *testing.T) {
		zeroCol := randomMatrix(rng, 30, 30)
		for i := range zeroCol {
			zeroCol[i][11] = 0
		}
		requireExtremesAgree(t, zeroCol)
		dupCol := randomMatrix(rng, 30, 30)
		for i := range dupCol {
			dupCol[i][3] = dupCol[i][17]
		}
		requireExtremesAgree(t, dupCol)
		rank1 := make([][]float64, 25)
		u, v := randomMatrix(rng, 25, 1), randomMatrix(rng, 1, 25)
		for i := range rank1 {
			rank1[i] = make([]float64, 25)
			for j := range rank1[i] {
				rank1[i][j] = u[i][0] * v[0][j]
			}
		}
		requireExtremesAgree(t, rank1)
		zero := make([][]float64, 5)
		for i := range zero {
			zero[i] = make([]float64, 5)
		}
		requireExtremesAgree(t, zero)
		smax, smin, ok := singularValueExtremes(zero)
		require.True(t, ok)
		require.Zero(t, smax)
		require.Zero(t, smin)
	})
	t.Run("single row and column", func(t *testing.T) {
		row := randomMatrix(rng, 1, 9)
		requireExtremesAgree(t, row)
		_, smin, _ := singularValueExtremes(row)
		require.Zero(t, smin, "nine columns over one row cannot be independent")
		col := randomMatrix(rng, 9, 1)
		requireExtremesAgree(t, col)
		smax, smin, _ := singularValueExtremes(col)
		require.Equal(t, smax, smin, "one column: its norm is the only singular value")
		requireExtremesAgree(t, randomMatrix(rng, 1, 1))
		// Wide in general: the surplus columns contribute exact zeros.
		wide := randomMatrix(rng, 20, 35)
		requireExtremesAgree(t, wide)
		_, smin, _ = singularValueExtremes(wide)
		require.Zero(t, smin)
	})
	t.Run("seeded random shapes", func(t *testing.T) {
		for k := 0; k < 300; k++ {
			m, n := 1+rng.Intn(40), 1+rng.Intn(40)
			A := randomMatrix(rng, m, n)
			if k%3 == 0 && n > 1 {
				scaleColumns(A, -8*rng.Float64())
			}
			requireExtremesAgree(t, A)
		}
	})
}

func TestSingularValueExtremesRefusals(t *testing.T) {
	_, _, ok := singularValueExtremes(nil)
	require.False(t, ok, "no rows")
	_, _, ok = singularValueExtremes([][]float64{{}, {}})
	require.False(t, ok, "no columns")
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		A := [][]float64{{1, 2}, {3, bad}}
		smax, smin, ok := singularValueExtremes(A)
		require.True(t, ok)
		require.True(t, math.IsNaN(smax), "a non-finite entry yields NaN, which the gate fails")
		require.True(t, math.IsNaN(smin))
	}
}

// TestBidiagonalizePreservesFrobeniusNorm checks the reduction on its own:
// orthogonal transforms preserve ‖·‖_F, so Σd² + Σe² must equal ‖A‖_F².
func TestBidiagonalizePreservesFrobeniusNorm(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, shape := range [][2]int{{1, 1}, {5, 1}, {8, 8}, {40, 25}, {120, 120}} {
		A := randomMatrix(rng, shape[0], shape[1])
		var fro float64
		W := make([][]float64, len(A))
		for i, row := range A {
			W[i] = append([]float64(nil), row...)
			for _, v := range row {
				fro += v * v
			}
		}
		d, e := bidiagonalize(W)
		var got float64
		for i := range d {
			got += d[i]*d[i] + e[i]*e[i]
		}
		require.InEpsilon(t, fro, got, 1e-12, "shape %v", shape)
	}
}
