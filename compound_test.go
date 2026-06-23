package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestAddRectangle(t *testing.T) {
	s := newSketch(t)
	r := s.AddRectangle(0, 0, 18, 2) // rough initial size
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, 20))
	s.AddConstraint(sketch.NewDistance(r.A, r.D, 12))

	res, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "fully constrained")
	require.InDelta(t, 20, r.C.X(), 1e-6, "c.X")
	require.InDelta(t, 12, r.C.Y(), 1e-6, "c.Y")
	require.InDelta(t, 0, r.D.X(), 1e-6, "d.X")
	require.InDelta(t, 12, r.D.Y(), 1e-6, "d.Y")
}

func TestAddPolygon(t *testing.T) {
	s := newSketch(t)
	p, err := s.AddPolygon(0, 0, 6, 5)
	require.NoError(t, err)
	s.Fix(p.Center)
	s.Fix(p.Vertices[0]) // at (5, 0): pins rotation and size

	// Knock a far vertex off the circle so the solve does real work.
	p.Vertices[3].MoveTo(-4.5, 0.8)

	res, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "fully constrained")
	for i, v := range p.Vertices {
		require.InDeltaf(t, 5, math.Hypot(p.Center.X()-v.X(), p.Center.Y()-v.Y()), 1e-6, "vertex %d circumradius", i)
	}
	for i, side := range p.Sides {
		require.InDeltaf(t, 5, side.Length(), 1e-6, "side %d (hexagon side == r)", i)
	}
}

func TestAddPolygonInvalid(t *testing.T) {
	s := newSketch(t)
	_, err := s.AddPolygon(0, 0, 2, 5)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "n < 3")
}

func TestAddSlot(t *testing.T) {
	s := newSketch(t)
	sl, err := s.AddSlot(0, 0, 10, 0, 2) // built at radius 2, driven to 3 below
	require.NoError(t, err)
	s.Fix(sl.C1)
	s.Fix(sl.C2)
	require.Equal(t, 1, s.DOF(), "only the radius is free (contact points pinned)")
	s.AddConstraint(sketch.NewDistance(sl.L1.Start, sl.C1, 3)) // cap contact point to center == radius

	res, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "fully constrained")
	require.InDelta(t, 3, sl.A1.R(), 1e-6, "cap 1 radius")
	require.InDelta(t, 3, sl.A2.R(), 1e-6, "cap 2 radius (equal)")
	require.InDelta(t, -3, sl.L1.Start.Y(), 1e-6, "right flank start")
	require.InDelta(t, -3, sl.L1.End.Y(), 1e-6, "right flank end")
	require.InDelta(t, 3, sl.L2.Start.Y(), 1e-6, "left flank start")
	require.InDelta(t, 3, sl.L2.End.Y(), 1e-6, "left flank end")
}

func TestJSONRoundTripSlot(t *testing.T) {
	s := newSketch(t)
	sl, err := s.AddSlot(0, 0, 10, 0, 3)
	require.NoError(t, err)
	s.Fix(sl.C1)
	s.Fix(sl.C2)
	s.AddConstraint(sketch.NewDistance(sl.L1.Start, sl.C1, 3))
	_, err = s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	require.Len(t, s2.Entities(), len(s.Entities()), "entities survive")
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive (internal ones recreated, not doubled)")

	res, err := s2.Solve()
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "reloaded DOF")
}
