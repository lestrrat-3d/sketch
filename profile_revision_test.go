package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// squareSketch is a solvable unit square, the smallest thing that yields one profile.
func squareSketch(t *testing.T) (*sketch.Sketch, []*sketch.Point) {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	bl, br := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	tr, tl := s.CreatePoint(10, 10), s.CreatePoint(0, 10)
	s.CreateLine(bl, br)
	s.CreateLine(br, tr)
	s.CreateLine(tr, tl)
	s.CreateLine(tl, bl)
	return s, []*sketch.Point{bl, br, tr, tl}
}

// nurbsCapSketch is a region closed by a NURBS arc over four control points and
// a line joining its endpoints. The control points are identical for every
// (degree, weights, knots) triple it is called with, so two sketches it builds
// differ ONLY in the NURBS' structural data — the state that lives on the entity
// rather than in the solver's var vector.
func nurbsCapSketch(t *testing.T, degree int, weights, knots []float64) *sketch.Sketch {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	// Deliberately asymmetric: a control polygon symmetric about its mid-line
	// makes the swept area insensitive to the interior knot, hiding a real shape
	// change behind an unchanged number.
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(2, 12)
	c := s.CreatePoint(15, 9)
	d := s.CreatePoint(20, 0)
	_, err = s.CreateNURBS(degree, []*sketch.Point{a, b, c, d}, weights, knots)
	require.NoError(t, err)
	s.CreateLine(d, a) // close the region back to the curve's start
	return s
}

func TestProfileStaleness(t *testing.T) {
	t.Run("a fresh profile is not stale and knows its sketch", func(t *testing.T) {
		s, _ := squareSketch(t)
		profiles := s.Profiles()
		require.Len(t, profiles, 1)
		p := profiles[0]

		require.False(t, p.IsStale())
		require.Same(t, s, p.Sketch(), "a profile must know the sketch it came from")
		require.Equal(t, s.Revision(), p.Revision())
	})

	t.Run("revision is stable when nothing changes", func(t *testing.T) {
		s, _ := squareSketch(t)
		before := s.Revision()
		// Reading must not mutate: profiles, area and repeated revision calls.
		_ = s.Profiles()
		_ = s.Revision()
		require.Equal(t, before, s.Revision(), "Revision must be pure")
		require.False(t, s.Profiles()[0].IsStale())
	})

	t.Run("moving geometry makes an existing profile stale", func(t *testing.T) {
		s, pts := squareSketch(t)
		p := s.Profiles()[0]
		require.False(t, p.IsStale())

		pts[2].MoveTo(20, 20) // the shape is now a different quadrilateral

		require.True(t, p.IsStale(), "the profile describes the OLD boundary")
		require.False(t, s.Profiles()[0].IsStale(), "a rebuilt profile is fresh")
	})

	t.Run("solving makes an existing profile stale", func(t *testing.T) {
		// The failure decad reported: build a profile, re-solve, extrude the ghost.
		s, pts := squareSketch(t)
		s.AddConstraint(sketch.NewHorizontal(s.Entities()[0].(*sketch.Line)))
		pts[0].MoveTo(0, 0)
		s.Fix(pts[0])

		p := s.Profiles()[0]
		areaBefore := p.Area

		pts[1].MoveTo(10, 3) // perturb, then let the solver pull it back
		_, err := s.Solve(t.Context())
		require.NoError(t, err)

		require.True(t, p.IsStale(), "geometry moved under the profile")
		require.Equal(t, areaBefore, p.Area, "the stale profile still reports its OLD area")
	})

	t.Run("adding or removing an entity makes a profile stale", func(t *testing.T) {
		s, _ := squareSketch(t)
		p := s.Profiles()[0]

		e := s.CreateLine(s.CreatePoint(20, 20), s.CreatePoint(30, 30))
		require.True(t, p.IsStale(), "a new entity changes the arrangement input")

		p2 := s.Profiles()[0]
		require.False(t, p2.IsStale())
		require.True(t, s.RemoveEntity(e))
		require.True(t, p2.IsStale(), "removing an entity changes it back but is still a change")
	})

	t.Run("toggling construction makes a profile stale without moving anything", func(t *testing.T) {
		// The adversarial case for a coordinate-only fingerprint: construction
		// geometry is EXCLUDED from profiles, so flipping the flag changes the
		// region set while every coordinate stays put. A revision that only
		// hashed coordinates would call this fresh — and it is not.
		s, _ := squareSketch(t)
		p := s.Profiles()[0]
		require.False(t, p.IsStale())

		s.Entities()[0].SetConstruction(true) // one wall of the square is now construction

		require.True(t, p.IsStale(), "the square no longer closes; the profile is a ghost")
		require.Empty(t, s.Profiles(), "and indeed there is no region any more")
	})

	t.Run("changing NURBS structural data makes a profile stale", func(t *testing.T) {
		// The adversarial case for a var-vector-only fingerprint: a NURBS' degree,
		// knots and weights are STRUCTURAL data, not solver vars, yet Profiles()
		// reads all three. Changing them reshapes the curve — and the profile's
		// area — without moving a single point, so a revision that hashed only
		// s.vars plus the topology would call the old profile fresh.
		//
		// Sketch.UnmarshalJSON rebuilds a sketch IN PLACE, so it is the public API
		// path that mutates structural data on a live sketch: loading a document
		// whose points match but whose NURBS differs is exactly the bug's trigger.
		// Every variant keeps the SAME four control points; only the structural
		// data differs from the baseline (degree 2, unit weights, mid-knot 0.5).
		for _, tc := range []struct {
			name    string
			degree  int
			weights []float64
			knots   []float64
		}{
			{
				name: "weights", degree: 2,
				weights: []float64{1, 8, 8, 1},
				knots:   []float64{0, 0, 0, 0.5, 1, 1, 1},
			},
			{
				name: "knots", degree: 2,
				weights: []float64{1, 1, 1, 1},
				knots:   []float64{0, 0, 0, 0.9, 1, 1, 1},
			},
			{
				name: "degree", degree: 3,
				weights: []float64{1, 1, 1, 1},
				knots:   []float64{0, 0, 0, 0, 1, 1, 1, 1},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// The baseline and the variant differ ONLY in the NURBS' structural
				// data: same points, same topology, same construction flags.
				base := nurbsCapSketch(t, 2, []float64{1, 1, 1, 1}, []float64{0, 0, 0, 0.5, 1, 1, 1})
				variant := nurbsCapSketch(t, tc.degree, tc.weights, tc.knots)

				require.NotEqual(t, base.Revision(), variant.Revision(),
					"a NURBS structural change must change the fingerprint")

				p := base.Profiles()[0]
				require.False(t, p.IsStale())
				areaBefore := p.Area

				doc, err := json.Marshal(variant)
				require.NoError(t, err)
				require.NoError(t, base.UnmarshalJSON(doc)) // rebuilds base in place

				require.True(t, p.IsStale(), "the curve was reshaped under the profile")
				rebuilt := base.Profiles()
				require.Len(t, rebuilt, 1)
				require.False(t, rebuilt[0].IsStale())
				require.Greater(t, math.Abs(rebuilt[0].Area-areaBefore), 1e-6,
					"the region really did change shape")
			})
		}
	})

	t.Run("a profile of another sketch is not confused for this one", func(t *testing.T) {
		s1, _ := squareSketch(t)
		s2, _ := squareSketch(t)

		p1 := s1.Profiles()[0]
		require.Same(t, s1, p1.Sketch())
		require.NotSame(t, s2, p1.Sketch())

		// Two identical sketches happen to share a revision — that is fine and
		// expected of a fingerprint. Provenance is what Sketch() is for; staleness
		// is always asked of the profile's OWN sketch.
		require.False(t, p1.IsStale())
	})
}

// TestProfileStalenessPointRewire covers the defining points an entity is built
// over. Entity fields (Line.Start, …) are exported, so a caller can point one at
// a DIFFERENT *Point that carries the same numeric id — a point of another
// sketch, or a removed handle whose id a later point has since inherited.
// buildProfiles follows the pointer, so the region set changes; the fingerprint
// must follow the pointer too, or the profile built before the rewire reads
// fresh while describing geometry that no longer exists.
func TestProfileStalenessPointRewire(t *testing.T) {
	t.Run("a defining point rewired to a foreign point with the same id", func(t *testing.T) {
		s, _ := squareSketch(t)

		// A second sketch whose points carry the very same ids (0..3) but sit
		// elsewhere: id equality across sketches is meaningless, the ids are
		// per-sketch slice positions.
		w2 := sketch.NewWorld()
		other, err := w2.CreateSketch(w2.XY())
		require.NoError(t, err)
		foreign := other.CreatePoint(100, 100)
		require.Equal(t, s.Points()[0].ID(), foreign.ID(), "the same numeric id in another sketch")

		before := s.Profiles()
		require.Len(t, before, 1)
		p := before[0]
		rev := s.Revision()

		line, ok := s.Entities()[0].(*sketch.Line)
		require.True(t, ok)
		require.Same(t, s.Points()[0], line.Start)
		line.Start = foreign // the square's corner now belongs to another sketch

		require.NotEqual(t, rev, s.Revision(),
			"the revision must follow the point pointer, not its id")
		require.True(t, p.IsStale(), "the profile no longer describes the sketch")
		require.Empty(t, s.Profiles(), "the boundary is broken open, so no region survives")
	})

	t.Run("a defining point rewired to a same-coordinate foreign point", func(t *testing.T) {
		// The subtlest form: the foreign point has the SAME id AND the SAME
		// coordinates, so only its identity differs. buildProfiles keys its shared
		// geom.Point map on the *Point, so identity is an input to the arrangement;
		// the revision hashes the sharing structure and reports the rewire
		// conservatively rather than betting on coincident vertices welding.
		s, pts := squareSketch(t)
		other, _ := squareSketch(t)
		foreign := other.Points()[0]
		require.Equal(t, pts[0].ID(), foreign.ID())
		require.Equal(t, pts[0].X(), foreign.X())
		require.Equal(t, pts[0].Y(), foreign.Y())

		p := s.Profiles()[0]
		rev := s.Revision()

		line, ok := s.Entities()[0].(*sketch.Line)
		require.True(t, ok)
		line.Start = foreign

		require.NotEqual(t, rev, s.Revision(),
			"point sharing is an input to the arrangement, so the revision must see it")
		require.True(t, p.IsStale())
	})

	t.Run("a defining point rewired to a removed handle with a reused id", func(t *testing.T) {
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		// The spare is created FIRST, so removing it renumbers the corners down and
		// the dead handle keeps the id (0) the bottom-left corner then inherits.
		spare := s.CreatePoint(50, 50)
		bl, br := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
		tr, tl := s.CreatePoint(10, 10), s.CreatePoint(0, 10)
		s.CreateLine(bl, br)
		s.CreateLine(br, tr)
		s.CreateLine(tr, tl)
		s.CreateLine(tl, bl)
		require.True(t, s.RemovePoint(spare))
		require.Equal(t, spare.ID(), bl.ID(), "the dead handle's id was reused by a live point")

		before := s.Profiles()
		require.Len(t, before, 1)
		p := before[0]
		rev := s.Revision()

		line, ok := s.Entities()[0].(*sketch.Line)
		require.True(t, ok)
		require.Same(t, bl, line.Start)
		line.Start = spare // a removed handle, still resolving to (50, 50)

		require.NotEqual(t, rev, s.Revision(),
			"a dead handle with a reused id is the same class of rewire")
		require.True(t, p.IsStale())
		require.Empty(t, s.Profiles(), "the corner moved to (50,50), so the square is open")
	})
}
