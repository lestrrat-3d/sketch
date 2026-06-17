package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestArcLengthQuarter(t *testing.T) {
	s := sketch.New()
	c := s.AddPoint(0, 0)
	start := s.AddPoint(4, 0)
	s.Fix(c)
	s.Fix(start) // pins R=4 and the start ray (angle 0)
	end := s.AddPoint(0, 4)
	arc := s.AddArc(c, start, end)
	s.AddConstraint(sketch.NewArcLength(arc, 3*math.Pi)) // 3π / R(4) = 3π/4 sweep

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 4, arc.R(), 1e-6, "radius held by the fixed start")
	require.InDelta(t, 3*math.Pi/4, arc.Sweep(), 1e-6, "sweep = length / radius")
	require.InDelta(t, 3*math.Pi, arc.R()*arc.Sweep(), 1e-6, "swept length driven")
	require.InDelta(t, -2*math.Sqrt2, end.X(), 1e-6, "end at angle 3π/4")
	require.InDelta(t, 2*math.Sqrt2, end.Y(), 1e-6)
}

func TestArcLengthBranchBeyondPi(t *testing.T) {
	// Target sweep 3π/2 > π — the case a sin(Δ−theta) coupling would risk.
	s := sketch.New()
	c := s.AddPoint(0, 0)
	start := s.AddPoint(2, 0)
	s.Fix(c)
	s.Fix(start)            // R=2
	end := s.AddPoint(0, 2) // sweep π/2 initially
	arc := s.AddArc(c, start, end)
	s.AddConstraint(sketch.NewArcLength(arc, 3*math.Pi)) // 3π / 2 = 3π/2 sweep

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 3*math.Pi/2, arc.Sweep(), 1e-6, "swept past π to 3π/2")
	require.InDelta(t, 0, end.X(), 1e-6, "end at angle 3π/2")
	require.InDelta(t, -2, end.Y(), 1e-6)
}

func TestArcLengthRejectsWrongBranch(t *testing.T) {
	// All three points fixed at a sweep of π/2 (length π·R/2 = π at R=2), but the
	// dimension demands length 3π (sweep 3π/2 — the antipodal branch). With the
	// geometry frozen the dimension cannot be met; a coupling that vanished on the
	// wrong branch would falsely report this solvable.
	s := sketch.New()
	c := s.AddPoint(0, 0)
	start := s.AddPoint(2, 0)
	end := s.AddPoint(0, 2) // sweep π/2
	s.Fix(c)
	s.Fix(start)
	s.Fix(end)
	arc := s.AddArc(c, start, end)
	s.AddConstraint(sketch.NewArcLength(arc, 3*math.Pi)) // wants sweep 3π/2

	s.Solve()
	require.False(t, s.Verify().Solvable, "the frozen wrong-branch arc is not solvable")
}

func TestArcLengthSemicircle(t *testing.T) {
	// Target sweep exactly π — Δ sits on the atan2 ±π cut, which the mod-2π wrap
	// absorbs, so the dimension still solves cleanly.
	s := sketch.New()
	c := s.AddPoint(0, 0)
	start := s.AddPoint(3, 0)
	s.Fix(c)
	s.Fix(start)            // R=3
	end := s.AddPoint(0, 3) // sweep π/2 initially
	arc := s.AddArc(c, start, end)
	s.AddConstraint(sketch.NewArcLength(arc, 3*math.Pi)) // 3π / 3 = π sweep

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, math.Pi, arc.Sweep(), 1e-6, "swept to a half turn")
	require.InDelta(t, -3, end.X(), 1e-6, "end antipodal to start")
	require.InDelta(t, 0, end.Y(), 1e-6)
}

func TestArcLengthDOFNeutral(t *testing.T) {
	s := sketch.New()
	c := s.AddPoint(0, 0)
	start := s.AddPoint(4, 0)
	s.Fix(c)
	s.Fix(start)
	end := s.AddPoint(0, 4)
	arc := s.AddArc(c, start, end)
	require.Equal(t, 1, s.DOF(), "the free end has one angular DOF")

	dim := sketch.NewArcLength(arc, 3*math.Pi)
	s.AddConstraint(dim)
	require.Equal(t, 0, s.DOF(), "the dimension removes exactly one DOF")

	require.True(t, s.RemoveConstraint(dim), "the dimension was removed")
	require.Equal(t, 1, s.DOF(), "removal restores the DOF (aux var retired)")
}

func TestArcLengthRoundTrip(t *testing.T) {
	s := sketch.New()
	c := s.AddPoint(0, 0)
	start := s.AddPoint(4, 0)
	s.Fix(c)
	s.Fix(start)
	arc := s.AddArc(c, start, s.AddPoint(0, 4))
	s.AddConstraint(sketch.NewArcLength(arc, 3*math.Pi))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")

	_, err = s2.Solve()
	require.NoError(t, err)
	for i, p := range s.Points() {
		require.InDeltaf(t, p.X(), s2.Points()[i].X(), 1e-6, "point %d X", i)
		require.InDeltaf(t, p.Y(), s2.Points()[i].Y(), 1e-6, "point %d Y", i)
	}
}
