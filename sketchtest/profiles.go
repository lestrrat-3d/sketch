package sketchtest

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
)

// SingleProfile fails tb unless an analysed report contains exactly one
// profile, then returns it. It does not assert the profile's validity.
func SingleProfile(tb testing.TB, report *sketch.VerificationReport) *sketch.Profile {
	tb.Helper()

	if !requireAnalysed(tb, "SingleProfile", report) {
		return nil
	}
	if len(report.Profiles) != 1 {
		tb.Fatalf("verify: report contains %d profile(s), want exactly 1", len(report.Profiles))
		return nil
	}
	return report.Profiles[0]
}

// IsValidProfile fails tb unless profile carries sketch's valid-region
// verdict. profile MUST NOT be nil.
func IsValidProfile(tb testing.TB, profile *sketch.Profile) {
	tb.Helper()

	if profile == nil {
		tb.Fatalf("sketchtest.IsValidProfile: profile must not be nil")
		return
	}
	if !profile.Valid {
		tb.Fatalf("profile: validity is false, want true")
	}
}

// IsCurrentProfile fails tb when profile predates a change to its sketch.
// profile MUST NOT be nil.
func IsCurrentProfile(tb testing.TB, profile *sketch.Profile) {
	tb.Helper()

	if profile == nil {
		tb.Fatalf("sketchtest.IsCurrentProfile: profile must not be nil")
		return
	}
	if profile.IsStale() {
		tb.Fatalf("profile: profile is stale")
	}
}

// HasExactCuts fails tb when a partial outer or hole edge carries approximate
// parameters. Whole edges need no trim and are ignored. profile MUST NOT be nil.
func HasExactCuts(tb testing.TB, profile *sketch.Profile) {
	tb.Helper()

	if profile == nil {
		tb.Fatalf("sketchtest.HasExactCuts: profile must not be nil")
		return
	}
	for i, edge := range profile.Outer {
		if edge.Partial && !edge.TExact {
			tb.Fatalf("profile outer edge[%d]: partial boundary has approximate parameters", i)
			return
		}
	}
	for holeIndex, hole := range profile.Holes {
		for edgeIndex, edge := range hole {
			if edge.Partial && !edge.TExact {
				tb.Fatalf("profile hole[%d] edge[%d]: partial boundary has approximate parameters",
					holeIndex, edgeIndex)
				return
			}
		}
	}
}
