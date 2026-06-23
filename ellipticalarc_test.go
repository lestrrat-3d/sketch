package sketch_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// topHalfEllipse adds a top-half elliptical arc (rx=4, ry=2, from (4,0) to
// (-4,0)) on a fixed center, with both endpoints fixed on the ellipse.
func topHalfEllipse(s *sketch.Sketch) *sketch.EllipticalArc {
	c := s.AddPoint(0, 0)
	start := s.AddPoint(4, 0)
	end := s.AddPoint(-4, 0)
	return s.AddEllipticalArc(c, start, end, 4, 2, 0)
}

func TestEllipticalArcGeometry(t *testing.T) {
	s := newSketch(t)
	ea := topHalfEllipse(s)
	require.InDelta(t, math.Pi, ea.Sweep(), 1e-9, "a half turn in eccentric angle")
	require.InDelta(t, 4, ea.Rx(), 1e-12)
	require.InDelta(t, 2, ea.Ry(), 1e-12)

	g := ea.Geometry()
	pts := g.Polyline(64)
	require.InDelta(t, 4, pts[0][0], 1e-9)
	require.InDelta(t, -4, pts[len(pts)-1][0], 1e-9)
	for _, p := range pts {
		f := (p[0]/4)*(p[0]/4) + (p[1]/2)*(p[1]/2)
		require.InDelta(t, 1, f, 1e-9, "every sample on the ellipse")
	}
}

func TestEllipticalArcInternalConstraintsAndDOF(t *testing.T) {
	s := newSketch(t)
	c := s.AddPoint(0, 0)
	// Endpoints start slightly OFF the ellipse; the auto-added internal
	// constraints pull them on after a solve.
	start := s.AddPoint(4.3, 0.1)
	end := s.AddPoint(-3.8, 0.2)
	ea := s.AddEllipticalArc(c, start, end, 4, 2, 0)

	require.Equal(t, 7, s.DOF(), "free elliptical arc: 9 vars − 2 internal on-ellipse")

	_, err := s.Solve()
	require.NoError(t, err)
	// The solver may move the ellipse as well as the points, so evaluate the
	// on-ellipse condition against the SOLVED ellipse (center/axes/rotation).
	g := ea.Geometry()
	cosr, sinr := math.Cos(g.Rotation), math.Sin(g.Rotation)
	for _, p := range []*sketch.Point{start, end} {
		dx, dy := p.X()-g.Center.X, p.Y()-g.Center.Y
		lx := cosr*dx + sinr*dy
		ly := -sinr*dx + cosr*dy
		f := (lx/g.Rx)*(lx/g.Rx) + (ly/g.Ry)*(ly/g.Ry)
		require.InDelta(t, 1, f, 1e-6, "endpoint pulled onto the ellipse")
	}
}

func TestEllipticalArcProfile(t *testing.T) {
	s := newSketch(t)
	ea := topHalfEllipse(s)
	s.AddLine(ea.End, ea.Start) // chord closing the half-ellipse

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "the arc plus its chord close one region")
	require.InDelta(t, 0.5*math.Pi*4*2, profiles[0].Area, 1e-2, "half the ellipse area")
	require.True(t, profiles[0].Valid)
}

func TestEllipticalArcRoundTrip(t *testing.T) {
	s := newSketch(t)
	c := s.AddPoint(0, 0)
	s.Fix(c)
	start := s.AddPoint(4, 0)
	end := s.AddPoint(0, 2)
	s.Fix(start)
	s.Fix(end)
	s.AddEllipticalArc(c, start, end, 4, 2, 0)
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), 1, "the elliptical arc survives reload")
	require.Len(t, s2.Constraints(), len(s.Constraints()),
		"internal on-ellipse constraints are recreated, not doubled")

	_, err = s2.Solve()
	require.NoError(t, err)
	ea, ok := s2.Entities()[0].(*sketch.EllipticalArc)
	require.True(t, ok)
	require.InDelta(t, 4, ea.Rx(), 1e-9)
	require.InDelta(t, 2, ea.Ry(), 1e-9)
}

func TestEllipticalArcExportAndFix(t *testing.T) {
	s := newSketch(t)
	ea := topHalfEllipse(s)

	svg, err := s.SVG()
	require.NoError(t, err)
	require.Contains(t, svg, "<path", "rendered as a sampled path")

	dxf, err := s.DXF()
	require.NoError(t, err)
	require.Contains(t, dxf, "ELLIPSE", "exported as a native DXF ellipse")

	s.FixEntity(ea)
	require.True(t, s.EntityFixed(ea), "FixEntity grounds the shape vars + points")
	require.True(t, strings.Contains(svg, "path"))
}
