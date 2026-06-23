package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// chordFractions returns the cumulative chord-length fraction of each point.
func chordFractions(pts [][2]float64) []float64 {
	cum := make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		cum[i] = cum[i-1] + math.Hypot(pts[i][0]-pts[i-1][0], pts[i][1]-pts[i-1][1])
	}
	total := cum[len(cum)-1]
	for i := range cum {
		cum[i] /= total
	}
	return cum
}

func TestFitSplineInterpolatesFitPoints(t *testing.T) {
	s := newSketch(t)
	pts := []*sketch.Point{s.AddPoint(0, 0), s.AddPoint(2, 3), s.AddPoint(5, -1), s.AddPoint(7, 1)}
	sp, err := s.AddFitSpline(pts...)
	require.NoError(t, err)
	coords := make([][2]float64, len(pts))
	for i, p := range pts {
		coords[i] = [2]float64{p.X(), p.Y()}
	}
	frac := chordFractions(coords)
	for i, p := range pts {
		x, y := sp.Eval(frac[i])
		require.InDelta(t, p.X(), x, 1e-9, "interpolates fit point %d", i)
		require.InDelta(t, p.Y(), y, 1e-9)
	}
}

func TestFitSplineInterpolatesAfterSolve(t *testing.T) {
	// The whole point of architecture A: the solver moves a fit point, and the
	// curve still passes through the MOVED point (the interpolant is recomputed).
	s := newSketch(t)
	p0 := s.AddPoint(0, 0)
	p1 := s.AddPoint(3, 2)
	p2 := s.AddPoint(6, 0)
	s.Fix(p0)
	s.Fix(p2)
	sp, err := s.AddFitSpline(p0, p1, p2)
	require.NoError(t, err)
	s.AddConstraint(sketch.NewVerticalDistance(p0, p1, 5)) // pull p1 up to y=5
	_, err = s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, p1.Y(), 1e-6, "the solver moved p1")

	coords := [][2]float64{{p0.X(), p0.Y()}, {p1.X(), p1.Y()}, {p2.X(), p2.Y()}}
	frac := chordFractions(coords)
	x, y := sp.Eval(frac[1])
	require.InDelta(t, p1.X(), x, 1e-9, "curve still passes through the moved middle fit point")
	require.InDelta(t, p1.Y(), y, 1e-9)
}

func TestFitSplineBoundsProfile(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	m1 := s.AddPoint(2, 3)
	m2 := s.AddPoint(4, 3)
	b := s.AddPoint(6, 0)
	sp, err := s.AddFitSpline(a, m1, m2, b)
	require.NoError(t, err)
	s.AddLine(b, a) // chord closes the loop

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "fit spline + chord bound one region")
	require.True(t, profiles[0].Valid)
	require.Greater(t, profiles[0].Area, 0.0)
	require.Contains(t, profiles[0].Entities, sketch.Entity(sp))
}

func TestFitSplineSelfCrossingInvalid(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	m1 := s.AddPoint(4, 1)
	m2 := s.AddPoint(0, 2)
	m3 := s.AddPoint(4, 3)
	_, err := s.AddFitSpline(a, m1, m2, m3)
	require.NoError(t, err)
	s.AddLine(m3, a)

	rep := s.Verify()
	require.False(t, rep.ProfilesValid)
	require.NotEmpty(t, rep.InvalidProfiles)
	var sawSelfX bool
	for _, p := range rep.InvalidProfiles {
		if p.SelfIntersecting {
			sawSelfX = true
		}
	}
	require.True(t, sawSelfX)
}

func TestFitSplineValidation(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	_, err := s.AddFitSpline(a)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "fewer than 2 fit points is rejected")
	b := s.AddPoint(1, 0)
	_, err = s.AddFitSpline(a, nil, b)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "a nil fit point is rejected")
}

func TestFitSplineFixEntityDOF(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	m := s.AddPoint(3, 2)
	b := s.AddPoint(6, 0)
	sp, err := s.AddFitSpline(a, m, b)
	require.NoError(t, err)
	require.Equal(t, 6, s.DOF(), "3 free fit points, no size vars or internal constraints")
	s.FixEntity(sp)
	require.Equal(t, 0, s.DOF())
}

func TestFitSplineConstructionExcluded(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	m1 := s.AddPoint(2, 3)
	m2 := s.AddPoint(4, 3)
	b := s.AddPoint(6, 0)
	sp, err := s.AddFitSpline(a, m1, m2, b)
	require.NoError(t, err)
	sp.SetConstruction(true)
	s.AddLine(b, a)
	require.Empty(t, s.Profiles(), "a construction fit spline bounds no reported profile")
}

func TestFitSplineRoundTrip(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	m1 := s.AddPoint(2, 3)
	m2 := s.AddPoint(4, 3)
	b := s.AddPoint(6, 0)
	_, err := s.AddFitSpline(a, m1, m2, b)
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	require.Contains(t, string(data), "fit_spline", "serialized as a distinct entity type")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), 1)
	fs, isFit := s2.Entities()[0].(*sketch.FitSpline)
	require.True(t, isFit, "reloads as a FitSpline, not an open Spline")
	// the reloaded curve still interpolates its first fit point
	x, y := fs.Eval(0)
	require.InDelta(t, 0, x, 1e-9)
	require.InDelta(t, 0, y, 1e-9)
}

func TestFitSplineExportersAndDegenerate(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	m := s.AddPoint(3, 2)
	b := s.AddPoint(6, 0)
	_, err := s.AddFitSpline(a, m, b)
	require.NoError(t, err)

	svg, err := s.SVG()
	require.NoError(t, err)
	require.Contains(t, svg, "<path")
	dxf, err := s.DXF()
	require.NoError(t, err)
	require.Contains(t, dxf, "LWPOLYLINE")
	png, err := s.PNG()
	require.NoError(t, err)
	require.NotEmpty(t, png)

	// Coincident consecutive fit points must not panic (the evaluator collapses
	// zero-length chord spans).
	s2 := newSketch(t)
	c0 := s2.AddPoint(0, 0)
	c1 := s2.AddPoint(0, 0) // coincident with c0
	c2 := s2.AddPoint(4, 0)
	sp, err := s2.AddFitSpline(c0, c1, c2)
	require.NoError(t, err)
	require.NotPanics(t, func() {
		sp.Polyline(16)
		s2.Profiles()
	})
}

func TestFitSplineForeignPointNotTrustworthy(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 0)
	s.Fix(a)
	s.Fix(b)
	other := newSketch(t)
	foreign := other.AddPoint(2, 3)
	_, err := s.AddFitSpline(a, foreign, b)
	require.NoError(t, err)
	rep := s.Verify()
	require.True(t, rep.ForeignHandles)
	require.False(t, rep.Trustworthy())
}

func TestFitSplineTypedNilEntity(t *testing.T) {
	s := newSketch(t)
	_, err := s.WorldPolyline((*sketch.FitSpline)(nil))
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
}
