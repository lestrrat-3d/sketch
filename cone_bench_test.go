package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// coneBenchSketch is coneSectionSketch (cone_line_test.go) for testing.TB: the
// cone section is the unit circle about the sketch origin.
func coneBenchSketch(tb testing.TB) (*sketch.Sketch, sketch.ConeSection) {
	tb.Helper()
	w := sketch.NewWorld()
	f, err := r3.NewFrame(r3.NewVec(0, 0, 1), r3.NewVec(1, 0, 0), r3.NewVec(0, 1, 0))
	require.NoError(tb, err)
	plane, err := w.CreatePlaneFromFrame(f)
	require.NoError(tb, err)
	s, err := w.CreateSketch(plane)
	require.NoError(tb, err)
	return s, sketch.ConeSection{Apex: r3.NewVec(0, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1}
}

// BenchmarkCertifyConeCurveSide certifies one whole curve of each kind the
// cone-side proof takes: a line, a fitted spline whose spans need bisection,
// and an arc whose wedge must be split.
func BenchmarkCertifyConeCurveSide(b *testing.B) {
	b.Run("line", func(b *testing.B) {
		s, cone := coneBenchSketch(b)
		line := s.CreateLine(s.CreatePoint(0.5, 0), s.CreatePoint(0.75, 0))
		b.ReportAllocs()
		for b.Loop() {
			if _, err := s.CertifyConeCurveSide(line, cone); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("fit", func(b *testing.B) {
		s, cone := coneBenchSketch(b)
		fit, err := s.CreateFitSpline(s.CreatePoint(0.2, 0.1), s.CreatePoint(0.5, 0.6),
			s.CreatePoint(0.8, 0.3), s.CreatePoint(0.6, -0.5), s.CreatePoint(0.1, -0.7))
		require.NoError(b, err)
		b.ReportAllocs()
		for b.Loop() {
			if _, err := s.CertifyConeCurveSide(fit, cone); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("arc", func(b *testing.B) {
		s, cone := coneBenchSketch(b)
		rad := func(deg float64) float64 { return deg * math.Pi / 180 }
		arc := s.CreateArc(s.CreatePoint(0, 0),
			s.CreatePoint(0.97*math.Cos(rad(10)), 0.97*math.Sin(rad(10))),
			s.CreatePoint(0.97*math.Cos(rad(170)), 0.97*math.Sin(rad(170))))
		b.ReportAllocs()
		for b.Loop() {
			if _, err := s.CertifyConeCurveSide(arc, cone); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkCertifyConeFitCrossing certifies the one crossing of a curved
// four-point fitted spline with the cone section.
func BenchmarkCertifyConeFitCrossing(b *testing.B) {
	s, cone := coneBenchSketch(b)
	fit, err := s.CreateFitSpline(s.CreatePoint(0.3, 0.2), s.CreatePoint(0.8, 0.4),
		s.CreatePoint(1.5, 0.4), s.CreatePoint(2.0, 0.6))
	require.NoError(b, err)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.CertifyConeFitCrossing(fit, cone); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncloseDistanceDriver encloses two bars from pivots 100 apart that
// meet at B, driven by one bar's length: a point, line and distance system
// with no angle, so the run is the Krawczyk test and the float solves alone.
func BenchmarkEncloseDistanceDriver(b *testing.B) {
	s := newFixtureSketch(b)
	o2 := s.CreatePoint(0, 0)
	o4 := s.CreatePoint(100, 0)
	pb := s.CreatePoint(0, 60)
	ground := s.CreateLine(o2, o4)
	s.CreateLine(o2, pb)
	s.CreateLine(o4, pb)
	driver := sketch.NewDistance(o4, pb, math.Hypot(100, 60))
	s.AddConstraint(
		sketch.NewCoincident(o2, s.Origin()),
		sketch.NewHorizontal(ground),
		sketch.NewDistance(o2, o4, 100),
		sketch.NewDistance(o2, pb, 60),
		driver,
	)
	_, err := s.Solve(b.Context())
	require.NoError(b, err)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Enclose(b.Context(), driver, 60, 155); err != nil {
			b.Fatal(err)
		}
	}
}
