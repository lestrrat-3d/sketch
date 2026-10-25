package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A circle offsets to a concentric copy sharing its center; positive is outward,
// as in Fusion, and Set retargets the copy.
func TestOffsetCircle(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	s.Fix(center)
	src := s.CreateCircle(center, 10)
	s.AddConstraint(sketch.NewRadius(src, 10))

	g, err := sketch.CreateOffset(s, []sketch.Entity{src}, 2)
	require.NoError(t, err)
	dst := g.Copies[0].(*sketch.Circle)
	require.Same(t, center, dst.Center)
	require.Len(t, g.Radials, 1)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF())
	require.InDelta(t, 12, dst.R(), 1e-6)

	g.Set(-3)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, dst.R(), 1e-6)
}

// A lone arc offsets to a concentric arc whose ends stay on the source normals.
func TestOffsetLoneArc(t *testing.T) {
	s := newSketch(t)
	center, start, end := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(0, 10)
	for _, p := range []*sketch.Point{center, start, end} {
		s.Fix(p)
	}
	src := s.CreateArc(center, start, end)

	g, err := sketch.CreateOffset(s, []sketch.Entity{src}, 2)
	require.NoError(t, err)
	dst := g.Copies[0].(*sketch.Arc)
	require.Same(t, center, dst.Center)
	require.Len(t, g.Constraints, 2, "both ends are held on the source normals")
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF())
	require.InDelta(t, 12, dst.Start.X(), 1e-6)
	require.InDelta(t, 0, dst.Start.Y(), 1e-6)
	require.InDelta(t, 0, dst.End.X(), 1e-6)
	require.InDelta(t, 12, dst.End.Y(), 1e-6)

	g.Set(1)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 11, dst.R(), 1e-6)
	require.InDelta(t, 11, dst.Start.X(), 1e-6)
}

// Refusals leave the sketch unchanged.
func TestOffsetCurveRefusals(t *testing.T) {
	cases := []struct {
		name  string
		build func(s *sketch.Sketch) []sketch.Entity
		want  error
	}{
		{"ellipse", func(s *sketch.Sketch) []sketch.Entity {
			return []sketch.Entity{s.CreateEllipse(s.CreatePoint(0, 0), 5, 3, 0)}
		}, sketch.ErrUnsupportedEntity},
		{"radius would vanish", func(s *sketch.Sketch) []sketch.Entity {
			return []sketch.Entity{s.CreateCircle(s.CreatePoint(0, 0), 2)}
		}, sketch.ErrInvalidShape},
		{"joined arc", func(s *sketch.Sketch) []sketch.Entity {
			p := s.CreatePoint(10, 0)
			arc := s.CreateArc(s.CreatePoint(0, 0), p, s.CreatePoint(0, 10))
			return []sketch.Entity{s.CreateLine(s.CreatePoint(20, 0), p), arc}
		}, sketch.ErrInvalidShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			ents := tc.build(s)
			pts, n, cons := len(s.Points()), len(s.Entities()), len(s.Constraints())

			_, err := sketch.CreateOffset(s, ents, -3)
			require.ErrorIs(t, err, tc.want)
			require.Len(t, s.Points(), pts)
			require.Len(t, s.Entities(), n)
			require.Len(t, s.Constraints(), cons)
		})
	}
}
