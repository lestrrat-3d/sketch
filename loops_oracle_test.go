package sketch_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// shape is one randomly placed loop with an analytic membership test, so the oracle
// shares no code with the arrangement.
type shape struct {
	loop     *sketch.Loop
	contains func(x, y float64) bool
}

func randomShape(t *testing.T, s *sketch.Sketch, rng *rand.Rand) shape {
	t.Helper()
	cx, cy := 2+rng.Float64()*8, 2+rng.Float64()*8
	switch rng.Intn(3) {
	case 0:
		r := 1 + rng.Float64()*3
		c := s.CreatePoint(cx, cy)
		s.Fix(c)
		circle := s.CreateCircle(c, r)
		l, err := s.DeclareLoop(circle)
		require.NoError(t, err)
		return shape{l, func(x, y float64) bool { return math.Hypot(x-cx, y-cy) < r }}
	case 1:
		hw, hh, ang := 0.8+rng.Float64()*2.5, 0.8+rng.Float64()*2.5, rng.Float64()*math.Pi
		cos, sin := math.Cos(ang), math.Sin(ang)
		var pts [4]*sketch.Point
		for i, c := range [][2]float64{{-hw, -hh}, {hw, -hh}, {hw, hh}, {-hw, hh}} {
			pts[i] = s.CreatePoint(cx+c[0]*cos-c[1]*sin, cy+c[0]*sin+c[1]*cos)
			s.Fix(pts[i])
		}
		var ents []sketch.Entity
		for i := range pts {
			ents = append(ents, s.CreateLine(pts[i], pts[(i+1)%4]))
		}
		l, err := s.DeclareLoop(ents...)
		require.NoError(t, err)
		return shape{l, func(x, y float64) bool {
			dx, dy := x-cx, y-cy
			u, v := dx*cos+dy*sin, -dx*sin+dy*cos
			return math.Abs(u) < hw && math.Abs(v) < hh
		}}
	default:
		var xs, ys [3]float64
		base := rng.Float64() * 2 * math.Pi
		for i := range xs {
			a := base + float64(i)*2*math.Pi/3 + (rng.Float64()-0.5)*0.6
			r := 1.2 + rng.Float64()*2.5
			xs[i], ys[i] = cx+r*math.Cos(a), cy+r*math.Sin(a)
		}
		var pts [3]*sketch.Point
		for i := range pts {
			pts[i] = s.CreatePoint(xs[i], ys[i])
			s.Fix(pts[i])
		}
		var ents []sketch.Entity
		for i := range pts {
			ents = append(ents, s.CreateLine(pts[i], pts[(i+1)%3]))
		}
		l, err := s.DeclareLoop(ents...)
		require.NoError(t, err)
		return shape{l, func(x, y float64) bool {
			sign := 0.0
			for i := range xs {
				j := (i + 1) % 3
				c := (xs[j]-xs[i])*(y-ys[i]) - (ys[j]-ys[i])*(x-xs[i])
				if sign == 0 {
					sign = c
				}
				if c*sign <= 0 {
					return false
				}
			}
			return true
		}}
	}
}

func inPolygon(poly [][2]float64, x, y float64) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		if (poly[i][1] > y) != (poly[j][1] > y) &&
			x < (poly[j][0]-poly[i][0])*(y-poly[i][1])/(poly[j][1]-poly[i][1])+poly[i][0] {
			in = !in
		}
	}
	return in
}

func distToPolygon(poly [][2]float64, x, y float64) float64 {
	best := math.Inf(1)
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		ax, ay, bx, by := poly[j][0], poly[j][1], poly[i][0], poly[i][1]
		dx, dy := bx-ax, by-ay
		t := 0.0
		if l := dx*dx + dy*dy; l > 0 {
			t = math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/l))
		}
		best = math.Min(best, math.Hypot(x-(ax+t*dx), y-(ay+t*dy)))
	}
	return best
}

func loopPolygon(edges []sketch.BoundaryEdge) [][2]float64 {
	var poly [][2]float64
	for _, e := range edges {
		if len(e.Polyline) > 0 {
			poly = append(poly, e.Polyline[:len(e.Polyline)-1]...)
		}
	}
	return poly
}

// interiorPoint finds a point well inside the profile's face (its outer boundary
// minus its holes), or false when no grid point is at least margin from every boundary.
func interiorPoint(p *sketch.Profile, margin float64) (float64, float64, bool) {
	outer := loopPolygon(p.Outer)
	holes := make([][][2]float64, len(p.Holes))
	for i, h := range p.Holes {
		holes[i] = loopPolygon(h)
	}
	b := profileBox(p)
	bestX, bestY, bestD := 0.0, 0.0, 0.0
	const n = 40
	for i := 1; i < n; i++ {
		for j := 1; j < n; j++ {
			x, y := b[0]+(b[2]-b[0])*float64(i)/n, b[1]+(b[3]-b[1])*float64(j)/n
			if !inPolygon(outer, x, y) {
				continue
			}
			inHole := false
			d := distToPolygon(outer, x, y)
			for _, h := range holes {
				if inPolygon(h, x, y) {
					inHole = true
				}
				d = math.Min(d, distToPolygon(h, x, y))
			}
			if !inHole && d > bestD {
				bestX, bestY, bestD = x, y, d
			}
		}
	}
	return bestX, bestY, bestD >= margin
}

// TestContainmentNeverContradictsAnAnalyticOracle: over random scenes of circles,
// rotated rectangles and triangles, every Inside or Outside verdict agrees with the
// shape's own membership test at a point well inside the region. Unproven is allowed.
func TestContainmentNeverContradictsAnAnalyticOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(20261023))
	judged, unproven := 0, 0
	for trial := 0; trial < 120; trial++ {
		s := newTestSketch(t)
		shapes := make([]shape, 2+rng.Intn(3))
		for i := range shapes {
			shapes[i] = randomShape(t, s, rng)
		}
		solved(t, s)
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
	t.Logf("verdicts judged: %d, unproven: %d", judged, unproven)
	require.Greater(t, judged, 300)
}
