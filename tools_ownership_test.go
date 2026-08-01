package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// The modification tools build new geometry and constraints in the receiving
// sketch out of the input entity's own points. A handle the sketch does not own
// — another sketch's entity, or one already removed — must therefore be refused
// before anything is manufactured, in the shape each signature already uses for
// reference geometry: an error for the Create… tools, nil for CreateMirror, and
// false for Break/Trim/Extend.

// foreignLine returns a live line of a DIFFERENT sketch in the same world,
// spanning (x1,y1)-(x2,y2).
func foreignLine(t *testing.T, x1, y1, x2, y2 float64) *sketch.Line {
	t.Helper()
	w := sketch.NewWorld()
	o, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	return o.CreateLine(o.CreatePoint(x1, y1), o.CreatePoint(x2, y2))
}

func TestToolsRejectForeignEntity(t *testing.T) {
	t.Run("Break", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 10, 0)

		e1, e2, ok := s.Break(fl, 5, 0)
		require.False(t, ok)
		require.Nil(t, e1)
		require.Nil(t, e2)
		require.Empty(t, s.Entities(), "nothing manufactured in the receiving sketch")
		require.Empty(t, s.Constraints())
	})

	t.Run("Trim", func(t *testing.T) {
		s := newSketch(t)
		// A local cutter crossing the foreign line: without the guard Trim would
		// find it and report a trim it never performed.
		s.CreateLine(s.CreatePoint(5, -5), s.CreatePoint(5, 5))
		fl := foreignLine(t, 0, 0, 10, 0)

		nl, ok := s.Trim(fl, 9, 0)
		require.False(t, ok)
		require.Nil(t, nl)
		require.Len(t, s.Entities(), 1, "only the cutter")
		require.Empty(t, s.Constraints())
	})

	t.Run("Extend", func(t *testing.T) {
		s := newSketch(t)
		s.CreateLine(s.CreatePoint(8, -5), s.CreatePoint(8, 5))
		fl := foreignLine(t, 0, 0, 4, 0)

		nl, ok := s.Extend(fl, fl.End)
		require.False(t, ok)
		require.Nil(t, nl)
		require.Len(t, s.Entities(), 1, "only the cutter")
		require.Empty(t, s.Constraints())
	})

	t.Run("ExtendForeignEndpoint", func(t *testing.T) {
		// A foreign point that is NOT an endpoint of l. Base Extend already
		// refuses this input for a different reason — end is neither l.Start nor
		// l.End — so the subtest passes with and without the endpoint half of the
		// guard. ExtendForeignStart below covers the path only the guard closes.
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
		fl := foreignLine(t, 0, 0, 4, 0)

		nl, ok := s.Extend(l, fl.End)
		require.False(t, ok)
		require.Nil(t, nl)
		require.Len(t, s.Entities(), 1)
		require.Empty(t, s.Constraints())
	})

	t.Run("ExtendForeignStart", func(t *testing.T) {
		// The path the endpoint half of the guard closes. CreateLine stores the
		// points it is given, so a LOCAL line can hold another sketch's point as
		// its Start; that point passes the endpoint test (end == l.Start), and
		// only !s.owns(end) refuses it. Without the guard this extends the line
		// and reports true, splicing the foreign point into the replacement.
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 4, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(4, 0))
		s.CreateLine(s.CreatePoint(-8, -5), s.CreatePoint(-8, 5)) // cutter beyond the Start end

		nl, ok := s.Extend(l, l.Start)
		require.False(t, ok)
		require.Nil(t, nl)
		require.Len(t, s.Entities(), 2, "l and the cutter, nothing manufactured")
		require.Empty(t, s.Constraints())
	})

	t.Run("CreateMirror", func(t *testing.T) {
		s := newSketch(t)
		axis := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
		fl := foreignLine(t, 2, 0, 6, 0)

		require.Nil(t, s.CreateMirror([]sketch.Entity{fl}, axis))
		require.Len(t, s.Entities(), 1, "only the axis")
		require.Empty(t, s.Constraints(), "no symmetric constraints on foreign points")
	})

	t.Run("CreateMirrorForeignAxis", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(2, 0), s.CreatePoint(6, 0))
		faxis := foreignLine(t, 0, -5, 0, 5)

		require.Nil(t, s.CreateMirror([]sketch.Entity{l}, faxis))
		require.Len(t, s.Entities(), 1)
		require.Empty(t, s.Constraints())
	})

	t.Run("CreatePatternRect", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 4, 0)

		p, err := s.CreatePatternRect([]sketch.Entity{fl}, 2, 1, 10, 0)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, p)
		require.Empty(t, s.Entities())
		require.Empty(t, s.Constraints())
	})

	t.Run("CreatePatternCircular", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 4, 0, 6, 0)

		p, err := s.CreatePatternCircular([]sketch.Entity{fl}, s.CreatePoint(0, 0), 4)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, p)
		require.Empty(t, s.Entities())
		require.Empty(t, s.Constraints())
	})

	t.Run("CreatePatternCircularForeignCenter", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(4, 0), s.CreatePoint(6, 0))
		fcenter := foreignLine(t, 0, 0, 1, 0).Start

		p, err := s.CreatePatternCircular([]sketch.Entity{l}, fcenter, 4)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, p)
		require.Len(t, s.Entities(), 1, "no spokes built from a foreign centre")
		require.Empty(t, s.Constraints())
	})

	t.Run("CreateOffset", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 10, 0)

		g, err := s.CreateOffset([]sketch.Entity{fl}, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, g)
		require.Empty(t, s.Entities())
		require.Empty(t, s.Constraints())
	})

	t.Run("CreateFillet", func(t *testing.T) {
		s := newSketch(t)
		corner := s.CreatePoint(0, 0)
		l1 := s.CreateLine(corner, s.CreatePoint(10, 0))
		fl := foreignLine(t, 0, 0, 0, 10)

		f, err := s.CreateFillet(l1, fl, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, f)
		require.Len(t, s.Entities(), 1, "the local leg survives untouched")
		require.Empty(t, s.Constraints())
	})

	t.Run("CreateChamfer", func(t *testing.T) {
		s := newSketch(t)
		corner := s.CreatePoint(0, 0)
		l1 := s.CreateLine(corner, s.CreatePoint(10, 0))
		fl := foreignLine(t, 0, 0, 0, 10)

		c, err := s.CreateChamfer(l1, fl, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, c)
		require.Len(t, s.Entities(), 1)
		require.Empty(t, s.Constraints())
	})
}

// A removed handle is dead: the sketch no longer owns it, so the same guard must
// refuse it. Without it these tools resurrect deleted geometry, and nothing
// downstream — Verify, MarshalJSON — reports anything wrong.
func TestToolsRejectRemovedEntity(t *testing.T) {
	t.Run("Trim", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
		s.CreateLine(s.CreatePoint(5, -5), s.CreatePoint(5, 5))
		require.True(t, s.RemoveEntity(l))
		require.Len(t, s.Entities(), 1)

		nl, ok := s.Trim(l, 9, 0)
		require.False(t, ok)
		require.Nil(t, nl)
		require.Len(t, s.Entities(), 1, "the removed line is not resurrected")
		require.Empty(t, s.Constraints())
	})

	t.Run("Break", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
		require.True(t, s.RemoveEntity(l))
		require.Empty(t, s.Entities())

		e1, e2, ok := s.Break(l, 5, 0)
		require.False(t, ok)
		require.Nil(t, e1)
		require.Nil(t, e2)
		require.Empty(t, s.Entities(), "the removed line is not resurrected")
		require.Empty(t, s.Constraints())
	})

	t.Run("BreakArc", func(t *testing.T) {
		// The arc is the case where the constraint assertion is the one that
		// bites: breaking it manufactures two halves, each auto-carrying an
		// internal arc-radius constraint, so an unguarded Break on the dead
		// handle leaves constraints behind as well as entities.
		s := newSketch(t)
		a := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))
		require.True(t, s.RemoveEntity(a))
		require.Empty(t, s.Entities())
		require.Empty(t, s.Constraints())

		e1, e2, ok := s.Break(a, 4, 4) // pick near the 45° point
		require.False(t, ok)
		require.Nil(t, e1)
		require.Nil(t, e2)
		require.Empty(t, s.Entities(), "the removed arc is not resurrected")
		require.Empty(t, s.Constraints(), "no internal arc-radius constraints manufactured")
	})
}

// Negative controls: one per refusal style, proving the guard admits ordinary
// same-sketch input rather than refusing everything.
func TestToolsAcceptOwnedEntity(t *testing.T) {
	t.Run("BreakStillSplits", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))

		e1, e2, ok := s.Break(l, 4, 0)
		require.True(t, ok)
		require.NotNil(t, e1)
		require.NotNil(t, e2)
		require.Len(t, s.Entities(), 2)
	})

	t.Run("TrimStillTrims", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
		s.CreateLine(s.CreatePoint(5, -5), s.CreatePoint(5, 5))

		nl, ok := s.Trim(l, 9, 0)
		require.True(t, ok)
		require.InDelta(t, 5, nl.End.X(), 1e-9)
	})

	t.Run("ExtendStillExtends", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
		s.CreateLine(s.CreatePoint(8, -5), s.CreatePoint(8, 5))

		nl, ok := s.Extend(l, l.End)
		require.True(t, ok)
		require.InDelta(t, 8, nl.End.X(), 1e-9)
	})

	t.Run("CreateMirrorStillMirrors", func(t *testing.T) {
		s := newSketch(t)
		axis := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
		l := s.CreateLine(s.CreatePoint(2, 0), s.CreatePoint(6, 0))

		m := s.CreateMirror([]sketch.Entity{l}, axis)
		require.NotNil(t, m)
		require.Len(t, m.Copies, 1)
	})

	t.Run("CreatePatternRectStillPatterns", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))

		p, err := s.CreatePatternRect([]sketch.Entity{l}, 2, 1, 10, 0)
		require.NoError(t, err)
		require.Len(t, p.Instances, 1)
	})

	t.Run("CreateFilletStillRounds", func(t *testing.T) {
		s := newSketch(t)
		corner := s.CreatePoint(0, 0)
		l1 := s.CreateLine(corner, s.CreatePoint(10, 0))
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		f, err := s.CreateFillet(l1, l2, 2)
		require.NoError(t, err)
		require.NotNil(t, f.Arc)
	})
}

// An entity this sketch owns can still be defined by another sketch's point:
// the entity fields (Line.Start, …) are exported, and the positional ownership
// test over s.ents never looks at them. The tools read those points and build
// from them, so the same guard must screen the defining points too — refusing
// through each signature's existing channel.
func TestToolsRejectForeignDefiningPoint(t *testing.T) {
	t.Run("Break", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 10, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(10, 0))

		e1, e2, ok := s.Break(l, 5, 0)
		require.False(t, ok)
		require.Nil(t, e1)
		require.Nil(t, e2)
		require.Len(t, s.Entities(), 1, "only l, nothing manufactured")
		require.Empty(t, s.Constraints())
	})

	t.Run("Trim", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 10, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(10, 0))
		s.CreateLine(s.CreatePoint(5, -5), s.CreatePoint(5, 5)) // cutter

		nl, ok := s.Trim(l, 9, 0)
		require.False(t, ok)
		require.Nil(t, nl)
		require.Len(t, s.Entities(), 2, "l and the cutter")
	})

	t.Run("Extend", func(t *testing.T) {
		// end is a LOCAL endpoint, so the pre-existing !s.owns(end) screen passes
		// it; the foreign point is the OTHER end, which only the defining-point
		// scan sees. Without it Extend splices that point into the replacement.
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 10, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(4, 0))
		s.CreateLine(s.CreatePoint(8, -5), s.CreatePoint(8, 5)) // cutter beyond the End end

		nl, ok := s.Extend(l, l.End)
		require.False(t, ok)
		require.Nil(t, nl)
		require.Len(t, s.Entities(), 2, "l and the cutter")
	})

	t.Run("CreateFillet", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 10, 0, 10, 10)
		corner := s.CreatePoint(0, 0)
		l1 := s.CreateLine(corner, fl.Start) // foreign FAR endpoint
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		f, err := s.CreateFillet(l1, l2, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, f)
		require.Len(t, s.Entities(), 2, "both legs survive untouched")
		require.Empty(t, s.Constraints())
	})

	t.Run("CreateChamfer", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 10, 0, 10, 10)
		corner := s.CreatePoint(0, 0)
		l1 := s.CreateLine(corner, fl.Start)
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		c, err := s.CreateChamfer(l1, l2, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, c)
		require.Len(t, s.Entities(), 2)
		require.Empty(t, s.Constraints())
	})

	t.Run("CreateMirror", func(t *testing.T) {
		s := newSketch(t)
		axis := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
		fl := foreignLine(t, 2, 0, 6, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(6, 0))

		require.Nil(t, s.CreateMirror([]sketch.Entity{l}, axis))
		require.Len(t, s.Entities(), 2, "the axis and l, no copies")
		require.Empty(t, s.Constraints(), "no symmetric constraints on a foreign point")
	})

	t.Run("CreateMirrorForeignAxisPoint", func(t *testing.T) {
		// The axis was screened by ownsEntity alone, so an axis this sketch owns
		// but whose endpoint is foreign passed. CreateMirror reads the axis
		// geometry through that point and reflects every source across it.
		s := newSketch(t)
		fl := foreignLine(t, 0, -5, 0, 5)
		axis := s.CreateLine(fl.Start, s.CreatePoint(0, 5))
		l := s.CreateLine(s.CreatePoint(2, 0), s.CreatePoint(6, 0))

		require.Nil(t, s.CreateMirror([]sketch.Entity{l}, axis))
		require.Len(t, s.Entities(), 2, "the axis and l, no copies")
		require.Empty(t, s.Constraints())
	})

	t.Run("CreatePatternRect", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 4, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(4, 0))

		p, err := s.CreatePatternRect([]sketch.Entity{l}, 2, 1, 10, 0)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, p)
		require.Len(t, s.Entities(), 1)
		require.Empty(t, s.Constraints())
	})

	t.Run("CreatePatternCircular", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 4, 0, 6, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(6, 0))

		p, err := s.CreatePatternCircular([]sketch.Entity{l}, s.CreatePoint(0, 0), 4)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, p)
		require.Len(t, s.Entities(), 1, "no spokes built from a foreign point")
		require.Empty(t, s.Constraints())
	})

	t.Run("CreateOffset", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 10, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(10, 0))

		g, err := s.CreateOffset([]sketch.Entity{l}, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, g)
		require.Len(t, s.Entities(), 1)
		require.Empty(t, s.Constraints())
	})
}

// The three laundering cases. Trim when the trimmed-away side holds the foreign
// endpoint, and CreateFillet / CreateChamfer when the foreign point IS the
// shared corner, all replace that point with fresh local points and retire the
// originals — so the foreign handle stops being reachable and the finding
// disappears. Measured before the fix: Verify went from reporting
// ErrForeignHandle to reporting none, and MarshalJSON from refusing to
// succeeding, leaving geometry computed from another sketch's coordinates with
// nothing anywhere to flag it. Each subtest asserts the finding survives the
// call.
func TestToolsDoNotLaunderForeignPoint(t *testing.T) {
	t.Run("Trim", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 10, 0)
		l := s.CreateLine(fl.Start, s.CreatePoint(10, 0))
		s.CreateLine(s.CreatePoint(5, -5), s.CreatePoint(5, 5)) // cutter

		require.True(t, s.Verify(t.Context()).ForeignHandles, "the foreign endpoint is visible up front")

		// Pick near the Start end, so the surviving portion is (5,0)-(10,0) and
		// the foreign endpoint is what gets trimmed away.
		nl, ok := s.Trim(l, 1, 0)
		require.False(t, ok, "a line with a foreign endpoint is not trimmable")
		require.Nil(t, nl)

		rep := s.Verify(t.Context())
		require.True(t, rep.ForeignHandles, "the foreign handle is still reachable")
		require.ErrorIs(t, rep.Check(), sketch.ErrForeignHandle)
		require.False(t, rep.Trustworthy())

		_, err := s.MarshalJSON()
		require.ErrorIs(t, err, sketch.ErrForeignHandle, "still unwritable")
	})

	t.Run("CreateFillet", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 0, 1)
		corner := fl.Start // the shared corner is another sketch's point
		l1 := s.CreateLine(corner, s.CreatePoint(10, 0))
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		require.True(t, s.Verify(t.Context()).ForeignHandles)

		f, err := s.CreateFillet(l1, l2, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, f)

		rep := s.Verify(t.Context())
		require.True(t, rep.ForeignHandles, "the foreign corner is still reachable")
		require.ErrorIs(t, rep.Check(), sketch.ErrForeignHandle)
		require.False(t, rep.Trustworthy())

		_, err = s.MarshalJSON()
		require.ErrorIs(t, err, sketch.ErrForeignHandle, "still unwritable")
	})

	t.Run("CreateChamfer", func(t *testing.T) {
		s := newSketch(t)
		fl := foreignLine(t, 0, 0, 0, 1)
		corner := fl.Start
		l1 := s.CreateLine(corner, s.CreatePoint(10, 0))
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		require.True(t, s.Verify(t.Context()).ForeignHandles)

		c, err := s.CreateChamfer(l1, l2, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, c)

		rep := s.Verify(t.Context())
		require.True(t, rep.ForeignHandles, "the foreign corner is still reachable")
		require.ErrorIs(t, rep.Check(), sketch.ErrForeignHandle)
		require.False(t, rep.Trustworthy())

		_, err = s.MarshalJSON()
		require.ErrorIs(t, err, sketch.ErrForeignHandle, "still unwritable")
	})
}

// The origin is a live point of its sketch that is deliberately absent from
// s.points, so a positional ownership test alone would call it foreign and the
// widened guard would refuse the anchoring the engine's own authoring guidance
// asks for. s.owns carries the exception; these pin that it reaches the tools.
func TestToolsAcceptOriginDefiningPoint(t *testing.T) {
	t.Run("TrimStillTrims", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.Origin(), s.CreatePoint(10, 0))
		s.CreateLine(s.CreatePoint(5, -5), s.CreatePoint(5, 5))

		nl, ok := s.Trim(l, 9, 0)
		require.True(t, ok)
		require.InDelta(t, 5, nl.End.X(), 1e-9)
		require.False(t, s.Verify(t.Context()).ForeignHandles)
	})

	t.Run("CreateFilletStillRounds", func(t *testing.T) {
		s := newSketch(t)
		l1 := s.CreateLine(s.Origin(), s.CreatePoint(10, 0))
		l2 := s.CreateLine(s.Origin(), s.CreatePoint(0, 10))

		f, err := s.CreateFillet(l1, l2, 2)
		require.NoError(t, err)
		require.NotNil(t, f.Arc)
		require.False(t, s.Verify(t.Context()).ForeignHandles)
	})

	t.Run("CreateChamferStillCuts", func(t *testing.T) {
		s := newSketch(t)
		l1 := s.CreateLine(s.Origin(), s.CreatePoint(10, 0))
		l2 := s.CreateLine(s.Origin(), s.CreatePoint(0, 10))

		c, err := s.CreateChamfer(l1, l2, 2)
		require.NoError(t, err)
		require.NotNil(t, c.Cut)
		require.False(t, s.Verify(t.Context()).ForeignHandles)
	})

	t.Run("BreakStillSplits", func(t *testing.T) {
		s := newSketch(t)
		l := s.CreateLine(s.Origin(), s.CreatePoint(10, 0))

		e1, e2, ok := s.Break(l, 4, 0)
		require.True(t, ok)
		require.NotNil(t, e1)
		require.NotNil(t, e2)
		require.False(t, s.Verify(t.Context()).ForeignHandles)
	})

	t.Run("CreateMirrorStillMirrors", func(t *testing.T) {
		s := newSketch(t)
		axis := s.CreateLine(s.Origin(), s.CreatePoint(0, 5))
		l := s.CreateLine(s.CreatePoint(2, 0), s.CreatePoint(6, 0))

		m := s.CreateMirror([]sketch.Entity{l}, axis)
		require.NotNil(t, m)
		require.Len(t, m.Copies, 1)
		require.False(t, s.Verify(t.Context()).ForeignHandles)
	})
}
