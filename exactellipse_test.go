package sketch_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileEllipseAreaExact(t *testing.T) {
	// A sketch ellipse's profile area is exact (pi*rx*ry), not a sampled approx —
	// so the oracle's reported area and validity rest on an exact number.
	s := newSketch(t)
	c := s.CreatePoint(1, 2)
	e := s.CreateEllipse(c, 7, 3, 0.5)
	profiles := s.Profiles()
	require.Len(t, profiles, 1)
	require.True(t, profiles[0].Valid)
	require.InDelta(t, math.Pi*7*3, profiles[0].Area, 1e-9, "exact ellipse profile area")
	_ = e
}
