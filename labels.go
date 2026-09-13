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
// least bad position rather than refusing to label.
//
// A name is tied to its own geometry the way a CAD note is tied to a feature —
// underlined text, a line off the end of that underline, an arrowhead on the
// vertex — whenever the search had to move it, or another point sits near enough
// that position alone cannot say which vertex is meant. A name beside its own
// geometry with nothing else near gets none of that. Names and their leaders are
// cleared against the page's own background colour, so they stay legible where
// they cross the drawing. SVG only.
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
	halo     string  // the page colour, painted under a name and its leader
	segments [][2]v2 // the drawing's own geometry, sampled
	markers  []rect  // the point markers
	placed   []rect  // the names already drawn
}

// newLabelPlacer samples the drawing once, so each name is scored against the
// same geometry rather than re-sampling per candidate.
func (s *Sketch) newLabelPlacer(a *annCtx, cfg svgConfig, tx, ty func(float64) float64, canvas rect) *labelPlacer {
	lp := &labelPlacer{a: a, canvas: canvas, halo: haloColor(cfg.background)}
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

	best, bestBox, bestRank := lp.search(anchor, name, spots, step, 0)
	leadered := lp.needsLeader(bestBox, anchor, bestRank)
	if leadered && lp.reach(bestBox, anchor) < lp.a.arrow*leaderMinReach {
		// The name needs a leader and is too close to its own anchor to carry a
		// visible one: the line would be a few pixels and the head smaller
		// still. Standing the name off by another step is what buys the room,
		// and it costs nothing a reader values — a name that needs a line drawn
		// to it is not being read by its position anyway.
		if far, farBox, _ := lp.search(anchor, name, spots, step, outerRingStep); farBox != (rect{}) {
			best, bestBox = far, farBox
		}
	}

	lp.placed = append(lp.placed, bestBox)
	pos := vadd(anchor, v2{best.dx * step, best.dy * step})
	if lp.halo != "" {
		lp.a.nameHalo(pos, name, best.anchor, kind == labelEntity, lp.halo)
	}
	lp.a.nameText(pos, name, best.anchor, kind == labelEntity)
	if leadered {
		lp.leader(bestBox, anchor)
	}
}

// search returns the best-scoring candidate among those standing at least
// minStep out from the anchor, with the rank it was found at.
//
// The rank is the candidate's own index in the ring, so rank 0 means the name
// kept the position a reader expects. Ties resolve by that index, which is what
// keeps an uncrowded drawing on its first choice and makes two runs agree.
func (lp *labelPlacer) search(anchor v2, name string, spots []labelSpot, step, minStep float64) (labelSpot, rect, int) {
	best, bestBox, bestScore, bestRank := labelSpot{}, rect{}, math.Inf(1), 0
	for i, sp := range spots {
		if math.Max(math.Abs(sp.dx), math.Abs(sp.dy)) < minStep {
			continue
		}
		box := lp.box(anchor, name, sp, step)
		if score := lp.score(box) + float64(i)*labelPenaltyRank; score < bestScore {
			best, bestBox, bestScore, bestRank = sp, box, score, i
		}
	}
	return best, bestBox, bestRank
}

// reach is how much line a leader would have between the marker's clearance and
// the text's own box.
func (lp *labelPlacer) reach(box rect, anchor v2) float64 {
	gap := vlen(vsub(closestOnRect(box, anchor), anchor))
	return gap - (lp.a.marker + lp.a.text*leaderClearance)
}

// needsLeader reports whether a name has to be tied to its geometry explicitly.
//
// Two things call for it. A name the search MOVED is no longer where a reader
// looks for it. And a name with a RIVAL — another point whose marker is nearly
// as close to the text as its own anchor — cannot be paired by proximity at all,
// however conventional its position: on a lattice of two dozen points, a name
// sitting up and to the right of its own dot is also sitting up and to the left
// of the next one, and the reader has no way to choose.
//
// A name with neither problem is beside its own geometry with nothing else near,
// and a leader on it would be ink spent saying what the drawing already says.
func (lp *labelPlacer) needsLeader(box rect, anchor v2, rank int) bool {
	if rank > 0 {
		return true
	}
	own := vlen(vsub(closestOnRect(box, anchor), anchor))
	for _, m := range lp.markers {
		centre := v2{(m.minX + m.maxX) / 2, (m.minY + m.maxY) / 2}
		if vlen(vsub(centre, anchor)) < 1e-9 {
			continue // the name's own marker
		}
		if vlen(vsub(closestOnRect(box, centre), centre)) <= own*leaderRivalRatio {
			return true
		}
	}
	return false
}

// leader ties a name to the geometry it names, as a CAD note is tied to a
// feature: an underline under the text, a line from the end of that underline,
// and an arrowhead on the vertex itself.
//
// The three parts are what make the pairing legible rather than merely present.
// A bare line from the text to the point was the first attempt and it failed on
// a crowded drawing: at one step of travel the visible segment is a few pixels,
// and the reader cannot see which end belongs to which name. The underline binds
// the line to ITS text — the line leaves the word, not the space near the word —
// and the arrowhead says which of the several nearby dots is the one meant.
func (lp *labelPlacer) leader(box rect, anchor v2) {
	// The underline sits just under the text, spanning its width.
	y := box.maxY + lp.a.text*leaderUnderlineDrop
	left, right := v2{box.minX, y}, v2{box.maxX, y}

	// The line leaves the underline at whichever end faces the anchor, so it
	// never has to cross back under the word it came from.
	start := right
	if anchor[0] < (box.minX+box.maxX)/2 {
		start = left
	}
	dir := vunit(vsub(anchor, start))
	if dir == (v2{}) {
		return
	}
	tip := vadd(anchor, vmul(dir, -(lp.a.marker+lp.a.text*leaderClearance)))
	reach := vlen(vsub(tip, start))
	if reach <= 0 {
		return // the text already sits against the marker
	}

	lp.leaderLine(left, right)
	lp.leaderLine(start, tip)
	// A short leader takes a proportionally shorter head, so the arrow cannot be
	// longer than the line it sits on.
	lp.a.arrowAtSize(tip, dir, math.Min(lp.a.arrow, reach*leaderArrowShare))
}

// leaderLine emits one hairline of a leader, thinner than the geometry so the
// annotation does not read as another edge of the drawing.
//
// It is painted twice: once wide in the page's own colour, then the line itself
// on top. The casing is what makes the leader visible where it crosses the
// drawing — a hairline laid along a dashed construction line is lost in the
// dashes, and on a lattice that is exactly where leaders run. A page with no
// background of its own gets no casing, since there is no colour to clear with.
func (lp *labelPlacer) leaderLine(p, q v2) {
	if lp.halo != "" {
		lp.a.strokeLine(p, q, lp.halo, lp.a.sw*leaderStrokeFraction*leaderHaloWidth)
	}
	lp.a.strokeLine(p, q, lp.a.col, lp.a.sw*leaderStrokeFraction)
}

func (a *annCtx) strokeLine(p, q v2, stroke string, width float64) {
	fmt.Fprintf(a.sb,
		`  <line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"/>`+"\n",
		a.sb.f(p[0]), a.sb.f(p[1]), a.sb.f(q[0]), a.sb.f(q[1]), stroke, a.sb.f(width))
}

// haloColor is the colour a name and its leader are cleared against: the page's
// own background, or nothing when the page is transparent.
func haloColor(background string) string {
	if background == "" || background == "none" {
		return ""
	}
	return background
}

// closestOnRect is the point of a box nearest p, which is p itself when p is
// inside the box.
func closestOnRect(r rect, p v2) v2 {
	return v2{
		math.Min(math.Max(p[0], r.minX), r.maxX),
		math.Min(math.Max(p[1], r.minY), r.maxY),
	}
}

// The leader's own sizes, each as a fraction of the font size, the stroke width
// or the arrowhead, so they scale with the drawing like every other annotation.
const (
	leaderRivalRatio     = 2.0  // how near another marker may come before a name needs its leader
	leaderMinReach       = 1.6  // a leadered name stands off this many arrowheads at least
	outerRingStep        = 2    // the ring a name is pushed out to when it needs the room
	leaderHaloWidth      = 3.5  // how much wider the casing under a leader is than the leader
	leaderClearance      = 0.25 // gap left between the marker and the arrow's tip
	leaderUnderlineDrop  = 0.1  // how far under the text the underline sits
	leaderArrowShare     = 0.5  // the most of a leader's length its head may take
	leaderStrokeFraction = 0.6  // thinner than the geometry it points into
)

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
	fmt.Fprintf(a.sb,
		`  <text x="%s" y="%s" font-size="%s" fill="%s" text-anchor="%s" dominant-baseline="central"%s>%s</text>`+"\n",
		a.sb.f(pos[0]), a.sb.f(pos[1]), a.sb.f(a.text), a.col, textAnchor, italicStyle(italic), svgEscape(name))
}

// nameHalo paints the same name in the page's colour, thickened, so the letters
// that follow it sit in a clearing of their own rather than on top of whatever
// the drawing has there. It is emitted immediately before the name, and drawn as
// a stroked copy rather than through paint-order, which not every renderer that
// takes this SVG supports.
func (a *annCtx) nameHalo(pos v2, name, textAnchor string, italic bool, halo string) {
	fmt.Fprintf(a.sb,
		`  <text x="%s" y="%s" font-size="%s" fill="%s" stroke="%s" stroke-width="%s" stroke-linejoin="round" text-anchor="%s" dominant-baseline="central"%s>%s</text>`+"\n",
		a.sb.f(pos[0]), a.sb.f(pos[1]), a.sb.f(a.text), halo, halo,
		a.sb.f(a.text*labelHaloWidth), textAnchor, italicStyle(italic), svgEscape(name))
}

func italicStyle(italic bool) string {
	if italic {
		return ` font-style="italic"`
	}
	return ""
}

// labelHaloWidth is how wide the clearing around a name's letters is, as a
// fraction of the font size.
const labelHaloWidth = 0.22
