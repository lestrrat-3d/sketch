package sketch

import (
	"math"
	"math/rand"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/stretchr/testify/require"
)

// coneFloatFixture returns a plane sketch, its frame and a plane-origin cone.
func coneFloatFixture(t *testing.T) (*Sketch, r3.Frame) {
	t.Helper()
	w := NewWorld()
	f, err := r3.NewFrame(r3.NewVec(0, 0, 1), r3.NewVec(1, 0, 0), r3.NewVec(0, 1, 0))
	require.NoError(t, err)
	plane, err := w.CreatePlaneFromFrame(f)
	require.NoError(t, err)
	s, err := w.CreateSketch(plane)
	require.NoError(t, err)
	return s, f
}

// randomCone returns a cone about the z axis with a random apex height and slope.
func randomCone(rng *rand.Rand) ConeSection {
	slope := 1.0
	if rng.Intn(3) == 0 {
		slope = 0.2 + rng.Float64()*3
	}
	return ConeSection{Apex: r3.NewVec(0, 0, rng.Float64()*0.2), Axis: r3.NewVec(0, 0, 1), Slope: slope}
}

// TestConeFloatPrefilterArcDifferential checks the wedge proof returns the
// same side and margin with the float prefilter on and off, over random arcs,
// including arcs whose radius is within 1e-12 of the cone circle.
func TestConeFloatPrefilterArcDifferential(t *testing.T) {
	s, frame := coneFloatFixture(t)
	rng := rand.New(rand.NewSource(7))
	certified := 0
	for range 1500 {
		cx, cy := rng.Float64()*2-1, rng.Float64()*2-1
		r := 0.05 + rng.Float64()*1.5
		a0 := rng.Float64() * 2 * math.Pi
		sweep := 0.01 + rng.Float64()*(math.Pi-0.02)
		if rng.Intn(4) == 0 {
			r = 1 + (rng.Float64()-0.5)*math.Pow(10, -float64(rng.Intn(12)))
			cx, cy = 0, 0
		}
		arc := s.CreateArc(s.CreatePoint(cx, cy), s.CreatePoint(cx+r*math.Cos(a0), cy+r*math.Sin(a0)),
			s.CreatePoint(cx+r*math.Cos(a0+sweep), cy+r*math.Sin(a0+sweep)))
		form := newConeSideForm(frame, randomCone(rng))
		s0, m0 := form.arcWedgeSide(arc)
		form.fp = newConeFloatForm(form)
		require.NotNil(t, form.fp)
		s1, m1 := form.arcWedgeSide(arc)
		require.Equal(t, s0, s1)
		if s0 != 0 {
			certified++
			require.Zero(t, m0.Cmp(m1))
		}
	}
	require.Greater(t, certified, 500)
}

// TestConeFloatPrefilterBoxDifferential checks that whenever boxZero holds the
// exact wedge box returns no side, and reports how many exact no-side boxes the
// prefilter decides.
func TestConeFloatPrefilterBoxDifferential(t *testing.T) {
	_, frame := coneFloatFixture(t)
	rng := rand.New(rand.NewSource(3))
	exactZero, skipped := 0, 0
	for range 4000 {
		form := newConeSideForm(frame, randomCone(rng))
		form.fp = newConeFloatForm(form)
		r := 0.05 + rng.Float64()*1.5
		if rng.Intn(4) == 0 {
			r = 1 + (rng.Float64()-0.5)*math.Pow(10, -float64(rng.Intn(12)))
		}
		cx, cy := rng.Float64()-0.5, rng.Float64()-0.5
		a0 := rng.Float64() * 2 * math.Pi
		sweep := 0.001 + rng.Float64()*(math.Pi-0.002)
		a := coneRay{coneRat(math.Cos(a0)), coneRat(math.Sin(a0))}
		b := coneRay{coneRat(math.Cos(a0 + sweep)), coneRat(math.Sin(a0 + sweep))}
		ua, ok := coneUnit(a)
		require.True(t, ok)
		ub, ok := coneUnit(b)
		require.True(t, ok)
		rLo, rHi := coneRat(r), coneRat(r*(1+1e-9))
		w := &coneArcWedge{form: form, cx: coneRat(cx), cy: coneRat(cy),
			radius: coneInterval{rLo, rHi}, budget: coneArcBudget}
		w.fl = form.newConeArcFloat(rLo, rHi, cx, cy)
		require.NotNil(t, w.fl)
		side, _ := w.box(a, b, ua, ub)
		if side != 0 {
			require.False(t, w.boxZero(a, b, ua, ub))
			continue
		}
		exactZero++
		if w.boxZero(a, b, ua, ub) {
			skipped++
		}
	}
	t.Logf("exact no-side boxes=%d decided by prefilter=%d", exactZero, skipped)
	require.Greater(t, skipped, exactZero/2)
}

// TestConeFloatPrefilterFitDifferential compares the fitted-spline proof with
// the prefilter on and off over random splines.
func TestConeFloatPrefilterFitDifferential(t *testing.T) {
	s, frame := coneFloatFixture(t)
	rng := rand.New(rand.NewSource(11))
	cone := ConeSection{Axis: r3.NewVec(0, 0, 1), Slope: 1}
	certified := 0
	for range 300 {
		pts := make([]*Point, 3+rng.Intn(3))
		for i := range pts {
			pts[i] = s.CreatePoint(rng.Float64()*1.6-0.8, rng.Float64()*1.6-0.8)
		}
		fit, err := s.CreateFitSpline(pts...)
		if err != nil {
			continue
		}
		fi, err := fit.Interpolant()
		require.NoError(t, err)
		form := newConeSideForm(frame, cone)
		s0, m0 := form.fitSide(fi)
		form.fp = newConeFloatForm(form)
		s1, m1 := form.fitSide(fi)
		require.Equal(t, s0, s1)
		if s0 != 0 {
			certified++
			require.Zero(t, m0.Cmp(m1))
		}
	}
	require.Greater(t, certified, 100)
}

// TestConeFloatPrefilterNonFinite checks a non-finite form disables the prefilter.
func TestConeFloatPrefilterNonFinite(t *testing.T) {
	_, frame := coneFloatFixture(t)
	form := newConeSideForm(frame, ConeSection{Axis: r3.NewVec(0, 0, 1), Slope: 1e200})
	require.Nil(t, newConeFloatForm(form))
	require.False(t, coneSideForm{}.floatSkip(coneInterval{coneRat(0), coneRat(1)}, coneInterval{coneRat(0), coneRat(1)}))
}
