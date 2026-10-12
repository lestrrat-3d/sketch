package sketch_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

const arcOracleBits = 256

func arcOracleFloat(v float64) *big.Float {
	return new(big.Float).SetPrec(arcOracleBits).SetFloat64(v)
}

func arcOracleAtanInverse(n int64) *big.Float {
	x := new(big.Float).SetPrec(arcOracleBits).Quo(arcOracleFloat(1), arcOracleFloat(float64(n)))
	x2 := new(big.Float).SetPrec(arcOracleBits).Mul(x, x)
	term, sum := new(big.Float).Set(x), arcOracleFloat(0)
	for k := 0; k < 128; k++ {
		part := new(big.Float).SetPrec(arcOracleBits).Quo(term, arcOracleFloat(float64(2*k+1)))
		if k%2 == 0 {
			sum.Add(sum, part)
		} else {
			sum.Sub(sum, part)
		}
		term.Mul(term, x2)
	}
	return sum
}

func arcOracleSinCos(x *big.Float) (*big.Float, *big.Float) {
	x2 := new(big.Float).SetPrec(arcOracleBits).Mul(x, x)
	sinTerm, cosTerm := new(big.Float).Set(x), arcOracleFloat(1)
	sin, cos := arcOracleFloat(0), arcOracleFloat(0)
	for k := 0; k < 90; k++ {
		sin.Add(sin, sinTerm)
		cos.Add(cos, cosTerm)
		sinTerm.Mul(sinTerm, x2)
		sinTerm.Quo(sinTerm, arcOracleFloat(float64((2*k+2)*(2*k+3))))
		sinTerm.Neg(sinTerm)
		cosTerm.Mul(cosTerm, x2)
		cosTerm.Quo(cosTerm, arcOracleFloat(float64((2*k+1)*(2*k+2))))
		cosTerm.Neg(cosTerm)
	}
	return sin, cos
}

func arcOraclePoint(t float64) (*big.Float, *big.Float) {
	// Machin's formula computes pi independently of Sketch's trig brackets.
	pi := new(big.Float).SetPrec(arcOracleBits).Mul(arcOracleAtanInverse(5), arcOracleFloat(16))
	pi.Sub(pi, new(big.Float).SetPrec(arcOracleBits).Mul(arcOracleAtanInverse(239), arcOracleFloat(4)))
	angle := new(big.Float).SetPrec(arcOracleBits).Mul(pi, arcOracleFloat(t))
	angle.Mul(angle, arcOracleFloat(1.5))
	sin, cos := arcOracleSinCos(angle)
	// Centre (11,-7), start vector (3,4), then rotate through 3*pi/2.
	x := new(big.Float).SetPrec(arcOracleBits).Sub(
		new(big.Float).SetPrec(arcOracleBits).Mul(cos, arcOracleFloat(3)),
		new(big.Float).SetPrec(arcOracleBits).Mul(sin, arcOracleFloat(4)))
	x.Add(x, arcOracleFloat(11))
	y := new(big.Float).SetPrec(arcOracleBits).Add(
		new(big.Float).SetPrec(arcOracleBits).Mul(cos, arcOracleFloat(4)),
		new(big.Float).SetPrec(arcOracleBits).Mul(sin, arcOracleFloat(3)))
	y.Add(y, arcOracleFloat(-7))
	return x, y
}

func requireArcOracleInside(t *testing.T, box sketch.Interval, point *big.Float) {
	t.Helper()
	require.LessOrEqual(t, new(big.Float).SetFloat64(box.Lo).Cmp(point), 0)
	require.GreaterOrEqual(t, new(big.Float).SetFloat64(box.Hi).Cmp(point), 0)
}

func TestArcCertifiedSpanBoundsEnclosesQuarterAndWrappedArc(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	c := s.CreatePoint(0, 0)
	a := s.CreateArc(c, s.CreatePoint(2, 0), s.CreatePoint(0, 2))
	x, y, err := a.CertifiedSpanBounds(0, 1)
	require.NoError(t, err)
	require.LessOrEqual(t, x.Lo, 0.0)
	require.GreaterOrEqual(t, x.Hi, 2.0)
	require.LessOrEqual(t, y.Lo, 0.0)
	require.GreaterOrEqual(t, y.Hi, 2.0)
	x, y, err = a.CertifiedSpanBounds(0, 0.25)
	require.NoError(t, err)
	require.Greater(t, x.Lo, 1.0)
	require.GreaterOrEqual(t, x.Hi, 2.0)
	require.GreaterOrEqual(t, y.Lo, -1e-12)
	require.Less(t, y.Hi, 1.0)

	wrapped := s.CreateArc(c, s.CreatePoint(0, -2), s.CreatePoint(2, 0))
	x, y, err = wrapped.CertifiedSpanBounds(0, 1)
	require.NoError(t, err)
	require.LessOrEqual(t, x.Lo, 0.0)
	require.GreaterOrEqual(t, x.Hi, 2.0)
	require.LessOrEqual(t, y.Lo, -2.0)
	require.GreaterOrEqual(t, y.Hi, 0.0)
}

func TestArcCertifiedSpanBoundsRefusesUndecidableOrDeadSource(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	c := s.CreatePoint(0, 0)
	p := s.CreatePoint(2, 0)
	a := s.CreateArc(c, p, p)
	_, _, err = a.CertifiedSpanBounds(0, 1)
	require.ErrorIs(t, err, sketch.ErrNotCertified)
	_, _, err = a.CertifiedSpanBounds(-0.1, 1)
	require.ErrorIs(t, err, sketch.ErrNotCertified)
	require.True(t, s.RemoveEntity(a))
	_, _, err = a.CertifiedSpanBounds(0, 1)
	require.True(t, errors.Is(err, sketch.ErrNotCertified))
}

func TestArcCertifiedSpanBoundsEnclosesTranslatedMajorArcOracle(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	arc := s.CreateArc(s.CreatePoint(11, -7), s.CreatePoint(14, -3), s.CreatePoint(15, -10))
	wideX, wideY, err := arc.CertifiedSpanBounds(0.125, 0.875)
	require.NoError(t, err)
	for _, parameter := range [...]float64{0.125, 0.25, 0.375, 0.5, 0.625, 0.75, 0.875} {
		wantX, wantY := arcOraclePoint(parameter)
		pointX, pointY, err := arc.CertifiedSpanBounds(parameter, parameter)
		require.NoError(t, err)
		requireArcOracleInside(t, pointX, wantX)
		requireArcOracleInside(t, pointY, wantY)
		requireArcOracleInside(t, wideX, wantX)
		requireArcOracleInside(t, wideY, wantY)
	}
}

func TestArcCertifiedSpanBoundsRefusesNearlyFullTurnAndTinyRadius(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	c := s.CreatePoint(0, 0)
	nearTurn := s.CreateArc(c, s.CreatePoint(5, 0), s.CreatePoint(5, -1e-18))
	_, _, err = nearTurn.CertifiedSpanBounds(0, 1)
	require.ErrorIs(t, err, sketch.ErrNotCertified)
	tiny := s.CreateArc(c, s.CreatePoint(1e-320, 0), s.CreatePoint(0, 1e-320))
	_, _, err = tiny.CertifiedSpanBounds(0, 1)
	require.ErrorIs(t, err, sketch.ErrNotCertified)
}

func TestArcCertifiedSpanBoundsRefusesStaleReference(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	c := s.CreateReferencePoint(0, 0, "source")
	start := s.CreateReferencePoint(3, 0, "source")
	end := s.CreateReferencePoint(0, 3, "source")
	arc, err := s.CreateReferenceArc(c, start, end, "source")
	require.NoError(t, err)
	_, _, err = arc.CertifiedSpanBounds(0, 1)
	require.NoError(t, err)
	s.MarkStale("source")
	_, _, err = arc.CertifiedSpanBounds(0, 1)
	require.ErrorIs(t, err, sketch.ErrNotCertified)
}
