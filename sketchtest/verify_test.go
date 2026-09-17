package sketchtest_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

func TestVerificationAssertions(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.CreateLine(a, b)
	sketchtest.Solve(t, s)

	report := sketchtest.Verify(t, s)
	sketchtest.HasStatus(t, report, sketch.Underconstrained)
	sketchtest.HasDOF(t, report, 4)
	sketchtest.HasFreePoints(t, report, b, a)
	reasons := sketchtest.FindReasons(t, report, sketch.ErrNotFullyConstrained)
	require.Len(t, reasons, 1)
	require.ErrorIs(t, reasons[0], sketch.ErrNotFullyConstrained)
	sketchtest.HasOnlyReasons(t, report, sketch.ErrNotFullyConstrained)
}

func TestHasOnlyReasonsAcceptsCleanReport(t *testing.T) {
	s, _, _ := constrainedRectangle(t)
	sketchtest.Solve(t, s)
	report := sketchtest.Verify(t, s)
	sketchtest.HasOnlyReasons(t, report)
}

func TestFindConflictUsesConstraintIdentity(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	first := sketch.NewDistance(a, b, 10)
	conflict := sketch.NewDistance(a, b, 20)
	s.AddConstraint(first, conflict)
	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)

	report := sketchtest.Verify(t, s)
	set := sketchtest.FindConflict(t, report, conflict)
	require.Same(t, conflict, set.Constraint)
	require.Contains(t, set.With, first)
}

func TestVerificationAssertionFailures(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(1, 2)
	sketchtest.Solve(t, s)
	report := sketchtest.Verify(t, s)

	statusMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.HasStatus(tb, report, sketch.FullyConstrained)
	})
	require.Contains(t, statusMessage, "status is underconstrained, want fully constrained")

	dofMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.HasDOF(tb, report, 0)
	})
	require.Contains(t, dofMessage, "DOF is 2, want 0")

	pointsMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.HasFreePoints(tb, report)
	})
	require.Contains(t, pointsMessage, "free points are [point[0]], want []")

	reasonMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.FindReasons(tb, report, sketch.ErrAmbiguous)
	})
	require.Contains(t, reasonMessage, "contains no reason matching")

	onlyMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.HasOnlyReasons(tb, report, sketch.ErrAmbiguous)
	})
	require.Contains(t, onlyMessage, sketch.ErrNotFullyConstrained.Error())

	trustMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.IsTrustworthy(tb, s)
	})
	require.Contains(t, trustMessage, "report is not trustworthy")
	require.NotNil(t, p)
}

func TestAssertionsRejectSkippedAnalysis(t *testing.T) {
	s := newSketch(t)
	s.CreatePoint(math.NaN(), 0)
	report := sketchtest.Verify(t, s)
	require.False(t, report.Analysed())

	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.HasDOF(tb, report, 0)
	})
	require.Contains(t, message, "report was not analysed")
	require.Contains(t, message, sketch.ErrVerificationIncomplete.Error())
}

func TestFindConflictReportsMissingIdentity(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	want := sketch.NewDistance(a, b, 10)
	s.AddConstraint(want)
	sketchtest.Solve(t, s)
	report := sketchtest.Verify(t, s)
	other := sketch.NewDistance(a, b, 10)

	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.FindConflict(tb, report, other)
	})
	require.Contains(t, message, "no conflict for constraint \"distance\"")
}

func TestVerifyRejectsNilSketch(t *testing.T) {
	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.Verify(tb, nil)
	})
	require.Equal(t, "sketchtest.Verify: s must not be nil", message)
}

func TestFindReasonsPreservesWrappedReason(t *testing.T) {
	s := newSketch(t)
	s.CreatePoint(0, 0)
	sketchtest.Solve(t, s)
	report := sketchtest.Verify(t, s)
	reason := sketchtest.FindReasons(t, report, sketch.ErrNotFullyConstrained)[0]
	require.True(t, errors.Is(reason, sketch.ErrNotFullyConstrained))
}
