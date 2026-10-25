package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Filleting the corner where a line meets an arc keeps both legs in place and
// rounds the corner with an arc tangent to both, at the closed-form position.
func TestCreateFilletLineArc(t *testing.T) {
	s := newSketch(t)
	center, corner, far := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(0, 10)
	arc := s.CreateArc(center, corner, far)
	lineFar := s.CreatePoint(20, 0)
	line := s.CreateLine(corner, lineFar)
	s.Fix(center)
	s.Fix(far)
	s.Fix(lineFar)
	s.AddConstraint(sketch.NewHorizontal(line))
	dof := s.DOF()

	f, err := sketch.CreateFillet(s, line, arc, corner, 2)
	require.NoError(t, err)
	require.Same(t, line, f.L1)
	require.Same(t, arc, f.L2, "the arc leg keeps its handle and type")
	require.Same(t, f.T1, line.Start)
	require.Same(t, f.T2, arc.Start)
	require.InDelta(t, math.Sqrt(140), f.Arc.Center.X(), 1e-9)
	require.InDelta(t, 2, f.Arc.Center.Y(), 1e-9)
	require.InDelta(t, math.Sqrt(140), f.T1.X(), 1e-9)
	require.InDelta(t, 5*math.Sqrt(140)/6, f.T2.X(), 1e-9)
	require.InDelta(t, 5.0/3, f.T2.Y(), 1e-9)
	require.Equal(t, 1, countKind(s, "tangent_line_circle"))
	require.Equal(t, 1, countKind(s, "tangent_circles"))
	require.Equal(t, 1, countKind(s, "horizontal"), "the constraint on the line leg survives")
	require.Equal(t, dof, s.DOF(), "the fillet adds as many rows as variables")

	f.Radius.Set(3)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 3, f.Arc.R(), 1e-6)
	require.InDelta(t, math.Sqrt(160), f.Arc.Center.X(), 1e-6, "13 from the arc center, 3 above the line")
	require.InDelta(t, 3, f.Arc.Center.Y(), 1e-6)
	for _, c := range s.Constraints() {
		for _, r := range sketch.ConstraintResiduals(c) {
			require.InDelta(t, 0, r, 1e-9)
		}
	}
}

// Filleting two arcs places the fillet inside both circles.
func TestCreateFilletArcArc(t *testing.T) {
	s := newSketch(t)
	corner := s.CreatePoint(10, 0)
	a1 := s.CreateArc(s.CreatePoint(0, 0), corner, s.CreatePoint(0, 10))
	h := 10 - 5*math.Sqrt2
	a2 := s.CreateArc(s.CreatePoint(10, 10), s.CreatePoint(h, h), corner)

	f, err := sketch.CreateFillet(s, a1, a2, corner, 2)
	require.NoError(t, err)
	require.Same(t, a1, f.L1)
	require.Same(t, a2, f.L2)
	require.InDelta(t, 5+math.Sqrt(7), f.Arc.Center.X(), 1e-9)
	require.InDelta(t, 5-math.Sqrt(7), f.Arc.Center.Y(), 1e-9)
	require.Equal(t, 2, countKind(s, "tangent_circles"))
	require.NotContains(t, s.Points(), corner)
}

// Curves that meet tangentially have no corner to round.
func TestCreateFilletArcRefusals(t *testing.T) {
	s := newSketch(t)
	corner := s.CreatePoint(10, 0)
	arc := s.CreateArc(s.CreatePoint(0, 0), corner, s.CreatePoint(0, 10))
	tangent := s.CreateLine(s.CreatePoint(10, -10), corner) // continues the arc's tangent
	pts, cons := len(s.Points()), len(s.Constraints())

	_, err := sketch.CreateFillet(s, tangent, arc, corner, 2)
	require.ErrorIs(t, err, sketch.ErrFilletInfeasible)
	require.Len(t, s.Points(), pts)
	require.Len(t, s.Constraints(), cons)

	_, err = sketch.CreateFillet(s, tangent, arc, tangent.Start, 2)
	require.ErrorIs(t, err, sketch.ErrNoSharedCorner)
}
