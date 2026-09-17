package sketchtest_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

func TestRectangleFlow(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)

	rect := s.CreateRectangle(0, 0, 20, 12)
	s.AddConstraint(
		sketch.NewCoincident(rect.A, s.Origin()),
		sketch.NewDistance(rect.A, rect.B, 20),
		sketch.NewDistance(rect.A, rect.D, 12),
	)

	result := sketchtest.Solve(t, s)
	require.True(t, result.Converged)
	report := sketchtest.IsTrustworthy(t, s)
	sketchtest.MeasuresPoint(t, rect.C, 20, 12, sketchtest.Within(1e-8))

	profile := sketchtest.SingleProfile(t, report)
	sketchtest.IsValidProfile(t, profile)
	sketchtest.IsCurrentProfile(t, profile)
	sketchtest.HasExactCuts(t, profile)
}
