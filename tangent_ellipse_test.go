package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// fixedEllipse builds a rigid rx=6, ry=3 ellipse at the origin with the given
// rotation (radians).
func fixedEllipse(s *sketch.Sketch, rot float64) *sketch.Ellipse {
	e := s.AddEllipse(s.AddPoint(0, 0), 6, 3, rot)
	s.FixEntity(e)
	return e
}

// ellipseTangentGap measures how far the line through p1,p2 is from tangency to
// the ellipse: |center-to-line distance| minus the ellipse's tangent distance
// for that orientation. Zero means tangent. Independent of the residual's own
// algebra (built from world coordinates only).
func ellipseTangentGap(p1, p2, center *sketch.Point, rx, ry, rot float64) float64 {
	ax, ay := p1.X(), p1.Y()
	abx, aby := p2.X()-ax, p2.Y()-ay
	ablen := math.Hypot(abx, aby)
	nx, ny := -aby/ablen, abx/ablen // world unit normal
	h := math.Abs((center.X()-ax)*nx + (center.Y()-ay)*ny)
	u := math.Cos(rot)*nx + math.Sin(rot)*ny
	v := -math.Sin(rot)*nx + math.Cos(rot)*ny
	return math.Hypot(u*rx, v*ry) - h
}

func TestTangentLineEllipse(t *testing.T) {
	// A horizontal line above an axis-aligned ellipse becomes tangent at the top:
	// |y| = ry = 3.
	s := sketch.New()
	e := fixedEllipse(s, 0)
	p1 := s.AddPoint(-5, 4)
	p2 := s.AddPoint(5, 4)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewHorizontal(line))
	s.AddConstraint(sketch.NewTangentEllipse(line, e))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 3, math.Abs(p1.Y()), 1e-6, "tangent to the top: |y| = ry")
	require.InDelta(t, 0, ellipseTangentGap(p1, p2, e.Center, 6, 3, 0), 1e-6)
}

func TestTangentLineEllipseRotated(t *testing.T) {
	// Vertical tangent to an ellipse rotated by π/6 exercises the local-frame
	// transform: the center-to-line distance is √((cos·rx)² + (sin·ry)²).
	rot := math.Pi / 6
	want := math.Hypot(math.Cos(rot)*6, math.Sin(rot)*3)

	s := sketch.New()
	e := fixedEllipse(s, rot)
	p1 := s.AddPoint(7, -5)
	p2 := s.AddPoint(7, 5)
	line := s.AddLine(p1, p2)
	s.AddConstraint(sketch.NewVertical(line))
	s.AddConstraint(sketch.NewTangentEllipse(line, e))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, want, math.Abs(p1.X()), 1e-6, "vertical tangent distance to the rotated ellipse")
	require.InDelta(t, 0, ellipseTangentGap(p1, p2, e.Center, 6, 3, rot), 1e-6)
}

// bottomEllipticalArc builds a rigid rx=6, ry=3 arc at the origin sweeping the
// bottom (eccentric −3π/4 → −π/4, straddling −π/2).
func bottomEllipticalArc(s *sketch.Sketch) *sketch.EllipticalArc {
	c := s.AddPoint(0, 0)
	start := s.AddPoint(6*math.Cos(-3*math.Pi/4), 3*math.Sin(-3*math.Pi/4))
	end := s.AddPoint(6*math.Cos(-math.Pi/4), 3*math.Sin(-math.Pi/4))
	ea := s.AddEllipticalArc(c, start, end, 6, 3, 0)
	s.FixEntity(ea)
	return ea
}

func TestTangentLineEllipticalArcOutOfSweepRejected(t *testing.T) {
	// The line y = −3 is a perfect tangent to the full ellipse, but its contact
	// (eccentric −π/2, the bottom) lies outside a rigid first-quadrant arc. The
	// oracle must not bless it.
	s := sketch.New()
	ea := quarterEllipticalArc(s) // eccentric [0, π/2]
	a := s.AddPoint(-5, -3)
	b := s.AddPoint(5, -3)
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewTangentEllipse(s.AddLine(a, b), ea))

	_, err := s.Solve()
	require.ErrorIs(t, err, sketch.ErrNotConverged, "out-of-sweep tangent is unsatisfiable")
	rep := s.Verify()
	require.False(t, rep.Solvable, "oracle must reject an out-of-sweep elliptical tangent")
}

func TestTangentLineEllipticalArcInSweepSatisfied(t *testing.T) {
	// The same line y = −3, but the arc spans the bottom so the contact (−π/2) is
	// within the sweep: a genuine tangent that solves with the arc unchanged.
	s := sketch.New()
	ea := bottomEllipticalArc(s)
	a := s.AddPoint(-5, -3)
	b := s.AddPoint(5, -3)
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewTangentEllipse(s.AddLine(a, b), ea))

	_, err := s.Solve()
	require.NoError(t, err)
	require.True(t, s.Verify().Solvable)
	require.InDelta(t, 3, ea.Ry(), 1e-9, "the rigid arc is unchanged")
}

func TestTangentLineEllipticalArcEndpoint(t *testing.T) {
	// A line sharing the arc's Start boundary point (6,0) is forced tangent there:
	// the ellipse normal at (6,0) is horizontal, so the line turns vertical.
	s := sketch.New()
	ea := quarterEllipticalArc(s)
	tip := ea.Start // shared boundary point, fixed by FixEntity
	free := s.AddPoint(7, 5)
	s.AddConstraint(sketch.NewTangentEllipse(s.AddLine(tip, free), ea))

	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 6, free.X(), 1e-6, "tangent at (6,0) is the vertical line x = 6")
}

func TestTangentEllipseDOFAndRemoval(t *testing.T) {
	s := sketch.New()
	e := fixedEllipse(s, 0)
	p1 := s.AddPoint(-5, 4)
	p2 := s.AddPoint(5, 4)
	require.Equal(t, 4, s.DOF(), "the free line has four DOF")

	con := sketch.NewTangentEllipse(s.AddLine(p1, p2), e)
	s.AddConstraint(con)
	require.Equal(t, 3, s.DOF(), "tangency removes one DOF")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 4, s.DOF(), "removal restores the DOF")
}

func TestTangentEllipticalArcDOFAndRemoval(t *testing.T) {
	// The elliptical-arc case allocates a sweep slack; its var and inequality row
	// net to zero, so the DOF accounting matches the full-ellipse case.
	s := sketch.New()
	ea := bottomEllipticalArc(s)
	a := s.AddPoint(-5, -3)
	b := s.AddPoint(5, -3)
	require.Equal(t, 4, s.DOF())

	con := sketch.NewTangentEllipse(s.AddLine(a, b), ea)
	s.AddConstraint(con)
	require.Equal(t, 3, s.DOF(), "tangency removes one DOF; the slack nets out")

	require.True(t, s.RemoveConstraint(con))
	require.Equal(t, 4, s.DOF(), "removal restores the DOF (slack retired)")
}

func TestTangentEllipseDegenerateRejected(t *testing.T) {
	// A degenerate ellipse (zero semi-axes) has no well-defined tangent, so the
	// oracle must never bless tangency to it — neither with a zero-length line nor
	// with a real line through its center (the support value would be 0 − 0 = 0).
	t.Run("zero-length line", func(t *testing.T) {
		s := sketch.New()
		e := s.AddEllipse(s.AddPoint(0, 0), 0, 0, 0)
		s.FixEntity(e)
		p1 := s.AddPoint(5, 5)
		p2 := s.AddPoint(5, 5) // coincident: the line has zero length
		s.Fix(p1)
		s.Fix(p2)
		s.AddConstraint(sketch.NewTangentEllipse(s.AddLine(p1, p2), e))

		_, err := s.Solve()
		require.Error(t, err)
		require.False(t, s.Verify().Solvable, "degenerate ellipse + zero-length line")
	})
	t.Run("real line through center", func(t *testing.T) {
		s := sketch.New()
		e := s.AddEllipse(s.AddPoint(0, 0), 0, 0, 0)
		s.FixEntity(e)
		a := s.AddPoint(-1, 0)
		b := s.AddPoint(1, 0) // a real line straight through the center
		s.Fix(a)
		s.Fix(b)
		s.AddConstraint(sketch.NewTangentEllipse(s.AddLine(a, b), e))

		_, err := s.Solve()
		require.Error(t, err)
		require.False(t, s.Verify().Solvable, "degenerate ellipse + line through center")
	})
}

func TestTangentEllipseRoundTrip(t *testing.T) {
	s := sketch.New()
	ea := bottomEllipticalArc(s)
	a := s.AddPoint(-5, -3)
	b := s.AddPoint(5, -3)
	s.Fix(a)
	s.Fix(b)
	s.AddConstraint(sketch.NewTangentEllipse(s.AddLine(a, b), ea))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraint survives reload")
	_, err = s2.Solve()
	require.NoError(t, err)
}
