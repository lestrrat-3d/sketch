package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestVerifyReuseMatchesIndependentReads proves that Verify's shared committed
// Jacobian (CLAUDE.md's "a single Verify call builds that Jacobian once and
// shares it among its own analyses") produces exactly the same answers the
// independently-rebuilding public methods do: DOF, FreePoints, and the
// redundant/conflicting constraint sets. It also proves Verify does not mutate
// the sketch — two consecutive calls agree, and Revision is unchanged — since
// sharing a Jacobian across a call's own analyses is only sound if nothing
// between building it and the last consumer moves the geometry (see
// committedJacobian's doc comment in conditioning.go).
func TestVerifyReuseMatchesIndependentReads(t *testing.T) {
	for _, f := range solveFixtures() {
		t.Run(f.name, func(t *testing.T) {
			s := f.build(t)
			_, err := s.Solve(t.Context())
			if err != nil {
				require.ErrorIs(t, err, sketch.ErrNotConverged)
			}

			revBefore := s.Revision()
			rep := s.Verify(t.Context())
			require.True(t, rep.Analysed(), "fixtures carry no non-finite/foreign geometry")

			require.Equal(t, s.DOF(), rep.DOF, "Verify's shared-Jacobian DOF must match Sketch.DOF's own build")
			require.Equal(t, s.FreePoints(), rep.FreePoints, "Verify's FreePoints must match Sketch.FreePoints element-wise")

			diag := s.Diagnose()
			require.Equal(t, diag.Redundant, rep.Redundant, "Verify's Redundant must match Sketch.Diagnose")
			var conflicting []sketch.Constraint
			for _, cs := range rep.Conflicts {
				conflicting = append(conflicting, cs.Constraint)
			}
			require.Equal(t, diag.Conflicting, conflicting, "Verify's Conflicts must name the same constraints as Sketch.Diagnose")

			require.Equal(t, revBefore, s.Revision(), "Verify must not mutate the sketch")

			rep2 := s.Verify(t.Context())
			require.True(t, rep2.Analysed())
			require.Equal(t, snapshotVerify(s, rep), snapshotVerify(s, rep2), "two consecutive Verify calls must agree")
			require.Equal(t, revBefore, s.Revision(), "a second Verify call must not mutate the sketch either")
		})
	}
}
