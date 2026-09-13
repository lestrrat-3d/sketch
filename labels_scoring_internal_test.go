package sketch

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

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
