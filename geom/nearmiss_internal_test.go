package geom

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestChordDeviationBoundsHold is the soundness regression for the guard's whole
// premise: every per-family bound must actually BOUND the curve's departure from
// its chord, on every segment the sampler produces.
//
// It is checked against a dense sub-sampling of the true curve rather than against
// a formula, so a wrong derivation is caught by the geometry itself. The looseness
// assertion is the other half: a bound that held by being enormous would flag every
// scene, so each family is required to stay within a small multiple of the true
// maximum — the previous attempt in this repository failed the opposite way, using
// each segment's MIDPOINT deviation as if it bounded the span (measured at a 22195x
// UNDERestimate), which is the direction that publishes a fused map as sound.
func TestChordDeviationBoundsHold(t *testing.T) {
	arch := []*Point{NewPoint(-40, 30), NewPoint(-20, -20), NewPoint(0, -34), NewPoint(20, -20), NewPoint(40, 30)}
	tight := []*Point{NewPoint(-40, 30), NewPoint(-1, -20), NewPoint(0, -34), NewPoint(1, -20), NewPoint(40, 30)}
	loop := []*Point{NewPoint(-30, -20), NewPoint(0, 34), NewPoint(30, -20)}

	spline := func(ps []*Point) *Spline {
		sp, err := NewSpline(ps...)
		require.NoError(t, err)
		return sp
	}
	fitSpline := func(ps []*Point) *FitSpline {
		sp, err := NewFitSpline(ps...)
		require.NoError(t, err)
		return sp
	}
	closedSpline := func(ps []*Point) *ClosedSpline {
		sp, err := NewClosedSpline(ps...)
		require.NoError(t, err)
		return sp
	}

	// clustered puts every interior knot inside a narrow window, the adversarial end
	// of the range: the curve turns almost entirely inside one knot span.
	clustered := func(n, deg int, w float64) []float64 {
		k := make([]float64, 0, n+deg+1)
		for i := 0; i <= deg; i++ {
			k = append(k, 0)
		}
		inner := n - deg - 1
		for i := 1; i <= inner; i++ {
			k = append(k, 0.5+w*(float64(i)/float64(inner+1)-0.5))
		}
		for i := 0; i <= deg; i++ {
			k = append(k, 1)
		}
		return k
	}

	for _, tc := range []struct {
		name    string
		open    []Curve
		closed  []ClosedCurve
		maxOver float64 // how many times the true maximum the bound may reach
	}{
		{name: "line", open: []Curve{NewLine(NewPoint(-10, -4), NewPoint(12, 7))}, maxOver: 1},
		{name: "circle", closed: []ClosedCurve{NewCircle(NewPoint(1, 2), 17)}, maxOver: 1.01},
		{name: "arc", open: []Curve{NewArc(NewPoint(0, 0), NewPoint(20, 0), NewPoint(-3, 19))}, maxOver: 1.01},
		{name: "ellipse", closed: []ClosedCurve{NewEllipse(NewPoint(2, -1), 30, 7, 0.4)}, maxOver: 25},
		{name: "elliptical arc", open: []Curve{
			NewEllipticalArc(NewPoint(0, 0), NewPoint(30, 0), NewPoint(-8, 6), 30, 7, 0.2)}, maxOver: 25},
		{name: "conic", open: []Curve{NewConic(NewPoint(-40, 30), NewPoint(0, -40), NewPoint(40, 30), 0.8)}, maxOver: 20},
		{name: "spline", open: []Curve{spline(arch)}, maxOver: 20},
		{name: "spline with a tight bend", open: []Curve{spline(tight)}, maxOver: 20},
		{name: "closed spline", closed: []ClosedCurve{closedSpline(loop)}, maxOver: 20},
		{name: "fit spline", open: []Curve{fitSpline(arch)}, maxOver: 20},
		{name: "nurbs uniform", open: []Curve{
			NewNURBS(3, arch, ClampedUniformKnots(len(arch), 3), []float64{1, 1, 1, 1, 1})}, maxOver: 20},
		{name: "nurbs rational", open: []Curve{
			NewNURBS(3, arch, ClampedUniformKnots(len(arch), 3), []float64{1, 1, 40, 1, 1})}, maxOver: 20},
		{name: "nurbs degree 2", open: []Curve{
			NewNURBS(2, arch, ClampedUniformKnots(len(arch), 2), []float64{1, 3, 1, 0.5, 1})}, maxOver: 20},
		{name: "nurbs knots clustered", open: []Curve{
			NewNURBS(3, arch, clustered(len(arch), 3, 1e-3), []float64{1, 1, 1, 1, 1})}, maxOver: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newArranger(tc.open, tc.closed, arrangeConfig{})
			a.densify()
			require.NotEmpty(t, a.segs)
			require.Len(t, a.segDev, len(a.segs))
			// round-off floor: evaluating a curve and its chord separately differs in
			// the last bits, which is not a deviation the bound has to carry.
			eps := a.scale * 1e-12
			maxBound, maxTruth := 0.0, 0.0
			for i := range a.segs {
				seg := &a.segs[i]
				s := &a.sources[seg.src]
				bound := a.segDev[i]
				require.Falsef(t, math.IsNaN(bound), "segment %d: bound is NaN", i)
				truth := 0.0
				const samples = 400
				for k := 0; k <= samples; k++ {
					p := seg.pa + (seg.pb-seg.pa)*float64(k)/samples
					q := s.at(p)
					if d := distPointSeg(q[0], q[1], seg.ax, seg.ay, seg.bx, seg.by); d > truth {
						truth = d
					}
				}
				require.GreaterOrEqualf(t, bound+eps, truth,
					"segment %d: bound %g does not cover the true deviation %g", i, bound, truth)
				maxBound = math.Max(maxBound, bound)
				maxTruth = math.Max(maxTruth, truth)
			}
			// Looseness is judged on the LARGEST bound against the largest true bow,
			// since that pair is what sets the band a near miss is measured against.
			// A near-straight span's ratio says nothing: both numbers are round-off.
			require.LessOrEqual(t, maxBound, tc.maxOver*maxTruth+eps, "the bound is too loose to be useful")
		})
	}
}

// guardPairs runs the near-miss guard on a fresh arranger of the scene and returns the
// source pairs it flags, with the point it reports for each.
func guardPairs(curves []Curve, closed []ClosedCurve, reference bool) map[[2]int][2]float64 {
	a := newArranger(curves, closed, arrangeConfig{segsPerTurn: 48})
	a.densify()
	a.intersect()
	a.degen = nil
	a.degenSet = false
	if reference {
		allPairsNearMissGuard(a)
	} else {
		a.nearMissGuard()
	}
	out := map[[2]int][2]float64{}
	for _, d := range a.degen {
		if len(d.srcs) == 2 {
			out[pairKey(d.srcs[0], d.srcs[1])] = [2]float64{d.x, d.y}
		}
	}
	return out
}

// allPairsNearMissGuard is nearMissGuard without the source-level reject: every
// qualifying pair of sources goes to pairNearMiss.
func allPairsNearMissGuard(a *arranger) {
	boxes := make([]nearMissBox, len(a.segs))
	for k := range a.segs {
		s := &a.segs[k]
		boxes[k] = nearMissBox{
			minX: math.Min(s.ax, s.bx), maxX: math.Max(s.ax, s.bx),
			minY: math.Min(s.ay, s.by), maxY: math.Max(s.ay, s.by),
		}
	}
	for i := range a.sources {
		ki := a.sources[i].kind
		if ki == srcDegenerate {
			continue
		}
		for j := i + 1; j < len(a.sources); j++ {
			kj := a.sources[j].kind
			if kj == srcDegenerate {
				continue
			}
			_, certifiedFitPair := a.specialHandled[pairKey(i, j)]
			if (analyticKind(ki) && analyticKind(kj)) || certifiedFitPair {
				continue
			}
			if x, y, miss := a.pairNearMiss(i, j, boxes); miss {
				a.flagDegenerate(x, y, i, j)
			}
		}
	}
}

// TestNearMissGuardSourceRejectKeepsEveryVerdict checks the guard against the same
// guard run over every pair of sources, on random scenes with near misses built in: a
// circle whose radius falls a hair short of, or past, a point of a spline, and a
// second spline laid a hair beside the first.
func TestNearMissGuardSourceRejectKeepsEveryVerdict(t *testing.T) {
	rng := rand.New(rand.NewSource(20261023))
	pt := func() *Point { return NewPoint(rng.Float64()*20-10, rng.Float64()*20-10) }
	gaps := []float64{1e-3, 1e-4, 1e-5, 1e-7, 1e-9, 0}
	flagged := 0
	for trial := 0; trial < 150; trial++ {
		var curves []Curve
		var closed []ClosedCurve
		for range 1 + rng.Intn(3) {
			fitPts := make([]*Point, 3+rng.Intn(3))
			for i := range fitPts {
				fitPts[i] = pt()
			}
			fit, err := NewFitSpline(fitPts...)
			require.NoError(t, err)
			curves = append(curves, fit)
			gap := gaps[rng.Intn(len(gaps))] * float64(1-2*rng.Intn(2))
			switch rng.Intn(4) {
			case 3:
				// A circle standing on the spline's highest point, its bottom a hair
				// above it (or below it): the two bounding boxes are apart by about the
				// gap, which is inside the band of the pair.
				top := [2]float64{0, math.Inf(-1)}
				for k := 0; k <= 800; k++ {
					x, y := fit.Eval(float64(k) / 800)
					if y > top[1] {
						top = [2]float64{x, y}
					}
				}
				r := 0.5 + rng.Float64()*3
				closed = append(closed, NewCircle(NewPoint(top[0], top[1]+r+math.Abs(gap)*float64(1-2*rng.Intn(2))), r))
			case 0:
				on := fitPts[rng.Intn(len(fitPts))]
				c := pt()
				closed = append(closed, NewCircle(c, math.Hypot(on.X-c.X, on.Y-c.Y)+gap))
			case 1:
				beside := make([]*Point, len(fitPts))
				for i, p := range fitPts {
					beside[i] = NewPoint(p.X, p.Y+gap)
				}
				other, err := NewFitSpline(beside...)
				require.NoError(t, err)
				curves = append(curves, other)
			default:
				curves = append(curves, NewLine(pt(), pt()), NewArc(pt(), pt(), pt()))
			}
		}
		// An ellipse far from the rest keeps the pair ledger out of the scene, so the
		// test measures the guard and not the exact avoidance proof.
		closed = append(closed, NewEllipse(NewPoint(500, 500), 1, 2, 0.3))
		got := guardPairs(curves, closed, false)
		want := guardPairs(curves, closed, true)
		require.Equalf(t, len(want), len(got), "trial %d: flagged pairs", trial)
		for k, w := range want {
			g, ok := got[k]
			require.Truef(t, ok, "trial %d: pair %v flagged by the all-pairs guard only", trial, k)
			require.Equalf(t, w, g, "trial %d: pair %v witness", trial, k)
		}
		flagged += len(want)
	}
	require.Greater(t, flagged, 3, "the generator must produce flagged pairs")
}
