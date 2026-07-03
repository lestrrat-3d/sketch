package sketch_test

import (
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// framedSquare builds a solved 100x100 square grounded at the origin.
func framedSquare(t *testing.T) *sketch.Sketch {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(100, 0)
	c := s.CreatePoint(100, 100)
	d := s.CreatePoint(0, 100)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	cd := s.CreateLine(c, d)
	da := s.CreateLine(d, a)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(
		sketch.NewHorizontal(ab), sketch.NewHorizontal(cd),
		sketch.NewVertical(bc), sketch.NewVertical(da),
		sketch.NewDistance(a, b, 100), sketch.NewDistance(a, d, 100),
	)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	return s
}

func TestFrameDefaultOffByteIdentical(t *testing.T) {
	s := framedSquare(t)
	base, err := s.SVG()
	require.NoError(t, err)
	for _, opt := range []sketch.SVGOption{
		sketch.WithFrame(false),
		sketch.WithGrid(false),
	} {
		out, err := s.SVG(opt)
		require.NoError(t, err)
		require.Equal(t, base, out)
	}
}

func TestFrameDrawsBorderAndPads(t *testing.T) {
	s := framedSquare(t)
	base, err := s.SVG()
	require.NoError(t, err)
	framed, err := s.SVG(sketch.WithFrame(true))
	require.NoError(t, err)

	// A border rectangle is drawn.
	require.Contains(t, framed, `stroke="#90a4ae"`)
	require.Regexp(t, `<rect x="[0-9.]+" y="[0-9.]+" width="[0-9.]+" height="[0-9.]+" fill="none" stroke="#90a4ae"`, framed)
	// The canvas grew (outer padding added), so the viewBox differs from baseline.
	require.NotEqual(t, viewBox(t, base), viewBox(t, framed))
}

func TestGridImpliesFrameAndDrawsLines(t *testing.T) {
	s := framedSquare(t)
	out, err := s.SVG(sketch.WithGrid(true))
	require.NoError(t, err)
	require.Contains(t, out, `stroke="#90a4ae"`, "grid implies a frame border")
	require.Contains(t, out, `stroke="#eceff1"`, "minor grid lines")
	require.Contains(t, out, `stroke="#cfd8dc"`, "emphasized origin axis line")
}

func TestGridSpacingControlsLineCount(t *testing.T) {
	s := framedSquare(t)
	coarse, err := s.SVG(sketch.WithGrid(true), sketch.WithGridSpacing(50))
	require.NoError(t, err)
	fine, err := s.SVG(sketch.WithGrid(true), sketch.WithGridSpacing(10))
	require.NoError(t, err)
	require.Greater(t, strings.Count(fine, "<line"), strings.Count(coarse, "<line"))
	require.NotContains(t, fine, "NaN")
}

func TestWatermarkDrawn(t *testing.T) {
	// A framed render always carries the fixed provenance watermark — no option.
	s := framedSquare(t)
	out, err := s.SVG(sketch.WithFrame(true))
	require.NoError(t, err)
	require.Contains(t, out, sketch.WatermarkText)
	require.Contains(t, out, `fill="#b0b3b8"`, "watermark tint")

	// An unframed render has no watermark (nowhere to put it, baseline stays clean).
	plain, err := s.SVG()
	require.NoError(t, err)
	require.NotContains(t, plain, sketch.WatermarkText)
}

func viewBox(t *testing.T, svg string) string {
	t.Helper()
	const key = `viewBox="`
	i := strings.Index(svg, key)
	require.GreaterOrEqual(t, i, 0)
	rest := svg[i+len(key):]
	j := strings.IndexByte(rest, '"')
	require.GreaterOrEqual(t, j, 0)
	return rest[:j]
}
