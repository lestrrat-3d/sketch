package sketch_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// dimensionedRect builds a solved, fully constrained 20x12 rectangle grounded at
// its A corner, and returns the DRIVING width dimension plus a DRIVEN one
// measuring the same span. Every COORDINATE stays finite, which is what makes it
// the right fixture for this family: a NaN placed in either dimension's target
// leaves the geometry perfectly renderable (the exporters' own bbox refusal never
// fires) and, for the driven one, leaves every residual row finite too — so a
// site tested only with a NaN coordinate looks clean twice over, once because the
// poison is loud everywhere and once because the render refuses before the
// overlay runs.
func dimensionedRect(t *testing.T) (*sketch.Sketch, *sketch.Distance, *sketch.Distance) {
	t.Helper()
	s := newSketch(t)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.Fix(r.A)
	width := sketch.NewDistance(r.A, r.B, 20)
	s.AddConstraint(width, sketch.NewDistance(r.A, r.D, 12))
	meas := sketch.NewDistance(r.A, r.B, 0)
	meas.SetDriven(true)
	s.AddConstraint(meas)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF(), "the fixture is fully constrained before any NaN")
	return s, width, meas
}

// TestSolveResultRefusesNonFiniteRankPass covers G1: [Sketch.Solve] filled
// Result.DOF and Result.Redundant from the same unscreened rank pass
// [Sketch.DOF] was corrected for, so a poisoned sketch and a finite one produced
// a byte-identical Result and a caller gating on `res.DOF == 0 && res.Converged`
// was falsely blessed. Result.DOF already documents -1 as its own not-computed
// sentinel, so the refusal needed no signature change.
//
// Both branches are covered because they reach the screen differently: with
// constraint rows it rides in on the rank pass's second return, and with none it
// has to be asked directly — and the no-rows branch is the confirmed false bless,
// since the free-variable count it reports collapses to 0 on an all-grounded
// sketch.
func TestSolveResultRefusesNonFiniteRankPass(t *testing.T) {
	t.Run("no constraint rows", func(t *testing.T) {
		s, _ := allFixedNaNStraySketch(t, true)

		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.True(t, res.Converged, "no rows to violate, so the solve still converges")
		require.Equal(t, -1, res.DOF, "the not-computed sentinel, never a 0 that reads fully constrained")
		require.Equal(t, -1, res.Redundant)
		require.Equal(t, 8, s.DOF(), "and the bare read still answers with maximum ignorance")
	})

	t.Run("no constraint rows, finite control", func(t *testing.T) {
		s, _ := allFixedNaNStraySketch(t, false)

		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res.DOF, "the identical all-grounded fixture still reports a real DOF")
		require.Equal(t, 0, res.Redundant)
	})

	t.Run("dimension target", func(t *testing.T) {
		s, width, _ := dimensionedRect(t)
		width.Set(math.NaN())

		res, _ := s.Solve(t.Context())
		require.Equal(t, -1, res.DOF)
		require.Equal(t, -1, res.Redundant)
	})

	t.Run("dimension target, finite control", func(t *testing.T) {
		s, _, _ := dimensionedRect(t)

		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res.DOF, "the same rectangle with a finite target is genuinely DOF 0")
		require.Equal(t, 0, res.Redundant)
	})
}

// TestDOFColoringRefusesNonFiniteAnalysis covers G2: the DOF-colouring overlay
// read the null-space support unscreened, so on a finite rectangle whose
// committed DRIVING dimension target is NaN the render SUCCEEDED and painted
// "fully constrained" black on geometry [Sketch.FreePoints] reports free and
// [Point.IsFullyConstrained] reports not-fully-constrained. That mixed picture is
// worse than a uniformly free one: it reads as a specific credible diagnosis with
// no cue to distrust it.
//
// Everything is marked free instead of inventing a fourth colour — see the note
// in computeOverlaySets — so the assertion is that no mark claims "constrained"
// and that the drawing agrees with the three API reads on the same geometry.
func TestDOFColoringRefusesNonFiniteAnalysis(t *testing.T) {
	s, width, _ := dimensionedRect(t)
	width.Set(math.NaN())

	out, err := s.SVG(sketch.WithDOFColoring(true))
	require.NoError(t, err, "every coordinate is finite, so the render really does reach the overlay")
	require.NotContains(t, out, "#202124",
		"nothing may read fully constrained when nothing was analysed")
	require.NotContains(t, out, "#188038",
		"the grounded-anchor green is not a verdict either; free wins over it")
	require.Contains(t, out, "#1a73e8", "every point and entity reads free")

	// The agreement the screen exists to restore: the render and the API say the
	// same thing about the same geometry.
	require.Equal(t, s.Points(), s.FreePoints())
	for _, p := range s.Points() {
		require.False(t, p.IsFullyConstrained(), "point %d", p.ID())
	}
	for _, e := range s.Entities() {
		require.False(t, s.EntityIsFullyConstrained(e))
	}
}

// TestDOFColoringFiniteControlStillPaintsConstrained is the finite control for
// TestDOFColoringRefusesNonFiniteAnalysis: the identical rectangle with an
// ordinary target still paints the constrained colours, so the refusal above is
// the NaN and not a blanket "everything free".
func TestDOFColoringFiniteControlStillPaintsConstrained(t *testing.T) {
	s, _, _ := dimensionedRect(t)

	out, err := s.SVG(sketch.WithDOFColoring(true))
	require.NoError(t, err)
	require.Contains(t, out, "#202124", "a genuinely fully-constrained rectangle reads constrained")
	require.NotContains(t, out, "#1a73e8", "and nothing in it reads free")
}

// TestDrivenDimensionNonFiniteTargetIsReported covers G3, the sharpest of the
// five: the scan excluded DRIVEN dimensions, so a NaN in a driven target was a
// full bless — Verify reported Trustworthy, fully constrained, DOF 0, nothing
// non-finite — while [Sketch.MarshalJSON] failed and the annotated SVG refused.
// The serialization invariant is that a sketch the report calls trustworthy
// always marshals, so this test asserts the report AND both writers together: a
// fix that only silenced one of them would leave the invariant broken the other
// way round.
//
// It is also PERMANENT: refreshDriven recomputes the measurement as
// d.base()+r[0], both non-finite, so no solve repairs it.
func TestDrivenDimensionNonFiniteTargetIsReported(t *testing.T) {
	s, _, meas := dimensionedRect(t)
	meas.Set(math.NaN())

	_, err := s.Solve(t.Context())
	require.NoError(t, err, "the driven target contributes no residual row, so the solve is untouched")

	rep := s.Verify(t.Context())
	require.Equal(t, []sketch.Dimension{sketch.Dimension(meas)}, rep.NonFiniteDimensions,
		"the driven dimension is named, and only it")
	require.False(t, rep.Trustworthy(), "a sketch no writer can write must not be blessed")
	require.ErrorIs(t, rep.Check(), sketch.ErrNonFiniteGeometry)
	require.False(t, rep.Analysed())

	// The invariant the bless broke: the report and the writers must agree.
	_, err = json.Marshal(s)
	require.Error(t, err, "encoding/json rejects NaN, so an untrustworthy verdict is the honest one")
	_, err = s.SVG(sketch.WithDimensions(true))
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)

	// Permanent: another solve does not repair it.
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.True(t, math.IsNaN(meas.Target().Mag()), "refreshDriven recomputes NaN from NaN")
	require.False(t, s.Verify(t.Context()).Trustworthy())
}

// TestDrivenCandidateStillSkipsNonFiniteScreen pins the asymmetry G3 must
// preserve. Scanning a COMMITTED driven target and exempting a driven CANDIDATE
// are different questions: the committed one is a value this sketch holds and no
// writer can write, while the candidate contributes no equation, is never ranked,
// and has no verdict for a poisoned pivot to corrupt.
func TestDrivenCandidateStillSkipsNonFiniteScreen(t *testing.T) {
	s, a, b := nonFiniteCheckConstraintFixture(t, true)

	driven := sketch.NewDistance(a, b, 20)
	driven.SetDriven(true)
	require.NoError(t, s.CheckConstraint(driven),
		"a driven candidate is never ranked, so the poisoned rank pass cannot misjudge it")
}

// TestSkippedVerifyStillReportsStaleness covers G4: the early-out discarded the
// staleness scan, whose ZERO VALUE is the blessing one — Stale=false reads as
// "verified against a current 3D snapshot". The scan reads boolean provenance
// flags and cannot be poisoned by a non-finite value or confused by a foreign
// handle, so skipping it bought nothing and lost a real finding. Both skip causes
// are covered because the loss was pre-existing on the foreign-handle path too.
func TestSkippedVerifyStillReportsStaleness(t *testing.T) {
	t.Run("finite control", func(t *testing.T) {
		s := newSketch(t)
		s.CreateReferencePoint(5, 5, "edge")
		s.MarkStale("edge")

		rep := s.Verify(t.Context())
		require.True(t, rep.Stale)
		require.Len(t, rep.StaleReferencePoints, 1)
		require.ErrorIs(t, rep.Check(), sketch.ErrStaleReference)
	})

	t.Run("non-finite geometry", func(t *testing.T) {
		s := newSketch(t)
		ref := s.CreateReferencePoint(5, 5, "edge")
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10, 0)
		s.CreateLine(a, b)
		nan := sketch.NewDistance(a, b, 10)
		s.AddConstraint(nan)
		nan.Set(math.NaN())
		s.MarkStale("edge")

		rep := s.Verify(t.Context())
		require.False(t, rep.Analysed(), "the analysis really was skipped")
		require.True(t, rep.Stale, "staleness survives the skip it never depended on")
		require.Equal(t, []*sketch.Point{ref}, rep.StaleReferencePoints)

		err := rep.Check()
		require.ErrorIs(t, err, sketch.ErrStaleReference)
		require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
		require.ErrorIs(t, err, sketch.ErrVerificationIncomplete)
	})

	t.Run("foreign handle", func(t *testing.T) {
		s := newSketch(t)
		ref := s.CreateReferencePoint(5, 5, "edge")
		other := newSketch(t)
		s.AddConstraint(sketch.NewCoincident(s.CreatePoint(0, 0), other.CreatePoint(1, 1)))
		s.MarkStale("edge")

		rep := s.Verify(t.Context())
		require.False(t, rep.Analysed())
		require.True(t, rep.Stale)
		require.Equal(t, []*sketch.Point{ref}, rep.StaleReferencePoints)

		err := rep.Check()
		require.ErrorIs(t, err, sketch.ErrStaleReference)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
	})

	t.Run("nil topology still skips it", func(t *testing.T) {
		// The one cause the scan must still sit behind: an entity's staleness is
		// derived from its defining points, so a nil point panics it.
		s := newSketch(t)
		p1 := s.CreateReferencePoint(0, 0, "a")
		p2 := s.CreateReferencePoint(10, 0, "b")
		l, err := s.CreateReferenceLine(p1, p2, "edge")
		require.NoError(t, err)
		l.Start = nil
		s.MarkStale("a")

		require.NotPanics(t, func() {
			rep := s.Verify(t.Context())
			require.False(t, rep.Analysed())
		})
	})
}

// TestProbeRefusesNonFiniteGeometry covers G5: the probe computed its DOF-0
// precondition from the same poisoned rank pass and proceeded, returning output
// identical to the finite sketch's. It HAS an error return, so it refuses the way
// [Sketch.CheckConstraint] does — which also makes the direct call agree with
// [Sketch.Verify] (WithProbe), which already refuses to run it on this state.
func TestProbeRefusesNonFiniteGeometry(t *testing.T) {
	t.Run("dimension target", func(t *testing.T) {
		s, width, _ := dimensionedRect(t)
		width.Set(math.NaN())

		_, err := s.ProbeConfigurations(t.Context())
		require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)

		// The aggregate call already behaved; the two now agree.
		rep := s.Verify(t.Context(), sketch.WithProbe())
		require.Nil(t, rep.Probe)
	})

	t.Run("no constraint rows", func(t *testing.T) {
		s, _ := allFixedNaNStraySketch(t, true)

		_, err := s.ProbeConfigurations(t.Context())
		require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	})

	t.Run("finite control", func(t *testing.T) {
		s, _, _ := dimensionedRect(t)

		res, err := s.ProbeConfigurations(t.Context())
		require.NoError(t, err, "an ordinary DOF-0 rectangle is still probed")
		require.NotEmpty(t, res.Configurations)
	})
}

// TestReportAnalysedNamesTheSkippedPath pins the exported accessor. The skip was
// unexported, so no consumer outside the package could do what the status badge
// does internally — and every field the skipped passes never wrote holds a zero
// value that is ALSO a legitimate analysed reading, so the report is not
// self-describing without it.
func TestReportAnalysedNamesTheSkippedPath(t *testing.T) {
	s, width, _ := dimensionedRect(t)

	rep := s.Verify(t.Context())
	require.True(t, rep.Analysed())
	require.True(t, rep.Trustworthy())

	width.Set(math.NaN())
	rep = s.Verify(t.Context())
	require.False(t, rep.Analysed())
	require.ErrorIs(t, rep.Check(), sketch.ErrVerificationIncomplete)
	// The trap the accessor exists for: every one of these is indistinguishable
	// from an analysed reading.
	require.Equal(t, 0, rep.DOF)
	require.Equal(t, sketch.Underconstrained, rep.Status)
	require.False(t, rep.Solvable)
	require.Zero(t, rep.Conditioning)

	// Analysed is exactly "no ErrVerificationIncomplete among the reasons".
	incomplete := errors.Is(rep.Check(), sketch.ErrVerificationIncomplete)
	require.Equal(t, rep.Analysed(), !incomplete)
}

// TestDependencyPartitionRefusesNonFiniteGeometry pins the screen at
// conflictAnalysis, the shared dependency pass [Sketch.Diagnose],
// [Sketch.RedundantConstraints] and [Sketch.Verify] all read their
// redundant/conflicting verdict from. It was the one primitive of this family
// still screened by its callers rather than by its own return, so a new reader
// of it compiled while publishing a verdict computed from a poisoned Jacobian.
//
// The fixture carries a GENUINE redundancy — a duplicate width dimension the
// finite control reports as Redundant — so the poisoned run cannot pass by
// having nothing to find. Unscreened, both tests the Gram-Schmidt projection
// makes ("scale < eps" and "rest <= eps*scale") are false against a NaN row
// norm, so every row is accepted as independent, the duplicate stops being
// reported, and both reads answer "no constraint problem found" for geometry
// nothing analysed.
func TestDependencyPartitionRefusesNonFiniteGeometry(t *testing.T) {
	t.Run("finite control", func(t *testing.T) {
		s, a, b := nonFiniteCheckConstraintFixture(t, false)
		dup := sketch.NewDistance(a, b, 20)
		s.AddConstraint(dup)
		_, err := s.Solve(t.Context())
		require.NoError(t, err)

		d := s.Diagnose()
		require.Equal(t, []sketch.Constraint{dup}, d.Redundant,
			"the duplicate dimension is a consistent duplicate, so the real analysis names it")
		require.Empty(t, d.Conflicting)
		require.Equal(t, []sketch.Constraint{dup}, s.RedundantConstraints())
	})

	t.Run("non-finite", func(t *testing.T) {
		s, a, b := nonFiniteCheckConstraintFixture(t, true)
		dup := sketch.NewDistance(a, b, 20)
		s.AddConstraint(dup)

		cons := s.Constraints()
		require.Contains(t, cons, sketch.Constraint(dup))
		d := s.Diagnose()
		require.Equal(t, cons, d.Conflicting,
			"no constraint here has been proven consistent, the duplicate included")
		require.Empty(t, d.Redundant,
			"the real redundancy must not be republished as a finding nothing analysed")
		require.Equal(t, cons, s.RedundantConstraints())

		rep := s.Verify(t.Context())
		require.False(t, rep.Analysed())
		require.Empty(t, rep.Redundant, "the report records the skip instead of a partition")
		require.Empty(t, rep.Conflicts)
	})
}

// arcLengthAuxNaNSketch reproduces the confirmed gap: an EXTREME-BUT-FINITE
// arc (center (0,0), start (1e200,0), end (0,1e200), every point fixed) whose
// [sketch.ArcLength] target of 0 drives the solver to push the constraint's own
// unwrapped-sweep aux variable, theta, to NaN — not a poisoned authored
// coordinate, entity shape variable or dimension target, but a variable the
// AUX-VAR-OWNING CONSTRAINT itself allocates and the solver moves like any
// other free unknown. Every one of the three sources nonFiniteVars scanned
// before this fix stays perfectly finite throughout, so the pre-fix screen
// reported nothing wrong while DOF()/Verify kept reading a rank computed from
// a NaN pivot.
func arcLengthAuxNaNSketch(t *testing.T) (*sketch.Sketch, *sketch.ArcLength) {
	t.Helper()
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	start := s.CreatePoint(1e200, 0)
	end := s.CreatePoint(0, 1e200)
	s.Fix(center)
	s.Fix(start)
	s.Fix(end)
	arc := s.CreateArc(center, start, end)
	al := sketch.NewArcLength(arc, 0)
	s.AddConstraint(al)

	// The target is unreachable at this radius (driving theta toward 0 fights
	// the sweep-pinning row at r=1e200), so the solve does not converge —
	// that is expected and irrelevant here; what matters is the state theta is
	// left in.
	_, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	return s, al
}

// TestArcLengthAuxVarNonFiniteStopsVerify covers FINDING 1: nonFiniteVars/
// hasNonFiniteVars never walked a live auxiliary solver variable, so this
// fixture's poisoned theta was invisible to the screen even though every
// authored point stays exactly as given. Verify must stop before analysing,
// naming the offending ArcLength constraint, exactly as it already does for a
// poisoned point/entity/dimension target.
func TestArcLengthAuxVarNonFiniteStopsVerify(t *testing.T) {
	s, al := arcLengthAuxNaNSketch(t)

	rep := s.Verify(t.Context())
	require.False(t, rep.Analysed(), "the poisoned aux variable must stop analysis, not silently pass")
	require.Equal(t, []sketch.Constraint{sketch.Constraint(al)}, rep.NonFiniteConstraints,
		"the offending constraint is named, and only it")
	require.Empty(t, rep.NonFinitePoints, "every authored point stays exactly as given")
	require.Empty(t, rep.NonFiniteEntities)
	require.Empty(t, rep.NonFiniteDimensions)

	err := rep.Check()
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.ErrorIs(t, err, sketch.ErrVerificationIncomplete)
	require.False(t, rep.Trustworthy())
}

// TestArcLengthAuxVarNonFiniteRefusesRankDerivedConsumers pins the same
// fixture against every rank-derived bare read and refusal: each must answer
// with its own documented non-blessing shape rather than a number computed
// from the NaN theta pivot — Result.DOF's -1 sentinel (not the flipped-DOF
// false read the finding reproduced), DOF()'s maximum-ignorance total variable
// count, FreePoints() naming every point, CheckConstraint's refusal, and
// ProbeConfigurations' refusal.
func TestArcLengthAuxVarNonFiniteRefusesRankDerivedConsumers(t *testing.T) {
	s, _ := arcLengthAuxNaNSketch(t)

	res, err := s.Solve(t.Context())
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.Equal(t, -1, res.DOF, "the not-computed sentinel, never a flipped DOF")
	require.Equal(t, -1, res.Redundant)

	// Maximum ignorance: every point's x/y, the origin's, and ArcLength's own
	// theta, as if neither a constraint nor a Fix had ever been applied.
	wantVars := 2*len(s.Points()) + 2 /* origin */ + 1 /* ArcLength's theta */
	require.Equal(t, wantVars, s.DOF())

	require.Equal(t, s.Points(), s.FreePoints(),
		"every point reads free, the maximum-ignorance answer")

	require.ErrorIs(t, s.CheckConstraint(sketch.NewHorizontal(s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(1, 0)))),
		sketch.ErrNonFiniteGeometry)

	_, err = s.ProbeConfigurations(t.Context())
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
}
