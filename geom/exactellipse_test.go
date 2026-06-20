package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestExactFullEllipseArea(t *testing.T) {
	// A full ellipse's region area is EXACTLY pi*rx*ry, independent of centre and
	// rotation — not a sampled approximation.
	e := &geom.Ellipse{Center: geom.NewPoint(2, 1), Rx: 6, Ry: 3, Rotation: 0.7}
	arr := geom.Regions(nil, []geom.ClosedCurve{e})
	require.Len(t, arr.Regions, 1)
	require.InDelta(t, math.Pi*6*3, arr.Regions[0].Area, 1e-9, "exact ellipse area")
}

func TestExactHalfEllipseArea(t *testing.T) {
	// An elliptical arc (upper half) closed by its major-axis chord encloses
	// exactly half the ellipse.
	c := geom.NewPoint(0, 0)
	start := geom.NewPoint(4, 0) // eccentric angle 0
	end := geom.NewPoint(-4, 0)  // eccentric angle pi
	ea := geom.NewEllipticalArc(c, start, end, 4, 2, 0)
	chord := geom.NewLine(end, start)
	arr := geom.Regions([]geom.Curve{ea, chord}, nil)
	require.Len(t, arr.Regions, 1)
	require.InDelta(t, math.Pi*4*2/2, arr.Regions[0].Area, 1e-9, "exact half-ellipse area")
}

func TestExactRotatedEllipseArea(t *testing.T) {
	// Rotation does not change the area (the segment correction is rotation- and
	// translation-invariant by construction).
	for _, rot := range []float64{0, 0.3, 1.2, 2.5} {
		e := &geom.Ellipse{Center: geom.NewPoint(-3, 5), Rx: 5, Ry: 2, Rotation: rot}
		arr := geom.Regions(nil, []geom.ClosedCurve{e})
		require.Len(t, arr.Regions, 1)
		require.InDeltaf(t, math.Pi*5*2, arr.Regions[0].Area, 1e-9, "rotation %v", rot)
	}
}
