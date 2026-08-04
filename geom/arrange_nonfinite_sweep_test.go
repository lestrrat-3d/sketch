package geom_test

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
)

// nonFiniteSweepScene builds one legitimate (all-finite-INPUT) scene of the given
// curve family at the given scale — a reduced version of the differential sweep
// that validated this file's fix (geom/densify's per-source finiteness check):
// the same investigation ran 11 families x 15 scales x 327 repetitions (53955
// scenes total) through geom.Regions on both the pre-fix and post-fix build and
// found zero differing (Degenerate, region count, total area) lines; the only
// behavioural difference the fix's new code path caused, found by instrumenting
// it directly, was 198 scenes — all NURBS at scale 1e307, where the pre-fix
// build already reported the scene Degenerate with no regions, so the fix costs
// nothing there either.
//
// This bounded fixture keeps the same family/scale shape (so it exercises the
// same variety of legitimate geometry the investigation did) but trims the
// scale list and the repetitions per combination so it runs in a fraction of a
// second rather than the ~2 minutes the full sweep takes, and asserts the
// property every one of those 53955 scenes held: geom.Regions must not panic on
// legitimate finite-input geometry, and every reported region's Area must be
// finite. This is what a future geom change breaking on such input would trip.
func nonFiniteSweepScene(family int, s float64, rng *rand.Rand) ([]geom.Curve, []geom.ClosedCurve) {
	j := func() float64 { return (rng.Float64() - 0.5) * 0.4 }
	p := func(x, y float64) *geom.Point { return geom.NewPoint(x, y) }
	switch family {
	case 0: // lines: a quadrilateral plus a crossing chord
		return []geom.Curve{
			geom.NewLine(p(0, 0), p(s, 0)),
			geom.NewLine(p(s, 0), p(s*(1+j()), s)),
			geom.NewLine(p(s*(1+j()), s), p(0, s)),
			geom.NewLine(p(0, s), p(0, 0)),
			geom.NewLine(p(-0.2*s, 0.5*s), p(1.2*s, 0.5*s*(1+j()))),
		}, nil
	case 1: // circles: two overlapping disks
		return nil, []geom.ClosedCurve{
			geom.NewCircle(p(0, 0), s*(1+j())),
			geom.NewCircle(p(s*(1+j()), 0), s),
		}
	case 2: // arcs closing a lens with lines
		a := geom.NewArc(p(0, 0), p(s, 0), p(0, s))
		b := geom.NewArc(p(s, s), p(s, 0), p(0, s))
		return []geom.Curve{a, b, geom.NewLine(p(-0.5*s, 0.5*s), p(1.5*s, 0.5*s))}, nil
	case 3: // ellipse + crossing line
		return []geom.Curve{
				geom.NewLine(p(-1.5*s, 0.2*s), p(1.5*s, 0.2*s*(1+j()))),
			}, []geom.ClosedCurve{
				geom.NewEllipse(p(0, 0), s*(1+j()), 0.6*s, 0.3),
			}
	case 4: // elliptical arc + lines closing it
		ea := geom.NewEllipticalArc(p(0, 0), p(s, 0), p(-s, 0), s, 0.5*s, 0.2)
		return []geom.Curve{ea, geom.NewLine(p(-s, 0), p(s, 0)),
			geom.NewLine(p(-0.5*s, -0.2*s), p(0.5*s, 0.4*s))}, nil
	case 5: // conic + line
		c := geom.NewConic(p(-s, 0), p(0, s), p(s, 0), 0.4+0.2*rng.Float64())
		return []geom.Curve{c, geom.NewLine(p(-s, 0), p(s, 0)),
			geom.NewLine(p(-0.6*s, 0.1*s), p(0.6*s, 0.5*s))}, nil
	case 6: // open cubic B-spline + closing line
		sp, err := geom.NewSpline(p(-s, 0), p(-0.4*s, s*(1+j())), p(0.4*s, -s), p(s, 0), p(1.4*s, 0.3*s))
		if err != nil {
			return nil, nil
		}
		return []geom.Curve{sp, geom.NewLine(p(-s, 0), p(1.4*s, 0.3*s)),
			geom.NewLine(p(-s, 0.1*s), p(1.4*s, 0.1*s))}, nil
	case 7: // closed spline
		cs, err := geom.NewClosedSpline(p(-s, -s), p(s*(1+j()), -s), p(s, s), p(-s, s*(1+j())))
		if err != nil {
			return nil, nil
		}
		return []geom.Curve{geom.NewLine(p(-2*s, 0), p(2*s, 0.1*s))},
			[]geom.ClosedCurve{cs}
	case 8: // fit spline
		fs, err := geom.NewFitSpline(p(-s, 0), p(-0.3*s, 0.6*s*(1+j())), p(0.3*s, -0.6*s), p(s, 0))
		if err != nil {
			return nil, nil
		}
		return []geom.Curve{fs, geom.NewLine(p(-s, 0), p(s, 0)),
			geom.NewLine(p(-0.5*s, 0.2*s), p(0.5*s, -0.2*s))}, nil
	case 9: // NURBS, unit weights
		ctrl := []*geom.Point{p(-s, 0), p(-0.5*s, s*(1+j())), p(0, -0.5*s), p(0.5*s, s), p(s, 0)}
		kn := geom.ClampedUniformKnots(len(ctrl), 3)
		nb := geom.NewNURBS(3, ctrl, kn, nil)
		return []geom.Curve{nb, geom.NewLine(p(-s, 0), p(s, 0)),
			geom.NewLine(p(-0.7*s, 0.2*s), p(0.7*s, 0.2*s))}, nil
	case 10: // NURBS, wide legitimate weight range 1e-6 .. 1e6
		ctrl := []*geom.Point{p(-s, 0), p(-0.5*s, s), p(0, -0.5*s), p(0.5*s, s), p(s, 0)}
		kn := geom.ClampedUniformKnots(len(ctrl), 3)
		w := []float64{1, math.Pow(10, -6+12*rng.Float64()), 1, math.Pow(10, -6+12*rng.Float64()), 1}
		nb := geom.NewNURBS(3, ctrl, kn, w)
		return []geom.Curve{nb, geom.NewLine(p(-s, 0), p(s, 0))}, nil
	}
	return nil, nil
}

// TestNonFiniteFixSweepDoesNotBreakLegitimateGeometry is the bounded, checked-in
// fixture: 11 families x 5 scales (spanning the same 1e-12..1e307 range the full
// investigation swept) x 5 repetitions = 275 scenes, chosen to run in well under
// a second while still touching every curve family and both scale extremes.
func TestNonFiniteFixSweepDoesNotBreakLegitimateGeometry(t *testing.T) {
	scales := []float64{1e-12, 1e-3, 1, 1e6, 1e307}
	const perCombo = 5
	total := 0
	for family := 0; family < 11; family++ {
		for si, s := range scales {
			rng := rand.New(rand.NewSource(int64(family*1000 + si)))
			for k := 0; k < perCombo; k++ {
				curves, closed := nonFiniteSweepScene(family, s, rng)
				if curves == nil && closed == nil {
					continue
				}
				name := fmt.Sprintf("family=%d/scale=%g/rep=%d", family, s, k)
				t.Run(name, func(t *testing.T) {
					arr := geom.Regions(curves, closed) // must not panic
					for i, r := range arr.Regions {
						if math.IsNaN(r.Area) || math.IsInf(r.Area, 0) {
							t.Fatalf("region %d has a non-finite area: %v", i, r.Area)
						}
					}
				})
				total++
			}
		}
	}
	if total == 0 {
		t.Fatal("the fixture produced no scenes")
	}
}
