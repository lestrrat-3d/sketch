package sketch_test

import (
	"encoding/json"
	"math"
	"sync"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
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
	pts := []*sketch.Point{s.CreatePoint(0, 0), s.CreatePoint(2, 3), s.CreatePoint(5, -1), s.CreatePoint(7, 1)}
	sp, err := s.CreateFitSpline(pts...)
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
	p0 := s.CreatePoint(0, 0)
	p1 := s.CreatePoint(3, 2)
	p2 := s.CreatePoint(6, 0)
	s.Fix(p0)
	s.Fix(p2)
	sp, err := s.CreateFitSpline(p0, p1, p2)
	require.NoError(t, err)
	s.AddConstraint(sketch.NewVerticalDistance(p0, p1, 5)) // pull p1 up to y=5
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 5, p1.Y(), 1e-6, "the solver moved p1")

	coords := [][2]float64{{p0.X(), p0.Y()}, {p1.X(), p1.Y()}, {p2.X(), p2.Y()}}
	frac := chordFractions(coords)
	x, y := sp.Eval(frac[1])
	require.InDelta(t, p1.X(), x, 1e-9, "curve still passes through the moved middle fit point")
	require.InDelta(t, p1.Y(), y, 1e-9)
}

func TestFitSplineInterpolantFollowsTheSolve(t *testing.T) {
	// A consumer that integrates the exact curve reads the interpolant, so it must
	// describe the SOLVED geometry — and describe it as Eval does.
	s := newSketch(t)
	p0 := s.CreatePoint(0, 0)
	p1 := s.CreatePoint(3, 2)
	p2 := s.CreatePoint(6, 0)
	s.Fix(p0)
	s.Fix(p2)
	sp, err := s.CreateFitSpline(p0, p1, p2)
	require.NoError(t, err)
	s.AddConstraint(sketch.NewVerticalDistance(p0, p1, 5))
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	fi, err := sp.Interpolant()
	require.NoError(t, err)
	require.Len(t, fi.Points, 3)
	require.Equal(t, [2]float64{p1.X(), p1.Y()}, fi.Points[1], "the interpolant carries the solved coordinates")
	require.Len(t, fi.Spans(), 2)
	for i := 0; i <= 10; i++ {
		u := float64(i) / 10
		wantX, wantY := sp.Eval(u)
		gotX, gotY := fi.Eval(u)
		require.Equal(t, wantX, gotX, "the interpolant reproduces Eval at t=%v", u)
		require.Equal(t, wantY, gotY)
	}
}

func TestFitSplineBoundsProfile(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	m1 := s.CreatePoint(2, 3)
	m2 := s.CreatePoint(4, 3)
	b := s.CreatePoint(6, 0)
	sp, err := s.CreateFitSpline(a, m1, m2, b)
	require.NoError(t, err)
	s.CreateLine(b, a) // chord closes the loop

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "fit spline + chord bound one region")
	require.True(t, profiles[0].Valid)
	require.Greater(t, profiles[0].Area, 0.0)
	require.Contains(t, profiles[0].Entities, sketch.Entity(sp))
}

func TestFitSplineSelfCrossingInvalid(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	m1 := s.CreatePoint(4, 1)
	m2 := s.CreatePoint(0, 2)
	m3 := s.CreatePoint(4, 3)
	_, err := s.CreateFitSpline(a, m1, m2, m3)
	require.NoError(t, err)
	s.CreateLine(m3, a)

	rep := s.Verify(t.Context())
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
	a := s.CreatePoint(0, 0)
	_, err := s.CreateFitSpline(a)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "fewer than 2 fit points is rejected")
	b := s.CreatePoint(1, 0)
	_, err = s.CreateFitSpline(a, nil, b)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "a nil fit point is rejected")
}

func TestFitSplineFixEntityDOF(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	m := s.CreatePoint(3, 2)
	b := s.CreatePoint(6, 0)
	sp, err := s.CreateFitSpline(a, m, b)
	require.NoError(t, err)
	require.Equal(t, 6, s.DOF(), "3 free fit points, no size vars or internal constraints")
	s.FixEntity(sp)
	require.Equal(t, 0, s.DOF())
}

func TestFitSplineConstructionExcluded(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	m1 := s.CreatePoint(2, 3)
	m2 := s.CreatePoint(4, 3)
	b := s.CreatePoint(6, 0)
	sp, err := s.CreateFitSpline(a, m1, m2, b)
	require.NoError(t, err)
	sp.SetConstruction(true)
	s.CreateLine(b, a)
	require.Empty(t, s.Profiles(), "a construction fit spline bounds no reported profile")
}

func TestFitSplineRoundTrip(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	m1 := s.CreatePoint(2, 3)
	m2 := s.CreatePoint(4, 3)
	b := s.CreatePoint(6, 0)
	_, err := s.CreateFitSpline(a, m1, m2, b)
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
	a := s.CreatePoint(0, 0)
	m := s.CreatePoint(3, 2)
	b := s.CreatePoint(6, 0)
	_, err := s.CreateFitSpline(a, m, b)
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
	c0 := s2.CreatePoint(0, 0)
	c1 := s2.CreatePoint(0, 0) // coincident with c0
	c2 := s2.CreatePoint(4, 0)
	sp, err := s2.CreateFitSpline(c0, c1, c2)
	require.NoError(t, err)
	require.NotPanics(t, func() {
		sp.Polyline(16)
		s2.Profiles()
	})
}

func TestFitSplineForeignPointNotTrustworthy(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(4, 0)
	s.Fix(a)
	s.Fix(b)
	other := newSketch(t)
	foreign := other.CreatePoint(2, 3)
	_, err := s.CreateFitSpline(a, foreign, b)
	require.NoError(t, err)
	rep := s.Verify(t.Context())
	require.True(t, rep.ForeignHandles)
	require.False(t, rep.Trustworthy())
}

func TestFitSplineTypedNilEntity(t *testing.T) {
	s := newSketch(t)
	_, err := s.WorldPolyline((*sketch.FitSpline)(nil))
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
}

func TestFitSplineInterpolantRefusesIndescribableCurve(t *testing.T) {
	// Every coordinate here is finite and every call below returns a nil error, yet
	// the chord between the two fit points is not representable. The interpolant
	// would then describe the FIRST POINT while Eval describes the whole curve, so
	// the sketch method surfaces the refusal rather than a truncated description.
	s := newSketch(t)
	sp, err := s.CreateFitSpline(s.CreatePoint(-1e308, 0), s.CreatePoint(1e308, 1))
	require.NoError(t, err)

	x, y := sp.Eval(1)
	require.Equal(t, 1e308, x, "the spline still evaluates its own last fit point")
	require.Equal(t, 1.0, y)

	fi, err := sp.Interpolant()
	require.ErrorIs(t, err, geom.ErrNonFiniteFitInterpolant)
	require.Nil(t, fi, "a refused interpolant is not handed back at all")
}

func TestFitSplineEvaluationFollowsFitEdits(t *testing.T) {
	t.Parallel()
	s := newSketch(t)
	a, middle, b := s.CreatePoint(0, 0), s.CreatePoint(3, 2), s.CreatePoint(6, 0)
	sp, err := s.CreateFitSpline(a, middle, b)
	require.NoError(t, err)
	check := func() {
		t.Helper()
		fit := make([][2]float64, len(sp.Fit))
		for i, p := range sp.Fit {
			fit[i] = [2]float64{p.X(), p.Y()}
		}
		for i := range 17 {
			parameter := float64(i) / 16
			x, y := sp.Eval(parameter)
			wx, wy, err := geom.EvalFitSpline(fit, parameter)
			if len(fit) < 2 {
				require.ErrorIs(t, err, geom.ErrTooFewFitPoints)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, math.Float64bits(wx), math.Float64bits(x), "parameter %v x", parameter)
			require.Equal(t, math.Float64bits(wy), math.Float64bits(y), "parameter %v y", parameter)
		}
	}
	check()
	sp.Fit[0] = s.CreatePoint(math.Copysign(0, -1), 0)
	check()
	sp.Fit[0] = a
	check()
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewVerticalDistance(a, middle, 5))
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 5, middle.Y(), 1e-6)
	check()
	sp.Fit[1] = s.CreatePoint(3, -4)
	check()
	sp.Fit = append(sp.Fit, s.CreatePoint(8, 3))
	check()
	sp.Fit[0], sp.Fit[1] = sp.Fit[1], sp.Fit[0]
	check()
	sp.Fit = sp.Fit[:1]
	check()
	sp.Fit = nil
	check()
}

func TestFitSplineConcurrentEvaluation(t *testing.T) {
	t.Parallel()
	s := newSketch(t)
	sp, err := s.CreateFitSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 3), s.CreatePoint(5, -1))
	require.NoError(t, err)
	wx, wy, err := geom.EvalFitSpline([][2]float64{{0, 0}, {2, 3}, {5, -1}}, 0.375)
	require.NoError(t, err)
	var group sync.WaitGroup
	failures := make(chan int, 8)
	for worker := range 8 {
		group.Go(func() {
			for range 32 {
				x, y := sp.Eval(0.375)
				if x != wx || y != wy {
					failures <- worker
					return
				}
			}
		})
	}
	group.Wait()
	require.Empty(t, failures)
}

// nanFitPointSpline builds a fit-point spline describing a hump-shaped curve
// crossing a 10x10 square at (0,0)-(10,10), with the fit point at index nanAt
// either an ordinary finite coordinate or NaN (nanAt < 0 leaves every point
// finite). CreateFitSpline has no way to validate a NaN fit point the way
// CreateNURBS validates a knot: CreatePoint has no error return and the
// solver moves point coordinates after construction, so a construction-time
// check could never hold as a precondition. Every fit point is fixed, the
// poisoned one included, mirroring nanControlCoordinateNURBS.
func nanFitPointSpline(t *testing.T, s *sketch.Sketch, nanAt int) *sketch.FitSpline {
	t.Helper()
	pts := [][2]float64{{-2, 1}, {2, 9}, {5, 8}, {8, 9}, {12, 1}}
	fit := make([]*sketch.Point, len(pts))
	for i, p := range pts {
		x := p[0]
		if i == nanAt {
			x = math.NaN()
		}
		fit[i] = s.CreatePoint(x, p[1])
		s.Fix(fit[i])
	}
	fs, err := s.CreateFitSpline(fit...)
	require.NoError(t, err)
	return fs
}

// TestNaNFitPointSplineCrossingSquareIsInvalidProfile reproduces the
// wrong-region-count defect end to end through the public API: a fit-point
// spline with a NaN fit point was silently reinterpolated through its
// remaining (finite) points by newFitEvaluator's own coincidence filter — so
// the arrangement never saw a non-finite sample, and the crossing curve's
// profile split was reported as trustworthy despite depending on the dropped
// point. The arrangement's own refusal is asserted directly on
// [Sketch.Profiles], which runs whatever geometry it is given: Verify no
// longer reaches that pass on this fixture, since the NaN fit point is a
// solver variable the non-finite-geometry screen catches first, so the report
// half asserts the finding Verify does make.
func TestNaNFitPointSplineCrossingSquareIsInvalidProfile(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	fs := nanFitPointSpline(t, s, 2)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	profiles := s.Profiles()
	require.NotEmpty(t, profiles, "the square still bounds a region, so the check below is not vacuous")
	for _, p := range profiles {
		require.False(t, p.Valid, "the NaN fit point must not be silently dropped")
	}

	rep := s.Verify(t.Context())
	require.Equal(t, []*sketch.Point{fs.Fit[2]}, rep.NonFinitePoints,
		"the poisoned fit point is named, and only it")
	err := rep.Check()
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.ErrorIs(t, err, sketch.ErrVerificationIncomplete)
	require.NotErrorIs(t, err, sketch.ErrInvalidProfile,
		"the profile pass never ran, so it must not be asserted as a finding")
	require.False(t, rep.Trustworthy(), "non-finite geometry must never be blessed")
}

// TestFiniteFitPointSplineCrossingSquareIsTrustworthy is the healthy control
// for TestNaNFitPointSplineCrossingSquareIsInvalidProfile: the identical
// crossing curve with every fit point finite cleanly splits the square into
// two regions and is fully trustworthy.
func TestFiniteFitPointSplineCrossingSquareIsTrustworthy(t *testing.T) {
	s := newSketch(t)
	groundedSquare(t, s)
	nanFitPointSpline(t, s, -1)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Len(t, rep.Profiles, 2, "the crossing fit spline splits the square into two regions")
	require.True(t, rep.ProfilesValid)
	require.Empty(t, rep.InvalidProfiles)
	total := 0.0
	for _, p := range rep.Profiles {
		total += p.Area
	}
	require.InDelta(t, 100, total, 1e-6, "areas still sum exactly to the whole square")
	require.NoError(t, rep.Check())
	require.True(t, rep.Trustworthy(), "a clean crossing is trustworthy")
}
