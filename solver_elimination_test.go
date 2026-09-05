// This file, like solver_matrix_test.go, is a deliberate INTERNAL test (package
// sketch): the claim under test is that the guarded zero-multiplier shortcut in
// solveLinearInto reproduces the elimination it replaces BIT FOR BIT, on the
// solved vector AND on the working matrix, and no public call can observe
// either. Solve reports converged geometry, and two elimination kernels
// differing in the last ulp still converge to geometry that passes every public
// assertion. Exporting the kernel to serve a test would be worse.
package sketch

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// referenceSolveLinearInto is solveLinearInto's elimination as it stood before
// the zero-multiplier shortcut, preserved verbatim as the comparison baseline:
// same copy-in, same partial-pivot search, same 1e-15 singularity threshold,
// same unconditional row update over the same ascending c, same back
// substitution. Nothing in it may be "tidied" — its value is that it is the old
// code, not that it is good code.
func referenceSolveLinearInto(M [][]float64, A [][]float64, b, x []float64) bool {
	n := len(b)
	for i := 0; i < n; i++ {
		copy(M[i], A[i])
		M[i][n] = b[i]
	}
	for col := 0; col < n; col++ {
		piv := col
		best := math.Abs(M[col][col])
		for r := col + 1; r < n; r++ {
			if v := math.Abs(M[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < 1e-15 {
			return false
		}
		M[col], M[piv] = M[piv], M[col]
		for r := col + 1; r < n; r++ {
			f := M[r][col] / M[col][col]
			for c := col; c <= n; c++ {
				M[r][c] -= f * M[col][c]
			}
		}
	}
	for i := n - 1; i >= 0; i-- {
		sum := M[i][n]
		for c := i + 1; c < n; c++ {
			sum -= M[i][c] * x[c]
		}
		x[i] = sum / M[i][i]
	}
	return true
}

// eliminationSystem is one square system A·x = b handed to both kernels.
type eliminationSystem struct {
	name string
	a    [][]float64
	b    []float64
}

// newEliminationScratch allocates the n×(n+1) working matrix both kernels take,
// shaped exactly the way lmWorkspace.alloc shapes ws.M.
func newEliminationScratch(n int) [][]float64 {
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n+1)
	}
	return m
}

func cloneRows(a [][]float64) [][]float64 {
	out := make([][]float64, len(a))
	for i, row := range a {
		out[i] = append([]float64(nil), row...)
	}
	return out
}

// eliminationSentinel is written into x before every run so "x is left
// unmodified when the kernel refuses" is checkable rather than assumed: a
// kernel that wrote zeros on the refusal path would be indistinguishable from
// one that wrote nothing if x started at zero.
const eliminationSentinel = -12345.75

func sentinelVector(n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = eliminationSentinel
	}
	return x
}

// requireSameMatrixBits compares two working matrices entry by entry. The
// matrix is compared as well as the solution because it is the elimination's
// real output: back substitution reads only the upper triangle and the
// augmented column, so a divergence anywhere below the diagonal would otherwise
// pass unnoticed until some later fixture happened to pivot on it.
func requireSameMatrixBits(t *testing.T, name string, want, got [][]float64, n int) {
	t.Helper()
	require.Len(t, got, len(want), "%s: working matrix row count", name)
	for i := 0; i < n; i++ {
		for c := 0; c <= n; c++ {
			requireSameBitsf(t, want[i][c], got[i][c], "%s: M[%d][%d]", name, i, c)
		}
	}
}

// requireEliminationMatchesReference runs both kernels over private copies of
// one system and asserts they agree on everything a caller can observe: the
// success result, every bit of x, every bit of the working matrix, and the
// untouched state of the caller's A and b.
func requireEliminationMatchesReference(t *testing.T, sys eliminationSystem) {
	t.Helper()
	n := len(sys.b)

	pristineA, pristineB := cloneRows(sys.a), append([]float64(nil), sys.b...)

	refA, refB := cloneRows(sys.a), append([]float64(nil), sys.b...)
	refM, refX := newEliminationScratch(n), sentinelVector(n)
	wantOK := referenceSolveLinearInto(refM, refA, refB, refX)

	gotA, gotB := cloneRows(sys.a), append([]float64(nil), sys.b...)
	gotM, gotX := newEliminationScratch(n), sentinelVector(n)
	gotOK := solveLinearInto(gotM, gotA, gotB, gotX)

	require.Equal(t, wantOK, gotOK, "%s: success result", sys.name)
	for i := 0; i < n; i++ {
		requireSameBitsf(t, refX[i], gotX[i], "%s: x[%d]", sys.name, i)
	}
	requireSameMatrixBits(t, sys.name, refM, gotM, n)

	// Input immutability, against the caller's originals rather than against
	// the reference run's copies: both kernels promise A and b are not written.
	for i := 0; i < n; i++ {
		for c := range pristineA[i] {
			requireSameBitsf(t, pristineA[i][c], gotA[i][c], "%s: A[%d][%d] was modified", sys.name, i, c)
		}
		requireSameBitsf(t, pristineB[i], gotB[i], "%s: b[%d] was modified", sys.name, i)
	}
}

// eliminationFixtures are the hand-written systems the shortcut's guards are
// argued on, one family per row of the task's regression matrix.
func eliminationFixtures() []eliminationSystem {
	negZero := math.Copysign(0, -1)
	tiny := math.SmallestNonzeroFloat64
	return []eliminationSystem{
		{name: "empty system", a: [][]float64{}, b: []float64{}},
		{name: "one variable", a: [][]float64{{2.5}}, b: []float64{-5}},
		{name: "one variable singular", a: [][]float64{{0}}, b: []float64{1}},
		{
			// The shortcut's best case: every off-diagonal multiplier is zero.
			name: "diagonal",
			a:    [][]float64{{2, 0, 0, 0}, {0, -4, 0, 0}, {0, 0, 0.5, 0}, {0, 0, 0, 8}},
			b:    []float64{1, 2, 3, 4},
		},
		{
			name: "block diagonal",
			a: [][]float64{
				{2, 1, 0, 0, 0, 0},
				{1, 3, 0, 0, 0, 0},
				{0, 0, 4, -1, 0, 0},
				{0, 0, -1, 5, 0, 0},
				{0, 0, 0, 0, 6, 2},
				{0, 0, 0, 0, 2, 7},
			},
			b: []float64{1, -1, 2, -2, 3, -3},
		},
		{
			// Nonsymmetric on purpose: rowCombo (diagnose.go) hands this kernel
			// an upper-triangular Gram matrix, so nothing may assume symmetry
			// or positive definiteness.
			name: "dense nonsymmetric",
			a: [][]float64{
				{3, -1, 2, 7},
				{0.5, 4, -6, 1},
				{-2, 8, 1.25, -3},
				{9, 0.125, -4, 5},
			},
			b: []float64{1, -2, 3, -4},
		},
		{
			name: "upper triangular as rowCombo builds it",
			a: [][]float64{
				{2, 0.5, -1, 3},
				{0, 4, 2, -1},
				{0, 0, 1.5, 0.25},
				{0, 0, 0, 6},
			},
			b: []float64{1, 1, 1, 1},
		},
		{
			// A zero leading entry forces a swap at column 0, and the second
			// column forces another one row down, so later pivots read rows
			// that have already moved.
			name: "rows requiring pivot swaps",
			a: [][]float64{
				{0, 1, 2},
				{0, 0, 3},
				{4, 5, 6},
			},
			b: []float64{1, 2, 3},
		},
		{
			name: "singular: duplicate rows",
			a:    [][]float64{{1, 2, 3}, {1, 2, 3}, {4, 5, 6}},
			b:    []float64{1, 2, 3},
		},
		{
			name: "singular: zero column",
			a:    [][]float64{{1, 0, 3}, {4, 0, 6}, {7, 0, 9}},
			b:    []float64{1, 2, 3},
		},
		{
			// Just above and just below the 1e-15 pivot threshold, so a kernel
			// that moved the acceptance boundary by a hair would disagree on
			// the success result rather than on a value.
			name: "pivot just above the threshold",
			a:    [][]float64{{1, 1}, {1, 1 + 2e-15}},
			b:    []float64{1, 1},
		},
		{
			name: "pivot just below the threshold",
			a:    [][]float64{{1, 1}, {1, 1 + 1e-16}},
			b:    []float64{1, 1},
		},
		{
			// The fixture the negative-zero guard exists for. Column 0 gives
			// row 1 a multiplier of exactly +0; the augmented column then holds
			// -0 against a positive pivot entry, so the reference computes
			// -0 − (+0 × 1) = -0 − (+0) ... which stays -0. Flip the pivot's
			// sign (below) and it does not.
			name: "negative zero in b against a positive pivot row",
			a:    [][]float64{{1, 2}, {0, 3}},
			b:    []float64{1, negZero},
		},
		{
			// Same shape, negative b[0]: the product +0 × -1 is -0, and
			// -0 − (-0) is +0, so the reference TURNS the augmented entry
			// positive where an unguarded skip would leave it negative. The
			// sign survives back substitution into x[1].
			name: "negative zero in b flipped by the update",
			a:    [][]float64{{1, 2}, {0, 3}},
			b:    []float64{-1, negZero},
		},
		{
			name: "negative zero in the matrix body",
			a:    [][]float64{{-1, 2, 3}, {0, negZero, 1}, {0, 4, negZero}},
			b:    []float64{1, 2, 3},
		},
		{
			name: "negative zeros throughout",
			a:    [][]float64{{negZero, -2, negZero}, {0, negZero, -3}, {-4, negZero, 5}},
			b:    []float64{negZero, 0, negZero},
		},
		{
			// A nonzero numerator whose quotient UNDERFLOWS to zero: the
			// multiplier is exactly zero without the entry being zero, which is
			// a different route into the shortcut than an exact zero and is
			// tested separately for that reason.
			name: "multiplier underflows to zero",
			a:    [][]float64{{1e300, 1, -2}, {tiny, 1, 3}, {0, 4, 5}},
			b:    []float64{1, 2, 3},
		},
		{
			name: "subnormal entries and extreme scales",
			a: [][]float64{
				{1e-300, 1e300, tiny},
				{tiny, 1e-300, 1e300},
				{1e300, tiny, 1e-300},
			},
			b: []float64{1e-320, 1e300, tiny},
		},
		{
			// Finite input, infinite intermediate: eliminating row 1 against
			// row 0 overflows the augmented column to +Inf, so the pivot row at
			// column 1 is non-finite while row 2's multiplier there is exactly
			// zero. The reference then computes 0 × Inf = NaN into row 2's
			// augmented entry and back-substitutes it into x[2]; a skip that
			// ignored the pivot row's finiteness would report 0.2 instead.
			name: "finite input overflowing into a zero-multiplier row",
			a:    [][]float64{{1, 1, 0}, {-1, 1, 0}, {0, 0, 5}},
			b:    []float64{1e308, 1e308, 1},
		},
		{
			name: "infinity in the matrix",
			a:    [][]float64{{math.Inf(1), 1, 0}, {0, 2, 0}, {0, 0, 3}},
			b:    []float64{1, 2, 3},
		},
		{
			name: "negative infinity in b",
			a:    [][]float64{{2, 1, 0}, {0, 3, 0}, {0, 0, 4}},
			b:    []float64{math.Inf(-1), 1, 1},
		},
		{
			name: "NaN in the matrix",
			a:    [][]float64{{2, math.NaN(), 0}, {0, 3, 0}, {0, 0, 4}},
			b:    []float64{1, 2, 3},
		},
		{
			name: "NaN in b",
			a:    [][]float64{{2, 1, 0}, {0, 3, 0}, {0, 0, 4}},
			b:    []float64{1, math.NaN(), 3},
		},
		{
			name: "NaN below a finite pivot",
			a:    [][]float64{{5, 1, 1}, {math.NaN(), 2, 1}, {0, 1, 3}},
			b:    []float64{1, 1, 1},
		},
	}
}

// randomEliminationSystems returns deterministic pseudo-random systems at a
// spread of sizes and densities. Density is what the shortcut's benefit scales
// with: a sparse system is mostly zero multipliers, and density 1 is the case
// where the shortcut never fires and only its guards are paid for.
func randomEliminationSystems(count int) []eliminationSystem {
	rng := rand.New(rand.NewPCG(0x454c, 0x494d))
	out := make([]eliminationSystem, 0, count)
	for k := 0; k < count; k++ {
		n := 1 + rng.IntN(9)
		density := []float64{0.1, 0.3, 0.6, 1}[rng.IntN(4)]
		a := make([][]float64, n)
		for i := range a {
			a[i] = make([]float64, n)
			for c := range a[i] {
				if rng.Float64() >= density {
					continue
				}
				a[i][c] = (2*rng.Float64() - 1) * math.Ldexp(1, rng.IntN(41)-20)
			}
		}
		b := make([]float64, n)
		for i := range b {
			b[i] = (2*rng.Float64() - 1) * math.Ldexp(1, rng.IntN(41)-20)
		}
		out = append(out, eliminationSystem{
			name: fmt.Sprintf("random case %d (n=%d density=%g)", k, n, density),
			a:    a, b: b,
		})
	}
	return out
}

func TestSolveLinearEliminationMatchesReference(t *testing.T) {
	t.Run("fixed shapes and values", func(t *testing.T) {
		for _, sys := range eliminationFixtures() {
			t.Run(sys.name, func(t *testing.T) {
				requireEliminationMatchesReference(t, sys)
			})
		}
	})

	t.Run("deterministic pseudo-random systems", func(t *testing.T) {
		for _, sys := range randomEliminationSystems(200) {
			requireEliminationMatchesReference(t, sys)
		}
	})

	t.Run("real damped normal equations", func(t *testing.T) {
		// Hand-written fixtures choose their own sparsity. These do not: they
		// are the damped normal-equation systems Sketch.lm actually hands the
		// kernel, at the configuration a real solve leaves the geometry in.
		for _, sys := range solvedSketchNormalEquations(t) {
			t.Run(sys.name, func(t *testing.T) {
				zero, total := countZeroMultipliers(sys.a, sys.b)
				require.Positive(t, zero,
					"%s: a system with no zero multiplier exercises none of the shortcut", sys.name)
				t.Logf("%s: n=%d, %d of %d multipliers zero", sys.name, len(sys.b), zero, total)
				requireEliminationMatchesReference(t, sys)
			})
		}
	})
}

func TestSolveLinearEliminationRefusalLeavesXUnmodified(t *testing.T) {
	// The documented contract on the refusal path, asserted rather than
	// inferred from where the return statement sits.
	sys := eliminationSystem{
		a: [][]float64{{1, 2, 3}, {2, 4, 6}, {1, 1, 1}},
		b: []float64{1, 2, 3},
	}
	n := len(sys.b)
	x := sentinelVector(n)
	require.False(t, solveLinearInto(newEliminationScratch(n), sys.a, sys.b, x), "system is singular")
	for i := range x {
		requireSameBitsf(t, eliminationSentinel, x[i], "x[%d] after a refusal", i)
	}
}

func TestSolveLinearEliminationReusesScratch(t *testing.T) {
	// Pivoting reorders the scratch matrix's ROWS, and lm reuses one scratch
	// across every damping trial of every iteration. The reference gets a fresh
	// scratch per call and the candidate reuses one, so any state carried in
	// the reordered rows — the negative-zero verdict above all, which is
	// computed once per call — shows up as a divergence.
	systems := []eliminationSystem{
		{
			name: "swaps at both columns",
			a:    [][]float64{{0, 1, 2}, {0, 0, 3}, {4, 5, 6}},
			b:    []float64{1, 2, 3},
		},
		{
			name: "singular, refused mid-elimination",
			a:    [][]float64{{1, 2, 3}, {2, 4, 6}, {1, 1, 1}},
			b:    []float64{1, 2, 3},
		},
		{
			name: "negative zero present",
			a:    [][]float64{{1, 2, 0}, {0, 3, 1}, {0, 0, 4}},
			b:    []float64{-1, math.Copysign(0, -1), 2},
		},
		{
			name: "no negative zero, dense",
			a:    [][]float64{{3, -1, 2}, {0.5, 4, -6}, {-2, 8, 1.25}},
			b:    []float64{1, -2, 3},
		},
		{
			name: "diagonal again",
			a:    [][]float64{{2, 0, 0}, {0, -4, 0}, {0, 0, 0.5}},
			b:    []float64{1, 2, 3},
		},
	}

	const n = 3
	shared := newEliminationScratch(n)
	for _, sys := range systems {
		refM, refX := newEliminationScratch(n), sentinelVector(n)
		wantOK := referenceSolveLinearInto(refM, sys.a, sys.b, refX)

		gotX := sentinelVector(n)
		gotOK := solveLinearInto(shared, sys.a, sys.b, gotX)

		require.Equal(t, wantOK, gotOK, "%s: success result", sys.name)
		for i := 0; i < n; i++ {
			requireSameBitsf(t, refX[i], gotX[i], "%s: x[%d] on a reused scratch", sys.name, i)
		}
		requireSameMatrixBits(t, sys.name, refM, shared, n)
	}
}

func TestSolveLinearEliminationNoAllocation(t *testing.T) {
	// The guards must not have added a buffer: the negative-zero verdict is one
	// bool and the finiteness verdict is one bool per pivot, both on the stack.
	const n = 48
	a, b := dampedNormalEquations(96, n, 5, 0x4e4f)
	m := newEliminationScratch(n)
	x := make([]float64, n)
	require.True(t, solveLinearInto(m, a, b, x), "warm-up solve")

	allocs := testing.AllocsPerRun(50, func() {
		solveLinearInto(m, a, b, x)
	})
	require.Zero(t, allocs, "the elimination kernel allocated on a caller-supplied scratch")
}

// countZeroMultipliers replays the elimination and reports how many of its row
// multipliers are exactly zero, out of how many it forms. It is the measurement
// the shortcut is sized by, kept here as a test helper rather than as
// instrumentation inside the kernel.
func countZeroMultipliers(a [][]float64, b []float64) (int, int) {
	n := len(b)
	m := newEliminationScratch(n)
	for i := 0; i < n; i++ {
		copy(m[i], a[i])
		m[i][n] = b[i]
	}
	var zero, total int
	for col := 0; col < n; col++ {
		piv := col
		best := math.Abs(m[col][col])
		for r := col + 1; r < n; r++ {
			if v := math.Abs(m[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < 1e-15 {
			return zero, total
		}
		m[col], m[piv] = m[piv], m[col]
		for r := col + 1; r < n; r++ {
			f := m[r][col] / m[col][col]
			total++
			if f == 0 {
				zero++
			}
			for c := col; c <= n; c++ {
				m[r][c] -= f * m[col][c]
			}
		}
	}
	return zero, total
}

// benchRows and benchBand shape the banded fixture so its zero-multiplier
// fraction lands near the 44.8% measured in the bevel proof (see
// TestSolveLinearEliminationBenchmarkSystemsAreRepresentative). A third of the
// columns per residual row is what reproduces it; a narrower band would report
// a speedup the engine never sees.
func benchRows(n int) int { return n + n/4 }
func benchBand(n int) int { return max(2, n/3) }

// dampedNormalEquations builds the system Sketch.lm hands the kernel: A = JᵀJ
// damped on the diagonal, b = -Jᵀr, over a Jacobian whose every row touches one
// contiguous band of band columns. That banded pattern is what a chain of
// sketch geometry produces — a constraint's residual row reaches only the few
// variables of the geometry it references — and it is what makes most of the
// elimination's multipliers zero.
func dampedNormalEquations(m, n, band int, seed uint64) ([][]float64, []float64) {
	rng := rand.New(rand.NewPCG(0x44414d50, seed))
	j := make([][]float64, m)
	for row := 0; row < m; row++ {
		j[row] = make([]float64, n)
		// The first rows tile the columns so every one of them is touched by
		// some residual row; the rest start anywhere. A column no row touches
		// would make its gradient component an exact +0 and so its right-hand
		// side a NEGATIVE zero, which costs the whole call its shortcut — a real
		// case (see negZeroFixture) but an accident here rather than a property
		// of the structure.
		start := rng.IntN(n)
		if row*band < n {
			start = row * band
		}
		for c := start; c < start+band && c < n; c++ {
			j[row][c] = 2*rng.Float64() - 1
		}
	}
	r := make([]float64, m)
	for k := range r {
		r[k] = 2*rng.Float64() - 1
	}

	ws := &lmWorkspace{}
	ws.alloc(m, n)
	for row := 0; row < m; row++ {
		copy(ws.J[row], j[row])
	}
	transposeInto(ws.JT, ws.J, m, n)
	normalEquationsInto(ws.A, ws.g, ws.JT, r, m, n)

	maxDiag := 0.0
	for i := 0; i < n; i++ {
		if ws.A[i][i] > maxDiag {
			maxDiag = ws.A[i][i]
		}
	}
	if maxDiag == 0 {
		maxDiag = 1
	}
	mu := 1e-3 * maxDiag
	a := make([][]float64, n)
	b := make([]float64, n)
	for i := 0; i < n; i++ {
		a[i] = append([]float64(nil), ws.A[i]...)
		a[i][i] += mu + 1e-12
		b[i] = -ws.g[i]
	}
	return a, b
}

// denseSystem is the opposite workload: no zero multiplier anywhere, so the
// shortcut can only cost. A specialization that hid a regression here would be
// a bad trade even at its own best case.
func denseSystem(n int, seed uint64) ([][]float64, []float64) {
	rng := rand.New(rand.NewPCG(0x44454e53, seed))
	a := make([][]float64, n)
	for i := range a {
		a[i] = make([]float64, n)
		for c := range a[i] {
			a[i][c] = 2*rng.Float64() - 1
		}
		a[i][i] += float64(n) // keep it comfortably nonsingular
	}
	b := make([]float64, n)
	for i := range b {
		b[i] = 2*rng.Float64() - 1
	}
	return a, b
}

// negZeroFixture returns a copy of one system with the augmented column's first
// entry forced to negative zero. That is the shape the profiled workload
// produces whenever a free variable's gradient component is exactly zero, and
// it costs the call its shortcut, so the benchmark times it as its own case
// rather than letting it hide inside the sparse one.
func negZeroFixture(a [][]float64, b []float64) ([][]float64, []float64) {
	out := cloneRows(a)
	ob := append([]float64(nil), b...)
	ob[0] = math.Copysign(0, -1)
	return out, ob
}

func countNegZeros(a [][]float64, b []float64) int {
	count := 0
	for i := range a {
		for _, v := range a[i] {
			if math.Float64bits(v) == negZeroBits {
				count++
			}
		}
		if math.Float64bits(b[i]) == negZeroBits {
			count++
		}
	}
	return count
}

func TestSolveLinearEliminationBenchmarkSystemsAreRepresentative(t *testing.T) {
	// The benchmark below is only evidence if each of its fixtures really is
	// the workload it claims to be. All three are also run through the
	// differential comparison, so the benchmark's inputs are covered by the
	// correctness claim too.
	//
	// The window is around the structure measured in the gear generator's bevel
	// proof, whose every call to this kernel was 128×128 with 44.8% of its
	// multipliers exactly zero and 10.3% of its calls carrying a negative zero.
	// A fixture far outside that window would time a workload the engine does
	// not actually meet.
	a, b := dampedNormalEquations(benchRows(128), 128, benchBand(128), 0x4245)
	zero, total := countZeroMultipliers(a, b)
	t.Logf("bevel-sized damped normal equations: %d of %d multipliers zero (%.1f%%)",
		zero, total, 100*float64(zero)/float64(total))
	require.InDelta(t, 0.448, float64(zero)/float64(total), 0.1,
		"the sparse benchmark fixture must resemble the measured bevel structure")
	require.Zero(t, countNegZeros(a, b),
		"a stray negative zero would silently disable the shortcut this fixture times")
	requireEliminationMatchesReference(t, eliminationSystem{name: "bevel-sized damped normal equations", a: a, b: b})

	na, nb := negZeroFixture(a, b)
	require.Equal(t, 1, countNegZeros(na, nb), "the negative-zero fixture must carry exactly one")
	requireEliminationMatchesReference(t, eliminationSystem{name: "bevel-sized with a negative zero", a: na, b: nb})

	da, db := denseSystem(128, 0x4445)
	dzero, dtotal := countZeroMultipliers(da, db)
	t.Logf("dense 128: %d of %d multipliers zero", dzero, dtotal)
	require.Zero(t, dzero, "the dense benchmark fixture must exercise none of the shortcut")
	requireEliminationMatchesReference(t, eliminationSystem{name: "dense 128", a: da, b: db})
}

// solvedSketchNormalEquations builds the damped normal equations of real solved
// sketches, the same fixtures the sparse accumulation kernel is checked on.
func solvedSketchNormalEquations(t *testing.T) []eliminationSystem {
	t.Helper()
	out := make([]eliminationSystem, 0, 3)
	for _, tc := range sparseJacobianSketches(t) {
		free := tc.s.freeVars()
		n := len(free)
		r := tc.s.residuals(nil)
		m := len(r)
		require.Positive(t, m, "%s: no residual rows", tc.name)
		require.Positive(t, n, "%s: no free variables", tc.name)

		ws := &lmWorkspace{}
		ws.alloc(m, n)
		tc.s.jacobianInto(ws.J, free, m, tc.s.residuals, ws.rp, ws.rm)
		transposeInto(ws.JT, ws.J, m, n)
		ws.r = r
		normalEquationsSparseInto(ws, m, n)

		maxDiag := 0.0
		for i := 0; i < n; i++ {
			if ws.A[i][i] > maxDiag {
				maxDiag = ws.A[i][i]
			}
		}
		if maxDiag == 0 {
			maxDiag = 1
		}
		mu := 1e-3 * maxDiag
		a := make([][]float64, n)
		b := make([]float64, n)
		for i := 0; i < n; i++ {
			a[i] = append([]float64(nil), ws.A[i]...)
			a[i][i] += mu + 1e-12
			b[i] = -ws.g[i]
		}
		out = append(out, eliminationSystem{name: tc.name, a: a, b: b})
	}
	return out
}

// eliminationSink keeps the solved vector observable so the compiler cannot
// delete the elimination the benchmark is meant to time.
var eliminationSink float64

func BenchmarkSolveLinearElimination(b *testing.B) {
	type benchCase struct {
		name string
		a    [][]float64
		b    []float64
	}
	cases := []benchCase{}
	for _, n := range []int{32, 128, 256} {
		sa, sb := dampedNormalEquations(benchRows(n), n, benchBand(n), uint64(n))
		cases = append(cases, benchCase{fmt.Sprintf("banded-normal-equations/%d", n), sa, sb})
		na, nb := negZeroFixture(sa, sb)
		cases = append(cases, benchCase{fmt.Sprintf("banded-negative-zero/%d", n), na, nb})
	}
	for _, n := range []int{32, 128, 256} {
		da, db := denseSystem(n, uint64(n))
		cases = append(cases, benchCase{fmt.Sprintf("dense/%d", n), da, db})
	}

	for _, tc := range cases {
		n := len(tc.b)
		// Both variants run on scratch allocated OUTSIDE the timed region, the
		// way lm allocates its workspace once per solve and reuses it.
		b.Run("reference/"+tc.name, func(b *testing.B) {
			m, x := newEliminationScratch(n), make([]float64, n)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				referenceSolveLinearInto(m, tc.a, tc.b, x)
				eliminationSink += x[n-1]
			}
		})
		b.Run("guarded/"+tc.name, func(b *testing.B) {
			m, x := newEliminationScratch(n), make([]float64, n)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				solveLinearInto(m, tc.a, tc.b, x)
				eliminationSink += x[n-1]
			}
		})
	}
}
