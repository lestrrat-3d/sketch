package sketch

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	"github.com/lestrrat-go/option/v3"
)

// PNGOption configures [Sketch.PNG] rendering. Construct values with the With…
// helpers; any option left unset falls back to a sensible default. The shared
// style options ([WithMargin], [WithStroke], …) are [SVGPNGOption] values and
// are accepted here too.
type PNGOption interface {
	option.Interface
	pngOption()
}

type pngOption struct{ option.Interface }

func (pngOption) pngOption() {}

type identScale struct{}

// WithScale sets the raster resolution in pixels per sketch unit. When unset
// (or non-positive) the scale is chosen so the drawing's long side, margins
// included, fits 1024 pixels.
func WithScale(v float64) PNGOption { return pngOption{option.New(identScale{}, v)} }

// pngConfig holds the resolved PNG rendering options: the shared style options
// plus the raster scale.
type pngConfig struct {
	svgConfig
	scale float64 // pixels per sketch unit; <= 0 means fit the long side to 1024 px
}

func defaultPNGConfig() pngConfig {
	return pngConfig{svgConfig: defaultSVGConfig()}
}

// pngFitLongSide is the pixel size the drawing's long side is fitted to when
// no explicit [WithScale] is given.
const pngFitLongSide = 1024

// PNG renders the sketch to a PNG image and returns the encoded bytes. The
// output is visually equivalent to [Sketch.SVG] rendered at the same size:
// the y-axis is flipped to math orientation, geometry uses the stroke color,
// construction geometry the construction color (drawn solid, without the SVG
// dash pattern), and points are drawn as markers when enabled. Style options
// are interpreted in sketch units and scaled by the raster resolution (see
// [WithScale]).
//
// Colors accept "#rgb"/"#rrggbb" hex plus the names "white" and "black";
// "none" (or an empty string) as the background renders transparent pixels.
func (s *Sketch) PNG(options ...PNGOption) ([]byte, error) {
	cfg := defaultPNGConfig()
	for _, o := range options {
		if applyRenderOption(&cfg.svgConfig, o) {
			continue
		}
		switch o.Ident().(type) {
		case identScale:
			cfg.scale = option.MustGet[float64](o)
		}
	}

	b, w, h := s.renderBounds(cfg.margin)
	scale := cfg.scale
	if scale <= 0 {
		scale = pngFitLongSide / math.Max(w, h)
	}
	pw := int(math.Max(1, math.Round(w*scale)))
	ph := int(math.Max(1, math.Round(h*scale)))

	background, err := parseRenderColor(cfg.background)
	if err != nil {
		return nil, fmt.Errorf("sketch: background: %w", err)
	}
	stroke, err := parseRenderColor(cfg.stroke)
	if err != nil {
		return nil, fmt.Errorf("sketch: stroke: %w", err)
	}
	construction, err := parseRenderColor(cfg.construction)
	if err != nil {
		return nil, fmt.Errorf("sketch: construction: %w", err)
	}
	reference, err := parseRenderColor(cfg.reference)
	if err != nil {
		return nil, fmt.Errorf("sketch: reference: %w", err)
	}

	img := image.NewNRGBA(image.Rect(0, 0, pw, ph))
	if background.A > 0 {
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i+0] = background.R
			img.Pix[i+1] = background.G
			img.Pix[i+2] = background.B
			img.Pix[i+3] = background.A
		}
	}
	r := raster{img: img}

	// Map sketch coords to pixel coords (flip y), like SVG's tx/ty but scaled.
	px := func(x float64) float64 { return (x - b.minX + cfg.margin) * scale }
	py := func(y float64) float64 { return (b.maxY - y + cfg.margin) * scale }
	toPixels := func(pts [][2]float64) [][2]float64 {
		out := make([][2]float64, len(pts))
		for i, p := range pts {
			out[i] = [2]float64{px(p[0]), py(p[1])}
		}
		return out
	}

	width := cfg.strokeWidth * scale
	for _, e := range s.ents {
		col := stroke
		switch {
		case e.IsReference():
			col = reference
		case e.IsConstruction():
			col = construction
		}
		switch t := e.(type) {
		case *Line:
			r.strokePolyline(toPixels([][2]float64{
				{t.Start.x(), t.Start.y()},
				{t.End.x(), t.End.y()},
			}), width, col)
		case *Circle:
			r.strokePolyline(toPixels(circlePolyline(t, cfg.arcSegments)), width, col)
		case *Arc:
			r.strokePolyline(toPixels(arcPolyline(t, cfg.arcSegments)), width, col)
		case *EllipticalArc:
			r.strokePolyline(toPixels(ellipticalArcPolyline(t, cfg.arcSegments)), width, col)
		case *Ellipse:
			r.strokePolyline(toPixels(ellipsePolyline(t, cfg.arcSegments)), width, col)
		case *Spline:
			r.strokePolyline(toPixels(t.Polyline(cfg.arcSegments)), width, col)
		case *ClosedSpline:
			r.strokePolyline(toPixels(t.Polyline(cfg.arcSegments)), width, col)
		case *FitSpline:
			r.strokePolyline(toPixels(t.Polyline(cfg.arcSegments)), width, col)
		case *Conic:
			r.strokePolyline(toPixels(t.Polyline(cfg.arcSegments)), width, col)
		case *NURBS:
			r.strokePolyline(toPixels(t.Polyline(cfg.arcSegments)), width, col)
		}
	}

	if cfg.showPoints {
		free, err := parseRenderColor("#d93025")
		if err != nil {
			return nil, err
		}
		fixedCol, err := parseRenderColor("#202124")
		if err != nil {
			return nil, err
		}
		for _, p := range s.points {
			col := free
			if p.IsFixed() {
				col = fixedCol
			}
			r.fillDisc(px(p.x()), py(p.y()), cfg.pointRadius*scale, col)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("sketch: failed to encode PNG: %w", err)
	}
	return buf.Bytes(), nil
}

// circlePolyline samples the full circle counter-clockwise.
// circlePolyline and ellipsePolyline sample for rasterizing. The sampling math
// lives in geom (geom/sample.go) so the exporters and the world-space sampler
// agree exactly.
func circlePolyline(c *Circle, segments int) [][2]float64 {
	return c.Geometry().Polyline(segments)
}

func ellipsePolyline(e *Ellipse, segments int) [][2]float64 {
	return e.Geometry().Polyline(segments)
}

// parseRenderColor resolves the color strings the renderers accept: #rgb and
// #rrggbb hex, the names "white" and "black", and "none"/"" for transparent.
func parseRenderColor(s string) (color.NRGBA, error) {
	switch strings.ToLower(s) {
	case "", "none", "transparent":
		return color.NRGBA{}, nil
	case "white":
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, nil
	case "black":
		return color.NRGBA{A: 0xff}, nil
	}
	hex := strings.TrimPrefix(s, "#")
	if len(hex) == len(s) { // no "#" prefix
		return color.NRGBA{}, fmt.Errorf("unsupported color %q", s)
	}
	nib := func(c byte) (uint8, bool) {
		switch {
		case c >= '0' && c <= '9':
			return c - '0', true
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10, true
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10, true
		}
		return 0, false
	}
	var v [6]uint8
	switch len(hex) {
	case 3:
		for i := 0; i < 3; i++ {
			n, ok := nib(hex[i])
			if !ok {
				return color.NRGBA{}, fmt.Errorf("invalid hex color %q", s)
			}
			v[2*i], v[2*i+1] = n, n
		}
	case 6:
		for i := 0; i < 6; i++ {
			n, ok := nib(hex[i])
			if !ok {
				return color.NRGBA{}, fmt.Errorf("invalid hex color %q", s)
			}
			v[i] = n
		}
	default:
		return color.NRGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	return color.NRGBA{R: v[0]<<4 | v[1], G: v[2]<<4 | v[3], B: v[4]<<4 | v[5], A: 0xff}, nil
}

// raster draws antialiased strokes onto an NRGBA image by distance-to-shape
// coverage: a pixel's coverage falls off linearly within half a pixel of the
// stroke edge.
type raster struct{ img *image.NRGBA }

// strokePolyline strokes consecutive segments of pts at the given width.
func (r *raster) strokePolyline(pts [][2]float64, width float64, col color.NRGBA) {
	half := width / 2
	if half < 0.5 {
		half = 0.5 // keep hairlines visible at small scales
	}
	for i := 0; i+1 < len(pts); i++ {
		r.strokeSegment(pts[i], pts[i+1], half, col)
	}
}

func (r *raster) strokeSegment(a, b [2]float64, half float64, col color.NRGBA) {
	r.cover(
		math.Min(a[0], b[0])-half, math.Min(a[1], b[1])-half,
		math.Max(a[0], b[0])+half, math.Max(a[1], b[1])+half,
		col,
		func(x, y float64) float64 { return half + 0.5 - distToSegment(x, y, a, b) },
	)
}

// fillDisc fills a disc of the given radius centered at (cx, cy).
func (r *raster) fillDisc(cx, cy, rad float64, col color.NRGBA) {
	r.cover(cx-rad, cy-rad, cx+rad, cy+rad, col,
		func(x, y float64) float64 { return rad + 0.5 - math.Hypot(x-cx, y-cy) },
	)
}

// cover walks the pixels of a bounding box and blends col into each at the
// coverage reported by cov (clamped to [0, 1]) for the pixel center.
func (r *raster) cover(minX, minY, maxX, maxY float64, col color.NRGBA, cov func(x, y float64) float64) {
	bounds := r.img.Bounds()
	x0 := int(math.Max(math.Floor(minX-1), float64(bounds.Min.X)))
	y0 := int(math.Max(math.Floor(minY-1), float64(bounds.Min.Y)))
	x1 := int(math.Min(math.Ceil(maxX+1), float64(bounds.Max.X-1)))
	y1 := int(math.Min(math.Ceil(maxY+1), float64(bounds.Max.Y-1)))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			c := cov(float64(x)+0.5, float64(y)+0.5)
			if c <= 0 {
				continue
			}
			if c > 1 {
				c = 1
			}
			r.blend(x, y, col, c)
		}
	}
}

// blend composites col over the pixel at (x, y) with the given coverage
// (straight-alpha source-over).
func (r *raster) blend(x, y int, col color.NRGBA, cov float64) {
	dst := r.img.NRGBAAt(x, y)
	sa := cov * float64(col.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		r.img.SetNRGBA(x, y, color.NRGBA{})
		return
	}
	mix := func(s, d uint8) uint8 {
		v := (float64(s)*sa + float64(d)*da*(1-sa)) / outA
		return uint8(v + 0.5)
	}
	r.img.SetNRGBA(x, y, color.NRGBA{
		R: mix(col.R, dst.R),
		G: mix(col.G, dst.G),
		B: mix(col.B, dst.B),
		A: uint8(outA*255 + 0.5),
	})
}

// distToSegment is the distance from (px, py) to the segment a–b.
func distToSegment(px, py float64, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	dd := dx*dx + dy*dy
	if dd == 0 {
		return math.Hypot(px-a[0], py-a[1])
	}
	t := ((px-a[0])*dx + (py-a[1])*dy) / dd
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(a[0]+t*dx), py-(a[1]+t*dy))
}
