package sketch_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/units"
	"github.com/stretchr/testify/require"
)

func TestEllipseDimensions(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	e := s.CreateEllipse(c, 2, 1, 0) // rough initial shape
	s.Fix(e.Center)
	s.AddConstraint(sketch.NewSemiMajor(e, 6), sketch.NewSemiMinor(e, 4), sketch.NewEllipseRotation(e, 30))

	res, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, res.DOF, "fully constrained")
	require.InDelta(t, 6, e.Rx(), 1e-6, "semi-major")
	require.InDelta(t, 4, e.Ry(), 1e-6, "semi-minor")
	require.InDelta(t, math.Pi/6, e.Rotation(), 1e-6, "rotation (30° in radians)")
}

func TestPointOnEllipse(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	e := s.CreateEllipse(c, 5, 3, 0)
	s.Fix(e.Center)
	s.AddConstraint(sketch.NewSemiMajor(e, 5), sketch.NewSemiMinor(e, 3), sketch.NewEllipseRotation(e, 0))

	// Pin the point to the x axis; the ellipse constraint pushes it to x=±5,
	// and the rough start at (4, 1) selects the +x vertex.
	p := s.CreatePoint(4, 1)
	s.AddConstraint(sketch.NewPointOnEllipse(p, e), sketch.NewVerticalDistance(c, p, 0))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 5, p.X(), 1e-6, "on the major vertex")
	require.InDelta(t, 0, p.Y(), 1e-6, "on the x axis")
}

func TestPointOnRotatedEllipse(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	e := s.CreateEllipse(c, 5, 3, math.Pi/2)
	s.Fix(e.Center)
	s.AddConstraint(sketch.NewSemiMajor(e, 5), sketch.NewSemiMinor(e, 3))
	rot := sketch.NewEllipseRotation(e, 0)
	require.NoError(t, rot.SetValue(units.Radians(math.Pi/2)), "set rotation in radians")
	s.AddConstraint(rot)

	// With the frame rotated 90°, the long axis lies along global y: a point
	// pinned to x=0 lands at y=±5.
	p := s.CreatePoint(0.5, 4)
	s.AddConstraint(sketch.NewPointOnEllipse(p, e), sketch.NewHorizontalDistance(c, p, 0))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 0, p.X(), 1e-6, "on the y axis")
	require.InDelta(t, 5, p.Y(), 1e-6, "major vertex now along y")
}

func TestEllipseDrivenMeasure(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	e := s.CreateEllipse(c, 5, 3, 0)
	s.Fix(e.Center)
	s.AddConstraint(sketch.NewSemiMajor(e, 5), sketch.NewSemiMinor(e, 3), sketch.NewEllipseRotation(e, 0))

	d := sketch.NewSemiMajor(e, 0)
	d.SetDriven(true)
	s.AddConstraint(d)

	res, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.Zero(t, res.Redundant, "driven dim adds no equation")
	require.InDelta(t, 5, d.Target().Mag(), 1e-6, "measures the semi-major axis")
}

func TestEllipseJSONRoundTrip(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(1, 2)
	e := s.CreateEllipse(c, 5, 3, math.Pi/6)
	s.Fix(e.Center)
	s.AddConstraint(sketch.NewSemiMajor(e, 5), sketch.NewSemiMinor(e, 3), sketch.NewEllipseRotation(e, 30))
	p := s.CreatePoint(4, 3)
	s.AddConstraint(sketch.NewPointOnEllipse(p, e))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	require.Len(t, s2.Entities(), len(s.Entities()), "entities survive")
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive")

	res, err := s2.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, res.DOF, "reloaded DOF (point may slide along the ellipse)")
	e2, ok := s2.Entities()[0].(*sketch.Ellipse)
	require.True(t, ok, "ellipse reloaded")
	require.InDelta(t, 5, e2.Rx(), 1e-6, "reloaded semi-major")
	require.InDelta(t, 3, e2.Ry(), 1e-6, "reloaded semi-minor")
	require.InDelta(t, math.Pi/6, e2.Rotation(), 1e-6, "reloaded rotation")
}

func TestEllipseExports(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	s.CreateEllipse(c, 5, 3, math.Pi/4)

	svg, err := s.SVG()
	require.NoError(t, err, "svg")
	require.Contains(t, svg, "<ellipse", "SVG ellipse element")

	dxf, err := s.DXF()
	require.NoError(t, err, "dxf")
	require.Contains(t, dxf, "ELLIPSE", "DXF ellipse entity")
}

// topHalfEllipse adds a top-half elliptical arc (rx=4, ry=2, from (4,0) to
// (-4,0)) on a fixed center, with both endpoints fixed on the ellipse.
func topHalfEllipse(s *sketch.Sketch) *sketch.EllipticalArc {
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0)
	end := s.CreatePoint(-4, 0)
	return s.CreateEllipticalArc(c, start, end, 4, 2, 0)
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
	c := s.CreatePoint(0, 0)
	// Endpoints start slightly OFF the ellipse; the auto-added internal
	// constraints pull them on after a solve.
	start := s.CreatePoint(4.3, 0.1)
	end := s.CreatePoint(-3.8, 0.2)
	ea := s.CreateEllipticalArc(c, start, end, 4, 2, 0)

	require.Equal(t, 7, s.DOF(), "free elliptical arc: 9 vars − 2 internal on-ellipse")

	_, err := s.Solve(t.Context())
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
	s.CreateLine(ea.End, ea.Start) // chord closing the half-ellipse

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "the arc plus its chord close one region")
	require.InDelta(t, 0.5*math.Pi*4*2, profiles[0].Area, 1e-2, "half the ellipse area")
	require.True(t, profiles[0].Valid)
}

func TestEllipticalArcRoundTrip(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	s.Fix(c)
	start := s.CreatePoint(4, 0)
	end := s.CreatePoint(0, 2)
	s.Fix(start)
	s.Fix(end)
	s.CreateEllipticalArc(c, start, end, 4, 2, 0)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), 1, "the elliptical arc survives reload")
	require.Len(t, s2.Constraints(), len(s.Constraints()),
		"internal on-ellipse constraints are recreated, not doubled")

	_, err = s2.Solve(t.Context())
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

func TestEllipticalArcShapeDimensions(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	s.Fix(c)
	start := s.CreatePoint(5, 0)
	end := s.CreatePoint(0, 3)
	ea := s.CreateEllipticalArc(c, start, end, 5, 3, 0)

	// Drive the arc's underlying-ellipse shape via the widened dimensions.
	s.AddConstraint(
		sketch.NewSemiMajor(ea, 8),
		sketch.NewSemiMinor(ea, 4),
		sketch.NewEllipseRotation(ea, 0),
	)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 8, ea.Rx(), 1e-6, "semi-major driven")
	require.InDelta(t, 4, ea.Ry(), 1e-6, "semi-minor driven")
	require.InDelta(t, 0, ea.Rotation(), 1e-6, "rotation driven")
}

func TestEllipticalArcShapeDimensionsRoundTrip(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	s.Fix(c)
	start := s.CreatePoint(6, 0)
	end := s.CreatePoint(0, 2)
	ea := s.CreateEllipticalArc(c, start, end, 6, 2, 0)
	s.AddConstraint(sketch.NewSemiMajor(ea, 6), sketch.NewEllipseRotation(ea, 0))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "dims on the elliptical arc survive reload")
	_, err = s2.Solve(t.Context())
	require.NoError(t, err)
}

// The widening is non-breaking: a plain ellipse still works with the same dims.
func TestEllipseShapeDimensionsStillWork(t *testing.T) {
	s := newSketch(t)
	o := s.CreatePoint(0, 0)
	s.Fix(o)
	e := s.CreateEllipse(o, 3, 2, 0)
	s.AddConstraint(sketch.NewSemiMajor(e, 10), sketch.NewSemiMinor(e, 5))
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 10, e.Rx(), 1e-6)
	require.InDelta(t, 5, e.Ry(), 1e-6)
}
