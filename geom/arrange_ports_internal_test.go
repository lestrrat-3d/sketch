package geom

import (
	"math"
	"testing"
)

func TestSourceDifferentialFD(t *testing.T) {
	srcs := []source{
		{kind: srcLine, ax: 1, ay: 2, bx: 5, by: -3},
		{kind: srcCircle, cx: 1, cy: -2, r: 4},
		{kind: srcArc, cx: 0, cy: 0, r: 3, phi0: 0.4, sweep: 1.9},
		{kind: srcArc, cx: 2, cy: 1, r: 2.5, phi0: 1.0, sweep: -2.2}, // CW arc
	}
	const h = 1e-6
	for si, s := range srcs {
		for _, tt := range []float64{0.1, 0.37, 0.5, 0.83} {
			d1, d2, ok := s.differential(tt)
			if !ok {
				t.Fatalf("src %d: differential ok=false", si)
			}
			p0, pm, pp := s.at(tt), s.at(tt-h), s.at(tt+h)
			fd1 := [2]float64{(pp[0] - pm[0]) / (2 * h), (pp[1] - pm[1]) / (2 * h)}
			fd2 := [2]float64{(pp[0] - 2*p0[0] + pm[0]) / (h * h), (pp[1] - 2*p0[1] + pm[1]) / (h * h)}
			if math.Abs(d1[0]-fd1[0]) > 1e-3 || math.Abs(d1[1]-fd1[1]) > 1e-3 {
				t.Errorf("src %d t=%.2f d1 analytic %v vs FD %v", si, tt, d1, fd1)
			}
			if math.Abs(d2[0]-fd2[0]) > 1e-1 || math.Abs(d2[1]-fd2[1]) > 1e-1 {
				t.Errorf("src %d t=%.2f d2 analytic %v vs FD %v", si, tt, d2, fd2)
			}
		}
	}
}

func TestSortExactPortsTotalOrder(t *testing.T) {
	// Regression for the intransitive-comparator bug: three near-parallel ports
	// within the clustering epsilon pairwise but spanning more than it overall once
	// produced an ordering cycle (A<B<C<A), corrupting the sort. Exact-key bucketed
	// sorting must instead yield a valid, deterministic permutation.
	a := &arranger{scale: 1, verts: newVertexTable(1e-9)}
	a.verts.canon(0, 0)
	mk := func(th, k float64) halfEdge {
		return halfEdge{tx: math.Cos(th), ty: math.Sin(th), kappa: k, exact: true}
	}
	a.halfs = []halfEdge{mk(1.5e-9, 3), mk(0.75e-9, 1), mk(0, 2)}

	run := func() []int {
		ring := []int{0, 1, 2}
		a.sortExactPorts(0, ring)
		return ring
	}
	got := run()
	seen := map[int]bool{}
	for _, h := range got {
		if seen[h] {
			t.Fatalf("sorted ring %v is not a permutation (duplicate %d)", got, h)
		}
		seen[h] = true
	}
	if len(seen) != 3 {
		t.Fatalf("sorted ring %v lost a port", got)
	}
	// Deterministic across runs and independent of input order (a valid total order).
	if again := run(); again[0] != got[0] || again[1] != got[1] || again[2] != got[2] {
		t.Fatalf("non-deterministic sort: %v then %v", got, again)
	}
}
