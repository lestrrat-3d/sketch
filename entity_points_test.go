package sketch_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// entityPointSlots returns every *Point slot reachable through an entity's
// exported fields — a plain *Point field (Line.Start, Arc.Center, …) and each
// element of a []*Point field (Spline.Control, FitSpline.Fit, …). It is
// deliberately reflective rather than a hand-written list: a hand-written list
// is the very thing this test exists to stop anyone from having to maintain.
func entityPointSlots(t *testing.T, e sketch.Entity) []reflect.Value {
	t.Helper()
	pointType := reflect.TypeOf((*sketch.Point)(nil))
	v := reflect.ValueOf(e).Elem()
	et := v.Type()
	var slots []reflect.Value
	for i := range et.NumField() {
		f := et.Field(i)
		if !f.IsExported() {
			continue
		}
		switch {
		case f.Type == pointType:
			slots = append(slots, v.Field(i))
		case f.Type.Kind() == reflect.Slice && f.Type.Elem() == pointType:
			fv := v.Field(i)
			for j := range fv.Len() {
				slots = append(slots, fv.Index(j))
			}
		}
	}
	return slots
}

// TestEntityDefiningPointsAreAllSeen pins the property the engine's single
// entityPoints definition exists to provide: EVERY point an entity is drawn from
// is seen by the consumers that walk an entity's defining points.
//
// The check is mechanical. An entity's defining points ARE its exported *Point
// fields, so the test reflects over those fields instead of restating the list —
// which means a new point on an existing entity type is covered the moment the
// field is added, with no test edit. (A brand-new entity TYPE still has to be
// added to the sketch built below; nothing at run time can enumerate the types
// implementing a sealed interface.)
//
// The two observable consumers assert the two halves that fail SILENTLY when a
// defining point is missed, and neither can be faked by a coincidence:
//
//   - Rewiring a slot to another point of the SAME sketch moves no solver
//     variable, so the whole var vector is unchanged. Only the per-entity
//     defining-point hash can make [Sketch.Revision] move — and it must, or a
//     [sketch.Profile] built before the rewire reads fresh while describing
//     geometry that no longer exists.
//   - Rewiring a slot to a point of ANOTHER sketch must make Verify report a
//     foreign handle. Missed, the sketch verifies clean while a solver-visible
//     handle belongs to a different sketch.
func TestEntityDefiningPointsAreAllSeen(t *testing.T) {
	s := newSketch(t)

	// One of every entity type. Coordinates are spread out so no two entities
	// share a point: each slot must be caught on its own.
	s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
	s.CreateCircle(s.CreatePoint(30, 0), 4)
	s.CreateArc(s.CreatePoint(50, 0), s.CreatePoint(55, 0), s.CreatePoint(50, 5))
	s.CreateEllipse(s.CreatePoint(70, 0), 6, 3, 0)
	s.CreateEllipticalArc(s.CreatePoint(90, 0), s.CreatePoint(96, 0), s.CreatePoint(90, 3), 6, 3, 0)

	_, err := s.CreateSpline(s.CreatePoint(0, 30), s.CreatePoint(4, 34), s.CreatePoint(8, 30), s.CreatePoint(12, 34))
	require.NoError(t, err)
	_, err = s.CreateClosedSpline(s.CreatePoint(30, 30), s.CreatePoint(38, 30), s.CreatePoint(34, 38))
	require.NoError(t, err)
	_, err = s.CreateFitSpline(s.CreatePoint(50, 30), s.CreatePoint(55, 36), s.CreatePoint(60, 30))
	require.NoError(t, err)
	_, err = s.CreateConic(s.CreatePoint(70, 30), s.CreatePoint(75, 38), s.CreatePoint(80, 30), 0.4)
	require.NoError(t, err)
	nurbsControl := []*sketch.Point{s.CreatePoint(0, 60), s.CreatePoint(5, 68), s.CreatePoint(10, 60)}
	_, err = s.CreateNURBS(2, nurbsControl, nil, sketch.ClampedUniformKnots(len(nurbsControl), 2))
	require.NoError(t, err)

	require.Len(t, s.Entities(), 10, "one of every entity type")

	// A point of the same sketch that nothing is drawn from, and a point of a
	// different sketch entirely.
	spare := s.CreatePoint(-40, -40)
	foreign := newSketch(t).CreatePoint(-40, -40)

	baseline := s.Verify(context.Background())
	require.False(t, baseline.ForeignHandles, "the sketch is sound before any rewire")

	for _, e := range s.Entities() {
		slots := entityPointSlots(t, e)
		require.NotEmptyf(t, slots, "%T exposes no defining point", e)

		for i, slot := range slots {
			original, ok := slot.Interface().(*sketch.Point)
			require.True(t, ok)
			rev := s.Revision()

			slot.Set(reflect.ValueOf(spare))
			require.NotEqualf(t, rev, s.Revision(),
				"%T defining point %d rewired within the sketch left the revision unchanged, "+
					"so a stale Profile would read fresh", e, i)

			slot.Set(reflect.ValueOf(foreign))
			rep := s.Verify(context.Background())
			require.Truef(t, rep.ForeignHandles,
				"%T defining point %d rewired to another sketch was not reported foreign", e, i)
			require.ErrorIs(t, rep.Check(), sketch.ErrForeignHandle)

			slot.Set(reflect.ValueOf(original))
			require.Equalf(t, rev, s.Revision(), "%T defining point %d was not restored", e, i)
		}
	}
}
