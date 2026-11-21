package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestRevisionFingerprintValue pins the fingerprint of a small scene of integer
// coordinates. The value is the FNV-1a hash of the byte stream Revision documents, so a
// change to it means the stream changed, and a consumer that stored a revision across
// versions would read every stored profile as stale.
func TestRevisionFingerprintValue(t *testing.T) {
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	a, b, c, d := s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(4, 3), s.CreatePoint(0, 3)
	s.CreateLine(a, b)
	s.CreateLine(b, c).SetConstruction(true)
	s.CreateArc(a, b, d)
	s.CreateCircle(c, 2)
	_, err = s.CreateFitSpline(a, c, d)
	require.NoError(t, err)
	require.Equal(t, uint64(0xb4e9018811dd69cd), s.Revision())
}

// TestRevisionFingerprintValueForeignPoints pins the fingerprint of a scene whose
// lines also reach a point owned by another sketch and the origin, so the
// first-seen point numbering is pinned across own and foreign points alike.
func TestRevisionFingerprintValueForeignPoints(t *testing.T) {
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	other, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	a, b, c := s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(4, 3)
	foreign := other.CreatePoint(7, 1)
	l1 := s.CreateLine(a, b)
	l2 := s.CreateLine(c, b)
	l3 := s.CreateLine(b, c)
	s.CreateLine(c, a)
	l1.End = foreign
	l2.Start = s.Origin()
	l3.Start = foreign
	require.Equal(t, uint64(0x415333fd8d3d1965), s.Revision())
}
