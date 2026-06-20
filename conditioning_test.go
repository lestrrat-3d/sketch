package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// orthogonalPinned builds a fully-constrained healthy sketch (a point pinned by
// orthogonal H/V distances) at geometric scale k, translated by (tx,ty). The
// constraint system is identical up to a similarity, so its scale-invariant
// Conditioning must be identical while the raw RankMargin need not be.
func orthogonalPinned(k, tx, ty float64) *sketch.VerificationReport {
	s := sketch.New()
	a := s.AddPoint(tx, ty)
	b := s.AddPoint(tx+3*k, ty+1*k)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontalDistance(a, b, 10*k))
	s.AddConstraint(sketch.NewVerticalDistance(a, b, 5*k))
	s.Solve()
	return s.Verify()
}

// mixedUnitFixture is Codex's mixed-row counterexample: a free point determined by
// a LENGTH distance row and a DIMENSIONLESS angle row (plus a coincidence). The raw
// Jacobian mixes row units, so RankMargin moves with scale; the nondimensional
// Conditioning must not. Scaled by k (lengths scale, the 30° angle does not).
func mixedUnitFixture(k float64) *sketch.VerificationReport {
	s := sketch.New()
	o := s.AddPoint(0, 0)
	e := s.AddPoint(10*k, 0)
	s.Fix(o)
	s.Fix(e)
	l1 := s.AddLine(o, e) // fixed reference line (the x-axis)
	p2 := s.AddPoint(0, 0)
	p3 := s.AddPoint(4*k, 2*k)
	l2 := s.AddLine(p2, p3)
	s.AddConstraint(sketch.NewCoincident(p2, o))     // 2 length rows
	s.AddConstraint(sketch.NewDistance(p2, p3, 5*k)) // 1 length row
	s.AddConstraint(sketch.NewAngle(l1, l2, 30))     // 1 dimensionless row
	s.Solve()
	return s.Verify()
}

func TestConditioningScaleInvariantHealthy(t *testing.T) {
	base := orthogonalPinned(1, 0, 0)
	require.Equal(t, 0, base.DOF)
	require.True(t, base.Trustworthy())
	require.Greater(t, base.Conditioning, 1e-6, "an orthogonal pin is well-conditioned")
	require.Less(t, base.Conditioning, math.Inf(1))

	// Same construction at 1000×, at inch-equivalent 25.4×, and translated far from
	// the origin: Conditioning is invariant to the digit.
	for _, v := range []struct {
		name string
		rep  *sketch.VerificationReport
	}{
		{"1000x", orthogonalPinned(1000, 0, 0)},
		{"25.4x (inch)", orthogonalPinned(25.4, 0, 0)},
		{"translated", orthogonalPinned(1, 1e5, -2e5)},
		{"big+translated", orthogonalPinned(1000, 1e6, 1e6)},
	} {
		require.InEpsilonf(t, base.Conditioning, v.rep.Conditioning, 1e-9,
			"%s: Conditioning is scale/translation invariant", v.name)
		require.Truef(t, v.rep.Trustworthy(), "%s still trustworthy", v.name)
	}
}

func TestRankAndConditioningScaleInvariant(t *testing.T) {
	// On a mixed length/dimensionless system, BOTH the structural RankMargin and the
	// Conditioning are now scale-invariant: the rank/DOF analysis runs on the same
	// nondimensional Jacobian as the conditioning gate, so the whole rank story no
	// longer moves with the geometry's size. (RankMargin still does not gate —
	// Conditioning is the designated trust gate; they measure different things.)
	a := mixedUnitFixture(1)
	b := mixedUnitFixture(1000)
	require.Equal(t, 0, a.DOF)
	require.Equal(t, 0, b.DOF)
	require.True(t, a.Trustworthy())
	require.True(t, b.Trustworthy())

	require.InEpsilon(t, a.Conditioning, b.Conditioning, 1e-9,
		"Conditioning is invariant across a 1000× rescale of the mixed-unit system")
	require.InEpsilon(t, a.RankMargin, b.RankMargin, 1e-6,
		"RankMargin is now scale-invariant too (computed on the nondimensional Jacobian)")
}

func TestConditioningNearSingularScaleInvariant(t *testing.T) {
	// A point pinned by two lines δ radians apart, at scale k. Conditioning ≈ δ/2,
	// independent of k; it crosses the 1e-6 gate at a scale-invariant δ.
	mk := func(delta, k float64) *sketch.VerificationReport {
		s := sketch.New()
		o1 := s.AddPoint(0, 0)
		e1 := s.AddPoint(1*k, 0)
		o2 := s.AddPoint(0, 0)
		e2 := s.AddPoint(1*k, delta*k)
		for _, p := range []*sketch.Point{o1, e1, o2, e2} {
			s.Fix(p)
		}
		l1 := s.AddLine(o1, e1)
		l2 := s.AddLine(o2, e2)
		p := s.AddPoint(0, 0)
		s.AddConstraint(sketch.NewPointOnLine(p, l1))
		s.AddConstraint(sketch.NewPointOnLine(p, l2))
		s.Solve()
		return s.Verify()
	}
	// Healthy separation passes and is scale-invariant.
	h1, hk := mk(1e-2, 1), mk(1e-2, 1000)
	require.True(t, h1.Trustworthy())
	require.InEpsilon(t, h1.Conditioning, hk.Conditioning, 1e-9, "scale-invariant")

	// Fragile separation fails the gate at every scale.
	f1, fk := mk(1e-7, 1), mk(1e-7, 1000)
	require.Equal(t, 0, f1.DOF, "structurally fully constrained")
	require.False(t, f1.Trustworthy(), "near-singular: gated out")
	require.False(t, fk.Trustworthy(), "and at 1000× too")
	require.InEpsilon(t, f1.Conditioning, fk.Conditioning, 1e-9, "fragility is scale-invariant")
}

func TestConditioningHealthyFixtures(t *testing.T) {
	// Real well-posed sketches sit comfortably above the threshold at any scale.
	for _, k := range []float64{1, 1000} {
		// A hexagon-ish pin: a point fixed by distance + perpendicular offset lines.
		s := sketch.New()
		a := s.AddPoint(0, 0)
		s.Fix(a)
		b := s.AddPoint(5*k, 0)
		c := s.AddPoint(5*k, 5*k)
		s.AddConstraint(sketch.NewDistance(a, b, 5*k))
		s.AddConstraint(sketch.NewHorizontalPoints(a, b))
		s.AddConstraint(sketch.NewDistance(b, c, 5*k))
		s.AddConstraint(sketch.NewVerticalPoints(b, c))
		s.Solve()
		rep := s.Verify()
		require.Equal(t, 0, rep.DOF)
		require.True(t, rep.Trustworthy(), "healthy fixture trustworthy at k=%v", k)
		require.Greater(t, rep.Conditioning, 1e-3, "comfortably above the gate at k=%v", k)
	}
}

func TestConditioningNotApplicableUnderconstrained(t *testing.T) {
	// An under-constrained sketch is genuinely singular by its free DOF; the
	// conditioning measure is left +Inf (not applicable) rather than a misleading 0.
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(3, 4)
	s.Fix(a)
	s.AddConstraint(sketch.NewDistance(a, b, 5)) // b still free on a circle
	s.Solve()
	rep := s.Verify()
	require.Greater(t, rep.DOF, 0)
	require.Equal(t, math.Inf(1), rep.Conditioning, "not applicable when under-constrained")
	require.False(t, rep.Trustworthy(), "under-constrained is untrustworthy on its own")
}

// TestConditioningClassifiesAuxConstraints exercises the aux/mixed constraints
// whose residual() row count is state-dependent (a sweep/box slack or unwrapped-
// sweep row appears only once allocVars has run). Each builds a fully-constrained
// sketch with at least one free variable, so conditioningMatrix runs and its
// row-kind table must align with residuals(): a misclassified row count yields the
// NaN classification-gap sentinel, which this test forbids. A finite Conditioning
// proves the rows were classified.
func TestConditioningClassifiesAuxConstraints(t *testing.T) {
	cases := []struct {
		name  string
		build func(s *sketch.Sketch)
	}{
		{"pointOnArc", func(s *sketch.Sketch) {
			o, a, b := s.AddPoint(0, 0), s.AddPoint(5, 0), s.AddPoint(0, 5)
			s.Fix(o)
			s.Fix(a)
			s.Fix(b)
			arc := s.AddArc(o, a, b)
			p := s.AddPoint(4, 3)
			line := s.AddLine(o, s.AddPoint(5, 5)) // y=x through center
			s.Fix(line.End)
			s.AddConstraint(sketch.NewPointOnArc(p, arc))
			s.AddConstraint(sketch.NewPointOnLine(p, line))
		}},
		{"distancePointArc", func(s *sketch.Sketch) {
			o, a, b := s.AddPoint(0, 0), s.AddPoint(5, 0), s.AddPoint(0, 5)
			s.Fix(o)
			s.Fix(a)
			s.Fix(b)
			arc := s.AddArc(o, a, b)
			p := s.AddPoint(6, 6) // free; radial distance + a diagonal line pin it
			diag := s.AddPoint(7, 7)
			s.Fix(diag)
			s.AddConstraint(sketch.NewDistancePointArc(p, arc, 2))
			s.AddConstraint(sketch.NewPointOnLine(p, s.AddLine(o, diag)))
		}},
		{"arcLength", func(s *sketch.Sketch) {
			o, a := s.AddPoint(0, 0), s.AddPoint(4, 0)
			s.Fix(o)
			s.Fix(a)
			end := s.AddPoint(0, 4)
			arc := s.AddArc(o, a, end)
			s.AddConstraint(sketch.NewArcLength(arc, 2*math.Pi))
			s.AddConstraint(sketch.NewDistance(o, end, 4))
			s.AddConstraint(sketch.NewVerticalPoints(o, end))
		}},
	}
	for _, c := range cases {
		s := sketch.New()
		c.build(s)
		_, err := s.Solve()
		require.NoErrorf(t, err, "%s solves", c.name)
		rep := s.Verify()
		require.Falsef(t, math.IsNaN(rep.Conditioning),
			"%s: Conditioning is not the NaN classification-gap sentinel (rows aligned)", c.name)
		if rep.DOF == 0 {
			require.Greaterf(t, rep.Conditioning, 0.0, "%s: a real conditioning value", c.name)
		}
	}
}

// TestConditioningGateToleranceDerived protects the slack-flat-spot fix: the trust
// threshold is max(1e-6, 4·√tolerance), not a constant. A slack-encoded inequality
// at its active boundary only resolves the slack to ≈√tolerance, so the gate must
// rise with √tolerance or a near-singular system slips through at a loose
// tolerance. A near-parallel pin with a fixed, tolerance-independent Conditioning
// of 1e-4 stays trustworthy at a tight tolerance (low gate) and is correctly gated
// out at a loose one (high gate) — a constant gate could not do both.
func TestConditioningGateToleranceDerived(t *testing.T) {
	mk := func(tol float64) *sketch.VerificationReport {
		s := sketch.New()
		o1, e1 := s.AddPoint(0, 0), s.AddPoint(1, 0)
		o2, e2 := s.AddPoint(0, 0), s.AddPoint(1, 2e-4) // ≈2e-4 rad apart → Conditioning ≈ 1e-4
		for _, p := range []*sketch.Point{o1, e1, o2, e2} {
			s.Fix(p)
		}
		p := s.AddPoint(0, 0)
		s.AddConstraint(sketch.NewPointOnLine(p, s.AddLine(o1, e1)))
		s.AddConstraint(sketch.NewPointOnLine(p, s.AddLine(o2, e2)))
		s.Solve(sketch.WithTolerance(tol))
		return s.Verify(sketch.WithTolerance(tol))
	}
	tight := mk(1e-12) // gate 4e-6 < 1e-4
	loose := mk(1e-8)  // gate 4e-4 > 1e-4
	require.Empty(t, tight.Redundant, "structurally clean (slack-free near-parallel)")
	require.InEpsilon(t, 1e-4, tight.Conditioning, 1e-2, "Conditioning is tolerance-independent here")
	require.InEpsilon(t, 1e-4, loose.Conditioning, 1e-2)
	require.True(t, tight.Trustworthy(), "above the tight-tolerance gate")
	require.False(t, loose.Trustworthy(), "the loose-tolerance gate (4·√tol) refuses it")
}

// TestConditioningSlackFlatSpotGated is the boundary-slack regression for Codex's
// HIGH finding: a point confined to an arc with the contact driven to the sweep
// boundary, where the slack variable w → 0 (it only resolves to ≈√tolerance) and
// its column 2w bounds σ_min. The fixture is STRUCTURALLY CLEAN — the arc end is
// free (pinned by verticalPoints + the internal arcRadius, so no constraint is
// redundant) — so the ONLY thing that can refuse it is the conditioning gate. At
// the default tolerance its Conditioning lands in [1e-6, 4e-5): above the old
// constant 1e-6 gate (which would have FALSELY blessed it) and below the
// tolerance-derived 4·√tol gate (which correctly refuses it). A looser tolerance
// raises both the slack residue and the gate, so it stays gated.
func TestConditioningSlackFlatSpotGated(t *testing.T) {
	build := func() *sketch.Sketch {
		s := sketch.New()
		o, a := s.AddPoint(0, 0), s.AddPoint(5, 0)
		s.Fix(o)
		s.Fix(a)
		b := s.AddPoint(0, 5) // free arc end: verticalPoints + arcRadius pin it to (0,5)
		arc := s.AddArc(o, a, b)
		s.AddConstraint(sketch.NewVerticalPoints(o, b))
		xend := s.AddPoint(10, 0)
		s.Fix(xend)
		xaxis := s.AddLine(o, xend) // y=0 meets the arc at the (5,0) sweep boundary
		p := s.AddPoint(4.9, 0.3)
		s.AddConstraint(sketch.NewPointOnArc(p, arc))
		s.AddConstraint(sketch.NewPointOnLine(p, xaxis))
		return s
	}

	// Default tolerance: structurally clean, but the boundary slack is near-singular
	// and gated. Conditioning sits in the window the old constant gate missed.
	def := build()
	def.Solve()
	rep := def.Verify()
	require.True(t, rep.Solvable)
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.Empty(t, rep.Redundant, "the arc end is genuinely constrained, nothing redundant")
	require.Empty(t, rep.Conflicts)
	require.Greater(t, rep.Conditioning, 1e-6, "above the old constant gate that falsely blessed it")
	require.Less(t, rep.Conditioning, 4e-5, "below the tolerance-derived gate that now refuses it")
	require.False(t, rep.Trustworthy(), "a boundary slack flat-spot must not be trustworthy")

	// A looser tolerance lets the slack rest farther out, but the gate rises with it.
	loose := build()
	loose.Solve(sketch.WithTolerance(1e-8))
	lrep := loose.Verify(sketch.WithTolerance(1e-8))
	require.Equal(t, sketch.FullyConstrained, lrep.Status)
	require.Empty(t, lrep.Redundant)
	require.False(t, lrep.Trustworthy(), "still gated at a looser tolerance")
}
