package sketch

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// The placement search is tested from inside the package because what it
// promises is about BOXES — a name's own area against a marker's, another
// name's, and the canvas — and those are what the renderer computes and the
// rendered text only implies.

// placerFixture is a placer over an empty drawing at a known scale, so a test
// can say where the obstacles are.
func placerFixture(canvas rect) *labelPlacer {
	a := &annCtx{text: 4, marker: 2}
	return &labelPlacer{a: a, canvas: canvas, weights: defaultLabelWeights(), spots: defaultLabelSpots()}
}

// placedBox is the box the placer chose for one name, which is the last box it
// recorded.
func placedBox(lp *labelPlacer) rect { return lp.placed[len(lp.placed)-1] }

func TestLabelPlacerTakesItsFirstChoiceWhenFree(t *testing.T) {
	lp := placerFixture(rect{maxX: 1000, maxY: 1000})
	lp.a.sb = newSVGWriter()

	anchor := v2{500, 500}
	lp.place(anchor, "A", labelPoint)

	box := placedBox(lp)
	require.Greater(t, box.minX, anchor[0], "a point's name goes to the right of its marker")
	require.Less(t, box.maxY, anchor[1], "and above it, the y axis being flipped")
}

func TestLabelPlacerMovesOffAMarker(t *testing.T) {
	anchor := v2{500, 500}
	step := 2 + 4*0.4 // marker + 0.4 * text, the placer's own step

	// A marker sitting exactly where the first choice would put the name.
	blocked := placerFixture(rect{maxX: 1000, maxY: 1000})
	blocked.a.sb = newSVGWriter()
	blocked.markers = []rect{{
		minX: anchor[0], minY: anchor[1] - 2*step,
		maxX: anchor[0] + 40, maxY: anchor[1],
	}}
	blocked.place(anchor, "A", labelPoint)

	for _, m := range blocked.markers {
		require.False(t, placedBox(blocked).overlaps(m), "the name was moved clear of the marker")
	}
}

func TestLabelPlacerMovesOffAPlacedName(t *testing.T) {
	lp := placerFixture(rect{maxX: 1000, maxY: 1000})
	lp.a.sb = newSVGWriter()

	// Two anchors close enough that both names want the same area.
	lp.place(v2{500, 500}, "first", labelPoint)
	first := placedBox(lp)
	lp.place(v2{502, 500}, "second", labelPoint)
	second := placedBox(lp)

	require.False(t, first.overlaps(second), "two names must not be drawn on the same area")
}

func TestLabelPlacerStaysInsideTheCanvas(t *testing.T) {
	// A canvas that ends just past the anchor, so the first choice would run off
	// the right-hand edge and only a leftward position fits.
	canvas := rect{maxX: 60, maxY: 1000}
	lp := placerFixture(canvas)
	lp.a.sb = newSVGWriter()

	lp.place(v2{56, 500}, "a long name", labelPoint)
	require.True(t, canvas.contains(placedBox(lp)), "the name stays on the page")
}

// When every position collides, the search keeps the least bad one rather than
// dropping the name: a label that is hard to read still says more than no label.
func TestLabelPlacerLabelsAnyway(t *testing.T) {
	lp := placerFixture(rect{maxX: 1000, maxY: 1000})
	lp.a.sb = newSVGWriter()
	lp.markers = []rect{{minX: 0, minY: 0, maxX: 1000, maxY: 1000}}

	lp.place(v2{500, 500}, "A", labelPoint)
	require.Len(t, lp.placed, 1)
	require.Contains(t, lp.a.sb.String(), ">A<")
}

// A name beside its own geometry with nothing else near is already paired with
// it, and a leader there would be ink spent saying what the drawing says.
func TestLabelLeaderIsNotDrawnForALoneName(t *testing.T) {
	lp := placerFixture(rect{maxX: 1000, maxY: 1000})
	lp.a.sb = newSVGWriter()
	anchor := v2{500, 500}
	lp.markers = []rect{markerAt(lp, anchor)}

	lp.place(anchor, "A", labelPoint)
	require.NotContains(t, lp.a.sb.String(), "<line", "no leader for a name at its first choice")
}

// A name whose own vertex has a near neighbour cannot be paired by position at
// all: it sits up and to the right of its own dot and up and to the left of the
// next one. That name gets a leader even though the search never moved it.
func TestLabelLeaderIsDrawnWhenAnotherVertexIsAsNear(t *testing.T) {
	lp := placerFixture(rect{maxX: 1000, maxY: 1000})
	lp.a.sb = newSVGWriter()
	anchor := v2{500, 500}
	lp.markers = []rect{markerAt(lp, anchor), markerAt(lp, v2{507, 497})}

	lp.place(anchor, "A", labelPoint)
	require.Contains(t, lp.a.sb.String(), "<line", "the rival makes the pairing ambiguous")
}

// A name that needs a leader is stood off far enough to carry a visible one: a
// line of a few pixels with a smaller head on it says nothing.
//
// The standoff is measured on the line that is actually DRAWN, not on the gap
// to the nearest edge of the text. The two differ most for a name sitting
// straight above or below its vertex, which is how three names on a real
// drawing ended up with quarter-sized arrowheads.
func TestLabelStandsOffFarEnoughToCarryItsLeader(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rival v2
	}{
		{"a rival up and to the right", v2{507, 497}},
		{"a rival straight above", v2{500, 493}},
		{"a rival straight below", v2{500, 503}},
		{"a rival straight left", v2{497, 500}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lp := placerFixture(rect{maxX: 1000, maxY: 1000})
			lp.a.sb = newSVGWriter()
			lp.a.arrow = 2
			anchor := v2{500, 500}
			lp.markers = []rect{markerAt(lp, anchor), markerAt(lp, tc.rival)}

			lp.place(anchor, "A", labelPoint)

			// The second segment of a leader is the angled line the arrowhead
			// sits on. A name the placer decided needs no leader has none, and
			// there is nothing to measure.
			require.Len(t, lp.leaders, 2, "this rival should have forced a leader")
			line := lp.leaders[1]
			require.GreaterOrEqual(t, vlen(vsub(line[1], line[0])), lp.a.arrow*leaderMinReach,
				"the drawn leader is too short to carry a full-size arrowhead")
		})
	}
}

// Every arrowhead on a crowded drawing is full size. A head sized down to a
// quarter of the others reads as a smudge rather than as the thing saying which
// vertex a name belongs to.
func TestEveryArrowheadIsFullSize(t *testing.T) {
	lp, anchors := scatterFixture(scatterNames, scatterSpread)
	lp.a.sb = newSVGWriter()
	for i, anchor := range anchors {
		lp.place(anchor, fmt.Sprintf("P%d", i), labelPoint)
	}

	// lp.leaders holds the landing and then the angled line, per leader drawn.
	var short int
	for i := 1; i < len(lp.leaders); i += 2 {
		if vlen(vsub(lp.leaders[i][1], lp.leaders[i][0])) < lp.a.arrow*leaderMinReach {
			short++
		}
	}
	require.Zero(t, short, "%d of %d leaders are too short for a full arrowhead",
		short, len(lp.leaders)/2)
}

// A leader must never run back through the name it belongs to. Its lines are
// cased in the page colour so they stay visible over the drawing, so a crossing
// does not merely clutter the letters — it ERASES part of one. The drawing this
// came from had a name sitting directly below its vertex: the line up to the
// arrow left the bottom-right of the word and re-entered it, taking the
// right-hand side out of an "O" so that a reader saw a "C".
func TestLabelLeaderNeverCrossesItsOwnText(t *testing.T) {
	box := rect{minX: 100, minY: 96, maxX: 120, maxY: 104}
	// The eight directions a vertex can lie in, each far enough out that a leader
	// really is drawn. Straight above is the case that failed.
	for _, tc := range []struct {
		name   string
		anchor v2
	}{
		{"straight above", v2{110, 40}},
		{"straight below", v2{110, 160}},
		{"above and left", v2{60, 40}},
		{"above and right", v2{160, 40}},
		{"below and left", v2{60, 160}},
		{"below and right", v2{160, 160}},
		{"level and left", v2{40, 100}},
		{"level and right", v2{180, 100}},
		// Barely off the top edge, where the landing line and the vertex are
		// within a hair of each other.
		{"just above the top edge", v2{110, 94}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lp := placerFixture(rect{maxX: 1000, maxY: 1000})
			lp.a.sb = newSVGWriter()
			lp.a.arrow = 2

			lp.leader(box, tc.anchor)

			require.Len(t, lp.leaders, 2, "a landing line and the angled line off it")
			for _, seg := range lp.leaders {
				require.False(t, box.crossedBy(seg[0], seg[1]),
					"the leader runs through the name it points from")
			}
		})
	}
}

// The landing line stays flush against one edge of the text, whichever edge it
// takes, so the leader still reads as leaving the word rather than passing near
// it.
func TestLabelLeaderLandingHugsTheText(t *testing.T) {
	box := rect{minX: 100, minY: 96, maxX: 120, maxY: 104}
	gap := 4 * leaderLandingGap // 4 is the fixture's font size

	for _, tc := range []struct {
		name   string
		anchor v2
		wantY  float64
	}{
		{"a vertex below takes the underline", v2{110, 160}, box.maxY + gap},
		{"a vertex above takes the overline", v2{110, 40}, box.minY - gap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lp := placerFixture(rect{maxX: 1000, maxY: 1000})
			lp.a.sb = newSVGWriter()
			lp.a.arrow = 2

			lp.leader(box, tc.anchor)

			landing := lp.leaders[0]
			require.InDelta(t, box.minX, landing[0][0], 1e-9, "the landing spans the text")
			require.InDelta(t, box.maxX, landing[1][0], 1e-9)
			require.InDelta(t, tc.wantY, landing[0][1], 1e-9)
			require.InDelta(t, tc.wantY, landing[1][1], 1e-9)
		})
	}
}

// A leader already on the page is an obstacle for the names placed after it. It
// is the line a reader follows from a name to its vertex, so a later name
// dropped across it breaks a pairing the drawing has already made.
func TestLabelPlacerMovesOffAnEarlierLeader(t *testing.T) {
	anchor := v2{500, 500}
	step := 2 + 4*0.4 // marker + 0.4 * text, the placer's own step

	lp := placerFixture(rect{maxX: 1000, maxY: 1000})
	lp.a.sb = newSVGWriter()

	// A leader lying straight across the position the name would otherwise take.
	first := lp.box(anchor, "A", lp.spots.point[0], step)
	drawn := [2]v2{
		{first.minX - 10, (first.minY + first.maxY) / 2},
		{first.maxX + 10, (first.minY + first.maxY) / 2},
	}
	lp.leaders = [][2]v2{drawn}

	lp.place(anchor, "A", labelPoint)

	require.False(t, placedBox(lp).crossedBy(drawn[0], drawn[1]),
		"the name was moved clear of the leader")
}

// markerAt is the box a point marker covers at a screen position.
func markerAt(lp *labelPlacer, c v2) rect {
	return rect{
		minX: c[0] - lp.a.marker, minY: c[1] - lp.a.marker,
		maxX: c[0] + lp.a.marker, maxY: c[1] + lp.a.marker,
	}
}

// A name the search had to move is tied to its geometry the way a CAD note is
// tied to a feature: an underline under the text, a line off the end of it, and
// an arrowhead on the vertex.
func TestLabelLeaderUnderlinesTheNameAndPointsAtTheAnchor(t *testing.T) {
	lp := placerFixture(rect{maxX: 1000, maxY: 1000})
	lp.a.sb = newSVGWriter()
	anchor := v2{500, 500}

	// Block exactly the first choice, so the name has to go somewhere else and
	// the test does not depend on which of the remaining positions wins.
	step := lp.a.marker + lp.a.text*0.4
	lp.markers = []rect{lp.box(anchor, "A", lp.spots.point[0], step)}
	lp.place(anchor, "A", labelPoint)

	out := lp.a.sb.String()
	lines := leaderLines(t, out)
	require.Len(t, lines, 2, "an underline and the line off it")
	require.Contains(t, out, "<path", "and an arrowhead")

	box := placedBox(lp)
	// The tolerance is the exporter's own: it writes coordinates at four decimal
	// places, so a position read back off the document carries that rounding.
	const written = 1e-3

	underline := lines[0]
	require.InDelta(t, box.minX, underline[0][0], written, "the underline spans the text")
	require.InDelta(t, box.maxX, underline[1][0], written)
	require.InDelta(t, underline[0][1], underline[1][1], written, "and is level")
	require.Greater(t, underline[0][1], box.maxY, "sitting just under it")

	leader := lines[1]
	require.Contains(t, []v2{underline[0], underline[1]}, leader[0],
		"the line leaves one end of the underline")
	require.InDelta(t, lp.a.marker+lp.a.text*leaderClearance, vlen(vsub(leader[1], anchor)), written,
		"and stops clear of the marker it points at")
	require.False(t, box.holdsPoint(leader[1]), "it never runs under the letters")
}

// leaderLines reads every <line> back off a rendered fragment, as endpoint
// pairs in document order.
func leaderLines(t *testing.T, svg string) [][2]v2 {
	t.Helper()
	re := regexp.MustCompile(`<line x1="([^"]*)" y1="([^"]*)" x2="([^"]*)" y2="([^"]*)"`)
	var out [][2]v2
	for _, m := range re.FindAllStringSubmatch(svg, -1) {
		var v [4]float64
		for i := range v {
			f, err := strconv.ParseFloat(m[i+1], 64)
			require.NoError(t, err)
			v[i] = f
		}
		out = append(out, [2]v2{{v[0], v[1]}, {v[2], v[3]}})
	}
	return out
}

func TestClosestOnRect(t *testing.T) {
	box := rect{minX: 10, minY: 10, maxX: 20, maxY: 20}
	require.Equal(t, v2{10, 15}, closestOnRect(box, v2{0, 15}), "left of the box")
	require.Equal(t, v2{20, 20}, closestOnRect(box, v2{30, 30}), "past a corner")
	require.Equal(t, v2{15, 15}, closestOnRect(box, v2{15, 15}), "inside it")
}

func TestRectCrossedBySegment(t *testing.T) {
	box := rect{minX: 10, minY: 10, maxX: 20, maxY: 20}
	for _, tc := range []struct {
		name string
		p, q v2
		want bool
	}{
		{"straight through", v2{0, 15}, v2{30, 15}, true},
		{"diagonally through a corner", v2{0, 0}, v2{30, 30}, true},
		{"ending inside", v2{0, 15}, v2{15, 15}, true},
		{"wholly inside", v2{12, 12}, v2{18, 18}, true},
		{"touching an edge", v2{0, 10}, v2{10, 10}, true},
		{"clear of it", v2{0, 0}, v2{9, 0}, false},
		{"parallel and outside", v2{0, 25}, v2{30, 25}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, box.crossedBy(tc.p, tc.q))
		})
	}
}

func TestLabelWidthGrowsWithTheName(t *testing.T) {
	require.InDelta(t, 0, labelWidth("", 10), 1e-9)
	require.Greater(t, labelWidth("AA", 10), labelWidth("A", 10))
	require.InDelta(t, 2*labelWidth("A", 10), labelWidth("AA", 10), 1e-9)
	// Multi-byte names are measured in characters, not bytes.
	require.InDelta(t, labelWidth("AB", 10), labelWidth("Åß", 10), 1e-9)
	require.False(t, math.IsNaN(labelWidth("A", 0)))
}

// What the placement weights are worth cannot be argued from the constants
// themselves, and for a long time nothing here tried: the only evidence that
// any of them helped was one drawing in a downstream repository, which turned
// out to be insensitive to every weight it exercised. The tests below ask the
// question a reader would ask instead — place names on a crowded drawing, then
// count what they ended up sitting on — and they ask it of each weight in turn
// by TURNING THAT WEIGHT OFF and measuring what gets worse.

// labelCollisions is what the names ended up sitting on once every one of them
// is down: how many chosen boxes overlap another name, cover someone else's
// vertex, or cross a leader.
type labelCollisions struct{ onLabel, onMarker, onLeader int }

// scatterFixture is a deterministic irregular cloud of named vertices joined by
// a polyline.
//
// IRREGULAR is the load-bearing word. A uniform lattice was the obvious fixture
// and it cannot measure the marker weight at all: on a grid every position a
// name can reach is near somebody's vertex, so the weight shuffles which vertex
// gets covered without ever reducing the count. A cloud with open space in it
// gives a name somewhere better to go, which is the only condition under which
// a weight can be shown to help.
func scatterFixture(n int, spread float64) (*labelPlacer, []v2) {
	a := &annCtx{text: 4, marker: 2, arrow: 2, col: "#000", sb: newSVGWriter()}
	lp := &labelPlacer{
		a:       a,
		canvas:  rect{minX: -300, minY: -300, maxX: 900, maxY: 900},
		weights: defaultLabelWeights(),
		spots:   defaultLabelSpots(),
	}
	// A linear congruential generator rather than math/rand, so the cloud is the
	// same on every run and on every Go version.
	seed := uint64(12345)
	next := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64((seed>>33)%10000) / 10000.0
	}
	var anchors []v2
	for range n {
		c := v2{100 + next()*spread, 100 + next()*spread}
		anchors = append(anchors, c)
		lp.markers = append(lp.markers, rect{
			minX: c[0] - a.marker, minY: c[1] - a.marker,
			maxX: c[0] + a.marker, maxY: c[1] + a.marker,
		})
	}
	for i := 1; i < len(anchors); i++ {
		lp.segments = append(lp.segments, [2]v2{anchors[i-1], anchors[i]})
	}
	return lp, anchors
}

// placeScatter runs the whole cloud through the placer under one set of weights
// and counts what the chosen positions collide with.
//
// Every count is taken AFTER all names are down, against the finished drawing,
// rather than against the part of it that existed when each name was placed. A
// reader reads the finished page, so that is what the count has to measure. A
// name's own vertex is not counted: every position it can take is beside it.
func placeScatter(w labelWeights) ([]rect, labelCollisions) {
	lp, anchors := scatterFixture(scatterNames, scatterSpread)
	lp.weights = w
	for i, anchor := range anchors {
		lp.place(anchor, fmt.Sprintf("P%d", i), labelPoint)
	}

	return lp.placed, collisionsOf(lp)
}

// collisionsOf counts what the finished drawing's names ended up sitting on.
func collisionsOf(lp *labelPlacer) labelCollisions {
	var got labelCollisions
	for i, box := range lp.placed {
		for j, other := range lp.placed {
			if i != j && box.overlaps(other) {
				got.onLabel++
				break
			}
		}
		for j, m := range lp.markers {
			if j != i && box.overlaps(m) {
				got.onMarker++
				break
			}
		}
		for _, seg := range lp.leaders {
			if box.crossedBy(seg[0], seg[1]) {
				got.onLeader++
				break
			}
		}
	}
	return got
}

// The size of the cloud the weights are measured on. It is crowded enough that
// names collide and open enough that moving one can help; both halves are
// needed, and a much tighter or much looser cloud measures nothing.
const (
	scatterNames  = 80
	scatterSpread = 120.0
)

// Each weight has to EARN its place: switching it off must make the drawing
// measurably worse in the thing that weight exists to prevent. A weight that
// changes nothing when removed is not tuning, it is decoration, and this test
// is what tells the two apart.
//
// It deliberately does not pin the MAGNITUDES. No drawing in this repository
// can tell a leader weight of 1 from one of 100; only the ranking against its
// neighbours shows up, and [TestLabelWeightsRankHarmInOrder] is what holds
// that.
func TestLabelWeightsEarnTheirPlace(t *testing.T) {
	_, full := placeScatter(defaultLabelWeights())
	t.Logf("every weight on: %d names on a name, %d on someone else's vertex, %d on a leader",
		full.onLabel, full.onMarker, full.onLeader)

	for _, tc := range []struct {
		name  string
		off   func(*labelWeights)
		count func(labelCollisions) int
	}{
		{
			"a name landing on another name",
			func(w *labelWeights) { w.onLabel = 0 },
			func(c labelCollisions) int { return c.onLabel },
		},
		{
			"a name hiding someone else's vertex",
			func(w *labelWeights) { w.onMarker = 0 },
			func(c labelCollisions) int { return c.onMarker },
		},
		{
			"a name landing on a leader",
			func(w *labelWeights) { w.onLeader = 0 },
			func(c labelCollisions) int { return c.onLeader },
		},
		{
			"a name whose own leader would cross the drawing",
			func(w *labelWeights) { w.ownLeader = 0 },
			func(c labelCollisions) int { return c.onLeader },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := defaultLabelWeights()
			tc.off(&w)
			_, without := placeScatter(w)
			t.Logf("with the weight %d, with it off %d", tc.count(full), tc.count(without))
			require.Less(t, tc.count(full), tc.count(without),
				"switching this weight off left the drawing no worse, so the weight buys nothing")
		})
	}
}

// The weights encode an ORDER, and the order is the part a change has to
// justify. Each rung is a straight choice between two positions colliding with
// one thing each, so only the ranking decides it.
func TestLabelWeightsRankHarmInOrder(t *testing.T) {
	w := defaultLabelWeights()
	box := rect{minX: 10, minY: 10, maxX: 30, maxY: 20}
	page := rect{maxX: 100, maxY: 100}
	across := [2]v2{{box.minX - 5, (box.minY + box.maxY) / 2}, {box.maxX + 5, (box.minY + box.maxY) / 2}}

	clear := &labelPlacer{canvas: page, weights: w}
	onCurve := &labelPlacer{canvas: page, weights: w, segments: [][2]v2{across}}
	onLeader := &labelPlacer{canvas: page, weights: w, leaders: [][2]v2{across}}
	onMarker := &labelPlacer{canvas: page, weights: w, markers: []rect{box}}
	onLabel := &labelPlacer{canvas: page, weights: w, placed: []rect{box}}
	offPage := &labelPlacer{canvas: rect{maxX: 5, maxY: 5}, weights: w}

	require.Equal(t, 0.0, clear.score(box), "a position colliding with nothing is free")
	require.Less(t, onCurve.score(box), onLeader.score(box),
		"crossing the drawing's own line beats crossing a leader")
	require.Less(t, onLeader.score(box), onMarker.score(box),
		"crossing a leader beats hiding a vertex")
	require.Less(t, onMarker.score(box), onLabel.score(box),
		"hiding a vertex beats making two names unreadable")
	require.Less(t, onLabel.score(box), offPage.score(box),
		"any collision beats not being on the page at all")
}

// The rank term breaks ties and does nothing else. Walking the entire ring has
// to cost less than the cheapest real collision, or a name would accept a worse
// position to keep a place nearer the front of the ring.
func TestLabelRankOnlyBreaksTies(t *testing.T) {
	w := defaultLabelWeights()
	point, _ := newLabelSpots(labelRingDepth)
	wholeRing := float64(len(point)) * w.rank
	require.Less(t, wholeRing, w.onCurve,
		"walking the whole ring must cost less than one collision, or rank would outrank harm")
}

// Placement has to land in the same place on every run, since the drawing is
// committed to a repository and a diff that moves on its own is unreadable.
// The search feeds on what has already been placed, so the guarantee rests on
// walking the geometry in a fixed order, not on the scoring alone.
func TestLabelPlacementIsDeterministic(t *testing.T) {
	first, firstCounts := placeScatter(defaultLabelWeights())
	second, secondCounts := placeScatter(defaultLabelWeights())
	require.Equal(t, first, second, "the same drawing placed twice moved a name")
	require.Equal(t, firstCounts, secondCounts)
}

// A name may be moved as far as [labelRingDepth] rings, and that depth is only
// safe because the search pays for the leader each candidate would need. The
// two are one change and this is the test that says so: widening the ring while
// the own-leader cost is off leaves the drawing WORSE than not widening it.
func TestDeeperRingNeedsTheOwnLeaderCost(t *testing.T) {
	place := func(depth int, own float64) labelCollisions {
		lp, anchors := scatterFixture(scatterNames, scatterSpread)
		point, entity := newLabelSpots(depth)
		lp.spots = labelSpots{point: point, entity: entity}
		lp.weights.ownLeader = own
		for i, anchor := range anchors {
			lp.place(anchor, fmt.Sprintf("P%d", i), labelPoint)
		}
		return collisionsOf(lp)
	}

	shallow := place(2, 0)
	wideAlone := place(labelRingDepth, 0)
	wideWithCost := place(labelRingDepth, defaultLabelWeights().ownLeader)
	t.Logf("two rings %+v, %d rings alone %+v, %d rings with the cost %+v",
		shallow, labelRingDepth, wideAlone, labelRingDepth, wideWithCost)

	require.Greater(t, wideAlone.onLeader, shallow.onLeader,
		"widening the ring on its own should cost leader collisions, which is why the cost exists")
	require.LessOrEqual(t, wideWithCost.onLabel, shallow.onLabel)
	require.LessOrEqual(t, wideWithCost.onMarker, shallow.onMarker)
	require.LessOrEqual(t, wideWithCost.onLeader, shallow.onLeader)
	require.Less(t, wideWithCost.onLeader, wideAlone.onLeader,
		"the own-leader cost is what pays for the wider ring")
}
