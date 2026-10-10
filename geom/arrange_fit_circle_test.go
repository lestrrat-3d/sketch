package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestFitSplineCircleCrossingCertifiesOriginalFlanks(t *testing.T) {
	t.Parallel()
	const teeth, samples = 30, 5
	const module, pressure = 0.5, math.Pi / 6
	pitch := module * teeth / 2
	base, root, tip := pitch*math.Cos(pressure), (module*teeth-2.5*module)/2, (module*teeth+2*module)/2
	flank := func(radius float64) (float64, float64) {
		a := math.Acos(base / radius)
		v := math.Tan(a)
		return base * (math.Cos(v) + v*math.Sin(v)), base * (math.Sin(v) - v*math.Cos(v))
	}
	px, py := flank(pitch)
	rotation := math.Pi/(2*teeth) - math.Atan2(-py, px)
	left, right := make([]*geom.Point, samples), make([]*geom.Point, samples)
	for i := range left {
		radius := base + (tip-base)*float64(i)/float64(samples-1)
		x, y := flank(radius)
		xl, yl := x*math.Cos(rotation)+y*math.Sin(rotation), x*math.Sin(rotation)-y*math.Cos(rotation)
		left[i], right[i] = geom.NewPoint(xl, yl), geom.NewPoint(xl, -yl)
	}
	rightFit, err := geom.NewFitSpline(right...)
	require.NoError(t, err)
	leftFit, err := geom.NewFitSpline(left...)
	require.NoError(t, err)
	center := geom.NewPoint(0, 0)
	curves := []geom.Curve{rightFit, geom.NewArc(center, right[samples-1], left[samples-1]), leftFit}
	closed := []geom.ClosedCurve{geom.NewCircle(center, root)}
	arr := geom.Regions(curves, closed)
	requireExactBoundsReproduce(t, curves, closed, arr)
	var tooth *geom.Region
	for _, region := range arr.Regions {
		if len(region.Outer) == 5 {
			tooth = region
		}
	}
	require.NotNil(t, tooth)
	var flanks int
	for _, edge := range tooth.Outer {
		if edge.SourceIndex != 0 && edge.SourceIndex != 2 {
			continue
		}
		flanks++
		require.True(t, edge.TExact)
		require.Greater(t, edge.TStart, 0.0)
		require.Equal(t, 1.0, edge.TEnd)
		fit := curves[edge.SourceIndex].(*geom.FitSpline)
		x, y := fit.Eval(edge.TStart)
		require.InDelta(t, root, math.Hypot(x, y), 1e-12)
	}
	require.Equal(t, 2, flanks)

	// An extra source falls outside the four-source certificate.
	extra := append(append([]geom.Curve{}, curves...), geom.NewLine(geom.NewPoint(20, 20), geom.NewPoint(21, 20)))
	uncertified := geom.Regions(extra, closed)
	for _, region := range uncertified.Regions {
		for _, edge := range region.Outer {
			require.False(t, edge.TExact)
		}
	}
}
