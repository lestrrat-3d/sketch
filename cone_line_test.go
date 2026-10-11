package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/units"
	"github.com/stretchr/testify/require"
)

func coneSectionSketch(t *testing.T) (*sketch.Sketch, sketch.ConeSection) {
	t.Helper()
	w := sketch.NewWorld()
	f, err := r3.NewFrame(r3.NewVec(0, 0, 1), r3.NewVec(1, 0, 0), r3.NewVec(0, 1, 0))
	require.NoError(t, err)
	plane, err := w.CreatePlaneFromFrame(f)
	require.NoError(t, err)
	s, err := w.CreateSketch(plane)
	require.NoError(t, err)
	return s, sketch.ConeSection{Apex: r3.NewVec(0, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1}
}

func TestCertifyConeLineCrossing(t *testing.T) {
	s, cone := coneSectionSketch(t)
	start, end := s.CreatePoint(0, 0.5), s.CreatePoint(2, 0.5)
	line := s.CreateLine(start, end)
	cert, err := s.CertifyConeLineCrossing(line, cone)
	require.NoError(t, err)
	require.True(t, cert.TExact())
	require.False(t, cert.IsStale())
	require.Same(t, line, cert.Line())
	require.Equal(t, cone, cert.Cone())
	require.Equal(t, s.Revision(), cert.SketchRevision())
	lo, hi := cert.TBounds()
	require.LessOrEqual(t, lo, math.Sqrt(0.75)/2)
	require.GreaterOrEqual(t, hi, math.Sqrt(0.75)/2)
	require.InDelta(t, math.Sqrt(0.75)/2, cert.T(), 2e-16)
	require.InDelta(t, math.Sqrt(0.75), cert.Point()[0], 4e-16)
	require.InDelta(t, 0.5, cert.Point()[1], 1e-16)
	cone.Slope = 2
	require.Equal(t, 1.0, cert.Cone().Slope, "the certificate copies source values")

	end.MoveTo(3, 0.5)
	require.True(t, cert.IsStale())
	require.False(t, cert.TExact())
	other, err := s.CertifyConeLineCrossing(line, cone)
	require.NoError(t, err)
	require.False(t, other.IsStale())
}

func TestCertifyConeLineCrossingRefusesAmbiguousContacts(t *testing.T) {
	tests := []struct {
		name   string
		ax, ay float64
		bx, by float64
	}{
		{"tangent", -2, 1, 2, 1},
		{"miss", -2, 2, 2, 2},
		{"two crossings", -2, 0, 2, 0},
		{"endpoint", -1, 0, 0, 0},
		{"behind apex", 0, 0, 2, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, cone := coneSectionSketch(t)
			if tc.name == "behind apex" {
				cone.Axis = r3.NewVec(0, 0, -1)
			}
			line := s.CreateLine(s.CreatePoint(tc.ax, tc.ay), s.CreatePoint(tc.bx, tc.by))
			_, err := s.CertifyConeLineCrossing(line, cone)
			require.ErrorIs(t, err, sketch.ErrConeLineCrossing)
		})
	}
}

func TestCertifyConeLineCrossingSourceIdentity(t *testing.T) {
	s, cone := coneSectionSketch(t)
	line := s.CreateLine(s.CreatePoint(0, 0.5), s.CreatePoint(2, 0.5))
	other, _ := coneSectionSketch(t)
	_, err := other.CertifyConeLineCrossing(line, cone)
	require.ErrorIs(t, err, sketch.ErrConeLineCrossing)
	cone.Slope = math.NaN()
	_, err = s.CertifyConeLineCrossing(line, cone)
	require.ErrorIs(t, err, sketch.ErrConeLineCrossing)
	foreignPoint := other.CreatePoint(2, 0.5)
	line.End = foreignPoint
	_, err = s.CertifyConeLineCrossing(line, sketch.ConeSection{
		Apex: r3.NewVec(0, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1,
	})
	require.ErrorIs(t, err, sketch.ErrConeLineCrossing)
	line.End = nil
	_, err = s.CertifyConeLineCrossing(line, sketch.ConeSection{
		Apex: r3.NewVec(0, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1,
	})
	require.ErrorIs(t, err, sketch.ErrConeLineCrossing)
}

func TestCertifyConeLineCrossingTracksPlaneFrame(t *testing.T) {
	w := sketch.NewWorld()
	require.NoError(t, w.Params().SetValue("height", units.Millimeters(1)))
	plane, err := w.CreateOffsetPlane(w.XY(), 0)
	require.NoError(t, err)
	require.NoError(t, w.BindOffsetPlane(plane, "height"))
	s, err := w.CreateSketch(plane)
	require.NoError(t, err)
	line := s.CreateLine(s.CreatePoint(0, 0.5), s.CreatePoint(2, 0.5))
	cone := sketch.ConeSection{Apex: r3.NewVec(0, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1}
	cert, err := s.CertifyConeLineCrossing(line, cone)
	require.NoError(t, err)
	require.False(t, cert.IsStale())
	revision := s.Revision()
	require.NoError(t, w.Params().SetValue("height", units.Millimeters(2)))
	require.Equal(t, revision, s.Revision(), "the plane can move without a sketch revision")
	require.True(t, cert.IsStale())
	require.False(t, cert.TExact())
}

func TestCertifyConeLineCrossingTracksEndpointHandles(t *testing.T) {
	s, cone := coneSectionSketch(t)
	start, end := s.CreatePoint(0, 0.5), s.CreatePoint(2, 0.5)
	spare := s.CreatePoint(2, 0.5)
	line := s.CreateLine(start, end)
	cert, err := s.CertifyConeLineCrossing(line, cone)
	require.NoError(t, err)
	line.End = spare // Public fields permit rewiring after certification.
	require.True(t, cert.IsStale())
	require.False(t, cert.TExact())
}

func TestCertifyConeLineCrossingRefusesUnrepresentableLineEvaluation(t *testing.T) {
	w := sketch.NewWorld()
	plane, err := w.CreateOffsetPlane(w.XY(), 1.5e308)
	require.NoError(t, err)
	s, err := w.CreateSketch(plane)
	require.NoError(t, err)
	line := s.CreateLine(s.CreatePoint(-1e308, 0), s.CreatePoint(1e308, 0))
	cone := sketch.ConeSection{
		Apex: r3.NewVec(1e308, 0, 0), Axis: r3.NewVec(0, 0, 1), Slope: 1,
	}
	_, err = s.CertifyConeLineCrossing(line, cone)
	require.ErrorIs(t, err, sketch.ErrConeLineCrossing)
}

func TestCertifyConeLineCrossingTinyParameter(t *testing.T) {
	s, cone := coneSectionSketch(t)
	cone.Slope = 3
	line := s.CreateLine(s.CreatePoint(2, 0.5), s.CreatePoint(1e30, 0.5))
	cert, err := s.CertifyConeLineCrossing(line, cone)
	require.NoError(t, err)
	require.True(t, cert.TExact())
	lo, hi := cert.TBounds()
	ulp := math.Nextafter(cert.T(), math.Inf(1)) - cert.T()
	require.LessOrEqual(t, hi-lo, 4*ulp)
	require.InDelta(t, math.Sqrt(8.75), cert.Point()[0], 2e-15)
}
