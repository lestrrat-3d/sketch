package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

// polylineArea is the shoelace area of the closed polygon obtained by
// concatenating each boundary edge's own published Polyline, dropping the
// duplicated joint at each edge's end (a closed loop shares each vertex with
// its neighbor). It measures the boundary a profile PUBLISHES, independent of
// Profile.Area itself, so a test can check the two agree.
func polylineArea(edges []sketch.BoundaryEdge) float64 {
	var pts [][2]float64
	for _, e := range edges {
		pts = append(pts, e.Polyline[:len(e.Polyline)-1]...)
	}
	var s float64
	for i := range pts {
		p, q := pts[i], pts[(i+1)%len(pts)]
		s += p[0]*q[1] - q[0]*p[1]
	}
	return s / 2
}

// TestProfilesWeldedSectorOuterCycleNotHole reproduces fu90 through the public
// Sketch.Profiles() API with default options. A radius-10, 15deg arc is closed
// by two chords that both land within 2e-7 of the arc's own start point — under
// the default vertex-merge tolerance — welding all three curves' meeting point
// onto the arc's start and splitting the sector into a 0.1deg sliver and the
// remaining wedge.
//
// Before the fix, extract's only guard against assigning a cycle its own
// unbounded/adjacent boundary as someone else's hole was an area-magnitude
// compare, and the weld shrank that cycle's computed magnitude below the
// bounded wedge it was tested against: Sketch.Profiles() published a profile
// with Area=2.1276907741230033e-07 while its own Outer polyline enclosed
// 0.147804554 — off by six orders of magnitude — carrying one bogus hole that
// was the region's own boundary walked backwards.
func TestProfilesWeldedSectorOuterCycleNotHole(t *testing.T) {
	const radius, gap, inner = 10.0, 2e-7, 0.1
	s := newSketch(t)
	polar := func(r, deg float64) *sketch.Point {
		rad := deg * math.Pi / 180
		return s.CreatePoint(r*math.Cos(rad), r*math.Sin(rad))
	}
	q0 := s.CreatePoint(radius-gap, 0)
	s.CreateArc(s.CreatePoint(0, 0), polar(radius, 0), polar(radius, 15))
	s.CreateLine(polar(radius, inner), q0)
	s.CreateLine(polar(radius, 15), q0)

	profiles := s.Profiles()
	require.Len(t, profiles, 2, "the 0.1deg sliver and the remaining wedge")
	for _, p := range profiles {
		sketchtest.IsValidProfile(t, p)
		require.Empty(t, p.Holes, "neither wedge has an interior void")
	}

	// The sliver's own dense chord polygon is itself near-degenerate (its
	// straight-chord shoelace rounds to zero at this thinness), so the
	// meaningful check — Area against the region's own published Outer
	// polyline, within the discretized boundary's own sampling error — is
	// made against the big wedge, the profile the defect actually corrupted.
	big := profiles[0]
	small := profiles[1]
	if small.Area > big.Area {
		big, small = small, big
	}
	require.Greater(t, small.Area, 0.0, "the sliver still carries a tiny positive area")
	want := polylineArea(big.Outer)
	require.InDelta(t, want, big.Area, math.Abs(want)*0.05,
		"Area must agree with the wedge's own published Outer polyline, "+
			"up to its densified boundary's own sampling error")

	report := s.Verify(t.Context())
	require.True(t, report.ProfilesValid)
}
