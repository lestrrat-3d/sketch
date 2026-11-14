package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// requireFullyConstrained solves s and asserts the whole Verify verdict, not
// just DOF: a stray constraint row shows up only in Redundant.
func requireFullyConstrained(t *testing.T, s *sketch.Sketch) {
	t.Helper()
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	rep := s.Verify(t.Context())
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.Zero(t, rep.DOF)
	require.Empty(t, rep.Redundant)
	require.Empty(t, rep.Conflicts)
}

func countArcs(s *sketch.Sketch) int {
	n := 0
	for _, e := range s.Entities() {
		if _, ok := e.(*sketch.Arc); ok {
			n++
		}
	}
	return n
}

// The result of CreateFillet must not depend on the argument order, the
// corner's convexity or the loop direction.
func TestCreateFilletLegOrder(t *testing.T) {
	ccw := [][2]float64{{5, 5}, {25, 5}, {25, 15}, {15, 15}, {15, 25}, {5, 25}}
	cw := [][2]float64{{5, 25}, {15, 25}, {15, 15}, {25, 15}, {25, 5}, {5, 5}}
	for _, loop := range []struct {
		name string
		pts  [][2]float64
	}{{"ccw", ccw}, {"cw", cw}} {
		for _, corner := range []struct {
			name string
			at   [2]float64
		}{{"convex", [2]float64{25, 5}}, {"concave", [2]float64{15, 15}}} {
			for _, swap := range []bool{false, true} {
				name := loop.name + "/" + corner.name + "/incoming-first"
				if swap {
					name = loop.name + "/" + corner.name + "/outgoing-first"
				}
				t.Run(name, func(t *testing.T) {
					s := newSketch(t)
					pts := make([]*sketch.Point, len(loop.pts))
					for i, v := range loop.pts {
						pts[i] = s.CreatePoint(v[0], v[1])
					}
					lines := make([]*sketch.Line, len(pts))
					for i := range pts {
						l := s.CreateLine(pts[i], pts[(i+1)%len(pts)])
						lines[i] = l
						a, b := loop.pts[i], loop.pts[(i+1)%len(pts)]
						// Every edge is placed without using a corner, so any corner can go.
						if a[0] == b[0] {
							s.AddConstraint(sketch.NewVertical(l), sketch.NewDistancePointLine(s.Origin(), l, a[0]))
							continue
						}
						s.AddConstraint(sketch.NewHorizontal(l), sketch.NewDistancePointLine(s.Origin(), l, a[1]))
					}
					requireFullyConstrained(t, s)

					ci := -1
					for i, v := range loop.pts {
						if v == corner.at {
							ci = i
						}
					}
					require.GreaterOrEqual(t, ci, 0)
					in, out := lines[(ci+len(pts)-1)%len(pts)], lines[ci]
					if swap {
						in, out = out, in
					}
					_, err := sketch.CreateFillet(s, in, out, pts[ci], 2)
					require.NoError(t, err)

					requireFullyConstrained(t, s)
					require.Len(t, s.Entities(), 7, "six edges and one fillet arc")
					require.Equal(t, 1, countArcs(s), "no stray arc")
					for _, e := range s.Entities() {
						if a, ok := e.(*sketch.Arc); ok {
							require.Less(t, a.Sweep(), math.Pi, "the fillet is the minor arc")
						}
					}
					require.Len(t, s.Profiles(), 1)
				})
			}
		}
	}
}

// Corners that are not square, turning either way.
func TestCreateFilletLegOrderAngles(t *testing.T) {
	for _, deg := range []float64{60, 90, 120, -60, -90, -120} {
		for _, swap := range []bool{false, true} {
			name := "turn"
			if swap {
				name += "-swapped"
			}
			t.Run(name+"/"+formatDeg(deg), func(t *testing.T) {
				rad := deg * math.Pi / 180
				bx, by := 10+10*math.Cos(rad), 10*math.Sin(rad)
				s := newSketch(t)
				a, corner, b := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(bx, by)
				l1, l2 := s.CreateLine(a, corner), s.CreateLine(corner, b)
				s.AddConstraint(
					sketch.NewCoincident(a, s.Origin()), sketch.NewHorizontal(l1),
					sketch.NewHorizontalDistance(s.Origin(), b, bx), sketch.NewVerticalDistance(s.Origin(), b, by),
					sketch.NewAngle(l1, l2, deg),
				)
				first, second := l1, l2
				if swap {
					first, second = l2, l1
				}
				_, err := sketch.CreateFillet(s, first, second, corner, 2)
				require.NoError(t, err)
				requireFullyConstrained(t, s)
				require.Len(t, s.Entities(), 3)
			})
		}
	}
}

func formatDeg(d float64) string {
	if d < 0 {
		return "minus" + formatDeg(-d)
	}
	return string(rune('0'+int(d)/100)) + string(rune('0'+int(d)/10%10)) + string(rune('0'+int(d)%10))
}

// Line-arc and arc-arc legs, both argument orders.
func TestCreateFilletArcLegOrder(t *testing.T) {
	for _, swap := range []bool{false, true} {
		name := "in-order"
		if swap {
			name = "swapped"
		}
		t.Run("line-arc/"+name, func(t *testing.T) {
			s := newSketch(t)
			center, corner, far := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(0, 10)
			arc := s.CreateArc(center, corner, far)
			lineFar := s.CreatePoint(20, 0)
			line := s.CreateLine(corner, lineFar)
			s.Fix(center)
			s.Fix(far)
			s.Fix(lineFar)
			s.AddConstraint(sketch.NewHorizontal(line))
			var err error
			if swap {
				_, err = sketch.CreateFillet(s, arc, line, corner, 2)
			} else {
				_, err = sketch.CreateFillet(s, line, arc, corner, 2)
			}
			require.NoError(t, err)
			requireFullyConstrained(t, s)
			require.Equal(t, 2, countArcs(s))
		})
		t.Run("arc-arc/"+name, func(t *testing.T) {
			s := newSketch(t)
			corner := s.CreatePoint(10, 0)
			c1, e1 := s.CreatePoint(0, 0), s.CreatePoint(0, 10)
			a1 := s.CreateArc(c1, corner, e1)
			h := 10 - 5*math.Sqrt2
			c2, e2 := s.CreatePoint(10, 10), s.CreatePoint(h, h)
			a2 := s.CreateArc(c2, e2, corner)
			// Both centers and both far ends are held, so only the fillet is free.
			s.Fix(c1)
			s.Fix(e1)
			s.Fix(c2)
			s.Fix(e2)
			var err error
			if swap {
				_, err = sketch.CreateFillet(s, a2, a1, corner, 2)
			} else {
				_, err = sketch.CreateFillet(s, a1, a2, corner, 2)
			}
			require.NoError(t, err)
			requireFullyConstrained(t, s)
			require.Equal(t, 3, countArcs(s))
		})
	}
}

// A fillet whose far ends are fixed leaves only the fillet's own freedom, and
// the radius dimension must be the one row that removes it.
func TestCreateFilletFixedEnds(t *testing.T) {
	for _, swap := range []bool{false, true} {
		t.Run(map[bool]string{false: "in-order", true: "swapped"}[swap], func(t *testing.T) {
			s := newSketch(t)
			a, corner, b := s.CreatePoint(0, 10), s.CreatePoint(0, 0), s.CreatePoint(10, 0)
			l1, l2 := s.CreateLine(a, corner), s.CreateLine(corner, b)
			first, second := l1, l2
			if swap {
				first, second = l2, l1
			}
			f, err := sketch.CreateFillet(s, first, second, corner, 3)
			require.NoError(t, err)
			s.Fix(a)
			s.Fix(b)
			s.AddConstraint(sketch.NewVertical(l1), sketch.NewHorizontal(l2))
			requireFullyConstrained(t, s)
			require.Len(t, s.Entities(), 3)
			require.InDelta(t, 3, f.Arc.R(), 1e-6)
		})
	}
}

// Editing the fillet radius keeps the radius dimension and both tangencies
// independent, on a concave corner in either order.
func TestCreateFilletRadiusEdit(t *testing.T) {
	for _, swap := range []bool{false, true} {
		t.Run(map[bool]string{false: "in-order", true: "swapped"}[swap], func(t *testing.T) {
			s := newSketch(t)
			v := [][2]float64{{5, 5}, {25, 5}, {25, 15}, {15, 15}, {15, 25}, {5, 25}}
			pts := make([]*sketch.Point, len(v))
			for i, p := range v {
				pts[i] = s.CreatePoint(p[0], p[1])
			}
			lines := make([]*sketch.Line, len(v))
			for i := range v {
				l := s.CreateLine(pts[i], pts[(i+1)%len(v)])
				lines[i] = l
				a, b := v[i], v[(i+1)%len(v)]
				if a[0] == b[0] {
					s.AddConstraint(sketch.NewVertical(l), sketch.NewDistancePointLine(s.Origin(), l, a[0]))
					continue
				}
				s.AddConstraint(sketch.NewHorizontal(l), sketch.NewDistancePointLine(s.Origin(), l, a[1]))
			}
			in, out := lines[2], lines[3] // the concave corner (15,15)
			if swap {
				in, out = out, in
			}
			f, err := sketch.CreateFillet(s, in, out, pts[3], 2)
			require.NoError(t, err)

			f.Radius.Set(3)
			requireFullyConstrained(t, s)
			require.InDelta(t, 3, f.Arc.R(), 1e-6)
			for _, c := range s.Constraints() {
				for _, r := range sketch.ConstraintResiduals(c) {
					require.InDelta(t, 0, r, 1e-9)
				}
			}
		})
	}
}

// Control: a round whose two ends are pinned on one ray already has its radius
// fixed by them, so a radius dimension on it is a genuine redundancy. This is
// the slot-end report that is not a CreateFillet defect.
func TestArcEndsOnOneRayRadiusIsRedundant(t *testing.T) {
	build := func(t *testing.T) (*sketch.Sketch, *sketch.Arc) {
		s := newSketch(t)
		o, p1, end, p2 := s.CreatePoint(0, 0), s.CreatePoint(7.5, 0), s.CreatePoint(0, 7.5), s.CreatePoint(9, 0)
		core := s.CreateArc(o, p1, end)
		round := s.CreateArc(s.CreatePoint(8.25, 0), p1, p2)
		s.AddConstraint(
			sketch.NewCoincident(o, s.Origin()),
			sketch.NewHorizontalDistance(s.Origin(), p1, 7.5), sketch.NewVerticalDistance(s.Origin(), p1, 0),
			sketch.NewHorizontalDistance(s.Origin(), p2, 9), sketch.NewVerticalDistance(s.Origin(), p2, 0),
			sketch.NewHorizontalDistance(s.Origin(), end, 0),
			sketch.NewTangentCircles(core, round, false),
		)
		return s, round
	}

	t.Run("without radius", func(t *testing.T) {
		s, _ := build(t)
		requireFullyConstrained(t, s)
	})
	t.Run("with radius", func(t *testing.T) {
		s, round := build(t)
		rad := sketch.NewRadius(round, 0.75)
		s.AddConstraint(rad)
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		rep := s.Verify(t.Context())
		require.Equal(t, sketch.Overconstrained, rep.Status)
		require.Len(t, rep.Redundant, 1)
		require.Same(t, rad, rep.Redundant[0])
	})
}
