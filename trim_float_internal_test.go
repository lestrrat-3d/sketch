package sketch

// This is an internal test file (the repo's tests otherwise live in external
// xxx_test packages and exercise only the exported API). It is here on purpose:
// trimFloat itself is unexported, and the property under test — its bytes match
// a fmt.Sprintf-built reference for every float64, not just the fixtures the SVG
// and DXF goldens happen to cover — needs direct access to it rather than an
// exported round-about accessor added only to test it.

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// refTrimFloat is the pre-optimization reference implementation trimFloat must
// stay byte-identical to.
func refTrimFloat(v float64, prec int) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.*f", prec, v), "0"), ".")
}

// fixedTrimFloatValues are the individual values and boundary cases every
// precision is checked against, plus their negatives where the negation is not
// already present in the list.
func fixedTrimFloatValues() []float64 {
	return []float64{
		0,
		math.Copysign(0, -1),
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		math.MaxFloat64,
		-math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		-math.SmallestNonzeroFloat64,
		1e21, -1e21,
		1e-5, -1e-5,
		5e-5, -5e-5,
		0.00015, -0.00015,
		0.99995, -0.99995,
		123456789.123456789, -123456789.123456789,
	}
}

// requireTrimFloatMatches asserts trimFloat(v, prec) equals the fmt-built
// reference, for prec in {4, 6}, with v identified in the failure message
// (NaN/Inf print fine with %v, and this is the only place the mismatch needs
// pinpointing).
func requireTrimFloatMatches(t *testing.T, v float64) {
	t.Helper()
	for _, prec := range []int{4, 6} {
		want := refTrimFloat(v, prec)
		got := trimFloat(v, prec)
		require.Equal(t, want, got, "trimFloat(%v, %d)", v, prec)
	}
}

// TestTrimFloatMatchesFmt pins trimFloat's strconv.AppendFloat-based
// implementation to the byte-for-byte output of the fmt.Sprintf-based
// reference it replaced, across fixed boundary values, a large log-uniform
// random sweep, and a rounding-tie sweep. Every case must produce identical
// bytes, including for NaN, +Inf, -Inf and negative zero, since the SVG/DXF
// exporters' byte-identical output depends on it.
func TestTrimFloatMatchesFmt(t *testing.T) {
	t.Run("fixed", func(t *testing.T) {
		for _, v := range fixedTrimFloatValues() {
			requireTrimFloatMatches(t, v)
		}
	})

	// logUniformSweep draws n values with exponents uniform in [-12, 18] (so
	// magnitudes span 1e-12 .. 1e18, per the plan) and a random sign, from a
	// fixed seed so the sweep is deterministic run to run.
	t.Run("log_uniform_sweep", func(t *testing.T) {
		n := 2000
		if !testing.Short() {
			n = 1_000_000
		}
		rng := rand.New(rand.NewSource(20260903))
		for i := 0; i < n; i++ {
			exp := -12 + rng.Float64()*30
			v := math.Pow(10, exp)
			if rng.Intn(2) == 0 {
				v = -v
			}
			requireTrimFloatMatches(t, v)
		}
	})

	// tieSweep hits the exact-.5-ULP rounding ties '%.*f' formatting is most
	// likely to disagree on: values of the form k*1e-4 + 5e-5, nudged by
	// +-2^-40 (far below the 4/6-decimal precision under test, but enough to
	// land on either side of the representable tie) so the sweep exercises
	// both round-up and round-down paths deterministically.
	t.Run("tie_sweep", func(t *testing.T) {
		n := 2000
		if !testing.Short() {
			n = 100_000
		}
		nudge := math.Ldexp(1, -40)
		for i := 0; i < n; i++ {
			base := float64(i)*1e-4 + 5e-5
			sign := 1.0
			if i%2 == 1 {
				sign = -1
			}
			v := base + sign*nudge
			requireTrimFloatMatches(t, v)
			requireTrimFloatMatches(t, -v)
		}
	})
}
