package sketch_test

import (
	"context"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// CheckConstraint is documented as non-mutating and error-returning, so every
// input shape a caller can construct must come back as an error rather than a
// panic. A corrupt candidate — nil, a typed nil, or one holding a nil operand —
// reaches three different dereferences: constraintRefs (the ownership screen),
// allocVars (for an aux-variable type) and residual.
func TestCheckConstraintRefusesCorruptCandidate(t *testing.T) {
	var nilDistance *sketch.Distance
	for _, tc := range []struct {
		name  string
		build func(*sketch.Sketch) sketch.Constraint
	}{
		{"nil candidate", func(*sketch.Sketch) sketch.Constraint { return nil }},
		{"typed-nil candidate", func(*sketch.Sketch) sketch.Constraint { return nilDistance }},
		{"nil point operand", func(s *sketch.Sketch) sketch.Constraint {
			return sketch.NewDistance(s.CreatePoint(0, 0), nil, 5)
		}},
		{"typed-nil entity operand", func(s *sketch.Sketch) sketch.Constraint {
			return sketch.NewPointOnCircle(s.CreatePoint(0, 0), nil)
		}},
		{"untyped-nil sealed-interface operand", func(*sketch.Sketch) sketch.Constraint {
			return sketch.NewRadius(nil, 5)
		}},
		{"nil operand on an aux-variable type", func(s *sketch.Sketch) sketch.Constraint {
			return sketch.NewPointOnSpline(s.CreatePoint(0, 0), nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			var err error
			require.NotPanics(t, func() { err = s.CheckConstraint(tc.build(s)) })
			require.ErrorIs(t, err, sketch.ErrCorruptHandle)
		})
	}
}

// The corrupt screen must not swallow the ownership screen beside it: a removed
// operand is a live handle this sketch does not own, and stays ErrForeignHandle.
func TestCheckConstraintRemovedOperandStillForeign(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(0, 0)
	q := s.CreatePoint(1, 0)
	require.True(t, s.RemovePoint(q))
	err := s.CheckConstraint(sketch.NewDistance(p, q, 5))
	require.ErrorIs(t, err, sketch.ErrForeignHandle)
	require.NotErrorIs(t, err, sketch.ErrCorruptHandle)
}

// A candidate can carry BOTH defects at once: NewDistance(foreignPoint, nil, 5)
// names a point this sketch does not own AND a nil operand. The corruption
// screen runs first — it must, since the ownership screen dereferences the
// candidate through constraintRefs — so the candidate reads corrupt, never
// foreign, and the foreign operand is never evaluated. Nothing is lost: once
// committed, Verify reports the constraint's foreign handle on its own account.
func TestCheckConstraintCorruptAndForeignReportsCorrupt(t *testing.T) {
	s := newSketch(t)
	fp := foreignPoint(t, 0, 5, 5)

	c := sketch.NewDistance(fp, nil, 5)
	err := s.CheckConstraint(c)
	require.ErrorIs(t, err, sketch.ErrCorruptHandle)
	require.NotErrorIs(t, err, sketch.ErrForeignHandle)

	s.AddConstraint(c)
	require.Len(t, s.Constraints(), 1)

	rep := s.Verify(context.Background())
	require.True(t, rep.ForeignHandles)
	require.ErrorIs(t, rep.Check(), sketch.ErrForeignHandle)
}

// The load-bearing half: a candidate whose operands are all owned is probed as
// before, including the aux-variable type the screen runs ahead of.
func TestCheckConstraintOwnedCandidatesUnchanged(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	require.NoError(t, s.CheckConstraint(sketch.NewDistance(a, b, 10)))

	c0 := s.CreatePoint(0, 5)
	c1 := s.CreatePoint(2, 9)
	c2 := s.CreatePoint(6, 9)
	c3 := s.CreatePoint(8, 5)
	sp, err := s.CreateSpline(c0, c1, c2, c3)
	require.NoError(t, err)
	require.NoError(t, s.CheckConstraint(sketch.NewPointOnSpline(s.CreatePoint(4, 6), sp)))
	require.Equal(t, 0, len(s.Constraints()))
}

// A nil or typed-nil constraint names no geometry, so no report can describe it.
// Committing one poisons every later pass instead: residuals dereferences it, so
// Solve, Verify, Diagnose and RedundantConstraints all panic.
func TestAddConstraintDropsNilCandidate(t *testing.T) {
	var nilDistance *sketch.Distance
	for _, tc := range []struct {
		name string
		c    sketch.Constraint
	}{
		{"nil", nil},
		{"typed nil", nilDistance},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			p := s.CreatePoint(0, 0)
			s.AddConstraint(sketch.NewCoincident(p, s.Origin()))
			require.NotPanics(t, func() { s.AddConstraint(tc.c) })
			require.Len(t, s.Constraints(), 1)
			require.NotPanics(t, func() { s.Verify(context.Background()) })
			require.NotPanics(t, func() { s.Diagnose() })
		})
	}
}

// A constraint holding a nil OPERAND is committed on purpose, so Verify stays
// loud about it. An aux-variable type could not reach that state: allocVars read
// the nil operand's coordinates inside AddConstraint and panicked.
func TestAddConstraintCommitsNilOperandAuxConstraint(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(0, 0)
	require.NotPanics(t, func() { s.AddConstraint(sketch.NewPointOnSpline(p, nil)) })
	require.Len(t, s.Constraints(), 1)

	rep := s.Verify(context.Background())
	require.False(t, rep.Trustworthy())
	require.ErrorIs(t, rep.Check(), sketch.ErrVerificationIncomplete)
}
