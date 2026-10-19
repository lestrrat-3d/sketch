package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

func TestVerifyWithoutConditioningKeepsStatusAndFailsCheck(t *testing.T) {
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))
	sketchtest.Solve(t, s)

	full := s.Verify(t.Context())
	rep := s.Verify(t.Context(), sketch.WithoutConditioning())

	require.Equal(t, full.Status, rep.Status)
	require.Equal(t, full.DOF, rep.DOF)
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.True(t, math.IsNaN(rep.Conditioning))
	require.ErrorIs(t, rep.Check(), sketch.ErrConditioningUnmeasured)
	require.NoError(t, full.Check())
}

func TestVerifyFullRankStillReportsRedundancy(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.CreateLine(a, b)
	s.AddConstraint(sketch.NewDistance(a, b, 10), sketch.NewHorizontalPoints(a, b),
		sketch.NewHorizontalPoints(a, b))
	sketchtest.Solve(t, s)

	rep := s.Verify(t.Context(), sketch.WithoutConditioning())

	require.Equal(t, sketch.Overconstrained, rep.Status)
	require.NotEmpty(t, rep.Redundant)
	require.Empty(t, rep.Conflicts)
}
