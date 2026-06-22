package sketch

import (
	"fmt"
	"math"
	"strings"

	"github.com/lestrrat-go/option/v3"
)

// SVGOption configures [Sketch.SVG] rendering. Construct values with the With…
// helpers; any option left unset falls back to a sensible default.
type SVGOption interface {
	option.Interface
	svgOption()
}

// SVGPNGOption is a rendering option accepted by both [Sketch.SVG] and
// [Sketch.PNG]: it satisfies [SVGOption] and [PNGOption] simultaneously, so
// one constructed value can be passed to either exporter. All the shared
// style options (margin, stroke width, colors, …) return it.
type SVGPNGOption interface {
	option.Interface
	svgOption()
	pngOption()
}

type svgPNGOption struct{ option.Interface }

func (svgPNGOption) svgOption() {}
func (svgPNGOption) pngOption() {}

type (
	identMargin       struct{}
	identStrokeWidth  struct{}
	identShowPoints   struct{}
	identPointRadius  struct{}
	identArcSegments  struct{}
	identBackground   struct{}
	identStroke       struct{}
	identConstruction struct{}
	identReference    struct{}
)

// WithMargin sets the blank border around the geometry, in sketch units.
func WithMargin(v float64) SVGPNGOption { return svgPNGOption{option.New(identMargin{}, v)} }

// WithStrokeWidth sets the stroke width, in sketch units.
func WithStrokeWidth(v float64) SVGPNGOption {
	return svgPNGOption{option.New(identStrokeWidth{}, v)}
}

// WithShowPoints toggles drawing a small marker at each point.
func WithShowPoints(v bool) SVGPNGOption { return svgPNGOption{option.New(identShowPoints{}, v)} }

// WithPointRadius sets the point-marker radius, in sketch units.
func WithPointRadius(v float64) SVGPNGOption {
	return svgPNGOption{option.New(identPointRadius{}, v)}
}

// WithArcSegments sets the number of polyline segments used to approximate an arc.
func WithArcSegments(v int) SVGPNGOption { return svgPNGOption{option.New(identArcSegments{}, v)} }

// WithBackground sets the background fill (e.g. "white"); empty or "none" for
// no background ([Sketch.PNG] renders it transparent).
func WithBackground(v string) SVGPNGOption { return svgPNGOption{option.New(identBackground{}, v)} }

// WithStroke sets the geometry color.
func WithStroke(v string) SVGPNGOption { return svgPNGOption{option.New(identStroke{}, v)} }

// WithConstruction sets the construction-geometry color.
func WithConstruction(v string) SVGPNGOption {
	return svgPNGOption{option.New(identConstruction{}, v)}
}

// WithReference sets the reference-geometry (externally-locked snapshot) color.
func WithReference(v string) SVGPNGOption {
	return svgPNGOption{option.New(identReference{}, v)}
}

// svgConfig holds the resolved SVG rendering options.
type svgConfig struct {
	margin       float64
	strokeWidth  float64
	showPoints   bool
	pointRadius  float64
	arcSegments  int
	background   string
	stroke       string
	construction string
	reference    string
}

func defaultSVGConfig() svgConfig {
	return svgConfig{
		margin:       10,
		strokeWidth:  1,
		showPoints:   true,
		pointRadius:  2,
		arcSegments:  64,
		background:   "white",
		stroke:       "#1a73e8",
		construction: "#bbbbbb",
		reference:    "#e8731a",
	}
}

// applyRenderOption folds one shared rendering option into cfg, reporting
// whether the option was recognized. [Sketch.SVG] and [Sketch.PNG] both
// resolve their option lists through it.
func applyRenderOption(cfg *svgConfig, o option.Interface) bool {
	switch o.Ident().(type) {
	case identMargin:
		cfg.margin = option.MustGet[float64](o)
	case identStrokeWidth:
		cfg.strokeWidth = option.MustGet[float64](o)
	case identShowPoints:
		cfg.showPoints = option.MustGet[bool](o)
	case identPointRadius:
		cfg.pointRadius = option.MustGet[float64](o)
	case identArcSegments:
		cfg.arcSegments = option.MustGet[int](o)
	case identBackground:
		cfg.background = option.MustGet[string](o)
	case identStroke:
		cfg.stroke = option.MustGet[string](o)
	case identConstruction:
		cfg.construction = option.MustGet[string](o)
	case identReference:
		cfg.reference = option.MustGet[string](o)
	default:
		return false
	}
	return true
}

type bbox struct{ minX, minY, maxX, maxY float64 }

func (b *bbox) add(x, y float64) {
	b.minX, b.minY = math.Min(b.minX, x), math.Min(b.minY, y)
	b.maxX, b.maxY = math.Max(b.maxX, x), math.Max(b.maxY, y)
}

// bounds returns the axis-aligned bounding box of all geometry. ok is false
// when the sketch has nothing to draw.
func (s *Sketch) bounds() (bbox, bool) {
	b := bbox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	any := false
	for _, p := range s.points {
		b.add(p.x(), p.y())
		any = true
	}
	for _, e := range s.ents {
		switch t := e.(type) {
		case *Circle:
			b.add(t.Center.x()-t.r(), t.Center.y()-t.r())
			b.add(t.Center.x()+t.r(), t.Center.y()+t.r())
			any = true
		case *Arc:
			for _, p := range arcPolyline(t, 32) {
				b.add(p[0], p[1])
			}
			any = true
		case *EllipticalArc:
			for _, p := range ellipticalArcPolyline(t, 32) {
				b.add(p[0], p[1])
			}
			any = true
		case *Ellipse:
			// Axis-aligned extents of a rotated ellipse.
			cosr, sinr := math.Cos(t.rot()), math.Sin(t.rot())
			ex := math.Hypot(t.rx()*cosr, t.ry()*sinr)
			ey := math.Hypot(t.rx()*sinr, t.ry()*cosr)
			b.add(t.Center.x()-ex, t.Center.y()-ey)
			b.add(t.Center.x()+ex, t.Center.y()+ey)
			any = true
		case *Spline:
			for _, p := range t.Polyline(32) {
				b.add(p[0], p[1])
			}
			any = true
		case *ClosedSpline:
			for _, p := range t.Polyline(32) {
				b.add(p[0], p[1])
			}
			any = true
		case *FitSpline:
			for _, p := range t.Polyline(32) {
				b.add(p[0], p[1])
			}
			any = true
		case *Conic:
			for _, p := range t.Polyline(32) {
				b.add(p[0], p[1])
			}
			any = true
		case *NURBS:
			for _, p := range t.Polyline(32) {
				b.add(p[0], p[1])
			}
			any = true
		}
	}
	return b, any
}

// arcPolyline samples the arc counter-clockwise from start to end.
// arcPolyline samples an arc for rendering. The sampling math lives in geom
// (geom/sample.go) so the exporters and the world-space sampler agree exactly.
func arcPolyline(a *Arc, segments int) [][2]float64 {
	return a.Geometry().Polyline(segments)
}

// ellipticalArcPolyline samples an elliptical arc for rendering.
func ellipticalArcPolyline(e *EllipticalArc, segments int) [][2]float64 {
	return e.Geometry().Polyline(segments)
}

// SVG renders the sketch to an SVG document. The y-axis is flipped so the
// output matches conventional math orientation (y up). Called with no options
// it uses sensible defaults; override individual settings with the With…
// helpers.
func (s *Sketch) SVG(options ...SVGOption) (string, error) {
	cfg := defaultSVGConfig()
	for _, o := range options {
		applyRenderOption(&cfg, o)
	}

	b, ok := s.bounds()
	if !ok {
		b = bbox{0, 0, 1, 1}
	}
	w := (b.maxX - b.minX) + 2*cfg.margin
	h := (b.maxY - b.minY) + 2*cfg.margin
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}

	// Map sketch coords to SVG coords (flip y).
	tx := func(x float64) float64 { return x - b.minX + cfg.margin }
	ty := func(y float64) float64 { return b.maxY - y + cfg.margin }

	var sb strings.Builder
	fmt.Fprintf(&sb,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s">`,
		f(w), f(h), f(w), f(h))
	sb.WriteByte('\n')
	if cfg.background != "" {
		fmt.Fprintf(&sb, `  <rect width="100%%" height="100%%" fill="%s"/>`+"\n", cfg.background)
	}

	color := func(e Entity) string {
		switch {
		case e.IsReference():
			return cfg.reference
		case e.IsConstruction():
			return cfg.construction
		}
		return cfg.stroke
	}
	dash := func(e Entity) string {
		if e.IsConstruction() { // reference geometry renders solid, like real geometry
			return fmt.Sprintf(` stroke-dasharray="%s,%s"`, f(cfg.strokeWidth*4), f(cfg.strokeWidth*3))
		}
		return ""
	}

	for _, e := range s.ents {
		switch t := e.(type) {
		case *Line:
			fmt.Fprintf(&sb,
				`  <line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"%s/>`+"\n",
				f(tx(t.Start.x())), f(ty(t.Start.y())), f(tx(t.End.x())), f(ty(t.End.y())),
				color(t), f(cfg.strokeWidth), dash(t))
		case *Circle:
			fmt.Fprintf(&sb,
				`  <circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				f(tx(t.Center.x())), f(ty(t.Center.y())), f(t.r()),
				color(t), f(cfg.strokeWidth), dash(t))
		case *Arc:
			pts := arcPolyline(t, cfg.arcSegments)
			var d strings.Builder
			for i, p := range pts {
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%s %s ", cmd, f(tx(p[0])), f(ty(p[1])))
			}
			fmt.Fprintf(&sb,
				`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				strings.TrimSpace(d.String()), color(t), f(cfg.strokeWidth), dash(t))
		case *EllipticalArc:
			pts := ellipticalArcPolyline(t, cfg.arcSegments)
			var d strings.Builder
			for i, p := range pts {
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%s %s ", cmd, f(tx(p[0])), f(ty(p[1])))
			}
			fmt.Fprintf(&sb,
				`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				strings.TrimSpace(d.String()), color(t), f(cfg.strokeWidth), dash(t))
		case *Ellipse:
			// The y-flip mirrors the plane, so a CCW sketch rotation becomes
			// CW in SVG coordinates: negate the angle.
			cx, cy := tx(t.Center.x()), ty(t.Center.y())
			fmt.Fprintf(&sb,
				`  <ellipse cx="%s" cy="%s" rx="%s" ry="%s" transform="rotate(%s %s %s)" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				f(cx), f(cy), f(t.rx()), f(t.ry()),
				f(-t.rot()*180/math.Pi), f(cx), f(cy),
				color(t), f(cfg.strokeWidth), dash(t))
		case *Spline:
			// Sampled polyline, like arcs; cfg.arcSegments governs fidelity.
			pts := t.Polyline(cfg.arcSegments)
			var d strings.Builder
			for i, p := range pts {
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%s %s ", cmd, f(tx(p[0])), f(ty(p[1])))
			}
			fmt.Fprintf(&sb,
				`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				strings.TrimSpace(d.String()), color(t), f(cfg.strokeWidth), dash(t))
		case *ClosedSpline:
			// The sampled ring already closes (last point == first), so the same
			// M/L path draws a closed loop.
			pts := t.Polyline(cfg.arcSegments)
			var d strings.Builder
			for i, p := range pts {
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%s %s ", cmd, f(tx(p[0])), f(ty(p[1])))
			}
			fmt.Fprintf(&sb,
				`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				strings.TrimSpace(d.String()), color(t), f(cfg.strokeWidth), dash(t))
		case *FitSpline:
			// Sampled interpolating polyline through the fit points.
			pts := t.Polyline(cfg.arcSegments)
			var d strings.Builder
			for i, p := range pts {
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%s %s ", cmd, f(tx(p[0])), f(ty(p[1])))
			}
			fmt.Fprintf(&sb,
				`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				strings.TrimSpace(d.String()), color(t), f(cfg.strokeWidth), dash(t))
		case *Conic:
			// Sampled polyline, like arcs/splines; cfg.arcSegments governs fidelity.
			pts := t.Polyline(cfg.arcSegments)
			var d strings.Builder
			for i, p := range pts {
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%s %s ", cmd, f(tx(p[0])), f(ty(p[1])))
			}
			fmt.Fprintf(&sb,
				`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				strings.TrimSpace(d.String()), color(t), f(cfg.strokeWidth), dash(t))
		case *NURBS:
			// Sampled polyline, like the spline/conic; cfg.arcSegments governs fidelity.
			pts := t.Polyline(cfg.arcSegments)
			var d strings.Builder
			for i, p := range pts {
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%s %s ", cmd, f(tx(p[0])), f(ty(p[1])))
			}
			fmt.Fprintf(&sb,
				`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				strings.TrimSpace(d.String()), color(t), f(cfg.strokeWidth), dash(t))
		}
	}

	if cfg.showPoints {
		for _, p := range s.points {
			fill := "#d93025"
			if p.IsFixed() {
				fill = "#202124"
			}
			fmt.Fprintf(&sb,
				`  <circle cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n",
				f(tx(p.x())), f(ty(p.y())), f(cfg.pointRadius), fill)
		}
	}

	sb.WriteString("</svg>\n")
	return sb.String(), nil
}

// f formats a float compactly without a trailing ".000000".
func f(v float64) string { return trimFloat(v, 4) }

// trimFloat formats v with prec decimals and drops trailing zeros (and a bare
// trailing decimal point).
func trimFloat(v float64, prec int) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.*f", prec, v), "0"), ".")
}
