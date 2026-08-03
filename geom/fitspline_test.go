package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestRegionsFitSplineWithChord(t *testing.T) {
	a := geom.NewPoint(0, 0)
	m1 := geom.NewPoint(2, 3)
	m2 := geom.NewPoint(4, 3)
	b := geom.NewPoint(6, 0)
	fs, err := geom.NewFitSpline(a, m1, m2, b)
	require.NoError(t, err)
	chord := geom.NewLine(b, a)
	arr := geom.Regions([]geom.Curve{fs, chord}, nil)
	require.Len(t, arr.Regions, 1, "fit spline + chord bound one region")
	require.Greater(t, arr.Regions[0].Area, 0.0)
	require.False(t, arr.Regions[0].SelfIntersecting)
	require.Empty(t, arr.SelfIntersections)
}

func TestRegionsFitSplineSelfCrossing(t *testing.T) {
	// Fit points whose natural-cubic interpolant loops across itself, closed by a chord.
	a := geom.NewPoint(0, 0)
	m1 := geom.NewPoint(4, 1)
	m2 := geom.NewPoint(0, 2)
	m3 := geom.NewPoint(4, 3)
	fs, err := geom.NewFitSpline(a, m1, m2, m3)
	require.NoError(t, err)
	chord := geom.NewLine(m3, a)
	arr := geom.Regions([]geom.Curve{fs, chord}, nil)
	require.NotEmpty(t, arr.SelfIntersections, "the interpolant weaves across itself")
}

func TestFitSplineInterpolatesEval(t *testing.T) {
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1), geom.NewPoint(5, 1), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	var cum []float64
	var total float64
	cum = append(cum, 0)
	for i := 1; i < len(fit); i++ {
		total += math.Hypot(fit[i].X-fit[i-1].X, fit[i].Y-fit[i-1].Y)
		cum = append(cum, total)
	}
	for i, p := range fit {
		x, y := sp.Eval(cum[i] / total)
		require.InDelta(t, p.X, x, 1e-9, "interpolates fit point %d", i)
		require.InDelta(t, p.Y, y, 1e-9)
	}
}

func TestFitSplineTwoPointsIsLine(t *testing.T) {
	sp, err := geom.NewFitSpline(geom.NewPoint(0, 0), geom.NewPoint(6, 3))
	require.NoError(t, err)
	x, y := sp.Eval(0.5)
	require.InDelta(t, 3, x, 1e-9, "two fit points evaluate as a straight line")
	require.InDelta(t, 1.5, y, 1e-9)
}

func TestNewFitSplineMinTwo(t *testing.T) {
	_, err := geom.NewFitSpline(geom.NewPoint(0, 0))
	require.ErrorIs(t, err, geom.ErrTooFewFitPoints)
}

// evalFitSpan evaluates one monomial span at chord parameter p.
func evalFitSpan(sp geom.FitSpan, p float64) (float64, float64) {
	u := p - sp.PStart
	x := sp.X[0] + u*(sp.X[1]+u*(sp.X[2]+u*sp.X[3]))
	y := sp.Y[0] + u*(sp.Y[1]+u*(sp.Y[2]+u*sp.Y[3]))
	return x, y
}

func TestFitInterpolantDescribesTheEvaluatedCurve(t *testing.T) {
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1), geom.NewPoint(5, 1), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	fi := sp.Interpolant()

	require.Len(t, fi.Points, len(fit), "no fit point coincides, so none collapses")
	require.Len(t, fi.Params, len(fit))
	require.Len(t, fi.SecondDerivs, len(fit))
	require.Equal(t, 0.0, fi.Params[0], "the chord parameter starts at zero")
	for i, p := range fit {
		require.Equal(t, [2]float64{p.X, p.Y}, fi.Points[i], "point %d is the fit point itself", i)
		if i > 0 {
			require.Greater(t, fi.Params[i], fi.Params[i-1], "chord parameters increase")
			require.InDelta(t, math.Hypot(p.X-fit[i-1].X, p.Y-fit[i-1].Y), fi.Params[i]-fi.Params[i-1], 1e-12,
				"span %d spans its own chord length", i-1)
		}
	}
	require.Equal(t, [2]float64{0, 0}, fi.SecondDerivs[0], "natural end condition")
	require.Equal(t, [2]float64{0, 0}, fi.SecondDerivs[len(fi.SecondDerivs)-1])

	for i := 0; i <= 20; i++ {
		u := float64(i) / 20
		wantX, wantY := sp.Eval(u)
		gotX, gotY := fi.Eval(u)
		require.Equal(t, wantX, gotX, "the interpolant reproduces Eval bit for bit at t=%v", u)
		require.Equal(t, wantY, gotY)

		coords := [][2]float64{{fit[0].X, fit[0].Y}}
		for _, p := range fit[1:] {
			coords = append(coords, [2]float64{p.X, p.Y})
		}
		wantDX, wantDY, err := geom.EvalFitSplineDeriv(coords, u)
		require.NoError(t, err)
		gotDX, gotDY := fi.EvalDeriv(u)
		require.Equal(t, wantDX, gotDX, "and reproduces the tangent at t=%v", u)
		require.Equal(t, wantDY, gotDY)
	}
}

func TestFitInterpolantSpansReconstructCurve(t *testing.T) {
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1), geom.NewPoint(5, 1), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	fi := sp.Interpolant()
	spans := fi.Spans()
	require.Len(t, spans, len(fit)-1, "one cubic piece per interval")

	total := fi.Params[len(fi.Params)-1]
	require.Equal(t, 0.0, spans[0].TStart, "the normalized spans cover [0,1]")
	require.Equal(t, 1.0, spans[len(spans)-1].TEnd)
	for i, s := range spans {
		require.Equal(t, fi.Params[i], s.PStart)
		require.Equal(t, fi.Params[i+1], s.PEnd)
		require.InDelta(t, s.PStart/total, s.TStart, 1e-15, "span %d normalizes by the total chord length", i)
		if i > 0 {
			require.Equal(t, spans[i-1].TEnd, s.TStart, "spans meet at span %d", i)
		}
		// The polynomial reproduces the curve across its own span, endpoints included.
		for k := 0; k <= 8; k++ {
			p := s.PStart + (s.PEnd-s.PStart)*float64(k)/8
			wantX, wantY := sp.Eval(p / total)
			gotX, gotY := evalFitSpan(s, p)
			require.InDelta(t, wantX, gotX, 1e-9, "span %d at p=%v", i, p)
			require.InDelta(t, wantY, gotY, 1e-9)
		}
	}
}

func TestFitInterpolantCollapsesCoincidentFitPoints(t *testing.T) {
	// The exported spans must describe the curve actually evaluated, so a repeated
	// fit point (a zero-length chord) is absent from them.
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(2, 3), geom.NewPoint(2, 3), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	fi := sp.Interpolant()
	require.Len(t, fi.Points, 3, "the repeated fit point collapses away")
	require.Equal(t, [2]float64{2, 3}, fi.Points[1])
	require.Len(t, fi.Spans(), 2)
}

func TestFitInterpolantAllCoincidentIsOnePoint(t *testing.T) {
	fi, err := geom.NewFitInterpolant([][2]float64{{4, 5}, {4, 5}, {4, 5}})
	require.NoError(t, err)
	require.Equal(t, [][2]float64{{4, 5}}, fi.Points)
	require.Empty(t, fi.Spans(), "a single point spans nothing")
	x, y := fi.Eval(0.5)
	require.Equal(t, 4.0, x)
	require.Equal(t, 5.0, y)
	dx, dy := fi.EvalDeriv(0.5)
	require.Equal(t, 0.0, dx, "a degenerate interpolant has no tangent")
	require.Equal(t, 0.0, dy)
}

func TestFitInterpolantEmpty(t *testing.T) {
	_, err := geom.NewFitInterpolant([][2]float64{{0, 0}})
	require.ErrorIs(t, err, geom.ErrTooFewFitPoints)

	empty := &geom.FitInterpolant{}
	x, y := empty.Eval(0.5)
	require.Equal(t, 0.0, x, "an empty interpolant evaluates without panicking")
	require.Equal(t, 0.0, y)
	require.Empty(t, empty.Spans())
	require.Empty(t, (&geom.FitSpline{}).Interpolant().Points)
}
