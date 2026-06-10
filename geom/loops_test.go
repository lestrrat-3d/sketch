package geom_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestLoopsSquare(t *testing.T) {
	a := geom.NewPoint(0, 0)
	b := geom.NewPoint(10, 0)
	c := geom.NewPoint(10, 10)
	d := geom.NewPoint(0, 10)
	curves := []geom.Curve{
		geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a),
	}
	loops := geom.Loops(curves)
	require.Len(t, loops, 1, "one loop")
	require.Len(t, loops[0].Curves, 4, "four sides")
}

func TestLoopsOpenChain(t *testing.T) {
	a := geom.NewPoint(0, 0)
	b := geom.NewPoint(10, 0)
	c := geom.NewPoint(10, 10)
	curves := []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c)}
	require.Empty(t, geom.Loops(curves), "open chain has no loop")
}

func TestLoopsCoincidentButUnshared(t *testing.T) {
	// Same coordinates, distinct points: not connected.
	curves := []geom.Curve{
		geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(10, 0)),
		geom.NewLine(geom.NewPoint(10, 0), geom.NewPoint(0, 0)),
	}
	require.Empty(t, geom.Loops(curves), "identity, not coordinates, connects")
}

func TestLoopsTwoTrianglesSharedVertex(t *testing.T) {
	a := geom.NewPoint(0, 0)
	b := geom.NewPoint(10, 0)
	c := geom.NewPoint(5, 8)
	d := geom.NewPoint(-10, 0)
	e := geom.NewPoint(-5, 8)
	curves := []geom.Curve{
		geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, a),
		geom.NewLine(a, d), geom.NewLine(d, e), geom.NewLine(e, a),
	}
	loops := geom.Loops(curves)
	require.Len(t, loops, 2, "two loops through the shared vertex")
	require.Len(t, loops[0].Curves, 3, "first triangle")
	require.Len(t, loops[1].Curves, 3, "second triangle")
}

func TestLoopsLineArcLens(t *testing.T) {
	a := geom.NewPoint(-5, 0)
	b := geom.NewPoint(5, 0)
	center := geom.NewPoint(0, 0)
	curves := []geom.Curve{
		geom.NewLine(a, b),
		geom.NewArc(center, b, a), // upper semicircle closing the lens
	}
	loops := geom.Loops(curves)
	require.Len(t, loops, 1, "line+arc lens closes")
	require.Len(t, loops[0].Curves, 2, "two curves")
}
