package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestArcLengthQuarter(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	s.Fix(c)
	s.Fix(start) // pins R=4 and the start ray (angle 0)
	end := s.CreatePoint(0, 4)
	arc := s.CreateArc(c, start, end)
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
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(2, 0)
	s.Fix(c)
	s.Fix(start)               // R=2
	end := s.CreatePoint(0, 2) // sweep π/2 initially
	arc := s.CreateArc(c, start, end)
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
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(2, 0)
	end := s.CreatePoint(0, 2) // sweep π/2
	s.Fix(c)
	s.Fix(start)
	s.Fix(end)
	arc := s.CreateArc(c, start, end)
	s.AddConstraint(sketch.NewArcLength(arc, 3*math.Pi)) // wants sweep 3π/2

	s.Solve()
	require.False(t, s.Verify().Solvable, "the frozen wrong-branch arc is not solvable")
}

func TestArcLengthSemicircle(t *testing.T) {
	// Target sweep exactly π — Δ sits on the atan2 ±π cut, which the mod-2π wrap
	// absorbs, so the dimension still solves cleanly.
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(3, 0)
	s.Fix(c)
	s.Fix(start)               // R=3
	end := s.CreatePoint(0, 3) // sweep π/2 initially
	arc := s.CreateArc(c, start, end)
	s.AddConstraint(sketch.NewArcLength(arc, 3*math.Pi)) // 3π / 3 = π sweep

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, math.Pi, arc.Sweep(), 1e-6, "swept to a half turn")
	require.InDelta(t, -3, end.X(), 1e-6, "end antipodal to start")
	require.InDelta(t, 0, end.Y(), 1e-6)
}

func TestArcLengthDOFNeutral(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	s.Fix(c)
	s.Fix(start)
	end := s.CreatePoint(0, 4)
	arc := s.CreateArc(c, start, end)
	require.Equal(t, 1, s.DOF(), "the free end has one angular DOF")

	dim := sketch.NewArcLength(arc, 3*math.Pi)
	s.AddConstraint(dim)
	require.Equal(t, 0, s.DOF(), "the dimension removes exactly one DOF")

	require.True(t, s.RemoveConstraint(dim), "the dimension was removed")
	require.Equal(t, 1, s.DOF(), "removal restores the DOF (aux var retired)")
}

func TestArcLengthDriven(t *testing.T) {
	// A driven (reference) arc-length measures the swept length R·Sweep() of a
	// fully determined quarter arc: R=4, sweep π/2, so length = 2π.
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	end := s.CreatePoint(0, 4)
	s.Fix(c)
	s.Fix(start)
	s.Fix(end)
	arc := s.CreateArc(c, start, end)
	dim := sketch.NewArcLength(arc, 0) // initial target irrelevant for a driven dim
	dim.SetDriven(true)
	s.AddConstraint(dim)

	_, err := s.Solve()
	require.NoError(t, err)
	require.True(t, dim.Driven())
	require.InDelta(t, 2*math.Pi, dim.Target().Mag(), 1e-6, "measures R·Sweep()")
}

func TestArcLengthDrivenDOFNeutralAndToggle(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	s.Fix(c)
	s.Fix(start)
	end := s.CreatePoint(0, 4)
	arc := s.CreateArc(c, start, end)
	require.Equal(t, 1, s.DOF(), "the free end has one angular DOF")

	dim := sketch.NewArcLength(arc, 3*math.Pi)
	s.AddConstraint(dim)
	require.Equal(t, 0, s.DOF(), "driving removes one DOF")

	dim.SetDriven(true)
	require.Equal(t, 1, s.DOF(), "driven measures only — the aux var is retired, DOF restored")

	dim.SetDriven(false)
	require.Equal(t, 0, s.DOF(), "back to driving — the aux var is re-allocated")
}

func TestArcLengthSetDrivenBeforeCommitNoOrphan(t *testing.T) {
	// CheckConstraint probes a candidate by temporarily allocating its aux vars
	// (setting c.s) and rolling back. A subsequent SetDriven on that still-
	// uncommitted dimension must NOT mutate the sketch's variables — membership,
	// not c.s != nil, is the committed test — or it leaks an orphan free DOF.
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	s.Fix(c)
	s.Fix(start)
	arc := s.CreateArc(c, start, s.CreatePoint(0, 4))
	require.Equal(t, 1, s.DOF())

	dim := sketch.NewArcLength(arc, 3*math.Pi)
	require.NoError(t, s.CheckConstraint(dim)) // sets dim.s via temporary allocVars
	dim.SetDriven(true)                        // uncommitted: record only
	dim.SetDriven(false)                       // uncommitted: must not allocate an aux var
	require.Equal(t, 1, s.DOF(), "an uncommitted toggle must not change the sketch")
}

func TestArcLengthDrivenRoundTrip(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	end := s.CreatePoint(0, 4)
	s.Fix(c)
	s.Fix(start)
	s.Fix(end)
	arc := s.CreateArc(c, start, end)
	dim := sketch.NewArcLength(arc, 0)
	dim.SetDriven(true)
	s.AddConstraint(dim)
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	d2, ok := s2.Constraints()[len(s2.Constraints())-1].(*sketch.ArcLength)
	require.True(t, ok, "the reloaded constraint is an ArcLength")
	require.True(t, d2.Driven(), "driven flag survives the round-trip")
	_, err = s2.Solve()
	require.NoError(t, err)
	require.InDelta(t, 2*math.Pi, d2.Target().Mag(), 1e-6, "measured value after reload")
}

func TestArcLengthRoundTrip(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	s.Fix(c)
	s.Fix(start)
	arc := s.CreateArc(c, start, s.CreatePoint(0, 4))
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
