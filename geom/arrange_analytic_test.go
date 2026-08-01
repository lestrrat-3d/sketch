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

// TestAnalyticCoincidentCarrierReversalInvariant is the "Curve reversal" half of
// docs/coincident-carrier-resolution-design.md's "Determinism": reversing the root
// arc's own authored direction (building it End-to-Start instead of Start-to-End)
// changes only that source's own natural-parameter direction, never its position
// in the input list — so it must not change which source is named, the region
// count, or the areas.
func TestAnalyticCoincidentCarrierReversalInvariant(t *testing.T) {
	hub := geom.NewCircle(geom.NewPoint(0, 0), 10)
	ax, ay := 10*math.Cos(0.3), 10*math.Sin(0.3)
	rootArc := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(ax, ay), geom.NewPoint(ax, -ay)) // reversed vs gearProbeCaseCCurves
	f1 := geom.NewLine(geom.NewPoint(ax, ay), geom.NewPoint(13, 1))
	tip := geom.NewLine(geom.NewPoint(13, 1), geom.NewPoint(13, -1))
	f2 := geom.NewLine(geom.NewPoint(13, -1), geom.NewPoint(ax, -ay))
	curves := []geom.Curve{rootArc, f1, tip, f2}
	closed := []geom.ClosedCurve{hub}

	arr := geom.Regions(curves, closed, geom.WithSegmentsPerTurn(64))
	require.False(t, arr.Degenerate)
	require.Len(t, arr.Regions, 2)
	requireExactBoundsReproduce(t, curves, closed, arr)
	var total float64
	for _, r := range arr.Regions {
		total += r.Area
	}
	require.InDelta(t, math.Pi*100+11.864262, total, 1e-3)
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
