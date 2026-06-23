package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestProbeMirrorTriangle(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	p := s.CreatePoint(5, 3)
	s.AddConstraint(sketch.NewDistance(a, p, 8))
	s.AddConstraint(sketch.NewDistance(b, p, 8))

	res, err := s.Solve()
	require.NoError(t, err, "triangle apex must solve")
	require.Equal(t, 0, res.DOF, "apex is fully constrained")
	require.Greater(t, p.Y(), 0.0, "seeded above the base, solves above")

	pr, err := s.ProbeConfigurations()
	require.NoError(t, err, "probe must succeed on a solved DOF-0 sketch")
	require.True(t, pr.Ambiguous(), "two distance constraints admit a mirror apex")
	require.Len(t, pr.Configurations, 2, "exactly the apex-above and apex-below branches")

	bx, by := pr.Configurations[0].PointXY(p)
	require.Equal(t, p.X(), bx, "baseline configuration is the live solved x")
	require.Equal(t, p.Y(), by, "baseline configuration is the live solved y")

	_, altY := pr.Configurations[1].PointXY(p)
	require.InDelta(t, -by, altY, 1e-8, "alternative is the mirrored apex")
}

func TestProbeTangentSideFlip(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	l := s.CreateLine(a, b)
	center := s.CreatePoint(5, 3)
	c := s.CreateCircle(center, 2.5)
	s.AddConstraint(sketch.NewTangent(l, c))
	s.AddConstraint(sketch.NewRadius(c, 2))
	s.AddConstraint(sketch.NewHorizontalDistance(a, center, 5))

	res, err := s.Solve()
	require.NoError(t, err, "tangent circle must solve")
	require.Equal(t, 0, res.DOF, "circle is fully constrained")
	require.InDelta(t, 2, center.Y(), 1e-8, "seeded above the line, tangent above")

	pr, err := s.ProbeConfigurations()
	require.NoError(t, err, "probe must succeed")
	require.Len(t, pr.Configurations, 2, "tangency is unsigned: above and below the line")

	_, altY := pr.Configurations[1].PointXY(center)
	require.InDelta(t, -2, altY, 1e-8, "alternative is tangent below the line")
}

func TestProbeUniquelyPinnedPoint(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	s.Fix(a)
	p := s.CreatePoint(1, 1)
	s.AddConstraint(sketch.NewHorizontalDistance(a, p, 3))
	s.AddConstraint(sketch.NewVerticalDistance(a, p, 4))

	_, err := s.Solve()
	require.NoError(t, err, "signed Δx/Δy must solve")

	pr, err := s.ProbeConfigurations()
	require.NoError(t, err, "probe must succeed")
	require.False(t, pr.Ambiguous(), "signed dimensions pin a single configuration")
	require.Len(t, pr.Configurations, 1, "only the baseline")
}

func TestProbeSignedAngleUnique(t *testing.T) {
	// The construction from the agent feedback report: a signed 30° angle
	// admits exactly one configuration — (8.66, -5) is NOT a solution, so the
	// probe must not report a mirror branch.
	s := newSketch(t)
	o := s.CreatePoint(0, 0)
	ref := s.CreatePoint(10, 0)
	s.Fix(o)
	s.Fix(ref)
	lref := s.CreateLine(o, ref)
	p := s.CreatePoint(7, 5)
	lp := s.CreateLine(o, p)
	s.AddConstraint(sketch.NewDistance(o, p, 10))
	s.AddConstraint(sketch.NewAngle(lref, lp, 30))

	_, err := s.Solve()
	require.NoError(t, err, "angle construction must solve")

	pr, err := s.ProbeConfigurations()
	require.NoError(t, err, "probe must succeed")
	require.False(t, pr.Ambiguous(), "a signed angle pins the branch")
}

func TestProbeDistanceRectangleIsAmbiguous(t *testing.T) {
	// Rectangles held only by unsigned distances + perpendicularity are the
	// classic trap: every corner has a mirror branch.
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	s.Fix(a)
	b := s.CreatePoint(4, 0.5)
	c := s.CreatePoint(4.5, 3)
	d := s.CreatePoint(0.5, 3.5)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	cd := s.CreateLine(c, d)
	s.AddConstraint(sketch.NewHorizontal(ab))
	s.AddConstraint(sketch.NewDistance(a, b, 4))
	s.AddConstraint(sketch.NewPerpendicular(ab, bc))
	s.AddConstraint(sketch.NewDistance(b, c, 3))
	s.AddConstraint(sketch.NewPerpendicular(bc, cd))
	s.AddConstraint(sketch.NewDistance(c, d, 4))

	res, err := s.Solve()
	require.NoError(t, err, "rectangle must solve")
	require.Equal(t, 0, res.DOF, "rectangle is fully constrained")

	pr, err := s.ProbeConfigurations()
	require.NoError(t, err, "probe must succeed")
	require.GreaterOrEqual(t, len(pr.Configurations), 2,
		"unsigned distances leave mirror branches the probe must find")
}

func TestProbeDeterministic(t *testing.T) {
	build := func() (*sketch.Sketch, *sketch.Point) {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(10, 0)
		s.Fix(a)
		s.Fix(b)
		p := s.CreatePoint(5, 3)
		s.AddConstraint(sketch.NewDistance(a, p, 8))
		s.AddConstraint(sketch.NewDistance(b, p, 8))
		_, err := s.Solve()
		require.NoError(t, err, "fixture must solve")
		return s, p
	}

	s1, p1 := build()
	s2, p2 := build()

	r1, err := s1.ProbeConfigurations()
	require.NoError(t, err, "first probe must succeed")
	r2, err := s2.ProbeConfigurations()
	require.NoError(t, err, "second probe must succeed")
	require.Equal(t, len(r1.Configurations), len(r2.Configurations), "identical inputs find identical counts")
	for i := range r1.Configurations {
		x1, y1 := r1.Configurations[i].PointXY(p1)
		x2, y2 := r2.Configurations[i].PointXY(p2)
		require.Equal(t, x1, x2, "configuration %d x is bit-identical across runs", i)
		require.Equal(t, y1, y2, "configuration %d y is bit-identical across runs", i)
	}

	r3, err := s1.ProbeConfigurations(sketch.WithSeed(42), sketch.WithRestarts(20))
	require.NoError(t, err, "probe with explicit options must succeed")
	r4, err := s1.ProbeConfigurations(sketch.WithSeed(42), sketch.WithRestarts(20))
	require.NoError(t, err, "repeated probe with explicit options must succeed")
	require.Equal(t, len(r3.Configurations), len(r4.Configurations), "explicit seed is reproducible")
}

func TestProbeDoesNotMutateSketch(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	p := s.CreatePoint(5, 3)
	s.AddConstraint(sketch.NewDistance(a, p, 8))
	s.AddConstraint(sketch.NewDistance(b, p, 8))
	ref := sketch.NewDistance(a, p, 1)
	ref.SetDriven(true)
	s.AddConstraint(ref)

	_, err := s.Solve()
	require.NoError(t, err, "fixture must solve")

	wantX, wantY := p.X(), p.Y()
	wantTarget := ref.Target()

	_, err = s.ProbeConfigurations()
	require.NoError(t, err, "probe must succeed")

	require.Equal(t, wantX, p.X(), "probe restores point x exactly")
	require.Equal(t, wantY, p.Y(), "probe restores point y exactly")
	require.Equal(t, wantTarget.Mag(), ref.Target().Mag(), "driven dimension target untouched")
	require.Equal(t, wantTarget.Unit(), ref.Target().Unit(), "driven dimension unit untouched")
}

func TestProbeApplySelectsBranch(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	p := s.CreatePoint(5, 3)
	s.AddConstraint(sketch.NewDistance(a, p, 8))
	s.AddConstraint(sketch.NewDistance(b, p, 8))

	_, err := s.Solve()
	require.NoError(t, err, "fixture must solve")

	pr, err := s.ProbeConfigurations()
	require.NoError(t, err, "probe must succeed")
	require.Len(t, pr.Configurations, 2, "mirror apex expected")

	alt := pr.Configurations[1]
	wantX, wantY := alt.PointXY(p)
	alt.Apply()
	require.Equal(t, wantX, p.X(), "Apply writes the configuration's x")
	require.Equal(t, wantY, p.Y(), "Apply writes the configuration's y")

	res, err := s.Solve()
	require.NoError(t, err, "re-solve from the applied configuration succeeds")
	require.Equal(t, 0, res.DOF, "still fully constrained")
	require.InDelta(t, wantY, p.Y(), 1e-8, "solver stays in the applied basin")
}

func TestProbeRejectsUnderconstrained(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	s.Fix(a)
	p := s.CreatePoint(3, 0)
	s.AddConstraint(sketch.NewDistance(a, p, 5))

	_, err := s.Solve()
	require.NoError(t, err, "underconstrained sketch still solves")

	_, err = s.ProbeConfigurations()
	require.ErrorIs(t, err, sketch.ErrUnderconstrained, "DOF > 0 has a continuum, not branches")
}
