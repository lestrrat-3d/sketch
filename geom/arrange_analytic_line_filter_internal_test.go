package geom

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyticLinePairFilterMatchesUnfilteredArrangement(t *testing.T) {
	curves, closed := regularPolygonFixture(192, 50)
	closed = append(closed, NewCircle(NewPoint(0, 0), 10))
	scenes := []struct {
		name   string
		curves []Curve
		closed []ClosedCurve
		cfg    arrangeConfig
	}{
		{name: "polygon192", curves: curves, closed: closed},
		{name: "disjoint-lines", curves: append(append([]Curve{}, curves[:24]...), NewLine(NewPoint(200, 200), NewPoint(210, 210)))},
		{name: "reordered", curves: reverseCurves(curves[:32])},
		{name: "overlapping-lines", curves: []Curve{
			NewLine(NewPoint(0, 0), NewPoint(10, 0)),
			NewLine(NewPoint(4, 0), NewPoint(14, 0)),
			NewLine(NewPoint(30, 0), NewPoint(30, 10)),
		}},
		{name: "custom-merge", curves: curves[:32], cfg: arrangeConfig{vertexMerge: 0.005}},
	}
	for _, scene := range scenes {
		t.Run(scene.name, func(t *testing.T) {
			filtered := regionsWithBudgetPairFilter(scene.curves, scene.closed, scene.cfg, nil, true)
			unfiltered := regionsWithBudgetPairFilter(scene.curves, scene.closed, scene.cfg, nil, false)
			requireSameArrangement(t, filtered, unfiltered)
			requireSameLinePairPrepass(t, scene.curves, scene.closed, scene.cfg)
		})
	}
}

func TestAnalyticLinePairFilterDoesNotChangeLineCircleAmbiguity(t *testing.T) {
	line := lineSource(lineOperand(100, 1.00005, 101, 1.00005))
	circle := &source{kind: srcCircle, cx: 0, cy: 0, r: 1}
	lineOp, circleOp := operandOf(line), operandOf(circle)
	events, ambiguous, ok := analyticEventsOf(line, circle, lineOp, circleOp, 101)
	require.True(t, ok)
	require.True(t, ambiguous)
	require.Empty(t, events)
	require.False(t, filterLinePair(lineOp, circleOp, 101, 0))
}

func TestAnalyticLinePairFilterPreservesBoundedCancellation(t *testing.T) {
	curves, _ := regularPolygonFixture(64, 50)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var work uint64
	arr, err := RegionsBounded(ctx, curves, nil, func(w, _, _ uint64) error {
		work += w
		if work >= 5000 {
			cancel()
		}
		return nil
	})
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, arr)
}

func requireSameLinePairPrepass(t *testing.T, curves []Curve, closed []ClosedCurve, cfg arrangeConfig) {
	t.Helper()
	filtered := newArranger(curves, closed, cfg)
	filtered.densify()
	filtered.intersectWithLinePairFilter(true)
	unfiltered := newArranger(curves, closed, cfg)
	unfiltered.densify()
	unfiltered.intersectWithLinePairFilter(false)
	require.Equal(t, len(unfiltered.events), len(filtered.events))
	for key, want := range unfiltered.events {
		got, ok := filtered.events[key]
		require.True(t, ok, "event pair %v", key)
		require.Equal(t, len(want), len(got), "event pair %v", key)
		for i := range want {
			requireSameEvent(t, got[i], want[i])
		}
	}
	require.Equal(t, len(unfiltered.deferredCross), len(filtered.deferredCross))
	for key, want := range unfiltered.deferredCross {
		got, ok := filtered.deferredCross[key]
		require.True(t, ok, "deferred pair %v", key)
		require.Equal(t, len(want), len(got), "deferred pair %v", key)
		for i := range want {
			requireSameEvent(t, got[i], want[i])
		}
	}
}

func requireSameEvent(t *testing.T, got, want xEvent) {
	t.Helper()
	require.Equal(t, got.x, want.x)
	require.Equal(t, got.y, want.y)
	require.Equal(t, got.ti, want.ti)
	require.Equal(t, got.tj, want.tj)
	require.Equal(t, got.kind, want.kind)
	require.Equal(t, got.circleBranch, want.circleBranch)
	require.Equal(t, got.overlap == nil, want.overlap == nil)
	if got.overlap != nil && want.overlap != nil {
		require.Equal(t, *got.overlap, *want.overlap)
	}
}

func requireSameArrangement(t *testing.T, got, want *Arrangement) {
	t.Helper()
	require.Equal(t, got.Degenerate, want.Degenerate)
	require.Equal(t, got.Degeneracies, want.Degeneracies)
	require.Equal(t, got.SelfIntersections, want.SelfIntersections)
	require.Equal(t, len(got.Regions), len(want.Regions))
	for i := range want.Regions {
		g, w := got.Regions[i], want.Regions[i]
		require.Equal(t, g.Area, w.Area, "region %d", i)
		require.Equal(t, g.SelfIntersecting, w.SelfIntersecting, "region %d", i)
		require.Equal(t, g.Degenerate, w.Degenerate, "region %d", i)
		requireSameEdges(t, g.Outer, w.Outer)
		require.Equal(t, len(g.Holes), len(w.Holes), "region %d holes", i)
		for j := range w.Holes {
			requireSameEdges(t, g.Holes[j], w.Holes[j])
		}
	}
	require.Equal(t, len(got.Chains), len(want.Chains))
	for i := range want.Chains {
		g, w := got.Chains[i], want.Chains[i]
		require.Equal(t, g.Length, w.Length, "chain %d", i)
		require.Equal(t, g.SelfIntersecting, w.SelfIntersecting, "chain %d", i)
		require.Equal(t, g.Degenerate, w.Degenerate, "chain %d", i)
		requireSameEdges(t, g.Edges, w.Edges)
	}
}

func requireSameEdges(t *testing.T, got, want []BoundaryEdge) {
	t.Helper()
	require.Equal(t, len(got), len(want))
	for i := range want {
		g, w := got[i], want[i]
		require.Equal(t, g.SourceIndex, w.SourceIndex, "edge %d", i)
		require.Equal(t, g.Whole, w.Whole, "edge %d", i)
		require.Equal(t, g.Reversed, w.Reversed, "edge %d", i)
		require.Equal(t, g.TStart, w.TStart, "edge %d", i)
		require.Equal(t, g.TEnd, w.TEnd, "edge %d", i)
		require.Equal(t, g.TExact, w.TExact, "edge %d", i)
		require.Equal(t, g.Polyline, w.Polyline, "edge %d", i)
		requireSameLineCross(t, g.crossStart, w.crossStart, i)
		requireSameLineCross(t, g.crossEnd, w.crossEnd, i)
		requireSameCircleCross(t, g.circleStart, w.circleStart, i)
		requireSameCircleCross(t, g.circleEnd, w.circleEnd, i)
	}
}

func requireSameLineCross(t *testing.T, got, want *LineLineCrossing, edge int) {
	t.Helper()
	require.Equal(t, got == nil, want == nil, "edge %d line crossing presence", edge)
	if got != nil && want != nil {
		require.Equal(t, got.sources, want.sources, "edge %d line crossing sources", edge)
		require.Equal(t, got.point, want.point, "edge %d line crossing point", edge)
	}
}

func requireSameCircleCross(t *testing.T, got, want *LineCircleCrossing, edge int) {
	t.Helper()
	require.Equal(t, got == nil, want == nil, "edge %d circle crossing presence", edge)
	if got == nil || want == nil {
		return
	}
	require.Equal(t, got.sources, want.sources, "edge %d circle crossing sources", edge)
	require.Equal(t, got.point, want.point, "edge %d circle crossing point", edge)
	require.Equal(t, got.branch, want.branch, "edge %d circle crossing branch", edge)
	require.Equal(t, got.allBounds, want.allBounds, "edge %d circle crossing bounds", edge)
}

func regularPolygonFixture(n int, radius float64) ([]Curve, []ClosedCurve) {
	points := make([]*Point, n)
	for i := range points {
		angle := 2 * math.Pi * float64(i) / float64(n)
		points[i] = NewPoint(radius*math.Cos(angle), radius*math.Sin(angle))
	}
	curves := make([]Curve, n)
	for i := range points {
		curves[i] = NewLine(points[i], points[(i+1)%n])
	}
	return curves, nil
}

func reverseCurves(curves []Curve) []Curve {
	out := slices.Clone(curves)
	slices.Reverse(out)
	return out
}

func TestAnalyticLineBoxRejectMatchesKernel(t *testing.T) {
	rng := rand.New(rand.NewSource(0x51a7))
	for i := 0; i < 500; i++ {
		scale := math.Pow(10, float64(rng.Intn(19)-9))
		tx := (rng.Float64()*2 - 1) * scale * 1e6
		ty := (rng.Float64()*2 - 1) * scale * 1e6
		a := lineOperand(tx, ty, tx+scale, ty+scale*(rng.Float64()*2-1))
		b := lineOperand(tx+20*scale, ty+5*scale, tx+21*scale, ty+5*scale+scale*(rng.Float64()*2-1))
		merge := scale * math.Pow(10, float64(rng.Intn(5)-8))
		if i%2 == 0 {
			a, b = b, a
		}
		if !filterLinePair(a, b, 100*scale, merge) {
			continue
		}
		si, sj := lineSource(a), lineSource(b)
		events, ambiguous, ok := analyticEventsOf(si, sj, a, b, 100*scale)
		require.True(t, ok, "case %d", i)
		require.False(t, ambiguous, "case %d", i)
		require.Empty(t, events, "case %d", i)
	}
}

func TestAnalyticLineBoxFilterKeepsBoundaryCases(t *testing.T) {
	cases := []struct {
		name  string
		a, b  operand
		scale float64
		merge float64
	}{
		{name: "crossing", a: lineOperand(-2, -1, 2, 1), b: lineOperand(-2, 1, 2, -1), scale: 4},
		{name: "endpoint reach", a: lineOperand(0, 0, 1, 0), b: lineOperand(1+0.5*arcParamEps, -1, 1+0.5*arcParamEps, 1), scale: 2},
		{name: "scene collinearity band", a: lineOperand(0, 0, 1, 0), b: lineOperand(0, 0.5*mergeEps*10, 1, 0.5*mergeEps*10), scale: 10},
		{name: "custom merge", a: lineOperand(0, 0, 1, 0), b: lineOperand(2, 0, 2, 1), scale: 2, merge: 1.01},
		{name: "near parallel", a: lineOperand(0, 0, 1, 0), b: lineOperand(0, 1, 1, 1+1e-8), scale: 2},
		{name: "zero length", a: lineOperand(0, 0, 0, 0), b: lineOperand(2, 0, 3, 1), scale: 3},
		{name: "large coordinates", a: lineOperand(1e16, 1e16, 1e16+8, 1e16+8), b: lineOperand(1e16+32, 1e16, 1e16+32, 1e16+8), scale: 64},
		{name: "nonfinite", a: lineOperand(0, 0, math.Inf(1), 1), b: lineOperand(2, 0, 3, 1), scale: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, filterLinePair(tc.a, tc.b, tc.scale, tc.merge))
		})
	}
}

func lineOperand(ax, ay, bx, by float64) operand {
	return operand{isLine: true, ax: ax, ay: ay, bx: bx, by: by}
}

func lineSource(o operand) *source {
	return &source{kind: srcLine, ax: o.ax, ay: o.ay, bx: o.bx, by: o.by}
}

func filterLinePair(a, b operand, scale, merge float64) bool {
	margin, ok := analyticLineBoxMargin([]source{*lineSource(a), *lineSource(b)}, scale, merge, nil)
	if !ok {
		return false
	}
	return analyticLineBoxesDisjoint(analyticLineBoxExpand(analyticLineBoxOf(a), margin),
		analyticLineBoxExpand(analyticLineBoxOf(b), margin))
}
