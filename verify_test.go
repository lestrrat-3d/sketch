package sketch_test

import (
	"context"
	"math"
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

func TestVerifyWithoutConditioningKeepsStatusAndFailsCheck(t *testing.T) {
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20), sketch.NewDistance(r.A, r.D, 12))
	sketchtest.Solve(t, s)

	full := s.Verify(t.Context())
	rep := s.Verify(t.Context(), sketch.WithoutConditioning())

	require.Equal(t, full.Status, rep.Status)
	require.Equal(t, full.DOF, rep.DOF)
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.True(t, math.IsNaN(rep.Conditioning))
	require.ErrorIs(t, rep.Check(), sketch.ErrConditioningUnmeasured)
	require.NoError(t, full.Check())
}

func TestVerifyFullRankStillReportsRedundancy(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.CreateLine(a, b)
	s.AddConstraint(sketch.NewDistance(a, b, 10), sketch.NewHorizontalPoints(a, b),
		sketch.NewHorizontalPoints(a, b))
	sketchtest.Solve(t, s)

	rep := s.Verify(t.Context(), sketch.WithoutConditioning())

	require.Equal(t, sketch.Overconstrained, rep.Status)
	require.NotEmpty(t, rep.Redundant)
	require.Empty(t, rep.Conflicts)
}

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

// horizontalBarSketch is fully constrained (DOF 0): a grounded origin and a
// second point placed 10 to the right by a distance + horizontal pair. The two
// solutions (±10) make it a legitimate target for the ambiguity probe.
func horizontalBarSketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(7, 3)
	ab := s.CreateLine(a, b)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.AddConstraint(sketch.NewHorizontal(ab))
	return s
}

func TestProbeConfigurationsContext(t *testing.T) {
	s := horizontalBarSketch(t)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	t.Run("already-cancelled context aborts the probe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := s.ProbeConfigurations(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("live context probes normally", func(t *testing.T) {
		pr, err := s.ProbeConfigurations(t.Context())
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(pr.Configurations), 1)
	})

	t.Run("cancellation after the baseline solve reports the context error", func(t *testing.T) {
		// An under-constrained sketch (DOF 1: one free point pinned only by a
		// distance to the grounded origin, authored already satisfying it) makes
		// the probe's post-baseline precondition return ErrUnderconstrained — a
		// non-context verdict. A context that goes done only once every earlier
		// poll has passed exercises the guard added after the baseline rank/DOF
		// pass: it must surface the cancellation, not the ErrUnderconstrained
		// precondition. Without the guard the probe never polls ctx again after
		// the rank pass, so it returns ErrUnderconstrained and the trip never fires.
		u := newSketch(t)
		a := u.CreatePoint(0, 0)
		b := u.CreatePoint(10, 0) // already 10 from a, so lm converges at iteration 0
		a.MoveTo(0, 0)
		u.Fix(a)
		u.AddConstraint(sketch.NewDistance(a, b, 10))
		fc := &tripContext{tripAfterN: probeBaselineRankTripCount}

		_, err := u.ProbeConfigurations(fc)
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, sketch.ErrUnderconstrained)
		require.True(t, fc.tripped, "context never reached the post-baseline-rank guard")
	})
}

// probeBaselineRankTripCount is the number of live ctx.Err() polls the probe of
// an already-converged, DOF-1 sketch makes before the guard that follows the
// baseline rank/DOF pass: the entry check, lm's entry + first-iteration checks,
// and the post-lm check are the first four; the post-rank guard is the fifth. If
// the probe's pre-DOF poll cadence changes this test fails loudly (the guard is
// never reached and ErrUnderconstrained leaks through), which is the intended
// tripwire.
const probeBaselineRankTripCount = 5

func TestVerifyContext(t *testing.T) {
	s := horizontalBarSketch(t)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	t.Run("cancelled context discards the probe but still reports", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rep := s.Verify(ctx, sketch.WithProbe())
		// Verify has no error result: it still returns a populated report, but
		// the requested probe could not run, so it is marked incomplete and must
		// not be blessed as trustworthy (the ambiguity check never happened).
		require.Nil(t, rep.Probe)
		require.True(t, rep.ProbeIncomplete)
		require.False(t, rep.Trustworthy())
		require.Equal(t, 0, rep.DOF)
		require.True(t, rep.Solvable)
	})

	t.Run("cancelled context without WithProbe is not marked incomplete", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rep := s.Verify(ctx)
		// No probe was requested, so cancellation does not make the report
		// incomplete on the probe axis.
		require.False(t, rep.ProbeIncomplete)
	})

	t.Run("live context runs the probe", func(t *testing.T) {
		rep := s.Verify(t.Context(), sketch.WithProbe())
		require.NotNil(t, rep.Probe)
	})
}
