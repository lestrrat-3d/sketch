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

// arcLengthDonor builds a healthy sketch holding one committed ArcLength — the
// aux-var dimension whose only operand is an exported field, so a consumer can
// point it at ANOTHER sketch's arc after this sketch has already allocated the
// unwrapped-sweep variable. Its arc has radius 4 and a quarter-turn sweep.
func arcLengthDonor(t *testing.T) (*sketch.Sketch, *sketch.ArcLength) {
	t.Helper()
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(0, 4))
	c := sketch.NewArcLength(arc, 3*math.Pi)
	s.AddConstraint(c)
	require.Len(t, sketch.ConstraintResiduals(c), 2, "the donor allocated the unwrapped-sweep variable")
	return s, c
}

// arcLengthReceiver builds a sketch owning an arc of radius 5, plus extra free
// points so its variable vector can be made shorter or longer than the donor's.
// The donor's arc costs six variables and its aux variable takes index 6, so
// with no extra points the receiver's vector is too short for that index and
// with extra points the index resolves onto the first extra point's x.
func arcLengthReceiver(t *testing.T, extra int) (*sketch.Sketch, *sketch.Arc, []*sketch.Point) {
	t.Helper()
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))
	var pts []*sketch.Point
	for i := 0; i < extra; i++ {
		pts = append(pts, s.CreatePoint(float64(6+i), float64(7+i)))
	}
	return s, arc, pts
}

// The reported case. A constraint the donor parameterized, with its arc operand
// rewired to the RECEIVER's own arc, passes the reference-ownership screen —
// every handle it names belongs to the receiver — while its stored index still
// addresses the donor's variable vector. The receiver must refuse it outright,
// and neither sketch may panic or move.
func TestForeignAllocationRefusedOnAdd(t *testing.T) {
	donor, c := arcLengthDonor(t)
	receiver, rarc, _ := arcLengthReceiver(t, 0)
	c.A = rarc

	// Read the donor AFTER the rewire: the rewire itself is what the donor's own
	// reference scan reports, and the receiver's refusal must change nothing more.
	beforeDOF := donor.DOF()
	beforeRows := sketch.ConstraintResiduals(c)
	beforeRep := donor.Verify(t.Context())
	rDOFBefore := receiver.DOF()

	require.NotPanics(t, func() { receiver.AddConstraint(c) })

	require.NotContains(t, receiver.Constraints(), c, "a constraint another sketch allocated is dropped, not committed")

	require.NotPanics(t, func() {
		require.Equal(t, beforeDOF, donor.DOF())
		require.Equal(t, beforeRows, sketch.ConstraintResiduals(c))
		after := donor.Verify(t.Context())
		require.Equal(t, beforeRep.DOF, after.DOF)
		require.Equal(t, beforeRep.Status, after.Status)
		require.Equal(t, beforeRep.ForeignHandles, after.ForeignHandles)
	}, "the donor is untouched by the receiver's refusal")

	require.NotPanics(t, func() {
		require.Equal(t, rDOFBefore, receiver.DOF())
		require.NotNil(t, receiver.Verify(t.Context()))
	}, "the receiver is untouched too")

	// The rewired operand is what the DONOR's own report flags, so refusing the
	// constraint here loses no signal.
	require.True(t, donor.Verify(t.Context()).ForeignHandles)
}

// The quieter half of the same defect: give the receiver a LONGER variable
// vector and the donor's stale index resolves instead of running off the end,
// silently aliasing one of the receiver's own point coordinates. Nothing panics
// there, and the receiver's report reads merely under-constrained — but a solve
// drags the aliased point. The refusal must leave the receiver's geometry alone.
func TestForeignAllocationDoesNotAliasReceiverCoordinate(t *testing.T) {
	donor, c := arcLengthDonor(t)
	receiver, rarc, pts := arcLengthReceiver(t, 2)
	require.Len(t, pts, 2)
	aliased := pts[0]
	x, y := aliased.X(), aliased.Y()
	c.A = rarc

	receiver.AddConstraint(c)
	require.NotContains(t, receiver.Constraints(), c, "a constraint another sketch allocated is dropped, not committed")

	_, err := receiver.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, x, aliased.X(), "the receiver's own point is not driven by the donor's aux variable")
	require.Equal(t, y, aliased.Y())
	require.InDelta(t, 5, rarc.R(), 1e-9, "the receiver's arc keeps its radius")

	// And the donor still owns its variable: its constraint keeps both rows.
	require.Len(t, sketch.ConstraintResiduals(c), 2)
	require.NotNil(t, donor.Verify(t.Context()))
}

// CheckConstraint is documented as non-mutating and error-returning, so the same
// candidate must come back as an error rather than a panic, and neither sketch
// may move. This path is not covered by the add door: the branch's rollback is
// gated on an allocation the candidate no longer needs, so it never ran here.
func TestCheckConstraintRefusesForeignAllocation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra int
	}{
		{name: "receiver shorter than the donor", extra: 0},
		{name: "receiver longer than the donor", extra: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			donor, c := arcLengthDonor(t)
			receiver, rarc, _ := arcLengthReceiver(t, tc.extra)
			c.A = rarc

			beforeDOF := donor.DOF()
			beforeRows := sketch.ConstraintResiduals(c)
			rDOFBefore := receiver.DOF()

			var err error
			require.NotPanics(t, func() { err = receiver.CheckConstraint(c) })
			require.ErrorIs(t, err, sketch.ErrForeignHandle)

			require.NotPanics(t, func() {
				require.Equal(t, beforeDOF, donor.DOF())
				require.Equal(t, beforeRows, sketch.ConstraintResiduals(c))
				require.Equal(t, rDOFBefore, receiver.DOF())
			}, "the probe leaves both sketches untouched")
		})
	}
}

// The screen asks who ALLOCATED the variables, so a constraint whose variables
// have been given back must be freely re-addable elsewhere: removal ends the
// ownership it recorded. Without that, the screen refuses a legitimate move and
// the receiving sketch is left with a degree of freedom the dimension should
// have removed.
func TestRemovedConstraintCanBeAddedToAnotherSketch(t *testing.T) {
	donor, c := arcLengthDonor(t)
	require.True(t, donor.RemoveConstraint(c))
	require.Len(t, sketch.ConstraintResiduals(c), 1, "the donor took its variable back")

	moved, movedArc, _ := arcLengthReceiver(t, 0)
	c.A = movedArc
	moved.AddConstraint(c)

	require.Contains(t, moved.Constraints(), c, "the moved constraint is committed")
	require.Len(t, sketch.ConstraintResiduals(c), 2, "and parameterized in its new sketch")

	// It behaves exactly like a dimension authored in that sketch from scratch.
	fresh, freshArc, _ := arcLengthReceiver(t, 0)
	fresh.AddConstraint(sketch.NewArcLength(freshArc, 3*math.Pi))
	require.Equal(t, fresh.DOF(), moved.DOF())

	_, err := moved.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 3*math.Pi, movedArc.R()*movedArc.Sweep(), 1e-6)
}

// The same for a candidate a probe allocated: CheckConstraint must hand the
// variables back AND forget the ownership they recorded, or its own screen
// refuses the commit the probe was asked about.
func TestProbedCandidateCanStillBeCommittedElsewhere(t *testing.T) {
	probe, probeArc, _ := arcLengthReceiver(t, 0)
	c := sketch.NewArcLength(probeArc, 3*math.Pi)
	require.NoError(t, probe.CheckConstraint(c))
	require.Len(t, sketch.ConstraintResiduals(c), 1, "the probe rolled its allocation back")

	other, otherArc, _ := arcLengthReceiver(t, 0)
	c.A = otherArc
	other.AddConstraint(c)
	require.Contains(t, other.Constraints(), c)
	require.Len(t, sketch.ConstraintResiduals(c), 2, "and it parameterized in the sketch that committed it")
}

// A DRIVEN dimension owns no auxiliary variable — it contributes no residual
// rows, so one would be left unconstrained — while still carrying the sketch
// pointer allocVars wrote. The screen must therefore read the live allocation,
// not the pointer: there are no stale indices to address the donor's vector
// with, so a receiver that owns every handle the dimension names has every right
// to take it. Trigger one, the post-commit toggle: SetDriven(true) hands theta
// back and leaves the owner behind.
func TestDrivenAfterCommitCanBeAddedToAnotherSketch(t *testing.T) {
	donor, c := arcLengthDonor(t)
	c.SetDriven(true)
	require.Len(t, sketch.ConstraintResiduals(c), 1, "a driven dimension gave its variable back")
	require.Contains(t, donor.Constraints(), c)

	receiver, rarc, _ := arcLengthReceiver(t, 0)
	c.A = rarc

	require.NoError(t, receiver.CheckConstraint(c), "nothing addresses the donor's vector any more")
	receiver.AddConstraint(c)
	require.Contains(t, receiver.Constraints(), c, "the receiver holds the constraint")

	// And it parameterizes in the receiver when it goes back to driving there.
	c.SetDriven(false)
	require.Len(t, sketch.ConstraintResiduals(c), 2, "the receiver allocated the unwrapped-sweep variable")
	_, err := receiver.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 3*math.Pi, rarc.R()*rarc.Sweep(), 1e-6)
}

// Trigger two, and the sharper one: the dimension is driven BEFORE it is ever
// committed, so no SetDriven call follows the commit at all. allocVars writes the
// sketch pointer ahead of its own driven guard, so the donor records an owner
// while allocating nothing.
func TestDrivenBeforeCommitCanBeAddedToAnotherSketch(t *testing.T) {
	donor := newSketch(t)
	darc := donor.CreateArc(donor.CreatePoint(0, 0), donor.CreatePoint(4, 0), donor.CreatePoint(0, 4))
	c := sketch.NewArcLength(darc, 3*math.Pi)
	c.SetDriven(true)
	donor.AddConstraint(c)
	require.Len(t, sketch.ConstraintResiduals(c), 1, "a dimension driven at commit time allocates nothing")

	receiver, rarc, _ := arcLengthReceiver(t, 0)
	c.A = rarc

	require.NoError(t, receiver.CheckConstraint(c))
	receiver.AddConstraint(c)
	require.Contains(t, receiver.Constraints(), c, "the receiver holds the constraint")
}

// The same two triggers on the other aux-var dimension whose operands are all
// exported fields, rewiring both of them.
func TestDrivenDistancePointArcCanBeAddedToAnotherSketch(t *testing.T) {
	for _, tc := range []struct {
		name           string
		drivenAtCommit bool
	}{
		{name: "driven after commit", drivenAtCommit: false},
		{name: "driven before commit", drivenAtCommit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			donor := newSketch(t)
			darc := donor.CreateArc(donor.CreatePoint(0, 0), donor.CreatePoint(4, 0), donor.CreatePoint(0, 4))
			dp := donor.CreatePoint(6, 6)
			c := sketch.NewDistancePointArc(dp, darc, 2)
			if tc.drivenAtCommit {
				c.SetDriven(true)
				donor.AddConstraint(c)
			} else {
				donor.AddConstraint(c)
				c.SetDriven(true)
			}
			require.Contains(t, donor.Constraints(), c)

			receiver := newSketch(t)
			rarc := receiver.CreateArc(receiver.CreatePoint(0, 0), receiver.CreatePoint(5, 0), receiver.CreatePoint(0, 5))
			rp := receiver.CreatePoint(9, 9)
			c.P, c.A = rp, rarc

			require.NoError(t, receiver.CheckConstraint(c))
			receiver.AddConstraint(c)
			require.Contains(t, receiver.Constraints(), c, "the receiver holds the constraint")
		})
	}
}

// The guard on the shape this fix deliberately did NOT take: clearing the owner
// pointer in SetDriven's retire closure would make the toggle back to driving
// find no sketch to allocate in, and the dimension would fall back to the
// wrap-discontinuous single-row residual the aux variable exists to avoid.
func TestCommittedDimensionSurvivesDrivenRoundTrip(t *testing.T) {
	s, c := arcLengthDonor(t)
	require.Len(t, sketch.ConstraintResiduals(c), 2)

	c.SetDriven(true)
	require.Len(t, sketch.ConstraintResiduals(c), 1, "driven: no rows to carry a variable")

	c.SetDriven(false)
	require.Len(t, sketch.ConstraintResiduals(c), 2, "driving again: the coupling row is back")

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.NotNil(t, s.Verify(t.Context()))
}

// auxDimCase is one aux-var constraint reachable from outside the package: an
// exported type with exported operand fields, so a consumer can rewire an
// operand after the sketch has already parameterized the constraint. Those three
// are the whole reachable set.
type auxDimCase struct {
	name string
	// build adds the geometry the dimension needs to s and returns the
	// not-yet-committed dimension together with the arc it is built on.
	build func(t *testing.T, s *sketch.Sketch) (sketch.Constraint, *sketch.Arc)
	// setArc points the dimension's arc operand at another arc.
	setArc func(c sketch.Constraint, a *sketch.Arc)
	// setOther points an operand OTHER than the arc at fresh geometry of the
	// given sketch. It is nil for ArcLength, whose only operand is the arc.
	setOther func(c sketch.Constraint, from *sketch.Sketch)
}

func auxDimCases() []auxDimCase {
	return []auxDimCase{
		{
			name: "arc length",
			build: func(t *testing.T, s *sketch.Sketch) (sketch.Constraint, *sketch.Arc) {
				t.Helper()
				arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(0, 4))
				return sketch.NewArcLength(arc, 1), arc
			},
			setArc: func(c sketch.Constraint, a *sketch.Arc) { c.(*sketch.ArcLength).A = a },
		},
		{
			name: "distance point arc",
			build: func(t *testing.T, s *sketch.Sketch) (sketch.Constraint, *sketch.Arc) {
				t.Helper()
				arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(0, 4))
				return sketch.NewDistancePointArc(s.CreatePoint(8, 8), arc, 1), arc
			},
			setArc: func(c sketch.Constraint, a *sketch.Arc) { c.(*sketch.DistancePointArc).A = a },
			setOther: func(c sketch.Constraint, from *sketch.Sketch) {
				c.(*sketch.DistancePointArc).P = from.CreatePoint(3, 3)
			},
		},
		{
			name: "distance line arc",
			build: func(t *testing.T, s *sketch.Sketch) (sketch.Constraint, *sketch.Arc) {
				t.Helper()
				arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(0, 4))
				l := s.CreateLine(s.CreatePoint(-5, 8), s.CreatePoint(5, 8))
				return sketch.NewDistanceLineArc(l, arc, 1), arc
			},
			setArc: func(c sketch.Constraint, a *sketch.Arc) { c.(*sketch.DistanceLineArc).A = a },
			setOther: func(c sketch.Constraint, from *sketch.Sketch) {
				c.(*sketch.DistanceLineArc).L = from.CreateLine(from.CreatePoint(-1, 9), from.CreatePoint(1, 9))
			},
		},
	}
}

// foreignArc returns an arc belonging to a sketch other than s.
func foreignArc(t *testing.T) *sketch.Arc {
	t.Helper()
	other := newSketch(t)
	return other.CreateArc(other.CreatePoint(1, 1), other.CreatePoint(3, 1), other.CreatePoint(1, 3))
}

// Retirement must be decided by which sketch ALLOCATED the auxiliary variable,
// not by whether the constraint still references geometry this sketch owns. A
// constraint parameterized here whose exported operand is rewired to another
// sketch's handle afterwards still owns one of THIS sketch's variables, and
// removing it must give that variable back — otherwise it is grounded forever
// and the sketch carries a phantom free degree of freedom nothing reports.
func TestRemoveConstraintRetiresAuxVarsAfterOperandRewiredToForeignHandle(t *testing.T) {
	for _, tc := range auxDimCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			c, _ := tc.build(t, s)

			before := s.DOF()
			s.AddConstraint(c)
			require.Equal(t, before-1, s.DOF(), "committing allocates one auxiliary variable")

			tc.setArc(c, foreignArc(t))

			require.True(t, s.RemoveConstraint(c))
			require.Equal(t, before, s.DOF(), "the auxiliary variable this sketch allocated is retired")
		})
	}
}

// The same leak needs no second sketch: owns/ownsEntity report a handle this
// sketch has REMOVED as not-owned too, so rewiring an operand to a dead handle
// of the same sketch reaches the identical path.
func TestRemoveConstraintRetiresAuxVarsAfterOperandRewiredToDeadHandle(t *testing.T) {
	for _, tc := range auxDimCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			c, _ := tc.build(t, s)

			// A dead arc of this same sketch: created, then removed, so every
			// ownership predicate reports it foreign while its points live on.
			dead := s.CreateArc(s.CreatePoint(20, 20), s.CreatePoint(24, 20), s.CreatePoint(20, 24))
			require.True(t, s.RemoveEntity(dead))

			before := s.DOF()
			s.AddConstraint(c)
			require.Equal(t, before-1, s.DOF(), "committing allocates one auxiliary variable")

			tc.setArc(c, dead)

			require.True(t, s.RemoveConstraint(c))
			require.Equal(t, before, s.DOF(), "the auxiliary variable this sketch allocated is retired")
		})
	}
}

// RemoveConstraint is not the only door. The removal cascade matches a
// constraint on ONE operand and retires through the same path, so a DIFFERENT
// operand being rewired leaks exactly as above. The expected DOF is read from a
// control run of the identical sequence with no rewire.
func TestRemoveEntityCascadeRetiresAuxVarsWhenAnotherOperandIsForeign(t *testing.T) {
	for _, tc := range auxDimCases() {
		if tc.setOther == nil {
			continue // ArcLength's only operand is the arc the cascade matches
		}
		t.Run(tc.name, func(t *testing.T) {
			control := newSketch(t)
			cc, controlArc := tc.build(t, control)
			control.AddConstraint(cc)
			require.True(t, control.RemoveEntity(controlArc))
			want := control.DOF()

			s := newSketch(t)
			c, arc := tc.build(t, s)
			before := s.DOF()
			s.AddConstraint(c)
			require.Equal(t, before-1, s.DOF(), "committing allocates one auxiliary variable")

			tc.setOther(c, newSketch(t)) // an operand the cascade does NOT match on

			require.True(t, s.RemoveEntity(arc))
			require.Equal(t, want, s.DOF(), "the cascade retires the variable this sketch allocated")
		})
	}
}

// The converse must still hold: a constraint that was FOREIGN when it was added
// was never parameterized here, so its indices name the donor's variables and
// this sketch must not touch them on removal.
func TestRemoveConstraintLeavesDonorAllocatedAuxVars(t *testing.T) {
	for _, tc := range auxDimCases() {
		t.Run(tc.name, func(t *testing.T) {
			donor := newSketch(t)
			c, _ := tc.build(t, donor)
			donorBefore := donor.DOF()
			donor.AddConstraint(c)
			donorDOF := donor.DOF()
			rows := len(sketch.ConstraintResiduals(c))
			require.Equal(t, donorBefore-1, donorDOF)

			receiver := newSketch(t)
			receiver.CreatePoint(1, 1)
			receiverDOF := receiver.DOF()
			receiver.AddConstraint(c)
			require.Contains(t, receiver.Constraints(), c, "a foreign constraint is committed, not dropped")

			require.True(t, receiver.RemoveConstraint(c))
			require.Equal(t, receiverDOF, receiver.DOF(), "the receiver never allocated anything to retire")
			require.Len(t, sketch.ConstraintResiduals(c), rows, "the donor's auxiliary variable survives")
			require.Equal(t, donorDOF, donor.DOF(), "the donor is untouched")

			// The donor itself still retires the variable it allocated.
			require.True(t, donor.RemoveConstraint(c))
			require.Equal(t, donorBefore, donor.DOF())
		})
	}
}
