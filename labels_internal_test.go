package sketch

import (
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
func TestLabelStandsOffFarEnoughToCarryItsLeader(t *testing.T) {
	lp := placerFixture(rect{maxX: 1000, maxY: 1000})
	lp.a.sb = newSVGWriter()
	lp.a.arrow = 2
	anchor := v2{500, 500}
	lp.markers = []rect{markerAt(lp, anchor), markerAt(lp, v2{507, 497})}

	lp.place(anchor, "A", labelPoint)
	require.GreaterOrEqual(t, lp.reach(placedBox(lp), anchor), lp.a.arrow*leaderMinReach,
		"the name stands clear of its own marker by more than its arrowhead")
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
