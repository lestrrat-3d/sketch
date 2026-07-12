package sketch

// This is the one IN-PACKAGE test file (the repo's tests otherwise live in
// external xxx_test packages and exercise only the exported API). It is here on
// purpose: the property under test is that [Sketch.Revision] hashes the shape
// value an entity RESOLVES — not the var vector it happens to resolve out of —
// and the only way to break the two apart is to rebind an entity's var selector
// (Circle.ri, Ellipse.rxi, Conic.rhoi, …), which is unexported state. An
// external test cannot reach it, and the right answer is NOT to export a
// selector-rebinding accessor just to test it: that would add a footgun to the
// public API to prove the fingerprint is robust against a footgun.
//
// The consumer-visible half of the contract (a shape-value change makes a
// profile stale) is covered externally in profile_revision_test.go.

import (
	"encoding/json"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRevisionResolvesShapeValues rebinds each shape-carrying entity's var
// selector without touching the var vector, the points, or the topology. Every
// proxy for the shape — the var vector's contents, the multiset of selector
// indices, the entity set, the point coordinates — is invariant under the
// rebinding, while Profiles() changes shape, so a fingerprint built on any of
// those proxies would report a mutated sketch as unchanged and a stale Profile
// would read fresh. Only hashing the value the entity actually resolves
// (Circle.r(), Ellipse.rx()/ry()/rot(), Conic.rho() — the very accessors
// buildProfiles reads through) survives this.
func TestRevisionResolvesShapeValues(t *testing.T) {
	t.Run("circle radius selectors swapped", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		c1 := s.CreateCircle(s.CreatePoint(0, 0), 2)  // area 4pi
		c2 := s.CreateCircle(s.CreatePoint(20, 0), 5) // area 25pi

		before := s.Revision()
		p1, p2 := s.Profiles()[0], s.Profiles()[1]
		require.False(t, p1.IsStale())

		c1.ri, c2.ri = c2.ri, c1.ri // the two disks trade radii; s.vars is untouched

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the radius each circle RESOLVES, not the var vector")
		require.True(t, p1.IsStale())
		require.True(t, p2.IsStale())

		after := s.Profiles()
		require.Len(t, after, 2)
		require.Greater(t, math.Abs(after[0].Area-p1.Area), 1e-9, "the profiles really did change")
	})

	t.Run("ellipse semi-axis selectors swapped", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		e := s.CreateEllipse(s.CreatePoint(0, 0), 8, 3, 0)

		before := s.Revision()
		p := s.Profiles()[0]

		e.rxi, e.ryi = e.ryi, e.rxi // an 8x3 ellipse becomes a 3x8 one

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the semi-axes the ellipse RESOLVES")
		require.True(t, p.IsStale())
	})

	t.Run("ellipse rotation selector rebound", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		e1 := s.CreateEllipse(s.CreatePoint(0, 0), 8, 3, 0)
		e2 := s.CreateEllipse(s.CreatePoint(30, 0), 8, 3, 1)

		before := s.Revision()
		p := s.Profiles()[0]

		e1.roti = e2.roti // the first ellipse is now rotated too

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the rotation the ellipse RESOLVES")
		require.True(t, p.IsStale())
	})

	t.Run("conic rho selector rebound", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		a, apex, b := s.CreatePoint(0, 0), s.CreatePoint(10, 10), s.CreatePoint(20, 0)
		c1, err := s.CreateConic(a, apex, b, 0.3)
		require.NoError(t, err)
		s.CreateLine(b, a) // close the region under the conic

		// A second conic elsewhere, purely to own a different rho var.
		c2, err := s.CreateConic(s.CreatePoint(40, 0), s.CreatePoint(50, 10), s.CreatePoint(60, 0), 0.8)
		require.NoError(t, err)
		require.NotNil(t, c2)

		before := s.Revision()
		p := s.Profiles()[0]
		areaBefore := p.Area

		c1.rhoi = c2.rhoi // the conic bulges out to rho 0.8; no point moves

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the rho the conic RESOLVES")
		require.True(t, p.IsStale())
		require.Greater(t, math.Abs(s.Profiles()[0].Area-areaBefore), 1e-9,
			"the region under the conic really did change area")
	})

	t.Run("elliptical arc semi-axis selectors swapped", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		ea := s.CreateEllipticalArc(s.CreatePoint(0, 0), s.CreatePoint(8, 0), s.CreatePoint(0, 3), 8, 3, 0)
		require.NotNil(t, ea)

		before := s.Revision()

		ea.rxi, ea.ryi = ea.ryi, ea.rxi

		require.NotEqual(t, before, s.Revision(),
			"the revision must hash the semi-axes the elliptical arc RESOLVES")
	})
}

// TestEntityUIDInvariant asserts the invariant Sketch.Revision RESTS on rather
// than repairs: every entity in s.ents carries a nonzero instance identity,
// because addEntity is the sole funnel into the slice and Sketch.Entities hands
// out a copy so no caller can splice one in behind it. Revision only READS the
// uid (Sketch.entUID) — it must never stamp one, or fingerprinting a sketch
// would mutate it and race with a concurrent read (see the purity/race tests in
// profile_revision_test.go).
//
// Every creation path is exercised: the primitive builders, the compound
// builders, the modification tools' build-then-replace, reference geometry, and
// both loaders. Observing s.ents/s.entUIDs directly is only possible in-package.
func TestEntityUIDInvariant(t *testing.T) {
	requireStamped := func(t *testing.T, s *Sketch) {
		t.Helper()
		for i, e := range s.ents {
			require.NotZerof(t, s.entUIDs[e], "entity %d (%T) reached s.ents without a uid", i, e)
			require.LessOrEqualf(t, s.entUIDs[e], s.nextEntID, "entity %d (%T) carries a uid above the counter", i, e)
		}
	}

	t.Run("primitive builders", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		a, b, c := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(10, 10)
		s.CreateLine(a, b)
		s.CreateCircle(s.CreatePoint(40, 0), 3)
		s.CreateArc(a, b, c)
		s.CreateEllipse(s.CreatePoint(60, 0), 8, 3, 0)
		s.CreateEllipticalArc(s.CreatePoint(80, 0), s.CreatePoint(88, 0), s.CreatePoint(80, 3), 8, 3, 0)
		_, err = s.CreateConic(s.CreatePoint(100, 0), s.CreatePoint(110, 10), s.CreatePoint(120, 0), 0.4)
		require.NoError(t, err)
		_, err = s.CreateSpline(s.CreatePoint(0, 40), s.CreatePoint(10, 50), s.CreatePoint(20, 40), s.CreatePoint(30, 50))
		require.NoError(t, err)
		_, err = s.CreateClosedSpline(s.CreatePoint(0, 80), s.CreatePoint(10, 90), s.CreatePoint(20, 80))
		require.NoError(t, err)
		_, err = s.CreateFitSpline(s.CreatePoint(40, 80), s.CreatePoint(50, 90), s.CreatePoint(60, 80))
		require.NoError(t, err)
		_, err = s.CreateNURBS(2,
			[]*Point{s.CreatePoint(80, 80), s.CreatePoint(90, 90), s.CreatePoint(100, 80)},
			[]float64{1, 2, 1}, []float64{0, 0, 0, 1, 1, 1})
		require.NoError(t, err)
		require.Len(t, s.ents, 10)
		requireStamped(t, s)
	})

	t.Run("compound builders", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		s.CreateRectangle(0, 0, 10, 5)
		_, err = s.CreatePolygon(40, 0, 6, 5)
		require.NoError(t, err)
		_, err = s.CreateSlot(0, 40, 20, 40, 4)
		require.NoError(t, err)
		requireStamped(t, s)
	})

	t.Run("modification tools", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		o, a, b := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(0, 10)
		l1, l2 := s.CreateLine(o, a), s.CreateLine(o, b)
		_, err = s.CreateFillet(l1, l2, 2)
		require.NoError(t, err)

		axis := s.CreateLine(s.CreatePoint(30, -10), s.CreatePoint(30, 10))
		seed := s.CreateLine(s.CreatePoint(20, 0), s.CreatePoint(25, 5))
		require.NotNil(t, s.CreateMirror([]Entity{seed}, axis))
		_, err = s.CreatePatternRect([]Entity{seed}, 2, 2, 5, 5)
		require.NoError(t, err)
		_, err = s.CreateOffset([]Entity{seed}, 1)
		require.NoError(t, err)

		long := s.CreateLine(s.CreatePoint(0, 60), s.CreatePoint(40, 60))
		_, _, ok := s.Break(long, 20, 60)
		require.True(t, ok)

		cut := s.CreateLine(s.CreatePoint(0, 80), s.CreatePoint(40, 80))
		s.CreateLine(s.CreatePoint(20, 70), s.CreatePoint(20, 90))
		_, ok = s.Trim(cut, 5, 80)
		require.True(t, ok)

		requireStamped(t, s)
	})

	t.Run("reference geometry", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		r1 := s.CreateReferencePoint(0, 0, "edge-1")
		r2 := s.CreateReferencePoint(10, 0, "edge-1")
		r3 := s.CreateReferencePoint(0, 10, "edge-1") // equidistant from r1 with r2, so the arc is well formed
		_, err = s.CreateReferenceLine(r1, r2, "edge-1")
		require.NoError(t, err)
		_, err = s.CreateReferenceArc(r1, r2, r3, "edge-2")
		require.NoError(t, err)
		_, err = s.CreateReferenceCircle(r1, 5, "edge-3")
		require.NoError(t, err)
		requireStamped(t, s)
	})

	t.Run("Sketch.UnmarshalJSON", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		s.CreateRectangle(0, 0, 10, 5)
		s.CreateCircle(s.CreatePoint(5, 2), 1)
		data, err := json.Marshal(s)
		require.NoError(t, err)

		// The rebuild resets the struct in place; the uid counter is carried over,
		// so every rebuilt entity is stamped ABOVE every retired one.
		before := s.nextEntID
		require.NoError(t, s.UnmarshalJSON(data))
		requireStamped(t, s)
		for _, e := range s.ents {
			require.Greater(t, s.entUIDs[e], before, "a rebuilt entity must not reuse a retired uid")
		}
	})

	t.Run("World.UnmarshalJSON", func(t *testing.T) {
		w := NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		s.CreateRectangle(0, 0, 10, 5)
		data, err := json.Marshal(w)
		require.NoError(t, err)

		var loaded World
		require.NoError(t, loaded.UnmarshalJSON(data))
		require.Len(t, loaded.sketches, 1)
		requireStamped(t, loaded.sketches[0])
	})
}

// TestRevisionDoesNotStamp pins the fix for the read-mutator bug: Revision must
// write NOTHING, not even when it meets an entity that reached s.ents without a
// uid (an invariant violation TestEntityUIDInvariant shows is unreachable). It
// used to STAMP such an entity — growing the uid map and the counter during what
// the API advertises as a pure read, which raced two concurrent Revision calls
// on one sketch (see TestRevisionConcurrent in the external suite).
//
// Splicing into s.ents is only possible in-package, which is why this lives here.
func TestRevisionDoesNotStamp(t *testing.T) {
	w := NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a, b := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	s.CreateLine(a, b)

	// Bypass addEntity entirely: no uid is stamped.
	intruder := &Line{Start: a, End: b}
	s.ents = append(s.ents, intruder)

	uids, next := len(s.entUIDs), s.nextEntID
	rev := s.Revision()
	require.Equal(t, uids, len(s.entUIDs), "Revision must not write the uid map")
	require.Equal(t, next, s.nextEntID, "Revision must not advance the uid counter")
	require.Zero(t, s.entUIDs[intruder], "Revision must not stamp an unstamped entity")
	require.Equal(t, rev, s.Revision(), "the revision is a fingerprint: unchanged state, equal value")

	// The unstamped entity hashes as a sentinel disjoint from every real uid, so
	// it can never fingerprint like a stamped instance.
	require.Equal(t, uint64(unstampedUID), s.entUID(intruder))
	require.NotEqual(t, uint64(unstampedUID), s.entUID(s.ents[0]))
}

// TestRevisionConcurrentUnstamped is the reviewer's race, pinned. Revision used
// to STAMP an entity it found in s.ents without a uid — a map write (and a
// counter bump) inside a read — so two goroutines merely READING one sketch that
// contains such an entity raced on s.entUIDs and could die with "concurrent map
// read and map write". With Revision a pure read the same program is race-free.
//
// It lives in-package because splicing an unstamped entity into s.ents is the
// only way to reach the state, and it is unreachable from the exported API.
func TestRevisionConcurrentUnstamped(t *testing.T) {
	w := NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a, b := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	s.CreateLine(a, b)
	s.ents = append(s.ents, &Line{Start: a, End: b}) // never passed through addEntity

	// Nothing reads the revision before the goroutines start: a stamp-on-read
	// Revision writes on its FIRST look at the intruder, so the racing write only
	// happens if the concurrent calls are the first ones.
	got := make([]uint64, 8)
	var wg sync.WaitGroup
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				got[i] = s.Revision()
			}
		}()
	}
	wg.Wait()

	// A pure read gives every goroutine the same fingerprint; a stamping one hands
	// out a different uid to whoever looks first (when it does not simply crash on
	// the concurrent map write).
	for _, v := range got {
		require.Equal(t, got[0], v, "concurrent reads of one sketch must agree")
	}
	require.Equal(t, got[0], s.Revision(), "and must not have moved the fingerprint")
}
