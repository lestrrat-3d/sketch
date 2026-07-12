package sketch_test

import (
	"math"
	"sort"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// evalEntityAt evaluates an entity's own curve at the normalized parameter t
// documented on BoundaryEdge.
//
// This is deliberately an INDEPENDENT oracle: it reconstructs the point from the
// entity's public geometry and the documented parameterization, without touching
// the arrangement that produced TStart/TEnd. A test that re-derived the parameter
// the way the arrangement does would pass vacuously even if both were wrong.
func evalEntityAt(t *testing.T, e sketch.Entity, u float64) [2]float64 {
	t.Helper()
	switch c := e.(type) {
	case *sketch.Line:
		g := c.Geometry()
		return [2]float64{
			g.Start.X + u*(g.End.X-g.Start.X),
			g.Start.Y + u*(g.End.Y-g.Start.Y),
		}
	case *sketch.Circle:
		g := c.Geometry()
		a := 2 * math.Pi * u // from the absolute +x axis
		return [2]float64{g.Center.X + g.Radius*math.Cos(a), g.Center.Y + g.Radius*math.Sin(a)}
	case *sketch.Arc:
		g := c.Geometry()
		a := g.StartAngle() + u*g.Sweep()
		r := g.Radius()
		return [2]float64{g.Center.X + r*math.Cos(a), g.Center.Y + r*math.Sin(a)}
	case *sketch.Spline:
		x, y := c.Eval(u)
		return [2]float64{x, y}
	}
	t.Fatalf("no oracle for entity type %T", e)
	return [2]float64{}
}

// requireEdgeParamsConsistent asserts the load-bearing contract of TStart/TEnd on
// one edge: the range is a monotone sub-interval of [0,1], and evaluating the
// entity at those parameters reproduces the fragment's own polyline endpoints
// (swapped when the walk is reversed).
//
// analytic says whether EVERY crossing in the fixture is one the closed-form kernel
// solves (line vs line/circle/arc). It gates the exactness half of the invariant:
// a fragment whose range is a strict sub-interval of [0,1] was bounded by a
// crossing, so it MUST report Partial, and it may only report TExact when that
// crossing was analytic — a sampled crossing wearing an exact flag is precisely the
// false certification this API exists to prevent.
func requireEdgeParamsConsistent(t *testing.T, e sketch.BoundaryEdge, analytic bool) {
	t.Helper()
	require.Less(t, e.TStart, e.TEnd, "TStart must precede TEnd in the natural direction")
	require.GreaterOrEqual(t, e.TStart, -1e-12)
	require.LessOrEqual(t, e.TEnd, 1+1e-12)

	if e.TStart > 1e-9 || e.TEnd < 1-1e-9 {
		require.True(t, e.Partial,
			"a strict sub-range [%v, %v] of %T means the entity was cut — it cannot be whole",
			e.TStart, e.TEnd, e.Entity)
		if !analytic {
			require.False(t, e.TExact,
				"the sub-range [%v, %v] of %T came from a SAMPLED crossing — it must not claim to be exact",
				e.TStart, e.TEnd, e.Entity)
		}
	}

	walkStart := e.Polyline[0]
	walkEnd := e.Polyline[len(e.Polyline)-1]
	if e.Reversed {
		walkStart, walkEnd = walkEnd, walkStart
	}

	// An exact range must land on the curve to machine precision. A sampled one is
	// only as good as the densification, so allow the chord error — but still
	// require it to be the right part of the curve, not merely plausible.
	tol := 1e-9
	if !e.TExact {
		tol = 1e-2
	}
	gotStart := evalEntityAt(t, e.Entity, e.TStart)
	gotEnd := evalEntityAt(t, e.Entity, e.TEnd)
	require.InDelta(t, walkStart[0], gotStart[0], tol, "eval(TStart).x should be the fragment's start")
	require.InDelta(t, walkStart[1], gotStart[1], tol, "eval(TStart).y should be the fragment's start")
	require.InDelta(t, walkEnd[0], gotEnd[0], tol, "eval(TEnd).x should be the fragment's end")
	require.InDelta(t, walkEnd[1], gotEnd[1], tol, "eval(TEnd).y should be the fragment's end")
}

func TestBoundaryEdgeParams(t *testing.T) {
	t.Run("whole entities span the full domain and are exact", func(t *testing.T) {
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		c := s.CreateCircle(s.CreatePoint(0, 0), 5)

		profiles := s.Profiles()
		require.Len(t, profiles, 1)
		require.Len(t, profiles[0].Outer, 1)

		e := profiles[0].Outer[0]
		require.Equal(t, sketch.Entity(c), e.Entity)
		require.False(t, e.Partial, "an uncut circle is whole")
		require.Equal(t, 0.0, e.TStart)
		require.Equal(t, 1.0, e.TEnd)
		require.True(t, e.TExact, "an uncut curve's full domain is exactly known")
		requireEdgeParamsConsistent(t, e, true)
	})

	t.Run("line-line split is exact and lands on the closed-form parameter", func(t *testing.T) {
		// A 10x10 square cut clean in half by a horizontal line through y=5. The
		// two vertical edges are split at exactly t=0.5 — a line/line crossing,
		// which the analytic kernel solves in closed form.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		bl, br := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
		tr, tl := s.CreatePoint(10, 10), s.CreatePoint(0, 10)
		s.CreateLine(bl, br)
		right := s.CreateLine(br, tr)
		s.CreateLine(tr, tl)
		left := s.CreateLine(tl, bl)
		// The cutting line overhangs the square on both sides; the overhangs are
		// dangling spurs and get pruned.
		s.CreateLine(s.CreatePoint(-5, 5), s.CreatePoint(15, 5))

		profiles := s.Profiles()
		require.Len(t, profiles, 2, "the square is cut into two regions")

		var checked int
		for _, p := range profiles {
			for _, e := range p.Outer {
				requireEdgeParamsConsistent(t, e, true)
				require.True(t, e.TExact, "every crossing here is line/line — all exact")

				if e.Entity == sketch.Entity(left) || e.Entity == sketch.Entity(right) {
					require.True(t, e.Partial, "the vertical edges are cut in half")
					// The closed-form answer: the crossing is at the midpoint, so the
					// fragment is either [0,0.5] or [0.5,1] of the vertical edge.
					require.True(t,
						(math.Abs(e.TStart) < 1e-12 && math.Abs(e.TEnd-0.5) < 1e-12) ||
							(math.Abs(e.TStart-0.5) < 1e-12 && math.Abs(e.TEnd-1) < 1e-12),
						"expected an exact half-range, got [%v, %v]", e.TStart, e.TEnd)
					checked++
				}
			}
		}
		require.Equal(t, 4, checked, "both halves of both vertical edges")
	})

	t.Run("a sampled crossing reports TExact false", func(t *testing.T) {
		// Two overlapping circles: a curve/curve crossing, which the arrangement
		// deliberately resolves on the sampled polyline rather than in closed form.
		// The fragments' parameters are therefore approximate and MUST say so.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.CreateCircle(s.CreatePoint(6, 0), 5)

		profiles := s.Profiles()
		require.NotEmpty(t, profiles)

		var partials int
		for _, p := range profiles {
			for _, e := range p.Outer {
				requireEdgeParamsConsistent(t, e, false)
				if e.Partial {
					partials++
					require.False(t, e.TExact,
						"a circle/circle crossing is sampled — its range must not claim to be exact")
				}
			}
		}
		require.NotZero(t, partials, "the two circles must cut each other")
	})

	t.Run("a line crossing a spline is sampled, not exact", func(t *testing.T) {
		// The analytic kernel admits only line/circle/arc operands, so even a plain
		// LINE crossing a spline falls to the sampled path. Both the spline's and
		// the line's fragments must therefore report Partial and TExact = false.
		//
		// This fixture is also the sample-vertex regression: the spline meets the
		// chord at its own t = 0.5, which IS one of the densified sample vertices, so
		// the crossing produces no interior cut on the spline. The split still exists
		// in the arrangement graph, so the spline's fragments must still be reported
		// as cut, with sampled parameters — never as a whole curve carrying an exact
		// half-range.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		sp, err := s.CreateSpline(
			s.CreatePoint(0, 0), s.CreatePoint(3, 8),
			s.CreatePoint(7, -8), s.CreatePoint(10, 0),
		)
		require.NoError(t, err)
		// A chord back from the spline's end to its start closes a region, and the
		// spline crosses it in the middle, cutting both curves in two.
		ln := s.CreateLine(s.CreatePoint(10, 0), s.CreatePoint(0, 0))

		profiles := s.Profiles()
		require.Len(t, profiles, 2, "the crossing cuts the shape into two lobes")

		var splineFrags, lineFrags [][2]float64
		for _, p := range profiles {
			for _, e := range p.Outer {
				requireEdgeParamsConsistent(t, e, false)
				require.True(t, e.Partial,
					"both curves are cut by the crossing, so no fragment is whole")
				require.False(t, e.TExact,
					"a spline crossing is sampled — no fragment's range may claim to be exact")
				switch e.Entity {
				case sketch.Entity(sp):
					splineFrags = append(splineFrags, [2]float64{e.TStart, e.TEnd})
				case sketch.Entity(ln):
					lineFrags = append(lineFrags, [2]float64{e.TStart, e.TEnd})
				default:
					t.Fatalf("unexpected boundary entity %T", e.Entity)
				}
			}
		}
		require.Len(t, splineFrags, 2, "the spline is split into two fragments")
		require.Len(t, lineFrags, 2, "the chord is split into two fragments")

		// Each curve's two fragments meet at the crossing and together cover its
		// whole domain: the halves are strict sub-ranges, not [0,1].
		for _, frags := range [][][2]float64{splineFrags, lineFrags} {
			sort.Slice(frags, func(i, j int) bool { return frags[i][0] < frags[j][0] })
			require.InDelta(t, 0, frags[0][0], 1e-9)
			require.InDelta(t, 1, frags[1][1], 1e-9)
			require.InDelta(t, frags[0][1], frags[1][0], 1e-9, "the halves meet at the crossing")
			require.Greater(t, frags[0][1], 0.1, "neither half is degenerate")
			require.Less(t, frags[0][1], 0.9)
		}
	})

	t.Run("a T-junction on a sample vertex is sampled, not exact", func(t *testing.T) {
		// A chord whose ENDPOINT lands on the spline's interior — at t = 0.5, which is
		// one of the densified sample vertices. Both tiny segments meet at their own
		// endpoints there, so the arrangement takes its join/corner branch — but it is
		// a join only for the line (that is the line's own endpoint); for the spline it
		// is an interior sample vertex, and the spline IS split there in the graph.
		// The split came from a SAMPLED contact, so the spline's fragment must report
		// Partial with a non-exact range, never a whole curve wearing an exact
		// half-range.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		sp, err := s.CreateSpline(
			s.CreatePoint(0, 0), s.CreatePoint(3, 8),
			s.CreatePoint(7, -8), s.CreatePoint(10, 0),
		)
		require.NoError(t, err)
		mx, my := sp.Eval(0.5)
		// From the spline's midpoint back to its start: closes the first lobe. The
		// spline's second half is a dangling spur and gets pruned.
		s.CreateLine(s.CreatePoint(mx, my), s.CreatePoint(0, 0))

		profiles := s.Profiles()
		require.Len(t, profiles, 1, "the chord closes the spline's first lobe")

		var splineEdges int
		for _, e := range profiles[0].Outer {
			requireEdgeParamsConsistent(t, e, false)
			if e.Entity != sketch.Entity(sp) {
				continue
			}
			splineEdges++
			require.True(t, e.Partial,
				"the spline is split at the T-junction — its fragment is not whole")
			require.False(t, e.TExact,
				"the T-junction is a SAMPLED contact — the fragment's range must not claim to be exact")
			require.InDelta(t, 0, e.TStart, 1e-9)
			require.InDelta(t, 0.5, e.TEnd, 1e-9)
		}
		require.Equal(t, 1, splineEdges, "the lobe is bounded by one spline fragment")
	})

	t.Run("an arc fragment's range is in sweep fraction, not absolute angle", func(t *testing.T) {
		// Guards the parameter-domain contract: an arc's t is the fraction of its
		// own sweep (StartAngle + t*Sweep), NOT an absolute angle and NOT a chord
		// fraction. The oracle reconstructs the point from StartAngle/Sweep, so a
		// wrong domain would miss the polyline endpoint.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		c := s.CreatePoint(0, 0)
		a := s.CreateArc(c, s.CreatePoint(5, 0), s.CreatePoint(-5, 0)) // upper half, CCW
		s.CreateLine(s.CreatePoint(-5, 0), s.CreatePoint(5, 0))        // close it

		profiles := s.Profiles()
		require.Len(t, profiles, 1)
		for _, e := range profiles[0].Outer {
			requireEdgeParamsConsistent(t, e, true)
			if e.Entity == sketch.Entity(a) {
				require.False(t, e.Partial)
				require.Equal(t, 0.0, e.TStart)
				require.Equal(t, 1.0, e.TEnd)
				require.True(t, e.TExact)
			}
		}
	})
}
