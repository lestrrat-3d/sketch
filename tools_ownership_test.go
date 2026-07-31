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
		// Defensive: a foreign point can never be l.Start or l.End of a local
		// line, so the endpoint half of the guard has no reachable failure to
		// distinguish. It pins the contract rather than a live defect.
		s := newSketch(t)
		l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
		fl := foreignLine(t, 0, 0, 4, 0)

		nl, ok := s.Extend(l, fl.End)
		require.False(t, ok)
		require.Nil(t, nl)
		require.Len(t, s.Entities(), 1)
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
