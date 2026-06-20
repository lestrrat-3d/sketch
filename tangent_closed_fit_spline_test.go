package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func polyMaxY(poly [][2]float64) float64 {
	best := math.Inf(-1)
	for _, q := range poly {
		if q[1] > best {
			best = q[1]
		}
	}
	return best
}

// lineGapToPolyline is the minimum perpendicular distance from the polyline to the
// infinite line through p1,p2 — zero when the line touches the curve.
func lineGapToPolyline(p1, p2 *sketch.Point, poly [][2]float64) float64 {
	ax, ay := p1.X(), p1.Y()
	dx, dy := p2.X()-ax, p2.Y()-ay
	dlen := math.Hypot(dx, dy)
	best := math.Inf(1)
	for _, q := range poly {
		if d := math.Abs(dx*(q[1]-ay)-dy*(q[0]-ax)) / dlen; d < best {
			best = d
		}
	}
	return best
}

func TestTangentToClosedSpline(t *testing.T) {
	// A horizontal line above the loop becomes tangent at its top (the point whose
	// tangent is horizontal), settling at the loop's max height.
	s := sketch.New()
	sp := loopSpline(s) // fixed convex-pentagon control loop
	p1 := s.AddPoint(-4, 4.8)
	p2 := s.AddPoint(8, 4.8)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToClosedSpline(line, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, p1.Y(), p2.Y(), 1e-9, "line stays horizontal")
	require.InDelta(t, polyMaxY(sp.Polyline(600)), p1.Y(), 1e-3, "tangent at the loop's top")
	require.InDelta(t, 0, lineGapToPolyline(p1, p2, sp.Polyline(600)), 1e-3, "line touches the loop")
}

func TestTangentToClosedSplineDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	sp := loopSpline(s)
	p1 := s.AddPoint(-4, 4.8)
	p2 := s.AddPoint(8, 4.8)
	require.Equal(t, 4, s.DOF(), "a free line has four DOF")

	con := sketch.NewTangentToClosedSpline(s.AddLine(p1, p2), sp)
	s.AddConstraint(con)
	require.Equal(t, 3, s.DOF(), "tangency removes one DOF (the no-cusp slack nets out)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 4, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestTangentToClosedSplineCheckConstraint(t *testing.T) {
	s := sketch.New()
	sp := loopSpline(s)
	p1 := s.AddPoint(-4, 4.8)
	p2 := s.AddPoint(8, 4.8)
	require.NoError(t, s.CheckConstraint(sketch.NewTangentToClosedSpline(s.AddLine(p1, p2), sp)))
}

func TestTangentToClosedSplineRoundTrip(t *testing.T) {
	s := sketch.New()
	sp := loopSpline(s)
	line := s.AddLine(s.AddPoint(-4, 4.8), s.AddPoint(8, 4.8))
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToClosedSpline(line, sp))
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

func TestTangentToFitSpline(t *testing.T) {
	// A horizontal line above the fit arch becomes tangent at its top.
	s := sketch.New()
	sp := archFit(s) // fixed fit points (0,0),(2,3),(6,3),(8,0)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToFitSpline(line, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, p1.Y(), p2.Y(), 1e-9, "line stays horizontal")
	require.InDelta(t, polyMaxY(sp.Polyline(400)), p1.Y(), 1e-3, "tangent at the arch's peak")
	require.InDelta(t, 0, lineGapToPolyline(p1, p2, sp.Polyline(400)), 1e-3, "line touches the curve")
}

func TestTangentToFitSplineDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	sp := archFit(s)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	require.Equal(t, 4, s.DOF(), "a free line has four DOF")

	con := sketch.NewTangentToFitSpline(s.AddLine(p1, p2), sp)
	s.AddConstraint(con)
	require.Equal(t, 3, s.DOF(), "tangency removes one DOF (the slacks net out)")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 4, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestTangentToFitSplineCheckConstraint(t *testing.T) {
	s := sketch.New()
	sp := archFit(s)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	require.NoError(t, s.CheckConstraint(sketch.NewTangentToFitSpline(s.AddLine(p1, p2), sp)))
}

func TestTangentToFitSplineRoundTrip(t *testing.T) {
	s := sketch.New()
	sp := archFit(s)
	line := s.AddLine(s.AddPoint(-2, 3.5), s.AddPoint(10, 3.5))
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToFitSpline(line, sp))
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

func TestTangentToFitSplineTransverseRejected(t *testing.T) {
	// A fixed vertical line crosses the arch transversally; the fit arch's x is
	// monotonic so it never has a vertical tangent. Tangency is impossible and the
	// oracle must report it unsolvable, not bless the transverse crossing.
	s := sketch.New()
	sp := archFit(s)
	a := s.AddPoint(4, -2)
	b := s.AddPoint(4, 8)
	s.Fix(a)
	s.Fix(b) // a rigid vertical line x=4
	s.AddConstraint(sketch.NewTangentToFitSpline(s.AddLine(a, b), sp))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "a transverse crossing is not a tangent")
}

func TestTangentToClosedSplineNonTouchingRejected(t *testing.T) {
	// A rigid line far from the loop cannot touch it, so it cannot be tangent — the
	// contact residual can never reach zero. (A closed loop has tangents in every
	// direction, so the rejection case is a non-touching line, not a transverse one.)
	s := sketch.New()
	sp := loopSpline(s)
	a := s.AddPoint(100, -5)
	b := s.AddPoint(100, 5)
	s.Fix(a)
	s.Fix(b) // rigid line x=100, far from the loop near the origin
	s.AddConstraint(sketch.NewTangentToClosedSpline(s.AddLine(a, b), sp))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "a line that cannot reach the loop is not tangent")
}

func TestTangentToClosedSplineDOF0Verify(t *testing.T) {
	// A tangent line from a fixed external point, pinned to DOF 0 by a distance,
	// flows through Verify with a real Conditioning and reads trustworthy.
	s := sketch.New()
	sp := loopSpline(s)
	e := s.AddPoint(10, 8)
	s.Fix(e)
	f := s.AddPoint(3, 4) // free; tangency + distance determine the line
	s.AddConstraint(sketch.NewTangentToClosedSpline(s.AddLine(e, f), sp))
	s.AddConstraint(sketch.NewDistance(e, f, 9))

	_, err := s.Solve()
	require.NoError(t, err)
	rep := s.Verify()
	require.Equal(t, 0, rep.DOF)
	require.False(t, math.IsNaN(rep.Conditioning), "row-kinds classified (not the NaN gap sentinel)")
	require.True(t, rep.Trustworthy())
}

func TestTangentToFitSplineDOF0Verify(t *testing.T) {
	// A horizontal tangent at the arch's peak, pinned to DOF 0 by fixing both
	// endpoints' x to a datum, flows through Verify and reads trustworthy.
	s := sketch.New()
	sp := archFit(s)
	ref := s.AddPoint(0, 100)
	s.Fix(ref)
	p1 := s.AddPoint(-2, 3.5)
	p2 := s.AddPoint(10, 3.5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontalDistance(ref, p1, -2))
	s.AddConstraint(sketch.NewHorizontalDistance(ref, p2, 10))
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentToFitSpline(line, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	rep := s.Verify()
	require.Equal(t, 0, rep.DOF)
	require.False(t, math.IsNaN(rep.Conditioning))
	require.True(t, rep.Trustworthy())
}
