package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// profileAreaScaleSides spans the old, now-removed absolute area floor
// (areaEps = 1e-9 mm²) that profiles.go layered on top of geom's own
// scale-relative one: an equilateral triangle of the two smaller side lengths
// used to read Valid=false/Trustworthy=false, and the two larger ones true —
// the same shape flipping verdict on physical size alone.
var profileAreaScaleSides = []float64{3.2e-6, 3.2e-5, 3.2e-4, 3.2e-3}

// TestProfileAreaFloorIsScaleInvariant pins that a fully constrained
// equilateral triangle reports the same Profile.Valid, ProfilesValid and
// Trustworthy() verdict at every physical size: the trust verdict must depend
// on the triangle's shape, never on how large it is drawn.
func TestProfileAreaFloorIsScaleInvariant(t *testing.T) {
	build := func(side float64) *sketch.VerificationReport {
		s := newSketch(t)
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(side, 0)
		c := s.CreatePoint(side/2, side*math.Sqrt(3)/2)
		ab := s.CreateLine(a, b)
		bc := s.CreateLine(b, c)
		ca := s.CreateLine(c, a)
		s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
		s.AddConstraint(sketch.NewHorizontal(ab))
		s.AddConstraint(sketch.NewDistance(a, b, side))
		s.AddConstraint(sketch.NewEqual(ab, bc))
		s.AddConstraint(sketch.NewEqual(ab, ca))
		_, err := s.Solve(t.Context())
		require.NoErrorf(t, err, "side=%v", side)
		return s.Verify(t.Context())
	}
	for _, side := range profileAreaScaleSides {
		rep := build(side)
		require.Lenf(t, rep.Profiles, 1, "side=%v", side)
		require.Truef(t, rep.Profiles[0].Valid, "side=%v: triangle profile must be valid regardless of scale", side)
		require.Truef(t, rep.ProfilesValid, "side=%v", side)
		require.Truef(t, rep.Trustworthy(), "side=%v", side)
	}
}
