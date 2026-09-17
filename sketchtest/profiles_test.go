package sketchtest_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

func TestSingleProfileReportsWrongCount(t *testing.T) {
	s := newSketch(t)
	s.CreateCircle(s.CreatePoint(0, 0), 2)
	s.CreateCircle(s.CreatePoint(10, 0), 2)
	report := sketchtest.Verify(t, s)

	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.SingleProfile(tb, report)
	})
	require.Contains(t, message, "contains 2 profile(s), want exactly 1")
}

func TestIsCurrentProfileReportsStaleProfile(t *testing.T) {
	s, rect, _ := constrainedRectangle(t)
	sketchtest.Solve(t, s)
	profile := sketchtest.SingleProfile(t, sketchtest.Verify(t, s))
	rect.C.MoveTo(25, 12)

	message := captureFailure(t, func(tb testing.TB) {
		sketchtest.IsCurrentProfile(tb, profile)
	})
	require.Equal(t, "profile: profile is stale", message)
}

func TestProfileAssertionsReportInvalidAndApproximateCuts(t *testing.T) {
	invalidSketch := newSketch(t)
	a := invalidSketch.CreatePoint(0, 0)
	b := invalidSketch.CreatePoint(4, 4)
	c := invalidSketch.CreatePoint(4, 0)
	d := invalidSketch.CreatePoint(0, 4)
	invalidSketch.CreateLine(a, b)
	invalidSketch.CreateLine(b, c)
	invalidSketch.CreateLine(c, d)
	invalidSketch.CreateLine(d, a)
	invalidReport := sketchtest.Verify(t, invalidSketch)
	require.NotEmpty(t, invalidReport.InvalidProfiles)
	invalid := invalidReport.InvalidProfiles[0]
	invalidMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.IsValidProfile(tb, invalid)
	})
	require.Equal(t, "profile: validity is false, want true", invalidMessage)

	approximateSketch := newSketch(t)
	approximateSketch.CreateRectangle(-5, -3, 5, 3)
	approximateSketch.CreateEllipse(approximateSketch.CreatePoint(0, 0), 6, 2, 0)
	var approximate *sketch.Profile
	for _, profile := range approximateSketch.Profiles() {
		for _, edge := range profile.Outer {
			if edge.Partial && !edge.TExact {
				approximate = profile
				break
			}
		}
		if approximate != nil {
			break
		}
	}
	require.NotNil(t, approximate, "the real arrangement must produce an approximate partial edge")
	cutMessage := captureFailure(t, func(tb testing.TB) {
		sketchtest.HasExactCuts(tb, approximate)
	})
	require.Contains(t, cutMessage, "partial boundary has approximate parameters")
}

func TestHasExactCutsIgnoresWholeApproximateEdge(t *testing.T) {
	s := newSketch(t)
	s.CreateEllipse(s.CreatePoint(0, 0), 6, 2, 0)
	profiles := s.Profiles()
	require.Len(t, profiles, 1)
	profile := profiles[0]
	require.Len(t, profile.Outer, 1)
	require.False(t, profile.Outer[0].Partial)
	require.False(t, profile.Outer[0].TExact)
	sketchtest.HasExactCuts(t, profile)
}

func TestHasExactCutsAcceptsRealAnalyticPartialEdges(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(0, 0, 6, 4)
	s.CreateRectangle(3, 2, 9, 6)
	profiles := s.Profiles()
	require.Len(t, profiles, 3)

	sawPartial := false
	for _, profile := range profiles {
		for _, edge := range profile.Outer {
			sawPartial = sawPartial || edge.Partial
		}
		sketchtest.HasExactCuts(t, profile)
	}
	require.True(t, sawPartial)
}
