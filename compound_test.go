package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestAddRectangle(t *testing.T) {
	s := sketch.New()
	r := s.AddRectangle(0, 0, 18, 2) // rough initial size
	s.Fix(r.A)
	addDist(s, r.A, r.B, 20)
	addDist(s, r.A, r.D, 12)

	res := mustSolve(t, s)
	require.Equal(t, 0, res.DOF, "fully constrained")
	require.InDelta(t, 20, r.C.X(), 1e-6, "c.X")
	require.InDelta(t, 12, r.C.Y(), 1e-6, "c.Y")
	require.InDelta(t, 0, r.D.X(), 1e-6, "d.X")
	require.InDelta(t, 12, r.D.Y(), 1e-6, "d.Y")
}

func TestAddPolygon(t *testing.T) {
	s := sketch.New()
	p := s.AddPolygon(0, 0, 6, 5)
	s.Fix(p.Center)
	s.Fix(p.Vertices[0]) // at (5, 0): pins rotation and size

	// Knock a far vertex off the circle so the solve does real work.
	p.Vertices[3].MoveTo(-4.5, 0.8)

	res := mustSolve(t, s)
	require.Equal(t, 0, res.DOF, "fully constrained")
	for i, v := range p.Vertices {
		require.InDeltaf(t, 5, pointDist(p.Center, v), 1e-6, "vertex %d circumradius", i)
	}
	for i, side := range p.Sides {
		require.InDeltaf(t, 5, side.Length(), 1e-6, "side %d (hexagon side == r)", i)
	}
}

func TestAddPolygonPanics(t *testing.T) {
	s := sketch.New()
	require.Panics(t, func() { s.AddPolygon(0, 0, 2, 5) }, "n < 3")
}

func TestAddSlot(t *testing.T) {
	s := sketch.New()
	sl := s.AddSlot(0, 0, 10, 0, 2) // built at radius 2, driven to 3 below
	s.Fix(sl.C1)
	s.Fix(sl.C2)
	require.Equal(t, 1, s.DOF(), "only the radius is free (contact points pinned)")
	addDist(s, sl.L1.Start, sl.C1, 3) // cap contact point to center == radius

	res := mustSolve(t, s)
	require.Equal(t, 0, res.DOF, "fully constrained")
	require.InDelta(t, 3, sl.A1.R(), 1e-6, "cap 1 radius")
	require.InDelta(t, 3, sl.A2.R(), 1e-6, "cap 2 radius (equal)")
	require.InDelta(t, -3, sl.L1.Start.Y(), 1e-6, "right flank start")
	require.InDelta(t, -3, sl.L1.End.Y(), 1e-6, "right flank end")
	require.InDelta(t, 3, sl.L2.Start.Y(), 1e-6, "left flank start")
	require.InDelta(t, 3, sl.L2.End.Y(), 1e-6, "left flank end")
}

func TestJSONRoundTripSlot(t *testing.T) {
	s := sketch.New()
	sl := s.AddSlot(0, 0, 10, 0, 3)
	s.Fix(sl.C1)
	s.Fix(sl.C2)
	addDist(s, sl.L1.Start, sl.C1, 3)
	mustSolve(t, s)

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	require.Len(t, s2.Entities(), len(s.Entities()), "entities survive")
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive (internal ones recreated, not doubled)")

	res := mustSolve(t, &s2)
	require.Equal(t, 0, res.DOF, "reloaded DOF")
}
