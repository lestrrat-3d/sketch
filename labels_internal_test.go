package sketch

import (
	"math"
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
	return &labelPlacer{a: a, canvas: canvas}
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
