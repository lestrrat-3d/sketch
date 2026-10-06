package sketch_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// The fixtures below are decad's reference mechanisms (crank-rocker,
// non-Grashof four-bar, slider-crank). Every expected value is the closed form
// evaluated in float64, so containment is asserted with a slack of 1e-9 — far
// above the closed form's own rounding, far below anything the other branch or
// a wrong pose would produce.
const encloseSlack = 1e-9

// tableSlack is the rounding of decad's tables: a 4-decimal degree value is off
// by up to 5e-5°, and a 6-decimal millimetre value by up to 5e-7.
var (
	degTableSlack = rad(0.00005) + encloseSlack
	mmTableSlack  = 5e-7 + encloseSlack
)

func rad(deg float64) float64 { return deg * math.Pi / 180 }

// fourBar is a four-bar linkage scene: ground O2→O4 along +x, crank O2→A
// driven by a signed angle from the ground, coupler A→B, follower O4→B whose
// angle from the ground is a driven dimension.
type fourBar struct {
	s                   *sketch.Sketch
	o2, o4, a, b        *sketch.Point
	crank, follower     *sketch.Angle
	g, r, l, f, theta2d float64
}

// newFourBar builds the scene seeded at crank angle theta2d (degrees) with B
// seeded at (bx, by), and solves it, so the driven follower angle reads the
// pose the seed selects.
func newFourBar(t *testing.T, g, r, l, f, theta2d, bx, by float64) *fourBar {
	t.Helper()
	s := newSketch(t)
	th := rad(theta2d)
	o2 := s.CreatePoint(0, 0)
	o4 := s.CreatePoint(g, 0)
	a := s.CreatePoint(r*math.Cos(th), r*math.Sin(th))
	b := s.CreatePoint(bx, by)
	ground := s.CreateLine(o2, o4)
	crankLine := s.CreateLine(o2, a)
	s.CreateLine(a, b)
	followerLine := s.CreateLine(o4, b)
	crank := sketch.NewAngle(ground, crankLine, theta2d)
	follower := sketch.NewAngle(ground, followerLine, 0)
	follower.SetDriven(true)
	s.AddConstraint(
		sketch.NewCoincident(o2, s.Origin()),
		sketch.NewHorizontal(ground),
		sketch.NewDistance(o2, o4, g),
		sketch.NewDistance(o2, a, r),
		sketch.NewDistance(a, b, l),
		sketch.NewDistance(o4, b, f),
		crank,
		follower,
	)
	res, err := s.Solve(t.Context())
	require.NoError(t, err, "the seeded four-bar must solve")
	require.Equal(t, 0, res.DOF, "the four-bar is fully constrained with the crank held")
	return &fourBar{s: s, o2: o2, o4: o4, a: a, b: b, crank: crank, follower: follower, g: g, r: r, l: l, f: f, theta2d: theta2d}
}

// pose is the closed-form pose at crank angle th (radians): the follower angle
// and the coupler pin B, on the branch with B above the ground line (above)
// or below it.
func (fb *fourBar) pose(th float64, above bool) (float64, float64, float64) {
	d := math.Sqrt(fb.g*fb.g + fb.r*fb.r - 2*fb.g*fb.r*math.Cos(th))
	phi := math.Atan2(fb.r*math.Sin(th), fb.r*math.Cos(th)-fb.g)
	beta := math.Acos((fb.f*fb.f + d*d - fb.l*fb.l) / (2 * fb.f * d))
	t4 := phi - beta
	if !above {
		t4 = phi + beta
	}
	return t4, fb.g + fb.f*math.Cos(t4), fb.f * math.Sin(t4)
}

// requireContains asserts v lies in iv up to the closed form's slack.
func requireContains(t *testing.T, iv sketch.Interval, v float64, msg string) {
	t.Helper()
	requireContainsWithin(t, iv, v, encloseSlack, msg)
}

func requireContainsWithin(t *testing.T, iv sketch.Interval, v, slack float64, msg string) {
	t.Helper()
	require.LessOrEqual(t, iv.Lo, v+slack, msg)
	require.GreaterOrEqual(t, iv.Hi, v-slack, msg)
}

// nearTurn shifts an angle by whole turns to the representative nearest ref.
func nearTurn(v, ref float64) float64 {
	return v - 2*math.Pi*math.Round((v-ref)/(2*math.Pi))
}

// requireAngleContains asserts the angle v (mod 2π) lies in iv.
func requireAngleContains(t *testing.T, iv sketch.Interval, v float64, msg string) {
	t.Helper()
	requireContains(t, iv, nearTurn(v, (iv.Lo+iv.Hi)/2), msg)
}

// requireTableAngle asserts a table angle (degrees, 4 decimals) lies in iv
// (mod 2π) up to the table's rounding.
func requireTableAngle(t *testing.T, iv sketch.Interval, deg float64, msg string) {
	t.Helper()
	requireContainsWithin(t, iv, nearTurn(rad(deg), (iv.Lo+iv.Hi)/2), degTableSlack, msg)
}

// requireAngleExcludes asserts no representative of v (mod 2π) lies in iv.
func requireAngleExcludes(t *testing.T, iv sketch.Interval, v float64, msg string) {
	t.Helper()
	w := nearTurn(v, (iv.Lo+iv.Hi)/2)
	require.True(t, w < iv.Lo-encloseSlack || w > iv.Hi+encloseSlack, msg)
}

// requirePieceHolds asserts that the piece covering crank angle th holds the
// closed-form pins A and B of the stated branch.
func requirePieceHolds(t *testing.T, fb *fourBar, e *sketch.Enclosure, th float64, above bool) {
	t.Helper()
	_, bx, by := fb.pose(th, above)
	for _, pc := range e.Pieces() {
		if !pc.Range().Contains(th) {
			continue
		}
		ax, ay, ok := pc.PointBox(fb.a)
		require.True(t, ok, "A is a point of the sketch")
		requireContains(t, ax, fb.r*math.Cos(th), "A.x")
		requireContains(t, ay, fb.r*math.Sin(th), "A.y")
		x, y, ok := pc.PointBox(fb.b)
		require.True(t, ok, "B is a point of the sketch")
		requireContains(t, x, bx, "B.x")
		requireContains(t, y, by, "B.y")
		return
	}
	require.Failf(t, "no piece covers the crank angle", "%v rad", th)
}

// requireContiguous asserts the pieces cover the range in order with no gap.
func requireContiguous(t *testing.T, e *sketch.Enclosure) {
	t.Helper()
	pcs := e.Pieces()
	require.NotEmpty(t, pcs, "an enclosure has at least one piece")
	require.Equal(t, e.Range().Lo, pcs[0].Range().Lo, "the first piece starts the range")
	require.Equal(t, e.Range().Hi, pcs[len(pcs)-1].Range().Hi, "the last piece ends the range")
	for i := 1; i < len(pcs); i++ {
		require.Equal(t, pcs[i-1].Range().Hi, pcs[i].Range().Lo, "adjacent pieces share their endpoint")
	}
}

// crankRocker is decad's crank-rocker (g=100, r=30, l=80, f=70), seeded at
// theta2d with B on the requested side of the ground line.
func crankRocker(t *testing.T, theta2d float64, above bool) *fourBar {
	t.Helper()
	ref := &fourBar{g: 100, r: 30, l: 80, f: 70}
	_, bx, by := ref.pose(rad(theta2d), above)
	return newFourBar(t, 100, 30, 80, 70, theta2d, bx+0.5, by+0.5)
}

func TestEncloseCrankRockerPoint(t *testing.T) {
	t.Run("E1 B above the ground line", func(t *testing.T) {
		fb := crankRocker(t, 90, true)
		before := fb.s.Revision()
		bx0, by0 := fb.b.X(), fb.b.Y()

		e, err := fb.s.Enclose(t.Context(), fb.crank, math.Pi/2, math.Pi/2)
		require.NoError(t, err, "the crank-rocker at 90° is certifiable")
		require.Len(t, e.Pieces(), 1, "a point ask is one piece")
		require.False(t, e.IsStale(), "a fresh enclosure is current")
		require.Equal(t, before, fb.s.Revision(), "Enclose leaves the sketch untouched")
		require.Equal(t, bx0, fb.b.X(), "Enclose restores every coordinate bit for bit")
		require.Equal(t, by0, fb.b.Y(), "Enclose restores every coordinate bit for bit")

		requirePieceHolds(t, fb, e, math.Pi/2, true)
		bx, by, ok := e.PointBox(fb.b)
		require.True(t, ok, "B has a box")
		require.Less(t, bx.Hi-bx.Lo, 1e-9, "a point box is tight")
		require.Less(t, by.Hi-by.Lo, 1e-9, "a point box is tight")
		// The boxes enclose the EXACT solution; Solve's float coordinates satisfy
		// the residual only to its 1e-10 tolerance, so they may sit just outside
		// a box this tight, but never farther than the tolerance allows.
		requireContainsWithin(t, bx, fb.b.X(), 1e-8, "the float solve is within tolerance of the box")
		requireContainsWithin(t, by, fb.b.Y(), 1e-8, "the float solve is within tolerance of the box")

		ox, oy, ok := e.PointBox(fb.o2)
		require.True(t, ok, "O2 has a box")
		requireContains(t, ox, 0, "O2 sits on the origin")
		requireContains(t, oy, 0, "O2 sits on the origin")

		fol, ok := e.Driven(fb.follower)
		require.True(t, ok, "the driven follower has an interval")
		requireTableAngle(t, fol, 113.3250, "follower at 113.3250° (4-decimal table value)")
		t4, _, _ := fb.pose(math.Pi/2, true)
		requireAngleContains(t, fol, t4, "follower at the closed form")
		requireAngleExcludes(t, fol, rad(213.2765), "the other branch is outside the box")

		_, ok = e.Driven(fb.crank)
		require.False(t, ok, "the driver is not a driven dimension")
	})
	t.Run("E2 B below the ground line", func(t *testing.T) {
		fb := crankRocker(t, 90, false)
		e, err := fb.s.Enclose(t.Context(), fb.crank, math.Pi/2, math.Pi/2)
		require.NoError(t, err, "the lower branch is certifiable too")
		requirePieceHolds(t, fb, e, math.Pi/2, false)
		fol, ok := e.Driven(fb.follower)
		require.True(t, ok, "the driven follower has an interval")
		t4, _, _ := fb.pose(math.Pi/2, false)
		requireAngleContains(t, fol, t4, "follower at the lower closed form")
		requireTableAngle(t, fol, 213.2765, "follower at 213.2765°")
		requireAngleExcludes(t, fol, rad(113.3250), "the upper branch is outside the box")
	})
}

// sampledRange returns the closed-form follower angle's min and max over
// [lo, hi] on the upper branch.
func sampledRange(fb *fourBar, lo, hi float64) (float64, float64) {
	mn, mx := math.Inf(1), math.Inf(-1)
	prev, _, _ := fb.pose(lo, true)
	const n = 20000
	for i := 0; i <= n; i++ {
		t4, _, _ := fb.pose(lo+(hi-lo)*float64(i)/n, true)
		t4 = nearTurn(t4, prev) // follow the angle continuously across atan2's cut
		prev = t4
		mn, mx = min(mn, t4), max(mx, t4)
	}
	return mn, mx
}

func TestEncloseCrankRockerRange(t *testing.T) {
	t.Run("E3 a quarter turn", func(t *testing.T) {
		fb := crankRocker(t, 0, true)
		e, err := fb.s.Enclose(t.Context(), fb.crank, 0, math.Pi/2)
		require.NoError(t, err, "the quarter turn is certifiable")
		requireContiguous(t, e)
		fol, ok := e.Driven(fb.follower)
		require.True(t, ok, "the driven follower has an interval")
		requireTableAngle(t, fol, 101.5370, "the follower's minimum inside the range")
		requireTableAngle(t, fol, 113.3250, "the follower at 90°")
		mn, mx := sampledRange(fb, 0, math.Pi/2)
		requireContains(t, fol, mn, "sampled minimum")
		requireContains(t, fol, mx, "sampled maximum")
		require.Less(t, fol.Hi-fol.Lo, rad(13), "the hull stays near the true 11.8° swing")
		for _, deg := range []float64{0, 15, 38.5727, 60, 90} {
			requirePieceHolds(t, fb, e, rad(deg), true)
		}
	})
	t.Run("E4 a full turn", func(t *testing.T) {
		fb := crankRocker(t, 0, true)
		e, err := fb.s.Enclose(t.Context(), fb.crank, 0, 2*math.Pi)
		require.NoError(t, err, "a crank-rocker's full turn is certifiable")
		requireContiguous(t, e)
		fol, ok := e.Driven(fb.follower)
		require.True(t, ok, "the driven follower has an interval")
		requireTableAngle(t, fol, 101.5370, "the follower's minimum")
		requireTableAngle(t, fol, 152.3396, "the follower's maximum")
		mn, mx := sampledRange(fb, 0, 2*math.Pi)
		requireContains(t, fol, mn, "sampled minimum")
		requireContains(t, fol, mx, "sampled maximum")
		require.Less(t, fol.Hi-fol.Lo, rad(52), "the hull stays near the true 50.80° swing")
		for deg := 0.0; deg <= 360; deg += 7.5 {
			requirePieceHolds(t, fb, e, rad(deg), true)
		}
	})
}

// nonGrashof is decad's folding four-bar (g=100, r=50, l=60, f=50), seeded at
// 80° with B above the ground line (θ4 = 130.3270°).
func nonGrashof(t *testing.T) *fourBar {
	t.Helper()
	t4 := rad(130.3270)
	return newFourBar(t, 100, 50, 60, 50, 80, 100+50*math.Cos(t4), 50*math.Sin(t4))
}

func TestEncloseNonGrashofRefuses(t *testing.T) {
	fold := math.Acos(0.04) // the crank angle where the branches meet
	t.Run("E5 at the table's fold angle", func(t *testing.T) {
		fb := nonGrashof(t)
		e, err := fb.s.Enclose(t.Context(), fb.crank, rad(87.7076), rad(87.7076))
		require.Error(t, err, "87.7076° is past the fold by 4e-5°: no configuration exists")
		require.ErrorIs(t, err, sketch.ErrNotConverged)
		require.Nil(t, e, "a refusal carries no enclosure")
	})
	t.Run("E5 at the fold itself", func(t *testing.T) {
		fb := nonGrashof(t)
		e, err := fb.s.Enclose(t.Context(), fb.crank, fold, fold)
		require.Error(t, err, "the branches meet at the fold")
		require.True(t, isRefusal(err), "refused as unconverged or uncertified: %v", err)
		require.Nil(t, e, "a refusal carries no enclosure")
	})
	t.Run("E5 over a range holding the fold", func(t *testing.T) {
		fb := nonGrashof(t)
		e, err := fb.s.Enclose(t.Context(), fb.crank, rad(80), rad(90))
		require.ErrorIs(t, err, sketch.ErrNotCertified, "the range holds the fold")
		require.Nil(t, e, "a refusal carries no enclosure")
	})
	t.Run("E5 control just short of the fold", func(t *testing.T) {
		fb := nonGrashof(t)
		e, err := fb.s.Enclose(t.Context(), fb.crank, rad(80), rad(87))
		require.NoError(t, err, "the range stops short of the fold")
		fol, ok := e.Driven(fb.follower)
		require.True(t, ok, "the driven follower has an interval")
		requireTableAngle(t, fol, 130.3270, "the follower at 80°")
		requireTableAngle(t, fol, 146.5043, "the follower at 87°")
		requireAngleExcludes(t, fol, rad(173.0040), "the other branch at 80°")
	})
	t.Run("E6 past the fold", func(t *testing.T) {
		fb := nonGrashof(t)
		before := fb.s.Revision()
		e, err := fb.s.Enclose(t.Context(), fb.crank, math.Pi/2, math.Pi/2)
		require.ErrorIs(t, err, sketch.ErrNotConverged, "no real configuration exists at 90°")
		require.Nil(t, e, "a refusal carries no enclosure")
		require.Equal(t, before, fb.s.Revision(), "a refused call also leaves the sketch untouched")
	})
}

func isRefusal(err error) bool {
	return errors.Is(err, sketch.ErrNotConverged) || errors.Is(err, sketch.ErrNotCertified)
}

// sliderCrank is decad's slider-crank: crank O→A of length r driven by a
// signed angle from the ground line O→G, rod A→P of length l, slider P on the
// ground's height, and the slider's signed x as a driven dimension.
type sliderCrank struct {
	s     *sketch.Sketch
	crank *sketch.Angle
	x     *sketch.HorizontalDistance
}

func newSliderCrank(t *testing.T, r, l, thetaDeg float64) *sliderCrank {
	t.Helper()
	s := newSketch(t)
	th := rad(thetaDeg)
	o := s.CreatePoint(0, 0)
	g := s.CreatePoint(100, 0)
	a := s.CreatePoint(r*math.Cos(th), r*math.Sin(th))
	p := s.CreatePoint(r*math.Cos(th)+math.Sqrt(l*l-r*r*math.Sin(th)*math.Sin(th)), 0)
	ground := s.CreateLine(o, g)
	crankLine := s.CreateLine(o, a)
	s.CreateLine(a, p)
	crank := sketch.NewAngle(ground, crankLine, thetaDeg)
	x := sketch.NewHorizontalDistance(o, p, 0)
	x.SetDriven(true)
	s.AddConstraint(
		sketch.NewCoincident(o, s.Origin()),
		sketch.NewHorizontal(ground),
		sketch.NewDistance(o, g, 100),
		sketch.NewDistance(o, a, r),
		sketch.NewDistance(a, p, l),
		sketch.NewHorizontalPoints(o, p),
		crank,
		x,
	)
	_, err := s.Solve(t.Context())
	require.NoError(t, err, "the seeded slider-crank must solve")
	return &sliderCrank{s: s, crank: crank, x: x}
}

func TestEncloseSliderCrank(t *testing.T) {
	t.Run("E7 the far branch over [30°, 60°]", func(t *testing.T) {
		sc := newSliderCrank(t, 30, 80, 30)
		e, err := sc.s.Enclose(t.Context(), sc.crank, rad(30), rad(60))
		require.NoError(t, err, "the slider-crank has no fold")
		requireContiguous(t, e)
		x, ok := e.Driven(sc.x)
		require.True(t, ok, "the driven slider dimension has an interval")
		closed := func(th float64) float64 {
			return 30*math.Cos(th) + math.Sqrt(80*80-30*30*math.Sin(th)*math.Sin(th))
		}
		requireContainsWithin(t, x, 90.663730, mmTableSlack, "x at 60° (table)")
		requireContainsWithin(t, x, 104.561930, mmTableSlack, "x at 30° (table)")
		requireContains(t, x, closed(rad(60)), "x at 60° (closed form)")
		requireContains(t, x, closed(rad(30)), "x at 30° (closed form)")
		require.Less(t, x.Hi-x.Lo, 15.0, "the hull stays near the true 13.9 travel")
	})
	t.Run("E8 a rod shorter than the crank", func(t *testing.T) {
		sc := newSliderCrank(t, 50, 40, 50)
		e, err := sc.s.Enclose(t.Context(), sc.crank, rad(50), rad(55))
		require.ErrorIs(t, err, sketch.ErrNotCertified, "the branches meet at 53.1301°")
		require.Nil(t, e, "a refusal carries no enclosure")
	})
}

func TestEncloseRefusesUncertifiedKind(t *testing.T) {
	fb := crankRocker(t, 90, true)
	ground := fb.s.CreateLine(fb.o2, fb.o4)
	mid := fb.s.CreatePoint(50, 0)
	m := sketch.NewMidpoint(mid, ground)
	fb.s.AddConstraint(m)
	_, err := fb.s.Solve(t.Context())
	require.NoError(t, err, "the extra midpoint is consistent")

	e, err := fb.s.Enclose(t.Context(), fb.crank, math.Pi/2, math.Pi/2)
	require.ErrorIs(t, err, sketch.ErrUncertifiedConstraint, "E9: one uncertified kind refuses the whole sketch")
	require.Nil(t, e, "a refusal carries no enclosure")

	require.True(t, fb.s.RemoveConstraint(m), "the midpoint is removable")
	require.True(t, fb.s.RemovePoint(mid), "the extra point is removable")
	_, err = fb.s.Enclose(t.Context(), fb.crank, math.Pi/2, math.Pi/2)
	require.NoError(t, err, "E9: removing it restores the certificate")
}

func TestEncloseRefusesFreePoint(t *testing.T) {
	fb := crankRocker(t, 90, true)
	fb.s.CreatePoint(5, 5)
	e, err := fb.s.Enclose(t.Context(), fb.crank, math.Pi/2, math.Pi/2)
	require.ErrorIs(t, err, sketch.ErrUnderconstrained, "E10: a free point leaves two degrees of freedom")
	require.Nil(t, e, "a refusal carries no enclosure")
}

func TestEncloseIsDeterministic(t *testing.T) {
	fb := crankRocker(t, 90, true)
	e1, err := fb.s.Enclose(t.Context(), fb.crank, 0, math.Pi/2)
	require.NoError(t, err, "first call")
	e2, err := fb.s.Enclose(t.Context(), fb.crank, 0, math.Pi/2)
	require.NoError(t, err, "second call")
	p1, p2 := e1.Pieces(), e2.Pieces()
	require.Len(t, p2, len(p1), "E11: the same piece split")
	for i := range p1 {
		require.Equal(t, p1[i].Range(), p2[i].Range(), "E11: the same sub-range")
		for _, p := range fb.s.Points() {
			x1, y1, _ := p1[i].PointBox(p)
			x2, y2, _ := p2[i].PointBox(p)
			require.Equal(t, x1, x2, "E11: bit-identical boxes")
			require.Equal(t, y1, y2, "E11: bit-identical boxes")
		}
		f1, _ := p1[i].Driven(fb.follower)
		f2, _ := p2[i].Driven(fb.follower)
		require.Equal(t, f1, f2, "E11: bit-identical driven intervals")
	}
}

func TestEncloseStaleness(t *testing.T) {
	fb := crankRocker(t, 90, true)
	e, err := fb.s.Enclose(t.Context(), fb.crank, math.Pi/2, math.Pi/2)
	require.NoError(t, err, "E1 enclosure")

	fb.crank.Set(45)
	require.True(t, e.IsStale(), "E12: a changed dimension target makes the enclosure stale before any solve")
	_, err = fb.s.Solve(t.Context())
	require.NoError(t, err, "re-solve at 45°")
	require.True(t, e.IsStale(), "E12: the moved geometry keeps it stale")

	fresh, err := fb.s.Enclose(t.Context(), fb.crank, math.Pi/4, math.Pi/4)
	require.NoError(t, err, "a fresh enclosure at 45°")
	require.False(t, fresh.IsStale(), "the fresh enclosure is current")
	requirePieceHolds(t, fb, fresh, math.Pi/4, true)
	old, _, _ := e.PointBox(fb.b)
	now, _, _ := fresh.PointBox(fb.b)
	require.NotEqual(t, old, now, "E12: the fresh enclosure reports the moved geometry")
}

func TestEncloseContinuation(t *testing.T) {
	fb := crankRocker(t, 0, true)
	start, err := fb.s.Enclose(t.Context(), fb.crank, 0, 0)
	require.NoError(t, err, "certify the document's pose at the drive's start")
	first, err := fb.s.Enclose(t.Context(), fb.crank, 0, math.Pi/4, sketch.WithContinuation(start))
	require.NoError(t, err, "carry the branch over the first eighth turn")
	second, err := fb.s.Enclose(t.Context(), fb.crank, math.Pi/4, math.Pi/2, sketch.WithContinuation(first))
	require.NoError(t, err, "and over the second")
	requirePieceHolds(t, fb, first, rad(30), true)
	requirePieceHolds(t, fb, second, rad(80), true)

	_, err = fb.s.Enclose(t.Context(), fb.crank, math.Pi/3, math.Pi/2, sketch.WithContinuation(first))
	require.ErrorIs(t, err, sketch.ErrNotCertified, "a continuation must start where the previous enclosure ended")

	fb.crank.Set(10)
	_, err = fb.s.Enclose(t.Context(), fb.crank, math.Pi/4, math.Pi/2, sketch.WithContinuation(first))
	require.ErrorIs(t, err, sketch.ErrNotCertified, "a stale enclosure cannot be continued")
}

func TestEncloseInvalidCalls(t *testing.T) {
	fb := crankRocker(t, 90, true)
	_, err := fb.s.Enclose(t.Context(), fb.crank, 1, 0)
	require.ErrorIs(t, err, sketch.ErrNotCertified, "a reversed range is refused")
	_, err = fb.s.Enclose(t.Context(), fb.follower, 1, 1)
	require.ErrorIs(t, err, sketch.ErrNotCertified, "a driven dimension cannot drive")
	_, err = fb.s.Enclose(t.Context(), nil, 1, 1)
	require.ErrorIs(t, err, sketch.ErrNotCertified, "a nil driver is refused")
	other := crankRocker(t, 90, true)
	_, err = fb.s.Enclose(t.Context(), other.crank, 1, 1)
	require.ErrorIs(t, err, sketch.ErrNotCertified, "another sketch's dimension cannot drive")
}
