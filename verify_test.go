package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

func TestVerifyUnderconstrained(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.CreateLine(a, b)

	sketchtest.Solve(t, s)

	rep := sketchtest.Verify(t, s)
	require.True(t, rep.Solvable, "no constraints to violate")
	sketchtest.HasStatus(t, rep, sketch.Underconstrained)
	sketchtest.HasDOF(t, rep, 4)
	sketchtest.HasFreePoints(t, rep, a, b)
	require.Empty(t, rep.Redundant)
	require.Empty(t, rep.Conflicts)
	require.Empty(t, rep.Profiles, "an open line is not a closed profile")
	require.Nil(t, rep.Probe, "probe is opt-in")
}

func TestVerifyFullyConstrained(t *testing.T) {
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))

	sketchtest.Solve(t, s)

	rep := sketchtest.IsTrustworthy(t, s)
	require.True(t, rep.Solvable)
	sketchtest.HasDOF(t, rep, 0)
	sketchtest.HasStatus(t, rep, sketch.FullyConstrained)
	require.Empty(t, rep.Redundant)
	require.Empty(t, rep.Conflicts)
	sketchtest.HasFreePoints(t, rep)
	profile := sketchtest.SingleProfile(t, rep)
	sketchtest.IsValidProfile(t, profile)
	sketchtest.IsCurrentProfile(t, profile)
	sketchtest.HasExactCuts(t, profile)
	require.True(t, rep.ProfilesValid, "the region is a valid profile")
	require.Empty(t, rep.InvalidProfiles)
}

func TestVerifySelfIntersectingUntrustworthy(t *testing.T) {
	// A fully-constrained, solvable bowtie: structurally clean, but its boundary
	// self-intersects, so the oracle must refuse to bless it.
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
	s.CreateLine(d, a) // a-b crosses c-d

	sketchtest.Solve(t, s)
	rep := sketchtest.Verify(t, s)
	require.True(t, rep.Solvable)
	sketchtest.HasDOF(t, rep, 0)
	sketchtest.HasStatus(t, rep, sketch.FullyConstrained)
	require.False(t, rep.ProfilesValid, "the boundary self-intersects")
	require.NotEmpty(t, rep.InvalidProfiles, "the offending region is reported")
	require.False(t, rep.Trustworthy(), "a self-intersecting sketch is not trustworthy")
	sketchtest.HasOnlyReasons(t, rep, sketch.ErrInvalidProfile)
}

func TestVerifyRedundant(t *testing.T) {
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	width := sketch.NewDistance(r.A, r.B, 20)
	s.AddConstraint(width, sketch.NewDistance(r.A, r.D, 12))
	dup := sketch.NewDistance(r.A, r.B, 20) // consistent duplicate of the width dimension
	s.AddConstraint(dup)

	sketchtest.Solve(t, s)

	rep := sketchtest.Verify(t, s)
	require.True(t, rep.Solvable, "the duplicate agrees, so the sketch solves")
	sketchtest.HasStatus(t, rep, sketch.Overconstrained)
	require.Len(t, rep.Redundant, 1)
	require.Same(t, dup, rep.Redundant[0], "creation order: the later duplicate is reported")
	require.Empty(t, rep.Conflicts, "a satisfied duplicate is not a conflict")
	sketchtest.FindReasons(t, rep, sketch.ErrRedundant)
	sketchtest.HasOnlyReasons(t, rep, sketch.ErrNotFullyConstrained, sketch.ErrRedundant)
}

func TestVerifyConflictSet(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(18, 2)
	c := s.CreatePoint(17, 11)
	d := s.CreatePoint(1, 13)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	dc := s.CreateLine(d, c)
	ad := s.CreateLine(a, d)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	width := sketch.NewDistance(a, b, 20)
	s.AddConstraint(width, sketch.NewDistance(a, d, 12))

	conflict := sketch.NewDistance(a, b, 25) // fights the width-20 dimension
	s.AddConstraint(conflict)

	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged, "contradictory dimensions cannot converge")

	rep := sketchtest.Verify(t, s)
	require.False(t, rep.Solvable, "the contradiction leaves residuals")
	sketchtest.HasStatus(t, rep, sketch.Overconstrained)
	require.Empty(t, rep.Redundant)
	require.Len(t, rep.Conflicts, 1, "one conflicting constraint")
	set := sketchtest.FindConflict(t, rep, conflict)
	require.Same(t, conflict, set.Constraint, "creation order: the later dimension is blamed")
	require.Len(t, set.With, 1, "it fights exactly the width-20 dimension")
	require.Same(t, width, set.With[0], "the conflict set names the width-20 dimension")
}

func TestVerifyGroundedConflict(t *testing.T) {
	// A distance between two grounded points is violated by the geometry alone:
	// its equation touches no free variable, so it is reported with an empty
	// conflict set — there is no other constraint to fight.
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	bad := sketch.NewDistance(a, b, 25) // the points are 10 apart, not 25
	s.AddConstraint(bad)

	s.Solve(t.Context()) // cannot converge: grounded points cannot move

	rep := sketchtest.Verify(t, s)
	require.False(t, rep.Solvable)
	sketchtest.HasStatus(t, rep, sketch.Overconstrained)
	require.Len(t, rep.Conflicts, 1)
	set := sketchtest.FindConflict(t, rep, bad)
	require.Same(t, bad, set.Constraint)
	require.Empty(t, set.With, "violated by grounded geometry, no constraint to fight")
}

func TestVerifyCoincidentConflictSet(t *testing.T) {
	// A multi-row constraint (coincident contributes dx and dy) that conflicts:
	// pinning one point to two different fixed anchors. The conflict set must
	// aggregate across both rows and name the earlier coincident constraint.
	s := newSketch(t)
	p := s.CreatePoint(5, 5)
	anchorA := s.CreatePoint(0, 0)
	anchorB := s.CreatePoint(10, 10)
	s.Fix(anchorA)
	s.Fix(anchorB)
	c1 := sketch.NewCoincident(p, anchorA)
	c2 := sketch.NewCoincident(p, anchorB) // fights c1: p cannot be both anchors
	s.AddConstraint(c1, c2)

	s.Solve(t.Context())

	rep := sketchtest.Verify(t, s)
	require.False(t, rep.Solvable)
	require.Len(t, rep.Conflicts, 1)
	set := sketchtest.FindConflict(t, rep, c2)
	require.Same(t, c2, set.Constraint, "creation order: the later coincident is blamed")
	require.Contains(t, set.With, c1, "it fights the first coincident constraint")
}

func TestVerifyProbeOptIn(t *testing.T) {
	// Two unsigned distances pin a triangle apex to DOF 0, yet the mirror image
	// below the base satisfies them just as well — a configuration ambiguity the
	// DOF count cannot see.
	build := func() (*sketch.Sketch, *sketch.Point) {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10, 0)
		s.Fix(a)
		s.Fix(b)
		apex := s.CreatePoint(5, 3)
		s.AddConstraint(sketch.NewDistance(a, apex, 8), sketch.NewDistance(b, apex, 8))
		sketchtest.Solve(t, s)
		return s, apex
	}

	s, _ := build()
	rep := sketchtest.Verify(t, s)
	sketchtest.HasDOF(t, rep, 0)
	sketchtest.HasStatus(t, rep, sketch.FullyConstrained)
	require.Nil(t, rep.Probe, "the probe does not run unless requested")

	rep = sketchtest.Verify(t, s, sketch.WithProbe())
	require.NotNil(t, rep.Probe, "WithProbe runs the ambiguity probe")
	require.True(t, rep.Probe.Ambiguous(), "the mirror-image branch is found")
}

func TestVerifyDoesNotMutate(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	apex := s.CreatePoint(5, 3)
	s.AddConstraint(sketch.NewDistance(a, apex, 8), sketch.NewDistance(b, apex, 8))
	sketchtest.Solve(t, s)
	x, y := apex.X(), apex.Y()

	// Even with the probe (which re-solves from many perturbations) Verify must
	// leave the geometry exactly where it found it.
	sketchtest.Verify(t, s, sketch.WithProbe())
	require.Equal(t, x, apex.X(), "Verify must not move geometry")
	require.Equal(t, y, apex.Y())
}

// Fix 3: a DOF-0 sketch whose current config does NOT satisfy its constraints
// (here, fully constrained then perturbed without re-solving) must not report
// FullyConstrained — that would read as "valid" for an unsolved sketch.
func TestVerifyUnsolvedDOF0NotFullyConstrained(t *testing.T) {
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))
	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}
	// Knock a corner off the solution without re-solving.
	r.B.MoveTo(25, 1)

	rep := sketchtest.Verify(t, s)
	t.Logf("perturbed: solvable=%v DOF=%d status=%s redundant=%d conflicts=%d",
		rep.Solvable, rep.DOF, rep.Status, len(rep.Redundant), len(rep.Conflicts))
	require.False(t, rep.Solvable)
	require.NotEqual(t, sketch.FullyConstrained, rep.Status, "an unsolved sketch must never read as fully constrained")
}

// Fix 4: WithTolerance is honored by Verify (shared with Solve) — the same
// residual is solvable under a loose tolerance and not under the default.
func TestVerifyToleranceOption(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10.0000005, 0) // 5e-7 from the dimensioned distance
	s.Fix(a)
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	// Deliberately do NOT solve: the residual stays at 5e-7.

	require.False(t, sketchtest.Verify(t, s).Solvable, "5e-7 exceeds the default 1e-10 tolerance")
	require.True(t, sketchtest.Verify(t, s, sketch.WithTolerance(1e-6)).Solvable,
		"5e-7 is within a 1e-6 tolerance")
}
