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
			a := s.AddPoint(1, 2)
			s.Fix(a)
			s.AddConstraint(sketch.NewCoincident(a, s.AddPoint(5, 5)))
		}},
		{"horizontal", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewHorizontal(s.AddLine(a, s.AddPoint(5, 1))))
		}},
		{"vertical", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewVertical(s.AddLine(a, s.AddPoint(1, 5))))
		}},
		{"horizontalPoints", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 4)
			s.Fix(a)
			s.AddConstraint(sketch.NewHorizontalPoints(a, s.AddPoint(6, -2)))
		}},
		{"verticalPoints", func(s *sketch.Sketch) {
			a := s.AddPoint(3, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewVerticalPoints(a, s.AddPoint(-1, 7)))
		}},
		{"parallel", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			c := s.AddPoint(0, 5)
			s.Fix(c)
			s.AddConstraint(sketch.NewParallel(s.AddLine(a, b), s.AddLine(c, s.AddPoint(8, 7))))
		}},
		{"perpendicular", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewPerpendicular(s.AddLine(a, b), s.AddLine(a, s.AddPoint(1, 5))))
		}},
		{"pointOnLine", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewPointOnLine(s.AddPoint(3, 4), s.AddLine(a, b)))
		}},
		{"collinear", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewCollinear(s.AddLine(a, b), s.AddLine(s.AddPoint(2, 3), s.AddPoint(7, 5))))
		}},
		{"pointOnCircle", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			s.Fix(o)
			circ := s.AddCircle(o, 5)
			s.AddConstraint(sketch.NewRadius(circ, 5), sketch.NewPointOnCircle(s.AddPoint(7, 1), circ))
		}},
		{"pointOnArc", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			start := s.AddPoint(5, 0)
			end := s.AddPoint(0, 5)
			s.Fix(o)
			s.Fix(start)
			s.Fix(end)
			arc := s.AddArc(o, start, end)
			s.AddConstraint(sketch.NewPointOnArc(s.AddPoint(3, 3), arc))
		}},
		{"midpoint", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewMidpoint(s.AddPoint(3, 3), s.AddLine(a, b)))
		}},
		{"midpointOf", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 6)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewMidpointOf(s.AddPoint(2, 2), a, b))
		}},
		{"symmetric", func(s *sketch.Sketch) {
			axA := s.AddPoint(0, 0)
			axB := s.AddPoint(0, 10)
			s.Fix(axA)
			s.Fix(axB)
			p1 := s.AddPoint(-3, 4)
			s.Fix(p1)
			s.AddConstraint(sketch.NewSymmetric(p1, s.AddPoint(5, 1), s.AddLine(axA, axB)))
		}},
		{"symmetricLines", func(s *sketch.Sketch) {
			axA := s.AddPoint(0, 0)
			axB := s.AddPoint(0, 10)
			s.Fix(axA)
			s.Fix(axB)
			a := s.AddPoint(2, 1)
			b := s.AddPoint(5, 3)
			s.Fix(a)
			s.Fix(b)
			l1 := s.AddLine(a, b)
			l2 := s.AddLine(s.AddPoint(-2, 1), s.AddPoint(-4, 2))
			s.AddConstraint(sketch.NewSymmetricLines(l1, l2, s.AddLine(axA, axB)))
		}},
		{"symmetricCircles", func(s *sketch.Sketch) {
			axA := s.AddPoint(0, 0)
			axB := s.AddPoint(0, 10)
			s.Fix(axA)
			s.Fix(axB)
			o1 := s.AddPoint(3, 2)
			c1 := s.AddCircle(o1, 4)
			s.FixEntity(c1)
			c2 := s.AddCircle(s.AddPoint(-3, 2), 4)
			s.AddConstraint(sketch.NewSymmetricCircles(c1, c2, s.AddLine(axA, axB)))
		}},
		{"symmetricArcs", func(s *sketch.Sketch) {
			axA := s.AddPoint(0, 0)
			axB := s.AddPoint(1, 0)
			s.Fix(axA)
			s.Fix(axB)
			c1 := s.AddPoint(2, 3)
			st1 := s.AddPoint(3, 3)
			en1 := s.AddPoint(2, 4)
			s.Fix(c1)
			s.Fix(st1)
			s.Fix(en1)
			a1 := s.AddArc(c1, st1, en1)
			a2 := s.AddArc(s.AddPoint(2, -2.8), s.AddPoint(2.1, -3.9), s.AddPoint(2.9, -3.1))
			s.AddConstraint(sketch.NewSymmetricArcs(a1, a2, s.AddLine(axA, axB)))
		}},
		{"concentric", func(s *sketch.Sketch) {
			o1 := s.AddPoint(0, 0)
			s.Fix(o1)
			c1 := s.AddCircle(o1, 5)
			c2 := s.AddCircle(s.AddPoint(3, 2), 4)
			s.AddConstraint(sketch.NewConcentric(c1, c2))
		}},
		{"concentricArcs", func(s *sketch.Sketch) {
			o1 := s.AddPoint(0, 0)
			s.Fix(o1)
			a1 := s.AddArc(o1, s.AddPoint(3, 0), s.AddPoint(0, 3))
			a2 := s.AddArc(s.AddPoint(5, 5), s.AddPoint(7, 5), s.AddPoint(5, 7))
			s.AddConstraint(sketch.NewConcentric(a1, a2))
		}},
		{"equal", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(8, 0)
			s.Fix(a)
			s.Fix(b)
			c := s.AddPoint(20, 0)
			s.Fix(c)
			s.AddConstraint(sketch.NewEqual(s.AddLine(a, b), s.AddLine(c, s.AddPoint(25, 3))))
		}},
		{"equalRadius", func(s *sketch.Sketch) {
			o1 := s.AddPoint(0, 0)
			s.Fix(o1)
			c1 := s.AddCircle(o1, 7)
			o2 := s.AddPoint(20, 0)
			s.Fix(o2)
			c2 := s.AddCircle(o2, 3)
			s.AddConstraint(sketch.NewRadius(c1, 7), sketch.NewEqualRadius(c1, c2))
		}},
		{"pointOnEllipse", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			s.Fix(o)
			e := s.AddEllipse(o, 10, 5, 0)
			s.Fix(e.Center)
			s.AddConstraint(sketch.NewSemiMajor(e, 10), sketch.NewSemiMinor(e, 5), sketch.NewEllipseRotation(e, 0))
			s.AddConstraint(sketch.NewPointOnEllipse(s.AddPoint(12, 1), e))
		}},
		{"tangentLineCircle", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			o := s.AddPoint(5, 5)
			s.Fix(o)
			s.AddConstraint(sketch.NewTangent(s.AddLine(a, b), s.AddCircle(o, 2)))
		}},
		{"tangentCirclesExternal", func(s *sketch.Sketch) {
			o1 := s.AddPoint(0, 0)
			s.Fix(o1)
			c1 := s.AddCircle(o1, 3)
			o2 := s.AddPoint(10, 0)
			s.Fix(o2)
			c2 := s.AddCircle(o2, 2)
			s.AddConstraint(sketch.NewRadius(c1, 3), sketch.NewTangentCircles(c1, c2, false))
		}},
		{"tangentCirclesInternal", func(s *sketch.Sketch) {
			o1 := s.AddPoint(0, 0)
			s.Fix(o1)
			c1 := s.AddCircle(o1, 10)
			o2 := s.AddPoint(4, 0)
			s.Fix(o2)
			c2 := s.AddCircle(o2, 2)
			s.AddConstraint(sketch.NewRadius(c1, 10), sketch.NewTangentCircles(c1, c2, true))
		}},
		{"distance", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewDistance(a, s.AddPoint(4, 1), 5))
		}},
		{"horizontalDistance", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewHorizontalDistance(a, s.AddPoint(3, 1), 4))
		}},
		{"verticalDistance", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewVerticalDistance(a, s.AddPoint(1, 2), 3))
		}},
		{"distancePointLine", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewDistancePointLine(s.AddPoint(3, 2), s.AddLine(a, b), 5))
		}},
		{"distanceLines", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			l2 := s.AddLine(s.AddPoint(0, 3), s.AddPoint(10, 4))
			s.AddConstraint(sketch.NewDistanceLines(s.AddLine(a, b), l2, 6))
		}},
		{"radius", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewRadius(s.AddCircle(o, 3), 7))
		}},
		{"diameter", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewDiameter(s.AddCircle(o, 3), 14))
		}},
		{"radiusOnArc", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewRadius(s.AddArc(o, s.AddPoint(3, 0), s.AddPoint(0, 3)), 7))
		}},
		{"diameterOnArc", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewDiameter(s.AddArc(o, s.AddPoint(3, 0), s.AddPoint(0, 3)), 14))
		}},
		{"arcLength", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			start := s.AddPoint(4, 0)
			s.Fix(o)
			s.Fix(start)
			s.AddConstraint(sketch.NewArcLength(s.AddArc(o, start, s.AddPoint(0, 4)), 3*math.Pi))
		}},
		{"distancePointArc", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			start := s.AddPoint(5, 0)
			end := s.AddPoint(0, 5)
			s.Fix(o)
			s.Fix(start)
			s.Fix(end)
			arc := s.AddArc(o, start, end)
			s.AddConstraint(sketch.NewDistancePointArc(s.AddPoint(7, 7), arc, 2))
		}},
		{"distanceLineArc", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			start := s.AddPoint(5, 0)
			end := s.AddPoint(0, 5)
			s.Fix(o)
			s.Fix(start)
			s.Fix(end)
			arc := s.AddArc(o, start, end)
			s.AddConstraint(sketch.NewDistanceLineArc(s.AddLine(s.AddPoint(10, 0), s.AddPoint(10, 8)), arc, 2))
		}},
		{"angle", func(s *sketch.Sketch) {
			a := s.AddPoint(0, 0)
			b := s.AddPoint(10, 0)
			s.Fix(a)
			s.Fix(b)
			c := s.AddPoint(5, 5)
			l2 := s.AddLine(a, c)
			s.AddConstraint(sketch.NewAngle(s.AddLine(a, b), l2, 45))
			s.AddConstraint(sketch.NewDistance(a, c, 8))
		}},
		{"semiMajorMinorRotation", func(s *sketch.Sketch) {
			o := s.AddPoint(0, 0)
			s.Fix(o)
			e := s.AddEllipse(o, 4, 2, 0.5)
			s.Fix(e.Center)
			s.AddConstraint(sketch.NewSemiMajor(e, 10), sketch.NewSemiMinor(e, 5), sketch.NewEllipseRotation(e, 30))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := sketch.New()
			tc.build(s)
			_, err := s.Solve()
			require.NoError(t, err)

			data, err := json.Marshal(s)
			require.NoError(t, err, "marshal")
			var s2 sketch.Sketch
			require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
			require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint count survives")

			_, err = s2.Solve()
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
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	_, err := s.Solve()
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
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")

	res, err := s2.Solve(sketch.WithMaxIterations(0))
	require.NoError(t, err, "already converged on load")
	require.True(t, res.Converged, "converged without iterating")
	require.Equal(t, 0, res.Iterations, "no iterations spent")
	require.InDelta(t, b.X(), s2.Points()[b.ID()].X(), 1e-12, "coordinates preserved verbatim")
}
