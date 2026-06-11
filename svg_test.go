package sketch_test

import (
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestSVGOptions exercises each SVG rendering option against the emitted
// document. TestSVGOutput covers the default rendering; these subtests pin
// that every With… option actually changes the output as documented.
func TestSVGOptions(t *testing.T) {
	newLineSketch := func() *sketch.Sketch {
		s := sketch.New()
		a := s.AddPoint(0, 0)
		b := s.AddPoint(10, 0)
		s.AddLine(a, b)
		return s
	}

	t.Run("show points", func(t *testing.T) {
		s := newLineSketch()
		on, err := s.SVG()
		require.NoError(t, err)
		require.Contains(t, on, "<circle", "default draws point markers")
		off, err := s.SVG(sketch.WithShowPoints(false))
		require.NoError(t, err)
		require.NotContains(t, off, "<circle", "markers suppressed")
	})
	t.Run("point radius", func(t *testing.T) {
		s := newLineSketch()
		svg, err := s.SVG(sketch.WithPointRadius(3.5))
		require.NoError(t, err)
		require.Contains(t, svg, `r="3.5"`, "marker radius applied")
	})
	t.Run("background", func(t *testing.T) {
		s := newLineSketch()
		svg, err := s.SVG(sketch.WithBackground("black"))
		require.NoError(t, err)
		require.Contains(t, svg, `fill="black"`, "background color applied")
		none, err := s.SVG(sketch.WithBackground(""))
		require.NoError(t, err)
		require.NotContains(t, none, "<rect", "no background rect when empty")
	})
	t.Run("stroke", func(t *testing.T) {
		s := newLineSketch()
		svg, err := s.SVG(sketch.WithStroke("#ff0000"))
		require.NoError(t, err)
		require.Contains(t, svg, `stroke="#ff0000"`, "stroke color applied")
	})
	t.Run("stroke width", func(t *testing.T) {
		s := newLineSketch()
		svg, err := s.SVG(sketch.WithStrokeWidth(2.5))
		require.NoError(t, err)
		require.Contains(t, svg, `stroke-width="2.5"`, "stroke width applied")
	})
	t.Run("margin", func(t *testing.T) {
		s := newLineSketch()
		svg, err := s.SVG(sketch.WithMargin(0))
		require.NoError(t, err)
		require.Contains(t, svg, `width="10"`, "zero margin leaves the raw bounds")
	})
	t.Run("arc segments", func(t *testing.T) {
		s := sketch.New()
		o := s.AddPoint(0, 0)
		st := s.AddPoint(5, 0)
		en := s.AddPoint(0, 5)
		s.AddArc(o, st, en)
		svg, err := s.SVG(sketch.WithArcSegments(4), sketch.WithShowPoints(false))
		require.NoError(t, err)
		require.Equal(t, 4, strings.Count(svg, "L"), "4 segments render as 4 line commands")
	})
	t.Run("construction color", func(t *testing.T) {
		s := sketch.New()
		a := s.AddPoint(0, 0)
		b := s.AddPoint(10, 0)
		s.AddLine(a, b).SetConstruction(true)
		svg, err := s.SVG(sketch.WithConstruction("#00ff00"), sketch.WithShowPoints(false))
		require.NoError(t, err)
		require.Contains(t, svg, `stroke="#00ff00"`, "construction color applied")
		require.Contains(t, svg, "stroke-dasharray", "construction renders dashed")
	})
}
