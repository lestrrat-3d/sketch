package sketch

// This is the one IN-PACKAGE test file (the repo's tests otherwise live in
// external xxx_test packages and exercise only the exported API). It is here on
// purpose: the property under test is that [Sketch.Revision] hashes the shape
// value an entity RESOLVES — not the var vector it happens to resolve out of —
// and the only way to break the two apart is to rebind an entity's var selector
// (Circle.ri, Ellipse.rxi, Conic.rhoi, …), which is unexported state. An
// external test cannot reach it, and the right answer is NOT to export a
// selector-rebinding accessor just to test it: that would add a footgun to the
// public API to prove the fingerprint is robust against a footgun.
//
// The consumer-visible half of the contract (a shape-value change makes a
// profile stale) is covered externally in profile_revision_test.go.

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRevisionResolvesShapeValues rebinds each shape-carrying entity's var
// selector without touching the var vector, the points, or the topology. Every
// proxy for the shape — the var vector's contents, the multiset of selector
// indices, the entity set, the point coordinates — is invariant under the
// rebinding, while Profiles() changes shape, so a fingerprint built on any of
// those proxies would report a mutated sketch as unchanged and a stale Profile
// would read fresh. Only hashing the value the entity actually resolves
// (Circle.r(), Ellipse.rx()/ry()/rot(), Conic.rho() — the very accessors
// buildProfiles reads through) survives this.
func TestRevisionResolvesShapeValues(t *testing.T) {
	t.Run("circle radius selectors swapped", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		c1 := s.CreateCircle(s.CreatePoint(0, 0), 2)  // area 4pi
		c2 := s.CreateCircle(s.CreatePoint(20, 0), 5) // area 25pi

		before := s.Revision()
		p1, p2 := s.Profiles()[0], s.Profiles()[1]
		require.False(t, p1.IsStale())

		c1.ri, c2.ri = c2.ri, c1.ri // the two disks trade radii; s.vars is untouched

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the radius each circle RESOLVES, not the var vector")
		require.True(t, p1.IsStale())
		require.True(t, p2.IsStale())

		after := s.Profiles()
		require.Len(t, after, 2)
		require.Greater(t, math.Abs(after[0].Area-p1.Area), 1e-9, "the profiles really did change")
	})

	t.Run("ellipse semi-axis selectors swapped", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		e := s.CreateEllipse(s.CreatePoint(0, 0), 8, 3, 0)

		before := s.Revision()
		p := s.Profiles()[0]

		e.rxi, e.ryi = e.ryi, e.rxi // an 8x3 ellipse becomes a 3x8 one

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the semi-axes the ellipse RESOLVES")
		require.True(t, p.IsStale())
	})

	t.Run("ellipse rotation selector rebound", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		e1 := s.CreateEllipse(s.CreatePoint(0, 0), 8, 3, 0)
		e2 := s.CreateEllipse(s.CreatePoint(30, 0), 8, 3, 1)

		before := s.Revision()
		p := s.Profiles()[0]

		e1.roti = e2.roti // the first ellipse is now rotated too

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the rotation the ellipse RESOLVES")
		require.True(t, p.IsStale())
	})

	t.Run("conic rho selector rebound", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		a, apex, b := s.CreatePoint(0, 0), s.CreatePoint(10, 10), s.CreatePoint(20, 0)
		c1, err := s.CreateConic(a, apex, b, 0.3)
		require.NoError(t, err)
		s.CreateLine(b, a) // close the region under the conic

		// A second conic elsewhere, purely to own a different rho var.
		c2, err := s.CreateConic(s.CreatePoint(40, 0), s.CreatePoint(50, 10), s.CreatePoint(60, 0), 0.8)
		require.NoError(t, err)
		require.NotNil(t, c2)

		before := s.Revision()
		p := s.Profiles()[0]
		areaBefore := p.Area

		c1.rhoi = c2.rhoi // the conic bulges out to rho 0.8; no point moves

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the rho the conic RESOLVES")
		require.True(t, p.IsStale())
		require.Greater(t, math.Abs(s.Profiles()[0].Area-areaBefore), 1e-9,
			"the region under the conic really did change area")
	})

	t.Run("elliptical arc semi-axis selectors swapped", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		ea := s.CreateEllipticalArc(s.CreatePoint(0, 0), s.CreatePoint(8, 0), s.CreatePoint(0, 3), 8, 3, 0)
		require.NotNil(t, ea)

		before := s.Revision()

		ea.rxi, ea.ryi = ea.ryi, ea.rxi

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the semi-axes the elliptical arc RESOLVES")
	})
}
