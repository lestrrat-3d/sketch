package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/sketchtest"
	"github.com/stretchr/testify/require"
)

// TestProfilesArcLensDoesNotAdoptDisjointCircleAsHole reaches the same defect
// geom's TestRegionsArcLensDoesNotAdoptDisjointCircleAsHole pins, through the
// public Sketch.Profiles() API with no options: two arcs forming a small lens,
// plus a circle whose bounding box is fully disjoint from the lens's. Before
// the fix, angInFragment's wrap aliased a ray-cast crossing onto the wrong arc
// fragment, and Sketch.Profiles() published the lens with the far circle
// subtracted as its hole (Area=0.0037486246867295143, Valid=true) and the
// circle published a second time as its own region — reachable with no
// options at all, so nothing in Sketch.Verify's profile trust gate caught it.
func TestProfilesArcLensDoesNotAdoptDisjointCircleAsHole(t *testing.T) {
	s := newSketch(t)

	c1 := s.CreatePoint(0.35092155472478259, -0.92392819187699393)
	s1 := s.CreatePoint(0.91737230032440398, -1.1556555089536711)
	e1 := s.CreatePoint(-0.17199298451132139, -0.6059275923092966)
	s.CreateArc(c1, s1, e1)

	c2 := s.CreatePoint(0.34326886354944652, -0.6557519008634316)
	s2 := s.CreatePoint(0.32461533217526556, -1.0464592649851203)
	e2 := s.CreatePoint(-0.022307692217990949, -0.51663331660933887)
	s.CreateArc(c2, s2, e2)

	circCenter := s.CreatePoint(-0.81323037456758496, -0.91951610002249584)
	s.CreateCircle(circCenter, 0.068365088974454882)

	report := sketchtest.Verify(t, s)
	require.True(t, report.ProfilesValid, "no invalid profile from this scene")
	profiles := s.Profiles()
	require.Len(t, profiles, 2, "the lens and the circle, each its own profile")

	var lens *sketch.Profile
	for _, p := range profiles {
		if len(p.Outer) == 2 { // the lens has two arc edges; the circle has one
			lens = p
		}
	}
	require.NotNil(t, lens, "the two-arc lens must be one of the published profiles")
	require.Empty(t, lens.Holes, "the disjoint circle must never be adopted as the lens's hole")
	sketchtest.MeasuresProfileArea(t, lens, 0.01843175453393291, sketchtest.Within(1e-9))
	sketchtest.IsValidProfile(t, lens)
}
