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
// their leaders, the point markers and the drawing's own geometry, and never off
// the edge of the canvas — is kept. On a crowded drawing some overlap is
// unavoidable, and the search then keeps the least bad position rather than
// refusing to label.
//
// A name is tied to its own geometry the way a CAD note is tied to a feature —
// a landing line along the text, a line off the end of that landing, an
// arrowhead on the vertex — whenever the search had to move it, or another point
// sits near enough that position alone cannot say which vertex is meant. A name
// beside its own geometry with nothing else near gets none of that. The landing
// takes the edge of the text that faces away from the vertex, so the line to the
// arrow never runs back through the name it belongs to. Names and their leaders
// are cleared against the page's own background colour, so they stay legible
// where they cross the drawing. SVG only.
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
	halo     string       // the page colour, painted under a name and its leader
	weights  labelWeights // what each kind of collision costs
	spots    labelSpots   // the positions a name is tried in
	segments [][2]v2      // the drawing's own geometry, sampled
	markers  []rect       // the point markers
	placed   []rect       // the names already drawn
	leaders  [][2]v2      // the leaders already drawn, landing and angled line alike
}

// newLabelPlacer samples the drawing once, so each name is scored against the
// same geometry rather than re-sampling per candidate.
func (s *Sketch) newLabelPlacer(a *annCtx, cfg svgConfig, tx, ty func(float64) float64, canvas rect) *labelPlacer {
	lp := &labelPlacer{
		a: a, canvas: canvas, halo: haloColor(cfg.background),
		weights: defaultLabelWeights(), spots: defaultLabelSpots(),
	}
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

// labelRingDepth is how many rings out a name may be moved.
//
// Two was the original depth, and on a crowded drawing it is not enough: names
// still land on each other and on vertices because nothing within two steps is
// free. Widening it alone makes a drawing WORSE, though, and that is the whole
// reason this constant did not simply grow. A name the search moves grows a
// leader, a name moved further grows a LONGER one, and leaders are obstacles
// for the names placed after them. Measured on an 80-name cloud, going from two
// rings to five left the number of leaders flat at 76 or 77 while their total
// length went from 977 to 1190, and the names crossed by one went from 9 to 16.
//
// The depth is only safe to raise alongside [labelPlacer.ownLeaderCost], which
// makes the search pay for the leader a candidate would need. With both, the
// same cloud places every one of its 80 names clear of every other name, every
// vertex and every leader.
const labelRingDepth = 5

// labelSpots is the ring a point's name is tried in and the ring an entity's
// is, held on the placer rather than read from a package variable so a test can
// vary the depth and measure the drawing that comes out.
type labelSpots struct{ point, entity []labelSpot }

// defaultLabelSpots is the rings at [labelRingDepth].
func defaultLabelSpots() labelSpots {
	p, e := newLabelSpots(labelRingDepth)
	return labelSpots{point: p, entity: e}
}

// newLabelSpots builds the rings a name is tried in, in order of preference.
//
// A point's first choice is up and to the right of its marker, which is where a
// draughtsman puts it and where every uncrowded drawing still has it. An
// entity's first choice is its own anchor, centred, since an entity has no
// marker of its own to clear. Each ring after that walks the eight compass
// directions one step further out, so a name that cannot sit in the obvious
// place still moves as little as it has to.
func newLabelSpots(depth int) ([]labelSpot, []labelSpot) {
	var point []labelSpot
	for r := 1; r <= depth; r++ {
		point = append(point, spotRing(float64(r))...)
	}
	entity := append([]labelSpot{{0, 0, textAnchorMiddle}}, point...)
	return point, entity
}

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

// labelWeights is what each kind of collision costs the search.
//
// It is a value rather than a set of constants read straight from [score] so a
// test can vary one weight and watch the drawing that comes out. What the
// numbers are worth cannot be argued from the page they are written on: the
// question "is a leader worth five curves" is answerable only by placing names
// on a crowded drawing twice and counting what each answer collides with. See
// [defaultLabelWeights] for the ordering they encode and
// labels_internal_test.go for the tests that hold it.
type labelWeights struct {
	offCanvas float64
	onLabel   float64
	onMarker  float64
	onLeader  float64
	onCurve   float64
	ownLeader float64
	rank      float64
}

// The weights the search scores a position by. A name on another name is the
// worst thing on the page, because two texts on one spot are both unreadable
// while a name over a line still reads; a name off the canvas is worse still,
// since it is not there at all. A leader sits between a marker and a curve: it
// hides no vertex, so it costs less than a marker, but it is the line a reader
// follows from a name to the vertex it belongs to, so a name dropped across it
// costs more than one crossing a construction line the drawing has anyway. The
// rank term is what keeps an uncrowded drawing on its first choice and makes
// ties resolve the same way on every run.
//
// The ORDER is what the tests hold and what a change here has to justify. The
// magnitudes inside that order are not pinned to any drawing, and deliberately
// so: no drawing in this repository can tell 1 from 100 for a single weight,
// only the ranking against its neighbours.
func defaultLabelWeights() labelWeights {
	return labelWeights{
		offCanvas: 1000.0,
		onLabel:   100.0,
		onMarker:  10.0,
		onLeader:  5.0,
		onCurve:   1.0,
		ownLeader: 5.0,
		rank:      0.01,
	}
}

// place draws one name at the least-colliding position its ring offers.
func (lp *labelPlacer) place(anchor v2, name string, kind labelKind) {
	spots := lp.spots.point
	if kind == labelEntity {
		spots = lp.spots.entity
	}
	step := lp.a.marker + lp.a.text*0.4

	best, bestBox, bestRank := lp.search(anchor, name, spots, step, nil)
	leadered := lp.needsLeader(bestBox, anchor, bestRank)
	if leadered && !lp.carriesLeader(bestBox, anchor) {
		// The name needs a leader and stands too close to its own vertex to
		// carry a visible one: the line would be a few pixels and the head
		// smaller still. Standing the name further off is what buys the room,
		// and it costs nothing a reader values — a name that needs a line drawn
		// to it is not being read by its position anyway. Only positions that
		// really can carry the leader are considered, so the search cannot
		// settle again on one that cannot.
		roomy := func(b rect) bool { return lp.carriesLeader(b, anchor) }
		if far, farBox, _ := lp.search(anchor, name, spots, step, roomy); farBox != (rect{}) {
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

// search returns the best-scoring candidate the filter admits, with the rank it
// was found at. A nil filter admits every candidate.
//
// The rank is the candidate's own index in the ring, so rank 0 means the name
// kept the position a reader expects. Ties resolve by that index, which is what
// keeps an uncrowded drawing on its first choice and makes two runs agree.
//
// When the filter admits nothing the returned box is the zero rect, which the
// caller checks: a name is never dropped for failing a filter, it keeps the
// position the unfiltered search gave it.
func (lp *labelPlacer) search(anchor v2, name string, spots []labelSpot, step float64, admit func(rect) bool) (labelSpot, rect, int) {
	best, bestBox, bestScore, bestRank := labelSpot{}, rect{}, math.Inf(1), 0
	for i, sp := range spots {
		box := lp.box(anchor, name, sp, step)
		if admit != nil && !admit(box) {
			continue
		}
		if score := lp.score(box) + lp.ownLeaderCost(box, anchor, i) + float64(i)*lp.weights.rank; score < bestScore {
			best, bestBox, bestScore, bestRank = sp, box, score, i
		}
	}
	return best, bestBox, bestRank
}

// ownLeaderCost is what the leader THIS candidate would need costs the drawing.
//
// Every other term scores where the text lands. Without this one the search
// picks a position blind to the line it is about to draw back to the vertex,
// which is exactly how a wider ring makes a drawing worse: a name free to roam
// takes a distant position it likes, and a long leader is then drawn across
// everything between the two. Charging the candidate for its own leader is what
// makes [labelRingDepth] safe to raise.
//
// Only names and leaders already down are counted, not the markers or the
// drawing's own curves: a leader is cased against the page and is MEANT to
// cross the drawing, while a leader over a name or tangled with another leader
// is the thing a reader cannot follow.
func (lp *labelPlacer) ownLeaderCost(box rect, anchor v2, rank int) float64 {
	if lp.weights.ownLeader == 0 || !lp.needsLeader(box, anchor, rank) {
		return 0
	}
	r, ok := lp.leaderRoute(box, anchor)
	if !ok {
		return 0
	}
	cost := 0.0
	for _, seg := range [2][2]v2{r.landing, {r.start, r.tip}} {
		for _, p := range lp.placed {
			if p.crossedBy(seg[0], seg[1]) {
				cost += lp.weights.ownLeader
			}
		}
		for _, o := range lp.leaders {
			if segmentsCross(seg[0], seg[1], o[0], o[1]) {
				cost += lp.weights.ownLeader
			}
		}
	}
	return cost
}

// carriesLeader reports whether the leader this position needs would be long
// enough to carry an arrowhead a reader can see.
//
// It asks [leaderRoute] for the line that would actually be drawn rather than
// measuring to the nearest edge of the text, and the difference is not academic.
// A name sitting directly above or below its vertex has its nearest edge close
// to it, but the leader leaves the END of a landing line off to one side, and
// once the landing moved to whichever edge faces away from the vertex, the line
// left over got shorter still. Three names on the bevel gear's own S10 drawing
// came out with arrowheads a quarter the size of every other one, because the
// old measure said they had room when the drawn line did not.
func (lp *labelPlacer) carriesLeader(box rect, anchor v2) bool {
	r, ok := lp.leaderRoute(box, anchor)
	return ok && r.reach >= lp.a.arrow*leaderMinReach
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
// feature: a landing line along one edge of the text, a line from the end of
// that landing, and an arrowhead on the vertex itself.
//
// The three parts are what make the pairing legible rather than merely present.
// A bare line from the text to the point was the first attempt and it failed on
// a crowded drawing: at one step of travel the visible segment is a few pixels,
// and the reader cannot see which end belongs to which name. The landing binds
// the line to ITS text — the line leaves the word, not the space near the word —
// and the arrowhead says which of the several nearby dots is the one meant.
//
// Every segment drawn is kept, because a leader is an obstacle for the names
// placed after it just as a marker or an earlier name is.
func (lp *labelPlacer) leader(box rect, anchor v2) {
	r, ok := lp.leaderRoute(box, anchor)
	if !ok {
		return // the text already sits against the marker
	}
	lp.leaderLine(r.landing[0], r.landing[1])
	lp.leaderLine(r.start, r.tip)
	lp.leaders = append(lp.leaders, r.landing, [2]v2{r.start, r.tip})
	// A short leader takes a proportionally shorter head, so the arrow cannot be
	// longer than the line it sits on.
	lp.a.arrowAtSize(r.tip, r.dir, math.Min(lp.a.arrow, r.reach*leaderArrowShare))
}

// leaderRoute is one way a leader could leave its text: a landing line along the
// top or the bottom edge of the name's box, and the end of that landing the
// angled line sets off from.
type leaderRoute struct {
	landing [2]v2   // the horizontal line along one edge of the text
	start   v2      // whichever end of it the angled line leaves
	tip     v2      // where the angled line stops, clear of the marker
	dir     v2      // the direction the arrowhead points
	reach   float64 // how long the angled line is
}

// leaderRoute picks the route that does not run back through the name it came
// from.
//
// A leader that crosses its own text is not a blemish but a misreading: the line
// is cased in the page colour so it stays visible over the drawing, so where it
// crosses a letter it ERASES part of it. The case that taught this had a name
// sitting directly below its vertex, where the line up to the arrow left the
// bottom-right of the word and re-entered it — the casing took the right-hand
// side out of an "O" and a reader saw a "C".
//
// The four routes are the two edges the landing can sit on crossed with the two
// ends it can leave from, tried in the order a draughtsman would: the underline
// first, since that is the conventional note and the one an uncrowded drawing
// keeps; then the overline, which is what reaches a vertex standing above the
// word without passing through it; and only then the far end of each, which
// means the line travels back past its own word and is ugly rather than wrong.
// When every route crosses — a vertex inside the name's own box — the first is
// drawn anyway, on the same reasoning the placement search keeps its least bad
// position: a crossed leader still pairs the name with its vertex.
func (lp *labelPlacer) leaderRoute(box rect, anchor v2) (leaderRoute, bool) {
	gap := lp.a.text * leaderLandingGap
	below := [2]v2{{box.minX, box.maxY + gap}, {box.maxX, box.maxY + gap}}
	above := [2]v2{{box.minX, box.minY - gap}, {box.maxX, box.minY - gap}}

	// The end of a landing that faces the anchor, so the angled line does not
	// have to travel back past the word it came from.
	near := 1
	if anchor[0] < (box.minX+box.maxX)/2 {
		near = 0
	}
	far := 1 - near

	var fallback leaderRoute
	var found bool
	for _, c := range [4]struct {
		landing [2]v2
		end     int
	}{{below, near}, {above, near}, {below, far}, {above, far}} {
		r, ok := lp.routeFrom(c.landing, c.end, anchor)
		if !ok {
			continue
		}
		if !box.crossedBy(r.start, r.tip) {
			return r, true
		}
		if !found {
			fallback, found = r, true
		}
	}
	return fallback, found
}

// routeFrom completes one candidate route, or reports that it cannot be drawn
// because the text already sits against the marker it would point at.
func (lp *labelPlacer) routeFrom(landing [2]v2, end int, anchor v2) (leaderRoute, bool) {
	start := landing[end]
	dir := vunit(vsub(anchor, start))
	if dir == (v2{}) {
		return leaderRoute{}, false
	}
	tip := vadd(anchor, vmul(dir, -(lp.a.marker+lp.a.text*leaderClearance)))
	reach := vlen(vsub(tip, start))
	if reach <= 0 {
		return leaderRoute{}, false
	}
	return leaderRoute{landing: landing, start: start, tip: tip, dir: dir, reach: reach}, true
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
	leaderMinReach       = 1.6  // a drawn leader is at least this many arrowheads long
	leaderHaloWidth      = 3.5  // how much wider the casing under a leader is than the leader
	leaderClearance      = 0.25 // gap left between the marker and the arrow's tip
	leaderLandingGap     = 0.1  // how far off the text's edge the landing line sits
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
		score += lp.weights.offCanvas
	}
	for _, p := range lp.placed {
		if box.overlaps(p) {
			score += lp.weights.onLabel
		}
	}
	for _, m := range lp.markers {
		if box.overlaps(m) {
			score += lp.weights.onMarker
		}
	}
	for _, seg := range lp.leaders {
		if box.crossedBy(seg[0], seg[1]) {
			score += lp.weights.onLeader
		}
	}
	for _, seg := range lp.segments {
		if box.crossedBy(seg[0], seg[1]) {
			score += lp.weights.onCurve
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
