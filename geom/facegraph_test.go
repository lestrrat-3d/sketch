package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func rectLines(x0, y0, x1, y1 float64) []geom.Curve {
	a, b, c, d := geom.NewPoint(x0, y0), geom.NewPoint(x1, y0), geom.NewPoint(x1, y1), geom.NewPoint(x0, y1)
	return []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
}

func faceGraphOf(t *testing.T, curves []geom.Curve, closed []geom.ClosedCurve) *geom.Arrangement {
	t.Helper()
	arr := geom.Regions(curves, closed, geom.WithFaceGraph())
	require.False(t, arr.Degenerate)
	require.NotNil(t, arr.FaceGraph)
	require.Len(t, arr.FaceGraph.Faces, len(arr.Regions))
	return arr
}

// regionAt returns the index of the region whose bounding box contains the point
// and whose area is the one given, so a test names a region without relying on order.
func regionWithArea(t *testing.T, arr *geom.Arrangement, area float64, minX float64) int {
	t.Helper()
	for i, r := range arr.Regions {
		if math.Abs(r.Area-area) > 1e-6 {
			continue
		}
		lo := math.Inf(1)
		for _, e := range r.Outer {
			for _, p := range e.Polyline {
				lo = math.Min(lo, p[0])
			}
		}
		if math.Abs(lo-minX) < 1e-9 {
			return i
		}
	}
	t.Fatalf("no region of area %v at min x %v", area, minX)
	return -1
}

func TestFaceGraphIsAbsentWithoutTheOption(t *testing.T) {
	arr := geom.Regions(rectLines(0, 0, 4, 4), nil)
	require.Nil(t, arr.FaceGraph)
}

func TestWalkLoopsOverlappingRectangles(t *testing.T) {
	curves := append(rectLines(0, 0, 4, 4), rectLines(2, 2, 6, 6)...)
	arr := faceGraphOf(t, curves, nil)
	require.Len(t, arr.Regions, 3)
	walks := arr.FaceGraph.WalkLoops([][]int{{0, 1, 2, 3}, {4, 5, 6, 7}})
	aOnly := regionWithArea(t, arr, 12, 0)
	bOnly := regionWithArea(t, arr, 12, 2)
	both := regionWithArea(t, arr, 4, 2)
	for k, w := range walks {
		require.Equalf(t, geom.LoopDefectNone, w.Defect, "loop %d", k)
	}
	require.Equal(t, [3]int8{1, 0, 1}, [3]int8{walks[0].Side[aOnly], walks[0].Side[bOnly], walks[0].Side[both]})
	require.Equal(t, [3]int8{0, 1, 1}, [3]int8{walks[1].Side[aOnly], walks[1].Side[bOnly], walks[1].Side[both]})
}

func TestWalkLoopsNestedRectangles(t *testing.T) {
	curves := append(rectLines(0, 0, 10, 10), rectLines(3, 3, 6, 6)...)
	arr := faceGraphOf(t, curves, nil)
	require.Len(t, arr.Regions, 2)
	walks := arr.FaceGraph.WalkLoops([][]int{{0, 1, 2, 3}, {4, 5, 6, 7}})
	annulus := regionWithArea(t, arr, 100-9, 0)
	inner := regionWithArea(t, arr, 9, 3)
	require.Equal(t, [2]int8{1, 1}, [2]int8{walks[0].Side[annulus], walks[0].Side[inner]})
	require.Equal(t, [2]int8{0, 1}, [2]int8{walks[1].Side[annulus], walks[1].Side[inner]})
}

func TestWalkLoopsRectanglesSharingASide(t *testing.T) {
	left := rectLines(0, 0, 3, 3)
	// The second rectangle reuses the first one's right side coordinates, drawn
	// again as its own line.
	right := rectLines(3, 0, 6, 3)
	arr := faceGraphOf(t, append(left, right...), nil)
	require.Len(t, arr.Regions, 2)
	walks := arr.FaceGraph.WalkLoops([][]int{{0, 1, 2, 3}, {4, 5, 6, 7}})
	l := regionWithArea(t, arr, 9, 0)
	r := regionWithArea(t, arr, 9, 3)
	require.Equal(t, [2]int8{1, 0}, [2]int8{walks[0].Side[l], walks[0].Side[r]})
	require.Equal(t, [2]int8{0, 1}, [2]int8{walks[1].Side[l], walks[1].Side[r]})
	for k, w := range walks {
		require.Equalf(t, geom.LoopDefectNone, w.Defect, "loop %d", k)
	}
}

func TestWalkLoopsOpenChainIsAGap(t *testing.T) {
	// A square with its last side missing, plus a closed square elsewhere so the
	// arrangement has a region at all.
	curves := rectLines(0, 0, 4, 4)[:3]
	curves = append(curves, rectLines(10, 10, 12, 12)...)
	arr := faceGraphOf(t, curves, nil)
	walks := arr.FaceGraph.WalkLoops([][]int{{0, 1, 2}, {3, 4, 5, 6}})
	require.Equal(t, geom.LoopDefectGap, walks[0].Defect)
	require.Equal(t, geom.LoopDefectNone, walks[1].Defect)
}

func TestWalkLoopsBowtieIsSelfCrossing(t *testing.T) {
	a, b, c, d := geom.NewPoint(0, 0), geom.NewPoint(4, 4), geom.NewPoint(4, 0), geom.NewPoint(0, 4)
	curves := []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
	arr := geom.Regions(curves, nil, geom.WithFaceGraph())
	require.NotNil(t, arr.FaceGraph)
	walks := arr.FaceGraph.WalkLoops([][]int{{0, 1, 2, 3}})
	require.Equal(t, geom.LoopDefectSelfCrossing, walks[0].Defect)
}

func TestWalkLoopsCircleWithCoincidentArcRegion(t *testing.T) {
	// A disc and a region that leaves it: two radial lines, a tip arc and a root arc
	// lying on the disc's circle. The root arc is suppressed in favour of the circle
	// (or the reverse), and both loops must still cover the shared span.
	c := geom.NewPoint(0, 0)
	a := 0.3
	pt := func(r, ang float64) *geom.Point { return geom.NewPoint(r*math.Cos(ang), r*math.Sin(ang)) }
	rootR, rootL := pt(3, -a), pt(3, a)
	tipR, tipL := pt(4, -a), pt(4, a)
	curves := []geom.Curve{
		geom.NewLine(rootR, tipR), geom.NewArc(c, tipR, tipL), geom.NewLine(tipL, rootL), geom.NewArc(c, rootR, rootL),
	}
	closed := []geom.ClosedCurve{geom.NewCircle(c, 3)}
	arr := faceGraphOf(t, curves, closed)
	require.Len(t, arr.Regions, 2)
	// sources: 0..3 the region, 4 the circle
	walks := arr.FaceGraph.WalkLoops([][]int{{0, 1, 2, 3}, {4}})
	for k, w := range walks {
		require.Equalf(t, geom.LoopDefectNone, w.Defect, "loop %d", k)
	}
	var disc, tooth int
	if arr.Regions[0].Area > arr.Regions[1].Area {
		disc, tooth = 0, 1
	} else {
		disc, tooth = 1, 0
	}
	require.Equal(t, [2]int8{0, 1}, [2]int8{walks[0].Side[disc], walks[0].Side[tooth]}, "region loop")
	require.Equal(t, [2]int8{1, 0}, [2]int8{walks[1].Side[disc], walks[1].Side[tooth]}, "disc loop")
}

func TestFaceGraphOptionChangesNoRegion(t *testing.T) {
	curves := append(rectLines(0, 0, 4, 4), rectLines(2, 2, 6, 6)...)
	closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(8, 3), 2.5)}
	plain := geom.Regions(curves, closed)
	with := geom.Regions(curves, closed, geom.WithFaceGraph())
	require.Nil(t, plain.FaceGraph)
	require.Equal(t, plain.Degenerate, with.Degenerate)
	require.Len(t, with.Regions, len(plain.Regions))
	for i := range plain.Regions {
		a, b := plain.Regions[i], with.Regions[i]
		require.Equal(t, a.Area, b.Area)
		require.Equal(t, a.Degenerate, b.Degenerate)
		require.Len(t, b.Outer, len(a.Outer))
		for j := range a.Outer {
			require.Equal(t, a.Outer[j].SourceIndex, b.Outer[j].SourceIndex)
			require.Equal(t, a.Outer[j].TStart, b.Outer[j].TStart)
			require.Equal(t, a.Outer[j].TEnd, b.Outer[j].TEnd)
			require.Equal(t, a.Outer[j].TExact, b.Outer[j].TExact)
		}
	}
	require.Len(t, plain.Chains, len(with.Chains))
}
