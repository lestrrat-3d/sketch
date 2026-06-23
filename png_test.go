package sketch_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// pngSquare builds a 10×10 square of lines with the top edge marked as
// construction geometry. No constraints are needed — exporters render the
// current coordinates.
func pngSquare(t *testing.T) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	c := s.AddPoint(10, 10)
	d := s.AddPoint(0, 10)
	s.AddLine(a, b)
	s.AddLine(b, c)
	top := s.AddLine(c, d)
	s.AddLine(d, a)
	top.SetConstruction(true)
	return s
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err, "PNG output must decode")
	return img
}

func at(img image.Image, x, y int) color.NRGBA {
	c, ok := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	if !ok {
		return color.NRGBA{}
	}
	return c
}

func TestPNG(t *testing.T) {
	t.Run("dimensions follow the scale", func(t *testing.T) {
		s := pngSquare(t)
		// Bounds 0..10 plus default margin 10 per side = 30 units; ×10 px/unit.
		data, err := s.PNG(sketch.WithScale(10))
		require.NoError(t, err, "render must succeed")
		img := decodePNG(t, data)
		require.Equal(t, 300, img.Bounds().Dx(), "30 units at 10 px/unit")
		require.Equal(t, 300, img.Bounds().Dy(), "30 units at 10 px/unit")
	})

	t.Run("default scale fits the long side to 1024", func(t *testing.T) {
		s := pngSquare(t)
		data, err := s.PNG()
		require.NoError(t, err, "render must succeed")
		img := decodePNG(t, data)
		require.Equal(t, 1024, img.Bounds().Dx(), "long side fitted")
		require.Equal(t, 1024, img.Bounds().Dy(), "square drawing stays square")
	})

	t.Run("background, stroke and construction colors land on pixels", func(t *testing.T) {
		s := pngSquare(t)
		data, err := s.PNG(sketch.WithScale(10))
		require.NoError(t, err, "render must succeed")
		img := decodePNG(t, data)

		white := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		require.Equal(t, white, at(img, 5, 5), "margin corner is background")
		// Bottom edge a–b runs at pixel row (10-0+10)*10 = 200, x 100..200.
		require.Equal(t, color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0xff}, at(img, 150, 200),
			"geometry midpoint uses the stroke color")
		// Construction top edge c–d runs at pixel row (10-10+10)*10 = 100.
		require.Equal(t, color.NRGBA{R: 0xbb, G: 0xbb, B: 0xbb, A: 0xff}, at(img, 150, 100),
			"construction geometry uses the construction color")
	})

	t.Run("none background renders transparent", func(t *testing.T) {
		s := pngSquare(t)
		data, err := s.PNG(sketch.WithScale(10), sketch.WithBackground("none"))
		require.NoError(t, err, "render must succeed")
		img := decodePNG(t, data)
		require.Equal(t, uint8(0), at(img, 5, 5).A, "margin pixel is fully transparent")
		require.Equal(t, uint8(0xff), at(img, 150, 200).A, "stroke pixel is opaque")
	})

	t.Run("point markers reflect fixed status", func(t *testing.T) {
		s := newSketch(t)
		free := s.AddPoint(0, 0)
		fixed := s.AddPoint(10, 0)
		s.AddLine(free, fixed)
		s.Fix(fixed)
		data, err := s.PNG(sketch.WithScale(10))
		require.NoError(t, err, "render must succeed")
		img := decodePNG(t, data)
		// Points sit at pixel (100,100) and (200,100); markers r = 20 px.
		require.Equal(t, color.NRGBA{R: 0xd9, G: 0x30, B: 0x25, A: 0xff}, at(img, 100, 95),
			"free point marker color")
		require.Equal(t, color.NRGBA{R: 0x20, G: 0x21, B: 0x24, A: 0xff}, at(img, 200, 95),
			"fixed point marker color")
	})

	t.Run("a shared option value flows into both exporters", func(t *testing.T) {
		s := pngSquare(t)
		bg := sketch.WithBackground("#00ff00")
		svg, err := s.SVG(bg)
		require.NoError(t, err, "SVG render must succeed")
		require.True(t, strings.Contains(svg, `fill="#00ff00"`), "SVG honours the shared background")
		data, err := s.PNG(bg, sketch.WithScale(10))
		require.NoError(t, err, "PNG render must succeed")
		img := decodePNG(t, data)
		require.Equal(t, color.NRGBA{G: 0xff, A: 0xff}, at(img, 5, 5), "PNG honours the shared background")
	})

	t.Run("short hex colors parse", func(t *testing.T) {
		s := pngSquare(t)
		data, err := s.PNG(sketch.WithScale(10), sketch.WithStroke("#f00"))
		require.NoError(t, err, "render must succeed")
		img := decodePNG(t, data)
		require.Equal(t, color.NRGBA{R: 0xff, A: 0xff}, at(img, 150, 200), "#f00 expands to #ff0000")
	})

	t.Run("unsupported colors are rejected", func(t *testing.T) {
		s := pngSquare(t)
		_, err := s.PNG(sketch.WithStroke("rebeccapurple"))
		require.Error(t, err, "named colors beyond white/black are not supported")
	})

	t.Run("empty sketch still renders", func(t *testing.T) {
		s := newSketch(t)
		data, err := s.PNG(sketch.WithScale(10))
		require.NoError(t, err, "render must succeed")
		img := decodePNG(t, data)
		require.Greater(t, img.Bounds().Dx(), 0, "fallback viewport is non-empty")
	})
}
