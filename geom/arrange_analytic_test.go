package geom_test

import (
	"math"
	"sort"
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
		curves := []geom.Curve{geom.NewLine(geom.NewPoint(-8, 2), geom.NewPoint(8, 2))}
		closed := []geom.ClosedCurve{&geom.Circle{Center: geom.NewPoint(0, 0), Radius: 5}}
		arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
		requireExactBoundsReproduce(t, curves, closed, arr)
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

// twoCircleUnionArea is the closed-form area of the union of two transversally
// crossing circles — the exact answer a certified curve/curve crossing must reproduce.
func twoCircleUnionArea(R, r, d float64) float64 {
	a1 := R * R * math.Acos((d*d+R*R-r*r)/(2*d*R))
	a2 := r * r * math.Acos((d*d+r*r-R*R)/(2*d*r))
	a3 := 0.5 * math.Sqrt((-d+R+r)*(d+R-r)*(d-R+r)*(d+R+r))
	return math.Pi*(R*R+r*r) - (a1 + a2 - a3)
}

// arrangementArea sums an arrangement's net region areas and reports whether EVERY
// emitted boundary fragment carries an exact parameter range, and whether any
// fragment is a PARTIAL curve at all.
//
// Both are needed to recognise a certified crossing: an arrangement whose curves were
// never cut reports every fragment exact trivially (a whole curve's bounds are its own
// domain ends), so "all exact" alone would also match a sampling too coarse to find
// the crossing — the sampled fallback, not the analytic path.
func arrangementArea(arr *geom.Arrangement) (float64, bool, bool) {
	total, exact, partial := 0.0, true, false
	note := func(e geom.BoundaryEdge) {
		exact = exact && e.TExact
		partial = partial || !e.Whole
	}
	for _, rg := range arr.Regions {
		total += rg.Area
		for _, e := range rg.Outer {
			note(e)
		}
		for _, h := range rg.Holes {
			for _, e := range h {
				note(e)
			}
		}
	}
	return total, exact, partial
}

func TestAnalyticCircleCircleCrossingCertified(t *testing.T) {
	// SUPERSEDED EXPECTATION. This test was TestAnalyticCircleCircleSecantDeferredToSampled
	// and asserted that a curve/curve transverse crossing is ALWAYS resolved on the
	// sampled path. A curve/curve crossing now takes analytic authority whenever the
	// arrangement can certify the exact crossing points splice into both polylines
	// without changing the sampled topology, so its fragments carry exact parameters
	// and the region areas are closed-form. The old assertions are kept verbatim (they
	// still hold), because the fallback for an uncertified pair is the sampled path —
	// not a degeneracy — so nothing this test blessed before is refused now.
	//
	// The round-2 geometry is the adversarial case: injecting exact cuts here once
	// fused three regions into one. It is correct at every sampling, and exact from
	// spt=8 — the density at which the chord map can host the crossing.
	for _, spt := range []int{3, 4, 5, 6, 7, 8, 16, 32, 64, 128, 256} {
		curves := []geom.ClosedCurve{
			geom.NewCircle(geom.NewPoint(0, 0), 5),
			geom.NewCircle(geom.NewPoint(1.9088280743172588, 0.8754286851013426), 3),
		}
		arr := geom.Regions(nil, curves, geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, arr.Degenerate, "round-2 pair is a clean transverse crossing at spt=%d", spt)
		require.Lenf(t, arr.Regions, 3, "round-2 pair is two lune caps + lens, never a fused blob, at spt=%d", spt)
		requireExactBoundsReproduce(t, nil, curves, arr)

		d := math.Hypot(1.9088280743172588, 0.8754286851013426)
		total, exact, partial := arrangementArea(arr)
		require.Truef(t, partial, "both circles are cut by the crossing at spt=%d", spt)
		if spt >= 8 {
			require.Truef(t, exact, "the round-2 crossing is certified from spt=8; spt=%d must report exact fragments", spt)
		}
		if exact {
			require.InDeltaf(t, twoCircleUnionArea(5, 3, d), total, 1e-9,
				"a certified crossing gives the closed-form union area at spt=%d", spt)
		}
	}

	// Across the transverse band the pair is always the correct three regions, and
	// wherever the fragments read exact the area is the closed-form union — never a
	// value that merely converges. An uncertified sampling still nets ~the union.
	const R, r = 5.0, 3.0
	for _, ang := range []float64{0.0, 0.45, math.Pi / 4, 1.3, 2.6} {
		for _, d := range []float64{2.5, 3.0, 4.0, 5.0, 6.5} { // transverse: |R-r|=2 < d < R+r=8
			cx, cy := d*math.Cos(ang), d*math.Sin(ang)
			union := twoCircleUnionArea(R, r, d)
			for spt := 8; spt <= 120; spt++ {
				arr := geom.Regions(nil, []geom.ClosedCurve{
					geom.NewCircle(geom.NewPoint(0, 0), R),
					geom.NewCircle(geom.NewPoint(cx, cy), r),
				}, geom.WithSegmentsPerTurn(spt))
				if arr.Degenerate {
					continue
				}
				total, exact, partial := arrangementArea(arr)
				exact = exact && partial
				require.Lenf(t, arr.Regions, 3, "blessed circle pair ang=%.3f d=%.1f spt=%d is two lune caps + lens", ang, d, spt)
				if exact {
					require.InDeltaf(t, union, total, 1e-9,
						"certified circle pair ang=%.3f d=%.1f spt=%d nets the closed-form union area", ang, d, spt)
					continue
				}
				require.InDeltaf(t, union, total, 0.1*union, "blessed circle pair ang=%.3f d=%.1f spt=%d nets ~the union area (sampled)", ang, d, spt)
			}
		}
	}
}

func TestAnalyticCircleCircleCrossingSamplingStable(t *testing.T) {
	// The curve/curve analog of TestAnalyticCircleChordSamplingStable: a well-separated
	// circle/circle secant reports the SAME exact fragment parameters and the SAME
	// closed-form area at every sampling density, because the cuts land on the exact
	// intersection points rather than on chord crossings.
	//
	// The circles cross at (3, ±4), so on the first circle the fragment bounds are the
	// parameters of the angles ±atan2(4,3) — asserted against the closed form, not
	// against a re-derivation of what the arrangement did.
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 5),
		geom.NewCircle(geom.NewPoint(6, 0), 5),
	}
	lo := math.Atan2(4, 3) / (2 * math.Pi)
	want := map[float64]struct{}{0: {}, 1: {}, lo: {}, 1 - lo: {}, 0.5 - lo: {}, 0.5 + lo: {}}
	onBound := func(v float64) bool {
		for u := range want {
			if math.Abs(v-u) < 1e-12 {
				return true
			}
		}
		return false
	}
	for _, spt := range []int{8, 16, 32, 64, 128, 256, 512} {
		arr := geom.Regions(nil, closed, geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, arr.Degenerate, "spt=%d", spt)
		require.Lenf(t, arr.Regions, 3, "two lune caps + lens at spt=%d", spt)
		requireExactBoundsReproduce(t, nil, closed, arr)

		total, exact, partial := arrangementArea(arr)
		require.Truef(t, partial, "both circles are cut by the crossing at spt=%d", spt)
		require.Truef(t, exact, "every fragment of a certified curve/curve crossing is exact at spt=%d", spt)
		require.InDeltaf(t, twoCircleUnionArea(5, 5, 6), total, 1e-9, "closed-form union area at spt=%d", spt)
		for _, rg := range arr.Regions {
			for _, e := range rg.Outer {
				require.Truef(t, onBound(e.TStart) && onBound(e.TEnd),
					"fragment [%v, %v] of source %d must be bounded by the closed-form crossing parameters at spt=%d",
					e.TStart, e.TEnd, e.SourceIndex, spt)
			}
		}
	}
}

func TestAnalyticCurveCrossingNeverBlessedWrong(t *testing.T) {
	// The soundness invariant for the curve/curve lift, over the whole transverse band
	// including its two near-tangent ends: an arrangement whose fragments all read
	// EXACT took analytic authority, so its topology and area must be exactly right.
	// A pair the certificate refuses falls back to the sampled path, which is only
	// held to the region count. Neither may be a blessed wrong topology.
	for _, rr := range [][2]float64{{5, 3}, {5, 5}, {5, 0.7}} {
		R, r := rr[0], rr[1]
		lo, hi := math.Abs(R-r), R+r
		for _, f := range []float64{0.02, 0.1, 0.5, 0.9, 0.98} {
			d := lo + f*(hi-lo)
			union := twoCircleUnionArea(R, r, d)
			for _, ang := range []float64{0.0, 0.45, 1.3, 2.6} {
				cx, cy := d*math.Cos(ang), d*math.Sin(ang)
				for spt := 3; spt <= 60; spt++ {
					arr := geom.Regions(nil, []geom.ClosedCurve{
						geom.NewCircle(geom.NewPoint(0, 0), R),
						geom.NewCircle(geom.NewPoint(cx, cy), r),
					}, geom.WithSegmentsPerTurn(spt))
					if arr.Degenerate {
						continue // conservatively rejected — sound
					}
					total, exact, partial := arrangementArea(arr)
					if !exact || !partial {
						continue // the sampled fallback, unchanged by the lift
					}
					require.Lenf(t, arr.Regions, 3,
						"certified R=%g r=%g d=%.4f ang=%.3f spt=%d is two lune caps + lens", R, r, d, ang, spt)
					require.InDeltaf(t, union, total, 1e-9,
						"certified R=%g r=%g d=%.4f ang=%.3f spt=%d nets the closed-form union area", R, r, d, ang, spt)
				}
			}
		}
	}
}

func TestAnalyticArcEndpointOnCrossingCircleKeepsRegion(t *testing.T) {
	// An arc whose own ENDPOINT lies exactly on a circle it also crosses is the
	// contact the incidence certificate cannot judge: the arc stops at the point, so
	// it contributes ONE chord departure there and the four cannot alternate. Blessing
	// it certified nothing, and the injected cut then bent the polylines through each
	// other and left the disk with no region at all — an arrangement reporting
	// degenerate=false, zero regions and zero area, which no consumer can detect.
	//
	// The invariant asserted is the one the collapse broke: the circle's disk survives,
	// whole, at every density. The arc dips inside the circle by far less than one
	// chord sagitta here, so the sub-sample sliver it carves is legitimately not
	// resolved at these densities — the disk being the single region is the correct
	// sampled answer, and the fault was losing it, not merging it.
	P := geom.NewPoint(5, 0) // on the r=5 circle centred at the origin
	for _, tc := range []struct {
		name    string
		rB      float64
		psiDeg  float64 // direction of B's centre from P, off the radial direction
		sweep   float64 // arc sweep from P; negative means the arc ENDS at P
		density []int
	}{
		// The contact at the arc's t=0 end.
		{"start-endpoint-r12", 12, 3, math.Pi / 6, []int{32, 64, 128}},
		{"start-endpoint-r5", 5, 0.25, math.Pi / 2, []int{32, 64, 256}},
		{"start-endpoint-r3", 3, -1, math.Pi, []int{32, 128}},
		// The contact at the arc's t=1 end — the mirrored case, where the contact's
		// segment-local parameter is 1 rather than 0.
		{"end-endpoint-r5", 5, -0.25, -math.Pi / 2, []int{32, 64, 256}},
		{"end-endpoint-r12", 12, -3, -math.Pi / 6, []int{32, 64, 128}},
		{"end-endpoint-r3", 3, 1, -math.Pi, []int{32, 128}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			psi := tc.psiDeg * math.Pi / 180
			cB := geom.NewPoint(P.X+tc.rB*math.Cos(psi), P.Y+tc.rB*math.Sin(psi))
			far := geom.NewPoint(5*math.Cos(tc.sweep), 5*math.Sin(tc.sweep))
			start, end := P, far
			if tc.sweep < 0 {
				start, end = far, P
			}
			curves := []geom.Curve{geom.NewArc(geom.NewPoint(0, 0), start, end)}
			closed := []geom.ClosedCurve{geom.NewCircle(cB, tc.rB)}
			for _, spt := range tc.density {
				arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
				if arr.Degenerate {
					continue // conservatively refused — sound
				}
				requireExactBoundsReproduce(t, curves, closed, arr)
				total, _, _ := arrangementArea(arr)
				require.NotEmptyf(t, arr.Regions, "the disk must not vanish at spt=%d", spt)
				require.InDeltaf(t, math.Pi*tc.rB*tc.rB, total, 1e-3,
					"the disk survives whole at spt=%d", spt)
			}
		})
	}
}

func TestAnalyticNearSampleVertexCrossingNotBlessedExact(t *testing.T) {
	// A crossing whose source parameter sits a hair from a sample vertex is snapped
	// onto that vertex (the decision is made in the PARAMETER, within segEps of the
	// segment boundary) and records no cut, so the fragment it bounds carries the
	// VERTEX's parameter — a plain sample fraction i/n. Certifying such a contact
	// published that fraction as an exact bound for a crossing it is not at, and a
	// certified curve/curve pair is exempt from the taint passes that would otherwise
	// have marked the weld inexact, so nothing downstream corrected it.
	//
	// Two r=5 circles crossing a hair off the 1/8 sample vertex, with the offset kept
	// inside the snap band at every density. Every bound a fragment reports as EXACT
	// must be the source's own domain end or a true crossing parameter — asserted
	// against the closed form, so the test cannot be satisfied by whatever the
	// arrangement happened to emit.
	const r = 5.0
	for _, spt := range []int{8, 16, 32, 64, 128, 256} {
		delta := 2 * math.Pi * 5e-10 / float64(spt) // inside the snap band at this density
		d := 2 * r * math.Cos(math.Pi/4-delta)
		closed := []geom.ClosedCurve{
			geom.NewCircle(geom.NewPoint(0, 0), r),
			geom.NewCircle(geom.NewPoint(d, 0), r),
		}
		// The circles meet at x=d/2, y=±r·sin(π/4−δ): the eighth-turn parameter, minus
		// δ. On the second circle the same two points sit half a turn away.
		p := (math.Pi/4 - delta) / (2 * math.Pi)
		allowed := map[int][]float64{
			0: {0, 1, p, 1 - p},
			1: {0, 1, 0.5 - p, 0.5 + p},
		}
		arr := geom.Regions(nil, closed, geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, arr.Degenerate, "a transverse crossing near a sample vertex is not degenerate (spt=%d)", spt)
		require.Lenf(t, arr.Regions, 3, "two lune caps + lens at spt=%d", spt)
		requireExactBoundsReproduce(t, nil, closed, arr)
		near := func(v float64, src int) bool {
			for _, u := range allowed[src] {
				if math.Abs(v-u) <= 1e-13 {
					return true
				}
			}
			return false
		}
		for _, rg := range arr.Regions {
			for _, e := range rg.Outer {
				if !e.TExact {
					continue
				}
				require.Truef(t, near(e.TStart, e.SourceIndex) && near(e.TEnd, e.SourceIndex),
					"exact fragment [%.17g %.17g] of source %d at spt=%d reports a bound that is neither a domain end nor a closed-form crossing parameter",
					e.TStart, e.TEnd, e.SourceIndex, spt)
			}
		}
	}
}

func TestAnalyticFusedCurveCrossingNotBlessedExact(t *testing.T) {
	// A refused curve/curve crossing goes back to the SAMPLED path, and the sampled
	// path can miss it entirely at a density that certifies the scene's OTHER pairs.
	// The regions that crossing separates then fuse, while every surviving fragment is
	// bounded by an exact cut of a certified pair — so the arrangement publishes a
	// topology that is missing a crossing as fully certified.
	//
	// Three r=5 circles in general position do exactly that: pairs (0,1) and (0,2)
	// certify, the shallow (1,2) crossing is below the sampling until spt=24, and the
	// seven true regions read as five with all sixteen bounds exact.
	//
	// The wrong region count at coarse sampling is the sampled path's own pre-existing
	// density limit — it moves with WithSegmentsPerTurn with or without any analytic
	// authority, and this test does not assert it away. What must never happen is the
	// BLESSING: an arrangement whose every bound reads exact has to be the converged
	// one. Seven is the truth here (stable from spt=24 to spt=2048).
	circles := func(c1, c2 [2]float64) []geom.ClosedCurve {
		return []geom.ClosedCurve{
			geom.NewCircle(geom.NewPoint(0, 0), 5),
			geom.NewCircle(geom.NewPoint(c1[0], c1[1]), 5),
			geom.NewCircle(geom.NewPoint(c2[0], c2[1]), 5),
		}
	}
	c1 := [2]float64{0.9620341521811382, -7.264319290590297}
	c2 := [2]float64{-1.343538285914439, 2.4540578044489263}
	for _, spt := range []int{8, 12, 16, 24, 32, 48, 64, 128, 256, 512} {
		closed := circles(c1, c2)
		arr := geom.Regions(nil, closed, geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, arr.Degenerate, "three clean transverse crossings at spt=%d", spt)
		requireExactBoundsReproduce(t, nil, closed, arr)
		_, exact, partial := arrangementArea(arr)
		if exact && partial {
			require.Lenf(t, arr.Regions, 7, "an all-exact arrangement is the converged one; spt=%d blessed a fused map", spt)
		}
		if spt <= 16 {
			// The reported case, pinned directly: at these densities the (1,2) crossing
			// is absent from the sampled map, so no bound of the component may read
			// exact — whatever the region count does.
			require.Falsef(t, exact, "the fused map at spt=%d must report no exact bound", spt)
		}
		if spt >= 24 {
			require.Truef(t, exact && partial, "the resolved map at spt=%d keeps its exact bounds", spt)
		}
	}

	// The same construction with the two outer circles a hair short of tangency pushes
	// the failure window past the ADAPTIVE DEFAULT density (256 segments per turn) that
	// Sketch.Profiles renders at — the ordinary consumer path, no option involved.
	dx, dy := c2[0]-c1[0], c2[1]-c1[1]
	s := (10 - 1e-4) / math.Hypot(dx, dy)
	closed := circles(c1, [2]float64{c1[0] + dx*s, c1[1] + dy*s})
	arr := geom.Regions(nil, closed)
	require.False(t, arr.Degenerate, "a shallow crossing is not a degeneracy")
	requireExactBoundsReproduce(t, nil, closed, arr)
	_, exact, partial := arrangementArea(arr)
	require.Len(t, arr.Regions, 5, "the default density still fuses this shallow crossing")
	require.True(t, partial, "the certified pairs still cut their circles")
	require.False(t, exact, "a fused map is never exact, default sampling included")
}

// exactBySource reports, per source index the arrangement emitted a bound for, whether
// EVERY bound that source contributes reads exact.
func exactBySource(arr *geom.Arrangement) map[int]bool {
	out := map[int]bool{}
	note := func(e geom.BoundaryEdge) {
		if v, seen := out[e.SourceIndex]; seen {
			out[e.SourceIndex] = v && e.TExact
			return
		}
		out[e.SourceIndex] = e.TExact
	}
	for _, rg := range arr.Regions {
		for _, e := range rg.Outer {
			note(e)
		}
		for _, h := range rg.Holes {
			for _, e := range h {
				note(e)
			}
		}
	}
	return out
}

func TestAnalyticFusedComponentLeavesUntouchedClusterExact(t *testing.T) {
	// The withdrawal is per CONNECTED COMPONENT, and a component is joined by CONTACT.
	// A handled pair the kernel classified and found NOT to meet carries no contact and
	// must join nothing: joining on the mere existence of a classification collapsed
	// every analytic source in the scene into one component, so one refused crossing
	// withdrew exactness scene-wide — including from a cluster nothing in the fused
	// component can reach.
	//
	// TestAnalyticFusedCurveCrossingNotBlessedExact's three fused circles, plus an
	// ordinary two-circle crossing 100 units away. The triple loses its exactness; the
	// distant pair keeps its own.
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 5),
		geom.NewCircle(geom.NewPoint(0.9620341521811382, -7.264319290590297), 5),
		geom.NewCircle(geom.NewPoint(-1.343538285914439, 2.4540578044489263), 5),
		geom.NewCircle(geom.NewPoint(100, 0), 5),
		geom.NewCircle(geom.NewPoint(106, 0), 5),
	}
	arr := geom.Regions(nil, closed, geom.WithSegmentsPerTurn(8))
	require.False(t, arr.Degenerate, "five clean transverse crossings")
	requireExactBoundsReproduce(t, nil, closed, arr)
	require.Len(t, arr.Regions, 8, "the fused triple's five regions plus the far pair's three")
	exact := exactBySource(arr)
	require.Len(t, exact, 5, "every circle bounds something")
	for _, src := range []int{0, 1, 2} {
		require.Falsef(t, exact[src], "source %d is in the fused component", src)
	}
	for _, src := range []int{3, 4} {
		require.Truef(t, exact[src], "source %d is 100 units from the fused component", src)
	}
}

// requireNoExactBounds asserts the scene gate: not one emitted bound of the
// arrangement reports TExact.
func requireNoExactBounds(t *testing.T, arr *geom.Arrangement, what string) {
	t.Helper()
	for src, ok := range exactBySource(arr) {
		require.Falsef(t, ok, "%s: source %d publishes an exact bound", what, src)
	}
}

func TestFreeFormSourceWithholdsExactBoundsSceneWide(t *testing.T) {
	// A free-form source — ellipse, elliptical arc, conic, spline, closed spline, fit
	// spline or NURBS — reaches the planar map only as chords, and nothing bounds how
	// far one of them runs from its chord: the sampler places vertices, it does not
	// certify a deviation. So such a curve can cross another entirely BETWEEN two
	// samples; the crossing is then missing from the map, the regions it separates fuse,
	// and the scene's analytic pairs — certified on their own merits and cut exactly —
	// publish the fused map with every bound exact.
	//
	// The gate is therefore on the SOURCE KINDS, not on any estimate of how far a chord
	// can stray: a scene holding ANY free-form source publishes NO exact bound anywhere
	// in it, including on the lines, circles and arcs beside it, and however far apart
	// they sit. This test pins all three halves of that — the hidden crossing, the
	// resolved crossing, and the untouching curve — plus the all-analytic control that
	// keeps its exactness.
	circles := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 5),
		geom.NewCircle(geom.NewPoint(-6, 0), 5),
	}

	// (a) The demonstrated hazard: an ellipse whose rim clips the first circle in a lens
	// far below 256 segments per turn. The arrangement returns four regions — the lens
	// absent — and no bound of the scene may read exact.
	clipping := append(append([]geom.ClosedCurve{}, circles...),
		geom.NewEllipse(geom.NewPoint(9.9997, 0), 5, 5, math.Pi/256))
	coarse := geom.Regions(nil, clipping, geom.WithSegmentsPerTurn(256))
	require.False(t, coarse.Degenerate, "three clean transverse crossings")
	requireExactBoundsReproduce(t, nil, clipping, coarse)
	require.Len(t, coarse.Regions, 4, "the ellipse lens is below this sampling")
	requireNoExactBounds(t, coarse, "a map missing a crossing")

	// (b) Raising the density resolves the lens. Exactness stays withheld: whether a
	// given density happens to resolve a given free-form contact is not something the
	// arrangement can certify, so the verdict does not turn on it.
	fine := geom.Regions(nil, clipping, geom.WithSegmentsPerTurn(2048))
	require.False(t, fine.Degenerate, "the finer sampling is clean too")
	requireExactBoundsReproduce(t, nil, clipping, fine)
	require.Len(t, fine.Regions, 5, "the lens resolves at this sampling")
	requireNoExactBounds(t, fine, "a resolved lens is still a sampled contact")

	// (c) The accepted coverage cost, stated directly: a free-form curve that touches
	// nothing withholds exactness from the certified circle pair just the same. Distance
	// is not the test — the source kind is.
	for _, tc := range []struct {
		name   string
		curves []geom.Curve
		closed []geom.ClosedCurve
	}{
		{name: "ellipse", closed: []geom.ClosedCurve{geom.NewEllipse(geom.NewPoint(40, 0), 5, 5, math.Pi/256)}},
		{name: "elliptical arc", curves: []geom.Curve{
			geom.NewEllipticalArc(geom.NewPoint(40, 0), geom.NewPoint(45, 0), geom.NewPoint(40, 3), 5, 3, 0),
		}},
		{name: "conic", curves: []geom.Curve{
			geom.NewConic(geom.NewPoint(37, 0), geom.NewPoint(40, 4), geom.NewPoint(43, 0), 0.6),
		}},
		{name: "spline", curves: []geom.Curve{mustSpline(t,
			geom.NewPoint(36, 0), geom.NewPoint(38, 4), geom.NewPoint(42, 4), geom.NewPoint(44, 0))}},
		{name: "closed spline", closed: []geom.ClosedCurve{mustClosedSpline(t,
			geom.NewPoint(37, -3), geom.NewPoint(43, -3), geom.NewPoint(43, 3), geom.NewPoint(37, 3))}},
		{name: "fit spline", curves: []geom.Curve{mustFitSpline(t,
			geom.NewPoint(36, 0), geom.NewPoint(40, 3), geom.NewPoint(44, 0))}},
		{name: "nurbs", curves: []geom.Curve{geom.NewNURBS(2,
			[]*geom.Point{geom.NewPoint(37, 0), geom.NewPoint(40, 4), geom.NewPoint(43, 0)},
			[]float64{0, 0, 0, 1, 1, 1}, []float64{1, 2, 1})}},
	} {
		t.Run("parked "+tc.name, func(t *testing.T) {
			closed := append(append([]geom.ClosedCurve{}, circles...), tc.closed...)
			arr := geom.Regions(tc.curves, closed, geom.WithSegmentsPerTurn(256))
			require.False(t, arr.Degenerate, "the parked curve touches nothing")
			requireExactBoundsReproduce(t, tc.curves, closed, arr)
			requireNoExactBounds(t, arr, "a scene holding a "+tc.name)
		})
	}

	// (d) The control: the same two circles ALONE — every source a line, circle or arc —
	// keep every exact bound the certificate earns them. What withholds exactness is the
	// free-form source in the scene, nothing about the circles or their crossing.
	only := geom.Regions(nil, circles, geom.WithSegmentsPerTurn(256))
	require.False(t, only.Degenerate, "one clean transverse crossing")
	requireExactBoundsReproduce(t, nil, circles, only)
	require.Len(t, only.Regions, 3, "two lune caps plus the lens")
	for src, ok := range exactBySource(only) {
		require.Truef(t, ok, "source %d of an all-analytic scene must keep its exact bounds", src)
	}
}

func mustSpline(t *testing.T, ctrl ...*geom.Point) *geom.Spline {
	t.Helper()
	sp, err := geom.NewSpline(ctrl...)
	require.NoError(t, err)
	return sp
}

func mustClosedSpline(t *testing.T, ctrl ...*geom.Point) *geom.ClosedSpline {
	t.Helper()
	sp, err := geom.NewClosedSpline(ctrl...)
	require.NoError(t, err)
	return sp
}

func mustFitSpline(t *testing.T, fit ...*geom.Point) *geom.FitSpline {
	t.Helper()
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	return sp
}

func TestAnalyticCrossingAtSampleVertexBlessedExact(t *testing.T) {
	// A crossing whose point IS a sample vertex of both curves, to within round-off, is
	// CERTIFIED: the reported parameter is the true crossing parameter, which happens to
	// equal that vertex's own sample fraction, and evaluating it reproduces the emitted
	// point. What the certificate refuses is a contact merely NEAR a sample vertex —
	// mapped onto it in PARAMETER while sitting away from it in POSITION
	// (TestAnalyticNearSampleVertexCrossingNotBlessedExact) — and which of the two a
	// given contact is moves with the sampling rather than persisting through it.
	//
	// Two r=5 circles centred 10·cos(45°) apart cross exactly at the ±45° sample vertex
	// of both at 8 segments per turn.
	const r = 5.0
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), r),
		geom.NewCircle(geom.NewPoint(2*r*math.Cos(math.Pi/4), 0), r),
	}
	arr := geom.Regions(nil, closed, geom.WithSegmentsPerTurn(8))
	require.False(t, arr.Degenerate, "a transverse crossing on a sample vertex is not degenerate")
	require.Len(t, arr.Regions, 3, "two lune caps plus the lens")
	requireExactBoundsReproduce(t, nil, closed, arr)
	_, exact, partial := arrangementArea(arr)
	require.True(t, partial, "both circles are cut")
	require.True(t, exact, "a contact that IS a sample vertex carries the true crossing parameter")
}

// regionAreasDescending returns the arrangement's region areas, largest first, and
// whether EVERY emitted boundary fragment of every region carries an exact range.
func regionAreasDescending(arr *geom.Arrangement) ([]float64, bool) {
	areas := make([]float64, 0, len(arr.Regions))
	exact := true
	for _, rg := range arr.Regions {
		areas = append(areas, rg.Area)
		for _, e := range rg.Outer {
			exact = exact && e.TExact
		}
		for _, h := range rg.Holes {
			for _, e := range h {
				exact = exact && e.TExact
			}
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(areas)))
	return areas, exact
}

func TestAnalyticCurveCrossingEndpointOnChordNotCertified(t *testing.T) {
	// An arc whose own ENDPOINT rests on the INTERIOR of one of the circle's chords is a
	// contact the pair has no analytic event for — the exact geometry puts that endpoint
	// strictly inside the disk, and only the chord approximation brings the two polylines
	// together there. Taking analytic authority skips the sampled loop for the pair, so
	// the contact is never recorded, and the map the pair then publishes is NOT the
	// sampled one the certificate promises to reproduce: one region instead of two, with
	// every bound reading exact.
	//
	// The contact is placed by construction. At 8 segments per turn the circle's samples
	// sit every 45 degrees, and the arc ends a quarter of the way along the chord between
	// the 0 and 45 degree samples. Its host parameters there are 1 on the arc's own last
	// segment and 0.25 on the chord, so a check that excuses a contact for sitting at a
	// segment END lets it through; deciding by identity with an injected crossing point
	// does not.
	const r = 5.0
	v1x, v1y := r*math.Cos(math.Pi/4), r*math.Sin(math.Pi/4)
	ex, ey := r+0.25*(v1x-r), 0.25*v1y

	cx, cy := -3.0, -2.75
	ra := math.Hypot(ex-cx, ey-cy)
	end := math.Atan2(ey-cy, ex-cx)
	start := end - 1.2
	curves := []geom.Curve{geom.NewArc(
		geom.NewPoint(cx, cy),
		geom.NewPoint(cx+ra*math.Cos(start), cy+ra*math.Sin(start)),
		geom.NewPoint(ex, ey))}
	closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), r)}

	arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(8))
	require.False(t, arr.Degenerate,
		"the fallback for an uncertified pair is the sampled path, never a degeneracy")
	require.Len(t, arr.Regions, 2, "the sampled map's own regions, which the certified pair fused into one")
	areas, exact := regionAreasDescending(arr)
	require.InDelta(t, 78.512659246063137, areas[0], 1e-9)
	require.InDelta(t, 0.14921088265903304, areas[1], 1e-9)
	require.False(t, exact,
		"a pair whose polylines meet away from every injected crossing publishes no exact bound")
}

func TestAnalyticCurveCrossingAtSampleVertexNotCertified(t *testing.T) {
	// The same failure reached by the other door: a genuine transverse crossing of the two
	// SAMPLED polylines that lands on a sample vertex of one of them, and on no analytic
	// contact at all. It is a crossing with no node — the pair's sampled crossings are
	// never recorded once it takes analytic authority — so the face walk runs the two
	// edges through each other and fuses the regions on either side. Here two regions
	// become one, reported with every bound exact.
	//
	// Built by placing an INTERIOR sample vertex of the first arc exactly on the interior
	// of a chord of the second: at 8 segments per turn the second arc's 1.0 rad sweep is
	// sampled into 2 chords and the first arc's 2.0 rad sweep into 3, so vertex 1 of the
	// first lands three quarters of the way along chord 0 of the second.
	const spt, r = 8, 5.0
	const qStart, qSweep = -2.4, 1.0
	qAt := func(u float64) *geom.Point {
		a := qStart + u*qSweep
		return geom.NewPoint(r*math.Cos(a), r*math.Sin(a))
	}
	const nq = 2 // ceil(spt * qSweep / 2pi)
	c0, c1 := qAt(0), qAt(1.0/nq)
	ex := c0.X + 0.75*(c1.X-c0.X)
	ey := c0.Y + 0.75*(c1.Y-c0.Y)

	const pSweep, nP, iVert = 2.0, 3, 1 // nP = ceil(spt * pSweep / 2pi)
	pcx, pcy := 0.0, -1.0
	rp := math.Hypot(ex-pcx, ey-pcy)
	pStart := math.Atan2(ey-pcy, ex-pcx) - float64(iVert)/float64(nP)*pSweep
	curves := []geom.Curve{
		geom.NewArc(geom.NewPoint(pcx, pcy),
			geom.NewPoint(pcx+rp*math.Cos(pStart), pcy+rp*math.Sin(pStart)),
			geom.NewPoint(pcx+rp*math.Cos(pStart+pSweep), pcy+rp*math.Sin(pStart+pSweep))),
		geom.NewArc(geom.NewPoint(0, 0), qAt(0), qAt(1)),
	}

	arr := geom.Regions(curves, nil, geom.WithSegmentsPerTurn(spt))
	require.False(t, arr.Degenerate, "an uncertified pair falls back to the sampled path, not to a degeneracy")
	require.Len(t, arr.Regions, 2, "the two regions the node-less crossing separates")
	areas, exact := regionAreasDescending(arr)
	require.InDelta(t, 0.022316286135887403, areas[0], 1e-9)
	require.InDelta(t, 0.0035697752231414488, areas[1], 1e-9)
	require.False(t, exact, "a crossing the map has no node for withholds every exact bound of the pair")
}

func TestAnalyticContactAtVertexBandIsChordLocal(t *testing.T) {
	// The identity band that decides whether a contact IS the sample vertex it was mapped
	// to must be a property of the PAIR, not of the drawing. Bounding it by the scene's
	// bounding-box extent alone let an unrelated object far away widen it: the two circles
	// below cross 1.1e-10 away from the r=5 circle's own 0.125 sample vertex — a gap the
	// pair correctly refuses when drawn alone — and one construction line parked ~100 units
	// off flipped it to certified, publishing the sample fraction 0.125 as the exact
	// crossing parameter. Nothing about the pair changed; only the scene did.
	//
	// So the verdict must be the same with and without the distant line, at every density,
	// and it must be the refusal: the contact is not the vertex.
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 5),
		geom.NewCircle(geom.NewPoint(7, 0), 4.9500025573766155),
	}
	for _, spt := range []int{8, 16, 64, 256} {
		alone := geom.Regions(nil, closed, geom.WithSegmentsPerTurn(spt))
		far := geom.Regions([]geom.Curve{geom.NewLine(geom.NewPoint(122, -1), geom.NewPoint(122, 1))},
			closed, geom.WithSegmentsPerTurn(spt))

		require.Falsef(t, alone.Degenerate, "spt=%d", spt)
		require.Falsef(t, far.Degenerate, "spt=%d", spt)
		require.Lenf(t, alone.Regions, 3, "spt=%d: two lune caps plus the lens", spt)
		require.Lenf(t, far.Regions, 3, "spt=%d: the distant line bounds no region", spt)

		aAreas, aExact := regionAreasDescending(alone)
		fAreas, fExact := regionAreasDescending(far)
		require.Falsef(t, aExact, "spt=%d: a contact 1.1e-10 off its vertex is not that vertex", spt)
		require.Falsef(t, fExact, "spt=%d: and a line 122 units away cannot make it one", spt)
		for k := range aAreas {
			require.InDeltaf(t, aAreas[k], fAreas[k], 1e-12,
				"spt=%d: region %d must not move because a distant line was drawn", spt, k)
		}
	}
}

func TestAnalyticArcArcCrossingCertified(t *testing.T) {
	// The certificate is about the PAIR being two sampled curves, not about them being
	// full circles: two arcs on different carriers, closed into a wire by two lines,
	// carry the same exact cuts. Asserted against the arrangement's own high-density
	// answer, which the low densities must reproduce EXACTLY once certified — the
	// sampling-independence the lift buys.
	c1, c2 := geom.NewPoint(0, 0), geom.NewPoint(2, 1.5)
	at := func(c *geom.Point, ang float64) *geom.Point {
		return geom.NewPoint(c.X+3*math.Cos(ang), c.Y+3*math.Sin(ang))
	}
	s1, e1 := at(c1, -2.4), at(c1, 2.4)
	s2, e2 := at(c2, math.Pi-2.4), at(c2, math.Pi+2.4)
	curves := []geom.Curve{
		geom.NewArc(c1, s1, e1), geom.NewArc(c2, s2, e2),
		geom.NewLine(e1, s2), geom.NewLine(e2, s1),
	}
	ref := geom.Regions(curves, nil, geom.WithSegmentsPerTurn(1024))
	require.False(t, ref.Degenerate, "the reference sampling is clean")
	refArea, _, _ := arrangementArea(ref)
	require.NotEmpty(t, ref.Regions)

	for _, spt := range []int{8, 16, 32, 64, 128} {
		arr := geom.Regions(curves, nil, geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, arr.Degenerate, "spt=%d", spt)
		requireExactBoundsReproduce(t, curves, nil, arr)
		total, _, _ := arrangementArea(arr)
		require.Lenf(t, arr.Regions, len(ref.Regions), "same region count as the reference at spt=%d", spt)
		require.InDeltaf(t, refArea, total, 1e-9, "the arc/arc crossing is exact, so the area does not move with sampling (spt=%d)", spt)
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

	// Overlapping sweeps: docs/coincident-carrier-resolution-design.md resolves this
	// case (a single window, at least one operand an arc) rather than flagging it —
	// superseding the old unconditional Degenerate=true this sub-case used to assert.
	// Neither arc closes on its own or on the other (their four endpoints are all
	// distinct points), so the resolved boundary is one open chain from the first
	// arc's own start to the second arc's own end — a dangling spur with nothing to
	// bound a region, pruned away exactly like any other open curve. The interesting,
	// CLOSED case (a real merged region, exercising SourceIndex naming and area) is
	// TestAnalyticSameCarrierArcsResolvedRegion below.
	overlapping := geom.Regions([]geom.Curve{
		geom.NewArc(c, at(0), at(math.Pi)),
		geom.NewArc(c, at(math.Pi/2), at(3*math.Pi/2)),
	}, nil, geom.WithSegmentsPerTurn(32))
	require.False(t, overlapping.Degenerate, "a resolved same-carrier arc overlap is no longer degenerate")
	require.Empty(t, overlapping.Regions, "the merged chain has nothing closing it into a region")
}

// gearArcArcCurves builds an arc/arc analog of probe case C
// (.tmp/decad-2d-region-asks/probe/main.go) — a "hub" that is a large, non-full
// ARC (not a full circle, so BOTH sides of the coincident pair are genuine arcs)
// closed into its own disk-like region by a chord, plus a small "tooth" whose root
// arc lies exactly on the hub arc's carrier. hubFirst controls authoring order —
// and so, per "The SourceIndex decision" in
// docs/coincident-carrier-resolution-design.md, which of {hubArc, rootArc} ends up
// the lower (named) index.
func gearArcArcCurves(hubFirst bool) (curves []geom.Curve, hubArcIdx, rootArcIdx int) {
	c := geom.NewPoint(0, 0)
	at := func(ang float64) *geom.Point { return geom.NewPoint(10*math.Cos(ang), 10*math.Sin(ang)) }
	deg := math.Pi / 180
	hubArc := geom.NewArc(c, at(-170*deg), at(170*deg)) // 340° sweep, a 20° gap facing left
	hubClose := geom.NewLine(at(170*deg), at(-170*deg))
	ax, ay := 10*math.Cos(0.3), 10*math.Sin(0.3)
	rootArc := geom.NewArc(c, geom.NewPoint(ax, -ay), geom.NewPoint(ax, ay))
	f1 := geom.NewLine(geom.NewPoint(ax, ay), geom.NewPoint(13, 1))
	tip := geom.NewLine(geom.NewPoint(13, 1), geom.NewPoint(13, -1))
	f2 := geom.NewLine(geom.NewPoint(13, -1), geom.NewPoint(ax, -ay))
	if hubFirst {
		return []geom.Curve{hubArc, hubClose, rootArc, f1, tip, f2}, 0, 2
	}
	return []geom.Curve{rootArc, f1, tip, f2, hubClose, hubArc}, 5, 0
}

// TestAnalyticSameCarrierArcsResolvedRegion is the CLOSED counterpart of
// TestAnalyticSameCarrierArcs's "overlapping" sub-case: an arc/arc coincident
// carrier (docs/coincident-carrier-resolution-design.md) that DOES bound real
// regions, so resolution produces a genuinely merged shared boundary rather than a
// pruned open chain. Mirrors probe case C's arc/circle shape but with the hub as a
// (non-full) arc, so both sides of the resolved pair are open curves.
func TestAnalyticSameCarrierArcsResolvedRegion(t *testing.T) {
	for _, hubFirst := range []bool{false, true} {
		curves, hubIdx, rootIdx := gearArcArcCurves(hubFirst)
		for _, spt := range []int{8, 16, 32, 64} {
			arr := geom.Regions(curves, nil, geom.WithSegmentsPerTurn(spt))
			require.Falsef(t, arr.Degenerate, "hubFirst=%v spt=%d", hubFirst, spt)
			require.Lenf(t, arr.Regions, 2, "hubFirst=%v spt=%d: hub + tooth", hubFirst, spt)
			requireExactBoundsReproduce(t, curves, nil, arr)

			named, losing := rootIdx, hubIdx
			if hubIdx < rootIdx {
				named, losing = hubIdx, rootIdx
			}
			var hubArea, toothArea float64
			var hubUsesNamed, toothUsesNamed, toothUsesLosing bool
			for _, r := range arr.Regions {
				usesNamed, usesLosing := false, false
				for _, e := range r.Outer {
					usesNamed = usesNamed || e.SourceIndex == named
					usesLosing = usesLosing || e.SourceIndex == losing
				}
				if r.Area > 100 { // the hub-like region, area ~313.8; the tooth is ~11.9
					hubArea, hubUsesNamed = r.Area, usesNamed
					// The hub region legitimately uses the LOSING source too, for the
					// part of its domain OUTSIDE the suppressed coincidence window (e.g.
					// hubArc's own remaining ~306° when rootArc is named) — that is not
					// the coincident span, so it is not a resolution failure.
				} else {
					toothArea, toothUsesNamed, toothUsesLosing = r.Area, usesNamed, usesLosing
				}
			}
			require.InDeltaf(t, 313.80698, hubArea, 1e-3, "hubFirst=%v spt=%d", hubFirst, spt)
			require.InDeltaf(t, 11.864262, toothArea, 1e-3, "hubFirst=%v spt=%d", hubFirst, spt)
			require.Truef(t, hubUsesNamed, "hubFirst=%v spt=%d: hub region must use the named source", hubFirst, spt)
			require.Truef(t, toothUsesNamed, "hubFirst=%v spt=%d: tooth region must use the named source", hubFirst, spt)
			// The tooth's boundary is ENTIRELY inside the coincident span plus its own
			// flank/tip lines, so the losing source — whose only contribution there was
			// exactly that span — must never appear in the tooth region at all.
			require.Falsef(t, toothUsesLosing, "hubFirst=%v spt=%d: the losing source must not appear in the tooth region", hubFirst, spt)
		}
	}
}

func TestAnalyticMergedExternalTangentBlessed(t *testing.T) {
	// The tangent contact canonicalizes onto a shared sample vertex of both
	// cycle-bearing circles (here (3,0), a cardinal sample point of both). Chord
	// ordering would branch-swap there; exact tangent-port ordering (increment 3)
	// separates the two loops by opposite curvature sign, so this is now blessed as
	// two clean disks at every sampling density — not conservatively degenerate.
	for _, spt := range []int{8, 16, 32, 64, 128} {
		closed := []geom.ClosedCurve{
			geom.NewCircle(geom.NewPoint(0, 0), 3),
			geom.NewCircle(geom.NewPoint(6, 0), 3),
		}
		arr := geom.Regions(nil, closed, geom.WithSegmentsPerTurn(spt))
		requireExactBoundsReproduce(t, nil, closed, arr)
		require.Falsef(t, arr.Degenerate, "merged external tangency is certified clean at spt=%d", spt)
		require.Lenf(t, arr.Regions, 2, "two disks at spt=%d", spt)
		var total float64
		for _, rg := range arr.Regions {
			total += rg.Area
		}
		require.InDeltaf(t, 2*math.Pi*9, total, 1e-9, "two full disks (exact) at spt=%d", spt)
	}
}

func TestAnalyticInternalTangentBlessed(t *testing.T) {
	// Internal (containment) tangency is now blessed (increment 7 §7a): exact
	// tangent-port ordering separates the loops at the shared vertex and exact
	// point-in-region containment nests the inner cycle into the outer, so the result
	// is the annulus π·(R²−r²) plus the inner disk π·r² — at every sampling, for a
	// merged (shared-vertex) contact and for a tiny inner alike (where the chord
	// poke-out used to defeat the sampled containment).
	const R, r = 6.0, 3.0
	for _, spt := range []int{8, 16, 32, 64, 128} {
		closed := []geom.ClosedCurve{
			geom.NewCircle(geom.NewPoint(0, 0), R),
			geom.NewCircle(geom.NewPoint(R-r, 0), r), // internally tangent at (R,0), a shared cardinal vertex
		}
		arr := geom.Regions(nil, closed, geom.WithSegmentsPerTurn(spt))
		requireExactBoundsReproduce(t, nil, closed, arr)
		require.Falsef(t, arr.Degenerate, "merged internal tangency is certified clean at spt=%d", spt)
		require.Lenf(t, arr.Regions, 2, "annulus + inner disk at spt=%d", spt)
		var total float64
		for _, g := range arr.Regions {
			total += g.Area
		}
		require.InDeltaf(t, math.Pi*R*R, total, 1e-9, "total nets the outer disk at spt=%d", spt)
		// the two regions are exactly {π·r², π·(R²−r²)}, in either order
		got0, got1 := arr.Regions[0].Area, arr.Regions[1].Area
		want0, want1 := math.Pi*r*r, math.Pi*(R*R-r*r)
		ok := (math.Abs(got0-want0) < 1e-9 && math.Abs(got1-want1) < 1e-9) ||
			(math.Abs(got0-want1) < 1e-9 && math.Abs(got1-want0) < 1e-9)
		require.Truef(t, ok, "regions are the inner disk + annulus at spt=%d: got %v", spt, []float64{got0, got1})
	}
}

func TestAnalyticExactContainmentConcentricNested(t *testing.T) {
	// Disjoint nested circles, including CONCENTRIC (the exact ray-cast containment's
	// whole-circle seam edge case: a centre-aligned probe's +x ray hits the outer
	// circle exactly at its param seam, which must still count as one crossing). The
	// inner must be subtracted as a hole — annulus + inner disk, never a double count.
	const R, r = 5.0, 1.5
	for _, off := range []float64{0.0, 0.0001, 2.0, 3.0} {
		for _, spt := range []int{8, 16, 32, 64} {
			arr := geom.Regions(nil, []geom.ClosedCurve{
				geom.NewCircle(geom.NewPoint(0, 0), R),
				geom.NewCircle(geom.NewPoint(off, 0), r),
			}, geom.WithSegmentsPerTurn(spt))
			require.Falsef(t, arr.Degenerate, "disjoint nested is clean off=%.4f spt=%d", off, spt)
			require.Lenf(t, arr.Regions, 2, "annulus + inner disk off=%.4f spt=%d", off, spt)
			var total float64
			for _, g := range arr.Regions {
				total += g.Area
			}
			require.InDeltaf(t, math.Pi*R*R, total, 1e-9, "inner subtracted (no double count) off=%.4f spt=%d", off, spt)
		}
	}
}

func TestAnalyticExactContainmentSeamHole(t *testing.T) {
	// A circular hole sitting near the angle-0 (+x) param seam of a circular face.
	// The exact ray-cast's horizontal ray crosses the face circle right at its seam,
	// where the seam param rounds to the fragment-endpoint boundary; without the
	// generic probe perturbation the half-open test dropped that crossing and the hole
	// was double-counted (blessed wrong). The hole must be subtracted from the lune and
	// the inner disk counted as its own region — total nets the full outer disk.
	const R, hr = 2.0, 0.05
	outer := geom.NewCircle(geom.NewPoint(0, 0), R)
	for _, hx := range []float64{1.80, 1.85, 1.90, 1.93} { // sweep the hole across the +x extreme
		for _, spt := range []int{16, 32, 64, 128, 256} {
			arr := geom.Regions(
				[]geom.Curve{geom.NewLine(geom.NewPoint(1.7, -6), geom.NewPoint(1.7, 6))},
				[]geom.ClosedCurve{outer, geom.NewCircle(geom.NewPoint(hx, 0), hr)},
				geom.WithSegmentsPerTurn(spt))
			require.Falsef(t, arr.Degenerate, "hx=%.2f spt=%d", hx, spt)
			var total float64
			holes := 0
			for _, g := range arr.Regions {
				total += g.Area
				holes += len(g.Holes)
			}
			require.Equalf(t, 1, holes, "the hole is subtracted from the lune hx=%.2f spt=%d", hx, spt)
			require.InDeltaf(t, math.Pi*R*R, total, 1e-9, "no seam double-count hx=%.2f spt=%d", hx, spt)
		}
	}
}

func TestAnalyticInternalTangentTinyInnerBlessed(t *testing.T) {
	// The tiny-inner regime that defeated the sampled containment (the inner chord
	// polygon poked outside the outer near the contact, so the hole was not subtracted
	// and the area double-counted) is now exact. Sweep small radii and contact angles.
	const R = 5.0
	for _, r := range []float64{0.2, 0.5, 1.0} {
		for _, th := range []float64{0.0, 0.37, 1.1, 2.6} {
			d := R - r
			for _, spt := range []int{8, 24, 64} {
				arr := geom.Regions(nil, []geom.ClosedCurve{
					geom.NewCircle(geom.NewPoint(0, 0), R),
					geom.NewCircle(geom.NewPoint(d*math.Cos(th), d*math.Sin(th)), r),
				}, geom.WithSegmentsPerTurn(spt))
				if arr.Degenerate {
					continue
				}
				var total float64
				for _, g := range arr.Regions {
					total += g.Area
				}
				require.Lenf(t, arr.Regions, 2, "annulus + inner disk r=%.1f th=%.2f spt=%d", r, th, spt)
				require.InDeltaf(t, math.Pi*R*R, total, 1e-9, "no double-count r=%.1f th=%.2f spt=%d", r, th, spt)
			}
		}
	}
}

func TestAnalyticUnexplainedWeldIsInexact(t *testing.T) {
	// The vertex table welds by DISTANCE while the analytic kernel decides in exact
	// closed form, so a handled (line/circle) pair can still be split by a weld the
	// kernel never accounted for. Here the line's start (0,y) sits just inside the
	// circle's top sample vertex (0,1) — 1-y < merge — so the two weld and the circle
	// is split at t=0.25 by nothing but that distance rule. The pair's real analytic
	// crossing is elsewhere, at (1.5·merge, y): farther than merge from BOTH welded
	// endpoints, so it canonicalizes to a graph vertex of its own and cannot explain
	// the weld. A fragment bounded by the weld must therefore report TExact = false —
	// blessing it would certify a range that only a sampling tolerance produced.
	merge := 0.01
	a := 1.5 * merge
	y := math.Sqrt(1 - a*a)

	curves := []geom.Curve{geom.NewLine(geom.NewPoint(0, y), geom.NewPoint(0.5, y))}
	closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), 1)}
	arr := geom.Regions(curves, closed, geom.WithVertexMerge(merge))
	requireExactBoundsReproduce(t, curves, closed, arr)

	const weldT = 0.25
	welded := 0
	for _, r := range arr.Regions {
		for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
			if e.SourceIndex != 1 { // the circle
				continue
			}
			if math.Abs(e.TStart-weldT) > 1e-9 && math.Abs(e.TEnd-weldT) > 1e-9 {
				continue
			}
			welded++
			require.Falsef(t, e.TExact,
				"a circle fragment bounded by the unexplained distance weld must not read exact: t=[%v %v]",
				e.TStart, e.TEnd)
		}
	}
	require.NotZero(t, welded, "the weld at t=0.25 must bound at least one emitted fragment")
}

func TestAnalyticExplainedWeldStaysExact(t *testing.T) {
	// The other direction: a weld the kernel DOES account for must keep its honest
	// exactness — an exact analytic cut is never laundered into a sampled one. The line
	// starts exactly at the circle's top sample vertex (0,1) and cuts a chord to the
	// EXACT on-circle point (0.8,0.6). Both crossings are certified contacts.
	//
	// Exactness is decided per emitted bound by whether its reported parameter evaluates
	// to the emitted polyline endpoint (the universal invariant), NOT by whether the
	// vertex merely happens to sit at some contact. The genuinely explained bounds — the
	// (0,1) crossing (t=0.25, a sample vertex) and the analytic cut at (0.8,0.6)
	// (t≈0.1024, a recorded cut whose point IS the vertex) — reproduce their polyline
	// endpoints and stay TExact; the fix does not over-taint them. The single seam-split
	// edge that reaches the (0.8,0.6) vertex via a nearby circle SAMPLE vertex (t=0.1016,
	// welded ~0.005 onto the contact) reports that sample parameter, which evaluates
	// ~0.005 off the vertex — so it is correctly inexact (blessing it would certify a
	// range a distance weld produced).
	curves := []geom.Curve{geom.NewLine(geom.NewPoint(0, 1), geom.NewPoint(2, 0))}
	closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), 1)}
	arr := geom.Regions(curves, closed, geom.WithVertexMerge(0.01))

	// Every TExact bound reproduces its polyline endpoints (no false certification), and
	// the genuinely explained crossings are NOT over-tainted: a fragment bounded by the
	// analytic cut at (0.8,0.6) stays exact.
	requireExactBoundsReproduce(t, curves, closed, arr)

	exactCut := 0
	for _, r := range arr.Regions {
		for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
			if e.SourceIndex != 1 { // the circle
				continue
			}
			// The bound at the analytic cut (0.8,0.6) is at circle param ≈0.10242 — a
			// non-sample value; a fragment carrying it must stay exact.
			onCut := math.Abs(e.TStart-0.10242) < 1e-4 || math.Abs(e.TEnd-0.10242) < 1e-4
			if onCut {
				require.Truef(t, e.TExact,
					"a fragment bounded by the exact analytic cut must stay exact (no over-taint): t=[%v %v]",
					e.TStart, e.TEnd)
				exactCut++
			}
		}
	}
	require.NotZero(t, exactCut, "the analytic cut at (0.8,0.6) must bound an emitted fragment")
}

func TestAnalyticChainedWeldEndpointIsInexact(t *testing.T) {
	// The vertex table welds a point onto the FIRST vertex within the merge tolerance
	// of it and keeps that vertex's coordinates, so the relation is not transitive:
	// three sources here chain into ONE graph vertex even though two of them are
	// farther apart than merge.
	//
	//	stub endpoint   (0, 0.991)  — inserted first, so it IS the vertex
	//	chord endpoint  (0, 0.982)  — 0.009 from the stub: welds
	//	circle sample   (0, 1)      — 0.009 from the stub: welds
	//
	// The chord's endpoint and the circle's sample are 0.018 apart — MORE than merge —
	// yet both canonicalize through the stub's representative. So the emitted chord
	// fragment starts at (0, 0.991), not at its own t=0 point (0, 0.982): the reported
	// range does not describe the emitted geometry and must not read exact. Nothing
	// pairwise can catch this (the chord's bound is the curve's own endpoint, which is
	// a join and is never cut, and its weld partner is a source endpoint too) — only
	// the vertex the bound actually landed on tells the truth.
	merge := 0.01
	curves := []geom.Curve{
		geom.NewLine(geom.NewPoint(0, 0.991), geom.NewPoint(-0.5, 0.5)), // the stub
		geom.NewLine(geom.NewPoint(0, 0.982), geom.NewPoint(1.5, 0.2)),  // the chord
	}
	closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), 1)}
	arr := geom.Regions(curves, closed, geom.WithVertexMerge(merge))
	requireExactBoundsReproduce(t, curves, closed, arr)

	chords := 0
	for _, r := range arr.Regions {
		for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
			if e.SourceIndex != 1 { // the chord
				continue
			}
			chords++
			require.Falsef(t, e.TExact,
				"a chord fragment bounded by the chained weld must not read exact: t=[%v %v] whole=%v",
				e.TStart, e.TEnd, e.Whole)
		}
	}
	require.NotZero(t, chords, "the chord must bound an emitted region")
}

func TestAnalyticSpurEndingOnCircleClean(t *testing.T) {
	// A line whose ENDPOINT lies exactly on a circle — the gear flank meeting its
	// root circle, and the shape a point-on-circle constraint produces. The contact
	// splits only the circle, at the exact point the line's own endpoint welds to,
	// so it is an ordinary T-junction: the spur is pruned and the disk stands whole.
	// The gate used to demand a sampled interior crossing as a witness, which a
	// contact at a segment endpoint can never produce, and flagged every one of
	// these degenerate.
	const R = 10.0
	c := geom.NewCircle(geom.NewPoint(0, 0), R)
	// 0° and 90° put the contact exactly ON a sample vertex at most densities; the
	// others fall between vertices.
	for _, deg := range []float64{0, 37, 90, 123.456, 200, 359.5} {
		th := deg * math.Pi / 180
		ex, ey := R*math.Cos(th), R*math.Sin(th)
		for _, spt := range []int{8, 16, 64, 256} {
			for _, dir := range []struct {
				name string
				far  float64 // multiple of the contact radius: >1 points out, <1 points in
			}{{"outward", 1.5}, {"inward", 0.4}} {
				curves := []geom.Curve{geom.NewLine(
					geom.NewPoint(ex, ey), geom.NewPoint(dir.far*ex, dir.far*ey))}
				closed := []geom.ClosedCurve{c}
				arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
				requireExactBoundsReproduce(t, curves, closed, arr)
				require.Falsef(t, arr.Degenerate,
					"a %s spur at %g° is a clean contact at spt=%d", dir.name, deg, spt)
				require.Lenf(t, arr.Regions, 1, "just the disk (%s, %g°, spt=%d)", dir.name, deg, spt)
				require.InDeltaf(t, math.Pi*R*R, arr.Regions[0].Area, 1e-9,
					"the spur takes nothing off the disk (%s, %g°, spt=%d)", dir.name, deg, spt)
			}
		}
	}
}

func TestAnalyticCrossingOnSampleVertexClean(t *testing.T) {
	// A chord crossing the circle exactly where the sampling puts vertices (y=0
	// hits the +x and -x sample vertices at any even density). The contact needs no
	// cut — the vertex is already there, at the true parameter — so it cannot show
	// up as a crossing interior to the circle's chords, and demanding that as a
	// witness false-flagged the whole arrangement.
	const R = 10.0
	for _, spt := range []int{4, 8, 64, 256} {
		curves := []geom.Curve{geom.NewLine(geom.NewPoint(-15, 0), geom.NewPoint(15, 0))}
		closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), R)}
		arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
		requireExactBoundsReproduce(t, curves, closed, arr)
		require.Falsef(t, arr.Degenerate, "a chord through two sample vertices is clean at spt=%d", spt)
		require.Lenf(t, arr.Regions, 2, "two half-disks at spt=%d", spt)
		var total float64
		for _, r := range arr.Regions {
			total += r.Area
		}
		require.InDeltaf(t, math.Pi*R*R, total, 1e-9, "the halves partition the disk at spt=%d", spt)
	}
}

func TestAnalyticSubSampleChordNeverBlessedWrong(t *testing.T) {
	// The soundness boundary of the two tests above. A line whose BOTH ends sit on
	// the circle needs no witness at either contact, so nothing in the gate measures
	// the stretch between them: when that stretch falls inside ONE sampled chord,
	// the line runs through the sliver outside the polygon and the planar map
	// collapses — the disk vanishes. The invariant is the same as for the shallow
	// secant: at any sampling the result is either right (the disk, split in two)
	// or Degenerate, never a blessed wrong one.
	const R = 5.0
	for _, halfDeg := range []float64{0.5, 2, 5, 15, 45, 90} {
		h := halfDeg * math.Pi / 180
		for _, base := range []float64{0, 0.37, 1.9} { // rotate the chord off the sample grid
			p0 := geom.NewPoint(R*math.Cos(base-h), R*math.Sin(base-h))
			p1 := geom.NewPoint(R*math.Cos(base+h), R*math.Sin(base+h))
			for spt := 3; spt <= 90; spt++ {
				arr := geom.Regions(
					[]geom.Curve{geom.NewLine(p0, p1)},
					[]geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), R)},
					geom.WithSegmentsPerTurn(spt))
				if arr.Degenerate {
					continue // conservatively rejected — sound
				}
				var total float64
				for _, r := range arr.Regions {
					total += r.Area
				}
				require.Lenf(t, arr.Regions, 2, "blessed half=%g° base=%g spt=%d splits the disk in two", halfDeg, base, spt)
				require.InDeltaf(t, math.Pi*R*R, total, 1e-5, "blessed half=%g° base=%g spt=%d partitions the disk", halfDeg, base, spt)
			}
		}
	}
}

func flattenHoles(r *geom.Region) []geom.BoundaryEdge {
	var out []geom.BoundaryEdge
	for _, h := range r.Holes {
		out = append(out, h...)
	}
	return out
}

// gearProbeCaseCCurves builds probe case C
// (.tmp/decad-2d-region-asks/probe/main.go, ask 2's motivating example): a root
// arc lying EXACTLY on a hub circle's carrier, closed by two flank lines and a tip
// line into a tooth. Mirrors the probe's own construction exactly.
func gearProbeCaseCCurves() (curves []geom.Curve, closed []geom.ClosedCurve, rootArcIdx, hubIdx int) {
	hub := geom.NewCircle(geom.NewPoint(0, 0), 10)
	ax, ay := 10*math.Cos(0.3), 10*math.Sin(0.3)
	rootArc := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(ax, -ay), geom.NewPoint(ax, ay))
	f1 := geom.NewLine(geom.NewPoint(ax, ay), geom.NewPoint(13, 1))
	tip := geom.NewLine(geom.NewPoint(13, 1), geom.NewPoint(13, -1))
	f2 := geom.NewLine(geom.NewPoint(13, -1), geom.NewPoint(ax, -ay))
	return []geom.Curve{rootArc, f1, tip, f2}, []geom.ClosedCurve{hub}, 0, 4
}

// TestAnalyticCoincidentCarrierResolvesGearTooth is probe case C's exact geometry
// (docs/coincident-carrier-resolution-design.md, ask 2's acceptance criteria): the
// arrangement must stop flagging Degenerate and return the hub disk (π·10²) and the
// tooth (≈11.864262) as two separate regions, with the coincident root-arc span
// reported under the SAME SourceIndex (the root arc, the lower of the pair) in both
// adjoining regions — never the hub circle in one and the arc in the other, the
// pre-resolution bug the design's problem statement describes. Sampling-density and
// scale independent, per "Determinism".
func TestAnalyticCoincidentCarrierResolvesGearTooth(t *testing.T) {
	for _, scale := range []float64{1, 0.001, 1000} {
		curves, closed, rootArcIdx, hubIdx := gearProbeCaseCCurves()
		if scale != 1 {
			hub := geom.NewCircle(geom.NewPoint(0, 0), 10*scale)
			ax, ay := 10*scale*math.Cos(0.3), 10*scale*math.Sin(0.3)
			rootArc := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(ax, -ay), geom.NewPoint(ax, ay))
			f1 := geom.NewLine(geom.NewPoint(ax, ay), geom.NewPoint(13*scale, scale))
			tip := geom.NewLine(geom.NewPoint(13*scale, scale), geom.NewPoint(13*scale, -scale))
			f2 := geom.NewLine(geom.NewPoint(13*scale, -scale), geom.NewPoint(ax, -ay))
			curves, closed = []geom.Curve{rootArc, f1, tip, f2}, []geom.ClosedCurve{hub}
		}
		for _, spt := range []int{3, 4, 5, 6, 7, 8, 16, 32, 64, 128} {
			arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
			require.Falsef(t, arr.Degenerate, "scale=%g spt=%d", scale, spt)
			require.Lenf(t, arr.Regions, 2, "scale=%g spt=%d: hub + tooth", scale, spt)
			requireExactBoundsReproduce(t, curves, closed, arr)

			var hubArea, toothArea float64
			var hubUsesRootArc, toothUsesRootArc, anyUsesHub bool
			for _, r := range arr.Regions {
				usesRootArc, usesHub := false, false
				for _, e := range r.Outer {
					usesRootArc = usesRootArc || e.SourceIndex == rootArcIdx
					usesHub = usesHub || e.SourceIndex == hubIdx
					// The merged span's own bound is the root arc's WHOLE domain (both
					// its natural ends), so it is unconditionally exact — but every
					// OTHER bound in this arrangement (the hub's own surviving fragment,
					// the flank/tip lines) is exact too, since every contact here is
					// either a resolved coincidence or a line-involved cut. Assert it on
					// every edge, not just the merged one, closing the "TExact must be
					// true on the merged span" acceptance criterion without singling out
					// one fragment by index.
					require.Truef(t, e.TExact, "scale=%g spt=%d: src=%d t=%v..%v", scale, spt, e.SourceIndex, e.TStart, e.TEnd)
				}
				if r.Area > math.Pi*25*scale*scale { // hub (π·100·scale²) vs tooth (≈11.86·scale²)
					hubArea, hubUsesRootArc = r.Area, usesRootArc
				} else {
					toothArea, toothUsesRootArc = r.Area, usesRootArc
					anyUsesHub = anyUsesHub || usesHub
				}
			}
			require.InDeltaf(t, math.Pi*100*scale*scale, hubArea, 1e-6*scale*scale, "scale=%g spt=%d", scale, spt)
			require.InDeltaf(t, 11.864262*scale*scale, toothArea, 1e-3*scale*scale, "scale=%g spt=%d", scale, spt)
			require.Truef(t, hubUsesRootArc, "scale=%g spt=%d: hub region must reuse the named root arc", scale, spt)
			require.Truef(t, toothUsesRootArc, "scale=%g spt=%d: tooth region must use the named root arc", scale, spt)
			require.Falsef(t, anyUsesHub, "scale=%g spt=%d: the tooth region must not fall back to hub-circle fragments", scale, spt)
		}
	}
}

// TestAnalyticCoincidentCarrierMajorArcResolves builds probe case C with the root
// arc's two endpoints swapped. That is NOT the same arc authored backwards:
// geom.Arc is {Center, Start, End} with no direction flag and Sweep() returns
// wrapSweep(start,end) ∈ (0,2π], so swapping the ends yields the COMPLEMENTARY
// major arc on the far side of the hub (0.6 rad becomes 2π−0.6 rad). A reversed arc
// has no representation here at all, so reversal invariance is not a property this
// package can state — the assertion below is about the major arc's own geometry.
//
// The overlap window is then the major arc's own near-full sweep, and the span the
// tooth sits on falls OUTSIDE it: the hub circle is the only source covering that
// span and correctly names it, while the coincident window elsewhere is named by
// the arc. Both regions still come out with the same areas, since the two arcs are
// complements and the answer either way is the hub disk plus the same tooth.
func TestAnalyticCoincidentCarrierMajorArcResolves(t *testing.T) {
	const rootArcIdx, hubIdx = 0, 4
	hub := geom.NewCircle(geom.NewPoint(0, 0), 10)
	ax, ay := 10*math.Cos(0.3), 10*math.Sin(0.3)
	rootArc := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(ax, ay), geom.NewPoint(ax, -ay))
	require.InDelta(t, 2*math.Pi-0.6, rootArc.Sweep(), 1e-12, "the swapped-endpoint arc is the complementary major arc")
	f1 := geom.NewLine(geom.NewPoint(ax, ay), geom.NewPoint(13, 1))
	tip := geom.NewLine(geom.NewPoint(13, 1), geom.NewPoint(13, -1))
	f2 := geom.NewLine(geom.NewPoint(13, -1), geom.NewPoint(ax, -ay))
	curves := []geom.Curve{rootArc, f1, tip, f2}
	closed := []geom.ClosedCurve{hub}

	arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(64))
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 2)
	requireExactBoundsReproduce(t, curves, closed, arr)

	var hubArea, toothArea float64
	var toothUsesRootArc, toothUsesHub bool
	for _, r := range arr.Regions {
		if r.Area > math.Pi*25 {
			hubArea = r.Area
			continue
		}
		toothArea = r.Area
		for _, e := range r.Outer {
			toothUsesRootArc = toothUsesRootArc || e.SourceIndex == rootArcIdx
			toothUsesHub = toothUsesHub || e.SourceIndex == hubIdx
		}
	}
	require.InDelta(t, math.Pi*100, hubArea, 1e-6)
	require.InDelta(t, 11.864262, toothArea, 1e-3)
	require.False(t, toothUsesRootArc, "the major arc does not sweep the tooth's root span")
	require.True(t, toothUsesHub, "so the hub circle is the only source covering it, and names it")
}

// TestAnalyticCoincidentCarrierCompetingCutStaysDegenerate is the postcondition
// guard: a suppression window is acted on only when its two boundaries survive
// split's per-segment dedup as distinct fragment bounds shared by both sources.
//
// The fixture puts a radial line's crossing a hair OUTSIDE the overlap window, so
// the line's cut on the circle and the window's own lower boundary land on the same
// tiny segment. split's dedup keeps only the first of two boundaries closer than
// segEps in that segment's LOCAL chord parameter, so the window boundary is dropped
// — while nothing at analytic-pass time can see it coming, since the collapse is a
// global operation over every cut on the segment, decided by a competing cut from an
// unrelated pair. Suppressing against the dropped boundary detached the tooth span
// from the arc that was to represent it, and the whole arrangement pruned away:
// zero regions with Degenerate=false, a silent false bless of a drawn unit disk.
//
// The separation is 5e-11 rad with a vertex merge (1e-12) below it, so the two cuts
// cannot weld into one vertex instead; the collapse threshold is a segEps fraction
// of one chord, so it clears at high enough density.
func TestAnalyticCoincidentCarrierCompetingCutStaysDegenerate(t *testing.T) {
	const theta, delta = 0.3, 5e-11
	c := geom.NewPoint(0, 0)
	at := func(r, ang float64) *geom.Point {
		return geom.NewPoint(r*math.Cos(ang), r*math.Sin(ang))
	}
	build := func(spt int) *geom.Arrangement {
		root := geom.NewArc(c, at(1, theta), at(1, math.Pi))
		line := geom.NewLine(at(0.5, theta-delta), at(1.5, theta-delta))
		return geom.Regions([]geom.Curve{root, line}, []geom.ClosedCurve{geom.NewCircle(c, 1)},
			geom.WithVertexMerge(1e-12), geom.WithSegmentsPerTurn(spt))
	}
	requireDisk := func(t *testing.T, arr *geom.Arrangement, spt int) {
		t.Helper()
		require.Lenf(t, arr.Regions, 1, "spt=%d: the drawn unit disk must never vanish", spt)
		require.InDeltaf(t, math.Pi, arr.Regions[0].Area, 1e-9, "spt=%d", spt)
	}

	// Below the density at which a chord is wide enough to separate the two cuts,
	// the window boundary is deduped away and the resolution must be withdrawn.
	for _, spt := range []int{16, 32, 64, 96} {
		arr := build(spt)
		require.Truef(t, arr.Degenerate,
			"spt=%d: a window boundary a competing cut deduplicates away must be refused, not suppressed against", spt)
		requireDisk(t, arr, spt)
	}

	// Above it both boundaries survive as distinct bounds and the pair resolves.
	for _, spt := range []int{128, 256} {
		arr := build(spt)
		require.Falsef(t, arr.Degenerate, "spt=%d: both boundaries survive the dedup, so the pair resolves", spt)
		requireDisk(t, arr, spt)
	}

	// The same line a hair INSIDE the window is unaffected: there the window's own
	// boundary is the survivor, so the resolution stands at every density.
	for _, spt := range []int{16, 64, 256} {
		root := geom.NewArc(c, at(1, theta), at(1, math.Pi))
		line := geom.NewLine(at(0.5, theta+delta), at(1.5, theta+delta))
		arr := geom.Regions([]geom.Curve{root, line}, []geom.ClosedCurve{geom.NewCircle(c, 1)},
			geom.WithVertexMerge(1e-12), geom.WithSegmentsPerTurn(spt))
		require.Falsef(t, arr.Degenerate, "spt=%d: a crossing inside the window keeps the boundary", spt)
		requireDisk(t, arr, spt)
	}
}

// TestAnalyticCoincidentCarrierMultiWindowStaysDegenerate is the "Scope" exclusion:
// a pair whose sweeps overlap in more than one disjoint angular window is left
// Degenerate — the pre-existing `coincidentArcOverlap` limit (reporting only the
// longest contiguous overlap) means resolving just one window would silently drop
// the other, so the design leaves the whole pair refused rather than resolve
// unsoundly. Two near-full arcs, each covering all but a small gap, on the same
// carrier, positioned so the gaps do not align: the sweeps overlap in two separate
// places (on either side of the two gaps).
func TestAnalyticCoincidentCarrierMultiWindowStaysDegenerate(t *testing.T) {
	c := geom.NewPoint(0, 0)
	at := func(ang float64) *geom.Point { return geom.NewPoint(5*math.Cos(ang), 5*math.Sin(ang)) }
	// a: gap centered at angle 0 (spans 10°..350°, i.e. [10°,350°] the long way through 180°).
	a := geom.NewArc(c, at(10*math.Pi/180), at(350*math.Pi/180))
	// b: gap centered at angle 180° (spans 190°..170°, the long way through 0°).
	b := geom.NewArc(c, at(190*math.Pi/180), at(170*math.Pi/180))
	arr := geom.Regions([]geom.Curve{a, b}, nil, geom.WithSegmentsPerTurn(64))
	require.True(t, arr.Degenerate, "a multi-window coincident overlap stays refused")
}

// TestAnalyticCoincidentCarrierNearCertifyStaysDegenerate is the refusal-band
// regression guard for a genuine arc sweep (docs/coincident-carrier-resolution-design.md's
// "The refusal band"): a carrier pair whose radius differs by just inside the
// ambiguous band (between tangentCertify and tangentBand) must never be resolved —
// only an EXACTLY certified carrier match may merge.
func TestAnalyticCoincidentCarrierNearCertifyStaysDegenerate(t *testing.T) {
	c := geom.NewPoint(0, 0)
	at := func(r, ang float64) *geom.Point { return geom.NewPoint(r*math.Cos(ang), r*math.Sin(ang)) }
	const r = 5.0
	const offBand = 1.5e-6 * r // 1.5×tangentBand×scale — inside the ambiguous zone, not certified
	a := geom.NewArc(c, at(r, 0), at(r, math.Pi))
	b := geom.NewArc(c, at(r+offBand, math.Pi/2), at(r+offBand, 3*math.Pi/2))
	arr := geom.Regions([]geom.Curve{a, b}, nil, geom.WithSegmentsPerTurn(32))
	require.True(t, arr.Degenerate, "a near-certify (not exact) carrier match stays refused, never resolved")
}

// TestAnalyticCoincidentCarrierMultiTooth exercises a single losing source (the
// hub circle) suppressed against SEVERAL different named sources (multiple teeth),
// the actual gear workload (docs/coincident-carrier-resolution-design.md: "12–45
// teeth per gear"): each tooth's root arc must independently cut and suppress its
// own span of the hub, with no interference between teeth.
func TestAnalyticCoincidentCarrierMultiTooth(t *testing.T) {
	hub := geom.NewCircle(geom.NewPoint(0, 0), 10)
	tooth := func(center float64) (arc geom.Curve, f1, tip, f2 geom.Curve) {
		half := 0.15
		ax0, ay0 := 10*math.Cos(center-half), 10*math.Sin(center-half)
		ax1, ay1 := 10*math.Cos(center+half), 10*math.Sin(center+half)
		tipx0, tipy0 := 13*math.Cos(center-half*0.6), 13*math.Sin(center-half*0.6)
		tipx1, tipy1 := 13*math.Cos(center+half*0.6), 13*math.Sin(center+half*0.6)
		root := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(ax0, ay0), geom.NewPoint(ax1, ay1))
		flank1 := geom.NewLine(geom.NewPoint(ax1, ay1), geom.NewPoint(tipx1, tipy1))
		tipLine := geom.NewLine(geom.NewPoint(tipx1, tipy1), geom.NewPoint(tipx0, tipy0))
		flank2 := geom.NewLine(geom.NewPoint(tipx0, tipy0), geom.NewPoint(ax0, ay0))
		return root, flank1, tipLine, flank2
	}
	var curves []geom.Curve
	const n = 5
	centers := make([]float64, n) // evenly spaced, well clear of each tooth's own 0.3 rad width
	for i := range centers {
		centers[i] = float64(i) * 2 * math.Pi / n
	}
	for _, center := range centers {
		root, f1, tip, f2 := tooth(center)
		curves = append(curves, root, f1, tip, f2)
	}
	arr := geom.Regions(curves, []geom.ClosedCurve{hub}, geom.WithSegmentsPerTurn(64))
	require.False(t, arr.Degenerate, "several teeth on the same hub all resolve")
	require.Len(t, arr.Regions, n+1, "one region per tooth, plus the hub")
	requireExactBoundsReproduce(t, curves, []geom.ClosedCurve{hub}, arr)

	var hubArea float64
	var toothAreas []float64
	for _, r := range arr.Regions {
		if r.Area > 50 { // the hub disk is π·100; each tooth is a couple of square units
			hubArea = r.Area
			continue
		}
		toothAreas = append(toothAreas, r.Area)
	}
	require.InDelta(t, math.Pi*100, hubArea, 1e-6, "the hub disk area is unaffected by how many teeth cut into it")
	require.Lenf(t, toothAreas, n, "every tooth produced its own region")
	for i, a := range toothAreas {
		require.InDeltaf(t, toothAreas[0], a, 1e-9, "tooth %d: congruent teeth (rotated copies) have identical area", i)
	}
}

// TestAnalyticCoincidentCarrierInCertifyBandStaysDegenerate is the unequal-carrier
// regression for docs/coincident-carrier-resolution-design.md's "The refusal band".
// A root arc CONCENTRIC with the hub circle but a hair larger in radius is still
// classified coincident (the offset sits inside certify = scale·tangentCertify), yet
// the two carriers are different curves: resolution would compute both window
// boundary points on one carrier and cut BOTH sources there as exact, so the
// surviving fragment's reported bounds would evaluate somewhere off the emitted
// polyline. Only a carrier match at round-off (carriersIdentical) may resolve; a
// certify-band match stays Degenerate.
//
// The deltas below straddle the band: each is well inside certify (scale ≈ 23 here,
// so certify ≈ 2.3e-8) yet orders of magnitude above either half of the identity band
// carriersIdentical applies (weldIdentEps·scale ≈ 2.3e-11, and the carrier-local
// weldIdentEps·r = 1e-11), which is exactly the region that used to resolve unsoundly.
func TestAnalyticCoincidentCarrierInCertifyBandStaysDegenerate(t *testing.T) {
	for _, delta := range []float64{5e-9, 1e-8, 2e-8} {
		hub := geom.NewCircle(geom.NewPoint(0, 0), 10)
		r := 10 + delta
		ax, ay := r*math.Cos(0.3), r*math.Sin(0.3)
		rootArc := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(ax, -ay), geom.NewPoint(ax, ay))
		f1 := geom.NewLine(geom.NewPoint(ax, ay), geom.NewPoint(13, 1))
		tip := geom.NewLine(geom.NewPoint(13, 1), geom.NewPoint(13, -1))
		f2 := geom.NewLine(geom.NewPoint(13, -1), geom.NewPoint(ax, -ay))
		curves := []geom.Curve{rootArc, f1, tip, f2}
		closed := []geom.ClosedCurve{hub}

		arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(64))
		require.Truef(t, arr.Degenerate,
			"delta=%g: a carrier equal only within the certify band must never resolve", delta)
		// Whatever regions the sampled fallback still reports, no bound of them may
		// claim an exactness it cannot reproduce.
		requireExactBoundsReproduce(t, curves, closed, arr)
	}
}

// TestAnalyticCoincidentCarrierDistantSceneStaysDegenerate pins that the identity
// gate admitting a coincident pair for resolution is CARRIER-LOCAL: it may not be
// widened by geometry that has nothing to do with the pair. An r=2 arc and an r=1
// circle share a centre and are a whole unit apart in radius — never the same
// carrier — but a line far out at x=1e15 stretches the arrangement's global scale,
// and an identity band derived from that scale grows with it. Blessing the pair
// there let resolveCoincidentOverlap record a suppression window over the whole
// circle carrier, for a pair that shares no carrier at all.
//
// The same pair without the distant line is ordinary disjoint geometry, so the far
// line is doing all the work — that is the point of the fixture.
func TestAnalyticCoincidentCarrierDistantSceneStaysDegenerate(t *testing.T) {
	c := geom.NewPoint(0, 0)
	at := func(r, ang float64) *geom.Point { return geom.NewPoint(r*math.Cos(ang), r*math.Sin(ang)) }
	arc := func() geom.Curve { return geom.NewArc(c, at(2, 0), at(2, math.Pi/2)) }
	circle := func() geom.ClosedCurve { return geom.NewCircle(c, 1) }

	far := geom.NewLine(geom.NewPoint(1e15, 0), geom.NewPoint(1e15, 1))
	stretched := geom.Regions([]geom.Curve{arc(), far}, []geom.ClosedCurve{circle()},
		geom.WithVertexMerge(1e-9))
	require.True(t, stretched.Degenerate,
		"carriers a unit apart must stay refused however far the rest of the scene reaches")

	alone := geom.Regions([]geom.Curve{arc()}, []geom.ClosedCurve{circle()},
		geom.WithVertexMerge(1e-9))
	require.False(t, alone.Degenerate, "the pair on its own is plain concentric geometry")
	require.Len(t, alone.Regions, 1, "the unit disk survives; the arc is a pruned spur")
	require.InDelta(t, math.Pi, alone.Regions[0].Area, 1e-9)
}

// TestAnalyticCoincidentCarrierHalfTurnFragmentSurvives pins that a suppression
// window classifies a fragment by the source EVALUATED at the fragment's parameter
// midpoint, not by the midpoint of the fragment's chord. densify floors a source at
// two tiny segments, so a coincident circle at a low WithSegmentsPerTurn is two
// semicircle fragments whose chords are diameters — each chord midpoint is the
// carrier CENTRE, where the window's angle test reads atan2(0,0)=0 and answers about
// nothing. With the centre offset far enough for the perpendicular residue to cancel
// (any offset of order 1 and up), both semicircles then read as angle 0, both were
// suppressed, and the disk vanished with Degenerate=false.
func TestAnalyticCoincidentCarrierHalfTurnFragmentSurvives(t *testing.T) {
	for _, cy := range []float64{0, 1, 10, 1e3} {
		for _, spt := range []int{1, 2, 3, 4, 8, 64} {
			c := geom.NewPoint(0, cy)
			arc := geom.NewArc(c, geom.NewPoint(1, cy), geom.NewPoint(-1, cy)) // sweep [0,π]
			arr := geom.Regions([]geom.Curve{arc}, []geom.ClosedCurve{geom.NewCircle(c, 1)},
				geom.WithSegmentsPerTurn(spt))
			require.Falsef(t, arr.Degenerate, "cy=%g spt=%d: an ordinary single-window overlap resolves", cy, spt)
			require.Lenf(t, arr.Regions, 1, "cy=%g spt=%d: the disk closes over the arc plus the circle's other half", cy, spt)
			require.InDeltaf(t, math.Pi, arr.Regions[0].Area, 1e-9, "cy=%g spt=%d", cy, spt)
		}
	}
}

// TestAnalyticCoincidentCarrierUnsplitBoundaryStaysDegenerate pins that a
// suppression window is recorded only when both of its boundaries really become
// fragment boundaries on both sources. cutSite declines to cut whenever a boundary's
// local chord parameter falls within segEps of a segment end — a PARAMETER test — so
// at coarse sampling a boundary sitting a whole vertex-merge tolerance away from the
// nearest sample vertex can still record no cut. The window was recorded anyway, and
// split then suppressed the unsplit whole-segment fragments, deleting the hair that
// closes the region with Degenerate=false and no region at all.
//
// The fixture is a unit circle and an exactly coincident arc whose gap is 1.2e-9 rad
// with a vertex merge (1e-10) below it, so the hair cannot weld shut instead: the
// overlap boundary sits at source parameter t≈9.55e-11, whose local parameter t·spt
// stays under segEps for spt ≤ 10.
func TestAnalyticCoincidentCarrierUnsplitBoundaryStaysDegenerate(t *testing.T) {
	c := geom.NewPoint(0, 0)
	at := func(ang float64) *geom.Point { return geom.NewPoint(math.Cos(ang), math.Sin(ang)) }
	half := 0.6e-9
	build := func(spt int) *geom.Arrangement {
		return geom.Regions([]geom.Curve{geom.NewArc(c, at(half), at(-half))},
			[]geom.ClosedCurve{geom.NewCircle(c, 1)},
			geom.WithVertexMerge(1e-10), geom.WithSegmentsPerTurn(spt))
	}

	for _, spt := range []int{2, 3, 4, 6, 8, 10} {
		arr := build(spt)
		require.Truef(t, arr.Degenerate,
			"spt=%d: a window boundary that materializes no split must be refused, not suppressed against", spt)
	}

	// Above that density the boundaries cut normally and the resolution is unchanged.
	for _, spt := range []int{11, 16, 64, 256} {
		arr := build(spt)
		require.Falsef(t, arr.Degenerate, "spt=%d: both boundaries cut, so the pair resolves", spt)
		require.Lenf(t, arr.Regions, 1, "spt=%d", spt)
		require.InDeltaf(t, math.Pi, arr.Regions[0].Area, 1e-9, "spt=%d", spt)
	}
}

// TestAnalyticCoincidentCarrierNearFullGapStaysClosed covers a coincident overlap
// whose window leaves only a hair of the losing source outside it: a unit circle and
// an exactly coincident arc sweeping all but `gap` radians. The circle's exterior
// fragment over that gap is the ONLY thing left to close the disk, so suppression
// must act on the overlap window itself and carry no outward slop — a slop of
// `arcParamEps` swallowed the gap fragment for every gap up to twice it, and the
// region vanished with no degeneracy flag to warn anyone.
func TestAnalyticCoincidentCarrierNearFullGapStaysClosed(t *testing.T) {
	c := geom.NewPoint(0, 0)
	at := func(ang float64) *geom.Point { return geom.NewPoint(math.Cos(ang), math.Sin(ang)) }
	// The gap is centred on angle 0, a sample vertex at every density below, so the
	// two cuts straddle a segment boundary — the alignment that leaves them least room.
	for _, spt := range []int{16, 64, 256} {
		for _, gap := range []float64{1.2e-9, 1.5e-9, 2e-9, 2.5e-9, 1e-7, 1e-3} {
			arc := geom.NewArc(c, at(gap/2), at(-gap/2)) // sweep 2π−gap, coincident with the circle
			closed := []geom.ClosedCurve{geom.NewCircle(c, 1)}
			arr := geom.Regions([]geom.Curve{arc}, closed,
				geom.WithVertexMerge(1e-10), geom.WithSegmentsPerTurn(spt))
			require.Falsef(t, arr.Degenerate, "spt=%d gap=%g resolves like any other single-window overlap", spt, gap)
			require.Lenf(t, arr.Regions, 1, "spt=%d gap=%g: the disk closes over the arc plus the circle's gap fragment", spt, gap)
			require.InDeltaf(t, math.Pi, arr.Regions[0].Area, 1e-9, "spt=%d gap=%g", spt, gap)
		}
	}

	// Below arcParamEps the arc is a COMPLETE carrier (coversFullTurn), which is the
	// separate out-of-scope exclusion — refused, not resolved. That boundary is
	// unchanged; it is asserted here so the two thresholds are not confused.
	tiny := geom.NewArc(c, at(5e-10), at(-5e-10))
	arr := geom.Regions([]geom.Curve{tiny}, []geom.ClosedCurve{geom.NewCircle(c, 1)},
		geom.WithVertexMerge(1e-10), geom.WithSegmentsPerTurn(64))
	require.True(t, arr.Degenerate, "a 2π-within-arcParamEps arc is a complete carrier, so the pair stays refused")
}

// TestAnalyticCoincidentCarrierFullTurnArcStaysDegenerate is the "Scope" exclusion
// keyed on GEOMETRY rather than on the srcCircle flag: geom.NewArc(c, p, p) sweeps a
// full turn (wrapSweep maps a non-positive delta to 2π), so it is a complete carrier
// with no finite domain end to bound an overlap window — exactly the "two
// fully-coincident COMPLETE carriers" case the design leaves refused. Keying the
// exclusion on the flag instead let such an arc pair with a real circle, resolve, and
// suppress the whole circle carrier.
func TestAnalyticCoincidentCarrierFullTurnArcStaysDegenerate(t *testing.T) {
	c := geom.NewPoint(0, 0)
	p := geom.NewPoint(10, 0)
	fullArc := func() geom.Curve { return geom.NewArc(c, p, p) }

	t.Run("full-turn arc vs coincident circle", func(t *testing.T) {
		arr := geom.Regions([]geom.Curve{fullArc()},
			[]geom.ClosedCurve{geom.NewCircle(c, 10)}, geom.WithSegmentsPerTurn(32))
		require.True(t, arr.Degenerate, "a 2π arc is a complete carrier, so the pair is two coincident full circles")
	})

	t.Run("full-turn arc vs full-turn arc", func(t *testing.T) {
		arr := geom.Regions([]geom.Curve{fullArc(), fullArc()}, nil, geom.WithSegmentsPerTurn(32))
		require.True(t, arr.Degenerate, "two complete carriers have no resolvable overlap window")
	})

	t.Run("two coincident circles still refused", func(t *testing.T) {
		arr := geom.Regions(nil, []geom.ClosedCurve{
			geom.NewCircle(c, 10), geom.NewCircle(c, 10),
		}, geom.WithSegmentsPerTurn(32))
		require.True(t, arr.Degenerate, "the flag-keyed exclusion this generalizes still holds")
	})
}

// TestAnalyticCoincidentCarrierSubEpsilonSecondWindowStaysDegenerate pins that ANY
// positive second overlap window makes a coincident pair multi-window, and so refused
// (docs/coincident-carrier-resolution-design.md's "Scope"). Arc a spans [0,π]; arc b
// spans [π/2, 2π+5e-10], so the two share [π/2,π] AND, separately, [0,5e-10]. Only
// one suppression window is ever recorded, so treating the second — however short —
// as absent would leave that span emitted by both sources as an un-deduplicated
// coincident boundary, with no Degenerate flag left to warn the consumer.
func TestAnalyticCoincidentCarrierSubEpsilonSecondWindowStaysDegenerate(t *testing.T) {
	c := geom.NewPoint(0, 0)
	at := func(ang float64) *geom.Point { return geom.NewPoint(5*math.Cos(ang), 5*math.Sin(ang)) }
	for _, second := range []float64{5e-10, 1e-9, 2e-9, 1e-8} {
		a := geom.NewArc(c, at(0), at(math.Pi))
		b := geom.NewArc(c, at(math.Pi/2), at(second))
		arr := geom.Regions([]geom.Curve{a, b}, nil,
			geom.WithVertexMerge(1e-12), geom.WithSegmentsPerTurn(64))
		require.Truef(t, arr.Degenerate,
			"second window %g rad is positive, so the pair is multi-window and stays refused", second)
	}
}

// TestAnalyticCoincidentCarrierBoundOnSampleVertexStaysExact covers the weld audit:
// when an overlap boundary point lands exactly on a hub SAMPLE vertex, the two
// sources' tiny-segment endpoints weld there, and auditMergedEndpoints asks
// eventExplains whether an analytic contact sits at that weld. The resolvable
// overlap's authoritative contacts are its two window BOUNDARY points — e.x/e.y is
// only the window midpoint — so answering from the midpoint alone tainted the exact
// cuts the resolution had just made, and the surviving hub fragment reported
// TExact=false. All four alignments (neither / lo / hi / both bounds on a sample
// vertex) must read exact.
func TestAnalyticCoincidentCarrierBoundOnSampleVertexStaysExact(t *testing.T) {
	const hubIdx = 4 // one open curve per tooth edge (4), then the closed hub
	for _, spt := range []int{4, 8, 64} {
		// At spt segments per turn the hub's sample vertices sit at multiples of
		// 2π/spt; a tooth of width 0.3 rad placed at lo hits none, one or both.
		step := 2 * math.Pi / float64(spt)
		for _, lo := range []float64{0.15, 0, step - 0.3, step} {
			hub := geom.NewCircle(geom.NewPoint(0, 0), 10)
			p := func(ang float64) *geom.Point {
				return geom.NewPoint(10*math.Cos(ang), 10*math.Sin(ang))
			}
			q := func(ang float64) *geom.Point {
				return geom.NewPoint(13*math.Cos(ang), 13*math.Sin(ang))
			}
			rootArc := geom.NewArc(geom.NewPoint(0, 0), p(lo), p(lo+0.3))
			f1 := geom.NewLine(p(lo+0.3), q(lo+0.25))
			tip := geom.NewLine(q(lo+0.25), q(lo+0.05))
			f2 := geom.NewLine(q(lo+0.05), p(lo))
			curves := []geom.Curve{rootArc, f1, tip, f2}
			closed := []geom.ClosedCurve{hub}

			arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(spt))
			require.Falsef(t, arr.Degenerate, "spt=%d lo=%v", spt, lo)
			require.Lenf(t, arr.Regions, 2, "spt=%d lo=%v: hub + tooth", spt, lo)
			requireExactBoundsReproduce(t, curves, closed, arr)

			var sawHub bool
			for _, r := range arr.Regions {
				for _, e := range r.Outer {
					require.Truef(t, e.TExact,
						"spt=%d lo=%v: src=%d t=%v..%v must stay exact through the overlap weld",
						spt, lo, e.SourceIndex, e.TStart, e.TEnd)
					sawHub = sawHub || e.SourceIndex == hubIdx
				}
			}
			require.Truef(t, sawHub, "spt=%d lo=%v: the hub's surviving fragment is the bound under test", spt, lo)
		}
	}
}

// TestAnalyticCoincidentCarrierCollapsedWindowStaysDegenerate is the third clause of
// certifySuppression's postcondition: the two window boundaries must resolve to
// DISTINCT graph vertices. The other two clauses — each boundary resolving to a
// vertex that bounds a fragment of the losing source AND of the named source, and it
// being the SAME vertex on both — are satisfied here, so this clause alone decides
// the verdict.
//
// Two arcs on one carrier: arc a sweeps [0,π] and arc b sweeps [π−w,2π], so the pair
// overlaps in the single window [π−w,π] and, taken together, the two close the unit
// disk. With w below the vertex merge the window's two boundary points are nearer to
// each other than the welding radius, so both canonicalize onto ONE graph vertex —
// which is also a bound of a fragment of each source, a's endpoint at π and b's at
// π−w having welded there too. All four lookups therefore succeed and agree across
// the sources, and only "the two must be distinct" is left to refuse the window.
//
// The span between two boundaries that are one vertex was never emitted, so nothing
// downstream could attach the named source's edge where the losing source's kept
// fragments end. Withdrawing is the conservative answer: the pair is flagged and the
// losing source emits its coincident span, so the disk stays drawn — a doubled sliver
// of width w wide rather than a region silently deleted.
func TestAnalyticCoincidentCarrierCollapsedWindowStaysDegenerate(t *testing.T) {
	c := geom.NewPoint(0, 0)
	at := func(ang float64) *geom.Point { return geom.NewPoint(math.Cos(ang), math.Sin(ang)) }
	const merge = 1e-6
	build := func(w float64, spt int) ([]geom.Curve, *geom.Arrangement) {
		curves := []geom.Curve{
			geom.NewArc(c, at(0), at(math.Pi)),
			geom.NewArc(c, at(math.Pi-w), at(2*math.Pi)),
		}
		return curves, geom.Regions(curves, nil,
			geom.WithVertexMerge(merge), geom.WithSegmentsPerTurn(spt))
	}

	// Window narrower than the merge: the two boundaries collapse onto one vertex.
	// The collapse is a welding-radius fact, not a sampling one, so every density
	// must refuse.
	for _, w := range []float64{1e-9, 1e-8, 1e-7} {
		for _, spt := range []int{8, 16, 64, 256, 1024} {
			curves, arr := build(w, spt)
			require.Truef(t, arr.Degenerate,
				"w=%g spt=%d: a window whose boundaries are one vertex spans nothing, so it must be withdrawn", w, spt)
			require.Lenf(t, arr.Regions, 1, "w=%g spt=%d: the drawn disk must survive the withdrawal", w, spt)
			require.InDeltaf(t, math.Pi, arr.Regions[0].Area, 2*w,
				"w=%g spt=%d: the disk keeps its drawn area up to the un-suppressed sliver", w, spt)
			requireExactBoundsReproduce(t, curves, nil, arr)
		}
	}

	// The same construction with the window wider than the merge is the control: the
	// boundaries stay two vertices, every clause holds, and the pair resolves to the
	// plain disk with no sliver.
	for _, w := range []float64{1e-5, 1e-4, 1e-3} {
		for _, spt := range []int{8, 16, 64, 256, 1024} {
			curves, arr := build(w, spt)
			require.Falsef(t, arr.Degenerate,
				"w=%g spt=%d: boundaries a merge apart are distinct vertices, so the window stands", w, spt)
			require.Lenf(t, arr.Regions, 1, "w=%g spt=%d", w, spt)
			require.InDeltaf(t, math.Pi, arr.Regions[0].Area, 1e-9, "w=%g spt=%d", w, spt)
			requireExactBoundsReproduce(t, curves, nil, arr)
		}
	}
}
