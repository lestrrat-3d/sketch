package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// nanControlCoordinateNURBS builds a degree-3 NURBS whose control points are
// collinear along y=5 — so the curve is an exact straight line from (-2,5) to
// (12,5), crossing a 10x10 square at (0,0)-(10,10) clean through the middle —
// with its middle control point's x coordinate either an ordinary 4 or NaN.
// CreateNURBS rejects a non-finite knot, but CreatePoint has no error return
// and the solver moves control-point coordinates after construction, so a
// construction-time check there could never hold as a precondition; a NaN
// coordinate reaches committed geometry exactly as a NaN knot once did. Every
// control point is fixed in BOTH shapes, the poisoned one included: whether it
// is fixed changes nothing the two tests assert (same DOF, same status, same
// profile verdict), so the fixtures differ only in the NaN itself.
func nanControlCoordinateNURBS(t *testing.T, s *sketch.Sketch, nan bool) *sketch.NURBS {
	t.Helper()
	mid := 4.0
	if nan {
		mid = math.NaN()
	}
	ctrl := []*sketch.Point{
		s.CreatePoint(-2, 5), s.CreatePoint(1, 5), s.CreatePoint(mid, 5),
		s.CreatePoint(7, 5), s.CreatePoint(12, 5),
	}
	for _, p := range ctrl {
		s.Fix(p)
	}
	knots := sketch.ClampedUniformKnots(len(ctrl), 3)
	c, err := s.CreateNURBS(3, ctrl, nil, knots)
	require.NoError(t, err)
	return c
}

func groundedSquare(t *testing.T, s *sketch.Sketch) *sketch.Rectangle {
	t.Helper()
	r := s.CreateRectangle(0, 0, 10, 10)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 10), sketch.NewDistance(r.A, r.D, 10))
	return r
}

// TestNaNControlCoordinateNURBSReportsNonFiniteGeometry reproduces the
// non-finite-geometry defect end to end through the public API. The
// NaN-coordinate NURBS control point is a solver variable like any point's
// x/y, so it poisons the finite-difference centroid every rank/DOF/
// conditioning pass subtracts (positionShift) before the arrangement engine
// ever sees the curve — which is why Verify must stop at the non-finite-
// geometry screen rather than let the rank/profile passes run on it and
// report a number. This used to assert DOF == 2, a fully-populated FreePoints
// naming only the SQUARE's own points (never a NURBS control point, even
// though every one of them is fixed), and an invalid profile — the corrupted
// rank/DOF answer and the vanished-curve profile answer, both accepted as
// expected behaviour. Under the fix, Verify stops before computing either:
// DOF, FreePoints and the profile pass all keep their unevaluated zero
// values, and the report instead names the poisoned point directly.
func TestNaNControlCoordinateNURBSReportsNonFiniteGeometry(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nurbs := nanControlCoordinateNURBS(t, s, true)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Equal(t, []*sketch.Point{nurbs.Control[2]}, rep.NonFinitePoints,
		"the poisoned control point is named, and only it")
	require.Empty(t, rep.NonFiniteEntities)
	require.Empty(t, rep.NonFiniteDimensions)
	err := rep.Check()
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.ErrorIs(t, err, sketch.ErrVerificationIncomplete)
	require.NotErrorIs(t, err, sketch.ErrInvalidProfile,
		"the profile pass never ran, so it must not be asserted as a finding")
	require.NotErrorIs(t, err, sketch.ErrNotFullyConstrained,
		"DOF was never computed, so it must not be asserted as a finding")
	require.False(t, rep.Trustworthy(), "non-finite geometry must never be blessed")
}

// TestFiniteControlNURBSCrossingSquareIsTrustworthy is the healthy control for
// TestNaNControlCoordinateNURBSReportsNonFiniteGeometry: the identical
// crossing curve with an ordinary finite control point cleanly splits the
// square into two ~50 area regions and is fully trustworthy — the converged
// answer the NaN-coordinate case must never silently substitute (one region
// of area 100).
func TestFiniteControlNURBSCrossingSquareIsTrustworthy(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nanControlCoordinateNURBS(t, s, false)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Equal(t, 0, rep.DOF)
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.Len(t, rep.Profiles, 2, "the crossing line splits the square into two halves")
	require.True(t, rep.ProfilesValid)
	require.Empty(t, rep.InvalidProfiles)
	total := 0.0
	for _, p := range rep.Profiles {
		require.InDelta(t, 50, p.Area, 0.01, "each half is ~10 x 5")
		total += p.Area
	}
	require.InDelta(t, 100, total, 1e-6, "areas still sum exactly to the whole square")
	require.True(t, rep.Trustworthy(), "a clean, fully-constrained crossing is trustworthy")
}

// TestStrayFixedNaNPointReportsNonFiniteGeometry is the widest-reach case: a
// single grounded point at (NaN, 3), belonging to no entity and no
// constraint, sitting beside an otherwise ordinary fully-constrained square.
// Nothing else in the sketch references it, so no OTHER signal (a broken
// reference, a foreign handle, an invalid profile) exists to catch it —
// before the fix this point alone corrupted the whole sketch's rank analysis
// through the finite-difference centroid every pass subtracts
// (positionShift), reporting DOF 2 and naming the SQUARE's own points as
// free even though the square is fully dimensioned. It must instead be named
// directly and stop the analysis.
func TestStrayFixedNaNPointReportsNonFiniteGeometry(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	stray := s.CreatePoint(math.NaN(), 3)
	s.Fix(stray)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Equal(t, []*sketch.Point{stray}, rep.NonFinitePoints)
	require.ErrorIs(t, rep.Check(), sketch.ErrNonFiniteGeometry)
	require.False(t, rep.Trustworthy(), "a stray non-finite point must never be silently absorbed")
}

// TestStrayFixedFinitePointIsTrustworthy is the finite control for
// TestStrayFixedNaNPointReportsNonFiniteGeometry: the identical fully-
// constrained square with an ordinary finite grounded stray point beside it
// stays fully trustworthy, so the failure above is the NaN, not the presence
// of an unconnected fixed point.
func TestStrayFixedFinitePointIsTrustworthy(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	stray := s.CreatePoint(20, 3)
	s.Fix(stray)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Empty(t, rep.NonFinitePoints)
	require.Equal(t, 0, rep.DOF)
	require.True(t, rep.Trustworthy())
}

// allFixedNaNStraySketch builds the CONFIRMED false-bless fixture: a line whose
// both endpoints are grounded, plus a grounded stray point at either (NaN, NaN)
// or an ordinary finite coordinate. Every variable in it is grounded, which is
// what makes it the sharpest case a Jacobian-level guard cannot catch — the
// matrix is perfectly finite because it has zero columns, only the GEOMETRY
// behind it is not, and [Sketch.conditioningOn]'s own len(free)==0 shortcut
// returns +Inf (maximal trust) without ever looking at a value.
func allFixedNaNStraySketch(t *testing.T, nan bool) (*sketch.Sketch, *sketch.Point) {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(5, 0)
	s.Fix(a)
	s.Fix(b)
	s.CreateLine(a, b)
	x, y := 1.0, 1.0
	if nan {
		x, y = math.NaN(), math.NaN()
	}
	stray := s.CreatePoint(x, y)
	s.Fix(stray)
	return s, stray
}

// TestAllFixedNaNPointIsNoLongerFalselyBlessed pins the whole report on that
// fixture. Before the fix it read DOF 0, fully constrained, solvable, with no
// free points and no invalid profile — every signal byte-identical to the
// finite control below.
func TestAllFixedNaNPointIsNoLongerFalselyBlessed(t *testing.T) {
	s, stray := allFixedNaNStraySketch(t, true)

	rep := s.Verify(t.Context())
	require.Equal(t, []*sketch.Point{stray}, rep.NonFinitePoints)
	err := rep.Check()
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.ErrorIs(t, err, sketch.ErrVerificationIncomplete)
	require.False(t, rep.Trustworthy(),
		"an all-fixed sketch with a non-finite stray point must never be blessed")
	// Conditioning is +Inf when it is not applicable, which is its BEST
	// possible reading, so a report that evaluated nothing must not carry it:
	// the skipped path leaves it at the zero value the report's own doc
	// comment promises for every unevaluated field.
	require.Zero(t, rep.Conditioning,
		"the skipped path must not publish the most trusting conditioning value there is")
	// Status must not publish Overconstrained either: no rank was computed, so
	// asserting the most severe status is a finding about a sketch nothing
	// analysed. The zero value, Underconstrained, is what the doc comment
	// promises for every unevaluated field, Status included.
	require.Zero(t, rep.Status,
		"the skipped path must not publish a severity finding for analysis that never ran")
}

// TestAllFixedNaNPointBareReadsAreMaximumIgnorance pins the three bare reads
// DIRECTLY on the same fixture, not through the report. Each has no error
// return and so cannot refuse; each must still answer in the direction a
// caller cannot read as a pass. DOF and FreePoints are the load-bearing pair
// here: both are computed from the GROUNDING flags, and on an all-grounded
// sketch a free-variable count and a free-point set collapse onto 0 and empty
// — exactly the readings that say "fully constrained", on the very fixture the
// screen exists for.
func TestAllFixedNaNPointBareReadsAreMaximumIgnorance(t *testing.T) {
	s, _ := allFixedNaNStraySketch(t, true)

	// The origin's two variables plus two each for a, b and the stray point;
	// a line owns no variable of its own. Grounded variables are counted: Fix
	// IS a constraint, so a count that credits it is not maximum ignorance.
	require.Equal(t, 8, s.DOF(), "the total variable count, never the free-variable count")
	require.Equal(t, s.Points(), s.FreePoints(),
		"every point of the sketch, grounded ones included")
	// This fixture holds no constraints at all, so there is nothing for the
	// dependency analysis to be ignorant ABOUT; the constrained fixture in
	// TestNonFiniteBareReadsAreMaximumIgnorance covers that half.
	require.Empty(t, s.Constraints())
	require.Empty(t, s.Diagnose().Conflicting)
	require.Empty(t, s.Diagnose().Redundant)
	require.Empty(t, s.RedundantConstraints())
}

// TestAllFixedFinitePointBareReadsAreUnchanged is the finite control for
// TestAllFixedNaNPointBareReadsAreMaximumIgnorance: the identical all-grounded
// fixture with a finite stray point still answers from the real analysis, so
// the readings above are the NaN, not the fixture's shape.
func TestAllFixedFinitePointBareReadsAreUnchanged(t *testing.T) {
	s, _ := allFixedNaNStraySketch(t, false)

	require.Equal(t, 0, s.DOF())
	require.Empty(t, s.FreePoints())
	require.Empty(t, s.Diagnose().Conflicting)
	require.Empty(t, s.RedundantConstraints())
}

// TestNonFinitePerHandleReadsAreMaximumIgnorance pins the two PER-HANDLE bare
// reads on the same fixtures. Both answer from the null-space support the
// poisoned Jacobian produces, so unscreened they certify "nothing here can
// move" for geometry nothing analysed — and they did so on the very fixture
// where [Sketch.FreePoints] reports every point and [Sketch.DOF] reports the
// total variable count, so the per-handle reads contradicted the aggregate ones
// in the blessing direction. False is the answer that agrees with both and is
// already each method's documented refusal for a foreign, removed or nil
// handle.
func TestNonFinitePerHandleReadsAreMaximumIgnorance(t *testing.T) {
	t.Run("all grounded", func(t *testing.T) {
		s, _ := allFixedNaNStraySketch(t, true)

		require.NotEmpty(t, s.Points(), "the loop below is not vacuous")
		for _, p := range s.Points() {
			require.False(t, p.IsFullyConstrained(),
				"point %d must not read fully constrained on poisoned geometry", p.ID())
		}
		require.False(t, s.Origin().IsFullyConstrained(),
			"the origin is screened like every other handle, not exempted")
		require.NotEmpty(t, s.Entities(), "the loop below is not vacuous")
		for _, e := range s.Entities() {
			require.False(t, s.EntityIsFullyConstrained(e))
		}
		// The agreement the screen exists to restore: FreePoints names every
		// point, so no point may read fully constrained.
		require.Equal(t, s.Points(), s.FreePoints())
	})

	t.Run("with constraints", func(t *testing.T) {
		s, _, _ := nonFiniteCheckConstraintFixture(t, true)

		for _, p := range s.Points() {
			require.False(t, p.IsFullyConstrained())
		}
		for _, e := range s.Entities() {
			require.False(t, s.EntityIsFullyConstrained(e))
		}
		require.Equal(t, s.Points(), s.FreePoints())
	})
}

// TestFinitePerHandleReadsAreUnchanged is the finite control for
// TestNonFinitePerHandleReadsAreMaximumIgnorance: on identical geometry with an
// ordinary finite stray point both reads still answer from the real analysis,
// so the refusals above are the NaN and not a blanket false.
func TestFinitePerHandleReadsAreUnchanged(t *testing.T) {
	t.Run("all grounded", func(t *testing.T) {
		s, _ := allFixedNaNStraySketch(t, false)

		for _, p := range s.Points() {
			require.True(t, p.IsFullyConstrained(),
				"point %d is grounded, so it cannot move", p.ID())
		}
		require.True(t, s.Origin().IsFullyConstrained())
		for _, e := range s.Entities() {
			require.True(t, s.EntityIsFullyConstrained(e))
		}
	})

	t.Run("with constraints", func(t *testing.T) {
		s, _, _ := nonFiniteCheckConstraintFixture(t, false)

		pts := s.Points()
		stray := pts[len(pts)-1]
		require.False(t, stray.IsFullyConstrained(), "the free stray point can still move")
		for _, p := range pts[:len(pts)-1] {
			require.True(t, p.IsFullyConstrained(),
				"point %d is pinned by the dimensioned rectangle", p.ID())
		}
		for _, e := range s.Entities() {
			require.True(t, s.EntityIsFullyConstrained(e))
		}
	})
}

// TestNonFiniteBareReadsAreMaximumIgnorance covers the half the all-grounded
// fixture cannot: a sketch that actually holds constraints. [Sketch.Diagnose]
// and [Sketch.RedundantConstraints] must name every constraint the dependency
// analysis could ever flag rather than return empty, since a caller reads two
// empty lists as "no constraint problem found" — the one direction a poisoned
// analysis must never report.
func TestNonFiniteBareReadsAreMaximumIgnorance(t *testing.T) {
	s, _, _ := nonFiniteCheckConstraintFixture(t, true)

	cons := s.Constraints()
	require.NotEmpty(t, cons, "the assertions below are not vacuous")
	d := s.Diagnose()
	require.Equal(t, cons, d.Conflicting, "no constraint here has been proven consistent")
	require.Empty(t, d.Redundant, "'removing one changes nothing' is a claim nothing here supports")
	require.Equal(t, cons, s.RedundantConstraints(),
		"the flat list and the partition Diagnose refines it into name one set")
	require.Equal(t, s.Points(), s.FreePoints())
	// Five points at two variables each, plus the origin's two.
	require.Equal(t, 12, s.DOF())
}

// TestFiniteBareReadsAreUnchanged is the finite control for
// TestNonFiniteBareReadsAreMaximumIgnorance: the identical fixture with an
// ordinary finite stray point still answers from the real analysis — the free
// stray point alone, its own two degrees of freedom, and no dependent
// constraint anywhere.
func TestFiniteBareReadsAreUnchanged(t *testing.T) {
	s, _, _ := nonFiniteCheckConstraintFixture(t, false)

	pts := s.Points()
	require.Equal(t, []*sketch.Point{pts[len(pts)-1]}, s.FreePoints())
	require.Equal(t, 2, s.DOF())
	require.Empty(t, s.Diagnose().Conflicting)
	require.Empty(t, s.Diagnose().Redundant)
	require.Empty(t, s.RedundantConstraints())
}

// TestAllFixedFinitePointIsTrustworthy is the finite control for
// TestAllFixedNaNPointIsNoLongerFalselyBlessed: the identical all-grounded
// line plus stray point, both finite, is a genuine DOF-0 pass.
func TestAllFixedFinitePointIsTrustworthy(t *testing.T) {
	s, _ := allFixedNaNStraySketch(t, false)

	rep := s.Verify(t.Context())
	require.Empty(t, rep.NonFinitePoints)
	require.Equal(t, 0, rep.DOF)
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.True(t, math.IsInf(rep.Conditioning, 1),
		"an evaluated report still reports +Inf where conditioning does not apply")
	require.True(t, rep.Trustworthy())
}

// nonFiniteCheckConstraintFixture builds the exact "rejects a duplicate"
// fixture from TestCheckConstraint, plus one extra FREE point whose y is
// either an ordinary 3 or NaN, so the two tests using it differ only in the
// stray point's finiteness. The stray point is deliberately left free (not
// [Sketch.Fix]ed): this is the specific shape that was confirmed, by sweeping
// every combination of which existing point a NaN coordinate replaces, which
// coordinate it lands in, and whether the extra point is fixed or free,
// against this exact fixture — a FREE stray point with a NaN y is the one
// that reliably makes the pre-fix rank probe misjudge the duplicate's row as
// independent (a fixed stray point, or a NaN in x rather than y, still
// happened to reject it correctly here, for the wrong reason, on this
// particular fixture — the 29.6% figure in the design is a rate over many
// fixtures, not a guarantee on every one).
func nonFiniteCheckConstraintFixture(t *testing.T, nan bool) (*sketch.Sketch, *sketch.Point, *sketch.Point) {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(18, 2)
	c := s.CreatePoint(17, 11)
	dd := s.CreatePoint(1, 13)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	dc := s.CreateLine(dd, c)
	ad := s.CreateLine(a, dd)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, dd, 12))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	y := 3.0
	if nan {
		y = math.NaN()
	}
	s.CreatePoint(3, y)
	return s, a, b
}

// TestCheckConstraintRefusesNonFiniteGeometry reproduces the CheckConstraint
// false-accept: this sketch's OWN geometry already over-constrains a.b to 20
// (the identical dimension is a consistent duplicate, correctly refused by
// TestCheckConstraint's "rejects a duplicate" case), but the poisoned stray
// point corrupts the rank probe's pivot comparisons — never correctly
// selected or correctly rejected against a NaN — so before the fix
// CheckConstraint ranked the duplicate's row as independent and returned nil,
// accepting a constraint that genuinely over-constrains the sketch. The
// refusal must survive: CheckConstraint now reports this sketch's geometry is
// non-finite before it ever probes the candidate.
func TestCheckConstraintRefusesNonFiniteGeometry(t *testing.T) {
	s, a, b := nonFiniteCheckConstraintFixture(t, true)

	err := s.CheckConstraint(sketch.NewDistance(a, b, 20))
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry,
		"a duplicate must still be refused, now on the non-finite-geometry condition")
}

// TestCheckConstraintDrivenCandidateSkipsNonFiniteScreen pins the one candidate
// the non-finite screen deliberately does not reach. A driven dimension returns
// nil before the probe allocates, evaluates a residual or ranks anything, and it
// contributes no residual row at all (residuals() skips driven dimensions), so
// no verdict about it is ever computed from the poisoned Jacobian the screen
// exists to refuse. Refusing it would report a defect on an answer the geometry
// never takes part in. The same candidate left DRIVING is refused, which is what
// makes this a statement about the driven shortcut rather than about the
// candidate.
func TestCheckConstraintDrivenCandidateSkipsNonFiniteScreen(t *testing.T) {
	s, a, b := nonFiniteCheckConstraintFixture(t, true)

	driving := sketch.NewDistance(a, b, 20)
	require.ErrorIs(t, s.CheckConstraint(driving), sketch.ErrNonFiniteGeometry,
		"a driving candidate is still refused on this sketch's poisoned geometry")

	driven := sketch.NewDistance(a, b, 20)
	driven.SetDriven(true)
	require.NoError(t, s.CheckConstraint(driven),
		"a driven dimension is never ranked, so the poisoned rank pass cannot misjudge it")
}

// TestCheckConstraintFiniteControlStillRejectsDuplicate is the finite control
// for TestCheckConstraintRefusesNonFiniteGeometry: the identical fixture with
// an ordinary finite stray point still refuses the duplicate on the ordinary
// over-constraint condition.
func TestCheckConstraintFiniteControlStillRejectsDuplicate(t *testing.T) {
	s, a, b := nonFiniteCheckConstraintFixture(t, false)

	err := s.CheckConstraint(sketch.NewDistance(a, b, 20))
	require.ErrorIs(t, err, sketch.ErrOverconstrained, "consistent duplicate rejected")
}

// TestCheckConstraintRefusesNonFiniteCandidateTarget covers the second gap:
// CheckConstraint screened this SKETCH's geometry for non-finite variables but
// never the CANDIDATE's own target, so a poisoned candidate reached the rank
// probe unscreened. The decisive fixture is a sketch with NO constraints at
// all (DOF 4): a NaN target makes the whole central-difference row NaN, and
// the pre-fix rank probe read that row as dependent and returned nil — a
// FALSE statement about the sketch (there is nothing for it to depend on),
// not merely a differently-phrased true one. The identical finite candidate
// on the same empty sketch is the control: it must still pass, so the fix is
// the candidate's own non-finite target, not a blanket refusal.
func TestCheckConstraintRefusesNonFiniteCandidateTarget(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)

	err := s.CheckConstraint(sketch.NewDistance(a, b, math.NaN()))
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry,
		"a NaN candidate target must be refused even against an otherwise unconstrained, finite sketch")
}

// TestCheckConstraintFiniteCandidateTargetControl is the finite control for
// TestCheckConstraintRefusesNonFiniteCandidateTarget: the identical
// constraint-free sketch with an ordinary finite candidate target passes,
// since the candidate adds an independent equation to an empty sketch.
func TestCheckConstraintFiniteCandidateTargetControl(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)

	require.NoError(t, s.CheckConstraint(sketch.NewDistance(a, b, 10)))
}
