package geom

import (
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// circleSignChanges walks every piece of the spline at n+1 parameters per piece in
// 256-bit arithmetic and returns, for each sign change of |P-c|^2 - r^2 between
// neighbouring samples, the global parameter interval that holds it.
func circleSignChanges(pieces []exactFitPiece, n int, c *Point, r float64) [][2]float64 {
	cx := new(big.Float).SetPrec(oraclePrec).SetFloat64(c.X)
	cy := new(big.Float).SetPrec(oraclePrec).SetFloat64(c.Y)
	r2 := new(big.Float).SetPrec(oraclePrec).Mul(big.NewFloat(r), big.NewFloat(r))
	f := func(x, y *big.Float) *big.Float {
		a := new(big.Float).SetPrec(oraclePrec).Sub(x, cx)
		b := new(big.Float).SetPrec(oraclePrec).Sub(y, cy)
		a.Mul(a, a)
		b.Mul(b, b)
		return a.Add(a, b).Sub(a, r2)
	}
	var out [][2]float64
	for pi := range pieces {
		p := &pieces[pi]
		var prev *big.Float
		for k := 0; k <= n; k++ {
			x, y := piecePoint(p, oracleU(k, n))
			v := f(x, y)
			if prev != nil && v.Sign()*prev.Sign() <= 0 {
				lo := p.t0 + (p.t1-p.t0)*float64(k-1)/float64(n)
				hi := p.t0 + (p.t1-p.t0)*float64(k)/float64(n)
				out = append(out, [2]float64{lo, hi})
			}
			prev = v
		}
	}
	return out
}

// TestPairLedgerCrossingAgreesWithA256BitOracle checks every crossing verdict of
// random scenes against an oracle that shares no code with the certificate: the
// number of crossings must equal the number of sign changes of the circle's carrier
// function along the exact pieces, and each reported parameter must lie in its sign
// change's sample interval and put the evaluated point on the circle.
func TestPairLedgerCrossingAgreesWithA256BitOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(7770042))
	pt := func() *Point { return NewPoint(rng.Float64()*20-10, rng.Float64()*20-10) }
	crossings, total := 0, 0
	for trial := 0; trial < 300; trial++ {
		fitPts := make([]*Point, 2+rng.Intn(6))
		for i := range fitPts {
			fitPts[i] = pt()
		}
		fit, err := NewFitSpline(fitPts...)
		require.NoError(t, err)
		// A circle centred on a point of the curve crosses it more often than a random one.
		on := fitPts[rng.Intn(len(fitPts))]
		c := NewPoint(on.X+rng.Float64()*4-2, on.Y+rng.Float64()*4-2)
		r := 0.3 + rng.Float64()*6
		a := proveOnly([]Curve{fit}, []ClosedCurve{NewCircle(c, r)})
		if a.ledger == nil {
			continue
		}
		total++
		e := a.ledger[pairKey(0, 1)]
		if e.verdict != verdictCrossing {
			continue
		}
		crossings++
		want := circleSignChanges(exactPiecesOf(t, a, 0), 400, c, r)
		require.Lenf(t, e.crossings, len(want), "trial %d: crossing count", trial)
		for i, tt := range e.crossings {
			require.GreaterOrEqualf(t, tt, want[i][0]-1e-12, "trial %d crossing %d", trial, i)
			require.LessOrEqualf(t, tt, want[i][1]+1e-12, "trial %d crossing %d", trial, i)
			p := a.sources[0].at(tt)
			require.InDeltaf(t, r, math.Hypot(p[0]-c.X, p[1]-c.Y), 1e-9, "trial %d crossing %d is on the circle", trial, i)
		}
	}
	require.Greater(t, crossings, 40, "the generator must certify crossings (%d scenes)", total)
}

// TestPairLedgerCrossingRoundsOntoJoint puts a fit point within rounding of the
// circle, so a root lies a hair from a piece joint. The pair is a crossing at the
// joint's own parameter or is unproven, and it is never a different pair relation.
func TestPairLedgerCrossingRoundsOntoJoint(t *testing.T) {
	fit := mustFit(t, NewPoint(-4, 0), NewPoint(0, 3), NewPoint(4, 0))
	// The circle passes through the middle fit point up to one rounding of its radius.
	r := math.Nextafter(1.5, 2)
	a := proveOnly([]Curve{fit}, []ClosedCurve{NewCircle(NewPoint(0, 1.5), r)})
	e := a.ledger[pairKey(0, 1)]
	if e.verdict == verdictCrossing {
		require.Len(t, e.crossings, 2)
		return
	}
	require.Equal(t, verdictUnproven, e.verdict)
}
