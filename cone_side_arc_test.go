package sketch_test

import (
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// coneSectionSketch (cone_line_test.go) puts the cone section on the unit
// circle about the sketch origin: its implicit function is x*x + y*y - 1.

// polarArc adds a counter-clockwise arc about (cx, cy) with radius r from
// angle a0 to a1, given in degrees.
func polarArc(s *sketch.Sketch, cx, cy, r, a0, a1 float64) *sketch.Arc {
	pt := func(deg float64) *sketch.Point {
		rad := deg * math.Pi / 180
		return s.CreatePoint(cx+r*math.Cos(rad), cy+r*math.Sin(rad))
	}
	return s.CreateArc(s.CreatePoint(cx, cy), pt(a0), pt(a1))
}

// TestCertifyConeCurveSideGeneralArc covers arcs the single-quadrant class
// refuses: any position, any orientation, any sweep short of a half turn.
func TestCertifyConeCurveSideGeneralArc(t *testing.T) {
	for _, tc := range []struct {
		name              string
		cx, cy, r, a0, a1 float64
		want              sketch.ConeSide
	}{
		// A fillet-style arc off the origin, sweeping up and to the right.
		{"fillet outside", 2, 2, 0.5, 20, 110, sketch.ConeOutside},
		{"fillet outside left", -2, 1, 0.3, 200, 290, sketch.ConeOutside},
		{"inside centred", 0, 0, 0.5, 200, 300, sketch.ConeInside},
		{"inside off centre", 0.1, -0.1, 0.4, 10, 100, sketch.ConeInside},
		// A shallow root-style arc a hair inside the section: the whole
		// sector's box pokes out of it, so the proof must split the wedge.
		{"shallow root arc", 0, 0, 0.97, 70, 110, sketch.ConeInside},
		{"shallow tip arc", 0, 0, 1.03, 70, 110, sketch.ConeOutside},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, cone := coneSectionSketch(t)
			arc := polarArc(s, tc.cx, tc.cy, tc.r, tc.a0, tc.a1)
			cert, err := s.CertifyConeCurveSide(arc, cone)
			require.NoError(t, err)
			require.Equal(t, tc.want, cert.Side())
			require.Positive(t, cert.ImplicitMargin().Sign())
			require.Same(t, arc, cert.Entity())
			arc.End.MoveTo(arc.End.X(), arc.End.Y()+1e-3)
			require.True(t, cert.IsStale())
		})
	}
}

// TestCertifyConeCurveSideGeneralArcRefusals lists what must stay refused.
func TestCertifyConeCurveSideGeneralArcRefusals(t *testing.T) {
	for _, tc := range []struct {
		name              string
		cx, cy, r, a0, a1 float64
	}{
		{"crosses the section", 1, 0, 0.5, 30, 150},
		{"touches the section", 2, 0, 1, 120, 240},
		{"touches from inside", 0.5, 0, 0.5, -60, 60},
		{"just over a half turn", 3, 0, 0.5, 0, 181},
		{"more than a half turn", 3, 0, 0.5, 0, 270},
		{"clockwise reading is a long sweep", 3, 0, 0.5, 90, 0},
		{"zero sweep", 3, 0, 0.5, 40, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, cone := coneSectionSketch(t)
			arc := polarArc(s, tc.cx, tc.cy, tc.r, tc.a0, tc.a1)
			_, err := s.CertifyConeCurveSide(arc, cone)
			require.ErrorIs(t, err, sketch.ErrConeCurveSide)
		})
	}

	t.Run("exact half turn", func(t *testing.T) {
		s, cone := coneSectionSketch(t)
		arc := s.CreateArc(s.CreatePoint(3, 0), s.CreatePoint(3.5, 0), s.CreatePoint(2.5, 0))
		_, err := s.CertifyConeCurveSide(arc, cone)
		require.ErrorIs(t, err, sketch.ErrConeCurveSide)
	})

	t.Run("far side of the apex", func(t *testing.T) {
		// The cone's negative half (below the apex) is never certified.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		cone := sketch.ConeSection{Apex: r3.NewVec(0, 0, 1), Axis: r3.NewVec(0, 0, 1), Slope: 1}
		arc := polarArc(s, 3, 3, 0.5, 20, 110)
		_, err = s.CertifyConeCurveSide(arc, cone)
		require.ErrorIs(t, err, sketch.ErrConeCurveSide)
	})
}

// TestCertifyConeCurveSideArcHugsSection checks that a proof which cannot
// settle within its budget refuses instead of guessing: the arc hugs the
// section so closely that no box within the depth limit clears it.
func TestCertifyConeCurveSideArcHugsSection(t *testing.T) {
	s, cone := coneSectionSketch(t)
	arc := polarArc(s, 0, 0, 1+1e-13, 70, 110)
	_, err := s.CertifyConeCurveSide(arc, cone)
	require.ErrorIs(t, err, sketch.ErrConeCurveSide)
}

// TestCertifyConeCurveSideArcEndRadiusIgnored: the carrier takes its radius from
// Start; End only gives the final direction, as Arc.CertifiedSpanBounds does.
func TestCertifyConeCurveSideArcEndRadiusIgnored(t *testing.T) {
	s, cone := coneSectionSketch(t)
	center := s.CreatePoint(2, 2)
	start := s.CreatePoint(2+0.5*math.Cos(0.3), 2+0.5*math.Sin(0.3))
	// End sits well off the circle, in the direction of angle 1.4.
	end := s.CreatePoint(2+0.9*math.Cos(1.4), 2+0.9*math.Sin(1.4))
	arc := s.CreateArc(center, start, end)
	cert, err := s.CertifyConeCurveSide(arc, cone)
	require.NoError(t, err)
	require.Equal(t, sketch.ConeOutside, cert.Side())
}

// TestCertifyConeCurveSideArcSoundness certifies random arcs and checks every
// claim against dense samples of the exact carrier: a certified side must hold
// at each sample with |F| at least the reported margin.
func TestCertifyConeCurveSideArcSoundness(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var outside, inside, refused int
	for range 600 {
		s, cone := coneSectionSketch(t)
		cx, cy := rng.Float64()*4-2, rng.Float64()*4-2
		r := 0.05 + rng.Float64()*1.5
		a0 := rng.Float64() * 2 * math.Pi
		sweep := 0.02 + rng.Float64()*3.0 // up to just short of a half turn
		start := s.CreatePoint(cx+r*math.Cos(a0), cy+r*math.Sin(a0))
		end := s.CreatePoint(cx+r*math.Cos(a0+sweep), cy+r*math.Sin(a0+sweep))
		arc := s.CreateArc(s.CreatePoint(cx, cy), start, end)
		cert, err := s.CertifyConeCurveSide(arc, cone)
		if err != nil {
			refused++
			continue
		}
		margin, _ := cert.ImplicitMargin().Float64()
		rr := arc.R()
		ang := arc.StartAngle()
		sw := arc.Sweep()
		for i := 0; i <= 400; i++ {
			a := ang + sw*float64(i)/400
			x, y := cx+rr*math.Cos(a), cy+rr*math.Sin(a)
			f := x*x + y*y - 1
			if cert.Side() == sketch.ConeOutside {
				require.GreaterOrEqual(t, f, margin*(1-1e-9), "arc %d sample %d", i, i)
			} else {
				require.LessOrEqual(t, f, -margin*(1-1e-9), "arc %d sample %d", i, i)
			}
		}
		if cert.Side() == sketch.ConeOutside {
			outside++
		} else {
			inside++
		}
	}
	require.Positive(t, outside)
	require.Positive(t, inside)
	require.Positive(t, refused)
}

// TestCertifyConeCurveSideToothFillet mirrors the bevel tooth sketch: a fillet
// arc, tangent to a radial flank and lying just above the root circle, built
// with constraints and solved at the default tolerance.
func TestCertifyConeCurveSideToothFillet(t *testing.T) {
	s, cone := coneSectionSketch(t)
	const rho, rd = 0.3, 1.0
	h := rho * (1 - math.Cos(10*math.Pi/180)) / 2
	cd := rd - h + rho // centre distance from the apex
	psi := math.Pi - math.Asin(rho/cd)
	lenT1 := math.Sqrt(cd*cd - rho*rho)
	c := s.CreatePoint(-cd, 0)
	t1 := s.CreatePoint(lenT1*math.Cos(psi), lenT1*math.Sin(psi))
	t3 := s.CreatePoint(-cd+rho*math.Cos(10*math.Pi/180), rho*math.Sin(10*math.Pi/180))
	arc := s.CreateArc(c, t3, t1)
	flank := s.CreateLine(s.Origin(), t1)
	s.AddConstraint(sketch.NewTangent(flank, arc))
	s.AddConstraint(sketch.NewRadius(arc, rho))
	s.AddConstraint(sketch.NewDistance(s.Origin(), c, cd))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	cert, err := s.CertifyConeCurveSide(arc, cone)
	require.NoError(t, err)
	require.Equal(t, sketch.ConeOutside, cert.Side())
	require.Positive(t, cert.ImplicitMargin().Sign())
}

// TestCertifyConeCurveSideQuadrantArcMargin checks the margin of a short arc
// the single-box quadrant proof certifies against the exact arc. Arc.R()
// rounds hypot(-6, 1) below the exact radius sqrt(37), so a box built from
// Arc.R() misses the exact arc point (-sqrt(37), 0), where F is smallest.
func TestCertifyConeCurveSideQuadrantArcMargin(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(-6, 1), s.CreatePoint(-6, -1))
	cone := sketch.ConeSection{Apex: r3.NewVec(10, 0, 5), Axis: r3.NewVec(-1, 0, 0), Slope: 0.25}
	cert, err := s.CertifyConeCurveSide(arc, cone)
	require.NoError(t, err)
	require.Equal(t, sketch.ConeOutside, cert.Side())
	margin := cert.ImplicitMargin()
	require.Positive(t, margin.Sign())

	// At (x, 0) on the plane, F = 25 - slope^2 (10 - x)^2. At x = -sqrt(37),
	// margin <= F is (10 + sqrt(37))^2 <= T with T = (25 - margin)/slope^2,
	// that is 20 sqrt(37) <= T - 137, checked exactly by squaring.
	slope2 := big.NewRat(1, 16)
	T := new(big.Rat).Quo(new(big.Rat).Sub(big.NewRat(25, 1), margin), slope2)
	d := new(big.Rat).Sub(T, big.NewRat(137, 1))
	require.Positive(t, d.Sign())
	require.LessOrEqual(t, big.NewRat(400*37, 1).Cmp(new(big.Rat).Mul(d, d)), 0,
		"margin exceeds F at the exact arc point (-sqrt(37), 0)")
}
