package sketch

// In-package for the same reason conditioning_internal_test.go is: the banded
// path is an unexported optimization of [singularValueExtremes], and the property
// worth pinning is that it agrees with the dense kernel it stands in for.

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// ringMatrix builds a tall sparse matrix shaped like a closed chain of points:
// n columns numbered around a ring, each row coupling a column to the next few
// around the ring (the last ones wrap to the start), plus unary rows.
func ringMatrix(rng *rand.Rand, n, reach, extraRows int, shuffle bool) [][]float64 {
	colOf := make([]int, n)
	for i := range colOf {
		colOf[i] = i
	}
	if shuffle {
		rng.Shuffle(n, func(i, j int) { colOf[i], colOf[j] = colOf[j], colOf[i] })
	}
	var A [][]float64
	add := func(entries map[int]float64) {
		row := make([]float64, n)
		for c, v := range entries {
			row[colOf[c]] = v
		}
		A = append(A, row)
	}
	for i := 0; i < n; i++ {
		e := map[int]float64{i: 2 + rng.Float64()}
		for k := 1; k <= reach; k++ {
			e[(i+k)%n] = rng.NormFloat64() * 0.5
		}
		add(e)
	}
	for r := 0; r < extraRows; r++ {
		i := rng.Intn(n)
		add(map[int]float64{i: rng.NormFloat64(), (i + 1) % n: rng.NormFloat64()})
	}
	return A
}

func denseExtremes(A [][]float64) (float64, float64) {
	m := len(A)
	W := make([][]float64, m)
	for i := range W {
		W[i] = append([]float64(nil), A[i]...)
	}
	d, e := bidiagonalize(W)
	return bidiagonalExtremes(d, e)
}

func TestBandedExtremesMatchDenseOnRings(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, tc := range []struct {
		name         string
		n, reach, ex int
		shuffle      bool
	}{
		{"unit reach", 300, 1, 0, false},
		{"reach three", 400, 3, 0, false},
		{"shuffled columns", 350, 2, 0, true},
		{"redundant rows", 320, 2, 40, false},
		{"wide reach", 600, 6, 10, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			A := ringMatrix(rng, tc.n, tc.reach, tc.ex, tc.shuffle)
			smax, smin, ok := bandedSingularValueExtremes(A)
			require.True(t, ok, "matrix must qualify for the banded path")
			dmax, dmin := denseExtremes(A)
			require.InDelta(t, dmax, smax, 1e-11*dmax)
			require.InDelta(t, dmin, smin, 1e-11*dmax)
		})
	}
}

func TestBandedExtremesAreDeterministic(t *testing.T) {
	A := ringMatrix(rand.New(rand.NewSource(3)), 300, 2, 5, true)
	a1, b1, ok1 := bandedSingularValueExtremes(A)
	a2, b2, ok2 := bandedSingularValueExtremes(A)
	require.True(t, ok1 && ok2)
	require.Equal(t, math.Float64bits(a1), math.Float64bits(a2))
	require.Equal(t, math.Float64bits(b1), math.Float64bits(b2))
}

func TestBandHypotMatchesHypot(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y float64
	}{
		{name: "ordinary", x: 3, y: 4},
		{name: "large fast path", x: 1e150, y: 1e149},
		{name: "small fast path", x: 1e-150, y: 2e-150},
		{name: "underflow fallback", x: 1e-200, y: 1e-200},
		{name: "overflow fallback", x: 1e200, y: 1e200},
		{name: "infinity and NaN", x: math.Inf(1), y: math.NaN()},
		{name: "zero", x: 0, y: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, want := bandHypot(tc.x, tc.y), math.Hypot(tc.x, tc.y)
			switch {
			case math.IsNaN(want):
				require.True(t, math.IsNaN(got))
			case math.IsInf(want, 0):
				require.Equal(t, want, got)
			case want == 0:
				require.Equal(t, math.Float64bits(want), math.Float64bits(got))
			default:
				require.LessOrEqual(t, math.Abs(got-want)/want, 2*math.Nextafter(1, 2)-2)
			}
		})
	}
}

func TestBandedExtremesRankDeficientReadsSingular(t *testing.T) {
	A := ringMatrix(rand.New(rand.NewSource(5)), 300, 1, 0, false)
	// Zero a column so the matrix loses rank.
	for _, row := range A {
		row[10] = 0
	}
	smax, smin, ok := bandedSingularValueExtremes(A)
	if !ok {
		// Declining is also correct: the dense kernel then answers.
		return
	}
	require.Greater(t, smax, 0.0)
	require.Zero(t, smin/smax)
}

func TestBandedExtremesDecline(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	t.Run("small", func(t *testing.T) {
		_, _, ok := bandedSingularValueExtremes(ringMatrix(rng, 64, 1, 0, false))
		require.False(t, ok)
	})
	t.Run("non-finite", func(t *testing.T) {
		A := ringMatrix(rng, 300, 1, 0, false)
		A[7][7] = math.NaN()
		_, _, ok := bandedSingularValueExtremes(A)
		require.False(t, ok)
		smax, smin, ok := singularValueExtremes(A)
		require.True(t, ok)
		require.True(t, math.IsNaN(smax) && math.IsNaN(smin))
	})
	t.Run("wide", func(t *testing.T) {
		A := ringMatrix(rng, 300, 1, 0, false)
		// Random chords across the ring leave no narrow numbering to find.
		for i := 0; i < 300; i++ {
			row := make([]float64, 300)
			row[i], row[rng.Intn(300)] = 1, 1
			A = append(A, row)
		}
		_, _, ok := bandedSingularValueExtremes(A)
		require.False(t, ok)
	})
	t.Run("fewer rows than columns", func(t *testing.T) {
		A := ringMatrix(rng, 300, 1, 0, false)[:299]
		_, _, ok := bandedSingularValueExtremes(A)
		require.False(t, ok)
	})
}
