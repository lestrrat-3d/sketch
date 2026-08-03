package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// splineDonor builds a healthy sketch whose only constraint owns auxiliary
// solver variables (a point confined to a spline: foot parameter + two box
// slacks). It is the donor in the ownership tests below: every one of its
// handles is its own, so nothing in its own report can ever flag it — which is
// exactly why a foreign commit corrupting it went unnoticed.
func splineDonor(t *testing.T) (*sketch.Sketch, sketch.Constraint) {
	t.Helper()
	s := newSketch(t)
	c0 := s.CreatePoint(0, 0)
	c1 := s.CreatePoint(2, 4)
	c2 := s.CreatePoint(6, 4)
	c3 := s.CreatePoint(8, 0)
	sp, err := s.CreateSpline(c0, c1, c2, c3)
	require.NoError(t, err)
	p := s.CreatePoint(4, 1)
	c := sketch.NewPointOnSpline(p, sp)
	s.AddConstraint(c)
	return s, c
}

// donorState is the observable state of the donor sketch that a foreign commit
// elsewhere must not move.
type donorState struct {
	dof       int
	status    sketch.Status
	solvable  bool
	residual  float64
	conRows   []float64
	redundant int
	conflicts int
}

func readDonor(t *testing.T, s *sketch.Sketch, c sketch.Constraint) donorState {
	t.Helper()
	rep := s.Verify(t.Context())
	return donorState{
		dof:       rep.DOF,
		status:    rep.Status,
		solvable:  rep.Solvable,
		residual:  rep.Residual,
		conRows:   sketch.ConstraintResiduals(c),
		redundant: len(rep.Redundant),
		conflicts: len(rep.Conflicts),
	}
}

// A constraint already committed to one sketch, then offered to another, must
// leave the donor exactly as it was. Both index regimes are covered: a receiver
// SMALLER than the donor (the donor's variable indices run off the receiver's
// vector, which panicked the donor's DOF and Verify) and a receiver LARGER than
// it (the indices resolve, so the donor silently read a stranger's coordinates
// and flipped from underconstrained to overconstrained).
func TestForeignConstraintLeavesDonorUntouched(t *testing.T) {
	for _, tc := range []struct {
		name     string
		receiver func(*testing.T) *sketch.Sketch
	}{
		{
			name: "receiver smaller than donor",
			receiver: func(t *testing.T) *sketch.Sketch {
				r := newSketch(t)
				r.CreatePoint(1, 1)
				return r
			},
		},
		{
			name: "receiver larger than donor",
			receiver: func(t *testing.T) *sketch.Sketch {
				r := newSketch(t)
				for i := 0; i < 12; i++ {
					r.CreatePoint(float64(i), float64(i))
				}
				return r
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			donor, c := splineDonor(t)
			before := readDonor(t, donor, c)
			require.Equal(t, 9, before.dof, "the donor starts under-constrained")
			require.Len(t, before.conRows, 4, "the aux vars are allocated in the donor")

			receiver := tc.receiver(t)
			receiver.AddConstraint(c)

			require.Equal(t, before, readDonor(t, donor, c), "the donor is untouched by a commit elsewhere")

			// The donor still solves to the same answer it did before.
			_, err := donor.Solve(t.Context())
			require.NoError(t, err)
			require.True(t, donor.Verify(t.Context()).Solvable)
		})
	}
}

// CheckConstraint is documented as non-mutating and error-returning, so a
// foreign candidate must come back as an error rather than a panic — and
// neither sketch may move.
func TestCheckConstraintRefusesForeignCandidate(t *testing.T) {
	donor, c := splineDonor(t)
	before := readDonor(t, donor, c)

	receiver := newSketch(t)
	receiver.CreatePoint(1, 1)
	rdofBefore := receiver.DOF()

	require.ErrorIs(t, receiver.CheckConstraint(c), sketch.ErrForeignHandle)

	require.Equal(t, before, readDonor(t, donor, c), "the donor is untouched by the probe")
	require.Equal(t, rdofBefore, receiver.DOF(), "the probe leaves the receiver untouched too")
}

// Refusing to parameterize a foreign constraint must not make it disappear:
// dropping it would erase the very signal the receiving sketch reports.
func TestForeignConstraintStaysReportedByVerify(t *testing.T) {
	donor, c := splineDonor(t)

	receiver := newSketch(t)
	receiver.CreatePoint(1, 1)
	receiver.AddConstraint(c)

	require.Contains(t, receiver.Constraints(), c, "the constraint is committed, not silently dropped")
	rep := receiver.Verify(t.Context())
	require.True(t, rep.ForeignHandles)
	require.ErrorIs(t, rep.Check(), sketch.ErrForeignHandle)
	require.False(t, rep.Trustworthy())

	// Removing it again must not retire a variable the donor owns: the donor's
	// constraint keeps its aux rows and the sketch keeps its verdict.
	require.True(t, receiver.RemoveConstraint(c))
	require.Len(t, sketch.ConstraintResiduals(c), 4, "the donor's aux vars survive the receiver's removal")
	require.Equal(t, 9, donor.Verify(t.Context()).DOF)
}

// The screen must be invisible to constraints built from the sketch's own
// geometry. Each case is an aux-var owner: the residual arity grows when
// AddConstraint parameterizes it, the sketch solves, and RemoveConstraint
// retires the variables again (arity back to the unparameterized count).
func TestOwnAuxVarConstraintsUnaffected(t *testing.T) {
	t.Run("interior arc tangency", func(t *testing.T) {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10, 0)
		s.Fix(a)
		s.Fix(b)
		center := s.CreatePoint(5, 5)
		s.Fix(center)
		start := s.CreatePoint(1, 1)
		end := s.CreatePoint(9, 1)
		arc := s.CreateArc(center, start, end)
		c := sketch.NewTangent(s.CreateLine(a, b), arc)

		require.Len(t, sketch.ConstraintResiduals(c), 1, "unparameterized: the carrier row only")
		require.NoError(t, s.CheckConstraint(c))
		require.Len(t, sketch.ConstraintResiduals(c), 1, "the probe rolled its allocation back")

		s.AddConstraint(c)
		require.Len(t, sketch.ConstraintResiduals(c), 2, "committed: the sweep-slack row is in")

		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 5, arc.R(), 1e-6)

		require.True(t, s.RemoveConstraint(c))
		require.Len(t, sketch.ConstraintResiduals(c), 1, "the slack is retired again")
	})

	t.Run("arc length dimension", func(t *testing.T) {
		s := newSketch(t)
		center := s.CreatePoint(0, 0)
		start := s.CreatePoint(4, 0)
		s.Fix(center)
		s.Fix(start)
		end := s.CreatePoint(0, 4)
		arc := s.CreateArc(center, start, end)
		c := sketch.NewArcLength(arc, 3*math.Pi)

		require.Len(t, sketch.ConstraintResiduals(c), 1, "unparameterized: the measured-length row only")
		s.AddConstraint(c)
		require.Len(t, sketch.ConstraintResiduals(c), 2, "committed: the unwrapped-sweep coupling row is in")

		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 3*math.Pi/4, arc.Sweep(), 1e-6)

		require.True(t, s.RemoveConstraint(c))
		require.Len(t, sketch.ConstraintResiduals(c), 1, "theta is retired again")
	})

	t.Run("point on spline", func(t *testing.T) {
		s := newSketch(t)
		c0 := s.CreatePoint(0, 0)
		c1 := s.CreatePoint(2, 4)
		c2 := s.CreatePoint(6, 4)
		c3 := s.CreatePoint(8, 0)
		sp, err := s.CreateSpline(c0, c1, c2, c3)
		require.NoError(t, err)
		for _, cp := range []*sketch.Point{c0, c1, c2, c3} {
			s.Fix(cp)
		}
		p := s.CreatePoint(4, 1)
		c := sketch.NewPointOnSpline(p, sp)

		require.Empty(t, sketch.ConstraintResiduals(c), "unparameterized: no rows at all")
		require.NoError(t, s.CheckConstraint(c))
		require.Empty(t, sketch.ConstraintResiduals(c), "the probe rolled its allocation back")

		s.AddConstraint(c)
		require.Len(t, sketch.ConstraintResiduals(c), 4, "committed: membership + box rows")

		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 0, distToSpline(p, sp), 1e-6, "the point landed on the curve")

		require.True(t, s.RemoveConstraint(c))
		require.Empty(t, sketch.ConstraintResiduals(c), "the foot parameter and box slacks are retired")
	})
}
