package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestOriginExistsBeforeAnythingIsDrawn(t *testing.T) {
	s := newSketch(t)
	o := s.Origin()
	require.NotNil(t, o, "a sketch has an origin from the start")
	require.Equal(t, 0.0, o.X())
	require.Equal(t, 0.0, o.Y())
	require.True(t, o.IsFixed(), "the origin is grounded from the start")
	require.Empty(t, s.Points(), "the origin is not an authored point")
	require.Equal(t, 0, s.DOF(), "an empty sketch has no free degrees of freedom")
}

func TestOriginIsNotInPointsAndDoesNotShiftIDs(t *testing.T) {
	// The origin lives outside the authored id space, so ids stay dense from 0
	// and every existing consumer sees exactly the points it created.
	s := newSketch(t)
	a := s.CreatePoint(3, 4)
	b := s.CreatePoint(7, 1)

	require.Len(t, s.Points(), 2, "only the authored points")
	require.Equal(t, 0, a.ID())
	require.Equal(t, 1, b.ID())
	require.NotContains(t, s.Points(), s.Origin())
}

func TestOriginGroundsWithoutFix(t *testing.T) {
	// The reported use: a point that would otherwise be pinned with Fix, or left
	// unconstrained, is instead tied to the origin by an ordinary constraint.
	s := newSketch(t)
	p := s.CreatePoint(5, 5)
	c := sketch.NewCoincident(p, s.Origin())
	s.AddConstraint(c)

	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	require.InDelta(t, 0, p.X(), 1e-9, "the constraint pulls the point onto the origin")
	require.InDelta(t, 0, p.Y(), 1e-9)
	require.False(t, p.IsFixed(), "it is CONSTRAINED, not pinned — Fix was never called")
	require.Equal(t, 0, s.DOF(), "both of its coordinates are determined")

	rep := s.Verify(t.Context())
	require.NoError(t, rep.Check(), "a point grounded on the origin is a trustworthy sketch")

	// Being a constraint rather than a fixed flag is the point: it is visible to
	// the diagnostics and removable like any other.
	require.Contains(t, s.Constraints(), c)
	require.True(t, s.RemoveConstraint(c))
	require.Equal(t, 2, s.DOF(), "removing it frees the point again")
}

func TestOriginIsUsableWithEveryPointConstraint(t *testing.T) {
	// "Usable as a constraint target the same way any other point is" — not a
	// special case the constraint layer has to know about.
	t.Run("distance from the origin", func(t *testing.T) {
		s := newSketch(t)
		p := s.CreatePoint(3, 0)
		l := s.CreateLine(s.Origin(), p)
		s.AddConstraint(sketch.NewHorizontal(l), sketch.NewDistance(s.Origin(), p, 10))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 10, math.Abs(p.X()), 1e-9, "the length is measured from the origin")
		require.InDelta(t, 0, p.Y(), 1e-9)
		require.Equal(t, 0, s.DOF())
	})

	t.Run("the origin as a line endpoint", func(t *testing.T) {
		s := newSketch(t)
		far := s.CreatePoint(4, 4)
		l := s.CreateLine(s.Origin(), far)
		s.AddConstraint(sketch.NewVertical(l), sketch.NewDistance(s.Origin(), far, 6))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 0, far.X(), 1e-9)
		require.InDelta(t, 6, math.Abs(far.Y()), 1e-9)
		require.Equal(t, 0, s.DOF(), "one endpoint is the grounded origin")
	})

	t.Run("a circle centred on the origin", func(t *testing.T) {
		s := newSketch(t)
		c := s.CreateCircle(s.Origin(), 5)
		s.AddConstraint(sketch.NewRadius(c, 5))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, s.DOF(), "centre grounded, radius dimensioned")
		require.NoError(t, s.Verify(t.Context()).Check())
	})
}

func TestOriginCannotBeMovedUnfixedOrRemoved(t *testing.T) {
	s := newSketch(t)
	o := s.Origin()

	o.MoveTo(10, 20)
	require.Equal(t, 0.0, o.X(), "MoveTo is a no-op on the origin")
	require.Equal(t, 0.0, o.Y())

	s.Unfix(o)
	require.True(t, o.IsFixed(), "Unfix is a no-op on the origin")

	require.False(t, s.RemovePoint(o), "the origin is not removable")
	require.NotNil(t, s.Origin(), "and is still there afterwards")

	o.SetConstruction(true)
	require.False(t, o.IsConstruction(), "the origin is not drawn geometry")
}

func TestOriginSurvivesSolving(t *testing.T) {
	// The solver must never move it, including when the rest of the sketch is
	// dragged far away.
	s := newSketch(t)
	a := s.CreatePoint(100, 100)
	b := s.CreatePoint(140, 100)
	l := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewCoincident(a, s.Origin()), sketch.NewHorizontal(l),
		sketch.NewDistance(a, b, 40))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	require.Equal(t, 0.0, s.Origin().X(), "the solver never moves the origin")
	require.Equal(t, 0.0, s.Origin().Y())
	require.InDelta(t, 0, a.X(), 1e-9, "the geometry came to the origin instead")
	require.InDelta(t, 0, a.Y(), 1e-9)
}

func TestOriginIsNotAForeignHandle(t *testing.T) {
	// The origin is live-owned but deliberately absent from s.points, so the
	// integrity scan must recognise it rather than read it as a foreign handle —
	// which would abort verification for every sketch that uses it.
	s := newSketch(t)
	p := s.CreatePoint(1, 1)
	s.AddConstraint(sketch.NewCoincident(p, s.Origin()))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context())
	require.False(t, rep.ForeignHandles, "the sketch's own origin is not foreign")
	require.Empty(t, rep.BrokenReferences)
	require.NoError(t, rep.Check())
}

func TestOriginOfAnotherSketchIsForeign(t *testing.T) {
	// Each sketch owns its own origin; borrowing another's is a cross-sketch
	// reference and must read as one.
	w := sketch.NewWorld()
	s1, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	s2, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	require.NotSame(t, s1.Origin(), s2.Origin(), "each sketch has its own")

	p := s1.CreatePoint(1, 1)
	s1.AddConstraint(sketch.NewCoincident(p, s2.Origin()))

	rep := s1.Verify(t.Context())
	require.True(t, rep.ForeignHandles, "another sketch's origin is a foreign handle")
	require.ErrorIs(t, rep.Check(), sketch.ErrForeignHandle)
}

func TestOriginRoundTripsThroughJSON(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(5, 0)
	l := s.CreateLine(s.Origin(), p)
	s.AddConstraint(sketch.NewCoincident(p, s.Origin()), sketch.NewHorizontal(l))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var back sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &back), "a document using the origin loads")
	require.NotNil(t, back.Origin(), "the rebuilt sketch has its own origin")
	require.Len(t, back.Points(), 1, "the origin is not serialized as a point")

	// The reloaded constraint points at the RELOADED sketch's origin, not a copy.
	pts, _ := sketch.ConstraintRefs(back.Constraints()[0])
	require.Contains(t, pts, back.Origin())

	_, err = back.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, s.DOF(), back.DOF(), "the round trip preserves the grounding")
	require.InDelta(t, 0, back.Points()[0].X(), 1e-9)
}

func TestOriginReferenceRaisesTheDocumentVersion(t *testing.T) {
	// A document referencing the origin carries a point reference older readers
	// cannot resolve, so it declares a version they reject. A document that does
	// NOT use the origin keeps the old version and stays readable by them.
	version := func(s *sketch.Sketch) float64 {
		data, err := json.Marshal(s)
		require.NoError(t, err)
		var doc map[string]any
		require.NoError(t, json.Unmarshal(data, &doc))
		v, ok := doc["version"].(float64)
		require.True(t, ok, "the document declares a version")
		return v
	}

	plain := newSketch(t)
	plain.CreateLine(plain.CreatePoint(0, 0), plain.CreatePoint(5, 0))
	baseline := version(plain)

	usesOrigin := newSketch(t)
	q := usesOrigin.CreatePoint(1, 1)
	usesOrigin.AddConstraint(sketch.NewCoincident(q, usesOrigin.Origin()))

	require.Greater(t, version(usesOrigin), baseline,
		"using the origin raises the declared version above the baseline")

	// Merely HAVING an origin does not: every sketch has one.
	untouched := newSketch(t)
	untouched.CreatePoint(2, 2)
	require.Equal(t, baseline, version(untouched),
		"a sketch that never references its origin writes the old version")
}

func TestOriginInWorldDocumentRoundTrips(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XZ())
	require.NoError(t, err)
	p := s.CreatePoint(2, 0)
	s.AddConstraint(sketch.NewCoincident(p, s.Origin()))
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(w)
	require.NoError(t, err)

	var back sketch.World
	require.NoError(t, json.Unmarshal(data, &back))
	require.Len(t, back.Sketches(), 1)
	rs := back.Sketches()[0]
	require.NotNil(t, rs.Origin())
	pts, _ := sketch.ConstraintRefs(rs.Constraints()[0])
	require.Contains(t, pts, rs.Origin(), "the world loader rebuilt the origin reference")
}

func TestOriginIsNotReportedAsFree(t *testing.T) {
	s := newSketch(t)
	s.CreatePoint(4, 4) // one genuinely free point
	rep := s.Verify(t.Context())
	require.Len(t, rep.FreePoints, 1, "only the authored point is free")
	require.NotContains(t, rep.FreePoints, s.Origin())
	require.Equal(t, 2, rep.DOF, "the origin contributes no degrees of freedom")
}

func TestOriginAnchorMatchesAFixedAnchor(t *testing.T) {
	// Grounding on the origin and grounding by fixing an authored point at (0,0)
	// must give the SAME verdict. They did not: the finite-difference pass
	// translates every position by the authored centroid, and the origin — being
	// outside s.points — was left behind. That is not a rigid translation, so with
	// a single authored point the shift collapsed it onto the origin, and the
	// engine reported a phantom redundant constraint and a lost degree of freedom.
	build := func(useOrigin bool) *sketch.Sketch {
		s := newSketch(t)
		var anchor *sketch.Point
		if useOrigin {
			anchor = s.Origin()
		} else {
			anchor = s.CreatePoint(0, 0)
			s.Fix(anchor)
		}
		p := s.CreatePoint(3, 0)
		l := s.CreateLine(anchor, p)
		s.AddConstraint(sketch.NewHorizontal(l), sketch.NewDistance(anchor, p, 10))
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		return s
	}

	withOrigin, withFix := build(true), build(false)
	require.Equal(t, withFix.DOF(), withOrigin.DOF(), "same sketch, same degrees of freedom")
	require.Equal(t, 0, withOrigin.DOF())
	require.Empty(t, withOrigin.Verify(t.Context()).Redundant,
		"anchoring on the origin invents no redundant constraint")
	require.NoError(t, withOrigin.Verify(t.Context()).Check())
}
