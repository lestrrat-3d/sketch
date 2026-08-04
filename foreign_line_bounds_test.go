package sketch_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// foreignLineSketch builds a sketch with an ordinary owned line plus a second
// line whose far endpoint is a *Point belonging to ANOTHER sketch, carrying
// endVal in both its coordinates. Sketch.bounds had no case for *Line at all
// — it leaned entirely on the s.points loop to cover a line's endpoints, and
// that loop never sees a point owned by a different sketch — so a line's
// entire geometry could sit outside the box every exporter checks.
func foreignLineSketch(t *testing.T, endVal float64) (*sketch.Sketch, *sketch.Point) {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	other, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	s.CreateLine(a, s.CreatePoint(10, 0)) // ordinary owned geometry, so the sketch is not empty
	foreign := other.CreatePoint(endVal, endVal)
	s.CreateLine(a, foreign)
	return s, foreign
}

// TestSVGDXFPNGRefuseForeignNonFiniteLineEndpoint covers the door the missing
// *Line case left open: a NaN or +Inf point from another sketch, reachable
// only through a line, poisoned no bounding box before the fix because
// nothing read the line's endpoints at all.
func TestSVGDXFPNGRefuseForeignNonFiniteLineEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  float64
	}{
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := foreignLineSketch(t, tc.val)

			_, err := s.SVG()
			require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)

			_, err = s.DXF()
			require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)

			var (
				data []byte
				perr error
			)
			require.NotPanics(t, func() {
				data, perr = s.PNG()
			}, "a non-finite bound reached only through a foreign line endpoint must be refused before it reaches image.Rect, never panic")
			require.ErrorIs(t, perr, sketch.ErrNonFiniteGeometry)
			require.Nil(t, data)
		})
	}
}

// TestSVGViewBoxCoversFiniteForeignLineEndpoint is the plain-correctness half
// of the bug: no NaN anywhere, but a line to a foreign, finite endpoint far
// outside the sketch's own geometry must still be inside the reported
// viewBox. Before the fix bounds() silently ignored the foreign endpoint, so
// the viewBox reported a box the drawn line reached a thousand units past.
func TestSVGViewBoxCoversFiniteForeignLineEndpoint(t *testing.T) {
	s, foreign := foreignLineSketch(t, 1000)

	svg, err := s.SVG()
	require.NoError(t, err)

	var minX, minY, w, h float64
	_, serr := fmt.Sscanf(viewBox(t, svg), "%g %g %g %g", &minX, &minY, &w, &h)
	require.NoError(t, serr)
	maxX, maxY := minX+w, minY+h

	require.LessOrEqualf(t, minX, foreign.X(), "viewBox %q does not reach the foreign endpoint (%g, %g)", viewBox(t, svg), foreign.X(), foreign.Y())
	require.LessOrEqualf(t, minY, foreign.Y(), "viewBox %q does not reach the foreign endpoint (%g, %g)", viewBox(t, svg), foreign.X(), foreign.Y())
	require.GreaterOrEqualf(t, maxX, foreign.X(), "viewBox %q does not reach the foreign endpoint (%g, %g)", viewBox(t, svg), foreign.X(), foreign.Y())
	require.GreaterOrEqualf(t, maxY, foreign.Y(), "viewBox %q does not reach the foreign endpoint (%g, %g)", viewBox(t, svg), foreign.X(), foreign.Y())
}

// TestSVGOwnedLineBoundsUnaffected is the control the fix must leave alone: a
// line whose two endpoints are both owned by the sketch was already fully
// covered by the s.points loop, so the new *Line case in bounds is redundant
// there and must not move the output.
func TestSVGOwnedLineBoundsUnaffected(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 10)
	s.CreateLine(a, b)

	svg, err := s.SVG()
	require.NoError(t, err)
	require.Equal(t, "0 0 30 30", viewBox(t, svg))
	require.Contains(t, svg, `<line x1="10" y1="20" x2="20" y2="10"`)
}

// TestSVGOriginDrawnLineBoundsCoversOrigin exercises the other point
// Sketch.bounds could miss for a *Line: Sketch.Origin() is deliberately
// absent from s.points (see CLAUDE.md), so a line drawn from it depended on
// the missing *Line case exactly as a foreign endpoint did. Before the fix
// the origin's (0,0) corner was silently dropped from the box; the new case
// reads a line's endpoints straight off the entity, owned or not.
func TestSVGOriginDrawnLineBoundsCoversOrigin(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(10, 10)
	s.CreateLine(s.Origin(), p)

	svg, err := s.SVG()
	require.NoError(t, err)
	require.Equal(t, "0 0 30 30", viewBox(t, svg), "the box must include the origin's (0,0) corner, not just p's")
}
