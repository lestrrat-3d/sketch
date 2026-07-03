package main

// banner renders the project's masthead: the word "sketch" drawn large with a
// smaller "A parametric 2D sketch engine" tagline beneath it — every glyph
// authored as ordinary sketch geometry (straight strokes as lines, curved
// strokes as fit splines) so the whole image is literally drawn by the engine.
//
// Glyphs live in a compact single-stroke font (bannerFont) on a font-unit grid:
// baseline y=0, x-height y=8, cap/ascender y=12, descender y=-3. drawText
// typesets a string by placing each glyph's strokes at an advancing x.

import (
	"math"

	"github.com/lestrrat-3d/sketch"
)

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

// strokeHandles are the sketch entities created for one pen stroke: its points
// in order, and its line segments (nil for a fit-spline stroke). They let the
// caller attach constraints to specific features (a stem, a crossbar).
type strokeHandles struct {
	pts   []*sketch.Point
	lines []*sketch.Line
}

// textHandles collects the geometry a drawText call produced: per-glyph strokes
// (in string order) and every point, so the caller can dimension the whole run.
type textHandles struct {
	glyphs [][]strokeHandles
	points []*sketch.Point
}

// drawText authors text into s as geometry, its left edge at x0 and baseline at
// baseline, scaled by scale with tracking (extra gap) between glyphs, returning
// handles to everything it created.
func drawText(s *sketch.Sketch, text string, x0, baseline, scale, tracking float64) (*textHandles, error) {
	h := &textHandles{}
	x := x0
	for i, r := range text {
		if i > 0 {
			x += tracking * scale
		}
		g := glyphFor(r)
		gh := make([]strokeHandles, 0, len(g.strokes))
		for _, st := range g.strokes {
			pts := make([]*sketch.Point, len(st.pts))
			for j, p := range st.pts {
				pts[j] = s.CreatePoint(x+p[0]*scale, baseline+p[1]*scale)
				h.points = append(h.points, pts[j])
			}
			sh := strokeHandles{pts: pts}
			if st.curved && len(pts) >= 3 {
				if _, err := s.CreateFitSpline(pts...); err != nil {
					return nil, err
				}
			} else {
				for j := 0; j+1 < len(pts); j++ {
					sh.lines = append(sh.lines, s.CreateLine(pts[j], pts[j+1]))
				}
			}
			gh = append(gh, sh)
		}
		h.glyphs = append(h.glyphs, gh)
		x += g.adv * scale
	}
	return h, nil
}

// buildBannerSketch composes the masthead sketch: the wordmark "sketch" —
// annotated like a real CAD sketch, with vertical/horizontal constraints on its
// stems and crossbars and overall width/height dimensions — centered above the
// smaller plain tagline, both centered on x=0.
func buildBannerSketch() (*sketch.Sketch, error) {
	const (
		wordScale = 1.0
		wordTrack = 2.5
		wordBase  = 8.0

		tagScale = 0.28
		tagTrack = 0.7
		tagBase  = -3.5 // clears the wordmark's overall-width dimension below it
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
	wm, err := drawText(s, word, -textWidth(word, wordScale, wordTrack)/2, wordBase, wordScale, wordTrack)
	if err != nil {
		return nil, err
	}
	if _, err := drawText(s, tag, -textWidth(tag, tagScale, tagTrack)/2, tagBase, tagScale, tagTrack); err != nil {
		return nil, err
	}
	annotateWordmark(s, wm)
	return s, nil
}

// annotateWordmark attaches the constraints and dimensions that make the "sketch"
// wordmark read as a constrained CAD sketch. It never solves — the letterforms
// stay exactly as drawn — so the constraints are the ones already satisfied by
// construction (vertical stems, horizontal crossbars) and the dimensions carry
// the measured extents.
//
// Glyph strokes (see bannerFont), indexed [glyph][stroke]:
//
//	s=0  k=1  e=2  t=3  c=4  h=5
//	k stroke0 = stem;         h stroke0 = stem
//	e stroke0 = crossbar;     t stroke0 = stem (seg 0 vertical), t stroke1 = crossbar
func annotateWordmark(s *sketch.Sketch, wm *textHandles) {
	kStem := wm.glyphs[1][0].lines[0]
	hStem := wm.glyphs[5][0].lines[0]
	eBar := wm.glyphs[2][0].lines[0]
	tStem := wm.glyphs[3][0].lines[0] // the vertical first segment
	tBar := wm.glyphs[3][1].lines[0]

	s.AddConstraint(
		sketch.NewVertical(kStem),
		sketch.NewVertical(hStem),
		sketch.NewVertical(tStem),
		sketch.NewHorizontal(eBar),
		sketch.NewHorizontal(tBar),
		sketch.NewEqual(kStem, hStem), // the two full-height stems match
	)

	// Overall dimensions across the wordmark's extent. The dimension endpoints
	// sit at the bounding corners (invisible — point markers are off), so the
	// dimensions read as the classic overall width/height of the "part".
	minX, minY, maxX, maxY := wm.points[0].X(), wm.points[0].Y(), wm.points[0].X(), wm.points[0].Y()
	for _, p := range wm.points {
		minX, maxX = math.Min(minX, p.X()), math.Max(maxX, p.X())
		minY, maxY = math.Min(minY, p.Y()), math.Max(maxY, p.Y())
	}
	bl := s.CreatePoint(minX, minY)
	br := s.CreatePoint(maxX, minY)
	tr := s.CreatePoint(maxX, maxY)
	s.AddConstraint(
		sketch.NewHorizontalDistance(bl, br, maxX-minX), // overall width, below
		sketch.NewVerticalDistance(br, tr, maxY-minY),   // overall height, right
	)
}

// bannerOptions is the shared render styling for the masthead: blue geometry
// with grey CAD annotations, no point markers (so the tagline stays clean), no
// frame.
var bannerOptions = []sketch.SVGOption{
	sketch.WithShowPoints(false),
	sketch.WithBackground("white"),
	sketch.WithStroke("#1a73e8"),
	sketch.WithStrokeWidth(0.5),
	sketch.WithDimensions(true),
	sketch.WithConstraints(true),
	sketch.WithMargin(4),
	sketch.WithPixelWidth(720),
}

func banner() (string, error) {
	s, err := buildBannerSketch()
	if err != nil {
		return "", err
	}
	return s.SVG(bannerOptions...)
}
