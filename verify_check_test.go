package sketch_test

import (
	"errors"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// reasonsOf runs Check and returns its reasons, failing when the report passed.
func reasonsOf(t *testing.T, rep *sketch.VerificationReport) []error {
	t.Helper()
	err := rep.Check()
	require.Error(t, err, "expected the report to fail at least one condition")
	return err.Unwrap()
}

func TestCheckCleanReportReturnsNilInterface(t *testing.T) {
	// The nil-interface trap: Check returns the Reasons INTERFACE, so a concrete
	// nil pointer stuffed into it would make `err != nil` true for a sound sketch —
	// the exact failure a caller cannot see and cannot work around. Assert on the
	// interface comparison a caller actually writes, not just on Trustworthy.
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context())
	require.True(t, rep.Trustworthy(), "a clean rectangle is trustworthy")

	reasons := rep.Check()
	require.Nil(t, reasons, "a trustworthy report has no reasons")

	var asErr error = rep.Check()
	require.NoError(t, asErr, "assigned to a plain error, a clean Check is still nil")
	require.False(t, asErr != nil, "the interface itself must be nil, not a nil pointer inside one")
}

func TestCheckAgreesWithTrustworthy(t *testing.T) {
	// Trustworthy is DEFINED as Check() == nil, so the two cannot drift. Pin that
	// across reports that fail in different ways, including one that passes.
	build := map[string]func(t *testing.T) *sketch.VerificationReport{
		"clean rectangle": func(t *testing.T) *sketch.VerificationReport {
			s := newSketch(t)
			r := s.CreateRectangle(0, 0, 20, 12)
			s.Fix(r.A)
			s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))
			_, err := s.Solve(t.Context())
			require.NoError(t, err)
			return s.Verify(t.Context())
		},
		"underconstrained line": func(t *testing.T) *sketch.VerificationReport {
			s := newSketch(t)
			s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
			_, err := s.Solve(t.Context())
			require.NoError(t, err)
			return s.Verify(t.Context())
		},
		"self-intersecting bowtie": func(t *testing.T) *sketch.VerificationReport {
			s := newSketch(t)
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(4, 4)
			c := s.CreatePoint(4, 0)
			d := s.CreatePoint(0, 4)
			s.Fix(a)
			s.CreateLine(a, b)
			s.CreateLine(b, c)
			s.CreateLine(c, d)
			s.CreateLine(d, a)
			s.AddConstraint(sketch.NewDistance(a, b, 4*1.41421356), sketch.NewDistance(b, c, 4),
				sketch.NewDistance(c, d, 4*1.41421356), sketch.NewDistance(d, a, 4),
				sketch.NewHorizontalDistance(a, c, 4), sketch.NewVerticalDistance(a, c, 0))
			_, err := s.Solve(t.Context())
			require.NoError(t, err)
			return s.Verify(t.Context())
		},
	}
	for name, mk := range build {
		t.Run(name, func(t *testing.T) {
			rep := mk(t)
			require.Equal(t, rep.Check() == nil, rep.Trustworthy(),
				"Trustworthy must be exactly Check() == nil")
		})
	}
}

func TestCheckNamesEachFailedCondition(t *testing.T) {
	t.Run("underconstrained", func(t *testing.T) {
		s := newSketch(t)
		s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)

		rep := s.Verify(t.Context())
		err2 := rep.Check()
		require.ErrorIs(t, err2, sketch.ErrNotFullyConstrained,
			"errors.Is matches through Unwrap without ranging")
		require.NotErrorIs(t, err2, sketch.ErrConflicting, "no constraints to conflict")
		require.Contains(t, err2.Error(), "DOF 4", "the reason carries the specifics")

		for _, reason := range reasonsOf(t, rep) {
			require.NotErrorIs(t, reason, sketch.ErrUnsolvable,
				"a constraint-free sketch solves; only its DOF is reported")
		}
	})

	t.Run("redundant", func(t *testing.T) {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10, 0)
		s.CreateLine(a, b)
		s.Fix(a)
		s.AddConstraint(sketch.NewDistance(a, b, 10), sketch.NewDistance(a, b, 10))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)

		require.ErrorIs(t, s.Verify(t.Context()).Check(), sketch.ErrRedundant)
	})

	t.Run("self-intersecting profile", func(t *testing.T) {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(4, 4)
		c := s.CreatePoint(4, 0)
		d := s.CreatePoint(0, 4)
		s.CreateLine(a, b)
		s.CreateLine(b, c)
		s.CreateLine(c, d)
		s.CreateLine(d, a)
		_, err := s.Solve(t.Context())
		require.NoError(t, err)

		require.ErrorIs(t, s.Verify(t.Context()).Check(), sketch.ErrInvalidProfile)
	})
}

func TestCheckWaiveOneConditionKeepsTheRest(t *testing.T) {
	// The reported use case: a sketch built from unsigned constraints is ambiguous
	// BY DESIGN, so its consumer waives that one condition and keeps every other.
	// A triangle pinned by two distances admits the mirrored apex.
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(5, 6)
	s.Fix(a)
	ab := s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, a)
	s.AddConstraint(sketch.NewHorizontal(ab))
	s.AddConstraint(sketch.NewDistance(a, b, 10), sketch.NewDistance(a, c, 8), sketch.NewDistance(b, c, 7))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context(), sketch.WithProbe())
	require.NotNil(t, rep.Probe)
	require.True(t, rep.Probe.Ambiguous(), "the mirrored apex is a second configuration")
	require.False(t, rep.Trustworthy(), "the whole verdict fails on ambiguity alone")

	// Everything except ambiguity is decided per reason.
	var fatal []error
	for _, reason := range reasonsOf(t, rep) {
		if !errors.Is(reason, sketch.ErrAmbiguous) {
			fatal = append(fatal, reason)
		}
	}
	require.Empty(t, fatal, "ambiguity is the ONLY condition this sketch fails: %v", fatal)
}

func TestCheckReportsEveryFailedConditionAtOnce(t *testing.T) {
	// A report failing several conditions returns one reason each, so a caller
	// waiving one still sees the others rather than the first only.
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(4, 4)
	c := s.CreatePoint(4, 0)
	d := s.CreatePoint(0, 4)
	s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, d)
	s.CreateLine(d, a) // a bowtie, and nothing pins it
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context())
	reasons := reasonsOf(t, rep)
	require.GreaterOrEqual(t, len(reasons), 2, "an unpinned bowtie fails on DOF and on profiles")

	err2 := rep.Check()
	require.ErrorIs(t, err2, sketch.ErrNotFullyConstrained)
	require.ErrorIs(t, err2, sketch.ErrInvalidProfile)

	// Each element answers errors.Is on its OWN sentinel, which is what makes a
	// per-reason waiver possible.
	matched := map[error]int{}
	for _, reason := range reasons {
		for _, sentinel := range []error{sketch.ErrNotFullyConstrained, sketch.ErrInvalidProfile} {
			if errors.Is(reason, sentinel) {
				matched[sentinel]++
			}
		}
	}
	require.Equal(t, 1, matched[sketch.ErrNotFullyConstrained], "exactly one reason per condition")
	require.Equal(t, 1, matched[sketch.ErrInvalidProfile], "exactly one reason per condition")
}

func TestCheckSentinelsAreDistinct(t *testing.T) {
	// Every condition needs its OWN sentinel: two conditions sharing one would make
	// a waiver silently waive both.
	sentinels := []error{
		sketch.ErrUnsolvable,
		sketch.ErrNotFullyConstrained,
		sketch.ErrConflicting,
		sketch.ErrRedundant,
		sketch.ErrStaleReference,
		sketch.ErrBrokenReference,
		sketch.ErrForeignHandle,
		sketch.ErrInvalidProfile,
		sketch.ErrInvalidParameter,
		sketch.ErrNearSingular,
		sketch.ErrProbeIncomplete,
		sketch.ErrAmbiguous,
	}
	seen := map[string]struct{}{}
	for _, s := range sentinels {
		require.Error(t, s)
		_, dup := seen[s.Error()]
		require.Falsef(t, dup, "duplicate sentinel message %q", s.Error())
		seen[s.Error()] = struct{}{}
		for _, other := range sentinels {
			if s == other {
				continue
			}
			require.NotErrorIs(t, s, other, "sentinels must not match one another")
		}
	}
}
