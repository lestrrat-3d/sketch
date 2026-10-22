package sketch_test

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func newTestSketch(t testing.TB) *sketch.Sketch {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	return s
}

// rectLoop draws a rectangle of four lines and returns them in chain order.
func rectLoop(s *sketch.Sketch, x0, y0, x1, y1 float64) []sketch.Entity {
	a, b, c, d := s.CreatePoint(x0, y0), s.CreatePoint(x1, y0), s.CreatePoint(x1, y1), s.CreatePoint(x0, y1)
	for _, p := range []*sketch.Point{a, b, c, d} {
		s.Fix(p)
	}
	return []sketch.Entity{s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, d), s.CreateLine(d, a)}
}

func solved(t testing.TB, s *sketch.Sketch) {
	t.Helper()
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
}

// profileBox returns the bounding box of a profile's outer boundary.
func profileBox(p *sketch.Profile) [4]float64 {
	box := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, e := range p.Outer {
		for _, pt := range e.Polyline {
			box[0], box[1] = math.Min(box[0], pt[0]), math.Min(box[1], pt[1])
			box[2], box[3] = math.Max(box[2], pt[0]), math.Max(box[3], pt[1])
		}
	}
	return box
}

// profileWith finds the one profile of the given area whose box starts at (x, y).
func profileWith(t *testing.T, s *sketch.Sketch, area, x, y float64) *sketch.Profile {
	t.Helper()
	for _, p := range s.Profiles() {
		b := profileBox(p)
		if math.Abs(p.Area-area) < 1e-6 && math.Abs(b[0]-x) < 1e-9 && math.Abs(b[1]-y) < 1e-9 {
			return p
		}
	}
	t.Fatalf("no profile of area %v at (%v, %v)", area, x, y)
	return nil
}

func requireVerdict(t *testing.T, p *sketch.Profile, l *sketch.Loop, want sketch.ContainmentState, why string) {
	t.Helper()
	got := p.Containment(l)
	require.Equalf(t, want, got.State, "%s (reason %d)", why, got.Reason)
	if want != sketch.ContainmentUnproven {
		require.Equal(t, sketch.ReasonNone, got.Reason, why)
	}
}

// regionLoopScene builds a root disc and n closed regions about the origin, each of
// two lines that start on the circle, two fit splines of fitPoints points, a tip arc
// and a root arc lying on the circle. It returns the sketch, the disc loop and one
// loop per region together with that region's tip arc.
func regionLoopScene(t testing.TB, n, fitPoints int) (*sketch.Sketch, *sketch.Loop, []*sketch.Loop, []*sketch.Arc) {
	t.Helper()
	const rootRadius = 3.0
	s := newTestSketch(t)
	center := s.Origin()
	pitch := math.Pi / float64(n)
	var regionEnts [][]sketch.Entity
	var tips []*sketch.Arc
	for k := range n {
		rot := 2 * math.Pi * float64(k) / float64(n)
		point := func(r, a float64) *sketch.Point {
			p := s.CreatePoint(r*math.Cos(a+rot), r*math.Sin(a+rot))
			s.Fix(p)
			return p
		}
		a := 0.4 * pitch
		rootRight, rootLeft := point(rootRadius, -a), point(rootRadius, a)
		right := make([]*sketch.Point, fitPoints)
		left := make([]*sketch.Point, fitPoints)
		for i := range fitPoints {
			f := float64(i) / float64(fitPoints-1)
			r := 3.35 + f*(4.0-3.35)
			half := a * (1 - 0.4*f)
			right[i], left[i] = point(r, -half), point(r, half)
		}
		l1 := s.CreateLine(rootRight, right[0])
		sp1, err := s.CreateFitSpline(right...)
		require.NoError(t, err)
		tip := s.CreateArc(center, right[fitPoints-1], left[fitPoints-1])
		sp2, err := s.CreateFitSpline(left...)
		require.NoError(t, err)
		l2 := s.CreateLine(left[0], rootLeft)
		root := s.CreateArc(center, rootRight, rootLeft)
		// chain order: root point right -> line -> spline -> tip -> spline reversed ->
		// line -> root point left -> root arc back to the right root point
		regionEnts = append(regionEnts, []sketch.Entity{l1, sp1, tip, sp2, l2, root})
		tips = append(tips, tip)
	}
	circle := s.CreateCircle(center, rootRadius)
	s.AddConstraint(sketch.NewRadius(circle, rootRadius))
	solved(t, s)
	disc, err := s.DeclareLoop(circle)
	require.NoError(t, err)
	var loops []*sketch.Loop
	for _, ents := range regionEnts {
		l, err := s.DeclareLoop(ents...)
		require.NoError(t, err)
		loops = append(loops, l)
	}
	return s, disc, loops, tips
}

func TestContainmentDiscWithRegionsClosedByCoincidentArcs(t *testing.T) {
	for _, n := range []int{8, 12, 17, 24} {
		for _, fit := range []int{4, 15} {
			t.Run(fmt.Sprintf("n=%d/fit=%d", n, fit), func(t *testing.T) {
				s, disc, loops, tips := regionLoopScene(t, n, fit)
				profiles := s.Profiles()
				require.Len(t, profiles, n+1)
				tooth := make([]*sketch.Profile, n)
				var discProfile *sketch.Profile
				for _, p := range profiles {
					if k := slices.IndexFunc(tips, func(a *sketch.Arc) bool { return slices.Contains(p.Entities, sketch.Entity(a)) }); k >= 0 {
						tooth[k] = p
					} else {
						discProfile = p
					}
				}
				require.NotNil(t, discProfile)
				requireVerdict(t, discProfile, disc, sketch.ContainmentInside, "the disc region is inside the disc loop")
				for k, l := range loops {
					requireVerdict(t, discProfile, l, sketch.ContainmentOutside, "the disc region is outside every region loop")
					requireVerdict(t, tooth[k], disc, sketch.ContainmentOutside, "a region is outside the disc loop")
					for j, p := range tooth {
						want := sketch.ContainmentOutside
						if j == k {
							want = sketch.ContainmentInside
						}
						requireVerdict(t, p, l, want, fmt.Sprintf("region %d against loop %d", j, k))
					}
				}
				// Containments reads the same answers in Loops() order.
				all := tooth[0].Containments()
				require.Len(t, all, n+1)
				require.Equal(t, tooth[0].Containment(disc), all[0])
			})
		}
	}
}

func TestContainmentOverlappingRectangles(t *testing.T) {
	s := newTestSketch(t)
	a := rectLoop(s, 0, 0, 4, 4)
	b := rectLoop(s, 2, 2, 6, 6)
	la, err := s.DeclareLoop(a...)
	require.NoError(t, err)
	lb, err := s.DeclareLoop(b...)
	require.NoError(t, err)
	solved(t, s)
	require.Len(t, s.Profiles(), 3)
	aOnly := profileWith(t, s, 12, 0, 0)
	bOnly := profileWith(t, s, 12, 2, 2)
	both := profileWith(t, s, 4, 2, 2)
	requireVerdict(t, aOnly, la, sketch.ContainmentInside, "A only")
	requireVerdict(t, aOnly, lb, sketch.ContainmentOutside, "A only")
	requireVerdict(t, bOnly, la, sketch.ContainmentOutside, "B only")
	requireVerdict(t, bOnly, lb, sketch.ContainmentInside, "B only")
	requireVerdict(t, both, la, sketch.ContainmentInside, "overlap")
	requireVerdict(t, both, lb, sketch.ContainmentInside, "overlap")
}

func TestContainmentNestedLoops(t *testing.T) {
	s := newTestSketch(t)
	outer := rectLoop(s, 0, 0, 10, 10)
	inner := rectLoop(s, 3, 3, 6, 6)
	lo, err := s.DeclareLoop(outer...)
	require.NoError(t, err)
	li, err := s.DeclareLoop(inner...)
	require.NoError(t, err)
	solved(t, s)
	annulus := profileWith(t, s, 91, 0, 0)
	core := profileWith(t, s, 9, 3, 3)
	// A region with a hole is the whole face, outer boundary minus holes.
	requireVerdict(t, annulus, lo, sketch.ContainmentInside, "the face around the hole")
	requireVerdict(t, annulus, li, sketch.ContainmentOutside, "the face around the hole")
	requireVerdict(t, core, lo, sketch.ContainmentInside, "the face in the hole")
	requireVerdict(t, core, li, sketch.ContainmentInside, "the face in the hole")
}

func TestContainmentTwoLoopsSharingAWholeEdge(t *testing.T) {
	s := newTestSketch(t)
	pts := make([]*sketch.Point, 6)
	for i, xy := range [][2]float64{{0, 0}, {3, 0}, {6, 0}, {6, 3}, {3, 3}, {0, 3}} {
		pts[i] = s.CreatePoint(xy[0], xy[1])
		s.Fix(pts[i])
	}
	shared := s.CreateLine(pts[1], pts[4])
	l0 := s.CreateLine(pts[0], pts[1])
	l1 := s.CreateLine(pts[4], pts[5])
	l2 := s.CreateLine(pts[5], pts[0])
	r0 := s.CreateLine(pts[1], pts[2])
	r1 := s.CreateLine(pts[2], pts[3])
	r2 := s.CreateLine(pts[3], pts[4])
	left, err := s.DeclareLoop(l0, shared, l1, l2)
	require.NoError(t, err)
	right, err := s.DeclareLoop(r0, r1, r2, shared)
	require.NoError(t, err)
	solved(t, s)
	require.Len(t, s.Profiles(), 2)
	lp := profileWith(t, s, 9, 0, 0)
	rp := profileWith(t, s, 9, 3, 0)
	requireVerdict(t, lp, left, sketch.ContainmentInside, "left")
	requireVerdict(t, lp, right, sketch.ContainmentOutside, "left")
	requireVerdict(t, rp, left, sketch.ContainmentOutside, "right")
	requireVerdict(t, rp, right, sketch.ContainmentInside, "right")
}

func TestContainmentOpenChainIsAGap(t *testing.T) {
	s := newTestSketch(t)
	open := rectLoop(s, 0, 0, 4, 4)[:3]
	closed := rectLoop(s, 10, 10, 12, 12)
	lo, err := s.DeclareLoop(open...)
	require.NoError(t, err)
	lc, err := s.DeclareLoop(closed...)
	require.NoError(t, err)
	solved(t, s)
	p := profileWith(t, s, 4, 10, 10)
	require.Equal(t, sketch.Containment{State: sketch.ContainmentUnproven, Reason: sketch.ReasonGap}, p.Containment(lo))
	requireVerdict(t, p, lc, sketch.ContainmentInside, "the closed loop is judged on its own")
}

func TestContainmentMismatchedJoinIsAGap(t *testing.T) {
	// A rectangle whose last corner is two points one unit in the last place apart.
	// The arrangement welds them, so the region exists, but the loop's join is not
	// bit-identical.
	s := newTestSketch(t)
	pt := func(x, y float64) *sketch.Point {
		p := s.CreatePoint(x, y)
		s.Fix(p)
		return p
	}
	a, b, c := pt(0, 0), pt(4, 0), pt(4, 4)
	d1, d2 := pt(0, 4), pt(math.Nextafter(0, 1), 4)
	ents := []sketch.Entity{s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, d1), s.CreateLine(d2, a)}
	l, err := s.DeclareLoop(ents...)
	require.NoError(t, err)
	other := rectLoop(s, 20, 20, 22, 22)
	lo, err := s.DeclareLoop(other...)
	require.NoError(t, err)
	solved(t, s)
	p := profileWith(t, s, 4, 20, 20)
	require.Equal(t, sketch.ReasonGap, p.Containment(l).Reason)
	requireVerdict(t, p, lo, sketch.ContainmentInside, "an intact loop beside it")
	for _, q := range s.Profiles() {
		require.Equal(t, sketch.ContainmentUnproven, q.Containment(l).State)
	}
}

func TestContainmentLoopThatTouchesItselfIsSelfCrossing(t *testing.T) {
	// Two squares meeting at one corner, declared as one eight-line loop.
	s := newTestSketch(t)
	corner := s.CreatePoint(0, 0)
	s.Fix(corner)
	mk := func(sx, sy float64) []sketch.Entity {
		b := s.CreatePoint(sx*3, 0)
		c := s.CreatePoint(sx*3, sy*3)
		d := s.CreatePoint(0, sy*3)
		for _, p := range []*sketch.Point{b, c, d} {
			s.Fix(p)
		}
		return []sketch.Entity{s.CreateLine(corner, b), s.CreateLine(b, c), s.CreateLine(c, d), s.CreateLine(d, corner)}
	}
	ents := append(mk(1, 1), mk(-1, -1)...)
	l, err := s.DeclareLoop(ents...)
	require.NoError(t, err)
	solved(t, s)
	for _, p := range s.Profiles() {
		require.Equal(t, sketch.Containment{State: sketch.ContainmentUnproven, Reason: sketch.ReasonSelfCrossing}, p.Containment(l))
	}
}

func TestContainmentKindAndConstruction(t *testing.T) {
	s := newTestSketch(t)
	e := s.CreateEllipse(s.CreatePoint(0, 0), 5, 3, 0)
	rect := rectLoop(s, 20, 20, 22, 22)
	le, err := s.DeclareLoop(e)
	require.NoError(t, err)
	lr, err := s.DeclareLoop(rect...)
	require.NoError(t, err)
	solved(t, s)
	p := profileWith(t, s, 4, 20, 20)
	require.Equal(t, sketch.ReasonKind, p.Containment(le).Reason)
	// An ellipse anywhere closes the exactness gate, which is a component reason.
	require.Equal(t, sketch.ContainmentUnproven, p.Containment(lr).State)
}

func TestContainmentDeclarationErrors(t *testing.T) {
	s := newTestSketch(t)
	other := newTestSketch(t)
	a := rectLoop(s, 0, 0, 1, 1)
	foreign := rectLoop(other, 0, 0, 1, 1)
	_, err := s.DeclareLoop()
	require.ErrorIs(t, err, sketch.ErrLoopEmpty)
	require.ErrorIs(t, err, sketch.ErrInvalidLoop)
	_, err = s.DeclareLoop(a[0], nil)
	require.ErrorIs(t, err, sketch.ErrLoopNilEntity)
	_, err = s.DeclareLoop(a[0], foreign[0])
	require.ErrorIs(t, err, sketch.ErrLoopForeignEntity)
	_, err = s.DeclareLoop(a[0], a[1], a[0])
	require.ErrorIs(t, err, sketch.ErrLoopDuplicateEntity)
	require.NotErrorIs(t, err, sketch.ErrLoopNilEntity)
	require.Empty(t, s.Loops())
}

func TestContainmentUndeclaredAndStale(t *testing.T) {
	s := newTestSketch(t)
	ents := rectLoop(s, 0, 0, 4, 4)
	solved(t, s)
	// A profile built before the declaration reads Undeclared for the loop.
	before := s.Profiles()
	require.Len(t, before, 1)
	l, err := s.DeclareLoop(ents...)
	require.NoError(t, err)
	require.Equal(t, sketch.ContainmentUndeclared, before[0].Containment(l).State)
	after := s.Profiles()
	requireVerdict(t, after[0], l, sketch.ContainmentInside, "declared before the build")
	// Another sketch's loop, a nil loop, and a removed loop.
	other := newTestSketch(t)
	ol, err := other.DeclareLoop(rectLoop(other, 0, 0, 1, 1)...)
	require.NoError(t, err)
	require.Equal(t, sketch.ContainmentUndeclared, after[0].Containment(ol).State)
	require.Equal(t, sketch.ContainmentUndeclared, after[0].Containment(nil).State)
	// A change to the sketch makes the profile stale.
	s.CreateLine(s.CreatePoint(50, 50), s.CreatePoint(51, 51))
	require.True(t, after[0].IsStale())
	require.Equal(t, sketch.Containment{State: sketch.ContainmentUnproven, Reason: sketch.ReasonStale}, after[0].Containment(l))
	// Removing the loop, or an entity of it, retires the handle.
	require.True(t, s.RemoveLoop(l))
	require.True(t, l.Removed())
	require.False(t, s.RemoveLoop(l))
	l2, err := s.DeclareLoop(ents...)
	require.NoError(t, err)
	require.True(t, s.RemoveEntity(ents[0]))
	require.True(t, l2.Removed())
	require.Empty(t, s.Loops())
}

func TestContainmentDoesNotDependOnDeclarationOrder(t *testing.T) {
	build := func(rotate int, reverse bool) sketch.Containment {
		s := newTestSketch(t)
		a := rectLoop(s, 0, 0, 4, 4)
		b := rectLoop(s, 2, 2, 6, 6)
		ents := slices.Clone(a)
		if reverse {
			slices.Reverse(ents)
		}
		ents = append(ents[rotate:], ents[:rotate]...)
		la, err := s.DeclareLoop(ents...)
		require.NoError(t, err)
		_, err = s.DeclareLoop(b...)
		require.NoError(t, err)
		solved(t, s)
		return profileWith(t, s, 4, 2, 2).Containment(la)
	}
	want := build(0, false)
	require.Equal(t, sketch.ContainmentInside, want.State)
	for _, rot := range []int{1, 2, 3} {
		require.Equal(t, want, build(rot, false), "rotated %d", rot)
		require.Equal(t, want, build(rot, true), "reversed and rotated %d", rot)
	}
}

func TestContainmentLeavesASketchWithoutLoopsAlone(t *testing.T) {
	s := newTestSketch(t)
	rectLoop(s, 0, 0, 4, 4)
	solved(t, s)
	ps := s.Profiles()
	require.Len(t, ps, 1)
	require.Empty(t, ps[0].Containments())
	require.Equal(t, sketch.ContainmentUndeclared, ps[0].Containment(nil).State)
}

func TestDeclaringLoopsChangesNoProfile(t *testing.T) {
	build := func(declare bool) []*sketch.Profile {
		s := newTestSketch(t)
		a := rectLoop(s, 0, 0, 4, 4)
		b := rectLoop(s, 2, 2, 6, 6)
		if declare {
			_, err := s.DeclareLoop(a...)
			require.NoError(t, err)
			_, err = s.DeclareLoop(b...)
			require.NoError(t, err)
		}
		solved(t, s)
		return s.Profiles()
	}
	plain, declared := build(false), build(true)
	require.Len(t, declared, len(plain))
	for i := range plain {
		require.Equal(t, plain[i].Area, declared[i].Area)
		require.Equal(t, plain[i].Valid, declared[i].Valid)
		require.Len(t, declared[i].Outer, len(plain[i].Outer))
		for j := range plain[i].Outer {
			require.Equal(t, plain[i].Outer[j].TStart, declared[i].Outer[j].TStart)
			require.Equal(t, plain[i].Outer[j].TEnd, declared[i].Outer[j].TEnd)
			require.Equal(t, plain[i].Outer[j].TExact, declared[i].Outer[j].TExact)
		}
	}
}

// TestContainmentsMatchesContainmentPerLoop pins that Containments, which asks once
// whether the profile is stale, answers exactly what Containment answers loop by loop:
// before and after a change to the sketch, and with a loop declared later, a loop
// removed, and a loop from another sketch.
func TestContainmentsMatchesContainmentPerLoop(t *testing.T) {
	s := newTestSketch(t)
	first, err := s.DeclareLoop(rectLoop(s, 0, 0, 4, 4)...)
	require.NoError(t, err)
	_, err = s.DeclareLoop(rectLoop(s, 2, 2, 6, 6)...)
	require.NoError(t, err)
	solved(t, s)
	profiles := s.Profiles()
	require.NotEmpty(t, profiles)

	perLoop := func(p *sketch.Profile) []sketch.Containment {
		var out []sketch.Containment
		for _, l := range s.Loops() {
			out = append(out, p.Containment(l))
		}
		return out
	}
	check := func(label string) {
		t.Helper()
		for i, p := range profiles {
			require.Equalf(t, perLoop(p), p.Containments(), "%s: profile %d", label, i)
		}
	}
	check("fresh")
	for _, p := range profiles {
		require.NotEqual(t, sketch.ContainmentUndeclared, p.Containments()[0].State)
	}

	_, err = s.DeclareLoop(rectLoop(s, 10, 10, 12, 12)...)
	require.NoError(t, err)
	check("a loop declared after the profiles")
	for _, p := range profiles {
		require.Equal(t, sketch.ContainmentUndeclared, p.Containments()[2].State)
	}

	require.True(t, s.RemoveLoop(first))
	check("a loop removed")
	require.Len(t, profiles[0].Containments(), 2)
	require.Equal(t, sketch.ContainmentUndeclared, profiles[0].Containment(first).State)

	s.CreatePoint(20, 20)
	check("stale")
	for _, p := range profiles {
		c := p.Containments()
		require.Equal(t, sketch.Containment{State: sketch.ContainmentUnproven, Reason: sketch.ReasonStale}, c[0])
		require.Equal(t, sketch.ContainmentUndeclared, c[1].State, "the third loop has no verdict on these profiles")
	}
}
