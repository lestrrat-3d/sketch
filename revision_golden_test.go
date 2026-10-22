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
