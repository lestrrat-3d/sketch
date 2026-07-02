package main

// banner renders the project's masthead: the word "sketch" drawn large with a
// smaller "A parametric 2D sketch engine" tagline beneath it — every glyph
// authored as ordinary sketch geometry (straight strokes as lines, curved
// strokes as fit splines) so the whole image is literally drawn by the engine.
//
// Glyphs live in a compact single-stroke font (bannerFont) on a font-unit grid:
// baseline y=0, x-height y=8, cap/ascender y=12, descender y=-3. drawText
// typesets a string by placing each glyph's strokes at an advancing x.

import "github.com/lestrrat-3d/sketch"

// glyphStroke is one pen stroke: a polyline of font-unit points, drawn as a
// smooth fit spline when curved (and long enough) else as straight line
// segments.
type glyphStroke struct {
	pts    [][2]float64
	curved bool
}

// glyph is a single character: its strokes plus the horizontal advance to the
// next character, in font units.
type glyph struct {
	strokes []glyphStroke
	adv     float64
}

func poly(pts ...[2]float64) glyphStroke  { return glyphStroke{pts: pts} }
func curve(pts ...[2]float64) glyphStroke { return glyphStroke{pts: pts, curved: true} }

// bannerFont covers exactly the characters used by the masthead. It is a
// deliberately minimal monoline face — enough to render the wordmark and
// tagline, not a general typeface.
var bannerFont = map[rune]glyph{
	' ': {adv: 4},

	// Lowercase (x-height 8).
	's': {adv: 7, strokes: []glyphStroke{
		curve([2]float64{6.0, 6.4}, [2]float64{4.2, 8.0}, [2]float64{2.1, 7.8}, [2]float64{1.0, 6.3}, [2]float64{2.2, 4.9}, [2]float64{4.6, 4.2}, [2]float64{5.7, 2.7}, [2]float64{4.6, 0.2}, [2]float64{2.0, 0.0}, [2]float64{0.7, 1.4}),
	}},
	'k': {adv: 7, strokes: []glyphStroke{
		poly([2]float64{0.7, 12.0}, [2]float64{0.7, 0.0}),
		poly([2]float64{5.8, 8.0}, [2]float64{0.7, 3.2}),
		poly([2]float64{2.4, 4.6}, [2]float64{6.0, 0.0}),
	}},
	'e': {adv: 7, strokes: []glyphStroke{
		poly([2]float64{0.9, 4.2}, [2]float64{5.9, 4.2}),
		curve([2]float64{5.9, 4.2}, [2]float64{6.0, 6.1}, [2]float64{4.5, 8.0}, [2]float64{2.3, 8.0}, [2]float64{0.8, 6.1}, [2]float64{0.8, 1.9}, [2]float64{2.3, 0.0}, [2]float64{4.6, 0.0}, [2]float64{6.0, 1.7}),
	}},
	't': {adv: 5, strokes: []glyphStroke{
		poly([2]float64{2.3, 11.0}, [2]float64{2.3, 1.6}, [2]float64{3.3, 0.2}, [2]float64{4.5, 0.8}),
		poly([2]float64{0.6, 8.0}, [2]float64{4.3, 8.0}),
	}},
	'c': {adv: 7, strokes: []glyphStroke{
		curve([2]float64{6.0, 6.1}, [2]float64{4.5, 8.0}, [2]float64{2.3, 8.0}, [2]float64{0.8, 6.2}, [2]float64{0.8, 1.8}, [2]float64{2.3, 0.0}, [2]float64{4.5, 0.0}, [2]float64{6.0, 1.9}),
	}},
	'h': {adv: 7, strokes: []glyphStroke{
		poly([2]float64{0.8, 12.0}, [2]float64{0.8, 0.0}),
		curve([2]float64{0.8, 5.6}, [2]float64{1.8, 7.5}, [2]float64{3.6, 8.0}, [2]float64{5.2, 7.2}, [2]float64{6.0, 5.3}, [2]float64{6.0, 0.0}),
	}},
	'a': {adv: 7, strokes: []glyphStroke{
		// Spine with a top-left entry arch, then the bowl in the lower half —
		// the arch is what distinguishes a two-story 'a' from a 'd'.
		curve([2]float64{1.3, 6.5}, [2]float64{2.7, 8.0}, [2]float64{4.6, 8.0}, [2]float64{6.0, 6.3}, [2]float64{6.0, 0.0}),
		curve([2]float64{6.0, 3.0}, [2]float64{4.5, 4.5}, [2]float64{2.6, 4.5}, [2]float64{1.0, 3.1}, [2]float64{1.0, 1.4}, [2]float64{2.6, 0.0}, [2]float64{4.5, 0.0}, [2]float64{6.0, 1.3}),
	}},
	'p': {adv: 7, strokes: []glyphStroke{
		poly([2]float64{0.8, 8.0}, [2]float64{0.8, -3.0}),
		curve([2]float64{0.8, 7.2}, [2]float64{2.4, 8.0}, [2]float64{4.4, 8.0}, [2]float64{6.0, 6.3}, [2]float64{6.0, 1.7}, [2]float64{4.4, 0.0}, [2]float64{2.4, 0.0}, [2]float64{0.8, 0.8}),
	}},
	'r': {adv: 5, strokes: []glyphStroke{
		poly([2]float64{0.8, 8.0}, [2]float64{0.8, 0.0}),
		curve([2]float64{0.8, 5.6}, [2]float64{1.8, 7.4}, [2]float64{3.4, 8.0}, [2]float64{4.8, 7.6}),
	}},
	'm': {adv: 10, strokes: []glyphStroke{
		poly([2]float64{0.8, 8.0}, [2]float64{0.8, 0.0}),
		curve([2]float64{0.8, 5.8}, [2]float64{1.6, 7.6}, [2]float64{3.0, 8.0}, [2]float64{4.2, 7.2}, [2]float64{4.6, 5.4}, [2]float64{4.6, 0.0}),
		curve([2]float64{4.6, 5.8}, [2]float64{5.4, 7.6}, [2]float64{6.8, 8.0}, [2]float64{8.0, 7.2}, [2]float64{8.4, 5.4}, [2]float64{8.4, 0.0}),
	}},
	'i': {adv: 3, strokes: []glyphStroke{
		poly([2]float64{1.3, 8.0}, [2]float64{1.3, 0.0}),
		poly([2]float64{1.3, 10.4}, [2]float64{1.3, 11.4}),
	}},
	'n': {adv: 7, strokes: []glyphStroke{
		poly([2]float64{0.8, 8.0}, [2]float64{0.8, 0.0}),
		curve([2]float64{0.8, 5.6}, [2]float64{1.8, 7.5}, [2]float64{3.6, 8.0}, [2]float64{5.2, 7.2}, [2]float64{6.0, 5.3}, [2]float64{6.0, 0.0}),
	}},
	'g': {adv: 7, strokes: []glyphStroke{
		curve([2]float64{6.0, 4.8}, [2]float64{4.4, 5.3}, [2]float64{2.3, 5.2}, [2]float64{0.9, 3.5}, [2]float64{0.9, 1.5}, [2]float64{2.3, 0.0}, [2]float64{4.4, 0.0}, [2]float64{6.0, 1.0}),
		curve([2]float64{6.0, 5.3}, [2]float64{6.0, -1.5}, [2]float64{4.8, -3.0}, [2]float64{2.6, -3.0}, [2]float64{1.4, -2.2}),
	}},

	// Uppercase (cap 12).
	'A': {adv: 9, strokes: []glyphStroke{
		poly([2]float64{0.5, 0.0}, [2]float64{4.5, 12.0}),
		poly([2]float64{4.5, 12.0}, [2]float64{8.5, 0.0}),
		poly([2]float64{1.9, 4.2}, [2]float64{7.1, 4.2}),
	}},
	'D': {adv: 9, strokes: []glyphStroke{
		poly([2]float64{1.0, 12.0}, [2]float64{1.0, 0.0}),
		curve([2]float64{1.0, 12.0}, [2]float64{5.0, 12.0}, [2]float64{7.8, 9.0}, [2]float64{7.8, 3.0}, [2]float64{5.0, 0.0}, [2]float64{1.0, 0.0}),
	}},

	// Digit.
	'2': {adv: 7, strokes: []glyphStroke{
		curve([2]float64{0.8, 9.2}, [2]float64{1.6, 11.0}, [2]float64{3.6, 12.0}, [2]float64{5.6, 11.2}, [2]float64{6.2, 9.0}, [2]float64{5.2, 6.8}, [2]float64{0.9, 0.0}),
		poly([2]float64{0.9, 0.0}, [2]float64{6.4, 0.0}),
	}},
}

// glyphFor returns the glyph for r, falling back to a space for any character
// outside the minimal font.
func glyphFor(r rune) glyph {
	if g, ok := bannerFont[r]; ok {
		return g
	}
	return bannerFont[' ']
}

// textWidth returns the typeset width of text in sketch units.
func textWidth(text string, scale, tracking float64) float64 {
	w, n := 0.0, 0
	for _, r := range text {
		w += glyphFor(r).adv * scale
		n++
	}
	if n > 1 {
		w += float64(n-1) * tracking * scale
	}
	return w
}

// drawText authors text into s as geometry, its left edge at x0 and baseline at
// baseline, scaled by scale with tracking (extra gap) between glyphs.
func drawText(s *sketch.Sketch, text string, x0, baseline, scale, tracking float64) error {
	x := x0
	for i, r := range text {
		if i > 0 {
			x += tracking * scale
		}
		g := glyphFor(r)
		for _, st := range g.strokes {
			pts := make([]*sketch.Point, len(st.pts))
			for j, p := range st.pts {
				pts[j] = s.CreatePoint(x+p[0]*scale, baseline+p[1]*scale)
			}
			if st.curved && len(pts) >= 3 {
				if _, err := s.CreateFitSpline(pts...); err != nil {
					return err
				}
				continue
			}
			for j := 0; j+1 < len(pts); j++ {
				s.CreateLine(pts[j], pts[j+1])
			}
		}
		x += g.adv * scale
	}
	return nil
}

// buildBannerSketch composes the masthead sketch: the wordmark centered above
// the smaller tagline, both centered on x=0.
func buildBannerSketch() (*sketch.Sketch, error) {
	const (
		wordScale = 1.0
		wordTrack = 2.5
		wordBase  = 8.0

		tagScale = 0.28
		tagTrack = 0.7
		tagBase  = 0.0
	)
	const (
		word = "sketch"
		tag  = "A parametric 2D sketch engine"
	)

	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	if err != nil {
		return nil, err
	}
	if err := drawText(s, word, -textWidth(word, wordScale, wordTrack)/2, wordBase, wordScale, wordTrack); err != nil {
		return nil, err
	}
	if err := drawText(s, tag, -textWidth(tag, tagScale, tagTrack)/2, tagBase, tagScale, tagTrack); err != nil {
		return nil, err
	}
	return s, nil
}

// bannerOptions is the shared render styling for the masthead: a clean stroke,
// no point markers, no frame.
var bannerOptions = []sketch.SVGOption{
	sketch.WithShowPoints(false),
	sketch.WithBackground("white"),
	sketch.WithStroke("#1a73e8"),
	sketch.WithStrokeWidth(0.5),
	sketch.WithMargin(3),
	sketch.WithPixelWidth(720),
}

func banner() (string, error) {
	s, err := buildBannerSketch()
	if err != nil {
		return "", err
	}
	return s.SVG(bannerOptions...)
}
