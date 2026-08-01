package geom_test

import (
	"fmt"
	"math"
	"sort"
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
//
// The comparison is RELATIVE to the coordinates under test (reproduceTol), never a fixed
// absolute distance. "Reproduces to machine precision" is a statement about round-off,
// and round-off is proportional to the magnitude of the numbers involved, so a fixed
// number is the wrong shape at both ends: on small geometry it waves through a miss that
// is orders of magnitude above round-off — at 1e-9 absolute it passed a bound whose
// reported parameter missed its own polyline endpoint by 1e-9 on a scene 10 units across,
// which is the whole defect TestExactBoundIdentityBandIsSourceLocal pins — and on large
// geometry it would fail bounds that are exact. Every TExact bound the suite emits
// reproduces to within 1.4e-15 relative, so the 1e-12 allowance below sits three orders
// above the observed round-off and three below the engine's merge tolerance (1e-7).
func requireExactBoundsReproduce(t *testing.T, curves []geom.Curve, closed []geom.ClosedCurve, arr *geom.Arrangement) {
	t.Helper()
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
		tol := reproduceTol(p0, p1, a, b)
		// {eval(TStart), eval(TEnd)} must equal {polyline start, polyline end} as a set;
		// Reversed decides which is which, so accept either pairing.
		fwd := math.Hypot(p0[0]-a[0], p0[1]-a[1]) <= tol && math.Hypot(p1[0]-b[0], p1[1]-b[1]) <= tol
		rev := math.Hypot(p0[0]-b[0], p0[1]-b[1]) <= tol && math.Hypot(p1[0]-a[0], p1[1]-a[1]) <= tol
		require.Truef(t, fwd || rev,
			"TExact bound does not reproduce its polyline endpoints (tol %g): src=%d reversed=%v t=[%v %v] eval=%v,%v poly=%v,%v",
			tol, e.SourceIndex, e.Reversed, e.TStart, e.TEnd, p0, p1, a, b)
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

// reproduceTol is the round-off allowance for "eval(the reported parameter) IS the
// emitted polyline endpoint", stated relative to the largest coordinate being compared
// — the only length floating-point evaluation error is proportional to. It is
// deliberately independent of the arrangement's own bands: the oracle must not restate
// what the engine did, only what a consumer is promised.
func reproduceTol(pts ...[2]float64) float64 {
	m := 1.0
	for _, p := range pts {
		m = math.Max(m, math.Max(math.Abs(p[0]), math.Abs(p[1])))
	}
	return 1e-12 * m
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
// the free-form curve families the analytic battery never exercised, and pins what the
// scene gate does to them: a scene holding ANY free-form source — ellipse, elliptical
// arc, conic, spline, closed spline, fit spline or NURBS — publishes NO exact bound
// anywhere in it, whole edges of the free-form curve itself included. A chord's
// deviation from its curve is not bounded for those families, so a crossing can hide
// between two samples and leave a whole edge that is really a fragment (see
// TestFreeFormSourceWithholdsExactBoundsSceneWide). The invariant itself is unchanged
// and still runs: whatever DOES report TExact must reproduce its polyline endpoints.
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

	// Each fixture holds at least one free-form source, so the expectation is the same
	// for all of them: nothing in the scene reports TExact. The fixtures still differ in
	// what they exercise — an uncut whole edge, a pinned elliptical arc, a sampled
	// crossing — which is what the per-case assertions below check.
	cases := []struct {
		name   string
		curves []geom.Curve
		closed []geom.ClosedCurve
	}{
		{
			name: "whole ellipse: no exact bound in a free-form scene",
			closed: []geom.ClosedCurve{
				geom.NewEllipse(geom.NewPoint(0, 0), 5, 3, 0.4),
			},
		},
		{
			name: "whole conic: no exact bound in a free-form scene",
			curves: []geom.Curve{
				geom.NewConic(geom.NewPoint(-3, 0), geom.NewPoint(0, 4), geom.NewPoint(3, 0), 0.6),
				geom.NewLine(geom.NewPoint(3, 0), geom.NewPoint(-3, 0)),
			},
		},
		{
			name:   "whole closed spline: no exact bound in a free-form scene",
			closed: []geom.ClosedCurve{closedSpline},
		},
		{
			name: "whole open spline: no exact bound in a free-form scene",
			curves: []geom.Curve{
				spline,
				geom.NewLine(geom.NewPoint(4, 0), geom.NewPoint(-4, 0)),
			},
		},
		{
			name: "whole fit spline: no exact bound in a free-form scene",
			curves: []geom.Curve{
				fit,
				geom.NewLine(geom.NewPoint(4, 0), geom.NewPoint(-4, 0)),
			},
		},
		{
			name: "whole nurbs: no exact bound in a free-form scene",
			curves: []geom.Curve{
				nurbs,
				geom.NewLine(geom.NewPoint(3, 0), geom.NewPoint(-3, 0)),
			},
		},
		{
			name: "whole elliptical arc: pinned ends, no exact bound",
			curves: []geom.Curve{
				geom.NewEllipticalArc(geom.NewPoint(0, 0), geom.NewPoint(4, 0.1), geom.NewPoint(-4, 0), 4, 2, 0),
				geom.NewLine(geom.NewPoint(-4, 0), geom.NewPoint(4, 0.1)),
			},
		},
		{
			name: "line crossing an ellipse: fragments inexact",
			curves: []geom.Curve{
				geom.NewLine(geom.NewPoint(-6, 1), geom.NewPoint(6, 1)),
			},
			closed: []geom.ClosedCurve{
				geom.NewEllipse(geom.NewPoint(0, 0), 5, 3, 0),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arr := geom.Regions(tc.curves, tc.closed)
			require.NotEmpty(t, arr.Regions)
			requireExactBoundsReproduce(t, tc.curves, tc.closed, arr)

			// The scene gate, asserted on every emitted bound of every source: a
			// free-form source is present in each fixture, so nothing here is exact.
			var whole0, frags1 int
			for _, r := range arr.Regions {
				for _, e := range append(append([]geom.BoundaryEdge{}, r.Outer...), flattenHoles(r)...) {
					require.Falsef(t, e.TExact,
						"source %d publishes an exact bound in a scene holding a free-form curve (t=[%v %v])",
						e.SourceIndex, e.TStart, e.TEnd)
					switch {
					case e.SourceIndex == 0 && e.Whole:
						whole0++
					case e.SourceIndex == 1 && !e.Whole:
						frags1++
					}
				}
			}
			if tc.name == "line crossing an ellipse: fragments inexact" {
				// The sampled line/ellipse crossing cuts the ellipse (source 1) into
				// fragments; the gate above already covered their exactness.
				require.NotZero(t, frags1, "the line must cut the ellipse")
				return
			}
			require.NotZero(t, whole0, "source 0 must contribute a whole edge")
		})
	}
}

// TestExactBoundIdentityBandIsSourceLocal pins that the identity band deciding whether
// a bound's reported parameter may be published as exact belongs to the SOURCE under
// test, not to the drawing. The band was stated only against the arrangement's scale —
// the whole scene's bounding-box extent — so an object with nothing to do with the
// bound widened it.
//
// The fixture is the ordinary consumer path with no options at all: a circle of radius
// 5 and a chord line crossing it a fixed distance g past one of the circle's own sample
// vertices. g is below the merge tolerance, so the exact crossing welds ONTO that sample
// vertex: the emitted polyline endpoint is the vertex while the reported parameter is
// the crossing, g away. That bound is not exact, and the circle alone reports it so. Add
// ONE unrelated line at x=1000 and the scene band grows past g, and the same bound reads
// exact — publishing a parameter that misses its own polyline endpoint by the whole g.
//
// So the verdict must be the same with and without the distant line, at every extent,
// density and gap; and it must be the refusal. Areas, region counts, parameter ranges
// and Whole must be identical either way — only the exactness claim was ever at stake.
func TestExactBoundIdentityBandIsSourceLocal(t *testing.T) {
	const r = 5.0
	// scene builds the circle plus a chord whose first crossing sits arc-distance g past
	// the circle's sample vertex at t=0.125, optionally with an unrelated far line.
	scene := func(g, far float64) ([]geom.Curve, []geom.ClosedCurve) {
		aP := 2*math.Pi*0.125 + g/r
		aR := 2*math.Pi*0.625 + 0.013
		px, py := r*math.Cos(aP), r*math.Sin(aP)
		qx, qy := r*math.Cos(aR), r*math.Sin(aR)
		ux, uy := px-qx, py-qy
		l := math.Hypot(ux, uy)
		ux, uy = ux/l, uy/l
		curves := []geom.Curve{geom.NewLine(
			geom.NewPoint(px+0.7*ux, py+0.7*uy), geom.NewPoint(qx-0.7*ux, qy-0.7*uy))}
		if far > 0 {
			curves = append(curves, geom.NewLine(geom.NewPoint(far, -1), geom.NewPoint(far, 1)))
		}
		return curves, []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, 0), r)}
	}
	// signature is everything a consumer reads off the arrangement, exactness last, so a
	// difference reports which half moved. Sources are named rather than indexed: the far
	// line takes index 1, which shifts the closed circle's own index, and that renumbering
	// is bookkeeping about the input list, not a change in what the circle reports.
	signature := func(arr *geom.Arrangement, nCurves int) []string {
		var out []string
		for _, reg := range arr.Regions {
			for _, e := range append(append([]geom.BoundaryEdge{}, reg.Outer...), flattenHoles(reg)...) {
				name := "chord"
				if e.SourceIndex == nCurves {
					name = "circle"
				}
				out = append(out, fmt.Sprintf("area=%.17g src=%s t=[%.17g %.17g] rev=%v whole=%v exact=%v",
					reg.Area, name, e.TStart, e.TEnd, e.Reversed, e.Whole, e.TExact))
			}
		}
		sort.Strings(out)
		return out
	}

	for _, g := range []float64{1e-10, 1e-9, 1e-8, 1e-7} {
		for _, spt := range []int{0, 64, 128, 256} { // 0 = the adaptive default (256)
			var opts []geom.Option
			if spt > 0 {
				opts = append(opts, geom.WithSegmentsPerTurn(spt))
			}
			curves, closed := scene(g, 0)
			alone := geom.Regions(curves, closed, opts...)
			require.Falsef(t, alone.Degenerate, "g=%g spt=%d: a chord across a disk is clean", g, spt)
			require.Lenf(t, alone.Regions, 2, "g=%g spt=%d: the chord splits the disk in two", g, spt)
			requireExactBoundsReproduce(t, curves, closed, alone)
			base := signature(alone, len(curves))

			for _, far := range []float64{1e3, 1e4, 1e5} {
				fc, fcl := scene(g, far)
				got := geom.Regions(fc, fcl, opts...)
				requireExactBoundsReproduce(t, fc, fcl, got)
				require.Equalf(t, base, signature(got, len(fc)),
					"g=%g spt=%d far=%g: a line %g units away changed what the circle's own bounds report",
					g, spt, far, far)
			}
		}
	}
}

// TestExactBoundIdentityBandAdmitsRoundOff is the converse guard: tightening the band to
// the source's own size must not start refusing genuine identity. A shared corner is
// expressed by two curves holding the same point, and each reaches the arrangement
// through its OWN evaluation of it — a line reproduces it exactly, an arc rebuilds it
// through trig a few ulps out — so the band has to stay above that round-off. Over a
// battery of arc/line wires, at every density, every bound the closed-form kernel
// produced is still published exact.
func TestExactBoundIdentityBandAdmitsRoundOff(t *testing.T) {
	for _, spt := range []int{0, 8, 16, 64, 256} {
		var opts []geom.Option
		if spt > 0 {
			opts = append(opts, geom.WithSegmentsPerTurn(spt))
		}
		for _, th := range []float64{0.3, 0.9, 1.5, 2.4} {
			// A circular segment: an arc closed by the chord between its own endpoints.
			c := geom.NewPoint(0, 0)
			a := geom.NewPoint(4*math.Cos(-th), 4*math.Sin(-th))
			b := geom.NewPoint(4*math.Cos(th), 4*math.Sin(th))
			curves := []geom.Curve{geom.NewArc(c, a, b), geom.NewLine(b, a)}
			arr := geom.Regions(curves, nil, opts...)
			require.Lenf(t, arr.Regions, 1, "spt=%d th=%g: the segment closes", spt, th)
			requireExactBoundsReproduce(t, curves, nil, arr)
			for _, reg := range arr.Regions {
				for _, e := range reg.Outer {
					require.Truef(t, e.TExact,
						"spt=%d th=%g: source %d bounds a shared-endpoint join, which is identity, not tolerance",
						spt, th, e.SourceIndex)
				}
			}
		}
	}
}
