package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// The pair ledger lifts the free-form kind gate for fit splines whose every
// relation to the other sources is proven. These tests pin what that lifts and what
// stays withheld (docs/pair-certificate-design.md).

func TestPairLedgerFitArchClosedByChordIsExact(t *testing.T) {
	arch := mustFitSpline(t, geom.NewPoint(-4, 0), geom.NewPoint(0, 3), geom.NewPoint(4, 0))
	curves := []geom.Curve{arch, geom.NewLine(geom.NewPoint(4, 0), geom.NewPoint(-4, 0))}
	arr := geom.Regions(curves, nil)
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 1)
	requireExactBoundsReproduce(t, curves, nil, arr)
	for src, ok := range exactBySource(arr) {
		require.Truef(t, ok, "source %d: the spline and the chord share only their two ends", src)
	}
}

func TestPairLedgerParkedFitSplineKeepsCirclesExact(t *testing.T) {
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 5),
		geom.NewCircle(geom.NewPoint(-6, 0), 5),
	}
	curves := []geom.Curve{mustFitSpline(t, geom.NewPoint(36, 0), geom.NewPoint(40, 3), geom.NewPoint(44, 0))}
	arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(256))
	require.False(t, arr.Degenerate)
	requireExactBoundsReproduce(t, curves, closed, arr)
	for src, ok := range exactBySource(arr) {
		require.Truef(t, ok, "source %d: the parked spline provably touches nothing", src)
	}
}

func TestPairLedgerCrossingWithholdsItsComponentOnly(t *testing.T) {
	// A circle touching the arch's apex is not certified, so the pair's component
	// publishes no exact bound. An ordinary circle crossing 100 units away keeps its own.
	arch := mustFitSpline(t, geom.NewPoint(-4, 0), geom.NewPoint(0, 3), geom.NewPoint(4, 0))
	curves := []geom.Curve{arch, geom.NewLine(geom.NewPoint(4, 0), geom.NewPoint(-4, 0))}
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 4.5), 1.5),
		geom.NewCircle(geom.NewPoint(100, 0), 5),
		geom.NewCircle(geom.NewPoint(106, 0), 5),
	}
	arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(256))
	requireExactBoundsReproduce(t, curves, closed, arr)
	exact := exactBySource(arr)
	// sources: 0 arch, 1 chord, 2 circle on the arch, 3 and 4 the distant pair
	for _, src := range []int{0, 1, 2} {
		if v, ok := exact[src]; ok {
			require.Falsef(t, v, "source %d is in the component of the unproven crossing", src)
		}
	}
	for _, src := range []int{3, 4} {
		require.Truef(t, exact[src], "source %d is 100 units from the unproven crossing", src)
	}
}

func TestPairLedgerTouchingWithoutSharedEndIsNotExact(t *testing.T) {
	// A line standing on the arch's interior touches it at a point that is no end of
	// the arch. The certificate does not cover that contact, so nothing is exact.
	arch := mustFitSpline(t, geom.NewPoint(-4, 0), geom.NewPoint(0, 3), geom.NewPoint(4, 0))
	curves := []geom.Curve{
		arch,
		geom.NewLine(geom.NewPoint(4, 0), geom.NewPoint(-4, 0)),
		geom.NewLine(geom.NewPoint(0, 3), geom.NewPoint(0, 0)),
	}
	arr := geom.Regions(curves, nil)
	requireExactBoundsReproduce(t, curves, nil, arr)
	for src, ok := range exactBySource(arr) {
		require.Falsef(t, ok, "source %d is in the component of the unproven contact", src)
	}
}

func TestPairLedgerVerdictDoesNotDependOnAuthoringOrder(t *testing.T) {
	build := func(order []int) []geom.Curve {
		const n = 5
		var all []geom.Curve
		for k := range n {
			rot := 2 * math.Pi * float64(k) / n
			pt := func(r, a float64) *geom.Point {
				return geom.NewPoint(r*math.Cos(a+rot), r*math.Sin(a+rot))
			}
			a := 0.3
			rootR, rootL := pt(3, -a), pt(3, a)
			r0, r1, r2 := pt(3.4, -a), pt(3.7, -0.8*a), pt(4, -0.6*a)
			l0, l1, l2 := pt(3.4, a), pt(3.7, 0.8*a), pt(4, 0.6*a)
			all = append(all,
				geom.NewLine(rootR, r0), mustFitSpline(t, r0, r1, r2),
				geom.NewArc(geom.NewPoint(0, 0), r2, l2), mustFitSpline(t, l0, l1, l2),
				geom.NewLine(l0, rootL))
		}
		out := make([]geom.Curve, len(all))
		for i, j := range order {
			out[i] = all[j]
		}
		return out
	}
	identity := make([]int, 25)
	reversed := make([]int, 25)
	for i := range identity {
		identity[i], reversed[i] = i, 24-i
	}
	for name, order := range map[string][]int{"authored": identity, "reversed": reversed} {
		t.Run(name, func(t *testing.T) {
			closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), 3)}
			curves := build(order)
			arr := geom.Regions(curves, closed)
			requireExactBoundsReproduce(t, curves, closed, arr)
			for src, ok := range exactBySource(arr) {
				require.Truef(t, ok, "source %d", src)
			}
		})
	}
}
