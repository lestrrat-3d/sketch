package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// unsupportedSeeds builds one entity of every kind the pattern/mirror copy
// funnel (Sketch.instantiate) cannot reproduce, in a fresh sketch each time.
// A new entity type is caught by TestEntityTypeSwitchesAreExhaustive bringing
// the author to unsupportedSeed, and its author should add a row here.
var unsupportedSeeds = []struct {
	name  string
	build func(t *testing.T, s *sketch.Sketch) sketch.Entity
}{
	{
		name: "Ellipse",
		build: func(t *testing.T, s *sketch.Sketch) sketch.Entity {
			t.Helper()
			return s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0.3)
		},
	},
	{
		name: "EllipticalArc",
		build: func(t *testing.T, s *sketch.Sketch) sketch.Entity {
			t.Helper()
			return s.CreateEllipticalArc(s.CreatePoint(0, 0), s.CreatePoint(4, 0), s.CreatePoint(0, 2), 4, 2, 0)
		},
	},
	{
		name: "Conic",
		build: func(t *testing.T, s *sketch.Sketch) sketch.Entity {
			t.Helper()
			c, err := s.CreateConic(s.CreatePoint(0, 0), s.CreatePoint(5, 5), s.CreatePoint(10, 0), 0.4)
			require.NoError(t, err)
			return c
		},
	},
	{
		name: "Spline",
		build: func(t *testing.T, s *sketch.Sketch) sketch.Entity {
			t.Helper()
			pts := make([]*sketch.Point, 5)
			for i := range pts {
				u := float64(i) / 4
				pts[i] = s.CreatePoint(u*10, math.Sin(u*math.Pi)*3)
			}
			sp, err := s.CreateSpline(pts...)
			require.NoError(t, err)
			return sp
		},
	},
	{
		name: "ClosedSpline",
		build: func(t *testing.T, s *sketch.Sketch) sketch.Entity {
			t.Helper()
			cs, err := s.CreateClosedSpline(s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(5, 8))
			require.NoError(t, err)
			return cs
		},
	},
	{
		name: "FitSpline",
		build: func(t *testing.T, s *sketch.Sketch) sketch.Entity {
			t.Helper()
			pts := make([]*sketch.Point, 5)
			for i := range pts {
				u := float64(i) / 4
				pts[i] = s.CreatePoint(u*10, math.Sin(u*math.Pi)*3)
			}
			fs, err := s.CreateFitSpline(pts...)
			require.NoError(t, err)
			return fs
		},
	},
	{
		name: "NURBS",
		build: func(t *testing.T, s *sketch.Sketch) sketch.Entity {
			t.Helper()
			ctrl := []*sketch.Point{
				s.CreatePoint(0, 0),
				s.CreatePoint(3, 5),
				s.CreatePoint(7, -5),
				s.CreatePoint(10, 0),
			}
			n, err := s.CreateNURBS(3, ctrl, []float64{1, 1, 1, 1}, []float64{0, 0, 0, 0, 1, 1, 1, 1})
			require.NoError(t, err)
			return n
		},
	},
}

// TestPatternMirrorRefuseUnsupportedSeed asserts that every entity kind the
// copy funnel cannot reproduce is refused (rather than silently dropped) by
// all three builders, and that the refusal names the offending kind.
func TestPatternMirrorRefuseUnsupportedSeed(t *testing.T) {
	for _, tc := range unsupportedSeeds {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("rect", func(t *testing.T) {
				s := newSketch(t)
				seed := []sketch.Entity{tc.build(t, s)}

				p, err := s.CreatePatternRect(seed, 2, 1, 30, 0)
				require.Nil(t, p)
				require.ErrorIs(t, err, sketch.ErrUnsupportedEntity)
				require.ErrorContains(t, err, "*sketch."+tc.name)
				require.ErrorContains(t, err, "CreatePatternRect")
			})

			t.Run("circular", func(t *testing.T) {
				s := newSketch(t)
				seed := []sketch.Entity{tc.build(t, s)}
				center := s.CreatePoint(-20, 0)

				p, err := s.CreatePatternCircular(seed, center, 4)
				require.Nil(t, p)
				require.ErrorIs(t, err, sketch.ErrUnsupportedEntity)
				require.ErrorContains(t, err, "*sketch."+tc.name)
				require.ErrorContains(t, err, "CreatePatternCircular")
			})

			t.Run("mirror", func(t *testing.T) {
				s := newSketch(t)
				seed := []sketch.Entity{tc.build(t, s)}
				axis := s.CreateLine(s.CreatePoint(-20, -50), s.CreatePoint(-20, 50))

				require.Nil(t, s.CreateMirror(seed, axis))
			})
		})
	}
}

// TestPatternMirrorRefusalLeavesSketchUnchanged asserts that a refused
// pattern/mirror call commits nothing: not a point, not an entity, not a
// constraint, and the state fingerprint (Revision) is untouched.
func TestPatternMirrorRefusalLeavesSketchUnchanged(t *testing.T) {
	for _, tc := range unsupportedSeeds {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("rect", func(t *testing.T) {
				s := newSketch(t)
				seed := []sketch.Entity{tc.build(t, s)}

				pts, ents, cons, dof, rev := len(s.Points()), len(s.Entities()), len(s.Constraints()), s.DOF(), s.Revision()
				_, err := s.CreatePatternRect(seed, 2, 1, 30, 0)
				require.Error(t, err)
				require.Len(t, s.Points(), pts, "a refused pattern commits no point")
				require.Len(t, s.Entities(), ents, "a refused pattern commits no entity")
				require.Len(t, s.Constraints(), cons, "a refused pattern commits no constraint")
				require.Equal(t, dof, s.DOF(), "a refused pattern allocates no variable")
				require.Equal(t, rev, s.Revision(), "a refused pattern leaves the state fingerprint untouched")
			})

			t.Run("circular", func(t *testing.T) {
				s := newSketch(t)
				seed := []sketch.Entity{tc.build(t, s)}
				center := s.CreatePoint(-20, 0)

				pts, ents, cons, dof, rev := len(s.Points()), len(s.Entities()), len(s.Constraints()), s.DOF(), s.Revision()
				_, err := s.CreatePatternCircular(seed, center, 4)
				require.Error(t, err)
				require.Len(t, s.Points(), pts, "a refused pattern commits no point")
				require.Len(t, s.Entities(), ents, "a refused pattern commits no entity")
				require.Len(t, s.Constraints(), cons, "a refused pattern commits no constraint")
				require.Equal(t, dof, s.DOF(), "a refused pattern allocates no variable")
				require.Equal(t, rev, s.Revision(), "a refused pattern leaves the state fingerprint untouched")
			})

			t.Run("mirror", func(t *testing.T) {
				s := newSketch(t)
				seed := []sketch.Entity{tc.build(t, s)}
				axis := s.CreateLine(s.CreatePoint(-20, -50), s.CreatePoint(-20, 50))

				pts, ents, cons, dof, rev := len(s.Points()), len(s.Entities()), len(s.Constraints()), s.DOF(), s.Revision()
				require.Nil(t, s.CreateMirror(seed, axis))
				require.Len(t, s.Points(), pts, "a refused mirror commits no point")
				require.Len(t, s.Entities(), ents, "a refused mirror commits no entity")
				require.Len(t, s.Constraints(), cons, "a refused mirror commits no constraint")
				require.Equal(t, dof, s.DOF(), "a refused mirror allocates no variable")
				require.Equal(t, rev, s.Revision(), "a refused mirror leaves the state fingerprint untouched")
			})
		})
	}
}

// TestPatternMirrorMixedSeedRefused is the gear-generator's exact case: a
// FitSpline flank plus the Line that closes it. Before this refusal existed,
// CreatePatternCircular returned a nil error and a Pattern holding only the
// copyable line — the spline silently dropped, with no signal that the
// resulting geometry was not what was asked for.
func TestPatternMirrorMixedSeedRefused(t *testing.T) {
	s := newSketch(t)
	p0 := s.CreatePoint(0, 0)
	p1 := s.CreatePoint(1, 3)
	p2 := s.CreatePoint(2, 5)
	fs, err := s.CreateFitSpline(p0, p1, p2)
	require.NoError(t, err)
	closingLine := s.CreateLine(p2, p0)
	seed := []sketch.Entity{fs, closingLine}
	center := s.CreatePoint(-20, 0)

	pts, ents, cons, dof, rev := len(s.Points()), len(s.Entities()), len(s.Constraints()), s.DOF(), s.Revision()

	p, err := s.CreatePatternCircular(seed, center, 6)
	require.Nil(t, p)
	require.ErrorIs(t, err, sketch.ErrUnsupportedEntity)

	// The refusal is all-or-nothing: the copyable line must not be copied
	// either, which is what makes "the sketch is unchanged" true.
	require.Len(t, s.Points(), pts, "the copyable line is not copied either")
	require.Len(t, s.Entities(), ents)
	require.Len(t, s.Constraints(), cons)
	require.Equal(t, dof, s.DOF())
	require.Equal(t, rev, s.Revision())
}

// TestMirrorArcSwapsEnds covers the arc-reversal branch of instantiate
// (swapArc), which no existing mirror test exercises.
func TestMirrorArcSwapsEnds(t *testing.T) {
	s := newSketch(t)
	axis := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5)) // the y axis
	a := s.CreateArc(s.CreatePoint(3, 0), s.CreatePoint(5, 0), s.CreatePoint(3, 2))

	m := s.CreateMirror([]sketch.Entity{a}, axis)
	require.NotNil(t, m)
	require.Len(t, m.Copies, 1)
	cp := m.Copies[0].(*sketch.Arc)
	require.InDelta(t, -3, cp.Center.X(), 1e-9)
	// Reflection reverses sweep, so the copy is committed start/end-swapped:
	// the copy's Start is the mirror of the source's End.
	require.InDelta(t, -3, cp.Start.X(), 1e-9)
	require.InDelta(t, 2, cp.Start.Y(), 1e-9)
	require.InDelta(t, -5, cp.End.X(), 1e-9)
	require.InDelta(t, 0, cp.End.Y(), 1e-9)
}

// TestPatternMirrorCopiesInheritConstruction covers the construction-flag
// inheritance in instantiate (per entity) and each builder's link closure
// (per point), which no existing test exercises.
func TestPatternMirrorCopiesInheritConstruction(t *testing.T) {
	buildSeed := func(t *testing.T, s *sketch.Sketch) *sketch.Line {
		t.Helper()
		p1, p2 := s.CreatePoint(0, 0), s.CreatePoint(1, 0)
		p1.SetConstruction(true)
		p2.SetConstruction(true)
		seed := s.CreateLine(p1, p2)
		seed.SetConstruction(true)
		return seed
	}

	t.Run("rect", func(t *testing.T) {
		s := newSketch(t)
		seed := buildSeed(t, s)

		p, err := s.CreatePatternRect([]sketch.Entity{seed}, 2, 1, 30, 0)
		require.NoError(t, err)
		require.Len(t, p.Instances, 1)
		cp := p.Instances[0].(*sketch.Line)
		require.True(t, cp.IsConstruction(), "the copy inherits the seed's construction flag")
		require.True(t, cp.Start.IsConstruction(), "the copy's points inherit too")
		require.True(t, cp.End.IsConstruction())
	})

	t.Run("circular", func(t *testing.T) {
		s := newSketch(t)
		seed := buildSeed(t, s)
		center := s.CreatePoint(-20, 0)

		p, err := s.CreatePatternCircular([]sketch.Entity{seed}, center, 2)
		require.NoError(t, err)
		require.Len(t, p.Instances, 1)
		cp := p.Instances[0].(*sketch.Line)
		require.True(t, cp.IsConstruction(), "the copy inherits the seed's construction flag")
		require.True(t, cp.Start.IsConstruction(), "the copy's points inherit too")
		require.True(t, cp.End.IsConstruction())
	})

	t.Run("mirror", func(t *testing.T) {
		s := newSketch(t)
		seed := buildSeed(t, s)
		axis := s.CreateLine(s.CreatePoint(-20, -50), s.CreatePoint(-20, 50))

		m := s.CreateMirror([]sketch.Entity{seed}, axis)
		require.NotNil(t, m)
		require.Len(t, m.Copies, 1)
		cp := m.Copies[0].(*sketch.Line)
		require.True(t, cp.IsConstruction(), "the copy inherits the seed's construction flag")
		require.True(t, cp.Start.IsConstruction(), "the copy's points inherit too")
		require.True(t, cp.End.IsConstruction())
	})
}
