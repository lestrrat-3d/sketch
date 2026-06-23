package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// loopSpline builds a fixed closed (periodic) cubic B-spline over a convex
// pentagon of grounded control points — a rigid loop to attach points to.
func loopSpline(s *sketch.Sketch) *sketch.ClosedSpline {
	ctrl := []*sketch.Point{
		s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(5, 3),
		s.CreatePoint(2, 5), s.CreatePoint(-1, 3),
	}
	sp, err := s.CreateClosedSpline(ctrl...)
	if err != nil {
		panic(err)
	}
	for _, c := range ctrl {
		s.Fix(c)
	}
	return sp
}

// archFit builds a fixed fit-point (interpolating) spline over grounded fit
// points — a rigid arch the curve passes through.
func archFit(s *sketch.Sketch) *sketch.FitSpline {
	fit := []*sketch.Point{
		s.CreatePoint(0, 0), s.CreatePoint(2, 3), s.CreatePoint(6, 3), s.CreatePoint(8, 0),
	}
	sp, err := s.CreateFitSpline(fit...)
	if err != nil {
		panic(err)
	}
	for _, c := range fit {
		s.Fix(c)
	}
	return sp
}

func distToPolyline(px, py float64, poly [][2]float64) float64 {
	best := math.Inf(1)
	for i := 1; i < len(poly); i++ {
		ax, ay := poly[i-1][0], poly[i-1][1]
		bx, by := poly[i][0], poly[i][1]
		dx, dy := bx-ax, by-ay
		seg2 := dx*dx + dy*dy
		u := 0.0
		if seg2 > 0 {
			u = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/seg2))
		}
		cx, cy := ax+u*dx, ay+u*dy
		if d := math.Hypot(px-cx, py-cy); d < best {
			best = d
		}
	}
	return best
}

func TestPointOnClosedSpline(t *testing.T) {
	s := newSketch(t)
	sp := loopSpline(s)
	p := s.CreatePoint(2, 2) // inside the loop; pulled outward onto the curve
	s.AddConstraint(sketch.NewPointOnClosedSpline(p, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToPolyline(p.X(), p.Y(), sp.Polyline(600)), 1e-4,
		"point pulled onto the closed spline")
}

func TestPointOnClosedSplineToIntersection(t *testing.T) {
	// Pinning the point onto the loop AND a fixed line lands it at their crossing
	// — a fully determined membership, witnessing the constraint does real work.
	s := newSketch(t)
	sp := loopSpline(s)
	a, b := s.CreatePoint(2, -2), s.CreatePoint(2, 6) // vertical line x=2
	s.Fix(a)
	s.Fix(b)
	p := s.CreatePoint(2, 0.5)
	s.AddConstraint(sketch.NewPointOnClosedSpline(p, sp))
	s.AddConstraint(sketch.NewPointOnLine(p, s.CreateLine(a, b)))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 2, p.X(), 1e-6, "on the x=2 line")
	require.InDelta(t, 0, distToPolyline(p.X(), p.Y(), sp.Polyline(600)), 1e-4, "and on the loop")

	// The membership flows through Verify: a DOF-0 point-on-closed-spline sketch
	// gets a real (non-NaN, finite) Conditioning and reads trustworthy — the
	// periodic foot-parameter aux var is correctly classified for the gate.
	rep := s.Verify()
	require.Equal(t, 0, rep.DOF)
	require.False(t, math.IsNaN(rep.Conditioning), "row-kinds classified (not the NaN gap sentinel)")
	require.True(t, rep.Trustworthy())
}

func TestPointOnClosedSplineDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	sp := loopSpline(s)
	p := s.CreatePoint(2, 2)
	require.Equal(t, 2, s.DOF(), "the free point has two DOF")

	con := sketch.NewPointOnClosedSpline(p, sp)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "on a 1-D loop the point keeps one sliding DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal restores the DOF (aux var retired)")
}

func TestPointOnClosedSplineCheckConstraint(t *testing.T) {
	s := newSketch(t)
	sp := loopSpline(s)
	p := s.CreatePoint(2, 2)
	require.NoError(t, s.CheckConstraint(sketch.NewPointOnClosedSpline(p, sp)))
}

func TestPointOnClosedSplineRoundTrip(t *testing.T) {
	s := newSketch(t)
	sp := loopSpline(s)
	p := s.CreatePoint(2, 2)
	s.AddConstraint(sketch.NewPointOnClosedSpline(p, sp))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve()
	require.NoError(t, err)
}

func TestPointOnFitSpline(t *testing.T) {
	s := newSketch(t)
	sp := archFit(s)
	p := s.CreatePoint(4, 1) // below the arch interior; pulled up onto it
	s.AddConstraint(sketch.NewPointOnFitSpline(p, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToPolyline(p.X(), p.Y(), sp.Polyline(400)), 1e-4,
		"point pulled onto the fit spline")
	require.Greater(t, p.Y(), 1.0, "moved up onto the interior of the arch")
}

func TestPointOnFitSplineConfinedToRange(t *testing.T) {
	// A point started past the curve's end must attach at the endpoint (the last
	// fit point, which the curve passes through), not extrapolate — the slack box
	// keeps the foot parameter within [0, 1].
	s := newSketch(t)
	sp := archFit(s)
	p := s.CreatePoint(20, -5) // far past the (8,0) end fit point
	s.AddConstraint(sketch.NewPointOnFitSpline(p, sp))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 0, distToPolyline(p.X(), p.Y(), sp.Polyline(400)), 1e-4, "still on the curve")
	require.InDelta(t, 8, p.X(), 0.1, "attached at the end fit point")
	require.InDelta(t, 0, p.Y(), 0.1)
}

func TestPointOnFitSplineToIntersectionVerify(t *testing.T) {
	// A DOF-0 point-on-fit-spline sketch (point pinned to the curve and a vertical
	// line) flows through Verify with a real Conditioning and reads trustworthy.
	s := newSketch(t)
	sp := archFit(s)
	a, b := s.CreatePoint(4, -2), s.CreatePoint(4, 6) // vertical line x=4
	s.Fix(a)
	s.Fix(b)
	p := s.CreatePoint(4, 1)
	s.AddConstraint(sketch.NewPointOnFitSpline(p, sp))
	s.AddConstraint(sketch.NewPointOnLine(p, s.CreateLine(a, b)))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 4, p.X(), 1e-6, "on the x=4 line")
	require.InDelta(t, 0, distToPolyline(p.X(), p.Y(), sp.Polyline(400)), 1e-4, "and on the curve")

	rep := s.Verify()
	require.Equal(t, 0, rep.DOF)
	require.False(t, math.IsNaN(rep.Conditioning))
	require.True(t, rep.Trustworthy())
}

func TestPointOnFitSplineDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	sp := archFit(s)
	p := s.CreatePoint(4, 1)
	require.Equal(t, 2, s.DOF(), "the free point has two DOF")

	con := sketch.NewPointOnFitSpline(p, sp)
	s.AddConstraint(con)
	require.Equal(t, 1, s.DOF(), "on a 1-D curve the point keeps one sliding DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 2, s.DOF(), "removal restores the DOF (aux vars retired)")
}

func TestPointOnFitSplineCheckConstraint(t *testing.T) {
	s := newSketch(t)
	sp := archFit(s)
	p := s.CreatePoint(4, 1)
	require.NoError(t, s.CheckConstraint(sketch.NewPointOnFitSpline(p, sp)))
}

func TestPointOnFitSplineRoundTrip(t *testing.T) {
	s := newSketch(t)
	sp := archFit(s)
	p := s.CreatePoint(4, 1)
	s.AddConstraint(sketch.NewPointOnFitSpline(p, sp))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve()
	require.NoError(t, err)
}
