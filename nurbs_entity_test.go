package sketch_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// quarterCircleControl returns the three control points + weights + knots of the
// classic rational quadratic NURBS that traces an exact unit quarter circle.
func quarterCircleNURBS(t *testing.T, s *sketch.Sketch) *sketch.NURBS {
	p0 := s.AddPoint(1, 0)
	p1 := s.AddPoint(1, 1)
	p2 := s.AddPoint(0, 1)
	c, err := s.AddNURBS(2, []*sketch.Point{p0, p1, p2},
		[]float64{1, 1 / math.Sqrt2, 1}, []float64{0, 0, 0, 1, 1, 1})
	require.NoError(t, err)
	return c
}

func TestNURBSAddAndAccessors(t *testing.T) {
	s := sketch.New()
	c := quarterCircleNURBS(t, s)
	require.Equal(t, 2, c.Degree())
	require.Equal(t, []float64{0, 0, 0, 1, 1, 1}, c.Knots())
	require.InDeltaSlice(t, []float64{1, 1 / math.Sqrt2, 1}, c.Weights(), 1e-15)
	require.True(t, c.Rational())
	require.Len(t, c.Control, 3)

	// Endpoints interpolate the first/last control point (clamped curve).
	x0, y0 := c.Eval(0)
	require.InDelta(t, 1, x0, 1e-15)
	require.InDelta(t, 0, y0, 1e-15)
	x1, y1 := c.Eval(1)
	require.InDelta(t, 0, x1, 1e-15)
	require.InDelta(t, 1, y1, 1e-15)

	// Copies, not aliases.
	c.Knots()[0] = 99
	require.Equal(t, []float64{0, 0, 0, 1, 1, 1}, c.Knots(), "Knots() returns a copy")
	c.Weights()[0] = 99
	require.InDelta(t, 1, c.Weights()[0], 1e-15, "Weights() returns a copy")

	g := c.Geometry()
	require.Equal(t, 2, g.Degree)
	require.True(t, g.Rational())
}

func TestNURBSNonRationalDefaultWeights(t *testing.T) {
	s := sketch.New()
	p := []*sketch.Point{s.AddPoint(0, 0), s.AddPoint(1, 2), s.AddPoint(3, -1), s.AddPoint(5, 1)}
	c, err := s.AddNURBS(3, p, nil, sketch.ClampedUniformKnots(4, 3))
	require.NoError(t, err)
	require.False(t, c.Rational(), "nil weights → all 1 → non-rational")
	require.Equal(t, []float64{1, 1, 1, 1}, c.Weights())
}

func TestNURBSValidation(t *testing.T) {
	s := sketch.New()
	p := func(n int) []*sketch.Point {
		out := make([]*sketch.Point, n)
		for i := range out {
			out[i] = s.AddPoint(float64(i), 0)
		}
		return out
	}
	uk := sketch.ClampedUniformKnots // length n+degree+1

	cases := []struct {
		name    string
		degree  int
		control []*sketch.Point
		weights []float64
		knots   []float64
	}{
		{"degree < 1", 0, p(3), nil, []float64{0, 0, 1, 1}},
		{"too few control points", 3, p(3), nil, uk(3, 3)},
		{"wrong knot count", 2, p(3), nil, []float64{0, 0, 0, 1, 1}},
		{"non-monotone knots", 2, p(3), nil, []float64{0, 0, 0, 1, 0.5, 1}},
		{"unclamped start", 2, p(3), nil, []float64{0, 0.1, 0.2, 1, 1, 1}},
		{"unclamped end", 2, p(3), nil, []float64{0, 0, 0, 1, 1, 2}},
		{"wrong weight count", 2, p(3), []float64{1, 1}, []float64{0, 0, 0, 1, 1, 1}},
		{"non-positive weight", 2, p(3), []float64{1, 0, 1}, []float64{0, 0, 0, 1, 1, 1}},
		{"empty domain", 2, p(3), nil, []float64{0, 0, 0, 0, 0, 0}},
	}
	for _, tc := range cases {
		_, err := s.AddNURBS(tc.degree, tc.control, tc.weights, tc.knots)
		require.ErrorIsf(t, err, sketch.ErrInvalidShape, "%s rejected", tc.name)
	}

	// A nil control point is rejected, not a deferred panic.
	good := p(3)
	_, err := s.AddNURBS(2, []*sketch.Point{good[0], nil, good[2]}, nil, []float64{0, 0, 0, 1, 1, 1})
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "nil control point rejected")

	// A well-formed curve is accepted.
	_, err = s.AddNURBS(2, p(3), nil, []float64{0, 0, 0, 1, 1, 1})
	require.NoError(t, err)
}

func TestNURBSFreeDOF(t *testing.T) {
	// A free NURBS has DOF 2·(n+1) — only its control points are unknowns; degree,
	// knots and weights are stored structural data, not solver vars.
	s := sketch.New()
	p := []*sketch.Point{s.AddPoint(1, 0), s.AddPoint(1, 1), s.AddPoint(0, 1)}
	_, err := s.AddNURBS(2, p, []float64{1, 1 / math.Sqrt2, 1}, []float64{0, 0, 0, 1, 1, 1})
	require.NoError(t, err)
	require.Equal(t, 6, s.DOF(), "3 control points × 2 = 2(n+1), no weight/knot vars")

	// A degree-3 NURBS with 5 control points: 10 DOF.
	s2 := sketch.New()
	p2 := make([]*sketch.Point, 5)
	for i := range p2 {
		p2[i] = s2.AddPoint(float64(i), float64(i%2))
	}
	_, err = s2.AddNURBS(3, p2, nil, sketch.ClampedUniformKnots(5, 3))
	require.NoError(t, err)
	require.Equal(t, 10, s2.DOF(), "5 control points × 2")
}

func TestNURBSProfileParticipation(t *testing.T) {
	s := sketch.New()
	c := quarterCircleNURBS(t, s)
	// Close the loop back to the origin with two lines.
	o := s.AddPoint(0, 0)
	s.AddLine(c.Control[2], o) // (0,1) → origin
	s.AddLine(o, c.Control[0]) // origin → (1,0)

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "NURBS + two chords bound one region")
	require.True(t, profiles[0].Valid)
	require.InDelta(t, math.Pi/4, profiles[0].Area, 1e-9, "quarter sector area")
	require.Contains(t, profiles[0].Entities, sketch.Entity(c))
}

func TestNURBSSelfIntersectingFlagged(t *testing.T) {
	// A cubic NURBS whose control polygon loops crosses itself; closed by a chord it
	// is a self-intersecting boundary the oracle must NOT bless.
	s := sketch.New()
	p0 := s.AddPoint(0, 0)
	p1 := s.AddPoint(-4.0/3.0, -5.0/12.0)
	p2 := s.AddPoint(-4.0/3.0, -3.0/2.0)
	p3 := s.AddPoint(0, 3.0/4.0)
	_, err := s.AddNURBS(3, []*sketch.Point{p0, p1, p2, p3}, nil, sketch.ClampedUniformKnots(4, 3))
	require.NoError(t, err)
	s.AddLine(p3, p0)

	rep := s.Verify()
	require.False(t, rep.ProfilesValid, "a self-intersecting NURBS loop is not valid")
	require.NotEmpty(t, rep.InvalidProfiles)
	var sawSelfX bool
	for _, p := range rep.InvalidProfiles {
		if p.SelfIntersecting {
			sawSelfX = true
		}
	}
	require.True(t, sawSelfX, "the invalid profile is flagged self-intersecting")
}

func TestNURBSConstructionExcluded(t *testing.T) {
	s := sketch.New()
	c := quarterCircleNURBS(t, s)
	c.SetConstruction(true)
	o := s.AddPoint(0, 0)
	s.AddLine(c.Control[2], o)
	s.AddLine(o, c.Control[0])
	require.Empty(t, s.Profiles(), "a construction NURBS bounds no reported profile")
}

func TestNURBSRoundTrip(t *testing.T) {
	s := sketch.New()
	c := quarterCircleNURBS(t, s)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	require.Contains(t, string(data), `"nurbs"`, "serialized as a distinct entity type")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), 1, "no doubled internal constraints")
	cc, ok := s2.Entities()[0].(*sketch.NURBS)
	require.True(t, ok, "reloads as a NURBS")
	require.Equal(t, c.Degree(), cc.Degree(), "degree preserved")
	require.Equal(t, c.Knots(), cc.Knots(), "knots preserved")
	require.InDeltaSlice(t, c.Weights(), cc.Weights(), 1e-15, "weights preserved")
	require.Len(t, cc.Control, 3, "control points preserved")
	x1, y1 := cc.Eval(1)
	require.InDelta(t, 0, x1, 1e-12)
	require.InDelta(t, 1, y1, 1e-12)
	require.Equal(t, s.DOF(), s2.DOF(), "DOF stable across round-trip")
}

func TestNURBSExportersContainIt(t *testing.T) {
	s := sketch.New()
	quarterCircleNURBS(t, s)

	svg, err := s.SVG()
	require.NoError(t, err)
	require.Contains(t, svg, "<path", "SVG draws the NURBS as a path")

	png, err := s.PNG()
	require.NoError(t, err)
	require.NotEmpty(t, png)

	dxf, err := s.DXF()
	require.NoError(t, err)
	require.Contains(t, dxf, "SPLINE", "DXF emits a native SPLINE for the NURBS")
}

func TestNURBSDXFRationalRoundTrip(t *testing.T) {
	s := sketch.New()
	c := quarterCircleNURBS(t, s)

	dxf, err := s.DXF()
	require.NoError(t, err)

	// Flag 70 = 12 (planar | rational), degree 71 = 2, knot count 72 = 6,
	// control count 73 = 3.
	require.Equal(t, "12", dxfGroup(t, dxf, "SPLINE", "70"))
	require.Equal(t, "2", dxfGroup(t, dxf, "SPLINE", "71"))
	require.Equal(t, "6", dxfGroup(t, dxf, "SPLINE", "72"))
	require.Equal(t, "3", dxfGroup(t, dxf, "SPLINE", "73"))

	// The full knot vector is emitted (group 40).
	knots := dxfGroupAll(dxf, "SPLINE", "40")
	require.Len(t, knots, 6)
	wantKnots := c.Knots()
	for i, k := range knots {
		require.InDelta(t, wantKnots[i], mustFloat(t, k), 1e-9, "knot %d", i)
	}

	// The weights are emitted (group 41) and rebuild the same curve.
	weights := dxfGroupAll(dxf, "SPLINE", "41")
	require.Len(t, weights, 3)
	wantW := c.Weights()
	for i, w := range weights {
		require.InDelta(t, wantW[i], mustFloat(t, w), 1e-6, "weight %d", i) // DXF text is 6-digit
	}

	// Rebuild a NURBS from the DXF-carried structural data + control points and
	// confirm it traces the same quarter circle.
	xs := dxfGroupAll(dxf, "SPLINE", "10")
	ys := dxfGroupAll(dxf, "SPLINE", "20")
	require.Len(t, xs, 3)
	s2 := sketch.New()
	ctrl := make([]*sketch.Point, 3)
	for i := range ctrl {
		ctrl[i] = s2.AddPoint(mustFloat(t, xs[i]), mustFloat(t, ys[i]))
	}
	w := make([]float64, 3)
	for i := range w {
		w[i] = mustFloat(t, weights[i])
	}
	k := make([]float64, 6)
	for i := range k {
		k[i] = mustFloat(t, knots[i])
	}
	rebuilt, err := s2.AddNURBS(2, ctrl, w, k)
	require.NoError(t, err)
	for i := 0; i <= 16; i++ {
		u := float64(i) / 16
		x, y := rebuilt.Eval(u)
		require.InDelta(t, 1, math.Hypot(x, y), 1e-6, "rebuilt NURBS on unit circle at u=%v", u)
	}
}

func TestNURBSNonRationalDXFFlag(t *testing.T) {
	s := sketch.New()
	p := []*sketch.Point{s.AddPoint(0, 0), s.AddPoint(1, 2), s.AddPoint(3, -1), s.AddPoint(5, 1)}
	_, err := s.AddNURBS(3, p, nil, sketch.ClampedUniformKnots(4, 3))
	require.NoError(t, err)

	dxf, err := s.DXF()
	require.NoError(t, err)
	require.Equal(t, "8", dxfGroup(t, dxf, "SPLINE", "70"), "non-rational: planar only, no rational bit")
	require.Empty(t, dxfGroupAll(dxf, "SPLINE", "41"), "non-rational emits no weights")
}

func TestNURBSDXFWorldSpaceControlPoints(t *testing.T) {
	s := tiltedSketch(t)
	c := quarterCircleNURBS(t, s)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	xs := dxfGroupAll(dxf, "SPLINE", "10")
	ys := dxfGroupAll(dxf, "SPLINE", "20")
	zs := dxfGroupAll(dxf, "SPLINE", "30")
	require.Len(t, xs, 3)
	for i, p := range c.Control {
		w := p.World()
		require.InDeltaf(t, w.X, mustFloat(t, xs[i]), 1e-5, "ctrl %d world x", i)
		require.InDeltaf(t, w.Y, mustFloat(t, ys[i]), 1e-5, "ctrl %d world y", i)
		require.InDeltaf(t, w.Z, mustFloat(t, zs[i]), 1e-5, "ctrl %d world z", i)
	}
}

func TestNURBSRemoveEntityKeepsPoints(t *testing.T) {
	s := sketch.New()
	c := quarterCircleNURBS(t, s)
	require.Equal(t, 6, s.DOF())

	require.True(t, s.RemoveEntity(c))
	require.Empty(t, s.Entities())
	// The three control points survive (6 DOF); a NURBS owns no vars to retire.
	require.Equal(t, 6, s.DOF(), "control points remain after removal")
	require.Len(t, s.Points(), 3)
}

func TestNURBSTypedNilEntity(t *testing.T) {
	s := sketch.New()
	_, err := s.WorldPolyline((*sketch.NURBS)(nil))
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
}

func TestNURBSSVGPathDrawn(t *testing.T) {
	s := sketch.New()
	quarterCircleNURBS(t, s)
	svg, err := s.SVG()
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(svg, "<path"), "one path for the one NURBS")
}
