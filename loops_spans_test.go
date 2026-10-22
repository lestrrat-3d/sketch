package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// lobeSlots is the number of evenly spaced places a lobe can take about the origin.
const lobeSlots = 8

// lobeScene builds a root circle (the first entity) and one lobe in each of the
// first count slots. A lobe is a chord, two fit splines of fitPoints points that start
// inside the circle and cross it once, and a tip arc about the origin. Slot k of a
// scene with more lobes holds the same coordinates as slot k of a scene with fewer,
// and the entities come in the same order, so entity i of one scene is entity i of
// the other for every slot both hold.
func lobeScene(t testing.TB, count, fitPoints int) *sketch.Sketch {
	t.Helper()
	const (
		rootRadius = 3.0
		innerR     = 2.6
		outerR     = 4.0
	)
	world := sketch.NewWorld()
	s, err := world.CreateSketch(world.XY())
	require.NoError(t, err)
	center := s.Origin()
	circle := s.CreateCircle(center, rootRadius)
	s.AddConstraint(sketch.NewRadius(circle, rootRadius))
	pitch := math.Pi / lobeSlots
	for k := range count {
		rot := 2 * math.Pi * float64(k) / lobeSlots
		point := func(r, a float64) *sketch.Point {
			p := s.CreatePoint(r*math.Cos(a+rot), r*math.Sin(a+rot))
			s.Fix(p)
			return p
		}
		a := 0.4 * pitch
		right := make([]*sketch.Point, fitPoints)
		left := make([]*sketch.Point, fitPoints)
		for i := range fitPoints {
			f := float64(i) / float64(fitPoints-1)
			r := innerR + f*(outerR-innerR)
			half := a * (1 - 0.4*f)
			right[i], left[i] = point(r, -half), point(r, half)
		}
		s.CreateLine(left[0], right[0])
		_, err := s.CreateFitSpline(right...)
		require.NoError(t, err)
		s.CreateArc(center, right[fitPoints-1], left[fitPoints-1])
		_, err = s.CreateFitSpline(left...)
		require.NoError(t, err)
	}
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	return s
}

// outlineSpans reads the outer boundary of a union as the spans a loop in another
// sketch declares. from and to hold the same entities in the same order.
func outlineSpans(t testing.TB, from, to *sketch.Sketch, u *sketch.Profile) []sketch.LoopSpan {
	t.Helper()
	index := map[sketch.Entity]int{}
	for i, e := range from.Entities() {
		index[e] = i
	}
	spans := make([]sketch.LoopSpan, 0, len(u.Outer))
	for _, e := range u.Outer {
		i, ok := index[e.Entity]
		require.True(t, ok)
		spans = append(spans, sketch.LoopSpan{Entity: to.Entities()[i], TStart: e.TStart, TEnd: e.TEnd})
	}
	return spans
}

func unionOfAll(t testing.TB, s *sketch.Sketch) *sketch.Profile {
	t.Helper()
	valid := validProfileIndices(s)
	u, err := s.UnionProfiles(valid...)
	require.NoError(t, err)
	return u
}

// TestDeclareLoopSpansFollowsAUnionOutline declares, in a larger scene, the loop that
// is the outline of the union of the disc and the first three lobes of a smaller
// scene, as spans of the circle and of the lobes' splines. Every face of the first
// three lobes and the disc lies inside it, and the fourth lobe's cap lies outside.
// The fourth lobe then joins the union through the verdicts, with exact fragments.
func TestDeclareLoopSpansFollowsAUnionOutline(t *testing.T) {
	for _, fit := range []int{4, 15} {
		small := lobeScene(t, 3, fit)
		large := lobeScene(t, 4, fit)
		first := unionOfAll(t, small)
		require.Len(t, first.Outer, 12)

		loop, err := large.DeclareLoopSpans(outlineSpans(t, small, large, first)...)
		require.NoError(t, err)
		profiles := large.Profiles()
		var inside, outside []int
		for i, p := range profiles {
			require.Truef(t, p.Valid, "profile %d", i)
			c := p.Containment(loop)
			switch c.State {
			case sketch.ContainmentInside:
				inside = append(inside, i)
			case sketch.ContainmentOutside:
				outside = append(outside, i)
			default:
				t.Fatalf("fit=%d profile %d: %+v", fit, i, c)
			}
		}
		// The disc's middle, and a cap and an inner piece for each of three lobes, are
		// inside; the fourth lobe's inner piece is inside the disc, and only its cap is out.
		require.Len(t, profiles, 9)
		require.Len(t, outside, 1)
		require.Len(t, inside, 8)

		joined, err := large.UnionProfiles(append(inside, outside...)...)
		require.NoError(t, err)
		require.Len(t, joined.Outer, 16)
		for _, e := range joined.Outer {
			if e.Partial {
				require.Truef(t, e.TExact, "%T t=[%v %v]", e.Entity, e.TStart, e.TEnd)
			}
		}
	}
}

func TestDeclareLoopSpansAcceptsReversedSpans(t *testing.T) {
	small := lobeScene(t, 3, 4)
	large := lobeScene(t, 4, 4)
	spans := outlineSpans(t, small, large, unionOfAll(t, small))
	for i := range spans {
		spans[i].TStart, spans[i].TEnd = spans[i].TEnd, spans[i].TStart
	}
	loop, err := large.DeclareLoopSpans(spans...)
	require.NoError(t, err)
	for _, p := range large.Profiles() {
		require.NotEqual(t, sketch.ContainmentUnproven, p.Containment(loop).State)
	}
	for _, sp := range loop.Spans() {
		require.Less(t, sp.TStart, sp.TEnd, "a span reads in natural order")
	}
}

// TestDeclareLoopSpansRefusesWhatItCannotPlace pins the refusals: a span end that is
// not a bound of the arrangement, a missing element, and a loop that is two cycles.
func TestDeclareLoopSpansRefusesWhatItCannotPlace(t *testing.T) {
	small := lobeScene(t, 3, 4)
	large := lobeScene(t, 4, 4)
	good := outlineSpans(t, small, large, unionOfAll(t, small))

	var partialAt int
	for i, sp := range good {
		if sp.TStart > 0 {
			partialAt = i
			break
		}
	}
	allUnproven := func(t *testing.T, spans []sketch.LoopSpan, reason sketch.ContainmentReason) {
		t.Helper()
		loop, err := large.DeclareLoopSpans(spans...)
		require.NoError(t, err)
		for i, p := range large.Profiles() {
			c := p.Containment(loop)
			require.Equalf(t, sketch.ContainmentUnproven, c.State, "profile %d", i)
			if reason != sketch.ReasonNone {
				require.Equalf(t, reason, c.Reason, "profile %d", i)
			}
		}
	}

	t.Run("start moved off its bound", func(t *testing.T) {
		spans := append([]sketch.LoopSpan(nil), good...)
		spans[partialAt].TStart = math.Nextafter(spans[partialAt].TStart, 1)
		allUnproven(t, spans, sketch.ReasonSpanBound)
	})
	t.Run("end moved off its bound", func(t *testing.T) {
		spans := append([]sketch.LoopSpan(nil), good...)
		spans[partialAt].TEnd = math.Nextafter(spans[partialAt].TEnd, 0)
		allUnproven(t, spans, sketch.ReasonSpanBound)
	})
	t.Run("an element missing", func(t *testing.T) {
		spans := append(append([]sketch.LoopSpan(nil), good[:partialAt]...), good[partialAt+1:]...)
		allUnproven(t, spans, sketch.ReasonNone)
	})
	t.Run("two cycles", func(t *testing.T) {
		// The outline of the first lobe's cap and the outline of the disc, as one loop.
		circle := large.Entities()[0]
		spans := append([]sketch.LoopSpan(nil), good...)
		spans = append(spans, sketch.LoopSpan{Entity: circle, TStart: 0, TEnd: 1})
		allUnproven(t, spans, sketch.ReasonNone)
	})
}

// TestDeclareLoopSpansNeedsExactBounds pins that a scene whose bounds are not exact
// gives no verdict: a line ending on the interior of a fourth-lobe spline leaves the
// pair unproven, so the whole component, circle included, publishes inexact bounds.
func TestDeclareLoopSpansNeedsExactBounds(t *testing.T) {
	small := lobeScene(t, 3, 4)
	large := lobeScene(t, 4, 4)
	spans := outlineSpans(t, small, large, unionOfAll(t, small))
	flank, ok := large.Entities()[1+4*3+1].(*sketch.FitSpline)
	require.True(t, ok)
	large.CreateLine(large.Origin(), flank.Fit[1])
	loop, err := large.DeclareLoopSpans(spans...)
	require.NoError(t, err)
	judged := 0
	for i, p := range large.Profiles() {
		if !p.Valid {
			continue
		}
		judged++
		c := p.Containment(loop)
		require.Equalf(t, sketch.ContainmentUnproven, c.State, "profile %d", i)
		require.Equalf(t, sketch.ReasonSpanBound, c.Reason, "profile %d", i)
	}
	require.Positive(t, judged)
}

func TestDeclareLoopSpansValidatesItsInput(t *testing.T) {
	s := lobeScene(t, 1, 4)
	other := lobeScene(t, 1, 4)
	circle := s.Entities()[0]

	_, err := s.DeclareLoopSpans()
	require.ErrorIs(t, err, sketch.ErrLoopEmpty)
	_, err = s.DeclareLoopSpans(sketch.LoopSpan{TEnd: 1})
	require.ErrorIs(t, err, sketch.ErrLoopNilEntity)
	_, err = s.DeclareLoopSpans(sketch.LoopSpan{Entity: other.Entities()[0], TEnd: 1})
	require.ErrorIs(t, err, sketch.ErrLoopForeignEntity)
	for name, sp := range map[string]sketch.LoopSpan{
		"NaN":       {Entity: circle, TStart: math.NaN(), TEnd: 0.5},
		"below 0":   {Entity: circle, TStart: -0.1, TEnd: 0.5},
		"above 1":   {Entity: circle, TStart: 0.1, TEnd: 1.5},
		"empty":     {Entity: circle, TStart: 0.5, TEnd: 0.5},
		"infinite":  {Entity: circle, TStart: 0, TEnd: math.Inf(1)},
		"no extent": {Entity: circle},
	} {
		_, err = s.DeclareLoopSpans(sp)
		require.ErrorIsf(t, err, sketch.ErrLoopSpanRange, name)
		require.ErrorIs(t, err, sketch.ErrInvalidLoop)
	}
	span := sketch.LoopSpan{Entity: circle, TStart: 0.1, TEnd: 0.5}
	_, err = s.DeclareLoopSpans(span, sketch.LoopSpan{Entity: circle, TStart: 0.5, TEnd: 0.1})
	require.ErrorIs(t, err, sketch.ErrLoopDuplicateEntity, "the same span in the other order")
	l, err := s.DeclareLoopSpans(span, sketch.LoopSpan{Entity: circle, TStart: 0.5, TEnd: 0.9})
	require.NoError(t, err, "two spans of one entity")
	require.Len(t, l.Entities(), 2)
	require.Len(t, s.Loops(), 1)
}

// TestDeclareLoopSpansWholeEntityMatchesDeclareLoop pins that a span from 0 to 1 is
// the whole entity: the loop reads the same as the same entities declared whole.
func TestDeclareLoopSpansWholeEntityMatchesDeclareLoop(t *testing.T) {
	s := splineSectionScene(t, 8, 4, true)
	var circle sketch.Entity
	for _, e := range s.Entities() {
		if _, ok := e.(*sketch.Circle); ok {
			circle = e
		}
	}
	require.NotNil(t, circle)
	whole, err := s.DeclareLoop(circle)
	require.NoError(t, err)
	spans, err := s.DeclareLoopSpans(sketch.LoopSpan{Entity: circle, TStart: 1, TEnd: 0})
	require.NoError(t, err)
	for i, p := range s.Profiles() {
		require.Equalf(t, p.Containment(whole), p.Containment(spans), "profile %d", i)
	}
}
