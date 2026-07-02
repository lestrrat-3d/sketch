package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestNamesRoundTrip pins durable identity across a save/load boundary: named
// points, entities and constraints (geometric and dimensional) must be
// retrievable by name from the reloaded sketch.
func TestNamesRoundTrip(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20, 0)
	c := s.CreatePoint(20, 12)
	d := s.CreatePoint(0, 12)
	a.SetName("origin")
	s.Fix(a)

	top := s.CreateLine(d, c)
	top.SetName("top edge")
	s.CreateLine(a, b)
	arc := s.CreateArc(s.CreatePoint(10, 20), s.CreatePoint(13, 20), s.CreatePoint(7, 20))
	arc.SetName("cap")

	par := sketch.NewParallel(top, s.CreateLine(a, b))
	s.AddConstraint(par)
	s.SetConstraintName(par, "tops parallel")

	width := sketch.NewDistance(a, b, 20)
	s.AddConstraint(width)
	s.SetConstraintName(width, "width")

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))

	require.NotNil(t, s2.PointByName("origin"))
	require.Equal(t, 0.0, s2.PointByName("origin").X())

	e := s2.EntityByName("top edge")
	require.NotNil(t, e)
	require.IsType(t, &sketch.Line{}, e)
	require.IsType(t, &sketch.Arc{}, s2.EntityByName("cap"))

	pc := s2.ConstraintByName("tops parallel")
	require.NotNil(t, pc)
	require.Equal(t, "parallel", sketch.ConstraintKind(pc))

	wc := s2.ConstraintByName("width")
	require.NotNil(t, wc)
	require.Equal(t, "distance", sketch.ConstraintKind(wc))
	require.InDelta(t, 20, wc.(*sketch.Distance).Target().Mag(), 1e-12)
}

// TestNamesWorldRoundTrip pins that names survive the world document path too
// (json_world.go), which decodes through the same shared body as a standalone
// sketch.
func TestNamesWorldRoundTrip(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)

	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	edge := s.CreateLine(a, b)
	edge.SetName("base")
	h := sketch.NewHorizontal(edge)
	s.AddConstraint(h)
	s.SetConstraintName(h, "level")

	data, err := json.Marshal(w)
	require.NoError(t, err)

	var w2 sketch.World
	require.NoError(t, json.Unmarshal(data, &w2))
	require.Len(t, w2.Sketches(), 1)
	s2 := w2.Sketches()[0]

	require.IsType(t, &sketch.Line{}, s2.EntityByName("base"))
	require.Equal(t, "horizontal", sketch.ConstraintKind(s2.ConstraintByName("level")))
}

func TestNameLookupSemantics(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(1, 0)
	first := s.CreateLine(a, b)
	second := s.CreateLine(b, a)
	first.SetName("edge")
	second.SetName("edge")

	require.Same(t, sketch.Entity(first), s.EntityByName("edge"), "first match in creation order")
	require.Nil(t, s.EntityByName("nope"))
	require.Nil(t, s.PointByName("nope"))
	require.Nil(t, s.ConstraintByName("nope"))
	require.Nil(t, s.EntityByName(""), "empty name never matches")

	// Clearing a name.
	first.SetName("")
	require.Same(t, sketch.Entity(second), s.EntityByName("edge"))

	co := sketch.NewCoincident(a, b)
	s.AddConstraint(co)
	s.SetConstraintName(co, "join")
	require.Equal(t, "join", s.ConstraintName(co))
	s.SetConstraintName(co, "")
	require.Equal(t, "", s.ConstraintName(co))
	require.Nil(t, s.ConstraintByName("join"))
}

// TestConstraintNameOnlyLiveNonInternal pins that SetConstraintName ignores a
// constraint that is not a live, user-facing member of the sketch — a detached
// (not-yet-added) or internal (auto-added) constraint — so no dangling label is
// left that ConstraintByName can't resolve and JSON can't persist.
func TestConstraintNameOnlyLiveNonInternal(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(5, 0)

	// Not added to the sketch → no-op.
	detached := sketch.NewCoincident(a, b)
	s.SetConstraintName(detached, "ghost")
	require.Equal(t, "", s.ConstraintName(detached))
	require.Nil(t, s.ConstraintByName("ghost"))

	// Internal (auto-added) constraint → no-op (never serializes).
	s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(1, 0), s.CreatePoint(0, 1))
	var internal sketch.Constraint
	for _, c := range s.Constraints() {
		if sketch.IsInternal(c) {
			internal = c
		}
	}
	require.NotNil(t, internal)
	s.SetConstraintName(internal, "radius-guard")
	require.Equal(t, "", s.ConstraintName(internal))
	require.Nil(t, s.ConstraintByName("radius-guard"))

	// Once added, naming a user-facing constraint works.
	live := sketch.NewCoincident(a, b)
	s.AddConstraint(live)
	s.SetConstraintName(live, "join")
	require.Same(t, live, s.ConstraintByName("join"))
}

// TestNamesRemovalCleanup pins that removal purges constraint-name entries:
// both direct RemoveConstraint and the RemoveEntity cascade.
func TestNamesRemovalCleanup(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(5, 0)
	l := s.CreateLine(a, b)

	h := sketch.NewHorizontal(l)
	s.AddConstraint(h)
	s.SetConstraintName(h, "level")

	require.True(t, s.RemoveEntity(l), "cascade removes the constraint")
	require.Nil(t, s.ConstraintByName("level"))
	require.Equal(t, "", s.ConstraintName(h), "name purged with the cascade")

	l2 := s.CreateLine(a, b)
	h2 := sketch.NewHorizontal(l2)
	s.AddConstraint(h2)
	s.SetConstraintName(h2, "level2")
	require.True(t, s.RemoveConstraint(h2))
	require.Equal(t, "", s.ConstraintName(h2), "name purged on direct removal")
}

// TestNamesLegacyDocument pins that documents without name fields still load
// (unnamed entities/constraints come back empty). A kind-less, version-absent
// document loads as a world-XY sketch.
func TestNamesLegacyDocument(t *testing.T) {
	legacy := `{
		"points": [{"x": 0, "y": 0}, {"x": 5, "y": 0}],
		"entities": [{"type": "line", "points": [0, 1]}],
		"constraints": [{"type": "horizontal", "entities": [0]}]
	}`
	var s sketch.Sketch
	require.NoError(t, json.Unmarshal([]byte(legacy), &s))
	require.Equal(t, "", s.Entities()[0].Name())
	require.Equal(t, "", s.ConstraintName(s.Constraints()[0]))
}
