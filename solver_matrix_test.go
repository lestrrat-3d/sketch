// This file is the package's one deliberate INTERNAL test (package sketch, not
// sketch_test), against the repository convention that tests exercise only the
// exported API. The claim under test is that the column-major normal-equation
// kernel reproduces the row-major loop it replaced BIT FOR BIT, and no public
// call can observe that: Solve reports converged geometry, and two accumulators
// differing in the last ulp still converge to geometry that passes every public
// assertion. The alternative the convention offers — an exported accessor —
// would put a solver kernel in the public API to serve a test, which is worse.
// Nothing here reaches into sketch state; it exercises two free functions.
package sketch

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// referenceNormalEquationsInto is the row-major accumulation that Sketch.lm ran
// before the column-major kernel replaced it, preserved verbatim as the
// comparison baseline: same factor order, same ascending k, same running sum
// from positive zero, same upper-triangle mirror. It reads the Jacobian as
// j[row][col] and writes into caller-provided buffers so the benchmark can time
// it without counting allocation.
//
// It takes m and n explicitly rather than deriving them from len(j): an m=0
// fixture has no row to read a column count from, and the empty shapes are
// exactly the ones a hand-derived bound gets wrong.
func referenceNormalEquationsInto(a [][]float64, g []float64, j [][]float64, r []float64, m, n int) {
	for i := 0; i < n; i++ {
		for c := i; c < n; c++ {
			var sum float64
			for k := 0; k < m; k++ {
				sum += j[k][i] * j[k][c]
			}
			a[i][c] = sum
			a[c][i] = sum
		}
		var gs float64
		for k := 0; k < m; k++ {
			gs += j[k][i] * r[k]
		}
		g[i] = gs
	}
}

// referenceNormalEquations is referenceNormalEquationsInto over freshly
// allocated outputs, which is the shape the equivalence test wants.
func referenceNormalEquations(j [][]float64, r []float64, m, n int) ([][]float64, []float64) {
	a := make([][]float64, n)
	for i := range a {
		a[i] = make([]float64, n)
	}
	g := make([]float64, n)
	referenceNormalEquationsInto(a, g, j, r, m, n)
	return a, g
}

// requireSameBitsf compares two float64 values by their IEEE bit patterns, not
// by ==. Bit equality is the actual claim, and == is too weak for it twice
// over: it reports +0 and -0 as equal, and it would report every NaN as
// unequal. The fixtures carry signed zeros deliberately and no NaN at all.
func requireSameBitsf(t *testing.T, want, got float64, format string, args ...any) {
	t.Helper()
	if math.Float64bits(want) == math.Float64bits(got) {
		return
	}
	require.Failf(t, "bit patterns differ",
		"%s: want %v (0x%016x), got %v (0x%016x)",
		fmt.Sprintf(format, args...), want, math.Float64bits(want), got, math.Float64bits(got))
}

// runColumnLayout drives the production path end to end for one fixture: the
// transpose and then the accumulation, over buffers sized the way
// lmWorkspace.alloc sizes them. Transposition is INSIDE the compared path on
// purpose — feeding a pre-transposed fixture straight to normalEquationsInto
// would pass just as happily with the index arithmetic transposed, mirrored, or
// off by a row.
func runColumnLayout(j [][]float64, r []float64, m, n int) ([][]float64, []float64) {
	ws := &lmWorkspace{}
	ws.alloc(m, n)
	for row := 0; row < m; row++ {
		copy(ws.J[row], j[row])
	}
	transposeInto(ws.JT, ws.J, m, n)
	normalEquationsInto(ws.A, ws.g, ws.JT, r, m, n)
	return ws.A, ws.g
}

func requireMatchesReference(t *testing.T, name string, j [][]float64, r []float64, m, n int) {
	t.Helper()
	wantA, wantG := referenceNormalEquations(j, r, m, n)
	gotA, gotG := runColumnLayout(j, r, m, n)
	require.Len(t, gotA, n, "%s: A row count", name)
	require.Len(t, gotG, n, "%s: g length", name)
	for i := 0; i < n; i++ {
		require.Len(t, gotA[i], n, "%s: A[%d] column count", name, i)
		for c := 0; c < n; c++ {
			requireSameBitsf(t, wantA[i][c], gotA[i][c], "%s: A[%d][%d]", name, i, c)
		}
		requireSameBitsf(t, wantG[i], gotG[i], "%s: g[%d]", name, i)
	}
}

// matrixFromColumns builds a row-major m×n Jacobian from n columns of m entries
// each, which is how the fixtures below are easiest to read: a normal-equation
// entry is a dot product of two COLUMNS, so the interesting structure
// (duplicate, zero, cancelling) is per column. m is passed rather than read off
// cols[0] because a no-column fixture still has rows, and that shape has no
// column to measure.
func matrixFromColumns(m int, cols [][]float64) [][]float64 {
	n := len(cols)
	j := make([][]float64, m)
	for row := 0; row < m; row++ {
		j[row] = make([]float64, n)
		for c := 0; c < n; c++ {
			j[row][c] = cols[c][row]
		}
	}
	return j
}

func TestNormalEquationsColumnLayoutMatchesReference(t *testing.T) {
	t.Run("fixed shapes and values", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			cols [][]float64
			r    []float64
			m, n int
		}{
			{name: "no rows and no columns", m: 0, n: 0},
			{
				name: "no rows with columns",
				cols: [][]float64{{}, {}, {}},
				m:    0, n: 3,
			},
			{
				name: "rows with no columns",
				r:    []float64{1.5, -2.5},
				m:    2, n: 0,
			},
			{
				name: "one row one column",
				cols: [][]float64{{3.25}},
				r:    []float64{-1.5},
				m:    1, n: 1,
			},
			{
				name: "one row several columns",
				cols: [][]float64{{2}, {-3}, {0.5}},
				r:    []float64{7},
				m:    1, n: 3,
			},
			{
				name: "one column several rows",
				cols: [][]float64{{1, -2, 3, -4}},
				r:    []float64{0.25, 0.5, -0.75, 1},
				m:    4, n: 1,
			},
			{
				name: "square",
				cols: [][]float64{{1, 2, 3}, {-4, 5, -6}, {7, -8, 9}},
				r:    []float64{1, -1, 2},
				m:    3, n: 3,
			},
			{
				name: "tall",
				cols: [][]float64{{1, -2, 3, -4, 5, -6}, {0.5, 0.25, -0.125, 8, -16, 32}},
				r:    []float64{1, 2, 3, 4, 5, 6},
				m:    6, n: 2,
			},
			{
				name: "wide",
				cols: [][]float64{{1, -1}, {2, 3}, {-4, 5}, {6, -7}, {0.5, 0.25}},
				r:    []float64{-2, 3},
				m:    2, n: 5,
			},
			{
				// Signed zeros survive as themselves through both paths: -0
				// times +0 is -0, and -0 + -0 stays -0 while -0 + +0 is +0, so
				// a reordered or zero-skipping accumulation shows up here as a
				// sign-bit difference that == would not report.
				name: "positive and negative zero",
				cols: [][]float64{
					{0, math.Copysign(0, -1), 0},
					{math.Copysign(0, -1), math.Copysign(0, -1), math.Copysign(0, -1)},
					{1, -1, 0},
				},
				r: []float64{math.Copysign(0, -1), 0, math.Copysign(0, -1)},
				m: 3, n: 3,
			},
			{
				name: "zero column beside nonzero ones",
				cols: [][]float64{{0, 0, 0, 0}, {1, 2, 3, 4}, {0, 0, 0, 0}, {-5, 6, -7, 8}},
				r:    []float64{1, 1, 1, 1},
				m:    4, n: 4,
			},
			{
				// Duplicate columns make A[i][j] equal along whole blocks, so a
				// mirror written from the wrong triangle still looks plausible
				// unless the values around it disagree.
				name: "duplicate columns",
				cols: [][]float64{{1.5, -2.5, 3.5}, {1.5, -2.5, 3.5}, {-9, 0.125, 4}, {1.5, -2.5, 3.5}},
				r:    []float64{2, -3, 4},
				m:    3, n: 4,
			},
			{
				// Alternating large and small terms: each product is finite and
				// well inside float64, but the running sum swings across ~16
				// orders of magnitude, so the small terms are absorbed or
				// retained depending on WHEN they are added. Any reassociation
				// or compensated summation changes these bits.
				name: "cancelling large and small terms",
				cols: [][]float64{
					{1e8, 1, -1e8, 1, 1e8, -1},
					{1e8, -1, 1e8, 1, -1e8, 1},
					{1, 1e8, 1, -1e8, 1, 1e8},
				},
				r: []float64{1e8, 1, -1e8, 1, 1e8, -1},
				m: 6, n: 3,
			},
			{
				name: "mixed signs and magnitudes",
				cols: [][]float64{
					{-1e-8, 2e7, -3.5, 4e-7},
					{5e6, -6e-9, 7.25, -8e8},
					{-9e-7, 1e9, -2.75, 3e-6},
				},
				r: []float64{1e7, -2e-7, 3e8, -4e-8},
				m: 4, n: 3,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				requireMatchesReference(t, tc.name, matrixFromColumns(tc.m, tc.cols), tc.r, tc.m, tc.n)
			})
		}
	})

	t.Run("deterministic pseudo-random matrices", func(t *testing.T) {
		// Fixed seed: a failure here must be reproducible from the test name
		// alone. Exponents stay in [-30, 30] so every product and every partial
		// sum is finite by construction — this test is about layout, and an
		// overflow fixture would only prove that Inf equals Inf.
		rng := rand.New(rand.NewPCG(0x5153, 0x4c31))
		value := func() float64 {
			return (2*rng.Float64() - 1) * math.Ldexp(1, rng.IntN(61)-30)
		}
		for c := 0; c < 120; c++ {
			m := rng.IntN(9)
			n := rng.IntN(9)
			j := make([][]float64, m)
			for row := 0; row < m; row++ {
				j[row] = make([]float64, n)
				for col := 0; col < n; col++ {
					j[row][col] = value()
				}
			}
			r := make([]float64, m)
			for k := range r {
				r[k] = value()
			}
			requireMatchesReference(t, fmt.Sprintf("random case %d (m=%d n=%d)", c, m, n), j, r, m, n)
		}
	})

	t.Run("reused workspace does not leak stale results", func(t *testing.T) {
		// lmWorkspace.alloc runs once per lm call and every later iteration
		// overwrites the same buffers, so an accumulation that skipped an entry
		// (an early continue on a zero column, say) would silently report the
		// PREVIOUS iteration's value there. Two runs on one workspace, with
		// different values in every slot, is what catches that.
		const m, n = 5, 4
		first := matrixFromColumns(m, [][]float64{
			{1, 2, 3, 4, 5}, {-1, -2, -3, -4, -5}, {0.5, 0.25, 0.125, 0.0625, 0.03125}, {9, 8, 7, 6, 5},
		})
		firstR := []float64{1, -1, 2, -2, 3}
		second := matrixFromColumns(m, [][]float64{
			{0, 0, 0, 0, 0}, {1e6, -1e-6, 2e6, -2e-6, 3e6}, {-7, 7, -7, 7, -7}, {0, 0, 0, 0, 0},
		})
		secondR := []float64{-4, 5, -6, 7, -8}

		ws := &lmWorkspace{}
		ws.alloc(m, n)
		run := func(j [][]float64, r []float64) {
			for row := 0; row < m; row++ {
				copy(ws.J[row], j[row])
			}
			transposeInto(ws.JT, ws.J, m, n)
			normalEquationsInto(ws.A, ws.g, ws.JT, r, m, n)
		}
		run(first, firstR)
		run(second, secondR)

		wantA, wantG := referenceNormalEquations(second, secondR, m, n)
		for i := 0; i < n; i++ {
			for c := 0; c < n; c++ {
				requireSameBitsf(t, wantA[i][c], ws.A[i][c], "second run A[%d][%d]", i, c)
			}
			requireSameBitsf(t, wantG[i], ws.g[i], "second run g[%d]", i)
		}
	})
}

// normalEquationsSink keeps the benchmark's results observable so the compiler
// cannot delete the accumulation it is meant to time.
var normalEquationsSink float64

func benchmarkJacobian(m, n int) ([][]float64, []float64) {
	rng := rand.New(rand.NewPCG(0x6265, 0x6e63))
	j := make([][]float64, m)
	for row := 0; row < m; row++ {
		j[row] = make([]float64, n)
		for col := 0; col < n; col++ {
			j[row][col] = 2*rng.Float64() - 1
		}
	}
	r := make([]float64, m)
	for k := range r {
		r[k] = 2*rng.Float64() - 1
	}
	return j, r
}

func BenchmarkNormalEquationsLayout(b *testing.B) {
	for _, size := range []struct{ m, n int }{{32, 16}, {128, 96}, {256, 192}} {
		j, r := benchmarkJacobian(size.m, size.n)

		// Both variants run on buffers allocated OUTSIDE the timed region, the
		// way lm's workspace is allocated once per solve and reused across
		// iterations. The transpose stays inside the new variant's timed
		// region: it is a real per-Jacobian cost the old layout did not pay,
		// and hiding it would compare the wrong two things.
		b.Run(fmt.Sprintf("row-major/%dx%d", size.m, size.n), func(b *testing.B) {
			ws := &lmWorkspace{}
			ws.alloc(size.m, size.n)
			for row := 0; row < size.m; row++ {
				copy(ws.J[row], j[row])
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				referenceNormalEquationsInto(ws.A, ws.g, ws.J, r, size.m, size.n)
				normalEquationsSink += ws.A[size.n-1][size.n-1] + ws.g[size.n-1]
			}
		})

		b.Run(fmt.Sprintf("column-major/%dx%d", size.m, size.n), func(b *testing.B) {
			ws := &lmWorkspace{}
			ws.alloc(size.m, size.n)
			for row := 0; row < size.m; row++ {
				copy(ws.J[row], j[row])
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				transposeInto(ws.JT, ws.J, size.m, size.n)
				normalEquationsInto(ws.A, ws.g, ws.JT, r, size.m, size.n)
				normalEquationsSink += ws.A[size.n-1][size.n-1] + ws.g[size.n-1]
			}
		})
	}
}
