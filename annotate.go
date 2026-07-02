package sketch

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/lestrrat-3d/sketch/units"
	"github.com/lestrrat-go/option/v3"
)

// Annotation rendering overlays dimensional and (later) geometric constraints
// onto the SVG output. It is additive and gated behind render options that
// default off, so the baseline exporter output stays byte-identical.
//
// Coordinate-space rule (load-bearing): svg.go flips y per coordinate in ty (a
// pure translate + y-flip at scale 1:1, so screen units == sketch mm). Every
// annotation computes its key points in sketch coordinates, maps them through
// tx/ty, and derives all screen directions/arrowheads/arcs from the mapped
// points — so nothing needs a per-case sign flip.

// --- options ----------------------------------------------------------------

type (
	identDimensions  struct{}
	identConstraints struct{}
	identDOFColoring struct{}
	identConflicts   struct{}
	identStatusBadge struct{}
	identProfileFill struct{}
	identAnnColor    struct{}
	identAnnScale    struct{}
	identPixelWidth  struct{}
)

// WithDimensions toggles drawing dimensional constraints (distance, radius,
// diameter, angle, arc length, ellipse semi-axes) as CAD-style dimensions:
// extension lines, a dimension line with arrowheads, and a value label in the
// dimension's own unit. Driven (reference) dimensions render parenthesized.
func WithDimensions(v bool) SVGPNGOption { return svgPNGOption{option.New(identDimensions{}, v)} }

// WithConstraints toggles drawing geometric-constraint glyphs (horizontal,
// vertical, parallel, perpendicular, tangent, equal, coincident, concentric,
// point-on, midpoint, symmetric) as small badges anchored to the geometry each
// constraint references.
func WithConstraints(v bool) SVGPNGOption { return svgPNGOption{option.New(identConstraints{}, v)} }

// WithDOFColoring toggles degree-of-freedom coloring: points that are not fully
// constrained (and any entity that owns one) render blue, fully constrained
// geometry renders black. Free points are also drawn hollow — a redundant,
// colorblind-safe channel. This is the canonical "under-constrained is blue" cue.
func WithDOFColoring(v bool) SVGPNGOption { return svgPNGOption{option.New(identDOFColoring{}, v)} }

// WithConflicts toggles highlighting the geometry of conflicting constraints
// (from [Sketch.Diagnose]) in red. When combined with WithDOFColoring,
// conflict-red takes precedence over DOF-blue.
func WithConflicts(v bool) SVGPNGOption { return svgPNGOption{option.New(identConflicts{}, v)} }

// WithStatusBadge toggles a small corner card summarizing the sketch's
// verification state: DOF, constraint status and solvability (from
// [Sketch.Verify]).
func WithStatusBadge(v bool) SVGPNGOption { return svgPNGOption{option.New(identStatusBadge{}, v)} }

// WithProfileFill toggles a translucent fill under every valid closed region
// (from [Sketch.Profiles]); self-intersecting or degenerate regions are skipped.
func WithProfileFill(v bool) SVGPNGOption { return svgPNGOption{option.New(identProfileFill{}, v)} }

// WithAnnotationColor sets the color of dimension lines and constraint glyphs.
func WithAnnotationColor(v string) SVGPNGOption {
	return svgPNGOption{option.New(identAnnColor{}, v)}
}

// WithAnnotationScale multiplies the size of annotation glyphs, text and
// arrowheads (all otherwise derived from the drawing's bounding-box diagonal).
func WithAnnotationScale(v float64) SVGPNGOption {
	return svgPNGOption{option.New(identAnnScale{}, v)}
}

// WithPixelWidth sets the SVG's display width in pixels, scaling the height to
// preserve aspect ratio; the viewBox stays in geometry units so the drawing
// simply scales up. Use it to embed a small sketch at a legible size. Zero (the
// default) keeps the display size equal to the geometry units. SVG only.
func WithPixelWidth(v float64) SVGPNGOption { return svgPNGOption{option.New(identPixelWidth{}, v)} }

// --- screen-space vector helpers --------------------------------------------

type v2 = [2]float64

func vsub(a, b v2) v2         { return v2{a[0] - b[0], a[1] - b[1]} }
func vadd(a, b v2) v2         { return v2{a[0] + b[0], a[1] + b[1]} }
func vmul(a v2, s float64) v2 { return v2{a[0] * s, a[1] * s} }
func vlen(a v2) float64       { return math.Hypot(a[0], a[1]) }
func vperp(a v2) v2           { return v2{-a[1], a[0]} }
func vmid(a, b v2) v2         { return v2{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2} }

func vunit(a v2) v2 {
	n := vlen(a)
	if n < 1e-9 {
		return v2{0, 0}
	}
	return v2{a[0] / n, a[1] / n}
}

// --- label formatting -------------------------------------------------------

// annFormat renders a dimension value for display: the units library's own
// string form (e.g. "20 mm", "4 in") with the ASCII degree symbol "deg"
// prettified to "°" (units.Value.String has no degree glyph).
func annFormat(v units.Value) string {
	s := v.String()
	if strings.HasSuffix(s, " deg") {
		return strings.TrimSuffix(s, " deg") + "°"
	}
	return s
}

// dimText builds a dimension label with an optional prefix (e.g. "R", "⌀"),
// wrapping driven (reference) dimensions in parentheses per CAD convention.
func dimText(prefix string, d Dimension) string {
	s := prefix + annFormat(d.Target())
	if d.Driven() {
		return "(" + s + ")"
	}
	return s
}

// --- the dimension pass -----------------------------------------------------

// annCtx carries the resolved sizes and transforms for one annotation pass so
// each dimension case stays terse. tx/ty are svg.go's per-coordinate transforms
// (translate + y-flip); all geometry is mapped through them before any
// direction is derived.
type annCtx struct {
	sb     *strings.Builder
	tx, ty func(float64) float64
	center v2 // screen-space bbox center, so dimensions offset outward
	col    string
	sw     float64 // stroke width
	arrow  float64 // arrowhead length
	text   float64 // font size
	gap    float64 // dimension-line offset from the geometry
	ext    float64 // extension beyond the dimension line

	placed []v2 // base anchors of glyphs already drawn, for per-anchor stacking
}

// scr maps a sketch point to screen space.
func (a *annCtx) scr(p *Point) v2 { return v2{a.tx(p.x()), a.ty(p.y())} }

// xy maps raw sketch coordinates to screen space (for points built on the fly,
// e.g. a circle rim, that are not committed sketch points).
func (a *annCtx) xy(x, y float64) v2 { return v2{a.tx(x), a.ty(y)} }

// lineMid returns the screen midpoint of a line.
func (a *annCtx) lineMid(l *Line) v2 { return vmid(a.scr(l.Start), a.scr(l.End)) }

// overlaySets holds the DOF/conflict membership sets consulted by the geometry
// and point coloring. Empty (nil) maps mean the corresponding overlay is off, so
// lookups fall through and the baseline colors apply.
type overlaySets struct {
	conf    map[Entity]struct{} // geometry of conflicting constraints (red)
	freeEnt map[Entity]struct{} // entities owning a not-fully-constrained point (blue)
	freePt  map[*Point]struct{} // points that are not fully constrained
}

// computeOverlaySets builds the DOF-coloring and conflict-highlight membership
// sets from the diagnostics, once per render. Membership is by pointer lookup
// only — never iterated in the emit path — so output order stays deterministic.
func (s *Sketch) computeOverlaySets(cfg svgConfig) overlaySets {
	var ov overlaySets
	if cfg.conflicts {
		ov.conf = make(map[Entity]struct{})
		confPts := make(map[*Point]struct{})
		for _, c := range s.Diagnose().Conflicting {
			pts, ents := constraintRefs(c)
			for _, e := range ents {
				ov.conf[e] = struct{}{}
			}
			for _, p := range pts {
				confPts[p] = struct{}{}
			}
		}
		// A dimensional constraint references its endpoints, not the line entity,
		// so also redden any entity whose defining points are all conflict points
		// (e.g. the segment between two conflicting-distance points).
		if len(confPts) > 0 {
			for _, e := range s.ents {
				ep := entityPoints(e)
				if len(ep) == 0 {
					continue
				}
				all := true
				for _, p := range ep {
					if _, ok := confPts[p]; !ok {
						all = false
						break
					}
				}
				if all {
					ov.conf[e] = struct{}{}
				}
			}
		}
	}
	if cfg.dofColoring {
		ov.freePt = make(map[*Point]struct{})
		for _, p := range s.FreePoints() {
			ov.freePt[p] = struct{}{}
		}
		ov.freeEnt = make(map[Entity]struct{})
		for _, e := range s.ents {
			for _, p := range entityPoints(e) {
				if _, ok := ov.freePt[p]; ok {
					ov.freeEnt[e] = struct{}{}
					break
				}
			}
		}
	}
	return ov
}

// Verification-overlay colors.
const (
	colorFree        = "#1a73e8" // not fully constrained (blue)
	colorConstrained = "#202124" // fully constrained (black)
	colorConflict    = "#d93025" // conflicting-constraint geometry (red)
)

// newAnnCtx resolves the annotation sizes/transforms shared by the dimension
// and glyph passes. Sizes derive from the bounding-box diagonal so annotations
// scale with the drawing at any coordinate scale.
func newAnnCtx(sb *strings.Builder, cfg svgConfig, b bbox, tx, ty func(float64) float64) *annCtx {
	diag := math.Hypot(b.maxX-b.minX, b.maxY-b.minY)
	if diag <= 0 {
		diag = 1
	}
	sz := cfg.annScale
	return &annCtx{
		sb:     sb,
		tx:     tx,
		ty:     ty,
		center: v2{tx((b.minX + b.maxX) / 2), ty((b.minY + b.maxY) / 2)},
		col:    cfg.annColor,
		sw:     cfg.strokeWidth,
		arrow:  0.02 * diag * sz,
		text:   0.04 * diag * sz,
		gap:    0.06 * diag * sz,
		ext:    0.015 * diag * sz,
	}
}

// writeDimensions draws every dimensional constraint.
func (s *Sketch) writeDimensions(sb *strings.Builder, cfg svgConfig, b bbox, tx, ty func(float64) float64) {
	a := newAnnCtx(sb, cfg, b, tx, ty)
	for _, c := range s.cons {
		d, ok := c.(Dimension)
		if !ok {
			continue
		}
		a.dimension(d)
	}
}

// writeGlyphs draws a small badge for every geometric constraint, anchored to
// the geometry it references. Constraints are walked in slice order and glyphs
// on a shared anchor stack downward (deterministic, no pointer-keyed map).
func (s *Sketch) writeGlyphs(sb *strings.Builder, cfg svgConfig, b bbox, tx, ty func(float64) float64) {
	a := newAnnCtx(sb, cfg, b, tx, ty)
	for _, c := range s.cons {
		a.glyph(c)
	}
}

// glyph dispatches one geometric constraint to its badge(s). Dimensional
// constraints are handled by the dimension pass and skipped here.
func (a *annCtx) glyph(c Constraint) {
	switch t := c.(type) {
	case *horizontal:
		a.badge(a.lineMid(t.L), "H")
	case *vertical:
		a.badge(a.lineMid(t.L), "V")
	case *horizontalPoints:
		a.badge(vmid(a.scr(t.P1), a.scr(t.P2)), "H")
	case *verticalPoints:
		a.badge(vmid(a.scr(t.P1), a.scr(t.P2)), "V")
	case *parallel:
		a.badge(a.lineMid(t.L1), "∥")
		a.badge(a.lineMid(t.L2), "∥")
	case *perpendicular:
		a.badge(a.lineMid(t.L1), "⊥")
		a.badge(a.lineMid(t.L2), "⊥")
	case *collinear:
		a.badge(a.lineMid(t.L1), "—")
		a.badge(a.lineMid(t.L2), "—")
	case *equalLines:
		a.badge(a.lineMid(t.L1), "=")
		a.badge(a.lineMid(t.L2), "=")
	case *equalRadii:
		a.badge(a.rimBadgeAnchor(t.C1), "=")
		a.badge(a.rimBadgeAnchor(t.C2), "=")
	case *coincident:
		a.badge(a.scr(t.P1), "○")
	case *concentric:
		a.badge(a.scr(t.C1.centerPt()), "◎")
	case *pointOnLine:
		a.badge(a.scr(t.P), "•")
	case *pointOnCircle:
		a.badge(a.scr(t.P), "•")
	case *pointOnEllipse:
		a.badge(a.scr(t.P), "•")
	case *midpoint:
		a.badge(a.scr(t.P), "M")
	case *midpointOf:
		a.badge(a.scr(t.Mid), "M")
	case *symmetric:
		a.badge(vmid(a.scr(t.P1), a.scr(t.P2)), "S")
	case *symmetricLines:
		a.badge(vmid(a.lineMid(t.L1), a.lineMid(t.L2)), "S")
	case *tangentLineCircle:
		a.badge(a.lineMid(t.L), "T")
	case *tangentCircles:
		if t.shared != nil {
			a.badge(a.scr(t.shared), "T")
			return
		}
		a.badge(vmid(a.scr(t.C1.centerPt()), a.scr(t.C2.centerPt())), "T")
	}
}

// rimBadgeAnchor returns a screen anchor near a circle/arc rim (at the anchor
// angle) for a badge that belongs to the circle rather than its center.
func (a *annCtx) rimBadgeAnchor(c Circular) v2 {
	cp := c.centerPt()
	ang := circularAnchorAngle(c)
	return a.xy(cp.x()+c.R()*math.Cos(ang), cp.y()+c.R()*math.Sin(ang))
}

// badge draws a small boxed symbol at anchor, stacking downward when several
// glyphs share an anchor.
func (a *annCtx) badge(anchor v2, sym string) {
	idx := 0
	for _, p := range a.placed {
		if vlen(vsub(p, anchor)) < a.text*0.5 {
			idx++
		}
	}
	a.placed = append(a.placed, anchor)
	pos := vadd(anchor, v2{0, float64(idx) * a.text * 1.7})
	r := a.text * 0.85
	fmt.Fprintf(a.sb,
		`  <rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="white" stroke="%s" stroke-width="%s"/>`+"\n",
		f(pos[0]-r), f(pos[1]-r), f(2*r), f(2*r), f(r*0.3), a.col, f(a.sw*0.75))
	a.label(pos, sym, false)
}

// dimension dispatches one dimensional constraint to its renderer.
func (a *annCtx) dimension(d Dimension) {
	switch t := d.(type) {
	case *Distance:
		a.linear(a.scr(t.P1), a.scr(t.P2), dimText("", t), t.Driven())
	case *HorizontalDistance:
		a.axisAligned(a.scr(t.P1), a.scr(t.P2), true, dimText("", t), t.Driven())
	case *VerticalDistance:
		a.axisAligned(a.scr(t.P1), a.scr(t.P2), false, dimText("", t), t.Driven())
	case *DistancePointLine:
		a.pointToLine(t.P, t.L, dimText("", t), t.Driven())
	case *DistanceLines:
		a.lineToLine(t.L1, t.L2, dimText("", t), t.Driven())
	case *Offset:
		a.lineToLine(t.Src, t.Dst, dimText("", t), t.Driven())
	case *Radius:
		a.radial(t.C, dimText("R", t), t.Driven())
	case *Diameter:
		a.diametral(t.C, dimText("⌀", t), t.Driven())
	case *Angle:
		a.angle(t.L1, t.L2, dimText("", t), t.Driven())
	case *ArcLength:
		a.arcLength(t.A, dimText("", t), t.Driven())
	case *SemiMajor:
		a.semiAxis(t.E, true, dimText("", t), t.Driven())
	case *SemiMinor:
		a.semiAxis(t.E, false, dimText("", t), t.Driven())
	case *EllipseRotation:
		a.ellipseRotation(t.E, dimText("", t), t.Driven())
	case *DistancePointCircle:
		a.leaderToCircular(a.scr(t.P), t.C, dimText("", t), t.Driven())
	case *DistanceLineCircle:
		a.leaderToCircular(a.lineMid(t.L), t.C, dimText("", t), t.Driven())
	case *DistancePointArc:
		a.leaderToCircular(a.scr(t.P), t.A, dimText("", t), t.Driven())
	case *DistanceLineArc:
		a.leaderToCircular(a.lineMid(t.L), t.A, dimText("", t), t.Driven())
	}
}

// linear draws a dimension between two screen points, offset perpendicular to
// the line joining them.
func (a *annCtx) linear(p1, p2 v2, label string, driven bool) {
	dir := vunit(vsub(p2, p1))
	if dir == (v2{}) { // coincident points: just label the spot
		a.label(vadd(p1, v2{0, -a.text}), label, driven)
		return
	}
	n := vperp(dir)
	// Offset outward: flip the perpendicular if it points toward the drawing
	// center, so the dimension sits clear of the geometry.
	mid := vmid(p1, p2)
	if n[0]*(a.center[0]-mid[0])+n[1]*(a.center[1]-mid[1]) > 0 {
		n = vmul(n, -1)
	}
	q1 := vadd(p1, vmul(n, a.gap))
	q2 := vadd(p2, vmul(n, a.gap))
	a.line(p1, vadd(q1, vmul(n, a.ext)))
	a.line(p2, vadd(q2, vmul(n, a.ext)))
	a.dimLine(q1, q2)
	a.label(vadd(vmid(q1, q2), vmul(n, a.text*0.7)), label, driven)
}

// axisAligned draws a horizontal (Δx) or vertical (Δy) dimension. Horizontal
// and vertical directions are invariant under the y-flip, so the offset is a
// fixed screen direction placed clear of the span.
func (a *annCtx) axisAligned(p1, p2 v2, horizontal bool, label string, driven bool) {
	var q1, q2, n v2
	if horizontal {
		y := math.Max(p1[1], p2[1]) + a.gap
		q1, q2 = v2{p1[0], y}, v2{p2[0], y}
		n = v2{0, 1}
	} else {
		x := math.Max(p1[0], p2[0]) + a.gap
		q1, q2 = v2{x, p1[1]}, v2{x, p2[1]}
		n = v2{1, 0}
	}
	a.line(p1, vadd(q1, vmul(n, a.ext)))
	a.line(p2, vadd(q2, vmul(n, a.ext)))
	a.dimLine(q1, q2)
	a.label(vadd(vmid(q1, q2), vmul(n, a.text*0.7)), label, driven)
}

// pointToLine draws the perpendicular distance from a point to a line.
func (a *annCtx) pointToLine(p *Point, l *Line, label string, driven bool) {
	fx, fy := footOnLine(l, p.x(), p.y())
	ps, fs := a.scr(p), a.xy(fx, fy)
	if vlen(vsub(ps, fs)) < a.arrow { // point on the line: no room for a dimension line
		a.label(vadd(ps, v2{0, -a.text}), label, driven)
		return
	}
	a.dimLine(ps, fs)
	a.label(vadd(vmid(ps, fs), v2{a.text * 0.5, 0}), label, driven)
}

// lineToLine draws the distance between two (parallel) lines, anchored at the
// midpoint of the first line.
func (a *annCtx) lineToLine(l1, l2 *Line, label string, driven bool) {
	mx, my := (l1.Start.x()+l1.End.x())/2, (l1.Start.y()+l1.End.y())/2
	fx, fy := footOnLine(l2, mx, my)
	ms, fs := a.xy(mx, my), a.xy(fx, fy)
	if vlen(vsub(ms, fs)) < a.arrow {
		a.label(vadd(ms, v2{0, -a.text}), label, driven)
		return
	}
	a.dimLine(ms, fs)
	a.label(vadd(vmid(ms, fs), v2{a.text * 0.5, 0}), label, driven)
}

// radial draws a leader from a circle/arc center to its rim. For an arc the rim
// direction is the arc midpoint (so the leader lands on the drawn sweep); for a
// full circle a fixed 45° direction is used.
func (a *annCtx) radial(c Circular, label string, driven bool) {
	ctr := c.centerPt()
	ang := circularAnchorAngle(c)
	r := c.R()
	cs := a.scr(ctr)
	rim := a.xy(ctr.x()+r*math.Cos(ang), ctr.y()+r*math.Sin(ang))
	a.dimLineOneArrow(cs, rim)
	out := vunit(vsub(rim, cs))
	a.label(vadd(rim, vmul(out, a.text)), label, driven)
}

// diametral draws a diameter dimension. For a full circle it draws a chord
// through the center; for an arc it falls back to a radial leader (a full
// diameter chord would leave the sweep).
func (a *annCtx) diametral(c Circular, label string, driven bool) {
	if _, isArc := c.(*Arc); isArc {
		a.radial(c, label, driven)
		return
	}
	ctr := c.centerPt()
	r := c.R()
	cx, cy := ctr.x(), ctr.y()
	ang := math.Pi / 4
	p1 := a.xy(cx+r*math.Cos(ang), cy+r*math.Sin(ang))
	p2 := a.xy(cx-r*math.Cos(ang), cy-r*math.Sin(ang))
	a.dimLine(p1, p2)
	a.label(vadd(vmid(p1, p2), v2{0, -a.text * 0.5}), label, driven)
}

// angle draws an arc between two lines' directions, anchored between their
// midpoints. Parallel lines (no defined angle glyph) get a plain label.
func (a *annCtx) angle(l1, l2 *Line, label string, driven bool) {
	d1 := vunit(vsub(a.scr(l1.End), a.scr(l1.Start)))
	d2 := vunit(vsub(a.scr(l2.End), a.scr(l2.Start)))
	// Anchor at the shared vertex when the lines meet at a point (the common
	// angle-between-two-edges case); otherwise midway between the line midpoints.
	anchor := vmid(a.lineMid(l1), a.lineMid(l2))
	if sp := sharedLinePoint(l1, l2); sp != nil {
		anchor = a.scr(sp)
	}
	cross := d1[0]*d2[1] - d1[1]*d2[0]
	if math.Abs(cross) < 1e-6 { // parallel: no arc, just the value
		a.label(anchor, label, driven)
		return
	}
	r := a.gap
	a1 := math.Atan2(d1[1], d1[0])
	a2 := math.Atan2(d2[1], d2[0])
	a.arcPath(anchor, r, a1, a2)
	amid := a1 + shortDelta(a1, a2)/2
	lp := vadd(anchor, v2{(r + a.text) * math.Cos(amid), (r + a.text) * math.Sin(amid)})
	a.label(lp, label, driven)
}

// arcLength draws a short leader to the arc's midpoint with a length label.
func (a *annCtx) arcLength(arc *Arc, label string, driven bool) {
	mp := arc.Geometry().MidPoint()
	mpS := a.xy(mp.X, mp.Y)
	out := vunit(vsub(mpS, a.scr(arc.Center)))
	lp := vadd(mpS, vmul(out, a.gap))
	a.line(mpS, lp)
	a.label(vadd(lp, vmul(out, a.text*0.4)), label, driven)
}

// semiAxis draws a leader from an ellipse center along its major (local x) or
// minor (local y) axis. The axis endpoint is built in sketch space and mapped,
// so the y-flip is handled without a manual angle negation.
func (a *annCtx) semiAxis(e Elliptical, major bool, label string, driven bool) {
	ctr := e.centerPt()
	rot := e.Rotation()
	r := e.Rx()
	if !major {
		rot += math.Pi / 2
		r = e.Ry()
	}
	cs := a.scr(ctr)
	end := a.xy(ctr.x()+r*math.Cos(rot), ctr.y()+r*math.Sin(rot))
	a.dimLineOneArrow(cs, end)
	out := vunit(vsub(end, cs))
	a.label(vadd(end, vmul(out, a.text)), label, driven)
}

// ellipseRotation draws a small angle glyph at the ellipse center between the
// x-axis and the major axis.
func (a *annCtx) ellipseRotation(e Elliptical, label string, driven bool) {
	ctr := e.centerPt()
	cs := a.scr(ctr)
	// +x axis direction and the major-axis direction in screen space, both
	// built from mapped points so the flip is already applied.
	xdir := vunit(vsub(a.xy(ctr.x()+1, ctr.y()), cs))
	mdir := vunit(vsub(a.xy(ctr.x()+math.Cos(e.Rotation()), ctr.y()+math.Sin(e.Rotation())), cs))
	a1 := math.Atan2(xdir[1], xdir[0])
	a2 := math.Atan2(mdir[1], mdir[0])
	a.arcPath(cs, a.gap, a1, a2)
	a.label(vadd(cs, v2{0, -a.gap - a.text}), label, driven)
}

// leaderToCircular draws a leader from a screen point to a point on a circle/arc
// rim, labeling the gap. Used by the point/line-to-circle/arc distances.
func (a *annCtx) leaderToCircular(from v2, c Circular, label string, driven bool) {
	cp := c.centerPt()
	ang := circularAnchorAngle(c)
	rim := a.xy(cp.x()+c.R()*math.Cos(ang), cp.y()+c.R()*math.Sin(ang))
	a.dimLine(from, rim)
	a.label(vadd(vmid(from, rim), v2{a.text * 0.5, 0}), label, driven)
}

// --- emit helpers -----------------------------------------------------------

func (a *annCtx) line(p, q v2) {
	fmt.Fprintf(a.sb,
		`  <line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"/>`+"\n",
		f(p[0]), f(p[1]), f(q[0]), f(q[1]), a.col, f(a.sw))
}

// dimLine draws a dimension line with an arrowhead at each end (pointing
// outward, toward the extension lines).
func (a *annCtx) dimLine(p, q v2) {
	a.line(p, q)
	a.arrowAt(p, vunit(vsub(p, q)))
	a.arrowAt(q, vunit(vsub(q, p)))
}

// dimLineOneArrow draws a leader with a single arrowhead at its far end (q).
func (a *annCtx) dimLineOneArrow(p, q v2) {
	a.line(p, q)
	a.arrowAt(q, vunit(vsub(q, p)))
}

// arrowAt draws a filled triangular arrowhead whose tip is at p, pointing along
// dir.
func (a *annCtx) arrowAt(p, dir v2) {
	if dir == (v2{}) {
		return
	}
	back := vsub(p, vmul(dir, a.arrow))
	n := vmul(vperp(dir), a.arrow*0.35)
	l := vadd(back, n)
	r := vsub(back, n)
	fmt.Fprintf(a.sb,
		`  <path d="M%s %s L%s %s L%s %s Z" fill="%s"/>`+"\n",
		f(p[0]), f(p[1]), f(l[0]), f(l[1]), f(r[0]), f(r[1]), a.col)
}

// arcPath emits a sampled arc (short way) from angle a1 to a2 about center c.
func (a *annCtx) arcPath(c v2, r, a1, a2 float64) {
	const n = 16
	delta := shortDelta(a1, a2)
	var d strings.Builder
	for i := 0; i <= n; i++ {
		ang := a1 + delta*float64(i)/n
		x := c[0] + r*math.Cos(ang)
		y := c[1] + r*math.Sin(ang)
		cmd := "L"
		if i == 0 {
			cmd = "M"
		}
		fmt.Fprintf(&d, "%s%s %s ", cmd, f(x), f(y))
	}
	fmt.Fprintf(a.sb,
		`  <path d="%s" fill="none" stroke="%s" stroke-width="%s"/>`+"\n",
		strings.TrimSpace(d.String()), a.col, f(a.sw))
}

// label emits centered text at p. Driven labels are tinted lighter.
func (a *annCtx) label(p v2, s string, driven bool) {
	col := a.col
	if driven {
		col = "#9aa0a6"
	}
	fmt.Fprintf(a.sb,
		`  <text x="%s" y="%s" font-size="%s" fill="%s" text-anchor="middle" dominant-baseline="central">%s</text>`+"\n",
		f(p[0]), f(p[1]), f(a.text), col, svgEscape(s))
}

// --- small math helpers -----------------------------------------------------

// footOnLine returns the foot of the perpendicular from (px,py) onto the
// infinite line through l, in sketch coordinates. A degenerate line returns its
// start.
func footOnLine(l *Line, px, py float64) (float64, float64) {
	ax, ay := l.Start.x(), l.Start.y()
	bx, by := l.End.x(), l.End.y()
	abx, aby := bx-ax, by-ay
	d := abx*abx + aby*aby
	if d < 1e-18 {
		return ax, ay
	}
	t := ((px-ax)*abx + (py-ay)*aby) / d
	return ax + t*abx, ay + t*aby
}

// sharedLinePoint returns the point two lines share as an endpoint (their common
// vertex), or nil if they do not meet at a point.
func sharedLinePoint(l1, l2 *Line) *Point {
	for _, p := range []*Point{l1.Start, l1.End} {
		if p == l2.Start || p == l2.End {
			return p
		}
	}
	return nil
}

// shortDelta returns the signed shortest angular difference from a1 to a2, in
// (-π, π].
func shortDelta(a1, a2 float64) float64 {
	d := math.Mod(a2-a1, 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	}
	if d <= -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

// circularAnchorAngle returns the angle about the center at which to anchor an
// annotation: the arc midpoint for an *Arc (on the drawn sweep), else 45°.
func circularAnchorAngle(c Circular) float64 {
	if arc, ok := c.(*Arc); ok {
		return arc.Geometry().MidAngle()
	}
	return math.Pi / 4
}

// --- verification overlays --------------------------------------------------

// writeProfileFill fills every valid closed region with a translucent color,
// under the geometry. Invalid / self-intersecting regions are skipped (filling a
// self-crossing boundary renders even-odd artifacts). Regions are emitted in a
// canonical order (min-corner, then area, then edge count) so output is
// deterministic regardless of the arrangement walk order.
func (s *Sketch) writeProfileFill(sb *strings.Builder, tx, ty func(float64) float64) {
	var valid []*Profile
	for _, p := range s.Profiles() {
		if p.Valid && !p.SelfIntersecting {
			valid = append(valid, p)
		}
	}
	sort.SliceStable(valid, func(i, j int) bool { return profileLess(valid[i], valid[j]) })

	for _, p := range valid {
		var d strings.Builder
		writeLoopPath(&d, p.Outer, tx, ty)
		for _, h := range p.Holes {
			writeLoopPath(&d, h, tx, ty)
		}
		fmt.Fprintf(sb,
			`  <path d="%s" fill="#1a73e8" fill-opacity="0.12" fill-rule="evenodd" stroke="none"/>`+"\n",
			strings.TrimSpace(d.String()))
	}
}

// writeLoopPath appends one closed subpath (M…L…Z) for a boundary loop, walking
// each edge's densified polyline and dropping the duplicated shared endpoints.
func writeLoopPath(d *strings.Builder, loop []BoundaryEdge, tx, ty func(float64) float64) {
	first := true
	var prevX, prevY float64
	for _, e := range loop {
		for _, pt := range e.Polyline {
			x, y := tx(pt[0]), ty(pt[1])
			if !first && x == prevX && y == prevY {
				continue // shared endpoint between consecutive edges
			}
			cmd := "L"
			if first {
				cmd = "M"
				first = false
			}
			fmt.Fprintf(d, "%s%s %s ", cmd, f(x), f(y))
			prevX, prevY = x, y
		}
	}
	d.WriteString("Z ")
}

// profileLess orders profiles by a total key so the fill order is stable
// independent of the arrangement walk order: min-corner (x, then y), area, edge
// count, hole count, then the outer loop's first sampled point as the final
// tiebreak (two distinct regions cannot share all of these).
func profileLess(a, b *Profile) bool {
	ax, ay := loopMin(a.Outer)
	bx, by := loopMin(b.Outer)
	switch {
	case ax != bx:
		return ax < bx
	case ay != by:
		return ay < by
	case a.Area != b.Area:
		return a.Area < b.Area
	case len(a.Outer) != len(b.Outer):
		return len(a.Outer) < len(b.Outer)
	case len(a.Holes) != len(b.Holes):
		return len(a.Holes) < len(b.Holes)
	}
	afx, afy := loopFirst(a.Outer)
	bfx, bfy := loopFirst(b.Outer)
	if afx != bfx {
		return afx < bfx
	}
	return afy < bfy
}

// loopFirst returns the first sampled point of a loop (0,0 for an empty loop).
func loopFirst(loop []BoundaryEdge) (float64, float64) {
	for _, e := range loop {
		if len(e.Polyline) > 0 {
			return e.Polyline[0][0], e.Polyline[0][1]
		}
	}
	return 0, 0
}

// loopMin returns the minimum x and y over a loop's edge polylines.
func loopMin(loop []BoundaryEdge) (float64, float64) {
	minX, minY := math.Inf(1), math.Inf(1)
	for _, e := range loop {
		for _, pt := range e.Polyline {
			minX = math.Min(minX, pt[0])
			minY = math.Min(minY, pt[1])
		}
	}
	return minX, minY
}

// writeStatusBadge draws a corner card summarizing the verification state.
func (s *Sketch) writeStatusBadge(sb *strings.Builder, cfg svgConfig, w float64) {
	rep := s.Verify()
	txt := fmt.Sprintf("DOF %d · %s · solvable=%t", rep.DOF, rep.Status, rep.Solvable)
	size := w * 0.035 * cfg.annScale
	if size <= 0 {
		size = 1
	}
	// Top-left, inside the blank margin so it clears the geometry.
	x, y := cfg.strokeWidth*2, cfg.strokeWidth*2
	boxW := float64(len(txt)) * size * 0.58
	fmt.Fprintf(sb,
		`  <rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="white" fill-opacity="0.85" stroke="%s" stroke-width="%s"/>`+"\n",
		f(x), f(y), f(boxW), f(size*1.8), f(size*0.4), cfg.annColor, f(cfg.strokeWidth*0.75))
	fmt.Fprintf(sb,
		`  <text x="%s" y="%s" font-size="%s" fill="%s" dominant-baseline="central">%s</text>`+"\n",
		f(x+size*0.5), f(y+size*0.9), f(size), cfg.annColor, svgEscape(txt))
}

// svgEscape escapes the XML-special characters that could appear in a label.
func svgEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
