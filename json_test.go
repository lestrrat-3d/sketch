package sketch_test

import (
	"encoding/json"
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
			a := addPt(s, 1, 2)
			s.Fix(a)
			s.AddConstraint(sketch.NewCoincident(a, addPt(s, 5, 5)))
		}},
		{"horizontal", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewHorizontal(addLn(s, a, addPt(s, 5, 1))))
		}},
		{"vertical", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewVertical(addLn(s, a, addPt(s, 1, 5))))
		}},
		{"parallel", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			c := addPt(s, 0, 5)
			s.Fix(c)
			s.AddConstraint(sketch.NewParallel(addLn(s, a, b), addLn(s, c, addPt(s, 8, 7))))
		}},
		{"perpendicular", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewPerpendicular(addLn(s, a, b), addLn(s, a, addPt(s, 1, 5))))
		}},
		{"pointOnLine", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewPointOnLine(addPt(s, 3, 4), addLn(s, a, b)))
		}},
		{"collinear", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewCollinear(addLn(s, a, b), addLn(s, addPt(s, 2, 3), addPt(s, 7, 5))))
		}},
		{"pointOnCircle", func(s *sketch.Sketch) {
			o := addPt(s, 0, 0)
			s.Fix(o)
			circ := addCir(s, o, 5)
			s.AddConstraint(sketch.NewRadius(circ, 5), sketch.NewPointOnCircle(addPt(s, 7, 1), circ))
		}},
		{"midpoint", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewMidpoint(addPt(s, 3, 3), addLn(s, a, b)))
		}},
		{"symmetric", func(s *sketch.Sketch) {
			axA := addPt(s, 0, 0)
			axB := addPt(s, 0, 10)
			s.Fix(axA)
			s.Fix(axB)
			p1 := addPt(s, -3, 4)
			s.Fix(p1)
			s.AddConstraint(sketch.NewSymmetric(p1, addPt(s, 5, 1), addLn(s, axA, axB)))
		}},
		{"concentric", func(s *sketch.Sketch) {
			o1 := addPt(s, 0, 0)
			s.Fix(o1)
			c1 := addCir(s, o1, 5)
			c2 := addCir(s, addPt(s, 3, 2), 4)
			s.AddConstraint(sketch.NewConcentric(c1, c2))
		}},
		{"equal", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 8, 0)
			s.Fix(a)
			s.Fix(b)
			c := addPt(s, 20, 0)
			s.Fix(c)
			s.AddConstraint(sketch.NewEqual(addLn(s, a, b), addLn(s, c, addPt(s, 25, 3))))
		}},
		{"equalRadius", func(s *sketch.Sketch) {
			o1 := addPt(s, 0, 0)
			s.Fix(o1)
			c1 := addCir(s, o1, 7)
			o2 := addPt(s, 20, 0)
			s.Fix(o2)
			c2 := addCir(s, o2, 3)
			s.AddConstraint(sketch.NewRadius(c1, 7), sketch.NewEqualRadius(c1, c2))
		}},
		{"pointOnEllipse", func(s *sketch.Sketch) {
			o := addPt(s, 0, 0)
			s.Fix(o)
			e := addEl(s, o, 10, 5, 0)
			pinEllipse(s, e, 10, 5, 0)
			s.AddConstraint(sketch.NewPointOnEllipse(addPt(s, 12, 1), e))
		}},
		{"tangentLineCircle", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			o := addPt(s, 5, 5)
			s.Fix(o)
			s.AddConstraint(sketch.NewTangent(addLn(s, a, b), addCir(s, o, 2)))
		}},
		{"tangentCirclesExternal", func(s *sketch.Sketch) {
			o1 := addPt(s, 0, 0)
			s.Fix(o1)
			c1 := addCir(s, o1, 3)
			o2 := addPt(s, 10, 0)
			s.Fix(o2)
			c2 := addCir(s, o2, 2)
			s.AddConstraint(sketch.NewRadius(c1, 3), sketch.NewTangentCircles(c1, c2, false))
		}},
		{"tangentCirclesInternal", func(s *sketch.Sketch) {
			o1 := addPt(s, 0, 0)
			s.Fix(o1)
			c1 := addCir(s, o1, 10)
			o2 := addPt(s, 4, 0)
			s.Fix(o2)
			c2 := addCir(s, o2, 2)
			s.AddConstraint(sketch.NewRadius(c1, 10), sketch.NewTangentCircles(c1, c2, true))
		}},
		{"distance", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			s.Fix(a)
			addDist(s, a, addPt(s, 4, 1), 5)
		}},
		{"horizontalDistance", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewHorizontalDistance(a, addPt(s, 3, 1), 4))
		}},
		{"verticalDistance", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			s.Fix(a)
			s.AddConstraint(sketch.NewVerticalDistance(a, addPt(s, 1, 2), 3))
		}},
		{"distancePointLine", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewDistancePointLine(addPt(s, 3, 2), addLn(s, a, b), 5))
		}},
		{"distanceLines", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			l2 := addLn(s, addPt(s, 0, 3), addPt(s, 10, 4))
			s.AddConstraint(sketch.NewDistanceLines(addLn(s, a, b), l2, 6))
		}},
		{"radius", func(s *sketch.Sketch) {
			o := addPt(s, 0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewRadius(addCir(s, o, 3), 7))
		}},
		{"diameter", func(s *sketch.Sketch) {
			o := addPt(s, 0, 0)
			s.Fix(o)
			s.AddConstraint(sketch.NewDiameter(addCir(s, o, 3), 14))
		}},
		{"angle", func(s *sketch.Sketch) {
			a := addPt(s, 0, 0)
			b := addPt(s, 10, 0)
			s.Fix(a)
			s.Fix(b)
			c := addPt(s, 5, 5)
			l2 := addLn(s, a, c)
			s.AddConstraint(sketch.NewAngle(addLn(s, a, b), l2, 45))
			addDist(s, a, c, 8)
		}},
		{"semiMajorMinorRotation", func(s *sketch.Sketch) {
			o := addPt(s, 0, 0)
			s.Fix(o)
			e := addEl(s, o, 4, 2, 0.5)
			pinEllipse(s, e, 10, 5, 30)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := sketch.New()
			tc.build(s)
			mustSolve(t, s)

			data, err := json.Marshal(s)
			require.NoError(t, err, "marshal")
			var s2 sketch.Sketch
			require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
			require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint count survives")

			mustSolve(t, &s2)
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
	s, _, _, _, _ := newRectangle(t)
	mustSolve(t, s)

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
	s, _, b, _, _ := newRectangle(t)
	mustSolve(t, s)

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
