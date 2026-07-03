package param_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/stretchr/testify/require"
)

// nest wraps "1" in n balanced parentheses: nest(3) == "(((1)))".
func nest(n int) string {
	return strings.Repeat("(", n) + "1" + strings.Repeat(")", n)
}

func TestParseNestingDepth(t *testing.T) {
	t.Run("moderately nested expression still parses", func(t *testing.T) {
		e, err := param.Parse(nest(500))
		require.NoError(t, err)
		require.NotNil(t, e)
	})

	t.Run("pathological nesting is a ParseError, not a crash", func(t *testing.T) {
		// Deep enough to overflow the goroutine stack before the guard existed;
		// it must now surface as an ordinary error instead of crashing.
		_, err := param.Parse(nest(200000))
		require.Error(t, err)
		var pe *param.ParseError
		require.True(t, errors.As(err, &pe))
		require.Contains(t, err.Error(), "too deep")
	})

	t.Run("deeply nested function arguments are bounded too", func(t *testing.T) {
		// Function-argument lists recurse through parseExpr as well, so the
		// same guard must catch "sqrt(sqrt(sqrt(…)))" nesting.
		expr := strings.Repeat("sqrt(", 200000) + "1" + strings.Repeat(")", 200000)
		_, err := param.Parse(expr)
		require.Error(t, err)
		require.Contains(t, err.Error(), "too deep")
	})

	t.Run("deep unary-operator chain is bounded", func(t *testing.T) {
		// A long "-----…1" recurses through parseUnary directly, never through a
		// parenthesised parseExpr, so a guard only on parseExpr would miss it.
		_, err := param.Parse(strings.Repeat("-", 200000) + "1")
		require.Error(t, err)
		require.Contains(t, err.Error(), "too deep")
	})

	t.Run("deep exponent chain is bounded", func(t *testing.T) {
		// "1^1^1^…" recurses right-associatively through parsePower → parseUnary,
		// again bypassing any parseExpr-only guard.
		_, err := param.Parse("1" + strings.Repeat("^1", 200000))
		require.Error(t, err)
		require.Contains(t, err.Error(), "too deep")
	})

	t.Run("wide flat expression is not falsely rejected", func(t *testing.T) {
		// Width is not depth: a long sum nests only one level deep, so the guard
		// (which counts recursion, not operand count) must accept it.
		e, err := param.Parse("1" + strings.Repeat("+1", 5000))
		require.NoError(t, err)
		require.NotNil(t, e)
	})
}
