package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestVerifyWithoutProfilesKeepsConstraintAnalysis(t *testing.T) {
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	full := s.Verify(t.Context())
	skipped := s.Verify(t.Context(), sketch.WithoutProfiles())
	require.True(t, full.Analysed())
	require.True(t, full.ProfilesAnalysed())
	require.True(t, skipped.Analysed())
	require.False(t, skipped.ProfilesAnalysed())
	require.Equal(t, full.Solvable, skipped.Solvable)
	require.Equal(t, full.Residual, skipped.Residual)
	require.Equal(t, full.DOF, skipped.DOF)
	require.Equal(t, full.Status, skipped.Status)
	require.Equal(t, full.Conflicts, skipped.Conflicts)
	require.Equal(t, full.Redundant, skipped.Redundant)
	require.Empty(t, skipped.Profiles)
	require.False(t, skipped.ProfilesValid, "the skipped zero value is not a profile verdict")
	require.True(t, full.Trustworthy())
	require.ErrorIs(t, skipped.Check(), sketch.ErrProfilesUnmeasured)
	require.NotErrorIs(t, skipped.Check(), sketch.ErrInvalidProfile)
}

func TestVerifyWithoutProfilesDoesNotHideInvalidProfiles(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(4, 4)
	c := s.CreatePoint(4, 0)
	d := s.CreatePoint(0, 4)
	for _, p := range []*sketch.Point{a, b, c, d} {
		s.Fix(p)
	}
	s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, d)
	s.CreateLine(d, a)

	full := s.Verify(t.Context())
	skipped := s.Verify(t.Context(), sketch.WithoutProfiles())
	require.True(t, skipped.Analysed())
	require.False(t, skipped.ProfilesAnalysed())
	require.Equal(t, full.Solvable, skipped.Solvable)
	require.Equal(t, full.DOF, skipped.DOF)
	require.Equal(t, full.Status, skipped.Status)
	require.ErrorIs(t, full.Check(), sketch.ErrInvalidProfile)
	require.ErrorIs(t, skipped.Check(), sketch.ErrProfilesUnmeasured)
	require.NotErrorIs(t, skipped.Check(), sketch.ErrInvalidProfile)
}

func TestVerifyWithoutProfilesCombinesWithWithoutConditioning(t *testing.T) {
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context(), sketch.WithoutProfiles(), sketch.WithoutConditioning())
	require.True(t, rep.Analysed())
	require.False(t, rep.ProfilesAnalysed())
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.ErrorIs(t, rep.Check(), sketch.ErrProfilesUnmeasured)
	require.ErrorIs(t, rep.Check(), sketch.ErrConditioningUnmeasured)
}

func TestVerifyWithoutProfilesWithSkippedCoreReportsIncomplete(t *testing.T) {
	s := newSketch(t)
	other := newSketch(t)
	s.AddConstraint(sketch.NewCoincident(s.CreatePoint(0, 0), other.CreateReferencePoint(5, 5, "foreign")))

	rep := s.Verify(t.Context(), sketch.WithoutProfiles())
	require.False(t, rep.Analysed())
	require.False(t, rep.ProfilesAnalysed())
	require.ErrorIs(t, rep.Check(), sketch.ErrForeignHandle)
	require.ErrorIs(t, rep.Check(), sketch.ErrVerificationIncomplete)
	require.NotErrorIs(t, rep.Check(), sketch.ErrProfilesUnmeasured)
	require.NotErrorIs(t, rep.Check(), sketch.ErrInvalidProfile)
}
