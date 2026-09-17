package sketchtest_test

import (
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

func TestHelpersRejectNilInputs(t *testing.T) {
	tests := []struct {
		name string
		run  func(testing.TB)
		want string
	}{
		{
			name: "IsTrustworthy sketch",
			run:  func(tb testing.TB) { sketchtest.IsTrustworthy(tb, nil) },
			want: "sketchtest.Verify: s must not be nil",
		},
		{
			name: "HasStatus report",
			run:  func(tb testing.TB) { sketchtest.HasStatus(tb, nil, sketch.FullyConstrained) },
			want: "sketchtest.HasStatus: report must not be nil",
		},
		{
			name: "HasDOF report",
			run:  func(tb testing.TB) { sketchtest.HasDOF(tb, nil, 0) },
			want: "sketchtest.HasDOF: report must not be nil",
		},
		{
			name: "FindReasons report",
			run:  func(tb testing.TB) { sketchtest.FindReasons(tb, nil, sketch.ErrAmbiguous) },
			want: "sketchtest.FindReasons: report must not be nil",
		},
		{
			name: "HasOnlyReasons report",
			run:  func(tb testing.TB) { sketchtest.HasOnlyReasons(tb, nil) },
			want: "sketchtest.HasOnlyReasons: report must not be nil",
		},
		{
			name: "FindConflict report",
			run:  func(tb testing.TB) { sketchtest.FindConflict(tb, nil, nil) },
			want: "sketchtest.FindConflict: report must not be nil",
		},
		{
			name: "HasFreePoints report",
			run:  func(tb testing.TB) { sketchtest.HasFreePoints(tb, nil) },
			want: "sketchtest.HasFreePoints: report must not be nil",
		},
		{
			name: "SingleProfile report",
			run:  func(tb testing.TB) { sketchtest.SingleProfile(tb, nil) },
			want: "sketchtest.SingleProfile: report must not be nil",
		},
		{
			name: "IsValidProfile profile",
			run:  func(tb testing.TB) { sketchtest.IsValidProfile(tb, nil) },
			want: "sketchtest.IsValidProfile: profile must not be nil",
		},
		{
			name: "IsCurrentProfile profile",
			run:  func(tb testing.TB) { sketchtest.IsCurrentProfile(tb, nil) },
			want: "sketchtest.IsCurrentProfile: profile must not be nil",
		},
		{
			name: "HasExactCuts profile",
			run:  func(tb testing.TB) { sketchtest.HasExactCuts(tb, nil) },
			want: "sketchtest.HasExactCuts: profile must not be nil",
		},
		{
			name: "MeasuresPoint point",
			run:  func(tb testing.TB) { sketchtest.MeasuresPoint(tb, nil, 0, 0, sketchtest.Within(0)) },
			want: "sketchtest.MeasuresPoint: point must not be nil",
		},
		{
			name: "MeasuresWorldPoint point",
			run: func(tb testing.TB) {
				sketchtest.MeasuresWorldPoint(tb, nil, r3.Vec{}, sketchtest.Within(0))
			},
			want: "sketchtest.MeasuresWorldPoint: point must not be nil",
		},
		{
			name: "MeasuresProfileArea profile",
			run:  func(tb testing.TB) { sketchtest.MeasuresProfileArea(tb, nil, 0, sketchtest.Within(0)) },
			want: "sketchtest.MeasuresProfileArea: profile must not be nil",
		},
		{
			name: "Satisfies constraint",
			run:  func(tb testing.TB) { sketchtest.Satisfies(tb, nil, sketchtest.Within(0)) },
			want: "sketchtest.Satisfies: constraint must not be nil",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := captureFailure(t, test.run)
			require.Equal(t, test.want, message)
		})
	}
}

func TestReasonHelpersRejectNilSentinels(t *testing.T) {
	s, _, _ := constrainedRectangle(t)
	sketchtest.Solve(t, s)
	report := sketchtest.Verify(t, s)

	findMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.FindReasons(tb, report, nil)
	})
	require.Equal(t, "sketchtest.FindReasons: sentinel must not be nil", findMessage)

	onlyMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.HasOnlyReasons(tb, report, nil)
	})
	require.Equal(t, "sketchtest.HasOnlyReasons: allowed sentinel must not be nil", onlyMessage)
}
