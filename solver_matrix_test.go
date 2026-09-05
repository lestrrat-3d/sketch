// This file is the package's one deliberate INTERNAL test (package sketch, not
// sketch_test), against the repository convention that tests exercise only the
// exported API. The claim under test is that the column-major normal-equation
// kernel reproduces the row-major loop it replaced BIT FOR BIT, and that the
// sparse kernel in turn reproduces the column-major one BIT FOR BIT. No public
// call can observe either: Solve reports converged geometry, and two
// accumulators differing in the last ulp still converge to geometry that passes
// every public assertion. The alternative the convention offers — an exported
// accessor — would put a solver kernel in the public API to serve a test, which
// is worse. Only the real-Jacobian subtest reaches into sketch state, and it
// reads the same buffers Sketch.lm does; the rest exercises free functions.
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

// normalEquationsFixture is one hand-written Jacobian, given as its columns
// (see matrixFromColumns), plus the residual vector it is accumulated against.
type normalEquationsFixture struct {
	name string
	cols [][]float64
	r    []float64
	m, n int
}

// layoutFixtures are the shapes and values the column-layout equivalence rests
// on. They are a named set rather than an inline table because the sparse
// kernel (TestNormalEquationsSparseMatchesReference) must clear every one of
// them too: it is the same claim about the same accumulation, so a fixture that
// is worth checking against one kernel is worth checking against both.
func layoutFixtures() []normalEquationsFixture {
	return []normalEquationsFixture{
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
			// Signed zeros are where a reassociated accumulation shows up
			// as a sign-bit difference that == would not report: -0 times
			// +0 is -0, and -0 + -0 stays -0 while -0 + +0 is +0. Every
			// sum here still starts at +0, which is exactly why the sparse
			// kernel may drop the zero terms and land on the same bits.
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
	}
}

// generatedMatrix is a Jacobian built by one of the generators below, already
// row-major, with the name a failure is reported under.
type generatedMatrix struct {
	name string
	j    [][]float64
	r    []float64
	m, n int
}

// randomLayoutMatrices returns count deterministic pseudo-random matrices with
// a fixed seed, so a failure is reproducible from the test name alone.
// Exponents stay in [-30, 30] so every product and every partial sum is finite
// by construction — these fixtures are about layout, and an overflow fixture
// would only prove that Inf equals Inf. Shapes are drawn too, empty ones
// included.
func randomLayoutMatrices(count int) []generatedMatrix {
	rng := rand.New(rand.NewPCG(0x5153, 0x4c31))
	value := func() float64 {
		return (2*rng.Float64() - 1) * math.Ldexp(1, rng.IntN(61)-30)
	}
	out := make([]generatedMatrix, 0, count)
	for c := 0; c < count; c++ {
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
		out = append(out, generatedMatrix{
			name: fmt.Sprintf("random case %d (m=%d n=%d)", c, m, n),
			j:    j, r: r, m: m, n: n,
		})
	}
	return out
}

// randomDensityMatrix returns one m×n matrix whose entries are nonzero with
// probability density, drawn from a seed derived from the density so each
// density has its own reproducible stream. Density is what the sparse kernel's
// benefit and its risk both scale with: a low density leaves most pairs sharing
// no row at all, and 1 leaves it doing the dense kernel's work through an index
// list.
func randomDensityMatrix(name string, m, n int, density float64, seed uint64) generatedMatrix {
	rng := rand.New(rand.NewPCG(0x53324e45, seed))
	value := func() float64 {
		if rng.Float64() >= density {
			return 0
		}
		return (2*rng.Float64() - 1) * math.Ldexp(1, rng.IntN(41)-20)
	}
	j := make([][]float64, m)
	for row := 0; row < m; row++ {
		j[row] = make([]float64, n)
		for col := 0; col < n; col++ {
			j[row][col] = value()
		}
	}
	r := make([]float64, m)
	for k := range r {
		r[k] = (2*rng.Float64() - 1) * math.Ldexp(1, rng.IntN(41)-20)
	}
	return generatedMatrix{name: name, j: j, r: r, m: m, n: n}
}

func TestNormalEquationsColumnLayoutMatchesReference(t *testing.T) {
	t.Run("fixed shapes and values", func(t *testing.T) {
		for _, tc := range layoutFixtures() {
			t.Run(tc.name, func(t *testing.T) {
				requireMatchesReference(t, tc.name, matrixFromColumns(tc.m, tc.cols), tc.r, tc.m, tc.n)
			})
		}
	})

	t.Run("deterministic pseudo-random matrices", func(t *testing.T) {
		for _, tc := range randomLayoutMatrices(120) {
			requireMatchesReference(t, tc.name, tc.j, tc.r, tc.m, tc.n)
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

// loadWorkspace puts one fixture into a workspace exactly the way Sketch.lm
// leaves it just before the accumulation runs: the row-major Jacobian in J, its
// transpose in JT, and the residual vector in r.
func loadWorkspace(ws *lmWorkspace, j [][]float64, r []float64, m, n int) {
	for row := 0; row < m; row++ {
		copy(ws.J[row], j[row])
	}
	transposeInto(ws.JT, ws.J, m, n)
	ws.r = r
}

// runSparseLayout drives the sparse kernel over a freshly sized workspace. Like
// runColumnLayout it includes the transpose, and it additionally includes the
// index-list build, because that is where a sparsity pattern can go wrong.
func runSparseLayout(j [][]float64, r []float64, m, n int) ([][]float64, []float64) {
	ws := &lmWorkspace{}
	ws.alloc(m, n)
	loadWorkspace(ws, j, r, m, n)
	normalEquationsSparseInto(ws, m, n)
	return ws.A, ws.g
}

// requireSparseMatchesBoth asserts the sparse kernel's output against BOTH
// earlier kernels. Comparing only against normalEquationsInto would let a
// shared misreading of the layout pass twice; comparing only against the
// row-major reference would not pin the sparse kernel to the code actually
// shipping in Sketch.lm.
func requireSparseMatchesBoth(t *testing.T, name string, j [][]float64, r []float64, m, n int) {
	t.Helper()
	refA, refG := referenceNormalEquations(j, r, m, n)
	denseA, denseG := runColumnLayout(j, r, m, n)
	gotA, gotG := runSparseLayout(j, r, m, n)
	require.Len(t, gotA, n, "%s: A row count", name)
	require.Len(t, gotG, n, "%s: g length", name)
	for i := 0; i < n; i++ {
		require.Len(t, gotA[i], n, "%s: A[%d] column count", name, i)
		for c := 0; c < n; c++ {
			requireSameBitsf(t, refA[i][c], gotA[i][c], "%s: A[%d][%d] against the row-major reference", name, i, c)
			requireSameBitsf(t, denseA[i][c], gotA[i][c], "%s: A[%d][%d] against the dense kernel", name, i, c)
		}
		requireSameBitsf(t, refG[i], gotG[i], "%s: g[%d] against the row-major reference", name, i)
		requireSameBitsf(t, denseG[i], gotG[i], "%s: g[%d] against the dense kernel", name, i)
	}
}

// sparseFixtures are the structural shapes the sparse kernel adds on top of
// layoutFixtures: patterns where whole columns or whole column PAIRS carry no
// shared nonzero, which is the case the dense kernel never distinguishes and
// this one takes a different path through.
func sparseFixtures() []normalEquationsFixture {
	negZero := math.Copysign(0, -1)
	return []normalEquationsFixture{
		{
			// Nothing to accumulate anywhere: every entry of A and of g is the
			// positive zero the clearing pass wrote, and the dense kernel agrees
			// because every one of its terms is an exact zero added to a sum that
			// started positive.
			name: "every entry zero",
			cols: [][]float64{{0, 0, 0}, {0, 0, 0}},
			r:    []float64{1, -2, 3},
			m:    3, n: 2,
		},
		{
			// No two columns share a row, so every off-diagonal pair is skipped
			// entirely and only the diagonal is accumulated.
			name: "columns share no rows",
			cols: [][]float64{{1, 0, 0, 0}, {0, 2, 0, 0}, {0, 0, -3, 0}},
			r:    []float64{1, -1, 2, -2},
			m:    4, n: 3,
		},
		{
			name: "exactly one nonzero",
			cols: [][]float64{{0, 0, 0}, {0, 5, 0}, {0, 0, 0}},
			r:    []float64{0, 0, 0},
			m:    3, n: 3,
		},
		{
			// The skipped-term claim is a claim about SIGN as much as value: the
			// dense kernel adds -0 terms here that the sparse one drops, and both
			// must still land on +0, because a sum that starts at +0 stays there.
			// Column 0 and column 1 have their single nonzeros in different rows,
			// so their whole pair is one skipped-term sum.
			name: "negative zero beside positive zero in one column",
			cols: [][]float64{
				{negZero, 0, 1},
				{2, negZero, negZero},
			},
			r: []float64{negZero, 0, negZero},
			m: 3, n: 2,
		},
		{
			// Finite entries whose products overflow. Every sum here is +Inf or a
			// finite value; none mixes signs, so none becomes NaN and the
			// skipped-zero argument still holds (adding an exact zero to an
			// infinity returns that infinity).
			name: "products overflow to one-signed infinity",
			cols: [][]float64{
				{1e200, 1e200, 0},
				{1e200, 0, 1e200},
				{0, 1e200, 1e200},
			},
			r: []float64{1e200, 1e200, 1e200},
			m: 3, n: 3,
		},
	}
}

func TestNormalEquationsSparseMatchesReference(t *testing.T) {
	t.Run("column layout fixtures", func(t *testing.T) {
		for _, tc := range layoutFixtures() {
			t.Run(tc.name, func(t *testing.T) {
				requireSparseMatchesBoth(t, tc.name, matrixFromColumns(tc.m, tc.cols), tc.r, tc.m, tc.n)
			})
		}
	})

	t.Run("structural zero fixtures", func(t *testing.T) {
		for _, tc := range sparseFixtures() {
			t.Run(tc.name, func(t *testing.T) {
				requireSparseMatchesBoth(t, tc.name, matrixFromColumns(tc.m, tc.cols), tc.r, tc.m, tc.n)
			})
		}
	})

	t.Run("deterministic pseudo-random matrices", func(t *testing.T) {
		for _, tc := range randomLayoutMatrices(120) {
			requireSparseMatchesBoth(t, tc.name, tc.j, tc.r, tc.m, tc.n)
		}
	})

	t.Run("random matrices by density", func(t *testing.T) {
		for _, density := range []float64{0.01, 0.05, 0.25, 1} {
			for _, size := range []struct{ m, n int }{{7, 5}, {24, 18}, {64, 40}} {
				name := fmt.Sprintf("density %g at %dx%d", density, size.m, size.n)
				t.Run(name, func(t *testing.T) {
					tc := randomDensityMatrix(name, size.m, size.n,
						density, uint64(size.m*1000+size.n)^math.Float64bits(density))
					requireSparseMatchesBoth(t, tc.name, tc.j, tc.r, tc.m, tc.n)
				})
			}
		}
	})

	t.Run("non-finite input falls back to the dense kernel", func(t *testing.T) {
		// Each fixture pairs a non-finite entry with an exact zero, so the two
		// kernels can only agree if the guard actually fired: the dense product
		// 0*NaN (or 0*Inf) is NaN, while a skipped term would leave the +0 the
		// clearing pass wrote. Comparison is against normalEquationsInto only —
		// the equivalence claim is a finite-input claim, and this subtest is
		// about which kernel ran, not about NaN arithmetic.
		for _, tc := range []normalEquationsFixture{
			{
				name: "NaN in the Jacobian",
				cols: [][]float64{{0, 0}, {math.NaN(), 1}},
				r:    []float64{1, 1},
				m:    2, n: 2,
			},
			{
				name: "infinity in the Jacobian",
				cols: [][]float64{{0, 0}, {math.Inf(1), 1}},
				r:    []float64{1, 1},
				m:    2, n: 2,
			},
			{
				name: "negative infinity in the Jacobian",
				cols: [][]float64{{0, 0}, {math.Inf(-1), 1}},
				r:    []float64{1, 1},
				m:    2, n: 2,
			},
			{
				name: "NaN in the residual vector",
				cols: [][]float64{{1, 0}},
				r:    []float64{0, math.NaN()},
				m:    2, n: 1,
			},
			{
				name: "infinity in the residual vector",
				cols: [][]float64{{1, 0}},
				r:    []float64{0, math.Inf(1)},
				m:    2, n: 1,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				j := matrixFromColumns(tc.m, tc.cols)
				wantA, wantG := runColumnLayout(j, tc.r, tc.m, tc.n)
				gotA, gotG := runSparseLayout(j, tc.r, tc.m, tc.n)
				for i := 0; i < tc.n; i++ {
					for c := 0; c < tc.n; c++ {
						requireSameBitsf(t, wantA[i][c], gotA[i][c], "%s: A[%d][%d]", tc.name, i, c)
					}
					requireSameBitsf(t, wantG[i], gotG[i], "%s: g[%d]", tc.name, i)
				}
			})
		}
	})

	t.Run("reused workspace does not leak stale results", func(t *testing.T) {
		// The sparse kernel writes a pair's slot only when the pair shares a row,
		// so a pattern that loses a pair between iterations must still see the
		// slot cleared — in BOTH triangles, since the lower one is written
		// nowhere else. Columns 0 and 1 share rows 0 and 1 in the first call and
		// share nothing in the second.
		const m, n = 3, 2
		first := matrixFromColumns(m, [][]float64{{1, 2, 0}, {3, 4, 0}})
		firstR := []float64{1, -1, 2}
		second := matrixFromColumns(m, [][]float64{{1, 0, 0}, {0, 5, 0}})
		secondR := []float64{-4, 5, -6}

		ws := &lmWorkspace{}
		ws.alloc(m, n)
		loadWorkspace(ws, first, firstR, m, n)
		normalEquationsSparseInto(ws, m, n)
		require.NotZero(t, ws.A[0][1], "the first call must leave a nonzero pair to go stale")

		loadWorkspace(ws, second, secondR, m, n)
		normalEquationsSparseInto(ws, m, n)

		wantA, wantG := referenceNormalEquations(second, secondR, m, n)
		for i := 0; i < n; i++ {
			for c := 0; c < n; c++ {
				requireSameBitsf(t, wantA[i][c], ws.A[i][c], "second run A[%d][%d]", i, c)
			}
			requireSameBitsf(t, wantG[i], ws.g[i], "second run g[%d]", i)
		}
	})

	t.Run("reused workspace does not carry a pattern across calls", func(t *testing.T) {
		// The pattern buffers live on the workspace and outlive a call, so a
		// bookkeeping value left set by one call must not suppress work in the
		// next. n == 1 is the shape with the least room to hide it: the sketch has
		// exactly one entry, so a carried-over mark loses the whole result.
		const m, n = 3, 1
		j := matrixFromColumns(m, [][]float64{{2, 3, 4}})
		r := []float64{1, 1, 1}

		ws := &lmWorkspace{}
		ws.alloc(m, n)
		loadWorkspace(ws, j, r, m, n)
		normalEquationsSparseInto(ws, m, n)
		loadWorkspace(ws, j, r, m, n)
		normalEquationsSparseInto(ws, m, n)

		wantA, wantG := referenceNormalEquations(j, r, m, n)
		requireSameBitsf(t, wantA[0][0], ws.A[0][0], "second run A[0][0]")
		requireSameBitsf(t, wantG[0], ws.g[0], "second run g[0]")
	})

	t.Run("real solved sketch Jacobians", func(t *testing.T) {
		// Hand-written fixtures choose their own sparsity. These do not: they are
		// the matrices Sketch.lm actually accumulates, at the configuration a
		// real solve leaves the geometry in.
		for _, tc := range sparseJacobianSketches(t) {
			t.Run(tc.name, func(t *testing.T) {
				free := tc.s.freeVars()
				n := len(free)
				r := tc.s.residuals(nil)
				m := len(r)
				require.Positive(t, m, "%s: no residual rows to build a Jacobian from", tc.name)
				require.Positive(t, n, "%s: no free variables to build a Jacobian over", tc.name)

				ws := &lmWorkspace{}
				ws.alloc(m, n)
				tc.s.jacobianInto(ws.J, free, m, tc.s.residuals, ws.rp, ws.rm)

				var zeros int
				for row := 0; row < m; row++ {
					for col := 0; col < n; col++ {
						if ws.J[row][col] == 0 {
							zeros++
						}
					}
				}
				require.Positive(t, zeros,
					"%s: a fully dense Jacobian would exercise none of the skipping", tc.name)
				t.Logf("%s: %dx%d Jacobian, %d of %d entries zero", tc.name, m, n, zeros, m*n)

				requireSparseMatchesBoth(t, tc.name, ws.J, r, m, n)
			})
		}
	})
}

// TestNormalEquationsSparseNoAllocation pins the "no iteration allocates"
// half of the design: the index lists live in the workspace and are refilled
// through their existing capacity, so a solve pays for them once in
// lmWorkspace.alloc and never again.
func TestNormalEquationsSparseNoAllocation(t *testing.T) {
	const m, n = 64, 48
	tc := randomDensityMatrix("allocation", m, n, 0.05, 0x415a)
	ws := &lmWorkspace{}
	ws.alloc(m, n)
	loadWorkspace(ws, tc.j, tc.r, m, n)
	normalEquationsSparseInto(ws, m, n) // warm the workspace, as one lm iteration would

	allocs := testing.AllocsPerRun(100, func() {
		normalEquationsSparseInto(ws, m, n)
	})
	require.Zero(t, allocs, "the sparse kernel allocated on a warmed workspace")
}

// sparseSketchCase is one solved sketch whose real Jacobian the sparse kernel is
// checked against.
type sparseSketchCase struct {
	name string
	s    *Sketch
}

// sparseJacobianSketches builds and solves three sketches spanning the geometry
// the solver meets in practice: line-only with axis constraints, a triangle
// driven by three distances, and a mix of a circle and a line. Each is anchored
// the parametric way — one coincidence to the origin plus one orientation
// constraint — so the free variables are real unknowns rather than pinned ones.
func sparseJacobianSketches(t *testing.T) []sparseSketchCase {
	t.Helper()

	w1 := NewWorld()
	rect, err := w1.CreateSketch(w1.XY())
	require.NoError(t, err)
	r0 := rect.CreatePoint(0.3, -0.2)
	r1 := rect.CreatePoint(40, 1)
	r2 := rect.CreatePoint(41, 25)
	r3 := rect.CreatePoint(-1, 24)
	rect.AddConstraint(
		NewCoincident(r0, rect.Origin()),
		NewHorizontal(rect.CreateLine(r0, r1)),
		NewVertical(rect.CreateLine(r1, r2)),
		NewHorizontal(rect.CreateLine(r2, r3)),
		NewVertical(rect.CreateLine(r3, r0)),
		NewHorizontalDistance(r0, r1, 40),
		NewVerticalDistance(r0, r3, 25),
	)

	w2 := NewWorld()
	tri, err := w2.CreateSketch(w2.XY())
	require.NoError(t, err)
	t0 := tri.CreatePoint(0.1, 0.1)
	t1 := tri.CreatePoint(30, 2)
	t2 := tri.CreatePoint(12, 26)
	tri.CreateLine(t0, t1)
	tri.CreateLine(t1, t2)
	tri.CreateLine(t2, t0)
	tri.AddConstraint(
		NewCoincident(t0, tri.Origin()),
		NewHorizontalPoints(t0, t1),
		NewDistance(t0, t1, 30),
		NewDistance(t1, t2, 28),
		NewDistance(t2, t0, 26),
	)

	w3 := NewWorld()
	mixed, err := w3.CreateSketch(w3.XY())
	require.NoError(t, err)
	center := mixed.CreatePoint(1, 1)
	q0 := mixed.CreatePoint(-20, -12)
	q1 := mixed.CreatePoint(20, -11)
	circle := mixed.CreateCircle(center, 8)
	base := mixed.CreateLine(q0, q1)
	mixed.AddConstraint(
		NewCoincident(center, mixed.Origin()),
		NewHorizontal(base),
		NewRadius(circle, 8),
		NewDistancePointLine(center, base, 12),
		NewHorizontalDistance(q0, q1, 40),
	)

	out := []sparseSketchCase{
		{name: "grounded rectangle", s: rect},
		{name: "triangle from three distances", s: tri},
		{name: "circle above a line", s: mixed},
	}
	for _, tc := range out {
		_, err := tc.s.Solve(t.Context())
		require.NoError(t, err, "%s: solve", tc.name)
	}
	return out
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

		// The sparse kernel is timed against the dense one at the SAME density,
		// so each pair of names is a ratio that can be read directly. Density 1
		// is the case where the index lists buy nothing and are pure overhead;
		// it is reported rather than omitted, because that is the price a dense
		// Jacobian pays for the sparse one's benefit. The index-list build is
		// inside the timed region — it is per-Jacobian work, rebuilt every
		// iteration by design.
		for _, density := range []float64{1, 0.05} {
			dj, dr := j, r
			if density < 1 {
				g := randomDensityMatrix("bench", size.m, size.n, density, uint64(size.m*1000+size.n))
				dj, dr = g.j, g.r
			}
			label := fmt.Sprintf("d%d", int(density*100))

			b.Run(fmt.Sprintf("column-major-%s/%dx%d", label, size.m, size.n), func(b *testing.B) {
				ws := &lmWorkspace{}
				ws.alloc(size.m, size.n)
				loadWorkspace(ws, dj, dr, size.m, size.n)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					transposeInto(ws.JT, ws.J, size.m, size.n)
					normalEquationsInto(ws.A, ws.g, ws.JT, ws.r, size.m, size.n)
					normalEquationsSink += ws.A[size.n-1][size.n-1] + ws.g[size.n-1]
				}
			})

			b.Run(fmt.Sprintf("sparse-%s/%dx%d", label, size.m, size.n), func(b *testing.B) {
				ws := &lmWorkspace{}
				ws.alloc(size.m, size.n)
				loadWorkspace(ws, dj, dr, size.m, size.n)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					transposeInto(ws.JT, ws.J, size.m, size.n)
					normalEquationsSparseInto(ws, size.m, size.n)
					normalEquationsSink += ws.A[size.n-1][size.n-1] + ws.g[size.n-1]
				}
			})
		}
	}
}
