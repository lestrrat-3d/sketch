package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// evalAnalyticSource evaluates a line/circle/arc source at the arrangement's
// normalized parameter t (the convention documented on BoundaryEdge.TStart). It
// returns ok=false for any other curve type — a BoundaryEdge with TExact=true on a
// non-line/circle/arc source is itself a defect (only those pairs run the closed-form
// kernel), which the invariant below asserts against.
func evalAnalyticSource(t *testing.T, curves []geom.Curve, closed []geom.ClosedCurve, idx int, tp float64) ([2]float64, bool) {
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
	}
	return [2]float64{}, false
}

// requireExactBoundsReproduce is the UNIVERSAL exactness invariant. TExact means the
// reported [TStart,TEnd] are the TRUE source parameters — so evaluating the source
// curve at TStart and TEnd MUST reproduce the emitted polyline's endpoints to machine
// precision. That is the definition of "exact", and it is checkable mechanically for
// any fixture, present or future. This is the property the whole class of false-
// certification defects violated: a fragment bounded by a distance weld reports a
// sample/cut parameter that does NOT evaluate to the vertex the weld moved it to.
//
// It also asserts that TExact never appears on a non-line/circle/arc source (only those
// pairs run the closed-form kernel, so every other source is sampled and inexact).
func requireExactBoundsReproduce(t *testing.T, curves []geom.Curve, closed []geom.ClosedCurve, arr *geom.Arrangement) {
	t.Helper()
	const tol = 1e-9
	check := func(e geom.BoundaryEdge) {
		if !e.TExact {
			return
		}
		p0, ok0 := evalAnalyticSource(t, curves, closed, e.SourceIndex, e.TStart)
		p1, ok1 := evalAnalyticSource(t, curves, closed, e.SourceIndex, e.TEnd)
		require.Truef(t, ok0 && ok1,
			"TExact=true on a non-line/circle/arc source idx=%d — only those pairs run the closed-form kernel, so nothing else may report exact",
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
			p0, _ := evalAnalyticSource(t, curves, nil, 0, e.TStart)
			p1, _ := evalAnalyticSource(t, curves, nil, 0, e.TEnd)
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
