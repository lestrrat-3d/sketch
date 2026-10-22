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
			want:   verdictCrossing,
		},
		{
			name:   "circle touching the spline at its apex",
			closed: []ClosedCurve{NewCircle(NewPoint(0, 4.5), 1.5)},
			curves: []Curve{arch()},
			want:   verdictUnproven,
		},
		{
			name:   "circle through a fit point of the spline",
			closed: []ClosedCurve{NewCircle(NewPoint(1, 3), 1)},
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

// exactPiecesOf returns the exact power-basis pieces of source i, a fit spline.
func exactPiecesOf(t *testing.T, a *arranger, i int) []exactFitPiece {
	t.Helper()
	fi := a.sources[i].fitEval.interpolant()
	pieces, ok := exactFitPieces(fi)
	require.True(t, ok)
	return pieces
}

const oraclePrec = 256

// piecePoint evaluates a piece at u in 256-bit arithmetic.
func piecePoint(p *exactFitPiece, u *big.Float) (*big.Float, *big.Float) {
	eval := func(c [4]*big.Rat) *big.Float {
		acc := new(big.Float).SetPrec(oraclePrec)
		for k := 3; k >= 0; k-- {
			acc.Mul(acc, u)
			acc.Add(acc, new(big.Float).SetPrec(oraclePrec).SetRat(c[k]))
		}
		return acc
	}
	return eval(p.x), eval(p.y)
}

func oracleU(k, n int) *big.Float {
	return new(big.Float).SetPrec(oraclePrec).Quo(big.NewFloat(float64(k)), big.NewFloat(float64(n)))
}

// oracleContacts walks every piece of the spline at n+1 parameters per piece in
// 256-bit arithmetic and counts where f changes sign between neighbouring samples
// while the point lies within keep of nowhere in ends. f is the signed carrier
// function of the target and extent says whether a sample is inside the target.
func oracleContacts(pieces []exactFitPiece, n int, f func(x, y *big.Float) *big.Float,
	extent func(x, y *big.Float) bool, ends [][2]float64, keep float64) int {
	near := func(x, y *big.Float) bool {
		fx, _ := x.Float64()
		fy, _ := y.Float64()
		for _, e := range ends {
			if math.Hypot(fx-e[0], fy-e[1]) <= keep {
				return true
			}
		}
		return false
	}
	count := 0
	for pi := range pieces {
		var px, py, pf *big.Float
		for k := 0; k <= n; k++ {
			x, y := piecePoint(&pieces[pi], oracleU(k, n))
			v := f(x, y)
			if px != nil && extent(x, y) && !near(x, y) && v.Sign()*pf.Sign() <= 0 {
				count++
			}
			px, py, pf = x, y, v
		}
		_, _ = px, py
	}
	return count
}

// TestPairLedgerAgreesWithA256BitContactOracle checks every certified pair of the
// random scenes against an oracle that evaluates the exact pieces in 256-bit
// arithmetic and looks for a sign change of the target's carrier function.
func TestPairLedgerAgreesWithA256BitContactOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(7770001))
	pt := func() *Point { return NewPoint(rng.Float64()*20-10, rng.Float64()*20-10) }
	certified := 0
	for trial := 0; trial < 150; trial++ {
		fitPts := make([]*Point, 2+rng.Intn(5))
		for i := range fitPts {
			fitPts[i] = pt()
		}
		fit, err := NewFitSpline(fitPts...)
		require.NoError(t, err)
		start := pt()
		if rng.Intn(3) == 0 {
			start = fitPts[len(fitPts)-1]
		}
		var curves []Curve
		var closed []ClosedCurve
		curves = append(curves, fit)
		var carrier func(x, y *big.Float) *big.Float
		var extent func(x, y *big.Float) bool
		if rng.Intn(2) == 0 {
			end := pt()
			curves = append(curves, NewLine(start, end))
			dx, dy := end.X-start.X, end.Y-start.Y
			carrier = func(x, y *big.Float) *big.Float {
				a := new(big.Float).SetPrec(oraclePrec).Sub(y, big.NewFloat(start.Y))
				b := new(big.Float).SetPrec(oraclePrec).Sub(x, big.NewFloat(start.X))
				a.Mul(a, big.NewFloat(dx))
				b.Mul(b, big.NewFloat(dy))
				return a.Sub(a, b)
			}
			extent = func(x, y *big.Float) bool {
				fx, _ := x.Float64()
				fy, _ := y.Float64()
				s := ((fx-start.X)*dx + (fy-start.Y)*dy) / (dx*dx + dy*dy)
				return s >= 0 && s <= 1
			}
		} else {
			c := pt()
			r := 0.5 + rng.Float64()*8
			closed = append(closed, NewCircle(c, r))
			carrier = func(x, y *big.Float) *big.Float {
				a := new(big.Float).SetPrec(oraclePrec).Sub(x, big.NewFloat(c.X))
				b := new(big.Float).SetPrec(oraclePrec).Sub(y, big.NewFloat(c.Y))
				a.Mul(a, a)
				b.Mul(b, b)
				a.Add(a, b)
				return a.Sub(a, new(big.Float).SetPrec(oraclePrec).Mul(big.NewFloat(r), big.NewFloat(r)))
			}
			extent = func(x, y *big.Float) bool { return true }
		}
		a := proveOnly(curves, closed)
		if a.ledger == nil {
			continue
		}
		e := a.ledger[pairKey(0, 1)]
		if e.verdict != verdictDisjoint && e.verdict != verdictEndpointOnly {
			continue
		}
		certified++
		got := oracleContacts(exactPiecesOf(t, a, 0), 400, carrier, extent, e.ends, 1e-3*a.scale)
		require.Zerof(t, got, "trial %d verdict %d: the 256-bit oracle finds %d sign changes", trial, e.verdict, got)
	}
	require.Greater(t, certified, 20, "the generator must certify pairs")
}

// TestPairLedgerAtOneUlpFromTouching puts a horizontal line one unit in the last
// place on each side of the exact maximum of a spline. A line at or below the
// maximum meets the curve, so the proof must not call it disjoint.
func TestPairLedgerAtOneUlpFromTouching(t *testing.T) {
	rng := rand.New(rand.NewSource(424242))
	disjointAbove := 0
	for trial := 0; trial < 20; trial++ {
		n := 3 + rng.Intn(4)
		pts := make([]*Point, n)
		for i := range pts {
			x := -6 + 12*float64(i)/float64(n-1)
			pts[i] = NewPoint(x, 2+rng.Float64()*3)
		}
		fit, err := NewFitSpline(pts...)
		require.NoError(t, err)
		probe := proveOnly([]Curve{fit, NewLine(NewPoint(-1, 100), NewPoint(1, 100))}, nil)
		pieces := exactPiecesOf(t, probe, 0)
		top := maxY(pieces)
		topF, _ := top.Float64()
		for step := -3; step <= 3; step++ {
			c := topF
			for range max(step, -step) {
				if step < 0 {
					c = math.Nextafter(c, math.Inf(-1))
				} else {
					c = math.Nextafter(c, math.Inf(1))
				}
			}
			a := proveOnly([]Curve{fit, NewLine(NewPoint(-6.5, c), NewPoint(6.5, c))}, nil)
			e := a.ledger[pairKey(0, 1)]
			meets := new(big.Float).SetPrec(oraclePrec).SetFloat64(c).Cmp(top) <= 0
			if meets {
				require.NotEqualf(t, verdictDisjoint, e.verdict, "trial %d step %d: the line is not above the maximum", trial, step)
				require.NotEqual(t, verdictEndpointOnly, e.verdict)
			} else if e.verdict == verdictDisjoint {
				disjointAbove++
			}
		}
	}
	t.Logf("lines strictly above the maximum certified disjoint: %d", disjointAbove)
}

// maxY finds the largest y of the pieces in 256-bit arithmetic.
func maxY(pieces []exactFitPiece) *big.Float {
	best := new(big.Float).SetPrec(oraclePrec).SetInf(true)
	for pi := range pieces {
		bu := 0
		bestPiece := new(big.Float).SetPrec(oraclePrec).SetInf(true)
		const grid = 300
		for k := 0; k <= grid; k++ {
			_, y := piecePoint(&pieces[pi], oracleU(k, grid))
			if y.Cmp(bestPiece) > 0 {
				bestPiece, bu = y, k
			}
		}
		lo, hi := oracleU(max(bu-1, 0), grid), oracleU(min(bu+1, grid), grid)
		for range 200 {
			third := new(big.Float).SetPrec(oraclePrec).Quo(new(big.Float).Sub(hi, lo), big.NewFloat(3))
			m1 := new(big.Float).SetPrec(oraclePrec).Add(lo, third)
			m2 := new(big.Float).SetPrec(oraclePrec).Sub(hi, third)
			_, y1 := piecePoint(&pieces[pi], m1)
			_, y2 := piecePoint(&pieces[pi], m2)
			if y1.Cmp(y2) < 0 {
				lo = m1
			} else {
				hi = m2
			}
		}
		_, y := piecePoint(&pieces[pi], lo)
		if y.Cmp(bestPiece) > 0 {
			bestPiece = y
		}
		if bestPiece.Cmp(best) > 0 {
			best = bestPiece
		}
	}
	return best
}

// TestArcBoundsHoldTheArc samples random arcs (sweeps below a half turn, either
// direction, any quadrant) and requires every sample, and both end points, inside the
// box arcBounds returns. A box that misses part of the arc would make the pair ledger
// call a spline that touches the arc disjoint from it.
func TestArcBoundsHoldTheArc(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	tested := 0
	for trial := 0; trial < 500; trial++ {
		c := NewPoint(rng.Float64()*40-20, rng.Float64()*40-20)
		r := 0.01 + rng.Float64()*30
		start := rng.Float64() * 2 * math.Pi
		sweep := (rng.Float64()*0.99 + 0.005) * math.Pi
		if rng.Intn(2) == 0 {
			sweep = -sweep
		}
		s := NewPoint(c.X+r*math.Cos(start), c.Y+r*math.Sin(start))
		e := NewPoint(c.X+r*math.Cos(start+sweep), c.Y+r*math.Sin(start+sweep))
		a := newArranger([]Curve{NewArc(c, s, e)}, nil, arrangeConfig{})
		a.densify()
		src := &a.sources[0]
		b, ok := arcBounds(src)
		if math.Abs(src.sweep) >= math.Pi {
			require.Falsef(t, ok, "trial %d: a sweep of a half turn or more has no box", trial)
			continue
		}
		tested++
		require.Truef(t, ok, "trial %d: sweep %v", trial, src.sweep)
		inside := func(p [2]float64) bool {
			for axis := range 2 {
				v := new(big.Rat).SetFloat64(p[axis])
				if v.Cmp(b.min[axis]) < 0 || v.Cmp(b.max[axis]) > 0 {
					return false
				}
			}
			return true
		}
		for k := 0; k <= 2000; k++ {
			p := src.at(float64(k) / 2000)
			require.Truef(t, inside(p), "trial %d: arc point %d %v outside the box", trial, k, p)
		}
	}
	require.Greater(t, tested, 100, "the generator must produce arcs below a half turn")
}

// ledgerOf runs the pair proofs of the scene and returns every ledger entry.
func ledgerOf(curves []Curve, closed []ClosedCurve) map[[2]int]pairEntry {
	a := proveOnly(curves, closed)
	return a.ledger
}

// TestLedgerFloatPrefilterKeepsEveryVerdict runs the pair proofs of random scenes with
// and without the float boxes and requires identical entries: verdict, reason, shared
// ends and crossing parameters. The float box only decides a pair the exact box test
// would decide the same way, so no entry may differ. The scenes put splines near
// lines, circles, arcs and other splines, some touching and some a hair apart, since
// those are the pairs on which a box test and the proofs behind it disagree most easily.
func TestLedgerFloatPrefilterKeepsEveryVerdict(t *testing.T) {
	defer func(v bool) { ledgerFloatPrefilter = v }(ledgerFloatPrefilter)
	rng := rand.New(rand.NewSource(20261025))
	pt := func() *Point { return NewPoint(rng.Float64()*20-10, rng.Float64()*20-10) }
	gaps := []float64{0.2, 0.05, 1e-2, 1e-3, 0}
	pairs, verdicts := 0, map[pairVerdict]int{}
	for trial := 0; trial < 20; trial++ {
		var curves []Curve
		var closed []ClosedCurve
		for range 1 + rng.Intn(3) {
			fitPts := make([]*Point, 2+rng.Intn(5))
			for i := range fitPts {
				fitPts[i] = pt()
			}
			fit, err := NewFitSpline(fitPts...)
			require.NoError(t, err)
			curves = append(curves, fit)
			gap := gaps[rng.Intn(len(gaps))] * float64(1-2*rng.Intn(2))
			switch rng.Intn(5) {
			case 0:
				on := fitPts[rng.Intn(len(fitPts))]
				c := pt()
				closed = append(closed, NewCircle(c, math.Hypot(on.X-c.X, on.Y-c.Y)+gap))
			case 1:
				beside := make([]*Point, len(fitPts))
				for i, p := range fitPts {
					beside[i] = NewPoint(p.X+gap, p.Y+3*gap)
				}
				other, err := NewFitSpline(beside...)
				require.NoError(t, err)
				curves = append(curves, other)
			case 2:
				// A line starting on the spline's end, and one a hair off it.
				last := fitPts[len(fitPts)-1]
				curves = append(curves, NewLine(last, pt()), NewLine(NewPoint(last.X+gap, last.Y), pt()))
			case 3:
				c := pt()
				r := 0.5 + rng.Float64()*8
				a0 := rng.Float64() * 2 * math.Pi
				curves = append(curves, NewArc(c,
					NewPoint(c.X+r*math.Cos(a0), c.Y+r*math.Sin(a0)),
					NewPoint(c.X+r*math.Cos(a0+1+rng.Float64()), c.Y+r*math.Sin(a0+1+rng.Float64()))))
			default:
				curves = append(curves, NewLine(pt(), pt()))
			}
		}
		ledgerFloatPrefilter = true
		with := ledgerOf(curves, closed)
		ledgerFloatPrefilter = false
		without := ledgerOf(curves, closed)
		require.Equalf(t, len(without), len(with), "trial %d: ledger size", trial)
		for k, w := range without {
			g, ok := with[k]
			require.Truef(t, ok, "trial %d: pair %v missing", trial, k)
			require.Equalf(t, w.verdict, g.verdict, "trial %d: pair %v verdict", trial, k)
			require.Equalf(t, w.reason, g.reason, "trial %d: pair %v reason", trial, k)
			require.Equalf(t, w.ends, g.ends, "trial %d: pair %v ends", trial, k)
			require.Equalf(t, w.crossings, g.crossings, "trial %d: pair %v crossings", trial, k)
			verdicts[w.verdict]++
			pairs++
		}
	}
	require.Greater(t, pairs, 30)
	require.Greater(t, verdicts[verdictDisjoint], 8, "the scenes must contain separated pairs")
	require.Greater(t, verdicts[verdictUnproven]+verdicts[verdictEndpointOnly]+verdicts[verdictCrossing], 3,
		"the scenes must contain pairs the box test cannot settle")
}

// TestFloatHullBoxHoldsExactHull splits random cubics to depth 30 in exact and in float
// arithmetic and requires every float box to hold the exact bounds of the same piece.
func TestFloatHullBoxHoldsExactHull(t *testing.T) {
	rng := rand.New(rand.NewSource(20261026))
	for trial := 0; trial < 60; trial++ {
		scale := math.Pow(10, float64(rng.Intn(9)-4))
		var h exactFitHull
		for i := range h.points {
			for axis := range 2 {
				h.points[i][axis] = rat((rng.Float64()*2 - 1) * scale)
			}
		}
		h.fl = floatHullOf(&h)
		require.True(t, h.fl.ok)
		var check func(h exactFitHull, depth int)
		check = func(h exactFitHull, depth int) {
			box := h.fl.box()
			require.True(t, box.ok)
			exact := hullBounds(&h)
			for axis := range 2 {
				lo, _ := exact.min[axis].Float64()
				hi, _ := exact.max[axis].Float64()
				require.LessOrEqualf(t, box.min[axis], lo, "trial %d depth %d axis %d", trial, depth, axis)
				require.GreaterOrEqualf(t, box.max[axis], hi, "trial %d depth %d axis %d", trial, depth, axis)
			}
			if depth == 30 {
				return
			}
			l, r := splitHullHalf(h)
			// Follow one random child per level, so the cost stays linear in depth.
			if rng.Intn(2) == 0 {
				check(l, depth+1)
			} else {
				check(r, depth+1)
			}
		}
		check(h, 0)
	}
}
