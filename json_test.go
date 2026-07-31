package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestJSONRoundTripAllConstraintKinds serializes a small solvable sketch for
// every public constraint kind and checks that the reloaded sketch has the
// same constraints and re-solves to the same coordinates. This is the safety
// net for the marshal/unmarshal switches in json.go: a constraint kind whose
// rebuild branch is missing or wrong fails its row here. Add a row whenever a
// constraint kind is added.
func TestJSONRoundTripAllConstraintKinds(t *testing.T) {
	cases := []struct {
		name  string
		build func(s *sketch.Sketch)
	}{
		{"coincident", func(s *sketch.Sketch) {
			a := s.CreatePoint(1, 2)
			s.Fix(a)
			s.AddConstraint(sketch.NewCoincident(a, s.CreatePoint(5, 5)))
		}},
		{"horizontal", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewHorizontal(s.CreateLine(a, s.CreatePoint(5, 1))))
		}},
		{"vertical", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewVertical(s.CreateLine(a, s.CreatePoint(1, 5))))
		}},
		{"horizontalPoints", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 4)
			s.Fix(a)
			s.AddConstraint(sketch.NewHorizontalPoints(a, s.CreatePoint(6, -2)))
		}},
		{"verticalPoints", func(s *sketch.Sketch) {
			a := s.CreatePoint(3, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewVerticalPoints(a, s.CreatePoint(-1, 7)))
		}},
		{"parallel", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			c := s.CreatePoint(0, 5)
			s.Fix(c)
			s.AddConstraint(sketch.NewParallel(s.CreateLine(a, b), s.CreateLine(c, s.CreatePoint(8, 7))))
		}},
		{"perpendicular", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewPerpendicular(s.CreateLine(a, b), s.CreateLine(a, s.CreatePoint(1, 5))))
		}},
		{"pointOnLine", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewPointOnLine(s.CreatePoint(3, 4), s.CreateLine(a, b)))
		}},
		{"collinear", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewCollinear(s.CreateLine(a, b), s.CreateLine(s.CreatePoint(2, 3), s.CreatePoint(7, 5))))
		}},
		{"pointOnCircle", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			s.Fix(o)
			circ := s.CreateCircle(o, 5)
			s.AddConstraint(sketch.NewRadius(circ, 5), sketch.NewPointOnCircle(s.CreatePoint(7, 1), circ))
		}},
		{"pointOnArc", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			start := s.CreatePoint(5, 0)
			end := s.CreatePoint(0, 5)
			s.Fix(o)
			s.Fix(start)
			s.Fix(end)
			arc := s.CreateArc(o, start, end)
			s.AddConstraint(sketch.NewPointOnArc(s.CreatePoint(3, 3), arc))
		}},
		{"midpoint", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewMidpoint(s.CreatePoint(3, 3), s.CreateLine(a, b)))
		}},
		{"midpointOf", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 6)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewMidpointOf(s.CreatePoint(2, 2), a, b))
		}},
		{"symmetric", func(s *sketch.Sketch) {
			axA := s.CreatePoint(0, 0)
			axB := s.CreatePoint(0, 10)
			s.Fix(axA)
			s.Fix(axB)
			p1 := s.CreatePoint(-3, 4)
			s.Fix(p1)
			s.AddConstraint(sketch.NewSymmetric(p1, s.CreatePoint(5, 1), s.CreateLine(axA, axB)))
		}},
		{"symmetricLines", func(s *sketch.Sketch) {
			axA := s.CreatePoint(0, 0)
			axB := s.CreatePoint(0, 10)
			s.Fix(axA)
			s.Fix(axB)
			a := s.CreatePoint(2, 1)
			b := s.CreatePoint(5, 3)
			s.Fix(a)
			s.Fix(b)
			l1 := s.CreateLine(a, b)
			l2 := s.CreateLine(s.CreatePoint(-2, 1), s.CreatePoint(-4, 2))
			s.AddConstraint(sketch.NewSymmetricLines(l1, l2, s.CreateLine(axA, axB)))
		}},
		{"symmetricCircles", func(s *sketch.Sketch) {
			axA := s.CreatePoint(0, 0)
			axB := s.CreatePoint(0, 10)
			s.Fix(axA)
			s.Fix(axB)
			o1 := s.CreatePoint(3, 2)
			c1 := s.CreateCircle(o1, 4)
			s.FixEntity(c1)
			c2 := s.CreateCircle(s.CreatePoint(-3, 2), 4)
			s.AddConstraint(sketch.NewSymmetricCircles(c1, c2, s.CreateLine(axA, axB)))
		}},
		{"symmetricArcs", func(s *sketch.Sketch) {
			axA := s.CreatePoint(0, 0)
			axB := s.CreatePoint(1, 0)
			s.Fix(axA)
			s.Fix(axB)
			c1 := s.CreatePoint(2, 3)
			st1 := s.CreatePoint(3, 3)
			en1 := s.CreatePoint(2, 4)
			s.Fix(c1)
			s.Fix(st1)
			s.Fix(en1)
			a1 := s.CreateArc(c1, st1, en1)
			a2 := s.CreateArc(s.CreatePoint(2, -2.8), s.CreatePoint(2.1, -3.9), s.CreatePoint(2.9, -3.1))
			s.AddConstraint(sketch.NewSymmetricArcs(a1, a2, s.CreateLine(axA, axB)))
		}},
		{"concentric", func(s *sketch.Sketch) {
			o1 := s.CreatePoint(0, 0)
			s.Fix(o1)
			c1 := s.CreateCircle(o1, 5)
			c2 := s.CreateCircle(s.CreatePoint(3, 2), 4)
			s.AddConstraint(sketch.NewConcentric(c1, c2))
		}},
		{"concentricArcs", func(s *sketch.Sketch) {
			o1 := s.CreatePoint(0, 0)
			s.Fix(o1)
			a1 := s.CreateArc(o1, s.CreatePoint(3, 0), s.CreatePoint(0, 3))
			a2 := s.CreateArc(s.CreatePoint(5, 5), s.CreatePoint(7, 5), s.CreatePoint(5, 7))
			s.AddConstraint(sketch.NewConcentric(a1, a2))
		}},
		{"equal", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(8, 0)
			s.Fix(a)
			s.Fix(b)
			c := s.CreatePoint(20, 0)
			s.Fix(c)
			s.AddConstraint(sketch.NewEqual(s.CreateLine(a, b), s.CreateLine(c, s.CreatePoint(25, 3))))
		}},
		{"equalRadius", func(s *sketch.Sketch) {
			o1 := s.CreatePoint(0, 0)
			s.Fix(o1)
			c1 := s.CreateCircle(o1, 7)
			o2 := s.CreatePoint(20, 0)
			s.Fix(o2)
			c2 := s.CreateCircle(o2, 3)
			s.AddConstraint(sketch.NewRadius(c1, 7), sketch.NewEqualRadius(c1, c2))
		}},
		{"pointOnEllipse", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			s.Fix(o)
			e := s.CreateEllipse(o, 10, 5, 0)
			s.Fix(e.Center)
			s.AddConstraint(sketch.NewSemiMajor(e, 10), sketch.NewSemiMinor(e, 5), sketch.NewEllipseRotation(e, 0))
			s.AddConstraint(sketch.NewPointOnEllipse(s.CreatePoint(12, 1), e))
		}},
		{"tangentLineCircle", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			o := s.CreatePoint(5, 5)
			s.Fix(o)
			s.AddConstraint(sketch.NewTangent(s.CreateLine(a, b), s.CreateCircle(o, 2)))
		}},
		{"tangentCirclesExternal", func(s *sketch.Sketch) {
			o1 := s.CreatePoint(0, 0)
			s.Fix(o1)
			c1 := s.CreateCircle(o1, 3)
			o2 := s.CreatePoint(10, 0)
			s.Fix(o2)
			c2 := s.CreateCircle(o2, 2)
			s.AddConstraint(sketch.NewRadius(c1, 3), sketch.NewTangentCircles(c1, c2, false))
		}},
		{"tangentCirclesInternal", func(s *sketch.Sketch) {
			o1 := s.CreatePoint(0, 0)
			s.Fix(o1)
			c1 := s.CreateCircle(o1, 10)
			o2 := s.CreatePoint(4, 0)
			s.Fix(o2)
			c2 := s.CreateCircle(o2, 2)
			s.AddConstraint(sketch.NewRadius(c1, 10), sketch.NewTangentCircles(c1, c2, true))
		}},
		{"distance", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewDistance(a, s.CreatePoint(4, 1), 5))
		}},
		{"horizontalDistance", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewHorizontalDistance(a, s.CreatePoint(3, 1), 4))
		}},
		{"verticalDistance", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewVerticalDistance(a, s.CreatePoint(1, 2), 3))
		}},
		{"distancePointLine", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewDistancePointLine(s.CreatePoint(3, 2), s.CreateLine(a, b), 5))
		}},
		{"distanceLines", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			l2 := s.CreateLine(s.CreatePoint(0, 3), s.CreatePoint(10, 4))
			s.AddConstraint(sketch.NewDistanceLines(s.CreateLine(a, b), l2, 6))
		}},
		{"radius", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewRadius(s.CreateCircle(o, 3), 7))
		}},
		{"diameter", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewDiameter(s.CreateCircle(o, 3), 14))
		}},
		{"radiusOnArc", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewRadius(s.CreateArc(o, s.CreatePoint(3, 0), s.CreatePoint(0, 3)), 7))
		}},
		{"diameterOnArc", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewDiameter(s.CreateArc(o, s.CreatePoint(3, 0), s.CreatePoint(0, 3)), 14))
		}},
		{"arcLength", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			start := s.CreatePoint(4, 0)
			s.Fix(o)
			s.Fix(start)
			s.AddConstraint(sketch.NewArcLength(s.CreateArc(o, start, s.CreatePoint(0, 4)), 3*math.Pi))
		}},
		{"distancePointArc", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			start := s.CreatePoint(5, 0)
			end := s.CreatePoint(0, 5)
			s.Fix(o)
			s.Fix(start)
			s.Fix(end)
			arc := s.CreateArc(o, start, end)
			s.AddConstraint(sketch.NewDistancePointArc(s.CreatePoint(7, 7), arc, 2))
		}},
		{"distanceLineArc", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			start := s.CreatePoint(5, 0)
			end := s.CreatePoint(0, 5)
			s.Fix(o)
			s.Fix(start)
			s.Fix(end)
			arc := s.CreateArc(o, start, end)
			s.AddConstraint(sketch.NewDistanceLineArc(s.CreateLine(s.CreatePoint(10, 0), s.CreatePoint(10, 8)), arc, 2))
		}},
		{"angle", func(s *sketch.Sketch) {
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			c := s.CreatePoint(5, 5)
			l2 := s.CreateLine(a, c)
			s.AddConstraint(sketch.NewAngle(s.CreateLine(a, b), l2, 45))
			s.AddConstraint(sketch.NewDistance(a, c, 8))
		}},
		{"semiMajorMinorRotation", func(s *sketch.Sketch) {
			o := s.CreatePoint(0, 0)
			s.Fix(o)
			e := s.CreateEllipse(o, 4, 2, 0.5)
			s.Fix(e.Center)
			s.AddConstraint(sketch.NewSemiMajor(e, 10), sketch.NewSemiMinor(e, 5), sketch.NewEllipseRotation(e, 30))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			tc.build(s)
			_, err := s.Solve(t.Context())
			require.NoError(t, err)

			data, err := json.Marshal(s)
			require.NoError(t, err, "marshal")
			var s2 sketch.Sketch
			require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
			require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint count survives")

			_, err = s2.Solve(t.Context())
			require.NoError(t, err)
			for i, p := range s.Points() {
				require.InDeltaf(t, p.X(), s2.Points()[i].X(), 1e-6, "point %d X after reload", i)
				require.InDeltaf(t, p.Y(), s2.Points()[i].Y(), 1e-6, "point %d Y after reload", i)
			}
		})
	}
}

// TestJSONFixedPoint pins marshal∘unmarshal as a fixed point: serializing the
// reloaded sketch reproduces the original document byte for byte. Any drift —
// reordered ids, re-derived values, double-serialized internal constraints —
// shows up as a diff here.
func TestJSONFixedPoint(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(18, 2)
	c := s.CreatePoint(17, 11)
	d := s.CreatePoint(1, 13)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	dc := s.CreateLine(d, c)
	ad := s.CreateLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data1, err := json.Marshal(s)
	require.NoError(t, err, "first marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data1, &s2), "unmarshal")
	data2, err := json.Marshal(&s2)
	require.NoError(t, err, "second marshal")
	require.Equal(t, string(data1), string(data2), "marshal∘unmarshal is a fixed point")
}

// TestRoundTripPreservesSolvedState verifies that a document stores solved
// coordinates, not just structure: the reloaded sketch is already on the
// constraint manifold and a zero-iteration solve reports convergence.
func TestRoundTripPreservesSolvedState(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(18, 2)
	c := s.CreatePoint(17, 11)
	d := s.CreatePoint(1, 13)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	dc := s.CreateLine(d, c)
	ad := s.CreateLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")

	res, err := s2.Solve(t.Context(), sketch.WithMaxIterations(0))
	require.NoError(t, err, "already converged on load")
	require.True(t, res.Converged, "converged without iterating")
	require.Equal(t, 0, res.Iterations, "no iterations spent")
	require.InDelta(t, b.X(), s2.Points()[b.ID()].X(), 1e-12, "coordinates preserved verbatim")
}

// TestForeignHandleIsNotSerialized covers the wider half of the guard that
// TestForeignOriginIsNotSerialized covers for the origin: an ORDINARY point or
// entity of another sketch. Every reference is written as a bare id and the
// loader resolves it against the receiving sketch, so serializing a foreign one
// silently rebinds it to whatever local point or entity carries that number —
// and, because small ids are the common ones, a collision is the likely case.
// The round trip then reads clean, turning a sketch Verify reports as
// ForeignHandles into one it blesses. Marshalling must refuse instead.
func TestForeignHandleIsNotSerialized(t *testing.T) {
	build := func(t *testing.T) (*sketch.World, *sketch.Sketch, *sketch.Sketch) {
		t.Helper()
		w := sketch.NewWorld()
		a, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		b, err := w.CreateSketch(w.XZ())
		require.NoError(t, err)
		return w, a, b
	}

	t.Run("a constraint's point operand", func(t *testing.T) {
		w, a, b := build(t)
		// Both points get id 0 in their own sketch, so the written reference would
		// come back as a local self-coincidence.
		a.AddConstraint(sketch.NewCoincident(a.CreatePoint(3, 4), b.CreatePoint(0, 0)))
		require.True(t, a.Verify(t.Context()).ForeignHandles, "the oracle flags it before the round trip")

		_, err := json.Marshal(a)
		require.ErrorIs(t, err, sketch.ErrForeignHandle, "the sketch document refuses it")
		_, err = json.Marshal(w)
		require.ErrorIs(t, err, sketch.ErrForeignHandle, "and so does the world document")
	})

	t.Run("a constraint's entity operand", func(t *testing.T) {
		w, a, b := build(t)
		la := a.CreateLine(a.CreatePoint(0, 0), a.CreatePoint(5, 0))
		lb := b.CreateLine(b.CreatePoint(0, 0), b.CreatePoint(0, 5))
		a.AddConstraint(sketch.NewPerpendicular(la, lb))
		require.True(t, a.Verify(t.Context()).ForeignHandles, "the oracle flags it before the round trip")

		_, err := json.Marshal(a)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
		_, err = json.Marshal(w)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
	})

	t.Run("an entity's defining point", func(t *testing.T) {
		// No constraint involved: an entity writes its points' ids exactly as a
		// constraint does, and this one would reload as a zero-length line.
		w, a, b := build(t)
		a.CreateLine(b.CreatePoint(0, 0), a.CreatePoint(3, 4))
		require.True(t, a.Verify(t.Context()).ForeignHandles, "the oracle flags it before the round trip")

		_, err := json.Marshal(a)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
		_, err = json.Marshal(w)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
	})

	t.Run("an ordinary document is unaffected", func(t *testing.T) {
		// The negative control: the guard fires only on a foreign reference, so a
		// single-sketch document still marshals and round-trips byte-for-byte.
		w, a, _ := build(t)
		p := a.CreatePoint(0, 0)
		q := a.CreatePoint(5, 0)
		l := a.CreateLine(p, q)
		a.AddConstraint(sketch.NewCoincident(p, a.Origin()), sketch.NewHorizontal(l),
			sketch.NewDistance(p, q, 5))
		_, err := a.Solve(t.Context())
		require.NoError(t, err)
		require.NoError(t, a.Verify(t.Context()).Check(), "a sound sketch stays sound")

		data, err := json.Marshal(a)
		require.NoError(t, err)
		var back sketch.Sketch
		require.NoError(t, json.Unmarshal(data, &back))
		again, err := json.Marshal(&back)
		require.NoError(t, err)
		require.JSONEq(t, string(data), string(again), "the reloaded sketch writes the same document")

		wdata, err := json.Marshal(w)
		require.NoError(t, err)
		var wback sketch.World
		require.NoError(t, json.Unmarshal(wdata, &wback))
		wagain, err := json.Marshal(&wback)
		require.NoError(t, err)
		require.JSONEq(t, string(wdata), string(wagain))
	})
}

// TestDeadPointIsNotSerialized covers the half of the guard a screen on the
// point's own sketch pointer cannot see: a point of THIS sketch that
// RemovePoint has spliced out. Its sketch pointer still names this sketch, so
// only Sketch.owns — the predicate Verify uses to set ForeignHandles — rejects
// it. Its id is stale, so writing it either names a DIFFERENT live point (the
// reference silently rebinds and the reload reads clean) or names nothing at
// all (the document marshals and then fails to load). Marshalling must refuse
// in both shapes.
func TestDeadPointIsNotSerialized(t *testing.T) {
	build := func(t *testing.T) (*sketch.World, *sketch.Sketch) {
		t.Helper()
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		return w, s
	}

	t.Run("an entity's defining point", func(t *testing.T) {
		w, s := build(t)
		p0 := s.CreatePoint(0, 0)
		dead := s.CreatePoint(10, 0)
		heir := s.CreatePoint(20, 7)
		require.True(t, s.RemovePoint(dead), "the point is removed")
		require.Equal(t, dead.ID(), heir.ID(), "the removal renumbered a live point onto the dead id")

		// The endpoint would be written as the dead point's stale id and come back
		// bound to heir, moving the line's end from (10,0) to (20,7).
		s.CreateLine(dead, p0)
		require.True(t, s.Verify(t.Context()).ForeignHandles, "the oracle flags it before the round trip")

		_, err := json.Marshal(s)
		require.ErrorIs(t, err, sketch.ErrForeignHandle, "the sketch document refuses it")
		_, err = json.Marshal(w)
		require.ErrorIs(t, err, sketch.ErrForeignHandle, "and so does the world document")
	})

	t.Run("a constraint's point operand", func(t *testing.T) {
		w, s := build(t)
		p0 := s.CreatePoint(0, 0)
		dead := s.CreatePoint(10, 0)
		heir := s.CreatePoint(20, 7)
		require.True(t, s.RemovePoint(dead))
		require.Equal(t, dead.ID(), heir.ID())

		s.AddConstraint(sketch.NewCoincident(dead, p0))
		require.True(t, s.Verify(t.Context()).ForeignHandles, "the oracle flags it before the round trip")

		_, err := json.Marshal(s)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
		_, err = json.Marshal(w)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
	})

	t.Run("a dead LAST point, whose stale id is out of range", func(t *testing.T) {
		// No live point inherits the id here, so the unguarded document was written
		// successfully and then failed to reload ("point id out of range"): a corrupt
		// document produced with no error at write time.
		w, s := build(t)
		p0 := s.CreatePoint(0, 0)
		dead := s.CreatePoint(10, 0)
		require.True(t, s.RemovePoint(dead))
		require.GreaterOrEqual(t, dead.ID(), len(s.Points()), "the stale id names no point at all")

		s.CreateLine(dead, p0)
		require.True(t, s.Verify(t.Context()).ForeignHandles, "the oracle flags it before the round trip")

		_, err := json.Marshal(s)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
		_, err = json.Marshal(w)
		require.ErrorIs(t, err, sketch.ErrForeignHandle)
	})

	t.Run("live points and the origin are unaffected", func(t *testing.T) {
		// The negative control. `owns` is a stricter predicate than the sketch-pointer
		// screen it replaces, so it must still accept every LIVE reference — including
		// the origin, which is deliberately absent from s.points and would read as
		// foreign under a bare positional check.
		w, s := build(t)
		gone := s.CreatePoint(99, 99)
		require.True(t, s.RemovePoint(gone), "an unrelated removal renumbers the ids below")
		p := s.CreatePoint(0, 0)
		q := s.CreatePoint(5, 0)
		l := s.CreateLine(p, q)
		origin := s.Origin()
		s.AddConstraint(sketch.NewCoincident(p, origin), sketch.NewHorizontal(l),
			sketch.NewDistance(p, q, 5))
		// The origin as an entity's defining point too, not just a constraint operand.
		s.CreateCircle(origin, 2)
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.False(t, s.Verify(t.Context()).ForeignHandles, "every reference is live")

		data, err := json.Marshal(s)
		require.NoError(t, err)
		var back sketch.Sketch
		require.NoError(t, json.Unmarshal(data, &back))
		again, err := json.Marshal(&back)
		require.NoError(t, err)
		require.JSONEq(t, string(data), string(again), "the reloaded sketch writes the same document")

		wdata, err := json.Marshal(w)
		require.NoError(t, err)
		var wback sketch.World
		require.NoError(t, json.Unmarshal(wdata, &wback))
		wagain, err := json.Marshal(&wback)
		require.NoError(t, err)
		require.JSONEq(t, string(wdata), string(wagain))
	})
}
