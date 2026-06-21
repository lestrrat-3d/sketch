package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// Increment 2 of the analytic arrangement: line/circle/arc source pairs are now
// classified analytically (the kernel is authoritative), so the oracle no longer
// false-flags clean tangencies and shallow crossings as Degenerate, and exact cuts
// make circle/arc splits sampling-independent.

func TestAnalyticShallowCrossingNotDegenerate(t *testing.T) {
	// Two lines crossing at a tiny angle (sin ≈ 5e-4, below the old p.sin<1e-3
	// heuristic) is a clean transverse crossing — the analytic verdict keeps it from
	// being falsely flagged degenerate.
	l1 := geom.NewLine(geom.NewPoint(-10, 0), geom.NewPoint(10, 0))
	l2 := geom.NewLine(geom.NewPoint(-10, -0.005), geom.NewPoint(10, 0.005))
	arr := geom.Regions([]geom.Curve{l1, l2}, nil)
	require.False(t, arr.Degenerate, "a clean shallow crossing is not degenerate")
}

func TestAnalyticCircleChordSamplingStable(t *testing.T) {
	// A chord through a circle splits it into two regions whose areas sum to the
	// exact disk area, INDEPENDENT of sampling density — analytic cuts land the split
	// vertices on the exact intersection points.
	for _, spt := range []int{8, 64, 256} {
		arr := geom.Regions(
			[]geom.Curve{geom.NewLine(geom.NewPoint(-8, 2), geom.NewPoint(8, 2))},
			[]geom.ClosedCurve{&geom.Circle{Center: geom.NewPoint(0, 0), Radius: 5}},
			geom.WithSegmentsPerTurn(spt),
		)
		require.Falsef(t, arr.Degenerate, "spt=%d", spt)
		require.Lenf(t, arr.Regions, 2, "cap + major region at spt=%d", spt)
		var total float64
		for _, r := range arr.Regions {
			total += r.Area
		}
		require.InDeltaf(t, math.Pi*25, total, 1e-9, "regions partition the disk exactly at spt=%d", spt)
	}
}

func TestAnalyticTangentLineCircleClean(t *testing.T) {
	// A line tangent to a circle is a contact, not a crossing: the line is a dangling
	// spur (pruned away), the circle bounds one disk, and nothing is degenerate.
	arr := geom.Regions(
		[]geom.Curve{geom.NewLine(geom.NewPoint(-8, 5), geom.NewPoint(8, 5))},
		[]geom.ClosedCurve{&geom.Circle{Center: geom.NewPoint(0, 0), Radius: 5}},
	)
	require.False(t, arr.Degenerate, "a tangent line is a clean contact")
	require.Len(t, arr.Regions, 1, "just the disk")
	require.InDelta(t, math.Pi*25, arr.Regions[0].Area, 1e-9)
}

func TestAnalyticTangentCirclesNonMergedClean(t *testing.T) {
	// Two externally tangent circles whose contact (1,1) falls BETWEEN sample
	// vertices (segsPerTurn=17 places no vertex at the 45° contact) stay two clean
	// disk regions — the contact is a topologically invisible touch, not a crossing.
	r := math.Sqrt2
	arr := geom.Regions(nil, []geom.ClosedCurve{
		&geom.Circle{Center: geom.NewPoint(0, 0), Radius: r},
		&geom.Circle{Center: geom.NewPoint(2, 2), Radius: r},
	}, geom.WithSegmentsPerTurn(17))
	require.False(t, arr.Degenerate, "non-merged tangent circles are clean")
	require.Len(t, arr.Regions, 2, "two disks")
	var total float64
	for _, rg := range arr.Regions {
		total += rg.Area
	}
	require.InDelta(t, 2*math.Pi*r*r, total, 1e-9, "two full disks")
}

func TestAnalyticShallowSecantNeverBlessedWrong(t *testing.T) {
	// A near-tangent line-circle secant whose cap falls within a single chord
	// segment cannot be hosted by the coarse sampled map: injecting the exact
	// crossings there once vanished the whole disk (regions=0) yet blessed it
	// Degenerate=false. The soundness invariant: at ANY sampling the arrangement
	// is either correct (two regions partitioning the disk) or Degenerate — never
	// a blessed wrong/empty topology.
	const R = 5.0
	for _, y := range []float64{4.5, 4.9, 4.99, 4.999} {
		for spt := 3; spt <= 120; spt++ {
			arr := geom.Regions(
				[]geom.Curve{geom.NewLine(geom.NewPoint(-8, y), geom.NewPoint(8, y))},
				[]geom.ClosedCurve{&geom.Circle{Center: geom.NewPoint(0, 0), Radius: R}},
				geom.WithSegmentsPerTurn(spt),
			)
			if arr.Degenerate {
				continue // conservatively rejected — sound
			}
			var total float64
			for _, rg := range arr.Regions {
				total += rg.Area
			}
			// Region COUNT is the strict soundness invariant (a vanished disk was
			// regions=0); the net area is asserted only to sampled-curve precision —
			// the exact circular-segment correction can wobble ~1e-6 when a cut lands
			// against a sample vertex, far below any real topology error (~O(cap)).
			require.Lenf(t, arr.Regions, 2, "blessed y=%.4f spt=%d must split the disk in two", y, spt)
			require.InDeltaf(t, math.Pi*R*R, total, 1e-5, "blessed y=%.4f spt=%d must partition the disk", y, spt)
		}
	}
}

func TestAnalyticInternalTangentNeverBlessedWrong(t *testing.T) {
	// Internally tangent circles (inner disk a hole in the outer): a blessed
	// result must net to the outer disk area; otherwise it must be Degenerate.
	// Coarse sampling (the inner circle a triangle) once blessed a wrong total
	// (inner counted as a separate disk). The count-consistency gate now flags
	// these conservatively (the near-tangent sampled polygons cross transversally
	// near the contact, disagreeing with the analytic tangency), so in practice
	// every sampling is Degenerate — but the invariant asserted is the general one:
	// blessed ⇒ correct net area.
	const R, r = 3.0, 1.0
	for _, ang := range []float64{0.13, 0.41, math.Pi / 4, 1.1, 2.3} {
		cx, cy := (R-r)*math.Cos(ang), (R-r)*math.Sin(ang) // inner center: contact lies on the outer circle
		for spt := 3; spt <= 120; spt++ {
			arr := geom.Regions(nil, []geom.ClosedCurve{
				&geom.Circle{Center: geom.NewPoint(0, 0), Radius: R},
				&geom.Circle{Center: geom.NewPoint(cx, cy), Radius: r},
			}, geom.WithSegmentsPerTurn(spt))
			if arr.Degenerate {
				continue
			}
			var total float64
			for _, rg := range arr.Regions {
				total += rg.Area
			}
			require.InDeltaf(t, math.Pi*R*R, total, 1e-5, "blessed internal tangent ang=%.3f spt=%d nets the outer disk", ang, spt)
		}
	}
}

func TestAnalyticCircleCircleSecantDeferredToSampled(t *testing.T) {
	// Curve/curve TRANSVERSE crossings are deferred to the sampled path: injecting
	// exact cuts into a coarse curved map is unsound (round-2: two equal-count coarse
	// crossings at the wrong locations fused three regions into one, regions=1) or
	// over-conservative (a sampled crossing one chord segment off the analytic param
	// false-flagged well-separated valid crossings) until increment 3's tangent-port
	// certificate. The sampled DCEL already resolves circle/circle topology correctly,
	// so deferring is both sound and non-conservative.
	//
	// The exact round-2 geometry (a near-internal pair that exact-cut injection fused
	// to regions=1) is now blessed with the CORRECT three regions across sampling:
	for _, spt := range []int{3, 4, 5, 6, 7, 8, 16, 32, 64} {
		arr := geom.Regions(nil, []geom.ClosedCurve{
			geom.NewCircle(geom.NewPoint(0, 0), 5),
			geom.NewCircle(geom.NewPoint(1.9088280743172588, 0.8754286851013426), 3),
		}, geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, arr.Degenerate, "round-2 pair is a clean transverse crossing at spt=%d", spt)
		require.Lenf(t, arr.Regions, 3, "round-2 pair is two lune caps + lens, never a fused blob, at spt=%d", spt)
	}

	// Across the transverse band, at adequate sampling (spt>=8 resolves even a thin
	// lens), a blessed pair is the correct three regions netting ~the union area
	// (sampled, so only sanity-bounded). Coarser spt is the sampled path's domain and
	// may merge a sub-resolution lens — a pre-existing limitation, not the injection
	// bug, lifted when increment 3 makes curve/curve crossings analytic.
	const R, r = 5.0, 3.0
	for _, ang := range []float64{0.0, 0.45, math.Pi / 4, 1.3, 2.6} {
		for _, d := range []float64{2.5, 3.0, 4.0, 5.0, 6.5} { // transverse: |R-r|=2 < d < R+r=8
			cx, cy := d*math.Cos(ang), d*math.Sin(ang)
			a1 := R * R * math.Acos((d*d+R*R-r*r)/(2*d*R))
			a2 := r * r * math.Acos((d*d+r*r-R*R)/(2*d*r))
			a3 := 0.5 * math.Sqrt((-d+R+r)*(d+R-r)*(d-R+r)*(d+R+r))
			union := math.Pi*(R*R+r*r) - (a1 + a2 - a3)
			for spt := 8; spt <= 120; spt++ {
				arr := geom.Regions(nil, []geom.ClosedCurve{
					geom.NewCircle(geom.NewPoint(0, 0), R),
					geom.NewCircle(geom.NewPoint(cx, cy), r),
				}, geom.WithSegmentsPerTurn(spt))
				if arr.Degenerate {
					continue
				}
				var total float64
				for _, rg := range arr.Regions {
					total += rg.Area
				}
				require.Lenf(t, arr.Regions, 3, "blessed circle pair ang=%.3f d=%.1f spt=%d is two lune caps + lens", ang, d, spt)
				require.InDeltaf(t, union, total, 0.1*union, "blessed circle pair ang=%.3f d=%.1f spt=%d nets ~the union area (sampled)", ang, d, spt)
			}
		}
	}
}

func TestAnalyticSameCarrierArcs(t *testing.T) {
	// Two arcs on the SAME carrier circle. The analytic overlap classification is
	// extent-aware: coincident carriers are a degenerate overlap only where their
	// swept arcs actually coincide. Disjoint or endpoint-only sweeps are clean — a
	// regression guard, since a carrier-only (extent-blind) overlap once flagged any
	// same-circle arc pair Degenerate.
	c := geom.NewPoint(0, 0)
	at := func(ang float64) *geom.Point { return geom.NewPoint(5*math.Cos(ang), 5*math.Sin(ang)) }

	disjoint := geom.Regions([]geom.Curve{
		geom.NewArc(c, at(0), at(math.Pi/4)),
		geom.NewArc(c, at(math.Pi/2), at(math.Pi)),
	}, nil, geom.WithSegmentsPerTurn(32))
	require.False(t, disjoint.Degenerate, "disjoint same-carrier arcs are clean")

	endpoint := geom.Regions([]geom.Curve{
		geom.NewArc(c, at(0), at(math.Pi/2)),
		geom.NewArc(c, at(math.Pi/2), at(math.Pi)),
	}, nil, geom.WithSegmentsPerTurn(32))
	require.False(t, endpoint.Degenerate, "same-carrier arcs sharing only an endpoint are a clean join")

	overlapping := geom.Regions([]geom.Curve{
		geom.NewArc(c, at(0), at(math.Pi)),
		geom.NewArc(c, at(math.Pi/2), at(3*math.Pi/2)),
	}, nil, geom.WithSegmentsPerTurn(32))
	require.True(t, overlapping.Degenerate, "same-carrier arcs with overlapping sweeps are coincident geometry")
}

func TestAnalyticMergedExternalTangentBlessed(t *testing.T) {
	// The tangent contact canonicalizes onto a shared sample vertex of both
	// cycle-bearing circles (here (3,0), a cardinal sample point of both). Chord
	// ordering would branch-swap there; exact tangent-port ordering (increment 3)
	// separates the two loops by opposite curvature sign, so this is now blessed as
	// two clean disks at every sampling density — not conservatively degenerate.
	for _, spt := range []int{8, 16, 32, 64, 128} {
		arr := geom.Regions(nil, []geom.ClosedCurve{
			geom.NewCircle(geom.NewPoint(0, 0), 3),
			geom.NewCircle(geom.NewPoint(6, 0), 3),
		}, geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, arr.Degenerate, "merged external tangency is certified clean at spt=%d", spt)
		require.Lenf(t, arr.Regions, 2, "two disks at spt=%d", spt)
		var total float64
		for _, rg := range arr.Regions {
			total += rg.Area
		}
		require.InDeltaf(t, 2*math.Pi*9, total, 1e-9, "two full disks (exact) at spt=%d", spt)
	}
}

func TestAnalyticMergedInternalTangentDegenerate(t *testing.T) {
	// Internal (containment) tangency at a shared vertex stays conservatively
	// degenerate: exact tangent-port ordering separates the loops, but the
	// inner-as-hole assignment is not yet certified (a later increment).
	arr := geom.Regions(nil, []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 6),
		geom.NewCircle(geom.NewPoint(3, 0), 3), // internally tangent at (6,0), a shared cardinal vertex
	}, geom.WithSegmentsPerTurn(32))
	require.True(t, arr.Degenerate, "merged internal tangency stays conservatively degenerate")
}
