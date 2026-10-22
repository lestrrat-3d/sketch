package sketch_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// splineBlob draws a closed loop of two fit splines that leave the same two points
// and bulge to opposite sides, and returns its loop with a membership test made from
// dense polylines of the splines, which shares no code with the arrangement.
func splineBlob(t *testing.T, s *sketch.Sketch, rng *rand.Rand) shape {
	t.Helper()
	cx, cy := 2+rng.Float64()*8, 2+rng.Float64()*8
	r := 1.5 + rng.Float64()*2.5
	ang := rng.Float64() * math.Pi
	cos, sin := math.Cos(ang), math.Sin(ang)
	place := func(u, v float64) *sketch.Point {
		p := s.CreatePoint(cx+u*cos-v*sin, cy+u*sin+v*cos)
		s.Fix(p)
		return p
	}
	a, b := place(-r, 0), place(r, 0)
	side := func(from, to *sketch.Point, sign float64, n int) *sketch.FitSpline {
		pts := []*sketch.Point{from}
		for i := 1; i <= n; i++ {
			f := float64(i) / float64(n+1)
			u := -r + 2*r*f
			if from == b {
				u = r - 2*r*f
			}
			v := sign * r * (0.4 + 0.5*rng.Float64()) * math.Sin(math.Pi*f)
			pts = append(pts, place(u, v))
		}
		pts = append(pts, to)
		sp, err := s.CreateFitSpline(pts...)
		require.NoError(t, err)
		return sp
	}
	upper := side(a, b, 1, 2+rng.Intn(3))
	lower := side(b, a, -1, 2+rng.Intn(3))
	l, err := s.DeclareLoop(upper, lower)
	require.NoError(t, err)
	// The oracle is built after Solve from the splines' own sampled polylines.
	var poly [][2]float64
	build := func() {
		poly = poly[:0]
		for _, sp := range []*sketch.FitSpline{upper, lower} {
			line := sp.Geometry().Polyline(300)
			poly = append(poly, line[:len(line)-1]...)
		}
	}
	build()
	return shape{l, func(x, y float64) bool { return inPolygon(poly, x, y) }}
}

// TestContainmentWithSplineLoopsNeverContradictsAnOracle mixes spline blobs with
// circles, rotated rectangles and triangles. Every Inside or Outside verdict must
// agree with the shapes' own membership tests.
func TestContainmentWithSplineLoopsNeverContradictsAnOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	judged, unproven, withBlob := 0, 0, 0
	for trial := 0; trial < 40; trial++ {
		s := newTestSketch(t)
		shapes := make([]shape, 2+rng.Intn(3))
		hasBlob := false
		for i := range shapes {
			if rng.Intn(2) == 0 {
				shapes[i] = splineBlob(t, s, rng)
				hasBlob = true
			} else {
				shapes[i] = randomShape(t, s, rng)
			}
		}
		solved(t, s)
		if hasBlob {
			withBlob++
		}
		for pi, p := range s.Profiles() {
			x, y, ok := interiorPoint(p, 0.02)
			if !ok {
				continue
			}
			for si, sh := range shapes {
				c := p.Containment(sh.loop)
				switch c.State {
				case sketch.ContainmentInside, sketch.ContainmentOutside:
					judged++
					require.Equalf(t, sh.contains(x, y), c.State == sketch.ContainmentInside,
						"trial %d profile %d shape %d at (%.3f, %.3f)", trial, pi, si, x, y)
				case sketch.ContainmentUnproven:
					unproven++
				default:
					t.Fatalf("trial %d: a declared loop read Undeclared", trial)
				}
			}
		}
	}
	t.Logf("scenes with a spline loop: %d, verdicts judged: %d, unproven: %d", withBlob, judged, unproven)
	require.Greater(t, judged, 100)
}

// TestContainmentSplineLoopNearATangentLoopEdge puts a rectangle whose bottom side is
// at the top of a fit-spline arch, from a clear gap down to a deep overlap. Wherever a
// verdict is given it must match the oracle. Near the touching position the sampled
// map cannot rule out a hidden crossing, so those verdicts are Unproven.
func TestContainmentSplineLoopNearATangentLoopEdge(t *testing.T) {
	probe := func(gap float64) (judged, unproven int) {
		s := newTestSketch(t)
		pt := func(x, y float64) *sketch.Point {
			p := s.CreatePoint(x, y)
			s.Fix(p)
			return p
		}
		a, apex, b := pt(-4, 0), pt(0, 3), pt(4, 0)
		arch, err := s.CreateFitSpline(a, apex, b)
		require.NoError(t, err)
		chord := s.CreateLine(b, a)
		under, err := s.DeclareLoop(arch, chord)
		require.NoError(t, err)
		// The arch's top is at the apex by symmetry; find it from the polyline.
		top := math.Inf(-1)
		for _, p := range arch.Geometry().Polyline(4000) {
			top = math.Max(top, p[1])
		}
		y0 := top + gap
		r0, r1, r2, r3 := pt(-1, y0), pt(1, y0), pt(1, y0+2), pt(-1, y0+2)
		box, err := s.DeclareLoop(s.CreateLine(r0, r1), s.CreateLine(r1, r2), s.CreateLine(r2, r3), s.CreateLine(r3, r0))
		require.NoError(t, err)
		solved(t, s)
		poly := arch.Geometry().Polyline(4000)
		inUnder := func(x, y float64) bool { return inPolygon(poly, x, y) }
		inBox := func(x, y float64) bool { return x > -1 && x < 1 && y > y0 && y < y0+2 }
		for _, p := range s.Profiles() {
			x, y, ok := interiorPoint(p, 1e-4)
			for _, tc := range []struct {
				l    *sketch.Loop
				want bool
			}{{under, inUnder(x, y)}, {box, inBox(x, y)}} {
				c := p.Containment(tc.l)
				if c.State == sketch.ContainmentUnproven {
					unproven++
					continue
				}
				if !ok {
					continue
				}
				judged++
				require.Equalf(t, tc.want, c.State == sketch.ContainmentInside, "gap %v at (%.4f, %.4f)", gap, x, y)
			}
		}
		return judged, unproven
	}
	for _, gap := range []float64{0.5, 1e-3, 1e-6, 1e-9, 0, -1e-9, -1e-6, -1e-3, -0.5} {
		j, u := probe(gap)
		t.Logf("gap %g: judged %d unproven %d", gap, j, u)
	}
	// A clear gap is certified. An overlap crosses the spline with the box's sides,
	// which the pair ledger does not prove (that is the crossing stage), so it is
	// Unproven and never a guess.
	j, _ := probe(0.5)
	require.Positive(t, j)
	_, u := probe(-0.5)
	require.Positive(t, u)
}

// TestContainmentArcsOverlappingInTwoWindows draws two arcs of one circle that share
// two separate spans. The arrangement refuses that overlap, so no profile may read
// Inside or Outside for a loop that uses either arc.
func TestContainmentArcsOverlappingInTwoWindows(t *testing.T) {
	s := newTestSketch(t)
	c := s.CreatePoint(0, 0)
	s.Fix(c)
	const r = 5.0
	at := func(deg float64) *sketch.Point {
		p := s.CreatePoint(r*math.Cos(deg*math.Pi/180), r*math.Sin(deg*math.Pi/180))
		s.Fix(p)
		return p
	}
	// Arc A runs 10 to 300 degrees, arc B runs 250 to 70 (through 0): they share
	// 250..300 and 10..70.
	a0, a1 := at(10), at(300)
	b0, b1 := at(250), at(70)
	arcA := s.CreateArc(c, a0, a1)
	arcB := s.CreateArc(c, b0, b1)
	chordA := s.CreateLine(a1, a0)
	chordB := s.CreateLine(b1, b0)
	la, err := s.DeclareLoop(arcA, chordA)
	require.NoError(t, err)
	lb, err := s.DeclareLoop(arcB, chordB)
	require.NoError(t, err)
	other := rectLoop(s, 20, 20, 22, 22)
	lo, err := s.DeclareLoop(other...)
	require.NoError(t, err)
	solved(t, s)
	for _, p := range s.Profiles() {
		b := profileBox(p)
		if b[0] >= 19 {
			requireVerdict(t, p, lo, sketch.ContainmentInside, "a rectangle far from the refused overlap")
			continue
		}
		for _, l := range []*sketch.Loop{la, lb} {
			require.Equalf(t, sketch.ContainmentUnproven, p.Containment(l).State,
				"a profile at %v against a loop on a refused overlap", b)
		}
	}
}
