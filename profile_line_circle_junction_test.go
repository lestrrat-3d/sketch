package sketch_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

type cancelAfterJunctionChecks struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
	checks    int
}

func (c *cancelAfterJunctionChecks) Err() error {
	c.checks++
	c.remaining--
	if c.remaining == 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCertifyLineCircleJunctionCancelsDuringSourceCopy(t *testing.T) {
	s := newSketch(t)
	s.CreateCircle(s.CreatePoint(0, 0), 10)
	s.CreateRectangle(-12, -2, 12, 2)
	for i := 0; i < 12; i++ {
		y := float64(20 + i)
		s.CreateLine(s.CreatePoint(20, y), s.CreatePoint(21, y))
	}
	var pair [2]sketch.BoundaryEdge
	var found bool
	for _, candidate := range junctions(s) {
		_, ok, err := s.CertifyLineCircleJunction(t.Context(), candidate[0], candidate[1])
		require.NoError(t, err)
		if ok {
			pair, found = candidate, true
			break
		}
	}
	require.True(t, found)
	base, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &cancelAfterJunctionChecks{Context: base, cancel: cancel, remaining: 6}
	w, ok, err := s.CertifyLineCircleJunction(ctx, pair[0], pair[1])
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, ok)
	require.Equal(t, sketch.LineCircleJunction{}, w)
	require.Equal(t, 6, ctx.checks)
}

func TestCertifyLineCircleJunctionRealProfiles(t *testing.T) {
	s := newSketch(t)
	s.CreateCircle(s.CreatePoint(0, 0), 10)
	s.CreateRectangle(-12, -2, 12, 2)
	var positive, negative, lineFirst, circleFirst, reversed int
	for _, pair := range junctions(s) {
		w, ok, err := s.CertifyLineCircleJunction(t.Context(), pair[0], pair[1])
		require.NoError(t, err)
		if !ok {
			continue
		}
		sources := w.Sources()
		require.True(t, sources[0].AtWalkEnd)
		require.False(t, sources[1].AtWalkEnd)
		if sources[0].Kind == sketch.JunctionLine {
			lineFirst++
			require.Equal(t, sketch.JunctionCircle, sources[1].Kind)
		} else {
			circleFirst++
			require.Equal(t, sketch.JunctionCircle, sources[0].Kind)
			require.Equal(t, sketch.JunctionLine, sources[1].Kind)
		}
		if sources[0].Reversed || sources[1].Reversed {
			reversed++
		}
		x, y := w.Point()
		xLo, xHi := x.Bounds()
		yLo, yHi := y.Bounds()
		require.NotNil(t, xLo)
		require.NotNil(t, yLo)
		require.LessOrEqual(t, xLo.Cmp(xHi), 0)
		require.LessOrEqual(t, yLo.Cmp(yHi), 0)
		if xLo.Sign() > 0 {
			positive++
			lo2 := new(big.Rat).Mul(xLo, xLo)
			hi2 := new(big.Rat).Mul(xHi, xHi)
			require.LessOrEqual(t, lo2.Cmp(big.NewRat(96, 1)), 0)
			require.GreaterOrEqual(t, hi2.Cmp(big.NewRat(96, 1)), 0)
		} else if xHi.Sign() < 0 {
			negative++
			lo2 := new(big.Rat).Mul(xHi, xHi)
			hi2 := new(big.Rat).Mul(xLo, xLo)
			require.LessOrEqual(t, lo2.Cmp(big.NewRat(96, 1)), 0)
			require.GreaterOrEqual(t, hi2.Cmp(big.NewRat(96, 1)), 0)
		}
		require.True(t, yLo.Cmp(big.NewRat(2, 1)) <= 0 && yHi.Cmp(big.NewRat(2, 1)) >= 0 ||
			yLo.Cmp(big.NewRat(-2, 1)) <= 0 && yHi.Cmp(big.NewRat(-2, 1)) >= 0)
		xLo.SetInt64(1000)
		freshLo, _ := x.Bounds()
		require.NotEqual(t, 0, freshLo.Cmp(big.NewRat(1000, 1)))
		for _, b := range []sketch.RationalBracket{x, y} {
			lo, hi := b.Bounds()
			width := new(big.Rat).Sub(hi, lo)
			require.LessOrEqual(t, width.Cmp(new(big.Rat).SetFrac64(12, 1<<48)), 0)
		}
		left, right := w.Parameters()
		for _, b := range []sketch.RationalBracket{left, right} {
			lo, hi := b.Bounds()
			width := new(big.Rat).Sub(hi, lo)
			require.LessOrEqual(t, width.Cmp(new(big.Rat).SetFrac64(1, 1<<48)), 0)
		}
	}
	require.Positive(t, positive)
	require.Positive(t, negative)
	require.Positive(t, lineFirst)
	require.Positive(t, circleFirst)
	require.Positive(t, reversed)
}

func TestCertifyLineCircleJunctionSnapshotAndContext(t *testing.T) {
	s := newSketch(t)
	s.CreateCircle(s.CreatePoint(0, 0), 10)
	s.CreateRectangle(-12, -2, 12, 2)
	var pair [2]sketch.BoundaryEdge
	var found bool
	for _, candidate := range junctions(s) {
		_, ok, err := s.CertifyLineCircleJunction(t.Context(), candidate[0], candidate[1])
		require.NoError(t, err)
		if ok {
			pair, found = candidate, true
			break
		}
	}
	require.True(t, found)
	_, ok, err := s.CertifyLineCircleJunction(nil, pair[0], pair[1])
	require.False(t, ok)
	require.ErrorIs(t, err, sketch.ErrInvalidJunctionContext)
	_, ok, err = s.CertifyLineCircleJunction(t.Context(), pair[1], pair[0])
	require.NoError(t, err)
	require.False(t, ok)
	altered := pair[0]
	altered.TEnd = (altered.TStart + altered.TEnd) / 2
	_, ok, err = s.CertifyLineCircleJunction(t.Context(), altered, pair[1])
	require.NoError(t, err)
	require.False(t, ok)
	altered = pair[0]
	altered.Reversed = !altered.Reversed
	_, ok, err = s.CertifyLineCircleJunction(t.Context(), altered, pair[1])
	require.NoError(t, err)
	require.False(t, ok)
	altered = pair[0]
	altered.Polyline = append([][2]float64(nil), altered.Polyline...)
	altered.Polyline[0][0]++
	_, ok, err = s.CertifyLineCircleJunction(t.Context(), altered, pair[1])
	require.NoError(t, err)
	require.False(t, ok)
	for _, candidate := range junctions(s) {
		if candidate[0].Entity != pair[0].Entity || candidate[1].Entity != pair[1].Entity {
			continue
		}
		_, ok, err = s.CertifyLineCircleJunction(t.Context(), pair[0], candidate[1])
		require.NoError(t, err)
		require.False(t, ok)
		break
	}
	s.CreateLine(s.CreatePoint(20, 0), s.CreatePoint(21, 0))
	_, ok, err = s.CertifyLineCircleJunction(t.Context(), pair[0], pair[1])
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCertifyLineCircleJunctionMergedThirdSource(t *testing.T) {
	s := newSketch(t)
	s.CreateCircle(s.CreatePoint(0, 0), 5)
	s.CreateRectangle(-6, 3, 6, 4)
	var baseline int
	for _, pair := range junctions(s) {
		w, ok, err := s.CertifyLineCircleJunction(t.Context(), pair[0], pair[1])
		require.NoError(t, err)
		if !ok {
			continue
		}
		x, y := w.Point()
		xLo, xHi := x.Bounds()
		yLo, yHi := y.Bounds()
		if xLo.Cmp(big.NewRat(4, 1)) <= 0 && xHi.Cmp(big.NewRat(4, 1)) >= 0 &&
			yLo.Cmp(big.NewRat(3, 1)) <= 0 && yHi.Cmp(big.NewRat(3, 1)) >= 0 {
			baseline++
		}
	}
	require.Positive(t, baseline)
	s.CreateLine(s.CreatePoint(4, 2), s.CreatePoint(4, 5))
	var checked int
	for _, p := range s.Profiles() {
		for _, loop := range append([][]sketch.BoundaryEdge{p.Outer}, p.Holes...) {
			for i := range loop {
				left, right := loop[i], loop[(i+1)%len(loop)]
				if left.Polyline[len(left.Polyline)-1] != [2]float64{4, 3} ||
					right.Polyline[0] != [2]float64{4, 3} {
					continue
				}
				_, ok, err := s.CertifyLineCircleJunction(t.Context(), left, right)
				require.NoError(t, err)
				require.False(t, ok)
				checked++
			}
		}
	}
	require.Positive(t, checked)
}

func TestCertifyLineCircleJunctionRightTabSurvivesSecondTab(t *testing.T) {
	s := newSketch(t)
	s.CreateCircle(s.CreatePoint(0, 0), 10)
	s.CreateRectangle(8, -2, 12, 2)
	countRight := func() int {
		var count int
		for _, pair := range junctions(s) {
			w, ok, err := s.CertifyLineCircleJunction(t.Context(), pair[0], pair[1])
			require.NoError(t, err)
			if !ok {
				continue
			}
			x, _ := w.Point()
			lo, _ := x.Bounds()
			if lo.Sign() > 0 {
				count++
			}
		}
		return count
	}
	require.Positive(t, countRight())
	s.CreateRectangle(-12, -2, -8, 2)
	require.Positive(t, countRight(), "the independent left tab must not erase the right source event")
}
