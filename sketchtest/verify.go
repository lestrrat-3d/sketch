package sketchtest

import (
	"errors"
	"testing"

	"github.com/lestrrat-3d/sketch"
)

// Verify returns s.Verify for the test's context. An untrustworthy report is
// returned unchanged so the test can inspect its reasons. s MUST NOT be nil.
func Verify(tb testing.TB, s *sketch.Sketch, opts ...sketch.VerifyOption) *sketch.VerificationReport {
	tb.Helper()

	if s == nil {
		tb.Fatalf("sketchtest.Verify: s must not be nil")
		return nil
	}
	return s.Verify(tb.Context(), opts...)
}

// IsTrustworthy verifies s and fails tb unless the report's canonical Check
// verdict passes. It does not solve s first. s MUST NOT be nil.
func IsTrustworthy(tb testing.TB, s *sketch.Sketch, opts ...sketch.VerifyOption) *sketch.VerificationReport {
	tb.Helper()

	report := Verify(tb, s, opts...)
	if report == nil {
		return nil
	}
	if reasons := report.Check(); reasons != nil {
		tb.Fatalf("verify: report is not trustworthy; %s", reasonBlock(reasons))
		return nil
	}
	return report
}

func requireAnalysed(tb testing.TB, helper string, report *sketch.VerificationReport) bool {
	tb.Helper()

	if report == nil {
		tb.Fatalf("sketchtest.%s: report must not be nil", helper)
		return false
	}
	if !report.Analysed() {
		tb.Fatalf("verify: report was not analysed; %s", reasonBlock(report.Check()))
		return false
	}
	return true
}

// HasStatus fails tb unless an analysed report has status want.
func HasStatus(tb testing.TB, report *sketch.VerificationReport, want sketch.Status) {
	tb.Helper()

	if !requireAnalysed(tb, "HasStatus", report) {
		return
	}
	if report.Status != want {
		tb.Fatalf("verify: status is %s, want %s", report.Status, want)
	}
}

// HasDOF fails tb unless an analysed report has want remaining degrees of
// freedom.
func HasDOF(tb testing.TB, report *sketch.VerificationReport, want int) {
	tb.Helper()

	if !requireAnalysed(tb, "HasDOF", report) {
		return
	}
	if report.DOF != want {
		tb.Fatalf("verify: DOF is %d, want %d", report.DOF, want)
	}
}

// FindReasons returns every canonical verification reason that matches
// sentinel through errors.Is. It fails tb when none match. report and sentinel
// MUST NOT be nil.
func FindReasons(tb testing.TB, report *sketch.VerificationReport, sentinel error) []error {
	tb.Helper()

	if report == nil {
		tb.Fatalf("sketchtest.FindReasons: report must not be nil")
		return nil
	}
	if sentinel == nil {
		tb.Fatalf("sketchtest.FindReasons: sentinel must not be nil")
		return nil
	}

	reasons := report.Check()
	if reasons == nil {
		tb.Fatalf("verify: report contains no reason matching %v; 0 reason(s)", sentinel)
		return nil
	}
	var found []error
	for _, reason := range reasons.Unwrap() {
		if errors.Is(reason, sentinel) {
			found = append(found, reason)
		}
	}
	if len(found) == 0 {
		tb.Fatalf("verify: report contains no reason matching %v; %s", sentinel, reasonBlock(reasons))
		return nil
	}
	return found
}

// HasOnlyReasons fails tb when report carries a canonical verification reason
// that matches none of allowed. Passing no allowed sentinel requires a clean
// report. report and every allowed sentinel MUST NOT be nil.
func HasOnlyReasons(tb testing.TB, report *sketch.VerificationReport, allowed ...error) {
	tb.Helper()

	if report == nil {
		tb.Fatalf("sketchtest.HasOnlyReasons: report must not be nil")
		return
	}
	for _, sentinel := range allowed {
		if sentinel == nil {
			tb.Fatalf("sketchtest.HasOnlyReasons: allowed sentinel must not be nil")
			return
		}
	}

	reasons := report.Check()
	if reasons == nil {
		return
	}
	var disallowed []error
	for _, reason := range reasons.Unwrap() {
		matched := false
		for _, sentinel := range allowed {
			if errors.Is(reason, sentinel) {
				matched = true
				break
			}
		}
		if !matched {
			disallowed = append(disallowed, reason)
		}
	}
	if len(disallowed) > 0 {
		tb.Fatalf("verify: report contains disallowed reasons; %s", errorsBlock(disallowed))
	}
}

// FindConflict returns the conflict set for constraint from an analysed
// report. Matching uses exact constraint identity. report and constraint MUST
// NOT be nil.
func FindConflict(
	tb testing.TB,
	report *sketch.VerificationReport,
	constraint sketch.Constraint,
) *sketch.ConflictSet {
	tb.Helper()

	if !requireAnalysed(tb, "FindConflict", report) {
		return nil
	}
	if constraint == nil {
		tb.Fatalf("sketchtest.FindConflict: constraint must not be nil")
		return nil
	}
	for i := range report.Conflicts {
		if report.Conflicts[i].Constraint == constraint {
			return &report.Conflicts[i]
		}
	}
	tb.Fatalf("verify: report contains no conflict for %s", constraintName(constraint))
	return nil
}

// HasFreePoints fails tb unless an analysed report contains exactly the wanted
// point identities. Slice order is ignored. report and every wanted point MUST
// NOT be nil.
func HasFreePoints(tb testing.TB, report *sketch.VerificationReport, want ...*sketch.Point) {
	tb.Helper()

	if !requireAnalysed(tb, "HasFreePoints", report) {
		return
	}
	for _, point := range want {
		if point == nil {
			tb.Fatalf("sketchtest.HasFreePoints: wanted point must not be nil")
			return
		}
	}

	wantCounts := make(map[*sketch.Point]int, len(want))
	for _, point := range want {
		wantCounts[point]++
	}
	gotCounts := make(map[*sketch.Point]int, len(report.FreePoints))
	for _, point := range report.FreePoints {
		gotCounts[point]++
	}
	if len(report.FreePoints) == len(want) {
		matches := true
		for point, count := range wantCounts {
			if gotCounts[point] != count {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	tb.Fatalf("verify: free points are %s, want %s", pointsText(report.FreePoints), pointsText(want))
}
