package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// buildIntrospectionSketch assembles one sketch containing a broad spread of
// public constraint types plus the internal arc-radius constraint (via
// CreateArc), so the introspection helpers can be exercised over real handles in
// creation order.
func buildIntrospectionSketch(s *sketch.Sketch) {
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(10, 8)
	d := s.CreatePoint(0, 8)
	s.Fix(a)
	s.Fix(b)

	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	cd := s.CreateLine(c, d)

	o1 := s.CreatePoint(3, 3)
	o2 := s.CreatePoint(7, 3)
	c1 := s.CreateCircle(o1, 1.5)
	c2 := s.CreateCircle(o2, 1.5)

	s.CreateArc(s.CreatePoint(5, 6), s.CreatePoint(6, 6), s.CreatePoint(4, 6))
	ell := s.CreateEllipse(s.CreatePoint(5, 12), 3, 2, 0)

	s.AddConstraint(
		sketch.NewCoincident(s.CreatePoint(0, 0), a),
		sketch.NewHorizontal(ab),
		sketch.NewVertical(bc),
		sketch.NewParallel(ab, cd),
		sketch.NewPerpendicular(ab, bc),
		sketch.NewPointOnLine(s.CreatePoint(5, 0), ab),
		sketch.NewCollinear(ab, s.CreateLine(s.CreatePoint(12, 0), s.CreatePoint(15, 0))),
		sketch.NewConcentric(c1, s.CreateCircle(s.CreatePoint(3.2, 3.1), 0.5)),
		sketch.NewPointOnCircle(s.CreatePoint(4.5, 3), c1),
		sketch.NewPointOnEllipse(s.CreatePoint(8, 12), ell),
		sketch.NewMidpoint(s.CreatePoint(5, 0.1), ab),
		sketch.NewSymmetric(s.CreatePoint(2, 2), s.CreatePoint(8, 2), bc),
		sketch.NewEqual(ab, cd),
		sketch.NewEqualRadius(c1, c2),
		sketch.NewTangent(cd, c1),
		sketch.NewTangentCircles(c1, c2, false),
		sketch.NewDistance(a, b, 10),
		sketch.NewHorizontalDistance(a, b, 10),
		sketch.NewVerticalDistance(a, d, 8),
		sketch.NewDistancePointLine(o1, ab, 3),
		sketch.NewDistanceLines(ab, cd, 8),
		sketch.NewOffset(ab, cd, 8),
		sketch.NewRadius(c1, 1.5),
		sketch.NewDiameter(c2, 3),
		sketch.NewAngle(ab, bc, 90),
		sketch.NewSemiMajor(ell, 3),
		sketch.NewSemiMinor(ell, 2),
		sketch.NewEllipseRotation(ell, 0),
	)
}

// TestConstraintKindMatchesJSON pins that ConstraintKind stays identical to the
// serialization "type" strings: over the non-internal constraints, in document
// order, the two must agree.
func TestConstraintKindMatchesJSON(t *testing.T) {
	s := newSketch(t)
	buildIntrospectionSketch(s)

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var doc struct {
		Constraints []struct {
			Type string `json:"type"`
		} `json:"constraints"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))

	var kinds []string
	for _, c := range s.Constraints() {
		if sketch.IsInternal(c) {
			continue
		}
		kinds = append(kinds, sketch.ConstraintKind(c))
	}
	require.Len(t, doc.Constraints, len(kinds))
	for i, jc := range doc.Constraints {
		require.Equal(t, jc.Type, kinds[i], "constraint %d", i)
	}
}

// TestConstraintKindExhaustive guards against a constraint type reachable in a
// sketch without its ConstraintKind / constraintRefs cases: every constraint
// must report a non-empty kind and at least one reference.
func TestConstraintKindExhaustive(t *testing.T) {
	s := newSketch(t)
	buildIntrospectionSketch(s)

	for i, c := range s.Constraints() {
		require.NotEmpty(t, sketch.ConstraintKind(c), "constraint %d has no kind", i)
		pts, ents := sketch.ConstraintRefs(c)
		require.True(t, len(pts)+len(ents) > 0, "constraint %d (%s) has no refs", i, sketch.ConstraintKind(c))
	}
}

func TestConstraintRefsIdentity(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(5, 5)
	ab := s.CreateLine(a, b)
	axis := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))

	pol := sketch.NewPointOnLine(c, ab)
	pts, ents := sketch.ConstraintRefs(pol)
	require.Len(t, pts, 1)
	require.Same(t, c, pts[0])
	require.Len(t, ents, 1)
	require.Same(t, ab, ents[0])

	sym := sketch.NewSymmetric(a, b, axis)
	pts, ents = sketch.ConstraintRefs(sym)
	require.Len(t, pts, 2)
	require.Same(t, a, pts[0])
	require.Same(t, b, pts[1])
	require.Len(t, ents, 1)
	require.Same(t, axis, ents[0])

	dist := sketch.NewDistance(a, b, 10)
	pts, ents = sketch.ConstraintRefs(dist)
	require.Len(t, pts, 2)
	require.Same(t, a, pts[0])
	require.Same(t, b, pts[1])
	require.Empty(t, ents)

	// Fresh slices: mutating one call's result must not leak into the next.
	pts[0] = c
	again, _ := sketch.ConstraintRefs(dist)
	require.Same(t, a, again[0])
}

// TestConstraintKindNilOperandsSafe pins that ConstraintKind is a pure query:
// asking a constraint's kind must never dereference its operands, so it does not
// panic even on a constraint constructed with nil points.
func TestConstraintKindNilOperandsSafe(t *testing.T) {
	require.NotPanics(t, func() {
		require.Equal(t, "coincident", sketch.ConstraintKind(sketch.NewCoincident(nil, nil)))
		require.Equal(t, "distance", sketch.ConstraintKind(sketch.NewDistance(nil, nil, 0)))
	})
}

func TestConstraintResiduals(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(6, 0)
	s.Fix(a)

	d := sketch.NewDistance(a, b, 10)
	s.AddConstraint(d)

	// Pre-solve: points are 6 apart, target 10 -> residual -4.
	res := sketch.ConstraintResiduals(d)
	require.Len(t, res, 1)
	require.InDelta(t, -4, res[0], 1e-12)

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	res = sketch.ConstraintResiduals(d)
	require.Len(t, res, 1)
	require.InDelta(t, 0, res[0], 1e-8)
}

func TestInternalConstraintIntrospection(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	start := s.CreatePoint(5, 0)
	end := s.CreatePoint(0, 5)
	arc := s.CreateArc(center, start, end)

	var internal []sketch.Constraint
	for _, c := range s.Constraints() {
		if sketch.IsInternal(c) {
			internal = append(internal, c)
		}
	}
	require.Len(t, internal, 1)
	require.Equal(t, "arc_radius", sketch.ConstraintKind(internal[0]))
	pts, ents := sketch.ConstraintRefs(internal[0])
	require.Empty(t, pts)
	require.Len(t, ents, 1)
	require.Same(t, arc, ents[0].(*sketch.Arc))
}
