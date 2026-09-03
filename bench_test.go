package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// This file is the performance-measurement harness for the plan in
// .tmp/perf-plan.md, section 1.1. It adds ONLY benchmarks, fixture builders and
// TestBenchFixtures — no production code changes. The fixtures here are also
// used by golden_test.go (same package) so the two stay in lockstep.

// newFixtureSketch returns an empty sketch on a fresh world's XY datum. It
// mirrors helper_test.go's newSketch but accepts testing.TB so it can be
// shared between *testing.T golden tests and *testing.B benchmarks.
func newFixtureSketch(tb testing.TB) *sketch.Sketch {
	tb.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(tb, err)
	return s
}

// --- small: one grounded rectangle, DOF 0 -----------------------------------

// buildSmallFixture returns a sketch holding one rectangle grounded at the
// origin with a horizontal/vertical shape (from CreateRectangle) plus width
// and height dimensions. DOF 0.
func buildSmallFixture(tb testing.TB) (*sketch.Sketch, *sketch.Distance) {
	tb.Helper()
	s := newFixtureSketch(tb)
	r := s.CreateRectangle(0, 0, 20, 12)
	s.AddConstraint(sketch.NewCoincident(r.A, s.Origin()))
	wd := sketch.NewDistance(r.A, r.B, 20)
	hd := sketch.NewDistance(r.A, r.D, 12)
	s.AddConstraint(wd, hd)
	return s, wd
}

// --- large: a chain of 40 rectangles, DOF 0 ---------------------------------

const largeChainLen = 40

// buildLargeFixture returns a sketch holding a chain of largeChainLen
// rectangles, each grounded to the previous one's B corner (the first to the
// origin), each carrying its own width/height dimension. ~320 free variables,
// DOF 0.
func buildLargeFixture(tb testing.TB) (*sketch.Sketch, *sketch.Distance) {
	tb.Helper()
	s := newFixtureSketch(tb)
	var firstWidth *sketch.Distance
	var prev *sketch.Rectangle
	x := 0.0
	for k := 0; k < largeChainLen; k++ {
		r := s.CreateRectangle(x, 0, x+20, 12)
		wd := sketch.NewDistance(r.A, r.B, 20)
		hd := sketch.NewDistance(r.A, r.D, 12)
		s.AddConstraint(wd, hd)
		if prev == nil {
			s.AddConstraint(sketch.NewCoincident(r.A, s.Origin()))
			firstWidth = wd
		} else {
			s.AddConstraint(sketch.NewCoincident(r.A, prev.B))
		}
		prev = r
		x += 20
	}
	return s, firstWidth
}

// --- circles12: 12 disjoint circles on a 4x3 grid ---------------------------

// buildCircles12Fixture returns a sketch holding 12 disjoint circles of radius
// 10 laid out on a 4x3 grid with spacing 30. Pure geometry — no constraints —
// since Profiles/Regions reads current coordinates directly.
func buildCircles12Fixture(tb testing.TB) *sketch.Sketch {
	tb.Helper()
	s := newFixtureSketch(tb)
	for row := 0; row < 3; row++ {
		for col := 0; col < 4; col++ {
			cx, cy := float64(col)*30, float64(row)*30
			s.CreateCircle(s.CreatePoint(cx, cy), 10)
		}
	}
	return s
}

// --- splines: closed splines plus spline-closed loops -----------------------

// circularClosedSplineControl returns n control points roughly on a circle of
// radius r centered at (cx, cy), which ClosedSpline (a periodic uniform cubic
// B-spline) approximates as a smooth closed loop.
func circularClosedSplineControl(s *sketch.Sketch, cx, cy, r float64, n int) []*sketch.Point {
	pts := make([]*sketch.Point, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = s.CreatePoint(cx+r*math.Cos(a), cy+r*math.Sin(a))
	}
	return pts
}

// splineClosedLoop builds a loop whose fourth side is an open spline instead of
// a straight line: three lines around corners (x0,y0)-(x1,y0)-(x1,y1)-(x0,y1),
// then a 4-control spline from (x0,y1) back to (x0,y0) — sharing those corner
// points, so the loop closes by point identity, not by a coincidence
// constraint.
func splineClosedLoop(s *sketch.Sketch, x0, y0, x1, y1 float64) error {
	p0 := s.CreatePoint(x0, y0)
	p1 := s.CreatePoint(x1, y0)
	p2 := s.CreatePoint(x1, y1)
	p3 := s.CreatePoint(x0, y1)
	s.CreateLine(p0, p1)
	s.CreateLine(p1, p2)
	s.CreateLine(p2, p3)
	mid1 := s.CreatePoint(x0-0.25*(x1-x0), y0+0.35*(y1-y0))
	mid2 := s.CreatePoint(x0-0.1*(x1-x0), y0+0.7*(y1-y0))
	_, err := s.CreateSpline(p3, mid2, mid1, p0)
	return err
}

// buildSplinesFixture returns a sketch holding 4 disjoint closed splines
// (roughly circular, 6 control points each) plus 2 loops each closed by one
// open spline (4 controls) and 3 lines sharing endpoints with it.
func buildSplinesFixture(tb testing.TB) *sketch.Sketch {
	tb.Helper()
	s := newFixtureSketch(tb)
	centers := [][2]float64{{0, 0}, {50, 0}, {0, 50}, {50, 50}}
	for _, c := range centers {
		ctrl := circularClosedSplineControl(s, c[0], c[1], 10, 6)
		_, err := s.CreateClosedSpline(ctrl...)
		require.NoError(tb, err)
	}
	require.NoError(tb, splineClosedLoop(s, 120, 0, 150, 30))
	require.NoError(tb, splineClosedLoop(s, 120, 60, 150, 90))
	return s
}

// --- gallery: one of every entity kind, solved ------------------------------

// buildGalleryFixture returns a solved sketch holding a rectangle, circle,
// arc, ellipse, elliptical arc, spline, closed spline, fit spline, conic,
// NURBS, one construction line, two dimensions and a horizontal constraint —
// the fixture BenchmarkSVG/BenchmarkDXF/TestGoldenExport render.
func buildGalleryFixture(tb testing.TB) *sketch.Sketch {
	tb.Helper()
	s := newFixtureSketch(tb)

	s.CreateRectangle(0, 0, 40, 25)
	circle := s.CreateCircle(s.CreatePoint(90, 12), 12)

	arcCenter := s.CreatePoint(150, 12)
	arcStart := s.CreatePoint(162, 12) // 0 deg, r=12
	arcEnd := s.CreatePoint(150, 24)   // 90 deg, r=12
	s.CreateArc(arcCenter, arcStart, arcEnd)

	s.CreateEllipse(s.CreatePoint(20, 80), 20, 10, 0.3)

	eaCenter := s.CreatePoint(90, 80)
	eaStart := s.CreatePoint(108, 80)             // eccentric angle 0
	eaEnd := s.CreatePoint(81, 87.79422863405995) // eccentric angle 120deg, rx=18 ry=9
	s.CreateEllipticalArc(eaCenter, eaStart, eaEnd, 18, 9, 0)

	_, err := s.CreateSpline(
		s.CreatePoint(140, 70), s.CreatePoint(155, 95), s.CreatePoint(170, 65), s.CreatePoint(185, 90),
	)
	require.NoError(tb, err)

	_, err = s.CreateClosedSpline(circularClosedSplineControl(s, 230, 80, 15, 6)...)
	require.NoError(tb, err)

	_, err = s.CreateFitSpline(
		s.CreatePoint(0, 140), s.CreatePoint(20, 160), s.CreatePoint(40, 140), s.CreatePoint(60, 160),
	)
	require.NoError(tb, err)

	_, err = s.CreateConic(s.CreatePoint(90, 140), s.CreatePoint(110, 160), s.CreatePoint(130, 140), 0.5)
	require.NoError(tb, err)

	nurbsCtrl := []*sketch.Point{
		s.CreatePoint(150, 140), s.CreatePoint(165, 160), s.CreatePoint(180, 130),
		s.CreatePoint(195, 160), s.CreatePoint(210, 130), s.CreatePoint(225, 150),
	}
	_, err = s.CreateNURBS(3, nurbsCtrl, nil, sketch.ClampedUniformKnots(len(nurbsCtrl), 3))
	require.NoError(tb, err)

	cl := s.CreateLine(s.CreatePoint(0, 180), s.CreatePoint(60, 180))
	cl.SetConstruction(true)
	s.AddConstraint(sketch.NewHorizontal(cl))
	s.AddConstraint(sketch.NewDistance(cl.Start, cl.End, 60))
	s.AddConstraint(sketch.NewRadius(circle, 12))

	_, err = s.Solve(tb.Context())
	require.NoError(tb, err)
	return s
}

// --- hexagon-style: a regular polygon grounded per CLAUDE.md's convention --

// buildHexagonFixture returns a sketch holding a regular hexagon grounded by
// one coincidence to the origin, one orientation constraint and a circumradius
// dimension — the "ground, don't pin" pattern, DOF 0.
func buildHexagonFixture(tb testing.TB) *sketch.Sketch {
	tb.Helper()
	s := newFixtureSketch(tb)
	poly, err := s.CreatePolygon(0, 0, 6, 10)
	require.NoError(tb, err)
	s.AddConstraint(sketch.NewCoincident(poly.Center, s.Origin()))
	s.AddConstraint(sketch.NewHorizontal(poly.Spokes[0]))
	s.AddConstraint(sketch.NewDistance(poly.Center, poly.Vertices[0], 10))
	return s
}

// --- conflict: two contradictory distance dimensions on one segment --------

// buildConflictFixture returns a grounded, otherwise DOF-0 sketch carrying two
// contradictory Distance dimensions on the same pair of points (10 vs 15), so
// it is unsolvable and Diagnose reports a conflict.
func buildConflictFixture(tb testing.TB) *sketch.Sketch {
	tb.Helper()
	s := newFixtureSketch(tb)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	l := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
	s.AddConstraint(sketch.NewHorizontal(l))
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.AddConstraint(sketch.NewDistance(a, b, 15))
	return s
}

// --- drivenDim: a fully-constrained rectangle plus a driven diagonal -------

// buildDrivenDimFixture returns a DOF-0 grounded rectangle carrying a driven
// (reference) diagonal Distance, which contributes no residual but is
// refreshed to the measured value after Solve.
func buildDrivenDimFixture(tb testing.TB) *sketch.Sketch {
	tb.Helper()
	s := newFixtureSketch(tb)
	r := s.CreateRectangle(0, 0, 20, 10)
	s.AddConstraint(sketch.NewCoincident(r.A, s.Origin()))
	wd := sketch.NewDistance(r.A, r.B, 20)
	hd := sketch.NewDistance(r.A, r.D, 10)
	s.AddConstraint(wd, hd)
	diag := sketch.NewDistance(r.A, r.C, 999)
	diag.SetDriven(true)
	s.AddConstraint(diag)
	return s
}

// --- tangentAux: a line tangent to an arc's interior sweep -----------------

// buildTangentAuxFixture returns a sketch with a rigid arc and a rigid line
// already tangent at a point inside the arc's sweep, exercising the
// tangentLineCircle sweep-slack auxiliary variable (see constraint.go).
func buildTangentAuxFixture(tb testing.TB) *sketch.Sketch {
	tb.Helper()
	s := newFixtureSketch(tb)
	center := s.CreatePoint(0, 10)
	start := s.CreatePoint(-10, 10) // 180 deg
	end := s.CreatePoint(10, 10)    // 0 deg (360), sweep CCW through 270 deg
	arc := s.CreateArc(center, start, end)
	s.Fix(center)
	s.Fix(start)
	s.Fix(end)

	p1 := s.CreatePoint(-10, 0)
	p2 := s.CreatePoint(10, 0)
	s.Fix(p1)
	s.Fix(p2)
	s.AddConstraint(sketch.NewTangent(s.CreateLine(p1, p2), arc))
	return s
}

// --- benchmarks --------------------------------------------------------

func BenchmarkSolve(b *testing.B) {
	b.Run("small", func(b *testing.B) {
		s, wd := buildSmallFixture(b)
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			if i%2 == 0 {
				wd.Set(20)
			} else {
				wd.Set(25)
			}
			if _, err := s.Solve(b.Context()); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("large", func(b *testing.B) {
		s, wd := buildLargeFixture(b)
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			if i%2 == 0 {
				wd.Set(20)
			} else {
				wd.Set(25)
			}
			if _, err := s.Solve(b.Context()); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkVerify(b *testing.B) {
	b.Run("small", func(b *testing.B) {
		s, _ := buildSmallFixture(b)
		if _, err := s.Solve(b.Context()); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for b.Loop() {
			s.Verify(b.Context())
		}
	})
	b.Run("large", func(b *testing.B) {
		s, _ := buildLargeFixture(b)
		if _, err := s.Solve(b.Context()); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for b.Loop() {
			s.Verify(b.Context())
		}
	})
}

func BenchmarkProfiles(b *testing.B) {
	b.Run("circles12", func(b *testing.B) {
		s := buildCircles12Fixture(b)
		b.ReportAllocs()
		for b.Loop() {
			s.Profiles()
		}
	})
	b.Run("splines", func(b *testing.B) {
		s := buildSplinesFixture(b)
		b.ReportAllocs()
		for b.Loop() {
			s.Profiles()
		}
	})
}

func BenchmarkSVG(b *testing.B) {
	b.Run("plain", func(b *testing.B) {
		s := buildGalleryFixture(b)
		b.ReportAllocs()
		for b.Loop() {
			if _, err := s.SVG(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("annotated", func(b *testing.B) {
		s := buildGalleryFixture(b)
		opts := []sketch.SVGOption{
			sketch.WithDimensions(true), sketch.WithConstraints(true),
			sketch.WithShowPoints(true), sketch.WithStatusBadge(true),
		}
		b.ReportAllocs()
		for b.Loop() {
			if _, err := s.SVG(opts...); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkDXF(b *testing.B) {
	b.Run("local", func(b *testing.B) {
		s := buildGalleryFixture(b)
		b.ReportAllocs()
		for b.Loop() {
			if _, err := s.DXF(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("world", func(b *testing.B) {
		s := buildGalleryFixture(b)
		b.ReportAllocs()
		for b.Loop() {
			if _, err := s.DXF(sketch.WithWorldSpace(true)); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestBenchFixtures asserts each benchmark fixture's basic health — DOF,
// Converged, region count, non-empty exports — so a benchmark can never
// silently measure a broken fixture.
func TestBenchFixtures(t *testing.T) {
	t.Run("small", func(t *testing.T) {
		s, _ := buildSmallFixture(t)
		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res.DOF)
		require.True(t, res.Converged)
	})
	t.Run("large", func(t *testing.T) {
		s, _ := buildLargeFixture(t)
		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res.DOF)
		require.True(t, res.Converged)
	})
	t.Run("circles12", func(t *testing.T) {
		s := buildCircles12Fixture(t)
		require.Len(t, s.Profiles(), 12)
	})
	t.Run("splines", func(t *testing.T) {
		s := buildSplinesFixture(t)
		require.NotEmpty(t, s.Profiles())
	})
	t.Run("gallery", func(t *testing.T) {
		s := buildGalleryFixture(t)
		svg, err := s.SVG()
		require.NoError(t, err)
		require.NotEmpty(t, svg)
		dxf, err := s.DXF()
		require.NoError(t, err)
		require.NotEmpty(t, dxf)
	})
}
