package geom_test

import (
	"sort"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func a7Loop(points [][2]float64) []geom.Curve {
	vertices := make([]*geom.Point, len(points))
	for i, p := range points {
		vertices[i] = geom.NewPoint(p[0], p[1])
	}
	lines := make([]geom.Curve, len(vertices))
	for i := range vertices {
		lines[i] = geom.NewLine(vertices[i], vertices[(i+1)%len(vertices)])
	}
	return lines
}

func a7LineAt(line *geom.Line, t float64) [2]float64 {
	return [2]float64{
		line.Start.X + t*(line.End.X-line.Start.X),
		line.Start.Y + t*(line.End.Y-line.Start.Y),
	}
}

func a7FaceEdge(t *testing.T, arr *geom.Arrangement, source int, lo, hi float64) geom.FaceEdge {
	t.Helper()
	for _, edge := range arr.FaceGraph.Edges {
		if edge.Src != source || !edge.ExactU || !edge.ExactV {
			continue
		}
		start, end := edge.PU, edge.PV
		if end < start {
			start, end = end, start
		}
		if start == lo && end == hi {
			return edge
		}
	}
	t.Fatalf("no exact graph edge for source %d on [%g,%g]", source, lo, hi)
	return geom.FaceEdge{}
}

func a7VertexAt(t *testing.T, edge geom.FaceEdge, param float64) int {
	t.Helper()
	if edge.PU == param {
		return edge.U
	}
	if edge.PV == param {
		return edge.V
	}
	t.Fatalf("graph edge source %d has no endpoint at t=%g", edge.Src, param)
	return -1
}

func a7LoopArea(edges []geom.BoundaryEdge) float64 {
	area := 0.0
	for _, edge := range edges {
		for i := 0; i+1 < len(edge.Polyline); i++ {
			a, b := edge.Polyline[i], edge.Polyline[i+1]
			area += a[0]*b[1] - b[0]*a[1]
		}
	}
	return area / 2
}

func TestA7SharedLineHoleHasFaithfulSelectedSide(t *testing.T) {
	curves := a7Loop([][2]float64{{-10, -10}, {10, -10}, {10, 10}, {-10, 10}})
	curves = append(curves, a7Loop([][2]float64{
		{0, -2}, {-4, -2}, {-4, 2}, {0, 2},
	})...)
	curves = append(curves, a7Loop([][2]float64{
		{-1, -2}, {3, -2}, {3, 2}, {-1, 2},
	})...)

	arr := geom.Regions(curves, nil, geom.WithFaceGraph())
	require.False(t, arr.Degenerate, "degeneracies: %v", arr.Degeneracies)
	require.Len(t, arr.Regions, 4)
	for i, face := range arr.FaceGraph.Faces {
		require.Truef(t, face.Faithful, "face %d must be sound", i)
		require.Falsef(t, arr.Regions[i].Degenerate, "region %d", i)
	}

	selected := arr.Regions[0]
	require.InDelta(t, 372, selected.Area, 1e-9)
	require.Len(t, selected.Holes, 1)
	hole := selected.Holes[0]
	require.Equal(t, []int{4, 5, 6, 10, 9, 8}, func() []int {
		sources := make([]int, len(hole))
		for i, edge := range hole {
			sources[i] = edge.SourceIndex
		}
		return sources
	}())
	require.Less(t, a7LoopArea(hole), 0.0, "the selected hole walk is clockwise")

	want := []struct {
		source   int
		lo, hi   float64
		reversed bool
	}{
		{4, 0, 1, false}, {5, 0, 1, false}, {6, 0, 1, false},
		{10, 0, 0.75, true}, {9, 0, 1, true}, {8, 0.25, 1, true},
	}
	for i, expected := range want {
		edge := hole[i]
		require.Equal(t, expected.source, edge.SourceIndex)
		require.Equal(t, expected.lo, edge.TStart)
		require.Equal(t, expected.hi, edge.TEnd)
		require.Equal(t, expected.reversed, edge.Reversed)
		require.Truef(t, edge.TExact, "source %d", edge.SourceIndex)
		var covered [][2]float64
		for _, graphEdge := range arr.FaceGraph.Edges {
			if graphEdge.Src != edge.SourceIndex || !graphEdge.ExactU || !graphEdge.ExactV {
				continue
			}
			lo, hi := graphEdge.PU, graphEdge.PV
			if hi < lo {
				lo, hi = hi, lo
			}
			if lo >= edge.TStart && hi <= edge.TEnd &&
				(graphEdge.A == 0 || graphEdge.B == 0) {
				covered = append(covered, [2]float64{lo, hi})
			}
		}
		sort.Slice(covered, func(i, j int) bool { return covered[i][0] < covered[j][0] })
		cursor := edge.TStart
		for _, interval := range covered {
			require.Equalf(t, cursor, interval[0], "source %d graph edge starts without a gap", edge.SourceIndex)
			cursor = interval[1]
		}
		require.Equalf(t, edge.TEnd, cursor, "source %d graph edges cover the selected interval", edge.SourceIndex)
	}

	// At [0,2], target source 6 ends where tool source 10 has its exact t=0.75
	// cut. Both emitted graph edges name the same vertex.
	targetTopEnd := a7FaceEdge(t, arr, 6, 0.75, 1)
	toolTopCut := a7FaceEdge(t, arr, 10, 0, 0.75)
	require.Equal(t, a7VertexAt(t, targetTopEnd, 1), a7VertexAt(t, toolTopCut, 0.75))

	// At [-1,2], target source 6 splits at t=0.75, and the same graph edge is
	// covered by tool source 10, whose authored endpoint is t=1.
	targetTopBefore := a7FaceEdge(t, arr, 6, 0, 0.75)
	targetTopAfter := a7FaceEdge(t, arr, 6, 0.75, 1)
	topVertex := a7VertexAt(t, targetTopBefore, 0.75)
	require.Equal(t, topVertex, a7VertexAt(t, targetTopAfter, 0.75))
	require.Contains(t, targetTopAfter.Covers, 10)
	targetTop, ok := curves[6].(*geom.Line)
	require.True(t, ok)
	toolTop, ok := curves[10].(*geom.Line)
	require.True(t, ok)
	require.Equal(t, a7LineAt(targetTop, 0.75), [2]float64{toolTop.End.X, toolTop.End.Y})

	// At [-1,-2], target source 4 splits at t=0.25, and the shared graph edge
	// covers tool source 8, whose authored endpoint is t=0.
	targetBottomSplit := a7FaceEdge(t, arr, 4, 0, 0.25)
	targetBottomAfter := a7FaceEdge(t, arr, 4, 0.25, 1)
	bottomVertex := a7VertexAt(t, targetBottomSplit, 0.25)
	require.Equal(t, bottomVertex, a7VertexAt(t, targetBottomAfter, 0.25))
	require.Contains(t, targetBottomSplit.Covers, 8)
	targetBottom, ok := curves[4].(*geom.Line)
	require.True(t, ok)
	toolBottom, ok := curves[8].(*geom.Line)
	require.True(t, ok)
	require.Equal(t, a7LineAt(targetBottom, 0.25), [2]float64{toolBottom.Start.X, toolBottom.Start.Y})
}
