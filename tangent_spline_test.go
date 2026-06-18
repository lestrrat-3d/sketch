package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// splineMaxY samples the spline and returns its highest y (the arch's peak,
// where the tangent is horizontal).
func splineMaxY(sp *sketch.Spline) float64 {
	best := math.Inf(-1)
	for _, q := range sp.Polyline(400) {
		if q[1] > best {
			best = q[1]
		}
	}
	return best
}

func TestTangentToSpline(t *testing.T) {
	// A horizontal line above the arch becomes tangent at the peak (the one point
	// whose tangent is horizontal), so it settles at the spline's max height.
	s := sketch.New()
	sp := archSpline(s) // fixed control points (0,0),(2,4),(6,4),(8,0)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToSpline(line, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, p1.Y(), p2.Y(), 1e-9, "line stays horizontal")
	require.InDelta(t, splineMaxY(sp), p1.Y(), 1e-3, "tangent at the peak (horizontal tangent)")
	require.InDelta(t, 0, lineGapToSpline(p1, p2, sp), 1e-3, "line touches the curve")
}

// lineGapToSpline returns the minimum perpendicular distance from the spline to
// the infinite line through p1,p2 — zero when the line touches the curve.
func lineGapToSpline(p1, p2 *sketch.Point, sp *sketch.Spline) float64 {
	ax, ay := p1.X(), p1.Y()
	dx, dy := p2.X()-ax, p2.Y()-ay
	dlen := math.Hypot(dx, dy)
	best := math.Inf(1)
	for _, q := range sp.Polyline(400) {
		d := math.Abs(dx*(q[1]-ay)-dy*(q[0]-ax)) / dlen
		if d < best {
			best = d
		}
	}
	return best
}

func TestTangentToSplineTransverseRejected(t *testing.T) {
	// A fixed vertical line crosses the arch transversally; the spline's x is
	// monotonic so it never has a vertical tangent. Tangency is impossible and the
	// oracle must report it unsolvable, not bless the transverse crossing.
	s := sketch.New()
	sp := archSpline(s)
	a := s.AddPoint(5, -2)
	b := s.AddPoint(5, 8)
	s.Fix(a)
	s.Fix(b) // a rigid vertical line x=5
	s.AddConstraint(sketch.NewTangentToSpline(s.AddLine(a, b), sp))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "a transverse crossing is not a tangent")
}

func TestTangentToSplineDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	sp := archSpline(s) // control points fixed
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	require.Equal(t, 4, s.DOF(), "a free line has four DOF")

	con := sketch.NewTangentToSpline(s.AddLine(p1, p2), sp)
	s.AddConstraint(con)
	require.Equal(t, 3, s.DOF(), "tangency removes one DOF (the slacks net out)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 4, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestTangentToSplineCheckConstraint(t *testing.T) {
	s := sketch.New()
	sp := archSpline(s)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	// Tangent to a free line removes one DOF — not over-constraining.
	require.NoError(t, s.CheckConstraint(sketch.NewTangentToSpline(s.AddLine(p1, p2), sp)))
}

func TestTangentToSplineScaleIsCurrentNotSnapshot(t *testing.T) {
	// The zero-length-line cutoff is scale-relative, and the scale must track the
	// spline's CURRENT size — not a snapshot from when the constraint was added.
	// The tangency is added while the spline is small (where a snapshot cutoff is
	// tiny and the 1e-7 line reads as a valid horizontal carrier tangent at the
	// curve's near-horizontal end), then distance dimensions drive the spline to a
	// ~1000-unit box. A snapshot scale keeps blessing the tangent; the current
	// scale makes the 1e-7 line degenerate (cutoff ≈ 1e-9·1000 ≫ 1e-7), so the
	// oracle must report it unsolvable.
	x := math.Sqrt(99) // c0–c1 distance is exactly 10 to start in a clean basin
	s := sketch.New()
	c0 := s.AddPoint(0, 0)
	c1 := s.AddPoint(x, 1)
	c2 := s.AddPoint(x+10, 1)
	c3 := s.AddPoint(x+20, 1)
	sp, err := s.AddSpline(c0, c1, c2, c3)
	require.NoError(t, err)
	s.Fix(c0)

	a := s.AddPoint(10, 1)
	b := s.AddPoint(10.0000001, 1) // length 1e-7, horizontal
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewTangentToSpline(s.AddLine(a, b), sp)) // allocVars sees the small spline

	s.AddConstraint(sketch.NewDistance(c0, c1, 1000))
	s.AddConstraint(sketch.NewDistance(c1, c2, 1000))
	s.AddConstraint(sketch.NewDistance(c2, c3, 1000))

	_, err = s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "a degenerate-length line is not a tangent at the enlarged scale")
}

func TestTangentToSplineRoundTrip(t *testing.T) {
	s := sketch.New()
	sp := archSpline(s)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToSpline(line, sp))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive reload")
	_, err = s2.Solve()
	require.NoError(t, err)
}
