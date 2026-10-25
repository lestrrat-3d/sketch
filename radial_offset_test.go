package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Two concentric circles sharing a center: the radial offset drives the copy's
// radius to the source's radius plus d.
func TestRadialOffsetCircles(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	src := s.CreateCircle(center, 10)
	dst := s.CreateCircle(center, 11)
	off := sketch.NewRadialOffset(src, dst, 2)
	s.AddConstraint(sketch.NewRadius(src, 10), off)

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 12, dst.R(), 1e-6)

	off.Set(-3)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, dst.R(), 1e-6)
}

// An arc operand reads its radius from its Start.
func TestRadialOffsetArcs(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	s.Fix(center)
	srcStart, srcEnd := s.CreatePoint(10, 0), s.CreatePoint(0, 10)
	s.Fix(srcStart)
	s.Fix(srcEnd)
	src := s.CreateArc(center, srcStart, srcEnd)
	dst := s.CreateArc(center, s.CreatePoint(11, 0), s.CreatePoint(0, 11))
	s.AddConstraint(sketch.NewRadialOffset(src, dst, 2))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 12, dst.R(), 1e-6)
}

// The dimension survives a JSON round trip with its kind and signed target, and
// its row is classified for the conditioning gate.
func TestRadialOffsetRoundTrip(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	src := s.CreateCircle(center, 10)
	dst := s.CreateCircle(center, 7)
	s.AddConstraint(sketch.NewRadialOffset(src, dst, -3))

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Equal(t, 1, countKind(&s2, "radial_offset"))
	for _, c := range s2.Constraints() {
		if ro, ok := c.(*sketch.RadialOffset); ok {
			require.InDelta(t, -3, ro.Target().Base(), 1e-12)
		}
	}

	rep := s.Verify(t.Context())
	require.True(t, rep.Analysed())
	require.False(t, math.IsNaN(rep.Conditioning))
}
