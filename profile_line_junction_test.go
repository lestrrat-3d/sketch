package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func junctions(s *sketch.Sketch) [][2]sketch.BoundaryEdge {
	var pairs [][2]sketch.BoundaryEdge
	for _, p := range s.Profiles() {
		if !p.Valid {
			continue
		}
		for _, loop := range append([][]sketch.BoundaryEdge{p.Outer}, p.Holes...) {
			for i := range loop {
				pairs = append(pairs, [2]sketch.BoundaryEdge{loop[i], loop[(i+1)%len(loop)]})
			}
		}
	}
	return pairs
}

func TestCertifyLineLineJunctionProperPair(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(-5, -5, 5, 5)
	s.CreateRectangle(3, -2, 7, 2)
	var certified int
	var saved [2]sketch.BoundaryEdge
	for _, pair := range junctions(s) {
		point, ok := s.CertifyLineLineJunction(pair[0], pair[1])
		if !ok {
			continue
		}
		require.Equal(t, 5.0, point[0])
		require.Contains(t, []float64{-2, 2}, point[1])
		certified++
		saved = pair
	}
	require.Positive(t, certified)
	_, ok := s.CertifyLineLineJunction(saved[1], saved[0])
	require.False(t, ok, "walk order is part of the event witness")
	altered := saved[0]
	altered.Reversed = !altered.Reversed
	_, ok = s.CertifyLineLineJunction(altered, saved[1])
	require.False(t, ok, "a changed walk direction cannot reuse the event witness")
	altered = saved[0]
	altered.TEnd = (altered.TStart + altered.TEnd) / 2
	_, ok = s.CertifyLineLineJunction(altered, saved[1])
	require.False(t, ok, "a changed trim cannot reuse the event witness")
	altered = saved[0]
	altered.Partial = !altered.Partial
	_, ok = s.CertifyLineLineJunction(altered, saved[1])
	require.False(t, ok, "a changed whole/partial role cannot reuse the event witness")
	altered = saved[0]
	altered.Polyline = append([][2]float64(nil), altered.Polyline...)
	altered.Polyline[0][0]++
	_, ok = s.CertifyLineLineJunction(altered, saved[1])
	require.False(t, ok, "a changed polyline cannot reuse the event witness")
	var foundSecondPass bool
	for _, pair := range junctions(s) {
		if pair[0].Entity != saved[0].Entity || pair[1].Entity != saved[1].Entity {
			continue
		}
		foundSecondPass = true
		_, ok = s.CertifyLineLineJunction(saved[0], pair[1])
		require.False(t, ok, "separate arrangement passes have distinct event identities")
		break
	}
	require.True(t, foundSecondPass)
	s.CreateLine(s.CreatePoint(20, 0), s.CreatePoint(21, 0))
	_, ok = s.CertifyLineLineJunction(saved[0], saved[1])
	require.False(t, ok, "a stale arrangement cannot certify a live junction")
}

func TestCertifyLineLineJunctionMergedContact(t *testing.T) {
	s := newSketch(t)
	s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(2, 2))
	s.CreateLine(s.CreatePoint(0, 2), s.CreatePoint(2, 0))
	s.CreateLine(s.CreatePoint(0, 1), s.CreatePoint(2, 1))
	var ends, starts []sketch.BoundaryEdge
	for _, chain := range s.Chains() {
		for _, edge := range chain.Edges {
			if edge.Polyline[len(edge.Polyline)-1] == [2]float64{1, 1} {
				ends = append(ends, edge)
			}
			if edge.Polyline[0] == [2]float64{1, 1} {
				starts = append(starts, edge)
			}
		}
	}
	require.NotEmpty(t, ends)
	require.NotEmpty(t, starts)
	var checked int
	for _, left := range ends {
		for _, right := range starts {
			if left.Entity == right.Entity {
				continue
			}
			_, ok := s.CertifyLineLineJunction(left, right)
			require.False(t, ok, "three sources at one graph vertex are ambiguous")
			checked++
		}
	}
	require.Positive(t, checked)
}

func TestCertifyLineLineJunctionNonrepresentableRoot(t *testing.T) {
	s := newSketch(t)
	a0, a1, a2 := s.CreatePoint(0, 0), s.CreatePoint(2, 1), s.CreatePoint(0, 2)
	lineA := s.CreateLine(a0, a1)
	s.CreateLine(a1, a2)
	s.CreateLine(a2, a0)
	b0, b1, b2 := s.CreatePoint(0, 1), s.CreatePoint(1, 0), s.CreatePoint(2, 2)
	lineB := s.CreateLine(b0, b1)
	s.CreateLine(b1, b2)
	s.CreateLine(b2, b0)
	var found bool
	for _, pair := range junctions(s) {
		if pair[0].Entity != lineA && pair[0].Entity != lineB {
			continue
		}
		if pair[1].Entity != lineA && pair[1].Entity != lineB ||
			pair[0].Entity == pair[1].Entity {
			continue
		}
		found = true
		_, ok := s.CertifyLineLineJunction(pair[0], pair[1])
		require.False(t, ok, "the rational 2/3,1/3 crossing is not binary64 exact")
	}
	require.True(t, found)
}

func TestCertifyLineLineJunctionReversedChain(t *testing.T) {
	s := newSketch(t)
	s.CreateLine(s.CreatePoint(2, 2), s.CreatePoint(0, 0))
	s.CreateLine(s.CreatePoint(0, 2), s.CreatePoint(2, 0))
	var ends []sketch.BoundaryEdge
	var starts []sketch.BoundaryEdge
	for _, chain := range s.Chains() {
		require.True(t, chain.Valid)
		for _, edge := range chain.Edges {
			if edge.Polyline[len(edge.Polyline)-1] == [2]float64{1, 1} {
				ends = append(ends, edge)
			}
			if edge.Polyline[0] == [2]float64{1, 1} {
				starts = append(starts, edge)
			}
		}
	}
	var certifiedReversed bool
	for _, left := range ends {
		for _, right := range starts {
			if left.Entity == right.Entity || !left.Reversed && !right.Reversed {
				continue
			}
			point, ok := s.CertifyLineLineJunction(left, right)
			if ok {
				require.Equal(t, [2]float64{1, 1}, point)
				certifiedReversed = true
			}
		}
	}
	require.True(t, certifiedReversed)
}
