package sketch

// Name labels and where they go.
//
// A label is the optional name a point or an entity carries (names.go) written
// onto the drawing. Two things decide how one is drawn: WHICH KIND of geometry
// carries it, which fixes its style, and WHERE there is room for it, which this
// file searches for rather than assuming.
//
// The search exists because a fixed offset does not survive a real drawing. On a
// sketch that names two dozen points inside one small figure — a gear's section
// lattice, say — a name pinned up and to the right of every marker lands on the
// next marker, on a construction line, or on another name, and the reader is
// left with a block of text over a drawing. The placement below tries a fixed
// ring of positions around each anchor and keeps the one that collides least,
// which is the standard way a map labels its towns.

import (
	"fmt"
	"math"

	"github.com/lestrrat-go/option/v3"
)

type identLabels struct{}

// WithLabels toggles drawing the optional names points and entities carry (see
// [Point.SetName] and [named.SetName]) beside the geometry they belong to.
// Geometry with no name draws nothing, so a sketch that names its six hexagon
// corners and leaves its construction lines unnamed labels the six corners.
//
// The two kinds are drawn differently, so a drawing carrying both says which is
// which. A point's name is upright and hung to one side of its marker. An
// entity's is italic and reads off the mean of the points that define it, which
// is the midpoint of a line and the centre of a circle.
//
// Where each name goes is searched for, not fixed: a ring of positions about the
// anchor is tried and the one that sits on the least — other names first, then
// the drawing's own geometry, and never off the edge of the canvas — is kept. On
// a crowded drawing some overlap is unavoidable, and the search then keeps the
// least bad position rather than refusing to label. SVG only.
func WithLabels(v bool) SVGPNGOption { return svgPNGOption{option.New(identLabels{}, v)} }

// writeLabels draws the optional name every named point and entity carries.
//
// Points come first and entities second, each in creation order, so the output
// is deterministic and so is the placement: each name is placed against the ones
// already down, and walking a slice is what makes "already" mean the same thing
// on every run.
func (s *Sketch) writeLabels(sb *svgWriter, cfg svgConfig, b bbox, tx, ty func(float64) float64, canvas rect) {
	a := newAnnCtx(sb, cfg, b, tx, ty)
	lp := s.newLabelPlacer(a, cfg, tx, ty, canvas)
	for _, p := range s.points {
		if p.Name() == "" {
			continue
		}
		lp.place(a.scr(p), p.Name(), labelPoint)
	}
	for _, e := range s.ents {
		if e.Name() == "" {
			continue
		}
		anchor, ok := a.entityAnchor(e)
		if !ok {
			continue
		}
		lp.place(anchor, e.Name(), labelEntity)
	}
}

// entityAnchor is the point an entity's own name reads off: the mean of the
// points that define it, which is the midpoint of a line, the centre of a circle
// or ellipse, and the average of a spline's control points.
//
// It reads those points through [entityPoints], the same accessor grounding and
// the removal cascade read, rather than through a type switch of its own. A new
// entity type then gets an anchor by satisfying the contract it already has to
// satisfy, instead of by someone remembering this file exists. An entity that
// entityPoints does not know reports no anchor and is skipped rather than
// labelled at the origin.
func (a *annCtx) entityAnchor(e Entity) (v2, bool) {
	pts := entityPoints(e)
	if len(pts) == 0 {
		return v2{}, false
	}
	var sum v2
	for _, p := range pts {
		sum = vadd(sum, a.scr(p))
	}
	return vmul(sum, 1/float64(len(pts))), true
}

// labelKind is which of the two styles a name is drawn in.
type labelKind int

const (
	labelPoint labelKind = iota
	labelEntity
)

// rect is a screen-space axis-aligned box: the area a label's text covers, the
// square a point marker sits in, or the canvas itself.
type rect struct{ minX, minY, maxX, maxY float64 }

func (r rect) width() float64  { return r.maxX - r.minX }
func (r rect) height() float64 { return r.maxY - r.minY }

// overlaps reports whether two boxes share any area. Touching edges do not
// count: a label set flush against another is legible.
func (r rect) overlaps(o rect) bool {
	return r.minX < o.maxX && o.minX < r.maxX && r.minY < o.maxY && o.minY < r.maxY
}

// contains reports whether r holds o entirely.
func (r rect) contains(o rect) bool {
	return o.minX >= r.minX && o.maxX <= r.maxX && o.minY >= r.minY && o.maxY <= r.maxY
}

// holdsPoint reports whether the box covers a screen point.
func (r rect) holdsPoint(p v2) bool {
	return p[0] >= r.minX && p[0] <= r.maxX && p[1] >= r.minY && p[1] <= r.maxY
}

// crossedBy reports whether a screen segment touches the box, either by ending
// inside it or by cutting across it.
func (r rect) crossedBy(p, q v2) bool {
	if r.holdsPoint(p) || r.holdsPoint(q) {
		return true
	}
	corners := [4]v2{
		{r.minX, r.minY}, {r.maxX, r.minY}, {r.maxX, r.maxY}, {r.minX, r.maxY},
	}
	for i := range corners {
		if segmentsCross(p, q, corners[i], corners[(i+1)%4]) {
			return true
		}
	}
	return false
}

// segmentsCross reports whether two screen segments properly intersect. It is
// the standard orientation test; a shared endpoint or a collinear touch counts
// as a crossing here, since either one means the text sits on the line.
func segmentsCross(p1, p2, q1, q2 v2) bool {
	d1 := cross(vsub(q2, q1), vsub(p1, q1))
	d2 := cross(vsub(q2, q1), vsub(p2, q1))
	d3 := cross(vsub(p2, p1), vsub(q1, p1))
	d4 := cross(vsub(p2, p1), vsub(q2, p1))
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	// Collinear or touching: the point-on-segment cases.
	return (d1 == 0 && onSegment(q1, q2, p1)) || (d2 == 0 && onSegment(q1, q2, p2)) ||
		(d3 == 0 && onSegment(p1, p2, q1)) || (d4 == 0 && onSegment(p1, p2, q2))
}

func cross(a, b v2) float64 { return a[0]*b[1] - a[1]*b[0] }

// onSegment reports whether r lies within the bounding box of segment p-q, which
// for a point already known to be collinear with it means on the segment.
func onSegment(p, q, r v2) bool {
	return math.Min(p[0], q[0]) <= r[0] && r[0] <= math.Max(p[0], q[0]) &&
		math.Min(p[1], q[1]) <= r[1] && r[1] <= math.Max(p[1], q[1])
}

// labelPlacer holds what a name has to stay clear of, and what has been placed
// so far.
type labelPlacer struct {
	a        *annCtx
	canvas   rect
	segments [][2]v2 // the drawing's own geometry, sampled
	markers  []rect  // the point markers
	placed   []rect  // the names already drawn
}

// newLabelPlacer samples the drawing once, so each name is scored against the
// same geometry rather than re-sampling per candidate.
func (s *Sketch) newLabelPlacer(a *annCtx, cfg svgConfig, tx, ty func(float64) float64, canvas rect) *labelPlacer {
	lp := &labelPlacer{a: a, canvas: canvas}
	for _, e := range s.ents {
		pts := entityPolyline(e, cfg.arcSegments)
		for i := 1; i < len(pts); i++ {
			lp.segments = append(lp.segments, [2]v2{
				{tx(pts[i-1][0]), ty(pts[i-1][1])},
				{tx(pts[i][0]), ty(pts[i][1])},
			})
		}
	}
	if cfg.showPoints {
		for _, p := range s.points {
			c := v2{tx(p.x()), ty(p.y())}
			lp.markers = append(lp.markers, rect{
				minX: c[0] - a.marker, minY: c[1] - a.marker,
				maxX: c[0] + a.marker, maxY: c[1] + a.marker,
			})
		}
	}
	return lp
}

// textAnchorStart and friends are the SVG text-anchor values a candidate can
// carry. They decide which way the text grows from the position written, which
// is what lets a name to the LEFT of its anchor end at the anchor rather than
// start there and run back over it.
const (
	textAnchorStart  = "start"
	textAnchorMiddle = "middle"
	textAnchorEnd    = "end"
)

// labelSpot is one place a name could go, as an offset from its anchor in
// multiples of the step size, with the alignment that suits that direction.
type labelSpot struct {
	dx, dy float64
	anchor string
}

// pointSpots and entitySpots are the rings tried, in order of preference.
//
// A point's first choice is up and to the right of its marker, which is where a
// draughtsman puts it and where every uncrowded drawing still has it. An
// entity's first choice is its own anchor, centred, since an entity has no
// marker of its own to clear. The rest of each ring walks the eight compass
// directions and then a second ring twice as far out, so a name that cannot sit
// in the obvious place moves as little as it has to.
var (
	pointSpots  = append(spotRing(1), spotRing(2)...)
	entitySpots = append([]labelSpot{{0, 0, textAnchorMiddle}}, append(spotRing(1), spotRing(2)...)...)
)

// spotRing is the eight compass positions at the given multiple of the step,
// starting up-right and going clockwise in screen terms (y grows downward).
func spotRing(n float64) []labelSpot {
	return []labelSpot{
		{n, -n, textAnchorStart},
		{n, 0, textAnchorStart},
		{n, n, textAnchorStart},
		{0, -n, textAnchorMiddle},
		{0, n, textAnchorMiddle},
		{-n, -n, textAnchorEnd},
		{-n, 0, textAnchorEnd},
		{-n, n, textAnchorEnd},
	}
}

// The weights the search scores a position by. A name on another name is the
// worst thing on the page, because two texts on one spot are both unreadable
// while a name over a line still reads; a name off the canvas is worse still,
// since it is not there at all. The rank term is what keeps an uncrowded drawing
// on its first choice and makes ties resolve the same way on every run.
const (
	labelPenaltyOffCanvas = 1000.0
	labelPenaltyOnLabel   = 100.0
	labelPenaltyOnMarker  = 10.0
	labelPenaltyOnCurve   = 1.0
	labelPenaltyRank      = 0.01
)

// place draws one name at the least-colliding position its ring offers.
func (lp *labelPlacer) place(anchor v2, name string, kind labelKind) {
	spots := pointSpots
	if kind == labelEntity {
		spots = entitySpots
	}
	step := lp.a.marker + lp.a.text*0.4

	best, bestBox, bestScore := spots[0], rect{}, math.Inf(1)
	for i, sp := range spots {
		box := lp.box(anchor, name, sp, step)
		if score := lp.score(box) + float64(i)*labelPenaltyRank; score < bestScore {
			best, bestBox, bestScore = sp, box, score
		}
	}

	lp.placed = append(lp.placed, bestBox)
	pos := vadd(anchor, v2{best.dx * step, best.dy * step})
	lp.a.nameText(pos, name, best.anchor, kind == labelEntity)
}

// box is the area a name would cover at one candidate position.
func (lp *labelPlacer) box(anchor v2, name string, sp labelSpot, step float64) rect {
	pos := vadd(anchor, v2{sp.dx * step, sp.dy * step})
	w, h := labelWidth(name, lp.a.text), lp.a.text
	var minX float64
	switch sp.anchor {
	case textAnchorMiddle:
		minX = pos[0] - w/2
	case textAnchorEnd:
		minX = pos[0] - w
	default:
		minX = pos[0]
	}
	return rect{minX: minX, minY: pos[1] - h/2, maxX: minX + w, maxY: pos[1] + h/2}
}

// score is how bad a position is: the weighted count of what it sits on.
func (lp *labelPlacer) score(box rect) float64 {
	score := 0.0
	if !lp.canvas.contains(box) {
		score += labelPenaltyOffCanvas
	}
	for _, p := range lp.placed {
		if box.overlaps(p) {
			score += labelPenaltyOnLabel
		}
	}
	for _, m := range lp.markers {
		if box.overlaps(m) {
			score += labelPenaltyOnMarker
		}
	}
	for _, seg := range lp.segments {
		if box.crossedBy(seg[0], seg[1]) {
			score += labelPenaltyOnCurve
		}
	}
	return score
}

// labelWidthPerRune is how wide one character is taken to be, as a fraction of
// the font size.
//
// It is an estimate and has to be: the exporter writes text for a viewer's own
// font, so no true advance width is knowable here. The value is sized for the
// digits and capitals sketch names are mostly made of, and it is deliberately on
// the generous side, since a box a little too wide moves a name that would have
// just fitted, while one too narrow lets two names overlap after the search
// reported they would not.
const labelWidthPerRune = 0.62

// labelWidth estimates the width of a name at a font size.
func labelWidth(name string, fontSize float64) float64 {
	return float64(len([]rune(name))) * fontSize * labelWidthPerRune
}

// nameText emits one name. It is the one place a label's text element is
// written, so the two kinds cannot drift apart in anything but the two
// differences they are meant to have: an entity's name is italic, and each kind
// starts from its own ring of candidate positions.
func (a *annCtx) nameText(pos v2, name, textAnchor string, italic bool) {
	style := ""
	if italic {
		style = ` font-style="italic"`
	}
	fmt.Fprintf(a.sb,
		`  <text x="%s" y="%s" font-size="%s" fill="%s" text-anchor="%s" dominant-baseline="central"%s>%s</text>`+"\n",
		a.sb.f(pos[0]), a.sb.f(pos[1]), a.sb.f(a.text), a.col, textAnchor, style, svgEscape(name))
}
