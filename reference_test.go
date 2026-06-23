package sketch_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A reference point is locked: the solver never moves it, and a free point made
// coincident with it solves TO it (the pierce case).
func TestReferencePointLockedAndPierce(t *testing.T) {
	s := newSketch(t)
	ref := s.CreateReferencePoint(3, 4, "vertex#1")
	require.True(t, ref.IsReference())
	require.Equal(t, "vertex#1", ref.Source())
	require.True(t, ref.IsFixed(), "reference geometry is locked")

	free := s.CreatePoint(0, 0)
	s.AddConstraint(sketch.NewCoincident(free, ref))
	if _, err := s.Solve(); err != nil {
		t.Fatalf("solve: %v", err)
	}
	require.InDelta(t, 3, free.X(), 1e-9, "free point pierces the reference")
	require.InDelta(t, 4, free.Y(), 1e-9)
	require.InDelta(t, 3, ref.X(), 1e-12, "reference did not move")
	require.InDelta(t, 4, ref.Y(), 1e-12)
	require.Empty(t, s.FreePoints(), "the reference point is not a free DOF")
	require.True(t, s.Verify().Trustworthy())
}

// Reference coordinates are read-only through the ordinary API; only Refresh*
// rewrites them.
func TestReferenceReadOnly(t *testing.T) {
	s := newSketch(t)
	ref := s.CreateReferencePoint(1, 1, "v")
	ref.MoveTo(9, 9)
	require.Equal(t, 1.0, ref.X(), "MoveTo is a no-op on reference geometry")
	s.Unfix(ref)
	require.True(t, ref.IsFixed(), "Unfix is a no-op on reference geometry")
	ref.SetConstruction(true)
	require.False(t, ref.IsConstruction(), "reference and construction are exclusive")

	require.NoError(t, s.RefreshReference(ref, 2, 3))
	require.Equal(t, 2.0, ref.X())
	require.Equal(t, 3.0, ref.Y())
	require.ErrorIs(t, s.RefreshReference(s.CreatePoint(0, 0), 1, 1), sketch.ErrNotReference)
}

// Reference curves require reference points of this sketch.
func TestReferenceCurveRequiresRefPoints(t *testing.T) {
	s := newSketch(t)
	r1 := s.CreateReferencePoint(0, 0, "a")
	r2 := s.CreateReferencePoint(5, 0, "b")
	free := s.CreatePoint(1, 1)

	_, err := s.CreateReferenceLine(r1, free, "edge")
	require.ErrorIs(t, err, sketch.ErrNotReference, "a reference line needs reference points")

	other := newSketch(t)
	foreign := other.CreateReferencePoint(0, 0, "x")
	_, err = s.CreateReferenceLine(r1, foreign, "edge")
	require.ErrorIs(t, err, sketch.ErrForeignPoint, "a reference line cannot use a foreign point")

	l, err := s.CreateReferenceLine(r1, r2, "edge")
	require.NoError(t, err)
	require.True(t, l.IsReference())
	require.Equal(t, "edge", l.Source())
}

// A reference circle's radius is locked too (not just its center point).
func TestReferenceCircleRadiusLocked(t *testing.T) {
	s := newSketch(t)
	center := s.CreateReferencePoint(0, 0, "hole")
	circ, err := s.CreateReferenceCircle(center, 5, "hole")
	require.NoError(t, err)
	require.True(t, circ.IsReference())

	// A dimension that disagrees with the locked radius cannot be satisfied by
	// resizing the reference circle.
	s.AddConstraint(sketch.NewRadius(circ, 8))
	_, err = s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged)
	require.InDelta(t, 5, circ.R(), 1e-9, "the locked radius did not move")

	require.NoError(t, s.RefreshReferenceCircle(circ, 7))
	require.InDelta(t, 7, circ.R(), 1e-9)
}

// The modification tools refuse reference geometry — both the mutating ones
// (Trim/Break) and the copying ones (Mirror/Pattern/Offset).
func TestReferenceToolsReject(t *testing.T) {
	s := newSketch(t)
	p1 := s.CreateReferencePoint(0, 0, "a")
	p2 := s.CreateReferencePoint(10, 0, "b")
	l, err := s.CreateReferenceLine(p1, p2, "edge")
	require.NoError(t, err)

	_, ok := s.Trim(l, 5, 0)
	require.False(t, ok, "Trim refuses reference geometry")
	_, _, ok = s.Break(l, 5, 0)
	require.False(t, ok, "Break refuses reference geometry")

	axis := s.CreateLine(s.CreatePoint(0, -1), s.CreatePoint(10, -1))
	require.Nil(t, s.CreateMirror([]sketch.Entity{l}, axis), "Mirror refuses reference geometry")
	_, err = s.CreatePatternRect([]sketch.Entity{l}, 2, 1, 5, 5)
	require.ErrorIs(t, err, sketch.ErrReferenceGeometry)
	_, err = s.CreateOffset([]sketch.Entity{l}, 2)
	require.ErrorIs(t, err, sketch.ErrReferenceGeometry)
}

// A reference entity rewired to a nil defining point must be reported broken,
// not panic the residual/profile analysis.
func TestReferenceNilTopologyNoPanic(t *testing.T) {
	s := newSketch(t)
	p1 := s.CreateReferencePoint(0, 0, "a")
	p2 := s.CreateReferencePoint(10, 0, "b")
	l, err := s.CreateReferenceLine(p1, p2, "edge")
	require.NoError(t, err)
	l.Start = nil // corrupt topology

	var rep *sketch.VerificationReport
	require.NotPanics(t, func() { rep = s.Verify() })
	require.Contains(t, rep.BrokenReferences, sketch.Entity(l))
	require.False(t, rep.Trustworthy())
}

// Rewiring a reference entity's exported field — to a free point OR to a
// different reference point — is detected as a broken reference.
func TestReferenceIntegrityRewire(t *testing.T) {
	s := newSketch(t)
	p1 := s.CreateReferencePoint(0, 0, "a")
	p2 := s.CreateReferencePoint(10, 0, "b")
	l, err := s.CreateReferenceLine(p1, p2, "edge")
	require.NoError(t, err)
	require.Empty(t, s.Verify().BrokenReferences, "a freshly built reference is intact")

	free := s.CreatePoint(1, 1)
	l.Start = free // rewire to a free point
	rep := s.Verify()
	require.Contains(t, rep.BrokenReferences, sketch.Entity(l))
	require.False(t, rep.Trustworthy())

	p3 := s.CreateReferencePoint(2, 2, "c")
	l.Start = p3 // rewire to a DIFFERENT reference point: still caught by the seal
	require.Contains(t, s.Verify().BrokenReferences, sketch.Entity(l))
}

// A constraint reaching a foreign reference point (from another sketch) is
// surfaced rather than silently trusted.
func TestReferenceForeignHandle(t *testing.T) {
	s := newSketch(t)
	local := s.CreatePoint(0, 0)
	other := newSketch(t)
	foreign := other.CreateReferencePoint(5, 5, "x")
	s.AddConstraint(sketch.NewCoincident(local, foreign))

	rep := s.Verify()
	require.True(t, rep.ForeignHandles)
	require.False(t, rep.Trustworthy())
}

// Staleness is marked per source and clears only by refreshing every unit.
func TestReferenceStaleness(t *testing.T) {
	s := newSketch(t)
	p1 := s.CreateReferencePoint(0, 0, "edge")
	p2 := s.CreateReferencePoint(10, 0, "edge")
	l, err := s.CreateReferenceLine(p1, p2, "edge")
	require.NoError(t, err)
	require.False(t, s.Verify().Stale)
	require.True(t, s.Verify().Trustworthy())

	s.MarkStale("edge")
	rep := s.Verify()
	require.True(t, rep.Stale)
	require.True(t, p1.IsStale())
	require.True(t, l.IsStale(), "line staleness is derived from its points")
	require.Contains(t, rep.StaleReferencePoints, p1)
	require.Contains(t, rep.StaleReferences, sketch.Entity(l))
	require.False(t, rep.Trustworthy(), "a stale sketch is never a clean pass")

	require.NoError(t, s.RefreshReference(p1, 0, 0))
	require.True(t, s.Verify().Stale, "p2 is still stale (partial refresh)")
	require.NoError(t, s.RefreshReference(p2, 10, 0))
	require.False(t, s.Verify().Stale, "all units re-fed")
	require.False(t, l.IsStale())
}

// MarkStale on an entity's source reaches the entity's points even when they
// carry different (vertex) sources.
func TestReferenceStaleEntitySource(t *testing.T) {
	s := newSketch(t)
	p1 := s.CreateReferencePoint(0, 0, "v1")
	p2 := s.CreateReferencePoint(10, 0, "v2")
	l, err := s.CreateReferenceLine(p1, p2, "edge1")
	require.NoError(t, err)

	s.MarkStale("edge1") // matches the line's source, not the points'
	require.True(t, p1.IsStale(), "marked via the entity's sealed points")
	require.True(t, p2.IsStale())
	require.True(t, l.IsStale())
}

// Reference edges participate in profiles: a loop closed by sketch lines plus a
// projected (reference) edge is detected.
func TestReferenceProfileMixedLoop(t *testing.T) {
	s := newSketch(t)
	a := s.CreateReferencePoint(0, 0, "e")
	b := s.CreateReferencePoint(10, 0, "e")
	if _, err := s.CreateReferenceLine(a, b, "e"); err != nil {
		t.Fatalf("ref line: %v", err)
	}
	c := s.CreatePoint(5, 8)
	s.CreateLine(b, c)
	s.CreateLine(c, a)
	require.Len(t, s.Profiles(), 1, "a mixed sketch+reference loop closes")
}

// reference/source/stale/lock survive a JSON round-trip.
func TestReferenceRoundTrip(t *testing.T) {
	s := newSketch(t)
	s.CreateReferencePoint(3, 4, "v")
	center := s.CreateReferencePoint(0, 0, "h")
	circ, err := s.CreateReferenceCircle(center, 5, "h")
	require.NoError(t, err)
	circ.SetConstruction(true) // must stay non-construction
	s.MarkStale("h")

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var loaded sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &loaded))

	var v, h *sketch.Point
	for _, p := range loaded.Points() {
		switch p.Source() {
		case "v":
			v = p
		case "h":
			h = p
		}
	}
	require.NotNil(t, v)
	require.True(t, v.IsReference() && v.IsFixed())
	require.True(t, h.IsStale(), "staleness round-trips")

	require.Len(t, loaded.Entities(), 1)
	lc := loaded.Entities()[0]
	require.True(t, lc.IsReference())
	require.False(t, lc.IsConstruction())
	require.True(t, lc.IsStale(), "reference circle radius staleness round-trips")
	require.Empty(t, loaded.Verify().BrokenReferences, "reloaded reference is intact")
}

// A corrupt document — a reference entity on non-reference points — is rejected.
func TestReferenceLoadRejectsCorrupt(t *testing.T) {
	// Build a valid normal-line sketch, then forge reference:true onto the line
	// without making its points reference.
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.CreateLine(a, b)
	data, err := json.Marshal(s)
	require.NoError(t, err)
	forged := strings.Replace(string(data), `"type":"line"`, `"type":"line","reference":true,"source":"e"`, 1)

	var loaded sketch.Sketch
	require.Error(t, json.Unmarshal([]byte(forged), &loaded), "reference entity on free points is rejected")
}

// A reference entity and its points remove cleanly.
func TestReferenceRemoval(t *testing.T) {
	s := newSketch(t)
	p1 := s.CreateReferencePoint(0, 0, "a")
	p2 := s.CreateReferencePoint(10, 0, "b")
	l, err := s.CreateReferenceLine(p1, p2, "edge")
	require.NoError(t, err)
	require.True(t, s.RemoveEntity(l))
	require.True(t, s.RemovePoint(p1))
	require.True(t, s.RemovePoint(p2))
	require.Empty(t, s.Entities())
	require.Empty(t, s.Verify().BrokenReferences)
}

// A valid reference arc is trustworthy: it must not be flagged with a redundant
// internal constraint (its locked points need no radius-consistency solver row).
func TestReferenceArcTrustworthy(t *testing.T) {
	s := newSketch(t)
	center := s.CreateReferencePoint(0, 0, "arc")
	start := s.CreateReferencePoint(5, 0, "arc")
	end := s.CreateReferencePoint(0, 5, "arc")
	arc, err := s.CreateReferenceArc(center, start, end, "arc")
	require.NoError(t, err)
	require.True(t, arc.IsReference())

	rep := s.Verify()
	require.Empty(t, rep.Redundant, "a locked reference arc adds no redundant constraint")
	require.True(t, rep.Trustworthy())

	// An inconsistent arc snapshot (start/end not equidistant) is rejected.
	_, err = s.CreateReferenceArc(center, s.CreateReferencePoint(5, 0, "bad"), s.CreateReferencePoint(0, 9, "bad"), "bad")
	require.ErrorIs(t, err, sketch.ErrInvalidShape)
}

// A constraint with a typed-nil entity operand must not panic Verify.
func TestReferenceVerifyTypedNilOperand(t *testing.T) {
	s := newSketch(t)
	s.AddConstraint(sketch.NewHorizontal(nil)) // typed-nil *Line operand
	var rep *sketch.VerificationReport
	require.NotPanics(t, func() { rep = s.Verify() })
	require.False(t, rep.Trustworthy())

	// A non-nil but foreign entity with nil endpoints must not panic either.
	s2 := newSketch(t)
	s2.AddConstraint(sketch.NewHorizontal(&sketch.Line{}))
	require.NotPanics(t, func() { rep = s2.Verify() })
	require.False(t, rep.Trustworthy())
}

// Refreshing a reference arc endpoint to a different radius breaks the arc
// invariant; the integrity check (not a solver constraint) catches it.
func TestReferenceArcRefreshBreaksInvariant(t *testing.T) {
	s := newSketch(t)
	center := s.CreateReferencePoint(0, 0, "arc")
	start := s.CreateReferencePoint(5, 0, "arc")
	end := s.CreateReferencePoint(0, 5, "arc")
	arc, err := s.CreateReferenceArc(center, start, end, "arc")
	require.NoError(t, err)
	require.True(t, s.Verify().Trustworthy())

	require.NoError(t, s.RefreshReference(end, 0, 9)) // radius 9 != 5: now inconsistent
	rep := s.Verify()
	require.Contains(t, rep.BrokenReferences, sketch.Entity(arc))
	require.False(t, rep.Trustworthy())
}
