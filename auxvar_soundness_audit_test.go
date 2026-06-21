package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// These tests lock in soundness invariants confirmed by an adversarial audit of the
// aux-variable constraint machinery (bounded foot-parameter slack boxes, no-cusp
// guards, conic internal/external branch slack) and of goal-solve's interaction with
// Verify. The oracle's prime directive: it must never report Solvable/Trustworthy
// for a sketch whose hard constraints do not actually hold.

// humpSpline is a fixed convex spline over x∈[0,3], rising to y≈1.5.
func humpSpline(s *sketch.Sketch) *sketch.Spline {
	cp := []*sketch.Point{s.AddPoint(0, 0), s.AddPoint(1, 2), s.AddPoint(2, 2), s.AddPoint(3, 0)}
	for _, p := range cp {
		s.Fix(p)
	}
	sp, err := s.AddSpline(cp[0], cp[1], cp[2], cp[3])
	if err != nil {
		panic(err)
	}
	return sp
}

func TestAuditTangentToSplineImpossibleRejected(t *testing.T) {
	// A line pinned horizontally at y=5 — well above the hump's max (~1.5) — cannot be
	// tangent to the finite spline. The bounded foot-parameter must NOT manufacture an
	// off-domain "tangent": the constraint stays unsatisfied, so Solvable is false.
	s := sketch.New()
	sp := humpSpline(s)
	a, b := s.AddPoint(-5, 5), s.AddPoint(5, 5)
	s.Fix(a)
	s.Fix(b)
	l := s.AddLine(a, b)
	s.AddConstraint(sketch.NewTangentToSpline(l, sp))
	s.Solve()
	require.False(t, s.Verify().Solvable, "an impossible line/spline tangency must not be blessed Solvable")
}

func TestAuditTangentToSplineFeasibleReallyTouches(t *testing.T) {
	// A free line constrained tangent must converge to a REAL tangency — the solved
	// line genuinely touches the finite spline (perpendicular distance ~0).
	s := sketch.New()
	sp := humpSpline(s)
	a, b := s.AddPoint(-2, 0.5), s.AddPoint(5, 0.5)
	l := s.AddLine(a, b)
	s.AddConstraint(sketch.NewTangentToSpline(l, sp))
	s.Solve()
	require.True(t, s.Verify().Solvable, "a feasible tangency must be solvable")

	poly := sp.Polyline(3000)
	ax, ay := l.Start.X(), l.Start.Y()
	dx, dy := l.End.X()-ax, l.End.Y()-ay
	length := math.Hypot(dx, dy)
	require.Greater(t, length, 1e-9, "the tangent line must not collapse to a point")
	minDist := math.Inf(1)
	for _, p := range poly {
		d := math.Abs((p[0]-ax)*dy-(p[1]-ay)*dx) / length
		if d < minDist {
			minDist = d
		}
	}
	require.Less(t, minDist, 1e-3, "a blessed tangent must actually touch the finite spline")
}

func TestAuditPointOnSplinePulledOffDomainRejected(t *testing.T) {
	// A point on a fixed spline, pulled toward a far anchor (20,0) past the spline's
	// x∈[0,3] domain. The bounded foot-parameter cannot extrapolate the spline, so the
	// point cannot both stay on it and reach the anchor: the system is not solvable, and
	// the oracle must NOT bless a point that has left the curve.
	s := sketch.New()
	sp := humpSpline(s)
	p := s.AddPoint(0, 0)
	s.AddConstraint(sketch.NewPointOnSpline(p, sp))
	anchor := s.AddPoint(20, 0)
	s.Fix(anchor)
	s.AddConstraint(sketch.NewDistance(p, anchor, 0))
	s.Solve()

	rep := s.Verify()
	poly := sp.Polyline(3000)
	onSpline := math.Inf(1)
	for _, q := range poly {
		d := math.Hypot(q[0]-p.X(), q[1]-p.Y())
		if d < onSpline {
			onSpline = d
		}
	}
	// Either the point genuinely stayed on the spline, or the sketch is reported
	// not-solvable. It must never be blessed Solvable with the point off the curve.
	require.Falsef(t, rep.Solvable && onSpline > 1e-6,
		"point-on-spline blessed Solvable while off the finite curve (dist=%.4f)", onSpline)
}

func TestAuditConicTangencyImpossibleRejected(t *testing.T) {
	// A small ellipse and a circle with both centres fixed far apart, asked for INTERNAL
	// tangency. No internal tangency exists, so the contact-witness/branch-slack machinery
	// must not fabricate one: Solvable is false.
	s := sketch.New()
	ec := s.AddPoint(0, 0)
	s.Fix(ec)
	e := s.AddEllipse(ec, 2.0, 1.0, 0.0)
	cc := s.AddPoint(10, 0)
	s.Fix(cc)
	c := s.AddCircle(cc, 1.0)
	s.AddConstraint(sketch.NewTangentEllipseCircular(e, c, true))
	s.Solve()
	require.False(t, s.Verify().Solvable, "an impossible internal conic tangency must not be blessed Solvable")
}

func TestAuditGoalSolveDoesNotCorruptVerify(t *testing.T) {
	// A hard distance constraint |ab|=10 plus a goal pulling b toward (1000,0). Goals are
	// transient solver rows, never constraints: the hard constraint must win, and Verify's
	// Solvable/Residual must reflect ONLY the hard constraints, never the unreachable goal.
	s := sketch.New()
	a := s.AddPoint(0, 0)
	s.Fix(a)
	b := s.AddPoint(10, 0)
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.Solve(sketch.WithGoal(b, 1000, 0))

	require.InDelta(t, 10.0, math.Hypot(b.X()-a.X(), b.Y()-a.Y()), 1e-6, "the hard distance must win over the goal")
	require.True(t, s.Verify().Solvable, "Verify must reflect only the hard constraints, which hold")
}
