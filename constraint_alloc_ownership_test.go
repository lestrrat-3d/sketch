package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

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
