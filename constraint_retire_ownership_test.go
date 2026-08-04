package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

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
