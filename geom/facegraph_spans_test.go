package geom_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// squareGraph is a hand-built face graph: one inside face, bounded by four edges of
// source 0 whose parameters run 0, 1/4, 1/2, 3/4, 1 around the vertices 0 to 3.
func squareGraph() *geom.FaceGraph {
	g := &geom.FaceGraph{Faces: []geom.FaceNode{{Faithful: true}}}
	for i := range 4 {
		g.Edges = append(g.Edges, geom.FaceEdge{
			A: 0, B: geom.FaceOuter, U: i, V: (i + 1) % 4, Covers: []int{0},
			Src: 0, PU: float64(i) / 4, PV: float64(i+1) / 4, ExactU: true, ExactV: true,
		})
	}
	return g
}

func span(src int, lo, hi float64) geom.LoopElement {
	return geom.LoopElement{Source: src, Span: true, Lo: lo, Hi: hi}
}

func walkOne(g *geom.FaceGraph, elems ...geom.LoopElement) geom.LoopWalk {
	return g.WalkLoopElements([][]geom.LoopElement{elems})[0]
}

func TestWalkLoopElementsSpans(t *testing.T) {
	t.Run("one span around the cycle", func(t *testing.T) {
		w := walkOne(squareGraph(), span(0, 0, 1))
		require.Equal(t, geom.LoopDefectNone, w.Defect)
		require.Equal(t, []int8{1}, w.Side)
	})
	t.Run("two spans that meet at a vertex", func(t *testing.T) {
		w := walkOne(squareGraph(), span(0, 0.5, 1), span(0, 0, 0.5))
		require.Equal(t, geom.LoopDefectNone, w.Defect)
		require.Equal(t, []int8{1}, w.Side)
	})
	t.Run("an edge walked against its parameter", func(t *testing.T) {
		g := squareGraph()
		g.Edges[2].U, g.Edges[2].V = g.Edges[2].V, g.Edges[2].U
		g.Edges[2].PU, g.Edges[2].PV = g.Edges[2].PV, g.Edges[2].PU
		g.Edges[2].ExactU, g.Edges[2].ExactV = true, true
		w := walkOne(g, span(0, 0, 1))
		require.Equal(t, geom.LoopDefectNone, w.Defect)
	})
	t.Run("half the cycle leaves open ends", func(t *testing.T) {
		w := walkOne(squareGraph(), span(0, 0, 0.5))
		require.Equal(t, geom.LoopDefectSelfCrossing, w.Defect)
	})
	t.Run("an end inside an edge", func(t *testing.T) {
		w := walkOne(squareGraph(), span(0, 0, 0.4), span(0, 0.4, 1))
		require.Equal(t, geom.LoopDefectSpanBound, w.Defect)
	})
	t.Run("an end that is not an exact bound", func(t *testing.T) {
		g := squareGraph()
		g.Edges[1].ExactU, g.Edges[0].ExactV = false, false
		w := walkOne(g, span(0, 0.25, 0.75), span(0, 0.75, 1), span(0, 0, 0.25))
		require.Equal(t, geom.LoopDefectSpanBound, w.Defect)
	})
	t.Run("a bound inside the span does not need to be exact", func(t *testing.T) {
		g := squareGraph()
		g.Edges[1].ExactU, g.Edges[0].ExactV = false, false
		w := walkOne(g, span(0, 0, 1))
		require.Equal(t, geom.LoopDefectNone, w.Defect)
	})
	t.Run("a span of a source that draws nothing", func(t *testing.T) {
		w := walkOne(squareGraph(), span(7, 0, 1))
		require.Equal(t, geom.LoopDefectSpanBound, w.Defect)
	})
	t.Run("an edge missing between the ends", func(t *testing.T) {
		g := squareGraph()
		g.Edges = append(g.Edges[:1], g.Edges[2:]...)
		w := walkOne(g, span(0, 0, 1))
		require.Equal(t, geom.LoopDefectGap, w.Defect)
	})
	t.Run("the same stretch twice", func(t *testing.T) {
		w := walkOne(squareGraph(), span(0, 0, 1), span(0, 0, 0.5))
		require.Equal(t, geom.LoopDefectSelfCrossing, w.Defect)
	})
	t.Run("a span and the whole source", func(t *testing.T) {
		w := walkOne(squareGraph(), span(0, 0, 1), geom.LoopElement{Source: 0})
		require.Equal(t, geom.LoopDefectSelfCrossing, w.Defect)
	})
	t.Run("two cycles", func(t *testing.T) {
		g := squareGraph()
		g.Faces = append(g.Faces, geom.FaceNode{Faithful: true})
		for i := range 4 {
			g.Edges = append(g.Edges, geom.FaceEdge{
				A: 1, B: geom.FaceOuter, U: 4 + i, V: 4 + (i+1)%4, Covers: []int{1},
				Src: 1, PU: float64(i) / 4, PV: float64(i+1) / 4, ExactU: true, ExactV: true,
			})
		}
		w := walkOne(g, span(0, 0, 1), span(1, 0, 1))
		require.Equal(t, geom.LoopDefectGap, w.Defect)
	})
	t.Run("whole elements keep their old verdict", func(t *testing.T) {
		g := squareGraph()
		whole := g.WalkLoops([][]int{{0}})[0]
		require.Equal(t, geom.LoopDefectNone, whole.Defect)
		require.Equal(t, []int8{1}, whole.Side)
	})
}
