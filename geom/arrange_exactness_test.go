package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// ellipsePt maps an eccentric angle on a rotated ellipse to world coordinates —
// the same formula the arrangement's densifier uses (geom/sample.go), reproduced
// here so the exactness invariant is an INDEPENDENT oracle.
func ellipsePt(cx, cy, rx, ry, rot, ang float64) [2]float64 {
	lx, ly := rx*math.Cos(ang), ry*math.Sin(ang)
	cosr, sinr := math.Cos(rot), math.Sin(rot)
	return [2]float64{cx + lx*cosr - ly*sinr, cy + lx*sinr + ly*cosr}
}

// evalSource evaluates a source curve at the arrangement's normalized parameter t
// (the convention documented on BoundaryEdge.TStart) for EVERY source type, so the
// exactness invariant can be checked mechanically on any fixture, present or future.
// It reconstructs the point from the curve's public geometry and the documented
// parameterization — matching source.at in arrange.go — without touching the
// arrangement that produced TStart/TEnd, so it is an independent oracle. ok is false
// ONLY for an unknown Curve implementation, which means the invariant needs a new case,
// not that TExact is a defect.
func evalSource(t *testing.T, curves []geom.Curve, closed []geom.ClosedCurve, idx int, tp float64) ([2]float64, bool) {
	t.Helper()
	var c interface{}
	switch {
	case idx < len(curves):
		c = curves[idx]
	case idx-len(curves) < len(closed):
		c = closed[idx-len(curves)]
	default:
		t.Fatalf("BoundaryEdge.SourceIndex %d out of range (%d curves, %d closed)", idx, len(curves), len(closed))
	}
	switch s := c.(type) {
	case *geom.Line:
		return [2]float64{s.Start.X + tp*(s.End.X-s.Start.X), s.Start.Y + tp*(s.End.Y-s.Start.Y)}, true
	case *geom.Arc:
		ang := s.StartAngle() + s.Sweep()*tp
		r := s.Radius()
		return [2]float64{s.Center.X + r*math.Cos(ang), s.Center.Y + r*math.Sin(ang)}, true
	case *geom.Circle:
		ang := 2 * math.Pi * tp
		return [2]float64{s.Center.X + s.Radius*math.Cos(ang), s.Center.Y + s.Radius*math.Sin(ang)}, true
	case *geom.Ellipse:
		return ellipsePt(s.Center.X, s.Center.Y, s.Rx, s.Ry, s.Rotation, 2*math.Pi*tp), true
	case *geom.EllipticalArc:
		return ellipsePt(s.Center.X, s.Center.Y, s.Rx, s.Ry, s.Rotation, s.StartParam()+tp*s.Sweep()), true
	case *geom.Conic:
		x, y := s.Eval(tp)
		return [2]float64{x, y}, true
	case *geom.Spline:
		x, y := s.Eval(tp)
		return [2]float64{x, y}, true
	case *geom.ClosedSpline:
		x, y := s.Eval(tp)
		return [2]float64{x, y}, true
	case *geom.FitSpline:
		x, y := s.Eval(tp)
		return [2]float64{x, y}, true
	case *geom.NURBS:
		lo, hi := s.Domain()
		x, y := s.Eval(lo + (hi-lo)*tp)
		return [2]float64{x, y}, true
	}
	return [2]float64{}, false
}

// requireExactBoundsReproduce is the UNIVERSAL exactness invariant. TExact means the
// reported [TStart,TEnd] are the TRUE source parameters — so evaluating the source
// curve at TStart and TEnd MUST reproduce the emitted polyline's endpoints to machine
// precision. That is the definition of "exact", and it is checkable mechanically for
// EVERY source type (line/circle/arc AND the free-form ellipse/elliptical-arc/conic/
// spline/closed-spline/fit-spline/NURBS), present or future. This is the property the
// whole class of false-certification defects violated: a bound reports a sample/cut/
// pinned parameter that does NOT evaluate to the vertex it was emitted at.
//
// Crucially it covers the pinned-endpoint hole: an elliptical arc pins its polyline
// ends to sketch points off the parametric ellipse, so eval(reported param) misses them
// by the solver tolerance. Before the fix such a whole edge reported TExact=true, and
// this invariant — now evaluating the actual EllipticalArc — catches the ~0.005 miss.
// (An unknown Curve implementation makes evalSource return ok=false; that is a test gap
// to fill with a new case, and the invariant fails loudly rather than skipping it.)
func requireExactBoundsReproduce(t *testing.T, curves []geom.Curve, closed []geom.ClosedCurve, arr *geom.Arrangement) {
	t.Helper()
	const tol = 1e-9
	check := func(e geom.BoundaryEdge) {
		if !e.TExact {
			return
		}
		p0, ok0 := evalSource(t, curves, closed, e.SourceIndex, e.TStart)
		p1, ok1 := evalSource(t, curves, closed, e.SourceIndex, e.TEnd)
		require.Truef(t, ok0 && ok1,
			"no evaluator for source idx=%d — extend evalSource with this Curve type so the exactness invariant covers it",
			e.SourceIndex)
		a := e.Polyline[0]
		b := e.Polyline[len(e.Polyline)-1]
		// {eval(TStart), eval(TEnd)} must equal {polyline start, polyline end} as a set;
		// Reversed decides which is which, so accept either pairing.
		fwd := math.Hypot(p0[0]-a[0], p0[1]-a[1]) <= tol && math.Hypot(p1[0]-b[0], p1[1]-b[1]) <= tol
		rev := math.Hypot(p0[0]-b[0], p0[1]-b[1]) <= tol && math.Hypot(p1[0]-a[0], p1[1]-a[1]) <= tol
		require.Truef(t, fwd || rev,
			"TExact bound does not reproduce its polyline endpoints: src=%d reversed=%v t=[%v %v] eval=%v,%v poly=%v,%v",
			e.SourceIndex, e.Reversed, e.TStart, e.TEnd, p0, p1, a, b)
	}
	for _, r := range arr.Regions {
		for _, e := range r.Outer {
			check(e)
		}
		for _, h := range r.Holes {
			for _, e := range h {
				check(e)
			}
		}
	}
}

// TestBoundaryEdgeExactInvariant is the mechanical property test for the whole false-
// exact class: across a broad battery of arrangements — the very families that once
// produced thousands of false certifications — EVERY emitted TExact bound must
// reproduce its polyline endpoints. A weld (analytic cut or endpoint) canonicalizing
// onto a nearby sample vertex must NOT keep reading exact, because the sample/cut
// parameter it reports evaluates to the source point, not to the welded vertex.
func TestBoundaryEdgeExactInvariant(t *testing.T) {
	// (a) A chord splitting a disk: the primary generator. Before the fix this alone
	// emitted thousands of TExact bounds whose parameter was displaced from the vertex
	// by up to the merge tolerance (the crossing welded onto a circle sample vertex).
	for _, merge := range []float64{0.001, 0.01, 0.05, 0.1, 0.2} {
		for _, spt := range []int{3, 4, 5, 6, 8, 12} {
			for k := 0; k <= 200; k++ {
				h := -0.99 + 1.98*float64(k)/200.0
				curves := []geom.Curve{geom.NewLine(geom.NewPoint(-2, h), geom.NewPoint(2, h))}
				closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), 1)}
				arr := geom.Regions(curves, closed, geom.WithVertexMerge(merge), geom.WithSegmentsPerTurn(spt))
				requireExactBoundsReproduce(t, curves, closed, arr)
			}
		}
	}

	// (b) A circular SEGMENT (arc + chord) cut by a line crossing the arc near an arc
	// sample vertex — the reviewer's handled line/arc case, swept.
	for _, merge := range []float64{0.005, 0.01, 0.05} {
		for _, spt := range []int{4, 6, 8, 12, 16} {
			for k := 1; k < 300; k++ {
				const th = 1.2
				A := geom.NewPoint(math.Cos(th), -math.Sin(th))
				B := geom.NewPoint(math.Cos(th), math.Sin(th))
				h := -math.Sin(th) + 2*math.Sin(th)*float64(k)/300.0
				curves := []geom.Curve{
					geom.NewArc(geom.NewPoint(0, 0), A, B),
					geom.NewLine(B, A),
					geom.NewLine(geom.NewPoint(math.Cos(th)-0.001, h), geom.NewPoint(1.2, h)),
				}
				arr := geom.Regions(curves, nil, geom.WithVertexMerge(merge), geom.WithSegmentsPerTurn(spt))
				requireExactBoundsReproduce(t, curves, nil, arr)
			}
		}
	}

	// (c) Multi-source (5) chords partitioning a disk, welding at coarse sampling.
	for _, merge := range []float64{0.005, 0.02, 0.05} {
		for _, spt := range []int{3, 5, 8, 16} {
			for _, off := range []float64{0.0, 0.11, 0.23, 0.37} {
				curves := []geom.Curve{
					geom.NewLine(geom.NewPoint(-2, off), geom.NewPoint(2, off)),
					geom.NewLine(geom.NewPoint(off, -2), geom.NewPoint(off, 2)),
					geom.NewLine(geom.NewPoint(-2, -1.4+off), geom.NewPoint(2, 1.4+off)),
					geom.NewLine(geom.NewPoint(-2, 1.4-off), geom.NewPoint(2, -1.4-off)),
				}
				closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), 1)}
				arr := geom.Regions(curves, closed, geom.WithVertexMerge(merge), geom.WithSegmentsPerTurn(spt))
				requireExactBoundsReproduce(t, curves, closed, arr)
			}
		}
	}
}

// TestAnalyticArcCutWeldNotFalseExact is the reviewer's handled line/arc repro: an
// analytic cut whose exact contact lands within the merge tolerance of an arc sample
// vertex canonicalizes ONTO that sample vertex. The emitted arc fragment is then
// bounded by the sample vertex — displaced from the exact contact by ~0.002 — while its
// reported parameter is that sample vertex's (a fraction i/n). Reporting TExact=true
// there certifies a range that does NOT describe the emitted geometry. The fragment
// must read TExact=false unless the emitted vertex really is the exact contact.
func TestAnalyticArcCutWeldNotFalseExact(t *testing.T) {
	const th = 1.2
	A := geom.NewPoint(math.Cos(th), -math.Sin(th))
	B := geom.NewPoint(math.Cos(th), math.Sin(th))
	// h chosen so the cutter's arc crossing sits ~0.002 from the arc's t=1/3 sample
	// vertex (angle −0.4): within the 0.01 merge, so it welds, but far from identity.
	h := -math.Sin(th) + 2*math.Sin(th)*87.0/300.0
	curves := []geom.Curve{
		geom.NewArc(geom.NewPoint(0, 0), A, B), // src 0
		geom.NewLine(B, A),                     // src 1: the chord closing the segment
		geom.NewLine(geom.NewPoint(math.Cos(th)-0.001, h), geom.NewPoint(1.2, h)), // src 2: the cutter
	}
	arr := geom.Regions(curves, nil, geom.WithVertexMerge(0.01), geom.WithSegmentsPerTurn(6))

	// The universal invariant must hold — this alone fails on the buggy code, because
	// the welded arc fragment is emitted TExact with a parameter that does not evaluate
	// to its polyline endpoint.
	requireExactBoundsReproduce(t, curves, nil, arr)

	// And concretely: the arc fragment bounded by the weld is displaced from its
	// reported parameter, so it must not read exact.
	welded := 0
	for _, r := range arr.Regions {
		for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
			if e.SourceIndex != 0 { // the arc
				continue
			}
			p0, _ := evalSource(t, curves, nil, 0, e.TStart)
			p1, _ := evalSource(t, curves, nil, 0, e.TEnd)
			a := e.Polyline[0]
			b := e.Polyline[len(e.Polyline)-1]
			displ := math.Min(
				math.Max(math.Hypot(p0[0]-a[0], p0[1]-a[1]), math.Hypot(p1[0]-b[0], p1[1]-b[1])),
				math.Max(math.Hypot(p0[0]-b[0], p0[1]-b[1]), math.Hypot(p1[0]-a[0], p1[1]-a[1])),
			)
			if displ <= 1e-6 {
				continue // this bound genuinely lands on the vertex — exactness allowed
			}
			welded++
			require.Falsef(t, e.TExact,
				"an arc fragment whose parameter is displaced %.4g from its polyline endpoint must not read exact: t=[%v %v]",
				displ, e.TStart, e.TEnd)
		}
	}
	require.NotZero(t, welded, "the cutter must weld an arc fragment displaced from its parameter")
}

// TestAnalyticWeldChainNoFalseExact drives a five-source arrangement whose distance
// welds chain several sample vertices and crossings through shared graph vertices, with
// a coarse merge producing displacements up to ~0.02. No emitted fragment may claim
// TExact while its parameter fails to reproduce its polyline endpoint, and — since the
// chain genuinely displaces bounds — at least one emitted bound must read inexact.
func TestAnalyticWeldChainNoFalseExact(t *testing.T) {
	sawInexact := false
	for _, merge := range []float64{0.02, 0.05} {
		for _, spt := range []int{4, 6, 10} {
			curves := []geom.Curve{
				geom.NewLine(geom.NewPoint(-2, 0.004), geom.NewPoint(2, 0.004)),
				geom.NewLine(geom.NewPoint(0.004, -2), geom.NewPoint(0.004, 2)),
				geom.NewLine(geom.NewPoint(-2, -0.6), geom.NewPoint(2, 0.62)),
				geom.NewLine(geom.NewPoint(-1.4, 2), geom.NewPoint(1.42, -2)),
			}
			closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), 1)}
			arr := geom.Regions(curves, closed, geom.WithVertexMerge(merge), geom.WithSegmentsPerTurn(spt))
			requireExactBoundsReproduce(t, curves, closed, arr)
			for _, r := range arr.Regions {
				for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
					if !e.TExact {
						sawInexact = true
					}
				}
			}
		}
	}
	require.True(t, sawInexact, "a coarse weld chain must produce at least one inexact bound")
}

// TestEllipticalArcWholeEdgePinnedNotFalseExact is the round-8 pinned-endpoint repro.
// An elliptical arc PINS its segment ends to its sketch Start/End points, which lie on
// the parametric ellipse only within solver tolerance. The whole-arc edge therefore
// emits a polyline whose ends are those pinned points, while evaluating the ellipse at
// the reported parameter (t=0/t=1) lands ~0.005 away. Certifying that range TExact=true
// is a false certification on a WHOLE edge: the welded vertex IS the pinned point, so
// the old identity audit (vertexCertifies) passed, but the parameter does not reproduce
// the endpoint. The fix decides endpoint exactness by whether eval(param) reproduces the
// emitted coordinate, so this edge reads TExact=false while staying topologically Whole.
func TestEllipticalArcWholeEdgePinnedNotFalseExact(t *testing.T) {
	// The reviewer's exact fixture: rx=4, ry=2, Start=(4,0.1) sits off the ellipse
	// (the ellipse point at that eccentric angle is ~(3.995, 0.0999)).
	center := geom.NewPoint(0, 0)
	start := geom.NewPoint(4, 0.1)
	end := geom.NewPoint(-4, 0)
	ea := geom.NewEllipticalArc(center, start, end, 4, 2, 0)
	line := geom.NewLine(end, start) // close the region
	curves := []geom.Curve{ea, line}

	arr := geom.Regions(curves, nil)
	require.Len(t, arr.Regions, 1)

	// The universal invariant must hold — before the fix it fails here, because the
	// elliptical-arc whole edge is emitted TExact with a parameter 0.005 off its end.
	requireExactBoundsReproduce(t, curves, nil, arr)

	var sawArc bool
	for _, r := range arr.Regions {
		for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
			if e.SourceIndex != 0 { // the elliptical arc
				continue
			}
			sawArc = true
			require.True(t, e.Whole, "the arc is uncut — it is topologically the whole curve")
			// eval(reported param) misses the pinned polyline endpoint by ~0.005, so the
			// bound is NOT exact and must not be certified.
			p0, ok := evalSource(t, curves, nil, 0, e.TStart)
			require.True(t, ok)
			a := e.Polyline[0]
			miss := math.Hypot(p0[0]-a[0], p0[1]-a[1])
			require.Greater(t, miss, 1e-3,
				"the pinned Start (4,0.1) sits off the parametric ellipse — eval(0) must miss it")
			require.False(t, e.TExact,
				"a pinned endpoint whose parameter misses it by %.4g must not read exact", miss)
		}
	}
	require.True(t, sawArc, "the elliptical arc must bound the region")
}

// TestBoundaryEdgeExactInvariantFreeform extends the universal exactness invariant over
// the free-form curve families the analytic battery never exercised. It also nails the
// MEANING of TExact for a whole edge of a free-form curve: an uncut ellipse/conic/
// spline/closed-spline/fit-spline/NURBS reports TExact=true because its polyline ends
// ARE its own domain-end evaluations (which the invariant verifies eval reproduces),
// whereas the pinned elliptical arc reports TExact=false. Every emitted TExact bound —
// whole or fragment — must reproduce its polyline endpoints for every one of them.
func TestBoundaryEdgeExactInvariantFreeform(t *testing.T) {
	closedSpline, err := geom.NewClosedSpline(
		geom.NewPoint(-3, -3), geom.NewPoint(3, -3), geom.NewPoint(3, 3), geom.NewPoint(-3, 3),
	)
	require.NoError(t, err)
	spline, err := geom.NewSpline(
		geom.NewPoint(-4, 0), geom.NewPoint(-2, 4), geom.NewPoint(2, 4), geom.NewPoint(4, 0),
	)
	require.NoError(t, err)
	fit, err := geom.NewFitSpline(
		geom.NewPoint(-4, 0), geom.NewPoint(0, 3), geom.NewPoint(4, 0),
	)
	require.NoError(t, err)
	nurbs := geom.NewNURBS(2,
		[]*geom.Point{geom.NewPoint(-3, 0), geom.NewPoint(0, 4), geom.NewPoint(3, 0)},
		[]float64{0, 0, 0, 1, 1, 1}, []float64{1, 2, 1})

	// wholeExact records which SourceIndex values are expected to report a whole edge
	// with TExact=true (an uncut free-form curve whose ends are its own evaluated domain
	// ends), so the test asserts the MEANING, not just "the invariant did not fire".
	cases := []struct {
		name        string
		curves      []geom.Curve
		closed      []geom.ClosedCurve
		wantTExact0 bool // whether source 0 (the free-form curve) must read a whole exact edge
	}{
		{
			name: "whole ellipse is exact",
			closed: []geom.ClosedCurve{
				geom.NewEllipse(geom.NewPoint(0, 0), 5, 3, 0.4),
			},
			wantTExact0: true,
		},
		{
			name: "whole conic is exact",
			curves: []geom.Curve{
				geom.NewConic(geom.NewPoint(-3, 0), geom.NewPoint(0, 4), geom.NewPoint(3, 0), 0.6),
				geom.NewLine(geom.NewPoint(3, 0), geom.NewPoint(-3, 0)),
			},
			wantTExact0: true,
		},
		{
			name:        "whole closed spline is exact",
			closed:      []geom.ClosedCurve{closedSpline},
			wantTExact0: true,
		},
		{
			name: "whole open spline is exact",
			curves: []geom.Curve{
				spline,
				geom.NewLine(geom.NewPoint(4, 0), geom.NewPoint(-4, 0)),
			},
			wantTExact0: true,
		},
		{
			name: "whole fit spline is exact",
			curves: []geom.Curve{
				fit,
				geom.NewLine(geom.NewPoint(4, 0), geom.NewPoint(-4, 0)),
			},
			wantTExact0: true,
		},
		{
			name: "whole nurbs is exact",
			curves: []geom.Curve{
				nurbs,
				geom.NewLine(geom.NewPoint(3, 0), geom.NewPoint(-3, 0)),
			},
			wantTExact0: true,
		},
		{
			name: "whole elliptical arc is inexact (pinned ends)",
			curves: []geom.Curve{
				geom.NewEllipticalArc(geom.NewPoint(0, 0), geom.NewPoint(4, 0.1), geom.NewPoint(-4, 0), 4, 2, 0),
				geom.NewLine(geom.NewPoint(-4, 0), geom.NewPoint(4, 0.1)),
			},
			wantTExact0: false,
		},
		{
			name: "line crossing an ellipse: fragments inexact",
			curves: []geom.Curve{
				geom.NewLine(geom.NewPoint(-6, 1), geom.NewPoint(6, 1)),
			},
			closed: []geom.ClosedCurve{
				geom.NewEllipse(geom.NewPoint(0, 0), 5, 3, 0),
			},
			wantTExact0: false, // source 0 here is the LINE; asserted separately below
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arr := geom.Regions(tc.curves, tc.closed)
			require.NotEmpty(t, arr.Regions)
			requireExactBoundsReproduce(t, tc.curves, tc.closed, arr)

			if tc.name == "line crossing an ellipse: fragments inexact" {
				// Every ellipse fragment (source 1, the closed curve) is bounded by a
				// SAMPLED line/ellipse crossing, so none may read exact.
				var frags int
				for _, r := range arr.Regions {
					for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
						if e.SourceIndex == 1 && !e.Whole {
							frags++
							require.False(t, e.TExact, "a sampled line/ellipse crossing is not exact")
						}
					}
				}
				require.NotZero(t, frags, "the line must cut the ellipse")
				return
			}

			// For the whole-curve fixtures, source 0's whole edge must carry the
			// expected TExact — true for evaluated ends, false for the pinned arc.
			var sawWhole bool
			for _, r := range arr.Regions {
				for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
					if e.SourceIndex != 0 || !e.Whole {
						continue
					}
					sawWhole = true
					require.Equalf(t, tc.wantTExact0, e.TExact,
						"whole edge of source 0 TExact: got %v want %v (t=[%v %v])",
						e.TExact, tc.wantTExact0, e.TStart, e.TEnd)
				}
			}
			require.True(t, sawWhole, "source 0 must contribute a whole edge")
		})
	}
}
