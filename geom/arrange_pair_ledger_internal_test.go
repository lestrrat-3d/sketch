package geom

// In-package because the verdicts live on the unexported arranger. The properties
// are soundness ones: a pair certified disjoint or endpoint-only must really be so,
// judged by an oracle that shares no code with the certificate (dense polylines of
// source.at and a segment-crossing test).

import (
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// proveOnly runs the arrangement up to the pair proofs. The sampled-map audit that
// intersect adds afterwards can demote a pair, so a test of the proof itself stops
// before it.
func proveOnly(curves []Curve, closed []ClosedCurve) *arranger {
	a := newArranger(curves, closed, arrangeConfig{})
	a.densify()
	a.analyticPrepass()
	return a
}

// ledgerPair arranges the curves up to the intersection pass and returns the entry
// of the pair of source 0 (a fit spline) and source 1.
func ledgerPair(t *testing.T, curves []Curve, closed []ClosedCurve) (pairEntry, *arranger) {
	t.Helper()
	a := proveOnly(curves, closed)
	require.NotNil(t, a.ledger, "the scene holds a fit spline and analytic sources only")
	return a.ledger[pairKey(0, 1)], a
}

func mustFit(t *testing.T, pts ...*Point) *FitSpline {
	t.Helper()
	f, err := NewFitSpline(pts...)
	require.NoError(t, err)
	return f
}

func TestPairLedgerConstructedRelations(t *testing.T) {
	arch := func() *FitSpline {
		return mustFit(t, NewPoint(-4, 0), NewPoint(0, 3), NewPoint(4, 0))
	}
	cases := []struct {
		name   string
		curves []Curve
		closed []ClosedCurve
		want   pairVerdict
	}{
		{
			name:   "line far above",
			curves: []Curve{arch(), NewLine(NewPoint(-5, 6), NewPoint(5, 6))},
			want:   verdictDisjoint,
		},
		{
			name:   "line a hair above the apex",
			curves: []Curve{arch(), NewLine(NewPoint(-1, 3+1e-9), NewPoint(1, 3+1e-9))},
			want:   verdictDisjoint,
		},
		{
			name:   "line through the apex",
			curves: []Curve{arch(), NewLine(NewPoint(-1, 2), NewPoint(1, 4))},
			want:   verdictUnproven,
		},
		{
			name:   "line tangent at the apex",
			curves: []Curve{arch(), NewLine(NewPoint(-1, 3), NewPoint(1, 3))},
			want:   verdictUnproven,
		},
		{
			name:   "line a hair below the apex crosses twice",
			curves: []Curve{arch(), NewLine(NewPoint(-1, 3-1e-9), NewPoint(1, 3-1e-9))},
			want:   verdictUnproven,
		},
		{
			name:   "chord shares both ends",
			curves: []Curve{arch(), NewLine(NewPoint(4, 0), NewPoint(-4, 0))},
			want:   verdictEndpointOnly,
		},
		{
			name:   "line leaves one end at an angle",
			curves: []Curve{arch(), NewLine(NewPoint(4, 0), NewPoint(8, 4))},
			want:   verdictEndpointOnly,
		},
		{
			name:   "line leaves the end and re-crosses the spline",
			curves: []Curve{arch(), NewLine(NewPoint(4, 0), NewPoint(-4, 2))},
			want:   verdictUnproven,
		},
		{
			name:   "line ends on the spline interior",
			curves: []Curve{arch(), NewLine(NewPoint(0, 3), NewPoint(0, 8))},
			want:   verdictUnproven,
		},
		{
			name:   "circle far away",
			closed: []ClosedCurve{NewCircle(NewPoint(0, 20), 3)},
			curves: []Curve{arch()},
			want:   verdictDisjoint,
		},
		{
			name:   "circle crossing the spline",
			closed: []ClosedCurve{NewCircle(NewPoint(0, 3), 1)},
			curves: []Curve{arch()},
			want:   verdictUnproven,
		},
		{
			name:   "circle inside the arch",
			closed: []ClosedCurve{NewCircle(NewPoint(0, 1), 0.5)},
			curves: []Curve{arch()},
			want:   verdictDisjoint,
		},
		{
			name:   "arc whose circle crosses the spline outside its sweep",
			curves: []Curve{arch(), NewArc(NewPoint(0, 0), NewPoint(0, -5), NewPoint(3, -4))},
			want:   verdictDisjoint,
		},
		{
			name:   "arc crossing the spline",
			curves: []Curve{arch(), NewArc(NewPoint(0, 0), NewPoint(0, 3.5), NewPoint(3.5, 0))},
			want:   verdictUnproven,
		},
		{
			name:   "arc well outside the spline",
			curves: []Curve{arch(), NewArc(NewPoint(0, 0), NewPoint(0, 5), NewPoint(5, 0))},
			want:   verdictDisjoint,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			curves := tc.curves
			e, _ := ledgerPair(t, curves, tc.closed)
			require.Equalf(t, tc.want, e.verdict, "reason %d", e.reason)
		})
	}
}

func TestPairLedgerSplinesSharingAnEnd(t *testing.T) {
	a := mustFit(t, NewPoint(0, 0), NewPoint(2, 1), NewPoint(4, 0))
	apart := mustFit(t, NewPoint(4, 0), NewPoint(6, 2), NewPoint(8, 2))
	e, _ := ledgerPairFits(t, a, apart)
	require.Equal(t, verdictEndpointOnly, e.verdict, "two splines leaving one point at an angle")

	straight := mustFit(t, NewPoint(0, 0), NewPoint(2, 0), NewPoint(4, 0))
	onward := mustFit(t, NewPoint(4, 0), NewPoint(6, 0), NewPoint(8, 0))
	e, _ = ledgerPairFits(t, straight, onward)
	require.Equal(t, verdictEndpointOnly, e.verdict, "a smooth join leaves the shared end in opposite directions")

	twin := mustFit(t, NewPoint(0, 0), NewPoint(2, 1), NewPoint(4, 0))
	e, _ = ledgerPairFits(t, a, twin)
	require.Equal(t, verdictUnproven, e.verdict, "two splines through the same points overlap")

	back := mustFit(t, NewPoint(4, 0), NewPoint(2, 0.5), NewPoint(0, 0.2))
	e, _ = ledgerPairFits(t, a, back)
	require.Equal(t, verdictUnproven, e.verdict, "the second spline runs back through the first")
}

func ledgerPairFits(t *testing.T, a, b *FitSpline) (pairEntry, *arranger) {
	t.Helper()
	arr := proveOnly([]Curve{a, b}, nil)
	require.NotNil(t, arr.ledger)
	return arr.ledger[pairKey(0, 1)], arr
}

func TestPairLedgerSelfEntry(t *testing.T) {
	simple := mustFit(t, NewPoint(0, 0), NewPoint(2, 1), NewPoint(4, 0), NewPoint(6, 2))
	arr := proveOnly([]Curve{simple}, nil)
	require.NotNil(t, arr.ledger)
	require.Equal(t, verdictSimple, arr.ledger[[2]int{0, 0}].verdict)

	loop := mustFit(t, NewPoint(0, 0), NewPoint(4, 0), NewPoint(4, 4), NewPoint(0, 4), NewPoint(0, -1))
	arr = proveOnly([]Curve{loop}, nil)
	require.NotNil(t, arr.ledger)
	require.Equal(t, verdictUnproven, arr.ledger[[2]int{0, 0}].verdict, "a spiral that turns past a half turn")
}

// denseCurve samples source i at n+1 parameters.
func denseCurve(a *arranger, i, n int) [][2]float64 {
	pts := make([][2]float64, n+1)
	for k := range pts {
		pts[k] = a.sources[i].at(float64(k) / float64(n))
	}
	return pts
}

// denseCross reports whether two dense polylines cross or touch anywhere farther
// than keep from every listed point.
func denseCross(p, q [][2]float64, ends [][2]float64, keep float64) bool {
	near := func(x, y float64) bool {
		for _, e := range ends {
			if math.Hypot(x-e[0], y-e[1]) <= keep {
				return true
			}
		}
		return false
	}
	for i := 0; i+1 < len(p); i++ {
		for j := 0; j+1 < len(q); j++ {
			x, y, ok := segmentMeet(p[i], p[i+1], q[j], q[j+1])
			if ok && !near(x, y) {
				return true
			}
		}
	}
	return false
}

func segmentMeet(a, b, c, d [2]float64) (float64, float64, bool) {
	r := [2]float64{b[0] - a[0], b[1] - a[1]}
	s := [2]float64{d[0] - c[0], d[1] - c[1]}
	den := r[0]*s[1] - r[1]*s[0]
	if den == 0 {
		return 0, 0, false
	}
	t := ((c[0]-a[0])*s[1] - (c[1]-a[1])*s[0]) / den
	u := ((c[0]-a[0])*r[1] - (c[1]-a[1])*r[0]) / den
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return 0, 0, false
	}
	return a[0] + t*r[0], a[1] + t*r[1], true
}

// TestPairLedgerNeverCertifiesAMeetingPair is the soundness property: over random
// scenes, a pair certified disjoint or endpoint-only has no crossing that a dense
// sampling can find, outside the neighbourhood of its shared ends.
func TestPairLedgerNeverCertifiesAMeetingPair(t *testing.T) {
	rng := rand.New(rand.NewSource(20261022))
	pt := func() *Point { return NewPoint(rng.Float64()*20-10, rng.Float64()*20-10) }
	counts := map[pairVerdict]int{}
	for trial := 0; trial < 400; trial++ {
		n := 2 + rng.Intn(5)
		fitPts := make([]*Point, n)
		for i := range fitPts {
			fitPts[i] = pt()
		}
		fit, err := NewFitSpline(fitPts...)
		require.NoError(t, err)
		shared := rng.Intn(3) == 0
		start := pt()
		if shared {
			start = fitPts[len(fitPts)-1]
		}
		var curves []Curve
		var closed []ClosedCurve
		curves = append(curves, fit)
		switch rng.Intn(4) {
		case 3:
			other := make([]*Point, 2+rng.Intn(4))
			for i := range other {
				other[i] = pt()
			}
			if shared {
				other[0] = fitPts[len(fitPts)-1]
			}
			second, err := NewFitSpline(other...)
			require.NoError(t, err)
			curves = append(curves, second)
		case 0:
			curves = append(curves, NewLine(start, pt()))
		case 1:
			closed = append(closed, NewCircle(pt(), 0.5+rng.Float64()*8))
		default:
			centre := pt()
			from := start
			to := pt()
			curves = append(curves, NewArc(centre, from, to))
		}
		a := proveOnly(curves, closed)
		if a.ledger == nil {
			continue
		}
		e := a.ledger[pairKey(0, 1)]
		counts[e.verdict]++
		if e.verdict != verdictDisjoint && e.verdict != verdictEndpointOnly {
			continue
		}
		keep := 1e-3 * a.scale
		require.Falsef(t, denseCross(denseCurve(a, 0, 400), denseCurve(a, 1, 400), e.ends, keep),
			"trial %d certified verdict %d but a dense sampling finds a crossing", trial, e.verdict)
	}
	t.Logf("verdicts over 400 scenes: %v", counts)
	require.Positive(t, counts[verdictDisjoint], "the generator must produce disjoint pairs")
	require.Positive(t, counts[verdictUnproven], "the generator must produce meeting pairs")
}

func hullOf(pts ...[2]float64) exactFitHull {
	var h exactFitHull
	for i, p := range pts {
		h.points[i] = [2]*big.Rat{rat(p[0]), rat(p[1])}
	}
	return h
}

func TestConesApart(t *testing.T) {
	// Every hull leaves the origin from point 0 (end 0).
	right := hullOf([2]float64{0, 0}, [2]float64{1, 0}, [2]float64{2, 0}, [2]float64{3, 0})
	upper := hullOf([2]float64{0, 0}, [2]float64{1, 1}, [2]float64{2, 1}, [2]float64{3, 3})
	lower := hullOf([2]float64{0, 0}, [2]float64{1, -1}, [2]float64{2, -1}, [2]float64{3, -3})
	wide := hullOf([2]float64{0, 0}, [2]float64{1, 1}, [2]float64{2, -1}, [2]float64{3, 0})
	back := hullOf([2]float64{0, 0}, [2]float64{-1, 0}, [2]float64{-2, 0}, [2]float64{-3, 0})
	loop := hullOf([2]float64{0, 0}, [2]float64{1, 0}, [2]float64{-1, 1}, [2]float64{-1, -1})

	require.True(t, conesApart(&upper, 0, &lower, 0), "cones on opposite sides of the x axis")
	require.True(t, conesApart(&right, 0, &back, 0), "two rays pointing in opposite directions")
	require.False(t, conesApart(&right, 0, &right, 0), "the same ray")
	require.False(t, conesApart(&wide, 0, &right, 0), "a cone holding the other's ray")
	require.False(t, conesApart(&upper, 0, &wide, 0), "cones that overlap in an interval")
	require.False(t, conesApart(&loop, 0, &upper, 0), "a cone of more than a half turn is refused")
	// End 1 reads the control points from the other side.
	reversed := hullOf([2]float64{3, 3}, [2]float64{2, 1}, [2]float64{1, 1}, [2]float64{0, 0})
	require.True(t, conesApart(&reversed, 1, &lower, 0))
}

func TestSplitAndSubHullAgreeWithEvaluation(t *testing.T) {
	h := hullOf([2]float64{0, 0}, [2]float64{1, 2}, [2]float64{3, -1}, [2]float64{4, 1})
	bez := func(h exactFitHull, u float64) [2]float64 {
		b := [4]float64{(1 - u) * (1 - u) * (1 - u), 3 * u * (1 - u) * (1 - u), 3 * u * u * (1 - u), u * u * u}
		var out [2]float64
		for i := range b {
			for axis := range 2 {
				f, _ := h.points[i][axis].Float64()
				out[axis] += b[i] * f
			}
		}
		return out
	}
	left, right := splitHull(h, big.NewRat(1, 4))
	for _, u := range []float64{0, 0.3, 0.7, 1} {
		want := bez(h, 0.25*u)
		got := bez(left, u)
		require.InDelta(t, want[0], got[0], 1e-12)
		require.InDelta(t, want[1], got[1], 1e-12)
		want = bez(h, 0.25+0.75*u)
		got = bez(right, u)
		require.InDelta(t, want[0], got[0], 1e-12)
		require.InDelta(t, want[1], got[1], 1e-12)
	}
	mid := subHull(h, big.NewRat(1, 4), big.NewRat(3, 4))
	for _, u := range []float64{0, 0.5, 1} {
		want := bez(h, 0.25+0.5*u)
		got := bez(mid, u)
		require.InDelta(t, want[0], got[0], 1e-12)
		require.InDelta(t, want[1], got[1], 1e-12)
	}
}
