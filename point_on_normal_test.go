package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// The point is pulled onto the normal of a line at a point: the perpendicular
// to the line through that point.
func TestPointOnNormalLine(t *testing.T) {
	s := newSketch(t)
	a, b := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	l := s.CreateLine(a, b)
	p := s.CreatePoint(7, 3)
	s.AddConstraint(sketch.NewPointOnNormal(p, l, b))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 10, p.X(), 1e-6, "on the perpendicular through (10, 0)")
}

// For an arc the normal is the radial line through the center and the point.
func TestPointOnNormalArc(t *testing.T) {
	s := newSketch(t)
	c, st, en := s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5)
	s.Fix(c)
	s.Fix(st)
	s.Fix(en)
	arc := s.CreateArc(c, st, en)
	p := s.CreatePoint(7, 2)
	s.AddConstraint(sketch.NewPointOnNormal(p, arc, st))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 0, p.Y(), 1e-6, "on the radial line through (5, 0)")
}

// The constraint survives a JSON round trip for both operand kinds, and its row
// is classified for the conditioning gate (an unclassified row reads as NaN).
func TestPointOnNormalRoundTripAndVerify(t *testing.T) {
	s := newSketch(t)
	l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
	arc := s.CreateArc(s.CreatePoint(20, 0), s.CreatePoint(25, 0), s.CreatePoint(20, 5))
	s.AddConstraint(
		sketch.NewPointOnNormal(s.CreatePoint(10, 3), l, l.End),
		sketch.NewPointOnNormal(s.CreatePoint(27, 0), arc, arc.Start),
	)
	require.Equal(t, 2, countKind(s, "point_on_normal"))

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Equal(t, 2, countKind(&s2, "point_on_normal"))

	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	rep := s.Verify(t.Context())
	require.True(t, rep.Analysed())
	require.False(t, math.IsNaN(rep.Conditioning))
}
