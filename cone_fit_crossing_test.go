package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/units"
	"github.com/stretchr/testify/require"
)

func TestConeFitCrossingCertifiesOneTransverseRoot(t *testing.T) {
	s, cone := coneSectionSketch(t)
	fit, err := s.CreateFitSpline(s.CreatePoint(0.5, 0), s.CreatePoint(1.5, 0))
	require.NoError(t, err)
	cert, err := s.CertifyConeFitCrossing(fit, cone)
	require.NoError(t, err)
	require.Same(t, fit, cert.Spline())
	require.Equal(t, cone, cert.Cone())
	require.Equal(t, s.Revision(), cert.SketchRevision())
	require.False(t, cert.IsStale())
	require.Equal(t, sketch.ConeInside, cert.BeforeSide())
	require.Equal(t, sketch.ConeOutside, cert.AfterSide())
	lo, hi := cert.TBounds()
	require.LessOrEqual(t, lo, 0.5)
	require.GreaterOrEqual(t, hi, 0.5)
	pointLo, pointHi, ok := cert.PointBounds(lo, hi)
	require.True(t, ok)
	require.LessOrEqual(t, pointLo[0], 1.0)
	require.GreaterOrEqual(t, pointHi[0], 1.0)
	require.Equal(t, 0.0, pointLo[1])
	require.Equal(t, 0.0, pointHi[1])
	tangentLo, tangentHi, ok := cert.TangentBounds(0, 1)
	require.True(t, ok)
	require.LessOrEqual(t, tangentLo[0], 1.0)
	require.GreaterOrEqual(t, tangentHi[0], 1.0)
	sign, derivative := cert.DerivativeBound(lo, hi)
	require.Equal(t, 1, sign)
	require.Positive(t, derivative.Sign())
	require.Positive(t, cert.MarginBefore(0.25).Sign())
	require.Positive(t, cert.MarginAfter(0.75).Sign())
	require.Nil(t, cert.MarginBefore(0.5))
	require.Nil(t, cert.MarginAfter(0.5))
	fit.Fit[0].MoveTo(0.25, 0)
	require.True(t, cert.IsStale())
	require.Zero(t, cert.BeforeSide())
	require.Zero(t, cert.AfterSide())
	require.Nil(t, cert.MarginBefore(0.25))
	_, _, ok = cert.PointBounds(lo, hi)
	require.False(t, ok)
}

func TestConeFitCrossingRefusesTwoRoots(t *testing.T) {
	s, cone := coneSectionSketch(t)
	fit, err := s.CreateFitSpline(s.CreatePoint(0.5, 0), s.CreatePoint(1.5, 0),
		s.CreatePoint(0.5, 0))
	require.NoError(t, err)
	_, err = s.CertifyConeFitCrossing(fit, cone)
	require.ErrorIs(t, err, sketch.ErrConeFitCrossing)
}

func TestConeFitCrossingCurvedMultiSpanBounds(t *testing.T) {
	s, cone := coneSectionSketch(t)
	fit, err := s.CreateFitSpline(s.CreatePoint(0.3, 0.2), s.CreatePoint(0.8, 0.4),
		s.CreatePoint(1.5, 0.4), s.CreatePoint(2.0, 0.6))
	require.NoError(t, err)
	cert, err := s.CertifyConeFitCrossing(fit, cone)
	require.NoError(t, err)
	require.Equal(t, sketch.ConeInside, cert.BeforeSide())
	require.Equal(t, sketch.ConeOutside, cert.AfterSide())
	lo, hi := cert.TBounds()
	require.Greater(t, lo, 0.0)
	require.Less(t, hi, 1.0)
	loPoint, hiPoint, ok := cert.PointBounds(lo, hi)
	require.True(t, ok)
	require.LessOrEqual(t, loPoint[0], hiPoint[0])
	require.LessOrEqual(t, loPoint[1], hiPoint[1])
	tangentLo, tangentHi, ok := cert.TangentBounds(0, 1)
	require.True(t, ok)
	require.Greater(t, tangentLo[0], 0.0)
	require.GreaterOrEqual(t, tangentHi[0], tangentLo[0])
	sign, derivative := cert.DerivativeBound(lo, hi)
	require.Equal(t, 1, sign)
	require.Positive(t, derivative.Sign())
	require.Positive(t, cert.MarginBefore(0.05).Sign())
	require.Positive(t, cert.MarginAfter(0.95).Sign())
	fi, err := fit.Interpolant()
	require.NoError(t, err)
	knot := fi.Params[1] / fi.Params[len(fi.Params)-1]
	low, high, ok := cert.PointBounds(math.Nextafter(knot, math.Inf(-1)),
		math.Nextafter(knot, math.Inf(1)))
	require.True(t, ok)
	require.LessOrEqual(t, low[0], fi.Points[1][0])
	require.GreaterOrEqual(t, high[0], fi.Points[1][0])
	require.LessOrEqual(t, low[1], fi.Points[1][1])
	require.GreaterOrEqual(t, high[1], fi.Points[1][1])
	spare := s.CreatePoint(0.8, 0.4)
	fit.Fit[1] = spare
	require.True(t, cert.IsStale())
	require.Nil(t, cert.MarginAfter(0.95))
}

func TestConeFitCrossingRefusesTangentAndMiss(t *testing.T) {
	s, cone := coneSectionSketch(t)
	tangent, err := s.CreateFitSpline(s.CreatePoint(1, -0.5), s.CreatePoint(1, 0),
		s.CreatePoint(1, 0.5))
	require.NoError(t, err)
	_, err = s.CertifyConeFitCrossing(tangent, cone)
	require.ErrorIs(t, err, sketch.ErrConeFitCrossing)
	miss, err := s.CreateFitSpline(s.CreatePoint(0.2, 0), s.CreatePoint(0.4, 0))
	require.NoError(t, err)
	_, err = s.CertifyConeFitCrossing(miss, cone)
	require.ErrorIs(t, err, sketch.ErrConeFitCrossing)
}

func TestConeFitCrossingTracksPlaneFrame(t *testing.T) {
	w := sketch.NewWorld()
	require.NoError(t, w.Params().SetValue("height", units.Millimeters(1)))
	plane, err := w.CreateOffsetPlane(w.XY(), 0)
	require.NoError(t, err)
	require.NoError(t, w.BindOffsetPlane(plane, "height"))
	s, err := w.CreateSketch(plane)
	require.NoError(t, err)
	fit, err := s.CreateFitSpline(s.CreatePoint(0.5, 0), s.CreatePoint(1.5, 0))
	require.NoError(t, err)
	cone := sketch.ConeSection{Apex: r3.NewVec(0, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1}
	cert, err := s.CertifyConeFitCrossing(fit, cone)
	require.NoError(t, err)
	require.NoError(t, w.Params().SetValue("height", units.Millimeters(2)))
	require.True(t, cert.IsStale())
	require.Zero(t, cert.BeforeSide())
	_, _, ok := cert.TangentBounds(0, 1)
	require.False(t, ok)
}
