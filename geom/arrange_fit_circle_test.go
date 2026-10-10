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

func TestFitSplineCircleCrossingCertifiesCompleteGear(t *testing.T) {
	for _, tc := range []struct {
		name         string
		teeth, steps int
		pressure     float64
	}{
		{name: "30 teeth and five fit points", teeth: 30, steps: 5, pressure: math.Pi / 6},
		{name: "60 teeth and fifteen fit points", teeth: 60, steps: 15, pressure: math.Pi / 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const module = 0.5
			pitch := module * float64(tc.teeth) / 2
			base := pitch * math.Cos(tc.pressure)
			root := (module*float64(tc.teeth) - 2.5*module) / 2
			tip := (module*float64(tc.teeth) + 2*module) / 2
			flank := func(radius float64) (float64, float64) {
				a := math.Acos(base / radius)
				v := math.Tan(a)
				return base * (math.Cos(v) + v*math.Sin(v)), base * (math.Sin(v) - v*math.Cos(v))
			}
			px, py := flank(pitch)
			rotation := math.Pi/(2*float64(tc.teeth)) - math.Atan2(-py, px)
			center := geom.NewPoint(0, 0)
			curves := make([]geom.Curve, 0, 3*tc.teeth)
			for tooth := 0; tooth < tc.teeth; tooth++ {
				angle := 2 * math.Pi * float64(tooth) / float64(tc.teeth)
				left, right := make([]*geom.Point, tc.steps), make([]*geom.Point, tc.steps)
				for i := range left {
					radius := base + (tip-base)*float64(i)/float64(tc.steps-1)
					x, y := flank(radius)
					xl, yl := x*math.Cos(rotation)+y*math.Sin(rotation), x*math.Sin(rotation)-y*math.Cos(rotation)
					lx, ly := xl*math.Cos(angle)-yl*math.Sin(angle), xl*math.Sin(angle)+yl*math.Cos(angle)
					rx, ry := xl*math.Cos(angle)+yl*math.Sin(angle), xl*math.Sin(angle)-yl*math.Cos(angle)
					left[i], right[i] = geom.NewPoint(lx, ly), geom.NewPoint(rx, ry)
				}
				rightFit, err := geom.NewFitSpline(right...)
				require.NoError(t, err)
				leftFit, err := geom.NewFitSpline(left...)
				require.NoError(t, err)
				curves = append(curves, rightFit, geom.NewArc(center, right[tc.steps-1], left[tc.steps-1]), leftFit)
			}
			arr := geom.Regions(curves, []geom.ClosedCurve{geom.NewCircle(center, root)})
			require.False(t, arr.Degenerate)
			seen := make(map[int]struct{})
			innerInexact := false
			for _, region := range arr.Regions {
				for _, edge := range region.Outer {
					if edge.SourceIndex >= len(curves) || edge.SourceIndex%3 == 1 || edge.Whole {
						continue
					}
					if edge.TStart == 0 || edge.TEnd != 1 {
						if !edge.TExact {
							innerInexact = true
						}
						continue
					}
					require.True(t, edge.TExact, "outer tooth source %d", edge.SourceIndex)
					fit := curves[edge.SourceIndex].(*geom.FitSpline)
					x, y := fit.Eval(edge.TStart)
					require.InDelta(t, root, math.Hypot(x, y), 1e-10)
					seen[edge.SourceIndex] = struct{}{}
				}
			}
			require.Len(t, seen, 2*tc.teeth)
			if tc.teeth == 30 {
				require.True(t, innerInexact, "the inner flank contacts must not gain exact bounds")
			}
		})
	}
}
