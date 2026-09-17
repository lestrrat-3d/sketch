package sketchtest

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
)

// Solve runs s.Solve with the test's context and fails tb on any error.
// s MUST NOT be nil.
func Solve(tb testing.TB, s *sketch.Sketch, opts ...sketch.SolveOption) *sketch.Result {
	tb.Helper()

	if s == nil {
		tb.Fatalf("sketchtest.Solve: s must not be nil")
		return nil
	}

	result, err := s.Solve(tb.Context(), opts...)
	if err != nil {
		tb.Fatalf("solve: Sketch.Solve failed: %v", err)
		return nil
	}
	return result
}
