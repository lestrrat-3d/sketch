package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// nurbsControl returns five control points for a degree-3 NURBS, giving one
// interior knot to poison.
func nurbsControl(s *sketch.Sketch) []*sketch.Point {
	return []*sketch.Point{
		s.CreatePoint(-2, 5),
		s.CreatePoint(2, 5),
		s.CreatePoint(5, 5),
		s.CreatePoint(8, 5),
		s.CreatePoint(12, 5),
	}
}

// nonFiniteSketch builds a sketch holding a NURBS whose one interior knot is
// NaN. CreateNURBS's non-decreasing check compares knot values with <, and
// every ordered comparison against NaN is false, so the NaN passes it silently;
// the clamping check uses != and never examines an interior knot, and nothing
// tests knot finiteness. The NaN reaches the curve: sampling it for the bounding
// box (Polyline) evaluates to NaN, poisoning every exporter's bounds.
func nonFiniteSketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	knots := sketch.ClampedUniformKnots(5, 3)
	knots[4] = math.NaN()
	_, err := s.CreateNURBS(3, nurbsControl(s), nil, knots)
	require.NoError(t, err, "CreateNURBS does not validate knot finiteness")
	return s
}

// healthySketch is the same shape with a well-formed knot vector, so any
// exporter difference between it and nonFiniteSketch is attributable to the
// NaN alone.
func healthySketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	_, err := s.CreateNURBS(3, nurbsControl(s), nil, sketch.ClampedUniformKnots(5, 3))
	require.NoError(t, err)
	return s
}

func TestSVGRefusesNonFiniteGeometry(t *testing.T) {
	s := nonFiniteSketch(t)
	out, err := s.SVG()
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, out)
}

func TestDXFRefusesNonFiniteGeometry(t *testing.T) {
	s := nonFiniteSketch(t)
	out, err := s.DXF()
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, out)
}

// TestPNGRefusesNonFiniteGeometry doubles as a no-panic regression: before the
// fix, renderBounds' "w <= 0 -> 1" clamp never fires against NaN (every
// comparison against NaN is false), so a NaN width reached
// image.NewNRGBA(image.Rect(...)) and panicked.
func TestPNGRefusesNonFiniteGeometry(t *testing.T) {
	s := nonFiniteSketch(t)
	var (
		data []byte
		err  error
	)
	require.NotPanics(t, func() {
		data, err = s.PNG()
	}, "a non-finite bound must be refused before it reaches image.Rect, never panic")
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Nil(t, data)
}

// TestExportersUnaffectedByHealthyGeometry pins that the refusal is scoped to
// a non-finite bounding box: the same shape with a finite knot vector renders
// normally through all three exporters.
func TestExportersUnaffectedByHealthyGeometry(t *testing.T) {
	s := healthySketch(t)
	svg, err := s.SVG()
	require.NoError(t, err)
	require.NotEmpty(t, svg)
	png, err := s.PNG()
	require.NoError(t, err)
	require.NotEmpty(t, png)
	dxf, err := s.DXF()
	require.NoError(t, err)
	require.NotEmpty(t, dxf)
}
