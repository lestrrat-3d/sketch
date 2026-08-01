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
	case *sketch.Ellipse:
		g := c.Geometry()
		a := 2 * math.Pi * u // eccentric angle in the rotated local frame
		lx, ly := g.Rx*math.Cos(a), g.Ry*math.Sin(a)
		cos, sin := math.Cos(g.Rotation), math.Sin(g.Rotation)
		return [2]float64{g.Center.X + lx*cos - ly*sin, g.Center.Y + lx*sin + ly*cos}
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
// solves (BOTH curves a line, circle or arc). It gates the exactness half of the
// invariant: a fragment bounded by a crossing MUST report Partial, and it may only
// report TExact when that crossing was analytic — a sampled crossing wearing an exact
// flag is precisely the false certification this API exists to prevent.
func requireEdgeParamsConsistent(t *testing.T, e sketch.BoundaryEdge, analytic bool) {
	t.Helper()
	require.Less(t, e.TStart, e.TEnd, "TStart must precede TEnd in the natural direction")
	require.GreaterOrEqual(t, e.TStart, -1e-12)
	require.LessOrEqual(t, e.TEnd, 1+1e-12)

	if e.Partial {
		if !analytic {
			require.False(t, e.TExact,
				"the fragment [%v, %v] of %T was bounded by a SAMPLED crossing — it must not claim to be exact",
				e.TStart, e.TEnd, e.Entity)
		}
	} else {
		// A NON-partial edge claims to be the whole entity, so it must report the
		// entity's full domain EXACTLY — no epsilon on either side. The engine decides
		// Whole structurally (both bounds are the entity's own domain ends, a fact it
		// knows when it builds the boundary), so a whole edge's range is the domain's
		// own parameters bit-for-bit. Asserting that with zero tolerance is what makes
		// this test independent of the implementation: a helper that re-used the
		// engine's own numeric gate could not catch the engine's gate being wrong, and
		// an edge bounded by a crossing at t = 5e-10 reporting Partial = false is
		// exactly the false certification we are hunting.
		require.Equal(t, 0.0, e.TStart,
			"a whole edge of %T must start at the entity's own domain start", e.Entity)
		require.Equal(t, 1.0, e.TEnd,
			"a whole edge of %T must end at the entity's own domain end", e.Entity)
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

	t.Run("a circle/circle crossing is exact", func(t *testing.T) {
		// SUPERSEDED EXPECTATION. This fixture used to assert the opposite — that a
		// curve/curve crossing is resolved on the sampled polyline and so must report
		// TExact = false. A curve/curve transverse crossing now takes analytic
		// authority whenever the arrangement can certify that splicing the exact
		// crossing points into both polylines leaves the sampled topology unchanged,
		// which it can here. The general "a sampled crossing must not claim to be
		// exact" contract is unchanged and is still covered, by the line/spline
		// fixture below: the analytic kernel admits only line/circle/arc operands, so
		// everything involving an ellipse, conic or spline is still sampled.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.CreateCircle(s.CreatePoint(6, 0), 5)

		profiles := s.Profiles()
		require.NotEmpty(t, profiles)

		// The circles cross at (3, ±4). Every fragment bound must therefore evaluate
		// either to one of those two points — the closed-form answer, not a value that
		// merely converges to it with sampling — or to the circle's own seam.
		onExactBound := func(e sketch.BoundaryEdge, u float64) bool {
			if u == 0 || u == 1 {
				return true // the circle's own domain end (its seam)
			}
			q := evalEntityAt(t, e.Entity, u)
			return math.Abs(q[0]-3) < 1e-12 && math.Abs(math.Abs(q[1])-4) < 1e-12
		}

		var partials int
		for _, p := range profiles {
			for _, e := range p.Outer {
				requireEdgeParamsConsistent(t, e, true)
				if !e.Partial {
					continue
				}
				partials++
				require.True(t, e.TExact,
					"a certified circle/circle crossing is analytic — its range is exact")
				require.Truef(t, onExactBound(e, e.TStart) && onExactBound(e, e.TEnd),
					"fragment [%v, %v] must be bounded by the closed-form crossing points", e.TStart, e.TEnd)
			}
		}
		require.NotZero(t, partials, "the two circles must cut each other")
	})

	t.Run("a free-form entity withholds exact bounds across the whole sketch", func(t *testing.T) {
		// The closed-form kernel never classifies a contact involving a spline (or an
		// ellipse, elliptical arc, conic or NURBS), and nothing bounds how far such a
		// curve runs from the chords it is sampled into — so it can cross another entity
		// entirely between two samples, leaving the profile set fused while the certified
		// circle/circle crossing publishes exact bounds describing it.
		//
		// So the engine withholds exactness for the whole sketch whenever any free-form
		// entity is present: every BoundaryEdge reports TExact = false, the ones bounded
		// by the certified circle crossing included, and however far the free-form entity
		// sits from them. Only the flag is withheld — the profiles, their areas and their
		// ranges are what they were.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.CreateCircle(s.CreatePoint(6, 0), 5)
		_, err = s.CreateSpline(
			s.CreatePoint(40, 0), s.CreatePoint(42, 4),
			s.CreatePoint(46, 4), s.CreatePoint(48, 0),
		)
		require.NoError(t, err)

		profiles := s.Profiles()
		require.Len(t, profiles, 3, "two lune caps plus the lens; the spline bounds nothing")
		var edges int
		for _, p := range profiles {
			for _, e := range p.Outer {
				requireEdgeParamsConsistent(t, e, false)
				require.Falsef(t, e.TExact,
					"the %T fragment [%v, %v] claims exactness in a sketch holding a spline",
					e.Entity, e.TStart, e.TEnd)
				edges++
			}
		}
		require.NotZero(t, edges, "the circles must bound the regions")

		// The control: the identical circles WITHOUT the spline keep their exact bounds,
		// so what withholds exactness is the free-form entity, not the crossing.
		w2 := sketch.NewWorld()
		s2, err := w2.CreateSketch(w2.XY())
		require.NoError(t, err)
		s2.CreateCircle(s2.CreatePoint(0, 0), 5)
		s2.CreateCircle(s2.CreatePoint(6, 0), 5)
		for _, p := range s2.Profiles() {
			for _, e := range p.Outer {
				require.Truef(t, e.TExact,
					"an all line/circle/arc sketch keeps its exact bounds: %T [%v, %v]",
					e.Entity, e.TStart, e.TEnd)
			}
		}
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

	t.Run("a distance-weld with no analytic event is not exact", func(t *testing.T) {
		// A line whose endpoints sit 5e-7 INSIDE a circle of radius 5. The true
		// line/circle intersections are at (0, ±5) — just outside the segment — so the
		// analytic kernel, which is authoritative for a line/circle pair, finds no
		// crossing and records no cut. But the vertex table welds by DISTANCE (the
		// merge tolerance is scale·1e-7 = 1e-6 here), so each line endpoint still
		// canonicalizes onto the circle's sample vertex at (0, ±5) and the circle IS
		// split there in the graph.
		//
		// That split is a distance weld nothing exact explains. Its fragments may be
		// reported — the topology is what it is — but they must never wear TExact over
		// a strict sub-range: an "exact" fragment of a circle bounded by a weld is the
		// false certification this flag exists to prevent.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.CreateLine(s.CreatePoint(0, 5-5e-7), s.CreatePoint(0, -5+5e-7))

		profiles := s.Profiles()
		require.NotEmpty(t, profiles)
		for _, p := range profiles {
			for _, e := range p.Outer {
				// analytic=false: the weld is NOT an analytic event, so no fragment it
				// bounds may claim exactness.
				requireEdgeParamsConsistent(t, e, false)
			}
		}
	})

	t.Run("a pruned contact leaves the curve whole", func(t *testing.T) {
		// An ellipse plus a DANGLING line: one endpoint sits exactly on the ellipse's
		// sample vertex at (0,3), the other floats free. The line bounds nothing, so
		// pruning drops it and the ellipse is left as a single, complete boundary.
		//
		// Partial must follow the edge that actually survives: the ellipse is covered
		// whole, over [0,1]. A per-source "was this curve touched anywhere" flag would
		// outlive the pruned contact and report a phantom Partial on a whole curve.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		el := s.CreateEllipse(s.CreatePoint(0, 0), 5, 3, 0)
		s.CreateLine(s.CreatePoint(0, 3), s.CreatePoint(0, 8))

		profiles := s.Profiles()
		require.Len(t, profiles, 1, "only the ellipse bounds a region; the spur is pruned")
		require.Len(t, profiles[0].Outer, 1, "the ellipse is one boundary edge")

		e := profiles[0].Outer[0]
		requireEdgeParamsConsistent(t, e, false)
		require.Equal(t, sketch.Entity(el), e.Entity)
		require.False(t, e.Partial, "the ellipse is covered whole — its contact was pruned away")
		require.InDelta(t, 0.0, e.TStart, 1e-12)
		require.InDelta(t, 1.0, e.TEnd, 1e-12)
		require.InDelta(t, math.Pi*5*3, profiles[0].Area, 1e-3, "the whole ellipse's area")
	})

	t.Run("a crossing just inside the seam still cuts the curve", func(t *testing.T) {
		// A near-vertical line grazing the ellipse's right vertex — its seam, t = 0.
		// The two sampled crossings land a hair either side of it, at t ~ 5e-10 and
		// t ~ 1 - 5e-10, so the surviving fragment covers all but two invisible slivers
		// of the domain.
		//
		// It is STILL a fragment: both its bounds are sampled crossings, not the
		// curve's own ends. Whole must be decided by that provenance, never by asking
		// whether the range comes numerically close to [0,1] — no float compare can
		// tell "this bound IS the seam" from "this bound is a crossing that landed
		// 5e-10 away from it", and answering "whole" is the unsafe way to be wrong: the
		// consumer would record a sampled-bounded fragment as a certified whole curve.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		el := s.CreateEllipse(s.CreatePoint(0, 0), 5, 3, 0)
		s.CreateLine(s.CreatePoint(5-2e-10, -2), s.CreatePoint(5-2e-10, 2))

		profiles := s.Profiles()
		require.NotEmpty(t, profiles)

		var ellipseEdges int
		for _, p := range profiles {
			for _, e := range p.Outer {
				requireEdgeParamsConsistent(t, e, false)
				if e.Entity != sketch.Entity(el) {
					continue
				}
				ellipseEdges++
				require.True(t, e.Partial,
					"the ellipse is cut at t=[%v, %v] — sampled crossings, so it is NOT whole",
					e.TStart, e.TEnd)
				require.False(t, e.TExact, "a sampled crossing's range is not exact")
			}
		}
		require.NotZero(t, ellipseEdges, "the ellipse must bound a region")
	})

	t.Run("a line tangent to an ellipse is sampled, not exact", func(t *testing.T) {
		// The closed-form kernel runs only when BOTH curves are a line, circle or arc —
		// it is a rule about the PAIR, not about "a line was involved" and not about
		// the contact being a tangency. A line TANGENT to an ellipse is therefore
		// resolved on the sampled polyline like any other ellipse contact.
		//
		// Here a box's top edge is tangent to the ellipse at its top vertex (0,3),
		// which is one of the ellipse's own sample vertices. The contact splits both
		// curves in the graph (the box-with-a-hole region is bounded by two ellipse
		// fragments and two halves of the top edge). Neither may claim exactness: the
		// tangency was found by chords, not solved in closed form.
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		el := s.CreateEllipse(s.CreatePoint(0, 0), 5, 3, 0)
		tl, tr := s.CreatePoint(-8, 3), s.CreatePoint(8, 3)
		br, bl := s.CreatePoint(8, -6), s.CreatePoint(-8, -6)
		top := s.CreateLine(tl, tr) // tangent to the ellipse at (0, 3)
		s.CreateLine(tr, br)
		s.CreateLine(br, bl)
		s.CreateLine(bl, tl)

		profiles := s.Profiles()
		require.NotEmpty(t, profiles)

		var tangentFrags int
		for _, p := range profiles {
			for _, e := range p.Outer {
				requireEdgeParamsConsistent(t, e, false)
				if !e.Partial {
					continue
				}
				require.Contains(t, []sketch.Entity{el, top}, e.Entity,
					"only the tangent pair is cut")
				require.False(t, e.TExact,
					"a line/ellipse TANGENCY is sampled, not analytic — the fragment [%v, %v] of %T must not claim to be exact",
					e.TStart, e.TEnd, e.Entity)
				tangentFrags++
			}
		}
		require.NotZero(t, tangentFrags, "the tangency must cut the ellipse and the top edge")
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
