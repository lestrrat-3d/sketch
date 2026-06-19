package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// xAxis returns a fixed line along the global x axis for use as a mirror axis.
func xAxis(s *sketch.Sketch) *sketch.Line {
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	s.Fix(a)
	s.Fix(b)
	return s.AddLine(a, b)
}

func TestSymmetricArcsSolvedMirror(t *testing.T) {
	// a1 is a fixed quarter arc above the x axis; a2 is free and seeded roughly
	// below it. The constraint must land a2 on the swapped mirror: a2.Start =
	// mirror(a1.End), a2.End = mirror(a1.Start), centers mirrored.
	s := sketch.New()
	axis := xAxis(s)
	c1 := s.AddPoint(2, 3)
	st1 := s.AddPoint(3, 3) // angle 0 from the center
	en1 := s.AddPoint(2, 4) // angle 90° from the center
	s.Fix(c1)
	s.Fix(st1)
	s.Fix(en1)
	a1 := s.AddArc(c1, st1, en1)

	a2 := s.AddArc(s.AddPoint(2, -2.8), s.AddPoint(2.1, -3.9), s.AddPoint(2.9, -3.1))
	s.AddConstraint(sketch.NewSymmetricArcs(a1, a2, axis))

	_, err := s.Solve()
	require.NoError(t, err)
	require.True(t, s.Verify().Solvable)

	require.InDelta(t, 2, a2.Center.X(), 1e-6)
	require.InDelta(t, -3, a2.Center.Y(), 1e-6)
	require.InDelta(t, 2, a2.Start.X(), 1e-6) // mirror(a1.End=(2,4))
	require.InDelta(t, -4, a2.Start.Y(), 1e-6)
	require.InDelta(t, 3, a2.End.X(), 1e-6) // mirror(a1.Start=(3,3))
	require.InDelta(t, -3, a2.End.Y(), 1e-6)
	require.InDelta(t, a1.Sweep(), a2.Sweep(), 1e-6, "the mirrored arc sweeps CCW the same amount")
	require.Equal(t, 0, s.DOF(), "a1 and axis fixed → a2 is fully determined")
	// (No-spurious-redundancy is asserted in TestSymmetricArcsDOFNoRedundancy with
	// both arcs free; a fully-fixed a1 here flags its own internal arcRadius row,
	// an unrelated pre-existing artifact, so RedundantConstraints is not asserted.)
}

func TestSymmetricArcsDOFNoRedundancy(t *testing.T) {
	// Both arcs free, axis fixed: a true mirror pair has exactly a1's 5 DOF (a2 is
	// determined by a1). The constraint must remove the right count AND introduce
	// no spurious redundancy — the whole point of the radial-line+branch form
	// instead of a second full point-mirror.
	s := sketch.New()
	axis := xAxis(s)
	a1 := s.AddArc(s.AddPoint(2, 3), s.AddPoint(3, 3), s.AddPoint(2, 4))
	a2 := s.AddArc(s.AddPoint(2, -3), s.AddPoint(2, -4), s.AddPoint(3, -3))
	s.AddConstraint(sketch.NewSymmetricArcs(a1, a2, axis))

	_, err := s.Solve()
	require.NoError(t, err)
	require.Equal(t, 5, s.DOF(), "a mirror pair retains exactly a1's 5 DOF")
	require.Empty(t, s.RedundantConstraints(), "no spurious redundancy against the arcs' radius constraints")
}

func TestSymmetricArcsWrongBranchRejected(t *testing.T) {
	// a2 pinned to the antipodal endpoint: centers mirror, a2.Start mirrors a1.End,
	// and a2.End is collinear with mirror(a1.Start) through the center — but on the
	// OPPOSITE ray. The branch row must reject it as unsolvable.
	s := sketch.New()
	axis := xAxis(s)
	a1 := s.AddArc(s.AddPoint(2, 3), s.AddPoint(3, 3), s.AddPoint(2, 4))
	s.Fix(a1.Center)
	s.Fix(a1.Start)
	s.Fix(a1.End)

	c2 := s.AddPoint(2, -3)
	st2 := s.AddPoint(2, -4) // mirror(a1.End): correct
	en2 := s.AddPoint(1, -3) // antipode of mirror(a1.Start)=(3,-3) about the center
	s.Fix(c2)
	s.Fix(st2)
	s.Fix(en2)
	a2 := s.AddArc(c2, st2, en2)
	s.AddConstraint(sketch.NewSymmetricArcs(a1, a2, axis))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.False(t, s.Verify().Solvable, "the antipodal endpoint is not the mirror image")
}

func TestSymmetricArcsRoundTrip(t *testing.T) {
	s := sketch.New()
	axis := xAxis(s)
	c1 := s.AddPoint(2, 3)
	st1 := s.AddPoint(3, 3)
	en1 := s.AddPoint(2, 4)
	s.Fix(c1)
	s.Fix(st1)
	s.Fix(en1)
	a1 := s.AddArc(c1, st1, en1)
	a2 := s.AddArc(s.AddPoint(2, -2.8), s.AddPoint(2.1, -3.9), s.AddPoint(2.9, -3.1))
	s.AddConstraint(sketch.NewSymmetricArcs(a1, a2, axis))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload, not doubled")
	_, err = s2.Solve()
	require.NoError(t, err)
	require.True(t, s2.Verify().Solvable)
	// the reloaded a2 (entity 2: axis=0, a1=1, a2=2) still lands on the swapped
	// mirror after reload — the branch slack is recomputed on load, not doubled.
	a2r := s2.Entities()[2].(*sketch.Arc)
	require.InDelta(t, 2, a2r.Center.X(), 1e-6)
	require.InDelta(t, -3, a2r.Center.Y(), 1e-6)
	require.InDelta(t, 3, a2r.End.X(), 1e-6)
	require.InDelta(t, -3, a2r.End.Y(), 1e-6)
}
