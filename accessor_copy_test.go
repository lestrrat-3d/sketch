package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestEntitiesReturnsCopy is the accessor contract: the slice Sketch.Entities
// hands back is a copy, so a caller cannot write THROUGH it into the sketch's
// entity slice. The elements are still the live handles.
func TestEntitiesReturnsCopy(t *testing.T) {
	s, pts := squareSketch(t)
	bl, tl := pts[0], pts[3]

	before := s.Entities()
	require.Len(t, before, 4)

	// The elements must be the live handles, not clones of them.
	require.Same(t, before[0], s.Entities()[0])

	// Overwrite a slot of the returned slice with an entity the sketch never made.
	intruder := &sketch.Line{Start: tl, End: bl}
	before[0] = intruder

	after := s.Entities()
	require.NotSame(t, intruder, after[0], "write to the returned slice must not reach s.ents")
	for i, e := range after {
		require.NotSame(t, intruder, e, "intruder must not appear at any slot (i=%d)", i)
	}
	// And the sketch's own view is unchanged: same handles, same order.
	require.Equal(t, 4, len(after))
	for i := 1; i < 4; i++ {
		require.Same(t, before[i], after[i])
	}
}

// TestPointsReturnsCopy mirrors TestEntitiesReturnsCopy for the point slice,
// whose position IS the point's id.
func TestPointsReturnsCopy(t *testing.T) {
	s, _ := squareSketch(t)

	before := s.Points()
	require.Len(t, before, 4)
	require.Same(t, before[0], s.Points()[0])

	intruder := &sketch.Point{}
	before[0] = intruder

	after := s.Points()
	require.NotSame(t, intruder, after[0], "write to the returned slice must not reach s.points")
	require.Same(t, before[1], after[1])
}

// TestConstraintsReturnsCopy mirrors it for the constraint slice, whose order is
// the row->constraint attribution every diagnostic depends on.
func TestConstraintsReturnsCopy(t *testing.T) {
	s, pts := squareSketch(t)
	bl, br := pts[0], pts[1]
	c := sketch.NewHorizontal(s.Entities()[0].(*sketch.Line))
	s.AddConstraint(c)
	d := sketch.NewDistance(bl, br, 10)
	s.AddConstraint(d)

	before := s.Constraints()
	require.Len(t, before, 2)

	// Duplicate the first constraint over the second slot: if the slice were live,
	// the distance constraint would vanish from the sketch's own residual set.
	before[1] = before[0]

	after := s.Constraints()
	require.Len(t, after, 2)
	require.Same(t, c, after[0])
	require.Same(t, d, after[1], "write to the returned slice must not reach s.cons")
}

// TestPlanesAndSketchesReturnCopy: the world's slices are the plane id space and
// the sketch list; neither may be written through either.
func TestPlanesAndSketchesReturnCopy(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)

	planes := w.Planes()
	require.Len(t, planes, 3)
	planes[0] = planes[2]
	require.Same(t, w.XY(), w.Planes()[0], "write to the returned slice must not reach w.planes")

	sketches := w.Sketches()
	require.Len(t, sketches, 1)
	sketches[0] = nil
	require.Same(t, s, w.Sketches()[0], "write to the returned slice must not reach w.sketches")
}

// TestRevisionEntitySwapThroughAccessor is the adversarial repro: replace an
// entity slot through Sketch.Entities with a hand-built, never-committed Line,
// build a profile, then replace it AGAIN with another identical hand-built Line.
// The two intruders are indistinguishable by type, points and shape, so if the
// write reached s.ents (or if an unstamped entity hashed as uid 0) the revision
// would stay equal and the profile — still holding the FIRST intruder — would
// report fresh.
//
// With Entities returning a copy the sketch never sees either intruder at all:
// its entity set, revision and profile all stay exactly as built.
func TestRevisionEntitySwapThroughAccessor(t *testing.T) {
	s, pts := squareSketch(t)
	bl, tl := pts[0], pts[3]

	original := s.Entities()[3] // the tl->bl closing edge
	rev0 := s.Revision()

	ents := s.Entities()
	ents[3] = &sketch.Line{Start: tl, End: bl}

	// The sketch's own entity set is untouched by the write.
	require.Same(t, original, s.Entities()[3])
	require.Equal(t, rev0, s.Revision(), "an unreachable write must not change the fingerprint")

	profs := s.Profiles()
	require.Len(t, profs, 1)
	require.False(t, profs[0].IsStale())

	// A second, identical intruder through a fresh call.
	ents2 := s.Entities()
	ents2[3] = &sketch.Line{Start: tl, End: bl}

	require.Same(t, original, s.Entities()[3], "s.ents must still hold the committed entity")
	require.Equal(t, rev0, s.Revision())
	require.False(t, profs[0].IsStale(), "nothing changed, so the profile is not stale")

	// The profile's live handles still belong to the sketch — no dangling instance.
	owned := map[sketch.Entity]struct{}{}
	for _, e := range s.Entities() {
		owned[e] = struct{}{}
	}
	for _, e := range profs[0].Entities {
		require.Contains(t, owned, e, "profile handle must still be owned by the sketch")
	}

	// And a REAL mutation through the API still moves the revision, so the copy did
	// not just freeze the fingerprint.
	s.RemoveEntity(original)
	s.CreateLine(tl, bl)
	require.NotEqual(t, rev0, s.Revision())
	require.True(t, profs[0].IsStale())
}
