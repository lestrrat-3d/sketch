package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestRemoveConstraint(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(18, 2)
	c := s.CreatePoint(17, 11)
	d := s.CreatePoint(1, 13)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	dc := s.CreateLine(d, c)
	ad := s.CreateLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	w := sketch.NewDistance(a, b, 20)
	s.AddConstraint(w)
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	res, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "fully constrained")

	require.True(t, s.RemoveConstraint(w), "width dimension removed")
	require.False(t, s.RemoveConstraint(w), "second removal is a no-op")

	res, err = s.Solve()
	require.NoError(t, err)
	require.Equal(t, 1, res.DOF, "width is free again")
}

func TestRemoveEntityCascades(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	line := s.CreateLine(a, b)

	center := s.CreatePoint(5, 5)
	s.Fix(center)
	start := s.CreatePoint(8, 5)
	end := s.CreatePoint(5, 8)
	arc := s.CreateArc(center, start, end) // auto-adds internal arcRadius
	s.AddConstraint(sketch.NewTangent(line, arc))

	before := len(s.Constraints())
	require.True(t, s.RemoveEntity(arc), "arc removed")
	require.Len(t, s.Constraints(), before-2, "tangent and internal arcRadius cascaded")
	require.Len(t, s.Entities(), 1, "line remains")

	// The arc's points survive; remove an orphan explicitly.
	require.True(t, s.RemovePoint(start), "orphaned arc point removable")
}

func TestRemoveEntityRetiresVars(t *testing.T) {
	s := newSketch(t)
	o := s.CreatePoint(0, 0)
	s.Fix(o)
	circ := s.CreateCircle(o, 5)
	require.Equal(t, 1, s.DOF(), "radius is the only free variable")

	require.True(t, s.RemoveEntity(circ), "circle removed")
	require.Equal(t, 0, s.DOF(), "retired radius var no longer counts")
}

func TestRemovePointGuards(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.CreateLine(a, b)
	require.False(t, s.RemovePoint(a), "endpoint of a live line is not removable")
	require.Len(t, s.Points(), 2, "nothing changed")

	orphan := s.CreatePoint(3, 4)
	s.AddConstraint(sketch.NewCoincident(orphan, a))
	consBefore := len(s.Constraints())
	require.True(t, s.RemovePoint(orphan), "orphan point removable")
	require.Len(t, s.Constraints(), consBefore-1, "its constraint cascaded")
	require.Len(t, s.Points(), 2, "spliced out")
}

func TestRemoveKeepsUnrelatedConstraints(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(8, 1)
	s.Fix(a)
	line := s.CreateLine(a, b)
	d := sketch.NewDistance(a, b, 10) // references the points, not the line
	s.AddConstraint(d)

	require.True(t, s.RemoveEntity(line), "line removed")
	require.Contains(t, s.Constraints(), sketch.Constraint(d), "point dimension survives")

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 10, a.DistanceTo(b), 1e-6, "dimension still drives the points")
}

func TestReAddAfterRemove(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	c1 := s.CreateCircle(center, 5)
	require.True(t, s.RemoveEntity(c1), "removed")
	// The center point survives entity removal and can carry a fresh circle.
	c2 := s.CreateCircle(center, 5)
	require.NotSame(t, c1, c2, "a new circle is a fresh instance")
	require.Len(t, s.Entities(), 1, "one live entity")
}

func TestRemovalJSONRoundTrip(t *testing.T) {
	s := newSketch(t)
	o1 := s.CreatePoint(0, 0)
	o2 := s.CreatePoint(20, 0)
	o3 := s.CreatePoint(40, 0)
	s.Fix(o1)
	s.Fix(o2)
	s.Fix(o3)
	c1 := s.CreateCircle(o1, 3)
	c2 := s.CreateCircle(o2, 4)
	c3 := s.CreateCircle(o3, 5)
	s.AddConstraint(sketch.NewRadius(c3, 7)) // references the LAST entity

	require.True(t, s.RemoveEntity(c2), "middle circle removed")

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	require.Contains(t, string(data), `"version":2`, "version written")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	require.Len(t, s2.Entities(), 2, "two circles after removal")

	_, err = s2.Solve()
	require.NoError(t, err)
	// The radius dim must still target the (renumbered) third circle.
	reloaded, ok := s2.Entities()[1].(*sketch.Circle)
	require.True(t, ok, "renumbered entity is a circle")
	require.InDelta(t, 7, reloaded.R(), 1e-6, "dimension follows the renumbered id")
	first, ok := s2.Entities()[0].(*sketch.Circle)
	require.True(t, ok, "first entity intact")
	require.InDelta(t, 3, first.R(), 1e-6, "unconstrained circle keeps its radius")
	_ = c1
}

func TestJSONVersionGuard(t *testing.T) {
	s := newSketch(t)
	s.CreatePoint(1, 2)
	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")

	// Legacy document: no version field at all.
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &doc), "decode to map")
	delete(doc, "version")
	legacy, err := json.Marshal(doc)
	require.NoError(t, err, "re-encode legacy")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(legacy, &s2), "legacy document loads")

	// Future document: rejected loudly.
	doc["version"] = json.RawMessage("3")
	future, err := json.Marshal(doc)
	require.NoError(t, err, "re-encode future")
	var s3 sketch.Sketch
	require.ErrorContains(t, json.Unmarshal(future, &s3), "unsupported document version 3")
}

func TestRemoveSplineGuardsControlPoints(t *testing.T) {
	s := newSketch(t)
	sp, err := s.CreateSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 4), s.CreatePoint(8, 4), s.CreatePoint(10, 0))
	require.NoError(t, err)
	require.False(t, s.RemovePoint(sp.Control[2]), "control point of a live spline is not removable")
	require.True(t, s.RemoveEntity(sp), "spline removable")
	require.True(t, s.RemovePoint(sp.Control[2]), "control point orphaned after spline removal")
}
