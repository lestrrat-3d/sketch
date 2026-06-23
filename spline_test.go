package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestSplineSolveReshapesCurve(t *testing.T) {
	s := newSketch(t)
	sp, err := s.CreateSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 4), s.CreatePoint(8, 4), s.CreatePoint(9, 1))
	require.NoError(t, err)
	s.Fix(sp.Control[0])

	// Dimension the last control point to (10, 0): the curve's clamped end
	// must follow it exactly.
	s.AddConstraint(
		sketch.NewHorizontalDistance(sp.Control[0], sp.Control[3], 10),
		sketch.NewVerticalDistance(sp.Control[0], sp.Control[3], 0),
	)
	_, err = s.Solve()
	require.NoError(t, err)

	x, y := sp.Eval(1)
	require.InDelta(t, 10, x, 1e-6, "clamped end follows the solved control point")
	require.InDelta(t, 0, y, 1e-6, "end y")
	x, y = sp.Eval(0)
	require.InDelta(t, 0, x, 1e-12, "fixed start")
	require.InDelta(t, 0, y, 1e-12, "fixed start")
}

func TestSplineControlPointGoal(t *testing.T) {
	s := newSketch(t)
	sp, err := s.CreateSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 4), s.CreatePoint(8, 4), s.CreatePoint(10, 0))
	require.NoError(t, err)

	// Drag an interior control point; the curve follows.
	res, err := s.Solve(sketch.WithGoal(sp.Control[1], 2, 8))
	require.NoError(t, err, "goal solve")
	require.True(t, res.Converged, "no constraints to violate")
	require.InDelta(t, 8, sp.Control[1].Y(), 1e-5, "control point tracked the goal")
}

func TestSplineJSONRoundTrip(t *testing.T) {
	s := newSketch(t)
	sp, err := s.CreateSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 4), s.CreatePoint(8, 4), s.CreatePoint(10, 0), s.CreatePoint(12, -2))
	require.NoError(t, err)
	s.Fix(sp.Control[0])

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	require.Contains(t, string(data), `"degree":3`, "degree written")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	sp2, ok := s2.Entities()[0].(*sketch.Spline)
	require.True(t, ok, "spline reloaded")
	require.Len(t, sp2.Control, 5, "control points reattached")
	for _, tt := range []float64{0, 0.3, 0.7, 1} {
		x1, y1 := sp.Eval(tt)
		x2, y2 := sp2.Eval(tt)
		require.InDeltaf(t, x1, x2, 1e-9, "x at t=%v", tt)
		require.InDeltaf(t, y1, y2, 1e-9, "y at t=%v", tt)
	}
}

func TestSplineExports(t *testing.T) {
	s := newSketch(t)
	_, err := s.CreateSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 4), s.CreatePoint(8, 4), s.CreatePoint(10, 0))
	require.NoError(t, err)

	svg, err := s.SVG()
	require.NoError(t, err, "svg")
	require.Contains(t, svg, "<path", "SVG path for the spline")

	dxf, err := s.DXF()
	require.NoError(t, err, "dxf")
	require.Contains(t, dxf, "SPLINE", "DXF spline entity")
}
