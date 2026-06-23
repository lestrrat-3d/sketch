package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestEqualLineArc(t *testing.T) {
	// A free horizontal line is forced to equal the swept length of a fixed
	// quarter arc (R=4, sweep π/2 → length 2π), so the line grows to 2π.
	s := newSketch(t)
	ac := s.CreatePoint(0, 0)
	astart := s.CreatePoint(4, 0)
	aend := s.CreatePoint(0, 4)
	s.Fix(ac)
	s.Fix(astart)
	s.Fix(aend)
	arc := s.CreateArc(ac, astart, aend)

	p1 := s.CreatePoint(0, -1)
	p2 := s.CreatePoint(10, -1)
	s.Fix(p1)
	line := s.CreateLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewEqualLineArc(line, arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 2*math.Pi, arc.R()*arc.Sweep(), 1e-9, "the arc is unchanged")
	require.InDelta(t, 2*math.Pi, line.Length(), 1e-6, "line length equals the arc's swept length")
}

func TestEqualLineArcDrivesArcBeyondPi(t *testing.T) {
	// A fixed line of length 3π drives a free arc's sweep past π: with R=2 fixed,
	// the swept length 2·sweep must reach 3π, so the sweep settles at 3π/2 — the
	// branch the unwrapped-sweep variable exists to reach.
	s := newSketch(t)
	p1 := s.CreatePoint(0, 10)
	p2 := s.CreatePoint(3*math.Pi, 10)
	s.Fix(p1)
	s.Fix(p2) // a rigid line of length 3π
	line := s.CreateLine(p1, p2)

	ac := s.CreatePoint(0, 0)
	astart := s.CreatePoint(2, 0)
	s.Fix(ac)
	s.Fix(astart)               // R=2, start ray at angle 0
	aend := s.CreatePoint(0, 2) // sweep π/2 initially
	arc := s.CreateArc(ac, astart, aend)
	s.AddConstraint(sketch.NewEqualLineArc(line, arc))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 3*math.Pi/2, arc.Sweep(), 1e-6, "sweep driven past π to 3π/2")
	require.InDelta(t, 3*math.Pi, arc.R()*arc.Sweep(), 1e-6, "swept length equals the line length")
}

func TestEqualLineArcOverLengthRejected(t *testing.T) {
	// A line longer than the arc's full circumference (2πR) cannot equal the arc's
	// swept length — Sweep() maxes at 2π. The oracle must report this unsolvable,
	// not bless it via a multi-turn parameterization. Here R=2 (max length 4π) and
	// the line is 5π, with everything fixed.
	s := newSketch(t)
	ac := s.CreatePoint(0, 0)
	astart := s.CreatePoint(2, 0)
	aend := s.CreatePoint(0, 2) // quarter arc, length π
	s.Fix(ac)
	s.Fix(astart)
	s.Fix(aend)
	arc := s.CreateArc(ac, astart, aend)
	p1 := s.CreatePoint(0, 10)
	p2 := s.CreatePoint(5*math.Pi, 10) // line length 5π > 2πR = 4π
	s.Fix(p1)
	s.Fix(p2)
	s.AddConstraint(sketch.NewEqualLineArc(s.CreateLine(p1, p2), arc))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "a line longer than the arc's full circumference is not matchable")
}

func TestEqualLineArcDOFAndRemoval(t *testing.T) {
	s := newSketch(t)
	ac := s.CreatePoint(0, 0)
	astart := s.CreatePoint(4, 0)
	aend := s.CreatePoint(0, 4)
	s.Fix(ac)
	s.Fix(astart)
	s.Fix(aend)
	arc := s.CreateArc(ac, astart, aend)

	p1 := s.CreatePoint(0, -1)
	p2 := s.CreatePoint(10, -1)
	s.Fix(p1)
	line := s.CreateLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	require.Equal(t, 1, s.DOF(), "the horizontal line's free end slides along x")

	con := sketch.NewEqualLineArc(line, arc)
	s.AddConstraint(con)
	require.Equal(t, 0, s.DOF(), "equal-length removes the remaining DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 1, s.DOF(), "removal restores the DOF")
}

func TestEqualLineArcRoundTrip(t *testing.T) {
	s := newSketch(t)
	ac := s.CreatePoint(0, 0)
	astart := s.CreatePoint(4, 0)
	aend := s.CreatePoint(0, 4)
	s.Fix(ac)
	s.Fix(astart)
	s.Fix(aend)
	arc := s.CreateArc(ac, astart, aend)
	p1 := s.CreatePoint(0, -1)
	p2 := s.CreatePoint(10, -1)
	s.Fix(p1)
	line := s.CreateLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewEqualLineArc(line, arc))
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
