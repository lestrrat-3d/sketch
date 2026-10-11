package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/units"
	"github.com/stretchr/testify/require"
)

func TestCertifyConeCurveSideFit(t *testing.T) {
	s, cone := coneSectionSketch(t)
	far, err := s.CreateFitSpline(
		s.CreatePoint(-2, 0.5), s.CreatePoint(-3, 0.5), s.CreatePoint(-4, 0.5))
	require.NoError(t, err)
	outside, err := s.CertifyConeCurveSide(far, cone)
	require.NoError(t, err)
	require.Equal(t, sketch.ConeOutside, outside.Side())
	require.Same(t, far, outside.Entity())
	require.Equal(t, cone, outside.Cone())
	require.Positive(t, outside.ImplicitMargin().Sign())
	outside.ImplicitMargin().SetInt64(0)
	require.Positive(t, outside.ImplicitMargin().Sign(), "the returned rational is a copy")
	far.Fit[1].MoveTo(-3, 0.6)
	require.True(t, outside.IsStale())
	require.Zero(t, outside.Side())
	require.Nil(t, outside.ImplicitMargin())

	near, err := s.CreateFitSpline(
		s.CreatePoint(-0.2, 0), s.CreatePoint(0, 0), s.CreatePoint(0.2, 0))
	require.NoError(t, err)
	inside, err := s.CertifyConeCurveSide(near, cone)
	require.NoError(t, err)
	require.Equal(t, sketch.ConeInside, inside.Side())
	require.Positive(t, inside.ImplicitMargin().Sign())
	near.Fit[0] = nil
	require.True(t, inside.IsStale())
}

func TestCertifyConeCurveSideRefusesContact(t *testing.T) {
	s, cone := coneSectionSketch(t)
	for name, points := range map[string][][2]float64{
		"tangent":  {{-0.5, 1}, {0, 1}, {0.5, 1}},
		"crossing": {{0, 0}, {1, 0}, {2, 0}},
	} {
		t.Run(name, func(t *testing.T) {
			fit := make([]*sketch.Point, len(points))
			for i, p := range points {
				fit[i] = s.CreatePoint(p[0], p[1])
			}
			curve, err := s.CreateFitSpline(fit...)
			require.NoError(t, err)
			_, err = s.CertifyConeCurveSide(curve, cone)
			require.ErrorIs(t, err, sketch.ErrConeCurveSide)
		})
	}
}

func TestCertifyConeCurveSideArc(t *testing.T) {
	for _, tc := range []struct {
		radius float64
		want   sketch.ConeSide
	}{
		{2, sketch.ConeOutside},
		{0.5, sketch.ConeInside},
	} {
		s, cone := coneSectionSketch(t)
		y := 0.1
		x := -math.Sqrt(tc.radius*tc.radius - y*y)
		center := s.CreatePoint(0, 0)
		arc := s.CreateArc(center, s.CreatePoint(x, y), s.CreatePoint(x, -y))
		cert, err := s.CertifyConeCurveSide(arc, cone)
		require.NoError(t, err)
		require.Equal(t, tc.want, cert.Side())
		require.Same(t, arc, cert.Entity())
		arc.Start = nil
		require.True(t, cert.IsStale())
	}
}

func TestCertifyConeCurveSideTracksPlaneAndOwnership(t *testing.T) {
	w := sketch.NewWorld()
	require.NoError(t, w.Params().SetValue("height", units.Millimeters(1)))
	plane, err := w.CreateOffsetPlane(w.XY(), 0)
	require.NoError(t, err)
	require.NoError(t, w.BindOffsetPlane(plane, "height"))
	s, err := w.CreateSketch(plane)
	require.NoError(t, err)
	fit, err := s.CreateFitSpline(s.CreatePoint(-2, 0.5), s.CreatePoint(-3, 0.5),
		s.CreatePoint(-4, 0.5))
	require.NoError(t, err)
	cone := sketch.ConeSection{Apex: r3.NewVec(0, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1}
	cert, err := s.CertifyConeCurveSide(fit, cone)
	require.NoError(t, err)
	require.NoError(t, w.Params().SetValue("height", units.Millimeters(2)))
	require.True(t, cert.IsStale())
	require.Zero(t, cert.Side())

	other, _ := coneSectionSketch(t)
	_, err = other.CertifyConeCurveSide(fit, cone)
	require.ErrorIs(t, err, sketch.ErrConeCurveSide)
}

func TestCertifyConeCurveSideTracksPointHandles(t *testing.T) {
	s, cone := coneSectionSketch(t)
	p0, p1, p2 := s.CreatePoint(-2, 0.5), s.CreatePoint(-3, 0.5), s.CreatePoint(-4, 0.5)
	spare := s.CreatePoint(-3, 0.5)
	fit, err := s.CreateFitSpline(p0, p1, p2)
	require.NoError(t, err)
	cert, err := s.CertifyConeCurveSide(fit, cone)
	require.NoError(t, err)
	fit.Fit[1] = spare // Public fields permit rewiring after certification.
	require.True(t, cert.IsStale())
	require.Nil(t, cert.ImplicitMargin())
}

func TestCertifyConeCurveSideManyContactSpansRefuse(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XZ())
	require.NoError(t, err)
	points := make([]*sketch.Point, 30)
	for i := range points {
		x := float64(i + 1)
		points[i] = s.CreatePoint(x, x)
	}
	fit, err := s.CreateFitSpline(points...)
	require.NoError(t, err)
	cone := sketch.ConeSection{Apex: r3.NewVec(0, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1}
	_, err = s.CertifyConeCurveSide(fit, cone)
	require.ErrorIs(t, err, sketch.ErrConeCurveSide)
}
