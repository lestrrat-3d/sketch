package sketchtest_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

func TestMeasuresAcceptsAbsoluteAndRelativeTolerance(t *testing.T) {
	sketchtest.Measures(t, "length", 10.01, 10, sketchtest.Within(0.02))
	sketchtest.Measures(t, "length", 10.01, 10, sketchtest.WithinRel(0.002))
}

func TestNumericDomainHelpers(t *testing.T) {
	s, rect, width := constrainedRectangle(t)
	sketchtest.Solve(t, s)
	report := sketchtest.Verify(t, s)
	profile := sketchtest.SingleProfile(t, report)

	sketchtest.MeasuresPoint(t, rect.C, 20, 12, sketchtest.Within(1e-8))
	sketchtest.MeasuresWorldPoint(t, rect.C, r3.Vec{X: 20, Y: 12}, sketchtest.Within(1e-8))
	sketchtest.MeasuresProfileArea(t, profile, 240, sketchtest.Within(1e-8))
	sketchtest.Satisfies(t, width, sketchtest.Within(1e-8))
}

func TestMeasuresFailureMessages(t *testing.T) {
	tests := []struct {
		name string
		run  func(testing.TB)
		want string
	}{
		{
			name: "missing option",
			run:  func(tb testing.TB) { sketchtest.Measures(tb, "length", 1, 1) },
			want: "pass Within or WithinRel",
		},
		{
			name: "miss",
			run:  func(tb testing.TB) { sketchtest.Measures(tb, "length", 2, 1, sketchtest.Within(0.5)) },
			want: "difference 1 exceeds absolute tolerance 0.5",
		},
		{
			name: "non-finite",
			run:  func(tb testing.TB) { sketchtest.Measures(tb, "length", math.NaN(), 1, sketchtest.Within(1)) },
			want: "got and want must be finite",
		},
		{
			name: "negative tolerance",
			run:  func(tb testing.TB) { sketchtest.Measures(tb, "length", 1, 1, sketchtest.Within(-1)) },
			want: "tolerance must be finite and non-negative",
		},
		{
			name: "nil option",
			run: func(tb testing.TB) {
				var option sketchtest.Option
				sketchtest.Measures(tb, "length", 1, 1, option)
			},
			want: "a nil Option was passed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := captureFailure(t, test.run)
			require.Contains(t, message, test.want)
		})
	}
}

func TestLaterToleranceOptionWins(t *testing.T) {
	sketchtest.Measures(t, "length", 2, 1, sketchtest.Within(0.5), sketchtest.Within(1))
}

func TestMeasuresPointNamesThePointAndCoordinate(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(1, 2)
	p.SetName("tip")

	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.MeasuresPoint(tb, p, 0, 2, sketchtest.Within(0.1))
	})
	require.Contains(t, message, `point[0] "tip" x`)
}

func TestSatisfiesReportsResidualMiss(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	constraint := sketch.NewDistance(a, b, 20)
	s.AddConstraint(constraint)

	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.Satisfies(tb, constraint, sketchtest.Within(1e-8))
	})
	require.Contains(t, message, `constraint "distance" residual[0]`)
}
