package sketch

import (
	"errors"
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
		case *Line:
			// A line lies within its two endpoints, and an OWNED endpoint was
			// already added by the point loop above. The case exists for the
			// endpoint this sketch does not own: entity fields are exported and
			// CreateLine takes a foreign *Point silently, so a line is the one
			// entity whose whole geometry can sit outside s.points.
			b.add(t.Start.x(), t.Start.y())
			b.add(t.End.x(), t.End.y())
			any = true
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

// finite reports whether every corner of the box is a finite number. A sketch
// carrying a non-finite coordinate (a NaN interior knot, a NaN control point, a
// point moved to NaN) poisons the box, and every downstream comparison against a
// NaN is false — so the non-positive-span clamp below does not catch it and the
// renderers emit NaN width/height/viewBox or, in PNG's case, hand image.Rect an
// out-of-range int. Checked first as an early, cheap refusal with a better
// locus than the postcondition below — but it is a PRECONDITION over the
// geometry that produced the box, not over what the exporter actually writes,
// and a finite box does not guarantee finite output: a finite span can still
// overflow in a later margin/scale/pixel-width multiply, and DXF's display-unit
// conversion can overflow a finite coordinate with no span arithmetic at all.
// Kept as the first, narrower check; svgWriter.f and DXF's pairf are the
// postcondition that closes those gaps.
func (b bbox) finite() bool {
	for _, v := range [4]float64{b.minX, b.minY, b.maxX, b.maxY} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// ErrNonFiniteGeometry is returned by the exporters when a non-finite (NaN or
// infinite) value would otherwise reach the output — either the early
// [bbox.finite] refusal, or the POSTCONDITION refusal when a value actually
// WRITTEN to the SVG/DXF output is non-finite, however it got that way (a
// finite bounding box overflowing in a margin/scale/pixel-width multiply, or
// DXF's display-unit conversion overflowing a finite coordinate) — tracked by
// [svgWriter.f] and DXF's pairf, the one formatter every numeric value in each
// output funnels through. PNG has no textual output to check this way; see its
// own pixel-dimension guard in PNG. Exported deliberately: a future
// [Sketch.Verify] condition is meant to reuse this same sentinel, so the
// oracle's reason and the exporter's refusal name one fact rather than two
// that could drift apart.
var ErrNonFiniteGeometry = errors.New("sketch: geometry has non-finite coordinates: a point or entity evaluates to NaN or infinity")

// svgWriter accumulates one [Sketch.SVG] call's output through a single
// formatting funnel, f, so the exporter's refusal is a POSTCONDITION over what
// was actually written — never a precondition over an input hoped to stand in
// for it. bbox.finite is exactly such a precondition, and each of several
// earlier rounds of this guard found another input for which it did not hold:
// WithMargin/WithPixelWidth/WithScale multiplying a finite box into a
// non-finite one after the check already ran, and a DXF unit conversion
// overflowing a finite coordinate with no span arithmetic involved at all.
// Checking the formatter itself needs no enumeration of arithmetic sites and
// cannot be evaded by a new one.
type svgWriter struct {
	strings.Builder
	nonFinite *bool // shared with every writer scratch() spawns from this one
}

// newSVGWriter starts a fresh top-level writer for one [Sketch.SVG] call, with
// its own nonFinite flag.
func newSVGWriter() *svgWriter { return &svgWriter{nonFinite: new(bool)} }

// scratch starts a nested writer for a fragment assembled separately and later
// embedded into w's own output (e.g. one <path> "d" attribute built in a loop,
// then written into w as a single %s) — sharing w's nonFinite flag, so a value
// that goes non-finite while building the fragment is still recorded on the
// writer whose output is actually returned to the caller.
func (w *svgWriter) scratch() *svgWriter { return &svgWriter{nonFinite: w.nonFinite} }

// f formats v compactly (see trimFloat) and flags the writer's shared
// nonFinite bit when v is NaN or infinite. It is the funnel every numeric
// value the SVG/annotation/frame renderers WRITE AS A NUMBER must pass
// through — grep confirms no other format verb bypasses it. Annotations and
// the watermark do write caller-influenced TEXT into the same buffer (an
// entity name, a dimension's unit-formatted value label): an entity name is
// never built from a float at all, so it can never false-trip this, but a
// dimension's label goes through units.Value.String, not f, since the
// rendered text (e.g. "20 mm") is not a bare formatted float. Its magnitude
// is screened separately, by note below, so a NaN/Inf target still refuses
// the document rather than rendering as the literal text "NaN mm"/"+Inf mm".
func (w *svgWriter) f(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		*w.nonFinite = true
	}
	return trimFloat(v, 4)
}

// note raises w's shared nonFinite bit when v is NaN or infinite, without
// writing any bytes. It exists for a value that never itself becomes SVG
// output text but whose non-finiteness must still refuse the document: a
// dimension's label goes through units.Value.String, not f, since the
// rendered text (e.g. "20 mm") is not a bare formatted float — but its
// magnitude must still be screened, or a NaN/Inf target renders as the
// literal text "NaN mm"/"+Inf mm" with no refusal at all. Screening the
// magnitude rather than the rendered string is deliberate: a string check
// would false-trip on an entity or dimension legitimately named "Inf".
func (w *svgWriter) note(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		*w.nonFinite = true
	}
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

// pointRadiusMinFrac floors the point-marker radius at this fraction of the
// geometry's bounding-box diagonal. A fixed radius (in sketch units) renders too
// small to read on a large-scale drawing — the same 2-unit marker is legible on a
// 200-unit sketch but a speck on a 500-unit one — so markers get a scale-relative
// minimum, matching how the annotation sizes derive from the diagonal.
const pointRadiusMinFrac = 0.008

// pointRadius returns the point-marker radius to render: the configured radius,
// floored to a fraction of the bbox diagonal so markers stay visible at any scale.
// Shared by the SVG and PNG exporters (whose configs both carry a point radius).
func pointRadius(configured float64, b bbox) float64 {
	return math.Max(configured, pointRadiusMinFrac*math.Hypot(b.maxX-b.minX, b.maxY-b.minY))
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
	if !b.finite() {
		return "", ErrNonFiniteGeometry
	}

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

	sb := newSVGWriter()
	fmt.Fprintf(sb,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s">`,
		sb.f(outW), sb.f(outH), sb.f(canvasW), sb.f(canvasH))
	sb.WriteByte('\n')
	if cfg.background != "" {
		fmt.Fprintf(sb, `  <rect width="100%%" height="100%%" fill="%s"/>`+"\n", cfg.background)
	}

	// Grid renders behind everything (but inside the frame); the frame border is
	// drawn on top of the grid, before the geometry.
	if pad > 0 {
		s.writeFrameGrid(sb, cfg, b, pad, w, h, tx, ty)
	}

	// Profile fill renders under the geometry, so it is emitted first.
	if cfg.profileFill {
		s.writeProfileFill(sb, tx, ty)
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
			return fmt.Sprintf(` stroke-dasharray="%s,%s"`, sb.f(cfg.strokeWidth*4), sb.f(cfg.strokeWidth*3))
		}
		return ""
	}
	// writePath emits one M/L SVG <path> for a sampled curve (arc, elliptical
	// arc, every spline family, conic, NURBS); only the pts source differs
	// between the curve cases below.
	writePath := func(pts [][2]float64, stroke string, sw float64, dasharray string) {
		d := sb.scratch()
		for i, p := range pts {
			cmd := "L"
			if i == 0 {
				cmd = "M"
			}
			fmt.Fprintf(d, "%s%s %s ", cmd, d.f(tx(p[0])), d.f(ty(p[1])))
		}
		fmt.Fprintf(sb,
			`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
			strings.TrimSpace(d.String()), stroke, sb.f(sw), dasharray)
	}

	for _, e := range s.ents {
		switch t := e.(type) {
		case *Line:
			fmt.Fprintf(sb,
				`  <line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"%s/>`+"\n",
				sb.f(tx(t.Start.x())), sb.f(ty(t.Start.y())), sb.f(tx(t.End.x())), sb.f(ty(t.End.y())),
				color(t), sb.f(cfg.strokeWidth), dash(t))
		case *Circle:
			fmt.Fprintf(sb,
				`  <circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				sb.f(tx(t.Center.x())), sb.f(ty(t.Center.y())), sb.f(t.r()),
				color(t), sb.f(cfg.strokeWidth), dash(t))
		case *Arc:
			writePath(arcPolyline(t, cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		case *EllipticalArc:
			writePath(ellipticalArcPolyline(t, cfg.arcSegments), color(t), cfg.strokeWidth, dash(t))
		case *Ellipse:
			// The y-flip mirrors the plane, so a CCW sketch rotation becomes
			// CW in SVG coordinates: negate the angle.
			cx, cy := tx(t.Center.x()), ty(t.Center.y())
			fmt.Fprintf(sb,
				`  <ellipse cx="%s" cy="%s" rx="%s" ry="%s" transform="rotate(%s %s %s)" fill="none" stroke="%s" stroke-width="%s"%s/>`+"\n",
				sb.f(cx), sb.f(cy), sb.f(t.rx()), sb.f(t.ry()),
				sb.f(-radToDeg(t.rot())), sb.f(cx), sb.f(cy),
				color(t), sb.f(cfg.strokeWidth), dash(t))
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
		pr := pointRadius(cfg.pointRadius, b) // scale-relative minimum so markers read at any size
		for _, p := range s.points {
			if cfg.dofColoring {
				// Free points render hollow blue, grounded points a filled green
				// square, other constrained points a filled black circle — a
				// redundant shape channel so DOF/grounding reads without color.
				if _, free := ov.freePt[p]; free {
					fmt.Fprintf(sb,
						`  <circle cx="%s" cy="%s" r="%s" fill="white" stroke="%s" stroke-width="%s"/>`+"\n",
						sb.f(tx(p.x())), sb.f(ty(p.y())), sb.f(pr), colorFree, sb.f(cfg.strokeWidth))
					continue
				}
				if p.IsFixed() {
					// A square anchor marks the grounded point(s): the sketch's tie to
					// the origin, distinct from geometry constrained by other relations.
					side := pr * 2
					fmt.Fprintf(sb,
						`  <rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`+"\n",
						sb.f(tx(p.x())-pr), sb.f(ty(p.y())-pr), sb.f(side), sb.f(side), colorFixed)
					continue
				}
				fmt.Fprintf(sb,
					`  <circle cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n",
					sb.f(tx(p.x())), sb.f(ty(p.y())), sb.f(pr), colorConstrained)
				continue
			}
			fill := "#d93025"
			if p.IsFixed() {
				fill = "#202124"
			}
			fmt.Fprintf(sb,
				`  <circle cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n",
				sb.f(tx(p.x())), sb.f(ty(p.y())), sb.f(pr), fill)
		}
	}

	// Annotations render on top of geometry and point markers (see annotate.go).
	if cfg.dimensions {
		s.writeDimensions(sb, cfg, b, tx, ty)
	}
	if cfg.constraints {
		s.writeGlyphs(sb, cfg, b, tx, ty)
	}
	if cfg.statusBadge {
		s.writeStatusBadge(sb, cfg, pad, w)
	}
	// A framed render always carries the provenance watermark, on top, inside
	// the frame's bottom band.
	if pad > 0 {
		s.writeWatermark(sb, pad, w, h)
	}

	sb.WriteString("</svg>\n")
	// Postcondition: refuse rather than return a document any of whose written
	// values was non-finite, however it got that way (see svgWriter.f).
	if *sb.nonFinite {
		return "", ErrNonFiniteGeometry
	}
	return sb.String(), nil
}

// radToDeg converts radians to degrees. Kept as the literal r*180/math.Pi
// expression so the exporters' angle output is bit-for-bit unchanged (routing
// through units conversion would diverge at the last ULP).
func radToDeg(r float64) float64 { return r * 180 / math.Pi }

// trimFloat formats v with prec decimals and drops trailing zeros (and a bare
// trailing decimal point).
func trimFloat(v float64, prec int) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.*f", prec, v), "0"), ".")
}
