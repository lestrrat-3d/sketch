package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Configuration.PointXY indexes a Configuration's captured variable vector by
// the passed point's own xi/yi. Those indices only mean anything in the
// sketch the configuration came from, so a handle that sketch does not own
// must be refused before it is read: a foreign point can alias one of this
// sketch's variables at a small index (silently reading the wrong
// coordinates), run off the captured vector at a large index (a panic in a
// documented non-panicking call), or be a point this sketch has since
// removed (answering from a retired slot, or from whatever point now
// occupies its old id). The refusal is (NaN, NaN): every comparison against
// NaN is false, so a consumer diffing two configurations to test ambiguity
// reads "different" rather than a false "identical".

// mirrorTriangleProbe returns a DOF-0 sketch (the classic unsigned-distance
// triangle apex) together with its two-configuration probe result: a small,
// solved fixture with an owned free point (p) and two owned fixed points (a,
// b) to exercise the refusal and the owned-handle cases against.
func mirrorTriangleProbe(t *testing.T) (*sketch.Sketch, *sketch.Point, *sketch.Point, *sketch.ProbeResult) {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	p := s.CreatePoint(5, 3)
	s.AddConstraint(sketch.NewDistance(a, p, 8))
	s.AddConstraint(sketch.NewDistance(b, p, 8))

	_, err := s.Solve(t.Context())
	require.NoError(t, err, "triangle apex must solve")

	pr, err := s.ProbeConfigurations(t.Context())
	require.NoError(t, err, "probe must succeed on a solved DOF-0 sketch")
	require.True(t, pr.Ambiguous(), "two distance constraints admit a mirror apex")
	require.Len(t, pr.Configurations, 2)
	return s, a, p, pr
}

// mirrorTriangleProbeResult returns only the probe result from the mirror
// triangle fixture, for callers that need just the probe without the sketch
// or its points.
func mirrorTriangleProbeResult(t *testing.T) *sketch.ProbeResult {
	t.Helper()
	s, a, p, pr := mirrorTriangleProbe(t)
	_ = s // used to ensure full fixture is valid
	_ = a
	_ = p
	return pr
}

// A foreign point at a SMALL index sits at the same variable slot as this
// sketch's own first-created point (a, grounded at the origin's (0,0) by
// Fix), so an unscreened read would silently return that local point's
// coordinates instead of refusing.
func TestProbeConfigurationPointXYRefusesForeignPointSmallIndex(t *testing.T) {
	_, a, _, pr := mirrorTriangleProbe(t)

	fp := foreignPoint(t, 0, 99, 99) // pad=0: same var slot as a in a fresh sketch

	var x, y float64
	require.NotPanics(t, func() { x, y = pr.Configurations[0].PointXY(fp) })

	// It specifically must not have returned the aliased local point's
	// coordinates (what the unguarded index read would have produced: a's
	// own (0, 0)).
	require.NotEqual(t, a.X(), x)
	require.NotEqual(t, a.Y(), y)
	require.True(t, math.IsNaN(x), "a foreign handle must never be read")
	require.True(t, math.IsNaN(y))
}

// A foreign point at a LARGE index runs off the configuration's captured
// variable vector; the documented-non-panicking call must not panic.
func TestProbeConfigurationPointXYRefusesForeignPointLargeIndex(t *testing.T) {
	pr := mirrorTriangleProbeResult(t)

	fp := foreignPoint(t, 64, 99, 99) // pad=64: far past this sketch's vector

	var x, y float64
	require.NotPanics(t, func() { x, y = pr.Configurations[0].PointXY(fp) })
	require.True(t, math.IsNaN(x))
	require.True(t, math.IsNaN(y))
}

// A point this sketch has since removed is dead: RemovePoint splices it out
// and renumbers the later ids, so its old id slot now names a different
// point while its variable slots are merely retired (not reclaimed) — the
// case owns() catches by identity, not by index range.
func TestProbeConfigurationPointXYRefusesRemovedPoint(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	dead := s.CreatePoint(20, 20) // unused; removed below, its id inherited by b
	b := s.CreatePoint(10, 0)
	s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
	s.AddConstraint(sketch.NewHorizontalDistance(a, b, 10))
	s.AddConstraint(sketch.NewVerticalDistance(a, b, 0))
	require.True(t, s.RemovePoint(dead))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	pr, err := s.ProbeConfigurations(t.Context())
	require.NoError(t, err)

	var x, y float64
	require.NotPanics(t, func() { x, y = pr.Configurations[0].PointXY(dead) })
	require.True(t, math.IsNaN(x), "a removed point must never be read")
	require.True(t, math.IsNaN(y))
}

// The load-bearing half: an owned point, the origin, and every point of the
// sketch must read exactly as before — unchanged by the added screen — across
// every configuration the probe returns.
func TestProbeConfigurationPointXYOwnedHandlesUnchanged(t *testing.T) {
	s, _, p, pr := mirrorTriangleProbe(t)

	for i, cfg := range pr.Configurations {
		ox, oy := cfg.PointXY(s.Origin())
		require.False(t, math.IsNaN(ox), "configuration %d: origin must read normally", i)
		require.False(t, math.IsNaN(oy))
		require.Equal(t, 0.0, ox, "origin is always (0,0)")
		require.Equal(t, 0.0, oy)

		for _, pt := range s.Points() {
			x, y := cfg.PointXY(pt)
			require.Falsef(t, math.IsNaN(x), "configuration %d: owned point must never refuse", i)
			require.Falsef(t, math.IsNaN(y), "configuration %d: owned point must never refuse", i)
		}
	}

	// Cross-check the apex's own free point against Apply: what PointXY
	// reported for the alternative (mirrored) configuration is exactly what
	// lands on the point once that configuration is adopted.
	altX, altY := pr.Configurations[1].PointXY(p)
	pr.Configurations[1].Apply()
	require.Equal(t, altX, p.X())
	require.Equal(t, altY, p.Y())
}
