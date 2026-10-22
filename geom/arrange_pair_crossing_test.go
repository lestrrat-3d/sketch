package geom_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// embeddedSections builds n copies, rotated evenly about the origin, of a closed
// region made of an inner chord, two fit splines of fitPoints points each and a
// tip arc about the origin. The splines start inside the circle of radius rootRadius
// and end outside it, so each crosses the circle once. No two copies meet.
func embeddedSections(t *testing.T, n, fitPoints int) ([]geom.Curve, []geom.ClosedCurve) {
	t.Helper()
	const (
		rootRadius = 3.0
		innerR     = 2.6
		outerR     = 4.0
	)
	pitch := math.Pi / float64(n)
	center := geom.NewPoint(0, 0)
	var curves []geom.Curve
	for k := range n {
		rot := 2 * math.Pi * float64(k) / float64(n)
		point := func(r, a float64) *geom.Point {
			return geom.NewPoint(r*math.Cos(a+rot), r*math.Sin(a+rot))
		}
		a := 0.4 * pitch
		right := make([]*geom.Point, fitPoints)
		left := make([]*geom.Point, fitPoints)
		for i := range fitPoints {
			f := float64(i) / float64(fitPoints-1)
			r := innerR + f*(outerR-innerR)
			half := a * (1 - 0.4*f)
			right[i], left[i] = point(r, -half), point(r, half)
		}
		curves = append(curves,
			geom.NewLine(left[0], right[0]),
			mustFitSpline(t, right...),
			geom.NewArc(center, right[fitPoints-1], left[fitPoints-1]),
			mustFitSpline(t, left...))
	}
	return curves, []geom.ClosedCurve{geom.NewCircle(center, rootRadius)}
}

func TestSplineCrossingCircleOncePerFlankIsExact(t *testing.T) {
	for _, n := range []int{3, 8, 24} {
		for _, fit := range []int{4, 15} {
			t.Run(fmt.Sprintf("n=%d/fit=%d", n, fit), func(t *testing.T) {
				curves, closed := embeddedSections(t, n, fit)
				arr := geom.Regions(curves, closed)
				require.False(t, arr.Degenerate)
				requireExactBoundsReproduce(t, curves, closed, arr)
				for src, ok := range exactBySource(arr) {
					require.Truef(t, ok, "source %d publishes an inexact bound", src)
				}
			})
		}
	}
}

func TestSplineCrossingCircleTwiceIsExact(t *testing.T) {
	// A circle centred on the arch cuts it at two points; the chord closes the arch.
	arch := mustFitSpline(t, geom.NewPoint(-4, 0), geom.NewPoint(0, 3), geom.NewPoint(4, 0))
	curves := []geom.Curve{arch, geom.NewLine(geom.NewPoint(4, 0), geom.NewPoint(-4, 0))}
	closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 3), 1.5)}
	arr := geom.Regions(curves, closed)
	require.False(t, arr.Degenerate)
	requireExactBoundsReproduce(t, curves, closed, arr)
	var crossings int
	for _, region := range arr.Regions {
		for _, e := range region.Outer {
			require.Truef(t, e.TExact, "source %d t=[%v %v]", e.SourceIndex, e.TStart, e.TEnd)
			if e.SourceIndex != 0 {
				continue
			}
			for _, tt := range []float64{e.TStart, e.TEnd} {
				if tt == 0 || tt == 1 {
					continue
				}
				x, y := arch.Eval(tt)
				require.InDelta(t, 1.5, math.Hypot(x, y-3), 1e-12, "a fragment bound of the spline lies on the circle")
				crossings++
			}
		}
	}
	require.Positive(t, crossings)
}
