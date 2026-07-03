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

	// Annotation options (see annotate.go). All default off so the baseline
	// output is byte-identical.
	dimensions  bool    // draw dimensional constraints
	constraints bool    // draw geometric-constraint glyphs
	dofColoring bool    // color free vs. constrained geometry
	conflicts   bool    // highlight conflicting-constraint geometry
	statusBadge bool    // draw a verification status card
	profileFill bool    // fill valid closed regions
	annColor    string  // dimension line / glyph stroke color
	annScale    float64 // multiplies annotation glyph/text/arrow sizes
	pixelWidth  float64 // target display width in px (0 = geometry units); viewBox unchanged

	// Windowed framing (see frame.go). Default off keeps output byte-identical.
	frame       bool    // draw a border rectangle framing the sketch
	grid        bool    // draw a background grid inside the frame
	gridSpacing float64 // grid spacing in sketch units (0 = auto nice step)
	framePad    float64 // outer padding canvas edge -> frame (0 = auto)
	watermark   string  // provenance text along the frame's bottom edge
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
		annColor:     "#5f6368",
		annScale:     1,
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
	case identDimensions:
		cfg.dimensions = option.MustGet[bool](o)
	case identConstraints:
		cfg.constraints = option.MustGet[bool](o)
	case identDOFColoring:
		cfg.dofColoring = option.MustGet[bool](o)
	case identConflicts:
		cfg.conflicts = option.MustGet[bool](o)
	case identStatusBadge:
		cfg.statusBadge = option.MustGet[bool](o)
	case identProfileFill:
		cfg.profileFill = option.MustGet[bool](o)
	case identPixelWidth:
		cfg.pixelWidth = option.MustGet[float64](o)
	case identFrame:
		cfg.frame = option.MustGet[bool](o)
	case identGrid:
		cfg.grid = option.MustGet[bool](o)
	case identGridSpacing:
		cfg.gridSpacing = option.MustGet[float64](o)
	case identFramePad:
		cfg.framePad = option.MustGet[float64](o)
	case identWatermark:
		cfg.watermark = option.MustGet[string](o)
	case identAnnColor:
		cfg.annColor = option.MustGet[string](o)
	case identAnnScale:
		cfg.annScale = option.MustGet[float64](o)
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
	addAll := func(pts [][2]float64) {
		for _, p := range pts {
			b.add(p[0], p[1])
		}
		any = true
	}
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
			addAll(arcPolyline(t, 32))
		case *EllipticalArc:
			addAll(ellipticalArcPolyline(t, 32))
		case *Ellipse:
			// Axis-aligned extents of a rotated ellipse.
			cosr, sinr := math.Cos(t.rot()), math.Sin(t.rot())
			ex := math.Hypot(t.rx()*cosr, t.ry()*sinr)
			ey := math.Hypot(t.rx()*sinr, t.ry()*cosr)
			b.add(t.Center.x()-ex, t.Center.y()-ey)
			b.add(t.Center.x()+ex, t.Center.y()+ey)
			any = true
		case *Spline:
			addAll(t.Polyline(32))
		case *ClosedSpline:
			addAll(t.Polyline(32))
		case *FitSpline:
			addAll(t.Polyline(32))
		case *Conic:
			addAll(t.Polyline(32))
		case *NURBS:
			addAll(t.Polyline(32))
		}
	}
	return b, any
}

// renderBounds resolves the drawing's bounding box and the margin-padded
// width/height shared by the SVG and PNG exporters: an empty sketch falls back
// to a unit box, and a non-positive span is clamped to 1.
func (s *Sketch) renderBounds(margin float64) (bbox, float64, float64) {
	b, ok := s.bounds()
	if !ok {
		b = bbox{0, 0, 1, 1}
	}
	w := (b.maxX - b.minX) + 2*margin
	h := (b.maxY - b.minY) + 2*margin
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	return b, w, h
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

	b, w, h := s.renderBounds(cfg.margin)

	// Windowed framing adds an outer padding P around the margin-padded content;
	// the frame border sits at that boundary and the sketch's own margin becomes
	// the gap between the frame and the geometry (see frame.go). Off by default,
	// so pad is 0 and the layout is unchanged.
	pad := s.framePadding(cfg, w, h)
	canvasW, canvasH := w+2*pad, h+2*pad

	// Map sketch coords to SVG coords (flip y), shifted by the frame padding.
	tx := func(x float64) float64 { return x - b.minX + cfg.margin + pad }
	ty := func(y float64) float64 { return b.maxY - y + cfg.margin + pad }

	// Display size defaults to the geometry units, but WithPixelWidth decouples
	// it: the viewBox stays in geometry units while the root width/height are a
	// pixel size (aspect preserved), so the SVG scales up for embedding.
	outW, outH := canvasW, canvasH
	if cfg.pixelWidth > 0 {
		outW = cfg.pixelWidth
		outH = cfg.pixelWidth * canvasH / canvasW
	}

	var sb strings.Builder
	fmt.Fprintf(&sb,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s">`,
		f(outW), f(outH), f(canvasW), f(canvasH))
	sb.WriteByte('\n')
	if cfg.background != "" {
		fmt.Fprintf(&sb, `  <rect width="100%%" height="100%%" fill="%s"/>`+"\n", cfg.background)
	}

	// Grid renders behind everything (but inside the frame); the frame border is
	// drawn on top of the grid, before the geometry.
	if pad > 0 {
		s.writeFrameGrid(&sb, cfg, b, pad, w, h, tx, ty)
	}

	// Profile fill renders under the geometry, so it is emitted first.
	if cfg.profileFill {
		s.writeProfileFill(&sb, tx, ty)
	}

	ov := s.computeOverlaySets(cfg)
	color := func(e Entity) string {
		// Precedence: conflict-red > DOF-blue > reference/construction > default.
		if _, ok := ov.conf[e]; ok {
			return colorConflict
		}
		if _, ok := ov.freeEnt[e]; ok {
			return colorFree
		}
		switch {
		case e.IsReference():
			return cfg.reference
		case e.IsConstruction():
			return cfg.construction
		}
		if cfg.dofColoring { // fully constrained under DOF coloring reads black
			return colorConstrained
		}
		return cfg.stroke
	}
	dash := func(e Entity) string {
		if e.IsConstruction() { // reference geometry renders solid, like real geometry
			return fmt.Sprintf(` stroke-dasharray="%s,%s"`, f(cfg.strokeWidth*4), f(cfg.strokeWidth*3))
		}
		return ""
	}
	// writePath emits one M/L SVG <path> for a sampled curve (arc, elliptical
	// arc, every spline family, conic, NURBS); only the pts source differs
	// between the curve cases below.
	writePath := func(pts [][2]float64, stroke string, sw float64, dasharray string) {
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
			strings.TrimSpace(d.String()), stroke, f(sw), dasharray)
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
			writePath(arcPolyline(t, cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		case *EllipticalArc:
			writePath(ellipticalArcPolyline(t, cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		case *Ellipse:
			// The y-flip mirrors the plane, so a CCW sketch rotation becomes
			// CW in SVG coordinates: negate the angle.
			cx, cy := tx(t.Center.x()), ty(t.Center.y())
			fmt.Fprintf(&sb,
				`  <ellipse cx="%s" cy="%s" rx="%s" ry="%s" transform="rotate(%s %s %s)" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				f(cx), f(cy), f(t.rx()), f(t.ry()),
				f(-radToDeg(t.rot())), f(cx), f(cy),
				color(t), f(cfg.strokeWidth), dash(t))
		case *Spline:
			// Sampled polyline, like arcs; cfg.arcSegments governs fidelity.
			writePath(t.Polyline(cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		case *ClosedSpline:
			// The sampled ring already closes (last point == first), so the same
			// M/L path draws a closed loop.
			writePath(t.Polyline(cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		case *FitSpline:
			// Sampled interpolating polyline through the fit points.
			writePath(t.Polyline(cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		case *Conic:
			// Sampled polyline, like arcs/splines; cfg.arcSegments governs fidelity.
			writePath(t.Polyline(cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		case *NURBS:
			// Sampled polyline, like the spline/conic; cfg.arcSegments governs fidelity.
			writePath(t.Polyline(cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		}
	}

	if cfg.showPoints {
		for _, p := range s.points {
			if cfg.dofColoring {
				// Free points render hollow blue, grounded points a filled green
				// square, other constrained points a filled black circle — a
				// redundant shape channel so DOF/grounding reads without color.
				if _, free := ov.freePt[p]; free {
					fmt.Fprintf(&sb,
						`  <circle cx="%s" cy="%s" r="%s" fill="white" stroke="%s" stroke-width="%s"/>`+"\n",
						f(tx(p.x())), f(ty(p.y())), f(cfg.pointRadius), colorFree, f(cfg.strokeWidth))
					continue
				}
				if p.IsFixed() {
					// A square anchor marks the grounded point(s): the sketch's tie to
					// the origin, distinct from geometry constrained by other relations.
					side := cfg.pointRadius * 2
					fmt.Fprintf(&sb,
						`  <rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`+"\n",
						f(tx(p.x())-cfg.pointRadius), f(ty(p.y())-cfg.pointRadius), f(side), f(side), colorFixed)
					continue
				}
				fmt.Fprintf(&sb,
					`  <circle cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n",
					f(tx(p.x())), f(ty(p.y())), f(cfg.pointRadius), colorConstrained)
				continue
			}
			fill := "#d93025"
			if p.IsFixed() {
				fill = "#202124"
			}
			fmt.Fprintf(&sb,
				`  <circle cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n",
				f(tx(p.x())), f(ty(p.y())), f(cfg.pointRadius), fill)
		}
	}

	// Annotations render on top of geometry and point markers (see annotate.go).
	if cfg.dimensions {
		s.writeDimensions(&sb, cfg, b, tx, ty)
	}
	if cfg.constraints {
		s.writeGlyphs(&sb, cfg, b, tx, ty)
	}
	if cfg.statusBadge {
		s.writeStatusBadge(&sb, cfg, pad, w)
	}
	// Watermark sits on top, inside the frame's bottom band.
	if pad > 0 && cfg.watermark != "" {
		s.writeWatermark(&sb, cfg, pad, w, h)
	}

	sb.WriteString("</svg>\n")
	return sb.String(), nil
}

// radToDeg converts radians to degrees. Kept as the literal r*180/math.Pi
// expression so the exporters' angle output is bit-for-bit unchanged (routing
// through units conversion would diverge at the last ULP).
func radToDeg(r float64) float64 { return r * 180 / math.Pi }

// f formats a float compactly without a trailing ".000000".
func f(v float64) string { return trimFloat(v, 4) }

// trimFloat formats v with prec decimals and drops trailing zeros (and a bare
// trailing decimal point).
func trimFloat(v float64, prec int) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.*f", prec, v), "0"), ".")
}
