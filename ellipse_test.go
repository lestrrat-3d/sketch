package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/sketch/units"
	"github.com/stretchr/testify/require"
)

func addEl(s *sketch.Sketch, c *sketch.Point, rx, ry, rot float64) *sketch.Ellipse {
	return s.AddEllipse(geom.NewEllipse(c.Generic(), rx, ry, rot))
}

// pinEllipse grounds the center and pins the axes/rotation with dimensions.
func pinEllipse(s *sketch.Sketch, e *sketch.Ellipse, rx, ry, rotDeg float64) {
	s.Fix(e.Center)
	s.AddConstraint(sketch.NewSemiMajor(e, rx), sketch.NewSemiMinor(e, ry), sketch.NewEllipseRotation(e, rotDeg))
}

func TestEllipseDimensions(t *testing.T) {
	s := sketch.New()
	c := addPt(s, 0, 0)
	e := addEl(s, c, 2, 1, 0) // rough initial shape
	pinEllipse(s, e, 6, 4, 30)

	res := mustSolve(t, s)
	require.Equal(t, 0, res.DOF, "fully constrained")
	require.InDelta(t, 6, e.Rx(), 1e-6, "semi-major")
	require.InDelta(t, 4, e.Ry(), 1e-6, "semi-minor")
	require.InDelta(t, math.Pi/6, e.Rotation(), 1e-6, "rotation (30° in radians)")
}

func TestPointOnEllipse(t *testing.T) {
	s := sketch.New()
	c := addPt(s, 0, 0)
	e := addEl(s, c, 5, 3, 0)
	pinEllipse(s, e, 5, 3, 0)

	// Pin the point to the x axis; the ellipse constraint pushes it to x=±5,
	// and the rough start at (4, 1) selects the +x vertex.
	p := addPt(s, 4, 1)
	s.AddConstraint(sketch.NewPointOnEllipse(p, e), sketch.NewVerticalDistance(c, p, 0))

	mustSolve(t, s)
	require.InDelta(t, 5, p.X(), 1e-6, "on the major vertex")
	require.InDelta(t, 0, p.Y(), 1e-6, "on the x axis")
}

func TestPointOnRotatedEllipse(t *testing.T) {
	s := sketch.New()
	c := addPt(s, 0, 0)
	e := addEl(s, c, 5, 3, math.Pi/2)
	s.Fix(e.Center)
	s.AddConstraint(sketch.NewSemiMajor(e, 5), sketch.NewSemiMinor(e, 3))
	rot := sketch.NewEllipseRotation(e, 0)
	require.NoError(t, rot.SetValue(units.Radians(math.Pi/2)), "set rotation in radians")
	s.AddConstraint(rot)

	// With the frame rotated 90°, the long axis lies along global y: a point
	// pinned to x=0 lands at y=±5.
	p := addPt(s, 0.5, 4)
	s.AddConstraint(sketch.NewPointOnEllipse(p, e), sketch.NewHorizontalDistance(c, p, 0))

	mustSolve(t, s)
	require.InDelta(t, 0, p.X(), 1e-6, "on the y axis")
	require.InDelta(t, 5, p.Y(), 1e-6, "major vertex now along y")
}

func TestEllipseDrivenMeasure(t *testing.T) {
	s := sketch.New()
	c := addPt(s, 0, 0)
	e := addEl(s, c, 5, 3, 0)
	pinEllipse(s, e, 5, 3, 0)

	d := sketch.NewSemiMajor(e, 0)
	d.SetDriven(true)
	s.AddConstraint(d)

	res := mustSolve(t, s)
	require.Zero(t, res.Redundant, "driven dim adds no equation")
	require.InDelta(t, 5, d.Target().Mag(), 1e-6, "measures the semi-major axis")
}

func TestEllipseJSONRoundTrip(t *testing.T) {
	s := sketch.New()
	c := addPt(s, 1, 2)
	e := addEl(s, c, 5, 3, math.Pi/6)
	pinEllipse(s, e, 5, 3, 30)
	p := addPt(s, 4, 3)
	s.AddConstraint(sketch.NewPointOnEllipse(p, e))
	mustSolve(t, s)

	data, err := json.Marshal(s)
	require.NoError(t, err, "marshal")
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2), "unmarshal")
	require.Len(t, s2.Entities(), len(s.Entities()), "entities survive")
	require.Len(t, s2.Constraints(), len(s.Constraints()), "constraints survive")

	res := mustSolve(t, &s2)
	require.Equal(t, 1, res.DOF, "reloaded DOF (point may slide along the ellipse)")
	e2, ok := s2.Entities()[0].(*sketch.Ellipse)
	require.True(t, ok, "ellipse reloaded")
	require.InDelta(t, 5, e2.Rx(), 1e-6, "reloaded semi-major")
	require.InDelta(t, 3, e2.Ry(), 1e-6, "reloaded semi-minor")
	require.InDelta(t, math.Pi/6, e2.Rotation(), 1e-6, "reloaded rotation")
}

func TestEllipseExports(t *testing.T) {
	s := sketch.New()
	c := addPt(s, 0, 0)
	addEl(s, c, 5, 3, math.Pi/4)

	svg, err := s.SVG()
	require.NoError(t, err, "svg")
	require.Contains(t, svg, "<ellipse", "SVG ellipse element")

	dxf, err := s.DXF()
	require.NoError(t, err, "dxf")
	require.Contains(t, dxf, "ELLIPSE", "DXF ellipse entity")
}
